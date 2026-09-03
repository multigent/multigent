---
name: task-management
description: Create, inspect, update, complete, confirm, and cancel tasks through the mga runtime CLI.
---

# Skill: Task Management

Use `mga task` for task work inside an agent sandbox. The CLI talks to the Multigent Server with the scoped runtime token; it does not read or write workspace control files directly.

## Priority

| Value | Label | Use |
| --- | --- | --- |
| 0 | critical | Production/blocking issue |
| 1 | high | Should run in the current cycle |
| 2 | normal | Default backlog work |
| 3 | low | Nice-to-have |

## List And Inspect

```bash
mga task list
mga task list --status pending
mga task list --scope active --format table
mga task show <task-id>
```

## Create

```bash
mga task add \
  --agent <agent> \
  --title "Short title" \
  --prompt "Detailed instructions and acceptance criteria" \
  --priority 2 \
  --type chore

# Defer execution until a future time.
mga task add \
  --title "Check rollout" \
  --prompt "Check the rollout and report the result." \
  --not-before 30m

# One-shot self wakeup/reminder.
mga wakeup schedule --in 10m --title "Reminder" --message "Remind owner-a to review the PR"
mga wakeup schedule --at "2026-08-26 15:30" --prompt "Follow up with the human in the original channel."
```

If `--agent` is omitted, the task is assigned to the current agent. Agents may create tasks for other agents in the same project.

Use `mga wakeup schedule` instead of the model provider's built-in timer when a
human asks you to remind them later or when you decide to defer work. Multigent
stores the reminder as a visible scheduled task and wakes the agent after the
task's `notBefore` time.

For standard workflow tasks, prefer task templates instead of manually choosing a workflow and assignees:

```bash
mga task templates --format table
# A dispatcher may inspect a template in another project only when it is a
# member of that project.
mga task templates --project <target-project> --format table
mga task create-from-template <template-id> \
	--project <target-project> \
	--input repo=owner/repo \
	--input issue_number=123
```

Templates are maintained by the workspace and include the workflow definition, actor bindings, labels, priority, and prompt shape.

## Update

```bash
mga task set <task-id> --status in_progress
mga task set <task-id> --summary "Progress so far"
mga task set <task-id> --priority 1
```

Supported statuses:

- `pending`
- `in_progress`
- `awaiting_confirmation`
- `blocked`
- `done_success`
- `done_failed`
- `cancelled`

## Complete

```bash
mga task complete --id <task-id> --status success --summary "What was accomplished"
mga task complete --id <task-id> --status failed --error "Reason"

# If the task is inside a workflow, submit every required workflow output field
# with --output-json. Field names are validated by the server.
# Use repeated --output field=value only for simple ASCII field names with no
# spaces; Chinese labels or field names with spaces must use --output-json.
mga task step done --id <task-id> --status success \
  --summary "One-line completion summary" \
  --output-json '{"technical_spec_doc_id":"doc-...","test_plan_doc_id":"doc-..."}'
```

Completion summaries must describe actual output, produced files/links, residual risks, and whether human follow-up is needed.

## Confirmation

```bash
mga task confirm-request \
  --id <task-id> \
  --summary "One-line decision needed" \
  --action-item "Approve option A" \
  --action-item "Reject with reason"
```

Use confirmation only for decisions that should not be automated: money, external publication, irreversible production actions, policy exceptions, or unclear strategic direction.

## Cancel

```bash
mga task cancel <task-id> --reason "No longer needed"
```
