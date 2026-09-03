package lark

import (
	"encoding/json"
	"testing"
)

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
	if !HasExplicitMention(EventMessage{Content: `{"text":"@bot hi"}`, Mentions: []json.RawMessage{json.RawMessage(`{"key":"@bot"}`)}}) {
		t.Fatalf("top-level mentions should be detected")
	}
	if !HasExplicitMention(EventMessage{Content: `{"text":"@_user_1 hi"}`}) {
		t.Fatalf("feishu mention placeholder should be detected")
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

func TestExpandMergeForwardItemsExtractsChildResources(t *testing.T) {
	text, attachments, err := ExpandMergeForwardItems([]map[string]any{
		{
			"message_id": "child-text",
			"msg_type":   "text",
			"body":       map[string]any{"content": `{"text":"日报详情"}`},
		},
		{
			"message_id": "child-file",
			"msg_type":   "file",
			"body":       map[string]any{"content": `{"file_key":"file_1","file_name":"report.md","file_size":42}`},
		},
	}, "om_outer")
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if text != "日报详情" {
		t.Fatalf("unexpected text: %q", text)
	}
	if len(attachments) != 1 || attachments[0].ID != "file_1" || attachments[0].Type != "file" || attachments[0].Name != "report.md" || attachments[0].Size != 42 {
		t.Fatalf("unexpected attachments: %#v", attachments)
	}
}

func TestExpandMergeForwardItemsExtractsCardEmbeddedImage(t *testing.T) {
	_, attachments, err := ExpandMergeForwardItems([]map[string]any{{
		"message_id": "child-card",
		"msg_type":   "interactive",
		"body":       map[string]any{"content": `{"body":{"elements":[{"tag":"img","img_key":"img_1"},{"tag":"markdown","content":"日报"}]}}`},
	}}, "om_outer")
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if len(attachments) != 1 || attachments[0].ID != "img_1" || attachments[0].Type != "image" {
		t.Fatalf("unexpected card attachments: %#v", attachments)
	}

	_, attachments, err = ExpandMergeForwardItems([]map[string]any{{
		"message_id": "child-card",
		"msg_type":   "interactive",
		"body":       map[string]any{"content": `{"body":{"elements":[{"tag":"img","image_key":"img_2"}]}}`},
	}}, "om_outer")
	if err != nil || len(attachments) != 1 || attachments[0].ID != "img_2" || attachments[0].Type != "image" {
		t.Fatalf("unexpected card attachments: err=%v attachments=%#v", err, attachments)
	}
}

func TestExtractAttachmentsExtractsInteractiveCardResources(t *testing.T) {
	message := EventMessage{
		MessageType: "interactive",
		Content:     `{"title":"日报","body":{"elements":[{"tag":"img","img_key":"img_direct"},{"tag":"file","file_token":"file_direct","file_name":"report.md"}]}}`,
	}
	attachments := ExtractAttachments(message)
	if len(attachments) != 2 {
		t.Fatalf("expected two interactive card resources, got %#v", attachments)
	}
	if attachments[0].ID != "img_direct" || attachments[0].Type != "image" {
		t.Fatalf("unexpected image attachment: %#v", attachments[0])
	}
	if attachments[1].ID != "file_direct" || attachments[1].Type != "file" || attachments[1].Name != "report.md" {
		t.Fatalf("unexpected file attachment: %#v", attachments[1])
	}
}

func TestExtractAttachmentsDecodesInteractiveCardJSONString(t *testing.T) {
	message := EventMessage{
		MessageType: "interactive",
		Content:     `{"user_dsl":"{\"elements\":[{\"tag\":\"img\",\"image_key\":\"img_nested\"}]}"}`,
	}
	attachments := ExtractAttachments(message)
	if len(attachments) != 1 || attachments[0].ID != "img_nested" || attachments[0].Type != "image" {
		t.Fatalf("unexpected nested card attachments: %#v", attachments)
	}
}

func TestExtractAttachmentsExtractsInteractiveCardLinks(t *testing.T) {
	message := EventMessage{
		MessageType: "interactive",
		Content:     `{"elements":[{"tag":"div","text":{"tag":"lark_md","content":"[报告](https://storage.example.com/report.xlsx)"}}]}`,
	}
	attachments := ExtractAttachments(message)
	if len(attachments) != 1 || attachments[0].Type != "link" || attachments[0].URL != "https://storage.example.com/report.xlsx" {
		t.Fatalf("unexpected interactive card links: %#v", attachments)
	}
}
