package db

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
)

func TestKnowledgeBaseMigrationCopiesLegacyContextTables(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "multigent.db")

	store, err := Open(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.UpsertWorkspace(Workspace{
		ID:        "ws-kb-migration",
		Name:      "KB Migration",
		Slug:      "kb-migration",
		Root:      dir,
		CreatedAt: nowUTC(),
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close seeded db: %v", err)
	}

	raw, err := sql.Open("sqlite", sqliteURI(path))
	if err != nil {
		t.Fatalf("open raw sqlite: %v", err)
	}
	if _, err := raw.Exec(`CREATE TABLE IF NOT EXISTS context_sources (
	id TEXT PRIMARY KEY,
	workspace_id TEXT NOT NULL,
	type TEXT NOT NULL,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	connection_ref TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'active',
	config_json TEXT NOT NULL DEFAULT '{}',
	metadata_json TEXT NOT NULL DEFAULT '{}',
	created_by TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL DEFAULT ''
)`); err != nil {
		t.Fatalf("create legacy sources: %v", err)
	}
	if _, err := raw.Exec(`CREATE TABLE IF NOT EXISTS context_items (
	id TEXT PRIMARY KEY,
	workspace_id TEXT NOT NULL,
	source_id TEXT NOT NULL DEFAULT '',
	source_type TEXT NOT NULL,
	source_item_id TEXT NOT NULL DEFAULT '',
	source_url TEXT NOT NULL DEFAULT '',
	project_id TEXT NOT NULL DEFAULT '',
	agent_worker_id TEXT NOT NULL DEFAULT '',
	author_type TEXT NOT NULL DEFAULT '',
	author_id TEXT NOT NULL DEFAULT '',
	occurred_at TEXT NOT NULL DEFAULT '',
	collected_at TEXT NOT NULL,
	title TEXT NOT NULL DEFAULT '',
	summary TEXT NOT NULL DEFAULT '',
	content_text TEXT NOT NULL DEFAULT '',
	content_ref TEXT NOT NULL DEFAULT '',
	payload_json TEXT NOT NULL DEFAULT '{}',
	labels_json TEXT NOT NULL DEFAULT '{}',
	sensitivity TEXT NOT NULL DEFAULT 'L2',
	status TEXT NOT NULL DEFAULT 'active',
	dedupe_key TEXT NOT NULL DEFAULT '',
	acl_policy_id TEXT NOT NULL DEFAULT '',
	retention TEXT NOT NULL DEFAULT '',
	expires_at TEXT NOT NULL DEFAULT '',
	last_used_at TEXT NOT NULL DEFAULT '',
	usage_count INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL DEFAULT ''
)`); err != nil {
		t.Fatalf("create legacy items: %v", err)
	}
	if _, err := raw.Exec(`CREATE TABLE IF NOT EXISTS context_subscriptions (
	id TEXT PRIMARY KEY,
	workspace_id TEXT NOT NULL,
	subscriber_type TEXT NOT NULL,
	subscriber_id TEXT NOT NULL,
	source_ids_json TEXT NOT NULL DEFAULT '[]',
	label_filter_json TEXT NOT NULL DEFAULT '{}',
	max_sensitivity TEXT NOT NULL DEFAULT 'L2',
	delivery_mode TEXT NOT NULL DEFAULT 'searchable',
	signal_rule_json TEXT NOT NULL DEFAULT '{}',
	status TEXT NOT NULL DEFAULT 'active',
	created_by TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL DEFAULT ''
)`); err != nil {
		t.Fatalf("create legacy subscriptions: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO context_sources (
id, workspace_id, type, name, description, connection_ref, status, config_json, metadata_json, created_by, created_at, updated_at
) VALUES ('ctxsrc-old', 'ws-kb-migration', 'lark_im', 'Legacy Source', 'old', '', 'active', '{}', '{}', 'admin', '2026-08-29T00:00:00Z', '2026-08-29T00:00:00Z')`); err != nil {
		t.Fatalf("seed legacy source: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO context_items (
id, workspace_id, source_id, source_type, source_item_id, source_url, project_id, agent_worker_id,
author_type, author_id, occurred_at, collected_at, title, summary, content_text, content_ref,
payload_json, labels_json, sensitivity, status, dedupe_key, acl_policy_id, retention, expires_at,
last_used_at, usage_count, created_at, updated_at
) VALUES ('ctxitem-old', 'ws-kb-migration', 'ctxsrc-old', 'lark_im', 'msg-1', '', '', '',
'user', 'u1', '2026-08-29T00:00:00Z', '2026-08-29T00:00:00Z', 'Legacy Item', 'legacy summary', 'legacy content', '',
'{}', '{}', 'L2', 'active', 'legacy-dedupe', '', '', '', '', 0, '2026-08-29T00:00:00Z', '2026-08-29T00:00:00Z')`); err != nil {
		t.Fatalf("seed legacy item: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO context_subscriptions (
id, workspace_id, subscriber_type, subscriber_id, source_ids_json, label_filter_json, max_sensitivity, delivery_mode, signal_rule_json, status, created_by, created_at, updated_at
) VALUES ('ctxsub-old', 'ws-kb-migration', 'agent_worker', 'aw-old', '["ctxsrc-old"]', '{}', 'L2', 'searchable', '{}', 'active', 'admin', '2026-08-29T00:00:00Z', '2026-08-29T00:00:00Z')`); err != nil {
		t.Fatalf("seed legacy subscription: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw sqlite: %v", err)
	}

	store, err = Open(path)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer store.Close()

	sources, err := store.ListContextSources(ContextSourceFilter{WorkspaceID: "ws-kb-migration"})
	if err != nil {
		t.Fatalf("list migrated sources: %v", err)
	}
	if len(sources) != 1 || sources[0].ID != "ctxsrc-old" {
		t.Fatalf("unexpected migrated sources: %+v", sources)
	}

	items, err := store.ListContextItems(ContextItemFilter{WorkspaceID: "ws-kb-migration"})
	if err != nil {
		t.Fatalf("list migrated items: %v", err)
	}
	if len(items) != 1 || items[0].ID != "ctxitem-old" || items[0].Title != "Legacy Item" {
		t.Fatalf("unexpected migrated items: %+v", items)
	}

	subs, err := store.ListContextSubscriptions(ContextSubscriptionFilter{WorkspaceID: "ws-kb-migration"})
	if err != nil {
		t.Fatalf("list migrated subscriptions: %v", err)
	}
	if len(subs) != 1 || subs[0].ID != "ctxsub-old" {
		t.Fatalf("unexpected migrated subscriptions: %+v", subs)
	}

	var count int
	if err := store.sql.QueryRow(`SELECT COUNT(*) FROM knowledge_base_items WHERE workspace_id = ?`, "ws-kb-migration").Scan(&count); err != nil {
		t.Fatalf("count knowledge base items: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 migrated knowledge base item, got %d", count)
	}
}

func sqliteURI(path string) string {
	return fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)", filepath.ToSlash(path))
}
