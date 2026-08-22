package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/multigent/multigent/internal/avatar"
	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
)

type agentWorkerRequest struct {
	Name                  string         `json:"name"`
	DisplayName           string         `json:"displayName"`
	Description           string         `json:"description"`
	ProfilePrompt         string         `json:"profilePrompt"`
	Avatar                string         `json:"avatar"`
	Team                  string         `json:"team"`
	Role                  string         `json:"role"`
	Status                string         `json:"status"`
	Model                 string         `json:"model"`
	RuntimeModel          string         `json:"runtimeModel"`
	DefaultModelAccountID string         `json:"defaultModelAccountId"`
	DefaultRuntimeNodeID  string         `json:"defaultRuntimeNodeId"`
	DefaultRuntimeMode    string         `json:"defaultRuntimeMode"`
	Schedule              map[string]any `json:"schedule"`
	AttentionPolicy       map[string]any `json:"attentionPolicy"`
	MemoryPolicy          map[string]any `json:"memoryPolicy"`
	RuntimeConfig         map[string]any `json:"runtimeConfig"`
	Skills                []string       `json:"skills"`
}

type patchAgentWorkerRequest struct {
	DisplayName           *string        `json:"displayName"`
	Description           *string        `json:"description"`
	ProfilePrompt         *string        `json:"profilePrompt"`
	Avatar                *string        `json:"avatar"`
	Team                  *string        `json:"team"`
	Role                  *string        `json:"role"`
	Status                *string        `json:"status"`
	Model                 *string        `json:"model"`
	RuntimeModel          *string        `json:"runtimeModel"`
	DefaultModelAccountID *string        `json:"defaultModelAccountId"`
	DefaultRuntimeNodeID  *string        `json:"defaultRuntimeNodeId"`
	DefaultRuntimeMode    *string        `json:"defaultRuntimeMode"`
	Schedule              map[string]any `json:"schedule"`
	AttentionPolicy       map[string]any `json:"attentionPolicy"`
	MemoryPolicy          map[string]any `json:"memoryPolicy"`
	RuntimeConfig         map[string]any `json:"runtimeConfig"`
	Skills                []string       `json:"skills"`
}

type projectMembershipRequest struct {
	WorkerID         string   `json:"workerId"`
	WorkerName       string   `json:"workerName"`
	Role             string   `json:"role"`
	Title            string   `json:"title"`
	Prompt           string   `json:"prompt"`
	Permissions      []string `json:"permissions"`
	AutoPickTasks    *bool    `json:"autoPickTasks"`
	AttentionEnabled *bool    `json:"attentionEnabled"`
	PriorityWeight   float64  `json:"priorityWeight"`
}

func (s *Server) handleAgentWorkers(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}
	workers, err := s.controlDB.ListAgentWorkers(workspaceID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	memberships, err := s.controlDB.ListProjectMemberships(controldb.ProjectMembershipFilter{
		WorkspaceID: workspaceID,
		MemberType:  "agent_worker",
	})
	if err != nil {
		s.serverError(w, err)
		return
	}
	membershipsByWorker := make(map[string][]controldb.ProjectMembership, len(workers))
	roleTeams := s.projectMembershipRoleTeams()
	membershipTeams := make(map[string]string, len(memberships))
	workersByID := make(map[string]controldb.AgentWorker, len(workers))
	for _, worker := range workers {
		workersByID[worker.ID] = worker
	}
	for _, membership := range memberships {
		membershipsByWorker[membership.MemberID] = append(membershipsByWorker[membership.MemberID], membership)
		if worker, ok := workersByID[membership.MemberID]; ok {
			membershipTeams[membership.ID] = s.projectMembershipTeam(membership, worker, roleTeams)
		}
	}
	if !s.canAdminWorkspace(r, workspaceID) {
		workers, membershipsByWorker = s.visibleAgentWorkersForRequest(r, workers, membershipsByWorker)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"agents": agentWorkerResponses(workers, membershipsByWorker, membershipTeams)})
}

