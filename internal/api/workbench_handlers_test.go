package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/multigent/multigent/internal/entity"
)

func TestWorkbenchTasksIncludeDirectHumanAssigneeWithoutProjectAccess(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	if err := s.users.CreateUser("member1", "pass123", RoleMember, "Dashell", "dashell@example.test", "", "", ""); err != nil {
		t.Fatalf("create member1: %v", err)
	}
	if err := s.controlDB.UpsertWorkspaceMember(workspaceID, "member1", WorkspaceRoleMember); err != nil {
		t.Fatalf("workspace member1: %v", err)
	}

	now := time.Now().UTC()
	task := &entity.Task{
		ID:        "t-review",
		Title:     "Review product spec",
		Type:      entity.TaskTypeReview,
		Priority:  2,
		Assignee:  "member1",
		CreatedBy: "owner",
		Status:    entity.TaskStatusAwaitingConfirmation,
		Prompt:    "Review the workflow output.",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.ts.AddTask("sample", "pm", task); err != nil {
		t.Fatalf("add task: %v", err)
	}

	rec := httptest.NewRecorder()
	req := providerTestRequest(http.MethodGet, "/api/v1/workbench/tasks", "member1", nil)
	s.handleWorkbenchTasks(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var rows []taskRow
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode rows: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != "t-review" {
		t.Fatalf("expected assigned review task, got %#v", rows)
	}
	if rows[0].AssigneeLabel != "Dashell" {
		t.Fatalf("assignee label=%q", rows[0].AssigneeLabel)
	}
}

func TestWorkbenchTasksExcludeProjectTaskNotAssignedToMember(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	grantProjectRoleForTest(t, s, workspaceID, "member1", ProjectRoleViewer)

	now := time.Now().UTC()
	task := &entity.Task{
		ID:        "t-agent",
		Title:     "PM rework",
		Type:      entity.TaskTypeChore,
		Priority:  2,
		Assignee:  "sample/pm",
		CreatedBy: "owner",
		Status:    entity.TaskStatusPending,
		Prompt:    "Revise the spec.",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.ts.AddTask("sample", "pm", task); err != nil {
		t.Fatalf("add task: %v", err)
	}

	rec := httptest.NewRecorder()
	req := providerTestRequest(http.MethodGet, "/api/v1/workbench/tasks", "member1", nil)
	s.handleWorkbenchTasks(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var rows []taskRow
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode rows: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("project member workbench should only show direct responsibilities, got %#v", rows)
	}
}

func TestWorkbenchTasksIncludeAdminDirectAssigneeInAgentQueue(t *testing.T) {
	s, _ := newConnectionGrantPolicyServer(t)

	now := time.Now().UTC()
	task := &entity.Task{
		ID:        "t-admin-review",
		Title:     "Admin review",
		Type:      entity.TaskTypeReview,
		Priority:  2,
		Assignee:  "admin",
		CreatedBy: "owner",
		Status:    entity.TaskStatusAwaitingConfirmation,
		Prompt:    "Review the workflow output.",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.ts.AddTask("sample", "backend", task); err != nil {
		t.Fatalf("add task: %v", err)
	}

	rec := httptest.NewRecorder()
	req := providerTestRequest(http.MethodGet, "/api/v1/workbench/tasks", "admin", nil)
	s.handleWorkbenchTasks(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var rows []taskRow
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode rows: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != "t-admin-review" {
		t.Fatalf("expected admin direct review task, got %#v", rows)
	}
}
