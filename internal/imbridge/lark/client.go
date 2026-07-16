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
