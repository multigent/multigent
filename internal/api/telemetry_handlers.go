package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/telemetry"
)

func (s *Server) handleTelemetrySummary(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	since := strings.TrimSpace(q.Get("since"))
	until := strings.TrimSpace(q.Get("until"))
	allTime := q.Get("allTime") == "1" || strings.EqualFold(q.Get("allTime"), "true")
	project := strings.TrimSpace(q.Get("project"))
	if project != "" && !s.checkProjectAccess(w, r, project) {
		return
	}

	from, to, err := telemetry.ParseWindow(since, until, allTime, time.Now(), time.Local)
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	db, err := telemetry.OpenReadOnly(s.root)
	if err != nil {
		if err == telemetry.ErrNoDatabase {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"window":    windowJSON(from, to, allTime),
				"available": false,
				"summary":   nil,
				"byAgent":   []any{},
			})
			return
		}
		s.serverError(w, err)
		return
	}
	defer db.Close()

	rows, err := telemetry.ReadRuns(db, from, to, project)
	if err != nil {
		s.serverError(w, err)
		return
	}
	rows = s.filterTelemetryRunsForRequest(r, rows)
	rows = s.enrichTelemetryUsageFromLogs(rows)
	sum := telemetry.Summarize(rows)
	byAgent := telemetry.SummarizeByAgent(rows)
	agentMeta := s.telemetryAgentMetadata()

	_ = json.NewEncoder(w).Encode(map[string]any{
		"window":    windowJSON(from, to, allTime),
		"available": true,
		"summary":   summaryJSON(sum),
		"byAgent":   s.agentSummariesJSON(byAgent, agentMeta),
	})
}

func windowJSON(from, to *time.Time, allTime bool) map[string]any {
	out := map[string]any{"allTime": allTime}
	if from != nil {
		out["from"] = from.UTC().Format(time.RFC3339Nano)
	}
	if to != nil {
		out["to"] = to.UTC().Format(time.RFC3339Nano)
	}
	return out
}

func summaryJSON(sum telemetry.Summary) map[string]any {
	return map[string]any{
		"runs":            sum.Runs,
		"taskRuns":        sum.TaskRuns,
		"execRuns":        sum.ExecRuns,
		"inputTokens":     sum.InputTokens,
		"outputTokens":    sum.OutputTokens,
		"cacheReadTokens": sum.CacheReadTokens,
		"costUSD":         sum.CostUSD,
		"runsWithCost":    sum.RunsWithCost,
		"success":         sum.Success,
		"failed":          sum.Failed,
		"awaiting":        sum.Awaiting,
		"other":           sum.Other,
		"wallDurationMs":  sum.WallDuration.Milliseconds(),
	}
}

func (s *Server) agentSummariesJSON(in []telemetry.AgentSummary, meta telemetryAgentMetaMap) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	merged := make(map[string]map[string]any)
	order := make([]string, 0, len(in))
	for _, a := range in {
		item := map[string]any{
			"project":         a.Project,
			"agent":           a.Agent,
			"runs":            a.Runs,
			"task":            a.Task,
			"exec":            a.Exec,
			"inputTokens":     a.InputTokens,
			"outputTokens":    a.OutputTokens,
			"cacheReadTokens": a.CacheReadTokens,
			"costUSD":         a.CostUSD,
			"runsWithCost":    a.RunsWithCost,
			"success":         a.Success,
			"failed":          a.Failed,
			"awaiting":        a.Awaiting,
			"other":           a.Other,
			"wallDurationMs":  a.WallDuration.Milliseconds(),
		}
		s.applyTelemetryAgentMetadata(item, meta, a.Project, a.Agent, "", "")
		key := strings.TrimSpace(stringFromAny(item["agentWorkerId"]))
		if key == "" {
			key = telemetryProjectAgentKey(a.Project, a.Agent)
		}
		if existing, ok := merged[key]; ok {
			addInt64Field(existing, "runs", a.Runs)
			addInt64Field(existing, "task", a.Task)
			addInt64Field(existing, "exec", a.Exec)
			addInt64Field(existing, "inputTokens", a.InputTokens)
			addInt64Field(existing, "outputTokens", a.OutputTokens)
			addInt64Field(existing, "cacheReadTokens", a.CacheReadTokens)
			addFloat64Field(existing, "costUSD", a.CostUSD)
			addInt64Field(existing, "runsWithCost", a.RunsWithCost)
			addInt64Field(existing, "success", a.Success)
			addInt64Field(existing, "failed", a.Failed)
			addInt64Field(existing, "awaiting", a.Awaiting)
			addInt64Field(existing, "other", a.Other)
			addInt64Field(existing, "wallDurationMs", a.WallDuration.Milliseconds())
			continue
		}
		merged[key] = item
		order = append(order, key)
	}
	for _, key := range order {
		out = append(out, merged[key])
	}
	return out
}

