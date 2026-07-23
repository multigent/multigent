# multigent — Roadmap

This document captures upcoming features and longer-horizon ideas. Nothing here is a commitment or a timeline — it is a living record of where the project is heading and why.

---

## Near-term

### Skill registry & one-command install

Right now skills are downloaded with `curl`. We want a first-class registry:

```bash
multigent skill install agency-messaging
multigent skill install github-pr-review
multigent skill search "code review"
multigent skill list --available
```

A central index (hosted on GitHub or a CDN) maps skill names to URLs. Anyone can publish a skill by submitting a PR to the index. Skills can declare dependencies on other skills.

---

### Role template library

Pre-built role definitions for the most common positions — engineering, QA, product management, design, data, DevOps, security, technical writing — available as a community-maintained library. Pull a role in one command:

```bash
multigent role install --team engineering --name "senior-engineer"
multigent role install --team qa          --name "qa-lead"
multigent role install --team product     --name "product-manager"
```

Each role ships with a prompt, recommended skills, suggested heartbeat intervals, and an example wakeup routine. Users customise from there.

---

### Sprint workflow wakeup templates

A set of opinionated wakeup routines that follow a structured sprint lifecycle:

```
Think → Plan → Build → Review → Test → Ship → Reflect
```

Agents that follow this pattern produce consistent, auditable work cycles. Ship as downloadable `wakeup.md` templates alongside skill definitions.

---

### Event-driven wakeups (webhooks)

Today agents only wake up on a time-based heartbeat. Add event-driven triggers:

```bash
multigent scheduler webhook add \
  --project my-api --agent qa \
  --event "github.pull_request.opened" \
  --secret $WEBHOOK_SECRET
```

When a GitHub PR is opened, the QA agent wakes up immediately — no polling, no waiting for the next heartbeat window. Support GitHub, GitLab, Linear, Jira, and custom HTTP webhooks.

---

### Budget caps and cost alerts

```bash
multigent hire ... --daily-budget 5.00 --alert-at 4.00
```

Track token spend per agent per day/week/month. Pause the agent automatically when the budget is reached. Alert the human inbox when spend passes a threshold. Surfaces in `overview`.

---

### Parallel task execution

Today each agent runs tasks sequentially. Allow an agent to execute up to N tasks in parallel (in separate sandboxed processes):

```bash
multigent hire ... --max-parallel 3
```

Useful for agents whose tasks are independent (e.g. a writer generating multiple blog posts, a QA agent running separate test suites).

---

## Medium-term

### Web dashboard

A local web UI (served on `localhost`) that renders everything currently in `multigent overview` — plus live log tailing, task history, inbox, token spend graphs, and the ability to approve/reject confirm-requests from the browser.

```bash
multigent dashboard        # open in default browser
multigent dashboard --port 8080
```

No cloud, no account. Runs entirely on your machine.

---

### Agent memory (persistent context between wakeups)

Today each wakeup is stateless — agents have no memory of previous cycles beyond what they wrote to disk. Add a structured memory layer:

- **Short-term memory**: what happened in the last N wakeup cycles (auto-summarised)
- **Long-term memory**: key facts, decisions, and observations the agent explicitly stores with `multigent memory set`
- Memory is injected into the wakeup prompt alongside unread messages

```bash
multigent memory list  --project my-api --agent dev
multigent memory set   --project my-api --agent dev --key "auth-approach" --value "JWT, RS256"
multigent memory clear --project my-api --agent dev
```

---

### Agent-to-agent task delegation

Agents can currently send inbox messages to other agents. Extend this to formal task delegation: a PM agent creates a task directly in a dev agent's queue, with context and acceptance criteria.

```bash
# From inside an agent's wakeup session:
multigent task add \
  --project my-api --agent dev \
  --title "Implement rate limiting on /auth/login" \
  --delegated-by my-api/pm \
  --prompt "Limit to 5 attempts per 15 minutes per IP. Add integration tests."
```

The delegating agent tracks the delegated task and is notified on completion or failure.

---

### MCP server integration

Expose multigent as an [MCP](https://modelcontextprotocol.io/) server so AI assistants (Claude Desktop, Cursor, etc.) can control multigent directly as a tool:

- `list_agents` — return running agents and their status
- `add_task` — queue a task for an agent
- `read_inbox` — read pending messages and confirmations
- `confirm_task` — approve or reject a confirm-request

Also support agents *consuming* MCP servers as part of their skill set.

---

### Multi-model review chains

Assign reviewer agents with a different model than the author agent. The chain runs automatically:

```yaml
# teams/engineering/roles/developer/role.yaml
review_chain:
  - agent: review-claude   # model: claudecode
  - agent: review-codex    # model: codex
  merge_require: all_approved
```

When a dev agent completes a task and opens a PR, the review chain triggers. Both reviewers must approve before the PR can merge. Cross-model blind spots are surfaced automatically.

---

### GitHub / GitLab native integration

First-class integration beyond what skills can do today:

- Auto-create tasks from issues (labelled `agent-ready`)
- Auto-link task completion to PR status checks
- Populate `projects/<name>/prompt.md` from repo metadata (language, framework, CI config) on first hire
- Surface PR review status in `overview`

---

## Longer-horizon

### Template marketplace

A community hub for sharing full workspace templates (teams + roles + skills + wakeup routines + project blueprints). Browse, preview, and install in one command:

```bash
multigent template browse
multigent template install saas-startup
multigent template install content-agency
multigent template publish --name "starter-workspace"
```

Templates are versioned, rated, and searchable by use case, model, or team structure.

---

### Cloud sync for team collaboration

Today one person runs one agency on one machine. Enable multiple humans to share an agency:

- Agency state (tasks, inbox, agent configs) is synced to a lightweight backend (self-hostable)
- Multiple humans can send inbox messages, approve confirmations, and monitor the dashboard from anywhere
- Agents remain local; only their state (task queue, inbox, metadata) is shared

No proprietary cloud required. Works with any S3-compatible store or a simple sync server.

---

### Inter-agency federation

Let two separate Multigent workspaces communicate — useful for organisations where different teams run independent workspaces:

```bash
multigent agency peer add \
  --name "design-studio" \
  --endpoint https://design-studio.internal/multigent

# Then agents can inbox-message across agencies:
multigent inbox send \
  --to design-studio/lead-designer \
  --subject "Need mockups for issue #88"
```

---

### Audit log and compliance mode

A tamper-evident, append-only log of every action taken by every agent: tasks started, commands run, files modified, messages sent, confirmations requested. Exportable as JSON or signed NDJSON.

```bash
multigent audit log --project my-api --agent dev --since "7 days ago"
multigent audit export --format jsonl --sign
```

Useful for teams with compliance requirements who need to demonstrate what AI agents did and when.

---

### Agent health and anomaly detection

Monitor agents for signs of trouble — infinite loops, repeated failures, runaway token spend, or suspiciously long wakeup cycles — and surface them proactively:

```
[alert] my-api/dev  task #42 has been in_progress for 4h (threshold: 1h)
[alert] my-api/pm   token spend today: $12.40 (budget: $5.00) — agent paused
[alert] my-api/qa   3 consecutive failures — entering cooldown (next wakeup in 2h)
```

Auto-pause, notify inbox, and require human confirmation before resuming.

---

## Philosophy

Features are prioritised by this question: **does this let agents do more useful autonomous work, or does it reduce the risk of them doing harmful autonomous work?** Both matter equally. Speed without safety is reckless; safety without capability is useless.

The goal is never to replace human judgment — it is to make human judgment necessary only at the moments that actually require it.
