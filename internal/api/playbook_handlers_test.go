package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/multigent/multigent/internal/entity"
	playbookstore "github.com/multigent/multigent/internal/playbook"
)

func TestPlaybookTemplateHandlers(t *testing.T) {
	s, _ := newConnectionGrantPolicyServer(t)

	listReq := providerTestRequest(http.MethodGet, "/api/v1/playbook-templates?locale=zh-CN", "owner", nil)
	listRec := httptest.NewRecorder()
	s.handleListPlaybookTemplates(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	var listBody struct {
		Templates []entity.PlaybookTemplate `json:"templates"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listBody.Templates) < 4 {
		t.Fatalf("expected builtin playbooks, got %d", len(listBody.Templates))
	}
	if listBody.Templates[0].Locale != "zh-CN" {
		t.Fatalf("locale=%q", listBody.Templates[0].Locale)
	}

	detailReq := providerTestRequest(http.MethodGet, "/api/v1/playbook-templates/garry-startup-validation?locale=zh-CN", "owner", nil)
	detailReq.SetPathValue("playbookId", "garry-startup-validation")
	detailRec := httptest.NewRecorder()
	s.handleGetPlaybookTemplate(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", detailRec.Code, detailRec.Body.String())
	}
	var detail entity.PlaybookTemplate
	if err := json.Unmarshal(detailRec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detail.ID != "garry-startup-validation" || len(detail.Roles) == 0 || len(detail.Skills) == 0 || len(detail.Workflows) == 0 {
		t.Fatalf("unexpected detail=%#v", detail)
	}

	missingReq := providerTestRequest(http.MethodGet, "/api/v1/playbook-templates/missing", "owner", nil)
	missingReq.SetPathValue("playbookId", "missing")
	missingRec := httptest.NewRecorder()
	s.handleGetPlaybookTemplate(missingRec, missingReq)
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("missing status=%d body=%s", missingRec.Code, missingRec.Body.String())
	}
}

func TestPlaybookTemplateHandlersIncludeRemoteRegistry(t *testing.T) {
	s, _ := newConnectionGrantPolicyServer(t)
	registryPath := writePlaybookRegistry(t, playbookstore.RemoteRegistry{
		SchemaVersion: 1,
		Templates: []entity.PlaybookTemplate{
			{
				ID:          "remote-onboarding",
				Version:     "0.1.0",
				Name:        "Remote Onboarding",
				Description: "Fetched from remote registry",
				Category:    "Demo",
				Complexity:  "Easy",
				Skills: []entity.PlaybookSkillTemplate{
					{ID: "remote-skill", Name: "Remote Skill", Body: "Remote body"},
				},
			},
		},
	})
	t.Setenv(playbookstore.EnvRegistryURLs, "file://"+registryPath)

	listReq := providerTestRequest(http.MethodGet, "/api/v1/playbook-templates?locale=en", "owner", nil)
	listRec := httptest.NewRecorder()
	s.handleListPlaybookTemplates(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	var listBody struct {
		Templates []entity.PlaybookTemplate `json:"templates"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	found := false
	for _, tmpl := range listBody.Templates {
		if tmpl.ID == "remote-onboarding" {
			found = true
			if len(tmpl.Skills) != 1 || tmpl.Skills[0].Body != "" {
				t.Fatalf("list should return summary without skill body: %#v", tmpl)
			}
		}
	}
	if !found {
		t.Fatalf("remote template missing from list: %#v", listBody.Templates)
	}

	detailReq := providerTestRequest(http.MethodGet, "/api/v1/playbook-templates/remote-onboarding?locale=en", "owner", nil)
	detailReq.SetPathValue("playbookId", "remote-onboarding")
	detailRec := httptest.NewRecorder()
	s.handleGetPlaybookTemplate(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", detailRec.Code, detailRec.Body.String())
	}
	var detail entity.PlaybookTemplate
	if err := json.Unmarshal(detailRec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detail.ID != "remote-onboarding" || len(detail.Skills) != 1 || detail.Skills[0].Body != "Remote body" {
		t.Fatalf("unexpected remote detail=%#v", detail)
	}
}

func TestInstallPlaybookTemplateCreatesObjectsAndRecordsProvenance(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)

	memberReq := providerTestRequest(http.MethodPost, "/api/v1/playbook-templates/bug-triage-and-fix/install", "owner", installPlaybookRequest{Locale: "zh-CN"})
	memberReq.SetPathValue("playbookId", "bug-triage-and-fix")
	memberRec := httptest.NewRecorder()
	s.handleInstallPlaybookTemplate(memberRec, memberReq)
	if memberRec.Code != http.StatusForbidden {
		t.Fatalf("member install status=%d body=%s", memberRec.Code, memberRec.Body.String())
	}

	adminReq := providerTestRequest(http.MethodPost, "/api/v1/playbook-templates/bug-triage-and-fix/install", "admin", installPlaybookRequest{Locale: "zh-CN"})
	adminReq.SetPathValue("playbookId", "bug-triage-and-fix")
	adminRec := httptest.NewRecorder()
	s.handleInstallPlaybookTemplate(adminRec, adminReq)
	if adminRec.Code != http.StatusCreated {
		t.Fatalf("admin install status=%d body=%s", adminRec.Code, adminRec.Body.String())
	}
	var body installPlaybookResponse
	if err := json.Unmarshal(adminRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode install: %v", err)
	}
	if body.Install.PlaybookID != "bug-triage-and-fix" || len(body.Install.Objects) == 0 {
		t.Fatalf("unexpected install=%#v", body.Install)
	}
	if body.Install.TemplateVersion == "" {
		t.Fatalf("template version was not recorded: %#v", body.Install)
	}
	if _, err := s.st.Skill("root-cause-investigation"); err != nil {
		t.Fatalf("installed skill missing: %v", err)
	}
	if _, err := s.st.Team("engineering"); err != nil {
		t.Fatalf("installed team missing: %v", err)
	}
	if _, err := s.st.Role("engineering", "triage"); err != nil {
		t.Fatalf("installed role missing: %v", err)
	}
	foundWorkflow := false
	for _, obj := range body.Install.Objects {
		if obj.Type == "workflow" {
			foundWorkflow = true
			raw, ok, err := s.controlDB.GetRecord("workflow_definitions", workspaceID, []string{obj.ID})
			if err != nil || !ok || raw == "" {
				t.Fatalf("installed workflow missing: ok=%v err=%v", ok, err)
			}
		}
	}
	if !foundWorkflow {
		t.Fatalf("expected workflow object in install=%#v", body.Install.Objects)
	}
	updateSkillReq := providerTestRequest(http.MethodPut, "/api/v1/skills/root-cause-investigation", "admin", promptSaveBody{Content: "# Updated skill\n"})
	updateSkillReq.SetPathValue("name", "root-cause-investigation")
	updateSkillRec := httptest.NewRecorder()
	s.handlePutSkillPrompt(updateSkillRec, updateSkillReq)
	if updateSkillRec.Code != http.StatusOK {
		t.Fatalf("update skill status=%d body=%s", updateSkillRec.Code, updateSkillRec.Body.String())
	}
	prov := s.playbookObjectProvenanceForRequest(updateSkillReq, "skill", "", "root-cause-investigation")
	if prov == nil || !prov.Customized || prov.CustomizedBy != "admin" || prov.TemplateVersion == "" {
		t.Fatalf("skill provenance not customized: %#v", prov)
	}

	dupReq := providerTestRequest(http.MethodPost, "/api/v1/playbook-templates/bug-triage-and-fix/install", "admin", installPlaybookRequest{Locale: "zh-CN"})
	dupReq.SetPathValue("playbookId", "bug-triage-and-fix")
	dupRec := httptest.NewRecorder()
	s.handleInstallPlaybookTemplate(dupRec, dupReq)
	if dupRec.Code != http.StatusOK {
		t.Fatalf("duplicate status=%d body=%s", dupRec.Code, dupRec.Body.String())
	}
	var dupBody installPlaybookResponse
	if err := json.Unmarshal(dupRec.Body.Bytes(), &dupBody); err != nil {
		t.Fatalf("decode duplicate: %v", err)
	}
	if !dupBody.AlreadyInstalled || dupBody.Install.ID != body.Install.ID {
		t.Fatalf("duplicate not idempotent: %#v", dupBody)
	}
}

func writePlaybookRegistry(t *testing.T, registry playbookstore.RemoteRegistry) string {
	t.Helper()
	body, err := json.Marshal(registry)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "playbooks.json")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
