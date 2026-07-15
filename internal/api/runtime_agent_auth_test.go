package api

import (
	"testing"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
)

func TestAgentRuntimeTokenValidateAndExpire(t *testing.T) {
	users := newTestUserStore(t)
	s := &Server{controlDB: users.db, users: users}

	token := s.issueAgentRuntimeToken(runtimeAgentTokenPayload{
		WorkspaceID:  "ws-one",
		Project:      "tapnow",
		Agent:        "pm",
		RunID:        "run-one",
		Capabilities: []string{"connection.use"},
	}, time.Minute)
	principal, ok := s.validateAgentRuntimeToken(token)
	if !ok {
		t.Fatalf("runtime token did not validate")
	}
	if principal.WorkspaceID != "ws-one" || principal.Project != "tapnow" || principal.Agent != "pm" || principal.RunID != "run-one" {
		t.Fatalf("principal mismatch: %#v", principal)
	}
	if !runtimeHasCapability(principal, "connection.use") {
		t.Fatalf("capability missing: %#v", principal.Capabilities)
	}

	expired := s.issueAgentRuntimeToken(runtimeAgentTokenPayload{
		WorkspaceID:  "ws-one",
		Project:      "tapnow",
		Agent:        "pm",
		Capabilities: []string{"connection.use"},
	}, -time.Second)
	if _, ok := s.validateAgentRuntimeToken(expired); ok {
		t.Fatalf("expired token validated")
	}
}

func TestFindRuntimeConnectionRequiresMatchingGrant(t *testing.T) {
	users := newTestUserStore(t)
	s := &Server{controlDB: users.db, users: users}
	workspaceID := "ws-one"

	if err := users.db.UpsertWorkspace(controldb.Workspace{ID: workspaceID, Name: "One", Slug: "one"}); err != nil {
		t.Fatalf("workspace: %v", err)
	}
	granted := controldb.Connection{
		ID:             "conn-granted",
		WorkspaceID:    workspaceID,
		Provider:       "github",
		ConnectionName: "default",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       ConnectionAuthAPIKey,
		Status:         "active",
		ProfileJSON:    "{}",
		CreatedBy:      "admin",
	}
	userOnly := granted
	userOnly.ID = "conn-user-only"
	userOnly.ConnectionName = "personal"
	if err := users.db.UpsertConnection(granted); err != nil {
		t.Fatalf("granted connection: %v", err)
	}
	if err := users.db.UpsertConnection(userOnly); err != nil {
		t.Fatalf("user connection: %v", err)
	}
	if err := users.db.CreateConnectionGrant(controldb.ConnectionGrant{
		ID:           "grant-agent",
		WorkspaceID:  workspaceID,
		ConnectionID: granted.ID,
		TargetType:   ConnectionTargetAgent,
		TargetID:     "tapnow/pm",
	}); err != nil {
		t.Fatalf("agent grant: %v", err)
	}
	if err := users.db.CreateConnectionGrant(controldb.ConnectionGrant{
		ID:           "grant-user",
		WorkspaceID:  workspaceID,
		ConnectionID: userOnly.ID,
		TargetType:   ConnectionTargetUser,
		TargetID:     "pm-owner",
	}); err != nil {
		t.Fatalf("user grant: %v", err)
	}

	principal := runtimeAgentPrincipal{
		WorkspaceID:  workspaceID,
		Project:      "tapnow",
		Agent:        "pm",
		Capabilities: []string{"connection.use"},
	}
	conn, ok, err := s.findRuntimeConnection(principal, granted.ID, "")
	if err != nil {
		t.Fatalf("find granted connection: %v", err)
	}
	if !ok || conn.ID != granted.ID {
		t.Fatalf("granted connection not found: ok=%v conn=%#v", ok, conn)
	}
	if _, ok, err := s.findRuntimeConnection(principal, userOnly.ID, ""); err != nil || ok {
		t.Fatalf("user-only connection should not be available: ok=%v err=%v", ok, err)
	}
	if conn, ok, err := s.findRuntimeConnection(principal, "", "github"); err != nil || !ok || conn.ID != granted.ID {
		t.Fatalf("alias lookup failed: ok=%v conn=%#v err=%v", ok, conn, err)
	}
}
