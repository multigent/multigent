package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/multigent/multigent/internal/connector"
)

type mcpGatewayRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpGatewayToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type mcpGatewayToolInfo struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Provider    string         `json:"provider"`
	Connection  string         `json:"connection"`
	Adapter     string         `json:"adapter"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

func (s *Server) handleRuntimeMCPGateway(w http.ResponseWriter, r *http.Request) {
	principal, ok := runtimeAgentFromRequest(r)
	if !ok {
		s.jsonErrorCode(w, http.StatusUnauthorized, ErrCodeRuntimeAgentTokenRequired, "runtime agent token required")
		return
	}
	if !runtimeHasCapability(principal, "connection.use") {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeRuntimeCapabilityRequired, "runtime token lacks connection.use capability")
		return
	}
	var req mcpGatewayRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&req); err != nil {
		writeMCPGatewayError(w, nil, -32700, "invalid JSON-RPC request")
		return
	}
	switch req.Method {
	case "initialize":
		writeMCPGatewayResult(w, req.ID, map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo":      map[string]any{"name": "multigent-mcp-gateway", "version": "v1"},
			"capabilities":    map[string]any{"tools": map[string]any{}},
		})
	case "tools/list":
		writeMCPGatewayResult(w, req.ID, map[string]any{"tools": mcpGatewayBrokerTools()})
	case "tools/call":
		s.handleRuntimeMCPGatewayToolCall(w, r, principal, req)
	default:
		writeMCPGatewayError(w, req.ID, -32601, "method not found")
	}
}

func (s *Server) handleRuntimeMCPGatewayToolCall(w http.ResponseWriter, r *http.Request, principal runtimeAgentPrincipal, req mcpGatewayRequest) {
	var params mcpGatewayToolCallParams
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			writeMCPGatewayError(w, req.ID, -32602, "invalid tools/call params")
			return
		}
	}
	switch params.Name {
	case "multigent.list_tools":
		tools, err := s.mcpGatewayRuntimeTools(principal, params.Arguments)
		if err != nil {
			writeMCPGatewayError(w, req.ID, -32000, err.Error())
			return
		}
		writeMCPGatewayTextResult(w, req.ID, tools)
	case "multigent.call_tool":
		result, err := s.mcpGatewayCallTool(r, principal, params.Arguments)
		if err != nil {
			writeMCPGatewayError(w, req.ID, -32000, err.Error())
			return
		}
		writeMCPGatewayTextResult(w, req.ID, result)
	default:
		writeMCPGatewayError(w, req.ID, -32602, "unknown tool name")
	}
}

func mcpGatewayBrokerTools() []map[string]any {
	return []map[string]any{
		{
			"name":        "multigent.list_tools",
			"description": "List external tools available to this agent through Multigent runtime adapters.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"provider": map[string]any{"type": "string", "description": "Optional provider filter, such as github or figma."},
					"adapter":  map[string]any{"type": "string", "description": "Optional adapter filter: cli, mcp_gateway, http_action, or skill_only."},
				},
				"additionalProperties": false,
			},
		},
		{
			"name":        "multigent.call_tool",
			"description": "Call one listed Multigent runtime tool by tool_id. The server applies credentials, policy, and audit.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"tool_id":   map[string]any{"type": "string", "description": "Tool id returned by multigent.list_tools."},
					"arguments": map[string]any{"type": "object", "description": "Tool arguments."},
				},
				"required":             []string{"tool_id"},
				"additionalProperties": false,
			},
		},
	}
}

func (s *Server) mcpGatewayRuntimeTools(principal runtimeAgentPrincipal, args map[string]any) ([]mcpGatewayToolInfo, error) {
	providerFilter := strings.TrimSpace(stringArg(args, "provider"))
	adapterFilter := strings.TrimSpace(stringArg(args, "adapter"))
	connections, err := s.resolveAgentRuntimeConnections(principal.WorkspaceID, principal.Project, principal.Agent)
	if err != nil {
		return nil, err
	}
	out := make([]mcpGatewayToolInfo, 0)
	for _, connection := range connections {
		if providerFilter != "" && connection.Provider != providerFilter {
			continue
		}
		for _, action := range connection.Runtime.Actions {
			adapter := connector.RuntimeAdapterHTTPAction
			if adapterFilter != "" && adapterFilter != adapter {
				continue
			}
			out = append(out, mcpGatewayToolInfo{
				ID:          "action:" + connection.Runtime.Alias + ":" + action.Name,
				Name:        action.Name,
				Provider:    connection.Provider,
				Connection:  connection.Runtime.Alias,
				Adapter:     adapter,
				Description: action.Description,
				InputSchema: action.InputSchema,
			})
		}
		for _, adapter := range connection.Runtime.Adapters {
			if adapter.Type != connector.RuntimeAdapterMCPGateway {
				continue
			}
			if adapterFilter != "" && adapterFilter != connector.RuntimeAdapterMCPGateway {
				continue
			}
			out = append(out, mcpGatewayToolInfo{
				ID:          "mcp:" + connection.Runtime.Alias + ":tools/call",
				Name:        "tools/call",
				Provider:    connection.Provider,
				Connection:  connection.Runtime.Alias,
				Adapter:     connector.RuntimeAdapterMCPGateway,
				Description: adapter.Description,
			})
		}
	}
	return out, nil
}

func (s *Server) mcpGatewayCallTool(r *http.Request, principal runtimeAgentPrincipal, args map[string]any) (map[string]any, error) {
	toolID := strings.TrimSpace(stringArg(args, "tool_id"))
	if toolID == "" {
		return nil, fmt.Errorf("tool_id is required")
	}
	toolArgs, _ := args["arguments"].(map[string]any)
	parts := strings.SplitN(toolID, ":", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid tool_id")
	}
	switch parts[0] {
	case "action":
		return s.mcpGatewayCallActionTool(r, principal, parts[1], parts[2], toolArgs)
	case "mcp":
		return s.mcpGatewayCallCustomMCPTool(r, principal, parts[1], parts[2], toolArgs)
	default:
		return nil, fmt.Errorf("unsupported tool_id type %q", parts[0])
	}
}

func (s *Server) mcpGatewayCallActionTool(r *http.Request, principal runtimeAgentPrincipal, alias, actionName string, args map[string]any) (map[string]any, error) {
	connection, ok, err := s.findRuntimeConnection(principal, "", alias)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("connection is not granted to this agent")
	}
	provider, ok, err := s.findConnectorProvider(connection.Provider)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("provider %q is not configured", connection.Provider)
	}
	var action connector.ProviderAction
	for _, candidate := range runtimeActionsForProviderConnection(connection, provider) {
		if candidate.Name == actionName {
			action = candidate
			break
		}
	}
	if action.Name == "" {
		return nil, fmt.Errorf("action %q is not granted for connection %q", actionName, alias)
	}
	body, err := runtimeActionRequestFromProviderAction(action, args)
	if err != nil {
		return nil, err
	}
	proxyReq := r.Clone(r.Context())
	proxyReq.Body = ioNopCloser{Reader: bytes.NewReader(body)}
	proxyReq.ContentLength = int64(len(body))
	rec := newRuntimeProxyRecorder()
	if err := s.proxyRuntimeAction(rec, proxyReq, principal, connection); err != nil {
		return nil, err
	}
	if rec.status < 200 || rec.status >= 300 {
		return nil, fmt.Errorf("runtime action returned HTTP %d: %s", rec.status, strings.TrimSpace(rec.body.String()))
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.body.Bytes(), &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (s *Server) mcpGatewayCallCustomMCPTool(r *http.Request, principal runtimeAgentPrincipal, alias, method string, args map[string]any) (map[string]any, error) {
	connection, ok, err := s.findRuntimeConnection(principal, "", alias)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("connection is not granted to this agent")
	}
	if connection.Provider != "custom-mcp" {
		return nil, fmt.Errorf("MCP gateway upstream calls are only implemented for custom-mcp connections")
	}
	reqBody, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  args,
	})
	proxyReq := r.Clone(r.Context())
	proxyReq.Body = ioNopCloser{Reader: bytes.NewReader(reqBody)}
	proxyReq.ContentLength = int64(len(reqBody))
	rec := newRuntimeProxyRecorder()
	if err := s.proxyCustomMCP(rec, proxyReq, principal, connection); err != nil {
		return nil, err
	}
	if rec.status < 200 || rec.status >= 300 {
		return nil, fmt.Errorf("runtime MCP returned HTTP %d: %s", rec.status, strings.TrimSpace(rec.body.String()))
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.body.Bytes(), &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func runtimeActionRequestFromProviderAction(action connector.ProviderAction, args map[string]any) ([]byte, error) {
	endpoint := action.Endpoint
	remaining := make(map[string]any, len(args))
	for key, value := range args {
		placeholder := "{" + key + "}"
		if strings.Contains(endpoint, placeholder) {
			endpoint = strings.ReplaceAll(endpoint, placeholder, url.PathEscape(fmt.Sprint(value)))
			continue
		}
		remaining[key] = value
	}
	if strings.Contains(endpoint, "{") || strings.Contains(endpoint, "}") {
		return nil, fmt.Errorf("missing path argument for endpoint %q", action.Endpoint)
	}
	req := runtimeActionProxyRequest{
		Method:   action.Method,
		Endpoint: endpoint,
	}
	if strings.EqualFold(action.Method, http.MethodGet) || strings.EqualFold(action.Method, http.MethodHead) {
		req.Query = make(map[string]string, len(remaining))
		for key, value := range remaining {
			req.Query[key] = fmt.Sprint(value)
		}
	} else if len(remaining) > 0 {
		body, err := json.Marshal(remaining)
		if err != nil {
			return nil, err
		}
		req.Body = body
	}
	return json.Marshal(req)
}

type runtimeProxyRecorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newRuntimeProxyRecorder() *runtimeProxyRecorder {
	return &runtimeProxyRecorder{header: http.Header{}, status: http.StatusOK}
}

func (r *runtimeProxyRecorder) Header() http.Header { return r.header }

func (r *runtimeProxyRecorder) WriteHeader(status int) { r.status = status }

func (r *runtimeProxyRecorder) Write(body []byte) (int, error) {
	return r.body.Write(body)
}

type ioNopCloser struct {
	*bytes.Reader
}

func (c ioNopCloser) Close() error { return nil }

func writeMCPGatewayResult(w http.ResponseWriter, id json.RawMessage, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(normalizeMCPID(id)), "result": result})
}

func writeMCPGatewayTextResult(w http.ResponseWriter, id json.RawMessage, value any) {
	body, _ := json.MarshalIndent(value, "", "  ")
	writeMCPGatewayResult(w, id, map[string]any{
		"content": []map[string]string{{"type": "text", "text": string(body)}},
	})
}

func writeMCPGatewayError(w http.ResponseWriter, id json.RawMessage, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      json.RawMessage(normalizeMCPID(id)),
		"error":   map[string]any{"code": code, "message": message},
	})
}

func normalizeMCPID(id json.RawMessage) []byte {
	if len(id) == 0 {
		return []byte("null")
	}
	return id
}

func stringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	if value, ok := args[key].(string); ok {
		return value
	}
	return ""
}
