package workflow

import (
	"strings"
	"testing"

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
