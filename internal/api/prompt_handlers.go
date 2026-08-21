package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/multigent/multigent/internal/agentcli"
	"github.com/multigent/multigent/internal/ctxbuild"
	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/formatter"
	"github.com/multigent/multigent/internal/sandbox"
)

type promptResponse struct {
	Content string `json:"content"`
}

type promptSaveBody struct {
	Content     string  `json:"content"`
	DisplayName *string `json:"displayName,omitempty"`
}

// ── Agency prompt ─────────────────────────────────────────────────────────────

func (s *Server) handleGetAgencyPrompt(w http.ResponseWriter, _ *http.Request) {
	content, err := s.st.AgencyPrompt()
	if err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(promptResponse{Content: content})
}

func (s *Server) handlePutAgencyPrompt(w http.ResponseWriter, r *http.Request) {
	var body promptSaveBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.st.SaveAgencyPrompt(body.Content); err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// ── Team prompt ───────────────────────────────────────────────────────────────

func (s *Server) handleGetTeamPrompt(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.PathValue("teamPath"), "/")
	if path == "" {
		s.jsonError(w, http.StatusBadRequest, "missing team path")
		return
	}
	content, err := s.st.TeamPrompt(path)
	if err != nil {
		if isNotFoundErr(err) {
			_ = json.NewEncoder(w).Encode(promptResponse{Content: ""})
			return
		}
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(promptResponse{Content: content})
}

func (s *Server) handlePutTeamPrompt(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.PathValue("teamPath"), "/")
	if path == "" {
		s.jsonError(w, http.StatusBadRequest, "missing team path")
		return
	}
	var body promptSaveBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.st.SaveTeamPrompt(path, body.Content); err != nil {
		s.serverError(w, err)
		return
	}
	s.markPlaybookObjectCustomized(r, "team", "", path)
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// ── Role prompt ───────────────────────────────────────────────────────────────

func (s *Server) handleGetRolePrompt(w http.ResponseWriter, r *http.Request) {
	teamPath := strings.TrimSpace(r.URL.Query().Get("team"))
	roleName := strings.TrimSpace(r.URL.Query().Get("role"))
	if teamPath == "" || roleName == "" {
		s.jsonError(w, http.StatusBadRequest, "team and role query params are required")
		return
	}
	content, err := s.st.RolePrompt(teamPath, roleName)
	if err != nil {
		if isNotFoundErr(err) {
			_ = json.NewEncoder(w).Encode(promptResponse{Content: ""})
			return
		}
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(promptResponse{Content: content})
}

func (s *Server) handlePutRolePrompt(w http.ResponseWriter, r *http.Request) {
	teamPath := strings.TrimSpace(r.URL.Query().Get("team"))
	roleName := strings.TrimSpace(r.URL.Query().Get("role"))
	if teamPath == "" || roleName == "" {
		s.jsonError(w, http.StatusBadRequest, "team and role query params are required")
		return
	}
	var body promptSaveBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.st.SaveRolePrompt(teamPath, roleName, body.Content); err != nil {
		s.serverError(w, err)
		return
	}
	s.markPlaybookObjectCustomized(r, "role", teamPath, roleName)
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// ── Project prompt ────────────────────────────────────────────────────────────

func (s *Server) handleGetProjectPrompt(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !s.checkProjectAccess(w, r, name) {
		return
	}
	content, err := s.st.ProjectPrompt(name)
	if err != nil {
		if isNotFoundErr(err) {
			_ = json.NewEncoder(w).Encode(promptResponse{Content: ""})
			return
		}
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(promptResponse{Content: content})
}

func (s *Server) handlePutProjectPrompt(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !s.checkProjectManager(w, r, name) {
		return
	}
	if _, err := s.st.Project(name); err != nil {
		if isNotFoundErr(err) {
			s.jsonError(w, http.StatusNotFound, "project not found")
			return
		}
		s.serverError(w, err)
		return
	}
	var body promptSaveBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.st.SaveProjectPrompt(name, body.Content); err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// ── Agent context (merged, read-only) + wakeup ───────────────────────────────

func (s *Server) handleGetAgentContext(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("name")
	agent := r.PathValue("agent")
	if !s.checkAgentAccess(w, r, project, agent) {
		return
	}
	agentDir := s.st.AgentDir(project, agent)

	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}
	meta, err := s.agentMetaForProjectMember(workspaceID, project, agent)
	if err != nil {
		if isNotFoundErr(err) {
			s.jsonError(w, http.StatusNotFound, "agent not found")
			return
		}
		s.serverError(w, err)
		return
	}

	contextFile := contextFileName(string(meta.Model))
	includeMerged := true
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("includeMerged"))) {
	case "0", "false", "no":
		includeMerged = false
	}
	includeReadiness := true
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("includeReadiness"))) {
	case "0", "false", "no":
		includeReadiness = false
	}
	var merged []byte
	var mergedContext *ctxbuild.MergedContext
	if includeMerged {
		if content, mc, err := s.renderCurrentAgentContext(project, agent, meta, contextFile); err == nil {
			merged = content
			mergedContext = mc
		} else {
			mergedPath := filepath.Join(agentDir, contextFile)
			merged, _ = os.ReadFile(mergedPath)
		}
	}

	wakeup := s.readAgentWakeupPrompt(project, agent)

	var skills []string
	var skillDetails []map[string]string
	seen := map[string]bool{}
	addSkill := func(skillName string) {
		if skillName == "" || seen[skillName] {
			return
		}
		skills = append(skills, skillName)
		seen[skillName] = true
		detail := map[string]string{"name": skillName}
		if sk, err := s.st.Skill(skillName); err == nil && sk != nil {
			if strings.TrimSpace(sk.DisplayName) != "" {
				detail["displayName"] = strings.TrimSpace(sk.DisplayName)
			}
			if strings.TrimSpace(sk.Description) != "" {
				detail["description"] = strings.TrimSpace(sk.Description)
			}
		}
		skillDetails = append(skillDetails, detail)
	}
	if mergedContext != nil {
		for _, sk := range mergedContext.Skills {
			addSkill(sk.Name)
		}
	} else {
		for _, sk := range ctxbuild.DefaultSkillNames() {
			addSkill(sk)
		}
		if t, err := s.st.Team(meta.Team); err == nil && t != nil {
			for _, sk := range t.Skills {
				addSkill(sk)
			}
		}
		if meta.Role != "" {
			if rl, err := s.st.Role(meta.Team, meta.Role); err == nil && rl != nil {
				for _, sk := range rl.Skills {
					addSkill(sk)
				}
			}
		}
	}
	if skills == nil {
		skills = []string{}
	}

	addDirs := meta.AddDirs
	if addDirs == nil {
		addDirs = []string{}
	}
	resp := map[string]any{
		"contextFile":   contextFile,
		"context":       string(merged),
		"wakeup":        string(wakeup),
		"model":         string(meta.Model),
		"runtimeModel":  meta.RuntimeModel,
		"runtimeNodeId": meta.RuntimeNodeID,
		"team":          meta.Team,
		"role":          meta.Role,
		"avatar":        meta.Avatar,
		"syncedAt":      meta.SyncedAt,
		"skills":        skills,
		"skillDetails":  skillDetails,
		"workDir":       agentDir,
		"addDirs":       addDirs,
	}
	if meta.HTTPAgent != nil {
		resp["httpAgent"] = meta.HTTPAgent
	}
	if len(meta.Env) > 0 {
		resp["env"] = meta.Env
	}
	if meta.Provider != "" {
		resp["provider"] = meta.Provider
	}
	if meta.Sandbox != nil {
		sandboxCfg := *meta.Sandbox
		if sandboxCfg.AgentCLI == nil {
			sandboxCfg.AgentCLI = agentcli.DefaultForModel(meta.Model)
		} else {
			sandboxCfg.AgentCLI = agentcli.Normalize(sandboxCfg.AgentCLI)
		}
		resp["sandbox"] = &sandboxCfg
	}

	goalSummary := s.buildGoalSummary(project)
	if goalSummary != "" {
		resp["goals"] = goalSummary
	}

	readiness := buildRuntimeReadinessLight(meta)
	if includeReadiness {
		if workspaceID != "" {
			readiness = s.runtimeReadinessForExecution(workspaceID, meta)
		}
	}
	resp["setupChecks"] = readiness.Checks
	resp["runtimeReadiness"] = readiness

	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) renderCurrentAgentContext(project, agent string, meta *entity.AgentMeta, contextFile string) ([]byte, *ctxbuild.MergedContext, error) {
	if meta == nil {
		return nil, nil, os.ErrInvalid
	}
	team := strings.TrimSpace(meta.Team)
	role := strings.TrimSpace(meta.Role)
	if team == "" {
		role = ""
	}
	mc, err := ctxbuild.NewBuilder(s.st).BuildForAgent(project, agent, team, role)
	if err != nil {
		return nil, nil, err
	}
	f, err := formatter.New(meta.Model)
	if err != nil {
		return nil, nil, err
	}
	dir, err := os.MkdirTemp("", "multigent-agent-context-*")
	if err != nil {
		return nil, nil, err
	}
	defer os.RemoveAll(dir)
	if err := f.Format(mc, dir); err != nil {
		return nil, nil, err
	}
	content, err := os.ReadFile(filepath.Join(dir, contextFile))
	if err != nil {
		return nil, nil, err
	}
	return content, mc, nil
}

