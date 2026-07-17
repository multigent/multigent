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
	"sort"
	"strings"
	"time"

	"github.com/multigent/multigent/internal/connector"
	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/imbridge"
	larkbridge "github.com/multigent/multigent/internal/imbridge/lark"
)

const (
	ConnectionOwnerWorkspace = "workspace"
	ConnectionOwnerUser      = "user"

	ConnectionTargetWorkspace = "workspace"
	ConnectionTargetProject   = "project"
	ConnectionTargetAgent     = "agent"
	ConnectionTargetUser      = "user"

	ConnectionAuthNoAuth           = connector.AuthNoAuth
	ConnectionAuthAPIKey           = connector.AuthAPIKey
	ConnectionAuthCustomCredential = connector.AuthCustomCredential
	ConnectionAuthOAuth2           = connector.AuthOAuth2
)

const githubCLIClientID = "178c6fc778ccc68e1d6a"

func (s *Server) handleConnectorProviders(w http.ResponseWriter, r *http.Request) {
	includeDisabled := s.canAdminCurrentWorkspace(r) && strings.TrimSpace(r.URL.Query().Get("includeDisabled")) == "true"
	providers, err := s.controlDB.ListConnectorProviders(includeDisabled)
	if err != nil {
		s.serverError(w, err)
		return
	}
	out := make([]connector.Provider, 0, len(providers))
	for _, row := range providers {
		provider, err := connectorProviderFromDB(row)
		if err != nil {
			s.serverError(w, err)
			return
		}
		out = append(out, provider)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"providers": out})
}

func (s *Server) handleConnectorProvider(w http.ResponseWriter, r *http.Request) {
	providerID := strings.TrimSpace(r.PathValue("provider"))
	provider, ok, err := s.findConnectorProvider(providerID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !ok {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeProviderNotFound, "provider not found")
		return
	}
	_ = json.NewEncoder(w).Encode(provider)
}

func (s *Server) findConnectorProvider(providerID string) (connector.Provider, bool, error) {
	row, ok, err := s.controlDB.ConnectorProviderByID(providerID)
	if err != nil || !ok || !row.Enabled {
		return connector.Provider{}, false, err
	}
	provider, err := connectorProviderFromDB(row)
	if err != nil {
		return connector.Provider{}, false, err
	}
	return provider, true, nil
}

func connectorProviderFromDB(row controldb.ConnectorProvider) (connector.Provider, error) {
	var provider connector.Provider
	if err := json.Unmarshal([]byte(row.CatalogJSON), &provider); err != nil {
		return connector.Provider{}, err
	}
	provider.Provider = row.Provider
	provider.DisplayName = row.DisplayName
	provider.Enabled = row.Enabled
	if len(provider.AuthTypes) == 0 {
		_ = json.Unmarshal([]byte(row.AuthTypesJSON), &provider.AuthTypes)
	}
	if len(provider.RuntimeAdapters) == 0 {
		provider.RuntimeAdapters = connector.DefaultRuntimeAdapters(provider)
	}
	return provider, nil
}

func (s *Server) handleListConnections(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := s.connectionWorkspace(w, r)
	if !ok {
		return
	}
	cur := s.currentUser(r)
	connections, err := s.controlDB.ListConnections(controldb.ConnectionFilter{
		WorkspaceID: workspaceID,
		Provider:    strings.TrimSpace(r.URL.Query().Get("provider")),
		Status:      "active",
	})
	if err != nil {
		s.serverError(w, err)
		return
	}
	out := make([]connectionResponse, 0, len(connections))
	for _, connection := range connections {
		if !s.canReadConnection(r, connection, cur) {
			continue
		}
		grants, _ := s.controlDB.ListConnectionGrants(connection.ID)
		out = append(out, connectionToResponse(connection, grants))
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"connections": out})
}

type createConnectionRequest struct {
	Provider       string            `json:"provider"`
	ConnectionName string            `json:"connectionName"`
	OwnerType      string            `json:"ownerType"`
	AuthType       string            `json:"authType"`
	Values         map[string]string `json:"values"`
	Profile        map[string]any    `json:"profile"`
}

type connectorProviderSetupPollRequest struct {
	DeviceCode string `json:"deviceCode"`
	BaseURL    string `json:"baseUrl"`
}

type connectorProviderSetupBeginResponse struct {
	Status     string `json:"status,omitempty"`
	DeviceCode string `json:"deviceCode"`
	QRURL      string `json:"qrUrl"`
	UserCode   string `json:"userCode,omitempty"`
	Interval   int    `json:"interval"`
	ExpiresIn  int    `json:"expiresIn"`
	BaseURL    string `json:"baseUrl,omitempty"`
	Stage      string `json:"stage,omitempty"`
}

