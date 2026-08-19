package db

import (
	"database/sql"
	"errors"
	"strings"
)

func (db *SQLiteStore) UpsertUserChannelIdentity(identity UserChannelIdentity) error {
	if identity.CreatedAt == "" {
		identity.CreatedAt = nowUTC()
	}
	if identity.UpdatedAt == "" {
		identity.UpdatedAt = nowUTC()
	}
	if identity.MetadataJSON == "" {
		identity.MetadataJSON = "{}"
	}
	_, err := db.sql.Exec(`INSERT INTO user_channel_identities (
	id, workspace_id, user_id, channel_binding_id, provider, external_user_id,
	external_chat_id, metadata_json, created_by, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(workspace_id, channel_binding_id, user_id) DO UPDATE SET
	external_user_id = excluded.external_user_id,
	external_chat_id = excluded.external_chat_id,
	metadata_json = excluded.metadata_json,
	updated_at = excluded.updated_at`,
		identity.ID, identity.WorkspaceID, identity.UserID, identity.ChannelBindingID, identity.Provider,
		identity.ExternalUserID, identity.ExternalChatID, identity.MetadataJSON, identity.CreatedBy,
		identity.CreatedAt, identity.UpdatedAt)
	return err
}

func (db *SQLiteStore) ListUserChannelIdentities(filter UserChannelIdentityFilter) ([]UserChannelIdentity, error) {
	query := `SELECT id, workspace_id, user_id, channel_binding_id, provider, external_user_id,
external_chat_id, metadata_json, created_by, created_at, updated_at
FROM user_channel_identities WHERE 1=1`
	args := make([]any, 0, 5)
	if strings.TrimSpace(filter.WorkspaceID) != "" {
		query += ` AND workspace_id = ?`
		args = append(args, strings.TrimSpace(filter.WorkspaceID))
	}
	if strings.TrimSpace(filter.UserID) != "" {
		query += ` AND user_id = ?`
		args = append(args, strings.TrimSpace(filter.UserID))
	}
	if strings.TrimSpace(filter.ChannelBindingID) != "" {
		query += ` AND channel_binding_id = ?`
		args = append(args, strings.TrimSpace(filter.ChannelBindingID))
	}
	if strings.TrimSpace(filter.Provider) != "" {
		query += ` AND provider = ?`
		args = append(args, strings.TrimSpace(filter.Provider))
	}
	if strings.TrimSpace(filter.ExternalUserID) != "" {
		query += ` AND external_user_id = ?`
		args = append(args, strings.TrimSpace(filter.ExternalUserID))
	}
	query += ` ORDER BY updated_at DESC, created_at DESC`
	rows, err := db.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]UserChannelIdentity, 0)
	for rows.Next() {
		identity, err := scanUserChannelIdentity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, identity)
	}
	return out, rows.Err()
}

func scanUserChannelIdentity(row interface{ Scan(dest ...any) error }) (UserChannelIdentity, error) {
	var identity UserChannelIdentity
	err := row.Scan(&identity.ID, &identity.WorkspaceID, &identity.UserID, &identity.ChannelBindingID,
		&identity.Provider, &identity.ExternalUserID, &identity.ExternalChatID, &identity.MetadataJSON,
		&identity.CreatedBy, &identity.CreatedAt, &identity.UpdatedAt)
	return identity, err
}

func (db *SQLiteStore) UpsertAgentChannelTarget(target AgentChannelTarget) error {
	if target.CreatedAt == "" {
		target.CreatedAt = nowUTC()
	}
	if target.UpdatedAt == "" {
		target.UpdatedAt = nowUTC()
	}
	if target.MetadataJSON == "" {
		target.MetadataJSON = "{}"
	}
	if target.TargetType == "" {
		target.TargetType = "chat"
	}
	_, err := db.sql.Exec(`INSERT INTO agent_channel_targets (
id, workspace_id, channel_binding_id, provider, target_type, display_name,
external_user_id, external_chat_id, metadata_json, created_by, created_at, updated_at, last_activity_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(workspace_id, channel_binding_id, target_type, external_chat_id) DO UPDATE SET
	display_name = excluded.display_name,
	external_user_id = excluded.external_user_id,
	metadata_json = excluded.metadata_json,
	updated_at = excluded.updated_at,
	last_activity_at = excluded.last_activity_at`,
		target.ID, target.WorkspaceID, target.ChannelBindingID, target.Provider, target.TargetType,
		target.DisplayName, target.ExternalUserID, target.ExternalChatID, target.MetadataJSON,
		target.CreatedBy, target.CreatedAt, target.UpdatedAt, target.LastActivityAt)
	return err
}

