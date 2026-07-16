package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	larkbridge "github.com/multigent/multigent/internal/imbridge/lark"
)

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
	binding, secretValues, found, err := s.resolveChannelEventBinding(workspaceID, provider, env.Header.AppID, event.Message.ChatID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ignored": true, "reason": "binding_not_found"})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})

	go s.runAgentForIMEvent(provider, binding, secretValues, event, text)
}

func (s *Server) resolveChannelEventBinding(workspaceID, provider, appID, chatID string) (controldb.AgentChannelBinding, map[string]string, bool, error) {
	bindings, err := s.controlDB.ListAgentChannelBindings(controldb.AgentChannelBindingFilter{
		WorkspaceID: workspaceID,
		Provider:    provider,
		Status:      "connected",
	})
	if err != nil {
		return controldb.AgentChannelBinding{}, nil, false, err
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
			return controldb.AgentChannelBinding{}, nil, false, err
		}
		if !ok {
			continue
		}
		values, err := openConnectionSecret(secret)
		if err != nil {
			return controldb.AgentChannelBinding{}, nil, false, err
		}
		return binding, values, true, nil
	}
	return controldb.AgentChannelBinding{}, nil, false, nil
}

func (s *Server) runAgentForIMEvent(provider string, binding controldb.AgentChannelBinding, secretValues map[string]string, event larkbridge.MessageEvent, text string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if binding.ExternalChatID == "" && event.Message.ChatID != "" {
		binding.ExternalChatID = event.Message.ChatID
		binding.LastActivityAt = time.Now().UTC().Format(time.RFC3339)
		binding.UpdatedAt = binding.LastActivityAt
		_ = s.controlDB.UpsertAgentChannelBinding(binding)
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  binding.WorkspaceID,
		ActorType:    "external_user",
		ActorID:      firstNonEmptyString(event.Sender.SenderID.OpenID, event.Sender.SenderID.UserID, "unknown"),
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
	output, err := s.execAgentPrompt(ctx, binding.ProjectID, binding.AgentID, text)
	reply := strings.TrimSpace(output)
	if err != nil {
		reply = "Agent run failed: " + err.Error()
		if output != "" {
			reply += "\n\n" + output
		}
	}
	reply = trimForIM(reply, 3500)
	client := larkbridge.OpenAPIClient{
		BaseURL:   secretValues["baseUrl"],
		AppID:     secretValues["appId"],
		AppSecret: secretValues["appSecret"],
	}
	if err := client.ReplyText(ctx, event.Message.MessageID, reply); err != nil {
		log.Printf("[im:%s] reply failed for %s/%s: %v", provider, binding.ProjectID, binding.AgentID, err)
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
			"error":     errString(err),
		},
	})
}

func (s *Server) execAgentPrompt(ctx context.Context, project, agent, prompt string) (string, error) {
	args := []string{"--dir", s.root, "exec", "--project", project, "--agent", agent, "--prompt", prompt}
	cmd := exec.CommandContext(ctx, s.sched.binPath, args...)
	cmd.Dir = s.root
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

func trimForIM(s string, max int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max]) + "\n\n...(truncated)"
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
