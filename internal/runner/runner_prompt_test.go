package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/store"
)

func TestBuildTaskPromptIncludesAgentWorkerContract(t *testing.T) {
	root := t.TempDir()
	db, err := controldb.Open(filepath.Join(root, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := os.MkdirAll(filepath.Join(root, ".multigent"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".multigent", "agency.yaml"), []byte("name: test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspaceID := filepath.Base(root)
	if err := db.UpsertWorkspace(controldb.Workspace{
		ID:   workspaceID,
		Name: "Prompt test",
		Slug: workspaceID,
		Root: root,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertAgentWorker(controldb.AgentWorker{
		ID:            "aw-dispatcher",
		WorkspaceID:   workspaceID,
		Name:          "dispatcher",
		ProfilePrompt: "You are a dispatcher. Never perform the review yourself.",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertProjectMembership(controldb.ProjectMembership{
		ID:          "pm-dispatcher",
		WorkspaceID: workspaceID,
		ProjectID:   "review-project",
		MemberType:  "agent_worker",
		MemberID:    "aw-dispatcher",
		Role:        "dispatcher",
	}); err != nil {
		t.Fatal(err)
	}

	prompt := (&Runner{
		agentStore: store.NewDB(root, db),
	}).BuildTaskPrompt("review-project", "dispatcher", &entity.Task{
		ID:     "task-1",
		Prompt: "Handle the incoming request.",
	})

	for _, want := range []string{
		"## Agent Long-term Prompt",
		"Never perform the review yourself.",
		"## Current Task",
		"Handle the incoming request.",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt does not contain %q:\n%s", want, prompt)
		}
	}
}
