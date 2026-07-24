// Package sandbox wraps agent CLI execution inside isolated Docker containers.
//
// Design:
//   - Only plain `docker run` is used (no Docker Desktop sandbox/microVM API).
//     This works on any OS with Docker installed: Linux, macOS, Windows.
//   - The agent working directory is bind-mounted into the container at
//     /workspace; the container process runs with that as its cwd.
//   - Agent CLI session/config directories are mounted from the agent's own
//     .multigent/runtime-home tree, never from host-global ~/.claude or ~/.codex.
//   - API keys are injected as environment variables.
//   - The container is removed after each run (--rm).
//   - All stdout/stderr is streamed to the caller via the provided io.Writer.
package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/multigent/multigent/internal/entity"
)

const (
	// WorkspaceMount is the path inside the container where the agent
	// working directory is mounted.
	WorkspaceMount = "/workspace"

	// DefaultMemoryMB is the container memory limit when none is specified.
	DefaultMemoryMB = 4096

	// Image registry prefix for multigent-provided sandbox images.
	imagePrefix = "ghcr.io/multigent/multigent"

	// ChinaImagePrefix is the official mainland China mirror for users who
	// cannot reliably pull from GHCR.
	ChinaImagePrefix = "crpi-fu3b7e7lggtmh7za.cn-hangzhou.personal.cr.aliyuncs.com/multigent"

	// LocalBaseImage is the image tag created by local builds. Prefer it when it
	// exists so development and self-hosted installs do not accidentally pull
	// from an unavailable registry package.
	LocalBaseImage = "multigent/runtime-base:latest"

	// BaseImage is the published runtime image used by default. Agent CLI binaries
	// are installed at runtime into a persistent toolchain cache, so CLI version
	// bumps do not require rebuilding this image.
	BaseImage = imagePrefix + "/runtime-base:latest"

	// ChinaBaseImage is the official mainland China mirror of BaseImage.
	ChinaBaseImage = ChinaImagePrefix + "/runtime-base:latest"

	EnvRuntimeImage  = "MULTIGENT_RUNTIME_IMAGE"
	EnvRuntimeRegion = "MULTIGENT_RUNTIME_REGION"

	// UserBin is where user-provided binaries are mounted inside the
	// container. If <root>/bin/ exists on the host, it is mounted here and
	// prepended to PATH so those binaries are directly accessible.
	UserBin = "/multigent/bin"

	// ContainerDefaultPATH mirrors the tool locations provided by the sandbox
	// images. Keep Go paths here because Docker -e PATH=... replaces the image
	// ENV PATH instead of expanding it.
	ContainerDefaultPATH = "/opt/multigent/mga/bin:/usr/local/go/bin:/root/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
)

var dockerImageExists = imageExists

// Default credential/session mounts are scoped to one agent directory. Do not
// mount host-global ~/.claude, ~/.codex, ~/.ssh, or ~/.config/gh by default:
// those would let one agent read another agent's sessions or credentials.

