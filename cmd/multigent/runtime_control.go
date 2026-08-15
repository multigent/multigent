package main

import (
	"log"
	"net"
	"os"
	"strings"

	"github.com/multigent/multigent/internal/daemon"
)

// configureRuntimeAPIURL makes the control plane reachable by schedulers and
// agent runners started by this server process. An explicit operator override
// still wins (for example when agents reach the API through a reverse proxy).
func configureRuntimeAPIURL(addr string) error {
	if strings.TrimSpace(os.Getenv("MULTIGENT_API_URL")) != "" {
		return nil
	}
	return os.Setenv("MULTIGENT_API_URL", daemon.RuntimeAPIURL(addr))
}

func warnIfDockerCallbackMayFail(addr string) {
	if !isLoopbackListenAddr(addr) {
		return
	}
	log.Printf("warning: Multigent is listening on %s. Docker sandbox agents may not be able to call back into the Runtime API from host.docker.internal. If agent runs fail with connection refused, restart with --addr 0.0.0.0:<port> or set MULTIGENT_API_URL to a Docker-reachable address.", addr)
}

func isLoopbackListenAddr(addr string) bool {
	addr = strings.TrimSpace(addr)
	if addr == "" || strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return false
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		if strings.HasPrefix(addr, "localhost:") || strings.HasPrefix(addr, "127.0.0.1:") || strings.HasPrefix(addr, "[::1]:") {
			return true
		}
		return false
	}
	host = strings.Trim(host, "[]")
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}
