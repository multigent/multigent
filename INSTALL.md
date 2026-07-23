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
4. Start Multigent and provide the local URL.
5. Do not create teams, agents, workflows, external tools, or credentials without explaining the plan and asking the user to confirm.
6. Do not ask the user to paste secrets into chat unless there is no safer option. Prefer the web UI credential forms.
7. After setup, verify that the user can open the web console, create or enter a workspace, and configure at least one model account.

Suggested user-facing plan before setup:

```text
I will install Multigent, verify Docker, start the web console, and help you create the first workspace.
I will not configure model keys, external tools, or agents until you confirm the next step.
```

## Requirements

- Docker, for sandboxed agent execution.
- A modern browser.
- Optional for source builds: Go 1.26+ and Node.js 20+.

Multigent publishes the default runtime image:

```text
ghcr.io/multigent/multigent/runtime-base:latest
```

This image is public and does not require `docker login`.

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

### Port Already In Use

Use another port:

```bash
multigent --dir ./multigent-data start --addr 127.0.0.1:27992 --open
```

### Runtime Image Pull Fails

Try:

```bash
docker pull ghcr.io/multigent/multigent/runtime-base:latest
```

If the network is slow, pull manually or configure a registry mirror.

### CLI Not Found

Check `PATH`:

```bash
which multigent
which mga
```

If installed to a custom directory, add that directory to `PATH`.
