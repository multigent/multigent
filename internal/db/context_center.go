package db

import (
	"database/sql"
	"errors"
	"strings"
)

func (db *SQLiteStore) UpsertContextSource(source ContextSource) error {
	if source.CreatedAt == "" {
		source.CreatedAt = nowUTC()
	}
	if source.UpdatedAt == "" {
		source.UpdatedAt = source.CreatedAt
	}
	if source.Status == "" {
		source.Status = "active"
	}
	if source.ConfigJSON == "" {
		source.ConfigJSON = "{}"
	}
	if source.MetadataJSON == "" {
		source.MetadataJSON = "{}"
	}
	_, err := db.sql.Exec(`INSERT INTO context_sources (
id, workspace_id, type, name, description, connection_ref, status, config_json, metadata_json, created_by, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	type = excluded.type,
	name = excluded.name,
	description = excluded.description,
	connection_ref = excluded.connection_ref,
	status = excluded.status,
	config_json = excluded.config_json,
	metadata_json = excluded.metadata_json,
	updated_at = excluded.updated_at`,
		source.ID, source.WorkspaceID, source.Type, source.Name, source.Description, source.ConnectionRef, source.Status,
		defaultJSONObject(source.ConfigJSON), defaultJSONObject(source.MetadataJSON), source.CreatedBy, source.CreatedAt, source.UpdatedAt)
	return err
}

func (db *SQLiteStore) ContextSourceByID(workspaceID, id string) (ContextSource, bool, error) {
	row := db.sql.QueryRow(contextSourceSelectSQL()+` WHERE workspace_id = ? AND id = ?`, strings.TrimSpace(workspaceID), strings.TrimSpace(id))
	source, err := scanContextSource(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ContextSource{}, false, nil
	}
	if err != nil {
		return ContextSource{}, false, err
	}
	return source, true, nil
}

func (db *SQLiteStore) ListContextSources(filter ContextSourceFilter) ([]ContextSource, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := contextSourceSelectSQL() + ` WHERE workspace_id = ?`
	args := []any{strings.TrimSpace(filter.WorkspaceID)}
	if strings.TrimSpace(filter.Type) != "" {
		query += ` AND type = ?`
		args = append(args, strings.TrimSpace(filter.Type))
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
	out := []ContextSource{}
	for rows.Next() {
		source, err := scanContextSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, source)
	}
	return out, rows.Err()
}

func (db *SQLiteStore) UpsertContextItem(item ContextItem) error {
	if item.CollectedAt == "" {
		item.CollectedAt = nowUTC()
	}
	if item.CreatedAt == "" {
		item.CreatedAt = item.CollectedAt
	}
	if item.UpdatedAt == "" {
		item.UpdatedAt = item.CreatedAt
	}
	if item.Status == "" {
		item.Status = "active"
	}
	if item.Sensitivity == "" {
		item.Sensitivity = "L2"
	}
	if item.PayloadJSON == "" {
		item.PayloadJSON = "{}"
	}
	if item.LabelsJSON == "" {
		item.LabelsJSON = "{}"
	}
	query := `INSERT INTO context_items (
id, workspace_id, source_id, source_type, source_item_id, source_url, project_id, agent_worker_id,
author_type, author_id, occurred_at, collected_at, title, summary, content_text, content_ref,
payload_json, labels_json, sensitivity, status, dedupe_key, acl_policy_id, retention, expires_at,
last_used_at, usage_count, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	source_id = excluded.source_id,
	source_type = excluded.source_type,
	source_item_id = excluded.source_item_id,
	source_url = excluded.source_url,
	project_id = excluded.project_id,
	agent_worker_id = excluded.agent_worker_id,
	author_type = excluded.author_type,
	author_id = excluded.author_id,
	occurred_at = excluded.occurred_at,
	collected_at = excluded.collected_at,
	title = excluded.title,
	summary = excluded.summary,
	content_text = excluded.content_text,
	content_ref = excluded.content_ref,
	payload_json = excluded.payload_json,
	labels_json = excluded.labels_json,
	sensitivity = excluded.sensitivity,
	status = excluded.status,
	dedupe_key = excluded.dedupe_key,
	acl_policy_id = excluded.acl_policy_id,
	retention = excluded.retention,
	expires_at = excluded.expires_at,
	updated_at = excluded.updated_at`
	if strings.TrimSpace(item.DedupeKey) != "" {
		query += `
ON CONFLICT(workspace_id, dedupe_key) WHERE dedupe_key != '' DO UPDATE SET
	source_id = excluded.source_id,
	source_type = excluded.source_type,
	source_item_id = excluded.source_item_id,
	source_url = excluded.source_url,
	project_id = excluded.project_id,
	agent_worker_id = excluded.agent_worker_id,
	author_type = excluded.author_type,
	author_id = excluded.author_id,
	occurred_at = excluded.occurred_at,
	collected_at = excluded.collected_at,
	title = excluded.title,
	summary = excluded.summary,
	content_text = excluded.content_text,
	content_ref = excluded.content_ref,
	payload_json = excluded.payload_json,
	labels_json = excluded.labels_json,
	sensitivity = excluded.sensitivity,
	status = excluded.status,
	acl_policy_id = excluded.acl_policy_id,
	retention = excluded.retention,
	expires_at = excluded.expires_at,
	updated_at = excluded.updated_at`
	}
	_, err := db.sql.Exec(query,
		item.ID, item.WorkspaceID, item.SourceID, item.SourceType, item.SourceItemID, item.SourceURL, item.ProjectID, item.AgentWorkerID,
		item.AuthorType, item.AuthorID, item.OccurredAt, item.CollectedAt, item.Title, item.Summary, item.ContentText, item.ContentRef,
		defaultJSONObject(item.PayloadJSON), defaultJSONObject(item.LabelsJSON), item.Sensitivity, item.Status, item.DedupeKey, item.ACLPolicyID,
		item.Retention, item.ExpiresAt, item.LastUsedAt, item.UsageCount, item.CreatedAt, item.UpdatedAt)
	return err
}

