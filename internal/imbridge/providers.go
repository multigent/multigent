package imbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	larkbridge "github.com/multigent/multigent/internal/imbridge/lark"
)

type ProviderInfo struct {
	ID        string             `json:"id"`
	Label     string             `json:"label"`
	SetupMode string             `json:"setupMode"`
	Fields    []ManualSetupField `json:"fields,omitempty"`
}

type SetupBeginResponse = larkbridge.BeginResponse
type SetupPollResponse = larkbridge.PollResponse

type ManualSetupField struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Placeholder string `json:"placeholder,omitempty"`
	Help        string `json:"help,omitempty"`
}

type ManualSetupRequest struct {
	Values map[string]string `json:"values"`
}

type ManualSetupResult struct {
	Provider        string            `json:"provider"`
	BaseURL         string            `json:"baseUrl,omitempty"`
	AuthType        string            `json:"authType"`
	SecretValues    map[string]string `json:"-"`
	Profile         map[string]any    `json:"profile,omitempty"`
	AppID           string            `json:"appId,omitempty"`
	ExternalBotID   string            `json:"externalBotId,omitempty"`
	ExternalChatID  string            `json:"externalChatId,omitempty"`
	ExternalOwnerID string            `json:"externalOwnerId,omitempty"`
}

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
	ManualSetup(ctx context.Context, req ManualSetupRequest) (ManualSetupResult, error)
	ExtractEncryptedPayload(raw []byte) (string, bool)
	DecryptEvent(encryptedPayload, encryptKey string) ([]byte, error)
	ParseEvent(raw []byte) (ParsedEvent, error)
	ShouldHandleMessage(boundChatID string, message IncomingMessage) bool
	ReplyText(ctx context.Context, secrets map[string]string, message IncomingMessage, text string) error
}

