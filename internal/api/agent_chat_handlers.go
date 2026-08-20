package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/interaction"
	"github.com/multigent/multigent/internal/telemetry"
)

type agentChatBody struct {
	Message   string `json:"message"`
	SessionID string `json:"sessionId"`
	NoSession bool   `json:"noSession,omitempty"`
}

func localRuntimeAPIURLForRequest(r *http.Request) string {
	host := ""
	if r != nil {
		host = strings.TrimSpace(r.Host)
	}
	port := ""
	if host != "" {
		if _, p, err := net.SplitHostPort(host); err == nil {
			port = p
		} else if i := strings.LastIndex(host, ":"); i >= 0 && i+1 < len(host) {
			port = host[i+1:]
		}
	}
	if port == "" {
		if r != nil && r.TLS != nil {
			port = "443"
		} else {
			port = "80"
		}
	}
	return "http://" + net.JoinHostPort("127.0.0.1", port)
}

type agentChatHistoryRun struct {
	StartedAt string `json:"startedAt"`
	Status    string `json:"status"`
	LogPath   string `json:"logPath"`
}

type agentChatSessionInfo struct {
	SessionID string `json:"sessionId"`
	Title     string `json:"title"`
	Source    string `json:"source"`
	Model     string `json:"model,omitempty"`
	Status    string `json:"status,omitempty"`
	RunCount  int    `json:"runCount"`
	StartedAt string `json:"startedAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

func (s *Server) handleAgentChatSessions(w http.ResponseWriter, r *http.Request) {
	project, agent, ok := s.parseProjectAgent(w, r)
	if !ok {
		return
	}
	if !s.checkProjectAccess(w, r, project) {
		return
	}
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}

	sessions, err := s.listAgentChatSessions(workspaceID, project, agent)
	if err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"sessions": sessions})
}

func (s *Server) listAgentChatSessions(workspaceID, project, agent string) ([]agentChatSessionInfo, error) {
	byID := map[string]*agentChatSessionAcc{}
	if err := s.addTelemetryAgentChatSessions(byID, project, agent); err != nil {
		return nil, err
	}
	if err := s.addRuntimeNodeAgentChatSessions(byID, workspaceID, project, agent); err != nil {
		return nil, err
	}

	out := make([]agentChatSessionInfo, 0, len(byID))
	for _, item := range byID {
		title := strings.TrimSpace(item.info.Title)
		if title == "" {
			title = strings.TrimSpace(item.taskTitle)
		}
		if title == "" && item.logPath != "" {
			title = s.extractSessionTitleFromRunLog(item.logPath)
		}
		if title == "" {
			title = "Session " + shortSessionID(item.info.SessionID)
		}
		item.info.Title = title
		if !item.startedAt.IsZero() {
			item.info.StartedAt = item.startedAt.UTC().Format(time.RFC3339Nano)
		}
		if !item.updatedAt.IsZero() {
			item.info.UpdatedAt = item.updatedAt.UTC().Format(time.RFC3339Nano)
		}
		out = append(out, item.info)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt > out[j].UpdatedAt
	})
	const maxSessions = 50
	if len(out) > maxSessions {
		out = out[:maxSessions]
	}
	return out, nil
}

type agentChatSessionAcc struct {
	info      agentChatSessionInfo
	startedAt time.Time
	updatedAt time.Time
	logPath   string
	taskTitle string
	runIDs    map[string]bool
}

func (s *Server) addTelemetryAgentChatSessions(byID map[string]*agentChatSessionAcc, project, agent string) error {
	db, err := telemetry.OpenReadOnly(s.root)
	if err != nil {
		if err == telemetry.ErrNoDatabase {
			return nil
		}
		return err
	}
	defer db.Close()

	rows, err := telemetry.ReadRuns(db, nil, nil, project)
	if err != nil {
		return err
	}

	for _, row := range rows {
		if row.Agent != agent {
			continue
		}
		sessionID := ""
		if row.SessionID.Valid {
			sessionID = strings.TrimSpace(row.SessionID.String)
		}
		if sessionID == "" && row.LogPath != "" {
			if sid := s.extractSessionIDFromRunLog(row.LogPath); sid != "" {
				sessionID = sid
			}
		}
		if sessionID == "" {
			continue
		}
		item := byID[sessionID]
		if item == nil {
			item = &agentChatSessionAcc{
				info: agentChatSessionInfo{
					SessionID: sessionID,
					Source:    string(row.Kind),
					Model:     firstNonEmpty(row.APIModel, row.Model),
					Status:    row.Status,
				},
				startedAt: row.StartedAt,
				updatedAt: row.StartedAt,
				runIDs:    map[string]bool{},
			}
			byID[sessionID] = item
		}
		runKey := firstNonEmpty(row.LogPath, row.StartedAt.UTC().Format(time.RFC3339Nano))
		if !item.runIDs[runKey] {
			item.runIDs[runKey] = true
			item.info.RunCount++
		}
		if item.startedAt.IsZero() || row.StartedAt.Before(item.startedAt) {
			item.startedAt = row.StartedAt
		}
		if row.StartedAt.After(item.updatedAt) {
			item.updatedAt = row.StartedAt
			item.info.Source = string(row.Kind)
			item.info.Model = firstNonEmpty(row.APIModel, row.Model, item.info.Model)
			item.info.Status = row.Status
			item.logPath = row.LogPath
		}
		if item.logPath == "" && row.LogPath != "" {
			item.logPath = row.LogPath
		}
		if row.TaskTitle.Valid && strings.TrimSpace(row.TaskTitle.String) != "" {
			item.taskTitle = strings.TrimSpace(row.TaskTitle.String)
		}
	}
	return nil
}

func (s *Server) addRuntimeNodeAgentChatSessions(byID map[string]*agentChatSessionAcc, workspaceID, project, agent string) error {
	if s.controlDB == nil || strings.TrimSpace(workspaceID) == "" {
		return nil
	}

	sessions, err := s.controlDB.ListInteractionSessions(controldb.InteractionSessionFilter{
		WorkspaceID: workspaceID,
		ProjectID:   project,
		AgentID:     agent,
		Limit:       500,
	})
	if err != nil {
		return err
	}
	for _, session := range sessions {
		sessionID := strings.TrimSpace(session.RuntimeSessionID)
		if sessionID == "" {
			continue
		}
		item := ensureAgentChatSessionAcc(byID, sessionID)
		if item.info.Source == "" {
			item.info.Source = firstNonEmpty(session.SourceKind, "runtime-node")
		}
		if session.Status != "" {
			item.info.Status = session.Status
		}
		createdAt := parseDBTime(session.CreatedAt)
		updatedAt := latestDBTime(session.UpdatedAt, session.LastActivityAt, session.CompletedAt, session.CreatedAt)
		updateAgentChatSessionTimes(item, createdAt, updatedAt)
		if item.info.Title == "" {
			title, err := s.interactionSessionTitle(workspaceID, session.ID)
			if err != nil {
				return err
			}
			item.info.Title = title
		}
	}

	runs, err := s.controlDB.ListRuntimeRuns(controldb.RuntimeRunFilter{
		WorkspaceID: workspaceID,
		ProjectID:   project,
		AgentID:     agent,
		Limit:       500,
	})
	if err != nil {
		return err
	}
	for _, run := range runs {
		result := decodeRuntimeRunResult(run.ResultJSON)
		sessionID := strings.TrimSpace(result["sessionId"])
		logText := result["logText"]
		if sessionID == "" && logText != "" {
			sessionID = extractAgentChatSessionID(logText)
		}
		if sessionID == "" {
			continue
		}
		item := ensureAgentChatSessionAcc(byID, sessionID)
		if item.info.Source == "" {
			item.info.Source = "runtime-node"
		}
		if run.Status != "" {
			item.info.Status = run.Status
		}
		if item.logPath == "" {
			item.logPath = result["logPath"]
		}
		createdAt := latestDBTime(run.StartedAt, run.CreatedAt)
		updatedAt := latestDBTime(run.FinishedAt, run.UpdatedAt, run.StartedAt, run.CreatedAt)
		updateAgentChatSessionTimes(item, createdAt, updatedAt)
		if item.runIDs == nil {
			item.runIDs = map[string]bool{}
		}
		if !item.runIDs[run.ID] {
			item.runIDs[run.ID] = true
			item.info.RunCount++
		}
		if item.info.Title == "" && logText != "" {
			item.info.Title = summarizeSessionTitleFromLog(logText)
		}
	}
	return nil
}

func ensureAgentChatSessionAcc(byID map[string]*agentChatSessionAcc, sessionID string) *agentChatSessionAcc {
	item := byID[sessionID]
	if item == nil {
		item = &agentChatSessionAcc{
			info:   agentChatSessionInfo{SessionID: sessionID},
			runIDs: map[string]bool{},
		}
		byID[sessionID] = item
	}
	return item
}

func updateAgentChatSessionTimes(item *agentChatSessionAcc, startedAt, updatedAt time.Time) {
	if !startedAt.IsZero() && (item.startedAt.IsZero() || startedAt.Before(item.startedAt)) {
		item.startedAt = startedAt
	}
	if !updatedAt.IsZero() && updatedAt.After(item.updatedAt) {
		item.updatedAt = updatedAt
	}
}

func (s *Server) interactionSessionTitle(workspaceID, sessionID string) (string, error) {
	if s.controlDB == nil || sessionID == "" {
		return "", nil
	}
	events, err := s.controlDB.ListInteractionEvents(controldb.InteractionEventFilter{
		WorkspaceID: workspaceID,
		SessionID:   sessionID,
		Limit:       200,
	})
	if err != nil {
		return "", err
	}
	for _, event := range events {
		if strings.EqualFold(event.ActorType, "user") && strings.TrimSpace(event.Content) != "" {
			return truncateSessionTitle(event.Content), nil
		}
	}
	return "", nil
}

func latestDBTime(values ...string) time.Time {
	var latest time.Time
	for _, value := range values {
		parsed := parseDBTime(value)
		if !parsed.IsZero() && parsed.After(latest) {
			latest = parsed
		}
	}
	return latest
}

func parseDBTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func (s *Server) extractSessionIDFromRunLog(logPath string) string {
	data, err := s.readSmallRunLog(logPath)
	if err != nil {
		return ""
	}
	return extractAgentChatSessionID(string(data))
}

func (s *Server) extractSessionTitleFromRunLog(logPath string) string {
	data, err := s.readSmallRunLog(logPath)
	if err != nil {
		return ""
	}
	return summarizeSessionTitleFromLog(string(data))
}

func (s *Server) readSmallRunLog(logPath string) ([]byte, error) {
	absLogPath := logPath
	if !filepath.IsAbs(absLogPath) {
		absLogPath = filepath.Join(s.root, absLogPath)
	}
	f, err := os.Open(absLogPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	limited := io.LimitReader(f, 256*1024)
	return io.ReadAll(limited)
}

func readFileTail(path string, maxBytes int64) ([]byte, bool, error) {
	if maxBytes <= 0 {
		return nil, false, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return nil, false, err
	}
	size := stat.Size()
	if size <= maxBytes {
		data, err := io.ReadAll(f)
		return data, false, err
	}
	if _, err := f.Seek(size-maxBytes, io.SeekStart); err != nil {
		return nil, false, err
	}
	data, err := io.ReadAll(f)
	return data, true, err
}

func (s *Server) handleAgentChatHistory(w http.ResponseWriter, r *http.Request) {
	project, agent, ok := s.parseProjectAgent(w, r)
	if !ok {
		return
	}
	if !s.checkProjectAccess(w, r, project) {
		return
	}
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}

	sessionID := strings.TrimSpace(r.URL.Query().Get("sessionId"))
	resolvedSessionID := sessionID
	content := ""
	truncated := false
	runs := []agentChatHistoryRun{}
	if sessionID != "" {
		var err error
		content, runs, resolvedSessionID, truncated, err = s.readAgentSessionHistory(workspaceID, project, agent, sessionID)
		if err != nil {
			s.serverError(w, err)
			return
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"sessionId": resolvedSessionID,
		"content":   content,
		"runs":      runs,
		"truncated": truncated,
	})
}

type historySegment struct {
	startedAt time.Time
	status    string
	logPath   string
	data      []byte
}

const (
	agentChatHistoryMaxRuns        = 6
	agentChatHistoryMaxBytes       = 384 * 1024
	agentChatHistoryMaxBytesPerRun = 160 * 1024
)

func (s *Server) readAgentSessionHistory(workspaceID, project, agent, sessionID string) (string, []agentChatHistoryRun, string, bool, error) {
	segments := make([]historySegment, 0, agentChatHistoryMaxRuns)

	telemetrySegments, resolvedFromTelemetry, err := s.readTelemetryAgentSessionHistory(project, agent, sessionID, agentChatHistoryMaxRuns)
	if err != nil {
		return "", nil, sessionID, false, err
	}
	segments = append(segments, telemetrySegments...)
	if sessionID == "" && resolvedFromTelemetry != "" {
		sessionID = resolvedFromTelemetry
	}

	runtimeSegments, resolvedFromRuntime, err := s.readRuntimeNodeAgentSessionHistory(workspaceID, project, agent, sessionID, agentChatHistoryMaxRuns)
	if err != nil {
		return "", nil, sessionID, false, err
	}
	segments = append(segments, runtimeSegments...)
	if sessionID == "" && resolvedFromRuntime != "" {
		sessionID = resolvedFromRuntime
	}
	if len(segments) == 0 && sessionID != "" {
		nativeSegment, err := s.readNativeClaudeAgentSessionHistory(project, agent, sessionID)
		if err != nil {
			return "", nil, sessionID, false, err
		}
		if nativeSegment != nil {
			segments = append(segments, *nativeSegment)
		}
	}

	sort.SliceStable(segments, func(i, j int) bool {
		return segments[i].startedAt.Before(segments[j].startedAt)
	})
	if len(segments) > agentChatHistoryMaxRuns {
		segments = segments[len(segments)-agentChatHistoryMaxRuns:]
	}

	total := 0
	selected := make([]historySegment, 0, len(segments))
	truncated := false
	for i := len(segments) - 1; i >= 0; i-- {
		seg := segments[i]
		if total+len(seg.data) > agentChatHistoryMaxBytes {
			remaining := agentChatHistoryMaxBytes - total
			if remaining <= 0 {
				truncated = true
				break
			}
			seg.data = append([]byte("=== earlier log content truncated ===\n"), seg.data[len(seg.data)-remaining:]...)
			truncated = true
		}
		selected = append([]historySegment{seg}, selected...)
		total += len(seg.data)
		if truncated {
			break
		}
	}

	var sb strings.Builder
	outRuns := make([]agentChatHistoryRun, 0, len(selected))
	for _, seg := range selected {
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.Write(seg.data)
		outRuns = append(outRuns, agentChatHistoryRun{
			StartedAt: seg.startedAt.UTC().Format(time.RFC3339Nano),
			Status:    seg.status,
			LogPath:   seg.logPath,
		})
	}
	log.Printf("[chat-history] %s/%s: returning %d runs, resolvedSession=%q, totalBytes=%d, truncated=%v",
		project, agent, len(outRuns), sessionID, sb.Len(), truncated)
	return sb.String(), outRuns, sessionID, truncated, nil
}

func (s *Server) readNativeClaudeAgentSessionHistory(project, agent, sessionID string) (*historySegment, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, nil
	}
	claudeProjectsDir := filepath.Join(s.st.AgentDir(project, agent), ".multigent", "runtime-home", "claudecode", ".claude", "projects")
	matches, err := filepath.Glob(filepath.Join(claudeProjectsDir, "*", sessionID+".jsonl"))
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		matches, err = filepath.Glob(filepath.Join(claudeProjectsDir, "*", sessionID))
		if err != nil {
			return nil, err
		}
	}
	if len(matches) == 0 {
		return nil, nil
	}
	sort.Strings(matches)
	path := matches[len(matches)-1]
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil, nil
	}
	startedAt := time.Now().UTC()
	if stat, err := os.Stat(path); err == nil {
		startedAt = stat.ModTime().UTC()
	}
	return &historySegment{
		startedAt: startedAt,
		status:    "native",
		logPath:   path,
		data:      data,
	}, nil
}

func (s *Server) readTelemetryAgentSessionHistory(project, agent, sessionID string, maxRuns int) ([]historySegment, string, error) {
	db, err := telemetry.OpenReadOnly(s.root)
	if err != nil {
		if err == telemetry.ErrNoDatabase {
			return nil, sessionID, nil
		}
		return nil, sessionID, err
	}
	defer db.Close()

	rows, err := telemetry.ReadRuns(db, nil, nil, project)
	if err != nil {
		return nil, sessionID, err
	}

	filtered := make([]telemetry.RunRow, 0, maxRuns)
	for i := len(rows) - 1; i >= 0; i-- {
		row := rows[i]
		if row.Agent != agent || row.LogPath == "" {
			continue
		}
		if sessionID != "" && (!row.SessionID.Valid || row.SessionID.String != sessionID) {
			continue
		}
		filtered = append(filtered, row)
		if len(filtered) >= maxRuns {
			break
		}
	}
	log.Printf("[chat-history] %s/%s: telemetry query sessionID=%q -> %d candidate runs (total rows=%d)", project, agent, sessionID, len(filtered), len(rows))
	for i, j := 0, len(filtered)-1; i < j; i, j = i+1, j-1 {
		filtered[i], filtered[j] = filtered[j], filtered[i]
	}

	segments := make([]historySegment, 0, len(filtered))
	for _, row := range filtered {
		logPath := row.LogPath
		absLogPath := logPath
		if !filepath.IsAbs(absLogPath) {
			absLogPath = filepath.Join(s.root, absLogPath)
		}
		data, truncated, err := readFileTail(absLogPath, agentChatHistoryMaxBytesPerRun)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, sessionID, err
		}
		if truncated {
			data = append([]byte("=== earlier run log content truncated ===\n"), data...)
		}
		if sessionID == "" {
			if row.SessionID.Valid && row.SessionID.String != "" {
				sessionID = row.SessionID.String
			} else if sid := extractAgentChatSessionID(string(data)); sid != "" {
				sessionID = sid
			}
		}
		segments = append(segments, historySegment{
			startedAt: row.StartedAt,
			status:    row.Status,
			logPath:   logPath,
			data:      data,
		})
	}
	return segments, sessionID, nil
}

func (s *Server) readRuntimeNodeAgentSessionHistory(workspaceID, project, agent, sessionID string, maxRuns int) ([]historySegment, string, error) {
	if s.controlDB == nil || strings.TrimSpace(workspaceID) == "" {
		return nil, sessionID, nil
	}
	runs, err := s.controlDB.ListRuntimeRuns(controldb.RuntimeRunFilter{
		WorkspaceID: workspaceID,
		ProjectID:   project,
		AgentID:     agent,
		Limit:       500,
	})
	if err != nil {
		return nil, sessionID, err
	}

	segments := make([]historySegment, 0, maxRuns)
	for _, run := range runs {
		result := decodeRuntimeRunResult(run.ResultJSON)
		runSessionID := strings.TrimSpace(result["sessionId"])
		logText := result["logText"]
		if runSessionID == "" && logText != "" {
			runSessionID = extractAgentChatSessionID(logText)
		}
		if runSessionID == "" {
			continue
		}
		if sessionID != "" && runSessionID != sessionID {
			continue
		}
		if sessionID == "" {
			sessionID = runSessionID
		}
		if strings.TrimSpace(logText) == "" {
			logText = s.runtimeRunHistoryFallback(workspaceID, run.ID)
		}
		if strings.TrimSpace(logText) == "" {
			continue
		}
		logData := []byte(logText)
		if len(logData) > agentChatHistoryMaxBytesPerRun {
			logData = append([]byte("=== earlier runtime log content truncated ===\n"), logData[len(logData)-agentChatHistoryMaxBytesPerRun:]...)
		}
		startedAt := latestDBTime(run.StartedAt, run.CreatedAt)
		if startedAt.IsZero() {
			startedAt = time.Now().UTC()
		}
		segments = append(segments, historySegment{
			startedAt: startedAt,
			status:    run.Status,
			logPath:   result["logPath"],
			data:      logData,
		})
		if len(segments) >= maxRuns {
			break
		}
	}
	for i, j := 0, len(segments)-1; i < j; i, j = i+1, j-1 {
		segments[i], segments[j] = segments[j], segments[i]
	}
	return segments, sessionID, nil
}

func (s *Server) runtimeRunHistoryFallback(workspaceID, runID string) string {
	if s.controlDB == nil || runID == "" {
		return ""
	}
	events, err := s.controlDB.ListRuntimeEvents(workspaceID, runID, 200)
	if err != nil || len(events) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, event := range events {
		if strings.TrimSpace(event.PayloadJSON) == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(event.PayloadJSON)
	}
	return sb.String()
}

func (s *Server) handleAgentChat(w http.ResponseWriter, r *http.Request) {
	project, agent, ok := s.parseProjectAgent(w, r)
	if !ok {
		return
	}
	if !s.checkProjectAccess(w, r, project) {
		return
	}
	if !s.canOperateAgent(r, project, agent) {
		s.jsonError(w, http.StatusForbidden, "agent operator access required")
		return
	}
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}

	var body agentChatBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	msg := strings.TrimSpace(body.Message)
	if msg == "" {
		s.jsonError(w, http.StatusBadRequest, "message is required")
		return
	}

	meta, err := s.st.AgentMeta(project, agent)
	if err != nil {
		if isNotFoundErr(err) {
			s.jsonErrorCode(w, http.StatusNotFound, ErrCodeAgentNotFound, "agent not found")
			return
		}
		s.serverError(w, err)
		return
	}
	if readiness := s.runtimeReadinessForExecution(workspaceID, meta); readiness.Blocking {
		s.jsonErrorCode(w, http.StatusConflict, ErrCodeRuntimeNotReady, runtimeReadinessErrorMessage(readiness))
		return
	}

	if s.usesAssignedRuntimeNode(workspaceID, meta) {
		s.handleAgentChatViaRuntimeNode(w, r, workspaceID, project, agent, meta, body, msg)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.jsonError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	lease, ok := s.acquireAgentInteraction(w, s.interactionAgentRef(workspaceID, project, agent), interaction.Source{
		Kind:    "web_chat",
		ActorID: requestUsername(r),
		Channel: "web",
	}, "interactive")
	if !ok {
		return
	}
	defer lease.Release()
	_ = s.createInteractionEvent(lease.session, "user", requestUsername(r), "web", "message", msg, map[string]any{
		"source": "web_chat",
	})

	key := project + "/" + agent
	s.execMu.Lock()
	if _, ok := s.execProcs[key]; ok {
		s.execMu.Unlock()
		s.jsonError(w, http.StatusConflict, "agent is already running")
		return
	}
	s.execProcs[key] = nil // placeholder; will be replaced after cmd.Start
	s.execMu.Unlock()

	args := []string{"--dir", s.root, "exec", "--project", project, "--agent", agent, "--prompt", msg, "--no-save-session"}
	sessionID := strings.TrimSpace(body.SessionID)
	if sessionID != "" && !body.NoSession {
		args = append(args, "--session", sessionID)
	}
	if body.NoSession || sessionID == "" {
		args = append(args, "--no-session")
	}

	// Do not bind the child process to the HTTP request context. The browser
	// aborts fetches when navigating away; killing the agent at that point would
	// prevent run logs and telemetry from being recorded.
	cmd := exec.Command(s.sched.binPath, args...)
	cmd.Dir = s.root
	runID := "exec-" + time.Now().UTC().Format("20060102-150405")
	runtimeToken := s.issueAgentRuntimeToken(runtimeAgentTokenPayload{
		WorkspaceID:  workspaceID,
		Project:      project,
		Agent:        agent,
		RunID:        runID,
		Capabilities: defaultRuntimeCapabilities(),
	}, 6*time.Hour)
	cmd.Env = append(os.Environ(),
		"MULTIGENT_API_URL="+localRuntimeAPIURLForRequest(r),
		"MULTIGENT_AGENT_TOKEN="+runtimeToken,
		"MULTIGENT_RUN_ID="+runID,
		"MULTIGENT_WORKSPACE_ID="+workspaceID,
	)
	setProcGroup(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.execMu.Lock()
		delete(s.execProcs, key)
		s.execMu.Unlock()
		s.serverError(w, err)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		s.execMu.Lock()
		delete(s.execProcs, key)
		s.execMu.Unlock()
		s.serverError(w, err)
		return
	}
	if err := cmd.Start(); err != nil {
		s.execMu.Lock()
		delete(s.execProcs, key)
		s.execMu.Unlock()
		s.serverError(w, err)
		return
	}

	// Register the running process so it can be stopped via the /chat DELETE endpoint.
	s.execMu.Lock()
	s.execProcs[key] = &execProcess{cmd: cmd, started: time.Now()}
	s.execMu.Unlock()
	_ = s.createInteractionEvent(lease.session, "system", "", "web", "run_started", "", map[string]any{
		"sessionId": sessionID,
	})

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	lines := make(chan string, 64)
	var wg sync.WaitGroup
	scan := func(src io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(src)
		scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimRight(scanner.Text(), "\r")
			if line != "" {
				lines <- line
			}
		}
	}
	wg.Add(2)
	go scan(stdout)
	go scan(stderr)
	go func() {
		wg.Wait()
		close(lines)
	}()

	detectedSessionID := sessionID
	agentModel := entity.AgentModel("")
	if meta, err := s.st.AgentMeta(project, agent); err == nil && meta != nil {
		agentModel = meta.Model
	}
	clientGone := false
	lineCount := 0
	lastStreamError := ""
	recentLines := []string{}
	for line := range lines {
		lineCount++
		recentLines = appendRecentAgentChatLine(recentLines, line, 14)
		if sid := extractAgentChatSessionID(line); sid != "" {
			if detectedSessionID == "" {
				log.Printf("[chat] %s/%s: detected session_id=%s", project, agent, sid)
			}
			detectedSessionID = sid
			lease.SetRuntimeSessionID(sid)
		}
		if msg := extractAgentChatError(line); msg != "" {
			lastStreamError = msg
		}
		if clientGone {
			continue
		}
		payload := chatSSEPayload(line, agentModel)
		if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
			log.Printf("[chat] %s/%s: client gone after %d lines (write err: %v)", project, agent, lineCount, err)
			clientGone = true
			continue
		}
		flusher.Flush()
	}
	log.Printf("[chat] %s/%s: streamed %d lines, session=%q clientGone=%v", project, agent, lineCount, detectedSessionID, clientGone)

	waitErr := cmd.Wait()

	// Unregister the process now that it has finished.
	s.execMu.Lock()
	delete(s.execProcs, key)
	s.execMu.Unlock()

	if waitErr != nil {
		errMsg := summarizeAgentChatExit(waitErr, lastStreamError, recentLines)
		lease.Fail(errMsg)
		detectedSessionID = ""
		_ = s.createInteractionEvent(lease.session, "system", "", "web", "run_failed", "", map[string]any{
			"error": errMsg,
		})
	} else {
		_ = s.createInteractionEvent(lease.session, "agent", project+"/"+agent, "web", "run_completed", "", map[string]any{
			"runtimeSessionId": detectedSessionID,
		})
	}

	if waitErr != nil && !clientGone {
		errMsg := summarizeAgentChatExit(waitErr, lastStreamError, recentLines)
		evt, _ := json.Marshal(map[string]any{
			"type":  "chat_error",
			"error": errMsg,
		})
		fmt.Fprintf(w, "data: %s\n\n", evt)
		flusher.Flush()
	}

	if clientGone {
		return
	}

	done, _ := json.Marshal(map[string]any{
		"type":       "chat_done",
		"session_id": detectedSessionID,
	})
	fmt.Fprintf(w, "data: %s\n\n", done)
	flusher.Flush()
}

func (s *Server) handleAgentChatViaRuntimeNode(w http.ResponseWriter, r *http.Request, workspaceID, project, agent string, meta *entity.AgentMeta, body agentChatBody, msg string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.jsonError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}
	lease, ok := s.acquireAgentInteraction(w, s.interactionAgentRef(workspaceID, project, agent), interaction.Source{
		Kind:    "web_chat",
		ActorID: requestUsername(r),
		Channel: "web",
	}, "interactive")
	if !ok {
		return
	}
	defer lease.Release()
	_ = s.createInteractionEvent(lease.session, "user", requestUsername(r), "web", "message", msg, map[string]any{
		"source":  "web_chat",
		"runtime": "node",
	})

	sessionID := strings.TrimSpace(body.SessionID)
	if body.NoSession {
		sessionID = ""
	}
	run, err := s.enqueueRuntimeExecRun(workspaceID, project, agent, msg, sessionID, externalServerURL(r), requestUsername(r))
	if err != nil {
		s.serverError(w, err)
		return
	}
	_ = s.createInteractionEvent(lease.session, "system", "", "web", "run_started", "", map[string]any{
		"sessionId": sessionID,
		"runId":     run.ID,
		"runtime":   "node",
	})

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	sendChatSSE := func(payload map[string]any) bool {
		raw, _ := json.Marshal(payload)
		if _, err := fmt.Fprintf(w, "data: %s\n\n", raw); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	if !sendChatSSE(map[string]any{
		"type":         "chat_event",
		"payload":      "Runtime node queued run " + run.ID,
		"payloadType":  "log",
		"raw":          false,
		"runtimeRunId": run.ID,
	}) {
		return
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	timeout := time.NewTimer(30 * time.Minute)
	defer timeout.Stop()
	var lastStatus string
	agentModel := entity.AgentModel("")
	if meta != nil {
		agentModel = meta.Model
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case <-timeout.C:
			lease.Fail("runtime run timed out")
			_ = sendChatSSE(map[string]any{"type": "chat_error", "error": "runtime run timed out"})
			return
		case <-ticker.C:
			current, found, err := s.controlDB.RuntimeRunByID(workspaceID, run.ID)
			if err != nil {
				lease.Fail(err.Error())
				_ = sendChatSSE(map[string]any{"type": "chat_error", "error": err.Error()})
				return
			}
			if !found {
				lease.Fail("runtime run disappeared")
				_ = sendChatSSE(map[string]any{"type": "chat_error", "error": "runtime run disappeared"})
				return
			}
			if current.Status != lastStatus {
				lastStatus = current.Status
				if !sendChatSSE(map[string]any{
					"type":         "chat_event",
					"payload":      "Runtime node status: " + current.Status,
					"payloadType":  "log",
					"raw":          false,
					"runtimeRunId": current.ID,
				}) {
					return
				}
			}
			switch current.Status {
			case "succeeded", "failed":
				result := decodeRuntimeRunResult(current.ResultJSON)
				detectedSessionID := strings.TrimSpace(result["sessionId"])
				if detectedSessionID != "" {
					lease.SetRuntimeSessionID(detectedSessionID)
				}
				logText := strings.TrimSpace(result["logText"])
				if logText != "" {
					for _, line := range strings.Split(logText, "\n") {
						line = strings.TrimRight(line, "\r")
						if line == "" {
							continue
						}
						payload := chatSSEPayload(line, agentModel)
						if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
							return
						}
						flusher.Flush()
					}
				}
				if current.Status == "failed" {
					errMsg := strings.TrimSpace(current.ErrorMessage)
					if errMsg == "" {
						errMsg = strings.TrimSpace(result["error"])
					}
					if errMsg == "" {
						errMsg = "runtime run failed"
					}
					lease.Fail(errMsg)
					_ = s.createInteractionEvent(lease.session, "system", "", "web", "run_failed", "", map[string]any{"error": errMsg, "runtimeRunId": current.ID})
					_ = sendChatSSE(map[string]any{"type": "chat_error", "error": errMsg})
					return
				}
				_ = s.createInteractionEvent(lease.session, "agent", project+"/"+agent, "web", "run_completed", "", map[string]any{
					"runtimeSessionId": detectedSessionID,
					"runtimeRunId":     current.ID,
				})
				_ = sendChatSSE(map[string]any{"type": "chat_done", "session_id": detectedSessionID, "runtimeRunId": current.ID})
				return
			}
		}
	}
}

// handleAgentChatStop kills a running agent exec process for a project/agent.
func (s *Server) handleAgentChatStop(w http.ResponseWriter, r *http.Request) {
	project, agent, ok := s.parseProjectAgent(w, r)
	if !ok {
		return
	}
	if !s.canOperateAgent(r, project, agent) {
		s.jsonError(w, http.StatusForbidden, "agent operator access required")
		return
	}

	key := project + "/" + agent
	s.execMu.Lock()
	proc, ok := s.execProcs[key]
	if ok {
		delete(s.execProcs, key)
	}
	s.execMu.Unlock()

	if proc == nil || proc.cmd.Process == nil {
		// No process running, treat as success (idempotent).
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "msg": "no process running"})
		return
	}

	pid := proc.cmd.Process.Pid
	killProcessGroup(pid)

	// Give it a moment then force kill if still alive.
	time.Sleep(500 * time.Millisecond)
	if proc.cmd.Process != nil {
		_ = proc.cmd.Process.Kill()
	}

	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "pid": pid})
}

func chatSSEPayload(line string, model entity.AgentModel) string {
	trimmed := strings.TrimSpace(line)
	payloadType := "text"
	normalized := line
	if model == entity.ModelCodex || model == entity.ModelQoder {
		payloadType = "cli"
	} else if strings.HasPrefix(trimmed, "{") {
		payloadType = "cli"
	} else if strings.HasPrefix(trimmed, "===") ||
		strings.HasPrefix(trimmed, "Command:") || strings.HasPrefix(trimmed, "Started:") {
		payloadType = "log"
	} else {
		payloadType = "log"
		normalized = "=== " + line + " ==="
	}
	out := map[string]any{
		"type":        "chat_event",
		"payloadType": payloadType,
		"payload":     normalized,
	}
	if sid := extractAgentChatSessionID(line); sid != "" {
		out["session_id"] = sid
	}
	raw, err := json.Marshal(out)
	if err != nil {
		fallback, _ := json.Marshal(map[string]any{
			"type":        "chat_event",
			"payloadType": "log",
			"payload":     "=== failed to encode chat event ===",
		})
		return string(fallback)
	}
	return string(raw)
}

func extractAgentChatError(line string) string {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "{") {
		lower := strings.ToLower(trimmed)
		switch {
		case strings.HasPrefix(lower, "docker: error response from daemon:"):
			return trimmed
		case strings.HasPrefix(lower, "docker: error during connect:"):
			return trimmed
		case strings.Contains(lower, "docker_engine") || strings.Contains(lower, "docker client must be run with elevated privileges"):
			return trimmed
		case strings.HasPrefix(lower, "unable to find image "):
			return trimmed
		case strings.Contains(lower, "unauthorized") && (strings.Contains(lower, "docker") || strings.Contains(lower, "registry")):
			return trimmed
		case strings.Contains(lower, "cannot reach docker daemon") || strings.Contains(lower, "is docker running?"):
			return trimmed
		case strings.Contains(lower, "authentication required") || strings.Contains(lower, "not logged in"):
			return trimmed
		}
		return ""
	}
	var ev struct {
		Type              string          `json:"type"`
		Subtype           string          `json:"subtype"`
		Message           json.RawMessage `json:"message"`
		Result            string          `json:"result"`
		IsError           bool            `json:"is_error"`
		IsAPIErrorMessage bool            `json:"is_api_error_message"`
		Errors            []string        `json:"errors"`
		Error             struct {
			Message string `json:"message"`
		} `json:"error"`
		Item struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"item"`
	}
	if err := json.Unmarshal([]byte(trimmed), &ev); err != nil {
		return ""
	}
	switch ev.Type {
	case "error":
		return strings.TrimSpace(agentChatRawMessageText(ev.Message))
	case "assistant":
		if ev.IsAPIErrorMessage {
			if text := agentChatRawMessageText(ev.Message); text != "" {
				return text
			}
		}
	case "result":
		if ev.IsError && strings.TrimSpace(ev.Result) != "" {
			return strings.TrimSpace(ev.Result)
		}
		if ev.Subtype == "error_during_execution" && len(ev.Errors) > 0 {
			return strings.TrimSpace(strings.Join(ev.Errors, "; "))
		}
	case "turn.failed":
		return strings.TrimSpace(ev.Error.Message)
	case "item.completed":
		if ev.Item.Type == "error" {
			return strings.TrimSpace(ev.Item.Message)
		}
	}
	return ""
}

