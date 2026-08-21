package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
)

func TestAgentToolBindingCreatesAgentGrantForConnectionManager(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	connection := controldb.Connection{
		ID:             "conn-github",
		WorkspaceID:    workspaceID,
		Provider:       "github",
		ConnectionName: "default",
		OwnerType:      ConnectionOwnerUser,
		OwnerID:        "owner",
		AuthType:       ConnectionAuthAPIKey,
		Status:         "active",
		ProfileJSON:    `{}`,
		CreatedBy:      "owner",
	}
	if err := s.controlDB.UpsertConnection(connection); err != nil {
		t.Fatalf("connection: %v", err)
	}

	blockedReq := agentToolBindingRequest("viewer", "sample", "pm", upsertAgentToolBindingRequest{
		ConnectionID: connection.ID,
		AdapterType:  "http_action",
	})
	blockedRec := httptest.NewRecorder()
	s.handleUpsertAgentToolBinding(blockedRec, blockedReq)
	if blockedRec.Code != http.StatusForbidden {
		t.Fatalf("viewer binding status=%d body=%s", blockedRec.Code, blockedRec.Body.String())
	}

	allowedReq := agentToolBindingRequest("owner", "sample", "pm", upsertAgentToolBindingRequest{
		ConnectionID: connection.ID,
		AdapterType:  "http_action",
	})
	allowedRec := httptest.NewRecorder()
	s.handleUpsertAgentToolBinding(allowedRec, allowedReq)
	if allowedRec.Code != http.StatusCreated {
		t.Fatalf("granted binding status=%d body=%s", allowedRec.Code, allowedRec.Body.String())
	}
	var body agentToolBindingModel
	if err := json.Unmarshal(allowedRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode binding: %v", err)
	}
	if body.ConnectionID != connection.ID || body.AdapterType != "http_action" || body.Status != "enabled" {
		t.Fatalf("binding=%#v", body)
	}
	grants, err := s.controlDB.ListConnectionGrants(connection.ID)
	if err != nil {
		t.Fatalf("list grants: %v", err)
	}
	if len(matchingAgentConnectionGrants(grants, workspaceID, "sample", "pm")) != 1 {
		t.Fatalf("agent grant missing: %#v", grants)
	}
}

