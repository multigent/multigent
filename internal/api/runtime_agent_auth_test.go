package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
)

func TestAgentRuntimeTokenValidateAndExpire(t *testing.T) {
	users := newTestUserStore(t)
	s := &Server{controlDB: users.db, users: users}

	token := s.issueAgentRuntimeToken(runtimeAgentTokenPayload{
		WorkspaceID:  "ws-one",
		Project:      "sample",
		Agent:        "pm",
		RunID:        "run-one",
		Capabilities: []string{"connection.use"},
	}, time.Minute)
	principal, ok := s.validateAgentRuntimeToken(token)
	if !ok {
		t.Fatalf("runtime token did not validate")
	}
	if principal.WorkspaceID != "ws-one" || principal.Project != "sample" || principal.Agent != "pm" || principal.RunID != "run-one" {
		t.Fatalf("principal mismatch: %#v", principal)
	}
	if !runtimeHasCapability(principal, "connection.use") {
		t.Fatalf("capability missing: %#v", principal.Capabilities)
	}

	expired := s.issueAgentRuntimeToken(runtimeAgentTokenPayload{
		WorkspaceID:  "ws-one",
		Project:      "sample",
		Agent:        "pm",
		Capabilities: []string{"connection.use"},
	}, -time.Second)
	if _, ok := s.validateAgentRuntimeToken(expired); ok {
		t.Fatalf("expired token validated")
	}
}

func TestNormalizeRuntimeCapabilitiesFiltersUnsupportedValues(t *testing.T) {
	got := normalizeRuntimeCapabilities([]string{"connection.use", "task.use", "connection.use", "", "agent.admin"})
	if len(got) != 2 || got[0] != "connection.use" || got[1] != "task.use" {
		t.Fatalf("capabilities=%#v", got)
	}
	got = normalizeRuntimeCapabilities([]string{"task.read"})
	if len(got) != len(defaultRuntimeCapabilities()) {
		t.Fatalf("fallback capabilities=%#v", got)
	}
}

func TestIssueAgentRuntimeTokenRequiresAgentOperatorAccess(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	grantProjectRoleForTest(t, s, workspaceID, "viewer", ProjectRoleViewer)
	grantProjectRoleForTest(t, s, workspaceID, "operator", ProjectRoleOperator)

	viewerReq := issueAgentRuntimeTokenRequestForTest("viewer", "sample", "pm")
	viewerRec := httptest.NewRecorder()
	s.handleIssueAgentRuntimeToken(viewerRec, viewerReq)
	if viewerRec.Code != http.StatusForbidden {
		t.Fatalf("viewer status=%d body=%s", viewerRec.Code, viewerRec.Body.String())
	}

	operatorReq := issueAgentRuntimeTokenRequestForTest("operator", "sample", "backend")
	operatorRec := httptest.NewRecorder()
	s.handleIssueAgentRuntimeToken(operatorRec, operatorReq)
	if operatorRec.Code != http.StatusOK {
		t.Fatalf("operator status=%d body=%s", operatorRec.Code, operatorRec.Body.String())
	}
	var body struct {
		Token        string   `json:"token"`
		Capabilities []string `json:"capabilities"`
		Project      string   `json:"project"`
		Agent        string   `json:"agent"`
	}
	if err := json.Unmarshal(operatorRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	if body.Token == "" || body.Project != "sample" || body.Agent != "backend" {
		t.Fatalf("bad token response=%#v", body)
	}
	if len(body.Capabilities) != 1 || body.Capabilities[0] != "connection.use" {
		t.Fatalf("capabilities=%#v", body.Capabilities)
	}

	ownerReq := issueAgentRuntimeTokenRequestForTest("owner", "sample", "pm")
	ownerRec := httptest.NewRecorder()
	s.handleIssueAgentRuntimeToken(ownerRec, ownerReq)
	if ownerRec.Code != http.StatusOK {
		t.Fatalf("linked owner status=%d body=%s", ownerRec.Code, ownerRec.Body.String())
	}
}

func issueAgentRuntimeTokenRequestForTest(username, project, agent string) *http.Request {
	req := providerTestRequest(http.MethodPost, "/api/v1/projects/"+project+"/agents/"+agent+"/runtime/token", username, issueAgentRuntimeTokenRequest{
		Capabilities: []string{"connection.use", "agent.admin"},
	})
	req.SetPathValue("name", project)
	req.SetPathValue("agent", agent)
	return req
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
		TargetID:     "sample/pm",
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
		Project:      "sample",
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
