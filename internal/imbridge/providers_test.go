package imbridge

import "testing"

func TestProviderRegistryExposesFeishuAndLark(t *testing.T) {
	providers := Providers()
	if len(providers) != 2 {
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
	if _, ok := LookupProvider("slack"); ok {
		t.Fatalf("unknown provider should not resolve")
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
