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
	if strings.TrimSpace(b.AgentWorkerID) != "" {
		res, err := db.sql.Exec(`UPDATE agent_tool_bindings SET
	project_id = '',
	agent_id = '',
	provider = ?,
	adapter_type = ?,
	status = ?,
	config_json = ?,
	updated_at = ?
WHERE workspace_id = ? AND agent_worker_id = ? AND connection_id = ?`,
			b.Provider, b.AdapterType, b.Status, b.ConfigJSON, b.UpdatedAt,
			b.WorkspaceID, b.AgentWorkerID, b.ConnectionID)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err == nil && n > 0 {
			return nil
		}
		// The one-way migration may have already loaded this row under its
		// project identity. Reuse it instead of inserting a second row.
		if strings.TrimSpace(b.ProjectID) != "" && strings.TrimSpace(b.AgentID) != "" {
			res, err = db.sql.Exec(`UPDATE agent_tool_bindings SET
			agent_worker_id = ?,
			project_id = '',
			agent_id = '',
			provider = ?,
			adapter_type = ?,
			status = ?,
			config_json = ?,
			updated_at = ?
		WHERE workspace_id = ? AND project_id = ? AND agent_id = ? AND connection_id = ? AND agent_worker_id = ''`,
				b.AgentWorkerID, b.Provider, b.AdapterType, b.Status, b.ConfigJSON, b.UpdatedAt,
				b.WorkspaceID, b.ProjectID, b.AgentID, b.ConnectionID)
			if err != nil {
				return err
			}
			if n, err := res.RowsAffected(); err == nil && n > 0 {
				return nil
			}
		}
		_, err = db.sql.Exec(`INSERT INTO agent_tool_bindings (
		id, workspace_id, agent_worker_id, project_id, agent_id, connection_id, provider, adapter_type,
		status, config_json, created_by, created_at, updated_at
	) VALUES (?, ?, ?, '', '', ?, ?, ?, ?, ?, ?, ?, ?)
	`,
			b.ID, b.WorkspaceID, b.AgentWorkerID, b.ConnectionID, b.Provider, b.AdapterType,
			b.Status, b.ConfigJSON, b.CreatedBy, b.CreatedAt, b.UpdatedAt)
		return err
	}
	// This branch is only used by the one-way 1.x migration. Runtime/API
	// callers must provide AgentWorkerID; project and agent fields are not a
	// supported binding identity anymore.
	if strings.TrimSpace(b.ProjectID) == "" || strings.TrimSpace(b.AgentID) == "" {
		return errors.New("agent worker is required for tool binding")
	}
	res, err := db.sql.Exec(`UPDATE agent_tool_bindings SET
	provider = ?,
	adapter_type = ?,
	status = ?,
	config_json = ?,
	updated_at = ?
WHERE workspace_id = ? AND project_id = ? AND agent_id = ? AND connection_id = ? AND agent_worker_id = ''`,
		b.Provider, b.AdapterType, b.Status, b.ConfigJSON, b.UpdatedAt,
		b.WorkspaceID, b.ProjectID, b.AgentID, b.ConnectionID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n > 0 {
		return nil
	}
	_, err = db.sql.Exec(`INSERT INTO agent_tool_bindings (
	id, workspace_id, agent_worker_id, project_id, agent_id, connection_id, provider, adapter_type,
	status, config_json, created_by, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		b.ID, b.WorkspaceID, b.AgentWorkerID, b.ProjectID, b.AgentID, b.ConnectionID, b.Provider, b.AdapterType,
		b.Status, b.ConfigJSON, b.CreatedBy, b.CreatedAt, b.UpdatedAt)
	return err
}

func (db *SQLiteStore) AgentToolBindingByID(id string) (AgentToolBinding, bool, error) {
	row := db.sql.QueryRow(`SELECT id, workspace_id, agent_worker_id, project_id, agent_id, connection_id, provider,
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
	query := `SELECT id, workspace_id, agent_worker_id, project_id, agent_id, connection_id, provider,
adapter_type, status, config_json, created_by, created_at, updated_at
FROM agent_tool_bindings WHERE 1=1`
	args := make([]any, 0, 6)
	if strings.TrimSpace(filter.WorkspaceID) != "" {
		query += ` AND workspace_id = ?`
		args = append(args, strings.TrimSpace(filter.WorkspaceID))
	}
	if strings.TrimSpace(filter.AgentWorkerID) != "" {
		query += ` AND agent_worker_id = ?`
		args = append(args, strings.TrimSpace(filter.AgentWorkerID))
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
	err := row.Scan(&b.ID, &b.WorkspaceID, &b.AgentWorkerID, &b.ProjectID, &b.AgentID, &b.ConnectionID, &b.Provider,
		&b.AdapterType, &b.Status, &b.ConfigJSON, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt)
	return b, err
}
