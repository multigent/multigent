package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/store"
	"github.com/multigent/multigent/internal/taskstore"
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
	ts := taskstore.NewDB(root, db)
	s := &Server{root: root, controlDB: db, st: st, ts: ts, users: newUserStore(db)}
	s.triggers = newTriggerManager(root, "", ts)
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
	if err := s.users.UpdateUser("owner", nil, nil, nil, nil, nil, nil, nil, nil, []string{"sample/pm"}, nil); err != nil {
		t.Fatalf("link owner agent: %v", err)
	}
	if err := st.SaveProject("sample", &entity.Project{Name: "sample"}); err != nil {
		t.Fatalf("save project: %v", err)
	}
	for _, agent := range []string{"pm", "backend"} {
		if err := st.SaveAgentMeta("sample", agent, &entity.AgentMeta{Name: agent, Project: "sample"}); err != nil {
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
	req = req.WithContext(context.WithValue(req.Context(), ctxUserKey, "owner"))

	allowed := []struct {
		targetType string
		targetID   string
	}{
		{ConnectionTargetUser, "owner"},
		{ConnectionTargetAgent, "sample/pm"},
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
		{ConnectionTargetProject, "sample"},
		{ConnectionTargetUser, "admin"},
		{ConnectionTargetAgent, "sample/backend"},
	}
	for _, tc := range blocked {
		if err := s.validateConnectionGrantTarget(req, connection, tc.targetType, tc.targetID); err == nil {
			t.Fatalf("expected %s/%s to be blocked", tc.targetType, tc.targetID)
		}
	}
}

func TestUserOwnedConnectionGrantMustBeCreatedByOwner(t *testing.T) {
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
		ProfileJSON:    `{}`,
		CreatedBy:      "owner",
	}
	if err := s.controlDB.UpsertConnection(connection); err != nil {
		t.Fatalf("connection: %v", err)
	}

	adminRec := httptest.NewRecorder()
	adminReq := providerTestRequest(http.MethodPost, "/api/v1/connections/conn-owner/grants", "admin", createConnectionGrantRequest{
		TargetType: ConnectionTargetAgent,
		TargetID:   "sample/pm",
	})
	adminReq.SetPathValue("id", "conn-owner")
	s.handleCreateConnectionGrant(adminRec, adminReq)
	if adminRec.Code != http.StatusForbidden {
		t.Fatalf("admin grant status=%d body=%s", adminRec.Code, adminRec.Body.String())
	}

	ownerRec := httptest.NewRecorder()
	ownerReq := providerTestRequest(http.MethodPost, "/api/v1/connections/conn-owner/grants", "owner", createConnectionGrantRequest{
		TargetType: ConnectionTargetAgent,
		TargetID:   "sample/pm",
	})
	ownerReq.SetPathValue("id", "conn-owner")
	s.handleCreateConnectionGrant(ownerRec, ownerReq)
	if ownerRec.Code != http.StatusCreated {
		t.Fatalf("owner grant status=%d body=%s", ownerRec.Code, ownerRec.Body.String())
	}
}

func TestUserOwnedConnectionCanGrantToOperatedProjectAgents(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	grantProjectRoleForTest(t, s, workspaceID, "operator", ProjectRoleOperator)
	grantProjectRoleForTest(t, s, workspaceID, "viewer", ProjectRoleViewer)
	for _, username := range []string{"operator", "viewer"} {
		connection := controldb.Connection{
			ID:             "conn-" + username,
			WorkspaceID:    workspaceID,
			Provider:       "github",
			ConnectionName: "default",
			OwnerType:      ConnectionOwnerUser,
			OwnerID:        username,
			AuthType:       ConnectionAuthAPIKey,
			Status:         "active",
			ProfileJSON:    `{}`,
			CreatedBy:      username,
		}
		if err := s.controlDB.UpsertConnection(connection); err != nil {
			t.Fatalf("connection %s: %v", username, err)
		}
	}

	operatorRec := httptest.NewRecorder()
	operatorReq := providerTestRequest(http.MethodPost, "/api/v1/connections/conn-operator/grants", "operator", createConnectionGrantRequest{
		TargetType: ConnectionTargetAgent,
		TargetID:   "sample/backend",
	})
	operatorReq.SetPathValue("id", "conn-operator")
	s.handleCreateConnectionGrant(operatorRec, operatorReq)
	if operatorRec.Code != http.StatusCreated {
		t.Fatalf("operator grant status=%d body=%s", operatorRec.Code, operatorRec.Body.String())
	}

	viewerRec := httptest.NewRecorder()
	viewerReq := providerTestRequest(http.MethodPost, "/api/v1/connections/conn-viewer/grants", "viewer", createConnectionGrantRequest{
		TargetType: ConnectionTargetAgent,
		TargetID:   "sample/backend",
	})
	viewerReq.SetPathValue("id", "conn-viewer")
	s.handleCreateConnectionGrant(viewerRec, viewerReq)
	if viewerRec.Code != http.StatusBadRequest {
		t.Fatalf("viewer grant status=%d body=%s", viewerRec.Code, viewerRec.Body.String())
	}
}