// BuildArgs constructs the full `docker run` argument list for running an
// agent inside a container. The returned slice is ready to pass to exec.Command.
//
// agentDir is the agent's working directory on the host (will be mounted at /workspace).
// model is used to select defaults when cfg fields are empty.
// innerArgs are the agent CLI arguments to run inside the container.
func BuildArgs(agentDir string, model entity.AgentModel, cfg *entity.DockerSandboxConfig, innerArgs []string) ([]string, error) {
	args := []string{"run", "--rm", "-i"}

	// ── Image ────────────────────────────────────────────────────────────────
	image := resolveImage(model, cfg)

	// ── Memory / CPU limits ──────────────────────────────────────────────────
	memMB := DefaultMemoryMB
	if cfg != nil && cfg.MemoryMB > 0 {
		memMB = cfg.MemoryMB
	}
	args = append(args, fmt.Sprintf("--memory=%dm", memMB))

	if cfg != nil && cfg.CPUs > 0 {
		args = append(args, fmt.Sprintf("--cpus=%.2f", cfg.CPUs))
	}

	// ── Codex sandbox bypass ─────────────────────────────────────────────────
	// Docker IS the sandbox, so Codex's internal sandbox (bwrap/landlock) is
	// redundant and causes read-only filesystem issues. Inject the bypass flag
	// into innerArgs so Codex runs commands directly without nested sandboxing.
	switch entity.NormaliseModel(model) {
	case entity.ModelCodex, entity.ModelQoder:
		innerArgs = injectCodexSandboxBypass(innerArgs)
	}

	// ── Network ──────────────────────────────────────────────────────────────
	networkMode := "bridge"
	if cfg != nil && cfg.NetworkMode != "" {
		networkMode = cfg.NetworkMode
	}
	args = append(args, "--network="+networkMode)
	if networkMode == "bridge" {
		args = append(args, "--add-host=host.docker.internal:host-gateway")
	}

	// ── Workspace mount ──────────────────────────────────────────────────────
	// Mount the agent directory at a stable Linux path inside the container.
	// This keeps Docker commands valid on Windows hosts while session/config
	// state remains persistent through the agent-scoped runtime-home mounts.
	absAgentDir, err := filepath.Abs(agentDir)
	if err != nil {
		return nil, fmt.Errorf("sandbox: resolve agent dir: %w", err)
	}
	args = append(args,
		"-v", absAgentDir+":"+WorkspaceMount,
		"-w", WorkspaceMount,
	)

	// ── User bin directory ────────────────────────────────────────────────────
	// If <root>/bin/ exists, mount it to /multigent/bin and add to PATH so
	// custom binaries are directly accessible inside the container.
	// agentDir = <root>/projects/<project>/agents/<agent> → root = ../../
	workspaceRoot := filepath.Join(
		filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(agentDir)))),
	)
	binHostDir := filepath.Join(
		workspaceRoot,
		"bin",
	)
	if isWorkspaceRoot(workspaceRoot) {
		if fi, err := os.Stat(binHostDir); err == nil && fi.IsDir() {
			args = append(args, "-v", binHostDir+":"+UserBin)
			args = append(args, "-e", "PATH="+UserBin+":"+ContainerDefaultPATH)
		}
	}

	// ── Agent-scoped credential/session mounts ───────────────────────────────
	mounts := resolveCredentialMounts(model, cfg, agentDir)
	for _, m := range mounts {
		expanded := expandTilde(m)
		hostPath := strings.SplitN(expanded, ":", 2)[0]
		if ensureRuntimeMountPath(hostPath) == nil {
			args = append(args, "-v", expanded)
		}
	}

	// ── Cursor Agent binary ──────────────────────────────────────────────────
	// Cursor Agent is a self-contained Node.js bundle (cursor-agent + node +
	// index.js all in the same directory). The entry script resolves its own
	// directory to locate sibling files, so we must mount the entire version
	// directory at a fixed container path and rewrite innerArgs to use the
	// full path so SCRIPT_DIR resolves correctly.
	if entity.NormaliseModel(model) == entity.ModelCursor {
		if agentBin, err := exec.LookPath("agent"); err == nil {
			if realBin, err := filepath.EvalSymlinks(agentBin); err == nil {
				realDir := filepath.Dir(realBin)
				const containerCursorDir = "/opt/cursor-agent"
				args = append(args, "-v", realDir+":"+containerCursorDir+":ro")
				entryName := filepath.Base(realBin)
				fullPath := containerCursorDir + "/" + entryName
				for i, a := range innerArgs {
					if a == "agent" {
						innerArgs[i] = fullPath
						break
					}
				}
			}
		}
	}

	// ── Extra volumes ────────────────────────────────────────────────────────
	if cfg != nil {
		for _, v := range cfg.ExtraVolumes {
			args = append(args, "-v", expandTilde(v))
		}
	}

	// ── Environment variables ────────────────────────────────────────────────
	// 1. Fixed sandbox env vars: always injected with explicit values because
	//    they must be set regardless of the host environment. These tell each
	//    agent CLI that it is running inside a sandbox and should not enforce
	//    extra permission prompts that require human interaction.
	for _, kv := range sandboxEnvVars(model) {
		args = append(args, "-e", kv)
	}

	// 2. API keys & provider overrides: use `-e KEY` so Docker copies from the host.
	//    Values never appear in the docker argv. For ANTHROPIC_BASE_URL,
	//    ANTHROPIC_AUTH_TOKEN, and ANTHROPIC_MODEL we forward whenever the host
	//    has the variable defined (even if empty), so Claude Code can target a
	//    custom gateway; other keys are forwarded only when non-empty.
	for _, envKey := range wellKnownEnvKeys(model) {
		if forwardHostEnvIntoDocker(envKey) {
			args = append(args, "-e", envKey)
		}
	}

	// 3. ExtraEnv supports both "KEY" (inherit) and "KEY=VALUE" (explicit).
	if cfg != nil {
		for _, kv := range cfg.ExtraEnv {
			args = append(args, "-e", kv)
		}
	}

	// ── Image + inner command ────────────────────────────────────────────────
	args = append(args, image)
	args = append(args, innerArgs...)

	return args, nil
}

