package db

import (
	"path/filepath"
	"testing"
	"time"
)

func TestRuntimeRunClaimLeaseAndExpiredReclaim(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "multigent.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	workspaceID := "ws-runtime"
	if err := db.UpsertWorkspace(Workspace{ID: workspaceID, Name: "Runtime", Slug: "runtime", Root: "/tmp/runtime", CreatedAt: nowUTC()}); err != nil {
		t.Fatalf("workspace: %v", err)
	}
	for _, node := range []RuntimeNode{
		{ID: "node-a", WorkspaceID: workspaceID, Name: "A", Kind: "personal_computer", Status: "online", LastSeenAt: nowUTC(), CreatedByUserID: "admin"},
		{ID: "node-b", WorkspaceID: workspaceID, Name: "B", Kind: "personal_computer", Status: "online", LastSeenAt: nowUTC(), CreatedByUserID: "admin"},
	} {
		if err := db.UpsertRuntimeNode(node); err != nil {
			t.Fatalf("node %s: %v", node.ID, err)
		}
	}
	if err := db.UpsertRuntimeRun(RuntimeRun{
		ID:                  "run-one",
		WorkspaceID:         workspaceID,
		AgentWorkerID:       "aw-one",
		ProjectMembershipID: "pm-one",
		ProjectID:           "project",
		AgentID:             "agent",
		TaskID:              "task-one",
		Status:              "queued",
		Priority:            1,
		SpecJSON:            `{"kind":"exec_prompt"}`,
		ResultJSON:          `{}`,
		CreatedAt:           time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
		UpdatedAt:           nowUTC(),
	}); err != nil {
		t.Fatalf("run: %v", err)
	}

	claimed, found, err := db.ClaimRuntimeRun(workspaceID, "node-a", 30, nil)
	if err != nil || !found {
		t.Fatalf("claim found=%v err=%v", found, err)
	}
	if claimed.Status != "running" || claimed.RuntimeNodeID != "node-a" || claimed.LeaseExpiresAt == "" {
		t.Fatalf("unexpected claimed run: %#v", claimed)
	}
	filtered, err := db.ListRuntimeRuns(RuntimeRunFilter{WorkspaceID: workspaceID, ProjectID: "project", AgentID: "agent", TaskID: "task-one", Status: "running"})
	if err != nil {
		t.Fatalf("filter by task: %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != "run-one" {
		t.Fatalf("unexpected task-filtered runs: %#v", filtered)
	}
	workerFiltered, err := db.ListRuntimeRuns(RuntimeRunFilter{WorkspaceID: workspaceID, AgentWorkerID: "aw-one", ProjectMembershipID: "pm-one", Status: "running"})
	if err != nil {
		t.Fatalf("filter by worker: %v", err)
	}
	if len(workerFiltered) != 1 || workerFiltered[0].ID != "run-one" {
		t.Fatalf("unexpected worker-filtered runs: %#v", workerFiltered)
	}

	renewed, found, err := db.ExtendRuntimeRunLease(workspaceID, "run-one", "node-a", 60)
	if err != nil || !found {
		t.Fatalf("renew found=%v err=%v", found, err)
	}
	if renewed.RuntimeNodeID != "node-a" || renewed.Status != "running" || renewed.LeaseExpiresAt == "" {
		t.Fatalf("unexpected renewed run: %#v", renewed)
	}

	_, found, err = db.ClaimRuntimeRun(workspaceID, "node-b", 30, nil)
	if err != nil {
		t.Fatalf("claim while leased: %v", err)
	}
	if found {
		t.Fatalf("leased running run should not be claimed by another node")
	}

	expired := renewed
	expired.LeaseExpiresAt = time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	if err := db.UpsertRuntimeRun(expired); err != nil {
		t.Fatalf("expire lease: %v", err)
	}
	reclaimed, found, err := db.ClaimRuntimeRun(workspaceID, "node-b", 30, nil)
	if err != nil || !found {
		t.Fatalf("reclaim found=%v err=%v", found, err)
	}
	if reclaimed.RuntimeNodeID != "node-b" || reclaimed.Status != "running" {
		t.Fatalf("unexpected reclaimed run: %#v", reclaimed)
	}

	cancelled := reclaimed
	cancelled.Status = "cancelled"
	cancelled.UpdatedAt = nowUTC()
	if err := db.UpsertRuntimeRun(cancelled); err != nil {
		t.Fatalf("cancel run: %v", err)
	}
	_, found, err = db.ClaimRuntimeRun(workspaceID, "node-a", 30, nil)
	if err != nil {
		t.Fatalf("claim cancelled: %v", err)
	}
	if found {
		t.Fatalf("cancelled run should not be claimed")
	}
}

func TestClaimRuntimeRunSkipsBusyAgent(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "multigent.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	workspaceID := "ws-runtime"
	if err := db.UpsertWorkspace(Workspace{ID: workspaceID, Name: "Runtime", Slug: "runtime", Root: "/tmp/runtime", CreatedAt: nowUTC()}); err != nil {
		t.Fatalf("workspace: %v", err)
	}
	now := time.Now().UTC()
	for _, run := range []RuntimeRun{
		{
			ID:          "run-busy",
			WorkspaceID: workspaceID,
			ProjectID:   "project",
			AgentID:     "agent-a",
			Status:      "queued",
			Priority:    1,
			SpecJSON:    `{"kind":"exec_prompt"}`,
			ResultJSON:  `{}`,
			CreatedAt:   now.Add(-2 * time.Minute).Format(time.RFC3339),
			UpdatedAt:   nowUTC(),
		},
		{
			ID:          "run-free",
			WorkspaceID: workspaceID,
			ProjectID:   "project",
			AgentID:     "agent-b",
			Status:      "queued",
			Priority:    2,
			SpecJSON:    `{"kind":"exec_prompt"}`,
			ResultJSON:  `{}`,
			CreatedAt:   now.Add(-time.Minute).Format(time.RFC3339),
			UpdatedAt:   nowUTC(),
		},
	} {
		if err := db.UpsertRuntimeRun(run); err != nil {
			t.Fatalf("run %s: %v", run.ID, err)
		}
	}

	claimed, found, err := db.ClaimRuntimeRun(workspaceID, "node-a", 30, []string{"project/agent-a"})
	if err != nil || !found {
		t.Fatalf("claim found=%v err=%v", found, err)
	}
	if claimed.ID != "run-free" {
		t.Fatalf("expected run-free, got %#v", claimed)
	}
}

func TestClaimRuntimeRunSkipsBusyAgentWorker(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "multigent.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	workspaceID := "ws-runtime"
	if err := db.UpsertWorkspace(Workspace{ID: workspaceID, Name: "Runtime", Slug: "runtime", Root: "/tmp/runtime", CreatedAt: nowUTC()}); err != nil {
		t.Fatalf("workspace: %v", err)
	}
	now := time.Now().UTC()
	for _, run := range []RuntimeRun{
		{
			ID:            "run-busy-worker",
			WorkspaceID:   workspaceID,
			AgentWorkerID: "aw-busy",
			ProjectID:     "project-a",
			AgentID:       "agent-a",
			Status:        "queued",
			Priority:      1,
			SpecJSON:      `{"kind":"exec_prompt"}`,
			ResultJSON:    `{}`,
			CreatedAt:     now.Add(-2 * time.Minute).Format(time.RFC3339),
			UpdatedAt:     nowUTC(),
		},
		{
			ID:            "run-free-worker",
			WorkspaceID:   workspaceID,
			AgentWorkerID: "aw-free",
			ProjectID:     "project-b",
			AgentID:       "agent-b",
			Status:        "queued",
			Priority:      2,
			SpecJSON:      `{"kind":"exec_prompt"}`,
			ResultJSON:    `{}`,
			CreatedAt:     now.Add(-time.Minute).Format(time.RFC3339),
			UpdatedAt:     nowUTC(),
		},
	} {
		if err := db.UpsertRuntimeRun(run); err != nil {
			t.Fatalf("run %s: %v", run.ID, err)
		}
	}

	claimed, found, err := db.ClaimRuntimeRun(workspaceID, "node-a", 30, []string{"worker/aw-busy"})
	if err != nil || !found {
		t.Fatalf("claim found=%v err=%v", found, err)
	}
	if claimed.ID != "run-free-worker" {
		t.Fatalf("expected run-free-worker, got %#v", claimed)
	}
}
