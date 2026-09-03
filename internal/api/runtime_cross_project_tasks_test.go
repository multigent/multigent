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
	body := `{"templateId":"tt-target-review","project":"target","inputs":{"repo":"example-org/example-repo","pr":"42","url":"https://github.com/example-org/example-repo/pull/42"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/tasks/from-template", strings.NewReader(body))
	req = runtimePrincipalContext(req, principal)
	rec := httptest.NewRecorder()
	s.handleRuntimePostTaskFromTemplate(rec, req)
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
