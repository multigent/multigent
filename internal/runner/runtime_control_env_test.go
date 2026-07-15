package runner

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/multigent/multigent/internal/entity"
)

func TestNormalizeRuntimeAPIURL(t *testing.T) {
	tests := map[string]string{
		"127.0.0.1:27893":       "http://127.0.0.1:27893",
		":27893":                "http://127.0.0.1:27893",
		"http://localhost:123/": "http://localhost:123",
		"0.0.0.0:27893":         "http://127.0.0.1:27893",
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			if got := normalizeRuntimeAPIURL(input); got != want {
				t.Fatalf("normalizeRuntimeAPIURL(%q)=%q, want %q", input, got, want)
			}
		})
	}
}

func TestInjectRuntimeControlEnvIntoRuntimeUsesInheritedEnv(t *testing.T) {
	cfg := &entity.SandboxConfig{}
	injectRuntimeControlEnvIntoRuntime(cfg, map[string]string{
		"MULTIGENT_AGENT_TOKEN": "secret-token",
		"MULTIGENT_API_URL":     "http://127.0.0.1:27893",
	})
	if len(cfg.Env) != 2 {
		t.Fatalf("env=%#v", cfg.Env)
	}
	for _, env := range cfg.Env {
		if !env.Inherit {
			t.Fatalf("runtime env should inherit rather than embed value: %#v", env)
		}
		if env.Value != "" || env.SecretRef != "" {
			t.Fatalf("runtime env leaked value: %#v", env)
		}
	}
}

func TestInjectProviderEnvIntoRuntimeSkipsRuntimeControlKeys(t *testing.T) {
	cfg := &entity.SandboxConfig{}
	injectProviderEnvIntoRuntime(cfg, map[string]string{
		"MULTIGENT_AGENT_TOKEN":   "user-token",
		runtimeConnectionsFileEnv: "/tmp/connections.json",
		"MULTIGENT_API_URL":       "http://example.invalid",
		"OPENAI_API_KEY":          "provider-key",
	})
	if len(cfg.Env) != 1 {
		t.Fatalf("env=%#v", cfg.Env)
	}
	if cfg.Env[0].Name != "OPENAI_API_KEY" || cfg.Env[0].Value != "provider-key" {
		t.Fatalf("provider env not preserved: %#v", cfg.Env)
	}
}

func TestMaterializeRuntimeConnectionsFile(t *testing.T) {
	const token = "agent-runtime-token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/runtime/connections" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Fatalf("authorization=%q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"project":"p","agent":"a","manifest":{"version":"multigent.connections.v1"},"connections":[{"id":"conn_1","provider":"custom-mcp"}]}`))
	}))
	defer server.Close()

	agentDir := t.TempDir()
	env := map[string]string{
		"MULTIGENT_API_URL":     server.URL,
		"MULTIGENT_AGENT_TOKEN": token,
	}
	cleanup := (&Runner{}).materializeRuntimeConnectionsFile(agentDir, env)
	if cleanup == nil {
		t.Fatalf("expected cleanup")
	}
	path := env[runtimeConnectionsFileEnv]
	if path == "" {
		t.Fatalf("expected %s", runtimeConnectionsFileEnv)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, `"connections"`) || !strings.Contains(text, `"conn_1"`) {
		t.Fatalf("unexpected manifest: %s", text)
	}
	if strings.Contains(text, token) {
		t.Fatalf("manifest leaked agent token: %s", text)
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("manifest file should be removed, err=%v", err)
	}
}

func TestMaterializeRuntimeConnectionsFileSkipsWithoutRuntimeEnv(t *testing.T) {
	env := map[string]string{"MULTIGENT_API_URL": "http://127.0.0.1:1"}
	cleanup := (&Runner{}).materializeRuntimeConnectionsFile(t.TempDir(), env)
	if cleanup != nil {
		t.Fatalf("expected no cleanup")
	}
	if env[runtimeConnectionsFileEnv] != "" {
		t.Fatalf("unexpected manifest path: %q", env[runtimeConnectionsFileEnv])
	}
}
