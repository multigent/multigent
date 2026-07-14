---
name: multigent
description: "Manage AI agent teams with multigent — a CLI tool for organising AI agents (Claude Code, Codex, Gemini, Cursor, etc.) into hierarchical teams (Agency → Team → Role → Project → Agent). Key capabilities: create agencies and teams, hire agents with merged context layers, assign and run tasks with priority queues, manage autonomous playbooks (wakeup.md), send async inbox messages between human and agents, configure heartbeat schedules and cron jobs, run agents inside Docker sandboxes, forward/confirm tasks via inbox, manage templates, and more. Use this skill whenever you need to: create or manage an multigent workspace, hire/fire/sync agents, add/run/cancel tasks, check inbox confirmations, send messages, configure heartbeats or crons, start the scheduler, or work with agency templates."
---

# multigent

`multigent` is a CLI tool for organising AI agents into hierarchical teams (Agency → Team → Role → Project → Agent). Each agent = Model + Context + Skills. The binary is at `/usr/local/bin/multigent` or in `$PATH`.

## Installation

```bash
# via npm (recommended)
npm install -g @multigent/agentctl

# via install script
curl -fsSL https://raw.githubusercontent.com/multigent/multigent/main/scripts/install.sh | sh

# verify
multigent version
```

## Core concepts

| Concept | What it is |
|---------|-----------|
| **Agency** | The root workspace (`agency-prompt.md`, `.multigent/agency.yaml`) |
| **Team** | A group with a shared prompt (`teams/<team>/prompt.md`) |
| **Role** | A position within a team — has its own prompt + bound skills + workspace setup |
| **Project** | A work unit with its own prompt, linked to a code repo |
| **Agent** | A hired instance: model + merged context from all layers |
| **Skill** | Reusable instructions copied into agent workspace on hire |
| **Task** | A unit of work with status, priority (0=critical…3=low), prompt, and optional heartbeat/cron |
| **Playbook** | `wakeup.md` — the autonomous routine an agent runs when its task queue is empty |
| **Message** | Async non-blocking communication between any two participants (human or agent) |

Context is merged in this order (later layers override): **Agency → Team → Role → Project → Agent**

---

## Quick start — blank agency

```bash
# 1. Create agency
multigent create agency --name "MyAgency" --desc "My first agency"
cd MyAgency

# 2. Create team + role
multigent create team --name "engineering" --desc "Software engineers"
multigent create role --team "engineering" --name "developer"

# 3. Create project
multigent create project --name "my-service" --repo "/path/to/repo"

# 4. Hire an agent
multigent hire --project my-service --team engineering --role developer \
  --model claudecode --name dev --sandbox docker

# 5. Add and run a task
multigent task add --project my-service --agent dev \
  --title "Implement feature X" --prompt "..." --created-by human
multigent run --project my-service --agent dev
```

## Quick start — from a template (recommended)

```bash
# 1. Create agency from template
multigent create agency --name "MyAgency" \
  --template https://example.com/tech-agency.tar.gz
cd MyAgency

# 2. List available project blueprints
multigent project blueprints

# 3. Create a project from a blueprint
multigent create project --name "my-service" --blueprint default

# 4. Review and apply (hires agents, configures heartbeats, installs wakeup.md playbooks)
multigent project show  --project my-service
multigent project apply --project my-service

# 5. Start the scheduler — agents wake up on schedule and run their playbooks when idle
multigent scheduler start

# 6. Monitor
multigent inbox list          # task confirmations awaiting your decision
multigent inbox messages      # async messages from agents
multigent task list --project my-service --agent pm
```

---

## Global flag

All commands support `--dir <workspace>` to point to the agency root when not running from inside it:

```bash
multigent --dir /root/code/MyAgency task list --project my-service --agent dev
```

---

## Command reference

### Workspace setup

```bash
multigent create agency  --name "Name" [--desc "..."] [--template file.tar.gz|dir|URL]
multigent create team    --name "engineering" [--desc "..."]
multigent create role    --team "engineering" --name "developer" [--desc "..."]
multigent create project --name "my-service"  --repo "/path/to/repo" [--desc "..."]
multigent create project --name "my-service"  --blueprint default   # from blueprint
```

### Hiring agents

