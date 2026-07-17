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
	} else if ok && def.Scope == "workspace" && def.Project == "" && def.Version >= 2 && def.StartStepID == "requirement_draft" {
		return nil
	}
	now := time.Now().UTC()
	def := entity.WorkflowDefinition{
		ID:          "software-delivery-v1",
		Name:        "Agentic Software Delivery",
		Description: "A configurable human-agent delivery workflow. Agents draft artifacts, humans review only the decision gates, and rejected outputs loop back with structured feedback.",
		Version:     2,
		Scope:       "workspace",
		StartStepID: "requirement_draft",
		CreatedAt:   now,
		UpdatedAt:   now,
		Steps: []entity.WorkflowStep{
			{
				ID: "requirement_draft", Type: "agent_task", Title: "Requirement Draft",
				Description:  "An agent turns an incoming request into a structured understanding: problem, goal, scope, non-goals, risks, and open questions.",
				ActorRole:    "pm-agent",
				InputFields:  []entity.WorkflowField{{Name: "request", Description: "Original user, customer, founder, or internal request."}, {Name: "context", Description: "Known background, links, meeting notes, or existing discussions."}},
				OutputFields: []entity.WorkflowField{{Name: "requirement_draft", Description: "Structured requirement draft."}, {Name: "open_questions", Description: "Questions that still need human or stakeholder clarification."}},
				Position:     entity.WorkflowPosition{X: 80, Y: 180},
				Config:       map[string]string{"color": "sky"},
			},
			{
				ID: "requirement_review", Type: "human_review", Title: "Requirement Review",
				Description:  "A human reviews whether the requirement draft expresses the real problem and whether more clarification is needed.",
				ActorRole:    "product-owner",
				InputFields:  []entity.WorkflowField{{Name: "requirement_draft", Description: "Draft produced by the PM agent."}},
				OutputFields: []entity.WorkflowField{{Name: "decision", Description: "approve, request_changes, or need_discussion."}, {Name: "comments", Description: "Review comments and clarification notes."}},
				ReviewPolicy: "manual",
				Position:     entity.WorkflowPosition{X: 360, Y: 180},
				Config:       map[string]string{"color": "amber"},
			},
			{
				ID: "prd_draft", Type: "agent_task", Title: "PRD Draft",
				Description:  "The PM agent produces the product spec, acceptance criteria, release scope, and non-goals.",
				ActorRole:    "pm-agent",
				InputFields:  []entity.WorkflowField{{Name: "approved_requirement", Description: "Reviewed requirement with comments folded in."}},
				OutputFields: []entity.WorkflowField{{Name: "prd", Description: "Product requirements document or spec."}, {Name: "acceptance_criteria", Description: "Observable acceptance criteria."}},
				Position:     entity.WorkflowPosition{X: 640, Y: 180},
				Config:       map[string]string{"color": "sky"},
			},
			{
				ID: "prd_review", Type: "human_review", Title: "PRD Review",
				Description:  "Product and engineering stakeholders review scope, non-goals, and acceptance criteria.",
				ActorRole:    "product-owner",
				InputFields:  []entity.WorkflowField{{Name: "prd", Description: "PRD draft to review."}},
				OutputFields: []entity.WorkflowField{{Name: "decision", Description: "approve or request_changes."}, {Name: "comments", Description: "Review comments."}, {Name: "approved_prd", Description: "Final PRD when approved."}},
				ReviewPolicy: "manual",
				Position:     entity.WorkflowPosition{X: 920, Y: 180},
				Config:       map[string]string{"color": "amber"},
			},
			{
				ID: "tech_spec_draft", Type: "agent_task", Title: "Technical Spec Draft",
				Description:  "Engineering agents inspect the codebase and produce implementation plan, affected surfaces, test strategy, and task split recommendation.",
				ActorRole:    "engineering-agent",
				InputFields:  []entity.WorkflowField{{Name: "approved_prd", Description: "Reviewed product spec."}},
				OutputFields: []entity.WorkflowField{{Name: "technical_spec", Description: "Implementation plan and technical decisions."}, {Name: "task_split", Description: "Optional child task split for parallel work."}},
				Position:     entity.WorkflowPosition{X: 1200, Y: 180},
				Config:       map[string]string{"color": "violet"},
			},
			{
				ID: "tech_spec_review", Type: "human_review", Title: "Technical Spec Review",
				Description:  "Responsible engineers review the plan before implementation starts.",
				ActorRole:    "tech-lead",
				InputFields:  []entity.WorkflowField{{Name: "technical_spec", Description: "Technical plan to review."}},
				OutputFields: []entity.WorkflowField{{Name: "decision", Description: "approve or request_changes."}, {Name: "comments", Description: "Review comments."}, {Name: "approved_technical_spec", Description: "Final technical spec when approved."}},
				ReviewPolicy: "manual",
				Position:     entity.WorkflowPosition{X: 1480, Y: 180},
				Config:       map[string]string{"color": "amber"},
			},
			{
				ID: "implementation", Type: "agent_task", Title: "Implementation",
				Description:  "Development agents implement the approved technical plan and produce code changes, tests, and a PR or patch summary.",
				ActorRole:    "developer-agent",
				InputFields:  []entity.WorkflowField{{Name: "approved_technical_spec", Description: "Approved implementation plan."}},
				OutputFields: []entity.WorkflowField{{Name: "pr", Description: "Pull request, patch, or change summary."}, {Name: "tests_run", Description: "Tests executed by the agent."}, {Name: "risks", Description: "Known risks or manual checks needed."}},
				Position:     entity.WorkflowPosition{X: 1760, Y: 180},
				Config:       map[string]string{"color": "emerald"},
			},
			{
				ID: "code_review", Type: "human_review", Title: "Code Review",
				Description:  "The responsible human reviews code quality, risk, and whether the output matches the approved spec.",
				ActorRole:    "owner-engineer",
				InputFields:  []entity.WorkflowField{{Name: "pr", Description: "PR or patch to review."}},
				OutputFields: []entity.WorkflowField{{Name: "decision", Description: "approve or request_changes."}, {Name: "comments", Description: "Code review comments."}, {Name: "approved_change", Description: "Approved code artifact."}},
				ReviewPolicy: "manual",
				Position:     entity.WorkflowPosition{X: 2040, Y: 180},
				Config:       map[string]string{"color": "amber"},
			},
			{
				ID: "qa", Type: "agent_task", Title: "QA Test",
				Description:  "QA agents generate and execute test cases, then identify remaining manual test needs.",
				ActorRole:    "qa-agent",
				InputFields:  []entity.WorkflowField{{Name: "approved_change", Description: "Code artifact approved for testing."}},
				OutputFields: []entity.WorkflowField{{Name: "test_cases", Description: "Test cases."}, {Name: "test_report", Description: "Automated and manual test result summary."}},
				Position:     entity.WorkflowPosition{X: 2320, Y: 180},
				Config:       map[string]string{"color": "rose"},
			},
			{
				ID: "qa_review", Type: "human_review", Title: "QA Review",
				Description:  "Human QA or owner reviews the test report and decides whether release can proceed.",
				ActorRole:    "qa-owner",
				InputFields:  []entity.WorkflowField{{Name: "test_report", Description: "Test report to review."}},
				OutputFields: []entity.WorkflowField{{Name: "decision", Description: "approve or request_changes."}, {Name: "comments", Description: "QA feedback."}, {Name: "release_candidate", Description: "Approved release candidate."}},
				ReviewPolicy: "manual",
				Position:     entity.WorkflowPosition{X: 2600, Y: 180},
				Config:       map[string]string{"color": "amber"},
			},
			{
				ID: "release", Type: "agent_task", Title: "Release and Observe",
				Description:  "Release agents prepare rollout notes, execute allowed release steps, and check post-release signals.",
				ActorRole:    "release-agent",
				InputFields:  []entity.WorkflowField{{Name: "release_candidate", Description: "Approved release candidate."}},
				OutputFields: []entity.WorkflowField{{Name: "release_report", Description: "Release result, monitoring checks, and follow-up items."}},
				Position:     entity.WorkflowPosition{X: 2880, Y: 180},
				Config:       map[string]string{"color": "emerald"},
			},
			{ID: "done", Type: "terminal", Title: "Done", Description: "Workflow completed with artifacts and metrics ready for retrospective.", Position: entity.WorkflowPosition{X: 3160, Y: 180}},
		},
		Edges: []entity.WorkflowEdge{
			edge("e-req-to-review", "requirement_draft", "requirement_review", "", nil, nil, true),
			edge("e-req-review-approve", "requirement_review", "prd_draft", "approved", cond("decision", "eq", "approve"), map[string]string{"approved_requirement": "$output.requirement_draft"}, false),
			edge("e-req-review-rework", "requirement_review", "requirement_draft", "changes requested", cond("decision", "eq", "request_changes"), map[string]string{"review_comments": "$output.comments", "previous_draft": "$input.requirement_draft"}, false),
			edge("e-prd-to-review", "prd_draft", "prd_review", "", nil, nil, true),
			edge("e-prd-review-approve", "prd_review", "tech_spec_draft", "approved", cond("decision", "eq", "approve"), map[string]string{"approved_prd": "$output.approved_prd"}, false),
			edge("e-prd-review-rework", "prd_review", "prd_draft", "changes requested", cond("decision", "eq", "request_changes"), map[string]string{"review_comments": "$output.comments", "previous_prd": "$input.prd"}, false),
			edge("e-tech-to-review", "tech_spec_draft", "tech_spec_review", "", nil, nil, true),
			edge("e-tech-review-approve", "tech_spec_review", "implementation", "approved", cond("decision", "eq", "approve"), map[string]string{"approved_technical_spec": "$output.approved_technical_spec"}, false),
			edge("e-tech-review-rework", "tech_spec_review", "tech_spec_draft", "changes requested", cond("decision", "eq", "request_changes"), map[string]string{"review_comments": "$output.comments", "previous_spec": "$input.technical_spec"}, false),
			edge("e-impl-to-review", "implementation", "code_review", "", nil, nil, true),
			edge("e-code-review-approve", "code_review", "qa", "approved", cond("decision", "eq", "approve"), map[string]string{"approved_change": "$output.approved_change"}, false),
			edge("e-code-review-rework", "code_review", "implementation", "changes requested", cond("decision", "eq", "request_changes"), map[string]string{"review_comments": "$output.comments", "previous_pr": "$input.pr"}, false),
			edge("e-qa-to-review", "qa", "qa_review", "", nil, nil, true),
			edge("e-qa-review-approve", "qa_review", "release", "approved", cond("decision", "eq", "approve"), map[string]string{"release_candidate": "$output.release_candidate"}, false),
			edge("e-qa-review-rework", "qa_review", "qa", "changes requested", cond("decision", "eq", "request_changes"), map[string]string{"review_comments": "$output.comments", "previous_report": "$input.test_report"}, false),
			edge("e-release-done", "release", "done", "", nil, nil, true),
		},
	}
	return s.SaveDefinition(&def)
}

func cond(field, operator, value string) *entity.WorkflowEdgeCondition {
	return &entity.WorkflowEdgeCondition{Field: field, Operator: operator, Value: value}
}

func edge(id, from, to, label string, condition *entity.WorkflowEdgeCondition, inputMapping map[string]string, isDefault bool) entity.WorkflowEdge {
	return entity.WorkflowEdge{
		ID:           id,
		From:         from,
		To:           to,
		Label:        label,
		Condition:    condition,
		InputMapping: inputMapping,
		IsDefault:    isDefault,
	}
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
