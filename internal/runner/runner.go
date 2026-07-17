// Package runner executes agent CLI processes for a given task,
// handles sentinel detection (confirmation requests), captures session IDs,
// and writes run logs.
//
// When the agent's SandboxConfig specifies provider=docker, execution is
// delegated to the sandbox package which wraps the agent command in
// `docker run`. Otherwise the agent CLI is invoked directly on the host.
package runner

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/multigent/multigent/internal/agentcli"
	"github.com/multigent/multigent/internal/daemon"
	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/runenv"
	"github.com/multigent/multigent/internal/runtimeauth"
	"github.com/multigent/multigent/internal/runtimecli"
	"github.com/multigent/multigent/internal/store"
	"github.com/multigent/multigent/internal/taskstore"
	"github.com/multigent/multigent/internal/telemetry"
)

const (
	// ConfirmSentinel is the magic prefix agents write to stdout to request
	// human confirmation. Everything after the colon is used as the summary.
	ConfirmSentinel = "MULTIGENT_AWAIT_CONFIRM:"

	// SessionSentinel lets agents explicitly report a new session ID.
	SessionSentinel = "MULTIGENT_SESSION_ID:"

	runtimeConnectionsFileEnv = "MULTIGENT_CONNECTIONS_FILE"
	runtimeToolsFileEnv       = "MULTIGENT_TOOLS_FILE"
	runtimeToolDirEnv         = "MULTIGENT_TOOL_RUNTIME_DIR"
	runtimeToolBinDirEnv      = "MULTIGENT_TOOL_BIN_DIR"
	maxRuntimeConnectionsFile = 1 << 20
)

// systemMetaFooter is appended to every task prompt so agents know how to
// call back into the Multigent Server through the agent runtime CLI.
const systemMetaFooter = `

---
## System Metadata (do not modify)

Task ID : %s
Agent   : %s/%s

When complete successfully, run:
  mga task done --id %s --status success

If human confirmation needed, run:
  mga task confirm-request --id %s --summary "one-line explanation"
  (then exit 0)

If unable to complete, run:
  mga task done --id %s --status failed --error "reason"
`

// Runner executes tasks for agents using their configured CLI.
type Runner struct {
	root       string
	ts         taskstore.Store
	agentStore store.Store
}

// New creates a Runner. root is the workspace root.
func New(root string, ts taskstore.Store, as store.Store) *Runner {
	return &Runner{root: root, ts: ts, agentStore: as}
}

// RunResult holds the outcome of a single task execution.
type RunResult struct {
	Status    entity.TaskStatus
	SessionID string
	LogPath   string
	Summary   string // set when Status == TaskStatusAwaitingConfirmation
	ErrorMsg  string // set when Status == TaskStatusDoneFailed
}

// ExecPrompt runs a raw prompt against an agent directly, bypassing the task
// queue entirely. It is intended for quick interactive testing:
//
//	multigent exec --project p --agent a --prompt "hello"
//
// Unlike RunTask, it does NOT append the system meta footer, does NOT create
// or update any task record, and streams output directly to stdout in real
// time (as well as writing a log file).
//
// sessionID may be "" to start a fresh conversation, or a previous session ID
// to resume. The returned RunResult contains the detected session ID (if any)
// and the log path.
func (r *Runner) ExecPrompt(project, agentName, prompt, sessionID string) (*RunResult, error) {
	meta, err := r.agentStore.AgentMeta(project, agentName)
	if err != nil {
		return nil, fmt.Errorf("load agent meta: %w", err)
	}

	agentDir := filepath.Join(r.root, "projects", project, "agents", agentName)

	// HTTP agent: bypass CLI subprocess.
	if entity.NormaliseModel(meta.Model) == entity.ModelHTTPAgent {
		return r.execPromptHTTP(agentDir, meta, prompt)
	}

	// Write prompt to a temp file.
	promptFile, err := writeTempPrompt(agentDir, prompt)
	if err != nil {
		return nil, fmt.Errorf("write prompt file: %w", err)
	}
	defer os.Remove(promptFile)

	model := entity.NormaliseModel(meta.Model)
	agentEnv := resolveProviderEnv(r.root, meta)
	runtimeEnv := r.resolveRuntimeControlEnv(project, agentName, "exec-"+time.Now().UTC().Format("20060102-150405"))
	if cleanup := r.materializeRuntimeFiles(agentDir, runtimeEnv); cleanup != nil {
		defer cleanup()
	}
	effectiveEnv := mergeEnv(os.Environ(), agentEnv)
	effectiveEnv = mergeEnv(effectiveEnv, runtimeEnv)
	apiModel, apiBaseURL := resolveAPIModelFromEnv(model, effectiveEnv)
	invoker := InvokerFor(model, meta.RunCommand, meta.AddDirs)
	innerArgs := invoker.Args(promptFile, sessionID)

	var (
		executable string
		args       []string
		execDir    string
	)

	if meta.Sandbox != nil && meta.Sandbox.Provider != entity.SandboxNone {
		provider, ok := runenv.ProviderFor(meta.Sandbox.Provider)
		if !ok {
			return nil, fmt.Errorf("runtime provider %q is not implemented", meta.Sandbox.Provider)
		}
		if err := provider.Available(); err != nil {
			return nil, err
		}
		runtimeCfg := cloneRuntimeCfg(meta.Sandbox)
		agentCLI := agentcli.Effective(model, runtimeCfg.AgentCLI)
		injectProviderEnvIntoRuntime(runtimeCfg, agentEnv)
		injectRuntimeControlEnvIntoRuntime(runtimeCfg, runtimeEnv)
		mounts := append([]entity.RuntimeMount(nil), runtimeCfg.Mounts...)
		for _, addDir := range meta.AddDirs {
			mounts = runenv.AddPathMount(mounts, addDir, "repo", runenv.MountModeReadWrite)
		}
		if wsMount := r.root + ":" + r.root; r.root != "" {
			runtimeCfg.Docker.ExtraVolumes = append(runtimeCfg.Docker.ExtraVolumes, wsMount)
		}
		if binMount := runtimecli.ResolveHostBinaryMount(); binMount != "" {
			runtimeCfg.Docker.ExtraVolumes = append(runtimeCfg.Docker.ExtraVolumes, binMount)
		}
		containerPromptFile := agentDir + "/" + filepath.Base(promptFile)
		remappedInner := remapPromptFile(innerArgs, promptFile, containerPromptFile)
		var err error
		executable, args, err = provider.Command(runenv.ProcessSpec{
			WorkspaceRoot: r.root,
			Project:       project,
			Agent:         agentName,
			AgentDir:      agentDir,
			Model:         model,
			Command:       remappedInner,
			Env:           agentEnv,
			Runtime:       runtimeCfg,
			AgentCLI:      agentCLI,
			Mounts:        mounts,
			Limits:        runtimeCfg.Resources,
		})
		if err != nil {
			return nil, fmt.Errorf("runtime %s: build command: %w", meta.Sandbox.Provider, err)
		}
	} else {
		executable = innerArgs[0]
		args = innerArgs[1:]
		execDir = agentDir
	}

	// Prepare log file.
	logDir, err := r.ts.RunLogDir(project, agentName)
	if err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	logName := fmt.Sprintf("%s-exec.log", time.Now().UTC().Format("20060102-150405"))
	logPath := filepath.Join(logDir, logName)
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("create log file: %w", err)
	}
	defer logFile.Close()

	sandboxLabel := "host"
	if meta.Sandbox != nil && meta.Sandbox.Provider != entity.SandboxNone {
		sandboxLabel = string(meta.Sandbox.Provider)
	}
	fmt.Fprintf(logFile, "=== multigent exec: %s/%s sandbox=%s ===\n", project, agentName, sandboxLabel)
	fmt.Fprintf(logFile, "Command: %s\n", telemetry.FormatExecCommand(executable, args))
	fmt.Fprintf(logFile, "Started: %s\n\n", time.Now().UTC().Format(time.RFC3339))

	// Stream output to stdout AND the log file simultaneously.
	cmd := exec.Command(executable, args...)
	if execDir != "" {
		cmd.Dir = execDir
	}
	cmd.Env = effectiveEnv

	// When the invoker reads the prompt from stdin, open the prompt file and
	// pipe it through. For Docker this works because `-i` is always present in
	// the run args (see sandbox.BuildArgs), so stdin is forwarded into the
	// container transparently.
	if invoker.UseStdinPrompt() {
		pf, err := os.Open(promptFile)
		if err != nil {
			return nil, fmt.Errorf("open prompt file for stdin: %w", err)
		}
		defer pf.Close()
		cmd.Stdin = pf
	}

	var outBuf bytes.Buffer
	multiOut := io.MultiWriter(&outBuf, logFile, os.Stdout)
	cmd.Stdout = multiOut
	cmd.Stderr = multiOut

	runStarted := time.Now()
	runErr := cmd.Run()
	runFinished := time.Now()

	fmt.Fprintf(logFile, "\n=== exit code: %v  finished: %s ===\n",
		cmd.ProcessState.ExitCode(), time.Now().UTC().Format(time.RFC3339))

	output := outBuf.String()
	result := &RunResult{LogPath: logPath}

	if sid := invoker.ParseSessionID(output); sid != "" {
		result.SessionID = sid
	} else if sid := parseLineSentinel(output, SessionSentinel); sid != "" {
		result.SessionID = sid
	}

	ec := exitCodeOrZero(cmd)
	if runErr != nil {
		if sessionID != "" && isCodexResumeMissingRolloutError(output) {
			fmt.Fprintf(logFile, "\n=== codex rollout missing for saved session — clearing heartbeat session + retrying fresh ===\n")
			r.recordAgentRun(telemetry.KindExec, project, agentName, "", "", string(model), sandboxLabel,
				apiModel, apiBaseURL,
				runStarted, runFinished, entity.TaskStatusDoneFailed, &ec, result.SessionID,
				"codex rollout missing for saved session, retrying fresh",
				logPath, telemetry.FormatExecCommand(executable, args), prompt, outBuf.Bytes())
			r.clearHeartbeatSession(project, agentName)
			return r.ExecPrompt(project, agentName, prompt, "")
		}
		if sessionID != "" && isThinkingSignatureError(output) {
			fmt.Fprintf(logFile, "\n=== thinking block signature invalid — clearing heartbeat session + retrying fresh ===\n")
			r.recordAgentRun(telemetry.KindExec, project, agentName, "", "", string(model), sandboxLabel,
				apiModel, apiBaseURL,
				runStarted, runFinished, entity.TaskStatusDoneFailed, &ec, result.SessionID,
				"thinking block signature invalid, retrying fresh",
				logPath, telemetry.FormatExecCommand(executable, args), prompt, outBuf.Bytes())
			r.clearHeartbeatSession(project, agentName)
			return r.ExecPrompt(project, agentName, prompt, "")
		}
		if discardSessionIDOnFailure(model) {
			result.SessionID = ""
		}
		result.Status = entity.TaskStatusDoneFailed
		result.ErrorMsg = buildErrorMsg(runErr, outBuf.Bytes())
	} else {
		result.Status = entity.TaskStatusDoneSuccess
	}
	r.recordAgentRun(telemetry.KindExec, project, agentName, "", "", string(model), sandboxLabel,
		apiModel, apiBaseURL,
		runStarted, runFinished, result.Status, &ec, result.SessionID, result.ErrorMsg,
		logPath, telemetry.FormatExecCommand(executable, args), prompt, outBuf.Bytes())
	return result, nil
}

