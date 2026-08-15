package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/multigent/multigent/internal/entity"
	workflowstore "github.com/multigent/multigent/internal/workflow"
	"github.com/spf13/cobra"
)

func newWorkflowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflow",
		Short: "Manage workspace workflow definitions",
		Long: `Manage reusable workflow definitions.

Workflows are workspace-level SOP/state-machine definitions. Tasks can bind to
a workflow through a task template and then move across agent and human steps.`,
	}
	cmd.AddCommand(
		newWorkflowListCmd(),
		newWorkflowTemplatesCmd(),
		newWorkflowShowCmd(),
		newWorkflowCreateCmd(),
		newWorkflowUpdateCmd(),
		newWorkflowDeleteCmd(),
		newWorkflowExportCmd(),
		newWorkflowScaffoldCmd(),
	)
	return cmd
}

func newWorkflowListCmd() *cobra.Command {
	var workspaceRef, format string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workflow definitions",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := openCLIWorkspaceDB(workspaceRef)
			if err != nil {
				return err
			}
			defer ctx.Close()
			defs, err := workflowstore.NewStore(ctx.db, ctx.workspaceID).ListDefinitions()
			if err != nil {
				return err
			}
			if resolveFormat(format) == "json" {
				if defs == nil {
					defs = []entity.WorkflowDefinition{}
				}
				return printJSON(defs)
			}
			if len(defs) == 0 {
				fmt.Println("No workflows found. Run: multigent workflow create --file workflow.json")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tNAME\tSTEPS\tUPDATED")
			for _, def := range defs {
				fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", def.ID, def.Name, len(def.Steps), formatWorkflowTime(def.UpdatedAt))
			}
			return w.Flush()
		},
	}
	addWorkspaceAndFormatFlags(cmd, &workspaceRef, &format)
	return cmd
}

func newWorkflowTemplatesCmd() *cobra.Command {
	var locale, format string
	cmd := &cobra.Command{
		Use:   "templates",
		Short: "List built-in workflow templates",
		RunE: func(cmd *cobra.Command, args []string) error {
			templates := workflowstore.Templates(locale)
			if resolveFormat(format) == "json" {
				return printJSON(templates)
			}
			if len(templates) == 0 {
				fmt.Println("No workflow templates found.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tNAME\tSTEPS\tDESCRIPTION")
			for _, tmpl := range templates {
				fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", tmpl.ID, tmpl.Name, len(tmpl.Steps), tmpl.Description)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&locale, "locale", "", "template locale, e.g. zh-CN or en-US")
	cmd.Flags().StringVar(&format, "format", "", "output format: json or table")
	return cmd
}

func newWorkflowShowCmd() *cobra.Command {
	var workspaceRef, format string
	cmd := &cobra.Command{
		Use:   "show <workflow-id>",
		Short: "Show one workflow definition",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			def, err := loadWorkflowDefinition(workspaceRef, args[0])
			if err != nil {
				return err
			}
			if resolveFormat(format) == "json" {
				return printJSON(def)
			}
			printWorkflowSummary(def)
			return nil
		},
	}
	addWorkspaceAndFormatFlags(cmd, &workspaceRef, &format)
	return cmd
}

func newWorkflowCreateCmd() *cobra.Command {
	var workspaceRef, file, templateID, locale, name string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a workflow from JSON or a built-in template",
		Example: `  multigent workflow create --file workflow.json
  multigent workflow create --template software-delivery --name "研发交付流程" --locale zh-CN`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := openCLIWorkspaceDB(workspaceRef)
			if err != nil {
				return err
			}
			defer ctx.Close()
			var def entity.WorkflowDefinition
			if strings.TrimSpace(templateID) != "" {
				var ok bool
				def, ok = workflowstore.DefinitionFromTemplate(templateID, locale, name)
				if !ok {
					return fmt.Errorf("workflow template %q not found", templateID)
				}
			} else {
				if err := readJSONFile(file, &def); err != nil {
					return err
				}
				normalizeWorkflowDefinition(&def, false)
			}
			if err := validateWorkflowDefinition(def); err != nil {
				return err
			}
			if err := workflowstore.NewStore(ctx.db, ctx.workspaceID).SaveDefinition(&def); err != nil {
				return err
			}
			return printJSON(def)
		},
	}
	cmd.Flags().StringVar(&workspaceRef, "workspace", "", "workspace id, name, slug, or root path")
	cmd.Flags().StringVar(&file, "file", "", "workflow JSON file, or '-' for stdin")
	cmd.Flags().StringVar(&templateID, "template", "", "built-in workflow template id")
	cmd.Flags().StringVar(&locale, "locale", "", "template locale, e.g. zh-CN or en-US")
	cmd.Flags().StringVar(&name, "name", "", "workflow name when creating from template")
	return cmd
}

func newWorkflowUpdateCmd() *cobra.Command {
	var workspaceRef, file string
	cmd := &cobra.Command{
		Use:   "update <workflow-id> --file workflow.json",
		Short: "Replace a workflow definition from JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := openCLIWorkspaceDB(workspaceRef)
			if err != nil {
				return err
			}
			defer ctx.Close()
			store := workflowstore.NewStore(ctx.db, ctx.workspaceID)
			existing, ok, err := store.Definition(args[0])
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("workflow %q not found", args[0])
			}
			var def entity.WorkflowDefinition
			if err := readJSONFile(file, &def); err != nil {
				return err
			}
			def.ID = existing.ID
			def.CreatedAt = existing.CreatedAt
			if def.Version <= existing.Version {
				def.Version = existing.Version + 1
			}
			normalizeWorkflowDefinition(&def, true)
			if err := validateWorkflowDefinition(def); err != nil {
				return err
			}
			if err := store.SaveDefinition(&def); err != nil {
				return err
			}
			return printJSON(def)
		},
	}
	cmd.Flags().StringVar(&workspaceRef, "workspace", "", "workspace id, name, slug, or root path")
	cmd.Flags().StringVar(&file, "file", "", "workflow JSON file, or '-' for stdin")
	return cmd
}

