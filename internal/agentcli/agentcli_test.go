package agentcli

import (
	"strings"
	"testing"

	"github.com/multigent/multigent/internal/entity"
)

func TestDefaultForModelCodexUsesManagedNPMInstaller(t *testing.T) {
	cfg := DefaultForModel(entity.ModelCodex)
	if cfg == nil {
		t.Fatal("DefaultForModel(codex) returned nil")
	}
	if cfg.Binary != "codex" {
		t.Fatalf("Binary = %q, want codex", cfg.Binary)
	}
	if cfg.PackageManager != "npm" {
		t.Fatalf("PackageManager = %q, want npm", cfg.PackageManager)
	}
	if cfg.Package != "@openai/codex" {
		t.Fatalf("Package = %q, want @openai/codex", cfg.Package)
	}
}

func TestWrapCommandAddsBootstrapAndExecsOriginalCommand(t *testing.T) {
	cfg := &entity.AgentCLIConfig{
		Vendor:         "codex",
		Version:        "1.2.3",
		Binary:         "codex",
		PackageManager: "npm",
		Package:        "@openai/codex",
	}
	got := WrapCommand([]string{"codex", "exec", "-"}, cfg)
	if len(got) < 7 {
		t.Fatalf("wrapped command too short: %#v", got)
	}
	if got[0] != "/bin/sh" || got[1] != "-lc" {
		t.Fatalf("wrapped command prefix = %#v", got[:2])
	}
	if !strings.Contains(got[2], "npm install -g @openai/codex@1.2.3") {
		t.Fatalf("bootstrap script missing versioned npm install:\n%s", got[2])
	}
	if got[len(got)-3] != "codex" || got[len(got)-2] != "exec" || got[len(got)-1] != "-" {
		t.Fatalf("original command not preserved at tail: %#v", got)
	}
}

func TestNormalizeRewritesRemovedCodexPresetToLatest(t *testing.T) {
	cfg := Normalize(&entity.AgentCLIConfig{
		Vendor:         "codex",
		Version:        "0.18.0",
		Binary:         "codex",
		PackageManager: "npm",
		Package:        "@openai/codex",
	})
	if cfg.Version != "latest" {
		t.Fatalf("Version = %q, want latest", cfg.Version)
	}
}
