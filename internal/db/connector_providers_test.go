package db

import (
	"path/filepath"
	"testing"
)

func TestOpenSeedsDefaultConnectorProviders(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "multigent.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	providers, err := db.ListConnectorProviders(false)
	if err != nil {
		t.Fatalf("list providers: %v", err)
	}
	if len(providers) < 4 {
		t.Fatalf("expected default providers, got %#v", providers)
	}
	github, ok, err := db.ConnectorProviderByID("github")
	if err != nil || !ok {
		t.Fatalf("github provider ok=%v err=%v", ok, err)
	}
	if !github.Enabled || github.AuthTypesJSON == "" || github.CatalogJSON == "" {
		t.Fatalf("github provider not seeded correctly: %#v", github)
	}
	for _, id := range []string{"feishu", "lark", "dingtalk_bot"} {
		provider, ok, err := db.ConnectorProviderByID(id)
		if err != nil || !ok {
			t.Fatalf("%s provider ok=%v err=%v", id, ok, err)
		}
		if !provider.Enabled || provider.AuthTypesJSON == "" || provider.CatalogJSON == "" {
			t.Fatalf("%s provider not seeded correctly: %#v", id, provider)
		}
	}
}

func TestSeedDefaultConnectorProvidersPreservesDisabledState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "multigent.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.UpsertConnectorProvider(ConnectorProvider{
		Provider:      "github",
		DisplayName:   "GitHub",
		AuthTypesJSON: `["api_key"]`,
		CatalogJSON:   `{}`,
		Enabled:       false,
	}); err != nil {
		t.Fatalf("disable github: %v", err)
	}
	db.Close()

	db, err = Open(path)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer db.Close()
	github, ok, err := db.ConnectorProviderByID("github")
	if err != nil || !ok {
		t.Fatalf("github provider ok=%v err=%v", ok, err)
	}
	if github.Enabled {
		t.Fatalf("seed should preserve disabled state: %#v", github)
	}
	enabled, err := db.ListConnectorProviders(false)
	if err != nil {
		t.Fatalf("list enabled providers: %v", err)
	}
	for _, provider := range enabled {
		if provider.Provider == "github" {
			t.Fatalf("disabled github should not be listed as enabled")
		}
	}
}
