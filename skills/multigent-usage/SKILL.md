---
name: multigent-usage
description: Use the mga runtime CLI inside Multigent agent sandboxes for tasks, messages, and granted tool connections.
---

# Skill: Multigent Agent Runtime CLI

Use `mga` inside an agent sandbox to talk to the Multigent Server. Do not use the human/admin `multigent` CLI from a sandbox.

`mga` requires the runtime environment injected by Multigent:

- `MULTIGENT_API_URL`
- `MULTIGENT_AGENT_TOKEN`
- `MULTIGENT_RUN_ID`
- `MULTIGENT_WORKSPACE_ID`
- `MULTIGENT_CONNECTIONS_FILE`
- `MULTIGENT_TOOLS_FILE`
- `MULTIGENT_TOOL_RUNTIME_DIR`
- `MULTIGENT_TOOL_BIN_DIR`
- `MULTIGENT_TOOL_BOOTSTRAP_FILE`
- `MULTIGENT_TOOL_SKILLS_FILE`
- `MULTIGENT_TOOL_CLI_AUDIT_FILE`

## Task Commands

```bash
# List tasks visible to this runtime project.
mga task list --status pending
mga task list --scope all --format table

# Inspect a task.
mga task show <task-id>

# Create a task for yourself or another agent in the same project.
mga task add --agent <agent> --title "Title" --prompt "Detailed instructions" --priority 2 --type chore

# Update task state or metadata.
mga task set <task-id> --status in_progress
mga task set <task-id> --summary "Current progress"

# Mark completion.
mga task done --id <task-id> --status success --summary "What was actually done"
mga task done --id <task-id> --status failed --error "Failure reason"

# Ask for human or agent confirmation.
mga task confirm-request --id <task-id> --summary "Decision needed" --action-item "Approve X" --action-item "Reject with reason"

# Cancel a task.
mga task cancel <task-id> --reason "No longer needed"
```

## Message Commands

```bash
# Read this agent's mailbox.
mga inbox messages
mga inbox list --archived

# Send a non-blocking message.
mga inbox send --to human --subject "Update" --body "Message body"
mga inbox send --to <project>/<agent> --subject "Context" --body "Details"

# Reply to a received message.
mga inbox reply <message-id> --body "Reply body"
```

`mga message ...` and `mga messages ...` are aliases for `mga inbox ...`.

## Runtime Tool Connections

Use `mga runtime connections` to inspect the tools granted to this agent. The response includes `tools`, `recommendedAdapter`, adapter details, skills, actions, and connection aliases.

```bash
mga runtime tools --format table
mga runtime skill-guide
mga runtime connections --format table
mga runtime action --connection <alias> --data '{"method":"GET","endpoint":"/path"}'
mga runtime mcp --connection <alias> --data '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
mga runtime gateway list-tools --format table
mga runtime gateway call-tool action:github:get_authenticated_user --data '{}'
mga runtime version --check
```

Rules:

- Start with `mga runtime skill-guide`. It is generated from the tools enabled for this agent and explains whether each tool should be used through a platform CLI, MCP Gateway, HTTP action, or skill-only instructions.
- Prefer the provider's recommended adapter:
  - `cli`: use the platform CLI and bundled skill, for example `gh` or `lark-cli`.
  - `mcp_gateway`: use Multigent MCP Gateway tool discovery/calls.
  - `http_action`: use `mga runtime action`.
  - `skill_only`: follow the bundled skill; no executable tool is configured.
- Prefer `mga runtime gateway list-tools` and `mga runtime gateway call-tool` when you need the unified MCP Gateway broker without writing JSON-RPC manually.
- Use connection aliases from `mga runtime connections` when calling runtime proxies.
- Never ask humans to paste provider secrets into chat.
- Runtime writes are audited by the Multigent Server.
- Platform CLI adapters write best-effort low-sensitive command metadata to `MULTIGENT_TOOL_CLI_AUDIT_FILE`; do not write provider secrets or full sensitive arguments there.
- If a needed command is missing, report the missing capability instead of using local workspace files as a control plane.

## Knowledge Base Docs

```bash
mga docs list
mga docs search "query" --content
mga docs show <doc-id>
mga docs create --title "Runbook" --index "engineering/runbooks" --tags runbook,api --content "# Runbook..."
mga docs create --title "Research note" --file notes.md --index "research"
```

Use docs for durable knowledge: runbooks, decisions, task receipts, research notes, handoffs, and reusable project context.
