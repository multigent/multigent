package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/taskstore"
)

// triggerManager handles event-driven agent wakeups.
// It deduplicates concurrent triggers for the same agent and runs wakeups
// asynchronously so the originating API call returns immediately.
//
// It also runs a background poller that periodically checks for agents with
// message triggers that have unread messages (to catch messages sent via CLI).
// The poller debounces: when unread messages are first detected it waits for
// TriggerDebounce (default 5m) before firing, so that multiple agent-to-agent
// messages accumulate and are processed in a single wakeup.
type triggerManager struct {
	mu        sync.Mutex
	inflight  map[string]time.Time // key = "project/agent" → trigger start time
	firstSeen map[string]time.Time // key = "project/agent" → when unread was first detected (debounce)
	root      string
	binPath   string
	ts        taskstore.Store
	db        controldb.Store

	cancel   context.CancelFunc
	pollDone chan struct{}
}

const defaultTriggerDebounce = 5 * time.Minute

func newTriggerManager(root, binPath string, ts taskstore.Store, db controldb.Store) *triggerManager {
	return &triggerManager{
		inflight:  make(map[string]time.Time),
		firstSeen: make(map[string]time.Time),
		root:      root,
		binPath:   binPath,
		ts:        ts,
		db:        db,
	}
}

// StartPoller launches a background goroutine that periodically checks for
// agents with message triggers that have unread messages, plus due scheduled
// wakeup tasks created with task.notBefore.
// This catches messages sent via CLI (which bypass the API trigger path).
func (tm *triggerManager) StartPoller() {
	ctx, cancel := context.WithCancel(context.Background())
	tm.cancel = cancel
	tm.pollDone = make(chan struct{})
	go tm.pollLoop(ctx)
}

// StopPoller stops the background poller.
func (tm *triggerManager) StopPoller() {
	if tm.cancel != nil {
		tm.cancel()
		<-tm.pollDone
	}
}

func (tm *triggerManager) pollLoop(ctx context.Context) {
	defer close(tm.pollDone)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tm.checkMessageTriggers()
			tm.checkDueScheduledTaskWakeups()
		}
	}
}

// checkMessageTriggers scans all agents for those with message triggers
// that have unread messages, and fires triggers for them after a debounce
// period to allow multiple messages to accumulate.
func (tm *triggerManager) checkMessageTriggers() {
	projects, err := tm.ts.ListProjects()
	if err != nil {
		return
	}
	now := time.Now()
	workspaceID := tm.workspaceID()
	for _, project := range projects {
		if tm.db == nil || workspaceID == "" {
			continue
		}
		memberships, err := tm.db.ListProjectMemberships(controldb.ProjectMembershipFilter{
			WorkspaceID: workspaceID,
			ProjectID:   project,
			MemberType:  "agent_worker",
		})
		if err != nil {
			continue
		}
		for _, membership := range memberships {
			agent := strings.TrimSpace(membership.Title)
			if agent == "" {
				worker, ok, err := tm.db.AgentWorkerByID(workspaceID, membership.MemberID)
				if err == nil && ok {
					agent = strings.TrimSpace(worker.Name)
				}
			}
			if agent == "" {
				continue
			}
			hb, configured := tm.heartbeatForTrigger(project, agent)
			if !configured || hb == nil {
				continue
			}
			if hb.Paused {
				continue
			}
			if !hb.HasTrigger(entity.TriggerOnMessage) {
				continue
			}

			key := project + "/" + agent
			unread, err := tm.ts.ListUnreadMessages(key)
			if err != nil || len(unread) == 0 {
				tm.mu.Lock()
				delete(tm.firstSeen, key)
				tm.mu.Unlock()
				continue
			}

			debounce := defaultTriggerDebounce
			if hb.TriggerDebounce != "" {
				if d, err := time.ParseDuration(hb.TriggerDebounce); err == nil && d > 0 {
					debounce = d
				}
			}

			tm.mu.Lock()
			seen, exists := tm.firstSeen[key]
			if !exists {
				tm.firstSeen[key] = now
				tm.mu.Unlock()
				fmt.Fprintf(os.Stderr, "[trigger-poller] %s: %d unread detected, debouncing %s\n", key, len(unread), debounce)
				continue
			}
			if now.Sub(seen) < debounce {
				tm.mu.Unlock()
				continue
			}
			delete(tm.firstSeen, key)
			tm.mu.Unlock()

			fmt.Fprintf(os.Stderr, "[trigger-poller] %s: debounce elapsed, firing (%d unread)\n", key, len(unread))
			tm.Fire(project, agent, entity.TriggerOnMessage, fmt.Sprintf("poller: %d unread", len(unread)))
		}
	}
}

