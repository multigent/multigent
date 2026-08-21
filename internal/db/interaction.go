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
id, workspace_id, agent_worker_id, project_id, agent_id, source_kind, source_channel, actor_type, actor_id,
status, lock_reason, runtime_session_id, current_run_id, human_intervened, metadata_json,
created_at, updated_at, last_activity_at, completed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID, session.WorkspaceID, session.AgentWorkerID, session.ProjectID, session.AgentID, session.SourceKind,
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
	agent_worker_id = ?, project_id = ?, agent_id = ?, source_kind = ?, source_channel = ?, actor_type = ?, actor_id = ?, status = ?, lock_reason = ?,
	runtime_session_id = ?, current_run_id = ?, human_intervened = ?, metadata_json = ?,
	updated_at = ?, last_activity_at = ?, completed_at = ?
WHERE id = ?`,
		session.AgentWorkerID, session.ProjectID, session.AgentID, session.SourceKind, session.SourceChannel, session.ActorType, session.ActorID, session.Status, session.LockReason,
		session.RuntimeSessionID, session.CurrentRunID, boolInt(session.HumanIntervened), session.MetadataJSON,
		session.UpdatedAt, session.LastActivityAt, session.CompletedAt, session.ID)
	return err
}

func (db *SQLiteStore) ActiveInteractionSession(workspaceID, projectID, agentID string) (InteractionSession, bool, error) {
	row := db.sql.QueryRow(`SELECT id, workspace_id, agent_worker_id, project_id, agent_id, source_kind, source_channel,
actor_type, actor_id, status, lock_reason, runtime_session_id, current_run_id, human_intervened,
metadata_json, created_at, updated_at, last_activity_at, completed_at
FROM interactive_sessions
WHERE workspace_id = ? AND project_id = ? AND agent_id = ? AND status IN ('active', 'waiting_input')
ORDER BY updated_at DESC, created_at DESC LIMIT 1`, workspaceID, projectID, agentID)
	return scanInteractionSessionFound(row)
}

func (db *SQLiteStore) ActiveInteractionSessionForWorker(workspaceID, agentWorkerID string) (InteractionSession, bool, error) {
	row := db.sql.QueryRow(`SELECT id, workspace_id, agent_worker_id, project_id, agent_id, source_kind, source_channel,
actor_type, actor_id, status, lock_reason, runtime_session_id, current_run_id, human_intervened,
metadata_json, created_at, updated_at, last_activity_at, completed_at
FROM interactive_sessions
WHERE workspace_id = ? AND agent_worker_id = ? AND status IN ('active', 'waiting_input')
ORDER BY updated_at DESC, created_at DESC LIMIT 1`, strings.TrimSpace(workspaceID), strings.TrimSpace(agentWorkerID))
	return scanInteractionSessionFound(row)
}

func (db *SQLiteStore) ActiveInteractionSessionForSource(workspaceID, projectID, agentID, sourceKind, sourceChannel, actorID string) (InteractionSession, bool, error) {
	row := db.sql.QueryRow(`SELECT id, workspace_id, agent_worker_id, project_id, agent_id, source_kind, source_channel,
actor_type, actor_id, status, lock_reason, runtime_session_id, current_run_id, human_intervened,
metadata_json, created_at, updated_at, last_activity_at, completed_at
FROM interactive_sessions
WHERE workspace_id = ? AND project_id = ? AND agent_id = ?
  AND source_kind = ? AND source_channel = ? AND actor_id = ?
  AND status IN ('active', 'waiting_input')
ORDER BY updated_at DESC, created_at DESC LIMIT 1`, strings.TrimSpace(workspaceID), strings.TrimSpace(projectID), strings.TrimSpace(agentID), strings.TrimSpace(sourceKind), strings.TrimSpace(sourceChannel), strings.TrimSpace(actorID))
	return scanInteractionSessionFound(row)
}

func (db *SQLiteStore) ActiveInteractionSessionForWorkerSource(workspaceID, agentWorkerID, sourceKind, sourceChannel, actorID string) (InteractionSession, bool, error) {
	row := db.sql.QueryRow(`SELECT id, workspace_id, agent_worker_id, project_id, agent_id, source_kind, source_channel,
