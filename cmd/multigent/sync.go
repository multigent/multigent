package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Deprecated: Agent Worker context is resolved at runtime",
		Long: `sync was used by the old project-agent directory model to regenerate
CLAUDE.md, AGENTS.md, and other runtime-specific context files.

Multigent 2.x does not keep project-local agent context directories. Agent
Worker context is resolved from workspace, team, role, project membership,
skills, and bound context sources when the agent runs.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("sync is not supported in Multigent 2.x; use Agent Worker settings and project memberships instead")
		},
	}
}
