package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/taskstore"
)

const maxJSONBody = 1 << 20 // 1 MiB

func (s *Server) readJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
}

func validTaskType(s string) bool {
	return entity.ValidTaskType(s)
}

func validTaskStatus(s string) bool {
	return entity.ValidTaskStatus(s)
}

type postTaskBody struct {
	Agent                 string                                 `json:"agent"`
	Title                 string                                 `json:"title"`
	Prompt                string                                 `json:"prompt"`
	Description           string                                 `json:"description"`
	Type                  string                                 `json:"type"`
	Priority              int                                    `json:"priority"`
	Assignee              string                                 `json:"assignee"`
	CreatedBy             string                                 `json:"createdBy"`
	Labels                []string                               `json:"labels"`
	ParentID              string                                 `json:"parentId"`
	DueDate               string                                 `json:"dueDate"`          // YYYY-MM-DD
	EstimateDuration      string                                 `json:"estimateDuration"` // Go duration, e.g. "30m", "2h"
	WorkflowDefinitionID  string                                 `json:"workflowDefinitionId"`
	WorkflowActorBindings map[string]entity.WorkflowActorBinding `json:"workflowActorBindings"`
}

func (s *Server) handlePostProjectTask(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var body postTaskBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	s.createProjectTaskFromBody(w, r, name, body)
}

func (s *Server) createProjectTaskFromBody(w http.ResponseWriter, r *http.Request, name string, body postTaskBody) {
	if !s.checkProjectAccess(w, r, name) {
		return
	}
	if _, err := s.st.Project(name); err != nil {
		if isNotFoundErr(err) {
			s.jsonError(w, http.StatusNotFound, "project not found")
			return
		}
		s.serverError(w, err)
		return
	}

	agentName := strings.TrimSpace(body.Agent)
	title := strings.TrimSpace(body.Title)
	promptText := strings.TrimSpace(body.Prompt)
	if agentName == "" || title == "" || promptText == "" {
		s.jsonError(w, http.StatusBadRequest, "agent, title, and prompt are required")
		return
	}
	if !s.agentExistsInProject(name, agentName) {
		s.jsonError(w, http.StatusBadRequest, "agent not found in this project")
		return
	}
	if !s.canOperateAgent(r, name, agentName) {
		s.jsonError(w, http.StatusForbidden, "agent operator access required")
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
		s.jsonError(w, http.StatusBadRequest, "priority must be 0–3")
		return
	}

	assignee := strings.TrimSpace(body.Assignee)
	if assignee == "" {
		assignee = name + "/" + agentName
	}
	workflowID := strings.TrimSpace(body.WorkflowDefinitionID)

	createdBy := strings.TrimSpace(body.CreatedBy)
	if createdBy == "" {
		if cur := s.currentUser(r); cur != nil && strings.TrimSpace(cur.Username) != "" {
			createdBy = cur.Username
		} else {
			createdBy = "system"
		}
	}

	now := time.Now().UTC()
	t := &entity.Task{
		ID:          entity.NewTaskID(),
		Title:       title,
		Description: strings.TrimSpace(body.Description),
		Type:        entity.TaskType(taskType),
		Priority:    priority,
		Assignee:    assignee,
		CreatedBy:   createdBy,
		Status:      entity.TaskStatusPending,
		Prompt:      promptText,
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
		if dd, err := time.Parse("2006-01-02", body.DueDate); err == nil {
			t.DueDate = &dd
		}
	}

	var workflowStore interface {
		Definition(string) (entity.WorkflowDefinition, bool, error)
		StartRun(string, string, string, map[string]entity.WorkflowActorBinding) (entity.WorkflowRun, []entity.WorkflowStepInstance, error)
	}
	if workflowID != "" {
		wfStore, ok := s.workflowStoreForRequest(w, r)
		if !ok {
			return
		}
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
		if _, inst, ok := workflowStartActor(def, body.WorkflowActorBindings); ok {
			switch inst.ActorType {
			case "agent":
				startAgent := strings.TrimSpace(inst.ActorID)
				if startAgent == "" || !s.agentExistsInProject(name, startAgent) {
					s.jsonError(w, http.StatusBadRequest, "workflow start agent not found in this project")
					return
				}
				agentName = startAgent
				assignee = name + "/" + startAgent
				t.Assignee = assignee
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
				t.Assignee = assignee
			}
		}
	}

	if assignee == "human" || !strings.Contains(assignee, "/") {
		if err := s.ts.AddTask(name, agentName, t); err != nil {
			s.serverError(w, err)
			return
		}
		item := &entity.InboxItem{
			TaskID:  t.ID,
			Project: name,
			Agent:   agentName,
			To:      assignee,
			Title:   t.Title,
			Summary: promptText,
		}
		if err := s.ts.AddToInbox(item); err != nil {
			s.serverError(w, err)
			return
		}
	} else {
		if err := s.validateIdentity(assignee, "assignee"); err != nil {
			s.jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.ts.AddTask(name, agentName, t); err != nil {
			s.serverError(w, err)
			return
		}
	}

	if workflowID != "" {
		if workflowStore == nil {
			s.serverError(w, fmt.Errorf("workflow store unavailable"))
			return
		}
		if _, _, err := workflowStore.StartRun(name, t.ID, workflowID, body.WorkflowActorBindings); err != nil {
			s.serverError(w, err)
			return
		}
	}

	if assignee != "human" {
		// Fire trigger if configured for the actual task owner. Workflow tasks
		// are routed to the start step actor, which can differ from the form's
		// initially selected agent.
		s.triggers.Fire(name, agentName, entity.TriggerOnTask, "task "+t.ID)
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"id":      t.ID,
		"project": name,
		"agent":   agentName,
	})
}