```bash
multigent hire --project <proj> --team <team> [--role <role>] \
               --model <model> --name <name> [--sandbox docker]
# Supported models: claudecode  codex  gemini  cursor  generic-cli

multigent sync --project <proj> --agent <name>   # re-sync after editing prompts/skills
multigent sync --project <proj>                  # sync all agents in project
multigent fire --project <proj> --agent <name>          # soft delete → .fired/
multigent fire --project <proj> --agent <name> --force  # hard delete
```

### Project lifecycle

```bash
multigent project blueprints
multigent project show  --project P
multigent project apply --project P             # hire agents + configure heartbeats + install playbooks
multigent project apply --project P --dry-run
multigent project apply --project P --force
```

**project-blueprints/default.yaml** example:
```yaml
name: "{{PROJECT_NAME}}"
description: "..."
agents:
  - name: dev
    role: developer
    team: engineering
    model: claudecode
    sandbox: true
    playbook: dev.md        # installed as wakeup.md by project apply
    heartbeat:
      enabled: true
      interval: 30m
      active_hours: "09:00-20:00"
      active_days: weekdays

  - name: pm
    role: product-manager
    team: product
    model: claudecode
    playbook: pm.md
    heartbeat:
      enabled: true
      interval: 30m
```

### Tasks

```bash
multigent task add    --project P --agent A --title "T" --prompt "..." \
                      --created-by human|<project>/<agent> \
                      [--type feature|bug|chore] [--priority 0-3]
multigent task list   --project P --agent A [--status pending] [--archived]
multigent task show   <task-id>
multigent task set    <task-id> [--title T] [--status S] [--priority N] [--due-date D] \
                      [--estimate-duration 30m] [--parent ID] [--label L]...
multigent task stats  [--since today] [--project P] [--agent A] [--assignee X] [--label L] \
                      [--by agent|assignee|label|label:value|label:category] [--detail] [--format json]
multigent task cancel <task-id>
multigent task retry  <task-id>

# Emergency halt — cancel pending (and optionally running) tasks
multigent task stop-all --project P --all-agents
multigent task stop-all --project P --agent A --include-running

# View token usage and cost
multigent task tokens --project P --agent A
multigent task tokens --project P --all-agents
multigent task tokens --project P --agent A --task <task-id>

# Called by agent inside its prompt:
multigent task done --id <task-id> --status success --summary "brief description"
multigent task done --id <task-id> --status failed  --error "reason"

# Pause and wait for human confirmation (blocks until human responds):
multigent task confirm-request --id <task-id> \
  --summary "PR #42 ready for review" \
  --action-item "Open the PR and check the diff" \
  --action-item "Reply: merge / hold <reason>"
```

Task priority: 0=critical, 1=high, 2=normal (default), 3=low. The scheduler always picks the highest-priority pending task first. After `confirm-request`, the human's reply is available as `$CONFIRMATION_REPLY`.

### Running agents

```bash
multigent run  --project P --agent A              # pick next pending task
multigent run  --project P --agent A --task <id>  # run specific task
multigent exec --project P --agent A --prompt "..." # one-off prompt (no task queue)
```

### Inbox — task confirmations (blocking)

Agents call `task confirm-request` to pause a task and wait for the human's decision.

```bash
multigent inbox list
multigent inbox show    <task-id>
multigent inbox confirm <task-id> --message "approved, go ahead"
multigent inbox reject  <task-id> --reason "needs rework"
multigent inbox comment <task-id> --message "check the auth module first"
multigent inbox forward <task-id> --to <project>/<agent> --note "please re-check"
```

### Inbox — async messages (non-blocking)

Any participant (human or agent) can send messages to any other. Recipients read them on their next wakeup — the scheduler auto-injects unread messages at the top of the wakeup prompt.

**Address format:** `human` or `project/agent` (e.g. `cc-connect/pm`, `cc-connect/dev-claude`)

