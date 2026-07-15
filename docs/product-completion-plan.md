# Multigent Product Completion Plan

This document defines the target product journey and the implementation plan for making Multigent a team-grade multi-agent collaboration platform.

Multigent is still in active development. We do not need to preserve legacy behavior. When the current model conflicts with the target product shape, prefer correcting the model directly over adding compatibility layers.

## Product Direction

Multigent should feel like a SaaS product for operating an agent workforce, not like a local folder manager.

Users should think in terms of:

- Workspace
- Team
- Role
- Skill
- Credential / Connector
- Project
- Agent
- Task
- Message
- Run
- Audit event

Users should not need to think in terms of:

- Local workspace root
- Agent working directory
- Markdown/YAML file layout
- Internal database path
- Scheduler process implementation

The binary may still run with a local `--data-dir`, but the Web product should treat storage paths as implementation details.

## Target User Journey

### 1. First Admin Journey

1. Admin registers with email.
2. Admin verifies email.
3. Admin logs in.
4. If the user does not belong to any workspace, show first-run onboarding.
5. User creates a workspace.
6. Creator becomes workspace owner automatically.
7. User can invite teammates by email in bulk.
8. User can skip invites and continue.
9. User enters the workspace home.
10. Home shows an onboarding checklist rather than forcing a strict wizard.

Recommended onboarding checklist:

- Create or review teams.
- Create or review roles.
- Add skills.
- Connect external tools.
- Create a project.
- Hire agents.
- Configure agent wakeup prompts.
- Configure scheduler.
- Send the first task or message.
- Review the first run.

Do not force team creation before project creation. Some teams will start from a project and only later refine teams/roles.

### 2. Invited Member Journey

1. User receives an email invite.
2. User opens invite link.
3. If not registered, user registers first.
4. After registration, user returns to the invite acceptance page.
5. User accepts or rejects the invite.
6. If accepted, user enters the invited workspace.
7. If rejected and the user has no workspace, show the create-workspace onboarding.
8. User sees only projects, agents, tasks, messages, and settings allowed by their role.

Invite creation should support:

- Workspace role.
- Project grants.
- Agent ownership / operation grants.
- Optional display name.
- Expiration and revocation.

### 3. Project And Agent Journey

1. Workspace admin or project manager creates a project.
2. Creator assigns project members and their project roles.
3. Creator hires agents into the project.
4. Each agent gets:
   - Role
   - Human owner / responsible person
   - Runtime provider
   - Sandbox profile
   - Toolchain profile
   - Wakeup prompt
   - Allowed skills
   - Allowed credentials/connectors
   - Scheduler policy
5. Human sends first message or creates first task.
6. Agent runs in isolated sandbox.
7. Human reviews output, run logs, token/cost, task state, and messages.
8. Human can enter a live agent chat/session to guide the agent.
9. Agent behavior improvements are recorded as prompt, skill, policy, or workflow changes.

### 4. Ongoing Workbench Journey

Admin home should be operational:

- Workspace health.
- Active projects.
- Agent health.
- Scheduler state.
- Running/blocked tasks.
- Recent audit events.
- Cost and token trend.

Member home should be personal:

- My messages.
- My tasks.
- Agents I own or operate.
- Projects I can access.
- Runs that need my review.
- Pending approvals assigned to me.

The same workspace can support both views, but the default landing page must depend on the user role and permission grants.

## Target Data Model

### Workspace

Workspace should be the SaaS tenant boundary.

Fields:

- `id` UUID
- `name`
- `slug`
- `description`
- `created_by_user_id`
- `created_at`
- `updated_at`

Rules:

- Creator becomes owner.
- All major resources belong to a workspace.
- Web/API should use `workspace_id`, not filesystem path.
- Internal storage directory may be derived from `workspace_id`.

### Membership

Fields:

- `workspace_id`
- `user_id`
- `role`: owner / admin / member / guest
- `status`: active / invited / suspended
- `created_at`
- `updated_at`

