# Connector And Credential Architecture

This document captures the target connector and credential model for Multigent.

It is informed by `/root/code/spaceship/3rd/open-connector`, especially its provider catalog, auth type, connection name, credential profile, and encrypted local storage concepts. Multigent adds workspace/user/project/agent ownership and grant rules because agents are delegated workers inside a team.

## Product Goal

Agents need safe access to external tools without receiving broad workspace secrets.

Examples:

- Feishu / Lark
- DingTalk
- GitHub
- Google Drive
- Notion
- Linear
- Jira
- Plane
- Databases
- Custom MCP servers

The product should support both:

- Workspace-level connections created by admins and reusable by many agents.
- User-level connections created by normal users and usable only by agents they own or manage.

## Concepts From OpenConnector To Reuse

OpenConnector has several ideas that map well to Multigent:

- Provider catalog: provider capabilities and auth requirements are data-driven.
- Auth types: `no_auth`, `api_key`, `custom_credential`, `oauth2`.
- Connection name: each provider can have a `default` connection and named connections.
- Credential validation: providers can validate a credential and return profile metadata.
- Credential profile: stable account identity, display name, scopes.
- Raw secrets are not returned to callers.
- Secrets can be encrypted at rest.

Multigent should keep these concepts, but expose them as team product primitives.

## Multigent-Specific Additions

### Ownership Scope

Every connection has an owner scope:

- `workspace`: created by workspace owner/admin, usable according to grants.
- `user`: created by a user, usable by that user and agents they are allowed to operate.
- `project`: bound to one project.
- `agent`: bound to one agent only.

Recommended first implementation:

- Support `workspace` and `user` owners first.
- Add project/agent owner scopes after grant checks are mature.

### Grants

Connections are not automatically visible to every agent.

Grant target types:

- `workspace`: broad default for all workspace agents, admin only.
- `project`: all agents in a project.
- `agent`: one agent.
- `user`: user can manually use it in UI/API.

Grant rules:

- Workspace admin can grant workspace-owned connections to any project/agent.
- User-owned connections can only be granted to agents the user owns or can operate.
- Agent runtime receives only connections granted to that agent.
- Agents never read raw secrets from the database.

### Credential Runtime Profile

Store profile metadata separately from encrypted secret payload:

- `account_id`
- `display_name`
- `granted_scopes`
- `expires_at`
- `last_validated_at`

This lets humans and agents understand which external account will be used without exposing credentials.

Profile data is not a credential store. If a client accidentally submits secret-like profile keys such as `token`, `apiKey`, `appSecret`, `password`, `secret`, or `credential`, API responses and audit events must redact those keys before returning profile data.

## Target Tables

### connector_providers

Provider registry can start as static code or catalog files, then move to DB if needed.

Fields:

- `provider`
- `display_name`
- `auth_types`
- `catalog_json`
- `enabled`

### connections

Fields:

- `id` UUID
- `workspace_id`
- `provider`
- `connection_name`
- `owner_type`: workspace / user / project / agent
- `owner_id`
- `auth_type`
- `status`: active / invalid / revoked
- `profile_json`
- `created_by_user_id`
- `created_at`
- `updated_at`
- `last_used_at`

Unique key:

- `(workspace_id, provider, owner_type, owner_id, connection_name)`

### connection_secrets

Fields:

- `connection_id`
- `ciphertext`
- `nonce`
- `key_version`
- `updated_at`

Rules:

- Keep secret payload out of normal connection list APIs.
- Encryption key is configured by deployment.
- Missing encryption key can be allowed in local development only with a startup warning.

### connection_grants

Fields:

- `id` UUID
- `workspace_id`
- `connection_id`
- `target_type`: workspace / project / agent / user
- `target_id`
- `created_by_user_id`
- `created_at`

Rules:

- Grants are checked every time an agent runtime requests connector config.
- Grant changes produce audit log events.

## API Shape

Provider catalog:

- `GET /api/v1/connectors/providers`
- `GET /api/v1/connectors/providers/{provider}`

Connections:

- `GET /api/v1/connections`
- `POST /api/v1/connections`
- `GET /api/v1/connections/{id}`
- `DELETE /api/v1/connections/{id}`

Grants:

- `GET /api/v1/connections/{id}/grants`
- `POST /api/v1/connections/{id}/grants`
- `DELETE /api/v1/connections/{id}/grants/{grantId}`

Agent runtime:

- `GET /api/v1/agents/{agentId}/runtime/connections`

The runtime endpoint should return connection references and injected environment/MCP config, not raw DB rows.

Current implementation uses the project/name route while the data model is still moving toward durable IDs:

- `GET /api/v1/projects/{project}/agents/{agent}/runtime/connections`
- `POST /api/v1/projects/{project}/agents/{agent}/runtime/token`
- `GET /api/v1/runtime/connections`
- `POST /api/v1/runtime/mcp`
- `POST /api/v1/runtime/actions`

Response contract:

