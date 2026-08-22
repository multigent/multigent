package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/runtimeexec"
)

type runtimeSessionForkRequest struct {
	Title              string         `json:"title"`
	Purpose            string         `json:"purpose"`
	Project            string         `json:"project"`
	TaskID             string         `json:"taskId"`
	WorkflowInstanceID string         `json:"workflowInstanceId"`
	ParentSessionID    string         `json:"parentSessionId"`
	InitialPrompt      string         `json:"initialPrompt"`
	RuntimeProvider    string         `json:"runtimeProvider"`
	PermissionPolicy   string         `json:"permissionPolicy"`
	Capabilities       map[string]any `json:"capabilities"`
}

type runtimeSessionPatchRequest struct {
	Status           string         `json:"status"`
	ResultSummary    string         `json:"resultSummary"`
	ResultRefs       map[string]any `json:"resultRefs"`
	RuntimeSessionID string         `json:"runtimeSessionId"`
	LastRunID        string         `json:"lastRunId"`
	Prompt           string         `json:"prompt"`
	Run              bool           `json:"run"`
}

func (s *Server) handleRuntimeSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleRuntimeSessionList(w, r)
	case http.MethodPost:
		s.handleRuntimeSessionFork(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleRuntimeSession(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleRuntimeSessionGet(w, r)
	case http.MethodPatch:
		s.handleRuntimeSessionPatch(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAgentSessions(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	workerID := strings.TrimSpace(r.URL.Query().Get("workerId"))
	if workerID == "" {
		workerID = strings.TrimSpace(r.URL.Query().Get("agentWorkerId"))
	}
	if workerID == "" {
		workerID = s.agentWorkerIDForSessionQuery(workspaceID, strings.TrimSpace(r.URL.Query().Get("agent")))
	}
	status, statuses := runtimeSessionStatusFilter(strings.TrimSpace(r.URL.Query().Get("status")))
	filter := controldb.AgentSessionFilter{
		WorkspaceID:   workspaceID,
		AgentWorkerID: workerID,
		SessionKind:   firstNonEmpty(strings.TrimSpace(r.URL.Query().Get("kind")), "fork"),
		ProjectID:     strings.TrimSpace(r.URL.Query().Get("project")),
		TaskID:        strings.TrimSpace(r.URL.Query().Get("taskId")),
		Status:        status,
		Statuses:      statuses,
		Limit:         limit,
	}
	sessions, err := s.controlDB.ListAgentSessions(filter)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !s.canAdminWorkspace(r, workspaceID) {
		sessions = s.filterReadableAgentSessions(r, workspaceID, sessions)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"sessions": runtimeSessionResponses(sessions)})
}

func (s *Server) handleAgentSession(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "session id is required")
		return
	}
	session, found, err := s.controlDB.AgentSessionByID(workspaceID, id)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeNotFound, "session not found")
		return
	}
	if !s.canAdminWorkspace(r, workspaceID) && !s.currentUserCanOperateAgentWorker(r, workspaceID, session.AgentWorkerID) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeAgentAccessRequired, "agent session access required")
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"session": runtimeSessionResponse(session)})
}

func (s *Server) handleRuntimeSessionList(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.runtimeRequireCapability(w, r, "session.use")
	if !ok {
		return
	}
	workerID := s.runtimePrincipalAgentWorkerID(principal)
	if strings.TrimSpace(workerID) == "" {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeRuntimeCapabilityRequired, "runtime agent is not linked to an agent worker")
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	status, statuses := runtimeSessionStatusFilter(strings.TrimSpace(r.URL.Query().Get("status")))
	sessions, err := s.controlDB.ListAgentSessions(controldb.AgentSessionFilter{
		WorkspaceID:   principal.WorkspaceID,
		AgentWorkerID: workerID,
		SessionKind:   firstNonEmpty(strings.TrimSpace(r.URL.Query().Get("kind")), "fork"),
		ProjectID:     strings.TrimSpace(r.URL.Query().Get("project")),
		TaskID:        strings.TrimSpace(r.URL.Query().Get("taskId")),
		Status:        status,
		Statuses:      statuses,
		Limit:         limit,
	})
	if err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"sessions": runtimeSessionResponses(sessions)})
}

