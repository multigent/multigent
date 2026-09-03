package lark

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const MessageReceiveEvent = "im.message.receive_v1"
const CardActionEvent = "card.action.trigger"

type EventEnvelope struct {
	Challenge string          `json:"challenge"`
	Token     string          `json:"token"`
	Type      string          `json:"type"`
	Schema    string          `json:"schema"`
	Header    EventHeader     `json:"header"`
	Event     json.RawMessage `json:"event"`
}

type EventHeader struct {
	EventType string `json:"event_type"`
	AppID     string `json:"app_id"`
	TenantKey string `json:"tenant_key"`
}

type MessageEvent struct {
	Sender  MessageSender `json:"sender"`
	Message EventMessage  `json:"message"`
}

type MessageSender struct {
	SenderID MessageSenderID `json:"sender_id"`
}

type MessageSenderID struct {
	OpenID  string `json:"open_id"`
	UserID  string `json:"user_id"`
	UnionID string `json:"union_id"`
}

type EventMessage struct {
	MessageID   string            `json:"message_id"`
	RootID      string            `json:"root_id"`
	ParentID    string            `json:"parent_id"`
	ChatID      string            `json:"chat_id"`
	ChatType    string            `json:"chat_type"`
	MessageType string            `json:"message_type"`
	Content     string            `json:"content"`
	Mentions    []json.RawMessage `json:"mentions"`
}

type MessageAttachment struct {
	ID      string
	Type    string
	Name    string
	URL     string
	MIME    string
	Size    int64
	Preview string
	Raw     map[string]any
}

type CardActionCallback struct {
	InteractionID  string
	MessageID      string
	ChatID         string
	OperatorOpenID string
	UpdateToken    string
	ActionID       string
	ActionLabel    string
	Inputs         map[string]string
	Raw            json.RawMessage
}

func IsURLVerification(env EventEnvelope) bool {
	return env.Challenge != "" && (env.Type == "url_verification" || env.Header.EventType == "url_verification")
}

func ParseMessageEvent(env EventEnvelope) (MessageEvent, bool, error) {
	if env.Header.EventType != MessageReceiveEvent {
		return MessageEvent{}, false, nil
	}
	var event MessageEvent
	if err := json.Unmarshal(env.Event, &event); err != nil {
		return MessageEvent{}, false, err
	}
	return event, true, nil
}

func ParseCardActionEvent(env EventEnvelope) (CardActionCallback, bool, error) {
	if env.Header.EventType != CardActionEvent {
		return CardActionCallback{}, false, nil
	}
	var raw map[string]any
	if err := json.Unmarshal(env.Event, &raw); err != nil {
		return CardActionCallback{}, true, err
	}
	callback := CardActionCallback{Raw: env.Event, Inputs: map[string]string{}}
	callback.UpdateToken, _ = raw["token"].(string)
	if operator, _ := raw["operator"].(map[string]any); operator != nil {
		if id, _ := operator["operator_id"].(map[string]any); id != nil {
			callback.OperatorOpenID, _ = id["open_id"].(string)
		}
	}
	if context, _ := raw["context"].(map[string]any); context != nil {
		callback.MessageID, _ = context["open_message_id"].(string)
		callback.ChatID, _ = context["open_chat_id"].(string)
	}
	if action, _ := raw["action"].(map[string]any); action != nil {
		if value, _ := action["value"].(map[string]any); value != nil {
			callback.InteractionID, _ = value["interaction_id"].(string)
			callback.ActionID, _ = value["action_id"].(string)
			callback.ActionLabel, _ = value["action_label"].(string)
			callback.ChatID, _ = value["chat_id"].(string)
		}
		if form, _ := action["form_value"].(map[string]any); form != nil {
			for key, value := range form {
				if key = strings.TrimSpace(key); key == "" {
					continue
				}
				switch v := value.(type) {
				case string:
					callback.Inputs[key] = strings.TrimSpace(v)
				default:
					raw, _ := json.Marshal(v)
					callback.Inputs[key] = strings.TrimSpace(string(raw))
				}
			}
		}
	}
	return callback, true, nil
}

