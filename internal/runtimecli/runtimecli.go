package runtimecli

import (
	"encoding/binary"
	"os"
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

// ResolveHostBinaryMount returns a read-only Docker volume mount for an
// explicitly configured Linux agent runtime binary.
//
// Published runtime images already contain a target-platform mga binary. Do
// not auto-discover the native host binary here: on macOS and Windows that
// would shadow the Linux binary in the image and fail with exec format error.
// MULTIGENT_AGENT_CLI remains available for development and local image
// overrides, but the configured binary must be an ELF executable.
func ResolveHostBinaryMount() string {
	binPath := os.Getenv(HostBinaryEnv)
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
	if !isELFExecutable(resolved) {
		return ""
	}
	return resolved + ":" + BinaryPath + ":ro"
}

func isELFExecutable(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	var magic uint32
	if err := binary.Read(f, binary.BigEndian, &magic); err != nil {
		return false
	}
	return magic == 0x7f454c46
}