// RunArgs is a convenience wrapper: returns ("docker", BuildArgs(...)).
// Callers can pass this directly to exec.Command.
func RunArgs(agentDir string, model entity.AgentModel, cfg *entity.DockerSandboxConfig, innerArgs []string) (string, []string, error) {
	a, err := BuildArgs(agentDir, model, cfg, innerArgs)
	if err != nil {
		return "", nil, err
	}
	return DockerExecutable(), a, nil
}

// DockerCommand returns an exec.Cmd configured with a PATH that works in
// non-interactive macOS/Windows sessions. Docker Desktop may need credential
// helpers next to the docker binary even when the image is public.
func DockerCommand(args ...string) *exec.Cmd {
	cmd := exec.Command(DockerExecutable(), args...)
	cmd.Env = DockerCommandEnv(os.Environ())
	return cmd
}

// DockerCommandContext is the context-aware form of DockerCommand.
func DockerCommandContext(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, DockerExecutable(), args...)
	cmd.Env = DockerCommandEnv(os.Environ())
	return cmd
}

// DockerCommandEnv prefixes common Docker Desktop binary directories to PATH.
func DockerCommandEnv(env []string) []string {
	prefixes := []string{
		"/Applications/Docker.app/Contents/Resources/bin",
		"/usr/local/bin",
		"/opt/homebrew/bin",
		filepath.Join(os.Getenv("ProgramFiles"), "Docker", "Docker", "resources", "bin"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Docker"),
	}
	current := ""
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(key, "PATH") {
			current = value
			break
		}
	}
	if current == "" {
		current = os.Getenv("PATH")
	}
	parts := append([]string{}, prefixes...)
	if current != "" {
		parts = append(parts, strings.Split(current, string(os.PathListSeparator))...)
	}
	next := strings.Join(dedupePath(parts), string(os.PathListSeparator))
	out := append([]string{}, env...)
	for i, entry := range out {
		key, _, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(key, "PATH") {
			out[i] = key + "=" + next
			return out
		}
	}
	return append(out, "PATH="+next)
}

// CheckDocker verifies that the docker CLI is available and the daemon is reachable.
// Returns a user-friendly error if not.
func CheckDocker() error {
	docker := DockerExecutable()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := DockerCommandContext(ctx, "info", "--format", "{{.ServerVersion}}")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("docker sandbox: Docker daemon did not respond within 3s — Docker may be starting, stuck, or unhealthy\n  Restart Docker Desktop / Docker Engine, then run: multigent sandbox prepare\n  Docker executable checked: %s", docker)
		}
		return fmt.Errorf("docker sandbox: cannot reach Docker daemon — is Docker running?\n  Install Docker: https://docs.docker.com/get-docker/\n  Start Docker Desktop on macOS/Windows, or start daemon: sudo systemctl start docker (Linux)\n  Docker executable checked: %s\n  Original error: %w", docker, err)
	}
	return nil
}

