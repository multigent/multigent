package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/runner"
	"github.com/multigent/multigent/internal/runtimeexec"
	"github.com/multigent/multigent/internal/sandbox"
	"github.com/multigent/multigent/internal/store"
)

const ctxRuntimeNodeKey contextKey = "runtime-node"
const runtimeNodeOnlineWindow = 2 * time.Minute

type runtimeNodePrincipal struct {
	Token controldb.RuntimeNodeToken
	Node  controldb.RuntimeNode
}

type createRuntimeNodeTokenRequest struct {
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	ExpiresIn     string `json:"expiresIn"`
	RuntimeNodeID string `json:"runtimeNodeId"`
}

type runtimeNodeRegisterRequest struct {
	OS               string         `json:"os"`
	Arch             string         `json:"arch"`
	Hostname         string         `json:"hostname"`
	Version          string         `json:"version"`
	Capabilities     map[string]any `json:"capabilities"`
	CapabilitiesHash string         `json:"capabilitiesHash"`
	LastError        string         `json:"lastError"`
}

type runtimeNodeHeartbeatRequest struct {
	Status       string         `json:"status"`
	OS           string         `json:"os"`
	Arch         string         `json:"arch"`
	Hostname     string         `json:"hostname"`
	Version      string         `json:"version"`
	Capabilities map[string]any `json:"capabilities"`
	LastError    string         `json:"lastError"`
}

type runtimeRunClaimRequest struct {
	Capacity         int      `json:"capacity"`
	CapabilitiesHash string   `json:"capabilitiesHash"`
	BusyAgents       []string `json:"busyAgents,omitempty"`
}

type runtimeRunEventRequest struct {
	Sequence int64          `json:"sequence"`
	Type     string         `json:"type"`
	Payload  map[string]any `json:"payload"`
}

type runtimeRunLeaseRequest struct {
	LeaseSeconds int `json:"leaseSeconds"`
}

type runtimeRunFinishRequest struct {
	Result       map[string]any `json:"result"`
	ErrorCode    string         `json:"errorCode"`
	ErrorMessage string         `json:"errorMessage"`
}

const runtimeWorkflowStepNotCompletedError = "workflow step was not completed by the agent; use `mga task step done --id <id>` with every required output field, or `mga task step done --id <id> --status failed --error <reason>` if the step cannot be completed"

type createRuntimeExecRunRequest struct {
	Message   string `json:"message"`
	SessionID string `json:"sessionId"`
}

func (s *Server) handleRuntimeNodes(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}
	if !s.canAdminWorkspace(r, workspaceID) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeWorkspaceAdminRequired, "workspace admin access required")
		return
	}
	nodes, err := s.controlDB.ListRuntimeNodes(workspaceID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	resp := make([]map[string]any, 0, len(nodes))
	for _, node := range nodes {
		resp = append(resp, runtimeNodeResponse(node))
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"nodes": resp})
}

func (s *Server) hasOnlineRuntimeNode(workspaceID string) bool {
	if s == nil || s.controlDB == nil || strings.TrimSpace(workspaceID) == "" {
		return false
	}
	nodes, err := s.controlDB.ListRuntimeNodes(workspaceID)
	if err != nil {
		return false
	}
	for _, node := range nodes {
		if runtimeNodeIsOnline(node) {
			return true
		}
	}
	return false
}

func (s *Server) assignedRuntimeNode(workspaceID string, meta *entity.AgentMeta) (controldb.RuntimeNode, bool) {
	if s == nil || s.controlDB == nil || meta == nil || strings.TrimSpace(workspaceID) == "" {
		return controldb.RuntimeNode{}, false
	}
	nodeID := strings.TrimSpace(meta.RuntimeNodeID)
	if nodeID == "" {
		return controldb.RuntimeNode{}, false
	}
	node, found, err := s.controlDB.RuntimeNodeByID(workspaceID, nodeID)
	if err != nil || !found || !runtimeNodeIsOnline(node) {
		return controldb.RuntimeNode{}, false
	}
	return node, true
}

func (s *Server) usesAssignedRuntimeNode(workspaceID string, meta *entity.AgentMeta) bool {
	_, ok := s.assignedRuntimeNode(workspaceID, meta)
	return ok
}

