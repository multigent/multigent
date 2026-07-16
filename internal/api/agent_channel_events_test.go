package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	controldb "github.com/multigent/multigent/internal/db"
	larkbridge "github.com/multigent/multigent/internal/imbridge/lark"
)

func TestChannelEventBindingRequiresExternalIdentity(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	if err := s.controlDB.UpsertConnection(controldb.Connection{
		ID:             "conn-feishu",
		WorkspaceID:    workspaceID,
		Provider:       "feishu",
		ConnectionName: "agent-sample-pm",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       "app_secret",
		Status:         "active",
		ProfileJSON:    "{}",
	}); err != nil {
		t.Fatalf("connection: %v", err)
	}
	secret, err := sealConnectionSecret(map[string]string{"baseUrl": "https://open.feishu.cn", "appId": "cli_app", "appSecret": "secret"})
	if err != nil {
		t.Fatalf("seal secret: %v", err)
	}
	secret.ConnectionID = "conn-feishu"
	if err := s.controlDB.UpsertConnectionSecret(secret); err != nil {
		t.Fatalf("secret: %v", err)
	}
	if err := s.controlDB.UpsertAgentChannelBinding(controldb.AgentChannelBinding{
		ID:           "chan-feishu",
		WorkspaceID:  workspaceID,
		ProjectID:    "sample",
		AgentID:      "pm",
		Provider:     "feishu",
		ConnectionID: "conn-feishu",
		Status:       "connected",
		MetadataJSON: `{"appId":"cli_app"}`,
	}); err != nil {
		t.Fatalf("binding: %v", err)
	}

	if _, found, err := s.resolveChannelEventBinding(workspaceID, "feishu", "cli_app", "", "ou_missing"); err != nil || found {
		t.Fatalf("missing identity found=%v err=%v", found, err)
	}
	if err := s.controlDB.UpsertExternalIdentity(controldb.ExternalIdentity{
		ID:             "ext-feishu",
		WorkspaceID:    workspaceID,
		Provider:       "feishu",
		ExternalUserID: "ou_owner",
		UserID:         "owner",
	}); err != nil {
		t.Fatalf("identity: %v", err)
	}
	resolved, found, err := s.resolveChannelEventBinding(workspaceID, "feishu", "cli_app", "", "ou_owner")
	if err != nil || !found {
		t.Fatalf("resolve found=%v err=%v", found, err)
	}
	if resolved.Identity.UserID != "owner" || resolved.Binding.ID != "chan-feishu" || resolved.SecretValues["appSecret"] != "secret" {
		t.Fatalf("resolved=%#v secrets=%#v", resolved, resolved.SecretValues)
	}
}

func TestChannelEventUserPermissionUsesAgentRBAC(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	grantProjectRoleForTest(t, s, workspaceID, "viewer", ProjectRoleViewer)
	grantProjectRoleForTest(t, s, workspaceID, "operator", ProjectRoleOperator)

	if s.userCanOperateAgent("viewer", "sample", "pm") {
		t.Fatalf("viewer should not operate agent")
	}
	if !s.userCanOperateAgent("operator", "sample", "pm") {
		t.Fatalf("operator should operate agent")
	}
	if !s.userCanOperateAgent("owner", "sample", "pm") {
		t.Fatalf("linked owner should operate own agent")
	}
	if s.userCanOperateAgent("owner", "sample", "backend") {
		t.Fatalf("linked owner should not operate unlinked agent")
	}
}

func TestLarkFamilyEventTokenVerification(t *testing.T) {
	env := larkbridge.EventEnvelope{Token: "token-one"}
	if !verifyLarkFamilyEventToken(env, map[string]string{}) {
		t.Fatalf("empty configured token should allow event")
	}
	if !verifyLarkFamilyEventToken(env, map[string]string{"verificationToken": "token-one"}) {
		t.Fatalf("matching token should allow event")
	}
	if verifyLarkFamilyEventToken(env, map[string]string{"verificationToken": "token-two"}) {
		t.Fatalf("mismatched token should reject event")
	}
	if verifyLarkFamilyEventToken(larkbridge.EventEnvelope{}, map[string]string{"verificationToken": "token-one"}) {
		t.Fatalf("missing event token should reject when configured")
	}
}

func TestAgentChannelSecurityPreservesConnectionSecret(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	if err := s.controlDB.UpsertConnection(controldb.Connection{
		ID:             "conn-feishu",
		WorkspaceID:    workspaceID,
		Provider:       "feishu",
		ConnectionName: "agent-sample-pm",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       "app_secret",
		Status:         "active",
		ProfileJSON:    "{}",
	}); err != nil {
		t.Fatalf("connection: %v", err)
	}
	secret, err := sealConnectionSecret(map[string]string{"baseUrl": "https://open.feishu.cn", "appId": "cli_app", "appSecret": "secret"})
	if err != nil {
		t.Fatalf("seal secret: %v", err)
	}
	secret.ConnectionID = "conn-feishu"
	if err := s.controlDB.UpsertConnectionSecret(secret); err != nil {
		t.Fatalf("secret: %v", err)
	}
	if err := s.controlDB.UpsertAgentChannelBinding(controldb.AgentChannelBinding{
		ID:           "chan-feishu",
		WorkspaceID:  workspaceID,
		ProjectID:    "sample",
		AgentID:      "pm",
		Provider:     "feishu",
		ConnectionID: "conn-feishu",
		Status:       "connected",
		MetadataJSON: `{"appId":"cli_app"}`,
	}); err != nil {
		t.Fatalf("binding: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/sample/agents/pm/channels/feishu/security", strings.NewReader(`{"verificationToken":"verify-one","encryptKey":"encrypt-one"}`))
	req = req.WithContext(context.WithValue(req.Context(), ctxUserKey, "owner"))
	req.SetPathValue("name", "sample")
	req.SetPathValue("agent", "pm")
	req.SetPathValue("provider", "feishu")
	rec := httptest.NewRecorder()
	s.handleAgentChannelSecurity(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	updated, ok, err := s.controlDB.ConnectionSecret("conn-feishu")
	if err != nil || !ok {
		t.Fatalf("updated secret ok=%v err=%v", ok, err)
	}
	values, err := openConnectionSecret(updated)
	if err != nil {
		t.Fatalf("open secret: %v", err)
	}
	if values["appId"] != "cli_app" || values["appSecret"] != "secret" || values["verificationToken"] != "verify-one" || values["encryptKey"] != "encrypt-one" {
		t.Fatalf("secret values not preserved/updated: %#v", values)
	}
}
