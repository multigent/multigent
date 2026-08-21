package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
)

func TestAgentAttentionSignalsListAndPatchStatus(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	worker := controldb.AgentWorker{
		ID:          "aw-attention",
		WorkspaceID: workspaceID,
		Name:        "nova",
		DisplayName: "Nova",
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
		t.Fatalf("worker: %v", err)
	}
	if err := s.controlDB.UpsertAttentionSignal(controldb.AttentionSignal{
		ID:            "asig-1",
		WorkspaceID:   workspaceID,
		AgentWorkerID: worker.ID,
		DedupeKey:     "im:lark:msg-1",
		SourceKind:    "im",
		SourceID:      "msg-1",
		SourceChannel: "lark:chat-1",
		Reason:        "mention",
		Priority:      "normal",
		ActorType:     "user",
		ActorID:       "admin",
		Summary:       "用户 @ 了 Nova",
		Status:        "pending",
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("signal: %v", err)
	}

	ownerReq := providerTestRequest(http.MethodGet, "/api/v1/agents/nova/attention?status=pending", "owner", nil)
	ownerReq.SetPathValue("id", "nova")
	ownerRec := httptest.NewRecorder()
	s.handleAgentAttentionSignals(ownerRec, ownerReq)
	if ownerRec.Code != http.StatusForbidden {
		t.Fatalf("owner list status=%d body=%s", ownerRec.Code, ownerRec.Body.String())
	}

	req := providerTestRequest(http.MethodGet, "/api/v1/agents/nova/attention?status=pending", "admin", nil)
	req.SetPathValue("id", "nova")
	rec := httptest.NewRecorder()
	s.handleAgentAttentionSignals(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"asig-1"`) || !strings.Contains(rec.Body.String(), `"pending"`) {
		t.Fatalf("unexpected list body: %s", rec.Body.String())
	}

	ownerPatchReq := providerTestRequest(http.MethodPatch, "/api/v1/attention/asig-1", "owner", patchAttentionSignalRequest{Status: "seen"})
	ownerPatchReq.SetPathValue("id", "asig-1")
	ownerPatchRec := httptest.NewRecorder()
	s.handlePatchAttentionSignal(ownerPatchRec, ownerPatchReq)
	if ownerPatchRec.Code != http.StatusForbidden {
		t.Fatalf("owner patch status=%d body=%s", ownerPatchRec.Code, ownerPatchRec.Body.String())
	}

	patchReq := providerTestRequest(http.MethodPatch, "/api/v1/attention/asig-1", "admin", patchAttentionSignalRequest{Status: "seen"})
	patchReq.SetPathValue("id", "asig-1")
	patchRec := httptest.NewRecorder()
	s.handlePatchAttentionSignal(patchRec, patchReq)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", patchRec.Code, patchRec.Body.String())
	}
	updated, ok, err := s.controlDB.AttentionSignalByID(workspaceID, "asig-1")
	if err != nil || !ok {
		t.Fatalf("load updated: ok=%v err=%v", ok, err)
	}
	if updated.Status != "seen" || updated.SeenAt == "" {
		t.Fatalf("signal was not marked seen: %#v", updated)
	}
}

func TestPatchAttentionSignalRejectsInvalidStatus(t *testing.T) {
	s, _ := newConnectionGrantPolicyServer(t)
	req := providerTestRequest(http.MethodPatch, "/api/v1/attention/asig-1", "admin", patchAttentionSignalRequest{Status: "done"})
	req.SetPathValue("id", "asig-1")
	rec := httptest.NewRecorder()
	s.handlePatchAttentionSignal(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
