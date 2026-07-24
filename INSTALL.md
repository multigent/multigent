# Multigent Installation Guide

This guide is written for two readers:

- **AI agents** helping a user install and configure Multigent.
- **Humans** who prefer to install it manually.

Recommended path: let a local coding agent read this file, install Multigent, start the web console, and guide the user through the first workspace.

## For AI Agents

If you are Claude Code, Codex, Cursor, or another local coding agent installing Multigent for a user:

1. Check the OS, CPU architecture, shell, package managers, Docker status, and whether ports `27892`, `27893`, or `27894` are already occupied.
2. Prefer the official install script on macOS/Linux. Use Homebrew, npm, binary releases, or Docker only when they fit the user's environment better.
3. Install both CLIs when possible:
   - `multigent`: admin CLI and self-hosted web server.
   - `mga`: runtime CLI used inside agent sandboxes.
4. Verify Docker Desktop / Docker Engine is running, then run `multigent sandbox prepare` so the runtime image and common agent CLI toolchains are cached before the first chat.
5. Start Multigent and provide the local URL.
6. Do not create teams, agents, workflows, external tools, or credentials without explaining the plan and asking the user to confirm.
7. Do not ask the user to paste secrets into chat unless there is no safer option. Prefer the web UI credential forms.
8. After setup, verify that the user can open the web console, create or enter a workspace, configure at least one model account, and run one short agent chat.

Suggested user-facing plan before setup:

```text
I will install Multigent, verify Docker, start the web console, and help you create the first workspace.
I will not configure model keys, external tools, or agents until you confirm the next step.
```

## Requirements

- Docker, for sandboxed agent execution.
- A modern browser.
- Optional for source builds: Go 1.26+ and Node.js 20+.

Docker is required when an agent actually runs. The web console can start
without Docker so users can configure a workspace first, but chat, wakeups, and
workflow execution for CLI agents will fail until Docker is installed and
running.

Before the first agent run:

```bash
docker info
```

If this fails:

- Windows/macOS: install and start Docker Desktop, then wait until it reports
  that the engine is running.
- Linux: install Docker Engine and start the daemon, for example
  `sudo systemctl start docker`.

Multigent publishes the default runtime image:

```text
ghcr.io/multigent/multigent/runtime-base:latest
```

This image is public and does not require `docker login`.
The first agent run may pull this image and install the selected agent CLI
toolchain, which can take several minutes. Later runs reuse the local Docker
image cache and the persistent `multigent-toolchains` Docker volume.
The current runtime image is roughly 1 GB after unpacking locally; network
download is smaller because registry layers are compressed. On slow networks,
especially when accessing GHCR or npm from regions with poor connectivity,
run the prewarm step before opening the first agent chat.

Recommended setup prewarm:

```bash
multigent sandbox prepare
```

This pulls the runtime image and warms the common Codex / Claude Code
toolchains. To warm only one toolchain:

```bash
multigent sandbox prepare --toolchain codex
multigent sandbox prepare --toolchain claudecode
```

Mainland China mirror:

```bash
multigent sandbox prepare --region cn
```

This uses the official Alibaba Cloud ACR mirror:

```text
crpi-fu3b7e7lggtmh7za.cn-hangzhou.personal.cr.aliyuncs.com/multigent/runtime-base:latest
```

The mirror is intended to improve first-install reliability for mainland China
users. It currently runs on ACR Personal Edition, which is free within public
limits but has usage limits and no production SLA. If it is unavailable, use
the GHCR image or configure your own mirror with:

```bash
multigent sandbox prepare --image <your-registry>/runtime-base:latest
```

If you only want to pull the image manually:

```bash
docker pull ghcr.io/multigent/multigent/runtime-base:latest
```

## Install Options

### macOS / Linux Install Script

```bash
curl -fsSL https://raw.githubusercontent.com/multigent/multigent/main/scripts/install.sh | bash
multigent version
mga version
```

Optional custom install directory:

```bash
curl -fsSL https://raw.githubusercontent.com/multigent/multigent/main/scripts/install.sh \
  | MULTIGENT_INSTALL_DIR="$HOME/.local/bin" bash
```

### Homebrew

```bash
brew install multigent/tap/multigent
multigent version
mga version
```

### npm Wrapper

```bash
npm install -g @multigent/multigent
multigent version
mga version
```

### Windows PowerShell

```powershell
irm https://raw.githubusercontent.com/multigent/multigent/main/scripts/install.ps1 | iex
multigent version
mga version
```

### Docker Self-Host

