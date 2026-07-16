package api

import (
	"os"
	"path/filepath"
	"testing"

	controldb "github.com/multigent/multigent/internal/db"
)

func TestNormalizeServerWorkspaceRootSelectsWorkspaceFromDataDir(t *testing.T) {
	dataRoot := t.TempDir()
	db, err := controldb.Open(filepath.Join(dataRoot, ".multigent", "multigent.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	firstRoot := filepath.Join(dataRoot, "ws-first")
	secondRoot := filepath.Join(dataRoot, "ws-second")
	for _, root := range []string{firstRoot, secondRoot} {
		if err := os.MkdirAll(filepath.Join(root, ".multigent"), 0o755); err != nil {
			t.Fatalf("mkdir agency: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, ".multigent", "agency.yaml"), []byte("name: workspace\n"), 0o644); err != nil {
			t.Fatalf("write agency: %v", err)
		}
	}
	if err := db.UpsertWorkspace(controldb.Workspace{
		ID:        "ws-first",
		Name:      "First",
		Slug:      "first",
		Root:      firstRoot,
		CreatedAt: "2026-07-16T00:00:00Z",
	}); err != nil {
		t.Fatalf("first workspace: %v", err)
	}
	if err := db.UpsertWorkspace(controldb.Workspace{
		ID:        "ws-second",
		Name:      "Second",
		Slug:      "second",
		Root:      secondRoot,
		CreatedAt: "2026-07-16T01:00:00Z",
	}); err != nil {
		t.Fatalf("second workspace: %v", err)
	}

	got := normalizeServerWorkspaceRoot(dataRoot, db)
	if got != secondRoot {
		t.Fatalf("root=%q, want %q", got, secondRoot)
	}
}
