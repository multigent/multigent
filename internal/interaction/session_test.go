package interaction

import (
	"errors"
	"testing"
)

func TestManagerAllowsOneActiveSessionPerConversationSource(t *testing.T) {
	m := NewManager()
	m.nextIDFn = func() string { return "sess-one" }
	agent := AgentRef{WorkspaceID: "ws", ProjectID: "project", AgentID: "pm"}
	source := Source{Kind: "web_chat", ActorID: "owner", Channel: "web"}

	session, lease, err := m.Acquire(agent, source, "interactive")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if session.ID != "sess-one" || session.Source.Kind != "web_chat" {
		t.Fatalf("session=%#v", session)
	}
	if _, _, err := m.Acquire(agent, source, "interactive"); !errors.Is(err, ErrAgentLocked) {
		t.Fatalf("same source acquire err=%v", err)
	}
	if _, second, err := m.Acquire(agent, Source{Kind: "lark", ActorID: "ou_one", Channel: "oc_one"}, "interactive"); err != nil {
		t.Fatalf("different source should not conflict: %v", err)
	} else {
		second.Release()
	}
	lease.Release()
	if _, _, err := m.Acquire(agent, Source{Kind: "lark", ActorID: "ou_one"}, "interactive"); err != nil {
		t.Fatalf("reacquire after release: %v", err)
	}
}

func TestManagerScopesLocksByWorkspaceProjectAgent(t *testing.T) {
	m := NewManager()
	if _, _, err := m.Acquire(AgentRef{WorkspaceID: "ws-a", ProjectID: "project", AgentID: "pm"}, Source{Kind: "web_chat"}, "interactive"); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if _, _, err := m.Acquire(AgentRef{WorkspaceID: "ws-b", ProjectID: "project", AgentID: "pm"}, Source{Kind: "web_chat"}, "interactive"); err != nil {
		t.Fatalf("workspace-scoped acquire should not conflict: %v", err)
	}
	if _, _, err := m.Acquire(AgentRef{WorkspaceID: "ws-a", ProjectID: "other", AgentID: "pm"}, Source{Kind: "web_chat"}, "interactive"); err != nil {
		t.Fatalf("project-scoped acquire should not conflict: %v", err)
	}
}
