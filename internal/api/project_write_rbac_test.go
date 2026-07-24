package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/multigent/multigent/internal/entity"
)

func grantProjectRoleForTest(t *testing.T, s *Server, workspaceID, username, role string) {
	t.Helper()
	if err := s.users.CreateUser(username, "pass123", RoleMember, "", "", "", "", ""); err != nil {
		t.Fatalf("create %s: %v", username, err)
	}
	if err := s.controlDB.UpsertWorkspaceMember(workspaceID, username, WorkspaceRoleMember); err != nil {
		t.Fatalf("workspace member %s: %v", username, err)
	}
	if err := s.users.UpdateUser(username, nil, nil, nil, nil, nil, nil, nil, []projectAccess{{Project: "sample", Role: role}}, nil, nil); err != nil {
		t.Fatalf("grant %s project role: %v", username, err)
	}
}

func TestProjectWriteRBACDistinguishesViewerOperatorAndLinkedAgent(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	grantProjectRoleForTest(t, s, workspaceID, "viewer", ProjectRoleViewer)
	grantProjectRoleForTest(t, s, workspaceID, "operator", ProjectRoleOperator)

	taskBody := postTaskBody{Agent: "pm", Title: "Plan launch", Prompt: "Create the launch plan.", Priority: 2}
	viewerRec := httptest.NewRecorder()
	viewerReq := providerTestRequest(http.MethodPost, "/api/v1/projects/sample/tasks", "viewer", taskBody)
	viewerReq.SetPathValue("name", "sample")
	s.handlePostProjectTask(viewerRec, viewerReq)
	if viewerRec.Code != http.StatusForbidden {
		t.Fatalf("viewer create task status=%d body=%s", viewerRec.Code, viewerRec.Body.String())
	}

	operatorRec := httptest.NewRecorder()
	operatorReq := providerTestRequest(http.MethodPost, "/api/v1/projects/sample/tasks", "operator", taskBody)
	operatorReq.SetPathValue("name", "sample")
	s.handlePostProjectTask(operatorRec, operatorReq)
	if operatorRec.Code != http.StatusCreated {
		t.Fatalf("operator create task status=%d body=%s", operatorRec.Code, operatorRec.Body.String())
	}

	linkedRec := httptest.NewRecorder()
	linkedReq := providerTestRequest(http.MethodPost, "/api/v1/projects/sample/tasks", "owner", taskBody)
	linkedReq.SetPathValue("name", "sample")
	s.handlePostProjectTask(linkedRec, linkedReq)
	if linkedRec.Code != http.StatusCreated {
		t.Fatalf("linked owner create own-agent task status=%d body=%s", linkedRec.Code, linkedRec.Body.String())
	}

	otherAgentBody := postTaskBody{Agent: "backend", Title: "Implement API", Prompt: "Build it.", Priority: 2}
	linkedOtherRec := httptest.NewRecorder()
	linkedOtherReq := providerTestRequest(http.MethodPost, "/api/v1/projects/sample/tasks", "owner", otherAgentBody)
	linkedOtherReq.SetPathValue("name", "sample")
	s.handlePostProjectTask(linkedOtherRec, linkedOtherReq)
	if linkedOtherRec.Code != http.StatusForbidden {
		t.Fatalf("linked owner create other-agent task status=%d body=%s", linkedOtherRec.Code, linkedOtherRec.Body.String())
	}
}

