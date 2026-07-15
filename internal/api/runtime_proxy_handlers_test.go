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

func TestDingTalkBotRuntimeActionConfigAddsWebhookAuth(t *testing.T) {
	users := newTestUserStore(t)
	s := &Server{controlDB: users.db, users: users}
	if err := users.db.UpsertWorkspace(controldb.Workspace{ID: "ws-one", Name: "One", Slug: "one"}); err != nil {
		t.Fatalf("workspace: %v", err)
	}
	connection := controldb.Connection{
		ID:             "conn-dingtalk",
		WorkspaceID:    "ws-one",
		Provider:       "dingtalk_bot",
		ConnectionName: "alerts",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        "ws-one",
		AuthType:       ConnectionAuthAPIKey,
		Status:         "active",
		ProfileJSON:    "{}",
		CreatedBy:      "admin",
	}
	if err := users.db.UpsertConnection(connection); err != nil {
		t.Fatalf("connection: %v", err)
	}
	secret, err := sealConnectionSecret(map[string]string{
		"apiKey":        "https://oapi.dingtalk.com/robot/send?access_token=ding-token",
		"signingSecret": "SEC-secret",
	})
	if err != nil {
		t.Fatalf("seal secret: %v", err)
	}
	secret.ConnectionID = connection.ID
	if err := users.db.UpsertConnectionSecret(secret); err != nil {
		t.Fatalf("secret: %v", err)
	}

	cfg, err := s.runtimeHTTPActionConfig(connection)
	if err != nil {
		t.Fatalf("runtime action config: %v", err)
	}
	if cfg.BaseURL != "https://oapi.dingtalk.com" {
		t.Fatalf("baseURL=%q", cfg.BaseURL)
	}
	endpoint, query, err := cfg.EndpointRewrite("/robot/send", map[string]string{"ignored": "kept"})
	if err != nil {
		t.Fatalf("rewrite endpoint: %v", err)
	}
	if endpoint != "/robot/send" || query["access_token"] != "ding-token" || query["timestamp"] == "" || query["sign"] == "" || query["ignored"] != "kept" {
		t.Fatalf("rewrite endpoint=%q query=%#v", endpoint, query)
	}
	redactJoined := strings.Join(cfg.RedactValues, "\n")
	if !strings.Contains(redactJoined, "ding-token") || !strings.Contains(redactJoined, "SEC-secret") {
		t.Fatalf("redact values missing secrets: %#v", cfg.RedactValues)
	}
	if _, _, err := cfg.EndpointRewrite("/v1/users", nil); err == nil {
		t.Fatalf("non-DingTalk endpoint should fail")
	}
}

func TestNormalizeDingTalkBotAccessToken(t *testing.T) {
	token, err := normalizeDingTalkBotAccessToken("https://oapi.dingtalk.com/robot/send?access_token=abc")
	if err != nil {
		t.Fatalf("normalize webhook: %v", err)
	}
	if token != "abc" {
		t.Fatalf("token=%q", token)
	}
	if token, err := normalizeDingTalkBotAccessToken("plain-token"); err != nil || token != "plain-token" {
		t.Fatalf("normalize plain token=%q err=%v", token, err)
	}
	for _, invalid := range []string{
		"http://oapi.dingtalk.com/robot/send?access_token=abc",
		"https://example.com/robot/send?access_token=abc",
		"https://oapi.dingtalk.com/robot/send",
	} {
		if _, err := normalizeDingTalkBotAccessToken(invalid); err == nil {
			t.Fatalf("expected invalid webhook to fail: %s", invalid)
		}
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

func TestRuntimeActionProxyEnforcesConnectionActionPolicy(t *testing.T) {
	users := newTestUserStore(t)
	s := &Server{controlDB: users.db, users: users}
	workspaceID := "ws-one"
	var upstreamHits int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHits++
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}))
	defer upstream.Close()
	connection := controldb.Connection{
		ID:             "conn-policy",
		WorkspaceID:    workspaceID,
		Provider:       "custom-http",
		ConnectionName: "api",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       ConnectionAuthCustomCredential,
		Status:         "active",
		ProfileJSON:    `{"allowedActionMethods":["GET"],"allowedActionEndpoints":["/v1/read/*"],"blockedActionEndpoints":["/v1/read/private"]}`,
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
		ID:           "grant-policy",
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
	for _, tc := range []struct {
		name     string
		body     string
		wantCode int
	}{
		{name: "allowed", body: `{"method":"GET","endpoint":"/v1/read/items"}`, wantCode: http.StatusOK},
		{name: "method blocked by allowlist", body: `{"method":"POST","endpoint":"/v1/read/items","body":{"ok":true}}`, wantCode: http.StatusBadRequest},
		{name: "endpoint not allowed", body: `{"method":"GET","endpoint":"/v1/write/items"}`, wantCode: http.StatusBadRequest},
		{name: "endpoint blocked", body: `{"method":"GET","endpoint":"/v1/read/private"}`, wantCode: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/actions", stringsReader(tc.body))
			req.Header.Set(agentConnectionManifest().ConnectionIDHeader, connection.ID)
			req = req.WithContext(context.WithValue(req.Context(), ctxRuntimeAgentKey, principal))
			rec := httptest.NewRecorder()
			s.handleRuntimeActionProxy(rec, req)
			if rec.Code != tc.wantCode {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
	if upstreamHits != 1 {
		t.Fatalf("upstream hits=%d", upstreamHits)
	}
}

func TestRuntimeActionPolicyParsesStringLists(t *testing.T) {
	connection := controldb.Connection{
		ProfileJSON: `{"allowedActionMethods":"GET, POST","blockedActionEndpoints":"/admin/*\n/private"}`,
	}
	policy := runtimeActionPolicyFromConnection(connection)
	if !matchesRuntimeActionPolicy("GET", policy.AllowedMethods, true) || !matchesRuntimeActionPolicy("POST", policy.AllowedMethods, true) {
		t.Fatalf("methods=%#v", policy.AllowedMethods)
	}
	if !matchesRuntimeActionPolicy("/admin/users", policy.BlockedEndpoints, false) || !matchesRuntimeActionPolicy("/private", policy.BlockedEndpoints, false) {
		t.Fatalf("endpoints=%#v", policy.BlockedEndpoints)
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
