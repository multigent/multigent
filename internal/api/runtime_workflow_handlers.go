package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/taskstore"
	workflowstore "github.com/multigent/multigent/internal/workflow"
)

type runtimeTaskBody struct {
	Agent            string   `json:"agent"`
	Title            string   `json:"title"`
	Prompt           string   `json:"prompt"`
	Description      string   `json:"description"`
	Type             string   `json:"type"`
	Priority         int      `json:"priority"`
	Assignee         string   `json:"assignee"`
	Labels           []string `json:"labels"`
	ParentID         string   `json:"parentId"`
	DueDate          string   `json:"dueDate"`
	EstimateDuration string   `json:"estimateDuration"`
}

type runtimeTaskUpdateBody struct {
	Agent            string    `json:"agent"`
	Title            *string   `json:"title,omitempty"`
	Description      *string   `json:"description,omitempty"`
	Status           *string   `json:"status,omitempty"`
	Priority         *int      `json:"priority,omitempty"`
	Type             *string   `json:"type,omitempty"`
	Summary          *string   `json:"summary,omitempty"`
	Error            *string   `json:"error,omitempty"`
	Labels           *[]string `json:"labels,omitempty"`
	ParentID         *string   `json:"parentId,omitempty"`
	DueDate          *string   `json:"dueDate,omitempty"`
	EstimateDuration *string   `json:"estimateDuration,omitempty"`
	Position         *float64  `json:"position,omitempty"`
	Assignee         *string   `json:"assignee,omitempty"`
	Prompt           *string   `json:"prompt,omitempty"`
}

type runtimeTaskCompleteBody struct {
	Agent   string            `json:"agent"`
	Status  string            `json:"status"`
	Summary string            `json:"summary"`
	Error   string            `json:"error"`
	Outputs map[string]string `json:"outputs"`
}

type runtimeConfirmRequestBody struct {
	Agent       string   `json:"agent"`
	To          string   `json:"to"`
	Summary     string   `json:"summary"`
	ActionHint  string   `json:"actionHint"`
	ActionItems []string `json:"actionItems"`
}

