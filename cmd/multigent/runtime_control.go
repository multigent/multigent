package main

import (
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