```bash
# Send (single recipient)
multigent inbox send --to cc-connect/pm --subject "Prioritise #55" --body "..."
multigent inbox send --from cc-connect/pm --to human --subject "Update" --body "..."

# Group send (repeat --to)
multigent inbox send \
  --to cc-connect/pm --to cc-connect/dev-claude --to human \
  --subject "All-hands" --body "..."

# Read (human's mailbox by default)
multigent inbox messages                                              # unread only
multigent inbox messages --recipient cc-connect/pm                   # agent's mailbox
multigent inbox messages --from cc-connect/pm                        # filter by sender
multigent inbox messages --all                                        # include already-read
multigent inbox messages --archived                                   # show archived messages
multigent inbox messages --mark-read                                  # mark all as read after listing

# Reply
multigent inbox reply <msg-id> --from cc-connect/pm --body "Acknowledged."

# Forward a message to one or more recipients
multigent inbox fwd <msg-id> --to cc-connect/dev-claude
multigent inbox fwd <msg-id> --to cc-connect/pm --to human --note "FYI"

# Per-message status management
multigent inbox read    <msg-id>                    # mark single message as read
multigent inbox archive <msg-id>                    # archive (hidden from normal listing)
multigent inbox delete  <msg-id>                    # permanently delete
multigent inbox rm      <msg-id>                    # alias for delete
# --recipient flag available on all above to specify mailbox (default: human)
```

### Knowledge base (docs)

A bookmark index for documents. Files stay where they are; only metadata is tracked in `.multigent/docs.yaml`. Virtual directories are created automatically.

```bash
# Add a document
multigent docs add --path ./reports/design.md --title "System Design" \
  --index "cc-connect/architecture" --created-by human --tag design

# List / search
multigent docs list [--index prefix] [--tag tag] [--created-by human] [--json]
multigent docs search "design"
multigent docs tree                     # virtual directory tree

# View details
multigent docs show <doc-id> [--content]

# Update metadata / move
multigent docs update <doc-id> --title "New Title" --tag newtag
multigent docs move   <doc-id> --index "new/category"

# Remove from index (file is NOT deleted)
multigent docs remove <doc-id>
```

The Web UI provides a visual knowledge base viewer at the "Knowledge Base" page with directory tree navigation and Markdown rendering.

### Daemon (heartbeat + wakeup routines)

```bash
# Configure heartbeat
multigent scheduler heartbeat --project P --agent A \
  --enable --interval 30m \
  --active-hours "09:00-18:00" \
  --active-days  "weekdays"

# Set wakeup routine (runs as synthetic task when queue is empty)
multigent scheduler heartbeat --project P --agent A \
  --wakeup-prompt-file /path/to/wakeup.md

# Start scheduler (aliases: sched, s)
multigent scheduler start
multigent scheduler stop
multigent scheduler status

# Cron jobs
multigent cron add     --project P --agent A \
  --title "Daily standup" --schedule "0 9 * * 1-5" --prompt "..."
multigent cron list    --project P --agent A
multigent cron delete  <cron-id>  --project P --agent A
multigent cron enable  <cron-id>  --project P --agent A
multigent cron disable <cron-id>  --project P --agent A
```

Each heartbeat cycle: if pending tasks exist → run highest-priority task; if queue is empty and `wakeup.md` is set → run wakeup routine. Unread messages are always prepended automatically.

### Agent playbooks

A playbook (`wakeup.md`) defines what an agent does when its task queue is empty. Store in `agent-playbooks/` and reference from `project.yaml` via `playbook:`.

`project apply` copies `agent-playbooks/<playbook>` → `agents/<name>/.multigent/context/wakeup.md` and sets `wakeup_prompt: "@.multigent/context/wakeup.md"` in `heartbeat.yaml`.

Typical wakeup.md patterns:
- Check injected unread messages (auto-prepended — no `inbox messages` call needed)
- Reply with: `multigent inbox reply <msg-id> --from project/agent --body "..."`
- Send async update: `multigent inbox send --from project/agent --to human --subject "..." --body "..."`
- Pause for human decision: `multigent task confirm-request --id $TASK_ID --summary "..." --action-item "..."`
- Complete: `multigent task done --id $TASK_ID --status success --summary "..."`

### Skills

```bash
multigent role skill add    --team <t> --role <r> --skill <s>
multigent role skill remove --team <t> --role <r> --skill <s>
multigent role list         --team <t>
multigent list skills
```

### Templates

A template bundles: `agency-prompt.md`, `teams/`, `skills/`, `agent-playbooks/`, `project-blueprints/`.

