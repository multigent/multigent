package api

import (
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
