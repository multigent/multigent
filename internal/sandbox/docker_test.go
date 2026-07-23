package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/multigent/multigent/internal/entity"
)

func TestBuildArgsBinPATHKeepsToolchainPaths(t *testing.T) {
	root := t.TempDir()
	agentDir := filepath.Join(root, "projects", "demo", "agents", "dev")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("create agent dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatalf("create bin dir: %v", err)
	}

	args, err := BuildArgs(agentDir, entity.ModelCodex, nil, []string{"codex", "exec", "-"})
	if err != nil {
		t.Fatalf("BuildArgs: %v", err)
	}

	pathEnv := findEnvArg(args, "PATH=")
	if pathEnv == "" {
		t.Fatalf("PATH env not found in args: %v", args)
	}
	for _, want := range []string{UserBin, "/usr/local/go/bin", "/root/go/bin", "/usr/local/bin"} {
		if !strings.Contains(pathEnv, want) {
			t.Fatalf("PATH %q missing %s", pathEnv, want)
		}
	}
}

func TestBuildArgsMountsWorkspaceAtStableContainerPath(t *testing.T) {
	root := t.TempDir()
	agentDir := filepath.Join(root, "projects", "demo", "agents", "dev")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("create agent dir: %v", err)
	}

	args, err := BuildArgs(agentDir, entity.ModelClaudeCode, nil, []string{"claude", "-p"})
	if err != nil {
		t.Fatalf("BuildArgs: %v", err)
	}
	absAgentDir, err := filepath.Abs(agentDir)
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}

	joined := strings.Join(args, "\n")
	if !strings.Contains(joined, absAgentDir+":"+WorkspaceMount) {
		t.Fatalf("missing workspace mount %q in args:\n%s", absAgentDir+":"+WorkspaceMount, joined)
	}
	if got := argAfter(args, "-w"); got != WorkspaceMount {
		t.Fatalf("workdir = %q, want %q; args=%v", got, WorkspaceMount, args)
	}
}

func TestImageForManagedModelsUsesRuntimeBase(t *testing.T) {
	restore := dockerImageExists
	dockerImageExists = func(string) bool { return false }
	t.Cleanup(func() { dockerImageExists = restore })
	for _, model := range []entity.AgentModel{entity.ModelCodex, entity.ModelClaudeCode, entity.ModelGemini, entity.ModelQoder, entity.ModelCursor} {
		if got := ImageForModel(model); got != BaseImage {
			t.Fatalf("ImageForModel(%s) = %q, want %q", model, got, BaseImage)
		}
	}
}

func TestEffectiveImagePrefersLocalRuntimeBaseWhenPresent(t *testing.T) {
	restore := dockerImageExists
	dockerImageExists = func(image string) bool { return image == LocalBaseImage }
	t.Cleanup(func() { dockerImageExists = restore })
	cfg := &entity.DockerSandboxConfig{Image: BaseImage}
	if got := EffectiveImage(entity.ModelCodex, cfg); got != LocalBaseImage {
		t.Fatalf("EffectiveImage() = %q, want %q", got, LocalBaseImage)
	}
}

func TestEffectiveImageUsesPublishedRuntimeBaseWhenLocalMissing(t *testing.T) {
	restore := dockerImageExists
	dockerImageExists = func(string) bool { return false }
	t.Cleanup(func() { dockerImageExists = restore })
	cfg := &entity.DockerSandboxConfig{Image: LocalBaseImage}
	if got := EffectiveImage(entity.ModelCodex, cfg); got != BaseImage {
		t.Fatalf("EffectiveImage() = %q, want %q", got, BaseImage)
	}
}

func TestEffectiveImageNormalizesManagedLegacySandboxImages(t *testing.T) {
	restore := dockerImageExists
	dockerImageExists = func(string) bool { return false }
	t.Cleanup(func() { dockerImageExists = restore })
	cfg := &entity.DockerSandboxConfig{Image: "ghcr.io/multigent/multigent/sandbox-claudecode:latest"}
	if got := EffectiveImage(entity.ModelCursor, cfg); got != BaseImage {
		t.Fatalf("EffectiveImage() = %q, want %q", got, BaseImage)
	}
}

func TestBuildArgsUsesAgentScopedRuntimeHome(t *testing.T) {
	root := t.TempDir()
	agentDir := filepath.Join(root, "projects", "demo", "agents", "dev")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("create agent dir: %v", err)
	}
	args, err := BuildArgs(agentDir, entity.ModelCodex, nil, []string{"codex", "exec", "-"})
	if err != nil {
		t.Fatalf("BuildArgs: %v", err)
	}
	joined := strings.Join(args, "\n")
	if strings.Contains(joined, "~/.codex") || strings.Contains(joined, "~/.claude") || strings.Contains(joined, "~/.ssh") {
		t.Fatalf("global host credential mount leaked into args:\n%s", joined)
	}
	want := filepath.Join(agentDir, ".multigent", "runtime-home", "codex", ".codex") + ":/root/.codex"
	if !strings.Contains(joined, want) {
		t.Fatalf("missing agent-scoped codex mount %q in args:\n%s", want, joined)
	}
}

func TestDockerExecutableHonorsOverride(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "docker")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write docker shim: %v", err)
	}
	t.Setenv("MULTIGENT_DOCKER", bin)
	if got := DockerExecutable(); got != bin {
		t.Fatalf("DockerExecutable() = %q, want %q", got, bin)
	}
}

func findEnvArg(args []string, prefix string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-e" && strings.HasPrefix(args[i+1], prefix) {
			return args[i+1]
		}
	}
	return ""
}

func argAfter(args []string, flag string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}