func ExtractText(message EventMessage) string {
	var body map[string]any
	if json.Unmarshal([]byte(message.Content), &body) != nil {
		return strings.TrimSpace(message.Content)
	}
	messageType := strings.ToLower(strings.TrimSpace(message.MessageType))
	if messageType != "text" && messageType != "post" {
		// Interactive cards and forwarded messages use nested Card 2.0 JSON
		// instead of the text/post envelope. Extract only user-facing fields so
		// the message remains useful to the agent without exposing every
		// implementation field as prose.
		return extractStructuredMessageText(body)
	}
	if text, _ := body["text"].(string); text != "" {
		return strings.TrimSpace(text)
	}
	if title, _ := body["title"].(string); title != "" {
		return strings.TrimSpace(title)
	}
	if message.MessageType == "post" {
		return strings.TrimSpace(extractPostPlainText(body))
	}
	return ""
}

func ExtractAttachments(message EventMessage) []MessageAttachment {
	messageType := strings.TrimSpace(strings.ToLower(message.MessageType))
	var body map[string]any
	_ = json.Unmarshal([]byte(message.Content), &body)
	out := make([]MessageAttachment, 0, 2)
	switch messageType {
	case "image":
		if id := firstMapString(body, "image_key", "imageKey", "file_key", "fileKey"); id != "" {
			out = append(out, MessageAttachment{ID: id, Type: "image", Raw: body})
		}
	case "file":
		if id := firstMapString(body, "file_key", "fileKey"); id != "" {
			out = append(out, MessageAttachment{
				ID:   id,
				Type: "file",
				Name: firstMapString(body, "file_name", "fileName", "name"),
				MIME: firstMapString(body, "mime_type", "mimeType"),
				Size: firstMapInt64(body, "file_size", "fileSize", "size"),
				Raw:  body,
			})
		}
	case "media", "audio":
		if id := firstMapString(body, "file_key", "fileKey"); id != "" {
			out = append(out, MessageAttachment{ID: id, Type: messageType, Name: firstMapString(body, "file_name", "fileName", "name"), Raw: body})
		}
	case "text", "post":
		for _, link := range extractLinksFromMessageBody(body) {
			out = append(out, link)
		}
	case "interactive", "merge_forward", "share_chat", "share_user":
		if body != nil {
			// Interactive cards can contain real image/file elements. Keep the
			// card marker only when no downloadable resource was found; otherwise
			// expose the concrete resources so the runtime can download them.
			if nested := extractNestedResourceAttachments(body); len(nested) > 0 {
				out = append(out, nested...)
			}
			out = append(out, extractLinksFromMessageBody(body)...)
			if len(out) > 0 {
				break
			}
			name := firstMapString(body, "title", "name")
			if name == "" {
				name = messageType
			}
			out = append(out, MessageAttachment{
				Type: messageType,
				Name: name,
				ID:   firstMapString(body, "message_id", "messageId", "chat_id", "chatId"),
				Raw:  body,
			})
		}
	default:
		if urlValue := firstMapString(body, "url", "href"); urlValue != "" {
			out = append(out, MessageAttachment{
				Type: classifyLarkURL(urlValue),
				Name: firstMapString(body, "title", "name"),
				URL:  urlValue,
				Raw:  body,
			})
		}
	}
	return dedupeAttachments(out)
}

var structuredMessageTextKeys = map[string]bool{
	"text":        true,
	"content":     true,
	"markdown":    true,
	"plain_text":  true,
	"title":       true,
	"description": true,
	"label":       true,
}

func extractStructuredMessageText(body map[string]any) string {
	if body == nil {
		return ""
	}
	parts := make([]string, 0, 8)
	seen := map[string]bool{}
	var walk func(string, any)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		parts = append(parts, value)
	}
	walk = func(key string, value any) {
		switch v := value.(type) {
		case string:
			if structuredMessageTextKeys[strings.ToLower(strings.TrimSpace(key))] {
				var nested map[string]any
				if strings.HasPrefix(strings.TrimSpace(v), "{") && json.Unmarshal([]byte(v), &nested) == nil {
					walk(key, nested)
					return
				}
				add(v)
			}
		case []any:
			for _, item := range v {
				walk(key, item)
			}
		case map[string]any:
			for childKey, child := range v {
				walk(childKey, child)
			}
		}
	}
	walk("", body)
	return strings.Join(parts, "\n")
}

var markdownURLPattern = regexp.MustCompile(`https?://[^\s<>"')\]]+`)

