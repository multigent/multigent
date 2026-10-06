package entity

import (
	"encoding/json"
	"strings"
)

// AgentWorkerRuntimeConfig is the runtime_config_json payload stored on a
// 2.x agent worker. It carries the per-worker execution settings that 1.x kept
// in the agent's agent.yaml.
type AgentWorkerRuntimeConfig struct {
	Env        map[string]string `json:"env,omitempty"`
	Sandbox    *SandboxConfig    `json:"sandbox,omitempty"`
	AddDirs    []string          `json:"addDirs,omitempty"`
	RunCommand string            `json:"runCommand,omitempty"`
	HTTPAgent  *HTTPAgentConfig  `json:"httpAgent,omitempty"`
}

// legacyAgentWorkerRuntimeConfig lists the snake_case keys written by 1.x
// tooling and by hand-edited migrations. They are read as fallbacks so a
// worker never silently loses its run command or extra directories.
type legacyAgentWorkerRuntimeConfig struct {
	AddDirs    []string         `json:"add_dirs,omitempty"`
	RunCommand string           `json:"run_command,omitempty"`
	Command    string           `json:"command,omitempty"`
	HTTPAgent  *HTTPAgentConfig `json:"http_agent,omitempty"`
}

// DecodeAgentWorkerRuntimeConfig parses runtime_config_json, accepting the
// legacy snake_case keys when the canonical camelCase key is absent.
func DecodeAgentWorkerRuntimeConfig(raw string) AgentWorkerRuntimeConfig {
	var cfg AgentWorkerRuntimeConfig
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return cfg
	}
	_ = json.Unmarshal([]byte(raw), &cfg)
	var legacy legacyAgentWorkerRuntimeConfig
	_ = json.Unmarshal([]byte(raw), &legacy)
	if cfg.AddDirs == nil && legacy.AddDirs != nil {
		cfg.AddDirs = legacy.AddDirs
	}
	if strings.TrimSpace(cfg.RunCommand) == "" {
		cfg.RunCommand = firstNonBlank(legacy.RunCommand, legacy.Command)
	}
	cfg.RunCommand = strings.TrimSpace(cfg.RunCommand)
	if cfg.HTTPAgent == nil {
		cfg.HTTPAgent = legacy.HTTPAgent
	}
	return cfg
}

// Encode returns the canonical JSON form. Legacy keys are not written back.
func (c AgentWorkerRuntimeConfig) Encode() string {
	raw, err := json.Marshal(c)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

// ApplyTo copies the configured runtime settings onto meta, leaving fields
// that are not configured untouched.
func (c AgentWorkerRuntimeConfig) ApplyTo(meta *AgentMeta) {
	if meta == nil {
		return
	}
	if c.Env != nil {
		meta.Env = c.Env
	}
	if c.Sandbox != nil {
		meta.Sandbox = c.Sandbox
	}
	if c.AddDirs != nil {
		meta.AddDirs = c.AddDirs
	}
	if c.RunCommand != "" {
		meta.RunCommand = c.RunCommand
	}
	if c.HTTPAgent != nil {
		meta.HTTPAgent = c.HTTPAgent
	}
}

func firstNonBlank(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
