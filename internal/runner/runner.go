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
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/multigent/multigent/internal/agentcli"
	"github.com/multigent/multigent/internal/daemon"
	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/runenv"
	"github.com/multigent/multigent/internal/runtimeauth"
	"github.com/multigent/multigent/internal/runtimecli"
	"github.com/multigent/multigent/internal/runtimeguide"
	"github.com/multigent/multigent/internal/store"
	"github.com/multigent/multigent/internal/taskstore"
	"github.com/multigent/multigent/internal/telemetry"
	workflowstore "github.com/multigent/multigent/internal/workflow"
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
	runtimeToolCacheBinDirEnv = "MULTIGENT_TOOL_CACHE_BIN_DIR"
	runtimeToolBootstrapEnv   = "MULTIGENT_TOOL_BOOTSTRAP_FILE"
	runtimeToolSkillsFileEnv  = "MULTIGENT_TOOL_SKILLS_FILE"
	runtimeToolCLIAuditEnv    = "MULTIGENT_TOOL_CLI_AUDIT_FILE"
	runtimeMCPConfigEnv       = "MULTIGENT_MCP_CONFIGURED"
	maxRuntimeConnectionsFile = 1 << 20
)

// systemMetaFooter is appended to every task prompt so agents know how to
// call back into the Multigent Server through the agent runtime CLI.
const systemMetaFooter = `

---
## System Metadata (do not modify)

Task ID : %s
Agent   : %s/%s

For a regular task with no Workflow Context, complete the whole task with:
  mga task complete --id %s --status success

If human confirmation needed, run:
  mga task confirm-request --id %s --summary "one-line explanation"
  (then exit 0)

If a regular task cannot be completed, run:
  mga task complete --id %s --status failed --error "reason"

For a workflow task, do not complete the whole task directly. Complete only the current step with:
  mga task step done --id %s --status success --output <field>=<value>

Important: do not use Claude Code built-in Task, TaskUpdate, or Todo tools to update Multigent task/workflow state. They are model-local planning tools and do not change Multigent records. Use mga task ... only.
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
	if err := r.materializeProviderCredentials(agentDir, meta); err != nil {
		return nil, fmt.Errorf("materialize provider credentials: %w", err)
	}
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
		processRuntimeEnv := runtimeControlEnvForProvider(runtimeEnv, meta.Sandbox.Provider)
		effectiveEnv = mergeEnv(effectiveEnv, processRuntimeEnv)
		injectProviderEnvIntoRuntime(runtimeCfg, agentEnv)
		injectRuntimeControlEnvIntoRuntime(runtimeCfg, processRuntimeEnv)
		mounts := append([]entity.RuntimeMount(nil), runtimeCfg.Mounts...)
		r.addRuntimeDockerSystemMounts(runtimeCfg)
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
	writePromptMessageToLog(logFile, prompt)

	// Stream output to stdout AND the log file simultaneously.
	cmd := exec.Command(executable, args...)
	if execDir != "" {
		cmd.Dir = execDir
	}
	if meta.Sandbox != nil && meta.Sandbox.Provider == entity.SandboxDocker {
		effectiveEnv = ensureHostDockerPATH(effectiveEnv)
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

	fullPrompt := r.taskPromptWithWorkflowContext(project, agentName, task) + fmt.Sprintf(systemMetaFooter,
		task.ID, project, agentName, task.ID, task.ID, task.ID, task.ID)

	// Write prompt to a temp file (avoids shell escaping issues).
	promptFile, err := writeTempPrompt(agentDir, fullPrompt)
	if err != nil {
		return nil, fmt.Errorf("write prompt file: %w", err)
	}
	defer os.Remove(promptFile)

	model := entity.NormaliseModel(meta.Model)
	agentEnv := resolveProviderEnv(r.root, meta)
	if err := r.materializeProviderCredentials(agentDir, meta); err != nil {
		return nil, fmt.Errorf("materialize provider credentials: %w", err)
	}
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
		processRuntimeEnv := runtimeControlEnvForProvider(runtimeEnv, meta.Sandbox.Provider)
		effectiveEnv = mergeEnv(effectiveEnv, processRuntimeEnv)
		injectProviderEnvIntoRuntime(runtimeCfg, agentEnv)
		injectRuntimeControlEnvIntoRuntime(runtimeCfg, processRuntimeEnv)
		mounts := append([]entity.RuntimeMount(nil), runtimeCfg.Mounts...)

		r.addRuntimeDockerSystemMounts(runtimeCfg)

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
	if meta.Sandbox != nil && meta.Sandbox.Provider == entity.SandboxDocker {
		effectiveEnv = ensureHostDockerPATH(effectiveEnv)
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

func (r *Runner) taskPromptWithWorkflowContext(project, agentName string, task *entity.Task) string {
	if task == nil {
		return ""
	}
	ctx := strings.TrimSpace(r.workflowPromptContext(project, agentName, task.ID))
	if ctx == "" {
		return task.Prompt
	}
	return ctx + "\n\n---\n## Task Prompt\n\n" + task.Prompt
}

func (r *Runner) workflowPromptContext(project, agentName, taskID string) string {
	controlDB, err := controldb.OpenDefault()
	if err != nil {
		return ""
	}
	defer controlDB.Close()
	workspaceID := resolveRuntimeWorkspaceID(r.root, controlDB)
	wfStore := workflowstore.NewStore(controlDB, workspaceID)
	run, ok, err := wfStore.RunForTask(project, taskID)
	if err != nil || !ok {
		return ""
	}
	def, ok, err := wfStore.Definition(run.DefinitionID)
	if err != nil || !ok {
		return ""
	}
	instances, err := wfStore.ListStepInstances(run.ID)
	if err != nil {
		instances = nil
	}
	step, ok := workflowStepByID(def.Steps, run.ActiveStepID)
	if !ok {
		return ""
	}
	inst, _ := workflowStepInstanceByStepID(instances, step.ID)

	var b strings.Builder
	b.WriteString("## Workflow Context\n\n")
	fmt.Fprintf(&b, "- Workflow: %s (`%s`)\n", def.Name, def.ID)
	fmt.Fprintf(&b, "- Run ID: `%s`\n", run.ID)
	fmt.Fprintf(&b, "- Current step: %s (`%s`, type: `%s`)\n", step.Title, step.ID, step.Type)
	if step.Description != "" {
		fmt.Fprintf(&b, "- Step goal: %s\n", step.Description)
	}
	if step.ActorRole != "" {
		fmt.Fprintf(&b, "- Step actor role: `%s`\n", step.ActorRole)
	}
	if inst.ActorType != "" || inst.ActorID != "" {
		fmt.Fprintf(&b, "- Assigned actor: `%s:%s`\n", inst.ActorType, inst.ActorID)
	}
	fmt.Fprintf(&b, "- Running agent: `%s/%s`\n", project, agentName)
	b.WriteString("\n")
	if len(step.InputFields) > 0 {
		b.WriteString("Expected input fields:\n")
		writeWorkflowFields(&b, step.InputFields)
		b.WriteString("\n")
	}
	if strings.TrimSpace(inst.InputArtifact) != "" {
		b.WriteString("Current step input artifact:\n")
		fmt.Fprintf(&b, "%s\n\n", indentWorkflowBlock(limitWorkflowText(inst.InputArtifact, 3000)))
	}
	if len(inst.InputValues) > 0 {
		b.WriteString("Current step structured inputs:\n")
		writeWorkflowValues(&b, inst.InputValues)
		b.WriteString("\n")
	}
	if len(step.OutputFields) > 0 {
		b.WriteString("Required output fields:\n")
		writeWorkflowFields(&b, step.OutputFields)
		b.WriteString("\n")
	}
	if previous := workflowPreviousOutputs(def.Steps, instances, step.ID); previous != "" {
		b.WriteString("Previous workflow outputs and review notes:\n")
		b.WriteString(previous)
		b.WriteString("\n")
	}
	b.WriteString("Instructions:\n")
	b.WriteString("- Treat the current step as the workflow contract for this task.\n")
	b.WriteString("- Finish workflow steps with structured outputs using `mga task step done --id ")
	b.WriteString(taskID)
	b.WriteString(" --status success --output <field>=<value>` for every required output field, or use `--output-json '{...}'`.\n")
	b.WriteString("- Do not put required workflow fields only in natural-language summary; the server validates output field names against the current step spec.\n")
	b.WriteString("- If this step needs human review or clarification, use `mga task confirm-request` instead of blocking silently.\n")
	b.WriteString("- To inspect the full workflow run, run `mga workflow current --task-id ")
	b.WriteString(taskID)
	b.WriteString("`.\n")
	b.WriteString("- To inspect the task record, run `mga task show ")
	b.WriteString(taskID)
	b.WriteString("`.\n")
	return b.String()
}

