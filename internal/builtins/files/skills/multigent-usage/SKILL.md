---
name: multigent-usage
description: Use the Multigent agent runtime CLI from inside a sandbox to coordinate tasks, messages, OKRs, and granted tools through the server API.
---

# Skill: Multigent Agent Runtime CLI

Use this skill when you need to coordinate with Multigent from inside an agent sandbox.

The command for agents is:

```bash
multigent-agent
```

Do not use the human/admin `multigent` CLI from a sandbox. Human users operate Multigent through the Web UI; agents use `multigent-agent`, which talks to the Multigent Server with the scoped runtime token injected into the sandbox.

## Runtime Identity

Multigent injects these environment variables into each run:

```bash
MULTIGENT_API_URL
MULTIGENT_AGENT_TOKEN
MULTIGENT_RUN_ID
MULTIGENT_WORKSPACE_ID
MULTIGENT_PROJECT
MULTIGENT_AGENT
```

Do not ask humans for API tokens or provider secrets. If a command fails with a permission error, report which permission or grant is missing.

## Completion

When a task is complete:

```bash
multigent-agent task done \
  --id "$TASK_ID" \
  --status success \
  --summary "What was actually completed and where the result is."
```

If the task failed:

```bash
multigent-agent task done \
  --id "$TASK_ID" \
  --status failed \
  --error "Concrete reason and attempted fixes."
```

If human input is required:

```bash
multigent-agent task confirm-request \
  --id "$TASK_ID" \
  --summary "Decision needed in one line" \
  --action-item "Option A: ..." \
  --action-item "Option B: ..."
```

Only request human confirmation when the action is blocked by policy, missing authority, external publication, real money, or an irreversible decision.

## Task Work

Common task operations:

```bash
multigent-agent task list --status pending
multigent-agent task show <task-id>
multigent-agent task add \
  --project <project> \
  --agent <agent> \
  --title "Short title" \
  --prompt "Detailed, actionable instructions"
multigent-agent task set <task-id> --status in_progress
multigent-agent task retry <task-id>
multigent-agent task cancel <task-id> --reason "Reason"
```

Use task operations to create follow-up work instead of burying new work inside a chat response.

## Messages

Use messages for async coordination:

```bash
multigent-agent inbox send \
  --from "$MULTIGENT_PROJECT/$MULTIGENT_AGENT" \
  --to <project>/<agent> \
  --subject "Short subject" \
  --body "Context, request, and expected next step."

multigent-agent inbox messages --recipient "$MULTIGENT_PROJECT/$MULTIGENT_AGENT"
multigent-agent inbox reply <msg-id> --body "Reply"
```

Keep messages concise and include enough context for the recipient to act without asking for clarification.

## OKRs

If your task clearly advances an OKR, update the relevant progress or add a review note:

```bash
multigent-agent okr list
multigent-agent okr show <okr-id>
multigent-agent okr update <okr-id> --status on_track
multigent-agent okr review --okr <okr-id> --note "Progress made and evidence."
```

Do not invent OKR changes. If you are unsure whether a task maps to an OKR, mention that uncertainty in the task summary.

## Runtime Connections

Use runtime connection commands for granted external tools:

```bash
multigent-agent runtime connections --format table
multigent-agent runtime action --connection <alias> --data '{"method":"GET","endpoint":"/path"}'
multigent-agent runtime mcp --connection <alias> --data '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

Rules:

- Use only connection aliases shown by `runtime connections`.
- Do not read or expose raw provider credentials.
- If a required connection is missing, report the missing provider and target agent.

## Output Discipline

Prefer structured output when available:

```bash
multigent-agent task list --json
```

When reporting completion, include:

- actual work done
- files, links, or artifacts produced
- decisions made
- open questions
- risks
- next suggested step
