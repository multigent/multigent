package main

import (
	"strings"
	"testing"

	"github.com/multigent/multigent/internal/entity"
)

func TestScaffoldParallelWorkflowBuildsChildSubflows(t *testing.T) {
	def, err := scaffoldParallelWorkflow(scaffoldParallelWorkflowOptions{
		Name:            "Parallel spec review",
		StartRole:       "pm",
		JoinPolicy:      "any",
		FinalReviewRole: "tech_lead",
		Branches: []string{
			"Frontend spec=frontend_engineer",
			"Backend spec=backend_engineer",
		},
	})
	if err != nil {
		t.Fatalf("scaffoldParallelWorkflow: %v", err)
	}
	if err := validateWorkflowDefinition(def); err != nil {
		t.Fatalf("validateWorkflowDefinition: %v", err)
	}
	if got, want := len(def.Steps), 3; got != want {
		t.Fatalf("steps=%d, want %d", got, want)
	}
	stage := def.Steps[1]
	if stage.Type != "parallel_stage" {
		t.Fatalf("stage type=%q, want parallel_stage", stage.Type)
	}
	if stage.JoinPolicy != "any" {
		t.Fatalf("joinPolicy=%q, want any", stage.JoinPolicy)
	}
	if got, want := len(stage.Branches), 2; got != want {
		t.Fatalf("branches=%d, want %d", got, want)
	}
	for _, branch := range stage.Branches {
		if branch.Workflow == nil {
			t.Fatalf("branch %q has no child workflow", branch.ID)
		}
		if branch.Workflow.StartStepID != "work" {
			t.Fatalf("branch %q start=%q, want work", branch.ID, branch.Workflow.StartStepID)
		}
		if got, want := len(branch.Workflow.Steps), 1; got != want {
			t.Fatalf("branch %q steps=%d, want %d", branch.ID, got, want)
		}
	}
}

func TestValidateWorkflowDefinitionRejectsBrokenBranchSubflow(t *testing.T) {
	def, err := scaffoldParallelWorkflow(scaffoldParallelWorkflowOptions{
		Name:       "Broken branch",
		JoinPolicy: "all",
		Branches:   []string{"Frontend spec=frontend_engineer"},
	})
	if err != nil {
		t.Fatalf("scaffoldParallelWorkflow: %v", err)
	}
	def.Steps[1].Branches[0].Workflow.Edges = []entity.WorkflowEdge{
		{ID: "broken", From: "work", To: "missing"},
	}
	err = validateWorkflowDefinition(def)
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if !strings.Contains(err.Error(), "missing to step") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateWorkflowDefinitionAllowsTerminalEdge(t *testing.T) {
	def := entity.WorkflowDefinition{
		ID:          "terminal-flow",
		Name:        "Terminal Flow",
		StartStepID: "review",
		Steps: []entity.WorkflowStep{
			{ID: "review", Type: "human_review", Title: "Review"},
		},
		Edges: []entity.WorkflowEdge{
			{ID: "done", From: "review", To: "", Condition: &entity.WorkflowEdgeCondition{Field: "decision", Operator: "eq", Value: "approve"}},
		},
	}
	if err := validateWorkflowDefinition(def); err != nil {
		t.Fatalf("validateWorkflowDefinition: %v", err)
	}
}
