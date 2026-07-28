package api

import (
	"net/http/httptest"
	"testing"

	"github.com/multigent/multigent/internal/entity"
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

func TestWorkflowStepNotifyAssigneeRequiresExplicitConfig(t *testing.T) {
	if workflowStepNotifyAssignee(entity.WorkflowStep{Type: "human_review"}) {
		t.Fatal("expected notification disabled by default")
	}
	step := entity.WorkflowStep{Type: "human_review", Config: map[string]string{workflowNotifyAssigneeKey: "true"}}
	if !workflowStepNotifyAssignee(step) {
		t.Fatal("expected notification enabled when config is true")
	}
}

func TestWorkflowStepNotifyProviders(t *testing.T) {
	auto := workflowStepNotifyProviders(entity.WorkflowStep{Config: map[string]string{workflowNotifyChannelKey: workflowNotifyChannelAuto}})
	if len(auto) != 2 || auto[0] != workflowNotifyChannelFeishu || auto[1] != workflowNotifyChannelLark {
		t.Fatalf("auto providers=%v", auto)
	}
	feishu := workflowStepNotifyProviders(entity.WorkflowStep{Config: map[string]string{workflowNotifyChannelKey: workflowNotifyChannelFeishu}})
	if len(feishu) != 1 || feishu[0] != workflowNotifyChannelFeishu {
		t.Fatalf("feishu providers=%v", feishu)
	}
	unknown := workflowStepNotifyProviders(entity.WorkflowStep{Config: map[string]string{workflowNotifyChannelKey: "email"}})
	if len(unknown) != 0 {
		t.Fatalf("unknown providers=%v", unknown)
	}
}
