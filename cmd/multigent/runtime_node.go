package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/runner"
	"github.com/multigent/multigent/internal/runtimeexec"
	"github.com/multigent/multigent/internal/sandbox"
	"github.com/multigent/multigent/internal/store"
	"github.com/multigent/multigent/internal/taskstore"
	"github.com/spf13/cobra"
)

const runtimeNodeConfigEnv = "MULTIGENT_RUNTIME_NODE_CONFIG"

var errRuntimeRunCancelled = errors.New("runtime run cancelled")

type runtimeNodeConfig struct {
	ServerURL     string `json:"serverUrl"`
	Token         string `json:"token"`
	RuntimeNodeID string `json:"runtimeNodeId,omitempty"`
	JoinedAt      string `json:"joinedAt,omitempty"`
}

type runtimeNodeRunEnvelope struct {
	Run          *runtimeNodeRun `json:"run"`
	RetryAfterMS int             `json:"retryAfterMs"`
}

type runtimeNodeRun struct {
	ID                  string `json:"id"`
	WorkspaceID         string `json:"workspaceId"`
	AgentWorkerID       string `json:"agentWorkerId"`
	ProjectMembershipID string `json:"projectMembershipId"`
	ProjectID           string `json:"projectId"`
	AgentID             string `json:"agentId"`
	TaskID              string `json:"taskId"`
	RuntimeNodeID       string `json:"runtimeNodeId"`
	Status              string `json:"status"`
}

type runtimeNodeSpecEnvelope struct {
	Run  *runtimeNodeRun  `json:"run"`
	Spec runtimeexec.Spec `json:"spec"`
}

type runtimeNodeLeaseEnvelope struct {
	Run       *runtimeNodeRun `json:"run"`
	Cancelled bool            `json:"cancelled"`
}

type runtimeNodeCoordinator struct {
	mu         sync.Mutex
	busyAgents map[string]struct{}
}

func newRuntimeNodeCoordinator() *runtimeNodeCoordinator {
	return &runtimeNodeCoordinator{busyAgents: map[string]struct{}{}}
}

func (c *runtimeNodeCoordinator) busyAgentListLocked() []string {
	out := make([]string, 0, len(c.busyAgents))
	for agent := range c.busyAgents {
		out = append(out, agent)
	}
	return out
}

func newRuntimeJoinCmd() *cobra.Command {
	var serverURL string
	var token string
	var skipPrepare bool
	cmd := &cobra.Command{
		Use:   "join",
		Short: "Join this machine as a Multigent Runtime Node",
		RunE: func(_ *cobra.Command, _ []string) error {
			serverURL = strings.TrimRight(strings.TrimSpace(serverURL), "/")
			token = strings.TrimSpace(token)
			if serverURL == "" || token == "" {
				return fmt.Errorf("--server and --token are required")
			}
			cfg := runtimeNodeConfig{
				ServerURL: serverURL,
				Token:     token,
				JoinedAt:  time.Now().UTC().Format(time.RFC3339),
			}
			if err := cfg.save(); err != nil {
				return err
			}
			if err := runtimeNodeRegister(cfg); err != nil {
				return err
			}
			if !skipPrepare {
				if err := runtimeNodePrepareLocal(); err != nil {
					_ = runtimeNodeHeartbeat(cfg, "online", "prepare warning: "+err.Error())
					fmt.Fprintf(os.Stderr, "runtime prepare warning: %v\n", err)
					fmt.Println("You can retry later with: multigent runtime prepare")
				}
			}
			fmt.Printf("Runtime node joined: %s\n", serverURL)
			fmt.Println("Next: multigent runtime doctor")
			fmt.Println("Then: multigent runtime start")
			return nil
		},
	}
	cmd.Flags().StringVar(&serverURL, "server", "", "Multigent server URL")
	cmd.Flags().StringVar(&token, "token", "", "runtime node join token")
	cmd.Flags().BoolVar(&skipPrepare, "skip-prepare", false, "skip Docker image pull and smoke test during join")
	return cmd
}

func newRuntimeStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show local Runtime Node configuration",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := loadRuntimeNodeConfig()
			if err != nil {
				return err
			}
			out := map[string]any{
				"serverUrl": cfg.ServerURL,
				"joinedAt":  cfg.JoinedAt,
				"config":    runtimeNodeConfigPath(),
				"tokenSet":  cfg.Token != "",
			}
			return printJSON(out)
		},
	}
	return cmd
}

func newRuntimeDoctorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check whether this machine can run agent sandboxes",
		RunE: func(_ *cobra.Command, _ []string) error {
			caps := detectRuntimeNodeCapabilities()
			body, _ := json.MarshalIndent(caps, "", "  ")
			fmt.Println(string(body))
			if docker, _ := caps["docker"].(map[string]any); docker != nil {
				if ok, _ := docker["available"].(bool); !ok {
					return fmt.Errorf("Docker sandbox is not ready: %v", docker["reason"])
				}
			}
			return nil
		},
	}
	return cmd
}

func newRuntimePrepareCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prepare",
		Short: "Prepare this Runtime Node for sandboxed agent execution",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runtimeNodePrepareLocal()
		},
	}
	return cmd
}

func newRuntimeStartCmd() *cobra.Command {
	var once bool
	var pollInterval time.Duration
	var concurrency int
	var daemonMode bool
	var streamAgentOutput bool
	var logFile string
	var logLevel string
	var logFormat string
	var logMaxSizeMB int
	var logStderr bool
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Runtime Node loop",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadRuntimeNodeConfig()
			if err != nil {
				return err
			}
			opts := resolveRuntimeNodeLogOptions(logFile, logLevel, logFormat, logMaxSizeMB, logStderr, cmd.Flags().Changed)
			if daemonMode && os.Getenv("MULTIGENT_RUNTIME_NODE_DAEMON_CHILD") == "" {
				return startRuntimeNodeDaemon(opts.File)
			}
			logCloser, err := initServiceLogger(opts, "runtime-node")
			if err != nil {
				return fmt.Errorf("init runtime node logger: %w", err)
			}
			defer logCloser()
			if err := runtimeNodeRegister(cfg); err != nil {
				return err
			}
			if concurrency <= 0 {
				concurrency = 1
			}
			slog.Info("runtime node connected", "server", cfg.ServerURL, "concurrency", concurrency, "once", once)
			coord := newRuntimeNodeCoordinator()
			if once {
				return runtimeNodeLoopOnce(cfg, 1, coord, streamAgentOutput)
			}
			return runtimeNodeRunWorkers(cfg, concurrency, pollInterval, coord, streamAgentOutput)
		},
	}
	cmd.Flags().BoolVar(&once, "once", false, "run one heartbeat/claim cycle and exit")
	cmd.Flags().DurationVar(&pollInterval, "poll-interval", 3*time.Second, "run claim polling interval")
	cmd.Flags().IntVar(&concurrency, "concurrency", 1, "maximum concurrent agent runs claimed by this runtime node")
	cmd.Flags().BoolVar(&daemonMode, "daemon", false, "start in the background and write logs to --log-file")
	cmd.Flags().BoolVar(&streamAgentOutput, "stream-agent-output", false, "stream raw agent stdout/stderr to the runtime node output for debugging")
	cmd.Flags().StringVar(&logFile, "log-file", "", "runtime node log file path")
	cmd.Flags().StringVar(&logLevel, "log-level", "", "log level: debug|info|warn|error")
	cmd.Flags().StringVar(&logFormat, "log-format", "", "log format: json|text")
	cmd.Flags().IntVar(&logMaxSizeMB, "log-max-size", 0, "max log file size in MB")
	cmd.Flags().BoolVar(&logStderr, "log-stderr", false, "also write runtime node service logs to stderr")
	return cmd
}

