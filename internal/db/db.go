// Package db defines the control-plane storage interface and the default SQLite implementation.
package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store interface {
	Close() error

	UpsertWorkspace(w Workspace) error
	ListWorkspaces() ([]Workspace, error)
	WorkspaceByID(id string) (Workspace, bool, error)
	MarkWorkspaceOpened(id string) error

	GetSetting(key string) (string, bool, error)
	SetSetting(key, value string) error

	UpsertUser(u User) error
	ListUsers() ([]User, error)
	UserByUsername(username string) (User, bool, error)
	UserByLogin(login string) (User, bool, error)
	DeleteUser(username string) error

	UpsertWorkspaceMember(workspaceID, username, role string) error
	WorkspaceMember(workspaceID, username string) (WorkspaceMember, bool, error)
	ListWorkspaceMembers(workspaceID string) ([]WorkspaceMember, error)
	ListWorkspaceMembersForUser(username string) ([]WorkspaceMember, error)

	UpsertRecord(table string, workspaceID string, key []string, payload string) error
	GetRecord(table string, workspaceID string, key []string) (string, bool, error)
	ListRecords(table string, workspaceID string, keyPrefix []string) ([]Record, error)
	DeleteRecord(table string, workspaceID string, key []string) error

	CreateInvitation(inv Invitation) error
	InvitationByToken(token string) (Invitation, bool, error)
	ListInvitations() ([]Invitation, error)
	UpdateInvitation(inv Invitation) error

	CreateAuditEvent(event AuditEvent) error
	ListAuditEvents(filter AuditEventFilter) ([]AuditEvent, error)

	UpsertConnectorProvider(provider ConnectorProvider) error
	ConnectorProviderByID(provider string) (ConnectorProvider, bool, error)
	ListConnectorProviders(includeDisabled bool) ([]ConnectorProvider, error)
	DeleteConnectorProvider(provider string) error
	UpsertOAuthClientConfig(config OAuthClientConfig) error
	OAuthClientConfigByProvider(workspaceID, provider string) (OAuthClientConfig, bool, error)
	ListOAuthClientConfigs(workspaceID string) ([]OAuthClientConfig, error)
	DeleteOAuthClientConfig(workspaceID, provider string) error

	UpsertModelProvider(workspaceID string, provider ModelProvider) error
	ModelProviderByID(workspaceID, id string) (ModelProvider, bool, error)
	ListModelProviders(workspaceID string) ([]ModelProvider, error)
	DeleteModelProvider(workspaceID, id string) error

	UpsertConnection(connection Connection) error
	UpdateConnection(connection Connection) error
	ConnectionByID(id string) (Connection, bool, error)
	ListConnections(filter ConnectionFilter) ([]Connection, error)
	DeleteConnection(id string) error
	UpsertConnectionSecret(secret ConnectionSecret) error
	ConnectionSecret(connectionID string) (ConnectionSecret, bool, error)
	CreateConnectionGrant(grant ConnectionGrant) error
	DeleteConnectionGrant(id string) error
	ListConnectionGrants(connectionID string) ([]ConnectionGrant, error)
	UpsertAgentToolBinding(binding AgentToolBinding) error
	AgentToolBindingByID(id string) (AgentToolBinding, bool, error)
	ListAgentToolBindings(filter AgentToolBindingFilter) ([]AgentToolBinding, error)
	DeleteAgentToolBinding(id string) error

	UpsertAgentChannelBinding(binding AgentChannelBinding) error
	AgentChannelBindingByID(id string) (AgentChannelBinding, bool, error)
	ListAgentChannelBindings(filter AgentChannelBindingFilter) ([]AgentChannelBinding, error)
	DeleteAgentChannelBinding(id string) error

	UpsertExternalIdentity(identity ExternalIdentity) error
	ExternalIdentityByExternalID(workspaceID, provider, externalUserID string) (ExternalIdentity, bool, error)
	ListExternalIdentities(filter ExternalIdentityFilter) ([]ExternalIdentity, error)

	CreateInteractionSession(session InteractionSession) error
	UpdateInteractionSession(session InteractionSession) error
	ActiveInteractionSession(workspaceID, projectID, agentID string) (InteractionSession, bool, error)
	InteractionSessionByID(id string) (InteractionSession, bool, error)
	ListInteractionSessions(filter InteractionSessionFilter) ([]InteractionSession, error)
	CreateInteractionEvent(event InteractionEvent) error
	ListInteractionEvents(filter InteractionEventFilter) ([]InteractionEvent, error)
}

type SQLiteStore struct {
	sql *sql.DB
}

type Workspace struct {
	ID           string
	Name         string
	Slug         string
	Description  string
	Root         string
	CreatedBy    string
	CreatedAt    string
	UpdatedAt    string
	LastOpenedAt string
}

type WorkspaceMember struct {
	WorkspaceID string
	Username    string
	Role        string
	CreatedAt   string
}

type User struct {
	Username     string
	Email        string
	DisplayName  string
	Role         string
	Avatar       string
	Phone        string
	Bio          string
	PasswordHash string
	Disabled     bool
	CreatedAt    string
	ProjectsJSON string
	LinkedJSON   string
}

type Invitation struct {
	Token        string
	WorkspaceID  string
	Email        string
	Role         string
	DisplayName  string
	ProjectsJSON string
	LinkedJSON   string
	InvitedBy    string
	Status       string
	CreatedAt    string
	ExpiresAt    string
	AcceptedAt   string
}