func workflowStepByID(steps []entity.WorkflowStep, id string) (entity.WorkflowStep, bool) {
	for _, step := range steps {
		if step.ID == id {
			return step, true
		}
	}
	return entity.WorkflowStep{}, false
}

func workflowStepInstanceByStepID(instances []entity.WorkflowStepInstance, stepID string) (entity.WorkflowStepInstance, bool) {
	for _, inst := range instances {
		if inst.StepID == stepID {
			return inst, true
		}
	}
	return entity.WorkflowStepInstance{}, false
}

func writeWorkflowFields(b *strings.Builder, fields []entity.WorkflowField) {
	for _, field := range fields {
		name := strings.TrimSpace(field.Name)
		if name == "" {
			continue
		}
		if desc := strings.TrimSpace(field.Description); desc != "" {
			fmt.Fprintf(b, "- `%s`: %s\n", name, desc)
		} else {
			fmt.Fprintf(b, "- `%s`\n", name)
		}
	}
}

func writeWorkflowValues(b *strings.Builder, values map[string]string) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := strings.TrimSpace(values[key])
		if value == "" {
			continue
		}
		fmt.Fprintf(b, "- `%s`: %s\n", key, limitWorkflowText(value, 1200))
	}
}

func workflowPreviousOutputs(steps []entity.WorkflowStep, instances []entity.WorkflowStepInstance, activeStepID string) string {
	titleByID := make(map[string]string, len(steps))
	for _, step := range steps {
		titleByID[step.ID] = step.Title
	}
	var b strings.Builder
	for _, inst := range instances {
		if inst.StepID == activeStepID {
			continue
		}
		summary := strings.TrimSpace(inst.Summary)
		output := strings.TrimSpace(inst.OutputArtifact)
		if summary == "" && output == "" && len(inst.OutputValues) == 0 {
			continue
		}
		title := titleByID[inst.StepID]
		if title == "" {
			title = inst.StepID
		}
		fmt.Fprintf(&b, "- %s (`%s`, status: `%s`):\n", title, inst.StepID, inst.Status)
		if summary != "" {
			fmt.Fprintf(&b, "  - Summary: %s\n", limitWorkflowText(summary, 1200))
		}
		if output != "" {
			fmt.Fprintf(&b, "  - Output: %s\n", limitWorkflowText(strings.ReplaceAll(output, "\n", " "), 1600))
		}
		if len(inst.OutputValues) > 0 {
			writeWorkflowValues(&b, inst.OutputValues)
		}
	}
	return b.String()
}

func limitWorkflowText(s string, limit int) string {
	s = strings.TrimSpace(s)
	if len(s) <= limit {
		return s
	}
	if limit <= 3 {
		return s[:limit]
	}
	return s[:limit-3] + "..."
}