func runtimeNodeRunWorkers(cfg runtimeNodeConfig, concurrency int, pollInterval time.Duration, coord *runtimeNodeCoordinator, streamAgentOutput bool) error {
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > 64 {
		concurrency = 64
	}
	if pollInterval <= 0 {
		pollInterval = 3 * time.Second
	}
	var wg sync.WaitGroup
	for i := 1; i <= concurrency; i++ {
		workerID := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				if err := runtimeNodeLoopOnce(cfg, workerID, coord, streamAgentOutput); err != nil {
					slog.Warn("runtime worker loop error", "worker", workerID, "error", err)
					_ = runtimeNodeHeartbeat(cfg, "online", err.Error())
				}
				time.Sleep(pollInterval)
			}
		}()
	}
	wg.Wait()
	return nil
}

func runtimeNodeLoopOnce(cfg runtimeNodeConfig, workerID int, coord *runtimeNodeCoordinator, streamAgentOutput bool) error {
	if err := runtimeNodeHeartbeat(cfg, "online", ""); err != nil {
		return err
	}
	if coord == nil {
		coord = newRuntimeNodeCoordinator()
	}
	coord.mu.Lock()
	env, err := runtimeNodePost(cfg, "/api/v1/runtime-node/runs/claim", map[string]any{
		"capacity":   1,
		"busyAgents": coord.busyAgentListLocked(),
	})
	if err != nil {
		coord.mu.Unlock()
		return err
	}
	var claim runtimeNodeRunEnvelope
	if err := json.Unmarshal(env, &claim); err != nil {
		coord.mu.Unlock()
		return fmt.Errorf("decode claim response: %w", err)
	}
	if claim.Run == nil {
		coord.mu.Unlock()
		return nil
	}
	agentKey := runtimeNodeRunAgentKey(*claim.Run)
	coord.busyAgents[agentKey] = struct{}{}
	coord.mu.Unlock()
	defer func() {
		coord.mu.Lock()
		delete(coord.busyAgents, agentKey)
		coord.mu.Unlock()
	}()
	slog.Info("runtime run claimed", "worker", workerID, "run", claim.Run.ID, "project", claim.Run.ProjectID, "agent", claim.Run.AgentID)
	return runtimeNodeExecuteRun(cfg, *claim.Run, workerID, streamAgentOutput)
}

func runtimeNodeRunAgentKey(run runtimeNodeRun) string {
	if strings.TrimSpace(run.AgentWorkerID) != "" {
		return "worker/" + strings.TrimSpace(run.AgentWorkerID)
	}
	return strings.TrimSpace(run.ProjectID) + "/" + strings.TrimSpace(run.AgentID)
}

func resolveRuntimeNodeLogOptions(file, level, format string, maxSizeMB int, logStderr bool, flagChanged func(string) bool) serviceLogOptions {
	opts := resolveServiceLogOptions(loadedConfig, file, level, format, maxSizeMB, flagChanged)
	if !flagChanged("log-file") && strings.TrimSpace(file) == "" && os.Getenv("MULTIGENT_LOG_FILE") == "" {
		opts.File = runtimeNodeDefaultLogFile()
	}
	if flagChanged("log-stderr") {
		opts.Stderr = logStderr
	}
	return opts
}

func runtimeNodeDefaultLogFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".multigent", "runtime", "runtime-node.log")
	}
	return filepath.Join(home, ".multigent", "runtime", "runtime-node.log")
}

func startRuntimeNodeDaemon(logFile string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := filterRuntimeNodeDaemonArgs(os.Args[1:])
	logPath := strings.TrimSpace(logFile)
	if logPath == "" {
		logPath = runtimeNodeDefaultLogFile()
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	cmd := exec.Command(exe, args...)
	cmd.Env = append(os.Environ(), "MULTIGENT_RUNTIME_NODE_DAEMON_CHILD=1", "MULTIGENT_LOG_STDERR=false")
	cmd.Stdout = file
	cmd.Stderr = file
	setBackgroundProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	fmt.Printf("Runtime node daemon started (pid %d)\n", cmd.Process.Pid)
	fmt.Printf("Log file: %s\n", logPath)
	return cmd.Process.Release()
}

func filterRuntimeNodeDaemonArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--daemon" || arg == "-d" {
			continue
		}
		if strings.HasPrefix(arg, "--daemon=") {
			continue
		}
		out = append(out, arg)
	}
	return out
}

