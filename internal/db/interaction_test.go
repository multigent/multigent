package db

import (
	"path/filepath"
	"testing"
)

func TestInteractionSessionsPersistActiveLockAndEvents(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "multigent.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := db.UpsertWorkspace(Workspace{ID: "ws-one", Name: "One", Slug: "one", Root: "/tmp/one", CreatedAt: nowUTC()}); err != nil {
		t.Fatalf("workspace: %v", err)
	}
	first := InteractionSession{
		ID:            "sess-one",
		WorkspaceID:   "ws-one",
		ProjectID:     "project",
		AgentID:       "pm",
		SourceKind:    "web_chat",
		SourceChannel: "web",
		ActorType:     "user",
		ActorID:       "owner",
	}
	if err := db.CreateInteractionSession(first); err != nil {
		t.Fatalf("create first: %v", err)
	}
	if err := db.CreateInteractionSession(InteractionSession{
		ID:            "sess-two",
		WorkspaceID:   "ws-one",
		ProjectID:     "project",
		AgentID:       "pm",
		SourceKind:    "lark",
		SourceChannel: "chat",
		ActorType:     "user",
		ActorID:       "owner",
	}); err == nil {
		t.Fatalf("second active session should violate unique active lock")
	}
	active, ok, err := db.ActiveInteractionSession("ws-one", "project", "pm")
	if err != nil || !ok || active.ID != "sess-one" {
		t.Fatalf("active ok=%v err=%v session=%#v", ok, err, active)
	}
	active.Status = "completed"
	active.CompletedAt = nowUTC()
	if err := db.UpdateInteractionSession(active); err != nil {
		t.Fatalf("complete first: %v", err)
	}
	if _, ok, err := db.ActiveInteractionSession("ws-one", "project", "pm"); err != nil || ok {
		t.Fatalf("active after complete ok=%v err=%v", ok, err)
	}
	if err := db.CreateInteractionSession(InteractionSession{
		ID:            "sess-two",
		WorkspaceID:   "ws-one",
		ProjectID:     "project",
		AgentID:       "pm",
		SourceKind:    "lark",
		SourceChannel: "chat",
		ActorType:     "user",
		ActorID:       "owner",
	}); err != nil {
		t.Fatalf("create second after complete: %v", err)
	}

	if err := db.CreateInteractionEvent(InteractionEvent{
		ID:          "evt-one",
		SessionID:   "sess-two",
		WorkspaceID: "ws-one",
		ActorType:   "user",
		ActorID:     "owner",
		Channel:     "lark",
		EventType:   "message",
		Content:     "hello",
	}); err != nil {
		t.Fatalf("create event: %v", err)
	}
	events, err := db.ListInteractionEvents(InteractionEventFilter{WorkspaceID: "ws-one", SessionID: "sess-two"})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 || events[0].Content != "hello" {
		t.Fatalf("events=%#v", events)
	}
}
