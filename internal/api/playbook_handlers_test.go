package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multigent/multigent/internal/entity"
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
