package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	workflowstore "github.com/multigent/multigent/internal/workflow"
)

func TestRuntimeWorkflowDecisionSubmitAdvancesHumanReview(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	taskID := "task-human-review"
	now := time.Now().UTC()
	task := &entity.Task{
		ID:        taskID,
		Title:     "Needs review",
		Priority:  2,
		Assignee:  "owner",
		Status:    entity.TaskStatusAwaitingConfirmation,
		Prompt:    "review",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.ts.AddTask("sample", "pm", task); err != nil {
		t.Fatalf("add task: %v", err)
	}
	wfStore := workflowstore.NewStore(s.controlDB, workspaceID)
	def := &entity.WorkflowDefinition{
		ID:          "wf-runtime-decision",
		Name:        "Runtime Decision",
		Version:     1,
		Scope:       "workspace",
		StartStepID: "review",
		Steps: []entity.WorkflowStep{{
			ID:    "review",
			Type:  "human_review",
			Title: "Review",
			OutputFields: []entity.WorkflowField{{
				Name:        "decision",
				Description: "Decision",
			}, {
				Name:        "comments",
				Description: "Comments",
			}},
		}},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := wfStore.SaveDefinition(def); err != nil {
		t.Fatalf("save workflow definition: %v", err)
	}
	if _, _, err := wfStore.StartRun("sample", taskID, def.ID, map[string]entity.WorkflowActorBinding{
		"review": {Type: "human", ID: "owner"},
	}); err != nil {
		t.Fatalf("start workflow run: %v", err)
	}
	if err := s.controlDB.CreateInteractionRequest(controldb.InteractionRequest{
		ID:          "ir_decision_ok",
		WorkspaceID: workspaceID,
		ProjectID:   "sample",
		AgentID:     "pm",
		Recipient:   "user:owner",
		Title:       "Decision",
		ContextJSON: `{"taskId":"task-human-review"}`,
		Status:      "submitted",
		CreatedBy:   "sample/pm",
		CreatedAt:   now.Format(time.RFC3339),
		SubmittedAt: now.Format(time.RFC3339),
		SubmittedBy: "owner",
	}); err != nil {
		t.Fatalf("create interaction request: %v", err)
	}
	delegationToken := s.issueRuntimeDelegationToken(runtimeDelegationTokenPayload{
		WorkspaceID:   workspaceID,
		Project:       "sample",
		Agent:         "pm",
		UserID:        "owner",
		InteractionID: "ir_decision_ok",
		Scopes:        []string{"act_as_user"},
	}, time.Minute)

	req := providerTestRequest(http.MethodPost, "/api/v1/runtime/workflow/decision", "", runtimeWorkflowDecisionBody{
		InteractionID:   "ir_decision_ok",
		DelegationToken: delegationToken,
		TaskID:          taskID,
		Decision:        "approve",
		Comments:        "ok",
	})
	req = req.WithContext(context.WithValue(req.Context(), ctxRuntimeAgentKey, runtimeAgentPrincipal{
		WorkspaceID:  workspaceID,
		Project:      "sample",
		Agent:        "pm",
		Capabilities: []string{"task.use"},
	}))
	rec := httptest.NewRecorder()
	s.handleRuntimeWorkflowDecision(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	got, err := s.ts.GetTask("sample", "pm", taskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Status != entity.TaskStatusDoneSuccess {
		t.Fatalf("task status=%s", got.Status)
	}
	request, found, err := s.controlDB.InteractionRequestByID(workspaceID, "ir_decision_ok")
	if err != nil || !found {
		t.Fatalf("get interaction request found=%v err=%v", found, err)
	}
	if request.Status != "handled" {
		t.Fatalf("interaction status=%s", request.Status)
	}
}

func TestRuntimeWorkflowDecisionRejectsWrongReviewer(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	taskID := "task-wrong-reviewer"
	now := time.Now().UTC()
	task := &entity.Task{
		ID:        taskID,
		Title:     "Needs review",
		Priority:  2,
		Assignee:  "owner",
		Status:    entity.TaskStatusAwaitingConfirmation,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.ts.AddTask("sample", "pm", task); err != nil {
		t.Fatalf("add task: %v", err)
	}
	wfStore := workflowstore.NewStore(s.controlDB, workspaceID)
	def := &entity.WorkflowDefinition{
		ID:          "wf-runtime-decision-forbidden",
		Name:        "Runtime Decision Forbidden",
		Version:     1,
		Scope:       "workspace",
		StartStepID: "review",
		Steps:       []entity.WorkflowStep{{ID: "review", Type: "human_review", Title: "Review"}},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := wfStore.SaveDefinition(def); err != nil {
		t.Fatalf("save workflow definition: %v", err)
	}
	if _, _, err := wfStore.StartRun("sample", taskID, def.ID, map[string]entity.WorkflowActorBinding{
		"review": {Type: "human", ID: "owner"},
	}); err != nil {
		t.Fatalf("start workflow run: %v", err)
	}
	if err := s.controlDB.CreateInteractionRequest(controldb.InteractionRequest{
		ID:          "ir_decision_wrong_user",
		WorkspaceID: workspaceID,
		ProjectID:   "sample",
		AgentID:     "pm",
		ContextJSON: `{"taskId":"task-wrong-reviewer"}`,
		Status:      "submitted",
		CreatedBy:   "sample/pm",
		CreatedAt:   now.Format(time.RFC3339),
		SubmittedAt: now.Format(time.RFC3339),
		SubmittedBy: "other-user",
	}); err != nil {
		t.Fatalf("create interaction request: %v", err)
	}
	delegationToken := s.issueRuntimeDelegationToken(runtimeDelegationTokenPayload{
		WorkspaceID:   workspaceID,
		Project:       "sample",
		Agent:         "pm",
		UserID:        "other-user",
		InteractionID: "ir_decision_wrong_user",
		Scopes:        []string{"act_as_user"},
	}, time.Minute)

	req := providerTestRequest(http.MethodPost, "/api/v1/runtime/workflow/decision", "", runtimeWorkflowDecisionBody{
		InteractionID:   "ir_decision_wrong_user",
		DelegationToken: delegationToken,
		TaskID:          taskID,
		Decision:        "approve",
	})
	req = req.WithContext(context.WithValue(req.Context(), ctxRuntimeAgentKey, runtimeAgentPrincipal{
		WorkspaceID:  workspaceID,
		Project:      "sample",
		Agent:        "pm",
		Capabilities: []string{"task.use"},
	}))
	rec := httptest.NewRecorder()
	s.handleRuntimeWorkflowDecision(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "not the current workflow reviewer") {
		t.Fatalf("unexpected body=%s", rec.Body.String())
	}
	got, err := s.ts.GetTask("sample", "pm", taskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Status != entity.TaskStatusAwaitingConfirmation {
		t.Fatalf("task status changed to %s", got.Status)
	}
}

func TestRuntimeWorkflowDecisionRequiresDelegationToken(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	req := providerTestRequest(http.MethodPost, "/api/v1/runtime/workflow/decision", "", runtimeWorkflowDecisionBody{
		InteractionID: "ir_missing_delegation",
		TaskID:        "task-one",
		Decision:      "approve",
	})
	req = req.WithContext(context.WithValue(req.Context(), ctxRuntimeAgentKey, runtimeAgentPrincipal{
		WorkspaceID:  workspaceID,
		Project:      "sample",
		Agent:        "pm",
		Capabilities: []string{"task.use"},
	}))
	rec := httptest.NewRecorder()
	s.handleRuntimeWorkflowDecision(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "delegation token is required") {
		t.Fatalf("unexpected body=%s", rec.Body.String())
	}
}
