# Agent Isolation and Permission Architecture

Multigent is a multi-agent scheduling platform. Production readiness depends on one hard rule:

> An agent is an untrusted actor unless a policy explicitly grants it data, tools, network, compute, and collaboration rights.

This is different from the old local-first agency model where one trusted owner ran agents inside one machine and one workspace folder.

## Product Decision

Multigent should treat every agent as a first-class principal, not just as metadata under a project.

Principal types:

- `human`
- `agent`
- `worker`
- `system`

An agent principal has:

- stable `agent_id`
- workspace membership
- project membership
- agent role grants
- context grants
- tool grants
- resource quotas
- runtime isolation policy

The scheduler dispatches work as that agent identity. API calls, DB reads, messages, context retrieval, and tool execution must authorize against that identity.

## Isolation Layers

### 1. Data Isolation

Every workspace-scoped table must carry `workspace_id`.

Agent-scoped reads must also be filtered by capability:

- Agent can read its own tasks by default.
- Agent can read project tasks only if granted `task.read` on the project or task.
- Agent can message another agent only if granted collaboration rights for that target.
- Agent can read context packs only through explicit `context.read` grants.
- Agent should not receive raw DB credentials.

The agent process should call Multigent through a scoped API token, not connect to the database directly.

### 2. Credential Isolation

Do not mount or forward host-wide credentials by default in production.

Production model:

- agent gets a short-lived scoped token issued for one run
- token contains `workspace_id`, `agent_id`, `run_id`, capability scope, and expiry
- external credentials are mediated by Multigent tool gateways
- sensitive provider keys are never written into agent workdirs

Local developer mode can keep explicit credential mounts, but it must be clearly marked as unsafe for production.

### 3. Filesystem Isolation

Each run gets an isolated workspace:

```text
/runs/<run_id>/workspace
/runs/<run_id>/artifacts
/runs/<run_id>/tmp
```

Inputs are materialized into the run workspace:

- selected context bundle
- task prompt
- allowed repo snapshot or checkout
- allowed skill scripts
- tool manifest

Outputs are copied out after the run:

- patch/diff
- artifacts
- logs
- memory candidates
- task summary

Agents should not mount the entire Multigent workspace or another agent's workdir. Cross-agent collaboration goes through API/message/task surfaces, not shared files.

### 4. Resource Isolation

Every run needs enforceable limits:

- CPU
- memory
- wall-clock timeout
- max output bytes
- max artifact size
- max token budget
- max tool calls
- network policy
- concurrency per workspace/project/agent

When the limit is reached, the run is stopped and recorded as a controlled failure, not allowed to degrade the whole platform.

### 5. Network Isolation

Default production policy should be deny-by-default with allowlists:

- no network
- allow Git provider
- allow package registry
- allow configured API host
- allow Multigent control API

`host` network mode must not be available in SaaS execution.

## Shared Resources

Shared resources should be copied or mounted read-only, never mounted as writable global state.

### Skills

Preferred model:

- skills are versioned assets in the control plane
- run materializer copies the selected skill version into the run workspace
- scripts execute from the copied directory
- writes stay inside run temp/artifacts

This gives reproducibility: a run can be replayed with the exact skill version it used.

For large skills or binary toolchains, use content-addressed cache:

```text
/cache/skills/<sha256>        read-only
/runs/<run_id>/skills/<name>  symlink or copy
```

The run process still sees only its allowed skill set.

### Repositories

Do not mount arbitrary host paths in production.

Options:

- clone a repo at a specific commit into the run workspace
- use a persistent per-project checkout owned by a worker, then copy/overlay into each run
- return patches instead of letting the agent push directly

Write-back to Git should be a separate reviewed action with human or policy approval.

### Context

Context should be materialized as a bundle:

```text
context.json
context.md
sources/
```

The bundle is produced by authorization-aware retrieval:

- workspace facts
- team prompt
- role prompt
- project context
- task context
- explicitly granted context packs
- agent memory approved for this agent

The agent never gets a general "search all docs" capability unless explicitly granted.

## Runtime Provider Strategy

Multigent should define a sandbox provider interface:

```go
type RuntimeProvider interface {
    Prepare(run RunSpec) (PreparedRun, error)
    Start(prepared PreparedRun) (RunHandle, error)
    StreamLogs(runID string) (<-chan Event, error)
    Stop(runID string) error
    Collect(runID string) (RunResult, error)
}
```

Provider options:

