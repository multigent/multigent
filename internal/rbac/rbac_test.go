package rbac

import "testing"

func TestOrgOwnerCanDoEverything(t *testing.T) {
	a := NewAuthorizer()
	p := Principal{ID: "alice", OrgRole: OrgRoleOwner}

	decision := a.HasCapability(p, OrgManageBilling, Resource{Kind: ResourceOrganization})
	if !decision.Allowed {
		t.Fatalf("owner should manage billing: %+v", decision)
	}

	decision = a.HasCapability(p, ContextPromoteMemory, Resource{Kind: ResourceContextPack, ContextPack: "ctx-main"})
	if !decision.Allowed {
		t.Fatalf("owner should promote context memory: %+v", decision)
	}
}

func TestOrgAdminCannotManageBilling(t *testing.T) {
	a := NewAuthorizer()
	p := Principal{ID: "admin", OrgRole: OrgRoleAdmin}

	decision := a.HasCapability(p, OrgManageBilling, Resource{Kind: ResourceOrganization})
	if decision.Allowed {
		t.Fatalf("admin should not manage billing: %+v", decision)
	}

	decision = a.HasCapability(p, AgentCreate, Resource{Kind: ResourceProject, Project: "tapnow"})
	if !decision.Allowed {
		t.Fatalf("admin should create agents: %+v", decision)
	}
}

func TestProjectRolesInheritToTaskAndAgent(t *testing.T) {
	a := NewAuthorizer()
	p := Principal{
		ID:           "pm",
		OrgRole:      OrgRoleMember,
		ProjectRoles: map[string]Role{ProjectKey("tapnow"): ProjectRoleManager},
	}

	decision := a.HasCapability(p, TaskAssign, Resource{
		Kind: ResourceTask, Project: "tapnow", Task: "connector-backend",
	})
	if !decision.Allowed || decision.Reason != "inherited project role" {
		t.Fatalf("project manager should assign project tasks: %+v", decision)
	}

	decision = a.HasCapability(p, AgentEditPrompt, Resource{
		Kind: ResourceAgent, Project: "tapnow", Agent: "backend-connector",
	})
	if !decision.Allowed {
		t.Fatalf("project manager should edit project agent prompt: %+v", decision)
	}
}

func TestAgentOwnerCanCoachAgentWithoutProjectManager(t *testing.T) {
	a := NewAuthorizer()
	p := Principal{
		ID:         "glenn",
		OrgRole:    OrgRoleMember,
		AgentRoles: map[string]Role{AgentKey("tapnow", "connector-dev"): AgentRoleOwner},
	}

	decision := a.HasCapability(p, AgentEditPrompt, Resource{
		Kind: ResourceAgent, Project: "tapnow", Agent: "connector-dev",
	})
	if !decision.Allowed {
		t.Fatalf("agent owner should edit prompt: %+v", decision)
	}

	decision = a.HasCapability(p, AgentDelegateTask, Resource{
		Kind: ResourceAgent, Project: "tapnow", Agent: "connector-dev",
	})
	if !decision.Allowed {
		t.Fatalf("agent owner should delegate agent tasks: %+v", decision)
	}

	decision = a.HasCapability(p, AgentReadRuns, Resource{
		Kind: ResourceAgent, Project: "tapnow", Agent: "connector-dev",
	})
	if !decision.Allowed {
		t.Fatalf("agent owner should read agent runs: %+v", decision)
	}

	decision = a.HasCapability(p, ProjectManageMembers, Resource{
		Kind: ResourceProject, Project: "tapnow",
	})
	if decision.Allowed {
		t.Fatalf("agent owner should not manage project members: %+v", decision)
	}
}

func TestAgentViewerHasNarrowAgentVisibility(t *testing.T) {
	a := NewAuthorizer()
	p := Principal{
		ID:         "viewer",
		OrgRole:    OrgRoleMember,
		AgentRoles: map[string]Role{AgentKey("tapnow", "pm-codex"): AgentRoleViewer},
	}

	decision := a.HasCapability(p, AgentReadProfile, Resource{
		Kind: ResourceAgent, Project: "tapnow", Agent: "pm-codex",
	})
	if !decision.Allowed {
		t.Fatalf("agent viewer should read agent profile: %+v", decision)
	}

	decision = a.HasCapability(p, AgentReadRuns, Resource{
		Kind: ResourceAgent, Project: "tapnow", Agent: "pm-codex",
	})
	if decision.Allowed {
		t.Fatalf("agent viewer should not read agent runs: %+v", decision)
	}

	decision = a.HasCapability(p, AgentMessage, Resource{
		Kind: ResourceAgent, Project: "tapnow", Agent: "pm-codex",
	})
	if decision.Allowed {
		t.Fatalf("agent viewer should not message as operator: %+v", decision)
	}
}

func TestContextRequiresExplicitGrant(t *testing.T) {
	a := NewAuthorizer()
	p := Principal{
		ID:           "dev",
		OrgRole:      OrgRoleMember,
		ProjectRoles: map[string]Role{ProjectKey("tapnow"): ProjectRoleManager},
	}

	decision := a.HasCapability(p, ContextRead, Resource{
		Kind: ResourceContextPack, ContextPack: "customer-contracts",
	})
	if decision.Allowed {
		t.Fatalf("project manager should not read unrelated context without grant: %+v", decision)
	}

	p.ContextRoles = map[string]Role{"customer-contracts": ContextRoleViewer}
	decision = a.HasCapability(p, ContextRead, Resource{
		Kind: ResourceContextPack, ContextPack: "customer-contracts",
	})
	if !decision.Allowed {
		t.Fatalf("context viewer should read context: %+v", decision)
	}
}

func TestCanGrantRoleGuard(t *testing.T) {
	if err := CanGrantRole(ResourceProject, ProjectRoleOperator, ProjectRoleManager); err == nil {
		t.Fatalf("operator should not grant manager")
	}
	if err := CanGrantRole(ResourceProject, ProjectRoleManager, ProjectRoleViewer); err != nil {
		t.Fatalf("manager should grant viewer: %v", err)
	}
}

func TestCannotRemoveLastOrgOwner(t *testing.T) {
	err := CanChangeOrgRole(OrgRoleOwner, OrgRoleOwner, OrgRoleAdmin, 1, true)
	if err == nil {
		t.Fatalf("should not demote last owner")
	}
	err = CanChangeOrgRole(OrgRoleOwner, OrgRoleAdmin, OrgRoleMember, 1, false)
	if err != nil {
		t.Fatalf("owner should demote admin: %v", err)
	}
}