func (s *Server) readAgentWakeupPrompt(project, agent string) string {
	hb, err := s.ts.GetHeartbeat(project, agent)
	if err == nil && hb != nil {
		raw := strings.TrimSpace(hb.WakeupPrompt)
		if raw != "" && !strings.HasPrefix(raw, "@") {
			return hb.WakeupPrompt
		}
		if strings.HasPrefix(raw, "@") {
			rel := strings.TrimSpace(strings.TrimPrefix(raw, "@"))
			if rel != "" {
				path := rel
				if !filepath.IsAbs(path) {
					path = filepath.Join(s.st.AgentDir(project, agent), rel)
				}
				if body, err := os.ReadFile(path); err == nil {
					return string(body)
				}
			}
		}
	}
	wakeupPath := filepath.Join(s.st.AgentDir(project, agent), ".multigent", "context", "wakeup.md")
	wakeup, _ := os.ReadFile(wakeupPath)
	return string(wakeup)
}

func (s *Server) handleGetAgentRuntimeReadiness(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("name")
	agent := r.PathValue("agent")
	if !s.checkAgentAccess(w, r, project, agent) {
		return
	}
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}
	meta, err := s.agentMetaForProjectMember(workspaceID, project, agent)
	if err != nil {
		if isNotFoundErr(err) {
			s.jsonErrorCode(w, http.StatusNotFound, ErrCodeNotFound, "agent not found")
			return
		}
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(s.runtimeReadinessForExecution(workspaceID, meta))
}

