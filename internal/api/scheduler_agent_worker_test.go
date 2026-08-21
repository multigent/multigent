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

func TestRuntimeSchedulerTargetsIncludeAgentWorkerMemberships(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	worker := controldb.AgentWorker{
		ID:          "aw-reviewer",
		WorkspaceID: workspaceID,
		Name:        "reviewer",
		DisplayName: "Reviewer",
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
		t.Fatalf("worker: %v", err)
	}
	if err := s.controlDB.UpsertProjectMembership(controldb.ProjectMembership{
		ID:               "pm-reviewer",
		WorkspaceID:      workspaceID,
		ProjectID:        "sample",
		MemberType:       "agent_worker",
		MemberID:         worker.ID,
		Role:             "qa",
		Title:            "reviewer",
		AutoPickTasks:    true,
		AttentionEnabled: true,
		PriorityWeight:   1,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("membership: %v", err)
	}
	targets := s.runtimeSchedulerTargets(workspaceID, "sample", "")
	if !hasSchedulerTarget(targets, "sample", "reviewer") {
		t.Fatalf("missing worker membership target: %#v", targets)
	}
	if hasSchedulerTarget(targets, "sample", "pm") || hasSchedulerTarget(targets, "sample", "backend") {
		t.Fatalf("migrated project should not include legacy targets: %#v", targets)
	}
}

func TestRuntimeSchedulerTargetsDeduplicateMembershipAndLegacyAgent(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	worker := controldb.AgentWorker{
		ID:          "aw-pm",
		WorkspaceID: workspaceID,
		Name:        "pm",
		DisplayName: "PM",
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
		t.Fatalf("worker: %v", err)
	}
	if err := s.controlDB.UpsertProjectMembership(controldb.ProjectMembership{
		ID:               "pm-pm",
		WorkspaceID:      workspaceID,
		ProjectID:        "sample",
		MemberType:       "agent_worker",
		MemberID:         worker.ID,
		Role:             "pm",
		Title:            "pm",
		AutoPickTasks:    true,
		AttentionEnabled: true,
		PriorityWeight:   1,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("membership: %v", err)
	}
	targets := s.runtimeSchedulerTargets(workspaceID, "sample", "")
	count := 0
	for _, target := range targets {
		if target.project == "sample" && target.agent == "pm" {
			count++
			if target.workerID != worker.ID || target.membershipID != "pm-pm" {
				t.Fatalf("expected membership target to win, got %#v", target)
			}
		}
	}
	if count != 1 {
		t.Fatalf("expected one pm target, got %d in %#v", count, targets)
	}
}

func TestRuntimeSchedulerTargetsGroupAgentWorkerMemberships(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	worker := controldb.AgentWorker{
		ID:          "aw-cross-project",
		WorkspaceID: workspaceID,
		Name:        "nova",
		DisplayName: "Nova",
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
		t.Fatalf("worker: %v", err)
	}
	for _, project := range []string{"alpha", "beta"} {
		if err := s.controlDB.UpsertProjectMembership(controldb.ProjectMembership{
			ID:               "pm-" + project,
			WorkspaceID:      workspaceID,
			ProjectID:        project,
			MemberType:       "agent_worker",
			MemberID:         worker.ID,
			Role:             "manager",
			Title:            "nova",
			AutoPickTasks:    true,
			AttentionEnabled: true,
			PriorityWeight:   1,
			CreatedAt:        now,
			UpdatedAt:        now,
		}); err != nil {
			t.Fatalf("membership %s: %v", project, err)
		}
	}

	targets := s.runtimeSchedulerTargets(workspaceID, "alpha", "")
	count := 0
	for _, target := range targets {
		if target.workerID == worker.ID {
			count++
			if len(target.memberships) != 2 {
				t.Fatalf("expected two memberships on one worker target, got %#v", target)
			}
		}
	}
	if count != 1 {
		t.Fatalf("expected one scheduler target for worker, got %d in %#v", count, targets)
	}
}

func TestRuntimeSchedulerSelectsHighestPriorityMembershipTask(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339)
	worker := controldb.AgentWorker{
		ID:          "aw-priority",
		WorkspaceID: workspaceID,
		Name:        "nova",
		DisplayName: "Nova",
		Status:      "active",
		CreatedAt:   nowText,
		UpdatedAt:   nowText,
	}
	if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
		t.Fatalf("worker: %v", err)
	}
	for _, project := range []string{"alpha", "beta"} {
		if err := s.controlDB.UpsertProjectMembership(controldb.ProjectMembership{
			ID:               "pm-priority-" + project,
			WorkspaceID:      workspaceID,
			ProjectID:        project,
			MemberType:       "agent_worker",
			MemberID:         worker.ID,
			Role:             "manager",
			Title:            "nova",
			AutoPickTasks:    true,
			AttentionEnabled: true,
			PriorityWeight:   1,
			CreatedAt:        nowText,
			UpdatedAt:        nowText,
		}); err != nil {
			t.Fatalf("membership %s: %v", project, err)
		}
	}
	if err := s.ts.AddTask("alpha", "nova", &entity.Task{
		ID:        "t-alpha",
		Title:     "Alpha low priority",
		Status:    entity.TaskStatusPending,
		Priority:  3,
		CreatedAt: now.Add(-time.Hour),
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("alpha task: %v", err)
	}
	if err := s.ts.AddTask("beta", "nova", &entity.Task{
		ID:        "t-beta",
		Title:     "Beta high priority",
		Status:    entity.TaskStatusPending,
		Priority:  1,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("beta task: %v", err)
	}

	target := s.runtimeSchedulerTargetGroupForProjectAgent(workspaceID, "alpha", "nova")
	selected := s.selectRuntimeSchedulerExecutionTarget(target)
	if selected.project != "beta" || selected.agent != "nova" {
		t.Fatalf("expected beta/nova, got %#v", selected)
	}
}

func TestSchedulerProcessKeyUsesAgentWorkerForRuntimeNode(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	worker := controldb.AgentWorker{
		ID:          "aw-scheduler-key",
		WorkspaceID: workspaceID,
		Name:        "nova",
		DisplayName: "Nova",
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
		t.Fatalf("worker: %v", err)
	}
	if err := s.controlDB.UpsertProjectMembership(controldb.ProjectMembership{
		ID:               "pm-scheduler-key",
		WorkspaceID:      workspaceID,
		ProjectID:        "alpha",
		MemberType:       "agent_worker",
		MemberID:         worker.ID,
		Role:             "manager",
		Title:            "nova",
		AutoPickTasks:    true,
		AttentionEnabled: true,
		PriorityWeight:   1,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("membership: %v", err)
	}

	if key := s.schedulerProcessKeyForProjectAgent(workspaceID, "alpha", "nova", schedulerModeRuntimeNode); key != "worker/"+worker.ID {
		t.Fatalf("expected worker scheduler key, got %q", key)
	}
	if key := s.schedulerProcessKeyForProjectAgent(workspaceID, "alpha", "legacy", schedulerModeRuntimeNode); key != "alpha/legacy" {
		t.Fatalf("expected legacy scheduler key, got %q", key)
	}
	if key := s.schedulerProcessKeyForProjectAgent(workspaceID, "alpha", "nova", schedulerModeLocal); key != "alpha/nova" {
		t.Fatalf("expected local scheduler key to remain legacy, got %q", key)
	}
}

func TestSchedulerManagerStartLoopWithKeyDeduplicatesWorker(t *testing.T) {
	m := newSchedulerManager(t.TempDir())
	defer m.Cleanup()

	loop := func(ctx context.Context) {
		<-ctx.Done()
	}
	if err := m.StartLoopWithKey("worker/aw-one", "alpha", "nova", schedulerModeRuntimeNode, loop); err != nil {
		t.Fatalf("start first loop: %v", err)
	}
	if err := m.StartLoopWithKey("worker/aw-one", "beta", "nova", schedulerModeRuntimeNode, loop); err == nil {
		t.Fatalf("expected duplicate worker scheduler key to conflict")
	}
	if err := m.StopKey("worker/aw-one"); err != nil {
		t.Fatalf("stop worker loop: %v", err)
	}
}

func hasSchedulerTarget(targets []runtimeSchedulerAgentTarget, project, agent string) bool {
	for _, target := range targets {
		if target.project == project && target.agent == agent {
			return true
		}
	}
	return false
}

func TestSchedulerTargetHeartbeatUsesAgentWorkerSchedule(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	nowText := time.Now().UTC().Format(time.RFC3339)
	schedule := entity.HeartbeatConfig{Enabled: true, Interval: "15m"}
	scheduleJSON, err := json.Marshal(schedule)
	if err != nil {
		t.Fatalf("marshal schedule: %v", err)
	}
	worker := controldb.AgentWorker{
		ID:               "aw-schedule",
		WorkspaceID:      workspaceID,
		Name:             "planner",
		DisplayName:      "Planner",
		Status:           "active",
		ScheduleJSON:     string(scheduleJSON),
		PrimarySessionID: "sess-worker-primary",
		CreatedAt:        nowText,
		UpdatedAt:        nowText,
	}
	if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
		t.Fatalf("worker: %v", err)
	}
	if err := s.controlDB.UpsertProjectMembership(controldb.ProjectMembership{
		ID:               "pm-schedule",
		WorkspaceID:      workspaceID,
		ProjectID:        "sample",
		MemberType:       "agent_worker",
		MemberID:         worker.ID,
		Role:             "pm",
		Title:            "pm",
		AutoPickTasks:    true,
		AttentionEnabled: true,
		PriorityWeight:   1,
		CreatedAt:        nowText,
		UpdatedAt:        nowText,
	}); err != nil {
		t.Fatalf("membership: %v", err)
	}
	legacySchedule, _ := json.Marshal(entity.HeartbeatConfig{Enabled: false, Interval: "99h"})
	if err := s.controlDB.UpsertRecord("heartbeat", workspaceID, []string{"sample", "pm"}, string(legacySchedule)); err != nil {
		t.Fatalf("legacy heartbeat record: %v", err)
	}
	target := s.runtimeSchedulerTargetForProjectAgent(workspaceID, "sample", "pm")
	hb, err := s.loadSchedulerTargetHeartbeat(workspaceID, target)
	if err != nil {
		t.Fatalf("load heartbeat: %v", err)
	}
	if !hb.Enabled || hb.Interval != "15m" || hb.SessionID != "" {
		t.Fatalf("expected worker schedule, got %#v", hb)
	}
	now := time.Now().UTC()
	hb.LastWakeup = &now
	hb.LastWakeupStatus = "running"
	if err := s.saveSchedulerTargetHeartbeat(workspaceID, target, hb); err != nil {
		t.Fatalf("save heartbeat: %v", err)
	}
	updated, ok, err := s.controlDB.AgentWorkerByID(workspaceID, worker.ID)
	if err != nil || !ok {
		t.Fatalf("load worker: ok=%v err=%v", ok, err)
	}
	var updatedSchedule entity.HeartbeatConfig
	if err := json.Unmarshal([]byte(updated.ScheduleJSON), &updatedSchedule); err != nil {
		t.Fatalf("parse saved schedule: %v", err)
	}
	if updatedSchedule.LastWakeupStatus != "running" || updatedSchedule.LastWakeup == nil {
		t.Fatalf("worker schedule was not updated: %#v", updatedSchedule)
	}
}

func TestProjectScheduleAgentsUseMembershipAliases(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	nowText := time.Now().UTC().Format(time.RFC3339)
	worker := controldb.AgentWorker{
		ID:          "aw-project-schedule",
		WorkspaceID: workspaceID,
		Name:        "global-planner",
		DisplayName: "Global Planner",
		Status:      "active",
		CreatedAt:   nowText,
		UpdatedAt:   nowText,
	}
	if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
		t.Fatalf("worker: %v", err)
	}
	if err := s.controlDB.UpsertProjectMembership(controldb.ProjectMembership{
		ID:               "pm-project-schedule",
		WorkspaceID:      workspaceID,
		ProjectID:        "sample",
		MemberType:       "agent_worker",
		MemberID:         worker.ID,
		Role:             "pm",
		Title:            "项目经理",
		AutoPickTasks:    true,
		AttentionEnabled: true,
		PriorityWeight:   1,
		CreatedAt:        nowText,
		UpdatedAt:        nowText,
	}); err != nil {
		t.Fatalf("membership: %v", err)
	}

	agents, err := s.projectScheduleAgents(workspaceID, "sample")
	if err != nil {
		t.Fatalf("schedule agents: %v", err)
	}
	if len(agents) != 1 || agents[0].Name != "项目经理" || agents[0].AgentWorkerID != worker.ID || agents[0].ProjectMembershipID != "pm-project-schedule" {
		t.Fatalf("unexpected schedule agents: %#v", agents)
	}
}

func TestCreateProjectTaskAnnotatesAgentWorkerAssignee(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	worker := controldb.AgentWorker{
		ID:          "aw-pm-task",
		WorkspaceID: workspaceID,
		Name:        "pm",
		DisplayName: "PM",
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
		t.Fatalf("worker: %v", err)
	}
	if err := s.controlDB.UpsertProjectMembership(controldb.ProjectMembership{
		ID:               "pm-pm-task",
		WorkspaceID:      workspaceID,
		ProjectID:        "sample",
		MemberType:       "agent_worker",
		MemberID:         worker.ID,
		Role:             "pm",
		Title:            "pm",
		AutoPickTasks:    true,
		AttentionEnabled: true,
		PriorityWeight:   1,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("membership: %v", err)
	}

	req := providerTestRequest(http.MethodPost, "/api/v1/projects/sample/tasks", "admin", postTaskBody{
		Agent:    "pm",
		Title:    "Plan work",
		Prompt:   "Plan the next step.",
		Type:     string(entity.TaskTypeChore),
		Priority: 2,
	})
	req.SetPathValue("name", "sample")
	rec := httptest.NewRecorder()
	s.handlePostProjectTask(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create task status=%d body=%s", rec.Code, rec.Body.String())
	}
	rows, err := s.ts.ListTasks("sample", "pm", entity.TaskStatusPending)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("tasks=%d", len(rows))
	}
	task := rows[0]
	if task.AssigneeType != "agent_worker" || task.AssigneeID != worker.ID || task.AssigneeMembershipID != "pm-pm-task" {
		t.Fatalf("missing worker assignee fields: %#v", task)
	}
}

func TestWorkflowTaskMoveAnnotatesAgentWorkerAssignee(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	nowText := time.Now().UTC().Format(time.RFC3339)
	worker := controldb.AgentWorker{
		ID:          "aw-backend-flow",
		WorkspaceID: workspaceID,
		Name:        "backend",
		DisplayName: "Backend",
		Status:      "active",
		CreatedAt:   nowText,
		UpdatedAt:   nowText,
	}
	if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
		t.Fatalf("worker: %v", err)
	}
	if err := s.controlDB.UpsertProjectMembership(controldb.ProjectMembership{
		ID:               "pm-backend-flow",
		WorkspaceID:      workspaceID,
		ProjectID:        "sample",
		MemberType:       "agent_worker",
		MemberID:         worker.ID,
		Role:             "backend",
		Title:            "backend",
		AutoPickTasks:    true,
		AttentionEnabled: true,
		PriorityWeight:   1,
		CreatedAt:        nowText,
		UpdatedAt:        nowText,
	}); err != nil {
		t.Fatalf("membership: %v", err)
	}

	now := time.Now().UTC()
	task := &entity.Task{
		ID:        entity.NewTaskID(),
		Title:     "Implement flow",
		Type:      entity.TaskTypeFeature,
		Priority:  2,
		Assignee:  "sample/pm",
		CreatedBy: "admin",
		Status:    entity.TaskStatusInProgress,
		Prompt:    "Do the work.",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.ts.AddTask("sample", "pm", task); err != nil {
		t.Fatalf("add task: %v", err)
	}
	if err := s.moveWorkflowTaskToAgent(workspaceID, "sample", "pm", "backend", task, entity.TaskStatusPending, now.Add(time.Minute)); err != nil {
		t.Fatalf("move task: %v", err)
	}
	moved, err := s.ts.GetTask("sample", "backend", task.ID)
	if err != nil {
		t.Fatalf("get moved task: %v", err)
	}
	if moved.AssigneeType != "agent_worker" || moved.AssigneeID != worker.ID || moved.AssigneeMembershipID != "pm-backend-flow" {
		t.Fatalf("missing worker assignee fields after workflow move: %#v", moved)
	}
}
