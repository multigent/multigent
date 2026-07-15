# Multigent Product Completion Plan

This document defines the target product journey and the implementation plan for making Multigent a team-grade multi-agent collaboration platform.

Multigent is still in active development. We do not need to preserve legacy behavior. When the current model conflicts with the target product shape, prefer correcting the model directly over adding compatibility layers.

## Current Build Baseline

As of the current development line, Multigent already has the first version of several core SaaS primitives:

- Workspace and membership records are in the control database.
- Project visibility and project-level roles are enforced in the main project APIs.
- Users can be linked to specific agents they operate.
- Model providers support workspace-owned and user-owned scopes.
- Connector connections support workspace/user ownership, grants, encrypted secret records, runtime manifests, MCP proxying, action proxying, and test calls.
- Agent runtime tokens exist and are scoped to workspace/project/agent/run capabilities.
- Agent provider binding is filtered by the concrete agent, so an agent config page only lists providers that the current user can actually bind.
- Audit events exist for many sensitive operations, including model provider and connection changes.

The remaining work is not compatibility migration. This is a new product, so when an old local-first concept conflicts with the target SaaS shape, remove or replace it instead of adding fallback behavior.

## Development Principles

Because Multigent is still before real external production usage, we should optimize for the correct product architecture instead of preserving historical behavior.

Rules:

- Prefer deleting incorrect legacy concepts over wrapping them in compatibility layers.
- Public UI/API should expose SaaS product concepts, not local-first implementation details.
- Storage, runtime, scheduler, and sandbox internals can remain configurable for deployment, but normal users should not need to understand paths, mounted folders, database files, or process flags.
- RBAC must be enforced by API first. The Web UI may hide unavailable actions, but it is never the security boundary.
- Agents are principals, not trusted internal scripts. They authenticate, receive scoped tokens, and call Multigent APIs like users do.
- Every sensitive operation should be auditable before the product is considered team-ready.
- Each phase should leave the product in a coherent state. Avoid adding temporary migration commands or transitional UX that users will later need to unlearn.

## Product North Star

The product goal is to let a team operate cloud agent coworkers with less human blocking and less repeated context transmission.

Multigent should help a company answer:

- Which agents exist in this workspace?
- Who is responsible for each agent's behavior?
- Which projects, tasks, context, skills, credentials, and tools can each agent access?
- What did each agent do, with which permissions, using which external connections?
- Where does a human need to review or tune the process instead of synchronously driving every step?

The core loop is:

```text
human expertise -> role/agent configuration -> authorized run -> observable result -> prompt/skill/policy improvement
```

Humans remain accountable for expertise, review, and policy. They should not be the synchronous transport layer between scattered documents, tools, and agents.

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

## Journey Gap Checklist

This checklist is the main acceptance frame for the next development cycles.

### Admin Setup Path

Current target:

```text
register -> verify/login -> create workspace -> invite members -> configure teams/roles/skills/connections -> create project -> hire agents -> grant tools -> schedule/run -> review/audit
```

Must be true:

- A first admin can create a workspace without seeing filesystem paths.
- Workspace creator automatically becomes owner/admin.
- Admin can invite members by email, set their workspace role, and optionally pre-grant project or agent access.
- Admin can create teams and roles as reusable agent context primitives, but is not forced through that step before creating a project.
- Admin can create external connections and grant them to projects or agents.
- Admin can see workspace-level health, blocked runs, sensitive audit events, and setup progress.

Known gaps:

- Email verification/delivery and invite acceptance still need production-ready UX.
- Bulk invite and invite revoke/expiry management need polish.
- Admin home is still not fully role-aware and operational enough.
- Teams/roles/skills setup needs clearer first-run empty states and templates.

### Invited Member Path

Current target:

```text
open invite -> register if needed -> accept/reject -> enter workspace -> see only accessible projects/agents/tasks/messages -> operate or tune assigned agents
```

Must be true:

