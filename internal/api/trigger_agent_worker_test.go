package api

import (
	"testing"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
)

func TestTriggerManagerUsesAgentWorkerSchedule(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
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
		t.Fatalf("unexpected message trigger: %#v", hb.Triggers)
	}
}

func TestTriggerManagerFindsDueScheduledTaskWithoutTaskTrigger(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC()
	worker := controldb.AgentWorker{
		ID:           "aw-nova-scheduled",
		WorkspaceID:  workspaceID,
		Name:         "nova",
		DisplayName:  "Nova",
		Status:       "active",
		ScheduleJSON: `{"triggers":["im_direct_message"],"triggerDebounce":"1s"}`,
		CreatedAt:    now.Format(time.RFC3339),
		UpdatedAt:    now.Format(time.RFC3339),
	}
	if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
		t.Fatalf("worker: %v", err)
	}
	if err := s.controlDB.UpsertProjectMembership(controldb.ProjectMembership{
		ID:          "pm-nova-scheduled",
		WorkspaceID: workspaceID,
		ProjectID:   "sample",
		MemberType:  "agent_worker",
		MemberID:    worker.ID,
		Role:        "pm",
		Title:       "pm",
		CreatedAt:   now.Format(time.RFC3339),
		UpdatedAt:   now.Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("membership: %v", err)
	}

	future := now.Add(10 * time.Minute)
	if err := s.ts.AddTask("sample", "pm", &entity.Task{
		ID:        "future-wakeup",
		Title:     "Future reminder",
		Status:    entity.TaskStatusPending,
		Priority:  1,
		CreatedAt: now,
		UpdatedAt: now,
		NotBefore: &future,
	}); err != nil {
		t.Fatalf("add future task: %v", err)
	}
	if task, ok := s.triggers.nextDueScheduledTask("sample", "pm", now); ok {
		t.Fatalf("future task should not be due yet: %#v", task)
	}

	due := now.Add(-time.Minute)
	if err := s.ts.AddTask("sample", "pm", &entity.Task{
		ID:        "due-wakeup",
		Title:     "Due reminder",
		Status:    entity.TaskStatusPending,
		Priority:  2,
		CreatedAt: now.Add(time.Second),
		UpdatedAt: now,
		NotBefore: &due,
	}); err != nil {
		t.Fatalf("add due task: %v", err)
	}
	task, ok := s.triggers.nextDueScheduledTask("sample", "pm", now)
	if !ok || task == nil || task.ID != "due-wakeup" {
		t.Fatalf("expected due scheduled task, got ok=%v task=%#v", ok, task)
	}

	hb, configured := s.triggers.heartbeatForTrigger("sample", "pm")
	if !configured || hb == nil {
		t.Fatalf("expected heartbeat config")
	}
	if hb.HasTrigger(entity.TriggerOnTask) {
		t.Fatalf("test setup should not include task trigger: %#v", hb.Triggers)
	}
}
