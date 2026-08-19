package lark

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OpenAPIClient struct {
	BaseURL    string
	AppID      string
	AppSecret  string
	HTTPClient *http.Client
}

type InteractiveCard struct {
	InteractionID string
	Title         string
	Body          string
	Fields        []InteractiveCardField
	Actions       []InteractiveCardAction
	Links         []InteractiveCardLink
}

type InteractiveCardField struct {
	Label string
	Value string
}

type InteractiveCardAction struct {
	ID           string
	Label        string
	Style        string
	RequiresText bool
}

type InteractiveCardLink struct {
	Label string
	URL   string
}

func (c OpenAPIClient) ReplyText(ctx context.Context, messageID, text string) error {
	messageID = strings.TrimSpace(messageID)
	text = strings.TrimSpace(text)
	if messageID == "" {
		return fmt.Errorf("message id is required")
	}
	if text == "" {
		text = "(empty response)"
	}
	token, err := c.tenantAccessToken(ctx)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]string{
		"msg_type": "text",
		"content":  mustJSON(map[string]string{"text": text}),
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.openBaseURL(), "/")+"/open-apis/im/v1/messages/"+messageID+"/reply", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("reply message http %d: %s", resp.StatusCode, string(raw))
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if json.Unmarshal(raw, &parsed) == nil && parsed.Code != 0 {
		return fmt.Errorf("reply message failed: code=%d msg=%s", parsed.Code, parsed.Msg)
	}
	return nil
}

func (c OpenAPIClient) SendText(ctx context.Context, receiveIDType, receiveID, text string) error {
	receiveIDType = strings.TrimSpace(receiveIDType)
	receiveID = strings.TrimSpace(receiveID)
	text = strings.TrimSpace(text)
	if receiveIDType == "" {
		receiveIDType = "open_id"
	}
	if receiveID == "" {
		return fmt.Errorf("receive id is required")
	}
	if text == "" {
		text = "(empty message)"
	}
	token, err := c.tenantAccessToken(ctx)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]string{
		"receive_id": receiveID,
		"msg_type":   "text",
		"content":    mustJSON(map[string]string{"text": text}),
	})
	u := strings.TrimRight(c.openBaseURL(), "/") + "/open-apis/im/v1/messages?receive_id_type=" + receiveIDType
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("send message http %d: %s", resp.StatusCode, string(raw))
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if json.Unmarshal(raw, &parsed) == nil && parsed.Code != 0 {
		return fmt.Errorf("send message failed: code=%d msg=%s", parsed.Code, parsed.Msg)
	}
	return nil
}

func (c OpenAPIClient) SendMarkdown(ctx context.Context, receiveIDType, receiveID, title, markdown string) error {
	receiveIDType = strings.TrimSpace(receiveIDType)
	receiveID = strings.TrimSpace(receiveID)
	title = strings.TrimSpace(title)
	markdown = strings.TrimSpace(markdown)
	if receiveIDType == "" {
		receiveIDType = "open_id"
	}
	if receiveID == "" {
		return fmt.Errorf("receive id is required")
	}
	if title == "" {
		title = "Multigent notification"
	}
	if markdown == "" {
		markdown = "(empty message)"
	}
	token, err := c.tenantAccessToken(ctx)
	if err != nil {
		return err
	}
	card := map[string]any{
		"config": map[string]any{"wide_screen_mode": true},
		"header": map[string]any{
			"title": map[string]string{"tag": "plain_text", "content": title},
		},
		"elements": []map[string]any{
			{"tag": "markdown", "content": markdown},
		},
	}
	body, _ := json.Marshal(map[string]string{
		"receive_id": receiveID,
		"msg_type":   "interactive",
		"content":    mustJSON(card),
	})
	u := strings.TrimRight(c.openBaseURL(), "/") + "/open-apis/im/v1/messages?receive_id_type=" + receiveIDType
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("send markdown message http %d: %s", resp.StatusCode, string(raw))
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if json.Unmarshal(raw, &parsed) == nil && parsed.Code != 0 {
		return fmt.Errorf("send markdown message failed: code=%d msg=%s", parsed.Code, parsed.Msg)
	}
	return nil
}

