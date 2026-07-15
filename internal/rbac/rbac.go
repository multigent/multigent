// Package rbac defines Multigent's role and capability model.
//
// The package is intentionally independent from the HTTP API and filesystem
// store. It can be used by the current local-first product and by the future
// SaaS control plane.
package rbac

import "fmt"

// Role is a named role at one resource scope.
type Role string

// Workspace roles.
const (
	WorkspaceRoleOwner  Role = "owner"
	WorkspaceRoleAdmin  Role = "admin"
	WorkspaceRoleMember Role = "member"
	WorkspaceRoleGuest  Role = "guest"

	// Backward-compatible aliases used by the current local API user model.
	OrgRoleOwner  = WorkspaceRoleOwner
	OrgRoleAdmin  = WorkspaceRoleAdmin
	OrgRoleMember = WorkspaceRoleMember
	OrgRoleGuest  = WorkspaceRoleGuest
)

// Project roles. These values are compatible with the current web API.
const (
	ProjectRoleManager  Role = "manager"
	ProjectRoleOperator Role = "operator"
	ProjectRoleViewer   Role = "viewer"
)

// Task roles.
const (
	TaskRoleOwner    Role = "owner"
	TaskRoleAssignee Role = "assignee"
	TaskRoleReviewer Role = "reviewer"
	TaskRoleViewer   Role = "viewer"
)

// Agent roles.
const (
	AgentRoleOwner    Role = "owner"
	AgentRoleOperator Role = "operator"
	AgentRoleViewer   Role = "viewer"
)

// Context pack roles.
const (
	ContextRoleMaintainer  Role = "maintainer"
	ContextRoleContributor Role = "contributor"
	ContextRoleViewer      Role = "viewer"
)

// Worker roles.
const (
	WorkerRoleAdmin    Role = "admin"
	WorkerRoleOperator Role = "operator"
	WorkerRoleViewer   Role = "viewer"
)

// Capability is a stable permission identifier checked by product code.
type Capability string

const (
	WorkspaceManageMembers Capability = "workspace.manage_members"
	WorkspaceManageBilling Capability = "workspace.manage_billing"
	TeamManage             Capability = "team.manage"

	// Backward-compatible aliases used by existing API code.
	OrgManageMembers = WorkspaceManageMembers
	OrgManageBilling = WorkspaceManageBilling

	ProjectCreate        Capability = "project.create"
	ProjectRead          Capability = "project.read"
	ProjectManage        Capability = "project.manage"
	ProjectManageMembers Capability = "project.manage_members"

	TaskCreate Capability = "task.create"
	TaskRead   Capability = "task.read"
	TaskUpdate Capability = "task.update"
	TaskAssign Capability = "task.assign"
	TaskReview Capability = "task.review"

	AgentCreate        Capability = "agent.create"
	AgentAssignOwner   Capability = "agent.assign_owner"
	AgentView          Capability = "agent.view"
	AgentRun           Capability = "agent.run"
	AgentPause         Capability = "agent.pause"
	AgentEditPrompt    Capability = "agent.edit_prompt"
	AgentApproveMemory Capability = "agent.approve_memory"
	AgentMessage       Capability = "agent.message"
	AgentDelegateTask  Capability = "agent.delegate_task"
	AgentReadProfile   Capability = "agent.read_profile"
	AgentReadRuns      Capability = "agent.read_runs"
	AgentReadArtifacts Capability = "agent.read_artifacts"

	ContextRead          Capability = "context.read"
	ContextWrite         Capability = "context.write"
	ContextPromoteMemory Capability = "context.promote_memory"

	WorkerRegister Capability = "worker.register"
	WorkerDispatch Capability = "worker.dispatch"

	IntegrationConfigure Capability = "integration.configure"
)

// ResourceKind identifies the resource being authorized.
type ResourceKind string

const (
	ResourceWorkspace   ResourceKind = "workspace"
	ResourceTeam        ResourceKind = "team"
	ResourceProject     ResourceKind = "project"
	ResourceTask        ResourceKind = "task"
	ResourceAgent       ResourceKind = "agent"
	ResourceContextPack ResourceKind = "context_pack"
	ResourceWorker      ResourceKind = "worker"
	ResourceIntegration ResourceKind = "integration"

	// Backward-compatible alias used by existing API tests and helpers.
	ResourceOrganization = ResourceWorkspace
)

