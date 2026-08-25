package api

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/imbridge"
	"github.com/multigent/multigent/internal/interaction"
)

type testIMProvider struct {
	id          string
	label       string
	replies     []string
	messages    []imbridge.OutgoingMessage
	cardUpdates []imbridge.OutgoingMessage
}

func (p *testIMProvider) Info() imbridge.ProviderInfo {
	return imbridge.ProviderInfo{ID: p.id, Label: p.label, SetupMode: "manual"}
}
func (p *testIMProvider) OpenBaseURL() string { return "" }
func (p *testIMProvider) BeginSetup(context.Context) (imbridge.SetupBeginResponse, error) {
	return imbridge.SetupBeginResponse{}, nil
}
func (p *testIMProvider) PollSetup(context.Context, string, string) (imbridge.SetupPollResponse, error) {
	return imbridge.SetupPollResponse{}, nil
}
func (p *testIMProvider) ManualSetup(context.Context, imbridge.ManualSetupRequest) (imbridge.ManualSetupResult, error) {
	return imbridge.ManualSetupResult{}, nil
}
func (p *testIMProvider) ExtractEncryptedPayload([]byte) (string, bool) { return "", false }
func (p *testIMProvider) DecryptEvent(string, string) ([]byte, error)   { return nil, nil }
func (p *testIMProvider) ParseEvent([]byte) (imbridge.ParsedEvent, error) {
	return imbridge.ParsedEvent{}, nil
}
func (p *testIMProvider) ShouldHandleMessage(string, imbridge.IncomingMessage) bool { return true }
func (p *testIMProvider) ReplyText(ctx context.Context, secrets map[string]string, message imbridge.IncomingMessage, text string) error {
	p.replies = append(p.replies, text)
	return nil
}
func (p *testIMProvider) SendText(context.Context, map[string]string, imbridge.OutgoingTarget, string) error {
	return nil
}
func (p *testIMProvider) SendMessage(context.Context, map[string]string, imbridge.OutgoingTarget, imbridge.OutgoingMessage) error {
	return nil
}
func (p *testIMProvider) ReplyMessage(ctx context.Context, secrets map[string]string, message imbridge.IncomingMessage, reply imbridge.OutgoingMessage) error {
	p.messages = append(p.messages, reply)
	return nil
}
func (p *testIMProvider) UpdateInteractionCard(ctx context.Context, secrets map[string]string, callback imbridge.IncomingInteractionCallback, message imbridge.OutgoingMessage) error {
	p.cardUpdates = append(p.cardUpdates, message)
	return nil
}

func TestParseAgentChannelControlCommandAllowsLeadingMentions(t *testing.T) {
	tests := []struct {
		name string
		text string
		ok   bool
	}{
		{name: "plain", text: "/status", ok: true},
		{name: "leading mention", text: "@bot /status", ok: true},
		{name: "lark mention token", text: "@_user_1 /status", ok: true},
		{name: "normal sentence", text: "please run /status", ok: false},
		{name: "unknown", text: "/help", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, ok := parseAgentChannelControlCommand(tt.text)
			if ok != tt.ok {
				t.Fatalf("ok=%v, want %v, cmd=%q", ok, tt.ok, cmd)
			}
			if ok && cmd != "status" {
				t.Fatalf("cmd=%q, want status", cmd)
			}
		})
	}
}

func TestAgentChannelStatusCommandRepliesWithoutWakeup(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC()
	next := now.Add(30 * time.Minute)
	scheduleRaw, _ := json.Marshal(entity.HeartbeatConfig{
		Enabled:          true,
		Interval:         "30m",
		ActiveHours:      "09:00-18:00",
		ActiveDays:       "weekdays",
		Triggers:         []entity.TriggerType{entity.TriggerOnAttention},
		TriggerDebounce:  "45s",
		Jitter:           "2m",
		LastWakeup:       &now,
		LastWakeupStatus: "done",
		NextWakeupAt:     &next,
		WakeupCount:      12,
		WakeupCountToday: 2,
	})
	if err := s.controlDB.UpsertModelProvider(workspaceID, controldb.ModelProvider{
		ID:          "prov-codex",
		WorkspaceID: workspaceID,
		Name:        "Codex Official",
		Model:       "gpt-5.5",
	}); err != nil {
		t.Fatalf("model provider: %v", err)
	}
	if err := s.controlDB.UpsertAgentWorker(controldb.AgentWorker{
		ID:                    "aw-pm",
		WorkspaceID:           workspaceID,
		Name:                  "pm",
		DisplayName:           "PM",
		Team:                  "product",
		Role:                  "pm",
		Status:                "active",
		Model:                 "codex",
		RuntimeModel:          "gpt-5.5",
		DefaultModelAccountID: "prov-codex",
		DefaultRuntimeNodeID:  "node-local",
		DefaultRuntimeMode:    "runtime_node",
		ScheduleJSON:          string(scheduleRaw),
		PrimarySessionID:      "sess-primary",
	}); err != nil {
		t.Fatalf("worker: %v", err)
	}
	if err := s.controlDB.UpsertConnection(controldb.Connection{
		ID:             "conn-feishu",
		WorkspaceID:    workspaceID,
		Provider:       "feishu",
		ConnectionName: "agent-pm",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       "app_secret",
		Status:         "active",
		ProfileJSON:    "{}",
	}); err != nil {
		t.Fatalf("connection: %v", err)
	}
	secret, err := sealConnectionSecret(map[string]string{"baseUrl": "https://open.feishu.cn", "appId": "cli_app", "appSecret": "secret"})
	if err != nil {
		t.Fatalf("seal secret: %v", err)
	}
	secret.ConnectionID = "conn-feishu"
	if err := s.controlDB.UpsertConnectionSecret(secret); err != nil {
		t.Fatalf("secret: %v", err)
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
		MetadataJSON:  `{"appId":"cli_app"}`,
	}); err != nil {
		t.Fatalf("binding: %v", err)
	}
	if err := s.controlDB.UpsertExternalIdentity(controldb.ExternalIdentity{
		ID:             "ext-owner",
		WorkspaceID:    workspaceID,
		Provider:       "feishu",
		ExternalUserID: "ou_owner",
		UserID:         "owner",
	}); err != nil {
		t.Fatalf("identity: %v", err)
	}

	provider := &testIMProvider{id: "feishu", label: "Feishu"}
	result, err := s.acceptIMMessage(provider, "cli_app", "", imbridge.IncomingMessage{
		MessageID:    "om_status",
		ChatID:       "oc_p2p",
		ChatType:     "p2p",
		SenderOpenID: "ou_owner",
		Text:         "/status",
	}, "")
	if err != nil {
		t.Fatalf("accept status: %v", err)
	}
	if result["command"] != "status" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(provider.messages) != 1 {
		t.Fatalf("expected one rich status reply, got messages=%#v replies=%#v", provider.messages, provider.replies)
	}
	if provider.messages[0].Format != "markdown" {
		t.Fatalf("status reply should use markdown, got %#v", provider.messages[0])
	}
	if provider.messages[0].Subject != "智能体状态" {
		t.Fatalf("status reply subject=%q, want 智能体状态", provider.messages[0].Subject)
	}
	reply := provider.messages[0].Text
	for _, want := range []string{
		"PM 状态",
		"标识: `pm`",
		"运行时: codex",
		"模型: gpt-5.5",
		"模型账号: Codex Official (gpt-5.5)",
		"运行节点: node-local",
		"间隔: 30m",
		"触发器: attention",
		"唤醒次数: 总计 12 / 今日 2",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("status reply missing %q:\n%s", want, reply)
		}
	}
	signals, err := s.controlDB.ListAttentionSignals(controldb.AttentionSignalFilter{WorkspaceID: workspaceID, AgentWorkerID: "aw-pm"})
	if err != nil {
		t.Fatalf("list signals: %v", err)
	}
	if len(signals) != 0 {
		t.Fatalf("status command should not create attention signals: %#v", signals)
	}
}

