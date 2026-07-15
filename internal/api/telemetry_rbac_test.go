package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/multigent/multigent/internal/telemetry"
)

func seedTelemetryRun(t *testing.T, s *Server, project, agent, logBody string) string {
	t.Helper()
	logPath := filepath.Join(s.root, ".multigent", "test-logs", project+"-"+agent+".log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	if err := os.WriteFile(logPath, []byte(logBody), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}
	logPathRel := telemetry.RelLogPath(s.root, logPath)
	now := time.Now().UTC()
	if err := telemetry.Insert(s.root, telemetry.Record{
		Kind:           telemetry.KindTask,
		StartedAt:      now.Add(-time.Minute),
		FinishedAt:     now,
		Project:        project,
		Agent:          agent,
		TaskID:         "task-" + agent,
		TaskTitle:      "Task " + agent,
		Model:          "codex",
		Status:         "done_success",
		LogPathRel:     logPathRel,
		CommandSummary: "run " + agent,
		InputTokens:    sql.NullInt64{Int64: 10, Valid: true},
		OutputTokens:   sql.NullInt64{Int64: 5, Valid: true},
	}); err != nil {
		t.Fatalf("insert telemetry: %v", err)
	}
	return logPathRel
}

func TestTelemetryRunsAreFilteredByAgentAccess(t *testing.T) {
	s, _ := newConnectionGrantPolicyServer(t)
	pmLog := seedTelemetryRun(t, s, "tapnow", "pm", "pm log")
	backendLog := seedTelemetryRun(t, s, "tapnow", "backend", "backend log")

	ownerRec := httptest.NewRecorder()
	s.handleTelemetryRuns(ownerRec, providerTestRequest(http.MethodGet, "/api/v1/telemetry/runs?allTime=1", "owner", nil))
	if ownerRec.Code != http.StatusOK {
		t.Fatalf("owner runs status=%d body=%s", ownerRec.Code, ownerRec.Body.String())
	}
	var ownerBody struct {
		Runs []map[string]any `json:"runs"`
	}
	if err := json.Unmarshal(ownerRec.Body.Bytes(), &ownerBody); err != nil {
		t.Fatalf("decode owner runs: %v", err)
	}
	if len(ownerBody.Runs) != 1 || ownerBody.Runs[0]["agent"] != "pm" {
		t.Fatalf("owner runs not filtered: %#v", ownerBody.Runs)
	}

	pmLogRec := httptest.NewRecorder()
	s.handleTelemetryLog(pmLogRec, providerTestRequest(http.MethodGet, "/api/v1/telemetry/log?path="+pmLog, "owner", nil))
	if pmLogRec.Code != http.StatusOK {
		t.Fatalf("owner pm log status=%d body=%s", pmLogRec.Code, pmLogRec.Body.String())
	}

	backendLogRec := httptest.NewRecorder()
	s.handleTelemetryLog(backendLogRec, providerTestRequest(http.MethodGet, "/api/v1/telemetry/log?path="+backendLog, "owner", nil))
	if backendLogRec.Code != http.StatusForbidden {
		t.Fatalf("owner backend log status=%d body=%s", backendLogRec.Code, backendLogRec.Body.String())
	}

	adminRec := httptest.NewRecorder()
	s.handleTelemetryRuns(adminRec, providerTestRequest(http.MethodGet, "/api/v1/telemetry/runs?allTime=1", "admin", nil))
	if adminRec.Code != http.StatusOK {
		t.Fatalf("admin runs status=%d body=%s", adminRec.Code, adminRec.Body.String())
	}
	var adminBody struct {
		Runs []map[string]any `json:"runs"`
	}
	if err := json.Unmarshal(adminRec.Body.Bytes(), &adminBody); err != nil {
		t.Fatalf("decode admin runs: %v", err)
	}
	if len(adminBody.Runs) != 2 {
		t.Fatalf("admin runs should include all accessible runs: %#v", adminBody.Runs)
	}
}
