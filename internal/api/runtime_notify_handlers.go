package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/imbridge"
)

type runtimeChannelRow struct {
	ID             string                    `json:"id"`
	Provider       string                    `json:"provider"`
	ConnectionID   string                    `json:"connectionId"`
	Status         string                    `json:"status"`
	CanNotify      bool                      `json:"canNotify"`
	OwnerBound     bool                      `json:"ownerBound"`
	ChatBound      bool                      `json:"chatBound"`
	Targets        []runtimeChannelTargetRow `json:"targets,omitempty"`
	LastActivityAt *time.Time                `json:"lastActivityAt,omitempty"`
}

type runtimeChannelTargetRow struct {
	ID             string `json:"id"`
	Type           string `json:"type"`
	Name           string `json:"name"`
	ExternalChatID string `json:"externalChatId,omitempty"`
}

type runtimeNotifyBody struct {
	To            string                 `json:"to"`
	Channel       string                 `json:"channel"`
	Subject       string                 `json:"subject"`
	Body          string                 `json:"body"`
	TaskID        string                 `json:"taskId"`
	Urgency       string                 `json:"urgency"`
	MessageFormat string                 `json:"messageFormat"`
	Card          *runtimeNotifyCardBody `json:"card,omitempty"`
	Context       map[string]any         `json:"context,omitempty"`
	ExpiresInSec  int                    `json:"expiresInSec,omitempty"`
}

type runtimeNotifyCardBody struct {
	Title       string                        `json:"title"`
	Body        string                        `json:"body"`
	Fields      []runtimeNotifyCardFieldBody  `json:"fields,omitempty"`
	Actions     []runtimeNotifyCardActionBody `json:"actions,omitempty"`
	Links       []runtimeNotifyCardLinkBody   `json:"links,omitempty"`
	HandlerType string                        `json:"handlerType,omitempty"`
	Context     map[string]any                `json:"context,omitempty"`
}

type runtimeNotifyCardFieldBody struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type runtimeNotifyCardActionBody struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Style        string `json:"style,omitempty"`
	RequiresText bool   `json:"requiresText,omitempty"`
}

type runtimeNotifyCardLinkBody struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

func (s *Server) handleRuntimeChannels(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.runtimeRequireCapability(w, r, "message.use")
	if !ok {
		return
	}
	bindings, err := s.runtimeAgentChannelBindings(principal)
	if err != nil {
		s.serverError(w, err)
		return
	}
	rows := make([]runtimeChannelRow, 0, len(bindings))
	for _, binding := range bindings {
		row, err := s.runtimeChannelToRow(principal, binding)
		if err != nil {
			s.serverError(w, err)
			return
		}
		rows = append(rows, row)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"project":  principal.Project,
		"agent":    principal.Agent,
		"channels": rows,
	})
}

