package db

import (
	"path/filepath"
	"testing"
)

func TestMigrateAgentToolBindingsToWorkerIdentity(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "multigent.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer store.Close()

	workspaceID := "ws-binding-migration"
	if err := store.UpsertWorkspace(Workspace{
		ID:        workspaceID,
		Name:      "Binding migration",
		Slug:      "binding-migration",
		Root:      t.TempDir(),
		CreatedAt: nowUTC(),
	}); err != nil {
		t.Fatalf("workspace: %v", err)
	}
	if err := store.UpsertAgentWorker(AgentWorker{
		ID:          "aw-pm",
		WorkspaceID: workspaceID,
		Name:        "pm",
		DisplayName: "PM",
		Status:      "active",
		CreatedAt:   nowUTC(),
		UpdatedAt:   nowUTC(),
	}); err != nil {
		t.Fatalf("worker: %v", err)
	}
	for _, projectID := range []string{"project-a", "project-b"} {
		if err := store.UpsertProjectMembership(ProjectMembership{
			ID:          "membership-" + projectID,
			WorkspaceID: workspaceID,
			ProjectID:   projectID,
			MemberType:  "agent_worker",
			MemberID:    "aw-pm",
			Title:       "pm",
			CreatedAt:   nowUTC(),
			UpdatedAt:   nowUTC(),
		}); err != nil {
			t.Fatalf("membership %s: %v", projectID, err)
		}
	}
	if err := store.UpsertConnection(Connection{
		ID:             "conn-github",
		WorkspaceID:    workspaceID,
		Provider:       "github",
		ConnectionName: "default",
		OwnerType:      "workspace",
		OwnerID:        workspaceID,
		AuthType:       "api_key",
		Status:         "active",
		CreatedAt:      nowUTC(),
		UpdatedAt:      nowUTC(),
	}); err != nil {
		t.Fatalf("connection: %v", err)
	}

	if _, err := store.sql.Exec(`DROP TABLE agent_tool_bindings`); err != nil {
		t.Fatalf("drop current table: %v", err)
	}
	if _, err := store.sql.Exec(`CREATE TABLE agent_tool_bindings (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
		agent_worker_id TEXT NOT NULL DEFAULT '',
		project_id TEXT NOT NULL,
		agent_id TEXT NOT NULL,
		connection_id TEXT NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
		provider TEXT NOT NULL,
		adapter_type TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'enabled',
		config_json TEXT NOT NULL DEFAULT '{}',
		created_by TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL DEFAULT '',
		UNIQUE(workspace_id, project_id, agent_id, connection_id)
	)`); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	for _, row := range []struct {
		id        string
		projectID string
		updatedAt string
	}{
		{id: "binding-old-a", projectID: "project-a", updatedAt: "2026-09-04T01:00:00Z"},
		{id: "binding-old-b", projectID: "project-b", updatedAt: "2026-09-04T02:00:00Z"},
	} {
		if _, err := store.sql.Exec(`INSERT INTO agent_tool_bindings (
			id, workspace_id, project_id, agent_id, connection_id, provider, adapter_type,
			status, config_json, created_by, created_at, updated_at
		) VALUES (?, ?, ?, 'pm', 'conn-github', 'github', 'http_action', 'enabled', '{}', 'admin', ?, ?)`,
			row.id, workspaceID, row.projectID, row.updatedAt, row.updatedAt); err != nil {
			t.Fatalf("insert legacy binding %s: %v", row.id, err)
		}
	}

	if err := store.migrateAgentToolBindingsSchema(); err != nil {
		t.Fatalf("migrate bindings: %v", err)
	}
	bindings, err := store.ListAgentToolBindings(AgentToolBindingFilter{WorkspaceID: workspaceID})
	if err != nil {
		t.Fatalf("list migrated bindings: %v", err)
	}
	if len(bindings) != 1 {
		t.Fatalf("expected one deduplicated binding, got %+v", bindings)
	}
	if bindings[0].ID != "binding-old-b" || bindings[0].AgentWorkerID != "aw-pm" || bindings[0].ProjectID != "" || bindings[0].AgentID != "" {
		t.Fatalf("unexpected migrated binding: %+v", bindings[0])
	}

	if err := store.migrateAgentToolBindingsSchema(); err != nil {
		t.Fatalf("repeat migration: %v", err)
	}
	bindings, err = store.ListAgentToolBindings(AgentToolBindingFilter{WorkspaceID: workspaceID})
	if err != nil || len(bindings) != 1 {
		t.Fatalf("repeat migration changed bindings: bindings=%+v err=%v", bindings, err)
	}
}