```bash
multigent template pack --output my-agency.tar.gz \
  --name "tech-project" --version "1.0.0" \
  --author "Alice" --email "alice@example.com" \
  --description "Standard software engineering agency" \
  --keywords "engineering,software"

multigent template info my-agency.tar.gz
multigent template info my-agency.tar.gz --json

multigent create agency --name "My Agency" --template my-agency.tar.gz
multigent create agency --name "My Agency" --template https://example.com/tpl.tar.gz
```

### Sessions & misc

```bash
multigent session show  --project P --agent A
multigent session clear --project P --agent A
multigent list teams | projects | agents | skills
multigent show team engineering
multigent show project my-api
multigent show agent my-api dev [--raw]
multigent version
```

---

## Agent context file locations

| Model | Context file | Skills dir |
|-------|-------------|------------|
| claudecode | `CLAUDE.md` | `.claude/skills/` |
| codex | `AGENTS.md` | (inlined) |
| gemini | `GEMINI.md` | `.gemini/skills/` |
| cursor | `.cursorrules` | `.cursor/rules/` |
| generic-cli | `context.md` | — |

---

## Agency directory structure

```
<AgencyName>/
  .multigent/
    agency.yaml            ← workspace marker
    inbox.yaml             ← human task-confirmation inbox
    inbox.md               ← human-readable inbox summary
    messages.yaml          ← async messages for the human
  agency-prompt.md
  teams/
    <team>/
      team.yaml
      prompt.md
      roles/<role>/
        role.yaml
        prompt.md
  skills/
    <skill>/
      skill.yaml
      prompt.md
      [other files, e.g. scripts]
  agent-playbooks/         ← wakeup.md templates, distributed with agency template
    pm.md
    qa-reviewer.md
  project-blueprints/
    default.yaml
  projects/
    <project>/
      project.yaml         ← agents + heartbeats + crons + playbooks (declarative)
      prompt.md
      agents/
        <agent>/
          CLAUDE.md           ← merged context (claudecode)
          .claude/skills/     ← deployed skill files
          .multigent/
            context/
              agency.md       ← agency-level prompt
              role-<team>-<role>.md
              project-<project>.md
              wakeup.md       ← autonomous routine (installed by project apply)
          heartbeat.yaml      ← set by project apply or scheduler heartbeat
          crons.yaml          ← set by project apply or cron add
          tasks.yaml          ← active tasks
          tasks_archive.yaml  ← completed tasks
          messages.yaml       ← async messages for this agent
          runs/               ← execution logs
```

---

## Context compression

Long-running agents accumulate context that may degrade quality. Each CLI has built-in auto-compression:

| Agent | Mechanism | Default | Configuration |
|-------|-----------|---------|---------------|
| **Claude Code** | auto-compact | ~90% of context window | Env vars: `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE` (threshold %, recommend 70-80), `CLAUDE_CODE_AUTO_COMPACT_WINDOW` (effective token window size) |
| **Codex** | auto-compact | ~90% of context window | Config key `model_auto_compact_token_limit` in codex config |
| **Gemini** | auto-compress | 70% of context window | `chatCompression.contextPercentageThreshold` (0-1) in `.gemini/settings.json` |

Set per-agent env vars via the Web UI (Agent → API Provider) or directly in `agent.yaml`:

```yaml
env:
  CLAUDE_AUTOCOMPACT_PCT_OVERRIDE: "75"
```

---

## Tips for agents

1. Always use `--dir <workspace>` if you are not inside the agency directory.
2. Get your own task ID from `$TASK_ID` (set by the scheduler when running a task).
3. When you need human approval **and must wait**: use `task confirm-request` — do NOT call `task done`. The task resumes when the human confirms; their reply is in `$CONFIRMATION_REPLY`.
4. When you want to notify someone **without blocking**: use `inbox send --from <your-address> --to <recipient> --body "..."`.
5. Unread messages are auto-injected at the top of your wakeup prompt — no need to call `inbox messages` yourself.
6. Use `inbox reply <msg-id> --from <your-address>` to reply to a message.
7. Use `multigent list agents` to discover all agents and their `project/agent` addresses.
8. Use `multigent exec --project P --agent A --prompt "..."` for quick one-off tests without adding a task.
