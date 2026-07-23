# Runtime Sandbox Research and Product Model

This document records the current sandbox research and the product model Multigent should use.

## Conclusion

Do not ask normal users to choose Docker, E2B, Kubernetes Pod, Kata, or gVisor directly.

Users should configure:

- Runtime template: what CLI/runtime/tools/dependencies are available.
- Isolation tier: local trusted, isolated cloud, high-security cloud, private enterprise.
- Resource requirements: CPU, memory, timeout, disk, network.
- Allowed connections: provider credentials, MCP servers, integrations, repos, context packs.

Multigent chooses the concrete provider based on deployment and policy:

| Deployment | Default provider direction |
| --- | --- |
| local developer | Docker or host dev mode |
| self-hosted team | Docker first, then Kubernetes with gVisor/Kata |
| hosted SaaS | managed sandbox such as E2B / Modal / Daytona |
| high-security enterprise | microVM / Kata / Firecracker-backed runtime |

The product surface should say "Runtime Environment", not "Docker".

## Research Notes

### E2B

E2B is close to the agent sandbox use case. Its templates can define base image, env vars, files, commands, and build/start commands captured into a snapshot. It supports creating sandboxes from templates, command execution, filesystem access, env vars, and long-lived sandbox timeouts. This maps well to Multigent's need for prebuilt agent CLI environments and per-run execution.

Important fit:

- templates for preinstalling Codex/Claude Code/toolchains
- env injection
- filesystem operations
- command execution
- managed cloud isolation

Open questions:

- large repo sync cost
- exact network allowlist model
- credential injection and revocation
- long-running CLI session behavior
- pricing under continuous loop workloads

### Modal Sandboxes

Modal explicitly positions Sandboxes for executing untrusted user or agent code. It supports custom container images, arbitrary dependencies/setup scripts, running commands, filesystem access, resource configuration, networking/security controls, snapshots, and VM sandboxes. Modal is a credible provider candidate, especially if we want strong resource controls and high-scale managed execution.

Important fit:

- arbitrary code execution
- custom images/dependencies
- file upload/download
- CPU/memory resource model
- secure-by-default networking posture
- VM sandbox option

Open questions:

- agent CLI interactive/session behavior
- long-lived process ergonomics
- cost for many small loop runs
- integration complexity from Go service

### Daytona

Daytona is strongly aligned with "coding agent sandbox" workflows. It has SDKs, sandbox snapshots/forks, process execution, volumes, memory configuration, organization-scoped quotas/API keys, and network allow/block controls. Daytona also documents Claude/OpenAI agent sandbox examples.

Important fit:

- coding-agent examples
- snapshots/forks for reusable environments
- network allow/block
- volumes
- organization-level resource controls

Open questions:

- SaaS embedding/commercial fit
- Go SDK maturity and version requirements
- fine-grained credential handling
- cost and lifecycle behavior for many agents

### Docker

Docker is good for local and self-hosted developer ergonomics, but it is not the right product abstraction for a hosted multi-tenant agent platform.

Docker's own security documentation notes that default capabilities and mounts can provide incomplete isolation, especially combined with kernel vulnerabilities. This is acceptable for trusted local work, not enough as our SaaS isolation promise.

Use Docker for:

- local development
- self-hosted private deployment
- testing runtime templates
- enterprise environments where the customer accepts and controls the host

Avoid Docker as:

- hosted multi-tenant security boundary
- user-facing product concept
- direct mount of the whole Multigent workspace

### Kubernetes Pods

Plain Kubernetes Pods are an orchestration unit, not an agent sandbox by themselves. Kubernetes Pod Security Standards provide profiles from privileged to restricted, but standard pods still share node/kernel realities. If Kubernetes is used, it should be combined with sandboxed runtimes such as gVisor or Kata Containers through RuntimeClass.

Use Kubernetes for:

- scheduling
- quotas
- network policy
- enterprise self-hosted scale

Do not treat a vanilla pod as sufficient sandboxing for untrusted agents.

### gVisor / Kata / Firecracker

These are stronger isolation directions:

- gVisor can run sandboxed containers in Kubernetes through RuntimeClass.
- Kata uses lightweight VMs while preserving container ecosystem compatibility.
- Firecracker is purpose-built for secure multi-tenant container/function workloads using lightweight microVMs.

These are good infrastructure primitives, but building a complete product runtime directly on them is more work than using E2B/Modal/Daytona for SaaS MVP.

## Product Abstractions

### RuntimeTemplate