- Invite token survives registration and returns the user to the accept/reject screen.
- Rejecting an invite does not grant access.
- A member cannot see projects they are not assigned to.
- A member sees personal workbench data first: my tasks, my messages, agents I own/operate, runs needing my review.
- A member can create personal connections and grant them only to agents they are allowed to operate.

Known gaps:

- Member landing page still needs to diverge from admin overview.
- Some task/message/run/scheduler endpoints still need endpoint-by-endpoint RBAC verification.
- Personal connection grants exist, but the UX should make the owner/operator boundary obvious.

### Project Manager Path

Current target:

```text
create project -> add members -> hire agents -> bind runtime/sandbox/toolchain/provider -> grant skills/connections -> assign tasks -> monitor runs -> tune agents
```

Must be true:

- Project membership is explicit.
- Project manager can manage project members, agents, scheduler, and tool grants inside that project.
- Project operator can execute work and interact with assigned agents without changing high-risk configuration by default.
- Viewer can read state but cannot mutate tasks, scheduler, agents, or credentials.
- Agent detail page is centered on product concepts: role, owner, runtime profile, sandbox profile, toolchain, skills, connections, schedule, runs.

Known gaps:

- Runtime/sandbox/toolchain profiles are not yet fully first-class product records.
- Some UI pages still need permission-aware action hiding.
- Agent performance/tuning view is still early.

### Agent Runtime Path

Current target:

```text
scheduled/task/event trigger -> isolated sandbox -> scoped runtime token -> fetch manifest -> call Multigent API/proxies -> produce result -> write audit/run telemetry -> human reviews/tunes
```

Must be true:

- Agent does not directly receive broad workspace secrets.
- Agent does not directly access the control database.
- Agent receives only granted connections, tools, skills, and environment variables.
- Runtime proxy endpoints re-check grants on each call.
- Run logs, token/cost, connection usage, and important side effects are traceable.

Known gaps:

- Per-run workspace materialization and strict production mount policy still need completion.
- Runtime profile and sandbox profile are still too close to implementation flags.
- Resource limits and network policies need product-level configuration.
- Agent-runtime credential/tool usage needs richer audit coverage.

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

Status:

- Mostly implemented.
- Remaining work: polish no-workspace onboarding, workspace switch edge cases, and route users to role-appropriate home pages after workspace creation or switch.

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

Status:

- Partially implemented.
- Invitation management now has list/revoke/reject APIs. Workspace admins can create and manage invites without needing platform-global admin role.
- Accepted invitations now create a normal user and add that user to the current workspace with the invited workspace role. Rejected/revoked invitations cannot be accepted.
- People page shows invitation status, supports workspace role selection, bulk email input, invite link copy, and invite revoke.
- Accepted invites route the browser back to the workspace home instead of leaving the user on the invite URL.
- Remaining work: email delivery abstraction, invite acceptance UX polish, project/agent pre-grant UI, invite expiry screens, and no-workspace onboarding after invite rejection.

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

Status:

- Partially implemented.
- Workbench aggregate endpoints and telemetry run/log endpoints now use workspace/project/agent access helpers instead of global login role checks. Linked-agent users only see telemetry for their linked agents; run logs require access to the corresponding run.
- Global scheduler start/stop now require current workspace admin access instead of platform-global admin role. Telemetry log reads resolve through authorized run records, including relative log paths.
- Runtime token issuing now filters requested capabilities through an explicit allowlist instead of signing arbitrary caller-provided capability strings.
- Current priority: finish endpoint-by-endpoint RBAC audit for tasks, messages, runs, scheduler, agent config, provider binding, connection grants, and runtime manifest access.
- UI must hide unavailable actions, but API checks remain authoritative.

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

Status:

- First implementation exists for provider registry, connection ownership, grants, encrypted secrets, runtime connection manifests, custom MCP proxy, custom HTTP action proxy, GitHub/Linear action proxy, and connection test calls.
- Connection test results now persist the latest validation status in connection profile metadata and surface it in the Web connection list.
- Remaining work: OAuth flow abstraction, DingTalk provider, GitHub app installation flow, richer profile/scopes display, scheduled/background connection health checks, and per-provider action executor hardening.

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

Status:

