# Command Reference

All commands accept a `--dir <path>` global flag to operate on a workspace outside the current directory.

```bash
multigent --dir /path/to/MyAgency inbox list
```

---

## `create` — workspace setup

```bash
multigent create agency  --name "MyAgency" [--desc "..."] [--template file.tar.gz|dir|URL]
multigent create team    --name "engineering" [--desc "..."]
multigent create role    --team "engineering" --name "developer" [--desc "..."]
multigent create project --name "my-api" [--desc "..."] [--repo "/path/to/repo"]
multigent create project --name "my-api" --blueprint default  # from a project blueprint
```

---

## `project` — project lifecycle

```bash
# List blueprints shipped with the template
multigent project blueprints

# Show project.yaml (agents, heartbeats, playbooks)
multigent project show --project my-api

# One-command bootstrap: hire all agents + configure heartbeats/crons + install playbooks
multigent project apply --project my-api
multigent project apply --project my-api --dry-run   # preview
multigent project apply --project my-api --force     # re-hire existing agents
```

**`project-blueprints/default.yaml`** example:

```yaml
name: "{{PROJECT_NAME}}"
description: "REST API service"
agents:
  - name: dev
    role: developer
    team: engineering
    model: claudecode
    sandbox: true
    heartbeat:
      enabled: true
      interval: 30m
      active_hours: "09:00-20:00"
      active_days: weekdays
    playbook: dev.md          # installed as wakeup.md by project apply

  - name: pm
    role: product-manager
    team: product
    model: claudecode
    heartbeat:
      enabled: true
      interval: 30m
    playbook: pm.md
```

---

## `hire` / `assign` / `fire` / `sync`

```bash
# Hire an agent (hire and assign are identical)
multigent hire \
  --project my-api --team engineering --role developer \
  --model claudecode --name dev \
  [--sandbox docker] [--force]

# Re-sync context after editing prompts or skills
multigent sync --project my-api --name dev
multigent sync --project my-api   # all agents in project
multigent sync                    # entire agency

# Fire (remove) an agent
multigent fire --project my-api --agent dev           # soft delete → .fired/
multigent fire --project my-api --agent dev --force   # hard delete

# HTTP / OpenAI-compatible backend (Ollama, LM Studio, custom API)
multigent hire --project my-api --team engineering --role developer \
  --model http-agent --name local-llm \
  --http-url "http://localhost:11434/v1/chat/completions" \
  --http-model "llama3.2"
# See docs/http-agent.md for the full wire format and agent.yaml reference.
```

---

## `agent` — per-agent utilities

```bash
# Change runtime (e.g. Claude Code → Codex): regenerates context, removes the old format’s files
multigent agent set-model --project my-api --name dev --model codex

# Switch to http-agent (requires --http-url; same flags as hire)
multigent agent set-model --project my-api --name bot --model http-agent \
  --http-url "http://localhost:11434/v1/chat/completions" --http-model "llama3.2"
```

`set-model` keeps hire metadata (team, role, `hired_at`, playbook, sandbox, `add_dirs`) but clears `run_command` (so the new model’s default CLI is used) and drops `http_agent` when leaving `http-agent`. If you use a **fixed** Docker `sandbox.docker.image`, verify it matches the new model or clear the image so the default for that model is used.

---

## `task` — task queue

```bash
multigent task add    --project P --agent A --title "T" --prompt "..." \
                      [--type feature|bug|chore] [--priority 0-3]
multigent task list   --project P --agent A [--status pending] [--archived]
multigent task show   <task-id>
multigent task set    <task-id> [--title T] [--status S] [--priority N] [--type T] \
  [--description D] [--summary S] [--label L]... [--parent ID] [--due-date YYYY-MM-DD] \
  [--estimate-duration 30m] [--assignee A] [--prompt P | --prompt-file PATH] [--position N]
multigent task stats  [--since today] [--project P] [--agent A] [--assignee X] [--label L] \
  [--by agent|assignee|label|label:value|label:category] [--detail] [--format json]
multigent task cancel <task-id>
multigent task retry  <task-id>

# Stop all running or pending tasks (emergency halt)
multigent task stop-all --project P [--agent A | --all-agents] \
                        [--include-running] [--no-pending]

# View token usage and cost across agent runs
multigent task tokens --project P [--agent A | --all-agents] [--all]

# Called by the agent inside its prompt:
multigent task complete --id <id> --status success --summary "what was done"
multigent task complete --id <id> --status failed  --error "reason"

# Route to human inbox for a decision (blocks current task until human responds):
multigent task confirm-request --id <id> --summary "PR ready" \
  --action-item "Review the diff" \
  --action-item "Confirm merge"
```

**Task priority:** 0=critical, 1=high, 2=normal (default), 3=low. The scheduler always picks the highest-priority pending task first.

**Task lifecycle:**
```
pending → in_progress → done_success
                      → done_failed  → (auto-retry if max_retries set)
                      → awaiting_confirmation → done_success (via inbox reply)
```

---

## `run` / `exec`

