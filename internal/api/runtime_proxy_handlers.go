package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
)

func (s *Server) handleRuntimeMCPProxy(w http.ResponseWriter, r *http.Request) {
	principal, connection, ok := s.runtimeConnectionForRequest(w, r)
	if !ok {
		return
	}
	if connection.Provider != "custom-mcp" {
		s.handleRuntimeProxyUnsupported(w, r, principal, connection, "mcp")
		return
	}
	if err := s.proxyCustomMCP(w, r, principal, connection); err != nil {
		s.jsonError(w, http.StatusBadGateway, err.Error())
		return
	}
}

func (s *Server) handleRuntimeActionProxy(w http.ResponseWriter, r *http.Request) {
	principal, connection, ok := s.runtimeConnectionForRequest(w, r)
	if !ok {
		return
	}
	if err := s.proxyRuntimeAction(w, r, principal, connection); err != nil {
		var inputErr runtimeActionInputError
		if errors.As(err, &inputErr) {
			s.jsonError(w, http.StatusBadRequest, inputErr.Error())
			return
		}
		s.jsonError(w, http.StatusBadGateway, err.Error())
		return
	}
}

func (s *Server) handleRuntimeProxyUnsupported(w http.ResponseWriter, r *http.Request, principal runtimeAgentPrincipal, connection controldb.Connection, surface string) {
	s.auditRuntimeConnectionUse(r, principal, connection, surface)
	w.WriteHeader(http.StatusNotImplemented)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": false,
		"error":   "runtime proxy executor is not implemented yet",
		"surface": surface,
		"agent": map[string]string{
			"workspaceId": principal.WorkspaceID,
			"project":     principal.Project,
			"agent":       principal.Agent,
			"runId":       principal.RunID,
		},
		"connection": map[string]string{
			"id":             connection.ID,
			"provider":       connection.Provider,
			"connectionName": connection.ConnectionName,
			"alias":          runtimeConnectionAlias(connection.Provider, connection.ConnectionName),
		},
	})
}

func (s *Server) proxyCustomMCP(w http.ResponseWriter, r *http.Request, principal runtimeAgentPrincipal, connection controldb.Connection) error {
	cfg, err := s.customMCPRuntimeConfig(connection)
	if err != nil {
		return err
	}
	if cfg.ServerURL == "" {
		return fmt.Errorf("custom MCP connection is missing serverUrl")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxJSONBody))
	if err != nil {
		return fmt.Errorf("read MCP request: %w", err)
	}
	defer r.Body.Close()
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, cfg.ServerURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build MCP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if accept := strings.TrimSpace(r.Header.Get("Accept")); accept != "" {
		req.Header.Set("Accept", accept)
	}
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("call custom MCP server: %w", err)
	}
	defer resp.Body.Close()
	s.auditRuntimeConnectionUse(r, principal, connection, "mcp")
	if contentType := strings.TrimSpace(resp.Header.Get("Content-Type")); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxJSONBody))
	if err != nil {
		return fmt.Errorf("read custom MCP response: %w", err)
	}
	respBody = redactRuntimeProxyResponse(respBody, cfg.Token)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)
	return nil
}

type runtimeActionProxyRequest struct {
	Endpoint string            `json:"endpoint"`
	Method   string            `json:"method"`
	Query    map[string]string `json:"query"`
	Headers  map[string]string `json:"headers"`
	Body     json.RawMessage   `json:"body"`
}

type runtimeActionInputError struct {
	message string
}

func (e runtimeActionInputError) Error() string {
	return e.message
}

