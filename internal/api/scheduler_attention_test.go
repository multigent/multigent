package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/runtimeexec"
)

func TestRuntimeWakeupTaskIncludesPendingAttentionSignals(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedTaskAttentionWorker(t, s, workspaceID, "sample", "pm", true)
	if err := s.controlDB.UpsertAttentionSignal(controldb.AttentionSignal{
		ID:            "sig-runtime-wakeup",
		WorkspaceID:   workspaceID,
		AgentWorkerID: "aw-pm",
		DedupeKey:     "test:runtime-wakeup",
		SourceKind:    "task",
		SourceID:      "task-1",
		Reason:        "task_assigned",
		Priority:      "high",
		Summary:       "Review the new customer issue",
		Status:        "pending",
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("upsert attention: %v", err)
	}

	task, ids, err := s.nextRuntimeWakeupTask(workspaceID, "sample", "pm", &entity.HeartbeatConfig{
		WakeupPrompt: "Check your work queue.",
	})
	if err != nil {
		t.Fatalf("next wakeup task: %v", err)
	}
	if task == nil {
		t.Fatal("expected wakeup task")
	}
	if len(ids) != 1 || ids[0] != "sig-runtime-wakeup" {
		t.Fatalf("unexpected attention ids: %+v", ids)
	}
	if task.CreatedBy != attentionWakeupTaskCreatedBy || task.Priority != 0 {
		t.Fatalf("expected high-priority attention wakeup task, got %+v", task)
	}
	if !strings.Contains(task.Prompt, "Attention Signals") ||
		!strings.Contains(task.Prompt, "sig-runtime-wakeup") ||
		!strings.Contains(task.Prompt, "mga attention list") ||
		!strings.Contains(task.Prompt, "mga notify send --to source") ||
		!strings.Contains(task.Prompt, "mga contacts list") ||
		!strings.Contains(task.Prompt, "mga runtime channels") ||
		!strings.Contains(task.Prompt, "mga notify send --to chat") ||
		!strings.Contains(task.Prompt, "mga inbox send") {
		t.Fatalf("wakeup prompt did not include attention guidance:\n%s", task.Prompt)
	}
	if got, err := s.ts.GetTask("sample", "pm", task.ID); err != nil || got == nil {
		t.Fatalf("wakeup task should be persisted before enqueue, task=%+v err=%v", got, err)
	}
	if task.Vars["MULTIGENT_ATTENTION_SIGNAL_IDS_JSON"] != `["sig-runtime-wakeup"]` {
		t.Fatalf("wakeup task should retain its signal ids, vars=%v", task.Vars)
	}
}

func TestAttentionWakeupDoesNotInjectDomainSpecificContract(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedTaskAttentionWorker(t, s, workspaceID, "sample", "pm", true)
	if err := s.controlDB.UpsertAttentionSignal(controldb.AttentionSignal{
		ID:            "sig-normal-attention",
		WorkspaceID:   workspaceID,
		AgentWorkerID: "aw-pm",
		DedupeKey:     "test:normal-attention",
		SourceKind:    "task",
		SourceID:      "task-normal-attention",
		Reason:        "task_assigned",
		Summary:       "Continue the assigned task",
		Status:        "pending",
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("upsert normal signal: %v", err)
	}
	task, _, err := s.nextRuntimeWakeupTask(workspaceID, "sample", "pm", &entity.HeartbeatConfig{})
	if err != nil {
		t.Fatalf("next wakeup task: %v", err)
	}
	if task == nil {
		t.Fatal("expected wakeup task")
	}
	for _, forbidden := range []string{"Active Operating Contract", "PR Review workflow", "not a reviewer"} {
		if strings.Contains(task.Prompt, forbidden) {
			t.Fatalf("attention wakeup should not inject domain contract %q:\n%s", forbidden, task.Prompt)
		}
	}
}

func TestRuntimeWakeupTaskDoesNotReincludeSeenAttentionSignals(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedTaskAttentionWorker(t, s, workspaceID, "sample", "pm", true)
	if err := s.controlDB.UpsertAttentionSignal(controldb.AttentionSignal{
		ID:            "sig-seen-but-open",
		WorkspaceID:   workspaceID,
		AgentWorkerID: "aw-pm",
		DedupeKey:     "test:seen-open",
		SourceKind:    "im_message",
		SourceID:      "msg-seen-open",
		Reason:        "im_mention",
		Priority:      "high",
		Summary:       "This signal was seen by a failed run but still needs handling.",
		Status:        "seen",
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("upsert attention: %v", err)
	}

	task, ids, err := s.nextRuntimeWakeupTask(workspaceID, "sample", "pm", &entity.HeartbeatConfig{
		WakeupPrompt: "Check your work queue.",
	})
	if err != nil {
		t.Fatalf("next wakeup task: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("seen signal should not be selected: task=%+v ids=%+v", task, ids)
	}
	if task != nil && strings.Contains(task.Prompt, "sig-seen-but-open") {
		t.Fatalf("seen signal should not be injected into wakeup prompt:\n%s", task.Prompt)
	}
}

func TestAttentionWakeupTaskCanFocusTriggeredSignal(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedTaskAttentionWorker(t, s, workspaceID, "sample", "pm", true)
	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.controlDB.UpsertAttentionSignal(controldb.AttentionSignal{
		ID:            "sig-old-task",
		WorkspaceID:   workspaceID,
		AgentWorkerID: "aw-pm",
		DedupeKey:     "test:old-task",
		SourceKind:    "task",
		SourceID:      "task-old",
		Reason:        "task_assigned",
		Priority:      "high",
		Summary:       "Old task signal that should not distract this IM wakeup",
		Status:        "pending",
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("upsert old attention: %v", err)
	}
	if err := s.controlDB.UpsertAttentionSignal(controldb.AttentionSignal{
		ID:            "sig-new-im",
		WorkspaceID:   workspaceID,
		AgentWorkerID: "aw-pm",
		DedupeKey:     "test:new-im",
		SourceKind:    "im_message",
		SourceID:      "om_new",
		Reason:        "im_mention",
		Priority:      "high",
		Summary:       "user-b asked a direct question in the group",
		Status:        "pending",
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("upsert new attention: %v", err)
	}
	if err := s.controlDB.UpsertAttentionSignal(controldb.AttentionSignal{
		ID:            "sig-new-im-2",
		WorkspaceID:   workspaceID,
		AgentWorkerID: "aw-pm",
		DedupeKey:     "test:new-im-2",
		SourceKind:    "im_message",
		SourceID:      "om_new_2",
		Reason:        "im_mention",
		Priority:      "high",
		Summary:       "owner-a asked a second question in the group",
		Status:        "pending",
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("upsert second im attention: %v", err)
	}
	if err := s.controlDB.UpsertAttentionSignal(controldb.AttentionSignal{
		ID:            "sig-seen-im",
		WorkspaceID:   workspaceID,
		AgentWorkerID: "aw-pm",
		DedupeKey:     "test:seen-im",
		SourceKind:    "im_message",
		SourceID:      "om_seen",
		Reason:        "im_mention",
		Priority:      "high",
		Summary:       "Already seen IM should not be pulled into a fresh focused wakeup",
		Status:        "seen",
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("upsert seen im attention: %v", err)
	}

	task, ids, err := s.ensurePendingAttentionWakeupTask(workspaceID, "sample", "pm", "sig-new-im")
	if err != nil {
		t.Fatalf("ensure attention task: %v", err)
	}
	if task == nil {
		t.Fatal("expected attention task")
	}
	if len(ids) != 2 || ids[0] != "sig-new-im" || ids[1] != "sig-new-im-2" {
		t.Fatalf("unexpected focused ids with open im aggregation: %+v", ids)
	}
	if !strings.Contains(task.Prompt, "sig-new-im") || !strings.Contains(task.Prompt, "user-b asked") {
		t.Fatalf("focused prompt missing new signal:\n%s", task.Prompt)
	}
	if !strings.Contains(task.Prompt, "sig-new-im-2") || !strings.Contains(task.Prompt, "owner-a asked") {
		t.Fatalf("focused prompt missing second open im signal:\n%s", task.Prompt)
	}
	if strings.Contains(task.Prompt, "sig-old-task") || strings.Contains(task.Prompt, "Old task signal") {
		t.Fatalf("focused prompt should not include unrelated old signal:\n%s", task.Prompt)
	}
	if strings.Contains(task.Prompt, "sig-seen-im") || strings.Contains(task.Prompt, "Already seen IM") {
		t.Fatalf("focused prompt should not include already seen im signal:\n%s", task.Prompt)
	}
}

func TestAttentionWakeupTaskFocusFindsNewSignalBehindOldBacklog(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedTaskAttentionWorker(t, s, workspaceID, "sample", "pm", true)
	base := time.Date(2026, 8, 22, 9, 0, 0, 0, time.UTC)
	for i := 0; i < 25; i++ {
		if err := s.controlDB.UpsertAttentionSignal(controldb.AttentionSignal{
			ID:            fmt.Sprintf("sig-old-task-%02d", i),
			WorkspaceID:   workspaceID,
			AgentWorkerID: "aw-pm",
			DedupeKey:     fmt.Sprintf("test:old-task:%02d", i),
			SourceKind:    "task",
			SourceID:      fmt.Sprintf("task-old-%02d", i),
			Reason:        "task_assigned",
			Priority:      "normal",
			Summary:       "Old task backlog",
			Status:        "pending",
			CreatedAt:     base.Add(time.Duration(i) * time.Minute).Format(time.RFC3339),
		}); err != nil {
			t.Fatalf("upsert old attention %d: %v", i, err)
		}
	}
	if err := s.controlDB.UpsertAttentionSignal(controldb.AttentionSignal{
		ID:            "sig-new-im-behind-backlog",
		WorkspaceID:   workspaceID,
		AgentWorkerID: "aw-pm",
		DedupeKey:     "test:new-im-behind-backlog",
		SourceKind:    "im_message",
		SourceID:      "om_new_behind_backlog",
		Reason:        "im_mention",
		Priority:      "high",
		Summary:       "Fresh IM should be focused even when old backlog is longer than 20 signals.",
		Status:        "pending",
		CreatedAt:     base.Add(30 * time.Minute).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("upsert new attention: %v", err)
	}

	task, ids, err := s.ensurePendingAttentionWakeupTask(workspaceID, "sample", "pm", "sig-new-im-behind-backlog")
	if err != nil {
		t.Fatalf("ensure attention task: %v", err)
	}
	if task == nil {
		t.Fatal("expected focused attention task")
	}
	if len(ids) != 1 || ids[0] != "sig-new-im-behind-backlog" {
		t.Fatalf("unexpected focused ids: %+v", ids)
	}
	if !strings.Contains(task.Prompt, "sig-new-im-behind-backlog") {
		t.Fatalf("focused prompt missed new IM behind backlog:\n%s", task.Prompt)
	}
	if strings.Contains(task.Prompt, "sig-old-task-00") {
		t.Fatalf("focused prompt should not include unrelated old task backlog:\n%s", task.Prompt)
	}
}

func TestAttentionWakeupTaskTakesPriorityOverNormalPendingTask(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedTaskAttentionWorker(t, s, workspaceID, "sample", "pm", true)
	pending := &entity.Task{
		ID:        "task-pending",
		Title:     "Existing pending task",
		Prompt:    "Do the specific pending task.",
		Status:    entity.TaskStatusPending,
		Priority:  2,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := s.ts.AddTask("sample", "pm", pending); err != nil {
		t.Fatalf("add pending task: %v", err)
	}
	if err := s.controlDB.UpsertAttentionSignal(controldb.AttentionSignal{
		ID:            "sig-should-not-inject",
		WorkspaceID:   workspaceID,
		AgentWorkerID: "aw-pm",
		DedupeKey:     "test:pending-task",
		SourceKind:    "task",
		SourceID:      "task-2",
		Reason:        "task_assigned",
		Summary:       "This should wait for idle wakeup.",
		Status:        "pending",
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("upsert attention: %v", err)
	}

	task, ids, err := s.nextRuntimeWakeupTask(workspaceID, "sample", "pm", &entity.HeartbeatConfig{
		WakeupPrompt: "Check your work queue.",
	})
	if err != nil {
		t.Fatalf("next wakeup task: %v", err)
	}
	if task == nil {
		t.Fatal("expected wakeup task")
	}
	if task.ID == pending.ID {
		t.Fatalf("attention wakeup should not be hidden behind normal pending task: %+v", task)
	}
	if task.CreatedBy != attentionWakeupTaskCreatedBy || task.Priority != 0 || task.Type != "wakeup" {
		t.Fatalf("unexpected attention task metadata: %+v", task)
	}
	if len(ids) != 1 || ids[0] != "sig-should-not-inject" {
		t.Fatalf("unexpected attention ids: %+v", ids)
	}
	if !strings.Contains(task.Prompt, "Attention Signals") ||
		!strings.Contains(task.Prompt, "sig-should-not-inject") ||
		!strings.Contains(task.Prompt, "mga attention list") ||
		!strings.Contains(task.Prompt, "mga notify send --to source") ||
		!strings.Contains(task.Prompt, "mga contacts list") ||
		!strings.Contains(task.Prompt, "mga runtime channels") ||
		!strings.Contains(task.Prompt, "mga notify send --to chat") ||
		!strings.Contains(task.Prompt, "mga inbox send") {
		t.Fatalf("attention task prompt should include attention guidance:\n%s", task.Prompt)
	}
}

func TestRecoverablePendingAttentionWakeupTargetsGroupsPendingSignals(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedTaskAttentionWorker(t, s, workspaceID, "sample", "pm", true)
	now := time.Now().UTC().Format(time.RFC3339)
	refs := `{"project":"sample","agent":"pm","chatType":"group"}`
	cases := []controldb.AttentionSignal{
		{
			ID:            "sig-recover-im-1",
			WorkspaceID:   workspaceID,
			AgentWorkerID: "aw-pm",
			DedupeKey:     "recover:im:1",
			SourceKind:    "im_message",
			SourceID:      "om_recover_1",
			Reason:        "im_mention",
			Status:        "pending",
			RefsJSON:      refs,
			CreatedAt:     now,
		},
		{
			ID:            "sig-recover-card",
			WorkspaceID:   workspaceID,
			AgentWorkerID: "aw-pm",
			DedupeKey:     "recover:card",
			SourceKind:    "im_card_action",
			SourceID:      "ir_recover",
			Reason:        "card_action",
			Status:        "pending",
			RefsJSON:      refs,
			CreatedAt:     now,
		},
		{
			ID:            "sig-recover-message",
			WorkspaceID:   workspaceID,
			AgentWorkerID: "aw-pm",
			DedupeKey:     "recover:message",
			SourceKind:    "message",
			SourceID:      "msg-recover",
			Reason:        "inbox_message",
			Status:        "pending",
			RefsJSON:      refs,
			CreatedAt:     now,
		},
		{
			ID:            "sig-recover-task",
			WorkspaceID:   workspaceID,
			AgentWorkerID: "aw-pm",
			DedupeKey:     "recover:task",
			SourceKind:    "task",
			Reason:        "task_assigned",
			Status:        "pending",
			RefsJSON:      refs,
			CreatedAt:     now,
		},
		{
			ID:            "sig-recover-seen",
			WorkspaceID:   workspaceID,
			AgentWorkerID: "aw-pm",
			DedupeKey:     "recover:seen",
			SourceKind:    "im_message",
			Reason:        "im_mention",
			Status:        "seen",
			RefsJSON:      refs,
			CreatedAt:     now,
		},
		{
			ID:            "sig-recover-no-refs",
			WorkspaceID:   workspaceID,
			AgentWorkerID: "aw-pm",
			DedupeKey:     "recover:no-refs",
			SourceKind:    "im_message",
			Reason:        "im_mention",
			Status:        "pending",
			RefsJSON:      `{}`,
			CreatedAt:     now,
		},
	}
	for _, signal := range cases {
		if err := s.controlDB.UpsertAttentionSignal(signal); err != nil {
			t.Fatalf("upsert %s: %v", signal.ID, err)
		}
	}

	targets, err := s.recoverablePendingAttentionWakeupTargets(100)
	if err != nil {
		t.Fatalf("recoverable targets: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected one grouped target, got %+v", targets)
	}
	target := targets[0]
	if target.WorkspaceID != workspaceID || target.ProjectID != "sample" || target.AgentID != "pm" || target.AgentWorkerID != "aw-pm" {
		t.Fatalf("unexpected target: %+v", target)
	}
	if strings.Join(target.AttentionIDs, ",") != "sig-recover-message,sig-recover-task" {
		t.Fatalf("unexpected recovered ids: %+v", target.AttentionIDs)
	}
}

func TestEnsurePendingAttentionWakeupTaskDedupesExistingTask(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedTaskAttentionWorker(t, s, workspaceID, "sample", "pm", true)
	now := time.Now().UTC()
	existing := &entity.Task{
		ID:        "attention-existing",
		Title:     attentionWakeupTaskTitle,
		Type:      "wakeup",
		Status:    entity.TaskStatusPending,
		Priority:  0,
		Prompt:    "existing attention prompt",
		CreatedBy: attentionWakeupTaskCreatedBy,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.ts.AddTask("sample", "pm", existing); err != nil {
		t.Fatalf("add existing attention task: %v", err)
	}
	if err := s.controlDB.UpsertAttentionSignal(controldb.AttentionSignal{
		ID:            "sig-dedupe",
		WorkspaceID:   workspaceID,
		AgentWorkerID: "aw-pm",
		DedupeKey:     "test:dedupe",
		SourceKind:    "im",
		Reason:        "im_direct_message",
		Summary:       "Please look at this.",
		Status:        "pending",
		CreatedAt:     now.Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("upsert attention: %v", err)
	}

	task, ids, err := s.ensurePendingAttentionWakeupTask(workspaceID, "sample", "pm")
	if err != nil {
		t.Fatalf("ensure attention task: %v", err)
	}
	if task == nil || task.ID != existing.ID {
		t.Fatalf("expected existing attention task, got %+v", task)
	}
	if len(ids) != 1 || ids[0] != "sig-dedupe" {
		t.Fatalf("unexpected ids: %+v", ids)
	}
	tasks, err := s.ts.ListTasks("sample", "pm", entity.TaskStatusPending)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	count := 0
	for _, task := range tasks {
		if task != nil && task.CreatedBy == attentionWakeupTaskCreatedBy {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one attention task, got %d", count)
	}
}

func TestEnsurePendingAttentionWakeupTaskDoesNotReuseInProgressTask(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedTaskAttentionWorker(t, s, workspaceID, "sample", "pm", true)
	now := time.Now().UTC()
	stale := &entity.Task{
		ID:        "attention-in-progress",
		Title:     attentionWakeupTaskTitle,
		Type:      "wakeup",
		Status:    entity.TaskStatusInProgress,
		Priority:  0,
		Prompt:    "stale running attention prompt",
		CreatedBy: attentionWakeupTaskCreatedBy,
		CreatedAt: now.Add(-time.Hour),
		UpdatedAt: now.Add(-time.Hour),
	}
	if err := s.ts.AddTask("sample", "pm", stale); err != nil {
		t.Fatalf("add in-progress attention task: %v", err)
	}
	if err := s.controlDB.UpsertAttentionSignal(controldb.AttentionSignal{
		ID:            "sig-new-after-stale",
		WorkspaceID:   workspaceID,
		AgentWorkerID: "aw-pm",
		DedupeKey:     "test:new-after-stale",
		SourceKind:    "im_message",
		Reason:        "im_mention",
		Summary:       "New message should create a runnable attention task.",
		Status:        "pending",
		CreatedAt:     now.Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("upsert attention: %v", err)
	}

	task, ids, err := s.ensurePendingAttentionWakeupTask(workspaceID, "sample", "pm", "sig-new-after-stale")
	if err != nil {
		t.Fatalf("ensure attention task: %v", err)
	}
	if task == nil || task.ID == stale.ID || task.Status != entity.TaskStatusPending {
		t.Fatalf("expected new pending attention task, got %+v", task)
	}
	if len(ids) != 1 || ids[0] != "sig-new-after-stale" {
		t.Fatalf("unexpected ids: %+v", ids)
	}
}

func TestRuntimeWakeupRunMarksAttentionSeenAfterEnqueue(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedTaskAttentionWorker(t, s, workspaceID, "sample", "pm", true)
	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.controlDB.UpsertRuntimeNode(controldb.RuntimeNode{
		ID:               "rnode-wakeup",
		WorkspaceID:      workspaceID,
		Name:             "local wakeup node",
		Kind:             "local",
		Status:           "online",
		LastSeenAt:       now,
		CapabilitiesJSON: `{"agents":["codex"]}`,
		PolicyJSON:       `{}`,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("runtime node: %v", err)
	}
	worker, found, err := s.controlDB.AgentWorkerByID(workspaceID, "aw-pm")
	if err != nil || !found {
		t.Fatalf("worker lookup: found=%v err=%v", found, err)
	}
	worker.Model = string(entity.ModelCodex)
	worker.DefaultRuntimeNodeID = "rnode-wakeup"
	if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
		t.Fatalf("update worker runtime node: %v", err)
	}
	if err := s.controlDB.UpsertAttentionSignal(controldb.AttentionSignal{
		ID:            "sig-enqueue-seen",
		WorkspaceID:   workspaceID,
		AgentWorkerID: "aw-pm",
		DedupeKey:     "test:enqueue-seen",
		SourceKind:    "task",
		SourceID:      "task-3",
		Reason:        "task_assigned",
		Summary:       "Include this in the queued wakeup run.",
		Status:        "pending",
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("upsert attention: %v", err)
	}
	hb := &entity.HeartbeatConfig{
		WakeupPrompt: "Check pending attention and decide what to do.",
		SessionID:    "sess-wakeup",
	}

	run, task, err := s.enqueueRuntimeWakeupRunFromRequest(workspaceID, "sample", "pm", hb, "http://127.0.0.1:27893", "admin")
	if err != nil {
		t.Fatalf("enqueue wakeup run: %v", err)
	}
	if run.ID == "" || run.Status != "queued" || run.AgentWorkerID != "aw-pm" || run.ProjectMembershipID != "pm-sample-pm" || run.DesiredRuntimeNodeID != "rnode-wakeup" {
		t.Fatalf("unexpected runtime run: %#v", run)
	}
	if task == nil || task.Status != entity.TaskStatusInProgress || !strings.Contains(task.Prompt, "sig-enqueue-seen") {
		t.Fatalf("unexpected wakeup task: %#v", task)
	}
	updated, found, err := s.controlDB.AttentionSignalByID(workspaceID, "sig-enqueue-seen")
	if err != nil || !found {
		t.Fatalf("signal lookup: found=%v err=%v", found, err)
	}
	if updated.Status != "seen" || strings.TrimSpace(updated.SeenAt) == "" {
		t.Fatalf("signal should be marked seen after enqueue succeeds: %#v", updated)
	}
}

func TestRuntimeWakeupRunInjectsDelegationEnvForCardAction(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedTaskAttentionWorker(t, s, workspaceID, "sample", "pm", true)
	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.controlDB.UpsertRuntimeNode(controldb.RuntimeNode{
		ID:               "rnode-card-action",
		WorkspaceID:      workspaceID,
		Name:             "local card node",
		Kind:             "local",
		Status:           "online",
		LastSeenAt:       now,
		CapabilitiesJSON: `{"agents":["codex"]}`,
		PolicyJSON:       `{}`,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("runtime node: %v", err)
	}
	worker, found, err := s.controlDB.AgentWorkerByID(workspaceID, "aw-pm")
	if err != nil || !found {
		t.Fatalf("worker lookup: found=%v err=%v", found, err)
	}
	worker.Model = string(entity.ModelCodex)
	worker.DefaultRuntimeNodeID = "rnode-card-action"
	if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
		t.Fatalf("update worker runtime node: %v", err)
	}
	if err := s.controlDB.CreateInteractionRequest(controldb.InteractionRequest{
		ID:             "ir-card-action",
		WorkspaceID:    workspaceID,
		AgentWorkerID:  "aw-pm",
		ProjectID:      "sample",
		AgentID:        "pm",
		HandlerType:    "agent_event",
		Status:         "submitted",
		SubmittedBy:    "admin",
		SubmissionJSON: `{"actionId":"approve"}`,
		ContextJSON:    `{"taskId":"task-workflow"}`,
		CreatedAt:      now,
		SubmittedAt:    now,
	}); err != nil {
		t.Fatalf("interaction request: %v", err)
	}
	if err := s.controlDB.UpsertAttentionSignal(controldb.AttentionSignal{
		ID:            "sig-card-action",
		WorkspaceID:   workspaceID,
		AgentWorkerID: "aw-pm",
		DedupeKey:     "test:card-action",
		SourceKind:    "im_card_action",
		SourceID:      "ir-card-action",
		Reason:        "card_action",
		Summary:       "批准",
		Status:        "pending",
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("upsert attention: %v", err)
	}

	run, task, err := s.enqueueRuntimeWakeupRunFromRequest(workspaceID, "sample", "pm", &entity.HeartbeatConfig{
		WakeupPrompt: "Check pending attention and decide what to do.",
	}, "http://127.0.0.1:27893", "admin")
	if err != nil {
		t.Fatalf("enqueue wakeup run: %v", err)
	}
	if task == nil || strings.TrimSpace(task.Vars["MULTIGENT_DELEGATION_TOKEN"]) == "" {
		t.Fatalf("expected delegation vars on attention task: %#v", task)
	}
	if strings.Contains(task.Prompt, task.Vars["MULTIGENT_DELEGATION_TOKEN"]) {
		t.Fatalf("attention prompt leaked delegation token")
	}
	var spec runtimeexec.Spec
	if err := json.Unmarshal([]byte(run.SpecJSON), &spec); err != nil {
		t.Fatalf("decode runtime spec: %v", err)
	}
	if spec.RuntimeControlEnv["MULTIGENT_DELEGATION_TOKEN"] == "" {
		t.Fatalf("runtime spec missing delegation token env: %#v", spec.RuntimeControlEnv)
	}
	if spec.RuntimeControlEnv["MULTIGENT_DELEGATION_INTERACTION_ID"] != "ir-card-action" {
		t.Fatalf("runtime spec missing interaction id env: %#v", spec.RuntimeControlEnv)
	}
}

func TestRuntimeWakeupRunInjectsDelegationEnvMapForMultipleCardActions(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	seedTaskAttentionWorker(t, s, workspaceID, "sample", "pm", true)
	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.controlDB.UpsertRuntimeNode(controldb.RuntimeNode{
		ID:               "rnode-card-actions",
		WorkspaceID:      workspaceID,
		Name:             "card action node",
		Kind:             "local",
		Status:           "online",
		LastSeenAt:       now,
		CapabilitiesJSON: `{"agents":["codex"]}`,
		PolicyJSON:       `{}`,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("runtime node: %v", err)
	}
	worker, found, err := s.controlDB.AgentWorkerByID(workspaceID, "aw-pm")
	if err != nil || !found {
		t.Fatalf("worker lookup: found=%v err=%v", found, err)
	}
	worker.Model = string(entity.ModelCodex)
	worker.DefaultRuntimeNodeID = "rnode-card-actions"
	if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
		t.Fatalf("update worker runtime node: %v", err)
	}
	for _, id := range []string{"ir-card-a", "ir-card-b"} {
		if err := s.controlDB.CreateInteractionRequest(controldb.InteractionRequest{
			ID:             id,
			WorkspaceID:    workspaceID,
			AgentWorkerID:  "aw-pm",
			ProjectID:      "sample",
			AgentID:        "pm",
			HandlerType:    "agent_event",
			Status:         "submitted",
			SubmittedBy:    "admin",
			SubmissionJSON: `{"actionId":"approve"}`,
			ContextJSON:    `{"taskId":"task-workflow"}`,
			CreatedAt:      now,
			SubmittedAt:    now,
		}); err != nil {
			t.Fatalf("interaction request %s: %v", id, err)
		}
		if err := s.controlDB.UpsertAttentionSignal(controldb.AttentionSignal{
			ID:            "sig-" + id,
			WorkspaceID:   workspaceID,
			AgentWorkerID: "aw-pm",
			DedupeKey:     "test:" + id,
			SourceKind:    "im_card_action",
			SourceID:      id,
			Reason:        "card_action",
			Summary:       "批准",
			Status:        "pending",
			CreatedAt:     now,
		}); err != nil {
			t.Fatalf("upsert attention %s: %v", id, err)
		}
	}

	run, task, err := s.enqueueRuntimeWakeupRunFromRequest(workspaceID, "sample", "pm", &entity.HeartbeatConfig{
		WakeupPrompt: "Check pending attention and decide what to do.",
	}, "http://127.0.0.1:27893", "admin")
	if err != nil {
		t.Fatalf("enqueue wakeup run: %v", err)
	}
	if task == nil || strings.TrimSpace(task.Vars["MULTIGENT_DELEGATION_TOKENS_JSON"]) == "" {
		t.Fatalf("expected delegation token map on attention task: %#v", task)
	}
	if strings.Contains(task.Prompt, "MULTIGENT_DELEGATION_TOKENS_JSON") || strings.Contains(task.Prompt, task.Vars["MULTIGENT_DELEGATION_TOKENS_JSON"]) {
		t.Fatalf("attention prompt leaked delegation token map")
	}
	var spec runtimeexec.Spec
	if err := json.Unmarshal([]byte(run.SpecJSON), &spec); err != nil {
		t.Fatalf("decode runtime spec: %v", err)
	}
	var tokens map[string]string
	if err := json.Unmarshal([]byte(spec.RuntimeControlEnv["MULTIGENT_DELEGATION_TOKENS_JSON"]), &tokens); err != nil {
		t.Fatalf("decode delegation token map: %v", err)
	}
	if strings.TrimSpace(tokens["ir-card-a"]) == "" || strings.TrimSpace(tokens["ir-card-b"]) == "" {
		t.Fatalf("runtime spec missing per-interaction delegation tokens: %#v", spec.RuntimeControlEnv)
	}
}
