package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/imbridge"
	"github.com/multigent/multigent/internal/store"
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

type runtimeNotifyReactionBody struct {
	To      string `json:"to"`
	Channel string `json:"channel"`
	Emoji   string `json:"emoji"`
	TaskID  string `json:"taskId"`
}

const maxRuntimeNotifyAttachmentBytes = 50 << 20

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

var (
	runtimeNotifyDocIDPattern            = regexp.MustCompile(`\b(?:doc-\d{8}-[a-zA-Z0-9_-]+|kb-doc-[a-zA-Z0-9_-]+)\b`)
	runtimeNotifyDocURLTailPattern       = regexp.MustCompile(`https?://[^\s<>"')\]]*/docs/$`)
	runtimeNotifyMarkdownLinkTailPattern = regexp.MustCompile(`\]\([^)\n]*$`)
)
var (
	runtimeNotifyMarkdownHeadingPattern    = regexp.MustCompile(`(?m)^\s{0,3}#{1,6}\s+\S`)
	runtimeNotifyMarkdownListPattern       = regexp.MustCompile(`(?m)^\s{0,3}(?:[-*+]\s+|\d+[.)]\s+)\S`)
	runtimeNotifyMarkdownBlockquotePattern = regexp.MustCompile(`(?m)^\s{0,3}>\s+\S`)
	runtimeNotifyMarkdownFencePattern      = regexp.MustCompile("(?m)^\\s{0,3}```")
	runtimeNotifyMarkdownLinkPattern       = regexp.MustCompile(`\[[^\]\n]{1,120}\]\([^) \n]+(?:\s+"[^"\n]*")?\)`)
)

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
	body.Body = normalizeRuntimeNotifyTextEscapes(body.Body)
	if body.Card != nil {
		body.Card.Body = normalizeRuntimeNotifyTextEscapes(body.Card.Body)
	}
	text := strings.TrimSpace(body.Body)
	if body.Card != nil && text == "" {
		text = strings.TrimSpace(body.Card.Body)
	}
	if text == "" {
		s.jsonError(w, http.StatusBadRequest, "body is required")
		return
	}
	docMessageFormat := body.MessageFormat
	if body.Card != nil {
		docMessageFormat = "markdown"
	}
	text = s.enrichRuntimeNotifyDocLinks(r, docMessageFormat, text)
	if body.Card != nil && strings.TrimSpace(body.Card.Body) != "" {
		body.Card.Body = text
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
		"messageFormat": normalizeRuntimeNotifyMessageFormat(body.MessageFormat, text),
	}
	if runtimeNotifyLooksLikeMarkdown(text) && normalizeRuntimeNotifyMessageFormat(body.MessageFormat, text) == "text" && strings.EqualFold(strings.TrimSpace(body.MessageFormat), "text") {
		result["formatHint"] = "body looks like markdown; use --message-format markdown or leave --message-format as auto for rendered IM output"
	}
	if !found {
		result["externalError"] = "no connected human collaboration channel for this agent"
		s.auditRuntimeNotify(r, principal, msg.ID, "", subject, false, result["externalError"].(string), runtimeNotifyAuditExtra(result))
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
		s.auditRuntimeNotify(r, principal, msg.ID, binding.Provider, subject, false, result["externalError"].(string), runtimeNotifyAuditExtra(result))
		_ = json.NewEncoder(w).Encode(result)
		return
	}
	channelProvider, ok := imbridge.LookupProvider(binding.Provider)
	if !ok {
		result["externalError"] = "unsupported IM provider: " + binding.Provider
		s.auditRuntimeNotify(r, principal, msg.ID, binding.Provider, subject, false, result["externalError"].(string), runtimeNotifyAuditExtra(result))
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
		s.auditRuntimeNotify(r, principal, msg.ID, binding.Provider, subject, false, result["externalError"].(string), runtimeNotifyAuditExtra(result))
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
	notifyMessage.MentionOpenID = target.MentionOpenID
	notifyMessage.Text = trimForIM(notifyMessage.Text, 3500)
	prepareRuntimeNotifyExternalMessage(&notifyMessage, target)
	if target.ReplyToMessageID != "" {
		if replyProvider, ok := channelProvider.(imbridge.RichReplyProvider); ok {
			err := replyProvider.ReplyMessage(ctx, secrets, imbridge.IncomingMessage{
				MessageID:    target.ReplyToMessageID,
				ChatID:       target.ChatID,
				ChatType:     target.ChatType,
				SenderOpenID: target.MentionOpenID,
			}, notifyMessage)
			if err == nil {
				now := time.Now().UTC().Format(time.RFC3339)
				binding.LastActivityAt = now
				binding.UpdatedAt = now
				_ = s.controlDB.UpsertAgentChannelBinding(binding)
				result["provider"] = binding.Provider
				result["channelId"] = binding.ID
				result["externalSent"] = true
				result["externalReply"] = true
				s.auditRuntimeNotify(r, principal, msg.ID, binding.Provider, subject, true, "", runtimeNotifyAuditExtra(result))
				_ = json.NewEncoder(w).Encode(result)
				return
			}
			result["externalReplyError"] = err.Error()
		}
	}
	if err := channelProvider.SendMessage(ctx, secrets, target, notifyMessage); err != nil {
		result["provider"] = binding.Provider
		result["channelId"] = binding.ID
		result["externalError"] = err.Error()
		s.auditRuntimeNotify(r, principal, msg.ID, binding.Provider, subject, false, err.Error(), runtimeNotifyAuditExtra(result))
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
	s.auditRuntimeNotify(r, principal, msg.ID, binding.Provider, subject, true, "", runtimeNotifyAuditExtra(result))
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Server) handleRuntimeNotifyReaction(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.runtimeRequireCapability(w, r, "message.use")
	if !ok {
		return
	}
	var body runtimeNotifyReactionBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	emoji := strings.TrimSpace(body.Emoji)
	if emoji == "" {
		emoji = "OK"
	}
	if strings.TrimSpace(body.To) == "" {
		body.To = "source"
	}
	recipient, err := s.resolveRuntimeNotifyRecipient(principal, body.To)
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	binding, found, err := s.selectRuntimeNotifyChannel(principal, body.Channel)
	if err != nil {
		s.serverError(w, err)
		return
	}
	result := map[string]any{
		"recipient":     recipient,
		"emoji":         emoji,
		"externalSent":  false,
		"externalError": "",
	}
	if !found {
		result["externalError"] = "no connected human collaboration channel for this agent"
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
		result["externalError"] = fmt.Sprintf("recipient %q is not bound for this %s agent channel", recipient, binding.Provider)
		_ = json.NewEncoder(w).Encode(result)
		return
	}
	if strings.TrimSpace(target.ReplyToMessageID) == "" {
		s.jsonError(w, http.StatusBadRequest, "reaction requires a source message; use --to source from an IM attention wakeup")
		return
	}
	channelProvider, ok := imbridge.LookupProvider(binding.Provider)
	if !ok {
		result["externalError"] = "unsupported IM provider: " + binding.Provider
		_ = json.NewEncoder(w).Encode(result)
		return
	}
	reactionProvider, ok := channelProvider.(imbridge.ReactionProvider)
	if !ok {
		result["provider"] = binding.Provider
		result["channelId"] = binding.ID
		result["externalError"] = "IM provider does not support reactions"
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
		_ = json.NewEncoder(w).Encode(result)
		return
	}
	secrets, err := openConnectionSecret(secret)
	if err != nil {
		s.serverError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	reactionID, err := reactionProvider.AddReaction(ctx, secrets, imbridge.IncomingMessage{
		MessageID:    target.ReplyToMessageID,
		ChatID:       target.ChatID,
		ChatType:     target.ChatType,
		SenderOpenID: target.MentionOpenID,
	}, emoji)
	if err != nil {
		result["provider"] = binding.Provider
		result["channelId"] = binding.ID
		result["externalError"] = err.Error()
		s.auditLog(auditLogInput{
			WorkspaceID:  principal.WorkspaceID,
			ActorType:    "agent",
			ActorID:      runtimeAgentAddress(principal),
			Action:       "runtime.notify.react",
			ResourceType: "message",
			ResourceID:   target.ReplyToMessageID,
			Summary:      fmt.Sprintf("Runtime agent reacted via %s", binding.Provider),
			After: map[string]any{
				"project":       principal.Project,
				"agent":         principal.Agent,
				"runId":         principal.RunID,
				"provider":      binding.Provider,
				"emoji":         emoji,
				"externalSent":  false,
				"externalError": err.Error(),
			},
			Request: r,
		})
		_ = json.NewEncoder(w).Encode(result)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	binding.LastActivityAt = now
	binding.UpdatedAt = now
	_ = s.controlDB.UpsertAgentChannelBinding(binding)
	result["provider"] = binding.Provider
	result["channelId"] = binding.ID
	result["reactionId"] = reactionID
	result["externalSent"] = true
	s.auditLog(auditLogInput{
		WorkspaceID:  principal.WorkspaceID,
		ActorType:    "agent",
		ActorID:      runtimeAgentAddress(principal),
		Action:       "runtime.notify.react",
		ResourceType: "message",
		ResourceID:   target.ReplyToMessageID,
		Summary:      fmt.Sprintf("Runtime agent reacted via %s", binding.Provider),
		After: map[string]any{
			"project":      principal.Project,
			"agent":        principal.Agent,
			"runId":        principal.RunID,
			"provider":     binding.Provider,
			"emoji":        emoji,
			"externalSent": true,
		},
		Request: r,
	})
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Server) handleRuntimeNotifyFile(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.runtimeRequireCapability(w, r, "message.use")
	if !ok {
		return
	}
	if err := r.ParseMultipartForm(maxRuntimeNotifyAttachmentBytes + (1 << 20)); err != nil {
		s.jsonError(w, http.StatusBadRequest, "parse multipart form: "+err.Error())
		return
	}
	to := strings.TrimSpace(r.FormValue("to"))
	if to == "" {
		to = "human"
	}
	channel := strings.TrimSpace(r.FormValue("channel"))
	caption := strings.TrimSpace(r.FormValue("caption"))
	taskID := strings.TrimSpace(r.FormValue("taskId"))
	kind := strings.TrimSpace(r.FormValue("kind"))
	fileName := strings.TrimSpace(r.FormValue("filename"))
	mimeType := strings.TrimSpace(r.FormValue("mime"))
	docID := strings.TrimSpace(r.FormValue("docId"))

	var data []byte
	var err error
	if docID != "" {
		data, fileName, mimeType, err = s.runtimeNotifyDocAttachment(docID, fileName, mimeType)
		if err != nil {
			s.jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		if caption == "" {
			caption = "相关文档：" + docID
		}
	} else {
		data, fileName, mimeType, err = runtimeNotifyUploadedAttachment(r, fileName, mimeType)
		if err != nil {
			s.jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if len(data) == 0 {
		s.jsonError(w, http.StatusBadRequest, "file is required")
		return
	}
	if len(data) > maxRuntimeNotifyAttachmentBytes {
		s.jsonError(w, http.StatusBadRequest, "file is too large; max 50 MiB")
		return
	}
	if fileName == "" {
		fileName = "attachment"
	}
	if mimeType == "" {
		mimeType = mime.TypeByExtension(filepath.Ext(fileName))
	}
	if kind == "" {
		kind = runtimeNotifyAttachmentKind(fileName, mimeType)
	}

	recipient, err := s.resolveRuntimeNotifyRecipient(principal, to)
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	subject := strings.TrimSpace(r.FormValue("subject"))
	if subject == "" {
		subject = "Attachment: " + fileName
	}
	msg := &entity.Message{
		ID:      entity.NewMessageID(),
		From:    runtimeAgentAddress(principal),
		To:      recipient,
		Subject: subject,
		Body:    runtimeNotifyAttachmentInboxBody(fileName, caption, docID),
		SentAt:  time.Now().UTC(),
	}
	if err := s.ts.SendMessage(msg); err != nil {
		s.serverError(w, err)
		return
	}

	binding, found, err := s.selectRuntimeNotifyChannel(principal, channel)
	if err != nil {
		s.serverError(w, err)
		return
	}
	result := map[string]any{
		"messageId":     msg.ID,
		"internalSent":  true,
		"externalSent":  false,
		"externalError": "",
		"messageFormat": kind,
		"fileName":      fileName,
		"mime":          mimeType,
		"size":          len(data),
	}
	if taskID != "" {
		result["taskId"] = taskID
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
	attachmentProvider, ok := channelProvider.(imbridge.AttachmentSender)
	if !ok {
		result["provider"] = binding.Provider
		result["channelId"] = binding.ID
		result["externalError"] = fmt.Sprintf("%s collaboration channel does not support file sending yet", binding.Provider)
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
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	attachment := imbridge.OutgoingAttachment{Kind: kind, FileName: fileName, MIME: mimeType, Data: data, Caption: caption}
	if target.ReplyToMessageID != "" {
		err = attachmentProvider.ReplyAttachment(ctx, secrets, imbridge.IncomingMessage{
			MessageID:    target.ReplyToMessageID,
			ChatID:       target.ChatID,
			ChatType:     target.ChatType,
			SenderOpenID: target.MentionOpenID,
		}, attachment)
	} else {
		err = attachmentProvider.SendAttachment(ctx, secrets, target, attachment)
	}
	if err != nil {
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
	if recipient == "source" {
		targetUserID = s.runtimeNotifySourceUserID(principal, binding)
	}
	if strings.HasPrefix(recipient, "chat:") {
		targetType = "chat"
		targetUserID = ""
		targetChatID = strings.TrimPrefix(recipient, "chat:")
	}
	title := firstNonEmpty(strings.TrimSpace(cardBody.Title), strings.TrimSpace(body.Subject), runtimeNotifyDefaultSubject(principal))
	cardText := firstNonEmpty(strings.TrimSpace(cardBody.Body), fallbackText)
	if err := s.controlDB.CreateInteractionRequest(controldb.InteractionRequest{
		ID:               interactionID,
		WorkspaceID:      principal.WorkspaceID,
		AgentWorkerID:    binding.AgentWorkerID,
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

func (s *Server) runtimeNotifySourceUserID(principal runtimeAgentPrincipal, binding controldb.AgentChannelBinding) string {
	source, ok := s.runtimeNotifySourceInfoForPrincipal(principal, binding.Provider)
	if !ok {
		return ""
	}
	if strings.TrimSpace(source.SenderOpenID) == "" {
		return strings.TrimSpace(source.ActorUserID)
	}
	identities, err := s.controlDB.ListUserChannelIdentities(controldb.UserChannelIdentityFilter{
		WorkspaceID:      principal.WorkspaceID,
		ChannelBindingID: binding.ID,
		Provider:         binding.Provider,
		ExternalUserID:   strings.TrimSpace(source.SenderOpenID),
	})
	if err != nil || len(identities) == 0 {
		return strings.TrimSpace(source.ActorUserID)
	}
	return strings.TrimSpace(identities[0].UserID)
}

func newRuntimeInteractionRequestID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("ir_%d", time.Now().UnixNano())
	}
	return "ir_" + hex.EncodeToString(b[:])
}

func runtimeNotifyUploadedAttachment(r *http.Request, fileName, mimeType string) ([]byte, string, string, error) {
	file, header, err := r.FormFile("file")
	if err != nil {
		return nil, "", "", fmt.Errorf("file is required")
	}
	defer file.Close()
	if header != nil {
		if fileName == "" {
			fileName = filepath.Base(header.Filename)
		}
		if mimeType == "" {
			mimeType = header.Header.Get("Content-Type")
		}
	}
	data, err := io.ReadAll(io.LimitReader(file, maxRuntimeNotifyAttachmentBytes+1))
	if err != nil {
		return nil, "", "", err
	}
	return data, fileName, mimeType, nil
}

func (s *Server) runtimeNotifyDocAttachment(docID, fileName, mimeType string) ([]byte, string, string, error) {
	ds := store.NewDocsStore(s.root)
	doc, err := ds.Get(docID)
	if err != nil {
		return nil, "", "", err
	}
	content, err := ds.ReadContent(doc.FilePath)
	if err != nil {
		return nil, "", "", err
	}
	if fileName == "" {
		ext := filepath.Ext(doc.FilePath)
		if ext == "" {
			ext = ".md"
		}
		fileName = slugForNotifyAttachment(firstNonEmpty(doc.Title, doc.ID)) + ext
	}
	if mimeType == "" {
		mimeType = mime.TypeByExtension(filepath.Ext(fileName))
	}
	return []byte(content), fileName, mimeType, nil
}

func runtimeNotifyAttachmentKind(fileName, mimeType string) string {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	name := strings.ToLower(strings.TrimSpace(fileName))
	if strings.HasPrefix(mimeType, "image/") ||
		strings.HasSuffix(name, ".png") || strings.HasSuffix(name, ".jpg") ||
		strings.HasSuffix(name, ".jpeg") || strings.HasSuffix(name, ".gif") ||
		strings.HasSuffix(name, ".webp") {
		return "image"
	}
	return "file"
}

func runtimeNotifyAttachmentInboxBody(fileName, caption, docID string) string {
	var b strings.Builder
	b.WriteString("Sent attachment: ")
	b.WriteString(fileName)
	if docID != "" {
		b.WriteString("\nDoc ID: ")
		b.WriteString(docID)
	}
	if caption != "" {
		b.WriteString("\n\n")
		b.WriteString(caption)
	}
	return b.String()
}

func slugForNotifyAttachment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "attachment"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		case r > 127:
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
		if b.Len() >= 80 {
			break
		}
	}
	out := strings.Trim(b.String(), ".-_")
	if out == "" {
		return "attachment"
	}
	return out
}

func (s *Server) runtimeAgentChannelBindings(principal runtimeAgentPrincipal) ([]controldb.AgentChannelBinding, error) {
	if workerID := s.runtimePrincipalAgentWorkerID(principal); strings.TrimSpace(workerID) != "" {
		return s.controlDB.ListAgentChannelBindings(controldb.AgentChannelBindingFilter{
			WorkspaceID:   principal.WorkspaceID,
			AgentWorkerID: workerID,
			Status:        "connected",
		})
	}
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
	if strings.EqualFold(raw, "source") {
		return "source", nil
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
	if recipient == "source" {
		return s.runtimeNotifySourceTarget(principal, binding)
	}
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

func (s *Server) runtimeNotifySourceTarget(principal runtimeAgentPrincipal, binding controldb.AgentChannelBinding) (imbridge.OutgoingTarget, bool, error) {
	source, ok := s.runtimeNotifySourceInfoForPrincipal(principal, binding.Provider)
	if !ok {
		return imbridge.OutgoingTarget{}, false, nil
	}
	return imbridge.OutgoingTarget{
		ReceiveID:        source.ChatID,
		ReceiveIDType:    "chat_id",
		ChatID:           source.ChatID,
		ChatType:         source.ChatType,
		ReplyToMessageID: source.MessageID,
		MentionOpenID:    source.SenderOpenID,
	}, true, nil
}

func (s *Server) runtimeNotifySourceInfoForPrincipal(principal runtimeAgentPrincipal, provider string) (runtimeNotifySourceInfo, bool) {
	runID := strings.TrimSpace(principal.RunID)
	if runID == "" {
		return runtimeNotifySourceInfo{}, false
	}
	taskID := ""
	if s.controlDB != nil {
		run, found, err := s.controlDB.RuntimeRunByID(principal.WorkspaceID, runID)
		if err != nil {
			return runtimeNotifySourceInfo{}, false
		}
		if found {
			taskID = strings.TrimSpace(run.TaskID)
		}
	}
	if taskID == "" {
		// Local scheduler/runner tokens use the task id itself as RunID and do not
		// create a runtime_runs row. Fall back to that id so --to source works in
		// both local and remote runtime execution modes.
		taskID = runID
	}
	task, err := s.ts.GetTask(principal.Project, principal.Agent, taskID)
	if err != nil || task == nil {
		return runtimeNotifySourceInfo{}, false
	}
	return runtimeNotifySourceFromPrompt(task.Prompt, provider)
}

func runtimeNotifySourceChatID(prompt, provider string) (string, bool) {
	source, ok := runtimeNotifySourceFromPrompt(prompt, provider)
	return source.ChatID, ok
}

type runtimeNotifySourceInfo struct {
	ChatID       string
	ChatType     string
	MessageID    string
	SenderOpenID string
	ActorUserID  string
}

func runtimeNotifySourceFromPrompt(prompt, provider string) (runtimeNotifySourceInfo, bool) {
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" {
		return runtimeNotifySourceInfo{}, false
	}
	lines := strings.Split(prompt, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "Source:") || !strings.Contains(line, "im:"+provider+":") {
			continue
		}
		info := runtimeNotifySourceInfo{ActorUserID: runtimeNotifySourceActorUserID(line, provider)}
		for j := i + 1; j < len(lines) && j <= i+12; j++ {
			refLine := strings.TrimSpace(lines[j])
			if !strings.HasPrefix(refLine, "Refs:") && !strings.HasPrefix(refLine, "Payload:") {
				continue
			}
			raw := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(refLine, "Refs:"), "Payload:"))
			raw = strings.Trim(raw, "`")
			var refs map[string]any
			if err := json.Unmarshal([]byte(raw), &refs); err != nil {
				continue
			}
			if info.ChatID == "" {
				if chatID, _ := refs["chatId"].(string); strings.TrimSpace(chatID) != "" {
					info.ChatID = strings.TrimSpace(chatID)
				}
			}
			if info.ChatID == "" {
				if chatID, _ := refs["externalChatId"].(string); strings.TrimSpace(chatID) != "" {
					info.ChatID = strings.TrimSpace(chatID)
				}
			}
			if chatID, _ := refs["chatId"].(string); strings.TrimSpace(chatID) != "" && info.ChatID == "" {
				info.ChatID = strings.TrimSpace(chatID)
			}
			if chatType, _ := refs["chatType"].(string); strings.TrimSpace(chatType) != "" && info.ChatType == "" {
				info.ChatType = strings.TrimSpace(chatType)
			}
			if messageID, _ := refs["messageId"].(string); strings.TrimSpace(messageID) != "" && info.MessageID == "" {
				info.MessageID = strings.TrimSpace(messageID)
			}
			if senderOpenID, _ := refs["senderOpenId"].(string); strings.TrimSpace(senderOpenID) != "" && info.SenderOpenID == "" {
				info.SenderOpenID = strings.TrimSpace(senderOpenID)
			}
		}
		if info.ChatID != "" {
			return info, true
		}
	}
	return runtimeNotifySourceInfo{}, false
}

func runtimeNotifySourceActorUserID(sourceLine, provider string) string {
	sourceLine = strings.TrimSpace(sourceLine)
	sourceLine = strings.TrimPrefix(sourceLine, "Source:")
	sourceLine = strings.TrimSpace(strings.Trim(sourceLine, "`"))
	marker := "im:" + strings.TrimSpace(strings.ToLower(provider)) + ":"
	idx := strings.Index(strings.ToLower(sourceLine), marker)
	if idx < 0 {
		return ""
	}
	parts := strings.Split(sourceLine[idx:], ":")
	for i := 0; i+1 < len(parts); i++ {
		if strings.EqualFold(strings.TrimSpace(parts[i]), "user") {
			return strings.TrimSpace(strings.Trim(parts[i+1], "`"))
		}
	}
	return ""
}

func formatRuntimeNotifyMessage(principal runtimeAgentPrincipal, body runtimeNotifyBody, subject, text string) imbridge.OutgoingMessage {
	subject = strings.TrimSpace(subject)
	format := normalizeRuntimeNotifyMessageFormat(body.MessageFormat, text)
	if format == "markdown" {
		return imbridge.OutgoingMessage{
			Format:  format,
			Subject: subject,
			Text:    strings.TrimSpace(text),
		}
	}
	return imbridge.OutgoingMessage{
		Format:  "text",
		Subject: subject,
		Text:    strings.TrimSpace(text),
	}
}

func runtimeNotifyDefaultSubject(principal runtimeAgentPrincipal) string {
	if agent := strings.TrimSpace(principal.Agent); agent != "" {
		return agent
	}
	if project := strings.TrimSpace(principal.Project); project != "" {
		return project
	}
	return "Agent"
}

func normalizeRuntimeNotifyMessageFormat(format, text string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "markdown", "md":
		return "markdown"
	case "text", "plain":
		if runtimeNotifyLooksLikeMarkdown(text) {
			return "markdown"
		}
		return "text"
	case "", "auto":
		if runtimeNotifyLooksLikeMarkdown(text) {
			return "markdown"
		}
		return "text"
	}
	return "text"
}

func runtimeNotifyLooksLikeMarkdown(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	if runtimeNotifyMarkdownHeadingPattern.MatchString(text) ||
		runtimeNotifyMarkdownFencePattern.MatchString(text) ||
		runtimeNotifyMarkdownLinkPattern.MatchString(text) ||
		runtimeNotifyMarkdownBlockquotePattern.MatchString(text) {
		return true
	}
	score := 0
	if runtimeNotifyMarkdownListPattern.MatchString(text) {
		score++
	}
	if strings.Contains(text, "**") || strings.Contains(text, "__") {
		score++
	}
	if strings.Contains(text, "`") {
		score++
	}
	if strings.Count(text, "\n") >= 2 {
		score++
	}
	return score >= 2
}

func normalizeRuntimeNotifyTextEscapes(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return text
	}
	if !strings.Contains(text, `\n`) && !strings.Contains(text, `\r`) && !strings.Contains(text, `\t`) {
		return text
	}
	replacer := strings.NewReplacer(
		`\r\n`, "\n",
		`\n`, "\n",
		`\r`, "\n",
		`\t`, "\t",
	)
	return strings.TrimSpace(replacer.Replace(text))
}

type runtimeNotifyDocLink struct {
	ID    string
	Title string
	URL   string
}

func (s *Server) enrichRuntimeNotifyDocLinks(r *http.Request, messageFormat, text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return text
	}
	ids := runtimeNotifyDocIDPattern.FindAllString(text, -1)
	if len(ids) == 0 {
		return text
	}
	seen := map[string]bool{}
	links := make([]runtimeNotifyDocLink, 0, len(ids))
	ds := store.NewDocsStore(s.root)
	base := strings.TrimRight(workflowWebBaseURL(r), "/")
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		webPath := docsWebPath(id)
		doc, err := ds.Get(id)
		if err != nil || doc == nil {
			continue
		}
		title := strings.TrimSpace(doc.Title)
		if title == "" {
			title = id
		}
		links = append(links, runtimeNotifyDocLink{ID: id, Title: title, URL: base + webPath})
	}
	text = normalizeRuntimeNotifyDocURLs(text, links)
	if len(links) == 0 {
		return text
	}
	if runtimeNotifyDocLinksShouldRenderMarkdown(messageFormat, text) {
		text = linkRuntimeNotifyBareDocIDs(text, links)
		var b strings.Builder
		b.WriteString(text)
		b.WriteString("\n\n相关文档：")
		for _, link := range links {
			b.WriteString("\n- [")
			b.WriteString(escapeMarkdownLinkText(link.Title))
			b.WriteString("](")
			b.WriteString(link.URL)
			b.WriteString(") (`")
			b.WriteString(link.ID)
			b.WriteString("`)")
		}
		return b.String()
	}
	var b strings.Builder
	b.WriteString(text)
	b.WriteString("\n\n相关文档：")
	for _, link := range links {
		b.WriteString("\n- ")
		b.WriteString(link.Title)
		b.WriteString(": ")
		b.WriteString(link.URL)
		b.WriteString(" (")
		b.WriteString(link.ID)
		b.WriteString(")")
	}
	return b.String()
}

func runtimeNotifyDocLinksShouldRenderMarkdown(messageFormat, text string) bool {
	switch strings.ToLower(strings.TrimSpace(messageFormat)) {
	case "", "auto", "markdown", "md":
		return true
	case "text", "plain":
		return runtimeNotifyLooksLikeMarkdown(text)
	default:
		return runtimeNotifyLooksLikeMarkdown(text)
	}
}

func normalizeRuntimeNotifyDocURLs(text string, links []runtimeNotifyDocLink) string {
	for _, link := range links {
		if strings.TrimSpace(link.ID) == "" || strings.TrimSpace(link.URL) == "" {
			continue
		}
		pattern := regexp.MustCompile(`https?://[^\s<>"')\]]*/docs/` + regexp.QuoteMeta(link.ID))
		text = pattern.ReplaceAllString(text, link.URL)
	}
	return text
}

func linkRuntimeNotifyBareDocIDs(text string, links []runtimeNotifyDocLink) string {
	if strings.TrimSpace(text) == "" || len(links) == 0 {
		return text
	}
	byID := make(map[string]runtimeNotifyDocLink, len(links))
	for _, link := range links {
		if strings.TrimSpace(link.ID) != "" && strings.TrimSpace(link.URL) != "" {
			byID[link.ID] = link
		}
	}
	matches := runtimeNotifyDocIDPattern.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return text
	}
	var b strings.Builder
	last := 0
	for _, match := range matches {
		start, end := match[0], match[1]
		if start < last || end > len(text) {
			continue
		}
		id := text[start:end]
		b.WriteString(text[last:start])
		link, ok := byID[id]
		if !ok || runtimeNotifyDocIDLooksLinkedAt(text, start, end) {
			b.WriteString(id)
		} else {
			b.WriteString("[")
			b.WriteString(id)
			b.WriteString("](")
			b.WriteString(link.URL)
			b.WriteString(")")
		}
		last = end
	}
	b.WriteString(text[last:])
	return b.String()
}

func runtimeNotifyDocIDLooksLinkedAt(text string, start, end int) bool {
	if start < 0 || end > len(text) || start >= end {
		return false
	}
	before := text[:start]
	after := text[end:]
	tail := before
	if len(tail) > 160 {
		tail = tail[len(tail)-160:]
	}
	if runtimeNotifyDocURLTailPattern.MatchString(tail) {
		return true
	}
	if runtimeNotifyMarkdownLinkTailPattern.MatchString(tail) {
		return true
	}
	if strings.HasSuffix(tail, "`") && strings.HasPrefix(after, "`") {
		return true
	}
	return false
}

func escapeMarkdownLinkText(text string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `[`, `\[`, `]`, `\]`)
	return replacer.Replace(text)
}

