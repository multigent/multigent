package store

import (
	"os"
	"path/filepath"
)

func workspaceConfigDir(root string) string {
	if _, err := os.Stat(filepath.Join(root, ".multigent", "agency.yaml")); err == nil {
		return ".multigent"
	}
	if _, err := os.Stat(filepath.Join(root, ".agencycli", "agency.yaml")); err == nil {
		return ".agencycli"
	}
	return ".multigent"
}

func workspaceConfigPath(root string, parts ...string) string {
	return filepath.Join(append([]string{root, workspaceConfigDir(root)}, parts...)...)
}
