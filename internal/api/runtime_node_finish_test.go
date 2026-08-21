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
)

func TestRuntimeNodeCompleteMarksNonWorkflowTaskDone(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedSampleAgentsForTest(t, s, workspaceID)
	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339)

	node := controldb.RuntimeNode{
		ID:              "rtn-local",
		WorkspaceID:     workspaceID,
		Name:            "Local",
		Kind:            "personal_computer",
		Status:          "online",
		LastSeenAt:      nowText,
		CreatedByUserID: "admin",
		CreatedAt:       nowText,
		UpdatedAt:       nowText,
	}
	if err := s.controlDB.UpsertRuntimeNode(node); err != nil {
		t.Fatalf("runtime node: %v", err)
	}

	task := &entity.Task{
		ID:        "task-runtime-success",
		Title:     "Runtime success",
		Status:    entity.TaskStatusInProgress,
		Priority:  2,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.ts.AddTask("sample", "pm", task); err != nil {
		t.Fatalf("add task: %v", err)
	}

	run := controldb.RuntimeRun{
		ID:                  "rtrun-success",
		WorkspaceID:         workspaceID,
		RuntimeNodeID:       node.ID,
		AgentWorkerID:       "aw-pm",
		ProjectMembershipID: "pm-sample-pm",
		ProjectID:           "sample",
		AgentID:             "pm",
		TaskID:              task.ID,
		Status:              "running",
		Priority:            2,
		SpecJSON:            `{"kind":"task"}`,
		ResultJSON:          `{}`,
		ClaimedAt:           nowText,
		StartedAt:           nowText,
		LeaseExpiresAt:      now.Add(time.Minute).Format(time.RFC3339),
		CreatedAt:           nowText,
		UpdatedAt:           nowText,
	}
	if err := s.controlDB.UpsertRuntimeRun(run); err != nil {
		t.Fatalf("runtime run: %v", err)
	}

	body, _ := json.Marshal(runtimeRunFinishRequest{Result: map[string]any{
		"summary":   "runtime completed successfully",
		"sessionId": "session-runtime-success",
	}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime-node/runs/rtrun-success/complete", bytes.NewReader(body))
	req.SetPathValue("runId", run.ID)
	req = req.WithContext(context.WithValue(req.Context(), ctxRuntimeNodeKey, runtimeNodePrincipal{
		Node: node,
	}))
	rec := httptest.NewRecorder()
	s.handleRuntimeNodeRunComplete(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("complete status=%d body=%s", rec.Code, rec.Body.String())
	}

	updatedRun, found, err := s.controlDB.RuntimeRunByID(workspaceID, run.ID)
	if err != nil || !found {
		t.Fatalf("load run found=%v err=%v", found, err)
	}
	if updatedRun.Status != "succeeded" || updatedRun.FinishedAt == "" {
		t.Fatalf("run was not completed: %#v", updatedRun)
	}

	archived, err := s.ts.ListArchivedTasks("sample", "pm")
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	var done *entity.Task
	for i := range archived {
		if archived[i].ID == task.ID {
			done = archived[i]
			break
		}
	}
	if done == nil {
		t.Fatalf("completed task was not archived: %#v", archived)
	}
	if done.Status != entity.TaskStatusDoneSuccess || done.Summary != "runtime completed successfully" {
		t.Fatalf("task was not marked success: %#v", done)
	}
	worker, ok, err := s.controlDB.AgentWorkerByID(workspaceID, "aw-pm")
	if err != nil {
		t.Fatalf("worker: %v", err)
	}
	if !ok {
		t.Fatalf("worker not found")
	}
	hb := agentWorkerScheduleForTest(t, worker)
	if hb == nil || hb.SessionID != "session-runtime-success" || hb.LastWakeupStatus != "done" {
		t.Fatalf("heartbeat was not updated: %#v", hb)
	}
}

func TestRuntimeNodeClaimSkipsExpiredRunWhenTaskAlreadyTerminal(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339)

	node := controldb.RuntimeNode{
		ID:              "rtn-local",
		WorkspaceID:     workspaceID,
		Name:            "Local",
		Kind:            "personal_computer",
		Status:          "online",
		LastSeenAt:      nowText,
		CreatedByUserID: "admin",
		CreatedAt:       nowText,
		UpdatedAt:       nowText,
	}
	if err := s.controlDB.UpsertRuntimeNode(node); err != nil {
		t.Fatalf("runtime node: %v", err)
	}

	doneTask := &entity.Task{
		ID:        "task-already-done",
		Title:     "Already done",
		Status:    entity.TaskStatusDoneSuccess,
		Summary:   "completed before stale run was reclaimed",
		Priority:  1,
		CreatedAt: now.Add(-time.Minute),
		UpdatedAt: now.Add(-time.Minute),
	}
	if err := s.ts.AddTask("sample", "pm", doneTask); err != nil {
		t.Fatalf("add done task: %v", err)
	}
	if err := s.ts.ArchiveTask("sample", "pm", doneTask); err != nil {
		t.Fatalf("archive done task: %v", err)
	}
	staleRun := controldb.RuntimeRun{
		ID:             "rtrun-stale-terminal",
		WorkspaceID:    workspaceID,
		ProjectID:      "sample",
		AgentID:        "pm",
		TaskID:         doneTask.ID,
		Status:         "running",
		Priority:       1,
		SpecJSON:       `{"kind":"task"}`,
		ResultJSON:     `{}`,
		LeaseExpiresAt: now.Add(-time.Minute).Format(time.RFC3339),
		ClaimedAt:      now.Add(-2 * time.Minute).Format(time.RFC3339),
		StartedAt:      now.Add(-2 * time.Minute).Format(time.RFC3339),
		CreatedAt:      now.Add(-2 * time.Minute).Format(time.RFC3339),
		UpdatedAt:      now.Add(-time.Minute).Format(time.RFC3339),
	}
	if err := s.controlDB.UpsertRuntimeRun(staleRun); err != nil {
		t.Fatalf("stale run: %v", err)
	}

	pendingTask := &entity.Task{
		ID:        "task-pending",
		Title:     "Pending",
		Status:    entity.TaskStatusPending,
		Priority:  2,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.ts.AddTask("sample", "pm", pendingTask); err != nil {
		t.Fatalf("add pending task: %v", err)
	}
	queuedRun := controldb.RuntimeRun{
		ID:          "rtrun-queued",
		WorkspaceID: workspaceID,
		ProjectID:   "sample",
		AgentID:     "pm",
		TaskID:      pendingTask.ID,
		Status:      "queued",
		Priority:    2,
		SpecJSON:    `{"kind":"task"}`,
		ResultJSON:  `{}`,
		CreatedAt:   nowText,
		UpdatedAt:   nowText,
	}
	if err := s.controlDB.UpsertRuntimeRun(queuedRun); err != nil {
		t.Fatalf("queued run: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime-node/runs/claim", bytes.NewReader([]byte(`{}`)))
	req = req.WithContext(context.WithValue(req.Context(), ctxRuntimeNodeKey, runtimeNodePrincipal{Node: node}))
	rec := httptest.NewRecorder()
	s.handleRuntimeNodeClaimRun(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("claim status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Run *struct {
			ID string `json:"id"`
		} `json:"run"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode claim response: %v body=%s", err, rec.Body.String())
	}
	if resp.Run == nil || resp.Run.ID != queuedRun.ID {
		t.Fatalf("expected queued run after stale cleanup, got body=%s", rec.Body.String())
	}
	cleaned, found, err := s.controlDB.RuntimeRunByID(workspaceID, staleRun.ID)
	if err != nil || !found {
		t.Fatalf("load stale run: found=%v err=%v", found, err)
	}
	if cleaned.Status != "succeeded" || cleaned.FinishedAt == "" || !strings.Contains(cleaned.ResultJSON, "task_already_terminal") {
		t.Fatalf("stale run was not finalized as skipped: %#v", cleaned)
	}
}
