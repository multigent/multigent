package db

func (db *SQLiteStore) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS workspaces (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	slug TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	root TEXT NOT NULL UNIQUE,
	created_by TEXT NOT NULL DEFAULT 'system',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL DEFAULT '',
	last_opened_at TEXT NOT NULL DEFAULT ''
)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_workspaces_slug ON workspaces(slug)`,
		`CREATE TABLE IF NOT EXISTS users (
	username TEXT PRIMARY KEY,
	email TEXT UNIQUE,
	display_name TEXT NOT NULL DEFAULT '',
	role TEXT NOT NULL DEFAULT 'member',
	avatar TEXT NOT NULL DEFAULT '',
	phone TEXT NOT NULL DEFAULT '',
	bio TEXT NOT NULL DEFAULT '',
	password_hash TEXT NOT NULL DEFAULT '',
	disabled INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL,
	projects_json TEXT NOT NULL DEFAULT '[]',
	linked_agents_json TEXT NOT NULL DEFAULT '[]'
)`,
		`ALTER TABLE users ADD COLUMN projects_json TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE users ADD COLUMN linked_agents_json TEXT NOT NULL DEFAULT '[]'`,
		`CREATE TABLE IF NOT EXISTS workspace_members (
	workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
	username TEXT NOT NULL REFERENCES users(username) ON DELETE CASCADE,
	role TEXT NOT NULL DEFAULT 'member',
	created_at TEXT NOT NULL,
	PRIMARY KEY (workspace_id, username)
)`,
		`CREATE INDEX IF NOT EXISTS idx_workspace_members_username ON workspace_members(username)`,
		`CREATE TABLE IF NOT EXISTS settings (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS invitations (
	token TEXT PRIMARY KEY,
	email TEXT NOT NULL,
	role TEXT NOT NULL,
	display_name TEXT NOT NULL DEFAULT '',
	projects_json TEXT NOT NULL DEFAULT '[]',
	linked_agents_json TEXT NOT NULL DEFAULT '[]',
	invited_by TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL,
	created_at TEXT NOT NULL,
	expires_at TEXT NOT NULL,
	accepted_at TEXT NOT NULL DEFAULT ''
)`,
		`CREATE INDEX IF NOT EXISTS idx_invitations_email_status ON invitations(email, status)`,
		`CREATE TABLE IF NOT EXISTS kv_records (
	table_name TEXT NOT NULL,
	workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
	k1 TEXT NOT NULL DEFAULT '',
	k2 TEXT NOT NULL DEFAULT '',
	k3 TEXT NOT NULL DEFAULT '',
	payload TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	PRIMARY KEY (table_name, workspace_id, k1, k2, k3)
)`,
		`CREATE INDEX IF NOT EXISTS idx_kv_records_lookup ON kv_records(table_name, workspace_id, k1, k2)`,
	}
	for _, stmt := range stmts {
		if _, err := db.sql.Exec(stmt); err != nil {
			if len(stmt) > 5 && stmt[:5] == "ALTER" {
				continue
			}
			return err
		}
	}
	return nil
}