func TestChannelEventBindingRequiresExternalIdentity(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	if err := s.controlDB.UpsertConnection(controldb.Connection{
		ID:             "conn-feishu",
		WorkspaceID:    workspaceID,
		Provider:       "feishu",
		ConnectionName: "agent-sample-pm",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       "app_secret",
		Status:         "active",
		ProfileJSON:    "{}",
	}); err != nil {
		t.Fatalf("connection: %v", err)
	}
	secret, err := sealConnectionSecret(map[string]string{"baseUrl": "https://open.feishu.cn", "appId": "cli_app", "appSecret": "secret"})
	if err != nil {
		t.Fatalf("seal secret: %v", err)
	}
	secret.ConnectionID = "conn-feishu"
	if err := s.controlDB.UpsertConnectionSecret(secret); err != nil {
		t.Fatalf("secret: %v", err)
	}
	if err := s.controlDB.UpsertAgentChannelBinding(controldb.AgentChannelBinding{
		ID:           "chan-feishu",
		WorkspaceID:  workspaceID,
		ProjectID:    "sample",
		AgentID:      "pm",
		Provider:     "feishu",
		ConnectionID: "conn-feishu",
		Status:       "connected",
		MetadataJSON: `{"appId":"cli_app"}`,
	}); err != nil {
		t.Fatalf("binding: %v", err)
	}

	if _, found, err := s.resolveChannelEventBinding("feishu", "cli_app", "", "ou_missing"); err != nil || found {
		t.Fatalf("missing identity found=%v err=%v", found, err)
	}
	if err := s.controlDB.UpsertExternalIdentity(controldb.ExternalIdentity{
		ID:             "ext-feishu",
		WorkspaceID:    workspaceID,
		Provider:       "feishu",
		ExternalUserID: "ou_owner",
		UserID:         "owner",
	}); err != nil {
		t.Fatalf("identity: %v", err)
	}
	resolved, found, err := s.resolveChannelEventBinding("feishu", "cli_app", "", "ou_owner")
	if err != nil || !found {
		t.Fatalf("resolve found=%v err=%v", found, err)
	}
	if resolved.Identity.UserID != "owner" || resolved.Binding.ID != "chan-feishu" || resolved.SecretValues["appSecret"] != "secret" {
		t.Fatalf("resolved=%#v secrets=%#v", resolved, resolved.SecretValues)
	}
}