func addInt64Field(row map[string]any, key string, delta int64) {
	current, _ := row[key].(int64)
	row[key] = current + delta
}

func addFloat64Field(row map[string]any, key string, delta float64) {
	current, _ := row[key].(float64)
	row[key] = current + delta
}

func (s *Server) handleTelemetryRuns(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	since := strings.TrimSpace(q.Get("since"))
	until := strings.TrimSpace(q.Get("until"))
	allTime := q.Get("allTime") == "1" || strings.EqualFold(q.Get("allTime"), "true")
	project := strings.TrimSpace(q.Get("project"))
	if project != "" && !s.checkProjectAccess(w, r, project) {
		return
	}
	limit := 200
	if v := strings.TrimSpace(q.Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 2000 {
			limit = n
		}
	}

	from, to, err := telemetry.ParseWindow(since, until, allTime, time.Now(), time.Local)
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	db, err := telemetry.OpenReadOnly(s.root)
	if err != nil {
		if err == telemetry.ErrNoDatabase {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"window":    windowJSON(from, to, allTime),
				"available": false,
				"runs":      []any{},
			})
			return
		}
		s.serverError(w, err)
		return
	}
	defer db.Close()

	rows, err := telemetry.ReadRuns(db, from, to, project)
	if err != nil {
		s.serverError(w, err)
		return
	}
	rows = s.filterTelemetryRunsForRequest(r, rows)
	rows = s.enrichTelemetryUsageFromLogs(rows)
	agentMeta := s.telemetryAgentMetadata()
	// Newest first
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}

	runOut := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		m := map[string]any{
			"project":    row.Project,
			"agent":      row.Agent,
			"kind":       row.Kind,
			"status":     row.Status,
			"startedAt":  row.StartedAt.UTC().Format(time.RFC3339Nano),
			"finishedAt": row.FinishedAt.UTC().Format(time.RFC3339Nano),
			"model":      row.Model,
			"command":    row.CommandSummary,
			"logPath":    row.LogPath,
		}
		if row.TaskID.Valid && row.TaskID.String != "" {
			m["taskId"] = row.TaskID.String
		}
		if row.TaskTitle.Valid && row.TaskTitle.String != "" {
			m["taskTitle"] = row.TaskTitle.String
		}
		if row.SessionID.Valid && row.SessionID.String != "" {
			m["sessionId"] = row.SessionID.String
		}
		if row.ErrorMsg.Valid && row.ErrorMsg.String != "" {
			m["errorMsg"] = row.ErrorMsg.String
		}
		if row.InputTokens.Valid {
			m["inputTokens"] = row.InputTokens.Int64
		}
		if row.OutputTokens.Valid {
			m["outputTokens"] = row.OutputTokens.Int64
		}
		if row.CacheReadTokens.Valid {
			m["cacheReadTokens"] = row.CacheReadTokens.Int64
		}
		if row.HasCost && row.TotalCostUSD.Valid {
			m["costUSD"] = row.TotalCostUSD.Float64
		}
		if row.APIModel != "" {
			m["apiModel"] = row.APIModel
		}
		if row.APIBaseURL != "" {
			m["apiBaseUrl"] = row.APIBaseURL
		}
		s.applyTelemetryAgentMetadata(m, agentMeta, row.Project, row.Agent, "", "")
		runOut = append(runOut, m)
	}
	runOut = append(runOut, s.runtimeRunRowsForTelemetry(project, limit, agentMeta)...)
	sortRunRowsNewestFirst(runOut)
	if len(runOut) > limit {
		runOut = runOut[:limit]
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"window":    windowJSON(from, to, allTime),
		"available": true,
		"runs":      runOut,
	})
}

