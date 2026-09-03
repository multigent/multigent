package attention

import (
	"path/filepath"
	"testing"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
)

func TestSignalIDsForTaskRequiresWakeupMetadata(t *testing.T) {
	if got := SignalIDsForTask(&entity.Task{Type: "task", Vars: map[string]string{
		signalIDsVar: `["sig-1"]`,
	}}); len(got) != 0 {
		t.Fatalf("regular task should not consume attention signals: %v", got)
	}
	if got := SignalIDsForTask(&entity.Task{Type: "wakeup"}); len(got) != 0 {
		t.Fatalf("wakeup without scheduler metadata should not consume signals: %v", got)
	}
}

func TestSignalIDsForTaskDeduplicatesAndTrims(t *testing.T) {
	task := &entity.Task{
		Type: "wakeup",
		Vars: map[string]string{
			signalIDsVar: `[" sig-1 ","sig-1","","sig-2"]`,
		},
	}
	got := SignalIDsForTask(task)
	want := []string{"sig-1", "sig-2"}
	if len(got) != len(want) {
		t.Fatalf("signal ids=%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("signal ids=%v, want %v", got, want)
		}
	}
}

func TestCloseTaskSignalsRecordsTaskAndDoesNotReopenTerminalSignal(t *testing.T) {
	db, err := controldb.Open(filepath.Join(t.TempDir(), "multigent.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	workspaceID := "ws-attention"
	if err := db.UpsertWorkspace(controldb.Workspace{
		ID:        workspaceID,
		Name:      "Attention Test",
		Slug:      "attention-test",
		Root:      t.TempDir(),
		CreatedAt: "2026-09-03T00:00:00Z",
	}); err != nil {
		t.Fatalf("upsert workspace: %v", err)
	}
	if err := db.UpsertAgentWorker(controldb.AgentWorker{
		ID:          "aw-agent",
		WorkspaceID: workspaceID,
		Name:        "agent",
		DisplayName: "Agent",
	}); err != nil {
		t.Fatalf("upsert worker: %v", err)
	}
	if err := db.UpsertAttentionSignal(controldb.AttentionSignal{
		ID:            "sig-close",
		WorkspaceID:   workspaceID,
		AgentWorkerID: "aw-agent",
		DedupeKey:     "test:close",
		Summary:       "signal to close",
		Status:        "pending",
	}); err != nil {
		t.Fatalf("upsert signal: %v", err)
	}
	task := &entity.Task{
		ID:     "task-wakeup",
		Type:   entity.TaskType("wakeup"),
		Status: entity.TaskStatusDoneSuccess,
		Vars: map[string]string{
			signalIDsVar: `["sig-close"]`,
		},
	}
	if err := CloseTaskSignals(db, workspaceID, task, "task:task-wakeup"); err != nil {
		t.Fatalf("close signals: %v", err)
	}
	signal, ok, err := db.AttentionSignalByID(workspaceID, "sig-close")
	if err != nil || !ok {
		t.Fatalf("load closed signal ok=%v err=%v", ok, err)
	}
	if signal.Status != "handled" || signal.ResultRef != "task:task-wakeup" || signal.HandledAt == "" {
		t.Fatalf("unexpected closed signal: %+v", signal)
	}
	if err := CloseTaskSignals(db, workspaceID, task, "task:task-wakeup-again"); err != nil {
		t.Fatalf("close signal twice: %v", err)
	}
	signal, _, _ = db.AttentionSignalByID(workspaceID, "sig-close")
	if signal.Status != "handled" || signal.ResultRef != "task:task-wakeup" {
		t.Fatalf("terminal signal was reopened or overwritten: %+v", signal)
	}
}
