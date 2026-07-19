package runtimecli

import (
	"os"
	"os/exec"
	"path/filepath"
)

const (
	// BinDir is prepended to PATH inside agent sandboxes.
	BinDir = "/opt/multigent/mga/bin"

	// BinaryName is the CLI command exposed to agents. It is intentionally
	// separate from the human/admin `multigent` CLI contract.
	BinaryName = "mga"

	// BinaryPath is the absolute path where the agent runtime CLI is available
	// inside a sandbox.
	BinaryPath = BinDir + "/" + BinaryName

	// HostBinaryEnv overrides the host-side mga binary mounted into sandboxes.
	HostBinaryEnv = "MULTIGENT_AGENT_CLI"
)

// ResolveHostBinaryMount returns a read-only Docker volume mount for the
// host agent runtime binary as the sandbox agent runtime CLI.
//
// It deliberately refuses to mount the human/admin multigent binary as mga.
// Use MULTIGENT_AGENT_CLI or put a real mga binary on PATH in development.
func ResolveHostBinaryMount() string {
	binPath := ""
	if override := os.Getenv(HostBinaryEnv); override != "" {
		binPath = override
	} else if found, err := exec.LookPath(BinaryName); err == nil {
		binPath = found
	} else if exe, err := os.Executable(); err == nil && filepath.Base(exe) == BinaryName {
		binPath = exe
	} else if exe, err := os.Executable(); err == nil {
		// Development and packaged installs often place `multigent` and `mga`
		// side by side. The sandbox must mount the agent-scoped `mga` binary,
		// never the human/admin `multigent` binary under an alias.
		sibling := filepath.Join(filepath.Dir(exe), BinaryName)
		if _, err := os.Stat(sibling); err == nil {
			binPath = sibling
		}
	}
	if binPath == "" {
		return ""
	}
	resolved, err := filepath.EvalSymlinks(binPath)
	if err != nil {
		return ""
	}
	if filepath.Base(resolved) != BinaryName {
		return ""
	}
	if _, err := os.Stat(resolved); err != nil {
		return ""
	}
	return resolved + ":" + BinaryPath + ":ro"
}