// DockerExecutable returns the best Docker CLI path for the current host.
// macOS services and SSH sessions often omit /usr/local/bin from PATH even
// when Docker Desktop is installed, so check common installation locations
// before falling back to PATH resolution.
func DockerExecutable() string {
	candidates := []string{}
	if configured := strings.TrimSpace(os.Getenv("MULTIGENT_DOCKER")); configured != "" {
		candidates = append(candidates, configured)
	}
	candidates = append(candidates,
		"docker",
		"/usr/local/bin/docker",
		"/opt/homebrew/bin/docker",
		"/Applications/Docker.app/Contents/Resources/bin/docker",
	)
	seen := map[string]bool{}
	for _, candidate := range candidates {
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		if strings.ContainsRune(candidate, os.PathSeparator) || filepath.IsAbs(candidate) {
			if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
				return candidate
			}
			continue
		}
		if path, err := exec.LookPath(candidate); err == nil {
			return path
		}
	}
	return "docker"
}

// PullImage pulls a Docker image if it is not already present locally.
// This is a best-effort call; errors are non-fatal.
func PullImage(image string) error {
	if imageExists(image) {
		return nil
	}
	cmd := DockerCommand("pull", image)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ImageAvailable reports whether a compatible local image is already present.
// It never pulls from the registry; callers can use this for fast readiness
// checks before deciding whether to run sandbox prepare.
func ImageAvailable(image string) bool {
	return imageExists(image)
}

// RuntimeContainerAvailable verifies that Docker can start a container from the
// runtime image. `docker info` can succeed while Docker Desktop/WSL is unable
// to transition containers out of Created state.
func RuntimeContainerAvailable(image string, timeout time.Duration) error {
	image = strings.TrimSpace(image)
	if image == "" {
		return fmt.Errorf("runtime image is empty")
	}
	if timeout <= 0 {
		timeout = 4 * time.Second
	}
	name := fmt.Sprintf("multigent-readiness-%d", time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := DockerCommandContext(ctx, "run", "--rm", "--pull=never", "--name", name, image, "/bin/sh", "-lc", "true")
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cleanupCancel()
			_ = DockerCommandContext(cleanupCtx, "rm", "-f", name).Run()
			return fmt.Errorf("Docker daemon is reachable but containers did not start within %s", timeout)
		}
		return fmt.Errorf("start runtime container: %w", err)
	}
	return nil
}

// ToolchainBinaryAvailable checks whether a binary is already present in the
// persistent Multigent toolchain volume for the given runtime image. It assumes
// the image is already local and uses a short timeout so readiness checks do not
// accidentally become slow runtime preparation.
func ToolchainBinaryAvailable(image, binary string, timeout time.Duration) (bool, error) {
	image = strings.TrimSpace(image)
	binary = strings.TrimSpace(binary)
	if image == "" || binary == "" {
		return false, nil
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	args := []string{
		"run", "--rm",
		"-v", "multigent-toolchains:/opt/multigent/toolchains",
		image,
		"/bin/sh", "-lc",
		"export PATH=/opt/multigent/toolchains/npm/bin:/opt/multigent/toolchains/mga/bin:$PATH; command -v " + shellQuote(binary) + " >/dev/null 2>&1",
	}
	cmd := DockerCommandContext(ctx, args...)
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, nil
	}
	return true, nil
}

// ImageForModel returns the default Docker image name for an agent model.
func ImageForModel(model entity.AgentModel) string {
	return resolveImage(model, nil)
}

// DefaultBaseImage returns the managed runtime image selected by environment.
// MULTIGENT_RUNTIME_IMAGE is an explicit override; MULTIGENT_RUNTIME_REGION=cn
// selects the official Alibaba Cloud mirror for mainland China installs.
func DefaultBaseImage() string {
	if image := strings.TrimSpace(os.Getenv(EnvRuntimeImage)); image != "" {
		return image
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvRuntimeRegion))) {
	case "cn", "china", "zh-cn", "mainland", "mainland-china":
		return ChinaBaseImage
	default:
		return BaseImage
	}
}

