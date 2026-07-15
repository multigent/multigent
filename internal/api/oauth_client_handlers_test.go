package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/multigent/multigent/internal/connector"
	controldb "github.com/multigent/multigent/internal/db"
)

func TestOAuthClientConfigHandlersAreAdminScopedAndDoNotLeakSecret(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)

	memberReq := providerTestRequest(http.MethodGet, "/api/v1/oauth/client-configs", "owner", nil)
	memberRec := httptest.NewRecorder()
	s.handleListOAuthClientConfigs(memberRec, memberReq)
	if memberRec.Code != http.StatusForbidden {
		t.Fatalf("member list status=%d body=%s", memberRec.Code, memberRec.Body.String())
	}

	adminListReq := providerTestRequest(http.MethodGet, "/api/v1/oauth/client-configs", "admin", nil)
	adminListReq.Host = "multigent.example.test"
	adminListRec := httptest.NewRecorder()
	s.handleListOAuthClientConfigs(adminListRec, adminListReq)
	if adminListRec.Code != http.StatusOK {
		t.Fatalf("admin list status=%d body=%s", adminListRec.Code, adminListRec.Body.String())
	}
	if !strings.Contains(adminListRec.Body.String(), "github") || !strings.Contains(adminListRec.Body.String(), "/api/v1/oauth/callback") {
		t.Fatalf("list response missing oauth metadata: %s", adminListRec.Body.String())
	}

	upsertReq := providerTestRequest(http.MethodPut, "/api/v1/oauth/client-configs/github", "admin", oauthClientConfigRequest{
		ClientID:     "gh-client",
		ClientSecret: "gh-secret",
		Extra:        map[string]any{"enterprise": "acme"},
	})
	upsertReq.SetPathValue("provider", "github")
	upsertRec := httptest.NewRecorder()
	s.handleUpsertOAuthClientConfig(upsertRec, upsertReq)
	if upsertRec.Code != http.StatusOK {
		t.Fatalf("upsert status=%d body=%s", upsertRec.Code, upsertRec.Body.String())
	}
	if strings.Contains(upsertRec.Body.String(), "gh-secret") || strings.Contains(upsertRec.Body.String(), "secret_ciphertext") {
		t.Fatalf("upsert response leaked secret: %s", upsertRec.Body.String())
	}
	var resp oauthClientConfigResponse
	if err := json.Unmarshal(upsertRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode upsert: %v", err)
	}
	if !resp.Configured || resp.ClientID != "gh-client" || resp.Extra["enterprise"] != "acme" {
		t.Fatalf("unexpected response=%#v", resp)
	}

	config, ok, err := s.controlDB.OAuthClientConfigByProvider(workspaceID, "github")
	if err != nil || !ok {
		t.Fatalf("stored config ok=%v err=%v", ok, err)
	}
	if config.ClientID != "gh-client" || config.SecretCiphertext == "" || strings.Contains(config.SecretCiphertext, "gh-secret") {
		t.Fatalf("stored config not encrypted/sanitized: %#v", config)
	}
}

func TestOAuthClientConfigUpsertCanKeepExistingSecret(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	req := providerTestRequest(http.MethodPut, "/api/v1/oauth/client-configs/github", "admin", oauthClientConfigRequest{
		ClientID:     "gh-client",
		ClientSecret: "gh-secret",
	})
	req.SetPathValue("provider", "github")
	rec := httptest.NewRecorder()
	s.handleUpsertOAuthClientConfig(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("initial status=%d body=%s", rec.Code, rec.Body.String())
	}
	initial, ok, err := s.controlDB.OAuthClientConfigByProvider(workspaceID, "github")
	if err != nil || !ok {
		t.Fatalf("initial lookup ok=%v err=%v", ok, err)
	}

	updateReq := providerTestRequest(http.MethodPut, "/api/v1/oauth/client-configs/github", "admin", oauthClientConfigRequest{
		ClientID: "gh-client-v2",
	})
	updateReq.SetPathValue("provider", "github")
	updateRec := httptest.NewRecorder()
	s.handleUpsertOAuthClientConfig(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updateRec.Code, updateRec.Body.String())
	}
	updated, ok, err := s.controlDB.OAuthClientConfigByProvider(workspaceID, "github")
	if err != nil || !ok {
		t.Fatalf("updated lookup ok=%v err=%v", ok, err)
	}
	if updated.ClientID != "gh-client-v2" {
		t.Fatalf("clientID=%q", updated.ClientID)
	}
	if updated.SecretCiphertext == "" || updated.SecretCiphertext != initial.SecretCiphertext {
		t.Fatalf("secret was not preserved")
	}
}

