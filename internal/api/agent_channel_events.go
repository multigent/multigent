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
	larkbridge "github.com/multigent/multigent/internal/imbridge/lark"
	"github.com/multigent/multigent/internal/interaction"
)

type resolvedChannelEventBinding struct {
	Binding      controldb.AgentChannelBinding
	SecretValues map[string]string
	Identity     controldb.ExternalIdentity
}

func (s *Server) handleIMEvent(w http.ResponseWriter, r *http.Request) {
	provider, ok := larkbridge.NormalizeProvider(r.PathValue("provider"))
	if !ok {
		s.jsonError(w, http.StatusBadRequest, "unsupported IM provider")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, "read event body failed")
		return
	}
	var env larkbridge.EventEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid event JSON")
		return
	}
	if larkbridge.IsURLVerification(env) {
		_ = json.NewEncoder(w).Encode(map[string]string{"challenge": env.Challenge})
		return
	}
	event, isMessage, err := larkbridge.ParseMessageEvent(env)
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid message event")
		return
	}
	if !isMessage {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ignored": true})
		return
	}
	text := larkbridge.ExtractText(event.Message)
	if strings.TrimSpace(text) == "" {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ignored": true})
		return
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	resolved, found, err := s.resolveChannelEventBinding(workspaceID, provider, env.Header.AppID, event.Message.ChatID, event.Sender.SenderID.OpenID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ignored": true, "reason": "binding_not_found"})
		return
	}
	if !verifyLarkFamilyEventToken(env, resolved.SecretValues) {
		s.auditLog(auditLogInput{
			WorkspaceID:  workspaceID,
			Action:       "agent_channel.verification_failed",
			ResourceType: "agent_channel",
			ResourceID:   resolved.Binding.ID,
			Summary:      fmt.Sprintf("Rejected %s event for %s/%s because verification token did not match", provider, resolved.Binding.ProjectID, resolved.Binding.AgentID),
			After: map[string]any{
				"provider":  provider,
				"messageId": event.Message.MessageID,
				"chatId":    event.Message.ChatID,
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
				"externalUserId": event.Sender.SenderID.OpenID,
				"messageId":      event.Message.MessageID,
			},
		})
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ignored": true, "reason": "permission_denied"})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})

	go s.runAgentForIMEvent(provider, resolved, event, text)
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

func (s *Server) runAgentForIMEvent(provider string, resolved resolvedChannelEventBinding, event larkbridge.MessageEvent, text string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	binding := resolved.Binding
	source := interaction.Source{
		Kind:    provider,
		ActorID: resolved.Identity.UserID,
		Channel: event.Message.ChatID,
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
				Summary:      fmt.Sprintf("Ignored %s message for %s/%s because the agent is busy", provider, binding.ProjectID, binding.AgentID),
				After: map[string]any{
					"provider":  provider,
					"messageId": event.Message.MessageID,
					"chatId":    event.Message.ChatID,
				},
			})
			s.replyToLarkFamilyEvent(ctx, provider, resolved, event, "Agent is currently busy in another session. Please wait for the current run to finish, or stop it from Multigent and try again.")
			return
		}
		log.Printf("[im:%s] acquire session failed for %s/%s: %v", provider, binding.ProjectID, binding.AgentID, err)
		return
	}
	defer lease.Release()
	_ = s.createInteractionEvent(lease.session, "user", resolved.Identity.UserID, provider, "message", text, map[string]any{
		"messageId": event.Message.MessageID,
		"chatId":    event.Message.ChatID,
	})
	if binding.ExternalChatID == "" && event.Message.ChatID != "" {
		binding.ExternalChatID = event.Message.ChatID
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
		Summary:      fmt.Sprintf("Received %s message for %s/%s", provider, binding.ProjectID, binding.AgentID),
		After: map[string]any{
			"provider":  provider,
			"messageId": event.Message.MessageID,
			"chatId":    event.Message.ChatID,
		},
	})
	_ = s.createInteractionEvent(lease.session, "system", "", provider, "run_started", "", map[string]any{
		"messageId": event.Message.MessageID,
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
	replyErr := s.replyToLarkFamilyEvent(ctx, provider, resolved, event, reply)
	if err != nil {
		_ = s.createInteractionEvent(lease.session, "system", "", provider, "run_failed", output, map[string]any{
			"messageId":        event.Message.MessageID,
			"error":            err.Error(),
			"runtimeSessionId": detectedRuntimeSessionID,
		})
	} else {
		_ = s.createInteractionEvent(lease.session, "agent", binding.ProjectID+"/"+binding.AgentID, provider, "run_completed", reply, map[string]any{
			"messageId":        event.Message.MessageID,
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
		Summary:      fmt.Sprintf("Replied to %s message for %s/%s", provider, binding.ProjectID, binding.AgentID),
		After: map[string]any{
			"provider":  provider,
			"messageId": event.Message.MessageID,
			"error":     errString(replyErr),
		},
	})
}

func (s *Server) replyToLarkFamilyEvent(ctx context.Context, provider string, resolved resolvedChannelEventBinding, event larkbridge.MessageEvent, reply string) error {
	binding := resolved.Binding
	client := larkbridge.OpenAPIClient{
		BaseURL:   resolved.SecretValues["baseUrl"],
		AppID:     resolved.SecretValues["appId"],
		AppSecret: resolved.SecretValues["appSecret"],
	}
	if err := client.ReplyText(ctx, event.Message.MessageID, reply); err != nil {
		log.Printf("[im:%s] reply failed for %s/%s: %v", provider, binding.ProjectID, binding.AgentID, err)
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

func verifyLarkFamilyEventToken(env larkbridge.EventEnvelope, values map[string]string) bool {
	expected := strings.TrimSpace(values["verificationToken"])
	if expected == "" {
		return true
	}
	return subtleConstantTimeEqual(strings.TrimSpace(env.Token), expected)
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