func runtimeNodeRegister(cfg runtimeNodeConfig) error {
	hostname, _ := os.Hostname()
	_, err := runtimeNodePost(cfg, "/api/v1/runtime-node/register", map[string]any{
		"os":           runtime.GOOS,
		"arch":         runtime.GOARCH,
		"hostname":     hostname,
		"version":      version,
		"capabilities": detectRuntimeNodeCapabilities(),
	})
	return err
}

func runtimeNodeHeartbeat(cfg runtimeNodeConfig, status, lastError string) error {
	hostname, _ := os.Hostname()
	_, err := runtimeNodePost(cfg, "/api/v1/runtime-node/heartbeat", map[string]any{
		"status":       status,
		"os":           runtime.GOOS,
		"arch":         runtime.GOARCH,
		"hostname":     hostname,
		"version":      version,
		"lastError":    lastError,
		"capabilities": detectRuntimeNodeCapabilities(),
	})
	return err
}

func runtimeNodeFailRun(cfg runtimeNodeConfig, runID, code, message string) error {
	_, err := runtimeNodePost(cfg, "/api/v1/runtime-node/runs/"+runID+"/fail", map[string]any{
		"errorCode":    code,
		"errorMessage": message,
		"result": map[string]any{
			"summary": message,
		},
	})
	return err
}

func runtimeNodeCompleteRun(cfg runtimeNodeConfig, runID string, result map[string]any) error {
	_, err := runtimeNodePost(cfg, "/api/v1/runtime-node/runs/"+runID+"/complete", map[string]any{
		"result": result,
	})
	return err
}

func runtimeNodeExtendRunLease(cfg runtimeNodeConfig, runID string, leaseSeconds int) error {
	if leaseSeconds <= 0 {
		leaseSeconds = 90
	}
	body, err := runtimeNodePost(cfg, "/api/v1/runtime-node/runs/"+runID+"/lease", map[string]any{
		"leaseSeconds": leaseSeconds,
	})
	if err != nil {
		return err
	}
	var env runtimeNodeLeaseEnvelope
	if err := json.Unmarshal(body, &env); err == nil && (env.Cancelled || (env.Run != nil && env.Run.Status == "cancelled")) {
		return errRuntimeRunCancelled
	}
	return nil
}