type Record struct {
	Key     []string
	Payload string
}

type AuditEvent struct {
	ID           string
	WorkspaceID  string
	ActorType    string
	ActorID      string
	Action       string
	ResourceType string
	ResourceID   string
	Summary      string
	BeforeJSON   string
	AfterJSON    string
	IP           string
	UserAgent    string
	CreatedAt    string
}

type AuditEventFilter struct {
	WorkspaceID  string
	ActorID      string
	Action       string
	ResourceType string
	ResourceID   string
	Limit        int
}

type Connection struct {
	ID             string
	WorkspaceID    string
	Provider       string
	ConnectionName string
	OwnerType      string
	OwnerID        string
	AuthType       string
	Status         string
	ProfileJSON    string
	CreatedBy      string
	CreatedAt      string
	UpdatedAt      string
	LastUsedAt     string
}

type ConnectorProvider struct {
	Provider      string
	DisplayName   string
	AuthTypesJSON string
	CatalogJSON   string
	Enabled       bool
	CreatedAt     string
	UpdatedAt     string
}

type OAuthClientConfig struct {
	WorkspaceID      string
	Provider         string
	ClientID         string
	SecretCiphertext string
	Nonce            string
	KeyVersion       string
	ExtraJSON        string
	CreatedBy        string
	CreatedAt        string
	UpdatedAt        string
}

type ModelProvider struct {
	ID          string
	WorkspaceID string
	OwnerType   string
	OwnerID     string
	Name        string
	Type        string
	BaseURL     string
	APIKey      string
	Model       string
	EnvJSON     string
	CreatedAt   string
	UpdatedAt   string
}

type ConnectionFilter struct {
	WorkspaceID string
	Provider    string
	OwnerType   string
	OwnerID     string
	Status      string
}

type ConnectionSecret struct {
	ConnectionID string
	Ciphertext   string
	Nonce        string
	KeyVersion   string
	UpdatedAt    string
}

type ConnectionGrant struct {
	ID           string
	WorkspaceID  string
	ConnectionID string
	TargetType   string
	TargetID     string
	CreatedBy    string
	CreatedAt    string
}

type AgentToolBinding struct {
	ID           string
	WorkspaceID  string
	ProjectID    string
	AgentID      string
	ConnectionID string
	Provider     string
	AdapterType  string
	Status       string
	ConfigJSON   string
	CreatedBy    string
	CreatedAt    string
	UpdatedAt    string
}

type AgentToolBindingFilter struct {
	WorkspaceID  string
	ProjectID    string
	AgentID      string
	ConnectionID string
	Provider     string
	Status       string
}

type AgentChannelBinding struct {
	ID              string
	WorkspaceID     string
	ProjectID       string
	AgentID         string
	Provider        string
	ConnectionID    string
	ExternalBotID   string
	ExternalChatID  string
	ExternalOwnerID string
	Status          string
	MetadataJSON    string
	CreatedBy       string
	CreatedAt       string
	UpdatedAt       string
	LastActivityAt  string
}

type AgentChannelBindingFilter struct {
	WorkspaceID  string
	ProjectID    string
	AgentID      string
	Provider     string
	ConnectionID string
	Status       string
}

type ExternalIdentity struct {
	ID             string
	WorkspaceID    string
	Provider       string
	ExternalUserID string
	UserID         string
	MetadataJSON   string
	CreatedBy      string
	CreatedAt      string
	UpdatedAt      string
}

type ExternalIdentityFilter struct {
	WorkspaceID    string
	Provider       string
	ExternalUserID string
	UserID         string
}

type InteractionSession struct {
	ID               string
	WorkspaceID      string
	ProjectID        string
	AgentID          string
	SourceKind       string
	SourceChannel    string
	ActorType        string
	ActorID          string
	Status           string
	LockReason       string
	RuntimeSessionID string
	CurrentRunID     string
	HumanIntervened  bool
	MetadataJSON     string
	CreatedAt        string
	UpdatedAt        string
	LastActivityAt   string
	CompletedAt      string
}

type InteractionSessionFilter struct {
	WorkspaceID string
	ProjectID   string
	AgentID     string
	Status      string
	Limit       int
}

type InteractionEvent struct {
	ID           string
	SessionID    string
	WorkspaceID  string
	ActorType    string
	ActorID      string
	Channel      string
	EventType    string
	Content      string
	MetadataJSON string
	CreatedAt    string
}

type InteractionEventFilter struct {
	WorkspaceID string
	SessionID   string
	Limit       int
}

func OpenDefault() (*SQLiteStore, error) {
	path, err := defaultPath()
	if err != nil {
		return nil, err
	}
	return Open(path)
}

func Open(path string) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	uri := "file:" + filepath.ToSlash(path) + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)"
	sqlDB, err := sql.Open("sqlite", uri)
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	db := &SQLiteStore{sql: sqlDB}
	if err := db.migrate(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	if err := db.SeedDefaultConnectorProviders(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return db, nil
}

func (db *SQLiteStore) Close() error {
	if db == nil || db.sql == nil {
		return nil
	}
	return db.sql.Close()
}

func defaultPath() (string, error) {
	if dataDir := os.Getenv("MULTIGENT_DATA_DIR"); dataDir != "" {
		return filepath.Join(dataDir, ".multigent", "multigent.db"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".multigent", "multigent.db"), nil
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}
