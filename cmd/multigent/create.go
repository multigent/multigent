package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/scaffold"
	tmpl "github.com/multigent/multigent/internal/template"
	"github.com/spf13/cobra"
)

func newCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create workspace objects (agency, team, role, project)",
	}
	cmd.AddCommand(
		newCreateAgencyCmd(),
		newCreateTeamCmd(),
		newCreateRoleCmd(),
		newCreateProjectCmd(),
	)
	return cmd
}

// ── create agency ─────────────────────────────────────────────────────────────

func newCreateAgencyCmd() *cobra.Command {
	var (
		name        string
		desc        string
		templateSrc string
	)

	cmd := &cobra.Command{
		Use:   "agency",
		Short: "Initialise a new multigent workspace",
		Example: `  # Blank agency
  multigent create agency --name "Acme Agency" --desc "Building the future"

  # From a local template archive
  multigent create agency --name "Acme Agency" --template tech-project.tar.gz

  # From a template directory
  multigent create agency --name "Acme Agency" --template ~/templates/tech-project

  # From a URL
  multigent create agency --name "Acme Agency" \
    --template https://github.com/multigent/multigent-templates/releases/download/v1.0.0/tech-project.tar.gz`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}

			root, err := filepath.Abs(name)
			if err != nil {
				return err
			}

			if templateSrc != "" {
				if err := applyTemplate(root, name, desc, templateSrc); err != nil {
					return err
				}
			} else {
				if err := os.MkdirAll(root, 0o755); err != nil {
					return fmt.Errorf("create agency dir: %w", err)
				}
				displayName := name
				if filepath.IsAbs(name) || strings.ContainsAny(name, `/\`) {
					displayName = filepath.Base(filepath.Clean(name))
				}
				a := &entity.Agency{Name: displayName, Description: desc}
				if err := scaffold.InitAgency(root, a); err != nil {
					return err
				}
			}

			fmt.Printf("✓ Agency workspace created: %s\n", root)
			fmt.Printf("\nNext steps:\n")
			fmt.Printf("  cd %q\n", name)
			if templateSrc == "" {
				fmt.Printf("  multigent create team --name \"engineering\"\n")
			} else {
				fmt.Printf("  multigent show\n")
				fmt.Printf("  multigent hire --project <project> --team <team> --role <role> --model claudecode --name <name>\n")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Agency name (also used as directory name)")
	cmd.Flags().StringVar(&desc, "desc", "", "Short description")
	cmd.Flags().StringVar(&templateSrc, "template", "", "template source: local .tar.gz file, directory, or HTTPS URL")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

// ── create team ───────────────────────────────────────────────────────────────

func newCreateTeamCmd() *cobra.Command {
	var (
		name               string
		desc               string
		defaultContextPack string
		owners             []string
		skills             []string
	)

	cmd := &cobra.Command{
		Use:   "team",
		Short: "Create a team",
		Example: `  multigent create team --name "engineering"
  multigent create team --name "backend" --desc "Go/gRPC" --skills "git,bash"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			if strings.Contains(name, "/") {
				return fmt.Errorf("team names are flat and cannot contain '/'; use project tasks, labels, roles, or separate teams instead")
			}

			root, err := resolveRoot()
			if err != nil {
				return err
			}
			s := mustStore(root)
			sc := scaffold.New(s)

			t := &entity.Team{
				Name:               name,
				Description:        desc,
				Owners:             owners,
				DefaultContextPack: defaultContextPack,
				Skills:             skills,
			}
			if err := sc.CreateTeam(name, t); err != nil {
				return err
			}

			fmt.Printf("✓ Team created: teams/%s\n", name)
			fmt.Printf("  Edit the prompt: vim teams/%s/prompt.md\n",
				filepath.FromSlash(name))
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Team name, e.g. \"engineering\"")
	cmd.Flags().StringVar(&desc, "desc", "", "Short description")
	cmd.Flags().StringSliceVar(&owners, "owner", nil, "Human owner(s) responsible for this team")
	cmd.Flags().StringVar(&defaultContextPack, "context-pack", "", "Default context pack for agents in this team")
	cmd.Flags().StringSliceVar(&skills, "skills", nil, "Comma-separated skill names, e.g. git,bash")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

// ── create role ───────────────────────────────────────────────────────────────

func newCreateRoleCmd() *cobra.Command {
	var (
		teamPath  string
		name      string
		desc      string
		skills    []string
		setupDirs []string
	)

	cmd := &cobra.Command{
		Use:   "role",
		Short: "Create a role definition under a team",
		Long: `create role adds a new role under teams/<team>/roles/<name>/.

A role is a reusable job template that provides:
  - An extra prompt layer (teams/<team>/roles/<name>/prompt.md)
  - Bound skills merged on top of the team's skills
  - Workspace setup: directories and files created inside the agent dir at hire time

Roles are referenced at hire time with --role.`,
		Example: `  multigent create role --team growth --name content-writer \
               --desc "Creates and publishes marketing content" \
               --skills content-writing,article-publisher \
               --setup-dirs "images,reference,generates"

  multigent create role --team engineering --name backend-dev \
               --desc "Go backend developer"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if teamPath == "" || name == "" {
				return fmt.Errorf("--team and --name are required")
			}

			root, err := resolveRoot()
			if err != nil {
				return err
			}
			s := mustStore(root)

			// Verify the team exists.
			if _, err := s.Team(teamPath); err != nil {
				return fmt.Errorf("team %q not found — create it first with: multigent create team --name %q", teamPath, teamPath)
			}

			roleDir := s.RoleDir(teamPath, name)
			if _, err := os.Stat(roleDir); err == nil {
				return fmt.Errorf("role %q already exists at %s", name, roleDir)
			}

			r := &entity.Role{
				Name:        name,
				Description: desc,
				Skills:      skills,
				Setup: entity.RoleSetup{
					Dirs: setupDirs,
				},
			}
			if err := s.SaveRole(teamPath, name, r); err != nil {
				return err
			}

			// Create an empty prompt.md stub.
			stub := fmt.Sprintf("# Role: %s\n\n", name)
			if desc != "" {
				stub += desc + "\n\n"
			}
			stub += "<!-- Describe this role's responsibilities, working style, and expectations. -->\n"
			if err := s.SaveRolePrompt(teamPath, name, stub); err != nil {
				return err
			}

			fmt.Printf("✓ Role created: teams/%s/roles/%s/\n", teamPath, name)
			fmt.Printf("  Edit the prompt:  vim teams/%s/roles/%s/prompt.md\n", teamPath, name)
			if len(skills) > 0 {
				fmt.Printf("  Bound skills:     %s\n", strings.Join(skills, ", "))
			}
			if len(setupDirs) > 0 {
				fmt.Printf("  Workspace dirs:   %s\n", strings.Join(setupDirs, ", "))
			}
			fmt.Printf("\n  Hire an agent into this role:\n")
			fmt.Printf("    multigent hire --project <project> --team %q --role %q --model claudecode --name <name>\n", teamPath, name)
			return nil
		},
	}

	cmd.Flags().StringVar(&teamPath, "team", "", "Team name the role belongs to, e.g. \"growth\" or \"engineering\"")
	cmd.Flags().StringVar(&name, "name", "", "Role name, e.g. \"content-writer\"")
	cmd.Flags().StringVar(&desc, "desc", "", "Short description of the role")
	cmd.Flags().StringSliceVar(&skills, "skills", nil, "Comma-separated skill names to bind to this role")
	cmd.Flags().StringSliceVar(&setupDirs, "setup-dirs", nil, "Comma-separated subdirectories to create in the agent workspace at hire time")
	_ = cmd.MarkFlagRequired("team")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

// ── create project ────────────────────────────────────────────────────────────

func newCreateProjectCmd() *cobra.Command {
	var (
		name        string
		desc        string
		repo        string
		blueprint   string
		contextPack string
		owners      []string
	)

	cmd := &cobra.Command{
		Use:   "project",
		Short: "Create a project (optionally from a blueprint)",
		Long: `Creates a new project directory under projects/<name>/ and writes project.yaml.

If --blueprint is provided, project.yaml is pre-populated from
project-blueprints/<blueprint>.yaml — so agents, heartbeats and workflows
are already declared.  Run 'multigent project apply' afterwards to
hire the agents and wire up their schedules.`,
		Example: `  # blank project
  multigent create project --name "my-api" --desc "REST API" --repo "../my-api"

  # from a blueprint (defines agents + heartbeats + workflows)
  multigent create project --name "my-api" --blueprint default

  # list available blueprints in this agency
  multigent project blueprints`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}

			root, err := resolveRoot()
			if err != nil {
				return err
			}
			s := mustStore(root)
			sc := scaffold.New(s)
			ts := mustTaskStore(root)

			p := &entity.Project{Name: name, Description: desc, Repo: repo, Owners: owners, ContextPack: contextPack}
			if err := sc.CreateProject(name, p); err != nil {
				return err
			}

			// Write project.yaml — either from a blueprint or as a skeleton.
			var cfg *entity.ProjectConfig
			if blueprint != "" {
				cfg, err = ts.GetProjectBlueprint(blueprint)
				if err != nil {
					return fmt.Errorf("load blueprint %q: %w", blueprint, err)
				}
				if cfg == nil {
					return fmt.Errorf("blueprint %q not found — run `multigent project blueprints` to list available ones", blueprint)
				}
				// Replace placeholder name with the actual project name.
				cfg.Name = name
				if desc != "" {
					cfg.Description = desc
				}
				if repo != "" {
					cfg.Repo = repo
				}
				if len(owners) > 0 {
					cfg.Owners = owners
				}
				if contextPack != "" {
					cfg.ContextPack = contextPack
				}
			} else {
				cfg = &entity.ProjectConfig{
					Name:        name,
					Description: desc,
					Repo:        repo,
					Owners:      owners,
					ContextPack: contextPack,
					Agents:      []entity.AgentSpec{},
				}
			}

			if err := ts.SaveProjectConfig(name, cfg); err != nil {
				return fmt.Errorf("save project.yaml: %w", err)
			}

			fmt.Printf("✓ Project created: projects/%s\n", name)
			if blueprint != "" {
				fmt.Printf("  Blueprint : %s (%d agents)\n", blueprint, len(cfg.Agents))
				fmt.Printf("\nNext steps:\n")
				fmt.Printf("  1. Review the config :  multigent project show --project %s\n", name)
				fmt.Printf("  2. Apply (hire+setup):  multigent project apply --project %s\n", name)
				fmt.Printf("  3. Start scheduler   :  multigent scheduler start\n")
			} else {
				fmt.Printf("  Edit the prompt : vim projects/%s/prompt.md\n", name)
				fmt.Printf("  Edit the config : vim projects/%s/project.yaml\n", name)
				fmt.Printf("  Apply agents    : multigent project apply --project %s\n", name)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Project name")
	cmd.Flags().StringVar(&desc, "desc", "", "Short description")
	cmd.Flags().StringVar(&repo, "repo", "", "Path to the project code repository")
	cmd.Flags().StringSliceVar(&owners, "owner", nil, "Human owner(s) accountable for project scope and context")
	cmd.Flags().StringVar(&contextPack, "context-pack", "", "Project context pack reference")
	cmd.Flags().StringVar(&blueprint, "blueprint", "", "Name of a project blueprint in project-blueprints/")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func joinStrings(ss []string, empty string) string {
	if len(ss) == 0 {
		return empty
	}
	return strings.Join(ss, ", ")
}

// ── template apply helper ─────────────────────────────────────────────────────

// applyTemplate creates an agency workspace at root using the template at src.
// src can be a local .tar.gz, a local directory, or an HTTPS URL to a .tar.gz.
func applyTemplate(root, agencyName, agencyDesc, src string) error {
	isURL := strings.HasPrefix(src, "https://") || strings.HasPrefix(src, "http://")

	if isURL {
		return applyTemplateURL(root, agencyName, agencyDesc, src)
	}

	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("template source %q not found: %w", src, err)
	}

	if info.IsDir() {
		return tmpl.InitAgencyFromTemplate(root, agencyName, agencyDesc,
			func(dest, name string) error { return tmpl.ApplyDir(src, dest, name) })
	}

	// Local archive.
	return tmpl.InitAgencyFromTemplate(root, agencyName, agencyDesc,
		func(dest, name string) error { return tmpl.Unpack(src, dest, name) })
}

func applyTemplateURL(root, agencyName, agencyDesc, url string) error {
	fmt.Printf("Downloading template from %s ...\n", url)

	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		return fmt.Errorf("download template: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download template: HTTP %d from %s", resp.StatusCode, url)
	}

	// Write to a temp file so we can seek.
	tmp, err := os.CreateTemp("", "multigent-template-*.tar.gz")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		return fmt.Errorf("save template: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return tmpl.InitAgencyFromTemplate(root, agencyName, agencyDesc,
		func(dest, name string) error { return tmpl.Unpack(tmp.Name(), dest, name) })
}
