package lark

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUpdateInteractiveCardUsesDelayedUpdateAPI(t *testing.T) {
	var updateBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "tenant-token"})
		case "/open-apis/interactive/v1/card/update":
			if got := r.Header.Get("Authorization"); got != "Bearer tenant-token" {
				t.Fatalf("unexpected auth header: %s", got)
			}
			if err := json.NewDecoder(r.Body).Decode(&updateBody); err != nil {
				t.Fatalf("decode update body: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := OpenAPIClient{BaseURL: server.URL, AppID: "cli_app", AppSecret: "secret", HTTPClient: server.Client()}
	if err := client.UpdateInteractiveCard(context.Background(), "update-token", "ou_user", InteractiveCard{
		Title: "已提交",
		Body:  "正在处理",
		Fields: []InteractiveCardField{
			{Label: "选择", Value: "通过"},
		},
	}); err != nil {
		t.Fatalf("update card: %v", err)
	}
	if updateBody["token"] != "update-token" {
		t.Fatalf("unexpected token: %#v", updateBody["token"])
	}
	card, _ := updateBody["card"].(map[string]any)
	if card == nil {
		t.Fatalf("missing card in body: %#v", updateBody)
	}
	openIDs, _ := card["open_ids"].([]any)
	if len(openIDs) != 1 || openIDs[0] != "ou_user" {
		t.Fatalf("unexpected open_ids: %#v", card["open_ids"])
	}
}
