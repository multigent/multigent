package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/multigent/multigent/internal/contextpack"
	"github.com/spf13/cobra"
)

func newContextCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Import and link reference material for agents",
		Long: `Import and link reference material for agents.

Reference material is stored as managed context artifacts. Text notes become
knowledge-base documents; raw agent sessions are kept as workspace files with
a small knowledge-base index card. Bindings decide which workspace, project,
or agent can see it at runtime through mga context.`,
	}
	cmd.AddCommand(
		newContextCollectorsCmd(),
		newContextScanSessionsCmd(),
		newContextImportSessionCmd(),
		newContextImportFileCmd(),
		newContextBindCmd(),
		newContextListCmd(),
	)
	return cmd
}

func newContextCollectorsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "collectors",
		Short: "List available context collectors",
		RunE: func(cmd *cobra.Command, args []string) error {
			return printJSON(map[string]any{"collectors": contextpack.NewRegistry().Specs()})
		},
	}
}

func newContextScanSessionsCmd() *cobra.Command {
	var cli, home string
	var limit int
	cmd := &cobra.Command{
		Use:   "scan-sessions",
		Short: "Scan local Claude Code, Codex, and Cursor session files",
		RunE: func(cmd *cobra.Command, args []string) error {
			candidates, err := contextpack.ScanLocalAgentSessions(contextpack.SessionScanOptions{
				Home:  home,
				CLI:   cli,
				Limit: limit,
			})
			if err != nil {
				return err
			}
			return printJSON(map[string]any{"sessions": candidates})
		},
	}
	cmd.Flags().StringVar(&cli, "cli", "", "filter by agent CLI: claudecode, codex, cursor")
	cmd.Flags().StringVar(&home, "home", "", "home directory to scan (default: current user)")
	cmd.Flags().IntVar(&limit, "limit", 50, "max sessions to return")
	return cmd
}

func newContextImportSessionCmd() *cobra.Command {
	var path, cli, title, project, bindAgent, bindProject, createdBy, description string
	var server, token, workspaceID string
	var tags []string
	var required bool
	cmd := &cobra.Command{
		Use:   "import-session",
		Short: "Import a local agent session file as reference material",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(path) == "" {
				return fmt.Errorf("--path is required")
			}
			scope, scopeID, err := contextScopeFromFlags(bindAgent, bindProject, "")
			if err != nil {
				return err
			}
			if createdBy == "" {
				createdBy = "human"
			}
			meta := map[string]string{}
			if strings.TrimSpace(cli) != "" {
				meta["cli"] = strings.TrimSpace(cli)
			}
			if endpoint := contextUploadEndpoint(server); endpoint != "" {
				res, err := uploadContextContent(endpoint, token, workspaceID, contextpack.ImportContentInput{
					Path:          path,
					CollectorType: contextpack.CollectorLocalAgentSession,
					Title:         title,
					Project:       projectFromBinding(project, bindAgent, bindProject),
					Tags:          append([]string{"session"}, tags...),
					Description:   description,
					BindScope:     scope,
					BindScopeID:   scopeID,
					Required:      required,
					Metadata:      meta,
				})
				if err != nil {
					return err
				}
				printImportResult(res)
				return nil
			}
			root, err := resolveRoot()
			if err != nil {
				return err
			}
			res, err := contextpack.NewStore(root).ImportLocalPath(contextpack.ImportLocalPathInput{
				Path:          path,
				CollectorType: contextpack.CollectorLocalAgentSession,
				Title:         title,
				CreatedBy:     createdBy,
				Project:       projectFromBinding(project, bindAgent, bindProject),
				Tags:          append([]string{"session"}, tags...),
				Description:   description,
				BindScope:     scope,
				BindScopeID:   scopeID,
				Required:      required,
				Metadata:      meta,
			})
			if err != nil {
				return err
			}
			printImportResult(res)
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "local session file path")
	cmd.Flags().StringVar(&cli, "cli", "", "source agent CLI: claudecode, codex, cursor, or custom")
	cmd.Flags().StringVar(&title, "title", "", "reference material title")
	cmd.Flags().StringVar(&project, "project", "", "project used for knowledge-base index")
	cmd.Flags().StringVar(&bindAgent, "bind-agent", "", "bind to one agent, format: project/agent")
	cmd.Flags().StringVar(&bindProject, "bind-project", "", "bind to one project")
	cmd.Flags().StringVar(&createdBy, "created-by", "", "creator label (default: human)")
	cmd.Flags().StringVar(&description, "description", "", "short description")
	cmd.Flags().StringVar(&server, "server", "", "control-plane API base URL (or MULTIGENT_API_URL)")
	cmd.Flags().StringVar(&token, "token", "", "client token for remote upload (or MULTIGENT_CLIENT_TOKEN)")
	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "workspace id for remote upload (or MULTIGENT_WORKSPACE_ID)")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "tag (repeatable)")
	cmd.Flags().BoolVar(&required, "required", false, "mark this material as required for the bound scope")
	return cmd
}