func TestOAuthClientConfigRejectsUnsupportedProviderAndSecretLikeExtra(t *testing.T) {
	s, _ := newConnectionGrantPolicyServer(t)
	unsupportedReq := providerTestRequest(http.MethodPut, "/api/v1/oauth/client-configs/custom-http", "admin", oauthClientConfigRequest{
		ClientID:     "client",
		ClientSecret: "secret",
	})
	unsupportedReq.SetPathValue("provider", "custom-http")
	unsupportedRec := httptest.NewRecorder()
	s.handleUpsertOAuthClientConfig(unsupportedRec, unsupportedReq)
	if unsupportedRec.Code != http.StatusBadRequest {
		t.Fatalf("unsupported status=%d body=%s", unsupportedRec.Code, unsupportedRec.Body.String())
	}

	extraReq := providerTestRequest(http.MethodPut, "/api/v1/oauth/client-configs/github", "admin", oauthClientConfigRequest{
		ClientID:     "client",
		ClientSecret: "secret",
		Extra:        map[string]any{"refreshToken": "bad"},
	})
	extraReq.SetPathValue("provider", "github")
	extraRec := httptest.NewRecorder()
	s.handleUpsertOAuthClientConfig(extraRec, extraReq)
	if extraRec.Code != http.StatusBadRequest {
		t.Fatalf("secret-like extra status=%d body=%s", extraRec.Code, extraRec.Body.String())
	}
}

