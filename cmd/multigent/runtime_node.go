package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	ID            string `json:"id"`
	WorkspaceID   string `json:"workspaceId"`
	ProjectID     string `json:"projectId"`
	AgentID       string `json:"agentId"`
	TaskID        string `json:"taskId"`
	RuntimeNodeID string `json:"runtimeNodeId"`
	Status        string `json:"status"`
}

type runtimeNodeSpecEnvelope struct {
	Run  *runtimeNodeRun  `json:"run"`
	Spec runtimeexec.Spec `json:"spec"`
}

type runtimeNodeLeaseEnvelope struct {
	Run       *runtimeNodeRun `json:"run"`
	Cancelled bool            `json:"cancelled"`
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
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Runtime Node foreground loop",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := loadRuntimeNodeConfig()
			if err != nil {
				return err
			}
			if err := runtimeNodeRegister(cfg); err != nil {
				return err
			}
			fmt.Printf("Runtime node connected to %s\n", cfg.ServerURL)
			if once {
				return runtimeNodeLoopOnce(cfg)
			}
			for {
				if err := runtimeNodeLoopOnce(cfg); err != nil {
					fmt.Fprintf(os.Stderr, "runtime loop error: %v\n", err)
				}
				if pollInterval <= 0 {
					pollInterval = 3 * time.Second
				}
				time.Sleep(pollInterval)
			}
		},
	}
	cmd.Flags().BoolVar(&once, "once", false, "run one heartbeat/claim cycle and exit")
	cmd.Flags().DurationVar(&pollInterval, "poll-interval", 3*time.Second, "run claim polling interval")
	return cmd
}

func runtimeNodeLoopOnce(cfg runtimeNodeConfig) error {
	if err := runtimeNodeHeartbeat(cfg, "online", ""); err != nil {
		return err
	}
	env, err := runtimeNodePost(cfg, "/api/v1/runtime-node/runs/claim", map[string]any{
		"capacity": 1,
	})
	if err != nil {
		return err
	}
	var claim runtimeNodeRunEnvelope
	if err := json.Unmarshal(env, &claim); err != nil {
		return fmt.Errorf("decode claim response: %w", err)
	}
	if claim.Run == nil {
		return nil
	}
	fmt.Printf("claimed run %s for %s/%s\n", claim.Run.ID, claim.Run.ProjectID, claim.Run.AgentID)
	return runtimeNodeExecuteRun(cfg, *claim.Run)
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

func runtimeNodeExecuteRun(cfg runtimeNodeConfig, run runtimeNodeRun) error {
	spec, err := runtimeNodeFetchRunSpec(cfg, run.ID)
	if err != nil {
		if errors.Is(err, errRuntimeRunCancelled) {
			fmt.Printf("runtime run %s was cancelled before execution\n", run.ID)
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
	if err := st.SaveAgentMeta(spec.ProjectID, spec.AgentID, &meta); err != nil {
		_ = runtimeNodeFailRun(cfg, run.ID, "agent_prepare_failed", err.Error())
		return err
	}
	r := runner.New(root, taskstore.New(root), st)
	ctx, stopLease := startRuntimeRunLeaseLoop(cfg, run.ID)
	defer stopLease()
	started := time.Now().UTC()
	result, err := r.ExecPromptWithRuntimeControlEnvContext(ctx, spec.ProjectID, spec.AgentID, spec.Prompt, spec.SessionID, spec.RuntimeControlEnv)
	durationMS := time.Since(started).Milliseconds()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Printf("runtime run %s was cancelled during execution\n", run.ID)
			return nil
		}
		_ = runtimeNodeFailRun(cfg, run.ID, "executor_failed", err.Error())
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
		return fmt.Errorf("%s", msg)
	}
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
				if err := runtimeNodeExtendRunLease(cfg, runID, 90); err != nil {
					if errors.Is(err, errRuntimeRunCancelled) {
						fmt.Fprintf(os.Stderr, "runtime run %s was cancelled by the control plane\n", runID)
						cancel()
						return
					}
					fmt.Fprintf(os.Stderr, "runtime lease renewal failed for %s: %v\n", runID, err)
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
		"os":           runtime.GOOS,
		"arch":         runtime.GOARCH,
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