// RunTask executes a single task in the context of the given agent.
// It handles:
//   - building the full prompt (task prompt + system footer)
//   - invoking the agent CLI (with optional session resume)
//   - detecting MULTIGENT_AWAIT_CONFIRM sentinel
//   - capturing session ID from output
//   - writing the run log
//
// It does NOT update task state in tasks.yaml — callers are responsible
// for calling ts.UpdateTask / ts.ArchiveTask based on the returned RunResult.
func (r *Runner) RunTask(project, agentName string, task *entity.Task, sessionID string) (*RunResult, error) {
	meta, err := r.agentStore.AgentMeta(project, agentName)
	if err != nil {
		return nil, fmt.Errorf("load agent meta: %w", err)
	}

	agentDir := filepath.Join(r.root, "projects", project, "agents", agentName)

	// HTTP agent: bypass CLI subprocess, send prompt to HTTP endpoint directly.
	if entity.NormaliseModel(meta.Model) == entity.ModelHTTPAgent {
		return r.runTaskHTTP(project, agentName, agentDir, meta, task)
	}

	fullPrompt := task.Prompt + fmt.Sprintf(systemMetaFooter,
		task.ID, project, agentName, task.ID, task.ID, task.ID)

	// Write prompt to a temp file (avoids shell escaping issues).
	promptFile, err := writeTempPrompt(agentDir, fullPrompt)
	if err != nil {
		return nil, fmt.Errorf("write prompt file: %w", err)
	}
	defer os.Remove(promptFile)

	model := entity.NormaliseModel(meta.Model)
	agentEnv := resolveProviderEnv(r.root, meta)
	runtimeEnv := r.resolveRuntimeControlEnv(project, agentName, task.ID)
	if cleanup := r.materializeRuntimeFiles(agentDir, runtimeEnv); cleanup != nil {
		defer cleanup()
	}
	effectiveEnv := mergeEnv(os.Environ(), agentEnv)
	effectiveEnv = mergeEnv(effectiveEnv, runtimeEnv)
	apiModel, apiBaseURL := resolveAPIModelFromEnv(model, effectiveEnv)
	invoker := InvokerFor(model, meta.RunCommand, meta.AddDirs)

	// Build the inner agent CLI arguments.
	innerArgs := invoker.Args(promptFile, sessionID)

	// Determine the actual executable and final argument list.
	// When a Docker sandbox is configured the inner args become the command
	// run inside the container; otherwise they run directly on the host.
	var (
		executable string
		args       []string
		execDir    string // working directory for the host process
	)

	if meta.Sandbox != nil && meta.Sandbox.Provider != entity.SandboxNone {
		provider, ok := runenv.ProviderFor(meta.Sandbox.Provider)
		if !ok {
			return nil, fmt.Errorf("runtime provider %q is not implemented", meta.Sandbox.Provider)
		}
		if err := provider.Available(); err != nil {
			return nil, err
		}

		runtimeCfg := cloneRuntimeCfg(meta.Sandbox)
		agentCLI := agentcli.Effective(model, runtimeCfg.AgentCLI)
		injectProviderEnvIntoRuntime(runtimeCfg, agentEnv)
		injectRuntimeControlEnvIntoRuntime(runtimeCfg, runtimeEnv)
		mounts := append([]entity.RuntimeMount(nil), runtimeCfg.Mounts...)

		// Auto-mount the project's code repository at the same absolute path
		// inside the container. This lets the agent read/write/commit code at
		// the exact path it expects (e.g. /root/code/cc-connect), matching
		// what is written in CLAUDE.md / the project prompt.
		for _, addDir := range meta.AddDirs {
			mounts = runenv.AddPathMount(mounts, addDir, "repo", runenv.MountModeReadWrite)
		}

		// Auto-mount the workspace root at the same path so agents can use
		// `multigent task add --agent other-agent` to assign tasks to peers.
		// This enables PM agents to coordinate dev/qa agents without human
		// intervention.
		if wsMount := r.root + ":" + r.root; r.root != "" {
			runtimeCfg.Docker.ExtraVolumes = append(runtimeCfg.Docker.ExtraVolumes, wsMount)
		}

		// Auto-mount the multigent binary itself (read-only) so agents can
		// invoke `multigent` inside the container.
		if binMount := runtimecli.ResolveHostBinaryMount(); binMount != "" {
			runtimeCfg.Docker.ExtraVolumes = append(runtimeCfg.Docker.ExtraVolumes, binMount)
		}

		// The prompt file path inside the container.
		// innerArgs reference the host promptFile path — remap it to the real agent path.
		containerPromptFile := agentDir + "/" + filepath.Base(promptFile)
		remappedInner := remapPromptFile(innerArgs, promptFile, containerPromptFile)

		var err error
		executable, args, err = provider.Command(runenv.ProcessSpec{
			WorkspaceRoot: r.root,
			Project:       project,
			Agent:         agentName,
			AgentDir:      agentDir,
			Model:         model,
			Command:       remappedInner,
			Env:           agentEnv,
			Runtime:       runtimeCfg,
			AgentCLI:      agentCLI,
			Mounts:        mounts,
			Limits:        runtimeCfg.Resources,
		})
		if err != nil {
			return nil, fmt.Errorf("runtime %s: build command: %w", meta.Sandbox.Provider, err)
		}
		// docker run executes from wherever; cwd doesn't matter for container.
		execDir = ""
	} else {
		// Direct host execution.
		executable = innerArgs[0]
		args = innerArgs[1:]
		execDir = agentDir
	}

	// Prepare log file.
	logDir, err := r.ts.RunLogDir(project, agentName)
	if err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	logName := fmt.Sprintf("%s-%s.log", time.Now().UTC().Format("20060102-150405"), task.ID)
	logPath := filepath.Join(logDir, logName)
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("create log file: %w", err)
	}
	defer logFile.Close()

	sandboxLabel := "host"
	if meta.Sandbox != nil && meta.Sandbox.Provider != entity.SandboxNone {
		sandboxLabel = string(meta.Sandbox.Provider)
	}
	fmt.Fprintf(logFile, "=== multigent run: %s/%s task=%s sandbox=%s ===\n",
		project, agentName, task.ID, sandboxLabel)
	fmt.Fprintf(logFile, "Command: %s\n", telemetry.FormatExecCommand(executable, args))
	fmt.Fprintf(logFile, "Started: %s\n\n", time.Now().UTC().Format(time.RFC3339))

	// Run the agent.
	cmd := exec.Command(executable, args...)
	if execDir != "" {
		cmd.Dir = execDir
	}
	cmd.Env = effectiveEnv

	if invoker.UseStdinPrompt() {
		pf, err := os.Open(promptFile)
		if err != nil {
			return nil, fmt.Errorf("open prompt file for stdin: %w", err)
		}
		defer pf.Close()
		cmd.Stdin = pf
	}

	var outBuf bytes.Buffer
	multiOut := io.MultiWriter(&outBuf, logFile)
	cmd.Stdout = multiOut
	cmd.Stderr = multiOut

	runStarted := time.Now()
	runErr := cmd.Run()
	runFinished := time.Now()

	fmt.Fprintf(logFile, "\n=== exit code: %v  finished: %s ===\n",
		cmd.ProcessState.ExitCode(), time.Now().UTC().Format(time.RFC3339))

	output := outBuf.String()
	result := &RunResult{LogPath: logPath}

	// Parse session ID (model-specific + universal sentinel).
	if sid := invoker.ParseSessionID(output); sid != "" {
		result.SessionID = sid
	}
	if sid := parseLineSentinel(output, SessionSentinel); sid != "" {
		result.SessionID = sid
	}

	ec := exitCodeOrZero(cmd)
	cmdSummary := telemetry.FormatExecCommand(executable, args)

	// Check for confirmation sentinel (takes priority over exit code).
	if summary := parseLineSentinel(output, ConfirmSentinel); summary != "" {
		result.Status = entity.TaskStatusAwaitingConfirmation
		result.Summary = strings.TrimSpace(summary)
		r.recordAgentRun(telemetry.KindTask, project, agentName, task.ID, task.Title, string(model), sandboxLabel,
			apiModel, apiBaseURL,
			runStarted, runFinished, result.Status, &ec, result.SessionID, result.ErrorMsg,
			logPath, cmdSummary, fullPrompt, outBuf.Bytes())
		return result, nil
	}

	if runErr != nil {
		if sessionID != "" && isCodexResumeMissingRolloutError(output) {
			fmt.Fprintf(logFile, "\n=== codex rollout missing for saved session — clearing heartbeat session + retrying fresh ===\n")
			r.recordAgentRun(telemetry.KindTask, project, agentName, task.ID, task.Title, string(model), sandboxLabel,
				apiModel, apiBaseURL,
				runStarted, runFinished, entity.TaskStatusDoneFailed, &ec, result.SessionID,
				"codex rollout missing for saved session, retrying fresh",
				logPath, cmdSummary, fullPrompt, outBuf.Bytes())
			r.clearHeartbeatSession(project, agentName)
			return r.RunTask(project, agentName, task, "")
		}
		if sessionID != "" && isThinkingSignatureError(output) {
			fmt.Fprintf(logFile, "\n=== thinking block signature invalid — clearing heartbeat session + retrying fresh ===\n")
			r.recordAgentRun(telemetry.KindTask, project, agentName, task.ID, task.Title, string(model), sandboxLabel,
				apiModel, apiBaseURL,
				runStarted, runFinished, entity.TaskStatusDoneFailed, &ec, result.SessionID,
				"thinking block signature invalid, retrying fresh",
				logPath, cmdSummary, fullPrompt, outBuf.Bytes())
			r.clearHeartbeatSession(project, agentName)
			return r.RunTask(project, agentName, task, "")
		}
		if discardSessionIDOnFailure(model) {
			result.SessionID = ""
		}
		result.Status = entity.TaskStatusDoneFailed
		result.ErrorMsg = buildErrorMsg(runErr, outBuf.Bytes())
		r.recordAgentRun(telemetry.KindTask, project, agentName, task.ID, task.Title, string(model), sandboxLabel,
			apiModel, apiBaseURL,
			runStarted, runFinished, result.Status, &ec, result.SessionID, result.ErrorMsg,
			logPath, cmdSummary, fullPrompt, outBuf.Bytes())
		return result, runErr
	}

	result.Status = entity.TaskStatusDoneSuccess
	r.recordAgentRun(telemetry.KindTask, project, agentName, task.ID, task.Title, string(model), sandboxLabel,
		apiModel, apiBaseURL,
		runStarted, runFinished, result.Status, &ec, result.SessionID, result.ErrorMsg,
		logPath, cmdSummary, fullPrompt, outBuf.Bytes())
	return result, nil
}

