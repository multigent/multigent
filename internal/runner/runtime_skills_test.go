package runner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/store"
)

func writeTestSkill(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, "skills", name)
	if err := os.MkdirAll(filepath.Join(dir, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	skill := "---\nname: " + name + "\ndescription: test skill\n---\n\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skill), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "references", "rules.md"), []byte("rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeAddDirsMaterializesRoleSkillsForClaudeCode(t *testing.T) {
	root := t.TempDir()
	st := store.NewFS(root)
	writeTestSkill(t, root, "case-authoring", "write real cases")
	if err := st.SaveTeam("quality", &entity.Team{Name: "quality"}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveRole("quality", "analyst", &entity.Role{Name: "analyst", Skills: []string{"case-authoring"}}); err != nil {
		t.Fatal(err)
	}
	r := New(root, nil, st)
	agentDir := filepath.Join(root, "projects", "p", "agents", "analyst")
	meta := &entity.AgentMeta{Name: "analyst", Project: "p", Team: "quality", Role: "analyst",
		Model: entity.ModelClaudeCode, AddDirs: []string{"/srv/repo"}}

	dirs := r.runtimeAddDirs("p", "analyst", agentDir, meta)

	bundle := filepath.Join(agentDir, ".multigent", runtimeSkillsBundleDir)
	if len(dirs) != 2 || dirs[0] != "/srv/repo" || dirs[1] != bundle {
		t.Fatalf("add dirs = %v, want configured dir then %s", dirs, bundle)
	}
	for _, rel := range []string{"case-authoring/SKILL.md", "case-authoring/references/rules.md"} {
		if _, err := os.Stat(filepath.Join(bundle, ".claude", "skills", rel)); err != nil {
			t.Fatalf("role skill file %s not materialized: %v", rel, err)
		}
	}
	if len(meta.AddDirs) != 1 {
		t.Fatalf("meta.AddDirs mutated: %v", meta.AddDirs)
	}
	args := InvokerFor(meta.Model, meta.RunCommand, dirs).Args("prompt.md", "")
	found := false
	for i := range args {
		if args[i] == "--add-dir" && i+1 < len(args) && args[i+1] == bundle {
			found = true
		}
	}
	if !found {
		t.Fatalf("claude args %v do not include the skills bundle", args)
	}
}

func TestRuntimeAddDirsLeavesSandboxedAndOtherModelsAlone(t *testing.T) {
	root := t.TempDir()
	r := New(root, nil, store.NewFS(root))
	agentDir := filepath.Join(root, "agent")
	docker := &entity.AgentMeta{Model: entity.ModelClaudeCode, Sandbox: &entity.SandboxConfig{Provider: entity.SandboxDocker}}
	if dirs := r.runtimeAddDirs("p", "a", agentDir, docker); len(dirs) != 0 {
		t.Fatalf("sandboxed run got %v", dirs)
	}
	codex := &entity.AgentMeta{Model: entity.ModelCodex, AddDirs: []string{"/x"}}
	if dirs := r.runtimeAddDirs("p", "a", agentDir, codex); len(dirs) != 1 || dirs[0] != "/x" {
		t.Fatalf("codex run got %v", dirs)
	}
}