func runtimeNodeExecuteRun(cfg runtimeNodeConfig, run runtimeNodeRun, workerID int, streamAgentOutput bool) error {
	spec, err := runtimeNodeFetchRunSpec(cfg, run.ID)
	if err != nil {
		if errors.Is(err, errRuntimeRunCancelled) {
			slog.Info("runtime run cancelled before execution", "worker", workerID, "run", run.ID)
			return nil
		}
		_ = runtimeNodeFailRun(cfg, run.ID, "spec_fetch_failed", err.Error())
		return err
	}
	if spec.Kind != runtimeexec.KindExecPrompt && spec.Kind != runtimeexec.KindTask {
		msg := "unsupported runtime run kind: " + spec.Kind
		_ = runtimeNodeFailRun(cfg, run.ID, "unsupported_run_kind", msg)
		return fmt.Errorf("%s", msg)
	}
	if strings.TrimSpace(spec.Prompt) == "" {
		msg := "runtime run prompt is empty"
		_ = runtimeNodeFailRun(cfg, run.ID, "empty_prompt", msg)
		return fmt.Errorf("%s", msg)
	}
	meta := spec.Agent
	if strings.TrimSpace(meta.Name) == "" {
		meta.Name = spec.AgentID
	}
	if strings.TrimSpace(meta.Project) == "" {
		meta.Project = spec.ProjectID
	}
	if meta.Env == nil {
		meta.Env = map[string]string{}
	}
	for k, v := range spec.ProviderEnv {
		meta.Env[k] = v
	}
	meta.Provider = ""

	root, err := runtimeNodeWorkspaceRoot(spec.WorkspaceID)
	if err != nil {
		_ = runtimeNodeFailRun(cfg, run.ID, "workspace_prepare_failed", err.Error())
		return err
	}
	st := store.NewFS(root)
	agentDir := filepath.Join(root, "projects", spec.ProjectID, "agents", spec.AgentID)
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		_ = runtimeNodeFailRun(cfg, run.ID, "agent_prepare_failed", err.Error())
		return err
	}
	r := runner.New(root, taskstore.New(root), st)
	r.SetAgentMetaOverride(spec.ProjectID, spec.AgentID, &meta)
	r.SuppressStdout = !streamAgentOutput
	ctx, stopLease := startRuntimeRunLeaseLoop(cfg, run.ID)
	defer stopLease()
	started := time.Now().UTC()
	slog.Info("runtime run started", "worker", workerID, "run", run.ID, "kind", spec.Kind, "project", spec.ProjectID, "agent", spec.AgentID)
	result, err := r.ExecPromptWithRuntimeControlEnvContext(ctx, spec.ProjectID, spec.AgentID, spec.Prompt, spec.SessionID, spec.RuntimeControlEnv)
	durationMS := time.Since(started).Milliseconds()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			slog.Info("runtime run cancelled during execution", "worker", workerID, "run", run.ID)
			return nil
		}
		_ = runtimeNodeFailRun(cfg, run.ID, "executor_failed", err.Error())
		slog.Error("runtime run executor failed", "worker", workerID, "run", run.ID, "duration_ms", durationMS, "error", err)
		return err
	}
	out := map[string]any{
		"status":     string(result.Status),
		"sessionId":  result.SessionID,
		"logPath":    result.LogPath,
		"logText":    readRuntimeNodeLogText(result.LogPath, 512*1024),
		"summary":    result.Summary,
		"error":      result.ErrorMsg,
		"durationMs": durationMS,
	}
	if result.Status == entity.TaskStatusDoneFailed {
		msg := strings.TrimSpace(result.ErrorMsg)
		if msg == "" {
			msg = "agent run failed"
		}
		_ = runtimeNodeFailRun(cfg, run.ID, "agent_run_failed", msg)
		slog.Warn("runtime run failed", "worker", workerID, "run", run.ID, "status", result.Status, "duration_ms", durationMS, "log", result.LogPath, "error", msg)
		return fmt.Errorf("%s", msg)
	}
	slog.Info("runtime run completed", "worker", workerID, "run", run.ID, "status", result.Status, "duration_ms", durationMS, "session", result.SessionID, "log", result.LogPath)
	return runtimeNodeCompleteRun(cfg, run.ID, out)
}

func startRuntimeRunLeaseLoop(cfg runtimeNodeConfig, runID string) (context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := runtimeNodeHeartbeat(cfg, "online", ""); err != nil {
					slog.Warn("runtime node heartbeat failed during run", "run", runID, "error", err)
				}
				if err := runtimeNodeExtendRunLease(cfg, runID, 90); err != nil {
					if errors.Is(err, errRuntimeRunCancelled) {
						slog.Info("runtime run cancelled by control plane", "run", runID)
						cancel()
						return
					}
					slog.Warn("runtime lease renewal failed", "run", runID, "error", err)
				}
			case <-done:
				return
			}
		}
	}()
	return ctx, func() {
		cancel()
		close(done)
	}
}

func readRuntimeNodeLogText(path string, limit int64) string {
	path = strings.TrimSpace(path)
	if path == "" || limit <= 0 {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return ""
	}
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	if info.Size() > limit {
		if _, err := file.Seek(info.Size()-limit, io.SeekStart); err != nil {
			return ""
		}
		body, _ := io.ReadAll(io.LimitReader(file, limit))
		return "[log truncated]\n" + string(body)
	}
	body, _ := io.ReadAll(io.LimitReader(file, limit))
	return string(body)
}

func runtimeNodeFetchRunSpec(cfg runtimeNodeConfig, runID string) (runtimeexec.Spec, error) {
	body, err := runtimeNodeGet(cfg, "/api/v1/runtime-node/runs/"+runID+"/spec")
	if err != nil {
		return runtimeexec.Spec{}, err
	}
	var env runtimeNodeSpecEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return runtimeexec.Spec{}, fmt.Errorf("decode runtime run spec: %w", err)
	}
	if env.Run != nil && env.Run.Status == "cancelled" {
		return runtimeexec.Spec{}, errRuntimeRunCancelled
	}
	return env.Spec, nil
}

