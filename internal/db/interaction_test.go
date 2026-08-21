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
	}); err != nil {
		t.Fatalf("different active source should be allowed: %v", err)
	}
	if err := db.CreateInteractionSession(InteractionSession{
		ID:            "sess-same-source",
		WorkspaceID:   "ws-one",
		ProjectID:     "project",
		AgentID:       "pm",
		SourceKind:    "web_chat",
		SourceChannel: "web",
		ActorType:     "user",
		ActorID:       "owner",
	}); err == nil {
		t.Fatalf("same active source should violate unique active lock")
	}
	active, ok, err := db.ActiveInteractionSession("ws-one", "project", "pm")
	if err != nil || !ok {
		t.Fatalf("active ok=%v err=%v session=%#v", ok, err, active)
	}
	active, ok, err = db.ActiveInteractionSessionForSource("ws-one", "project", "pm", "web_chat", "web", "owner")
	if err != nil || !ok || active.ID != "sess-one" {
		t.Fatalf("active source ok=%v err=%v session=%#v", ok, err, active)
	}
	active.Status = "completed"
	active.CompletedAt = nowUTC()
	if err := db.UpdateInteractionSession(active); err != nil {
		t.Fatalf("complete first: %v", err)
	}
	if _, ok, err := db.ActiveInteractionSessionForSource("ws-one", "project", "pm", "web_chat", "web", "owner"); err != nil || ok {
		t.Fatalf("active source after complete ok=%v err=%v", ok, err)
	}
	if err := db.CreateInteractionSession(InteractionSession{
		ID:            "sess-three",
		WorkspaceID:   "ws-one",
		ProjectID:     "project",
		AgentID:       "pm",
		SourceKind:    "web_chat",
		SourceChannel: "web",
		ActorType:     "user",
		ActorID:       "owner",
	}); err != nil {
		t.Fatalf("create same source after complete: %v", err)
	}

	if err := db.CreateInteractionEvent(InteractionEvent{
		ID:          "evt-one",
		SessionID:   "sess-three",
		WorkspaceID: "ws-one",
		ActorType:   "user",
		ActorID:     "owner",
		Channel:     "lark",
		EventType:   "message",
		Content:     "hello",
	}); err != nil {
		t.Fatalf("create event: %v", err)
	}
	events, err := db.ListInteractionEvents(InteractionEventFilter{WorkspaceID: "ws-one", SessionID: "sess-three"})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 || events[0].Content != "hello" {
		t.Fatalf("events=%#v", events)
	}
}

func TestInteractionRequestLifecycle(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "multigent.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := db.UpsertWorkspace(Workspace{ID: "ws-ir", Name: "IR", Slug: "ir", Root: "/tmp/ir", CreatedAt: nowUTC()}); err != nil {
		t.Fatalf("workspace: %v", err)
	}
	req := InteractionRequest{
		ID:               "ir_test",
		WorkspaceID:      "ws-ir",
		ProjectID:        "sample",
		AgentID:          "pm",
		ChannelBindingID: "chan-one",
		Provider:         "feishu",
		Recipient:        "owner",
		TargetType:       "user",
		TargetUserID:     "owner",
		Title:            "Decision",
		Body:             "Choose one",
		SchemaJSON:       `{"actions":[{"id":"approve"}]}`,
		ContextJSON:      `{"taskId":"t-1"}`,
		HandlerType:      "agent_event",
	}
	if err := db.CreateInteractionRequest(req); err != nil {
		t.Fatalf("create request: %v", err)
	}
	got, ok, err := db.InteractionRequestByID("ws-ir", req.ID)
	if err != nil || !ok {
		t.Fatalf("lookup ok=%v err=%v", ok, err)
	}
	if got.Status != "active" || got.Title != "Decision" || got.TargetUserID != "owner" {
		t.Fatalf("unexpected request: %#v", got)
	}
	got.Status = "submitted"
	got.AgentWorkerID = "aw-one"
	got.SubmittedBy = "owner"
	got.SubmissionJSON = `{"actionId":"approve"}`
	if err := db.UpdateInteractionRequest(got); err != nil {
		t.Fatalf("update request: %v", err)
	}
	got, ok, err = db.InteractionRequestByID("ws-ir", req.ID)
	if err != nil || !ok || got.Status != "submitted" || got.SubmittedBy != "owner" {
		t.Fatalf("updated request ok=%v err=%v got=%#v", ok, err, got)
	}
	requests, err := db.ListInteractionRequests(InteractionRequestFilter{WorkspaceID: "ws-ir", AgentWorkerID: "aw-one", Status: "submitted"})
	if err != nil {
		t.Fatalf("list interaction requests: %v", err)
	}
	if len(requests) != 1 || requests[0].ID != req.ID {
		t.Fatalf("unexpected listed requests: %#v", requests)
	}
}
