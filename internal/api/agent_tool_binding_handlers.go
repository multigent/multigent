package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/multigent/multigent/internal/connector"
	controldb "github.com/multigent/multigent/internal/db"
)

type upsertAgentToolBindingRequest struct {
	ConnectionID string         `json:"connectionId"`
	AdapterType  string         `json:"adapterType"`
	Status       string         `json:"status"`
	Config       map[string]any `json:"config"`
}

func (s *Server) handleListAgentToolBindings(w http.ResponseWriter, r *http.Request) {
	project, agent, workspaceID, ok := s.agentToolBindingScope(w, r)
	if !ok {
		return
	}
	bindings, err := s.controlDB.ListAgentToolBindings(controldb.AgentToolBindingFilter{
		WorkspaceID: workspaceID,
		ProjectID:   project,
		AgentID:     agent,
	})
	if err != nil {
		s.serverError(w, err)
		return
	}
	out := make([]agentToolBindingModel, 0, len(bindings))
	for _, binding := range bindings {
		out = append(out, agentToolBindingToModel(binding))
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"bindings": out})
}

func (s *Server) handleUpsertAgentToolBinding(w http.ResponseWriter, r *http.Request) {
	project, agent, workspaceID, ok := s.agentToolBindingScope(w, r)
	if !ok {
		return
	}
	cur := s.currentUser(r)
	if cur == nil || cur.Username == "" || cur.Username == "apikey" {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeAuthenticatedUserRequired, "authenticated user required")
		return
	}
	var body upsertAgentToolBindingRequest
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	body.ConnectionID = strings.TrimSpace(body.ConnectionID)
	body.AdapterType = strings.TrimSpace(body.AdapterType)
	body.Status = strings.TrimSpace(body.Status)
	if body.Status == "" {
		body.Status = "enabled"
	}
	if body.Status != "enabled" && body.Status != "disabled" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "status must be enabled or disabled")
		return
	}
	connection, exists, err := s.controlDB.ConnectionByID(body.ConnectionID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !exists || connection.WorkspaceID != workspaceID || connection.Status != "active" {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeConnectionNotFound, "connection not found")
		return
	}
	if !s.canReadConnection(r, connection, cur) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeConnectionAccessRequired, "connection access required")
		return
	}
	allowed, err := s.connectionAvailableToRuntimeAgent(connection, workspaceID, project, agent)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !allowed {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "connection must be granted to this agent before it can be enabled")
		return
	}
	if body.AdapterType != "" {
		if err := s.validateRuntimeAdapterType(connection, body.AdapterType); err != nil {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, err.Error())
			return
		}
	}
	configJSON := "{}"
	if body.Config != nil {
		raw, err := json.Marshal(body.Config)
		if err != nil {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "config must be a JSON object")
			return
		}
		configJSON = string(raw)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	binding := controldb.AgentToolBinding{
		ID:           newConnectionID("toolbind"),
		WorkspaceID:  workspaceID,
		ProjectID:    project,
		AgentID:      agent,
		ConnectionID: connection.ID,
		Provider:     connection.Provider,
		AdapterType:  body.AdapterType,
		Status:       body.Status,
		ConfigJSON:   configJSON,
		CreatedBy:    cur.Username,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.controlDB.UpsertAgentToolBinding(binding); err != nil {
		s.serverError(w, err)
		return
	}
	bindings, _ := s.controlDB.ListAgentToolBindings(controldb.AgentToolBindingFilter{
		WorkspaceID:  workspaceID,
		ProjectID:    project,
		AgentID:      agent,
		ConnectionID: connection.ID,
	})
	if len(bindings) > 0 {
		binding = bindings[0]
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "agent.tool_binding.upsert",
		ResourceType: "agent",
		ResourceID:   project + "/" + agent,
		Summary:      "Agent tool binding updated",
		After:        agentToolBindingToModel(binding),
		Request:      r,
	})
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(agentToolBindingToModel(binding))
}

func (s *Server) handleDeleteAgentToolBinding(w http.ResponseWriter, r *http.Request) {
	project, agent, workspaceID, ok := s.agentToolBindingScope(w, r)
	if !ok {
		return
	}
	bindingID := strings.TrimSpace(r.PathValue("bindingId"))
	binding, exists, err := s.controlDB.AgentToolBindingByID(bindingID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !exists || binding.WorkspaceID != workspaceID || binding.ProjectID != project || binding.AgentID != agent {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeConnectionGrantNotFound, "tool binding not found")
		return
	}
	if err := s.controlDB.DeleteAgentToolBinding(binding.ID); err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "agent.tool_binding.delete",
		ResourceType: "agent",
		ResourceID:   project + "/" + agent,
		Summary:      "Agent tool binding deleted",
		Before:       agentToolBindingToModel(binding),
		Request:      r,
	})
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *Server) agentToolBindingScope(w http.ResponseWriter, r *http.Request) (string, string, string, bool) {
	project := strings.TrimSpace(r.PathValue("name"))
	agent := strings.TrimSpace(r.PathValue("agent"))
	if !s.checkProjectAccess(w, r, project) {
		return "", "", "", false
	}
	if !s.agentExistsInProject(project, agent) {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeAgentNotFound, "agent not found")
		return "", "", "", false
	}
	if !s.canOperateAgent(r, project, agent) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeAgentOperatorRequired, "agent operator access required")
		return "", "", "", false
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return "", "", "", false
	}
	return project, agent, workspaceID, true
}

func (s *Server) connectionAvailableToRuntimeAgent(connection controldb.Connection, workspaceID, project, agent string) (bool, error) {
	grants, err := s.controlDB.ListConnectionGrants(connection.ID)
	if err != nil {
		return false, err
	}
	return len(matchingAgentConnectionGrants(grants, workspaceID, project, agent)) > 0 ||
		workspaceConnectionAvailableToAgent(connection, workspaceID), nil
}

func (s *Server) validateRuntimeAdapterType(connection controldb.Connection, adapterType string) error {
	provider, ok, err := s.findConnectorProvider(connection.Provider)
	if err != nil {
		return err
	}
	if !ok {
		provider = connector.Provider{Provider: connection.Provider}
	}
	actions := runtimeActionsForProviderConnection(connection, provider)
	adapters := runtimeAdaptersForProviderConnection(provider, actions)
	for _, adapter := range adapters {
		if adapter.Type == adapterType {
			return nil
		}
	}
	return fmt.Errorf("adapterType %q is not available for connection %q", adapterType, connection.ID)
}
