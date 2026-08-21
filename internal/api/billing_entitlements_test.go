package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
)

func requestWithEntitlements(req *http.Request, ent workspaceEntitlements) *http.Request {
	return req.WithContext(contextWithEntitlements(req.Context(), ent))
}

func contextWithEntitlements(ctx context.Context, ent workspaceEntitlements) context.Context {
	return context.WithValue(ctx, ctxEntitlementsKey, ent)
}

func TestCreateInvitationRespectsSeatLimit(t *testing.T) {
	s, _ := newConnectionGrantPolicyServer(t)
	req := providerTestRequest(http.MethodPost, "/api/v1/invitations", "admin", map[string]any{
		"emails": []string{"one@example.com"},
		"role":   WorkspaceRoleMember,
	})
	req = requestWithEntitlements(req, workspaceEntitlements{PlanCode: "trial", BillingStatus: "trialing", SeatLimit: 2})

	rec := httptest.NewRecorder()
	s.handleCreateInvitation(rec, req)
	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body apiErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if body.Error.Code != ErrCodeBillingLimitExceeded {
		t.Fatalf("code=%q", body.Error.Code)
	}
}

func TestBillingEntitlementsEndpointReturnsUsage(t *testing.T) {
	s, _ := newConnectionGrantPolicyServer(t)
	req := providerTestRequest(http.MethodGet, "/api/v1/billing/entitlements", "admin", nil)
	req = requestWithEntitlements(req, workspaceEntitlements{PlanCode: "business", BillingStatus: "active", SeatLimit: 10, AgentLimit: 50, RuntimeNodeLimit: 50})

	rec := httptest.NewRecorder()
	s.handleBillingEntitlements(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Entitlements workspaceEntitlements `json:"entitlements"`
		Usage        entitlementUsage      `json:"usage"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Entitlements.PlanCode != "business" || body.Usage.Seats == 0 {
		t.Fatalf("unexpected response: %+v", body)
	}
}

func TestBillingUsageCountsAgentWorkersOnceAcrossProjects(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	if err := s.st.SaveProject("other", &entity.Project{Name: "other"}); err != nil {
		t.Fatalf("save project: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	upsertRuntimeAttentionWorker(t, s, workspaceID, "aw-shared-dev", "sample", "shared-dev", now)
	if err := s.controlDB.UpsertProjectMembership(controldb.ProjectMembership{
		ID:          "pm-shared-other",
		WorkspaceID: workspaceID,
		ProjectID:   "other",
		MemberType:  "agent_worker",
		MemberID:    "aw-shared-dev",
		Title:       "shared-dev",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("other membership: %v", err)
	}

	usage, err := s.entitlementUsage(workspaceID)
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	if usage.Agents != 1 {
		t.Fatalf("expected one workspace-level agent worker, got %+v", usage)
	}
}
