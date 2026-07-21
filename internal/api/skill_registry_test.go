package api

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/store"
)

func TestSkillRegistryInstallsReferenceAcrossWorkspacesAndForksOnEdit(t *testing.T) {
	dataRoot := t.TempDir()
	workspaceA := filepath.Join(dataRoot, "workspace-a")
	workspaceB := filepath.Join(dataRoot, "workspace-b")
	sa := &Server{root: workspaceA, st: store.NewFS(workspaceA)}
	sb := &Server{root: workspaceB, st: store.NewFS(workspaceB)}

	skillDir := sa.st.SkillDir("shared-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: shared-skill\ndescription: Shared package\n---\n\nUse this skill.\n"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	meta := entity.Skill{
		Name:        "shared-skill",
		Description: "Shared package",
		Source:      "test",
		SourceType:  "manual",
		Version:     "v1",
		Managed:     true,
	}
	if err := writeSkillRegistryMeta(skillDir, meta); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	if err := sa.snapshotSkillPackage(skillDir, meta); err != nil {
		t.Fatalf("snapshot package: %v", err)
	}

	if err := sb.installSkillPackageReference("shared-skill", "v1", ""); err != nil {
		t.Fatalf("install reference: %v", err)
	}
	dst := sb.st.SkillDir("shared-skill")
	info, err := os.Lstat(dst)
	if err != nil {
		t.Fatalf("lstat installed skill: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected workspace install to be a package reference")
	}
	skills, err := sb.st.ListSkills()
	if err != nil {
		t.Fatalf("list skills: %v", err)
	}
	if len(skills) != 1 || skills[0].Name != "shared-skill" {
		t.Fatalf("expected symlinked skill to be listed, got %#v", skills)
	}

	if err := sb.ensureSkillWritableCopy("shared-skill"); err != nil {
		t.Fatalf("fork skill: %v", err)
	}
	info, err = os.Lstat(dst)
	if err != nil {
		t.Fatalf("lstat forked skill: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("expected edited skill to be forked into a workspace-local copy")
	}
	forkMeta := sb.skillRegistryMeta("shared-skill")
	if forkMeta.Managed || !forkMeta.Dirty || forkMeta.Source != "registry:shared-skill@v1" || forkMeta.SourceType != "workspace" {
		t.Fatalf("unexpected fork metadata: %#v", forkMeta)
	}
}
