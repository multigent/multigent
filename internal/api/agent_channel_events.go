package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/imbridge"
	"github.com/multigent/multigent/internal/interaction"
)

type resolvedChannelEventBinding struct {
	Binding      controldb.AgentChannelBinding
	SecretValues map[string]string
	Identity     controldb.ExternalIdentity
}

type channelEventResolution struct {
	Resolved     resolvedChannelEventBinding
	Found        bool
	Candidate    controldb.AgentChannelBinding
	HasCandidate bool
}

func (s *Server) handleIMEvent(w http.ResponseWriter, r *http.Request) {
	channelProvider, ok := imbridge.LookupProvider(r.PathValue("provider"))
	if !ok {
		s.jsonError(w, http.StatusBadRequest, "unsupported IM provider")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, "read event body failed")
		return
	}
	if encryptedPayload, encrypted := channelProvider.ExtractEncryptedPayload(raw); encrypted {
		decrypted, ok, err := s.decryptIMEvent(channelProvider, encryptedPayload)
		if err != nil {
			s.serverError(w, err)
			return
		}
		if !ok {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ignored": true, "reason": "decrypt_failed"})
			return
		}
		raw = decrypted
	}
	parsed, err := channelProvider.ParseEvent(raw)
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid event JSON")
		return
	}
	if parsed.IsURLVerification {
		_ = json.NewEncoder(w).Encode(map[string]string{"challenge": parsed.Challenge})
		return
	}
	if !parsed.IsMessage {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ignored": true})
		return
	}
	result, err := s.acceptIMMessage(channelProvider, parsed.AppID, parsed.VerificationToken, parsed.Message)
	if err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Server) acceptIMMessage(channelProvider imbridge.Provider, appID, verificationToken string, message imbridge.IncomingMessage) (map[string]any, error) {
	provider := channelProvider.Info().ID
	text := strings.TrimSpace(message.Text)
	if text == "" {
		return map[string]any{"ok": true, "ignored": true}, nil
	}
	if bindCmd, code, ok := parseAgentChannelBindCommand(text); ok {
		return s.acceptAgentChannelBindCommand(channelProvider, appID, verificationToken, message, bindCmd, code)
	}
	resolution, err := s.resolveChannelEventBindingDetailed(provider, appID, message.ChatID, message.SenderOpenID)
	if err != nil {
		return nil, err
	}
	if !resolution.Found {
		reason := "binding_not_found"
		if resolution.HasCandidate {
			reason = "unknown_identity"
			s.recordAgentChannelCallback(resolution.Candidate, "rejected", reason, message, "")
			s.auditLog(auditLogInput{
				WorkspaceID:  resolution.Candidate.WorkspaceID,
				Action:       "agent_channel.identity_missing",
				ResourceType: "agent_channel",
				ResourceID:   resolution.Candidate.ID,
				Summary:      fmt.Sprintf("Ignored %s message for %s/%s because the sender is not linked to a Multigent user", provider, resolution.Candidate.ProjectID, resolution.Candidate.AgentID),
				After: map[string]any{
					"provider":       provider,
					"externalUserId": message.SenderOpenID,
					"messageId":      message.MessageID,
					"chatId":         message.ChatID,
				},
			})
		} else {
			log.Printf("[im:%s] binding not found app=%s chat=%s sender=%s message=%s", provider, appID, message.ChatID, message.SenderOpenID, message.MessageID)
		}
		return map[string]any{"ok": true, "ignored": true, "reason": reason}, nil
	}
	resolved := resolution.Resolved
	if !channelProvider.ShouldHandleMessage(resolved.Binding.ExternalChatID, message) {
		s.recordAgentChannelCallback(resolved.Binding, "ignored", "group_not_addressed", message, "")
		s.auditLog(auditLogInput{
			WorkspaceID:  resolved.Binding.WorkspaceID,
			ActorType:    "user",
			ActorID:      resolved.Identity.UserID,
			Action:       "agent_channel.message_ignored",
			ResourceType: "agent_channel",
			ResourceID:   resolved.Binding.ID,
			Summary:      fmt.Sprintf("Ignored %s group message for %s/%s because the chat is not bound and the bot was not addressed", provider, resolved.Binding.ProjectID, resolved.Binding.AgentID),
			After: map[string]any{
				"provider":  provider,
				"messageId": message.MessageID,
				"chatId":    message.ChatID,
				"chatType":  message.ChatType,
			},
		})
		return map[string]any{"ok": true, "ignored": true, "reason": "group_not_addressed"}, nil
	}
	if !verifyIMEventToken(verificationToken, resolved.SecretValues) {
		s.recordAgentChannelCallback(resolved.Binding, "rejected", "verification_failed", message, "")
		s.auditLog(auditLogInput{
			WorkspaceID:  resolved.Binding.WorkspaceID,
			Action:       "agent_channel.verification_failed",
			ResourceType: "agent_channel",
			ResourceID:   resolved.Binding.ID,
			Summary:      fmt.Sprintf("Rejected %s event for %s/%s because verification token did not match", provider, resolved.Binding.ProjectID, resolved.Binding.AgentID),
			After: map[string]any{
				"provider":  provider,
				"messageId": message.MessageID,
				"chatId":    message.ChatID,
			},
		})
		return map[string]any{"ok": true, "ignored": true, "reason": "verification_failed"}, nil
	}
	if !s.userCanOperateAgentInWorkspace(resolved.Identity.UserID, resolved.Binding.WorkspaceID, resolved.Binding.ProjectID, resolved.Binding.AgentID) {
		s.recordAgentChannelCallback(resolved.Binding, "rejected", "permission_denied", message, "")
		s.auditLog(auditLogInput{
			WorkspaceID:  resolved.Binding.WorkspaceID,
			ActorType:    "user",
			ActorID:      resolved.Identity.UserID,
			Action:       "agent_channel.permission_denied",
			ResourceType: "agent_channel",
			ResourceID:   resolved.Binding.ID,
			Summary:      fmt.Sprintf("Denied %s message for %s/%s", provider, resolved.Binding.ProjectID, resolved.Binding.AgentID),
			After: map[string]any{
				"provider":       provider,
				"externalUserId": message.SenderOpenID,
				"messageId":      message.MessageID,
			},
		})
		return map[string]any{"ok": true, "ignored": true, "reason": "permission_denied"}, nil
	}
	meta, err := s.st.AgentMeta(resolved.Binding.ProjectID, resolved.Binding.AgentID)
	if err != nil {
		return nil, err
	}
	if readiness := buildRuntimeReadiness(meta); readiness.Blocking {
		detail := runtimeReadinessErrorMessage(readiness)
		reply := "Agent runtime is not ready. Please fix the runtime environment in Multigent and try again.\n\n" + detail
		s.recordAgentChannelCallback(resolved.Binding, "rejected", "runtime_not_ready", message, detail)
		s.auditLog(auditLogInput{
			WorkspaceID:  resolved.Binding.WorkspaceID,
			ActorType:    "user",
			ActorID:      resolved.Identity.UserID,
			Action:       "agent_channel.runtime_not_ready",
			ResourceType: "agent_channel",
			ResourceID:   resolved.Binding.ID,
			Summary:      fmt.Sprintf("Rejected %s message for %s/%s because runtime is not ready", provider, resolved.Binding.ProjectID, resolved.Binding.AgentID),
			After: map[string]any{
				"provider":  provider,
				"messageId": message.MessageID,
				"detail":    detail,
			},
		})
		if err := s.replyToIMEvent(context.Background(), channelProvider, resolved, message, trimForIM(reply, 3500)); err != nil {
			s.recordAgentChannelCallback(resolved.Binding, "reply_failed", "runtime_not_ready", message, err.Error())
		}
		return map[string]any{"ok": true, "ignored": true, "reason": "runtime_not_ready"}, nil
	}
	s.recordAgentChannelCallback(resolved.Binding, "accepted", "", message, "")
	go s.runAgentForIMEvent(channelProvider, resolved, message, text)
	return map[string]any{"ok": true}, nil
}

