// Package agentcli describes and bootstraps agent CLI toolchains inside
// isolated runtimes.
package agentcli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"github.com/multigent/multigent/internal/entity"
)

const (
	ToolchainHome = "/opt/multigent/toolchains"
	ToolchainBin  = ToolchainHome + "/npm/bin"
)

var safeShellToken = regexp.MustCompile(`^[A-Za-z0-9._/@:+-]+$`)

// DefaultForModel returns the default managed CLI install plan for a model.
//
// These defaults are intentionally data-like and can later move to a DB-backed
// installer catalog. Unknown or hard-to-standardize CLIs return nil so callers
// can use a custom runtime template instead of silently guessing.
func DefaultForModel(model entity.AgentModel) *entity.AgentCLIConfig {
	switch entity.NormaliseModel(model) {
	case entity.ModelCodex, entity.ModelQoder:
		return &entity.AgentCLIConfig{
			Vendor:         "codex",
			Version:        "latest",
			Channel:        "stable",
			Binary:         "codex",
			PackageManager: "npm",
			Package:        "@openai/codex",
		}
	case entity.ModelClaudeCode:
		return &entity.AgentCLIConfig{
			Vendor:         "claude-code",
			Version:        "latest",
			Channel:        "stable",
			Binary:         "claude",
			PackageManager: "npm",
			Package:        "@anthropic-ai/claude-code",
		}
	case entity.ModelGemini:
		return &entity.AgentCLIConfig{
			Vendor:         "gemini",
			Version:        "latest",
			Channel:        "stable",
			Binary:         "gemini",
			PackageManager: "npm",
			Package:        "@google/gemini-cli",
		}
	default:
		return nil
	}
}

// Effective returns cfg if set, otherwise the model default.
func Effective(model entity.AgentModel, cfg *entity.AgentCLIConfig) *entity.AgentCLIConfig {
	if cfg != nil {
		copy := *cfg
		return Normalize(&copy)
	}
	return Normalize(DefaultForModel(model))
}

// WrapCommand prepends a runtime bootstrap script that installs or verifies the
// configured CLI before executing the agent command. The install is idempotent
// through a marker under ToolchainHome, which should be backed by a persistent
// volume or cache in the runtime provider.
func WrapCommand(cmd []string, cfg *entity.AgentCLIConfig) []string {
	if len(cmd) == 0 || cfg == nil || cfg.PackageManager == "none" {
		return cmd
	}
	script := BootstrapScript(cfg)
	if script == "" {
		return cmd
	}
	wrapped := []string{"/bin/sh", "-lc", script + "\nexec \"$@\"", "--"}
	wrapped = append(wrapped, cmd...)
	return wrapped
}

// BootstrapScript returns a POSIX shell script for installing/verifying cfg.
func BootstrapScript(cfg *entity.AgentCLIConfig) string {
	if cfg == nil {
		return ""
	}
	cfg = Normalize(cfg)
	if len(cfg.Install) > 0 {
		return scriptInstaller(cfg)
	}
	switch cfg.PackageManager {
	case "", "npm":
		if cfg.Package == "" || cfg.Binary == "" {
			return ""
		}
		return npmInstaller(cfg)
	case "script":
		return scriptInstaller(cfg)
	default:
		return ""
	}
}

// Normalize returns a copy of cfg with product-safe defaults. It also cleans up
// early Multigent UI presets that referenced Codex versions never published to
// npm, which would otherwise fail every runtime bootstrap with npm ETARGET.
func Normalize(cfg *entity.AgentCLIConfig) *entity.AgentCLIConfig {
	if cfg == nil {
		return nil
	}
	out := *cfg
	vendor := strings.TrimSpace(out.Vendor)
	pkg := strings.TrimSpace(out.Package)
	version := strings.TrimSpace(out.Version)
	if version == "" {
		version = "latest"
	}
	if (vendor == "codex" || vendor == "qoder" || pkg == "@openai/codex") && isRemovedCodexPreset(version) {
		version = "latest"
	}
	out.Version = version
	return &out
}

func isRemovedCodexPreset(version string) bool {
	switch version {
	case "0.18.0", "0.17.0", "0.16.0":
		return true
	default:
		return false
	}
}

func npmInstaller(cfg *entity.AgentCLIConfig) string {
	pkg := cfg.Package
	if version := strings.TrimSpace(cfg.Version); version != "" && version != "latest" {
		pkg += "@" + version
	}
	marker := markerPath(cfg)
	return strings.Join([]string{
		"set -eu",
		"export MULTIGENT_TOOLCHAIN_HOME=" + shellQuote(ToolchainHome),
		"export NPM_CONFIG_PREFIX=\"$MULTIGENT_TOOLCHAIN_HOME/npm\"",
		"export PATH=\"$NPM_CONFIG_PREFIX/bin:$PATH\"",
		"mkdir -p \"$NPM_CONFIG_PREFIX\" \"$MULTIGENT_TOOLCHAIN_HOME/markers\"",
		fmt.Sprintf("if [ ! -f %s ] || [ \"${MULTIGENT_AGENT_CLI_FORCE_INSTALL:-}\" = \"1\" ]; then", shellQuote(marker)),
		fmt.Sprintf("  npm install -g %s", shellQuote(pkg)),
		fmt.Sprintf("  touch %s", shellQuote(marker)),
		"fi",
		fmt.Sprintf("command -v %s >/dev/null 2>&1", shellQuote(cfg.Binary)),
	}, "\n")
}

func scriptInstaller(cfg *entity.AgentCLIConfig) string {
	lines := []string{
		"set -eu",
		"export MULTIGENT_TOOLCHAIN_HOME=" + shellQuote(ToolchainHome),
		"export PATH=" + shellQuote(ToolchainBin) + ":$PATH",
		"mkdir -p \"$MULTIGENT_TOOLCHAIN_HOME/markers\"",
	}
	lines = append(lines, cfg.Install...)
	if cfg.Binary != "" {
		lines = append(lines, fmt.Sprintf("command -v %s >/dev/null 2>&1", shellQuote(cfg.Binary)))
	}
	lines = append(lines, cfg.Check...)
	return strings.Join(lines, "\n")
}

func markerPath(cfg *entity.AgentCLIConfig) string {
	key := strings.Join([]string{cfg.Vendor, cfg.PackageManager, cfg.Package, cfg.Version, cfg.Channel}, "\x00")
	sum := sha256.Sum256([]byte(key))
	return ToolchainHome + "/markers/" + hex.EncodeToString(sum[:8])
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if safeShellToken.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
