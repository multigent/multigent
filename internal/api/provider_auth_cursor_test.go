package api

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCursorBrowserAuthUsesXDGConfigCredentialPath(t *testing.T) {
	spec, ok := browserAuthSpec("cursor")
	if !ok {
		t.Fatal("cursor browser auth spec not found")
	}
	home := t.TempDir()
	src := filepath.Join(home, ".config", "cursor", "cli-config.json")
	if err := os.MkdirAll(filepath.Dir(src), 0o700); err != nil {
		t.Fatalf("mkdir cursor config: %v", err)
	}
	if err := os.WriteFile(src, []byte(`{"token":"test"}`), 0o600); err != nil {
		t.Fatalf("write cursor config: %v", err)
	}
	auth := filepath.Join(home, ".config", "cursor", "auth.json")
	if err := os.WriteFile(auth, []byte(`{"user":"test"}`), 0o600); err != nil {
		t.Fatalf("write cursor auth: %v", err)
	}

	if !spec.CredentialsReady(home) {
		t.Fatal("cursor credentials were not detected")
	}
	dst := t.TempDir()
	if err := spec.CopyCredentials(home, dst); err != nil {
		t.Fatalf("copy credentials: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, ".config", "cursor", "cli-config.json")); err != nil {
		t.Fatalf("copied cli config missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, ".config", "cursor", "auth.json")); err != nil {
		t.Fatalf("copied auth config missing: %v", err)
	}
}