func newWorkflowDeleteCmd() *cobra.Command {
	var workspaceRef string
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete <workflow-id>",
		Short: "Delete a workflow definition",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				return fmt.Errorf("refusing to delete workflow %q without --yes", args[0])
			}
			ctx, err := openCLIWorkspaceDB(workspaceRef)
			if err != nil {
				return err
			}
			defer ctx.Close()
			store := workflowstore.NewStore(ctx.db, ctx.workspaceID)
			if _, ok, err := store.Definition(args[0]); err != nil {
				return err
			} else if !ok {
				return fmt.Errorf("workflow %q not found", args[0])
			}
			if err := store.DeleteDefinition(args[0]); err != nil {
				return err
			}
			return printJSON(map[string]string{"deleted": args[0]})
		},
	}
	cmd.Flags().StringVar(&workspaceRef, "workspace", "", "workspace id, name, slug, or root path")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm deletion")
	return cmd
}

func newWorkflowExportCmd() *cobra.Command {
	var workspaceRef, out string
	cmd := &cobra.Command{
		Use:   "export <workflow-id>",
		Short: "Export a workflow definition as JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			def, err := loadWorkflowDefinition(workspaceRef, args[0])
			if err != nil {
				return err
			}
			return writeJSONFile(out, def)
		},
	}
	cmd.Flags().StringVar(&workspaceRef, "workspace", "", "workspace id, name, slug, or root path")
	cmd.Flags().StringVar(&out, "out", "", "write JSON to file instead of stdout")
	return cmd
}

func newWorkflowScaffoldCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scaffold",
		Short: "Generate workflow JSON scaffolds",
	}
	cmd.AddCommand(newWorkflowScaffoldParallelCmd())
	return cmd
}

