package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/multigent/multigent/internal/entity"
)

func TestRuntimeTasksListUsesProjectMemberships(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	upsertRuntimeAttentionWorker(t, s, workspaceID, "aw-worker-only", "sample", "worker-only", now)
	task := &entity.Task{
		ID:        "t-worker-only",
		Title:     "Worker-only task",
		Status:    entity.TaskStatusPending,
		Priority:  2,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := s.ts.AddTask("sample", "worker-only", task); err != nil {
		t.Fatalf("add task: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/tasks?scope=all", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxRuntimeAgentKey, runtimeAgentPrincipal{
		WorkspaceID:  workspaceID,
		Project:      "sample",
		Agent:        "worker-only",
		Capabilities: []string{"task.use"},
	}))
	rec := httptest.NewRecorder()
	s.handleRuntimeTasks(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("runtime tasks status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "t-worker-only") || !strings.Contains(body, "worker-only") {
		t.Fatalf("worker-backed task missing from runtime response: %s", body)
	}
}

func TestProjectTasksListUsesProjectMemberships(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	upsertRuntimeAttentionWorker(t, s, workspaceID, "aw-worker-only", "sample", "worker-only", now)
	task := &entity.Task{
		ID:        "t-project-worker-only",
		Title:     "Project worker-only task",
		Status:    entity.TaskStatusPending,
		Priority:  2,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := s.ts.AddTask("sample", "worker-only", task); err != nil {
		t.Fatalf("add task: %v", err)
	}

	req := providerTestRequest(http.MethodGet, "/api/v1/projects/sample/tasks?scope=all", "admin", nil)
	req.SetPathValue("name", "sample")
	rec := httptest.NewRecorder()
	s.handleProjectTasks(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("project tasks status=%d body=%s", rec.Code, rec.Body.String())
	}
	var rows []taskRow
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode tasks: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != "t-project-worker-only" || rows[0].Agent != "worker-only" {
		t.Fatalf("worker-backed task missing from project response: %#v", rows)
	}
}

func TestProjectMessagesListUsesProjectMemberships(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	upsertRuntimeAttentionWorker(t, s, workspaceID, "aw-worker-only", "sample", "worker-only", now)
	if err := s.ts.SendMessage(&entity.Message{
		ID:     "msg-worker-only",
		From:   "human",
		To:     "sample/worker-only",
		Body:   "hello worker",
		SentAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("send message: %v", err)
	}

	req := providerTestRequest(http.MethodGet, "/api/v1/projects/sample/messages?archived=all", "admin", nil)
	req.SetPathValue("name", "sample")
	rec := httptest.NewRecorder()
	s.handleProjectMessages(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("project messages status=%d body=%s", rec.Code, rec.Body.String())
	}
	var rows []msgRow
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode messages: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != "msg-worker-only" || rows[0].Mailbox != "sample/worker-only" {
		t.Fatalf("worker-backed message missing from project response: %#v", rows)
	}
}

func TestStatsUsesProjectMemberships(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	upsertRuntimeAttentionWorker(t, s, workspaceID, "aw-worker-only", "sample", "worker-only", now)
	if err := s.ts.AddTask("sample", "worker-only", &entity.Task{
		ID:        "t-stats-worker-only",
		Title:     "Stats worker-only task",
		Status:    entity.TaskStatusPending,
		Priority:  2,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("add task: %v", err)
	}

	req := providerTestRequest(http.MethodGet, "/api/v1/stats", "admin", nil)
	rec := httptest.NewRecorder()
	s.handleStats(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("stats status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode stats: %v", err)
	}
	if payload["pendingTasks"] != float64(1) {
		t.Fatalf("expected one worker-backed pending task, got %#v", payload)
	}
}

func TestFindTaskInProjectUsesProjectMemberships(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	upsertRuntimeAttentionWorker(t, s, workspaceID, "aw-worker-only", "sample", "worker-only", now)
	if err := s.ts.AddTask("sample", "worker-only", &entity.Task{
		ID:        "t-find-worker-only",
		Title:     "Find worker-only task",
		Status:    entity.TaskStatusPending,
		Priority:  2,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("add task: %v", err)
	}

	task, agent, err := s.findTaskInProject("sample", "t-find-worker-only")
	if err != nil {
		t.Fatalf("find task: %v", err)
	}
	if task.ID != "t-find-worker-only" || agent != "worker-only" {
		t.Fatalf("unexpected task=%#v agent=%q", task, agent)
	}
}
