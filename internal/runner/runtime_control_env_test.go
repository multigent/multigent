package runner

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
		runtimeToolsFileEnv:       "/tmp/tools.json",
		runtimeToolDirEnv:         "/tmp/tool-runtime",
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
		"MULTIGENT_RUN_ID":      "run-one",
	}
	cleanup := (&Runner{}).materializeRuntimeFiles(agentDir, env)
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
	toolsPath := env[runtimeToolsFileEnv]
	if toolsPath != "" {
		t.Fatalf("did not expect tools file without tools payload: %q", toolsPath)
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("manifest file should be removed, err=%v", err)
	}
}

func TestMaterializeRuntimeFilesWritesToolPlan(t *testing.T) {
	const token = "agent-runtime-token"
	body := `{
		"project":"p",
		"agent":"a",
		"manifest":{"version":"multigent.connections.v1"},
		"connections":[],
		"tools":[{
			"provider":"github",
			"displayName":"GitHub",
			"connectionId":"conn_gh",
			"connectionAlias":"github",
			"connectionName":"default",
			"recommendedAdapter":"cli",
			"skills":["github"],
			"adapters":[{
				"type":"cli",
				"priority":100,
				"skills":["github"],
				"cli":{
					"binary":"gh",
					"configFiles":[{"path":"~/.config/gh/hosts.yml","format":"yaml"}]
				},
				"credentialMaterialize":"runtime_file"
			}]
		}]
	}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	agentDir := t.TempDir()
	env := map[string]string{
		"MULTIGENT_API_URL":     server.URL,
		"MULTIGENT_AGENT_TOKEN": token,
		"MULTIGENT_RUN_ID":      "task/123",
	}
	cleanup := (&Runner{}).materializeRuntimeFiles(agentDir, env)
	if cleanup == nil {
		t.Fatalf("expected cleanup")
	}
	defer cleanup()
	if env[runtimeConnectionsFileEnv] == "" || env[runtimeToolsFileEnv] == "" || env[runtimeToolDirEnv] == "" {
		t.Fatalf("runtime env missing files: %#v", env)
	}
	planBody, err := os.ReadFile(env[runtimeToolsFileEnv])
	if err != nil {
		t.Fatalf("read tools file: %v", err)
	}
	if strings.Contains(string(planBody), token) {
		t.Fatalf("tools file leaked token: %s", string(planBody))
	}
	var plan struct {
		Version string `json:"version"`
		Tools   []struct {
			Provider string `json:"provider"`
			Adapters []struct {
				CLI *struct {
					ConfigFiles []struct {
						MaterializedPath string `json:"materializedPath"`
					} `json:"configFiles"`
				} `json:"cli"`
			} `json:"adapters"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(planBody, &plan); err != nil {
		t.Fatalf("decode tools plan: %v", err)
	}
	if plan.Version != "multigent.tools.v1" || len(plan.Tools) != 1 || plan.Tools[0].Provider != "github" {
		t.Fatalf("unexpected tools plan: %s", string(planBody))
	}
	materializedPath := plan.Tools[0].Adapters[0].CLI.ConfigFiles[0].MaterializedPath
	if materializedPath == "" || !strings.Contains(materializedPath, env[runtimeToolDirEnv]) {
		t.Fatalf("materialized config path not scoped to runtime dir: %q", materializedPath)
	}
	if _, err := os.Stat(env[runtimeToolDirEnv]); err != nil {
		t.Fatalf("runtime tool dir missing: %v", err)
	}
}

func TestWriteRuntimeToolsFileMaterializesGitHubCLIConfig(t *testing.T) {
	body := []byte(`{
		"tools":[{
			"provider":"github",
			"displayName":"GitHub",
			"connectionId":"conn_gh",
			"connectionAlias":"github",
			"connectionName":"default",
			"recommendedAdapter":"cli",
			"skills":["github"],
			"adapters":[{
				"type":"cli",
				"priority":100,
				"skills":["github"],
				"cli":{
					"binary":"gh",
					"configFiles":[{"path":"~/.config/gh/hosts.yml","format":"yaml"}]
				},
				"credentialMaterialize":"runtime_file"
			}]
		}]
	}`)
	agentDir := t.TempDir()
	toolDir, toolsPath, env, err := writeRuntimeToolsFile(agentDir, "run-gh", "/tmp/connections.json", body, func(connectionID string) (map[string]string, bool, error) {
		if connectionID != "conn_gh" {
			t.Fatalf("connectionID=%q", connectionID)
		}
		return map[string]string{"apiKey": "ghp_test_token"}, true, nil
	})
	if err != nil {
		t.Fatalf("write tools file: %v", err)
	}
	if toolDir == "" || toolsPath == "" {
		t.Fatalf("toolDir=%q toolsPath=%q", toolDir, toolsPath)
	}
	ghConfigDir := env["GH_CONFIG_DIR"]
	if ghConfigDir == "" || !strings.Contains(ghConfigDir, toolDir) {
		t.Fatalf("GH_CONFIG_DIR=%q toolDir=%q", ghConfigDir, toolDir)
	}
	hostsPath := filepath.Join(ghConfigDir, "hosts.yml")
	hostsBody, err := os.ReadFile(hostsPath)
	if err != nil {
		t.Fatalf("read hosts.yml: %v", err)
	}
	if !strings.Contains(string(hostsBody), "ghp_test_token") || !strings.Contains(string(hostsBody), "git_protocol: https") {
		t.Fatalf("unexpected hosts.yml: %s", string(hostsBody))
	}
	toolsBody, err := os.ReadFile(toolsPath)
	if err != nil {
		t.Fatalf("read tools file: %v", err)
	}
	if strings.Contains(string(toolsBody), "ghp_test_token") {
		t.Fatalf("tools file leaked token: %s", string(toolsBody))
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