func runtimeSessionStatusFilter(status string) (string, []string) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active", "open":
		return "", []string{"pending", "running", "waiting", "blocked"}
	case "all":
		return "", nil
	default:
		return strings.TrimSpace(status), nil
	}
}

func (s *Server) agentWorkerIDForSessionQuery(workspaceID, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" || s == nil || s.controlDB == nil {
		return ""
	}
	if worker, ok, err := s.controlDB.AgentWorkerByID(workspaceID, ref); err == nil && ok {
		return worker.ID
	}
	if worker, ok, err := s.controlDB.AgentWorkerByName(workspaceID, ref); err == nil && ok {
		return worker.ID
	}
	return ""
}

func (s *Server) filterReadableAgentSessions(r *http.Request, workspaceID string, sessions []controldb.AgentSession) []controldb.AgentSession {
	out := make([]controldb.AgentSession, 0, len(sessions))
	allowed := map[string]bool{}
	for _, session := range sessions {
		workerID := strings.TrimSpace(session.AgentWorkerID)
		if workerID == "" {
			continue
		}
		can, ok := allowed[workerID]
		if !ok {
			can = s.currentUserCanOperateAgentWorker(r, workspaceID, workerID)
			allowed[workerID] = can
		}
		if can {
			out = append(out, session)
		}
	}
	return out
}

func (s *Server) handleRuntimeSessionFork(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.runtimeRequireCapability(w, r, "session.use")
	if !ok {
		return
	}
	workerID := s.runtimePrincipalAgentWorkerID(principal)
	if strings.TrimSpace(workerID) == "" {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeRuntimeCapabilityRequired, "runtime agent is not linked to an agent worker")
		return
	}
	var body runtimeSessionForkRequest
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	title := strings.TrimSpace(body.Title)
	if title == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "session title is required")
		return
	}
	project := strings.TrimSpace(body.Project)
	if project == "" {
		project = principal.Project
	}
	if project != principal.Project {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeRuntimeCapabilityRequired, "fork session project must match the current runtime project in this version")
		return
	}
	if ok := s.checkRuntimeForkSessionCapacity(w, principal.WorkspaceID, workerID); !ok {
		return
	}
	membershipID := strings.TrimSpace(principal.ProjectMembershipID)
	if membershipID == "" {
		_, membershipID = s.agentWorkerContextForProjectAgent(principal.WorkspaceID, project, principal.Agent)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	capabilities := body.Capabilities
	if capabilities == nil {
		capabilities = map[string]any{"mode": "inherit"}
	}
	capabilitiesJSON, _ := json.Marshal(capabilities)
	permissionPolicy := strings.TrimSpace(body.PermissionPolicy)
	if permissionPolicy == "" {
		permissionPolicy = "inherit"
	}
	session := controldb.AgentSession{
		ID:                  "fs_" + randomHex(12),
		WorkspaceID:         principal.WorkspaceID,
		AgentWorkerID:       workerID,
		SessionKind:         "fork",
		ParentSessionID:     strings.TrimSpace(body.ParentSessionID),
		ProjectID:           project,
		ProjectMembershipID: membershipID,
		TaskID:              strings.TrimSpace(body.TaskID),
		WorkflowInstanceID:  strings.TrimSpace(body.WorkflowInstanceID),
		Title:               title,
		Purpose:             strings.TrimSpace(body.Purpose),
		InitialPrompt:       strings.TrimSpace(body.InitialPrompt),
		Status:              "pending",
		RuntimeProvider:     strings.TrimSpace(body.RuntimeProvider),
		ForkMode:            "fresh_with_context",
		PermissionPolicy:    permissionPolicy,
		CapabilitiesJSON:    string(capabilitiesJSON),
		ResultRefsJSON:      "{}",
		CreatedByRunID:      strings.TrimSpace(principal.RunID),
		CreatedAt:           now,
		UpdatedAt:           now,
		LastActivityAt:      now,
	}
	if session.ParentSessionID != "" && supportsNativeFork(body.RuntimeProvider) {
		session.ForkMode = "native_fork"
	}
	if err := s.controlDB.UpsertAgentSession(session); err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  principal.WorkspaceID,
		ActorType:    "agent",
		ActorID:      runtimeAgentAddress(principal),
		Action:       "agent_session.fork",
		ResourceType: "agent_session",
		ResourceID:   session.ID,
		Summary:      "Runtime agent created a fork session",
		After:        runtimeSessionResponse(session),
		Request:      r,
	})
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"session": runtimeSessionResponse(session)})
}