// ResumeTask re-invokes the agent after human confirmation.
// The original task prompt is extended with the confirmation reply.
func (r *Runner) ResumeTask(project, agentName string, task *entity.Task, confirmReply, sessionID string) (*RunResult, error) {
	original := task.Prompt
	task.Prompt = original + "\n\n---\n[Human confirmed at " +
		time.Now().UTC().Format(time.RFC3339) + "]\n" +
		confirmReply + "\n\nPlease continue from where you left off.\n"
	result, err := r.RunTask(project, agentName, task, sessionID)
	task.Prompt = original // restore
	return result, err
}

func isThinkingSignatureError(output string) bool {
	return strings.Contains(output, "Invalid signature in thinking block") ||
		strings.Contains(output, "Invalid `signature` in `thinking` block")
}

func isCodexResumeMissingRolloutError(output string) bool {
	return strings.Contains(output, "thread/resume failed: no rollout found for thread id") ||
		strings.Contains(output, "no rollout found for thread id")
}

func discardSessionIDOnFailure(model entity.AgentModel) bool {
	switch entity.NormaliseModel(model) {
	case entity.ModelCodex, entity.ModelQoder:
		return true
	default:
		return false
	}
}

// clearHeartbeatSession zeroes the stored session ID in the heartbeat config
// so subsequent heartbeat triggers don't reuse a stale/invalid session.
func (r *Runner) clearHeartbeatSession(project, agent string) {
	hb, err := r.ts.GetHeartbeat(project, agent)
	if err != nil || hb == nil {
		return
	}
	hb.SessionID = ""
	hb.SessionStartedAt = nil
	_ = r.ts.SaveHeartbeat(project, agent, hb)
}

func exitCodeOrZero(cmd *exec.Cmd) int {
	if cmd == nil || cmd.ProcessState == nil {
		return 0
	}
	return cmd.ProcessState.ExitCode()
}

// buildErrorMsg combines the Go error string with the last few lines of
// process output so that the caller can see *why* the command failed,
// not just the exit code.  It also appends setup hints for common errors.
func buildErrorMsg(err error, output []byte) string {
	base := err.Error()
	tail := tailLines(output, 30, 2000)
	var msg string
	if tail == "" {
		msg = base
	} else {
		msg = base + "\n\n" + tail
	}
	if hint := detectSetupHint(msg); hint != "" {
		msg += "\n\n" + hint
	}
	return msg
}

