package builtins

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed files/skills/*/SKILL.md
var skillFiles embed.FS

const skillRoot = "files/skills"

// EnsureSkills writes bundled Multigent skills into a workspace. Builtin skills
// are managed system assets and should track the running Multigent version;
// user-installed/custom skills live in the registry and should not shadow these
// core runtime instructions.
func EnsureSkills(workspaceRoot string) error {
	entries, err := skillFiles.ReadDir(skillRoot)
	if err != nil {
		return fmt.Errorf("builtins: read embedded skills: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if err := ensureSkill(workspaceRoot, entry.Name()); err != nil {
			return err
		}
	}
	return nil
}

func ensureSkill(workspaceRoot, name string) error {
	destDir := filepath.Join(workspaceRoot, "skills", name)
	dest := filepath.Join(destDir, "SKILL.md")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("builtins: create skill dir %q: %w", name, err)
	}
	src := strings.Join([]string{skillRoot, name, "SKILL.md"}, "/")
	data, err := fs.ReadFile(skillFiles, src)
	if err != nil {
		return fmt.Errorf("builtins: read skill %q: %w", name, err)
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return fmt.Errorf("builtins: write skill %q: %w", name, err)
	}
	return nil
}
