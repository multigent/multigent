package playbook

import (
	"embed"
	"fmt"
)

// openSpecAssetFS vendors the upstream OpenSpec skills so the playbook installs
// the actual operating procedures instead of short summaries. See
// openspec_assets/LICENSE for the upstream MIT license.
//
//go:embed openspec_assets
var openSpecAssetFS embed.FS

func openSpecSkillBody(name string) string {
	body := OpenSpecSkillBody(name)
	if body == "" {
		return fmt.Sprintf("# Skill: %s\n\nBundled OpenSpec skill source is missing.", name)
	}
	return body
}

func OpenSpecSkillBody(name string) string {
	body, err := openSpecAssetFS.ReadFile(fmt.Sprintf("openspec_assets/%s/SKILL.md", name))
	if err != nil {
		return ""
	}
	return string(body)
}

func CopyOpenSpecSkillAssets(name, dstDir string) error {
	return copyOpenSpecEmbeddedDir(fmt.Sprintf("openspec_assets/%s", name), dstDir)
}

func copyOpenSpecEmbeddedDir(srcRoot, dstDir string) error {
	return copyEmbeddedDirFromFS(openSpecAssetFS, srcRoot, dstDir)
}
