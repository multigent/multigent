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
