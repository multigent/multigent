package main

import (
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
