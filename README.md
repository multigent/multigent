# Multigent

Multigent is an Agent Workstream Platform for teams that want to keep using their existing tools while making their agent workforce easier to coordinate, observe, and improve.

It started from `multigent`, but the product direction is different:

- Keep humans, Linear/Jira/ONES, Feishu/Lark, GitHub, and local agent tools in the loop.
- Centralize agent-friendly context, roles, tasks, reviews, run history, and cost signals.
- Use local workers to execute jobs close to repos, credentials, sandboxes, and existing CLI agents.
- Move humans from synchronous drivers to role owners, reviewers, and process designers.

## Current Status

This repository is the first Multigent codebase bootstrap from multigent.

Already changed:

- Go module: `github.com/multigent/multigent`
- CLI binary: `multigent`
- Local metadata directory: `.multigent`
- NPM package name: `@multigent/multigent`
- Initial local worker boundary: `multigent worker inspect`

The current local workspace workflow is still available, but the architecture is being reshaped around:

```text
SaaS Control Plane  <->  Local Worker  <->  Local Agent Runtimes
```

## Build

```bash
make web
make build-go
./dist/multigent --help
```

## Worker

Inspect local worker configuration:

```bash
./dist/multigent worker inspect
```

Preview a worker start config without contacting a control plane:

```bash
./dist/multigent worker start --dry-run \
  --id local-dev \
  --control-plane-url https://app.multigent.ai \
  --token test-token \
  --workspace /tmp/multigent-worker
```

Worker protocol implementation is intentionally not faked yet. See:

- `docs/local-worker-architecture.md`
