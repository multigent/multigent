package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
		TargetID:     "sample/pm",
	}); err != nil {
		t.Fatalf("grant: %v", err)
	}

	principal := runtimeAgentPrincipal{
		WorkspaceID:  workspaceID,
		Project:      "sample",
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

func TestRuntimeActionConfigRefreshesExpiredOAuthToken(t *testing.T) {
	users := newTestUserStore(t)
	s := &Server{controlDB: users.db, users: users}
	workspaceID := "ws-one"
	var tokenGrantType string
	var tokenRefreshToken string
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse token request: %v", err)
		}
		tokenGrantType = r.Form.Get("grant_type")
		tokenRefreshToken = r.Form.Get("refresh_token")
		if r.Form.Get("client_id") != "client-id" || r.Form.Get("client_secret") != "client-secret" {
			t.Fatalf("unexpected client credentials: %#v", r.Form)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access",
			"refresh_token": "new-refresh",
			"token_type":    "Bearer",
			"expires_in":    3600,
			"scope":         "repo read:user",
		})
	}))
	defer tokenServer.Close()
	upsertOAuthProviderForTest(t, s, tokenServer.URL+"/token")
	if err := users.db.UpsertWorkspace(controldb.Workspace{ID: workspaceID, Name: "One", Slug: "one"}); err != nil {
		t.Fatalf("workspace: %v", err)
	}
	clientSecret, err := sealOAuthClientSecret("client-secret")
	if err != nil {
		t.Fatalf("seal client secret: %v", err)
	}
	if err := users.db.UpsertOAuthClientConfig(controldb.OAuthClientConfig{
		WorkspaceID:      workspaceID,
		Provider:         "github",
		ClientID:         "client-id",
		SecretCiphertext: clientSecret.Ciphertext,
		Nonce:            clientSecret.Nonce,
		KeyVersion:       clientSecret.KeyVersion,
		ExtraJSON:        "{}",
		CreatedBy:        "admin",
	}); err != nil {
		t.Fatalf("oauth client config: %v", err)
	}
	expiresAt := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	connection := controldb.Connection{
		ID:             "conn-oauth",
		WorkspaceID:    workspaceID,
		Provider:       "github",
		ConnectionName: "personal",
		OwnerType:      ConnectionOwnerUser,
		OwnerID:        "user-1",
		AuthType:       ConnectionAuthOAuth2,
		Status:         "active",
		ProfileJSON:    `{"displayName":"GitHub OAuth","expiresAt":"` + expiresAt + `"}`,
		CreatedBy:      "user-1",
	}
	if err := users.db.UpsertConnection(connection); err != nil {
		t.Fatalf("connection: %v", err)
	}
	secret, err := sealConnectionSecret(map[string]string{
		"accessToken":  "old-access",
		"refreshToken": "old-refresh",
		"tokenType":    "Bearer",
		"expiresAt":    expiresAt,
	})
	if err != nil {
		t.Fatalf("seal connection secret: %v", err)
	}
	secret.ConnectionID = connection.ID
	if err := users.db.UpsertConnectionSecret(secret); err != nil {
		t.Fatalf("connection secret: %v", err)
	}

	cfg, err := s.runtimeHTTPActionConfig(connection)
	if err != nil {
		t.Fatalf("runtime action config: %v", err)
	}
	if tokenGrantType != "refresh_token" || tokenRefreshToken != "old-refresh" {
		t.Fatalf("unexpected refresh request grant=%q refresh=%q", tokenGrantType, tokenRefreshToken)
	}
	if cfg.AuthValue != "Bearer new-access" {
		t.Fatalf("auth value=%q", cfg.AuthValue)
	}
	refreshedSecret, ok, err := users.db.ConnectionSecret(connection.ID)
	if err != nil || !ok {
		t.Fatalf("refreshed secret ok=%v err=%v", ok, err)
	}
	values, err := openConnectionSecret(refreshedSecret)
	if err != nil {
		t.Fatalf("open refreshed secret: %v", err)
	}
	if values["accessToken"] != "new-access" || values["refreshToken"] != "new-refresh" || values["expiresAt"] == "" {
		t.Fatalf("unexpected refreshed values: %#v", values)
	}
	updated, ok, err := users.db.ConnectionByID(connection.ID)
	if err != nil || !ok {
		t.Fatalf("updated connection ok=%v err=%v", ok, err)
	}
	if !strings.Contains(updated.ProfileJSON, "repo") || !strings.Contains(updated.ProfileJSON, "expiresAt") {
		t.Fatalf("profile not updated: %s", updated.ProfileJSON)
	}
	events, err := users.db.ListAuditEvents(controldb.AuditEventFilter{
		WorkspaceID:  workspaceID,
		Action:       "connection.oauth.refresh",
		ResourceType: "connection",
		ResourceID:   connection.ID,
		Limit:        1,
	})
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected refresh audit event, got %d", len(events))
	}
}