- `manifest.version`: `multigent.connections.v1`
- `manifest.apiBaseUrlEnv`: environment variable that points to the Multigent control API.
- `manifest.agentTokenEnv`: environment variable that contains the scoped agent/run token.
- `manifest.connectionsFileEnv`: environment variable that can point to a materialized JSON copy of this manifest inside a sandbox.
- `manifest.mcpProxyPath`: Multigent-hosted MCP proxy path.
- `manifest.actionProxyPath`: Multigent-hosted action proxy path.
- `connections[].runtime.alias`: stable provider/connection alias for agent prompts and MCP client config.
- `connections[].runtime.mcpProxy`: how the sandbox should call the MCP proxy.
- `connections[].runtime.actionProxy`: how the sandbox should call the action proxy.
- `connections[].runtime.actions`: provider-defined action catalog entries still allowed by this connection's action policy. Each entry includes action name, method, endpoint, description, and input schema. It does not include credentials.

Rules:

- Runtime response may include provider, connection name, owner scope, profile metadata, matched grants, and proxy call instructions.
- Runtime response must not include raw provider secrets, encrypted ciphertext, nonce, or submitted credential values.
- User grants do not automatically become agent runtime access. Agent runtime only receives workspace, project, or agent grants that match the current agent.
- The sandbox should receive this data as a manifest file or fetch it with a scoped agent token.
- The agent should call Multigent proxy endpoints with the scoped token; Multigent applies provider credentials server-side.
- User JWTs and workspace admin API keys are not runtime credentials. Runtime proxy endpoints accept only scoped agent runtime tokens.
- A runtime token contains workspace, project, agent, optional run ID, capabilities, and expiry.
- Agents fetch their own runtime connection manifest through `GET /api/v1/runtime/connections` with the scoped runtime token.
- Agent runs receive `MULTIGENT_API_URL`, `MULTIGENT_AGENT_TOKEN`, `MULTIGENT_RUN_ID`, and `MULTIGENT_WORKSPACE_ID` automatically when the API server address is known. Docker runtimes inherit these variables by name so the token value does not appear in the `docker run` argv.
- Runtime proxy endpoints re-check connection grants on every request instead of trusting the connection manifest.
- Provider catalogs can declare standard action schemas. Runtime manifests expose only actions allowed by the connection's method/endpoint action policy; proxy calls still re-check the same policy before upstream credentials are used.
- `custom-mcp` runtime MCP proxy is implemented first. The agent posts JSON-RPC to Multigent; Multigent forwards it to the configured MCP server and applies the stored connection token server-side.
- `custom-http`, `github`, and `linear` can use the runtime action proxy. The agent posts a JSON proxy request to Multigent; Multigent validates the relative endpoint, applies the stored credential server-side, and returns a sanitized response envelope.
- Other provider action/MCP executors should follow the same boundary: agent token in, grant check, server-side credential application, no raw credential response.

Runtime action proxy request:

```json
{
  "method": "GET",
  "endpoint": "/repos/org/repo/issues",
  "query": { "state": "open" },
  "headers": { "Accept": "application/json" }
}
```

Rules:

- `endpoint` must be a relative path beginning with `/`; absolute URLs, `//host`, backslashes, and `..` traversal segments are rejected.
- Allowed methods: `GET`, `HEAD`, `POST`, `PUT`, `PATCH`, `DELETE`.
- `GET` and `HEAD` requests cannot include a body.
- Agent-supplied `Authorization`, `Cookie`, host, connection, and Multigent connection headers are ignored. Multigent injects provider credentials from the stored connection.
- Large responses are rejected and known credential values are redacted from the response body.

Runtime CLI helpers:

```bash
multigent runtime connections --format table
multigent runtime mcp --connection custom-mcp_docs --data '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
multigent runtime action --connection github_default --data '{"method":"GET","endpoint":"/user"}'
```

## Permission Rules

Workspace admin:

- Create workspace-owned connections.
- Grant workspace-owned connections broadly.
- Revoke any workspace connection.

Project manager:

- View granted project connections.
- Grant eligible user-owned connections to agents they manage only if they own the connection or workspace admin approved it.

Normal user:

- Create user-owned connections.
- Grant user-owned connections to agents they own or operate.
- Revoke their own user-owned connections.

Agent:

- Can list only connections granted to itself.
- Can use connection through runtime injection/action proxy.
- Cannot inspect raw secret payload.

## Audit Events

Record:

- `connection.create`
- `connection.update`
- `connection.revoke`
- `connection.validate`
- `connection.grant.create`
- `connection.grant.delete`
- `connection.use`
- `connection.oauth.start`
- `connection.oauth.complete`
- `connection.oauth.fail`

Audit payload should include provider, connection name, owner scope, target scope, and profile summary. It must not include raw secrets.

## Implementation Order

1. Add audit log foundation.
2. Add static provider registry.
3. Add connection and grant tables.
4. Add API key/custom credential flow for one provider.
5. Add UI for workspace-owned and user-owned connections.
6. Add agent runtime connection resolver.
7. Add OAuth flow.
8. Add MCP/action proxy integration.

Do not implement OAuth before ownership, grants, and audit exist.
