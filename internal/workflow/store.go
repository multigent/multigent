package workflow

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/errs"
)

type Store struct {
	db          controldb.Store
	workspaceID string
}

func NewStore(db controldb.Store, workspaceID string) *Store {
	return &Store{db: db, workspaceID: workspaceID}
}

func (s *Store) SeedDefaults() error {
	if def, ok, err := s.Definition("software-delivery-v1"); err != nil {
		return err
	} else if ok && def.Scope == "workspace" && def.Project == "" {
		return nil
	}
	now := time.Now().UTC()
	def := entity.WorkflowDefinition{
		ID:          "software-delivery-v1",
		Name:        "Software Delivery",
		Description: "A practical human-agent workflow for product clarification, implementation, review, QA, and release.",
		Version:     1,
		Scope:       "workspace",
		StartStepID: "intake",
		CreatedAt:   now,
		UpdatedAt:   now,
		Steps: []entity.WorkflowStep{
			{ID: "intake", Type: "agent_task", Title: "Intake", Description: "Clarify the request and collect missing context.", ActorRole: "pm", OutputSchema: "problem, goal, scope, non-goals, acceptance criteria", Position: entity.WorkflowPosition{X: 80, Y: 180}},
			{ID: "spec_review", Type: "human_review", Title: "Spec Review", Description: "A human owner reviews scope and acceptance criteria.", ActorRole: "owner", ReviewPolicy: "manual", Position: entity.WorkflowPosition{X: 360, Y: 180}},
			{ID: "branch_plan", Type: "branch", Title: "Branch Plan", Description: "Decide whether this task stays single-path or splits into child tasks.", ActorRole: "pm", OutputSchema: "split_required, branches, join_policy", Position: entity.WorkflowPosition{X: 640, Y: 180}},
			{ID: "implementation", Type: "agent_task", Title: "Implementation", Description: "Implement the approved scope or execute the child task branch.", ActorRole: "developer", OutputSchema: "summary, changed files, tests run, risks", Position: entity.WorkflowPosition{X: 920, Y: 90}},
			{ID: "qa", Type: "agent_task", Title: "QA", Description: "Produce and execute test cases, then report release risk.", ActorRole: "qa", OutputSchema: "test cases, test results, known issues", Position: entity.WorkflowPosition{X: 920, Y: 270}},
			{ID: "join", Type: "join", Title: "Join", Description: "Merge branch outputs and decide whether the parent task can continue.", ReviewPolicy: "all_success", Position: entity.WorkflowPosition{X: 1200, Y: 180}},
			{ID: "done", Type: "terminal", Title: "Done", Description: "Workflow completed and ready for retrospective.", Position: entity.WorkflowPosition{X: 1480, Y: 180}},
		},
		Edges: []entity.WorkflowEdge{
			{ID: "e-intake-review", From: "intake", To: "spec_review"},
			{ID: "e-review-branch", From: "spec_review", To: "branch_plan"},
			{ID: "e-branch-impl", From: "branch_plan", To: "implementation", Label: "single or dev branch"},
			{ID: "e-branch-qa", From: "branch_plan", To: "qa", Label: "qa branch"},
			{ID: "e-impl-join", From: "implementation", To: "join"},
			{ID: "e-qa-join", From: "qa", To: "join"},
			{ID: "e-join-done", From: "join", To: "done"},
		},
	}
	return s.SaveDefinition(&def)
}

func (s *Store) SaveDefinition(def *entity.WorkflowDefinition) error {
	if def.ID == "" {
		def.ID = entity.NewWorkflowID()
	}
	now := time.Now().UTC()
	if def.CreatedAt.IsZero() {
		def.CreatedAt = now
	}
	def.UpdatedAt = now
	if def.Version == 0 {
		def.Version = 1
	}
	if def.Scope == "" {
		def.Scope = "project"
	}
	raw, err := json.Marshal(def)
	if err != nil {
		return err
	}
	return s.db.UpsertRecord("workflow_definitions", s.workspaceID, []string{def.ID}, string(raw))
}

