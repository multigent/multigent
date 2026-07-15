package db

import (
	"database/sql"
	"errors"
	"strings"
)

func (db *SQLiteStore) UpsertConnection(c Connection) error {
	if c.CreatedAt == "" {
		c.CreatedAt = nowUTC()
	}
	if c.UpdatedAt == "" {
		c.UpdatedAt = nowUTC()
	}
	if c.ConnectionName == "" {
		c.ConnectionName = "default"
	}
	if c.Status == "" {
		c.Status = "active"
	}
	if c.ProfileJSON == "" {
		c.ProfileJSON = "{}"
	}
	_, err := db.sql.Exec(`INSERT INTO connections (
	id, workspace_id, provider, connection_name, owner_type, owner_id, auth_type, status,
	profile_json, created_by, created_at, updated_at, last_used_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(workspace_id, provider, owner_type, owner_id, connection_name) DO UPDATE SET
	auth_type = excluded.auth_type,
	status = excluded.status,
	profile_json = excluded.profile_json,
	updated_at = excluded.updated_at`,
		c.ID, c.WorkspaceID, c.Provider, c.ConnectionName, c.OwnerType, c.OwnerID,
		c.AuthType, c.Status, c.ProfileJSON, c.CreatedBy, c.CreatedAt, c.UpdatedAt, c.LastUsedAt)
	return err
}

func (db *SQLiteStore) ConnectionByID(id string) (Connection, bool, error) {
	row := db.sql.QueryRow(`SELECT id, workspace_id, provider, connection_name, owner_type, owner_id,
auth_type, status, profile_json, created_by, created_at, updated_at, last_used_at
FROM connections WHERE id = ?`, id)
	c, err := scanConnection(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Connection{}, false, nil
	}
	if err != nil {
		return Connection{}, false, err
	}
	return c, true, nil
}

func (db *SQLiteStore) UpdateConnection(c Connection) error {
	if c.UpdatedAt == "" {
		c.UpdatedAt = nowUTC()
	}
	if c.ConnectionName == "" {
		c.ConnectionName = "default"
	}
	if c.Status == "" {
		c.Status = "active"
	}
	if c.ProfileJSON == "" {
		c.ProfileJSON = "{}"
	}
	res, err := db.sql.Exec(`UPDATE connections SET
	provider = ?,
	connection_name = ?,
	owner_type = ?,
	owner_id = ?,
	auth_type = ?,
	status = ?,
	profile_json = ?,
	updated_at = ?,
	last_used_at = ?
WHERE id = ? AND workspace_id = ?`,
		c.Provider, c.ConnectionName, c.OwnerType, c.OwnerID, c.AuthType, c.Status,
		c.ProfileJSON, c.UpdatedAt, c.LastUsedAt, c.ID, c.WorkspaceID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return errors.New("connection not found")
	}
	return nil
}

func (db *SQLiteStore) ListConnections(filter ConnectionFilter) ([]Connection, error) {
	query := `SELECT id, workspace_id, provider, connection_name, owner_type, owner_id,
auth_type, status, profile_json, created_by, created_at, updated_at, last_used_at
FROM connections WHERE 1=1`
	args := make([]any, 0, 5)
	if strings.TrimSpace(filter.WorkspaceID) != "" {
		query += ` AND workspace_id = ?`
		args = append(args, strings.TrimSpace(filter.WorkspaceID))
	}
	if strings.TrimSpace(filter.Provider) != "" {
		query += ` AND provider = ?`
		args = append(args, strings.TrimSpace(filter.Provider))
	}
	if strings.TrimSpace(filter.OwnerType) != "" {
		query += ` AND owner_type = ?`
		args = append(args, strings.TrimSpace(filter.OwnerType))
	}
	if strings.TrimSpace(filter.OwnerID) != "" {
		query += ` AND owner_id = ?`
		args = append(args, strings.TrimSpace(filter.OwnerID))
	}
	if strings.TrimSpace(filter.Status) != "" {
		query += ` AND status = ?`
		args = append(args, strings.TrimSpace(filter.Status))
	}
	query += ` ORDER BY updated_at DESC, created_at DESC, provider ASC, connection_name ASC`
	rows, err := db.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Connection, 0)
	for rows.Next() {
		c, err := scanConnection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (db *SQLiteStore) DeleteConnection(id string) error {
	_, err := db.sql.Exec(`DELETE FROM connections WHERE id = ?`, id)
	return err
}

func (db *SQLiteStore) UpsertConnectionSecret(secret ConnectionSecret) error {
	if secret.UpdatedAt == "" {
		secret.UpdatedAt = nowUTC()
	}
	_, err := db.sql.Exec(`INSERT INTO connection_secrets (connection_id, ciphertext, nonce, key_version, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(connection_id) DO UPDATE SET
	ciphertext = excluded.ciphertext,
	nonce = excluded.nonce,
	key_version = excluded.key_version,
	updated_at = excluded.updated_at`,
		secret.ConnectionID, secret.Ciphertext, secret.Nonce, secret.KeyVersion, secret.UpdatedAt)
	return err
}

func (db *SQLiteStore) ConnectionSecret(connectionID string) (ConnectionSecret, bool, error) {
	var s ConnectionSecret
	err := db.sql.QueryRow(`SELECT connection_id, ciphertext, nonce, key_version, updated_at
FROM connection_secrets WHERE connection_id = ?`, connectionID).
		Scan(&s.ConnectionID, &s.Ciphertext, &s.Nonce, &s.KeyVersion, &s.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ConnectionSecret{}, false, nil
	}
	if err != nil {
		return ConnectionSecret{}, false, err
	}
	return s, true, nil
}

func (db *SQLiteStore) CreateConnectionGrant(grant ConnectionGrant) error {
	if grant.CreatedAt == "" {
		grant.CreatedAt = nowUTC()
	}
	_, err := db.sql.Exec(`INSERT INTO connection_grants (
	id, workspace_id, connection_id, target_type, target_id, created_by, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(connection_id, target_type, target_id) DO UPDATE SET created_by = excluded.created_by`,
		grant.ID, grant.WorkspaceID, grant.ConnectionID, grant.TargetType, grant.TargetID, grant.CreatedBy, grant.CreatedAt)
	return err
}

func (db *SQLiteStore) DeleteConnectionGrant(id string) error {
	_, err := db.sql.Exec(`DELETE FROM connection_grants WHERE id = ?`, id)
	return err
}

func (db *SQLiteStore) ListConnectionGrants(connectionID string) ([]ConnectionGrant, error) {
	rows, err := db.sql.Query(`SELECT id, workspace_id, connection_id, target_type, target_id, created_by, created_at
FROM connection_grants WHERE connection_id = ? ORDER BY created_at ASC`, connectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ConnectionGrant, 0)
	for rows.Next() {
		var g ConnectionGrant
		if err := rows.Scan(&g.ID, &g.WorkspaceID, &g.ConnectionID, &g.TargetType, &g.TargetID, &g.CreatedBy, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

type connectionScanner interface {
	Scan(dest ...any) error
}

func scanConnection(row connectionScanner) (Connection, error) {
	var c Connection
	err := row.Scan(&c.ID, &c.WorkspaceID, &c.Provider, &c.ConnectionName, &c.OwnerType, &c.OwnerID,
		&c.AuthType, &c.Status, &c.ProfileJSON, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt, &c.LastUsedAt)
	return c, err
}
