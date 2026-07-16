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
- The old IM product path proxied to an external cc-connect service and asked that service to create projects/platforms.

This is not enough for the SaaS product direction because the external cc-connect service owns too much of the execution shape. It assumes a work directory and an agent type, while Multigent needs to own workspace isolation, agent credentials, RBAC, audit, model accounts, runtime connections, and sandbox lifecycle.

cc-connect can be treated only as historical reference. Multigent should not depend on a running cc-connect service, should not expose a cc-connect URL/token setting, and should not preserve a compatibility path that delegates agent interaction to cc-connect.

The existing external cc-connect integration should be removed from the product path. This is a new project and we do not need to preserve a compatibility layer that asks users to configure a cc-connect API URL. The final Web product should expose Multigent-native Feishu/Lark connection settings only.

## Recommended Direction

Build a first-class Multigent interaction layer and initially support only Feishu/Lark.

The first implementation should not depend on running a separate cc-connect process. Feishu and Lark should be implemented as native Multigent channel providers. The provider abstraction should hide platform-specific QR setup, event decryption, message parsing, group addressing rules, and reply sending from the API/session layer.

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

Later we can add Slack, DingTalk, WeCom, Telegram, or other platforms using the same bridge interface.

## MVP Target Experience

The first shippable version should deliver this exact user experience:

1. A user opens an agent detail page in Multigent.
2. The page shows separate `Connect Feishu` and `Connect Lark` actions when the agent is not connected.
3. The user clicks one action and sees a QR-code setup flow.
4. The user scans the QR code in Feishu/Lark and approves the created application bot.
5. Multigent stores the app/bot credential securely, binds the provider connection to this agent, and maps the scanning IM user to the current Multigent user.
6. The agent detail page changes to `Connected to Feishu` or `Connected to Lark`, with provider, app/bot status, callback URL, security status, and last activity.
7. The user opens the created Feishu/Lark application bot and sends a message directly to the agent.
8. Multigent receives the callback, verifies the event, authenticates the external user, checks RBAC, acquires the agent session lock, resumes or starts the agent runtime session, and sends the reply back through the same bot.
9. The Web agent page shows the connected provider status plus the same transcript/session/run state that Web Chat would show.
10. If the agent is already busy, Multigent does not start a conflicting run. It records the event and replies with a clear busy message.

The MVP does not need multiple IM providers. It only needs Feishu/Lark to work well.

## Why Not Keep cc-connect As An External Dependency

Keeping cc-connect as an external service is fast for a prototype, but it creates the wrong product boundary:

- It makes IM integration feel like an external add-on, not a core Multigent ability.
- It duplicates project/session concepts outside Multigent.
- It pushes work directory and agent runtime details back into user-visible configuration.
- It risks bypassing Multigent RBAC, audit, sandbox policy, model account selection, and credential injection.
- It makes deployment harder for SaaS users because they now need Multigent plus cc-connect.

The acceptable short-term use of cc-connect is reading it as reference while implementing native Multigent modules. It should not remain in runtime dependencies, user settings, API contracts, or product copy.

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

### Bind Agent To Feishu/Lark

1. User opens an agent detail page.
2. User clicks `Connect Feishu` or `Connect Lark`.
3. User chooses personal chat, group chat, or both.
4. Multigent creates a binding:

