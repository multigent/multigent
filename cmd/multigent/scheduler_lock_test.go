package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcquireSchedulerStartLockRejectsLiveProcess(t *testing.T) {
	root := t.TempDir()
	lock, err := acquireSchedulerStartLock(root, "sample", "")
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}
	defer lock.Release()

	_, err = acquireSchedulerStartLock(root, "sample", "")
	if err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("expected already running error, got %v", err)
	}
}

func TestAcquireSchedulerStartLockReplacesStaleLock(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".multigent", "scheduler-locks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, schedulerStartLockName(root, "sample", "pm")+".json")
	raw, _ := json.Marshal(schedulerStartLockFile{
		Root:    root,
		Project: "sample",
		Agent:   "pm",
		PID:     99999999,
	})
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	lock, err := acquireSchedulerStartLock(root, "sample", "pm")
	if err != nil {
		t.Fatalf("replace stale lock: %v", err)
	}
	defer lock.Release()
	updated, err := readSchedulerStartLock(path)
	if err != nil {
		t.Fatalf("read replaced lock: %v", err)
	}
	if updated.PID != os.Getpid() {
		t.Fatalf("lock pid was not replaced: %+v", updated)
	}
}