func TestAgentChannelBindCommandLinksExternalUserToChannel(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	if err := s.users.CreateUser("reviewer", "pass123", RoleMember, "", "", "", "", ""); err != nil {
		t.Fatalf("create reviewer: %v", err)
	}
	if err := s.controlDB.UpsertWorkspaceMember(workspaceID, "reviewer", WorkspaceRoleMember); err != nil {
		t.Fatalf("reviewer member: %v", err)
	}
	if err := s.controlDB.UpsertConnection(controldb.Connection{
		ID:             "conn-feishu",
		WorkspaceID:    workspaceID,
		Provider:       "feishu",
		ConnectionName: "agent-sample-pm",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       "app_secret",
		Status:         "active",
		ProfileJSON:    "{}",
	}); err != nil {
		t.Fatalf("connection: %v", err)
	}
	secret, err := sealConnectionSecret(map[string]string{"baseUrl": "https://open.feishu.cn", "appId": "cli_app", "appSecret": "secret"})
	if err != nil {
		t.Fatalf("seal secret: %v", err)
	}
	secret.ConnectionID = "conn-feishu"
	if err := s.controlDB.UpsertConnectionSecret(secret); err != nil {
		t.Fatalf("secret: %v", err)
	}
	if err := s.controlDB.UpsertAgentChannelBinding(controldb.AgentChannelBinding{
		ID:           "chan-feishu",
		WorkspaceID:  workspaceID,
		ProjectID:    "sample",
		AgentID:      "pm",
		Provider:     "feishu",
		ConnectionID: "conn-feishu",
		Status:       "connected",
		MetadataJSON: `{"appId":"cli_app"}`,
	}); err != nil {
		t.Fatalf("binding: %v", err)
	}
	if err := s.controlDB.CreateAgentChannelBindCode(controldb.AgentChannelBindCode{
		Code:             "MG-ABC12345",
		WorkspaceID:      workspaceID,
		ChannelBindingID: "chan-feishu",
		UserID:           "owner",
		ExpiresAt:        time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("bind code: %v", err)
	}
	if err := s.controlDB.CreateAgentChannelBindCode(controldb.AgentChannelBindCode{
		Code:             "MG-DEF67890",
		WorkspaceID:      workspaceID,
		ChannelBindingID: "chan-feishu",
		UserID:           "reviewer",
		ExpiresAt:        time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("second bind code: %v", err)
	}
	provider := &testIMProvider{id: "feishu", label: "Feishu"}
	result, err := s.acceptAgentChannelBindCommand(provider, "cli_app", "", imbridge.IncomingMessage{
		MessageID:    "om_msg",
		ChatID:       "oc_dm",
		SenderOpenID: "ou_sender",
		Text:         "/bind MG-ABC12345",
	}, "bind", "MG-ABC12345")
	if err != nil {
		t.Fatalf("bind command: %v", err)
	}
	if result["bound"] != true {
		t.Fatalf("unexpected result: %#v", result)
	}
	result, err = s.acceptAgentChannelBindCommand(provider, "cli_app", "", imbridge.IncomingMessage{
		MessageID:    "om_msg_2",
		ChatID:       "oc_reviewer_dm",
		SenderOpenID: "ou_reviewer",
		Text:         "/bind MG-DEF67890",
	}, "bind", "MG-DEF67890")
	if err != nil {
		t.Fatalf("second bind command: %v", err)
	}
	if result["bound"] != true {
		t.Fatalf("unexpected second result: %#v", result)
	}
	identities, err := s.controlDB.ListUserChannelIdentities(controldb.UserChannelIdentityFilter{
		WorkspaceID:      workspaceID,
		UserID:           "owner",
		ChannelBindingID: "chan-feishu",
		Provider:         "feishu",
	})
	if err != nil {
		t.Fatalf("list identities: %v", err)
	}
	if len(identities) != 1 || identities[0].ExternalUserID != "ou_sender" || identities[0].ExternalChatID != "oc_dm" {
		t.Fatalf("unexpected identities: %#v", identities)
	}
	identities, err = s.controlDB.ListUserChannelIdentities(controldb.UserChannelIdentityFilter{
		WorkspaceID:      workspaceID,
		ChannelBindingID: "chan-feishu",
		Provider:         "feishu",
	})
	if err != nil {
		t.Fatalf("list all identities: %v", err)
	}
	if len(identities) != 2 {
		t.Fatalf("expected two users bound to one agent channel, got %#v", identities)
	}
	code, found, err := s.controlDB.AgentChannelBindCodeByCode("MG-ABC12345")
	if err != nil || !found || code.UsedAt == "" {
		t.Fatalf("code found=%v usedAt=%q err=%v", found, code.UsedAt, err)
	}
	if len(provider.replies) != 2 || !strings.Contains(provider.replies[0], "绑定成功") || !strings.Contains(provider.replies[1], "绑定成功") {
		t.Fatalf("unexpected replies: %#v", provider.replies)
	}
}

func TestAgentChannelBindCommandLinksChatTargetToChannel(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	if err := s.controlDB.UpsertConnection(controldb.Connection{
		ID:             "conn-feishu",
		WorkspaceID:    workspaceID,
		Provider:       "feishu",
		ConnectionName: "agent-sample-pm",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       "app_secret",
		Status:         "active",
		ProfileJSON:    "{}",
	}); err != nil {
		t.Fatalf("connection: %v", err)
	}
	secret, err := sealConnectionSecret(map[string]string{"baseUrl": "https://open.feishu.cn", "appId": "cli_app", "appSecret": "secret"})
	if err != nil {
		t.Fatalf("seal secret: %v", err)
	}
	secret.ConnectionID = "conn-feishu"
	if err := s.controlDB.UpsertConnectionSecret(secret); err != nil {
		t.Fatalf("secret: %v", err)
	}
	if err := s.controlDB.UpsertAgentChannelBinding(controldb.AgentChannelBinding{
		ID:           "chan-feishu",
		WorkspaceID:  workspaceID,
		ProjectID:    "sample",
		AgentID:      "pm",
		Provider:     "feishu",
		ConnectionID: "conn-feishu",
		Status:       "connected",
		MetadataJSON: `{"appId":"cli_app"}`,
	}); err != nil {
		t.Fatalf("binding: %v", err)
	}
	if err := s.controlDB.CreateAgentChannelBindCode(controldb.AgentChannelBindCode{
		Code:             "MG-CHAT123",
		WorkspaceID:      workspaceID,
		ChannelBindingID: "chan-feishu",
		UserID:           "owner",
		TargetType:       "chat",
		TargetName:       "发布评审群",
		ExpiresAt:        time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("bind code: %v", err)
	}
	provider := &testIMProvider{id: "feishu", label: "Feishu"}
	result, err := s.acceptAgentChannelBindCommand(provider, "cli_app", "", imbridge.IncomingMessage{
		MessageID:    "om_group_msg",
		ChatID:       "oc_release_room",
		ChatType:     "group",
		SenderOpenID: "ou_sender",
		Text:         "/bind-chat MG-CHAT123",
	}, "bind-chat", "MG-CHAT123")
	if err != nil {
		t.Fatalf("bind command: %v", err)
	}
	if result["bound"] != true || result["target"] != "chat" {
		t.Fatalf("unexpected result: %#v", result)
	}
	targets, err := s.controlDB.ListAgentChannelTargets(controldb.AgentChannelTargetFilter{
		WorkspaceID:      workspaceID,
		ChannelBindingID: "chan-feishu",
		Provider:         "feishu",
		TargetType:       "chat",
	})
	if err != nil {
		t.Fatalf("list targets: %v", err)
	}
	if len(targets) != 1 || targets[0].DisplayName != "发布评审群" || targets[0].ExternalChatID != "oc_release_room" {
		t.Fatalf("unexpected targets: %#v", targets)
	}
	updated, ok, err := s.controlDB.AgentChannelBindingByID("chan-feishu")
	if err != nil || !ok {
		t.Fatalf("binding lookup ok=%v err=%v", ok, err)
	}
	if updated.ExternalChatID != "oc_release_room" {
		t.Fatalf("expected binding chat id to be updated, got %#v", updated)
	}
	if len(provider.replies) != 1 || !strings.Contains(provider.replies[0], "群聊绑定成功") || !strings.Contains(provider.replies[0], "发布评审群") {
		t.Fatalf("unexpected replies: %#v", provider.replies)
	}
}

func TestChannelEventBindingResolvesAcrossWorkspaces(t *testing.T) {
	s, currentWorkspaceID := newConnectionGrantPolicyServer(t)
	workspaceID := "ws-second"
	if err := s.controlDB.UpsertWorkspace(controldb.Workspace{ID: workspaceID, Name: "Second", Slug: "second"}); err != nil {
		t.Fatalf("workspace: %v", err)
	}
	if err := s.users.CreateUser("second-owner", "pass123", RoleMember, "", "", "", "", ""); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := s.controlDB.UpsertWorkspaceMember(workspaceID, "second-owner", WorkspaceRoleMember); err != nil {
		t.Fatalf("workspace member: %v", err)
	}
	if err := s.controlDB.UpsertConnection(controldb.Connection{
		ID:             "conn-feishu-second",
		WorkspaceID:    workspaceID,
		Provider:       "feishu",
		ConnectionName: "agent-sample-pm",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       "app_secret",
		Status:         "active",
		ProfileJSON:    "{}",
	}); err != nil {
		t.Fatalf("connection: %v", err)
	}
	secret, err := sealConnectionSecret(map[string]string{"baseUrl": "https://open.feishu.cn", "appId": "cli_second", "appSecret": "second-secret"})
	if err != nil {
		t.Fatalf("seal secret: %v", err)
	}
	secret.ConnectionID = "conn-feishu-second"
	if err := s.controlDB.UpsertConnectionSecret(secret); err != nil {
		t.Fatalf("secret: %v", err)
	}
	if err := s.controlDB.UpsertAgentChannelBinding(controldb.AgentChannelBinding{
		ID:           "chan-feishu-second",
		WorkspaceID:  workspaceID,
		ProjectID:    "sample",
		AgentID:      "pm",
		Provider:     "feishu",
		ConnectionID: "conn-feishu-second",
		Status:       "connected",
		MetadataJSON: `{"appId":"cli_second"}`,
	}); err != nil {
		t.Fatalf("binding: %v", err)
	}
	if err := s.controlDB.UpsertExternalIdentity(controldb.ExternalIdentity{
		ID:             "ext-feishu-second",
		WorkspaceID:    workspaceID,
		Provider:       "feishu",
		ExternalUserID: "ou_second",
		UserID:         "second-owner",
	}); err != nil {
		t.Fatalf("identity: %v", err)
	}

	resolved, found, err := s.resolveChannelEventBinding("feishu", "cli_second", "", "ou_second")
	if err != nil || !found {
		t.Fatalf("resolve found=%v err=%v", found, err)
	}
	if currentWorkspaceID == workspaceID {
		t.Fatalf("test setup expected a distinct current workspace")
	}
	if resolved.Binding.WorkspaceID != workspaceID || resolved.Binding.ID != "chan-feishu-second" || resolved.SecretValues["appSecret"] != "second-secret" {
		t.Fatalf("resolved wrong workspace binding: %#v secrets=%#v", resolved.Binding, resolved.SecretValues)
	}
}

func TestChannelEventBindingRecordsUnknownIdentityOnMatchedChannel(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	if err := s.controlDB.UpsertConnection(controldb.Connection{
		ID:             "conn-feishu",
		WorkspaceID:    workspaceID,
		Provider:       "feishu",
		ConnectionName: "agent-sample-pm",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       "app_secret",
		Status:         "active",
		ProfileJSON:    "{}",
	}); err != nil {
		t.Fatalf("connection: %v", err)
	}
	secret, err := sealConnectionSecret(map[string]string{"baseUrl": "https://open.feishu.cn", "appId": "cli_app", "appSecret": "secret"})
	if err != nil {
		t.Fatalf("seal secret: %v", err)
	}
	secret.ConnectionID = "conn-feishu"
	if err := s.controlDB.UpsertConnectionSecret(secret); err != nil {
		t.Fatalf("secret: %v", err)
	}
	if err := s.controlDB.UpsertAgentChannelBinding(controldb.AgentChannelBinding{
		ID:           "chan-feishu",
		WorkspaceID:  workspaceID,
		ProjectID:    "sample",
		AgentID:      "pm",
		Provider:     "feishu",
		ConnectionID: "conn-feishu",
		Status:       "connected",
		MetadataJSON: `{"appId":"cli_app"}`,
	}); err != nil {
		t.Fatalf("binding: %v", err)
	}

	resolution, err := s.resolveChannelEventBindingDetailed("feishu", "cli_app", "oc_one", "ou_unknown")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolution.Found || !resolution.HasCandidate || resolution.Candidate.ID != "chan-feishu" {
		t.Fatalf("unexpected resolution: %#v", resolution)
	}
	s.recordAgentChannelCallback(resolution.Candidate, "rejected", "unknown_identity", imbridge.IncomingMessage{
		MessageID:    "om_unknown",
		ChatID:       "oc_one",
		ChatType:     "p2p",
		SenderOpenID: "ou_unknown",
		Text:         "hello",
	}, "")
	updated, ok, err := s.controlDB.AgentChannelBindingByID("chan-feishu")
	if err != nil || !ok {
		t.Fatalf("load binding ok=%v err=%v", ok, err)
	}
	resp := agentChannelToResponse(updated)
	if resp.Callback.Status != "rejected" || resp.Callback.Reason != "unknown_identity" || resp.Callback.MessageID != "om_unknown" {
		t.Fatalf("callback metadata not recorded: %#v", resp.Callback)
	}
}

func TestAcceptIMMessageAutoBindsIdentityByEmail(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	if err := s.users.CreateUser("glenn", "pass123", RoleMember, "Glenn", "glenn@example.com", "", "", ""); err != nil {
		t.Fatalf("create glenn: %v", err)
	}
	if err := s.controlDB.UpsertWorkspaceMember(workspaceID, "glenn", WorkspaceRoleAdmin); err != nil {
		t.Fatalf("workspace member: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "tenant-token"})
		case "/open-apis/contact/v3/users/ou_glenn":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"user": map[string]any{
					"open_id": "ou_glenn",
					"name":    "Glenn",
					"email":   "glenn@example.com",
				}},
			})
		case "/open-apis/im/v1/messages/om_auto/reactions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"reaction_id": "reaction-one"},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.String())
		}
	}))
	defer server.Close()
	if err := s.controlDB.UpsertConnection(controldb.Connection{
		ID:             "conn-feishu-auto",
		WorkspaceID:    workspaceID,
		Provider:       "feishu",
		ConnectionName: "agent-sample-pm",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       "app_secret",
		Status:         "active",
		ProfileJSON:    "{}",
	}); err != nil {
		t.Fatalf("connection: %v", err)
	}
	secret, err := sealConnectionSecret(map[string]string{"baseUrl": server.URL, "appId": "cli_app", "appSecret": "secret"})
	if err != nil {
		t.Fatalf("seal secret: %v", err)
	}
	secret.ConnectionID = "conn-feishu-auto"
	if err := s.controlDB.UpsertConnectionSecret(secret); err != nil {
		t.Fatalf("secret: %v", err)
	}
	if err := s.controlDB.UpsertAgentChannelBinding(controldb.AgentChannelBinding{
		ID:           "chan-feishu-auto",
		WorkspaceID:  workspaceID,
		ProjectID:    "sample",
		AgentID:      "pm",
		Provider:     "feishu",
		ConnectionID: "conn-feishu-auto",
		Status:       "connected",
		MetadataJSON: `{"appId":"cli_app"}`,
	}); err != nil {
		t.Fatalf("binding: %v", err)
	}
	provider, ok := imbridge.LookupProvider("feishu")
	if !ok {
		t.Fatalf("feishu provider missing")
	}
	result, err := s.acceptIMMessage(provider, "cli_app", "", imbridge.IncomingMessage{
		MessageID:    "om_auto",
		ChatID:       "oc_p2p",
		ChatType:     "p2p",
		SenderOpenID: "ou_glenn",
		Text:         "hello",
	}, "")
	if err != nil {
		t.Fatalf("accept message: %v", err)
	}
	if result["queued"] != true {
		t.Fatalf("message should be queued after auto bind: %#v", result)
	}
	identities, err := s.controlDB.ListUserChannelIdentities(controldb.UserChannelIdentityFilter{
		WorkspaceID:      workspaceID,
		ChannelBindingID: "chan-feishu-auto",
		ExternalUserID:   "ou_glenn",
	})
	if err != nil {
		t.Fatalf("list identities: %v", err)
	}
	if len(identities) != 1 || identities[0].UserID != "glenn" || identities[0].CreatedBy != "auto" {
		t.Fatalf("unexpected identities: %#v", identities)
	}
}