func extractLinksFromMessageBody(body map[string]any) []MessageAttachment {
	if body == nil {
		return nil
	}
	out := make([]MessageAttachment, 0, 2)
	addURL := func(rawURL, label string) {
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			return
		}
		out = append(out, MessageAttachment{Type: classifyLarkURL(rawURL), Name: strings.TrimSpace(label), URL: rawURL})
	}
	if text, _ := body["text"].(string); text != "" {
		for _, match := range markdownURLPattern.FindAllString(text, -1) {
			addURL(match, "")
		}
	}
	if href, _ := body["href"].(string); href != "" {
		addURL(href, firstMapString(body, "text", "title"))
	}
	collectPostLinks(body["content"], addURL)
	collectPostLinks(body["elements"], addURL)
	for _, key := range []string{"user_dsl", "card", "data"} {
		if encoded, _ := body[key].(string); strings.TrimSpace(encoded) == "" {
			continue
		} else if len(encoded) <= 1<<20 {
			var decoded any
			if json.Unmarshal([]byte(encoded), &decoded) == nil {
				collectPostLinks(decoded, addURL)
			}
		}
	}
	return out
}

func collectPostLinks(value any, addURL func(string, string)) {
	switch v := value.(type) {
	case string:
		for _, match := range markdownURLPattern.FindAllString(v, -1) {
			addURL(match, "")
		}
	case []any:
		for _, item := range v {
			collectPostLinks(item, addURL)
		}
	case map[string]any:
		if href, _ := v["href"].(string); href != "" {
			addURL(href, firstMapString(v, "text", "title"))
		}
		if text, _ := v["text"].(string); text != "" {
			for _, match := range markdownURLPattern.FindAllString(text, -1) {
				addURL(match, "")
			}
		}
		for _, key := range []string{"content", "elements", "text"} {
			if child, ok := v[key]; ok {
				collectPostLinks(child, addURL)
			}
		}
	}
}

func extractPostPlainText(body map[string]any) string {
	var parts []string
	var walk func(any)
	walk = func(value any) {
		switch v := value.(type) {
		case []any:
			for _, item := range v {
				walk(item)
			}
		case map[string]any:
			if text, _ := v["text"].(string); strings.TrimSpace(text) != "" {
				parts = append(parts, strings.TrimSpace(text))
			}
			if title, _ := v["title"].(string); strings.TrimSpace(title) != "" {
				parts = append(parts, strings.TrimSpace(title))
			}
			for _, key := range []string{"content", "elements"} {
				if child, ok := v[key]; ok {
					walk(child)
				}
			}
		}
	}
	walk(body["content"])
	return strings.Join(parts, " ")
}

// ExpandMergeForwardItems converts the platform representation of a
// merge_forward message into the same provider-neutral fields used by normal
// messages. Resource IDs intentionally keep the outer message ID in the
// IncomingMessage; Lark requires that container ID when downloading a child
// file or image.
func ExpandMergeForwardItems(items []map[string]any, outerMessageID string) (string, []MessageAttachment, error) {
	var textParts []string
	var attachments []MessageAttachment
	for _, item := range items {
		messageType := strings.ToLower(firstMapString(item, "msg_type", "message_type", "messageType"))
		if messageType == "" {
			messageType = "text"
		}
		body, _ := item["body"].(map[string]any)
		content := firstMapString(body, "content")
		if content == "" {
			content = firstMapString(item, "content")
		}
		child := EventMessage{MessageID: firstMapString(item, "message_id", "messageId"), MessageType: messageType, Content: content}
		if value := strings.TrimSpace(ExtractText(child)); value != "" {
			textParts = append(textParts, value)
		}
		for _, attachment := range ExtractAttachments(child) {
			if attachment.ID == "" || attachment.Type == "merge_forward" {
				continue
			}
			attachment.Raw = item
			attachments = append(attachments, attachment)
		}
		attachments = append(attachments, extractNestedResourceAttachments(item)...)
		var decodedContent any
		if strings.TrimSpace(content) != "" && json.Unmarshal([]byte(content), &decodedContent) == nil {
			attachments = append(attachments, extractNestedResourceAttachments(decodedContent)...)
		}
	}
	attachments = dedupeAttachments(attachments)
	if len(items) == 0 {
		return "", nil, nil
	}
	if len(attachments) == 0 && len(textParts) == 0 {
		return "[转发消息未包含可读取的内容]", nil, nil
	}
	_ = outerMessageID // documents the download contract at the call site
	return strings.Join(textParts, "\n\n"), attachments, nil
}

