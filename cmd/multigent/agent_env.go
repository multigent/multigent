package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/multigent/multigent/internal/agentdir"
	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/spf13/cobra"
)

type cliAgentRuntimeConfig struct {
	Env       map[string]string       `json:"env,omitempty"`
	Sandbox   *entity.SandboxConfig   `json:"sandbox,omitempty"`
	AddDirs   []string                `json:"addDirs,omitempty"`
	HTTPAgent *entity.HTTPAgentConfig `json:"httpAgent,omitempty"`
}

func newAgentSetEnvCmd() *cobra.Command {
	var (
		project   string
		agentName string
	)
	cmd := &cobra.Command{
		Use:   "set-env KEY=VALUE",
		Short: "Set an environment variable on a specific agent",
		Long: `Per-agent env vars have the highest priority and override workspace
secrets and API provider settings.`,
		Example: `  multigent agent set-env ANTHROPIC_MODEL=claude-sonnet-4-20250514 -p myproj -a dev
  multigent agent set-env MY_TOKEN=secret123 --project myproj --agent pm`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := resolveRoot()
			if err != nil {
				return err
			}
			key, value, ok := strings.Cut(args[0], "=")
			if !ok || strings.TrimSpace(key) == "" {
				return fmt.Errorf("expected KEY=VALUE format")
			}
			key = strings.TrimSpace(key)

			if updated, err := updateWorkerAgentEnv(root, project, agentName, func(env map[string]string) (map[string]string, error) {
				env[key] = value
				return env, nil
			}); err != nil {
				return err
			} else if updated {
				fmt.Printf("Set %s on %s/%s\n", key, project, agentName)
				return nil
			}
			return fmt.Errorf("agent worker membership %s/%s not found", project, agentName)
		},
	}
	cmd.Flags().StringVarP(&project, "project", "p", "", "Project name (required)")
	cmd.Flags().StringVarP(&agentName, "agent", "a", "", "Agent name (required)")
	_ = cmd.MarkFlagRequired("project")
	_ = cmd.MarkFlagRequired("agent")
	return cmd
}

func newAgentUnsetEnvCmd() *cobra.Command {
	var (
		project   string
		agentName string
	)
	cmd := &cobra.Command{
		Use:     "unset-env KEY",
		Short:   "Remove an environment variable from a specific agent",
		Example: `  multigent agent unset-env ANTHROPIC_MODEL -p myproj -a dev`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := resolveRoot()
			if err != nil {
				return err
			}
			key := strings.TrimSpace(args[0])
			if key == "" {
				return fmt.Errorf("key is required")
			}

			if updated, err := updateWorkerAgentEnv(root, project, agentName, func(env map[string]string) (map[string]string, error) {
				if env == nil || env[key] == "" {
					return nil, fmt.Errorf("env %q not set on %s/%s", key, project, agentName)
				}
				delete(env, key)
				return env, nil
			}); err != nil {
				return err
			} else if updated {
				fmt.Printf("Unset %s from %s/%s\n", key, project, agentName)
				return nil
			}
			return fmt.Errorf("agent worker membership %s/%s not found", project, agentName)
		},
	}
	cmd.Flags().StringVarP(&project, "project", "p", "", "Project name (required)")
	cmd.Flags().StringVarP(&agentName, "agent", "a", "", "Agent name (required)")
	_ = cmd.MarkFlagRequired("project")
	_ = cmd.MarkFlagRequired("agent")
	return cmd
}

func newAgentListEnvCmd() *cobra.Command {
	var (
		project   string
		agentName string
	)
	cmd := &cobra.Command{
		Use:     "list-env",
		Short:   "List environment variables for a specific agent",
		Example: `  multigent agent list-env -p myproj -a dev`,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := resolveRoot()
			if err != nil {
				return err
			}
			if env, ok, err := workerAgentEnv(root, project, agentName); err != nil {
				return err
			} else if ok {
				if len(env) == 0 {
					fmt.Printf("No per-agent env vars set on %s/%s.\n", project, agentName)
					return nil
				}
				fmt.Printf("Per-agent env for %s/%s:\n", project, agentName)
				for k, v := range env {
					masked := maskValue(v)
					fmt.Printf("  %s=%s\n", k, masked)
				}
				return nil
			}
			return fmt.Errorf("agent worker membership %s/%s not found", project, agentName)
		},
	}
	cmd.Flags().StringVarP(&project, "project", "p", "", "Project name (required)")
	cmd.Flags().StringVarP(&agentName, "agent", "a", "", "Agent name (required)")
	_ = cmd.MarkFlagRequired("project")
	_ = cmd.MarkFlagRequired("agent")
	return cmd
}

func workerAgentEnv(root, project, agentName string) (map[string]string, bool, error) {
	worker, ok, db, workspaceID, err := resolveCLIProjectWorker(root, project, agentName)
	if err != nil || !ok {
		return nil, ok, err
	}
	cfg := decodeCLIWorkerRuntimeConfig(worker.RuntimeConfigJSON)
	if cfg.Env == nil {
		cfg.Env = map[string]string{}
	}
	_ = db
	_ = workspaceID
	return cfg.Env, true, nil
}

func updateWorkerAgentEnv(root, project, agentName string, update func(map[string]string) (map[string]string, error)) (bool, error) {
	worker, ok, db, _, err := resolveCLIProjectWorker(root, project, agentName)
	if err != nil || !ok {
		return ok, err
	}
	cfg := decodeCLIWorkerRuntimeConfig(worker.RuntimeConfigJSON)
	if cfg.Env == nil {
		cfg.Env = map[string]string{}
	}
	next, err := update(cfg.Env)
	if err != nil {
		return true, err
	}
	cfg.Env = next
	worker.RuntimeConfigJSON = encodeCLIWorkerRuntimeConfig(cfg)
	worker.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := db.UpsertAgentWorker(worker); err != nil {
		return true, err
	}
	return true, nil
}

func resolveCLIProjectWorker(root, project, agentName string) (controldb.AgentWorker, bool, controldb.Store, string, error) {
	db, err := openControlDBForRoot(root)
	if err != nil {
		return controldb.AgentWorker{}, false, nil, "", err
	}
	workspaceID, err := workspaceIDForRoot(db, root)
	if err != nil || strings.TrimSpace(workspaceID) == "" {
		return controldb.AgentWorker{}, false, db, workspaceID, err
	}
	resolved, ok, err := agentdir.New(db).ProjectWorker(workspaceID, project, agentName)
	if err != nil || !ok {
		return controldb.AgentWorker{}, ok, db, workspaceID, err
	}
	return resolved.Worker, true, db, workspaceID, nil
}

func decodeCLIWorkerRuntimeConfig(raw string) cliAgentRuntimeConfig {
	var cfg cliAgentRuntimeConfig
	if strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	return cfg
}

func encodeCLIWorkerRuntimeConfig(cfg cliAgentRuntimeConfig) string {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func maskValue(v string) string {
	if len(v) <= 8 {
		return "****"
	}
	return v[:4] + "****" + v[len(v)-4:]
}
