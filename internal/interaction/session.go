package interaction

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var ErrAgentLocked = errors.New("agent session is locked")

type Source struct {
	Kind    string
	ActorID string
	Channel string
}

type AgentRef struct {
	WorkspaceID string
	ProjectID   string
	AgentID     string
}

type Session struct {
	ID             string
	WorkspaceID    string
	ProjectID      string
	AgentID        string
	Source         Source
	LockReason     string
	CreatedAt      time.Time
	LastActivityAt time.Time
}

type Manager struct {
	mu       sync.Mutex
	active   map[string]Session
	nextIDFn func() string
}

type Lease struct {
	manager *Manager
	key     string
	id      string
	once    sync.Once
}

func NewManager() *Manager {
	return &Manager{
		active: make(map[string]Session),
		nextIDFn: func() string {
			return fmt.Sprintf("sess-%d", time.Now().UnixNano())
		},
	}
}

func (m *Manager) Acquire(agent AgentRef, source Source, reason string) (Session, *Lease, error) {
	if m == nil {
		return Session{}, nil, fmt.Errorf("interaction manager is nil")
	}
	key := agentKey(agent)
	if key == "" {
		return Session{}, nil, fmt.Errorf("agent ref is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.active[key]; ok {
		return existing, nil, ErrAgentLocked
	}
	now := time.Now().UTC()
	session := Session{
		ID:             m.nextIDFn(),
		WorkspaceID:    strings.TrimSpace(agent.WorkspaceID),
		ProjectID:      strings.TrimSpace(agent.ProjectID),
		AgentID:        strings.TrimSpace(agent.AgentID),
		Source:         source,
		LockReason:     strings.TrimSpace(reason),
		CreatedAt:      now,
		LastActivityAt: now,
	}
	if session.LockReason == "" {
		session.LockReason = "interactive"
	}
	m.active[key] = session
	return session, &Lease{manager: m, key: key, id: session.ID}, nil
}

func (m *Manager) Status(agent AgentRef) (Session, bool) {
	if m == nil {
		return Session{}, false
	}
	key := agentKey(agent)
	if key == "" {
		return Session{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.active[key]
	return session, ok
}

func (m *Manager) ForceRelease(agent AgentRef) bool {
	if m == nil {
		return false
	}
	key := agentKey(agent)
	if key == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.active[key]; !ok {
		return false
	}
	delete(m.active, key)
	return true
}

func (l *Lease) Release() {
	if l == nil || l.manager == nil {
		return
	}
	l.once.Do(func() {
		l.manager.mu.Lock()
		defer l.manager.mu.Unlock()
		if session, ok := l.manager.active[l.key]; ok && session.ID == l.id {
			delete(l.manager.active, l.key)
		}
	})
}

func agentKey(agent AgentRef) string {
	workspace := strings.TrimSpace(agent.WorkspaceID)
	project := strings.TrimSpace(agent.ProjectID)
	name := strings.TrimSpace(agent.AgentID)
	if workspace == "" || project == "" || name == "" {
		return ""
	}
	return workspace + "/" + project + "/" + name
}
