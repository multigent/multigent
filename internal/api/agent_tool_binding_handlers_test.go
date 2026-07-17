package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	controldb "github.com/multigent/multigent/internal/db"
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

func agentToolBindingRequest(username, project, agent string, body upsertAgentToolBindingRequest) *http.Request {
	req := providerTestRequest(http.MethodPost, "/api/v1/projects/"+project+"/agents/"+agent+"/tool-bindings", username, body)
	req.SetPathValue("name", project)
	req.SetPathValue("agent", agent)
	return req
}
