package runtimecli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveHostBinaryMountRequiresExplicitLinuxBinary(t *testing.T) {
	t.Setenv(HostBinaryEnv, "")
	if got := ResolveHostBinaryMount(); got != "" {
		t.Fatalf("unexpected implicit mount: %q", got)
	}

	dir := t.TempDir()
	machO := filepath.Join(dir, BinaryName)
	if err := os.WriteFile(machO, []byte{0xcf, 0xfa, 0xed, 0xfe}, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(HostBinaryEnv, machO)
	if got := ResolveHostBinaryMount(); got != "" {
		t.Fatalf("non-Linux binary must not be mounted: %q", got)
	}

	elf := filepath.Join(dir, "linux", BinaryName)
	if err := os.MkdirAll(filepath.Dir(elf), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(elf, []byte{0x7f, 'E', 'L', 'F', 2, 1, 1}, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(HostBinaryEnv, elf)
	got := ResolveHostBinaryMount()
	if !strings.HasSuffix(got, ":"+BinaryPath+":ro") {
		t.Fatalf("ELF mount=%q", got)
	}
}
