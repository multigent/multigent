package lark

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestReplyMarkdownUsesPostReplyAPI(t *testing.T) {
	var replyBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "tenant-token"})
		case "/open-apis/im/v1/messages/om_one/reply":
			if got := r.Header.Get("Authorization"); got != "Bearer tenant-token" {
				t.Fatalf("unexpected auth header: %s", got)
			}
			if err := json.NewDecoder(r.Body).Decode(&replyBody); err != nil {
				t.Fatalf("decode reply body: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := OpenAPIClient{BaseURL: server.URL, AppID: "cli_app", AppSecret: "secret", HTTPClient: server.Client()}
	if err := client.ReplyMarkdown(context.Background(), "om_one", "Nova", "## 结论\n\n- 已处理"); err != nil {
		t.Fatalf("reply markdown: %v", err)
	}
	if replyBody["msg_type"] != "post" {
		t.Fatalf("msg_type=%#v, want post", replyBody["msg_type"])
	}
	content, _ := replyBody["content"].(string)
	var post map[string]any
	if err := json.Unmarshal([]byte(content), &post); err != nil {
		t.Fatalf("content is not post JSON: %v", err)
	}
	zh, _ := post["zh_cn"].(map[string]any)
	if zh == nil {
		t.Fatalf("missing zh_cn post body: %#v", post)
	}
	if zh["title"] != "Nova" {
		t.Fatalf("unexpected post title: %#v", zh["title"])
	}
	contentRows, _ := zh["content"].([]any)
	if len(contentRows) != 1 {
		t.Fatalf("unexpected post rows: %#v", zh["content"])
	}
	row, _ := contentRows[0].([]any)
	if len(row) != 1 {
		t.Fatalf("unexpected post row: %#v", contentRows[0])
	}
	element, _ := row[0].(map[string]any)
	if element["tag"] != "md" || !strings.Contains(element["text"].(string), "## 结论") {
		t.Fatalf("final reply should render markdown as post md, got %#v", element)
	}
}

func TestProgressCardCollapsesReasoningWhenCompleted(t *testing.T) {
	card := buildProgressCardBody(ProgressCard{
		Title: "Multigent · nova",
		State: "completed",
		Reasoning: []ProgressCardEntry{
			{Kind: "thinking", Title: "Reasoning", Content: "long internal reasoning"},
		},
		Final: "## 结论\n\n- 已完成",
	})
	body, _ := card["body"].(map[string]any)
	elements, _ := body["elements"].([]map[string]any)
	if len(elements) == 0 {
		t.Fatalf("missing elements: %#v", body)
	}
	panel := elements[0]
	if panel["tag"] != "collapsible_panel" {
		t.Fatalf("first element should be reasoning panel: %#v", panel)
	}
	if panel["expanded"] != false {
		t.Fatalf("completed reasoning panel should be collapsed: %#v", panel["expanded"])
	}
	for _, element := range elements {
		if element["tag"] == "markdown" {
			t.Fatalf("progress card should not embed final markdown: %#v", element)
		}
	}
}

func TestProgressCardCollapsesLongRunningReasoning(t *testing.T) {
	long := ""
	for i := 0; i < 1000; i++ {
		long += "会"
	}
	card := buildProgressCardBody(ProgressCard{
		Title: "Multigent · nova",
		State: "running",
		Reasoning: []ProgressCardEntry{
			{Kind: "thinking", Title: "Reasoning", Content: long},
		},
	})
	body, _ := card["body"].(map[string]any)
	elements, _ := body["elements"].([]map[string]any)
	if len(elements) == 0 {
		t.Fatalf("missing elements: %#v", body)
	}
	if elements[0]["expanded"] != false {
		t.Fatalf("long running reasoning panel should be collapsed: %#v", elements[0]["expanded"])
	}
}
