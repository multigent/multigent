# Workflow Collaboration Accounts

This document defines how Multigent connects workflow human steps with external collaboration systems such as Feishu, Lark, DingTalk, Slack, or future channels.

The goal is simple: when a workflow reaches a human step, the right person should be notified in the collaboration tool they already use, and their action should flow back into the workflow.

## Product Principles

Multigent should not expose "trigger configuration" as a heavy standalone concept for normal users.

Users already understand these objects:

- External tools: where a workspace connects Feishu, Lark, GitHub, MCP servers, and other tools.
- Users: human members in a workspace.
- Workflow nodes: steps in a process.
- Task templates and tasks: where actual people or agents are assigned.

Workflow-triggered collaboration should be expressed through those existing objects.

## Core Concepts

### External Tool

An external tool connection provides the technical capability.

For example, a workspace admin connects Feishu by configuring:

- App ID
- App Secret
- Long connection / WebSocket event capability
- Required event subscriptions

This means Multigent can send messages and receive interaction events from Feishu.

The external tool does not decide when to notify someone. It only provides the channel capability.

### Collaboration Account

A collaboration account is the mapping between a Multigent user and that user's identity inside an external collaboration tool.

For example:

```text
Multigent user: dashell
Provider: feishu
Connection: workspace Feishu app
External identity: ou_xxx
```

This is not a global user property. It belongs to a workspace and an external tool connection.

The same person may have different external identities across different workspaces or different enterprise apps.

### Workflow Human Node

A workflow human node defines that a step requires human action.

At workflow design time, the node may define:

- This is a human review or human task node.
- Whether the node should notify the actual assignee when reached.
- Which channel strategy to use: auto, Feishu, Lark, or later DingTalk/Slack/Webhook.

The workflow definition usually does not know the concrete assignee yet.

### Task Template and Task

The concrete assignee is resolved at the task template or task instance level.

For example:

```text
Workflow node: Product review
Task template assignment: Dashell
Task instance assignment: Dashell, or overridden to Nicole
```

Only after a task uses a workflow and assigns node owners can Multigent know who should receive the external notification.

## Recommended User Journey

### 1. Workspace Admin Connects an External Tool

The admin opens External Tools and connects Feishu or Lark.

The setup should verify:

- The app credentials are valid.
- The long connection can start.
- Required events are enabled, including normal messages and card actions.

If this is not configured, workflow human nodes can still work inside Multigent, but external cards cannot be sent.

### 2. Workspace Users Bind Collaboration Accounts

Users bind their own collaboration accounts from account settings.

If Feishu is configured for the workspace, a user can see:

```text
Collaboration accounts
Feishu: Not bound  [Bind]
Lark: Not configured
```

If the workspace does not use Feishu, Feishu should not appear as a binding requirement.

Workspace admins should inspect binding status from the Users page, because this is a member management concern:

```text
Users
Dashell   Admin    Feishu bound
Glenn     Member   Feishu not bound
Nicole    Member   Lark bound
```

The external tool page should focus on tool health and connection configuration, not on managing every user's identity binding.

### 3. Workflow Designer Enables Notification on Human Nodes

In the workflow node editor:

```text
Human review
[x] Notify assignee when this node is reached
Channel: Auto
```

The user should not need to understand "trigger rules".

They only decide whether this human node should notify the assigned person.

### 4. Task Template Assigns Node Owners

When creating a task template from a workflow, the user maps workflow nodes to actual assignees:

```text
Product review -> Dashell
QA review      -> Ben
Final approval -> Owner
```

At this point Multigent can warn if a selected human assignee has not bound the required collaboration account:

```text
Dashell has not bound Feishu. This node will fall back to in-app notification.
```

### 5. Task Runs and Enters a Human Node

When the workflow reaches the human node, Multigent resolves:

1. The active workflow step.
2. The actual task-level assignee.
3. The enabled notification policy on the node.
4. The available external tool connection.
5. The assignee's collaboration account mapping.

Then Multigent sends an interactive card to the assignee.

The card should allow:

- Open task details in Multigent.
- Approve.
- Request changes.

For Feishu/Lark, card action callbacks should come back through long connection events instead of requiring a public callback URL.

### 6. User Action Advances the Workflow

When the user clicks approve or request changes in the card, Multigent should:

1. Verify the card action came from the configured external app.
2. Map the external user ID back to a Multigent user.
3. Confirm that user is allowed to operate the current task/node.
4. Confirm the task is still waiting at the same workflow node.
5. Write structured review output.
6. Advance the workflow.
7. Record audit logs and delivery status.

## Binding Flow

The first implementation should use verification-code binding through the collaboration bot.

1. User clicks "Bind Feishu" in account settings.
2. Multigent creates a short-lived binding session:

```text
workspace_id
provider
connection_id
multigent_user_id
code
status: pending
expires_at
```

3. UI asks the user to send the code to the Feishu/Lark bot:

```text
Send this code to the Multigent bot in Feishu:
MG-839214
```

4. Multigent receives the message through the long connection.
5. Multigent matches the code and stores:

```text
external_identities
- workspace_id
- provider
- connection_id
- user_id
- external_user_id / open_id
- union_id / email when available
- status
```

6. The bot replies that binding is complete.

This avoids requiring each user to understand OAuth, app IDs, or callback URLs.

## Relationship Between Objects

```text
External Tool Connection
  provides channel capability
  e.g. Feishu app can send cards and receive card actions

Collaboration Account
  maps a Multigent user to an external identity for a connection
  e.g. Dashell -> Feishu open_id

Workflow Node
  declares whether a human step should notify its assignee

Task Template / Task
  resolves the actual human assignee

Workflow Runtime
  sends card, receives action, verifies permission, advances workflow
```

## Permissions

Workspace admins can:

- Configure external tool connections.
- View collaboration account binding status on the Users page.
- Send binding invitations or copy binding instructions.
- Configure workflow node notification defaults.

Workspace members can:

- Bind or unbind their own collaboration accounts.
- Receive and act on tasks they are assigned to.

Agents cannot configure collaboration accounts or external tool credentials.

## Fallback Behavior

If a workflow reaches a human node but external delivery is unavailable:

- The task should still move to the human node.
- In-app notification should still be created.
- Delivery failure should be recorded.
- The task detail should show a clear warning:

```text
Feishu card was not sent because Dashell has not bound a collaboration account.
```

The workflow should not silently fail.

## Future Extensions

The same model can support:

- DingTalk, Slack, WeCom, Telegram, Discord.
- Workflow-level notification overrides.
- Per-node channel selection.
- Escalation after timeout.
- Group card notifications.
- HTTP/Webhook notifiers.
- Approval delegation.
- Multiple external connections per provider.

The product should still keep the user-facing concept simple: assign people in workflow tasks, and Multigent will reach them through their collaboration accounts when needed.

