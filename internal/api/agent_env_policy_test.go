package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/secretbox"
)

func agentEnvPolicyRequest(method, path, username, project, agent string, body any) *http.Request {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.SetPathValue("name", project)
	req.SetPathValue("agent", agent)
	return req.WithContext(context.WithValue(req.Context(), ctxUserKey, username))
}

func TestAgentEnvHandlersRequireAgentManagementAccess(t *testing.T) {
	s, _ := newConnectionGrantPolicyServer(t)

	req := agentEnvPolicyRequest(http.MethodGet, "/api/v1/projects/tapnow/agents/pm/env", "admin", "tapnow", "pm", nil)
	rec := httptest.NewRecorder()
	s.handleGetAgentEnv(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin get status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = agentEnvPolicyRequest(http.MethodGet, "/api/v1/projects/tapnow/agents/pm/env", "owner", "tapnow", "pm", nil)
	rec = httptest.NewRecorder()
	s.handleGetAgentEnv(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("linked owner get status=%d body=%s", rec.Code, rec.Body.String())
	}

	if err := s.users.CreateUser("outsider", "pass123", RoleMember, "", "", "", "", ""); err != nil {
		t.Fatalf("create outsider: %v", err)
	}
	req = agentEnvPolicyRequest(http.MethodGet, "/api/v1/projects/tapnow/agents/pm/env", "outsider", "tapnow", "pm", nil)
	rec = httptest.NewRecorder()
	s.handleGetAgentEnv(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("outsider get status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPutAgentEnvValidatesProviderAndAuditsWithoutValues(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	sealedKey, err := secretbox.SealString("sk-secret")
	if err != nil {
		t.Fatalf("seal key: %v", err)
	}
	if err := s.controlDB.UpsertModelProvider(workspaceID, controldb.ModelProvider{
		ID:        "prov-main",
		OwnerType: ConnectionOwnerWorkspace,
		OwnerID:   workspaceID,
		Name:      "Main",
		Type:      "openai",
		APIKey:    sealedKey,
		Model:     "gpt-test",
		EnvJSON:   `{}`,
		CreatedAt: "2026-07-15T00:00:00Z",
		UpdatedAt: "2026-07-15T00:00:00Z",
	}); err != nil {
		t.Fatalf("model provider: %v", err)
	}

	missingProvider := "prov-missing"
	req := agentEnvPolicyRequest(http.MethodPut, "/api/v1/projects/tapnow/agents/pm/env", "owner", "tapnow", "pm", agentEnvBody{
		Env:      map[string]string{"OPENAI_API_KEY": "value-secret"},
		Provider: &missingProvider,
	})
	rec := httptest.NewRecorder()
	s.handlePutAgentEnv(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing provider status=%d body=%s", rec.Code, rec.Body.String())
	}

	provider := "prov-main"
	req = agentEnvPolicyRequest(http.MethodPut, "/api/v1/projects/tapnow/agents/pm/env", "owner", "tapnow", "pm", agentEnvBody{
		Env:      map[string]string{"OPENAI_API_KEY": "value-secret"},
		Provider: &provider,
	})
	rec = httptest.NewRecorder()
	s.handlePutAgentEnv(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put env status=%d body=%s", rec.Code, rec.Body.String())
	}
	meta, err := s.st.AgentMeta("tapnow", "pm")
	if err != nil {
		t.Fatalf("agent meta: %v", err)
	}
	if meta.Provider != "prov-main" || meta.Env["OPENAI_API_KEY"] != "value-secret" {
		t.Fatalf("meta not updated: %#v", meta)
	}
	events, err := s.controlDB.ListAuditEvents(controldb.AuditEventFilter{
		WorkspaceID:  workspaceID,
		Action:       "agent.env.update",
		ResourceType: "agent",
		ResourceID:   "tapnow/pm",
		Limit:        10,
	})
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events=%#v", events)
	}
	raw := events[0].AfterJSON + events[0].BeforeJSON
	if strings.Contains(raw, "value-secret") || strings.Contains(raw, "sk-secret") {
		t.Fatalf("audit leaked secret: %#v", events[0])
	}
	if !strings.Contains(raw, "OPENAI_API_KEY") || !strings.Contains(raw, "prov-main") {
		t.Fatalf("audit missing metadata: %#v", events[0])
	}
}

func TestPutAgentEnvRestrictsPersonalModelProviders(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	sealedKey, err := secretbox.SealString("sk-owner")
	if err != nil {
		t.Fatalf("seal key: %v", err)
	}
	if err := s.controlDB.UpsertModelProvider(workspaceID, controldb.ModelProvider{
		ID:        "prov-owner",
		OwnerType: ConnectionOwnerUser,
		OwnerID:   "owner",
		Name:      "Owner Personal",
		Type:      "openai",
		APIKey:    sealedKey,
		Model:     "gpt-test",
		EnvJSON:   `{}`,
		CreatedAt: "2026-07-15T00:00:00Z",
		UpdatedAt: "2026-07-15T00:00:00Z",
	}); err != nil {
		t.Fatalf("model provider: %v", err)
	}

	provider := "prov-owner"
	req := agentEnvPolicyRequest(http.MethodPut, "/api/v1/projects/tapnow/agents/pm/env", "admin", "tapnow", "pm", agentEnvBody{
		Env:      map[string]string{},
		Provider: &provider,
	})
	rec := httptest.NewRecorder()
	s.handlePutAgentEnv(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("admin should not bind another user's provider status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = agentEnvPolicyRequest(http.MethodPut, "/api/v1/projects/tapnow/agents/pm/env", "owner", "tapnow", "pm", agentEnvBody{
		Env:      map[string]string{},
		Provider: &provider,
	})
	rec = httptest.NewRecorder()
	s.handlePutAgentEnv(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("owner linked agent bind status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = agentEnvPolicyRequest(http.MethodPut, "/api/v1/projects/tapnow/agents/backend/env", "owner", "tapnow", "backend", agentEnvBody{
		Env:      map[string]string{},
		Provider: &provider,
	})
	rec = httptest.NewRecorder()
	s.handlePutAgentEnv(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("owner unlinked agent bind status=%d body=%s", rec.Code, rec.Body.String())
	}
}