func agentChatRawMessageText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text)
	}
	var obj struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ""
	}
	for _, block := range obj.Content {
		if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
			return strings.TrimSpace(block.Text)
		}
	}
	return ""
}

func extractAgentChatReply(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return ""
	}
	var assistantParts []string
	result := ""
	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 0, 256*1024), 2*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var ev struct {
			Type    string `json:"type"`
			IsError bool   `json:"is_error"`
			Result  string `json:"result"`
			Item    struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"item"`
			ContentBlock struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content_block"`
			Message struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		if ev.Type == "result" && !ev.IsError && strings.TrimSpace(ev.Result) != "" {
			result = strings.TrimSpace(ev.Result)
			continue
		}
		if ev.Type == "assistant" {
			for _, block := range ev.Message.Content {
				if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
					assistantParts = append(assistantParts, strings.TrimSpace(block.Text))
				}
			}
			for _, block := range ev.Content {
				if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
					assistantParts = append(assistantParts, strings.TrimSpace(block.Text))
				}
			}
		}
		if ev.Type == "item.completed" && ev.Item.Type == "agent_message" && strings.TrimSpace(ev.Item.Text) != "" {
			assistantParts = append(assistantParts, strings.TrimSpace(ev.Item.Text))
		}
		if ev.Type == "content" && ev.ContentBlock.Type == "text" && strings.TrimSpace(ev.ContentBlock.Text) != "" {
			assistantParts = append(assistantParts, strings.TrimSpace(ev.ContentBlock.Text))
		}
	}
	if result != "" {
		return result
	}
	if len(assistantParts) > 0 {
		return collapseDuplicateParagraphs(strings.Join(assistantParts, "\n\n"))
	}
	return ""
}

