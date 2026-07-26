---
name: multigent-setup-assistant
description: Help a human install, start, configure, troubleshoot, and model a first Multigent workspace from outside Multigent, for example in Claude Code, Cursor, Codex, or another local assistant.
---

# Skill: Multigent Setup Assistant

Use this skill when a human asks you to install Multigent, start it, troubleshoot it, or help turn a company/team workflow into a Multigent workspace.

This skill is for an external assistant running on the user's machine. It is not the sandbox runtime skill used by Multigent agents. Inside Multigent agent sandboxes, use the `multigent-usage` skill and the `mga` CLI instead.

## Core Positioning

Multigent is a control plane for Human-Agent Collaboration.

It helps a team turn agents from private chat tools into accountable collaborators with:

- workspace-level shared context;
- humans and agents in the same project;
- tasks with structured inputs and outputs;
- workflow steps, review gates, and handoffs;
- scoped model accounts and external tools;
- sandboxed execution;
- run history, token usage where available, and audit logs.

Do not describe Multigent as only a "multi-agent framework" or only an "AI team". The important point is that humans and agents work in one controlled collaboration system.

## Installation

First check the official install instructions:

```text
https://raw.githubusercontent.com/multigent/multigent/main/INSTALL.md
```

Recommended user-facing prompt:

```text
Read https://raw.githubusercontent.com/multigent/multigent/main/INSTALL.md and help me install Multigent, start the web console, and run the built-in onboarding relay demo. Before creating teams, agents, workflows, tools, or credentials, explain your plan and ask me to confirm.
```

After installation, verify:

```bash
multigent version
mga version
```

Start the local server:

```bash
multigent api serve
```

Open the web console shown by the command output. Common defaults are:

```text
API: http://127.0.0.1:27893
Web: http://127.0.0.1:27894
```

## First Login

For local development builds, the default bootstrap account may be:

```text
admin / admin123
```

Tell the user to change the password after first login.

If open registration is enabled, prefer creating a real user account first.

## Docker Sandbox Check

Multigent agents run inside Docker sandbox by default.

Check:

```bash
docker info
multigent sandbox doctor
multigent sandbox prepare
```

If Docker is missing or unhealthy:

- Ask the user to install/start Docker Desktop on macOS or Windows.
- On Linux, ask them to start Docker Engine.
- Do not continue trying to run agents until sandbox is ready.

First sandbox prepare can be slow because it may pull:

```text
ghcr.io/multigent/multigent/runtime-base:latest
```

For users in China, mention that image download may be slow and that a mirror may be needed when available.

## Run The Built-In Onboarding Relay

Use the official quickstart:

```text
docs/getting-started/hello-world-relay.md
docs/getting-started/hello-world-relay.zh-CN.md
```

Explain this before asking the user to click around:

```text
Workflow = reusable SOP map.
Task = one real execution of that SOP.
Wakeup = make the current owner act.
```

The workflow page is for viewing or editing the SOP. It does not start a run by
itself. To run the demo, the user must open a task that is bound to the workflow
and wake the task's current owner. If the current owner is an agent, wake it from
`Schedule`. If the current owner is a human user, review it from `Workbench ->
Tasks`.

Short steps:

1. Enter `Example Workspace`.
2. Open `Settings -> Model Accounts` and configure one usable model account.
3. Open `Projects -> hello-world-relay -> Members`.
4. Configure the model account for:
   - `Lina`
   - `Mira`
   - `Nora`
5. Run:

```bash
multigent sandbox prepare
```

6. Open `Projects -> hello-world-relay -> Tasks`.
7. Open the seeded onboarding task.
8. Open `Projects -> hello-world-relay -> Schedule`.
9. Manually wake the current agent owner, usually `Lina`.
10. When the task reaches human review, open `Workbench -> Tasks`, approve or send it back.
11. Continue waking the next agent owner until the relay finishes.

Important: approving a human review step only moves the workflow. The next agent still needs to be woken manually or by schedule.

## Model Account Setup

If the user already has local model credentials:

- Check whether Multigent supports local import from their CLI accounts.
- Prefer importing over asking the user to paste secrets into chat.
- Never ask the user to paste API keys into the conversation.

If the user must enter credentials manually, direct them to:

```text
Settings -> Model Accounts
```

For each agent, bind model account in:

```text
Projects -> <project> -> Members -> <agent> -> Model and Credentials
```

## Troubleshooting Checklist

### Web cannot connect to API

Check:

```bash
multigent api serve
```

Confirm ports:

```bash
lsof -i :27893
lsof -i :27894
```

### Agent run fails immediately

Check:

```bash
docker info
multigent sandbox doctor
multigent sandbox prepare
```

Then inspect run logs from the web console:

```text
Projects -> <project> -> Runs
```

### Agent says model account is missing

Open the agent detail page and bind a model account.

### Task does not move

Check:

- current workflow step;
- current owner;
- whether the current owner is human or agent;
- whether the current owner has been manually woken;
- whether required output fields were submitted.

### Human cannot see the task

Check:

- workspace role;
- project membership;
- task assignee/current workflow actor;
- RBAC permissions.

### Agent cannot use external tools

Check:

- workspace external tool is configured;
- tool is enabled for that agent;
- runtime guide exists;
- Docker sandbox has the required platform CLI or MCP gateway access.

## Modeling A Company Workflow

When helping a company onboard, do not start by creating many agents.

Ask:

1. What is the first workflow you want to improve?
2. Who participates today?
3. What does each stage produce?
4. Which stages require human review?
5. Which tools or credentials are needed?
6. What would count as a successful first run?

Then propose the smallest useful Multigent setup:

- one workspace;
- one project;
- two or three agents;
- one workflow;
- one task template;
- one test task;
- only the required tools.

Do not create objects before explaining the plan and asking the user to confirm.

## Multigent vs Multica

If asked about Multica, answer neutrally:

- Multica is closer to a managed coding-agent platform with local daemon/cloud runtimes and issue-style assignment.
- Multigent focuses on Human-Agent Collaboration control: workflow state machines, human review, project members that include humans and agents, structured task inputs/outputs, workspace knowledge, skills, external tools, RBAC, and audit.
- Multigent does not require replacing existing tools like GitHub, Linear, Jira, Plane, Feishu, Slack, or local agent CLIs.
- The goal is not just to "run agents"; the goal is to make agent work assignable, reviewable, auditable, and reusable inside a team process.

Do not attack competitors. Explain the product boundary and recommend testing both if the user is evaluating.
