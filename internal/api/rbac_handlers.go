package api

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleRBACModel(w http.ResponseWriter, _ *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"scopes": []map[string]any{
			{"name": "workspace", "roles": []string{"owner", "admin", "member", "guest"}},
			{"name": "team", "roles": []string{"owner", "member", "viewer"}},
			{"name": "project", "roles": []string{"manager", "operator", "viewer"}},
			{"name": "task", "roles": []string{"owner", "assignee", "reviewer", "viewer"}},
			{"name": "agent", "roles": []string{"owner", "operator", "viewer"}},
			{"name": "context_pack", "roles": []string{"maintainer", "contributor", "viewer"}},
			{"name": "worker", "roles": []string{"admin", "operator", "viewer"}},
		},
		"capabilities": []map[string]string{
			{"id": "workspace.manage_members", "scope": "workspace", "label": "Manage workspace members"},
			{"id": "workspace.manage_billing", "scope": "workspace", "label": "Manage billing"},
			{"id": "team.manage", "scope": "team", "label": "Manage teams"},
			{"id": "project.create", "scope": "project", "label": "Create project"},
			{"id": "project.read", "scope": "project", "label": "Read project"},
			{"id": "project.manage", "scope": "project", "label": "Manage project"},
			{"id": "project.manage_members", "scope": "project", "label": "Manage project members"},
			{"id": "task.create", "scope": "task", "label": "Create task"},
			{"id": "task.read", "scope": "task", "label": "Read task"},
			{"id": "task.update", "scope": "task", "label": "Update task"},
			{"id": "task.assign", "scope": "task", "label": "Assign task"},
			{"id": "task.review", "scope": "task", "label": "Review task"},
			{"id": "agent.create", "scope": "agent", "label": "Create or import agent"},
			{"id": "agent.assign_owner", "scope": "agent", "label": "Assign agent owner"},
			{"id": "agent.view", "scope": "agent", "label": "View agent"},
			{"id": "agent.run", "scope": "agent", "label": "Run agent"},
			{"id": "agent.pause", "scope": "agent", "label": "Pause agent"},
			{"id": "agent.edit_prompt", "scope": "agent", "label": "Edit agent prompt"},
			{"id": "agent.approve_memory", "scope": "agent", "label": "Approve agent memory"},
			{"id": "context.read", "scope": "context_pack", "label": "Read context"},
			{"id": "context.write", "scope": "context_pack", "label": "Write context"},
			{"id": "context.promote_memory", "scope": "context_pack", "label": "Promote memory"},
			{"id": "worker.register", "scope": "worker", "label": "Register worker"},
			{"id": "worker.dispatch", "scope": "worker", "label": "Dispatch worker job"},
			{"id": "integration.configure", "scope": "integration", "label": "Configure integration"},
		},
	})
}
