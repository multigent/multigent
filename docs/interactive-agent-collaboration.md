# Interactive Agent Collaboration

This document defines how Multigent should support human-assisted agent work through Web Chat, Feishu/Lark, and future IM channels.

The product goal is not to "train" an agent in a separate mode. The goal is to let humans enter the agent's real working context when needed, add missing context, correct direction, approve decisions, or ask the agent to continue. Repeated intervention should later be distilled into prompts, skills, docs, policies, and task templates so the same human input is needed less often.

## Why This Matters

Task-only automation is too brittle for real company work.

Some bugs and product requests can be finished from a task description. Many cannot. A capable agent often needs a few rounds of clarification, business judgment, interface confirmation, or debugging feedback before it can produce useful output. If Multigent only supports "assign task and wait", users will experience the agent as unreliable. If Multigent supports easy intervention, the agent becomes a collaborative worker that can gradually absorb the company's workflow.

Web Chat is useful, but it should not be the only interaction surface. In many companies, Feishu/Lark is where work already happens. Opening a browser console to talk with an agent is less natural than mentioning or messaging the agent from the same IM flow where product, engineering, QA, and operations already coordinate.

## Product Principle

Multigent should treat Web Chat, Feishu/Lark messages, scheduled tasks, wakeups, and manual task runs as different entry points into the same underlying agent session system.

```text
Web Chat
Feishu/Lark
Task
Cron
Wakeup
   ↓
Interaction Gateway
   ↓
Auth / RBAC / Audit / Session Lock
   ↓
Agent Session Manager
   ↓
Sandbox Runtime Adapter
   ↓
Codex / Claude Code / Gemini CLI / future runtimes
```

Feishu is not a separate agent runtime. It is only a channel. The authority remains in Multigent: workspace, project, agent identity, credentials, sandbox policy, task state, audit logs, and permissions.

## Current State

Multigent currently has two related but incomplete pieces:

- Web Chat calls `multigent exec --project ... --agent ... --prompt ...` and streams the run output back to the browser.
- The existing `ccconnect` API and `IMConnectionPanel` proxy to an external cc-connect service and ask that service to create projects/platforms.

This is not enough for the SaaS product direction because the external cc-connect service owns too much of the execution shape. It assumes a work directory and an agent type, while Multigent needs to own workspace isolation, agent credentials, RBAC, audit, model accounts, runtime connections, and sandbox lifecycle.

cc-connect itself is still valuable. Its platform layer, especially Feishu/Lark message handling, card rendering, QR setup, retry logic, and event handling, is mature enough to reuse. The part we should not reuse as-is is the external agent execution ownership.

## Recommended Direction

Build a first-class Multigent interaction layer and initially support only Feishu/Lark.

The first implementation should not depend on running a separate cc-connect process. We should copy or vendor the minimum Feishu/Lark platform logic from cc-connect into Multigent, then adapt it to call Multigent's own session APIs.

Recommended module shape:

```text
internal/interaction/
  session.go          # active session state, locks, transcript events
  gateway.go          # channel-neutral message ingress
  audit.go            # intervention and message audit

internal/imbridge/
  lark/
    setup.go          # app setup / QR or manual app credential setup
    events.go         # receive message events
    send.go           # send replies/cards/files
    binding.go        # Feishu user/chat binding to Multigent user/agent

internal/runtime/session/
  manager.go          # start/send/stop interactive sessions
  adapter.go          # runtime-independent interface
  codex.go            # codex exec/resume based implementation
  claudecode.go       # long-running or resume-based implementation
```

Later we can add Slack, DingTalk, WeCom, Telegram, or other platforms using the same bridge interface. We should not expose "cc-connect instance URL" as a primary product setting in the final product.

## Why Not Keep cc-connect As An External Dependency

Keeping cc-connect as an external service is fast for a prototype, but it creates the wrong product boundary:

- It makes IM integration feel like an external add-on, not a core Multigent ability.
- It duplicates project/session concepts outside Multigent.
- It pushes work directory and agent runtime details back into user-visible configuration.
- It risks bypassing Multigent RBAC, audit, sandbox policy, model account selection, and credential injection.
- It makes deployment harder for SaaS users because they now need Multigent plus cc-connect.

