package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	controldb "github.com/multigent/multigent/internal/db"
)

func newConnectionTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	users := newTestUserStore(t)
	root := filepath.Join(t.TempDir(), "workspace")
	workspaceID := "ws-one"
	if err := users.db.UpsertWorkspace(controldb.Workspace{
		ID:        workspaceID,
		Name:      "One",
		Slug:      "one",
		Root:      root,
		CreatedAt: "2026-07-15T00:00:00Z",
	}); err != nil {
		t.Fatalf("workspace: %v", err)
	}
	for _, username := range []string{"owner", "other"} {
		if err := users.CreateUser(username, "pass123", RoleMember, "", "", "", "", ""); err != nil {
			t.Fatalf("create user %s: %v", username, err)
		}
		if err := users.db.UpsertWorkspaceMember(workspaceID, username, WorkspaceRoleMember); err != nil {
			t.Fatalf("workspace member %s: %v", username, err)
		}
	}
	return &Server{root: root, controlDB: users.db, users: users}, workspaceID
}

func TestConnectionTestCustomHTTPUsesServerSideCredential(t *testing.T) {
	s, workspaceID := newConnectionTestServer(t)
	var upstreamAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "session=secret")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"auth": upstreamAuth,
			"ok":   true,
		})
	}))
	defer upstream.Close()

	connection := controldb.Connection{
		ID:             "conn-http",
		WorkspaceID:    workspaceID,
		Provider:       "custom-http",
		ConnectionName: "api",
		OwnerType:      ConnectionOwnerUser,
		OwnerID:        "owner",
		AuthType:       ConnectionAuthCustomCredential,
		Status:         "active",
		ProfileJSON:    `{}`,
		CreatedBy:      "owner",
		CreatedAt:      "2026-07-15T00:00:00Z",
		UpdatedAt:      "2026-07-15T00:00:00Z",
	}
	if err := s.controlDB.UpsertConnection(connection); err != nil {
		t.Fatalf("connection: %v", err)
	}
	secret, err := sealConnectionSecret(map[string]string{"baseUrl": upstream.URL, "apiKey": "test-token"})
	if err != nil {
		t.Fatalf("seal secret: %v", err)
	}
	secret.ConnectionID = connection.ID
	if err := s.controlDB.UpsertConnectionSecret(secret); err != nil {
		t.Fatalf("secret: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/connections/conn-http/test", strings.NewReader(`{"headers":{"Authorization":"Bearer attacker"}}`))
	req.SetPathValue("id", connection.ID)
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), ctxUserKey, "owner"))
	rec := httptest.NewRecorder()

	s.handleTestConnection(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if upstreamAuth != "Bearer test-token" {
		t.Fatalf("upstream auth=%q", upstreamAuth)
	}
	body := rec.Body.String()
	if strings.Contains(body, "test-token") || strings.Contains(body, "Set-Cookie") || strings.Contains(body, "session=secret") {
		t.Fatalf("test response leaked sensitive data: %s", body)
	}
	if !strings.Contains(body, "Bearer [redacted]") {
		t.Fatalf("redacted marker missing: %s", body)
	}
}

func TestConnectionTestRequiresManagementAccess(t *testing.T) {
	s, workspaceID := newConnectionTestServer(t)
	if err := s.controlDB.UpsertConnection(controldb.Connection{
		ID:             "conn-owner",
		WorkspaceID:    workspaceID,
		Provider:       "custom-http",
		ConnectionName: "api",
		OwnerType:      ConnectionOwnerUser,
		OwnerID:        "owner",
		AuthType:       ConnectionAuthCustomCredential,
		Status:         "active",
		ProfileJSON:    `{}`,
		CreatedBy:      "owner",
		CreatedAt:      "2026-07-15T00:00:00Z",
		UpdatedAt:      "2026-07-15T00:00:00Z",
	}); err != nil {
		t.Fatalf("connection: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/connections/conn-owner/test", nil)
	req.SetPathValue("id", "conn-owner")
	req = req.WithContext(context.WithValue(req.Context(), ctxUserKey, "other"))
	rec := httptest.NewRecorder()

	s.handleTestConnection(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
