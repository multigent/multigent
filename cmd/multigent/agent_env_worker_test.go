package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
)

func TestAgentEnvCLIUsesAgentWorkerRuntimeConfig(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MULTIGENT_CONTROL_DATA_DIR", "")
	t.Setenv("MULTIGENT_DATA_DIR", root)
	if err := os.MkdirAll(filepath.Join(root, ".multigent"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".multigent", "agency.yaml"), []byte("name: Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := controldb.Open(filepath.Join(root, ".multigent", "multigent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	if err := db.UpsertWorkspace(controldb.Workspace{ID: "ws", Name: "Test", Slug: "test", Root: root, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertAgentWorker(controldb.AgentWorker{
		ID:                "aw-cli-env",
		WorkspaceID:       "ws",
		Name:              "nova",
		Model:             "codex",
		RuntimeConfigJSON: `{"env":{"EXISTING":"1"}}`,
		CreatedAt:         now,
		UpdatedAt:         now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertProjectMembership(controldb.ProjectMembership{
		ID:          "pm-cli-env",
		WorkspaceID: "ws",
		ProjectID:   "alpha",
		MemberType:  "agent_worker",
		MemberID:    "aw-cli-env",
		Title:       "nova",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatal(err)
	}

	updated, err := updateWorkerAgentEnv(root, "alpha", "nova", func(env map[string]string) (map[string]string, error) {
		env["RUNTIME_FLAG"] = "enabled"
		return env, nil
	})
	if err != nil || !updated {
		t.Fatalf("update worker env updated=%v err=%v", updated, err)
	}
	env, ok, err := workerAgentEnv(root, "alpha", "nova")
	if err != nil || !ok {
		t.Fatalf("worker env ok=%v err=%v", ok, err)
	}
	if env["EXISTING"] != "1" || env["RUNTIME_FLAG"] != "enabled" {
		t.Fatalf("unexpected worker env: %#v", env)
	}
	if updated, err := updateWorkerAgentEnv(root, "alpha", "nova", func(env map[string]string) (map[string]string, error) {
		delete(env, "RUNTIME_FLAG")
		return env, nil
	}); err != nil || !updated {
		t.Fatalf("unset worker env updated=%v err=%v", updated, err)
	}
	env, ok, err = workerAgentEnv(root, "alpha", "nova")
	if err != nil || !ok {
		t.Fatalf("worker env after unset ok=%v err=%v", ok, err)
	}
	if _, exists := env["RUNTIME_FLAG"]; exists {
		t.Fatalf("runtime flag was not removed: %#v", env)
	}
}
