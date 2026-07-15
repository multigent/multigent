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
	"sort"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
)

const (
	ConnectionOwnerWorkspace = "workspace"
	ConnectionOwnerUser      = "user"

	ConnectionTargetWorkspace = "workspace"
	ConnectionTargetProject   = "project"
	ConnectionTargetAgent     = "agent"
	ConnectionTargetUser      = "user"

	ConnectionAuthNoAuth           = "no_auth"
	ConnectionAuthAPIKey           = "api_key"
	ConnectionAuthCustomCredential = "custom_credential"
)

type connectorProvider struct {
	Provider    string                   `json:"provider"`
	DisplayName string                   `json:"displayName"`
	AuthTypes   []string                 `json:"authTypes"`
	Fields      []connectorProviderField `json:"fields,omitempty"`
	Enabled     bool                     `json:"enabled"`
}

type connectorProviderField struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	InputType string `json:"inputType"`
	Required  bool   `json:"required"`
	Secret    bool   `json:"secret"`
}

var connectorProviders = []connectorProvider{
	{
		Provider:    "github",
		DisplayName: "GitHub",
		AuthTypes:   []string{ConnectionAuthAPIKey},
		Fields: []connectorProviderField{
			{Key: "apiKey", Label: "Personal access token", InputType: "password", Required: true, Secret: true},
		},
		Enabled: true,
	},
	{
		Provider:    "feishu",
		DisplayName: "Feishu / Lark",
		AuthTypes:   []string{ConnectionAuthCustomCredential},
		Fields: []connectorProviderField{
			{Key: "appId", Label: "App ID", InputType: "text", Required: true, Secret: false},
			{Key: "appSecret", Label: "App Secret", InputType: "password", Required: true, Secret: true},
		},
		Enabled: true,
	},
	{
		Provider:    "linear",
		DisplayName: "Linear",
		AuthTypes:   []string{ConnectionAuthAPIKey},
		Fields: []connectorProviderField{
			{Key: "apiKey", Label: "API key", InputType: "password", Required: true, Secret: true},
		},
		Enabled: true,
	},
	{
		Provider:    "custom-mcp",
		DisplayName: "Custom MCP Server",
		AuthTypes:   []string{ConnectionAuthCustomCredential, ConnectionAuthNoAuth},
		Fields: []connectorProviderField{
			{Key: "serverUrl", Label: "Server URL", InputType: "text", Required: true, Secret: false},
			{Key: "token", Label: "Token", InputType: "password", Required: false, Secret: true},
		},
		Enabled: true,
	},
}

func (s *Server) handleConnectorProviders(w http.ResponseWriter, _ *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]any{"providers": connectorProviders})
}

func (s *Server) handleConnectorProvider(w http.ResponseWriter, r *http.Request) {
	providerID := strings.TrimSpace(r.PathValue("provider"))
	provider, ok := findConnectorProvider(providerID)
	if !ok {
		s.jsonError(w, http.StatusNotFound, "provider not found")
		return
	}
	_ = json.NewEncoder(w).Encode(provider)
}

func findConnectorProvider(providerID string) (connectorProvider, bool) {
	for _, provider := range connectorProviders {
		if provider.Provider == providerID && provider.Enabled {
			return provider, true
		}
	}
	return connectorProvider{}, false
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

func (s *Server) handleCreateConnection(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := s.connectionWorkspace(w, r)
	if !ok {
		return
	}
	cur := s.currentUser(r)
	if cur == nil || cur.Username == "" || cur.Username == "apikey" {
		s.jsonError(w, http.StatusForbidden, "authenticated user required")
		return
	}
	var body createConnectionRequest
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	body.Provider = strings.TrimSpace(body.Provider)
	provider, exists := findConnectorProvider(body.Provider)
	if !exists {
		s.jsonError(w, http.StatusBadRequest, "unsupported provider")
		return
	}
	authType := strings.TrimSpace(body.AuthType)
	if authType == "" {
		authType = provider.AuthTypes[0]
	}
	if !providerSupportsAuth(provider, authType) {
		s.jsonError(w, http.StatusBadRequest, "unsupported auth type")
		return
	}
	if err := validateConnectionValues(provider, authType, body.Values); err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
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
	if ownerType == ConnectionOwnerWorkspace && !s.canAdminCurrentWorkspace(r) {
		s.jsonError(w, http.StatusForbidden, "workspace admin access required")
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
			"profile":        profile,
		},
		Request: r,
	})
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(connectionToResponse(connection, nil))
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