// Resource identifies one protected object.
type Resource struct {
	Kind        ResourceKind
	Team        string
	Project     string
	Task        string
	Agent       string
	ContextPack string
	Worker      string
	Integration string
}

// PrincipalType identifies the kind of actor being authorized.
type PrincipalType string

const (
	PrincipalHuman  PrincipalType = "human"
	PrincipalAgent  PrincipalType = "agent"
	PrincipalWorker PrincipalType = "worker"
	PrincipalSystem PrincipalType = "system"
)

// Principal is the actor being authorized.
//
// Maps are keyed by ProjectKey, TaskKey, AgentKey, ContextPack ID, or
// Worker ID. The authorizer keeps this shape explicit so SaaS persistence can
// hydrate it from database grants without depending on local workspace files.
type Principal struct {
	ID   string
	Type PrincipalType

	System bool

	OrgRole Role

	ProjectRoles map[string]Role
	TaskRoles    map[string]Role
	AgentRoles   map[string]Role
	ContextRoles map[string]Role
	WorkerRoles  map[string]Role
}

// Decision explains the authorization result.
type Decision struct {
	Allowed    bool
	Capability Capability
	Resource   Resource
	Role       Role
	Reason     string
}

// Authorizer evaluates static Multigent RBAC rules.
type Authorizer struct{}

// NewAuthorizer returns the default static authorizer.
func NewAuthorizer() Authorizer {
	return Authorizer{}
}

// ProjectKey returns the stable key for project-scoped grants.
func ProjectKey(project string) string {
	return project
}

// TaskKey returns the stable key for task-scoped grants.
func TaskKey(project, task string) string {
	return project + "/" + task
}

// AgentKey returns the stable key for agent-scoped grants.
func AgentKey(project, agent string) string {
	return project + "/" + agent
}

// HasCapability checks whether principal p may perform capability cap on res.
func (a Authorizer) HasCapability(p Principal, cap Capability, res Resource) Decision {
	if p.System {
		return allow(cap, res, "system actor", "")
	}
	if p.OrgRole == OrgRoleOwner {
		return allow(cap, res, "workspace owner", p.OrgRole)
	}
	if roleHas(orgRoleCapabilities(p.OrgRole), cap) {
		return allow(cap, res, "workspace role", p.OrgRole)
	}

	switch res.Kind {
	case ResourceWorkspace, ResourceTeam:
		return deny(cap, res, "workspace capability not granted")
	case ResourceProject:
		return a.checkProject(p, cap, res)
	case ResourceTask:
		return a.checkTask(p, cap, res)
	case ResourceAgent:
		return a.checkAgent(p, cap, res)
	case ResourceContextPack:
		return a.checkContext(p, cap, res)
	case ResourceWorker:
		return a.checkWorker(p, cap, res)
	case ResourceIntegration:
		return deny(cap, res, "integration capability not granted")
	default:
		return deny(cap, res, "unknown resource kind")
	}
}

func (a Authorizer) checkProject(p Principal, cap Capability, res Resource) Decision {
	role := roleFor(p.ProjectRoles, ProjectKey(res.Project))
	if roleHas(projectRoleCapabilities(role), cap) {
		return allow(cap, res, "project role", role)
	}
	return deny(cap, res, "project capability not granted")
}

func (a Authorizer) checkTask(p Principal, cap Capability, res Resource) Decision {
	role := roleFor(p.TaskRoles, TaskKey(res.Project, res.Task))
	if roleHas(taskRoleCapabilities(role), cap) {
		return allow(cap, res, "task role", role)
	}
	projectRole := roleFor(p.ProjectRoles, ProjectKey(res.Project))
	if roleHas(projectRoleCapabilities(projectRole), cap) {
		return allow(cap, res, "inherited project role", projectRole)
	}
	return deny(cap, res, "task capability not granted")
}

func (a Authorizer) checkAgent(p Principal, cap Capability, res Resource) Decision {
	role := roleFor(p.AgentRoles, AgentKey(res.Project, res.Agent))
	if roleHas(agentRoleCapabilities(role), cap) {
		return allow(cap, res, "agent role", role)
	}
	projectRole := roleFor(p.ProjectRoles, ProjectKey(res.Project))
	if roleHas(projectRoleCapabilities(projectRole), cap) {
		return allow(cap, res, "inherited project role", projectRole)
	}
	return deny(cap, res, "agent capability not granted")
}

