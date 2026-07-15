package api

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/multigent/multigent/internal/connector"
	controldb "github.com/multigent/multigent/internal/db"
)

const oauthCallbackPath = "/api/v1/oauth/callback"
const oauthStateMaxAge = 15 * time.Minute

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

type oauthAuthorizationStartRequest struct {
	Provider       string         `json:"provider"`
	ConnectionName string         `json:"connectionName"`
	OwnerType      string         `json:"ownerType"`
	Profile        map[string]any `json:"profile"`
}

type oauthAuthorizationStartResponse struct {
	AuthorizationURL string `json:"authorizationUrl"`
	State            string `json:"state"`
}

type oauthAuthorizationState struct {
	WorkspaceID    string         `json:"workspaceId"`
	Provider       string         `json:"provider"`
	ConnectionName string         `json:"connectionName"`
	OwnerType      string         `json:"ownerType"`
	OwnerID        string         `json:"ownerId"`
	CreatedBy      string         `json:"createdBy"`
	CreatedAt      string         `json:"createdAt"`
	Profile        map[string]any `json:"profile"`
}

type oauthTokenResponse struct {
	AccessToken  string         `json:"access_token"`
	TokenType    string         `json:"token_type"`
	RefreshToken string         `json:"refresh_token"`
	ExpiresIn    int            `json:"expires_in"`
	Scope        string         `json:"scope"`
	Raw          map[string]any `json:"-"`
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

func (s *Server) handleStartOAuthAuthorization(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := s.connectionWorkspace(w, r)
	if !ok {
		return
	}
	cur := s.currentUser(r)
	if cur == nil || cur.Username == "" || cur.Username == "apikey" {
		s.jsonError(w, http.StatusForbidden, "authenticated user required")
		return
	}
	var body oauthAuthorizationStartRequest
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	provider, exists, err := s.findOAuthProvider(strings.TrimSpace(body.Provider))
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !exists {
		s.jsonError(w, http.StatusBadRequest, "provider does not support oauth2")
		return
	}
	ownerType := strings.TrimSpace(body.OwnerType)
	if ownerType == "" {
		ownerType = ConnectionOwnerUser
	}
	if ownerType != ConnectionOwnerWorkspace && ownerType != ConnectionOwnerUser {
		s.jsonError(w, http.StatusBadRequest, "ownerType must be workspace or user")
		return
	}
	ownerID := cur.Username
	if ownerType == ConnectionOwnerWorkspace {
		if !s.canAdminCurrentWorkspace(r) {
			s.jsonError(w, http.StatusForbidden, "workspace admin access required")
			return
		}
		ownerID = workspaceID
	}
	clientConfig, configured, err := s.controlDB.OAuthClientConfigByProvider(workspaceID, provider.Provider)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !configured {
		s.jsonError(w, http.StatusBadRequest, "OAuth client config is required")
		return
	}
	connectionName := strings.TrimSpace(body.ConnectionName)
	if connectionName == "" {
		connectionName = "default"
	}
	stateID := newConnectionID("oauth")
	state := oauthAuthorizationState{
		WorkspaceID:    workspaceID,
		Provider:       provider.Provider,
		ConnectionName: connectionName,
		OwnerType:      ownerType,
		OwnerID:        ownerID,
		CreatedBy:      cur.Username,
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
		Profile:        body.Profile,
	}
	rawState, _ := json.Marshal(state)
	if err := s.controlDB.UpsertRecord("oauth_states", workspaceID, []string{stateID}, string(rawState)); err != nil {
		s.serverError(w, err)
		return
	}
	authorizationURL, err := buildOAuthAuthorizationURL(r, provider, clientConfig, stateID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "connection.oauth.start",
		ResourceType: "connector_provider",
		ResourceID:   provider.Provider,
		Summary:      "OAuth authorization started",
		After: map[string]any{
			"provider":       provider.Provider,
			"connectionName": connectionName,
			"ownerType":      ownerType,
			"ownerId":        ownerID,
		},
		Request: r,
	})
	_ = json.NewEncoder(w).Encode(oauthAuthorizationStartResponse{AuthorizationURL: authorizationURL, State: stateID})
}

