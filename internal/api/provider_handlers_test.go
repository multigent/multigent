package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	controldb "github.com/multigent/multigent/internal/db"
)

func newProviderHandlerTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	db, err := controldb.Open(filepath.Join(t.TempDir(), "multigent.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	root := filepath.Join(t.TempDir(), "workspace")
	t.Cleanup(func() { _ = db.Close() })
	now := "2026-07-15T00:00:00Z"
	if err := db.UpsertWorkspace(controldb.Workspace{
		ID:        "ws-one",
		Name:      "One",
		Slug:      "one",
		Root:      root,
		CreatedBy: "owner",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("workspace: %v", err)
	}
	for _, user := range []string{"owner", "member", "outsider"} {
		if err := db.UpsertUser(controldb.User{Username: user, CreatedAt: now}); err != nil {
			t.Fatalf("user %s: %v", user, err)
		}
	}
	if err := db.UpsertWorkspaceMember("ws-one", "owner", WorkspaceRoleOwner); err != nil {
		t.Fatalf("owner member: %v", err)
	}
	if err := db.UpsertWorkspaceMember("ws-one", "member", WorkspaceRoleMember); err != nil {
		t.Fatalf("regular member: %v", err)
	}
	return &Server{
		root:      root,
		controlDB: db,
		users:     newUserStore(db),
	}, root
}

func providerTestRequest(method, path, username string, body any) *http.Request {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req.WithContext(context.WithValue(req.Context(), ctxUserKey, username))
}

func TestModelProviderHandlersRequireWorkspaceAdminForWrites(t *testing.T) {
	s, _ := newProviderHandlerTestServer(t)
	body := providerBody{Name: "Main", Type: "openai", APIKey: "sk-secret", Model: "gpt-test"}

	memberReq := providerTestRequest(http.MethodPost, "/api/v1/providers", "member", body)
	memberRec := httptest.NewRecorder()
	s.handleAddProvider(memberRec, memberReq)
	if memberRec.Code != http.StatusForbidden {
		t.Fatalf("member create status=%d body=%s", memberRec.Code, memberRec.Body.String())
	}

	ownerReq := providerTestRequest(http.MethodPost, "/api/v1/providers", "owner", body)
	ownerRec := httptest.NewRecorder()
	s.handleAddProvider(ownerRec, ownerReq)
	if ownerRec.Code != http.StatusOK {
		t.Fatalf("owner create status=%d body=%s", ownerRec.Code, ownerRec.Body.String())
	}
	if strings.Contains(ownerRec.Body.String(), "sk-secret") {
		t.Fatalf("provider response leaked api key: %s", ownerRec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(ownerRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created provider: %v", err)
	}
	id, _ := created["id"].(string)
	if id == "" || created["hasKey"] != true {
		t.Fatalf("unexpected created response: %#v", created)
	}

	deleteReq := providerTestRequest(http.MethodDelete, "/api/v1/providers/"+id, "member", nil)
	deleteReq.SetPathValue("id", id)
	deleteRec := httptest.NewRecorder()
	s.handleDeleteProvider(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusForbidden {
		t.Fatalf("member delete status=%d body=%s", deleteRec.Code, deleteRec.Body.String())
	}
}

func TestModelProviderHandlersAuditWithoutSecrets(t *testing.T) {
	s, _ := newProviderHandlerTestServer(t)
	req := providerTestRequest(http.MethodPost, "/api/v1/providers", "owner", providerBody{
		Name:   "Main",
		Type:   "anthropic",
		APIKey: "sk-secret",
		Model:  "claude-test",
	})
	rec := httptest.NewRecorder()
	s.handleAddProvider(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	events, err := s.controlDB.ListAuditEvents(controldb.AuditEventFilter{
		WorkspaceID:  "ws-one",
		Action:       "model_provider.create",
		ResourceType: "model_provider",
		Limit:        10,
	})
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events=%#v", events)
	}
	if strings.Contains(events[0].AfterJSON, "sk-secret") || strings.Contains(events[0].BeforeJSON, "sk-secret") {
		t.Fatalf("audit leaked api key: %#v", events[0])
	}
	if !strings.Contains(events[0].AfterJSON, `"hasKey":true`) {
		t.Fatalf("audit missing hasKey metadata: %#v", events[0])
	}
}

func TestModelProviderListRequiresWorkspaceMembership(t *testing.T) {
	s, _ := newProviderHandlerTestServer(t)
	req := providerTestRequest(http.MethodGet, "/api/v1/providers", "outsider", nil)
	rec := httptest.NewRecorder()
	s.handleListProviders(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("outsider list status=%d body=%s", rec.Code, rec.Body.String())
	}
}
