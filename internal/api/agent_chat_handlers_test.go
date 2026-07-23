package api

import (
	"encoding/json"
	"testing"

	"github.com/multigent/multigent/internal/entity"
)

func TestChatSSELineKeepsCodexTranscriptRaw(t *testing.T) {
	line := "codex"
	got := decodeChatSSEPayload(t, chatSSEPayload(line, entity.ModelCodex))
	if got["type"] != "chat_event" || got["payload"] != line || got["payloadType"] != "cli" {
		t.Fatalf("chatSSEPayload() = %#v", got)
	}
}

func TestChatSSELineWrapsPlainGenericStatus(t *testing.T) {
	got := decodeChatSSEPayload(t, chatSSEPayload("plain status", entity.ModelClaudeCode))
	if got["type"] != "chat_event" || got["payload"] != "=== plain status ===" || got["payloadType"] != "log" {
		t.Fatalf("chatSSEPayload() = %#v", got)
	}
}

func TestExtractAgentChatSessionID(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{
			name: "json",
			line: `{"type":"system","session_id":"abc123"}`,
			want: "abc123",
		},
		{
			name: "codex label",
			line: "Session ID: sess-456",
			want: "sess-456",
		},
		{
			name: "multiline log",
			line: "header\nSession: sess-789\nfooter",
			want: "sess-789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractAgentChatSessionID(tt.line); got != tt.want {
				t.Fatalf("extractAgentChatSessionID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractAgentChatError(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{
			name: "codex error event",
			line: `{"type":"error","message":"unexpected status 401 Unauthorized"}`,
			want: "unexpected status 401 Unauthorized",
		},
		{
			name: "codex turn failed",
			line: `{"type":"turn.failed","error":{"message":"Missing bearer or basic authentication in header"}}`,
			want: "Missing bearer or basic authentication in header",
		},
		{
			name: "codex item completed error",
			line: `{"type":"item.completed","item":{"type":"error","message":"Falling back from WebSockets"}}`,
			want: "Falling back from WebSockets",
		},
		{
			name: "plain log",
			line: `exit status 1`,
			want: "",
		},
		{
			name: "docker registry error",
			line: `docker: Error response from daemon: error from registry: unauthorized`,
			want: `docker: Error response from daemon: error from registry: unauthorized`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractAgentChatError(tt.line); got != tt.want {
				t.Fatalf("extractAgentChatError() = %q, want %q", got, tt.want)
			}
		})
	}
}

func decodeChatSSEPayload(t *testing.T, raw string) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("payload is not JSON: %v\n%s", err, raw)
	}
	return got
}
