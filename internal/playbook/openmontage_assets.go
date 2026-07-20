package playbook

import (
	"embed"
	"fmt"
)

// openMontageAssetFS vendors selected OpenMontage stage director and meta
// skills for the experimental Video Production Studio playbook. OpenMontage is
// AGPLv3; keep these assets clearly attributed and review licensing before
// commercial distribution.
//
//go:embed openmontage_assets
var openMontageAssetFS embed.FS

func openMontageSkillBody(name string) string {
	body := OpenMontageSkillBody(name)
	if body == "" {
		return fmt.Sprintf("# Skill: %s\n\nBundled OpenMontage skill source is missing.", name)
	}
	return body
}

func OpenMontageSkillBody(name string) string {
	body, err := openMontageAssetFS.ReadFile(fmt.Sprintf("openmontage_assets/%s/SKILL.md", name))
	if err != nil {
		return ""
	}
	return string(body)
}

func CopyOpenMontageSkillAssets(name, dstDir string) error {
	return copyOpenMontageEmbeddedDir(fmt.Sprintf("openmontage_assets/%s", name), dstDir)
}

func copyOpenMontageEmbeddedDir(srcRoot, dstDir string) error {
	return copyEmbeddedDirFromFS(openMontageAssetFS, srcRoot, dstDir)
}