func (s *Server) checkRuntimeForkSessionCapacity(w http.ResponseWriter, workspaceID, workerID string) bool {
	if s == nil || s.controlDB == nil {
		return true
	}
	worker, found, err := s.controlDB.AgentWorkerByID(workspaceID, workerID)
	if err != nil {
		s.serverError(w, err)
		return false
	}
	if !found {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeRuntimeCapabilityRequired, "runtime agent is not linked to an agent worker")
		return false
	}
	max := agentWorkerMaxForkSessions(worker)
	if max <= 0 {
		return true
	}
	sessions, err := s.controlDB.ListAgentSessions(controldb.AgentSessionFilter{
		WorkspaceID:   workspaceID,
		AgentWorkerID: workerID,
		SessionKind:   "fork",
		Statuses:      []string{"pending", "running", "waiting", "blocked"},
		Limit:         max + 1,
	})
	if err != nil {
		s.serverError(w, err)
		return false
	}
	if len(sessions) >= max {
		s.jsonErrorCode(w, http.StatusConflict, ErrCodeConflict, fmt.Sprintf("active fork sessions limit reached: %d", max))
		return false
	}
	return true
}

func agentWorkerMaxForkSessions(worker controldb.AgentWorker) int {
	config := normalizeAgentWorkerRuntimeConfig(decodeJSONValue(worker.RuntimeConfigJSON, map[string]any{}))
	return intFromRuntimeConfigValue(config["maxForkSessions"])
}

func (s *Server) handleRuntimeSessionGet(w http.ResponseWriter, r *http.Request) {
	principal, session, ok := s.runtimeSessionForRequest(w, r)
	if !ok {
		return
	}
	_ = principal
	_ = json.NewEncoder(w).Encode(map[string]any{"session": runtimeSessionResponse(session)})
}