func (s *Server) runtimeReadinessForExecution(workspaceID string, meta *entity.AgentMeta) runtimeReadinessResponse {
	requireNode := runtimeNodeRequired()
	nodeID := ""
	if meta != nil {
		nodeID = strings.TrimSpace(meta.RuntimeNodeID)
	}
	if requireNode && nodeID == "" {
		return runtimeNodeBlockingReadiness("Runtime node is required before this agent can run.", "This workspace is configured for customer-provided Runtime Nodes. Add a Runtime Node in Settings, then bind this agent to that node.")
	}
	readiness := buildRuntimeReadiness(meta)
	if nodeID == "" {
		return readiness
	}
	node, found, err := s.controlDB.RuntimeNodeByID(workspaceID, nodeID)
	if err != nil || !found || !runtimeNodeIsOnline(node) {
		statusDetail := "Assigned runtime node is not online."
		if found {
			statusDetail = fmt.Sprintf("Assigned runtime node %q is %s.", firstNonEmpty(node.Name, node.ID), runtimeNodeEffectiveStatus(node))
			if strings.TrimSpace(node.LastError) != "" {
				statusDetail += " Last error: " + strings.TrimSpace(node.LastError)
			}
		}
		if requireNode {
			return runtimeNodeBlockingReadiness("Assigned runtime node is not ready.", statusDetail)
		}
		checks := append([]setupCheck(nil), readiness.Checks...)
		checks = append(checks, runtimeNodeBlockingCheck(statusDetail))
		return runtimeReadinessResponse{Ready: false, Blocking: true, Summary: "Assigned runtime node is not ready.", Checks: checks}
	}
	provider := entity.SandboxDocker
	if meta.Sandbox != nil {
		provider = meta.Sandbox.Provider
	}
	if provider == entity.SandboxNone && entity.NormaliseModel(meta.Model) == entity.ModelClaudeCode && runtimeNodeDirectRunsAsRoot(node) {
		checks := append([]setupCheck(nil), readiness.Checks...)
		checks = append(checks, setupCheck{
			Key:      "runtime_node_claudecode_root",
			Label:    "Runtime Node",
			Status:   "error",
			Detail:   "Assigned runtime node is running as root. Claude Code refuses bypassPermissions under root/sudo privileges for direct host execution.",
			Action:   "Restart the Runtime Node as a normal user, or switch this agent to Docker sandbox execution.",
			Blocking: true,
		})
		return runtimeReadinessResponse{
			Ready:    false,
			Blocking: true,
			Summary:  "Assigned Runtime Node cannot run Claude Code direct execution as root.",
			Checks:   checks,
		}
	}
	if !readiness.Blocking {
		return readiness
	}
	filtered := readiness
	filtered.Checks = append([]setupCheck(nil), readiness.Checks...)
	for i := range filtered.Checks {
		switch filtered.Checks[i].Key {
		case "sandbox", "cli", "docker", "runtime_image", "runtime_container":
			if filtered.Checks[i].Status == "error" {
				filtered.Checks[i].Status = "warning"
				filtered.Checks[i].Detail = firstNonEmpty(filtered.Checks[i].Detail, "Local runtime is not ready, but this agent is assigned to an online Runtime Node.")
				filtered.Checks[i].Blocking = false
			}
		}
	}
	blocking := false
	warnings := 0
	for _, check := range filtered.Checks {
		if check.Blocking || check.Status == "error" {
			blocking = true
		}
		if check.Status == "warning" {
			warnings++
		}
	}
	filtered.Blocking = blocking
	filtered.Ready = !blocking
	if blocking {
		filtered.Summary = "Runtime is not ready. Resolve blocking checks before running this agent."
	} else if warnings > 0 {
		filtered.Summary = "Assigned Runtime Node is available. Local runtime preparation warnings will not block this run."
	} else {
		filtered.Summary = "Runtime is ready."
	}
	return filtered
}

func runtimeNodeBlockingReadiness(summary, detail string) runtimeReadinessResponse {
	return runtimeReadinessResponse{
		Ready:    false,
		Blocking: true,
		Summary:  summary,
		Checks:   []setupCheck{runtimeNodeBlockingCheck(detail)},
	}
}

func runtimeNodeBlockingCheck(detail string) setupCheck {
	return setupCheck{
		Key:      "runtime_node",
		Label:    "Runtime Node",
		Status:   "error",
		Detail:   detail,
		Action:   "Open Settings → Runtime Nodes, add a node, run the join command on your machine, then select it in this agent's advanced settings.",
		Blocking: true,
	}
}

func runtimeNodeRequired() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("MULTIGENT_REQUIRE_RUNTIME_NODE")))
	switch value {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func runtimeNodeDirectRunsAsRoot(node controldb.RuntimeNode) bool {
	var caps map[string]any
	if err := json.Unmarshal([]byte(node.CapabilitiesJSON), &caps); err != nil {
		return false
	}
	direct, ok := caps["direct"].(map[string]any)
	if !ok {
		return false
	}
	isRoot, _ := direct["isRoot"].(bool)
	return isRoot
}

func decodeRuntimeRunResult(raw string) map[string]string {
	out := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	var generic map[string]any
	if err := json.Unmarshal([]byte(raw), &generic); err != nil {
		return out
	}
	for k, v := range generic {
		switch typed := v.(type) {
		case string:
			out[k] = typed
		case float64, bool:
			out[k] = strings.TrimSpace(strings.Trim(fmt.Sprint(typed), "\""))
		default:
			if body, err := json.Marshal(typed); err == nil {
				out[k] = string(body)
			}
		}
	}
	return out
}

func (s *Server) handleCreateRuntimeExecRun(w http.ResponseWriter, r *http.Request) {
	project := strings.TrimSpace(r.PathValue("name"))
	agent := strings.TrimSpace(r.PathValue("agent"))
	if !s.checkProjectAccess(w, r, project) {
		return
	}
	if !s.agentExistsInProject(project, agent) {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeAgentNotFound, "agent not found")
		return
	}
	if !s.canOperateAgent(r, project, agent) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeAgentOperatorRequired, "agent operator access required")
		return
	}
	var body createRuntimeExecRunRequest
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	message := strings.TrimSpace(body.Message)
	if message == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "message is required")
		return
	}
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}
	meta, err := s.st.AgentMeta(project, agent)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !s.usesAssignedRuntimeNode(workspaceID, meta) {
		s.jsonErrorCode(w, http.StatusConflict, ErrCodeRuntimeNotReady, "agent is not assigned to an online runtime node")
		return
	}
	run, err := s.enqueueRuntimeExecRun(workspaceID, project, agent, message, strings.TrimSpace(body.SessionID), externalServerURL(r), requestUsername(r))
	if err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"run": runtimeRunResponse(run)})
}