```bash
multigent run  --project P --agent A              # execute next pending task
multigent run  --project P --agent A --task <id>  # run a specific task
multigent exec --project P --agent A --prompt "..." # one-shot, no task queue
```

---

## `inbox` — confirmations and async messaging

The inbox has two distinct concepts.

### Task confirmations — agent pauses and waits for your decision

```bash
multigent inbox list
multigent inbox list --to cc-connect/pm          # filter by recipient
multigent inbox show    <task-id>                # summary, action items, log tail
multigent inbox reply   <task-id> --body "yes, proceed"
multigent inbox forward <task-id> --to <project>/<agent> --note "..."
```

### Async messages — non-blocking communication between any participants

```bash
# Send (single or group)
multigent inbox send \
  --from cc-connect/pm \
  --to   cc-connect/dev-claude \
  --subject "New task context" \
  --body "Extra info for the task I just created..."

# Group send — repeat --to
multigent inbox send \
  --from cc-connect/pm \
  --to cc-connect/dev-claude --to cc-connect/qa-reviewer --to human \
  --subject "Sprint kick-off" \
  --body "New sprint starts Monday."

# Read
multigent inbox messages                                         # human's mailbox (unread)
multigent inbox messages --recipient cc-connect/pm              # agent's mailbox
multigent inbox messages --from human                           # filter by sender
multigent inbox messages --all                                  # include read messages
multigent inbox messages --archived                             # show archived
multigent inbox messages --mark-read                            # mark as read after listing

# Reply
multigent inbox reply <msg-id> --from <your-address> --body "..."

# Forward
multigent inbox fwd <msg-id> --from <your-address> --to <recipient>
multigent inbox fwd <msg-id> --from cc-connect/pm \
  --to cc-connect/dev-claude --to cc-connect/qa-reviewer \
  --note "Please coordinate."

# Per-message status management
multigent inbox read    <msg-id> --recipient <your-address>   # mark as read
multigent inbox archive <msg-id> --recipient <your-address>   # archive (hidden from normal list)
multigent inbox delete  <msg-id> --recipient <your-address>   # permanent delete
multigent inbox rm      <msg-id> --recipient <your-address>   # alias for delete
```

Agents receive unread messages automatically at the top of their wakeup prompt. No polling needed in `wakeup.md`.

---

## `scheduler` — heartbeat scheduler

The heartbeat is a **non-overlapping wakeup loop**: after each cycle completes all pending tasks, the agent sleeps for `interval`, then wakes again. When the queue is empty, the **wakeup routine** fires instead.

```bash
# Configure heartbeat for one agent
multigent scheduler heartbeat --project P --agent A \
  --enable --interval 30m \
  --active-hours "09:00-18:00" \  # only wake in this window (local time)
  --active-days  "weekdays"       # Mon–Fri only (or Mon,Wed,Fri / weekends)

# Set a wakeup routine (runs when queue is empty)
multigent scheduler heartbeat --project P --agent A \
  --wakeup-prompt-file /path/to/wakeup.md

# Start scheduler (all enabled agents)
multigent scheduler start         # alias: sched, s
multigent scheduler stop
multigent scheduler status
```

Overnight ranges like `22:00-06:00` are supported. Startup jitter is applied automatically so agents don't all wake up simultaneously after a restart.

---

## `cron` — scheduled tasks

```bash
multigent cron add    --project P --agent A \
  --title "Daily standup" --schedule "0 9 * * 1-5" \
  --prompt "Generate a standup report..."
multigent cron list   --project P --agent A
multigent cron delete <cron-id>  --project P --agent A
multigent cron enable <cron-id>  --project P --agent A
multigent cron disable <cron-id> --project P --agent A
```

Crons enqueue a new task each time the schedule fires. The scheduler checks for due crons on every heartbeat wakeup.

---

## `template` — share agencies

```bash
# Pack the current agency as a shareable template
# Includes: agency-prompt.md, teams/, skills/, agent-playbooks/, project-blueprints/
multigent template pack --output tech-agency.tar.gz \
  --name "tech-project" --version "1.0.0" \
  --author "Alice" --email "alice@example.com" \
  --description "Standard software engineering agency template" \
  --keywords "engineering,software"

# Inspect a template (local file, directory, or remote URL)
multigent template info tech-agency.tar.gz
multigent template info tech-agency.tar.gz --json

# Create an agency from a template
multigent create agency --name "MyAgency" --template tech-agency.tar.gz
multigent create agency --name "MyAgency" --template https://example.com/tpl.tar.gz
```

---

## `role` — role management

```bash
multigent role list  --team engineering
multigent role skill add    --team engineering --role developer --skill github-push-relay
multigent role skill remove --team engineering --role developer --skill github-push-relay
```

---

## `session` / `list` / `show` / `version`

```bash
multigent session show  --project P --agent A
multigent session clear --project P --agent A
multigent list teams | projects | agents | skills
multigent show team engineering
multigent show project my-api
multigent show agent my-api dev [--raw]
multigent version
```