func (db *SQLiteStore) ContextItemByID(workspaceID, id string) (ContextItem, bool, error) {
	row := db.sql.QueryRow(contextItemSelectSQL()+` WHERE workspace_id = ? AND id = ?`, strings.TrimSpace(workspaceID), strings.TrimSpace(id))
	item, err := scanContextItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ContextItem{}, false, nil
	}
	if err != nil {
		return ContextItem{}, false, err
	}
	if _, err := db.sql.Exec(`UPDATE context_items SET last_used_at = ?, usage_count = usage_count + 1 WHERE workspace_id = ? AND id = ?`,
		nowUTC(), strings.TrimSpace(workspaceID), strings.TrimSpace(id)); err != nil {
		return ContextItem{}, false, err
	}
	return item, true, nil
}

func (db *SQLiteStore) ListContextItems(filter ContextItemFilter) ([]ContextItem, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := contextItemSelectSQL() + ` WHERE workspace_id = ?`
	args := []any{strings.TrimSpace(filter.WorkspaceID)}
	if strings.TrimSpace(filter.SourceID) != "" {
		query += ` AND source_id = ?`
		args = append(args, strings.TrimSpace(filter.SourceID))
	}
	if strings.TrimSpace(filter.SourceType) != "" {
		query += ` AND source_type = ?`
		args = append(args, strings.TrimSpace(filter.SourceType))
	}
	if strings.TrimSpace(filter.ProjectID) != "" {
		query += ` AND project_id = ?`
		args = append(args, strings.TrimSpace(filter.ProjectID))
	}
	if strings.TrimSpace(filter.AgentWorkerID) != "" {
		query += ` AND agent_worker_id = ?`
		args = append(args, strings.TrimSpace(filter.AgentWorkerID))
	}
	if strings.TrimSpace(filter.Status) != "" {
		query += ` AND status = ?`
		args = append(args, strings.TrimSpace(filter.Status))
	}
	if strings.TrimSpace(filter.Since) != "" {
		query += ` AND collected_at >= ?`
		args = append(args, strings.TrimSpace(filter.Since))
	}
	if q := strings.TrimSpace(filter.Query); q != "" {
		like := "%" + q + "%"
		query += ` AND (title LIKE ? OR summary LIKE ? OR content_text LIKE ? OR payload_json LIKE ? OR labels_json LIKE ?)`
		args = append(args, like, like, like, like, like)
	}
	query += ` ORDER BY collected_at DESC, created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := db.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ContextItem{}
	for rows.Next() {
		item, err := scanContextItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (db *SQLiteStore) UpsertContextSubscription(sub ContextSubscription) error {
	if sub.CreatedAt == "" {
		sub.CreatedAt = nowUTC()
	}
	if sub.UpdatedAt == "" {
		sub.UpdatedAt = sub.CreatedAt
	}
	if sub.Status == "" {
		sub.Status = "active"
	}
	if sub.MaxSensitivity == "" {
		sub.MaxSensitivity = "L2"
	}
	if sub.DeliveryMode == "" {
		sub.DeliveryMode = "searchable"
	}
	if sub.SourceIDsJSON == "" {
		sub.SourceIDsJSON = "[]"
	}
	if sub.LabelFilterJSON == "" {
		sub.LabelFilterJSON = "{}"
	}
	if sub.SignalRuleJSON == "" {
		sub.SignalRuleJSON = "{}"
	}
	_, err := db.sql.Exec(`INSERT INTO context_subscriptions (
id, workspace_id, subscriber_type, subscriber_id, source_ids_json, label_filter_json,
max_sensitivity, delivery_mode, signal_rule_json, status, created_by, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	subscriber_type = excluded.subscriber_type,
	subscriber_id = excluded.subscriber_id,
	source_ids_json = excluded.source_ids_json,
	label_filter_json = excluded.label_filter_json,
	max_sensitivity = excluded.max_sensitivity,
	delivery_mode = excluded.delivery_mode,
	signal_rule_json = excluded.signal_rule_json,
	status = excluded.status,
	updated_at = excluded.updated_at`,
		sub.ID, sub.WorkspaceID, sub.SubscriberType, sub.SubscriberID, defaultJSONArray(sub.SourceIDsJSON), defaultJSONObject(sub.LabelFilterJSON),
		sub.MaxSensitivity, sub.DeliveryMode, defaultJSONObject(sub.SignalRuleJSON), sub.Status, sub.CreatedBy, sub.CreatedAt, sub.UpdatedAt)
	return err
}

func (db *SQLiteStore) ContextSubscriptionByID(workspaceID, id string) (ContextSubscription, bool, error) {
	row := db.sql.QueryRow(contextSubscriptionSelectSQL()+` WHERE workspace_id = ? AND id = ?`, strings.TrimSpace(workspaceID), strings.TrimSpace(id))
	sub, err := scanContextSubscription(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ContextSubscription{}, false, nil
	}
	if err != nil {
		return ContextSubscription{}, false, err
	}
	return sub, true, nil
}

func (db *SQLiteStore) ListContextSubscriptions(filter ContextSubscriptionFilter) ([]ContextSubscription, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := contextSubscriptionSelectSQL() + ` WHERE workspace_id = ?`
	args := []any{strings.TrimSpace(filter.WorkspaceID)}
	if strings.TrimSpace(filter.SubscriberType) != "" {
		query += ` AND subscriber_type = ?`
		args = append(args, strings.TrimSpace(filter.SubscriberType))
	}
	if strings.TrimSpace(filter.SubscriberID) != "" {
		query += ` AND subscriber_id = ?`
		args = append(args, strings.TrimSpace(filter.SubscriberID))
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
	out := []ContextSubscription{}
	for rows.Next() {
		sub, err := scanContextSubscription(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

func contextSourceSelectSQL() string {
	return `SELECT id, workspace_id, type, name, description, connection_ref, status, config_json, metadata_json, created_by, created_at, updated_at FROM context_sources`
}

func contextItemSelectSQL() string {
	return `SELECT id, workspace_id, source_id, source_type, source_item_id, source_url, project_id, agent_worker_id,
author_type, author_id, occurred_at, collected_at, title, summary, content_text, content_ref,
payload_json, labels_json, sensitivity, status, dedupe_key, acl_policy_id, retention, expires_at,
last_used_at, usage_count, created_at, updated_at FROM context_items`
}

func contextSubscriptionSelectSQL() string {
	return `SELECT id, workspace_id, subscriber_type, subscriber_id, source_ids_json, label_filter_json,
max_sensitivity, delivery_mode, signal_rule_json, status, created_by, created_at, updated_at FROM context_subscriptions`
}

type contextSourceScanner interface{ Scan(dest ...any) error }
type contextItemScanner interface{ Scan(dest ...any) error }
type contextSubscriptionScanner interface{ Scan(dest ...any) error }

func scanContextSource(row contextSourceScanner) (ContextSource, error) {
	var s ContextSource
	err := row.Scan(&s.ID, &s.WorkspaceID, &s.Type, &s.Name, &s.Description, &s.ConnectionRef, &s.Status,
		&s.ConfigJSON, &s.MetadataJSON, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

func scanContextItem(row contextItemScanner) (ContextItem, error) {
	var item ContextItem
	err := row.Scan(&item.ID, &item.WorkspaceID, &item.SourceID, &item.SourceType, &item.SourceItemID, &item.SourceURL,
		&item.ProjectID, &item.AgentWorkerID, &item.AuthorType, &item.AuthorID, &item.OccurredAt, &item.CollectedAt,
		&item.Title, &item.Summary, &item.ContentText, &item.ContentRef, &item.PayloadJSON, &item.LabelsJSON,
		&item.Sensitivity, &item.Status, &item.DedupeKey, &item.ACLPolicyID, &item.Retention, &item.ExpiresAt,
		&item.LastUsedAt, &item.UsageCount, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func scanContextSubscription(row contextSubscriptionScanner) (ContextSubscription, error) {
	var sub ContextSubscription
	err := row.Scan(&sub.ID, &sub.WorkspaceID, &sub.SubscriberType, &sub.SubscriberID, &sub.SourceIDsJSON, &sub.LabelFilterJSON,
		&sub.MaxSensitivity, &sub.DeliveryMode, &sub.SignalRuleJSON, &sub.Status, &sub.CreatedBy, &sub.CreatedAt, &sub.UpdatedAt)
	return sub, err
}
