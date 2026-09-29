package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/tasktemplate"
)

func runtimePrincipalContext(req *http.Request, principal runtimeAgentPrincipal) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), ctxRuntimeAgentKey, principal))
}

func TestRuntimeTaskFromTemplateCanDispatchToAuthorizedProject(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	if err := s.st.SaveProject("target", &entity.Project{Name: "target"}); err != nil {
		t.Fatalf("save target project: %v", err)
	}
	seedAgentWorkerWithIDForTest(t, s, workspaceID, "sample", "dispatcher", "aw-dispatcher", "pm-dispatcher-sample")
	seedAgentWorkerWithIDForTest(t, s, workspaceID, "target", "dispatcher", "aw-dispatcher", "pm-dispatcher-target")
	seedAgentWorkerWithIDForTest(t, s, workspaceID, "target", "reviewer", "aw-reviewer", "pm-reviewer-target")

	if err := tasktemplate.NewStore(s.controlDB, workspaceID).Save(&entity.TaskTemplate{
		ID:             "tt-target-review",
		Name:           "Target review",
		Project:        "target",
		Type:           string(entity.TaskTypeReview),
		TitleTemplate:  "Review {{repo}}#{{pr}}",
		PromptTemplate: "Review {{url}}",
		WorkflowActorBindings: map[string]entity.WorkflowActorBinding{
			"start": {Type: "agent", ID: "reviewer"},
		},
		Variables: []entity.TaskTemplateVariable{
			{Name: "repo", Required: true},
			{Name: "pr", Required: true},
			{Name: "url", Required: true},
		},
	}); err != nil {
		t.Fatalf("save template: %v", err)
	}

	principal := runtimeAgentPrincipal{
		WorkspaceID:  workspaceID,
		Project:      "sample",
		Agent:        "dispatcher",
		Capabilities: []string{"task.use"},
	}
	body := `{"templateId":"tt-target-review","project":"target","idempotencyKey":"review-example-42","inputs":{"repo":"example-org/example-repo","pr":"42","url":"https://github.com/example-org/example-repo/pull/42"}}`
	create := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/tasks/from-template", strings.NewReader(body))
		req = runtimePrincipalContext(req, principal)
		rec := httptest.NewRecorder()
		s.handleRuntimePostTaskFromTemplate(rec, req)
		return rec
	}
	rec := create()
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var row map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &row); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if row["project"] != "target" || row["agent"] != "reviewer" {
		t.Fatalf("unexpected target: %#v", row)
	}
	if _, err := s.ts.GetTask("target", "reviewer", row["id"].(string)); err != nil {
		t.Fatalf("target task not stored for reviewer: %v", err)
	}
	if _, err := s.ts.GetTask("sample", "dispatcher", row["id"].(string)); err == nil {
		t.Fatal("cross-project task was incorrectly stored in source project")
	}

	created, err := s.ts.GetTask("target", "reviewer", row["id"].(string))
	if err != nil {
		t.Fatalf("get created task: %v", err)
	}
	created.Status = entity.TaskStatusDoneSuccess
	if err := s.ts.ArchiveTask("target", "reviewer", created); err != nil {
		t.Fatalf("archive created task: %v", err)
	}

	replayRec := create()
	if replayRec.Code != http.StatusOK {
		t.Fatalf("replay status=%d body=%s", replayRec.Code, replayRec.Body.String())
	}
	var replay struct {
		Task             map[string]any `json:"task"`
		IdempotentReplay bool           `json:"idempotentReplay"`
	}
	if err := json.Unmarshal(replayRec.Body.Bytes(), &replay); err != nil {
		t.Fatalf("decode replay: %v", err)
	}
	if !replay.IdempotentReplay || replay.Task["id"] != row["id"] {
		t.Fatalf("unexpected replay: %#v", replay)
	}
	active, err := s.ts.ListTasks("target", "reviewer")
	if err != nil {
		t.Fatalf("list active target tasks: %v", err)
	}
	archived, err := s.ts.ListArchivedTasks("target", "reviewer")
	if err != nil {
		t.Fatalf("list archived target tasks: %v", err)
	}
	if len(active) != 0 || len(archived) != 1 {
		t.Fatalf("idempotent retry left active=%d archived=%d tasks", len(active), len(archived))
	}
}

func TestRuntimeTaskFromTemplateRejectsUnauthorizedProject(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	if err := s.st.SaveProject("target", &entity.Project{Name: "target"}); err != nil {
		t.Fatalf("save target project: %v", err)
	}
	seedAgentWorkerWithIDForTest(t, s, workspaceID, "sample", "dispatcher", "aw-dispatcher", "pm-dispatcher-sample")
	seedAgentWorkerWithIDForTest(t, s, workspaceID, "target", "reviewer", "aw-reviewer", "pm-reviewer-target")
	if err := tasktemplate.NewStore(s.controlDB, workspaceID).Save(&entity.TaskTemplate{
		ID:             "tt-target-review",
		Name:           "Target review",
		Project:        "target",
		TitleTemplate:  "Review",
		PromptTemplate: "Review",
	}); err != nil {
		t.Fatalf("save template: %v", err)
	}

	principal := runtimeAgentPrincipal{WorkspaceID: workspaceID, Project: "sample", Agent: "dispatcher", Capabilities: []string{"task.use"}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/tasks/from-template", strings.NewReader(`{"templateId":"tt-target-review","project":"target"}`))
	req = runtimePrincipalContext(req, principal)
	rec := httptest.NewRecorder()
	s.handleRuntimePostTaskFromTemplate(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRuntimeTaskTemplatesCanListAuthorizedTargetProject(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	if err := s.st.SaveProject("target", &entity.Project{Name: "target"}); err != nil {
		t.Fatalf("save target project: %v", err)
	}
	seedAgentWorkerWithIDForTest(t, s, workspaceID, "sample", "dispatcher", "aw-dispatcher", "pm-dispatcher-sample")
	seedAgentWorkerWithIDForTest(t, s, workspaceID, "target", "dispatcher", "aw-dispatcher", "pm-dispatcher-target")
	if err := tasktemplate.NewStore(s.controlDB, workspaceID).Save(&entity.TaskTemplate{ID: "tt-target", Name: "Target", Project: "target"}); err != nil {
		t.Fatalf("save template: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/task-templates?project=target", nil)
	req = runtimePrincipalContext(req, runtimeAgentPrincipal{WorkspaceID: workspaceID, Project: "sample", Agent: "dispatcher", Capabilities: []string{"task.use"}})
	rec := httptest.NewRecorder()
	s.handleRuntimeTaskTemplates(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "tt-target") || !strings.Contains(rec.Body.String(), `"project":"target"`) {
		t.Fatalf("target template missing: %s", rec.Body.String())
	}
}
