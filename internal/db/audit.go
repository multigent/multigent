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
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	where, args := auditEventWhere(filter)
	query := `SELECT id, workspace_id, actor_type, actor_id, action, resource_type, resource_id,
summary, before_json, after_json, ip, user_agent, created_at FROM audit_events ` + where + ` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

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

func (db *SQLiteStore) CountAuditEvents(filter AuditEventFilter) (int, error) {
	where, args := auditEventWhere(filter)
	var count int
	if err := db.sql.QueryRow(`SELECT COUNT(*) FROM audit_events `+where, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (db *SQLiteStore) ListAuditEventFacets(filter AuditEventFilter) (AuditEventFacets, error) {
	base := AuditEventFilter{
		WorkspaceID:  filter.WorkspaceID,
		CreatedAfter: filter.CreatedAfter,
	}
	actorIDs, err := db.distinctAuditValues(base, "actor_id", 100)
	if err != nil {
		return AuditEventFacets{}, err
	}
	actions, err := db.distinctAuditValues(base, "action", 120)
	if err != nil {
		return AuditEventFacets{}, err
	}
	resourceTypes, err := db.distinctAuditValues(base, "resource_type", 80)
	if err != nil {
		return AuditEventFacets{}, err
	}
	resourceIDs, err := db.distinctAuditValues(base, "resource_id", 200)
	if err != nil {
		return AuditEventFacets{}, err
	}
	return AuditEventFacets{
		ActorIDs:      actorIDs,
		Actions:       actions,
		ResourceTypes: resourceTypes,
		ResourceIDs:   resourceIDs,
	}, nil
}

func (db *SQLiteStore) distinctAuditValues(filter AuditEventFilter, column string, limit int) ([]string, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	switch column {
	case "actor_id", "action", "resource_type", "resource_id":
	default:
		return nil, nil
	}
	where, args := auditEventWhere(filter)
	query := `SELECT DISTINCT ` + column + ` FROM audit_events ` + where + ` AND ` + column + ` != '' ORDER BY ` + column + ` ASC LIMIT ?`
	args = append(args, limit)
	rows, err := db.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out, rows.Err()
}

func auditEventWhere(filter AuditEventFilter) (string, []any) {
	query := `WHERE 1=1`
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
	if strings.TrimSpace(filter.CreatedAfter) != "" {
		query += ` AND created_at >= ?`
		args = append(args, strings.TrimSpace(filter.CreatedAfter))
	}
	return query, args
}