var registry = []Provider{
	larkFamilyProvider{id: larkbridge.ProviderFeishu, label: "Feishu"},
	larkFamilyProvider{id: larkbridge.ProviderLark, label: "Lark"},
	slackProvider{},
	telegramProvider{},
	discordProvider{},
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
	return ProviderInfo{ID: p.id, Label: p.label, SetupMode: "qr"}
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

func (p larkFamilyProvider) ManualSetup(context.Context, ManualSetupRequest) (ManualSetupResult, error) {
	return ManualSetupResult{}, fmt.Errorf("%s uses QR setup", p.label)
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

type slackProvider struct{}

func (slackProvider) Info() ProviderInfo {
	return ProviderInfo{
		ID:        "slack",
		Label:     "Slack",
		SetupMode: "manual",
		Fields: []ManualSetupField{
			{Name: "botToken", Label: "Bot token", Type: "password", Required: true, Placeholder: "xoxb-..."},
			{Name: "appId", Label: "App ID", Type: "text", Placeholder: "A0123456789", Help: "Optional but recommended for Events API routing."},
			{Name: "verificationToken", Label: "Verification token", Type: "password", Help: "Optional legacy Slack Events verification token."},
		},
	}
}

func (slackProvider) OpenBaseURL() string { return "https://slack.com/api" }
func (slackProvider) BeginSetup(context.Context) (SetupBeginResponse, error) {
	return SetupBeginResponse{}, fmt.Errorf("slack uses manual setup")
}
func (slackProvider) PollSetup(context.Context, string, string) (SetupPollResponse, error) {
	return SetupPollResponse{}, fmt.Errorf("slack uses manual setup")
}
func (slackProvider) ManualSetup(ctx context.Context, req ManualSetupRequest) (ManualSetupResult, error) {
	values := normalizeManualValues(req.Values)
	token := values["botToken"]
	if token == "" {
		return ManualSetupResult{}, fmt.Errorf("slack bot token is required")
	}
	body, err := postForm(ctx, "https://slack.com/api/auth.test", token, nil)
	if err != nil {
		return ManualSetupResult{}, err
	}
	var parsed struct {
		OK     bool   `json:"ok"`
		Error  string `json:"error"`
		UserID string `json:"user_id"`
		BotID  string `json:"bot_id"`
		AppID  string `json:"app_id"`
		TeamID string `json:"team_id"`
		URL    string `json:"url"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ManualSetupResult{}, fmt.Errorf("slack auth.test: %w", err)
	}
	if !parsed.OK {
		return ManualSetupResult{}, fmt.Errorf("slack auth.test failed: %s", parsed.Error)
	}
	appID := firstNonEmpty(values["appId"], parsed.AppID)
	return ManualSetupResult{
		Provider:      "slack",
		BaseURL:       "https://slack.com/api",
		AuthType:      "bot_token",
		AppID:         appID,
		ExternalBotID: firstNonEmpty(parsed.BotID, parsed.UserID),
		SecretValues: map[string]string{
			"baseUrl":           "https://slack.com/api",
			"botToken":          token,
			"appId":             appID,
			"verificationToken": values["verificationToken"],
		},
		Profile: map[string]any{"teamId": parsed.TeamID, "appId": appID, "botId": firstNonEmpty(parsed.BotID, parsed.UserID), "url": parsed.URL},
	}, nil
}
func (slackProvider) ExtractEncryptedPayload([]byte) (string, bool) { return "", false }
func (slackProvider) DecryptEvent(string, string) ([]byte, error)   { return nil, nil }
func (slackProvider) ParseEvent(raw []byte) (ParsedEvent, error) {
	var env struct {
		Type      string          `json:"type"`
		Challenge string          `json:"challenge"`
		Token     string          `json:"token"`
		APIAppID  string          `json:"api_app_id"`
		Event     json.RawMessage `json:"event"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return ParsedEvent{}, err
	}
	if env.Type == "url_verification" {
		return ParsedEvent{AppID: env.APIAppID, VerificationToken: env.Token, IsURLVerification: true, Challenge: env.Challenge}, nil
	}
	var ev struct {
		Type            string `json:"type"`
		Subtype         string `json:"subtype"`
		BotID           string `json:"bot_id"`
		User            string `json:"user"`
		Channel         string `json:"channel"`
		ChannelType     string `json:"channel_type"`
		Text            string `json:"text"`
		TimeStamp       string `json:"ts"`
		ThreadTimeStamp string `json:"thread_ts"`
		ClientMsgID     string `json:"client_msg_id"`
	}
	if len(env.Event) == 0 || json.Unmarshal(env.Event, &ev) != nil || ev.Type != "message" {
		return ParsedEvent{AppID: env.APIAppID, VerificationToken: env.Token}, nil
	}
	if ev.BotID != "" || ev.User == "" || ev.Subtype != "" {
		return ParsedEvent{AppID: env.APIAppID, VerificationToken: env.Token}, nil
	}
	msgID := firstNonEmpty(ev.ClientMsgID, ev.TimeStamp)
	return ParsedEvent{
		AppID:             env.APIAppID,
		VerificationToken: env.Token,
		IsMessage:         true,
		Message: IncomingMessage{
			MessageID:    msgID,
			ChatID:       ev.Channel,
			ChatType:     ev.ChannelType,
			RootID:       ev.ThreadTimeStamp,
			ParentID:     ev.ThreadTimeStamp,
			SenderOpenID: ev.User,
			Text:         stripSlackMention(ev.Text),
			RawContent:   ev.Text,
		},
	}, nil
}
func (slackProvider) ShouldHandleMessage(boundChatID string, message IncomingMessage) bool {
	if strings.EqualFold(message.ChatType, "im") || strings.EqualFold(message.ChatType, "mpim") {
		return true
	}
	if strings.TrimSpace(boundChatID) != "" && strings.TrimSpace(boundChatID) == strings.TrimSpace(message.ChatID) {
		return true
	}
	return strings.Contains(message.RawContent, "<@") || strings.TrimSpace(message.ParentID) != ""
}
func (slackProvider) ReplyText(ctx context.Context, secrets map[string]string, message IncomingMessage, text string) error {
	form := url.Values{}
	form.Set("channel", message.ChatID)
	form.Set("text", emptyDefault(text))
	if threadTS := firstNonEmpty(message.RootID, message.ParentID); threadTS != "" {
		form.Set("thread_ts", threadTS)
	}
	body, err := postForm(ctx, "https://slack.com/api/chat.postMessage", secrets["botToken"], form)
	if err != nil {
		return err
	}
	var parsed struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("slack chat.postMessage: %w", err)
	}
	if !parsed.OK {
		return fmt.Errorf("slack chat.postMessage failed: %s", parsed.Error)
	}
	return nil
}

type telegramProvider struct{}

func (telegramProvider) Info() ProviderInfo {
	return ProviderInfo{ID: "telegram", Label: "Telegram", SetupMode: "manual", Fields: []ManualSetupField{
		{Name: "botToken", Label: "Bot token", Type: "password", Required: true, Placeholder: "123456:ABC..."},
	}}
}
func (telegramProvider) OpenBaseURL() string { return "https://api.telegram.org" }
func (telegramProvider) BeginSetup(context.Context) (SetupBeginResponse, error) {
	return SetupBeginResponse{}, fmt.Errorf("telegram uses manual setup")
}
func (telegramProvider) PollSetup(context.Context, string, string) (SetupPollResponse, error) {
	return SetupPollResponse{}, fmt.Errorf("telegram uses manual setup")
}
func (telegramProvider) ManualSetup(ctx context.Context, req ManualSetupRequest) (ManualSetupResult, error) {
	values := normalizeManualValues(req.Values)
	token := values["botToken"]
	if token == "" {
		return ManualSetupResult{}, fmt.Errorf("telegram bot token is required")
	}
	raw, err := httpJSON(ctx, http.MethodGet, "https://api.telegram.org/bot"+token+"/getMe", "", nil)
	if err != nil {
		return ManualSetupResult{}, err
	}
	var parsed struct {
		OK     bool `json:"ok"`
		Result struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
		} `json:"result"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ManualSetupResult{}, fmt.Errorf("telegram getMe: %w", err)
	}
	if !parsed.OK {
		return ManualSetupResult{}, fmt.Errorf("telegram getMe failed: %s", parsed.Description)
	}
	botID := strconv.FormatInt(parsed.Result.ID, 10)
	return ManualSetupResult{
		Provider:      "telegram",
		BaseURL:       "https://api.telegram.org",
		AuthType:      "bot_token",
		AppID:         botID,
		ExternalBotID: botID,
		SecretValues: map[string]string{
			"baseUrl":  "https://api.telegram.org",
			"botToken": token,
			"appId":    botID,
			"botName":  parsed.Result.Username,
		},
		Profile: map[string]any{"botId": botID, "botName": parsed.Result.Username},
	}, nil
}
func (telegramProvider) ExtractEncryptedPayload([]byte) (string, bool) { return "", false }
func (telegramProvider) DecryptEvent(string, string) ([]byte, error)   { return nil, nil }
func (telegramProvider) ParseEvent(raw []byte) (ParsedEvent, error) {
	msg, ok, err := parseTelegramMessage(raw)
	if err != nil || !ok {
		return ParsedEvent{}, err
	}
	return ParsedEvent{AppID: "", IsMessage: true, Message: msg}, nil
}
func (telegramProvider) ShouldHandleMessage(boundChatID string, message IncomingMessage) bool {
	if message.ChatType == "private" {
		return true
	}
	if strings.TrimSpace(boundChatID) != "" && strings.TrimSpace(boundChatID) == strings.TrimSpace(message.ChatID) {
		return true
	}
	raw := strings.TrimSpace(message.RawContent)
	return strings.Contains(raw, "@") || strings.HasPrefix(raw, "/")
}
func (telegramProvider) ReplyText(ctx context.Context, secrets map[string]string, message IncomingMessage, text string) error {
	body := map[string]any{"chat_id": message.ChatID, "text": emptyDefault(text), "reply_to_message_id": message.MessageID}
	_, err := httpJSON(ctx, http.MethodPost, "https://api.telegram.org/bot"+secrets["botToken"]+"/sendMessage", "", body)
	return err
}

type discordProvider struct{}

func (discordProvider) Info() ProviderInfo {
	return ProviderInfo{ID: "discord", Label: "Discord", SetupMode: "manual", Fields: []ManualSetupField{
		{Name: "botToken", Label: "Bot token", Type: "password", Required: true},
	}}
}
func (discordProvider) OpenBaseURL() string { return "https://discord.com/api/v10" }
func (discordProvider) BeginSetup(context.Context) (SetupBeginResponse, error) {
	return SetupBeginResponse{}, fmt.Errorf("discord uses manual setup")
}
func (discordProvider) PollSetup(context.Context, string, string) (SetupPollResponse, error) {
	return SetupPollResponse{}, fmt.Errorf("discord uses manual setup")
}
func (discordProvider) ManualSetup(ctx context.Context, req ManualSetupRequest) (ManualSetupResult, error) {
	values := normalizeManualValues(req.Values)
	token := values["botToken"]
	if token == "" {
		return ManualSetupResult{}, fmt.Errorf("discord bot token is required")
	}
	raw, err := httpJSON(ctx, http.MethodGet, "https://discord.com/api/v10/users/@me", "Bot "+token, nil)
	if err != nil {
		return ManualSetupResult{}, err
	}
	var me struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	}
	if err := json.Unmarshal(raw, &me); err != nil {
		return ManualSetupResult{}, fmt.Errorf("discord users/@me: %w", err)
	}
	if me.ID == "" {
		return ManualSetupResult{}, fmt.Errorf("discord users/@me returned empty bot id")
	}
	return ManualSetupResult{
		Provider:      "discord",
		BaseURL:       "https://discord.com/api/v10",
		AuthType:      "bot_token",
		AppID:         me.ID,
		ExternalBotID: me.ID,
		SecretValues: map[string]string{
			"baseUrl":  "https://discord.com/api/v10",
			"botToken": token,
			"appId":    me.ID,
			"botName":  me.Username,
		},
		Profile: map[string]any{"botId": me.ID, "botName": me.Username},
	}, nil
}
func (discordProvider) ExtractEncryptedPayload([]byte) (string, bool) { return "", false }
func (discordProvider) DecryptEvent(string, string) ([]byte, error)   { return nil, nil }
func (discordProvider) ParseEvent([]byte) (ParsedEvent, error)        { return ParsedEvent{}, nil }
func (discordProvider) ShouldHandleMessage(boundChatID string, message IncomingMessage) bool {
	if message.ChatType == "dm" {
		return true
	}
	if strings.TrimSpace(boundChatID) != "" && strings.TrimSpace(boundChatID) == strings.TrimSpace(message.ChatID) {
		return true
	}
	return strings.Contains(message.RawContent, "<@") || strings.TrimSpace(message.ParentID) != ""
}
func (discordProvider) ReplyText(ctx context.Context, secrets map[string]string, message IncomingMessage, text string) error {
	body := map[string]any{"content": emptyDefault(text), "message_reference": map[string]any{"message_id": message.MessageID, "channel_id": message.ChatID, "fail_if_not_exists": false}}
	_, err := httpJSON(ctx, http.MethodPost, "https://discord.com/api/v10/channels/"+message.ChatID+"/messages", "Bot "+secrets["botToken"], body)
	return err
}

func normalizeManualValues(values map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range values {
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func emptyDefault(text string) string {
	if strings.TrimSpace(text) == "" {
		return "(empty response)"
	}
	return strings.TrimSpace(text)
}

func stripSlackMention(text string) string {
	fields := strings.Fields(text)
	kept := make([]string, 0, len(fields))
	for _, field := range fields {
		if strings.HasPrefix(field, "<@") && strings.HasSuffix(field, ">") {
			continue
		}
		kept = append(kept, field)
	}
	return strings.TrimSpace(strings.Join(kept, " "))
}

func postForm(ctx context.Context, endpoint, bearer string, form url.Values) ([]byte, error) {
	if form == nil {
		form = url.Values{}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if strings.TrimSpace(bearer) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(bearer))
	}
	return doHTTP(req)
}

func httpJSON(ctx context.Context, method, endpoint, auth string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(auth) != "" {
		req.Header.Set("Authorization", strings.TrimSpace(auth))
	}
	return doHTTP(req)
}

func doHTTP(req *http.Request) ([]byte, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s %s http %d: %s", req.Method, req.URL.String(), resp.StatusCode, string(raw))
	}
	return raw, nil
}

func parseTelegramMessage(raw []byte) (IncomingMessage, bool, error) {
	var update struct {
		Message *struct {
			MessageID int64 `json:"message_id"`
			From      *struct {
				ID       int64  `json:"id"`
				Username string `json:"username"`
			} `json:"from"`
			Chat *struct {
				ID   int64  `json:"id"`
				Type string `json:"type"`
			} `json:"chat"`
			Text string `json:"text"`
		} `json:"message"`
	}
	if err := json.Unmarshal(raw, &update); err != nil {
		return IncomingMessage{}, false, err
	}
	if update.Message == nil || update.Message.Chat == nil || update.Message.From == nil || strings.TrimSpace(update.Message.Text) == "" {
		return IncomingMessage{}, false, nil
	}
	return IncomingMessage{
		MessageID:    strconv.FormatInt(update.Message.MessageID, 10),
		ChatID:       strconv.FormatInt(update.Message.Chat.ID, 10),
		ChatType:     update.Message.Chat.Type,
		SenderOpenID: strconv.FormatInt(update.Message.From.ID, 10),
		Text:         strings.TrimSpace(update.Message.Text),
		RawContent:   update.Message.Text,
	}, true, nil
}
