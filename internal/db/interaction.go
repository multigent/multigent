package db

import (
	"database/sql"
	"errors"
	"strings"
)

func (db *SQLiteStore) CreateInteractionSession(session InteractionSession) error {
	if session.CreatedAt == "" {
		session.CreatedAt = nowUTC()
	}
	if session.UpdatedAt == "" {
		session.UpdatedAt = session.CreatedAt
	}
	if session.LastActivityAt == "" {
		session.LastActivityAt = session.CreatedAt
	}
	if session.Status == "" {
		session.Status = "active"
	}
	if session.LockReason == "" {
		session.LockReason = "interactive"
	}
	if session.MetadataJSON == "" {
		session.MetadataJSON = "{}"
	}
	_, err := db.sql.Exec(`INSERT INTO interactive_sessions (
id, workspace_id, project_id, agent_id, source_kind, source_channel, actor_type, actor_id,
status, lock_reason, runtime_session_id, current_run_id, human_intervened, metadata_json,
created_at, updated_at, last_activity_at, completed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID, session.WorkspaceID, session.ProjectID, session.AgentID, session.SourceKind,
		session.SourceChannel, session.ActorType, session.ActorID, session.Status, session.LockReason,
		session.RuntimeSessionID, session.CurrentRunID, boolInt(session.HumanIntervened), session.MetadataJSON,
		session.CreatedAt, session.UpdatedAt, session.LastActivityAt, session.CompletedAt)
	return err
}

func (db *SQLiteStore) UpdateInteractionSession(session InteractionSession) error {
	if session.UpdatedAt == "" {
		session.UpdatedAt = nowUTC()
	}
	if session.MetadataJSON == "" {
		session.MetadataJSON = "{}"
	}
	_, err := db.sql.Exec(`UPDATE interactive_sessions SET
	source_kind = ?, source_channel = ?, actor_type = ?, actor_id = ?, status = ?, lock_reason = ?,
	runtime_session_id = ?, current_run_id = ?, human_intervened = ?, metadata_json = ?,
	updated_at = ?, last_activity_at = ?, completed_at = ?
WHERE id = ?`,
		session.SourceKind, session.SourceChannel, session.ActorType, session.ActorID, session.Status, session.LockReason,
		session.RuntimeSessionID, session.CurrentRunID, boolInt(session.HumanIntervened), session.MetadataJSON,
		session.UpdatedAt, session.LastActivityAt, session.CompletedAt, session.ID)
	return err
}

func (db *SQLiteStore) ActiveInteractionSession(workspaceID, projectID, agentID string) (InteractionSession, bool, error) {
	row := db.sql.QueryRow(`SELECT id, workspace_id, project_id, agent_id, source_kind, source_channel,
actor_type, actor_id, status, lock_reason, runtime_session_id, current_run_id, human_intervened,
metadata_json, created_at, updated_at, last_activity_at, completed_at
FROM interactive_sessions
WHERE workspace_id = ? AND project_id = ? AND agent_id = ? AND status IN ('active', 'waiting_input')
ORDER BY updated_at DESC, created_at DESC LIMIT 1`, workspaceID, projectID, agentID)
	return scanInteractionSessionFound(row)
}

func (db *SQLiteStore) InteractionSessionByID(id string) (InteractionSession, bool, error) {
	row := db.sql.QueryRow(`SELECT id, workspace_id, project_id, agent_id, source_kind, source_channel,
actor_type, actor_id, status, lock_reason, runtime_session_id, current_run_id, human_intervened,
metadata_json, created_at, updated_at, last_activity_at, completed_at
FROM interactive_sessions WHERE id = ?`, id)
	return scanInteractionSessionFound(row)
}

func (db *SQLiteStore) ListInteractionSessions(filter InteractionSessionFilter) ([]InteractionSession, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `SELECT id, workspace_id, project_id, agent_id, source_kind, source_channel,
actor_type, actor_id, status, lock_reason, runtime_session_id, current_run_id, human_intervened,
metadata_json, created_at, updated_at, last_activity_at, completed_at
FROM interactive_sessions WHERE 1=1`
	args := make([]any, 0, 5)
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
	if strings.TrimSpace(filter.Status) != "" {
		query += ` AND status = ?`
		args = append(args, strings.TrimSpace(filter.Status))
	}
	query += ` ORDER BY updated_at DESC, created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := db.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]InteractionSession, 0)
	for rows.Next() {
		session, err := scanInteractionSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, session)
	}
	return out, rows.Err()
}

func (db *SQLiteStore) CreateInteractionEvent(event InteractionEvent) error {
	if event.CreatedAt == "" {
		event.CreatedAt = nowUTC()
	}
	if event.MetadataJSON == "" {
		event.MetadataJSON = "{}"
	}
	_, err := db.sql.Exec(`INSERT INTO interaction_events (
id, session_id, workspace_id, actor_type, actor_id, channel, event_type, content, metadata_json, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.ID, event.SessionID, event.WorkspaceID, event.ActorType, event.ActorID,
		event.Channel, event.EventType, event.Content, event.MetadataJSON, event.CreatedAt)
	return err
}

func (db *SQLiteStore) ListInteractionEvents(filter InteractionEventFilter) ([]InteractionEvent, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `SELECT id, session_id, workspace_id, actor_type, actor_id, channel, event_type, content, metadata_json, created_at
FROM interaction_events WHERE 1=1`
	args := make([]any, 0, 3)
	if strings.TrimSpace(filter.WorkspaceID) != "" {
		query += ` AND workspace_id = ?`
		args = append(args, strings.TrimSpace(filter.WorkspaceID))
	}
	if strings.TrimSpace(filter.SessionID) != "" {
		query += ` AND session_id = ?`
		args = append(args, strings.TrimSpace(filter.SessionID))
	}
	query += ` ORDER BY created_at ASC, id ASC LIMIT ?`
	args = append(args, limit)
	rows, err := db.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]InteractionEvent, 0)
	for rows.Next() {
		var event InteractionEvent
		if err := rows.Scan(&event.ID, &event.SessionID, &event.WorkspaceID, &event.ActorType, &event.ActorID,
			&event.Channel, &event.EventType, &event.Content, &event.MetadataJSON, &event.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

type interactionSessionScanner interface {
	Scan(dest ...any) error
}

func scanInteractionSessionFound(row interactionSessionScanner) (InteractionSession, bool, error) {
	session, err := scanInteractionSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return InteractionSession{}, false, nil
	}
	if err != nil {
		return InteractionSession{}, false, err
	}
	return session, true, nil
}

func scanInteractionSession(row interactionSessionScanner) (InteractionSession, error) {
	var session InteractionSession
	var humanIntervened int
	err := row.Scan(&session.ID, &session.WorkspaceID, &session.ProjectID, &session.AgentID,
		&session.SourceKind, &session.SourceChannel, &session.ActorType, &session.ActorID,
		&session.Status, &session.LockReason, &session.RuntimeSessionID, &session.CurrentRunID,
		&humanIntervened, &session.MetadataJSON, &session.CreatedAt, &session.UpdatedAt,
		&session.LastActivityAt, &session.CompletedAt)
	session.HumanIntervened = humanIntervened != 0
	return session, err
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