func newContextImportFileCmd() *cobra.Command {
	var path, title, project, bindAgent, bindProject, createdBy, description string
	var server, token, workspaceID string
	var tags []string
	var required bool
	cmd := &cobra.Command{
		Use:   "import-file",
		Short: "Import a local text file into the knowledge base",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(path) == "" {
				return fmt.Errorf("--path is required")
			}
			scope, scopeID, err := contextScopeFromFlags(bindAgent, bindProject, "")
			if err != nil {
				return err
			}
			if createdBy == "" {
				createdBy = "human"
			}
			if endpoint := contextUploadEndpoint(server); endpoint != "" {
				res, err := uploadContextContent(endpoint, token, workspaceID, contextpack.ImportContentInput{
					Path:          path,
					CollectorType: contextpack.CollectorLocalFile,
					Title:         title,
					Project:       projectFromBinding(project, bindAgent, bindProject),
					Tags:          tags,
					Description:   description,
					BindScope:     scope,
					BindScopeID:   scopeID,
					Required:      required,
				})
				if err != nil {
					return err
				}
				printImportResult(res)
				return nil
			}
			root, err := resolveRoot()
			if err != nil {
				return err
			}
			res, err := contextpack.NewStore(root).ImportLocalPath(contextpack.ImportLocalPathInput{
				Path:          path,
				CollectorType: contextpack.CollectorLocalFile,
				Title:         title,
				CreatedBy:     createdBy,
				Project:       projectFromBinding(project, bindAgent, bindProject),
				Tags:          tags,
				Description:   description,
				BindScope:     scope,
				BindScopeID:   scopeID,
				Required:      required,
			})
			if err != nil {
				return err
			}
			printImportResult(res)
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "local text file path")
	cmd.Flags().StringVar(&title, "title", "", "knowledge document title")
	cmd.Flags().StringVar(&project, "project", "", "project used for knowledge-base index")
	cmd.Flags().StringVar(&bindAgent, "bind-agent", "", "bind to one agent, format: project/agent")
	cmd.Flags().StringVar(&bindProject, "bind-project", "", "bind to one project")
	cmd.Flags().StringVar(&createdBy, "created-by", "", "creator label (default: human)")
	cmd.Flags().StringVar(&description, "description", "", "short description")
	cmd.Flags().StringVar(&server, "server", "", "control-plane API base URL (or MULTIGENT_API_URL)")
	cmd.Flags().StringVar(&token, "token", "", "client token for remote upload (or MULTIGENT_CLIENT_TOKEN)")
	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "workspace id for remote upload (or MULTIGENT_WORKSPACE_ID)")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "tag (repeatable)")
	cmd.Flags().BoolVar(&required, "required", false, "mark this material as required for the bound scope")
	return cmd
}

func newContextBindCmd() *cobra.Command {
	var docID, artifactID, bindAgent, bindProject, bindWorkspace, createdBy string
	var required bool
	var priority int
	cmd := &cobra.Command{
		Use:   "bind",
		Short: "Bind an existing knowledge document to a workspace, project, or agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := resolveRoot()
			if err != nil {
				return err
			}
			scope, scopeID, err := contextScopeFromFlags(bindAgent, bindProject, bindWorkspace)
			if err != nil {
				return err
			}
			if scope == "" {
				scope = contextpack.ScopeWorkspace
			}
			if createdBy == "" {
				createdBy = "human"
			}
			binding, err := contextpack.NewStore(root).AddBinding(contextpack.Binding{
				ArtifactID: artifactID,
				DocID:      docID,
				ScopeType:  scope,
				ScopeID:    scopeID,
				Required:   required,
				Priority:   priority,
				CreatedBy:  createdBy,
			})
			if err != nil {
				return err
			}
			if err := printJSON(binding); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "Next: run `multigent sync` to refresh affected agent context files.")
			return nil
		},
	}
	cmd.Flags().StringVar(&docID, "doc", "", "knowledge document id")
	cmd.Flags().StringVar(&artifactID, "artifact", "", "context artifact id")
	cmd.Flags().StringVar(&bindAgent, "agent", "", "bind to one agent, format: project/agent")
	cmd.Flags().StringVar(&bindProject, "project", "", "bind to one project")
	cmd.Flags().StringVar(&bindWorkspace, "workspace", "", "bind to workspace; pass true or workspace id")
	cmd.Flags().StringVar(&createdBy, "created-by", "", "creator label (default: human)")
	cmd.Flags().BoolVar(&required, "required", false, "mark this material as required")
	cmd.Flags().IntVar(&priority, "priority", 0, "higher priority appears earlier in runtime context")
	return cmd
}