func (s *Server) handleCompleteOAuthAuthorization(w http.ResponseWriter, r *http.Request) {
	stateID := strings.TrimSpace(r.URL.Query().Get("state"))
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if stateID == "" || code == "" {
		s.jsonError(w, http.StatusBadRequest, "state and code are required")
		return
	}
	state, ok, err := s.takeOAuthState(stateID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !ok {
		s.jsonError(w, http.StatusBadRequest, "OAuth state is missing or expired")
		return
	}
	provider, exists, err := s.findOAuthProvider(state.Provider)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !exists {
		s.jsonError(w, http.StatusBadRequest, "provider does not support oauth2")
		return
	}
	clientConfig, configured, err := s.controlDB.OAuthClientConfigByProvider(state.WorkspaceID, provider.Provider)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !configured {
		s.jsonError(w, http.StatusBadRequest, "OAuth client config is required")
		return
	}
	clientSecret, err := openOAuthClientSecret(clientConfig)
	if err != nil {
		s.serverError(w, err)
		return
	}
	token, err := exchangeOAuthAuthorizationCode(r, provider, clientConfig, clientSecret, code)
	if err != nil {
		s.auditLog(auditLogInput{
			WorkspaceID:  state.WorkspaceID,
			ActorType:    "user",
			ActorID:      state.CreatedBy,
			Action:       "connection.oauth.fail",
			ResourceType: "connector_provider",
			ResourceID:   provider.Provider,
			Summary:      err.Error(),
			Request:      r,
		})
		s.jsonError(w, http.StatusBadGateway, err.Error())
		return
	}
	connection, err := s.upsertOAuthConnection(state, token)
	if err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  state.WorkspaceID,
		ActorType:    "user",
		ActorID:      state.CreatedBy,
		Action:       "connection.oauth.complete",
		ResourceType: "connection",
		ResourceID:   connection.ID,
		Summary:      "OAuth connection completed",
		After:        connectionAuditPayload(connection),
		Request:      r,
	})
	_ = json.NewEncoder(w).Encode(connectionToResponse(connection, nil))
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

func buildOAuthAuthorizationURL(r *http.Request, provider connector.Provider, config controldb.OAuthClientConfig, state string) (string, error) {
	if provider.OAuth == nil {
		return "", fmt.Errorf("provider does not support oauth2")
	}
	u, err := url.Parse(strings.TrimSpace(provider.OAuth.AuthorizationURL))
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("client_id", config.ClientID)
	q.Set("redirect_uri", expectedOAuthRedirectURI(r))
	q.Set("response_type", "code")
	q.Set("state", state)
	if len(provider.OAuth.Scopes) > 0 {
		sep := provider.OAuth.ScopeSeparator
		if sep == "" {
			sep = " "
		}
		q.Set("scope", strings.Join(provider.OAuth.Scopes, sep))
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (s *Server) takeOAuthState(stateID string) (oauthAuthorizationState, bool, error) {
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		return oauthAuthorizationState{}, false, err
	}
	raw, ok, err := s.controlDB.GetRecord("oauth_states", workspaceID, []string{stateID})
	if err != nil || !ok {
		return oauthAuthorizationState{}, false, err
	}
	_ = s.controlDB.DeleteRecord("oauth_states", workspaceID, []string{stateID})
	var state oauthAuthorizationState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return oauthAuthorizationState{}, false, err
	}
	createdAt, err := time.Parse(time.RFC3339, state.CreatedAt)
	if err != nil || time.Since(createdAt) > oauthStateMaxAge {
		return oauthAuthorizationState{}, false, nil
	}
	return state, true, nil
}