func indentWorkflowBlock(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i, line := range lines {
		lines[i] = "> " + line
	}
	return strings.Join(lines, "\n")
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
	case strings.Contains(lower, "unable to find image") && (strings.Contains(lower, "denied") || strings.Contains(lower, "unauthorized")):
		return `[hint] 官方 Runtime 镜像无法匿名拉取。这通常表示发布包被错误设置为私有，而不是用户需要登录 GHCR。请确认：
1. Docker 已安装并运行：docker info
2. 本地是否已有镜像：docker image inspect multigent/runtime-base:latest
3. 没有本地镜像时，在项目根目录执行：docker build -t multigent/runtime-base:latest -f docker/runtime-base/Dockerfile .
4. 维护者应将 ghcr.io/multigent/multigent/runtime-base 包设为 Public；用户也可以在成员详情页 → 沙箱配置中指定可访问的自定义镜像`

	// Docker not installed or daemon not running
	case strings.Contains(lower, "docker") && (strings.Contains(lower, "not found") || strings.Contains(lower, "cannot connect") || strings.Contains(lower, "daemon")):
		return `[hint] Docker 未安装或未启动。请：
1. 安装 Docker：https://docs.docker.com/get-docker/
2. Windows/macOS：启动 Docker Desktop，并确认当前用户可以运行 docker info
3. Linux：启动 Docker daemon：sudo systemctl start docker
4. 再次运行前确认：docker info`

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

func writePromptMessageToLog(w io.Writer, prompt string) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return
	}
	raw, err := json.Marshal(map[string]string{
		"type":    "human",
		"content": prompt,
	})
	if err != nil {
		return
	}
	fmt.Fprintln(w, string(raw))
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

func (r *Runner) addRuntimeDockerSystemMounts(runtimeCfg *entity.SandboxConfig) {
	if runtimeCfg == nil || runtimeCfg.Docker == nil {
		return
	}
	// Development override only. Published images carry their own Linux mga;
	// mounting a native macOS/Windows binary would shadow it.
	if binMount := runtimecli.ResolveAvailableBinaryMount(r.root); binMount != "" {
		runtimeCfg.Docker.ExtraVolumes = append(runtimeCfg.Docker.ExtraVolumes, binMount)
	}
}

// ── HTTP agent task/exec methods ───────────────────────────────────────────────