func (s *Server) handleCreateAgentWorker(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	var body agentWorkerRequest
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "agent name is required")
		return
	}
	if !validAgentName(name) {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidAgentName, "invalid agent name")
		return
	}
	if _, ok, err := s.controlDB.AgentWorkerByName(workspaceID, name); err != nil {
		s.serverError(w, err)
		return
	} else if ok {
		s.jsonErrorCode(w, http.StatusConflict, ErrCodeAgentAlreadyExists, "agent already exists")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	worker := controldb.AgentWorker{
		ID:                    "aw_" + randomHex(12),
		WorkspaceID:           workspaceID,
		Name:                  name,
		DisplayName:           strings.TrimSpace(body.DisplayName),
		Description:           strings.TrimSpace(body.Description),
		ProfilePrompt:         strings.TrimSpace(body.ProfilePrompt),
		Avatar:                agentWorkerAvatar(controldb.AgentWorker{WorkspaceID: workspaceID, Name: name, Avatar: strings.TrimSpace(body.Avatar)}),
		Team:                  strings.TrimSpace(body.Team),
		Role:                  strings.TrimSpace(body.Role),
		Status:                normalizeAgentWorkerStatus(body.Status),
		Model:                 strings.TrimSpace(body.Model),
		RuntimeModel:          strings.TrimSpace(body.RuntimeModel),
		DefaultModelAccountID: strings.TrimSpace(body.DefaultModelAccountID),
		DefaultRuntimeNodeID:  strings.TrimSpace(body.DefaultRuntimeNodeID),
		DefaultRuntimeMode:    strings.TrimSpace(body.DefaultRuntimeMode),
		ScheduleJSON:          normalizeAgentWorkerScheduleJSON(body.Schedule),
		AttentionPolicyJSON:   jsonStringOrDefault(body.AttentionPolicy, "{}"),
		MemoryPolicyJSON:      jsonStringOrDefault(body.MemoryPolicy, "{}"),
		RuntimeConfigJSON:     normalizeAgentWorkerRuntimeConfigJSON(body.RuntimeConfig),
		SkillsJSON:            jsonStringOrDefault(normalizeStringList(body.Skills), "[]"),
		PrimarySessionID:      "sess_" + randomHex(16),
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if worker.DisplayName == "" {
		worker.DisplayName = worker.Name
	}
	if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"agent": agentWorkerResponse(worker)})
}

func (s *Server) handleAgentWorker(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}
	worker, ok, err := s.agentDirectory.Worker(workspaceID, r.PathValue("id"))
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !ok {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeAgentNotFound, "agent not found")
		return
	}
	memberships, err := s.controlDB.ListProjectMemberships(controldb.ProjectMembershipFilter{
		WorkspaceID: workspaceID,
		MemberType:  "agent_worker",
		MemberID:    worker.ID,
	})
	if err != nil {
		s.serverError(w, err)
		return
	}
	roleTeams := s.projectMembershipRoleTeams()
	membershipTeams := make(map[string]string, len(memberships))
	for _, membership := range memberships {
		membershipTeams[membership.ID] = s.projectMembershipTeam(membership, worker, roleTeams)
	}
	if !s.canAdminWorkspace(r, workspaceID) {
		memberships = s.visibleAgentWorkerMembershipsForRequest(r, worker, memberships)
		if len(memberships) == 0 {
			s.jsonErrorCode(w, http.StatusForbidden, ErrCodeAgentAccessRequired, "agent access required")
			return
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"agent":       agentWorkerResponse(worker),
		"memberships": projectMembershipResponses(memberships, nil, membershipTeams),
	})
}

