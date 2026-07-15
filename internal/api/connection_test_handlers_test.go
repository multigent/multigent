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

	updated, found, err := s.controlDB.ConnectionByID(connection.ID)
	if err != nil || !found {
		t.Fatalf("get updated connection: found=%v err=%v", found, err)
	}
	var profile map[string]any
	if err := json.Unmarshal([]byte(updated.ProfileJSON), &profile); err != nil {
		t.Fatalf("profile json: %v", err)
	}
	if profile["lastValidatedAt"] == "" {
		t.Fatalf("lastValidatedAt missing in profile: %#v", profile)
	}
	if profile["lastValidationOK"] != true {
		t.Fatalf("lastValidationOK=%#v profile=%#v", profile["lastValidationOK"], profile)
	}
	if profile["lastValidationStatus"] != float64(http.StatusOK) {
		t.Fatalf("lastValidationStatus=%#v profile=%#v", profile["lastValidationStatus"], profile)
	}
	if profile["lastValidationMessage"] != "Connection test succeeded" {
		t.Fatalf("lastValidationMessage=%#v profile=%#v", profile["lastValidationMessage"], profile)
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

func TestConnectionTestPersistsFailedHTTPValidation(t *testing.T) {
	s, workspaceID := newConnectionTestServer(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "temporary unavailable", http.StatusServiceUnavailable)
	}))
	defer upstream.Close()

	connection := controldb.Connection{
		ID:             "conn-http-fail",
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

	req := httptest.NewRequest(http.MethodPost, "/api/v1/connections/conn-http-fail/test", nil)
	req.SetPathValue("id", connection.ID)
	req = req.WithContext(context.WithValue(req.Context(), ctxUserKey, "owner"))
	rec := httptest.NewRecorder()

	s.handleTestConnection(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var result testConnectionResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("result json: %v", err)
	}
	if result.OK || result.Status != http.StatusServiceUnavailable {
		t.Fatalf("unexpected result: %#v", result)
	}

	updated, found, err := s.controlDB.ConnectionByID(connection.ID)
	if err != nil || !found {
		t.Fatalf("get updated connection: found=%v err=%v", found, err)
	}
	var profile map[string]any
	if err := json.Unmarshal([]byte(updated.ProfileJSON), &profile); err != nil {
		t.Fatalf("profile json: %v", err)
	}
	if profile["lastValidatedAt"] == "" {
		t.Fatalf("lastValidatedAt missing in profile: %#v", profile)
	}
	if profile["lastValidationOK"] != false {
		t.Fatalf("lastValidationOK=%#v profile=%#v", profile["lastValidationOK"], profile)
	}
	if profile["lastValidationStatus"] != float64(http.StatusServiceUnavailable) {
		t.Fatalf("lastValidationStatus=%#v profile=%#v", profile["lastValidationStatus"], profile)
	}
	if !strings.Contains(profile["lastValidationMessage"].(string), "HTTP 503") {
		t.Fatalf("lastValidationMessage=%#v profile=%#v", profile["lastValidationMessage"], profile)
	}
}

func TestDingTalkBotConnectionTestUsesScopedWebhookEndpoint(t *testing.T) {
	s, workspaceID := newConnectionTestServer(t)
	connection := controldb.Connection{
		ID:             "conn-dingtalk",
		WorkspaceID:    workspaceID,
		Provider:       "dingtalk_bot",
		ConnectionName: "alerts",
		OwnerType:      ConnectionOwnerUser,
		OwnerID:        "owner",
		AuthType:       ConnectionAuthAPIKey,
		Status:         "active",
		ProfileJSON:    `{}`,
		CreatedBy:      "owner",
		CreatedAt:      "2026-07-15T00:00:00Z",
		UpdatedAt:      "2026-07-15T00:00:00Z",
	}
	if err := s.controlDB.UpsertConnection(connection); err != nil {
		t.Fatalf("connection: %v", err)
	}
	secret, err := sealConnectionSecret(map[string]string{"apiKey": "ding-token"})
	if err != nil {
		t.Fatalf("seal secret: %v", err)
	}
	secret.ConnectionID = connection.ID
	if err := s.controlDB.UpsertConnectionSecret(secret); err != nil {
		t.Fatalf("secret: %v", err)
	}

	actionReq := runtimeActionProxyRequest{}
	applyDefaultConnectionTestRequest(connection.Provider, &actionReq)
	if actionReq.Endpoint != "/robot/send" || actionReq.Method != http.MethodPost || !strings.Contains(string(actionReq.Body), "Multigent connection test") {
		t.Fatalf("default DingTalk test request=%#v body=%s", actionReq, string(actionReq.Body))
	}

	_, err = s.testHTTPConnection(httptest.NewRequest(http.MethodPost, "/", nil), connection, testConnectionRequest{
		Endpoint: "/not-supported",
		Method:   http.MethodPost,
		Body:     json.RawMessage(`{"msgtype":"text","text":{"content":"test"}}`),
	})
	if err == nil || !strings.Contains(err.Error(), "only supports /robot/send") {
		t.Fatalf("expected unsupported endpoint error, got %v", err)
	}
}