```bash
docker run --rm -p 27892:27892 \
  -v multigent-data:/data \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/multigent/multigent:latest
```

Open:

```text
http://127.0.0.1:27892
```

### Prebuilt Binary

Download the latest release:

```text
https://github.com/multigent/multigent/releases
```

Move both `multigent` and `mga` to a directory on `PATH`, then verify:

```bash
multigent version
mga version
```

### From Source

```bash
git clone https://github.com/multigent/multigent
cd multigent
make build
./dist/multigent version
./dist/mga version
```

## Start The Web Console

Use a persistent data directory:

```bash
mkdir -p ./multigent-data
multigent --dir ./multigent-data start --addr 127.0.0.1:27892 --open
```

Open:

```text
http://127.0.0.1:27892
```

The first user creates or enters a workspace from the web UI.

## Update Multigent

Multigent follows the mainstream CLI update model:

- Stable users stay on the `release` channel by default.
- Early testers can opt into `pre-release` or `beta`.
- The web console and CLI check for updates quietly and show a low-noise reminder when a newer version is available.
- Set `MULTIGENT_NO_UPDATE_CHECK=1` to disable update checks.

Check manually:

```bash
multigent check-update
multigent check-update --channel pre-release
multigent check-update --channel beta
```

Update a binary/script installation:

```bash
multigent update
multigent update --channel pre-release
multigent update --channel beta
```

Update by install method:

```bash
# Homebrew
brew update && brew upgrade multigent

# npm wrapper
npm update -g @multigent/multigent

# Docker self-host
docker pull ghcr.io/multigent/multigent:latest
docker pull ghcr.io/multigent/multigent/runtime-base:latest
```

If you want a deployment to follow a non-stable channel by default:

```bash
export MULTIGENT_UPDATE_CHANNEL=pre-release  # release | pre-release | beta
```

## First Setup Checklist

1. Register the first user.
2. Create or enter a workspace.
3. Configure one model account.
4. Open the example workspace or create a project.
5. Add agents to the project.
6. Configure any external tools the agents need.
7. Create or install a workflow.
8. Create a task and bind it to the workflow.
9. Trigger the agent and inspect the run record.

## Agent Setup Guidance

When helping a user build the first workspace, do not overbuild. Start with one concrete workflow:

- one project;
- two or three agents;
- one workflow;
- one task;
- one model account;
- only the external tools needed for the first task.

Before creating roles or agents, ask the user:

1. What kind of work should this workspace demonstrate first?
2. Which agents or human roles should participate?
3. Which tools are required?
4. Where should human review happen?

Then summarize the proposed setup and wait for confirmation.

## Troubleshooting

### Docker Is Missing Or Not Running

```bash
docker info
```

If this fails, install and start Docker before running agents.
On Windows and macOS, this usually means Docker Desktop is not running yet.
On Linux, start the Docker daemon.

Multigent does not block the web console when Docker is missing. It will warn at
startup and block only the agent run that needs a Docker sandbox.

### Port Already In Use

Use another port:

```bash
multigent --dir ./multigent-data start --addr 127.0.0.1:27992 --open
```

### Runtime Image Pull Fails

Try:

```bash
multigent sandbox prepare --skip-clis
```

Or pull the image manually:

```bash
docker pull ghcr.io/multigent/multigent/runtime-base:latest
```

If the network is slow, pull manually or configure a registry mirror. For China
mainland users, Docker Hub mirrors do not always accelerate GHCR. Prefer:

```bash
multigent sandbox prepare --region cn --skip-clis
```

or:

```bash
docker pull crpi-fu3b7e7lggtmh7za.cn-hangzhou.personal.cr.aliyuncs.com/multigent/runtime-base:latest
```

You can also pre-pull the image from a machine with better connectivity and
import it with `docker save` / `docker load`.

### Benchmark First Install

To simulate a first-install runtime preparation path on macOS/Linux:

```bash
scripts/bench-first-install.sh
scripts/bench-first-install.sh --region cn
```

By default this does not remove local Docker caches. To measure a colder path,
explicitly remove the selected runtime image and shared agent CLI toolchain
cache:

```bash
scripts/bench-first-install.sh --region cn --cold --yes
```

This removes only the selected runtime image and the `multigent-toolchains`
Docker volume. It does not run `docker system prune`.

To include a small `docker run` startup check:

```bash
scripts/bench-first-install.sh --region cn --verify-container
```

### CLI Not Found

Check `PATH`:

```bash
which multigent
which mga
```

If installed to a custom directory, add that directory to `PATH`.
