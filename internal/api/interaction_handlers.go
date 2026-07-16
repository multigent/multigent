package api

import (
	"encoding/json"
	"net/http"

	controldb "github.com/multigent/multigent/internal/db"
)

type interactionStatusResponse struct {
	Active  bool                        `json:"active"`
	Session *interactionSessionResponse `json:"session,omitempty"`
	Events  []interactionEventResponse  `json:"events,omitempty"`
}

type interactionSessionResponse struct {
	ID               string `json:"id"`
	SourceKind       string `json:"sourceKind"`
	SourceChannel    string `json:"sourceChannel,omitempty"`
	ActorType        string `json:"actorType,omitempty"`
	ActorID          string `json:"actorId,omitempty"`
	Status           string `json:"status"`
	LockReason       string `json:"lockReason"`
	RuntimeSessionID string `json:"runtimeSessionId,omitempty"`
	CurrentRunID     string `json:"currentRunId,omitempty"`
	HumanIntervened  bool   `json:"humanIntervened"`
	CreatedAt        string `json:"createdAt,omitempty"`
	UpdatedAt        string `json:"updatedAt,omitempty"`
	LastActivityAt   string `json:"lastActivityAt,omitempty"`
}

type interactionEventResponse struct {
	ID        string `json:"id"`
	ActorType string `json:"actorType"`
	ActorID   string `json:"actorId,omitempty"`
	Channel   string `json:"channel,omitempty"`
	EventType string `json:"eventType"`
	Content   string `json:"content,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
}

func (s *Server) handleAgentInteractionStatus(w http.ResponseWriter, r *http.Request) {
	project, agent, ok := s.parseProjectAgent(w, r)
	if !ok {
		return
	}
	if !s.checkProjectAccess(w, r, project) {
		return
	}
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}
	session, found, err := s.controlDB.ActiveInteractionSession(workspaceID, project, agent)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found {
		_ = json.NewEncoder(w).Encode(interactionStatusResponse{Active: false, Events: []interactionEventResponse{}})
		return
	}
	events, err := s.controlDB.ListInteractionEvents(controldb.InteractionEventFilter{
		WorkspaceID: workspaceID,
		SessionID:   session.ID,
		Limit:       20,
	})
	if err != nil {
		s.serverError(w, err)
		return
	}
	outEvents := make([]interactionEventResponse, 0, len(events))
	for _, event := range events {
		outEvents = append(outEvents, interactionEventToResponse(event))
	}
	respSession := interactionSessionToResponse(session)
	_ = json.NewEncoder(w).Encode(interactionStatusResponse{
		Active:  true,
		Session: &respSession,
		Events:  outEvents,
	})
}

func interactionSessionToResponse(session controldb.InteractionSession) interactionSessionResponse {
	return interactionSessionResponse{
		ID:               session.ID,
		SourceKind:       session.SourceKind,
		SourceChannel:    session.SourceChannel,
		ActorType:        session.ActorType,
		ActorID:          session.ActorID,
		Status:           session.Status,
		LockReason:       session.LockReason,
		RuntimeSessionID: session.RuntimeSessionID,
		CurrentRunID:     session.CurrentRunID,
		HumanIntervened:  session.HumanIntervened,
		CreatedAt:        session.CreatedAt,
		UpdatedAt:        session.UpdatedAt,
		LastActivityAt:   session.LastActivityAt,
	}
}

func interactionEventToResponse(event controldb.InteractionEvent) interactionEventResponse {
	return interactionEventResponse{
		ID:        event.ID,
		ActorType: event.ActorType,
		ActorID:   event.ActorID,
		Channel:   event.Channel,
		EventType: event.EventType,
		Content:   event.Content,
		CreatedAt: event.CreatedAt,
	}
}
