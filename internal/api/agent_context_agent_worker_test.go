package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/multigent/multigent/internal/contextpack"
	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/store"
)

func TestGetAgentContextRendersAgentWorkerMembershipContext(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	s.okrStore = store.NewOKRStore(s.root)
	s.msStore = store.NewMilestoneStore(s.root)
	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.controlDB.UpsertAgentWorker(controldb.AgentWorker{
		ID:          "aw-context",
		WorkspaceID: workspaceID,
		Name:        "context-worker",
		DisplayName: "Context Worker",
		Description: "Workspace-level worker description",
		Model:       "codex",
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("worker: %v", err)
	}
	if err := s.controlDB.UpsertProjectMembership(controldb.ProjectMembership{
		ID:               "pm-context",
		WorkspaceID:      workspaceID,
		ProjectID:        "sample",
		MemberType:       "agent_worker",
		MemberID:         "aw-context",
		Role:             "developer",
		Title:            "context-worker",
		Prompt:           "Project-specific membership prompt.",
		AttentionEnabled: true,
		AutoPickTasks:    true,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("membership: %v", err)
	}

	req := providerTestRequest(http.MethodGet, "/api/v1/projects/sample/agents/context-worker/context?includeReadiness=false", "admin", nil)
	req.SetPathValue("name", "sample")
	req.SetPathValue("agent", "context-worker")
	rec := httptest.NewRecorder()
	s.handleGetAgentContext(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("context status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Context string `json:"context"`
		Model   string `json:"model"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Model != "codex" {
		t.Fatalf("model=%q", body.Model)
	}
	if !strings.Contains(body.Context, "Workspace-level worker description") || !strings.Contains(body.Context, "Project-specific membership prompt") {
		t.Fatalf("context did not include worker and membership material:\n%s", body.Context)
	}
}

func TestPutAgentWakeupUsesAgentWorkerSchedule(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	s.okrStore = store.NewOKRStore(s.root)
	s.msStore = store.NewMilestoneStore(s.root)
	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.controlDB.UpsertAgentWorker(controldb.AgentWorker{
		ID:           "aw-wakeup",
		WorkspaceID:  workspaceID,
		Name:         "wakeup-worker",
		DisplayName:  "Wakeup Worker",
		Model:        "codex",
		Status:       "active",
		ScheduleJSON: `{"enabled":true,"interval":"45m"}`,
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("worker: %v", err)
	}
	if err := s.controlDB.UpsertProjectMembership(controldb.ProjectMembership{
		ID:               "pm-wakeup",
		WorkspaceID:      workspaceID,
		ProjectID:        "sample",
		MemberType:       "agent_worker",
		MemberID:         "aw-wakeup",
		Role:             "pm",
		Title:            "wakeup-worker",
		AttentionEnabled: true,
		AutoPickTasks:    true,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("membership: %v", err)
	}

	req := providerTestRequest(http.MethodPut, "/api/v1/projects/sample/agents/wakeup-worker/wakeup", "admin", promptSaveBody{
		Content: "Check attention, tasks, and project risks.",
	})
	req.SetPathValue("name", "sample")
	req.SetPathValue("agent", "wakeup-worker")
	rec := httptest.NewRecorder()
	s.handlePutAgentWakeup(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put wakeup status=%d body=%s", rec.Code, rec.Body.String())
	}
	worker, ok, err := s.controlDB.AgentWorkerByID(workspaceID, "aw-wakeup")
	if err != nil || !ok {
		t.Fatalf("reload worker ok=%v err=%v", ok, err)
	}
	var schedule struct {
		Enabled      bool   `json:"enabled"`
		Interval     string `json:"interval"`
		WakeupPrompt string `json:"wakeupPrompt"`
	}
	if err := json.Unmarshal([]byte(worker.ScheduleJSON), &schedule); err != nil {
		t.Fatalf("schedule json: %v", err)
	}
	if !schedule.Enabled || schedule.Interval != "45m" {
		t.Fatalf("existing schedule fields not preserved: %#v raw=%s", schedule, worker.ScheduleJSON)
	}
	if schedule.WakeupPrompt != "Check attention, tasks, and project risks." {
		t.Fatalf("wakeup prompt not saved to worker schedule: %#v raw=%s", schedule, worker.ScheduleJSON)
	}

	getReq := providerTestRequest(http.MethodGet, "/api/v1/projects/sample/agents/wakeup-worker/context?includeReadiness=false", "admin", nil)
	getReq.SetPathValue("name", "sample")
	getReq.SetPathValue("agent", "wakeup-worker")
	getRec := httptest.NewRecorder()
	s.handleGetAgentContext(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get context status=%d body=%s", getRec.Code, getRec.Body.String())
	}
	var body struct {
		Wakeup string `json:"wakeup"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode context: %v", err)
	}
	if body.Wakeup != "Check attention, tasks, and project risks." {
		t.Fatalf("context wakeup=%q", body.Wakeup)
	}
}

func TestWorkspaceAgentContextAndWakeupDoNotRequireProjectMembership(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	s.okrStore = store.NewOKRStore(s.root)
	s.msStore = store.NewMilestoneStore(s.root)
	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.controlDB.UpsertAgentWorker(controldb.AgentWorker{
		ID:           "aw-workspace-only",
		WorkspaceID:  workspaceID,
		Name:         "workspace-only",
		DisplayName:  "Workspace Only",
		Description:  "Can operate before joining a project.",
		Model:        "codex",
		Status:       "active",
		ScheduleJSON: `{"enabled":true,"interval":"2h"}`,
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("worker: %v", err)
	}
	res, err := contextpack.NewStore(s.root).ImportManual(contextpack.ImportManualInput{
		Title:       "Workspace agent brief",
		Content:     "Read this before work.",
		SourceName:  "brief.md",
		BindScope:   contextpack.ScopeAgent,
		BindScopeID: contextpack.AgentWorkerScopeID("aw-workspace-only"),
		Required:    true,
	})
	if err != nil || res.Binding == nil {
		t.Fatalf("import binding err=%v res=%#v", err, res)
	}

	req := providerTestRequest(http.MethodGet, "/api/v1/agents/aw-workspace-only/context?includeReadiness=false", "admin", nil)
	req.SetPathValue("id", "aw-workspace-only")
	rec := httptest.NewRecorder()
	s.handleGetAgentWorkerContext(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("context status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Context string   `json:"context"`
		Skills  []string `json:"skills"`
		Model   string   `json:"model"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Model != "codex" {
		t.Fatalf("model=%q", body.Model)
	}
	if !strings.Contains(body.Context, "Can operate before joining a project.") || !strings.Contains(body.Context, "Workspace agent brief") {
		t.Fatalf("workspace context missing worker or binding material:\n%s", body.Context)
	}

	putReq := providerTestRequest(http.MethodPut, "/api/v1/agents/aw-workspace-only/wakeup", "admin", promptSaveBody{
		Content: "Check workspace attention and decide what to do next.",
	})
	putReq.SetPathValue("id", "aw-workspace-only")
	putRec := httptest.NewRecorder()
	s.handlePutAgentWorkerWakeup(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("put wakeup status=%d body=%s", putRec.Code, putRec.Body.String())
	}
	worker, ok, err := s.controlDB.AgentWorkerByID(workspaceID, "aw-workspace-only")
	if err != nil || !ok {
		t.Fatalf("reload worker ok=%v err=%v", ok, err)
	}
	if !strings.Contains(worker.ScheduleJSON, "Check workspace attention") {
		t.Fatalf("wakeup prompt not saved: %s", worker.ScheduleJSON)
	}
}