func TestAcceptIMMessageAutoBindsIdentityByWorkspaceConnectionUnionID(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	if err := s.users.CreateUser("joey", "pass123", RoleMember, "Joey", "joey@example.com", "", "", ""); err != nil {
		t.Fatalf("create joey: %v", err)
	}
	if err := s.controlDB.UpsertWorkspaceMember(workspaceID, "joey", WorkspaceRoleAdmin); err != nil {
		t.Fatalf("workspace member: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			token := "tenant-agent"
			if body["app_id"] == "cli_workspace" {
				token = "tenant-workspace"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": token})
		case "/open-apis/contact/v3/users/ou_joey":
			if r.URL.Query().Get("user_id_type") != "open_id" {
				t.Fatalf("expected open_id lookup, got %s", r.URL.RawQuery)
			}
			if auth := r.Header.Get("Authorization"); auth != "Bearer tenant-agent" {
				t.Fatalf("unexpected auth header: %s", auth)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"user": map[string]any{
					"open_id":  "ou_joey",
					"union_id": "on_joey",
					"name":     "Joey without email",
				}},
			})
		case "/open-apis/contact/v3/users/on_joey":
			if r.URL.Query().Get("user_id_type") != "union_id" {
				t.Fatalf("expected union_id lookup, got %s", r.URL.RawQuery)
			}
			if auth := r.Header.Get("Authorization"); auth != "Bearer tenant-workspace" {
				t.Fatalf("unexpected auth header: %s", auth)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"user": map[string]any{
					"open_id":  "ou_workspace_joey",
					"union_id": "on_joey",
					"name":     "Joey",
					"email":    "joey@example.com",
				}},
			})
		case "/open-apis/im/v1/messages/om_auto_union/reactions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"reaction_id": "reaction-one"},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.String())
		}
	}))
	defer server.Close()
	if err := s.controlDB.UpsertConnection(controldb.Connection{
		ID:             "conn-feishu-agent",
		WorkspaceID:    workspaceID,
		Provider:       "feishu",
		ConnectionName: "agent-sample-pm",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       "app_secret",
		Status:         "active",
		ProfileJSON:    `{"purpose":"agent_channel","usage":"agent_im_channel"}`,
	}); err != nil {
		t.Fatalf("agent connection: %v", err)
	}
	agentSecret, err := sealConnectionSecret(map[string]string{"baseUrl": server.URL, "appId": "cli_agent", "appSecret": "agent-secret"})
	if err != nil {
		t.Fatalf("seal agent secret: %v", err)
	}
	agentSecret.ConnectionID = "conn-feishu-agent"
	if err := s.controlDB.UpsertConnectionSecret(agentSecret); err != nil {
		t.Fatalf("agent secret: %v", err)
	}
	if err := s.controlDB.UpsertConnection(controldb.Connection{
		ID:             "conn-feishu-workspace",
		WorkspaceID:    workspaceID,
		Provider:       "feishu",
		ConnectionName: "default",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       "app_secret",
		Status:         "active",
		ProfileJSON:    `{"displayName":"Feishu Directory"}`,
	}); err != nil {
		t.Fatalf("workspace connection: %v", err)
	}
	workspaceSecret, err := sealConnectionSecret(map[string]string{"baseUrl": server.URL, "appId": "cli_workspace", "appSecret": "workspace-secret"})
	if err != nil {
		t.Fatalf("seal workspace secret: %v", err)
	}
	workspaceSecret.ConnectionID = "conn-feishu-workspace"
	if err := s.controlDB.UpsertConnectionSecret(workspaceSecret); err != nil {
		t.Fatalf("workspace secret: %v", err)
	}
	if err := s.controlDB.UpsertAgentChannelBinding(controldb.AgentChannelBinding{
		ID:           "chan-feishu-auto-union",
		WorkspaceID:  workspaceID,
		ProjectID:    "sample",
		AgentID:      "pm",
		Provider:     "feishu",
		ConnectionID: "conn-feishu-agent",
		Status:       "connected",
		MetadataJSON: `{"appId":"cli_agent"}`,
	}); err != nil {
		t.Fatalf("binding: %v", err)
	}
	provider, ok := imbridge.LookupProvider("feishu")
	if !ok {
		t.Fatalf("feishu provider missing")
	}
	result, err := s.acceptIMMessage(provider, "cli_agent", "", imbridge.IncomingMessage{
		MessageID:    "om_auto_union",
		ChatID:       "oc_p2p",
		ChatType:     "p2p",
		SenderOpenID: "ou_joey",
		Text:         "hello",
	}, "")
	if err != nil {
		t.Fatalf("accept message: %v", err)
	}
	if result["queued"] != true {
		t.Fatalf("message should be queued after auto bind: %#v", result)
	}
	identities, err := s.controlDB.ListUserChannelIdentities(controldb.UserChannelIdentityFilter{
		WorkspaceID:      workspaceID,
		ChannelBindingID: "chan-feishu-auto-union",
		ExternalUserID:   "ou_joey",
	})
	if err != nil {
		t.Fatalf("list identities: %v", err)
	}
	if len(identities) != 1 || identities[0].UserID != "joey" || identities[0].CreatedBy != "auto" || !strings.Contains(identities[0].MetadataJSON, "workspace_connection_union_id") {
		t.Fatalf("unexpected identities: %#v", identities)
	}
}

func TestHandleIMEventRecordsUnknownIdentityDiagnostic(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	var replyBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "tenant-token"})
		case "/open-apis/contact/v3/users/ou_unknown":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"user": map[string]any{
					"open_id": "ou_unknown",
					"name":    "Unknown",
				}},
			})
		case "/open-apis/im/v1/messages/om_unknown/reply":
			if err := json.NewDecoder(r.Body).Decode(&replyBody); err != nil {
				t.Fatalf("decode reply body: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0})
		default:
			t.Fatalf("unexpected path: %s", r.URL.String())
		}
	}))
	defer server.Close()
	if err := s.controlDB.UpsertConnection(controldb.Connection{
		ID:             "conn-feishu",
		WorkspaceID:    workspaceID,
		Provider:       "feishu",
		ConnectionName: "agent-sample-pm",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       "app_secret",
		Status:         "active",
		ProfileJSON:    "{}",
	}); err != nil {
		t.Fatalf("connection: %v", err)
	}
	secret, err := sealConnectionSecret(map[string]string{"baseUrl": server.URL, "appId": "cli_app", "appSecret": "secret"})
	if err != nil {
		t.Fatalf("seal secret: %v", err)
	}
	secret.ConnectionID = "conn-feishu"
	if err := s.controlDB.UpsertConnectionSecret(secret); err != nil {
		t.Fatalf("secret: %v", err)
	}
	if err := s.controlDB.UpsertAgentChannelBinding(controldb.AgentChannelBinding{
		ID:           "chan-feishu",
		WorkspaceID:  workspaceID,
		ProjectID:    "sample",
		AgentID:      "pm",
		Provider:     "feishu",
		ConnectionID: "conn-feishu",
		Status:       "connected",
		MetadataJSON: `{"appId":"cli_app"}`,
	}); err != nil {
		t.Fatalf("binding: %v", err)
	}

	body := `{
		"schema":"2.0",
		"header":{"event_type":"im.message.receive_v1","app_id":"cli_app"},
		"event":{
			"sender":{"sender_id":{"open_id":"ou_unknown"}},
			"message":{"message_id":"om_unknown","chat_id":"oc_one","chat_type":"p2p","message_type":"text","content":"{\"text\":\"hello\"}"}
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/im/feishu/events", strings.NewReader(body))
	req.SetPathValue("provider", "feishu")
	rec := httptest.NewRecorder()
	s.handleIMEvent(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Ignored bool   `json:"ignored"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response JSON: %v body=%s", err, rec.Body.String())
	}
	if !got.Ignored || got.Reason != "unknown_identity" {
		t.Fatalf("unexpected response: %#v", got)
	}
	updated, ok, err := s.controlDB.AgentChannelBindingByID("chan-feishu")
	if err != nil || !ok {
		t.Fatalf("load binding ok=%v err=%v", ok, err)
	}
	resp := agentChannelToResponse(updated)
	if resp.Callback.Status != "rejected" || resp.Callback.Reason != "unknown_identity" || resp.Callback.MessageID != "om_unknown" {
		t.Fatalf("callback metadata not recorded: %#v", resp.Callback)
	}
	content, _ := replyBody["content"].(string)
	if replyBody["msg_type"] != "text" || !strings.Contains(content, "/bind MG-") {
		t.Fatalf("single-chat unknown identity should receive bind hint, body=%#v content=%s", replyBody, content)
	}
}

