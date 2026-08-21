package db

import (
	"database/sql"
	"errors"
	"strings"
)

func (db *SQLiteStore) UpsertAttentionCursor(c AttentionCursor) error {
	if c.UpdatedAt == "" {
		c.UpdatedAt = nowUTC()
	}
	_, err := db.sql.Exec(`INSERT INTO attention_cursors (
	id, workspace_id, agent_worker_id, source_kind, source_channel, cursor, seen_until, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(workspace_id, agent_worker_id, source_kind, source_channel) DO UPDATE SET
	cursor = excluded.cursor,
	seen_until = excluded.seen_until,
	updated_at = excluded.updated_at`,
		c.ID, strings.TrimSpace(c.WorkspaceID), strings.TrimSpace(c.AgentWorkerID),
		strings.TrimSpace(c.SourceKind), strings.TrimSpace(c.SourceChannel),
		c.Cursor, c.SeenUntil, c.UpdatedAt)
	return err
}

func (db *SQLiteStore) AttentionCursorBySource(workspaceID, agentWorkerID, sourceKind, sourceChannel string) (AttentionCursor, bool, error) {
	row := db.sql.QueryRow(`SELECT id, workspace_id, agent_worker_id, source_kind, source_channel, cursor, seen_until, updated_at
FROM attention_cursors
WHERE workspace_id = ? AND agent_worker_id = ? AND source_kind = ? AND source_channel = ?`,
		strings.TrimSpace(workspaceID), strings.TrimSpace(agentWorkerID),
		strings.TrimSpace(sourceKind), strings.TrimSpace(sourceChannel))
	return scanAttentionCursorFound(row)
}

func (db *SQLiteStore) ListAttentionCursors(filter AttentionCursorFilter) ([]AttentionCursor, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `SELECT id, workspace_id, agent_worker_id, source_kind, source_channel, cursor, seen_until, updated_at
FROM attention_cursors WHERE 1=1`
	args := make([]any, 0, 5)
	if strings.TrimSpace(filter.WorkspaceID) != "" {
		query += ` AND workspace_id = ?`
		args = append(args, strings.TrimSpace(filter.WorkspaceID))
	}
	if strings.TrimSpace(filter.AgentWorkerID) != "" {
		query += ` AND agent_worker_id = ?`
		args = append(args, strings.TrimSpace(filter.AgentWorkerID))
	}
	if strings.TrimSpace(filter.SourceKind) != "" {
		query += ` AND source_kind = ?`
		args = append(args, strings.TrimSpace(filter.SourceKind))
	}
	if strings.TrimSpace(filter.SourceChannel) != "" {
		query += ` AND source_channel = ?`
		args = append(args, strings.TrimSpace(filter.SourceChannel))
	}
	query += ` ORDER BY updated_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := db.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AttentionCursor, 0)
	for rows.Next() {
		c, err := scanAttentionCursor(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type attentionCursorScanner interface {
	Scan(dest ...any) error
}

func scanAttentionCursorFound(row attentionCursorScanner) (AttentionCursor, bool, error) {
	c, err := scanAttentionCursor(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AttentionCursor{}, false, nil
	}
	if err != nil {
		return AttentionCursor{}, false, err
	}
	return c, true, nil
}

func scanAttentionCursor(row attentionCursorScanner) (AttentionCursor, error) {
	var c AttentionCursor
	err := row.Scan(&c.ID, &c.WorkspaceID, &c.AgentWorkerID, &c.SourceKind, &c.SourceChannel, &c.Cursor, &c.SeenUntil, &c.UpdatedAt)
	return c, err
}
