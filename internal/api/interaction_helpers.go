package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/multigent/multigent/internal/interaction"
)

func (s *Server) interactionAgentRef(workspaceID, project, agent string) interaction.AgentRef {
	return interaction.AgentRef{
		WorkspaceID: workspaceID,
		ProjectID:   project,
		AgentID:     agent,
	}
}

func (s *Server) acquireAgentInteraction(w http.ResponseWriter, agent interaction.AgentRef, source interaction.Source, reason string) (*interaction.Lease, bool) {
	if s.interactions == nil {
		s.interactions = interaction.NewManager()
	}
	session, lease, err := s.interactions.Acquire(agent, source, reason)
	if err == nil {
		return lease, true
	}
	if errors.Is(err, interaction.ErrAgentLocked) {
		s.jsonError(w, http.StatusConflict, fmt.Sprintf("agent is busy in %s session from %s", session.Source.Kind, session.Source.Channel))
		return nil, false
	}
	s.serverError(w, err)
	return nil, false
}
