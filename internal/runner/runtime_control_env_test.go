package runner

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/multigent/multigent/internal/daemon"
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

func TestResolveRuntimeAPIURLFallsBackToDaemonAcrossWorkspaces(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("MULTIGENT_DATA_DIR", dataDir)
	t.Setenv("MULTIGENT_API_URL", "")
	if err := daemon.SaveMeta(&daemon.Meta{
		WorkDir: t.TempDir(),
		Addr:    "0.0.0.0:27892",
	}); err != nil {
		t.Fatal(err)
	}
	if got := resolveRuntimeAPIURL(filepath.Join(dataDir, "another-workspace")); got != "http://127.0.0.1:27892" {
		t.Fatalf("resolveRuntimeAPIURL()=%q", got)
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
	if env[runtimeConnectionsFileEnv] == "" || env[runtimeToolsFileEnv] == "" || env[runtimeToolDirEnv] == "" || env[runtimeToolSkillsFileEnv] == "" {
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
	guideBody, err := os.ReadFile(env[runtimeToolSkillsFileEnv])
	if err != nil {
		t.Fatalf("read tool skill guide: %v", err)
	}
	guideText := string(guideBody)
	for _, want := range []string{"# Runtime Tool Skills", "GitHub", "gh --help", "mga runtime tools --format table"} {
		if !strings.Contains(guideText, want) {
			t.Fatalf("guide missing %q: %s", want, guideText)
		}
	}
	if env[runtimeMCPConfigEnv] != "1" {
		t.Fatalf("expected MCP config marker env, got %#v", env[runtimeMCPConfigEnv])
	}
	for _, path := range []string{
		filepath.Join(agentDir, ".mcp.json"),
		filepath.Join(agentDir, ".cursor", "mcp.json"),
		filepath.Join(agentDir, ".multigent", "runtime-home", string(entity.ModelCodex), ".codex", "config.toml"),
		filepath.Join(agentDir, ".multigent", "runtime-home", string(entity.ModelQoder), ".codex", "config.toml"),
		filepath.Join(agentDir, ".multigent", "runtime-home", string(entity.ModelCursor), ".cursor", "mcp.json"),
	} {
		cfgBody, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read MCP config %s: %v", path, err)
		}
		text := string(cfgBody)
		if !strings.Contains(text, "multigent") || !strings.Contains(text, "mcp-server") {
			t.Fatalf("MCP config missing gateway entry %s: %s", path, text)
		}
		if strings.Contains(text, token) {
			t.Fatalf("MCP config leaked token %s: %s", path, text)
		}
	}
}

func TestWriteRuntimeMCPClientConfigsMergesExistingConfig(t *testing.T) {
	agentDir := t.TempDir()
	cursorPath := filepath.Join(agentDir, ".cursor", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(cursorPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cursorPath, []byte(`{"mcpServers":{"existing":{"command":"existing-mcp"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	codexPath := filepath.Join(agentDir, ".multigent", "runtime-home", string(entity.ModelCodex), ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(codexPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codexPath, []byte("[projects.\"/workspace\"]\ntrust_level = \"trusted\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeRuntimeMCPClientConfigs(agentDir); err != nil {
		t.Fatalf("write MCP configs: %v", err)
	}
	cursorBody, err := os.ReadFile(cursorPath)
	if err != nil {
		t.Fatal(err)
	}
	cursorText := string(cursorBody)
	for _, want := range []string{"existing-mcp", "multigent", "mcp-server"} {
		if !strings.Contains(cursorText, want) {
			t.Fatalf("cursor config missing %q: %s", want, cursorText)
		}
	}
	codexBody, err := os.ReadFile(codexPath)
	if err != nil {
		t.Fatal(err)
	}
	codexText := string(codexBody)
	for _, want := range []string{"trust_level", "BEGIN MULTIGENT MCP", "[mcp_servers.multigent]", "env_vars"} {
		if !strings.Contains(codexText, want) {
			t.Fatalf("codex config missing %q: %s", want, codexText)
		}
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
	toolDir, toolsPath, env, err := writeRuntimeToolsFile("", agentDir, "run-gh", "/tmp/connections.json", body, func(connectionID string) (map[string]string, bool, error) {
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

func TestWriteRuntimeToolsFileMaterializesLarkCLIConfig(t *testing.T) {
	body := []byte(`{
		"tools":[{
			"provider":"feishu",
			"displayName":"Feishu",
			"connectionId":"conn_feishu",
			"connectionAlias":"feishu-main",
			"connectionName":"Main Feishu",
			"recommendedAdapter":"cli",
			"skills":["lark-doc","lark-im"],
			"adapters":[{
				"type":"cli",
				"priority":100,
				"skills":["lark-doc","lark-im"],
				"cli":{
					"binary":"lark-cli",
					"installer":{"type":"npm","package":"@larksuite/cli","version":"latest","check":["lark-cli --version"]},
					"configFiles":[{"path":"~/.lark-cli/config.json","format":"json"}]
				},
				"credentialMaterialize":"runtime_file"
			}]
		}]
	}`)
	workspaceRoot := t.TempDir()
	agentDir := filepath.Join(workspaceRoot, "projects", "sample", "agents", "pm")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("mkdir agent dir: %v", err)
	}
	toolDir, toolsPath, env, err := writeRuntimeToolsFile(workspaceRoot, agentDir, "run-lark", "/tmp/connections.json", body, func(connectionID string) (map[string]string, bool, error) {
		if connectionID != "conn_feishu" {
			t.Fatalf("connectionID=%q", connectionID)
		}
		return map[string]string{"appId": "cli_a_test", "appSecret": "secret_test"}, true, nil
	})
	if err != nil {
		t.Fatalf("write tools file: %v", err)
	}
	if toolDir == "" || toolsPath == "" {
		t.Fatalf("toolDir=%q toolsPath=%q", toolDir, toolsPath)
	}
	larkHome := env["MULTIGENT_LARK_HOME"]
	if larkHome == "" || !strings.Contains(larkHome, toolDir) {
		t.Fatalf("MULTIGENT_LARK_HOME=%q toolDir=%q", larkHome, toolDir)
	}
	configPath := filepath.Join(larkHome, ".lark-cli", "config.json")
	configBody, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config.json: %v", err)
	}
	configText := string(configBody)
	for _, want := range []string{`"appId": "cli_a_test"`, `"appSecret": "secret_test"`, `"brand": "feishu"`} {
		if !strings.Contains(configText, want) {
			t.Fatalf("config missing %q: %s", want, configText)
		}
	}
	wrapperPath := filepath.Join(toolDir, "bin", "lark-cli")
	info, err := os.Stat(wrapperPath)
	if err != nil {
		t.Fatalf("stat wrapper: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("wrapper is not executable: %v", info.Mode())
	}
	wrapperBody, err := os.ReadFile(wrapperPath)
	if err != nil {
		t.Fatalf("read wrapper: %v", err)
	}
	wrapperText := string(wrapperBody)
	if !strings.Contains(wrapperText, "'lark-cli' \"$@\"") || !strings.Contains(wrapperText, larkHome) || !strings.Contains(wrapperText, "MULTIGENT_TOOL_CLI_AUDIT_FILE") {
		t.Fatalf("unexpected wrapper: %s", string(wrapperBody))
	}
	if env[runtimeToolCLIAuditEnv] == "" || !strings.Contains(env[runtimeToolCLIAuditEnv], toolDir) {
		t.Fatalf("cli audit env=%q toolDir=%q", env[runtimeToolCLIAuditEnv], toolDir)
	}
	toolsBody, err := os.ReadFile(toolsPath)
	if err != nil {
		t.Fatalf("read tools file: %v", err)
	}
	if strings.Contains(string(toolsBody), "secret_test") {
		t.Fatalf("tools file leaked app secret: %s", string(toolsBody))
	}
	guidePath := env[runtimeToolSkillsFileEnv]
	if guidePath == "" || !strings.Contains(guidePath, toolDir) {
		t.Fatalf("tool skill guide path=%q toolDir=%q", guidePath, toolDir)
	}
	guideBody, err := os.ReadFile(guidePath)
	if err != nil {
		t.Fatalf("read tool skill guide: %v", err)
	}
	guideText := string(guideBody)
	for _, want := range []string{"Feishu", "lark-cli --help", "lark-doc", "lark-im", "Adapter `cli`"} {
		if !strings.Contains(guideText, want) {
			t.Fatalf("guide missing %q: %s", want, guideText)
		}
	}
	bootstrapPath := filepath.Join(toolDir, "bootstrap-tools.sh")
	bootstrapBody, err := os.ReadFile(bootstrapPath)
	if err != nil {
		t.Fatalf("read bootstrap-tools.sh: %v", err)
	}
	bootstrapText := string(bootstrapBody)
	if !strings.Contains(bootstrapText, "npm install -g '@larksuite/cli'") || !strings.Contains(bootstrapText, "lark-cli --version") || !strings.Contains(bootstrapText, filepath.Join(workspaceRoot, ".multigent", "tool-cache", "npm")) {
		t.Fatalf("unexpected bootstrap script: %s", bootstrapText)
	}
}

func TestWriteRuntimeToolsFileMaterializesBasicExternalToolCredentials(t *testing.T) {
	body := []byte(`{
		"tools":[
			{
				"provider":"git_ssh",
				"connectionId":"conn_git",
				"connectionAlias":"git-ssh",
				"connectionName":"Git SSH",
				"adapters":[{
					"type":"cli",
					"priority":100,
					"cli":{
						"binary":"git",
						"configFiles":[
							{"path":"~/.ssh/id_git_multigent","format":"pem"},
							{"path":"~/.ssh/known_hosts","format":"text"},
							{"path":"~/.gitconfig","format":"ini"}
						]
					},
					"credentialMaterialize":"runtime_file"
				}]
			},
			{
				"provider":"npm_registry",
				"connectionId":"conn_npm",
				"connectionAlias":"npm",
				"connectionName":"npm",
				"adapters":[{
					"type":"cli",
					"priority":100,
					"cli":{"binary":"npm","configFiles":[{"path":"~/.npmrc","format":"ini"}]},
					"credentialMaterialize":"runtime_file"
				}]
			},
			{
				"provider":"docker_registry",
				"connectionId":"conn_docker",
				"connectionAlias":"docker",
				"connectionName":"Docker",
				"adapters":[{
					"type":"cli",
					"priority":100,
					"cli":{"binary":"docker","configFiles":[{"path":"~/.docker/config.json","format":"json"}]},
					"credentialMaterialize":"runtime_file"
				}]
			},
			{
				"provider":"aws",
				"connectionId":"conn_aws",
				"connectionAlias":"aws",
				"connectionName":"AWS",
				"adapters":[{
					"type":"cli",
					"priority":100,
					"cli":{"binary":"aws","configFiles":[{"path":"~/.aws/credentials","format":"ini"},{"path":"~/.aws/config","format":"ini"}]},
					"credentialMaterialize":"runtime_file"
				}]
			},
			{
				"provider":"gcloud",
				"connectionId":"conn_gcloud",
				"connectionAlias":"gcloud",
				"connectionName":"Google Cloud",
				"adapters":[{
					"type":"cli",
					"priority":100,
					"cli":{"binary":"gcloud","configFiles":[{"path":"~/.config/gcloud/application_default_credentials.json","format":"json"}]},
					"credentialMaterialize":"runtime_file"
				}]
			},
			{
				"provider":"cloudflare",
				"connectionId":"conn_cloudflare",
				"connectionAlias":"cloudflare",
				"connectionName":"Cloudflare",
				"adapters":[{
					"type":"cli",
					"priority":100,
					"cli":{"binary":"wrangler","configFiles":[{"path":"~/.cloudflare/env","format":"env"}]},
					"credentialMaterialize":"runtime_env"
				}]
			}
		]
	}`)
	agentDir := t.TempDir()
	toolDir, toolsPath, env, err := writeRuntimeToolsFile("", agentDir, "run-basic-tools", "/tmp/connections.json", body, func(connectionID string) (map[string]string, bool, error) {
		switch connectionID {
		case "conn_git":
			return map[string]string{
				"privateKey":   "-----BEGIN OPENSSH PRIVATE KEY-----\nkey-body\n-----END OPENSSH PRIVATE KEY-----",
				"gitUserName":  "Multigent Agent",
				"gitUserEmail": "agent@example.test",
				"proxyJump":    "root@120.79.164.240",
				"knownHosts":   "github.com ssh-ed25519 AAAATEST",
			}, true, nil
		case "conn_npm":
			return map[string]string{
				"registryUrl": "https://registry.npmjs.org/",
				"scope":       "@acme",
				"authToken":   "npm-secret-token",
				"alwaysAuth":  "true",
			}, true, nil
		case "conn_docker":
			return map[string]string{
				"registryUrl": "ghcr.io",
				"username":    "octo",
				"password":    "docker-secret-token",
			}, true, nil
		case "conn_aws":
			return map[string]string{
				"accessKeyId":     "AKIATEST",
				"secretAccessKey": "aws-secret-key",
				"sessionToken":    "aws-session-token",
				"region":          "us-west-2",
				"profile":         "multigent",
			}, true, nil
		case "conn_gcloud":
			return map[string]string{
				"serviceAccountJson": `{"type":"service_account","project_id":"demo-project","private_key_id":"kid","private_key":"key","client_email":"svc@example.test","client_id":"123"}`,
				"projectId":          "demo-project",
				"region":             "us-central1",
				"zone":               "us-central1-a",
			}, true, nil
		case "conn_cloudflare":
			return map[string]string{
				"apiKey":    "cf-secret-token",
				"accountId": "cf-account",
				"zoneId":    "cf-zone",
			}, true, nil
		default:
			t.Fatalf("unexpected connectionID=%q", connectionID)
			return nil, false, nil
		}
	})
	if err != nil {
		t.Fatalf("write tools file: %v", err)
	}
	if toolDir == "" || toolsPath == "" {
		t.Fatalf("toolDir=%q toolsPath=%q", toolDir, toolsPath)
	}
	if env["GIT_SSH_COMMAND"] == "" || !strings.Contains(env["GIT_SSH_COMMAND"], "id_git_multigent") {
		t.Fatalf("GIT_SSH_COMMAND=%q", env["GIT_SSH_COMMAND"])
	}
	if !strings.Contains(env["GIT_SSH_COMMAND"], "-J root@120.79.164.240") {
		t.Fatalf("GIT_SSH_COMMAND missing ProxyJump: %q", env["GIT_SSH_COMMAND"])
	}
	gitKey := env["MULTIGENT_GIT_SSH_KEY_FILE"]
	if gitKey == "" {
		t.Fatalf("git key env missing: %#v", env)
	}
	keyBody, err := os.ReadFile(gitKey)
	if err != nil {
		t.Fatalf("read git key: %v", err)
	}
	if !strings.Contains(string(keyBody), "BEGIN OPENSSH PRIVATE KEY") {
		t.Fatalf("unexpected key body: %s", string(keyBody))
	}
	knownHostsBody, err := os.ReadFile(filepath.Join(filepath.Dir(gitKey), "known_hosts"))
	if err != nil {
		t.Fatalf("read known_hosts: %v", err)
	}
	if !strings.Contains(string(knownHostsBody), "github.com ssh-ed25519") {
		t.Fatalf("unexpected known_hosts: %s", string(knownHostsBody))
	}
	gitConfigPath := env["GIT_CONFIG_GLOBAL"]
	if gitConfigPath == "" {
		t.Fatalf("GIT_CONFIG_GLOBAL missing: %#v", env)
	}
	gitConfigBody, err := os.ReadFile(gitConfigPath)
	if err != nil {
		t.Fatalf("read git config: %v", err)
	}
	if !strings.Contains(string(gitConfigBody), "name = Multigent Agent") || !strings.Contains(string(gitConfigBody), "email = agent@example.test") {
		t.Fatalf("unexpected git config: %s", string(gitConfigBody))
	}
	npmrcPath := env["NPM_CONFIG_USERCONFIG"]
	npmrcBody, err := os.ReadFile(npmrcPath)
	if err != nil {
		t.Fatalf("read npmrc: %v", err)
	}
	if !strings.Contains(string(npmrcBody), "@acme:registry=https://registry.npmjs.org/") || !strings.Contains(string(npmrcBody), "_authToken=npm-secret-token") {
		t.Fatalf("unexpected npmrc: %s", string(npmrcBody))
	}
	dockerConfigPath := filepath.Join(env["DOCKER_CONFIG"], "config.json")
	dockerBody, err := os.ReadFile(dockerConfigPath)
	if err != nil {
		t.Fatalf("read docker config: %v", err)
	}
	if !strings.Contains(string(dockerBody), "ghcr.io") || !strings.Contains(string(dockerBody), base64.StdEncoding.EncodeToString([]byte("octo:docker-secret-token"))) {
		t.Fatalf("unexpected docker config: %s", string(dockerBody))
	}
	awsCredentialsBody, err := os.ReadFile(env["AWS_SHARED_CREDENTIALS_FILE"])
	if err != nil {
		t.Fatalf("read aws credentials: %v", err)
	}
	if !strings.Contains(string(awsCredentialsBody), "aws_access_key_id = AKIATEST") || !strings.Contains(string(awsCredentialsBody), "aws_session_token = aws-session-token") {
		t.Fatalf("unexpected aws credentials: %s", string(awsCredentialsBody))
	}
	if env["AWS_PROFILE"] != "multigent" || env["AWS_REGION"] != "us-west-2" || env["AWS_CONFIG_FILE"] == "" {
		t.Fatalf("unexpected aws env: %#v", env)
	}
	gcloudPath := env["GOOGLE_APPLICATION_CREDENTIALS"]
	gcloudBody, err := os.ReadFile(gcloudPath)
	if err != nil {
		t.Fatalf("read gcloud credentials: %v", err)
	}
	if !strings.Contains(string(gcloudBody), `"type":"service_account"`) || env["CLOUDSDK_CORE_PROJECT"] != "demo-project" || env["CLOUDSDK_COMPUTE_ZONE"] != "us-central1-a" {
		t.Fatalf("unexpected gcloud materialization: env=%#v body=%s", env, string(gcloudBody))
	}
	if env["CLOUDFLARE_API_TOKEN"] != "cf-secret-token" || env["CLOUDFLARE_ACCOUNT_ID"] != "cf-account" || env["CLOUDFLARE_ZONE_ID"] != "cf-zone" {
		t.Fatalf("unexpected cloudflare env: %#v", env)
	}
	toolsBody, err := os.ReadFile(toolsPath)
	if err != nil {
		t.Fatalf("read tools plan: %v", err)
	}
	for _, secret := range []string{"npm-secret-token", "docker-secret-token", "key-body", "aws-secret-key", "aws-session-token", "cf-secret-token"} {
		if strings.Contains(string(toolsBody), secret) {
			t.Fatalf("tools file leaked %q: %s", secret, string(toolsBody))
		}
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

func TestDockerRuntimeControlEnvUsesHostGateway(t *testing.T) {
	env := map[string]string{
		"MULTIGENT_API_URL":     "http://127.0.0.1:27893",
		"MULTIGENT_AGENT_TOKEN": "token",
	}
	got := runtimeControlEnvForProvider(env, entity.SandboxDocker)
	if got["MULTIGENT_API_URL"] != "http://host.docker.internal:27893" {
		t.Fatalf("MULTIGENT_API_URL=%q", got["MULTIGENT_API_URL"])
	}
	if env["MULTIGENT_API_URL"] != "http://127.0.0.1:27893" {
		t.Fatalf("mutated source env: %q", env["MULTIGENT_API_URL"])
	}
}
