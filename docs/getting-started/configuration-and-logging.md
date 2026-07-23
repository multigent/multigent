# Configuration and Logging

## Configuration Sources

Long-running commands support TOML configuration:

```bash
multigent --config ./config.toml start
multigent --config ./config.toml api serve
multigent --config ./config.toml daemon install
```

Priority order:

```text
CLI flags > environment variables > config.toml > defaults
```

The global config path can also be set with:

```bash
MULTIGENT_CONFIG=/etc/multigent/config.toml
```

See [config.example.toml](../config.example.toml).

Currently covered by config:

- workspace directory
- server listen address
- API key
- SMTP invitation delivery
- service logging
- self-hosted E2B API URL
- remote playbook registries

## Logging Policy

Multigent has two log types:

- Service logs: API/web process lifecycle and platform events.
- Agent run logs: per-agent stdout/stderr and execution transcript.

Service logs:

- Default path: `~/.multigent/logs/multigent.log`
- Default format: JSON
- Default level: `info`
- Default rotation: one backup file at `<log>.1`
- Default max size: `10 MB`

Recommended production config:

```toml
[logging]
file = "/var/log/multigent/multigent.log"
level = "info"
format = "json"
max_size_mb = 100
stderr = false
```

Agent run logs remain scoped to each agent workspace:

```text
projects/<project>/agents/<agent>/.multigent/runs/
```

Those logs preserve raw agent output for replay/debugging. Service logs should be
structured and easy to ship to a central collector; run logs are artifacts tied
to an execution record.

## Environment Variables

Logging:

- `MULTIGENT_LOG_FILE`
- `MULTIGENT_LOG_LEVEL`
- `MULTIGENT_LOG_FORMAT`
- `MULTIGENT_LOG_MAX_SIZE_MB`
- `MULTIGENT_LOG_STDERR`

Compatibility:

- `MULTIGENT_LOG_MAX_SIZE` is still accepted as bytes for daemon installs.

Server:

- `MULTIGENT_SERVER_ADDR`
- `MULTIGENT_API_ADDR`
- `MULTIGENT_WEB_API_KEY`

SMTP:

- `MULTIGENT_SMTP_HOST`
- `MULTIGENT_SMTP_PORT`
- `MULTIGENT_SMTP_USERNAME`
- `MULTIGENT_SMTP_PASSWORD`
- `MULTIGENT_SMTP_FROM`
- `MULTIGENT_SMTP_FROM_NAME`
- `MULTIGENT_SMTP_TLS`

Sandbox:

- `MULTIGENT_E2B_API_URL`

Playbooks:

- `MULTIGENT_PLAYBOOK_REGISTRY_URLS`