func (s *Server) handleRuntimeSessionPatch(w http.ResponseWriter, r *http.Request) {
	principal, session, ok := s.runtimeSessionForRequest(w, r)
	if !ok {
		return
	}
	var body runtimeSessionPatchRequest
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	before := session
	status := normalizeRuntimeSessionStatus(body.Status)
	if strings.TrimSpace(body.Status) != "" && status == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "invalid session status")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if status != "" {
		session.Status = status
		if isTerminalRuntimeSessionStatus(status) && session.CompletedAt == "" {
			session.CompletedAt = now
		}
		if !isTerminalRuntimeSessionStatus(status) {
			session.CompletedAt = ""
		}
	}
	if strings.TrimSpace(body.ResultSummary) != "" {
		session.ResultSummary = strings.TrimSpace(body.ResultSummary)
	}
	if body.ResultRefs != nil {
		raw, _ := json.Marshal(body.ResultRefs)
		session.ResultRefsJSON = string(raw)
	}
	if strings.TrimSpace(body.RuntimeSessionID) != "" {
		session.RuntimeSessionID = strings.TrimSpace(body.RuntimeSessionID)
	}
	if strings.TrimSpace(body.LastRunID) != "" {
		session.LastRunID = strings.TrimSpace(body.LastRunID)
	}
	session.UpdatedAt = now
	session.LastActivityAt = now
	var queuedRun *controldb.RuntimeRun
	if body.Run {
		runPrompt := strings.TrimSpace(body.Prompt)
		if runPrompt == "" {
			runPrompt = strings.TrimSpace(body.ResultSummary)
		}
		run, err := s.enqueueRuntimeForkSessionRun(principal, session, runPrompt, externalServerURL(r))
		if err != nil {
			s.serverError(w, err)
			return
		}
		queuedRun = &run
		session.Status = "running"
		session.LastRunID = run.ID
		session.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		session.LastActivityAt = session.UpdatedAt
	}
	if err := s.controlDB.UpsertAgentSession(session); err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  principal.WorkspaceID,
		ActorType:    "agent",
		ActorID:      runtimeAgentAddress(principal),
		Action:       "agent_session.update",
		ResourceType: "agent_session",
		ResourceID:   session.ID,
		Summary:      "Runtime agent updated a fork session",
		Before:       runtimeSessionResponse(before),
		After:        runtimeSessionResponse(session),
		Request:      r,
	})
	resp := map[string]any{"session": runtimeSessionResponse(session)}
	if queuedRun != nil {
		resp["run"] = runtimeRunResponse(*queuedRun)
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) enqueueRuntimeForkSessionRun(principal runtimeAgentPrincipal, session controldb.AgentSession, prompt, serverURL string) (controldb.RuntimeRun, error) {
	if s == nil || s.controlDB == nil {
		return controldb.RuntimeRun{}, fmt.Errorf("control database is not available")
	}
	project := strings.TrimSpace(session.ProjectID)
	if project == "" {
		project = principal.Project
	}
	if project == "" {
		return controldb.RuntimeRun{}, fmt.Errorf("session project is required")
	}
	agent := strings.TrimSpace(principal.Agent)
	if agent == "" {
		return controldb.RuntimeRun{}, fmt.Errorf("runtime agent is required")
	}
	meta, err := s.agentMetaForProjectMember(principal.WorkspaceID, project, agent)
	if err != nil {
		return controldb.RuntimeRun{}, err
	}
	workerID := strings.TrimSpace(session.AgentWorkerID)
	if workerID == "" {
		workerID = s.runtimePrincipalAgentWorkerID(principal)
	}
	membershipID := strings.TrimSpace(session.ProjectMembershipID)
	if membershipID == "" {
		_, membershipID = s.agentWorkerContextForProjectAgent(principal.WorkspaceID, project, agent)
	}
	runID := newRuntimeID("rtrun")
	token := s.issueAgentRuntimeToken(runtimeAgentTokenPayload{
		Type:                "agent_runtime",
		WorkspaceID:         principal.WorkspaceID,
		Project:             project,
		Agent:               agent,
		AgentWorkerID:       workerID,
		ProjectMembershipID: membershipID,
		RunID:               runID,
		Capabilities:        defaultRuntimeCapabilities(),
	}, 6*time.Hour)
	if strings.TrimSpace(prompt) == "" {
		prompt = strings.TrimSpace(session.InitialPrompt)
	}
	if strings.TrimSpace(prompt) == "" {
		prompt = "Continue this fork session. Inspect the session purpose and complete the assigned work. When finished, run `mga session collect " + session.ID + " --summary \"...\"`."
	}
	preparedPrompt := buildForkSessionPrompt(session, prompt)
	runtimeControlEnv := map[string]string{
		"MULTIGENT_API_URL":               strings.TrimRight(serverURL, "/"),
		"MULTIGENT_AGENT_TOKEN":           token,
		"MULTIGENT_RUN_ID":                runID,
		"MULTIGENT_WORKSPACE_ID":          principal.WorkspaceID,
		"MULTIGENT_PROJECT":               project,
		"MULTIGENT_AGENT":                 agent,
		"MULTIGENT_AGENT_WORKER_ID":       workerID,
		"MULTIGENT_PROJECT_MEMBERSHIP_ID": membershipID,
		"MULTIGENT_FORK_SESSION_ID":       session.ID,
	}
	if strings.TrimSpace(session.TaskID) != "" {
		runtimeControlEnv["MULTIGENT_TASK_ID"] = strings.TrimSpace(session.TaskID)
	}
	spec := runtimeexec.Spec{
		Kind:              runtimeexec.KindForkSession,
		WorkspaceID:       principal.WorkspaceID,
		ProjectID:         project,
		AgentID:           agent,
		TaskID:            strings.TrimSpace(session.TaskID),
		ForkSessionID:     session.ID,
		SessionID:         strings.TrimSpace(session.RuntimeSessionID),
		Prompt:            preparedPrompt,
		Agent:             *meta,
		ProviderEnv:       s.runtimeProviderEnvForAgent(principal.WorkspaceID, project, agent, meta),
		RuntimeControlEnv: runtimeControlEnv,
	}
	specBody, err := json.Marshal(spec)
	if err != nil {
		return controldb.RuntimeRun{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	run := controldb.RuntimeRun{
		ID:                   runID,
		WorkspaceID:          principal.WorkspaceID,
		AgentWorkerID:        workerID,
		ProjectMembershipID:  membershipID,
		ProjectID:            project,
		AgentID:              agent,
		TaskID:               strings.TrimSpace(session.TaskID),
		WorkflowInstanceID:   strings.TrimSpace(session.WorkflowInstanceID),
		ForkSessionID:        session.ID,
		DesiredRuntimeNodeID: strings.TrimSpace(meta.RuntimeNodeID),
		Status:               "queued",
		Priority:             2,
		SpecJSON:             string(specBody),
		ResultJSON:           "{}",
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := s.controlDB.UpsertRuntimeRun(run); err != nil {
		return controldb.RuntimeRun{}, err
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  principal.WorkspaceID,
		ActorType:    "agent",
		ActorID:      runtimeAgentAddress(principal),
		Action:       "agent_session.run",
		ResourceType: "agent_session",
		ResourceID:   session.ID,
		Summary:      "Runtime agent queued a fork session run",
		After: map[string]any{
			"sessionId": session.ID,
			"runId":     run.ID,
			"project":   project,
			"agent":     agent,
			"taskId":    strings.TrimSpace(session.TaskID),
		},
	})
	return run, nil
}

func buildForkSessionPrompt(session controldb.AgentSession, prompt string) string {
	var b strings.Builder
	b.WriteString("You are running as a Multigent fork session, not a new long-lived Agent Worker.\n")
	b.WriteString("You inherit the parent agent's permissions, model, tools, and skills. Keep work scoped to this fork session.\n\n")
	b.WriteString("Fork session metadata:\n")
	b.WriteString("- Session ID: " + session.ID + "\n")
	if strings.TrimSpace(session.Title) != "" {
		b.WriteString("- Title: " + strings.TrimSpace(session.Title) + "\n")
	}
	if strings.TrimSpace(session.Purpose) != "" {
		b.WriteString("- Purpose: " + strings.TrimSpace(session.Purpose) + "\n")
	}
	if strings.TrimSpace(session.TaskID) != "" {
		b.WriteString("- Parent task ID: " + strings.TrimSpace(session.TaskID) + "\n")
	}
	if strings.TrimSpace(session.ParentSessionID) != "" {
		b.WriteString("- Parent runtime session ID: " + strings.TrimSpace(session.ParentSessionID) + "\n")
	}
	b.WriteString("\nWhen this fork session reaches a useful stopping point, report its result with:\n")
	b.WriteString("  mga session collect " + session.ID + " --summary \"what was done, outputs, blockers, next step\"\n")
	b.WriteString("If blocked, use:\n")
	b.WriteString("  mga session collect " + session.ID + " --status blocked --summary \"blocker and what is needed\"\n\n")
	b.WriteString("Work instruction:\n")
	b.WriteString(strings.TrimSpace(prompt))
	b.WriteString("\n")
	return b.String()
}

func (s *Server) runtimeSessionForRequest(w http.ResponseWriter, r *http.Request) (runtimeAgentPrincipal, controldb.AgentSession, bool) {
	principal, ok := s.runtimeRequireCapability(w, r, "session.use")
	if !ok {
		return runtimeAgentPrincipal{}, controldb.AgentSession{}, false
	}
	workerID := s.runtimePrincipalAgentWorkerID(principal)
	if strings.TrimSpace(workerID) == "" {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeRuntimeCapabilityRequired, "runtime agent is not linked to an agent worker")
		return runtimeAgentPrincipal{}, controldb.AgentSession{}, false
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "session id is required")
		return runtimeAgentPrincipal{}, controldb.AgentSession{}, false
	}
	session, found, err := s.controlDB.AgentSessionByID(principal.WorkspaceID, id)
	if err != nil {
		s.serverError(w, err)
		return runtimeAgentPrincipal{}, controldb.AgentSession{}, false
	}
	if !found {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeNotFound, "session not found")
		return runtimeAgentPrincipal{}, controldb.AgentSession{}, false
	}
	if strings.TrimSpace(session.AgentWorkerID) != workerID {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeRuntimeCapabilityRequired, "session belongs to another agent")
		return runtimeAgentPrincipal{}, controldb.AgentSession{}, false
	}
	return principal, session, true
}

func normalizeRuntimeSessionStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "":
		return ""
	case "pending", "running", "waiting", "blocked", "done", "failed", "stopped":
		return strings.ToLower(strings.TrimSpace(status))
	case "active", "resume", "resumed":
		return "pending"
	case "success", "completed":
		return "done"
	case "cancelled", "canceled", "stop":
		return "stopped"
	default:
		return ""
	}
}

func isTerminalRuntimeSessionStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "done", "failed", "stopped":
		return true
	default:
		return false
	}
}

func supportsNativeFork(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "claudecode", "claude", "claude-code", "codex":
		return true
	default:
		return false
	}
}

func runtimeSessionResponses(sessions []controldb.AgentSession) []map[string]any {
	out := make([]map[string]any, 0, len(sessions))
	for _, session := range sessions {
		out = append(out, runtimeSessionResponse(session))
	}
	return out
}

func runtimeSessionResponse(session controldb.AgentSession) map[string]any {
	return map[string]any{
		"id":                 session.ID,
		"workspaceId":        session.WorkspaceID,
		"agentWorkerId":      session.AgentWorkerID,
		"kind":               session.SessionKind,
		"parentSessionId":    session.ParentSessionID,
		"project":            session.ProjectID,
		"membershipId":       session.ProjectMembershipID,
		"taskId":             session.TaskID,
		"workflowInstanceId": session.WorkflowInstanceID,
		"title":              session.Title,
		"purpose":            session.Purpose,
		"initialPrompt":      session.InitialPrompt,
		"status":             session.Status,
		"runtimeProvider":    session.RuntimeProvider,
		"runtimeSessionId":   session.RuntimeSessionID,
		"forkMode":           session.ForkMode,
		"permissionPolicy":   session.PermissionPolicy,
		"capabilities":       jsonObject(session.CapabilitiesJSON),
		"resultSummary":      session.ResultSummary,
		"resultRefs":         jsonObject(session.ResultRefsJSON),
		"createdByRunId":     session.CreatedByRunID,
		"lastRunId":          session.LastRunID,
		"createdAt":          session.CreatedAt,
		"updatedAt":          session.UpdatedAt,
		"lastActivityAt":     session.LastActivityAt,
		"completedAt":        session.CompletedAt,
	}
}

func jsonObject(raw string) map[string]any {
	out := map[string]any{}
	_ = json.Unmarshal([]byte(firstNonEmpty(strings.TrimSpace(raw), "{}")), &out)
	return out
}
