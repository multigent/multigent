package api

import (
	"os/exec"
	"strings"
	"testing"

	controldb "github.com/multigent/multigent/internal/db"
)

func TestFormatInteractionCallbackPromptDoesNotLeakDelegationToken(t *testing.T) {
	token := "delegation-secret-token"
	prompt := formatInteractionCallbackPrompt(controldb.InteractionRequest{
		ID:          "ir_test",
		HandlerType: "agent_event",
		ContextJSON: `{"taskId":"t-1"}`,
	}, `{"actionId":"approve"}`, token, "2026-08-22T00:00:00Z")

	if strings.Contains(prompt, token) {
		t.Fatalf("prompt leaked delegation token: %s", prompt)
	}
	if !strings.Contains(prompt, "MULTIGENT_DELEGATION_TOKEN") {
		t.Fatalf("prompt should tell the agent to use MULTIGENT_DELEGATION_TOKEN: %s", prompt)
	}
	if strings.Contains(prompt, "--delegation-token <token>") {
		t.Fatalf("prompt should not encourage putting delegation tokens on the command line: %s", prompt)
	}
}

func TestConfigureAgentExecEnvIncludesExtraEnv(t *testing.T) {
	cmd := exec.Command("true")
	users := newTestUserStore(t)
	s := &Server{controlDB: users.db, users: users}

	s.configureAgentExecEnv(cmd, "ws", "project", "agent", "http://127.0.0.1:27893", map[string]string{
		"MULTIGENT_DELEGATION_TOKEN":      "delegation-token",
		"MULTIGENT_DELEGATION_EXPIRES_AT": "2026-08-22T00:00:00Z",
	})

	got := strings.Join(cmd.Env, "\n")
	if !strings.Contains(got, "MULTIGENT_DELEGATION_TOKEN=delegation-token") {
		t.Fatalf("missing delegation token env: %s", got)
	}
	if !strings.Contains(got, "MULTIGENT_DELEGATION_EXPIRES_AT=2026-08-22T00:00:00Z") {
		t.Fatalf("missing delegation expiry env: %s", got)
	}
}
