package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
)

const (
	connectionHealthCheckTick                = time.Minute
	defaultConnectionHealthCheckInterval     = 6 * time.Hour
	minConnectionHealthCheckInterval         = 5 * time.Minute
	maxConnectionHealthCheckInterval         = 30 * 24 * time.Hour
	connectionHealthCheckMaxPerBackgroundRun = 20
)

type testConnectionRequest struct {
	Endpoint string            `json:"endpoint"`
	Method   string            `json:"method"`
	Query    map[string]string `json:"query"`
	Headers  map[string]string `json:"headers"`
	Body     json.RawMessage   `json:"body"`
}

func (s *Server) handleTestConnection(w http.ResponseWriter, r *http.Request) {
	connection, ok := s.connectionByIDWithAccess(w, r)
	if !ok {
		return
	}
	if !s.canManageConnection(r, connection, s.currentUser(r)) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeConnectionManagementRequired, "connection management access required")
		return
	}
	var body testConnectionRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := s.readJSON(w, r, &body); err != nil {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
			return
		}
	}
	result, err := s.testConnection(r, connection, body)
	if err != nil {
		var inputErr runtimeActionInputError
		if errors.As(err, &inputErr) {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, inputErr.Error())
			return
		}
		s.recordConnectionValidation(r, connection, false, 0, err.Error(), false)
		s.jsonErrorCode(w, http.StatusBadGateway, ErrCodeUpstreamError, err.Error())
		return
	}
	s.recordConnectionValidation(r, connection, result.OK, result.Status, result.Message, false)
	s.auditLog(auditLogInput{
		WorkspaceID:  connection.WorkspaceID,
		Action:       "connection.test",
		ResourceType: "connection",
		ResourceID:   connection.ID,
		Summary:      "Connection tested",
		After: map[string]any{
			"provider":       connection.Provider,
			"connectionName": connection.ConnectionName,
			"ok":             result.OK,
			"status":         result.Status,
		},
		Request: r,
	})
	_ = json.NewEncoder(w).Encode(result)
}

type runConnectionHealthChecksRequest struct {
	ConnectionID string `json:"connectionId"`
	Force        bool   `json:"force"`
	Limit        int    `json:"limit"`
}

type connectionHealthCheckRunResponse struct {
	Checked int                              `json:"checked"`
	Skipped int                              `json:"skipped"`
	Results []connectionHealthCheckRunResult `json:"results"`
}

type connectionHealthCheckRunResult struct {
	ConnectionID   string `json:"connectionId"`
	Provider       string `json:"provider"`
	ConnectionName string `json:"connectionName"`
	OK             bool   `json:"ok"`
	Status         int    `json:"status"`
	Message        string `json:"message"`
	Error          string `json:"error,omitempty"`
}

func (s *Server) handleRunConnectionHealthChecks(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	var body runConnectionHealthChecksRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := s.readJSON(w, r, &body); err != nil {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
			return
		}
	}
	if body.Limit <= 0 || body.Limit > 100 {
		body.Limit = 100
	}
	resp := s.runConnectionHealthChecks(r.Context(), connectionHealthCheckOptions{
		WorkspaceID:  workspaceID,
		ConnectionID: strings.TrimSpace(body.ConnectionID),
		Force:        body.Force,
		Limit:        body.Limit,
		AuditRequest: r,
	})
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "connection.health_check.run",
		ResourceType: "connection",
		ResourceID:   strings.TrimSpace(body.ConnectionID),
		Summary:      "Connection health checks run",
		After: map[string]any{
			"checked": resp.Checked,
			"skipped": resp.Skipped,
			"force":   body.Force,
			"limit":   body.Limit,
		},
		Request: r,
	})
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) recordConnectionValidation(r *http.Request, connection controldb.Connection, ok bool, status int, message string, healthCheck bool) {
	now := time.Now().UTC().Format(time.RFC3339)
	updated := connection
	updated.UpdatedAt = now
	profile := connectionProfileMap(connection)
	profile["lastValidatedAt"] = now
	profile["lastValidationOK"] = ok
	profile["lastValidationStatus"] = status
	profile["lastValidationMessage"] = strings.TrimSpace(message)
	if healthCheck {
		profile["lastHealthCheckAt"] = now
	}
	if healthCheck && healthConnectionEnabled(profile) {
		profile["nextHealthCheckAt"] = time.Now().UTC().Add(healthConnectionInterval(profile)).Format(time.RFC3339)
	}
	profileJSON, _ := json.Marshal(profile)
	updated.ProfileJSON = string(profileJSON)
	if err := s.controlDB.UpdateConnection(updated); err != nil {
		return
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  connection.WorkspaceID,
		Action:       "connection.validate",
		ResourceType: "connection",
		ResourceID:   connection.ID,
		Summary:      "Connection validation result recorded",
		After: map[string]any{
			"provider":       connection.Provider,
			"connectionName": connection.ConnectionName,
			"ok":             ok,
			"status":         status,
			"message":        strings.TrimSpace(message),
		},
		Request: r,
	})
}

