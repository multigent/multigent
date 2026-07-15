package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInvitationManagementUsesWorkspaceAdmin(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	if err := s.users.CreateUser("member", "pass123", RoleMember, "", "member@example.com", "", "", ""); err != nil {
		t.Fatalf("create member: %v", err)
	}
	if err := s.controlDB.UpsertWorkspaceMember(workspaceID, "member", WorkspaceRoleMember); err != nil {
		t.Fatalf("workspace member: %v", err)
	}

	memberRec := httptest.NewRecorder()
	s.handleCreateInvitation(memberRec, providerTestRequest(http.MethodPost, "/api/v1/invitations", "member", map[string]string{
		"email": "new@example.com",
		"role":  WorkspaceRoleMember,
	}))
	if memberRec.Code != http.StatusForbidden {
		t.Fatalf("member create invitation status=%d body=%s", memberRec.Code, memberRec.Body.String())
	}

	adminRec := httptest.NewRecorder()
	s.handleCreateInvitation(adminRec, providerTestRequest(http.MethodPost, "/api/v1/invitations", "admin", map[string]string{
		"email":       "new@example.com",
		"role":        WorkspaceRoleAdmin,
		"displayName": "New User",
	}))
	if adminRec.Code != http.StatusOK {
		t.Fatalf("admin create invitation status=%d body=%s", adminRec.Code, adminRec.Body.String())
	}
	var created struct {
		Invitation invitationRecord `json:"invitation"`
		InviteURL  string           `json:"inviteUrl"`
	}
	if err := json.Unmarshal(adminRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create invitation: %v", err)
	}
	if created.Invitation.Token == "" || created.Invitation.Role != WorkspaceRoleAdmin || created.InviteURL == "" {
		t.Fatalf("created invitation=%#v url=%q", created.Invitation, created.InviteURL)
	}

	listRec := httptest.NewRecorder()
	s.handleListInvitations(listRec, providerTestRequest(http.MethodGet, "/api/v1/invitations", "admin", nil))
	if listRec.Code != http.StatusOK {
		t.Fatalf("list invitations status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	var list struct {
		Invitations []invitationRecord `json:"invitations"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list invitations: %v", err)
	}
	if len(list.Invitations) != 1 || list.Invitations[0].Token != created.Invitation.Token {
		t.Fatalf("listed invitations=%#v", list.Invitations)
	}

	revokeReq := providerTestRequest(http.MethodPost, "/api/v1/invitations/"+created.Invitation.Token+"/revoke", "admin", nil)
	revokeReq.SetPathValue("token", created.Invitation.Token)
	revokeRec := httptest.NewRecorder()
	s.handleRevokeInvitation(revokeRec, revokeReq)
	if revokeRec.Code != http.StatusOK {
		t.Fatalf("revoke invitation status=%d body=%s", revokeRec.Code, revokeRec.Body.String())
	}
	inv, ok := s.users.Invitation(created.Invitation.Token)
	if !ok || inv.Status != "revoked" {
		t.Fatalf("revoked invitation=%#v ok=%v", inv, ok)
	}
}

func TestAcceptInvitationJoinsCurrentWorkspaceAndRejectBlocksAccept(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	inv, err := s.users.CreateInvitation("accepted@example.com", WorkspaceRoleAdmin, "Accepted", "admin", nil, nil)
	if err != nil {
		t.Fatalf("create invitation: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/invitations/"+inv.Token+"/accept", bytes.NewReader([]byte(`{"password":"secret1"}`)))
	req.SetPathValue("token", inv.Token)
	rec := httptest.NewRecorder()
	s.handleAcceptInvitation(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("accept status=%d body=%s", rec.Code, rec.Body.String())
	}
	user := s.users.GetUser("accepted")
	if user == nil {
		t.Fatalf("accepted user missing")
	}
	if user.Role != RoleMember {
		t.Fatalf("accepted user should remain org member, role=%q", user.Role)
	}
	member, ok, err := s.controlDB.WorkspaceMember(workspaceID, user.Username)
	if err != nil || !ok {
		t.Fatalf("workspace membership ok=%v err=%v", ok, err)
	}
	if member.Role != WorkspaceRoleAdmin {
		t.Fatalf("workspace role=%q", member.Role)
	}

	rejected, err := s.users.CreateInvitation("rejected@example.com", WorkspaceRoleMember, "Rejected", "admin", nil, nil)
	if err != nil {
		t.Fatalf("create rejected invitation: %v", err)
	}
	rejectReq := httptest.NewRequest(http.MethodPost, "/api/v1/invitations/"+rejected.Token+"/reject", nil)
	rejectReq.SetPathValue("token", rejected.Token)
	rejectRec := httptest.NewRecorder()
	s.handleRejectInvitation(rejectRec, rejectReq)
	if rejectRec.Code != http.StatusOK {
		t.Fatalf("reject status=%d body=%s", rejectRec.Code, rejectRec.Body.String())
	}
	acceptRejectedReq := httptest.NewRequest(http.MethodPost, "/api/v1/invitations/"+rejected.Token+"/accept", bytes.NewReader([]byte(`{"password":"secret1"}`)))
	acceptRejectedReq.SetPathValue("token", rejected.Token)
	acceptRejectedRec := httptest.NewRecorder()
	s.handleAcceptInvitation(acceptRejectedRec, acceptRejectedReq)
	if acceptRejectedRec.Code != http.StatusBadRequest {
		t.Fatalf("accept rejected status=%d body=%s", acceptRejectedRec.Code, acceptRejectedRec.Body.String())
	}
}
