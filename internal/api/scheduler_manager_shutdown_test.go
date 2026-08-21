package api

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestSchedulerManagerGracefulShutdownDoesNotSignalOneShotRuns(t *testing.T) {
	m := newSchedulerManager(t.TempDir())
	cmd := exec.Command("sh", "-c", "sleep 0.2")
	if _, err := m.StartManagedCommand("manual/foo/bar", "foo", "bar", schedulerModeManualTask, cmd); err != nil {
		t.Fatalf("start managed command: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	running := m.GracefulShutdown(ctx)
	if len(running) != 1 {
		t.Fatalf("expected one active one-shot run after graceful timeout, got %d", len(running))
	}
	if running[0].Mode != schedulerModeManualTask {
		t.Fatalf("expected manual task mode, got %q", running[0].Mode)
	}

	if err := m.WaitKey("manual/foo/bar"); err != nil {
		t.Fatalf("one-shot run should finish naturally after drain timeout, got %v", err)
	}
}
