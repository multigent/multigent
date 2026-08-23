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
	resolved, ok, err := s.agentDirectory.ResolveProjectMailbox(workspaceID, project+"/"+agent)
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
	payload := trustedSystemAttentionPayload(map[string]any{
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
		PayloadJSON:   attentionPayloadJSON(payload),
		Status:        "pending",
		CreatedAt:     now.Format(time.RFC3339),
		ExpiresAt:     now.Add(14 * 24 * time.Hour).Format(time.RFC3339),
	}); err != nil {
		log.Printf("[attention] record task attention failed for %s/%s task=%s: %v", project, agent, task.ID, err)
		return ""
	}
	return signalID
}

func (s *Server) recordMessageAttentionSignal(workspaceID string, msg *entity.Message) string {
	if s == nil || s.controlDB == nil || s.agentDirectory == nil || msg == nil {
		return ""
	}
	workspaceID = strings.TrimSpace(workspaceID)
	recipient := strings.TrimSpace(msg.To)
	if workspaceID == "" || strings.TrimSpace(msg.ID) == "" || recipient == "" {
		return ""
	}
	project, agent, ok := splitAgentMailbox(recipient)
	if !ok {
		return ""
	}
	resolved, ok, err := s.agentDirectory.ResolveProjectMailbox(workspaceID, recipient)
	if err != nil {
		log.Printf("[attention] resolve message recipient failed mailbox=%s message=%s: %v", recipient, msg.ID, err)
		return ""
	}
	if !ok || !resolved.Membership.AttentionEnabled {
		return ""
	}
	refs := map[string]any{
		"messageId":    msg.ID,
		"from":         msg.From,
		"to":           msg.To,
		"project":      project,
		"agent":        agent,
		"membershipId": resolved.Membership.ID,
	}
	payload := trustedSystemAttentionPayload(map[string]any{
		"messageId": msg.ID,
		"from":      msg.From,
		"to":        msg.To,
		"subject":   msg.Subject,
		"body":      msg.Body,
		"replyTo":   msg.ReplyTo,
	})
	refsRaw, _ := json.Marshal(refs)
	now := time.Now().UTC()
	summary := strings.TrimSpace(msg.Subject)
	if summary == "" {
		summary = strings.TrimSpace(msg.Body)
	}
	if summary == "" {
		summary = msg.ID
	}
	signalID := newChannelID("asig")
	if err := s.controlDB.UpsertAttentionSignal(controldb.AttentionSignal{
		ID:            signalID,
		WorkspaceID:   workspaceID,
		AgentWorkerID: resolved.Worker.ID,
		DedupeKey:     "message:" + msg.ID,
		SourceKind:    "message",
		SourceID:      msg.ID,
		SourceChannel: "inbox:" + recipient,
		Reason:        "inbox_message",
		Priority:      "normal",
		ActorType:     "agent",
		ActorID:       strings.TrimSpace(msg.From),
		Summary:       trimForIM(summary, 240),
		RefsJSON:      string(refsRaw),
		PayloadJSON:   attentionPayloadJSON(payload),
		Status:        "pending",
		CreatedAt:     now.Format(time.RFC3339),
		ExpiresAt:     now.Add(14 * 24 * time.Hour).Format(time.RFC3339),
	}); err != nil {
		log.Printf("[attention] record message attention failed mailbox=%s message=%s: %v", recipient, msg.ID, err)
		return ""
	}
	return signalID
}

func (s *Server) requestMessageAttentionWakeup(workspaceID string, msg *entity.Message, attentionID string) {
	if s == nil || s.agentDirectory == nil || msg == nil || strings.TrimSpace(attentionID) == "" {
		return
	}
	workspaceID = strings.TrimSpace(workspaceID)
	recipient := strings.TrimSpace(msg.To)
	project, agent, ok := splitAgentMailbox(recipient)
	if workspaceID == "" || !ok {
		return
	}
	resolved, ok, err := s.agentDirectory.ResolveProjectMailbox(workspaceID, recipient)
	if err != nil {
		log.Printf("[attention] resolve message wakeup recipient failed mailbox=%s message=%s: %v", recipient, msg.ID, err)
		return
	}
	if !ok || !resolved.Membership.AttentionEnabled {
		return
	}
	binding := controldb.AgentChannelBinding{
		WorkspaceID:   workspaceID,
		AgentWorkerID: resolved.Worker.ID,
		ProjectID:     project,
		AgentID:       agent,
	}
	actor := strings.TrimSpace(msg.From)
	if actor == "" {
		actor = "system"
	}
	s.requestAgentAttentionWakeup(binding, "inbox_message", s.runtimeAPIURLForInternalEvent(), actor, attentionID)
}

func (s *Server) requestTaskAttentionWakeup(workspaceID, project, agent string, task *entity.Task, reason, attentionID string) {
	if s == nil || s.agentDirectory == nil || task == nil || strings.TrimSpace(attentionID) == "" {
		return
	}
	workspaceID = strings.TrimSpace(workspaceID)
	project = strings.TrimSpace(project)
	agent = strings.TrimSpace(agent)
	if workspaceID == "" || project == "" || agent == "" {
		return
	}
	resolved, ok, err := s.agentDirectory.ResolveProjectMailbox(workspaceID, project+"/"+agent)
	if err != nil {
		log.Printf("[attention] resolve task wakeup recipient failed for %s/%s task=%s: %v", project, agent, task.ID, err)
		return
	}
	if !ok || !resolved.Membership.AttentionEnabled {
		return
	}
	binding := controldb.AgentChannelBinding{
		WorkspaceID:   workspaceID,
		AgentWorkerID: resolved.Worker.ID,
		ProjectID:     project,
		AgentID:       agent,
	}
	actor := strings.TrimSpace(task.CreatedBy)
	if actor == "" {
		actor = "system"
	}
	s.requestAgentAttentionWakeup(binding, normalizeTaskAttentionReason(reason), s.runtimeAPIURLForInternalEvent(), actor, attentionID)
}

func (s *Server) markTaskAttentionSignalsForRun(run controldb.RuntimeRun, status string) {
	if s == nil || s.controlDB == nil {
		return
	}
	workspaceID := strings.TrimSpace(run.WorkspaceID)
	workerID := strings.TrimSpace(run.AgentWorkerID)
	taskID := strings.TrimSpace(run.TaskID)
	if workspaceID == "" || workerID == "" || taskID == "" {
		return
	}
	signals, err := s.controlDB.ListAttentionSignals(controldb.AttentionSignalFilter{
		WorkspaceID:   workspaceID,
		AgentWorkerID: workerID,
		SourceKind:    "task",
		Statuses:      []string{"pending", "seen", "handling"},
		Limit:         200,
	})
	if err != nil {
		log.Printf("[attention] list runtime task signals failed run=%s task=%s: %v", run.ID, taskID, err)
		return
	}
	for _, signal := range signals {
		if strings.TrimSpace(signal.SourceID) != taskID {
			continue
		}
		reason := normalizeTaskAttentionReason(signal.Reason)
		if reason != "task_assigned" && reason != string(entity.TriggerOnWorkflowStepAssigned) {
			continue
		}
		if err := s.controlDB.MarkAttentionSignalStatus(workspaceID, signal.ID, status); err != nil {
			log.Printf("[attention] mark runtime task signal failed signal=%s run=%s task=%s status=%s: %v", signal.ID, run.ID, taskID, status, err)
		}
	}
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
