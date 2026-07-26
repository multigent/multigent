package api

import (
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

func TestModelAccountNotRequiredForHumanOrHTTPAgent(t *testing.T) {
	for _, model := range []entity.AgentModel{entity.ModelHuman, entity.ModelHTTPAgent} {
		if modelRequiresModelAccount(model) {
			t.Fatalf("expected %s to run without model account binding", model)
		}
	}
}