// detectSetupHint checks the combined error output for well-known failure
// patterns and returns an actionable setup hint (prefixed with [hint]) that
// guides the user through resolving the issue.
func detectSetupHint(errText string) string {
	lower := strings.ToLower(errText)

	switch {
	// Docker image not found / pull denied
	case strings.Contains(lower, "unable to find image") && strings.Contains(lower, "denied"):
		return `[hint] Docker 镜像拉取失败。请确认：
1. Docker 已安装并运行：docker info
2. 如果是私有镜像，先登录：docker login ghcr.io
3. 或在成员详情页 → 沙箱配置中指定自定义镜像`

	// Docker not installed or daemon not running
	case strings.Contains(lower, "docker") && (strings.Contains(lower, "not found") || strings.Contains(lower, "cannot connect") || strings.Contains(lower, "daemon")):
		return `[hint] Docker 未安装或未启动。请：
1. 安装 Docker：https://docs.docker.com/get-docker/
2. 启动 Docker：sudo systemctl start docker
3. 或在成员详情页将沙箱切换为"无"（直接在宿主机运行）`

	// Claude Code CLI not found
	case strings.Contains(lower, "claude") && strings.Contains(lower, "not found"):
		return `[hint] Claude Code CLI 未安装。请：
1. 安装：npm install -g @anthropic-ai/claude-code
2. 验证：claude --version
3. 如果使用 Docker 沙箱，镜像内已预装，请检查沙箱配置`

	// Codex CLI not found
	case strings.Contains(lower, "codex") && strings.Contains(lower, "not found"):
		return `[hint] Codex CLI 未安装。请：
1. 安装：npm install -g @openai/codex
2. 验证：codex --version`

	// Cursor Agent not found
	case strings.Contains(lower, "agent") && strings.Contains(lower, "not found") && strings.Contains(lower, "executable"):
		return `[hint] Cursor Agent CLI 未安装。请：
1. 安装：curl -fsSL https://www.cursor.com/install-agent.sh | sh
2. 验证：agent --version
3. 如果已安装，确认 agent 在 PATH 中`

	// Cursor / agent authentication
	case strings.Contains(lower, "authentication required") && (strings.Contains(lower, "agent login") || strings.Contains(lower, "cursor_api_key")):
		return `[hint] Cursor Agent 未认证。请：
1. 在宿主机上运行：agent login
2. 或设置环境变量：CURSOR_API_KEY=your-key
3. 如果使用 Docker，确认 ~/.config/cursor/ 已正确挂载`

	// Claude Code authentication / invalid signature
	case strings.Contains(lower, "invalid signature") || (strings.Contains(lower, "anthropic") && strings.Contains(lower, "401")):
		return `[hint] Anthropic API 认证失败。请：
1. 检查 API Key 是否正确：设置页 → API 供应商
2. 如果使用第三方代理，确认 base URL 和 key 匹配
3. 通过 CLI 检查：multigent provider list`

	// OpenAI / Codex authentication
	case strings.Contains(lower, "openai") && (strings.Contains(lower, "401") || strings.Contains(lower, "unauthorized")):
		return `[hint] OpenAI API 认证失败。请：
1. 检查 OPENAI_API_KEY 是否正确
2. 设置页 → API 供应商中配置 OpenAI provider
3. 或通过 CLI：multigent envvar add OPENAI_API_KEY=sk-xxx`

	// dangerously-skip-permissions as root
	case strings.Contains(lower, "dangerously-skip-permissions") && strings.Contains(lower, "root"):
		return `[hint] Claude Code 不允许在 root 下使用 --dangerously-skip-permissions。请：
1. 在成员详情页将沙箱切换为 Docker（Docker 内已设置 IS_SANDBOX=1）
2. 或使用非 root 用户运行`

	// Read-only filesystem
	case strings.Contains(lower, "read-only file system") || strings.Contains(lower, "erofs"):
		return `[hint] 文件系统只读错误。请：
1. 检查 Docker 挂载是否使用了 :ro（只读）
2. 确认需要写入的路径没有被设为只读挂载
3. 在沙箱配置中调整挂载选项`

	// Sandbox / bwrap errors (Codex inner sandbox)
	case strings.Contains(lower, "bwrap") || strings.Contains(lower, "no permissions to create a new namespace"):
		return `[hint] Codex 内部沙箱冲突。请：
1. 在成员详情页将沙箱切换为 Docker（Docker 内会自动禁用内部沙箱）
2. 或设置环境变量：CODEX_UNSAFE_ALLOW_NO_SANDBOX=1`
	}
	return ""
}