func (s *Server) handlePatchAgentWorker(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	worker, ok, err := s.agentDirectory.Worker(workspaceID, r.PathValue("id"))
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !ok {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeAgentNotFound, "agent not found")
		return
	}
	var body patchAgentWorkerRequest
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	if body.DisplayName != nil {
		worker.DisplayName = strings.TrimSpace(*body.DisplayName)
	}
	if body.Description != nil {
		worker.Description = strings.TrimSpace(*body.Description)
	}
	if body.ProfilePrompt != nil {
		worker.ProfilePrompt = strings.TrimSpace(*body.ProfilePrompt)
	}
	if body.Avatar != nil {
		worker.Avatar = strings.TrimSpace(*body.Avatar)
	}
	if body.Team != nil {
		worker.Team = strings.TrimSpace(*body.Team)
	}
	if body.Role != nil {
		worker.Role = strings.TrimSpace(*body.Role)
	}
	if body.Status != nil {
		worker.Status = normalizeAgentWorkerStatus(*body.Status)
	}
	if body.Model != nil {
		worker.Model = strings.TrimSpace(*body.Model)
	}
	if body.RuntimeModel != nil {
		worker.RuntimeModel = strings.TrimSpace(*body.RuntimeModel)
	}
	if body.DefaultModelAccountID != nil {
		worker.DefaultModelAccountID = strings.TrimSpace(*body.DefaultModelAccountID)
	}
	if body.DefaultRuntimeNodeID != nil {
		worker.DefaultRuntimeNodeID = strings.TrimSpace(*body.DefaultRuntimeNodeID)
	}
	if body.DefaultRuntimeMode != nil {
		worker.DefaultRuntimeMode = strings.TrimSpace(*body.DefaultRuntimeMode)
	}
	if body.Schedule != nil {
		worker.ScheduleJSON = normalizeAgentWorkerScheduleJSON(body.Schedule)
	}
	if body.AttentionPolicy != nil {
		worker.AttentionPolicyJSON = jsonStringOrDefault(body.AttentionPolicy, "{}")
	}
	if body.MemoryPolicy != nil {
		worker.MemoryPolicyJSON = jsonStringOrDefault(body.MemoryPolicy, "{}")
	}
	if body.RuntimeConfig != nil {
		worker.RuntimeConfigJSON = normalizeAgentWorkerRuntimeConfigJSON(body.RuntimeConfig)
	}
	if body.Skills != nil {
		worker.SkillsJSON = jsonStringOrDefault(normalizeStringList(body.Skills), "[]")
	}
	worker.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"agent": agentWorkerResponse(worker)})
}

func (s *Server) handleDeleteAgentWorker(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	worker, ok, err := s.agentDirectory.Worker(workspaceID, r.PathValue("id"))
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !ok {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeAgentNotFound, "agent not found")
		return
	}
	memberships, err := s.controlDB.ListProjectMemberships(controldb.ProjectMembershipFilter{
		WorkspaceID: workspaceID,
		MemberType:  "agent_worker",
		MemberID:    worker.ID,
	})
	if err != nil {
		s.serverError(w, err)
		return
	}
	if len(memberships) > 0 {
		s.jsonErrorCode(w, http.StatusConflict, ErrCodeConflict, "agent is still assigned to projects")
		return
	}
	if err := s.controlDB.DeleteAgentWorker(workspaceID, worker.ID); err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *Server) handleProjectMemberships(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("name")
	if !s.checkProjectAccess(w, r, project) {
		return
	}
	if _, err := s.st.Project(project); err != nil {
		if isNotFoundErr(err) {
			s.jsonErrorCode(w, http.StatusNotFound, ErrCodeProjectNotFound, "project not found")
			return
		}
		s.serverError(w, err)
		return
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	memberships, err := s.controlDB.ListProjectMemberships(controldb.ProjectMembershipFilter{
		WorkspaceID: workspaceID,
		ProjectID:   project,
	})
	if err != nil {
		s.serverError(w, err)
		return
	}
	workers, err := s.controlDB.ListAgentWorkers(workspaceID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	workerMap := make(map[string]controldb.AgentWorker, len(workers))
	for _, worker := range workers {
		workerMap[worker.ID] = worker
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"memberships": projectMembershipResponses(memberships, workerMap)})
}

func (s *Server) handleCreateProjectMembership(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("name")
	if !s.checkProjectManager(w, r, project) {
		return
	}
	if _, err := s.st.Project(project); err != nil {
		if isNotFoundErr(err) {
			s.jsonErrorCode(w, http.StatusNotFound, ErrCodeProjectNotFound, "project not found")
			return
		}
		s.serverError(w, err)
		return
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	var body projectMembershipRequest
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	workerRef := strings.TrimSpace(body.WorkerID)
	if workerRef == "" {
		workerRef = strings.TrimSpace(body.WorkerName)
	}
	if workerRef == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "workerId or workerName is required")
		return
	}
	worker, ok, err := s.agentDirectory.Worker(workspaceID, workerRef)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !ok {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeAgentNotFound, "agent not found")
		return
	}
	autoPick := true
	if body.AutoPickTasks != nil {
		autoPick = *body.AutoPickTasks
	}
	attentionEnabled := true
	if body.AttentionEnabled != nil {
		attentionEnabled = *body.AttentionEnabled
	}
	role := strings.TrimSpace(body.Role)
	if role == "" {
		role = "member"
	}
	title := strings.TrimSpace(body.Title)
	if title == "" {
		title = worker.DisplayName
	}
	if title == "" {
		title = worker.Name
	}
	weight := body.PriorityWeight
	if weight <= 0 {
		weight = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	membership := controldb.ProjectMembership{
		ID:               "pm_" + randomHex(12),
		WorkspaceID:      workspaceID,
		ProjectID:        project,
		MemberType:       "agent_worker",
		MemberID:         worker.ID,
		Role:             role,
		Title:            title,
		Prompt:           strings.TrimSpace(body.Prompt),
		PermissionsJSON:  jsonStringOrDefault(normalizeStringList(body.Permissions), "[]"),
		AutoPickTasks:    autoPick,
		AttentionEnabled: attentionEnabled,
		PriorityWeight:   weight,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.controlDB.UpsertProjectMembership(membership); err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"membership": projectMembershipResponse(membership, &worker),
	})
}

