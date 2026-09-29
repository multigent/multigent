package runner

import (
	"path/filepath"
	"testing"

	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/runenv"
)

func TestRuntimeAddDirsMountsDockerPaths(t *testing.T) {
	source := t.TempDir()
	resolved, mounts := runtimeAddDirs([]string{source}, &entity.SandboxConfig{Provider: entity.SandboxDocker})
	if len(resolved) != 1 || resolved[0] != "/mnt/multigent/adddirs/0" {
		t.Fatalf("resolved add dirs = %#v", resolved)
	}
	if len(mounts) != 1 {
		t.Fatalf("mounts = %#v", mounts)
	}
	if mounts[0].Source != filepath.Clean(source) || mounts[0].Target != resolved[0] || mounts[0].Mode != runenv.MountModeReadWrite {
		t.Fatalf("mount = %#v", mounts[0])
	}
}

func TestRuntimeAddDirsKeepsHostPathsWithoutDocker(t *testing.T) {
	addDirs := []string{"/repo/one", "/repo/two"}
	resolved, mounts := runtimeAddDirs(addDirs, nil)
	if len(mounts) != 0 {
		t.Fatalf("unexpected mounts: %#v", mounts)
	}
	if len(resolved) != 2 || resolved[0] != addDirs[0] || resolved[1] != addDirs[1] {
		t.Fatalf("resolved add dirs = %#v", resolved)
	}
}
