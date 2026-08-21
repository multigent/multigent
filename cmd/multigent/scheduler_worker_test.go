package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/taskstore"
)

func TestSchedulerTargetsDeduplicateAgentWorkerMemberships(t *testing.T) {
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
	nowText := now.Format(time.RFC3339)
	if err := db.UpsertWorkspace(controldb.Workspace{ID: "ws", Name: "Test", Slug: "test", Root: root, UpdatedAt: nowText}); err != nil {
		t.Fatal(err)
	}
	scheduleRaw, _ := json.Marshal(entity.HeartbeatConfig{Enabled: true, Interval: "30m"})
	if err := db.UpsertAgentWorker(controldb.AgentWorker{
		ID:           "aw-nova",
		WorkspaceID:  "ws",
		Name:         "nova",
		DisplayName:  "Nova",
		ScheduleJSON: string(scheduleRaw),
		CreatedAt:    nowText,
		UpdatedAt:    nowText,
	}); err != nil {
		t.Fatal(err)
	}
	for _, project := range []string{"alpha", "beta"} {
		if err := db.UpsertProjectMembership(controldb.ProjectMembership{
			ID:               "pm-" + project,
			WorkspaceID:      "ws",
			ProjectID:        project,
			MemberType:       "agent_worker",
			MemberID:         "aw-nova",
			Role:             "manager",
			Title:            "nova",
			AutoPickTasks:    true,
			AttentionEnabled: true,
			CreatedAt:        nowText,
			UpdatedAt:        nowText,
		}); err != nil {
			t.Fatal(err)
		}
	}

	ts := taskstore.NewDB(root, db)
	if err := ts.AddTask("alpha", "nova", &entity.Task{ID: "t-alpha", Title: "Alpha", Priority: 3, Status: entity.TaskStatusPending, CreatedAt: now.Add(-time.Hour), UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := ts.AddTask("beta", "nova", &entity.Task{ID: "t-beta", Title: "Beta", Priority: 1, Status: entity.TaskStatusPending, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}

	heartbeatTargets, cronTargets := collectSchedulerStartTargets(root, []string{"alpha", "beta"}, "", ts)
	if len(heartbeatTargets) != 1 {
		t.Fatalf("expected one worker heartbeat target, got %#v", heartbeatTargets)
	}
	if len(heartbeatTargets[0].memberships) != 2 {
		t.Fatalf("expected both project memberships on the worker target, got %#v", heartbeatTargets[0].memberships)
	}
	if len(cronTargets) != 0 {
		t.Fatalf("unexpected cron targets: %#v", cronTargets)
	}

	selected := selectSchedulerExecutionTarget(ts, heartbeatTargets[0].memberships)
	if selected.project != "beta" || selected.agent != "nova" {
		t.Fatalf("expected beta/nova to run first, got %#v", selected)
	}
}