func (s *Server) proxyRuntimeAction(w http.ResponseWriter, r *http.Request, principal runtimeAgentPrincipal, connection controldb.Connection) error {
	var reqBody runtimeActionProxyRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxJSONBody)).Decode(&reqBody); err != nil {
		return runtimeActionInputError{message: "action proxy request body must be valid JSON"}
	}
	defer r.Body.Close()
	reqBody.Method = strings.ToUpper(strings.TrimSpace(reqBody.Method))
	if reqBody.Method == "" {
		reqBody.Method = http.MethodGet
	}
	if !runtimeActionMethodAllowed(reqBody.Method) {
		return runtimeActionInputError{message: "method must be one of DELETE, GET, HEAD, PATCH, POST, or PUT"}
	}
	if (reqBody.Method == http.MethodGet || reqBody.Method == http.MethodHead) && len(reqBody.Body) > 0 {
		return runtimeActionInputError{message: "GET and HEAD action proxy requests must not include a body"}
	}
	if err := validateRuntimeRelativeEndpoint(reqBody.Endpoint); err != nil {
		return err
	}
	cfg, err := s.runtimeHTTPActionConfig(connection)
	if err != nil {
		return err
	}
	target, err := buildRuntimeActionURL(cfg.BaseURL, reqBody.Endpoint, reqBody.Query)
	if err != nil {
		return err
	}
	var body io.Reader
	if len(reqBody.Body) > 0 {
		body = bytes.NewReader(reqBody.Body)
	}
	upstreamReq, err := http.NewRequestWithContext(r.Context(), reqBody.Method, target, body)
	if err != nil {
		return fmt.Errorf("build action proxy request: %w", err)
	}
	upstreamReq.Header.Set("Accept", "application/json")
	if len(reqBody.Body) > 0 {
		upstreamReq.Header.Set("Content-Type", "application/json")
	}
	for key, value := range reqBody.Headers {
		key = strings.TrimSpace(key)
		if key == "" || runtimeActionBlockedHeader(key) {
			continue
		}
		upstreamReq.Header.Set(key, strings.TrimSpace(value))
	}
	applyRuntimeActionAuth(upstreamReq, cfg)
	client := &http.Client{Timeout: 60 * time.Second}
	upstreamResp, err := client.Do(upstreamReq)
	if err != nil {
		return fmt.Errorf("call action proxy upstream: %w", err)
	}
	defer upstreamResp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(upstreamResp.Body, maxJSONBody+1))
	if err != nil {
		return fmt.Errorf("read action proxy response: %w", err)
	}
	if len(respBody) > maxJSONBody {
		return runtimeActionInputError{message: "action proxy response too large"}
	}
	respBody = redactRuntimeProxyResponse(respBody, cfg.RedactValues...)
	s.auditRuntimeConnectionUse(r, principal, connection, "action")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": upstreamResp.StatusCode >= 200 && upstreamResp.StatusCode < 300,
		"data": map[string]any{
			"status":  upstreamResp.StatusCode,
			"headers": safeRuntimeActionResponseHeaders(upstreamResp.Header),
			"body":    runtimeActionResponseBody(respBody, upstreamResp.Header.Get("Content-Type")),
		},
	})
	return nil
}

type runtimeHTTPActionConfig struct {
	BaseURL      string
	AuthHeader   string
	AuthValue    string
	RedactValues []string
}

