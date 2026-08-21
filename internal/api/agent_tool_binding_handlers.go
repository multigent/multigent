package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/multigent/multigent/internal/connector"
	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
)

type upsertAgentToolBindingRequest struct {
	ConnectionID string         `json:"connectionId"`
	AdapterType  string         `json:"adapterType"`
	Status       string         `json:"status"`
	Config       map[string]any `json:"config"`
}

type installProjectToolBindingsRequest struct {
	ConnectionID string         `json:"connectionId"`
	AdapterType  string         `json:"adapterType"`
	Config       map[string]any `json:"config"`
}

func (s *Server) handleListAgentToolBindings(w http.ResponseWriter, r *http.Request) {
	project, agent, workspaceID, ok := s.agentToolBindingScope(w, r)
	if !ok {
		return
	}
	workerID := s.agentWorkerIDForProjectAgent(workspaceID, project, agent)
	bindings, err := s.listAgentToolBindings(workspaceID, workerID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"bindings": agentToolBindingModels(bindings)})
}

func (s *Server) handleListAgentWorkerToolBindings(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.agentWorkerToolBindingScope(w, r)
	if !ok {
		return
	}
	bindings, err := s.listAgentToolBindings(scope.workspaceID, scope.worker.ID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"bindings": agentToolBindingModels(bindings)})
}

func (s *Server) handleUpsertAgentToolBinding(w http.ResponseWriter, r *http.Request) {
	project, agent, workspaceID, ok := s.agentToolBindingScope(w, r)
	if !ok {
		return
	}
	workerID := s.agentWorkerIDForProjectAgent(workspaceID, project, agent)
	s.upsertAgentToolBinding(w, r, agentToolBindingTarget{
		workspaceID: workspaceID,
		workerID:    workerID,
		project:     project,
		agent:       agent,
		resourceID:  project + "/" + agent,
	})
}

func (s *Server) handleUpsertAgentWorkerToolBinding(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.agentWorkerToolBindingScope(w, r)
	if !ok {
		return
	}
	s.upsertAgentToolBinding(w, r, agentToolBindingTarget{
		workspaceID: scope.workspaceID,
		workerID:    scope.worker.ID,
		project:     scope.projectID,
		agent:       scope.agentName,
		resourceID:  scope.worker.ID,
	})
}

