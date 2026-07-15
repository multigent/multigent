# Run Telemetry Storage Evaluation

Run telemetry should not be blindly merged into the control-plane DB yet.

Multigent has two different data classes:

- Control-plane state: users, workspaces, teams, projects, agents, tasks, messages, schedules, permissions.
- Execution telemetry: run rows, token usage, command summaries, session IDs, raw logs, stdout/stderr, tool events.

These classes have different retention, privacy, volume, and query requirements.

## Current Position

Keep the current local telemetry SQLite database and file-backed raw logs for now.

Reasons:

- Run logs can contain prompts, code, secrets accidentally printed by tools, and customer data. They need a stricter privacy and retention model than task metadata.
- Run events are append-heavy and can grow much faster than control-plane records.
- Raw logs are better stored as files or object storage, not relational rows.
- Summary queries already work through `internal/telemetry`, and moving them now does not unlock the team/project/workspace model work.

## SaaS Direction

For SaaS, Multigent should separate run metadata from raw artifacts:

- relational DB: compact `agent_runs` rows for filtering, billing, status, workspace/project/agent/task linkage
- object storage: raw logs, large JSONL event streams, stdout/stderr, artifacts
- event pipeline: optional streaming ingestion for live run views and loop engineering dashboards

The relational `agent_runs` table should include:

- `workspace_id`
- `project_id`
- `agent_id`
- `task_id`
- `run_id`
- `kind`
- `status`
- `model`
- `sandbox`
- `session_id`
- `started_at`
- `finished_at`
- `duration_ms`
- token and cost fields
- `log_object_key`
- `prompt_hash`
- `error_summary`

Raw prompt text and full command output should not be stored in ordinary relational columns by default.

## Migration Criteria

Move run summaries into the control-plane repository only when at least two of these are true:

- Web dashboards need cross-workspace run analytics.
- Billing needs authoritative usage aggregation.
- Worker/cloud-agent orchestration needs run status joins against task and agent tables.
- Retention policy and redaction rules are implemented.
- Object storage exists for raw logs.

Until then, telemetry remains a bounded subsystem with its own reader/writer API.