func (s *Server) enqueueRuntimeExecRun(workspaceID, project, agent, prompt, sessionID, serverURL, actor string) (controldb.RuntimeRun, error) {
	meta, err := s.st.AgentMeta(project, agent)
	if err != nil {
		return controldb.RuntimeRun{}, err
	}
	runID := newRuntimeID("rtrun")
	token := s.issueAgentRuntimeToken(runtimeAgentTokenPayload{
		Type:         "agent_runtime",
		WorkspaceID:  workspaceID,
		Project:      project,
		Agent:        agent,
		RunID:        runID,
		Capabilities: defaultRuntimeCapabilities(),
	}, 6*time.Hour)
	spec := runtimeexec.Spec{
		Kind:        runtimeexec.KindExecPrompt,
		WorkspaceID: workspaceID,
		ProjectID:   project,
		AgentID:     agent,
		SessionID:   sessionID,
		Prompt:      prompt,
		Agent:       *meta,
		ProviderEnv: s.runtimeProviderEnvForAgent(project, agent, meta),
		RuntimeControlEnv: map[string]string{
			"MULTIGENT_API_URL":      strings.TrimRight(serverURL, "/"),
			"MULTIGENT_AGENT_TOKEN":  token,
			"MULTIGENT_RUN_ID":       runID,
			"MULTIGENT_WORKSPACE_ID": workspaceID,
		},
	}
	specBody, err := json.Marshal(spec)
	if err != nil {
		return controldb.RuntimeRun{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	run := controldb.RuntimeRun{
		ID:                   runID,
		WorkspaceID:          workspaceID,
		ProjectID:            project,
		AgentID:              agent,
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
		WorkspaceID:  workspaceID,
		Action:       "runtime_run.enqueue",
		ResourceType: "runtime_run",
		ResourceID:   run.ID,
		Summary:      "Runtime run queued",
		After: map[string]any{
			"project": project,
			"agent":   agent,
			"kind":    spec.Kind,
			"actor":   actor,
		},
	})
	return run, nil
}

func (s *Server) enqueueRuntimeTaskRun(workspaceID, project, agent string, task *entity.Task, sessionID, serverURL, actor string) (controldb.RuntimeRun, error) {
	if task == nil {
		return controldb.RuntimeRun{}, fmt.Errorf("task is required")
	}
	meta, err := s.st.AgentMeta(project, agent)
	if err != nil {
		return controldb.RuntimeRun{}, err
	}
	runID := newRuntimeID("rtrun")
	token := s.issueAgentRuntimeToken(runtimeAgentTokenPayload{
		Type:         "agent_runtime",
		WorkspaceID:  workspaceID,
		Project:      project,
		Agent:        agent,
		RunID:        runID,
		Capabilities: defaultRuntimeCapabilities(),
	}, 6*time.Hour)
	preparedPrompt := runner.New(s.root, s.ts, s.st).BuildTaskPrompt(project, agent, task)
	spec := runtimeexec.Spec{
		Kind:        runtimeexec.KindTask,
		WorkspaceID: workspaceID,
		ProjectID:   project,
		AgentID:     agent,
		TaskID:      task.ID,
		SessionID:   sessionID,
		Prompt:      preparedPrompt,
		Agent:       *meta,
		ProviderEnv: s.runtimeProviderEnvForAgent(project, agent, meta),
		RuntimeControlEnv: map[string]string{
			"MULTIGENT_API_URL":      strings.TrimRight(serverURL, "/"),
			"MULTIGENT_AGENT_TOKEN":  token,
			"MULTIGENT_RUN_ID":       runID,
			"MULTIGENT_TASK_ID":      task.ID,
			"MULTIGENT_WORKSPACE_ID": workspaceID,
		},
	}
	specBody, err := json.Marshal(spec)
	if err != nil {
		return controldb.RuntimeRun{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	run := controldb.RuntimeRun{
		ID:                   runID,
		WorkspaceID:          workspaceID,
		ProjectID:            project,
		AgentID:              agent,
		TaskID:               task.ID,
		DesiredRuntimeNodeID: strings.TrimSpace(meta.RuntimeNodeID),
		Status:               "queued",
		Priority:             task.Priority,
		SpecJSON:             string(specBody),
		ResultJSON:           "{}",
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := s.controlDB.UpsertRuntimeRun(run); err != nil {
		return controldb.RuntimeRun{}, err
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "runtime_run.enqueue",
		ResourceType: "runtime_run",
		ResourceID:   run.ID,
		Summary:      "Runtime task run queued",
		After: map[string]any{
			"project": project,
			"agent":   agent,
			"taskId":  task.ID,
			"actor":   actor,
		},
	})
	return run, nil
}

func (s *Server) runtimeProviderEnvForAgent(project, agent string, meta *entity.AgentMeta) map[string]string {
	if meta == nil {
		return nil
	}
	env := map[string]string{
		"MULTIGENT":         "1",
		"MULTIGENT_PROJECT": meta.Project,
		"MULTIGENT_AGENT":   meta.Name,
		"MULTIGENT_TEAM":    meta.Team,
		"MULTIGENT_ROLE":    meta.Role,
		"MULTIGENT_MODEL":   string(meta.Model),
	}
	if wsEnv, err := store.NewEnvVarStore(s.root).ResolveEnvForAgent(project, agent); err == nil {
		for k, v := range wsEnv {
			env[k] = v
		}
	}
	if strings.TrimSpace(meta.Provider) != "" {
		if provEnv, err := store.NewProviderStoreWithDB(s.root, s.controlDB).ResolveEnvForModel(meta.Provider, meta.Model); err == nil {
			for k, v := range provEnv {
				env[k] = v
			}
		}
	}
	for k, v := range meta.Env {
		env[k] = v
	}
	for k, v := range runtimeModelEnvForSpec(meta.Model, meta.RuntimeModel) {
		env[k] = v
	}
	return env
}

func runtimeModelEnvForSpec(model entity.AgentModel, runtimeModel string) map[string]string {
	runtimeModel = strings.TrimSpace(runtimeModel)
	if runtimeModel == "" {
		return nil
	}
	switch entity.NormaliseModel(model) {
	case entity.ModelClaudeCode:
		return map[string]string{"ANTHROPIC_MODEL": runtimeModel, "CLAUDE_MODEL": runtimeModel}
	case entity.ModelCodex:
		return map[string]string{"OPENAI_MODEL": runtimeModel, "CODEX_MODEL": runtimeModel}
	case entity.ModelGemini:
		return map[string]string{"GEMINI_MODEL": runtimeModel, "GOOGLE_MODEL": runtimeModel}
	case entity.ModelCursor:
		return map[string]string{"CURSOR_MODEL": runtimeModel}
	case entity.ModelOpenCode:
		return map[string]string{"OPENAI_MODEL": runtimeModel}
	default:
		return map[string]string{"MULTIGENT_RUNTIME_MODEL": runtimeModel}
	}
}

func (s *Server) handleCreateRuntimeNodeJoinToken(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}
	if !s.canAdminWorkspace(r, workspaceID) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeWorkspaceAdminRequired, "workspace admin access required")
		return
	}
	var body createRuntimeNodeTokenRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := s.readJSON(w, r, &body); err != nil {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
			return
		}
	}
	now := time.Now().UTC()
	nodeID := strings.TrimSpace(body.RuntimeNodeID)
	var node controldb.RuntimeNode
	if nodeID != "" {
		foundNode, found, err := s.controlDB.RuntimeNodeByID(workspaceID, nodeID)
		if err != nil {
			s.serverError(w, err)
			return
		}
		if !found {
			s.jsonErrorCode(w, http.StatusNotFound, ErrCodeNotFound, "runtime node not found")
			return
		}
		node = foundNode
		if name := strings.TrimSpace(body.Name); name != "" && name != node.Name {
			node.Name = name
			node.UpdatedAt = now.Format(time.RFC3339)
			if err := s.controlDB.UpsertRuntimeNode(node); err != nil {
				s.serverError(w, err)
				return
			}
		}
	} else {
		if !s.checkRuntimeNodeEntitlement(w, r, workspaceID, 1) {
			return
		}
		name := strings.TrimSpace(body.Name)
		if name == "" {
			name = "Runtime Node"
		}
		kind := strings.TrimSpace(body.Kind)
		if kind == "" {
			kind = "personal_computer"
		}
		node = controldb.RuntimeNode{
			ID:               newRuntimeID("rtn"),
			WorkspaceID:      workspaceID,
			Name:             name,
			Kind:             kind,
			Status:           "pending",
			CapabilitiesJSON: "{}",
			PolicyJSON:       "{}",
			CreatedByUserID:  requestUsername(r),
			CreatedAt:        now.Format(time.RFC3339),
			UpdatedAt:        now.Format(time.RFC3339),
		}
		if err := s.controlDB.UpsertRuntimeNode(node); err != nil {
			s.serverError(w, err)
			return
		}
	}
	rawToken := "mgrt_" + randomRuntimeHex(32)
	ttl := parseRuntimeTTL(body.ExpiresIn, 30*time.Minute)
	token := controldb.RuntimeNodeToken{
		ID:            newRuntimeID("rtok"),
		WorkspaceID:   workspaceID,
		RuntimeNodeID: node.ID,
		TokenHash:     hashRuntimeToken(rawToken),
		Name:          "join",
		ScopesJSON:    `["runtime.join","runtime.heartbeat","runtime.claim"]`,
		ExpiresAt:     now.Add(ttl).Format(time.RFC3339),
		CreatedBy:     requestUsername(r),
		CreatedAt:     now.Format(time.RFC3339),
	}
	if err := s.controlDB.CreateRuntimeNodeToken(token); err != nil {
		s.serverError(w, err)
		return
	}
	serverURL := externalServerURL(r)
	installCommand := "multigent runtime join --server " + serverURL + " --token " + rawToken
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "runtime_node.join_token.create",
		ResourceType: "runtime_node",
		ResourceID:   node.ID,
		Summary:      "Runtime node join token created",
		After: map[string]any{
			"name":      node.Name,
			"kind":      node.Kind,
			"expiresAt": token.ExpiresAt,
		},
		Request: r,
	})
	_ = json.NewEncoder(w).Encode(map[string]any{
		"runtimeNode":    runtimeNodeResponse(node),
		"runtimeNodeId":  node.ID,
		"joinToken":      rawToken,
		"expiresAt":      token.ExpiresAt,
		"serverUrl":      serverURL,
		"installCommand": installCommand,
	})
}

