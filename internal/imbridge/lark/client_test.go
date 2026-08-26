package lark

import (
	"context"
	"encoding/json"
	"io"
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

func TestInteractiveCardBodyUsesSchema2MarkdownElements(t *testing.T) {
	card := buildInteractiveCardBody(InteractiveCard{
		Title: "评审",
		Body:  "## 结论\n\n- 可合并\n\n[文档](https://example.com/docs/doc-1)",
		Fields: []InteractiveCardField{
			{Label: "风险", Value: "低"},
		},
		Actions: []InteractiveCardAction{{ID: "approve", Label: "通过", Style: "primary"}},
	}, nil)
	if card["schema"] != "2.0" {
		t.Fatalf("interactive card should use schema 2.0, got %#v", card["schema"])
	}
	body, _ := card["body"].(map[string]any)
	elements, _ := body["elements"].([]map[string]any)
	if len(elements) == 0 {
		t.Fatalf("missing body elements: %#v", card)
	}
	first := elements[0]
	if first["tag"] != "markdown" {
		t.Fatalf("body should render markdown directly, got %#v", first)
	}
	if content, _ := first["content"].(string); !strings.Contains(content, "## 结论") || !strings.Contains(content, "[文档]") {
		t.Fatalf("markdown content not preserved: %#v", first)
	}
}

func TestInteractiveCardBodyCanUseRawCardJSON(t *testing.T) {
	raw := json.RawMessage(`{"schema":"2.0","config":{"wide_screen_mode":true},"header":{"template":"green","title":{"tag":"plain_text","content":"PASS"}},"body":{"elements":[{"tag":"markdown","content":"**OK**"}]}}`)
	card := buildInteractiveCardBody(InteractiveCard{RawJSON: raw}, nil)
	header, _ := card["header"].(map[string]any)
	title, _ := header["title"].(map[string]any)
	if card["schema"] != "2.0" || header["template"] != "green" || title["content"] != "PASS" {
		t.Fatalf("raw card was not preserved: %#v", card)
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

func TestDownloadMessageResourceUsesMessageResourceAPI(t *testing.T) {
	resourceBody := []byte("png-bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "tenant-token"})
		case "/open-apis/im/v1/messages/om_one/resources/img_one":
			if got := r.Header.Get("Authorization"); got != "Bearer tenant-token" {
				t.Fatalf("unexpected auth header: %s", got)
			}
			if got := r.URL.Query().Get("type"); got != "image" {
				t.Fatalf("unexpected type query: %s", got)
			}
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("Content-Disposition", `attachment; filename="logo.png"`)
			_, _ = w.Write(resourceBody)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := OpenAPIClient{BaseURL: server.URL, AppID: "cli_app", AppSecret: "secret", HTTPClient: server.Client()}
	got, err := client.DownloadMessageResource(context.Background(), "om_one", "img_one", "image")
	if err != nil {
		t.Fatalf("download resource: %v", err)
	}
	if got.FileName != "logo.png" || got.MIME != "image/png" || string(got.Data) != string(resourceBody) {
		t.Fatalf("unexpected download: %#v data=%q", got, string(got.Data))
	}
}

func TestMarkdownPostBodyOmitsEmptyTitle(t *testing.T) {
	content := buildMarkdownPostBody("", "## 结论\n\n- 已处理")
	var post map[string]any
	if err := json.Unmarshal([]byte(content), &post); err != nil {
		t.Fatalf("content is not post JSON: %v", err)
	}
	zh, _ := post["zh_cn"].(map[string]any)
	if zh == nil {
		t.Fatalf("missing zh_cn post body: %#v", post)
	}
	if _, ok := zh["title"]; ok {
		t.Fatalf("empty title should be omitted: %#v", zh["title"])
	}
}

func TestGetUserReturnsEmailFields(t *testing.T) {
	var gotPath string
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "tenant-token"})
		case "/open-apis/contact/v3/users/ou_one":
			gotPath = r.URL.RawQuery
			authHeader = r.Header.Get("Authorization")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"user": map[string]any{
					"open_id":          "ou_one",
					"user_id":          "u_one",
					"union_id":         "on_one",
					"name":             "Glenn",
					"email":            "glenn@example.com",
					"enterprise_email": "glenn@company.com",
				}},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.String())
		}
	}))
	defer server.Close()

	client := OpenAPIClient{BaseURL: server.URL, AppID: "cli_app", AppSecret: "secret", HTTPClient: server.Client()}
	profile, err := client.GetUser(context.Background(), "ou_one", "open_id")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if gotPath != "user_id_type=open_id" {
		t.Fatalf("unexpected query: %s", gotPath)
	}
	if profile.Email != "glenn@example.com" || profile.EnterpriseEmail != "glenn@company.com" || profile.OpenID != "ou_one" {
		t.Fatalf("unexpected profile: %#v", profile)
	}
	if authHeader != "Bearer tenant-token" {
		t.Fatalf("unexpected auth header: %s", authHeader)
	}
}

