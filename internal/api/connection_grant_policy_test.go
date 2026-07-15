package api

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"testing"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/store"
)

func newConnectionGrantPolicyServer(t *testing.T) (*Server, string) {
	t.Helper()
	db, err := controldb.Open(filepath.Join(t.TempDir(), "multigent.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	root := filepath.Join(t.TempDir(), "workspace")
	st := store.NewDB(root, db)
	s := &Server{root: root, controlDB: db, st: st, users: newUserStore(db)}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		t.Fatalf("workspace id: %v", err)
	}
	if err := s.users.CreateUser("owner", "pass123", RoleMember, "", "", "", "", ""); err != nil {
		t.Fatalf("create owner: %v", err)
	}
	if err := s.controlDB.UpsertWorkspaceMember(workspaceID, "admin", WorkspaceRoleAdmin); err != nil {
		t.Fatalf("admin member: %v", err)
	}
	if err := s.controlDB.UpsertWorkspaceMember(workspaceID, "owner", WorkspaceRoleMember); err != nil {
		t.Fatalf("owner member: %v", err)
	}
	if err := s.users.UpdateUser("owner", nil, nil, nil, nil, nil, nil, nil, nil, []string{"tapnow/pm"}, nil); err != nil {
		t.Fatalf("link owner agent: %v", err)
	}
	if err := st.SaveProject("tapnow", &entity.Project{Name: "tapnow"}); err != nil {
		t.Fatalf("save project: %v", err)
	}
	for _, agent := range []string{"pm", "backend"} {
		if err := st.SaveAgentMeta("tapnow", agent, &entity.AgentMeta{Name: agent, Project: "tapnow"}); err != nil {
			t.Fatalf("save agent %s: %v", agent, err)
		}
	}
	return s, workspaceID
}

func TestUserOwnedConnectionGrantTargetsAreLimitedToOwnerAndLinkedAgents(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	connection := controldb.Connection{
		ID:             "conn-owner",
		WorkspaceID:    workspaceID,
		Provider:       "github",
		ConnectionName: "default",
		OwnerType:      ConnectionOwnerUser,
		OwnerID:        "owner",
		AuthType:       ConnectionAuthAPIKey,
		Status:         "active",
	}
	req := httptest.NewRequest("POST", "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxUserKey, "admin"))

	allowed := []struct {
		targetType string
		targetID   string
	}{
		{ConnectionTargetUser, "owner"},
		{ConnectionTargetAgent, "tapnow/pm"},
	}
	for _, tc := range allowed {
		if err := s.validateConnectionGrantTarget(req, connection, tc.targetType, tc.targetID); err != nil {
			t.Fatalf("expected %s/%s to be allowed: %v", tc.targetType, tc.targetID, err)
		}
	}

	blocked := []struct {
		targetType string
		targetID   string
	}{
		{ConnectionTargetWorkspace, workspaceID},
		{ConnectionTargetProject, "tapnow"},
		{ConnectionTargetUser, "admin"},
		{ConnectionTargetAgent, "tapnow/backend"},
	}
	for _, tc := range blocked {
		if err := s.validateConnectionGrantTarget(req, connection, tc.targetType, tc.targetID); err == nil {
			t.Fatalf("expected %s/%s to be blocked", tc.targetType, tc.targetID)
		}
	}
}