func (s *Server) runtimeHTTPActionConfig(connection controldb.Connection) (runtimeHTTPActionConfig, error) {
	values := map[string]string{}
	if connection.AuthType != ConnectionAuthNoAuth {
		secret, ok, err := s.controlDB.ConnectionSecret(connection.ID)
		if err != nil {
			return runtimeHTTPActionConfig{}, err
		}
		if ok {
			opened, err := openConnectionSecret(secret)
			if err != nil {
				return runtimeHTTPActionConfig{}, err
			}
			values = opened
		}
	}
	profile := map[string]any{}
	_ = json.Unmarshal([]byte(connection.ProfileJSON), &profile)
	cfg := runtimeHTTPActionConfig{}
	cfg.BaseURL = strings.TrimSpace(values["baseUrl"])
	if cfg.BaseURL == "" {
		if v, ok := profile["baseUrl"].(string); ok {
			cfg.BaseURL = strings.TrimSpace(v)
		}
	}
	cfg.AuthHeader = strings.TrimSpace(values["authHeader"])
	if cfg.AuthHeader == "" {
		if v, ok := profile["authHeader"].(string); ok {
			cfg.AuthHeader = strings.TrimSpace(v)
		}
	}
	authScheme := strings.TrimSpace(values["authScheme"])
	apiKey := strings.TrimSpace(values["apiKey"])
	switch connection.Provider {
	case "custom-http":
		if cfg.AuthHeader == "" {
			cfg.AuthHeader = "Authorization"
		}
		if apiKey != "" {
			if authScheme == "" {
				authScheme = "Bearer"
			}
			cfg.AuthValue = strings.TrimSpace(strings.TrimSpace(authScheme) + " " + apiKey)
			cfg.RedactValues = append(cfg.RedactValues, apiKey, cfg.AuthValue)
		}
	case "github":
		if cfg.BaseURL == "" {
			cfg.BaseURL = "https://api.github.com"
		}
		cfg.AuthHeader = "Authorization"
		if apiKey != "" {
			cfg.AuthValue = "Bearer " + apiKey
			cfg.RedactValues = append(cfg.RedactValues, apiKey, cfg.AuthValue)
		}
	case "linear":
		if cfg.BaseURL == "" {
			cfg.BaseURL = "https://api.linear.app"
		}
		cfg.AuthHeader = "Authorization"
		if apiKey != "" {
			cfg.AuthValue = apiKey
			cfg.RedactValues = append(cfg.RedactValues, apiKey)
		}
	default:
		return runtimeHTTPActionConfig{}, fmt.Errorf("runtime action proxy is not supported for provider %q", connection.Provider)
	}
	if cfg.BaseURL == "" {
		return runtimeHTTPActionConfig{}, fmt.Errorf("action proxy connection is missing baseUrl")
	}
	if err := validateRuntimeActionBaseURL(cfg.BaseURL); err != nil {
		return runtimeHTTPActionConfig{}, err
	}
	return cfg, nil
}

func runtimeActionMethodAllowed(method string) bool {
	switch method {
	case http.MethodDelete, http.MethodGet, http.MethodHead, http.MethodPatch, http.MethodPost, http.MethodPut:
		return true
	default:
		return false
	}
}

func validateRuntimeRelativeEndpoint(endpoint string) error {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return runtimeActionInputError{message: "endpoint is required"}
	}
	if !strings.HasPrefix(endpoint, "/") || strings.HasPrefix(endpoint, "//") {
		return runtimeActionInputError{message: "endpoint must be a relative path starting with /"}
	}
	if strings.Contains(endpoint, "\\") {
		return runtimeActionInputError{message: "endpoint must not contain path traversal segments"}
	}
	if _, err := url.ParseRequestURI(endpoint); err != nil {
		return runtimeActionInputError{message: "endpoint must be a valid relative path"}
	}
	pathOnly := strings.SplitN(endpoint, "?", 2)[0]
	for _, segment := range strings.Split(pathOnly, "/") {
		decoded, err := url.PathUnescape(segment)
		if err != nil || decoded == ".." {
			return runtimeActionInputError{message: "endpoint must not contain path traversal segments"}
		}
	}
	return nil
}

func validateRuntimeActionBaseURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid action proxy baseUrl: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("action proxy baseUrl must use http or https")
	}
	if u.Host == "" {
		return fmt.Errorf("action proxy baseUrl must include a host")
	}
	return nil
}

func buildRuntimeActionURL(baseURL, endpoint string, query map[string]string) (string, error) {
	base, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil {
		return "", err
	}
	rel, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return "", err
	}
	target := base.ResolveReference(rel)
	q := target.Query()
	for key, value := range query {
		key = strings.TrimSpace(key)
		if key != "" {
			q.Set(key, value)
		}
	}
	target.RawQuery = q.Encode()
	return target.String(), nil
}