RuntimeTemplate describes the environment an agent runs in.

It should include:

```yaml
id: codex-go-node
base: ubuntu-24.04
agent_cli:
  vendor: codex
  version: 0.18.0
  install: managed
system_packages:
  - git
  - ripgrep
  - python3
  - nodejs
  - golang
language_packages:
  pip:
    - pytest
  npm:
    - pnpm
setup:
  - go version
  - node --version
ready:
  - codex --version
```

Provider mapping:

- Docker: build image or run setup against an image.
- E2B: build template/snapshot.
- Modal: build image/sandbox image.
- Daytona: create snapshot.
- K8s/Kata: use OCI image plus init step.

### Agent CLI Runtime

Agent CLI should be its own abstraction, separate from sandbox provider.

Fields:

- vendor: `codex`, `claude-code`, `cursor`, `gemini`, `custom`
- version
- install method
- auth adapter
- invocation adapter
- session adapter
- output parser

This lets us run the same Codex CLI inside Docker, E2B, Modal, or another provider.

### Connection

Connections represent credentials and external account access.

Examples:

- OpenAI API key
- Anthropic / Claude Code credential
- Codex credential
- GitHub token
- Feishu MCP OAuth credential
- Linear/Jira token
- customer database read-only credential

Connections must have:

- owner type: workspace, project, human, agent
- grant scope: which agents/tasks may use it
- secret reference
- provider type
- expiry/refresh behavior
- audit log

Important rule:

> An agent should receive a scoped connection grant, not raw owner credentials by default.

If a human owner grants their Feishu MCP credential to their agent, that grant should be explicit, revocable, audited, and bounded by agent/task scope.

### MCPTool

MCP tool configuration should be separated from credentials.

MCP server definition:

- name
- transport
- command/url
- required env/secret names
- capability tags

MCP grant:

- agent_id
- connection_id
- allowed tools
- allowed resource scopes
- approval policy

This supports both company-level MCP credentials and personal human-owned credentials.

### Skill

Skills are shared source assets, but each run should get an isolated version.

Model:

- skill source is versioned once
- run materializer copies or read-only mounts exact skill version
- writes go to run workspace/artifacts
- skill can declare required tools/connections

### RunMaterialization

Before execution, Multigent should create a run bundle:

```text
run/
  context/
  skills/
  repo/
  task.json
  tool-manifest.json
  env-manifest.json
  artifacts/
```

This bundle is built through authorization-aware retrieval.

Agent does not see:

- entire workspace filesystem
- raw database
- unrelated context packs
- other agent workdirs
- unrestricted credentials

### Scoped Agent API

Humans and agents should use the same API surface, but different principal types and tokens.

Agent run token allows only:

- read current task
- update current task
- send authorized messages
- create/delegate tasks if granted
- read authorized context
- use granted tools/connections
- write artifacts
- propose memory

It does not allow:

- list all DB data
- read unrelated projects
- manage workspace users
- edit another agent's prompt
- configure integrations
- access billing/admin APIs

## Recommended Architecture

```text
Agent definition
  -> RuntimeTemplate
  -> AgentCLI adapter
  -> Connection grants
  -> MCP grants
  -> Skill grants
  -> Context grants
  -> RuntimePolicy
  -> RunSpec
  -> RuntimeProvider
```

Provider-specific implementations are hidden behind `RuntimeProvider`.

Normal users configure templates and grants. Admins can set provider policy. Hosted SaaS should choose the provider automatically.

## Implementation Plan

1. Rename user-facing "sandbox" to "runtime environment".
2. Keep Docker as local provider but stop exposing it as the main product idea.
3. Add `RuntimeTemplate` and `Connection` tables/interfaces.
4. Add scoped agent run tokens.
5. Add materialized per-run workspace.
6. Move CLI provider auth into `AgentCLIAdapter` / `ConnectionAdapter`.
7. Add MCP server and MCP grant model.
8. Implement one managed cloud sandbox provider behind `RuntimeProvider`.
9. Disable direct host execution in production profile.

## Decision

For SaaS product direction:

- Do not make "choose Docker vs E2B" a normal user choice.
- Use managed sandbox providers first for hosted SaaS.
- Keep Docker only as local/self-hosted provider.
- Treat Kubernetes as an orchestration backend, only acceptable with gVisor/Kata/microVM RuntimeClass for untrusted agent execution.
- Invest in Multigent's own abstractions: RuntimeTemplate, AgentCLI, Connection, MCPGrant, SkillGrant, RunMaterialization, Scoped Agent API.
