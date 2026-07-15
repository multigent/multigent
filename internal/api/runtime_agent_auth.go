package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/runtimeauth"
)

const ctxRuntimeAgentKey contextKey = "runtime-agent"

type runtimeAgentPrincipal = runtimeauth.Principal
type runtimeAgentTokenPayload = runtimeauth.Payload

type issueAgentRuntimeTokenRequest struct {
	RunID        string   `json:"runId"`
	TTLSeconds   int64    `json:"ttlSeconds"`
	Capabilities []string `json:"capabilities"`
}

func (s *Server) handleIssueAgentRuntimeToken(w http.ResponseWriter, r *http.Request) {
	project := strings.TrimSpace(r.PathValue("name"))
	agent := strings.TrimSpace(r.PathValue("agent"))
	if !s.checkProjectAccess(w, r, project) {
		return
	}
	if !s.agentExistsInProject(project, agent) {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeAgentNotFound, "agent not found")
		return
	}
	if !s.canOperateAgent(r, project, agent) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeAgentOperatorRequired, "agent operator access required")
		return
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	var body issueAgentRuntimeTokenRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := s.readJSON(w, r, &body); err != nil {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
			return
		}
	}
	ttl := time.Duration(body.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	if ttl > 24*time.Hour {
		ttl = 24 * time.Hour
	}
	caps := normalizeRuntimeCapabilities(body.Capabilities)
	token := s.issueAgentRuntimeToken(runtimeAgentTokenPayload{
		Type:         "agent_runtime",
		WorkspaceID:  workspaceID,
		Project:      project,
		Agent:        agent,
		RunID:        strings.TrimSpace(body.RunID),
		Capabilities: caps,
	}, ttl)
	expiresAt := time.Now().UTC().Add(ttl)
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "agent.runtime_token.issue",
		ResourceType: "agent",
		ResourceID:   project + "/" + agent,
		Summary:      "Agent runtime token issued",
		After: map[string]any{
			"project":      project,
			"agent":        agent,
			"runId":        strings.TrimSpace(body.RunID),
			"capabilities": caps,
			"expiresAt":    expiresAt.Format(time.RFC3339),
		},
		Request: r,
	})
	_ = json.NewEncoder(w).Encode(map[string]any{
		"tokenType":    "Bearer",
		"token":        token,
		"expiresAt":    expiresAt.Format(time.RFC3339),
		"ttlSeconds":   int64(ttl.Seconds()),
		"workspaceId":  workspaceID,
		"project":      project,
		"agent":        agent,
		"runId":        strings.TrimSpace(body.RunID),
		"capabilities": caps,
	})
}

func normalizeRuntimeCapabilities(caps []string) []string {
	if len(caps) == 0 {
		return []string{"connection.use"}
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(caps))
	for _, cap := range caps {
		cap = strings.TrimSpace(cap)
		if cap == "" || seen[cap] || !runtimeCapabilityAllowed(cap) {
			continue
		}
		seen[cap] = true
		out = append(out, cap)
	}
	if len(out) == 0 {
		return []string{"connection.use"}
	}
	return out
}

func runtimeCapabilityAllowed(capability string) bool {
	switch capability {
	case "connection.use":
		return true
	default:
		return false
	}
}

func (s *Server) issueAgentRuntimeToken(payload runtimeAgentTokenPayload, ttl time.Duration) string {
	return runtimeauth.Issue(s.runtimeSecret(), payload, ttl)
}

func (s *Server) validateAgentRuntimeToken(token string) (runtimeAgentPrincipal, bool) {
	return runtimeauth.Validate(s.runtimeSecret(), token)
}

func (s *Server) runtimeSecret() string {
	if s != nil && s.controlDB != nil {
		return runtimeauth.EnsureSecret(s.controlDB)
	}
	if s != nil && s.users != nil {
		return s.users.Secret()
	}
	return runtimeauth.GenerateSecret()
}

func (s *Server) withRuntimeAgentAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			s.jsonErrorCode(w, http.StatusUnauthorized, ErrCodeRuntimeAgentTokenRequired, "runtime agent token required")
			return
		}
		principal, ok := s.validateAgentRuntimeToken(token)
		if !ok {
			s.jsonErrorCode(w, http.StatusUnauthorized, ErrCodeRuntimeAgentTokenRequired, "invalid or expired runtime agent token")
			return
		}
		ctx := context.WithValue(r.Context(), ctxRuntimeAgentKey, principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func bearerToken(r *http.Request) string {
	if r == nil {
		return ""
	}
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	return strings.TrimSpace(r.URL.Query().Get("_token"))
}

func runtimeAgentFromRequest(r *http.Request) (runtimeAgentPrincipal, bool) {
	principal, ok := r.Context().Value(ctxRuntimeAgentKey).(runtimeAgentPrincipal)
	return principal, ok
}

func runtimeHasCapability(principal runtimeAgentPrincipal, capability string) bool {
	for _, cap := range principal.Capabilities {
		if cap == capability {
			return true
		}
	}
	return false
}

func (s *Server) runtimeConnectionForRequest(w http.ResponseWriter, r *http.Request) (runtimeAgentPrincipal, controldb.Connection, bool) {
	principal, ok := runtimeAgentFromRequest(r)
	if !ok {
		s.jsonErrorCode(w, http.StatusUnauthorized, ErrCodeRuntimeAgentTokenRequired, "runtime agent token required")
		return runtimeAgentPrincipal{}, controldb.Connection{}, false
	}
	if !runtimeHasCapability(principal, "connection.use") {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeRuntimeCapabilityRequired, "runtime token lacks connection.use capability")
		return runtimeAgentPrincipal{}, controldb.Connection{}, false
	}
	connectionID := strings.TrimSpace(r.Header.Get(agentConnectionManifest().ConnectionIDHeader))
	if connectionID == "" {
		connectionID = strings.TrimSpace(r.URL.Query().Get("connection"))
	}
	alias := strings.TrimSpace(r.Header.Get(agentConnectionManifest().ConnectionAliasHeader))
	if alias == "" {
		alias = strings.TrimSpace(r.URL.Query().Get("alias"))
	}
	connection, ok, err := s.findRuntimeConnection(principal, connectionID, alias)
	if err != nil {
		s.serverError(w, err)
		return runtimeAgentPrincipal{}, controldb.Connection{}, false
	}
	if !ok {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeRuntimeConnectionNotGranted, "connection is not granted to this agent")
		return runtimeAgentPrincipal{}, controldb.Connection{}, false
	}
	return principal, connection, true
}

func (s *Server) findRuntimeConnection(principal runtimeAgentPrincipal, connectionID, alias string) (controldb.Connection, bool, error) {
	connections, err := s.controlDB.ListConnections(controldb.ConnectionFilter{
		WorkspaceID: principal.WorkspaceID,
		Status:      "active",
	})
	if err != nil {
		return controldb.Connection{}, false, err
	}
	for _, connection := range connections {
		if connectionID != "" && connection.ID != connectionID {
			continue
		}
		if connectionID == "" && alias != "" && runtimeConnectionAlias(connection.Provider, connection.ConnectionName) != alias {
			continue
		}
		if connectionID == "" && alias == "" {
			continue
		}
		grants, err := s.controlDB.ListConnectionGrants(connection.ID)
		if err != nil {
			return controldb.Connection{}, false, err
		}
		if len(matchingAgentConnectionGrants(grants, principal.WorkspaceID, principal.Project, principal.Agent)) == 0 {
			continue
		}
		return connection, true, nil
	}
	return controldb.Connection{}, false, nil
}
