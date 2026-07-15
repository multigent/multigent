package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
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
		s.jsonError(w, http.StatusForbidden, "connection management access required")
		return
	}
	var body testConnectionRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := s.readJSON(w, r, &body); err != nil {
			s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}
	result, err := s.testConnection(r, connection, body)
	if err != nil {
		var inputErr runtimeActionInputError
		if errors.As(err, &inputErr) {
			s.jsonError(w, http.StatusBadRequest, inputErr.Error())
			return
		}
		s.jsonError(w, http.StatusBadGateway, err.Error())
		return
	}
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
	case "custom-http", "github", "linear":
		return s.testHTTPConnection(r, connection, body)
	default:
		return testConnectionResult{}, fmt.Errorf("connection test is not supported for provider %q", connection.Provider)
	}
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
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	return executeConnectionTestRequest(req, []string{cfg.Token})
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
	target, err := buildRuntimeActionURL(cfg.BaseURL, actionReq.Endpoint, actionReq.Query)
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
	case "github":
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
	case "custom-http":
		if req.Endpoint == "" {
			req.Endpoint = "/"
		}
		if req.Method == "" {
			req.Method = http.MethodGet
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
