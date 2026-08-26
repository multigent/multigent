package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	workflowstore "github.com/multigent/multigent/internal/workflow"
)

type schedulerProcess struct {
	mu        sync.Mutex
	cmd       *exec.Cmd
	cancel    context.CancelFunc
	mode      string
	project   string
	agent     string
	startedAt time.Time
	stopped   bool
	exitErr   error
	doneCh    chan struct{}
}

const (
	schedulerModeLocal       = "local"
	schedulerModeRuntimeNode = "runtime-node"
	schedulerModeWakeup      = "wakeup"
	schedulerModeManualTask  = "manual-task"
)

type SchedulerManager struct {
	mu      sync.Mutex
	root    string
	binPath string
	procs   map[string]*schedulerProcess // key = "all", "project", "project/agent", or "worker/<id>"
}

func newSchedulerManager(root string) *SchedulerManager {
	bin, _ := os.Executable()
	return &SchedulerManager{
		root:    root,
		binPath: bin,
		procs:   make(map[string]*schedulerProcess),
	}
}

func schedKey(project, agent string) string {
	if project == "" {
		return "all"
	}
	if agent == "" {
		return project
	}
	return project + "/" + agent
}

func (m *SchedulerManager) Start(project, agent string) error {
	key := schedKey(project, agent)
	args := []string{"--dir", m.root, "scheduler", "start"}
	if project != "" {
		args = append(args, "--project", project)
	}
	if agent != "" {
		args = append(args, "--agent", agent)
	}

	cmd := exec.Command(m.binPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	setProcGroup(cmd)

	_, err := m.StartManagedCommand(key, project, agent, schedulerModeLocal, cmd)
	return err
}

func (m *SchedulerManager) StartManagedCommand(key, project, agent, mode string, cmd *exec.Cmd) (int, error) {
	if cmd == nil {
		return 0, fmt.Errorf("command is nil")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		key = schedKey(project, agent)
	}
	if strings.TrimSpace(mode) == "" {
		mode = schedulerModeLocal
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.procs[key]; ok {
		select {
		case <-p.doneCh:
			// process already exited, allow restart
		default:
			return 0, fmt.Errorf("scheduler already running for %q", key)
		}
	}
	logFile := m.attachManagedCommandLog(key, cmd)
	if err := cmd.Start(); err != nil {
		if logFile != nil {
			_ = logFile.Close()
		}
		return 0, fmt.Errorf("start managed command: %w", err)
	}

	proc := &schedulerProcess{
		cmd:       cmd,
		mode:      mode,
		project:   project,
		agent:     agent,
		startedAt: time.Now(),
		doneCh:    make(chan struct{}),
	}

	go func() {
		err := cmd.Wait()
		if logFile != nil {
			_ = logFile.Close()
		}
		proc.mu.Lock()
		proc.exitErr = err
		proc.stopped = true
		proc.mu.Unlock()
		close(proc.doneCh)
	}()

	m.procs[key] = proc
	pid := 0
	if cmd.Process != nil {
		pid = cmd.Process.Pid
	}
	return pid, nil
}

func (m *SchedulerManager) attachManagedCommandLog(key string, cmd *exec.Cmd) *os.File {
	if cmd == nil {
		return nil
	}
	cmd.Stdin = nil
	dir := filepath.Join(m.root, ".multigent", "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("warning: failed to create managed command log dir: %v", err)
		return nil
	}
	name := strings.TrimSpace(key)
	if name == "" {
		name = "command"
	}
	name = strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-").Replace(name)
	path := filepath.Join(dir, "managed-"+name+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Printf("warning: failed to open managed command log %s: %v", path, err)
		return nil
	}
	fmt.Fprintf(f, "\n--- %s start %s %s ---\n", time.Now().UTC().Format(time.RFC3339), cmd.Path, strings.Join(cmd.Args[1:], " "))
	cmd.Stdout = f
	cmd.Stderr = f
	return f
}

func (m *SchedulerManager) StartLoop(project, agent, mode string, loop func(context.Context)) error {
	return m.StartLoopWithKey(schedKey(project, agent), project, agent, mode, loop)
}

func (m *SchedulerManager) StartLoopWithKey(key, project, agent, mode string, loop func(context.Context)) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key = strings.TrimSpace(key)
	if key == "" {
		key = schedKey(project, agent)
	}
	if p, ok := m.procs[key]; ok {
		select {
		case <-p.doneCh:
			// loop already exited, allow restart
		default:
			return fmt.Errorf("scheduler already running for %q", key)
		}
	}
	if strings.TrimSpace(mode) == "" {
		mode = schedulerModeRuntimeNode
	}
	ctx, cancel := context.WithCancel(context.Background())
	proc := &schedulerProcess{
		cancel:    cancel,
		mode:      mode,
		project:   project,
		agent:     agent,
		startedAt: time.Now(),
		doneCh:    make(chan struct{}),
	}
	go func() {
		defer close(proc.doneCh)
		defer cancel()
		loop(ctx)
		proc.mu.Lock()
		proc.stopped = true
		proc.mu.Unlock()
	}()
	m.procs[key] = proc
	return nil
}

func (m *SchedulerManager) Stop(project, agent string) error {
	return m.StopKey(schedKey(project, agent))
}

func (m *SchedulerManager) StopKey(key string) error {
	m.mu.Lock()
	key = strings.TrimSpace(key)
	proc, ok := m.procs[key]
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("no scheduler running for %q", key)
	}

	select {
	case <-proc.doneCh:
		return fmt.Errorf("scheduler for %q already stopped", key)
	default:
	}

	if proc.cancel != nil {
		proc.cancel()
	} else if proc.cmd != nil && proc.cmd.Process != nil {
		killProcessGroup(proc.cmd.Process.Pid)
	}

	select {
	case <-proc.doneCh:
	case <-time.After(5 * time.Second):
		if proc.cmd != nil && proc.cmd.Process != nil {
			_ = proc.cmd.Process.Kill()
		}
	}

	m.mu.Lock()
	delete(m.procs, key)
	m.mu.Unlock()
	return nil
}

