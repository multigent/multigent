package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/multigent/multigent/internal/agentcli"
	"github.com/multigent/multigent/internal/avatar"
	"github.com/multigent/multigent/internal/ctxbuild"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/formatter"
	"github.com/multigent/multigent/internal/rbac"
	"github.com/multigent/multigent/internal/sandbox"
)

// ── Create Role ──────────────────────────────────────────────────────────────

type createRoleBody struct {
	Team        string   `json:"team"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Skills      []string `json:"skills"`
	SetupDirs   []string `json:"setupDirs"`
}

func (s *Server) handleCreateRole(w http.ResponseWriter, r *http.Request) {
	var body createRoleBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	team := strings.TrimSpace(body.Team)
	name := strings.TrimSpace(body.Name)
	if team == "" || name == "" {
		s.jsonError(w, http.StatusBadRequest, "team and name are required")
		return
	}

	if _, err := s.st.Team(team); err != nil {
		s.jsonError(w, http.StatusNotFound, fmt.Sprintf("team %q not found", team))
		return
	}

	roleDir := s.st.RoleDir(team, name)
	if _, err := os.Stat(roleDir); err == nil {
		s.jsonError(w, http.StatusConflict, fmt.Sprintf("role %q already exists under team %q", name, team))
		return
	}

	role := &entity.Role{
		Name:        name,
		Description: strings.TrimSpace(body.Description),
		Skills:      body.Skills,
		Setup: entity.RoleSetup{
			Dirs: body.SetupDirs,
		},
	}
	if err := s.st.SaveRole(team, name, role); err != nil {
		s.serverError(w, err)
		return
	}

	stub := fmt.Sprintf("# Role: %s\n\n", name)
	if body.Description != "" {
		stub += strings.TrimSpace(body.Description) + "\n\n"
	}
	stub += "<!-- Describe this role's responsibilities, working style, and expectations. -->\n"
	_ = s.st.SaveRolePrompt(team, name, stub)

	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":   true,
		"name": name,
		"team": team,
	})
}

// ── Hire Agent ───────────────────────────────────────────────────────────────

type hireAgentBody struct {
	Name  string `json:"name"`
	Team  string `json:"team"`
	Role  string `json:"role"`
	Model string `json:"model"`
}

func (s *Server) handleHireAgent(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("name")
	if project == "" {
		s.jsonError(w, http.StatusBadRequest, "missing project name")
		return
	}
	if !s.checkProjectManager(w, r, project) {
		return
	}

	var body hireAgentBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	agentName := strings.TrimSpace(body.Name)
	team := strings.TrimSpace(body.Team)
	model := strings.TrimSpace(body.Model)
	role := strings.TrimSpace(body.Role)

	if agentName == "" || team == "" || model == "" {
		s.jsonError(w, http.StatusBadRequest, "name, team, and model are required")
		return
	}
	if entity.AgentModel(model) != entity.ModelHuman {
		if !s.checkAgentEntitlement(w, r, 1) {
			return
		}
	}
	if _, err := s.st.AgentMeta(project, agentName); err == nil {
		s.jsonErrorCode(w, http.StatusConflict, ErrCodeAgentAlreadyExists, fmt.Sprintf("agent %q already exists", agentName))
		return
	} else if !isNotFoundErr(err) {
		s.serverError(w, err)
		return
	}

	out, err := s.hireAgent(project, agentName, team, role, model)
	if err != nil {
		s.writeHireAgentError(w, err)
		return
	}

	// Auto-link user account when hiring a human
	if entity.AgentModel(model) == entity.ModelHuman {
		if u := s.users.GetUser(agentName); u != nil {
			hasGrant := false
			for _, grant := range u.AgentGrants {
				if grant.Project == project && grant.Agent == agentName {
					hasGrant = true
					break
				}
			}
			if !hasGrant {
				newGrants := append(u.AgentGrants, agentAccess{Project: project, Agent: agentName, Role: string(rbac.AgentRoleOperator)})
				hasProj := false
				for _, pa := range u.Projects {
					if pa.Project == project {
						hasProj = true
						break
					}
				}
				var newProjects []projectAccess
				if hasProj {
					newProjects = u.Projects
				} else {
					newProjects = append(u.Projects, projectAccess{Project: project, Role: ProjectRoleOperator})
				}
				_ = s.users.UpdateUser(agentName, nil, nil, nil, nil, nil, nil, nil, newProjects, newGrants, nil)
			}
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":     true,
		"output": out,
		"agent":  agentName,
	})
}

type hireAgentError struct {
	status  int
	code    string
	message string
	err     error
}

func (e hireAgentError) Error() string {
	if e.err != nil {
		return e.err.Error()
	}
	return e.message
}

func (s *Server) writeHireAgentError(w http.ResponseWriter, err error) {
	if he, ok := err.(hireAgentError); ok {
		s.jsonErrorCode(w, he.status, he.code, he.message)
		return
	}
	s.jsonErrorCode(w, http.StatusInternalServerError, ErrCodeInternal, "hire failed")
}

func (s *Server) hireAgent(project, agentName, team, role, model string) (string, error) {
	agentModel := entity.NormaliseModel(entity.AgentModel(model))
	if !entity.IsValidModel(agentModel) {
		return "", hireAgentError{
			status:  http.StatusBadRequest,
			code:    ErrCodeValidationFailed,
			message: fmt.Sprintf("unknown agent CLI %q", model),
		}
	}
	if _, err := s.st.Project(project); err != nil {
		if isNotFoundErr(err) {
			return "", hireAgentError{status: http.StatusNotFound, code: ErrCodeProjectNotFound, message: "project not found", err: err}
		}
		return "", err
	}
	if team != "" {
		if _, err := s.st.Team(team); err != nil {
			if isNotFoundErr(err) {
				return "", hireAgentError{status: http.StatusNotFound, code: ErrCodeTeamNotFound, message: "team not found", err: err}
			}
			return "", err
		}
	}
	if role != "" {
		if _, err := s.st.Role(team, role); err != nil {
			if isNotFoundErr(err) {
				return "", hireAgentError{status: http.StatusBadRequest, code: ErrCodeValidationFailed, message: "role not found", err: err}
			}
			return "", err
		}
	}

	agentDir := s.st.AgentDir(project, agentName)
	if err := os.RemoveAll(agentDir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		return "", err
	}

	now := time.Now().UTC()
	meta := &entity.AgentMeta{
		Name:    agentName,
		Project: project,
		Team:    team,
		Role:    role,
		Model:   agentModel,
		HiredAt: now,
		Avatar:  avatar.RandomURL(project, agentName),
	}

	if agentModel != entity.ModelHuman {
		builder := ctxbuild.NewBuilder(s.st)
		mc, err := builder.BuildForAgent(project, agentName, team, role)
		if err != nil {
			return "", hireAgentError{status: http.StatusBadRequest, code: ErrCodeValidationFailed, message: "agent context is invalid", err: err}
		}
		f, err := formatter.New(agentModel)
		if err != nil {
			return "", hireAgentError{status: http.StatusBadRequest, code: ErrCodeValidationFailed, message: "unsupported agent CLI", err: err}
		}
		if err := f.Format(mc, agentDir); err != nil {
			return "", err
		}
		meta.ContextHash = ctxbuild.LayerHashes(mc)
		meta.Sandbox = defaultAPISandboxConfig(agentModel)
		if role != "" {
			roleMeta, err := s.st.Role(team, role)
			if err == nil {
				if err := applyAPIRoleSetup(roleMeta.Setup, agentDir); err != nil {
					return "", err
				}
			}
		}
		if projMeta, err := s.st.Project(project); err == nil && strings.TrimSpace(projMeta.Repo) != "" {
			repoAbs := strings.TrimSpace(projMeta.Repo)
			if !filepath.IsAbs(repoAbs) {
				repoAbs = filepath.Join(s.root, repoAbs)
			}
			meta.AddDirs = []string{repoAbs}
		}
	}

	if err := s.st.SaveAgentMeta(project, agentName, meta); err != nil {
		return "", err
	}
	return fmt.Sprintf("Agent %s/%s added", project, agentName), nil
}

func defaultAPISandboxConfig(agentModel entity.AgentModel) *entity.SandboxConfig {
	agentModel = entity.NormaliseModel(agentModel)
	if agentModel == entity.ModelHuman || agentModel == entity.ModelHTTPAgent {
		return nil
	}
	image := sandbox.ImageForModel(agentModel)
	return &entity.SandboxConfig{
		Provider: entity.SandboxDocker,
		Image:    image,
		AgentCLI: agentcli.DefaultForModel(agentModel),
		Docker: &entity.DockerSandboxConfig{
			Image:       image,
			NetworkMode: "bridge",
		},
	}
}

func applyAPIRoleSetup(setup entity.RoleSetup, agentDir string) error {
	multigentDir := filepath.Join(agentDir, ".multigent")
	for _, dir := range setup.Dirs {
		full := filepath.Join(multigentDir, dir)
		if err := os.MkdirAll(full, 0o755); err != nil {
			return fmt.Errorf("create dir %q: %w", dir, err)
		}
	}
	for _, file := range setup.Files {
		full := filepath.Join(multigentDir, file.Path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return fmt.Errorf("create parent for %q: %w", file.Path, err)
		}
		if _, err := os.Stat(full); os.IsNotExist(err) {
			if err := os.WriteFile(full, []byte(file.Content), 0o644); err != nil {
				return fmt.Errorf("create file %q: %w", file.Path, err)
			}
		}
	}
	return nil
}

// ── Run Agent ────────────────────────────────────────────────────────────────

type runAgentBody struct {
	Project string `json:"project"`
	Agent   string `json:"agent"`
	TaskID  string `json:"taskId"`
	Prompt  string `json:"prompt"`
	Title   string `json:"title"`
}

func (s *Server) handleRunAgent(w http.ResponseWriter, r *http.Request) {
	var body runAgentBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	project := strings.TrimSpace(body.Project)
	agent := strings.TrimSpace(body.Agent)
	if project == "" || agent == "" {
		s.jsonError(w, http.StatusBadRequest, "project and agent are required")
		return
	}
	if !s.canOperateAgent(r, project, agent) {
		s.jsonError(w, http.StatusForbidden, "agent operator access required")
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
	if readiness := buildRuntimeReadiness(meta); readiness.Blocking {
		s.jsonErrorCode(w, http.StatusConflict, ErrCodeRuntimeNotReady, runtimeReadinessErrorMessage(readiness))
		return
	}

	var allOutput strings.Builder

	// If prompt is provided, create a task first
	if prompt := strings.TrimSpace(body.Prompt); prompt != "" {
		title := strings.TrimSpace(body.Title)
		if title == "" {
			runes := []rune(prompt)
			if len(runes) > 40 {
				title = string(runes[:40]) + "…"
			} else {
				title = prompt
			}
		}
		addArgs := []string{
			"--dir", s.root, "task", "add",
			"--project", project, "--agent", agent,
			"--title", title, "--prompt", prompt,
			"--priority", "0",
		}
		addCmd := exec.Command(s.sched.binPath, addArgs...)
		addOut, err := addCmd.CombinedOutput()
		allOutput.WriteString(string(addOut))
		if err != nil {
			s.jsonError(w, http.StatusInternalServerError, fmt.Sprintf("task creation failed: %v\n%s", err, string(addOut)))
			return
		}
		allOutput.WriteString("\n")
	}

	args := []string{"--dir", s.root, "run", "--project", project, "--agent", agent}
	if body.TaskID != "" {
		args = append(args, "--task", strings.TrimSpace(body.TaskID))
	}

	cmd := exec.Command(s.sched.binPath, args...)
	out, err := cmd.CombinedOutput()
	allOutput.WriteString(string(out))
	if err != nil {
		s.jsonError(w, http.StatusInternalServerError, fmt.Sprintf("run failed: %v\n%s", err, allOutput.String()))
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "output": allOutput.String()})
}

// ── Set Model ────────────────────────────────────────────────────────────────

type setModelBody struct {
	Model       string `json:"model"`
	HttpURL     string `json:"httpUrl,omitempty"`
	HttpModel   string `json:"httpModel,omitempty"`
	HttpAPIKey  string `json:"httpApiKey,omitempty"`
	HttpTimeout string `json:"httpTimeout,omitempty"`
	HttpStream  *bool  `json:"httpStream,omitempty"`
}

func (s *Server) handleSetModel(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("name")
	agent := r.PathValue("agent")
	if project == "" || agent == "" {
		s.jsonError(w, http.StatusBadRequest, "missing project or agent")
		return
	}
	if !s.canManageAgentConfig(r, project, agent) {
		s.jsonError(w, http.StatusForbidden, "agent management access required")
		return
	}

	var body setModelBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	model := strings.TrimSpace(body.Model)
	if model == "" {
		s.jsonError(w, http.StatusBadRequest, "model is required")
		return
	}

	args := []string{
		"--dir", s.root,
		"agent", "set-model",
		"--project", project,
		"--name", agent,
		"--model", model,
	}

	if model == "http-agent" {
		if u := strings.TrimSpace(body.HttpURL); u != "" {
			args = append(args, "--http-url", u)
		}
		if m := strings.TrimSpace(body.HttpModel); m != "" {
			args = append(args, "--http-model", m)
		}
		if k := strings.TrimSpace(body.HttpAPIKey); k != "" {
			args = append(args, "--http-api-key", k)
		}
		if t := strings.TrimSpace(body.HttpTimeout); t != "" {
			args = append(args, "--http-timeout", t)
		}
		if body.HttpStream != nil {
			if *body.HttpStream {
				args = append(args, "--http-stream")
			} else {
				args = append(args, "--http-stream=false")
			}
		}
	}

	cmd := exec.Command(s.sched.binPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		s.jsonError(w, http.StatusInternalServerError, fmt.Sprintf("set-model failed: %v\n%s", err, string(out)))
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok": true,
	})
}

// ── Session Reset ────────────────────────────────────────────────────────────

type sessionResetBody struct {
	Project string `json:"project"`
	Agent   string `json:"agent"`
}

func (s *Server) handleSessionReset(w http.ResponseWriter, r *http.Request) {
	var body sessionResetBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	project := strings.TrimSpace(body.Project)
	agent := strings.TrimSpace(body.Agent)
	if project == "" || agent == "" {
		s.jsonError(w, http.StatusBadRequest, "project and agent are required")
		return
	}
	if !s.canManageAgentConfig(r, project, agent) {
		s.jsonError(w, http.StatusForbidden, "agent management access required")
		return
	}

	hb, err := s.ts.GetHeartbeat(project, agent)
	if err != nil {
		s.serverError(w, err)
		return
	}
	oldID := hb.SessionID
	hb.SessionID = ""
	hb.SessionStartedAt = nil
	if err := s.ts.SaveHeartbeat(project, agent, hb); err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "oldSessionId": oldID})
}

// ── Agent Environment Variables ──────────────────────────────────────────────

type agentEnvBody struct {
	Env          map[string]string `json:"env"`
	Provider     *string           `json:"provider,omitempty"`
	RuntimeModel *string           `json:"runtimeModel,omitempty"`
}

func (s *Server) handlePutAgentEnv(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("name")
	agent := r.PathValue("agent")
	if !s.canManageAgentConfig(r, project, agent) {
		s.jsonError(w, http.StatusForbidden, "agent management access required")
		return
	}

	var body agentEnvBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	meta, err := s.st.AgentMeta(project, agent)
	if err != nil {
		if isNotFoundErr(err) {
			s.jsonError(w, http.StatusNotFound, "agent not found")
			return
		}
		s.serverError(w, err)
		return
	}

	// Remove empty-value entries
	cleaned := make(map[string]string)
	for k, v := range body.Env {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k != "" && v != "" {
			cleaned[k] = v
		}
	}
	if len(cleaned) == 0 {
		meta.Env = nil
	} else {
		meta.Env = cleaned
	}
	if body.Provider != nil {
		providerID := strings.TrimSpace(*body.Provider)
		if providerID == "none" {
			providerID = ""
		}
		if providerID != "" {
			provider, err := s.providerStore().Get(providerID)
			if err != nil {
				if strings.Contains(err.Error(), "not found") {
					s.jsonError(w, http.StatusBadRequest, "provider not found")
					return
				}
				s.serverError(w, err)
				return
			}
			if !s.canUseModelProviderForAgent(r, *provider, project, agent) {
				s.jsonError(w, http.StatusForbidden, "model provider is not available for this agent")
				return
			}
		}
		meta.Provider = providerID
	}
	if body.RuntimeModel != nil {
		meta.RuntimeModel = strings.TrimSpace(*body.RuntimeModel)
	}

	if err := s.st.SaveAgentMeta(project, agent, meta); err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		Action:       "agent.env.update",
		ResourceType: "agent",
		ResourceID:   project + "/" + agent,
		Summary:      "Agent environment updated",
		After: map[string]any{
			"project":      project,
			"agent":        agent,
			"provider":     meta.Provider,
			"runtimeModel": meta.RuntimeModel,
			"envKeys":      sortedEnvKeys(meta.Env),
		},
		Request: r,
	})
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "env": meta.Env, "provider": meta.Provider, "runtimeModel": meta.RuntimeModel})
}

func (s *Server) canManageAgentConfig(r *http.Request, project, agent string) bool {
	if s.canManageProject(r, project) {
		return true
	}
	role, ok := currentUserAgentRole(s.currentUser(r), project, agent)
	return ok && agentRoleLevel(role) >= agentRoleLevel(string(rbac.AgentRoleOwner))
}

func (s *Server) canOperateAgent(r *http.Request, project, agent string) bool {
	if s.canOperateProject(r, project) {
		return true
	}
	role, ok := currentUserAgentRole(s.currentUser(r), project, agent)
	return ok && agentRoleLevel(role) >= agentRoleLevel(string(rbac.AgentRoleOperator))
}

func (s *Server) canUseModelProviderForAgent(r *http.Request, provider entity.APIProvider, project, agent string) bool {
	meta, err := s.st.AgentMeta(project, agent)
	if err != nil || meta == nil || !modelProviderMatchesAgentModel(provider, meta.Model) {
		return false
	}
	switch provider.OwnerType {
	case "", ConnectionOwnerWorkspace:
		return s.canManageAgentConfig(r, project, agent)
	default:
		return false
	}
}

func modelProviderMatchesAgentModel(provider entity.APIProvider, model entity.AgentModel) bool {
	if strings.TrimSpace(string(model)) == "" {
		return true
	}
	want := providerTypesForAgentModel(model)
	if len(want) == 0 {
		return false
	}
	providerType := strings.ToLower(strings.TrimSpace(provider.Type))
	for _, typ := range want {
		if providerType == typ {
			return true
		}
	}
	return false
}

func providerTypesForAgentModel(model entity.AgentModel) []string {
	switch entity.NormaliseModel(model) {
	case entity.ModelClaudeCode:
		return []string{"anthropic"}
	case entity.ModelCodex, entity.ModelOpenCode, entity.ModelHTTPAgent:
		return []string{"openai"}
	case entity.ModelCursor:
		return []string{"cursor"}
	case entity.ModelGemini:
		return []string{"gemini"}
	default:
		return nil
	}
}

func sortedEnvKeys(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
