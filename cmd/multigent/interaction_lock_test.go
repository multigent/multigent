package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
)

func TestAcquireCLIInteractionUsesControlDBActiveLock(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	db := openTestWorkspaceControlDB(t, root)
	defer db.Close()
	if err := db.UpsertWorkspace(controldb.Workspace{
		ID:        "ws-one",
		Name:      "One",
		Slug:      "one",
		Root:      root,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("workspace: %v", err)
	}

	lease, busy, err := acquireCLIInteraction(root, "project", "pm", "scheduler", "heartbeat", "scheduler", "running_task")
	if err != nil || busy || lease == nil {
		t.Fatalf("first acquire lease=%v busy=%v err=%v", lease != nil, busy, err)
	}
	second, busy, err := acquireCLIInteraction(root, "project", "pm", "manual_run", "cli", "cli", "running_task")
	if err != nil || !busy || second == nil {
		t.Fatalf("second acquire should be busy lease=%v busy=%v err=%v", second != nil, busy, err)
	}
	lease.Release()
	third, busy, err := acquireCLIInteraction(root, "project", "pm", "manual_run", "cli", "cli", "running_task")
	if err != nil || busy || third == nil {
		t.Fatalf("third acquire after release lease=%v busy=%v err=%v", third != nil, busy, err)
	}
	third.Release()
}

func TestCLIInteractionLeasePersistsRuntimeSessionID(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	db := openTestWorkspaceControlDB(t, root)
	defer db.Close()
	if err := db.UpsertWorkspace(controldb.Workspace{
		ID:        "ws-one",
		Name:      "One",
		Slug:      "one",
		Root:      root,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("workspace: %v", err)
	}

	lease, busy, err := acquireCLIInteraction(root, "project", "pm", "scheduler", "heartbeat", "scheduler", "running_task")
	if err != nil || busy || lease == nil {
		t.Fatalf("acquire lease=%v busy=%v err=%v", lease != nil, busy, err)
	}
	lease.SetRuntimeSessionID("runtime-one")
	stored, ok, err := lease.db.InteractionSessionByID(lease.session.ID)
	if err != nil || !ok {
		t.Fatalf("lookup session ok=%v err=%v", ok, err)
	}
	if stored.RuntimeSessionID != "runtime-one" {
		t.Fatalf("runtime session id=%q", stored.RuntimeSessionID)
	}
	lease.Release()
}

func openTestWorkspaceControlDB(t *testing.T, root string) controldb.Store {
	t.Helper()
	t.Setenv("MULTIGENT_CONTROL_DATA_DIR", "")
	t.Setenv("MULTIGENT_DATA_DIR", root)
	if err := os.MkdirAll(filepath.Join(root, ".multigent"), 0o755); err != nil {
		t.Fatalf("mkdir db dir: %v", err)
	}
	db, err := controldb.Open(filepath.Join(root, ".multigent", "multigent.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return db
}
