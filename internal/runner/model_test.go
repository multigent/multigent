package runner

import (
	"os"
	"path/filepath"
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

func TestClaudeInvokerUsesPermissionModeBypass(t *testing.T) {
	invoker := &claudeInvoker{}
	args := invoker.Args("/tmp/prompt.txt", "")
	if slices.Contains(args, "--dangerously-skip-permissions") {
		t.Fatalf("claude args should not use root-blocked dangerous flag: %#v", args)
	}
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--permission-mode" && args[i+1] == "bypassPermissions" {
			return
		}
	}
	t.Fatalf("claude args missing --permission-mode bypassPermissions: %#v", args)
}

func TestResumeSessionIDForCLIDropsMultigentLogicalSession(t *testing.T) {
	if got := ResumeSessionIDForCLI("sess_8bbb9f0a8f71b5a1"); got != "" {
		t.Fatalf("expected Multigent logical session to be dropped, got %q", got)
	}
	if got := ResumeSessionIDForCLI(" fb643778-f20d-45c3-8dce-7937cd5d9099 "); got != "fb643778-f20d-45c3-8dce-7937cd5d9099" {
		t.Fatalf("expected provider-native session to be preserved, got %q", got)
	}
}

func TestValidateDirectHostExecutionBlocksClaudeBypassWhenRoot(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-specific Claude Code behavior")
	}
	args := []string{"claude", "-p", "--permission-mode", "bypassPermissions", "--output-format", "stream-json"}
	if err := validateDirectHostExecution(entity.ModelClaudeCode, args, nil); err == nil {
		t.Fatalf("expected root direct host Claude bypass to be blocked")
	}
	if err := validateDirectHostExecution(entity.ModelClaudeCode, args, []string{"IS_SANDBOX=1"}); err != nil {
		t.Fatalf("expected sandbox-marked Claude bypass to be allowed: %v", err)
	}
}

func TestDirectHostRuntimeEnvMarksClaudeAsSandboxed(t *testing.T) {
	got := directHostRuntimeEnv(entity.ModelClaudeCode)
	if got["IS_SANDBOX"] != "1" {
		t.Fatalf("expected Claude Code direct host env to include IS_SANDBOX=1, got %#v", got)
	}
	if got := directHostRuntimeEnv(entity.ModelCodex); len(got) != 0 {
		t.Fatalf("expected Codex direct host env to be empty, got %#v", got)
	}
}

func TestEnsureDirectHostCLIPathResolvesUserLocalAgent(t *testing.T) {
	home := t.TempDir()
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	agentPath := filepath.Join(binDir, "agent")
	if err := os.WriteFile(agentPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write agent: %v", err)
	}
	t.Setenv("HOME", home)

	env := ensureDirectHostCLIPath([]string{"PATH=/usr/bin:/bin"})
	if got := resolveExecutableFromEnv("agent", env); got != agentPath {
		t.Fatalf("resolveExecutableFromEnv(agent) = %q, want %q; env=%v", got, agentPath, envLookup(env, "PATH"))
	}
}

func TestAdaptDirectHostArgsAddsCodexBypassSandbox(t *testing.T) {
	args := []string{"codex", "exec", "--json", "--skip-git-repo-check", "-"}
	got := adaptDirectHostArgs(entity.ModelCodex, args)
	want := []string{"codex", "exec", "--dangerously-bypass-approvals-and-sandbox", "--json", "--skip-git-repo-check", "-"}
	if !slices.Equal(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func TestAdaptDirectHostArgsAddsCodexBypassBeforeResume(t *testing.T) {
	args := []string{"codex", "exec", "--json", "resume", "session-1", "-"}
	got := adaptDirectHostArgs(entity.ModelCodex, args)
	want := []string{"codex", "exec", "--dangerously-bypass-approvals-and-sandbox", "--json", "resume", "session-1", "-"}
	if !slices.Equal(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func TestAdaptSandboxArgsKeepsClaudeDangerousBypass(t *testing.T) {
	args := []string{"claude", "-p", "--permission-mode", "bypassPermissions", "--output-format", "stream-json"}
	got := adaptSandboxArgs(entity.ModelClaudeCode, args)
	if !slices.Contains(got, "--dangerously-skip-permissions") {
		t.Fatalf("expected sandbox Claude args to use dangerous bypass: %#v", got)
	}
	if slices.Contains(got, "--permission-mode") || slices.Contains(got, "bypassPermissions") {
		t.Fatalf("expected sandbox Claude args to replace permission-mode bypass: %#v", got)
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
