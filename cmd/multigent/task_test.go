package main

import "testing"

func TestSplitWorkflowPromptKVForCLI(t *testing.T) {
	tests := []struct {
		line  string
		key   string
		value string
		ok    bool
	}{
		{line: "repo: multigent/workflow-sandbox", key: "repo", value: "multigent/workflow-sandbox", ok: true},
		{line: "issue_number： 1684", key: "issue_number", value: "1684", ok: true},
		{line: "`pr_url`: https://github.com/multigent/workflow-sandbox/pull/27", key: "pr_url", value: "https://github.com/multigent/workflow-sandbox/pull/27", ok: true},
		{line: "not a key value", ok: false},
		{line: "human readable title: should be ignored", ok: false},
	}
	for _, tt := range tests {
		key, value, ok := splitWorkflowPromptKVForCLI(tt.line)
		if ok != tt.ok || key != tt.key || value != tt.value {
			t.Fatalf("splitWorkflowPromptKVForCLI(%q)=(%q,%q,%v), want (%q,%q,%v)", tt.line, key, value, ok, tt.key, tt.value, tt.ok)
		}
	}
}