Rules:

- Workspace owner/admin can invite and manage members.
- Workspace role is the first gate.
- Project and agent grants refine access inside the workspace.

### Project

Fields:

- `id` UUID
- `workspace_id`
- `name`
- `description`
- `created_by_user_id`
- `created_at`
- `updated_at`

Rules:

- Web/API should use project IDs where possible.
- Project name is display metadata, not the durable identifier.
- Project members are explicit.
- Users without project access must not see the project.

### Project Member

Fields:

- `project_id`
- `user_id`
- `role`: manager / operator / viewer

Rules:

- Manager can manage project settings, members, agents, and scheduler.
- Operator can create/update tasks and interact with assigned agents.
- Viewer can read project state.

### Agent

Fields:

- `id` UUID
- `workspace_id`
- `project_id`
- `name`
- `role_id`
- `owner_user_id`
- `runtime_profile_id`
- `sandbox_profile_id`
- `toolchain_profile_id`
- `status`
- `created_at`
- `updated_at`

Rules:

- Agent directory should use `agent_id`.
- Display name can change.
- Agent should be treated as a principal for permissions and audit.
- Agent can only access resources explicitly granted to it.

### Credential / Connector

Credentials and connectors are required for real SaaS usage.

Connector examples:

- Feishu / Lark
- DingTalk
- GitHub
- GitLab
- Google Drive
- Notion
- Linear
- Jira
- Plane
- Database
- Cloud provider
- Custom MCP server

Fields:

- `id` UUID
- `workspace_id`
- `provider`
- `owner_type`: workspace / user / project / agent
- `owner_id`
- `display_name`
- `auth_type`: oauth / api_key / token / service_account
- `status`
- `created_at`
- `updated_at`

Rules:

- Store secrets encrypted.
- Do not expose raw secrets to agents.
- Agent access should go through Multigent authorization and injection policy.
- Credential usage must produce audit events.

### Audit Log

Audit log should be a first-class system table.

Fields:

- `id` UUID
- `workspace_id`
- `actor_type`: user / agent / system
- `actor_id`
- `action`
- `resource_type`
- `resource_id`
- `summary`
- `before_json`
- `after_json`
- `ip`
- `user_agent`
- `created_at`

Events to record:

- Registration and login.
- Workspace create/update/switch.
- Invite create/accept/reject/revoke.
- Member role changes.
- Project create/update/delete.
- Project member changes.
- Agent create/update/delete.
- Credential create/update/revoke/use.
- Scheduler start/stop/wakeup/abort.
- Task create/update/delete/archive.
- Message send/archive/delete/read.
- Agent run start/finish/fail.
- Sensitive admin actions.

## Permission Model

Use one consistent model for humans and agents.

Principals:

- User
- Agent
- System

Scopes:

- Workspace
- Project
- Agent
- Task
- Credential
- Connector
- Run

High-level rules:

- Workspace owner/admin can manage workspace-level resources.
- Project manager can manage the project and its scheduler.
- Project operator can execute work inside the project but should not manage scheduler/configuration by default.
- Viewer can read only.
- Agent sees only assigned project, allowed tasks/messages, allowed skills, and allowed credentials.
- Agents must call Multigent APIs for workspace resources; they should not directly access the control database or all workspace files.

## Storage And Path Policy

Target behavior:

- CLI supports `--data-dir`.
- If absent, use a default global data directory.
- Web does not expose filesystem paths.
- Workspace storage directory is based on workspace UUID.
- Agent working directory is based on agent UUID.
- Project and agent display names are not used as filesystem identifiers.

Suggested layout:

```text
$DATA_DIR/
  control.db
  workspaces/
    <workspace_uuid>/
      workspace.db
      agents/
        <agent_uuid>/
      artifacts/
      run-logs/
```

Do not show this layout in normal product UI. It can appear only in diagnostics/debug pages.

## Implementation Phases

### Phase 1: Workspace Tenant Foundation