func (s *Server) acceptAgentChannelBindCommand(channelProvider imbridge.Provider, appID, verificationToken string, message imbridge.IncomingMessage, bindCmd, code string) (map[string]any, error) {
	provider := channelProvider.Info().ID
	codeRow, found, err := s.controlDB.AgentChannelBindCodeByCode(code)
	if err != nil {
		return nil, err
	}
	if !found {
		return s.replyBindCommandFailure(channelProvider, nil, nil, message, "绑定码无效。请在 Multigent 中重新生成绑定码后再试。", "invalid_code")
	}
	now := time.Now().UTC()
	if strings.TrimSpace(codeRow.UsedAt) != "" {
		return s.replyBindCommandFailure(channelProvider, nil, nil, message, "绑定码已使用。请在 Multigent 中重新生成绑定码。", "used_code")
	}
	expiresAt, err := time.Parse(time.RFC3339, strings.TrimSpace(codeRow.ExpiresAt))
	if err != nil || now.After(expiresAt) {
		return s.replyBindCommandFailure(channelProvider, nil, nil, message, "绑定码已过期。请在 Multigent 中重新生成绑定码。", "expired_code")
	}
	targetType := strings.TrimSpace(codeRow.TargetType)
	if targetType == "" {
		targetType = "user"
	}
	expectedCmd := "bind"
	if targetType == "chat" {
		expectedCmd = "bind-chat"
	}
	if bindCmd != expectedCmd {
		return s.replyBindCommandFailure(channelProvider, nil, nil, message, fmt.Sprintf("绑定码类型不匹配。请使用 /%s %s。", expectedCmd, codeRow.Code), "target_mismatch")
	}
	if targetType == "chat" && strings.TrimSpace(message.ChatID) == "" {
		return s.replyBindCommandFailure(channelProvider, nil, nil, message, "绑定群聊失败：当前消息没有可识别的群聊 ID。", "chat_missing")
	}
	matches, err := s.matchChannelEventBindings(provider, appID, message.ChatID)
	if err != nil {
		return nil, err
	}
	var binding *controldb.AgentChannelBinding
	for i := range matches {
		if matches[i].WorkspaceID == codeRow.WorkspaceID && matches[i].ID == codeRow.ChannelBindingID {
			binding = &matches[i]
			break
		}
	}
	if binding == nil {
		return s.replyBindCommandFailure(channelProvider, nil, nil, message, "绑定码不属于当前机器人或会话。请确认你正在给对应 Agent 的机器人发送绑定命令。", "channel_mismatch")
	}
	secret, ok, err := s.controlDB.ConnectionSecret(binding.ConnectionID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return s.replyBindCommandFailure(channelProvider, binding, nil, message, "绑定失败：协作渠道凭证不存在。请联系管理员重新连接该 Agent 的协作渠道。", "secret_missing")
	}
	values, err := openConnectionSecret(secret)
	if err != nil {
		return nil, err
	}
	resolved := resolvedChannelEventBinding{Binding: *binding, SecretValues: values, Identity: controldb.ExternalIdentity{WorkspaceID: binding.WorkspaceID, Provider: provider, ExternalUserID: message.SenderOpenID, UserID: codeRow.UserID}}
	if !verifyIMEventToken(verificationToken, values) {
		_, _ = s.replyBindCommandFailure(channelProvider, binding, values, message, "绑定失败：事件校验未通过。请联系管理员检查协作渠道配置。", "verification_failed")
		return map[string]any{"ok": true, "ignored": true, "reason": "verification_failed"}, nil
	}
	if targetType == "chat" {
		return s.acceptAgentChannelChatBindCommand(channelProvider, *binding, values, codeRow, message, now)
	}
	metaRaw, _ := json.Marshal(map[string]any{
		"source":      "agent_channel_bind_code",
		"project":     binding.ProjectID,
		"agent":       binding.AgentID,
		"provider":    provider,
		"boundAt":     now.Format(time.RFC3339),
		"chatId":      message.ChatID,
		"messageId":   message.MessageID,
		"bindCode":    codeRow.Code,
		"channelId":   binding.ID,
		"workspaceId": binding.WorkspaceID,
	})
	if err := s.controlDB.UpsertUserChannelIdentity(controldb.UserChannelIdentity{
		ID:               newChannelID("uch"),
		WorkspaceID:      binding.WorkspaceID,
		UserID:           codeRow.UserID,
		ChannelBindingID: binding.ID,
		Provider:         provider,
		ExternalUserID:   strings.TrimSpace(message.SenderOpenID),
		ExternalChatID:   strings.TrimSpace(message.ChatID),
		MetadataJSON:     string(metaRaw),
		CreatedBy:        codeRow.UserID,
		CreatedAt:        now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
	}); err != nil {
		return nil, err
	}
	_ = s.controlDB.UpsertExternalIdentity(controldb.ExternalIdentity{
		ID:             newChannelID("ext"),
		WorkspaceID:    binding.WorkspaceID,
		Provider:       provider,
		ExternalUserID: strings.TrimSpace(message.SenderOpenID),
		UserID:         codeRow.UserID,
		MetadataJSON:   string(metaRaw),
		CreatedBy:      codeRow.UserID,
		CreatedAt:      now.Format(time.RFC3339),
		UpdatedAt:      now.Format(time.RFC3339),
	})
	binding.LastActivityAt = now.Format(time.RFC3339)
	binding.UpdatedAt = now.Format(time.RFC3339)
	_ = s.controlDB.UpsertAgentChannelBinding(*binding)
	_ = s.controlDB.MarkAgentChannelBindCodeUsed(codeRow.Code, now.Format(time.RFC3339))
	s.recordAgentChannelCallback(*binding, "accepted", "identity_bound", message, "")
	s.auditLog(auditLogInput{
		WorkspaceID:  binding.WorkspaceID,
		ActorType:    "user",
		ActorID:      codeRow.UserID,
		Action:       "agent_channel.identity_bound",
		ResourceType: "agent_channel",
		ResourceID:   binding.ID,
		Summary:      fmt.Sprintf("Bound %s user to %s/%s channel", provider, binding.ProjectID, binding.AgentID),
		After: map[string]any{
			"provider":       provider,
			"project":        binding.ProjectID,
			"agent":          binding.AgentID,
			"externalUserId": message.SenderOpenID,
			"chatId":         message.ChatID,
		},
	})
	reply := fmt.Sprintf("绑定成功。之后 %s/%s 可以通过 %s 通知你。", binding.ProjectID, binding.AgentID, channelProvider.Info().Label)
	if err := s.replyToIMEvent(context.Background(), channelProvider, resolved, message, reply); err != nil {
		s.recordAgentChannelCallback(*binding, "reply_failed", "identity_bound", message, err.Error())
	}
	return map[string]any{"ok": true, "bound": true, "user": codeRow.UserID, "channelId": binding.ID}, nil
}

