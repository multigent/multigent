package agentdir

import (
	"path/filepath"
	"testing"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
)

func TestDirectoryResolvesProjectWorkers(t *testing.T) {
	db, err := controldb.Open(filepath.Join(t.TempDir(), "multigent.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	workspaceID := "ws-directory"
	now := time.Now().UTC().Format(time.RFC3339)
	if err := db.UpsertWorkspace(controldb.Workspace{
		ID:        workspaceID,
		Name:      "Directory Test",
		Slug:      "directory-test",
		Root:      t.TempDir(),
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert workspace: %v", err)
	}
	worker := controldb.AgentWorker{
		ID:          "aw-nova",
		WorkspaceID: workspaceID,
		Name:        "nova",
		DisplayName: "Nova",
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := db.UpsertAgentWorker(worker); err != nil {
		t.Fatalf("upsert worker: %v", err)
	}
	membership := controldb.ProjectMembership{
		ID:               "pm-nova-cli",
		WorkspaceID:      workspaceID,
		ProjectID:        "customer-cli",
		MemberType:       MemberTypeAgentWorker,
		MemberID:         worker.ID,
		Role:             "project-manager",
		Title:            "pm",
		AutoPickTasks:    true,
		AttentionEnabled: true,
		PriorityWeight:   1,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := db.UpsertProjectMembership(membership); err != nil {
		t.Fatalf("upsert membership: %v", err)
	}

	dir := New(db)
	gotWorker, ok, err := dir.Worker(workspaceID, "nova")
	if err != nil {
		t.Fatalf("resolve worker: %v", err)
	}
	if !ok || gotWorker.ID != worker.ID {
		t.Fatalf("unexpected worker: ok=%v worker=%+v", ok, gotWorker)
	}

	projectWorker, ok, err := dir.ProjectWorker(workspaceID, "customer-cli", "aw-nova")
	if err != nil {
		t.Fatalf("resolve project worker: %v", err)
	}
	if !ok || projectWorker.Membership.ID != membership.ID {
		t.Fatalf("unexpected project worker: ok=%v value=%+v", ok, projectWorker)
	}

	byMembership, ok, err := dir.ProjectWorkerByMembership(workspaceID, membership.ID)
	if err != nil {
		t.Fatalf("resolve by membership: %v", err)
	}
	if !ok || byMembership.Worker.ID != worker.ID {
		t.Fatalf("unexpected membership worker: ok=%v value=%+v", ok, byMembership)
	}

	resolved, ok, err := dir.ResolveProjectMailbox(workspaceID, "customer-cli/pm")
	if err != nil {
		t.Fatalf("resolve project mailbox: %v", err)
	}
	if !ok || resolved.Worker.ID != worker.ID || resolved.Membership.ID != membership.ID {
		t.Fatalf("unexpected project mailbox resolution: ok=%v value=%+v", ok, resolved)
	}
}

func TestProjectWorkerPrefersProjectMembershipTitleOverGlobalWorkerName(t *testing.T) {
	db, err := controldb.Open(filepath.Join(t.TempDir(), "multigent.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	workspaceID := "ws-directory"
	now := time.Now().UTC().Format(time.RFC3339)
	if err := db.UpsertWorkspace(controldb.Workspace{
		ID:        workspaceID,
		Name:      "Directory Test",
		Slug:      "directory-test",
		Root:      t.TempDir(),
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert workspace: %v", err)
	}
	globalWorker := controldb.AgentWorker{
		ID:          "aw-global-dev-codex",
		WorkspaceID: workspaceID,
		Name:        "dev-codex",
		DisplayName: "dev-codex",
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	projectWorker := controldb.AgentWorker{
		ID:                   "aw-cc-connect-dev-codex",
		WorkspaceID:          workspaceID,
		Name:                 "cc-connect-dev-codex",
		DisplayName:          "dev-codex",
		Status:               "active",
		DefaultRuntimeNodeID: "rnode-cc-connect",
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := db.UpsertAgentWorker(globalWorker); err != nil {
		t.Fatalf("upsert global worker: %v", err)
	}
	if err := db.UpsertAgentWorker(projectWorker); err != nil {
		t.Fatalf("upsert project worker: %v", err)
	}
	membership := controldb.ProjectMembership{
		ID:               "pm-cc-connect-dev-codex",
		WorkspaceID:      workspaceID,
		ProjectID:        "cc-connect",
		MemberType:       MemberTypeAgentWorker,
		MemberID:         projectWorker.ID,
		Role:             "developer",
		Title:            "dev-codex",
		AutoPickTasks:    true,
		AttentionEnabled: true,
		PriorityWeight:   1,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := db.UpsertProjectMembership(membership); err != nil {
		t.Fatalf("upsert membership: %v", err)
	}

	resolved, ok, err := New(db).ProjectWorker(workspaceID, "cc-connect", "dev-codex")
	if err != nil {
		t.Fatalf("resolve project worker: %v", err)
	}
	if !ok || resolved.Worker.ID != projectWorker.ID || resolved.Membership.ID != membership.ID {
		t.Fatalf("resolved wrong worker: ok=%v value=%+v", ok, resolved)
	}
	if resolved.Worker.DefaultRuntimeNodeID != "rnode-cc-connect" {
		t.Fatalf("runtime node lost: %+v", resolved.Worker)
	}
}