func runtimeActionBlockedHeader(header string) bool {
	switch strings.ToLower(strings.TrimSpace(header)) {
	case "authorization", "cookie", "host", "content-length", "connection", "proxy-authorization", "x-multigent-connection-id", "x-multigent-connection-alias":
		return true
	default:
		return false
	}
}

func applyRuntimeActionAuth(req *http.Request, cfg runtimeHTTPActionConfig) {
	if cfg.AuthHeader != "" && cfg.AuthValue != "" {
		req.Header.Set(cfg.AuthHeader, cfg.AuthValue)
	}
}

func safeRuntimeActionResponseHeaders(headers http.Header) map[string]string {
	out := map[string]string{}
	for key, values := range headers {
		if runtimeActionBlockedHeader(key) || strings.EqualFold(key, "set-cookie") {
			continue
		}
		if len(values) > 0 {
			out[key] = values[0]
		}
	}
	return out
}

func runtimeActionResponseBody(body []byte, contentType string) any {
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if mediaType == "application/json" || strings.HasSuffix(mediaType, "+json") {
		var decoded any
		if json.Unmarshal(body, &decoded) == nil {
			return decoded
		}
	}
	return string(body)
}

type customMCPRuntimeConfig struct {
	ServerURL string
	Token     string
}

func (s *Server) customMCPRuntimeConfig(connection controldb.Connection) (customMCPRuntimeConfig, error) {
	values := map[string]string{}
	if connection.AuthType != ConnectionAuthNoAuth {
		secret, ok, err := s.controlDB.ConnectionSecret(connection.ID)
		if err != nil {
			return customMCPRuntimeConfig{}, err
		}
		if ok {
			opened, err := openConnectionSecret(secret)
			if err != nil {
				return customMCPRuntimeConfig{}, err
			}
			values = opened
		}
	}
	profile := map[string]any{}
	_ = json.Unmarshal([]byte(connection.ProfileJSON), &profile)
	serverURL := strings.TrimSpace(values["serverUrl"])
	if serverURL == "" {
		if v, ok := profile["serverUrl"].(string); ok {
			serverURL = strings.TrimSpace(v)
		}
	}
	if err := validateCustomMCPServerURL(serverURL); err != nil {
		return customMCPRuntimeConfig{}, err
	}
	return customMCPRuntimeConfig{
		ServerURL: serverURL,
		Token:     strings.TrimSpace(values["token"]),
	}, nil
}

func validateCustomMCPServerURL(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid custom MCP serverUrl: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("custom MCP serverUrl must use http or https")
	}
	if u.Host == "" {
		return fmt.Errorf("custom MCP serverUrl must include a host")
	}
	return nil
}

func redactRuntimeProxyResponse(body []byte, tokens ...string) []byte {
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" || len(body) == 0 {
			continue
		}
		body = bytes.ReplaceAll(body, []byte("Bearer "+token), []byte("Bearer [redacted]"))
		body = bytes.ReplaceAll(body, []byte(token), []byte("[redacted]"))
	}
	return body
}

func (s *Server) auditRuntimeConnectionUse(r *http.Request, principal runtimeAgentPrincipal, connection controldb.Connection, surface string) {
	s.auditLog(auditLogInput{
		WorkspaceID:  principal.WorkspaceID,
		ActorType:    "agent",
		ActorID:      principal.Project + "/" + principal.Agent,
		Action:       "connection.use",
		ResourceType: "connection",
		ResourceID:   connection.ID,
		Summary:      "Agent runtime connection proxy requested",
		After: map[string]any{
			"project":        principal.Project,
			"agent":          principal.Agent,
			"runId":          principal.RunID,
			"surface":        surface,
			"provider":       connection.Provider,
			"connectionName": connection.ConnectionName,
			"alias":          runtimeConnectionAlias(connection.Provider, connection.ConnectionName),
		},
		Request: r,
	})
}
