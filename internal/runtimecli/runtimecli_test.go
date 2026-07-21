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

func TestResolveAvailableBinaryMountUsesWorkspaceDist(t *testing.T) {
	t.Setenv(HostBinaryEnv, "")
	root := t.TempDir()
	bin := filepath.Join(root, "dist", BinaryName)
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte{0x7f, 'L', 'F'}, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := ResolveAvailableBinaryMount(root); got != "" {
		t.Fatalf("invalid ELF must not be mounted: %q", got)
	}
	if err := os.WriteFile(bin, []byte{0x7f, 'E', 'L', 'F', 2, 1, 1}, 0o755); err != nil {
		t.Fatal(err)
	}
	got := ResolveAvailableBinaryMount(root)
	if !strings.HasPrefix(got, bin+":") || !strings.HasSuffix(got, ":"+BinaryPath+":ro") {
		t.Fatalf("workspace dist mount=%q", got)
	}
}