func tailLines(data []byte, maxLines, maxBytes int) string {
	s := string(data)
	if len(s) > maxBytes*2 {
		s = s[len(s)-maxBytes*2:]
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	start := 0
	if len(lines) > maxLines {
		start = len(lines) - maxLines
	}
	result := strings.Join(lines[start:], "\n")
	if len(result) > maxBytes {
		result = result[len(result)-maxBytes:]
	}
	return strings.TrimSpace(result)
}

func httpCommandSummary(url string) string {
	return telemetry.TruncateCommand("HTTP POST "+strings.TrimSpace(url), 4000)
}

// recordAgentRun persists a row to .multigent/multigent.db; failures are ignored so runs are never blocked.
func (r *Runner) recordAgentRun(
	kind string,
	project, agent string,
	taskID, taskTitle string,
	modelNorm, sandbox string,
	apiModel, apiBaseURL string,
	started, finished time.Time,
	status entity.TaskStatus,
	exitCode *int,
	sessionID, errMsg string,
	absLogPath, cmdSummary, prompt string,
	stdout []byte,
) {
	// Keep the Claude sessions-index.json up-to-date so `claude /resume`
	// can list sessions created by non-interactive (pipeline) runs.
	agentDir := filepath.Join(r.root, "projects", project, "agents", agent)
	go updateClaudeSessionIndex(agentDir, sessionID)
	rec := telemetry.Record{
		Kind:           kind,
		StartedAt:      started,
		FinishedAt:     finished,
		Project:        project,
		Agent:          agent,
		TaskID:         taskID,
		TaskTitle:      taskTitle,
		Model:          modelNorm,
		APIModel:       apiModel,
		APIBaseURL:     apiBaseURL,
		Sandbox:        sandbox,
		Status:         string(status),
		SessionID:      sessionID,
		ErrorMsg:       errMsg,
		LogPathRel:     telemetry.RelLogPath(r.root, absLogPath),
		CommandSummary: telemetry.TruncateCommand(cmdSummary, 4000),
	}
	if exitCode != nil {
		rec.ExitCode = sql.NullInt64{Int64: int64(*exitCode), Valid: true}
	}
	rec.PromptBytes, rec.PromptSHA256 = telemetry.PromptFingerprint(prompt)
	telemetry.ApplyStreamUsage(&rec, telemetry.ParseStreamJSONUsage(stdout))
	_ = telemetry.Insert(r.root, rec)
}

// resolveAPIModelFromEnv extracts the actual API model name and base URL
// from the effective environment for the given agent model type.
func resolveAPIModelFromEnv(modelType entity.AgentModel, env []string) (apiModel, apiBaseURL string) {
	lookup := func(keys ...string) string {
		for i := len(env) - 1; i >= 0; i-- {
			k, v, _ := strings.Cut(env[i], "=")
			for _, want := range keys {
				if k == want && v != "" {
					return v
				}
			}
		}
		return ""
	}
	switch modelType {
	case entity.ModelClaudeCode:
		apiModel = lookup("ANTHROPIC_MODEL", "CLAUDE_MODEL")
		apiBaseURL = lookup("ANTHROPIC_BASE_URL", "ANTHROPIC_API_BASE")
	case entity.ModelCodex:
		apiModel = lookup("OPENAI_MODEL", "CODEX_MODEL")
		apiBaseURL = lookup("OPENAI_BASE_URL", "OPENAI_API_BASE")
	case entity.ModelGemini:
		apiModel = lookup("GEMINI_MODEL", "GOOGLE_MODEL")
		apiBaseURL = lookup("GOOGLE_API_BASE")
	case entity.ModelCursor:
		apiModel = lookup("CURSOR_MODEL")
	case entity.ModelOpenCode:
		apiModel = lookup("OPENAI_MODEL")
		apiBaseURL = lookup("OPENAI_BASE_URL", "OPENAI_API_BASE")
	}
	return
}

// ── helpers ────────────────────────────────────────────────────────────────────

func writeTempPrompt(agentDir, content string) (string, error) {
	// Store temp prompt files in .multigent/ to keep agent root clean.
	dir := filepath.Join(agentDir, ".multigent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, ".prompt-*.txt")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// shellEscape returns a single-quoted string safe for use in a bash command.
func shellEscape(s string) string {
	// Replace ' with '\'' and wrap in single quotes.
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// parseLineSentinel scans output line by line for lines starting with prefix.
// Returns everything after the prefix on the first matching line.
func parseLineSentinel(output, prefix string) string {
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if after, ok := strings.CutPrefix(line, prefix); ok {
			return strings.TrimSpace(after)
		}
	}
	return ""
}

// cloneDockerCfg returns a shallow copy of cfg (or a fresh struct if nil)
// so callers can mutate ExtraVolumes/ExtraEnv without affecting the original.
func cloneDockerCfg(cfg *entity.DockerSandboxConfig) *entity.DockerSandboxConfig {
	if cfg == nil {
		return &entity.DockerSandboxConfig{}
	}
	cp := *cfg
	cp.ExtraVolumes = append([]string(nil), cfg.ExtraVolumes...)
	cp.ExtraEnv = append([]string(nil), cfg.ExtraEnv...)
	cp.CredentialMounts = append([]string(nil), cfg.CredentialMounts...)
	return &cp
}

func cloneRuntimeCfg(cfg *entity.SandboxConfig) *entity.SandboxConfig {
	if cfg == nil {
		return &entity.SandboxConfig{Docker: &entity.DockerSandboxConfig{}}
	}
	cp := *cfg
	cp.Mounts = append([]entity.RuntimeMount(nil), cfg.Mounts...)
	cp.Env = append([]entity.RuntimeEnvVar(nil), cfg.Env...)
	if cfg.AgentCLI != nil {
		agentCLI := *cfg.AgentCLI
		agentCLI.Install = append([]string(nil), cfg.AgentCLI.Install...)
		agentCLI.Check = append([]string(nil), cfg.AgentCLI.Check...)
		cp.AgentCLI = &agentCLI
	}
	if cfg.Docker != nil {
		cp.Docker = cloneDockerCfg(cfg.Docker)
	} else {
		cp.Docker = &entity.DockerSandboxConfig{}
	}
	if cfg.E2B != nil {
		e2b := *cfg.E2B
		cp.E2B = &e2b
	}
	return &cp
}

// ── HTTP agent task/exec methods ───────────────────────────────────────────────

// runTaskHTTP runs a task by posting the full prompt to the agent's HTTP
// endpoint. The agent's context.md is sent as the system message; the task
// prompt + system meta footer become the user message.
func (r *Runner) runTaskHTTP(project, agentName, agentDir string, meta *entity.AgentMeta, task *entity.Task) (*RunResult, error) {
	if meta.HTTPAgent == nil {
		return nil, fmt.Errorf("http-agent: no http_agent config in .multigent-agent.yaml (re-hire with --http-url)")
	}

	userPrompt := task.Prompt + fmt.Sprintf(systemMetaFooter,
		task.ID, project, agentName, task.ID, task.ID, task.ID)

	logDir, err := r.ts.RunLogDir(project, agentName)
	if err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	logName := fmt.Sprintf("%s-%s.log", time.Now().UTC().Format("20060102-150405"), task.ID)
	logPath := filepath.Join(logDir, logName)
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("create log file: %w", err)
	}
	defer logFile.Close()

	fmt.Fprintf(logFile, "=== multigent run: %s/%s task=%s model=http-agent url=%s ===\n",
		project, agentName, task.ID, meta.HTTPAgent.URL)
	fmt.Fprintf(logFile, "Started: %s\n\n", time.Now().UTC().Format(time.RFC3339))

	systemPrompt := readAgentContextFile(agentDir)
	runStarted := time.Now()
	output, httpErr := httpExec(meta.HTTPAgent, systemPrompt, userPrompt, logFile, false)
	runFinished := time.Now()

	fmt.Fprintf(logFile, "\n=== finished: %s ===\n", time.Now().UTC().Format(time.RFC3339))

	result := &RunResult{LogPath: logPath}
	httpSummary := httpCommandSummary(meta.HTTPAgent.URL)
	modelNorm := string(entity.ModelHTTPAgent)
	sandboxLabel := "host"
	if meta.Sandbox != nil && meta.Sandbox.Provider != entity.SandboxNone {
		sandboxLabel = string(meta.Sandbox.Provider)
	}

	if httpErr != nil {
		result.Status = entity.TaskStatusDoneFailed
		result.ErrorMsg = httpErr.Error()
		r.recordAgentRun(telemetry.KindTask, project, agentName, task.ID, task.Title, modelNorm, sandboxLabel,
			"", "",
			runStarted, runFinished, result.Status, nil, result.SessionID, result.ErrorMsg,
			logPath, httpSummary, userPrompt, []byte(output))
		return result, nil
	}

	// Check for sentinels in the response text.
	if summary := parseLineSentinel(output, ConfirmSentinel); summary != "" {
		result.Status = entity.TaskStatusAwaitingConfirmation
		result.Summary = strings.TrimSpace(summary)
		r.recordAgentRun(telemetry.KindTask, project, agentName, task.ID, task.Title, modelNorm, sandboxLabel,
			"", "",
			runStarted, runFinished, result.Status, nil, result.SessionID, result.ErrorMsg,
			logPath, httpSummary, userPrompt, []byte(output))
		return result, nil
	}
	if sid := parseLineSentinel(output, SessionSentinel); sid != "" {
		result.SessionID = sid
	}

	result.Status = entity.TaskStatusDoneSuccess
	r.recordAgentRun(telemetry.KindTask, project, agentName, task.ID, task.Title, modelNorm, sandboxLabel,
		"", "",
		runStarted, runFinished, result.Status, nil, result.SessionID, result.ErrorMsg,
		logPath, httpSummary, userPrompt, []byte(output))
	return result, nil
}

// execPromptHTTP handles ExecPrompt for http-agent: sends the raw prompt to
// the HTTP endpoint and streams the response to stdout + log file.
func (r *Runner) execPromptHTTP(agentDir string, meta *entity.AgentMeta, prompt string) (*RunResult, error) {
	if meta.HTTPAgent == nil {
		return nil, fmt.Errorf("http-agent: no http_agent config in .multigent-agent.yaml (re-hire with --http-url)")
	}

	logDir, err := r.ts.RunLogDir(meta.Project, meta.Name)
	if err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	logName := fmt.Sprintf("%s-exec.log", time.Now().UTC().Format("20060102-150405"))
	logPath := filepath.Join(logDir, logName)
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("create log file: %w", err)
	}
	defer logFile.Close()

	fmt.Fprintf(logFile, "=== multigent exec: %s/%s model=http-agent url=%s ===\n",
		meta.Project, meta.Name, meta.HTTPAgent.URL)
	fmt.Fprintf(logFile, "Started: %s\n\n", time.Now().UTC().Format(time.RFC3339))

	systemPrompt := readAgentContextFile(agentDir)
	runStarted := time.Now()
	output, httpErr := httpExec(meta.HTTPAgent, systemPrompt, prompt, logFile, true)
	runFinished := time.Now()

	fmt.Fprintf(logFile, "\n=== finished: %s ===\n", time.Now().UTC().Format(time.RFC3339))

	result := &RunResult{LogPath: logPath}
	httpSummary := httpCommandSummary(meta.HTTPAgent.URL)
	modelNorm := string(entity.ModelHTTPAgent)
	sandboxLabel := "host"
	if meta.Sandbox != nil && meta.Sandbox.Provider != entity.SandboxNone {
		sandboxLabel = string(meta.Sandbox.Provider)
	}
	if httpErr != nil {
		result.Status = entity.TaskStatusDoneFailed
		result.ErrorMsg = httpErr.Error()
		r.recordAgentRun(telemetry.KindExec, meta.Project, meta.Name, "", "", modelNorm, sandboxLabel,
			"", "",
			runStarted, runFinished, result.Status, nil, result.SessionID, result.ErrorMsg,
			logPath, httpSummary, prompt, []byte(output))
		return result, nil
	}
	if sid := parseLineSentinel(output, SessionSentinel); sid != "" {
		result.SessionID = sid
	}
	result.Status = entity.TaskStatusDoneSuccess
	r.recordAgentRun(telemetry.KindExec, meta.Project, meta.Name, "", "", modelNorm, sandboxLabel,
		"", "",
		runStarted, runFinished, result.Status, nil, result.SessionID, result.ErrorMsg,
		logPath, httpSummary, prompt, []byte(output))
	return result, nil
}

// remapPromptFile replaces occurrences of hostPath with containerPath in args.
// This is needed when the prompt file is written to the host working directory
// but the container sees it at the /workspace mount point.
func remapPromptFile(args []string, hostPath, containerPath string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = strings.ReplaceAll(a, hostPath, containerPath)
	}
	return out
}