func (a Authorizer) checkContext(p Principal, cap Capability, res Resource) Decision {
	role := roleFor(p.ContextRoles, res.ContextPack)
	if roleHas(contextRoleCapabilities(role), cap) {
		return allow(cap, res, "context role", role)
	}
	return deny(cap, res, "context capability not granted")
}

func (a Authorizer) checkWorker(p Principal, cap Capability, res Resource) Decision {
	role := roleFor(p.WorkerRoles, res.Worker)
	if roleHas(workerRoleCapabilities(role), cap) {
		return allow(cap, res, "worker role", role)
	}
	return deny(cap, res, "worker capability not granted")
}

// RolePower returns a comparable privilege level for one role namespace.
func RolePower(scope ResourceKind, role Role) int {
	switch scope {
	case ResourceWorkspace:
		switch role {
		case WorkspaceRoleOwner:
			return 100
		case WorkspaceRoleAdmin:
			return 80
		case WorkspaceRoleMember:
			return 30
		case WorkspaceRoleGuest:
			return 10
		}
	case ResourceProject:
		switch role {
		case ProjectRoleManager:
			return 60
		case ProjectRoleOperator:
			return 40
		case ProjectRoleViewer:
			return 10
		}
	case ResourceTask:
		switch role {
		case TaskRoleOwner:
			return 50
		case TaskRoleAssignee, TaskRoleReviewer:
			return 30
		case TaskRoleViewer:
			return 10
		}
	case ResourceAgent:
		switch role {
		case AgentRoleOwner:
			return 50
		case AgentRoleOperator:
			return 30
		case AgentRoleViewer:
			return 10
		}
	case ResourceContextPack:
		switch role {
		case ContextRoleMaintainer:
			return 50
		case ContextRoleContributor:
			return 30
		case ContextRoleViewer:
			return 10
		}
	case ResourceWorker:
		switch role {
		case WorkerRoleAdmin:
			return 50
		case WorkerRoleOperator:
			return 30
		case WorkerRoleViewer:
			return 10
		}
	}
	return 0
}

// ProjectRolePower keeps compatibility with the existing API helper.
func ProjectRolePower(role Role) int {
	return RolePower(ResourceProject, role)
}

// CanGrantRole reports whether actorRole may grant targetRole within one
// resource scope. Users may not grant a role with greater power than their own.
func CanGrantRole(scope ResourceKind, actorRole, targetRole Role) error {
	actorPower := RolePower(scope, actorRole)
	targetPower := RolePower(scope, targetRole)
	if actorPower == 0 {
		return fmt.Errorf("actor role %q is not valid for %s", actorRole, scope)
	}
	if targetPower == 0 {
		return fmt.Errorf("target role %q is not valid for %s", targetRole, scope)
	}
	if targetPower > actorPower {
		return fmt.Errorf("cannot grant %s role %q above actor role %q", scope, targetRole, actorRole)
	}
	return nil
}

// CanChangeOrgRole checks role mutation guardrails for organization members.
func CanChangeOrgRole(actorRole, currentTargetRole, newTargetRole Role, ownerCount int, targetIsSelf bool) error {
	if actorRole != WorkspaceRoleOwner && actorRole != WorkspaceRoleAdmin {
		return fmt.Errorf("workspace role changes require owner or admin")
	}
	if RolePower(ResourceWorkspace, currentTargetRole) > RolePower(ResourceWorkspace, actorRole) {
		return fmt.Errorf("cannot modify a user with higher workspace role")
	}
	if err := CanGrantRole(ResourceWorkspace, actorRole, newTargetRole); err != nil {
		return err
	}
	if currentTargetRole == WorkspaceRoleOwner && newTargetRole != WorkspaceRoleOwner && ownerCount <= 1 {
		return fmt.Errorf("cannot remove the last workspace owner")
	}
	if targetIsSelf && currentTargetRole == WorkspaceRoleOwner && newTargetRole != WorkspaceRoleOwner && ownerCount <= 1 {
		return fmt.Errorf("cannot demote yourself as the last workspace owner")
	}
	return nil
}

