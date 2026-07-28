package api

import (
	"net/http/httptest"
	"testing"
)

func TestWorkflowWebBaseURLUsesConfiguredWebBase(t *testing.T) {
	t.Setenv("MULTIGENT_WEB_BASE_URL", "https://app.multigent.test")
	req := httptest.NewRequest("POST", "http://127.0.0.1:27893/api/v1/runtime", nil)
	if got := workflowWebBaseURL(req); got != "https://app.multigent.test" {
		t.Fatalf("workflowWebBaseURL=%q", got)
	}
}

func TestWorkflowWebBaseURLMapsLocalAPIPortToWebPort(t *testing.T) {
	req := httptest.NewRequest("POST", "http://127.0.0.1:27893/api/v1/runtime", nil)
	if got := workflowWebBaseURL(req); got != "http://127.0.0.1:27894" {
		t.Fatalf("workflowWebBaseURL=%q", got)
	}
}