// resolveProviderEnv loads env vars from all sources and merges them.
// Resolution priority (lowest → highest):
//  1. Workspace global secrets
//  2. Workspace agent-scoped secrets
//  3. API provider env
//  4. Per-agent env (AgentMeta.Env)
//  5. Explicit runtime model (AgentMeta.RuntimeModel)
func resolveProviderEnv(root string, meta *entity.AgentMeta) map[string]string {
	merged := make(map[string]string)

	// 0: agent identity env vars (lowest priority, injected by multigent)
	merged["MULTIGENT"] = "1"
	merged["MULTIGENT_PROJECT"] = meta.Project
	merged["MULTIGENT_AGENT"] = meta.Name
	merged["MULTIGENT_TEAM"] = meta.Team
	merged["MULTIGENT_ROLE"] = meta.Role
	merged["MULTIGENT_MODEL"] = string(meta.Model)
	merged["MULTIGENT_ROOT"] = root

	// 1+2: workspace env vars (global first, then agent-scoped overrides)
	es := store.NewEnvVarStore(root)
	if wsEnv, err := es.ResolveEnvForAgent(meta.Project, meta.Name); err == nil {
		for k, v := range wsEnv {
			merged[k] = v
		}
	}

	// 3: API provider env
	if meta.Provider != "" {
		ps := store.NewProviderStore(root)
		if provEnv, err := ps.ResolveEnv(meta.Provider); err == nil {
			for k, v := range provEnv {
				merged[k] = v
			}
		}
	}

	// 4: per-agent env (highest priority)
	for k, v := range meta.Env {
		merged[k] = v
	}
	for k, v := range runtimeModelEnv(meta.Model, meta.RuntimeModel) {
		merged[k] = v
	}
	return merged
}

func runtimeModelEnv(model entity.AgentModel, runtimeModel string) map[string]string {
	runtimeModel = strings.TrimSpace(runtimeModel)
	if runtimeModel == "" {
		return nil
	}
	switch entity.NormaliseModel(model) {
	case entity.ModelClaudeCode:
		return map[string]string{
			"ANTHROPIC_MODEL": runtimeModel,
			"CLAUDE_MODEL":    runtimeModel,
		}
	case entity.ModelCodex:
		return map[string]string{
			"OPENAI_MODEL": runtimeModel,
			"CODEX_MODEL":  runtimeModel,
		}
	case entity.ModelGemini:
		return map[string]string{
			"GEMINI_MODEL": runtimeModel,
			"GOOGLE_MODEL": runtimeModel,
		}
	case entity.ModelCursor:
		return map[string]string{
			"CURSOR_MODEL": runtimeModel,
		}
	case entity.ModelOpenCode:
		return map[string]string{
			"OPENAI_MODEL": runtimeModel,
		}
	default:
		return map[string]string{
			"MULTIGENT_RUNTIME_MODEL": runtimeModel,
		}
	}
}

func (r *Runner) resolveRuntimeControlEnv(project, agentName, runID string) map[string]string {
	apiURL := resolveRuntimeAPIURL(r.root)
	if apiURL == "" {
		return nil
	}
	controlDB, err := controldb.OpenDefault()
	if err != nil {
		return nil
	}
	defer controlDB.Close()
	workspaceID := resolveRuntimeWorkspaceID(r.root, controlDB)
	secret := runtimeauth.EnsureSecret(controlDB)
	token := runtimeauth.Issue(secret, runtimeauth.Payload{
		WorkspaceID:  workspaceID,
		Project:      project,
		Agent:        agentName,
		RunID:        runID,
		Capabilities: []string{"connection.use", "task.use", "message.use", "okr.use", "docs.use"},
	}, 6*time.Hour)
	return map[string]string{
		"MULTIGENT_API_URL":      apiURL,
		"MULTIGENT_AGENT_TOKEN":  token,
		"MULTIGENT_RUN_ID":       runID,
		"MULTIGENT_WORKSPACE_ID": workspaceID,
	}
}

func (r *Runner) materializeRuntimeFiles(agentDir string, env map[string]string) func() {
	if len(env) == 0 {
		return nil
	}
	apiURL := strings.TrimSpace(env["MULTIGENT_API_URL"])
	token := strings.TrimSpace(env["MULTIGENT_AGENT_TOKEN"])
	if apiURL == "" || token == "" {
		return nil
	}
	body, err := fetchRuntimeConnectionsManifest(apiURL, token)
	if err != nil {
		return nil
	}
	path, err := writeRuntimeConnectionsFile(agentDir, body)
	if err != nil {
		return nil
	}
	env[runtimeConnectionsFileEnv] = path
	secretResolver, closeResolver := newRuntimeConnectionSecretResolver()
	if closeResolver != nil {
		defer closeResolver()
	}
	toolDir, toolsPath, extraEnv, err := writeRuntimeToolsFile(agentDir, env["MULTIGENT_RUN_ID"], path, body, secretResolver)
	if err == nil && toolDir != "" && toolsPath != "" {
		env[runtimeToolDirEnv] = toolDir
		env[runtimeToolsFileEnv] = toolsPath
		env[runtimeToolBinDirEnv] = filepath.Join(toolDir, "bin")
		env["PATH"] = filepath.Join(toolDir, "bin") + string(os.PathListSeparator) + os.Getenv("PATH")
		for key, value := range extraEnv {
			if key != "" && value != "" {
				env[key] = value
			}
		}
	}
	return func() {
		_ = os.Remove(path)
		if toolDir != "" {
			_ = os.RemoveAll(toolDir)
		}
	}
}

// materializeRuntimeConnectionsFile is kept as a narrow helper for tests and
// callers that only need the raw connection manifest. Agent runs should use
// materializeRuntimeFiles so the tool adapter plan is available too.
func (r *Runner) materializeRuntimeConnectionsFile(agentDir string, env map[string]string) func() {
	if cleanup := r.materializeRuntimeFiles(agentDir, env); cleanup != nil {
		toolsPath := env[runtimeToolsFileEnv]
		toolDir := env[runtimeToolDirEnv]
		delete(env, runtimeToolsFileEnv)
		delete(env, runtimeToolDirEnv)
		return func() {
			if toolsPath != "" {
				_ = os.Remove(toolsPath)
			}
			if toolDir != "" {
				_ = os.RemoveAll(toolDir)
			}
			cleanup()
		}
	}
	return nil
}

