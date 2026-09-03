package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/multigent/multigent/internal/agentdir"
	controldb "github.com/multigent/multigent/internal/db"
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

func TestSummarizeSessionTitleFromLog(t *testing.T) {
	log := strings.Join([]string{
		`{"type":"system","session_id":"sess-one"}`,
		`{"type":"human","content":"请帮我看一下这个任务为什么失败"}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"我来看一下。"}]}}`,
	}, "\n")
	if got := summarizeSessionTitleFromLog(log); got != "请帮我看一下这个任务为什么失败" {
		t.Fatalf("summarizeSessionTitleFromLog() = %q", got)
	}
}

func TestSummarizeSessionTitleFromClaudeUserLog(t *testing.T) {
	log := `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"检查本地文件"}]},"session_id":"sess-two"}`
	if got := summarizeSessionTitleFromLog(log); got != "检查本地文件" {
		t.Fatalf("summarizeSessionTitleFromLog() = %q", got)
	}
}

func TestSummarizeSessionTitleFromCodexEventMsgUserLog(t *testing.T) {
	log := `{"type":"event_msg","payload":{"type":"item_completed","thread_id":"01a01f42-4128-74d3-8c6c-af980283229a","item":{"type":"UserMessage","text":"回顾一下 MCP Server 的 OAuth 接入"}}}`
	if got := summarizeSessionTitleFromLog(log); got != "回顾一下 MCP Server 的 OAuth 接入" {
		t.Fatalf("summarizeSessionTitleFromLog() = %q", got)
	}
}

func TestNativeSessionIDFromCodexRolloutPath(t *testing.T) {
	path := "/tmp/.codex/sessions/2026/08/20/rollout-2026-08-20T13-00-30-01a01f42-4128-74d3-8c6c-af980283229a.jsonl"
	if got := nativeSessionIDFromPath(path); got != "01a01f42-4128-74d3-8c6c-af980283229a" {
		t.Fatalf("nativeSessionIDFromPath() = %q", got)
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
		{
			name: "windows docker daemon error",
			line: `docker: error during connect: in the default daemon configuration on Windows, the docker client must be run with elevated privileges to connect: Post "http://%2F%2F.%2Fpipe%2Fdocker_engine/v1.24/containers/create": open //./pipe/docker_engine: The system cannot find the file specified.`,
			want: `docker: error during connect: in the default daemon configuration on Windows, the docker client must be run with elevated privileges to connect: Post "http://%2F%2F.%2Fpipe%2Fdocker_engine/v1.24/containers/create": open //./pipe/docker_engine: The system cannot find the file specified.`,
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

func TestExtractAgentChatReplyPrefersFinalResult(t *testing.T) {
	output := strings.Join([]string{
		"▶  exec example-project/pm",
		"multigent: preparing runtime tool github",
		`{"type":"system","subtype":"init","session_id":"sess-one"}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"draft reply"}]}}`,
		`{"type":"result","is_error":false,"result":"final reply"}`,
	}, "\n")
	if got := extractAgentChatReply(output); got != "final reply" {
		t.Fatalf("extractAgentChatReply() = %q", got)
	}
}

func TestExtractAgentChatReplyFallsBackToAssistantText(t *testing.T) {
	output := strings.Join([]string{
		"runtime setup log",
		`{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"hidden"},{"type":"text","text":"visible reply"}]}}`,
	}, "\n")
	if got := extractAgentChatReply(output); got != "visible reply" {
		t.Fatalf("extractAgentChatReply() = %q", got)
	}
}

func TestExtractAgentChatReplyFromCodexAgentMessage(t *testing.T) {
	output := strings.Join([]string{
		`{"type":"item.started","item":{"type":"command_execution","command":"pwd","status":"in_progress"}}`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"结论：manager-agent 可以正常响应。"}}`,
	}, "\n")
	if got := extractAgentChatReply(output); got != "结论：manager-agent 可以正常响应。" {
		t.Fatalf("extractAgentChatReply() = %q", got)
	}
}

func TestExtractAgentChatReplyFromContentBlock(t *testing.T) {
	output := strings.Join([]string{
		`{"type":"content","content_block":{"type":"text","text":"已经处理完成。"}}`,
	}, "\n")
	if got := extractAgentChatReply(output); got != "已经处理完成。" {
		t.Fatalf("extractAgentChatReply() = %q", got)
	}
}

func TestLocalRuntimeAPIURLForRequestUsesLoopbackPort(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "http://203.0.113.10:27892/api/v1/projects/p/agents/a/chat", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "203.0.113.10:27892"
	if got := localRuntimeAPIURLForRequest(req); got != "http://127.0.0.1:27892" {
		t.Fatalf("localRuntimeAPIURLForRequest() = %q", got)
	}
}

func TestAgentChatSessionsIncludeWorkerBackedRuntimeRuns(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.controlDB.UpsertAgentWorker(controldb.AgentWorker{
		ID:          "aw-manager-agent",
		WorkspaceID: workspaceID,
		Name:        "manager-agent",
		DisplayName: "manager-agent",
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("worker: %v", err)
	}
	if err := s.controlDB.UpsertProjectMembership(controldb.ProjectMembership{
		ID:               "pm-sample-manager-agent",
		WorkspaceID:      workspaceID,
		ProjectID:        "sample",
		MemberType:       agentdir.MemberTypeAgentWorker,
		MemberID:         "aw-manager-agent",
		Role:             "pm",
		Title:            "pm",
		AutoPickTasks:    true,
		AttentionEnabled: true,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("membership: %v", err)
	}
	resultJSON, _ := json.Marshal(map[string]string{
		"sessionId": "sess-worker-only",
		"logText":   `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"worker backed chat"}]},"session_id":"sess-worker-only"}`,
	})
	if err := s.controlDB.UpsertRuntimeRun(controldb.RuntimeRun{
		ID:                  "run-worker-only",
		WorkspaceID:         workspaceID,
		AgentWorkerID:       "aw-manager-agent",
		ProjectMembershipID: "pm-sample-manager-agent",
		ProjectID:           "sample",
		Status:              "succeeded",
		SpecJSON:            `{}`,
		ResultJSON:          string(resultJSON),
		CreatedAt:           now,
		UpdatedAt:           now,
		StartedAt:           now,
		FinishedAt:          now,
	}); err != nil {
		t.Fatalf("runtime run: %v", err)
	}

	sessions, err := s.listAgentChatSessions(workspaceID, "sample", "pm")
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].SessionID != "sess-worker-only" {
		t.Fatalf("unexpected sessions: %#v", sessions)
	}
	history, resolved, err := s.readRuntimeNodeAgentSessionHistory(workspaceID, "sample", "pm", "sess-worker-only", 5)
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	if resolved != "sess-worker-only" || len(history) != 1 || !strings.Contains(string(history[0].data), "worker backed chat") {
		t.Fatalf("unexpected history resolved=%q history=%#v", resolved, history)
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