func TestGetUserUsesOAuthAccessTokenWhenPresent(t *testing.T) {
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			t.Fatalf("tenant token endpoint should not be called when access token is present")
		case "/open-apis/contact/v3/users/on_one":
			authHeader = r.Header.Get("Authorization")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"user": map[string]any{
					"open_id": "ou_one",
					"name":    "Glenn",
					"email":   "glenn@example.com",
				}},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.String())
		}
	}))
	defer server.Close()

	client := OpenAPIClient{BaseURL: server.URL, AppID: "cli_app", AppSecret: "secret", AccessToken: "user-access-token", HTTPClient: server.Client()}
	profile, err := client.GetUser(context.Background(), "on_one", "union_id")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if profile.Email != "glenn@example.com" || profile.OpenID != "ou_one" {
		t.Fatalf("unexpected profile: %#v", profile)
	}
	if authHeader != "Bearer user-access-token" {
		t.Fatalf("unexpected auth header: %s", authHeader)
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

func TestAttachmentTypeInference(t *testing.T) {
	cases := []struct {
		name        string
		mimeType    string
		wantKind    string
		wantFile    string
		wantMessage string
	}{
		{name: "screenshot.png", mimeType: "image/png", wantKind: "image", wantFile: "stream", wantMessage: "file"},
		{name: "report.pdf", mimeType: "application/pdf", wantKind: "file", wantFile: "pdf", wantMessage: "file"},
		{name: "clip.mp4", mimeType: "video/mp4", wantKind: "file", wantFile: "mp4", wantMessage: "media"},
		{name: "voice.opus", mimeType: "audio/opus", wantKind: "file", wantFile: "opus", wantMessage: "audio"},
		{name: "debug.log", mimeType: "text/plain", wantKind: "file", wantFile: "stream", wantMessage: "file"},
	}
	for _, tc := range cases {
		if got := inferAttachmentKind(tc.name, tc.mimeType); got != tc.wantKind {
			t.Fatalf("%s kind=%s, want %s", tc.name, got, tc.wantKind)
		}
		fileType := detectLarkFileType(tc.mimeType, tc.name)
		if fileType != tc.wantFile {
			t.Fatalf("%s fileType=%s, want %s", tc.name, fileType, tc.wantFile)
		}
		if got := larkFileMessageType(fileType); got != tc.wantMessage {
			t.Fatalf("%s messageType=%s, want %s", tc.name, got, tc.wantMessage)
		}
	}
}

func TestReplyAttachmentUploadsFileThenReplies(t *testing.T) {
	var replyBody map[string]any
	uploaded := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "tenant-token"})
		case "/open-apis/im/v1/files":
			if got := r.Header.Get("Authorization"); got != "Bearer tenant-token" {
				t.Fatalf("unexpected upload auth header: %s", got)
			}
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatalf("parse upload form: %v", err)
			}
			if r.FormValue("file_type") != "pdf" || r.FormValue("file_name") != "report.pdf" {
				t.Fatalf("unexpected upload fields: file_type=%q file_name=%q", r.FormValue("file_type"), r.FormValue("file_name"))
			}
			file, _, err := r.FormFile("file")
			if err != nil {
				t.Fatalf("uploaded file missing: %v", err)
			}
			raw, _ := io.ReadAll(file)
			_ = file.Close()
			if string(raw) != "pdf-bytes" {
				t.Fatalf("unexpected uploaded file body: %q", string(raw))
			}
			uploaded = true
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"file_key": "file_v2_key"}})
		case "/open-apis/im/v1/messages/om_one/reply":
			if !uploaded {
				t.Fatalf("reply called before upload")
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
	if err := client.ReplyAttachment(context.Background(), "om_one", OutgoingAttachment{
		Kind:     "file",
		FileName: "report.pdf",
		MIME:     "application/pdf",
		Data:     []byte("pdf-bytes"),
	}); err != nil {
		t.Fatalf("reply attachment: %v", err)
	}
	if replyBody["msg_type"] != "file" {
		t.Fatalf("msg_type=%#v, want file", replyBody["msg_type"])
	}
	var content map[string]string
	if err := json.Unmarshal([]byte(replyBody["content"].(string)), &content); err != nil {
		t.Fatalf("content JSON: %v", err)
	}
	if content["file_key"] != "file_v2_key" {
		t.Fatalf("unexpected file key: %#v", content)
	}
}
