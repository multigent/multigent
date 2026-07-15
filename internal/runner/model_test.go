package runner

import (
	"slices"
	"testing"

	"github.com/multigent/multigent/internal/entity"
)

func TestCodexInvokerParseSessionID(t *testing.T) {
	invoker := &codexInvoker{}
	got := invoker.ParseSessionID("OpenAI Codex\nSession ID: sess-123\n")
	if got != "sess-123" {
		t.Fatalf("ParseSessionID() = %q, want %q", got, "sess-123")
	}
}

func TestCodexInvokerUsesJSONOutput(t *testing.T) {
	invoker := &codexInvoker{addDirs: []string{"/repo"}}
	args := invoker.Args("/tmp/prompt.txt", "")
	if !slices.Contains(args, "--json") {
		t.Fatalf("codex args missing --json: %#v", args)
	}
}

func TestCodexInvokerParseJSONSessionID(t *testing.T) {
	invoker := &codexInvoker{}
	got := invoker.ParseSessionID(`{"type":"thread.started","thread_id":"019f6786-2e90-7b02-9b30-7f4e78c4f64a"}`)
	if got != "019f6786-2e90-7b02-9b30-7f4e78c4f64a" {
		t.Fatalf("ParseSessionID() = %q", got)
	}
}

func TestCodexInvokerParseLowercaseSessionID(t *testing.T) {
	invoker := &codexInvoker{}
	got := invoker.ParseSessionID("OpenAI Codex\nsession id: 019e0262-618f-7d80-9a6d-fb5ed664ccaa\n")
	if got != "019e0262-618f-7d80-9a6d-fb5ed664ccaa" {
		t.Fatalf("ParseSessionID() = %q", got)
	}
}

func TestCodexResumeMissingRolloutError(t *testing.T) {
	output := "Error: thread/resume: thread/resume failed: no rollout found for thread id 019e0262-618f-7d80-9a6d-fb5ed664ccaa"
	if !isCodexResumeMissingRolloutError(output) {
		t.Fatal("expected missing rollout error to be detected")
	}
}

func TestDiscardSessionIDOnFailure(t *testing.T) {
	if !discardSessionIDOnFailure(entity.ModelCodex) {
		t.Fatal("expected codex failed sessions to be discarded")
	}
	if discardSessionIDOnFailure(entity.ModelClaudeCode) {
		t.Fatal("did not expect claude failed sessions to be discarded")
	}
}
