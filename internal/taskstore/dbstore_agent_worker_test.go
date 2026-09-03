package taskstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
)

func TestDBStoreHeartbeatUsesAgentWorkerSchedule(t *testing.T) {
	root := t.TempDir()
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
	now := time.Now().UTC().Format(time.RFC3339)
	if err := db.UpsertWorkspace(controldb.Workspace{ID: "ws", Name: "Test", Slug: "test", Root: root, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	initial := entity.HeartbeatConfig{Enabled: true, Interval: "20m"}
	initialRaw, _ := json.Marshal(initial)
	if err := db.UpsertAgentWorker(controldb.AgentWorker{
		ID:               "aw-pm",
		WorkspaceID:      "ws",
		Name:             "manager-agent",
		DisplayName:      "manager-agent",
		Model:            string(entity.ModelClaudeCode),
		ScheduleJSON:     string(initialRaw),
		PrimarySessionID: "session-primary",
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertProjectMembership(controldb.ProjectMembership{
		ID:          "pm-1",
		WorkspaceID: "ws",
		ProjectID:   "sample",
		MemberType:  "agent_worker",
		MemberID:    "aw-pm",
		Role:        "pm",
		Title:       "pm",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatal(err)
	}

	st := NewDB(root, db)
	hb, err := st.GetHeartbeat("sample", "pm")
	if err != nil {
		t.Fatal(err)
	}
	if hb.Interval != "20m" || hb.SessionID != "" {
		t.Fatalf("unexpected heartbeat from worker schedule: %#v", hb)
	}
	hb.Interval = "45m"
	hb.Triggers = []entity.TriggerType{entity.TriggerOnMessage}
	if err := st.SaveHeartbeat("sample", "pm", hb); err != nil {
		t.Fatal(err)
	}
	worker, ok, err := db.AgentWorkerByID("ws", "aw-pm")
	if err != nil || !ok {
		t.Fatalf("lookup worker ok=%v err=%v", ok, err)
	}
	var stored entity.HeartbeatConfig
	if err := json.Unmarshal([]byte(worker.ScheduleJSON), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Interval != "45m" || len(stored.Triggers) != 1 || stored.Triggers[0] != entity.TriggerOnMessage {
		t.Fatalf("worker schedule was not updated: %#v", stored)
	}
	if _, ok, err := db.GetRecord("heartbeat", "ws", []string{"sample", "pm"}); err != nil || ok {
		t.Fatalf("legacy heartbeat record should not be written, ok=%v err=%v", ok, err)
	}
}