func (s *Server) runtimeRunRowsForTelemetry(project string, limit int, meta telemetryAgentMetaMap) []map[string]any {
	if s == nil || s.controlDB == nil {
		return nil
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil || strings.TrimSpace(workspaceID) == "" {
		return nil
	}
	runs, err := s.controlDB.ListRuntimeRuns(controldb.RuntimeRunFilter{
		WorkspaceID: workspaceID,
		ProjectID:   project,
		Limit:       limit,
	})
	if err != nil || len(runs) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(runs))
	for _, run := range runs {
		result := map[string]any{}
		_ = json.Unmarshal([]byte(defaultRawJSON(run.ResultJSON)), &result)
		startedAt := firstNonEmpty(run.StartedAt, run.ClaimedAt, run.CreatedAt)
		finishedAt := firstNonEmpty(run.FinishedAt, run.UpdatedAt, startedAt)
		status := runtimeRunTelemetryStatus(run.Status, result)
		row := map[string]any{
			"project":      run.ProjectID,
			"agent":        run.AgentID,
			"kind":         "task",
			"status":       status,
			"startedAt":    startedAt,
			"finishedAt":   finishedAt,
			"taskId":       run.TaskID,
			"runtimeRunId": run.ID,
		}
		if sessionID, _ := result["sessionId"].(string); strings.TrimSpace(sessionID) != "" {
			row["sessionId"] = strings.TrimSpace(sessionID)
		}
		if logText, _ := result["logText"].(string); strings.TrimSpace(logText) != "" {
			row["logText"] = logText
		}
		if summary, _ := result["summary"].(string); strings.TrimSpace(summary) != "" {
			row["summary"] = strings.TrimSpace(summary)
		}
		if run.ErrorMessage != "" {
			row["errorMsg"] = run.ErrorMessage
		}
		s.applyTelemetryAgentMetadata(row, meta, run.ProjectID, run.AgentID, run.AgentWorkerID, run.ProjectMembershipID)
		out = append(out, row)
	}
	return out
}

type telemetryAgentMeta struct {
	Worker     controldb.AgentWorker
	Membership controldb.ProjectMembership
	Team       string
}

type telemetryAgentMetaMap struct {
	byProjectAgent map[string]telemetryAgentMeta
	byWorkerID     map[string]telemetryAgentMeta
	byMembershipID map[string]telemetryAgentMeta
}

func (s *Server) telemetryAgentMetadata() telemetryAgentMetaMap {
	out := telemetryAgentMetaMap{
		byProjectAgent: make(map[string]telemetryAgentMeta),
		byWorkerID:     make(map[string]telemetryAgentMeta),
		byMembershipID: make(map[string]telemetryAgentMeta),
	}
	if s == nil || s.controlDB == nil {
		return out
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil || strings.TrimSpace(workspaceID) == "" {
		return out
	}
	workers, err := s.controlDB.ListAgentWorkers(workspaceID)
	if err != nil {
		return out
	}
	workersByID := make(map[string]controldb.AgentWorker, len(workers))
	for _, worker := range workers {
		workersByID[worker.ID] = worker
	}
	memberships, err := s.controlDB.ListProjectMemberships(controldb.ProjectMembershipFilter{
		WorkspaceID: workspaceID,
		MemberType:  "agent_worker",
	})
	if err != nil {
		return out
	}
	roleTeams := s.projectMembershipRoleTeams()
	for _, membership := range memberships {
		worker, ok := workersByID[membership.MemberID]
		if !ok {
			continue
		}
		meta := telemetryAgentMeta{
			Worker:     worker,
			Membership: membership,
			Team:       s.projectMembershipTeam(membership, worker, roleTeams),
		}
		if membership.ID != "" {
			out.byMembershipID[membership.ID] = meta
		}
		if worker.ID != "" {
			if _, exists := out.byWorkerID[worker.ID]; !exists {
				out.byWorkerID[worker.ID] = meta
			}
		}
		for _, agentRef := range []string{membership.Title, worker.Name, worker.DisplayName} {
			agentRef = strings.TrimSpace(agentRef)
			if agentRef == "" {
				continue
			}
			out.byProjectAgent[telemetryProjectAgentKey(membership.ProjectID, agentRef)] = meta
		}
	}
	return out
}

func (s *Server) applyTelemetryAgentMetadata(row map[string]any, meta telemetryAgentMetaMap, project, agent, workerID, membershipID string) {
	resolved, ok := meta.byMembershipID[strings.TrimSpace(membershipID)]
	if !ok {
		resolved, ok = meta.byWorkerID[strings.TrimSpace(workerID)]
	}
	if !ok {
		resolved, ok = meta.byProjectAgent[telemetryProjectAgentKey(project, agent)]
	}
	if !ok {
		return
	}
	worker := resolved.Worker
	membership := resolved.Membership
	row["agentWorkerId"] = worker.ID
	row["agentWorkerName"] = worker.Name
	row["agentDisplayName"] = firstNonEmpty(worker.DisplayName, worker.Name)
	row["agentAvatar"] = agentWorkerAvatar(worker)
	row["projectMembershipId"] = membership.ID
	if membership.Role != "" {
		row["role"] = membership.Role
	}
	if membership.Title != "" {
		row["projectTitle"] = membership.Title
	}
	if resolved.Team != "" {
		row["team"] = resolved.Team
	}
}

func telemetryProjectAgentKey(project, agent string) string {
	return strings.ToLower(strings.TrimSpace(project)) + "/" + strings.ToLower(strings.TrimSpace(agent))
}

func runtimeRunTelemetryStatus(status string, result map[string]any) string {
	switch strings.TrimSpace(status) {
	case "queued":
		return "pending"
	case "running":
		return "in_progress"
	case "succeeded":
		if s, _ := result["status"].(string); strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
		return "done_success"
	case "failed", "cancelled":
		return "done_failed"
	default:
		if strings.TrimSpace(status) != "" {
			return status
		}
		return "pending"
	}
}

func sortRunRowsNewestFirst(rows []map[string]any) {
	sort.SliceStable(rows, func(i, j int) bool {
		return rowTime(rows[i], "startedAt").After(rowTime(rows[j], "startedAt"))
	})
}

func rowTime(row map[string]any, key string) time.Time {
	value, _ := row[key].(string)
	t, _ := time.Parse(time.RFC3339Nano, value)
	return t
}

func (s *Server) handleTelemetryLog(w http.ResponseWriter, r *http.Request) {
	logPath := strings.TrimSpace(r.URL.Query().Get("path"))
	if logPath == "" {
		s.jsonError(w, http.StatusBadRequest, "path is required")
		return
	}
	readPath, ok := s.resolveReadableTelemetryLogPath(r, logPath)
	if !ok {
		s.jsonError(w, http.StatusForbidden, "run log access required")
		return
	}
	data, err := os.ReadFile(readPath)
	if err != nil {
		if os.IsNotExist(err) {
			s.jsonError(w, http.StatusNotFound, "log file not found")
			return
		}
		s.serverError(w, err)
		return
	}
	const maxBytes = 512 * 1024
	content := string(data)
	truncated := false
	if len(data) > maxBytes {
		content = "=== earlier log content truncated ===\n" + string(data[len(data)-maxBytes:])
		truncated = true
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"path":      logPath,
		"content":   content,
		"truncated": truncated,
	})
}

