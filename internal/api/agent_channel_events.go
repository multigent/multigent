package api

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
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

func (s *Server) shouldWakeAgentForAttention(binding controldb.AgentChannelBinding, reason string) bool {
	if s == nil {
		return false
	}
	reason = strings.TrimSpace(reason)
	if strings.TrimSpace(binding.AgentWorkerID) != "" && s.controlDB != nil {
		worker, ok, err := s.controlDB.AgentWorkerByID(binding.WorkspaceID, binding.AgentWorkerID)
		if err == nil && ok {
			if hb, configured := agentWorkerScheduleHeartbeat(worker); configured {
				return hb.HasAttentionTrigger(reason)
			}
		}
	}
	return false
}

func (s *Server) requestAgentAttentionWakeup(binding controldb.AgentChannelBinding, reason, runtimeAPIURL, actor, attentionID string) {
	if s == nil {
		return
	}
	providerContext := map[string]any{
		"project":     binding.ProjectID,
		"agent":       binding.AgentID,
		"agentWorker": binding.AgentWorkerID,
		"reason":      strings.TrimSpace(reason),
		"attentionId": strings.TrimSpace(attentionID),
	}
	target := s.runtimeSchedulerTargetForProjectAgent(binding.WorkspaceID, binding.ProjectID, binding.AgentID)
	hb, err := s.loadSchedulerTargetHeartbeat(binding.WorkspaceID, target)
	if err != nil || hb == nil {
		log.Printf("[attention] load heartbeat failed for %s/%s: %v", binding.ProjectID, binding.AgentID, err)
		return
	}
	attentionTask, _, err := s.ensurePendingAttentionWakeupTask(binding.WorkspaceID, binding.ProjectID, binding.AgentID)
	if err != nil {
		log.Printf("[attention] ensure attention wakeup task failed for %s/%s: %v", binding.ProjectID, binding.AgentID, err)
		return
	}
	if attentionTask != nil {
		providerContext["attentionTaskId"] = attentionTask.ID
	}
	if hb.PID > 0 && hb.LastWakeupStatus == "running" && processAlive(hb.PID) {
		s.auditLog(auditLogInput{
			WorkspaceID:  binding.WorkspaceID,
			ActorType:    "system",
			ActorID:      "attention",
			Action:       "attention.wakeup_skipped",
			ResourceType: "agent",
			ResourceID:   binding.ProjectID + "/" + binding.AgentID,
			Summary:      "Skipped attention wakeup because agent is already running",
			After:        providerContext,
		})
		return
	}
	meta, err := s.agentMetaForProjectMember(binding.WorkspaceID, binding.ProjectID, binding.AgentID)
	if err != nil || meta == nil {
		log.Printf("[attention] load agent meta failed for %s/%s: %v", binding.ProjectID, binding.AgentID, err)
		return
	}
	if readiness := s.runtimeReadinessForExecution(binding.WorkspaceID, meta); readiness.Blocking {
		providerContext["readiness"] = runtimeReadinessErrorMessage(readiness)
		s.auditLog(auditLogInput{
			WorkspaceID:  binding.WorkspaceID,
			ActorType:    "system",
			ActorID:      "attention",
			Action:       "attention.wakeup_failed",
			ResourceType: "agent",
			ResourceID:   binding.ProjectID + "/" + binding.AgentID,
			Summary:      "Attention wakeup skipped because runtime is not ready",
			After:        providerContext,
		})
		return
	}
	if s.usesAssignedRuntimeNode(binding.WorkspaceID, meta) {
		if s.hasActiveRuntimeRunForTarget(binding.WorkspaceID, target, "") {
			return
		}
		run, task, err := s.enqueueRuntimeWakeupRunFromRequest(binding.WorkspaceID, binding.ProjectID, binding.AgentID, hb, runtimeAPIURL, actor)
		if err != nil {
			log.Printf("[attention] queue runtime wakeup failed for %s/%s: %v", binding.ProjectID, binding.AgentID, err)
			return
		}
		_ = s.saveSchedulerTargetHeartbeat(binding.WorkspaceID, target, hb)
		providerContext["runtimeRunId"] = run.ID
		if task != nil {
			providerContext["taskId"] = task.ID
		}
		s.auditLog(auditLogInput{
			WorkspaceID:  binding.WorkspaceID,
			ActorType:    "system",
			ActorID:      "attention",
			Action:       "attention.wakeup_queued",
			ResourceType: "agent",
			ResourceID:   binding.ProjectID + "/" + binding.AgentID,
			Summary:      "Attention wakeup queued on runtime node",
			After:        providerContext,
		})
		return
	}
	if s.sched == nil || strings.TrimSpace(s.sched.binPath) == "" || strings.TrimSpace(s.sched.root) == "" {
		log.Printf("[attention] scheduler unavailable for %s/%s", binding.ProjectID, binding.AgentID)
		return
	}
	args := []string{"--dir", s.sched.root, "scheduler", "wakeup", "--project", binding.ProjectID, "--agent", binding.AgentID}
	cmd := exec.Command(s.sched.binPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	setProcGroup(cmd)
	if err := cmd.Start(); err != nil {
		log.Printf("[attention] start wakeup failed for %s/%s: %v", binding.ProjectID, binding.AgentID, err)
		return
	}
	if cmd.Process != nil {
		providerContext["pid"] = cmd.Process.Pid
	}
	go func() {
		if err := cmd.Wait(); err != nil {
			log.Printf("[attention] wakeup %s/%s exited with error: %v", binding.ProjectID, binding.AgentID, err)
		}
	}()
	s.auditLog(auditLogInput{
		WorkspaceID:  binding.WorkspaceID,
		ActorType:    "system",
		ActorID:      "attention",
		Action:       "attention.wakeup_started",
		ResourceType: "agent",
		ResourceID:   binding.ProjectID + "/" + binding.AgentID,
		Summary:      "Attention wakeup started",
		After:        providerContext,
	})
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
	if parsed.IsInteraction {
		result, err := s.acceptIMInteractionCallback(channelProvider, parsed.AppID, parsed.VerificationToken, parsed.Interaction, localRuntimeAPIURLForRequest(r))
		if err != nil {
			s.serverError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(result)
		return
	}
	if !parsed.IsMessage {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ignored": true})
		return
	}
	result, err := s.acceptIMMessage(channelProvider, parsed.AppID, parsed.VerificationToken, parsed.Message, localRuntimeAPIURLForRequest(r))
	if err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Server) acceptIMInteractionCallback(channelProvider imbridge.Provider, appID, verificationToken string, callback imbridge.IncomingInteractionCallback, runtimeAPIURL string) (map[string]any, error) {
	provider := channelProvider.Info().ID
	interactionID := strings.TrimSpace(callback.InteractionID)
	if interactionID == "" {
		return map[string]any{"ok": true, "ignored": true, "reason": "interaction_id_missing"}, nil
	}
	matches, err := s.matchChannelEventBindings(provider, appID, callback.ChatID)
	if err != nil {
		return nil, err
	}
	for _, binding := range matches {
		request, found, err := s.controlDB.InteractionRequestByID(binding.WorkspaceID, interactionID)
		if err != nil {
			return nil, err
		}
		if !found || request.ChannelBindingID != binding.ID {
			continue
		}
		return s.acceptBoundIMInteractionCallback(channelProvider, binding, request, verificationToken, callback, runtimeAPIURL)
	}
	log.Printf("[im:%s] interaction request not found app=%s chat=%s interaction=%s sender=%s", provider, appID, callback.ChatID, interactionID, callback.SenderOpenID)
	return map[string]any{"ok": true, "ignored": true, "reason": "interaction_not_found"}, nil
}

func (s *Server) acceptBoundIMInteractionCallback(channelProvider imbridge.Provider, binding controldb.AgentChannelBinding, request controldb.InteractionRequest, verificationToken string, callback imbridge.IncomingInteractionCallback, runtimeAPIURL string) (map[string]any, error) {
	providerID := channelProvider.Info().ID
	secret, ok, err := s.controlDB.ConnectionSecret(binding.ConnectionID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return map[string]any{"ok": true, "ignored": true, "reason": "secret_missing"}, nil
	}
	values, err := openConnectionSecret(secret)
	if err != nil {
		return nil, err
	}
	if !verifyIMEventToken(verificationToken, values) {
		return map[string]any{"ok": true, "ignored": true, "reason": "verification_failed"}, nil
	}
	identities, err := s.controlDB.ListUserChannelIdentities(controldb.UserChannelIdentityFilter{
		WorkspaceID:      binding.WorkspaceID,
		ChannelBindingID: binding.ID,
		Provider:         providerID,
		ExternalUserID:   callback.SenderOpenID,
	})
	if err != nil {
		return nil, err
	}
	if len(identities) == 0 {
		return map[string]any{"ok": true, "ignored": true, "reason": "unknown_identity"}, nil
	}
	identity := controldb.ExternalIdentity{WorkspaceID: binding.WorkspaceID, Provider: providerID, ExternalUserID: callback.SenderOpenID, UserID: identities[0].UserID}
	userID := identity.UserID
	if strings.TrimSpace(request.TargetUserID) != "" && request.TargetUserID != userID {
		return map[string]any{"ok": true, "ignored": true, "reason": "actor_not_allowed"}, nil
	}
	if strings.TrimSpace(request.Status) != "" && request.Status != "active" {
		return map[string]any{"ok": true, "ignored": true, "reason": "interaction_not_active"}, nil
	}
	if exp := strings.TrimSpace(request.ExpiresAt); exp != "" {
		expiresAt, err := time.Parse(time.RFC3339, exp)
		if err == nil && time.Now().UTC().After(expiresAt) {
			request.Status = "expired"
			_ = s.controlDB.UpdateInteractionRequest(request)
			return map[string]any{"ok": true, "ignored": true, "reason": "interaction_expired"}, nil
		}
	}
	submission := map[string]any{
		"interactionId": request.ID,
		"actionId":      strings.TrimSpace(callback.ActionID),
		"actionLabel":   strings.TrimSpace(callback.ActionLabel),
		"inputs":        callback.Inputs,
		"userId":        userID,
		"provider":      providerID,
		"chatId":        callback.ChatID,
		"messageId":     callback.MessageID,
		"context":       rawJSONToMap(request.ContextJSON),
		"handlerType":   request.HandlerType,
	}
	rawSubmission, _ := json.Marshal(submission)
	now := time.Now().UTC().Format(time.RFC3339)
	request.Status = "submitted"
	request.SubmittedAt = now
	request.SubmittedBy = userID
	request.SubmissionJSON = string(rawSubmission)
	if err := s.controlDB.UpdateInteractionRequest(request); err != nil {
		return nil, err
	}
	resolved := resolvedChannelEventBinding{Binding: binding, SecretValues: values, Identity: identity}
	attentionID := s.recordIMInteractionAttentionSignal(resolved, providerID, callback, request, string(rawSubmission))
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.updateIMInteractionCardAccepted(ctx, channelProvider, resolved, callback, request); err != nil {
			log.Printf("[im:%s] interaction card update failed for %s/%s interaction=%s: %v", providerID, binding.ProjectID, binding.AgentID, request.ID, err)
			if replyErr := s.replyToIMInteraction(ctx, channelProvider, resolved, callback, "已收到你的选择，Agent 正在处理。"); replyErr != nil {
				log.Printf("[im:%s] interaction ack reply failed for %s/%s interaction=%s: %v", providerID, binding.ProjectID, binding.AgentID, request.ID, replyErr)
			}
		}
	}()
	if !s.shouldWakeAgentForAttention(binding, "card_action") {
		return map[string]any{"ok": true, "interactionId": request.ID, "status": "queued", "attentionId": attentionID}, nil
	}
	go s.requestAgentAttentionWakeup(binding, "card_action", runtimeAPIURL, userID, attentionID)
	return map[string]any{"ok": true, "interactionId": request.ID, "status": "accepted"}, nil
}

func (s *Server) updateIMInteractionCardAccepted(ctx context.Context, provider imbridge.Provider, resolved resolvedChannelEventBinding, callback imbridge.IncomingInteractionCallback, request controldb.InteractionRequest) error {
	updater, ok := provider.(imbridge.InteractionCardUpdater)
	if !ok {
		return fmt.Errorf("provider %s does not support interaction card updates", provider.Info().ID)
	}
	action := strings.TrimSpace(callback.ActionLabel)
	if action == "" {
		action = strings.TrimSpace(callback.ActionID)
	}
	if action == "" {
		action = "已选择"
	}
	fields := []imbridge.InteractiveCardField{{Label: "选择", Value: action}}
	if comment := strings.TrimSpace(firstNonEmpty(callback.Inputs["comment"], callback.Inputs["comments"], callback.Inputs["reason"])); comment != "" {
		fields = append(fields, imbridge.InteractiveCardField{Label: "补充说明", Value: comment})
	}
	return updater.UpdateInteractionCard(ctx, resolved.SecretValues, callback, imbridge.OutgoingMessage{Card: &imbridge.InteractiveCard{
		InteractionID: request.ID,
		Title:         "已提交，Agent 正在处理",
		Body:          "你的选择已经提交给 Multigent。Agent 会把这次卡片回调当作结构化消息继续处理。",
		Fields:        fields,
	}})
}

func (s *Server) acceptIMMessage(channelProvider imbridge.Provider, appID, verificationToken string, message imbridge.IncomingMessage, runtimeAPIURL string) (map[string]any, error) {
	provider := channelProvider.Info().ID
	text := strings.TrimSpace(message.Text)
	if text == "" {
		text = incomingMessageFallbackText(message)
	}
	if text == "" && len(message.Attachments) == 0 {
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
	reason := imAttentionReason(message)
	attentionID := s.recordIMAttentionSignal(resolved, provider, message, text)
	if !s.shouldWakeAgentForAttention(resolved.Binding, reason) {
		s.recordAgentChannelCallback(resolved.Binding, "queued", "attention_pending", message, "")
		return map[string]any{"ok": true, "queued": true, "attentionId": attentionID}, nil
	}
	meta, err := s.agentMetaForProjectMember(resolved.Binding.WorkspaceID, resolved.Binding.ProjectID, resolved.Binding.AgentID)
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
	go s.requestAgentAttentionWakeup(resolved.Binding, reason, runtimeAPIURL, resolved.Identity.UserID, attentionID)
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

func imConversationSource(providerID, userID, chatID, chatType string) interaction.Source {
	providerID = strings.TrimSpace(providerID)
	userID = strings.TrimSpace(userID)
	chatID = strings.TrimSpace(chatID)
	chatType = strings.TrimSpace(chatType)
	kind := providerID
	if chatID == "" {
		chatID = "direct"
	}
	if chatType == "" {
		chatType = "chat"
	}
	channel := "im:" + providerID + ":" + chatType + ":" + chatID
	if userID != "" {
		channel += ":user:" + userID
	}
	return interaction.Source{
		Kind:    kind,
		ActorID: userID,
		Channel: channel,
	}
}

func (s *Server) runAgentForIMEvent(provider imbridge.Provider, resolved resolvedChannelEventBinding, message imbridge.IncomingMessage, text string, runtimeAPIURL string, attentionID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	binding := resolved.Binding
	providerID := provider.Info().ID
	if attentionID != "" {
		_ = s.controlDB.MarkAttentionSignalStatus(binding.WorkspaceID, attentionID, "seen")
		_ = s.controlDB.MarkAttentionSignalStatus(binding.WorkspaceID, attentionID, "handling")
	}
	source := imConversationSource(providerID, resolved.Identity.UserID, message.ChatID, message.ChatType)
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
	stopIndicator := s.startIMProcessingIndicator(ctx, provider, resolved, message)
	defer stopIndicator()
	progress := newIMProgressReporter(ctx, provider, resolved, message, agentChannelReplySubject(binding.AgentID))
	prompt := formatIMAgentPromptWithSender(providerID, binding, resolved.Identity, s.imIdentityDisplayLabel(resolved.Identity), message, text)
	output, detectedRuntimeSessionID, err := s.execAgentPromptStream(ctx, binding.WorkspaceID, binding.ProjectID, binding.AgentID, prompt, lease.session.RuntimeSessionID, runtimeAPIURL, func(line string) {
		progress.Observe(line)
	})
	if detectedRuntimeSessionID != "" {
		lease.SetRuntimeSessionID(detectedRuntimeSessionID)
	}
	reply := extractAgentChatReply(output)
	if err != nil {
		lease.Fail(err.Error())
		reply = "### 处理失败\n\nAgent 运行失败，已记录到 Multigent 运行日志。\n\n**错误**：" + err.Error()
		if cleaned := extractAgentChatReply(output); cleaned != "" {
			reply += "\n\n" + cleaned
		}
	}
	if strings.TrimSpace(reply) == "" {
		reply = "Agent 已处理完成，但没有返回明确的最终文本。你可以继续补充问题，我会让它给出结论和下一步。"
	}
	reply = trimForIM(reply, 3500)
	state := "completed"
	if err != nil {
		state = "failed"
	}
	progressErr := progress.Finalize(state, reply)
	finalReplyErr := s.replyMessageToIMEvent(ctx, provider, resolved, message, imbridge.OutgoingMessage{
		Format:  "markdown",
		Subject: agentChannelReplySubject(binding.AgentID),
		Text:    reply,
	})
	replyErr := finalReplyErr
	if replyErr == nil && progressErr != nil {
		s.recordAgentChannelCallback(binding, "progress_card_failed", "", message, progressErr.Error())
	}
	if replyErr != nil {
		s.recordAgentChannelCallback(binding, "reply_failed", "", message, replyErr.Error())
	} else if err != nil {
		s.recordAgentChannelCallback(binding, "run_failed", "", message, err.Error())
	} else {
		s.recordAgentChannelCallback(binding, "replied", "", message, "")
	}
	if attentionID != "" {
		if err != nil || replyErr != nil {
			_ = s.controlDB.MarkAttentionSignalStatus(binding.WorkspaceID, attentionID, "seen")
		} else {
			_ = s.controlDB.MarkAttentionSignalStatus(binding.WorkspaceID, attentionID, "handled")
		}
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

func (s *Server) recordIMAttentionSignal(resolved resolvedChannelEventBinding, providerID string, message imbridge.IncomingMessage, text string) string {
	if s == nil || s.controlDB == nil {
		return ""
	}
	binding := resolved.Binding
	workerID := strings.TrimSpace(binding.AgentWorkerID)
	if workerID == "" {
		workerID = s.agentWorkerIDForProjectAgent(binding.WorkspaceID, binding.ProjectID, binding.AgentID)
	}
	if workerID == "" {
		return ""
	}
	source := imConversationSource(providerID, resolved.Identity.UserID, message.ChatID, message.ChatType)
	messageID := strings.TrimSpace(message.MessageID)
	if messageID == "" {
		messageID = strings.TrimSpace(message.ChatID) + ":" + strings.TrimSpace(message.SenderOpenID) + ":" + fmt.Sprintf("%x", sha256.Sum256([]byte(text)))
	}
	senderDisplayName := ""
	senderEmail := ""
	if s != nil && s.users != nil {
		if user := s.users.GetUser(resolved.Identity.UserID); user != nil {
			senderDisplayName = strings.TrimSpace(user.DisplayName)
			senderEmail = strings.TrimSpace(user.Email)
		}
	}
	dedupeKey := "im:" + strings.TrimSpace(providerID) + ":" + messageID
	refsRaw, _ := json.Marshal(map[string]any{
		"bindingId":   binding.ID,
		"project":     binding.ProjectID,
		"agent":       binding.AgentID,
		"chatId":      message.ChatID,
		"messageId":   message.MessageID,
		"chatType":    message.ChatType,
		"messageType": message.MessageType,
	})
	payloadRaw, _ := json.Marshal(map[string]any{
		"text":           text,
		"rawContent":     message.RawContent,
		"mentionCount":   len(message.Mentions),
		"senderOpenId":   message.SenderOpenID,
		"multigentUser":  resolved.Identity.UserID,
		"senderName":     senderDisplayName,
		"senderEmail":    senderEmail,
		"externalChatId": message.ChatID,
		"messageType":    message.MessageType,
		"attachments":    message.Attachments,
	})
	reason := imAttentionReason(message)
	now := time.Now().UTC().Format(time.RFC3339)
	signalID := newChannelID("asig")
	if err := s.controlDB.UpsertAttentionSignal(controldb.AttentionSignal{
		ID:            signalID,
		WorkspaceID:   binding.WorkspaceID,
		AgentWorkerID: workerID,
		DedupeKey:     dedupeKey,
		SourceKind:    "im_message",
		SourceID:      messageID,
		SourceChannel: source.Channel,
		Reason:        reason,
		Priority:      "high",
		ActorType:     "user",
		ActorID:       resolved.Identity.UserID,
		Summary:       trimForIM(text, 240),
		RefsJSON:      string(refsRaw),
		PayloadJSON:   string(payloadRaw),
		Status:        "pending",
		CreatedAt:     now,
		ExpiresAt:     time.Now().UTC().Add(7 * 24 * time.Hour).Format(time.RFC3339),
	}); err != nil {
		log.Printf("[im:%s] record attention failed for %s/%s message=%s: %v", providerID, binding.ProjectID, binding.AgentID, message.MessageID, err)
		return ""
	}
	_ = s.controlDB.UpsertAttentionCursor(controldb.AttentionCursor{
		ID:            newChannelID("acur"),
		WorkspaceID:   binding.WorkspaceID,
		AgentWorkerID: workerID,
		SourceKind:    "im_message",
		SourceChannel: source.Channel,
		Cursor:        messageID,
		SeenUntil:     messageID,
		UpdatedAt:     now,
	})
	return signalID
}

func (s *Server) recordIMInteractionAttentionSignal(resolved resolvedChannelEventBinding, providerID string, callback imbridge.IncomingInteractionCallback, request controldb.InteractionRequest, submissionJSON string) string {
	if s == nil || s.controlDB == nil {
		return ""
	}
	binding := resolved.Binding
	workerID := strings.TrimSpace(binding.AgentWorkerID)
	if workerID == "" {
		workerID = strings.TrimSpace(request.AgentWorkerID)
	}
	if workerID == "" {
		workerID = s.agentWorkerIDForProjectAgent(binding.WorkspaceID, binding.ProjectID, binding.AgentID)
	}
	if workerID == "" {
		return ""
	}
	source := imConversationSource(providerID, resolved.Identity.UserID, callback.ChatID, "")
	eventID := strings.TrimSpace(callback.MessageID)
	if eventID == "" {
		eventID = strings.TrimSpace(request.ID) + ":" + strings.TrimSpace(callback.ActionID)
	}
	dedupeKey := "im-card:" + strings.TrimSpace(providerID) + ":" + strings.TrimSpace(request.ID) + ":" + eventID + ":" + strings.TrimSpace(callback.ActionID)
	refsRaw, _ := json.Marshal(map[string]any{
		"bindingId":     binding.ID,
		"interactionId": request.ID,
		"project":       binding.ProjectID,
		"agent":         binding.AgentID,
		"chatId":        callback.ChatID,
		"messageId":     callback.MessageID,
	})
	payloadRaw, _ := json.Marshal(map[string]any{
		"submission":    rawJSONToMap(submissionJSON),
		"senderOpenId":  callback.SenderOpenID,
		"multigentUser": resolved.Identity.UserID,
		"actionId":      callback.ActionID,
		"actionLabel":   callback.ActionLabel,
		"interactionId": request.ID,
	})
	summary := strings.TrimSpace(callback.ActionLabel)
	if summary == "" {
		summary = strings.TrimSpace(callback.ActionID)
	}
	if summary == "" {
		summary = "card action"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	signalID := newChannelID("asig")
	if err := s.controlDB.UpsertAttentionSignal(controldb.AttentionSignal{
		ID:            signalID,
		WorkspaceID:   binding.WorkspaceID,
		AgentWorkerID: workerID,
		DedupeKey:     dedupeKey,
		SourceKind:    "im_card_action",
		SourceID:      request.ID,
		SourceChannel: source.Channel,
		Reason:        "card_action",
		Priority:      "high",
		ActorType:     "user",
		ActorID:       resolved.Identity.UserID,
		Summary:       trimForIM(summary, 240),
		RefsJSON:      string(refsRaw),
		PayloadJSON:   string(payloadRaw),
		Status:        "pending",
		CreatedAt:     now,
		ExpiresAt:     time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
	}); err != nil {
		log.Printf("[im:%s] record card attention failed for %s/%s interaction=%s: %v", providerID, binding.ProjectID, binding.AgentID, request.ID, err)
		return ""
	}
	_ = s.controlDB.UpsertAttentionCursor(controldb.AttentionCursor{
		ID:            newChannelID("acur"),
		WorkspaceID:   binding.WorkspaceID,
		AgentWorkerID: workerID,
		SourceKind:    "im_card_action",
		SourceChannel: source.Channel,
		Cursor:        eventID,
		SeenUntil:     eventID,
		UpdatedAt:     now,
	})
	return signalID
}

func imAttentionReason(message imbridge.IncomingMessage) string {
	if strings.TrimSpace(message.ChatType) != "" && strings.TrimSpace(message.ChatType) != "p2p" {
		return "im_mention"
	}
	return "im_direct_message"
}

func formatIMAgentPrompt(providerID string, binding controldb.AgentChannelBinding, identity controldb.ExternalIdentity, message imbridge.IncomingMessage, text string) string {
	return formatIMAgentPromptWithSender(providerID, binding, identity, "", message, text)
}

func formatIMAgentPromptWithSender(providerID string, binding controldb.AgentChannelBinding, identity controldb.ExternalIdentity, userLabel string, message imbridge.IncomingMessage, text string) string {
	var b strings.Builder
	b.WriteString("You received a message from a human through an external collaboration channel.\n")
	b.WriteString("Handle it as a direct conversation with the user, using Multigent tools when useful.\n\n")
	b.WriteString("Channel context:\n")
	b.WriteString("- Provider: " + strings.TrimSpace(providerID) + "\n")
	b.WriteString("- Agent: " + strings.TrimSpace(binding.ProjectID) + "/" + strings.TrimSpace(binding.AgentID) + "\n")
	if username := strings.TrimSpace(identity.UserID); username != "" {
		b.WriteString("- Multigent user: " + username + "\n")
	}
	if label := strings.TrimSpace(userLabel); label != "" && label != strings.TrimSpace(identity.UserID) {
		b.WriteString("- Sender identity: " + label + "\n")
	}
	if chatID := strings.TrimSpace(message.ChatID); chatID != "" {
		b.WriteString("- Chat ID: " + chatID + "\n")
	}
	if messageType := strings.TrimSpace(message.MessageType); messageType != "" {
		b.WriteString("- Message type: " + messageType + "\n")
	}
	if len(message.Attachments) > 0 {
		b.WriteString("\nAttachments:\n")
		for i, attachment := range message.Attachments {
			b.WriteString(fmt.Sprintf("%d. %s", i+1, strings.TrimSpace(attachment.Type)))
			if name := strings.TrimSpace(attachment.Name); name != "" {
				b.WriteString(" — " + name)
			}
			if id := strings.TrimSpace(attachment.ID); id != "" {
				b.WriteString(" (`" + id + "`)")
			}
			if size := attachment.Size; size > 0 {
				b.WriteString(fmt.Sprintf(" — %d bytes", size))
			}
			if url := strings.TrimSpace(attachment.URL); url != "" {
				b.WriteString("\n   URL: " + url)
			}
			if preview := strings.TrimSpace(attachment.Preview); preview != "" {
				b.WriteString("\n   Preview: " + preview)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("\nReply contract:\n")
	b.WriteString("- Always finish with a concise, human-facing final reply. Do not end silently after tool calls.\n")
	b.WriteString("- Reply in the same language as the user's message unless the user asks otherwise.\n")
	b.WriteString("- Prefer short Markdown: one conclusion first, then bullets for details or next steps.\n")
	b.WriteString("- For chat-like conversations, behave like a responsive coworker: you may first acknowledge with `mga notify react --to source --emoji EYES` or send a short `mga notify send --to source --body \"我先看下\"`, then continue working.\n")
	b.WriteString("- You may send several short source replies when that feels more natural than one long final block. Avoid spam; each message should move the conversation forward.\n")
	b.WriteString("- If you cannot complete the request, explain the blocker and the exact next action needed.\n")
	b.WriteString("- If you sent a separate notification/card, still return a brief summary here so the user sees a complete response.\n\n")
	b.WriteString("User message:\n")
	b.WriteString("```text\n")
	b.WriteString(strings.TrimSpace(text))
	b.WriteString("\n```\n")
	return b.String()
}

func (s *Server) imIdentityDisplayLabel(identity controldb.ExternalIdentity) string {
	userID := strings.TrimSpace(identity.UserID)
	if userID == "" {
		return ""
	}
	if s != nil && s.users != nil {
		if user := s.users.GetUser(userID); user != nil {
			return formatUserIdentityLabel(user.Username, user.DisplayName, user.Email)
		}
	}
	return userID
}

func formatUserIdentityLabel(username, displayName, email string) string {
	username = strings.TrimSpace(username)
	displayName = strings.TrimSpace(displayName)
	email = strings.TrimSpace(email)
	label := username
	if displayName != "" && displayName != username {
		if label != "" {
			label = displayName + " (" + label + ")"
		} else {
			label = displayName
		}
	}
	if email != "" {
		if label != "" {
			label += " <" + email + ">"
		} else {
			label = email
		}
	}
	return label
}

func incomingMessageFallbackText(message imbridge.IncomingMessage) string {
	if strings.TrimSpace(message.Text) != "" {
		return strings.TrimSpace(message.Text)
	}
	if len(message.Attachments) == 0 {
		return ""
	}
	parts := make([]string, 0, len(message.Attachments))
	for _, attachment := range message.Attachments {
		label := strings.TrimSpace(attachment.Type)
		if label == "" {
			label = "attachment"
		}
		if name := strings.TrimSpace(attachment.Name); name != "" {
			label += ":" + name
		} else if url := strings.TrimSpace(attachment.URL); url != "" {
			label += ":" + url
		} else if id := strings.TrimSpace(attachment.ID); id != "" {
			label += ":" + id
		}
		parts = append(parts, "["+label+"]")
	}
	return strings.Join(parts, " ")
}

func agentChannelReplySubject(agentID string) string {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return "Agent"
	}
	return agentID
}

func (s *Server) startIMProcessingIndicator(ctx context.Context, provider imbridge.Provider, resolved resolvedChannelEventBinding, message imbridge.IncomingMessage) func() {
	reactionProvider, ok := provider.(imbridge.ReactionProvider)
	if !ok {
		return func() {}
	}
	reactionID, err := reactionProvider.AddReaction(ctx, resolved.SecretValues, message, "OK")
	if err != nil || strings.TrimSpace(reactionID) == "" {
		return func() {}
	}
	return func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := reactionProvider.RemoveReaction(stopCtx, resolved.SecretValues, message, reactionID); err != nil {
			log.Printf("[im:%s] remove processing reaction failed: %v", provider.Info().ID, err)
		}
	}
}

type imProgressReporter struct {
	ctx        context.Context
	provider   imbridge.Provider
	resolved   resolvedChannelEventBinding
	message    imbridge.IncomingMessage
	title      string
	started    bool
	handle     any
	reasoning  []imbridge.ProgressCardEntry
	tools      []imbridge.ProgressCardEntry
	lastUpdate time.Time
	lastErr    error
}

func newIMProgressReporter(ctx context.Context, provider imbridge.Provider, resolved resolvedChannelEventBinding, message imbridge.IncomingMessage, title string) *imProgressReporter {
	return &imProgressReporter{ctx: ctx, provider: provider, resolved: resolved, message: message, title: title}
}

func (r *imProgressReporter) Started() bool {
	return r != nil && r.started
}

func (r *imProgressReporter) Observe(line string) {
	if r == nil {
		return
	}
	entries := parseIMProgressEntries(line)
	if len(entries) == 0 {
		return
	}
	for _, entry := range entries {
		switch strings.ToLower(strings.TrimSpace(entry.Kind)) {
		case "tool", "tool_use", "tool_result":
			r.tools = append(r.tools, entry)
		default:
			r.reasoning = append(r.reasoning, entry)
		}
	}
	r.update("running", "", false)
}

func (r *imProgressReporter) Finalize(state, final string) error {
	if r == nil {
		return nil
	}
	return r.update(state, final, true)
}

func (r *imProgressReporter) update(state, final string, force bool) error {
	progressProvider, ok := r.provider.(imbridge.ProgressCardReplyProvider)
	if !ok {
		return fmt.Errorf("provider %s does not support progress cards", r.provider.Info().ID)
	}
	now := time.Now()
	if !force && r.started && now.Sub(r.lastUpdate) < 1200*time.Millisecond {
		return r.lastErr
	}
	card := imbridge.ProgressCard{
		Title:     r.title,
		State:     state,
		Reasoning: r.reasoning,
		Tools:     r.tools,
		Final:     final,
	}
	if !r.started {
		handle, err := progressProvider.StartProgressCardReply(r.ctx, r.resolved.SecretValues, r.message, card)
		if err != nil {
			r.lastErr = err
			return err
		}
		r.handle = handle
		r.started = true
		r.lastUpdate = now
		return nil
	}
	err := progressProvider.UpdateProgressCardReply(r.ctx, r.resolved.SecretValues, r.handle, card)
	r.lastErr = err
	if err == nil {
		r.lastUpdate = now
	}
	return err
}

func parseIMProgressEntries(line string) []imbridge.ProgressCardEntry {
	line = strings.TrimSpace(line)
	if line == "" || !strings.HasPrefix(line, "{") {
		return nil
	}
	var raw map[string]any
	if json.Unmarshal([]byte(line), &raw) != nil {
		return nil
	}
	typ, _ := raw["type"].(string)
	switch typ {
	case "assistant":
		return parseAssistantProgress(raw)
	case "human", "user":
		return parseUserProgress(raw)
	case "tool_call":
		if tc, _ := raw["tool_call"].(map[string]any); tc != nil {
			name, _ := tc["name"].(string)
			if name == "" {
				name, _ = tc["tool_name"].(string)
			}
			return []imbridge.ProgressCardEntry{{Kind: "tool_use", Title: firstNonEmpty(name, "Tool call"), Content: compactJSON(tc)}}
		}
	case "item.started", "item.completed":
		if item, _ := raw["item"].(map[string]any); item != nil {
			itemType, _ := item["type"].(string)
			switch itemType {
			case "agent_message":
				text := stringValue(item["text"])
				if strings.TrimSpace(text) != "" {
					return []imbridge.ProgressCardEntry{{Kind: "thinking", Title: "Response", Content: trimForIM(text, 900)}}
				}
			case "tool_call":
				name, _ := item["name"].(string)
				return []imbridge.ProgressCardEntry{{Kind: "tool_use", Title: firstNonEmpty(name, "Tool call"), Content: compactJSON(item)}}
			case "command_execution":
				command := stringValue(item["command"])
				status := stringValue(item["status"])
				output := stringValue(item["aggregated_output"])
				if typ == "item.started" || status == "in_progress" {
					return []imbridge.ProgressCardEntry{{Kind: "tool_use", Title: "Shell", Content: trimForIM(command, 600), Status: "running"}}
				}
				exitCode := ""
				if v, ok := item["exit_code"].(float64); ok {
					exitCode = fmt.Sprintf("exit %.0f", v)
				}
				return []imbridge.ProgressCardEntry{{Kind: "tool_result", Title: "Shell result", Content: trimForIM(firstNonEmpty(output, exitCode, "completed"), 900), Status: codexToolStatus(item)}}
			case "reasoning", "agent_reasoning":
				text := stringValue(item["text"])
				if strings.TrimSpace(text) != "" {
					return []imbridge.ProgressCardEntry{{Kind: "thinking", Title: "Reasoning", Content: trimForIM(text, 900)}}
				}
			case "error":
				msg, _ := item["message"].(string)
				return []imbridge.ProgressCardEntry{{Kind: "error", Title: "Error", Content: msg, Status: "failed"}}
			default:
				if output := stringValue(item["output"]); strings.TrimSpace(output) != "" {
					return []imbridge.ProgressCardEntry{{Kind: "tool_result", Title: firstNonEmpty(itemType, "Tool result"), Content: trimForIM(output, 900)}}
				}
			}
		}
	case "content":
		if block, _ := raw["content_block"].(map[string]any); block != nil {
			return parseContentBlockProgress(block)
		}
	case "thinking":
		if text, _ := raw["text"].(string); text != "" {
			return []imbridge.ProgressCardEntry{{Kind: "thinking", Title: "Thinking", Content: text}}
		}
	case "error":
		if text := agentChatRawMessageText(jsonRaw(raw["message"])); text != "" {
			return []imbridge.ProgressCardEntry{{Kind: "error", Title: "Error", Content: text, Status: "failed"}}
		}
	}
	return nil
}

func codexToolStatus(item map[string]any) string {
	if v, ok := item["exit_code"].(float64); ok && v != 0 {
		return "failed"
	}
	status := strings.TrimSpace(stringValue(item["status"]))
	if status != "" {
		return status
	}
	return "completed"
}

func parseContentBlockProgress(block map[string]any) []imbridge.ProgressCardEntry {
	blockType, _ := block["type"].(string)
	switch blockType {
	case "text":
		text := stringValue(block["text"])
		if strings.TrimSpace(text) != "" {
			return []imbridge.ProgressCardEntry{{Kind: "thinking", Title: "Response", Content: trimForIM(text, 900)}}
		}
	case "thinking":
		text := stringValue(block["thinking"])
		if strings.TrimSpace(text) != "" {
			return []imbridge.ProgressCardEntry{{Kind: "thinking", Title: "Reasoning", Content: trimForIM(text, 900)}}
		}
	case "tool_use":
		name := stringValue(block["name"])
		return []imbridge.ProgressCardEntry{{Kind: "tool_use", Title: firstNonEmpty(name, "Tool use"), Content: compactJSON(block), Status: "running"}}
	case "tool_result":
		content := firstNonEmpty(stringValue(block["content"]), stringValue(block["output"]))
		status := "completed"
		if isErr, _ := block["is_error"].(bool); isErr {
			status = "failed"
		}
		return []imbridge.ProgressCardEntry{{Kind: "tool_result", Title: "Tool result", Content: trimForIM(content, 900), Status: status}}
	}
	return nil
}

func parseAssistantProgress(raw map[string]any) []imbridge.ProgressCardEntry {
	msg, _ := raw["message"].(map[string]any)
	content, _ := msg["content"].([]any)
	if len(content) == 0 {
		content, _ = raw["content"].([]any)
	}
	var out []imbridge.ProgressCardEntry
	for _, blockAny := range content {
		block, _ := blockAny.(map[string]any)
		if block == nil {
			continue
		}
		blockType, _ := block["type"].(string)
		switch blockType {
		case "text":
			text, _ := block["text"].(string)
			if strings.TrimSpace(text) != "" {
				out = append(out, imbridge.ProgressCardEntry{Kind: "thinking", Title: "Reasoning", Content: trimForIM(text, 600)})
			}
		case "thinking":
			text := stringValue(block["thinking"])
			if strings.TrimSpace(text) != "" {
				out = append(out, imbridge.ProgressCardEntry{Kind: "thinking", Title: "Reasoning", Content: trimForIM(text, 900)})
			}
		case "tool_use":
			name, _ := block["name"].(string)
			out = append(out, imbridge.ProgressCardEntry{Kind: "tool_use", Title: firstNonEmpty(name, "Tool use"), Content: compactJSON(block)})
		case "tool_result":
			content := firstNonEmpty(stringValue(block["content"]), stringValue(block["output"]))
			status := "completed"
			if isErr, _ := block["is_error"].(bool); isErr {
				status = "failed"
			}
			out = append(out, imbridge.ProgressCardEntry{Kind: "tool_result", Title: "Tool result", Content: trimForIM(content, 600), Status: status})
		}
	}
	return out
}

func parseUserProgress(raw map[string]any) []imbridge.ProgressCardEntry {
	msg, _ := raw["message"].(map[string]any)
	content, _ := msg["content"].([]any)
	if len(content) == 0 {
		content, _ = raw["content"].([]any)
	}
	var out []imbridge.ProgressCardEntry
	for _, blockAny := range content {
		block, _ := blockAny.(map[string]any)
		if block == nil {
			continue
		}
		blockType := stringValue(block["type"])
		if blockType != "tool_result" {
			continue
		}
		content := firstNonEmpty(stringValue(block["content"]), stringValue(block["output"]))
		if strings.TrimSpace(content) == "" {
			continue
		}
		status := "completed"
		if isErr, _ := block["is_error"].(bool); isErr {
			status = "failed"
		}
		out = append(out, imbridge.ProgressCardEntry{Kind: "tool_result", Title: "Tool result", Content: trimForIM(content, 900), Status: status})
	}
	return out
}

func compactJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return trimForIM(string(raw), 600)
}

func jsonRaw(v any) json.RawMessage {
	raw, _ := json.Marshal(v)
	return raw
}

func (s *Server) runAgentForInteractionCallback(provider imbridge.Provider, resolved resolvedChannelEventBinding, callback imbridge.IncomingInteractionCallback, request controldb.InteractionRequest, submissionJSON string, runtimeAPIURL string, attentionID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	binding := resolved.Binding
	providerID := provider.Info().ID
	if attentionID != "" {
		_ = s.controlDB.MarkAttentionSignalStatus(binding.WorkspaceID, attentionID, "seen")
		_ = s.controlDB.MarkAttentionSignalStatus(binding.WorkspaceID, attentionID, "handling")
	}
	source := imConversationSource(providerID, resolved.Identity.UserID, callback.ChatID, "")
	lease, err := s.acquireAgentInteractionLease(s.interactionAgentRef(binding.WorkspaceID, binding.ProjectID, binding.AgentID), source, "interactive_callback")
	if err != nil {
		log.Printf("[im:%s] acquire callback session failed for %s/%s: %v", providerID, binding.ProjectID, binding.AgentID, err)
		return
	}
	defer lease.Release()
	delegationToken, delegationExpiresAt := s.issueInteractionDelegationToken(request, resolved.Identity.UserID)
	prompt := formatInteractionCallbackPrompt(request, submissionJSON, delegationToken, delegationExpiresAt)
	logPrompt := formatInteractionCallbackPrompt(request, submissionJSON, "<redacted>", delegationExpiresAt)
	_ = s.createInteractionEvent(lease.session, "user", resolved.Identity.UserID, providerID, "interaction.callback", logPrompt, map[string]any{
		"interactionId": request.ID,
		"actionId":      callback.ActionID,
		"messageId":     callback.MessageID,
		"chatId":        callback.ChatID,
		"submission":    rawJSONToMap(submissionJSON),
		"delegation": map[string]any{
			"expiresAt": delegationExpiresAt,
			"redacted":  true,
		},
	})
	_ = s.createInteractionEvent(lease.session, "system", "", providerID, "run_started", "", map[string]any{
		"interactionId": request.ID,
	})
	output, detectedRuntimeSessionID, err := s.execAgentPrompt(ctx, binding.WorkspaceID, binding.ProjectID, binding.AgentID, prompt, lease.session.RuntimeSessionID, runtimeAPIURL)
	if detectedRuntimeSessionID != "" {
		lease.SetRuntimeSessionID(detectedRuntimeSessionID)
	}
	reply := extractAgentChatReply(output)
	if err != nil {
		lease.Fail(err.Error())
		if attentionID != "" {
			_ = s.controlDB.MarkAttentionSignalStatus(binding.WorkspaceID, attentionID, "seen")
		}
		_ = s.createInteractionEvent(lease.session, "system", "", providerID, "run_failed", output, map[string]any{
			"interactionId":    request.ID,
			"error":            err.Error(),
			"runtimeSessionId": detectedRuntimeSessionID,
		})
		return
	}
	if strings.TrimSpace(reply) == "" {
		reply = "已收到你的选择。"
	}
	if err := s.updateIMInteractionCardCompleted(ctx, provider, resolved, callback, request, reply); err != nil {
		log.Printf("[im:%s] interaction completion card update failed for %s/%s interaction=%s: %v", providerID, binding.ProjectID, binding.AgentID, request.ID, err)
		_ = s.replyToIMInteraction(ctx, provider, resolved, callback, trimForIM(reply, 3500))
	}
	_ = s.createInteractionEvent(lease.session, "agent", binding.ProjectID+"/"+binding.AgentID, providerID, "run_completed", reply, map[string]any{
		"interactionId":    request.ID,
		"runtimeSessionId": detectedRuntimeSessionID,
	})
	if attentionID != "" {
		_ = s.controlDB.MarkAttentionSignalStatus(binding.WorkspaceID, attentionID, "handled")
	}
}

func (s *Server) updateIMInteractionCardCompleted(ctx context.Context, provider imbridge.Provider, resolved resolvedChannelEventBinding, callback imbridge.IncomingInteractionCallback, request controldb.InteractionRequest, reply string) error {
	updater, ok := provider.(imbridge.InteractionCardUpdater)
	if !ok {
		return fmt.Errorf("provider %s does not support interaction card updates", provider.Info().ID)
	}
	action := strings.TrimSpace(callback.ActionLabel)
	if action == "" {
		action = strings.TrimSpace(callback.ActionID)
	}
	fields := []imbridge.InteractiveCardField{}
	if action != "" {
		fields = append(fields, imbridge.InteractiveCardField{Label: "选择", Value: action})
	}
	if comment := strings.TrimSpace(firstNonEmpty(callback.Inputs["comment"], callback.Inputs["comments"], callback.Inputs["reason"])); comment != "" {
		fields = append(fields, imbridge.InteractiveCardField{Label: "补充说明", Value: comment})
	}
	result := summarizeInteractionReplyForCard(reply)
	if result != "" {
		fields = append(fields, imbridge.InteractiveCardField{Label: "处理结果", Value: result})
	}
	return updater.UpdateInteractionCard(ctx, resolved.SecretValues, callback, imbridge.OutgoingMessage{Card: &imbridge.InteractiveCard{
		InteractionID: request.ID,
		Title:         "处理完成",
		Body:          "Agent 已完成这次卡片回调处理，结果已记录到 Multigent。",
		Fields:        fields,
	}})
}

func summarizeInteractionReplyForCard(reply string) string {
	text := strings.TrimSpace(reply)
	if text == "" {
		return ""
	}
	replacer := strings.NewReplacer("**", "", "`", "", "✅", "", "❌", "", "###", "", "##", "", "#", "")
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(replacer.Replace(line))
		line = strings.TrimLeft(line, "-*0123456789. \t")
		if line == "" {
			continue
		}
		runes := []rune(line)
		if len(runes) > 120 {
			line = string(runes[:120]) + "..."
		}
		return line
	}
	return ""
}

func (s *Server) issueInteractionDelegationToken(request controldb.InteractionRequest, userID string) (string, string) {
	now := time.Now().UTC()
	ttl := 30 * time.Minute
	if exp := strings.TrimSpace(request.ExpiresAt); exp != "" {
		if expiresAt, err := time.Parse(time.RFC3339, exp); err == nil {
			remaining := time.Until(expiresAt)
			if remaining > 0 && remaining < ttl {
				ttl = remaining
			}
		}
	}
	if ttl <= 0 {
		ttl = time.Minute
	}
	expiresAt := now.Add(ttl).Format(time.RFC3339)
	token := s.issueRuntimeDelegationToken(runtimeDelegationTokenPayload{
		WorkspaceID:   request.WorkspaceID,
		Project:       request.ProjectID,
		Agent:         request.AgentID,
		UserID:        strings.TrimSpace(userID),
		InteractionID: request.ID,
		Scopes:        []string{"act_as_user"},
	}, ttl)
	return token, expiresAt
}

func formatInteractionCallbackPrompt(request controldb.InteractionRequest, submissionJSON, delegationToken, delegationExpiresAt string) string {
	var b strings.Builder
	b.WriteString("A user responded to an interactive card you sent through a collaboration channel.\n")
	b.WriteString("Treat this callback like a structured user message. Decide the next step yourself.\n")
	b.WriteString("If this callback should control a workflow or another protected resource, use the appropriate mga command; Multigent will enforce permissions and audit the action.\n\n")
	if strings.TrimSpace(delegationToken) != "" {
		b.WriteString("Delegation token:\n")
		b.WriteString("The user has delegated short-lived authority to you for this interaction. Use this token only for actions that match the user's callback and intent.\n")
		b.WriteString("Set `MULTIGENT_DELEGATION_TOKEN` for mga commands or pass `--delegation-token` explicitly.\n")
		if strings.TrimSpace(delegationExpiresAt) != "" {
			b.WriteString("Expires at: " + strings.TrimSpace(delegationExpiresAt) + "\n")
		}
		b.WriteString("Token: " + strings.TrimSpace(delegationToken) + "\n\n")
	}
	if strings.EqualFold(strings.TrimSpace(request.HandlerType), "workflow_decision") {
		b.WriteString("This interaction is intended to submit a human workflow decision.\n")
		b.WriteString("If the interaction context contains a taskId, run `mga workflow current --task-id <taskId>` first, then submit the user's selected action with `mga workflow decision submit --interaction <interactionId> --task <taskId> --decision <actionId> --delegation-token <token>`.\n")
		b.WriteString("Map extra comments or form fields to `--comments` or `--output key=value` when useful. Do not invent a decision that the user did not choose.\n\n")
	} else {
		b.WriteString("If the interaction context contains a taskId and the current workflow step is a human review step, you may use `mga workflow decision submit --delegation-token <token>` to apply the user's selected action when that is clearly the card's purpose.\n\n")
	}
	b.WriteString("Interaction request:\n")
	b.WriteString("```json\n")
	raw, _ := json.MarshalIndent(map[string]any{
		"id":          request.ID,
		"title":       request.Title,
		"body":        request.Body,
		"recipient":   request.Recipient,
		"handlerType": request.HandlerType,
		"context":     rawJSONToMap(request.ContextJSON),
		"schema":      rawJSONToMap(request.SchemaJSON),
	}, "", "  ")
	b.Write(raw)
	b.WriteString("\n```\n\n")
	b.WriteString("User callback:\n")
	b.WriteString("```json\n")
	if json.Valid([]byte(submissionJSON)) {
		var decoded any
		_ = json.Unmarshal([]byte(submissionJSON), &decoded)
		raw, _ = json.MarshalIndent(decoded, "", "  ")
		b.Write(raw)
	} else {
		b.WriteString(submissionJSON)
	}
	b.WriteString("\n```\n")
	return b.String()
}

func (s *Server) replyToIMInteraction(ctx context.Context, provider imbridge.Provider, resolved resolvedChannelEventBinding, callback imbridge.IncomingInteractionCallback, reply string) error {
	target := imbridge.OutgoingTarget{ReceiveID: strings.TrimSpace(callback.ChatID), ReceiveIDType: "chat_id", ChatID: strings.TrimSpace(callback.ChatID)}
	if target.ReceiveID == "" {
		target = imbridge.OutgoingTarget{ReceiveID: resolved.Identity.ExternalUserID, ReceiveIDType: "open_id"}
	}
	return provider.SendText(ctx, resolved.SecretValues, target, reply)
}

func rawJSONToMap(raw string) map[string]any {
	var out map[string]any
	if json.Unmarshal([]byte(strings.TrimSpace(raw)), &out) != nil || out == nil {
		return map[string]any{}
	}
	return out
}

func (s *Server) replyToIMEvent(ctx context.Context, provider imbridge.Provider, resolved resolvedChannelEventBinding, message imbridge.IncomingMessage, reply string) error {
	binding := resolved.Binding
	if err := provider.ReplyText(ctx, resolved.SecretValues, message, reply); err != nil {
		log.Printf("[im:%s] reply failed for %s/%s: %v", provider.Info().ID, binding.ProjectID, binding.AgentID, err)
		return err
	}
	return nil
}

func (s *Server) replyMessageToIMEvent(ctx context.Context, provider imbridge.Provider, resolved resolvedChannelEventBinding, message imbridge.IncomingMessage, reply imbridge.OutgoingMessage) error {
	binding := resolved.Binding
	if richProvider, ok := provider.(imbridge.RichReplyProvider); ok {
		if err := richProvider.ReplyMessage(ctx, resolved.SecretValues, message, reply); err != nil {
			log.Printf("[im:%s] rich reply failed for %s/%s: %v", provider.Info().ID, binding.ProjectID, binding.AgentID, err)
			return err
		}
		return nil
	}
	text := reply.Text
	if strings.TrimSpace(text) == "" && reply.Card != nil {
		text = reply.Card.Body
	}
	if err := provider.ReplyText(ctx, resolved.SecretValues, message, text); err != nil {
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
		"at":           now,
		"status":       strings.TrimSpace(status),
		"reason":       strings.TrimSpace(reason),
		"messageId":    strings.TrimSpace(message.MessageID),
		"chatId":       strings.TrimSpace(message.ChatID),
		"chatType":     strings.TrimSpace(message.ChatType),
		"mentionCount": len(message.Mentions),
		"text":         truncateForMetadata(message.Text, 300),
		"rawContent":   truncateForMetadata(message.RawContent, 600),
		"error":        strings.TrimSpace(errorText),
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

func truncateForMetadata(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if maxRunes <= 0 || value == "" {
		return value
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-3]) + "..."
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

func (s *Server) execAgentPrompt(ctx context.Context, workspaceID, project, agent, prompt, sessionID, runtimeAPIURL string) (string, string, error) {
	args := []string{"--dir", s.root, "exec", "--project", project, "--agent", agent, "--prompt", prompt, "--no-save-session"}
	if strings.TrimSpace(sessionID) != "" {
		args = append(args, "--session", strings.TrimSpace(sessionID))
	} else {
		args = append(args, "--no-session")
	}
	cmd := exec.CommandContext(ctx, s.sched.binPath, args...)
	cmd.Dir = s.root
	s.configureAgentExecEnv(cmd, workspaceID, project, agent, runtimeAPIURL)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	output := out.String()
	return output, extractAgentChatSessionID(output), err
}

func (s *Server) execAgentPromptStream(ctx context.Context, workspaceID, project, agent, prompt, sessionID, runtimeAPIURL string, onLine func(string)) (string, string, error) {
	args := []string{"--dir", s.root, "exec", "--project", project, "--agent", agent, "--prompt", prompt, "--no-save-session"}
	if strings.TrimSpace(sessionID) != "" {
		args = append(args, "--session", strings.TrimSpace(sessionID))
	} else {
		args = append(args, "--no-session")
	}
	cmd := exec.CommandContext(ctx, s.sched.binPath, args...)
	cmd.Dir = s.root
	s.configureAgentExecEnv(cmd, workspaceID, project, agent, runtimeAPIURL)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", "", err
	}
	if err := cmd.Start(); err != nil {
		return "", "", err
	}
	lines := make(chan string, 64)
	var wg sync.WaitGroup
	scan := func(r io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, 256*1024), 2*1024*1024)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}
	wg.Add(2)
	go scan(stdout)
	go scan(stderr)
	go func() {
		wg.Wait()
		close(lines)
	}()
	var out bytes.Buffer
	for line := range lines {
		out.WriteString(line)
		out.WriteByte('\n')
		if onLine != nil {
			onLine(line)
		}
	}
	err = cmd.Wait()
	output := out.String()
	return output, extractAgentChatSessionID(output), err
}

func (s *Server) configureAgentExecEnv(cmd *exec.Cmd, workspaceID, project, agent, runtimeAPIURL string) {
	if cmd == nil {
		return
	}
	if strings.TrimSpace(runtimeAPIURL) == "" {
		runtimeAPIURL = "http://127.0.0.1"
	}
	runID := "im-" + time.Now().UTC().Format("20060102-150405")
	token := s.issueAgentRuntimeToken(runtimeAgentTokenPayload{
		WorkspaceID:  workspaceID,
		Project:      project,
		Agent:        agent,
		RunID:        runID,
		Capabilities: defaultRuntimeCapabilities(),
	}, 6*time.Hour)
	cmd.Env = append(os.Environ(),
		"MULTIGENT_API_URL="+strings.TrimRight(runtimeAPIURL, "/"),
		"MULTIGENT_AGENT_TOKEN="+token,
		"MULTIGENT_RUN_ID="+runID,
		"MULTIGENT_WORKSPACE_ID="+workspaceID,
	)
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