func exchangeOAuthAuthorizationCode(r *http.Request, provider connector.Provider, config controldb.OAuthClientConfig, clientSecret, code string) (oauthTokenResponse, error) {
	if provider.OAuth == nil {
		return oauthTokenResponse{}, fmt.Errorf("provider does not support oauth2")
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", config.ClientID)
	form.Set("client_secret", clientSecret)
	form.Set("redirect_uri", expectedOAuthRedirectURI(r))
	return exchangeOAuthToken(r.Context(), provider, form)
}

func exchangeOAuthRefreshToken(ctx context.Context, provider connector.Provider, config controldb.OAuthClientConfig, clientSecret, refreshToken string) (oauthTokenResponse, error) {
	if provider.OAuth == nil {
		return oauthTokenResponse{}, fmt.Errorf("provider does not support oauth2")
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", config.ClientID)
	form.Set("client_secret", clientSecret)
	return exchangeOAuthToken(ctx, provider, form)
}

func exchangeOAuthToken(ctx context.Context, provider connector.Provider, form url.Values) (oauthTokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.OAuth.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return oauthTokenResponse{}, fmt.Errorf("build OAuth token request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return oauthTokenResponse{}, fmt.Errorf("OAuth token request failed: %w", err)
	}
	defer resp.Body.Close()
	rawBody, err := io.ReadAll(io.LimitReader(resp.Body, maxJSONBody+1))
	if err != nil {
		return oauthTokenResponse{}, fmt.Errorf("read OAuth token response: %w", err)
	}
	if len(rawBody) > maxJSONBody {
		return oauthTokenResponse{}, fmt.Errorf("OAuth token response too large")
	}
	payload := map[string]any{}
	if len(rawBody) > 0 {
		if err := json.Unmarshal(rawBody, &payload); err != nil {
			return oauthTokenResponse{}, fmt.Errorf("decode OAuth token response: %w", err)
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return oauthTokenResponse{}, fmt.Errorf("OAuth token request returned HTTP %d", resp.StatusCode)
	}
	accessToken := strings.TrimSpace(stringValue(payload["access_token"]))
	if accessToken == "" {
		accessToken = strings.TrimSpace(stringValue(payload["token"]))
	}
	if accessToken == "" {
		return oauthTokenResponse{}, fmt.Errorf("OAuth token response missing access_token")
	}
	return oauthTokenResponse{
		AccessToken:  accessToken,
		TokenType:    strings.TrimSpace(stringValue(payload["token_type"])),
		RefreshToken: strings.TrimSpace(stringValue(payload["refresh_token"])),
		ExpiresIn:    intNumber(payload["expires_in"]),
		Scope:        strings.TrimSpace(stringValue(payload["scope"])),
		Raw:          payload,
	}, nil
}

func (s *Server) upsertOAuthConnection(state oauthAuthorizationState, token oauthTokenResponse) (controldb.Connection, error) {
	values := map[string]string{
		"accessToken": token.AccessToken,
		"tokenType":   token.TokenType,
	}
	if values["tokenType"] == "" {
		values["tokenType"] = "Bearer"
	}
	if token.RefreshToken != "" {
		values["refreshToken"] = token.RefreshToken
	}
	if token.ExpiresIn > 0 {
		values["expiresAt"] = time.Now().UTC().Add(time.Duration(token.ExpiresIn) * time.Second).Format(time.RFC3339)
	}
	secret, err := sealConnectionSecret(values)
	if err != nil {
		return controldb.Connection{}, err
	}
	connectionID := newConnectionID("conn")
	createdBy := state.CreatedBy
	createdAt := time.Now().UTC().Format(time.RFC3339)
	if existing, ok := s.existingConnection(state.WorkspaceID, state.Provider, state.OwnerType, state.OwnerID, state.ConnectionName); ok {
		connectionID = existing.ID
		createdBy = existing.CreatedBy
		createdAt = existing.CreatedAt
	}
	profile := map[string]any{}
	for key, value := range state.Profile {
		profile[key] = value
	}
	profile["provider"] = state.Provider
	profile["connectionName"] = state.ConnectionName
	profile["displayName"] = firstNonEmpty(firstProfileString(profile, "displayName"), state.Provider+" OAuth")
	profile["accountId"] = firstNonEmpty(firstProfileString(profile, "accountId"), "oauth2")
	if token.Scope != "" {
		profile["scopes"] = strings.Fields(token.Scope)
	}
	if values["expiresAt"] != "" {
		profile["expiresAt"] = values["expiresAt"]
	}
	profileJSON, _ := json.Marshal(profile)
	connection := controldb.Connection{
		ID:             connectionID,
		WorkspaceID:    state.WorkspaceID,
		Provider:       state.Provider,
		ConnectionName: state.ConnectionName,
		OwnerType:      state.OwnerType,
		OwnerID:        state.OwnerID,
		AuthType:       ConnectionAuthOAuth2,
		Status:         "active",
		ProfileJSON:    string(profileJSON),
		CreatedBy:      createdBy,
		CreatedAt:      createdAt,
		UpdatedAt:      time.Now().UTC().Format(time.RFC3339),
	}
	if err := s.controlDB.UpsertConnection(connection); err != nil {
		return controldb.Connection{}, err
	}
	secret.ConnectionID = connection.ID
	if err := s.controlDB.UpsertConnectionSecret(secret); err != nil {
		return controldb.Connection{}, err
	}
	return connection, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func stringValue(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func intNumber(value any) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	default:
		return 0
	}
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

func openOAuthClientSecret(config controldb.OAuthClientConfig) (string, error) {
	switch config.KeyVersion {
	case "dev-plain-base64":
		raw, err := base64.StdEncoding.DecodeString(config.SecretCiphertext)
		if err != nil {
			return "", err
		}
		var payload map[string]string
		if err := json.Unmarshal(raw, &payload); err != nil {
			return "", err
		}
		return strings.TrimSpace(payload["clientSecret"]), nil
	case "aes-gcm-sha256-env":
		key := strings.TrimSpace(os.Getenv("MULTIGENT_CONNECTION_ENCRYPTION_KEY"))
		if key == "" {
			return "", fmt.Errorf("MULTIGENT_CONNECTION_ENCRYPTION_KEY is required to decrypt OAuth client secret")
		}
		sum := sha256.Sum256([]byte(key))
		block, err := aes.NewCipher(sum[:])
		if err != nil {
			return "", err
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return "", err
		}
		nonce, err := hex.DecodeString(config.Nonce)
		if err != nil {
			return "", err
		}
		ciphertext, err := base64.StdEncoding.DecodeString(config.SecretCiphertext)
		if err != nil {
			return "", err
		}
		raw, err := gcm.Open(nil, nonce, ciphertext, nil)
		if err != nil {
			return "", err
		}
		var payload map[string]string
		if err := json.Unmarshal(raw, &payload); err != nil {
			return "", err
		}
		return strings.TrimSpace(payload["clientSecret"]), nil
	default:
		return "", fmt.Errorf("unsupported OAuth client secret key version %q", config.KeyVersion)
	}
}

func currentUsername(cur *userRecord) string {
	if cur == nil {
		return ""
	}
	return cur.Username
}
