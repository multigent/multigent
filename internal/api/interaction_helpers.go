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
	ref := interaction.AgentRef{
		WorkspaceID: workspaceID,
		ProjectID:   project,
		AgentID:     agent,
	}
	if s != nil && s.agentDirectory != nil {
		if resolved, ok, err := s.agentDirectory.ResolveLegacyMailbox(workspaceID, project+"/"+agent); err == nil && ok {
			ref.AgentWorkerID = resolved.Worker.ID
		}
	}
	return ref
}

func (s *Server) acquireAgentInteraction(w http.ResponseWriter, agent interaction.AgentRef, source interaction.Source, reason string) (*apiInteractionLease, bool) {
	lease, err := s.acquireAgentInteractionLease(agent, source, reason)
	if err == nil {
		return lease, true
	}
	if errors.Is(err, interaction.ErrAgentLocked) {
		var active controldb.InteractionSession
		if strings.TrimSpace(agent.AgentWorkerID) != "" {
			active, _, _ = s.controlDB.ActiveInteractionSessionForWorkerSource(agent.WorkspaceID, agent.AgentWorkerID, source.Kind, source.Channel, source.ActorID)
		} else {
			active, _, _ = s.controlDB.ActiveInteractionSessionForSource(agent.WorkspaceID, agent.ProjectID, agent.AgentID, source.Kind, source.Channel, source.ActorID)
		}
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
	record.RuntimeSessionID = s.latestRuntimeSessionForInteractionSource(agent, source)
	if err := s.controlDB.CreateInteractionSession(record); err != nil {
		lease.Release()
		var active controldb.InteractionSession
		var found bool
		var lookupErr error
		if strings.TrimSpace(agent.AgentWorkerID) != "" {
			active, found, lookupErr = s.controlDB.ActiveInteractionSessionForWorkerSource(agent.WorkspaceID, agent.AgentWorkerID, source.Kind, source.Channel, source.ActorID)
		} else {
			active, found, lookupErr = s.controlDB.ActiveInteractionSessionForSource(agent.WorkspaceID, agent.ProjectID, agent.AgentID, source.Kind, source.Channel, source.ActorID)
		}
		if lookupErr == nil && found {
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
		WorkspaceID:   session.WorkspaceID,
		AgentWorkerID: session.AgentWorkerID,
		ProjectID:     session.ProjectID,
		AgentID:       session.AgentID,
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

func (l *apiInteractionLease) SetRuntimeSessionID(runtimeSessionID string) {
	runtimeSessionID = strings.TrimSpace(runtimeSessionID)
	if l == nil || l.done || runtimeSessionID == "" || runtimeSessionID == l.session.RuntimeSessionID {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	l.session.RuntimeSessionID = runtimeSessionID
	l.session.UpdatedAt = now
	l.session.LastActivityAt = now
	_ = l.server.controlDB.UpdateInteractionSession(l.session)
	_ = l.server.createInteractionEvent(l.session, "system", "", l.session.SourceChannel, "runtime_session_updated", "", map[string]any{
		"runtimeSessionId": runtimeSessionID,
	})
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
		AgentWorkerID:  session.AgentWorkerID,
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

func (s *Server) latestRuntimeSessionForInteractionSource(agent interaction.AgentRef, source interaction.Source) string {
	if s == nil || s.controlDB == nil {
		return ""
	}
	filter := controldb.InteractionSessionFilter{
		WorkspaceID:    agent.WorkspaceID,
		AgentWorkerID:  agent.AgentWorkerID,
		RuntimeSession: true,
		Limit:          1,
	}
	if strings.TrimSpace(agent.AgentWorkerID) == "" {
		filter.ProjectID = agent.ProjectID
		filter.AgentID = agent.AgentID
		filter.SourceKind = strings.TrimSpace(source.Kind)
		filter.SourceChannel = strings.TrimSpace(source.Channel)
		filter.ActorID = strings.TrimSpace(source.ActorID)
	}
	sessions, err := s.controlDB.ListInteractionSessions(filter)
	if err != nil || len(sessions) == 0 {
		return ""
	}
	return strings.TrimSpace(sessions[0].RuntimeSessionID)
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
