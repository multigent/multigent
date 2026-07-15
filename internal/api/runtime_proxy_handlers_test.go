package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	controldb "github.com/multigent/multigent/internal/db"
)

func TestRuntimeMCPProxyForwardsCustomMCPWithServerSideToken(t *testing.T) {
	users := newTestUserStore(t)
	s := &Server{controlDB: users.db, users: users}
	workspaceID := "ws-one"
	var upstreamAuth string
	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
			t.Fatalf("decode upstream body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]any{
				"tools": []any{},
				"auth":  upstreamAuth,
			},
		})
	}))
	defer upstream.Close()

	connection := controldb.Connection{
		ID:             "conn-mcp",
		WorkspaceID:    workspaceID,
		Provider:       "custom-mcp",
		ConnectionName: "tools",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       ConnectionAuthCustomCredential,
		Status:         "active",
		ProfileJSON:    "{}",
		CreatedBy:      "admin",
	}
	if err := users.db.UpsertWorkspace(controldb.Workspace{ID: workspaceID, Name: "One", Slug: "one"}); err != nil {
		t.Fatalf("workspace: %v", err)
	}
	if err := users.db.UpsertConnection(connection); err != nil {
		t.Fatalf("connection: %v", err)
	}
	secret, err := sealConnectionSecret(map[string]string{"serverUrl": upstream.URL, "token": "mcp-token"})
	if err != nil {
		t.Fatalf("seal secret: %v", err)
	}
	secret.ConnectionID = connection.ID
	if err := users.db.UpsertConnectionSecret(secret); err != nil {
		t.Fatalf("secret: %v", err)
	}
	if err := users.db.CreateConnectionGrant(controldb.ConnectionGrant{
		ID:           "grant-mcp",
		WorkspaceID:  workspaceID,
		ConnectionID: connection.ID,
		TargetType:   ConnectionTargetAgent,
		TargetID:     "tapnow/pm",
	}); err != nil {
		t.Fatalf("grant: %v", err)
	}

	principal := runtimeAgentPrincipal{
		WorkspaceID:  workspaceID,
		Project:      "tapnow",
		Agent:        "pm",
		RunID:        "run-one",
		Capabilities: []string{"connection.use"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/mcp", stringsReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	req.Header.Set(agentConnectionManifest().ConnectionIDHeader, connection.ID)
	req = req.WithContext(context.WithValue(req.Context(), ctxRuntimeAgentKey, principal))
	rec := httptest.NewRecorder()

	s.handleRuntimeMCPProxy(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if upstreamAuth != "Bearer mcp-token" {
		t.Fatalf("upstream auth=%q", upstreamAuth)
	}
	if upstreamBody["method"] != "tools/list" {
		t.Fatalf("upstream body=%#v", upstreamBody)
	}
	if body := rec.Body.String(); body == "" || containsAny(body, []string{"mcp-token", "serverUrl"}) {
		t.Fatalf("proxy response leaked sensitive data or is empty: %s", body)
	}
	if !strings.Contains(rec.Body.String(), "Bearer [redacted]") {
		t.Fatalf("redacted authorization marker missing: %s", rec.Body.String())
	}
}

func TestCustomMCPRuntimeConfigSupportsNoAuthProfileURL(t *testing.T) {
	users := newTestUserStore(t)
	s := &Server{controlDB: users.db, users: users}
	cfg, err := s.customMCPRuntimeConfig(controldb.Connection{
		ID:             "conn-no-auth",
		Provider:       "custom-mcp",
		ConnectionName: "default",
		AuthType:       ConnectionAuthNoAuth,
		ProfileJSON:    `{"serverUrl":"http://127.0.0.1:3000/mcp","token":"should-not-be-used"}`,
	})
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if cfg.ServerURL != "http://127.0.0.1:3000/mcp" {
		t.Fatalf("serverURL=%q", cfg.ServerURL)
	}
	if cfg.Token != "" {
		t.Fatalf("no_auth profile token should not be used: %q", cfg.Token)
	}
}

func TestRuntimeActionProxyForwardsCustomHTTPWithServerSideCredential(t *testing.T) {
	users := newTestUserStore(t)
	s := &Server{controlDB: users.db, users: users}
	workspaceID := "ws-one"
	var upstreamAuth string
	var upstreamUserAgent string
	var upstreamQuery string
	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamAuth = r.Header.Get("Authorization")
		upstreamUserAgent = r.Header.Get("User-Agent")
		upstreamQuery = r.URL.RawQuery
		if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
			t.Fatalf("decode upstream body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "session=secret")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":   true,
			"auth": upstreamAuth,
		})
	}))
	defer upstream.Close()

	connection := controldb.Connection{
		ID:             "conn-http",
		WorkspaceID:    workspaceID,
		Provider:       "custom-http",
		ConnectionName: "api",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       ConnectionAuthCustomCredential,
		Status:         "active",
		ProfileJSON:    "{}",
		CreatedBy:      "admin",
	}
	if err := users.db.UpsertWorkspace(controldb.Workspace{ID: workspaceID, Name: "One", Slug: "one"}); err != nil {
		t.Fatalf("workspace: %v", err)
	}
	if err := users.db.UpsertConnection(connection); err != nil {
		t.Fatalf("connection: %v", err)
	}
	secret, err := sealConnectionSecret(map[string]string{"baseUrl": upstream.URL, "apiKey": "http-token"})
	if err != nil {
		t.Fatalf("seal secret: %v", err)
	}
	secret.ConnectionID = connection.ID
	if err := users.db.UpsertConnectionSecret(secret); err != nil {
		t.Fatalf("secret: %v", err)
	}
	if err := users.db.CreateConnectionGrant(controldb.ConnectionGrant{
		ID:           "grant-http",
		WorkspaceID:  workspaceID,
		ConnectionID: connection.ID,
		TargetType:   ConnectionTargetAgent,
		TargetID:     "tapnow/pm",
	}); err != nil {
		t.Fatalf("grant: %v", err)
	}

	principal := runtimeAgentPrincipal{
		WorkspaceID:  workspaceID,
		Project:      "tapnow",
		Agent:        "pm",
		RunID:        "run-one",
		Capabilities: []string{"connection.use"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/actions", stringsReader(`{
		"method":"POST",
		"endpoint":"/v1/items",
		"query":{"page":"1"},
		"headers":{"Authorization":"Bearer attacker","User-Agent":"agent-test"},
		"body":{"name":"demo"}
	}`))
	req.Header.Set(agentConnectionManifest().ConnectionIDHeader, connection.ID)
	req = req.WithContext(context.WithValue(req.Context(), ctxRuntimeAgentKey, principal))
	rec := httptest.NewRecorder()

	s.handleRuntimeActionProxy(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if upstreamAuth != "Bearer http-token" {
		t.Fatalf("upstream auth=%q", upstreamAuth)
	}
	if upstreamUserAgent != "agent-test" {
		t.Fatalf("upstream user agent=%q", upstreamUserAgent)
	}
	if upstreamQuery != "page=1" {
		t.Fatalf("upstream query=%q", upstreamQuery)
	}
	if upstreamBody["name"] != "demo" {
		t.Fatalf("upstream body=%#v", upstreamBody)
	}
	body := rec.Body.String()
	if containsAny(body, []string{"http-token", "Set-Cookie", "session=secret"}) {
		t.Fatalf("action response leaked sensitive data: %s", body)
	}
	if !strings.Contains(body, "Bearer [redacted]") {
		t.Fatalf("redacted auth marker missing: %s", body)
	}
}

func TestRuntimeActionProxyForwardsFeishuWithTenantToken(t *testing.T) {
	users := newTestUserStore(t)
	s := &Server{controlDB: users.db, users: users}
	workspaceID := "ws-one"
	var tokenRequest map[string]string
	var upstreamAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			if err := json.NewDecoder(r.Body).Decode(&tokenRequest); err != nil {
				t.Fatalf("decode token request: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":                0,
				"msg":                 "ok",
				"tenant_access_token": "tenant-token",
			})
		case "/open-apis/wiki/v2/spaces":
			upstreamAuth = r.Header.Get("Authorization")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"items": []any{}},
				"auth": upstreamAuth,
			})
		default:
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	connection := controldb.Connection{
		ID:             "conn-feishu",
		WorkspaceID:    workspaceID,
		Provider:       "feishu",
		ConnectionName: "default",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       ConnectionAuthCustomCredential,
		Status:         "active",
		ProfileJSON:    "{}",
		CreatedBy:      "admin",
	}
	if err := users.db.UpsertWorkspace(controldb.Workspace{ID: workspaceID, Name: "One", Slug: "one"}); err != nil {
		t.Fatalf("workspace: %v", err)
	}
	if err := users.db.UpsertConnection(connection); err != nil {
		t.Fatalf("connection: %v", err)
	}
	secret, err := sealConnectionSecret(map[string]string{"baseUrl": upstream.URL, "appId": "cli_app", "appSecret": "app-secret"})
	if err != nil {
		t.Fatalf("seal secret: %v", err)
	}
	secret.ConnectionID = connection.ID
	if err := users.db.UpsertConnectionSecret(secret); err != nil {
		t.Fatalf("secret: %v", err)
	}
	if err := users.db.CreateConnectionGrant(controldb.ConnectionGrant{
		ID:           "grant-feishu",
		WorkspaceID:  workspaceID,
		ConnectionID: connection.ID,
		TargetType:   ConnectionTargetAgent,
		TargetID:     "tapnow/pm",
	}); err != nil {
		t.Fatalf("grant: %v", err)
	}

	principal := runtimeAgentPrincipal{
		WorkspaceID:  workspaceID,
		Project:      "tapnow",
		Agent:        "pm",
		RunID:        "run-one",
		Capabilities: []string{"connection.use"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/actions", stringsReader(`{
		"method":"GET",
		"endpoint":"/open-apis/wiki/v2/spaces",
		"headers":{"Authorization":"Bearer attacker"}
	}`))
	req.Header.Set(agentConnectionManifest().ConnectionIDHeader, connection.ID)
	req = req.WithContext(context.WithValue(req.Context(), ctxRuntimeAgentKey, principal))
	rec := httptest.NewRecorder()

	s.handleRuntimeActionProxy(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if tokenRequest["app_id"] != "cli_app" || tokenRequest["app_secret"] != "app-secret" {
		t.Fatalf("token request=%#v", tokenRequest)
	}
	if upstreamAuth != "Bearer tenant-token" {
		t.Fatalf("upstream auth=%q", upstreamAuth)
	}
	body := rec.Body.String()
	if containsAny(body, []string{"tenant-token", "app-secret"}) {
		t.Fatalf("feishu proxy response leaked sensitive value: %s", body)
	}
	if !strings.Contains(body, "Bearer [redacted]") {
		t.Fatalf("redacted auth marker missing: %s", body)
	}
}

func TestRuntimeActionProxyRejectsUnsafeEndpoint(t *testing.T) {
	users := newTestUserStore(t)
	s := &Server{controlDB: users.db, users: users}
	connection := controldb.Connection{
		ID:             "conn-http",
		WorkspaceID:    "ws-one",
		Provider:       "custom-http",
		ConnectionName: "api",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        "ws-one",
		AuthType:       ConnectionAuthCustomCredential,
		Status:         "active",
		ProfileJSON:    `{"baseUrl":"https://example.com"}`,
	}
	if err := users.db.UpsertWorkspace(controldb.Workspace{ID: "ws-one", Name: "One", Slug: "one"}); err != nil {
		t.Fatalf("workspace: %v", err)
	}
	if err := users.db.UpsertConnection(connection); err != nil {
		t.Fatalf("connection: %v", err)
	}
	if err := users.db.CreateConnectionGrant(controldb.ConnectionGrant{
		ID:           "grant-http",
		WorkspaceID:  "ws-one",
		ConnectionID: connection.ID,
		TargetType:   ConnectionTargetAgent,
		TargetID:     "tapnow/pm",
	}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	principal := runtimeAgentPrincipal{
		WorkspaceID:  "ws-one",
		Project:      "tapnow",
		Agent:        "pm",
		RunID:        "run-one",
		Capabilities: []string{"connection.use"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/actions", stringsReader(`{"method":"GET","endpoint":"https://evil.test/x"}`))
	req.Header.Set(agentConnectionManifest().ConnectionIDHeader, connection.ID)
	req = req.WithContext(context.WithValue(req.Context(), ctxRuntimeAgentKey, principal))
	rec := httptest.NewRecorder()

	s.handleRuntimeActionProxy(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func stringsReader(s string) *strings.Reader {
	return strings.NewReader(s)
}

func containsAny(s string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