func (s *Server) acceptAgentChannelChatBindCommand(channelProvider imbridge.Provider, binding controldb.AgentChannelBinding, values map[string]string, codeRow controldb.AgentChannelBindCode, message imbridge.IncomingMessage, now time.Time) (map[string]any, error) {
	provider := channelProvider.Info().ID
	chatName := strings.TrimSpace(codeRow.TargetName)
	if chatName == "" {
		chatName = strings.TrimSpace(message.ChatID)
	}
	metaRaw, _ := json.Marshal(map[string]any{
		"source":      "agent_channel_bind_code",
		"project":     binding.ProjectID,
		"agent":       binding.AgentID,
		"provider":    provider,
		"targetType":  "chat",
		"boundAt":     now.Format(time.RFC3339),
		"chatId":      message.ChatID,
		"messageId":   message.MessageID,
		"bindCode":    codeRow.Code,
		"channelId":   binding.ID,
		"workspaceId": binding.WorkspaceID,
	})
	if err := s.controlDB.UpsertAgentChannelTarget(controldb.AgentChannelTarget{
		ID:               newChannelID("cht"),
		WorkspaceID:      binding.WorkspaceID,
		ChannelBindingID: binding.ID,
		Provider:         provider,
		TargetType:       "chat",
		DisplayName:      chatName,
		ExternalUserID:   strings.TrimSpace(message.SenderOpenID),
		ExternalChatID:   strings.TrimSpace(message.ChatID),
		MetadataJSON:     string(metaRaw),
		CreatedBy:        codeRow.UserID,
		CreatedAt:        now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		LastActivityAt:   now.Format(time.RFC3339),
	}); err != nil {
		return nil, err
	}
	binding.ExternalChatID = strings.TrimSpace(message.ChatID)
	binding.LastActivityAt = now.Format(time.RFC3339)
	binding.UpdatedAt = now.Format(time.RFC3339)
	_ = s.controlDB.UpsertAgentChannelBinding(binding)
	_ = s.controlDB.MarkAgentChannelBindCodeUsed(codeRow.Code, now.Format(time.RFC3339))
	s.recordAgentChannelCallback(binding, "accepted", "chat_bound", message, "")
	s.auditLog(auditLogInput{
		WorkspaceID:  binding.WorkspaceID,
		ActorType:    "user",
		ActorID:      codeRow.UserID,
		Action:       "agent_channel.chat_bound",
		ResourceType: "agent_channel",
		ResourceID:   binding.ID,
		Summary:      fmt.Sprintf("Bound %s chat target %q to %s/%s channel", provider, chatName, binding.ProjectID, binding.AgentID),
		After: map[string]any{
			"provider": provider,
			"project":  binding.ProjectID,
			"agent":    binding.AgentID,
			"name":     chatName,
			"chatId":   message.ChatID,
		},
	})
	resolved := resolvedChannelEventBinding{Binding: binding, SecretValues: values, Identity: controldb.ExternalIdentity{WorkspaceID: binding.WorkspaceID, Provider: provider, ExternalUserID: message.SenderOpenID, UserID: codeRow.UserID}}
	reply := fmt.Sprintf("群聊绑定成功。之后 %s/%s 可以通过 %s 通知群聊「%s」。", binding.ProjectID, binding.AgentID, channelProvider.Info().Label, chatName)
	if err := s.replyToIMEvent(context.Background(), channelProvider, resolved, message, reply); err != nil {
		s.recordAgentChannelCallback(binding, "reply_failed", "chat_bound", message, err.Error())
	}
	return map[string]any{"ok": true, "bound": true, "target": "chat", "name": chatName, "channelId": binding.ID}, nil
}

