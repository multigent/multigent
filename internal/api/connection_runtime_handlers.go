package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	controldb "github.com/multigent/multigent/internal/db"
)

type agentRuntimeConnectionResponse struct {
	ID             string                 `json:"id"`
	Provider       string                 `json:"provider"`
	ConnectionName string                 `json:"connectionName"`
	OwnerType      string                 `json:"ownerType"`
	OwnerID        string                 `json:"ownerId"`
	AuthType       string                 `json:"authType"`
	Profile        map[string]any         `json:"profile"`
	MatchedGrants  []connectionGrantModel `json:"matchedGrants"`
	Runtime        connectionRuntimeSpec  `json:"runtime"`
}

type agentRuntimeConnectionManifest struct {
	Version               string `json:"version"`
	ConnectionsFileEnv    string `json:"connectionsFileEnv"`
	APIBaseURLEnv         string `json:"apiBaseUrlEnv"`
	AgentTokenEnv         string `json:"agentTokenEnv"`
	MCPProxyPath          string `json:"mcpProxyPath"`
	ActionProxyPath       string `json:"actionProxyPath"`
	ConnectionAliasHeader string `json:"connectionAliasHeader"`
	ConnectionIDHeader    string `json:"connectionIdHeader"`
}

type connectionRuntimeSpec struct {
	Alias       string                     `json:"alias"`
	Env         map[string]string          `json:"env"`
	MCPProxy    connectionRuntimeProxySpec `json:"mcpProxy"`
	ActionProxy connectionRuntimeProxySpec `json:"actionProxy"`
}

type connectionRuntimeProxySpec struct {
	URLFromEnv  string                    `json:"urlFromEnv"`
	Path        string                    `json:"path"`
	Headers     []connectionRuntimeHeader `json:"headers"`
	Query       map[string]string         `json:"query,omitempty"`
	Description string                    `json:"description,omitempty"`
}

type connectionRuntimeHeader struct {
	Name         string `json:"name"`
	Value        string `json:"value,omitempty"`
	ValueFromEnv string `json:"valueFromEnv,omitempty"`
}

func (s *Server) handleAgentRuntimeConnections(w http.ResponseWriter, r *http.Request) {
	project := strings.TrimSpace(r.PathValue("name"))
	agent := strings.TrimSpace(r.PathValue("agent"))
	if !s.checkProjectAccess(w, r, project) {
		return
	}
	if !s.agentExistsInProject(project, agent) {
		s.jsonError(w, http.StatusNotFound, "agent not found")
		return
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	connections, err := s.controlDB.ListConnections(controldb.ConnectionFilter{
		WorkspaceID: workspaceID,
		Status:      "active",
	})
	if err != nil {
		s.serverError(w, err)
		return
	}

	out := make([]agentRuntimeConnectionResponse, 0)
	for _, connection := range connections {
		grants, err := s.controlDB.ListConnectionGrants(connection.ID)
		if err != nil {
			s.serverError(w, err)
			return
		}
		matched := matchingAgentConnectionGrants(grants, workspaceID, project, agent)
		if len(matched) == 0 {
			continue
		}
		out = append(out, agentRuntimeConnectionToResponse(connection, matched))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		if out[i].ConnectionName != out[j].ConnectionName {
			return out[i].ConnectionName < out[j].ConnectionName
		}
		return out[i].ID < out[j].ID
	})
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "connection.use",
		ResourceType: "agent",
		ResourceID:   project + "/" + agent,
		Summary:      "Agent runtime connections resolved",
		After: map[string]any{
			"project":       project,
			"agent":         agent,
			"connectionIds": runtimeConnectionIDs(out),
			"count":         len(out),
		},
		Request: r,
	})
	_ = json.NewEncoder(w).Encode(map[string]any{
		"project":     project,
		"agent":       agent,
		"manifest":    agentConnectionManifest(),
		"connections": out,
	})
}

