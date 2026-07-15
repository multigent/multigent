package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newTeamCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "team",
		Short: "Manage team definitions",
	}
	cmd.AddCommand(
		newTeamTreeCmd(),
		newTeamShowCmd(),
		newTeamSkillCmd(),
	)
	return cmd
}

func newTeamTreeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tree",
		Short: "List teams",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := resolveRoot()
			if err != nil {
				return err
			}
			s := mustStore(root)
			teams, err := s.ListTeams()
			if err != nil {
				return err
			}
			if len(teams) == 0 {
				fmt.Println("No teams found.")
				return nil
			}
			sort.Slice(teams, func(i, j int) bool { return teams[i].Path < teams[j].Path })
			for _, entry := range teams {
				fmt.Printf("- %s", entry.Path)
				if len(entry.Team.Owners) > 0 {
					fmt.Printf("  owners=%s", strings.Join(entry.Team.Owners, ","))
				}
				if len(entry.Team.Skills) > 0 {
					fmt.Printf("  skills=%s", strings.Join(entry.Team.Skills, ","))
				}
				fmt.Println()
			}
			return nil
		},
	}
	return cmd
}

func newTeamShowCmd() *cobra.Command {
	var teamPath string
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show team ownership and context information",
		RunE: func(cmd *cobra.Command, args []string) error {
			if teamPath == "" {
				return fmt.Errorf("--team is required")
			}
			root, err := resolveRoot()
			if err != nil {
				return err
			}
			s := mustStore(root)
			t, err := s.Team(teamPath)
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "Path:\t%s\n", teamPath)
			fmt.Fprintf(w, "Name:\t%s\n", t.Name)
			fmt.Fprintf(w, "Owners:\t%s\n", joinOrDash(t.Owners))
			fmt.Fprintf(w, "Skills:\t%s\n", joinOrDash(t.Skills))
			fmt.Fprintf(w, "Default context:\t%s\n", emptyDash(t.DefaultContextPack))
			fmt.Fprintf(w, "Description:\t%s\n", emptyDash(t.Description))
			w.Flush()
			return nil
		},
	}
	cmd.Flags().StringVar(&teamPath, "team", "", "team name, e.g. engineering")
	_ = cmd.MarkFlagRequired("team")
	return cmd
}

// ── team skill ────────────────────────────────────────────────────────────────

func newTeamSkillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Add or remove skills bound to a team",
	}
	cmd.AddCommand(
		newTeamSkillAddCmd(),
		newTeamSkillRemoveCmd(),
	)
	return cmd
}

func newTeamSkillAddCmd() *cobra.Command {
	var teamPath string
	var skills []string

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Bind one or more skills to a team",
		Example: `  multigent team skill add --team growth --skill article-publisher
  multigent team skill add --team engineering --skill github-push-relay,docker`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if teamPath == "" || len(skills) == 0 {
				return fmt.Errorf("--team and --skill are all required")
			}
			root, err := resolveRoot()
			if err != nil {
				return err
			}
			s := mustStore(root)

			t, err := s.Team(teamPath)
			if err != nil {
				return err
			}

			added := 0
			existing := make(map[string]bool, len(t.Skills))
			for _, sk := range t.Skills {
				existing[sk] = true
			}
			for _, sk := range skills {
				sk = strings.TrimSpace(sk)
				if sk == "" || existing[sk] {
					continue
				}
				t.Skills = append(t.Skills, sk)
				existing[sk] = true
				added++
			}

			if added == 0 {
				fmt.Printf("ℹ  No new skills to add (all already bound to team %q).\n", teamPath)
				return nil
			}

			if err := s.SaveTeam(teamPath, t); err != nil {
				return err
			}

			fmt.Printf("✓ Added %d skill(s) to team %q\n", added, teamPath)
			fmt.Printf("  Skills now: %s\n", strings.Join(t.Skills, ", "))
			fmt.Printf("\n  Run `multigent sync` to push the updated context to hired agents.\n")
			return nil
		},
	}

	cmd.Flags().StringVar(&teamPath, "team", "", "Team name, e.g. \"growth\" or \"engineering\"")
	cmd.Flags().StringSliceVar(&skills, "skill", nil, "Skill name(s) to add (comma-separated or repeated flag)")
	_ = cmd.MarkFlagRequired("team")
	_ = cmd.MarkFlagRequired("skill")
	return cmd
}

func newTeamSkillRemoveCmd() *cobra.Command {
	var teamPath string
	var skills []string

	cmd := &cobra.Command{
		Use:     "remove",
		Short:   "Unbind one or more skills from a team",
		Example: `  multigent team skill remove --team growth --skill article-publisher`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if teamPath == "" || len(skills) == 0 {
				return fmt.Errorf("--team and --skill are all required")
			}
			root, err := resolveRoot()
			if err != nil {
				return err
			}
			s := mustStore(root)

			t, err := s.Team(teamPath)
			if err != nil {
				return err
			}

			remove := make(map[string]bool, len(skills))
			for _, sk := range skills {
				remove[strings.TrimSpace(sk)] = true
			}

			before := len(t.Skills)
			kept := t.Skills[:0]
			for _, sk := range t.Skills {
				if !remove[sk] {
					kept = append(kept, sk)
				}
			}
			t.Skills = kept
			removed := before - len(t.Skills)

			if removed == 0 {
				fmt.Printf("ℹ  None of the specified skills were bound to team %q.\n", teamPath)
				return nil
			}

			if err := s.SaveTeam(teamPath, t); err != nil {
				return err
			}

			fmt.Printf("✓ Removed %d skill(s) from team %q\n", removed, teamPath)
			if len(t.Skills) > 0 {
				fmt.Printf("  Skills now: %s\n", strings.Join(t.Skills, ", "))
			} else {
				fmt.Printf("  Skills now: (none)\n")
			}
			fmt.Printf("\n  Run `multigent sync` to push the updated context to hired agents.\n")
			return nil
		},
	}

	cmd.Flags().StringVar(&teamPath, "team", "", "Team name, e.g. \"growth\" or \"engineering\"")
	cmd.Flags().StringSliceVar(&skills, "skill", nil, "Skill name(s) to remove (comma-separated or repeated flag)")
	_ = cmd.MarkFlagRequired("team")
	_ = cmd.MarkFlagRequired("skill")
	return cmd
}