func (s *Server) handleCreateConnection(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := s.connectionWorkspace(w, r)
	if !ok {
		return
	}
	cur := s.currentUser(r)
	if cur == nil || cur.Username == "" || cur.Username == "apikey" {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeAuthenticatedUserRequired, "authenticated user required")
		return
	}
	var body createConnectionRequest
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	body.Provider = strings.TrimSpace(body.Provider)
	provider, exists, err := s.findConnectorProvider(body.Provider)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !exists {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeUnsupportedProvider, "unsupported provider")
		return
	}
	if provider.ComingSoon {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeUnsupportedProvider, "provider is not available yet")
		return
	}
	authType := strings.TrimSpace(body.AuthType)
	if authType == "" {
		authType = provider.AuthTypes[0]
	}
	if !providerSupportsAuth(provider, authType) {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeUnsupportedAuthType, "unsupported auth type")
		return
	}
	if err := validateConnectionValues(provider, authType, body.Values); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, err.Error())
		return
	}
	ownerType := strings.TrimSpace(body.OwnerType)
	if ownerType == "" {
		ownerType = ConnectionOwnerUser
	}
	if ownerType != ConnectionOwnerWorkspace && ownerType != ConnectionOwnerUser {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "ownerType must be workspace or user")
		return
	}
	if ownerType == ConnectionOwnerWorkspace && !s.canAdminCurrentWorkspace(r) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeWorkspaceAdminRequired, "workspace admin access required")
		return
	}
	ownerID := cur.Username
	if ownerType == ConnectionOwnerWorkspace {
		ownerID = workspaceID
	}
	connectionName := strings.TrimSpace(body.ConnectionName)
	if connectionName == "" {
		connectionName = "default"
	}
	profile := body.Profile
	if profile == nil {
		profile = map[string]any{}
	}
	profile["provider"] = body.Provider
	profile["connectionName"] = connectionName
	profileJSON, _ := json.Marshal(profile)
	now := time.Now().UTC().Format(time.RFC3339)
	connectionID := newConnectionID("conn")
	createdBy := cur.Username
	createdAt := now
	if existing, ok := s.existingConnection(workspaceID, body.Provider, ownerType, ownerID, connectionName); ok {
		connectionID = existing.ID
		createdBy = existing.CreatedBy
		createdAt = existing.CreatedAt
	}
	connection := controldb.Connection{
		ID:             connectionID,
		WorkspaceID:    workspaceID,
		Provider:       body.Provider,
		ConnectionName: connectionName,
		OwnerType:      ownerType,
		OwnerID:        ownerID,
		AuthType:       authType,
		Status:         "active",
		ProfileJSON:    string(profileJSON),
		CreatedBy:      createdBy,
		CreatedAt:      createdAt,
		UpdatedAt:      now,
	}
	if err := s.controlDB.UpsertConnection(connection); err != nil {
		s.serverError(w, err)
		return
	}
	if authType != ConnectionAuthNoAuth {
		secret, err := sealConnectionSecret(body.Values)
		if err != nil {
			s.serverError(w, err)
			return
		}
		secret.ConnectionID = connection.ID
		if err := s.controlDB.UpsertConnectionSecret(secret); err != nil {
			s.serverError(w, err)
			return
		}
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "connection.create",
		ResourceType: "connection",
		ResourceID:   connection.ID,
		Summary:      "Connection created",
		After: map[string]any{
			"provider":       connection.Provider,
			"connectionName": connection.ConnectionName,
			"ownerType":      connection.OwnerType,
			"ownerId":        connection.OwnerID,
			"authType":       connection.AuthType,
			"profile":        sanitizeConnectionProfile(connection.Provider, profile),
		},
		Request: r,
	})
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(connectionToResponse(connection, nil))
}

func (s *Server) handleConnectorProviderSetupBegin(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := s.connectionWorkspace(w, r)
	if !ok {
		return
	}
	cur := s.currentUser(r)
	if cur == nil || cur.Username == "" || cur.Username == "apikey" {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeAuthenticatedUserRequired, "authenticated user required")
		return
	}
	providerID := strings.TrimSpace(r.PathValue("provider"))
	provider, exists, err := s.findConnectorProvider(providerID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !exists || provider.ComingSoon {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeUnsupportedProvider, "unsupported provider")
		return
	}
	if provider.Provider == "github" {
		resp, ok := s.beginGitHubDeviceSetup(w, r, workspaceID, provider)
		if !ok {
			return
		}
		_ = json.NewEncoder(w).Encode(resp)
		return
	}
	setupProvider, ok := imbridge.LookupProvider(provider.Provider)
	if !ok {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeUnsupportedProvider, "quick authorization is not supported for this provider")
		return
	}
	ctx, cancel := contextWithRequestTimeout(r, 20*time.Second)
	defer cancel()
	resp, err := setupProvider.BeginSetup(ctx)
	if err != nil {
		s.jsonErrorCode(w, http.StatusBadGateway, ErrCodeUpstreamError, err.Error())
		return
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "connection.setup_begin",
		ResourceType: "connector_provider",
		ResourceID:   provider.Provider,
		Summary:      "Started connector quick authorization",
		After: map[string]any{
			"provider": provider.Provider,
			"baseUrl":  resp.BaseURL,
		},
		Request: r,
	})
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleConnectorProviderSetupPoll(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := s.connectionWorkspace(w, r)
	if !ok {
		return
	}
	cur := s.currentUser(r)
	if cur == nil || cur.Username == "" || cur.Username == "apikey" {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeAuthenticatedUserRequired, "authenticated user required")
		return
	}
	var req connectorProviderSetupPollRequest
	if err := s.readJSON(w, r, &req); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	providerID := strings.TrimSpace(r.PathValue("provider"))
	provider, exists, err := s.findConnectorProvider(providerID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !exists || provider.ComingSoon {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeUnsupportedProvider, "unsupported provider")
		return
	}
	if provider.Provider == "github" {
		resp, ok := s.pollGitHubDeviceSetup(w, r, workspaceID, provider, req.DeviceCode)
		if !ok {
			return
		}
		_ = json.NewEncoder(w).Encode(resp)
		return
	}
	if session, ok := s.connectorSetupSession(req.DeviceCode); ok {
		resp, ok := s.pollConnectorAuthorizationSetup(w, r, workspaceID, provider, session, req.DeviceCode)
		if !ok {
			return
		}
		_ = json.NewEncoder(w).Encode(resp)
		return
	}
	setupProvider, ok := imbridge.LookupProvider(provider.Provider)
	if !ok {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeUnsupportedProvider, "quick authorization is not supported for this provider")
		return
	}
	ctx, cancel := contextWithRequestTimeout(r, 20*time.Second)
	defer cancel()
	poll, err := setupProvider.PollSetup(ctx, req.DeviceCode, req.BaseURL)
	if err != nil {
		s.jsonErrorCode(w, http.StatusBadGateway, ErrCodeUpstreamError, err.Error())
		return
	}
	if poll.Status != "completed" {
		_ = json.NewEncoder(w).Encode(poll)
		return
	}
	actualProvider := strings.TrimSpace(poll.Provider)
	if actualProvider == "" {
		actualProvider = provider.Provider
	}
	authBegin, err := s.beginConnectorAuthorization(r, actualProvider, poll)
	if err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(authBegin)
}

func (s *Server) beginConnectorAuthorization(r *http.Request, providerID string, appPoll imbridge.SetupPollResponse) (connectorProviderSetupBeginResponse, error) {
	setupProvider, ok := imbridge.LookupProvider(providerID)
	if !ok {
		return connectorProviderSetupBeginResponse{}, fmt.Errorf("quick authorization is not supported for this provider")
	}
	ctx, cancel := contextWithRequestTimeout(r, 20*time.Second)
	defer cancel()
	client := larkRegistrationClient()
	begin, err := client.BeginAuthorization(ctx, providerID, appPoll.AppID, appPoll.AppSecret, defaultConnectorScopes(providerID))
	if err != nil {
		return connectorProviderSetupBeginResponse{}, err
	}
	s.putConnectorSetupSession(begin.DeviceCode, connectorDeviceAuthSession{
		Provider:    setupProvider.Info().ID,
		AppID:       appPoll.AppID,
		AppSecret:   appPoll.AppSecret,
		OwnerOpenID: appPoll.OwnerOpenID,
		Brand:       providerID,
		CreatedAt:   time.Now(),
	})
	return connectorProviderSetupBeginResponse{
		Status:     "authorize",
		DeviceCode: begin.DeviceCode,
		QRURL:      begin.QRURL,
		UserCode:   begin.UserCode,
		Interval:   begin.Interval,
		ExpiresIn:  begin.ExpiresIn,
		BaseURL:    begin.BaseURL,
		Stage:      "authorize",
	}, nil
}

