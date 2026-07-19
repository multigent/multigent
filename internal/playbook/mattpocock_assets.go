package playbook

import (
	"embed"
	"fmt"
)

// mattPocockAssetFS vendors selected stable Matt Pocock skills. See
// mattpocock_assets/LICENSE for the upstream MIT license.
//
//go:embed mattpocock_assets
var mattPocockAssetFS embed.FS

func mattPocockSkillBody(category, name string) string {
	body := MattPocockSkillBody(category, name)
	if body == "" {
		return fmt.Sprintf("# Skill: %s\n\nBundled Matt Pocock skill source is missing.", name)
	}
	return body
}

func MattPocockSkillBody(category, name string) string {
	body, err := mattPocockAssetFS.ReadFile(fmt.Sprintf("mattpocock_assets/%s/%s/SKILL.md", category, name))
	if err != nil {
		return ""
	}
	return string(body)
}

func CopyMattPocockSkillAssets(category, name, dstDir string) error {
	return copyMattPocockEmbeddedDir(fmt.Sprintf("mattpocock_assets/%s/%s", category, name), dstDir)
}

func copyMattPocockEmbeddedDir(srcRoot, dstDir string) error {
	return copyEmbeddedDirFromFS(mattPocockAssetFS, srcRoot, dstDir)
}
