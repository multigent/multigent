package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
)

type agentWorkerRuntimeConfig struct {
	Env        map[string]string       `json:"env,omitempty"`
	Sandbox    *entity.SandboxConfig   `json:"sandbox,omitempty"`
	AddDirs    []string                `json:"addDirs,omitempty"`
	RunCommand string                  `json:"runCommand,omitempty"`
	HTTPAgent  *entity.HTTPAgentConfig `json:"httpAgent,omitempty"`
}

func decodeAgentWorkerRuntimeConfig(worker controldb.AgentWorker) agentWorkerRuntimeConfig {
	var cfg agentWorkerRuntimeConfig
	if raw := strings.TrimSpace(worker.RuntimeConfigJSON); raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	return cfg
}

func encodeAgentWorkerRuntimeConfig(cfg agentWorkerRuntimeConfig) string {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return "{}"
	}
	return string(raw)
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
		Role:          strings.TrimSpace(membership.Role),
		Model:         model,
		RuntimeModel:  strings.TrimSpace(worker.RuntimeModel),
		RuntimeMode:   strings.TrimSpace(worker.DefaultRuntimeMode),
		RuntimeNodeID: strings.TrimSpace(worker.DefaultRuntimeNodeID),
		Provider:      strings.TrimSpace(worker.DefaultModelAccountID),
		Avatar:        strings.TrimSpace(worker.Avatar),
		HiredAt:       createdAt,
	}
	runtimeConfig := decodeAgentWorkerRuntimeConfig(worker)
	if runtimeConfig.Env != nil {
		meta.Env = runtimeConfig.Env
	}
	if runtimeConfig.Sandbox != nil {
		meta.Sandbox = runtimeConfig.Sandbox
	}
	if runtimeConfig.AddDirs != nil {
		meta.AddDirs = runtimeConfig.AddDirs
	}
	if strings.TrimSpace(runtimeConfig.RunCommand) != "" {
		meta.RunCommand = strings.TrimSpace(runtimeConfig.RunCommand)
	}
	if runtimeConfig.HTTPAgent != nil {
		meta.HTTPAgent = runtimeConfig.HTTPAgent
	}
	return meta, nil
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
