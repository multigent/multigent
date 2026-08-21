package api

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	workflowstore "github.com/multigent/multigent/internal/workflow"
)

func (s *Server) recordTaskAttentionSignal(workspaceID, project, agent string, task *entity.Task, reason string) string {
	if s == nil || s.controlDB == nil || s.agentDirectory == nil || task == nil {
		return ""
	}
	workspaceID = strings.TrimSpace(workspaceID)
	project = strings.TrimSpace(project)
	agent = strings.TrimSpace(agent)
	if workspaceID == "" || project == "" || agent == "" || strings.TrimSpace(task.ID) == "" {
		return ""
	}
	resolved, ok, err := s.agentDirectory.ResolveLegacyMailbox(workspaceID, project+"/"+agent)
	if err != nil {
		log.Printf("[attention] resolve agent worker failed for %s/%s task=%s: %v", project, agent, task.ID, err)
		return ""
	}
	if !ok || !resolved.Membership.AttentionEnabled {
		return ""
	}
	reason = normalizeTaskAttentionReason(reason)
	refs := map[string]any{
		"project":      project,
		"agent":        agent,
		"taskId":       task.ID,
		"membershipId": resolved.Membership.ID,
	}
	if run, found := s.taskWorkflowRun(workspaceID, project, task.ID); found {
		refs["workflowRunId"] = run.ID
		refs["workflowId"] = run.DefinitionID
		refs["workflowStepId"] = run.ActiveStepID
		refs["currentAssigneeType"] = run.CurrentAssigneeType
		refs["currentAssigneeId"] = run.CurrentAssigneeID
		refs["currentAssigneeMembershipId"] = run.CurrentAssigneeMembershipID
	}
	payloadRaw, _ := json.Marshal(map[string]any{
		"title":       task.Title,
		"description": task.Description,
		"prompt":      task.Prompt,
		"status":      task.Status,
		"priority":    task.Priority,
		"assignee":    task.Assignee,
		"reason":      reason,
	})
	refsRaw, _ := json.Marshal(refs)
	now := time.Now().UTC()
	summary := strings.TrimSpace(task.Title)
	if summary == "" {
		summary = strings.TrimSpace(task.Description)
	}
	if summary == "" {
		summary = task.ID
	}
	dedupeKey := "task:" + project + ":" + task.ID + ":" + agent + ":" + reason
	signalID := newChannelID("asig")
	if err := s.controlDB.UpsertAttentionSignal(controldb.AttentionSignal{
		ID:            signalID,
		WorkspaceID:   workspaceID,
		AgentWorkerID: resolved.Worker.ID,
		DedupeKey:     dedupeKey,
		SourceKind:    "task",
		SourceID:      task.ID,
		SourceChannel: "project:" + project,
		Reason:        reason,
		Priority:      taskAttentionPriority(task.Priority),
		ActorType:     "system",
		ActorID:       "",
		Summary:       trimForIM(summary, 240),
		RefsJSON:      string(refsRaw),
		PayloadJSON:   string(payloadRaw),
		Status:        "pending",
		CreatedAt:     now.Format(time.RFC3339),
		ExpiresAt:     now.Add(14 * 24 * time.Hour).Format(time.RFC3339),
	}); err != nil {
		log.Printf("[attention] record task attention failed for %s/%s task=%s: %v", project, agent, task.ID, err)
		return ""
	}
	return signalID
}

func (s *Server) taskWorkflowRun(workspaceID, project, taskID string) (entity.WorkflowRun, bool) {
	if s == nil || s.controlDB == nil {
		return entity.WorkflowRun{}, false
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return entity.WorkflowRun{}, false
	}
	store := workflowstore.NewStore(s.controlDB, workspaceID)
	run, found, err := store.RunForTask(project, taskID)
	if err != nil || !found {
		return entity.WorkflowRun{}, false
	}
	if updated, err := store.ReconcileRunCurrentAssignee(run); err == nil && updated.ID != "" {
		return updated, true
	}
	return run, true
}

func normalizeTaskAttentionReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if strings.Contains(reason, "workflow") {
		return string(entity.TriggerOnWorkflowStepAssigned)
	}
	switch reason {
	case string(entity.TriggerOnWorkflowStepAssigned), "task_assigned":
		return reason
	default:
		return "task_assigned"
	}
}

func taskAttentionPriority(priority int) string {
	switch {
	case priority <= 0:
		return "critical"
	case priority == 1:
		return "high"
	case priority >= 3:
		return "low"
	default:
		return "normal"
	}
}
