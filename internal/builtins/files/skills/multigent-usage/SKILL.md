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

# Defer a task until a future time.
mga task add --title "Check deployment" --prompt "Check the deployment status and report back." --not-before 30m

# Schedule a one-shot future wakeup/reminder for yourself.
mga wakeup schedule --in 10m --title "Reminder" --message "Remind owner-a to review the PR"
mga wakeup schedule --at "2026-08-26 15:30" --prompt "Follow up on the human request and reply in the original channel if needed."

# Prefer task templates for standard workflow tasks. Templates bind the workflow,
# actor routing, labels, priority, and prompt shape so agents do not pick the
# wrong workflow or assignee.
mga task templates --format table
mga task create-from-template <template-id> \
	--input repo=owner/repo \
	--input issue_number=123

# Dispatch a standard task into another project when the current agent is a
# member of that project. The target project is explicit and is permission
# checked by Multigent.
mga task templates --project <target-project> --format table
mga task create-from-template <template-id> --project <target-project> \
	--input repo=owner/repo

# Update task state or metadata.
mga task set <task-id> --status in_progress
mga task set <task-id> --summary "Current progress"

# Complete a regular non-workflow task.
mga task complete --id <task-id> --status success --summary "What was actually done"
mga task complete --id <task-id> --status failed --error "Failure reason"

# Workflow tasks must submit every required output field structurally.
# Prefer --output-json, especially when field names contain spaces or non-ASCII
# characters such as Chinese labels.
mga task step done --id <task-id> --status success \
  --summary "One-line completion summary" \
  --output-json '{"product_spec_doc_id":"doc-...","acceptance_criteria_doc_id":"doc-..."}'

# Repeated --output field=value is only safe for simple ASCII field names with
# no spaces.
mga task step done --id <task-id> --status success \
  --output product_spec_doc_id="doc-..."

# Ask for human or agent confirmation.
mga task confirm-request --id <task-id> --summary "Decision needed" --action-item "Approve X" --action-item "Reject with reason"

# Cancel a task.
mga task cancel <task-id> --reason "No longer needed"
```

Use `mga wakeup schedule` instead of a model provider's built-in timer when a
human asks you to remind them later or when you intentionally defer work. The
scheduled wakeup is tracked by Multigent, visible in the task queue, and respects
the agent's heartbeat window.

## Message Commands

```bash
# Inspect valid recipients before messaging humans or other agents.
mga contacts list

# Read this agent's mailbox.
mga inbox messages
mga inbox list --archived

# Send a non-blocking message.
mga inbox send --to human --subject "Update" --body "Message body"
mga inbox send --to <project>/<agent> --subject "Context" --body "Details"
mga inbox send --to <username-or-email> --subject "Question" --body "Details"

# Reply to a received message.
mga inbox reply <message-id> --body "Reply body"
```

`mga message ...` and `mga messages ...` are aliases for `mga inbox ...`.
For human recipients, use `mga contacts list` and pass the returned `identity`
value when possible. Multigent also accepts exact email, display name, or
`Display Name (username)` forms and resolves them to the stable username.

## Runtime Tool Connections

Use `mga runtime connections` to inspect the tools granted to this agent. The response includes `tools`, `recommendedAdapter`, adapter details, skills, actions, and connection aliases.

```bash
mga runtime tools --format table
mga runtime skill-guide
mga runtime connections --format table
mga runtime action --connection <alias> --data '{"method":"GET","endpoint":"/path"}'
mga runtime action --connection <alias> --data '{"method":"POST","endpoint":"/repos/<owner>/<repo>/releases/<release-id>/attach_files"}' --upload file=dist/app.tar.gz
mga runtime version --check
```

Rules:

- Start with `mga runtime skill-guide`. It is generated from the tools enabled for this agent and explains whether each tool should be used through a platform CLI, HTTP action, MCP Gateway, or skill-only instructions.
- Prefer the provider's recommended adapter:
  - `cli`: use the platform CLI and bundled skill, for example `gh` or `lark-cli`.
  - `http_action`: use `mga runtime action --connection <alias>` so Multigent can enforce authorization and audit usage.
  - `mcp_gateway`: Use `mga runtime gateway list-tools` and `mga runtime gateway call-tool` when the task needs an external MCP tool granted to this Agent.
  - `skill_only`: follow the bundled skill; no executable tool is configured.
- Use connection aliases from `mga runtime connections` when calling runtime proxies.
- For HTTP file uploads, use `mga runtime action --connection <alias> --data '<request-json>' --upload <field>=<path>`; add `--form key=value` for multipart text fields. Do not build multipart bodies inside JSON.
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
