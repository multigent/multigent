package main

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/store"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newWorkstreamCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workstream",
		Short: "Manage project workstreams",
		Long: `A workstream is a feature, module, delivery stream, release, or technical
initiative inside a project. It is larger than a task and smaller than a
project, and records ownership, scope, context references, and participating
agents.`,
	}
	cmd.AddCommand(
		newWorkstreamCreateCmd(),
		newWorkstreamListCmd(),
		newWorkstreamShowCmd(),
		newWorkstreamAgentCmd(),
	)
	return cmd
}

func newWorkstreamCreateCmd() *cobra.Command {
	var (
		project            string
		name               string
		title              string
		goal               string
		scope              string
		phase              string
		status             string
		owners             []string
		nonGoals           []string
		acceptanceCriteria []string
		sourceRefs         []string
		contextRefs        []string
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a workstream under a project",
		Example: `  multigent workstream create --project tapnow-agent --name plugin-connector \
    --title "Plugin and Connector" --goal "Ship plugin/connector capability" \
    --owner dashell --context-ref docs/prd.md --source-ref lark://doc/...`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if project == "" || name == "" {
				return fmt.Errorf("--project and --name are required")
			}
			root, err := resolveRoot()
			if err != nil {
				return err
			}
			s := store.NewFS(root)
			if _, err := s.Project(project); err != nil {
				return err
			}
			if _, err := s.Workstream(project, name); err == nil {
				return fmt.Errorf("workstream %q already exists in project %q", name, project)
			}
			now := time.Now()
			if status == "" {
				status = "active"
			}
			ws := &entity.Workstream{
				Name:               name,
				Project:            project,
				Title:              title,
				Goal:               goal,
				Scope:              scope,
				NonGoals:           nonGoals,
				AcceptanceCriteria: acceptanceCriteria,
				Owners:             owners,
				Phase:              phase,
				Status:             status,
				SourceRefs:         sourceRefs,
				ContextRefs:        contextRefs,
				CreatedAt:          &now,
				UpdatedAt:          &now,
			}
			if err := s.SaveWorkstream(project, name, ws); err != nil {
				return err
			}
			fmt.Printf("✓ Workstream created: projects/%s/workstreams/%s\n", project, name)
			fmt.Printf("  Add agent responsibility with: multigent workstream agent add --project %s --workstream %s --name <lane> --agent <agent> --owner <human>\n", project, name)
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project name")
	cmd.Flags().StringVar(&name, "name", "", "workstream name")
	cmd.Flags().StringVar(&title, "title", "", "human-readable title")
	cmd.Flags().StringVar(&goal, "goal", "", "desired outcome")
	cmd.Flags().StringVar(&scope, "scope", "", "scope description")
	cmd.Flags().StringVar(&phase, "phase", "", "phase, e.g. discovery, design, build, qa, release")
	cmd.Flags().StringVar(&status, "status", "active", "status, e.g. active, paused, done")
	cmd.Flags().StringSliceVar(&owners, "owner", nil, "human owner(s)")
	cmd.Flags().StringSliceVar(&nonGoals, "non-goal", nil, "explicit non-goal(s)")
	cmd.Flags().StringSliceVar(&acceptanceCriteria, "acceptance", nil, "acceptance criteria")
	cmd.Flags().StringSliceVar(&sourceRefs, "source-ref", nil, "source references such as docs, tickets, meeting notes")
	cmd.Flags().StringSliceVar(&contextRefs, "context-ref", nil, "context pack/file references")
	_ = cmd.MarkFlagRequired("project")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func newWorkstreamListCmd() *cobra.Command {
	var project string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workstreams in a project",
		RunE: func(cmd *cobra.Command, args []string) error {
			if project == "" {
				return fmt.Errorf("--project is required")
			}
			root, err := resolveRoot()
			if err != nil {
				return err
			}
			s := store.NewFS(root)
			items, err := s.ListWorkstreams(project)
			if err != nil {
				return err
			}
			sort.Slice(items, func(i, j int) bool {
				return items[i].Name < items[j].Name
			})
			if len(items) == 0 {
				fmt.Printf("No workstreams found for project %q.\n", project)
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tSTATUS\tPHASE\tOWNERS\tTITLE")
			fmt.Fprintln(w, "────\t──────\t─────\t──────\t─────")
			for _, item := range items {
				ws := item.Workstream
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					item.Name,
					emptyDash(ws.Status),
					emptyDash(ws.Phase),
					joinOrDash(ws.Owners),
					emptyDash(ws.Title),
				)
			}
			w.Flush()
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project name")
	_ = cmd.MarkFlagRequired("project")
	return cmd
}

func newWorkstreamShowCmd() *cobra.Command {
	var (
		project string
		name    string
		asYAML  bool
	)
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show a workstream",
		RunE: func(cmd *cobra.Command, args []string) error {
			if project == "" || name == "" {
				return fmt.Errorf("--project and --name are required")
			}
			root, err := resolveRoot()
			if err != nil {
				return err
			}
			s := store.NewFS(root)
			ws, err := s.Workstream(project, name)
			if err != nil {
				return err
			}
			if asYAML {
				data, err := yaml.Marshal(ws)
				if err != nil {
					return err
				}
				fmt.Print(string(data))
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "Project:\t%s\n", project)
			fmt.Fprintf(w, "Name:\t%s\n", ws.Name)
			fmt.Fprintf(w, "Title:\t%s\n", emptyDash(ws.Title))
			fmt.Fprintf(w, "Status:\t%s\n", emptyDash(ws.Status))
			fmt.Fprintf(w, "Phase:\t%s\n", emptyDash(ws.Phase))
			fmt.Fprintf(w, "Owners:\t%s\n", joinOrDash(ws.Owners))
			fmt.Fprintf(w, "Goal:\t%s\n", emptyDash(ws.Goal))
			fmt.Fprintf(w, "Scope:\t%s\n", emptyDash(ws.Scope))
			fmt.Fprintf(w, "Context refs:\t%s\n", joinOrDash(ws.ContextRefs))
			fmt.Fprintf(w, "Source refs:\t%s\n", joinOrDash(ws.SourceRefs))
			if len(ws.Agents) > 0 {
				fmt.Fprintln(w, "\nAGENT\tTEAM\tROLE\tOWNER\tRESPONSIBILITY")
				for _, a := range ws.Agents {
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
						emptyDash(firstNonEmpty(a.Agent, a.Name)),
						emptyDash(a.Team),
						emptyDash(a.Role),
						emptyDash(a.Owner),
						emptyDash(a.Responsibility),
					)
				}
			}
			w.Flush()
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project name")
	cmd.Flags().StringVar(&name, "name", "", "workstream name")
	cmd.Flags().BoolVar(&asYAML, "yaml", false, "print raw YAML")
	_ = cmd.MarkFlagRequired("project")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func newWorkstreamAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage agent assignments inside a workstream",
	}
	cmd.AddCommand(newWorkstreamAgentAddCmd())
	return cmd
}

func newWorkstreamAgentAddCmd() *cobra.Command {
	var (
		project        string
		workstream     string
		name           string
		team           string
		role           string
		agent          string
		owner          string
		responsibility string
	)
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add or update an agent responsibility lane",
		RunE: func(cmd *cobra.Command, args []string) error {
			if project == "" || workstream == "" || name == "" {
				return fmt.Errorf("--project, --workstream, and --name are required")
			}
			root, err := resolveRoot()
			if err != nil {
				return err
			}
			s := store.NewFS(root)
			ws, err := s.Workstream(project, workstream)
			if err != nil {
				return err
			}
			assignment := entity.WorkstreamAgentAssignment{
				Name:           name,
				Team:           team,
				Role:           role,
				Agent:          agent,
				Owner:          owner,
				Responsibility: responsibility,
			}
			replaced := false
			for i := range ws.Agents {
				if ws.Agents[i].Name == name {
					ws.Agents[i] = assignment
					replaced = true
					break
				}
			}
			if !replaced {
				ws.Agents = append(ws.Agents, assignment)
			}
			now := time.Now()
			ws.UpdatedAt = &now
			if err := s.SaveWorkstream(project, workstream, ws); err != nil {
				return err
			}
			if replaced {
				fmt.Printf("✓ Updated workstream agent lane %q\n", name)
			} else {
				fmt.Printf("✓ Added workstream agent lane %q\n", name)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project name")
	cmd.Flags().StringVar(&workstream, "workstream", "", "workstream name")
	cmd.Flags().StringVar(&name, "name", "", "assignment lane name, e.g. frontend, backend, qa")
	cmd.Flags().StringVar(&team, "team", "", "team path")
	cmd.Flags().StringVar(&role, "role", "", "role name")
	cmd.Flags().StringVar(&agent, "agent", "", "concrete agent name")
	cmd.Flags().StringVar(&owner, "owner", "", "human owner responsible for this agent's performance")
	cmd.Flags().StringVar(&responsibility, "responsibility", "", "what this lane owns")
	_ = cmd.MarkFlagRequired("project")
	_ = cmd.MarkFlagRequired("workstream")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}
