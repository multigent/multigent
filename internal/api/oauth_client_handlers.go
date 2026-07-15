package api

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/multigent/multigent/internal/connector"
	controldb "github.com/multigent/multigent/internal/db"
)

const oauthCallbackPath = "/api/v1/oauth/callback"

type oauthClientConfigRequest struct {
	ClientID     string         `json:"clientId"`
	ClientSecret string         `json:"clientSecret"`
	Extra        map[string]any `json:"extra"`
}

type oauthClientConfigResponse struct {
	Provider            string                  `json:"provider"`
	DisplayName         string                  `json:"displayName"`
	Configured          bool                    `json:"configured"`
	ClientID            string                  `json:"clientId,omitempty"`
	ExpectedRedirectURI string                  `json:"expectedRedirectUri"`
	OAuth               *connector.OAuth2Config `json:"oauth,omitempty"`
	Extra               map[string]any          `json:"extra,omitempty"`
	CreatedBy           string                  `json:"createdBy,omitempty"`
	CreatedAt           string                  `json:"createdAt,omitempty"`
	UpdatedAt           string                  `json:"updatedAt,omitempty"`
}

func (s *Server) handleListOAuthClientConfigs(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	providers, err := s.oauthProviders()
	if err != nil {
		s.serverError(w, err)
		return
	}
	configs, err := s.controlDB.ListOAuthClientConfigs(workspaceID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	byProvider := map[string]controldb.OAuthClientConfig{}
	for _, config := range configs {
		byProvider[config.Provider] = config
	}
	out := make([]oauthClientConfigResponse, 0, len(providers))
	for _, provider := range providers {
		config, configured := byProvider[provider.Provider]
		out = append(out, oauthClientConfigToResponse(r, provider, config, configured))
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"configs": out})
}

func (s *Server) handleUpsertOAuthClientConfig(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	providerID := strings.TrimSpace(r.PathValue("provider"))
	provider, ok, err := s.findOAuthProvider(providerID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !ok {
		s.jsonError(w, http.StatusBadRequest, "provider does not support oauth2")
		return
	}
	var body oauthClientConfigRequest
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	clientID := strings.TrimSpace(body.ClientID)
	if clientID == "" {
		s.jsonError(w, http.StatusBadRequest, "clientId is required")
		return
	}
	existing, hasExisting, err := s.controlDB.OAuthClientConfigByProvider(workspaceID, provider.Provider)
	if err != nil {
		s.serverError(w, err)
		return
	}
	clientSecret := strings.TrimSpace(body.ClientSecret)
	if clientSecret == "" && !hasExisting {
		s.jsonError(w, http.StatusBadRequest, "clientSecret is required")
		return
	}
	config := controldb.OAuthClientConfig{
		WorkspaceID: workspaceID,
		Provider:    provider.Provider,
		ClientID:    clientID,
		CreatedBy:   currentUsername(s.currentUser(r)),
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if hasExisting {
		config.CreatedBy = existing.CreatedBy
		config.CreatedAt = existing.CreatedAt
		config.SecretCiphertext = existing.SecretCiphertext
		config.Nonce = existing.Nonce
		config.KeyVersion = existing.KeyVersion
	}
	if clientSecret != "" {
		secret, err := sealOAuthClientSecret(clientSecret)
		if err != nil {
			s.serverError(w, err)
			return
		}
		config.SecretCiphertext = secret.Ciphertext
		config.Nonce = secret.Nonce
		config.KeyVersion = secret.KeyVersion
	}
	extra, err := safeOAuthClientExtra(body.Extra)
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	extraJSON, _ := json.Marshal(extra)
	config.ExtraJSON = string(extraJSON)
	if err := s.controlDB.UpsertOAuthClientConfig(config); err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "connection.oauth_client_config.upsert",
		ResourceType: "connector_provider",
		ResourceID:   provider.Provider,
		Summary:      "OAuth client config saved",
		After: map[string]any{
			"provider": provider.Provider,
			"clientId": clientID,
			"extra":    extra,
		},
		Request: r,
	})
	_ = json.NewEncoder(w).Encode(oauthClientConfigToResponse(r, provider, config, true))
}