func (s *Server) handleDeleteProjectMembership(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("name")
	if !s.checkProjectManager(w, r, project) {
		return
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	membershipID := strings.TrimSpace(r.PathValue("membershipId"))
	membership, ok, err := s.controlDB.ProjectMembershipByID(workspaceID, membershipID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !ok || membership.ProjectID != project {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeNotFound, "project membership not found")
		return
	}
	if err := s.controlDB.DeleteProjectMembership(workspaceID, membership.ID); err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func agentWorkerResponses(workers []controldb.AgentWorker, membershipsByWorker map[string][]controldb.ProjectMembership, membershipTeams ...map[string]string) []map[string]any {
	out := make([]map[string]any, 0, len(workers))
	for _, worker := range workers {
		item := agentWorkerResponse(worker)
		if membershipsByWorker != nil {
			item["memberships"] = projectMembershipResponses(membershipsByWorker[worker.ID], nil, membershipTeams...)
		}
		out = append(out, item)
	}
	return out
}

func agentWorkerResponse(worker controldb.AgentWorker) map[string]any {
	return map[string]any{
		"id":                    worker.ID,
		"workspaceId":           worker.WorkspaceID,
		"name":                  worker.Name,
		"displayName":           worker.DisplayName,
		"description":           worker.Description,
		"profilePrompt":         worker.ProfilePrompt,
		"avatar":                agentWorkerAvatar(worker),
		"team":                  worker.Team,
		"role":                  worker.Role,
		"status":                worker.Status,
		"model":                 worker.Model,
		"runtimeModel":          worker.RuntimeModel,
		"defaultModelAccountId": worker.DefaultModelAccountID,
		"defaultRuntimeNodeId":  worker.DefaultRuntimeNodeID,
		"defaultRuntimeMode":    worker.DefaultRuntimeMode,
		"schedule":              decodeJSONValue(worker.ScheduleJSON, map[string]any{}),
		"attentionPolicy":       decodeJSONValue(worker.AttentionPolicyJSON, map[string]any{}),
		"memoryPolicy":          decodeJSONValue(worker.MemoryPolicyJSON, map[string]any{}),
		"runtimeConfig":         normalizeAgentWorkerRuntimeConfig(decodeJSONValue(worker.RuntimeConfigJSON, map[string]any{})),
		"skills":                decodeJSONValue(worker.SkillsJSON, []any{}),
		"primarySessionId":      worker.PrimarySessionID,
		"createdAt":             worker.CreatedAt,
		"updatedAt":             worker.UpdatedAt,
	}
}

func (s *Server) visibleAgentWorkersForRequest(r *http.Request, workers []controldb.AgentWorker, membershipsByWorker map[string][]controldb.ProjectMembership) ([]controldb.AgentWorker, map[string][]controldb.ProjectMembership) {
	filteredWorkers := make([]controldb.AgentWorker, 0, len(workers))
	filteredMemberships := make(map[string][]controldb.ProjectMembership, len(workers))
	for _, worker := range workers {
		visibleMemberships := s.visibleAgentWorkerMembershipsForRequest(r, worker, membershipsByWorker[worker.ID])
		if len(visibleMemberships) == 0 {
			continue
		}
		filteredWorkers = append(filteredWorkers, worker)
		filteredMemberships[worker.ID] = visibleMemberships
	}
	return filteredWorkers, filteredMemberships
}

func (s *Server) visibleAgentWorkerMembershipsForRequest(r *http.Request, worker controldb.AgentWorker, memberships []controldb.ProjectMembership) []controldb.ProjectMembership {
	out := make([]controldb.ProjectMembership, 0, len(memberships))
	for _, membership := range memberships {
		if s.canViewAgentWorkerMembership(r, worker, membership) {
			out = append(out, membership)
		}
	}
	return out
}

func (s *Server) canViewAgentWorkerMembership(r *http.Request, worker controldb.AgentWorker, membership controldb.ProjectMembership) bool {
	project := strings.TrimSpace(membership.ProjectID)
	if project == "" {
		return false
	}
	if s.canAccessWholeProject(r, project) {
		return true
	}
	cur := s.currentUser(r)
	for _, agent := range []string{membership.Title, worker.DisplayName, worker.Name} {
		agent = strings.TrimSpace(agent)
		if agent == "" {
			continue
		}
		if _, ok := currentUserAgentRole(cur, project, agent); ok {
			return true
		}
	}
	return false
}

func (s *Server) canAccessAgentWorkerForRequest(r *http.Request, workspaceID string, worker controldb.AgentWorker) (bool, error) {
	if s.canAdminWorkspace(r, workspaceID) {
		return true, nil
	}
	memberships, err := s.controlDB.ListProjectMemberships(controldb.ProjectMembershipFilter{
		WorkspaceID: workspaceID,
		MemberType:  "agent_worker",
		MemberID:    worker.ID,
	})
	if err != nil {
		return false, err
	}
	return len(s.visibleAgentWorkerMembershipsForRequest(r, worker, memberships)) > 0, nil
}

func agentWorkerAvatar(worker controldb.AgentWorker) string {
	if value := strings.TrimSpace(worker.Avatar); value != "" {
		return value
	}
	seed := strings.Trim(strings.Join([]string{worker.WorkspaceID, worker.Name, worker.ID}, "-"), "-")
	return avatar.URL(seed)
}

func projectMembershipResponses(memberships []controldb.ProjectMembership, workers map[string]controldb.AgentWorker, membershipTeams ...map[string]string) []map[string]any {
	out := make([]map[string]any, 0, len(memberships))
	for _, membership := range memberships {
		var worker *controldb.AgentWorker
		if workers != nil {
			if w, ok := workers[membership.MemberID]; ok {
				copy := w
				worker = &copy
			}
		}
		item := projectMembershipResponse(membership, worker)
		if len(membershipTeams) > 0 {
			if team := strings.TrimSpace(membershipTeams[0][membership.ID]); team != "" {
				item["team"] = team
			}
		}
		out = append(out, item)
	}
	return out
}

func (s *Server) projectMembershipRoleTeams() map[string]string {
	teams, err := s.st.ListTeams()
	if err != nil {
		return nil
	}
	out := make(map[string]string)
	for _, team := range teams {
		if team == nil || strings.TrimSpace(team.Path) == "" {
			continue
		}
		roles, err := s.st.ListRoles(team.Path)
		if err != nil {
			continue
		}
		for _, role := range roles {
			if role == nil {
				continue
			}
			name := strings.TrimSpace(role.Name)
			if name == "" {
				continue
			}
			if _, exists := out[name]; !exists {
				out[name] = team.Path
			}
		}
	}
	return out
}

func (s *Server) projectMembershipTeam(membership controldb.ProjectMembership, worker controldb.AgentWorker, roleTeams map[string]string) string {
	role := strings.TrimSpace(membership.Role)
	if role == "" || len(roleTeams) == 0 {
		return ""
	}
	return strings.TrimSpace(roleTeams[role])
}

func projectMembershipResponse(membership controldb.ProjectMembership, worker *controldb.AgentWorker) map[string]any {
	out := map[string]any{
		"id":               membership.ID,
		"workspaceId":      membership.WorkspaceID,
		"projectId":        membership.ProjectID,
		"memberType":       membership.MemberType,
		"memberId":         membership.MemberID,
		"role":             membership.Role,
		"title":            membership.Title,
		"prompt":           membership.Prompt,
		"permissions":      decodeJSONValue(membership.PermissionsJSON, []any{}),
		"autoPickTasks":    membership.AutoPickTasks,
		"attentionEnabled": membership.AttentionEnabled,
		"priorityWeight":   membership.PriorityWeight,
		"createdAt":        membership.CreatedAt,
		"updatedAt":        membership.UpdatedAt,
	}
	if worker != nil {
		out["agent"] = agentWorkerResponse(*worker)
	}
	return out
}

func normalizeAgentWorkerStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "active":
		return "active"
	case "paused":
		return "paused"
	case "archived":
		return "archived"
	default:
		return "active"
	}
}

