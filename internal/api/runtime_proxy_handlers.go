package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
	s.handleRuntimeProxyUnsupported(w, r, principal, connection, "action")
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

func redactRuntimeProxyResponse(body []byte, token string) []byte {
	token = strings.TrimSpace(token)
	if token == "" || len(body) == 0 {
		return body
	}
	body = bytes.ReplaceAll(body, []byte("Bearer "+token), []byte("Bearer [redacted]"))
	body = bytes.ReplaceAll(body, []byte(token), []byte("[redacted]"))
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