func workflowStartActor(def entity.WorkflowDefinition, bindings map[string]entity.WorkflowActorBinding) (*entity.WorkflowStep, *entity.WorkflowStepInstance, bool) {
	for i := range def.Steps {
		if def.Steps[i].ID != def.StartStepID {
			continue
		}
		inst := &entity.WorkflowStepInstance{
			StepID:    def.Steps[i].ID,
			Status:    "running",
			ActorType: "",
			ActorID:   "",
		}
		if binding, ok := bindings[def.Steps[i].ActorRole]; ok {
			inst.ActorType = strings.TrimSpace(binding.Type)
			inst.ActorID = strings.TrimSpace(binding.ID)
		}
		return &def.Steps[i], inst, true
	}
	return nil, nil, false
}

type taskActionBody struct {
	Project string `json:"project"`
	Agent   string `json:"agent"`
	ID      string `json:"id"`
}

func (s *Server) handlePostCancelTask(w http.ResponseWriter, r *http.Request) {
	var body taskActionBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	project := strings.TrimSpace(body.Project)
	agent := strings.TrimSpace(body.Agent)
	id := strings.TrimSpace(body.ID)
	if project == "" || agent == "" || id == "" {
		s.jsonError(w, http.StatusBadRequest, "project, agent, and id are required")
		return
	}
	if !s.checkProjectAccess(w, r, project) {
		return
	}
	if !s.canOperateAgent(r, project, agent) {
		s.jsonError(w, http.StatusForbidden, "agent operator access required")
		return
	}
	t, err := s.ts.GetTask(project, agent, id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.jsonError(w, http.StatusNotFound, "task not found")
			return
		}
		s.serverError(w, err)
		return
	}
	if t.Status.IsTerminal() {
		s.jsonError(w, http.StatusBadRequest, "task is already in terminal state")
		return
	}
	prev := t.Status
	now := time.Now().UTC()
	t.Status = entity.TaskStatusCancelled
	t.UpdatedAt = now
	entity.ApplyStatusTimestamps(t, prev, now)
	if err := s.ts.UpdateTask(project, agent, t); err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *Server) handlePostArchiveTask(w http.ResponseWriter, r *http.Request) {
	var body taskActionBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	project := strings.TrimSpace(body.Project)
	agent := strings.TrimSpace(body.Agent)
	id := strings.TrimSpace(body.ID)
	if project == "" || agent == "" || id == "" {
		s.jsonError(w, http.StatusBadRequest, "project, agent, and id are required")
		return
	}
	if !s.checkProjectAccess(w, r, project) {
		return
	}
	if !s.canOperateAgent(r, project, agent) {
		s.jsonError(w, http.StatusForbidden, "agent operator access required")
		return
	}
	t, err := s.ts.GetTask(project, agent, id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.jsonError(w, http.StatusNotFound, "task not found")
			return
		}
		s.serverError(w, err)
		return
	}
	if err := s.ts.ArchiveTask(project, agent, t); err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

