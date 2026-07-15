package db

import (
	"strings"
)

func (db *SQLiteStore) CreateAuditEvent(event AuditEvent) error {
	if event.CreatedAt == "" {
		event.CreatedAt = nowUTC()
	}
	_, err := db.sql.Exec(`INSERT INTO audit_events (
	id, workspace_id, actor_type, actor_id, action, resource_type, resource_id,
	summary, before_json, after_json, ip, user_agent, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.ID, event.WorkspaceID, event.ActorType, event.ActorID, event.Action,
		event.ResourceType, event.ResourceID, event.Summary, event.BeforeJSON, event.AfterJSON,
		event.IP, event.UserAgent, event.CreatedAt)
	return err
}

func (db *SQLiteStore) ListAuditEvents(filter AuditEventFilter) ([]AuditEvent, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `SELECT id, workspace_id, actor_type, actor_id, action, resource_type, resource_id,
summary, before_json, after_json, ip, user_agent, created_at FROM audit_events WHERE 1=1`
	args := make([]any, 0, 6)
	if strings.TrimSpace(filter.WorkspaceID) != "" {
		query += ` AND workspace_id = ?`
		args = append(args, strings.TrimSpace(filter.WorkspaceID))
	}
	if strings.TrimSpace(filter.ActorID) != "" {
		query += ` AND actor_id = ?`
		args = append(args, strings.TrimSpace(filter.ActorID))
	}
	if strings.TrimSpace(filter.Action) != "" {
		query += ` AND action = ?`
		args = append(args, strings.TrimSpace(filter.Action))
	}
	if strings.TrimSpace(filter.ResourceType) != "" {
		query += ` AND resource_type = ?`
		args = append(args, strings.TrimSpace(filter.ResourceType))
	}
	if strings.TrimSpace(filter.ResourceID) != "" {
		query += ` AND resource_id = ?`
		args = append(args, strings.TrimSpace(filter.ResourceID))
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := db.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]AuditEvent, 0)
	for rows.Next() {
		var e AuditEvent
		if err := rows.Scan(&e.ID, &e.WorkspaceID, &e.ActorType, &e.ActorID, &e.Action,
			&e.ResourceType, &e.ResourceID, &e.Summary, &e.BeforeJSON, &e.AfterJSON,
			&e.IP, &e.UserAgent, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
