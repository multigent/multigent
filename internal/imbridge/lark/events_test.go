package lark

import "testing"

func TestMessageAddressingHelpers(t *testing.T) {
	if !IsDirectChat(EventMessage{ChatType: "p2p"}) {
		t.Fatalf("p2p should be direct")
	}
	if IsDirectChat(EventMessage{ChatType: "group"}) {
		t.Fatalf("group should not be direct")
	}
	if !HasExplicitMention(EventMessage{Content: `{"text":"@bot hi","mentions":[{"key":"@bot"}]}`}) {
		t.Fatalf("mentions should be detected")
	}
	if HasExplicitMention(EventMessage{Content: `{"text":"hi"}`}) {
		t.Fatalf("plain text should not count as mention")
	}
	if !IsReplyMessage(EventMessage{ParentID: "om_parent"}) {
		t.Fatalf("parent id should count as reply")
	}
}

func TestParseCardActionEvent(t *testing.T) {
	env := EventEnvelope{
		Header: EventHeader{EventType: CardActionEvent, AppID: "cli_app"},
		Event: []byte(`{
			"token":"update_token",
			"operator":{"operator_id":{"open_id":"ou_user"}},
			"context":{"open_message_id":"om_card"},
			"action":{
				"value":{"interaction_id":"ir_123","action_id":"request_changes","action_label":"打回修改"},
				"form_value":{"reason":"测试覆盖不足"}
			}
		}`),
	}
	callback, ok, err := ParseCardActionEvent(env)
	if err != nil || !ok {
		t.Fatalf("callback ok=%v err=%v", ok, err)
	}
	if callback.InteractionID != "ir_123" || callback.OperatorOpenID != "ou_user" || callback.UpdateToken != "update_token" || callback.ActionID != "request_changes" || callback.Inputs["reason"] != "测试覆盖不足" {
		t.Fatalf("unexpected callback: %#v", callback)
	}
}
