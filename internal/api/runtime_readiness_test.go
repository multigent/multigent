package api

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/multigent/multigent/internal/entity"
)

func TestRuntimeReadinessRequiresExplicitModelAccount(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "host-env-key")
	t.Setenv("OPENAI_API_KEY", "host-env-key")

	meta := &entity.AgentMeta{
		Name:  "pm",
		Model: entity.ModelClaudeCode,
		Sandbox: &entity.SandboxConfig{
			Provider: entity.SandboxNone,
		},
	}

	readiness := buildRuntimeReadinessLight(meta)
	if !readiness.Blocking {
		t.Fatalf("expected readiness to block when agent has no explicit model account")
	}
	for _, check := range readiness.Checks {
		if check.Key == "provider" && check.Blocking && check.Status == "error" {
			return
		}
	}
	t.Fatalf("expected blocking provider check, got %#v", readiness.Checks)
}

func TestRuntimeReadinessAllowsExplicitDirectHostProcess(t *testing.T) {
	t.Setenv("MULTIGENT_ALLOW_DIRECT_HOST", "true")
	binDir := t.TempDir()
	codexPath := filepath.Join(binDir, "codex")
	if err := os.WriteFile(codexPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake codex: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	meta := &entity.AgentMeta{
		Name:     "dev",
		Model:    entity.ModelCodex,
		Provider: "openai-main",
		Sandbox: &entity.SandboxConfig{
			Provider: entity.SandboxNone,
		},
	}

	readiness := buildRuntimeReadinessLight(meta)
	if readiness.Blocking {
		t.Fatalf("expected explicit direct host process to be runnable, got %#v", readiness.Checks)
	}
	for _, check := range readiness.Checks {
		if check.Key == "docker" || check.Key == "runtime_image" || check.Key == "runtime_container" {
			t.Fatalf("explicit direct host process should not require docker checks, got %#v", readiness.Checks)
		}
	}
}

func TestRuntimeReadinessBlocksDirectHostWhenDisabled(t *testing.T) {
	t.Setenv("MULTIGENT_ALLOW_DIRECT_HOST", "false")
	meta := &entity.AgentMeta{
		Name:     "dev",
		Model:    entity.ModelCodex,
		Provider: "openai-main",
		Sandbox: &entity.SandboxConfig{
			Provider: entity.SandboxNone,
		},
	}

	readiness := buildRuntimeReadinessLight(meta)
	if !readiness.Blocking {
		t.Fatalf("expected direct host process to be blocked when disabled, got %#v", readiness.Checks)
	}
	for _, check := range readiness.Checks {
		if check.Key == "sandbox" && check.Blocking && check.Status == "error" {
			return
		}
	}
	t.Fatalf("expected blocking sandbox check, got %#v", readiness.Checks)
}

func TestNormalizeSandboxProviderAliases(t *testing.T) {
	for _, input := range []string{"", "none", "direct", "host", "local", " DIRECT "} {
		if got := normalizeSandboxProvider(input); got != entity.SandboxNone {
			t.Fatalf("normalizeSandboxProvider(%q) = %q, want none", input, got)
		}
	}
	if got := normalizeSandboxProvider("docker"); got != entity.SandboxDocker {
		t.Fatalf("normalizeSandboxProvider(docker) = %q", got)
	}
	if got := normalizeSandboxProvider("e2b"); got != entity.SandboxE2B {
		t.Fatalf("normalizeSandboxProvider(e2b) = %q", got)
	}
}

func TestModelAccountNotRequiredForHumanOrHTTPAgent(t *testing.T) {
	for _, model := range []entity.AgentModel{entity.ModelHuman, entity.ModelHTTPAgent} {
		if modelRequiresModelAccount(model) {
			t.Fatalf("expected %s to run without model account binding", model)
		}
	}
}