func runtimeNodeWorkspaceRoot(workspaceID string) (string, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		workspaceID = "default"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	root := filepath.Join(home, ".multigent", "runtime", "workspaces", workspaceID)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	return root, nil
}

func runtimeNodePost(cfg runtimeNodeConfig, path string, payload map[string]any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(cfg.ServerURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	client := http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxRuntimeCLIJSONBody+1))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("runtime node API %s returned HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return respBody, nil
}

func runtimeNodeGet(cfg runtimeNodeConfig, path string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(cfg.ServerURL, "/")+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Accept", "application/json")
	client := http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxRuntimeCLIJSONBody+1))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("runtime node API %s returned HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return respBody, nil
}

func detectRuntimeNodeCapabilities() map[string]any {
	dockerErr := sandbox.CheckDocker()
	docker := map[string]any{"available": dockerErr == nil}
	if dockerErr != nil {
		docker["reason"] = dockerErr.Error()
	}
	image := sandbox.DefaultBaseImage()
	runtimeImage := map[string]any{
		"image":     image,
		"available": dockerErr == nil && sandbox.ImageAvailable(image),
	}
	caps := sandbox.DetectCapabilities()
	euid := os.Geteuid()
	isRoot := euid == 0
	return map[string]any{
		"os":   runtime.GOOS,
		"arch": runtime.GOARCH,
		"direct": map[string]any{
			"available":                 true,
			"isRoot":                    isRoot,
			"euid":                      euid,
			"claudeCodeBypassAvailable": !isRoot || os.Getenv("IS_SANDBOX") != "",
			"reason":                    "host process execution is available on trusted runtime nodes",
		},
		"docker":       docker,
		"runtimeImage": runtimeImage,
		"kvm": map[string]any{
			"available": caps.KVM.Available,
			"reason":    caps.KVM.Reason,
		},
		"e2b": map[string]any{
			"available": caps.E2B.Available,
			"reason":    caps.E2B.Reason,
		},
		"resources": map[string]any{
			"cpuCount": runtime.NumCPU(),
		},
	}
}

func runtimeNodePrepareLocal() error {
	image := sandbox.DefaultBaseImage()
	fmt.Printf("Preparing Runtime Node sandbox image: %s\n", image)
	if err := sandbox.CheckDocker(); err != nil {
		return err
	}
	if err := sandbox.PullImage(image); err != nil {
		return fmt.Errorf("pull runtime image %s: %w", image, err)
	}
	if err := sandbox.RuntimeContainerAvailable(image, 30*time.Second); err != nil {
		return fmt.Errorf("runtime container smoke test failed: %w", err)
	}
	fmt.Println("Runtime Node sandbox is ready.")
	return nil
}

func loadRuntimeNodeConfig() (runtimeNodeConfig, error) {
	path := runtimeNodeConfigPath()
	body, err := os.ReadFile(path)
	if err != nil {
		return runtimeNodeConfig{}, fmt.Errorf("runtime node is not joined; run `multigent runtime join --server ... --token ...`: %w", err)
	}
	var cfg runtimeNodeConfig
	if err := json.Unmarshal(body, &cfg); err != nil {
		return runtimeNodeConfig{}, fmt.Errorf("read runtime node config: %w", err)
	}
	if strings.TrimSpace(cfg.ServerURL) == "" || strings.TrimSpace(cfg.Token) == "" {
		return runtimeNodeConfig{}, fmt.Errorf("runtime node config is incomplete; rerun `multigent runtime join`")
	}
	return cfg, nil
}

func (cfg runtimeNodeConfig) save() error {
	path := runtimeNodeConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(body, '\n'), 0o600)
}

func runtimeNodeConfigPath() string {
	if path := strings.TrimSpace(os.Getenv(runtimeNodeConfigEnv)); path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".multigent", "runtime", "config.json")
	}
	return filepath.Join(home, ".multigent", "runtime", "config.json")
}
