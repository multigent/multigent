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

func (s *Server) handleIMEvent(w http.ResponseWriter, r *http.Request) {
	channelProvider, ok := imbridge.LookupProvider(r.PathValue("provider"))
	if !ok {
		s.jsonError(w, http.StatusBadRequest, "unsupported IM provider")
		return
	}
	provider := channelProvider.Info().ID
	raw, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, "read event body failed")
		return
	}
	if encryptedPayload, encrypted := channelProvider.ExtractEncryptedPayload(raw); encrypted {
		workspaceID, err := s.currentWorkspaceID()
		if err != nil {
			s.serverError(w, err)
			return
		}
		decrypted, ok, err := s.decryptIMEvent(workspaceID, channelProvider, encryptedPayload)
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
	message := parsed.Message
	text := strings.TrimSpace(message.Text)
	if strings.TrimSpace(text) == "" {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ignored": true})
		return
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	resolved, found, err := s.resolveChannelEventBinding(workspaceID, provider, parsed.AppID, message.ChatID, message.SenderOpenID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ignored": true, "reason": "binding_not_found"})
		return
	}
	if !channelProvider.ShouldHandleMessage(resolved.Binding.ExternalChatID, message) {
		s.auditLog(auditLogInput{
			WorkspaceID:  workspaceID,
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
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ignored": true, "reason": "group_not_addressed"})
		return
	}
	if !verifyIMEventToken(parsed.VerificationToken, resolved.SecretValues) {
		s.auditLog(auditLogInput{
			WorkspaceID:  workspaceID,
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
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ignored": true, "reason": "verification_failed"})
		return
	}
	if !s.userCanOperateAgent(resolved.Identity.UserID, resolved.Binding.ProjectID, resolved.Binding.AgentID) {
		s.auditLog(auditLogInput{
			WorkspaceID:  workspaceID,
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
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ignored": true, "reason": "permission_denied"})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})

	go s.runAgentForIMEvent(channelProvider, resolved, message, text)
}

func (s *Server) decryptIMEvent(workspaceID string, provider imbridge.Provider, encryptedPayload string) ([]byte, bool, error) {
	providerID := provider.Info().ID
	bindings, err := s.controlDB.ListAgentChannelBindings(controldb.AgentChannelBindingFilter{
		WorkspaceID: workspaceID,
		Provider:    providerID,
		Status:      "connected",
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

func (s *Server) resolveChannelEventBinding(workspaceID, provider, appID, chatID, externalUserID string) (resolvedChannelEventBinding, bool, error) {
	identity, ok, err := s.controlDB.ExternalIdentityByExternalID(workspaceID, provider, strings.TrimSpace(externalUserID))
	if err != nil {
		return resolvedChannelEventBinding{}, false, err
	}
	if !ok {
		return resolvedChannelEventBinding{}, false, nil
	}
	bindings, err := s.controlDB.ListAgentChannelBindings(controldb.AgentChannelBindingFilter{
		WorkspaceID: workspaceID,
		Provider:    provider,
		Status:      "connected",
	})
	if err != nil {
		return resolvedChannelEventBinding{}, false, err
	}
	for _, binding := range bindings {
		if binding.ExternalChatID != "" && chatID != "" && binding.ExternalChatID != chatID {
			continue
		}
		var meta struct {
			AppID string `json:"appId"`
		}
		_ = json.Unmarshal([]byte(binding.MetadataJSON), &meta)
		if strings.TrimSpace(appID) != "" && strings.TrimSpace(meta.AppID) != "" && strings.TrimSpace(appID) != strings.TrimSpace(meta.AppID) {
			continue
		}
		secret, ok, err := s.controlDB.ConnectionSecret(binding.ConnectionID)
		if err != nil {
			return resolvedChannelEventBinding{}, false, err
		}
		if !ok {
			continue
		}
		values, err := openConnectionSecret(secret)
		if err != nil {
			return resolvedChannelEventBinding{}, false, err
		}
		return resolvedChannelEventBinding{Binding: binding, SecretValues: values, Identity: identity}, true, nil
	}
	return resolvedChannelEventBinding{}, false, nil
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
	runtimeSessionID := ""
	if hb, hbErr := s.ts.GetHeartbeat(binding.ProjectID, binding.AgentID); hbErr == nil && hb != nil {
		runtimeSessionID = strings.TrimSpace(hb.SessionID)
	}
	output, detectedRuntimeSessionID, err := s.execAgentPrompt(ctx, binding.ProjectID, binding.AgentID, text, runtimeSessionID)
	if detectedRuntimeSessionID != "" {
		lease.SetRuntimeSessionID(detectedRuntimeSessionID)
		if hb, hbErr := s.ts.GetHeartbeat(binding.ProjectID, binding.AgentID); hbErr == nil && hb != nil {
			hb.SessionID = detectedRuntimeSessionID
			if hb.SessionStartedAt == nil {
				now := time.Now().UTC()
				hb.SessionStartedAt = &now
			}
			_ = s.ts.SaveHeartbeat(binding.ProjectID, binding.AgentID, hb)
		}
	}
	reply := strings.TrimSpace(output)
	if err != nil {
		lease.Fail(err.Error())
		reply = "Agent run failed: " + err.Error()
		if output != "" {
			reply += "\n\n" + output
		}
	}
	reply = trimForIM(reply, 3500)
	replyErr := s.replyToIMEvent(ctx, provider, resolved, message, reply)
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

func (s *Server) userCanOperateAgent(username, project, agent string) bool {
	username = strings.TrimSpace(username)
	if username == "" {
		return false
	}
	req, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		return false
	}
	req = req.WithContext(context.WithValue(req.Context(), ctxUserKey, username))
	return s.canOperateAgent(req, project, agent)
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
	args := []string{"--dir", s.root, "exec", "--project", project, "--agent", agent, "--prompt", prompt}
	if strings.TrimSpace(sessionID) != "" {
		args = append(args, "--session", strings.TrimSpace(sessionID))
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