func newWorkflowScaffoldParallelCmd() *cobra.Command {
	var workspaceRef, name, description, startRole, startTitle, stageTitle, joinPolicy, finalReviewRole, out string
	var save bool
	var branches []string
	cmd := &cobra.Command{
		Use:   "parallel",
		Short: "Generate a workflow with one parallel subworkflow stage",
		Long: `Generate a workflow definition with a normal starting step, one parallel
stage, and optional final human review.

Each --branch value uses "title=actor_role" or "title:actor_role". The branch is
generated as a child subworkflow, so it can later be expanded into multiple
serial steps without changing the parent workflow shape.`,
		Example: `  multigent workflow scaffold parallel \
    --name "Frontend and backend design" \
    --start-role pm \
    --branch "Frontend technical spec=frontend_engineer" \
    --branch "Backend technical spec=backend_engineer" \
    --final-review-role tech_lead \
    --out workflow.json

  multigent workflow scaffold parallel \
    --name "Market research split" \
    --branch "Customer research=researcher" \
    --branch "Competitor research=analyst" \
    --join any \
    --save`,
		RunE: func(cmd *cobra.Command, args []string) error {
			def, err := scaffoldParallelWorkflow(scaffoldParallelWorkflowOptions{
				Name:            name,
				Description:     description,
				StartRole:       startRole,
				StartTitle:      startTitle,
				StageTitle:      stageTitle,
				JoinPolicy:      joinPolicy,
				FinalReviewRole: finalReviewRole,
				Branches:        branches,
			})
			if err != nil {
				return err
			}
			if err := validateWorkflowDefinition(def); err != nil {
				return err
			}
			if save {
				ctx, err := openCLIWorkspaceDB(workspaceRef)
				if err != nil {
					return err
				}
				defer ctx.Close()
				if err := workflowstore.NewStore(ctx.db, ctx.workspaceID).SaveDefinition(&def); err != nil {
					return err
				}
			}
			return writeJSONFile(out, def)
		},
	}
	cmd.Flags().StringVar(&workspaceRef, "workspace", "", "workspace id, name, slug, or root path, used with --save")
	cmd.Flags().StringVar(&name, "name", "", "workflow name")
	cmd.Flags().StringVar(&description, "description", "", "workflow description")
	cmd.Flags().StringVar(&startRole, "start-role", "pm", "actor role for the first preparation step")
	cmd.Flags().StringVar(&startTitle, "start-title", "Prepare workflow input", "title for the first preparation step")
	cmd.Flags().StringVar(&stageTitle, "stage-title", "Run parallel subflows", "title for the parallel stage")
	cmd.Flags().StringVar(&joinPolicy, "join", "all", "parallel join policy: all or any")
	cmd.Flags().StringVar(&finalReviewRole, "final-review-role", "", "optional human reviewer role after all required branches finish")
	cmd.Flags().StringArrayVar(&branches, "branch", nil, "parallel branch as title=actor_role, repeatable")
	cmd.Flags().StringVar(&out, "out", "", "write JSON to file instead of stdout")
	cmd.Flags().BoolVar(&save, "save", false, "save the generated workflow into the workspace")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("branch")
	return cmd
}

func loadWorkflowDefinition(workspaceRef, id string) (entity.WorkflowDefinition, error) {
	ctx, err := openCLIWorkspaceDB(workspaceRef)
	if err != nil {
		return entity.WorkflowDefinition{}, err
	}
	defer ctx.Close()
	def, ok, err := workflowstore.NewStore(ctx.db, ctx.workspaceID).Definition(id)
	if err != nil {
		return entity.WorkflowDefinition{}, err
	}
	if !ok {
		return entity.WorkflowDefinition{}, fmt.Errorf("workflow %q not found", id)
	}
	return def, nil
}

func normalizeWorkflowDefinition(def *entity.WorkflowDefinition, preserveID bool) {
	normalizeWorkflowDefinitionWithFallback(def, preserveID, "")
}