func contextFileName(model string) string {
	s := strings.ToLower(model)
	switch {
	case strings.Contains(s, "claude"):
		return "CLAUDE.md"
	case strings.Contains(s, "codex"):
		return "AGENTS.md"
	case strings.Contains(s, "gemini"):
		return "GEMINI.md"
	case strings.Contains(s, "cursor"):
		return ".cursorrules"
	default:
		return "context.md"
	}
}

// ── Agent wakeup prompt (editable) ───────────────────────────────────────────

func (s *Server) handlePutAgentWakeup(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("name")
	agent := r.PathValue("agent")
	if !s.canManageAgentConfig(r, project, agent) {
		s.jsonError(w, http.StatusForbidden, "agent management access required")
		return
	}

	var body promptSaveBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	hb, _ := s.ts.GetHeartbeat(project, agent)
	if hb == nil {
		hb = &entity.HeartbeatConfig{}
	}
	hb.WakeupPrompt = body.Content
	if err := s.ts.SaveHeartbeat(project, agent, hb); err != nil {
		if isNotFoundErr(err) {
			s.jsonError(w, http.StatusNotFound, "agent not found")
			return
		}
		s.serverError(w, err)
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// ── Sync agent ────────────────────────────────────────────────────────────────

type syncBody struct {
	Agent string `json:"agent"`
}

func (s *Server) handlePostProjectSync(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("name")
	if _, err := s.st.Project(project); err != nil {
		if isNotFoundErr(err) {
			s.jsonError(w, http.StatusNotFound, "project not found")
			return
		}
		s.serverError(w, err)
		return
	}

	var body syncBody
	if r.ContentLength > 0 {
		if err := s.readJSON(w, r, &body); err != nil {
			s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}
	agentName := strings.TrimSpace(body.Agent)
	if agentName == "" {
		if !s.checkProjectManager(w, r, project) {
			return
		}
	} else if !s.canManageAgentConfig(r, project, agentName) {
		s.jsonError(w, http.StatusForbidden, "agent management access required")
		return
	}

	bin, err := exec.LookPath("multigent")
	if err != nil {
		bin, err = os.Executable()
		if err != nil {
			s.jsonError(w, http.StatusInternalServerError, "cannot find multigent binary")
			return
		}
	}

	args := []string{"sync", "--dir", s.root, "--project", project}
	if agentName != "" {
		args = append(args, "--name", agentName)
	}

	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		s.jsonError(w, http.StatusInternalServerError, "sync failed: "+string(out))
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":     true,
		"output": string(out),
	})
}

func (s *Server) handlePutAgentSandbox(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("name")
	agent := r.PathValue("agent")
	if !s.canManageAgentConfig(r, project, agent) {
		s.jsonError(w, http.StatusForbidden, "agent management access required")
		return
	}

	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}
	var worker controldb.AgentWorker
	workerBacked := false
	if s.agentDirectory != nil {
		resolved, found, err := s.agentDirectory.ProjectWorker(workspaceID, project, agent)
		if err != nil {
			s.serverError(w, err)
			return
		}
		if found {
			worker = resolved.Worker
			workerBacked = true
		}
	}
	meta, err := s.agentMetaForProjectMember(workspaceID, project, agent)
	if err != nil {
		if isNotFoundErr(err) {
			s.jsonError(w, http.StatusNotFound, "agent not found")
			return
		}
		s.serverError(w, err)
		return
	}

	var body struct {
		Provider   string                 `json:"provider"`
		Image      string                 `json:"image"`
		Template   string                 `json:"template"`
		Network    string                 `json:"network"`
		MemoryMB   int                    `json:"memoryMb"`
		CPUs       float64                `json:"cpus"`
		TimeoutSec int                    `json:"timeoutSec"`
		AgentCLI   *entity.AgentCLIConfig `json:"agentCli"`
		AddDirs    []string               `json:"addDirs"`
	}
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	provider := normalizeSandboxProvider(body.Provider)
	if provider == entity.SandboxNone {
		if strings.TrimSpace(meta.RuntimeNodeID) == "" && !directHostExecutionEnabled() {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "direct host execution is disabled by server configuration")
			return
		}
		meta.Sandbox = &entity.SandboxConfig{
			Provider: entity.SandboxNone,
			AgentCLI: body.AgentCLI,
			Resources: entity.RuntimeResourceLimits{
				MemoryMB:   body.MemoryMB,
				CPUs:       body.CPUs,
				TimeoutSec: body.TimeoutSec,
			},
		}
		if meta.Sandbox.AgentCLI == nil {
			meta.Sandbox.AgentCLI = agentcli.DefaultForModel(meta.Model)
		} else {
			meta.Sandbox.AgentCLI = agentcli.Normalize(meta.Sandbox.AgentCLI)
		}
	} else {
		meta.Sandbox = &entity.SandboxConfig{
			Provider:    provider,
			Image:       body.Image,
			NetworkMode: body.Network,
			AgentCLI:    body.AgentCLI,
			Resources: entity.RuntimeResourceLimits{
				MemoryMB:   body.MemoryMB,
				CPUs:       body.CPUs,
				TimeoutSec: body.TimeoutSec,
			},
		}
		if meta.Sandbox.AgentCLI == nil {
			meta.Sandbox.AgentCLI = agentcli.DefaultForModel(meta.Model)
		} else {
			meta.Sandbox.AgentCLI = agentcli.Normalize(meta.Sandbox.AgentCLI)
		}
		if provider == entity.SandboxDocker {
			dc := &entity.DockerSandboxConfig{
				Image:       body.Image,
				NetworkMode: body.Network,
				MemoryMB:    body.MemoryMB,
				CPUs:        body.CPUs,
			}
			meta.Sandbox.Docker = dc
		} else if provider == entity.SandboxE2B {
			caps := sandbox.DetectCapabilities()
			if !caps.E2B.Available {
				s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "e2b runtime unavailable: "+caps.E2B.Reason)
				return
			}
			meta.Sandbox.E2B = &entity.E2BSandboxConfig{
				Template:   body.Template,
				TimeoutSec: body.TimeoutSec,
			}
		} else {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "unsupported sandbox provider")
			return
		}
	}

	// Update add_dirs — always overwrite with whatever the client sent.
	// nil body.AddDirs (field absent) is treated as "no change"; empty slice clears all.
	if body.AddDirs != nil {
		meta.AddDirs = body.AddDirs
	}

	if workerBacked {
		cfg := decodeAgentWorkerRuntimeConfig(worker)
		cfg.Sandbox = meta.Sandbox
		if body.AddDirs != nil {
			cfg.AddDirs = body.AddDirs
		}
		worker.RuntimeConfigJSON = encodeAgentWorkerRuntimeConfig(cfg)
		worker.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if err := s.controlDB.UpsertAgentWorker(worker); err != nil {
			s.serverError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		return
	}

	if err := s.st.SaveAgentMeta(project, agent, meta); err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func normalizeSandboxProvider(provider string) entity.SandboxProvider {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", "none", "direct", "host", "local":
		return entity.SandboxNone
	case string(entity.SandboxDocker):
		return entity.SandboxDocker
	case string(entity.SandboxE2B):
		return entity.SandboxE2B
	default:
		return entity.SandboxProvider(strings.ToLower(strings.TrimSpace(provider)))
	}
}