func (s *Server) pollConnectorAuthorizationSetup(w http.ResponseWriter, r *http.Request, workspaceID string, provider connector.Provider, session connectorDeviceAuthSession, deviceCode string) (map[string]any, bool) {
	ctx, cancel := contextWithRequestTimeout(r, 20*time.Second)
	defer cancel()
	client := larkRegistrationClient()
	poll, err := client.PollAuthorization(ctx, session.Provider, session.AppID, session.AppSecret, deviceCode)
	if err != nil {
		s.jsonErrorCode(w, http.StatusBadGateway, ErrCodeUpstreamError, err.Error())
		return nil, false
	}
	if poll.Status != "completed" {
		resp := map[string]any{
			"status": poll.Status,
			"stage":  "authorize",
		}
		if poll.SlowDown {
			resp["slowDown"] = true
		}
		if poll.Error != "" {
			resp["error"] = poll.Error
		}
		return resp, true
	}
	s.deleteConnectorSetupSession(deviceCode)
	poll.Provider = session.Provider
	poll.AppID = session.AppID
	poll.AppSecret = session.AppSecret
	poll.OwnerOpenID = session.OwnerOpenID
	connection, err := s.saveConnectorProviderSetupConnection(r, workspaceID, session.Provider, poll)
	if err != nil {
		s.serverError(w, err)
		return nil, false
	}
	return map[string]any{
		"status":     "connected",
		"baseUrl":    poll.BaseURL,
		"connection": connectionToResponse(connection, nil),
	}, true
}

func larkRegistrationClient() larkbridge.RegistrationClient {
	return larkbridge.RegistrationClient{}
}

func defaultConnectorScopes(provider string) []string {
	switch provider {
	case "feishu", "lark":
		return []string{}
	default:
		return nil
	}
}

func (s *Server) putConnectorSetupSession(deviceCode string, session connectorDeviceAuthSession) {
	deviceCode = strings.TrimSpace(deviceCode)
	if deviceCode == "" {
		return
	}
	s.connectorSetupMu.Lock()
	defer s.connectorSetupMu.Unlock()
	s.pruneConnectorSetupSessionsLocked(time.Now())
	s.connectorSetupSessions[deviceCode] = session
}

func (s *Server) connectorSetupSession(deviceCode string) (connectorDeviceAuthSession, bool) {
	deviceCode = strings.TrimSpace(deviceCode)
	if deviceCode == "" {
		return connectorDeviceAuthSession{}, false
	}
	s.connectorSetupMu.Lock()
	defer s.connectorSetupMu.Unlock()
	s.pruneConnectorSetupSessionsLocked(time.Now())
	session, ok := s.connectorSetupSessions[deviceCode]
	return session, ok
}

func (s *Server) deleteConnectorSetupSession(deviceCode string) {
	deviceCode = strings.TrimSpace(deviceCode)
	if deviceCode == "" {
		return
	}
	s.connectorSetupMu.Lock()
	defer s.connectorSetupMu.Unlock()
	delete(s.connectorSetupSessions, deviceCode)
}

func (s *Server) pruneConnectorSetupSessionsLocked(now time.Time) {
	for deviceCode, session := range s.connectorSetupSessions {
		if now.Sub(session.CreatedAt) > 30*time.Minute {
			delete(s.connectorSetupSessions, deviceCode)
		}
	}
}

func (s *Server) saveConnectorProviderSetupConnection(r *http.Request, workspaceID, providerID string, poll imbridge.SetupPollResponse) (controldb.Connection, error) {
	provider, exists, err := s.findConnectorProvider(providerID)
	if err != nil {
		return controldb.Connection{}, err
	}
	if !exists {
		return controldb.Connection{}, fmt.Errorf("unsupported provider %q", providerID)
	}
	openBaseURL, err := imbridge.MustOpenBaseURL(providerID)
	if err != nil {
		return controldb.Connection{}, err
	}
	cur := s.currentUser(r)
	ownerType := ConnectionOwnerUser
	ownerID := ""
	if cur != nil {
		ownerID = cur.Username
	}
	if s.canAdminWorkspace(r, workspaceID) {
		ownerType = ConnectionOwnerWorkspace
		ownerID = workspaceID
	}
	connectionName := "default"
	now := time.Now().UTC().Format(time.RFC3339)
	connectionID := newConnectionID("conn")
	createdBy := requestUsername(r)
	createdAt := now
	if existing, ok := s.existingConnection(workspaceID, providerID, ownerType, ownerID, connectionName); ok {
		connectionID = existing.ID
		createdBy = existing.CreatedBy
		createdAt = existing.CreatedAt
	}
	profileRaw, _ := json.Marshal(map[string]any{
		"displayName":             provider.DisplayName,
		"provider":                providerID,
		"connectionName":          connectionName,
		"baseUrl":                 openBaseURL,
		"accountsUrl":             poll.BaseURL,
		"appId":                   poll.AppID,
		"ownerOpenId":             poll.OwnerOpenID,
		"scope":                   strings.Fields(poll.Scope),
		"createdByQuickAuthorize": true,
	})
	connection := controldb.Connection{
		ID:             connectionID,
		WorkspaceID:    workspaceID,
		Provider:       providerID,
		ConnectionName: connectionName,
		OwnerType:      ownerType,
		OwnerID:        ownerID,
		AuthType:       ConnectionAuthCustomCredential,
		Status:         "active",
		ProfileJSON:    string(profileRaw),
		CreatedBy:      createdBy,
		CreatedAt:      createdAt,
		UpdatedAt:      now,
	}
	if err := s.controlDB.UpsertConnection(connection); err != nil {
		return controldb.Connection{}, err
	}
	secret, err := sealConnectionSecret(map[string]string{
		"baseUrl":          openBaseURL,
		"appId":            poll.AppID,
		"appSecret":        poll.AppSecret,
		"accessToken":      poll.AccessToken,
		"refreshToken":     poll.RefreshToken,
		"tokenType":        firstNonEmpty(poll.TokenType, "Bearer"),
		"scope":            poll.Scope,
		"expiresAt":        expiresAtFromSeconds(poll.ExpiresIn),
		"refreshExpiresAt": expiresAtFromSeconds(poll.RefreshExpiresIn),
	})
	if err != nil {
		return controldb.Connection{}, err
	}
	secret.ConnectionID = connectionID
	if err := s.controlDB.UpsertConnectionSecret(secret); err != nil {
		return controldb.Connection{}, err
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "connection.connected",
		ResourceType: "connection",
		ResourceID:   connection.ID,
		Summary:      "Connector quick authorization completed",
		After: map[string]any{
			"provider":       connection.Provider,
			"connectionName": connection.ConnectionName,
			"ownerType":      connection.OwnerType,
			"ownerId":        connection.OwnerID,
			"authType":       connection.AuthType,
			"profile":        sanitizeConnectionProfile(connection.Provider, connectionProfileMap(connection)),
		},
		Request: r,
	})
	return connection, nil
}

