package ctxbuild

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/multigent/multigent/internal/contextpack"
	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/store"
)

func TestLoadSkillFilesFollowsSymlinkRoot(t *testing.T) {
	tmp := t.TempDir()
	realDir := filepath.Join(tmp, "registry", "repo-image-hosting", "v1")
	if err := os.MkdirAll(filepath.Join(realDir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "SKILL.md"), []byte("# Skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "scripts", "upload.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	linkDir := filepath.Join(tmp, "skills", "repo-image-hosting")
	if err := os.MkdirAll(filepath.Dir(linkDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	files := loadSkillFiles(linkDir)
	if len(files) != 1 {
		t.Fatalf("expected 1 bundled file, got %d: %#v", len(files), files)
	}
	if files[0].Name != filepath.Join("scripts", "upload.sh") {
		t.Fatalf("unexpected bundled file name: %q", files[0].Name)
	}
}

func TestBuildForAgentIncludesAgentWorkerMembershipContext(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".multigent"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, ".multigent", "agency.yaml"), []byte("name: Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "agency-prompt.md"), []byte("Agency rules."), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, "projects", "sample"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "projects", "sample", "prompt.md"), []byte("Sample project context."), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := controldb.Open(filepath.Join(t.TempDir(), "multigent.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	st := store.NewDB(tmp, db)
	workspaces, err := db.ListWorkspaces()
	if err != nil || len(workspaces) != 1 {
		t.Fatalf("workspace len=%d err=%v", len(workspaces), err)
	}
	workspaceID := workspaces[0].ID
	if err := db.UpsertAgentWorker(controldb.AgentWorker{
		ID:          "aw-manager-agent",
		WorkspaceID: workspaceID,
		Name:        "manager-agent",
		DisplayName: "manager-agent",
		Description: "Cross-project PM agent",
		ProfilePrompt: strings.Join([]string{
			"owner-a is my final escalation owner.",
			"Treat workflows as milestones that may span multiple wakeups.",
		}, "\n"),
		Model:      "codex",
		SkillsJSON: `["customer-agent-debug"]`,
	}); err != nil {
		t.Fatalf("worker: %v", err)
	}
	for _, membership := range []controldb.ProjectMembership{
		{
			ID:               "pm-sample",
			WorkspaceID:      workspaceID,
			ProjectID:        "sample",
			MemberType:       "agent_worker",
			MemberID:         "aw-manager-agent",
			Role:             "project-manager",
			Title:            "manager-agent",
			Prompt:           "You own sample delivery coordination.",
			PermissionsJSON:  `["task.read","workflow.write"]`,
			AutoPickTasks:    true,
			AttentionEnabled: true,
		},
		{
			ID:               "pm-other",
			WorkspaceID:      workspaceID,
			ProjectID:        "other",
			MemberType:       "agent_worker",
			MemberID:         "aw-manager-agent",
			Role:             "reviewer",
			Title:            "manager-agent",
			AutoPickTasks:    false,
			AttentionEnabled: true,
		},
	} {
		if err := db.UpsertProjectMembership(membership); err != nil {
			t.Fatalf("membership %s: %v", membership.ID, err)
		}
	}
	imported, err := contextpack.NewStore(tmp).ImportManual(contextpack.ImportManualInput{
		Title:       "Imported project handoff",
		Content:     "Critical handoff notes for manager-agent.",
		SourceName:  "handoff.md",
		Project:     "sample",
		BindScope:   contextpack.ScopeAgent,
		BindScopeID: "sample/manager-agent",
		Required:    true,
	})
	if err != nil {
		t.Fatalf("import context: %v", err)
	}
	if imported.Binding == nil {
		t.Fatalf("expected binding")
	}

	mc, err := NewBuilder(st).BuildForAgent("sample", "manager-agent", "", "")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	text := strings.Join(layerContents(mc.Layers), "\n")
	for _, want := range []string{
		"## Agent Worker Identity",
		"Worker ID: aw-manager-agent",
		"## Agent Long-term Prompt",
		"owner-a is my final escalation owner.",
		"Treat workflows as milestones that may span multiple wakeups.",
		"Project: sample",
		"Membership ID: pm-sample",
		"You own sample delivery coordination.",
		"## Other Project Memberships",
		"`other`: role `reviewer`",
		"# Linked Reference Material",
		"Imported project handoff",
		"Context ID: `",
		"Scope: `agent:sample/manager-agent`",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("context missing %q:\n%s", want, text)
		}
	}
}

func layerContents(layers []ContextLayer) []string {
	out := make([]string, 0, len(layers))
	for _, layer := range layers {
		out = append(out, layer.Content)
	}
	return out
}