func (s *Server) upsertAgentToolBinding(w http.ResponseWriter, r *http.Request, target agentToolBindingTarget) {
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
	if !exists || connection.WorkspaceID != target.workspaceID || connection.Status != "active" {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeConnectionNotFound, "connection not found")
		return
	}
	if !s.canReadConnection(r, connection, cur) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeConnectionAccessRequired, "connection access required")
		return
	}
	allowed, err := s.connectionAvailableToRuntimeWorker(connection, target.workspaceID, target.workerID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !allowed {
		if !s.canManageConnection(r, connection, cur) {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "connection must be granted to this agent before it can be enabled")
			return
		}
		if err := s.controlDB.CreateConnectionGrant(controldb.ConnectionGrant{
			ID:           newConnectionID("grant"),
			WorkspaceID:  target.workspaceID,
			ConnectionID: connection.ID,
			TargetType:   ConnectionTargetAgent,
			TargetID:     connectionAgentWorkerTargetID(target.workerID),
			CreatedBy:    cur.Username,
			CreatedAt:    time.Now().UTC().Format(time.RFC3339),
		}); err != nil {
			s.serverError(w, err)
			return
		}
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
		ID:            newConnectionID("toolbind"),
		WorkspaceID:   target.workspaceID,
		AgentWorkerID: target.workerID,
		ProjectID:     target.project,
		AgentID:       target.agent,
		ConnectionID:  connection.ID,
		Provider:      connection.Provider,
		AdapterType:   body.AdapterType,
		Status:        body.Status,
		ConfigJSON:    configJSON,
		CreatedBy:     cur.Username,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.controlDB.UpsertAgentToolBinding(binding); err != nil {
		s.serverError(w, err)
		return
	}
	filter := controldb.AgentToolBindingFilter{WorkspaceID: target.workspaceID, AgentWorkerID: target.workerID, ConnectionID: connection.ID}
	bindings, _ := s.controlDB.ListAgentToolBindings(filter)
	if len(bindings) > 0 {
		binding = bindings[0]
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  target.workspaceID,
		Action:       "agent.tool_binding.upsert",
		ResourceType: "agent",
		ResourceID:   target.resourceID,
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
	if !exists || binding.WorkspaceID != workspaceID || !s.agentToolBindingBelongsToProjectAgent(binding, workspaceID, project, agent) {
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

func (s *Server) handleDeleteAgentWorkerToolBinding(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.agentWorkerToolBindingScope(w, r)
	if !ok {
		return
	}
	bindingID := strings.TrimSpace(r.PathValue("bindingId"))
	binding, exists, err := s.controlDB.AgentToolBindingByID(bindingID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !exists || binding.WorkspaceID != scope.workspaceID || strings.TrimSpace(binding.AgentWorkerID) != scope.worker.ID {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeConnectionGrantNotFound, "tool binding not found")
		return
	}
	if err := s.controlDB.DeleteAgentToolBinding(binding.ID); err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  scope.workspaceID,
		Action:       "agent.tool_binding.delete",
		ResourceType: "agent",
		ResourceID:   scope.worker.ID,
		Summary:      "Agent tool binding deleted",
		Before:       agentToolBindingToModel(binding),
		Request:      r,
	})
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *Server) handleInstallProjectToolBindings(w http.ResponseWriter, r *http.Request) {
	project := strings.TrimSpace(r.PathValue("name"))
	if !s.checkProjectManager(w, r, project) {
		return
	}
	cur := s.currentUser(r)
	if cur == nil || cur.Username == "" || cur.Username == "apikey" {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeAuthenticatedUserRequired, "authenticated user required")
		return
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	var body installProjectToolBindingsRequest
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	body.ConnectionID = strings.TrimSpace(body.ConnectionID)
	body.AdapterType = strings.TrimSpace(body.AdapterType)
	if body.ConnectionID == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "connectionId is required")
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
	if connection.OwnerType != ConnectionOwnerWorkspace {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "project-wide install requires a workspace-owned connection")
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
	if err := s.controlDB.CreateConnectionGrant(controldb.ConnectionGrant{
		ID:           newConnectionID("grant"),
		WorkspaceID:  workspaceID,
		ConnectionID: connection.ID,
		TargetType:   ConnectionTargetProject,
		TargetID:     project,
		CreatedBy:    cur.Username,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		s.serverError(w, err)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	type installTarget struct {
		AgentName string
		WorkerID  string
		Model     entity.AgentModel
	}
	targets := []installTarget{}
	if s.agentDirectory != nil {
		memberships, err := s.controlDB.ListProjectMemberships(controldb.ProjectMembershipFilter{
			WorkspaceID: workspaceID,
			ProjectID:   project,
			MemberType:  "agent_worker",
		})
		if err != nil {
			s.serverError(w, err)
			return
		}
		for _, membership := range memberships {
			worker, ok, err := s.agentDirectory.Worker(workspaceID, membership.MemberID)
			if err != nil {
				s.serverError(w, err)
				return
			}
			if !ok {
				continue
			}
			agentName := strings.TrimSpace(membership.Title)
			if agentName == "" {
				agentName = strings.TrimSpace(worker.Name)
			}
			if agentName == "" {
				continue
			}
			targets = append(targets, installTarget{
				AgentName: agentName,
				WorkerID:  worker.ID,
				Model:     entity.AgentModel(strings.TrimSpace(worker.Model)),
			})
		}
	}
	bindings := make([]agentToolBindingModel, 0, len(targets))
	skipped := 0
	for _, target := range targets {
		if strings.TrimSpace(target.AgentName) == "" {
			skipped++
			continue
		}
		if entity.NormaliseModel(target.Model) == entity.ModelHuman {
			skipped++
			continue
		}
		binding := controldb.AgentToolBinding{
			ID:            newConnectionID("toolbind"),
			WorkspaceID:   workspaceID,
			AgentWorkerID: target.WorkerID,
			ProjectID:     project,
			AgentID:       target.AgentName,
			ConnectionID:  connection.ID,
			Provider:      connection.Provider,
			AdapterType:   body.AdapterType,
			Status:        "enabled",
			ConfigJSON:    configJSON,
			CreatedBy:     cur.Username,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := s.controlDB.UpsertAgentToolBinding(binding); err != nil {
			s.serverError(w, err)
			return
		}
		filter := s.agentToolBindingFilterForProjectAgent(workspaceID, project, target.AgentName)
		filter.ConnectionID = connection.ID
		current, err := s.controlDB.ListAgentToolBindings(filter)
		if err != nil {
			s.serverError(w, err)
			return
		}
		if len(current) > 0 {
			binding = current[0]
		}
		bindings = append(bindings, agentToolBindingToModel(binding))
	}
	provider, ok, err := s.findConnectorProvider(connection.Provider)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if ok {
		s.prepareConnectorToolCacheAsync(provider)
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "project.tool_binding.install",
		ResourceType: "project",
		ResourceID:   project,
		Summary:      "Project external tool installed",
		After: map[string]any{
			"connectionId": connection.ID,
			"provider":     connection.Provider,
			"adapterType":  body.AdapterType,
			"installed":    len(bindings),
			"skipped":      skipped,
		},
		Request: r,
	})
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":           true,
		"connectionId": connection.ID,
		"provider":     connection.Provider,
		"installed":    len(bindings),
		"skipped":      skipped,
		"bindings":     bindings,
	})
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

func (s *Server) agentToolBindingFilterForProjectAgent(workspaceID, project, agent string) controldb.AgentToolBindingFilter {
	filter := controldb.AgentToolBindingFilter{WorkspaceID: workspaceID}
	if workerID := s.agentWorkerIDForProjectAgent(workspaceID, project, agent); strings.TrimSpace(workerID) != "" {
		filter.AgentWorkerID = workerID
		return filter
	}
	return filter
}

func (s *Server) agentToolBindingBelongsToProjectAgent(binding controldb.AgentToolBinding, workspaceID, project, agent string) bool {
	if strings.TrimSpace(binding.AgentWorkerID) != "" {
		return binding.AgentWorkerID == s.agentWorkerIDForProjectAgent(workspaceID, project, agent)
	}
	return false
}

type agentWorkerBindingScope struct {
	workspaceID string
	worker      controldb.AgentWorker
	projectID   string
	agentName   string
}

func (s *Server) agentWorkerToolBindingScope(w http.ResponseWriter, r *http.Request) (agentWorkerBindingScope, bool) {
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return agentWorkerBindingScope{}, false
	}
	worker, found, err := s.controlDB.AgentWorkerByID(workspaceID, strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		s.serverError(w, err)
		return agentWorkerBindingScope{}, false
	}
	if !found {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeAgentNotFound, "agent not found")
		return agentWorkerBindingScope{}, false
	}
	if !s.canAdminWorkspace(r, workspaceID) && !s.currentUserCanOperateAgentWorker(r, workspaceID, worker.ID) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeAgentOperatorRequired, "agent operator access required")
		return agentWorkerBindingScope{}, false
	}
	projectID, agentName, err := s.primaryProjectAgentForWorker(workspaceID, worker)
	if err != nil {
		s.serverError(w, err)
		return agentWorkerBindingScope{}, false
	}
	return agentWorkerBindingScope{
		workspaceID: workspaceID,
		worker:      worker,
		projectID:   projectID,
		agentName:   agentName,
	}, true
}