func matchingAgentConnectionGrants(grants []controldb.ConnectionGrant, workspaceID, project, agent string) []controldb.ConnectionGrant {
	out := make([]controldb.ConnectionGrant, 0, len(grants))
	for _, grant := range grants {
		if connectionGrantMatchesAgent(grant, workspaceID, project, agent) {
			out = append(out, grant)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out
}

func connectionGrantMatchesAgent(grant controldb.ConnectionGrant, workspaceID, project, agent string) bool {
	targetID := strings.TrimSpace(grant.TargetID)
	switch strings.TrimSpace(grant.TargetType) {
	case ConnectionTargetWorkspace:
		return targetID != "" && targetID == workspaceID
	case ConnectionTargetProject:
		return targetID != "" && targetID == project
	case ConnectionTargetAgent:
		return targetID != "" && targetID == project+"/"+agent
	default:
		return false
	}
}

func agentRuntimeConnectionToResponse(connection controldb.Connection, grants []controldb.ConnectionGrant) agentRuntimeConnectionResponse {
	profile := map[string]any{}
	_ = json.Unmarshal([]byte(connection.ProfileJSON), &profile)
	alias := runtimeConnectionAlias(connection.Provider, connection.ConnectionName)
	return agentRuntimeConnectionResponse{
		ID:             connection.ID,
		Provider:       connection.Provider,
		ConnectionName: connection.ConnectionName,
		OwnerType:      connection.OwnerType,
		OwnerID:        connection.OwnerID,
		AuthType:       connection.AuthType,
		Profile:        sanitizeRuntimeConnectionProfile(connection.Provider, profile),
		MatchedGrants:  grantsToResponse(grants),
		Runtime:        runtimeSpecForConnection(connection, alias),
	}
}

func agentConnectionManifest() agentRuntimeConnectionManifest {
	return agentRuntimeConnectionManifest{
		Version:               "multigent.connections.v1",
		ConnectionsFileEnv:    "MULTIGENT_CONNECTIONS_FILE",
		APIBaseURLEnv:         "MULTIGENT_API_URL",
		AgentTokenEnv:         "MULTIGENT_AGENT_TOKEN",
		MCPProxyPath:          "/api/v1/runtime/mcp",
		ActionProxyPath:       "/api/v1/runtime/actions",
		ConnectionAliasHeader: "X-Multigent-Connection-Alias",
		ConnectionIDHeader:    "X-Multigent-Connection-ID",
	}
}

func runtimeSpecForConnection(connection controldb.Connection, alias string) connectionRuntimeSpec {
	manifest := agentConnectionManifest()
	return connectionRuntimeSpec{
		Alias: alias,
		Env: map[string]string{
			"MULTIGENT_CONNECTION_ALIAS":    alias,
			"MULTIGENT_CONNECTION_ID":       connection.ID,
			"MULTIGENT_CONNECTION_PROVIDER": connection.Provider,
		},
		MCPProxy: connectionRuntimeProxySpec{
			URLFromEnv: manifest.APIBaseURLEnv,
			Path:       manifest.MCPProxyPath,
			Headers: []connectionRuntimeHeader{
				{Name: "Authorization", ValueFromEnv: manifest.AgentTokenEnv},
				{Name: manifest.ConnectionAliasHeader, Value: alias},
				{Name: manifest.ConnectionIDHeader, Value: connection.ID},
			},
			Query: map[string]string{
				"connection": connection.ID,
				"alias":      alias,
			},
			Description: "Use this MCP proxy with the scoped agent token. Raw provider credentials are held by Multigent.",
		},
		ActionProxy: connectionRuntimeProxySpec{
			URLFromEnv: manifest.APIBaseURLEnv,
			Path:       manifest.ActionProxyPath,
			Headers: []connectionRuntimeHeader{
				{Name: "Authorization", ValueFromEnv: manifest.AgentTokenEnv},
				{Name: manifest.ConnectionAliasHeader, Value: alias},
				{Name: manifest.ConnectionIDHeader, Value: connection.ID},
			},
			Query: map[string]string{
				"connection": connection.ID,
				"alias":      alias,
			},
			Description: "Use this action proxy for provider actions. The agent token must authorize the current run.",
		},
	}
}

func sanitizeRuntimeConnectionProfile(providerID string, profile map[string]any) map[string]any {
	secretKeys := map[string]bool{
		"apiKey":    true,
		"appSecret": true,
		"password":  true,
		"secret":    true,
		"token":     true,
	}
	if provider, ok := findConnectorProvider(providerID); ok {
		for _, field := range provider.Fields {
			if field.Secret {
				secretKeys[field.Key] = true
			}
		}
	}
	out := make(map[string]any, len(profile))
	for key, value := range profile {
		if secretKeys[key] {
			continue
		}
		out[key] = value
	}
	return out
}

func runtimeConnectionIDs(connections []agentRuntimeConnectionResponse) []string {
	ids := make([]string, 0, len(connections))
	for _, connection := range connections {
		ids = append(ids, connection.ID)
	}
	return ids
}

func runtimeConnectionAlias(provider, connectionName string) string {
	base := strings.TrimSpace(provider)
	name := strings.TrimSpace(connectionName)
	if name != "" && name != "default" {
		base += "-" + name
	}
	base = strings.ToLower(base)
	var b strings.Builder
	lastDash := false
	for _, r := range base {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "connection"
	}
	return out
}
