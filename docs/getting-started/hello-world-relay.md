# Run the New Teammate Onboarding Relay

This demo is not an engineering workflow and is not tied to one industry. It uses a small but real task to prove the core Multigent loop: prepare an onboarding note for a teammate who is seeing the workspace for the first time.

```text
Agent takes work -> human reviews -> agent continues -> agent records -> human confirms
```

If you just installed Multigent, run this demo before modeling your real company workflow.

## Understand Three Concepts First

New users often get stuck because they expect the workflow diagram itself to
run. In Multigent, the model is:

```text
Workflow = reusable SOP map
Task = one real execution
Wakeup = make the current owner act
```

A workflow does not run by itself. It defines:

- which steps exist;
- whether each step is handled by a human or an agent;
- what input each step needs;
- what output each step must produce;
- where the task goes after approval, rejection, or branching.

The task is the running object. Once a task is bound to a workflow, it records:

- the current workflow step;
- the current owner;
- upstream outputs;
- the input the next step should receive.

So the shortest way to run this demo is:

```text
Configure model account -> open the workflow-bound task -> wake the current owner -> human review -> wake the next owner
```

If the current owner is an agent, wake it from `Schedule` or wait for
task-triggered heartbeat. If the current owner is you, review it from
`Workbench -> Tasks`.

## 1. Open Example Workspace

After first registration, Multigent creates:

```text
Example Workspace
```

Switch to it from the workspace menu if needed.

The demo project is:

```text
hello-world-relay
```

## 2. Configure a model account

Open:

```text
Settings -> Model Accounts
```

Add one usable model account, then open:

```text
Projects -> hello-world-relay -> Members
```

Configure the same model account for:

- `greeter-agent`
- `responder-agent`
- `recorder-agent`

## 3. Prepare Docker Sandbox

Agents run inside Docker sandbox by default:

```bash
docker info
multigent sandbox prepare
```

The first prepare may take a few minutes because it downloads the runtime image and installs agent CLI toolchains.

## 4. Open the initial task

Open:

```text
Projects -> hello-world-relay -> Tasks
```

Find:

```text
Prepare a new teammate onboarding note
```

Open the task detail and confirm it is bound to the New Teammate Onboarding Relay workflow.

In task detail, focus on three fields:

- **Current step**: what should happen now.
- **Owner**: which agent or human should handle this step.
- **Upstream outputs**: structured handoff from the previous step, often with docID links.

## 5. Wake the first agent

Open:

```text
Projects -> hello-world-relay -> Schedule
```

Find `greeter-agent` and click manual wakeup.

The agent reads the active workflow step, creates the first welcome note and handoff document, and submits structured outputs.

After a successful run, the task is not necessarily done. It moves to the next
workflow step according to the workflow edge. If the next step is human review,
open Workbench and review it.

## 6. Review as a human

When the workflow reaches human review, the task owner becomes your user.

Open:

```text
Workbench -> Tasks
```

Review the upstream output. Approve it or send it back with concrete comments.

Approval moves the task to the next agent step. Sending it back returns the task
to the upstream agent step and carries your comments as the next input.

## 7. Continue the relay

After approval, the task moves to `responder-agent`. Wake the current owner manually or wait for task-triggered heartbeat.

The workflow then continues to:

```text
recorder-agent -> final review
```

## 8. Inspect results

Use:

- `Tasks` for current step, owner, upstream outputs, and structured outputs.
- `Runs` for agent execution history.
- `Knowledge Base` for generated docs.
- `Workflows` for the visual workflow definition.

## Common Blockers

### The task did not move

Open task detail and check the current owner. Wake that exact agent from
`Schedule`, or review it from `Workbench` if the owner is a human user.

### The workflow page opened, but nothing ran

That is expected. The workflow page is the SOP editor. To run the SOP, create or
open a task bound to that workflow.

### Human review passed, but the next agent did not run

Human review only advances the workflow state. The next agent still needs to be
woken manually or by task-triggered heartbeat.

## Replay The Product Tour

To replay the product tour later, open:

```text
Settings -> Start tour
```

The tour walks through model accounts, teams, projects, agents, workflows,
tasks, docs, and scheduling again.
