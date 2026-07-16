package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/interaction"
)

type apiInteractionLease struct {
	server  *Server
	lease   *interaction.Lease
	session controldb.InteractionSession
	done    bool
}

func (s *Server) interactionAgentRef(workspaceID, project, agent string) interaction.AgentRef {
	return interaction.AgentRef{
		WorkspaceID: workspaceID,
		ProjectID:   project,
		AgentID:     agent,
	}
}

func (s *Server) acquireAgentInteraction(w http.ResponseWriter, agent interaction.AgentRef, source interaction.Source, reason string) (*apiInteractionLease, bool) {
	lease, err := s.acquireAgentInteractionLease(agent, source, reason)
	if err == nil {
		return lease, true
	}
	if errors.Is(err, interaction.ErrAgentLocked) {
		active, _, _ := s.controlDB.ActiveInteractionSession(agent.WorkspaceID, agent.ProjectID, agent.AgentID)
		s.jsonError(w, http.StatusConflict, fmt.Sprintf("agent is busy in %s session from %s", active.SourceKind, active.SourceChannel))
		return nil, false
	}
	s.serverError(w, err)
	return nil, false
}

func (s *Server) acquireAgentInteractionLease(agent interaction.AgentRef, source interaction.Source, reason string) (*apiInteractionLease, error) {
	if s.interactions == nil {
		s.interactions = interaction.NewManager()
	}
	session, lease, err := s.interactions.Acquire(agent, source, reason)
	if err != nil {
		return nil, err
	}
	record := interactionSessionRecord(session, source)
	if err := s.controlDB.CreateInteractionSession(record); err != nil {
		lease.Release()
		if active, found, lookupErr := s.controlDB.ActiveInteractionSession(agent.WorkspaceID, agent.ProjectID, agent.AgentID); lookupErr == nil && found {
			syncInteractionSession(s.interactions, active)
			return nil, interaction.ErrAgentLocked
		}
		return nil, err
	}
	_ = s.createInteractionEvent(record, "system", source.ActorID, source.Channel, "session_acquired", "", map[string]any{
		"sourceKind":    source.Kind,
		"sourceChannel": source.Channel,
		"lockReason":    record.LockReason,
	})
	return &apiInteractionLease{server: s, lease: lease, session: record}, nil
}

func syncInteractionSession(manager *interaction.Manager, session controldb.InteractionSession) {
	if manager == nil {
		return
	}
	_, _, _ = manager.Acquire(interaction.AgentRef{
		WorkspaceID: session.WorkspaceID,
		ProjectID:   session.ProjectID,
		AgentID:     session.AgentID,
	}, interaction.Source{
		Kind:    session.SourceKind,
		ActorID: session.ActorID,
		Channel: session.SourceChannel,
	}, session.LockReason)
}

func (l *apiInteractionLease) Release() {
	if l == nil || l.done {
		return
	}
	l.done = true
	now := time.Now().UTC().Format(time.RFC3339)
	l.session.Status = "completed"
	l.session.UpdatedAt = now
	l.session.LastActivityAt = now
	l.session.CompletedAt = now
	_ = l.server.controlDB.UpdateInteractionSession(l.session)
	_ = l.server.createInteractionEvent(l.session, "system", "", l.session.SourceChannel, "session_released", "", nil)
	if l.lease != nil {
		l.lease.Release()
	}
}

func (l *apiInteractionLease) Fail(reason string) {
	if l == nil || l.done {
		return
	}
	l.done = true
	now := time.Now().UTC().Format(time.RFC3339)
	l.session.Status = "failed"
	l.session.UpdatedAt = now
	l.session.LastActivityAt = now
	l.session.CompletedAt = now
	_ = l.server.controlDB.UpdateInteractionSession(l.session)
	_ = l.server.createInteractionEvent(l.session, "system", "", l.session.SourceChannel, "session_failed", reason, nil)
	if l.lease != nil {
		l.lease.Release()
	}
}

func (l *apiInteractionLease) SessionID() string {
	if l == nil {
		return ""
	}
	return l.session.ID
}

func interactionSessionRecord(session interaction.Session, source interaction.Source) controldb.InteractionSession {
	now := session.CreatedAt.UTC().Format(time.RFC3339)
	actorType := "user"
	if strings.TrimSpace(source.ActorID) == "" {
		actorType = "system"
	}
	return controldb.InteractionSession{
		ID:             session.ID,
		WorkspaceID:    session.WorkspaceID,
		ProjectID:      session.ProjectID,
		AgentID:        session.AgentID,
		SourceKind:     strings.TrimSpace(source.Kind),
		SourceChannel:  strings.TrimSpace(source.Channel),
		ActorType:      actorType,
		ActorID:        strings.TrimSpace(source.ActorID),
		Status:         "active",
		LockReason:     session.LockReason,
		MetadataJSON:   "{}",
		CreatedAt:      now,
		UpdatedAt:      now,
		LastActivityAt: now,
	}
}

func (s *Server) createInteractionEvent(session controldb.InteractionSession, actorType, actorID, channel, eventType, content string, metadata map[string]any) error {
	if metadata == nil {
		metadata = map[string]any{}
	}
	raw, _ := json.Marshal(metadata)
	return s.controlDB.CreateInteractionEvent(controldb.InteractionEvent{
		ID:           newInteractionEventID(),
		SessionID:    session.ID,
		WorkspaceID:  session.WorkspaceID,
		ActorType:    strings.TrimSpace(actorType),
		ActorID:      strings.TrimSpace(actorID),
		Channel:      strings.TrimSpace(channel),
		EventType:    strings.TrimSpace(eventType),
		Content:      content,
		MetadataJSON: string(raw),
	})
}

func newInteractionEventID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("evt-%d", time.Now().UnixNano())
	}
	return "evt-" + hex.EncodeToString(b[:])
}
