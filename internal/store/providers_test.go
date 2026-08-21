package store

import (
	"testing"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/secretbox"
)

func TestProviderEnvForClaudeCodeUsesAnthropicCompatibleVars(t *testing.T) {
	provider := entity.APIProvider{
		Type:    "openai",
		APIKey:  "secret-key",
		BaseURL: "https://gateway.example.com/anthropic",
		Model:   "claude-compatible-model",
	}

	env := ProviderEnvForModel(provider, entity.ModelClaudeCode)

	if env["ANTHROPIC_AUTH_TOKEN"] != "secret-key" {
		t.Fatalf("ANTHROPIC_AUTH_TOKEN=%q", env["ANTHROPIC_AUTH_TOKEN"])
	}
	if env["ANTHROPIC_API_KEY"] != "secret-key" {
		t.Fatalf("ANTHROPIC_API_KEY=%q", env["ANTHROPIC_API_KEY"])
	}
	if env["ANTHROPIC_BASE_URL"] != provider.BaseURL {
		t.Fatalf("ANTHROPIC_BASE_URL=%q", env["ANTHROPIC_BASE_URL"])
	}
	if env["ANTHROPIC_MODEL"] != provider.Model {
		t.Fatalf("ANTHROPIC_MODEL=%q", env["ANTHROPIC_MODEL"])
	}
	if env["ANTHROPIC_DEFAULT_SONNET_MODEL"] != provider.Model {
		t.Fatalf("ANTHROPIC_DEFAULT_SONNET_MODEL=%q", env["ANTHROPIC_DEFAULT_SONNET_MODEL"])
	}
}

func TestProviderEnvForCodexUsesOpenAICompatibleVars(t *testing.T) {
	provider := entity.APIProvider{
		Type:    "anthropic",
		APIKey:  "secret-key",
		BaseURL: "https://gateway.example.com/v1",
		Model:   "openai-compatible-model",
	}

	env := ProviderEnvForModel(provider, entity.ModelCodex)

	if env["OPENAI_API_KEY"] != "secret-key" {
		t.Fatalf("OPENAI_API_KEY=%q", env["OPENAI_API_KEY"])
	}
	if env["CODEX_API_KEY"] != "secret-key" {
		t.Fatalf("CODEX_API_KEY=%q", env["CODEX_API_KEY"])
	}
	if env["OPENAI_BASE_URL"] != provider.BaseURL {
		t.Fatalf("OPENAI_BASE_URL=%q", env["OPENAI_BASE_URL"])
	}
	if env["OPENAI_MODEL"] != provider.Model {
		t.Fatalf("OPENAI_MODEL=%q", env["OPENAI_MODEL"])
	}
	if env["CODEX_MODEL"] != provider.Model {
		t.Fatalf("CODEX_MODEL=%q", env["CODEX_MODEL"])
	}
	if env["ANTHROPIC_AUTH_TOKEN"] != "" {
		t.Fatalf("unexpected ANTHROPIC_AUTH_TOKEN=%q", env["ANTHROPIC_AUTH_TOKEN"])
	}
}

func TestProviderStoreGetFallsBackAcrossWorkspacesWhenRootIsDataRoot(t *testing.T) {
	dataRoot := t.TempDir()
	workspaceRoot := dataRoot + "/workspace-one"
	t.Setenv("MULTIGENT_DATA_DIR", dataRoot)
	db, err := controldb.OpenDefault()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	now := time.Now().UTC().Format(time.RFC3339)
	if err := db.UpsertWorkspace(controldb.Workspace{
		ID:        "ws-one",
		Name:      "Workspace One",
		Slug:      "workspace-one",
		Root:      workspaceRoot,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("workspace: %v", err)
	}
	sealed, err := secretbox.SealString("sk-test")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if err := db.UpsertModelProvider("ws-one", controldb.ModelProvider{
		ID:        "prov-codex",
		Name:      "Codex",
		Type:      "openai",
		APIKey:    sealed,
		Model:     "gpt-5.5",
		EnvJSON:   "{}",
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("provider: %v", err)
	}
	env, err := NewProviderStore(dataRoot).ResolveEnvForModel("prov-codex", entity.ModelCodex)
	if err != nil {
		t.Fatalf("resolve env from data root: %v", err)
	}
	if env["OPENAI_API_KEY"] != "sk-test" || env["CODEX_MODEL"] != "gpt-5.5" {
		t.Fatalf("unexpected env: %#v", env)
	}
	credentialDir, err := NewProviderStore(dataRoot).CredentialDir("prov-codex", entity.ModelCodex)
	if err != nil {
		t.Fatalf("credential dir from data root: %v", err)
	}
	if want := ProviderCredentialDir(workspaceRoot, "prov-codex", entity.ModelCodex); credentialDir != want {
		t.Fatalf("credential dir = %q, want %q", credentialDir, want)
	}
}