func (s *Server) beginGitHubDeviceSetup(w http.ResponseWriter, r *http.Request, workspaceID string, provider connector.Provider) (connectorProviderSetupBeginResponse, bool) {
	ctx, cancel := contextWithRequestTimeout(r, 20*time.Second)
	defer cancel()
	resp, err := githubDeviceCode(ctx, provider, githubCLIClientID)
	if err != nil {
		s.jsonErrorCode(w, http.StatusBadGateway, ErrCodeUpstreamError, err.Error())
		return connectorProviderSetupBeginResponse{}, false
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "connection.setup_begin",
		ResourceType: "connector_provider",
		ResourceID:   provider.Provider,
		Summary:      "Started GitHub device authorization",
		Request:      r,
	})
	return resp, true
}

func (s *Server) pollGitHubDeviceSetup(w http.ResponseWriter, r *http.Request, workspaceID string, provider connector.Provider, deviceCode string) (map[string]any, bool) {
	deviceCode = strings.TrimSpace(deviceCode)
	if deviceCode == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "deviceCode is required")
		return nil, false
	}
	ctx, cancel := contextWithRequestTimeout(r, 20*time.Second)
	defer cancel()
	token, status, slowDown, tokenErr, err := githubDeviceToken(ctx, provider, githubCLIClientID, deviceCode)
	if err != nil {
		s.jsonErrorCode(w, http.StatusBadGateway, ErrCodeUpstreamError, err.Error())
		return nil, false
	}
	if status != "completed" {
		resp := map[string]any{"status": status}
		if slowDown {
			resp["slowDown"] = true
		}
		if tokenErr != "" {
			resp["error"] = tokenErr
		}
		return resp, true
	}
	state := s.deviceOAuthConnectionState(r, workspaceID, provider)
	connection, err := s.upsertOAuthConnection(state, token)
	if err != nil {
		s.serverError(w, err)
		return nil, false
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "connection.connected",
		ResourceType: "connection",
		ResourceID:   connection.ID,
		Summary:      "GitHub device authorization completed",
		After:        connectionAuditPayload(connection),
		Request:      r,
	})
	return map[string]any{
		"status":     "connected",
		"connection": connectionToResponse(connection, nil),
	}, true
}

func (s *Server) deviceOAuthConnectionState(r *http.Request, workspaceID string, provider connector.Provider) oauthAuthorizationState {
	ownerType := ConnectionOwnerUser
	ownerID := currentUsername(s.currentUser(r))
	if s.canAdminWorkspace(r, workspaceID) {
		ownerType = ConnectionOwnerWorkspace
		ownerID = workspaceID
	}
	return oauthAuthorizationState{
		WorkspaceID:    workspaceID,
		Provider:       provider.Provider,
		ConnectionName: "default",
		OwnerType:      ownerType,
		OwnerID:        ownerID,
		CreatedBy:      currentUsername(s.currentUser(r)),
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
		Profile: map[string]any{
			"displayName": provider.DisplayName,
		},
	}
}

func githubDeviceCode(ctx context.Context, provider connector.Provider, clientID string) (connectorProviderSetupBeginResponse, error) {
	endpoint := "https://github.com/login/device/code"
	scopes := []string{}
	if provider.OAuth != nil {
		endpoint = strings.TrimSpace(provider.OAuth.AuthorizationURL)
		endpoint = strings.TrimSuffix(endpoint, "/authorize") + "/device/code"
		scopes = provider.OAuth.Scopes
	}
	form := url.Values{}
	form.Set("client_id", clientID)
	if len(scopes) > 0 {
		form.Set("scope", strings.Join(scopes, " "))
	}
	payload, err := postFormJSON(ctx, endpoint, form)
	if err != nil {
		return connectorProviderSetupBeginResponse{}, err
	}
	if errMsg := strings.TrimSpace(stringValue(payload["error"])); errMsg != "" {
		return connectorProviderSetupBeginResponse{}, fmt.Errorf("%s: %s", errMsg, strings.TrimSpace(stringValue(payload["error_description"])))
	}
	deviceCode := strings.TrimSpace(stringValue(payload["device_code"]))
	verificationURI := strings.TrimSpace(stringValue(payload["verification_uri"]))
	userCode := strings.TrimSpace(stringValue(payload["user_code"]))
	if deviceCode == "" || verificationURI == "" || userCode == "" {
		return connectorProviderSetupBeginResponse{}, fmt.Errorf("GitHub device response is incomplete")
	}
	interval := intNumber(payload["interval"])
	if interval <= 0 {
		interval = 5
	}
	return connectorProviderSetupBeginResponse{
		DeviceCode: deviceCode,
		QRURL:      verificationURI,
		UserCode:   userCode,
		Interval:   interval,
		ExpiresIn:  intNumber(payload["expires_in"]),
		BaseURL:    "https://github.com",
	}, nil
}