func newContextListCmd() *cobra.Command {
	var scope, scopeID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List reference material bindings",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := resolveRoot()
			if err != nil {
				return err
			}
			var scopes []contextpack.ScopeRef
			if strings.TrimSpace(scope) != "" {
				scopes = []contextpack.ScopeRef{{Type: scope, ID: scopeID}}
			}
			views, err := contextpack.NewStore(root).ListBindingViews(scopes)
			if err != nil {
				return err
			}
			return printJSON(map[string]any{"bindings": views})
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "", "optional scope: workspace, project, agent")
	cmd.Flags().StringVar(&scopeID, "scope-id", "", "scope id, e.g. project or project/agent")
	return cmd
}

func contextScopeFromFlags(agent, project, workspace string) (string, string, error) {
	count := 0
	if strings.TrimSpace(agent) != "" {
		count++
	}
	if strings.TrimSpace(project) != "" {
		count++
	}
	if strings.TrimSpace(workspace) != "" {
		count++
	}
	if count > 1 {
		return "", "", fmt.Errorf("choose only one binding scope")
	}
	if strings.TrimSpace(agent) != "" {
		if !strings.Contains(agent, "/") {
			return "", "", fmt.Errorf("agent binding must use project/agent format")
		}
		return contextpack.ScopeAgent, strings.TrimSpace(agent), nil
	}
	if strings.TrimSpace(project) != "" {
		return contextpack.ScopeProject, strings.TrimSpace(project), nil
	}
	if strings.TrimSpace(workspace) != "" {
		return contextpack.ScopeWorkspace, "", nil
	}
	return "", "", nil
}

func projectFromBinding(project, bindAgent, bindProject string) string {
	if strings.TrimSpace(project) != "" {
		return strings.TrimSpace(project)
	}
	if strings.TrimSpace(bindProject) != "" {
		return strings.TrimSpace(bindProject)
	}
	if p, _, ok := strings.Cut(strings.TrimSpace(bindAgent), "/"); ok {
		return p
	}
	return ""
}

func printImportResult(res *contextpack.ImportManualResult) {
	if res == nil || res.Doc == nil {
		return
	}
	fmt.Printf("✓ Imported reference material: %s\n", res.Doc.ID)
	fmt.Printf("  Title: %s\n", res.Doc.Title)
	fmt.Printf("  Index: %s\n", res.Doc.Index)
	if res.Binding != nil {
		fmt.Printf("  Bound: %s", res.Binding.ScopeType)
		if res.Binding.ScopeID != "" {
			fmt.Printf(":%s", res.Binding.ScopeID)
		}
		fmt.Println()
		fmt.Println("  Next: run `multigent sync` to refresh affected agent context files.")
	}
}

func contextUploadEndpoint(server string) string {
	server = strings.TrimSpace(firstNonEmptyString(server, os.Getenv("MULTIGENT_API_URL")))
	if server == "" {
		return ""
	}
	return strings.TrimRight(server, "/") + "/api/v1/context/import"
}

func uploadContextContent(endpoint, token, workspaceID string, input contextpack.ImportContentInput) (*contextpack.ImportManualResult, error) {
	token = strings.TrimSpace(firstNonEmptyString(token, os.Getenv("MULTIGENT_CLIENT_TOKEN")))
	if token == "" {
		return nil, fmt.Errorf("remote context upload requires --token or MULTIGENT_CLIENT_TOKEN")
	}
	workspaceID = strings.TrimSpace(firstNonEmptyString(workspaceID, os.Getenv("MULTIGENT_WORKSPACE_ID")))
	path := strings.TrimSpace(input.Path)
	if path == "" {
		return nil, fmt.Errorf("--path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"collectorType": input.CollectorType,
		"title":         input.Title,
		"content":       string(raw),
		"sourceName":    filepath.Base(abs),
		"filePath":      abs,
		"project":       input.Project,
		"tags":          input.Tags,
		"description":   input.Description,
		"bindScope":     input.BindScope,
		"bindScopeId":   input.BindScopeID,
		"required":      input.Required,
		"metadata":      input.Metadata,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if workspaceID != "" {
		req.Header.Set("X-Multigent-Workspace-ID", workspaceID)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("context upload failed: %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}
	var res contextpack.ImportManualResult
	if err := json.Unmarshal(respBody, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
