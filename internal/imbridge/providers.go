package imbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	larkbridge "github.com/multigent/multigent/internal/imbridge/lark"
)

type ProviderInfo struct {
	ID    string
	Label string
}

type SetupBeginResponse = larkbridge.BeginResponse
type SetupPollResponse = larkbridge.PollResponse

type IncomingMessage struct {
	MessageID    string
	ChatID       string
	ChatType     string
	RootID       string
	ParentID     string
	SenderOpenID string
	Text         string
	RawContent   string
}

type ParsedEvent struct {
	AppID             string
	VerificationToken string
	IsURLVerification bool
	Challenge         string
	IsMessage         bool
	Message           IncomingMessage
}

type Provider interface {
	Info() ProviderInfo
	OpenBaseURL() string
	BeginSetup(ctx context.Context) (SetupBeginResponse, error)
	PollSetup(ctx context.Context, deviceCode, baseURL string) (SetupPollResponse, error)
	ExtractEncryptedPayload(raw []byte) (string, bool)
	DecryptEvent(encryptedPayload, encryptKey string) ([]byte, error)
	ParseEvent(raw []byte) (ParsedEvent, error)
	ShouldHandleMessage(boundChatID string, message IncomingMessage) bool
	ReplyText(ctx context.Context, secrets map[string]string, message IncomingMessage, text string) error
}

var registry = []Provider{
	larkFamilyProvider{id: larkbridge.ProviderFeishu, label: "Feishu"},
	larkFamilyProvider{id: larkbridge.ProviderLark, label: "Lark"},
}

func Providers() []ProviderInfo {
	out := make([]ProviderInfo, 0, len(registry))
	for _, provider := range registry {
		out = append(out, provider.Info())
	}
	return out
}

func LookupProvider(id string) (Provider, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, provider := range registry {
		if provider.Info().ID == id {
			return provider, true
		}
	}
	return nil, false
}

type larkFamilyProvider struct {
	id    string
	label string
}

func (p larkFamilyProvider) Info() ProviderInfo {
	return ProviderInfo{ID: p.id, Label: p.label}
}

func (p larkFamilyProvider) OpenBaseURL() string {
	if p.id == larkbridge.ProviderLark {
		return "https://open.larksuite.com"
	}
	return "https://open.feishu.cn"
}

func (p larkFamilyProvider) BeginSetup(ctx context.Context) (SetupBeginResponse, error) {
	client := larkbridge.RegistrationClient{}
	return client.Begin(ctx, p.id)
}

func (p larkFamilyProvider) PollSetup(ctx context.Context, deviceCode, baseURL string) (SetupPollResponse, error) {
	client := larkbridge.RegistrationClient{}
	return client.Poll(ctx, p.id, deviceCode, baseURL)
}

func (p larkFamilyProvider) ExtractEncryptedPayload(raw []byte) (string, bool) {
	return larkbridge.ExtractEncryptedPayload(raw)
}

func (p larkFamilyProvider) DecryptEvent(encryptedPayload, encryptKey string) ([]byte, error) {
	return larkbridge.DecryptEncryptedEvent(encryptedPayload, encryptKey)
}

func (p larkFamilyProvider) ParseEvent(raw []byte) (ParsedEvent, error) {
	var env larkbridge.EventEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return ParsedEvent{}, err
	}
	out := ParsedEvent{
		AppID:             env.Header.AppID,
		VerificationToken: env.Token,
		IsURLVerification: larkbridge.IsURLVerification(env),
		Challenge:         env.Challenge,
	}
	event, isMessage, err := larkbridge.ParseMessageEvent(env)
	if err != nil || !isMessage {
		return out, err
	}
	out.IsMessage = true
	out.Message = IncomingMessage{
		MessageID:    event.Message.MessageID,
		ChatID:       event.Message.ChatID,
		ChatType:     event.Message.ChatType,
		RootID:       event.Message.RootID,
		ParentID:     event.Message.ParentID,
		SenderOpenID: event.Sender.SenderID.OpenID,
		Text:         larkbridge.ExtractText(event.Message),
		RawContent:   event.Message.Content,
	}
	return out, nil
}

func (p larkFamilyProvider) ShouldHandleMessage(boundChatID string, message IncomingMessage) bool {
	larkMessage := larkbridge.EventMessage{
		ChatID:   message.ChatID,
		ChatType: message.ChatType,
		RootID:   message.RootID,
		ParentID: message.ParentID,
		Content:  message.RawContent,
	}
	if larkbridge.IsDirectChat(larkMessage) {
		return true
	}
	if strings.TrimSpace(boundChatID) != "" && strings.TrimSpace(boundChatID) == strings.TrimSpace(message.ChatID) {
		return true
	}
	return larkbridge.HasExplicitMention(larkMessage) || larkbridge.IsReplyMessage(larkMessage)
}

func (p larkFamilyProvider) ReplyText(ctx context.Context, secrets map[string]string, message IncomingMessage, text string) error {
	client := larkbridge.OpenAPIClient{
		BaseURL:   secrets["baseUrl"],
		AppID:     secrets["appId"],
		AppSecret: secrets["appSecret"],
	}
	return client.ReplyText(ctx, message.MessageID, text)
}

func MustOpenBaseURL(id string) (string, error) {
	provider, ok := LookupProvider(id)
	if !ok {
		return "", fmt.Errorf("unsupported IM provider %q", id)
	}
	return provider.OpenBaseURL(), nil
}
