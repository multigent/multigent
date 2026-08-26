package db

import (
	"path/filepath"
	"testing"
)

func TestAgentWorkerMembershipAndAttentionSignal(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "multigent.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	workspaceID := "ws-agent-worker"
	if err := db.UpsertWorkspace(Workspace{
		ID:        workspaceID,
		Name:      "Agent Worker Test",
		Slug:      "agent-worker-test",
		Root:      t.TempDir(),
		CreatedAt: nowUTC(),
	}); err != nil {
		t.Fatalf("upsert workspace: %v", err)
	}

	worker := AgentWorker{
		ID:                    "aw-nova",
		WorkspaceID:           workspaceID,
		Name:                  "nova",
		DisplayName:           "Nova",
		DefaultModelAccountID: "codex-official",
		ScheduleJSON:          `{"interval":"2h"}`,
		AttentionPolicyJSON:   `{"im_mention":true}`,
		PrimarySessionID:      "sess-primary",
	}
	if err := db.UpsertAgentWorker(worker); err != nil {
		t.Fatalf("upsert worker: %v", err)
	}
	got, ok, err := db.AgentWorkerByName(workspaceID, "nova")
	if err != nil {
		t.Fatalf("get worker by name: %v", err)
	}
	if !ok {
		t.Fatal("worker not found")
	}
	if got.ID != worker.ID || got.PrimarySessionID != "sess-primary" {
		t.Fatalf("unexpected worker: %+v", got)
	}

	membership := ProjectMembership{
		ID:               "pm-nova-mcp",
		WorkspaceID:      workspaceID,
		ProjectID:        "customer-mcp-server",
		MemberType:       "agent_worker",
		MemberID:         worker.ID,
		Role:             "project-manager",
		Prompt:           "负责 MCP Server 项目跟进。",
		PermissionsJSON:  `["task.read","task.write"]`,
		AutoPickTasks:    true,
		AttentionEnabled: true,
		PriorityWeight:   1.5,
	}
	if err := db.UpsertProjectMembership(membership); err != nil {
		t.Fatalf("upsert membership: %v", err)
	}
	memberships, err := db.ListProjectMemberships(ProjectMembershipFilter{
		WorkspaceID: workspaceID,
		ProjectID:   "customer-mcp-server",
		MemberType:  "agent_worker",
	})
	if err != nil {
		t.Fatalf("list memberships: %v", err)
	}
	if len(memberships) != 1 || memberships[0].MemberID != worker.ID || !memberships[0].AttentionEnabled {
		t.Fatalf("unexpected memberships: %+v", memberships)
	}

	signal := AttentionSignal{
		ID:            "sig-1",
		WorkspaceID:   workspaceID,
		AgentWorkerID: worker.ID,
		DedupeKey:     "lark:msg-1",
		SourceKind:    "lark_message",
		SourceID:      "msg-1",
		Reason:        "mention",
		Summary:       "Glenn 在群里 @Nova。",
	}
	if err := db.UpsertAttentionSignal(signal); err != nil {
		t.Fatalf("upsert signal: %v", err)
	}
	signal.ID = "sig-duplicate"
	signal.Summary = "duplicate should update, not insert"
	if err := db.UpsertAttentionSignal(signal); err != nil {
		t.Fatalf("upsert duplicate signal: %v", err)
	}
	signals, err := db.ListAttentionSignals(AttentionSignalFilter{WorkspaceID: workspaceID, AgentWorkerID: worker.ID})
	if err != nil {
		t.Fatalf("list signals: %v", err)
	}
	if len(signals) != 1 || signals[0].Summary != "duplicate should update, not insert" {
		t.Fatalf("unexpected signals after dedupe: %+v", signals)
	}
	if err := db.MarkAttentionSignalStatus(workspaceID, signals[0].ID, "handling"); err != nil {
		t.Fatalf("mark handling: %v", err)
	}
	updated, ok, err := db.AttentionSignalByID(workspaceID, signals[0].ID)
	if err != nil {
		t.Fatalf("get signal: %v", err)
	}
	if !ok || updated.Status != "handling" || updated.HandlingAt == "" {
		t.Fatalf("unexpected updated signal: %+v", updated)
	}
	if err := db.MarkAttentionSignalStatus(workspaceID, signals[0].ID, "handled"); err != nil {
		t.Fatalf("mark handled: %v", err)
	}
	signal.ID = "sig-after-handled"
	signal.Status = "pending"
	signal.Summary = "duplicate after handled should not reopen"
	if err := db.UpsertAttentionSignal(signal); err != nil {
		t.Fatalf("upsert handled duplicate signal: %v", err)
	}
	updated, ok, err = db.AttentionSignalByID(workspaceID, signals[0].ID)
	if err != nil || !ok {
		t.Fatalf("get handled signal ok=%v err=%v", ok, err)
	}
	if updated.Status != "handled" || updated.HandledAt == "" {
		t.Fatalf("handled signal was reopened: %+v", updated)
	}
	if err := db.MarkAttentionSignalStatus(workspaceID, signals[0].ID, "seen"); err != nil {
		t.Fatalf("mark handled signal seen: %v", err)
	}
	updated, ok, err = db.AttentionSignalByID(workspaceID, signals[0].ID)
	if err != nil || !ok {
		t.Fatalf("get handled signal after seen ok=%v err=%v", ok, err)
	}
	if updated.Status != "handled" {
		t.Fatalf("terminal signal should not move back to seen: %+v", updated)
	}
	if err := db.MarkAttentionSignalStatus(workspaceID, signals[0].ID, "handling"); err != nil {
		t.Fatalf("mark handled signal handling: %v", err)
	}
	updated, ok, err = db.AttentionSignalByID(workspaceID, signals[0].ID)
	if err != nil || !ok {
		t.Fatalf("get handled signal after handling ok=%v err=%v", ok, err)
	}
	if updated.Status != "handled" {
		t.Fatalf("terminal signal should not move back to handling: %+v", updated)
	}

	cursor := AttentionCursor{
		ID:            "cur-1",
		WorkspaceID:   workspaceID,
		AgentWorkerID: worker.ID,
		SourceKind:    "lark",
		SourceChannel: "chat-1",
		Cursor:        "msg-1",
		SeenUntil:     "2026-08-21T10:00:00Z",
	}
	if err := db.UpsertAttentionCursor(cursor); err != nil {
		t.Fatalf("upsert cursor: %v", err)
	}
	cursor.ID = "cur-duplicate"
	cursor.Cursor = "msg-2"
	cursor.SeenUntil = "2026-08-21T10:05:00Z"
	if err := db.UpsertAttentionCursor(cursor); err != nil {
		t.Fatalf("upsert duplicate cursor: %v", err)
	}
	gotCursor, ok, err := db.AttentionCursorBySource(workspaceID, worker.ID, "lark", "chat-1")
	if err != nil {
		t.Fatalf("get cursor: %v", err)
	}
	if !ok || gotCursor.Cursor != "msg-2" || gotCursor.SeenUntil != "2026-08-21T10:05:00Z" {
		t.Fatalf("unexpected cursor: %+v", gotCursor)
	}
	cursors, err := db.ListAttentionCursors(AttentionCursorFilter{WorkspaceID: workspaceID, AgentWorkerID: worker.ID})
	if err != nil {
		t.Fatalf("list cursors: %v", err)
	}
	if len(cursors) != 1 {
		t.Fatalf("unexpected cursors: %+v", cursors)
	}
}