func (db *SQLiteStore) ListAgentChannelTargets(filter AgentChannelTargetFilter) ([]AgentChannelTarget, error) {
	query := `SELECT id, workspace_id, channel_binding_id, provider, target_type, display_name,
external_user_id, external_chat_id, metadata_json, created_by, created_at, updated_at, last_activity_at
FROM agent_channel_targets WHERE 1=1`
	args := make([]any, 0, 6)
	if strings.TrimSpace(filter.WorkspaceID) != "" {
		query += ` AND workspace_id = ?`
		args = append(args, strings.TrimSpace(filter.WorkspaceID))
	}
	if strings.TrimSpace(filter.ChannelBindingID) != "" {
		query += ` AND channel_binding_id = ?`
		args = append(args, strings.TrimSpace(filter.ChannelBindingID))
	}
	if strings.TrimSpace(filter.Provider) != "" {
		query += ` AND provider = ?`
		args = append(args, strings.TrimSpace(filter.Provider))
	}
	if strings.TrimSpace(filter.TargetType) != "" {
		query += ` AND target_type = ?`
		args = append(args, strings.TrimSpace(filter.TargetType))
	}
	if strings.TrimSpace(filter.DisplayName) != "" {
		query += ` AND lower(display_name) = lower(?)`
		args = append(args, strings.TrimSpace(filter.DisplayName))
	}
	if strings.TrimSpace(filter.ExternalChatID) != "" {
		query += ` AND external_chat_id = ?`
		args = append(args, strings.TrimSpace(filter.ExternalChatID))
	}
	query += ` ORDER BY updated_at DESC, created_at DESC`
	rows, err := db.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AgentChannelTarget, 0)
	for rows.Next() {
		target, err := scanAgentChannelTarget(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, target)
	}
	return out, rows.Err()
}

func scanAgentChannelTarget(row interface{ Scan(dest ...any) error }) (AgentChannelTarget, error) {
	var target AgentChannelTarget
	err := row.Scan(&target.ID, &target.WorkspaceID, &target.ChannelBindingID, &target.Provider,
		&target.TargetType, &target.DisplayName, &target.ExternalUserID, &target.ExternalChatID,
		&target.MetadataJSON, &target.CreatedBy, &target.CreatedAt, &target.UpdatedAt, &target.LastActivityAt)
	return target, err
}

func (db *SQLiteStore) CreateAgentChannelBindCode(code AgentChannelBindCode) error {
	if code.CreatedAt == "" {
		code.CreatedAt = nowUTC()
	}
	if code.TargetType == "" {
		code.TargetType = "user"
	}
	_, err := db.sql.Exec(`INSERT INTO agent_channel_bind_codes (
	code, workspace_id, channel_binding_id, user_id, target_type, target_name, expires_at, used_at, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.ToUpper(strings.TrimSpace(code.Code)), code.WorkspaceID, code.ChannelBindingID, code.UserID,
		code.TargetType, code.TargetName, code.ExpiresAt, code.UsedAt, code.CreatedAt)
	return err
}

func (db *SQLiteStore) AgentChannelBindCodeByCode(code string) (AgentChannelBindCode, bool, error) {
	row := db.sql.QueryRow(`SELECT code, workspace_id, channel_binding_id, user_id, target_type, target_name, expires_at, used_at, created_at
FROM agent_channel_bind_codes WHERE code = ?`, strings.ToUpper(strings.TrimSpace(code)))
	item, err := scanAgentChannelBindCode(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentChannelBindCode{}, false, nil
	}
	if err != nil {
		return AgentChannelBindCode{}, false, err
	}
	return item, true, nil
}

func (db *SQLiteStore) MarkAgentChannelBindCodeUsed(code, usedAt string) error {
	_, err := db.sql.Exec(`UPDATE agent_channel_bind_codes SET used_at = ? WHERE code = ?`, usedAt, strings.ToUpper(strings.TrimSpace(code)))
	return err
}

func scanAgentChannelBindCode(row interface{ Scan(dest ...any) error }) (AgentChannelBindCode, error) {
	var code AgentChannelBindCode
	err := row.Scan(&code.Code, &code.WorkspaceID, &code.ChannelBindingID, &code.UserID, &code.TargetType, &code.TargetName, &code.ExpiresAt, &code.UsedAt, &code.CreatedAt)
	return code, err
}