type runtimeMessageBody struct {
	To      any    `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
	ReplyTo string `json:"replyTo"`
}

type runtimeReplyMessageBody struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func (s *Server) runtimeRequireCapability(w http.ResponseWriter, r *http.Request, capability string) (runtimeAgentPrincipal, bool) {
	principal, ok := runtimeAgentFromRequest(r)
	if !ok {
		s.jsonErrorCode(w, http.StatusUnauthorized, ErrCodeRuntimeAgentTokenRequired, "runtime agent token required")
		return runtimeAgentPrincipal{}, false
	}
	if !runtimeHasCapability(principal, capability) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeRuntimeCapabilityRequired, "runtime token lacks "+capability+" capability")
		return runtimeAgentPrincipal{}, false
	}
	return principal, true
}

func runtimeAgentAddress(principal runtimeAgentPrincipal) string {
	return principal.Project + "/" + principal.Agent
}

func (s *Server) runtimeTargetAgent(w http.ResponseWriter, principal runtimeAgentPrincipal, requested string) (string, bool) {
	agent := strings.TrimSpace(requested)
	if agent == "" {
		agent = principal.Agent
	}
	if !s.agentExistsInProject(principal.Project, agent) {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeAgentNotFound, "agent not found in runtime project")
		return "", false
	}
	return agent, true
}

func (s *Server) runtimeFindTask(principal runtimeAgentPrincipal, id, requestedAgent string) (*entity.Task, string, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, "", false, fmt.Errorf("task id is required")
	}
	if agent := strings.TrimSpace(requestedAgent); agent != "" {
		t, err := s.ts.GetTask(principal.Project, agent, id)
		return t, agent, false, err
	}
	agents, err := s.st.ListAgents(principal.Project)
	if err != nil {
		return nil, "", false, err
	}
	for _, ag := range agents {
		t, err := s.ts.GetTask(principal.Project, ag.Name, id)
		if err == nil {
			archived := t.Status.IsTerminal()
			return t, ag.Name, archived, nil
		}
	}
	return nil, "", false, fmt.Errorf("task not found")
}

func (s *Server) handleRuntimeTasks(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.runtimeRequireCapability(w, r, "task.use")
	if !ok {
		return
	}
	q := r.URL.Query()
	qStatus := strings.TrimSpace(q.Get("status"))
	qAgent := strings.TrimSpace(q.Get("agent"))
	qScope := strings.TrimSpace(q.Get("scope"))
	if qScope == "" {
		qScope = "all"
	}
	if qScope != "active" && qScope != "archived" && qScope != "all" {
		s.jsonError(w, http.StatusBadRequest, "scope must be active, archived, or all")
		return
	}
	agents, err := s.st.ListAgents(principal.Project)
	if err != nil {
		s.serverError(w, err)
		return
	}
	rows := make([]taskRow, 0)
	addTasks := func(agent string, archived bool) {
		var tasks []*entity.Task
		var err error
		if archived {
			tasks, err = s.ts.ListArchivedTasks(principal.Project, agent)
		} else {
			tasks, err = s.ts.ListTasks(principal.Project, agent)
		}
		if err != nil {
			return
		}
		for _, t := range tasks {
			if t == nil {
				continue
			}
			if qStatus != "" && string(t.Status) != qStatus {
				continue
			}
			rows = append(rows, taskToRow(t, principal.Project, agent, archived))
		}
	}
	for _, ag := range agents {
		if qAgent != "" && ag.Name != qAgent {
			continue
		}
		if qScope == "active" || qScope == "all" {
			addTasks(ag.Name, false)
		}
		if qScope == "archived" || qScope == "all" {
			addTasks(ag.Name, true)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].UpdatedAt.After(rows[j].UpdatedAt)
	})
	_ = json.NewEncoder(w).Encode(rows)
}

func (s *Server) handleRuntimeTask(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.runtimeRequireCapability(w, r, "task.use")
	if !ok {
		return
	}
	t, agent, archived, err := s.runtimeFindTask(principal, r.PathValue("id"), r.URL.Query().Get("agent"))
	if err != nil {
		s.jsonError(w, http.StatusNotFound, "task not found")
		return
	}
	_ = json.NewEncoder(w).Encode(taskToRow(t, principal.Project, agent, archived))
}

func (s *Server) handleRuntimePostTask(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.runtimeRequireCapability(w, r, "task.use")
	if !ok {
		return
	}
	var body runtimeTaskBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	agent, ok := s.runtimeTargetAgent(w, principal, body.Agent)
	if !ok {
		return
	}
	title := strings.TrimSpace(body.Title)
	prompt := strings.TrimSpace(body.Prompt)
	if title == "" || prompt == "" {
		s.jsonError(w, http.StatusBadRequest, "title and prompt are required")
		return
	}
	taskType := strings.TrimSpace(body.Type)
	if taskType == "" {
		taskType = string(entity.TaskTypeChore)
	}
	if !validTaskType(taskType) {
		s.jsonError(w, http.StatusBadRequest, "invalid task type")
		return
	}
	priority := body.Priority
	if priority < 0 || priority > 3 {
		s.jsonError(w, http.StatusBadRequest, "priority must be 0-3")
		return
	}
	assignee := strings.TrimSpace(body.Assignee)
	if assignee == "" {
		assignee = principal.Project + "/" + agent
	}
	if err := s.validateIdentity(assignee, "assignee"); err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	now := time.Now().UTC()
	t := &entity.Task{
		ID:          entity.NewTaskID(),
		Title:       title,
		Description: strings.TrimSpace(body.Description),
		Type:        entity.TaskType(taskType),
		Priority:    priority,
		Assignee:    assignee,
		CreatedBy:   runtimeAgentAddress(principal),
		Status:      entity.TaskStatusPending,
		Prompt:      prompt,
		Labels:      body.Labels,
		ParentID:    strings.TrimSpace(body.ParentID),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if est, err := entity.NormalizeEstimateDuration(body.EstimateDuration); err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	} else {
		t.EstimateDuration = est
	}
	if body.DueDate != "" {
		dd, err := time.Parse("2006-01-02", body.DueDate)
		if err != nil {
			s.jsonError(w, http.StatusBadRequest, "invalid due date, use YYYY-MM-DD")
			return
		}
		t.DueDate = &dd
	}
	if err := s.ts.AddTask(principal.Project, agent, t); err != nil {
		s.serverError(w, err)
		return
	}
	s.triggers.Fire(principal.Project, agent, entity.TriggerOnTask, "task "+t.ID)
	s.auditLog(auditLogInput{
		WorkspaceID:  principal.WorkspaceID,
		ActorType:    "agent",
		ActorID:      runtimeAgentAddress(principal),
		Action:       "runtime.task.create",
		ResourceType: "task",
		ResourceID:   principal.Project + "/" + agent + "/" + t.ID,
		Summary:      "Runtime agent created task",
		After:        taskToRow(t, principal.Project, agent, false),
		Request:      r,
	})
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(taskToRow(t, principal.Project, agent, false))
}

func (s *Server) handleRuntimePutTask(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.runtimeRequireCapability(w, r, "task.use")
	if !ok {
		return
	}
	var body runtimeTaskUpdateBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	t, agent, _, err := s.runtimeFindTask(principal, r.PathValue("id"), body.Agent)
	if err != nil {
		s.jsonError(w, http.StatusNotFound, "task not found")
		return
	}
	if body.Error != nil {
		t.LastError = strings.TrimSpace(*body.Error)
	}
	patch, err := runtimeTaskPatch(body)
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if runtimeTaskPatchHasFields(body) {
		if _, err := taskstore.ApplyTaskPatch(t, patch, time.Now().UTC()); err != nil {
			s.jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
	} else if body.Error != nil {
		t.UpdatedAt = time.Now().UTC()
	} else {
		s.jsonError(w, http.StatusBadRequest, "at least one field to update is required")
		return
	}
	if err := s.ts.PersistTask(principal.Project, agent, t); err != nil {
		s.serverError(w, err)
		return
	}
	if patch.Status != nil && t.Status.IsTerminal() && t.CreatedBy != "" {
		s.notifyTaskDone(t, principal.Project, agent)
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  principal.WorkspaceID,
		ActorType:    "agent",
		ActorID:      runtimeAgentAddress(principal),
		Action:       "runtime.task.update",
		ResourceType: "task",
		ResourceID:   principal.Project + "/" + agent + "/" + t.ID,
		Summary:      "Runtime agent updated task",
		After:        taskToRow(t, principal.Project, agent, t.Status.IsTerminal()),
		Request:      r,
	})
	_ = json.NewEncoder(w).Encode(taskToRow(t, principal.Project, agent, t.Status.IsTerminal()))
}

func runtimeTaskPatchHasFields(body runtimeTaskUpdateBody) bool {
	return body.Title != nil || body.Description != nil || body.Status != nil || body.Priority != nil ||
		body.Type != nil || body.Summary != nil || body.Labels != nil || body.ParentID != nil ||
		body.DueDate != nil || body.EstimateDuration != nil || body.Position != nil || body.Assignee != nil ||
		body.Prompt != nil
}

func runtimeTaskPatch(body runtimeTaskUpdateBody) (taskstore.TaskPatch, error) {
	patch := taskstore.TaskPatch{
		Title: body.Title, Description: body.Description, Summary: body.Summary,
		Labels: body.Labels, ParentID: body.ParentID, DueDate: body.DueDate,
		EstimateDuration: body.EstimateDuration, Position: body.Position,
		Assignee: body.Assignee, Prompt: body.Prompt,
	}
	if body.Status != nil {
		st := strings.TrimSpace(*body.Status)
		if st == "" || !validTaskStatus(st) {
			return patch, fmt.Errorf("invalid task status")
		}
		status := entity.TaskStatus(st)
		patch.Status = &status
	}
	if body.Priority != nil {
		p := *body.Priority
		patch.Priority = &p
	}
	if body.Type != nil {
		typ := strings.TrimSpace(*body.Type)
		if typ == "" || !validTaskType(typ) {
			return patch, fmt.Errorf("invalid task type")
		}
		taskType := entity.TaskType(typ)
		patch.Type = &taskType
	}
	return patch, nil
}

func (s *Server) handleRuntimeTaskComplete(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.runtimeRequireCapability(w, r, "task.use")
	if !ok {
		return
	}
	var body runtimeTaskCompleteBody
	if r.Body != nil && r.ContentLength != 0 {
		if err := s.readJSON(w, r, &body); err != nil {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
			return
		}
	}
	t, agent, _, err := s.runtimeFindTask(principal, r.PathValue("id"), body.Agent)
	if err != nil {
		s.jsonError(w, http.StatusNotFound, "task not found")
		return
	}
	if s.runtimeTaskHasWorkflow(principal.WorkspaceID, principal.Project, t.ID) {
		s.jsonError(w, http.StatusBadRequest, "workflow tasks must complete the current workflow step with `mga step done`")
		return
	}
	status := normalizeDoneStatus(body.Status, body.Error)
	now := time.Now().UTC()
	prev := t.Status
	t.Status = status
	t.Summary = strings.TrimSpace(body.Summary)
	t.LastError = strings.TrimSpace(body.Error)
	t.UpdatedAt = now
	entity.ApplyStatusTimestamps(t, prev, now)
	if err := s.ts.ArchiveTask(principal.Project, agent, t); err != nil {
		s.serverError(w, err)
		return
	}
	if t.CreatedBy != "" {
		s.notifyTaskDone(t, principal.Project, agent)
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  principal.WorkspaceID,
		ActorType:    "agent",
		ActorID:      runtimeAgentAddress(principal),
		Action:       "runtime.task.complete",
		ResourceType: "task",
		ResourceID:   principal.Project + "/" + agent + "/" + t.ID,
		Summary:      "Runtime agent completed task",
		After:        taskToRow(t, principal.Project, agent, true),
		Request:      r,
	})
	_ = json.NewEncoder(w).Encode(taskToRow(t, principal.Project, agent, true))
}

func (s *Server) handleRuntimeWorkflowStepComplete(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.runtimeRequireCapability(w, r, "task.use")
	if !ok {
		return
	}
	var body runtimeTaskCompleteBody
	if r.Body != nil && r.ContentLength != 0 {
		if err := s.readJSON(w, r, &body); err != nil {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
			return
		}
	}
	t, agent, _, err := s.runtimeFindTask(principal, r.PathValue("id"), body.Agent)
	if err != nil {
		s.jsonError(w, http.StatusNotFound, "task not found")
		return
	}
	doneStatus := normalizeDoneStatus(body.Status, body.Error)
	stepStatus := "completed"
	if doneStatus == entity.TaskStatusDoneFailed {
		stepStatus = "failed"
	}
	now := time.Now().UTC()
	t.Summary = strings.TrimSpace(body.Summary)
	t.LastError = strings.TrimSpace(body.Error)
	t.UpdatedAt = now
	transition, transitioned, err := s.completeRuntimeWorkflowStep(principal.WorkspaceID, principal.Project, t, body.Outputs, stepStatus)
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !transitioned {
		s.jsonError(w, http.StatusBadRequest, "task is not attached to an active workflow")
		return
	}
	if transition.Done {
		prev := t.Status
		t.Status = entity.TaskStatusDoneSuccess
		if stepStatus == "failed" {
			t.Status = entity.TaskStatusDoneFailed
		}
		entity.ApplyStatusTimestamps(t, prev, now)
		if err := s.ts.ArchiveTask(principal.Project, agent, t); err != nil {
			s.serverError(w, err)
			return
		}
		if t.CreatedBy != "" {
			s.notifyTaskDone(t, principal.Project, agent)
		}
	} else if err := s.activateNextWorkflowStep(principal.Project, agent, t, transition); err != nil {
		s.serverError(w, err)
		return
	}
	archived := transition.Done && t.Status.IsTerminal()
	s.auditLog(auditLogInput{
		WorkspaceID:  principal.WorkspaceID,
		ActorType:    "agent",
		ActorID:      runtimeAgentAddress(principal),
		Action:       "runtime.workflow.step.complete",
		ResourceType: "task",
		ResourceID:   principal.Project + "/" + agent + "/" + t.ID,
		Summary:      "Runtime agent completed workflow step",
		After:        taskToRow(t, principal.Project, agent, archived),
		Request:      r,
	})
	_ = json.NewEncoder(w).Encode(taskToRow(t, principal.Project, agent, archived))
}

func (s *Server) completeRuntimeWorkflowStep(workspaceID, project string, t *entity.Task, outputs map[string]string, stepStatus string) (workflowstore.TransitionResult, bool, error) {
	var result workflowstore.TransitionResult
	if s == nil || s.controlDB == nil || t == nil || strings.TrimSpace(workspaceID) == "" {
		return result, false, nil
	}
	wfStore := workflowstore.NewStore(s.controlDB, workspaceID)
	if _, ok, err := wfStore.RunForTask(project, t.ID); err != nil || !ok {
		return result, false, err
	}
	output := strings.TrimSpace(t.Summary)
	if output == "" {
		output = strings.TrimSpace(t.LastError)
	}
	result, err := wfStore.CompleteAndAdvance(project, t.ID, t.Summary, output, outputs, stepStatus)
	if err != nil {
		return result, false, err
	}
	return result, result.Next != nil || result.Done, nil
}

func (s *Server) runtimeTaskHasWorkflow(workspaceID, project, taskID string) bool {
	if s == nil || s.controlDB == nil || strings.TrimSpace(workspaceID) == "" {
		return false
	}
	wfStore := workflowstore.NewStore(s.controlDB, workspaceID)
	_, ok, err := wfStore.RunForTask(project, taskID)
	return err == nil && ok
}

func (s *Server) activateNextWorkflowStep(project, previousAgent string, completed *entity.Task, transition workflowstore.TransitionResult) error {
	if completed == nil || transition.Done || transition.Next == nil || transition.NextInst == nil {
		return nil
	}
	next := transition.Next
	inst := transition.NextInst
	now := time.Now().UTC()
	nextPrompt := workflowContinuationPrompt(completed, *next, *inst)
	if inst.ActorType == "agent" && strings.TrimSpace(inst.ActorID) != "" {
		nextAgent := strings.TrimSpace(inst.ActorID)
		if nextAgent == previousAgent {
			completed.Status = entity.TaskStatusPending
			completed.Prompt = nextPrompt
			completed.Assignee = project + "/" + nextAgent
			completed.UpdatedAt = now
			completed.FinishedAt = nil
			if err := s.ts.PersistTask(project, nextAgent, completed); err != nil {
				return err
			}
			s.triggers.Fire(project, nextAgent, entity.TriggerOnTask, "workflow task "+completed.ID)
			return nil
		}
		_ = s.ts.DeleteTask(project, previousAgent, completed.ID)
		completed.Status = entity.TaskStatusPending
		completed.Prompt = nextPrompt
		completed.Assignee = project + "/" + nextAgent
		completed.UpdatedAt = now
		completed.FinishedAt = nil
		if err := s.ts.AddTask(project, nextAgent, completed); err != nil {
			return err
		}
		s.triggers.Fire(project, nextAgent, entity.TriggerOnTask, "workflow task "+completed.ID)
		return nil
	}
	if inst.ActorType == "human" {
		reviewer := strings.TrimSpace(inst.ActorID)
		if reviewer == "" {
			return fmt.Errorf("workflow human review step %q has no assigned user", inst.StepID)
		}
		if err := s.validateIdentity(reviewer, "workflow reviewer"); err != nil {
			return err
		}
		completed.Status = entity.TaskStatusAwaitingConfirmation
		completed.Prompt = nextPrompt
		completed.Assignee = reviewer
		completed.UpdatedAt = now
		completed.FinishedAt = nil
		if err := s.ts.PersistTask(project, previousAgent, completed); err != nil {
			return err
		}
		return s.ts.AddToInbox(&entity.InboxItem{
			TaskID:      completed.ID,
			Project:     project,
			Agent:       previousAgent,
			To:          reviewer,
			Title:       completed.Title,
			Summary:     strings.TrimSpace(inst.InputArtifact),
			ActionHint:  "Review the workflow step and choose approved or needs_changes.",
			ActionItems: []string{"Open the task workflow panel.", "Review the previous step output.", "Approve or request changes with clear comments."},
		})
	}
	return nil
}

func workflowContinuationPrompt(task *entity.Task, step entity.WorkflowStep, inst entity.WorkflowStepInstance) string {
	var b strings.Builder
	b.WriteString("Continue this workflow task from the current active step.\n\n")
	b.WriteString("Current step: ")
	b.WriteString(step.Title)
	b.WriteString(" (`")
	b.WriteString(step.ID)
	b.WriteString("`, type `")
	b.WriteString(step.Type)
	b.WriteString("`)\n")
	if strings.TrimSpace(step.Description) != "" {
		b.WriteString("Step goal: ")
		b.WriteString(strings.TrimSpace(step.Description))
		b.WriteString("\n")
	}
	if strings.TrimSpace(inst.InputArtifact) != "" {
		b.WriteString("\nInput from previous step:\n")
		b.WriteString(inst.InputArtifact)
		b.WriteString("\n")
	}
	if strings.TrimSpace(task.Prompt) != "" {
		b.WriteString("\nOriginal task prompt:\n")
		b.WriteString(task.Prompt)
		b.WriteString("\n")
	}
	b.WriteString("\nFollow the workflow context injected by Multigent and complete only this step.")
	return b.String()
}

