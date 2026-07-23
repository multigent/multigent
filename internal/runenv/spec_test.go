package runenv

import (
	"strings"
	"testing"

	"github.com/multigent/multigent/internal/agentcli"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/runtimecli"
	"github.com/multigent/multigent/internal/sandbox"
)

func TestDockerProviderWrapsManagedAgentCLI(t *testing.T) {
	dir := t.TempDir()
	runtime := &entity.SandboxConfig{
		Provider: entity.SandboxDocker,
		Image:    "example/runtime-base:test",
		Docker:   &entity.DockerSandboxConfig{Image: "example/runtime-base:test"},
	}
	cli := &entity.AgentCLIConfig{
		Vendor:         "codex",
		Version:        "1.2.3",
		Binary:         "codex",
		PackageManager: "npm",
		Package:        "@openai/codex",
	}

	_, args, err := DockerProvider{}.Command(ProcessSpec{
		AgentDir: dir,
		Model:    entity.ModelCodex,
		Runtime:  runtime,
		AgentCLI: cli,
		Command:  []string{"codex", "exec", "-"},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}

	joined := strings.Join(args, "\n")
	for _, want := range []string{
		"multigent-toolchains:" + agentcli.ToolchainHome,
		"PATH=" + runtimecli.ManagedBinDir,
		runtimecli.BinDir,
		agentcli.ToolchainBin,
		"example/runtime-base:test",
		"npm install -g --no-audit --no-fund --loglevel=notice @openai/codex@1.2.3",
		"MULTIGENT_AGENT_CLI_INSTALL_TIMEOUT",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("docker args missing %q:\n%s", want, joined)
		}
	}
}

func TestDockerProviderPrependsRuntimeToolBin(t *testing.T) {
	dir := t.TempDir()
	runtime := &entity.SandboxConfig{
		Provider: entity.SandboxDocker,
		Image:    sandbox.BaseImage,
		Docker:   &entity.DockerSandboxConfig{Image: sandbox.BaseImage},
		Env: []entity.RuntimeEnvVar{
			{Name: "MULTIGENT_TOOL_BIN_DIR", Value: "/agent/.multigent/runtime-tools/run/bin"},
			{Name: "MULTIGENT_TOOL_CACHE_BIN_DIR", Value: "/workspace/.multigent/tool-cache/npm/bin"},
		},
	}

	_, args, err := DockerProvider{}.Command(ProcessSpec{
		AgentDir: dir,
		Model:    entity.ModelCodex,
		Runtime:  runtime,
		Command:  []string{"codex", "exec", "-"},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}

	joined := strings.Join(args, "\n")
	want := "PATH=/agent/.multigent/runtime-tools/run/bin:/workspace/.multigent/tool-cache/npm/bin:" + runtimecli.ManagedBinDir + ":" + runtimecli.BinDir
	if !strings.Contains(joined, want) {
		t.Fatalf("docker args missing tool bin path %q:\n%s", want, joined)
	}
}

func TestDockerProviderRunsRuntimeToolBootstrap(t *testing.T) {
	dir := t.TempDir()
	runtime := &entity.SandboxConfig{
		Provider: entity.SandboxDocker,
		Image:    sandbox.BaseImage,
		Docker:   &entity.DockerSandboxConfig{Image: sandbox.BaseImage},
		Env: []entity.RuntimeEnvVar{
			{Name: "MULTIGENT_TOOL_BOOTSTRAP_FILE", Value: "/agent/.multigent/runtime-tools/run/bootstrap-tools.sh"},
		},
	}

	_, args, err := DockerProvider{}.Command(ProcessSpec{
		AgentDir: dir,
		Model:    entity.ModelCodex,
		Runtime:  runtime,
		Command:  []string{"codex", "exec", "-"},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}

	joined := strings.Join(args, "\n")
	if !strings.Contains(joined, "/agent/.multigent/runtime-tools/run/bootstrap-tools.sh") {
		t.Fatalf("docker args missing bootstrap script:\n%s", joined)
	}
	if !strings.Contains(joined, "exec \"$@\"") {
		t.Fatalf("docker args missing command handoff:\n%s", joined)
	}
}