func (c OpenAPIClient) SendInteractiveCard(ctx context.Context, receiveIDType, receiveID string, card InteractiveCard) error {
	receiveIDType = strings.TrimSpace(receiveIDType)
	receiveID = strings.TrimSpace(receiveID)
	if receiveIDType == "" {
		receiveIDType = "open_id"
	}
	if receiveID == "" {
		return fmt.Errorf("receive id is required")
	}
	if strings.TrimSpace(card.Title) == "" {
		card.Title = "Multigent"
	}
	if strings.TrimSpace(card.Body) == "" {
		card.Body = "请选择一个操作。"
	}
	if strings.TrimSpace(card.InteractionID) == "" {
		return fmt.Errorf("interaction id is required")
	}
	token, err := c.tenantAccessToken(ctx)
	if err != nil {
		return err
	}
	cardBody := buildInteractiveCardBody(card, nil)
	body, _ := json.Marshal(map[string]string{
		"receive_id": receiveID,
		"msg_type":   "interactive",
		"content":    mustJSON(cardBody),
	})
	u := strings.TrimRight(c.openBaseURL(), "/") + "/open-apis/im/v1/messages?receive_id_type=" + receiveIDType
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("send interactive card http %d: %s", resp.StatusCode, string(raw))
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if json.Unmarshal(raw, &parsed) == nil && parsed.Code != 0 {
		return fmt.Errorf("send interactive card failed: code=%d msg=%s", parsed.Code, parsed.Msg)
	}
	return nil
}

