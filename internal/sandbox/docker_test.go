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
	if err := os.MkdirAll(filepath.Join(root, ".multigent"), 0o755); err != nil {
		t.Fatalf("create workspace meta dir: %v", err)
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

func TestFindCursorAgentBinaryFallsBackFromServicePATH(t *testing.T) {
	home := t.TempDir()
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	launcher := filepath.Join(binDir, "agent")
	if err := os.WriteFile(launcher, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", "/usr/bin:/bin")

	got, err := findCursorAgentBinary()
	if err != nil {
		t.Fatalf("findCursorAgentBinary: %v", err)
	}
	if got != launcher {
		t.Fatalf("binary = %q, want %q", got, launcher)
	}
}

func TestBuildArgsDoesNotMountSystemBinForNonWorkspaceTempDir(t *testing.T) {
	agentDir := t.TempDir()
	args, err := BuildArgs(agentDir, entity.ModelClaudeCode, nil, []string{"claude", "-p"})
	if err != nil {
		t.Fatalf("BuildArgs: %v", err)
	}
	joined := strings.Join(args, "\n")
	if strings.Contains(joined, string(filepath.Separator)+"bin:"+UserBin) {
		t.Fatalf("unexpected system bin mount in args:\n%s", joined)
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
	t.Setenv(EnvRuntimeImage, "")
	t.Setenv(EnvRuntimeRegion, "")
	restore := dockerImageExists
	dockerImageExists = func(string) bool { return false }
	t.Cleanup(func() { dockerImageExists = restore })
	for _, model := range []entity.AgentModel{entity.ModelCodex, entity.ModelClaudeCode, entity.ModelGemini, entity.ModelQoder, entity.ModelCursor} {
		if got := ImageForModel(model); got != BaseImage {
			t.Fatalf("ImageForModel(%s) = %q, want %q", model, got, BaseImage)
		}
	}
}

func TestImageForManagedModelsUsesChinaMirrorWhenConfigured(t *testing.T) {
	t.Setenv(EnvRuntimeImage, "")
	t.Setenv(EnvRuntimeRegion, "cn")
	restore := dockerImageExists
	dockerImageExists = func(string) bool { return false }
	t.Cleanup(func() { dockerImageExists = restore })
	if got := ImageForModel(entity.ModelCodex); got != ChinaBaseImage {
		t.Fatalf("ImageForModel() = %q, want %q", got, ChinaBaseImage)
	}
}

func TestImageForManagedModelsUsesExplicitRuntimeImage(t *testing.T) {
	t.Setenv(EnvRuntimeImage, "registry.example.com/multigent/runtime-base:latest")
	t.Setenv(EnvRuntimeRegion, "cn")
	restore := dockerImageExists
	dockerImageExists = func(string) bool { return false }
	t.Cleanup(func() { dockerImageExists = restore })
	if got := ImageForModel(entity.ModelCodex); got != "registry.example.com/multigent/runtime-base:latest" {
		t.Fatalf("ImageForModel() = %q", got)
	}
}

func TestEffectiveImagePrefersLocalRuntimeBaseWhenPresent(t *testing.T) {
	t.Setenv(EnvRuntimeImage, "")
	t.Setenv(EnvRuntimeRegion, "")
	restore := dockerImageExists
	dockerImageExists = func(image string) bool { return image == LocalBaseImage }
	t.Cleanup(func() { dockerImageExists = restore })
	cfg := &entity.DockerSandboxConfig{Image: BaseImage}
	if got := EffectiveImage(entity.ModelCodex, cfg); got != LocalBaseImage {
		t.Fatalf("EffectiveImage() = %q, want %q", got, LocalBaseImage)
	}
}

func TestEffectiveImageUsesPublishedRuntimeBaseWhenLocalMissing(t *testing.T) {
	t.Setenv(EnvRuntimeImage, "")
	t.Setenv(EnvRuntimeRegion, "")
	restore := dockerImageExists
	dockerImageExists = func(string) bool { return false }
	t.Cleanup(func() { dockerImageExists = restore })
	cfg := &entity.DockerSandboxConfig{Image: LocalBaseImage}
	if got := EffectiveImage(entity.ModelCodex, cfg); got != BaseImage {
		t.Fatalf("EffectiveImage() = %q, want %q", got, BaseImage)
	}
}

func TestEffectiveImageNormalizesManagedLegacySandboxImages(t *testing.T) {
	t.Setenv(EnvRuntimeImage, "")
	t.Setenv(EnvRuntimeRegion, "")
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

func TestBuildArgsForwardsLoopbackProxyIntoDocker(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:7890")
	t.Setenv("HTTP_PROXY", "http://localhost:7890")
	t.Setenv("NO_PROXY", "localhost,127.0.0.1")
	args, err := BuildArgs(t.TempDir(), entity.ModelClaudeCode, nil, []string{"claude", "-p"})
	if err != nil {
		t.Fatalf("BuildArgs: %v", err)
	}
	if !hasExactEnvArg(args, "HTTPS_PROXY") || !hasExactEnvArg(args, "HTTP_PROXY") {
		t.Fatalf("loopback proxy should be inherited into docker args: %v", args)
	}
	if !hasExactEnvArg(args, "NO_PROXY") {
		t.Fatalf("NO_PROXY should still be forwarded: %v", args)
	}
}

func TestDockerReachableProxyEnvValueRewritesLoopback(t *testing.T) {
	cases := map[string]string{
		"HTTPS_PROXY=http://127.0.0.1:7890":  "http://host.docker.internal:7890",
		"HTTP_PROXY=http://localhost:7890":   "http://host.docker.internal:7890",
		"http_proxy=http://[::1]:7890":       "http://host.docker.internal:7890",
		"ALL_PROXY=socks5h://127.0.0.1:7890": "socks5h://host.docker.internal:7890",
		"NO_PROXY=localhost,127.0.0.1":       "localhost,127.0.0.1",
		"HTTPS_PROXY=https://proxy.example":  "https://proxy.example",
	}
	for input, want := range cases {
		key, value, _ := strings.Cut(input, "=")
		if got := DockerReachableProxyEnvValue(key, value); got != want {
			t.Fatalf("DockerReachableProxyEnvValue(%q, %q) = %q, want %q", key, value, got, want)
		}
	}
}

func TestBuildArgsForwardsContainerReachableProxyIntoDocker(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://host.docker.internal:7890")
	args, err := BuildArgs(t.TempDir(), entity.ModelClaudeCode, nil, []string{"claude", "-p"})
	if err != nil {
		t.Fatalf("BuildArgs: %v", err)
	}
	if !hasExactEnvArg(args, "HTTPS_PROXY") {
		t.Fatalf("container-reachable proxy was not forwarded: %v", args)
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

func TestDockerReachableRuntimeAPIURLRewritesLoopback(t *testing.T) {
	cases := map[string]string{
		"http://127.0.0.1:27892": "http://host.docker.internal:27892",
		"http://localhost:27892": "http://host.docker.internal:27892",
		"http://[::1]:27892":     "http://host.docker.internal:27892",
		"http://192.168.1.2:80":  "http://192.168.1.2:80",
		"https://api.example":    "https://api.example",
	}
	for input, want := range cases {
		if got := DockerReachableRuntimeAPIURL(input); got != want {
			t.Fatalf("DockerReachableRuntimeAPIURL(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestPlatformCompatibleNormalizesDockerArch(t *testing.T) {
	cases := []struct {
		image string
		host  string
		want  bool
	}{
		{"linux/amd64", "linux/x86_64", true},
		{"linux/arm64", "linux/aarch64", true},
		{"linux/amd64", "linux/aarch64", false},
		{"linux/arm64", "linux/amd64", false},
		{"linux/amd64", "windows/amd64", false},
	}
	for _, tc := range cases {
		if got := platformCompatible(tc.image, tc.host); got != tc.want {
			t.Fatalf("platformCompatible(%q, %q) = %v, want %v", tc.image, tc.host, got, tc.want)
		}
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

func hasExactEnvArg(args []string, key string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-e" && args[i+1] == key {
			return true
		}
	}
	return false
}

func argAfter(args []string, flag string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}