type connectionHealthCheckOptions struct {
	WorkspaceID  string
	ConnectionID string
	Force        bool
	Limit        int
	AuditRequest *http.Request
}

func (s *Server) startConnectionHealthChecker() {
	if s == nil || s.controlDB == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.connectionHealthCancel = cancel
	done := make(chan struct{})
	s.connectionHealthDone = done
	go func() {
		defer close(done)
		ticker := time.NewTicker(connectionHealthCheckTick)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runConnectionHealthChecks(ctx, connectionHealthCheckOptions{
					Force: false,
					Limit: connectionHealthCheckMaxPerBackgroundRun,
				})
			}
		}
	}()
}

func (s *Server) runConnectionHealthChecks(ctx context.Context, opts connectionHealthCheckOptions) connectionHealthCheckRunResponse {
	limit := opts.Limit
	if limit <= 0 {
		limit = connectionHealthCheckMaxPerBackgroundRun
	}
	connections, err := s.controlDB.ListConnections(controldb.ConnectionFilter{
		WorkspaceID: strings.TrimSpace(opts.WorkspaceID),
		Status:      "active",
	})
	if err != nil {
		return connectionHealthCheckRunResponse{Results: []connectionHealthCheckRunResult{{Error: err.Error()}}}
	}
	resp := connectionHealthCheckRunResponse{Results: []connectionHealthCheckRunResult{}}
	now := time.Now().UTC()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/internal/connection-health-check", nil)
	for _, connection := range connections {
		if opts.ConnectionID != "" && connection.ID != opts.ConnectionID {
			continue
		}
		profile := connectionProfileMap(connection)
		if !opts.Force && !connectionHealthCheckDue(profile, now) {
			resp.Skipped++
			continue
		}
		if resp.Checked >= limit {
			resp.Skipped++
			continue
		}
		result, err := s.testConnection(req, connection, testConnectionRequest{})
		runResult := connectionHealthCheckRunResult{
			ConnectionID:   connection.ID,
			Provider:       connection.Provider,
			ConnectionName: connection.ConnectionName,
			OK:             result.OK,
			Status:         result.Status,
			Message:        result.Message,
		}
		if err != nil {
			runResult.OK = false
			runResult.Error = err.Error()
			runResult.Message = err.Error()
			s.recordConnectionValidation(opts.AuditRequest, connection, false, 0, err.Error(), true)
		} else {
			s.recordConnectionValidation(opts.AuditRequest, connection, result.OK, result.Status, result.Message, true)
		}
		s.auditLog(auditLogInput{
			WorkspaceID:  connection.WorkspaceID,
			ActorType:    "system",
			ActorID:      "connection-health-checker",
			Action:       "connection.health_check",
			ResourceType: "connection",
			ResourceID:   connection.ID,
			Summary:      "Connection health check completed",
			After: map[string]any{
				"provider":       connection.Provider,
				"connectionName": connection.ConnectionName,
				"ok":             runResult.OK,
				"status":         runResult.Status,
				"message":        runResult.Message,
				"error":          runResult.Error,
			},
		})
		resp.Checked++
		resp.Results = append(resp.Results, runResult)
	}
	return resp
}

func connectionHealthCheckDue(profile map[string]any, now time.Time) bool {
	if !healthConnectionEnabled(profile) {
		return false
	}
	if raw, ok := profile["nextHealthCheckAt"].(string); ok && strings.TrimSpace(raw) != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			return !t.After(now)
		}
	}
	if raw, ok := profile["lastHealthCheckAt"].(string); ok && strings.TrimSpace(raw) != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			return !t.Add(healthConnectionInterval(profile)).After(now)
		}
	}
	if raw, ok := profile["lastValidatedAt"].(string); ok && strings.TrimSpace(raw) != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			return !t.Add(healthConnectionInterval(profile)).After(now)
		}
	}
	return true
}

