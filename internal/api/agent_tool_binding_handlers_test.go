package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
)

func TestAgentToolBindingRequiresGrantedConnection(t *testing.T) {
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

	blockedReq := agentToolBindingRequest("owner", "sample", "pm", upsertAgentToolBindingRequest{
		ConnectionID: connection.ID,
		AdapterType:  "http_action",
	})
	blockedRec := httptest.NewRecorder()
	s.handleUpsertAgentToolBinding(blockedRec, blockedReq)
	if blockedRec.Code != http.StatusBadRequest {
		t.Fatalf("ungranted binding status=%d body=%s", blockedRec.Code, blockedRec.Body.String())
	}

	if err := s.controlDB.CreateConnectionGrant(controldb.ConnectionGrant{
		ID:           "grant-agent",
		WorkspaceID:  workspaceID,
		ConnectionID: connection.ID,
		TargetType:   ConnectionTargetAgent,
		TargetID:     "sample/pm",
		CreatedBy:    "owner",
	}); err != nil {
		t.Fatalf("grant: %v", err)
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
