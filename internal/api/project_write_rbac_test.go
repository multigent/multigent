package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func grantProjectRoleForTest(t *testing.T, s *Server, workspaceID, username, role string) {
	t.Helper()
	if err := s.users.CreateUser(username, "pass123", RoleMember, "", "", "", "", ""); err != nil {
		t.Fatalf("create %s: %v", username, err)
	}
	if err := s.controlDB.UpsertWorkspaceMember(workspaceID, username, WorkspaceRoleMember); err != nil {
		t.Fatalf("workspace member %s: %v", username, err)
	}
	if err := s.users.UpdateUser(username, nil, nil, nil, nil, nil, nil, nil, []projectAccess{{Project: "tapnow", Role: role}}, nil, nil); err != nil {
		t.Fatalf("grant %s project role: %v", username, err)
	}
}

func TestProjectWriteRBACDistinguishesViewerOperatorAndLinkedAgent(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	grantProjectRoleForTest(t, s, workspaceID, "viewer", ProjectRoleViewer)
	grantProjectRoleForTest(t, s, workspaceID, "operator", ProjectRoleOperator)

	taskBody := postTaskBody{Agent: "pm", Title: "Plan launch", Prompt: "Create the launch plan.", Priority: 2}
	viewerRec := httptest.NewRecorder()
	viewerReq := providerTestRequest(http.MethodPost, "/api/v1/projects/tapnow/tasks", "viewer", taskBody)
	viewerReq.SetPathValue("name", "tapnow")
	s.handlePostProjectTask(viewerRec, viewerReq)
	if viewerRec.Code != http.StatusForbidden {
		t.Fatalf("viewer create task status=%d body=%s", viewerRec.Code, viewerRec.Body.String())
	}

	operatorRec := httptest.NewRecorder()
	operatorReq := providerTestRequest(http.MethodPost, "/api/v1/projects/tapnow/tasks", "operator", taskBody)
	operatorReq.SetPathValue("name", "tapnow")
	s.handlePostProjectTask(operatorRec, operatorReq)
	if operatorRec.Code != http.StatusCreated {
		t.Fatalf("operator create task status=%d body=%s", operatorRec.Code, operatorRec.Body.String())
	}

	linkedRec := httptest.NewRecorder()
	linkedReq := providerTestRequest(http.MethodPost, "/api/v1/projects/tapnow/tasks", "owner", taskBody)
	linkedReq.SetPathValue("name", "tapnow")
	s.handlePostProjectTask(linkedRec, linkedReq)
	if linkedRec.Code != http.StatusCreated {
		t.Fatalf("linked owner create own-agent task status=%d body=%s", linkedRec.Code, linkedRec.Body.String())
	}

	otherAgentBody := postTaskBody{Agent: "backend", Title: "Implement API", Prompt: "Build it.", Priority: 2}
	linkedOtherRec := httptest.NewRecorder()
	linkedOtherReq := providerTestRequest(http.MethodPost, "/api/v1/projects/tapnow/tasks", "owner", otherAgentBody)
	linkedOtherReq.SetPathValue("name", "tapnow")
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
		req := providerTestRequest(http.MethodPut, "/api/v1/projects/tapnow", username, map[string]string{"description": "new"})
		req.SetPathValue("name", "tapnow")
		s.handlePutProject(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s update project status=%d body=%s", username, rec.Code, rec.Body.String())
		}

		agentRec := httptest.NewRecorder()
		agentReq := providerTestRequest(http.MethodPatch, "/api/v1/projects/tapnow/agents/pm", username, map[string]string{"name": "pm"})
		agentReq.SetPathValue("name", "tapnow")
		agentReq.SetPathValue("agent", "pm")
		s.handlePatchAgent(agentRec, agentReq)
		if agentRec.Code != http.StatusForbidden {
			t.Fatalf("%s patch agent status=%d body=%s", username, agentRec.Code, agentRec.Body.String())
		}
	}

	adminReq := providerTestRequest(http.MethodPut, "/api/v1/projects/tapnow", "admin", map[string]string{"description": "new"})
	adminReq.SetPathValue("name", "tapnow")
	adminRec := httptest.NewRecorder()
	s.handlePutProject(adminRec, adminReq)
	if adminRec.Code != http.StatusOK {
		t.Fatalf("workspace admin update project status=%d body=%s", adminRec.Code, adminRec.Body.String())
	}
}

func TestMessageMailboxRBAC(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	grantProjectRoleForTest(t, s, workspaceID, "viewer", ProjectRoleViewer)

	msg := map[string]any{"from": "human", "to": "tapnow/backend", "body": "hello"}
	rec := httptest.NewRecorder()
	s.handlePostMessage(rec, providerTestRequest(http.MethodPost, "/api/v1/messages", "owner", msg))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("linked owner send to other agent status=%d body=%s", rec.Code, rec.Body.String())
	}

	ownMsg := map[string]any{"from": "human", "to": "tapnow/pm", "body": "hello"}
	ownRec := httptest.NewRecorder()
	s.handlePostMessage(ownRec, providerTestRequest(http.MethodPost, "/api/v1/messages", "owner", ownMsg))
	if ownRec.Code != http.StatusCreated {
		t.Fatalf("linked owner send to own agent status=%d body=%s", ownRec.Code, ownRec.Body.String())
	}

	viewerRec := httptest.NewRecorder()
	s.handlePostMarkMessageRead(viewerRec, providerTestRequest(http.MethodPost, "/api/v1/messages/mark-read", "viewer", markReadBody{
		Mailbox: "tapnow/pm",
		ID:      "msg-any",
	}))
	if viewerRec.Code != http.StatusForbidden {
		t.Fatalf("viewer mark mailbox status=%d body=%s", viewerRec.Code, viewerRec.Body.String())
	}
}