func (s *Server) handleRuntimeNode(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}
	if !s.canAdminWorkspace(r, workspaceID) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeWorkspaceAdminRequired, "workspace admin access required")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	node, found, err := s.controlDB.RuntimeNodeByID(workspaceID, id)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeNotFound, "runtime node not found")
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"node": runtimeNodeResponse(node)})
}

func (s *Server) handleDisableRuntimeNode(w http.ResponseWriter, r *http.Request) {
	s.setRuntimeNodeStatus(w, r, "disabled")
}

func (s *Server) handleEnableRuntimeNode(w http.ResponseWriter, r *http.Request) {
	s.setRuntimeNodeStatus(w, r, "pending")
}

func (s *Server) setRuntimeNodeStatus(w http.ResponseWriter, r *http.Request, status string) {
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}
	if !s.canAdminWorkspace(r, workspaceID) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeWorkspaceAdminRequired, "workspace admin access required")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	node, found, err := s.controlDB.RuntimeNodeByID(workspaceID, id)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeNotFound, "runtime node not found")
		return
	}
	node.Status = status
	node.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := s.controlDB.UpsertRuntimeNode(node); err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"node": runtimeNodeResponse(node)})
}

func (s *Server) handleDeleteRuntimeNode(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}
	if !s.canAdminWorkspace(r, workspaceID) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeWorkspaceAdminRequired, "workspace admin access required")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if err := s.controlDB.DeleteRuntimeNode(workspaceID, id); err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *Server) handleRuntimeNodeRegister(w http.ResponseWriter, r *http.Request) {
	principal, ok := runtimeNodeFromRequest(r)
	if !ok {
		s.jsonErrorCode(w, http.StatusUnauthorized, ErrCodeUnauthorized, "runtime node token required")
		return
	}
	var body runtimeNodeRegisterRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := s.readJSON(w, r, &body); err != nil {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
			return
		}
	}
	node := principal.Node
	now := time.Now().UTC().Format(time.RFC3339)
	reportedOS := firstNonEmpty(strings.TrimSpace(body.OS), runtime.GOOS)
	reportedArch := firstNonEmpty(strings.TrimSpace(body.Arch), runtime.GOARCH)
	reportedHostname := strings.TrimSpace(body.Hostname)
	if existing, found := s.findExistingRuntimeNodeByFingerprint(node.WorkspaceID, node.ID, reportedOS, reportedArch, reportedHostname); found {
		principal.Token.RuntimeNodeID = existing.ID
		principal.Token.ExpiresAt = ""
		principal.Token.Name = "runtime"
		if err := s.controlDB.UpdateRuntimeNodeToken(principal.Token); err != nil {
			s.serverError(w, err)
			return
		}
		if err := s.controlDB.DeleteRuntimeNode(node.WorkspaceID, node.ID); err != nil {
			s.serverError(w, err)
			return
		}
		node = existing
	}
	if node.Status != "disabled" {
		node.Status = "registered"
	}
	node.OS = reportedOS
	node.Arch = reportedArch
	node.Hostname = reportedHostname
	node.Version = strings.TrimSpace(body.Version)
	node.CapabilitiesJSON = marshalRuntimeObject(body.Capabilities)
	node.LastSeenAt = now
	node.LastError = strings.TrimSpace(body.LastError)
	node.UpdatedAt = now
	if err := s.controlDB.UpsertRuntimeNode(node); err != nil {
		s.serverError(w, err)
		return
	}
	if principal.Token.ExpiresAt != "" || principal.Token.Name != "runtime" {
		principal.Token.RuntimeNodeID = node.ID
		principal.Token.ExpiresAt = ""
		principal.Token.Name = "runtime"
		if err := s.controlDB.UpdateRuntimeNodeToken(principal.Token); err != nil {
			s.serverError(w, err)
			return
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"node": runtimeNodeResponse(node), "status": "registered"})
}

