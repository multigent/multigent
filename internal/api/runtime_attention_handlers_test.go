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
)

func TestRuntimeAttentionListAndMarkOwnSignals(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	upsertRuntimeAttentionWorker(t, s, workspaceID, "aw-pm", "sample", "pm", now)
	upsertRuntimeAttentionWorker(t, s, workspaceID, "aw-backend", "sample", "backend", now)
	if err := s.controlDB.UpsertAttentionSignal(controldb.AttentionSignal{
		ID:            "sig-pm",
		WorkspaceID:   workspaceID,
		AgentWorkerID: "aw-pm",
		DedupeKey:     "im:lark:msg-pm",
		SourceKind:    "im",
		SourceID:      "msg-pm",
		SourceChannel: "lark:chat",
		Reason:        "mention",
		Priority:      "normal",
		Summary:       "PM was mentioned",
		Status:        "pending",
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("pm signal: %v", err)
	}
	if err := s.controlDB.UpsertAttentionSignal(controldb.AttentionSignal{
		ID:            "sig-backend",
		WorkspaceID:   workspaceID,
		AgentWorkerID: "aw-backend",
		DedupeKey:     "im:lark:msg-backend",
		SourceKind:    "im",
		Reason:        "mention",
		Priority:      "normal",
		Summary:       "Backend was mentioned",
		Status:        "pending",
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("backend signal: %v", err)
	}

	req := runtimeAttentionRequest(workspaceID, "sample", "pm", http.MethodGet, "/api/v1/runtime/attention?status=pending", nil)
	rec := httptest.NewRecorder()
	s.handleRuntimeAttentionSignals(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "sig-pm") || strings.Contains(body, "sig-backend") {
		t.Fatalf("runtime list leaked or missed signals: %s", body)
	}
	seen, ok, err := s.controlDB.AttentionSignalByID(workspaceID, "sig-pm")
	if err != nil || !ok {
		t.Fatalf("load seen signal: ok=%v err=%v", ok, err)
	}
	if seen.Status != "seen" || seen.SeenAt == "" {
		t.Fatalf("runtime list should mark pending signal as seen: %#v", seen)
	}
	secondReq := runtimeAttentionRequest(workspaceID, "sample", "pm", http.MethodGet, "/api/v1/runtime/attention?status=pending", nil)
	secondRec := httptest.NewRecorder()
	s.handleRuntimeAttentionSignals(secondRec, secondReq)
	if secondRec.Code != http.StatusOK {
		t.Fatalf("second list status=%d body=%s", secondRec.Code, secondRec.Body.String())
	}
	if strings.Contains(secondRec.Body.String(), "sig-pm") {
		t.Fatalf("seen signal should not remain pending: %s", secondRec.Body.String())
	}
	openReq := runtimeAttentionRequest(workspaceID, "sample", "pm", http.MethodGet, "/api/v1/runtime/attention?status=open", nil)
	openRec := httptest.NewRecorder()
	s.handleRuntimeAttentionSignals(openRec, openReq)
	if openRec.Code != http.StatusOK {
		t.Fatalf("open list status=%d body=%s", openRec.Code, openRec.Body.String())
	}
	if !strings.Contains(openRec.Body.String(), "sig-pm") || strings.Contains(openRec.Body.String(), "sig-backend") {
		t.Fatalf("open list should include own seen signal only: %s", openRec.Body.String())
	}

	raw, _ := json.Marshal(runtimeAttentionPatchRequest{Status: "handled"})
	markReq := runtimeAttentionRequest(workspaceID, "sample", "pm", http.MethodPatch, "/api/v1/runtime/attention/sig-pm", raw)
	markReq.SetPathValue("id", "sig-pm")
	markRec := httptest.NewRecorder()
	s.handleRuntimePatchAttentionSignal(markRec, markReq)
	if markRec.Code != http.StatusOK {
		t.Fatalf("mark status=%d body=%s", markRec.Code, markRec.Body.String())
	}
	updated, ok, err := s.controlDB.AttentionSignalByID(workspaceID, "sig-pm")
	if err != nil || !ok {
		t.Fatalf("load updated: ok=%v err=%v", ok, err)
	}
	if updated.Status != "handled" || updated.HandledAt == "" {
		t.Fatalf("signal was not marked handled: %#v", updated)
	}
	afterHandledReq := runtimeAttentionRequest(workspaceID, "sample", "pm", http.MethodGet, "/api/v1/runtime/attention?status=open", nil)
	afterHandledRec := httptest.NewRecorder()
	s.handleRuntimeAttentionSignals(afterHandledRec, afterHandledReq)
	if strings.Contains(afterHandledRec.Body.String(), "sig-pm") {
		t.Fatalf("handled signal should not remain open: %s", afterHandledRec.Body.String())
	}
}

func TestRuntimeAttentionCannotMarkAnotherWorkerSignal(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	upsertRuntimeAttentionWorker(t, s, workspaceID, "aw-pm", "sample", "pm", now)
	upsertRuntimeAttentionWorker(t, s, workspaceID, "aw-backend", "sample", "backend", now)
	if err := s.controlDB.UpsertAttentionSignal(controldb.AttentionSignal{
		ID:            "sig-backend",
		WorkspaceID:   workspaceID,
		AgentWorkerID: "aw-backend",
		DedupeKey:     "task:backend",
		SourceKind:    "task",
		Reason:        "assigned",
		Priority:      "normal",
		Summary:       "Backend task",
		Status:        "pending",
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("signal: %v", err)
	}

	raw, _ := json.Marshal(runtimeAttentionPatchRequest{Status: "handled"})
	req := runtimeAttentionRequest(workspaceID, "sample", "pm", http.MethodPatch, "/api/v1/runtime/attention/sig-backend", raw)
	req.SetPathValue("id", "sig-backend")
	rec := httptest.NewRecorder()
	s.handleRuntimePatchAttentionSignal(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden, status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRuntimeAttentionRequiresCapability(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/attention", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxRuntimeAgentKey, runtimeAgentPrincipal{
		WorkspaceID:  workspaceID,
		Project:      "sample",
		Agent:        "pm",
		Capabilities: []string{"task.use"},
	}))
	rec := httptest.NewRecorder()
	s.handleRuntimeAttentionSignals(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden, status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func upsertRuntimeAttentionWorker(t *testing.T, s *Server, workspaceID, workerID, project, agent, now string) {
	t.Helper()
	if err := s.controlDB.UpsertAgentWorker(controldb.AgentWorker{
		ID:          workerID,
		WorkspaceID: workspaceID,
		Name:        agent,
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("worker %s: %v", workerID, err)
	}
	if err := s.controlDB.UpsertProjectMembership(controldb.ProjectMembership{
		ID:               "pm-" + workerID,
		WorkspaceID:      workspaceID,
		ProjectID:        project,
		MemberType:       "agent_worker",
		MemberID:         workerID,
		Title:            agent,
		AttentionEnabled: true,
		AutoPickTasks:    true,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("membership %s: %v", workerID, err)
	}
}

func runtimeAttentionRequest(workspaceID, project, agent, method, path string, body []byte) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), ctxRuntimeAgentKey, runtimeAgentPrincipal{
		WorkspaceID:  workspaceID,
		Project:      project,
		Agent:        agent,
		Capabilities: []string{"attention.use"},
	}))
	return req
}