func extractNestedResourceAttachments(value any) []MessageAttachment {
	var out []MessageAttachment
	var walk func(any, map[string]any)
	walk = func(current any, parent map[string]any) {
		switch node := current.(type) {
		case string:
			// Card payloads such as user_dsl and nested content are commonly
			// JSON-encoded strings rather than objects in the event envelope.
			// Decode only JSON-shaped strings so ordinary card text is untouched.
			trimmed := strings.TrimSpace(node)
			if len(trimmed) > 0 && len(trimmed) <= 1<<20 && (strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")) {
				var decoded any
				if json.Unmarshal([]byte(trimmed), &decoded) == nil {
					walk(decoded, parent)
				}
			}
		case []any:
			for _, child := range node {
				walk(child, parent)
			}
		case map[string]any:
			if key := firstMapString(node, "key", "resource_key"); key != "" {
				resourceType := strings.ToLower(firstMapString(node, "type", "resource_type"))
				if resourceType == "image" || resourceType == "file" || resourceType == "media" || resourceType == "audio" {
					attachmentType := resourceType
					if attachmentType == "media" || attachmentType == "audio" {
						attachmentType = "file"
					}
					out = append(out, MessageAttachment{ID: key, Type: attachmentType, Name: firstMapString(node, "file_name", "fileName", "name"), MIME: firstMapString(node, "mime_type", "mimeType"), Size: firstMapInt64(node, "file_size", "fileSize", "size"), Raw: parent})
				}
			}
			if imageKey := firstMapString(node, "image_key", "imageKey", "img_key", "imgKey"); imageKey != "" {
				out = append(out, MessageAttachment{ID: imageKey, Type: "image", Name: firstMapString(node, "file_name", "fileName", "name"), Raw: parent})
			}
			if imageToken := firstMapString(node, "image_token", "imageToken"); imageToken != "" {
				out = append(out, MessageAttachment{ID: imageToken, Type: "image", Name: firstMapString(node, "file_name", "fileName", "name"), Raw: parent})
			}
			if fileKey := firstMapString(node, "file_key", "fileKey", "file_token", "fileToken"); fileKey != "" {
				out = append(out, MessageAttachment{ID: fileKey, Type: "file", Name: firstMapString(node, "file_name", "fileName", "name"), MIME: firstMapString(node, "mime_type", "mimeType"), Size: firstMapInt64(node, "file_size", "fileSize", "size"), Raw: parent})
			}
			for _, child := range node {
				walk(child, node)
			}
		}
	}
	walk(value, nil)
	return out
}

func classifyLarkURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "link"
	}
	host := strings.ToLower(parsed.Hostname())
	path := strings.ToLower(parsed.Path)
	if strings.Contains(host, "feishu.cn") || strings.Contains(host, "larksuite.com") || strings.Contains(host, "larksuite.cn") {
		for _, marker := range []string{"/docx/", "/docs/", "/wiki/", "/sheets/", "/base/", "/bitable/", "/mindnotes/", "/minutes/"} {
			if strings.Contains(path, marker) {
				return "document"
			}
		}
	}
	return "link"
}

func dedupeAttachments(in []MessageAttachment) []MessageAttachment {
	seen := map[string]bool{}
	out := make([]MessageAttachment, 0, len(in))
	for _, attachment := range in {
		key := strings.TrimSpace(attachment.Type) + "|" + strings.TrimSpace(attachment.ID) + "|" + strings.TrimSpace(attachment.URL)
		if key == "||" {
			continue
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, attachment)
	}
	return out
}

func firstMapString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, _ := values[key].(string); strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstMapInt64(values map[string]any, keys ...string) int64 {
	for _, key := range keys {
		switch value := values[key].(type) {
		case float64:
			return int64(value)
		case int64:
			return value
		case int:
			return int64(value)
		case string:
			var parsed int64
			if _, err := fmt.Sscan(strings.TrimSpace(value), &parsed); err == nil {
				return parsed
			}
		}
	}
	return 0
}

func IsDirectChat(message EventMessage) bool {
	chatType := strings.ToLower(strings.TrimSpace(message.ChatType))
	return chatType == "" || chatType == "p2p" || chatType == "private"
}

func HasExplicitMention(message EventMessage) bool {
	if len(message.Mentions) > 0 {
		return true
	}
	var body struct {
		Text     string            `json:"text"`
		Mentions []json.RawMessage `json:"mentions"`
	}
	if json.Unmarshal([]byte(message.Content), &body) != nil {
		return false
	}
	if strings.Contains(body.Text, "@_user_") {
		return true
	}
	return len(body.Mentions) > 0
}

func IsReplyMessage(message EventMessage) bool {
	return strings.TrimSpace(message.RootID) != "" || strings.TrimSpace(message.ParentID) != ""
}
