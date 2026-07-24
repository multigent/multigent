package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multigent/multigent/internal/entity"
)

func TestTeamTemplateCatalogReturnsBuiltinTemplates(t *testing.T) {
	s, _ := newConnectionGrantPolicyServer(t)

	rec := httptest.NewRecorder()
	s.handleTeamTemplates(rec, providerTestRequest(http.MethodGet, "/api/v1/team-templates", "admin", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("catalog status=%d body=%s", rec.Code, rec.Body.String())
	}

	var templates []struct {
		ID       string `json:"id"`
		TeamName string `json:"teamName"`
		Roles    []struct {
			Name string `json:"name"`
		} `json:"roles"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &templates); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	if len(templates) == 0 {
		t.Fatalf("expected at least one builtin template")
	}
	if templates[0].ID != "software-delivery" || templates[0].TeamName != "engineering" {
		t.Fatalf("unexpected first template: %#v", templates[0])
	}
	if len(templates[0].Roles) < 5 {
		t.Fatalf("expected cross-functional roles, got %d", len(templates[0].Roles))
	}
}

func TestApplyTeamTemplateRequiresWorkspaceAdminAndCreatesRoles(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	grantProjectRoleForTest(t, s, workspaceID, "member", ProjectRoleViewer)

	memberRec := httptest.NewRecorder()
	memberReq := providerTestRequest(http.MethodPost, "/api/v1/team-templates/software-delivery/apply", "member", applyTeamTemplateBody{
		TeamName: "delivery",
	})
	memberReq.SetPathValue("id", "software-delivery")
	s.handleApplyTeamTemplate(memberRec, memberReq)
	if memberRec.Code != http.StatusForbidden {
		t.Fatalf("member apply status=%d body=%s", memberRec.Code, memberRec.Body.String())
	}

	adminRec := httptest.NewRecorder()
	adminReq := providerTestRequest(http.MethodPost, "/api/v1/team-templates/software-delivery/apply", "admin", applyTeamTemplateBody{
		TeamName: "delivery",
	})
	adminReq.SetPathValue("id", "software-delivery")
	s.handleApplyTeamTemplate(adminRec, adminReq)
	if adminRec.Code != http.StatusOK {
		t.Fatalf("admin apply status=%d body=%s", adminRec.Code, adminRec.Body.String())
	}

	team, err := s.st.Team("delivery")
	if err != nil {
		t.Fatalf("load created team: %v", err)
	}
	if team.Description == "" || len(team.Goals) == 0 {
		t.Fatalf("created team missing template metadata: %#v", team)
	}
	for _, role := range []string{"product-manager", "ui-designer", "frontend-developer", "backend-developer", "qa-engineer", "code-reviewer"} {
		got, err := s.st.Role("delivery", role)
		if err != nil {
			t.Fatalf("load role %s: %v", role, err)
		}
		if got.Description == "" {
			t.Fatalf("role %s missing description", role)
		}
	}
}

func TestCreateTeamAllowsBlankTeamAndMultipleInstancesFromSameTemplate(t *testing.T) {
	s, _ := newConnectionGrantPolicyServer(t)

	blankRec := httptest.NewRecorder()
	blankReq := providerTestRequest(http.MethodPost, "/api/v1/teams", "admin", createTeamBody{
		Name:        "research",
		Description: "Research and discovery",
	})
	s.handleCreateTeam(blankRec, blankReq)
	if blankRec.Code != http.StatusOK {
		t.Fatalf("blank create status=%d body=%s", blankRec.Code, blankRec.Body.String())
	}
	blank, err := s.st.Team("research")
	if err != nil {
		t.Fatalf("load blank team: %v", err)
	}
	if blank.Description != "Research and discovery" {
		t.Fatalf("blank description=%q", blank.Description)
	}

	for _, name := range []string{"delivery-a", "delivery-b"} {
		rec := httptest.NewRecorder()
		req := providerTestRequest(http.MethodPost, "/api/v1/teams", "admin", createTeamBody{
			Name:       name,
			TemplateID: "software-delivery",
			Locale:     "zh-CN",
		})
		s.handleCreateTeam(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("template create %s status=%d body=%s", name, rec.Code, rec.Body.String())
		}
		role, err := s.st.Role(name, "backend-developer")
		if err != nil {
			t.Fatalf("load templated role for %s: %v", name, err)
		}
		if role.Description == "" || role.Description == "Owns APIs, data model, integrations, permissions, reliability, and backend tests." {
			t.Fatalf("expected localized role description for %s, got %q", name, role.Description)
		}
	}
}

func TestUpdateTeamAndRoleDisplayMetadataRequiresWorkspaceAdmin(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	grantProjectRoleForTest(t, s, workspaceID, "member", ProjectRoleViewer)
	if err := s.st.SaveTeam("engineering", &entity.Team{Name: "Engineering", Description: "Old team"}); err != nil {
		t.Fatalf("save team: %v", err)
	}
	if err := s.st.SaveRole("engineering", "backend", &entity.Role{Name: "Backend", Description: "Old role"}); err != nil {
		t.Fatalf("save role: %v", err)
	}

	memberTeamReq := providerTestRequest(http.MethodPatch, "/api/v1/teams/engineering", "member", updateTeamBody{Name: strPtr("Platform")})
	memberTeamReq.SetPathValue("teamPath", "engineering")
	memberTeamRec := httptest.NewRecorder()
	s.handleUpdateTeam(memberTeamRec, memberTeamReq)
	if memberTeamRec.Code != http.StatusForbidden {
		t.Fatalf("member update team status=%d body=%s", memberTeamRec.Code, memberTeamRec.Body.String())
	}

	adminTeamReq := providerTestRequest(http.MethodPatch, "/api/v1/teams/engineering", "admin", updateTeamBody{Name: strPtr("Platform"), Description: strPtr("New team")})
	adminTeamReq.SetPathValue("teamPath", "engineering")
	adminTeamRec := httptest.NewRecorder()
	s.handleUpdateTeam(adminTeamRec, adminTeamReq)
	if adminTeamRec.Code != http.StatusOK {
		t.Fatalf("admin update team status=%d body=%s", adminTeamRec.Code, adminTeamRec.Body.String())
	}
	team, err := s.st.Team("engineering")
	if err != nil {
		t.Fatalf("load team: %v", err)
	}
	if team.Name != "Platform" || team.Description != "New team" {
		t.Fatalf("team not updated: %#v", team)
	}

	adminRoleReq := providerTestRequest(http.MethodPatch, "/api/v1/teams/engineering/roles/backend", "admin", updateRoleBody{Name: strPtr("API Engineer"), Description: strPtr("New role")})
	adminRoleReq.SetPathValue("team", "engineering")
	adminRoleReq.SetPathValue("role", "backend")
	adminRoleRec := httptest.NewRecorder()
	s.handleUpdateRole(adminRoleRec, adminRoleReq)
	if adminRoleRec.Code != http.StatusOK {
		t.Fatalf("admin update role status=%d body=%s", adminRoleRec.Code, adminRoleRec.Body.String())
	}
	role, err := s.st.Role("engineering", "backend")
	if err != nil {
		t.Fatalf("load role: %v", err)
	}
	if role.Name != "API Engineer" || role.Description != "New role" {
		t.Fatalf("role not updated: %#v", role)
	}
}

func strPtr(v string) *string { return &v }
