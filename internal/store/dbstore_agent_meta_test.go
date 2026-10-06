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

func TestAgentMetaFromWorkerMembershipCarriesRunCommand(t *testing.T) {
	membership := controldb.ProjectMembership{Role: "runner"}
	for name, raw := range map[string]string{
		"canonical": `{"runCommand":"python3 adapter.py --prompt-file {prompt_file}","addDirs":["/repo"],"env":{"K":"V"}}`,
		"legacy":    `{"run_command":"python3 adapter.py --prompt-file {prompt_file}","add_dirs":["/repo"],"env":{"K":"V"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			worker := controldb.AgentWorker{ID: "aw-runner", Name: "runner", Model: string(entity.ModelGenericCLI), RuntimeConfigJSON: raw}
			meta := agentMetaFromWorkerMembership("example-project", worker, membership)
			if meta.RunCommand != "python3 adapter.py --prompt-file {prompt_file}" {
				t.Fatalf("RunCommand = %q", meta.RunCommand)
			}
			if len(meta.AddDirs) != 1 || meta.AddDirs[0] != "/repo" || meta.Env["K"] != "V" {
				t.Fatalf("meta = %#v", meta)
			}
		})
	}
}
