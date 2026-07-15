package api

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"testing"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/rbac"
)

func newTestUserStore(t *testing.T) *UserStore {
	t.Helper()
	db, err := controldb.Open(filepath.Join(t.TempDir(), "multigent.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return newUserStore(db)
}

func TestUserStorePrincipalMapsProjectAndAgentRoles(t *testing.T) {
	users := newTestUserStore(t)
	role := RoleMember
	projects := []projectAccess{{Project: "tapnow", Role: ProjectRoleViewer}}
	linkedAgents := []string{"tapnow/connector-dev"}

	if err := users.CreateUser("dev", "pass", role, "", "", "", "", ""); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := users.UpdateUser("dev", nil, nil, nil, nil, nil, nil, nil, projects, linkedAgents, nil); err != nil {
		t.Fatalf("update user: %v", err)
	}

	p, ok := users.Principal("dev")
	if !ok {
		t.Fatalf("principal not found")
	}
	if p.OrgRole != rbac.OrgRoleMember {
		t.Fatalf("org role=%q", p.OrgRole)
	}
	if got := p.ProjectRoles[rbac.ProjectKey("tapnow")]; got != rbac.ProjectRoleViewer {
		t.Fatalf("project role=%q", got)
	}
	if got := p.AgentRoles[rbac.AgentKey("tapnow", "connector-dev")]; got != rbac.AgentRoleOperator {
		t.Fatalf("agent role=%q", got)
	}
}

func TestUserStoreRegisterByEmailAllowsEmailLogin(t *testing.T) {
	users := newTestUserStore(t)

	u, err := users.RegisterByEmail("Dev@Example.com", "secret1", "Dev User")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if u.Username != "dev" {
		t.Fatalf("username=%q", u.Username)
	}
	if u.Email != "dev@example.com" {
		t.Fatalf("email=%q", u.Email)
	}
	if got := users.Authenticate("dev@example.com", "secret1"); got == nil || got.Username != u.Username {
		t.Fatalf("email login failed")
	}
}

func TestUserStoreAcceptInvitationCreatesMemberWithGrants(t *testing.T) {
	users := newTestUserStore(t)
	projects := []projectAccess{{Project: "tapnow", Role: ProjectRoleOperator}}

	inv, err := users.CreateInvitation("Ella@Example.com", RoleMember, "Ella", "admin", projects, []string{"tapnow/frontend"})
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	u, err := users.AcceptInvitation(inv.Token, "secret1", "")
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if u.Email != "ella@example.com" || u.DisplayName != "Ella" {
		t.Fatalf("accepted user mismatch: %#v", u)
	}
	p, ok := users.Principal(u.Username)
	if !ok {
		t.Fatalf("principal not found")
	}
	if got := p.ProjectRoles[rbac.ProjectKey("tapnow")]; got != rbac.ProjectRoleOperator {
		t.Fatalf("project role=%q", got)
	}
	if got := p.AgentRoles[rbac.AgentKey("tapnow", "frontend")]; got != rbac.AgentRoleOperator {
		t.Fatalf("agent role=%q", got)
	}
}

func TestServerCanManageProjectRequiresManagerRole(t *testing.T) {
	users := newTestUserStore(t)
	if err := users.CreateUser("viewer", "pass123", RoleMember, "", "", "", "", ""); err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	if err := users.UpdateUser("viewer", nil, nil, nil, nil, nil, nil, nil, []projectAccess{{Project: "tapnow", Role: ProjectRoleViewer}}, nil, nil); err != nil {
		t.Fatalf("grant viewer: %v", err)
	}
	if err := users.CreateUser("operator", "pass123", RoleMember, "", "", "", "", ""); err != nil {
		t.Fatalf("create operator: %v", err)
	}
	if err := users.UpdateUser("operator", nil, nil, nil, nil, nil, nil, nil, []projectAccess{{Project: "tapnow", Role: ProjectRoleOperator}}, nil, nil); err != nil {
		t.Fatalf("grant operator: %v", err)
	}
	if err := users.CreateUser("manager", "pass123", RoleMember, "", "", "", "", ""); err != nil {
		t.Fatalf("create manager: %v", err)
	}
	if err := users.UpdateUser("manager", nil, nil, nil, nil, nil, nil, nil, []projectAccess{{Project: "tapnow", Role: ProjectRoleManager}}, nil, nil); err != nil {
		t.Fatalf("grant manager: %v", err)
	}

	s := &Server{users: users}
	cases := []struct {
		user string
		want bool
	}{
		{user: "viewer", want: false},
		{user: "operator", want: false},
		{user: "manager", want: true},
		{user: "admin", want: true},
	}
	for _, tc := range cases {
		req := httptest.NewRequest("GET", "/", nil)
		req = req.WithContext(context.WithValue(req.Context(), ctxUserKey, tc.user))
		if got := s.canManageProject(req, "tapnow"); got != tc.want {
			t.Fatalf("canManageProject(%s)=%v, want %v", tc.user, got, tc.want)
		}
	}
}

func TestWorkspaceMembershipQueriesByUser(t *testing.T) {
	users := newTestUserStore(t)
	if err := users.CreateUser("owner", "pass123", RoleMember, "", "", "", "", ""); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := users.db.UpsertWorkspace(controldb.Workspace{
		ID:        "ws-one",
		Name:      "One",
		Slug:      "one",
		Root:      filepath.Join(t.TempDir(), "one"),
		CreatedBy: "owner",
		CreatedAt: "2026-07-15T00:00:00Z",
	}); err != nil {
		t.Fatalf("workspace one: %v", err)
	}
	if err := users.db.UpsertWorkspace(controldb.Workspace{
		ID:        "ws-two",
		Name:      "Two",
		Slug:      "two",
		Root:      filepath.Join(t.TempDir(), "two"),
		CreatedBy: "someone",
		CreatedAt: "2026-07-15T00:00:00Z",
	}); err != nil {
		t.Fatalf("workspace two: %v", err)
	}
	if err := users.db.UpsertWorkspaceMember("ws-one", "owner", WorkspaceRoleOwner); err != nil {
		t.Fatalf("member: %v", err)
	}

	member, ok, err := users.db.WorkspaceMember("ws-one", "owner")
	if err != nil || !ok {
		t.Fatalf("workspace member ok=%v err=%v", ok, err)
	}
	if member.Role != WorkspaceRoleOwner {
		t.Fatalf("role=%q", member.Role)
	}
	memberships, err := users.db.ListWorkspaceMembersForUser("owner")
	if err != nil {
		t.Fatalf("memberships: %v", err)
	}
	if len(memberships) != 1 || memberships[0].WorkspaceID != "ws-one" {
		t.Fatalf("memberships=%#v", memberships)
	}
}

func TestAuditEventsCanBeCreatedAndFiltered(t *testing.T) {
	users := newTestUserStore(t)
	event := controldb.AuditEvent{
		ID:           "aud-test",
		WorkspaceID:  "ws-one",
		ActorType:    "user",
		ActorID:      "admin",
		Action:       "workspace.create",
		ResourceType: "workspace",
		ResourceID:   "ws-one",
		Summary:      "created",
		AfterJSON:    `{"name":"One"}`,
		CreatedAt:    "2026-07-15T00:00:00Z",
	}
	if err := users.db.CreateAuditEvent(event); err != nil {
		t.Fatalf("create audit event: %v", err)
	}
	events, err := users.db.ListAuditEvents(controldb.AuditEventFilter{
		WorkspaceID: "ws-one",
		Action:      "workspace.create",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events=%#v", events)
	}
	if events[0].ID != event.ID || events[0].AfterJSON != event.AfterJSON {
		t.Fatalf("event mismatch: %#v", events[0])
	}
	none, err := users.db.ListAuditEvents(controldb.AuditEventFilter{
		WorkspaceID: "ws-two",
		Action:      "workspace.create",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("list filtered audit events: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("expected no events, got %#v", none)
	}
}

func TestConnectionStorePersistsConnectionSecretAndGrant(t *testing.T) {
	users := newTestUserStore(t)
	now := "2026-07-15T00:00:00Z"
	if err := users.db.UpsertWorkspace(controldb.Workspace{
		ID:        "ws-one",
		Name:      "One",
		Slug:      "one",
		Root:      filepath.Join(t.TempDir(), "one"),
		CreatedBy: "admin",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("workspace: %v", err)
	}
	conn := controldb.Connection{
		ID:             "conn-one",
		WorkspaceID:    "ws-one",
		Provider:       "github",
		ConnectionName: "default",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        "ws-one",
		AuthType:       ConnectionAuthAPIKey,
		Status:         "active",
		ProfileJSON:    `{"displayName":"GitHub"}`,
		CreatedBy:      "admin",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := users.db.UpsertConnection(conn); err != nil {
		t.Fatalf("connection: %v", err)
	}
	if err := users.db.UpsertConnectionSecret(controldb.ConnectionSecret{
		ConnectionID: conn.ID,
		Ciphertext:   "secret-ciphertext",
		KeyVersion:   "test",
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("secret: %v", err)
	}
	grant := controldb.ConnectionGrant{
		ID:           "grant-one",
		WorkspaceID:  "ws-one",
		ConnectionID: conn.ID,
		TargetType:   ConnectionTargetWorkspace,
		TargetID:     "ws-one",
		CreatedBy:    "admin",
		CreatedAt:    now,
	}
	if err := users.db.CreateConnectionGrant(grant); err != nil {
		t.Fatalf("grant: %v", err)
	}

	connections, err := users.db.ListConnections(controldb.ConnectionFilter{WorkspaceID: "ws-one"})
	if err != nil {
		t.Fatalf("list connections: %v", err)
	}
	if len(connections) != 1 || connections[0].ID != conn.ID {
		t.Fatalf("connections=%#v", connections)
	}
	secret, ok, err := users.db.ConnectionSecret(conn.ID)
	if err != nil || !ok {
		t.Fatalf("connection secret ok=%v err=%v", ok, err)
	}
	if secret.Ciphertext != "secret-ciphertext" {
		t.Fatalf("secret=%#v", secret)
	}
	grants, err := users.db.ListConnectionGrants(conn.ID)
	if err != nil {
		t.Fatalf("list grants: %v", err)
	}
	if len(grants) != 1 || grants[0].ID != grant.ID {
		t.Fatalf("grants=%#v", grants)
	}
}
