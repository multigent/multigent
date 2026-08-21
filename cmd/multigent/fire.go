package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newFireCmd() *cobra.Command {
	var (
		project   string
		agentName string
		force     bool
	)

	cmd := &cobra.Command{
		Use:   "fire",
		Short: "Fire (remove) an agent from a project",
		Long: `fire removes an Agent Worker membership from a project.

The Agent Worker itself remains available at workspace level and can be assigned
to other projects. Delete the workspace-level Agent Worker separately when it is
no longer needed.`,
		Example: `  # Soft-delete — recoverable
  multigent fire --project my-api --agent dev

  # Hard delete — permanent, cannot be undone
  multigent fire --project my-api --agent dev --force

  # From outside the workspace
  multigent --dir /path/to/Agency fire --project my-api --agent dev`,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := resolveRoot()
			if err != nil {
				return err
			}
			_, ok, db, workspaceID, err := resolveCLIProjectWorker(root, project, agentName)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("agent worker membership %s/%s not found", project, agentName)
			}
			resolved, ok, err := db.ProjectMembershipByID(workspaceID, "pm_"+agentName)
			if err != nil || !ok || resolved.ProjectID != project {
				workers, listErr := listCLIProjectWorkers(root, project)
				if listErr != nil {
					return listErr
				}
				for _, worker := range workers {
					if worker.Name == agentName {
						resolved = worker.Membership
						ok = true
						break
					}
				}
			}
			if !ok {
				return fmt.Errorf("project membership for %s/%s not found", project, agentName)
			}
			if err := db.DeleteProjectMembership(workspaceID, resolved.ID); err != nil {
				return err
			}
			fmt.Printf("✓ Removed Agent Worker %q from project %q.\n", agentName, project)
			return nil
		},
	}

	cmd.Flags().StringVar(&project, "project", "", "Project name")
	cmd.Flags().StringVar(&agentName, "agent", "", "Name of the agent to fire")
	cmd.Flags().BoolVar(&force, "force", false, "deprecated; project removal never deletes the workspace Agent Worker")

	_ = cmd.MarkFlagRequired("project")
	_ = cmd.MarkFlagRequired("agent")

	return cmd
}
