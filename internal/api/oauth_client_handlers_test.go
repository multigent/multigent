package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
