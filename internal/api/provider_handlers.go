package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/store"
)

// providerBody is the JSON request struct for creating/updating providers.
// Separate from entity.APIProvider because APIKey uses json:"-" on the entity
// to prevent leaking in responses, but we need to accept it in requests.
type providerBody struct {
	OwnerType string            `json:"ownerType"`
	Name      string            `json:"name"`
	Type      string            `json:"type"`
	BaseURL   string            `json:"baseUrl"`
	APIKey    string            `json:"apiKey"`
	Model     string            `json:"model"`
	Env       map[string]string `json:"env,omitempty"`
}

func (b providerBody) toEntity() entity.APIProvider {
	return entity.APIProvider{
		OwnerType: strings.TrimSpace(b.OwnerType),
		Name:      strings.TrimSpace(b.Name),
		Type:      strings.TrimSpace(b.Type),
		BaseURL:   strings.TrimSpace(b.BaseURL),
		APIKey:    strings.TrimSpace(b.APIKey),
		Model:     strings.TrimSpace(b.Model),
		Env:       b.Env,
	}
}

func (s *Server) providerStore() *store.ProviderStore {
	return store.NewProviderStoreWithDB(s.root, s.controlDB)
}

func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAccess(w, r) {
		return
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	cur := s.currentUser(r)
	isAdmin := s.canAdminWorkspace(r, workspaceID)
	project := strings.TrimSpace(r.URL.Query().Get("project"))
	agent := strings.TrimSpace(r.URL.Query().Get("agent"))
	agentScoped := project != "" || agent != ""
	if agentScoped {
		if project == "" || agent == "" {
			s.jsonError(w, http.StatusBadRequest, "project and agent are required together")
			return
		}
		if !s.agentExistsInProject(project, agent) {
			s.jsonError(w, http.StatusNotFound, "agent not found")
			return
		}
		if !s.canManageAgentConfig(r, project, agent) {
			s.jsonError(w, http.StatusForbidden, "agent management access required")
			return
		}
	}
	items, err := s.providerStore().List()
	if err != nil {
		s.serverError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, p := range items {
		if agentScoped {
			if !s.canUseModelProviderForAgent(r, p, project, agent) {
				continue
			}
		} else if !canViewModelProvider(cur, isAdmin, p) {
			continue
		}
		out = append(out, providerToJSON(p))
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) handleAddProvider(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := s.modelProviderWorkspaceMember(w, r)
	if err != nil {
		return
	}
	var body providerBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	prov := body.toEntity()
	if !s.prepareNewModelProvider(w, r, workspaceID, &prov) {
		return
	}
	if prov.Name == "" {
		s.jsonError(w, http.StatusBadRequest, "name is required")
		return
	}
	p, err := s.providerStore().Add(prov)
	if err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "model_provider.create",
		ResourceType: "model_provider",
		ResourceID:   p.ID,
		Summary:      "Model provider created",
		After:        modelProviderAuditPayload(*p),
		Request:      r,
	})
	_ = json.NewEncoder(w).Encode(providerToJSON(*p))
}

func (s *Server) handleUpdateProvider(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := s.modelProviderWorkspaceMember(w, r)
	if err != nil {
		return
	}
	id := r.PathValue("id")
	var body providerBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	ps := s.providerStore()
	existing, err := ps.Get(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.jsonError(w, http.StatusNotFound, err.Error())
			return
		}
		s.serverError(w, err)
		return
	}
	if !s.canManageModelProvider(r, *existing) {
		s.jsonError(w, http.StatusForbidden, "model provider management access required")
		return
	}
	prov := body.toEntity()
	// If no new key provided, keep the existing one.
	if prov.APIKey == "" {
		prov.APIKey = existing.APIKey
	}
	p, err := ps.Update(id, prov)
	if err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "model_provider.update",
		ResourceType: "model_provider",
		ResourceID:   p.ID,
		Summary:      "Model provider updated",
		Before:       modelProviderAuditPayload(*existing),
		After:        modelProviderAuditPayload(*p),
		Request:      r,
	})
	_ = json.NewEncoder(w).Encode(providerToJSON(*p))
}

