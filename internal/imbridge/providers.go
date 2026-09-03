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
	MessageID     string
	ChatID        string
	ChatType      string
	RootID        string
	ParentID      string
	SenderOpenID  string
	SenderUserID  string
	SenderUnionID string
	MessageType   string
	Text          string
	RawContent    string
	Mentions      []json.RawMessage
	Attachments   []IncomingAttachment
}

type IncomingAttachment struct {
	ID      string         `json:"id,omitempty"`
	Type    string         `json:"type"`
	Name    string         `json:"name,omitempty"`
	URL     string         `json:"url,omitempty"`
	MIME    string         `json:"mime,omitempty"`
	Size    int64          `json:"size,omitempty"`
	Preview string         `json:"preview,omitempty"`
	Raw     map[string]any `json:"raw,omitempty"`
}

type ExternalUserProfile struct {
	OpenID          string
	UserID          string
	UnionID         string
	Name            string
	Email           string
	EnterpriseEmail string
}

type OutgoingTarget struct {
	ReceiveID        string
	ReceiveIDType    string
	ChatID           string
	ReplyToMessageID string
	MentionOpenID    string
	ChatType         string
}

type OutgoingMessage struct {
	Format        string
	Subject       string
	Text          string
	MentionOpenID string
	Card          *InteractiveCard
}

type OutgoingAttachment struct {
	Kind     string
	FileName string
	MIME     string
	Data     []byte
	Caption  string
}

type InteractiveCard struct {
	InteractionID string
	Title         string
	Body          string
	RawJSON       json.RawMessage
	Fields        []InteractiveCardField
	Actions       []InteractiveCardAction
	Links         []InteractiveCardLink
	Context       map[string]any
}

type InteractiveCardField struct {
	Label string
	Value string
}

type InteractiveCardAction struct {
	ID           string
	Label        string
	Style        string
	RequiresText bool
}

type InteractiveCardLink struct {
	Label string
	URL   string
}

type ProgressCardEntry struct {
	Kind    string
	Title   string
	Content string
	Status  string
}

type ProgressCard struct {
	Title     string
	State     string
	Reasoning []ProgressCardEntry
	Tools     []ProgressCardEntry
	Final     string
}

type IncomingInteractionCallback struct {
	InteractionID string
	MessageID     string
	ChatID        string
	SenderOpenID  string
	UpdateToken   string
	ActionID      string
	ActionLabel   string
	Inputs        map[string]string
	Raw           json.RawMessage
}

type ParsedEvent struct {
	AppID             string
	VerificationToken string
	IsURLVerification bool
	Challenge         string
	IsMessage         bool
	Message           IncomingMessage
	IsInteraction     bool
	Interaction       IncomingInteractionCallback
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
	SendText(ctx context.Context, secrets map[string]string, target OutgoingTarget, text string) error
	SendMessage(ctx context.Context, secrets map[string]string, target OutgoingTarget, message OutgoingMessage) error
}

type RichReplyProvider interface {
	ReplyMessage(ctx context.Context, secrets map[string]string, message IncomingMessage, reply OutgoingMessage) error
}

type ReactionProvider interface {
	AddReaction(ctx context.Context, secrets map[string]string, message IncomingMessage, emoji string) (string, error)
	RemoveReaction(ctx context.Context, secrets map[string]string, message IncomingMessage, reactionID string) error
}

type AttachmentSender interface {
	SendAttachment(ctx context.Context, secrets map[string]string, target OutgoingTarget, attachment OutgoingAttachment) error
	ReplyAttachment(ctx context.Context, secrets map[string]string, message IncomingMessage, attachment OutgoingAttachment) error
}

type IncomingAttachmentDownload struct {
	FileName string
	MIME     string
	Data     []byte
}

type AttachmentDownloader interface {
	DownloadAttachment(ctx context.Context, secrets map[string]string, message IncomingMessage, attachment IncomingAttachment) (IncomingAttachmentDownload, error)
}

// IncomingMessageEnricher lets a provider resolve references that are only
// available after receiving the platform event. For example, Lark's
// merge_forward event contains a summary while its child messages and file
// keys are fetched from the platform API. The core runtime stays provider
// agnostic and keeps the enrichment optional for other channels.
type IncomingMessageEnricher interface {
	EnrichIncomingMessage(ctx context.Context, secrets map[string]string, message IncomingMessage) (IncomingMessage, error)
}

