# Collaboration Workflow State Machine

Multigent workflows are not just task statuses. They describe how humans and agents collaborate around a real work item.

A normal project board might track:

```text
todo -> in progress -> done
```

A Multigent workflow tracks:

```text
input -> agent draft -> human review -> revision loop -> implementation -> QA -> approval -> release
```

Each step can define who acts, what inputs are required, what structured outputs must be produced, how review works, and where the work moves next.

## Core Model

The user-facing work item is still a **Task**. A task may run without a workflow, or it may bind to a workflow definition.

When a task binds to a workflow, Multigent creates a **Workflow Run** under that task. The workflow run records:

- current step
- status
- step history
- inputs and outputs
- actor assignments
- review decisions
- run links and audit records

This keeps task queue semantics separate from collaboration-stage semantics.

## Entities

### Task

The user-visible work item: a requirement, bug, release, customer response, campaign, or investigation.

Task status answers:

```text
Is this task waiting, running, blocked, completed, failed, or cancelled?
```

### Workflow Definition

A reusable process template at the workspace level. Examples:

- issue triage to PR
- beta release
- content production
- customer support handling
- human-reviewed research

### Workflow Run

The concrete execution of a workflow for one task.

### Step Definition

A node in the workflow graph. A step can specify:

- title and goal
- actor kind
- recommended role
- required input fields
- required output fields
- allowed transitions
- review loop behavior

### Step Instance

The execution record for one step in one workflow run. It records the actual actor, inputs, outputs, validation result, runtime link, review decision, and timestamp.

## Branches and Review Loops

Transitions should be configurable instead of hard-coded. A review step can route:

- `approve` -> next stage
- `request_changes` -> previous agent step, carrying review comments
- `hold` -> stop and wait for a human decision

This allows the same engine to support software delivery, marketing content, customer operations, business development, and other workflows.

## Metrics

Useful workflow metrics include:

- total task duration
- duration per step
- human wait time
- number of review loops
- number of agent runs
- token usage where available
- failure and retry count
- final outcome

These metrics help teams discover where agent collaboration is actually improving delivery and where more context, better skills, or different review rules are needed.