func (s *Server) handleRuntimeNotify(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.runtimeRequireCapability(w, r, "message.use")
	if !ok {
		return
	}
	var body runtimeNotifyBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	text := strings.TrimSpace(body.Body)
	if body.Card != nil && text == "" {
		text = strings.TrimSpace(body.Card.Body)
	}
	if text == "" {
		s.jsonError(w, http.StatusBadRequest, "body is required")
		return
	}
	recipient, err := s.resolveRuntimeNotifyRecipient(principal, body.To)
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	subject := strings.TrimSpace(body.Subject)
	from := runtimeAgentAddress(principal)
	msg := &entity.Message{
		ID:      entity.NewMessageID(),
		From:    from,
		To:      recipient,
		Subject: subject,
		Body:    text,
		SentAt:  time.Now().UTC(),
	}
	if err := s.ts.SendMessage(msg); err != nil {
		s.serverError(w, err)
		return
	}

	binding, found, err := s.selectRuntimeNotifyChannel(principal, body.Channel)
	if err != nil {
		s.serverError(w, err)
		return
	}
	result := map[string]any{
		"messageId":     msg.ID,
		"internalSent":  true,
		"externalSent":  false,
		"externalError": "",
		"messageFormat": normalizeRuntimeNotifyMessageFormat(body.MessageFormat),
	}
	if !found {
		result["externalError"] = "no connected human collaboration channel for this agent"
		s.auditRuntimeNotify(r, principal, msg.ID, "", subject, false, result["externalError"].(string))
		_ = json.NewEncoder(w).Encode(result)
		return
	}
	target, targetOK, err := s.runtimeNotifyTargetForRecipient(principal, binding, recipient)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !targetOK {
		result["provider"] = binding.Provider
		result["channelId"] = binding.ID
		if strings.HasPrefix(recipient, "chat:") {
			result["externalError"] = fmt.Sprintf("chat target %q is not bound for this %s agent channel", strings.TrimPrefix(recipient, "chat:"), binding.Provider)
		} else {
			result["externalError"] = fmt.Sprintf("recipient %q has not bound a %s collaboration account for this agent channel", recipient, binding.Provider)
		}
		s.auditRuntimeNotify(r, principal, msg.ID, binding.Provider, subject, false, result["externalError"].(string))
		_ = json.NewEncoder(w).Encode(result)
		return
	}
	channelProvider, ok := imbridge.LookupProvider(binding.Provider)
	if !ok {
		result["externalError"] = "unsupported IM provider: " + binding.Provider
		s.auditRuntimeNotify(r, principal, msg.ID, binding.Provider, subject, false, result["externalError"].(string))
		_ = json.NewEncoder(w).Encode(result)
		return
	}
	secret, ok, err := s.controlDB.ConnectionSecret(binding.ConnectionID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !ok {
		result["externalError"] = "channel connection secret not found"
		s.auditRuntimeNotify(r, principal, msg.ID, binding.Provider, subject, false, result["externalError"].(string))
		_ = json.NewEncoder(w).Encode(result)
		return
	}
	secrets, err := openConnectionSecret(secret)
	if err != nil {
		s.serverError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	notifyMessage := formatRuntimeNotifyMessage(principal, body, subject, text)
	if body.Card != nil {
		card, interactionID, err := s.runtimeNotifyCreateInteractionRequest(principal, binding, recipient, body, text)
		if err != nil {
			s.jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		notifyMessage.Card = card
		result["interactionId"] = interactionID
		result["messageFormat"] = "card"
	}
	notifyMessage.Text = trimForIM(notifyMessage.Text, 3500)
	if err := channelProvider.SendMessage(ctx, secrets, target, notifyMessage); err != nil {
		result["provider"] = binding.Provider
		result["channelId"] = binding.ID
		result["externalError"] = err.Error()
		s.auditRuntimeNotify(r, principal, msg.ID, binding.Provider, subject, false, err.Error())
		_ = json.NewEncoder(w).Encode(result)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	binding.LastActivityAt = now
	binding.UpdatedAt = now
	_ = s.controlDB.UpsertAgentChannelBinding(binding)
	result["provider"] = binding.Provider
	result["channelId"] = binding.ID
	result["externalSent"] = true
	s.auditRuntimeNotify(r, principal, msg.ID, binding.Provider, subject, true, "")
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Server) runtimeNotifyCreateInteractionRequest(principal runtimeAgentPrincipal, binding controldb.AgentChannelBinding, recipient string, body runtimeNotifyBody, fallbackText string) (*imbridge.InteractiveCard, string, error) {
	cardBody := body.Card
	if cardBody == nil {
		return nil, "", fmt.Errorf("card is required")
	}
	actions := make([]imbridge.InteractiveCardAction, 0, len(cardBody.Actions))
	for _, action := range cardBody.Actions {
		id := strings.TrimSpace(action.ID)
		label := strings.TrimSpace(action.Label)
		if id == "" || label == "" {
			continue
		}
		actions = append(actions, imbridge.InteractiveCardAction{ID: id, Label: label, Style: action.Style, RequiresText: action.RequiresText})
	}
	if len(actions) == 0 {
		return nil, "", fmt.Errorf("card.actions must include at least one action")
	}
	fields := make([]imbridge.InteractiveCardField, 0, len(cardBody.Fields))
	for _, field := range cardBody.Fields {
		if strings.TrimSpace(field.Label) == "" && strings.TrimSpace(field.Value) == "" {
			continue
		}
		fields = append(fields, imbridge.InteractiveCardField{Label: strings.TrimSpace(field.Label), Value: strings.TrimSpace(field.Value)})
	}
	links := make([]imbridge.InteractiveCardLink, 0, len(cardBody.Links))
	for _, link := range cardBody.Links {
		if strings.TrimSpace(link.Label) == "" || strings.TrimSpace(link.URL) == "" {
			continue
		}
		links = append(links, imbridge.InteractiveCardLink{Label: strings.TrimSpace(link.Label), URL: strings.TrimSpace(link.URL)})
	}
	interactionID := newRuntimeInteractionRequestID()
	contextMap := map[string]any{}
	for k, v := range body.Context {
		contextMap[k] = v
	}
	for k, v := range cardBody.Context {
		contextMap[k] = v
	}
	if taskID := strings.TrimSpace(body.TaskID); taskID != "" {
		contextMap["taskId"] = taskID
	}
	schemaRaw, _ := json.Marshal(map[string]any{"actions": cardBody.Actions, "fields": cardBody.Fields, "links": cardBody.Links})
	contextRaw, _ := json.Marshal(contextMap)
	expiresIn := time.Duration(body.ExpiresInSec) * time.Second
	if expiresIn <= 0 {
		expiresIn = time.Hour
	}
	if expiresIn > 24*time.Hour {
		expiresIn = 24 * time.Hour
	}
	now := time.Now().UTC()
	targetType := "user"
	targetUserID := recipient
	targetChatID := ""
	if recipient == "human" {
		targetUserID = ""
	}
	if strings.HasPrefix(recipient, "chat:") {
		targetType = "chat"
		targetUserID = ""
		targetChatID = strings.TrimPrefix(recipient, "chat:")
	}
	title := firstNonEmpty(strings.TrimSpace(cardBody.Title), strings.TrimSpace(body.Subject), "Multigent")
	cardText := firstNonEmpty(strings.TrimSpace(cardBody.Body), fallbackText)
	if err := s.controlDB.CreateInteractionRequest(controldb.InteractionRequest{
		ID:               interactionID,
		WorkspaceID:      principal.WorkspaceID,
		ProjectID:        principal.Project,
		AgentID:          principal.Agent,
		ChannelBindingID: binding.ID,
		Provider:         binding.Provider,
		Recipient:        recipient,
		TargetType:       targetType,
		TargetUserID:     targetUserID,
		TargetChatID:     targetChatID,
		Title:            title,
		Body:             cardText,
		SchemaJSON:       string(schemaRaw),
		ContextJSON:      string(contextRaw),
		HandlerType:      firstNonEmpty(strings.TrimSpace(cardBody.HandlerType), "agent_event"),
		Status:           "active",
		CreatedBy:        runtimeAgentAddress(principal),
		CreatedAt:        now.Format(time.RFC3339),
		ExpiresAt:        now.Add(expiresIn).Format(time.RFC3339),
	}); err != nil {
		return nil, "", err
	}
	return &imbridge.InteractiveCard{
		InteractionID: interactionID,
		Title:         title,
		Body:          cardText,
		Fields:        fields,
		Actions:       actions,
		Links:         links,
		Context:       contextMap,
	}, interactionID, nil
}

func newRuntimeInteractionRequestID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("ir_%d", time.Now().UnixNano())
	}
	return "ir_" + hex.EncodeToString(b[:])
}

func (s *Server) runtimeAgentChannelBindings(principal runtimeAgentPrincipal) ([]controldb.AgentChannelBinding, error) {
	return s.controlDB.ListAgentChannelBindings(controldb.AgentChannelBindingFilter{
		WorkspaceID: principal.WorkspaceID,
		ProjectID:   principal.Project,
		AgentID:     principal.Agent,
		Status:      "connected",
	})
}

func (s *Server) selectRuntimeNotifyChannel(principal runtimeAgentPrincipal, requested string) (controldb.AgentChannelBinding, bool, error) {
	bindings, err := s.runtimeAgentChannelBindings(principal)
	if err != nil {
		return controldb.AgentChannelBinding{}, false, err
	}
	requested = strings.ToLower(strings.TrimSpace(requested))
	if requested == "" {
		requested = "auto"
	}
	for _, binding := range bindings {
		if requested == "auto" || requested == strings.ToLower(strings.TrimSpace(binding.Provider)) {
			return binding, true, nil
		}
	}
	return controldb.AgentChannelBinding{}, false, nil
}

func (s *Server) runtimeChannelToRow(principal runtimeAgentPrincipal, binding controldb.AgentChannelBinding) (runtimeChannelRow, error) {
	row := runtimeChannelRow{
		ID:           binding.ID,
		Provider:     binding.Provider,
		ConnectionID: binding.ConnectionID,
		Status:       binding.Status,
		OwnerBound:   strings.TrimSpace(binding.ExternalOwnerID) != "",
		ChatBound:    strings.TrimSpace(binding.ExternalChatID) != "",
	}
	targets, err := s.controlDB.ListAgentChannelTargets(controldb.AgentChannelTargetFilter{
		WorkspaceID:      principal.WorkspaceID,
		ChannelBindingID: binding.ID,
		Provider:         binding.Provider,
	})
	if err != nil {
		return runtimeChannelRow{}, err
	}
	row.Targets = make([]runtimeChannelTargetRow, 0, len(targets))
	for _, target := range targets {
		if target.TargetType != "chat" {
			continue
		}
		row.Targets = append(row.Targets, runtimeChannelTargetRow{
			ID:             target.ID,
			Type:           target.TargetType,
			Name:           target.DisplayName,
			ExternalChatID: target.ExternalChatID,
		})
	}
	row.CanNotify = runtimeChannelCanNotify(binding) || len(row.Targets) > 0
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(binding.LastActivityAt)); err == nil {
		row.LastActivityAt = &t
	}
	return row, nil
}

func runtimeChannelCanNotify(binding controldb.AgentChannelBinding) bool {
	return strings.TrimSpace(binding.ExternalOwnerID) != "" || strings.TrimSpace(binding.ExternalChatID) != ""
}

func (s *Server) resolveRuntimeNotifyRecipient(principal runtimeAgentPrincipal, input string) (string, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		raw = "human"
	}
	if strings.EqualFold(raw, "owner") || strings.EqualFold(raw, "human") {
		return "human", nil
	}
	if strings.HasPrefix(strings.ToLower(raw), "chat:") || strings.HasPrefix(strings.ToLower(raw), "group:") {
		prefix, value, _ := strings.Cut(raw, ":")
		value = strings.TrimSpace(value)
		if value == "" {
			return "", fmt.Errorf("%s recipient requires a chat name or chat id", strings.ToLower(prefix))
		}
		return "chat:" + value, nil
	}
	raw = strings.TrimSpace(strings.TrimPrefix(raw, "user:"))
	recipient, err := s.resolveRuntimeRecipient(principal, raw)
	if err != nil {
		return "", err
	}
	if strings.Contains(recipient, "/") {
		return "", fmt.Errorf("notify recipient must be a workspace user, got agent %q", recipient)
	}
	return recipient, nil
}

