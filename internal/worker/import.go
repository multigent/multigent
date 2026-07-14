package worker

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var importContextFiles = map[string]string{
	"AGENTS.md":    "runtime-context",
	"CLAUDE.md":    "runtime-context",
	"GEMINI.md":    "runtime-context",
	"OPENCODE.md":  "runtime-context",
	"IFLOW.md":     "runtime-context",
	"context.md":   "runtime-context",
	".cursorrules": "runtime-context",
}

var importContextDirs = map[string]string{
	".multigent/context":   "multigent-context",
	legacyContextDirName(): "legacy-context",
	".claude/skills":       "runtime-skills",
	".gemini/skills":       "runtime-skills",
	".cursor/rules":        "runtime-rules",
}

func ScanImportManifest(root string) (*ImportManifest, error) {
	if root == "" {
		return nil, fmt.Errorf("path is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path must be a directory: %s", abs)
	}

	manifest := &ImportManifest{
		Path:      abs,
		Files:     []ImportContextFile{},
		ScannedAt: time.Now(),
	}
	err = filepath.WalkDir(abs, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "dist", "build", ".next", ".turbo":
				if path != abs {
					return filepath.SkipDir
				}
			}
			return nil
		}

		rel, err := filepath.Rel(abs, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		kind, ok := importContextFiles[filepath.Base(rel)]
		if !ok {
			for dir, dirKind := range importContextDirs {
				if strings.HasPrefix(rel, dir+"/") {
					kind = dirKind
					ok = true
					break
				}
			}
		}
		if !ok {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		manifest.Files = append(manifest.Files, ImportContextFile{
			Path: rel,
			Kind: kind,
			Size: info.Size(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(manifest.Files, func(i, j int) bool {
		return manifest.Files[i].Path < manifest.Files[j].Path
	})
	if len(manifest.Files) == 0 {
		manifest.Warnings = append(manifest.Warnings, "no known agent context files found")
	}
	if !hasAnyRuntimeContext(manifest.Files) {
		manifest.Warnings = append(manifest.Warnings, "no root runtime context file found, expected one of AGENTS.md, CLAUDE.md, GEMINI.md, OPENCODE.md, IFLOW.md, context.md, or .cursorrules")
	}
	return manifest, nil
}

func hasAnyRuntimeContext(files []ImportContextFile) bool {
	for _, file := range files {
		if file.Kind == "runtime-context" {
			return true
		}
	}
	return false
}

func legacyContextDirName() string {
	return "." + "agency" + "cli/context"
}