func TestProjectAndAgentConfigRequireManager(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	grantProjectRoleForTest(t, s, workspaceID, "viewer", ProjectRoleViewer)
	grantProjectRoleForTest(t, s, workspaceID, "operator", ProjectRoleOperator)

	for _, username := range []string{"viewer", "operator"} {
		rec := httptest.NewRecorder()
		req := providerTestRequest(http.MethodPut, "/api/v1/projects/sample", username, map[string]string{"description": "new"})
		req.SetPathValue("name", "sample")
		s.handlePutProject(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s update project status=%d body=%s", username, rec.Code, rec.Body.String())
		}

		agentRec := httptest.NewRecorder()
		agentReq := providerTestRequest(http.MethodPatch, "/api/v1/projects/sample/agents/pm", username, map[string]string{"name": "pm"})
		agentReq.SetPathValue("name", "sample")
		agentReq.SetPathValue("agent", "pm")
		s.handlePatchAgent(agentRec, agentReq)
		if agentRec.Code != http.StatusForbidden {
			t.Fatalf("%s patch agent status=%d body=%s", username, agentRec.Code, agentRec.Body.String())
		}
	}

	adminReq := providerTestRequest(http.MethodPut, "/api/v1/projects/sample", "admin", map[string]string{"description": "new"})
	adminReq.SetPathValue("name", "sample")
	adminRec := httptest.NewRecorder()
	s.handlePutProject(adminRec, adminReq)
	if adminRec.Code != http.StatusOK {
		t.Fatalf("workspace admin update project status=%d body=%s", adminRec.Code, adminRec.Body.String())
	}
}