func (s *Server) runtimeNotifyTargetForRecipient(principal runtimeAgentPrincipal, binding controldb.AgentChannelBinding, recipient string) (imbridge.OutgoingTarget, bool, error) {
	if recipient == "human" {
		if owner := strings.TrimSpace(binding.ExternalOwnerID); owner != "" {
			return imbridge.OutgoingTarget{ReceiveID: owner, ReceiveIDType: "open_id", ChatID: strings.TrimSpace(binding.ExternalChatID)}, true, nil
		}
		if chatID := strings.TrimSpace(binding.ExternalChatID); chatID != "" {
			return imbridge.OutgoingTarget{ReceiveID: chatID, ReceiveIDType: "chat_id", ChatID: chatID}, true, nil
		}
		return imbridge.OutgoingTarget{}, false, nil
	}
	if chatName, ok := strings.CutPrefix(recipient, "chat:"); ok {
		targets, err := s.controlDB.ListAgentChannelTargets(controldb.AgentChannelTargetFilter{
			WorkspaceID:      principal.WorkspaceID,
			ChannelBindingID: binding.ID,
			Provider:         binding.Provider,
			TargetType:       "chat",
		})
		if err != nil {
			return imbridge.OutgoingTarget{}, false, err
		}
		chatName = strings.TrimSpace(chatName)
		for _, target := range targets {
			if strings.EqualFold(strings.TrimSpace(target.DisplayName), chatName) || strings.TrimSpace(target.ExternalChatID) == chatName {
				chatID := strings.TrimSpace(target.ExternalChatID)
				if chatID == "" {
					continue
				}
				return imbridge.OutgoingTarget{ReceiveID: chatID, ReceiveIDType: "chat_id", ChatID: chatID}, true, nil
			}
		}
		return imbridge.OutgoingTarget{}, false, nil
	}
	identities, err := s.controlDB.ListUserChannelIdentities(controldb.UserChannelIdentityFilter{
		WorkspaceID:      principal.WorkspaceID,
		UserID:           recipient,
		ChannelBindingID: binding.ID,
		Provider:         binding.Provider,
	})
	if err != nil {
		return imbridge.OutgoingTarget{}, false, err
	}
	if len(identities) == 0 {
		return imbridge.OutgoingTarget{}, false, nil
	}
	identity := identities[0]
	if externalUserID := strings.TrimSpace(identity.ExternalUserID); externalUserID != "" {
		return imbridge.OutgoingTarget{ReceiveID: externalUserID, ReceiveIDType: "open_id", ChatID: strings.TrimSpace(identity.ExternalChatID)}, true, nil
	}
	if chatID := strings.TrimSpace(identity.ExternalChatID); chatID != "" {
		return imbridge.OutgoingTarget{ReceiveID: chatID, ReceiveIDType: "chat_id", ChatID: chatID}, true, nil
	}
	return imbridge.OutgoingTarget{}, false, nil
}