// EffectiveImage returns the Docker image after applying model defaults,
// docker-specific overrides, and compatibility normalization for older configs.
func EffectiveImage(model entity.AgentModel, cfg *entity.DockerSandboxConfig) string {
	return resolveImage(model, cfg)
}

// ── internal helpers ──────────────────────────────────────────────────────────

func resolveImage(model entity.AgentModel, cfg *entity.DockerSandboxConfig) string {
	if cfg != nil && cfg.Image != "" {
		return normalizeDefaultImage(cfg.Image)
	}
	model = entity.NormaliseModel(model)
	switch model {
	case entity.ModelClaudeCode, entity.ModelCodex, entity.ModelGemini, entity.ModelOpenCode, entity.ModelCursor, entity.ModelQoder:
		return normalizeDefaultImage(DefaultBaseImage())
	default:
		return DefaultBaseImage()
	}
}

func normalizeDefaultImage(image string) string {
	if isManagedRuntimeImage(image) {
		if dockerImageExists(LocalBaseImage) {
			return LocalBaseImage
		}
		return DefaultBaseImage()
	}
	return image
}

func isManagedRuntimeImage(image string) bool {
	if image == LocalBaseImage || image == BaseImage || image == ChinaBaseImage {
		return true
	}
	if !strings.HasPrefix(image, imagePrefix+"/sandbox-") && !strings.HasPrefix(image, ChinaImagePrefix+"/sandbox-") {
		return false
	}
	return strings.HasSuffix(image, ":latest") || !strings.Contains(filepath.Base(image), ":")
}

func imageExists(image string) bool {
	if strings.TrimSpace(image) == "" {
		return false
	}
	inspect, err := DockerCommand("image", "inspect", image, "--format", "{{.Os}}/{{.Architecture}}").Output()
	if err != nil {
		return false
	}
	imagePlatform := strings.TrimSpace(string(inspect))
	if imagePlatform == "" {
		return true
	}
	host, err := DockerCommand("info", "--format", "{{.OSType}}/{{.Architecture}}").Output()
	if err != nil {
		return true
	}
	return platformCompatible(imagePlatform, strings.TrimSpace(string(host)))
}

func platformCompatible(imagePlatform, hostPlatform string) bool {
	imageOS, imageArch := splitPlatform(imagePlatform)
	hostOS, hostArch := splitPlatform(hostPlatform)
	if imageOS != "" && hostOS != "" && imageOS != hostOS {
		return false
	}
	if imageArch != "" && hostArch != "" && imageArch != hostArch {
		return false
	}
	return true
}

func splitPlatform(platform string) (string, string) {
	parts := strings.Split(strings.TrimSpace(platform), "/")
	if len(parts) < 2 {
		return strings.TrimSpace(platform), ""
	}
	return strings.ToLower(strings.TrimSpace(parts[0])), normalizeDockerArch(parts[1])
}

func normalizeDockerArch(arch string) string {
	switch strings.ToLower(strings.TrimSpace(arch)) {
	case "x86_64":
		return "amd64"
	case "aarch64", "arm64/v8":
		return "arm64"
	default:
		return strings.ToLower(strings.TrimSpace(arch))
	}
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func isWorkspaceRoot(root string) bool {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." || root == string(filepath.Separator) || filepath.Dir(root) == root {
		return false
	}
	if fi, err := os.Stat(filepath.Join(root, ".multigent")); err == nil && fi.IsDir() {
		return true
	}
	return false
}

func dedupePath(parts []string) []string {
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || seen[part] {
			continue
		}
		seen[part] = true
		out = append(out, part)
	}
	return out
}

// ResolveCredentialMounts is the exported form for use by CLI commands.
func ResolveCredentialMounts(model entity.AgentModel, cfg *entity.DockerSandboxConfig) []string {
	return resolveCredentialMounts(model, cfg, "")
}