// checkDueScheduledTaskWakeups wakes agents that have at least one due
// notBefore-gated pending task. This is intentionally independent from the
// generic "task" trigger: when an agent schedules a reminder for itself, the
// due time is the trigger.
func (tm *triggerManager) checkDueScheduledTaskWakeups() {
	projects, err := tm.ts.ListProjects()
	if err != nil {
		return
	}
	now := time.Now().UTC()
	workspaceID := tm.workspaceID()
	for _, project := range projects {
		if tm.db == nil || workspaceID == "" {
			continue
		}
		memberships, err := tm.db.ListProjectMemberships(controldb.ProjectMembershipFilter{
			WorkspaceID: workspaceID,
			ProjectID:   project,
			MemberType:  "agent_worker",
		})
		if err != nil {
			continue
		}
		for _, membership := range memberships {
			agent := strings.TrimSpace(membership.Title)
			if agent == "" {
				worker, ok, err := tm.db.AgentWorkerByID(workspaceID, membership.MemberID)
				if err == nil && ok {
					agent = strings.TrimSpace(worker.Name)
				}
			}
			if agent == "" {
				continue
			}
			hb, configured := tm.heartbeatForTrigger(project, agent)
			if !configured || hb == nil || hb.Paused {
				continue
			}
			task, ok := tm.nextDueScheduledTask(project, agent, now)
			if !ok {
				continue
			}
			tm.fireWakeup(project, agent, hb, entity.TriggerOnTask, "scheduled task due: "+task.ID)
		}
	}
}

func (tm *triggerManager) nextDueScheduledTask(project, agent string, now time.Time) (*entity.Task, bool) {
	tasks, err := tm.ts.ListTasks(project, agent, entity.TaskStatusPending)
	if err != nil {
		return nil, false
	}
	var best *entity.Task
	for _, task := range tasks {
		if task == nil || task.NotBefore == nil || task.NotBefore.After(now) {
			continue
		}
		if best == nil ||
			task.NotBefore.Before(*best.NotBefore) ||
			(task.NotBefore.Equal(*best.NotBefore) && (task.Priority < best.Priority ||
				(task.Priority == best.Priority && task.CreatedAt.Before(best.CreatedAt)))) {
			best = task
		}
	}
	return best, best != nil
}

// Fire checks whether the agent has the given trigger configured and, if so,
// launches an asynchronous wakeup. It is safe to call from any goroutine.
// reason is a human-readable label for logging (e.g. "message from pm").
func (tm *triggerManager) Fire(project, agent string, triggerType entity.TriggerType, reason string) {
	hb, configured := tm.heartbeatForTrigger(project, agent)
	if !configured || hb == nil {
		fmt.Fprintf(os.Stderr, "[trigger] %s/%s: skip — no heartbeat or worker schedule\n", project, agent)
		return
	}
	if !hb.HasTrigger(triggerType) {
		fmt.Fprintf(os.Stderr, "[trigger] %s/%s: skip — trigger %q not configured (has: %v)\n", project, agent, triggerType, hb.Triggers)
		return
	}
	if hb.Paused {
		fmt.Fprintf(os.Stderr, "[trigger] %s/%s: skip — paused\n", project, agent)
		return
	}

	tm.fireWakeup(project, agent, hb, triggerType, reason)
}

