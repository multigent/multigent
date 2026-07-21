package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/tasktemplate"
)

var taskTemplateVarPattern = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_.-]+)\s*\}\}`)

type taskTemplateCreateBody struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Project     string   `json:"project"`
	Type        string   `json:"type"`
	Priority    int      `json:"priority"`
	Labels      []string `json:"labels"`

	TitleTemplate       string `json:"titleTemplate"`
	DescriptionTemplate string `json:"descriptionTemplate"`
	PromptTemplate      string `json:"promptTemplate"`

	WorkflowDefinitionID  string                                 `json:"workflowDefinitionId"`
	WorkflowActorBindings map[string]entity.WorkflowActorBinding `json:"workflowActorBindings"`
	Variables             []entity.TaskTemplateVariable          `json:"variables"`
}

type taskFromTemplateBody struct {
	TemplateID            string                                 `json:"templateId"`
	Inputs                map[string]string                      `json:"inputs"`
	Agent                 string                                 `json:"agent"`
	Assignee              string                                 `json:"assignee"`
	Priority              *int                                   `json:"priority"`
	DueDate               string                                 `json:"dueDate"`
	EstimateDuration      string                                 `json:"estimateDuration"`
	ParentID              string                                 `json:"parentId"`
	Labels                []string                               `json:"labels"`
	WorkflowActorBindings map[string]entity.WorkflowActorBinding `json:"workflowActorBindings"`
}

func (s *Server) taskTemplateStoreForRequest(w http.ResponseWriter, r *http.Request) (*tasktemplate.Store, bool) {
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return nil, false
	}
	if !s.checkWorkspaceAccess(w, r, workspaceID) {
		return nil, false
	}
	return tasktemplate.NewStore(s.controlDB, workspaceID), true
}

func (s *Server) handleListTaskTemplates(w http.ResponseWriter, r *http.Request) {
	store, ok := s.taskTemplateStoreForRequest(w, r)
	if !ok {
		return
	}
	templates, err := store.List()
	if err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"templates": templates})
}

func (s *Server) handleListProjectTaskTemplates(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("name")
	if !s.checkProjectAccess(w, r, project) {
		return
	}
	store, ok := s.taskTemplateStoreForRequest(w, r)
	if !ok {
		return
	}
	templates, err := store.List()
	if err != nil {
		s.serverError(w, err)
		return
	}
	filtered := make([]entity.TaskTemplate, 0, len(templates))
	for _, template := range templates {
		if template.Project == project {
			filtered = append(filtered, template)
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"templates": filtered})
}

func (s *Server) handleCreateTaskTemplate(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	var body taskTemplateCreateBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	template, ok := taskTemplateFromBody(w, s, body, "")
	if !ok {
		return
	}
	store, ok := s.taskTemplateStoreForRequest(w, r)
	if !ok {
		return
	}
	if err := store.Save(&template); err != nil {
		s.serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(template)
}

func (s *Server) handleCreateProjectTaskTemplate(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("name")
	if !s.checkProjectManager(w, r, project) {
		return
	}
	var body taskTemplateCreateBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	body.Project = project
	template, ok := taskTemplateFromBody(w, s, body, "")
	if !ok {
		return
	}
	store, ok := s.taskTemplateStoreForRequest(w, r)
	if !ok {
		return
	}
	if err := store.Save(&template); err != nil {
		s.serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(template)
}

func (s *Server) handleGetTaskTemplate(w http.ResponseWriter, r *http.Request) {
	store, ok := s.taskTemplateStoreForRequest(w, r)
	if !ok {
		return
	}
	template, found, err := store.Get(r.PathValue("templateId"))
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found {
		s.jsonError(w, http.StatusNotFound, "task template not found")
		return
	}
	_ = json.NewEncoder(w).Encode(template)
}

func (s *Server) handleUpdateTaskTemplate(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	store, ok := s.taskTemplateStoreForRequest(w, r)
	if !ok {
		return
	}
	templateID := r.PathValue("templateId")
	existing, found, err := store.Get(templateID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found {
		s.jsonError(w, http.StatusNotFound, "task template not found")
		return
	}
	var body taskTemplateCreateBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	template, valid := taskTemplateFromBody(w, s, body, templateID)
	if !valid {
		return
	}
	template.CreatedAt = existing.CreatedAt
	if err := store.Save(&template); err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(template)
}

func (s *Server) handleDeleteTaskTemplate(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	store, ok := s.taskTemplateStoreForRequest(w, r)
	if !ok {
		return
	}
	if err := store.Delete(r.PathValue("templateId")); err != nil {
		s.serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func taskTemplateFromBody(w http.ResponseWriter, s *Server, body taskTemplateCreateBody, id string) (entity.TaskTemplate, bool) {
	name := strings.TrimSpace(body.Name)
	titleTemplate := strings.TrimSpace(body.TitleTemplate)
	promptTemplate := strings.TrimSpace(body.PromptTemplate)
	if name == "" || titleTemplate == "" || promptTemplate == "" {
		s.jsonError(w, http.StatusBadRequest, "name, titleTemplate, and promptTemplate are required")
		return entity.TaskTemplate{}, false
	}
	taskType := strings.TrimSpace(body.Type)
	if taskType == "" {
		taskType = string(entity.TaskTypeChore)
	}
	if !validTaskType(taskType) {
		s.jsonError(w, http.StatusBadRequest, "invalid task type")
		return entity.TaskTemplate{}, false
	}
	priority := body.Priority
	if priority < 0 || priority > 3 {
		s.jsonError(w, http.StatusBadRequest, "priority must be 0-3")
		return entity.TaskTemplate{}, false
	}
	return entity.TaskTemplate{
		ID:                    strings.TrimSpace(id),
		Name:                  name,
		Description:           strings.TrimSpace(body.Description),
		Project:               strings.TrimSpace(body.Project),
		Type:                  taskType,
		Priority:              priority,
		Labels:                body.Labels,
		TitleTemplate:         titleTemplate,
		DescriptionTemplate:   strings.TrimSpace(body.DescriptionTemplate),
		PromptTemplate:        promptTemplate,
		WorkflowDefinitionID:  strings.TrimSpace(body.WorkflowDefinitionID),
		WorkflowActorBindings: body.WorkflowActorBindings,
		Variables:             normalizeTaskTemplateVariables(body.Variables),
	}, true
}

func normalizeTaskTemplateVariables(vars []entity.TaskTemplateVariable) []entity.TaskTemplateVariable {
	out := make([]entity.TaskTemplateVariable, 0, len(vars))
	seen := map[string]bool{}
	for _, item := range vars {
		name := strings.TrimSpace(item.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, entity.TaskTemplateVariable{
			Name:        name,
			Description: strings.TrimSpace(item.Description),
			Required:    item.Required,
			Default:     strings.TrimSpace(item.Default),
		})
	}
	return out
}

func renderTaskTemplateString(template string, values map[string]string) string {
	return taskTemplateVarPattern.ReplaceAllStringFunc(template, func(match string) string {
		parts := taskTemplateVarPattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		return values[strings.TrimSpace(parts[1])]
	})
}

func instantiateTaskTemplate(template entity.TaskTemplate, body taskFromTemplateBody) (postTaskBody, error) {
	values := map[string]string{}
	for _, variable := range template.Variables {
		if variable.Default != "" {
			values[variable.Name] = variable.Default
		}
	}
	for key, value := range body.Inputs {
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	for _, variable := range template.Variables {
		if variable.Required && strings.TrimSpace(values[variable.Name]) == "" {
			return postTaskBody{}, fmt.Errorf("template variable %q is required", variable.Name)
		}
	}
	actorBindings := template.WorkflowActorBindings
	if len(body.WorkflowActorBindings) > 0 {
		actorBindings = body.WorkflowActorBindings
	}
	priority := template.Priority
	if body.Priority != nil {
		priority = *body.Priority
	}
	labels := append([]string{}, template.Labels...)
	labels = append(labels, body.Labels...)
	return postTaskBody{
		Agent:                 strings.TrimSpace(body.Agent),
		Title:                 strings.TrimSpace(renderTaskTemplateString(template.TitleTemplate, values)),
		Description:           strings.TrimSpace(renderTaskTemplateString(template.DescriptionTemplate, values)),
		Prompt:                strings.TrimSpace(renderTaskTemplateString(template.PromptTemplate, values)),
		Type:                  template.Type,
		Priority:              priority,
		Assignee:              strings.TrimSpace(body.Assignee),
		Labels:                labels,
		ParentID:              strings.TrimSpace(body.ParentID),
		DueDate:               strings.TrimSpace(body.DueDate),
		EstimateDuration:      strings.TrimSpace(body.EstimateDuration),
		WorkflowDefinitionID:  template.WorkflowDefinitionID,
		WorkflowActorBindings: actorBindings,
	}, nil
}

func (s *Server) handlePostProjectTaskFromTemplate(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("name")
	var body taskFromTemplateBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	store, ok := s.taskTemplateStoreForRequest(w, r)
	if !ok {
		return
	}
	template, found, err := store.Get(strings.TrimSpace(body.TemplateID))
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found {
		s.jsonError(w, http.StatusNotFound, "task template not found")
		return
	}
	if template.Project != project {
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
	s.createProjectTaskFromBody(w, r, project, taskBody)
}

func firstTemplateAgentBinding(bindings map[string]entity.WorkflowActorBinding) string {
	for _, binding := range bindings {
		if binding.Type == "agent" && strings.TrimSpace(binding.ID) != "" {
			return strings.TrimSpace(binding.ID)
		}
	}
	return ""
}