Goal: make workspace a real tenant boundary.

Tasks:

- Add durable workspace ID and membership tables.
- Ensure creator is owner.
- Change APIs to resolve active workspace by ID, not path.
- Hide workspace root/path from normal Web UI.
- Add empty-state onboarding when user has no workspace.
- Keep local `--data-dir` as server deployment configuration.

Done when:

- A new user with no workspace sees create-workspace onboarding.
- Created workspace has owner membership.
- Web no longer shows local root path in normal pages.

### Phase 2: Invite And Member Onboarding

Goal: make team entry smooth.

Tasks:

- Add email invite model with token, expiry, status.
- Add invite acceptance and rejection pages.
- Preserve invite return path through registration.
- Support bulk invite UI.
- Allow invite creator to set workspace role, project grants, and linked agents.
- Add member management page scoped to workspace.

Done when:

- Admin can invite a user by email.
- New user can register and accept the invite.
- Accepted user enters the workspace with correct permissions.
- Rejected invite does not grant access.

### Phase 3: Project And Agent Permission Tightening

Goal: ensure all project and agent APIs enforce permissions.

Tasks:

- Move project membership to explicit project member table.
- Filter project list by membership.
- Enforce project role on all project write APIs.
- Add agent owner/operator/viewer grants.
- Audit all task/message/run/scheduler endpoints.
- Update UI to hide unavailable actions, while keeping API enforcement authoritative.

Done when:

- A user cannot list or access a project they do not belong to.
- A viewer cannot mutate tasks or scheduler.
- A project manager can manage project scheduler.
- UI and API agree on available actions.

### Phase 4: Connector And Credential Foundation

Goal: give agents safe access to external tools.

Tasks:

- Define connector provider registry.
- Add credential records with encrypted secret storage.
- Add OAuth/token flows for the first provider.
- Add credential scope and grant model.
- Add MCP/tool injection policy for agents.
- Add audit events for credential changes and usage.

Recommended first provider:

- GitHub or Feishu.

Done when:

- Workspace admin can connect one external provider.
- Admin can grant one agent access to that credential.
- Agent runtime receives only the authorized credential/tool config.
- Credential usage is audited.

### Phase 5: Agent Runtime Productization

Goal: make agent execution configurable without exposing infrastructure internals.

Tasks:

- Add runtime profile.
- Add sandbox profile.
- Add toolchain profile.
- Add skill bundle grants.
- Add MCP grants.
- Add environment variable policy.
- Replace path-based UI fields with profile selectors.
- Record runtime config changes in audit log.

Done when:

- Agent detail page shows product concepts, not file paths.
- Admin can configure runtime/sandbox/toolchain through profiles.
- Agent runs with isolated and authorized resources.

### Phase 6: Audit Log

Goal: make operations traceable.

Tasks:

- Add audit log table.
- Add audit writer helper.
- Instrument critical admin/member/agent operations.
- Add audit log Web page for admins.
- Add resource-level audit view for project/agent/task where useful.

Done when:

- Admin can answer who changed what, when, and on which resource.
- Sensitive actions are captured consistently.

### Phase 7: Onboarding Checklist And Home Pages

Goal: guide users without forcing a rigid wizard.

Tasks:

- Add workspace onboarding checklist.
- Admin home: operational overview.
- Member home: personal workbench.
- Add first-run quick actions.
- Add empty states for no project, no agent, no connector, no task.

Done when:

- New admin can reach first agent run without reading docs.
- Invited member sees relevant work immediately.

## Immediate Next Steps

Recommended implementation order:

1. Implement workspace ID + membership foundation.
2. Remove normal UI exposure of filesystem paths.
3. Add create-workspace onboarding for users without workspace.
4. Add invite acceptance flow.
5. Add audit log table and instrument workspace/member/project/agent changes.
6. Add connector/credential model document before coding providers.

Do not start connector implementation before workspace membership and audit basics exist. Credentials without strong ownership and audit will create security debt.