func healthConnectionEnabled(profile map[string]any) bool {
	enabled, _ := profile["healthCheckEnabled"].(bool)
	return enabled
}

func healthConnectionInterval(profile map[string]any) time.Duration {
	minutes := float64(defaultConnectionHealthCheckInterval / time.Minute)
	switch v := profile["healthCheckIntervalMinutes"].(type) {
	case float64:
		if v > 0 {
			minutes = v
		}
	case int:
		if v > 0 {
			minutes = float64(v)
		}
	case json.Number:
		if n, err := v.Float64(); err == nil && n > 0 {
			minutes = n
		}
	}
	interval := time.Duration(minutes) * time.Minute
	if interval < minConnectionHealthCheckInterval {
		return minConnectionHealthCheckInterval
	}
	if interval > maxConnectionHealthCheckInterval {
		return maxConnectionHealthCheckInterval
	}
	return interval
}

type testConnectionResult struct {
	OK      bool              `json:"ok"`
	Status  int               `json:"status"`
	Message string            `json:"message"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    any               `json:"body,omitempty"`
}

func (s *Server) testConnection(r *http.Request, connection controldb.Connection, body testConnectionRequest) (testConnectionResult, error) {
	switch connection.Provider {
	case "custom-mcp":
		return s.testCustomMCPConnection(r, connection)
	case "ssh_key", "git_ssh", "npm_registry", "docker_registry", "aws", "gcloud":
		return s.testStaticRuntimeCredentialConnection(connection)
	case "custom-http", "github", "gitlab", "gitee", "linear", "notion", "figma", "airtable", "asana", "clickup", "sentry", "vercel", "cloudflare", "exa", "brave_search", "feishu", "lark", "dingtalk_bot":
		return s.testHTTPConnection(r, connection, body)
	default:
		return testConnectionResult{}, fmt.Errorf("connection test is not supported for provider %q", connection.Provider)
	}
}

func (s *Server) testStaticRuntimeCredentialConnection(connection controldb.Connection) (testConnectionResult, error) {
	secret, ok, err := s.controlDB.ConnectionSecret(connection.ID)
	if err != nil {
		return testConnectionResult{}, err
	}
	if !ok {
		return testConnectionResult{}, fmt.Errorf("connection secret not found")
	}
	values, err := openConnectionSecret(secret)
	if err != nil {
		return testConnectionResult{}, err
	}
	switch connection.Provider {
	case "ssh_key":
		if normalizePrivateCredential(values["privateKey"]) == "" {
			return testConnectionResult{}, fmt.Errorf("privateKey is required")
		}
	case "git_ssh":
		if normalizePrivateCredential(values["privateKey"]) == "" {
			return testConnectionResult{}, fmt.Errorf("privateKey is required")
		}
	case "npm_registry":
		if strings.TrimSpace(values["registryUrl"]) == "" {
			return testConnectionResult{}, fmt.Errorf("registryUrl is required")
		}
		if strings.TrimSpace(firstNonEmpty(values["authToken"], values["apiKey"], values["token"])) == "" {
			return testConnectionResult{}, fmt.Errorf("authToken is required")
		}
	case "docker_registry":
		if strings.TrimSpace(values["registryUrl"]) == "" {
			return testConnectionResult{}, fmt.Errorf("registryUrl is required")
		}
		if strings.TrimSpace(firstNonEmpty(values["password"], values["authToken"], values["apiKey"], values["token"])) == "" {
			return testConnectionResult{}, fmt.Errorf("password is required")
		}
	case "aws":
		if strings.TrimSpace(values["accessKeyId"]) == "" {
			return testConnectionResult{}, fmt.Errorf("accessKeyId is required")
		}
		if strings.TrimSpace(values["secretAccessKey"]) == "" {
			return testConnectionResult{}, fmt.Errorf("secretAccessKey is required")
		}
	case "gcloud":
		if strings.TrimSpace(values["serviceAccountJson"]) == "" {
			return testConnectionResult{}, fmt.Errorf("serviceAccountJson is required")
		}
		if !json.Valid([]byte(strings.TrimSpace(values["serviceAccountJson"]))) {
			return testConnectionResult{}, fmt.Errorf("serviceAccountJson must be valid JSON")
		}
		if strings.TrimSpace(values["projectId"]) == "" {
			return testConnectionResult{}, fmt.Errorf("projectId is required")
		}
	}
	return testConnectionResult{OK: true, Status: http.StatusOK, Message: "credential format looks valid"}, nil
}

func normalizePrivateCredential(value string) string {
	return strings.TrimSpace(strings.ReplaceAll(value, "\r\n", "\n"))
}

func (s *Server) testCustomMCPConnection(r *http.Request, connection controldb.Connection) (testConnectionResult, error) {
	cfg, err := s.customMCPRuntimeConfig(connection)
	if err != nil {
		return testConnectionResult{}, err
	}
	if cfg.ServerURL == "" {
		return testConnectionResult{}, fmt.Errorf("custom MCP connection is missing serverUrl")
	}
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, cfg.ServerURL, bytes.NewReader(body))
	if err != nil {
		return testConnectionResult{}, fmt.Errorf("build MCP test request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if cfg.AuthHeader != "" && cfg.AuthValue != "" {
		req.Header.Set(cfg.AuthHeader, cfg.AuthValue)
	}
	return executeConnectionTestRequest(req, cfg.RedactValues)
}

func (s *Server) testHTTPConnection(r *http.Request, connection controldb.Connection, body testConnectionRequest) (testConnectionResult, error) {
	actionReq := runtimeActionProxyRequest{
		Endpoint: strings.TrimSpace(body.Endpoint),
		Method:   strings.TrimSpace(body.Method),
		Query:    body.Query,
		Headers:  body.Headers,
		Body:     body.Body,
	}
	if actionReq.Endpoint == "" || actionReq.Method == "" || len(actionReq.Body) == 0 {
		applyDefaultConnectionTestRequest(connection.Provider, &actionReq)
	}
	actionReq.Method = strings.ToUpper(strings.TrimSpace(actionReq.Method))
	if actionReq.Method == "" {
		actionReq.Method = http.MethodGet
	}
	if !runtimeActionMethodAllowed(actionReq.Method) {
		return testConnectionResult{}, runtimeActionInputError{message: "method must be one of DELETE, GET, HEAD, PATCH, POST, or PUT"}
	}
	if (actionReq.Method == http.MethodGet || actionReq.Method == http.MethodHead) && len(actionReq.Body) > 0 {
		return testConnectionResult{}, runtimeActionInputError{message: "GET and HEAD action proxy requests must not include a body"}
	}
	if err := validateRuntimeRelativeEndpoint(actionReq.Endpoint); err != nil {
		return testConnectionResult{}, err
	}
	cfg, err := s.runtimeHTTPActionConfig(connection)
	if err != nil {
		return testConnectionResult{}, err
	}
	endpoint := actionReq.Endpoint
	query := actionReq.Query
	if cfg.EndpointRewrite != nil {
		endpoint, query, err = cfg.EndpointRewrite(endpoint, query)
		if err != nil {
			return testConnectionResult{}, err
		}
	}
	if err := enforceRuntimeActionPolicy(connection, actionReq.Method, endpoint); err != nil {
		return testConnectionResult{}, err
	}
	target, err := buildRuntimeActionURL(cfg.BaseURL, endpoint, query)
	if err != nil {
		return testConnectionResult{}, err
	}
	var reqBody io.Reader
	if len(actionReq.Body) > 0 {
		reqBody = bytes.NewReader(actionReq.Body)
	}
	req, err := http.NewRequestWithContext(r.Context(), actionReq.Method, target, reqBody)
	if err != nil {
		return testConnectionResult{}, fmt.Errorf("build connection test request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if len(actionReq.Body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	applyRuntimeDefaultHeaders(req, cfg)
	for key, value := range actionReq.Headers {
		key = strings.TrimSpace(key)
		if key == "" || runtimeActionBlockedHeader(key) {
			continue
		}
		req.Header.Set(key, strings.TrimSpace(value))
	}
	applyRuntimeActionAuth(req, cfg)
	return executeConnectionTestRequest(req, cfg.RedactValues)
}

func applyDefaultConnectionTestRequest(provider string, req *runtimeActionProxyRequest) {
	switch provider {
	case "github", "gitlab", "gitee":
		if req.Endpoint == "" {
			req.Endpoint = "/user"
		}
		if req.Method == "" {
			req.Method = http.MethodGet
		}
	case "linear":
		if req.Endpoint == "" {
			req.Endpoint = "/graphql"
		}
		if req.Method == "" {
			req.Method = http.MethodPost
		}
		if len(req.Body) == 0 {
			req.Body = json.RawMessage(`{"query":"query { viewer { id name } }"}`)
		}
	case "notion":
		if req.Endpoint == "" {
			req.Endpoint = "/users/me"
		}
		if req.Method == "" {
			req.Method = http.MethodGet
		}
	case "figma":
		if req.Endpoint == "" {
			req.Endpoint = "/me"
		}
		if req.Method == "" {
			req.Method = http.MethodGet
		}
	case "airtable":
		if req.Endpoint == "" {
			req.Endpoint = "/meta/whoami"
		}
		if req.Method == "" {
			req.Method = http.MethodGet
		}
	case "asana":
		if req.Endpoint == "" {
			req.Endpoint = "/users/me"
		}
		if req.Method == "" {
			req.Method = http.MethodGet
		}
	case "clickup":
		if req.Endpoint == "" {
			req.Endpoint = "/user"
		}
		if req.Method == "" {
			req.Method = http.MethodGet
		}
	case "sentry":
		if req.Endpoint == "" {
			req.Endpoint = "/organizations/"
		}
		if req.Method == "" {
			req.Method = http.MethodGet
		}
	case "vercel":
		if req.Endpoint == "" {
			req.Endpoint = "/v2/user"
		}
		if req.Method == "" {
			req.Method = http.MethodGet
		}
	case "exa":
		if req.Endpoint == "" {
			req.Endpoint = "/search"
		}
		if req.Method == "" {
			req.Method = http.MethodPost
		}
		if len(req.Body) == 0 {
			req.Body = json.RawMessage(`{"query":"Multigent","numResults":1}`)
		}
	case "brave_search":
		if req.Endpoint == "" {
			req.Endpoint = "/res/v1/web/search"
		}
		if req.Method == "" {
			req.Method = http.MethodGet
		}
		if req.Query == nil {
			req.Query = map[string]string{}
		}
		if req.Query["q"] == "" {
			req.Query["q"] = "Multigent"
		}
		if req.Query["count"] == "" {
			req.Query["count"] = "1"
		}
	case "custom-http":
		if req.Endpoint == "" {
			req.Endpoint = "/"
		}
		if req.Method == "" {
			req.Method = http.MethodGet
		}
	case "feishu", "lark":
		if req.Endpoint == "" {
			req.Endpoint = "/"
		}
		if req.Method == "" {
			req.Method = http.MethodGet
		}
	case "dingtalk_bot":
		if req.Endpoint == "" {
			req.Endpoint = "/robot/send"
		}
		if req.Method == "" {
			req.Method = http.MethodPost
		}
		if len(req.Body) == 0 {
			req.Body = json.RawMessage(`{"msgtype":"text","text":{"content":"Multigent connection test"}}`)
		}
	}
}

func executeConnectionTestRequest(req *http.Request, redactValues []string) (testConnectionResult, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return testConnectionResult{}, fmt.Errorf("call connection test endpoint: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxJSONBody+1))
	if err != nil {
		return testConnectionResult{}, fmt.Errorf("read connection test response: %w", err)
	}
	if len(respBody) > maxJSONBody {
		return testConnectionResult{}, runtimeActionInputError{message: "connection test response too large"}
	}
	respBody = redactRuntimeProxyResponse(respBody, redactValues...)
	ok := resp.StatusCode >= 200 && resp.StatusCode < 300
	message := "Connection test succeeded"
	if !ok {
		message = fmt.Sprintf("Connection test returned HTTP %d", resp.StatusCode)
	}
	return testConnectionResult{
		OK:      ok,
		Status:  resp.StatusCode,
		Message: message,
		Headers: safeRuntimeActionResponseHeaders(resp.Header),
		Body:    runtimeActionResponseBody(respBody, resp.Header.Get("Content-Type")),
	}, nil
}
