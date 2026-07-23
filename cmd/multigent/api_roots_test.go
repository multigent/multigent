package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAPIServeRootsWithWorkspaceRootUsesParentDataRoot(t *testing.T) {
	oldGlobalDir := globalDir
	oldLoadedConfig := loadedConfig
	globalDir = ""
	loadedConfig = nil
	defer func() {
		globalDir = oldGlobalDir
		loadedConfig = oldLoadedConfig
	}()

	dataRoot := t.TempDir()
	workspaceRoot := filepath.Join(dataRoot, "6bbcd4cb-f08b-4268-8f93-926e5939eb59")
	if err := os.MkdirAll(filepath.Join(workspaceRoot, ".multigent"), 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspaceRoot, ".multigent", "agency.yaml"), []byte("name: Spaceship\n"), 0o644); err != nil {
		t.Fatalf("write agency: %v", err)
	}
	globalDir = workspaceRoot

	gotDataRoot, gotActiveRoot, err := resolveAPIServeRoots(nil)
	if err != nil {
		t.Fatalf("resolve roots: %v", err)
	}
	if gotDataRoot != dataRoot {
		t.Fatalf("data root=%q, want %q", gotDataRoot, dataRoot)
	}
	if gotActiveRoot != workspaceRoot {
		t.Fatalf("active root=%q, want %q", gotActiveRoot, workspaceRoot)
	}
	if env := os.Getenv("MULTIGENT_DATA_DIR"); env != dataRoot {
		t.Fatalf("MULTIGENT_DATA_DIR=%q, want %q", env, dataRoot)
	}
}
