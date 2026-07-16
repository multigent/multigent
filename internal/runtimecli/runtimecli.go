package runtimecli

import (
	"os"
	"path/filepath"
)

const (
	// BinDir is prepended to PATH inside agent sandboxes.
	BinDir = "/opt/multigent/agent-cli/bin"

	// BinaryName is the CLI command exposed to agents. It is intentionally
	// separate from the human/admin `multigent` CLI contract.
	BinaryName = "multigent-agent"

	// BinaryPath is the absolute path where the agent runtime CLI is available
	// inside a sandbox.
	BinaryPath = BinDir + "/" + BinaryName
)

// ResolveHostBinaryMount returns a read-only Docker volume mount for the
// current Multigent binary as the sandbox agent runtime CLI.
//
// This is a transitional packaging mechanism: the sandbox-visible command is
// already named multigent-agent, while the implementation can later become a
// separate restricted binary without changing sandbox PATH or prompts.
func ResolveHostBinaryMount() string {
	binPath, err := os.Executable()
	if err != nil {
		return ""
	}
	binPath, err = filepath.EvalSymlinks(binPath)
	if err != nil {
		return ""
	}
	if _, err := os.Stat(binPath); err != nil {
		return ""
	}
	return binPath + ":" + BinaryPath + ":ro"
}
