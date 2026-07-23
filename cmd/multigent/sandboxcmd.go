package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/multigent/multigent/internal/agentcli"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/runtimecli"
	"github.com/multigent/multigent/internal/sandbox"
	"github.com/spf13/cobra"
)

func newSandboxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sandbox",
		Short: "Inspect and manage agent sandbox configuration",
	}
	cmd.AddCommand(
		newSandboxShowCmd(),
		newSandboxPrepareCmd(),
		newSandboxTestCmd(),
	)
	return cmd
}

// ── sandbox show ──────────────────────────────────────────────────────────────

func newSandboxShowCmd() *cobra.Command {
	var (
		project   string
		agentName string
	)

	cmd := &cobra.Command{
		Use:     "show",
		Short:   "Show sandbox config and the generated docker run command for an agent",
		Example: `  multigent sandbox show --project web-app --agent dev`,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := resolveRoot()
			if err != nil {
				return err
			}
			if project == "" || agentName == "" {
				return fmt.Errorf("--project and --agent are required")
			}

			s := mustStore(root)
			meta, err := s.AgentMeta(project, agentName)
			if err != nil {
				return err
			}
			agentDir := s.AgentDir(project, agentName)

			fmt.Printf("Agent   : %s/%s\n", project, agentName)
			fmt.Printf("Model   : %s\n", meta.Model)

			if meta.Sandbox == nil || meta.Sandbox.Provider == entity.SandboxNone {
				fmt.Printf("Sandbox : none (agent runs directly on host)\n\n")
				fmt.Printf("To enable:\n")
				fmt.Printf("  multigent hire --project %s --team %s --model %s --name %s --sandbox docker --force\n",
					project, meta.Team, meta.Model, agentName)
				return nil
			}

			fmt.Printf("Sandbox : %s\n", meta.Sandbox.Provider)

			if meta.Sandbox.Provider != entity.SandboxDocker {
				return nil
			}

			dockerCfg := meta.Sandbox.Docker

			image := sandbox.EffectiveImage(meta.Model, dockerCfg)
			network := "bridge"
			if dockerCfg != nil && dockerCfg.NetworkMode != "" {
				network = dockerCfg.NetworkMode
			}
			memMB := sandbox.DefaultMemoryMB
			if dockerCfg != nil && dockerCfg.MemoryMB > 0 {
				memMB = dockerCfg.MemoryMB
			}

			fmt.Printf("Image   : %s\n", image)
			fmt.Printf("Network : %s\n", network)
			fmt.Printf("Memory  : %d MiB\n", memMB)

			fmt.Printf("\nRepo mount : none (agents only receive their own workspace directory)\n")

			// Credential mounts
			mounts := sandbox.ResolveCredentialMounts(meta.Model, dockerCfg)
			if len(mounts) > 0 {
				fmt.Printf("\nCredential mounts (read-only, skipped if path not found on host):\n")
				for _, m := range mounts {
					fmt.Printf("  %s\n", sbxExpandHome(m))
				}
			}

			// API key env vars
			envKeys := sandbox.WellKnownEnvKeys(meta.Model)
			var apiKeys []string
			for _, k := range envKeys {
				if !sbxIsProxyKey(k) {
					apiKeys = append(apiKeys, k)
				}
			}
			if len(apiKeys) > 0 {
				fmt.Printf("\nAPI keys forwarded from host (value hidden, use -e KEY form):\n")
				for _, k := range apiKeys {
					set := "not set on host"
					if os.Getenv(k) != "" {
						set = "✓ set"
					}
					fmt.Printf("  %-40s %s\n", k, set)
				}
			}

			fmt.Printf("\nWorkspace isolation:\n")
			fmt.Printf("  workspace root is not mounted; agents coordinate through the runtime API\n")

			// Optional agent runtime CLI override for local development.
			fmt.Printf("\nAgent runtime CLI mount (optional development override):\n")
			mgaMount := runtimecli.ResolveAvailableBinaryMount(root)
			if mgaMount == "" {
				fmt.Printf("  release runs sync `mga` into the Docker toolchain volume; image-bundled `mga` is a fallback (set %s to override with a Linux ELF binary)\n", runtimecli.HostBinaryEnv)
			} else {
				fmt.Printf("  -v %s\n", mgaMount)
			}

			// Preview the actual docker run command (include all auto mounts)
			previewCfg := sbxCloneDockerCfg(dockerCfg)
			if mgaMount != "" {
				previewCfg.ExtraVolumes = append(previewCfg.ExtraVolumes, mgaMount)
			}
			innerArgs := agentInnerArgs(meta.Model)
			dockerArgs, err := sandbox.BuildArgs(agentDir, meta.Model, previewCfg, innerArgs)
			if err != nil {
				return fmt.Errorf("build docker args preview: %w", err)
			}
			fmt.Printf("\nGenerated docker run command (example):\n")
			fmt.Printf("  docker %s\n", sbxFormatArgs(dockerArgs))
			return nil
		},
	}

	cmd.Flags().StringVar(&project, "project", "", "project name")
	cmd.Flags().StringVar(&agentName, "agent", "", "agent name")
	return cmd
}

