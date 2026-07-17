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
	project := r.PathValue("name")
	if !s.checkProjectAccess(w, r, project) {
		return
	}
	if _, err := s.st.Project(project); err != nil {
		if isNotFoundErr(err) {
			s.jsonError(w, http.StatusNotFound, "project not found")
			return
		}
		s.serverError(w, err)
		return
	}
	wfStore, ok := s.workflowStoreForRequest(w, r)
	if !ok {
		return
	}
	if err := wfStore.SeedDefaults(project); err != nil {
		s.serverError(w, err)
		return
	}
	defs, err := wfStore.ListDefinitions(project)
	if err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"workflows": defs})
}

func (s *Server) handleGetWorkflow(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("name")
	if !s.checkProjectAccess(w, r, project) {
		return
	}
	wfStore, ok := s.workflowStoreForRequest(w, r)
	if !ok {
		return
	}
	def, found, err := wfStore.Definition(project, r.PathValue("workflowId"))
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
	project := r.PathValue("name")
	if !s.checkProjectAccess(w, r, project) {
		return
	}
	if !s.canManageProject(r, project) {
		s.jsonError(w, http.StatusForbidden, "project manager access required")
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
	now := time.Now().UTC()
	def := entity.WorkflowDefinition{
		ID:          entity.NewWorkflowID(),
		Name:        name,
		Description: strings.TrimSpace(body.Description),
		Version:     1,
		Scope:       "project",
		Project:     project,
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
	def, found, err := wfStore.Definition(project, run.DefinitionID)
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
