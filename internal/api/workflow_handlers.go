package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/multigent/multigent/internal/entity"
	workflowstore "github.com/multigent/multigent/internal/workflow"
)

type workflowCreateBody struct {
	Name        string                `json:"name"`
	Description string                `json:"description"`
	TemplateID  string                `json:"templateId"`
	Locale      string                `json:"locale"`
	StartStepID string                `json:"startStepId"`
	Steps       []entity.WorkflowStep `json:"steps"`
	Edges       []entity.WorkflowEdge `json:"edges"`
}

type taskWorkflowResponse struct {
	Definition entity.WorkflowDefinition     `json:"definition"`
	Run        entity.WorkflowRun            `json:"run"`
	Steps      []entity.WorkflowStepInstance `json:"steps"`
}

func (s *Server) workflowStoreForRequest(w http.ResponseWriter, r *http.Request) (*workflowstore.Store, bool) {
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return nil, false
	}
	if !s.checkWorkspaceAccess(w, r, workspaceID) {
		return nil, false
	}
	return workflowstore.NewStore(s.controlDB, workspaceID), true
}

func (s *Server) handleListWorkflows(w http.ResponseWriter, r *http.Request) {
	wfStore, ok := s.workflowStoreForRequest(w, r)
	if !ok {
		return
	}
	defs, err := wfStore.ListDefinitions()
	if err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"workflows": defs})
}

func (s *Server) handleListWorkflowTemplates(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.workflowStoreForRequest(w, r); !ok {
		return
	}
	locale := strings.TrimSpace(r.URL.Query().Get("locale"))
	_ = json.NewEncoder(w).Encode(map[string]any{"templates": workflowstore.Templates(locale)})
}

func (s *Server) handleGetWorkflow(w http.ResponseWriter, r *http.Request) {
	wfStore, ok := s.workflowStoreForRequest(w, r)
	if !ok {
		return
	}
	def, found, err := wfStore.Definition(r.PathValue("workflowId"))
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found {
		s.jsonError(w, http.StatusNotFound, "workflow not found")
		return
	}
	_ = json.NewEncoder(w).Encode(def)
}

func (s *Server) handleCreateWorkflow(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	var body workflowCreateBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	name := strings.TrimSpace(body.Name)
	if strings.TrimSpace(body.TemplateID) != "" {
		def, found := workflowstore.DefinitionFromTemplate(body.TemplateID, body.Locale, name)
		if !found {
			s.jsonError(w, http.StatusNotFound, "workflow template not found")
			return
		}
		wfStore, ok := s.workflowStoreForRequest(w, r)
		if !ok {
			return
		}
		if err := wfStore.SaveDefinition(&def); err != nil {
			s.serverError(w, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(def)
		return
	}
	if name == "" {
		s.jsonError(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(body.Steps) == 0 {
		s.jsonError(w, http.StatusBadRequest, "at least one step is required")
		return
	}
	start := strings.TrimSpace(body.StartStepID)
	if start == "" {
		start = body.Steps[0].ID
	}
	now := time.Now().UTC()
	def := entity.WorkflowDefinition{
		ID:          entity.NewWorkflowID(),
		Name:        name,
		Description: strings.TrimSpace(body.Description),
		Version:     1,
		Scope:       "workspace",
		StartStepID: start,
		Steps:       body.Steps,
		Edges:       body.Edges,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	wfStore, ok := s.workflowStoreForRequest(w, r)
	if !ok {
		return
	}
	if err := wfStore.SaveDefinition(&def); err != nil {
		s.serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(def)
}

func (s *Server) handleUpdateWorkflow(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	wfStore, ok := s.workflowStoreForRequest(w, r)
	if !ok {
		return
	}
	workflowID := r.PathValue("workflowId")
	existing, found, err := wfStore.Definition(workflowID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found || existing.Scope != "workspace" || existing.Project != "" {
		s.jsonError(w, http.StatusNotFound, "workflow not found")
		return
	}
	var body workflowCreateBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		s.jsonError(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(body.Steps) == 0 {
		s.jsonError(w, http.StatusBadRequest, "at least one step is required")
		return
	}
	start := strings.TrimSpace(body.StartStepID)
	if start == "" {
		start = body.Steps[0].ID
	}
	existing.Name = name
	existing.Description = strings.TrimSpace(body.Description)
	existing.StartStepID = start
	existing.Steps = body.Steps
	existing.Edges = body.Edges
	existing.Scope = "workspace"
	existing.Project = ""
	existing.Version++
	if err := wfStore.SaveDefinition(&existing); err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(existing)
}

func (s *Server) handleDeleteWorkflow(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	wfStore, ok := s.workflowStoreForRequest(w, r)
	if !ok {
		return
	}
	workflowID := r.PathValue("workflowId")
	existing, found, err := wfStore.Definition(workflowID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found || existing.Scope != "workspace" || existing.Project != "" {
		s.jsonError(w, http.StatusNotFound, "workflow not found")
		return
	}
	if err := wfStore.DeleteDefinition(workflowID); err != nil {
		s.serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetTaskWorkflow(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("name")
	taskID := r.PathValue("taskId")
	if !s.checkProjectAccess(w, r, project) {
		return
	}
	wfStore, ok := s.workflowStoreForRequest(w, r)
	if !ok {
		return
	}
	run, found, err := wfStore.RunForTask(project, taskID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found {
		s.jsonError(w, http.StatusNotFound, "workflow run not found")
		return
	}
	def, found, err := wfStore.Definition(run.DefinitionID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found {
		s.jsonError(w, http.StatusNotFound, "workflow definition not found")
		return
	}
	steps, err := wfStore.ListStepInstances(run.ID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(taskWorkflowResponse{Definition: def, Run: run, Steps: steps})
}

func (s *Server) handleRuntimeTaskWorkflow(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.runtimeRequireCapability(w, r, "task.use")
	if !ok {
		return
	}
	t, _, _, err := s.runtimeFindTask(principal, r.PathValue("id"), r.URL.Query().Get("agent"))
	if err != nil || t == nil {
		s.jsonError(w, http.StatusNotFound, "task not found")
		return
	}
	wfStore := workflowstore.NewStore(s.controlDB, principal.WorkspaceID)
	run, found, err := wfStore.RunForTask(principal.Project, t.ID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found {
		s.jsonError(w, http.StatusNotFound, "workflow run not found")
		return
	}
	def, found, err := wfStore.Definition(run.DefinitionID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found {
		s.jsonError(w, http.StatusNotFound, "workflow definition not found")
		return
	}
	steps, err := wfStore.ListStepInstances(run.ID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(taskWorkflowResponse{Definition: def, Run: run, Steps: steps})
}
