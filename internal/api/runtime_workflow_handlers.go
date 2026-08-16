package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/errs"
	"github.com/multigent/multigent/internal/taskstore"
	"github.com/multigent/multigent/internal/tasktemplate"
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

const (
	workflowRootTaskIDVar = "workflow_root_task_id"
	workflowRunIDVar      = "workflow_run_id"
	workflowStepIDVar     = "workflow_step_id"
	workflowBranchIDVar   = "workflow_branch_id"
)

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

func (s *Server) handleRuntimeTaskTemplates(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.runtimeRequireCapability(w, r, "task.use")
	if !ok {
		return
	}
	store := tasktemplate.NewStore(s.controlDB, principal.WorkspaceID)
	templates, err := store.List()
	if err != nil {
		s.serverError(w, err)
		return
	}
	filtered := make([]entity.TaskTemplate, 0, len(templates))
	for _, template := range templates {
		if template.Project == principal.Project {
			filtered = append(filtered, template)
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"templates": filtered})
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

func (s *Server) handleRuntimePostTaskFromTemplate(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.runtimeRequireCapability(w, r, "task.use")
	if !ok {
		return
	}
	var body taskFromTemplateBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	store := tasktemplate.NewStore(s.controlDB, principal.WorkspaceID)
	template, found, err := store.Get(strings.TrimSpace(body.TemplateID))
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found {
		s.jsonError(w, http.StatusNotFound, "task template not found")
		return
	}
	if template.Project != "" && template.Project != principal.Project {
		s.jsonError(w, http.StatusBadRequest, "task template is not available for this project")
		return
	}
	taskBody, err := instantiateTaskTemplate(template, body)
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if taskBody.Agent == "" {
		taskBody.Agent = firstTemplateAgentBinding(taskBody.WorkflowActorBindings)
	}
	s.createRuntimeTaskFromBody(w, r, principal, taskBody)
}

func (s *Server) createRuntimeTaskFromBody(w http.ResponseWriter, r *http.Request, principal runtimeAgentPrincipal, body postTaskBody) {
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
	workflowID := strings.TrimSpace(body.WorkflowDefinitionID)
	var workflowStore interface {
		Definition(string) (entity.WorkflowDefinition, bool, error)
		StartRunWithInput(string, string, string, map[string]entity.WorkflowActorBinding, map[string]string) (entity.WorkflowRun, []entity.WorkflowStepInstance, error)
	}
	var workflowDef entity.WorkflowDefinition
	if workflowID != "" {
		wfStore := workflowstore.NewStore(s.controlDB, principal.WorkspaceID)
		workflowStore = wfStore
		def, found, err := wfStore.Definition(workflowID)
		if err != nil {
			s.serverError(w, err)
			return
		}
		if !found {
			s.jsonError(w, http.StatusNotFound, "workflow definition not found")
			return
		}
		workflowDef = def
		if _, inst, ok := workflowStartActor(def, body.WorkflowActorBindings); ok {
			switch inst.ActorType {
			case "agent":
				startAgent := strings.TrimSpace(inst.ActorID)
				if startAgent == "" || !s.agentExistsInProject(principal.Project, startAgent) {
					s.jsonError(w, http.StatusBadRequest, "workflow start agent not found in this project")
					return
				}
				agent = startAgent
				assignee = principal.Project + "/" + startAgent
			case "human":
				reviewer := strings.TrimSpace(inst.ActorID)
				if reviewer == "" {
					s.jsonError(w, http.StatusBadRequest, "workflow start reviewer is required")
					return
				}
				if err := s.validateIdentity(reviewer, "workflow start reviewer"); err != nil {
					s.jsonError(w, http.StatusBadRequest, err.Error())
					return
				}
				assignee = reviewer
			}
		}
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
	if !strings.Contains(assignee, "/") {
		item := &entity.InboxItem{TaskID: t.ID, Project: principal.Project, Agent: agent, To: assignee, Title: t.Title, Summary: prompt}
		if err := s.ts.AddToInbox(item); err != nil {
			s.serverError(w, err)
			return
		}
	}
	if workflowID != "" {
		if workflowStore == nil {
			s.serverError(w, fmt.Errorf("workflow store unavailable"))
			return
		}
		initialInputs := workflowInitialInputsFromPrompt(workflowDef, prompt)
		if _, _, err := workflowStore.StartRunWithInput(principal.Project, t.ID, workflowID, body.WorkflowActorBindings, initialInputs); err != nil {
			s.serverError(w, err)
			return
		}
	}
	if strings.Contains(assignee, "/") {
		s.triggers.Fire(principal.Project, agent, entity.TriggerOnTask, "task "+t.ID)
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  principal.WorkspaceID,
		ActorType:    "agent",
		ActorID:      runtimeAgentAddress(principal),
		Action:       "runtime.task.create_from_template",
		ResourceType: "task",
		ResourceID:   principal.Project + "/" + agent + "/" + t.ID,
		Summary:      "Runtime agent created task from template",
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
	if strings.TrimSpace(t.Vars[workflowBranchIDVar]) != "" {
		s.completeRuntimeWorkflowBranchHTTP(w, r, principal, t, agent, body)
		return
	}
	if s.runtimeTaskHasWorkflow(principal.WorkspaceID, principal.Project, t.ID) {
		s.jsonError(w, http.StatusBadRequest, "workflow tasks must complete the current workflow step with `mga task step done`")
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

func (s *Server) handleRuntimeWorkflowBranchComplete(w http.ResponseWriter, r *http.Request) {
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
	if strings.TrimSpace(t.Vars[workflowBranchIDVar]) == "" {
		s.jsonError(w, http.StatusBadRequest, "task is not attached to a workflow branch")
		return
	}
	s.completeRuntimeWorkflowBranchHTTP(w, r, principal, t, agent, body)
}

func (s *Server) completeRuntimeWorkflowBranchHTTP(w http.ResponseWriter, r *http.Request, principal runtimeAgentPrincipal, t *entity.Task, agent string, body runtimeTaskCompleteBody) {
	doneStatus := normalizeDoneStatus(body.Status, body.Error)
	stepStatus := "completed"
	if doneStatus == entity.TaskStatusDoneFailed {
		stepStatus = "failed"
	}
	result, err := s.completeRuntimeWorkflowBranch(principal.WorkspaceID, principal.Project, t, body.Outputs, stepStatus)
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	now := time.Now().UTC()
	prev := t.Status
	t.Status = doneStatus
	t.Summary = strings.TrimSpace(body.Summary)
	t.LastError = strings.TrimSpace(body.Error)
	t.UpdatedAt = now
	entity.ApplyStatusTimestamps(t, prev, now)
	if err := s.ts.ArchiveTask(principal.Project, agent, t); err != nil {
		s.serverError(w, err)
		return
	}
	if err := s.advanceParentAfterBranchCompletion(principal.WorkspaceID, principal.Project, result, r); err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  principal.WorkspaceID,
		ActorType:    "agent",
		ActorID:      runtimeAgentAddress(principal),
		Action:       "runtime.workflow.branch.complete",
		ResourceType: "task",
		ResourceID:   principal.Project + "/" + agent + "/" + t.ID,
		Summary:      "Runtime agent completed workflow branch",
		After:        taskToRow(t, principal.Project, agent, true),
		Request:      r,
	})
	_ = json.NewEncoder(w).Encode(map[string]any{
		"task":    taskToRow(t, principal.Project, agent, true),
		"branch":  result.Branch,
		"allDone": result.AllDone,
	})
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
		if t.CreatedBy != "" && strings.TrimSpace(t.Vars[workflowBranchIDVar]) == "" {
			s.notifyTaskDone(t, principal.Project, agent)
		}
		if strings.TrimSpace(t.Vars[workflowBranchIDVar]) != "" {
			branchResult, err := s.completeRuntimeWorkflowBranch(principal.WorkspaceID, principal.Project, t, body.Outputs, stepStatus)
			if err != nil {
				s.jsonError(w, http.StatusBadRequest, err.Error())
				return
			}
			if err := s.advanceParentAfterBranchCompletion(principal.WorkspaceID, principal.Project, branchResult, r); err != nil {
				s.serverError(w, err)
				return
			}
		}
	} else if err := s.activateNextWorkflowStep(principal.WorkspaceID, principal.Project, agent, t, transition, r); err != nil {
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

func (s *Server) completeRuntimeWorkflowBranch(workspaceID, project string, t *entity.Task, outputs map[string]string, stepStatus string) (workflowstore.BranchTransitionResult, error) {
	var result workflowstore.BranchTransitionResult
	if s == nil || s.controlDB == nil || t == nil || strings.TrimSpace(workspaceID) == "" {
		return result, fmt.Errorf("workflow store is not available")
	}
	rootTaskID := strings.TrimSpace(t.Vars[workflowRootTaskIDVar])
	runID := strings.TrimSpace(t.Vars[workflowRunIDVar])
	stepID := strings.TrimSpace(t.Vars[workflowStepIDVar])
	branchID := strings.TrimSpace(t.Vars[workflowBranchIDVar])
	if rootTaskID == "" || runID == "" || stepID == "" || branchID == "" {
		return result, fmt.Errorf("workflow branch metadata is incomplete")
	}
	wfStore := workflowstore.NewStore(s.controlDB, workspaceID)
	summary := strings.TrimSpace(t.Summary)
	if summary == "" {
		summary = strings.TrimSpace(t.LastError)
	}
	return wfStore.CompleteBranchAndMaybeAdvance(project, rootTaskID, runID, stepID, branchID, summary, outputs, stepStatus)
}

func (s *Server) advanceParentAfterBranchCompletion(workspaceID, project string, result workflowstore.BranchTransitionResult, r *http.Request) error {
	rootTaskID := strings.TrimSpace(result.Transition.Run.TaskID)
	if rootTaskID == "" {
		return nil
	}
	root, rootAgent, err := s.findTaskInProject(project, rootTaskID)
	if err != nil || root == nil {
		return nil
	}
	now := time.Now().UTC()
	if result.Branch.Status == "failed" {
		root.Status = entity.TaskStatusBlocked
		root.LastError = strings.TrimSpace(result.Branch.Summary)
		root.UpdatedAt = now
		return s.ts.PersistTask(project, rootAgent, root)
	}
	if !result.AllDone {
		return nil
	}
	if result.Transition.Done {
		prev := root.Status
		root.Status = entity.TaskStatusDoneSuccess
		root.Summary = strings.TrimSpace(result.Transition.Current.Summary)
		root.UpdatedAt = now
		entity.ApplyStatusTimestamps(root, prev, now)
		if err := s.ts.ArchiveTask(project, rootAgent, root); err != nil {
			return err
		}
		if root.CreatedBy != "" {
			s.notifyTaskDone(root, project, rootAgent)
		}
		return nil
	}
	return s.activateNextWorkflowStep(workspaceID, project, rootAgent, root, result.Transition, r)
}

func (s *Server) runtimeTaskHasWorkflow(workspaceID, project, taskID string) bool {
	if s == nil || s.controlDB == nil || strings.TrimSpace(workspaceID) == "" {
		return false
	}
	wfStore := workflowstore.NewStore(s.controlDB, workspaceID)
	_, ok, err := wfStore.RunForTask(project, taskID)
	return err == nil && ok
}

func (s *Server) activateNextWorkflowStep(workspaceID, project, previousAgent string, completed *entity.Task, transition workflowstore.TransitionResult, r *http.Request) error {
	if completed == nil || transition.Done || transition.Next == nil || transition.NextInst == nil {
		return nil
	}
	if transition.Next.Type == "parallel_stage" {
		return s.activateParallelWorkflowStep(workspaceID, project, previousAgent, completed, transition, r)
	}
	inst := transition.NextInst
	now := time.Now().UTC()
	if inst.ActorType == "agent" && strings.TrimSpace(inst.ActorID) != "" {
		nextAgent := strings.TrimSpace(inst.ActorID)
		if err := s.moveWorkflowTaskToAgent(project, previousAgent, nextAgent, completed, entity.TaskStatusPending, now); err != nil {
			return err
		}
		return s.fireTaskTriggerOrQueueRuntime(workspaceID, project, nextAgent, completed, r, "workflow task "+completed.ID)
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
		completed.Assignee = reviewer
		completed.UpdatedAt = now
		completed.FinishedAt = nil
		if err := s.ts.PersistTask(project, previousAgent, completed); err != nil {
			return err
		}
		if err := s.ts.AddToInbox(&entity.InboxItem{
			TaskID:      completed.ID,
			Project:     project,
			Agent:       previousAgent,
			To:          reviewer,
			Title:       completed.Title,
			Summary:     strings.TrimSpace(inst.InputArtifact),
			ActionHint:  "Review the workflow step and choose approved or needs_changes.",
			ActionItems: []string{"Open the task workflow panel.", "Review the previous step output.", "Approve or request changes with clear comments."},
		}); err != nil {
			return err
		}
		if strings.TrimSpace(workspaceID) != "" {
			wfStore := workflowstore.NewStore(s.controlDB, workspaceID)
			def, found, err := wfStore.RunDefinition(transition.Run)
			if err == nil && found {
				s.fireWorkflowStepTriggers(workspaceID, workflowTriggerEvent{
					Type:       "workflow.human_review.required",
					Project:    project,
					TaskID:     completed.ID,
					TaskTitle:  completed.Title,
					Run:        transition.Run,
					Definition: def,
					Step:       *transition.Next,
					Instance:   *transition.NextInst,
				}, r)
			}
		}
		return nil
	}
	return nil
}

func (s *Server) moveWorkflowTaskToAgent(project, previousAgent, nextAgent string, task *entity.Task, status entity.TaskStatus, now time.Time) error {
	if task == nil {
		return fmt.Errorf("task is required")
	}
	previousAgent = strings.TrimSpace(previousAgent)
	nextAgent = strings.TrimSpace(nextAgent)
	if nextAgent == "" {
		return fmt.Errorf("workflow next agent is required")
	}
	task.Status = status
	task.Assignee = project + "/" + nextAgent
	task.UpdatedAt = now
	task.FinishedAt = nil
	task.LastError = ""
	if previousAgent == nextAgent {
		if err := s.ts.PersistTask(project, nextAgent, task); err != nil {
			return err
		}
		return nil
	}
	if _, err := s.ts.GetTask(project, nextAgent, task.ID); err == nil {
		if err := s.ts.PersistTask(project, nextAgent, task); err != nil {
			return err
		}
	} else {
		var notFound *errs.NotFoundError
		if !errors.As(err, &notFound) {
			return err
		}
		if err := s.ts.AddTask(project, nextAgent, task); err != nil {
			return err
		}
	}
	if previousAgent != "" {
		_ = s.ts.DeleteTask(project, previousAgent, task.ID)
	}
	return nil
}

func (s *Server) activateParallelWorkflowStep(workspaceID, project, previousAgent string, completed *entity.Task, transition workflowstore.TransitionResult, r *http.Request) error {
	if completed == nil || transition.Next == nil || transition.NextInst == nil {
		return nil
	}
	step := *transition.Next
	if len(step.Branches) == 0 {
		return fmt.Errorf("parallel workflow step %q has no branches", step.Title)
	}
	wfStore := workflowstore.NewStore(s.controlDB, workspaceID)
	existing, err := wfStore.BranchInstancesForStep(transition.Run.ID, step.ID)
	if err != nil {
		return err
	}
	existingByBranch := make(map[string]bool, len(existing))
	for _, inst := range existing {
		existingByBranch[inst.BranchID] = true
	}
	now := time.Now().UTC()
	completed.Status = entity.TaskStatusInProgress
	completed.Assignee = project + "/" + previousAgent
	completed.UpdatedAt = now
	completed.FinishedAt = nil
	if err := s.ts.PersistTask(project, previousAgent, completed); err != nil {
		return err
	}
	for _, branch := range step.Branches {
		branch.ID = strings.TrimSpace(branch.ID)
		if branch.ID == "" || existingByBranch[branch.ID] {
			continue
		}
		childDef := workflowDefinitionForBranch(transition.Run.DefinitionID, step, branch)
		if err := wfStore.SaveDefinition(&childDef); err != nil {
			return err
		}
		startStep, startInst, ok := workflowStartActor(childDef, transition.Run.ActorBindings)
		if !ok || startStep == nil || startInst == nil {
			return fmt.Errorf("parallel branch %q has no start step", branch.Title)
		}
		if strings.TrimSpace(startInst.ActorType) != "agent" || strings.TrimSpace(startInst.ActorID) == "" {
			return fmt.Errorf("parallel branch %q requires an agent actor binding for role %q", branch.Title, startStep.ActorRole)
		}
		nextAgent := strings.TrimSpace(startInst.ActorID)
		inputValues := workflowBranchInputValuesForFields(transition.Current, workflowBranchInputFields(branch, *startStep))
		inputArtifact := workflowBranchInputArtifact(step, transition.Current, branch, inputValues)
		branchTask := &entity.Task{
			ID:          entity.NewTaskID(),
			Title:       strings.TrimSpace(completed.Title + " · " + branch.Title),
			Type:        completed.Type,
			Priority:    completed.Priority,
			Assignee:    project + "/" + nextAgent,
			CreatedBy:   completed.CreatedBy,
			Status:      entity.TaskStatusPending,
			Description: strings.TrimSpace(branch.Description),
			Prompt:      workflowBranchTaskPrompt(completed, step, branch, *startStep, inputArtifact),
			Labels:      append([]string{}, completed.Labels...),
			ParentID:    completed.ID,
			CreatedAt:   now,
			UpdatedAt:   now,
			Vars: map[string]string{
				workflowRootTaskIDVar: completed.ID,
				workflowRunIDVar:      transition.Run.ID,
				workflowStepIDVar:     step.ID,
				workflowBranchIDVar:   branch.ID,
			},
		}
		if branchTask.Type == "" {
			branchTask.Type = entity.TaskTypeChore
		}
		if err := s.ts.AddTask(project, nextAgent, branchTask); err != nil {
			return err
		}
		childRun, childInstances, err := wfStore.StartRun(project, branchTask.ID, childDef.ID, transition.Run.ActorBindings)
		if err != nil {
			return err
		}
		for i := range childInstances {
			if childInstances[i].StepID != childRun.ActiveStepID {
				continue
			}
			childInstances[i].InputArtifact = inputArtifact
			childInstances[i].InputValues = inputValues
			childInstances[i].ActorType = startInst.ActorType
			childInstances[i].ActorID = startInst.ActorID
			childInstances[i].UpdatedAt = now
			if err := wfStore.SaveStepInstance(&childInstances[i]); err != nil {
				return err
			}
			break
		}
		inst := &entity.WorkflowBranchInstance{
			ID:            entity.NewWorkflowBranchInstanceID(),
			RunID:         transition.Run.ID,
			StepID:        step.ID,
			BranchID:      branch.ID,
			Status:        "running",
			ActorType:     "agent",
			ActorID:       nextAgent,
			ChildTaskID:   branchTask.ID,
			ChildRunID:    childRun.ID,
			StartedAt:     now,
			UpdatedAt:     now,
			InputArtifact: inputArtifact,
			InputValues:   inputValues,
		}
		if err := wfStore.SaveBranchInstance(inst); err != nil {
			return err
		}
		if err := s.fireTaskTriggerOrQueueRuntime(workspaceID, project, nextAgent, branchTask, r, "workflow branch task "+branchTask.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) fireTaskTriggerOrQueueRuntime(workspaceID, project, agent string, task *entity.Task, r *http.Request, reason string) error {
	if s == nil || task == nil {
		return nil
	}
	hb, err := s.ts.GetHeartbeat(project, agent)
	if err != nil || hb == nil || hb.Paused || !hb.HasTrigger(entity.TriggerOnTask) {
		if s.triggers != nil {
			s.triggers.Fire(project, agent, entity.TriggerOnTask, reason)
		}
		return nil
	}
	meta, err := s.st.AgentMeta(project, agent)
	if err != nil || meta == nil || !s.usesAssignedRuntimeNode(workspaceID, meta) {
		if s.triggers != nil {
			s.triggers.Fire(project, agent, entity.TriggerOnTask, reason)
		}
		return nil
	}
	if s.hasActiveRuntimeRun(workspaceID, project, agent, task.ID) {
		return nil
	}
	now := time.Now().UTC()
	prev := task.Status
	task.Status = entity.TaskStatusInProgress
	task.UpdatedAt = now
	task.FinishedAt = nil
	entity.ApplyStatusTimestamps(task, prev, now)
	if err := s.ts.UpdateTask(project, agent, task); err != nil {
		return err
	}
	run, err := s.enqueueRuntimeTaskRun(workspaceID, project, agent, task, "", externalServerURL(r), requestUsername(r))
	if err != nil {
		task.Status = prev
		task.UpdatedAt = now
		_ = s.ts.UpdateTask(project, agent, task)
		return err
	}
	hb.LastWakeup = &now
	hb.LastWakeupStatus = "running"
	hb.PID = 0
	_ = s.ts.SaveHeartbeat(project, agent, hb)
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "runtime_run.enqueue",
		ResourceType: "task",
		ResourceID:   project + "/" + agent + "/" + task.ID,
		Summary:      "Workflow task queued on runtime node",
		After: map[string]any{
			"project":      project,
			"agent":        agent,
			"taskId":       task.ID,
			"runtimeRunId": run.ID,
			"reason":       reason,
		},
		Request: r,
	})
	return nil
}

func workflowBranchInputValues(parent entity.WorkflowStepInstance, branch entity.WorkflowBranch) map[string]string {
	return workflowBranchInputValuesForFields(parent, branch.InputFields)
}

func workflowBranchInputValuesForFields(parent entity.WorkflowStepInstance, fields []entity.WorkflowField) map[string]string {
	out := make(map[string]string)
	for _, field := range fields {
		name := strings.TrimSpace(field.Name)
		if name == "" {
			continue
		}
		if value := strings.TrimSpace(parent.InputValues[name]); value != "" {
			out[name] = value
			continue
		}
		if value := strings.TrimSpace(parent.OutputValues[name]); value != "" {
			out[name] = value
		}
	}
	if len(out) > 0 || len(fields) > 0 {
		return out
	}
	for key, value := range parent.InputValues {
		if strings.TrimSpace(value) != "" {
			out[key] = strings.TrimSpace(value)
		}
	}
	for key, value := range parent.OutputValues {
		if strings.TrimSpace(value) != "" {
			out[key] = strings.TrimSpace(value)
		}
	}
	return out
}

func workflowBranchInputFields(branch entity.WorkflowBranch, startStep entity.WorkflowStep) []entity.WorkflowField {
	if len(branch.InputFields) > 0 {
		return branch.InputFields
	}
	return startStep.InputFields
}

func workflowBranchInputArtifact(step entity.WorkflowStep, parent entity.WorkflowStepInstance, branch entity.WorkflowBranch, inputs map[string]string) string {
	payload := map[string]any{
		"parallel_stage": map[string]string{"id": step.ID, "title": step.Title},
		"branch":         map[string]string{"id": branch.ID, "title": branch.Title},
		"inputs":         inputs,
		"upstream":       parent.OutputValues,
	}
	if len(branch.InputFields) > 0 {
		names := make([]string, 0, len(branch.InputFields))
		for _, field := range branch.InputFields {
			if name := strings.TrimSpace(field.Name); name != "" {
				names = append(names, name)
			}
		}
		payload["expected_input_fields"] = names
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return ""
	}
	return string(raw)
}

func workflowDefinitionForBranch(parentDefinitionID string, parent entity.WorkflowStep, branch entity.WorkflowBranch) entity.WorkflowDefinition {
	defID := workflowBranchDefinitionID(parentDefinitionID, parent.ID, branch.ID)
	now := time.Now().UTC()
	if branch.Workflow != nil && len(branch.Workflow.Steps) > 0 {
		def := *branch.Workflow
		def.ID = defID
		if strings.TrimSpace(def.Name) == "" {
			def.Name = branch.Title
		}
		if strings.TrimSpace(def.Description) == "" {
			def.Description = branch.Description
		}
		if def.Version == 0 {
			def.Version = 1
		}
		def.Scope = "branch"
		def.Project = ""
		if strings.TrimSpace(def.StartStepID) == "" {
			def.StartStepID = strings.TrimSpace(def.Steps[0].ID)
		}
		def.CreatedAt = now
		def.UpdatedAt = now
		return def
	}
	actorRole := strings.TrimSpace(branch.ActorRole)
	if actorRole == "" {
		actorRole = strings.TrimSpace(branch.ID)
	}
	return entity.WorkflowDefinition{
		ID:          defID,
		Name:        strings.TrimSpace(branch.Title),
		Description: strings.TrimSpace(branch.Description),
		Version:     1,
		Scope:       "branch",
		StartStepID: "start",
		Steps: []entity.WorkflowStep{{
			ID:           "start",
			Type:         "agent_task",
			Title:        strings.TrimSpace(branch.Title),
			Description:  strings.TrimSpace(branch.Description),
			ActorRole:    actorRole,
			InputFields:  append([]entity.WorkflowField{}, branch.InputFields...),
			OutputFields: append([]entity.WorkflowField{}, branch.OutputFields...),
			Position:     entity.WorkflowPosition{X: 80, Y: 120},
		}},
		Edges:     []entity.WorkflowEdge{},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func workflowBranchDefinitionID(parentDefinitionID, stepID, branchID string) string {
	parts := []string{"branch", parentDefinitionID, stepID, branchID}
	replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-", "_", "-")
	for i, part := range parts {
		parts[i] = replacer.Replace(strings.TrimSpace(strings.ToLower(part)))
	}
	return strings.Join(parts, "-")
}

func workflowBranchTaskPrompt(root *entity.Task, step entity.WorkflowStep, branch entity.WorkflowBranch, startStep entity.WorkflowStep, inputArtifact string) string {
	var b strings.Builder
	b.WriteString("Continue this branch sub-workflow from the current active step.\n\n")
	b.WriteString("Root task: ")
	b.WriteString(root.ID)
	b.WriteString(" — ")
	b.WriteString(root.Title)
	b.WriteString("\n")
	b.WriteString("Parallel stage: ")
	b.WriteString(step.Title)
	b.WriteString(" (")
	b.WriteString(step.ID)
	b.WriteString(")\n")
	b.WriteString("Branch: ")
	b.WriteString(branch.Title)
	b.WriteString(" (")
	b.WriteString(branch.ID)
	b.WriteString(")\n\n")
	if strings.TrimSpace(startStep.Title) != "" {
		b.WriteString("Current step: ")
		b.WriteString(startStep.Title)
		b.WriteString(" (")
		b.WriteString(startStep.ID)
		b.WriteString(")\n")
	}
	if strings.TrimSpace(startStep.Description) != "" {
		b.WriteString("Step goal:\n")
		b.WriteString(strings.TrimSpace(startStep.Description))
		b.WriteString("\n\n")
	}
	if strings.TrimSpace(inputArtifact) != "" {
		b.WriteString("Input from previous workflow step:\n")
		b.WriteString(inputArtifact)
		b.WriteString("\n\n")
	}
	if len(startStep.OutputFields) > 0 {
		b.WriteString("Required structured outputs:\n")
		for _, field := range startStep.OutputFields {
			name := strings.TrimSpace(field.Name)
			if name == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(name)
			if strings.TrimSpace(field.Description) != "" {
				b.WriteString(": ")
				b.WriteString(strings.TrimSpace(field.Description))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("When finished, report completion with:\n")
	b.WriteString("mga task step done --id \"$MULTIGENT_TASK_ID\" --summary \"...\"")
	if len(startStep.OutputFields) > 0 {
		b.WriteString(" --output-json '{\"field\":\"value\"}'")
	}
	b.WriteString("\n")
	if len(startStep.OutputFields) > 0 {
		b.WriteString("Prefer --output-json for workflow outputs. Use --output field=value only for simple ASCII field names with no spaces; Chinese field names or field names with spaces must use --output-json.\n")
	}
	b.WriteString("Do not complete the root task directly. Multigent will advance the parent workflow after this branch sub-workflow reaches a terminal node.\n")
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
	resolvedTo, err := s.resolveRuntimeRecipient(principal, to)
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	t, agent, _, err := s.runtimeFindTask(principal, r.PathValue("id"), body.Agent)
	if err != nil {
		s.jsonError(w, http.StatusNotFound, "task not found")
		return
	}
	if s.runtimeTaskHasWorkflow(principal.WorkspaceID, principal.Project, t.ID) {
		s.jsonError(w, http.StatusBadRequest, "workflow tasks must request human review through the workflow step route")
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
		To:          resolvedTo,
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

func (s *Server) handleRuntimeContacts(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.runtimeRequireCapability(w, r, "message.use")
	if !ok {
		return
	}
	rows, err := s.runtimeContacts(principal.WorkspaceID, principal.Project)
	if err != nil {
		s.serverError(w, err)
		return
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
	resolved := make([]string, 0, len(recipients))
	for _, rec := range recipients {
		canonical, err := s.resolveRuntimeRecipient(principal, rec)
		if err != nil {
			s.jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		resolved = append(resolved, canonical)
	}
	from := runtimeAgentAddress(principal)
	sentAt := time.Now().UTC()
	ids := make([]string, 0, len(resolved))
	for _, rec := range resolved {
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
		After:        map[string]any{"to": resolved, "subject": strings.TrimSpace(body.Subject)},
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