func (s *Server) replyBindCommandFailure(channelProvider imbridge.Provider, binding *controldb.AgentChannelBinding, values map[string]string, message imbridge.IncomingMessage, reply, reason string) (map[string]any, error) {
	if binding != nil && values != nil {
		resolved := resolvedChannelEventBinding{Binding: *binding, SecretValues: values, Identity: controldb.ExternalIdentity{WorkspaceID: binding.WorkspaceID, Provider: channelProvider.Info().ID, ExternalUserID: message.SenderOpenID}}
		if err := s.replyToIMEvent(context.Background(), channelProvider, resolved, message, reply); err != nil {
			s.recordAgentChannelCallback(*binding, "reply_failed", reason, message, err.Error())
		} else {
			s.recordAgentChannelCallback(*binding, "rejected", reason, message, "")
		}
	}
	return map[string]any{"ok": true, "ignored": true, "reason": reason}, nil
}

func parseAgentChannelBindCommand(text string) (string, string, bool) {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) < 2 {
		return "", "", false
	}
	cmd := strings.ToLower(strings.TrimSpace(fields[0]))
	cmd = strings.TrimPrefix(cmd, "/")
	if cmd != "bind" && cmd != "bind-chat" {
		return "", "", false
	}
	code := strings.ToUpper(strings.TrimSpace(fields[1]))
	if !strings.HasPrefix(code, "MG-") {
		return "", "", false
	}
	return cmd, code, true
}

