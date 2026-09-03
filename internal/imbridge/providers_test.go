package imbridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestProviderRegistryExposesFeishuAndLark(t *testing.T) {
	providers := Providers()
	if len(providers) != 5 {
		t.Fatalf("providers len=%d", len(providers))
	}
	feishu, ok := LookupProvider("FEISHU")
	if !ok {
		t.Fatalf("feishu provider not found")
	}
	if feishu.Info().ID != "feishu" || feishu.OpenBaseURL() != "https://open.feishu.cn" {
		t.Fatalf("unexpected feishu provider: %#v %s", feishu.Info(), feishu.OpenBaseURL())
	}
	lark, ok := LookupProvider("lark")
	if !ok {
		t.Fatalf("lark provider not found")
	}
	if lark.Info().ID != "lark" || lark.OpenBaseURL() != "https://open.larksuite.com" {
		t.Fatalf("unexpected lark provider: %#v %s", lark.Info(), lark.OpenBaseURL())
	}
	for _, id := range []string{"slack", "telegram", "discord"} {
		provider, ok := LookupProvider(id)
		if !ok {
			t.Fatalf("%s provider not found", id)
		}
		if provider.Info().SetupMode != "manual" {
			t.Fatalf("%s setup mode=%q", id, provider.Info().SetupMode)
		}
	}
}