func (s *Server) handleDeleteOAuthClientConfig(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	providerID := strings.TrimSpace(r.PathValue("provider"))
	provider, ok, err := s.findOAuthProvider(providerID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !ok {
		s.jsonError(w, http.StatusBadRequest, "provider does not support oauth2")
		return
	}
	if err := s.controlDB.DeleteOAuthClientConfig(workspaceID, provider.Provider); err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "connection.oauth_client_config.delete",
		ResourceType: "connector_provider",
		ResourceID:   provider.Provider,
		Summary:      "OAuth client config deleted",
		Request:      r,
	})
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *Server) oauthProviders() ([]connector.Provider, error) {
	rows, err := s.controlDB.ListConnectorProviders(false)
	if err != nil {
		return nil, err
	}
	out := make([]connector.Provider, 0)
	for _, row := range rows {
		provider, err := connectorProviderFromDB(row)
		if err != nil {
			return nil, err
		}
		if provider.OAuth != nil && providerSupportsAuth(provider, ConnectionAuthOAuth2) {
			out = append(out, provider)
		}
	}
	return out, nil
}

func (s *Server) findOAuthProvider(providerID string) (connector.Provider, bool, error) {
	provider, ok, err := s.findConnectorProvider(providerID)
	if err != nil || !ok {
		return connector.Provider{}, false, err
	}
	if provider.OAuth == nil || !providerSupportsAuth(provider, ConnectionAuthOAuth2) {
		return connector.Provider{}, false, nil
	}
	return provider, true, nil
}

func oauthClientConfigToResponse(r *http.Request, provider connector.Provider, config controldb.OAuthClientConfig, configured bool) oauthClientConfigResponse {
	resp := oauthClientConfigResponse{
		Provider:            provider.Provider,
		DisplayName:         provider.DisplayName,
		Configured:          configured,
		ExpectedRedirectURI: expectedOAuthRedirectURI(r),
		OAuth:               provider.OAuth,
	}
	if configured {
		resp.ClientID = config.ClientID
		resp.Extra = oauthClientExtraMap(config.ExtraJSON)
		resp.CreatedBy = config.CreatedBy
		resp.CreatedAt = config.CreatedAt
		resp.UpdatedAt = config.UpdatedAt
	}
	return resp
}

func expectedOAuthRedirectURI(r *http.Request) string {
	scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		scheme = "http"
		if r.TLS != nil {
			scheme = "https"
		}
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	return scheme + "://" + host + oauthCallbackPath
}

func safeOAuthClientExtra(input map[string]any) (map[string]any, error) {
	out := map[string]any{}
	for key, value := range input {
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("extra field key must not be empty")
		}
		lower := strings.ToLower(key)
		if strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "password") || strings.Contains(lower, "credential") {
			return nil, fmt.Errorf("extra field %q looks secret; put secrets in clientSecret or future secretExtra storage", key)
		}
		out[key] = value
	}
	return out, nil
}

func oauthClientExtraMap(raw string) map[string]any {
	out := map[string]any{}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

type sealedOAuthClientSecret struct {
	Ciphertext string
	Nonce      string
	KeyVersion string
}

func sealOAuthClientSecret(value string) (sealedOAuthClientSecret, error) {
	raw, err := json.Marshal(map[string]string{"clientSecret": value})
	if err != nil {
		return sealedOAuthClientSecret{}, err
	}
	key := strings.TrimSpace(os.Getenv("MULTIGENT_CONNECTION_ENCRYPTION_KEY"))
	if key == "" {
		return sealedOAuthClientSecret{Ciphertext: base64.StdEncoding.EncodeToString(raw), KeyVersion: "dev-plain-base64"}, nil
	}
	sum := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return sealedOAuthClientSecret{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return sealedOAuthClientSecret{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return sealedOAuthClientSecret{}, err
	}
	return sealedOAuthClientSecret{
		Ciphertext: base64.StdEncoding.EncodeToString(gcm.Seal(nil, nonce, raw, nil)),
		Nonce:      hex.EncodeToString(nonce),
		KeyVersion: "aes-gcm-sha256-env",
	}, nil
}

func currentUsername(cur *userRecord) string {
	if cur == nil {
		return ""
	}
	return cur.Username
}