// ── sandbox prepare ───────────────────────────────────────────────────────────

func newSandboxPrepareCmd() *cobra.Command {
	var (
		image      string
		toolchain  []string
		skipPull   bool
		skipCLIs   bool
		memoryMB   int
		network    string
		timeoutSec int
	)

	cmd := &cobra.Command{
		Use:   "prepare",
		Short: "Pre-pull the runtime image and warm common agent CLI toolchains",
		Example: `  multigent sandbox prepare
  multigent sandbox prepare --toolchain codex
  multigent sandbox prepare --toolchain codex --toolchain claudecode`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if image == "" {
				image = sandbox.BaseImage
			}
			if network == "" {
				network = "bridge"
			}
			if memoryMB <= 0 {
				memoryMB = sandbox.DefaultMemoryMB
			}
			if timeoutSec <= 0 {
				timeoutSec = 600
			}

			if err := sandbox.CheckDocker(); err != nil {
				return err
			}
			fmt.Printf("Docker: %s\n", sandbox.DockerExecutable())
			fmt.Printf("Runtime image: %s\n", image)

			if !skipPull {
				fmt.Println("\nPulling runtime image. This can take a few minutes on the first install...")
				if err := sandbox.PullImage(image); err != nil {
					return fmt.Errorf("pull runtime image: %w", err)
				}
			}

			if skipCLIs {
				fmt.Println("\nSkipping agent CLI toolchain warmup.")
				fmt.Println("✓ Sandbox runtime image is ready.")
				return nil
			}

			if len(toolchain) == 0 {
				toolchain = []string{"codex", "claudecode"}
			}
			for _, name := range toolchain {
				model, ok := prepareToolchainModel(name)
				if !ok {
					return fmt.Errorf("unsupported toolchain %q (supported: codex, claudecode, gemini)", name)
				}
				cfg := agentcli.DefaultForModel(model)
				if cfg == nil {
					return fmt.Errorf("no installer configured for %q", name)
				}
				if err := runToolchainWarmup(image, network, memoryMB, timeoutSec, model, cfg); err != nil {
					return err
				}
			}

			fmt.Println("\n✓ Sandbox is prepared. First agent chat should start much faster.")
			return nil
		},
	}

	cmd.Flags().StringVar(&image, "image", sandbox.BaseImage, "runtime image to pull and use")
	cmd.Flags().StringSliceVar(&toolchain, "toolchain", nil, "agent CLI toolchain to warm (repeatable: codex, claudecode, gemini)")
	cmd.Flags().BoolVar(&skipPull, "skip-pull", false, "skip pulling the runtime image")
	cmd.Flags().BoolVar(&skipCLIs, "skip-clis", false, "skip warming agent CLI toolchains")
	cmd.Flags().IntVar(&memoryMB, "memory", sandbox.DefaultMemoryMB, "container memory limit in MiB")
	cmd.Flags().StringVar(&network, "network", "bridge", "Docker network mode")
	cmd.Flags().IntVar(&timeoutSec, "install-timeout", 600, "agent CLI install timeout in seconds")
	return cmd
}

func prepareToolchainModel(name string) (entity.AgentModel, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "codex", "openai":
		return entity.ModelCodex, true
	case "claude", "claudecode", "claude-code":
		return entity.ModelClaudeCode, true
	case "gemini":
		return entity.ModelGemini, true
	default:
		return "", false
	}
}

func runToolchainWarmup(image, network string, memoryMB, timeoutSec int, model entity.AgentModel, cliCfg *entity.AgentCLIConfig) error {
	dir, err := os.MkdirTemp("", "multigent-sandbox-prepare-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	script := agentcli.BootstrapScript(cliCfg)
	if script == "" {
		return nil
	}
	script += "\n" + fmt.Sprintf("echo %s >&2", sbxShellQuote("multigent: agent CLI ready: "+cliCfg.Binary))
	dockerCfg := &entity.DockerSandboxConfig{
		Image:       image,
		NetworkMode: network,
		MemoryMB:    memoryMB,
	}
	_, dockerArgs, err := sandbox.RunArgs(dir, model, dockerCfg, []string{"/bin/sh", "-lc", script})
	if err != nil {
		return err
	}
	fmt.Printf("\nWarming %s toolchain (%s)...\n", cliCfg.Vendor, cliCfg.Package)
	cmd := exec.Command(sandbox.DockerExecutable(), dockerArgs...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("MULTIGENT_AGENT_CLI_INSTALL_TIMEOUT=%d", timeoutSec))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("warm %s toolchain: %w", cliCfg.Vendor, err)
	}
	return nil
}

// ── sandbox test ──────────────────────────────────────────────────────────────

