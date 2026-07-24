package imbridge

import "testing"

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
			"sender":{"sender_id":{"open_id":"ou_one"}},
			"message":{
				"message_id":"om_one",
				"chat_id":"oc_one",
				"chat_type":"group",
				"message_type":"text",
				"content":"{\"text\":\"@bot hello\",\"mentions\":[{\"key\":\"@bot\"}]}"
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
	if parsed.Message.MessageID != "om_one" || parsed.Message.SenderOpenID != "ou_one" || parsed.Message.Text != "@bot hello" {
		t.Fatalf("unexpected message: %#v", parsed.Message)
	}
	if !provider.ShouldHandleMessage("", parsed.Message) {
		t.Fatalf("mentioned group message should be handled")
	}
	if provider.ShouldHandleMessage("oc_other", IncomingMessage{ChatType: "group", ChatID: "oc_one", RawContent: `{"text":"hello"}`}) {
		t.Fatalf("unmentioned unbound group should be ignored")
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