type updateTaskBody struct {
	Project          string    `json:"project"`
	Agent            string    `json:"agent"`
	ID               string    `json:"id"`
	Title            *string   `json:"title,omitempty"`
	Description      *string   `json:"description,omitempty"`
	Status           *string   `json:"status,omitempty"`
	Priority         *int      `json:"priority,omitempty"`
	Type             *string   `json:"type,omitempty"`
	Summary          *string   `json:"summary,omitempty"`
	Labels           *[]string `json:"labels,omitempty"`
	ParentID         *string   `json:"parentId,omitempty"`
	DueDate          *string   `json:"dueDate,omitempty"`          // YYYY-MM-DD or "" to clear
	EstimateDuration *string   `json:"estimateDuration,omitempty"` // Go duration or "" to clear
	Position         *float64  `json:"position,omitempty"`
	Assignee         *string   `json:"assignee,omitempty"`
	Prompt           *string   `json:"prompt,omitempty"`
}

func (s *Server) handlePutUpdateTask(w http.ResponseWriter, r *http.Request) {
	var body updateTaskBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	project := strings.TrimSpace(body.Project)
	agent := strings.TrimSpace(body.Agent)
	id := strings.TrimSpace(body.ID)
	if project == "" || agent == "" || id == "" {
		s.jsonError(w, http.StatusBadRequest, "project, agent, and id are required")
		return
	}
	if !s.checkProjectAccess(w, r, project) {
		return
	}
	if !s.canOperateAgent(r, project, agent) {
		s.jsonError(w, http.StatusForbidden, "agent operator access required")
		return
	}
	hasUpdate := body.Status != nil || body.Priority != nil || body.Type != nil || body.Summary != nil ||
		body.Title != nil || body.Description != nil || body.Labels != nil || body.ParentID != nil ||
		body.DueDate != nil || body.EstimateDuration != nil || body.Position != nil || body.Assignee != nil || body.Prompt != nil
	if !hasUpdate {
		s.jsonError(w, http.StatusBadRequest, "at least one field to update is required")
		return
	}

	t, err := s.ts.GetTask(project, agent, id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.jsonError(w, http.StatusNotFound, "task not found")
			return
		}
		s.serverError(w, err)
		return
	}

	patch := taskstore.TaskPatch{
		Title: body.Title, Description: body.Description, Summary: body.Summary,
		Labels: body.Labels, ParentID: body.ParentID, DueDate: body.DueDate,
		EstimateDuration: body.EstimateDuration, Position: body.Position,
		Assignee: body.Assignee, Prompt: body.Prompt,
	}
	if body.Status != nil {
		st := strings.TrimSpace(*body.Status)
		if st == "" {
			s.jsonError(w, http.StatusBadRequest, "status cannot be empty")
			return
		}
		if !validTaskStatus(st) {
			s.jsonError(w, http.StatusBadRequest, "invalid task status")
			return
		}
		status := entity.TaskStatus(st)
		patch.Status = &status
	}
	if body.Priority != nil {
		p := *body.Priority
		if p < 0 || p > 3 {
			s.jsonError(w, http.StatusBadRequest, "priority must be 0–3")
			return
		}
		patch.Priority = &p
	}
	if body.Type != nil {
		typ := strings.TrimSpace(*body.Type)
		if typ == "" {
			s.jsonError(w, http.StatusBadRequest, "type cannot be empty")
			return
		}
		if !validTaskType(typ) {
			s.jsonError(w, http.StatusBadRequest, "invalid task type")
			return
		}
		taskType := entity.TaskType(typ)
		patch.Type = &taskType
	}

	if _, err := taskstore.ApplyTaskPatch(t, patch, time.Now().UTC()); err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.ts.PersistTask(project, agent, t); err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.jsonError(w, http.StatusNotFound, "task not found")
			return
		}
		s.serverError(w, err)
		return
	}

	if patch.Status != nil && t.Status.IsTerminal() && t.CreatedBy != "" {
		s.notifyTaskDone(t, project, agent)
	}

	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *Server) handlePostDeleteTask(w http.ResponseWriter, r *http.Request) {
	var body taskActionBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	project := strings.TrimSpace(body.Project)
	agent := strings.TrimSpace(body.Agent)
	id := strings.TrimSpace(body.ID)
	if project == "" || agent == "" || id == "" {
		s.jsonError(w, http.StatusBadRequest, "project, agent, and id are required")
		return
	}
	if !s.checkProjectManager(w, r, project) {
		return
	}
	if err := s.ts.DeleteTask(project, agent, id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.jsonError(w, http.StatusNotFound, "task not found")
			return
		}
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

type postMessageBody struct {
	From    string `json:"from"`
	To      any    `json:"to"` // string or []any (strings)
	Subject string `json:"subject"`
	Body    string `json:"body"`
	ReplyTo string `json:"replyTo"`
}

func normalizeToRecipients(v any) ([]string, error) {
	if v == nil {
		return nil, fmt.Errorf("to is required")
	}
	switch x := v.(type) {
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return nil, fmt.Errorf("to is required")
		}
		return []string{s}, nil
	case []any:
		var out []string
		for _, e := range x {
			s, ok := e.(string)
			if !ok {
				return nil, fmt.Errorf("to must be strings")
			}
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("to is required")
		}
		return out, nil
	default:
		return nil, fmt.Errorf("to must be a string or array of strings")
	}
}