func normalizeWorkflowDefinitionWithFallback(def *entity.WorkflowDefinition, preserveID bool, fallbackID string) {
	if !preserveID && strings.TrimSpace(def.ID) == "" {
		if strings.TrimSpace(fallbackID) != "" {
			def.ID = strings.TrimSpace(fallbackID)
		} else {
			def.ID = entity.NewWorkflowID()
		}
	}
	def.ID = strings.TrimSpace(def.ID)
	def.Name = strings.TrimSpace(def.Name)
	def.Scope = "workspace"
	def.Project = ""
	if def.Version == 0 {
		def.Version = 1
	}
	if strings.TrimSpace(def.StartStepID) == "" && len(def.Steps) > 0 {
		def.StartStepID = def.Steps[0].ID
	}
	for i := range def.Steps {
		def.Steps[i].ID = strings.TrimSpace(def.Steps[i].ID)
		def.Steps[i].Title = strings.TrimSpace(def.Steps[i].Title)
		if strings.TrimSpace(def.Steps[i].Type) == "" {
			def.Steps[i].Type = "agent_task"
		}
		if def.Steps[i].Type == "parallel_stage" {
			def.Steps[i].JoinPolicy = normalizeWorkflowJoinPolicy(def.Steps[i].JoinPolicy)
		}
		for j := range def.Steps[i].Branches {
			branch := &def.Steps[i].Branches[j]
			branch.ID = strings.TrimSpace(branch.ID)
			branch.Title = strings.TrimSpace(branch.Title)
			branch.ActorRole = strings.TrimSpace(branch.ActorRole)
			if branch.Workflow != nil {
				fallback := strings.Trim(strings.Join([]string{def.ID, def.Steps[i].ID, branch.ID, "workflow"}, "-"), "-")
				normalizeWorkflowDefinitionWithFallback(branch.Workflow, false, fallback)
			}
		}
	}
}

func validateWorkflowDefinition(def entity.WorkflowDefinition) error {
	return validateWorkflowDefinitionAt(def, "workflow")
}

func validateWorkflowDefinitionAt(def entity.WorkflowDefinition, path string) error {
	if strings.TrimSpace(def.Name) == "" {
		return fmt.Errorf("%s name is required", path)
	}
	if len(def.Steps) == 0 {
		return fmt.Errorf("%s must contain at least one step", path)
	}
	stepIDs := map[string]bool{}
	for _, step := range def.Steps {
		if step.ID == "" {
			return fmt.Errorf("%s step id is required", path)
		}
		if step.Title == "" {
			return fmt.Errorf("%s step %q title is required", path, step.ID)
		}
		if stepIDs[step.ID] {
			return fmt.Errorf("%s has duplicate step id %q", path, step.ID)
		}
		stepIDs[step.ID] = true
		if step.Type == "parallel_stage" {
			if policy := normalizeWorkflowJoinPolicy(step.JoinPolicy); policy != "all" && policy != "any" {
				return fmt.Errorf("%s parallel step %q has invalid joinPolicy %q", path, step.ID, step.JoinPolicy)
			}
			if len(step.Branches) == 0 {
				return fmt.Errorf("%s parallel step %q must include at least one branch", path, step.ID)
			}
			branchIDs := map[string]bool{}
			for _, branch := range step.Branches {
				if strings.TrimSpace(branch.ID) == "" {
					return fmt.Errorf("%s parallel step %q branch id is required", path, step.ID)
				}
				if strings.TrimSpace(branch.Title) == "" {
					return fmt.Errorf("%s parallel step %q branch %q title is required", path, step.ID, branch.ID)
				}
				if branchIDs[branch.ID] {
					return fmt.Errorf("%s parallel step %q has duplicate branch id %q", path, step.ID, branch.ID)
				}
				branchIDs[branch.ID] = true
				if branch.Workflow != nil {
					if err := validateWorkflowDefinitionAt(*branch.Workflow, path+"."+step.ID+"."+branch.ID); err != nil {
						return err
					}
				}
			}
		}
	}
	if !stepIDs[def.StartStepID] {
		return fmt.Errorf("%s startStepId %q does not match any step", path, def.StartStepID)
	}
	for _, edge := range def.Edges {
		if edge.From == "" {
			return fmt.Errorf("%s edge must include from", path)
		}
		if !stepIDs[edge.From] {
			return fmt.Errorf("%s edge references missing from step %q", path, edge.From)
		}
		if edge.To != "" && !stepIDs[edge.To] {
			return fmt.Errorf("%s edge references missing to step %q", path, edge.To)
		}
	}
	return nil
}