func (s *Server) decryptIMEvent(provider imbridge.Provider, encryptedPayload string) ([]byte, bool, error) {
	providerID := provider.Info().ID
	bindings, err := s.controlDB.ListAgentChannelBindings(controldb.AgentChannelBindingFilter{
		Provider: providerID,
		Status:   "connected",
	})
	if err != nil {
		return nil, false, err
	}
	for _, binding := range bindings {
		secret, ok, err := s.controlDB.ConnectionSecret(binding.ConnectionID)
		if err != nil {
			return nil, false, err
		}
		if !ok {
			continue
		}
		values, err := openConnectionSecret(secret)
		if err != nil {
			return nil, false, err
		}
		encryptKey := strings.TrimSpace(values["encryptKey"])
		if encryptKey == "" {
			continue
		}
		decrypted, err := provider.DecryptEvent(encryptedPayload, encryptKey)
		if err == nil {
			return decrypted, true, nil
		}
	}
	return nil, false, nil
}

func (s *Server) resolveChannelEventBinding(provider, appID, chatID, externalUserID string) (resolvedChannelEventBinding, bool, error) {
	resolution, err := s.resolveChannelEventBindingDetailed(provider, appID, chatID, externalUserID)
	return resolution.Resolved, resolution.Found, err
}

func (s *Server) resolveChannelEventBindingDetailed(provider, appID, chatID, externalUserID string) (channelEventResolution, error) {
	bindings, err := s.matchChannelEventBindings(provider, appID, chatID)
	if err != nil {
		return channelEventResolution{}, err
	}
	if len(bindings) == 0 {
		return channelEventResolution{}, nil
	}
	identities, err := s.controlDB.ListExternalIdentities(controldb.ExternalIdentityFilter{
		Provider:       provider,
		ExternalUserID: strings.TrimSpace(externalUserID),
	})
	if err != nil {
		return channelEventResolution{}, err
	}
	userChannelIdentities, err := s.controlDB.ListUserChannelIdentities(controldb.UserChannelIdentityFilter{
		Provider:       provider,
		ExternalUserID: strings.TrimSpace(externalUserID),
	})
	if err != nil {
		return channelEventResolution{}, err
	}
	for _, binding := range bindings {
		for _, identity := range userChannelIdentities {
			if identity.WorkspaceID != binding.WorkspaceID || identity.ChannelBindingID != binding.ID {
				continue
			}
			secret, ok, err := s.controlDB.ConnectionSecret(binding.ConnectionID)
			if err != nil {
				return channelEventResolution{}, err
			}
			if !ok {
				continue
			}
			values, err := openConnectionSecret(secret)
			if err != nil {
				return channelEventResolution{}, err
			}
			return channelEventResolution{
				Resolved: resolvedChannelEventBinding{Binding: binding, SecretValues: values, Identity: controldb.ExternalIdentity{
					WorkspaceID: binding.WorkspaceID, Provider: provider, ExternalUserID: externalUserID, UserID: identity.UserID,
				}},
				Found: true,
			}, nil
		}
	}
	if len(identities) == 0 {
		return channelEventResolution{Candidate: bindings[0], HasCandidate: true}, nil
	}
	identityByWorkspace := map[string]controldb.ExternalIdentity{}
	for _, identity := range identities {
		identityByWorkspace[identity.WorkspaceID] = identity
	}
	for _, binding := range bindings {
		identity, ok := identityByWorkspace[binding.WorkspaceID]
		if !ok {
			continue
		}
		secret, ok, err := s.controlDB.ConnectionSecret(binding.ConnectionID)
		if err != nil {
			return channelEventResolution{}, err
		}
		if !ok {
			continue
		}
		values, err := openConnectionSecret(secret)
		if err != nil {
			return channelEventResolution{}, err
		}
		return channelEventResolution{
			Resolved: resolvedChannelEventBinding{Binding: binding, SecretValues: values, Identity: identity},
			Found:    true,
		}, nil
	}
	return channelEventResolution{Candidate: bindings[0], HasCandidate: true}, nil
}