func TestProviderParsesIMMessageEvent(t *testing.T) {
	provider, ok := LookupProvider("feishu")
	if !ok {
		t.Fatalf("feishu provider not found")
	}
	raw := []byte(`{
		"schema":"2.0",
		"token":"verify-one",
		"header":{"event_type":"im.message.receive_v1","app_id":"cli_app"},
		"event":{
			"sender":{"sender_id":{"open_id":"ou_one","user_id":"u_one","union_id":"on_one"}},
			"message":{
				"message_id":"om_one",
				"chat_id":"oc_one",
				"chat_type":"group",
				"message_type":"text",
				"content":"{\"text\":\"@bot hello\"}",
				"mentions":[{"key":"@bot","id":{"open_id":"ou_bot"}}]
			}
		}
	}`)
	parsed, err := provider.ParseEvent(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !parsed.IsMessage || parsed.AppID != "cli_app" || parsed.VerificationToken != "verify-one" {
		t.Fatalf("unexpected parsed event: %#v", parsed)
	}
	if parsed.Message.MessageID != "om_one" || parsed.Message.SenderOpenID != "ou_one" || parsed.Message.SenderUserID != "u_one" || parsed.Message.SenderUnionID != "on_one" || parsed.Message.Text != "@bot hello" {
		t.Fatalf("unexpected message: %#v", parsed.Message)
	}
	if len(parsed.Message.Mentions) != 1 {
		t.Fatalf("expected top-level mention to be parsed: %#v", parsed.Message)
	}
	if !provider.ShouldHandleMessage("", parsed.Message) {
		t.Fatalf("mentioned group message should be handled")
	}
	if provider.ShouldHandleMessage("oc_other", IncomingMessage{ChatType: "group", ChatID: "oc_one", RawContent: `{"text":"hello"}`}) {
		t.Fatalf("unmentioned unbound group should be ignored")
	}
}

func TestProviderParsesLarkImageAttachment(t *testing.T) {
	provider, ok := LookupProvider("feishu")
	if !ok {
		t.Fatalf("feishu provider not found")
	}
	raw := []byte(`{
		"schema":"2.0",
		"token":"verify-one",
		"header":{"event_type":"im.message.receive_v1","app_id":"cli_app"},
		"event":{
			"sender":{"sender_id":{"open_id":"ou_one"}},
			"message":{
				"message_id":"om_image",
				"chat_id":"oc_one",
				"chat_type":"p2p",
				"message_type":"image",
				"content":"{\"image_key\":\"img_v3_abc\"}"
			}
		}
	}`)
	parsed, err := provider.ParseEvent(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !parsed.IsMessage || parsed.Message.Text != "" || parsed.Message.MessageType != "image" {
		t.Fatalf("unexpected parsed image message: %#v", parsed.Message)
	}
	if len(parsed.Message.Attachments) != 1 || parsed.Message.Attachments[0].Type != "image" || parsed.Message.Attachments[0].ID != "img_v3_abc" {
		t.Fatalf("unexpected attachments: %#v", parsed.Message.Attachments)
	}
}

func TestProviderParsesLarkInteractiveAndForwardedMessages(t *testing.T) {
	provider, ok := LookupProvider("lark")
	if !ok {
		t.Fatalf("lark provider not found")
	}
	cases := []struct {
		name        string
		messageType string
		content     string
		wantText    []string
		wantType    string
	}{
		{
			name:        "interactive card",
			messageType: "interactive",
			content:     `{"header":{"title":{"tag":"plain_text","content":"Review 请求"}},"body":{"elements":[{"tag":"markdown","content":"**结论**\n\n请查看高风险问题"}]}}`,
			wantText:    []string{"Review 请求", "**结论**", "请查看高风险问题"},
			wantType:    "interactive",
		},
		{
			name:        "forwarded message",
			messageType: "merge_forward",
			content:     `{"message_id":"om_forwarded","title":"原始讨论"}`,
			wantText:    []string{"原始讨论"},
			wantType:    "merge_forward",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(`{"schema":"2.0","header":{"event_type":"im.message.receive_v1","app_id":"cli_app"},"event":{"sender":{"sender_id":{"open_id":"ou_one"}},"message":{"message_id":"om_card","chat_id":"oc_one","chat_type":"p2p","message_type":"` + tc.messageType + `","content":` + strconv.Quote(tc.content) + `}}}`)
			parsed, err := provider.ParseEvent(raw)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			for _, want := range tc.wantText {
				if !strings.Contains(parsed.Message.Text, want) {
					t.Fatalf("text %q missing %q", parsed.Message.Text, want)
				}
			}
			if len(parsed.Message.Attachments) != 1 || parsed.Message.Attachments[0].Type != tc.wantType {
				t.Fatalf("unexpected attachments: %#v", parsed.Message.Attachments)
			}
		})
	}
}

func TestProviderParsesLarkDocumentLinkAttachment(t *testing.T) {
	provider, ok := LookupProvider("feishu")
	if !ok {
		t.Fatalf("feishu provider not found")
	}
	raw := []byte(`{
		"schema":"2.0",
		"token":"verify-one",
		"header":{"event_type":"im.message.receive_v1","app_id":"cli_app"},
		"event":{
			"sender":{"sender_id":{"open_id":"ou_one"}},
			"message":{
				"message_id":"om_doc",
				"chat_id":"oc_one",
				"chat_type":"p2p",
				"message_type":"text",
				"content":"{\"text\":\"看这个 https://example.feishu.cn/docx/ABC123\"}"
			}
		}
	}`)
	parsed, err := provider.ParseEvent(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed.Message.Attachments) != 1 || parsed.Message.Attachments[0].Type != "document" || !strings.Contains(parsed.Message.Attachments[0].URL, "/docx/ABC123") {
		t.Fatalf("unexpected attachments: %#v", parsed.Message.Attachments)
	}
}

func TestFeishuAndLarkProvidersParseInteractionCallback(t *testing.T) {
	raw := []byte(`{
		"schema":"2.0",
		"token":"verify-one",
		"header":{"event_type":"card.action.trigger","app_id":"cli_app"},
		"event":{
			"token":"update-one",
			"operator":{"operator_id":{"open_id":"ou_user"}},
			"context":{"open_message_id":"om_card","open_chat_id":"oc_card"},
			"action":{
				"value":{"interaction_id":"ir_123","action_id":"approve","action_label":"Approve"},
				"form_value":{"comment":"looks good"}
			}
		}
	}`)
	for _, id := range []string{"feishu", "lark"} {
		provider, ok := LookupProvider(id)
		if !ok {
			t.Fatalf("%s provider not found", id)
		}
		parsed, err := provider.ParseEvent(raw)
		if err != nil {
			t.Fatalf("%s parse: %v", id, err)
		}
		if !parsed.IsInteraction || parsed.AppID != "cli_app" || parsed.VerificationToken != "verify-one" {
			t.Fatalf("%s unexpected parsed event: %#v", id, parsed)
		}
		got := parsed.Interaction
		if got.InteractionID != "ir_123" || got.ActionID != "approve" || got.SenderOpenID != "ou_user" || got.UpdateToken != "update-one" || got.Inputs["comment"] != "looks good" {
			t.Fatalf("%s unexpected callback: %#v", id, got)
		}
	}
}

func TestLarkMentionPrefixIsProviderConcernForNonReplySend(t *testing.T) {
	got := larkMentionPrefixedText("group", "ou_user", "## 结论\n\n- 已处理")
	if got != `<at user_id="ou_user"></at> ## 结论`+"\n\n- 已处理" {
		t.Fatalf("unexpected mention prefix: %q", got)
	}
	if got := larkMentionPrefixedText("p2p", "ou_user", "hello"); got != "hello" {
		t.Fatalf("p2p should not add mention: %q", got)
	}
	if got := larkMentionPrefixedText("group", "", "hello"); got != "hello" {
		t.Fatalf("empty open id should not add mention: %q", got)
	}
}

func TestLarkReplyMarkdownDoesNotMutateBodyWithMention(t *testing.T) {
	var replyBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "tenant-token"})
		case "/open-apis/im/v1/messages/om_one/reply":
			if err := json.NewDecoder(r.Body).Decode(&replyBody); err != nil {
				t.Fatalf("decode reply body: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	provider := larkFamilyProvider{id: "feishu", label: "Feishu"}
	err := provider.ReplyMessage(context.Background(), map[string]string{
		"baseUrl":   server.URL,
		"appId":     "cli_app",
		"appSecret": "secret",
	}, IncomingMessage{
		MessageID:    "om_one",
		ChatID:       "oc_one",
		ChatType:     "group",
		SenderOpenID: "ou_sender",
	}, OutgoingMessage{
		Format: "markdown",
		Text:   "## 结论\n\n- 已处理",
	})
	if err != nil {
		t.Fatalf("reply markdown: %v", err)
	}
	if replyBody["msg_type"] != "post" {
		t.Fatalf("markdown reply should use post msg_type, got %#v", replyBody["msg_type"])
	}
	content, _ := replyBody["content"].(string)
	if strings.Contains(content, "ou_sender") || strings.Contains(content, "<at") {
		t.Fatalf("markdown reply body should not be mention-prefixed: %s", content)
	}
	if !strings.Contains(content, "## 结论") {
		t.Fatalf("markdown body not preserved: %s", content)
	}
}

func TestProviderParsesSlackMessageEvent(t *testing.T) {
	provider, ok := LookupProvider("slack")
	if !ok {
		t.Fatalf("slack provider not found")
	}
	parsed, err := provider.ParseEvent([]byte(`{
		"type":"event_callback",
		"token":"verify-one",
		"api_app_id":"A123",
		"event":{"type":"message","user":"U1","channel":"C1","channel_type":"channel","text":"<@B1> hello","ts":"172.1"}
	}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !parsed.IsMessage || parsed.AppID != "A123" || parsed.Message.Text != "hello" || parsed.Message.SenderOpenID != "U1" {
		t.Fatalf("unexpected parsed event: %#v", parsed)
	}
	if !provider.ShouldHandleMessage("", parsed.Message) {
		t.Fatalf("mentioned slack group message should be handled")
	}
	if provider.ShouldHandleMessage("", IncomingMessage{ChatType: "channel", ChatID: "C1", RawContent: "hello"}) {
		t.Fatalf("unmentioned slack channel message should be ignored")
	}
}

func TestProviderParsesTelegramMessageEvent(t *testing.T) {
	provider, ok := LookupProvider("telegram")
	if !ok {
		t.Fatalf("telegram provider not found")
	}
	parsed, err := provider.ParseEvent([]byte(`{
		"update_id": 1,
		"message": {"message_id": 9, "from": {"id": 42, "username": "alice"}, "chat": {"id": -100, "type": "group"}, "text": "/start@agent hello"}
	}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !parsed.IsMessage || parsed.Message.MessageID != "9" || parsed.Message.ChatID != "-100" || parsed.Message.SenderOpenID != "42" {
		t.Fatalf("unexpected parsed telegram: %#v", parsed)
	}
	if !provider.ShouldHandleMessage("", parsed.Message) {
		t.Fatalf("telegram command mention should be handled")
	}
	if provider.ShouldHandleMessage("", IncomingMessage{ChatType: "group", ChatID: "-100", RawContent: "hello"}) {
		t.Fatalf("unmentioned telegram group message should be ignored")
	}
	if !provider.ShouldHandleMessage("-100", IncomingMessage{ChatType: "group", ChatID: "-100", RawContent: "hello"}) {
		t.Fatalf("bound telegram group message should be handled")
	}
}

func TestDiscordProviderShouldHandleMention(t *testing.T) {
	provider, ok := LookupProvider("discord")
	if !ok {
		t.Fatalf("discord provider not found")
	}
	if !provider.ShouldHandleMessage("", IncomingMessage{ChatType: "guild", RawContent: "<@123> hello"}) {
		t.Fatalf("discord mention should be handled")
	}
	if provider.ShouldHandleMessage("", IncomingMessage{ChatType: "guild", RawContent: "hello"}) {
		t.Fatalf("unmentioned discord guild message should be ignored")
	}
	if !provider.ShouldHandleMessage("", IncomingMessage{ChatType: "dm", RawContent: "hello"}) {
		t.Fatalf("discord dm should be handled")
	}
}
