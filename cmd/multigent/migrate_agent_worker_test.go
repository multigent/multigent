package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/store"
	"github.com/multigent/multigent/internal/taskstore"
	"gopkg.in/yaml.v3"
)

func TestAgentWorkerMigrationReadsLegacyAgentsAndMigratesTasks(t *testing.T) {
	root := t.TempDir()
	writeYAMLForTest(t, filepath.Join(root, ".agencycli", "agency.yaml"), map[string]string{"name": "Legacy"})
	writeYAMLForTest(t, filepath.Join(root, "projects", "sample", "project.yaml"), entity.Project{Name: "sample"})
	writeYAMLForTest(t, filepath.Join(root, "projects", "sample", "agents", "pm", ".agencycli", "agent.yaml"), entity.AgentMeta{
		Name:    "pm",
		Project: "sample",
		Team:    "product",
		Role:    "pm",
		Model:   entity.ModelCodex,
	})
	writeYAMLForTest(t, filepath.Join(root, "projects", "sample", "agents", "pm", ".agencycli", "tasks.yaml"), []*entity.Task{{
		ID:        "t-one",
		Title:     "Do the work",
		Priority:  2,
		Assignee:  "sample/pm",
		Status:    entity.TaskStatusPending,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}})

	db, err := controldb.Open(filepath.Join(t.TempDir(), "multigent.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	workspaceID := legacyMigrationWorkspaceID(root)
	plan, err := buildAgentWorkerMigrationPlan(root, workspaceID, store.NewFS(root), taskstore.New(root), nil)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if plan.Summary.AgentsScanned != 1 || plan.Summary.ActiveTasksScanned != 1 || plan.Summary.TaskAssigneesToRewrite != 1 {
		t.Fatalf("unexpected plan summary: %+v", plan.Summary)
	}
	if err := ensureMigrationWorkspace(db, root, workspaceID); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := db.UpsertRuntimeRun(controldb.RuntimeRun{
		ID:          "rtrun-one",
		WorkspaceID: workspaceID,
		ProjectID:   "sample",
		AgentID:     "pm",
		TaskID:      "t-one",
		Status:      "running",
		Priority:    1,
		SpecJSON:    `{"kind":"task"}`,
		ResultJSON:  `{}`,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("seed runtime run: %v", err)
	}
	if err := db.CreateInteractionRequest(controldb.InteractionRequest{
		ID:          "ir-one",
		WorkspaceID: workspaceID,
		ProjectID:   "sample",
		AgentID:     "pm",
		Provider:    "feishu",
		Recipient:   "owner",
		TargetType:  "user",
		Title:       "Decision",
		Body:        "Please choose",
		Status:      "active",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("seed interaction request: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err := db.UpsertConnection(controldb.Connection{
		ID:             "conn-github",
		WorkspaceID:    workspaceID,
		Provider:       "github",
		ConnectionName: "default",
		OwnerType:      "workspace",
		OwnerID:        workspaceID,
		AuthType:       "token",
		Status:         "active",
		ProfileJSON:    "{}",
		CreatedBy:      "admin",
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	if err := db.UpsertConnection(controldb.Connection{
		ID:             "conn-feishu-channel",
		WorkspaceID:    workspaceID,
		Provider:       "feishu",
		ConnectionName: "agent-sample-pm",
		OwnerType:      "workspace",
		OwnerID:        workspaceID,
		AuthType:       "app_secret",
		Status:         "active",
		ProfileJSON:    `{"purpose":"agent_channel","usage":"agent_im_channel"}`,
		CreatedBy:      "admin",
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("seed channel connection: %v", err)
	}
	if err := db.UpsertAgentToolBinding(controldb.AgentToolBinding{
		ID:           "bind-github",
		WorkspaceID:  workspaceID,
		ProjectID:    "sample",
		AgentID:      "pm",
		ConnectionID: "conn-github",
		Provider:     "github",
		AdapterType:  "github",
		Status:       "enabled",
		ConfigJSON:   "{}",
		CreatedBy:    "admin",
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("seed tool binding: %v", err)
	}
	if err := db.CreateConnectionGrant(controldb.ConnectionGrant{
		ID:           "grant-legacy-project",
		WorkspaceID:  workspaceID,
		ConnectionID: "conn-github",
		TargetType:   "project",
		TargetID:     "sample",
		CreatedBy:    "admin",
		CreatedAt:    now,
	}); err != nil {
		t.Fatalf("seed legacy project grant: %v", err)
	}
	if err := db.CreateConnectionGrant(controldb.ConnectionGrant{
		ID:           "grant-legacy-agent",
		WorkspaceID:  workspaceID,
		ConnectionID: "conn-github",
		TargetType:   "agent",
		TargetID:     "sample/pm",
		CreatedBy:    "admin",
		CreatedAt:    now,
	}); err != nil {
		t.Fatalf("seed legacy agent grant: %v", err)
	}
	if err := db.UpsertAgentChannelBinding(controldb.AgentChannelBinding{
		ID:           "chan-feishu",
		WorkspaceID:  workspaceID,
		ProjectID:    "sample",
		AgentID:      "pm",
		Provider:     "feishu",
		ConnectionID: "conn-feishu-channel",
		Status:       "connected",
		MetadataJSON: "{}",
		CreatedBy:    "admin",
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("seed channel binding: %v", err)
	}
	if err := applyAgentWorkerMigrationPlan(db, taskstore.New(root), &plan); err != nil {
		t.Fatalf("apply plan: %v", err)
	}
	if plan.Summary.ToolBindingsMigrated != 1 || plan.Summary.LegacyGrantsRemoved != 2 {
		t.Fatalf("migration report did not record tool grant cleanup: %+v", plan.Summary)
	}
	workers, err := db.ListAgentWorkers(workspaceID)
	if err != nil {
		t.Fatalf("list workers: %v", err)
	}
	if len(workers) != 1 || workers[0].Name != "pm" {
		t.Fatalf("unexpected workers: %+v", workers)
	}
	memberships, err := db.ListProjectMemberships(controldb.ProjectMembershipFilter{
		WorkspaceID: workspaceID,
		ProjectID:   "sample",
		MemberType:  "agent_worker",
	})
	if err != nil {
		t.Fatalf("list memberships: %v", err)
	}
	if len(memberships) != 1 || memberships[0].MemberID != workers[0].ID {
		t.Fatalf("unexpected memberships: %+v", memberships)
	}
	raw, ok, err := db.GetRecord("tasks", workspaceID, []string{"sample", "pm", "t-one"})
	if err != nil {
		t.Fatalf("get task record: %v", err)
	}
	if !ok {
		t.Fatal("task record not migrated")
	}
	var task entity.Task
	if err := json.Unmarshal([]byte(raw), &task); err != nil {
		t.Fatalf("unmarshal task: %v", err)
	}
	if task.AssigneeType != "agent_worker" || task.AssigneeID != workers[0].ID || task.AssigneeMembershipID != memberships[0].ID {
		t.Fatalf("unexpected migrated task assignee: %+v", task)
	}
	runs, err := db.ListRuntimeRuns(controldb.RuntimeRunFilter{
		WorkspaceID:         workspaceID,
		AgentWorkerID:       workers[0].ID,
		ProjectMembershipID: memberships[0].ID,
		Status:              "running",
	})
	if err != nil {
		t.Fatalf("list migrated runtime runs: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != "rtrun-one" {
		t.Fatalf("unexpected migrated runtime runs: %+v", runs)
	}
	requests, err := db.ListInteractionRequests(controldb.InteractionRequestFilter{
		WorkspaceID:   workspaceID,
		AgentWorkerID: workers[0].ID,
		Status:        "active",
	})
	if err != nil {
		t.Fatalf("list migrated interaction requests: %v", err)
	}
	if len(requests) != 1 || requests[0].ID != "ir-one" {
		t.Fatalf("unexpected migrated interaction requests: %+v", requests)
	}
	toolBindings, err := db.ListAgentToolBindings(controldb.AgentToolBindingFilter{
		WorkspaceID:   workspaceID,
		AgentWorkerID: workers[0].ID,
		ConnectionID:  "conn-github",
	})
	if err != nil {
		t.Fatalf("list migrated tool bindings: %v", err)
	}
	if len(toolBindings) != 1 || toolBindings[0].ID != "bind-github" {
		t.Fatalf("tool binding was not attached to worker: %+v", toolBindings)
	}
	if toolBindings[0].ProjectID != "" || toolBindings[0].AgentID != "" {
		t.Fatalf("tool binding retained legacy project identity: %+v", toolBindings[0])
	}
	grants, err := db.ListConnectionGrants("conn-github")
	if err != nil {
		t.Fatalf("list connection grants: %v", err)
	}
	if len(grants) != 1 || grants[0].TargetType != "agent" || grants[0].TargetID != "agent_worker:"+workers[0].ID {
		t.Fatalf("legacy tool grants were not canonicalized: %+v", grants)
	}
	channelBindings, err := db.ListAgentChannelBindings(controldb.AgentChannelBindingFilter{
		WorkspaceID:   workspaceID,
		AgentWorkerID: workers[0].ID,
		ConnectionID:  "conn-feishu-channel",
	})
	if err != nil {
		t.Fatalf("list migrated channel bindings: %v", err)
	}
	if len(channelBindings) != 1 || channelBindings[0].ID != "chan-feishu" {
		t.Fatalf("channel binding was not attached to worker: %+v", channelBindings)
	}
	channelGrants, err := db.ListConnectionGrants("conn-feishu-channel")
	if err != nil {
		t.Fatalf("list channel connection grants: %v", err)
	}
	if len(channelGrants) != 1 || channelGrants[0].TargetType != "agent" || channelGrants[0].TargetID != "agent_worker:"+workers[0].ID {
		t.Fatalf("legacy channel binding did not create worker grant: %+v", channelGrants)
	}
}

func TestOpenMigrationWorkspaceDBUsesDataRootControlDB(t *testing.T) {
	dataRoot := t.TempDir()
	root := filepath.Join(dataRoot, "workspace-one")
	writeYAMLForTest(t, filepath.Join(root, ".multigent", "agency.yaml"), map[string]string{"name": "Workspace One"})
	t.Setenv("MULTIGENT_DATA_DIR", dataRoot)

	db, workspaceID, err := openMigrationWorkspaceDB(root)
	if err != nil {
		t.Fatalf("open migration db: %v", err)
	}
	if workspaceID == "" {
		t.Fatal("workspace id is empty")
	}
	if err := ensureMigrationWorkspace(db, root, workspaceID); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := db.UpsertAgentWorker(controldb.AgentWorker{
		ID:          "aw-central",
		WorkspaceID: workspaceID,
		Name:        "central-worker",
		DisplayName: "central-worker",
		Status:      "active",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("upsert worker: %v", err)
	}
	_ = db.Close()

	central, err := controldb.Open(filepath.Join(dataRoot, ".multigent", "multigent.db"))
	if err != nil {
		t.Fatalf("open central db: %v", err)
	}
	defer central.Close()
	if _, ok, err := central.AgentWorkerByName(workspaceID, "central-worker"); err != nil || !ok {
		t.Fatalf("worker not written to data root control db ok=%v err=%v", ok, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".multigent", "multigent.db")); !os.IsNotExist(err) {
		t.Fatalf("workspace-local db should not be created, err=%v", err)
	}
}

func TestAgentWorkerMigrationReadsLegacyDBAgentProviders(t *testing.T) {
	root := t.TempDir()
	writeYAMLForTest(t, filepath.Join(root, ".agencycli", "agency.yaml"), map[string]string{"name": "Legacy"})
	writeYAMLForTest(t, filepath.Join(root, "projects", "sample", "project.yaml"), entity.Project{Name: "sample"})
	if err := os.MkdirAll(filepath.Join(root, "projects", "sample", "agents", "dev"), 0o755); err != nil {
		t.Fatalf("mkdir agent dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "projects", "sample", "agents", "dev", "AGENTS.md"), []byte("# Agent\n"), 0o644); err != nil {
		t.Fatalf("write context marker: %v", err)
	}
	db, err := controldb.Open(filepath.Join(t.TempDir(), "multigent.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	workspaceID := legacyMigrationWorkspaceID(root)
	if err := ensureMigrationWorkspace(db, root, workspaceID); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	legacyMeta := entity.AgentMeta{
		Name:         "dev",
		Project:      "sample",
		Team:         "engineering",
		Role:         "developer",
		Model:        entity.ModelCodex,
		Provider:     "prov-codex",
		RuntimeModel: "gpt-5.5",
	}
	raw, err := json.Marshal(legacyMeta)
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}
	if err := db.UpsertRecord("agents", workspaceID, []string{"sample", "dev"}, string(raw)); err != nil {
		t.Fatalf("upsert legacy agent record: %v", err)
	}
	plan, err := buildAgentWorkerMigrationPlan(root, workspaceID, store.NewDB(root, db), taskstore.NewDB(root, db), db)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if len(plan.Workers) != 1 {
		t.Fatalf("expected one worker: %+v", plan.Workers)
	}
	worker := plan.Workers[0]
	if worker.Provider != "prov-codex" || worker.RuntimeModel != "gpt-5.5" || worker.Membership.Role != "developer" {
		t.Fatalf("legacy DB metadata was not preserved: %+v", worker)
	}
	if err := applyAgentWorkerMigrationPlan(db, taskstore.NewDB(root, db), &plan); err != nil {
		t.Fatalf("apply plan: %v", err)
	}
	created, ok, err := db.AgentWorkerByName(workspaceID, "dev")
	if err != nil || !ok {
		t.Fatalf("created worker ok=%v err=%v", ok, err)
	}
	if created.DefaultModelAccountID != "prov-codex" || created.RuntimeModel != "gpt-5.5" {
		t.Fatalf("worker model account was not migrated: %+v", created)
	}
}

func TestAgentWorkerMigrationMergesLegacyDBProviderIntoFileAgent(t *testing.T) {
	root := t.TempDir()
	writeYAMLForTest(t, filepath.Join(root, ".agencycli", "agency.yaml"), map[string]string{"name": "Legacy"})
	writeYAMLForTest(t, filepath.Join(root, "projects", "sample", "project.yaml"), entity.Project{Name: "sample"})
	writeYAMLForTest(t, filepath.Join(root, "projects", "sample", "agents", "dev", ".multigent", "agent.yaml"), entity.AgentMeta{
		Name:    "dev",
		Project: "sample",
		Team:    "engineering",
		Role:    "developer",
		Model:   entity.ModelCodex,
	})
	db, err := controldb.Open(filepath.Join(t.TempDir(), "multigent.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	workspaceID := legacyMigrationWorkspaceID(root)
	if err := ensureMigrationWorkspace(db, root, workspaceID); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	legacyMeta := entity.AgentMeta{
		Name:          "dev",
		Project:       "sample",
		Model:         entity.ModelCodex,
		Provider:      "prov-codex",
		RuntimeModel:  "gpt-5.5",
		RuntimeNodeID: "node-one",
	}
	raw, err := json.Marshal(legacyMeta)
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}
	if err := db.UpsertRecord("agents", workspaceID, []string{"sample", "dev"}, string(raw)); err != nil {
		t.Fatalf("upsert legacy agent record: %v", err)
	}
	plan, err := buildAgentWorkerMigrationPlan(root, workspaceID, store.NewFS(root), taskstore.New(root), db)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if len(plan.Workers) != 1 {
		t.Fatalf("expected one worker: %+v", plan.Workers)
	}
	worker := plan.Workers[0]
	if worker.Provider != "prov-codex" || worker.RuntimeModel != "gpt-5.5" || worker.RuntimeNodeID != "node-one" {
		t.Fatalf("legacy DB provider fields were not merged into file agent: %+v", worker)
	}
}

func TestAgentWorkerMigrationKeepsHumanMembersOutOfWorkers(t *testing.T) {
	root := t.TempDir()
	writeYAMLForTest(t, filepath.Join(root, ".agencycli", "agency.yaml"), map[string]string{"name": "Legacy"})
	writeYAMLForTest(t, filepath.Join(root, "projects", "sample", "project.yaml"), entity.Project{Name: "sample"})
	writeYAMLForTest(t, filepath.Join(root, "projects", "sample", "agents", "admin", ".agencycli", "agent.yaml"), entity.AgentMeta{
		Name:    "admin",
		Project: "sample",
		Role:    "owner",
		Model:   entity.ModelHuman,
	})

	db, err := controldb.Open(filepath.Join(t.TempDir(), "multigent.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	workspaceID := legacyMigrationWorkspaceID(root)
	if err := ensureMigrationWorkspace(db, root, workspaceID); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	staleWorkerID := stableMigrationID("aw", workspaceID, "sample", "admin")
	staleMembershipID := stableMigrationID("pm", workspaceID, "sample", "admin")
	if err := db.UpsertAgentWorker(controldb.AgentWorker{
		ID:          staleWorkerID,
		WorkspaceID: workspaceID,
		Name:        "admin",
		DisplayName: "admin",
		Status:      "active",
		Model:       string(entity.ModelHuman),
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("seed stale worker: %v", err)
	}
	if err := db.UpsertProjectMembership(controldb.ProjectMembership{
		ID:          staleMembershipID,
		WorkspaceID: workspaceID,
		ProjectID:   "sample",
		MemberType:  "agent_worker",
		MemberID:    staleWorkerID,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("seed stale membership: %v", err)
	}

	plan, err := buildAgentWorkerMigrationPlan(root, workspaceID, store.NewFS(root), taskstore.New(root), nil)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if plan.Summary.WorkersPlanned != 0 || plan.Summary.HumanMemberships != 1 {
		t.Fatalf("unexpected summary: %+v", plan.Summary)
	}
	if err := applyAgentWorkerMigrationPlan(db, taskstore.New(root), &plan); err != nil {
		t.Fatalf("apply plan: %v", err)
	}
	if workers, err := db.ListAgentWorkers(workspaceID); err != nil {
		t.Fatalf("list workers: %v", err)
	} else if len(workers) != 0 {
		t.Fatalf("human should not be migrated as worker: %+v", workers)
	}
	memberships, err := db.ListProjectMemberships(controldb.ProjectMembershipFilter{
		WorkspaceID: workspaceID,
		ProjectID:   "sample",
	})
	if err != nil {
		t.Fatalf("list memberships: %v", err)
	}
	if len(memberships) != 1 || memberships[0].MemberType != "user" || memberships[0].MemberID != "admin" {
		t.Fatalf("unexpected memberships: %+v", memberships)
	}
}

func TestCreateWorkspaceBackupArchive(t *testing.T) {
	root := t.TempDir()
	writeYAMLForTest(t, filepath.Join(root, ".agencycli", "agency.yaml"), map[string]string{"name": "Legacy"})
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("hello backup"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	socketPath := filepath.Join(root, "runtime.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix socket: %v", err)
	}
	defer listener.Close()
	if err := createWorkspaceBackupArchive(root, filepath.Join(root, "inside.tar.gz")); err == nil {
		t.Fatalf("expected backup inside workspace to be rejected")
	}
	backup := filepath.Join(t.TempDir(), "workspace-backup.tar.gz")
	if err := createWorkspaceBackupArchive(root, backup); err != nil {
		t.Fatalf("backup: %v", err)
	}
	f, err := os.Open(backup)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	foundAgency := false
	foundNote := false
	foundSocket := false
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar next: %v", err)
		}
		if strings.HasSuffix(header.Name, "/.agencycli/agency.yaml") {
			foundAgency = true
		}
		if strings.HasSuffix(header.Name, "/notes.md") {
			foundNote = true
		}
		if strings.HasSuffix(header.Name, "/runtime.sock") {
			foundSocket = true
		}
	}
	if !foundAgency || !foundNote {
		t.Fatalf("backup missing expected files: agency=%v note=%v", foundAgency, foundNote)
	}
	if foundSocket {
		t.Fatal("backup should skip unix sockets")
	}
}

func writeYAMLForTest(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	data, err := yaml.Marshal(value)
	if err != nil {
		t.Fatalf("marshal yaml: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
