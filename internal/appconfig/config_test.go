package appconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigTOML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	body := `
[workspace]
dir = "/tmp/multigent"

[server]
addr = "0.0.0.0:8080"

[auth]
api_key = "secret"

[smtp]
host = "smtp.example.com"
port = 465
from = "noreply@example.com"
tls = "implicit"

[logging]
file = "/tmp/multigent.log"
level = "debug"
format = "json"
max_size_mb = 20
stderr = false

[sandbox.e2b]
api_url = "http://127.0.0.1:49999"
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Workspace.Dir != "/tmp/multigent" || cfg.Server.Addr != "0.0.0.0:8080" || cfg.Auth.APIKey != "secret" {
		t.Fatalf("basic config not loaded: %#v", cfg)
	}
	if cfg.SMTP.Port != 465 || cfg.SMTP.TLS != "implicit" {
		t.Fatalf("smtp config not loaded: %#v", cfg.SMTP)
	}
	if cfg.Logging.Stderr == nil || *cfg.Logging.Stderr {
		t.Fatalf("stderr bool not loaded: %#v", cfg.Logging)
	}
	if cfg.Sandbox.E2B.APIURL == "" {
		t.Fatalf("e2b api url missing")
	}
}
