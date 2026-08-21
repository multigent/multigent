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
	"github.com/multigent/multigent/internal/imbridge"
	"github.com/multigent/multigent/internal/store"
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

func TestRuntimeAgentChannelBindingsUseAgentWorker(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.st.SaveProject("other", &entity.Project{Name: "other"}); err != nil {
		t.Fatalf("save other project: %v", err)
	}
	if err := s.controlDB.UpsertAgentWorker(controldb.AgentWorker{
		ID:          "aw-pm",
		WorkspaceID: workspaceID,
		Name:        "pm",
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("worker: %v", err)
	}
	for _, project := range []string{"sample", "other"} {
		if err := s.controlDB.UpsertProjectMembership(controldb.ProjectMembership{
			ID:               "pm-" + project,
			WorkspaceID:      workspaceID,
			ProjectID:        project,
			MemberType:       "agent_worker",
			MemberID:         "aw-pm",
			Title:            "pm",
			AttentionEnabled: true,
			CreatedAt:        now,
			UpdatedAt:        now,
		}); err != nil {
			t.Fatalf("membership %s: %v", project, err)
		}
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
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("connection: %v", err)
	}
	if err := s.controlDB.UpsertAgentChannelBinding(controldb.AgentChannelBinding{
		ID:            "chan-feishu",
		WorkspaceID:   workspaceID,
		AgentWorkerID: "aw-pm",
		ProjectID:     "sample",
		AgentID:       "pm",
		Provider:      "feishu",
		ConnectionID:  "conn-feishu",
		Status:        "connected",
		MetadataJSON:  `{}`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("binding: %v", err)
	}

	bindings, err := s.runtimeAgentChannelBindings(runtimeAgentPrincipal{
		WorkspaceID: workspaceID,
		Project:     "other",
		Agent:       "pm",
	})
	if err != nil {
		t.Fatalf("runtime bindings: %v", err)
	}
	if len(bindings) != 1 || bindings[0].ID != "chan-feishu" {
		t.Fatalf("expected worker channel binding from other project context, got %+v", bindings)
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

func TestRuntimeNotifyTargetUsesAttentionSourceChat(t *testing.T) {
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
	task := &entity.Task{
		ID:     "t-attention",
		Title:  "attention wakeup",
		Status: entity.TaskStatusInProgress,
		Prompt: "## Attention Signals\n\n" +
			"Source: `im_message` / `im:feishu:group:oc_source:user:admin`\n" +
			"Reason: `im_mention`\n" +
			"Refs: `{\"chatId\":\"oc_source\",\"chatType\":\"group\",\"messageId\":\"om_one\"}`\n" +
			"Payload: `{\"senderOpenId\":\"ou_sender\"}`\n",
	}
	if err := s.ts.AddTask("sample", "pm", task); err != nil {
		t.Fatalf("persist task: %v", err)
	}
	if err := s.controlDB.UpsertRuntimeRun(controldb.RuntimeRun{
		ID:          "run-attention",
		WorkspaceID: workspaceID,
		ProjectID:   "sample",
		AgentID:     "pm",
		TaskID:      task.ID,
		Status:      "running",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	principal := runtimeAgentPrincipal{
		WorkspaceID: workspaceID,
		Project:     "sample",
		Agent:       "pm",
		RunID:       "run-attention",
	}
	recipient, err := s.resolveRuntimeNotifyRecipient(principal, "source")
	if err != nil || recipient != "source" {
		t.Fatalf("recipient=%q err=%v", recipient, err)
	}
	target, ok, err := s.runtimeNotifyTargetForRecipient(principal, binding, recipient)
	if err != nil || !ok {
		t.Fatalf("target ok=%v err=%v", ok, err)
	}
	if target.ReceiveID != "oc_source" || target.ReceiveIDType != "chat_id" || target.ChatID != "oc_source" || target.ReplyToMessageID != "om_one" || target.MentionOpenID != "ou_sender" {
		t.Fatalf("unexpected target: %#v", target)
	}
}

func TestRuntimeNotifyTargetUsesLocalRunnerTaskIDAsSource(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	binding := controldb.AgentChannelBinding{
		ID:            "chan-feishu",
		WorkspaceID:   workspaceID,
		ProjectID:     "sample",
		AgentID:       "pm",
		AgentWorkerID: "aw-pm",
		Provider:      "feishu",
		ConnectionID:  "conn-feishu",
		Status:        "connected",
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
	task := &entity.Task{
		ID:     "t-local-attention",
		Title:  "local attention wakeup",
		Status: entity.TaskStatusInProgress,
		Prompt: "## Attention Signals\n\n" +
			"Source: `im_message` / `im:feishu:group:oc_local:user:admin`\n" +
			"Reason: `im_mention`\n" +
			"Refs: `{\"chatId\":\"oc_local\",\"chatType\":\"group\",\"messageId\":\"om_local\"}`\n",
	}
	if err := s.ts.AddTask("sample", "pm", task); err != nil {
		t.Fatalf("persist task: %v", err)
	}
	principal := runtimeAgentPrincipal{
		WorkspaceID:   workspaceID,
		Project:       "sample",
		Agent:         "pm",
		AgentWorkerID: "aw-pm",
		RunID:         task.ID,
	}
	target, ok, err := s.runtimeNotifyTargetForRecipient(principal, binding, "source")
	if err != nil || !ok {
		t.Fatalf("target ok=%v err=%v", ok, err)
	}
	if target.ReceiveID != "oc_local" || target.ReceiveIDType != "chat_id" || target.ChatID != "oc_local" || target.ReplyToMessageID != "om_local" {
		t.Fatalf("unexpected target: %#v", target)
	}
}

func TestRuntimeNotifySourceChatID(t *testing.T) {
	prompt := "Source: `im_message` / `im:lark:group:oc_lark:user:admin`\n" +
		"Payload: `{\"externalChatId\":\"oc_lark\"}`\n"
	chatID, ok := runtimeNotifySourceChatID(prompt, "lark")
	if !ok || chatID != "oc_lark" {
		t.Fatalf("chatID=%q ok=%v", chatID, ok)
	}
	if _, ok := runtimeNotifySourceChatID(prompt, "feishu"); ok {
		t.Fatalf("wrong provider should not match")
	}
}

func TestRuntimeNotifySourceFromPromptCombinesRefsAndPayload(t *testing.T) {
	prompt := "Source: `im_message` / `im:feishu:group:oc_one:user:admin`\n" +
		"Reason: `im_mention`\n" +
		"Refs: `{\"chatId\":\"oc_one\",\"chatType\":\"group\",\"messageId\":\"om_one\"}`\n" +
		"Payload: `{\"externalChatId\":\"oc_one\",\"senderOpenId\":\"ou_sender\"}`\n"
	source, ok := runtimeNotifySourceFromPrompt(prompt, "feishu")
	if !ok {
		t.Fatalf("source should resolve")
	}
	if source.ChatID != "oc_one" || source.ChatType != "group" || source.MessageID != "om_one" || source.SenderOpenID != "ou_sender" {
		t.Fatalf("unexpected source: %#v", source)
	}
}

func TestRuntimeNotifyCreateInteractionRequestForCard(t *testing.T) {
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
	card, interactionID, err := s.runtimeNotifyCreateInteractionRequest(runtimeAgentPrincipal{
		WorkspaceID: workspaceID,
		Project:     "sample",
		Agent:       "pm",
	}, binding, "owner", runtimeNotifyBody{
		Subject: "Decision",
		Body:    "Choose one",
		TaskID:  "t-1",
		Card: &runtimeNotifyCardBody{
			Actions: []runtimeNotifyCardActionBody{{ID: "approve", Label: "通过", Style: "primary"}},
			Fields:  []runtimeNotifyCardFieldBody{{Label: "风险", Value: "低"}},
			Context: map[string]any{"stepId": "review"},
		},
	}, "Choose one")
	if err != nil {
		t.Fatalf("create card request: %v", err)
	}
	if card == nil || card.InteractionID == "" || interactionID != card.InteractionID || len(card.Actions) != 1 {
		t.Fatalf("unexpected card: %#v id=%s", card, interactionID)
	}
	req, ok, err := s.controlDB.InteractionRequestByID(workspaceID, interactionID)
	if err != nil || !ok {
		t.Fatalf("lookup interaction ok=%v err=%v", ok, err)
	}
	if req.TargetUserID != "owner" || req.HandlerType != "agent_event" || !strings.Contains(req.ContextJSON, "t-1") {
		t.Fatalf("unexpected request: %#v", req)
	}
}

func TestRuntimeNotifyCreateInteractionRequestHumanDoesNotLockToLiteralHuman(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	_, interactionID, err := s.runtimeNotifyCreateInteractionRequest(runtimeAgentPrincipal{
		WorkspaceID: workspaceID,
		Project:     "sample",
		Agent:       "pm",
	}, controldb.AgentChannelBinding{ID: "chan-feishu", WorkspaceID: workspaceID, ProjectID: "sample", AgentID: "pm", Provider: "feishu"}, "human", runtimeNotifyBody{
		Body: "Choose one",
		Card: &runtimeNotifyCardBody{Actions: []runtimeNotifyCardActionBody{{ID: "ack", Label: "收到"}}},
	}, "Choose one")
	if err != nil {
		t.Fatalf("create card request: %v", err)
	}
	req, ok, err := s.controlDB.InteractionRequestByID(workspaceID, interactionID)
	if err != nil || !ok {
		t.Fatalf("lookup interaction ok=%v err=%v", ok, err)
	}
	if req.TargetUserID != "" {
		t.Fatalf("human recipient should not lock to literal user, got %#v", req)
	}
}

func TestFormatRuntimeNotifyMarkdownPreservesAgentBodyWithoutPlatformFooter(t *testing.T) {
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
	if strings.Contains(msg.Text, "From:") || strings.Contains(msg.Text, "Task:") || strings.Contains(msg.Text, "Urgency:") {
		t.Fatalf("external message should not include platform footer: %q", msg.Text)
	}
}

func TestRuntimeNotifyEnrichesMarkdownDocLinks(t *testing.T) {
	s, _ := newConnectionGrantPolicyServer(t)
	t.Setenv("MULTIGENT_WEB_BASE_URL", "https://public.multigent.test")
	ds := store.NewDocsStore(s.root)
	if err := ds.AddManagedContent(&store.DocEntry{
		ID:    "doc-20260822-abcd12",
		Title: "发布评审说明",
	}, "# 发布评审说明\n", "review.md"); err != nil {
		t.Fatalf("add doc: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/notify", nil)
	req.Host = "127.0.0.1:27892"

	got := s.enrichRuntimeNotifyDocLinks(req, "markdown", "请看 doc-20260822-abcd12。")
	if !strings.Contains(got, "相关文档") || !strings.Contains(got, "[发布评审说明](https://public.multigent.test/docs/doc-20260822-abcd12)") || !strings.Contains(got, "`doc-20260822-abcd12`") {
		t.Fatalf("doc link was not enriched: %q", got)
	}
}

func TestRuntimeNotifyDocLinksIgnoreUnknownIDs(t *testing.T) {
	s, _ := newConnectionGrantPolicyServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/notify", nil)
	got := s.enrichRuntimeNotifyDocLinks(req, "markdown", "请看 doc-20260822-missing。")
	if got != "请看 doc-20260822-missing。" {
		t.Fatalf("unknown doc ids should not be linked: %q", got)
	}
}

func TestFormatRuntimeNotifyTextPreservesAgentBodyWithoutPlatformFooter(t *testing.T) {
	msg := formatRuntimeNotifyMessage(runtimeAgentPrincipal{
		Project: "sample",
		Agent:   "pm",
	}, runtimeNotifyBody{
		TaskID:  "t-1",
		Urgency: "review",
	}, "Need review", "Please review this.")
	if msg.Format != "text" || msg.Subject != "Need review" {
		t.Fatalf("unexpected message metadata: %#v", msg)
	}
	if msg.Text != "Please review this." {
		t.Fatalf("unexpected text body: %q", msg.Text)
	}
	if strings.Contains(msg.Text, "From:") || strings.Contains(msg.Text, "Task:") || strings.Contains(msg.Text, "Urgency:") {
		t.Fatalf("external message should not include platform footer: %q", msg.Text)
	}
}

func TestPrepareRuntimeNotifySourceReplyHidesExternalSubject(t *testing.T) {
	msg := formatRuntimeNotifyMessage(runtimeAgentPrincipal{
		Project: "sample",
		Agent:   "pm",
	}, runtimeNotifyBody{
		MessageFormat: "markdown",
	}, "Re: 你实际上比较擅长什么呢", "## 结论\n\n- 擅长结构化分流")
	prepareRuntimeNotifyExternalMessage(&msg, imbridge.OutgoingTarget{ReplyToMessageID: "om_one"})
	if msg.Subject != "" {
		t.Fatalf("source reply should hide external subject, got %q", msg.Subject)
	}
	if !strings.Contains(msg.Text, "## 结论") {
		t.Fatalf("body should be preserved: %q", msg.Text)
	}
}

func TestPrepareRuntimeNotifyDirectSendKeepsExternalSubject(t *testing.T) {
	msg := formatRuntimeNotifyMessage(runtimeAgentPrincipal{
		Project: "sample",
		Agent:   "pm",
	}, runtimeNotifyBody{
		MessageFormat: "markdown",
	}, "Review needed", "## 结论\n\n- 请确认")
	prepareRuntimeNotifyExternalMessage(&msg, imbridge.OutgoingTarget{ReceiveID: "ou_owner", ReceiveIDType: "open_id"})
	if msg.Subject != "Review needed" {
		t.Fatalf("direct notification should keep subject, got %q", msg.Subject)
	}
}
