package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/scaffold"
)

type createProjectBody struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Repo        string   `json:"repo"`
	Owners      []string `json:"owners"`
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	var body createProjectBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	name := strings.TrimSpace(body.Name)
	if err := validateWorkspaceObjectName("project", name); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, err.Error())
		return
	}
	if _, err := s.st.Project(name); err == nil {
		s.jsonErrorCode(w, http.StatusConflict, ErrCodeConflict, fmt.Sprintf("project %q already exists", name))
		return
	} else if !isNotFoundErr(err) {
		s.serverError(w, err)
		return
	}

	p := &entity.Project{
		Name:        name,
		Description: strings.TrimSpace(body.Description),
		Repo:        strings.TrimSpace(body.Repo),
		Owners:      body.Owners,
	}
	if err := scaffold.New(s.st).CreateProject(name, p); err != nil {
		s.serverError(w, err)
		return
	}
	cfg := &entity.ProjectConfig{
		Name:        name,
		Description: p.Description,
		Repo:        p.Repo,
		Owners:      p.Owners,
		Agents:      []entity.AgentSpec{},
	}
	if err := s.ts.SaveProjectConfig(name, cfg); err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		Action:       "project.create",
		ResourceType: "project",
		ResourceID:   name,
		Summary:      "Project created",
		After: map[string]any{
			"name":        name,
			"description": p.Description,
			"repo":        p.Repo,
		},
		Request: r,
	})
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"project": name,
	})
}

func validateWorkspaceObjectName(kind, name string) error {
	if name == "" {
		return fmt.Errorf("%s name is required", kind)
	}
	if len(name) > 80 {
		return fmt.Errorf("%s name must be 80 characters or fewer", kind)
	}
	for _, r := range name {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		if r == '-' || r == '_' || r == '.' {
			continue
		}
		return fmt.Errorf("%s name may only contain letters, numbers, '.', '_' and '-'", kind)
	}
	if strings.HasPrefix(name, ".") || strings.Contains(name, "..") {
		return fmt.Errorf("%s name cannot start with '.' or contain '..'", kind)
	}
	return nil
}
