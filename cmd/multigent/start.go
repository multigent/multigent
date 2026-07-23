package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/multigent/multigent/internal/api"
	"github.com/multigent/multigent/internal/daemon"
	"github.com/multigent/multigent/internal/sandbox"
	"github.com/multigent/multigent/web"
	"github.com/spf13/cobra"
)

func newStartCmd() *cobra.Command {
	var (
		addr         string
		apiKey       string
		open         bool
		logFile      string
		logLevel     string
		logFormat    string
		logMaxSizeMB int
	)

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the web console (API + embedded frontend)",
		Long: `Launches a single HTTP server that serves both the JSON API and the
built-in web console on the same port. The frontend is embedded in the
binary at build time (web/dist).

This is the recommended way to run multigent in production or on a
remote server. For local development with hot-reload, use
'multigent api serve' combined with 'cd web && pnpm dev'.`,
		Example: `  # Default: listen on 127.0.0.1:27892
  multigent start

  # Custom address
  multigent start --addr 0.0.0.0:8080

  # With API key auth
  multigent start --api-key my-secret

  # Auto-open browser
  multigent start --open`,
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
			logCloser, err := initServiceLogger(resolveServiceLogOptions(cfg, logFile, logLevel, logFormat, logMaxSizeMB, cmd.Flags().Changed), "web")
			if err != nil {
				return fmt.Errorf("init logger: %w", err)
			}
			defer logCloser()

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
			logDockerReadiness()
			key := api.ResolveAPIKey(apiKey)
			srv := api.NewServer(root, key)
			srv.SetVersion(version)
			srv.SetUpdateChecker(GetCachedUpdateInfo)
			srv.SetDaemonStatus(daemonStatusJSON)

			handler := newSPAHandler(srv.Handler())

			url := fmt.Sprintf("http://%s", addr)
			log.Printf("multigent web console: %s (workspace %s)", url, root)
			if err := daemon.SaveWebRuntimeMeta(&daemon.WebRuntimeMeta{
				WorkDir:   root,
				Addr:      addr,
				PID:       os.Getpid(),
				StartedAt: daemon.NowISO(),
			}); err != nil {
				log.Printf("warning: failed to save web runtime metadata: %v", err)
			}
			defer daemon.RemoveWebRuntimeMeta(root)
			if key != "" {
				log.Printf("API key auth enabled")
			}
			if open {
				go openBrowser(url)
			}

			httpSrv := &http.Server{Addr: addr, Handler: handler}

			quit := make(chan os.Signal, 1)
			signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-quit
				log.Println("shutting down — stopping schedulers…")
				srv.Shutdown()
				_ = httpSrv.Shutdown(context.Background())
			}()

			err = httpSrv.ListenAndServe()
			if err != nil && err != http.ErrServerClosed {
				return fmt.Errorf("http server: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:27892", "listen address (host:port)")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "optional Bearer token (or MULTIGENT_WEB_API_KEY)")
	cmd.Flags().BoolVar(&open, "open", false, "open the web console in default browser")
	cmd.Flags().StringVar(&logFile, "log-file", "", "log file path")
	cmd.Flags().StringVar(&logLevel, "log-level", "", "log level: debug|info|warn|error")
	cmd.Flags().StringVar(&logFormat, "log-format", "", "log format: json|text")
	cmd.Flags().IntVar(&logMaxSizeMB, "log-max-size", 0, "max log file size in MB")
	return cmd
}

func logDockerReadiness() {
	if err := sandbox.CheckDocker(); err != nil {
		log.Printf("warning: Docker sandbox is not ready. Multigent can start, but CLI agents cannot run until Docker is installed and running. Details: %v", err)
		return
	}
	image := sandbox.BaseImage
	log.Printf("Docker sandbox ready. First agent run may pull runtime image %s and take a few minutes.", image)
}

// newSPAHandler wraps the API handler with an SPA file server.
// /api/ requests are forwarded to the API handler; everything else is served
// from the embedded web/dist with SPA fallback to index.html.
func newSPAHandler(apiHandler http.Handler) http.Handler {
	distFS, err := fs.Sub(web.DistFS, "dist")
	if err != nil {
		log.Fatalf("embedded web assets not found: %v", err)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			apiHandler.ServeHTTP(w, r)
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		f, err := distFS.Open(path)
		if err != nil {
			serveEmbeddedFile(w, distFS, "index.html")
			return
		}
		f.Close()
		serveEmbeddedFile(w, distFS, path)
	})
}

func serveEmbeddedFile(w http.ResponseWriter, fsys fs.FS, name string) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	ct := mime.TypeByExtension(filepath.Ext(name))
	if ct == "" {
		ct = http.DetectContentType(data)
	}
	w.Header().Set("Content-Type", ct)

	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}

	w.Write(data)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd.exe", "/c", "start", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