func jsonStringOrDefault(v any, fallback string) string {
	if v == nil {
		return fallback
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return fallback
	}
	return string(raw)
}

func normalizeAgentWorkerScheduleJSON(v map[string]any) string {
	if len(v) == 0 {
		return "{}"
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	var hb entity.HeartbeatConfig
	if err := json.Unmarshal(raw, &hb); err != nil {
		return "{}"
	}
	hb.Triggers = normalizeHeartbeatTriggers(hb.Triggers)
	normalized, err := json.Marshal(hb)
	if err != nil {
		return "{}"
	}
	return string(normalized)
}

const defaultMaxForkSessions = 5

func normalizeAgentWorkerRuntimeConfigJSON(v map[string]any) string {
	normalized := normalizeAgentWorkerRuntimeConfig(v)
	raw, err := json.Marshal(normalized)
	if err != nil {
		return `{"maxForkSessions":5}`
	}
	return string(raw)
}

func normalizeAgentWorkerRuntimeConfig(v any) map[string]any {
	raw, ok := v.(map[string]any)
	if !ok || raw == nil {
		raw = map[string]any{}
	}
	out := make(map[string]any, len(raw)+1)
	for key, value := range raw {
		out[key] = value
	}
	max := intFromRuntimeConfigValue(out["maxForkSessions"])
	if max <= 0 {
		max = defaultMaxForkSessions
	}
	if max > 50 {
		max = 50
	}
	out["maxForkSessions"] = max
	return out
}

func intFromRuntimeConfigValue(v any) int {
	switch value := v.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		n, _ := value.Int64()
		return int(n)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(value))
		return n
	default:
		return 0
	}
}

func normalizeHeartbeatTriggers(values []entity.TriggerType) []entity.TriggerType {
	out := make([]entity.TriggerType, 0, len(values))
	seen := make(map[entity.TriggerType]bool, len(values))
	for _, value := range values {
		trigger := entity.TriggerType(strings.TrimSpace(string(value)))
		if !entity.IsValidTriggerType(trigger) || seen[trigger] {
			continue
		}
		seen[trigger] = true
		out = append(out, trigger)
	}
	return out
}

func decodeJSONValue(raw string, fallback any) any {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return fallback
	}
	return v
}

func normalizeStringList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