// runTaskHTTP runs a task by posting the full prompt to the agent's HTTP
// endpoint. The agent's context.md is sent as the system message; the task
// prompt + system meta footer become the user message.
func (r *Runner) runTaskHTTP(project, agentName, agentDir string, meta *entity.AgentMeta, task *entity.Task) (*RunResult, error) {
	if meta.HTTPAgent == nil {
		return nil, fmt.Errorf("http-agent: no http_agent config in .multigent-agent.yaml (re-hire with --http-url)")
	}

	userPrompt := r.taskPromptWithWorkflowContext(project, agentName, task) + fmt.Sprintf(systemMetaFooter,
		task.ID, project, agentName, task.ID, task.ID, task.ID, task.ID)

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
		if provEnv, err := ps.ResolveEnvForModel(meta.Provider, meta.Model); err == nil {
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
	if err := writeRuntimeMCPClientConfigs(agentDir); err == nil {
		env[runtimeMCPConfigEnv] = "1"
	}
	secretResolver, closeResolver := newRuntimeConnectionSecretResolver()
	if closeResolver != nil {
		defer closeResolver()
	}
	toolDir, toolsPath, extraEnv, err := writeRuntimeToolsFile(r.root, agentDir, env["MULTIGENT_RUN_ID"], path, body, secretResolver)
	if err == nil && toolDir != "" && toolsPath != "" {
		env[runtimeToolDirEnv] = toolDir
		env[runtimeToolsFileEnv] = toolsPath
		env[runtimeToolBinDirEnv] = filepath.Join(toolDir, "bin")
		if cacheBin := workspaceToolCacheBinDir(r.root); cacheBin != "" {
			env[runtimeToolCacheBinDirEnv] = cacheBin
			env["PATH"] = filepath.Join(toolDir, "bin") + string(os.PathListSeparator) + cacheBin + string(os.PathListSeparator) + os.Getenv("PATH")
		} else {
			env["PATH"] = filepath.Join(toolDir, "bin") + string(os.PathListSeparator) + os.Getenv("PATH")
		}
		if bootstrap := filepath.Join(toolDir, "bootstrap-tools.sh"); fileExists(bootstrap) {
			env[runtimeToolBootstrapEnv] = bootstrap
		}
		for key, value := range extraEnv {
			if key != "" && value != "" {
				env[key] = value
			}
		}
	}
	return func() {
		_ = os.Remove(path)
		delete(env, runtimeToolCacheBinDirEnv)
		if toolDir != "" {
			_ = os.RemoveAll(toolDir)
		}
	}
}

func (r *Runner) materializeProviderCredentials(agentDir string, meta *entity.AgentMeta) error {
	if meta == nil || strings.TrimSpace(meta.Provider) == "" {
		return nil
	}
	ps := store.NewProviderStore(r.root)
	provider, err := ps.Get(meta.Provider)
	if err != nil {
		return err
	}
	method := store.ProviderAuthMethod(*provider)
	model := entity.NormaliseModel(meta.Model)
	switch {
	case method == store.ProviderAuthMethodCodexChatGPT && (model == entity.ModelCodex || model == entity.ModelQoder):
		src := filepath.Join(store.ProviderCredentialDir(r.root, provider.ID, entity.ModelCodex), ".codex", "auth.json")
		if !fileExists(src) {
			return fmt.Errorf("codex ChatGPT auth file is missing for provider %s", provider.ID)
		}
		dst := filepath.Join(agentDir, ".multigent", "runtime-home", string(model), ".codex", "auth.json")
		return copyRuntimeCredentialFile(src, dst)
	case method == store.ProviderAuthMethodClaudeBrowser && model == entity.ModelClaudeCode:
		srcRoot := store.ProviderCredentialDir(r.root, provider.ID, entity.ModelClaudeCode)
		for _, rel := range []string{".claude.json", filepath.Join(".claude", ".credentials.json")} {
			src := filepath.Join(srcRoot, rel)
			if fileExists(src) {
				dst := filepath.Join(agentDir, ".multigent", "runtime-home", string(model), rel)
				if err := copyRuntimeCredentialFile(src, dst); err != nil {
					return err
				}
			}
		}
		return nil
	case method == store.ProviderAuthMethodCursorBrowser && model == entity.ModelCursor:
		srcRoot := store.ProviderCredentialDir(r.root, provider.ID, entity.ModelCursor)
		hasCredential := false
		for _, rel := range []string{
			filepath.Join(".config", "cursor", "cli-config.json"),
			filepath.Join(".config", "cursor", "auth.json"),
			filepath.Join(".cursor", "cli-config.json"),
		} {
			src := filepath.Join(srcRoot, rel)
			if !fileExists(src) {
				continue
			}
			hasCredential = true
			dst := filepath.Join(agentDir, ".multigent", "runtime-home", string(model), rel)
			if err := copyRuntimeCredentialFile(src, dst); err != nil {
				return err
			}
		}
		if !hasCredential {
			return fmt.Errorf("Cursor auth file is missing for provider %s", provider.ID)
		}
		return nil
	default:
		return nil
	}
}

func copyRuntimeCredentialFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// materializeRuntimeConnectionsFile is kept as a narrow helper for tests and
// callers that only need the raw connection manifest. Agent runs should use
// materializeRuntimeFiles so the tool adapter plan is available too.
func (r *Runner) materializeRuntimeConnectionsFile(agentDir string, env map[string]string) func() {
	if cleanup := r.materializeRuntimeFiles(agentDir, env); cleanup != nil {
		toolsPath := env[runtimeToolsFileEnv]
		toolDir := env[runtimeToolDirEnv]
		toolSkillsPath := env[runtimeToolSkillsFileEnv]
		delete(env, runtimeToolsFileEnv)
		delete(env, runtimeToolDirEnv)
		delete(env, runtimeToolSkillsFileEnv)
		return func() {
			if toolsPath != "" {
				_ = os.Remove(toolsPath)
			}
			if toolSkillsPath != "" {
				_ = os.Remove(toolSkillsPath)
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

func writeRuntimeMCPClientConfigs(agentDir string) error {
	if strings.TrimSpace(agentDir) == "" {
		return fmt.Errorf("agent dir is required")
	}
	projectMCPPath := filepath.Join(agentDir, ".mcp.json")
	if err := mergeMCPJSONConfig(projectMCPPath); err != nil {
		return err
	}
	cursorProjectPath := filepath.Join(agentDir, ".cursor", "mcp.json")
	if err := mergeMCPJSONConfig(cursorProjectPath); err != nil {
		return err
	}
	for _, model := range []entity.AgentModel{entity.ModelCodex, entity.ModelQoder} {
		path := filepath.Join(agentDir, ".multigent", "runtime-home", string(model), ".codex", "config.toml")
		if err := mergeCodexMCPConfig(path); err != nil {
			return err
		}
	}
	cursorHomePath := filepath.Join(agentDir, ".multigent", "runtime-home", string(entity.ModelCursor), ".cursor", "mcp.json")
	if err := mergeMCPJSONConfig(cursorHomePath); err != nil {
		return err
	}
	return nil
}

func mergeMCPJSONConfig(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	cfg := map[string]any{}
	if body, err := os.ReadFile(path); err == nil && len(bytes.TrimSpace(body)) > 0 {
		if err := json.Unmarshal(body, &cfg); err != nil {
			return fmt.Errorf("decode MCP config %s: %w", path, err)
		}
	}
	servers, _ := cfg["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	servers["multigent"] = map[string]any{
		"command": runtimecli.BinaryName,
		"args":    []string{"runtime", "mcp-server"},
	}
	cfg["mcpServers"] = servers
	body, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(body, '\n'), 0o600)
}

func mergeCodexMCPConfig(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	body, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	updated := replaceManagedBlock(string(body), "# BEGIN MULTIGENT MCP", "# END MULTIGENT MCP", codexMCPConfigBlock())
	return os.WriteFile(path, []byte(updated), 0o600)
}

func codexMCPConfigBlock() string {
	return `# BEGIN MULTIGENT MCP
[mcp_servers.multigent]
enabled = true
command = "mga"
args = ["runtime", "mcp-server"]
env_vars = ["MULTIGENT_API_URL", "MULTIGENT_AGENT_TOKEN"]
# END MULTIGENT MCP
`
}

func replaceManagedBlock(content, begin, end, block string) string {
	content = strings.TrimRight(content, "\n")
	start := strings.Index(content, begin)
	if start >= 0 {
		stopRel := strings.Index(content[start:], end)
		if stopRel >= 0 {
			stop := start + stopRel + len(end)
			prefix := strings.TrimRight(content[:start], "\n")
			suffix := strings.TrimLeft(content[stop:], "\n")
			parts := []string{}
			if prefix != "" {
				parts = append(parts, prefix)
			}
			parts = append(parts, strings.TrimRight(block, "\n"))
			if suffix != "" {
				parts = append(parts, suffix)
			}
			return strings.Join(parts, "\n\n") + "\n"
		}
	}
	if content == "" {
		return strings.TrimRight(block, "\n") + "\n"
	}
	return content + "\n\n" + strings.TrimRight(block, "\n") + "\n"
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
	Installer   *runtimeInstallerRef   `json:"installer,omitempty"`
	ConfigFiles []runtimeConfigFileRef `json:"configFiles,omitempty"`
}

type runtimeInstallerRef struct {
	Type    string   `json:"type,omitempty"`
	Package string   `json:"package,omitempty"`
	Version string   `json:"version,omitempty"`
	Command []string `json:"command,omitempty"`
	Check   []string `json:"check,omitempty"`
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

func writeRuntimeToolsFile(workspaceRoot, agentDir, runID, connectionsPath string, body []byte, secretResolver runtimeConnectionSecretResolver) (string, string, map[string]string, error) {
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
	cliAuditPath := filepath.Join(toolDir, "cli-audit.jsonl")
	extraEnv[runtimeToolCLIAuditEnv] = cliAuditPath
	var bootstrapSteps []string
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
			bootstrapSteps = append(bootstrapSteps, runtimeCLIInstallerScript(workspaceRoot, manifest.Tools[ti], *adapter)...)
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
			if err := materializeCLIWrapper(toolDir, manifest.Tools[ti], *adapter); err != nil {
				return "", "", nil, err
			}
		}
	}
	if len(bootstrapSteps) > 0 {
		bootstrapPath := filepath.Join(toolDir, "bootstrap-tools.sh")
		bootstrapBody := strings.Join(append([]string{"#!/bin/sh", "set -eu"}, bootstrapSteps...), "\n") + "\n"
		if err := os.WriteFile(bootstrapPath, []byte(bootstrapBody), 0o700); err != nil {
			return "", "", nil, err
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
	guide := runtimeguide.Render(runtimeguide.Plan{Tools: runtimeGuideTools(manifest.Tools)})
	guidePath := filepath.Join(toolDir, "tool-skills.md")
	if err := os.WriteFile(guidePath, []byte(guide), 0o644); err != nil {
		return "", "", nil, err
	}
	extraEnv[runtimeToolSkillsFileEnv] = guidePath
	return toolDir, planPath, extraEnv, nil
}

func runtimeGuideTools(tools []runtimeToolRef) []runtimeguide.Tool {
	out := make([]runtimeguide.Tool, 0, len(tools))
	for _, tool := range tools {
		adapters := make([]runtimeguide.Adapter, 0, len(tool.Adapters))
		for _, adapter := range tool.Adapters {
			item := runtimeguide.Adapter{
				Type:        adapter.Type,
				Priority:    adapter.Priority,
				Description: adapter.Description,
				Skills:      append([]string(nil), adapter.Skills...),
			}
			if adapter.CLI != nil {
				item.CLI = &runtimeguide.CLI{Binary: adapter.CLI.Binary}
			}
			if adapter.HTTPAction != nil {
				item.HTTPAction = &runtimeguide.HTTP{ActionNames: append([]string(nil), adapter.HTTPAction.ActionNames...)}
			}
			adapters = append(adapters, item)
		}
		actions := make([]runtimeguide.Action, 0, len(tool.Actions))
		for _, action := range tool.Actions {
			actions = append(actions, runtimeguide.Action{
				Name:        action.Name,
				DisplayName: action.DisplayName,
				Method:      action.Method,
				Endpoint:    action.Endpoint,
			})
		}
		out = append(out, runtimeguide.Tool{
			Provider:           tool.Provider,
			DisplayName:        tool.DisplayName,
			ConnectionAlias:    tool.ConnectionAlias,
			ConnectionName:     tool.ConnectionName,
			RecommendedAdapter: tool.RecommendedAdapter,
			Adapters:           adapters,
			Skills:             append([]string(nil), tool.Skills...),
			Actions:            actions,
		})
	}
	return out
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
	case "ssh_key":
		return materializeSSHKeyConfig(cfg, secretValues, "id_multigent", "MULTIGENT_SSH_KEY_FILE")
	case "git_ssh":
		return materializeGitSSHConfig(cfg, secretValues)
	case "npm_registry":
		return materializeNPMRegistryConfig(cfg, secretValues)
	case "docker_registry":
		return materializeDockerRegistryConfig(cfg, secretValues)
	case "aws":
		return materializeAWSCLIConfig(cfg, secretValues)
	case "gcloud":
		return materializeGCloudCLIConfig(cfg, secretValues)
	case "cloudflare":
		return materializeCloudflareCLIConfig(cfg, secretValues)
	default:
		return nil, nil
	}
}

func runtimeCLIInstallerScript(workspaceRoot string, tool runtimeToolRef, adapter runtimeAdapterRef) []string {
	if adapter.CLI == nil {
		return nil
	}
	binary := strings.TrimSpace(adapter.CLI.Binary)
	installer := adapter.CLI.Installer
	if installer == nil {
		if binary == "" {
			return nil
		}
		return []string{fmt.Sprintf("command -v %s >/dev/null 2>&1 || { echo %s >&2; exit 127; }", shellQuote(binary), shellQuote("Multigent runtime tool missing: "+binary))}
	}
	label := firstNonEmpty(tool.Provider, tool.ConnectionAlias, binary)
	var lines []string
	lines = append(lines, fmt.Sprintf("echo %s >&2", shellQuote("multigent: preparing runtime tool "+label)))
	npmPrefix := agentcli.ToolchainHome + "/npm"
	markerDir := agentcli.ToolchainHome + "/markers"
	if cacheRoot := workspaceToolCacheRoot(workspaceRoot); cacheRoot != "" {
		npmPrefix = filepath.Join(cacheRoot, "npm")
		markerDir = filepath.Join(cacheRoot, "markers")
	}
	lines = append(lines,
		"export MULTIGENT_TOOLCHAIN_HOME="+shellQuote(agentcli.ToolchainHome),
		"export NPM_CONFIG_PREFIX="+shellQuote(npmPrefix),
		"export PATH=\"$NPM_CONFIG_PREFIX/bin:$PATH\"",
		"mkdir -p \"$NPM_CONFIG_PREFIX\" "+shellQuote(markerDir),
	)
	switch strings.TrimSpace(installer.Type) {
	case "npm":
		pkg := firstNonEmpty(installer.Package, binary)
		if pkg == "" {
			break
		}
		version := strings.TrimSpace(installer.Version)
		installPkg := pkg
		if version != "" && version != "latest" {
			installPkg += "@" + version
		}
		marker := runtimeToolInstallerMarker(markerDir, *installer, binary)
		lines = append(lines,
			fmt.Sprintf("if [ ! -f %s ] || [ \"${MULTIGENT_TOOL_FORCE_INSTALL:-}\" = \"1\" ]; then", shellQuote(marker)),
			fmt.Sprintf("  npm install -g %s", shellQuote(installPkg)),
			fmt.Sprintf("  touch %s", shellQuote(marker)),
			"fi",
		)
	case "script":
		lines = append(lines, installer.Command...)
	case "system", "":
		if installer.Package != "" && binary != "" {
			lines = append(lines, fmt.Sprintf("command -v %s >/dev/null 2>&1 || { echo %s >&2; exit 127; }", shellQuote(binary), shellQuote("Multigent runtime tool package is not installed in sandbox image: "+installer.Package)))
		}
	default:
		if binary != "" {
			lines = append(lines, fmt.Sprintf("command -v %s >/dev/null 2>&1 || { echo %s >&2; exit 127; }", shellQuote(binary), shellQuote("Unsupported Multigent runtime tool installer type: "+installer.Type)))
		}
	}
	if binary != "" {
		lines = append(lines, fmt.Sprintf("command -v %s >/dev/null 2>&1", shellQuote(binary)))
	}
	for _, check := range installer.Check {
		if strings.TrimSpace(check) != "" {
			lines = append(lines, check)
		}
	}
	return lines
}

func materializeCLIWrapper(toolDir string, tool runtimeToolRef, adapter runtimeAdapterRef) error {
	if adapter.CLI == nil {
		return nil
	}
	binary := strings.TrimSpace(adapter.CLI.Binary)
	if binary == "" {
		return nil
	}
	wrapperPath := filepath.Join(toolDir, "bin", binary)
	if fileExists(wrapperPath) {
		return nil
	}
	return os.WriteFile(wrapperPath, []byte(runtimeCLIWrapperScript(binary, tool, nil)), 0o700)
}

func runtimeToolInstallerMarker(markerDir string, installer runtimeInstallerRef, binary string) string {
	key := strings.Join([]string{
		strings.TrimSpace(installer.Type),
		strings.TrimSpace(installer.Package),
		strings.TrimSpace(installer.Version),
		strings.TrimSpace(binary),
	}, "\x00")
	sum := sha256.Sum256([]byte(key))
	if markerDir == "" {
		markerDir = agentcli.ToolchainHome + "/markers"
	}
	return filepath.Join(markerDir, "tool-"+hex.EncodeToString(sum[:8]))
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

func materializeSSHKeyConfig(cfg runtimeConfigFileRef, secretValues map[string]string, keyBaseName, envKey string) (map[string]string, error) {
	path := strings.TrimSpace(cfg.Path)
	if cfg.MaterializedPath == "" {
		return nil, nil
	}
	if strings.HasSuffix(path, "known_hosts") {
		knownHosts := strings.TrimSpace(secretValues["knownHosts"])
		if knownHosts == "" {
			return nil, nil
		}
		if err := os.WriteFile(cfg.MaterializedPath, []byte(knownHosts+"\n"), 0o600); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if !strings.HasSuffix(path, keyBaseName) {
		return nil, nil
	}
	privateKey := normalizePrivateKey(secretValues["privateKey"])
	if privateKey == "" {
		return nil, nil
	}
	if err := os.WriteFile(cfg.MaterializedPath, []byte(privateKey), 0o600); err != nil {
		return nil, err
	}
	if envKey == "" {
		return nil, nil
	}
	return map[string]string{envKey: cfg.MaterializedPath}, nil
}

func materializeGitSSHConfig(cfg runtimeConfigFileRef, secretValues map[string]string) (map[string]string, error) {
	path := strings.TrimSpace(cfg.Path)
	if strings.HasSuffix(path, "known_hosts") {
		return materializeSSHKeyConfig(cfg, secretValues, "", "")
	}
	if strings.HasSuffix(path, ".gitconfig") {
		return materializeGitConfig(cfg, secretValues)
	}
	if !strings.HasSuffix(path, "id_git_multigent") {
		return nil, nil
	}
	env, err := materializeSSHKeyConfig(cfg, secretValues, "id_git_multigent", "MULTIGENT_GIT_SSH_KEY_FILE")
	if err != nil || len(env) == 0 {
		return env, err
	}
	knownHostsPath := filepath.Join(filepath.Dir(cfg.MaterializedPath), "known_hosts")
	command := []string{
		"ssh",
		"-i", cfg.MaterializedPath,
		"-o", "IdentitiesOnly=yes",
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "UserKnownHostsFile=" + knownHostsPath,
	}
	if proxyJump := strings.TrimSpace(secretValues["proxyJump"]); proxyJump != "" {
		command = append(command, "-o", "ProxyCommand="+shellQuote(gitSSHProxyCommand(cfg.MaterializedPath, knownHostsPath, proxyJump)))
	}
	env["GIT_SSH_COMMAND"] = strings.Join(command, " ")
	return env, nil
}

func gitSSHProxyCommand(keyPath, knownHostsPath, proxyJump string) string {
	args := []string{
		"ssh",
		"-i", keyPath,
		"-o", "IdentitiesOnly=yes",
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "UserKnownHostsFile=" + knownHostsPath,
		"-W", "%h:%p",
		proxyJump,
	}
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

func materializeGitConfig(cfg runtimeConfigFileRef, secretValues map[string]string) (map[string]string, error) {
	if cfg.MaterializedPath == "" {
		return nil, nil
	}
	userName := strings.TrimSpace(firstNonEmpty(secretValues["gitUserName"], secretValues["userName"]))
	userEmail := strings.TrimSpace(firstNonEmpty(secretValues["gitUserEmail"], secretValues["userEmail"], secretValues["email"]))
	if userName == "" && userEmail == "" {
		return nil, nil
	}
	lines := []string{"[user]"}
	if userName != "" {
		lines = append(lines, "\tname = "+userName)
	}
	if userEmail != "" {
		lines = append(lines, "\temail = "+userEmail)
	}
	if err := os.WriteFile(cfg.MaterializedPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		return nil, err
	}
	return map[string]string{"GIT_CONFIG_GLOBAL": cfg.MaterializedPath}, nil
}

func materializeNPMRegistryConfig(cfg runtimeConfigFileRef, secretValues map[string]string) (map[string]string, error) {
	if cfg.MaterializedPath == "" || !strings.HasSuffix(strings.TrimSpace(cfg.Path), ".npmrc") {
		return nil, nil
	}
	registryURL := firstNonEmpty(secretValues["registryUrl"], "https://registry.npmjs.org/")
	authToken := firstNonEmpty(secretValues["authToken"], secretValues["apiKey"], secretValues["token"])
	if authToken == "" {
		return nil, nil
	}
	registryURL = strings.TrimRight(registryURL, "/") + "/"
	authKey := npmRegistryAuthKey(registryURL)
	var lines []string
	if scope := strings.TrimSpace(secretValues["scope"]); scope != "" {
		if !strings.HasPrefix(scope, "@") {
			scope = "@" + scope
		}
		lines = append(lines, scope+":registry="+registryURL)
	} else {
		lines = append(lines, "registry="+registryURL)
	}
	lines = append(lines, authKey+":_authToken="+authToken)
	if value := strings.TrimSpace(secretValues["alwaysAuth"]); value != "" {
		lines = append(lines, "always-auth="+value)
	}
	if err := os.WriteFile(cfg.MaterializedPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		return nil, err
	}
	return map[string]string{"NPM_CONFIG_USERCONFIG": cfg.MaterializedPath}, nil
}

func materializeDockerRegistryConfig(cfg runtimeConfigFileRef, secretValues map[string]string) (map[string]string, error) {
	if cfg.MaterializedPath == "" || !strings.HasSuffix(strings.TrimSpace(cfg.Path), ".docker/config.json") {
		return nil, nil
	}
	registry := firstNonEmpty(secretValues["registryUrl"], "https://index.docker.io/v1/")
	username := firstNonEmpty(secretValues["username"], "token")
	password := firstNonEmpty(secretValues["password"], secretValues["authToken"], secretValues["apiKey"], secretValues["token"])
	if password == "" {
		return nil, nil
	}
	auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	body, err := json.MarshalIndent(map[string]any{
		"auths": map[string]any{
			dockerRegistryConfigKey(registry): map[string]any{
				"auth": auth,
			},
		},
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(cfg.MaterializedPath, append(body, '\n'), 0o600); err != nil {
		return nil, err
	}
	return map[string]string{"DOCKER_CONFIG": filepath.Dir(cfg.MaterializedPath)}, nil
}

func materializeAWSCLIConfig(cfg runtimeConfigFileRef, secretValues map[string]string) (map[string]string, error) {
	path := strings.TrimSpace(cfg.Path)
	if cfg.MaterializedPath == "" {
		return nil, nil
	}
	profile := firstNonEmpty(secretValues["profile"], "default")
	region := strings.TrimSpace(secretValues["region"])
	env := map[string]string{"AWS_PROFILE": profile}
	if strings.HasSuffix(path, ".aws/credentials") {
		accessKeyID := strings.TrimSpace(secretValues["accessKeyId"])
		secretAccessKey := strings.TrimSpace(secretValues["secretAccessKey"])
		if accessKeyID == "" || secretAccessKey == "" {
			return nil, nil
		}
		lines := []string{
			"[" + profile + "]",
			"aws_access_key_id = " + accessKeyID,
			"aws_secret_access_key = " + secretAccessKey,
		}
		if sessionToken := strings.TrimSpace(secretValues["sessionToken"]); sessionToken != "" {
			lines = append(lines, "aws_session_token = "+sessionToken)
		}
		if err := os.WriteFile(cfg.MaterializedPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
			return nil, err
		}
		env["AWS_SHARED_CREDENTIALS_FILE"] = cfg.MaterializedPath
		return env, nil
	}
	if strings.HasSuffix(path, ".aws/config") {
		section := "profile " + profile
		if profile == "default" {
			section = "default"
		}
		lines := []string{"[" + section + "]"}
		if region != "" {
			lines = append(lines, "region = "+region)
			env["AWS_REGION"] = region
			env["AWS_DEFAULT_REGION"] = region
		}
		if err := os.WriteFile(cfg.MaterializedPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
			return nil, err
		}
		env["AWS_CONFIG_FILE"] = cfg.MaterializedPath
		return env, nil
	}
	return nil, nil
}

func materializeGCloudCLIConfig(cfg runtimeConfigFileRef, secretValues map[string]string) (map[string]string, error) {
	if cfg.MaterializedPath == "" || !strings.HasSuffix(strings.TrimSpace(cfg.Path), "application_default_credentials.json") {
		return nil, nil
	}
	serviceAccountJSON := strings.TrimSpace(secretValues["serviceAccountJson"])
	projectID := strings.TrimSpace(secretValues["projectId"])
	if serviceAccountJSON == "" {
		return nil, nil
	}
	if !json.Valid([]byte(serviceAccountJSON)) {
		return nil, fmt.Errorf("serviceAccountJson must be valid JSON")
	}
	if err := os.WriteFile(cfg.MaterializedPath, []byte(serviceAccountJSON+"\n"), 0o600); err != nil {
		return nil, err
	}
	env := map[string]string{
		"GOOGLE_APPLICATION_CREDENTIALS":         cfg.MaterializedPath,
		"CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE": cfg.MaterializedPath,
	}
	if projectID != "" {
		env["GOOGLE_CLOUD_PROJECT"] = projectID
		env["CLOUDSDK_CORE_PROJECT"] = projectID
	}
	if region := strings.TrimSpace(secretValues["region"]); region != "" {
		env["CLOUDSDK_COMPUTE_REGION"] = region
	}
	if zone := strings.TrimSpace(secretValues["zone"]); zone != "" {
		env["CLOUDSDK_COMPUTE_ZONE"] = zone
	}
	return env, nil
}

func materializeCloudflareCLIConfig(cfg runtimeConfigFileRef, secretValues map[string]string) (map[string]string, error) {
	if cfg.MaterializedPath == "" || !strings.HasSuffix(strings.TrimSpace(cfg.Path), ".cloudflare/env") {
		return nil, nil
	}
	apiToken := strings.TrimSpace(firstNonEmpty(secretValues["apiKey"], secretValues["apiToken"], secretValues["token"]))
	if apiToken == "" {
		return nil, nil
	}
	env := map[string]string{"CLOUDFLARE_API_TOKEN": apiToken}
	lines := []string{"CLOUDFLARE_API_TOKEN=" + shellQuote(apiToken)}
	if accountID := strings.TrimSpace(secretValues["accountId"]); accountID != "" {
		env["CLOUDFLARE_ACCOUNT_ID"] = accountID
		lines = append(lines, "CLOUDFLARE_ACCOUNT_ID="+shellQuote(accountID))
	}
	if zoneID := strings.TrimSpace(secretValues["zoneId"]); zoneID != "" {
		env["CLOUDFLARE_ZONE_ID"] = zoneID
		lines = append(lines, "CLOUDFLARE_ZONE_ID="+shellQuote(zoneID))
	}
	if err := os.WriteFile(cfg.MaterializedPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		return nil, err
	}
	return env, nil
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
	if err := os.WriteFile(wrapperPath, []byte(runtimeCLIWrapperScript("lark-cli", tool, map[string]string{"HOME": larkHome})), 0o700); err != nil {
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

func runtimeCLIWrapperScript(binary string, tool runtimeToolRef, env map[string]string) string {
	var exports []string
	for key, value := range env {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		exports = append(exports, "export "+key+"="+shellQuote(value))
	}
	if len(exports) == 0 {
		exports = append(exports, ":")
	}
	return `#!/bin/sh
set -u
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
` + strings.Join(exports, "\n") + `
start_sec=$(date +%s 2>/dev/null || printf 0)
status=0
` + shellQuote(binary) + ` "$@" || status=$?
end_sec=$(date +%s 2>/dev/null || printf "$start_sec")
duration_ms=$(( (end_sec - start_sec) * 1000 ))
if [ -n "${MULTIGENT_TOOL_CLI_AUDIT_FILE:-}" ]; then
  audit_dir=$(dirname -- "$MULTIGENT_TOOL_CLI_AUDIT_FILE")
  mkdir -p "$audit_dir" 2>/dev/null || true
  subcommand=${1:-}
  safe_subcommand=$(printf '%s' "$subcommand" | sed 's/\\/\\\\/g; s/"/\\"/g')
  argc=$#
  printf '{"ts":"%s","provider":"%s","connection":"%s","binary":"%s","subcommand":"%s","argc":%s,"exitCode":%s,"durationMs":%s}\n' \
    "$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || printf '')" \
    ` + shellQuote(tool.Provider) + ` \
    ` + shellQuote(tool.ConnectionAlias) + ` \
    ` + shellQuote(binary) + ` \
    "$safe_subcommand" \
    "$argc" \
    "$status" \
    "$duration_ms" >> "$MULTIGENT_TOOL_CLI_AUDIT_FILE" 2>/dev/null || true
fi
exit "$status"
`
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func workspaceToolCacheRoot(workspaceRoot string) string {
	root := strings.TrimSpace(workspaceRoot)
	if root == "" {
		return ""
	}
	return filepath.Join(root, ".multigent", "tool-cache")
}

func workspaceToolCacheBinDir(workspaceRoot string) string {
	root := workspaceToolCacheRoot(workspaceRoot)
	if root == "" {
		return ""
	}
	return filepath.Join(root, "npm", "bin")
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

func normalizePrivateKey(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\r\n", "\n"))
	if value == "" {
		return ""
	}
	return value + "\n"
}

func npmRegistryAuthKey(registryURL string) string {
	raw := strings.TrimSpace(registryURL)
	parsed, err := url.Parse(raw)
	if err == nil && parsed.Host != "" {
		path := strings.TrimRight(parsed.EscapedPath(), "/")
		if path != "" {
			return "//" + parsed.Host + path + "/"
		}
		return "//" + parsed.Host + "/"
	}
	raw = strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
	raw = strings.TrimRight(raw, "/")
	return "//" + raw + "/"
}

func dockerRegistryConfigKey(registry string) string {
	raw := strings.TrimSpace(registry)
	if raw == "" {
		return "https://index.docker.io/v1/"
	}
	if parsed, err := url.Parse(raw); err == nil && parsed.Host != "" {
		if parsed.Host == "registry-1.docker.io" || parsed.Host == "docker.io" {
			return "https://index.docker.io/v1/"
		}
		return parsed.Host
	}
	return strings.TrimRight(raw, "/")
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
	if err == nil && meta != nil && strings.TrimSpace(meta.Addr) != "" {
		return normalizeRuntimeAPIURL(meta.Addr)
	}
	// A daemon serves all workspaces beneath its data directory, while legacy
	// web runtime metadata is keyed only by the workspace active at startup.
	// Fall back to daemon metadata so switching workspaces does not silently
	// drop MULTIGENT_API_URL and MULTIGENT_AGENT_TOKEN from agent runs.
	daemonMeta, err := daemon.LoadMeta()
	if err == nil && daemonMeta != nil && strings.TrimSpace(daemonMeta.Addr) != "" {
		return normalizeRuntimeAPIURL(daemonMeta.Addr)
	}
	return ""
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

func runtimeControlEnvForProvider(env map[string]string, provider entity.SandboxProvider) map[string]string {
	if len(env) == 0 {
		return env
	}
	if provider != entity.SandboxDocker {
		return env
	}
	out := make(map[string]string, len(env))
	for k, v := range env {
		out[k] = v
	}
	if apiURL := dockerReachableRuntimeAPIURL(env["MULTIGENT_API_URL"]); apiURL != "" {
		out["MULTIGENT_API_URL"] = apiURL
	}
	return out
}

func dockerReachableRuntimeAPIURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	host := u.Hostname()
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return strings.TrimRight(raw, "/")
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	u.Host = net.JoinHostPort("host.docker.internal", port)
	return strings.TrimRight(u.String(), "/")
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
		if k == runtimeToolBinDirEnv || k == runtimeToolCacheBinDirEnv || k == runtimeToolBootstrapEnv || k == runtimeToolSkillsFileEnv || k == runtimeToolCLIAuditEnv || k == "PATH" {
			cfg.Env = append(cfg.Env, entity.RuntimeEnvVar{Name: k, Value: v})
			continue
		}
		cfg.Env = append(cfg.Env, entity.RuntimeEnvVar{Name: k, Inherit: true})
	}
}

func isRuntimeControlEnvKey(key string) bool {
	switch key {
	case "MULTIGENT_API_URL", "MULTIGENT_AGENT_TOKEN", "MULTIGENT_RUN_ID", "MULTIGENT_WORKSPACE_ID", runtimeConnectionsFileEnv, runtimeToolsFileEnv, runtimeToolDirEnv, runtimeToolBinDirEnv, runtimeToolCacheBinDirEnv, runtimeToolBootstrapEnv, runtimeToolSkillsFileEnv, runtimeToolCLIAuditEnv:
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

func ensureHostDockerPATH(env []string) []string {
	const dockerAppBin = "/Applications/Docker.app/Contents/Resources/bin"
	prefixes := []string{dockerAppBin, "/usr/local/bin", "/opt/homebrew/bin"}
	current := ""
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if ok && key == "PATH" {
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
	return mergeEnv(env, map[string]string{"PATH": strings.Join(dedupeEnvPath(parts), string(os.PathListSeparator))})
}

func dedupeEnvPath(parts []string) []string {
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