func (s *Server) handlePostMessage(w http.ResponseWriter, r *http.Request) {
	var body postMessageBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	bodyText := strings.TrimSpace(body.Body)
	if bodyText == "" {
		s.jsonError(w, http.StatusBadRequest, "body is required")
		return
	}
	from := strings.TrimSpace(body.From)
	if from == "" {
		s.jsonError(w, http.StatusBadRequest, "from is required")
		return
	}
	if err := s.validateIdentity(from, "from"); err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	recipients, err := normalizeToRecipients(body.To)
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	for _, rec := range recipients {
		if err := s.validateIdentity(rec, "to"); err != nil {
			s.jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if !s.canSendMessageAs(r, from) {
		s.jsonError(w, http.StatusForbidden, "sender access required")
		return
	}
	for _, rec := range recipients {
		if !s.canSendMessageTo(r, rec) {
			s.jsonError(w, http.StatusForbidden, "recipient access required")
			return
		}
	}

	sentAt := time.Now().UTC()
	ids := make([]string, 0, len(recipients))
	replyTo := strings.TrimSpace(body.ReplyTo)
	subject := strings.TrimSpace(body.Subject)

	for _, recipient := range recipients {
		msg := &entity.Message{
			ID:      entity.NewMessageID(),
			From:    from,
			To:      recipient,
			Subject: subject,
			Body:    bodyText,
			ReplyTo: replyTo,
			SentAt:  sentAt,
		}
		if err := s.ts.SendMessage(msg); err != nil {
			s.serverError(w, fmt.Errorf("send to %s: %w", recipient, err))
			return
		}
		ids = append(ids, msg.ID)

		// Fire trigger if configured for the recipient agent.
		if parts := strings.SplitN(recipient, "/", 2); len(parts) == 2 {
			s.triggers.Fire(parts[0], parts[1], entity.TriggerOnMessage, "from "+from)
		}
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"ids": ids})
}

type markReadBody struct {
	Mailbox string `json:"mailbox"`
	ID      string `json:"id"`
}

func (s *Server) canSendMessageAs(r *http.Request, sender string) bool {
	cur := s.currentUser(r)
	if cur.Role == RoleAdmin || s.canAdminCurrentWorkspace(r) {
		return true
	}
	if sender == cur.Username || sender == "human" {
		return true
	}
	project, agent, ok := splitAgentMailbox(sender)
	return ok && s.canOperateAgent(r, project, agent)
}

func (s *Server) canSendMessageTo(r *http.Request, recipient string) bool {
	if recipient == "human" {
		return true
	}
	cur := s.currentUser(r)
	if recipient == cur.Username {
		return true
	}
	project, agent, ok := splitAgentMailbox(recipient)
	if !ok {
		return true
	}
	return s.canOperateAgent(r, project, agent)
}

func splitAgentMailbox(mailbox string) (string, string, bool) {
	parts := strings.SplitN(mailbox, "/", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func (s *Server) canOperateMailbox(r *http.Request, mailbox string) bool {
	cur := s.currentUser(r)
	if cur.Role == RoleAdmin || s.canAdminCurrentWorkspace(r) {
		return true
	}
	if mailbox == cur.Username {
		return true
	}
	project, agent, ok := splitAgentMailbox(mailbox)
	return ok && s.canOperateAgent(r, project, agent)
}

func (s *Server) handlePostMarkMessageRead(w http.ResponseWriter, r *http.Request) {
	var body markReadBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	mailbox := strings.TrimSpace(body.Mailbox)
	id := strings.TrimSpace(body.ID)
	if mailbox == "" || id == "" {
		s.jsonError(w, http.StatusBadRequest, "mailbox and id are required")
		return
	}
	if err := s.validateIdentity(mailbox, "mailbox"); err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !s.canOperateMailbox(r, mailbox) {
		s.jsonError(w, http.StatusForbidden, "mailbox operator access required")
		return
	}
	if err := s.ts.MarkMessageRead(mailbox, id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.jsonError(w, http.StatusNotFound, "message not found")
			return
		}
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *Server) handlePostArchiveMessage(w http.ResponseWriter, r *http.Request) {
	var body markReadBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	mailbox := strings.TrimSpace(body.Mailbox)
	id := strings.TrimSpace(body.ID)
	if mailbox == "" || id == "" {
		s.jsonError(w, http.StatusBadRequest, "mailbox and id are required")
		return
	}
	if err := s.validateIdentity(mailbox, "mailbox"); err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !s.canOperateMailbox(r, mailbox) {
		s.jsonError(w, http.StatusForbidden, "mailbox operator access required")
		return
	}
	if err := s.ts.ArchiveMessage(mailbox, id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.jsonError(w, http.StatusNotFound, "message not found")
			return
		}
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *Server) handlePostDeleteMessage(w http.ResponseWriter, r *http.Request) {
	var body markReadBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	mailbox := strings.TrimSpace(body.Mailbox)
	id := strings.TrimSpace(body.ID)
	if mailbox == "" || id == "" {
		s.jsonError(w, http.StatusBadRequest, "mailbox and id are required")
		return
	}
	if err := s.validateIdentity(mailbox, "mailbox"); err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !s.canOperateMailbox(r, mailbox) {
		s.jsonError(w, http.StatusForbidden, "mailbox operator access required")
		return
	}
	if err := s.ts.DeleteMessage(mailbox, id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.jsonError(w, http.StatusNotFound, "message not found")
			return
		}
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

type markAllMailboxBody struct {
	Mailbox string `json:"mailbox"`
}

func (s *Server) handlePostMarkAllMessagesRead(w http.ResponseWriter, r *http.Request) {
	var body markAllMailboxBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	mailbox := strings.TrimSpace(body.Mailbox)
	if mailbox == "" {
		s.jsonError(w, http.StatusBadRequest, "mailbox is required")
		return
	}
	if err := s.validateIdentity(mailbox, "mailbox"); err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !s.canOperateMailbox(r, mailbox) {
		s.jsonError(w, http.StatusForbidden, "mailbox operator access required")
		return
	}
	if err := s.ts.MarkMessagesRead(mailbox); err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

type markAllProjectBody struct {
	// Mailbox is optional "project/agent" scoped to this project; empty = all agent mailboxes in project.
	Mailbox string `json:"mailbox"`
}

func (s *Server) handlePostProjectMarkAllMessagesRead(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !s.checkProjectAccess(w, r, name) {
		return
	}
	if _, err := s.st.Project(name); err != nil {
		if isNotFoundErr(err) {
			s.jsonError(w, http.StatusNotFound, "project not found")
			return
		}
		s.serverError(w, err)
		return
	}

	var body markAllProjectBody
	if r.ContentLength > 0 {
		if err := s.readJSON(w, r, &body); err != nil {
			s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}
	mailbox := strings.TrimSpace(body.Mailbox)
	if mailbox != "" {
		if err := s.validateIdentity(mailbox, "mailbox"); err != nil {
			s.jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		parts := strings.SplitN(mailbox, "/", 2)
		if len(parts) != 2 || parts[0] != name {
			s.jsonError(w, http.StatusBadRequest, "mailbox must be an agent in this project")
			return
		}
		if !s.agentExistsInProject(name, parts[1]) {
			s.jsonError(w, http.StatusBadRequest, "agent not found in this project")
			return
		}
		if !s.canOperateAgent(r, name, parts[1]) {
			s.jsonError(w, http.StatusForbidden, "agent operator access required")
			return
		}
		if err := s.ts.MarkMessagesRead(mailbox); err != nil {
			s.serverError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "mailboxes": []string{mailbox}})
		return
	}

	agents, err := s.st.ListAgents(name)
	if err != nil {
		s.serverError(w, err)
		return
	}
	mailboxes := make([]string, 0, len(agents))
	for _, ag := range agents {
		if !s.canOperateAgent(r, name, ag.Name) {
			continue
		}
		mb := name + "/" + ag.Name
		mailboxes = append(mailboxes, mb)
		if err := s.ts.MarkMessagesRead(mb); err != nil {
			s.serverError(w, err)
			return
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "mailboxes": mailboxes})
}

func (s *Server) notifyTaskDone(t *entity.Task, project, agent string) {
	assignee := t.Assignee
	if assignee == "" {
		assignee = project + "/" + agent
	}
	if assignee == t.CreatedBy {
		return
	}

	statusLabel := "completed"
	if t.Status == entity.TaskStatusDoneFailed {
		statusLabel = "failed"
	} else if t.Status == entity.TaskStatusCancelled {
		statusLabel = "cancelled"
	}

	body := fmt.Sprintf("Task **%s** (`%s`) has been marked as **%s** by `%s`.",
		t.Title, t.ID, statusLabel, assignee)
	if t.Summary != "" {
		body += "\n\n**Summary:** " + t.Summary
	}

	msg := &entity.Message{
		ID:      entity.NewMessageID(),
		From:    assignee,
		To:      t.CreatedBy,
		Subject: fmt.Sprintf("[Task %s] %s", statusLabel, t.Title),
		Body:    body,
		SentAt:  time.Now().UTC(),
	}
	_ = s.ts.SendMessage(msg)
}

// ── Task Comments ─────────────────────────────────────────────────────────────

func (s *Server) handleGetComments(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	agent := r.PathValue("agent")
	taskID := r.PathValue("taskId")
	if !s.checkProjectAccess(w, r, project) {
		return
	}
	comments, err := s.ts.ListComments(project, agent, taskID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if comments == nil {
		comments = []*entity.TaskComment{}
	}
	_ = json.NewEncoder(w).Encode(comments)
}

func (s *Server) handlePostComment(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	agent := r.PathValue("agent")
	taskID := r.PathValue("taskId")
	if !s.canOperateAgent(r, project, agent) {
		s.jsonError(w, http.StatusForbidden, "agent operator access required")
		return
	}
	var body struct {
		Author string `json:"author"`
		Body   string `json:"body"`
	}
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	body.Body = strings.TrimSpace(body.Body)
	body.Author = strings.TrimSpace(body.Author)
	if body.Body == "" {
		s.jsonError(w, http.StatusBadRequest, "body is required")
		return
	}
	if body.Author == "" {
		cur := s.currentUser(r)
		body.Author = cur.Username
	}
	c := &entity.TaskComment{
		ID:        entity.NewCommentID(),
		TaskID:    taskID,
		Author:    body.Author,
		Body:      body.Body,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.ts.AddComment(project, agent, c); err != nil {
		s.serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(c)
}

func (s *Server) handleDeleteComment(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	agent := r.PathValue("agent")
	commentID := r.PathValue("commentId")
	if !s.canOperateAgent(r, project, agent) {
		s.jsonError(w, http.StatusForbidden, "agent operator access required")
		return
	}
	if err := s.ts.DeleteComment(project, agent, commentID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.jsonError(w, http.StatusNotFound, err.Error())
			return
		}
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *Server) handleFireAgent(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("name")
	agent := r.PathValue("agent")
	if project == "" || agent == "" {
		s.jsonError(w, http.StatusBadRequest, "project and agent are required")
		return
	}
	if !s.checkProjectManager(w, r, project) {
		return
	}
	if _, err := s.st.AgentMeta(project, agent); err != nil {
		s.jsonError(w, http.StatusNotFound, "agent not found")
		return
	}

	force := r.URL.Query().Get("force") == "true"
	agentDir := s.st.AgentDir(project, agent)

	if force {
		if err := s.st.DeleteAgentMeta(project, agent); err != nil {
			s.serverError(w, fmt.Errorf("fire: %w", err))
			return
		}
	} else {
		timestamp := time.Now().UTC().Format("20060102-150405")
		firedDirName := agent + "-" + timestamp
		firedDir := s.st.FiredAgentDir(project, firedDirName)
		if err := os.MkdirAll(filepath.Dir(firedDir), 0o755); err != nil {
			s.serverError(w, fmt.Errorf("fire: %w", err))
			return
		}
		if err := os.Rename(agentDir, firedDir); err != nil {
			s.serverError(w, fmt.Errorf("fire: %w", err))
			return
		}
		if err := s.st.DeleteAgentMeta(project, agent); err != nil {
			s.serverError(w, fmt.Errorf("fire: %w", err))
			return
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