func normalizeDoneStatus(status, errText string) entity.TaskStatus {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "failed", "failure", "error", string(entity.TaskStatusDoneFailed):
		return entity.TaskStatusDoneFailed
	case "success", "succeeded", "done", string(entity.TaskStatusDoneSuccess):
		return entity.TaskStatusDoneSuccess
	}
	if strings.TrimSpace(errText) != "" {
		return entity.TaskStatusDoneFailed
	}
	return entity.TaskStatusDoneSuccess
}

func (s *Server) handleRuntimeTaskConfirmRequest(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.runtimeRequireCapability(w, r, "task.use")
	if !ok {
		return
	}
	var body runtimeConfirmRequestBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	summary := strings.TrimSpace(body.Summary)
	if summary == "" {
		s.jsonError(w, http.StatusBadRequest, "summary is required")
		return
	}
	to := strings.TrimSpace(body.To)
	if to == "" {
		to = "human"
	}
	if err := s.validateIdentity(to, "to"); err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	t, agent, _, err := s.runtimeFindTask(principal, r.PathValue("id"), body.Agent)
	if err != nil {
		s.jsonError(w, http.StatusNotFound, "task not found")
		return
	}
	now := time.Now().UTC()
	prev := t.Status
	t.Status = entity.TaskStatusAwaitingConfirmation
	t.ConfirmationReq = &entity.ConfirmationRequest{
		Summary:     summary,
		ActionHint:  strings.TrimSpace(body.ActionHint),
		ActionItems: body.ActionItems,
	}
	t.UpdatedAt = now
	entity.ApplyStatusTimestamps(t, prev, now)
	if err := s.ts.PersistTask(principal.Project, agent, t); err != nil {
		s.serverError(w, err)
		return
	}
	if err := s.ts.AddToInbox(&entity.InboxItem{
		TaskID:      t.ID,
		Project:     principal.Project,
		Agent:       agent,
		To:          to,
		Title:       t.Title,
		Summary:     summary,
		ActionHint:  strings.TrimSpace(body.ActionHint),
		ActionItems: body.ActionItems,
	}); err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  principal.WorkspaceID,
		ActorType:    "agent",
		ActorID:      runtimeAgentAddress(principal),
		Action:       "runtime.task.confirm_request",
		ResourceType: "task",
		ResourceID:   principal.Project + "/" + agent + "/" + t.ID,
		Summary:      "Runtime agent requested confirmation",
		After:        taskToRow(t, principal.Project, agent, false),
		Request:      r,
	})
	_ = json.NewEncoder(w).Encode(taskToRow(t, principal.Project, agent, false))
}

