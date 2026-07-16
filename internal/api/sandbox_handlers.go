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
	_ = json.NewEncoder(w).Encode(sandbox.DetectCapabilities())
}
