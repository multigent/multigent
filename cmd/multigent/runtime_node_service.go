package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const runtimeNodeServiceName = "multigent-runtime-node"

type runtimeNodeServiceConfig struct {
	BinaryPath   string
	ConfigPath   string
	LogFile      string
	LogMaxSizeMB int
	PollInterval time.Duration
	Concurrency  int
	EnvPATH      string
	EnvHOME      string
	EnvUSER      string
}

type runtimeNodeServiceStatus struct {
	Supported bool
	Installed bool
	Running   bool
	PID       int
	Platform  string
	Error     string
}

func newRuntimeInstallServiceCmd() *cobra.Command {
	var (
		logFile      string
		logMaxSizeMB int
		pollInterval time.Duration
		concurrency  int
		force        bool
	)
	cmd := &cobra.Command{
		Use:   "install-service",
		Short: "Install Runtime Node as an auto-starting system service",
		RunE: func(_ *cobra.Command, _ []string) error {
			if _, err := loadRuntimeNodeConfig(); err != nil {
				return err
			}
			cfg, err := resolveRuntimeNodeServiceConfig(logFile, logMaxSizeMB, pollInterval, concurrency)
			if err != nil {
				return err
			}
			st, _ := runtimeNodeServiceStatusPlatform()
			if st != nil && st.Installed && !force {
				return fmt.Errorf("runtime node service already installed. Use --force to reinstall")
			}
			if err := runtimeNodeServiceInstallPlatform(cfg); err != nil {
				return err
			}
			fmt.Println("Runtime Node service installed and started.")
			fmt.Printf("  Platform:    %s\n", firstNonEmpty(servicePlatformName(), "unknown"))
			fmt.Printf("  Binary:      %s\n", cfg.BinaryPath)
			fmt.Printf("  Config:      %s\n", cfg.ConfigPath)
			fmt.Printf("  Concurrency: %d\n", cfg.Concurrency)
			fmt.Printf("  Poll:        %s\n", cfg.PollInterval)
			fmt.Printf("  Log:         %s\n", cfg.LogFile)
			return nil
		},
	}
	cmd.Flags().StringVar(&logFile, "log-file", "", "runtime node log file path")
	cmd.Flags().IntVar(&logMaxSizeMB, "log-max-size", 10, "max log file size in MB")
	cmd.Flags().DurationVar(&pollInterval, "poll-interval", 3*time.Second, "run claim polling interval")
	cmd.Flags().IntVar(&concurrency, "concurrency", 1, "maximum concurrent agent runs")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing runtime node service")
	return cmd
}

func newRuntimeUninstallServiceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall-service",
		Short: "Remove Runtime Node system service",
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := runtimeNodeServiceUninstallPlatform(); err != nil {
				return err
			}
			fmt.Println("Runtime Node service uninstalled.")
			return nil
		},
	}
}

func newRuntimeStartServiceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start-service",
		Short: "Start Runtime Node system service",
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := runtimeNodeServiceStartPlatform(); err != nil {
				return err
			}
			fmt.Println("Runtime Node service started.")
			return nil
		},
	}
}

func newRuntimeStopServiceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop-service",
		Short: "Stop Runtime Node system service",
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := runtimeNodeServiceStopPlatform(); err != nil {
				return err
			}
			fmt.Println("Runtime Node service stopped.")
			return nil
		},
	}
}

func newRuntimeRestartServiceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart-service",
		Short: "Restart Runtime Node system service",
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := runtimeNodeServiceRestartPlatform(); err != nil {
				return err
			}
			fmt.Println("Runtime Node service restarted.")
			return nil
		},
	}
}

func newRuntimeServiceStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "service-status",
		Short: "Show Runtime Node service status",
		RunE: func(_ *cobra.Command, _ []string) error {
			st, err := runtimeNodeServiceStatusPlatform()
			if err != nil {
				return err
			}
			fmt.Println("Runtime Node service status")
			fmt.Println()
			if st == nil || !st.Supported {
				msg := ""
				if st != nil {
					msg = st.Error
				}
				if msg == "" {
					msg = "service management is not supported on this platform"
				}
				fmt.Printf("  Status:   Unsupported\n  Reason:   %s\n", msg)
				return nil
			}
			status := "Stopped"
			if st.Running {
				status = "Running"
			}
			if !st.Installed {
				status = "Not installed"
			}
			fmt.Printf("  Status:   %s\n", status)
			fmt.Printf("  Platform: %s\n", st.Platform)
			if st.PID > 0 {
				fmt.Printf("  PID:      %d\n", st.PID)
			}
			fmt.Printf("  Log:      %s\n", runtimeNodeDefaultLogFile())
			return nil
		},
	}
}

func newRuntimeServiceLogsCmd() *cobra.Command {
	var follow bool
	var lines int
	var logFile string
	cmd := &cobra.Command{
		Use:   "service-logs",
		Short: "View Runtime Node service logs",
		RunE: func(_ *cobra.Command, _ []string) error {
			if strings.TrimSpace(logFile) == "" {
				logFile = runtimeNodeDefaultLogFile()
			}
			if _, err := os.Stat(logFile); err != nil {
				return fmt.Errorf("log file not found: %s", logFile)
			}
			printLastLines(logFile, lines)
			if follow {
				followRuntimeNodeLogFile(logFile)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow log output")
	cmd.Flags().IntVarP(&lines, "lines", "n", 100, "number of lines to show")
	cmd.Flags().StringVar(&logFile, "log-file", "", "custom log file path")
	return cmd
}

func resolveRuntimeNodeServiceConfig(logFile string, logMaxSizeMB int, pollInterval time.Duration, concurrency int) (runtimeNodeServiceConfig, error) {
	exe, err := os.Executable()
	if err != nil {
		return runtimeNodeServiceConfig{}, fmt.Errorf("cannot detect binary path: %w", err)
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	if strings.TrimSpace(logFile) == "" {
		logFile = runtimeNodeDefaultLogFile()
	}
	if logMaxSizeMB <= 0 {
		logMaxSizeMB = 10
	}
	if pollInterval <= 0 {
		pollInterval = 3 * time.Second
	}
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > 64 {
		concurrency = 64
	}
	home, _ := os.UserHomeDir()
	currentUser := ""
	if u, err := user.Current(); err == nil && u != nil {
		currentUser = firstNonEmpty(u.Username, u.Name)
	}
	return runtimeNodeServiceConfig{
		BinaryPath:   exe,
		ConfigPath:   runtimeNodeConfigPath(),
		LogFile:      logFile,
		LogMaxSizeMB: logMaxSizeMB,
		PollInterval: pollInterval,
		Concurrency:  concurrency,
		EnvPATH:      os.Getenv("PATH"),
		EnvHOME:      home,
		EnvUSER:      currentUser,
	}, nil
}

func followRuntimeNodeLogFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()
	_, _ = f.Seek(0, io.SeekEnd)
	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			fmt.Print(line)
		}
		if err == io.EOF {
			time.Sleep(300 * time.Millisecond)
			reader.Reset(f)
			continue
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return
		}
	}
}