func formatRuntimeNotifyMessage(principal runtimeAgentPrincipal, body runtimeNotifyBody, subject, text string) imbridge.OutgoingMessage {
	format := normalizeRuntimeNotifyMessageFormat(body.MessageFormat)
	if format == "markdown" {
		return imbridge.OutgoingMessage{
			Format:  format,
			Subject: subject,
			Text:    formatRuntimeNotifyMarkdown(principal, body, text),
		}
	}
	return imbridge.OutgoingMessage{
		Format:  "text",
		Subject: subject,
		Text:    formatRuntimeNotifyText(principal, body, subject, text),
	}
}

func normalizeRuntimeNotifyMessageFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "markdown", "md":
		return "markdown"
	default:
		return "text"
	}
}

func formatRuntimeNotifyText(principal runtimeAgentPrincipal, body runtimeNotifyBody, subject, text string) string {
	parts := make([]string, 0, 5)
	if subject != "" {
		parts = append(parts, subject)
	}
	parts = append(parts, text)
	meta := []string{"From: " + runtimeAgentAddress(principal)}
	if taskID := strings.TrimSpace(body.TaskID); taskID != "" {
		meta = append(meta, "Task: "+taskID)
	}
	if urgency := strings.TrimSpace(body.Urgency); urgency != "" {
		meta = append(meta, "Urgency: "+urgency)
	}
	parts = append(parts, "\n"+strings.Join(meta, " · "))
	return strings.Join(parts, "\n\n")
}

