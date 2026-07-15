package api

import (
	"encoding/json"
	"strings"
	"testing"

	controldb "github.com/multigent/multigent/internal/db"
)

func TestConnectionResponseSanitizesProfileSecrets(t *testing.T) {
	connection := controldb.Connection{
		ID:             "conn-one",
		Provider:       "custom-mcp",
		ConnectionName: "tools",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        "ws-one",
		AuthType:       ConnectionAuthCustomCredential,
		Status:         "active",
		ProfileJSON:    `{"displayName":"Tools","serverUrl":"http://localhost:3000/mcp","token":"secret-token","appSecret":"secret-app","credential":"secret-credential"}`,
		CreatedBy:      "admin",
	}
	resp := connectionToResponse(connection, nil)
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	body := string(raw)
	for _, forbidden := range []string{"secret-token", "secret-app", "secret-credential", "appSecret"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("connection response leaked %q: %s", forbidden, body)
		}
	}
	if _, ok := resp.Profile["credential"]; ok {
		t.Fatalf("credential key was not removed from profile: %#v", resp.Profile)
	}
	if resp.Profile["displayName"] != "Tools" || resp.Profile["serverUrl"] != "http://localhost:3000/mcp" {
		t.Fatalf("safe profile fields not preserved: %#v", resp.Profile)
	}
}

func TestConnectionAuditPayloadSanitizesProfileSecrets(t *testing.T) {
	connection := controldb.Connection{
		ID:             "conn-one",
		Provider:       "github",
		ConnectionName: "default",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        "ws-one",
		AuthType:       ConnectionAuthAPIKey,
		Status:         "active",
		ProfileJSON:    `{"displayName":"GitHub","apiKey":"ghp_secret","token":"secret-token"}`,
		CreatedBy:      "admin",
	}
	payload := connectionAuditPayload(connection)
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	body := string(raw)
	for _, forbidden := range []string{"ghp_secret", "secret-token", "apiKey"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("audit payload leaked %q: %s", forbidden, body)
		}
	}
	profile, ok := payload["profile"].(map[string]any)
	if !ok || profile["displayName"] != "GitHub" {
		t.Fatalf("safe audit profile not preserved: %#v", payload["profile"])
	}
}