func (s *Server) findExistingRuntimeNodeByFingerprint(workspaceID, currentID, osName, archName, hostname string) (controldb.RuntimeNode, bool) {
	workspaceID = strings.TrimSpace(workspaceID)
	currentID = strings.TrimSpace(currentID)
	hostname = strings.TrimSpace(hostname)
	if workspaceID == "" || hostname == "" {
		return controldb.RuntimeNode{}, false
	}
	nodes, err := s.controlDB.ListRuntimeNodes(workspaceID)
	if err != nil {
		return controldb.RuntimeNode{}, false
	}
	for _, node := range nodes {
		if node.ID == currentID || node.Status == "disabled" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(node.Hostname), hostname) &&
			strings.EqualFold(strings.TrimSpace(node.OS), strings.TrimSpace(osName)) &&
			strings.EqualFold(strings.TrimSpace(node.Arch), strings.TrimSpace(archName)) {
			return node, true
		}
	}
	return controldb.RuntimeNode{}, false
}

func (s *Server) handleRuntimeNodeHeartbeat(w http.ResponseWriter, r *http.Request) {
	principal, ok := runtimeNodeFromRequest(r)
	if !ok {
		s.jsonErrorCode(w, http.StatusUnauthorized, ErrCodeUnauthorized, "runtime node token required")
		return
	}
	var body runtimeNodeHeartbeatRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := s.readJSON(w, r, &body); err != nil {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
			return
		}
	}
	node := principal.Node
	now := time.Now().UTC().Format(time.RFC3339)
	if node.Status != "disabled" {
		node.Status = firstNonEmpty(strings.TrimSpace(body.Status), "online")
	}
	if strings.TrimSpace(body.OS) != "" {
		node.OS = strings.TrimSpace(body.OS)
	}
	if strings.TrimSpace(body.Arch) != "" {
		node.Arch = strings.TrimSpace(body.Arch)
	}
	if strings.TrimSpace(body.Hostname) != "" {
		node.Hostname = strings.TrimSpace(body.Hostname)
	}
	if strings.TrimSpace(body.Version) != "" {
		node.Version = strings.TrimSpace(body.Version)
	}
	if body.Capabilities != nil {
		node.CapabilitiesJSON = marshalRuntimeObject(body.Capabilities)
	}
	node.LastSeenAt = now
	node.LastError = strings.TrimSpace(body.LastError)
	node.UpdatedAt = now
	if err := s.controlDB.UpsertRuntimeNode(node); err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "node": runtimeNodeResponse(node), "serverTime": now})
}

