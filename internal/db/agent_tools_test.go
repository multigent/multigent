package db

import (
	"path/filepath"
	"testing"
)

func TestUpsertAgentToolBindingBackfillsLegacyProjectBinding(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "multigent.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	workspaceID := "ws-tool-bindings"
	if err := db.UpsertWorkspace(Workspace{
		ID:        workspaceID,
		Name:      "Tool Bindings",
		Slug:      "tool-bindings",
		Root:      t.TempDir(),
		CreatedAt: nowUTC(),
	}); err != nil {
		t.Fatalf("upsert workspace: %v", err)
	}
	if err := db.UpsertConnection(Connection{
		ID:             "conn-github",
		WorkspaceID:    workspaceID,
		Provider:       "github",
		ConnectionName: "default",
		OwnerType:      "workspace",
		OwnerID:        workspaceID,
		AuthType:       "api_key",
		Status:         "active",
		CreatedAt:      nowUTC(),
	}); err != nil {
		t.Fatalf("upsert connection: %v", err)
	}

	legacy := AgentToolBinding{
		ID:           "toolbind-legacy",
		WorkspaceID:  workspaceID,
		ProjectID:    "tapnow-mcp-server",
		AgentID:      "mason",
		ConnectionID: "conn-github",
		Provider:     "github",
		Status:       "enabled",
		ConfigJSON:   "{}",
		CreatedBy:    "admin",
		CreatedAt:    nowUTC(),
	}
	if err := db.UpsertAgentToolBinding(legacy); err != nil {
		t.Fatalf("upsert legacy binding: %v", err)
	}

	next := legacy
	next.ID = "toolbind-worker"
	next.AgentWorkerID = "aw-mason"
	next.AdapterType = "http_action"
	if err := db.UpsertAgentToolBinding(next); err != nil {
		t.Fatalf("upsert worker binding should backfill legacy row: %v", err)
	}

	bindings, err := db.ListAgentToolBindings(AgentToolBindingFilter{WorkspaceID: workspaceID, ProjectID: "tapnow-mcp-server", AgentID: "mason"})
	if err != nil {
		t.Fatalf("list project bindings: %v", err)
	}
	if len(bindings) != 1 {
		t.Fatalf("expected one updated binding, got %+v", bindings)
	}
	if bindings[0].ID != legacy.ID || bindings[0].AgentWorkerID != "aw-mason" || bindings[0].AdapterType != "http_action" {
		t.Fatalf("legacy binding was not backfilled: %+v", bindings[0])
	}

	workerBindings, err := db.ListAgentToolBindings(AgentToolBindingFilter{WorkspaceID: workspaceID, AgentWorkerID: "aw-mason"})
	if err != nil {
		t.Fatalf("list worker bindings: %v", err)
	}
	if len(workerBindings) != 1 || workerBindings[0].ID != legacy.ID {
		t.Fatalf("unexpected worker bindings: %+v", workerBindings)
	}
}
