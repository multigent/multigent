package runtimecli

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const (
	// BinDir is prepended to PATH inside agent sandboxes.
	BinDir = "/opt/multigent/mga/bin"

	// ManagedBinDir contains the runtime CLI synchronized for the currently
	// running multigent server version. It lives in the shared toolchain volume
	// and is preferred over the image-bundled fallback.
	ManagedBinDir = "/opt/multigent/toolchains/mga/bin"

	// BinaryName is the CLI command exposed to agents. It is intentionally
	// separate from the human/admin `multigent` CLI contract.
	BinaryName = "mga"

	// BinaryPath is the absolute path where the agent runtime CLI is available
	// inside a sandbox.
	BinaryPath = BinDir + "/" + BinaryName

	// ManagedBinaryPath is the preferred runtime CLI location inside a sandbox.
	ManagedBinaryPath = ManagedBinDir + "/" + BinaryName

	// HostBinaryEnv overrides the host-side mga binary mounted into sandboxes.
	HostBinaryEnv = "MULTIGENT_AGENT_CLI"

	// ServerVersionEnv is set by the multigent server process and used by the
	// Docker bootstrap to synchronize the matching Linux mga release asset.
	ServerVersionEnv = "MULTIGENT_SERVER_VERSION"
)

var releaseVersion = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z][0-9A-Za-z.-]*)?$`)

// BootstrapScript returns a POSIX shell snippet that prepares the runtime CLI
// inside a sandbox. Release builds download the matching Linux mga binary into
// the toolchain volume; dev/git-describe builds fall back to the image-bundled
// mga so local unreleased commits do not require published assets.
func BootstrapScript(version string) string {
	version = strings.TrimSpace(version)
	if version == "" || version == "dev" {
		return strings.Join([]string{
			"export PATH=" + shellQuote(ManagedBinDir) + ":$PATH",
			"command -v " + shellQuote(BinaryName) + " >/dev/null 2>&1",
		}, "\n")
	}
	lines := []string{
		"export MULTIGENT_TOOLCHAIN_HOME=/opt/multigent/toolchains",
		"export MULTIGENT_RUNTIME_CLI_HOME=\"$MULTIGENT_TOOLCHAIN_HOME/mga\"",
		"export PATH=" + shellQuote(ManagedBinDir) + ":$PATH",
		"mkdir -p \"$MULTIGENT_RUNTIME_CLI_HOME/bin\" \"$MULTIGENT_RUNTIME_CLI_HOME/releases\" \"$MULTIGENT_TOOLCHAIN_HOME/markers\"",
	}
	if releaseVersion.MatchString(version) {
		lines = append(lines,
			"  arch=\"$(uname -m)\"",
			"  case \"$arch\" in",
			"    x86_64|amd64) arch=amd64 ;;",
			"    aarch64|arm64) arch=arm64 ;;",
			"    *) echo \"multigent: unsupported sandbox architecture for mga: $arch\" >&2; arch=\"\" ;;",
			"  esac",
			"  if [ -n \"$arch\" ]; then",
			"    marker_hash=\""+markerHash(version)+"-${arch}\"",
			"    marker=\"$MULTIGENT_TOOLCHAIN_HOME/markers/mga-${marker_hash}\"",
			"    if [ -f \"$marker\" ] && [ \"${MULTIGENT_RUNTIME_CLI_FORCE_INSTALL:-}\" != \"1\" ]; then",
			"      ln -sf \"$MULTIGENT_RUNTIME_CLI_HOME/releases/mga-"+version+"-${arch}\" \"$MULTIGENT_RUNTIME_CLI_HOME/bin/mga\"",
			"    else",
			"      tmp=\"$(mktemp -d)\"",
			"      trap 'rm -rf \"$tmp\"' EXIT",
			"      url=\"${MULTIGENT_RUNTIME_CLI_URL:-https://github.com/multigent/multigent/releases/download/"+version+"/multigent-"+version+"-linux-${arch}.tar.gz}\"",
			"      echo \"multigent: preparing mga "+version+" for linux-${arch}\" >&2",
			"      if { command -v curl >/dev/null 2>&1 && curl -fsSL \"$url\" -o \"$tmp/multigent.tgz\"; } || { command -v wget >/dev/null 2>&1 && wget -qO \"$tmp/multigent.tgz\" \"$url\"; }; then",
			"        if tar -xzf \"$tmp/multigent.tgz\" -C \"$tmp\"; then",
			"          found=\"$(find \"$tmp\" -type f -name mga | head -n 1)\"",
			"          if [ -n \"$found\" ]; then",
			"            install -m 0755 \"$found\" \"$MULTIGENT_RUNTIME_CLI_HOME/releases/mga-"+version+"-${arch}\"",
			"            ln -sf \"$MULTIGENT_RUNTIME_CLI_HOME/releases/mga-"+version+"-${arch}\" \"$MULTIGENT_RUNTIME_CLI_HOME/bin/mga\"",
			"            \"$MULTIGENT_RUNTIME_CLI_HOME/bin/mga\" version >&2 || true",
			"            touch \"$marker\"",
			"          fi",
			"        fi",
			"      else",
			"        echo \"multigent: warning: failed to download mga "+version+"; falling back to runtime image bundled mga\" >&2",
			"      fi",
			"    fi",
			"  fi",
		)
	}
	lines = append(lines,
		"command -v "+shellQuote(BinaryName)+" >/dev/null 2>&1 || { echo 'multigent: mga runtime CLI is missing in sandbox' >&2; exit 127; }",
	)
	return strings.Join(lines, "\n")
}

func markerHash(version string) string {
	sum := sha256.Sum256([]byte(version))
	return hex.EncodeToString(sum[:8])
}

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
	return binaryMountForPath(binPath)
}

// ResolveAvailableBinaryMount returns a read-only Docker mount for an available
// Linux mga binary. It first honors MULTIGENT_AGENT_CLI, then checks common
// development build locations so local runtime-base images do not need to be
// rebuilt just because mga changed or was missing from an older image.
func ResolveAvailableBinaryMount(searchRoots ...string) string {
	if mount := ResolveHostBinaryMount(); mount != "" {
		return mount
	}
	if runtime.GOOS != "linux" {
		return ""
	}
	return resolveAvailableLinuxBinaryMount(searchRoots...)
}

func resolveAvailableLinuxBinaryMount(searchRoots ...string) string {
	candidates := []string{}
	if exe, err := os.Executable(); err == nil && exe != "" {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), BinaryName))
	}
	if cwd, err := os.Getwd(); err == nil && cwd != "" {
		candidates = append(candidates, filepath.Join(cwd, "dist", BinaryName))
	}
	for _, root := range searchRoots {
		if root == "" {
			continue
		}
		candidates = append(candidates,
			filepath.Join(root, "dist", BinaryName),
			filepath.Join(filepath.Dir(root), "dist", BinaryName),
		)
	}
	for _, candidate := range candidates {
		if mount := binaryMountForPath(candidate); mount != "" {
			return mount
		}
	}
	return ""
}

func binaryMountForPath(binPath string) string {
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

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
