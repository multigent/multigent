package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/multigent/multigent/internal/entity"
	tasktemplatestore "github.com/multigent/multigent/internal/tasktemplate"
	"github.com/spf13/cobra"
)

func newTaskTemplateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "task-template",
		Aliases: []string{"task-templates"},
		Short:   "Manage project task templates",
		Long: `Manage reusable project task templates.

Task templates define the title, prompt, workflow binding, variables, and
workflow actor bindings used when agents create repeatable tasks.`,
	}
	cmd.AddCommand(
		newTaskTemplateListCmd(),
		newTaskTemplateShowCmd(),
		newTaskTemplateCreateCmd(),
		newTaskTemplateUpdateCmd(),
		newTaskTemplateDeleteCmd(),
		newTaskTemplateExportCmd(),
	)
	return cmd
}

func newTaskTemplateListCmd() *cobra.Command {
	var workspaceRef, project, format string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List task templates",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := openCLIWorkspaceDB(workspaceRef)
			if err != nil {
				return err
			}
			defer ctx.Close()
			templates, err := tasktemplatestore.NewStore(ctx.db, ctx.workspaceID).List()
			if err != nil {
				return err
			}
			templates = filterTaskTemplatesByProject(templates, project)
			if resolveFormat(format) == "json" {
				if templates == nil {
					templates = []entity.TaskTemplate{}
				}
				return printJSON(templates)
			}
			if len(templates) == 0 {
				fmt.Println("No task templates found. Run: multigent task-template create --file template.json")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tPROJECT\tNAME\tWORKFLOW\tUPDATED")
			for _, tmpl := range templates {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					tmpl.ID,
					emptyDash(tmpl.Project),
					tmpl.Name,
					emptyDash(tmpl.WorkflowDefinitionID),
					formatTaskTemplateTime(tmpl.UpdatedAt),
				)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&workspaceRef, "workspace", "", "workspace id, name, slug, or root path")
	cmd.Flags().StringVar(&project, "project", "", "filter by project")
	cmd.Flags().StringVar(&format, "format", "", "output format: json or table")
	return cmd
}

func newTaskTemplateShowCmd() *cobra.Command {
	var workspaceRef, format string
	cmd := &cobra.Command{
		Use:   "show <template-id>",
		Short: "Show one task template",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tmpl, err := loadTaskTemplate(workspaceRef, args[0])
			if err != nil {
				return err
			}
			if resolveFormat(format) == "json" {
				return printJSON(tmpl)
			}
			printTaskTemplateSummary(tmpl)
			return nil
		},
	}
	cmd.Flags().StringVar(&workspaceRef, "workspace", "", "workspace id, name, slug, or root path")
	cmd.Flags().StringVar(&format, "format", "", "output format: json or table")
	return cmd
}