func (s *Server) handleRuntimeMessages(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.runtimeRequireCapability(w, r, "message.use")
	if !ok {
		return
	}
	mailbox := strings.TrimSpace(r.URL.Query().Get("mailbox"))
	if mailbox == "" {
		mailbox = runtimeAgentAddress(principal)
	}
	if mailbox != runtimeAgentAddress(principal) {
		s.jsonError(w, http.StatusForbidden, "runtime agents can only read their own mailbox")
		return
	}
	includeArchived := strings.EqualFold(r.URL.Query().Get("archived"), "all") || r.URL.Query().Get("includeArchived") == "1"
	var msgs []*entity.Message
	var err error
	if includeArchived {
		msgs, err = s.ts.ListAllMessages(mailbox)
	} else {
		msgs, err = s.ts.ListMessages(mailbox)
	}
	if err != nil {
		s.serverError(w, err)
		return
	}
	rows := make([]msgRow, 0, len(msgs))
	for _, m := range msgs {
		if m == nil {
			continue
		}
		rows = append(rows, messageToRow(m, mailbox))
	}
	_ = json.NewEncoder(w).Encode(rows)
}

func messageToRow(m *entity.Message, mailbox string) msgRow {
	sent := m.SentAt.UTC()
	var read *time.Time
	if m.ReadAt != nil {
		t := m.ReadAt.UTC()
		read = &t
	}
	var archived *time.Time
	if m.ArchivedAt != nil {
		t := m.ArchivedAt.UTC()
		archived = &t
	}
	return msgRow{
		ID:         m.ID,
		From:       m.From,
		To:         m.To,
		Subject:    m.Subject,
		Body:       m.Body,
		SentAt:     sent,
		ReadAt:     read,
		ArchivedAt: archived,
		Mailbox:    mailbox,
	}
}

