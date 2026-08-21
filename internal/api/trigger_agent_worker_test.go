package api

import (
	"testing"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
)

func TestTriggerManagerUsesAgentWorkerScheduleBeforeLegacyHeartbeat(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.ts.SaveHeartbeat("sample", "pm", &entity.HeartbeatConfig{
		Triggers: []entity.TriggerType{entity.TriggerOnMessage},
	}); err != nil {
		t.Fatalf("legacy heartbeat: %v", err)
	}
	worker := controldb.AgentWorker{
		ID:           "aw-nova-trigger",
		WorkspaceID:  workspaceID,
		Name:         "nova",
		DisplayName:  "Nova",
		Status:       "active",
		ScheduleJSON: `{"triggers":["task"],"triggerDebounce":"1s"}`,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
		t.Fatalf("worker: %v", err)
	}
	if err := s.controlDB.UpsertProjectMembership(controldb.ProjectMembership{
		ID:          "pm-nova-trigger",
		WorkspaceID: workspaceID,
		ProjectID:   "sample",
		MemberType:  "agent_worker",
		MemberID:    worker.ID,
		Role:        "pm",
		Title:       "pm",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("membership: %v", err)
	}

	if resolved, ok := s.triggers.resolveAgentWorker("sample", "pm"); !ok {
		memberships, _ := s.controlDB.ListProjectMemberships(controldb.ProjectMembershipFilter{WorkspaceID: workspaceID, ProjectID: "sample", MemberType: "agent_worker"})
		t.Fatalf("expected worker resolution; memberships=%#v", memberships)
	} else if resolved.ID != worker.ID {
		t.Fatalf("resolved wrong worker: %#v", resolved)
	} else if directHB, configured := agentWorkerScheduleHeartbeat(resolved); !configured || directHB == nil || !directHB.HasTrigger(entity.TriggerOnTask) {
		t.Fatalf("resolved worker schedule was not configured: raw=%q hb=%#v configured=%v", resolved.ScheduleJSON, directHB, configured)
	}
	hb, configured := s.triggers.heartbeatForTrigger("sample", "pm")
	if !configured || hb == nil {
		t.Fatalf("expected worker schedule heartbeat")
	}
	if !hb.HasTrigger(entity.TriggerOnTask) {
		t.Fatalf("expected worker task trigger: %#v", hb.Triggers)
	}
	if hb.HasTrigger(entity.TriggerOnMessage) {
		t.Fatalf("worker schedule should take precedence over legacy heartbeat: %#v", hb.Triggers)
	}
}

func TestTriggerManagerFallsBackToLegacyHeartbeatWithoutWorkerTriggers(t *testing.T) {
	s, _ := newConnectionGrantPolicyServer(t)
	if err := s.ts.SaveHeartbeat("sample", "pm", &entity.HeartbeatConfig{
		Triggers: []entity.TriggerType{entity.TriggerOnMessage},
	}); err != nil {
		t.Fatalf("legacy heartbeat: %v", err)
	}

	hb, configured := s.triggers.heartbeatForTrigger("sample", "pm")
	if !configured || hb == nil {
		t.Fatalf("expected fallback heartbeat")
	}
	if !hb.HasTrigger(entity.TriggerOnMessage) {
		t.Fatalf("expected legacy message trigger fallback: %#v", hb.Triggers)
	}
}
