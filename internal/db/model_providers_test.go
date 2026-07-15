package db

import (
	"path/filepath"
	"testing"
)

func TestModelProvidersAreWorkspaceScoped(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "multigent.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	for _, ws := range []Workspace{
		{ID: "ws-one", Name: "One", Slug: "one", Root: "/tmp/one", CreatedAt: nowUTC()},
		{ID: "ws-two", Name: "Two", Slug: "two", Root: "/tmp/two", CreatedAt: nowUTC()},
	} {
		if err := db.UpsertWorkspace(ws); err != nil {
			t.Fatalf("upsert workspace: %v", err)
		}
	}
	if err := db.UpsertModelProvider("ws-one", ModelProvider{
		ID:        "prov-main",
		Name:      "Main",
		Type:      "openai",
		APIKey:    "sk-one",
		EnvJSON:   `{"OPENAI_BASE_URL":"https://api.openai.com"}`,
		CreatedAt: nowUTC(),
		UpdatedAt: nowUTC(),
	}); err != nil {
		t.Fatalf("upsert provider one: %v", err)
	}
	if err := db.UpsertModelProvider("ws-two", ModelProvider{
		ID:        "prov-main",
		Name:      "Main",
		Type:      "anthropic",
		APIKey:    "sk-two",
		CreatedAt: nowUTC(),
		UpdatedAt: nowUTC(),
	}); err != nil {
		t.Fatalf("upsert provider two: %v", err)
	}

	one, ok, err := db.ModelProviderByID("ws-one", "prov-main")
	if err != nil || !ok {
		t.Fatalf("provider one ok=%v err=%v", ok, err)
	}
	two, ok, err := db.ModelProviderByID("ws-two", "prov-main")
	if err != nil || !ok {
		t.Fatalf("provider two ok=%v err=%v", ok, err)
	}
	if one.Type != "openai" || one.APIKey != "sk-one" || two.Type != "anthropic" || two.APIKey != "sk-two" {
		t.Fatalf("workspace isolation failed: one=%#v two=%#v", one, two)
	}
}
