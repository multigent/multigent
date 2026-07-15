# SQLite Storage Architecture

Multigent is moving from a mostly file-backed workspace model to a hybrid model:

- SQLite stores authoritative relational state.
- Files store human-editable content, large artifacts, and append-only run traces.

This keeps the local-first experience simple while giving the product a data model that can become a SaaS control plane later.

## Why Change

The file layout is useful for prompts, skills, notes, and generated artifacts, but it is a poor fit for objects that need permissions, queries, joins, auditability, and concurrent updates.

Relational data currently scattered across YAML and JSON files includes:

- workspaces
- users and workspace memberships
- teams, roles, projects, and agents
- tasks, messages, invitations, schedules
- workers and agent runtime state
- context pack metadata and document references

As soon as Multigent supports multiple workspaces, local workers, RBAC, and cloud agents, these objects need one consistent source of truth.

## Reference Models

Plane uses a conventional multi-tenant relational model:

- `workspaces` is the tenant root.
- business tables such as projects, teams, members, invites, and issues carry `workspace_id`.
- uniqueness is scoped by workspace, for example `(workspace_id, name)`.

Huly/platform carries `workspaceId` through service tokens, sessions, workers, and APIs. That is the right runtime boundary for Multigent:

- every request resolves a workspace
- every worker belongs to a workspace
- every agent run and event stream is tagged with workspace_id

Multigent should combine these two ideas.

## Target Layout

Local default:

```text
~/.multigent/multigent.db
~/.multigent/workspaces/<workspace_id>/
```

The DB is the local control plane. Workspace folders hold content and runtime artifacts.

Future SaaS:

```text
Postgres / managed SQL: relational control plane
Object storage: docs, artifacts, logs
Event stream / JSONL archive: raw agent run events
```

SQLite is the first storage engine, not a dead end. Tables and repository APIs should be written so Postgres can replace SQLite later.

## Workspace Isolation

Rules:

- `workspace_id` is required on all workspace-scoped tables.
- API handlers must resolve the current workspace before querying.
- Worker registration, task execution, run telemetry, and message delivery must include workspace_id.
- User accounts may be global, but permissions are granted through `workspace_members`.
- Unique names are scoped per workspace unless the object is truly global.

Examples:

```text
users
workspaces
workspace_members
teams              workspace_id + name
roles              workspace_id + team_id + name
projects           workspace_id + name
agents             workspace_id + project_id + name
tasks              workspace_id + project_id + agent_id
messages           workspace_id + thread_id
workers            workspace_id + worker_id
agent_runs         workspace_id + run_id
context_packs      workspace_id + name + version
```

## What Stays In Files

Files remain the right storage for content that humans or agents directly read and edit:

- `agency-prompt.md`
- team and role prompt markdown
- `SKILL.md` and bundled skill files
- knowledge base markdown
- handoff and journal notes
- raw run JSONL logs
- stdout/stderr logs
- generated artifacts and attachments

The DB stores metadata for these files:

- owner workspace
- path or object key
- content hash
- document type
- permissions
- index status
- version metadata

## First Migration Phase

Phase 1 creates a local control-plane database and moves the workspace registry into it.

Tables:

- `workspaces`
- `users`
- `workspace_members`

Users, JWT settings, invitations, and workspace memberships are stored in SQLite. `users.json` is not part of the new runtime model.

The existing file workspace layout is still used for prompts, teams, projects, tasks, and agent directories.

## Next Phases

Phase 2:

- Store current/last workspace on the user profile.
- Split project and agent grants out of user JSON fields into normalized grant tables.

Phase 3:

- Team, role, project, and agent metadata are stored through the DB-backed `store.Store` implementation.
- Prompt bodies, skills, and agent working directories remain file-backed content.

Phase 4:

- Tasks, messages, inbox items, heartbeat config, cron config, and task comments are stored through the DB-backed `taskstore.Store` implementation.
- Raw run events, stdout/stderr logs, and generated artifacts remain file-backed content.
- Run summaries are still handled by the existing telemetry SQLite DB and can be folded into the control-plane DB later.

Phase 5:

- Keep storage access behind repository interfaces so SQLite can be replaced by MySQL/Postgres for SaaS.
- Evaluate run summary storage separately instead of folding telemetry into the control-plane DB by default.
- Normalize JSON payload records into dedicated tables once product semantics stabilize.

Implemented in this phase:

- Runtime storage is behind interfaces (`db.Store`, `store.Store`, `taskstore.Store`) so the default SQLite implementation can be replaced by MySQL/Postgres-oriented implementations later.
- New product paths should write authoritative relational state to the DB-backed stores directly.
- Any filesystem import/export helpers should remain internal developer tooling, not public product surface, because Multigent is a new product and should not expose legacy migration concepts to users.

Known limits:

- Raw run logs and telemetry rows are intentionally not included in this bridge; they need a separate retention and privacy policy.
- `kv_records` is a portable first DB shape, not the final SaaS schema. Dedicated tables should be introduced after the product semantics settle.