func (tm *triggerManager) fireWakeup(project, agent string, hb *entity.HeartbeatConfig, triggerType entity.TriggerType, reason string) {
	key := project + "/" + agent

	tm.mu.Lock()
	if _, ok := tm.inflight[key]; ok {
		tm.mu.Unlock()
		fmt.Fprintf(os.Stderr, "[trigger] %s/%s: skip — already inflight\n", project, agent)
		return
	}
	if hb.PID > 0 && hb.LastWakeupStatus == "running" {
		if proc, err := os.FindProcess(hb.PID); err == nil {
			if proc.Signal(syscall.Signal(0)) == nil {
				tm.mu.Unlock()
				fmt.Fprintf(os.Stderr, "[trigger] %s/%s: skip — agent already running (pid=%d)\n", project, agent, hb.PID)
				return
			}
		}
	}
	tm.inflight[key] = time.Now()
	tm.mu.Unlock()

	go func() {
		defer func() {
			tm.mu.Lock()
			delete(tm.inflight, key)
			tm.mu.Unlock()
		}()

		fmt.Fprintf(os.Stderr, "[trigger] %s/%s fired (%s: %s)\n", project, agent, triggerType, reason)

		args := []string{"--dir", tm.root, "scheduler", "wakeup", "--project", project, "--agent", agent}
		cmd := exec.Command(tm.binPath, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		setProcGroup(cmd)
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "[trigger] %s/%s: wakeup command failed: %v\n", project, agent, err)
		}
	}()
}

func (tm *triggerManager) heartbeatForTrigger(project, agent string) (*entity.HeartbeatConfig, bool) {
	if tm == nil {
		return nil, false
	}
	if worker, ok := tm.resolveAgentWorker(project, agent); ok {
		if hb, configured := agentWorkerScheduleHeartbeat(worker); configured {
			return hb, true
		}
	}
	return nil, false
}

func (tm *triggerManager) resolveAgentWorker(project, agent string) (controldb.AgentWorker, bool) {
	if tm == nil || tm.db == nil {
		return controldb.AgentWorker{}, false
	}
	workspaceID := tm.workspaceID()
	if workspaceID == "" {
		return controldb.AgentWorker{}, false
	}
	memberships, err := tm.db.ListProjectMemberships(controldb.ProjectMembershipFilter{
		WorkspaceID: workspaceID,
		ProjectID:   strings.TrimSpace(project),
		MemberType:  "agent_worker",
	})
	if err != nil {
		return controldb.AgentWorker{}, false
	}
	for _, membership := range memberships {
		worker, ok, err := tm.db.AgentWorkerByID(workspaceID, membership.MemberID)
		if err != nil || !ok {
			continue
		}
		if triggerAgentMatches(agent, membership, worker) {
			return worker, true
		}
	}
	return controldb.AgentWorker{}, false
}

func (tm *triggerManager) workspaceID() string {
	if tm == nil || tm.db == nil {
		return ""
	}
	rows, err := tm.db.ListWorkspaces()
	if err != nil {
		return ""
	}
	for _, row := range rows {
		if samePath(row.Root, tm.root) {
			return strings.TrimSpace(row.ID)
		}
	}
	return workspaceID(tm.root)
}

func triggerAgentMatches(agent string, membership controldb.ProjectMembership, worker controldb.AgentWorker) bool {
	want := strings.TrimSpace(strings.ToLower(agent))
	if want == "" {
		return false
	}
	for _, value := range []string{membership.Title, membership.Role, worker.Name, worker.DisplayName, worker.ID, membership.ID} {
		if strings.TrimSpace(strings.ToLower(value)) == want {
			return true
		}
	}
	return false
}

func agentWorkerScheduleHeartbeat(worker controldb.AgentWorker) (*entity.HeartbeatConfig, bool) {
	raw := strings.TrimSpace(worker.ScheduleJSON)
	if raw == "" || raw == "{}" {
		return nil, false
	}
	var hb entity.HeartbeatConfig
	if err := json.Unmarshal([]byte(raw), &hb); err != nil {
		return nil, false
	}
	return &hb, len(hb.Triggers) > 0
}
