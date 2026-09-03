package db

import (
	"database/sql"
	"errors"
	"strings"
)

func (db *SQLiteStore) UpsertAttentionSignal(s AttentionSignal) error {
	if s.CreatedAt == "" {
		s.CreatedAt = nowUTC()
	}
	if s.Status == "" {
		s.Status = "pending"
	}
	if s.Priority == "" {
		s.Priority = "normal"
	}
	if s.RefsJSON == "" {
		s.RefsJSON = "{}"
	}
	if s.PayloadJSON == "" {
		s.PayloadJSON = "{}"
	}
	_, err := db.sql.Exec(`INSERT INTO attention_signals (
	id, workspace_id, agent_worker_id, dedupe_key, source_kind, source_id, source_channel,
	reason, priority, actor_type, actor_id, summary, refs_json, payload_json, result_ref,
	status, created_at, expires_at, seen_at, handling_at, handled_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(workspace_id, agent_worker_id, dedupe_key) DO UPDATE SET
	source_kind = excluded.source_kind,
	source_id = excluded.source_id,
	source_channel = excluded.source_channel,
	reason = excluded.reason,
	priority = excluded.priority,
	actor_type = excluded.actor_type,
	actor_id = excluded.actor_id,
	summary = excluded.summary,
	refs_json = excluded.refs_json,
	payload_json = excluded.payload_json,
	result_ref = excluded.result_ref,
	status = CASE
		WHEN attention_signals.status IN ('handled', 'ignored', 'expired') THEN attention_signals.status
		ELSE excluded.status
	END,
	expires_at = excluded.expires_at,
	seen_at = CASE WHEN attention_signals.seen_at != '' THEN attention_signals.seen_at ELSE excluded.seen_at END,
	handling_at = CASE WHEN attention_signals.handling_at != '' THEN attention_signals.handling_at ELSE excluded.handling_at END,
	handled_at = CASE WHEN attention_signals.handled_at != '' THEN attention_signals.handled_at ELSE excluded.handled_at END`,
		s.ID, s.WorkspaceID, s.AgentWorkerID, s.DedupeKey, s.SourceKind, s.SourceID, s.SourceChannel,
		s.Reason, s.Priority, s.ActorType, s.ActorID, s.Summary, s.RefsJSON, s.PayloadJSON, s.ResultRef,
		s.Status, s.CreatedAt, s.ExpiresAt, s.SeenAt, s.HandlingAt, s.HandledAt)
	return err
}

func (db *SQLiteStore) AttentionSignalByID(workspaceID, id string) (AttentionSignal, bool, error) {
	row := db.sql.QueryRow(`SELECT id, workspace_id, agent_worker_id, dedupe_key, source_kind, source_id, source_channel,
reason, priority, actor_type, actor_id, summary, refs_json, payload_json, result_ref,
status, created_at, expires_at, seen_at, handling_at, handled_at
FROM attention_signals WHERE workspace_id = ? AND id = ?`, strings.TrimSpace(workspaceID), strings.TrimSpace(id))
	return scanAttentionSignalFound(row)
}

func (db *SQLiteStore) ListAttentionSignals(filter AttentionSignalFilter) ([]AttentionSignal, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `SELECT id, workspace_id, agent_worker_id, dedupe_key, source_kind, source_id, source_channel,
reason, priority, actor_type, actor_id, summary, refs_json, payload_json, result_ref,
status, created_at, expires_at, seen_at, handling_at, handled_at
FROM attention_signals WHERE 1=1`
	args := make([]any, 0, 6)
	if strings.TrimSpace(filter.WorkspaceID) != "" {
		query += ` AND workspace_id = ?`
		args = append(args, strings.TrimSpace(filter.WorkspaceID))
	}
	if strings.TrimSpace(filter.AgentWorkerID) != "" {
		query += ` AND agent_worker_id = ?`
		args = append(args, strings.TrimSpace(filter.AgentWorkerID))
	}
	if strings.TrimSpace(filter.Status) != "" {
		query += ` AND status = ?`
		args = append(args, strings.TrimSpace(filter.Status))
	} else if len(filter.Statuses) > 0 {
		statuses := make([]string, 0, len(filter.Statuses))
		for _, status := range filter.Statuses {
			status = strings.TrimSpace(status)
			if status != "" {
				statuses = append(statuses, status)
			}
		}
		if len(statuses) > 0 {
			query += ` AND status IN (` + strings.TrimRight(strings.Repeat("?,", len(statuses)), ",") + `)`
			for _, status := range statuses {
				args = append(args, status)
			}
		}
	}
	if strings.TrimSpace(filter.SourceKind) != "" {
		query += ` AND source_kind = ?`
		args = append(args, strings.TrimSpace(filter.SourceKind))
	}
	if strings.TrimSpace(filter.Reason) != "" {
		query += ` AND reason = ?`
		args = append(args, strings.TrimSpace(filter.Reason))
	}
	query += ` ORDER BY created_at ASC LIMIT ?`
	args = append(args, limit)
	rows, err := db.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AttentionSignal, 0)
	for rows.Next() {
		s, err := scanAttentionSignal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (db *SQLiteStore) MarkAttentionSignalStatus(workspaceID, id, status string) error {
	return db.markAttentionSignalStatus(workspaceID, id, status, "")
}

func (db *SQLiteStore) MarkAttentionSignalStatusWithResult(workspaceID, id, status, resultRef string) error {
	return db.markAttentionSignalStatus(workspaceID, id, status, resultRef)
}

func (db *SQLiteStore) markAttentionSignalStatus(workspaceID, id, status, resultRef string) error {
	now := nowUTC()
	field := "seen_at"
	guard := ""
	switch strings.TrimSpace(status) {
	case "handling":
		field = "handling_at"
		guard = " AND status NOT IN ('handled', 'ignored', 'expired')"
	case "handled", "ignored", "expired":
		field = "handled_at"
		// Terminal signals are immutable. Replaying a wakeup or an explicit
		// close must not reopen the signal or overwrite its audit reference.
		guard = " AND status NOT IN ('handled', 'ignored', 'expired')"
	case "seen":
		guard = " AND status = 'pending'"
	}
	query := `UPDATE attention_signals SET status = ?, ` + field + ` = ?`
	args := []any{strings.TrimSpace(status), now}
	if strings.TrimSpace(resultRef) != "" {
		query += `, result_ref = ?`
		args = append(args, strings.TrimSpace(resultRef))
	}
	query += ` WHERE workspace_id = ? AND id = ?` + guard
	args = append(args, strings.TrimSpace(workspaceID), strings.TrimSpace(id))
	_, err := db.sql.Exec(query, args...)
	return err
}

type attentionSignalScanner interface {
	Scan(dest ...any) error
}

func scanAttentionSignalFound(row attentionSignalScanner) (AttentionSignal, bool, error) {
	s, err := scanAttentionSignal(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AttentionSignal{}, false, nil
	}
	if err != nil {
		return AttentionSignal{}, false, err
	}
	return s, true, nil
}

func scanAttentionSignal(row attentionSignalScanner) (AttentionSignal, error) {
	var s AttentionSignal
	err := row.Scan(&s.ID, &s.WorkspaceID, &s.AgentWorkerID, &s.DedupeKey, &s.SourceKind, &s.SourceID, &s.SourceChannel,
		&s.Reason, &s.Priority, &s.ActorType, &s.ActorID, &s.Summary, &s.RefsJSON, &s.PayloadJSON, &s.ResultRef,
		&s.Status, &s.CreatedAt, &s.ExpiresAt, &s.SeenAt, &s.HandlingAt, &s.HandledAt)
	return s, err
}