func (s *Server) syncAgent(project, agent string) {
	bin, err := exec.LookPath("multigent")
	if err != nil {
		bin, _ = os.Executable()
	}
	if bin == "" {
		return
	}
	cmd := exec.Command(bin, "sync", "--dir", s.root, "--project", project, "--name", agent, "--force")
	out, err := cmd.CombinedOutput()
	if err != nil {
		slog.Warn("sync after wakeup save failed", "project", project, "agent", agent, "err", err, "output", string(out))
	}
}

type setupCheck struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Status   string `json:"status"` // ok, warning, error
	Detail   string `json:"detail,omitempty"`
	Action   string `json:"action,omitempty"`
	Blocking bool   `json:"blocking,omitempty"`
}

func buildSetupChecks(meta *entity.AgentMeta) []setupCheck {
	return buildRuntimeReadiness(meta).Checks
}

type runtimeReadinessResponse struct {
	Ready    bool         `json:"ready"`
	Blocking bool         `json:"blocking"`
	Summary  string       `json:"summary"`
	Checks   []setupCheck `json:"checks"`
}

func buildRuntimeReadiness(meta *entity.AgentMeta) runtimeReadinessResponse {
	return buildRuntimeReadinessWithOptions(meta, runtimeReadinessOptions{ProbeRuntime: true, CheckContainer: true})
}

