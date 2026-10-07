package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/taskstore"
)

func TestRetryArchivedTaskKeepsTaskInDBStore(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MULTIGENT_CONTROL_DATA_DIR", "")
	t.Setenv("MULTIGENT_DATA_DIR", root)
	if err := os.MkdirAll(filepath.Join(root, ".multigent"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".multigent", "agency.yaml"), []byte("name: Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := controldb.Open(filepath.Join(root, ".multigent", "multigent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	if err := db.UpsertWorkspace(controldb.Workspace{ID: "ws", Name: "Test", Slug: "test", Root: root, UpdatedAt: now.Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	ts := taskstore.NewDB(root, db)
	for _, id := range []string{"t-failed", "t-other"} {
		task := &entity.Task{ID: id, Title: id, Status: entity.TaskStatusPending, CreatedAt: now, UpdatedAt: now}
		if err := ts.AddTask("alpha", "worker", task); err != nil {
			t.Fatal(err)
		}
		task.Status = entity.TaskStatusDoneFailed
		task.LastError = "boom"
		if err := ts.ArchiveTask("alpha", "worker", task); err != nil {
			t.Fatal(err)
		}
	}

	retried, err := retryArchivedTask(ts, "alpha", "worker", "t-failed", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if retried.Status != entity.TaskStatusPending || retried.RetryCount != 1 || retried.ArchivedAt != nil || retried.LastError != "" {
		t.Fatalf("retried task = %+v", retried)
	}
	active, err := ts.ListTasks("alpha", "worker")
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].ID != "t-failed" {
		t.Fatalf("active tasks = %+v, want t-failed pending", active)
	}
	archived, err := ts.ListArchivedTasks("alpha", "worker")
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 1 || archived[0].ID != "t-other" {
		t.Fatalf("archived tasks = %+v, want only t-other", archived)
	}
	if _, err := retryArchivedTask(ts, "alpha", "worker", "t-failed", now); err == nil {
		t.Fatal("retrying a pending task should fail")
	}
}