func TestHandleIMEventDoesNotPromptBindInGroupChat(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "tenant-token"})
		case "/open-apis/contact/v3/users/ou_unknown":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"user": map[string]any{
					"open_id": "ou_unknown",
					"name":    "Unknown",
				}},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.String())
		}
	}))
	defer server.Close()
	if err := s.controlDB.UpsertConnection(controldb.Connection{
		ID:             "conn-feishu-group",
		WorkspaceID:    workspaceID,
		Provider:       "feishu",
		ConnectionName: "agent-sample-pm",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       "app_secret",
		Status:         "active",
		ProfileJSON:    "{}",
	}); err != nil {
		t.Fatalf("connection: %v", err)
	}
	secret, err := sealConnectionSecret(map[string]string{"baseUrl": server.URL, "appId": "cli_group", "appSecret": "secret"})
	if err != nil {
		t.Fatalf("seal secret: %v", err)
	}
	secret.ConnectionID = "conn-feishu-group"
	if err := s.controlDB.UpsertConnectionSecret(secret); err != nil {
		t.Fatalf("secret: %v", err)
	}
	if err := s.controlDB.UpsertAgentChannelBinding(controldb.AgentChannelBinding{
		ID:           "chan-feishu-group",
		WorkspaceID:  workspaceID,
		ProjectID:    "sample",
		AgentID:      "pm",
		Provider:     "feishu",
		ConnectionID: "conn-feishu-group",
		Status:       "connected",
		MetadataJSON: `{"appId":"cli_group"}`,
	}); err != nil {
		t.Fatalf("binding: %v", err)
	}

	body := `{
		"schema":"2.0",
		"header":{"event_type":"im.message.receive_v1","app_id":"cli_group"},
		"event":{
			"sender":{"sender_id":{"open_id":"ou_unknown"}},
			"message":{"message_id":"om_group","chat_id":"oc_group","chat_type":"group","message_type":"text","content":"{\"text\":\"@pm hello\"}"}
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/im/feishu/events", strings.NewReader(body))
	req.SetPathValue("provider", "feishu")
	rec := httptest.NewRecorder()
	s.handleIMEvent(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Ignored bool   `json:"ignored"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response JSON: %v body=%s", err, rec.Body.String())
	}
	if !got.Ignored || got.Reason != "unknown_identity" {
		t.Fatalf("unexpected response: %#v", got)
	}
}

