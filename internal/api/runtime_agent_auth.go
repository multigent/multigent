package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
)

const ctxRuntimeAgentKey contextKey = "runtime-agent"

type runtimeAgentPrincipal struct {
	WorkspaceID  string   `json:"workspaceId"`
	Project      string   `json:"project"`
	Agent        string   `json:"agent"`
	RunID        string   `json:"runId,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Exp          int64    `json:"exp"`
	Iat          int64    `json:"iat"`
}

type runtimeAgentTokenPayload struct {
	Type         string   `json:"typ"`
	WorkspaceID  string   `json:"workspaceId"`
	Project      string   `json:"project"`
	Agent        string   `json:"agent"`
	RunID        string   `json:"runId,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Exp          int64    `json:"exp"`
	Iat          int64    `json:"iat"`
}

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
		s.jsonError(w, http.StatusNotFound, "agent not found")
		return
	}
	cur := s.currentUser(r)
	if !s.canManageProject(r, project) && !currentUserLinkedAgent(cur, project+"/"+agent) {
		s.jsonError(w, http.StatusForbidden, "agent operator access required")
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
			s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
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
		if cap == "" || seen[cap] {
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

func (s *Server) issueAgentRuntimeToken(payload runtimeAgentTokenPayload, ttl time.Duration) string {
	now := time.Now().UTC()
	payload.Type = "agent_runtime"
	payload.Iat = now.Unix()
	payload.Exp = now.Add(ttl).Unix()
	header := base64Encode([]byte(`{"alg":"HS256","typ":"JWT"}`))
	raw, _ := json.Marshal(payload)
	body := base64Encode(raw)
	sig := s.runtimeHMACSign(header + "." + body)
	return header + "." + body + "." + sig
}

func (s *Server) validateAgentRuntimeToken(token string) (runtimeAgentPrincipal, bool) {
	parts := strings.SplitN(strings.TrimSpace(token), ".", 3)
	if len(parts) != 3 {
		return runtimeAgentPrincipal{}, false
	}
	if !hmac.Equal([]byte(parts[2]), []byte(s.runtimeHMACSign(parts[0]+"."+parts[1]))) {
		return runtimeAgentPrincipal{}, false
	}
	raw, err := base64Decode(parts[1])
	if err != nil {
		return runtimeAgentPrincipal{}, false
	}
	var payload runtimeAgentTokenPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return runtimeAgentPrincipal{}, false
	}
	if payload.Type != "agent_runtime" || payload.WorkspaceID == "" || payload.Project == "" || payload.Agent == "" {
		return runtimeAgentPrincipal{}, false
	}
	if time.Now().Unix() > payload.Exp {
		return runtimeAgentPrincipal{}, false
	}
	return runtimeAgentPrincipal{
		WorkspaceID:  payload.WorkspaceID,
		Project:      payload.Project,
		Agent:        payload.Agent,
		RunID:        payload.RunID,
		Capabilities: payload.Capabilities,
		Exp:          payload.Exp,
		Iat:          payload.Iat,
	}, true
}

func (s *Server) runtimeHMACSign(msg string) string {
	secret := ""
	if s != nil && s.controlDB != nil {
		if value, ok, err := s.controlDB.GetSetting("agent_runtime_secret"); err == nil && ok && value != "" {
			secret = value
		}
		if secret == "" {
			secret = generateSecret()
			_ = s.controlDB.SetSetting("agent_runtime_secret", secret)
		}
	}
	if secret == "" && s != nil && s.users != nil {
		secret = s.users.Secret()
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msg))
	return base64Encode(mac.Sum(nil))
}

func (s *Server) withRuntimeAgentAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			s.jsonError(w, http.StatusUnauthorized, "runtime agent token required")
			return
		}
		principal, ok := s.validateAgentRuntimeToken(token)
		if !ok {
			s.jsonError(w, http.StatusUnauthorized, "invalid or expired runtime agent token")
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
		s.jsonError(w, http.StatusUnauthorized, "runtime agent token required")
		return runtimeAgentPrincipal{}, controldb.Connection{}, false
	}
	if !runtimeHasCapability(principal, "connection.use") {
		s.jsonError(w, http.StatusForbidden, "runtime token lacks connection.use capability")
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
		s.jsonError(w, http.StatusForbidden, "connection is not granted to this agent")
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