func printWorkflowSummary(def entity.WorkflowDefinition) {
	fmt.Printf("ID: %s\n", def.ID)
	fmt.Printf("Name: %s\n", def.Name)
	if def.Description != "" {
		fmt.Printf("Description: %s\n", def.Description)
	}
	fmt.Printf("Start step: %s\n", def.StartStepID)
	fmt.Printf("Steps: %d\n", len(def.Steps))
	for _, step := range def.Steps {
		fmt.Printf("- %s [%s] %s\n", step.ID, step.Type, step.Title)
	}
}

func addWorkspaceAndFormatFlags(cmd *cobra.Command, workspaceRef *string, format *string) {
	cmd.Flags().StringVar(workspaceRef, "workspace", "", "workspace id, name, slug, or root path")
	cmd.Flags().StringVar(format, "format", "", "output format: json or table")
}

func formatWorkflowTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04")
}

type scaffoldParallelWorkflowOptions struct {
	Name            string
	Description     string
	StartRole       string
	StartTitle      string
	StageTitle      string
	JoinPolicy      string
	FinalReviewRole string
	Branches        []string
}

func scaffoldParallelWorkflow(opts scaffoldParallelWorkflowOptions) (entity.WorkflowDefinition, error) {
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		return entity.WorkflowDefinition{}, fmt.Errorf("workflow name is required")
	}
	branches, err := parseScaffoldBranches(opts.Branches)
	if err != nil {
		return entity.WorkflowDefinition{}, err
	}
	joinPolicy := normalizeWorkflowJoinPolicy(opts.JoinPolicy)
	if joinPolicy != "all" && joinPolicy != "any" {
		return entity.WorkflowDefinition{}, fmt.Errorf("--join must be all or any")
	}
	startRole := strings.TrimSpace(opts.StartRole)
	if startRole == "" {
		startRole = "pm"
	}
	startTitle := strings.TrimSpace(opts.StartTitle)
	if startTitle == "" {
		startTitle = "Prepare workflow input"
	}
	stageTitle := strings.TrimSpace(opts.StageTitle)
	if stageTitle == "" {
		stageTitle = "Run parallel subflows"
	}
	now := time.Now().UTC()
	def := entity.WorkflowDefinition{
		ID:          entity.NewWorkflowID(),
		Name:        name,
		Description: strings.TrimSpace(opts.Description),
		Version:     1,
		Scope:       "workspace",
		StartStepID: "prepare_input",
		CreatedAt:   now,
		UpdatedAt:   now,
		Steps: []entity.WorkflowStep{
			{
				ID:          "prepare_input",
				Type:        "agent_task",
				Title:       startTitle,
				Description: "Clarify the original request and prepare structured input for the parallel subflows.",
				ActorRole:   startRole,
				OutputFields: []entity.WorkflowField{
					{Name: "brief_doc_id", Description: "Knowledge docID for the shared brief."},
					{Name: "summary", Description: "Short summary of the work to split."},
				},
				Position: entity.WorkflowPosition{X: 80, Y: 160},
			},
			{
				ID:          "parallel_subflows",
				Type:        "parallel_stage",
				Title:       stageTitle,
				Description: "Start every configured child subflow. This stage aggregates child outputs when the join policy is satisfied.",
				InputFields: []entity.WorkflowField{
					{Name: "brief_doc_id", Description: "Knowledge docID for the shared brief."},
					{Name: "summary", Description: "Short summary of the work to split."},
				},
				Branches:   branches,
				JoinPolicy: joinPolicy,
				Position:   entity.WorkflowPosition{X: 420, Y: 160},
			},
		},
		Edges: []entity.WorkflowEdge{
			{ID: "e-prepare_input-parallel_subflows", From: "prepare_input", To: "parallel_subflows"},
		},
	}
	if reviewRole := strings.TrimSpace(opts.FinalReviewRole); reviewRole != "" {
		def.Steps = append(def.Steps, entity.WorkflowStep{
			ID:          "final_review",
			Type:        "human_review",
			Title:       "Review aggregated branch outputs",
			Description: "Review the combined outputs from all required parallel branches.",
			ActorRole:   reviewRole,
			OutputFields: []entity.WorkflowField{
				{Name: "decision", Description: "Decision: approve or request_changes."},
				{Name: "comments", Description: "Review comments or approval note."},
			},
			Position: entity.WorkflowPosition{X: 760, Y: 160},
		})
		def.Edges = append(def.Edges, entity.WorkflowEdge{ID: "e-parallel_subflows-final_review", From: "parallel_subflows", To: "final_review"})
	}
	return def, nil
}

