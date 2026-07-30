package api

import (
	"encoding/json"
	"net/http"

	"github.com/multigent/multigent/internal/sandbox"
)

func (s *Server) handleSandboxCapabilities(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAccess(w, r) {
		return
	}
	caps := sandbox.DetectCapabilities()
	directHost := map[string]any{"available": directHostExecutionEnabled()}
	if !directHostExecutionEnabled() {
		directHost["reason"] = "Direct host execution is disabled by the server configuration."
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"docker":     caps.Docker,
		"kvm":        caps.KVM,
		"e2b":        caps.E2B,
		"directHost": directHost,
	})
}
