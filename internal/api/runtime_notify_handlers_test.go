package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	controldb "github.com/multigent/multigent/internal/db"
)

func TestRuntimeChannelsListAgentIMBindings(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	if err := s.controlDB.UpsertConnection(controldb.Connection{
		ID:             "conn-feishu",
		WorkspaceID:    workspaceID,
		Provider:       "feishu",
		ConnectionName: "default",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       ConnectionAuthAPIKey,
		Status:         "active",
	}); err != nil {
		t.Fatalf("upsert connection: %v", err)
	}
	if err := s.controlDB.UpsertAgentChannelBinding(controldb.AgentChannelBinding{
		ID:              "chan-feishu",
		WorkspaceID:     workspaceID,
		ProjectID:       "sample",
		AgentID:         "pm",
		Provider:        "feishu",
		ConnectionID:    "conn-feishu",
		ExternalOwnerID: "ou_owner",
		Status:          "connected",
	}); err != nil {
		t.Fatalf("upsert channel: %v", err)
	}
	if err := s.controlDB.UpsertAgentChannelTarget(controldb.AgentChannelTarget{
		ID:               "cht-release",
		WorkspaceID:      workspaceID,
		ChannelBindingID: "chan-feishu",
		Provider:         "feishu",
		TargetType:       "chat",
		DisplayName:      "发布评审群",
		ExternalChatID:   "oc_release",
	}); err != nil {
		t.Fatalf("upsert target: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/channels", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxRuntimeAgentKey, runtimeAgentPrincipal{
		WorkspaceID:  workspaceID,
		Project:      "sample",
		Agent:        "pm",
		Capabilities: []string{"message.use"},
	}))
	rec := httptest.NewRecorder()
	s.handleRuntimeChannels(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Channels []runtimeChannelRow `json:"channels"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Channels) != 1 || body.Channels[0].Provider != "feishu" || !body.Channels[0].CanNotify || !body.Channels[0].OwnerBound || len(body.Channels[0].Targets) != 1 {
		t.Fatalf("unexpected channels: %#v", body.Channels)
	}
}

func TestRuntimeNotifyWritesInboxWhenNoExternalChannel(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	raw, _ := json.Marshal(runtimeNotifyBody{To: "human", Subject: "Need review", Body: "Please review task", TaskID: "t-1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/notify", bytes.NewReader(raw))
	req = req.WithContext(context.WithValue(req.Context(), ctxRuntimeAgentKey, runtimeAgentPrincipal{
		WorkspaceID:  workspaceID,
		Project:      "sample",
		Agent:        "pm",
		Capabilities: []string{"message.use"},
	}))
	rec := httptest.NewRecorder()
	s.handleRuntimeNotify(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		InternalSent  bool   `json:"internalSent"`
		ExternalSent  bool   `json:"externalSent"`
		ExternalError string `json:"externalError"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.InternalSent || body.ExternalSent || body.ExternalError == "" {
		t.Fatalf("unexpected notify response: %#v", body)
	}
	msgs, err := s.ts.ListMessages("human")
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Subject != "Need review" || msgs[0].From != "sample/pm" {
		t.Fatalf("unexpected messages: %#v", msgs)
	}
}

func TestRuntimeNotifyTargetUsesUserChannelIdentity(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	binding := controldb.AgentChannelBinding{
		ID:           "chan-feishu",
		WorkspaceID:  workspaceID,
		ProjectID:    "sample",
		AgentID:      "pm",
		Provider:     "feishu",
		ConnectionID: "conn-feishu",
		Status:       "connected",
	}
	if err := s.controlDB.UpsertConnection(controldb.Connection{
		ID:             "conn-feishu",
		WorkspaceID:    workspaceID,
		Provider:       "feishu",
		ConnectionName: "default",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       ConnectionAuthAPIKey,
		Status:         "active",
	}); err != nil {
		t.Fatalf("upsert connection: %v", err)
	}
	if err := s.controlDB.UpsertAgentChannelBinding(binding); err != nil {
		t.Fatalf("upsert channel: %v", err)
	}
	if err := s.controlDB.UpsertUserChannelIdentity(controldb.UserChannelIdentity{
		ID:               "uch-owner",
		WorkspaceID:      workspaceID,
		UserID:           "owner",
		ChannelBindingID: "chan-feishu",
		Provider:         "feishu",
		ExternalUserID:   "ou_owner",
		ExternalChatID:   "oc_owner",
	}); err != nil {
		t.Fatalf("upsert identity: %v", err)
	}
	target, ok, err := s.runtimeNotifyTargetForRecipient(runtimeAgentPrincipal{
		WorkspaceID: workspaceID,
		Project:     "sample",
		Agent:       "pm",
	}, binding, "owner")
	if err != nil || !ok {
		t.Fatalf("target ok=%v err=%v", ok, err)
	}
	if target.ReceiveID != "ou_owner" || target.ReceiveIDType != "open_id" || target.ChatID != "oc_owner" {
		t.Fatalf("unexpected target: %#v", target)
	}
}

func TestRuntimeNotifyTargetUsesNamedChatTarget(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	binding := controldb.AgentChannelBinding{
		ID:           "chan-feishu",
		WorkspaceID:  workspaceID,
		ProjectID:    "sample",
		AgentID:      "pm",
		Provider:     "feishu",
		ConnectionID: "conn-feishu",
		Status:       "connected",
	}
	if err := s.controlDB.UpsertConnection(controldb.Connection{
		ID:             "conn-feishu",
		WorkspaceID:    workspaceID,
		Provider:       "feishu",
		ConnectionName: "default",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       ConnectionAuthAPIKey,
		Status:         "active",
	}); err != nil {
		t.Fatalf("upsert connection: %v", err)
	}
	if err := s.controlDB.UpsertAgentChannelBinding(binding); err != nil {
		t.Fatalf("upsert channel: %v", err)
	}
	if err := s.controlDB.UpsertAgentChannelTarget(controldb.AgentChannelTarget{
		ID:               "cht-release",
		WorkspaceID:      workspaceID,
		ChannelBindingID: "chan-feishu",
		Provider:         "feishu",
		TargetType:       "chat",
		DisplayName:      "发布评审群",
		ExternalChatID:   "oc_release",
	}); err != nil {
		t.Fatalf("upsert target: %v", err)
	}
	recipient, err := s.resolveRuntimeNotifyRecipient(runtimeAgentPrincipal{WorkspaceID: workspaceID, Project: "sample", Agent: "pm"}, "chat:发布评审群")
	if err != nil {
		t.Fatalf("resolve recipient: %v", err)
	}
	target, ok, err := s.runtimeNotifyTargetForRecipient(runtimeAgentPrincipal{
		WorkspaceID: workspaceID,
		Project:     "sample",
		Agent:       "pm",
	}, binding, recipient)
	if err != nil || !ok {
		t.Fatalf("target ok=%v err=%v", ok, err)
	}
	if target.ReceiveID != "oc_release" || target.ReceiveIDType != "chat_id" || target.ChatID != "oc_release" {
		t.Fatalf("unexpected target: %#v", target)
	}
}

func TestFormatRuntimeNotifyMarkdownPreservesBodyAndMetadata(t *testing.T) {
	msg := formatRuntimeNotifyMessage(runtimeAgentPrincipal{
		Project: "sample",
		Agent:   "pm",
	}, runtimeNotifyBody{
		MessageFormat: "markdown",
		TaskID:        "t-1",
		Urgency:       "review",
	}, "Need review", "## Summary\n\n- Impact: high\n- Decision: approve")
	if msg.Format != "markdown" || msg.Subject != "Need review" {
		t.Fatalf("unexpected message metadata: %#v", msg)
	}
	if !strings.Contains(msg.Text, "## Summary") || !strings.Contains(msg.Text, "- Impact: high") {
		t.Fatalf("markdown body was not preserved: %q", msg.Text)
	}
	if !strings.Contains(msg.Text, "From: `sample/pm`") || !strings.Contains(msg.Text, "Task: `t-1`") || !strings.Contains(msg.Text, "Urgency: `review`") {
		t.Fatalf("metadata missing: %q", msg.Text)
	}
}