func orgRoleCapabilities(role Role) map[Capability]bool {
	switch role {
	case WorkspaceRoleAdmin:
		return capabilitySet(
			WorkspaceManageMembers, TeamManage,
			ProjectCreate, ProjectRead, ProjectManage, ProjectManageMembers,
			TaskCreate, TaskRead, TaskUpdate, TaskAssign, TaskReview,
			AgentCreate, AgentAssignOwner, AgentView, AgentRun, AgentPause, AgentEditPrompt, AgentApproveMemory,
			ContextRead, ContextWrite, ContextPromoteMemory,
			WorkerRegister, WorkerDispatch,
			IntegrationConfigure,
		)
	case WorkspaceRoleMember:
		return capabilitySet(ProjectCreate, WorkerRegister)
	case WorkspaceRoleGuest:
		return nil
	default:
		return nil
	}
}

func projectRoleCapabilities(role Role) map[Capability]bool {
	switch role {
	case ProjectRoleManager:
		return capabilitySet(
			ProjectRead, ProjectManage, ProjectManageMembers,
			TaskCreate, TaskRead, TaskUpdate, TaskAssign, TaskReview,
			AgentCreate, AgentAssignOwner, AgentView, AgentRun, AgentPause, AgentEditPrompt, AgentApproveMemory,
		)
	case ProjectRoleOperator:
		return capabilitySet(ProjectRead, TaskCreate, TaskRead, TaskUpdate, TaskAssign, AgentView, AgentRun, AgentPause)
	case ProjectRoleViewer:
		return capabilitySet(ProjectRead, TaskRead, AgentView)
	default:
		return nil
	}
}

func taskRoleCapabilities(role Role) map[Capability]bool {
	switch role {
	case TaskRoleOwner:
		return capabilitySet(TaskRead, TaskUpdate, TaskAssign, TaskReview, AgentView, AgentRun, AgentPause)
	case TaskRoleAssignee:
		return capabilitySet(TaskRead, TaskUpdate, AgentView, AgentRun)
	case TaskRoleReviewer:
		return capabilitySet(TaskRead, TaskReview, AgentView)
	case TaskRoleViewer:
		return capabilitySet(TaskRead, AgentView)
	default:
		return nil
	}
}

func agentRoleCapabilities(role Role) map[Capability]bool {
	switch role {
	case AgentRoleOwner:
		return capabilitySet(
			AgentView,
			AgentRun,
			AgentPause,
			AgentEditPrompt,
			AgentApproveMemory,
			AgentMessage,
			AgentDelegateTask,
			AgentReadProfile,
			AgentReadRuns,
			AgentReadArtifacts,
		)
	case AgentRoleOperator:
		return capabilitySet(AgentView, AgentRun, AgentPause, AgentMessage, AgentReadProfile, AgentReadRuns)
	case AgentRoleViewer:
		return capabilitySet(AgentView, AgentReadProfile)
	default:
		return nil
	}
}

func contextRoleCapabilities(role Role) map[Capability]bool {
	switch role {
	case ContextRoleMaintainer:
		return capabilitySet(ContextRead, ContextWrite, ContextPromoteMemory)
	case ContextRoleContributor:
		return capabilitySet(ContextRead, ContextWrite)
	case ContextRoleViewer:
		return capabilitySet(ContextRead)
	default:
		return nil
	}
}

func workerRoleCapabilities(role Role) map[Capability]bool {
	switch role {
	case WorkerRoleAdmin:
		return capabilitySet(WorkerRegister, WorkerDispatch)
	case WorkerRoleOperator:
		return capabilitySet(WorkerDispatch)
	case WorkerRoleViewer:
		return nil
	default:
		return nil
	}
}

func capabilitySet(caps ...Capability) map[Capability]bool {
	out := make(map[Capability]bool, len(caps))
	for _, cap := range caps {
		out[cap] = true
	}
	return out
}

func roleHas(caps map[Capability]bool, cap Capability) bool {
	return caps != nil && caps[cap]
}

func roleFor(roles map[string]Role, key string) Role {
	if roles == nil {
		return ""
	}
	return roles[key]
}

func allow(cap Capability, res Resource, reason string, role Role) Decision {
	return Decision{Allowed: true, Capability: cap, Resource: res, Reason: reason, Role: role}
}

func deny(cap Capability, res Resource, reason string) Decision {
	return Decision{Allowed: false, Capability: cap, Resource: res, Reason: reason}
}