func TestOAuthAuthorizationStartAndCallbackCreateConnection(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	var tokenRequestBody string
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login/oauth/access_token" {
			t.Fatalf("unexpected token path: %s", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		tokenRequestBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "oauth-access",
			"token_type":    "bearer",
			"refresh_token": "oauth-refresh",
			"expires_in":    3600,
			"scope":         "repo read:user",
		})
	}))
	defer tokenServer.Close()
	upsertOAuthProviderForTest(t, s, tokenServer.URL+"/login/oauth/access_token")

	saveReq := providerTestRequest(http.MethodPut, "/api/v1/oauth/client-configs/github", "admin", oauthClientConfigRequest{
		ClientID:     "gh-client",
		ClientSecret: "gh-secret",
	})
	saveReq.SetPathValue("provider", "github")
	saveRec := httptest.NewRecorder()
	s.handleUpsertOAuthClientConfig(saveRec, saveReq)
	if saveRec.Code != http.StatusOK {
		t.Fatalf("save config status=%d body=%s", saveRec.Code, saveRec.Body.String())
	}

	startReq := providerTestRequest(http.MethodPost, "/api/v1/oauth/authorizations", "owner", oauthAuthorizationStartRequest{
		Provider:       "github",
		ConnectionName: "personal",
		OwnerType:      ConnectionOwnerUser,
		Profile:        map[string]any{"accountName": "octo"},
	})
	startReq.Host = "multigent.example.test"
	startRec := httptest.NewRecorder()
	s.handleStartOAuthAuthorization(startRec, startReq)
	if startRec.Code != http.StatusOK {
		t.Fatalf("start status=%d body=%s", startRec.Code, startRec.Body.String())
	}
	var started oauthAuthorizationStartResponse
	if err := json.Unmarshal(startRec.Body.Bytes(), &started); err != nil {
		t.Fatalf("decode start: %v", err)
	}
	if started.State == "" || !strings.Contains(started.AuthorizationURL, "client_id=gh-client") || !strings.Contains(started.AuthorizationURL, "state="+started.State) {
		t.Fatalf("authorization response=%#v", started)
	}

	callbackReq := httptest.NewRequest(http.MethodGet, "/api/v1/oauth/callback?state="+started.State+"&code=code-one", nil)
	callbackReq.Host = "multigent.example.test"
	callbackRec := httptest.NewRecorder()
	s.handleCompleteOAuthAuthorization(callbackRec, callbackReq)
	if callbackRec.Code != http.StatusOK {
		t.Fatalf("callback status=%d body=%s", callbackRec.Code, callbackRec.Body.String())
	}
	if !strings.Contains(tokenRequestBody, "client_id=gh-client") || !strings.Contains(tokenRequestBody, "client_secret=gh-secret") || !strings.Contains(tokenRequestBody, "code=code-one") {
		t.Fatalf("token request body=%s", tokenRequestBody)
	}
	if strings.Contains(callbackRec.Body.String(), "oauth-access") || strings.Contains(callbackRec.Body.String(), "oauth-refresh") {
		t.Fatalf("callback leaked token: %s", callbackRec.Body.String())
	}
	connections, err := s.controlDB.ListConnections(controldb.ConnectionFilter{WorkspaceID: workspaceID, Provider: "github", OwnerType: ConnectionOwnerUser, OwnerID: "owner"})
	if err != nil {
		t.Fatalf("list connections: %v", err)
	}
	if len(connections) != 1 {
		t.Fatalf("connections=%#v", connections)
	}
	if connections[0].AuthType != ConnectionAuthOAuth2 || connections[0].ConnectionName != "personal" {
		t.Fatalf("connection=%#v", connections[0])
	}
	secret, ok, err := s.controlDB.ConnectionSecret(connections[0].ID)
	if err != nil || !ok {
		t.Fatalf("secret ok=%v err=%v", ok, err)
	}
	values, err := openConnectionSecret(secret)
	if err != nil {
		t.Fatalf("open connection secret: %v", err)
	}
	if values["accessToken"] != "oauth-access" || values["refreshToken"] != "oauth-refresh" {
		t.Fatalf("secret values=%#v", values)
	}
}

func TestOAuthAuthorizationWorkspaceOwnerRequiresAdmin(t *testing.T) {
	s, _ := newConnectionGrantPolicyServer(t)
	startReq := providerTestRequest(http.MethodPost, "/api/v1/oauth/authorizations", "owner", oauthAuthorizationStartRequest{
		Provider:  "github",
		OwnerType: ConnectionOwnerWorkspace,
	})
	startRec := httptest.NewRecorder()
	s.handleStartOAuthAuthorization(startRec, startReq)
	if startRec.Code != http.StatusBadRequest && startRec.Code != http.StatusForbidden {
		t.Fatalf("workspace owner start status=%d body=%s", startRec.Code, startRec.Body.String())
	}
}

func upsertOAuthProviderForTest(t *testing.T, s *Server, tokenURL string) {
	t.Helper()
	provider := connector.Defaults()[0]
	if provider.Provider != "github" {
		t.Fatalf("expected github default first")
	}
	provider.OAuth.TokenURL = tokenURL
	authTypes, _ := json.Marshal(provider.AuthTypes)
	catalog, _ := json.Marshal(provider)
	if err := s.controlDB.UpsertConnectorProvider(controldb.ConnectorProvider{
		Provider:      provider.Provider,
		DisplayName:   provider.DisplayName,
		AuthTypesJSON: string(authTypes),
		CatalogJSON:   string(catalog),
		Enabled:       true,
	}); err != nil {
		t.Fatalf("upsert provider: %v", err)
	}
}