func buildRuntimeReadinessLight(meta *entity.AgentMeta) runtimeReadinessResponse {
	return buildRuntimeReadinessWithOptions(meta, runtimeReadinessOptions{ProbeRuntime: false, CheckContainer: false})
}

type runtimeReadinessOptions struct {
	ProbeRuntime   bool
	CheckContainer bool
	DockerCache    *runtimeReadinessDockerCache
	AgentDir       string
}

type runtimeReadinessDockerCache struct {
	checked bool
	err     error
	images  map[string]bool
}

func (c *runtimeReadinessDockerCache) checkDocker() error {
	if c == nil {
		return sandbox.CheckDocker()
	}
	if !c.checked {
		c.err = sandbox.CheckDocker()
		c.checked = true
	}
	return c.err
}

func (c *runtimeReadinessDockerCache) imageAvailable(image string) bool {
	if c == nil {
		return sandbox.ImageAvailable(image)
	}
	if c.images == nil {
		c.images = map[string]bool{}
	}
	if available, ok := c.images[image]; ok {
		return available
	}
	available := sandbox.ImageAvailable(image)
	c.images[image] = available
	return available
}

func buildRuntimeReadinessWithOptions(meta *entity.AgentMeta, opts runtimeReadinessOptions) runtimeReadinessResponse {
	model := entity.NormaliseModel(meta.Model)
	if model == entity.ModelHuman || model == entity.ModelHTTPAgent {
		return runtimeReadinessResponse{Ready: true, Summary: "This member does not require a sandboxed CLI runtime.", Checks: nil}
	}

	var checks []setupCheck
	provider := entity.SandboxDocker
	if meta.Sandbox != nil {
		provider = meta.Sandbox.Provider
	} else if strings.TrimSpace(meta.RuntimeNodeID) != "" {
		provider = entity.SandboxNone
	}
	isDocker := provider == entity.SandboxDocker
	if provider == entity.SandboxNone {
		if strings.TrimSpace(meta.RuntimeNodeID) != "" {
			checks = append(checks, setupCheck{
				Key: "runtime_node", Label: "Runtime node", Status: "ok",
				Detail: "Direct host execution on the assigned trusted runtime node.",
			})
		} else if opts.ProbeRuntime {
			if !directHostExecutionEnabled() {
				checks = append(checks, setupCheck{
					Key:      "sandbox",
					Label:    "Direct host process",
					Status:   "error",
					Detail:   "Direct host execution is disabled by the server configuration.",
					Action:   "Switch this agent to Docker sandbox execution, or enable MULTIGENT_ALLOW_DIRECT_HOST on a trusted local install.",
					Blocking: true,
				})
			} else {
				checks = append(checks, setupCheck{
					Key: "sandbox", Label: "Direct host process", Status: "warning",
					Detail: "Runs on the local control-plane host without Docker isolation. Use only on a trusted machine.",
				})
			}
			if model == entity.ModelClaudeCode && os.Geteuid() == 0 {
				checks = append(checks, setupCheck{
					Key:    "direct_claudecode_root",
					Label:  "Claude Code direct execution",
					Status: "warning",
					Detail: "Multigent injects IS_SANDBOX=1 for Claude Code direct execution so non-interactive runs can work under root. This is a trusted-local mode, not a security sandbox.",
					Action: "For safer production use, switch this agent to Docker sandbox execution or run a non-root Runtime Node.",
				})
			}
		}
	} else if provider == entity.SandboxE2B && opts.ProbeRuntime {
		caps := sandbox.DetectCapabilities()
		if caps.E2B.Available {
			checks = append(checks, setupCheck{Key: "sandbox", Label: "E2B", Status: "ok"})
		} else {
			checks = append(checks, setupCheck{
				Key: "sandbox", Label: "E2B", Status: "error", Blocking: true,
				Detail: caps.E2B.Reason,
				Action: "Configure a reachable E2B-compatible sandbox provider, or switch this agent to Docker.",
			})
		}
	}

	// 1. CLI tool check
	cliName, installCmd := cliInfoForModel(model)
	if cliName != "" && opts.ProbeRuntime {
		if isDocker {
			image := sandbox.EffectiveImage(model, dockerSandboxConfig(meta))
			if opts.DockerCache.imageAvailable(image) {
				checks = append(checks, setupCheck{
					Key: "cli", Label: cliName + " CLI", Status: "warning",
					Detail: "Agent CLI toolchain is installed on first use unless prepared in advance. First run may spend several minutes installing it.",
					Action: "Run: multigent sandbox prepare --toolchain " + prepareToolchainName(model),
				})
			} else {
				checks = append(checks, setupCheck{
					Key: "cli", Label: cliName + " CLI", Status: "warning",
					Detail: "Agent CLI toolchain cannot be checked until the runtime image is available.",
					Action: "Run: multigent sandbox prepare --toolchain " + prepareToolchainName(model),
				})
			}
		} else if _, err := exec.LookPath(cliName); err == nil {
			checks = append(checks, setupCheck{
				Key: "cli", Label: cliName + " CLI", Status: "ok",
			})
		} else {
			checks = append(checks, setupCheck{
				Key: "cli", Label: cliName + " CLI", Status: "error", Blocking: true,
				Detail: "未安装。请运行: " + installCmd,
			})
		}
	}

	// 2. Docker check (if using docker sandbox)
	if isDocker && opts.ProbeRuntime {
		if err := opts.DockerCache.checkDocker(); err == nil {
			image := sandbox.EffectiveImage(model, dockerSandboxConfig(meta))
			checks = append(checks, setupCheck{
				Key: "docker", Label: "Docker", Status: "ok",
				Detail: "Docker CLI: " + sandbox.DockerExecutable(),
			})
			if opts.DockerCache.imageAvailable(image) {
				checks = append(checks, setupCheck{
					Key: "runtime_image", Label: "Runtime image", Status: "ok",
					Detail: image,
				})
				if opts.CheckContainer {
					if err := sandbox.RuntimeContainerAvailable(image, 4*time.Second); err != nil {
						checks = append(checks, setupCheck{
							Key: "runtime_container", Label: "Runtime container", Status: "error", Blocking: true,
							Detail: err.Error(),
							Action: "Restart Docker Desktop / Docker Engine, then run: multigent sandbox prepare",
						})
					} else {
						checks = append(checks, setupCheck{
							Key: "runtime_container", Label: "Runtime container", Status: "ok",
							Detail: "Container startup check passed.",
						})
						if apiURL := strings.TrimSpace(os.Getenv("MULTIGENT_API_URL")); apiURL != "" {
							if err := sandbox.RuntimeAPIReachableFromContainer(image, apiURL, 4*time.Second); err != nil {
								checks = append(checks, setupCheck{
									Key:      "runtime_api",
									Label:    "Runtime API",
									Status:   "error",
									Blocking: true,
									Detail:   err.Error(),
									Action:   "Start Multigent with --addr 0.0.0.0:<port>, or set MULTIGENT_API_URL to an address reachable from Docker.",
								})
							} else {
								checks = append(checks, setupCheck{
									Key:    "runtime_api",
									Label:  "Runtime API",
									Status: "ok",
									Detail: "Container callback check passed.",
								})
							}
						}
					}
				}
			} else {
				checks = append(checks, setupCheck{
					Key: "runtime_image", Label: "Runtime image", Status: "error", Blocking: true,
					Detail: "Runtime image is not available locally: " + image,
					Action: "Run: multigent sandbox prepare",
				})
			}
		} else {
			checks = append(checks, setupCheck{
				Key: "docker", Label: "Docker", Status: "error", Blocking: true,
				Detail: err.Error(),
				Action: "Install and start Docker Desktop / Docker Engine.",
			})
		}
	}

	// 3. Auth / credential check
	if opts.ProbeRuntime {
		authCheck := checkAuthForModel(model, isDocker, opts.AgentDir, strings.TrimSpace(meta.Provider) != "")
		if authCheck != nil {
			checks = append(checks, *authCheck)
		}
	}

	// 4. Model account check. Multigent is a control-plane product: runnable
	// agents must bind an explicit model account instead of inheriting host env.
	if strings.TrimSpace(meta.Provider) != "" {
		checks = append(checks, setupCheck{
			Key: "provider", Label: "模型账号", Status: "ok",
			Detail: meta.Provider,
		})
	} else if modelRequiresModelAccount(model) {
		checks = append(checks, setupCheck{
			Key: "provider", Label: "模型账号", Status: "error", Blocking: true,
			Detail: "未配置模型账号。请先在设置页添加模型账号，并在 Agent 详情页关联。",
		})
	}

	blocking := false
	warnings := 0
	for _, check := range checks {
		if check.Blocking || check.Status == "error" {
			blocking = true
		}
		if check.Status == "warning" {
			warnings++
		}
	}
	summary := "Runtime is ready."
	if blocking {
		summary = "Runtime is not ready. Resolve blocking checks before running this agent."
	} else if warnings > 0 {
		summary = "Runtime can run, but preparation is incomplete. First run may be slower."
	}
	return runtimeReadinessResponse{
		Ready:    !blocking,
		Blocking: blocking,
		Summary:  summary,
		Checks:   checks,
	}
}