func fetchRuntimeConnectionsManifest(apiURL, token string) ([]byte, error) {
	apiURL = strings.TrimRight(strings.TrimSpace(apiURL), "/")
	token = strings.TrimSpace(token)
	if apiURL == "" || token == "" {
		return nil, fmt.Errorf("runtime API URL and token are required")
	}
	req, err := http.NewRequest(http.MethodGet, apiURL+"/api/v1/runtime/connections", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("runtime connections returned %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRuntimeConnectionsFile+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxRuntimeConnectionsFile {
		return nil, fmt.Errorf("runtime connections manifest too large")
	}
	if !json.Valid(body) {
		return nil, fmt.Errorf("runtime connections manifest is not valid JSON")
	}
	return body, nil
}

func writeRuntimeConnectionsFile(agentDir string, body []byte) (string, error) {
	dir := filepath.Join(agentDir, ".multigent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, ".connections-*.json")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.Write(body); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

type runtimeToolsPlan struct {
	Version         string           `json:"version"`
	GeneratedAt     string           `json:"generatedAt"`
	RuntimeDir      string           `json:"runtimeDir"`
	ConnectionsFile string           `json:"connectionsFile"`
	Tools           []runtimeToolRef `json:"tools"`
}

type runtimeToolRef struct {
	Provider           string              `json:"provider"`
	DisplayName        string              `json:"displayName,omitempty"`
	ConnectionID       string              `json:"connectionId"`
	ConnectionAlias    string              `json:"connectionAlias"`
	ConnectionName     string              `json:"connectionName"`
	RecommendedAdapter string              `json:"recommendedAdapter,omitempty"`
	Adapters           []runtimeAdapterRef `json:"adapters,omitempty"`
	Skills             []string            `json:"skills,omitempty"`
	Actions            []runtimeActionRef  `json:"actions,omitempty"`
}

type runtimeAdapterRef struct {
	Type                  string          `json:"type"`
	Priority              int             `json:"priority"`
	Description           string          `json:"description,omitempty"`
	Skills                []string        `json:"skills,omitempty"`
	CLI                   *runtimeCLIRef  `json:"cli,omitempty"`
	MCPGateway            map[string]any  `json:"mcpGateway,omitempty"`
	HTTPAction            *runtimeHTTPRef `json:"httpAction,omitempty"`
	CredentialMaterialize string          `json:"credentialMaterialize,omitempty"`
	Audit                 map[string]any  `json:"audit,omitempty"`
}

type runtimeCLIRef struct {
	Binary      string                 `json:"binary"`
	Installer   map[string]any         `json:"installer,omitempty"`
	ConfigFiles []runtimeConfigFileRef `json:"configFiles,omitempty"`
}

type runtimeConfigFileRef struct {
	Path             string `json:"path"`
	Format           string `json:"format,omitempty"`
	Description      string `json:"description,omitempty"`
	MaterializedPath string `json:"materializedPath,omitempty"`
}

type runtimeHTTPRef struct {
	ActionNames []string `json:"actionNames,omitempty"`
}

type runtimeActionRef struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName,omitempty"`
	Method      string `json:"method,omitempty"`
	Endpoint    string `json:"endpoint,omitempty"`
}

type runtimeConnectionSecretResolver func(connectionID string) (map[string]string, bool, error)

func writeRuntimeToolsFile(agentDir, runID, connectionsPath string, body []byte, secretResolver runtimeConnectionSecretResolver) (string, string, map[string]string, error) {
	var manifest struct {
		Tools []runtimeToolRef `json:"tools"`
	}
	if err := json.Unmarshal(body, &manifest); err != nil {
		return "", "", nil, err
	}
	if len(manifest.Tools) == 0 {
		return "", "", nil, nil
	}
	toolDir := filepath.Join(agentDir, ".multigent", "runtime-tools", safeRuntimePathPart(runID))
	if err := os.MkdirAll(toolDir, 0o700); err != nil {
		return "", "", nil, err
	}
	if err := os.MkdirAll(filepath.Join(toolDir, "bin"), 0o700); err != nil {
		return "", "", nil, err
	}
	extraEnv := map[string]string{}
	for ti := range manifest.Tools {
		var secretValues map[string]string
		if secretResolver != nil {
			values, ok, err := secretResolver(manifest.Tools[ti].ConnectionID)
			if err != nil {
				return "", "", nil, err
			}
			if ok {
				secretValues = values
			}
		}
		for ai := range manifest.Tools[ti].Adapters {
			adapter := &manifest.Tools[ti].Adapters[ai]
			if adapter.CLI == nil {
				continue
			}
			for ci := range adapter.CLI.ConfigFiles {
				cfg := &adapter.CLI.ConfigFiles[ci]
				cfg.MaterializedPath = runtimeConfigMaterializedPath(toolDir, manifest.Tools[ti], cfg.Path)
				if err := os.MkdirAll(filepath.Dir(cfg.MaterializedPath), 0o700); err != nil {
					return "", "", nil, err
				}
				env, err := materializeCLIConfig(manifest.Tools[ti], *adapter, *cfg, secretValues)
				if err != nil {
					return "", "", nil, err
				}
				for key, value := range env {
					extraEnv[key] = value
				}
			}
		}
	}
	plan := runtimeToolsPlan{
		Version:         "multigent.tools.v1",
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
		RuntimeDir:      toolDir,
		ConnectionsFile: connectionsPath,
		Tools:           manifest.Tools,
	}
	planBody, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return "", "", nil, err
	}
	planPath := filepath.Join(toolDir, "tools.json")
	if err := os.WriteFile(planPath, append(planBody, '\n'), 0o600); err != nil {
		return "", "", nil, err
	}
	return toolDir, planPath, extraEnv, nil
}

func runtimeConfigMaterializedPath(toolDir string, tool runtimeToolRef, configuredPath string) string {
	path := strings.TrimSpace(configuredPath)
	path = strings.TrimPrefix(path, "~/")
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		path = "config"
	}
	return filepath.Join(
		toolDir,
		"home",
		safeRuntimePathPart(tool.Provider),
		safeRuntimePathPart(tool.ConnectionAlias),
		filepath.FromSlash(path),
	)
}

func materializeCLIConfig(tool runtimeToolRef, adapter runtimeAdapterRef, cfg runtimeConfigFileRef, secretValues map[string]string) (map[string]string, error) {
	switch strings.TrimSpace(tool.Provider) {
	case "github":
		return materializeGitHubCLIConfig(adapter, cfg, secretValues)
	case "feishu", "lark":
		return materializeLarkCLIConfig(tool, adapter, cfg, secretValues)
	default:
		return nil, nil
	}
}

func materializeGitHubCLIConfig(adapter runtimeAdapterRef, cfg runtimeConfigFileRef, secretValues map[string]string) (map[string]string, error) {
	if adapter.CLI == nil || adapter.CLI.Binary != "gh" {
		return nil, nil
	}
	if !strings.HasSuffix(strings.TrimSpace(cfg.Path), "hosts.yml") {
		return nil, nil
	}
	token := firstNonEmpty(secretValues["apiKey"], secretValues["accessToken"], secretValues["token"])
	if token == "" || cfg.MaterializedPath == "" {
		return nil, nil
	}
	body := "github.com:\n  oauth_token: " + yamlQuote(token) + "\n  git_protocol: https\n"
	if err := os.WriteFile(cfg.MaterializedPath, []byte(body), 0o600); err != nil {
		return nil, err
	}
	return map[string]string{"GH_CONFIG_DIR": filepath.Dir(cfg.MaterializedPath)}, nil
}