func (s *Server) matchChannelEventBindings(provider, appID, chatID string) ([]controldb.AgentChannelBinding, error) {
	bindings, err := s.controlDB.ListAgentChannelBindings(controldb.AgentChannelBindingFilter{
		Provider: provider,
		Status:   "connected",
	})
	if err != nil {
		return nil, err
	}
	out := make([]controldb.AgentChannelBinding, 0, len(bindings))
	for _, binding := range bindings {
		if channelEventBindingMatches(binding, appID, chatID) {
			out = append(out, binding)
		}
	}
	return out, nil
}

func channelEventBindingMatches(binding controldb.AgentChannelBinding, appID, chatID string) bool {
	var meta struct {
		AppID string `json:"appId"`
	}
	_ = json.Unmarshal([]byte(binding.MetadataJSON), &meta)
	if strings.TrimSpace(meta.AppID) != "" {
		return strings.TrimSpace(appID) == strings.TrimSpace(meta.AppID)
	}
	return true
}

func (s *Server) runAgentForIMEvent(provider imbridge.Provider, resolved resolvedChannelEventBinding, message imbridge.IncomingMessage, text string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	binding := resolved.Binding
	providerID := provider.Info().ID
	source := interaction.Source{
		Kind:    providerID,
		ActorID: resolved.Identity.UserID,
		Channel: message.ChatID,
	}
	lease, err := s.acquireAgentInteractionLease(s.interactionAgentRef(binding.WorkspaceID, binding.ProjectID, binding.AgentID), source, "interactive")
	if err != nil {
		if errors.Is(err, interaction.ErrAgentLocked) {
			s.recordAgentChannelCallback(binding, "busy", "agent_locked", message, "")
			s.auditLog(auditLogInput{
				WorkspaceID:  binding.WorkspaceID,
				ActorType:    "user",
				ActorID:      resolved.Identity.UserID,
				Action:       "agent_channel.busy",
				ResourceType: "agent_channel",
				ResourceID:   binding.ID,
				Summary:      fmt.Sprintf("Ignored %s message for %s/%s because the agent is busy", providerID, binding.ProjectID, binding.AgentID),
				After: map[string]any{
					"provider":  providerID,
					"messageId": message.MessageID,
					"chatId":    message.ChatID,
				},
			})
			s.replyToIMEvent(ctx, provider, resolved, message, "Agent is currently busy in another session. Please wait for the current run to finish, or stop it from Multigent and try again.")
			return
		}
		log.Printf("[im:%s] acquire session failed for %s/%s: %v", providerID, binding.ProjectID, binding.AgentID, err)
		return
	}
	defer lease.Release()
	_ = s.createInteractionEvent(lease.session, "user", resolved.Identity.UserID, providerID, "message", text, map[string]any{
		"messageId": message.MessageID,
		"chatId":    message.ChatID,
	})
	if binding.ExternalChatID == "" && message.ChatID != "" {
		binding.ExternalChatID = message.ChatID
		binding.LastActivityAt = time.Now().UTC().Format(time.RFC3339)
		binding.UpdatedAt = binding.LastActivityAt
		_ = s.controlDB.UpsertAgentChannelBinding(binding)
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  binding.WorkspaceID,
		ActorType:    "user",
		ActorID:      resolved.Identity.UserID,
		Action:       "agent_channel.message_received",
		ResourceType: "agent_channel",
		ResourceID:   binding.ID,
		Summary:      fmt.Sprintf("Received %s message for %s/%s", providerID, binding.ProjectID, binding.AgentID),
		After: map[string]any{
			"provider":  providerID,
			"messageId": message.MessageID,
			"chatId":    message.ChatID,
		},
	})
	_ = s.createInteractionEvent(lease.session, "system", "", providerID, "run_started", "", map[string]any{
		"messageId": message.MessageID,
	})
	if err := s.replyToIMEvent(ctx, provider, resolved, message, "⏳"); err != nil {
		s.recordAgentChannelCallback(binding, "ack_failed", "", message, err.Error())
	}
	output, detectedRuntimeSessionID, err := s.execAgentPrompt(ctx, binding.ProjectID, binding.AgentID, text, "")
	if detectedRuntimeSessionID != "" {
		lease.SetRuntimeSessionID(detectedRuntimeSessionID)
	}
	reply := extractAgentChatReply(output)
	if err != nil {
		lease.Fail(err.Error())
		reply = "Agent run failed: " + err.Error()
		if cleaned := extractAgentChatReply(output); cleaned != "" {
			reply += "\n\n" + cleaned
		}
	}
	if strings.TrimSpace(reply) == "" {
		reply = "已处理，但没有生成可展示的回复。"
	}
	reply = trimForIM(reply, 3500)
	replyErr := s.replyToIMEvent(ctx, provider, resolved, message, reply)
	if replyErr != nil {
		s.recordAgentChannelCallback(binding, "reply_failed", "", message, replyErr.Error())
	} else if err != nil {
		s.recordAgentChannelCallback(binding, "run_failed", "", message, err.Error())
	} else {
		s.recordAgentChannelCallback(binding, "replied", "", message, "")
	}
	if err != nil {
		_ = s.createInteractionEvent(lease.session, "system", "", providerID, "run_failed", output, map[string]any{
			"messageId":        message.MessageID,
			"error":            err.Error(),
			"runtimeSessionId": detectedRuntimeSessionID,
		})
	} else {
		_ = s.createInteractionEvent(lease.session, "agent", binding.ProjectID+"/"+binding.AgentID, providerID, "run_completed", reply, map[string]any{
			"messageId":        message.MessageID,
			"replyErr":         errString(replyErr),
			"runtimeSessionId": detectedRuntimeSessionID,
		})
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  binding.WorkspaceID,
		ActorType:    "agent",
		ActorID:      binding.ProjectID + "/" + binding.AgentID,
		Action:       "agent_channel.replied",
		ResourceType: "agent_channel",
		ResourceID:   binding.ID,
		Summary:      fmt.Sprintf("Replied to %s message for %s/%s", providerID, binding.ProjectID, binding.AgentID),
		After: map[string]any{
			"provider":  providerID,
			"messageId": message.MessageID,
			"error":     errString(replyErr),
		},
	})
}