```text
workspace_id
project_id
agent_id
provider: feishu | lark
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
Multigent IM provider receives event
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
channel: feishu | lark
external_actor_id: <feishu_or_lark_open_id>
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

Final product effect:

- User opens an agent page and clicks `Connect Feishu` or `Connect Lark`.
- Multigent shows a QR-code setup flow. The user scans with Feishu/Lark and approves.
- Multigent stores the app/bot connection, binds it to the current agent, and maps the scanning IM user to the current Multigent user.
- The agent page shows `Connected to Feishu` or `Connected to Lark`, including bot/chat status and last activity.
- The user messages the created/bound Feishu/Lark app bot.
- Multigent receives the event, authenticates the external user, checks RBAC, resumes the agent's runtime session, and replies through the same IM bot.
- Web Chat, Feishu/Lark, task, cron, and wakeup share one interaction/session lock model so one agent does not receive conflicting mutable work.

Provider naming:

- `feishu` and `lark` are separate providers in product UI, API, audit, and stored connection data.
- They may share the same implementation package because their OpenAPI/event model is similar.
- User-facing text should say Feishu or Lark, never cc-connect.

### Phase 0: Remove External IM Dependency From The Product Path

Goal: Multigent owns the whole Feishu/Lark connection and message execution path. No external cc-connect service should be required or visible.

Code changes:

- Remove the user-facing cc-connect API URL/token settings from the Web settings flow.
- Remove or hide `IMConnectionPanel` behavior that creates projects in an external cc-connect instance.
- Remove backend routes whose only job is proxying to an external cc-connect service.
- Do not keep a compatibility layer that calls a cc-connect server.
- Replace UI wording from "cc-connect" to "Feishu/Lark connection".
- Keep platform-specific behavior behind `internal/imbridge.Provider` so the API layer does not know Feishu/Lark envelope details.

Acceptance:

- Users are never asked to configure a cc-connect endpoint.
- Agent pages do not expose work directories, external cc-connect projects, or agent runtime types as IM setup concepts.
- The only visible setup concept is connecting an agent to Feishu/Lark.
- There is no backend request path that requires a cc-connect process to be running.
- Feishu and Lark are separate provider IDs in UI, API, persisted data, and audit logs.

### Phase 1: Native Agent Channel Connection

Goal: on an agent detail page, a user can connect that specific agent to Feishu or Lark by scanning a QR code. After connection, the same page shows that Feishu/Lark is connected.

This phase is about setup and binding only. It should not run the agent yet.

User flow:

1. User opens `Project → Agent detail`.
2. User clicks `Connect Feishu` or `Connect Lark`.
3. Multigent opens a QR-code modal for the selected provider.
4. User scans the QR code with Feishu/Lark and approves.
5. Multigent receives the setup result and stores:
   - provider: `feishu` or `lark`
   - app id / app secret in encrypted connection secrets
   - bot/app metadata in non-secret profile fields
   - agent binding: workspace, project, agent, provider, bot/chat identifiers
   - external identity mapping from the scanning Feishu/Lark user to the current Multigent user
6. Agent detail page refreshes and shows `Connected to Feishu` or `Connected to Lark`.

Backend modules:

```text
internal/imbridge/
  providers.go      # provider registry and channel-neutral interface

internal/imbridge/lark/
  setup.go          # Feishu/Lark QR setup and setup polling
  events.go         # provider event envelope parsing
  client.go         # send replies through Feishu/Lark OpenAPI

internal/api/
  agent_channel_handlers.go  # connect/disconnect/list channel state
  agent_channel_events.go    # public event callback and message dispatch
```

HTTP API:

```text
GET    /api/v1/projects/{project}/agents/{agent}/channels
POST   /api/v1/projects/{project}/agents/{agent}/channels/{provider}/setup/begin
POST   /api/v1/projects/{project}/agents/{agent}/channels/{provider}/setup/poll
DELETE /api/v1/projects/{project}/agents/{agent}/channels/{provider}

POST   /api/v1/im/{provider}/events
```

Data model:

```text
connections
  provider: feishu | lark
  auth_type: app_secret
  owner_type: workspace
  profile_json: base_url, app_id, bot/app metadata

connection_secrets
  encrypted app_id
  encrypted app_secret
  encrypted verification token / encrypt key when configured

agent_channel_bindings
  workspace_id
  project_id
  agent_id
  provider: feishu | lark
  connection_id
  external_bot_id
  external_chat_id
  external_owner_id
  status: connected | disconnected | error
  metadata_json

external_identities
  workspace_id
  provider: feishu | lark
  external_user_id
  user_id
```

Frontend:

- Replace the old cc-connect panel with a native agent channel panel.
- Show Feishu and Lark as provider choices.
- Disconnected state: show provider name and a connect button.
- Connecting state: show QR modal, progress, timeout/failed state, and retry.
- Connected state: show provider, connected user, bot/chat status, last activity, callback status, and disconnect.
- The page should not ask the user for work directories, runtime names, or cc-connect configuration.

Acceptance:

- A user can complete QR scan from the agent detail page.
- The agent page displays connected Feishu/Lark status after setup.
- The connection survives page refresh and server restart.
- The scanning Feishu/Lark user is mapped to the current Multigent user.
- Disconnect removes the binding and writes an audit event.
- No cc-connect endpoint, token, project, or external runtime setting is visible in this flow.

### Phase 2: Feishu/Lark Message Callback

Goal: after connection, the created/bound Feishu/Lark app bot can receive user messages and route them to the correct Multigent agent.

This phase turns the connected bot into a usable conversation channel.

Inbound message flow:

```text
Feishu/Lark user sends message to bot
  ↓
Feishu/Lark calls /api/v1/im/{provider}/events
  ↓
Multigent verifies event token/signature/encryption when configured
  ↓
Multigent parses message, sender, chat, app id, message id
  ↓
Multigent resolves agent_channel_binding by workspace + provider + app/chat metadata
  ↓
