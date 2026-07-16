package db

import (
	"database/sql"
	"errors"
	"strings"
)

func (db *SQLiteStore) UpsertAgentChannelBinding(b AgentChannelBinding) error {
	if b.CreatedAt == "" {
		b.CreatedAt = nowUTC()
	}
	if b.UpdatedAt == "" {
		b.UpdatedAt = nowUTC()
	}
	if b.Status == "" {
		b.Status = "connected"
	}
	if b.MetadataJSON == "" {
		b.MetadataJSON = "{}"
	}
	_, err := db.sql.Exec(`INSERT INTO agent_channel_bindings (
	id, workspace_id, project_id, agent_id, provider, connection_id,
	external_bot_id, external_chat_id, external_owner_id, status, metadata_json,
	created_by, created_at, updated_at, last_activity_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(workspace_id, project_id, agent_id, provider) DO UPDATE SET
	connection_id = excluded.connection_id,
	external_bot_id = excluded.external_bot_id,
	external_chat_id = excluded.external_chat_id,
	external_owner_id = excluded.external_owner_id,
	status = excluded.status,
	metadata_json = excluded.metadata_json,
	updated_at = excluded.updated_at,
	last_activity_at = excluded.last_activity_at`,
		b.ID, b.WorkspaceID, b.ProjectID, b.AgentID, b.Provider, b.ConnectionID,
		b.ExternalBotID, b.ExternalChatID, b.ExternalOwnerID, b.Status, b.MetadataJSON,
		b.CreatedBy, b.CreatedAt, b.UpdatedAt, b.LastActivityAt)
	return err
}

func (db *SQLiteStore) AgentChannelBindingByID(id string) (AgentChannelBinding, bool, error) {
	row := db.sql.QueryRow(`SELECT id, workspace_id, project_id, agent_id, provider, connection_id,
external_bot_id, external_chat_id, external_owner_id, status, metadata_json,
created_by, created_at, updated_at, last_activity_at
FROM agent_channel_bindings WHERE id = ?`, id)
	b, err := scanAgentChannelBinding(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentChannelBinding{}, false, nil
	}
	if err != nil {
		return AgentChannelBinding{}, false, err
	}
	return b, true, nil
}

func (db *SQLiteStore) ListAgentChannelBindings(filter AgentChannelBindingFilter) ([]AgentChannelBinding, error) {
	query := `SELECT id, workspace_id, project_id, agent_id, provider, connection_id,
external_bot_id, external_chat_id, external_owner_id, status, metadata_json,
created_by, created_at, updated_at, last_activity_at
FROM agent_channel_bindings WHERE 1=1`
	args := make([]any, 0, 6)
	if strings.TrimSpace(filter.WorkspaceID) != "" {
		query += ` AND workspace_id = ?`
		args = append(args, strings.TrimSpace(filter.WorkspaceID))
	}
	if strings.TrimSpace(filter.ProjectID) != "" {
		query += ` AND project_id = ?`
		args = append(args, strings.TrimSpace(filter.ProjectID))
	}
	if strings.TrimSpace(filter.AgentID) != "" {
		query += ` AND agent_id = ?`
		args = append(args, strings.TrimSpace(filter.AgentID))
	}
	if strings.TrimSpace(filter.Provider) != "" {
		query += ` AND provider = ?`
		args = append(args, strings.TrimSpace(filter.Provider))
	}
	if strings.TrimSpace(filter.ConnectionID) != "" {
		query += ` AND connection_id = ?`
		args = append(args, strings.TrimSpace(filter.ConnectionID))
	}
	if strings.TrimSpace(filter.Status) != "" {
		query += ` AND status = ?`
		args = append(args, strings.TrimSpace(filter.Status))
	}
	query += ` ORDER BY updated_at DESC, created_at DESC, provider ASC`
	rows, err := db.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AgentChannelBinding, 0)
	for rows.Next() {
		b, err := scanAgentChannelBinding(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (db *SQLiteStore) DeleteAgentChannelBinding(id string) error {
	_, err := db.sql.Exec(`DELETE FROM agent_channel_bindings WHERE id = ?`, id)
	return err
}

type agentChannelBindingScanner interface {
	Scan(dest ...any) error
}

func scanAgentChannelBinding(row agentChannelBindingScanner) (AgentChannelBinding, error) {
	var b AgentChannelBinding
	err := row.Scan(&b.ID, &b.WorkspaceID, &b.ProjectID, &b.AgentID, &b.Provider, &b.ConnectionID,
		&b.ExternalBotID, &b.ExternalChatID, &b.ExternalOwnerID, &b.Status, &b.MetadataJSON,
		&b.CreatedBy, &b.CreatedAt, &b.UpdatedAt, &b.LastActivityAt)
	return b, err
}