func (s *Server) replyToIMEvent(ctx context.Context, provider imbridge.Provider, resolved resolvedChannelEventBinding, message imbridge.IncomingMessage, reply string) error {
	binding := resolved.Binding
	if err := provider.ReplyText(ctx, resolved.SecretValues, message, reply); err != nil {
		log.Printf("[im:%s] reply failed for %s/%s: %v", provider.Info().ID, binding.ProjectID, binding.AgentID, err)
		return err
	}
	return nil
}

func (s *Server) recordAgentChannelCallback(binding controldb.AgentChannelBinding, status, reason string, message imbridge.IncomingMessage, errorText string) {
	now := time.Now().UTC().Format(time.RFC3339)
	meta := map[string]any{}
	_ = json.Unmarshal([]byte(binding.MetadataJSON), &meta)
	meta["lastCallback"] = map[string]any{
		"at":        now,
		"status":    strings.TrimSpace(status),
		"reason":    strings.TrimSpace(reason),
		"messageId": strings.TrimSpace(message.MessageID),
		"chatId":    strings.TrimSpace(message.ChatID),
		"chatType":  strings.TrimSpace(message.ChatType),
		"error":     strings.TrimSpace(errorText),
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		log.Printf("[im:%s] marshal callback metadata failed for %s/%s: %v", binding.Provider, binding.ProjectID, binding.AgentID, err)
		return
	}
	binding.MetadataJSON = string(raw)
	binding.UpdatedAt = now
	if status == "accepted" || status == "replied" {
		binding.LastActivityAt = now
	}
	if err := s.controlDB.UpsertAgentChannelBinding(binding); err != nil {
		log.Printf("[im:%s] update callback metadata failed for %s/%s: %v", binding.Provider, binding.ProjectID, binding.AgentID, err)
	}
}

