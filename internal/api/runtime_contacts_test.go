package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
)

func TestRuntimeContactsListUsersAndCurrentProjectAgents(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedSampleAgentsForTest(t, s, workspaceID)
	if err := s.users.CreateUser("cg33", "pass123", RoleMember, "owner-a", "owner-a@example.com", "", "", ""); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := s.controlDB.UpsertWorkspaceMember(workspaceID, "cg33", WorkspaceRoleMember); err != nil {
		t.Fatalf("member: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/contacts", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxRuntimeAgentKey, runtimeAgentPrincipal{
		WorkspaceID:  workspaceID,
		Project:      "sample",
		Agent:        "pm",
		Capabilities: []string{"message.use"},
	}))
	rec := httptest.NewRecorder()
	s.handleRuntimeContacts(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var contacts []runtimeContactRow
	if err := json.Unmarshal(rec.Body.Bytes(), &contacts); err != nil {
		t.Fatalf("decode contacts: %v", err)
	}
	seenUser := false
	seenAgent := false
	for _, contact := range contacts {
		if contact.Type == "user" && contact.Identity == "cg33" && contact.DisplayName == "owner-a" && contact.Email == "owner-a@example.com" {
			seenUser = true
		}
		if contact.Type == "agent" && contact.Identity == "sample/backend" {
			seenAgent = true
		}
		if contact.Type == "agent" && contact.Project != "sample" {
			t.Fatalf("unexpected cross-project agent contact: %#v", contact)
		}
	}
	if !seenUser || !seenAgent {
		t.Fatalf("contacts missing expected rows: %#v", contacts)
	}
}

func TestRuntimeContactsListProjectMembershipAgents(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedSampleAgentsForTest(t, s, workspaceID)
	now := "2026-08-21T00:00:00Z"
	upsertRuntimeAttentionWorker(t, s, workspaceID, "aw-worker-only", "sample", "worker-only", now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/contacts", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxRuntimeAgentKey, runtimeAgentPrincipal{
		WorkspaceID:  workspaceID,
		Project:      "sample",
		Agent:        "worker-only",
		Capabilities: []string{"message.use"},
	}))
	rec := httptest.NewRecorder()
	s.handleRuntimeContacts(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var contacts []runtimeContactRow
	if err := json.Unmarshal(rec.Body.Bytes(), &contacts); err != nil {
		t.Fatalf("decode contacts: %v", err)
	}
	for _, contact := range contacts {
		if contact.Type == "agent" && contact.Identity == "sample/worker-only" && contact.Agent == "worker-only" {
			return
		}
	}
	t.Fatalf("worker-backed agent contact missing: %#v", contacts)
}

