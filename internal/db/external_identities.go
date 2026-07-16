package db

import (
	"database/sql"
	"errors"
	"strings"
)

func (db *SQLiteStore) UpsertExternalIdentity(identity ExternalIdentity) error {
	if identity.CreatedAt == "" {
		identity.CreatedAt = nowUTC()
	}
	if identity.UpdatedAt == "" {
		identity.UpdatedAt = nowUTC()
	}
	if identity.MetadataJSON == "" {
		identity.MetadataJSON = "{}"
	}
	_, err := db.sql.Exec(`INSERT INTO external_identities (
	id, workspace_id, provider, external_user_id, user_id, metadata_json, created_by, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(workspace_id, provider, external_user_id) DO UPDATE SET
	user_id = excluded.user_id,
	metadata_json = excluded.metadata_json,
	updated_at = excluded.updated_at`,
		identity.ID, identity.WorkspaceID, identity.Provider, identity.ExternalUserID, identity.UserID,
		identity.MetadataJSON, identity.CreatedBy, identity.CreatedAt, identity.UpdatedAt)
	return err
}

func (db *SQLiteStore) ExternalIdentityByExternalID(workspaceID, provider, externalUserID string) (ExternalIdentity, bool, error) {
	row := db.sql.QueryRow(`SELECT id, workspace_id, provider, external_user_id, user_id,
metadata_json, created_by, created_at, updated_at
FROM external_identities WHERE workspace_id = ? AND provider = ? AND external_user_id = ?`,
		workspaceID, provider, externalUserID)
	identity, err := scanExternalIdentity(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ExternalIdentity{}, false, nil
	}
	if err != nil {
		return ExternalIdentity{}, false, err
	}
	return identity, true, nil
}

func (db *SQLiteStore) ListExternalIdentities(filter ExternalIdentityFilter) ([]ExternalIdentity, error) {
	query := `SELECT id, workspace_id, provider, external_user_id, user_id,
metadata_json, created_by, created_at, updated_at
FROM external_identities WHERE 1=1`
	args := make([]any, 0, 4)
	if strings.TrimSpace(filter.WorkspaceID) != "" {
		query += ` AND workspace_id = ?`
		args = append(args, strings.TrimSpace(filter.WorkspaceID))
	}
	if strings.TrimSpace(filter.Provider) != "" {
		query += ` AND provider = ?`
		args = append(args, strings.TrimSpace(filter.Provider))
	}
	if strings.TrimSpace(filter.ExternalUserID) != "" {
		query += ` AND external_user_id = ?`
		args = append(args, strings.TrimSpace(filter.ExternalUserID))
	}
	if strings.TrimSpace(filter.UserID) != "" {
		query += ` AND user_id = ?`
		args = append(args, strings.TrimSpace(filter.UserID))
	}
	query += ` ORDER BY updated_at DESC, created_at DESC`
	rows, err := db.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ExternalIdentity, 0)
	for rows.Next() {
		identity, err := scanExternalIdentity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, identity)
	}
	return out, rows.Err()
}

type externalIdentityScanner interface {
	Scan(dest ...any) error
}

func scanExternalIdentity(row externalIdentityScanner) (ExternalIdentity, error) {
	var identity ExternalIdentity
	err := row.Scan(&identity.ID, &identity.WorkspaceID, &identity.Provider, &identity.ExternalUserID,
		&identity.UserID, &identity.MetadataJSON, &identity.CreatedBy, &identity.CreatedAt, &identity.UpdatedAt)
	return identity, err
}
