package lark

import (
	"encoding/json"
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
	MessageID   string `json:"message_id"`
	RootID      string `json:"root_id"`
	ParentID    string `json:"parent_id"`
	ChatID      string `json:"chat_id"`
	ChatType    string `json:"chat_type"`
	MessageType string `json:"message_type"`
	Content     string `json:"content"`
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
	if message.MessageType != "text" && message.MessageType != "post" {
		return ""
	}
	var body map[string]any
	if json.Unmarshal([]byte(message.Content), &body) != nil {
		return strings.TrimSpace(message.Content)
	}
	if text, _ := body["text"].(string); text != "" {
		return strings.TrimSpace(text)
	}
	if title, _ := body["title"].(string); title != "" {
		return strings.TrimSpace(title)
	}
	return ""
}

func IsDirectChat(message EventMessage) bool {
	chatType := strings.ToLower(strings.TrimSpace(message.ChatType))
	return chatType == "" || chatType == "p2p" || chatType == "private"
}

func HasExplicitMention(message EventMessage) bool {
	var body struct {
		Mentions []json.RawMessage `json:"mentions"`
	}
	if json.Unmarshal([]byte(message.Content), &body) != nil {
		return false
	}
	return len(body.Mentions) > 0
}

func IsReplyMessage(message EventMessage) bool {
	return strings.TrimSpace(message.RootID) != "" || strings.TrimSpace(message.ParentID) != ""
}
