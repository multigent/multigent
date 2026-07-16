package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multigent/multigent/internal/entity"
)

func TestDeleteRoleTeamAndProjectRequireWorkspaceAdmin(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	grantProjectRoleForTest(t, s, workspaceID, "member", ProjectRoleManager)
	if err := s.st.SaveTeam("engineering", &entity.Team{Name: "engineering"}); err != nil {
		t.Fatalf("team: %v", err)
	}
	if err := s.st.SaveRole("engineering", "backend", &entity.Role{Name: "backend"}); err != nil {
		t.Fatalf("role: %v", err)
	}

	memberRoleRec := httptest.NewRecorder()
	memberRoleReq := providerTestRequest(http.MethodDelete, "/api/v1/teams/engineering/roles/backend", "member", nil)
	memberRoleReq.SetPathValue("team", "engineering")
	memberRoleReq.SetPathValue("role", "backend")
	s.handleDeleteRole(memberRoleRec, memberRoleReq)
	if memberRoleRec.Code != http.StatusForbidden {
		t.Fatalf("member delete role status=%d body=%s", memberRoleRec.Code, memberRoleRec.Body.String())
	}

	adminRoleRec := httptest.NewRecorder()
	adminRoleReq := providerTestRequest(http.MethodDelete, "/api/v1/teams/engineering/roles/backend", "admin", nil)
	adminRoleReq.SetPathValue("team", "engineering")
	adminRoleReq.SetPathValue("role", "backend")
	s.handleDeleteRole(adminRoleRec, adminRoleReq)
	if adminRoleRec.Code != http.StatusOK {
		t.Fatalf("admin delete role status=%d body=%s", adminRoleRec.Code, adminRoleRec.Body.String())
	}
	if _, err := s.st.Role("engineering", "backend"); err == nil {
		t.Fatalf("role still exists")
	}

	memberTeamRec := httptest.NewRecorder()
	memberTeamReq := providerTestRequest(http.MethodDelete, "/api/v1/teams/engineering", "member", nil)
	memberTeamReq.SetPathValue("teamPath", "engineering")
	s.handleDeleteTeam(memberTeamRec, memberTeamReq)
	if memberTeamRec.Code != http.StatusForbidden {
		t.Fatalf("member delete team status=%d body=%s", memberTeamRec.Code, memberTeamRec.Body.String())
	}

	adminTeamRec := httptest.NewRecorder()
	adminTeamReq := providerTestRequest(http.MethodDelete, "/api/v1/teams/engineering", "admin", nil)
	adminTeamReq.SetPathValue("teamPath", "engineering")
	s.handleDeleteTeam(adminTeamRec, adminTeamReq)
	if adminTeamRec.Code != http.StatusOK {
		t.Fatalf("admin delete team status=%d body=%s", adminTeamRec.Code, adminTeamRec.Body.String())
	}
	if _, err := s.st.Team("engineering"); err == nil {
		t.Fatalf("team still exists")
	}

	memberProjectRec := httptest.NewRecorder()
	memberProjectReq := providerTestRequest(http.MethodDelete, "/api/v1/projects/tapnow", "member", nil)
	memberProjectReq.SetPathValue("name", "tapnow")
	s.handleDeleteProject(memberProjectRec, memberProjectReq)
	if memberProjectRec.Code != http.StatusForbidden {
		t.Fatalf("member delete project status=%d body=%s", memberProjectRec.Code, memberProjectRec.Body.String())
	}

	adminProjectRec := httptest.NewRecorder()
	adminProjectReq := providerTestRequest(http.MethodDelete, "/api/v1/projects/tapnow", "admin", nil)
	adminProjectReq.SetPathValue("name", "tapnow")
	s.handleDeleteProject(adminProjectRec, adminProjectReq)
	if adminProjectRec.Code != http.StatusOK {
		t.Fatalf("admin delete project status=%d body=%s", adminProjectRec.Code, adminProjectRec.Body.String())
	}
	if _, err := s.st.Project("tapnow"); err == nil {
		t.Fatalf("project still exists")
	}
	if agents, err := s.st.ListAgents("tapnow"); err != nil || len(agents) != 0 {
		t.Fatalf("project agents after delete len=%d err=%v", len(agents), err)
	}
}
