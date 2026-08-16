package ctxbuild

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSkillFilesFollowsSymlinkRoot(t *testing.T) {
	tmp := t.TempDir()
	realDir := filepath.Join(tmp, "registry", "repo-image-hosting", "v1")
	if err := os.MkdirAll(filepath.Join(realDir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "SKILL.md"), []byte("# Skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "scripts", "upload.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	linkDir := filepath.Join(tmp, "skills", "repo-image-hosting")
	if err := os.MkdirAll(filepath.Dir(linkDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	files := loadSkillFiles(linkDir)
	if len(files) != 1 {
		t.Fatalf("expected 1 bundled file, got %d: %#v", len(files), files)
	}
	if files[0].Name != filepath.Join("scripts", "upload.sh") {
		t.Fatalf("unexpected bundled file name: %q", files[0].Name)
	}
}