func prepareRuntimeNotifyExternalMessage(message *imbridge.OutgoingMessage, target imbridge.OutgoingTarget) {
	if message == nil {
		return
	}
	if strings.TrimSpace(target.ReplyToMessageID) != "" {
		// IM source replies should feel like normal chat replies. Strip obvious
		// mail-style subjects, but keep agent-provided/default names as card
		// titles because some IM card formats require a header.
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(message.Subject)), "re:") {
			message.Subject = ""
		}
	}
}

func runtimeNotifyAuditExtra(result map[string]any) map[string]any {
	extra := map[string]any{}
	for _, key := range []string{"messageFormat", "formatHint", "channelId", "interactionId", "externalReply"} {
		if value, ok := result[key]; ok {
			extra[key] = value
		}
	}
	return extra
}

func (s *Server) auditRuntimeNotify(r *http.Request, principal runtimeAgentPrincipal, messageID, provider, subject string, externalSent bool, externalError string, extra ...map[string]any) {
	after := map[string]any{
		"project":       principal.Project,
		"agent":         principal.Agent,
		"runId":         principal.RunID,
		"provider":      provider,
		"subject":       subject,
		"externalSent":  externalSent,
		"externalError": externalError,
	}
	for _, values := range extra {
		for key, value := range values {
			after[key] = value
		}
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  principal.WorkspaceID,
		ActorType:    "agent",
		ActorID:      runtimeAgentAddress(principal),
		Action:       "runtime.notify.send",
		ResourceType: "message",
		ResourceID:   messageID,
		Summary:      fmt.Sprintf("Runtime agent notified human via %s", firstNonEmpty(provider, "internal")),
		After:        after,
		Request:      r,
	})
}
