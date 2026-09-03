package store

import (
	"testing"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
)

func TestAgentMetaFromWorkerMembershipPreservesRuntimeSandbox(t *testing.T) {
	worker := controldb.AgentWorker{
		ID:                "aw-test",
		Name:              "qa-cursor",
		Model:             string(entity.ModelCursor),
		RuntimeConfigJSON: `{"sandbox":{"provider":"docker","image":"runtime:test","networkMode":"bridge"}}`,
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
}