func (s *Server) handleRuntimePostMessage(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.runtimeRequireCapability(w, r, "message.use")
	if !ok {
		return
	}
	var body runtimeMessageBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	bodyText := strings.TrimSpace(body.Body)
	if bodyText == "" {
		s.jsonError(w, http.StatusBadRequest, "body is required")
		return
	}
	recipients, err := normalizeToRecipients(body.To)
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	for _, rec := range recipients {
		if err := s.validateRuntimeRecipient(principal, rec); err != nil {
			s.jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	from := runtimeAgentAddress(principal)
	sentAt := time.Now().UTC()
	ids := make([]string, 0, len(recipients))
	for _, rec := range recipients {
		msg := &entity.Message{
			ID:      entity.NewMessageID(),
			From:    from,
			To:      rec,
			Subject: strings.TrimSpace(body.Subject),
			Body:    bodyText,
			ReplyTo: strings.TrimSpace(body.ReplyTo),
			SentAt:  sentAt,
		}
		if err := s.ts.SendMessage(msg); err != nil {
			s.serverError(w, err)
			return
		}
		ids = append(ids, msg.ID)
		if parts := strings.SplitN(rec, "/", 2); len(parts) == 2 {
			s.triggers.Fire(parts[0], parts[1], entity.TriggerOnMessage, "from "+from)
		}
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  principal.WorkspaceID,
		ActorType:    "agent",
		ActorID:      from,
		Action:       "runtime.message.send",
		ResourceType: "message",
		ResourceID:   strings.Join(ids, ","),
		Summary:      "Runtime agent sent message",
		After:        map[string]any{"to": recipients, "subject": strings.TrimSpace(body.Subject)},
		Request:      r,
	})
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"ids": ids})
}

