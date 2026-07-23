# multigent Runtime Images

Docker images for running AI agents in isolated containers.

Multigent now separates the stable base image from fast-moving agent CLI versions:

```text
runtime-base image + versioned agent CLI toolchain bootstrap
```

The base image contains system dependencies. Codex, Claude Code, Gemini, and other CLIs are installed into `/opt/multigent/toolchains` during sandbox initialization.

## Published images

| Image | Agent | Base |
|-------|-------|------|
| `ghcr.io/multigent/multigent/runtime-base:latest` | Managed CLI toolchains | ubuntu:24.04 + Node 22 + Python |
| `crpi-fu3b7e7lggtmh7za.cn-hangzhou.personal.cr.aliyuncs.com/multigent/runtime-base:latest` | Mainland China mirror | same as GHCR |

The default image for managed CLIs is `ghcr.io/multigent/multigent/runtime-base:latest`, so new users do not need to build a local image first.
Use `multigent sandbox prepare --region cn` to select the Alibaba Cloud mirror.

All images include: `git`, `gh` (GitHub CLI), `curl`, `jq`, `ripgrep`, `make`, `openssh-client`, `python3`, and `sqlite3`.

**Not included by default**: Go, C/C++ build toolchains, Rust, database clients beyond SQLite, browser stacks, and project-specific services. Add them with a custom base image or a runtime template. Keeping these out of the default image makes first install and first sandbox preparation substantially faster.

## Build locally

```bash
# Local development override (international mirrors)
docker build -t multigent/runtime-base:latest \
             -f docker/runtime-base/Dockerfile .

# Faster inside China (Aliyun apt + npmmirror)
docker build --build-arg CN_MIRROR=1 \
             -t multigent/runtime-base:latest \
             -f docker/runtime-base/Dockerfile .

# Include Go and build-essential for heavier local development sandboxes
docker build --build-arg INCLUDE_DEV_TOOLS=1 \
             -t multigent/runtime-base:dev \
             -f docker/runtime-base/Dockerfile .
```

To use the local image instead of GHCR, set the agent's sandbox image to
`multigent/runtime-base:latest`.

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
  image: ghcr.io/multigent/multigent/runtime-base:latest
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
  --add-host=host.docker.internal:host-gateway \
  -v /path/to/agent-dir:/path/to/agent-dir \
  -v /path/to/agent-dir/.multigent/runtime-home/claude/.claude:/root/.claude \
  -v multigent-toolchains:/opt/multigent/toolchains \
  -w /path/to/agent-dir \
  -e IS_SANDBOX=1 \
  -e MULTIGENT_API_URL=http://host.docker.internal:27893 \
  -e MULTIGENT_AGENT_TOKEN=<scoped-runtime-token> \
  ghcr.io/multigent/multigent/runtime-base:latest \
  claude --dangerously-skip-permissions \
         --output-format stream-json \
         --resume <session-id> \
         -p --print-file /workspace/.prompt-xxx.txt
```

The agent working directory is mounted at its real host path so vendor CLIs can persist sessions consistently. Credentials and CLI homes are agent-scoped under `.multigent/runtime-home`; Multigent does not mount host-global `~/.claude`, `~/.codex`, `~/.ssh`, or `~/.config/gh` by default. The container is ephemeral (`--rm`). The toolchain cache persists through the Docker named volume.

## Custom images

Extend any base image to add project-specific tools:

```dockerfile
FROM ghcr.io/multigent/multigent/runtime-base:latest

# Add your project's toolchain
RUN apt-get update && apt-get install -y golang-go build-essential python3-pip \
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