func (s *Server) handleRuntimeNodeClaimRun(w http.ResponseWriter, r *http.Request) {
	principal, ok := runtimeNodeFromRequest(r)
	if !ok {
		s.jsonErrorCode(w, http.StatusUnauthorized, ErrCodeUnauthorized, "runtime node token required")
		return
	}
	var body runtimeRunClaimRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := s.readJSON(w, r, &body); err != nil {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
			return
		}
	}
	if principal.Node.Status == "disabled" {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeForbidden, "runtime node is disabled")
		return
	}
	run, found, err := s.controlDB.ClaimRuntimeRun(principal.Node.WorkspaceID, principal.Node.ID, 90, body.BusyAgents)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found {
		_ = json.NewEncoder(w).Encode(map[string]any{"run": nil, "retryAfterMs": 3000})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"run": runtimeRunResponse(run), "retryAfterMs": 0})
}

func (s *Server) handleRuntimeNodeRunSpec(w http.ResponseWriter, r *http.Request) {
	principal, ok := runtimeNodeFromRequest(r)
	if !ok {
		s.jsonErrorCode(w, http.StatusUnauthorized, ErrCodeUnauthorized, "runtime node token required")
		return
	}
	runID := strings.TrimSpace(r.PathValue("runId"))
	run, found, err := s.controlDB.RuntimeRunByID(principal.Node.WorkspaceID, runID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found || run.RuntimeNodeID != principal.Node.ID {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeNotFound, "runtime run not found")
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"run": runtimeRunResponse(run), "spec": json.RawMessage(defaultRawJSON(run.SpecJSON))})
}

func (s *Server) handleRuntimeNodeRunEvent(w http.ResponseWriter, r *http.Request) {
	principal, ok := runtimeNodeFromRequest(r)
	if !ok {
		s.jsonErrorCode(w, http.StatusUnauthorized, ErrCodeUnauthorized, "runtime node token required")
		return
	}
	runID := strings.TrimSpace(r.PathValue("runId"))
	run, found, err := s.controlDB.RuntimeRunByID(principal.Node.WorkspaceID, runID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found || run.RuntimeNodeID != principal.Node.ID {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeNotFound, "runtime run not found")
		return
	}
	var body runtimeRunEventRequest
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	if body.Sequence <= 0 {
		body.Sequence = time.Now().UTC().UnixNano()
	}
	if strings.TrimSpace(body.Type) == "" {
		body.Type = "system"
	}
	event := controldb.RuntimeEvent{
		ID:          newRuntimeID("rtev"),
		WorkspaceID: principal.Node.WorkspaceID,
		RunID:       run.ID,
		Sequence:    body.Sequence,
		Type:        strings.TrimSpace(body.Type),
		PayloadJSON: marshalRuntimeObject(body.Payload),
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if err := s.controlDB.CreateRuntimeEvent(event); err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *Server) handleRuntimeNodeRunLease(w http.ResponseWriter, r *http.Request) {
	principal, ok := runtimeNodeFromRequest(r)
	if !ok {
		s.jsonErrorCode(w, http.StatusUnauthorized, ErrCodeUnauthorized, "runtime node token required")
		return
	}
	runID := strings.TrimSpace(r.PathValue("runId"))
	var body runtimeRunLeaseRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := s.readJSON(w, r, &body); err != nil {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
			return
		}
	}
	leaseSeconds := body.LeaseSeconds
	if leaseSeconds <= 0 {
		leaseSeconds = 90
	}
	run, found, err := s.controlDB.ExtendRuntimeRunLease(principal.Node.WorkspaceID, runID, principal.Node.ID, leaseSeconds)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found {
		existing, existingFound, existingErr := s.controlDB.RuntimeRunByID(principal.Node.WorkspaceID, runID)
		if existingErr != nil {
			s.serverError(w, existingErr)
			return
		}
		if existingFound && existing.RuntimeNodeID == principal.Node.ID && existing.Status == "cancelled" {
			_ = json.NewEncoder(w).Encode(map[string]any{"run": runtimeRunResponse(existing), "cancelled": true})
			return
		}
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeNotFound, "runtime run not found")
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"run": runtimeRunResponse(run), "leaseExpiresAt": run.LeaseExpiresAt})
}

func (s *Server) handleRuntimeNodeRunComplete(w http.ResponseWriter, r *http.Request) {
	s.finishRuntimeNodeRun(w, r, "succeeded")
}

func (s *Server) handleRuntimeNodeRunFail(w http.ResponseWriter, r *http.Request) {
	s.finishRuntimeNodeRun(w, r, "failed")
}

