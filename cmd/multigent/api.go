package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/multigent/multigent/internal/api"
	"github.com/multigent/multigent/internal/appconfig"
	"github.com/multigent/multigent/internal/daemon"
	controldb "github.com/multigent/multigent/internal/db"
	"github.com/spf13/cobra"
)

func newAPICmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "api",
		Short: "HTTP JSON API for the web UI and integrations",
	}
	cmd.AddCommand(newAPIServeCmd())
	return cmd
}

func newAPIServeCmd() *cobra.Command {
	var (
		addr         string
		apiKey       string
		logFile      string
		logLevel     string
		logFormat    string
		logMaxSizeMB int
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start read-only HTTP server for the current workspace",
		Long: `Serves JSON under /api/v1/ for the resolved Multigent workspace.

Typical local use with the Vite dev server (see web/vite.config.ts proxy):

  multigent --dir ./multigent-data api serve
  cd web && pnpm dev

Read: GET health, workspace, stats, teams, projects, tasks, messages, inbox, …
Write: POST .../tasks; POST /api/v1/messages; mark-read, archive, mark-all-read;
  POST .../projects/{name}/messages/mark-all-read; GET .../messages?includeArchived=1

Optional auth: set --api-key or MULTIGENT_WEB_API_KEY; clients must send
Authorization: Bearer <key>.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadAppConfig()
			if err != nil {
				return err
			}
			if !cmd.Flags().Changed("addr") {
				addr = effectiveServerAddr(cfg, addr)
			}
			if !cmd.Flags().Changed("api-key") && apiKey == "" {
				apiKey = effectiveAPIKey(cfg)
			}
			dataRoot, root, err := resolveAPIServeRoots(cfg)
			if err != nil {
				return err
			}
			if err := os.Setenv("MULTIGENT_DATA_DIR", dataRoot); err != nil {
				return err
			}
			if err := configureRuntimeAPIURL(addr); err != nil {
				return err
			}
			logCloser, err := initServiceLogger(resolveServiceLogOptions(cfg, logFile, logLevel, logFormat, logMaxSizeMB, cmd.Flags().Changed), "api")
			if err != nil {
				return fmt.Errorf("init logger: %w", err)
			}
			defer logCloser()
			key := api.ResolveAPIKey(apiKey)
			srv := api.NewServer(root, key)
			srv.SetVersion(version)
			srv.SetUpdateChecker(GetCachedUpdateInfo)
			srv.SetDaemonStatus(daemonStatusJSON)
			log.Printf("multigent api listening on http://%s (workspace %s)", addr, root)
			if err := daemon.SaveWebRuntimeMeta(&daemon.WebRuntimeMeta{
				WorkDir:   root,
				Addr:      addr,
				PID:       os.Getpid(),
				StartedAt: daemon.NowISO(),
			}); err != nil {
				log.Printf("warning: failed to save API runtime metadata: %v", err)
			}
			defer daemon.RemoveWebRuntimeMeta(root)
			if key != "" {
				log.Printf("API key auth enabled (set MULTIGENT_WEB_API_KEY or --api-key)")
			}

			httpSrv := &http.Server{Addr: addr, Handler: srv.Handler()}

			quit := make(chan os.Signal, 1)
			signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-quit
				log.Println("shutting down — stopping schedulers…")
				srv.Shutdown()
				_ = httpSrv.Shutdown(context.Background())
			}()

			if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				return fmt.Errorf("http server: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:27892", "listen address (host:port)")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "optional Bearer token (or MULTIGENT_WEB_API_KEY)")
	cmd.Flags().StringVar(&logFile, "log-file", "", "log file path")
	cmd.Flags().StringVar(&logLevel, "log-level", "", "log level: debug|info|warn|error")
	cmd.Flags().StringVar(&logFormat, "log-format", "", "log format: json|text")
	cmd.Flags().IntVar(&logMaxSizeMB, "log-max-size", 0, "max log file size in MB")
	return cmd
}

func resolveAPIServeRoots(cfg *appconfig.Config) (dataRoot, activeRoot string, err error) {
	start := strings.TrimSpace(globalDir)
	if start == "" && cfg != nil {
		start = strings.TrimSpace(cfg.Workspace.Dir)
	}
	if start == "" {
		start, err = os.Getwd()
		if err != nil {
			return "", "", err
		}
	}
	dataRoot, err = filepath.Abs(start)
	if err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(dataRoot, 0o755); err != nil {
		return "", "", err
	}
	if err := os.Setenv("MULTIGENT_DATA_DIR", dataRoot); err != nil {
		return "", "", err
	}
	db, err := controldb.OpenDefault()
	if err != nil {
		return "", "", err
	}
	defer db.Close()
	rows, err := db.ListWorkspaces()
	if err != nil {
		return "", "", err
	}
	for _, row := range rows {
		if row.Root != "" && workspaceRootBelongsToDataRoot(dataRoot, row.Root) && hasAgency(row.Root) {
			return dataRoot, row.Root, nil
		}
	}
	return dataRoot, dataRoot, nil
}

func workspaceRootBelongsToDataRoot(dataRoot, root string) bool {
	absData, err := filepath.Abs(dataRoot)
	if err != nil {
		return false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	return filepath.Dir(absRoot) == absData && filepath.Base(absRoot) != ".multigent"
}

func hasAgency(root string) bool {
	_, err := os.Stat(filepath.Join(root, ".multigent", "agency.yaml"))
	return err == nil
}
