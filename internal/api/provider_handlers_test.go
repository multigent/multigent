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
	"time"

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

func TestModelProviderHandlersScopeWritesByOwner(t *testing.T) {
	s, _ := newProviderHandlerTestServer(t)
	body := providerBody{Name: "Main", Type: "openai", APIKey: "sk-secret", Model: "gpt-test"}

	memberReq := providerTestRequest(http.MethodPost, "/api/v1/providers", "member", body)
	memberRec := httptest.NewRecorder()
	s.handleAddProvider(memberRec, memberReq)
	if memberRec.Code != http.StatusOK {
		t.Fatalf("member personal create status=%d body=%s", memberRec.Code, memberRec.Body.String())
	}
	var memberCreated map[string]any
	if err := json.Unmarshal(memberRec.Body.Bytes(), &memberCreated); err != nil {
		t.Fatalf("decode member provider: %v", err)
	}
	if memberCreated["ownerType"] != ConnectionOwnerUser || memberCreated["ownerId"] != "member" {
		t.Fatalf("member provider should be personal: %#v", memberCreated)
	}

	memberWorkspaceReq := providerTestRequest(http.MethodPost, "/api/v1/providers", "member", providerBody{
		OwnerType: ConnectionOwnerWorkspace,
		Name:      "Workspace",
		Type:      "openai",
	})
	memberWorkspaceRec := httptest.NewRecorder()
	s.handleAddProvider(memberWorkspaceRec, memberWorkspaceReq)
	if memberWorkspaceRec.Code != http.StatusForbidden {
		t.Fatalf("member workspace create status=%d body=%s", memberWorkspaceRec.Code, memberWorkspaceRec.Body.String())
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
	if created["ownerType"] != ConnectionOwnerWorkspace || created["ownerId"] != "ws-one" {
		t.Fatalf("owner provider should default to workspace scope: %#v", created)
	}
	stored, ok, err := s.controlDB.ModelProviderByID("ws-one", id)
	if err != nil || !ok {
		t.Fatalf("stored provider ok=%v err=%v", ok, err)
	}
	if strings.Contains(stored.APIKey, "sk-secret") {
		t.Fatalf("stored provider leaked raw api key: %#v", stored)
	}
	if !strings.HasPrefix(stored.APIKey, "sealed:") {
		t.Fatalf("stored provider api key is not sealed: %#v", stored)
	}

	deleteWorkspaceReq := providerTestRequest(http.MethodDelete, "/api/v1/providers/"+id, "member", nil)
	deleteWorkspaceReq.SetPathValue("id", id)
	deleteWorkspaceRec := httptest.NewRecorder()
	s.handleDeleteProvider(deleteWorkspaceRec, deleteWorkspaceReq)
	if deleteWorkspaceRec.Code != http.StatusForbidden {
		t.Fatalf("member delete workspace status=%d body=%s", deleteWorkspaceRec.Code, deleteWorkspaceRec.Body.String())
	}

	memberID, _ := memberCreated["id"].(string)
	deletePersonalReq := providerTestRequest(http.MethodDelete, "/api/v1/providers/"+memberID, "member", nil)
	deletePersonalReq.SetPathValue("id", memberID)
	deletePersonalRec := httptest.NewRecorder()
	s.handleDeleteProvider(deletePersonalRec, deletePersonalReq)
	if deletePersonalRec.Code != http.StatusOK {
		t.Fatalf("member delete personal status=%d body=%s", deletePersonalRec.Code, deletePersonalRec.Body.String())
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

func TestModelProviderListFiltersPersonalProviders(t *testing.T) {
	s, _ := newProviderHandlerTestServer(t)
	for _, req := range []struct {
		user string
		body providerBody
	}{
		{"owner", providerBody{Name: "Workspace", Type: "openai"}},
		{"member", providerBody{Name: "Member Personal", Type: "openai", OwnerType: ConnectionOwnerUser}},
		{"owner", providerBody{Name: "Owner Personal", Type: "anthropic", OwnerType: ConnectionOwnerUser}},
	} {
		rec := httptest.NewRecorder()
		s.handleAddProvider(rec, providerTestRequest(http.MethodPost, "/api/v1/providers", req.user, req.body))
		if rec.Code != http.StatusOK {
			t.Fatalf("create provider for %s status=%d body=%s", req.user, rec.Code, rec.Body.String())
		}
	}

	memberRec := httptest.NewRecorder()
	s.handleListProviders(memberRec, providerTestRequest(http.MethodGet, "/api/v1/providers", "member", nil))
	if memberRec.Code != http.StatusOK {
		t.Fatalf("member list status=%d body=%s", memberRec.Code, memberRec.Body.String())
	}
	body := memberRec.Body.String()
	if !strings.Contains(body, "Workspace") || !strings.Contains(body, "Member Personal") {
		t.Fatalf("member list missing visible providers: %s", body)
	}
	if strings.Contains(body, "Owner Personal") {
		t.Fatalf("member list included another user's provider: %s", body)
	}
}

func TestModelProviderAgentScopedListFiltersUsableProviders(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	insertProvider := func(p controldb.ModelProvider) {
		t.Helper()
		now := time.Now().UTC().Format(time.RFC3339)
		p.WorkspaceID = workspaceID
		p.Type = "openai"
		p.EnvJSON = "{}"
		p.CreatedAt = now
		p.UpdatedAt = now
		if err := s.controlDB.UpsertModelProvider(workspaceID, p); err != nil {
			t.Fatalf("upsert provider %s: %v", p.ID, err)
		}
	}
	insertProvider(controldb.ModelProvider{
		ID:        "prov-workspace",
		OwnerType: ConnectionOwnerWorkspace,
		OwnerID:   workspaceID,
		Name:      "Workspace Provider",
	})
	insertProvider(controldb.ModelProvider{
		ID:        "prov-owner",
		OwnerType: ConnectionOwnerUser,
		OwnerID:   "owner",
		Name:      "Owner Personal Provider",
	})
	insertProvider(controldb.ModelProvider{
		ID:        "prov-other",
		OwnerType: ConnectionOwnerUser,
		OwnerID:   "other",
		Name:      "Other Personal Provider",
	})

	adminGlobalRec := httptest.NewRecorder()
	s.handleListProviders(adminGlobalRec, providerTestRequest(http.MethodGet, "/api/v1/providers", "admin", nil))
	if adminGlobalRec.Code != http.StatusOK {
		t.Fatalf("admin global list status=%d body=%s", adminGlobalRec.Code, adminGlobalRec.Body.String())
	}
	adminGlobalBody := adminGlobalRec.Body.String()
	for _, want := range []string{"Workspace Provider", "Owner Personal Provider", "Other Personal Provider"} {
		if !strings.Contains(adminGlobalBody, want) {
			t.Fatalf("admin global list missing %q: %s", want, adminGlobalBody)
		}
	}

	adminAgentRec := httptest.NewRecorder()
	s.handleListProviders(adminAgentRec, providerTestRequest(http.MethodGet, "/api/v1/providers?project=tapnow&agent=pm", "admin", nil))
	if adminAgentRec.Code != http.StatusOK {
		t.Fatalf("admin agent-scoped list status=%d body=%s", adminAgentRec.Code, adminAgentRec.Body.String())
	}
	adminAgentBody := adminAgentRec.Body.String()
	if !strings.Contains(adminAgentBody, "Workspace Provider") {
		t.Fatalf("admin agent-scoped list missing workspace provider: %s", adminAgentBody)
	}
	for _, blocked := range []string{"Owner Personal Provider", "Other Personal Provider"} {
		if strings.Contains(adminAgentBody, blocked) {
			t.Fatalf("admin agent-scoped list included unusable personal provider %q: %s", blocked, adminAgentBody)
		}
	}

	ownerAgentRec := httptest.NewRecorder()
	s.handleListProviders(ownerAgentRec, providerTestRequest(http.MethodGet, "/api/v1/providers?project=tapnow&agent=pm", "owner", nil))
	if ownerAgentRec.Code != http.StatusOK {
		t.Fatalf("owner agent-scoped list status=%d body=%s", ownerAgentRec.Code, ownerAgentRec.Body.String())
	}
	ownerAgentBody := ownerAgentRec.Body.String()
	for _, want := range []string{"Workspace Provider", "Owner Personal Provider"} {
		if !strings.Contains(ownerAgentBody, want) {
			t.Fatalf("owner agent-scoped list missing %q: %s", want, ownerAgentBody)
		}
	}
	if strings.Contains(ownerAgentBody, "Other Personal Provider") {
		t.Fatalf("owner agent-scoped list included another user's provider: %s", ownerAgentBody)
	}
}

func TestModelProviderAgentScopedListRequiresAgentManagementAccess(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	if err := s.users.CreateUser("viewer", "pass123", RoleMember, "", "", "", "", ""); err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	if err := s.controlDB.UpsertWorkspaceMember(workspaceID, "viewer", WorkspaceRoleMember); err != nil {
		t.Fatalf("viewer workspace member: %v", err)
	}
	if err := s.users.UpdateUser("viewer", nil, nil, nil, nil, nil, nil, nil, []projectAccess{{Project: "tapnow", Role: ProjectRoleViewer}}, nil, nil); err != nil {
		t.Fatalf("grant viewer project role: %v", err)
	}

	rec := httptest.NewRecorder()
	s.handleListProviders(rec, providerTestRequest(http.MethodGet, "/api/v1/providers?project=tapnow&agent=pm", "viewer", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer agent-scoped list status=%d body=%s", rec.Code, rec.Body.String())
	}
}
