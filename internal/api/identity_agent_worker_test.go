package api

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/multigent/multigent/internal/agentdir"
	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/store"
)

func TestValidateIdentityResolvesAgentWorkerMembership(t *testing.T) {
	db, err := controldb.Open(filepath.Join(t.TempDir(), "multigent.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	root := t.TempDir()
	workspaceID := "ws-api-agent-worker"
	now := time.Now().UTC().Format(time.RFC3339)
	if err := db.UpsertWorkspace(controldb.Workspace{
		ID:        workspaceID,
		Name:      "API Agent Worker",
		Slug:      "api-agent-worker",
		Root:      root,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert workspace: %v", err)
	}
	worker := controldb.AgentWorker{
		ID:          "aw-manager-agent",
		WorkspaceID: workspaceID,
		Name:        "manager-agent",
		DisplayName: "manager-agent",
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := db.UpsertAgentWorker(worker); err != nil {
		t.Fatalf("upsert worker: %v", err)
	}
	if err := db.UpsertProjectMembership(controldb.ProjectMembership{
		ID:               "pm-manager-agent",
		WorkspaceID:      workspaceID,
		ProjectID:        "customer-cli",
		MemberType:       agentdir.MemberTypeAgentWorker,
		MemberID:         worker.ID,
		Role:             "pm",
		Title:            "pm",
		AutoPickTasks:    true,
		AttentionEnabled: true,
		PriorityWeight:   1,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("upsert membership: %v", err)
	}

	s := &Server{
		root:           root,
		controlDB:      db,
		st:             store.NewFS(root),
		users:          newUserStore(db),
		agentDirectory: agentdir.New(db),
	}
	if err := s.validateIdentity("customer-cli/pm", "assignee"); err != nil {
		t.Fatalf("validate identity: %v", err)
	}
	if !s.agentExistsInProject("customer-cli", "pm") {
		t.Fatalf("membership-backed agent should exist")
	}
	if s.agentExistsInProject("customer-cli", "missing") {
		t.Fatalf("missing membership should not exist")
	}
	if got := s.identityLabel("aw-manager-agent"); got != "manager-agent" {
		t.Fatalf("worker ID should resolve to display name, got %q", got)
	}
	if got := s.identityLabel("customer-cli/pm"); got != "manager-agent" {
		t.Fatalf("project worker mailbox should resolve to display name, got %q", got)
	}
}
