package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