| Provider | Use case | Notes |
| --- | --- | --- |
| `docker` | local/self-hosted MVP | Good enough for development and private deployment, but host daemon security must be treated carefully. |
| `e2b` | cloud code sandbox | Better SaaS fit for isolated ephemeral code execution; useful for remote agent coworkers. |
| `firecracker/microvm` | high isolation self-hosted | Stronger boundary, more infra work. |
| `kubernetes job` | enterprise/self-hosted scale | Natural quota and network policy story, heavier ops. |

Recommendation:

- Keep Docker as local/self-hosted provider.
- Add provider interface now so Docker is not hard-coded into scheduler/runner.
- Use E2B or microVM-backed provider for hosted SaaS execution.
- Disable host execution in production profiles.

## Agent API Access

Agent processes should not run arbitrary `multigent` CLI commands with owner-level authority.

Instead, each run receives:

- `MULTIGENT_RUN_ID`
- `MULTIGENT_AGENT_ID`
- `MULTIGENT_WORKSPACE_ID`
- `MULTIGENT_API_URL`
- `MULTIGENT_AGENT_TOKEN`

The token allows only scoped actions:

- read current task
- mark current task status
- send message to allowed recipients
- create task only if delegation capability is granted
- query allowed context
- write artifacts
- propose memory

It does not allow:

- list all users
- list all DB rows
- edit another agent's prompt
- read unrelated project context
- configure integrations
- access billing/admin APIs

## Agent Collaboration Permissions

Agent-to-agent visibility should be explicit.

Default:

- agent sees itself
- agent sees project-level public agents if project policy allows it
- agent can message agents in the same project only if `agent.message` or project collaboration policy allows it
- cross-project messages require explicit grant

We should add capabilities:

- `agent.message`
- `agent.delegate_task`
- `agent.read_profile`
- `agent.read_runs`
- `agent.read_artifacts`

This is important because "send message to another agent" is not harmless: messages are a context injection channel.

## Database Access

Agents should never receive direct DB credentials in SaaS.

All DB access goes through service APIs:

```text
agent process -> scoped token -> Multigent API -> repository layer -> DB
```

This lets us enforce:

- workspace isolation
- project grants
- context grants
- row-level filters
- audit logs
- rate limits

For customer databases or product databases, access must be exposed as explicit tools with least-privilege credentials and per-tool approval policy.

## Audit Model

Every agent action that crosses a boundary should be auditable:

- task read/update
- message send
- context read
- tool call
- artifact write
- memory proposal
- credential/tool access
- run start/stop
- quota stop

Audit events should include:

- `workspace_id`
- `actor_type`
- `actor_id`
- `run_id`
- `resource_type`
- `resource_id`
- `capability`
- `decision`
- timestamp
- reason

## Implementation Plan

### Phase A: Remove Unsafe Product Surface

- Remove public filesystem migration commands.
- Keep DB-backed state as the only product path.
- Keep any old-layout helpers as private test/dev utilities only if still needed.

### Phase B: Principal Model

- Add `principal_type` to auth/session context.
- Represent agents as principals.
- Persist agent grants separately from human user grants.
- Extend RBAC capabilities with agent collaboration and artifact permissions.

### Phase C: Scoped Agent Tokens

- Issue short-lived run tokens.
- Add API middleware that can authenticate `human`, `agent`, `worker`, and `system` principals.
- Replace in-agent broad CLI authority with scoped API calls.

### Phase D: Runtime Provider Interface

- Extract Docker execution behind `RuntimeProvider`.
- Keep Docker provider for local/self-hosted.
- Add run materialization step before provider start.
- Add production profile that rejects host execution.

### Phase E: Run Materialization

- Create per-run workspaces.
- Copy authorized context and skills into the run.
- Stop mounting whole workspace root into containers.
- Copy artifacts and logs out after completion.

### Phase F: Policy Enforcement

- Add resource quotas per workspace/project/agent.
- Add network policy to run spec.
- Add tool allowlist to run spec.
- Add audit log for all authorization decisions and tool calls.

## Agno Reference Notes

Agno is useful as a framework reference for:

- agent/team abstraction
- session IDs
- memory manager patterns
- tool execution events
- multi-user/multi-session examples

But it is not a replacement for Multigent's control-plane design. Multigent's differentiation is not "build an agent object"; it is scheduling, governance, identity, context management, and safe multi-agent execution for teams.

## North Star

The production architecture should let a company safely run many cloud agent coworkers continuously:

- each agent has a job, identity, owner, budget, and permissions
- each run is isolated and reproducible
- shared context is explicit and versioned
- every cross-boundary action is authorized and auditable
- humans review policies and outcomes, not every synchronous step
