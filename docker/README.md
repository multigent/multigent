# multigent Runtime Images

Docker images for running AI agents in isolated containers.

Multigent now separates the stable base image from fast-moving agent CLI versions:

```text
runtime-base image + versioned agent CLI toolchain bootstrap
```

The base image contains system dependencies. Codex, Claude Code, Gemini, and other CLIs are installed into `/opt/multigent/toolchains` during sandbox initialization.

## Available images

| Image | Agent | Base |
|-------|-------|------|
| `multigent/runtime-base:latest` | Managed CLI toolchains | ubuntu:24.04 + Node 22 + Go + Python |
| `ghcr.io/multigent/sandbox-claudecode:latest` | Legacy Claude Code image | ubuntu:24.04 + Node 22 + Go + Claude Code |
| `ghcr.io/multigent/sandbox-codex:latest` | Legacy Codex image | ubuntu:24.04 + Node 22 + Go + Codex + pnpm |
| `ghcr.io/multigent/sandbox-gemini:latest` | Legacy Gemini image | ubuntu:24.04 + Node 22 + Gemini |
| `ghcr.io/multigent/sandbox-generic:latest` | Any / custom | ubuntu:24.04 + Node 22 |

The default image for managed CLIs is `runtime-base`.

All images include: `git`, `gh` (GitHub CLI), `curl`, `jq`, `ripgrep`, `make`, `openssh-client`, `python3`, and `sqlite3`.

**Not included by default**: Rust, database clients beyond SQLite, browser stacks, and project-specific services. Add them with a custom base image or a runtime template.

## Build locally

```bash
# Default base image (international mirrors)
docker build -t multigent/runtime-base:latest \
             -f docker/runtime-base/Dockerfile .

# Faster inside China (Aliyun apt + npmmirror)
docker build --build-arg CN_MIRROR=1 \
             -t multigent/runtime-base:latest \
             -f docker/runtime-base/Dockerfile .
```

Same `--build-arg CN_MIRROR=1` works for all sandbox images.

## Agent CLI toolchain bootstrap

When a Docker runtime uses `runtime-base`, Multigent wraps the command with a small bootstrap:

1. Attach persistent toolchain cache:
   `multigent-toolchains:/opt/multigent/toolchains`
2. Resolve `agent_cli` from the runtime template or agent model.
3. Install the selected CLI if the version marker is missing.
4. Add `/opt/multigent/toolchains/npm/bin` to `PATH`.
5. Execute the agent command.

Example runtime config:

```yaml
sandbox:
  provider: docker
  image: multigent/runtime-base:latest
  agent_cli:
    vendor: codex
    version: 0.18.0
    package_manager: npm
    package: "@openai/codex"
    binary: codex
```

This means Codex/Claude/Gemini upgrades do not require rebuilding `runtime-base`.

## Root + permission bypass

Runtime images currently run as **root** so credential mounts at `/root/.claude`, `/root/.ssh`, etc. work without path remapping.

Each agent CLI requires specific flags/env vars to run non-interactively as root:

| Agent | Mechanism |
|-------|-----------|
| Claude Code | `IS_SANDBOX=1` + `--dangerously-skip-permissions` added by runner |
| Codex CLI | `CODEX_UNSAFE_ALLOW_NO_SANDBOX=1` + Codex sandbox bypass flag added by runner |
| Gemini CLI | No extra flags needed; use `GEMINI_API_KEY` or mount `~/.gemini/` |

These are set automatically — you don't need to configure anything.

## How multigent uses these images

When an agent is hired with `--sandbox docker`, every `multigent run` invocation becomes:

```bash
docker run --rm -i \
  --memory=4096m \
  --network=bridge \
  -v /path/to/agent-dir:/workspace \          ← agent working dir
  -v /path/to/workspace-root:/path/to/workspace-root \ ← full workspace (inter-agent tasks)
  -v /path/to/repo:/path/to/repo \            ← project repo (same path as host)
  -v ~/.claude:/root/.claude:ro \             ← credentials, read-only
  -v ~/.config/gh:/root/.config/gh:ro \
  -v ~/.ssh:/root/.ssh:ro \
  -v /usr/local/bin/multigent:/usr/local/bin/multigent:ro \  ← multigent binary
  -w /workspace \
  -e IS_SANDBOX=1 \                           ← claudecode: allow root run
  -e ANTHROPIC_API_KEY \                      ← inherited from host (value hidden)
  multigent/runtime-base:latest \
  claude --dangerously-skip-permissions \
         --output-format stream-json \
         --resume <session-id> \
         -p --print-file /workspace/.prompt-xxx.txt
```

The agent working directory (`CLAUDE.md`, tasks, skills, etc.) is bind-mounted at the real host path. The container is ephemeral (`--rm`). The toolchain cache persists through the Docker named volume.

## Custom images

Extend any base image to add project-specific tools:

```dockerfile
FROM multigent/runtime-base:latest

# Add your project's toolchain
RUN apt-get update && apt-get install -y golang-go python3-pip \
    && rm -rf /var/lib/apt/lists/*
RUN npm install -g yarn
```

Then use it:

```bash
multigent hire --project my-api --team engineering --model claudecode \
               --name dev --sandbox docker \
               --sandbox-image "myorg/my-claude-sandbox:v1"
```

## Credential setup

Credentials are mounted read-only from your host:

| Agent | Required path | How to set up |
|-------|--------------|---------------|
| Claude Code | `~/.claude.json` + `~/.claude/` | Run `claude` interactively once on host |
| Codex | `~/.codex/auth.json` | Run `codex login` on host, or set `OPENAI_API_KEY` env var |
| Gemini | `~/.gemini/` (`oauth_creds.json`) | Run `gemini` on host, or set `GEMINI_API_KEY` env var |
| GitHub CLI | `~/.config/gh/` | Run `gh auth login` on host |
| SSH (git push) | `~/.ssh/` | Standard SSH key setup |

API keys are forwarded from your host environment using `-e KEY` (value never appears in command line).

## Security notes

- Containers run as **root** — required for credential mounts at `/root/.*`
- Credential mounts are read-only (`:ro`) — containers cannot modify host credentials
- Network defaults to `bridge` — use `--sandbox-network none` for fully offline operation
- Each run is ephemeral: the container is destroyed after the agent exits (`--rm`)
- The multigent binary is mounted read-only — agents can use it but not modify it