func materializeLarkCLIConfig(tool runtimeToolRef, adapter runtimeAdapterRef, cfg runtimeConfigFileRef, secretValues map[string]string) (map[string]string, error) {
	if adapter.CLI == nil || adapter.CLI.Binary != "lark-cli" {
		return nil, nil
	}
	if !strings.HasSuffix(strings.TrimSpace(cfg.Path), ".lark-cli/config.json") {
		return nil, nil
	}
	appID := firstNonEmpty(secretValues["appId"], secretValues["clientId"])
	appSecret := firstNonEmpty(secretValues["appSecret"], secretValues["clientSecret"])
	if appID == "" || appSecret == "" || cfg.MaterializedPath == "" {
		return nil, nil
	}
	brand := larkBrand(tool.Provider, firstNonEmpty(secretValues["baseUrl"], secretValues["apiBaseUrl"]))
	body, err := json.MarshalIndent(map[string]any{
		"apps": []map[string]any{
			{
				"name":      firstNonEmpty(tool.ConnectionAlias, tool.ConnectionName, tool.Provider),
				"appId":     appID,
				"appSecret": appSecret,
				"brand":     brand,
				"users":     []any{},
			},
		},
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(cfg.MaterializedPath, append(body, '\n'), 0o600); err != nil {
		return nil, err
	}
	larkHome := filepath.Dir(filepath.Dir(cfg.MaterializedPath))
	binDir := filepath.Join(runtimeToolDirFromConfigPath(cfg.MaterializedPath), "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		return nil, err
	}
	wrapperPath := filepath.Join(binDir, "lark-cli")
	if err := os.WriteFile(wrapperPath, []byte(larkCLIWrapperScript(larkHome)), 0o700); err != nil {
		return nil, err
	}
	return map[string]string{"MULTIGENT_LARK_HOME": larkHome}, nil
}

func larkBrand(provider, baseURL string) string {
	if strings.EqualFold(strings.TrimSpace(provider), "lark") || strings.Contains(strings.ToLower(baseURL), "larksuite") {
		return "lark"
	}
	return "feishu"
}

func runtimeToolDirFromConfigPath(path string) string {
	dir := filepath.Dir(path)
	for {
		if filepath.Base(dir) == "home" {
			parent := filepath.Dir(dir)
			if parent != "." && parent != dir {
				return parent
			}
		}
		next := filepath.Dir(dir)
		if next == dir {
			return filepath.Dir(path)
		}
		dir = next
	}
}

func larkCLIWrapperScript(home string) string {
	return `#!/bin/sh
set -eu
tool_bin_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
clean_path=""
old_ifs=$IFS
IFS=:
for p in ${PATH:-}; do
  if [ "$p" = "$tool_bin_dir" ]; then
    continue
  fi
  if [ -z "$clean_path" ]; then
    clean_path=$p
  else
    clean_path=$clean_path:$p
  fi
done
IFS=$old_ifs
export PATH=$clean_path
export HOME=` + shellQuote(home) + `
exec lark-cli "$@"
`
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func yamlQuote(value string) string {
	escaped := strings.ReplaceAll(value, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

func newRuntimeConnectionSecretResolver() (runtimeConnectionSecretResolver, func()) {
	controlDB, err := controldb.OpenDefault()
	if err != nil {
		return nil, nil
	}
	return func(connectionID string) (map[string]string, bool, error) {
		secret, ok, err := controlDB.ConnectionSecret(connectionID)
		if err != nil || !ok {
			return nil, ok, err
		}
		values, err := openRuntimeConnectionSecret(secret)
		return values, true, err
	}, func() { _ = controlDB.Close() }
}

func openRuntimeConnectionSecret(secret controldb.ConnectionSecret) (map[string]string, error) {
	if secret.Ciphertext == "" {
		return map[string]string{}, nil
	}
	var raw []byte
	switch secret.KeyVersion {
	case "", "plain-dev":
		decoded, err := base64.StdEncoding.DecodeString(secret.Ciphertext)
		if err != nil {
			return nil, err
		}
		raw = decoded
	case "env-v1":
		key := strings.TrimSpace(os.Getenv("MULTIGENT_CONNECTION_ENCRYPTION_KEY"))
		if key == "" {
			return nil, fmt.Errorf("MULTIGENT_CONNECTION_ENCRYPTION_KEY is required to decrypt connection secret")
		}
		ciphertextBody, err := base64.StdEncoding.DecodeString(secret.Ciphertext)
		if err != nil {
			return nil, err
		}
		nonce, err := base64.StdEncoding.DecodeString(secret.Nonce)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256([]byte(key))
		block, err := aes.NewCipher(sum[:])
		if err != nil {
			return nil, err
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}
		opened, err := gcm.Open(nil, nonce, ciphertextBody, nil)
		if err != nil {
			return nil, err
		}
		raw = opened
	default:
		return nil, fmt.Errorf("unsupported connection secret key version %q", secret.KeyVersion)
	}
	out := map[string]string{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func safeRuntimePathPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "default"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(value) {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "default"
	}
	if len(out) > 80 {
		return out[:80]
	}
	return out
}

func resolveRuntimeAPIURL(root string) string {
	if value := strings.TrimSpace(os.Getenv("MULTIGENT_API_URL")); value != "" {
		return normalizeRuntimeAPIURL(value)
	}
	meta, err := daemon.LoadWebRuntimeMeta(root)
	if err != nil || meta == nil || strings.TrimSpace(meta.Addr) == "" {
		return ""
	}
	return normalizeRuntimeAPIURL(meta.Addr)
}

func normalizeRuntimeAPIURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return strings.TrimRight(value, "/")
	}
	if strings.HasPrefix(value, ":") {
		return "http://127.0.0.1" + value
	}
	host, port, err := net.SplitHostPort(value)
	if err == nil {
		if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
			host = "127.0.0.1"
		}
		return "http://" + net.JoinHostPort(host, port)
	}
	return "http://" + strings.TrimRight(value, "/")
}

func resolveRuntimeWorkspaceID(root string, controlDB controldb.Store) string {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	workspaces, err := controlDB.ListWorkspaces()
	if err == nil {
		for _, workspace := range workspaces {
			if sameRuntimePath(workspace.Root, absRoot) && workspace.ID != "" {
				return workspace.ID
			}
		}
	}
	return runtimeWorkspaceID(absRoot)
}

func sameRuntimePath(a, b string) bool {
	aa, err := filepath.Abs(a)
	if err != nil {
		aa = a
	}
	bb, err := filepath.Abs(b)
	if err != nil {
		bb = b
	}
	return filepath.Clean(aa) == filepath.Clean(bb)
}

func runtimeWorkspaceID(root string) string {
	sum := sha1.Sum([]byte(root))
	return hex.EncodeToString(sum[:])[:12]
}

// injectProviderEnvIntoDocker adds agent/provider env vars as explicit
// KEY=VALUE entries into the Docker config's ExtraEnv. This is necessary
// because Docker's `-e KEY` (inherit) mode only copies vars from the host
// OS environment, but provider-resolved vars exist only in the Go map.
func injectProviderEnvIntoDocker(cfg *entity.DockerSandboxConfig, env map[string]string) {
	for k, v := range env {
		cfg.ExtraEnv = append(cfg.ExtraEnv, k+"="+v)
	}
}

func injectProviderEnvIntoRuntime(cfg *entity.SandboxConfig, env map[string]string) {
	if cfg == nil {
		return
	}
	for k, v := range env {
		if k == "" {
			continue
		}
		if isRuntimeControlEnvKey(k) {
			continue
		}
		cfg.Env = append(cfg.Env, entity.RuntimeEnvVar{Name: k, Value: v})
	}
}

func injectRuntimeControlEnvIntoRuntime(cfg *entity.SandboxConfig, env map[string]string) {
	if cfg == nil {
		return
	}
	for k, v := range env {
		if k == "" {
			continue
		}
		if k == runtimeToolBinDirEnv || k == "PATH" {
			cfg.Env = append(cfg.Env, entity.RuntimeEnvVar{Name: k, Value: v})
			continue
		}
		cfg.Env = append(cfg.Env, entity.RuntimeEnvVar{Name: k, Inherit: true})
	}
}

func isRuntimeControlEnvKey(key string) bool {
	switch key {
	case "MULTIGENT_API_URL", "MULTIGENT_AGENT_TOKEN", "MULTIGENT_RUN_ID", "MULTIGENT_WORKSPACE_ID", runtimeConnectionsFileEnv, runtimeToolsFileEnv, runtimeToolDirEnv, runtimeToolBinDirEnv:
		return true
	default:
		return false
	}
}

// mergeEnv returns a copy of base with the entries in override applied.
// Keys in override replace matching keys in base (case-sensitive match on
// the part before '='); new keys are appended.
func mergeEnv(base []string, override map[string]string) []string {
	if len(override) == 0 {
		return base
	}
	overKeys := make(map[string]string, len(override))
	for k, v := range override {
		overKeys[k] = v
	}
	out := make([]string, 0, len(base)+len(override))
	seen := make(map[string]bool, len(override))
	for _, entry := range base {
		k, _, _ := strings.Cut(entry, "=")
		if v, ok := overKeys[k]; ok {
			out = append(out, k+"="+v)
			seen[k] = true
		} else {
			out = append(out, entry)
		}
	}
	for k, v := range overKeys {
		if !seen[k] {
			out = append(out, k+"="+v)
		}
	}
	return out
}