func (s *Store) ListDefinitions() ([]entity.WorkflowDefinition, error) {
	recs, err := s.db.ListRecords("workflow_definitions", s.workspaceID, nil)
	if err != nil {
		return nil, err
	}
	out := make([]entity.WorkflowDefinition, 0, len(recs))
	for _, rec := range recs {
		var def entity.WorkflowDefinition
		if json.Unmarshal([]byte(rec.Payload), &def) == nil && def.Scope == "workspace" && def.Project == "" {
			out = append(out, def)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func (s *Store) Definition(id string) (entity.WorkflowDefinition, bool, error) {
	raw, ok, err := s.db.GetRecord("workflow_definitions", s.workspaceID, []string{id})
	if err != nil || !ok {
		return entity.WorkflowDefinition{}, ok, err
	}
	var def entity.WorkflowDefinition
	if err := json.Unmarshal([]byte(raw), &def); err != nil {
		return entity.WorkflowDefinition{}, false, err
	}
	return def, true, nil
}

func (s *Store) StartRun(project, taskID, definitionID string) (entity.WorkflowRun, []entity.WorkflowStepInstance, error) {
	def, ok, err := s.Definition(definitionID)
	if err != nil {
		return entity.WorkflowRun{}, nil, err
	}
	if !ok {
		return entity.WorkflowRun{}, nil, errs.NotFound("workflow_definition", definitionID)
	}
	now := time.Now().UTC()
	run := entity.WorkflowRun{
		ID:           entity.NewWorkflowRunID(),
		DefinitionID: def.ID,
		Project:      project,
		TaskID:       taskID,
		Status:       "active",
		ActiveStepID: def.StartStepID,
		StartedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.SaveRun(&run); err != nil {
		return entity.WorkflowRun{}, nil, err
	}
	instances := make([]entity.WorkflowStepInstance, 0, len(def.Steps))
	for _, step := range def.Steps {
		status := "pending"
		started := time.Time{}
		if step.ID == def.StartStepID {
			status = "running"
			started = now
		}
		inst := entity.WorkflowStepInstance{
			ID:        entity.NewWorkflowStepInstanceID(),
			RunID:     run.ID,
			StepID:    step.ID,
			Status:    status,
			StartedAt: started,
			UpdatedAt: now,
		}
		if err := s.SaveStepInstance(&inst); err != nil {
			return entity.WorkflowRun{}, nil, err
		}
		instances = append(instances, inst)
	}
	return run, instances, nil
}

func (s *Store) SaveRun(run *entity.WorkflowRun) error {
	raw, err := json.Marshal(run)
	if err != nil {
		return err
	}
	return s.db.UpsertRecord("workflow_runs", s.workspaceID, []string{run.Project, run.TaskID, run.ID}, string(raw))
}

func (s *Store) RunForTask(project, taskID string) (entity.WorkflowRun, bool, error) {
	recs, err := s.db.ListRecords("workflow_runs", s.workspaceID, []string{project, taskID})
	if err != nil || len(recs) == 0 {
		return entity.WorkflowRun{}, false, err
	}
	var run entity.WorkflowRun
	if err := json.Unmarshal([]byte(recs[0].Payload), &run); err != nil {
		return entity.WorkflowRun{}, false, err
	}
	return run, true, nil
}

func (s *Store) SaveStepInstance(inst *entity.WorkflowStepInstance) error {
	raw, err := json.Marshal(inst)
	if err != nil {
		return err
	}
	return s.db.UpsertRecord("workflow_step_instances", s.workspaceID, []string{inst.RunID, inst.StepID, inst.ID}, string(raw))
}

func (s *Store) ListStepInstances(runID string) ([]entity.WorkflowStepInstance, error) {
	recs, err := s.db.ListRecords("workflow_step_instances", s.workspaceID, []string{runID})
	if err != nil {
		return nil, err
	}
	out := make([]entity.WorkflowStepInstance, 0, len(recs))
	for _, rec := range recs {
		var inst entity.WorkflowStepInstance
		if json.Unmarshal([]byte(rec.Payload), &inst) == nil {
			out = append(out, inst)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StepID < out[j].StepID })
	return out, nil
}
