package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/multigent/multigent/internal/entity"
)

func TestRuntimeContactsListUsersAndCurrentProjectAgents(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	if err := s.users.CreateUser("cg33", "pass123", RoleMember, "Glenn Chen", "glenn@example.com", "", "", ""); err != nil {
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
		if contact.Type == "user" && contact.Identity == "cg33" && contact.DisplayName == "Glenn Chen" && contact.Email == "glenn@example.com" {
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
	if err := s.users.CreateUser("cg33", "pass123", RoleMember, "Glenn Chen", "glenn@example.com", "", "", ""); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := s.controlDB.UpsertWorkspaceMember(workspaceID, "cg33", WorkspaceRoleMember); err != nil {
		t.Fatalf("member: %v", err)
	}

	raw, _ := json.Marshal(runtimeMessageBody{
		To:      "Glenn Chen (cg33)",
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
	if err := s.users.CreateUser("cg33", "pass123", RoleMember, "Glenn Chen", "glenn@example.com", "", "", ""); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := s.controlDB.UpsertWorkspaceMember(workspaceID, "cg33", WorkspaceRoleMember); err != nil {
		t.Fatalf("member: %v", err)
	}

	raw, _ := json.Marshal(runtimeMessageBody{
		To:      "glenn@example.com",
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
	if err := s.users.CreateUser("cg33", "pass123", RoleMember, "Glenn Chen", "glenn@example.com", "", "", ""); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := s.controlDB.UpsertWorkspaceMember(workspaceID, "cg33", WorkspaceRoleMember); err != nil {
		t.Fatalf("member: %v", err)
	}

	raw, _ := json.Marshal(runtimeMessageBody{
		To:   "glenn",
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
	for _, want := range []string{"did you mean", "cg33", "Glenn Chen", "glenn@example.com"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in body=%s", want, body)
		}
	}
}

func TestRuntimePostMessageRejectsCrossProjectAgentContact(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	if err := s.st.SaveProject("other", &entity.Project{Name: "other"}); err != nil {
		t.Fatalf("save project: %v", err)
	}
	if err := s.st.SaveAgentMeta("other", "qa", &entity.AgentMeta{Name: "qa", Project: "other"}); err != nil {
		t.Fatalf("save agent: %v", err)
	}

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