func parseScaffoldBranches(items []string) ([]entity.WorkflowBranch, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("at least one --branch is required")
	}
	out := make([]entity.WorkflowBranch, 0, len(items))
	seen := map[string]int{}
	for i, item := range items {
		title, role, err := parseScaffoldBranch(item)
		if err != nil {
			return nil, err
		}
		id := slugID(title)
		if id == "" {
			id = fmt.Sprintf("branch_%d", i+1)
		}
		if count := seen[id]; count > 0 {
			seen[id] = count + 1
			id = fmt.Sprintf("%s_%d", id, count+1)
		} else {
			seen[id] = 1
		}
		workflowID := id + "_subflow"
		out = append(out, entity.WorkflowBranch{
			ID:          id,
			Title:       title,
			Description: "Run this child subflow independently and return structured outputs to the parent stage.",
			ActorRole:   role,
			InputFields: []entity.WorkflowField{
				{Name: "brief_doc_id", Description: "Knowledge docID for the shared brief."},
				{Name: "summary", Description: "Short summary from the parent preparation step."},
			},
			OutputFields: []entity.WorkflowField{
				{Name: "result_doc_id", Description: "Knowledge docID for this branch result."},
				{Name: "summary", Description: "Short summary of this branch result."},
			},
			Workflow: &entity.WorkflowDefinition{
				ID:          workflowID,
				Name:        title,
				Description: "Single-step child subflow generated by multigent workflow scaffold parallel.",
				Version:     1,
				Scope:       "workspace",
				StartStepID: "work",
				Steps: []entity.WorkflowStep{
					{
						ID:          "work",
						Type:        "agent_task",
						Title:       title,
						Description: "Complete this branch and return the required structured outputs.",
						ActorRole:   role,
						InputFields: []entity.WorkflowField{
							{Name: "brief_doc_id", Description: "Knowledge docID for the shared brief."},
							{Name: "summary", Description: "Short summary from the parent preparation step."},
						},
						OutputFields: []entity.WorkflowField{
							{Name: "result_doc_id", Description: "Knowledge docID for this branch result."},
							{Name: "summary", Description: "Short summary of this branch result."},
						},
						Position: entity.WorkflowPosition{X: 80, Y: 120},
					},
				},
				Edges: []entity.WorkflowEdge{},
			},
		})
	}
	return out, nil
}

func parseScaffoldBranch(item string) (string, string, error) {
	item = strings.TrimSpace(item)
	if item == "" {
		return "", "", fmt.Errorf("--branch cannot be empty")
	}
	sep := strings.Index(item, "=")
	if sep < 0 {
		sep = strings.LastIndex(item, ":")
	}
	if sep < 0 {
		return "", "", fmt.Errorf("--branch %q must use title=actor_role", item)
	}
	title := strings.TrimSpace(item[:sep])
	role := strings.TrimSpace(item[sep+1:])
	if title == "" || role == "" {
		return "", "", fmt.Errorf("--branch %q must include both title and actor_role", item)
	}
	return title, role, nil
}

func normalizeWorkflowJoinPolicy(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "all"
	}
	return value
}

var workflowIDInvalid = regexp.MustCompile(`[^a-z0-9_]+`)

func slugID(value string) string {
	out := strings.ToLower(strings.TrimSpace(value))
	out = strings.ReplaceAll(out, "-", "_")
	out = strings.ReplaceAll(out, " ", "_")
	out = workflowIDInvalid.ReplaceAllString(out, "_")
	out = strings.Trim(out, "_")
	return out
}