// WellKnownEnvKeys is the exported form for use by CLI commands.
func WellKnownEnvKeys(model entity.AgentModel) []string {
	return wellKnownEnvKeys(model)
}

func resolveCredentialMounts(model entity.AgentModel, cfg *entity.DockerSandboxConfig, agentDir string) []string {
	if cfg != nil && cfg.NoAutoCredentials {
		return cfg.CredentialMounts
	}
	model = entity.NormaliseModel(model)
	defaults := defaultCredentialMountsForAgent(model, agentDir)
	if cfg == nil || len(cfg.CredentialMounts) == 0 {
		return defaults
	}
	// Merge: start with defaults, append user-specified extras.
	seen := make(map[string]bool)
	var merged []string
	for _, m := range defaults {
		merged = append(merged, m)
		seen[mountKey(m)] = true
	}
	for _, m := range cfg.CredentialMounts {
		if !seen[mountKey(m)] {
			merged = append(merged, m)
		}
	}
	return merged
}

func defaultCredentialMountsForAgent(model entity.AgentModel, agentDir string) []string {
	if agentDir == "" {
		return nil
	}
	base := filepath.Join(agentDir, ".multigent", "runtime-home", string(entity.NormaliseModel(model)))
	switch entity.NormaliseModel(model) {
	case entity.ModelClaudeCode:
		return []string{
			filepath.Join(base, ".claude.json") + ":/root/.claude.json",
			filepath.Join(base, ".claude") + ":/root/.claude",
		}
	case entity.ModelCodex, entity.ModelQoder:
		return []string{filepath.Join(base, ".codex") + ":/root/.codex"}
	case entity.ModelGemini:
		return []string{filepath.Join(base, ".gemini") + ":/root/.gemini"}
	case entity.ModelOpenCode:
		return []string{filepath.Join(base, ".config", "opencode") + ":/root/.config/opencode"}
	case entity.ModelCursor:
		return []string{
			filepath.Join(base, ".cursor") + ":/root/.cursor",
			filepath.Join(base, ".config", "cursor") + ":/root/.config/cursor",
			filepath.Join(base, ".local", "share", "cursor-agent") + ":/root/.local/share/cursor-agent",
		}
	default:
		return nil
	}
}

func ensureRuntimeMountPath(hostPath string) error {
	if strings.TrimSpace(hostPath) == "" {
		return fmt.Errorf("empty mount path")
	}
	if strings.HasSuffix(hostPath, ".json") {
		if err := os.MkdirAll(filepath.Dir(hostPath), 0o700); err != nil {
			return err
		}
		if _, err := os.Stat(hostPath); os.IsNotExist(err) {
			return os.WriteFile(hostPath, []byte("{}\n"), 0o600)
		}
		return nil
	}
	return os.MkdirAll(hostPath, 0o700)
}

// mountKey extracts the container path (the key for deduplication).
func mountKey(mount string) string {
	parts := strings.SplitN(mount, ":", 3)
	if len(parts) >= 2 {
		return parts[1]
	}
	return mount
}

// expandTilde replaces a leading ~ with the user's home directory.
func expandTilde(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return home + path[1:]
}

// sandboxEnvVars returns environment variables that MUST be explicitly set
// (as KEY=VALUE) inside the sandbox container for each agent model to function
// correctly as root without interactive permission prompts.
//
// These are NOT inherited from the host — they are fixed values that tell the
// agent CLI it is already running in an isolated sandbox environment.
func sandboxEnvVars(model entity.AgentModel) []string {
	switch entity.NormaliseModel(model) {
	case entity.ModelClaudeCode:
		// IS_SANDBOX=1 is required to use --dangerously-skip-permissions as
		// root. Without it, Claude Code refuses to start with:
		//   "--dangerously-skip-permissions cannot be used with root/sudo"
		return []string{"IS_SANDBOX=1"}

	case entity.ModelCodex, entity.ModelQoder:
		// Codex CLI mandates its own sandboxing mechanism. Inside a Docker
		// container we are already sandboxed, so we disable the inner sandbox
		// requirement. Without this it fails with:
		//   "Sandbox was mandated, but no sandbox is available!"
		return []string{"CODEX_UNSAFE_ALLOW_NO_SANDBOX=1"}
	}
	return nil
}