func TestRuntimePostMessageResolvesDisplayNameUsernameForm(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedSampleAgentsForTest(t, s, workspaceID)
	if err := s.users.CreateUser("cg33", "pass123", RoleMember, "owner-a", "owner-a@example.com", "", "", ""); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := s.controlDB.UpsertWorkspaceMember(workspaceID, "cg33", WorkspaceRoleMember); err != nil {
		t.Fatalf("member: %v", err)
	}

	raw, _ := json.Marshal(runtimeMessageBody{
		To:      "owner-a (cg33)",
		Subject: "hello",
		Body:    "test message",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/messages", bytes.NewReader(raw))
	req = req.WithContext(context.WithValue(req.Context(), ctxRuntimeAgentKey, runtimeAgentPrincipal{
		WorkspaceID:  workspaceID,
		Project:      "sample",
		Agent:        "pm",
		Capabilities: []string{"message.use"},
	}))
	rec := httptest.NewRecorder()
	s.handleRuntimePostMessage(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	msgs, err := s.ts.ListMessages("cg33")
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("messages=%d", len(msgs))
	}
	if msgs[0].To != "cg33" || msgs[0].From != "sample/pm" || msgs[0].Subject != "hello" {
		t.Fatalf("bad message: %#v", msgs[0])
	}
}

func TestRuntimePostMessageResolvesEmailRecipient(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedSampleAgentsForTest(t, s, workspaceID)
	if err := s.users.CreateUser("cg33", "pass123", RoleMember, "owner-a", "owner-a@example.com", "", "", ""); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := s.controlDB.UpsertWorkspaceMember(workspaceID, "cg33", WorkspaceRoleMember); err != nil {
		t.Fatalf("member: %v", err)
	}

	raw, _ := json.Marshal(runtimeMessageBody{
		To:      "owner-a@example.com",
		Subject: "email recipient",
		Body:    "test message",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/messages", bytes.NewReader(raw))
	req = req.WithContext(context.WithValue(req.Context(), ctxRuntimeAgentKey, runtimeAgentPrincipal{
		WorkspaceID:  workspaceID,
		Project:      "sample",
		Agent:        "pm",
		Capabilities: []string{"message.use"},
	}))
	rec := httptest.NewRecorder()
	s.handleRuntimePostMessage(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	msgs, err := s.ts.ListMessages("cg33")
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 1 || msgs[0].To != "cg33" {
		t.Fatalf("bad messages: %#v", msgs)
	}
}

func TestRuntimePostMessageSuggestsFuzzyContacts(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedSampleAgentsForTest(t, s, workspaceID)
	if err := s.users.CreateUser("cg33", "pass123", RoleMember, "owner-a", "owner-a@example.com", "", "", ""); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := s.controlDB.UpsertWorkspaceMember(workspaceID, "cg33", WorkspaceRoleMember); err != nil {
		t.Fatalf("member: %v", err)
	}

	raw, _ := json.Marshal(runtimeMessageBody{
		To:   "owner-a@example",
		Body: "test message",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/messages", bytes.NewReader(raw))
	req = req.WithContext(context.WithValue(req.Context(), ctxRuntimeAgentKey, runtimeAgentPrincipal{
		WorkspaceID:  workspaceID,
		Project:      "sample",
		Agent:        "pm",
		Capabilities: []string{"message.use"},
	}))
	rec := httptest.NewRecorder()
	s.handleRuntimePostMessage(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"did you mean", "cg33", "owner-a", "owner-a@example.com"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in body=%s", want, body)
		}
	}
}

func TestRuntimePostMessageRejectsCrossProjectAgentContact(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedSampleAgentsForTest(t, s, workspaceID)
	if err := s.st.SaveProject("other", &entity.Project{Name: "other"}); err != nil {
		t.Fatalf("save project: %v", err)
	}
	seedAgentWorkerForTest(t, s, workspaceID, "other", "qa")

	raw, _ := json.Marshal(runtimeMessageBody{To: "other/qa", Body: "hello"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/messages", bytes.NewReader(raw))
	req = req.WithContext(context.WithValue(req.Context(), ctxRuntimeAgentKey, runtimeAgentPrincipal{
		WorkspaceID:  workspaceID,
		Project:      "sample",
		Agent:        "pm",
		Capabilities: []string{"message.use"},
	}))
	rec := httptest.NewRecorder()
	s.handleRuntimePostMessage(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRuntimePostMessageCreatesAttentionForAgentRecipient(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedSampleAgentsForTest(t, s, workspaceID)
	now := "2026-08-21T00:00:00Z"
	upsertRuntimeAttentionWorker(t, s, workspaceID, "aw-worker-only", "sample", "worker-only", now)
	saveRuntimeAttentionHeartbeat(t, s, workspaceID, "sample", "worker-only")

	raw, _ := json.Marshal(runtimeMessageBody{
		To:      "sample/worker-only",
		Subject: "needs PM decision",
		Body:    "Please review this rule ambiguity.",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/messages", bytes.NewReader(raw))
	req = req.WithContext(context.WithValue(req.Context(), ctxRuntimeAgentKey, runtimeAgentPrincipal{
		WorkspaceID:  workspaceID,
		Project:      "sample",
		Agent:        "pm",
		Capabilities: []string{"message.use"},
	}))
	rec := httptest.NewRecorder()
	s.handleRuntimePostMessage(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	msgs, err := s.ts.ListMessages("sample/worker-only")
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("messages=%d", len(msgs))
	}
	resolved, ok, err := s.agentDirectory.ResolveProjectMailbox(workspaceID, "sample/worker-only")
	if err != nil || !ok {
		t.Fatalf("resolve backend ok=%v err=%v", ok, err)
	}
	signals, err := s.controlDB.ListAttentionSignals(controldb.AttentionSignalFilter{
		WorkspaceID:   workspaceID,
		AgentWorkerID: resolved.Worker.ID,
		SourceKind:    "message",
		Reason:        "inbox_message",
	})
	if err != nil {
		t.Fatalf("list attention: %v", err)
	}
	if len(signals) != 1 {
		t.Fatalf("signals=%d %#v", len(signals), signals)
	}
	signal := signals[0]
	if signal.Status != "pending" || signal.SourceID != msgs[0].ID || signal.ActorID != "sample/pm" || !strings.Contains(signal.Summary, "needs PM decision") {
		t.Fatalf("bad signal: %#v message=%#v", signal, msgs[0])
	}
	assertAttentionWakeupTaskForSignal(t, s, "sample", "worker-only", signal.ID)
}

func TestRuntimePostMessageDoesNotCreateAttentionForUserRecipient(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedSampleAgentsForTest(t, s, workspaceID)
	if err := s.users.CreateUser("cg33", "pass123", RoleMember, "owner-a", "owner-a@example.com", "", "", ""); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := s.controlDB.UpsertWorkspaceMember(workspaceID, "cg33", WorkspaceRoleMember); err != nil {
		t.Fatalf("member: %v", err)
	}

	raw, _ := json.Marshal(runtimeMessageBody{
		To:      "cg33",
		Subject: "human note",
		Body:    "FYI",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/messages", bytes.NewReader(raw))
	req = req.WithContext(context.WithValue(req.Context(), ctxRuntimeAgentKey, runtimeAgentPrincipal{
		WorkspaceID:  workspaceID,
		Project:      "sample",
		Agent:        "pm",
		Capabilities: []string{"message.use"},
	}))
	rec := httptest.NewRecorder()
	s.handleRuntimePostMessage(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	signals, err := s.controlDB.ListAttentionSignals(controldb.AttentionSignalFilter{WorkspaceID: workspaceID})
	if err != nil {
		t.Fatalf("list attention: %v", err)
	}
	if len(signals) != 0 {
		t.Fatalf("unexpected signals: %#v", signals)
	}
}

func TestRuntimeReplyMessageCreatesAttentionForAgentRecipient(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedSampleAgentsForTest(t, s, workspaceID)
	now := "2026-08-21T00:00:00Z"
	upsertRuntimeAttentionWorker(t, s, workspaceID, "aw-worker-only", "sample", "worker-only", now)
	saveRuntimeAttentionHeartbeat(t, s, workspaceID, "sample", "worker-only")
	original := &entity.Message{
		ID:      "msg-original",
		From:    "sample/worker-only",
		To:      "sample/pm",
		Subject: "needs decision",
		Body:    "Please decide.",
		SentAt:  time.Now().UTC(),
	}
	if err := s.ts.SendMessage(original); err != nil {
		t.Fatalf("send original: %v", err)
	}

	raw, _ := json.Marshal(runtimeReplyMessageBody{
		Body: "Decision: needs-info.",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/messages/msg-original/reply", bytes.NewReader(raw))
	req.SetPathValue("id", "msg-original")
	req = req.WithContext(context.WithValue(req.Context(), ctxRuntimeAgentKey, runtimeAgentPrincipal{
		WorkspaceID:  workspaceID,
		Project:      "sample",
		Agent:        "pm",
		Capabilities: []string{"message.use"},
	}))
	rec := httptest.NewRecorder()
	s.handleRuntimeReplyMessage(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	resolved, ok, err := s.agentDirectory.ResolveProjectMailbox(workspaceID, "sample/worker-only")
	if err != nil || !ok {
		t.Fatalf("resolve worker ok=%v err=%v", ok, err)
	}
	signals, err := s.controlDB.ListAttentionSignals(controldb.AttentionSignalFilter{
		WorkspaceID:   workspaceID,
		AgentWorkerID: resolved.Worker.ID,
		SourceKind:    "message",
		Reason:        "inbox_message",
	})
	if err != nil {
		t.Fatalf("list attention: %v", err)
	}
	if len(signals) != 1 {
		t.Fatalf("signals=%d %#v", len(signals), signals)
	}
	signal := signals[0]
	if signal.Status != "pending" || signal.ActorID != "sample/pm" || !strings.Contains(signal.Summary, "needs decision") {
		t.Fatalf("bad signal: %#v", signal)
	}
	assertAttentionWakeupTaskForSignal(t, s, "sample", "worker-only", signal.ID)
}

func saveRuntimeAttentionHeartbeat(t *testing.T, s *Server, workspaceID, project, agent string) {
	t.Helper()
	target := s.runtimeSchedulerTargetForProjectAgent(workspaceID, project, agent)
	hb := &entity.HeartbeatConfig{
		Enabled:      true,
		WakeupPrompt: "Check pending attention and decide what to do.",
	}
	if err := s.saveSchedulerTargetHeartbeat(workspaceID, target, hb); err != nil {
		t.Fatalf("save heartbeat: %v", err)
	}
}

func assertAttentionWakeupTaskForSignal(t *testing.T, s *Server, project, agent, signalID string) {
	t.Helper()
	tasks, err := s.ts.ListTasks(project, agent, entity.TaskStatusPending)
	if err != nil {
		t.Fatalf("list pending tasks: %v", err)
	}
	for _, task := range tasks {
		if task != nil && task.CreatedBy == attentionWakeupTaskCreatedBy && strings.Contains(task.Prompt, signalID) {
			return
		}
	}
	t.Fatalf("attention wakeup task for signal %s not found in %#v", signalID, tasks)
}
