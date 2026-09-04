package db

import (
	"path/filepath"
	"testing"
)

func TestUpsertAgentToolBindingUsesWorkerIdentity(t *testing.T) {
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

	for _, workerID := range []string{"aw-developer-a", "aw-developer-b"} {
		if err := db.UpsertAgentToolBinding(AgentToolBinding{
			ID:            "toolbind-" + workerID,
			WorkspaceID:   workspaceID,
			AgentWorkerID: workerID,
			ProjectID:     "ignored-project",
			AgentID:       "ignored-agent",
			ConnectionID:  "conn-github",
			Provider:      "github",
			AdapterType:   "http_action",
			Status:        "enabled",
			ConfigJSON:    "{}",
			CreatedBy:     "admin",
			CreatedAt:     nowUTC(),
		}); err != nil {
			t.Fatalf("upsert worker binding %s: %v", workerID, err)
		}
	}

	workerBindings, err := db.ListAgentToolBindings(AgentToolBindingFilter{WorkspaceID: workspaceID})
	if err != nil {
		t.Fatalf("list worker bindings: %v", err)
	}
	if len(workerBindings) != 2 {
		t.Fatalf("unexpected worker bindings: %+v", workerBindings)
	}
	for _, binding := range workerBindings {
		if binding.ProjectID != "" || binding.AgentID != "" {
			t.Fatalf("worker binding retained project identity: %+v", binding)
		}
	}
}
