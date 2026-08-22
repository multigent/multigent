package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/rbac"
)

func TestRuntimeSessionForkListAndCollect(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	upsertRuntimeAttentionWorker(t, s, workspaceID, "aw-pm", "sample", "pm", now)

	raw, _ := json.Marshal(runtimeSessionForkRequest{
		Title:           "接入 Monday 插件",
		Purpose:         "并发处理 Monday connector",
		TaskID:          "t-plugin-monday",
		ParentSessionID: "parent-runtime-session",
		RuntimeProvider: "codex",
		InitialPrompt:   "调研 Monday API 并给出接入方案。",
	})
	req := runtimeSessionRequest(workspaceID, "sample", "pm", "aw-pm", http.MethodPost, "/api/v1/runtime/sessions", raw)
	rec := httptest.NewRecorder()
	s.handleRuntimeSessions(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("fork status=%d body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Session map[string]any `json:"session"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	sessionID, _ := created.Session["id"].(string)
	if sessionID == "" {
		t.Fatalf("missing session id: %#v", created)
	}
	if created.Session["forkMode"] != "native_fork" {
		t.Fatalf("codex with parent session should prefer native fork, got %#v", created.Session["forkMode"])
	}

	listReq := runtimeSessionRequest(workspaceID, "sample", "pm", "aw-pm", http.MethodGet, "/api/v1/runtime/sessions?status=active", nil)
	listRec := httptest.NewRecorder()
	s.handleRuntimeSessions(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	if !strings.Contains(listRec.Body.String(), sessionID) || !strings.Contains(listRec.Body.String(), "Monday") {
		t.Fatalf("fork session missing from list: %s", listRec.Body.String())
	}

	collectRaw, _ := json.Marshal(runtimeSessionPatchRequest{
		Status:        "done",
		ResultSummary: "PR ready: https://example.test/pr/1",
		ResultRefs:    map[string]any{"pr": "https://example.test/pr/1"},
	})
	collectReq := runtimeSessionRequest(workspaceID, "sample", "pm", "aw-pm", http.MethodPatch, "/api/v1/runtime/sessions/"+sessionID, collectRaw)
	collectReq.SetPathValue("id", sessionID)
	collectRec := httptest.NewRecorder()
	s.handleRuntimeSession(collectRec, collectReq)
	if collectRec.Code != http.StatusOK {
		t.Fatalf("collect status=%d body=%s", collectRec.Code, collectRec.Body.String())
	}
	updated, ok, err := s.controlDB.AgentSessionByID(workspaceID, sessionID)
	if err != nil || !ok {
		t.Fatalf("load collected: ok=%v err=%v", ok, err)
	}
	if updated.Status != "done" || updated.CompletedAt == "" || !strings.Contains(updated.ResultSummary, "PR ready") {
		t.Fatalf("session not collected: %#v", updated)
	}
}

func TestRuntimeSessionCannotAccessAnotherWorkerSession(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	upsertRuntimeAttentionWorker(t, s, workspaceID, "aw-pm", "sample", "pm", now)
	upsertRuntimeAttentionWorker(t, s, workspaceID, "aw-dev", "sample", "dev", now)
	if err := s.controlDB.UpsertAgentSession(controldb.AgentSession{
		ID:               "fs-dev",
		WorkspaceID:      workspaceID,
		AgentWorkerID:    "aw-dev",
		SessionKind:      "fork",
		ProjectID:        "sample",
		Title:            "Dev session",
		Status:           "pending",
		ForkMode:         "fresh_with_context",
		PermissionPolicy: "inherit",
		CreatedAt:        now,
		UpdatedAt:        now,
		LastActivityAt:   now,
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	req := runtimeSessionRequest(workspaceID, "sample", "pm", "aw-pm", http.MethodGet, "/api/v1/runtime/sessions/fs-dev", nil)
	req.SetPathValue("id", "fs-dev")
	rec := httptest.NewRecorder()
	s.handleRuntimeSession(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden, status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRuntimeSessionForkRespectsWorkerLimit(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	upsertRuntimeAttentionWorker(t, s, workspaceID, "aw-pm", "sample", "pm", now)
	worker, ok, err := s.controlDB.AgentWorkerByID(workspaceID, "aw-pm")
	if err != nil || !ok {
		t.Fatalf("worker ok=%v err=%v", ok, err)
	}
	worker.RuntimeConfigJSON = `{"maxForkSessions":1}`
	if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
		t.Fatalf("update worker: %v", err)
	}
	if err := s.controlDB.UpsertAgentSession(controldb.AgentSession{
		ID:               "fs-existing",
		WorkspaceID:      workspaceID,
		AgentWorkerID:    "aw-pm",
		SessionKind:      "fork",
		ProjectID:        "sample",
		Title:            "Existing session",
		Status:           "running",
		ForkMode:         "fresh_with_context",
		PermissionPolicy: "inherit",
		CreatedAt:        now,
		UpdatedAt:        now,
		LastActivityAt:   now,
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	raw, _ := json.Marshal(runtimeSessionForkRequest{Title: "Another session", Project: "sample"})
	req := runtimeSessionRequest(workspaceID, "sample", "pm", "aw-pm", http.MethodPost, "/api/v1/runtime/sessions", raw)
	rec := httptest.NewRecorder()
	s.handleRuntimeSessions(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected conflict, status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "active fork sessions limit reached") {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestAgentSessionsWebListFiltersByAgentPermission(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	upsertRuntimeAttentionWorker(t, s, workspaceID, "aw-pm", "sample", "pm", now)
	upsertRuntimeAttentionWorker(t, s, workspaceID, "aw-dev", "sample", "dev", now)
	if err := s.users.CreateUser("member", "pw", RoleMember, "Member", "member@example.com", "", "", ""); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := s.controlDB.UpsertWorkspaceMember(workspaceID, "member", WorkspaceRoleMember); err != nil {
		t.Fatalf("workspace member: %v", err)
	}
	if err := s.users.UpdateUser("member", nil, nil, nil, nil, nil, nil, nil, nil, []agentAccess{{Project: "sample", Agent: "pm", Role: string(rbac.AgentRoleOperator)}}, nil); err != nil {
		t.Fatalf("grant user: %v", err)
	}
	for _, session := range []controldb.AgentSession{
		{ID: "fs-pm", WorkspaceID: workspaceID, AgentWorkerID: "aw-pm", SessionKind: "fork", ProjectID: "sample", Title: "PM visible", Status: "running", CreatedAt: now, UpdatedAt: now, LastActivityAt: now},
		{ID: "fs-dev", WorkspaceID: workspaceID, AgentWorkerID: "aw-dev", SessionKind: "fork", ProjectID: "sample", Title: "Dev hidden", Status: "running", CreatedAt: now, UpdatedAt: now, LastActivityAt: now},
	} {
		if err := s.controlDB.UpsertAgentSession(session); err != nil {
			t.Fatalf("seed session %s: %v", session.ID, err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent-sessions?status=active", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxUserKey, "member"))
	rec := httptest.NewRecorder()
	s.handleAgentSessions(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "fs-pm") || strings.Contains(body, "fs-dev") {
		t.Fatalf("unexpected filtered sessions: %s", body)
	}
}

func TestRuntimeSessionQueuedTaskRunLinksTaskAndRun(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339)
	upsertRuntimeAttentionWorker(t, s, workspaceID, "aw-pm", "sample", "pm", nowText)
	if err := s.controlDB.UpsertAgentSession(controldb.AgentSession{
		ID:               "fs-task-link",
		WorkspaceID:      workspaceID,
		AgentWorkerID:    "aw-pm",
		SessionKind:      "fork",
		ProjectID:        "sample",
		Title:            "Task linked session",
		Status:           "pending",
		ForkMode:         "fresh_with_context",
		PermissionPolicy: "inherit",
		CreatedAt:        nowText,
		UpdatedAt:        nowText,
		LastActivityAt:   nowText,
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	task := &entity.Task{
		ID:        "task-linked",
		Title:     "Linked task",
		Status:    entity.TaskStatusInProgress,
		Priority:  1,
		CreatedAt: now,
		UpdatedAt: now,
		Vars: map[string]string{
			workflowRunIDVar: "wf-run-one",
		},
	}

	s.markForkSessionRunQueued(workspaceID, "fs-task-link", "aw-pm", "rtrun-linked", task, "sample", "pm-aw-pm")

	session, found, err := s.controlDB.AgentSessionByID(workspaceID, "fs-task-link")
	if err != nil || !found {
		t.Fatalf("load session found=%v err=%v", found, err)
	}
	if session.Status != "running" || session.TaskID != task.ID || session.WorkflowInstanceID != "wf-run-one" || session.LastRunID != "rtrun-linked" {
		t.Fatalf("session was not linked to task run: %#v", session)
	}
}

func runtimeSessionRequest(workspaceID, project, agent, workerID, method, path string, body []byte) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), ctxRuntimeAgentKey, runtimeAgentPrincipal{
		WorkspaceID:         workspaceID,
		Project:             project,
		Agent:               agent,
		AgentWorkerID:       workerID,
		ProjectMembershipID: "pm-" + workerID,
		RunID:               "run-test",
		Capabilities:        []string{"session.use"},
	}))
	return req
}