func githubDeviceToken(ctx context.Context, provider connector.Provider, clientID, deviceCode string) (oauthTokenResponse, string, bool, string, error) {
	endpoint := "https://github.com/login/oauth/access_token"
	if provider.OAuth != nil && strings.TrimSpace(provider.OAuth.TokenURL) != "" {
		endpoint = provider.OAuth.TokenURL
	}
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("device_code", deviceCode)
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	payload, err := postFormJSON(ctx, endpoint, form)
	if err != nil {
		return oauthTokenResponse{}, "", false, "", err
	}
	if errMsg := strings.TrimSpace(stringValue(payload["error"])); errMsg != "" {
		desc := strings.TrimSpace(stringValue(payload["error_description"]))
		switch errMsg {
		case "authorization_pending":
			return oauthTokenResponse{}, "pending", false, "", nil
		case "slow_down":
			return oauthTokenResponse{}, "pending", true, "", nil
		case "expired_token":
			return oauthTokenResponse{}, "expired", false, firstNonEmpty(desc, errMsg), nil
		case "access_denied":
			return oauthTokenResponse{}, "denied", false, firstNonEmpty(desc, errMsg), nil
		default:
			return oauthTokenResponse{}, "error", false, firstNonEmpty(desc, errMsg), nil
		}
	}
	accessToken := strings.TrimSpace(stringValue(payload["access_token"]))
	if accessToken == "" {
		return oauthTokenResponse{}, "pending", false, "", nil
	}
	return oauthTokenResponse{
		AccessToken:  accessToken,
		TokenType:    strings.TrimSpace(stringValue(payload["token_type"])),
		RefreshToken: strings.TrimSpace(stringValue(payload["refresh_token"])),
		ExpiresIn:    intNumber(payload["expires_in"]),
		Scope:        strings.TrimSpace(stringValue(payload["scope"])),
		Raw:          payload,
	}, "completed", false, "", nil
}

func postFormJSON(ctx context.Context, endpoint string, form url.Values) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxJSONBody))
	if err != nil {
		return nil, err
	}
	payload := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if errMsg := strings.TrimSpace(stringValue(payload["error"])); errMsg != "" {
			return payload, nil
		}
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return payload, nil
}

func (s *Server) existingConnection(workspaceID, provider, ownerType, ownerID, connectionName string) (controldb.Connection, bool) {
	connections, err := s.controlDB.ListConnections(controldb.ConnectionFilter{
		WorkspaceID: workspaceID,
		Provider:    provider,
		OwnerType:   ownerType,
		OwnerID:     ownerID,
	})
	if err != nil {
		return controldb.Connection{}, false
	}
	for _, connection := range connections {
		if connection.ConnectionName == connectionName {
			return connection, true
		}
	}
	return controldb.Connection{}, false
}

func (s *Server) handleGetConnection(w http.ResponseWriter, r *http.Request) {
	connection, ok := s.connectionByIDWithAccess(w, r)
	if !ok {
		return
	}
	grants, err := s.controlDB.ListConnectionGrants(connection.ID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(connectionToResponse(connection, grants))
}

func (s *Server) handleUpdateConnection(w http.ResponseWriter, r *http.Request) {
	connection, ok := s.connectionByIDWithAccess(w, r)
	if !ok {
		return
	}
	cur := s.currentUser(r)
	if !s.canManageConnection(r, connection, cur) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeConnectionManagementRequired, "connection management access required")
		return
	}
	var body createConnectionRequest
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	provider, exists, err := s.findConnectorProvider(connection.Provider)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !exists {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeUnsupportedProvider, "unsupported provider")
		return
	}
	authType := strings.TrimSpace(body.AuthType)
	if authType == "" {
		authType = connection.AuthType
	}
	if !providerSupportsAuth(provider, authType) {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeUnsupportedAuthType, "unsupported auth type")
		return
	}
	existingSecret, hasExistingSecret, err := s.controlDB.ConnectionSecret(connection.ID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	shouldUpdateSecret := authType != ConnectionAuthNoAuth && len(body.Values) > 0
	if authType != ConnectionAuthNoAuth && !hasExistingSecret && !shouldUpdateSecret {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeCredentialValuesRequired, "credential values are required")
		return
	}
	if authType != ConnectionAuthNoAuth && authType != connection.AuthType && !shouldUpdateSecret {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeCredentialValuesRequired, "credential values are required when changing auth type")
		return
	}
	secretValues := body.Values
	if shouldUpdateSecret {
		if hasExistingSecret {
			opened, err := openConnectionSecret(existingSecret)
			if err != nil {
				s.serverError(w, err)
				return
			}
			secretValues = opened
			for key, value := range body.Values {
				secretValues[key] = value
			}
		}
		if err := validateConnectionValues(provider, authType, secretValues); err != nil {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, err.Error())
			return
		}
	}
	updated := connection
	if name := strings.TrimSpace(body.ConnectionName); name != "" {
		updated.ConnectionName = name
	}
	updated.AuthType = authType
	updated.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	profile := connectionProfileMap(connection)
	for k, v := range body.Profile {
		profile[k] = v
	}
	profile["provider"] = updated.Provider
	profile["connectionName"] = updated.ConnectionName
	profileJSON, _ := json.Marshal(profile)
	updated.ProfileJSON = string(profileJSON)

	if err := s.controlDB.UpdateConnection(updated); err != nil {
		s.serverError(w, err)
		return
	}
	if shouldUpdateSecret {
		secret, err := sealConnectionSecret(secretValues)
		if err != nil {
			s.serverError(w, err)
			return
		}
		secret.ConnectionID = updated.ID
		if err := s.controlDB.UpsertConnectionSecret(secret); err != nil {
			s.serverError(w, err)
			return
		}
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  updated.WorkspaceID,
		Action:       "connection.update",
		ResourceType: "connection",
		ResourceID:   updated.ID,
		Summary:      "Connection updated",
		Before:       connectionAuditPayload(connection),
		After:        connectionAuditPayload(updated),
		Request:      r,
	})
	grants, _ := s.controlDB.ListConnectionGrants(updated.ID)
	_ = json.NewEncoder(w).Encode(connectionToResponse(updated, grants))
}

func (s *Server) handleDeleteConnection(w http.ResponseWriter, r *http.Request) {
	connection, ok := s.connectionByIDWithAccess(w, r)
	if !ok {
		return
	}
	if !s.canManageConnection(r, connection, s.currentUser(r)) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeConnectionManagementRequired, "connection management access required")
		return
	}
	if err := s.controlDB.DeleteConnection(connection.ID); err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  connection.WorkspaceID,
		Action:       "connection.revoke",
		ResourceType: "connection",
		ResourceID:   connection.ID,
		Summary:      "Connection revoked",
		Before:       connectionAuditPayload(connection),
		Request:      r,
	})
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func connectionProfileMap(connection controldb.Connection) map[string]any {
	profile := map[string]any{}
	_ = json.Unmarshal([]byte(connection.ProfileJSON), &profile)
	return profile
}