// wellKnownEnvKeys returns the environment variable names an agent model needs
// to authenticate. These are forwarded from the host into the container using
// `-e KEY` (no value in the argument — Docker inherits from host env), so the
// token value never appears in the docker run command line or process table.
// anthropicProviderEnvKeys are Claude / Anthropic-related vars that should be
// passed into Docker whenever set on the host (including empty), so users can
// switch API base URL, auth token, or model without changing multigent config.
var anthropicProviderEnvKeys = map[string]struct{}{
	"ANTHROPIC_BASE_URL":   {},
	"ANTHROPIC_AUTH_TOKEN": {},
	"ANTHROPIC_MODEL":      {},
}

func forwardHostEnvIntoDocker(envKey string) bool {
	if _, isProvider := anthropicProviderEnvKeys[envKey]; isProvider {
		_, set := os.LookupEnv(envKey)
		return set
	}
	return os.Getenv(envKey) != ""
}

func wellKnownEnvKeys(model entity.AgentModel) []string {
	// Keys common to all models.
	common := []string{
		"HTTPS_PROXY", "HTTP_PROXY", "NO_PROXY", // honour proxy settings
		"NPM_CONFIG_REGISTRY", // allow regional npm mirrors for agent CLI installs
	}
	var modelKeys []string
	switch entity.NormaliseModel(model) {
	case entity.ModelClaudeCode:
		modelKeys = []string{
			"ANTHROPIC_API_KEY",
			"ANTHROPIC_AUTH_TOKEN", // alternative auth used by some proxies
			"ANTHROPIC_BASE_URL",   // allow pointing at a proxy / local model
			"ANTHROPIC_MODEL",      // override default model
			"CLAUDE_CODE_USE_BEDROCK",
			"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_REGION", // Bedrock auth
		}
	case entity.ModelCodex, entity.ModelQoder:
		modelKeys = []string{
			"OPENAI_API_KEY",
			"OPENAI_BASE_URL",
			"OPENAI_ORG_ID",
		}
	case entity.ModelGemini:
		modelKeys = []string{
			"GEMINI_API_KEY",
			"GOOGLE_API_KEY",
			"GOOGLE_APPLICATION_CREDENTIALS",
			"GOOGLE_CLOUD_PROJECT",
		}
	case entity.ModelOpenCode:
		// OpenCode supports multiple providers; pass all of them.
		modelKeys = []string{
			"ANTHROPIC_API_KEY", "ANTHROPIC_BASE_URL",
			"OPENAI_API_KEY", "OPENAI_BASE_URL",
			"GEMINI_API_KEY", "GOOGLE_API_KEY",
			"GROQ_API_KEY",
			"XAI_API_KEY",
		}
	case entity.ModelCursor:
		modelKeys = []string{
			"CURSOR_API_KEY",
		}
	case entity.ModelIFlow:
		modelKeys = []string{
			"IFLOW_API_KEY",
		}
	}
	return append(common, modelKeys...)
}

// injectCodexSandboxBypass inserts --dangerously-bypass-approvals-and-sandbox
// into a Codex CLI argument list. The flag disables Codex's internal sandbox
// (bwrap/landlock), which is redundant when already running inside Docker and
// otherwise causes read-only filesystem issues.
//
// The flag is inserted before the trailing "-" (stdin marker) if present.
func injectCodexSandboxBypass(args []string) []string {
	const flag = "--dangerously-bypass-approvals-and-sandbox"
	n := len(args)
	if n > 0 && args[n-1] == "-" {
		out := make([]string, 0, n+1)
		out = append(out, args[:n-1]...)
		out = append(out, flag, "-")
		return out
	}
	return append(args, flag)
}