func TestPatchAgentAllowsUnicodeName(t *testing.T) {
	s, _ := newConnectionGrantPolicyServer(t)
	if err := s.st.SaveAgentMeta("sample", "开发者", &entity.AgentMeta{
		Name:    "开发者",
		Project: "sample",
		Team:    "engineering",
		Model:   entity.ModelClaudeCode,
		HiredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("save unicode agent: %v", err)
	}

	req := providerTestRequest(http.MethodPatch, "/api/v1/projects/sample/agents/%E5%BC%80%E5%8F%91%E8%80%85", "admin", map[string]string{
		"name":   "开发者",
		"avatar": "https://api.dicebear.com/9.x/notionists/svg?seed=dev",
	})
	req.SetPathValue("name", "sample")
	req.SetPathValue("agent", "开发者")
	rec := httptest.NewRecorder()
	s.handlePatchAgent(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch unicode agent status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSessionResetRequiresAgentManagementAccess(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	grantProjectRoleForTest(t, s, workspaceID, "viewer", ProjectRoleViewer)
	grantProjectRoleForTest(t, s, workspaceID, "operator", ProjectRoleOperator)

	for _, username := range []string{"viewer", "operator"} {
		rec := httptest.NewRecorder()
		s.handleSessionReset(rec, providerTestRequest(http.MethodPost, "/api/v1/session/reset", username, sessionResetBody{
			Project: "sample",
			Agent:   "pm",
		}))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s session reset status=%d body=%s", username, rec.Code, rec.Body.String())
		}
	}

	rec := httptest.NewRecorder()
	s.handleSessionReset(rec, providerTestRequest(http.MethodPost, "/api/v1/session/reset", "owner", sessionResetBody{
		Project: "sample",
		Agent:   "pm",
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("linked owner session reset status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSetModelRequiresAgentManagementAccess(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	grantProjectRoleForTest(t, s, workspaceID, "viewer", ProjectRoleViewer)
	grantProjectRoleForTest(t, s, workspaceID, "operator", ProjectRoleOperator)

	for _, username := range []string{"viewer", "operator"} {
		rec := httptest.NewRecorder()
		req := providerTestRequest(http.MethodPost, "/api/v1/projects/sample/agents/pm/set-model", username, setModelBody{
			Model: "codex",
		})
		req.SetPathValue("name", "sample")
		req.SetPathValue("agent", "pm")
		s.handleSetModel(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s set model status=%d body=%s", username, rec.Code, rec.Body.String())
		}
	}
}

func TestMessageMailboxRBAC(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	grantProjectRoleForTest(t, s, workspaceID, "viewer", ProjectRoleViewer)

	msg := map[string]any{"from": "human", "to": "sample/backend", "body": "hello"}
	rec := httptest.NewRecorder()
	s.handlePostMessage(rec, providerTestRequest(http.MethodPost, "/api/v1/messages", "owner", msg))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("linked owner send to other agent status=%d body=%s", rec.Code, rec.Body.String())
	}

	ownMsg := map[string]any{"from": "human", "to": "sample/pm", "body": "hello"}
	ownRec := httptest.NewRecorder()
	s.handlePostMessage(ownRec, providerTestRequest(http.MethodPost, "/api/v1/messages", "owner", ownMsg))
	if ownRec.Code != http.StatusCreated {
		t.Fatalf("linked owner send to own agent status=%d body=%s", ownRec.Code, ownRec.Body.String())
	}

	viewerRec := httptest.NewRecorder()
	s.handlePostMarkMessageRead(viewerRec, providerTestRequest(http.MethodPost, "/api/v1/messages/mark-read", "viewer", markReadBody{
		Mailbox: "sample/pm",
		ID:      "msg-any",
	}))
	if viewerRec.Code != http.StatusForbidden {
		t.Fatalf("viewer mark mailbox status=%d body=%s", viewerRec.Code, viewerRec.Body.String())
	}
}

func TestLinkedAgentProjectViewsAreFilteredToLinkedAgents(t *testing.T) {
	s, _ := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC()
	for _, tc := range []struct {
		agent string
		title string
	}{
		{agent: "pm", title: "PM task"},
		{agent: "backend", title: "Backend task"},
	} {
		if err := s.ts.AddTask("sample", tc.agent, &entity.Task{
			ID:        entity.NewTaskID(),
			Title:     tc.title,
			Prompt:    tc.title,
			Status:    entity.TaskStatusPending,
			Priority:  2,
			CreatedAt: now,
			UpdatedAt: now,
		}); err != nil {
			t.Fatalf("add task %s: %v", tc.agent, err)
		}
		if err := s.ts.SendMessage(&entity.Message{
			ID:     entity.NewMessageID(),
			From:   "human",
			To:     "sample/" + tc.agent,
			Body:   tc.title,
			SentAt: now,
		}); err != nil {
			t.Fatalf("send message %s: %v", tc.agent, err)
		}
	}

	agentsReq := providerTestRequest(http.MethodGet, "/api/v1/projects/sample/agents", "owner", nil)
	agentsReq.SetPathValue("name", "sample")
	agentsRec := httptest.NewRecorder()
	s.handleProjectAgents(agentsRec, agentsReq)
	if agentsRec.Code != http.StatusOK {
		t.Fatalf("linked agents list status=%d body=%s", agentsRec.Code, agentsRec.Body.String())
	}
	if body := agentsRec.Body.String(); !containsAll(body, "pm") || containsAll(body, "backend") {
		t.Fatalf("linked agents list not filtered: %s", body)
	}

	tasksReq := providerTestRequest(http.MethodGet, "/api/v1/projects/sample/tasks?scope=all", "owner", nil)
	tasksReq.SetPathValue("name", "sample")
	tasksRec := httptest.NewRecorder()
	s.handleProjectTasks(tasksRec, tasksReq)
	if tasksRec.Code != http.StatusOK {
		t.Fatalf("linked task list status=%d body=%s", tasksRec.Code, tasksRec.Body.String())
	}
	var tasks []taskRow
	if err := json.Unmarshal(tasksRec.Body.Bytes(), &tasks); err != nil {
		t.Fatalf("decode tasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Agent != "pm" {
		t.Fatalf("linked task list=%#v", tasks)
	}

	messagesReq := providerTestRequest(http.MethodGet, "/api/v1/projects/sample/messages?archived=all", "owner", nil)
	messagesReq.SetPathValue("name", "sample")
	messagesRec := httptest.NewRecorder()
	s.handleProjectMessages(messagesRec, messagesReq)
	if messagesRec.Code != http.StatusOK {
		t.Fatalf("linked message list status=%d body=%s", messagesRec.Code, messagesRec.Body.String())
	}
	var messages []msgRow
	if err := json.Unmarshal(messagesRec.Body.Bytes(), &messages); err != nil {
		t.Fatalf("decode messages: %v", err)
	}
	if len(messages) != 1 || messages[0].Mailbox != "sample/pm" {
		t.Fatalf("linked message list=%#v", messages)
	}

	backendMailboxReq := providerTestRequest(http.MethodGet, "/api/v1/projects/sample/messages?mailbox=sample/backend", "owner", nil)
	backendMailboxReq.SetPathValue("name", "sample")
	backendMailboxRec := httptest.NewRecorder()
	s.handleProjectMessages(backendMailboxRec, backendMailboxReq)
	if backendMailboxRec.Code != http.StatusForbidden {
		t.Fatalf("linked backend mailbox status=%d body=%s", backendMailboxRec.Code, backendMailboxRec.Body.String())
	}
}

func TestProjectTasksAgentFilterMatchesAssigneeNotQueue(t *testing.T) {
	s, _ := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC()
	cases := []struct {
		queue    string
		id       string
		title    string
		assignee string
	}{
		{queue: "pm", id: "task-queued-pm-assigned-backend", title: "Queued PM assigned backend", assignee: "sample/backend"},
		{queue: "backend", id: "task-queued-backend-assigned-pm", title: "Queued backend assigned PM", assignee: "sample/pm"},
		{queue: "backend", id: "task-queued-backend-assigned-human", title: "Queued backend assigned human", assignee: "human"},
	}
	for _, tc := range cases {
		if err := s.ts.AddTask("sample", tc.queue, &entity.Task{
			ID:        tc.id,
			Title:     tc.title,
			Prompt:    tc.title,
			Status:    entity.TaskStatusPending,
			Priority:  2,
			Assignee:  tc.assignee,
			CreatedAt: now,
			UpdatedAt: now,
		}); err != nil {
			t.Fatalf("add task %s: %v", tc.id, err)
		}
	}

	req := providerTestRequest(http.MethodGet, "/api/v1/projects/sample/tasks?scope=all&agent=pm", "admin", nil)
	req.SetPathValue("name", "sample")
	rec := httptest.NewRecorder()
	s.handleProjectTasks(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("task list status=%d body=%s", rec.Code, rec.Body.String())
	}
	var rows []taskRow
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode tasks: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 PM-assigned task, got %#v", rows)
	}
	if rows[0].ID != "task-queued-backend-assigned-pm" || rows[0].Agent != "backend" || rows[0].Assignee != "sample/pm" {
		t.Fatalf("unexpected filtered task: %#v", rows[0])
	}
}

func TestUpdateTaskAssigneeMovesAgentQueue(t *testing.T) {
	s, _ := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC()
	if err := s.ts.AddTask("sample", "pm", &entity.Task{
		ID:        "task-reassign-agent",
		Title:     "Reassign me",
		Prompt:    "Move this task",
		Status:    entity.TaskStatusPending,
		Priority:  2,
		Assignee:  "sample/pm",
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("add task: %v", err)
	}

	rec := httptest.NewRecorder()
	req := providerTestRequest(http.MethodPut, "/api/v1/tasks/update", "admin", updateTaskBody{
		Project:  "sample",
		Agent:    "pm",
		ID:       "task-reassign-agent",
		Assignee: strPtr("sample/backend"),
	})
	s.handlePutUpdateTask(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update assignee status=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := s.ts.GetTask("sample", "pm", "task-reassign-agent"); err == nil {
		t.Fatalf("task still exists in old queue")
	}
	moved, err := s.ts.GetTask("sample", "backend", "task-reassign-agent")
	if err != nil {
		t.Fatalf("task not moved to backend queue: %v", err)
	}
	if moved.Assignee != "sample/backend" {
		t.Fatalf("assignee=%q", moved.Assignee)
	}
}

func containsAll(s string, substr string) bool {
	return strings.Contains(s, substr)
}