func modelRequiresModelAccount(model entity.AgentModel) bool {
	switch entity.NormaliseModel(model) {
	case entity.ModelHuman, entity.ModelHTTPAgent:
		return false
	default:
		return true
	}
}

func directHostExecutionEnabled() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("MULTIGENT_ALLOW_DIRECT_HOST")))
	if value == "" {
		value = strings.TrimSpace(strings.ToLower(os.Getenv("MULTIGENT_ALLOW_DIRECT_HOST_EXECUTION")))
	}
	switch value {
	case "", "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return true
	}
}

func runtimeReadinessErrorMessage(readiness runtimeReadinessResponse) string {
	var parts []string
	for _, check := range readiness.Checks {
		if !check.Blocking && check.Status != "error" {
			continue
		}
		msg := check.Label
		if check.Detail != "" {
			msg += ": " + check.Detail
		}
		if check.Action != "" {
			msg += " " + check.Action
		}
		parts = append(parts, msg)
	}
	if len(parts) == 0 {
		return "runtime is not ready"
	}
	return strings.Join(parts, "\n")
}

func dockerSandboxConfig(meta *entity.AgentMeta) *entity.DockerSandboxConfig {
	if meta == nil || meta.Sandbox == nil {
		return nil
	}
	return meta.Sandbox.Docker
}