func collapseDuplicateParagraphs(text string) string {
	parts := strings.Split(strings.TrimSpace(text), "\n\n")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		if len(out) > 0 && out[len(out)-1] == trimmed {
			continue
		}
		out = append(out, trimmed)
	}
	return strings.Join(out, "\n\n")
}

func extractAgentChatSessionID(line string) string {
	if strings.Contains(line, "\n") {
		scanner := bufio.NewScanner(strings.NewReader(line))
		for scanner.Scan() {
			if sid := extractAgentChatSessionID(scanner.Text()); sid != "" {
				return sid
			}
		}
		return ""
	}
	var raw map[string]any
	if (strings.Contains(line, `"session_id"`) || strings.Contains(line, `"thread_id"`)) && json.Unmarshal([]byte(line), &raw) == nil {
		if sid, ok := raw["session_id"].(string); ok && sid != "" {
			return sid
		}
		if sid, ok := raw["thread_id"].(string); ok && sid != "" {
			return sid
		}
	}
	trimmed := strings.TrimSpace(line)
	lower := strings.ToLower(trimmed)
	for _, prefix := range []string{"session id:", "session:", "session :"} {
		if after, ok := strings.CutPrefix(lower, prefix); ok {
			start := len(trimmed) - len(after)
			return strings.TrimSpace(trimmed[start:])
		}
	}
	return ""
}

