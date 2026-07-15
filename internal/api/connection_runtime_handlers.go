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
	return agentRuntimeConnectionResponse{
		ID:             connection.ID,
		Provider:       connection.Provider,
		ConnectionName: connection.ConnectionName,
		OwnerType:      connection.OwnerType,
		OwnerID:        connection.OwnerID,
		AuthType:       connection.AuthType,
		Profile:        sanitizeRuntimeConnectionProfile(connection.Provider, profile),
		MatchedGrants:  grantsToResponse(grants),
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
