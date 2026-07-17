// Package lark implements Feishu/Lark platform primitives used by Multigent's
// native IM bridge.
package lark

import (
	"context"
	"encoding/base64"
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
	UserCode   string `json:"userCode,omitempty"`
	Interval   int    `json:"interval"`
	ExpiresIn  int    `json:"expiresIn"`
	BaseURL    string `json:"baseUrl"`
	Stage      string `json:"stage,omitempty"`
}

type PollResponse struct {
	Status           string `json:"status"`
	BaseURL          string `json:"baseUrl"`
	Provider         string `json:"provider,omitempty"`
	Stage            string `json:"stage,omitempty"`
	AppID            string `json:"appId,omitempty"`
	AppSecret        string `json:"-"`
	AccessToken      string `json:"-"`
	RefreshToken     string `json:"-"`
	TokenType        string `json:"tokenType,omitempty"`
	ExpiresIn        int    `json:"expiresIn,omitempty"`
	RefreshExpiresIn int    `json:"refreshExpiresIn,omitempty"`
	Scope            string `json:"scope,omitempty"`
	OwnerOpenID      string `json:"ownerOpenId,omitempty"`
	SlowDown         bool   `json:"slowDown,omitempty"`
	Error            string `json:"error,omitempty"`
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
	beginBaseURL := feishuAccountsBaseURL
	pollBaseURL := accountsBaseURL(provider)
	client := c.httpClient()

	initResp, err := c.registrationCall(ctx, client, beginBaseURL, "init", nil)
	if err != nil {
		return BeginResponse{}, fmt.Errorf("%s init: %w", provider, err)
	}
	if errMsg, _ := initResp["error"].(string); errMsg != "" {
		desc, _ := initResp["error_description"].(string)
		return BeginResponse{}, fmt.Errorf("%s init: %s: %s", provider, errMsg, desc)
	}

	beginResp, err := c.registrationCall(ctx, client, beginBaseURL, "begin", map[string]string{
		"archetype":         "PersonalAgent",
		"auth_method":       "client_secret",
		"request_user_info": "open_id tenant_brand",
	})
	if err != nil {
		return BeginResponse{}, fmt.Errorf("%s begin: %w", provider, err)
	}
	if errMsg, _ := beginResp["error"].(string); errMsg != "" {
		desc, _ := beginResp["error_description"].(string)
		return BeginResponse{}, fmt.Errorf("%s begin: %s: %s", provider, errMsg, desc)
	}

	deviceCode, _ := beginResp["device_code"].(string)
	userCode, _ := beginResp["user_code"].(string)
	qrURL := appRegistrationVerificationURL(openBaseURL(provider), userCode)
	interval, _ := beginResp["interval"].(float64)
	expiresIn := float64(intFromAny(firstValue(beginResp, "expires_in", "expire_in")))
	if deviceCode == "" || qrURL == "" {
		return BeginResponse{}, fmt.Errorf("%s begin: incomplete response", provider)
	}
	if interval <= 0 {
		interval = 5
	}
	return BeginResponse{
		DeviceCode: deviceCode,
		QRURL:      qrURL,
		UserCode:   userCode,
		Interval:   int(interval),
		ExpiresIn:  int(expiresIn),
		BaseURL:    pollBaseURL,
		Stage:      "create_app",
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
			result.Stage = "create_app"
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

func (c RegistrationClient) BeginAuthorization(ctx context.Context, provider, clientID, clientSecret string, scopes []string) (BeginResponse, error) {
	provider, ok := NormalizeProvider(provider)
	if !ok {
		return BeginResponse{}, fmt.Errorf("unsupported provider %q", provider)
	}
	clientID = strings.TrimSpace(clientID)
	clientSecret = strings.TrimSpace(clientSecret)
	if clientID == "" || clientSecret == "" {
		return BeginResponse{}, fmt.Errorf("client credentials are required")
	}
	scopeSet := map[string]bool{"offline_access": true}
	for _, scope := range scopes {
		if scope = strings.TrimSpace(scope); scope != "" {
			scopeSet[scope] = true
		}
	}
	scopeList := make([]string, 0, len(scopeSet))
	for scope := range scopeSet {
		scopeList = append(scopeList, scope)
	}
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("scope", strings.Join(scopeList, " "))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(accountsBaseURL(provider), "/")+"/oauth/v1/device_authorization", strings.NewReader(form.Encode()))
	if err != nil {
		return BeginResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(clientID+":"+clientSecret)))
	body, err := doJSONMap(c.httpClient(), req)
	if err != nil {
		return BeginResponse{}, err
	}
	if errMsg, _ := body["error"].(string); errMsg != "" {
		desc, _ := body["error_description"].(string)
		return BeginResponse{}, fmt.Errorf("%s authorization begin: %s: %s", provider, errMsg, desc)
	}
	deviceCode, _ := body["device_code"].(string)
	userCode, _ := body["user_code"].(string)
	qrURL := deviceAuthorizationVerificationURL(body, openBaseURL(provider), userCode)
	interval, _ := body["interval"].(float64)
	expiresIn, _ := body["expires_in"].(float64)
	if deviceCode == "" || qrURL == "" {
		return BeginResponse{}, fmt.Errorf("%s authorization begin: incomplete response", provider)
	}
	if interval <= 0 {
		interval = 5
	}
	return BeginResponse{
		DeviceCode: deviceCode,
		QRURL:      qrURL,
		UserCode:   userCode,
		Interval:   int(interval),
		ExpiresIn:  int(expiresIn),
		BaseURL:    accountsBaseURL(provider),
		Stage:      "authorize",
	}, nil
}

