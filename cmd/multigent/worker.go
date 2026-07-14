package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/multigent/multigent/internal/worker"
	"github.com/spf13/cobra"
)

func newWorkerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worker",
		Short: "Manage local import and private-resource workers",
		Long: `Manage local-side Multigent connectors.

The default mode is import: inspect and package existing local agent context so
it can become a cloud agent teammate. Long-running workers are reserved for
private-resource or enterprise environments where Multigent needs controlled
access to private repositories, credentials, or sandboxes.`,
	}
	cmd.AddCommand(
		newWorkerInspectCmd(),
		newWorkerImportCmd(),
		newWorkerStartCmd(),
	)
	return cmd
}

func newWorkerInspectCmd() *cobra.Command {
	var cfg = worker.FromEnv()
	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Print resolved local worker configuration",
		RunE: func(_ *cobra.Command, _ []string) error {
			data, err := cfg.MarshalJSONPretty()
			if err != nil {
				return err
			}
			fmt.Fprintln(os.Stdout, string(data))
			return nil
		},
	}
	bindWorkerFlags(cmd, &cfg)
	return cmd
}

func newWorkerStartCmd() *cobra.Command {
	var cfg = worker.FromEnv()
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a private-resource or enterprise worker process",
		RunE: func(_ *cobra.Command, _ []string) error {
			if dryRun {
				data, err := cfg.MarshalJSONPretty()
				if err != nil {
					return err
				}
				fmt.Fprintln(os.Stdout, string(data))
				return nil
			}
			if err := cfg.ValidateForStart(); err != nil {
				return err
			}
			return fmt.Errorf("worker control-plane protocol is not implemented yet; use `multigent worker inspect` to validate local configuration")
		},
	}
	bindWorkerFlags(cmd, &cfg)
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print resolved config without starting")
	return cmd
}

func bindWorkerFlags(cmd *cobra.Command, cfg *worker.Config) {
	cmd.Flags().Var(&cfg.Mode, "mode", "worker mode: import, private-resource, or enterprise")
	cmd.Flags().StringVar(&cfg.WorkerID, "id", cfg.WorkerID, "worker id (default: hostname or MULTIGENT_WORKER_ID)")
	cmd.Flags().StringVar(&cfg.ControlPlaneURL, "control-plane-url", cfg.ControlPlaneURL, "Multigent control-plane URL")
	cmd.Flags().StringVar(&cfg.Token, "token", cfg.Token, "worker token (or MULTIGENT_WORKER_TOKEN)")
	cmd.Flags().StringVar(&cfg.Workspace, "workspace", cfg.Workspace, "local workspace/cache directory for worker execution")
	cmd.Flags().DurationVar(&cfg.PollInterval, "poll-interval", 10*time.Second, "job polling interval")
	cmd.Flags().IntVar(&cfg.Capacity, "capacity", cfg.Capacity, "max concurrent jobs")
}

func newWorkerImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Inspect local agent context for cloud-agent import",
	}
	cmd.AddCommand(newWorkerImportScanCmd())
	return cmd
}

func newWorkerImportScanCmd() *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan a local agent directory without reading file contents",
		RunE: func(_ *cobra.Command, _ []string) error {
			manifest, err := worker.ScanImportManifest(path)
			if err != nil {
				return err
			}
			data, err := json.MarshalIndent(manifest, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(os.Stdout, string(data))
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "local agent directory to scan")
	_ = cmd.MarkFlagRequired("path")
	return cmd
}
