package api

import "testing"

func TestPreferredExampleLocaleUsesExplicitUILanguage(t *testing.T) {
	got := preferredExampleLocale("en", "zh-CN,zh;q=0.9,en;q=0.8")
	if got != "en" {
		t.Fatalf("expected explicit UI locale en, got %q", got)
	}
}

func TestPreferredExampleLocaleFallsBackToAcceptLanguage(t *testing.T) {
	got := preferredExampleLocale("", "zh-CN,zh;q=0.9,en;q=0.8")
	if got != "zh-CN,zh;q=0.9,en;q=0.8" {
		t.Fatalf("expected accept-language fallback, got %q", got)
	}
	spec := exampleWorkspaceSpec(got)
	if spec.TaskTitle == "" || spec.TaskTitle == "Prepare a Multigent collaboration onboarding note for a new teammate" {
		t.Fatalf("expected zh-CN example spec, got task title %q", spec.TaskTitle)
	}
}
