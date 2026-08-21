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
	"github.com/multigent/multigent/internal/entity"
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
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedSampleAgentsForTest(t, s, workspaceID)

	req := agentEnvPolicyRequest(http.MethodGet, "/api/v1/projects/sample/agents/pm/env", "admin", "sample", "pm", nil)
	rec := httptest.NewRecorder()
	s.handleGetAgentEnv(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin get status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = agentEnvPolicyRequest(http.MethodGet, "/api/v1/projects/sample/agents/pm/env", "owner", "sample", "pm", nil)
	rec = httptest.NewRecorder()
	s.handleGetAgentEnv(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("linked owner get status=%d body=%s", rec.Code, rec.Body.String())
	}

	if err := s.users.CreateUser("outsider", "pass123", RoleMember, "", "", "", "", ""); err != nil {
		t.Fatalf("create outsider: %v", err)
	}
	req = agentEnvPolicyRequest(http.MethodGet, "/api/v1/projects/sample/agents/pm/env", "outsider", "sample", "pm", nil)
	rec = httptest.NewRecorder()
	s.handleGetAgentEnv(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("outsider get status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPutAgentEnvValidatesProviderAndAuditsWithoutValues(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedSampleAgentsForTest(t, s, workspaceID)
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
	req := agentEnvPolicyRequest(http.MethodPut, "/api/v1/projects/sample/agents/pm/env", "owner", "sample", "pm", agentEnvBody{
		Env:      map[string]string{"OPENAI_API_KEY": "value-secret"},
		Provider: &missingProvider,
	})
	rec := httptest.NewRecorder()
	s.handlePutAgentEnv(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing provider status=%d body=%s", rec.Code, rec.Body.String())
	}

	provider := "prov-main"
	req = agentEnvPolicyRequest(http.MethodPut, "/api/v1/projects/sample/agents/pm/env", "owner", "sample", "pm", agentEnvBody{
		Env:      map[string]string{"OPENAI_API_KEY": "value-secret"},
		Provider: &provider,
	})
	rec = httptest.NewRecorder()
	s.handlePutAgentEnv(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put env status=%d body=%s", rec.Code, rec.Body.String())
	}
	meta, err := s.agentMetaForProjectMember(workspaceID, "sample", "pm")
	if err != nil {
		t.Fatalf("agent meta: %v", err)
	}
	if meta.Provider != "prov-main" || meta.Env["OPENAI_API_KEY"] != "value-secret" {
		t.Fatalf("meta not updated: %#v", meta)
	}
	events, err := s.controlDB.ListAuditEvents(controldb.AuditEventFilter{
		WorkspaceID:  workspaceID,
		Action:       "agent.env.update",
		ResourceType: "agent_worker",
		ResourceID:   "aw-pm",
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

func TestSetModelUpdatesAgentWorkerFromProjectMemberRoute(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedSampleAgentsForTest(t, s, workspaceID)
	now := "2026-08-21T00:00:00Z"
	if err := s.controlDB.UpsertAgentWorker(controldb.AgentWorker{
		ID:          "aw-worker-model",
		WorkspaceID: workspaceID,
		Name:        "worker-model",
		DisplayName: "Worker Model",
		Model:       "claudecode",
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("worker: %v", err)
	}
	if err := s.controlDB.UpsertProjectMembership(controldb.ProjectMembership{
		ID:          "pm-worker-model",
		WorkspaceID: workspaceID,
		ProjectID:   "sample",
		MemberType:  "agent_worker",
		MemberID:    "aw-worker-model",
		Role:        "developer",
		Title:       "worker-model",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("membership: %v", err)
	}
	req := agentEnvPolicyRequest(http.MethodPost, "/api/v1/projects/sample/agents/worker-model/set-model", "admin", "sample", "worker-model", setModelBody{
		Model: "codex",
	})
	rec := httptest.NewRecorder()
	s.handleSetModel(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("set model status=%d body=%s", rec.Code, rec.Body.String())
	}
	worker, ok, err := s.controlDB.AgentWorkerByID(workspaceID, "aw-worker-model")
	if err != nil || !ok {
		t.Fatalf("worker ok=%v err=%v", ok, err)
	}
	if worker.Model != "codex" {
		t.Fatalf("worker model not updated: %+v", worker)
	}
}

func TestPutAgentEnvUpdatesAgentWorkerModelAccountFromProjectMemberRoute(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedSampleAgentsForTest(t, s, workspaceID)
	now := "2026-08-21T00:00:00Z"
	if err := s.controlDB.UpsertAgentWorker(controldb.AgentWorker{
		ID:                    "aw-worker-provider",
		WorkspaceID:           workspaceID,
		Name:                  "worker-provider",
		DisplayName:           "Worker Provider",
		Model:                 "codex",
		DefaultModelAccountID: "old-provider",
		RuntimeModel:          "old-model",
		Status:                "active",
		CreatedAt:             now,
		UpdatedAt:             now,
	}); err != nil {
		t.Fatalf("worker: %v", err)
	}
	if err := s.controlDB.UpsertProjectMembership(controldb.ProjectMembership{
		ID:          "pm-worker-provider",
		WorkspaceID: workspaceID,
		ProjectID:   "sample",
		MemberType:  "agent_worker",
		MemberID:    "aw-worker-provider",
		Role:        "developer",
		Title:       "worker-provider",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("membership: %v", err)
	}
	sealedKey, err := secretbox.SealString("sk-secret")
	if err != nil {
		t.Fatalf("seal key: %v", err)
	}
	if err := s.controlDB.UpsertModelProvider(workspaceID, controldb.ModelProvider{
		ID:        "prov-worker",
		OwnerType: ConnectionOwnerWorkspace,
		OwnerID:   workspaceID,
		Name:      "Worker Provider Account",
		Type:      "openai",
		APIKey:    sealedKey,
		Model:     "gpt-test",
		EnvJSON:   `{}`,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("model provider: %v", err)
	}
	provider := "prov-worker"
	runtimeModel := "gpt-5.5"
	req := agentEnvPolicyRequest(http.MethodPut, "/api/v1/projects/sample/agents/worker-provider/env", "admin", "sample", "worker-provider", agentEnvBody{
		Env:          map[string]string{},
		Provider:     &provider,
		RuntimeModel: &runtimeModel,
	})
	rec := httptest.NewRecorder()
	s.handlePutAgentEnv(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put env status=%d body=%s", rec.Code, rec.Body.String())
	}
	worker, ok, err := s.controlDB.AgentWorkerByID(workspaceID, "aw-worker-provider")
	if err != nil || !ok {
		t.Fatalf("worker ok=%v err=%v", ok, err)
	}
	if worker.DefaultModelAccountID != "prov-worker" || worker.RuntimeModel != "gpt-5.5" {
		t.Fatalf("worker model account not updated: %+v", worker)
	}
}

func TestAgentEnvCRUDUsesAgentWorkerRuntimeConfig(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedSampleAgentsForTest(t, s, workspaceID)
	now := "2026-08-21T00:00:00Z"
	if err := s.controlDB.UpsertAgentWorker(controldb.AgentWorker{
		ID:                "aw-worker-env",
		WorkspaceID:       workspaceID,
		Name:              "worker-env",
		Model:             "codex",
		RuntimeConfigJSON: `{"env":{"EXISTING":"1"}}`,
		Status:            "active",
		CreatedAt:         now,
		UpdatedAt:         now,
	}); err != nil {
		t.Fatalf("worker: %v", err)
	}
	if err := s.controlDB.UpsertProjectMembership(controldb.ProjectMembership{
		ID:          "pm-worker-env",
		WorkspaceID: workspaceID,
		ProjectID:   "sample",
		MemberType:  "agent_worker",
		MemberID:    "aw-worker-env",
		Title:       "worker-env",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("membership: %v", err)
	}

	setReq := agentEnvPolicyRequest(http.MethodPost, "/api/v1/projects/sample/agents/worker-env/env", "admin", "sample", "worker-env", map[string]string{
		"key":   "RUNTIME_FLAG",
		"value": "enabled",
	})
	setRec := httptest.NewRecorder()
	s.handleSetAgentEnv(setRec, setReq)
	if setRec.Code != http.StatusNoContent {
		t.Fatalf("set env status=%d body=%s", setRec.Code, setRec.Body.String())
	}
	worker, ok, err := s.controlDB.AgentWorkerByID(workspaceID, "aw-worker-env")
	if err != nil || !ok {
		t.Fatalf("worker ok=%v err=%v", ok, err)
	}
	cfg := decodeAgentWorkerRuntimeConfig(worker)
	if cfg.Env["EXISTING"] != "1" || cfg.Env["RUNTIME_FLAG"] != "enabled" {
		t.Fatalf("env not saved in runtime config: %#v", cfg.Env)
	}

	getReq := agentEnvPolicyRequest(http.MethodGet, "/api/v1/projects/sample/agents/worker-env/env", "admin", "sample", "worker-env", nil)
	getRec := httptest.NewRecorder()
	s.handleGetAgentEnv(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get env status=%d body=%s", getRec.Code, getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), "RUNTIME_FLAG") || !strings.Contains(getRec.Body.String(), "enabled") {
		t.Fatalf("get env did not return worker runtime config: %s", getRec.Body.String())
	}

	deleteReq := agentEnvPolicyRequest(http.MethodDelete, "/api/v1/projects/sample/agents/worker-env/env?key=RUNTIME_FLAG", "admin", "sample", "worker-env", nil)
	deleteRec := httptest.NewRecorder()
	s.handleDeleteAgentEnv(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("delete env status=%d body=%s", deleteRec.Code, deleteRec.Body.String())
	}
	worker, ok, err = s.controlDB.AgentWorkerByID(workspaceID, "aw-worker-env")
	if err != nil || !ok {
		t.Fatalf("worker ok=%v err=%v", ok, err)
	}
	cfg = decodeAgentWorkerRuntimeConfig(worker)
	if _, exists := cfg.Env["RUNTIME_FLAG"]; exists {
		t.Fatalf("env was not deleted from runtime config: %#v", cfg.Env)
	}
}

func TestPutAgentSandboxUsesAgentWorkerRuntimeConfig(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedSampleAgentsForTest(t, s, workspaceID)
	now := "2026-08-21T00:00:00Z"
	if err := s.controlDB.UpsertAgentWorker(controldb.AgentWorker{
		ID:          "aw-worker-sandbox",
		WorkspaceID: workspaceID,
		Name:        "worker-sandbox",
		Model:       "codex",
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("worker: %v", err)
	}
	if err := s.controlDB.UpsertProjectMembership(controldb.ProjectMembership{
		ID:          "pm-worker-sandbox",
		WorkspaceID: workspaceID,
		ProjectID:   "sample",
		MemberType:  "agent_worker",
		MemberID:    "aw-worker-sandbox",
		Title:       "worker-sandbox",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("membership: %v", err)
	}

	req := agentEnvPolicyRequest(http.MethodPut, "/api/v1/projects/sample/agents/worker-sandbox/sandbox", "admin", "sample", "worker-sandbox", map[string]any{
		"provider":   "docker",
		"image":      "ghcr.io/multigent/multigent/runtime-base:test",
		"network":    "host",
		"memoryMb":   2048,
		"cpus":       2,
		"timeoutSec": 600,
		"addDirs":    []string{"/workspace/repo"},
	})
	rec := httptest.NewRecorder()
	s.handlePutAgentSandbox(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put sandbox status=%d body=%s", rec.Code, rec.Body.String())
	}
	worker, ok, err := s.controlDB.AgentWorkerByID(workspaceID, "aw-worker-sandbox")
	if err != nil || !ok {
		t.Fatalf("worker ok=%v err=%v", ok, err)
	}
	cfg := decodeAgentWorkerRuntimeConfig(worker)
	if cfg.Sandbox == nil || cfg.Sandbox.Provider != entity.SandboxDocker || cfg.Sandbox.Image != "ghcr.io/multigent/multigent/runtime-base:test" {
		t.Fatalf("sandbox not saved in runtime config: %#v", cfg.Sandbox)
	}
	if len(cfg.AddDirs) != 1 || cfg.AddDirs[0] != "/workspace/repo" {
		t.Fatalf("addDirs not saved in runtime config: %#v", cfg.AddDirs)
	}
	meta, err := s.agentMetaForProjectMember(workspaceID, "sample", "worker-sandbox")
	if err != nil {
		t.Fatalf("meta: %v", err)
	}
	if meta.Sandbox == nil || meta.Sandbox.Provider != entity.SandboxDocker || len(meta.AddDirs) != 1 {
		t.Fatalf("runtime config not merged into meta: %#v", meta)
	}
}

func TestPutAgentEnvRestrictsPersonalModelProviders(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedSampleAgentsForTest(t, s, workspaceID)
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
	req := agentEnvPolicyRequest(http.MethodPut, "/api/v1/projects/sample/agents/pm/env", "admin", "sample", "pm", agentEnvBody{
		Env:      map[string]string{},
		Provider: &provider,
	})
	rec := httptest.NewRecorder()
	s.handlePutAgentEnv(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("admin should not bind another user's provider status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = agentEnvPolicyRequest(http.MethodPut, "/api/v1/projects/sample/agents/pm/env", "owner", "sample", "pm", agentEnvBody{
		Env:      map[string]string{},
		Provider: &provider,
	})
	rec = httptest.NewRecorder()
	s.handlePutAgentEnv(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("owner should not bind personal provider status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = agentEnvPolicyRequest(http.MethodPut, "/api/v1/projects/sample/agents/backend/env", "owner", "sample", "backend", agentEnvBody{
		Env:      map[string]string{},
		Provider: &provider,
	})
	rec = httptest.NewRecorder()
	s.handlePutAgentEnv(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("owner unlinked agent bind status=%d body=%s", rec.Code, rec.Body.String())
	}
}
