package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/multigent/multigent/internal/entity"
)

type msgWithMailbox struct {
	msg     *entity.Message
	mailbox string
}

func (s *Server) handleWorkbenchMessages(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	archivedMode := strings.TrimSpace(strings.ToLower(q.Get("archived")))
	if archivedMode == "" {
		archivedMode = "no"
	}
	readFilter := strings.TrimSpace(strings.ToLower(q.Get("read")))
	if readFilter == "" {
		readFilter = "all"
	}
	fromQ := strings.TrimSpace(q.Get("from"))
	direction := strings.TrimSpace(strings.ToLower(q.Get("direction")))
	if direction == "" {
		direction = "all"
	}

	cur := s.currentUser(r)
	if cur == nil || strings.TrimSpace(cur.Username) == "" {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeAuthenticatedUserRequired, "authenticated user required")
		return
	}
	isAdmin := cur.Role == RoleAdmin || s.canAdminCurrentWorkspace(r)
	useAll := archivedMode == "all" || archivedMode == "yes"
	seen := map[string]bool{}
	var msgs []*msgWithMailbox

	addMailbox := func(mailbox string, keep func(*entity.Message) bool) error {
		var raw []*entity.Message
		var err error
		if useAll {
			raw, err = s.ts.ListAllMessages(mailbox)
		} else {
			raw, err = s.ts.ListMessages(mailbox)
		}
		if err != nil {
			return err
		}
		for _, m := range raw {
			if m == nil || (keep != nil && !keep(m)) {
				continue
			}
			key := mailbox + "\x00" + m.ID
			if seen[key] {
				continue
			}
			seen[key] = true
			msgs = append(msgs, &msgWithMailbox{m, mailbox})
		}
		return nil
	}

	if cur.Username != "" && (direction == "inbox" || direction == "all") {
		if err := addMailbox(cur.Username, nil); err != nil {
			s.serverError(w, err)
			return
		}
	}

	if isAdmin {
		// Admin: show all messages to/from human
		if direction == "inbox" || direction == "all" {
			if err := addMailbox("human", nil); err != nil {
				s.serverError(w, err)
				return
			}
		}
		if direction == "sent" || direction == "all" {
			projects, err := s.ts.ListProjects()
			if err != nil {
				s.serverError(w, err)
				return
			}
			workspaceID, err := s.currentWorkspaceID()
			if err != nil {
				s.serverError(w, err)
				return
			}
			for _, proj := range projects {
				agents, err := s.projectAgentNames(workspaceID, proj)
				if err != nil {
					continue
				}
				for _, ag := range agents {
					mailbox := proj + "/" + ag
					_ = addMailbox(mailbox, func(m *entity.Message) bool {
						return m.From == "human" || m.From == cur.Username
					})
				}
			}
		}
	} else {
		for _, grant := range cur.AgentGrants {
			la := grant.Project + "/" + grant.Agent
			if direction == "inbox" || direction == "all" {
				_ = addMailbox(la, nil)
			}
			if direction == "sent" || direction == "all" {
				_ = addMailbox(la, func(m *entity.Message) bool {
					return m.From == cur.Username
				})
			}
		}
	}

	rows := make([]msgRow, 0, len(msgs))
	for _, mw := range msgs {
		m := mw.msg
		if !messagePassesFilters(m, archivedMode, readFilter, fromQ, "") {
			continue
		}
		sent := m.SentAt.UTC()
		var read *time.Time
		if m.ReadAt != nil {
			t := m.ReadAt.UTC()
			read = &t
		}
		var arch *time.Time
		if m.ArchivedAt != nil {
			t := m.ArchivedAt.UTC()
			arch = &t
		}
		rows = append(rows, msgRow{
			ID:         m.ID,
			From:       m.From,
			To:         m.To,
			Subject:    m.Subject,
			Body:       m.Body,
			SentAt:     sent,
			ReadAt:     read,
			ArchivedAt: arch,
			Mailbox:    mw.mailbox,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].SentAt.After(rows[j].SentAt) })
	_ = json.NewEncoder(w).Encode(rows)
}