func TestAcceptIMInteractionCallbackSubmitsRequestAndRecordsAttention(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedSampleAgentsForTest(t, s, workspaceID)
	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.controlDB.UpsertConnection(controldb.Connection{
		ID:             "conn-feishu-card",
		WorkspaceID:    workspaceID,
		Provider:       "feishu",
		ConnectionName: "agent-sample-pm",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       "app_secret",
		Status:         "active",
		ProfileJSON:    "{}",
	}); err != nil {
		t.Fatalf("connection: %v", err)
	}
	secret, err := sealConnectionSecret(map[string]string{"baseUrl": "https://open.feishu.cn", "appId": "cli_card", "appSecret": "secret"})
	if err != nil {
		t.Fatalf("seal secret: %v", err)
	}
	secret.ConnectionID = "conn-feishu-card"
	if err := s.controlDB.UpsertConnectionSecret(secret); err != nil {
		t.Fatalf("secret: %v", err)
	}
	if err := s.controlDB.UpsertAgentChannelBinding(controldb.AgentChannelBinding{
		ID:             "chan-feishu-card",
		WorkspaceID:    workspaceID,
		AgentWorkerID:  "aw-pm",
		ProjectID:      "sample",
		AgentID:        "pm",
		Provider:       "feishu",
		ConnectionID:   "conn-feishu-card",
		ExternalChatID: "oc_review",
		Status:         "connected",
		MetadataJSON:   `{"appId":"cli_card"}`,
	}); err != nil {
		t.Fatalf("binding: %v", err)
	}
	if err := s.controlDB.UpsertUserChannelIdentity(controldb.UserChannelIdentity{
		ID:               "uch-owner-card",
		WorkspaceID:      workspaceID,
		UserID:           "owner",
		ChannelBindingID: "chan-feishu-card",
		Provider:         "feishu",
		ExternalUserID:   "ou_owner_card",
		ExternalChatID:   "oc_review",
		CreatedBy:        "owner",
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("identity: %v", err)
	}
	if err := s.controlDB.CreateInteractionRequest(controldb.InteractionRequest{
		ID:               "ir-card-callback",
		WorkspaceID:      workspaceID,
		AgentWorkerID:    "aw-pm",
		ProjectID:        "sample",
		AgentID:          "pm",
		ChannelBindingID: "chan-feishu-card",
		Provider:         "feishu",
		Recipient:        "user:owner",
		TargetType:       "user",
		TargetUserID:     "owner",
		Title:            "Review",
		ContextJSON:      `{"taskId":"task-review"}`,
		HandlerType:      "agent_event",
		Status:           "active",
		CreatedBy:        "sample/pm",
		CreatedAt:        now,
		ExpiresAt:        time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("interaction request: %v", err)
	}
	provider := &testIMProvider{id: "feishu", label: "Feishu"}
	result, err := s.acceptIMInteractionCallback(provider, "cli_card", "", imbridge.IncomingInteractionCallback{
		InteractionID: "ir-card-callback",
		MessageID:     "om_card_callback",
		ChatID:        "oc_review",
		SenderOpenID:  "ou_owner_card",
		ActionID:      "approve",
		ActionLabel:   "批准",
		Inputs:        map[string]string{"comments": "ok"},
	}, "http://127.0.0.1:27893")
	if err != nil {
		t.Fatalf("accept callback: %v", err)
	}
	if result["status"] != "queued" {
		t.Fatalf("unexpected result: %#v", result)
	}
	for i := 0; i < 20 && len(provider.cardUpdates) == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if len(provider.cardUpdates) != 1 || provider.cardUpdates[0].Card == nil || !strings.Contains(provider.cardUpdates[0].Card.Title, "已提交") {
		t.Fatalf("expected accepted card update, got %#v", provider.cardUpdates)
	}
	request, found, err := s.controlDB.InteractionRequestByID(workspaceID, "ir-card-callback")
	if err != nil || !found {
		t.Fatalf("interaction lookup found=%v err=%v", found, err)
	}
	if request.Status != "submitted" || request.SubmittedBy != "owner" || !strings.Contains(request.SubmissionJSON, `"actionId":"approve"`) {
		t.Fatalf("unexpected submitted request: %#v", request)
	}
	signals, err := s.controlDB.ListAttentionSignals(controldb.AttentionSignalFilter{
		WorkspaceID:   workspaceID,
		AgentWorkerID: "aw-pm",
		SourceKind:    "im_card_action",
		Status:        "pending",
	})
	if err != nil {
		t.Fatalf("list signals: %v", err)
	}
	if len(signals) != 1 || signals[0].SourceID != "ir-card-callback" || signals[0].ActorID != "owner" {
		t.Fatalf("unexpected card attention signals: %#v", signals)
	}
	_, _, vars, err := s.pendingAttentionWakeupSectionAndVars(workspaceID, "sample", "pm", signals[0].ID)
	if err != nil {
		t.Fatalf("pending attention section: %v", err)
	}
	if strings.TrimSpace(vars["MULTIGENT_DELEGATION_TOKEN"]) == "" || vars["MULTIGENT_DELEGATION_INTERACTION_ID"] != "ir-card-callback" {
		t.Fatalf("expected delegation vars for submitted card action: %#v", vars)
	}
}

func TestChannelEventUserPermissionUsesAgentRBAC(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	grantProjectRoleForTest(t, s, workspaceID, "viewer", ProjectRoleViewer)
	grantProjectRoleForTest(t, s, workspaceID, "operator", ProjectRoleOperator)

	if s.userCanOperateAgent("viewer", "sample", "pm") {
		t.Fatalf("viewer should not operate agent")
	}
	if !s.userCanOperateAgent("operator", "sample", "pm") {
		t.Fatalf("operator should operate agent")
	}
	if !s.userCanOperateAgent("owner", "sample", "pm") {
		t.Fatalf("linked owner should operate own agent")
	}
	if s.userCanOperateAgent("owner", "sample", "backend") {
		t.Fatalf("linked owner should not operate unlinked agent")
	}
}

func TestIMEventTokenVerification(t *testing.T) {
	if !verifyIMEventToken("token-one", map[string]string{}) {
		t.Fatalf("empty configured token should allow event")
	}
	if !verifyIMEventToken("token-one", map[string]string{"verificationToken": "token-one"}) {
		t.Fatalf("matching token should allow event")
	}
	if verifyIMEventToken("token-one", map[string]string{"verificationToken": "token-two"}) {
		t.Fatalf("mismatched token should reject event")
	}
	if verifyIMEventToken("", map[string]string{"verificationToken": "token-one"}) {
		t.Fatalf("missing event token should reject when configured")
	}
}

func TestDecryptIMEventUsesConfiguredProviderKey(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	if err := s.controlDB.UpsertConnection(controldb.Connection{
		ID:             "conn-feishu",
		WorkspaceID:    workspaceID,
		Provider:       "feishu",
		ConnectionName: "agent-sample-pm",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       "app_secret",
		Status:         "active",
		ProfileJSON:    "{}",
	}); err != nil {
		t.Fatalf("connection: %v", err)
	}
	secret, err := sealConnectionSecret(map[string]string{"baseUrl": "https://open.feishu.cn", "appId": "cli_app", "appSecret": "secret", "encryptKey": "encrypt-one"})
	if err != nil {
		t.Fatalf("seal secret: %v", err)
	}
	secret.ConnectionID = "conn-feishu"
	if err := s.controlDB.UpsertConnectionSecret(secret); err != nil {
		t.Fatalf("secret: %v", err)
	}
	if err := s.controlDB.UpsertAgentChannelBinding(controldb.AgentChannelBinding{
		ID:           "chan-feishu",
		WorkspaceID:  workspaceID,
		ProjectID:    "sample",
		AgentID:      "pm",
		Provider:     "feishu",
		ConnectionID: "conn-feishu",
		Status:       "connected",
		MetadataJSON: `{"appId":"cli_app"}`,
	}); err != nil {
		t.Fatalf("binding: %v", err)
	}

	plaintext := []byte(`{"schema":"2.0","header":{"event_type":"url_verification"},"challenge":"ok"}`)
	encrypted := encryptLarkEventForAPITest(t, plaintext, "encrypt-one")
	feishu, _ := imbridge.LookupProvider("feishu")
	decrypted, ok, err := s.decryptIMEvent(feishu, encrypted)
	if err != nil || !ok {
		t.Fatalf("decrypt ok=%v err=%v", ok, err)
	}
	if string(decrypted) != string(plaintext) {
		t.Fatalf("decrypted=%s", decrypted)
	}
	lark, _ := imbridge.LookupProvider("lark")
	if _, ok, err := s.decryptIMEvent(lark, encrypted); err != nil || ok {
		t.Fatalf("wrong provider should not decrypt ok=%v err=%v", ok, err)
	}
}

func TestShouldHandleIMMessageRequiresGroupAddressing(t *testing.T) {
	provider, _ := imbridge.LookupProvider("feishu")
	if !provider.ShouldHandleMessage("", imbridge.IncomingMessage{ChatType: "p2p", ChatID: "oc_direct"}) {
		t.Fatalf("direct chat should be handled")
	}
	if provider.ShouldHandleMessage("", imbridge.IncomingMessage{ChatType: "group", ChatID: "oc_group", RawContent: `{"text":"hello"}`}) {
		t.Fatalf("unbound group without mention should be ignored")
	}
	if !provider.ShouldHandleMessage("", imbridge.IncomingMessage{ChatType: "group", ChatID: "oc_group", RawContent: `{"text":"@bot hello","mentions":[{"key":"@bot"}]}`}) {
		t.Fatalf("unbound group mention should be handled")
	}
	if !provider.ShouldHandleMessage("", imbridge.IncomingMessage{ChatType: "group", ChatID: "oc_group", ParentID: "om_parent", RawContent: `{"text":"continue"}`}) {
		t.Fatalf("unbound group reply should be handled")
	}
	if !provider.ShouldHandleMessage("oc_group", imbridge.IncomingMessage{ChatType: "group", ChatID: "oc_group", RawContent: `{"text":"hello"}`}) {
		t.Fatalf("bound group should be handled")
	}
	if provider.ShouldHandleMessage("oc_group", imbridge.IncomingMessage{ChatType: "group", ChatID: "oc_other", RawContent: `{"text":"hello"}`}) {
		t.Fatalf("different unmentioned group should be ignored")
	}
}

func TestChannelEventBindingRequiresMatchingAppIDWhenConfigured(t *testing.T) {
	binding := controldb.AgentChannelBinding{
		Provider:     "feishu",
		MetadataJSON: `{"appId":"cli_app"}`,
	}
	if channelEventBindingMatches(binding, "", "") {
		t.Fatalf("missing event app id should not match a configured channel app id")
	}
	if channelEventBindingMatches(binding, "other_app", "") {
		t.Fatalf("different event app id should not match")
	}
	if !channelEventBindingMatches(binding, "cli_app", "") {
		t.Fatalf("matching event app id should match")
	}
}

func TestAgentChannelSecurityPreservesConnectionSecret(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedSampleAgentsForTest(t, s, workspaceID)
	if err := s.controlDB.UpsertConnection(controldb.Connection{
		ID:             "conn-feishu",
		WorkspaceID:    workspaceID,
		Provider:       "feishu",
		ConnectionName: "agent-sample-pm",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       "app_secret",
		Status:         "active",
		ProfileJSON:    "{}",
	}); err != nil {
		t.Fatalf("connection: %v", err)
	}
	secret, err := sealConnectionSecret(map[string]string{"baseUrl": "https://open.feishu.cn", "appId": "cli_app", "appSecret": "secret"})
	if err != nil {
		t.Fatalf("seal secret: %v", err)
	}
	secret.ConnectionID = "conn-feishu"
	if err := s.controlDB.UpsertConnectionSecret(secret); err != nil {
		t.Fatalf("secret: %v", err)
	}
	if err := s.controlDB.UpsertAgentChannelBinding(controldb.AgentChannelBinding{
		ID:           "chan-feishu",
		WorkspaceID:  workspaceID,
		ProjectID:    "sample",
		AgentID:      "pm",
		Provider:     "feishu",
		ConnectionID: "conn-feishu",
		Status:       "connected",
		MetadataJSON: `{"appId":"cli_app"}`,
	}); err != nil {
		t.Fatalf("binding: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/sample/agents/pm/channels/feishu/security", strings.NewReader(`{"verificationToken":"verify-one","encryptKey":"encrypt-one"}`))
	req = req.WithContext(context.WithValue(req.Context(), ctxUserKey, "owner"))
	req.SetPathValue("name", "sample")
	req.SetPathValue("agent", "pm")
	req.SetPathValue("provider", "feishu")
	rec := httptest.NewRecorder()
	s.handleAgentChannelSecurity(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	updated, ok, err := s.controlDB.ConnectionSecret("conn-feishu")
	if err != nil || !ok {
		t.Fatalf("updated secret ok=%v err=%v", ok, err)
	}
	values, err := openConnectionSecret(updated)
	if err != nil {
		t.Fatalf("open secret: %v", err)
	}
	if values["appId"] != "cli_app" || values["appSecret"] != "secret" || values["verificationToken"] != "verify-one" || values["encryptKey"] != "encrypt-one" {
		t.Fatalf("secret values not preserved/updated: %#v", values)
	}
}

func TestAgentChannelBindingUsesAgentWorkerIdentity(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.controlDB.UpsertAgentWorker(controldb.AgentWorker{
		ID:          "aw-pm",
		WorkspaceID: workspaceID,
		Name:        "pm",
		DisplayName: "PM",
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("worker: %v", err)
	}
	if err := s.controlDB.UpsertProjectMembership(controldb.ProjectMembership{
		ID:               "pm-sample",
		WorkspaceID:      workspaceID,
		ProjectID:        "sample",
		MemberType:       "agent_worker",
		MemberID:         "aw-pm",
		Title:            "pm",
		AttentionEnabled: true,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("membership: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxUserKey, "owner"))
	binding, err := s.saveManualAgentIMChannel(req, workspaceID, "sample", "pm", imbridge.ManualSetupResult{
		Provider:        "feishu",
		AppID:           "cli_app",
		ExternalOwnerID: "ou_owner",
		SecretValues:    map[string]string{"appSecret": "secret"},
	})
	if err != nil {
		t.Fatalf("save channel: %v", err)
	}
	if binding.AgentWorkerID != "aw-pm" {
		t.Fatalf("binding worker=%q", binding.AgentWorkerID)
	}
	bindings, err := s.controlDB.ListAgentChannelBindings(controldb.AgentChannelBindingFilter{
		WorkspaceID:   workspaceID,
		AgentWorkerID: "aw-pm",
		Provider:      "feishu",
	})
	if err != nil {
		t.Fatalf("list by worker: %v", err)
	}
	if len(bindings) != 1 || bindings[0].ID != binding.ID {
		t.Fatalf("unexpected worker bindings: %+v", bindings)
	}
}

func TestRecordIMAttentionSignalPersistsSignalAndCursor(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
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
	binding := controldb.AgentChannelBinding{
		ID:            "chan-feishu",
		WorkspaceID:   workspaceID,
		AgentWorkerID: "aw-pm",
		ProjectID:     "sample",
		AgentID:       "pm",
		Provider:      "feishu",
		ConnectionID:  "conn-feishu",
		Status:        "connected",
		MetadataJSON:  `{"appId":"cli_app"}`,
	}
	signalID := s.recordIMAttentionSignal(resolvedChannelEventBinding{
		Binding: binding,
		Identity: controldb.ExternalIdentity{
			WorkspaceID:    workspaceID,
			Provider:       "feishu",
			ExternalUserID: "ou_owner",
			UserID:         "owner",
		},
	}, "feishu", imbridge.IncomingMessage{
		MessageID:    "om_one",
		ChatID:       "oc_one",
		ChatType:     "p2p",
		SenderOpenID: "ou_owner",
		RawContent:   `{"text":"hello"}`,
	}, "hello")
	if signalID == "" {
		t.Fatalf("signal id is empty")
	}
	duplicateSignalID := s.recordIMAttentionSignal(resolvedChannelEventBinding{
		Binding: binding,
		Identity: controldb.ExternalIdentity{
			WorkspaceID:    workspaceID,
			Provider:       "feishu",
			ExternalUserID: "ou_owner",
			UserID:         "owner",
		},
	}, "feishu", imbridge.IncomingMessage{
		MessageID:    "om_one",
		ChatID:       "oc_one",
		ChatType:     "p2p",
		SenderOpenID: "ou_owner",
		RawContent:   `{"text":"hello"}`,
	}, "hello")
	if duplicateSignalID == "" {
		t.Fatalf("duplicate signal id is empty")
	}
	signals, err := s.controlDB.ListAttentionSignals(controldb.AttentionSignalFilter{
		WorkspaceID:   workspaceID,
		AgentWorkerID: "aw-pm",
		Status:        "pending",
	})
	if err != nil {
		t.Fatalf("list signals: %v", err)
	}
	if len(signals) != 1 || signals[0].DedupeKey != "im:feishu:om_one" || signals[0].Reason != "im_direct_message" {
		t.Fatalf("unexpected signals: %+v", signals)
	}
	trust := attentionSignalTrust(signals[0])
	if trust["trustLevel"] != "authenticated_user" || trust["actorAuthenticated"] != true || trust["actorAuthorized"] != true {
		t.Fatalf("unexpected IM trust context: %#v", trust)
	}
	if trust["identitySubject"] != "owner" || trust["authorizationScope"] != "sample/pm" {
		t.Fatalf("unexpected IM trust identity/scope: %#v", trust)
	}
	cursor, ok, err := s.controlDB.AttentionCursorBySource(workspaceID, "aw-pm", "im_message", "im:feishu:p2p:oc_one:user:owner")
	if err != nil || !ok {
		t.Fatalf("cursor ok=%v err=%v", ok, err)
	}
	if cursor.Cursor != "om_one" || cursor.SeenUntil != "om_one" {
		t.Fatalf("unexpected cursor: %+v", cursor)
	}
}

func TestRecordIMAttentionSignalPersistsAttachmentOnlyMessage(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
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
	binding := controldb.AgentChannelBinding{
		ID:            "chan-feishu",
		WorkspaceID:   workspaceID,
		AgentWorkerID: "aw-pm",
		ProjectID:     "sample",
		AgentID:       "pm",
		Provider:      "feishu",
		ConnectionID:  "conn-feishu",
		Status:        "connected",
	}
	message := imbridge.IncomingMessage{
		MessageID:    "om_image",
		ChatID:       "oc_one",
		ChatType:     "p2p",
		SenderOpenID: "ou_owner",
		MessageType:  "image",
		RawContent:   `{"image_key":"img_v3_abc"}`,
		Attachments:  []imbridge.IncomingAttachment{{Type: "image", ID: "img_v3_abc"}},
	}
	text := incomingMessageFallbackText(message)
	if !strings.Contains(text, "image") || !strings.Contains(text, "img_v3_abc") {
		t.Fatalf("unexpected fallback text: %q", text)
	}
	signalID := s.recordIMAttentionSignal(resolvedChannelEventBinding{
		Binding: binding,
		Identity: controldb.ExternalIdentity{
			WorkspaceID:    workspaceID,
			Provider:       "feishu",
			ExternalUserID: "ou_owner",
			UserID:         "owner",
		},
	}, "feishu", message, text)
	if signalID == "" {
		t.Fatalf("signal id is empty")
	}
	signal, ok, err := s.controlDB.AttentionSignalByID(workspaceID, signalID)
	if err != nil || !ok {
		t.Fatalf("signal ok=%v err=%v", ok, err)
	}
	if !strings.Contains(signal.Summary, "img_v3_abc") {
		t.Fatalf("summary should include attachment fallback: %q", signal.Summary)
	}
	if !strings.Contains(signal.PayloadJSON, "attachments") || !strings.Contains(signal.PayloadJSON, "img_v3_abc") {
		t.Fatalf("payload should include attachment metadata: %s", signal.PayloadJSON)
	}
}

func TestShouldWakeAgentForAttentionUsesWorkerMessageTrigger(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	worker := controldb.AgentWorker{
		ID:          "aw-pm",
		WorkspaceID: workspaceID,
		Name:        "pm-worker",
		DisplayName: "PM",
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
		t.Fatalf("worker: %v", err)
	}
	if err := s.controlDB.UpsertProjectMembership(controldb.ProjectMembership{
		ID:          "pm-member",
		WorkspaceID: workspaceID,
		ProjectID:   "sample",
		MemberType:  "agent_worker",
		MemberID:    worker.ID,
		Role:        "pm",
		Title:       "pm",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("membership: %v", err)
	}
	binding := controldb.AgentChannelBinding{WorkspaceID: workspaceID, ProjectID: "sample", AgentID: "pm", AgentWorkerID: worker.ID}
	if s.shouldWakeAgentForAttention(binding, "im_direct_message") {
		t.Fatalf("agent should not wake without message trigger")
	}
	worker.ScheduleJSON = `{"triggers":["message"]}`
	if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
		t.Fatalf("save worker schedule: %v", err)
	}
	if !s.shouldWakeAgentForAttention(binding, "im_direct_message") {
		t.Fatalf("agent should wake when worker schedule enables message trigger")
	}
	worker.ScheduleJSON = `{"triggers":["attention"]}`
	if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
		t.Fatalf("save worker attention schedule: %v", err)
	}
	if !s.shouldWakeAgentForAttention(binding, "im_direct_message") {
		t.Fatalf("agent should wake when worker schedule enables attention trigger")
	}
	worker.ScheduleJSON = `{"triggers":["im_direct_message"]}`
	if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
		t.Fatalf("save worker reason schedule: %v", err)
	}
	if !s.shouldWakeAgentForAttention(binding, "im_direct_message") {
		t.Fatalf("agent should wake when worker schedule enables matching attention reason")
	}
	if s.shouldWakeAgentForAttention(binding, "im_mention") {
		t.Fatalf("agent should not wake for unmatched attention reason")
	}
	worker.ScheduleJSON = `{}`
	if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
		t.Fatalf("clear worker schedule: %v", err)
	}
	if s.shouldWakeAgentForAttention(binding, "im_mention") {
		t.Fatalf("agent should not wake when worker schedule has no matching triggers")
	}
	worker.AttentionPolicyJSON = `{"im_mention":true}`
	if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
		t.Fatalf("save worker attention policy: %v", err)
	}
	if !s.shouldWakeAgentForAttention(binding, "im_mention") {
		t.Fatalf("agent should wake when worker attention policy enables matching reason")
	}
	if s.shouldWakeAgentForAttention(binding, "im_direct_message") {
		t.Fatalf("agent should not wake for unmatched attention policy reason")
	}
	worker.AttentionPolicyJSON = `{"attention":true}`
	if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
		t.Fatalf("save broad worker attention policy: %v", err)
	}
	if !s.shouldWakeAgentForAttention(binding, "im_direct_message") {
		t.Fatalf("agent should wake when worker attention policy enables broad attention")
	}
}

func TestAgentChannelResponseIncludesPublicMetadata(t *testing.T) {
	resp := agentChannelToResponse(controldb.AgentChannelBinding{
		ID:             "chan-one",
		Provider:       "feishu",
		Status:         "connected",
		ConnectionID:   "conn-one",
		ExternalChatID: "oc_one",
		MetadataJSON:   `{"appId":"cli_app","accountsUrl":"https://accounts.feishu.cn","lastCallback":{"at":"2026-07-16T10:00:00Z","status":"replied","reason":"","messageId":"om_one","error":""}}`,
	})
	if resp.AppID != "cli_app" || resp.AccountsURL != "https://accounts.feishu.cn" || resp.ExternalChatID != "oc_one" {
		t.Fatalf("response metadata missing: %#v", resp)
	}
	if resp.Callback.LastAt != "2026-07-16T10:00:00Z" || resp.Callback.Status != "replied" || resp.Callback.MessageID != "om_one" {
		t.Fatalf("callback metadata missing: %#v", resp.Callback)
	}
}

func TestRecordAgentChannelCallbackPreservesMetadata(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	if err := s.controlDB.UpsertConnection(controldb.Connection{
		ID:             "conn-feishu",
		WorkspaceID:    workspaceID,
		Provider:       "feishu",
		ConnectionName: "agent-sample-pm",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       "app_secret",
		Status:         "active",
		ProfileJSON:    "{}",
	}); err != nil {
		t.Fatalf("connection: %v", err)
	}
	binding := controldb.AgentChannelBinding{
		ID:           "chan-feishu",
		WorkspaceID:  workspaceID,
		ProjectID:    "sample",
		AgentID:      "pm",
		Provider:     "feishu",
		ConnectionID: "conn-feishu",
		Status:       "connected",
		MetadataJSON: `{"appId":"cli_app","accountsUrl":"https://accounts.feishu.cn"}`,
	}
	if err := s.controlDB.UpsertAgentChannelBinding(binding); err != nil {
		t.Fatalf("binding: %v", err)
	}
	s.recordAgentChannelCallback(binding, "ignored", "group_not_addressed", imbridge.IncomingMessage{
		MessageID:  "om_one",
		ChatID:     "oc_one",
		ChatType:   "group",
		RawContent: `{"text":"hello"}`,
	}, "")
	updated, ok, err := s.controlDB.AgentChannelBindingByID("chan-feishu")
	if err != nil || !ok {
		t.Fatalf("load updated binding ok=%v err=%v", ok, err)
	}
	resp := agentChannelToResponse(updated)
	if resp.AppID != "cli_app" || resp.AccountsURL != "https://accounts.feishu.cn" {
		t.Fatalf("existing metadata not preserved: %#v", resp)
	}
	if resp.Callback.Status != "ignored" || resp.Callback.Reason != "group_not_addressed" || resp.Callback.MessageID != "om_one" {
		t.Fatalf("callback metadata not recorded: %#v", resp.Callback)
	}
}

func TestAPIInteractionLeasePersistsRuntimeSessionID(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	source := interaction.Source{
		Kind:    "web_chat",
		ActorID: "owner",
		Channel: "web",
	}
	lease, err := s.acquireAgentInteractionLease(s.interactionAgentRef(workspaceID, "sample", "pm"), source, "interactive")
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}
	lease.SetRuntimeSessionID("runtime-one")
	stored, ok, err := s.controlDB.InteractionSessionByID(lease.SessionID())
	if err != nil || !ok {
		t.Fatalf("lookup session ok=%v err=%v", ok, err)
	}
	if stored.RuntimeSessionID != "runtime-one" {
		t.Fatalf("runtime session id=%q", stored.RuntimeSessionID)
	}
	lease.Release()

	next, err := s.acquireAgentInteractionLease(s.interactionAgentRef(workspaceID, "sample", "pm"), source, "interactive")
	if err != nil {
		t.Fatalf("reacquire lease: %v", err)
	}
	if next.session.RuntimeSessionID != "runtime-one" {
		t.Fatalf("reused runtime session id=%q", next.session.RuntimeSessionID)
	}
	next.Release()
}

func TestAPIInteractionLeaseReusesWorkerRuntimeSessionAcrossSources(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
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
	if err := s.controlDB.UpsertProjectMembership(controldb.ProjectMembership{
		ID:               "pm-sample",
		WorkspaceID:      workspaceID,
		ProjectID:        "sample",
		MemberType:       "agent_worker",
		MemberID:         "aw-pm",
		Title:            "pm",
		AttentionEnabled: true,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("membership: %v", err)
	}
	first, err := s.acquireAgentInteractionLease(s.interactionAgentRef(workspaceID, "sample", "pm"), imConversationSource("lark", "alice", "oc_one", "p2p"), "interactive")
	if err != nil {
		t.Fatalf("first source: %v", err)
	}
	first.SetRuntimeSessionID("runtime-primary")
	first.Release()

	second, err := s.acquireAgentInteractionLease(s.interactionAgentRef(workspaceID, "sample", "pm"), imConversationSource("lark", "bob", "oc_two", "p2p"), "interactive")
	if err != nil {
		t.Fatalf("second source: %v", err)
	}
	defer second.Release()
	if second.session.RuntimeSessionID != "runtime-primary" {
		t.Fatalf("expected worker primary runtime session to be reused across sources, got %q", second.session.RuntimeSessionID)
	}
}

func TestAPIInteractionLeaseAllowsDifferentConversationSources(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	agent := s.interactionAgentRef(workspaceID, "sample", "pm")
	one, err := s.acquireAgentInteractionLease(agent, imConversationSource("lark", "alice", "oc_one", "p2p"), "interactive")
	if err != nil {
		t.Fatalf("first source: %v", err)
	}
	defer one.Release()
	two, err := s.acquireAgentInteractionLease(agent, imConversationSource("lark", "bob", "oc_one", "p2p"), "interactive")
	if err != nil {
		t.Fatalf("second source should not conflict: %v", err)
	}
	defer two.Release()
	if one.SessionID() == two.SessionID() {
		t.Fatalf("different sources should get different interaction sessions")
	}
}

func TestFormatIMAgentPromptRequiresHumanFacingReply(t *testing.T) {
	prompt := formatIMAgentPromptWithSender("lark", controldb.AgentChannelBinding{
		ProjectID: "tapnow-agent-platform",
		AgentID:   "nova",
	}, controldb.ExternalIdentity{UserID: "admin"}, "Glenn Chen (admin) <glenn@example.com>", imbridge.IncomingMessage{ChatID: "oc_one"}, "帮我看一下当前任务")
	for _, want := range []string{
		"Always finish with a concise, human-facing final reply",
		"Reply in the same language as the user's message",
		"tapnow-agent-platform/nova",
		"Glenn Chen (admin) <glenn@example.com>",
		"帮我看一下当前任务",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestParseIMProgressEntriesCodexCommandAndMessage(t *testing.T) {
	start := parseIMProgressEntries(`{"type":"item.started","item":{"type":"command_execution","command":"pwd","status":"in_progress"}}`)
	if len(start) != 1 || start[0].Kind != "tool_use" || start[0].Title != "Shell" || !strings.Contains(start[0].Content, "pwd") {
		t.Fatalf("unexpected command start entries: %#v", start)
	}
	done := parseIMProgressEntries(`{"type":"item.completed","item":{"type":"command_execution","command":"pwd","aggregated_output":"/workspace\n","exit_code":0,"status":"completed"}}`)
	if len(done) != 1 || done[0].Kind != "tool_result" || !strings.Contains(done[0].Content, "/workspace") {
		t.Fatalf("unexpected command done entries: %#v", done)
	}
	msg := parseIMProgressEntries(`{"type":"item.completed","item":{"type":"agent_message","text":"结论：可以继续。"}}`)
	if len(msg) != 1 || msg[0].Kind != "thinking" || !strings.Contains(msg[0].Content, "可以继续") {
		t.Fatalf("unexpected agent message entries: %#v", msg)
	}
}

func TestParseIMProgressEntriesClaudeContentBlocks(t *testing.T) {
	tool := parseIMProgressEntries(`{"type":"content","content_block":{"type":"tool_use","name":"Bash","input":{"command":"pwd"}}}`)
	if len(tool) != 1 || tool[0].Kind != "tool_use" || tool[0].Title != "Bash" {
		t.Fatalf("unexpected tool entry: %#v", tool)
	}
	text := parseIMProgressEntries(`{"type":"content","content_block":{"type":"text","text":"已经检查完成。"}}`)
	if len(text) != 1 || text[0].Kind != "thinking" || !strings.Contains(text[0].Content, "检查完成") {
		t.Fatalf("unexpected text entry: %#v", text)
	}
}

func TestParseIMProgressEntriesClaudeThinkingAndToolResult(t *testing.T) {
	thinking := parseIMProgressEntries(`{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"I will inspect the workspace."}]}}`)
	if len(thinking) != 1 || thinking[0].Kind != "thinking" || !strings.Contains(thinking[0].Content, "inspect") {
		t.Fatalf("unexpected thinking entry: %#v", thinking)
	}
	result := parseIMProgressEntries(`{"type":"user","message":{"content":[{"tool_use_id":"call_one","type":"tool_result","content":"/workspace\nCLAUDE.md","is_error":false}]}}`)
	if len(result) != 1 || result[0].Kind != "tool_result" || !strings.Contains(result[0].Content, "CLAUDE.md") {
		t.Fatalf("unexpected tool result entry: %#v", result)
	}
}

func encryptLarkEventForAPITest(t *testing.T, plaintext []byte, encryptKey string) string {
	t.Helper()
	key := sha256.Sum256([]byte(encryptKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		t.Fatal(err)
	}
	padding := aes.BlockSize - len(plaintext)%aes.BlockSize
	padded := append(append([]byte(nil), plaintext...), make([]byte, padding)...)
	for i := len(padded) - padding; i < len(padded); i++ {
		padded[i] = byte(padding)
	}
	iv := []byte("1234567890abcdef")
	body := append([]byte(nil), padded...)
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(body, body)
	return base64.StdEncoding.EncodeToString(append(append([]byte(nil), iv...), body...))
}
