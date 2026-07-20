package playbook

import (
	"strings"
	"testing"
)

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
	if len(tmpl.Roles) < 4 {
		t.Fatalf("expected roles, got %d", len(tmpl.Roles))
	}
	if len(tmpl.Skills) < 12 {
		t.Fatalf("expected skills, got %d", len(tmpl.Skills))
	}
	foundUpstreamSkill := false
	for _, sk := range tmpl.Skills {
		if sk.ID == "openspec-propose" && sk.Source != "" && sk.Body != "" {
			foundUpstreamSkill = true
			if !containsAll(sk.Body, "allowed-tools: Bash(openspec:*)", "Store selection", "openspec status --change") {
				t.Fatalf("openspec-propose does not look like upstream skill body")
			}
		}
	}
	if !foundUpstreamSkill {
		t.Fatalf("openspec-propose skill missing")
	}
	if len(tmpl.Workflows) != 1 {
		t.Fatalf("expected one workflow, got %d", len(tmpl.Workflows))
	}
	wf := tmpl.Workflows[0]
	if wf.Definition.StartStepID != "explore" {
		t.Fatalf("start step=%q", wf.Definition.StartStepID)
	}
	if len(wf.Definition.Steps) < 7 || len(wf.Definition.Edges) < 9 {
		t.Fatalf("workflow too small: steps=%d edges=%d", len(wf.Definition.Steps), len(wf.Definition.Edges))
	}
	if wf.RoleBindings["propose"] != "openspec-change-owner" {
		t.Fatalf("unexpected role bindings=%#v", wf.RoleBindings)
	}
	foundReviewLoop := false
	for _, edge := range wf.Definition.Edges {
		if edge.From == "plan_review" && edge.To == "propose" {
			foundReviewLoop = true
			break
		}
	}
	if !foundReviewLoop {
		t.Fatalf("expected plan review rework edge")
	}
}

func containsAll(s string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(s, needle) {
			return false
		}
	}
	return true
}
