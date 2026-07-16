package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (s *Server) handleDeleteTeam(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	team := strings.TrimPrefix(r.PathValue("teamPath"), "/")
	if team == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "missing team path")
		return
	}
	if _, err := s.st.Team(team); err != nil {
		if isNotFoundErr(err) {
			s.jsonErrorCode(w, http.StatusNotFound, ErrCodeTeamNotFound, "team not found")
			return
		}
		s.serverError(w, err)
		return
	}
	if err := s.st.DeleteTeam(team); err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		Action:       "team.delete",
		ResourceType: "team",
		ResourceID:   team,
		Summary:      "Team deleted",
		Request:      r,
	})
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *Server) handleDeleteRole(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	team := strings.TrimSpace(r.PathValue("team"))
	role := strings.TrimSpace(r.PathValue("role"))
	if team == "" || role == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "team and role are required")
		return
	}
	if _, err := s.st.Role(team, role); err != nil {
		if isNotFoundErr(err) {
			s.jsonErrorCode(w, http.StatusNotFound, ErrCodeNotFound, "role not found")
			return
		}
		s.serverError(w, err)
		return
	}
	if err := s.st.DeleteRole(team, role); err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		Action:       "role.delete",
		ResourceType: "role",
		ResourceID:   team + "/" + role,
		Summary:      "Role deleted",
		Request:      r,
	})
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	project := strings.TrimSpace(r.PathValue("name"))
	if project == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "missing project name")
		return
	}
	if _, err := s.st.Project(project); err != nil {
		if isNotFoundErr(err) {
			s.jsonErrorCode(w, http.StatusNotFound, ErrCodeProjectNotFound, "project not found")
			return
		}
		s.serverError(w, err)
		return
	}
	if err := s.st.DeleteProject(project); err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		Action:       "project.delete",
		ResourceType: "project",
		ResourceID:   project,
		Summary:      "Project deleted",
		Request:      r,
	})
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
