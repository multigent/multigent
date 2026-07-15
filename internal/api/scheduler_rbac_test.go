package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGlobalSchedulerActionsRequireWorkspaceAdmin(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	s.sched = newSchedulerManager(s.root)
	if err := s.users.CreateUser("member", "pass123", RoleMember, "", "", "", "", ""); err != nil {
		t.Fatalf("create member: %v", err)
	}
	if err := s.controlDB.UpsertWorkspaceMember(workspaceID, "member", WorkspaceRoleMember); err != nil {
		t.Fatalf("member workspace role: %v", err)
	}

	memberRec := httptest.NewRecorder()
	s.handleSchedulerStop(memberRec, providerTestRequest(http.MethodPost, "/api/v1/scheduler/stop", "member", map[string]string{}))
	if memberRec.Code != http.StatusForbidden {
		t.Fatalf("member global stop status=%d body=%s", memberRec.Code, memberRec.Body.String())
	}

	adminRec := httptest.NewRecorder()
	s.handleSchedulerStop(adminRec, providerTestRequest(http.MethodPost, "/api/v1/scheduler/stop", "admin", map[string]string{}))
	if adminRec.Code == http.StatusForbidden {
		t.Fatalf("workspace admin global stop should pass auth, body=%s", adminRec.Body.String())
	}
	if adminRec.Code != http.StatusNotFound {
		t.Fatalf("workspace admin global stop should reach scheduler layer, status=%d body=%s", adminRec.Code, adminRec.Body.String())
	}
}