type createConnectionGrantRequest struct {
	TargetType string `json:"targetType"`
	TargetID   string `json:"targetId"`
}

func (s *Server) handleListConnectionGrants(w http.ResponseWriter, r *http.Request) {
	connection, ok := s.connectionByIDWithAccess(w, r)
	if !ok {
		return
	}
	grants, err := s.controlDB.ListConnectionGrants(connection.ID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"grants": grantsToResponse(grants)})
}

func (s *Server) handleCreateConnectionGrant(w http.ResponseWriter, r *http.Request) {
	connection, ok := s.connectionByIDWithAccess(w, r)
	if !ok {
		return
	}
	cur := s.currentUser(r)
	if !s.canManageConnection(r, connection, cur) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeConnectionManagementRequired, "connection management access required")
		return
	}
	var body createConnectionGrantRequest
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	body.TargetType = strings.TrimSpace(body.TargetType)
	body.TargetID = strings.TrimSpace(body.TargetID)
	if err := s.validateConnectionGrantTarget(r, connection, body.TargetType, body.TargetID); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, err.Error())
		return
	}
	grant := controldb.ConnectionGrant{
		ID:           newConnectionID("grant"),
		WorkspaceID:  connection.WorkspaceID,
		ConnectionID: connection.ID,
		TargetType:   body.TargetType,
		TargetID:     body.TargetID,
		CreatedBy:    cur.Username,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	if err := s.controlDB.CreateConnectionGrant(grant); err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  connection.WorkspaceID,
		Action:       "connection.grant.create",
		ResourceType: "connection",
		ResourceID:   connection.ID,
		Summary:      "Connection grant created",
		After:        grantToResponse(grant),
		Request:      r,
	})
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(grantToResponse(grant))
}

func (s *Server) handleDeleteConnectionGrant(w http.ResponseWriter, r *http.Request) {
	connection, ok := s.connectionByIDWithAccess(w, r)
	if !ok {
		return
	}
	if !s.canManageConnection(r, connection, s.currentUser(r)) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeConnectionManagementRequired, "connection management access required")
		return
	}
	grantID := strings.TrimSpace(r.PathValue("grantId"))
	grants, err := s.controlDB.ListConnectionGrants(connection.ID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	var target *controldb.ConnectionGrant
	for _, grant := range grants {
		if grant.ID == grantID {
			g := grant
			target = &g
			break
		}
	}
	if target == nil {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeConnectionGrantNotFound, "grant not found")
		return
	}
	if err := s.controlDB.DeleteConnectionGrant(grantID); err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  connection.WorkspaceID,
		Action:       "connection.grant.delete",
		ResourceType: "connection",
		ResourceID:   connection.ID,
		Summary:      "Connection grant deleted",
		Before:       grantToResponse(*target),
		Request:      r,
	})
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *Server) connectionWorkspace(w http.ResponseWriter, r *http.Request) (string, bool) {
	if !s.checkCurrentWorkspaceAccess(w, r) {
		return "", false
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return "", false
	}
	return workspaceID, true
}

func (s *Server) connectionByIDWithAccess(w http.ResponseWriter, r *http.Request) (controldb.Connection, bool) {
	workspaceID, ok := s.connectionWorkspace(w, r)
	if !ok {
		return controldb.Connection{}, false
	}
	id := strings.TrimSpace(r.PathValue("id"))
	connection, ok, err := s.controlDB.ConnectionByID(id)
	if err != nil {
		s.serverError(w, err)
		return controldb.Connection{}, false
	}
	if !ok {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeConnectionNotFound, "connection not found")
		return controldb.Connection{}, false
	}
	if connection.WorkspaceID != workspaceID {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeConnectionNotFound, "connection not found")
		return controldb.Connection{}, false
	}
	if !s.canReadConnection(r, connection, s.currentUser(r)) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeConnectionAccessRequired, "connection access required")
		return controldb.Connection{}, false
	}
	return connection, true
}

func (s *Server) canReadConnection(r *http.Request, connection controldb.Connection, cur *userRecord) bool {
	if s.canAdminWorkspace(r, connection.WorkspaceID) {
		return true
	}
	if cur == nil || cur.Username == "" {
		return false
	}
	if connection.OwnerType == ConnectionOwnerUser && connection.OwnerID == cur.Username {
		return true
	}
	return false
}

func (s *Server) canManageConnection(r *http.Request, connection controldb.Connection, cur *userRecord) bool {
	switch connection.OwnerType {
	case ConnectionOwnerWorkspace:
		return s.canAdminWorkspace(r, connection.WorkspaceID)
	case ConnectionOwnerUser:
		return cur != nil && cur.Username != "" && connection.OwnerID == cur.Username
	default:
		return false
	}
}

func (s *Server) validateConnectionGrantTarget(r *http.Request, connection controldb.Connection, targetType, targetID string) error {
	if targetType == "" || targetID == "" {
		return fmt.Errorf("targetType and targetId are required")
	}
	if connection.OwnerType == ConnectionOwnerUser {
		return s.validateUserOwnedConnectionGrantTarget(r, connection, targetType, targetID)
	}
	switch targetType {
	case ConnectionTargetWorkspace:
		if !s.canAdminWorkspace(r, connection.WorkspaceID) {
			return fmt.Errorf("workspace grant requires workspace admin")
		}
		if targetID != connection.WorkspaceID {
			return fmt.Errorf("workspace targetId must be current workspace id")
		}
	case ConnectionTargetProject:
		if !s.canAdminWorkspace(r, connection.WorkspaceID) {
			return fmt.Errorf("project grant requires workspace admin")
		}
		if _, err := s.st.Project(targetID); err != nil {
			return fmt.Errorf("project not found")
		}
	case ConnectionTargetAgent:
		parts := strings.SplitN(targetID, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" || !s.agentExistsInProject(parts[0], parts[1]) {
			return fmt.Errorf("agent target must be project/agent")
		}
		if !s.canAdminWorkspace(r, connection.WorkspaceID) && !currentUserLinkedAgent(s.currentUser(r), targetID) {
			return fmt.Errorf("agent grant requires workspace admin or linked agent")
		}
	case ConnectionTargetUser:
		cur := s.currentUser(r)
		if cur == nil || cur.Username == "" {
			return fmt.Errorf("authenticated user required")
		}
		if targetID != cur.Username && !s.canAdminWorkspace(r, connection.WorkspaceID) {
			return fmt.Errorf("user grant can only target yourself")
		}
	default:
		return fmt.Errorf("unsupported targetType")
	}
	return nil
}

