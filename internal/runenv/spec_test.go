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
		Image:    sandbox.BaseImage,
		Docker:   &entity.DockerSandboxConfig{Image: sandbox.BaseImage},
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
		"PATH=" + runtimecli.BinDir,
		agentcli.ToolchainBin,
		sandbox.BaseImage,
		"npm install -g @openai/codex@1.2.3",
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
	want := "PATH=/agent/.multigent/runtime-tools/run/bin:" + runtimecli.BinDir
	if !strings.Contains(joined, want) {
		t.Fatalf("docker args missing tool bin path %q:\n%s", want, joined)
	}
}
