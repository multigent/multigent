package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
)

func seedAgentWorkerForTest(t *testing.T, s *Server, workspaceID, project, name string) {
	t.Helper()
	key := strings.ReplaceAll(strings.ToLower(project+"-"+name), "_", "-")
	seedAgentWorkerWithIDForTest(t, s, workspaceID, project, name, "aw-test-"+key, "pm-test-"+key)
}

func agentWorkerScheduleForTest(t *testing.T, worker controldb.AgentWorker) *entity.HeartbeatConfig {
	t.Helper()
	hb := &entity.HeartbeatConfig{}
	if raw := strings.TrimSpace(worker.ScheduleJSON); raw != "" && raw != "{}" {
		if err := json.Unmarshal([]byte(raw), hb); err != nil {
			t.Fatalf("parse worker schedule: %v", err)
		}
	}
	return hb
}

func seedAgentWorkerWithIDForTest(t *testing.T, s *Server, workspaceID, project, name, workerID, membershipID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.controlDB.UpsertAgentWorker(controldb.AgentWorker{
		ID:          workerID,
		WorkspaceID: workspaceID,
		Name:        name,
		DisplayName: name,
		Model:       "codex",
		Status:      "available",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("seed agent worker %s: %v", name, err)
	}
	if err := s.controlDB.UpsertProjectMembership(controldb.ProjectMembership{
		ID:          membershipID,
		WorkspaceID: workspaceID,
		ProjectID:   project,
		MemberType:  "agent_worker",
		MemberID:    workerID,
		Title:       name,
		Role:        "developer",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("seed project membership %s/%s: %v", project, name, err)
	}
}