func summarizeSessionTitleFromLog(content string) string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if title := extractUserTitleFromJSONLine(line); title != "" {
			return truncateSessionTitle(title)
		}
	}
	return ""
}

func extractUserTitleFromJSONLine(line string) string {
	if !strings.HasPrefix(line, "{") {
		return ""
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return ""
	}
	if typ, _ := raw["type"].(string); typ == "human" {
		return stringField(raw, "content")
	}
	if typ, _ := raw["type"].(string); typ == "user" {
		if text := messageText(raw["message"]); text != "" {
			return text
		}
		if text := contentText(raw["content"]); text != "" {
			return text
		}
	}
	if role, _ := raw["role"].(string); role == "user" {
		if text := contentText(raw["content"]); text != "" {
			return text
		}
	}
	msg, ok := raw["message"].(map[string]any)
	if !ok {
		return ""
	}
	if role, _ := msg["role"].(string); role == "user" {
		return messageText(msg)
	}
	return ""
}

func messageText(v any) string {
	msg, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	if text := stringField(msg, "content"); text != "" {
		return text
	}
	return contentText(msg["content"])
}

func contentText(v any) string {
	switch c := v.(type) {
	case string:
		return strings.TrimSpace(c)
	case []any:
		parts := make([]string, 0, len(c))
		for _, item := range c {
			switch x := item.(type) {
			case string:
				if s := strings.TrimSpace(x); s != "" {
					parts = append(parts, s)
				}
			case map[string]any:
				if typ, _ := x["type"].(string); typ == "text" || typ == "" {
					if text := stringField(x, "text"); text != "" {
						parts = append(parts, text)
					}
				}
			}
		}
		return strings.TrimSpace(strings.Join(parts, " "))
	default:
		return ""
	}
}

