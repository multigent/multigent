package builtins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureSkillsWritesManagedBuiltins(t *testing.T) {
	root := t.TempDir()
	if err := EnsureSkills(root); err != nil {
		t.Fatalf("EnsureSkills: %v", err)
	}
	for _, name := range []string{"multigent-usage", "task-management", "agency-messaging"} {
		path := filepath.Join(root, "skills", name, "SKILL.md")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if !strings.Contains(string(data), "name: "+name) {
			t.Fatalf("skill %s missing frontmatter name", name)
		}
	}

	custom := filepath.Join(root, "skills", "multigent-usage", "SKILL.md")
	if err := os.WriteFile(custom, []byte("custom"), 0o644); err != nil {
		t.Fatalf("write custom skill: %v", err)
	}
	if err := EnsureSkills(root); err != nil {
		t.Fatalf("EnsureSkills second run: %v", err)
	}
	data, err := os.ReadFile(custom)
	if err != nil {
		t.Fatalf("read custom skill: %v", err)
	}
	if string(data) == "custom" {
		t.Fatalf("builtin skill was not updated")
	}
}
