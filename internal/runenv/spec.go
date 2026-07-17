// Package runenv defines provider-neutral runtime execution specs.
package runenv

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/multigent/multigent/internal/agentcli"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/runtimecli"
	"github.com/multigent/multigent/internal/sandbox"
)

const (
	MountModeReadOnly  = "ro"
	MountModeReadWrite = "rw"
)

// ProcessSpec is the provider-neutral description of one agent subprocess run.
type ProcessSpec struct {
	WorkspaceRoot string
	Project       string
	Agent         string
	AgentDir      string
	Model         entity.AgentModel

	Command []string
	Env     map[string]string

	Runtime  *entity.SandboxConfig
	AgentCLI *entity.AgentCLIConfig
	Mounts   []entity.RuntimeMount
	Limits   entity.RuntimeResourceLimits
}

// Provider prepares and starts an isolated runtime.
//
// The current Docker path still returns an executable + argv because the
// runner streams stdout/stderr itself. Cloud providers such as E2B can later
// implement the same interface with remote process handles.
type Provider interface {
	Name() entity.SandboxProvider
	Available() error
	Command(spec ProcessSpec) (executable string, args []string, err error)
}

// DockerProvider adapts the existing Docker sandbox implementation to the
// provider-neutral runtime model.
type DockerProvider struct{}

func (DockerProvider) Name() entity.SandboxProvider { return entity.SandboxDocker }

func (DockerProvider) Available() error { return sandbox.CheckDocker() }

func (DockerProvider) Command(spec ProcessSpec) (string, []string, error) {
	cfg := DockerConfig(spec.Runtime)
	cfg.ExtraVolumes = append(cfg.ExtraVolumes, "multigent-toolchains:"+agentcli.ToolchainHome)
	pathParts := []string{}
	if toolBin := runtimeEnvValue(spec.Runtime, "MULTIGENT_TOOL_BIN_DIR"); toolBin != "" {
		pathParts = append(pathParts, toolBin)
	}
	pathParts = append(pathParts, runtimecli.BinDir, agentcli.ToolchainBin, sandbox.UserBin, sandbox.ContainerDefaultPATH)
	cfg.ExtraEnv = append(cfg.ExtraEnv, "PATH="+strings.Join(pathParts, ":"))
	for _, mount := range spec.Mounts {
		volume := DockerVolume(mount)
		if volume != "" {
			cfg.ExtraVolumes = append(cfg.ExtraVolumes, volume)
		}
	}
	command := agentcli.WrapCommand(spec.Command, spec.AgentCLI)
	return sandbox.RunArgs(spec.AgentDir, spec.Model, cfg, command)
}

func runtimeEnvValue(runtime *entity.SandboxConfig, name string) string {
	if runtime == nil || name == "" {
		return ""
	}
	for _, env := range runtime.Env {
		if env.Name == name && env.Value != "" {
			return env.Value
		}
	}
	return ""
}

// E2BProvider represents a self-hosted E2B-compatible runtime. It is currently
// wired for capability detection and explicit configuration validation; process
// lifecycle execution will be implemented against the selected self-hosted E2B
// API once that deployment contract is fixed.
type E2BProvider struct{}

func (E2BProvider) Name() entity.SandboxProvider { return entity.SandboxE2B }

func (E2BProvider) Available() error {
	caps := sandbox.DetectCapabilities()
	if !caps.E2B.Available {
		return fmt.Errorf("e2b runtime unavailable: %s", caps.E2B.Reason)
	}
	return nil
}

func (E2BProvider) Command(ProcessSpec) (string, []string, error) {
	return "", nil, fmt.Errorf("e2b runtime execution is not implemented yet; configure Docker/gVisor until the self-hosted E2B API adapter is connected")
}

// ProviderFor returns the local provider adapter for provider.
func ProviderFor(provider entity.SandboxProvider) (Provider, bool) {
	switch provider {
	case entity.SandboxDocker:
		return DockerProvider{}, true
	case entity.SandboxE2B:
		return E2BProvider{}, true
	default:
		return nil, false
	}
}

// DockerConfig converts provider-neutral runtime fields plus docker-specific
// overrides into the legacy DockerSandboxConfig expected by sandbox.BuildArgs.
func DockerConfig(runtime *entity.SandboxConfig) *entity.DockerSandboxConfig {
	cfg := &entity.DockerSandboxConfig{}
	if runtime == nil {
		return cfg
	}
	if runtime.Docker != nil {
		*cfg = *runtime.Docker
		cfg.ExtraVolumes = append([]string(nil), runtime.Docker.ExtraVolumes...)
		cfg.ExtraEnv = append([]string(nil), runtime.Docker.ExtraEnv...)
		cfg.CredentialMounts = append([]string(nil), runtime.Docker.CredentialMounts...)
	}
	if runtime.Image != "" && cfg.Image == "" {
		cfg.Image = runtime.Image
	}
	if runtime.NetworkMode != "" && cfg.NetworkMode == "" {
		cfg.NetworkMode = runtime.NetworkMode
	}
	if runtime.Resources.MemoryMB > 0 && cfg.MemoryMB == 0 {
		cfg.MemoryMB = runtime.Resources.MemoryMB
	}
	if runtime.Resources.CPUs > 0 && cfg.CPUs == 0 {
		cfg.CPUs = runtime.Resources.CPUs
	}
	for _, env := range runtime.Env {
		if env.Name == "" {
			continue
		}
		if env.Inherit {
			cfg.ExtraEnv = append(cfg.ExtraEnv, env.Name)
		} else if env.Value != "" {
			cfg.ExtraEnv = append(cfg.ExtraEnv, env.Name+"="+env.Value)
		}
	}
	return cfg
}

// DockerVolume renders one provider-neutral mount as a Docker -v value.
func DockerVolume(m entity.RuntimeMount) string {
	if m.Source == "" {
		return ""
	}
	target := m.Target
	if target == "" {
		target = m.Source
	}
	mode := m.Mode
	if mode == "" {
		mode = MountModeReadWrite
	}
	return m.Source + ":" + target + ":" + mode
}

// AddPathMount appends an existing host path as a runtime mount.
func AddPathMount(mounts []entity.RuntimeMount, path, kind, mode string) []entity.RuntimeMount {
	if path == "" {
		return mounts
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return mounts
	}
	if _, err := os.Stat(abs); err != nil {
		return mounts
	}
	return append(mounts, entity.RuntimeMount{
		Source: abs,
		Target: abs,
		Mode:   mode,
		Kind:   kind,
	})
}