type schedStatus struct {
	Key       string `json:"key"`
	Running   bool   `json:"running"`
	PID       int    `json:"pid,omitempty"`
	Mode      string `json:"mode,omitempty"`
	Project   string `json:"project,omitempty"`
	Agent     string `json:"agent,omitempty"`
	StartedAt string `json:"startedAt,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (m *SchedulerManager) Status() []schedStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]schedStatus, 0, len(m.procs))
	for key, proc := range m.procs {
		s := schedStatus{
			Key:     key,
			Mode:    proc.mode,
			Project: proc.project,
			Agent:   proc.agent,
		}
		select {
		case <-proc.doneCh:
			s.Running = false
			proc.mu.Lock()
			if proc.exitErr != nil {
				s.Error = proc.exitErr.Error()
			}
			proc.mu.Unlock()
		default:
			s.Running = true
			if proc.cmd != nil && proc.cmd.Process != nil {
				s.PID = proc.cmd.Process.Pid
			}
			s.StartedAt = proc.startedAt.UTC().Format(time.RFC3339)
		}
		out = append(out, s)
	}
	return out
}

func (m *SchedulerManager) WaitKey(key string) error {
	m.mu.Lock()
	proc, ok := m.procs[strings.TrimSpace(key)]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("no scheduler running for %q", key)
	}
	<-proc.doneCh
	proc.mu.Lock()
	err := proc.exitErr
	proc.mu.Unlock()
	m.pruneStopped()
	return err
}

type desiredSchedulerSpec struct {
	Key     string `json:"key,omitempty"`
	Project string `json:"project,omitempty"`
	Agent   string `json:"agent,omitempty"`
	Mode    string `json:"mode,omitempty"`
}

func (spec desiredSchedulerSpec) schedulerKey() string {
	if key := strings.TrimSpace(spec.Key); key != "" {
		return key
	}
	return schedKey(spec.Project, spec.Agent)
}

func schedulerDesiredSettingKey(root string) string {
	sum := sha256.Sum256([]byte(root))
	return "scheduler.desired." + hex.EncodeToString(sum[:8])
}

func (s *Server) loadDesiredSchedulers() ([]desiredSchedulerSpec, error) {
	raw, ok, err := s.controlDB.GetSetting(schedulerDesiredSettingKey(s.root))
	if err != nil || !ok || strings.TrimSpace(raw) == "" {
		return nil, err
	}
	var specs []desiredSchedulerSpec
	if err := json.Unmarshal([]byte(raw), &specs); err != nil {
		return nil, err
	}
	return specs, nil
}

func (s *Server) saveDesiredSchedulers(specs []desiredSchedulerSpec) error {
	b, err := json.Marshal(specs)
	if err != nil {
		return err
	}
	return s.controlDB.SetSetting(schedulerDesiredSettingKey(s.root), string(b))
}

func (s *Server) setSchedulerDesired(project, agent, mode string, running bool) {
	s.setSchedulerDesiredKey(schedKey(project, agent), project, agent, mode, running)
}

func (s *Server) setSchedulerDesiredKey(key, project, agent, mode string, running bool) {
	s.schedulerDesiredMu.Lock()
	defer s.schedulerDesiredMu.Unlock()

	specs, err := s.loadDesiredSchedulers()
	if err != nil {
		return
	}
	key = strings.TrimSpace(key)
	if key == "" {
		key = schedKey(project, agent)
	}
	next := make([]desiredSchedulerSpec, 0, len(specs)+1)
	found := false
	for _, spec := range specs {
		if spec.schedulerKey() == key {
			found = true
			if running {
				next = append(next, desiredSchedulerSpec{Key: key, Project: project, Agent: agent, Mode: strings.TrimSpace(mode)})
			}
			continue
		}
		next = append(next, spec)
	}
	if running && !found {
		next = append(next, desiredSchedulerSpec{Key: key, Project: project, Agent: agent, Mode: strings.TrimSpace(mode)})
	}
	_ = s.saveDesiredSchedulers(next)
}

func (s *Server) restoreDesiredSchedulers() {
	time.Sleep(300 * time.Millisecond)
	specs, err := s.loadDesiredSchedulers()
	if err != nil || len(specs) == 0 {
		return
	}
	for _, spec := range specs {
		if spec.Agent != "" && spec.Project == "" {
			continue
		}
		if spec.Mode == schedulerModeRuntimeNode {
			log.Printf("runtime-node scheduler %s restored as desired but not auto-started; start it from the web/API so the runtime API URL is known", schedKey(spec.Project, spec.Agent))
			continue
		}
		if err := s.sched.Start(spec.Project, spec.Agent); err != nil {
			continue
		}
	}
}

func (m *SchedulerManager) Cleanup() {
	m.mu.Lock()
	keys := make([]string, 0, len(m.procs))
	for k := range m.procs {
		keys = append(keys, k)
	}
	m.mu.Unlock()

	for _, k := range keys {
		_ = m.StopKey(k)
	}
}

func (m *SchedulerManager) GracefulShutdown(ctx context.Context) []schedStatus {
	m.mu.Lock()
	procs := make(map[string]*schedulerProcess, len(m.procs))
	for key, proc := range m.procs {
		procs[key] = proc
	}
	m.mu.Unlock()

	for _, proc := range procs {
		if proc.cancel != nil {
			proc.cancel()
			continue
		}
		// Long-lived scheduler commands should stop accepting new work. One-shot
		// manual runs and wakeups are left alone so the active agent can finish.
		if proc.mode == schedulerModeLocal && proc.cmd != nil && proc.cmd.Process != nil {
			_ = proc.cmd.Process.Signal(syscall.SIGTERM)
		}
	}

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		running := m.runningStatuses()
		if len(running) == 0 {
			m.pruneStopped()
			return nil
		}
		select {
		case <-ctx.Done():
			return running
		case <-ticker.C:
		}
	}
}

func (m *SchedulerManager) runningStatuses() []schedStatus {
	statuses := m.Status()
	running := make([]schedStatus, 0, len(statuses))
	for _, status := range statuses {
		if status.Running {
			running = append(running, status)
		}
	}
	return running
}

func (m *SchedulerManager) pruneStopped() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, proc := range m.procs {
		select {
		case <-proc.doneCh:
			delete(m.procs, key)
		default:
		}
	}
}

// ── HTTP handlers ──

func (s *Server) handleSchedulerStatus(w http.ResponseWriter, r *http.Request) {
	statuses := s.sched.Status()
	cur := s.currentUser(r)
	if cur.Role != RoleAdmin && !s.canAdminCurrentWorkspace(r) {
		filtered := make([]schedStatus, 0, len(statuses))
		for _, st := range statuses {
			if st.Project == "" {
				continue
			}
			if _, ok := s.users.HasProjectAccess(cur.Username, st.Project); ok {
				filtered = append(filtered, st)
			}
		}
		statuses = filtered
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"schedulers": statuses,
	})
}

type schedActionBody struct {
	Project string `json:"project"`
	Agent   string `json:"agent"`
}

func (s *Server) handleSchedulerStart(w http.ResponseWriter, r *http.Request) {
	var body schedActionBody
	if r.ContentLength > 0 {
		if err := s.readJSON(w, r, &body); err != nil {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
			return
		}
	}
	project := strings.TrimSpace(body.Project)
	agent := strings.TrimSpace(body.Agent)

	if agent != "" && project == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "agent requires project")
		return
	}
	if project == "" {
		if !s.checkCurrentWorkspaceAdmin(w, r) {
			return
		}
	} else if !s.checkProjectManager(w, r, project) {
		return
	}

	workspaceID, workspaceOK := s.currentWorkspaceForRequest(w, r)
	if !workspaceOK {
		return
	}
	mode := schedulerModeLocal
	key := schedKey(project, agent)
	useRuntimeNode := false
	if project != "" && agent != "" {
		if meta, err := s.agentMetaForProjectMember(workspaceID, project, agent); err == nil && meta != nil {
			useRuntimeNode = s.usesAssignedRuntimeNode(workspaceID, meta)
		}
	}
	if useRuntimeNode {
		serverURL := externalServerURL(r)
		actor := requestUsername(r)
		mode = schedulerModeRuntimeNode
		key = s.schedulerProcessKeyForProjectAgent(workspaceID, project, agent, mode)
		if err := s.sched.StartLoopWithKey(key, project, agent, mode, func(ctx context.Context) {
			s.runtimeSchedulerLoop(ctx, workspaceID, project, agent, serverURL, actor)
		}); err != nil {
			s.jsonErrorCode(w, http.StatusConflict, ErrCodeSchedulerConflict, err.Error())
			return
		}
	} else if err := s.sched.Start(project, agent); err != nil {
		s.jsonErrorCode(w, http.StatusConflict, ErrCodeSchedulerConflict, err.Error())
		return
	}
	s.setSchedulerDesiredKey(key, project, agent, mode, true)
	s.auditLog(auditLogInput{
		Action:       "scheduler.start",
		ResourceType: "scheduler",
		ResourceID:   schedKey(project, agent),
		Summary:      "Scheduler started",
		After: map[string]any{
			"project": project,
			"agent":   agent,
			"key":     key,
			"mode":    mode,
		},
		Request: r,
	})

	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":   true,
		"key":  key,
		"mode": mode,
	})
}

func (s *Server) handleSchedulerWakeup(w http.ResponseWriter, r *http.Request) {
	var body schedActionBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	project := strings.TrimSpace(body.Project)
	agent := strings.TrimSpace(body.Agent)
	if project == "" || agent == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "project and agent are required")
		return
	}
	if !s.checkProjectManager(w, r, project) {
		return
	}
	workspaceID, workspaceOK := s.currentWorkspaceForRequest(w, r)
	if !workspaceOK {
		return
	}

	target := s.runtimeSchedulerTargetForProjectAgent(workspaceID, project, agent)
	hb, err := s.loadSchedulerTargetHeartbeat(workspaceID, target)
	if err != nil || hb == nil {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeValidationFailed, "heartbeat not found")
		return
	}
	if hb.PID > 0 && hb.LastWakeupStatus == "running" && processAlive(hb.PID) {
		s.jsonErrorCode(w, http.StatusConflict, ErrCodeSchedulerWakeupFailed, fmt.Sprintf("agent %s/%s is already running", project, agent))
		return
	}
	meta, err := s.agentMetaForProjectMember(workspaceID, project, agent)
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
		run, task, err := s.enqueueRuntimeWakeupRunFromRequest(workspaceID, project, agent, hb, externalServerURL(r), requestUsername(r))
		if err != nil {
			s.jsonErrorCode(w, http.StatusInternalServerError, ErrCodeSchedulerWakeupFailed, fmt.Sprintf("queue runtime wakeup failed: %v", err))
			return
		}
		_ = s.saveSchedulerTargetHeartbeat(workspaceID, target, hb)
		s.auditLog(auditLogInput{
			Action:       "scheduler.wakeup",
			ResourceType: "agent",
			ResourceID:   project + "/" + agent,
			Summary:      "Agent wakeup queued on runtime node",
			After: map[string]any{
				"project":      project,
				"agent":        agent,
				"taskId":       task.ID,
				"runtimeRunId": run.ID,
				"runtime":      "node",
			},
			Request: r,
		})
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "status": "queued", "runtimeRunId": run.ID, "taskId": task.ID})
		return
	}

	args := []string{"--dir", s.sched.root, "scheduler", "wakeup", "--project", project, "--agent", agent}
	cmd := exec.Command(s.sched.binPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	setProcGroup(cmd)
	procKey := fmt.Sprintf("wakeup/%s/%s/%d", project, agent, time.Now().UnixNano())
	pid, err := s.sched.StartManagedCommand(procKey, project, agent, schedulerModeWakeup, cmd)
	if err != nil {
		s.jsonErrorCode(w, http.StatusInternalServerError, ErrCodeSchedulerWakeupFailed, fmt.Sprintf("start wakeup failed: %v", err))
		return
	}
	go func() {
		if err := s.sched.WaitKey(procKey); err != nil {
			log.Printf("scheduler wakeup %s/%s exited with error: %v", project, agent, err)
		}
	}()
	s.auditLog(auditLogInput{
		Action:       "scheduler.wakeup",
		ResourceType: "agent",
		ResourceID:   project + "/" + agent,
		Summary:      "Agent wakeup requested",
		After: map[string]any{
			"project": project,
			"agent":   agent,
		},
		Request: r,
	})
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "pid": pid, "status": "started"})
}

func (s *Server) handleStartProjectTask(w http.ResponseWriter, r *http.Request) {
	project := strings.TrimSpace(r.PathValue("name"))
	taskID := strings.TrimSpace(r.PathValue("taskId"))
	if project == "" || taskID == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "project and taskId are required")
		return
	}
	if !s.checkProjectManager(w, r, project) {
		return
	}
	workspaceID, workspaceOK := s.currentWorkspaceForRequest(w, r)
	if !workspaceOK {
		return
	}
	task, agent, err := s.findTaskInProject(project, taskID)
	if err != nil || task == nil {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeValidationFailed, "task not found")
		return
	}
	if err := s.reconcileWorkflowTaskBeforeManualStart(workspaceID, project, taskID, r); err != nil {
		s.serverError(w, err)
		return
	}
	task, agent, err = s.findTaskInProject(project, taskID)
	if err != nil || task == nil {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeValidationFailed, "task not found")
		return
	}
	if task.Status.IsTerminal() {
		s.jsonErrorCode(w, http.StatusConflict, ErrCodeValidationFailed, "task is already finished")
		return
	}
	if task.Status == entity.TaskStatusAwaitingConfirmation {
		s.jsonErrorCode(w, http.StatusConflict, ErrCodeValidationFailed, "current assignee is not an agent")
		return
	}
	target := s.runtimeSchedulerTargetForProjectAgent(workspaceID, project, agent)
	hb, err := s.loadSchedulerTargetHeartbeat(workspaceID, target)
	if err != nil || hb == nil {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeValidationFailed, "heartbeat not found")
		return
	}
	if hb.PID > 0 && hb.LastWakeupStatus == "running" && processAlive(hb.PID) {
		s.jsonErrorCode(w, http.StatusConflict, ErrCodeSchedulerWakeupFailed, fmt.Sprintf("agent %s/%s is already running", project, agent))
		return
	}
	meta, err := s.agentMetaForProjectMember(workspaceID, project, agent)
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
		if s.hasActiveRuntimeRun(workspaceID, project, agent, "") {
			s.jsonErrorCode(w, http.StatusConflict, ErrCodeSchedulerWakeupFailed, fmt.Sprintf("agent %s/%s is already running", project, agent))
			return
		}
		run, err := s.enqueueSpecificRuntimeTaskRunFromRequest(workspaceID, project, agent, task, hb, externalServerURL(r), requestUsername(r))
		if err != nil {
			s.jsonErrorCode(w, http.StatusInternalServerError, ErrCodeSchedulerWakeupFailed, fmt.Sprintf("queue task run failed: %v", err))
			return
		}
		_ = s.saveSchedulerTargetHeartbeat(workspaceID, target, hb)
		s.auditLog(auditLogInput{
			Action:       "task.start",
			ResourceType: "task",
			ResourceID:   project + "/" + agent + "/" + task.ID,
			Summary:      "Specific task queued on runtime node",
			After: map[string]any{
				"project":      project,
				"agent":        agent,
				"taskId":       task.ID,
				"runtimeRunId": run.ID,
				"runtime":      "node",
			},
			Request: r,
		})
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "status": "queued", "runtimeRunId": run.ID, "taskId": task.ID, "agent": agent})
		return
	}

	args := []string{"--dir", s.sched.root, "run", "--project", project, "--agent", agent, "--task", task.ID}
	cmd := exec.Command(s.sched.binPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	setProcGroup(cmd)
	procKey := fmt.Sprintf("manual-task/%s/%s/%s", project, agent, task.ID)
	pid, err := s.sched.StartManagedCommand(procKey, project, agent, schedulerModeManualTask, cmd)
	if err != nil {
		s.jsonErrorCode(w, http.StatusInternalServerError, ErrCodeSchedulerWakeupFailed, fmt.Sprintf("start task run failed: %v", err))
		return
	}
	now := time.Now().UTC()
	hb.LastWakeup = &now
	hb.LastWakeupStatus = "running"
	hb.PID = pid
	_ = s.saveSchedulerTargetHeartbeat(workspaceID, target, hb)
	go func() {
		if err := s.sched.WaitKey(procKey); err != nil {
			log.Printf("manual task run %s/%s task=%s exited with error: %v", project, agent, task.ID, err)
		}
		if hb2, err := s.loadSchedulerTargetHeartbeat(workspaceID, target); err == nil && hb2 != nil && hb2.PID == pid {
			hb2.PID = 0
			if hb2.LastWakeupStatus == "running" {
				hb2.LastWakeupStatus = "done"
			}
			_ = s.saveSchedulerTargetHeartbeat(workspaceID, target, hb2)
		}
	}()
	s.auditLog(auditLogInput{
		Action:       "task.start",
		ResourceType: "task",
		ResourceID:   project + "/" + agent + "/" + task.ID,
		Summary:      "Specific task run requested",
		After: map[string]any{
			"project": project,
			"agent":   agent,
			"taskId":  task.ID,
			"pid":     pid,
		},
		Request: r,
	})
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "pid": pid, "status": "started", "taskId": task.ID, "agent": agent})
}

func (s *Server) reconcileWorkflowTaskBeforeManualStart(workspaceID, project, taskID string, r *http.Request) error {
	if s == nil || s.controlDB == nil || strings.TrimSpace(workspaceID) == "" {
		return nil
	}
	wfStore := workflowstore.NewStore(s.controlDB, workspaceID)
	run, found, err := wfStore.RunForTask(project, taskID)
	if err != nil || !found || strings.TrimSpace(run.ActiveStepID) == "" || strings.TrimSpace(run.Status) == "completed" {
		return err
	}
	def, found, err := wfStore.RunDefinition(run)
	if err != nil || !found {
		return err
	}
	steps, err := wfStore.ListStepInstances(run.ID)
	if err != nil {
		return err
	}
	return s.reconcileActiveWorkflowTaskQueue(workspaceID, project, taskID, run, def, steps, r)
}

func (s *Server) enqueueSpecificRuntimeTaskRunFromRequest(workspaceID, project, agent string, task *entity.Task, hb *entity.HeartbeatConfig, serverURL, actor string) (controldb.RuntimeRun, error) {
	if task == nil {
		return controldb.RuntimeRun{}, fmt.Errorf("task not found")
	}
	now := time.Now().UTC()
	prev := task.Status
	task.Status = entity.TaskStatusInProgress
	task.UpdatedAt = now
	task.FinishedAt = nil
	entity.ApplyStatusTimestamps(task, prev, now)
	if err := s.ts.UpdateTask(project, agent, task); err != nil {
		return controldb.RuntimeRun{}, err
	}
	sessionID := ""
	if hb != nil {
		sessionID = strings.TrimSpace(hb.SessionID)
	}
	run, err := s.enqueueRuntimeTaskRun(workspaceID, project, agent, task, sessionID, serverURL, actor)
	if err != nil {
		task.Status = prev
		task.UpdatedAt = now
		_ = s.ts.UpdateTask(project, agent, task)
		return controldb.RuntimeRun{}, err
	}
	if hb != nil {
		hb.LastWakeup = &now
		hb.LastWakeupStatus = "running"
		hb.PID = 0
	}
	return run, nil
}

func (s *Server) enqueueRuntimeWakeupRunFromRequest(workspaceID, project, agent string, hb *entity.HeartbeatConfig, serverURL, actor string) (controldb.RuntimeRun, *entity.Task, error) {
	task, attentionIDs, err := s.nextRuntimeWakeupTask(workspaceID, project, agent, hb)
	if err != nil {
		return controldb.RuntimeRun{}, nil, err
	}
	if task == nil {
		return controldb.RuntimeRun{}, nil, fmt.Errorf("no pending task or wakeup prompt")
	}
	now := time.Now().UTC()
	prev := task.Status
	task.Status = entity.TaskStatusInProgress
	task.UpdatedAt = now
	entity.ApplyStatusTimestamps(task, prev, now)
	if err := s.ts.UpdateTask(project, agent, task); err != nil {
		return controldb.RuntimeRun{}, nil, err
	}
	run, err := s.enqueueRuntimeTaskRun(workspaceID, project, agent, task, strings.TrimSpace(hb.SessionID), serverURL, actor)
	if err != nil {
		return controldb.RuntimeRun{}, nil, err
	}
	s.markAttentionSignalsSeen(workspaceID, attentionIDs)
	hb.LastWakeup = &now
	hb.LastWakeupStatus = "running"
	hb.PID = 0
	return run, task, nil
}

func (s *Server) nextRuntimeWakeupTask(workspaceID, project, agent string, hb *entity.HeartbeatConfig) (*entity.Task, []string, error) {
	if task, ids, err := s.ensurePendingAttentionWakeupTask(workspaceID, project, agent); err != nil {
		return nil, nil, err
	} else if task != nil {
		return task, ids, nil
	}
	selected, err := s.nextRuntimePendingTask(project, agent)
	if err != nil {
		return nil, nil, err
	}
	if selected != nil {
		return selected, nil, nil
	}
	prompt, err := s.resolveRuntimeWakeupPrompt(project, agent, hb)
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(prompt) == "" {
		return nil, nil, nil
	}
	var attentionIDs []string
	if section, ids, err := s.pendingAttentionWakeupSection(workspaceID, project, agent); err == nil && section != "" {
		prompt = section + prompt
		attentionIDs = ids
	}
	task := s.createRuntimeWakeupTask(project, agent, prompt)
	if err := s.ts.AddTask(project, agent, task); err != nil {
		return nil, nil, err
	}
	return task, attentionIDs, nil
}

func (s *Server) createRuntimeWakeupTask(project, agent, prompt string) *entity.Task {
	now := time.Now().UTC()
	return &entity.Task{
		ID:        "t-" + now.Format("20060102") + "-" + randomRuntimeHex(3),
		Title:     "[wakeup] routine",
		Status:    entity.TaskStatusPending,
		Type:      "wakeup",
		Priority:  9,
		Prompt:    prompt,
		CreatedBy: "heartbeat:wakeup",
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (s *Server) nextRuntimePendingTask(project, agent string) (*entity.Task, error) {
	tasks, err := s.ts.ListTasks(project, agent, entity.TaskStatusPending)
	if err != nil {
		return nil, err
	}
	var selected *entity.Task
	now := time.Now().UTC()
	for _, task := range tasks {
		if task == nil {
			continue
		}
		if !entity.TaskReady(task, now) {
			continue
		}
		if selected == nil ||
			task.Priority < selected.Priority ||
			(task.Priority == selected.Priority && task.CreatedAt.Before(selected.CreatedAt)) ||
			(task.Priority == selected.Priority && task.CreatedAt.Equal(selected.CreatedAt) && task.ID < selected.ID) {
			selected = task
		}
	}
	return selected, nil
}

func (s *Server) resolveRuntimeWakeupPrompt(project, agent string, hb *entity.HeartbeatConfig) (string, error) {
	raw := ""
	if hb != nil {
		raw = strings.TrimSpace(hb.WakeupPrompt)
	}
	if raw == "" {
		return "Execute your wakeup routine. Check pending tasks, unread messages, and your scheduled activities.", nil
	}
	if !strings.HasPrefix(raw, "@") {
		return raw, nil
	}
	rel := strings.TrimSpace(strings.TrimPrefix(raw, "@"))
	if rel == "" {
		return "", nil
	}
	path := rel
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.st.AgentDir(project, agent), rel)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (s *Server) handleSchedulerStop(w http.ResponseWriter, r *http.Request) {
	var body schedActionBody
	if r.ContentLength > 0 {
		if err := s.readJSON(w, r, &body); err != nil {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
			return
		}
	}
	project := strings.TrimSpace(body.Project)
	agent := strings.TrimSpace(body.Agent)
	if agent != "" && project == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "agent requires project")
		return
	}
	if project == "" {
		if !s.checkCurrentWorkspaceAdmin(w, r) {
			return
		}
	} else if !s.checkProjectManager(w, r, project) {
		return
	}

	workspaceID, workspaceOK := s.currentWorkspaceForRequest(w, r)
	if !workspaceOK {
		return
	}
	key := s.schedulerProcessKeyForProjectAgent(workspaceID, project, agent, schedulerModeRuntimeNode)
	if err := s.sched.StopKey(key); err != nil {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeSchedulerNotFound, err.Error())
		return
	}
	s.setSchedulerDesiredKey(key, project, agent, "", false)

	s.clearSchedulerRuntimeFields(workspaceID, project, agent)
	s.auditLog(auditLogInput{
		Action:       "scheduler.stop",
		ResourceType: "scheduler",
		ResourceID:   schedKey(project, agent),
		Summary:      "Scheduler stopped",
		After: map[string]any{
			"project": project,
			"agent":   agent,
			"key":     schedKey(project, agent),
		},
		Request: r,
	})

	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *Server) schedulerProcessKeyForProjectAgent(workspaceID, project, agent, mode string) string {
	if strings.TrimSpace(mode) == schedulerModeRuntimeNode && strings.TrimSpace(project) != "" && strings.TrimSpace(agent) != "" {
		target := s.runtimeSchedulerTargetForProjectAgent(workspaceID, project, agent)
		if workerID := strings.TrimSpace(target.workerID); workerID != "" {
			return "worker/" + workerID
		}
	}
	return schedKey(project, agent)
}

func (s *Server) clearSchedulerRuntimeFields(workspaceID, project, agent string) {
	if project == "" {
		return
	}
	if strings.TrimSpace(agent) != "" {
		target := s.runtimeSchedulerTargetForProjectAgent(workspaceID, project, agent)
		hb, err := s.loadSchedulerTargetHeartbeat(workspaceID, target)
		if err != nil || hb == nil {
			return
		}
		hb.NextWakeupAt = nil
		hb.SchedulerStartedAt = nil
		hb.PID = 0
		if hb.LastWakeupStatus == "running" {
			hb.LastWakeupStatus = "done"
		}
		_ = s.saveSchedulerTargetHeartbeat(workspaceID, target, hb)
		return
	}
	targets := []runtimeSchedulerAgentTarget{s.runtimeSchedulerTargetForProjectAgent(workspaceID, project, agent)}
	if agent == "" {
		targets = s.runtimeSchedulerTargets(workspaceID, project, "")
	}
	for _, target := range targets {
		hb, err := s.loadSchedulerTargetHeartbeat(workspaceID, target)
		if err != nil || hb == nil {
			continue
		}
		hb.NextWakeupAt = nil
		hb.SchedulerStartedAt = nil
		hb.PID = 0
		if hb.LastWakeupStatus == "running" {
			hb.LastWakeupStatus = "done"
		}
		_ = s.saveSchedulerTargetHeartbeat(workspaceID, target, hb)
	}
}

func (s *Server) runtimeSchedulerLoop(ctx context.Context, workspaceID, project, agent, serverURL, actor string) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	s.runtimeSchedulerTick(ctx, workspaceID, project, agent, serverURL, actor)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runtimeSchedulerTick(ctx, workspaceID, project, agent, serverURL, actor)
		}
	}
}

func (s *Server) runtimeSchedulerTick(ctx context.Context, workspaceID, project, agent, serverURL, actor string) {
	if ctx.Err() != nil {
		return
	}
	targets := s.runtimeSchedulerTargets(workspaceID, project, agent)
	now := time.Now()
	for _, target := range targets {
		if ctx.Err() != nil {
			return
		}
		hb, err := s.loadSchedulerTargetHeartbeat(workspaceID, target)
		if err != nil || hb == nil {
			continue
		}
		if !s.runtimeHeartbeatDue(workspaceID, target, hb, now) {
			continue
		}
		execTarget := s.selectRuntimeSchedulerExecutionTarget(target)
		meta, err := s.agentMetaForProjectMember(workspaceID, execTarget.project, execTarget.agent)
		if err != nil || meta == nil || meta.Model == entity.ModelHuman {
			continue
		}
		if !s.usesAssignedRuntimeNode(workspaceID, meta) {
			continue
		}
		if s.hasActiveRuntimeRunForTarget(workspaceID, target, "") {
			continue
		}
		run, task, err := s.enqueueRuntimeWakeupRunFromRequest(workspaceID, execTarget.project, execTarget.agent, hb, serverURL, actor)
		if err != nil {
			log.Printf("runtime scheduler enqueue %s/%s failed: %v", execTarget.project, execTarget.agent, err)
			continue
		}
		if hb2, err := s.loadSchedulerTargetHeartbeat(workspaceID, target); err == nil && hb2 != nil {
			if next := nextRuntimeHeartbeatAt(hb2, time.Now()); !next.IsZero() {
				nextUTC := next.UTC()
				hb2.NextWakeupAt = &nextUTC
			}
			_ = s.saveSchedulerTargetHeartbeat(workspaceID, target, hb2)
		}
		log.Printf("runtime scheduler queued %s/%s task=%s run=%s", execTarget.project, execTarget.agent, task.ID, run.ID)
	}
}

type runtimeSchedulerAgentTarget struct {
	project      string
	agent        string
	workerID     string
	membershipID string
	memberships  []runtimeSchedulerAgentTarget
}

func (s *Server) runtimeSchedulerTargets(workspaceID, project, agent string) []runtimeSchedulerAgentTarget {
	project = strings.TrimSpace(project)
	agent = strings.TrimSpace(agent)
	if project != "" && agent != "" {
		return []runtimeSchedulerAgentTarget{s.runtimeSchedulerTargetGroupForProjectAgent(workspaceID, project, agent)}
	}
	projects := []string{}
	if project != "" {
		projects = []string{project}
	} else if rows, err := s.ts.ListProjects(); err == nil {
		projects = rows
	}
	out := []runtimeSchedulerAgentTarget{}
	seen := map[string]bool{}
	seenWorkers := map[string]bool{}
	for _, p := range projects {
		if s != nil && s.controlDB != nil && strings.TrimSpace(workspaceID) != "" {
			memberships, err := s.controlDB.ListProjectMemberships(controldb.ProjectMembershipFilter{
				WorkspaceID: workspaceID,
				ProjectID:   p,
				MemberType:  "agent_worker",
			})
			if err == nil {
				for _, membership := range memberships {
					if !membership.AutoPickTasks {
						continue
					}
					name := strings.TrimSpace(membership.Title)
					if name == "" {
						worker, ok, err := s.agentDirectory.Worker(workspaceID, membership.MemberID)
						if err == nil && ok {
							name = worker.Name
						}
					}
					if name == "" || strings.HasPrefix(name, ".") {
						continue
					}
					if strings.TrimSpace(membership.MemberID) != "" {
						if seenWorkers[membership.MemberID] {
							continue
						}
						seenWorkers[membership.MemberID] = true
						out = append(out, s.runtimeSchedulerTargetGroupForWorker(workspaceID, membership.MemberID, p, name))
						continue
					}
					key := p + "/" + name
					if seen[key] {
						continue
					}
					seen[key] = true
					out = append(out, runtimeSchedulerAgentTarget{
						project:      p,
						agent:        name,
						workerID:     membership.MemberID,
						membershipID: membership.ID,
					})
				}
			}
		}
	}
	return out
}

func (s *Server) runtimeSchedulerTargetGroupForProjectAgent(workspaceID, project, agent string) runtimeSchedulerAgentTarget {
	target := s.runtimeSchedulerTargetForProjectAgent(workspaceID, project, agent)
	if strings.TrimSpace(target.workerID) == "" {
		target.memberships = []runtimeSchedulerAgentTarget{target}
		return target
	}
	group := s.runtimeSchedulerTargetGroupForWorker(workspaceID, target.workerID, target.project, target.agent)
	if len(group.memberships) == 0 {
		group.memberships = []runtimeSchedulerAgentTarget{target}
	}
	return group
}

func (s *Server) runtimeSchedulerTargetGroupForWorker(workspaceID, workerID, preferredProject, preferredAgent string) runtimeSchedulerAgentTarget {
	base := runtimeSchedulerAgentTarget{project: strings.TrimSpace(preferredProject), agent: strings.TrimSpace(preferredAgent), workerID: strings.TrimSpace(workerID)}
	if s == nil || s.controlDB == nil || strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(workerID) == "" {
		base.memberships = []runtimeSchedulerAgentTarget{base}
		return base
	}
	memberships, err := s.controlDB.ListProjectMemberships(controldb.ProjectMembershipFilter{
		WorkspaceID: workspaceID,
		MemberType:  "agent_worker",
		MemberID:    workerID,
	})
	if err != nil || len(memberships) == 0 {
		base.memberships = []runtimeSchedulerAgentTarget{base}
		return base
	}
	worker := controldb.AgentWorker{}
	if loaded, ok, err := s.agentDirectory.Worker(workspaceID, workerID); err == nil && ok {
		worker = loaded
	}
	group := make([]runtimeSchedulerAgentTarget, 0, len(memberships))
	for _, membership := range memberships {
		if !membership.AutoPickTasks {
			continue
		}
		name := schedulerMembershipRuntimeAgentName(membership, worker)
		if name == "" || strings.HasPrefix(name, ".") {
			continue
		}
		item := runtimeSchedulerAgentTarget{
			project:      strings.TrimSpace(membership.ProjectID),
			agent:        name,
			workerID:     workerID,
			membershipID: membership.ID,
		}
		group = append(group, item)
		if item.project == base.project && item.agent == base.agent {
			base.membershipID = item.membershipID
		}
	}
	if len(group) == 0 {
		base.memberships = []runtimeSchedulerAgentTarget{base}
		return base
	}
	base.memberships = group
	if strings.TrimSpace(base.project) == "" || strings.TrimSpace(base.agent) == "" {
		base.project = group[0].project
		base.agent = group[0].agent
		base.membershipID = group[0].membershipID
	}
	return base
}

func schedulerMembershipRuntimeAgentName(membership controldb.ProjectMembership, worker controldb.AgentWorker) string {
	for _, value := range []string{membership.Title, membership.Role, worker.Name, worker.DisplayName} {
		if text := strings.TrimSpace(value); text != "" {
			return text
		}
	}
	return ""
}

func (s *Server) runtimeSchedulerTargetForProjectAgent(workspaceID, project, agent string) runtimeSchedulerAgentTarget {
	target := runtimeSchedulerAgentTarget{project: strings.TrimSpace(project), agent: strings.TrimSpace(agent)}
	if s == nil || s.agentDirectory == nil || strings.TrimSpace(workspaceID) == "" {
		return target
	}
	resolved, ok, err := s.agentDirectory.ResolveProjectMailbox(workspaceID, target.project+"/"+target.agent)
	if err == nil && ok {
		target.workerID = resolved.Worker.ID
		target.membershipID = resolved.Membership.ID
		if title := strings.TrimSpace(resolved.Membership.Title); title != "" {
			target.agent = title
		}
	}
	return target
}

func (s *Server) selectRuntimeSchedulerExecutionTarget(target runtimeSchedulerAgentTarget) runtimeSchedulerAgentTarget {
	memberships := target.memberships
	if len(memberships) == 0 {
		return target
	}
	best := memberships[0]
	var bestTask *entity.Task
	for _, membership := range memberships {
		task, err := s.nextRuntimePendingTask(membership.project, membership.agent)
		if err != nil || task == nil {
			continue
		}
		if bestTask == nil ||
			task.Priority < bestTask.Priority ||
			(task.Priority == bestTask.Priority && task.CreatedAt.Before(bestTask.CreatedAt)) ||
			(task.Priority == bestTask.Priority && task.CreatedAt.Equal(bestTask.CreatedAt) && task.ID < bestTask.ID) {
			best = membership
			bestTask = task
		}
	}
	if bestTask != nil {
		return best
	}
	if strings.TrimSpace(target.project) != "" && strings.TrimSpace(target.agent) != "" {
		return target
	}
	return best
}

func (s *Server) loadSchedulerTargetHeartbeat(workspaceID string, target runtimeSchedulerAgentTarget) (*entity.HeartbeatConfig, error) {
	if strings.TrimSpace(target.workerID) == "" {
		return nil, fmt.Errorf("agent worker membership not found for %s/%s", target.project, target.agent)
	}
	worker, ok, err := s.controlDB.AgentWorkerByID(workspaceID, target.workerID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("agent worker %s not found", target.workerID)
	}
	hb := &entity.HeartbeatConfig{}
	if raw := strings.TrimSpace(worker.ScheduleJSON); raw != "" && raw != "{}" {
		if err := json.Unmarshal([]byte(raw), hb); err != nil {
			return nil, fmt.Errorf("parse agent worker schedule %s: %w", worker.ID, err)
		}
	}
	return hb, nil
}

func (s *Server) saveSchedulerTargetHeartbeat(workspaceID string, target runtimeSchedulerAgentTarget, hb *entity.HeartbeatConfig) error {
	if hb == nil {
		return nil
	}
	if strings.TrimSpace(target.workerID) == "" {
		return fmt.Errorf("agent worker membership not found for %s/%s", target.project, target.agent)
	}
	worker, ok, err := s.controlDB.AgentWorkerByID(workspaceID, target.workerID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("agent worker %s not found", target.workerID)
	}
	raw, err := json.Marshal(hb)
	if err != nil {
		return err
	}
	worker.ScheduleJSON = string(raw)
	worker.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return s.controlDB.UpsertAgentWorker(worker)
}

func (s *Server) runtimeHeartbeatDue(workspaceID string, target runtimeSchedulerAgentTarget, hb *entity.HeartbeatConfig, now time.Time) bool {
	if hb == nil || !hb.Enabled || hb.Paused {
		return false
	}
	if hb.LastWakeupStatus == "running" {
		if s.hasActiveRuntimeRunForTarget(workspaceID, target, "") {
			return false
		}
		hb.LastWakeupStatus = "done"
		hb.PID = 0
		_ = s.saveSchedulerTargetHeartbeat(workspaceID, target, hb)
	}
	if !runtimeActiveDay(hb.ActiveDays, now) {
		return false
	}
	if hb.ActiveHours != "" {
		ok, _ := runtimeActiveHourAt(hb.ActiveHours, now)
		if !ok {
			return false
		}
	}
	interval := runtimeHeartbeatInterval(hb)
	next := now
	if hb.LastWakeup != nil {
		next = hb.LastWakeup.Add(interval)
	}
	if next.After(now) {
		nextUTC := next.UTC()
		if hb.NextWakeupAt == nil || !hb.NextWakeupAt.Equal(nextUTC) {
			hb.NextWakeupAt = &nextUTC
			_ = s.saveSchedulerTargetHeartbeat(workspaceID, target, hb)
		}
		return false
	}
	return true
}

func runtimeHeartbeatInterval(hb *entity.HeartbeatConfig) time.Duration {
	if hb == nil || strings.TrimSpace(hb.Interval) == "" {
		return 30 * time.Minute
	}
	d, err := time.ParseDuration(strings.TrimSpace(hb.Interval))
	if err != nil || d <= 0 {
		return 30 * time.Minute
	}
	return d
}

func nextRuntimeHeartbeatAt(hb *entity.HeartbeatConfig, now time.Time) time.Time {
	if hb == nil {
		return time.Time{}
	}
	base := now
	if hb.LastWakeup != nil {
		base = *hb.LastWakeup
	}
	return base.Add(runtimeHeartbeatInterval(hb))
}

func (s *Server) hasActiveRuntimeRun(workspaceID, project, agent, taskID string) bool {
	target := s.runtimeSchedulerTargetForProjectAgent(workspaceID, project, agent)
	return s.hasActiveRuntimeRunForTarget(workspaceID, target, taskID)
}

func (s *Server) hasActiveRuntimeRunForTarget(workspaceID string, target runtimeSchedulerAgentTarget, taskID string) bool {
	if s == nil || s.controlDB == nil {
		return false
	}
	filter := controldb.RuntimeRunFilter{
		WorkspaceID: workspaceID,
		TaskID:      taskID,
		Limit:       50,
	}
	if strings.TrimSpace(target.workerID) != "" {
		filter.AgentWorkerID = target.workerID
	} else {
		filter.ProjectID = target.project
		filter.AgentID = target.agent
	}
	if strings.TrimSpace(filter.WorkspaceID) == "" {
		if ws, err := s.currentWorkspaceID(); err == nil {
			filter.WorkspaceID = ws
		}
	}
	if strings.TrimSpace(filter.WorkspaceID) == "" {
		return false
	}
	for _, status := range []string{"queued", "running"} {
		filter.Status = status
		runs, err := s.controlDB.ListRuntimeRuns(filter)
		if err != nil {
			continue
		}
		for _, run := range runs {
			if runtimeRunBlocksAgent(run, time.Now().UTC()) {
				return true
			}
		}
	}
	return false
}

func runtimeRunBlocksAgent(run controldb.RuntimeRun, now time.Time) bool {
	switch strings.TrimSpace(run.Status) {
	case "queued":
		return true
	case "running":
		lease := strings.TrimSpace(run.LeaseExpiresAt)
		if lease == "" {
			return true
		}
		expiresAt, err := time.Parse(time.RFC3339, lease)
		if err != nil {
			return true
		}
		return expiresAt.After(now)
	default:
		return false
	}
}

func runtimeActiveHourAt(activeHours string, t time.Time) (bool, time.Duration) {
	parts := strings.SplitN(strings.TrimSpace(activeHours), "-", 2)
	if len(parts) != 2 {
		return true, 0
	}
	parse := func(s string) (int, bool) {
		v, err := time.Parse("15:04", strings.TrimSpace(s))
		if err != nil {
			return 0, false
		}
		return v.Hour()*60 + v.Minute(), true
	}
	start, ok1 := parse(parts[0])
	end, ok2 := parse(parts[1])
	if !ok1 || !ok2 || start == end {
		return true, 0
	}
	nowMin := t.Hour()*60 + t.Minute()
	if start < end {
		if nowMin >= start && nowMin < end {
			return true, 0
		}
		openAt := time.Date(t.Year(), t.Month(), t.Day(), start/60, start%60, 0, 0, t.Location())
		if openAt.Before(t) {
			openAt = openAt.Add(24 * time.Hour)
		}
		return false, openAt.Sub(t)
	}
	if nowMin >= start || nowMin < end {
		return true, 0
	}
	openAt := time.Date(t.Year(), t.Month(), t.Day(), start/60, start%60, 0, 0, t.Location())
	if openAt.Before(t) {
		openAt = openAt.Add(24 * time.Hour)
	}
	return false, openAt.Sub(t)
}

func runtimeActiveDay(activeDays string, now time.Time) bool {
	activeDays = strings.TrimSpace(activeDays)
	if activeDays == "" {
		return true
	}
	day := strings.ToLower(now.Weekday().String()[:3])
	for _, token := range strings.Split(activeDays, ",") {
		tok := strings.ToLower(strings.TrimSpace(token))
		switch tok {
		case "weekdays":
			if now.Weekday() >= time.Monday && now.Weekday() <= time.Friday {
				return true
			}
		case "weekends":
			if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
				return true
			}
		case day:
			return true
		}
	}
	return false
}

func (s *Server) handleSchedulerAbort(w http.ResponseWriter, r *http.Request) {
	var body schedActionBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	project := strings.TrimSpace(body.Project)
	agent := strings.TrimSpace(body.Agent)
	if project == "" || agent == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "project and agent are required")
		return
	}
	if !s.checkProjectManager(w, r, project) {
		return
	}
	currentWorkspaceID, workspaceOK := s.currentWorkspaceForRequest(w, r)
	if !workspaceOK {
		return
	}

	target := s.runtimeSchedulerTargetForProjectAgent(currentWorkspaceID, project, agent)
	hb, err := s.loadSchedulerTargetHeartbeat(currentWorkspaceID, target)
	if err != nil || hb == nil {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeHeartbeatNotFound, "heartbeat config not found")
		return
	}

	cancelledRemote, err := s.cancelRuntimeRunsForAgent(currentWorkspaceID, project, agent, "", "agent run aborted")
	if err != nil {
		s.serverError(w, err)
		return
	}

	if hb.PID <= 0 || hb.LastWakeupStatus != "running" {
		if cancelledRemote > 0 {
			hb.PID = 0
			hb.LastWakeupStatus = "aborted"
			_ = s.saveSchedulerTargetHeartbeat(currentWorkspaceID, target, hb)
			s.auditLog(auditLogInput{
				Action:       "scheduler.abort",
				ResourceType: "agent",
				ResourceID:   project + "/" + agent,
				Summary:      "Remote agent run cancelled",
				After: map[string]any{
					"project":              project,
					"agent":                agent,
					"runtimeRunsCancelled": cancelledRemote,
				},
				Request: r,
			})
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "runtimeRunsCancelled": cancelledRemote})
			return
		}
		s.jsonErrorCode(w, http.StatusConflict, ErrCodeAgentNotRunning, "agent is not currently running")
		return
	}

	pid := hb.PID

	proc, err := os.FindProcess(pid)
	if err != nil {
		s.jsonErrorCode(w, http.StatusInternalServerError, ErrCodeProcessNotFound, fmt.Sprintf("cannot find process %d: %v", pid, err))
		return
	}

	// Signal 0 checks liveness.
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		hb.PID = 0
		hb.LastWakeupStatus = "aborted"
		_ = s.saveSchedulerTargetHeartbeat(currentWorkspaceID, target, hb)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "msg": "process already dead, status updated"})
		return
	}

	// Kill the process group to ensure child processes (docker, claude) are also terminated.
	killProcessGroup(pid)

	// Give it a moment then force kill if needed.
	time.Sleep(500 * time.Millisecond)
	if err := proc.Signal(syscall.Signal(0)); err == nil {
		_ = proc.Kill()
	}

	hb.PID = 0
	hb.LastWakeupStatus = "aborted"
	_ = s.saveSchedulerTargetHeartbeat(currentWorkspaceID, target, hb)
	s.auditLog(auditLogInput{
		Action:       "scheduler.abort",
		ResourceType: "agent",
		ResourceID:   project + "/" + agent,
		Summary:      "Agent run aborted",
		After: map[string]any{
			"project":              project,
			"agent":                agent,
			"pid":                  pid,
			"runtimeRunsCancelled": cancelledRemote,
		},
		Request: r,
	})

	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "pid": pid, "runtimeRunsCancelled": cancelledRemote})
}