func (s *Server) filterTelemetryRunsForRequest(r *http.Request, rows []telemetry.RunRow) []telemetry.RunRow {
	if len(rows) == 0 {
		return rows
	}
	out := rows[:0]
	for _, row := range rows {
		if s.canAccessAgent(r, row.Project, row.Agent) {
			out = append(out, row)
		}
	}
	return out
}

func (s *Server) resolveReadableTelemetryLogPath(r *http.Request, logPath string) (string, bool) {
	db, err := telemetry.OpenReadOnly(s.root)
	if err != nil {
		return "", false
	}
	defer db.Close()
	rows, err := telemetry.ReadRuns(db, nil, nil, "")
	if err != nil {
		return "", false
	}
	cleanWant := filepath.Clean(logPath)
	for _, row := range rows {
		if !s.canAccessAgent(r, row.Project, row.Agent) {
			continue
		}
		rowPath := telemetryLogReadPath(s.root, row.LogPath)
		if filepath.Clean(row.LogPath) == cleanWant || filepath.Clean(rowPath) == cleanWant {
			return rowPath, true
		}
	}
	return "", false
}

func telemetryLogReadPath(root, logPath string) string {
	if filepath.IsAbs(logPath) {
		return filepath.Clean(logPath)
	}
	return filepath.Clean(filepath.Join(root, logPath))
}

func (s *Server) enrichTelemetryUsageFromLogs(rows []telemetry.RunRow) []telemetry.RunRow {
	for i := range rows {
		row := &rows[i]
		if row.InputTokens.Valid || row.OutputTokens.Valid || row.CacheReadTokens.Valid {
			continue
		}
		if strings.TrimSpace(row.LogPath) == "" {
			continue
		}
		data, err := os.ReadFile(telemetryLogReadPath(s.root, row.LogPath))
		if err != nil {
			continue
		}
		usage := telemetry.ParseStreamJSONUsage(data)
		if !usage.SawResult {
			continue
		}
		row.InputTokens = sql.NullInt64{Int64: usage.InputTokens, Valid: true}
		row.OutputTokens = sql.NullInt64{Int64: usage.OutputTokens, Valid: true}
		row.CacheReadTokens = sql.NullInt64{Int64: usage.CacheReadTokens, Valid: true}
		if usage.HasCost {
			row.TotalCostUSD = sql.NullFloat64{Float64: usage.TotalCostUSD, Valid: true}
			row.HasCost = true
		}
	}
	return rows
}
