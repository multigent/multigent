package api

import (
	"fmt"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
)

type agentWorkerRuntimeConfig = entity.AgentWorkerRuntimeConfig

func decodeAgentWorkerRuntimeConfig(worker controldb.AgentWorker) agentWorkerRuntimeConfig {
	return entity.DecodeAgentWorkerRuntimeConfig(worker.RuntimeConfigJSON)
}

func encodeAgentWorkerRuntimeConfig(cfg agentWorkerRuntimeConfig) string {
	return cfg.Encode()
}

func (s *Server) agentMetaForProjectMember(workspaceID, project, agent string) (*entity.AgentMeta, error) {
	if s == nil || s.agentDirectory == nil || strings.TrimSpace(workspaceID) == "" {
		return nil, fmt.Errorf("agent worker directory is not available")
	}
	resolved, ok, resolveErr := s.agentDirectory.ProjectWorker(workspaceID, project, agent)
	if resolveErr != nil {
		return nil, resolveErr
	}
	if !ok {
		return nil, fmt.Errorf("agent %q not found in project %q", agent, project)
	}
	worker := resolved.Worker
	membership := resolved.Membership
	name := strings.TrimSpace(membership.Title)
	if name == "" {
		name = strings.TrimSpace(worker.DisplayName)
	}
	if name == "" {
		name = strings.TrimSpace(worker.Name)
	}
	model := entity.AgentModel(strings.TrimSpace(worker.Model))
	if model == "" {
		model = entity.ModelHuman
	}
	createdAt := time.Now().UTC()
	if ts, parseErr := time.Parse(time.RFC3339, worker.CreatedAt); parseErr == nil {
		createdAt = ts
	}
	meta := &entity.AgentMeta{
		Name:          name,
		Project:       project,
		Team:          strings.TrimSpace(worker.Team),
		Role:          strings.TrimSpace(worker.Role),
		Model:         model,
		RuntimeModel:  strings.TrimSpace(worker.RuntimeModel),
		RuntimeMode:   strings.TrimSpace(worker.DefaultRuntimeMode),
		RuntimeNodeID: strings.TrimSpace(worker.DefaultRuntimeNodeID),
		Provider:      strings.TrimSpace(worker.DefaultModelAccountID),
		Avatar:        strings.TrimSpace(worker.Avatar),
		HiredAt:       createdAt,
	}
	if meta.Role == "" {
		meta.Role = strings.TrimSpace(membership.Role)
	}
	if meta.Team == "" {
		meta.Team = s.projectMembershipTeam(membership, worker, s.projectMembershipRoleTeams())
	}
	decodeAgentWorkerRuntimeConfig(worker).ApplyTo(meta)
	return meta, nil
}

func agentMetaForWorker(worker controldb.AgentWorker) *entity.AgentMeta {
	name := strings.TrimSpace(worker.DisplayName)
	if name == "" {
		name = strings.TrimSpace(worker.Name)
	}
	model := entity.AgentModel(strings.TrimSpace(worker.Model))
	if model == "" {
		model = entity.ModelHuman
	}
	createdAt := time.Now().UTC()
	if ts, parseErr := time.Parse(time.RFC3339, worker.CreatedAt); parseErr == nil {
		createdAt = ts
	}
	meta := &entity.AgentMeta{
		Name:          name,
		Project:       "",
		Team:          strings.TrimSpace(worker.Team),
		Role:          strings.TrimSpace(worker.Role),
		Model:         model,
		RuntimeModel:  strings.TrimSpace(worker.RuntimeModel),
		RuntimeMode:   strings.TrimSpace(worker.DefaultRuntimeMode),
		RuntimeNodeID: strings.TrimSpace(worker.DefaultRuntimeNodeID),
		Provider:      strings.TrimSpace(worker.DefaultModelAccountID),
		Avatar:        strings.TrimSpace(worker.Avatar),
		HiredAt:       createdAt,
	}
	decodeAgentWorkerRuntimeConfig(worker).ApplyTo(meta)
	return meta
}

func (s *Server) projectAgentNames(workspaceID, project string) ([]string, error) {
	agents, err := s.projectScheduleAgents(workspaceID, project)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(agents))
	seen := map[string]bool{}
	for _, agent := range agents {
		name := strings.TrimSpace(agent.Name)
		if name == "" || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		out = append(out, name)
	}
	return out, nil
}