func prepareToolchainName(model entity.AgentModel) string {
	switch entity.NormaliseModel(model) {
	case entity.ModelClaudeCode:
		return "claudecode"
	case entity.ModelCodex, entity.ModelQoder:
		return "codex"
	case entity.ModelGemini:
		return "gemini"
	default:
		return string(entity.NormaliseModel(model))
	}
}

func cliInfoForModel(model entity.AgentModel) (name, install string) {
	switch model {
	case entity.ModelClaudeCode:
		return "claude", "npm install -g @anthropic-ai/claude-code"
	case entity.ModelCodex:
		return "codex", "npm install -g @openai/codex"
	case entity.ModelCursor:
		return "agent", "curl -fsSL https://www.cursor.com/install-agent.sh | sh"
	case entity.ModelGemini:
		return "gemini", "npm install -g @anthropic-ai/gemini-cli"
	case entity.ModelQoder:
		return "qoder", "npm install -g @anthropic-ai/qoder"
	case entity.ModelOpenCode:
		return "opencode", "go install github.com/opencode-ai/opencode@latest"
	}
	return "", ""
}

func checkAuthForModel(model entity.AgentModel, isDocker bool, agentDir string, hasProvider bool) *setupCheck {
	home, _ := os.UserHomeDir()
	switch model {
	case entity.ModelCursor:
		if agentRuntimeCredentialExists(agentDir, model, filepath.Join(".config", "cursor", "auth.json")) ||
			agentRuntimeCredentialExists(agentDir, model, filepath.Join(".cursor", "cli-config.json")) {
			return &setupCheck{Key: "auth", Label: "Cursor 认证", Status: "ok"}
		}
		if hasProvider {
			return nil
		}
		authFile := filepath.Join(home, ".config", "cursor", "auth.json")
		if _, err := os.Stat(authFile); err == nil {
			return &setupCheck{Key: "auth", Label: "Cursor 认证", Status: "ok"}
		}
		return &setupCheck{
			Key: "auth", Label: "Cursor 认证", Status: "error",
			Detail: "未登录。请运行: agent login",
		}
	case entity.ModelClaudeCode:
		if agentRuntimeCredentialExists(agentDir, model, ".claude.json") ||
			agentRuntimeCredentialExists(agentDir, model, filepath.Join(".claude", ".credentials.json")) {
			return &setupCheck{Key: "auth", Label: "Claude 认证", Status: "ok"}
		}
		if hasProvider {
			return nil
		}
		claudeJSON := filepath.Join(home, ".claude.json")
		if _, err := os.Stat(claudeJSON); err == nil {
			return &setupCheck{Key: "auth", Label: "Claude 认证", Status: "ok"}
		}
		return &setupCheck{
			Key: "auth", Label: "Claude 认证", Status: "warning",
			Detail: "~/.claude.json 不存在（如果使用 API Key 可忽略）",
		}
	}
	return nil
}

func agentRuntimeCredentialExists(agentDir string, model entity.AgentModel, rel string) bool {
	if strings.TrimSpace(agentDir) == "" || strings.TrimSpace(rel) == "" {
		return false
	}
	path := filepath.Join(agentDir, ".multigent", "runtime-home", string(entity.NormaliseModel(model)), rel)
	if _, err := os.Stat(path); err == nil {
		return true
	}
	return false
}