Multigent maps external sender to a Multigent user
  ↓
Multigent checks whether the user can operate this agent
  ↓
Multigent checks whether the message should be handled
  - direct chat: handle
  - bound group chat: handle
  - unbound group mention/reply: handle and bind when appropriate
  - unrelated group message: ignore
  ↓
Multigent acquires the agent interaction lock
  ↓
Multigent records the incoming message in the transcript
  ↓
Multigent resumes or starts the runtime session
  ↓
Multigent replies through the same Feishu/Lark bot
```

Acceptance:

- Feishu/Lark URL verification succeeds.
- Encrypted callbacks work when an encrypt key is configured.
- Verification token mismatch is rejected and audited.
- Unknown external users are ignored instead of running the agent.
- Users without agent operator permission cannot trigger the agent.
- Group messages are ignored unless the bot is addressed or the group has already been bound.
- Direct bot messages can trigger the agent.
- Agent replies are sent back to Feishu/Lark and recorded in the Web transcript.
- Busy agents return a clear busy reply instead of launching a second conflicting run.

### Phase 3: Web Agent Status And Transcript Convergence

Goal: the agent page becomes the control center for both browser chat and Feishu/Lark intervention.

Frontend requirements:

- Agent channel panel shows one row per provider: provider name, disconnected/connecting/connected/error state, last activity, callback URL, and security status.
- Connected state shows `Connected to Feishu` or `Connected to Lark`.
- The user can disconnect with confirmation.
- The user can update callback security fields, such as verification token and encrypt key, without re-scanning.
- The chat/transcript panel shows Web Chat and Feishu/Lark messages in the same session timeline.
- If a Feishu/Lark-triggered run is active, the Web page shows the active run status and prevents conflicting manual actions.

Acceptance:

- After QR setup, refreshing the agent page still shows the connected provider.
- Sending a message in Feishu/Lark updates last activity on the Web page.
- Web transcript clearly distinguishes user, agent, and system events with source metadata.
- UI copy never mentions cc-connect.

### Phase 4: Runtime Session Quality

Goal: IM intervention should feel like talking to the same working agent, not like starting a brand-new one-off command each time.

Runtime requirements:

- Persist runtime session IDs per agent/session so Codex or other CLIs can resume context.
- Store provider/channel source metadata with each transcript event.
- Enforce one mutable run per agent session with an interaction lock.
- Keep agent credentials and CLI session files isolated per agent sandbox.
- Reuse the same execution path for Web Chat and Feishu/Lark messages.
- Add timeout and cleanup for long-running interactive sessions.

Acceptance:

- A Feishu/Lark reply can continue the prior agent context when the runtime supports resume.
- One agent cannot read another agent's CLI session files or credentials.
- A task run and an IM run cannot concurrently mutate the same agent session.
- Runtime failures are visible in both audit logs and transcript/system events.

### Phase 5: Production Hardening

Goal: make the channel usable by a real customer workspace, not only by local demos.

Hardening requirements:

- Add structured logs for setup, callback, binding resolution, permission denial, run start, reply success, and reply failure.
- Add retry policy for transient Feishu/Lark API failures.
- Add audit events for connect, disconnect, security update, message received, permission denied, busy, run completed, and reply failure.
- Add admin-visible diagnostics for callback URL, latest callback time, latest callback error, and missing security configuration.
- Add tests for provider registry, setup persistence, encrypted callbacks, token verification, group addressing, RBAC denial, and busy-session behavior.

Acceptance:

- A support engineer can diagnose why a Feishu/Lark message did not trigger an agent from Web status, audit logs, and server logs.
- Provider-specific code remains behind `internal/imbridge`, so adding another IM provider does not require rewriting API/session code.

## Open Questions

- Should a Feishu group binding target exactly one agent, or allow commands to route to multiple agents in the same project?
- Should users be able to create a task directly from a Feishu thread before running the agent?
- For Claude Code, do we require persistent containers for interactive sessions, or start with resume-based one-turn execution?
- Do we need rich Feishu/Lark card rendering in the first version, or is plain text enough?
- Should every human intervention produce a required review prompt, or only sessions above a certain duration/token threshold?

## Decision

The recommended first product path is:

1. Do not keep cc-connect as a separate required service.
2. Implement Feishu/Lark as native Multigent channel providers, using external code only as reference when useful.
3. Make Multigent own interaction sessions, locks, runtime execution, permissions, credentials, and audit.
4. Treat Feishu/Lark as a first-class interaction channel, not a separate agent runtime.
5. Make human intervention a normal part of agent collaboration, then help teams distill repeated intervention into durable agent capability.
