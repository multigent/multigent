# IM Agent Collaboration E2E Test Plan

## Goal

Validate that Multigent agents can collaborate through Feishu/Lark like autonomous coworkers, not passive webhooks.

The first testing scope is single-user, single-agent E2E. Later phases expand to multi-user, multi-agent, group chat, permissions, and auditability.

Primary goals:

- Basic capability works end to end.
- Agents can receive attention signals from IM channels and wake up autonomously.
- Agents can decide whether and how to respond.
- Agents can use IM channels to actively reply, notify, request decisions, and continue work.
- IM identity, Multigent user identity, permission checks, and audit records remain traceable.
- The user experience feels usable and natural, not like a brittle trigger system.

## Test Environment

Initial local test target:

- Workspace: `spaceship`
- Project: `github-sandbox`
- Agent: `pm`
- Channel: Feishu/Lark direct chat
- User: the locally authorized Lark user

Follow-up environments:

- Feishu/Lark group chat with the same agent.
- Multiple Multigent users bound to Feishu/Lark identities.
- Multiple agents connected to different collaboration channels.
- Remote SaaS and TapNow deployment after local validation.

## Phase 1: Single User Basic Loop

Purpose: verify that one user can talk to one agent and the agent can process IM attention signals.

Test cases:

1. Send a normal direct-message question to the agent.
   - Expected: the message is accepted, a lightweight received signal is visible if configured, and the agent replies after wakeup.

2. Send three short messages in quick succession.
   - Expected: the signals are debounced and merged into one wakeup instead of creating three independent runs.

3. Ask the agent about its role and current project context.
   - Expected: the agent understands who asked, which channel produced the signal, and what project context it should use.

4. Ask the agent to inspect current Multigent tasks.
   - Expected: the agent uses available runtime tools such as `mga` to query task state and summarizes the result back through IM.

5. Ask the agent to send a message back through Feishu/Lark.
   - Expected: the agent can actively use the collaboration channel instead of relying on system-generated replies.

6. Ask the agent to perform a risky or unauthorized action.
   - Expected: the agent refuses, asks for confirmation, or explains missing permission instead of executing blindly.

## Phase 2: Task And Workflow Loop

Purpose: verify that IM is connected to actual work, not only chat.

Test cases:

1. Ask the agent to create or advance a test task.
   - Expected: the task is created or updated, and the initiator/source is auditably recorded.

2. Ask the agent to list workflow human gates waiting for action.
   - Expected: the agent returns actionable decision context, not raw IDs only.

3. Ask the agent to send a decision request to the user.
   - Expected: the agent can use a suitable message format, such as text, markdown, or card, based on the situation.

4. Click or reply to a workflow decision request.
   - Expected: the workflow is advanced only if the represented Multigent user has permission to act on that node.

5. Check consistency after workflow advancement.
   - Expected: task state, workflow step state, run records, audit records, and IM reply all describe the same result.

## Phase 3: Group Chat Collaboration

Purpose: verify that an agent can participate in group context without becoming a noisy trigger bot.

Test cases:

1. Send normal group discussion without mentioning the agent.
   - Expected: the message can be stored as a lower-priority attention signal, but should not force immediate noisy work.

2. Mention the agent in a group chat.
   - Expected: the agent recognizes the group, sender, mention context, and replies to the correct conversation.

3. Multiple users mention the agent in a short time window.
   - Expected: the signals are merged; the agent handles them in one coherent wakeup or clearly prioritizes them.

4. The agent replies to a group message.
   - Expected: it replies to the right group and, when supported, references the source message or mentions the requester.

5. An unbound user gives an operational command in a group.
   - Expected: normal conversation may be allowed, but privileged operations are denied or require identity binding.

## Phase 4: Permission And Audit

Purpose: verify that "can chat" and "can operate resources" are different.

Test cases:

1. A bound user requests an operation on an allowed project.
   - Expected: allowed.

2. A bound user requests an operation on a project they cannot access.
   - Expected: denied with a clear reason.

3. An unbound user sends a direct-message command.
   - Expected: the agent can answer general questions, but cannot perform privileged actions.

4. A user tries to approve a workflow node assigned to someone else.
   - Expected: denied by workflow permission checks.

5. Inspect audit records after an operation.
   - Expected: records include IM provider, chat type, external sender identity, mapped Multigent user, agent, action, resource, result, and timestamp.

## Phase 5: Experience Quality

Purpose: make the experience feel like working with a capable coworker.

Quality checks:

- Response latency is acceptable for direct chat.
- Acknowledgement is lightweight and does not spam the chat.
- Short questions get short answers.
- Long answers are structured, readable, and not forced into cards.
- Markdown is rendered through the best native channel format where available.
- Cards are used for choices, approvals, delegation, or structured actions, not for every reply.
- The agent knows who is speaking and can reference that identity naturally.
- The agent can decide whether to reply, delay, refuse, ask for clarification, or escalate.
- Errors are human-readable and do not expose raw internal stack traces by default.
- Consecutive messages do not create duplicated runs or conflicting replies.

## Phase 6: Multi-User And Multi-Agent Expansion

Purpose: cover realistic team collaboration after the single-user path is stable.

Test dimensions:

- One user talks to multiple agents.
- Multiple users talk to one agent.
- Multiple users mention one agent in a group.
- One agent delegates or coordinates with another agent.
- Two agents share the same group but have different responsibilities.
- Users with different project roles attempt the same operation.
- Agents connected to different IM apps should still use the same attention-signal lifecycle.

Expected behavior:

- Identity binding is per external account and can map back to a Multigent user.
- Permissions are checked against the mapped Multigent user, not only the IM app.
- Agents do not receive unrestricted authority simply because they are in a chat.
- Attention signals preserve provider-specific metadata while exposing a provider-neutral interface to the agent.

## Acceptance Criteria For The First Local Pass

The first local pass is considered complete when:

- Direct chat from the authorized user wakes `github-sandbox/pm`.
- Multiple quick messages are merged into one wakeup.
- The agent can identify the sender and channel.
- The agent can query Multigent task/workflow state.
- The agent can reply through Feishu/Lark without system-injected branding text.
- The agent can send or process at least one workflow-related decision request.
- Unauthorized or unbound operations are blocked.
- Audit records are sufficient to explain who asked for what and what happened.

## Notes

- Do not hard-code Feishu/Lark-specific behavior into the generic attention-signal lifecycle.
- IM-specific features, such as replies, mentions, reactions, file messages, and cards, should live behind provider/channel interfaces.
- Agents should receive capabilities and context, not hidden trigger behavior that bypasses their autonomy.
- System-level safety checks still need to enforce permissions even when the agent chooses to act.
