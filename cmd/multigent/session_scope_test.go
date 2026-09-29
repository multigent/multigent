package main

import (
	"testing"

	"github.com/multigent/multigent/internal/entity"
)

func TestTaskRunSessionID(t *testing.T) {
	if got := taskRunSessionID(entity.SessionScopeTask, "stale-provider-session"); got != "" {
		t.Fatalf("task-scoped run resumed %q", got)
	}
	if got := taskRunSessionID(entity.SessionScopeCycle, "provider-session"); got != "provider-session" {
		t.Fatalf("cycle run session = %q", got)
	}
}
