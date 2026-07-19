package api

import (
	"encoding/json"
	"net/http"
	"strings"

	playbookstore "github.com/multigent/multigent/internal/playbook"
)

func (s *Server) handleListPlaybookTemplates(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !s.checkWorkspaceAccess(w, r, workspaceID) {
		return
	}
	locale := strings.TrimSpace(r.URL.Query().Get("locale"))
	_ = json.NewEncoder(w).Encode(map[string]any{"templates": playbookstore.Templates(locale)})
}

func (s *Server) handleGetPlaybookTemplate(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !s.checkWorkspaceAccess(w, r, workspaceID) {
		return
	}
	locale := strings.TrimSpace(r.URL.Query().Get("locale"))
	tmpl, ok := playbookstore.Template(r.PathValue("playbookId"), locale)
	if !ok {
		s.jsonError(w, http.StatusNotFound, "playbook template not found")
		return
	}
	_ = json.NewEncoder(w).Encode(tmpl)
}
