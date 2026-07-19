package playbook

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// gstackAssetFS vendors selected gstack skills so playbook templates can install
// the actual source material instead of short paraphrases. See
// gstack_assets/LICENSE for the upstream MIT license.
//
//go:embed gstack_assets
var gstackAssetFS embed.FS

func gstackSkillBody(name string) string {
	body := GstackSkillBody(name)
	if body == "" {
		return fmt.Sprintf("# Skill: %s\n\nBundled gstack source is missing.", name)
	}
	return body
}

func GstackSkillBody(name string) string {
	body, err := gstackAssetFS.ReadFile(fmt.Sprintf("gstack_assets/%s/SKILL.md", name))
	if err != nil {
		return ""
	}
	return string(body)
}

func CopyGstackSkillAssets(name, dstDir string) error {
	srcRoot := fmt.Sprintf("gstack_assets/%s", name)
	if _, err := fs.Stat(gstackAssetFS, srcRoot); err != nil {
		return err
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	return fs.WalkDir(gstackAssetFS, srcRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(path, srcRoot)
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			return nil
		}
		dst := filepath.Join(dstDir, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, err := gstackAssetFS.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	})
}