func (s *Server) userCanOperateAgent(username, project, agent string) bool {
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		return false
	}
	return s.userCanOperateAgentInWorkspace(username, workspaceID, project, agent)
}

func (s *Server) userCanOperateAgentInWorkspace(username, workspaceID, project, agent string) bool {
	username = strings.TrimSpace(username)
	if username == "" {
		return false
	}
	if username == "apikey" {
		return true
	}
	cur := s.users.GetUser(username)
	if cur == nil {
		cur = &userRecord{Username: username, Role: RoleMember}
	}
	if cur.Role == RoleAdmin {
		return true
	}
	if s.controlDB != nil {
		member, ok, err := s.controlDB.WorkspaceMember(workspaceID, username)
		if err == nil && ok && (member.Role == WorkspaceRoleOwner || member.Role == WorkspaceRoleAdmin) {
			return true
		}
	}
	role, ok := s.users.HasProjectAccess(username, project)
	if ok && projectRoleLevel(role) >= projectRoleLevel(ProjectRoleOperator) {
		return true
	}
	return currentUserLinkedAgent(cur, project+"/"+agent)
}

func verifyIMEventToken(token string, values map[string]string) bool {
	expected := strings.TrimSpace(values["verificationToken"])
	if expected == "" {
		return true
	}
	return subtleConstantTimeEqual(strings.TrimSpace(token), expected)
}

func subtleConstantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

func (s *Server) execAgentPrompt(ctx context.Context, project, agent, prompt, sessionID string) (string, string, error) {
	args := []string{"--dir", s.root, "exec", "--project", project, "--agent", agent, "--prompt", prompt, "--no-save-session"}
	if strings.TrimSpace(sessionID) != "" {
		args = append(args, "--session", strings.TrimSpace(sessionID))
	} else {
		args = append(args, "--no-session")
	}
	cmd := exec.CommandContext(ctx, s.sched.binPath, args...)
	cmd.Dir = s.root
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	output := out.String()
	return output, extractAgentChatSessionID(output), err
}

func trimForIM(s string, max int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max]) + "\n\n...(truncated)"
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
