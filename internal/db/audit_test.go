package db

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestListAuditEventFacets(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "multigent.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer store.Close()

	events := []AuditEvent{
		{ID: "aud-1", WorkspaceID: "ws-1", ActorType: "user", ActorID: "admin", Action: "connection.use", ResourceType: "connection", ResourceID: "conn-a", CreatedAt: "2026-08-22T00:00:00Z"},
		{ID: "aud-2", WorkspaceID: "ws-1", ActorType: "user", ActorID: "owner-a", Action: "agent.update", ResourceType: "agent", ResourceID: "aw-a", CreatedAt: "2026-08-22T00:01:00Z"},
		{ID: "aud-3", WorkspaceID: "ws-1", ActorType: "user", ActorID: "admin", Action: "connection.use", ResourceType: "connection", ResourceID: "conn-b", CreatedAt: "2026-08-22T00:02:00Z"},
		{ID: "aud-4", WorkspaceID: "ws-2", ActorType: "user", ActorID: "other", Action: "user.create", ResourceType: "user", ResourceID: "u-a", CreatedAt: "2026-08-22T00:03:00Z"},
	}
	for _, event := range events {
		if err := store.CreateAuditEvent(event); err != nil {
			t.Fatalf("create audit event %s: %v", event.ID, err)
		}
	}

	facets, err := store.ListAuditEventFacets(AuditEventFilter{WorkspaceID: "ws-1"})
	if err != nil {
		t.Fatalf("list facets: %v", err)
	}
	if want := []string{"admin", "owner-a"}; !reflect.DeepEqual(facets.ActorIDs, want) {
		t.Fatalf("actor facets = %#v, want %#v", facets.ActorIDs, want)
	}
	if want := []string{"agent.update", "connection.use"}; !reflect.DeepEqual(facets.Actions, want) {
		t.Fatalf("action facets = %#v, want %#v", facets.Actions, want)
	}
	if want := []string{"agent", "connection"}; !reflect.DeepEqual(facets.ResourceTypes, want) {
		t.Fatalf("resource type facets = %#v, want %#v", facets.ResourceTypes, want)
	}
	if want := []string{"aw-a", "conn-a", "conn-b"}; !reflect.DeepEqual(facets.ResourceIDs, want) {
		t.Fatalf("resource id facets = %#v, want %#v", facets.ResourceIDs, want)
	}
}