actor_type, actor_id, status, lock_reason, runtime_session_id, current_run_id, human_intervened,
metadata_json, created_at, updated_at, last_activity_at, completed_at
FROM interactive_sessions
WHERE workspace_id = ? AND agent_worker_id = ?
  AND source_kind = ? AND source_channel = ? AND actor_id = ?
  AND status IN ('active', 'waiting_input')
ORDER BY updated_at DESC, created_at DESC LIMIT 1`, strings.TrimSpace(workspaceID), strings.TrimSpace(agentWorkerID), strings.TrimSpace(sourceKind), strings.TrimSpace(sourceChannel), strings.TrimSpace(actorID))
	return scanInteractionSessionFound(row)
}

func (db *SQLiteStore) InteractionSessionByID(id string) (InteractionSession, bool, error) {
	row := db.sql.QueryRow(`SELECT id, workspace_id, agent_worker_id, project_id, agent_id, source_kind, source_channel,
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
	query := `SELECT id, workspace_id, agent_worker_id, project_id, agent_id, source_kind, source_channel,
actor_type, actor_id, status, lock_reason, runtime_session_id, current_run_id, human_intervened,
metadata_json, created_at, updated_at, last_activity_at, completed_at
FROM interactive_sessions WHERE 1=1`
	args := make([]any, 0, 5)
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
	if strings.TrimSpace(filter.SourceKind) != "" {
		query += ` AND source_kind = ?`
		args = append(args, strings.TrimSpace(filter.SourceKind))
	}
	if strings.TrimSpace(filter.SourceChannel) != "" {
		query += ` AND source_channel = ?`
		args = append(args, strings.TrimSpace(filter.SourceChannel))
	}
	if strings.TrimSpace(filter.ActorID) != "" {
		query += ` AND actor_id = ?`
		args = append(args, strings.TrimSpace(filter.ActorID))
	}
	if strings.TrimSpace(filter.Status) != "" {
		query += ` AND status = ?`
		args = append(args, strings.TrimSpace(filter.Status))
	}
	if filter.RuntimeSession {
		query += ` AND runtime_session_id != ''`
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
	err := row.Scan(&session.ID, &session.WorkspaceID, &session.AgentWorkerID, &session.ProjectID, &session.AgentID,
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

func (db *SQLiteStore) CreateInteractionRequest(request InteractionRequest) error {
	if request.CreatedAt == "" {
		request.CreatedAt = nowUTC()
	}
	if request.Status == "" {
		request.Status = "active"
	}
	if request.SchemaJSON == "" {
		request.SchemaJSON = "{}"
	}
	if request.ContextJSON == "" {
		request.ContextJSON = "{}"
	}
	if request.HandlerType == "" {
		request.HandlerType = "agent_event"
	}
	_, err := db.sql.Exec(`INSERT INTO interaction_requests (
id, workspace_id, agent_worker_id, project_id, agent_id, channel_binding_id, provider, recipient,
target_type, target_user_id, target_chat_id, title, body, schema_json, context_json,
handler_type, status, created_by, created_at, expires_at, submitted_at, submitted_by, submission_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		request.ID, request.WorkspaceID, request.AgentWorkerID, request.ProjectID, request.AgentID, request.ChannelBindingID,
		request.Provider, request.Recipient, request.TargetType, request.TargetUserID, request.TargetChatID,
		request.Title, request.Body, request.SchemaJSON, request.ContextJSON, request.HandlerType,
		request.Status, request.CreatedBy, request.CreatedAt, request.ExpiresAt, request.SubmittedAt,
		request.SubmittedBy, request.SubmissionJSON)
	return err
}

func (db *SQLiteStore) InteractionRequestByID(workspaceID, id string) (InteractionRequest, bool, error) {
	row := db.sql.QueryRow(`SELECT id, workspace_id, agent_worker_id, project_id, agent_id, channel_binding_id, provider, recipient,
target_type, target_user_id, target_chat_id, title, body, schema_json, context_json,
handler_type, status, created_by, created_at, expires_at, submitted_at, submitted_by, submission_json
FROM interaction_requests WHERE workspace_id = ? AND id = ?`, strings.TrimSpace(workspaceID), strings.TrimSpace(id))
	request, err := scanInteractionRequest(row)
	if errors.Is(err, sql.ErrNoRows) {
		return InteractionRequest{}, false, nil
	}
	if err != nil {
		return InteractionRequest{}, false, err
	}
	return request, true, nil
}

func (db *SQLiteStore) ListInteractionRequests(filter InteractionRequestFilter) ([]InteractionRequest, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `SELECT id, workspace_id, agent_worker_id, project_id, agent_id, channel_binding_id, provider, recipient,
target_type, target_user_id, target_chat_id, title, body, schema_json, context_json,
handler_type, status, created_by, created_at, expires_at, submitted_at, submitted_by, submission_json
FROM interaction_requests WHERE 1=1`
	args := make([]any, 0, 7)
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
	if strings.TrimSpace(filter.ChannelBindingID) != "" {
		query += ` AND channel_binding_id = ?`
		args = append(args, strings.TrimSpace(filter.ChannelBindingID))
	}
	if strings.TrimSpace(filter.Provider) != "" {
		query += ` AND provider = ?`
		args = append(args, strings.TrimSpace(filter.Provider))
	}
	if strings.TrimSpace(filter.Status) != "" {
		query += ` AND status = ?`
		args = append(args, strings.TrimSpace(filter.Status))
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := db.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]InteractionRequest, 0)
	for rows.Next() {
		request, err := scanInteractionRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, request)
	}
	return out, rows.Err()
}

func (db *SQLiteStore) UpdateInteractionRequest(request InteractionRequest) error {
	if request.SchemaJSON == "" {
		request.SchemaJSON = "{}"
	}
	if request.ContextJSON == "" {
		request.ContextJSON = "{}"
	}
	_, err := db.sql.Exec(`UPDATE interaction_requests SET
	agent_worker_id = ?, project_id = ?, agent_id = ?, channel_binding_id = ?, provider = ?, recipient = ?, target_type = ?, target_user_id = ?, target_chat_id = ?,
	title = ?, body = ?, schema_json = ?, context_json = ?, handler_type = ?, status = ?, created_by = ?,
	created_at = ?, expires_at = ?, submitted_at = ?, submitted_by = ?, submission_json = ?
WHERE workspace_id = ? AND id = ?`,
		request.AgentWorkerID, request.ProjectID, request.AgentID, request.ChannelBindingID, request.Provider, request.Recipient, request.TargetType,
		request.TargetUserID, request.TargetChatID, request.Title, request.Body, request.SchemaJSON,
		request.ContextJSON, request.HandlerType, request.Status, request.CreatedBy, request.CreatedAt,
		request.ExpiresAt, request.SubmittedAt, request.SubmittedBy, request.SubmissionJSON,
		request.WorkspaceID, request.ID)
	return err
}

func scanInteractionRequest(row interactionSessionScanner) (InteractionRequest, error) {
	var request InteractionRequest
	err := row.Scan(&request.ID, &request.WorkspaceID, &request.AgentWorkerID, &request.ProjectID, &request.AgentID,
		&request.ChannelBindingID, &request.Provider, &request.Recipient, &request.TargetType,
		&request.TargetUserID, &request.TargetChatID, &request.Title, &request.Body, &request.SchemaJSON,
		&request.ContextJSON, &request.HandlerType, &request.Status, &request.CreatedBy,
		&request.CreatedAt, &request.ExpiresAt, &request.SubmittedAt, &request.SubmittedBy,
		&request.SubmissionJSON)
	return request, err
}
