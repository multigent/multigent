# Agent Runtime CLI Architecture

Multigent separates the control plane from the agent runtime.

- Humans use the web console to manage workspaces, members, teams, projects, agents, credentials, tools, workflows, and audit history.
- Agents run inside isolated sandboxes and talk to Multigent through the scoped runtime CLI, `mga`.
- External capabilities are mounted through runtime adapters: platform CLIs, MCP servers, API credentials, OAuth apps, skills, and tool manifests.

The goal is to keep agent work observable and permissioned without forcing every company to replace its existing project tools, chat tools, repos, or documentation systems.

## CLI Surfaces

Multigent has two CLI surfaces:

| CLI | Primary user | Purpose | Runs inside sandbox |
| --- | --- | --- | --- |
| `multigent` | operators and developers | start the server, configure local/self-hosted deployments, debug, and maintain the installation | no |
| `mga` | agents | read tasks, complete workflow steps, send messages, read/write docs, inspect runtime context, and report outputs | yes |

`mga` is not a full admin CLI. It should not create workspaces, manage global users, start servers, run migrations, or bypass RBAC.

## Runtime Authentication

When an agent run starts, Multigent injects scoped runtime context into the sandbox:

```text
MULTIGENT_API_URL
MULTIGENT_AGENT_TOKEN
MULTIGENT_RUN_ID
MULTIGENT_WORKSPACE_ID
MULTIGENT_PROJECT
MULTIGENT_AGENT
MULTIGENT_CONNECTIONS_FILE
```

`mga` sends every request to the server with:

```http
Authorization: Bearer $MULTIGENT_AGENT_TOKEN
```

The server resolves that token to a workspace, project, agent, run, capability set, and granted tools. Mutating operations are checked by RBAC and recorded in audit logs.

## Why Not Use MCP as the Core Protocol?

MCP is useful for external tool ecosystems, but Multigent's internal collaboration protocol needs stable task, workflow, docs, messaging, and audit semantics. A CLI is better for agent loops that need shell scripting, deterministic exits, structured JSON output, and repeated status checks.

Recommended split:

- Use `mga` for Multigent-native collaboration: tasks, workflow steps, docs, messages, OKRs, and runtime context.
- Use platform CLIs and skills for providers such as GitHub or Lark when the provider already has a mature CLI.
- Use MCP for tools where schemas and interactive tool calling are the natural interface, such as browser automation, design tools, databases, or custom tool gateways.

## Sandbox Initialization

Every sandbox should receive:

1. A Linux `mga` binary synchronized to the current Multigent server version.
2. The selected agent CLI, such as Codex, Claude Code, or Cursor.
3. Runtime environment variables and a scoped agent token.
4. Materialized skills and tool manifests.
5. Explicitly granted credentials and external tools.
6. The task or wakeup prompt for this run.

Agents should not receive the entire workspace filesystem or global user credentials by default.