- Partially implemented.
- Docker remains the first sandbox backend, but the product model should expose runtime/sandbox/toolchain profiles instead of Docker flags.
- Current priority: materialize per-run workspaces, inject scoped runtime tokens, stop mounting broad workspace directories in production profiles, and make CLI/toolchain installation profile-driven.

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

Status:

- Partially implemented.
- Remaining work: centralize audit coverage expectations by endpoint group, add admin audit UI filters, and make agent-runtime credential/tool usage auditable.

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

Status:

- Early implementation.
- Root route, Sidebar admin-only navigation, project admin sub-navigation, command palette admin entries, and Workbench overview tab now use the current workspace admin permission instead of the global login role.
- Current priority: replace the generic overview with role-aware home pages:
  - Admin/owner: workspace health, setup checklist, active projects, agent health, blocked runs, audit highlights.
  - Member/operator: my messages, my assigned tasks, agents I operate, projects I can access, runs needing review.

### Phase 8: Agent Operations And Human Tuning Loop

Goal: make each human an agent owner/operator who improves agent behavior instead of manually relaying every step.

Tasks:

- Add agent performance view: recent runs, success/failure rate, blocked reasons, token/cost, response quality notes.
- Add tuning actions: edit wakeup prompt, propose skill, adjust tool grants, adjust model/provider, adjust runtime profile.
- Add review workflow: approve output, request rerun, create follow-up task, convert repeated intervention into policy/skill/prompt.
- Add memory candidate flow: agent proposes reusable memory; responsible human approves, edits, or rejects.
- Add human intervention ledger: track repeated human decisions and recommend automation.

Done when:

- A responsible human can see why their agent underperformed and change the configuration in one place.
- Repeated manual interventions are visible and can become rules, skills, prompts, or grants.
- Project completion produces reusable agent/team/project memory instead of only raw run logs.

## Immediate Next Steps

Recommended implementation order:

1. Finish the RBAC endpoint audit and fix any API that still returns unfiltered project, task, message, run, provider, connection, or scheduler data.
2. Build role-aware home pages so admin and member users land in different operational views.
3. Finish invite UX: bulk invite, accept/reject screen, expiry/revoke, and post-accept workspace routing.
4. Replace remaining path-facing UI with workspace/project/agent/profile concepts.
5. Productize runtime profiles: sandbox, toolchain, CLI version, dependency setup, resource policy, and network policy.
6. Add the agent operations view for performance, run review, and prompt/skill/policy tuning.
7. Expand connector providers: DingTalk, GitHub app, and Linear/Jira/Plane style project tools after Feishu/Lark stabilizes.
8. Add memory candidate approval so completed projects and sub-tasks leave reusable context for future agents.

## Next Coding Slices

The next slices should be small enough to ship and verify one by one:

1. Role-aware home routing:
   - Admin/owner enters workspace operations overview.
   - Member/operator enters personal workbench.
   - No-workspace user enters create-workspace onboarding.
2. RBAC audit pass:
   - Audit project, task, message, run, scheduler, agent config, provider binding, connection grant, and runtime manifest endpoints.
   - Add tests for forbidden list/read/write behavior.
3. Invite flow completion:
   - Add invite list/revoke/expiry UI.
   - Add bulk invite.
   - Preserve invite token across registration.
4. Runtime profile productization:
   - Introduce explicit sandbox/toolchain/runtime profile records.
   - Move Docker/runtime flags out of normal agent detail UI.
   - Materialize per-run workspace directories by UUID.
5. Agent operations page:
   - Show run history, success/failure, blocked reasons, token/cost, connection usage, and review actions.
   - Let responsible humans tune prompts, skills, grants, model/provider, and scheduler from one place.
6. Audit log UI hardening:
   - Add admin filters by actor, resource, action, project, agent, and time.
   - Add resource-level audit views where it helps debugging.

Near-term coding rule: prefer removing local-first or compatibility-oriented commands from the public surface. Internal maintenance commands can stay hidden or developer-only, but normal users should see a SaaS agent-operations product, not a migration toolkit.
