package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	controldb "github.com/multigent/multigent/internal/db"
)

func TestRuntimeMCPProxyForwardsCustomMCPWithServerSideToken(t *testing.T) {
	users := newTestUserStore(t)
	s := &Server{controlDB: users.db, users: users}
	workspaceID := "ws-one"
	var upstreamAuth string
	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
			t.Fatalf("decode upstream body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]any{
				"tools": []any{},
				"auth":  upstreamAuth,
			},
		})
	}))
	defer upstream.Close()

	connection := controldb.Connection{
		ID:             "conn-mcp",
		WorkspaceID:    workspaceID,
		Provider:       "custom-mcp",
		ConnectionName: "tools",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       ConnectionAuthCustomCredential,
		Status:         "active",
		ProfileJSON:    "{}",
		CreatedBy:      "admin",
	}
	if err := users.db.UpsertWorkspace(controldb.Workspace{ID: workspaceID, Name: "One", Slug: "one"}); err != nil {
		t.Fatalf("workspace: %v", err)
	}
	if err := users.db.UpsertConnection(connection); err != nil {
		t.Fatalf("connection: %v", err)
	}
	secret, err := sealConnectionSecret(map[string]string{"serverUrl": upstream.URL, "token": "mcp-token"})
	if err != nil {
		t.Fatalf("seal secret: %v", err)
	}
	secret.ConnectionID = connection.ID
	if err := users.db.UpsertConnectionSecret(secret); err != nil {
		t.Fatalf("secret: %v", err)
	}
	if err := users.db.CreateConnectionGrant(controldb.ConnectionGrant{
		ID:           "grant-mcp",
		WorkspaceID:  workspaceID,
		ConnectionID: connection.ID,
		TargetType:   ConnectionTargetAgent,
		TargetID:     "tapnow/pm",
	}); err != nil {
		t.Fatalf("grant: %v", err)
	}

	principal := runtimeAgentPrincipal{
		WorkspaceID:  workspaceID,
		Project:      "tapnow",
		Agent:        "pm",
		RunID:        "run-one",
		Capabilities: []string{"connection.use"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/mcp", stringsReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	req.Header.Set(agentConnectionManifest().ConnectionIDHeader, connection.ID)
	req = req.WithContext(context.WithValue(req.Context(), ctxRuntimeAgentKey, principal))
	rec := httptest.NewRecorder()

	s.handleRuntimeMCPProxy(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if upstreamAuth != "Bearer mcp-token" {
		t.Fatalf("upstream auth=%q", upstreamAuth)
	}
	if upstreamBody["method"] != "tools/list" {
		t.Fatalf("upstream body=%#v", upstreamBody)
	}
	if body := rec.Body.String(); body == "" || containsAny(body, []string{"mcp-token", "serverUrl"}) {
		t.Fatalf("proxy response leaked sensitive data or is empty: %s", body)
	}
	if !strings.Contains(rec.Body.String(), "Bearer [redacted]") {
		t.Fatalf("redacted authorization marker missing: %s", rec.Body.String())
	}
}

func TestCustomMCPRuntimeConfigSupportsNoAuthProfileURL(t *testing.T) {
	users := newTestUserStore(t)
	s := &Server{controlDB: users.db, users: users}
	cfg, err := s.customMCPRuntimeConfig(controldb.Connection{
		ID:             "conn-no-auth",
		Provider:       "custom-mcp",
		ConnectionName: "default",
		AuthType:       ConnectionAuthNoAuth,
		ProfileJSON:    `{"serverUrl":"http://127.0.0.1:3000/mcp","token":"should-not-be-used"}`,
	})
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if cfg.ServerURL != "http://127.0.0.1:3000/mcp" {
		t.Fatalf("serverURL=%q", cfg.ServerURL)
	}
	if cfg.Token != "" {
		t.Fatalf("no_auth profile token should not be used: %q", cfg.Token)
	}
}

func stringsReader(s string) *strings.Reader {
	return strings.NewReader(s)
}

func containsAny(s string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