func (s *Server) validateRuntimeRecipient(principal runtimeAgentPrincipal, recipient string) error {
	if err := s.validateIdentity(recipient, "to"); err != nil {
		return err
	}
	if recipient == "human" {
		return nil
	}
	project, _, ok := splitAgentMailbox(recipient)
	if !ok {
		return nil
	}
	if project != principal.Project {
		return fmt.Errorf("runtime agent can only message human or agents in project %s", principal.Project)
	}
	return nil
}

func (s *Server) handleRuntimeReplyMessage(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.runtimeRequireCapability(w, r, "message.use")
	if !ok {
		return
	}
	var body runtimeReplyMessageBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	bodyText := strings.TrimSpace(body.Body)
	if bodyText == "" {
		s.jsonError(w, http.StatusBadRequest, "body is required")
		return
	}
	mailbox := runtimeAgentAddress(principal)
	msgs, err := s.ts.ListAllMessages(mailbox)
	if err != nil {
		s.serverError(w, err)
		return
	}
	var original *entity.Message
	for _, m := range msgs {
		if m != nil && m.ID == r.PathValue("id") {
			original = m
			break
		}
	}
	if original == nil {
		s.jsonError(w, http.StatusNotFound, "message not found")
		return
	}
	subject := strings.TrimSpace(body.Subject)
	if subject == "" {
		subject = original.Subject
		if subject != "" && !strings.HasPrefix(strings.ToLower(subject), "re:") {
			subject = "Re: " + subject
		}
	}
	msg := &entity.Message{
		ID:      entity.NewMessageID(),
		From:    mailbox,
		To:      original.From,
		Subject: subject,
		Body:    bodyText,
		ReplyTo: original.ID,
		SentAt:  time.Now().UTC(),
	}
	if err := s.validateRuntimeRecipient(principal, msg.To); err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.ts.SendMessage(msg); err != nil {
		s.serverError(w, err)
		return
	}
	if parts := strings.SplitN(msg.To, "/", 2); len(parts) == 2 {
		s.triggers.Fire(parts[0], parts[1], entity.TriggerOnMessage, "from "+mailbox)
	}
	_ = s.ts.MarkMessageRead(mailbox, original.ID)
	s.auditLog(auditLogInput{
		WorkspaceID:  principal.WorkspaceID,
		ActorType:    "agent",
		ActorID:      mailbox,
		Action:       "runtime.message.reply",
		ResourceType: "message",
		ResourceID:   msg.ID,
		Summary:      "Runtime agent replied to message",
		After:        messageToRow(msg, msg.To),
		Request:      r,
	})
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"id": msg.ID})
}
