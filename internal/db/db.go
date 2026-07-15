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

	UpsertRecord(table string, workspaceID string, key []string, payload string) error
	GetRecord(table string, workspaceID string, key []string) (string, bool, error)
	ListRecords(table string, workspaceID string, keyPrefix []string) ([]Record, error)
	DeleteRecord(table string, workspaceID string, key []string) error

	CreateInvitation(inv Invitation) error
	InvitationByToken(token string) (Invitation, bool, error)
	UpdateInvitation(inv Invitation) error
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
	return db, nil
}

func (db *SQLiteStore) Close() error {
	if db == nil || db.sql == nil {
		return nil
	}
	return db.sql.Close()
}

func defaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".multigent", "multigent.db"), nil
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}
