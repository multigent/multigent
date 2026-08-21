package api

import (
	"fmt"
	"strings"
)

type runtimeContactRow struct {
	Type        string `json:"type"`
	Identity    string `json:"identity"`
	DisplayName string `json:"displayName,omitempty"`
	Email       string `json:"email,omitempty"`
	Role        string `json:"role,omitempty"`
	Project     string `json:"project,omitempty"`
	Agent       string `json:"agent,omitempty"`
}

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
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		return fmt.Errorf("resolve workspace for %s: %w", fieldName, err)
	}
	if s.agentDirectory != nil {
		if _, ok, err := s.agentDirectory.ResolveLegacyMailbox(workspaceID, identity); err != nil {
			return fmt.Errorf("resolve agent worker %q: %w", identity, err)
		} else if ok {
			return nil
		}
	}
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

func (s *Server) runtimeContacts(workspaceID, project string) ([]runtimeContactRow, error) {
	rows := make([]runtimeContactRow, 0)
	if s.controlDB != nil {
		members, err := s.controlDB.ListWorkspaceMembers(workspaceID)
		if err != nil {
			return nil, err
		}
		for _, member := range members {
			if strings.TrimSpace(member.Username) == "" {
				continue
			}
			row := runtimeContactRow{
				Type:     "user",
				Identity: member.Username,
				Role:     member.Role,
			}
			if s.users != nil {
				if u := s.users.GetUser(member.Username); u != nil {
					row.DisplayName = strings.TrimSpace(u.DisplayName)
					row.Email = strings.TrimSpace(u.Email)
				}
			}
			rows = append(rows, row)
		}
	}
	if strings.TrimSpace(project) != "" && s.st != nil {
		agents, err := s.projectAgentNames(workspaceID, project)
		if err != nil {
			return nil, err
		}
		for _, agentName := range agents {
			agentName = strings.TrimSpace(agentName)
			if agentName == "" {
				continue
			}
			rows = append(rows, runtimeContactRow{
				Type:        "agent",
				Identity:    project + "/" + agentName,
				DisplayName: agentName,
				Project:     project,
				Agent:       agentName,
			})
		}
	}
	return rows, nil
}

func (s *Server) resolveRuntimeRecipient(principal runtimeAgentPrincipal, input string) (string, error) {
	recipient := strings.TrimSpace(input)
	if recipient == "" {
		return "", fmt.Errorf("recipient is required")
	}
	if recipient == "human" {
		return recipient, nil
	}
	if err := s.validateRuntimeRecipient(principal, recipient); err == nil {
		return recipient, nil
	}
	candidates := []string{recipient}
	if strings.HasSuffix(recipient, ")") {
		if start := strings.LastIndex(recipient, "("); start >= 0 && start < len(recipient)-1 {
			candidates = append(candidates, strings.TrimSpace(recipient[start+1:len(recipient)-1]))
		}
	}
	contacts, err := s.runtimeContacts(principal.WorkspaceID, principal.Project)
	if err != nil {
		return "", fmt.Errorf("list contacts: %w", err)
	}
	var matches []runtimeContactRow
	for _, candidate := range candidates {
		needle := strings.ToLower(strings.TrimSpace(candidate))
		if needle == "" {
			continue
		}
		for _, contact := range contacts {
			values := []string{contact.Identity, contact.DisplayName, contact.Email}
			for _, value := range values {
				if strings.ToLower(strings.TrimSpace(value)) == needle {
					matches = append(matches, contact)
					break
				}
			}
		}
		if len(matches) > 0 {
			break
		}
	}
	if len(matches) == 1 {
		if err := s.validateRuntimeRecipient(principal, matches[0].Identity); err != nil {
			return "", err
		}
		return matches[0].Identity, nil
	}
	if len(matches) > 1 {
		ids := make([]string, 0, len(matches))
		for _, match := range matches {
			ids = append(ids, match.Identity)
		}
		return "", fmt.Errorf("recipient %q is ambiguous; use one of: %s", input, strings.Join(ids, ", "))
	}
	suggestions := suggestRuntimeContacts(contacts, recipient, 5)
	if len(suggestions) > 0 {
		return "", fmt.Errorf("recipient %q not found; did you mean: %s? Use the contact identity value, or run `mga contacts list`", input, strings.Join(suggestions, "; "))
	}
	return "", fmt.Errorf("recipient %q not found; run `mga contacts list` to inspect valid identities", input)
}

func suggestRuntimeContacts(contacts []runtimeContactRow, query string, limit int) []string {
	needle := strings.ToLower(strings.TrimSpace(query))
	if needle == "" || limit <= 0 {
		return nil
	}
	out := make([]string, 0, limit)
	seen := make(map[string]bool)
	for _, contact := range contacts {
		haystack := strings.ToLower(strings.Join([]string{
			contact.Identity,
			contact.DisplayName,
			contact.Email,
			contact.Project,
			contact.Agent,
		}, " "))
		if !strings.Contains(haystack, needle) || seen[contact.Identity] {
			continue
		}
		seen[contact.Identity] = true
		out = append(out, formatRuntimeContactSuggestion(contact))
		if len(out) >= limit {
			break
		}
	}
	return out
}

func formatRuntimeContactSuggestion(contact runtimeContactRow) string {
	label := strings.TrimSpace(contact.Identity)
	var details []string
	if displayName := strings.TrimSpace(contact.DisplayName); displayName != "" && displayName != label {
		details = append(details, displayName)
	}
	if email := strings.TrimSpace(contact.Email); email != "" && email != label {
		details = append(details, email)
	}
	if len(details) == 0 {
		return label
	}
	return label + " (" + strings.Join(details, ", ") + ")"
}

func (s *Server) agentExistsInProject(project, agentName string) bool {
	if s != nil && s.agentDirectory != nil {
		if workspaceID, err := s.currentWorkspaceID(); err == nil && strings.TrimSpace(workspaceID) != "" {
			if _, ok, err := s.agentDirectory.ProjectWorker(workspaceID, project, agentName); err == nil && ok {
				return true
			}
		}
	}
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
