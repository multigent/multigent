package store

import (
	"testing"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
)

func TestAgentMetaFromWorkerMembershipPreservesRuntimeConfig(t *testing.T) {
	worker := controldb.AgentWorker{
		ID:                "aw-test",
		Name:              "qa-cursor",
		Model:             string(entity.ModelCursor),
		RuntimeConfigJSON: `{"env":{"RUNTIME_SECRET":"present"},"sandbox":{"provider":"docker","image":"runtime:test","networkMode":"bridge"},"addDirs":["/workspace/shared"],"runCommand":"codex exec","httpAgent":{"url":"http://model.test/v1/chat/completions","model":"test-model"}}`,
	}
	membership := controldb.ProjectMembership{Role: "qa"}

	meta := agentMetaFromWorkerMembership("example-project", worker, membership)
	if meta.Sandbox == nil {
		t.Fatal("expected sandbox config to be preserved")
	}
	if meta.Sandbox.Provider != entity.SandboxDocker {
		t.Fatalf("sandbox provider = %q, want docker", meta.Sandbox.Provider)
	}
	if meta.Sandbox.Image != "runtime:test" {
		t.Fatalf("sandbox image = %q, want runtime:test", meta.Sandbox.Image)
	}
	if meta.Env["RUNTIME_SECRET"] != "present" {
		t.Fatalf("runtime env was not preserved: %#v", meta.Env)
	}
	if len(meta.AddDirs) != 1 || meta.AddDirs[0] != "/workspace/shared" {
		t.Fatalf("add dirs were not preserved: %#v", meta.AddDirs)
	}
	if meta.RunCommand != "codex exec" {
		t.Fatalf("run command = %q, want codex exec", meta.RunCommand)
	}
	if meta.HTTPAgent == nil || meta.HTTPAgent.URL != "http://model.test/v1/chat/completions" || meta.HTTPAgent.Model != "test-model" {
		t.Fatalf("http agent config was not preserved: %#v", meta.HTTPAgent)
	}
}
