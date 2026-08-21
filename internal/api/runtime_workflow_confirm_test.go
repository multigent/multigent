package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/multigent/multigent/internal/entity"
	workflowstore "github.com/multigent/multigent/internal/workflow"
)

func TestRuntimeConfirmRequestRejectsWorkflowTask(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedSampleAgentsForTest(t, s, workspaceID)
	now := time.Now().UTC()
	task := &entity.Task{
		ID:        "task-workflow",
		Title:     "Workflow task",
		Priority:  2,
		Assignee:  "sample/pm",
		Status:    entity.TaskStatusInProgress,
		Prompt:    "complete workflow step",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.ts.AddTask("sample", "pm", task); err != nil {
		t.Fatalf("add task: %v", err)
	}
	wfStore := workflowstore.NewStore(s.controlDB, workspaceID)
	def := &entity.WorkflowDefinition{
		ID:          "wf-confirm-guard",
		Name:        "Confirm Guard",
		Version:     1,
		Scope:       "workspace",
		StartStepID: "start",
		Steps: []entity.WorkflowStep{{
			ID:       "start",
			Type:     "agent_task",
			Title:    "Start",
			Position: entity.WorkflowPosition{X: 0, Y: 0},
		}},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := wfStore.SaveDefinition(def); err != nil {
		t.Fatalf("save workflow definition: %v", err)
	}
	if _, _, err := wfStore.StartRun("sample", task.ID, def.ID, nil); err != nil {
		t.Fatalf("start workflow run: %v", err)
	}

	req := providerTestRequest(http.MethodPost, "/api/v1/runtime/tasks/task-workflow/confirm-request", "", runtimeConfirmRequestBody{
		Summary: "Need review",
		To:      "owner",
	})
	req.SetPathValue("id", task.ID)
	req = req.WithContext(context.WithValue(req.Context(), ctxRuntimeAgentKey, runtimeAgentPrincipal{
		WorkspaceID:  workspaceID,
		Project:      "sample",
		Agent:        "pm",
		Capabilities: []string{"task.use"},
	}))
	rec := httptest.NewRecorder()
	s.handleRuntimeTaskConfirmRequest(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "workflow tasks must request human review") {
		t.Fatalf("unexpected body=%s", rec.Body.String())
	}
	got, err := s.ts.GetTask("sample", "pm", task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Status != entity.TaskStatusInProgress {
		t.Fatalf("task status changed to %s", got.Status)
	}
	if got.ConfirmationReq != nil {
		t.Fatalf("confirmation request was set: %#v", got.ConfirmationReq)
	}
}
