package main

import (
	"fmt"
	"os"
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
	if !preserveID && strings.TrimSpace(def.ID) == "" {
		def.ID = entity.NewWorkflowID()
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
	}
}

func validateWorkflowDefinition(def entity.WorkflowDefinition) error {
	if strings.TrimSpace(def.Name) == "" {
		return fmt.Errorf("workflow name is required")
	}
	if len(def.Steps) == 0 {
		return fmt.Errorf("workflow must contain at least one step")
	}
	stepIDs := map[string]bool{}
	for _, step := range def.Steps {
		if step.ID == "" {
			return fmt.Errorf("workflow step id is required")
		}
		if step.Title == "" {
			return fmt.Errorf("workflow step %q title is required", step.ID)
		}
		if stepIDs[step.ID] {
			return fmt.Errorf("duplicate workflow step id %q", step.ID)
		}
		stepIDs[step.ID] = true
	}
	if !stepIDs[def.StartStepID] {
		return fmt.Errorf("startStepId %q does not match any step", def.StartStepID)
	}
	for _, edge := range def.Edges {
		if edge.From == "" || edge.To == "" {
			return fmt.Errorf("workflow edge must include from and to")
		}
		if !stepIDs[edge.From] {
			return fmt.Errorf("workflow edge references missing from step %q", edge.From)
		}
		if !stepIDs[edge.To] {
			return fmt.Errorf("workflow edge references missing to step %q", edge.To)
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
