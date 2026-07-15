package api

import (
	"encoding/json"
	"net/http"

	controldb "github.com/multigent/multigent/internal/db"
)

func (s *Server) handleRuntimeMCPProxy(w http.ResponseWriter, r *http.Request) {
	s.handleRuntimeProxyPlaceholder(w, r, "mcp")
}

func (s *Server) handleRuntimeActionProxy(w http.ResponseWriter, r *http.Request) {
	s.handleRuntimeProxyPlaceholder(w, r, "action")
}

func (s *Server) handleRuntimeProxyPlaceholder(w http.ResponseWriter, r *http.Request, surface string) {
	principal, connection, ok := s.runtimeConnectionForRequest(w, r)
	if !ok {
		return
	}
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
