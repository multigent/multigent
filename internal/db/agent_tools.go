package db

import (
	"database/sql"
	"errors"
	"strings"
)

func (db *SQLiteStore) UpsertAgentToolBinding(b AgentToolBinding) error {
	if b.CreatedAt == "" {
		b.CreatedAt = nowUTC()
	}
	if b.UpdatedAt == "" {
		b.UpdatedAt = nowUTC()
	}
	if b.Status == "" {
		b.Status = "enabled"
	}
	if b.ConfigJSON == "" {
		b.ConfigJSON = "{}"
	}
	_, err := db.sql.Exec(`INSERT INTO agent_tool_bindings (
	id, workspace_id, project_id, agent_id, connection_id, provider, adapter_type,
	status, config_json, created_by, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(workspace_id, project_id, agent_id, connection_id) DO UPDATE SET
	provider = excluded.provider,
	adapter_type = excluded.adapter_type,
	status = excluded.status,
	config_json = excluded.config_json,
	updated_at = excluded.updated_at`,
		b.ID, b.WorkspaceID, b.ProjectID, b.AgentID, b.ConnectionID, b.Provider, b.AdapterType,
		b.Status, b.ConfigJSON, b.CreatedBy, b.CreatedAt, b.UpdatedAt)
	return err
}

func (db *SQLiteStore) AgentToolBindingByID(id string) (AgentToolBinding, bool, error) {
	row := db.sql.QueryRow(`SELECT id, workspace_id, project_id, agent_id, connection_id, provider,
adapter_type, status, config_json, created_by, created_at, updated_at
FROM agent_tool_bindings WHERE id = ?`, id)
	b, err := scanAgentToolBinding(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentToolBinding{}, false, nil
	}
	if err != nil {
		return AgentToolBinding{}, false, err
	}
	return b, true, nil
}

func (db *SQLiteStore) ListAgentToolBindings(filter AgentToolBindingFilter) ([]AgentToolBinding, error) {
	query := `SELECT id, workspace_id, project_id, agent_id, connection_id, provider,
adapter_type, status, config_json, created_by, created_at, updated_at
FROM agent_tool_bindings WHERE 1=1`
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
	if strings.TrimSpace(filter.ConnectionID) != "" {
		query += ` AND connection_id = ?`
		args = append(args, strings.TrimSpace(filter.ConnectionID))
	}
	if strings.TrimSpace(filter.Provider) != "" {
		query += ` AND provider = ?`
		args = append(args, strings.TrimSpace(filter.Provider))
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
	out := make([]AgentToolBinding, 0)
	for rows.Next() {
		b, err := scanAgentToolBinding(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (db *SQLiteStore) DeleteAgentToolBinding(id string) error {
	_, err := db.sql.Exec(`DELETE FROM agent_tool_bindings WHERE id = ?`, id)
	return err
}

type agentToolBindingScanner interface {
	Scan(dest ...any) error
}

func scanAgentToolBinding(row agentToolBindingScanner) (AgentToolBinding, error) {
	var b AgentToolBinding
	err := row.Scan(&b.ID, &b.WorkspaceID, &b.ProjectID, &b.AgentID, &b.ConnectionID, &b.Provider,
		&b.AdapterType, &b.Status, &b.ConfigJSON, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt)
	return b, err
}