func (s *Server) validateUserOwnedConnectionGrantTarget(r *http.Request, connection controldb.Connection, targetType, targetID string) error {
	cur := s.currentUser(r)
	if cur == nil || cur.Username == "" || cur.Username != connection.OwnerID {
		return fmt.Errorf("user-owned connection grants must be managed by the connection owner")
	}
	switch targetType {
	case ConnectionTargetWorkspace, ConnectionTargetProject:
		return fmt.Errorf("user-owned connections can only be granted to the owner or the owner's linked agents")
	case ConnectionTargetAgent:
		parts := strings.SplitN(targetID, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" || !s.agentExistsInProject(parts[0], parts[1]) {
			return fmt.Errorf("agent target must be project/agent")
		}
		if !s.canOperateAgent(r, parts[0], parts[1]) {
			return fmt.Errorf("user-owned connection can only be granted to agents the owner can operate")
		}
	case ConnectionTargetUser:
		if targetID != connection.OwnerID {
			return fmt.Errorf("user-owned connection can only be granted to its owner")
		}
	default:
		return fmt.Errorf("unsupported targetType")
	}
	return nil
}

func (s *Server) userRecordByUsername(username string) (*userRecord, bool) {
	if s == nil || s.users == nil {
		return nil, false
	}
	u := s.users.GetUser(username)
	if u == nil {
		return nil, false
	}
	return u, true
}

func currentUserLinkedAgent(cur *userRecord, agentRef string) bool {
	if cur == nil {
		return false
	}
	for _, linked := range cur.LinkedAgents {
		if linked == agentRef {
			return true
		}
	}
	return false
}

func (s *Server) canAdminCurrentWorkspace(r *http.Request) bool {
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		return false
	}
	return s.canAdminWorkspace(r, workspaceID)
}

func (s *Server) canAdminWorkspace(r *http.Request, workspaceID string) bool {
	cur := s.currentUser(r)
	if cur != nil && cur.Username == "apikey" {
		return true
	}
	if cur == nil || cur.Username == "" || s.controlDB == nil {
		return false
	}
	if cur.Role == RoleAdmin {
		return true
	}
	member, ok, err := s.controlDB.WorkspaceMember(workspaceID, cur.Username)
	if err != nil || !ok {
		return false
	}
	return member.Role == WorkspaceRoleOwner || member.Role == WorkspaceRoleAdmin
}

func providerSupportsAuth(provider connector.Provider, authType string) bool {
	return connector.SupportsAuth(provider, authType)
}

func validateConnectionValues(provider connector.Provider, authType string, values map[string]string) error {
	if authType == ConnectionAuthNoAuth {
		return nil
	}
	if values == nil {
		values = map[string]string{}
	}
	required := map[string]bool{}
	for _, field := range provider.Fields {
		if field.Required {
			required[field.Key] = true
		}
	}
	if authType == ConnectionAuthAPIKey {
		required["apiKey"] = true
	}
	for key := range values {
		if key == "" {
			return fmt.Errorf("empty credential key")
		}
		known := key == "apiKey"
		for _, field := range provider.Fields {
			if field.Key == key {
				known = true
				break
			}
		}
		if !known {
			return fmt.Errorf("unknown credential field %q", key)
		}
	}
	for key := range required {
		if strings.TrimSpace(values[key]) == "" {
			return fmt.Errorf("credential field %q is required", key)
		}
	}
	if raw := strings.TrimSpace(values["mcpServerUrl"]); raw != "" {
		if err := validateMCPGatewayServerURL(raw); err != nil {
			return err
		}
	}
	return nil
}

