package store

import (
	"testing"

	"github.com/multigent/multigent/internal/entity"
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
	if env["OPENAI_BASE_URL"] != provider.BaseURL {
		t.Fatalf("OPENAI_BASE_URL=%q", env["OPENAI_BASE_URL"])
	}
	if env["OPENAI_MODEL"] != provider.Model {
		t.Fatalf("OPENAI_MODEL=%q", env["OPENAI_MODEL"])
	}
	if env["ANTHROPIC_AUTH_TOKEN"] != "" {
		t.Fatalf("unexpected ANTHROPIC_AUTH_TOKEN=%q", env["ANTHROPIC_AUTH_TOKEN"])
	}
}
