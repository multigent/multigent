package api

import (
	"fmt"
	"strings"
)

// validateIdentity checks identity is a workspace user, "human" legacy alias,
// or an existing "project/agent".
func (s *Server) validateIdentity(identity, fieldName string) error {
	identity = strings.TrimSpace(identity)
	if identity == "human" {
		return nil
	}
	parts := strings.SplitN(identity, "/", 2)
	if len(parts) == 1 {
		workspaceID, err := s.currentWorkspaceID()
		if err != nil {
			return fmt.Errorf("resolve workspace for %s: %w", fieldName, err)
		}
		if _, ok, err := s.controlDB.WorkspaceMember(workspaceID, identity); err != nil {
			return fmt.Errorf("check workspace member %q: %w", identity, err)
		} else if ok {
			return nil
		}
		return fmt.Errorf("workspace user %q not found", identity)
	}
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("invalid %s: expected workspace user or project/agent", fieldName)
	}
	project, agentName := parts[0], parts[1]
	agents, err := s.st.ListAgents(project)
	if err != nil {
		return fmt.Errorf("list agents for %s: %w", project, err)
	}
	for _, a := range agents {
		if a.Name == agentName {
			return nil
		}
	}
	return fmt.Errorf("agent %q not found in project %q", agentName, project)
}

func (s *Server) agentExistsInProject(project, agentName string) bool {
	agents, err := s.st.ListAgents(project)
	if err != nil {
		return false
	}
	for _, a := range agents {
		if a.Name == agentName {
			return true
		}
	}
	return false
}