func newTaskTemplateCreateCmd() *cobra.Command {
	var (
		workspaceRef string
		file         string
		project      string
	)
	cmd := &cobra.Command{
		Use:   "create --file template.json",
		Short: "Create a task template from JSON",
		Example: `  multigent task-template create --project web-app --file issue-task-template.json
  multigent task-template create --file - < issue-task-template.json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := openCLIWorkspaceDB(workspaceRef)
			if err != nil {
				return err
			}
			defer ctx.Close()
			var tmpl entity.TaskTemplate
			if err := readJSONFile(file, &tmpl); err != nil {
				return err
			}
			normalizeTaskTemplate(&tmpl, false)
			if strings.TrimSpace(project) != "" {
				tmpl.Project = strings.TrimSpace(project)
			}
			if err := validateTaskTemplate(tmpl); err != nil {
				return err
			}
			if err := tasktemplatestore.NewStore(ctx.db, ctx.workspaceID).Save(&tmpl); err != nil {
				return err
			}
			return printJSON(tmpl)
		},
	}
	cmd.Flags().StringVar(&workspaceRef, "workspace", "", "workspace id, name, slug, or root path")
	cmd.Flags().StringVar(&file, "file", "", "task template JSON file, or '-' for stdin")
	cmd.Flags().StringVar(&project, "project", "", "project name; overrides the JSON project field")
	return cmd
}

func newTaskTemplateUpdateCmd() *cobra.Command {
	var workspaceRef, file string
	cmd := &cobra.Command{
		Use:   "update <template-id> --file template.json",
		Short: "Replace a task template from JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := openCLIWorkspaceDB(workspaceRef)
			if err != nil {
				return err
			}
			defer ctx.Close()
			store := tasktemplatestore.NewStore(ctx.db, ctx.workspaceID)
			existing, ok, err := store.Get(args[0])
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("task template %q not found", args[0])
			}
			var tmpl entity.TaskTemplate
			if err := readJSONFile(file, &tmpl); err != nil {
				return err
			}
			tmpl.ID = existing.ID
			tmpl.CreatedAt = existing.CreatedAt
			normalizeTaskTemplate(&tmpl, true)
			if err := validateTaskTemplate(tmpl); err != nil {
				return err
			}
			if err := store.Save(&tmpl); err != nil {
				return err
			}
			return printJSON(tmpl)
		},
	}
	cmd.Flags().StringVar(&workspaceRef, "workspace", "", "workspace id, name, slug, or root path")
	cmd.Flags().StringVar(&file, "file", "", "task template JSON file, or '-' for stdin")
	return cmd
}

func newTaskTemplateDeleteCmd() *cobra.Command {
	var workspaceRef string
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete <template-id>",
		Short: "Delete a task template",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				return fmt.Errorf("refusing to delete task template %q without --yes", args[0])
			}
			ctx, err := openCLIWorkspaceDB(workspaceRef)
			if err != nil {
				return err
			}
			defer ctx.Close()
			store := tasktemplatestore.NewStore(ctx.db, ctx.workspaceID)
			if _, ok, err := store.Get(args[0]); err != nil {
				return err
			} else if !ok {
				return fmt.Errorf("task template %q not found", args[0])
			}
			if err := store.Delete(args[0]); err != nil {
				return err
			}
			return printJSON(map[string]string{"deleted": args[0]})
		},
	}
	cmd.Flags().StringVar(&workspaceRef, "workspace", "", "workspace id, name, slug, or root path")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm deletion")
	return cmd
}

func newTaskTemplateExportCmd() *cobra.Command {
	var workspaceRef, out string
	cmd := &cobra.Command{
		Use:   "export <template-id>",
		Short: "Export a task template as JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tmpl, err := loadTaskTemplate(workspaceRef, args[0])
			if err != nil {
				return err
			}
			return writeJSONFile(out, tmpl)
		},
	}
	cmd.Flags().StringVar(&workspaceRef, "workspace", "", "workspace id, name, slug, or root path")
	cmd.Flags().StringVar(&out, "out", "", "write JSON to file instead of stdout")
	return cmd
}

func loadTaskTemplate(workspaceRef, id string) (entity.TaskTemplate, error) {
	ctx, err := openCLIWorkspaceDB(workspaceRef)
	if err != nil {
		return entity.TaskTemplate{}, err
	}
	defer ctx.Close()
	tmpl, ok, err := tasktemplatestore.NewStore(ctx.db, ctx.workspaceID).Get(id)
	if err != nil {
		return entity.TaskTemplate{}, err
	}
	if !ok {
		return entity.TaskTemplate{}, fmt.Errorf("task template %q not found", id)
	}
	return tmpl, nil
}

func normalizeTaskTemplate(tmpl *entity.TaskTemplate, preserveID bool) {
	if !preserveID && strings.TrimSpace(tmpl.ID) == "" {
		tmpl.ID = entity.NewTaskTemplateID()
	}
	tmpl.ID = strings.TrimSpace(tmpl.ID)
	tmpl.Name = strings.TrimSpace(tmpl.Name)
	tmpl.Project = strings.TrimSpace(tmpl.Project)
	tmpl.Type = strings.TrimSpace(tmpl.Type)
	if tmpl.Type == "" {
		tmpl.Type = string(entity.TaskTypeChore)
	}
	if tmpl.Priority < 0 || tmpl.Priority > 3 {
		tmpl.Priority = 2
	}
	tmpl.TitleTemplate = strings.TrimSpace(tmpl.TitleTemplate)
	tmpl.PromptTemplate = strings.TrimSpace(tmpl.PromptTemplate)
}

func validateTaskTemplate(tmpl entity.TaskTemplate) error {
	if tmpl.Name == "" {
		return fmt.Errorf("task template name is required")
	}
	if tmpl.Project == "" {
		return fmt.Errorf("task template project is required")
	}
	if tmpl.TitleTemplate == "" {
		return fmt.Errorf("task template titleTemplate is required")
	}
	if tmpl.PromptTemplate == "" {
		return fmt.Errorf("task template promptTemplate is required")
	}
	for stepID, binding := range tmpl.WorkflowActorBindings {
		if strings.TrimSpace(stepID) == "" {
			return fmt.Errorf("workflow actor binding step id is required")
		}
		if binding.Type != "agent" && binding.Type != "human" {
			return fmt.Errorf("workflow actor binding %q type must be agent or human", stepID)
		}
		if strings.TrimSpace(binding.ID) == "" {
			return fmt.Errorf("workflow actor binding %q id is required", stepID)
		}
	}
	return nil
}

func filterTaskTemplatesByProject(templates []entity.TaskTemplate, project string) []entity.TaskTemplate {
	project = strings.TrimSpace(project)
	if project == "" {
		return templates
	}
	out := make([]entity.TaskTemplate, 0, len(templates))
	for _, tmpl := range templates {
		if tmpl.Project == project {
			out = append(out, tmpl)
		}
	}
	return out
}

func printTaskTemplateSummary(tmpl entity.TaskTemplate) {
	fmt.Printf("ID: %s\n", tmpl.ID)
	fmt.Printf("Name: %s\n", tmpl.Name)
	fmt.Printf("Project: %s\n", tmpl.Project)
	fmt.Printf("Type: %s\n", tmpl.Type)
	fmt.Printf("Priority: %d\n", tmpl.Priority)
	if tmpl.WorkflowDefinitionID != "" {
		fmt.Printf("Workflow: %s\n", tmpl.WorkflowDefinitionID)
	}
	if len(tmpl.WorkflowActorBindings) > 0 {
		fmt.Println("Actor bindings:")
		for stepID, binding := range tmpl.WorkflowActorBindings {
			fmt.Printf("- %s: %s:%s\n", stepID, binding.Type, binding.ID)
		}
	}
}

func formatTaskTemplateTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04")
}

var taskTemplateVarPatternCLI = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_.-]+)\s*\}\}`)

func renderTaskTemplateStringCLI(template string, values map[string]string) string {
	return taskTemplateVarPatternCLI.ReplaceAllStringFunc(template, func(match string) string {
		parts := taskTemplateVarPatternCLI.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		return values[strings.TrimSpace(parts[1])]
	})
}

func parseKeyValueFlags(values []string, flagName string) (map[string]string, error) {
	out := map[string]string{}
	for _, raw := range values {
		key, value, ok := strings.Cut(raw, "=")
		if !ok {
			return nil, fmt.Errorf("%s value %q must use key=value", flagName, raw)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("%s value %q has an empty key", flagName, raw)
		}
		out[key] = strings.TrimSpace(value)
	}
	return out, nil
}

func parseWorkflowActorBindings(values []string) (map[string]entity.WorkflowActorBinding, error) {
	out := map[string]entity.WorkflowActorBinding{}
	for _, raw := range values {
		key, value, ok := strings.Cut(raw, "=")
		if !ok {
			return nil, fmt.Errorf("--binding value %q must use actorRole=agent:name or actorRole=human:username", raw)
		}
		typ, id, ok := strings.Cut(strings.TrimSpace(value), ":")
		if !ok {
			return nil, fmt.Errorf("--binding value %q must use actorRole=agent:name or actorRole=human:username", raw)
		}
		key = strings.TrimSpace(key)
		typ = strings.TrimSpace(typ)
		id = strings.TrimSpace(id)
		if key == "" || id == "" {
			return nil, fmt.Errorf("--binding value %q has an empty actor role or id", raw)
		}
		if typ != "agent" && typ != "human" {
			return nil, fmt.Errorf("--binding %q type must be agent or human", raw)
		}
		out[key] = entity.WorkflowActorBinding{Type: typ, ID: id}
	}
	return out, nil
}

func mergeWorkflowActorBindings(base map[string]entity.WorkflowActorBinding, overrides map[string]entity.WorkflowActorBinding) map[string]entity.WorkflowActorBinding {
	out := map[string]entity.WorkflowActorBinding{}
	for key, value := range base {
		out[key] = value
	}
	for key, value := range overrides {
		out[key] = value
	}
	return out
}

func firstWorkflowAgentBinding(bindings map[string]entity.WorkflowActorBinding) string {
	for _, binding := range bindings {
		if binding.Type == "agent" && strings.TrimSpace(binding.ID) != "" {
			return strings.TrimSpace(binding.ID)
		}
	}
	return ""
}