func (c RegistrationClient) PollAuthorization(ctx context.Context, provider, clientID, clientSecret, deviceCode string) (PollResponse, error) {
	provider, ok := NormalizeProvider(provider)
	if !ok {
		return PollResponse{}, fmt.Errorf("unsupported provider %q", provider)
	}
	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form.Set("device_code", strings.TrimSpace(deviceCode))
	form.Set("client_id", strings.TrimSpace(clientID))
	form.Set("client_secret", strings.TrimSpace(clientSecret))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(openBaseURL(provider), "/")+"/open-apis/authen/v2/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return PollResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	body, err := doJSONMap(c.httpClient(), req)
	if err != nil {
		return PollResponse{}, err
	}
	result := PollResponse{Status: "pending", Provider: provider, BaseURL: accountsBaseURL(provider), Stage: "authorize"}
	if errCode, _ := body["error"].(string); errCode != "" {
		switch errCode {
		case "authorization_pending":
		case "slow_down":
			result.SlowDown = true
		case "access_denied":
			result.Status = "denied"
		case "expired_token", "invalid_grant":
			result.Status = "expired"
		default:
			desc, _ := body["error_description"].(string)
			result.Status = "error"
			result.Error = strings.TrimSpace(fmt.Sprintf("%s: %s", errCode, desc))
		}
		return result, nil
	}
	accessToken, _ := body["access_token"].(string)
	if strings.TrimSpace(accessToken) == "" {
		return result, nil
	}
	result.Status = "completed"
	result.AccessToken = accessToken
	result.RefreshToken, _ = body["refresh_token"].(string)
	result.TokenType, _ = body["token_type"].(string)
	result.ExpiresIn = intFromAny(body["expires_in"])
	result.RefreshExpiresIn = intFromAny(body["refresh_token_expires_in"])
	result.Scope, _ = body["scope"].(string)
	return result, nil
}

func doJSONMap(client *http.Client, req *http.Request) (map[string]any, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if errMsg, _ := result["error"].(string); errMsg != "" {
			return result, nil
		}
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(body))
	}
	return result, nil
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
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(body))
		}
		return nil, fmt.Errorf("decode: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if errMsg, _ := result["error"].(string); errMsg != "" {
			return result, nil
		}
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(body))
	}
	return result, nil
}

func accountsBaseURL(provider string) string {
	if provider == ProviderLark {
		return larkAccountsBaseURL
	}
	return feishuAccountsBaseURL
}

func openBaseURL(provider string) string {
	if provider == ProviderLark {
		return "https://open.larksuite.com"
	}
	return "https://open.feishu.cn"
}

func appRegistrationVerificationURL(openHost, userCode string) string {
	if strings.TrimSpace(userCode) == "" {
		return ""
	}
	return strings.TrimRight(openHost, "/") + "/page/cli?user_code=" + url.QueryEscape(userCode)
}

func deviceAuthorizationVerificationURL(body map[string]any, openHost, userCode string) string {
	if complete, _ := body["verification_uri_complete"].(string); strings.TrimSpace(complete) != "" {
		return complete
	}
	if uri, _ := body["verification_uri"].(string); strings.TrimSpace(uri) != "" {
		sep := "?"
		if strings.Contains(uri, "?") {
			sep = "&"
		}
		return uri + sep + "user_code=" + url.QueryEscape(userCode)
	}
	return appRegistrationVerificationURL(openHost, userCode)
}

func firstValue(m map[string]any, keys ...string) any {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return nil
}

func intFromAny(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case json.Number:
		n, _ := x.Int64()
		return int(n)
	default:
		return 0
	}
}
