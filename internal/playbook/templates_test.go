package playbook

import "testing"

func TestOpenSpecPlaybookTemplate(t *testing.T) {
	tmpl, ok := Template("openspec-artifact-guided-delivery", "zh-CN")
	if !ok {
		t.Fatal("openspec playbook template missing")
	}
	if tmpl.Locale != "zh-CN" {
		t.Fatalf("locale=%q", tmpl.Locale)
	}
	if tmpl.Name == "" || tmpl.Version == "" {
		t.Fatalf("incomplete template metadata: %#v", tmpl)
	}
	if len(tmpl.Roles) < 6 {
		t.Fatalf("expected roles, got %d", len(tmpl.Roles))
	}
	if len(tmpl.Skills) < 8 {
		t.Fatalf("expected skills, got %d", len(tmpl.Skills))
	}
	if len(tmpl.Workflows) != 1 {
		t.Fatalf("expected one workflow, got %d", len(tmpl.Workflows))
	}
	wf := tmpl.Workflows[0]
	if wf.Definition.StartStepID != "explore" {
		t.Fatalf("start step=%q", wf.Definition.StartStepID)
	}
	if len(wf.Definition.Steps) < 8 || len(wf.Definition.Edges) < 10 {
		t.Fatalf("workflow too small: steps=%d edges=%d", len(wf.Definition.Steps), len(wf.Definition.Edges))
	}
	if wf.RoleBindings["specs"] != "openspec-spec-author" {
		t.Fatalf("unexpected role bindings=%#v", wf.RoleBindings)
	}
	foundReviewLoop := false
	for _, edge := range wf.Definition.Edges {
		if edge.From == "spec_review" && edge.To == "specs" {
			foundReviewLoop = true
			break
		}
	}
	if !foundReviewLoop {
		t.Fatalf("expected spec review rework edge")
	}
}