func TestRuntimeActionConfigExpiredOAuthTokenRequiresRefreshToken(t *testing.T) {
	users := newTestUserStore(t)
	s := &Server{controlDB: users.db, users: users}
	workspaceID := "ws-one"
	if err := users.db.UpsertWorkspace(controldb.Workspace{ID: workspaceID, Name: "One", Slug: "one"}); err != nil {
		t.Fatalf("workspace: %v", err)
	}
	expiresAt := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	connection := controldb.Connection{
		ID:             "conn-oauth-no-refresh",
		WorkspaceID:    workspaceID,
		Provider:       "github",
		ConnectionName: "personal",
		OwnerType:      ConnectionOwnerUser,
		OwnerID:        "user-1",
		AuthType:       ConnectionAuthOAuth2,
		Status:         "active",
		ProfileJSON:    "{}",
		CreatedBy:      "user-1",
	}
	if err := users.db.UpsertConnection(connection); err != nil {
		t.Fatalf("connection: %v", err)
	}
	secret, err := sealConnectionSecret(map[string]string{
		"accessToken": "old-access",
		"expiresAt":   expiresAt,
	})
	if err != nil {
		t.Fatalf("seal connection secret: %v", err)
	}
	secret.ConnectionID = connection.ID
	if err := users.db.UpsertConnectionSecret(secret); err != nil {
		t.Fatalf("connection secret: %v", err)
	}

	_, err = s.runtimeHTTPActionConfig(connection)
	if err == nil || !strings.Contains(err.Error(), "refresh token is missing") {
		t.Fatalf("expected missing refresh token error, got %v", err)
	}
}