func TestAgentToolBindingUsesAgentWorkerAcrossProjectMemberships(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := "2026-08-21T00:00:00Z"
	worker := controldb.AgentWorker{
		ID:          "aw-tooling",
		WorkspaceID: workspaceID,
		Name:        "nova",
		DisplayName: "Nova",
		Status:      "active",
		Model:       string(entity.ModelClaudeCode),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
		t.Fatalf("worker: %v", err)
	}
	for _, project := range []string{"sample", "other"} {
		if err := s.st.SaveProject(project, &entity.Project{Name: project}); err != nil {
			t.Fatalf("project %s: %v", project, err)
		}
		if err := s.controlDB.UpsertProjectMembership(controldb.ProjectMembership{
			ID:               "pm-tooling-" + project,
			WorkspaceID:      workspaceID,
			ProjectID:        project,
			MemberType:       "agent_worker",
			MemberID:         worker.ID,
			Role:             "manager",
			Title:            "nova",
			AutoPickTasks:    true,
			AttentionEnabled: true,
			PriorityWeight:   1,
			CreatedAt:        now,
			UpdatedAt:        now,
		}); err != nil {
			t.Fatalf("membership %s: %v", project, err)
		}
	}
	connection := controldb.Connection{
		ID:             "conn-worker-github",
		WorkspaceID:    workspaceID,
		Provider:       "github",
		ConnectionName: "default",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       ConnectionAuthAPIKey,
		Status:         "active",
		ProfileJSON:    `{}`,
		CreatedBy:      "admin",
	}
	if err := s.controlDB.UpsertConnection(connection); err != nil {
		t.Fatalf("connection: %v", err)
	}

	req := agentToolBindingRequest("admin", "sample", "nova", upsertAgentToolBindingRequest{
		ConnectionID: connection.ID,
		AdapterType:  "http_action",
	})
	rec := httptest.NewRecorder()
	s.handleUpsertAgentToolBinding(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("bind status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body agentToolBindingModel
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode binding: %v", err)
	}
	if body.AgentWorkerID != worker.ID {
		t.Fatalf("expected worker binding, got %#v", body)
	}

	connections, err := s.resolveAgentRuntimeConnections(workspaceID, "other", "nova")
	if err != nil {
		t.Fatalf("resolve from other project: %v", err)
	}
	if len(connections) != 1 || connections[0].ID != connection.ID {
		t.Fatalf("expected worker binding to resolve across projects, got %#v", connections)
	}
	if connections[0].ToolBinding == nil || connections[0].ToolBinding.AgentWorkerID != worker.ID {
		t.Fatalf("worker tool binding missing in response: %#v", connections[0])
	}
}

func TestInstallProjectToolBindingsInstallsWorkspaceConnectionForAllAgents(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	if err := s.st.SaveAgentMeta("sample", "human-reviewer", &entity.AgentMeta{
		Name:    "human-reviewer",
		Project: "sample",
		Model:   entity.ModelHuman,
	}); err != nil {
		t.Fatalf("save human: %v", err)
	}
	connection := controldb.Connection{
		ID:             "conn-github-workspace",
		WorkspaceID:    workspaceID,
		Provider:       "github",
		ConnectionName: "default",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       ConnectionAuthAPIKey,
		Status:         "active",
		ProfileJSON:    `{}`,
		CreatedBy:      "admin",
	}
	if err := s.controlDB.UpsertConnection(connection); err != nil {
		t.Fatalf("connection: %v", err)
	}

	rec := httptest.NewRecorder()
	req := providerTestRequest(http.MethodPost, "/api/v1/projects/sample/tool-bindings/install", "admin", installProjectToolBindingsRequest{
		ConnectionID: connection.ID,
		AdapterType:  "http_action",
	})
	req.SetPathValue("name", "sample")
	s.handleInstallProjectToolBindings(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("install status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Installed int `json:"installed"`
		Skipped   int `json:"skipped"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Installed != 2 || body.Skipped != 1 {
		t.Fatalf("installed/skipped=%d/%d", body.Installed, body.Skipped)
	}
	bindings, err := s.controlDB.ListAgentToolBindings(controldb.AgentToolBindingFilter{
		WorkspaceID:  workspaceID,
		ProjectID:    "sample",
		ConnectionID: connection.ID,
		Status:       "enabled",
	})
	if err != nil {
		t.Fatalf("bindings: %v", err)
	}
	if len(bindings) != 2 {
		t.Fatalf("bindings=%d", len(bindings))
	}
	grants, err := s.controlDB.ListConnectionGrants(connection.ID)
	if err != nil {
		t.Fatalf("grants: %v", err)
	}
	if len(matchingAgentConnectionGrants(grants, workspaceID, "sample", "pm")) == 0 {
		t.Fatalf("expected project grant to match sample/pm")
	}
}

func TestInstallProjectToolBindingsUsesAgentWorkerMemberships(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := "2026-08-21T00:00:00Z"
	workers := []controldb.AgentWorker{
		{ID: "aw-builder", WorkspaceID: workspaceID, Name: "builder-worker", DisplayName: "Builder", Model: string(entity.ModelClaudeCode), Status: "active", CreatedAt: now, UpdatedAt: now},
		{ID: "aw-reviewer-human", WorkspaceID: workspaceID, Name: "reviewer-human", DisplayName: "Reviewer", Model: string(entity.ModelHuman), Status: "active", CreatedAt: now, UpdatedAt: now},
	}
	for _, worker := range workers {
		if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
			t.Fatalf("worker %s: %v", worker.ID, err)
		}
	}
	for _, membership := range []controldb.ProjectMembership{
		{ID: "pm-builder", WorkspaceID: workspaceID, ProjectID: "sample", MemberType: "agent_worker", MemberID: "aw-builder", Role: "developer", Title: "builder", CreatedAt: now, UpdatedAt: now},
		{ID: "pm-reviewer", WorkspaceID: workspaceID, ProjectID: "sample", MemberType: "agent_worker", MemberID: "aw-reviewer-human", Role: "reviewer", Title: "reviewer", CreatedAt: now, UpdatedAt: now},
	} {
		if err := s.controlDB.UpsertProjectMembership(membership); err != nil {
			t.Fatalf("membership %s: %v", membership.ID, err)
		}
	}
	connection := controldb.Connection{
		ID:             "conn-github-memberships",
		WorkspaceID:    workspaceID,
		Provider:       "github",
		ConnectionName: "default",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       ConnectionAuthAPIKey,
		Status:         "active",
		ProfileJSON:    `{}`,
		CreatedBy:      "admin",
	}
	if err := s.controlDB.UpsertConnection(connection); err != nil {
		t.Fatalf("connection: %v", err)
	}

	rec := httptest.NewRecorder()
	req := providerTestRequest(http.MethodPost, "/api/v1/projects/sample/tool-bindings/install", "admin", installProjectToolBindingsRequest{
		ConnectionID: connection.ID,
		AdapterType:  "http_action",
	})
	req.SetPathValue("name", "sample")
	s.handleInstallProjectToolBindings(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("install status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Installed int `json:"installed"`
		Skipped   int `json:"skipped"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Installed != 1 || body.Skipped != 1 {
		t.Fatalf("installed/skipped=%d/%d", body.Installed, body.Skipped)
	}
	bindings, err := s.controlDB.ListAgentToolBindings(controldb.AgentToolBindingFilter{
		WorkspaceID:   workspaceID,
		AgentWorkerID: "aw-builder",
		ConnectionID:  connection.ID,
		Status:        "enabled",
	})
	if err != nil {
		t.Fatalf("bindings: %v", err)
	}
	if len(bindings) != 1 || bindings[0].AgentID != "builder" {
		t.Fatalf("membership-backed binding missing: %+v", bindings)
	}
}

func TestInstallProjectToolBindingsRejectsUserOwnedConnection(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	connection := controldb.Connection{
		ID:             "conn-github-personal",
		WorkspaceID:    workspaceID,
		Provider:       "github",
		ConnectionName: "default",
		OwnerType:      ConnectionOwnerUser,
		OwnerID:        "owner",
		AuthType:       ConnectionAuthAPIKey,
		Status:         "active",
		ProfileJSON:    `{}`,
		CreatedBy:      "owner",
	}
	if err := s.controlDB.UpsertConnection(connection); err != nil {
		t.Fatalf("connection: %v", err)
	}

	rec := httptest.NewRecorder()
	req := providerTestRequest(http.MethodPost, "/api/v1/projects/sample/tool-bindings/install", "admin", installProjectToolBindingsRequest{
		ConnectionID: connection.ID,
	})
	req.SetPathValue("name", "sample")
	s.handleInstallProjectToolBindings(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("install status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func agentToolBindingRequest(username, project, agent string, body upsertAgentToolBindingRequest) *http.Request {
	req := providerTestRequest(http.MethodPost, "/api/v1/projects/"+project+"/agents/"+agent+"/tool-bindings", username, body)
	req.SetPathValue("name", project)
	req.SetPathValue("agent", agent)
	return req
}
