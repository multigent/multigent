package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
)

func TestAgentWorkerAndProjectMembershipHandlers(t *testing.T) {
	s, _ := newConnectionGrantPolicyServer(t)

	createReq := providerTestRequest(http.MethodPost, "/api/v1/agents", "admin", agentWorkerRequest{
		Name:        "nova",
		DisplayName: "Nova",
		Description: "Workspace project manager",
		ProfilePrompt: strings.Join([]string{
			"Glenn is my final escalation owner.",
			"Small safe changes can be handled autonomously.",
		}, "\n"),
		Skills: []string{"github", "lark"},
		Schedule: map[string]any{
			"interval": "2h",
			"triggers": []string{"message", "im_direct_message", "unknown", "message"},
		},
		AttentionPolicy: map[string]any{
			"im_direct_message": true,
		},
	})
	createRec := httptest.NewRecorder()
	s.handleCreateAgentWorker(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("create worker status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		Agent map[string]any `json:"agent"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	workerID, _ := created.Agent["id"].(string)
	if workerID == "" {
		t.Fatalf("missing worker id: %#v", created.Agent)
	}
	if got := created.Agent["profilePrompt"]; got == nil || !strings.Contains(got.(string), "Glenn is my final escalation owner") {
		t.Fatalf("missing profile prompt in create response: %#v", created.Agent)
	}
	schedule, _ := created.Agent["schedule"].(map[string]any)
	triggers, _ := schedule["triggers"].([]any)
	if len(triggers) != 2 || triggers[0] != "message" || triggers[1] != "im_direct_message" {
		t.Fatalf("schedule triggers were not normalized: %#v", created.Agent["schedule"])
	}

	listReq := providerTestRequest(http.MethodGet, "/api/v1/agents", "admin", nil)
	listRec := httptest.NewRecorder()
	s.handleAgentWorkers(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list workers status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	var listed struct {
		Agents []map[string]any `json:"agents"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listed.Agents) != 1 || listed.Agents[0]["name"] != "nova" {
		t.Fatalf("unexpected workers: %#v", listed.Agents)
	}
	if err := s.st.SaveTeam("product", &entity.Team{Name: "product"}); err != nil {
		t.Fatalf("save team: %v", err)
	}
	if err := s.st.SaveRole("product", "project-manager", &entity.Role{Name: "project-manager"}); err != nil {
		t.Fatalf("save role: %v", err)
	}

	memberReq := providerTestRequest(http.MethodPost, "/api/v1/projects/sample/memberships", "admin", projectMembershipRequest{
		WorkerID:      workerID,
		Role:          "project-manager",
		Title:         "项目管理者",
		AutoPickTasks: boolPtr(true),
		Permissions:   []string{"task.read", "task.write"},
	})
	memberReq.SetPathValue("name", "sample")
	memberRec := httptest.NewRecorder()
	s.handleCreateProjectMembership(memberRec, memberReq)
	if memberRec.Code != http.StatusOK {
		t.Fatalf("create membership status=%d body=%s", memberRec.Code, memberRec.Body.String())
	}

	projectReq := providerTestRequest(http.MethodGet, "/api/v1/projects/sample/memberships", "owner", nil)
	projectReq.SetPathValue("name", "sample")
	projectRec := httptest.NewRecorder()
	s.handleProjectMemberships(projectRec, projectReq)
	if projectRec.Code != http.StatusOK {
		t.Fatalf("list memberships status=%d body=%s", projectRec.Code, projectRec.Body.String())
	}
	var projectMembers struct {
		Memberships []map[string]any `json:"memberships"`
	}
	if err := json.Unmarshal(projectRec.Body.Bytes(), &projectMembers); err != nil {
		t.Fatalf("decode memberships: %v", err)
	}
	if len(projectMembers.Memberships) != 1 {
		t.Fatalf("unexpected memberships: %#v", projectMembers.Memberships)
	}
	if projectMembers.Memberships[0]["memberType"] != "agent_worker" {
		t.Fatalf("unexpected member type: %#v", projectMembers.Memberships[0])
	}
	if _, ok := projectMembers.Memberships[0]["agent"].(map[string]any); !ok {
		t.Fatalf("membership did not include agent: %#v", projectMembers.Memberships[0])
	}
	listRec = httptest.NewRecorder()
	s.handleAgentWorkers(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list workers after membership status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list after membership: %v", err)
	}
	memberships, _ := listed.Agents[0]["memberships"].([]any)
	if len(memberships) != 1 {
		t.Fatalf("expected membership in worker response: %#v", listed.Agents[0])
	}
	membership, _ := memberships[0].(map[string]any)
	if membership["team"] != "product" || membership["role"] != "project-manager" {
		t.Fatalf("worker membership should include derived team and role: %#v", membership)
	}
}

func TestProjectAgentsEndpointReadsAgentWorkerMemberships(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := "2026-08-21T00:00:00Z"
	worker := controldb.AgentWorker{
		ID:          "aw-dev-codex",
		WorkspaceID: workspaceID,
		Name:        "cc-connect-dev-codex",
		DisplayName: "Dev Codex",
		Model:       "codex",
		Avatar:      "avatar.png",
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
		t.Fatalf("worker: %v", err)
	}
	if err := s.controlDB.UpsertProjectMembership(controldb.ProjectMembership{
		ID:          "pm-dev-codex",
		WorkspaceID: workspaceID,
		ProjectID:   "sample",
		MemberType:  "agent_worker",
		MemberID:    worker.ID,
		Role:        "developer",
		Title:       "dev-codex",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("membership: %v", err)
	}
	req := providerTestRequest(http.MethodGet, "/api/v1/projects/sample/agents", "admin", nil)
	req.SetPathValue("name", "sample")
	rec := httptest.NewRecorder()
	s.handleProjectAgents(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list agents status=%d body=%s", rec.Code, rec.Body.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode agents: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected only membership-backed agent, got %#v", rows)
	}
	if rows[0]["name"] != "dev-codex" || rows[0]["agentWorkerName"] != "cc-connect-dev-codex" || rows[0]["projectMembershipId"] != "pm-dev-codex" {
		t.Fatalf("unexpected agent row: %#v", rows[0])
	}
}

func TestAgentWorkersListFiltersByProjectAndAgentAccess(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := "2026-08-21T00:00:00Z"
	for _, worker := range []controldb.AgentWorker{
		{ID: "aw-pm", WorkspaceID: workspaceID, Name: "pm-worker", DisplayName: "PM Worker", Status: "active", CreatedAt: now, UpdatedAt: now},
		{ID: "aw-backend", WorkspaceID: workspaceID, Name: "backend-worker", DisplayName: "Backend Worker", Status: "active", CreatedAt: now, UpdatedAt: now},
	} {
		if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
			t.Fatalf("worker %s: %v", worker.ID, err)
		}
	}
	for _, membership := range []controldb.ProjectMembership{
		{ID: "pm-sample-pm", WorkspaceID: workspaceID, ProjectID: "sample", MemberType: "agent_worker", MemberID: "aw-pm", Role: "pm", Title: "pm", CreatedAt: now, UpdatedAt: now},
		{ID: "pm-sample-backend", WorkspaceID: workspaceID, ProjectID: "sample", MemberType: "agent_worker", MemberID: "aw-backend", Role: "developer", Title: "backend", CreatedAt: now, UpdatedAt: now},
	} {
		if err := s.controlDB.UpsertProjectMembership(membership); err != nil {
			t.Fatalf("membership %s: %v", membership.ID, err)
		}
	}

	ownerReq := providerTestRequest(http.MethodGet, "/api/v1/agents", "owner", nil)
	ownerRec := httptest.NewRecorder()
	s.handleAgentWorkers(ownerRec, ownerReq)
	if ownerRec.Code != http.StatusOK {
		t.Fatalf("owner list status=%d body=%s", ownerRec.Code, ownerRec.Body.String())
	}
	var ownerBody struct {
		Agents []map[string]any `json:"agents"`
	}
	if err := json.Unmarshal(ownerRec.Body.Bytes(), &ownerBody); err != nil {
		t.Fatalf("decode owner list: %v", err)
	}
	if len(ownerBody.Agents) != 1 || ownerBody.Agents[0]["id"] != "aw-pm" {
		t.Fatalf("owner should only see explicitly granted agent, got %#v", ownerBody.Agents)
	}

	backendReq := providerTestRequest(http.MethodGet, "/api/v1/agents/aw-backend", "owner", nil)
	backendReq.SetPathValue("id", "aw-backend")
	backendRec := httptest.NewRecorder()
	s.handleAgentWorker(backendRec, backendReq)
	if backendRec.Code != http.StatusForbidden {
		t.Fatalf("owner backend detail status=%d body=%s", backendRec.Code, backendRec.Body.String())
	}

	grantProjectRoleForTest(t, s, workspaceID, "viewer", ProjectRoleViewer)
	viewerReq := providerTestRequest(http.MethodGet, "/api/v1/agents", "viewer", nil)
	viewerRec := httptest.NewRecorder()
	s.handleAgentWorkers(viewerRec, viewerReq)
	if viewerRec.Code != http.StatusOK {
		t.Fatalf("viewer list status=%d body=%s", viewerRec.Code, viewerRec.Body.String())
	}
	var viewerBody struct {
		Agents []map[string]any `json:"agents"`
	}
	if err := json.Unmarshal(viewerRec.Body.Bytes(), &viewerBody); err != nil {
		t.Fatalf("decode viewer list: %v", err)
	}
	if len(viewerBody.Agents) != 2 {
		t.Fatalf("project viewer should see project agents, got %#v", viewerBody.Agents)
	}
}

func TestPatchProjectAgentUpdatesMembershipBackedWorker(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := "2026-08-21T00:00:00Z"
	worker := controldb.AgentWorker{
		ID:          "aw-worker-patch",
		WorkspaceID: workspaceID,
		Name:        "sample-worker-patch",
		DisplayName: "Worker Patch",
		Model:       "codex",
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
		t.Fatalf("worker: %v", err)
	}
	if err := s.controlDB.UpsertProjectMembership(controldb.ProjectMembership{
		ID:          "pm-worker-patch",
		WorkspaceID: workspaceID,
		ProjectID:   "sample",
		MemberType:  "agent_worker",
		MemberID:    worker.ID,
		Role:        "developer",
		Title:       "dev-old",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("membership: %v", err)
	}
	if err := s.controlDB.UpsertRuntimeNode(controldb.RuntimeNode{
		ID:          "rnode-one",
		WorkspaceID: workspaceID,
		Name:        "node one",
		Status:      "online",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("runtime node: %v", err)
	}
	req := providerTestRequest(http.MethodPatch, "/api/v1/projects/sample/agents/dev-old", "admin", map[string]any{
		"name":          "dev-new",
		"avatar":        "avatar-new.png",
		"runtimeNodeId": "rnode-one",
	})
	req.SetPathValue("name", "sample")
	req.SetPathValue("agent", "dev-old")
	rec := httptest.NewRecorder()
	s.handlePatchAgent(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch agent status=%d body=%s", rec.Code, rec.Body.String())
	}
	updatedWorker, ok, err := s.controlDB.AgentWorkerByID(workspaceID, worker.ID)
	if err != nil || !ok {
		t.Fatalf("worker lookup ok=%v err=%v", ok, err)
	}
	if updatedWorker.Avatar != "avatar-new.png" || updatedWorker.DefaultRuntimeNodeID != "rnode-one" {
		t.Fatalf("worker was not updated: %#v", updatedWorker)
	}
	membership, ok, err := s.controlDB.ProjectMembershipByID(workspaceID, "pm-worker-patch")
	if err != nil || !ok {
		t.Fatalf("membership lookup ok=%v err=%v", ok, err)
	}
	if membership.Title != "dev-new" {
		t.Fatalf("membership title=%q", membership.Title)
	}
	if _, err := os.Stat(s.st.AgentDir("sample", "dev-new")); err == nil {
		t.Fatalf("2.x patch should not create legacy agent dir")
	} else if !os.IsNotExist(err) {
		t.Fatalf("legacy agent dir stat: %v", err)
	}
}

func boolPtr(v bool) *bool {
	return &v
}