The acceptable short-term use of cc-connect is source reuse: copy or vendor the Feishu/Lark bridge code and adapt it to Multigent's interfaces.

## Session Model

The core entity is an agent session.

```text
Agent Profile
  role / prompt / skills / permissions / model account / sandbox policy

Agent Session
  one concrete working context for chat, task, cron, or wakeup
```

Session fields:

```text
id
workspace_id
project_id
agent_id
source: web_chat | lark | task | cron | wakeup | api
status: active | waiting_input | completed | failed | cancelled
lock_state: unlocked | locked
lock_reason: interactive | running_task | stopping
lock_owner_type: user | task | scheduler | channel
lock_owner_id
current_runtime_session_id
current_run_id
human_intervened
created_by
created_at
updated_at
last_activity_at
```

Transcript events:

```text
id
session_id
workspace_id
actor_type: user | agent | system
actor_id
channel: web | lark | scheduler | runtime
event_type: message | run_started | run_output | run_completed | interrupt | approval | summary
content
metadata_json
created_at
```

The transcript should be stored in the database for query, audit, and product UI. Large raw logs can still be stored in files or object storage, referenced by run IDs.

## Locking Rules

The lock is not about a "training mode". It is about preventing context corruption.

Rules:

1. One agent can have at most one active mutable session by default.
2. If an agent is running a task, a human can enter that same session to observe or intervene.
3. If a human sends a message into a running task session, the session is marked `human_intervened`.
4. Scheduler must not assign a new task to an agent with an active locked session.
5. A manual Web/Feishu conversation without an active task creates an interactive session and locks the agent.
6. When the session completes, fails, is cancelled, or is explicitly released, the lock is cleared.
7. Force unlock requires manager/admin permission and must write an audit event.

This gives the behavior users expect:

- The agent can keep working autonomously.
- A human can step in when quality is poor or context is missing.
- The agent is not allowed to silently mix another task into the same context.

Future versions may support forked sessions for experiments, but the first product version should stay strict.

## Feishu/Lark User Journey

### Admin Setup

1. Admin opens workspace settings.
2. Admin connects a Feishu/Lark app.
3. Multigent verifies the app credentials and required scopes.
4. Admin chooses whether the bot is workspace-wide or limited to selected projects.
5. Multigent stores the connection as a workspace connection, with audit and encrypted secrets.

### Bind Agent To Feishu

1. User opens an agent detail page.
2. User clicks "Connect Feishu".
3. User chooses personal chat, group chat, or both.
4. Multigent creates a binding:

```text
workspace_id
project_id
agent_id
platform: lark
chat_id
allowed_user_ids
created_by
status
```

5. Multigent sends a confirmation message to the chat.

### Daily Use

For a direct message:

```text
User messages agent in Feishu
  ↓
Lark bridge receives event
  ↓
Map Feishu user to Multigent user
  ↓
Check workspace/project/agent permission
  ↓
Find active session or create one
  ↓
Append message event
  ↓
Run/resume agent through session manager
  ↓
Stream reply back to Feishu
  ↓
Persist transcript, run log, token usage, audit
```

For a group chat:

- Only respond when mentioned, replied to, or explicitly bound to a command.
- Preserve the chat/thread reference so replies stay in the correct Feishu thread when possible.
- Apply project membership and agent operation permission based on the sender.

## Runtime Implications

The current Docker runtime is closer to per-run execution. That is acceptable for task execution and can work for Codex-style `exec resume`, as long as session files and runtime home are persisted per agent.

For richer Feishu conversations, we need an explicit session runtime abstraction:

```go
type AgentSession interface {
    Send(ctx context.Context, msg Message) (<-chan Event, error)
    Stop(ctx context.Context) error
    RuntimeSessionID() string
}

type SessionManager interface {
    Acquire(ctx context.Context, agent AgentRef, source SourceRef) (*Session, error)
    Send(ctx context.Context, sessionID string, msg Message) (<-chan Event, error)
    Release(ctx context.Context, sessionID string, reason string) error
}
```