func (s *Server) handleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := s.modelProviderWorkspaceMember(w, r)
	if err != nil {
		return
	}
	id := r.PathValue("id")
	ps := s.providerStore()
	existing, err := ps.Get(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.jsonError(w, http.StatusNotFound, err.Error())
			return
		}
		s.serverError(w, err)
		return
	}
	if !s.canManageModelProvider(r, *existing) {
		s.jsonError(w, http.StatusForbidden, "model provider management access required")
		return
	}
	if err := ps.Remove(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.jsonError(w, http.StatusNotFound, err.Error())
			return
		}
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "model_provider.delete",
		ResourceType: "model_provider",
		ResourceID:   id,
		Summary:      "Model provider deleted",
		Before:       modelProviderAuditPayload(*existing),
		Request:      r,
	})
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *Server) modelProviderWorkspaceMember(w http.ResponseWriter, r *http.Request) (string, error) {
	if !s.checkCurrentWorkspaceAccess(w, r) {
		return "", http.ErrAbortHandler
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return "", err
	}
	return workspaceID, nil
}

func (s *Server) prepareNewModelProvider(w http.ResponseWriter, r *http.Request, workspaceID string, p *entity.APIProvider) bool {
	ownerType := strings.TrimSpace(p.OwnerType)
	if ownerType == "" {
		if s.canAdminWorkspace(r, workspaceID) {
			ownerType = ConnectionOwnerWorkspace
		} else {
			ownerType = ConnectionOwnerUser
		}
	}
	cur := s.currentUser(r)
	switch ownerType {
	case ConnectionOwnerWorkspace:
		if !s.canAdminWorkspace(r, workspaceID) {
			s.jsonError(w, http.StatusForbidden, "workspace admin access required")
			return false
		}
		p.OwnerType = ConnectionOwnerWorkspace
		p.OwnerID = workspaceID
		return true
	case ConnectionOwnerUser:
		if cur == nil || cur.Username == "" {
			s.jsonError(w, http.StatusForbidden, "authenticated user required")
			return false
		}
		p.OwnerType = ConnectionOwnerUser
		p.OwnerID = cur.Username
		return true
	default:
		s.jsonError(w, http.StatusBadRequest, "ownerType must be workspace or user")
		return false
	}
}

func canViewModelProvider(cur *userRecord, isAdmin bool, p entity.APIProvider) bool {
	switch p.OwnerType {
	case "", ConnectionOwnerWorkspace:
		return true
	case ConnectionOwnerUser:
		return isAdmin || (cur != nil && p.OwnerID == cur.Username)
	default:
		return false
	}
}

func (s *Server) canManageModelProvider(r *http.Request, p entity.APIProvider) bool {
	switch p.OwnerType {
	case "", ConnectionOwnerWorkspace:
		return s.canAdminWorkspace(r, p.OwnerID)
	case ConnectionOwnerUser:
		cur := s.currentUser(r)
		return cur != nil && cur.Username != "" && p.OwnerID == cur.Username
	default:
		return false
	}
}

func providerToJSON(p entity.APIProvider) map[string]any {
	out := map[string]any{
		"id":        p.ID,
		"ownerType": p.OwnerType,
		"ownerId":   p.OwnerID,
		"name":      p.Name,
		"type":      p.Type,
		"baseUrl":   p.BaseURL,
		"model":     p.Model,
		"hasKey":    p.APIKey != "",
	}
	if len(p.Env) > 0 {
		out["env"] = p.Env
	}
	return out
}

func modelProviderAuditPayload(p entity.APIProvider) map[string]any {
	return map[string]any{
		"id":        p.ID,
		"ownerType": p.OwnerType,
		"ownerId":   p.OwnerID,
		"name":      p.Name,
		"type":      p.Type,
		"baseUrl":   p.BaseURL,
		"model":     p.Model,
		"hasKey":    p.APIKey != "",
	}
}
