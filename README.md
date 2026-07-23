<p align="center">
  <img src="docs/assets/banner.svg" alt="Multigent" width="100%">
</p>

<div align="center">

# Multigent

**Agent collaboration infrastructure for teams that want agents to actually deliver.**

Multigent helps teams turn prompts, tools, workflows, and human reviews into a coordinated agent workforce. Keep your existing project tools and chat tools; use Multigent as the control plane that gives agents shared context, structured tasks, safe execution, and observable handoffs.

[中文](README.zh-CN.md) · [Install Guide](INSTALL.md) · [Documentation](docs/README.md)

</div>

## Why Multigent

Most teams already have docs, task boards, repos, chats, meetings, and local coding agents. The hard part is not creating another chat box. The hard part is making agents understand the same context, follow the same process, use the right tools, ask for review at the right moment, and continue work without a human synchronously driving every step.

<p align="center">
  <img src="docs/assets/screenshots/main_pic.png" alt="Multigent visual workflow canvas" width="100%">
</p>

Multigent is built around that operating model:

- **Shared agent context**: workspace, team, role, project, task, docs, skills, tools, and workflow state are managed in one place.
- **Agent-ready task execution**: tasks can bind to workflows, carry structured inputs and outputs, and move between agents and humans.
- **Human review without human blocking**: humans act as owners, reviewers, and process designers instead of being the mandatory runtime loop.
- **External tools as capabilities**: GitHub, Feishu/Lark, Slack, Linear-style project systems, web search, design tools, and other services are modeled as workspace tools that agents can use through controlled runtime adapters.
- **Observable agent work**: runs, chat sessions, workflow steps, task history, tokens, logs, and audit events are visible from the web console.
- **Sandbox-first execution**: agents run in isolated environments with explicit credentials and tool access instead of reading the whole workspace by default.

## Highlights

- **Multi-agent autonomous wakeups**: agents can be woken by tasks, heartbeat schedules, cron jobs, manual triggers, and collaboration events, so work can keep moving without a human constantly copying prompts around.
- **Loop engineering for agent teams**: prompts, skills, tools, model accounts, schedules, and review policies are managed as a repeatable operating loop rather than one-off conversations.
- **Visual SOP and workflow orchestration**: design task flows on a board, define required inputs and outputs, add review loops and branch conditions, then bind real tasks to that workflow.
- **Human-in-the-loop by design**: humans can review, approve, reject, or redirect at key workflow steps while agents continue handling the repeatable work around them.
- **Demo video coming next**: a short product walkthrough will be added as the public demo workspace stabilizes.

## Screenshots

### Visual SOP and workflow orchestration

<p align="center">
  <img src="docs/assets/screenshots/gitHub_beta_stable_workflow.png" alt="Multigent workflow board" width="100%">
</p>

### Multi-agent task board

<p align="center">
  <img src="docs/assets/screenshots/task_panel.png" alt="Multigent multi-agent task board" width="100%">
</p>

### Workflow task detail

<p align="center">
  <img src="docs/assets/screenshots/task_detail.png" alt="Multigent workflow task detail" width="100%">
</p>

## Product Model

```text
Workspace
  -> Teams and roles
  -> Projects
  -> Agents and humans
  -> Tasks
  -> Workflows
  -> Docs, skills, model accounts, and external tools
```

Multigent does not try to replace every existing system on day one. A company can keep using Jira, Linear, Plane, Huly, GitHub, Feishu/Lark, Slack, local agent CLIs, and internal docs. Multigent provides the agent-native coordination layer across those systems.

## Core Features

### Agent Workforce

Create agent teammates with a role, model account, CLI runtime, sandbox, skills, and external tool access. Agents can work through web chat, scheduled wakeups, tasks, and workflow steps.

### Autonomous Wakeups and Loop Engineering

Agents are not limited to synchronous chat. They can run task-triggered wakeups, heartbeat routines, cron jobs, and manual wakeups. Teams can tune the loop around each agent: what context it receives, what tools it can use, when it should ask for review, and how it reports output.

### Workflow Engine

Design reusable workspace-level workflows on a visual board. A workflow defines steps, actors, required inputs, required outputs, review loops, branch conditions, and handoffs. Tasks can then choose a workflow and assign each actor to a human or an agent.

### Playbooks

Install opinionated collaboration packages that bundle teams, roles, skills, and workflows. Playbooks help new workspaces start with a proven process instead of an empty canvas.

### External Tools

Configure tools once at the workspace level and expose them to selected agents. Multigent is designed to support multiple access patterns: platform CLIs, MCP gateways, API keys, OAuth apps, and runtime materialization.

### Knowledge Base

Store and reference documents by doc ID. Agents can create and read docs through the runtime CLI so workflow outputs can point to durable knowledge instead of ephemeral chat text.

### Scheduling and Runs

Use task-triggered wakeups, heartbeat schedules, cron jobs, and manual wakeups. Run records capture status, runtime session IDs, token usage where available, logs, and workflow step outputs.

### RBAC and Audit

Workspace roles, project membership, task visibility, user invitations, and audit events are first-class concepts. Humans and agents are treated as principals with scoped permissions.

## Quick Start

### Recommended: Let Your Agent Install It

