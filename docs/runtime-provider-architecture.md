# Runtime Provider Architecture

Multigent should not bind the product model to Docker. Docker is one runtime provider, not the user-facing product concept.

The platform model is:

```text
Task -> RunSpec -> RuntimeProvider -> isolated run -> artifacts/events
```

## Product Boundary

Normal users should configure runtime environments, not infrastructure providers.

User-facing concepts:

- runtime template
- required tools and dependencies
- resource needs
- network policy
- connection grants
- MCP/tool grants
- isolation tier

Platform/admin concepts:

- Docker
- E2B
- Modal
- Daytona
- Kubernetes RuntimeClass
- gVisor/Kata/Firecracker

The scheduler should choose a provider from policy and deployment context.

## Runtime Responsibilities

A runtime provider is responsible for:

- base environment: image, template, preinstalled language runtimes, tools
- filesystem: per-run workspace, mounted or materialized inputs, artifact output
- environment variables: non-secret values, inherited values, secret references
- credentials: short-lived scoped tokens, never broad owner credentials
- resources: CPU, memory, timeout, output limits
- network: deny, offline, default, allowlist, provider-specific policy
- process lifecycle: start, stream logs, stop, collect result

These concepts must be provider-neutral. Docker, E2B, microVM, and K8s should all receive the same logical `RunSpec`.

## Current Code Shape

The current implementation introduces:

- `entity.SandboxConfig` as the persisted runtime config.
- provider-neutral fields:
  - `image`
  - `network_mode`
  - `mounts`
  - `env`
  - `resources`
- provider-specific fields:
  - `docker`
  - `e2b`
- `internal/runenv.ProcessSpec`
- `internal/runenv.Provider`
- `internal/runenv.DockerProvider`

The runner now routes configured runtime execution through `runenv.ProviderFor(...)`. Docker still uses the existing sandbox implementation underneath.

## Provider Matrix

| Provider | Status | Purpose |
| --- | --- | --- |
| `host` / none | Existing local mode | Development only. Not suitable for SaaS production. |
| `docker` | Implemented | Local/self-hosted runtime provider, not hosted SaaS default. |
| `e2b` | Config model only | Hosted SaaS candidate. Needs provider implementation and commercial validation. |
| `modal` | Research candidate | Managed sandbox provider for untrusted code and resource controls. |
| `daytona` | Research candidate | Coding-agent sandbox provider with snapshots/forks. |
| `microvm` | Future | Strong isolation for self-hosted/enterprise. |
| `k8s-job` | Future | Enterprise scale/self-hosted scheduler backend. |

## Shared vs Isolated

Shared source, isolated use:

- skills: versioned shared assets, copied or read-only mounted into each run
- context packs: shared documents, materialized into authorized run bundles
- repository source: shared remote, separate checkout/snapshot/worktree per run
- tool gateways: shared API service, scoped by agent token

Always isolated:

- writable run workspace
- temp directory
- process environment
- runtime token
- artifacts
- logs
- resource quota
- network policy

## Mount Policy

Local Docker can still mount host paths for developer ergonomics.

Production should prefer materialization:

```text
source repo/context/skill -> authorized materializer -> /runs/<run_id>/workspace
```

Mounts should be explicit:

```yaml
mounts:
  - source: /cache/skills/sha256...
    target: /workspace/skills/github
    mode: ro
    kind: skill
  - source: /runs/run_123/repo
    target: /workspace/repo
    mode: rw
    kind: repo
```

Do not mount the entire Multigent workspace in production.

## Environment Policy

Runtime env is split into:

- inherited local env: allowed only in developer mode
- explicit non-secret values
- secret references resolved by the control plane
- scoped run token issued by Multigent

Production run env should include:

```text
MULTIGENT_WORKSPACE_ID
MULTIGENT_AGENT_ID
MULTIGENT_RUN_ID
MULTIGENT_API_URL
MULTIGENT_AGENT_TOKEN
```

Agent code should call Multigent API with this scoped token instead of reading DB credentials or using owner-level CLI authority.

## E2B Integration Plan

E2B should implement the same `runenv.Provider` contract.

Expected mapping:

- `image/template` -> E2B template
- `mounts` -> files uploaded or synced into sandbox
- `env` -> sandbox env vars
- `resources.timeout_sec` -> sandbox/run timeout
- command -> `sandbox.commands.run(...)`
- logs -> streamed command output
- artifacts -> files downloaded from sandbox after run

Open questions before implementation:

- long-running agent sessions: keep sandbox alive vs one sandbox per task
- file sync cost for large repos
- network allowlist support
- secret injection model
- artifact size limits
- pricing per run and idle timeout

## Near-Term Implementation Order

1. Keep Docker provider working through `runenv.Provider`.
2. Move Docker-specific UI labels to runtime labels.
3. Add run materializer that creates `/runs/<run_id>/workspace`.
4. Stop mounting whole workspace root in production profile.
5. Issue scoped run tokens for agent API calls.
6. Implement E2B provider behind the same interface.
7. Add runtime policy enforcement and audit logs.

The important part is not whether Docker, E2B, Modal, or Daytona wins first. The important part is that Multigent schedules against `RunSpec`, not against Docker flags, and that the product exposes environment/policy abstractions instead of raw infrastructure choices.
