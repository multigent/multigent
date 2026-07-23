# Runtime Toolchain Architecture

Multigent should not rebuild and redistribute a new sandbox image every time Codex, Claude Code, Gemini, or another agent CLI releases a new version.

The runtime model should split the environment into two layers:

```text
stable base image + versioned agent CLI toolchain + per-run grants/context
```

## Decision

Use a stable base runtime image, then install the selected agent CLI during sandbox initialization into a persistent toolchain cache.

Default shape:

```yaml
runtime:
  image: ghcr.io/multigent/multigent/runtime-base:latest
  agent_cli:
    vendor: codex
    version: 0.18.0
    package_manager: npm
    package: "@openai/codex"
    binary: codex
```

This lets us update or pin CLI versions without rebuilding the base image.

## Base Image Responsibilities

The base image should contain stable system dependencies:

- shell, coreutils, git, openssh-client, ca-certificates
- curl/wget, unzip, jq, ripgrep
- Node.js/npm for npm-distributed CLIs
- Python runtime and common build essentials
- Go toolchain when required by common engineering templates
- Multigent runtime helper dependencies

It should not bake in fast-moving agent CLI versions as the main distribution mechanism.

## Agent CLI Responsibilities

Agent CLI installation is a provider-neutral toolchain concern:

- vendor: `codex`, `claude-code`, `gemini`, `cursor`, `opencode`, `qoder`, `custom`
- version: exact semver, tag, digest, or `latest`
- installer: npm, script, download, package registry, internal artifact
- binary: executable expected by the runner
- check: optional verification commands

The installer catalog can start in code and later move to the DB/control plane.

## Runtime Initialization Flow

For every sandbox run:

1. Start from the selected base image.
2. Mount or attach persistent toolchain cache, e.g. `/opt/multigent/toolchains`.
3. Resolve the agent CLI install spec from runtime template plus agent override.
4. If the version marker is missing or force-upgrade is requested, install the CLI.
5. Add the CLI bin path to `PATH`.
6. Inject scoped Multigent API token and allowed provider/MCP credentials.
7. Materialize authorized context, skills, repo files, and artifacts directories.
8. Execute the agent CLI command.

The first run pays installation cost. Later runs reuse the cached toolchain unless the version changes.

## Docker / gVisor Mapping

Docker/gVisor provider should map the provider-neutral toolchain model like this:

```text
image                -> docker image
agent_cli installer  -> /bin/sh bootstrap before agent command
toolchain cache      -> docker named volume or host-managed cache mount
runtime isolation    -> docker run --runtime=runsc when available
```

Current implementation direction:

- default managed image: `ghcr.io/multigent/multigent/runtime-base:latest`
- persistent Docker volume: `multigent-toolchains:/opt/multigent/toolchains`
- npm CLI bin path: `/opt/multigent/toolchains/npm/bin`
- command wrapper: install/check CLI, then `exec "$@"`

## Kubernetes Mapping

For K8s, the same model maps to:

```text
image                -> Pod image
agent_cli installer  -> initContainer or entrypoint bootstrap
toolchain cache      -> PVC, image layer cache, or prewarmed node cache
runtime isolation    -> RuntimeClass(gvisor)
```

The product model remains unchanged. Only the provider implementation changes.

## Why This Is Better Than Per-CLI Images

Per-CLI images have several problems:

- every CLI upgrade requires image rebuild and redistribution
- customers need to pull many large images
- rollback/pin behavior is tied to image tags
- runtime template changes become infrastructure changes
- custom/private CLI builds are awkward

The split model gives us:

- one stable base image
- explicit CLI version pinning
- faster CLI upgrades
- cleaner customer installation
- future support for enterprise-approved internal CLI mirrors
- better auditability: agent run records can say exactly which CLI vendor/version was used

## Security Rules

CLI installers must be controlled by runtime templates or admin-approved catalogs.

Rules:

- normal users should not paste arbitrary install scripts into production templates without review
- installer scripts run before untrusted agent work, but still inside the sandbox
- installer network access should be allowlisted by template policy
- credentials are injected after identity and grants are resolved
- CLI auth credentials are separate from workspace/project/database credentials
- agents receive scoped API tokens, not DB access

## Open Product Questions

- Should `latest` be allowed in production, or only exact versions?
- Should each workspace have an approved CLI catalog?
- Should CLI installation happen on first run, on template publish, or through a prewarm job?
- Should toolchain cache be global per deployment, per workspace, or per isolation tier?
- How do we expose CLI upgrade rollout: all agents, one team, one project, one agent?

## Near-Term Implementation Plan

1. Add `agent_cli` to runtime config.
2. Add default installer specs for Codex, Claude Code, and Gemini.
3. Publish the default Docker runtime image to GHCR and use it by default.
4. Mount persistent toolchain cache into Docker runs.
5. Bootstrap CLI install/check before command execution.
6. Add Web controls for CLI vendor/version/channel.
7. Add run telemetry fields for runtime image and CLI vendor/version.
8. Add admin-managed installer catalog in DB.
9. Add prewarm command/job for runtime templates.