func (s *Server) handleWorkbenchTasks(w http.ResponseWriter, r *http.Request) {
	projects, err := s.ts.ListProjects()
	if err != nil {
		s.serverError(w, err)
		return
	}
	q := r.URL.Query()
	statusFilters := parseTaskStatusFilters(q["status"])
	projectFilter := strings.TrimSpace(q.Get("project"))
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}

	cur := s.currentUser(r)

	isWakeup := func(t *entity.Task) bool {
		return strings.HasPrefix(t.Title, "[wakeup]") || t.Type == "wakeup"
	}

	linkedSet := map[string]bool{}
	for _, grant := range cur.AgentGrants {
		linkedSet[grant.Project+"/"+grant.Agent] = true
	}

	rows := make([]taskRow, 0)
	for _, proj := range projects {
		if projectFilter != "" && proj != projectFilter {
			continue
		}
		agents, err := s.projectAgentNames(workspaceID, proj)
		if err != nil {
			continue
		}
		for _, ag := range agents {
			active, _ := s.ts.ListTasks(proj, ag)
			archived, _ := s.ts.ListArchivedTasks(proj, ag)
			all := append(active, archived...)
			seenTask := map[string]bool{}
			for _, t := range all {
				if t == nil || isWakeup(t) {
					continue
				}
				if seenTask[t.ID] {
					continue
				}
				seenTask[t.ID] = true
				if cur.Role == RoleAdmin || s.canAdminCurrentWorkspace(r) {
					if ag != "human" && t.Assignee != "human" && t.Assignee != cur.Username {
						continue
					}
				} else {
					if t.Assignee != cur.Username && !linkedSet[t.Assignee] {
						continue
					}
				}
				if len(statusFilters) > 0 && !statusFilters[string(t.Status)] {
					continue
				}
				isArchived := !containsTask(active, t.ID)
				rows = append(rows, s.taskToRowWithWorkflow(workspaceID, t, proj, ag, isArchived))
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].UpdatedAt.After(rows[j].UpdatedAt) })
	_ = json.NewEncoder(w).Encode(rows)
}

func containsTask(tasks []*entity.Task, id string) bool {
	for _, t := range tasks {
		if t != nil && t.ID == id {
			return true
		}
	}
	return false
}

func parseTaskStatusFilters(values []string) map[string]bool {
	out := map[string]bool{}
	for _, raw := range values {
		for _, part := range strings.Split(raw, ",") {
			status := strings.TrimSpace(part)
			if status != "" {
				out[status] = true
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ── Project overview for workbench ──────────────────────────────────────────

type projectOverview struct {
	Project          string `json:"project"`
	AgentCount       int    `json:"agentCount"`
	HeartbeatEnabled int    `json:"heartbeatEnabled"`
	RunningAgents    int    `json:"runningAgents"`
	SchedulerRunning bool   `json:"schedulerRunning"`
	PendingTasks     int    `json:"pendingTasks"`
	RunningTasks     int    `json:"runningTasks"`
	CompletedTasks   int    `json:"completedTasks"`
	TotalTasks       int    `json:"totalTasks"`
	UnreadMessages   int    `json:"unreadMessages"`
	TotalMessages    int    `json:"totalMessages"`
}

func (s *Server) handleWorkbenchOverview(w http.ResponseWriter, r *http.Request) {
	projects, err := s.ts.ListProjects()
	if err != nil {
		s.serverError(w, err)
		return
	}

	schedStatuses := s.sched.Status()
	schedRunning := map[string]bool{}
	for _, ss := range schedStatuses {
		if ss.Running {
			schedRunning[ss.Key] = true
		}
	}

	rows := make([]projectOverview, 0, len(projects))
	for _, proj := range projects {
		if !s.canAccessProject(r, proj) {
			continue
		}
		workspaceID, err := s.currentWorkspaceID()
		if err != nil {
			s.serverError(w, err)
			return
		}
		agentNames, err := s.projectAgentNames(workspaceID, proj)
		if err != nil {
			continue
		}

		ov := projectOverview{Project: proj, AgentCount: len(agentNames)}

		for _, ag := range agentNames {
			target := s.runtimeSchedulerTargetForProjectAgent(workspaceID, proj, ag)
			hb, err := s.loadSchedulerTargetHeartbeat(workspaceID, target)
			if err == nil {
				if hb.Enabled {
					ov.HeartbeatEnabled++
				}
				if hb.LastWakeupStatus == "running" && hb.PID > 0 {
					ov.RunningAgents++
				}
			}
		}

		if schedRunning["all"] || schedRunning[proj] {
			ov.SchedulerRunning = true
		} else {
			for _, ag := range agentNames {
				if schedRunning[proj+"/"+ag] {
					ov.SchedulerRunning = true
					break
				}
			}
		}

		isWakeup := func(t *entity.Task) bool {
			return strings.HasPrefix(t.Title, "[wakeup]") || t.Type == "wakeup"
		}
		for _, ag := range agentNames {
			tasks, _ := s.ts.ListTasks(proj, ag)
			for _, t := range tasks {
				if t == nil || isWakeup(t) {
					continue
				}
				ov.TotalTasks++
				switch {
				case t.Status == entity.TaskStatusPending:
					ov.PendingTasks++
				case t.Status == entity.TaskStatusInProgress:
					ov.RunningTasks++
				case t.Status.IsTerminal():
					ov.CompletedTasks++
				}
			}
		}

		for _, ag := range agentNames {
			mailbox := proj + "/" + ag
			msgs, _ := s.ts.ListMessages(mailbox)
			for _, m := range msgs {
				if m == nil {
					continue
				}
				ov.TotalMessages++
				if m.ReadAt == nil {
					ov.UnreadMessages++
				}
			}
		}

		rows = append(rows, ov)
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].Project < rows[j].Project })
	_ = json.NewEncoder(w).Encode(rows)
}