func (s *Server) primaryProjectAgentForWorker(workspaceID string, worker controldb.AgentWorker) (string, string, error) {
	memberships, err := s.controlDB.ListProjectMemberships(controldb.ProjectMembershipFilter{
		WorkspaceID: workspaceID,
		MemberType:  "agent_worker",
		MemberID:    worker.ID,
	})
	if err != nil {
		return "", "", err
	}
	if len(memberships) == 0 {
		return "", worker.Name, nil
	}
	membership := memberships[0]
	agentName := strings.TrimSpace(membership.Title)
	if agentName == "" {
		agentName = strings.TrimSpace(worker.Name)
	}
	return strings.TrimSpace(membership.ProjectID), agentName, nil
}

func (s *Server) connectionAvailableToRuntimeAgent(connection controldb.Connection, workspaceID, project, agent string) (bool, error) {
	grants, err := s.controlDB.ListConnectionGrants(connection.ID)
	if err != nil {
		return false, err
	}
	return len(s.matchingAgentConnectionGrants(grants, workspaceID, project, agent)) > 0, nil
}

func (s *Server) connectionAvailableToRuntimeWorker(connection controldb.Connection, workspaceID, workerID string) (bool, error) {
	grants, err := s.controlDB.ListConnectionGrants(connection.ID)
	if err != nil {
		return false, err
	}
	return len(matchingAgentConnectionGrantsForTargets(grants, workspaceID, "", "", workerID)) > 0, nil
}

func (s *Server) listAgentToolBindings(workspaceID, workerID string) ([]controldb.AgentToolBinding, error) {
	if strings.TrimSpace(workerID) != "" {
		return s.controlDB.ListAgentToolBindings(controldb.AgentToolBindingFilter{WorkspaceID: workspaceID, AgentWorkerID: workerID})
	}
	return nil, nil
}

func agentToolBindingModels(bindings []controldb.AgentToolBinding) []agentToolBindingModel {
	out := make([]agentToolBindingModel, 0, len(bindings))
	for _, binding := range bindings {
		out = append(out, agentToolBindingToModel(binding))
	}
	return out
}

type agentToolBindingTarget struct {
	workspaceID string
	workerID    string
	project     string
	agent       string
	resourceID  string
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