type ProgressCardReplyProvider interface {
	StartProgressCardReply(ctx context.Context, secrets map[string]string, message IncomingMessage, card ProgressCard) (any, error)
	UpdateProgressCardReply(ctx context.Context, secrets map[string]string, handle any, card ProgressCard) error
}

type InteractionCardUpdater interface {
	UpdateInteractionCard(ctx context.Context, secrets map[string]string, callback IncomingInteractionCallback, message OutgoingMessage) error
}

type ExternalUserProfileResolver interface {
	ResolveExternalUserProfile(ctx context.Context, secrets map[string]string, externalUserID, userIDType string) (ExternalUserProfile, error)
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
	callback, isCallback, err := larkbridge.ParseCardActionEvent(env)
	if err != nil || isCallback {
		out.IsInteraction = isCallback
		out.Interaction = IncomingInteractionCallback{
			InteractionID: callback.InteractionID,
			MessageID:     callback.MessageID,
			ChatID:        callback.ChatID,
			SenderOpenID:  callback.OperatorOpenID,
			UpdateToken:   callback.UpdateToken,
			ActionID:      callback.ActionID,
			ActionLabel:   callback.ActionLabel,
			Inputs:        callback.Inputs,
			Raw:           callback.Raw,
		}
		return out, err
	}
	event, isMessage, err := larkbridge.ParseMessageEvent(env)
	if err != nil || !isMessage {
		return out, err
	}
	out.IsMessage = true
	out.Message = IncomingMessage{
		MessageID:     event.Message.MessageID,
		ChatID:        event.Message.ChatID,
		ChatType:      event.Message.ChatType,
		RootID:        event.Message.RootID,
		ParentID:      event.Message.ParentID,
		SenderOpenID:  event.Sender.SenderID.OpenID,
		SenderUserID:  event.Sender.SenderID.UserID,
		SenderUnionID: event.Sender.SenderID.UnionID,
		MessageType:   event.Message.MessageType,
		Text:          larkbridge.ExtractText(event.Message),
		RawContent:    event.Message.Content,
		Mentions:      event.Message.Mentions,
	}
	for _, attachment := range larkbridge.ExtractAttachments(event.Message) {
		out.Message.Attachments = append(out.Message.Attachments, IncomingAttachment{
			ID:      attachment.ID,
			Type:    attachment.Type,
			Name:    attachment.Name,
			URL:     attachment.URL,
			MIME:    attachment.MIME,
			Size:    attachment.Size,
			Preview: attachment.Preview,
			Raw:     attachment.Raw,
		})
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
		Mentions: message.Mentions,
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

func (p larkFamilyProvider) ReplyMessage(ctx context.Context, secrets map[string]string, message IncomingMessage, reply OutgoingMessage) error {
	client := larkbridge.OpenAPIClient{
		BaseURL:     secrets["baseUrl"],
		AppID:       secrets["appId"],
		AppSecret:   secrets["appSecret"],
		AccessToken: secrets["accessToken"],
	}
	reply.MentionOpenID = firstNonEmpty(reply.MentionOpenID, message.SenderOpenID)
	if reply.Card != nil {
		return client.ReplyInteractiveCard(ctx, message.MessageID, larkbridge.InteractiveCard{
			InteractionID: reply.Card.InteractionID,
			Title:         reply.Card.Title,
			Body:          reply.Card.Body,
			RawJSON:       reply.Card.RawJSON,
			Fields:        larkCardFields(reply.Card.Fields),
			Actions:       larkCardActions(reply.Card.Actions),
			Links:         larkCardLinks(reply.Card.Links),
		})
	}
	if strings.EqualFold(strings.TrimSpace(reply.Format), "markdown") {
		return client.ReplyMarkdown(ctx, message.MessageID, reply.Subject, reply.Text)
	}
	reply.Text = larkMentionPrefixedText(message.ChatType, reply.MentionOpenID, reply.Text)
	return client.ReplyText(ctx, message.MessageID, reply.Text)
}

func (p larkFamilyProvider) AddReaction(ctx context.Context, secrets map[string]string, message IncomingMessage, emoji string) (string, error) {
	client := larkbridge.OpenAPIClient{
		BaseURL:   secrets["baseUrl"],
		AppID:     secrets["appId"],
		AppSecret: secrets["appSecret"],
	}
	return client.AddReaction(ctx, message.MessageID, emoji)
}

func (p larkFamilyProvider) RemoveReaction(ctx context.Context, secrets map[string]string, message IncomingMessage, reactionID string) error {
	client := larkbridge.OpenAPIClient{
		BaseURL:   secrets["baseUrl"],
		AppID:     secrets["appId"],
		AppSecret: secrets["appSecret"],
	}
	return client.RemoveReaction(ctx, message.MessageID, reactionID)
}

func (p larkFamilyProvider) DownloadAttachment(ctx context.Context, secrets map[string]string, message IncomingMessage, attachment IncomingAttachment) (IncomingAttachmentDownload, error) {
	client := larkbridge.OpenAPIClient{
		BaseURL:   secrets["baseUrl"],
		AppID:     secrets["appId"],
		AppSecret: secrets["appSecret"],
	}
	resourceType := "file"
	if strings.EqualFold(strings.TrimSpace(attachment.Type), "image") {
		resourceType = "image"
	}
	download, err := client.DownloadMessageResource(ctx, message.MessageID, attachment.ID, resourceType)
	if err != nil {
		return IncomingAttachmentDownload{}, err
	}
	fileName := firstNonEmpty(attachment.Name, download.FileName, attachment.ID)
	return IncomingAttachmentDownload{
		FileName: fileName,
		MIME:     firstNonEmpty(attachment.MIME, download.MIME),
		Data:     download.Data,
	}, nil
}

func (p larkFamilyProvider) EnrichIncomingMessage(ctx context.Context, secrets map[string]string, message IncomingMessage) (IncomingMessage, error) {
	if !strings.EqualFold(strings.TrimSpace(message.MessageType), "merge_forward") {
		return message, nil
	}
	client := larkbridge.OpenAPIClient{
		BaseURL:   secrets["baseUrl"],
		AppID:     secrets["appId"],
		AppSecret: secrets["appSecret"],
	}
	text, attachments, err := client.ExpandMergeForwardMessage(ctx, message.MessageID)
	if err != nil {
		return message, err
	}
	if strings.TrimSpace(text) != "" {
		message.Text = strings.TrimSpace(text)
	}
	if len(attachments) > 0 {
		message.Attachments = make([]IncomingAttachment, 0, len(attachments))
		for _, attachment := range attachments {
			message.Attachments = append(message.Attachments, IncomingAttachment{
				ID:      attachment.ID,
				Type:    attachment.Type,
				Name:    attachment.Name,
				URL:     attachment.URL,
				MIME:    attachment.MIME,
				Size:    attachment.Size,
				Preview: attachment.Preview,
				Raw:     attachment.Raw,
			})
		}
	}
	return message, nil
}

func (p larkFamilyProvider) StartProgressCardReply(ctx context.Context, secrets map[string]string, message IncomingMessage, card ProgressCard) (any, error) {
	client := larkbridge.OpenAPIClient{
		BaseURL:   secrets["baseUrl"],
		AppID:     secrets["appId"],
		AppSecret: secrets["appSecret"],
	}
	return client.ReplyProgressCard(ctx, message.MessageID, larkbridge.ProgressCard{
		Title:     card.Title,
		State:     card.State,
		Reasoning: larkProgressEntries(card.Reasoning),
		Tools:     larkProgressEntries(card.Tools),
		Final:     card.Final,
	})
}

func (p larkFamilyProvider) UpdateProgressCardReply(ctx context.Context, secrets map[string]string, handle any, card ProgressCard) error {
	messageID, _ := handle.(string)
	client := larkbridge.OpenAPIClient{
		BaseURL:   secrets["baseUrl"],
		AppID:     secrets["appId"],
		AppSecret: secrets["appSecret"],
	}
	return client.PatchProgressCard(ctx, messageID, larkbridge.ProgressCard{
		Title:     card.Title,
		State:     card.State,
		Reasoning: larkProgressEntries(card.Reasoning),
		Tools:     larkProgressEntries(card.Tools),
		Final:     card.Final,
	})
}

func (p larkFamilyProvider) SendText(ctx context.Context, secrets map[string]string, target OutgoingTarget, text string) error {
	return p.SendMessage(ctx, secrets, target, OutgoingMessage{Format: "text", Text: text})
}

func (p larkFamilyProvider) SendMessage(ctx context.Context, secrets map[string]string, target OutgoingTarget, message OutgoingMessage) error {
	client := larkbridge.OpenAPIClient{
		BaseURL:   secrets["baseUrl"],
		AppID:     secrets["appId"],
		AppSecret: secrets["appSecret"],
	}
	receiveIDType := strings.TrimSpace(target.ReceiveIDType)
	receiveID := strings.TrimSpace(target.ReceiveID)
	if receiveID == "" {
		receiveID = strings.TrimSpace(target.ChatID)
		receiveIDType = "chat_id"
	}
	if message.Card != nil {
		message.Card.Body = larkMentionPrefixedText(target.ChatType, firstNonEmpty(message.MentionOpenID, target.MentionOpenID), message.Card.Body)
		return client.SendInteractiveCard(ctx, receiveIDType, receiveID, larkbridge.InteractiveCard{
			InteractionID: message.Card.InteractionID,
			Title:         message.Card.Title,
			Body:          message.Card.Body,
			RawJSON:       message.Card.RawJSON,
			Fields:        larkCardFields(message.Card.Fields),
			Actions:       larkCardActions(message.Card.Actions),
			Links:         larkCardLinks(message.Card.Links),
		})
	}
	message.Text = larkMentionPrefixedText(target.ChatType, firstNonEmpty(message.MentionOpenID, target.MentionOpenID), message.Text)
	if strings.EqualFold(strings.TrimSpace(message.Format), "markdown") {
		return client.SendMarkdown(ctx, receiveIDType, receiveID, message.Subject, message.Text)
	}
	return client.SendText(ctx, receiveIDType, receiveID, message.Text)
}

func (p larkFamilyProvider) ResolveExternalUserProfile(ctx context.Context, secrets map[string]string, externalUserID, userIDType string) (ExternalUserProfile, error) {
	client := larkbridge.OpenAPIClient{
		BaseURL:     secrets["baseUrl"],
		AppID:       secrets["appId"],
		AppSecret:   secrets["appSecret"],
		AccessToken: secrets["accessToken"],
	}
	profile, err := client.GetUser(ctx, externalUserID, userIDType)
	if err != nil {
		return ExternalUserProfile{}, err
	}
	return ExternalUserProfile{
		OpenID:          profile.OpenID,
		UserID:          profile.UserID,
		UnionID:         profile.UnionID,
		Name:            profile.Name,
		Email:           profile.Email,
		EnterpriseEmail: profile.EnterpriseEmail,
	}, nil
}

func (p larkFamilyProvider) SendAttachment(ctx context.Context, secrets map[string]string, target OutgoingTarget, attachment OutgoingAttachment) error {
	client := larkbridge.OpenAPIClient{
		BaseURL:   secrets["baseUrl"],
		AppID:     secrets["appId"],
		AppSecret: secrets["appSecret"],
	}
	receiveIDType := strings.TrimSpace(target.ReceiveIDType)
	receiveID := strings.TrimSpace(target.ReceiveID)
	if receiveID == "" {
		receiveID = strings.TrimSpace(target.ChatID)
		receiveIDType = "chat_id"
	}
	if strings.TrimSpace(attachment.Caption) != "" {
		caption := larkMentionPrefixedText(target.ChatType, target.MentionOpenID, attachment.Caption)
		if err := client.SendText(ctx, receiveIDType, receiveID, caption); err != nil {
			return err
		}
	}
	return client.SendAttachment(ctx, receiveIDType, receiveID, larkbridge.OutgoingAttachment{
		Kind:     attachment.Kind,
		FileName: attachment.FileName,
		MIME:     attachment.MIME,
		Data:     attachment.Data,
	})
}

func (p larkFamilyProvider) ReplyAttachment(ctx context.Context, secrets map[string]string, message IncomingMessage, attachment OutgoingAttachment) error {
	client := larkbridge.OpenAPIClient{
		BaseURL:   secrets["baseUrl"],
		AppID:     secrets["appId"],
		AppSecret: secrets["appSecret"],
	}
	if strings.TrimSpace(attachment.Caption) != "" {
		caption := larkMentionPrefixedText(message.ChatType, message.SenderOpenID, attachment.Caption)
		if err := client.ReplyText(ctx, message.MessageID, caption); err != nil {
			return err
		}
	}
	return client.ReplyAttachment(ctx, message.MessageID, larkbridge.OutgoingAttachment{
		Kind:     attachment.Kind,
		FileName: attachment.FileName,
		MIME:     attachment.MIME,
		Data:     attachment.Data,
	})
}

func larkMentionPrefixedText(chatType, openID, text string) string {
	text = strings.TrimSpace(text)
	openID = strings.TrimSpace(openID)
	if openID == "" || strings.EqualFold(strings.TrimSpace(chatType), "p2p") || strings.Contains(text, openID) {
		return text
	}
	return fmt.Sprintf(`<at user_id="%s"></at> %s`, openID, text)
}

func (p larkFamilyProvider) UpdateInteractionCard(ctx context.Context, secrets map[string]string, callback IncomingInteractionCallback, message OutgoingMessage) error {
	if message.Card == nil {
		return fmt.Errorf("interactive card is required")
	}
	client := larkbridge.OpenAPIClient{
		BaseURL:   secrets["baseUrl"],
		AppID:     secrets["appId"],
		AppSecret: secrets["appSecret"],
	}
	return client.UpdateInteractiveCard(ctx, callback.UpdateToken, callback.SenderOpenID, larkbridge.InteractiveCard{
		InteractionID: message.Card.InteractionID,
		Title:         message.Card.Title,
		Body:          message.Card.Body,
		Fields:        larkCardFields(message.Card.Fields),
		Actions:       larkCardActions(message.Card.Actions),
		Links:         larkCardLinks(message.Card.Links),
	})
}

func larkCardFields(fields []InteractiveCardField) []larkbridge.InteractiveCardField {
	out := make([]larkbridge.InteractiveCardField, 0, len(fields))
	for _, field := range fields {
		out = append(out, larkbridge.InteractiveCardField{Label: field.Label, Value: field.Value})
	}
	return out
}

func larkCardActions(actions []InteractiveCardAction) []larkbridge.InteractiveCardAction {
	out := make([]larkbridge.InteractiveCardAction, 0, len(actions))
	for _, action := range actions {
		out = append(out, larkbridge.InteractiveCardAction{ID: action.ID, Label: action.Label, Style: action.Style, RequiresText: action.RequiresText})
	}
	return out
}

func larkCardLinks(links []InteractiveCardLink) []larkbridge.InteractiveCardLink {
	out := make([]larkbridge.InteractiveCardLink, 0, len(links))
	for _, link := range links {
		out = append(out, larkbridge.InteractiveCardLink{Label: link.Label, URL: link.URL})
	}
	return out
}

func larkProgressEntries(entries []ProgressCardEntry) []larkbridge.ProgressCardEntry {
	out := make([]larkbridge.ProgressCardEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, larkbridge.ProgressCardEntry{
			Kind:    entry.Kind,
			Title:   entry.Title,
			Content: entry.Content,
			Status:  entry.Status,
		})
	}
	return out
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
func (slackProvider) SendText(ctx context.Context, secrets map[string]string, target OutgoingTarget, text string) error {
	return slackProvider{}.SendMessage(ctx, secrets, target, OutgoingMessage{Format: "text", Text: text})
}

func (slackProvider) SendMessage(ctx context.Context, secrets map[string]string, target OutgoingTarget, message OutgoingMessage) error {
	chatID := strings.TrimSpace(target.ChatID)
	if chatID == "" {
		chatID = strings.TrimSpace(target.ReceiveID)
	}
	if chatID == "" {
		return fmt.Errorf("slack chat id is required")
	}
	form := url.Values{}
	form.Set("channel", chatID)
	form.Set("text", emptyDefault(message.Text))
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
func (telegramProvider) SendText(ctx context.Context, secrets map[string]string, target OutgoingTarget, text string) error {
	return telegramProvider{}.SendMessage(ctx, secrets, target, OutgoingMessage{Format: "text", Text: text})
}

func (telegramProvider) SendMessage(ctx context.Context, secrets map[string]string, target OutgoingTarget, message OutgoingMessage) error {
	chatID := strings.TrimSpace(target.ChatID)
	if chatID == "" {
		chatID = strings.TrimSpace(target.ReceiveID)
	}
	if chatID == "" {
		return fmt.Errorf("telegram chat id is required")
	}
	body := map[string]any{"chat_id": chatID, "text": emptyDefault(message.Text)}
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
func (discordProvider) SendText(ctx context.Context, secrets map[string]string, target OutgoingTarget, text string) error {
	return discordProvider{}.SendMessage(ctx, secrets, target, OutgoingMessage{Format: "text", Text: text})
}

func (discordProvider) SendMessage(ctx context.Context, secrets map[string]string, target OutgoingTarget, message OutgoingMessage) error {
	chatID := strings.TrimSpace(target.ChatID)
	if chatID == "" {
		chatID = strings.TrimSpace(target.ReceiveID)
	}
	if chatID == "" {
		return fmt.Errorf("discord channel id is required")
	}
	body := map[string]any{"content": emptyDefault(message.Text)}
	_, err := httpJSON(ctx, http.MethodPost, "https://discord.com/api/v10/channels/"+chatID+"/messages", "Bot "+secrets["botToken"], body)
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
