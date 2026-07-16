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
		`CREATE TABLE IF NOT EXISTS audit_events (
	id TEXT PRIMARY KEY,
	workspace_id TEXT NOT NULL DEFAULT '',
	actor_type TEXT NOT NULL,
	actor_id TEXT NOT NULL,
	action TEXT NOT NULL,
	resource_type TEXT NOT NULL,
	resource_id TEXT NOT NULL,
	summary TEXT NOT NULL DEFAULT '',
	before_json TEXT NOT NULL DEFAULT '',
	after_json TEXT NOT NULL DEFAULT '',
	ip TEXT NOT NULL DEFAULT '',
	user_agent TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_events_workspace_time ON audit_events(workspace_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_events_resource ON audit_events(resource_type, resource_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS connector_providers (
	provider TEXT PRIMARY KEY,
	display_name TEXT NOT NULL,
	auth_types_json TEXT NOT NULL DEFAULT '[]',
	catalog_json TEXT NOT NULL DEFAULT '{}',
	enabled INTEGER NOT NULL DEFAULT 1,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL DEFAULT ''
)`,
		`CREATE INDEX IF NOT EXISTS idx_connector_providers_enabled ON connector_providers(enabled, provider)`,
		`CREATE TABLE IF NOT EXISTS oauth_client_configs (
	workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
	provider TEXT NOT NULL,
	client_id TEXT NOT NULL,
	secret_ciphertext TEXT NOT NULL DEFAULT '',
	nonce TEXT NOT NULL DEFAULT '',
	key_version TEXT NOT NULL DEFAULT '',
	extra_json TEXT NOT NULL DEFAULT '{}',
	created_by TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (workspace_id, provider)
)`,
		`CREATE INDEX IF NOT EXISTS idx_oauth_client_configs_workspace ON oauth_client_configs(workspace_id, provider)`,
		`CREATE TABLE IF NOT EXISTS connections (
	id TEXT PRIMARY KEY,
	workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
	provider TEXT NOT NULL,
	connection_name TEXT NOT NULL DEFAULT 'default',
	owner_type TEXT NOT NULL,
	owner_id TEXT NOT NULL,
	auth_type TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'active',
	profile_json TEXT NOT NULL DEFAULT '{}',
	created_by TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL DEFAULT '',
	last_used_at TEXT NOT NULL DEFAULT '',
	UNIQUE(workspace_id, provider, owner_type, owner_id, connection_name)
)`,
		`CREATE INDEX IF NOT EXISTS idx_connections_workspace_provider ON connections(workspace_id, provider)`,
		`CREATE INDEX IF NOT EXISTS idx_connections_owner ON connections(workspace_id, owner_type, owner_id)`,
		`CREATE TABLE IF NOT EXISTS connection_secrets (
	connection_id TEXT PRIMARY KEY REFERENCES connections(id) ON DELETE CASCADE,
	ciphertext TEXT NOT NULL,
	nonce TEXT NOT NULL DEFAULT '',
	key_version TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS connection_grants (
	id TEXT PRIMARY KEY,
	workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
	connection_id TEXT NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
	target_type TEXT NOT NULL,
	target_id TEXT NOT NULL,
	created_by TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	UNIQUE(connection_id, target_type, target_id)
)`,
		`CREATE INDEX IF NOT EXISTS idx_connection_grants_target ON connection_grants(workspace_id, target_type, target_id)`,
		`CREATE TABLE IF NOT EXISTS agent_channel_bindings (
	id TEXT PRIMARY KEY,
	workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
	project_id TEXT NOT NULL,
	agent_id TEXT NOT NULL,
	provider TEXT NOT NULL,
	connection_id TEXT NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
	external_bot_id TEXT NOT NULL DEFAULT '',
	external_chat_id TEXT NOT NULL DEFAULT '',
	external_owner_id TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'connected',
	metadata_json TEXT NOT NULL DEFAULT '{}',
	created_by TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL DEFAULT '',
	last_activity_at TEXT NOT NULL DEFAULT '',
	UNIQUE(workspace_id, project_id, agent_id, provider)
)`,
		`CREATE INDEX IF NOT EXISTS idx_agent_channel_bindings_agent ON agent_channel_bindings(workspace_id, project_id, agent_id)`,
		`CREATE INDEX IF NOT EXISTS idx_agent_channel_bindings_connection ON agent_channel_bindings(connection_id)`,
		`CREATE TABLE IF NOT EXISTS external_identities (
	id TEXT PRIMARY KEY,
	workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
	provider TEXT NOT NULL,
	external_user_id TEXT NOT NULL,
	user_id TEXT NOT NULL REFERENCES users(username) ON DELETE CASCADE,
	metadata_json TEXT NOT NULL DEFAULT '{}',
	created_by TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL DEFAULT '',
	UNIQUE(workspace_id, provider, external_user_id)
)`,
		`CREATE INDEX IF NOT EXISTS idx_external_identities_user ON external_identities(workspace_id, user_id)`,
		`CREATE TABLE IF NOT EXISTS interactive_sessions (
	id TEXT PRIMARY KEY,
	workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
	project_id TEXT NOT NULL,
	agent_id TEXT NOT NULL,
	source_kind TEXT NOT NULL,
	source_channel TEXT NOT NULL DEFAULT '',
	actor_type TEXT NOT NULL DEFAULT '',
	actor_id TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'active',
	lock_reason TEXT NOT NULL DEFAULT 'interactive',
	runtime_session_id TEXT NOT NULL DEFAULT '',
	current_run_id TEXT NOT NULL DEFAULT '',
	human_intervened INTEGER NOT NULL DEFAULT 0,
	metadata_json TEXT NOT NULL DEFAULT '{}',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL DEFAULT '',
	last_activity_at TEXT NOT NULL DEFAULT '',
	completed_at TEXT NOT NULL DEFAULT ''
)`,
		`CREATE INDEX IF NOT EXISTS idx_interactive_sessions_agent ON interactive_sessions(workspace_id, project_id, agent_id, status)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_interactive_sessions_one_active_agent ON interactive_sessions(workspace_id, project_id, agent_id) WHERE status IN ('active', 'waiting_input')`,
		`CREATE TABLE IF NOT EXISTS interaction_events (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL REFERENCES interactive_sessions(id) ON DELETE CASCADE,
	workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
	actor_type TEXT NOT NULL,
	actor_id TEXT NOT NULL,
	channel TEXT NOT NULL DEFAULT '',
	event_type TEXT NOT NULL,
	content TEXT NOT NULL DEFAULT '',
	metadata_json TEXT NOT NULL DEFAULT '{}',
	created_at TEXT NOT NULL
)`,
		`CREATE INDEX IF NOT EXISTS idx_interaction_events_session_time ON interaction_events(session_id, created_at ASC)`,
		`CREATE INDEX IF NOT EXISTS idx_interaction_events_workspace_time ON interaction_events(workspace_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS model_providers (
	id TEXT NOT NULL,
	workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
	owner_type TEXT NOT NULL DEFAULT 'workspace',
	owner_id TEXT NOT NULL DEFAULT '',
	name TEXT NOT NULL,
	type TEXT NOT NULL,
	base_url TEXT NOT NULL DEFAULT '',
	api_key TEXT NOT NULL DEFAULT '',
	model TEXT NOT NULL DEFAULT '',
	env_json TEXT NOT NULL DEFAULT '{}',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (workspace_id, id)
)`,
		`ALTER TABLE model_providers ADD COLUMN owner_type TEXT NOT NULL DEFAULT 'workspace'`,
		`ALTER TABLE model_providers ADD COLUMN owner_id TEXT NOT NULL DEFAULT ''`,
		`UPDATE model_providers SET owner_id = workspace_id WHERE owner_type = 'workspace' AND owner_id = ''`,
		`CREATE INDEX IF NOT EXISTS idx_model_providers_workspace ON model_providers(workspace_id, name)`,
		`CREATE INDEX IF NOT EXISTS idx_model_providers_owner ON model_providers(workspace_id, owner_type, owner_id)`,
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