The easiest way to try Multigent is to ask your coding agent to install and start it for you. Multigent ships an agent-readable setup guide:

```text
Read https://github.com/multigent/multigent/blob/main/INSTALL.md
Install Multigent on this machine, start the web console, and tell me the local URL.
Before creating teams, agents, workflows, or credentials, explain the plan and ask me to confirm.
```

This works well with Claude Code, Codex, Cursor, and similar local agent environments. The agent can choose the right install path, verify Docker, start the server, and help you configure the first workspace.

### Manual Install

macOS and Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/multigent/multigent/main/scripts/install.sh | bash
multigent version
mga version
```

Homebrew:

```bash
brew install multigent/tap/multigent
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/multigent/multigent/main/scripts/install.ps1 | iex
```

npm wrapper:

```bash
npm install -g @multigent/multigent
```

Docker self-host:

```bash
docker run --rm -p 27892:27892 \
  -v multigent-data:/data \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/multigent/multigent:latest
```

Open `http://127.0.0.1:27892`.

### Prerequisites for Agent Runs

- Docker, for sandboxed agent execution

Multigent publishes the default multi-architecture runtime image at `ghcr.io/multigent/multigent/runtime-base:latest`. The image provides stable sandbox dependencies and a fallback Linux `mga`; normal runs synchronize the `mga` version that matches the Multigent server into the persistent Docker toolchain volume. Native macOS and Windows binaries are never mounted into Linux sandboxes. The published GHCR package is public and does not require `docker login`.

Recommended first-run preparation:

```bash
multigent sandbox prepare
```

This pulls the runtime image and warms common agent CLI toolchains before the
first chat, so users do not confuse a long Docker pull with an agent failure.
The image is intentionally kept as a stable runtime base; fast-moving agent
CLIs are installed into the persistent toolchain cache, and heavier
project-specific compilers belong in custom runtime images or templates.

### Run the Web Console

The production-style command serves the API and embedded frontend from one binary:

```bash
multigent --dir ./data start --addr 127.0.0.1:27892 --open
```

For frontend development with Vite hot reload:

```bash
make build
./dist/multigent --dir ./data api serve --addr 127.0.0.1:27893
cd web
npm install
npm run dev
```

Open the Vite URL shown in the terminal, usually `http://127.0.0.1:27894`.

## First Journey

1. Register the first user.
2. Create a workspace.
3. Invite members or continue alone.
4. Create or install teams, roles, and playbooks.
5. Add a project.
6. Add agents to the project.
7. Configure model accounts and external tools.
8. Create a workflow or use a built-in workflow.
9. Create a task and bind it to the workflow.
10. Watch the task move between agents and humans, with structured outputs recorded at every step.

## Architecture

```text
┌─────────────────────────┐
│      Web Console        │
│  React + workflow UI    │
└───────────┬─────────────┘
            │ HTTP / SSE
┌───────────▼─────────────┐
│      Go API Server      │
│ auth, RBAC, tasks, docs │
│ workflows, tools, runs  │
└───────────┬─────────────┘
            │
┌───────────▼─────────────┐
│      Storage Layer      │
│ SQLite today, interface │
│ ready for other DBs     │
└───────────┬─────────────┘
            │
┌───────────▼─────────────┐
│   Runtime Materializer  │
│ sandbox, CLI, skills,   │
│ credentials, tools      │
└───────────┬─────────────┘
            │
┌───────────▼─────────────┐
│  Isolated Agent Runtime │
│ Codex, Claude Code,     │
│ Cursor, tool CLIs, MCP  │
└─────────────────────────┘
```

For deeper design notes, see the further reading section below.

## Further Reading

- [Documentation index](docs/README.md)
- [Agent runtime CLI architecture](docs/architecture/agent-runtime-cli-architecture.en.md)
- [Workflow state machine](docs/concepts/collaboration-workflow-state-machine.en.md)
- [Configuration and logging](docs/getting-started/configuration-and-logging.md)
- [Release and distribution](docs/operations/release-distribution.md)

## Development

```bash
make test
make web
make build-go
```

Useful commands:

```bash
# Start API only
./dist/multigent --dir ./data api serve --addr 127.0.0.1:27893

# Start API + embedded web
./dist/multigent --dir ./data start --addr 127.0.0.1:27892

# Inspect worker/runtime configuration
./dist/multigent worker inspect
```

Configuration can be supplied through CLI flags, environment variables, or a TOML file. See [configuration and logging](docs/getting-started/configuration-and-logging.md).

## Current Status

Multigent is under active product development. The repository already includes the web console, workspace model, users and invitations, teams and roles, agents, model accounts, external tools, tasks, workflow definitions, scheduler, sandbox runtime abstraction, docs, playbooks, and telemetry.

The near-term focus is making the end-to-end journey production-grade:

- smoother onboarding and example workspaces;
- stronger sandbox isolation and runtime materialization;
- richer workflow execution and visual observability;
- better external tool adapters;
- cleaner product packaging for self-hosted and commercial deployments.

## License

Multigent is source-available under the [PolyForm Noncommercial License 1.0.0](LICENSE).

Commercial use is not permitted without a separate written commercial license from the copyright holder.
