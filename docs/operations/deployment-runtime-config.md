# Deployment Runtime Config

## SMTP Invitations

For full startup configuration, see
[configuration-and-logging.md](configuration-and-logging.md) and
[config.example.toml](../config.example.toml).

Multigent sends invitation emails when SMTP is configured. Without SMTP, it keeps
the local development behavior and returns copyable invite links.

Environment variables:

```bash
MULTIGENT_SMTP_HOST=smtp.example.com
MULTIGENT_SMTP_PORT=587
MULTIGENT_SMTP_USERNAME=apikey
MULTIGENT_SMTP_PASSWORD=...
MULTIGENT_SMTP_FROM=noreply@example.com
MULTIGENT_SMTP_FROM_NAME=Multigent
MULTIGENT_SMTP_TLS=starttls # starttls | implicit | none
```

Required for SMTP delivery:

- `MULTIGENT_SMTP_HOST`
- `MULTIGENT_SMTP_FROM`

If email delivery fails, the invitation is still created and the API response
returns the invite URL plus a delivery error.

## Sandbox Capability Detection

The API exposes runtime capability detection at:

```text
GET /api/v1/sandbox/capabilities
```

It reports Docker, KVM, and E2B availability. E2B is only selectable when:

- `/dev/kvm` exists and is accessible.
- CPU virtualization flags are visible.
- `MULTIGENT_E2B_API_URL` or `E2B_API_URL` is configured.

E2B execution is still guarded behind the provider adapter. Until the self-hosted
E2B API lifecycle is connected, Docker/gVisor remains the recommended runtime
path for actual agent execution.