func formatRuntimeNotifyMarkdown(principal runtimeAgentPrincipal, body runtimeNotifyBody, text string) string {
	parts := []string{strings.TrimSpace(text)}
	meta := []string{"From: `" + runtimeAgentAddress(principal) + "`"}
	if taskID := strings.TrimSpace(body.TaskID); taskID != "" {
		meta = append(meta, "Task: `"+taskID+"`")
	}
	if urgency := strings.TrimSpace(body.Urgency); urgency != "" {
		meta = append(meta, "Urgency: `"+urgency+"`")
	}
	parts = append(parts, "---\n_"+strings.Join(meta, " · ")+"_")
	return strings.Join(parts, "\n\n")
}

func (s *Server) auditRuntimeNotify(r *http.Request, principal runtimeAgentPrincipal, messageID, provider, subject string, externalSent bool, externalError string) {
	s.auditLog(auditLogInput{
		WorkspaceID:  principal.WorkspaceID,
		ActorType:    "agent",
		ActorID:      runtimeAgentAddress(principal),
		Action:       "runtime.notify.send",
		ResourceType: "message",
		ResourceID:   messageID,
		Summary:      fmt.Sprintf("Runtime agent notified human via %s", firstNonEmpty(provider, "internal")),
		After: map[string]any{
			"project":       principal.Project,
			"agent":         principal.Agent,
			"runId":         principal.RunID,
			"provider":      provider,
			"subject":       subject,
			"externalSent":  externalSent,
			"externalError": externalError,
		},
		Request: r,
	})
}