func (s *Server) finishRuntimeNodeRun(w http.ResponseWriter, r *http.Request, status string) {
	principal, ok := runtimeNodeFromRequest(r)
	if !ok {
		s.jsonErrorCode(w, http.StatusUnauthorized, ErrCodeUnauthorized, "runtime node token required")
		return
	}
	runID := strings.TrimSpace(r.PathValue("runId"))
	run, found, err := s.controlDB.RuntimeRunByID(principal.Node.WorkspaceID, runID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found || run.RuntimeNodeID != principal.Node.ID {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeNotFound, "runtime run not found")
		return
	}
	var body runtimeRunFinishRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := s.readJSON(w, r, &body); err != nil {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
			return
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	run.Status = status
	run.ResultJSON = marshalRuntimeObject(body.Result)
	run.ErrorCode = strings.TrimSpace(body.ErrorCode)
	run.ErrorMessage = strings.TrimSpace(body.ErrorMessage)
	run.FinishedAt = now
	run.UpdatedAt = now
	s.finalizeRuntimeTaskRun(&run, body)
	if err := s.controlDB.UpsertRuntimeRun(run); err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"run": runtimeRunResponse(run)})
}

func (s *Server) finalizeRuntimeTaskRun(run *controldb.RuntimeRun, body runtimeRunFinishRequest) {
	if s == nil || s.ts == nil || run == nil || strings.TrimSpace(run.TaskID) == "" {
		return
	}
	now := time.Now().UTC()
	if hb, err := s.ts.GetHeartbeat(run.ProjectID, run.AgentID); err == nil && hb != nil {
		if run.Status == "failed" {
			hb.LastWakeupStatus = "failed"
		} else {
			hb.LastWakeupStatus = "done"
		}
		if sid, _ := body.Result["sessionId"].(string); strings.TrimSpace(sid) != "" {
			hb.SessionID = strings.TrimSpace(sid)
			hb.SessionStartedAt = &now
		}
		_ = s.ts.SaveHeartbeat(run.ProjectID, run.AgentID, hb)
	}
	task, err := s.ts.GetTask(run.ProjectID, run.AgentID, run.TaskID)
	if err != nil || task == nil {
		return
	}
	if task.Status.IsTerminal() {
		return
	}
	if s.runtimeTaskHasWorkflow(run.WorkspaceID, run.ProjectID, run.TaskID) {
		if run.Status != "failed" && (task.Status == entity.TaskStatusInProgress || task.Status == entity.TaskStatusPending) {
			msg := runtimeWorkflowStepNotCompletedError
			prev := task.Status
			task.Status = entity.TaskStatusDoneFailed
			task.LastError = msg
			task.UpdatedAt = now
			entity.ApplyStatusTimestamps(task, prev, now)
			_ = s.ts.ArchiveTask(run.ProjectID, run.AgentID, task)
			run.Status = "failed"
			run.ErrorCode = "workflow_step_not_completed"
			run.ErrorMessage = msg
			if body.Result == nil {
				body.Result = map[string]any{}
			}
			body.Result["error"] = msg
			run.ResultJSON = marshalRuntimeObject(body.Result)
		}
		return
	}
	if task.Status != entity.TaskStatusInProgress && task.Status != entity.TaskStatusPending {
		return
	}
	prev := task.Status
	if run.Status == "failed" {
		task.Status = entity.TaskStatusDoneFailed
		task.LastError = firstNonEmpty(strings.TrimSpace(body.ErrorMessage), strings.TrimSpace(body.ErrorCode), "runtime run failed")
	} else {
		task.Status = entity.TaskStatusDoneSuccess
		if summary, _ := body.Result["summary"].(string); strings.TrimSpace(summary) != "" {
			task.Summary = strings.TrimSpace(summary)
		}
	}
	task.UpdatedAt = now
	entity.ApplyStatusTimestamps(task, prev, now)
	_ = s.ts.ArchiveTask(run.ProjectID, run.AgentID, task)
}

func (s *Server) cancelRuntimeRunsForAgent(workspaceID, project, agent, taskID, reason string) (int, error) {
	if s == nil || s.controlDB == nil {
		return 0, nil
	}
	total := 0
	for _, status := range []string{"queued", "running"} {
		runs, err := s.controlDB.ListRuntimeRuns(controldb.RuntimeRunFilter{
			WorkspaceID: workspaceID,
			ProjectID:   project,
			AgentID:     agent,
			TaskID:      taskID,
			Status:      status,
			Limit:       500,
		})
		if err != nil {
			return total, err
		}
		for _, run := range runs {
			run.Status = "cancelled"
			run.ErrorCode = "cancelled"
			run.ErrorMessage = firstNonEmpty(strings.TrimSpace(reason), "runtime run cancelled")
			now := time.Now().UTC().Format(time.RFC3339)
			run.FinishedAt = now
			run.UpdatedAt = now
			if err := s.controlDB.UpsertRuntimeRun(run); err != nil {
				return total, err
			}
			total++
		}
	}
	return total, nil
}