func appendRecentAgentChatLine(lines []string, line string, limit int) []string {
	line = strings.TrimSpace(line)
	if line == "" {
		return lines
	}
	if len(line) > 1200 {
		line = line[:1200] + "..."
	}
	lines = append(lines, line)
	if limit > 0 && len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return lines
}

func summarizeAgentChatExit(waitErr error, lastStreamError string, recentLines []string) string {
	base := "agent run failed"
	if waitErr != nil {
		base = strings.TrimSpace(waitErr.Error())
	}
	if strings.TrimSpace(lastStreamError) != "" {
		return strings.TrimSpace(lastStreamError) + " (" + base + ")"
	}
	tail := usefulAgentChatTail(recentLines)
	if tail == "" {
		return base
	}
	return base + "\n\nLast output:\n" + tail
}

func usefulAgentChatTail(lines []string) string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "data: ") {
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "data: "))
		}
		out = append(out, trimmed)
	}
	if len(out) > 8 {
		out = out[len(out)-8:]
	}
	return strings.Join(out, "\n")
}

func stringField(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return strings.TrimSpace(v)
}

func truncateSessionTitle(title string) string {
	title = strings.Join(strings.Fields(title), " ")
	const maxRunes = 72
	runes := []rune(title)
	if len(runes) <= maxRunes {
		return title
	}
	return string(runes[:maxRunes]) + "..."
}

func shortSessionID(sessionID string) string {
	runes := []rune(strings.TrimSpace(sessionID))
	if len(runes) <= 12 {
		return string(runes)
	}
	return string(runes[:12]) + "..."
}