func sealConnectionSecret(values map[string]string) (controldb.ConnectionSecret, error) {
	raw, err := json.Marshal(values)
	if err != nil {
		return controldb.ConnectionSecret{}, err
	}
	key := strings.TrimSpace(os.Getenv("MULTIGENT_CONNECTION_ENCRYPTION_KEY"))
	if key == "" {
		return controldb.ConnectionSecret{
			Ciphertext: base64.StdEncoding.EncodeToString(raw),
			KeyVersion: "plain-dev",
			UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
		}, nil
	}
	sum := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return controldb.ConnectionSecret{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return controldb.ConnectionSecret{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return controldb.ConnectionSecret{}, err
	}
	ciphertext := gcm.Seal(nil, nonce, raw, nil)
	return controldb.ConnectionSecret{
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		KeyVersion: "env-v1",
		UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func openConnectionSecret(secret controldb.ConnectionSecret) (map[string]string, error) {
	if secret.Ciphertext == "" {
		return map[string]string{}, nil
	}
	var raw []byte
	switch secret.KeyVersion {
	case "", "plain-dev":
		decoded, err := base64.StdEncoding.DecodeString(secret.Ciphertext)
		if err != nil {
			return nil, err
		}
		raw = decoded
	case "env-v1":
		key := strings.TrimSpace(os.Getenv("MULTIGENT_CONNECTION_ENCRYPTION_KEY"))
		if key == "" {
			return nil, fmt.Errorf("MULTIGENT_CONNECTION_ENCRYPTION_KEY is required to decrypt connection secret")
		}
		ciphertext, err := base64.StdEncoding.DecodeString(secret.Ciphertext)
		if err != nil {
			return nil, err
		}
		nonce, err := base64.StdEncoding.DecodeString(secret.Nonce)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256([]byte(key))
		block, err := aes.NewCipher(sum[:])
		if err != nil {
			return nil, err
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}
		opened, err := gcm.Open(nil, nonce, ciphertext, nil)
		if err != nil {
			return nil, err
		}
		raw = opened
	default:
		return nil, fmt.Errorf("unsupported connection secret key version %q", secret.KeyVersion)
	}
	out := map[string]string{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

type connectionResponse struct {
	ID             string                   `json:"id"`
	Provider       string                   `json:"provider"`
	ConnectionName string                   `json:"connectionName"`
	OwnerType      string                   `json:"ownerType"`
	OwnerID        string                   `json:"ownerId"`
	AuthType       string                   `json:"authType"`
	Status         string                   `json:"status"`
	Profile        map[string]any           `json:"profile"`
	ProfileSummary connectionProfileSummary `json:"profileSummary"`
	Grants         []connectionGrantModel   `json:"grants,omitempty"`
	CreatedBy      string                   `json:"createdBy,omitempty"`
	CreatedAt      string                   `json:"createdAt"`
	UpdatedAt      string                   `json:"updatedAt,omitempty"`
	LastUsedAt     string                   `json:"lastUsedAt,omitempty"`
}

type connectionGrantModel struct {
	ID         string `json:"id"`
	TargetType string `json:"targetType"`
	TargetID   string `json:"targetId"`
	CreatedBy  string `json:"createdBy,omitempty"`
	CreatedAt  string `json:"createdAt"`
}

func connectionToResponse(connection controldb.Connection, grants []controldb.ConnectionGrant) connectionResponse {
	profile := map[string]any{}
	_ = json.Unmarshal([]byte(connection.ProfileJSON), &profile)
	sort.Slice(grants, func(i, j int) bool { return grants[i].CreatedAt < grants[j].CreatedAt })
	return connectionResponse{
		ID:             connection.ID,
		Provider:       connection.Provider,
		ConnectionName: connection.ConnectionName,
		OwnerType:      connection.OwnerType,
		OwnerID:        connection.OwnerID,
		AuthType:       connection.AuthType,
		Status:         connection.Status,
		Profile:        sanitizeConnectionProfile(connection.Provider, profile),
		ProfileSummary: summarizeConnectionProfile(connection, profile),
		Grants:         grantsToResponse(grants),
		CreatedBy:      connection.CreatedBy,
		CreatedAt:      connection.CreatedAt,
		UpdatedAt:      connection.UpdatedAt,
		LastUsedAt:     connection.LastUsedAt,
	}
}

func grantsToResponse(grants []controldb.ConnectionGrant) []connectionGrantModel {
	out := make([]connectionGrantModel, 0, len(grants))
	for _, grant := range grants {
		out = append(out, grantToResponse(grant))
	}
	return out
}

func grantToResponse(grant controldb.ConnectionGrant) connectionGrantModel {
	return connectionGrantModel{
		ID:         grant.ID,
		TargetType: grant.TargetType,
		TargetID:   grant.TargetID,
		CreatedBy:  grant.CreatedBy,
		CreatedAt:  grant.CreatedAt,
	}
}

func connectionAuditPayload(connection controldb.Connection) map[string]any {
	profile := map[string]any{}
	_ = json.Unmarshal([]byte(connection.ProfileJSON), &profile)
	return map[string]any{
		"provider":       connection.Provider,
		"connectionName": connection.ConnectionName,
		"ownerType":      connection.OwnerType,
		"ownerId":        connection.OwnerID,
		"authType":       connection.AuthType,
		"status":         connection.Status,
		"profile":        sanitizeConnectionProfile(connection.Provider, profile),
		"profileSummary": summarizeConnectionProfile(connection, profile),
	}
}

type connectionProfileSummary struct {
	DisplayName         string                   `json:"displayName,omitempty"`
	AccountID           string                   `json:"accountId,omitempty"`
	AccountName         string                   `json:"accountName,omitempty"`
	AccountEmail        string                   `json:"accountEmail,omitempty"`
	Scopes              []string                 `json:"scopes,omitempty"`
	ProviderPermissions []string                 `json:"providerPermissions,omitempty"`
	ActionPolicy        runtimeActionPolicyModel `json:"actionPolicy,omitempty"`
}

type runtimeActionPolicyModel struct {
	AllowedMethods   []string `json:"allowedMethods,omitempty"`
	BlockedMethods   []string `json:"blockedMethods,omitempty"`
	AllowedEndpoints []string `json:"allowedEndpoints,omitempty"`
	BlockedEndpoints []string `json:"blockedEndpoints,omitempty"`
}

func summarizeConnectionProfile(connection controldb.Connection, profile map[string]any) connectionProfileSummary {
	policy := runtimeActionPolicyFromProfile(profile)
	return connectionProfileSummary{
		DisplayName:         firstProfileString(profile, "displayName", "name", "label"),
		AccountID:           firstProfileString(profile, "accountId", "accountID", "userId", "userID", "teamId", "teamID", "tenantId", "tenantID", "botId", "botID"),
		AccountName:         firstProfileString(profile, "accountName", "username", "login", "teamName", "tenantName"),
		AccountEmail:        firstProfileString(profile, "accountEmail", "email"),
		Scopes:              firstProfileStringList(profile, "scopes", "grantedScopes", "granted_scopes", "requiredScopes"),
		ProviderPermissions: firstProfileStringList(profile, "providerPermissions", "permissions", "grantedPermissions", "granted_permissions"),
		ActionPolicy: runtimeActionPolicyModel{
			AllowedMethods:   policy.AllowedMethods,
			BlockedMethods:   policy.BlockedMethods,
			AllowedEndpoints: policy.AllowedEndpoints,
			BlockedEndpoints: policy.BlockedEndpoints,
		},
	}
}

func firstProfileString(profile map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := profile[key].(string); ok {
			if value = strings.TrimSpace(value); value != "" {
				return value
			}
		}
	}
	return ""
}

func firstProfileStringList(profile map[string]any, keys ...string) []string {
	for _, key := range keys {
		items := runtimeActionPolicyList(profile, key)
		if len(items) > 0 {
			return items
		}
	}
	return nil
}

func sanitizeConnectionProfile(providerID string, profile map[string]any) map[string]any {
	secretKeys := map[string]bool{
		"apiKey":     true,
		"api_key":    true,
		"appSecret":  true,
		"app_secret": true,
		"password":   true,
		"secret":     true,
		"token":      true,
		"credential": true,
	}
	if provider, ok := defaultConnectorProvider(providerID); ok {
		for _, field := range provider.Fields {
			if field.Secret {
				secretKeys[field.Key] = true
			}
		}
	}
	out := make(map[string]any, len(profile))
	for key, value := range profile {
		if secretKeys[key] {
			continue
		}
		out[key] = value
	}
	return out
}

func defaultConnectorProvider(providerID string) (connector.Provider, bool) {
	for _, provider := range connector.Defaults() {
		if provider.Provider == providerID {
			return provider, true
		}
	}
	return connector.Provider{}, false
}

func expiresAtFromSeconds(seconds int) string {
	if seconds <= 0 {
		return ""
	}
	return time.Now().UTC().Add(time.Duration(seconds) * time.Second).Format(time.RFC3339)
}

func newConnectionID(prefix string) string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(b[:])
}
