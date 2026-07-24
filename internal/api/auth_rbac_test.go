package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/rbac"
	"github.com/multigent/multigent/internal/store"
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
	projects := []projectAccess{{Project: "sample", Role: ProjectRoleViewer}}
	agentGrants := []agentAccess{{Project: "sample", Agent: "connector-dev", Role: string(rbac.AgentRoleOperator)}}

	if err := users.CreateUser("dev", "pass", role, "", "", "", "", ""); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := users.UpdateUser("dev", nil, nil, nil, nil, nil, nil, nil, projects, agentGrants, nil); err != nil {
		t.Fatalf("update user: %v", err)
	}

	p, ok := users.Principal("dev")
	if !ok {
		t.Fatalf("principal not found")
	}
	if p.OrgRole != rbac.OrgRoleMember {
		t.Fatalf("org role=%q", p.OrgRole)
	}
	if got := p.ProjectRoles[rbac.ProjectKey("sample")]; got != rbac.ProjectRoleViewer {
		t.Fatalf("project role=%q", got)
	}
	if got := p.AgentRoles[rbac.AgentKey("sample", "connector-dev")]; got != rbac.AgentRoleOperator {
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

func TestRegisterBlockedWhenOpenRegistrationDisabled(t *testing.T) {
	s, _ := newProviderHandlerTestServer(t)
	if err := s.controlDB.SetSetting(openRegistrationSettingKey, "false"); err != nil {
		t.Fatalf("set setting: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"email":"new@example.com","password":"secret1","displayName":"New User"}`))
	req.Header.Set("Content-Type", "application/json")
	s.handleRegister(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("register status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body apiErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if body.Error.Code != ErrCodeSignupDisabled {
		t.Fatalf("code=%q", body.Error.Code)
	}
}

func TestAuthSettingsToggleControlsOpenRegistration(t *testing.T) {
	s, _ := newProviderHandlerTestServer(t)
	if err := s.users.CreateUser("instance-admin", "pass123", RoleAdmin, "Instance Admin", "instance-admin@example.com", "", "", ""); err != nil {
		t.Fatalf("create instance admin: %v", err)
	}
	if !s.openRegistrationEnabled() {
		t.Fatalf("registration should default to enabled")
	}
	rec := httptest.NewRecorder()
	s.handlePutAuthSettings(rec, providerTestRequest(http.MethodPut, "/api/v1/auth/settings", "instance-admin", map[string]any{
		"openRegistrationEnabled": false,
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("settings status=%d body=%s", rec.Code, rec.Body.String())
	}
	if s.openRegistrationEnabled() {
		t.Fatalf("registration should be disabled")
	}
	publicRec := httptest.NewRecorder()
	s.handlePublicAuthSettings(publicRec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/settings/public", nil))
	var publicBody struct {
		OpenRegistrationEnabled bool `json:"openRegistrationEnabled"`
	}
	if err := json.Unmarshal(publicRec.Body.Bytes(), &publicBody); err != nil {
		t.Fatalf("decode public settings: %v", err)
	}
	if publicBody.OpenRegistrationEnabled {
		t.Fatalf("public settings should report disabled")
	}
}

func TestCreateUserWithEmailOnlyAddsWorkspaceMember(t *testing.T) {
	s, _ := newProviderHandlerTestServer(t)
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		t.Fatalf("workspace id: %v", err)
	}
	rec := httptest.NewRecorder()
	s.handleCreateUser(rec, providerTestRequest(http.MethodPost, "/api/v1/users", "owner", map[string]any{
		"email":         "new.person@example.com",
		"displayName":   "New Person",
		"password":      "secret1",
		"workspaceRole": WorkspaceRoleAdmin,
	}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create user status=%d body=%s", rec.Code, rec.Body.String())
	}
	u := s.users.UserByEmail("new.person@example.com")
	if u == nil {
		t.Fatalf("user not created")
	}
	if u.Username != "new-person" {
		t.Fatalf("username=%q", u.Username)
	}
	member, ok, err := s.controlDB.WorkspaceMember(workspaceID, u.Username)
	if err != nil || !ok {
		t.Fatalf("workspace member ok=%v err=%v", ok, err)
	}
	if member.Role != WorkspaceRoleAdmin {
		t.Fatalf("workspace role=%q", member.Role)
	}
}

func TestUserStoreAcceptInvitationCreatesMemberWithGrants(t *testing.T) {
	users := newTestUserStore(t)
	projects := []projectAccess{{Project: "sample", Role: ProjectRoleOperator}}

	inv, err := users.CreateInvitation("ws-one", "Ella@Example.com", RoleMember, "Ella", "admin", projects, []agentAccess{{Project: "sample", Agent: "frontend", Role: string(rbac.AgentRoleOperator)}})
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
	if got := p.ProjectRoles[rbac.ProjectKey("sample")]; got != rbac.ProjectRoleOperator {
		t.Fatalf("project role=%q", got)
	}
	if got := p.AgentRoles[rbac.AgentKey("sample", "frontend")]; got != rbac.AgentRoleOperator {
		t.Fatalf("agent role=%q", got)
	}
}

func TestServerCanManageProjectRequiresManagerRole(t *testing.T) {
	users := newTestUserStore(t)
	if err := users.CreateUser("viewer", "pass123", RoleMember, "", "", "", "", ""); err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	if err := users.UpdateUser("viewer", nil, nil, nil, nil, nil, nil, nil, []projectAccess{{Project: "sample", Role: ProjectRoleViewer}}, nil, nil); err != nil {
		t.Fatalf("grant viewer: %v", err)
	}
	if err := users.CreateUser("operator", "pass123", RoleMember, "", "", "", "", ""); err != nil {
		t.Fatalf("create operator: %v", err)
	}
	if err := users.UpdateUser("operator", nil, nil, nil, nil, nil, nil, nil, []projectAccess{{Project: "sample", Role: ProjectRoleOperator}}, nil, nil); err != nil {
		t.Fatalf("grant operator: %v", err)
	}
	if err := users.CreateUser("manager", "pass123", RoleMember, "", "", "", "", ""); err != nil {
		t.Fatalf("create manager: %v", err)
	}
	if err := users.UpdateUser("manager", nil, nil, nil, nil, nil, nil, nil, []projectAccess{{Project: "sample", Role: ProjectRoleManager}}, nil, nil); err != nil {
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
		if got := s.canManageProject(req, "sample"); got != tc.want {
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

func TestEnsureCurrentUserMembershipDoesNotDowngradeExistingRole(t *testing.T) {
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
		t.Fatalf("workspace: %v", err)
	}
	if err := users.db.UpsertWorkspaceMember("ws-one", "owner", WorkspaceRoleOwner); err != nil {
		t.Fatalf("seed owner membership: %v", err)
	}
	s := &Server{controlDB: users.db, users: users}
	if err := s.ensureCurrentUserMembership("ws-one", "owner"); err != nil {
		t.Fatalf("ensure membership: %v", err)
	}
	member, ok, err := users.db.WorkspaceMember("ws-one", "owner")
	if err != nil || !ok {
		t.Fatalf("workspace member ok=%v err=%v", ok, err)
	}
	if member.Role != WorkspaceRoleOwner {
		t.Fatalf("role downgraded to %q", member.Role)
	}
}

func TestCurrentWorkspaceAccessAutoSwitchesToUserWorkspace(t *testing.T) {
	users := newTestUserStore(t)
	base := t.TempDir()
	t.Setenv("MULTIGENT_DATA_DIR", base)
	defaultRoot := filepath.Join(base, "default")
	userRoot := filepath.Join(base, "user")
	for _, root := range []string{defaultRoot, userRoot} {
		if err := os.MkdirAll(filepath.Join(root, ".multigent"), 0o755); err != nil {
			t.Fatalf("mkdir workspace: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, ".multigent", "agency.yaml"), []byte("name: Test\n"), 0o644); err != nil {
			t.Fatalf("write agency: %v", err)
		}
	}
	if err := users.CreateUser("admin-2", "pass123", RoleMember, "Admin", "admin@example.test", "", "", ""); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := users.db.UpsertWorkspace(controldb.Workspace{
		ID:        "ws-default",
		Name:      "Default Workspace",
		Slug:      "default",
		Root:      defaultRoot,
		CreatedBy: "system",
		CreatedAt: "2026-07-18T00:00:00Z",
	}); err != nil {
		t.Fatalf("default workspace: %v", err)
	}
	if err := users.db.UpsertWorkspace(controldb.Workspace{
		ID:        "ws-user",
		Name:      "User Workspace",
		Slug:      "user",
		Root:      userRoot,
		CreatedBy: "admin-2",
		CreatedAt: "2026-07-18T00:00:00Z",
	}); err != nil {
		t.Fatalf("user workspace: %v", err)
	}
	if err := users.db.UpsertWorkspaceMember("ws-user", "admin-2", WorkspaceRoleOwner); err != nil {
		t.Fatalf("workspace member: %v", err)
	}
	s := &Server{root: defaultRoot, controlDB: users.db, users: users, st: store.NewDB(defaultRoot, users.db)}
	rec := httptest.NewRecorder()
	req := providerTestRequest(http.MethodGet, "/api/v1/workspace", "admin-2", nil)
	if !s.checkCurrentWorkspaceAccess(rec, req) {
		t.Fatalf("workspace access denied status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !samePath(s.root, userRoot) {
		t.Fatalf("root=%q, want %q", s.root, userRoot)
	}
}

func TestListUsersReturnsWorkspaceRole(t *testing.T) {
	s, _ := newProviderHandlerTestServer(t)
	if err := s.users.CreateUser("instance-admin", "pass123", RoleAdmin, "Instance Admin", "instance-admin@example.com", "", "", ""); err != nil {
		t.Fatalf("create instance admin: %v", err)
	}
	rec := httptest.NewRecorder()
	s.handleListUsers(rec, providerTestRequest(http.MethodGet, "/api/v1/users", "owner", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list users status=%d body=%s", rec.Code, rec.Body.String())
	}
	var rows []struct {
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode users: %v", err)
	}
	roles := map[string]string{}
	for _, row := range rows {
		roles[row.Username] = row.Role
	}
	if roles["owner"] != WorkspaceRoleOwner {
		t.Fatalf("owner role=%q", roles["owner"])
	}
	if roles["member"] != WorkspaceRoleMember {
		t.Fatalf("member role=%q", roles["member"])
	}
	if _, ok := roles["instance-admin"]; ok {
		t.Fatalf("global instance admin should not be listed as a workspace member: %#v", rows)
	}
}

func TestListUsersWorkspaceMemberCanReadSafeDirectory(t *testing.T) {
	s, _ := newProviderHandlerTestServer(t)

	rec := httptest.NewRecorder()
	s.handleListUsers(rec, providerTestRequest(http.MethodGet, "/api/v1/users", "member", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list users status=%d body=%s", rec.Code, rec.Body.String())
	}
	var rows []struct {
		Username     string          `json:"username"`
		Role         string          `json:"role"`
		DisplayName  string          `json:"displayName"`
		Email        string          `json:"email"`
		Projects     []projectAccess `json:"projects,omitempty"`
		LinkedAgents []string        `json:"linkedAgents,omitempty"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode users: %v", err)
	}
	if len(rows) == 0 {
		t.Fatalf("expected workspace directory rows")
	}
	seenOwner := false
	for _, row := range rows {
		if row.Username == "owner" {
			seenOwner = true
			if len(row.Projects) != 0 || len(row.LinkedAgents) != 0 {
				t.Fatalf("member should not receive other users' management fields: %#v", row)
			}
		}
	}
	if !seenOwner {
		t.Fatalf("expected owner in workspace directory: %#v", rows)
	}

	outsiderRec := httptest.NewRecorder()
	s.handleListUsers(outsiderRec, providerTestRequest(http.MethodGet, "/api/v1/users", "outsider", nil))
	if outsiderRec.Code != http.StatusForbidden {
		t.Fatalf("outsider status=%d body=%s", outsiderRec.Code, outsiderRec.Body.String())
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