func newSandboxTestCmd() *cobra.Command {
	var (
		project   string
		agentName string
	)

	cmd := &cobra.Command{
		Use:     "test",
		Short:   "Verify the sandbox works by running 'echo ok' inside the container",
		Example: `  multigent sandbox test --project web-app --agent dev`,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := resolveRoot()
			if err != nil {
				return err
			}
			if project == "" || agentName == "" {
				return fmt.Errorf("--project and --agent are required")
			}

			s := mustStore(root)
			meta, err := s.AgentMeta(project, agentName)
			if err != nil {
				return err
			}

			if meta.Sandbox == nil || meta.Sandbox.Provider == entity.SandboxNone {
				fmt.Printf("Agent %s/%s has no sandbox configured.\n", project, agentName)
				return nil
			}

			if err := sandbox.CheckDocker(); err != nil {
				return err
			}

			agentDir := s.AgentDir(project, agentName)
			dockerCfg := meta.Sandbox.Docker

			image := sandbox.EffectiveImage(meta.Model, dockerCfg)

			fmt.Printf("Testing sandbox for %s/%s ...\n", project, agentName)
			fmt.Printf("Image: %s\n\n", image)

			// Pull image (shows progress; no-op if already cached).
			fmt.Println("Pulling image (skipped if already cached)...")
			if err := sandbox.PullImage(image); err != nil {
				fmt.Printf("warning: pull failed (image may not exist yet): %v\n", err)
			}

			// Run a quick echo test — verifies workspace mount works.
			testArgs := []string{"echo", "multigent sandbox test: OK"}
			_, finalArgs, err := sandbox.RunArgs(agentDir, meta.Model, dockerCfg, testArgs)
			if err != nil {
				return err
			}

			testCmd := exec.Command(sandbox.DockerExecutable(), finalArgs...)
			testCmd.Stdout = os.Stdout
			testCmd.Stderr = os.Stderr
			if err := testCmd.Run(); err != nil {
				return fmt.Errorf("sandbox test failed: %w", err)
			}

			fmt.Printf("\n✓ Sandbox test passed for %s/%s\n", project, agentName)
			return nil
		},
	}

	cmd.Flags().StringVar(&project, "project", "", "project name")
	cmd.Flags().StringVar(&agentName, "agent", "", "agent name")
	return cmd
}

// ── helpers ───────────────────────────────────────────────────────────────────

// agentInnerArgs returns a representative inner command for display purposes.
func agentInnerArgs(model entity.AgentModel) []string {
	switch entity.NormaliseModel(model) {
	case entity.ModelClaudeCode:
		return []string{"claude", "--no-interactive", "--output-format", "stream-json",
			"--print-file", "/workspace/.prompt-example.txt"}
	case entity.ModelCodex, entity.ModelQoder:
		return []string{"codex", "-q", "--full-auto", "--input-file", "/workspace/.prompt-example.txt"}
	case entity.ModelGemini:
		return []string{"gemini", "--yolo", "--prompt-file", "/workspace/.prompt-example.txt"}
	case entity.ModelCursor:
		// Cursor CLI binary is `agent` (installed via curl https://cursor.com/install)
		return []string{"agent", "-p", "--force", "--output-format", "stream-json",
			"--print-file", "/workspace/.prompt-example.txt"}
	case entity.ModelOpenCode:
		return []string{"opencode", "run", "--file", "/workspace/.prompt-example.txt"}
	default:
		return []string{"sh", "-c", "cat /workspace/.prompt-example.txt"}
	}
}

// sbxFormatArgs pretty-prints docker args with line continuations.
func sbxFormatArgs(args []string) string {
	var sb strings.Builder
	i := 0
	lineLen := 0
	for i < len(args) {
		a := args[i]
		// Flags that take a value argument: print together on one line.
		if (a == "-v" || a == "-e" || a == "--memory" || a == "--network" ||
			a == "--cpus" || a == "-w") && i+1 < len(args) {
			chunk := a + " " + args[i+1]
			if lineLen > 0 {
				sb.WriteString(" \\\n    ")
				lineLen = 4
			}
			sb.WriteString(chunk)
			lineLen += len(chunk)
			i += 2
		} else {
			if lineLen > 60 {
				sb.WriteString(" \\\n    ")
				lineLen = 4
			} else if lineLen > 0 {
				sb.WriteString(" ")
				lineLen++
			}
			sb.WriteString(a)
			lineLen += len(a)
			i++
		}
	}
	return sb.String()
}

func sbxExpandHome(path string) string {
	home, _ := os.UserHomeDir()
	if strings.HasPrefix(path, "~/") {
		return home + path[1:]
	}
	return path
}

func sbxIsProxyKey(k string) bool {
	k = strings.ToUpper(k)
	return k == "HTTPS_PROXY" || k == "HTTP_PROXY" || k == "NO_PROXY"
}

func sbxShellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// sbxCloneDockerCfg returns a shallow copy so we can mutate ExtraVolumes.
func sbxCloneDockerCfg(cfg *entity.DockerSandboxConfig) *entity.DockerSandboxConfig {
	if cfg == nil {
		return &entity.DockerSandboxConfig{}
	}
	cp := *cfg
	cp.ExtraVolumes = append([]string(nil), cfg.ExtraVolumes...)
	cp.ExtraEnv = append([]string(nil), cfg.ExtraEnv...)
	return &cp
}
