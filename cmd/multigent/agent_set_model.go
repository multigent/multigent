package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/multigent/multigent/internal/entity"
	"github.com/spf13/cobra"
)

func newAgentSetModelCmd() *cobra.Command {
	var (
		project     string
		agentName   string
		model       string
		httpURL     string
		httpModel   string
		httpAPIKey  string
		httpTimeout string
		httpStream  bool
		httpHeaders []string
	)

	cmd := &cobra.Command{
		Use:   "set-model",
		Short: "Switch an agent to a different runtime (e.g. claudecode → codex)",
		Long: `set-model updates a workspace Agent Worker's runtime model.
Agent context is resolved dynamically from workspace, team, role, project,
membership, skills, and bound context sources; this command does not rewrite
project agent directories.

Examples:
  multigent agent set-model --project my-api --name dev --model codex
  multigent agent set-model --project my-api --name bot --model http-agent \
    --http-url http://localhost:11434/v1/chat/completions --http-model llama3.2`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if project == "" || agentName == "" || model == "" {
				return fmt.Errorf("--project, --name, and --model are required")
			}

			newModel := entity.NormaliseModel(entity.AgentModel(model))
			if !entity.IsValidModel(newModel) {
				return fmt.Errorf("unknown model %q (supported: %s)",
					model, joinModels(entity.KnownModels))
			}
			if newModel == entity.ModelHuman {
				return fmt.Errorf("human is not a runtime model for an Agent Worker")
			}

			root, err := resolveRoot()
			if err != nil {
				return err
			}
			worker, ok, db, _, err := resolveCLIProjectWorker(root, project, agentName)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("agent worker membership %s/%s not found", project, agentName)
			}

			oldModel := entity.NormaliseModel(entity.AgentModel(worker.Model))
			if oldModel == newModel {
				return fmt.Errorf("agent %s/%s already uses model %q", project, agentName, newModel)
			}

			if newModel == entity.ModelHTTPAgent && httpURL == "" {
				return fmt.Errorf("--http-url is required when switching to http-agent")
			}

			cfg := decodeCLIWorkerRuntimeConfig(worker.RuntimeConfigJSON)
			now := time.Now().UTC()
			worker.Model = string(newModel)
			worker.RuntimeModel = ""
			worker.UpdatedAt = now.Format(time.RFC3339)

			if newModel == entity.ModelHTTPAgent {
				cfg.HTTPAgent = &entity.HTTPAgentConfig{
					URL:     httpURL,
					Model:   httpModel,
					APIKey:  httpAPIKey,
					Timeout: httpTimeout,
					Stream:  httpStream,
				}
				if len(httpHeaders) > 0 {
					cfg.HTTPAgent.ExtraHeaders = make(map[string]string, len(httpHeaders))
					for _, h := range httpHeaders {
						k, v, ok := strings.Cut(h, ":")
						if !ok {
							return fmt.Errorf("--http-header %q: expected \"Key: Value\" format", h)
						}
						cfg.HTTPAgent.ExtraHeaders[strings.TrimSpace(k)] = strings.TrimSpace(v)
					}
				}
			} else {
				cfg.HTTPAgent = nil
			}
			worker.RuntimeConfigJSON = encodeCLIWorkerRuntimeConfig(cfg)
			if err := db.UpsertAgentWorker(worker); err != nil {
				return fmt.Errorf("save agent worker: %w", err)
			}

			fmt.Printf("✓ %s/%s: model %q → %q\n", project, agentName, oldModel, newModel)
			return nil
		},
	}

	cmd.Flags().StringVar(&project, "project", "", "Project name")
	cmd.Flags().StringVar(&agentName, "name", "", "Agent name (directory name under projects/<project>/agents/)")
	cmd.Flags().StringVar(&model, "model", "", fmt.Sprintf("New agent model (%s)", joinModels(entity.KnownModels)))
	cmd.Flags().StringVar(&httpURL, "http-url", "", "HTTP chat-completions URL (required when --model http-agent)")
	cmd.Flags().StringVar(&httpModel, "http-model", "", "Model id in JSON body for http-agent")
	cmd.Flags().StringVar(&httpAPIKey, "http-api-key", "", "Bearer token for http-agent (or MULTIGENT_HTTP_API_KEY)")
	cmd.Flags().StringVar(&httpTimeout, "http-timeout", "10m", "Per-request timeout for http-agent")
	cmd.Flags().BoolVar(&httpStream, "http-stream", true, "Enable SSE streaming for http-agent")
	cmd.Flags().StringArrayVar(&httpHeaders, "http-header", nil, `Extra HTTP headers "Key: Value" (repeatable, http-agent only)`)

	_ = cmd.MarkFlagRequired("project")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("model")

	return cmd
}
