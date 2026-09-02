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

func TestSchedulerTargetsUseWholeWorkspaceWhenProjectFilterIsEmpty(t *testing.T) {
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

	nowText := time.Now().UTC().Format(time.RFC3339)
	if err := db.UpsertWorkspace(controldb.Workspace{ID: "ws", Name: "Test", Slug: "test", Root: root, UpdatedAt: nowText}); err != nil {
		t.Fatal(err)
	}
	scheduleRaw, _ := json.Marshal(entity.HeartbeatConfig{Enabled: true, Interval: "30m"})
	if err := db.UpsertAgentWorker(controldb.AgentWorker{
		ID: "aw-nova", WorkspaceID: "ws", Name: "nova", ScheduleJSON: string(scheduleRaw), CreatedAt: nowText, UpdatedAt: nowText,
	}); err != nil {
		t.Fatal(err)
	}
	for _, project := range []string{"alpha", "beta"} {
		if err := db.UpsertProjectMembership(controldb.ProjectMembership{
			ID: "pm-" + project, WorkspaceID: "ws", ProjectID: project,
			MemberType: "agent_worker", MemberID: "aw-nova", Title: "nova",
			AutoPickTasks: true, AttentionEnabled: true, CreatedAt: nowText, UpdatedAt: nowText,
		}); err != nil {
			t.Fatal(err)
		}
	}

	ts := taskstore.NewDB(root, db)
	heartbeatTargets, _ := collectSchedulerStartTargets(root, nil, "", ts)
	if len(heartbeatTargets) != 1 || len(heartbeatTargets[0].memberships) != 2 {
		t.Fatalf("expected whole-workspace worker target, got %#v", heartbeatTargets)
	}
}

func TestSchedulerSelectionSkipsFutureTasks(t *testing.T) {
	root := t.TempDir()
	ts := taskstore.New(root)
	now := time.Now().UTC()
	future := now.Add(30 * time.Minute)
	if err := ts.AddTask("alpha", "nova", &entity.Task{
		ID:        "future",
		Title:     "Future",
		Priority:  0,
		Status:    entity.TaskStatusPending,
		NotBefore: &future,
		CreatedAt: now.Add(-time.Hour),
		UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := ts.AddTask("alpha", "nova", &entity.Task{
		ID:        "ready",
		Title:     "Ready",
		Priority:  3,
		Status:    entity.TaskStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	selected, err := nextPendingTask(ts, "alpha", "nova")
	if err != nil {
		t.Fatal(err)
	}
	if selected == nil || selected.ID != "ready" {
		t.Fatalf("selected %v, want ready", selected)
	}

	next, err := nextScheduledPendingTaskAt(ts, "alpha", "nova", now)
	if err != nil {
		t.Fatal(err)
	}
	if next == nil || !next.Equal(future) {
		t.Fatalf("next scheduled %v, want %v", next, future)
	}
}

func TestCapWaitForScheduledTasks(t *testing.T) {
	root := t.TempDir()
	ts := taskstore.New(root)
	now := time.Now().UTC()
	future := now.Add(5 * time.Minute)
	if err := ts.AddTask("alpha", "nova", &entity.Task{
		ID:        "future",
		Title:     "Future",
		Priority:  1,
		Status:    entity.TaskStatusPending,
		NotBefore: &future,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	got := capWaitForScheduledTasks(ts, []schedulerAgentKey{{project: "alpha", agent: "nova"}}, time.Hour, now)
	if got < 4*time.Minute || got > 6*time.Minute {
		t.Fatalf("wait = %s, want about 5m", got)
	}
}

func TestDueCronSlotRecoversMissedDailySlot(t *testing.T) {
	sched, err := schedulerCronParser.Parse("30 21 * * 2,4,6")
	if err != nil {
		t.Fatal(err)
	}
	lastRun := time.Date(2026, 8, 31, 21, 30, 0, 0, time.Local)
	now := time.Date(2026, 9, 2, 8, 49, 0, 0, time.Local)
	got := dueCronSlot(sched, &lastRun, now)
	want := time.Date(2026, 9, 1, 21, 30, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("dueCronSlot() = %v, want %v", got, want)
	}
}

func TestDueCronSlotDoesNotReplayConsumedSlot(t *testing.T) {
	sched, err := schedulerCronParser.Parse("30 21 * * 2,4,6")
	if err != nil {
		t.Fatal(err)
	}
	lastRun := time.Date(2026, 9, 1, 21, 30, 0, 0, time.Local)
	now := time.Date(2026, 9, 2, 8, 49, 0, 0, time.Local)
	if got := dueCronSlot(sched, &lastRun, now); !got.IsZero() {
		t.Fatalf("dueCronSlot() = %v, want no due slot", got)
	}
}

func TestDueCronSlotDoesNotFireBeforeNextWeekdaySlot(t *testing.T) {
	sched, err := schedulerCronParser.Parse("30 21 * * 0,1,3,5")
	if err != nil {
		t.Fatal(err)
	}
	lastRun := time.Date(2026, 8, 31, 21, 30, 0, 0, time.Local)
	now := time.Date(2026, 9, 2, 9, 49, 0, 0, time.Local)
	if got := dueCronSlot(sched, &lastRun, now); !got.IsZero() {
		t.Fatalf("dueCronSlot() = %v, want no due slot before 21:30", got)
	}
}

func TestDueCronSlotRecoversFirstRunAfterDowntime(t *testing.T) {
	sched, err := schedulerCronParser.Parse("0 8 * * *")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.Local)
	got := dueCronSlot(sched, nil, now)
	want := time.Date(2026, 9, 2, 8, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("dueCronSlot() = %v, want %v", got, want)
	}
}
