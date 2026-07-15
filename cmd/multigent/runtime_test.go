package main

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRuntimeConnectionTargetFromMaterializedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "connections.json")
	body := []byte(`{"connections":[{"id":"conn_123","provider":"custom-mcp","connectionName":"docs","runtime":{"alias":"custom-mcp_docs"}}]}`)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	t.Setenv(runtimeEnvConnectionsFile, path)

	target, err := resolveRuntimeConnectionTarget("custom-mcp_docs")
	if err != nil {
		t.Fatalf("resolve alias: %v", err)
	}
	if target.Alias != "custom-mcp_docs" || target.ID != "" {
		t.Fatalf("target=%#v", target)
	}

	target, err = resolveRuntimeConnectionTarget("conn_123")
	if err != nil {
		t.Fatalf("resolve id: %v", err)
	}
	if target.ID != "conn_123" || target.Alias != "" {
		t.Fatalf("target=%#v", target)
	}
}

func TestResolveRuntimeConnectionTargetFallback(t *testing.T) {
	t.Setenv(runtimeEnvConnectionsFile, "")
	if got, err := resolveRuntimeConnectionTarget("conn_abc"); err != nil || got.ID != "conn_abc" {
		t.Fatalf("id fallback target=%#v err=%v", got, err)
	}
	if got, err := resolveRuntimeConnectionTarget("custom-mcp_docs"); err != nil || got.Alias != "custom-mcp_docs" {
		t.Fatalf("alias fallback target=%#v err=%v", got, err)
	}
}

func TestRuntimeConnectionTargetQuery(t *testing.T) {
	tests := []struct {
		name string
		in   runtimeConnectionTarget
		want url.Values
	}{
		{name: "id", in: runtimeConnectionTarget{ID: "conn_1"}, want: url.Values{"connection": []string{"conn_1"}}},
		{name: "alias", in: runtimeConnectionTarget{Alias: "custom-mcp_docs"}, want: url.Values{"alias": []string{"custom-mcp_docs"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.in.query()
			if got.Encode() != tt.want.Encode() {
				t.Fatalf("query=%s want=%s", got.Encode(), tt.want.Encode())
			}
		})
	}
}

func TestReadRuntimeRequestBodyValidatesJSON(t *testing.T) {
	if _, err := readRuntimeRequestBody(`{"jsonrpc":"2.0"}`, ""); err != nil {
		t.Fatalf("valid json: %v", err)
	}
	if _, err := readRuntimeRequestBody(`{`, ""); err == nil {
		t.Fatalf("expected invalid json error")
	}
	if _, err := readRuntimeRequestBody(`{}`, "-"); err == nil {
		t.Fatalf("expected mutually exclusive input error")
	}
}
