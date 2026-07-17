package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/multigent/multigent/internal/runtimeguide"
)

func TestUnwrapMCPGatewayTextResult(t *testing.T) {
	body := []byte(`{
		"jsonrpc":"2.0",
		"id":1,
		"result":{"content":[{"type":"text","text":"[{\"id\":\"action:github:get_authenticated_user\"}]"}]}
	}`)
	out, err := unwrapMCPGatewayTextResult(body)
	if err != nil {
		t.Fatalf("unwrap: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != `[{"id":"action:github:get_authenticated_user"}]` {
		t.Fatalf("out=%s", got)
	}
}

func TestUnwrapMCPGatewayTextResultReturnsError(t *testing.T) {
	_, err := unwrapMCPGatewayTextResult([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"denied"}}`))
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("err=%v", err)
	}
}

func TestRuntimeSkillGuideRenderJSON(t *testing.T) {
	body := []byte(`{
		"tools":[{
			"provider":"figma",
			"displayName":"Figma",
			"connectionAlias":"figma",
			"recommendedAdapter":"mcp_gateway",
			"skills":["figma"],
			"adapters":[{"type":"mcp_gateway","priority":90,"skills":["figma"]}]
		}]
	}`)
	guide, err := runtimeguide.RenderJSON(body)
	if err != nil {
		t.Fatalf("render guide: %v", err)
	}
	for _, want := range []string{"Runtime Tool Skills", "Figma", "mga runtime gateway list-tools --provider figma", "figma"} {
		if !strings.Contains(guide, want) {
			t.Fatalf("guide missing %q: %s", want, guide)
		}
	}
}

func TestServeRuntimeMCPStdioForwardsFrames(t *testing.T) {
	const token = "agent-token"
	var forwardedAuth string
	var forwardedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/runtime/mcp/gateway" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		forwardedAuth = r.Header.Get("Authorization")
		body := new(bytes.Buffer)
		_, _ = body.ReadFrom(r.Body)
		forwardedBody = body.String()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`))
	}))
	defer server.Close()
	t.Setenv(envAPIURL, server.URL)
	t.Setenv(envAgentToken, token)

	req := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	var in bytes.Buffer
	if err := writeMCPStdioFrame(&in, req); err != nil {
		t.Fatalf("write input frame: %v", err)
	}
	var out bytes.Buffer
	if err := serveRuntimeMCPStdio(&in, &out); err != nil {
		t.Fatalf("serve MCP stdio: %v", err)
	}
	if forwardedAuth != "Bearer "+token {
		t.Fatalf("authorization=%q", forwardedAuth)
	}
	if strings.TrimSpace(forwardedBody) != string(req) {
		t.Fatalf("forwarded body=%s", forwardedBody)
	}
	resp, err := readMCPStdioFrame(bufioReader(&out))
	if err != nil {
		t.Fatalf("read output frame: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(resp, &decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if decoded["jsonrpc"] != "2.0" {
		t.Fatalf("unexpected response: %s", string(resp))
	}
}

func TestServeRuntimeMCPStdioIgnoresNotifications(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("notification should not be forwarded")
	}))
	defer server.Close()
	t.Setenv(envAPIURL, server.URL)
	t.Setenv(envAgentToken, "agent-token")

	req := []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	var in bytes.Buffer
	if err := writeMCPStdioFrame(&in, req); err != nil {
		t.Fatalf("write input frame: %v", err)
	}
	var out bytes.Buffer
	if err := serveRuntimeMCPStdio(&in, &out); err != nil {
		t.Fatalf("serve MCP stdio: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("notification produced output: %q", out.String())
	}
}

func bufioReader(buf *bytes.Buffer) *bufio.Reader {
	return bufio.NewReader(buf)
}