func TestRuntimeActionConfigForTokenFirstExternalTools(t *testing.T) {
	users := newTestUserStore(t)
	s := &Server{controlDB: users.db, users: users}
	workspaceID := "ws-one"
	if err := users.db.UpsertWorkspace(controldb.Workspace{ID: workspaceID, Name: "One", Slug: "one"}); err != nil {
		t.Fatalf("workspace: %v", err)
	}
	cases := []struct {
		provider        string
		baseURL         string
		authHeader      string
		authValue       string
		defaultHeader   string
		defaultValue    string
		rewrittenQuery  string
		redactedSnippet string
	}{
		{provider: "github", baseURL: "https://api.github.com", authHeader: "Authorization", authValue: "Bearer token-github", redactedSnippet: "Bearer token-github"},
		{provider: "gitlab", baseURL: "https://gitlab.com/api/v4", authHeader: "PRIVATE-TOKEN", authValue: "token-gitlab", redactedSnippet: "token-gitlab"},
		{provider: "gitee", baseURL: "https://gitee.com/api/v5", rewrittenQuery: "token-gitee", redactedSnippet: "token-gitee"},
		{provider: "linear", baseURL: "https://api.linear.app", authHeader: "Authorization", authValue: "token-linear", redactedSnippet: "token-linear"},
		{provider: "notion", baseURL: "https://api.notion.com/v1", authHeader: "Authorization", authValue: "Bearer token-notion", defaultHeader: "Notion-Version", defaultValue: "2022-06-28", redactedSnippet: "Bearer token-notion"},
		{provider: "figma", baseURL: "https://api.figma.com/v1", authHeader: "X-Figma-Token", authValue: "token-figma", redactedSnippet: "token-figma"},
		{provider: "airtable", baseURL: "https://api.airtable.com/v0", authHeader: "Authorization", authValue: "Bearer token-airtable", redactedSnippet: "Bearer token-airtable"},
		{provider: "asana", baseURL: "https://app.asana.com/api/1.0", authHeader: "Authorization", authValue: "Bearer token-asana", redactedSnippet: "Bearer token-asana"},
		{provider: "clickup", baseURL: "https://api.clickup.com/api/v2", authHeader: "Authorization", authValue: "token-clickup", redactedSnippet: "token-clickup"},
		{provider: "sentry", baseURL: "https://sentry.io/api/0", authHeader: "Authorization", authValue: "Bearer token-sentry", redactedSnippet: "Bearer token-sentry"},
		{provider: "vercel", baseURL: "https://api.vercel.com", authHeader: "Authorization", authValue: "Bearer token-vercel", redactedSnippet: "Bearer token-vercel"},
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			connection := controldb.Connection{
				ID:             "conn-" + tc.provider,
				WorkspaceID:    workspaceID,
				Provider:       tc.provider,
				ConnectionName: "default",
				OwnerType:      ConnectionOwnerWorkspace,
				OwnerID:        workspaceID,
				AuthType:       ConnectionAuthAPIKey,
				Status:         "active",
				ProfileJSON:    "{}",
				CreatedBy:      "admin",
			}
			if err := users.db.UpsertConnection(connection); err != nil {
				t.Fatalf("connection: %v", err)
			}
			token := "token-" + tc.provider
			secret, err := sealConnectionSecret(map[string]string{"apiKey": token})
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
			if cfg.BaseURL != tc.baseURL {
				t.Fatalf("baseURL=%q want %q", cfg.BaseURL, tc.baseURL)
			}
			if cfg.AuthHeader != tc.authHeader {
				t.Fatalf("authHeader=%q want %q", cfg.AuthHeader, tc.authHeader)
			}
			if cfg.AuthValue != tc.authValue {
				t.Fatalf("authValue=%q want %q", cfg.AuthValue, tc.authValue)
			}
			if tc.defaultHeader != "" && cfg.DefaultHeaders[tc.defaultHeader] != tc.defaultValue {
				t.Fatalf("default header %q=%q want %q", tc.defaultHeader, cfg.DefaultHeaders[tc.defaultHeader], tc.defaultValue)
			}
			if tc.rewrittenQuery != "" {
				_, query, err := cfg.EndpointRewrite("/user", nil)
				if err != nil {
					t.Fatalf("rewrite: %v", err)
				}
				if query["access_token"] != tc.rewrittenQuery {
					t.Fatalf("rewritten access_token=%q want %q", query["access_token"], tc.rewrittenQuery)
				}
			}
			if !strings.Contains(strings.Join(cfg.RedactValues, "\n"), tc.redactedSnippet) {
				t.Fatalf("redact values %#v missing %q", cfg.RedactValues, tc.redactedSnippet)
			}
		})
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
		TargetID:     "sample/pm",
	}); err != nil {
		t.Fatalf("grant: %v", err)
	}

	principal := runtimeAgentPrincipal{
		WorkspaceID:  workspaceID,
		Project:      "sample",
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
		TargetID:     "sample/pm",
	}); err != nil {
		t.Fatalf("grant: %v", err)
	}

	principal := runtimeAgentPrincipal{
		WorkspaceID:  workspaceID,
		Project:      "sample",
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
		TargetID:     "sample/pm",
	}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	principal := runtimeAgentPrincipal{
		WorkspaceID:  "ws-one",
		Project:      "sample",
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
		TargetID:     "sample/pm",
	}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	principal := runtimeAgentPrincipal{
		WorkspaceID:  workspaceID,
		Project:      "sample",
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
