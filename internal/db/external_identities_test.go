package db

import (
	"path/filepath"
	"testing"
)

func TestExternalIdentitiesAreWorkspaceAndProviderScoped(t *testing.T) {
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
			t.Fatalf("workspace: %v", err)
		}
	}
	for _, u := range []User{
		{Username: "ella", CreatedAt: nowUTC()},
		{Username: "glenn", CreatedAt: nowUTC()},
	} {
		if err := db.UpsertUser(u); err != nil {
			t.Fatalf("user: %v", err)
		}
	}

	if err := db.UpsertExternalIdentity(ExternalIdentity{
		ID:             "ext-one",
		WorkspaceID:    "ws-one",
		Provider:       "feishu",
		ExternalUserID: "ou_same",
		UserID:         "ella",
	}); err != nil {
		t.Fatalf("upsert one: %v", err)
	}
	if err := db.UpsertExternalIdentity(ExternalIdentity{
		ID:             "ext-two",
		WorkspaceID:    "ws-two",
		Provider:       "feishu",
		ExternalUserID: "ou_same",
		UserID:         "glenn",
	}); err != nil {
		t.Fatalf("upsert two: %v", err)
	}
	if err := db.UpsertExternalIdentity(ExternalIdentity{
		ID:             "ext-three",
		WorkspaceID:    "ws-one",
		Provider:       "lark",
		ExternalUserID: "ou_same",
		UserID:         "glenn",
	}); err != nil {
		t.Fatalf("upsert three: %v", err)
	}

	one, ok, err := db.ExternalIdentityByExternalID("ws-one", "feishu", "ou_same")
	if err != nil || !ok {
		t.Fatalf("lookup one ok=%v err=%v", ok, err)
	}
	two, ok, err := db.ExternalIdentityByExternalID("ws-two", "feishu", "ou_same")
	if err != nil || !ok {
		t.Fatalf("lookup two ok=%v err=%v", ok, err)
	}
	three, ok, err := db.ExternalIdentityByExternalID("ws-one", "lark", "ou_same")
	if err != nil || !ok {
		t.Fatalf("lookup three ok=%v err=%v", ok, err)
	}
	if one.UserID != "ella" || two.UserID != "glenn" || three.UserID != "glenn" {
		t.Fatalf("scope mismatch: one=%#v two=%#v three=%#v", one, two, three)
	}
}
