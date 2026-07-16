// Package lark implements Feishu/Lark platform primitives used by Multigent's
// native IM bridge.
package lark

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	ProviderFeishu = "feishu"
	ProviderLark   = "lark"

	feishuAccountsBaseURL = "https://accounts.feishu.cn"
	larkAccountsBaseURL   = "https://accounts.larksuite.com"
)

type RegistrationClient struct {
	HTTPClient *http.Client
}

type BeginResponse struct {
	DeviceCode string `json:"deviceCode"`
	QRURL      string `json:"qrUrl"`
	Interval   int    `json:"interval"`
	ExpiresIn  int    `json:"expiresIn"`
	BaseURL    string `json:"baseUrl"`
}

type PollResponse struct {
	Status      string `json:"status"`
	BaseURL     string `json:"baseUrl"`
	Provider    string `json:"provider,omitempty"`
	AppID       string `json:"appId,omitempty"`
	AppSecret   string `json:"-"`
	OwnerOpenID string `json:"ownerOpenId,omitempty"`
	SlowDown    bool   `json:"slowDown,omitempty"`
	Error       string `json:"error,omitempty"`
}

func NormalizeProvider(provider string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case ProviderFeishu:
		return ProviderFeishu, true
	case ProviderLark:
		return ProviderLark, true
	default:
		return "", false
	}
}

func (c RegistrationClient) Begin(ctx context.Context, provider string) (BeginResponse, error) {
	provider, ok := NormalizeProvider(provider)
	if !ok {
		return BeginResponse{}, fmt.Errorf("unsupported provider %q", provider)
	}
	baseURL := accountsBaseURL(provider)
	client := c.httpClient()

	initResp, err := c.registrationCall(ctx, client, baseURL, "init", nil)
	if err != nil {
		return BeginResponse{}, fmt.Errorf("%s init: %w", provider, err)
	}
	if errMsg, _ := initResp["error"].(string); errMsg != "" {
		desc, _ := initResp["error_description"].(string)
		return BeginResponse{}, fmt.Errorf("%s init: %s: %s", provider, errMsg, desc)
	}

	beginResp, err := c.registrationCall(ctx, client, baseURL, "begin", map[string]string{
		"archetype":         "PersonalAgent",
		"auth_method":       "client_secret",
		"request_user_info": "open_id",
	})
	if err != nil {
		return BeginResponse{}, fmt.Errorf("%s begin: %w", provider, err)
	}
	if errMsg, _ := beginResp["error"].(string); errMsg != "" {
		desc, _ := beginResp["error_description"].(string)
		return BeginResponse{}, fmt.Errorf("%s begin: %s: %s", provider, errMsg, desc)
	}

	deviceCode, _ := beginResp["device_code"].(string)
	qrURL, _ := beginResp["verification_uri_complete"].(string)
	interval, _ := beginResp["interval"].(float64)
	expiresIn, _ := beginResp["expire_in"].(float64)
	if deviceCode == "" || qrURL == "" {
		return BeginResponse{}, fmt.Errorf("%s begin: incomplete response", provider)
	}
	if interval <= 0 {
		interval = 5
	}
	return BeginResponse{
		DeviceCode: deviceCode,
		QRURL:      qrURL,
		Interval:   int(interval),
		ExpiresIn:  int(expiresIn),
		BaseURL:    baseURL,
	}, nil
}

func (c RegistrationClient) Poll(ctx context.Context, provider, deviceCode, baseURL string) (PollResponse, error) {
	provider, ok := NormalizeProvider(provider)
	if !ok {
		return PollResponse{}, fmt.Errorf("unsupported provider %q", provider)
	}
	deviceCode = strings.TrimSpace(deviceCode)
	if deviceCode == "" {
		return PollResponse{}, fmt.Errorf("device code is required")
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = accountsBaseURL(provider)
	}
	client := c.httpClient()

	for attempt := 0; attempt < 2; attempt++ {
		pollResp, err := c.registrationCall(ctx, client, baseURL, "poll", map[string]string{
			"device_code": deviceCode,
		})
		if err != nil {
			return PollResponse{}, fmt.Errorf("%s poll: %w", provider, err)
		}

		if userInfo, ok := pollResp["user_info"].(map[string]any); ok {
			if brand, _ := userInfo["tenant_brand"].(string); strings.EqualFold(brand, ProviderLark) && baseURL != larkAccountsBaseURL {
				baseURL = larkAccountsBaseURL
				continue
			}
		}

		result := PollResponse{Status: "pending", BaseURL: baseURL}
		clientID, _ := pollResp["client_id"].(string)
		clientSecret, _ := pollResp["client_secret"].(string)
		if clientID != "" && clientSecret != "" {
			result.Status = "completed"
			result.Provider = provider
			result.AppID = clientID
			result.AppSecret = clientSecret
			if userInfo, ok := pollResp["user_info"].(map[string]any); ok {
				if brand, _ := userInfo["tenant_brand"].(string); strings.EqualFold(brand, ProviderLark) {
					result.Provider = ProviderLark
				} else if strings.EqualFold(brand, ProviderFeishu) {
					result.Provider = ProviderFeishu
				}
				if oid, _ := userInfo["open_id"].(string); oid != "" {
					result.OwnerOpenID = oid
				}
			}
			return result, nil
		}

		if errCode, _ := pollResp["error"].(string); errCode != "" {
			switch errCode {
			case "authorization_pending":
			case "slow_down":
				result.SlowDown = true
			case "access_denied":
				result.Status = "denied"
			case "expired_token":
				result.Status = "expired"
			default:
				desc, _ := pollResp["error_description"].(string)
				result.Status = "error"
				result.Error = strings.TrimSpace(fmt.Sprintf("%s: %s", errCode, desc))
			}
		}
		return result, nil
	}

	return PollResponse{Status: "pending", BaseURL: baseURL}, nil
}

func (c RegistrationClient) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func (c RegistrationClient) registrationCall(ctx context.Context, client *http.Client, baseURL, action string, params map[string]string) (map[string]any, error) {
	form := url.Values{}
	form.Set("action", action)
	for k, v := range params {
		form.Set(k, v)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/oauth/v1/app/registration", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(body))
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return result, nil
}

func accountsBaseURL(provider string) string {
	if provider == ProviderLark {
		return larkAccountsBaseURL
	}
	return feishuAccountsBaseURL
}