func (c OpenAPIClient) UpdateInteractiveCard(ctx context.Context, updateToken, operatorOpenID string, card InteractiveCard) error {
	updateToken = strings.TrimSpace(updateToken)
	if updateToken == "" {
		return fmt.Errorf("card update token is required")
	}
	operatorOpenID = strings.TrimSpace(operatorOpenID)
	if strings.TrimSpace(card.Title) == "" {
		card.Title = "Multigent"
	}
	if strings.TrimSpace(card.Body) == "" {
		card.Body = "操作已提交。"
	}
	token, err := c.tenantAccessToken(ctx)
	if err != nil {
		return err
	}
	var openIDs []string
	if operatorOpenID != "" {
		openIDs = []string{operatorOpenID}
	}
	body, _ := json.Marshal(map[string]any{
		"token": updateToken,
		"card":  buildInteractiveCardBody(card, openIDs),
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.openBaseURL(), "/")+"/open-apis/interactive/v1/card/update", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("update interactive card http %d: %s", resp.StatusCode, string(raw))
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if json.Unmarshal(raw, &parsed) == nil && parsed.Code != 0 {
		return fmt.Errorf("update interactive card failed: code=%d msg=%s", parsed.Code, parsed.Msg)
	}
	return nil
}

func buildInteractiveCardBody(card InteractiveCard, openIDs []string) map[string]any {
	elements := make([]map[string]any, 0, 4+len(card.Fields)+len(card.Links))
	appendHR := func() {
		if len(elements) == 0 {
			return
		}
		if tag, _ := elements[len(elements)-1]["tag"].(string); tag == "hr" {
			return
		}
		elements = append(elements, map[string]any{"tag": "hr"})
	}
	body := strings.TrimSpace(card.Body)
	if body != "" {
		for _, chunk := range splitCardMarkdown(body, 900) {
			elements = append(elements, map[string]any{"tag": "div", "text": map[string]string{"tag": "lark_md", "content": chunk}})
		}
	}
	if len(card.Fields) > 0 {
		appendHR()
	}
	for _, field := range card.Fields {
		label := strings.TrimSpace(field.Label)
		value := strings.TrimSpace(field.Value)
		if label == "" && value == "" {
			continue
		}
		if label == "" {
			elements = append(elements, map[string]any{"tag": "div", "text": map[string]string{"tag": "lark_md", "content": value}})
			continue
		}
		content := "**" + label + "**"
		if value != "" {
			content += "\n" + value
		}
		elements = append(elements, map[string]any{"tag": "div", "text": map[string]string{"tag": "lark_md", "content": content}})
	}
	for _, link := range card.Links {
		if strings.TrimSpace(link.Label) == "" || strings.TrimSpace(link.URL) == "" {
			continue
		}
		elements = append(elements, map[string]any{
			"tag": "action",
			"actions": []map[string]any{{
				"tag":  "button",
				"text": map[string]string{"tag": "plain_text", "content": strings.TrimSpace(link.Label)},
				"url":  strings.TrimSpace(link.URL),
				"type": "default",
			}},
		})
	}
	actions := make([]map[string]any, 0, len(card.Actions))
	needsText := false
	for _, action := range card.Actions {
		id := strings.TrimSpace(action.ID)
		label := strings.TrimSpace(action.Label)
		if id == "" || label == "" {
			continue
		}
		if action.RequiresText {
			needsText = true
		}
		style := normalizeButtonStyle(action.Style)
		actions = append(actions, map[string]any{
			"tag":  "button",
			"text": map[string]string{"tag": "plain_text", "content": label},
			"type": style,
			"value": map[string]string{
				"interaction_id": strings.TrimSpace(card.InteractionID),
				"action_id":      id,
				"action_label":   label,
			},
		})
	}
	if needsText || len(actions) > 0 {
		appendHR()
	}
	if needsText {
		elements = append(elements, map[string]any{
			"tag":         "input",
			"name":        "comment",
			"placeholder": map[string]string{"tag": "plain_text", "content": "补充说明或理由（可选）"},
		})
	}
	if len(actions) > 0 {
		elements = append(elements, map[string]any{"tag": "action", "actions": actions})
	}
	cardBody := map[string]any{
		"config": map[string]any{"wide_screen_mode": true},
		"header": map[string]any{
			"template": cardHeaderTemplate(card.Title),
			"title":    map[string]string{"tag": "plain_text", "content": strings.TrimSpace(card.Title)},
		},
		"elements": elements,
	}
	if len(openIDs) > 0 {
		cardBody["open_ids"] = openIDs
	}
	return cardBody
}

func cardHeaderTemplate(title string) string {
	title = strings.ToLower(strings.TrimSpace(title))
	switch {
	case strings.Contains(title, "失败"), strings.Contains(title, "错误"), strings.Contains(title, "打回"), strings.Contains(title, "failed"), strings.Contains(title, "rejected"):
		return "red"
	case strings.Contains(title, "完成"), strings.Contains(title, "已提交"), strings.Contains(title, "通过"), strings.Contains(title, "completed"), strings.Contains(title, "approved"):
		return "green"
	case strings.Contains(title, "处理中"), strings.Contains(title, "正在"), strings.Contains(title, "running"):
		return "blue"
	default:
		return "blue"
	}
}

func splitCardMarkdown(text string, maxRunes int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if maxRunes <= 0 {
		maxRunes = 900
	}
	var chunks []string
	var current strings.Builder
	currentRunes := 0
	flush := func() {
		if strings.TrimSpace(current.String()) == "" {
			current.Reset()
			currentRunes = 0
			return
		}
		chunks = append(chunks, strings.TrimSpace(current.String()))
		current.Reset()
		currentRunes = 0
	}
	for _, line := range strings.Split(text, "\n") {
		lineRunes := len([]rune(line)) + 1
		if currentRunes > 0 && currentRunes+lineRunes > maxRunes {
			flush()
		}
		if current.Len() > 0 {
			current.WriteByte('\n')
		}
		current.WriteString(line)
		currentRunes += lineRunes
	}
	flush()
	return chunks
}

func normalizeButtonStyle(style string) string {
	switch strings.ToLower(strings.TrimSpace(style)) {
	case "primary", "green":
		return "primary"
	case "danger", "red":
		return "danger"
	default:
		return "default"
	}
}

func (c OpenAPIClient) tenantAccessToken(ctx context.Context) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"app_id":     strings.TrimSpace(c.AppID),
		"app_secret": strings.TrimSpace(c.AppSecret),
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.openBaseURL(), "/")+"/open-apis/auth/v3/tenant_access_token/internal", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("tenant token http %d: %s", resp.StatusCode, string(raw))
	}
	var parsed struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	if parsed.Code != 0 || parsed.TenantAccessToken == "" {
		return "", fmt.Errorf("tenant token failed: code=%d msg=%s", parsed.Code, parsed.Msg)
	}
	return parsed.TenantAccessToken, nil
}

func (c OpenAPIClient) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func (c OpenAPIClient) openBaseURL() string {
	if strings.TrimSpace(c.BaseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	}
	return "https://open.feishu.cn"
}

func mustJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(raw)
}