func (s *Server) withRuntimeNodeAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			s.jsonErrorCode(w, http.StatusUnauthorized, ErrCodeUnauthorized, "runtime node token required")
			return
		}
		record, found, err := s.controlDB.RuntimeNodeTokenByHash(hashRuntimeToken(token))
		if err != nil {
			s.serverError(w, err)
			return
		}
		if !found || strings.TrimSpace(record.RevokedAt) != "" || runtimeTokenExpired(record.ExpiresAt) {
			s.jsonErrorCode(w, http.StatusUnauthorized, ErrCodeUnauthorized, "invalid or expired runtime node token")
			return
		}
		node, found, err := s.controlDB.RuntimeNodeByID(record.WorkspaceID, record.RuntimeNodeID)
		if err != nil {
			s.serverError(w, err)
			return
		}
		if !found {
			s.jsonErrorCode(w, http.StatusUnauthorized, ErrCodeUnauthorized, "runtime node not found")
			return
		}
		ctx := context.WithValue(r.Context(), ctxRuntimeNodeKey, runtimeNodePrincipal{Token: record, Node: node})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func runtimeNodeFromRequest(r *http.Request) (runtimeNodePrincipal, bool) {
	principal, ok := r.Context().Value(ctxRuntimeNodeKey).(runtimeNodePrincipal)
	return principal, ok
}

func runtimeNodeResponse(node controldb.RuntimeNode) map[string]any {
	return map[string]any{
		"id":               node.ID,
		"workspaceId":      node.WorkspaceID,
		"name":             node.Name,
		"kind":             node.Kind,
		"status":           runtimeNodeEffectiveStatus(node),
		"storedStatus":     node.Status,
		"os":               node.OS,
		"arch":             node.Arch,
		"hostname":         node.Hostname,
		"version":          node.Version,
		"capabilitiesJson": node.CapabilitiesJSON,
		"policyJson":       node.PolicyJSON,
		"lastSeenAt":       node.LastSeenAt,
		"lastError":        node.LastError,
		"createdByUserId":  node.CreatedByUserID,
		"createdAt":        node.CreatedAt,
		"updatedAt":        node.UpdatedAt,
	}
}

func runtimeNodeEffectiveStatus(node controldb.RuntimeNode) string {
	status := strings.TrimSpace(node.Status)
	if status == "" {
		status = "pending"
	}
	if status != "online" {
		return status
	}
	if !runtimeNodeSeenRecently(node.LastSeenAt) {
		return "offline"
	}
	return "online"
}

func runtimeNodeIsOnline(node controldb.RuntimeNode) bool {
	return runtimeNodeEffectiveStatus(node) == "online"
}

func runtimeNodeSeenRecently(lastSeenAt string) bool {
	lastSeenAt = strings.TrimSpace(lastSeenAt)
	if lastSeenAt == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, lastSeenAt)
	if err != nil {
		return false
	}
	return time.Since(t) <= runtimeNodeOnlineWindow
}

func runtimeRunResponse(run controldb.RuntimeRun) map[string]any {
	return map[string]any{
		"id":                   run.ID,
		"workspaceId":          run.WorkspaceID,
		"projectId":            run.ProjectID,
		"agentId":              run.AgentID,
		"taskId":               run.TaskID,
		"workflowInstanceId":   run.WorkflowInstanceID,
		"workflowStepId":       run.WorkflowStepID,
		"desiredRuntimeNodeId": run.DesiredRuntimeNodeID,
		"runtimeNodeId":        run.RuntimeNodeID,
		"status":               run.Status,
		"priority":             run.Priority,
		"leaseExpiresAt":       run.LeaseExpiresAt,
		"claimedAt":            run.ClaimedAt,
		"startedAt":            run.StartedAt,
		"finishedAt":           run.FinishedAt,
		"errorCode":            run.ErrorCode,
		"errorMessage":         run.ErrorMessage,
		"createdAt":            run.CreatedAt,
		"updatedAt":            run.UpdatedAt,
	}
}

func runtimeTokenExpired(expiresAt string) bool {
	expiresAt = strings.TrimSpace(expiresAt)
	if expiresAt == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return true
	}
	return time.Now().UTC().After(t)
}

func hashRuntimeToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func newRuntimeID(prefix string) string {
	return prefix + "_" + randomRuntimeHex(8)
}

func randomRuntimeHex(n int) string {
	if n <= 0 {
		n = 16
	}
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format("150405.000000000")))
	}
	return hex.EncodeToString(b)
}

func parseRuntimeTTL(raw string, fallback time.Duration) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return fallback
	}
	if d > 24*time.Hour {
		return 24 * time.Hour
	}
	return d
}

func externalServerURL(r *http.Request) string {
	proto := r.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		proto = "http"
		if r.TLS != nil {
			proto = "https"
		}
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	prefix := strings.TrimSpace(r.Header.Get("X-Forwarded-Prefix"))
	if prefix != "" {
		prefix = "/" + strings.Trim(prefix, "/")
		if prefix == "/" || strings.Contains(prefix, "..") {
			prefix = ""
		}
	}
	return strings.TrimRight(proto+"://"+host+prefix, "/")
}

func marshalRuntimeObject(v map[string]any) string {
	if v == nil {
		return "{}"
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func defaultRawJSON(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "{}"
	}
	return raw
}

func detectLocalRuntimeCapabilities() map[string]any {
	caps := sandbox.DetectCapabilities()
	return map[string]any{
		"os":   runtime.GOOS,
		"arch": runtime.GOARCH,
		"sandbox": map[string]any{
			"docker": map[string]any{
				"available": caps.Docker.Available,
				"reason":    caps.Docker.Reason,
			},
			"e2b": map[string]any{
				"available": caps.E2B.Available,
				"reason":    caps.E2B.Reason,
			},
		},
	}
}