func (s *Server) handleDeleteConnection(w http.ResponseWriter, r *http.Request) {
	connection, ok := s.connectionByIDWithAccess(w, r)
	if !ok {
		return
	}
	if !s.canManageConnection(r, connection, s.currentUser(r)) {
		s.jsonError(w, http.StatusForbidden, "connection management access required")
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
		s.jsonError(w, http.StatusForbidden, "connection management access required")
		return
	}
	var body createConnectionGrantRequest
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	body.TargetType = strings.TrimSpace(body.TargetType)
	body.TargetID = strings.TrimSpace(body.TargetID)
	if err := s.validateConnectionGrantTarget(r, connection, body.TargetType, body.TargetID); err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
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
		s.jsonError(w, http.StatusForbidden, "connection management access required")
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
		s.jsonError(w, http.StatusNotFound, "grant not found")
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
	if _, ok := s.connectionWorkspace(w, r); !ok {
		return controldb.Connection{}, false
	}
	id := strings.TrimSpace(r.PathValue("id"))
	connection, ok, err := s.controlDB.ConnectionByID(id)
	if err != nil {
		s.serverError(w, err)
		return controldb.Connection{}, false
	}
	if !ok {
		s.jsonError(w, http.StatusNotFound, "connection not found")
		return controldb.Connection{}, false
	}
	if !s.canReadConnection(r, connection, s.currentUser(r)) {
		s.jsonError(w, http.StatusForbidden, "connection access required")
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
	if s.canAdminWorkspace(r, connection.WorkspaceID) {
		return true
	}
	if cur == nil || cur.Username == "" {
		return false
	}
	return connection.OwnerType == ConnectionOwnerUser && connection.OwnerID == cur.Username
}

func (s *Server) validateConnectionGrantTarget(r *http.Request, connection controldb.Connection, targetType, targetID string) error {
	if targetType == "" || targetID == "" {
		return fmt.Errorf("targetType and targetId are required")
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
	member, ok, err := s.controlDB.WorkspaceMember(workspaceID, cur.Username)
	if err != nil || !ok {
		return false
	}
	return member.Role == WorkspaceRoleOwner || member.Role == WorkspaceRoleAdmin
}

func providerSupportsAuth(provider connectorProvider, authType string) bool {
	for _, typ := range provider.AuthTypes {
		if typ == authType {
			return true
		}
	}
	return false
}

func validateConnectionValues(provider connectorProvider, authType string, values map[string]string) error {
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
	ID             string                 `json:"id"`
	Provider       string                 `json:"provider"`
	ConnectionName string                 `json:"connectionName"`
	OwnerType      string                 `json:"ownerType"`
	OwnerID        string                 `json:"ownerId"`
	AuthType       string                 `json:"authType"`
	Status         string                 `json:"status"`
	Profile        map[string]any         `json:"profile"`
	Grants         []connectionGrantModel `json:"grants,omitempty"`
	CreatedBy      string                 `json:"createdBy,omitempty"`
	CreatedAt      string                 `json:"createdAt"`
	UpdatedAt      string                 `json:"updatedAt,omitempty"`
	LastUsedAt     string                 `json:"lastUsedAt,omitempty"`
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
		Profile:        profile,
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
	return map[string]any{
		"provider":       connection.Provider,
		"connectionName": connection.ConnectionName,
		"ownerType":      connection.OwnerType,
		"ownerId":        connection.OwnerID,
		"authType":       connection.AuthType,
		"status":         connection.Status,
	}
}

func newConnectionID(prefix string) string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(b[:])
}
