package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/multigent/multigent/internal/appconfig"
	"github.com/multigent/multigent/internal/errs"
	"github.com/multigent/multigent/internal/playbook"
	"github.com/multigent/multigent/internal/workspace"
	"github.com/spf13/cobra"
)

// globalDir holds the value of the global --dir flag.
// When non-empty it is used as the starting point for workspace discovery
// instead of the current working directory.
var globalDir string
var globalConfigPath string
var loadedConfig *appconfig.Config

var rootCmd = &cobra.Command{
	SilenceErrors: true, // error is printed by main()
	SilenceUsage:  true, // no usage on error
	Use:           "multigent",
	Short:         "AI agent organisation and context management",
	Long: `multigent manages the organisational context for AI agents.

It lets you create an agency with teams and projects, then hire (or assign)
AI agents into projects with the right accumulated context so they can
start working immediately.

Typical workflow:

  multigent create agency --name "MyAgency"
  cd MyAgency

  multigent create team --name "engineering"
  multigent create project --name "my-api" --repo "../my-api"

  multigent agent hire --project "my-api" --team "engineering" \
                       --model "claudecode" --name "dev"
  multigent task add   --project "my-api" --agent "dev" \
                       --title "Implement feature X" --prompt "..." \
                       --created-by human
  multigent agent run  --project "my-api" --agent "dev"

Agent-friendly tips:
  - All list/show commands output JSON by default (--format table for humans)
  - multigent schema [command]  — discover any command's flags as JSON
  - Exit codes: 0=ok  1=error  2=bad-args  3=not-found  5=conflict
  - run --dry-run  outputs JSON preview instead of executing

You can run any command from outside the workspace by passing --dir:

  multigent --dir /path/to/MyAgency list agents`,
}

func init() {
	rootCmd.PersistentFlags().StringVar(
		&globalDir, "dir", "",
		"workspace directory (default: auto-discover from current directory)",
	)
	rootCmd.PersistentFlags().StringVar(
		&globalConfigPath, "config", "",
		"TOML config file path (or MULTIGENT_CONFIG)",
	)

	rootCmd.AddCommand(
		newVersionCmd(),
		newCreateCmd(),
		newTeamCmd(),
		newRoleCmd(),
		newHireCmd(),
		newAssignCmd(),
		newFireCmd(),
		newSyncCmd(),
		newListCmd(),
		newShowCmd(),
		newTaskCmd(),
		newAgentCmd(),
		newRunCmd(),
		newExecCmd(),
		newInboxCmd(),
		newSessionCmd(),
		newSchedulerCmd(),
		newCronCmd(),
		newTemplateCmd(),
		newProjectCmd(),
		newSandboxCmd(),
		newOverviewCmd(),
		newRunsCmd(),
		newClearCmd(),
		newAPICmd(),
		newStartCmd(),
		newDaemonCmd(),
		newCheckUpdateCmd(),
		newUpdateCmd(),
		newDocsCmd(),
		newOKRCmd(),
		newMilestoneCmd(),
		newEnvVarCmd(),
		newProviderCmd(),
		newRuntimeCmd(),
		newWorkerCmd(),
		newSchemaCmd(),
	)
}

// resolveRoot returns the absolute path of the multigent workspace root.
// If --dir is set it searches from that path; otherwise it searches from CWD.
func resolveRoot() (string, error) {
	if globalDir != "" {
		return workspace.FindRoot(globalDir)
	}
	cfg, err := loadAppConfig()
	if err != nil {
		return "", err
	}
	if cfg.Workspace.Dir != "" {
		return workspace.FindRoot(cfg.Workspace.Dir)
	}
	return workspace.FindRootFromCWD()
}

func loadAppConfig() (*appconfig.Config, error) {
	if loadedConfig != nil {
		return loadedConfig, nil
	}
	path := strings.TrimSpace(globalConfigPath)
	if path == "" {
		path = strings.TrimSpace(os.Getenv("MULTIGENT_CONFIG"))
	}
	cfg, err := appconfig.Load(path)
	if err != nil {
		return nil, err
	}
	loadedConfig = cfg
	applyConfigEnv(cfg)
	return cfg, nil
}

func activeConfigPath() string {
	path := strings.TrimSpace(globalConfigPath)
	if path == "" {
		path = strings.TrimSpace(os.Getenv("MULTIGENT_CONFIG"))
	}
	return path
}

func applyConfigEnv(cfg *appconfig.Config) {
	if cfg == nil {
		return
	}
	setEnvIfEmpty("MULTIGENT_SMTP_HOST", cfg.SMTP.Host)
	if cfg.SMTP.Port > 0 {
		setEnvIfEmpty("MULTIGENT_SMTP_PORT", fmt.Sprintf("%d", cfg.SMTP.Port))
	}
	setEnvIfEmpty("MULTIGENT_SMTP_USERNAME", cfg.SMTP.Username)
	setEnvIfEmpty("MULTIGENT_SMTP_PASSWORD", cfg.SMTP.Password)
	setEnvIfEmpty("MULTIGENT_SMTP_FROM", cfg.SMTP.From)
	setEnvIfEmpty("MULTIGENT_SMTP_FROM_NAME", cfg.SMTP.FromName)
	setEnvIfEmpty("MULTIGENT_SMTP_TLS", cfg.SMTP.TLS)
	setEnvIfEmpty("MULTIGENT_E2B_API_URL", cfg.Sandbox.E2B.APIURL)
	if len(cfg.Playbooks.RegistryURLs) > 0 {
		setEnvIfEmpty(playbook.EnvRegistryURLs, strings.Join(cfg.Playbooks.RegistryURLs, ","))
	} else {
		setEnvIfEmpty(playbook.EnvRegistryURLs, playbook.DefaultRegistryURL)
	}
}

func setEnvIfEmpty(key, value string) {
	value = strings.TrimSpace(value)
	if value == "" || os.Getenv(key) != "" {
		return
	}
	_ = os.Setenv(key, value)
}

func main() {
	checkUpdateAsync()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(exitCodeFor(err))
	}
}

// exitCodeFor maps sentinel error types to meaningful exit codes:
//
//	0   success
//	1   general error (default)
//	2   usage / bad arguments
//	3   resource not found
//	5   conflict / already exists
func exitCodeFor(err error) int {
	var notFound *errs.NotFoundError
	if errors.As(err, &notFound) {
		return 3
	}
	var conflict *errs.ConflictError
	if errors.As(err, &conflict) {
		return 5
	}
	var usage *errs.UsageError
	if errors.As(err, &usage) {
		return 2
	}
	return 1
}
