package db

import (
	"database/sql"
	"errors"
	"strings"
)

func (db *SQLiteStore) UpsertAgentSession(session AgentSession) error {
	if session.CreatedAt == "" {
		session.CreatedAt = nowUTC()
	}
	if session.UpdatedAt == "" {
		session.UpdatedAt = nowUTC()
	}
	if session.LastActivityAt == "" {
		session.LastActivityAt = session.UpdatedAt
	}
	if session.SessionKind == "" {
		session.SessionKind = "fork"
	}
	if session.Status == "" {
		session.Status = "pending"
	}
	if session.ForkMode == "" {
		session.ForkMode = "fresh_with_context"
	}
	if session.PermissionPolicy == "" {
		session.PermissionPolicy = "inherit"
	}
	if session.CapabilitiesJSON == "" {
		session.CapabilitiesJSON = "{}"
	}
	if session.ResultRefsJSON == "" {
		session.ResultRefsJSON = "{}"
	}
	_, err := db.sql.Exec(`INSERT INTO agent_sessions (
	id, workspace_id, agent_worker_id, session_kind, parent_session_id, project_id, project_membership_id,
	task_id, workflow_instance_id, title, purpose, initial_prompt, status, runtime_provider, runtime_session_id,
	fork_mode, permission_policy, capabilities_json, result_summary, result_refs_json,
	created_by_run_id, last_run_id, created_at, updated_at, last_activity_at, completed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	agent_worker_id = excluded.agent_worker_id,
	session_kind = excluded.session_kind,
	parent_session_id = excluded.parent_session_id,
	project_id = excluded.project_id,
	project_membership_id = excluded.project_membership_id,
	task_id = excluded.task_id,
	workflow_instance_id = excluded.workflow_instance_id,
	title = excluded.title,
	purpose = excluded.purpose,
	initial_prompt = excluded.initial_prompt,
	status = excluded.status,
	runtime_provider = excluded.runtime_provider,
	runtime_session_id = excluded.runtime_session_id,
	fork_mode = excluded.fork_mode,
	permission_policy = excluded.permission_policy,
	capabilities_json = excluded.capabilities_json,
	result_summary = excluded.result_summary,
	result_refs_json = excluded.result_refs_json,
	created_by_run_id = excluded.created_by_run_id,
	last_run_id = excluded.last_run_id,
	updated_at = excluded.updated_at,
	last_activity_at = excluded.last_activity_at,
	completed_at = excluded.completed_at`,
		session.ID, session.WorkspaceID, session.AgentWorkerID, session.SessionKind, session.ParentSessionID, session.ProjectID, session.ProjectMembershipID,
		session.TaskID, session.WorkflowInstanceID, session.Title, session.Purpose, session.InitialPrompt, session.Status, session.RuntimeProvider, session.RuntimeSessionID,
		session.ForkMode, session.PermissionPolicy, defaultJSONObject(session.CapabilitiesJSON), session.ResultSummary, defaultJSONObject(session.ResultRefsJSON),
		session.CreatedByRunID, session.LastRunID, session.CreatedAt, session.UpdatedAt, session.LastActivityAt, session.CompletedAt)
	return err
}

func (db *SQLiteStore) AgentSessionByID(workspaceID, id string) (AgentSession, bool, error) {
	row := db.sql.QueryRow(agentSessionSelectSQL()+` WHERE workspace_id = ? AND id = ?`, strings.TrimSpace(workspaceID), strings.TrimSpace(id))
	session, err := scanAgentSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentSession{}, false, nil
	}
	if err != nil {
		return AgentSession{}, false, err
	}
	return session, true, nil
}

func (db *SQLiteStore) ListAgentSessions(filter AgentSessionFilter) ([]AgentSession, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := agentSessionSelectSQL() + ` WHERE workspace_id = ?`
	args := []any{strings.TrimSpace(filter.WorkspaceID)}
	if strings.TrimSpace(filter.AgentWorkerID) != "" {
		query += ` AND agent_worker_id = ?`
		args = append(args, strings.TrimSpace(filter.AgentWorkerID))
	}
	if strings.TrimSpace(filter.SessionKind) != "" {
		query += ` AND session_kind = ?`
		args = append(args, strings.TrimSpace(filter.SessionKind))
	}
	if strings.TrimSpace(filter.ParentSessionID) != "" {
		query += ` AND parent_session_id = ?`
		args = append(args, strings.TrimSpace(filter.ParentSessionID))
	}
	if strings.TrimSpace(filter.ProjectID) != "" {
		query += ` AND project_id = ?`
		args = append(args, strings.TrimSpace(filter.ProjectID))
	}
	if strings.TrimSpace(filter.TaskID) != "" {
		query += ` AND task_id = ?`
		args = append(args, strings.TrimSpace(filter.TaskID))
	}
	statuses := normalizedStatuses(filter.Status, filter.Statuses)
	if len(statuses) == 1 {
		query += ` AND status = ?`
		args = append(args, statuses[0])
	} else if len(statuses) > 1 {
		query += ` AND status IN (` + placeholders(len(statuses)) + `)`
		for _, status := range statuses {
			args = append(args, status)
		}
	}
	query += ` ORDER BY updated_at DESC, created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := db.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AgentSession, 0)
	for rows.Next() {
		session, err := scanAgentSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, session)
	}
	return out, rows.Err()
}

func normalizedStatuses(status string, statuses []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(statuses)+1)
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		out = append(out, v)
	}
	add(status)
	for _, v := range statuses {
		add(v)
	}
	return out
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "?"
	}
	return strings.Join(parts, ",")
}

func agentSessionSelectSQL() string {
	return `SELECT id, workspace_id, agent_worker_id, session_kind, parent_session_id, project_id, project_membership_id,
task_id, workflow_instance_id, title, purpose, initial_prompt, status, runtime_provider, runtime_session_id,
fork_mode, permission_policy, capabilities_json, result_summary, result_refs_json,
created_by_run_id, last_run_id, created_at, updated_at, last_activity_at, completed_at
FROM agent_sessions`
}

type agentSessionScanner interface {
	Scan(dest ...any) error
}

func scanAgentSession(row agentSessionScanner) (AgentSession, error) {
	var session AgentSession
	err := row.Scan(&session.ID, &session.WorkspaceID, &session.AgentWorkerID, &session.SessionKind, &session.ParentSessionID,
		&session.ProjectID, &session.ProjectMembershipID, &session.TaskID, &session.WorkflowInstanceID,
		&session.Title, &session.Purpose, &session.InitialPrompt, &session.Status, &session.RuntimeProvider, &session.RuntimeSessionID,
		&session.ForkMode, &session.PermissionPolicy, &session.CapabilitiesJSON, &session.ResultSummary, &session.ResultRefsJSON,
		&session.CreatedByRunID, &session.LastRunID, &session.CreatedAt, &session.UpdatedAt, &session.LastActivityAt, &session.CompletedAt)
	return session, err
}
