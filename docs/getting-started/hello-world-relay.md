# Run the Hello World Relay

The Hello World relay is not a business workflow. It is a neutral demo that proves the core Multigent loop:

```text
Agent takes work -> human reviews -> agent continues -> agent records -> human confirms
```

If you just installed Multigent, run this demo before modeling your real company workflow.

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
Complete a Hello World collaboration relay
```

Open the task detail and confirm it is bound to the Hello World workflow.

## 5. Wake the first agent

Open:

```text
Projects -> hello-world-relay -> Schedule
```

Find `greeter-agent` and click manual wakeup.

The agent reads the active workflow step, creates the first handoff document, and submits structured outputs.

## 6. Review as a human

When the workflow reaches human review, the task owner becomes your user.

Open:

```text
Workbench -> Tasks
```

Review the upstream output. Approve it or send it back with concrete comments.

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