Codex can initially use one process per turn:

```text
codex exec <prompt>
codex exec resume <thread-id> <prompt>
```

Claude Code may need a long-running process for the best interactive experience. cc-connect's Claude Code session implementation is a useful reference because it keeps a persistent process and communicates through stream-json/stdin. Multigent should not expose that complexity to users; it belongs inside the runtime adapter.

Sandbox persistence must remain per-agent:

```text
workspace/<workspace_uuid>/agents/<agent_uuid>/
  runtime-home/
    codex/
    claude/
  sessions/
  runs/
  workspace/
```

No container should mount global host `~/.codex`, `~/.claude`, or other user-wide session directories. Credentials must be injected from Multigent-managed model accounts and runtime connections.

## Permissions

Feishu messages must be authorized the same way as Web operations.

Permission checks:

- The Feishu user must be linked to a Multigent user.
- The user must be a member of the workspace.
- The user must have access to the project.
- The user must have permission to operate or message the agent.
- Group chats must be explicitly bound to the project/agent.

Agent actions triggered from Feishu should use the same audit principal:

```text
principal_type: user
principal_id: <multigent_user_id>
channel: lark
external_actor_id: <feishu_open_id>
```

The agent itself remains a separate principal when it calls `mga` or other runtime APIs:

```text
principal_type: agent
principal_id: <agent_id>
```

## What Gets Distilled

After a human intervenes, Multigent should make it easy to save durable improvements:

- Add project doc
- Update agent instruction
- Create or update skill
- Create task checklist
- Add tool/credential requirement
- Add escalation rule
- Create task template
- Mark a repeated manual intervention pattern

The product should not automatically rewrite prompts from every chat. Instead, it should suggest candidates and let a responsible human approve changes. This keeps agent behavior predictable.

## Implementation Plan

### Phase 1: Internal Session API

- Create `interactive_sessions` and `session_events` storage.
- Refactor Web Chat to use `SessionManager` instead of directly spawning `multigent exec`.
- Add strict per-agent active session lock.
- Persist transcript events separately from raw run logs.
- Add scheduler skip behavior for locked agents.

### Phase 2: Lark Bridge MVP

- Copy/vendor the minimum Feishu/Lark event and send logic from cc-connect.
- Add workspace-level Lark app connection settings.
- Add Feishu user binding to Multigent users.
- Add project/agent chat binding.
- Route incoming Lark messages to `SessionManager.Send`.
- Return agent output to Feishu as text/card messages.
- Audit every inbound message, outbound reply, and permission denial.

### Phase 3: Human Intervention UX

- Show active session and lock status on agent detail.
- Let users join the current running task session from Web.
- Add "release session", "stop run", and "force unlock" actions with permissions.
- Add transcript review and "distill into prompt/skill/doc/task template" actions.
- Add Feishu shortcut cards for stop, continue, create task, and summarize.

### Phase 4: Runtime Improvements

- Add persistent session runtime support where needed.
- Keep per-agent runtime homes and credentials isolated.
- Add idle timeout and cleanup for long-running containers/processes.
- Add resource limits per active session.
- Normalize streaming events across Codex, Claude Code, Gemini, and future runtimes.

## Open Questions

- Should a Feishu group binding target exactly one agent, or allow commands to route to multiple agents in the same project?
- Should users be able to create a task directly from a Feishu thread before running the agent?
- For Claude Code, do we require persistent containers for interactive sessions, or start with resume-based one-turn execution?
- How much of cc-connect's rich card rendering should be copied in the first version?
- Should every human intervention produce a required review prompt, or only sessions above a certain duration/token threshold?

## Decision

The recommended first product path is:

1. Do not keep cc-connect as a separate required service.
2. Reuse cc-connect's Feishu/Lark bridge logic by copying or vendoring selected code.
3. Make Multigent own interaction sessions, locks, runtime execution, permissions, credentials, and audit.
4. Treat Feishu/Lark as a first-class interaction channel, not a separate agent runtime.
5. Make human intervention a normal part of agent collaboration, then help teams distill repeated intervention into durable agent capability.
