package workflow

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
)

func TestNormalizeWorkflowOutputValuesRequiresStructuredOutputs(t *testing.T) {
	step := entity.WorkflowStep{
		Title:        "PM Spec",
		OutputFields: []entity.WorkflowField{{Name: "spec_doc_id", Description: "Spec docID."}},
	}
	_, err := normalizeWorkflowOutputValues(step, nil, "", "plain text", false)
	if err == nil {
		t.Fatal("expected missing structured output error")
	}
	if !strings.Contains(err.Error(), "requires structured outputs") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeWorkflowOutputValuesValidatesDocIDFields(t *testing.T) {
	step := entity.WorkflowStep{
		Title: "PM Spec",
		OutputFields: []entity.WorkflowField{
			{Name: "spec_doc_id", Description: "Knowledge base document docID."},
			{Name: "summary", Description: "Short summary."},
		},
	}

	_, err := normalizeWorkflowOutputValues(step, map[string]string{
		"spec_doc_id": "I created doc-20260728-abc123",
		"summary":     "done",
	}, "", "", false)
	if err == nil {
		t.Fatal("expected invalid docID error")
	}
	if !strings.Contains(err.Error(), "must be a knowledge docID") {
		t.Fatalf("unexpected error: %v", err)
	}

	values, err := normalizeWorkflowOutputValues(step, map[string]string{
		"spec_doc_id": "doc-20260728-abc123",
		"summary":     "done",
	}, "", "", false)
	if err != nil {
		t.Fatalf("expected valid docID output: %v", err)
	}
	if values["spec_doc_id"] != "doc-20260728-abc123" {
		t.Fatalf("unexpected docID value: %q", values["spec_doc_id"])
	}
}

func TestWorkflowDocIDValueValidAllowsLists(t *testing.T) {
	if !workflowDocIDValueValid("doc-20260728-abc123, doc-20260728-def456") {
		t.Fatal("expected comma-separated docID list to be valid")
	}
	if workflowDocIDValueValid("doc-20260728-abc123 and doc-20260728-def456") {
		t.Fatal("expected prose docID value to be invalid")
	}
}

func TestWorkflowConditionEqDoesNotUseSubstringMatching(t *testing.T) {
	if compareWorkflowValue("不通过", "eq", "通过", nil) {
		t.Fatal("expected exact eq comparison; 不通过 must not match 通过")
	}
	if !compareWorkflowValue("通过", "eq", "通过", nil) {
		t.Fatal("expected exact eq comparison to match identical values")
	}
}

func TestWorkflowConditionInDoesNotUseSubstringMatching(t *testing.T) {
	if compareWorkflowValue("不通过", "in", "", []string{"通过", "approve"}) {
		t.Fatal("expected exact in comparison; 不通过 must not match 通过")
	}
	if !compareWorkflowValue("approve", "in", "", []string{"通过", "approve"}) {
		t.Fatal("expected exact in comparison to match listed value")
	}
}

func TestWorkflowActorBindingPrefersStepIDOverRole(t *testing.T) {
	controlDB, err := db.Open(filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer controlDB.Close()
	if err := controlDB.UpsertWorkspace(db.Workspace{ID: "workspace-1", Name: "Workspace", Slug: "workspace", Root: t.TempDir()}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	store := NewStore(controlDB, "workspace-1")
	now := time.Now().UTC()
	def := &entity.WorkflowDefinition{
		ID:          "wf-step-binding",
		Name:        "Step Binding Test",
		Version:     1,
		Scope:       "workspace",
		StartStepID: "draft",
		Steps: []entity.WorkflowStep{
			{
				ID:           "draft",
				Type:         "agent_task",
				Title:        "Draft",
				ActorRole:    "worker",
				OutputFields: []entity.WorkflowField{{Name: "draft_doc_id", Description: "Draft docID."}},
			},
			{
				ID:        "review",
				Type:      "agent_task",
				Title:     "Review",
				ActorRole: "worker",
			},
		},
		Edges:     []entity.WorkflowEdge{{ID: "e1", From: "draft", To: "review"}},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.SaveDefinition(def); err != nil {
		t.Fatalf("save definition: %v", err)
	}
	bindings := map[string]entity.WorkflowActorBinding{
		"worker": {Type: "agent", ID: "fallback-agent"},
		"draft":  {Type: "agent", ID: "analyst"},
		"review": {Type: "agent", ID: "reviewer"},
	}
	run, steps, err := store.StartRun("project", "task-1", def.ID, bindings)
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if run.ActorBindings["draft"].ID != "analyst" {
		t.Fatalf("expected run to preserve step binding, got %#v", run.ActorBindings["draft"])
	}
	start, ok := workflowStepInstanceByIDForTest(steps, "draft")
	if !ok {
		t.Fatal("missing draft step instance")
	}
	if start.ActorID != "analyst" {
		t.Fatalf("expected draft actor analyst, got %q", start.ActorID)
	}

	transition, err := store.CompleteAndAdvance("project", "task-1", "draft done", "", map[string]string{"draft_doc_id": "doc-20260730-abc123"}, "completed")
	if err != nil {
		t.Fatalf("complete and advance: %v", err)
	}
	if transition.NextInst == nil {
		t.Fatal("expected next instance")
	}
	if transition.NextInst.ActorID != "reviewer" {
		t.Fatalf("expected review actor reviewer, got %q", transition.NextInst.ActorID)
	}
}

func workflowStepInstanceByIDForTest(steps []entity.WorkflowStepInstance, stepID string) (entity.WorkflowStepInstance, bool) {
	for _, step := range steps {
		if step.StepID == stepID {
			return step, true
		}
	}
	return entity.WorkflowStepInstance{}, false
}

func TestCompleteBranchAndMaybeAdvanceWaitsForAllBranches(t *testing.T) {
	controlDB, err := db.Open(filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer controlDB.Close()
	if err := controlDB.UpsertWorkspace(db.Workspace{ID: "workspace-1", Name: "Workspace", Slug: "workspace", Root: t.TempDir()}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	store := NewStore(controlDB, "workspace-1")
	now := time.Now().UTC()
	def := &entity.WorkflowDefinition{
		ID:          "wf-parallel",
		Name:        "Parallel Test",
		Version:     1,
		Scope:       "workspace",
		StartStepID: "start",
		Steps: []entity.WorkflowStep{
			{
				ID:           "start",
				Type:         "agent_task",
				Title:        "Start",
				ActorRole:    "pm",
				OutputFields: []entity.WorkflowField{{Name: "spec_doc_id", Description: "Spec docID."}},
				Position:     entity.WorkflowPosition{X: 0, Y: 0},
			},
			{
				ID:       "parallel",
				Type:     "parallel_stage",
				Title:    "Parallel Stage",
				Position: entity.WorkflowPosition{X: 240, Y: 0},
				Branches: []entity.WorkflowBranch{
					{
						ID:           "frontend",
						Title:        "Frontend Spec",
						ActorRole:    "frontend",
						InputFields:  []entity.WorkflowField{{Name: "spec_doc_id", Description: "Spec docID."}},
						OutputFields: []entity.WorkflowField{{Name: "frontend_doc_id", Description: "Frontend docID."}},
					},
					{
						ID:           "backend",
						Title:        "Backend Spec",
						ActorRole:    "backend",
						InputFields:  []entity.WorkflowField{{Name: "spec_doc_id", Description: "Spec docID."}},
						OutputFields: []entity.WorkflowField{{Name: "backend_doc_id", Description: "Backend docID."}},
					},
				},
			},
			{
				ID:        "review",
				Type:      "human_review",
				Title:     "Review",
				ActorRole: "reviewer",
				Position:  entity.WorkflowPosition{X: 480, Y: 0},
			},
		},
		Edges: []entity.WorkflowEdge{
			{ID: "e1", From: "start", To: "parallel"},
			{ID: "e2", From: "parallel", To: "review"},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.SaveDefinition(def); err != nil {
		t.Fatalf("save definition: %v", err)
	}
	bindings := map[string]entity.WorkflowActorBinding{
		"pm":       {Type: "agent", ID: "pm"},
		"frontend": {Type: "agent", ID: "frontend"},
		"backend":  {Type: "agent", ID: "backend"},
		"reviewer": {Type: "human", ID: "owner"},
	}
	run, _, err := store.StartRun("project", "task-1", def.ID, bindings)
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	transition, err := store.CompleteAndAdvance("project", "task-1", "spec ready", "", map[string]string{"spec_doc_id": "doc-20260730-abc123"}, "completed")
	if err != nil {
		t.Fatalf("complete start: %v", err)
	}
	if transition.Next == nil || transition.Next.ID != "parallel" {
		t.Fatalf("expected transition to parallel stage, got %#v", transition.Next)
	}
	parent := transition.NextInst
	for _, branch := range transition.Next.Branches {
		inst := &entity.WorkflowBranchInstance{
			RunID:       run.ID,
			StepID:      transition.Next.ID,
			BranchID:    branch.ID,
			Status:      "running",
			ActorType:   "agent",
			ActorID:     bindings[branch.ActorRole].ID,
			ChildTaskID: entity.NewTaskID(),
			StartedAt:   now,
			UpdatedAt:   now,
			InputValues: buildBranchInputValues(*parent, branch),
		}
		if err := store.SaveBranchInstance(inst); err != nil {
			t.Fatalf("save branch instance: %v", err)
		}
	}
	first, err := store.CompleteBranchAndMaybeAdvance("project", "task-1", run.ID, "parallel", "frontend", "frontend done", map[string]string{"frontend_doc_id": "doc-20260730-front1"}, "completed")
	if err != nil {
		t.Fatalf("complete first branch: %v", err)
	}
	if first.AllDone {
		t.Fatal("expected first branch completion to wait for the remaining branch")
	}
	second, err := store.CompleteBranchAndMaybeAdvance("project", "task-1", run.ID, "parallel", "backend", "backend done", map[string]string{"backend_doc_id": "doc-20260730-back12"}, "completed")
	if err != nil {
		t.Fatalf("complete second branch: %v", err)
	}
	if !second.AllDone {
		t.Fatal("expected all branches done after second completion")
	}
	if second.Transition.Next == nil || second.Transition.Next.ID != "review" {
		t.Fatalf("expected transition to review, got %#v", second.Transition.Next)
	}
	if second.Transition.Run.ActiveStepID != "review" {
		t.Fatalf("expected active step review, got %q", second.Transition.Run.ActiveStepID)
	}
}

func TestCompleteBranchAndMaybeAdvanceAnyJoinSkipsRemainingBranches(t *testing.T) {
	controlDB, err := db.Open(filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer controlDB.Close()
	if err := controlDB.UpsertWorkspace(db.Workspace{ID: "workspace-1", Name: "Workspace", Slug: "workspace", Root: t.TempDir()}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	store := NewStore(controlDB, "workspace-1")
	now := time.Now().UTC()
	def := &entity.WorkflowDefinition{
		ID:          "wf-parallel-any",
		Name:        "Parallel Any Test",
		Version:     1,
		Scope:       "workspace",
		StartStepID: "parallel",
		Steps: []entity.WorkflowStep{
			{
				ID:         "parallel",
				Type:       "parallel_stage",
				Title:      "Parallel Stage",
				JoinPolicy: "any",
				Position:   entity.WorkflowPosition{X: 0, Y: 0},
				Branches: []entity.WorkflowBranch{
					{ID: "path_a", Title: "Path A", ActorRole: "agent_a", OutputFields: []entity.WorkflowField{{Name: "path_a_doc_id", Description: "docID"}}},
					{ID: "path_b", Title: "Path B", ActorRole: "agent_b", OutputFields: []entity.WorkflowField{{Name: "path_b_doc_id", Description: "docID"}}},
				},
			},
			{ID: "done", Type: "human_review", Title: "Done", ActorRole: "reviewer", Position: entity.WorkflowPosition{X: 240, Y: 0}},
		},
		Edges: []entity.WorkflowEdge{{ID: "e1", From: "parallel", To: "done"}},
	}
	if err := store.SaveDefinition(def); err != nil {
		t.Fatalf("save definition: %v", err)
	}
	run, _, err := store.StartRun("project", "task-1", def.ID, map[string]entity.WorkflowActorBinding{
		"agent_a":  {Type: "agent", ID: "agent-a"},
		"agent_b":  {Type: "agent", ID: "agent-b"},
		"reviewer": {Type: "human", ID: "owner"},
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	for _, branch := range def.Steps[0].Branches {
		if err := store.SaveBranchInstance(&entity.WorkflowBranchInstance{
			RunID:       run.ID,
			StepID:      "parallel",
			BranchID:    branch.ID,
			Status:      "running",
			ChildTaskID: entity.NewTaskID(),
			StartedAt:   now,
			UpdatedAt:   now,
		}); err != nil {
			t.Fatalf("save branch instance: %v", err)
		}
	}
	result, err := store.CompleteBranchAndMaybeAdvance("project", "task-1", run.ID, "parallel", "path_a", "path a done", map[string]string{"path_a_doc_id": "doc-20260730-aa11"}, "completed")
	if err != nil {
		t.Fatalf("complete branch: %v", err)
	}
	if !result.AllDone {
		t.Fatal("expected any join to advance after first completed branch")
	}
	branches, err := store.BranchInstancesForStep(run.ID, "parallel")
	if err != nil {
		t.Fatalf("list branches: %v", err)
	}
	statuses := map[string]string{}
	for _, branch := range branches {
		statuses[branch.BranchID] = branch.Status
	}
	if statuses["path_b"] != "skipped" {
		t.Fatalf("expected remaining branch to be skipped, got %q", statuses["path_b"])
	}
}
