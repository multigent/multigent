# Unified MCP Gateway

本文档描述 Multigent 对外部 MCP Server 的兼容接入设计。Multigent 的主路径是 **skill + `mga` CLI**；MCP 只作为外部工具生态的一种接入方式，不作为 Multigent 内部协作协议。

## 背景

目前 Multigent 已经有几块基础能力：

- Connection 支持 workspace/user ownership、grant、加密凭证、health check 和 runtime manifest。
- `custom-mcp` 已经可以通过 `/api/v1/runtime/mcp` 代理 Agent 对单个 MCP Server 的 JSON-RPC 请求。
- Agent runtime 已经能拿到 `MULTIGENT_API_URL`、`MULTIGENT_AGENT_TOKEN` 和 connection manifest。
- Agent prompt 中已经注入 `mga runtime connections`、`mga runtime action`、`mga runtime mcp` 的使用说明。

这说明我们已经有了“代理一个 MCP Server”的能力，但还没有“统一 MCP 接入点”：

- Agent 需要知道自己要调哪个 connection alias。
- 工具发现分散在每个 MCP Server 里。
- 不同 MCP Server 的认证、授权、审计还没有统一产品化。
- 如果以后把多个 MCP Server 的所有工具直接暴露给 Agent，工具列表会很快膨胀，增加模型选择成本和 token 成本。

## 外部参考

调研到的开源项目大致分成三类。

| 项目 | 值得参考的点 | 不适合直接照搬的点 |
| --- | --- | --- |
| [`agentic-community/mcp-gateway-registry`](https://github.com/agentic-community/mcp-gateway-registry) | “one secure entry point to many MCP servers”、集中发现、治理和审计。这个方向和 Multigent 很接近。 | 它更像通用 AI asset registry，我们需要更强绑定 workspace、agent identity、connection grant 和 run audit。 |
| [`IBM/mcp-context-forge`](https://github.com/IBM/mcp-context-forge) | MCP/A2A/REST/gRPC federation、集中 discovery、governance、observability，适合作为企业级网关参考。 | 能力很重；Multigent 初期不应该先做完整 federation 平台。 |
| [`open-webui/mcpo`](https://github.com/open-webui/mcpo) | MCP 到 OpenAPI 的桥，强调 HTTP、文档、认证和工具互操作。 | 它主要解决 MCP 变 OpenAPI，不解决 Agent 级授权和 Multigent 内部治理。 |
| [`supergateway`](https://github.com/supercorp-ai/supergateway) / [`mcp-proxy`](https://github.com/sparfenyuk/mcp-proxy) | stdio、SSE、WebSocket、Streamable HTTP 之间的 transport bridge。 | 它们解决传输适配，不解决工具目录、权限、凭证托管和审计。 |

结论：Multigent 可以借鉴 gateway/registry 的控制面思想，以及 proxy 项目的 transport 适配能力，但 MCP Gateway 应定位为外部工具兼容层。核心 Agent 工作流仍然应通过 `mga` CLI 调用 Server API。

## 产品目标

MCP 兼容层应该做到：

1. 外部 MCP Server 能以 Connection 的形式接入 Multigent。
2. 所有工具调用都走 Multigent 的身份认证、RBAC、connection grant 和审计。
3. 人可以在 Web 上管理 MCP Server、连接凭证、授权范围和可见工具。
4. Agent 不直接接触 workspace 全量文件、数据库或原始 provider secret。
5. 工具发现要按需，不把几百个工具一次性塞进模型上下文。

## 推荐形态

兼容层可以提供一个聚合 MCP Server：

```text
Agent MCP client
  -> Multigent MCP compatibility gateway
    -> RBAC / Grant / Audit / Policy
      -> Upstream MCP Server / REST Action / Skill Tool
```

如果需要给 MCP client 暴露入口，初期只暴露两个 broker tool：

1. `multigent.list_tools`
   - 输入：可选过滤条件，例如 provider、connection、category、keyword、project、risk level。
   - 输出：当前 Agent 被授权使用的工具目录。
   - 输出内容包含 tool id、名称、描述、输入 schema、来源 connection、权限要求、风险级别。

2. `multigent.call_tool`
   - 输入：`tool_id` 和 `arguments`。
   - 服务端根据 `tool_id` 找到真实 upstream tool 或 action executor。
   - 调用前做 Agent 身份校验、connection grant 校验、tool allowlist/denylist、risk policy 判断。
   - 调用后记录 audit log、run event、tool result metadata，并做 secret redaction。

这个设计有一个重要好处：Agent 的 MCP client 里始终只有很少的工具，真实工具目录通过 `list_tools` 按需发现，避免工具列表爆炸。

## 为什么不是直接暴露所有工具

MCP 原生协议本身有 `tools/list` 和 `tools/call`。如果 Multigent 把所有 upstream tools 都展开成原生 MCP tools，Agent 使用体验会更“原生”，但会带来几个问题：

- 工具数量增长后，模型选择工具的准确率会下降。
- 每次注入大量 tool schema 会消耗 context 和 token。
- 权限变化后需要频繁刷新工具列表。
- 很难在 UI 上解释“这个 Agent 到底被授权了哪一批工具”。

因此建议分两种模式：

- Broker mode：默认模式，只暴露 `multigent.list_tools` 和 `multigent.call_tool`。
- Native mode：后续高级模式，把少量高频、安全、稳定的工具直接暴露为 MCP 原生 tools。

## 核心数据模型

建议新增或明确以下概念：

### MCPServer

表示一个可接入的 MCP Server 定义，不直接包含用户凭证。

字段建议：

- `id`
- `workspace_id`
- `name`
- `transport`: `streamable_http` / `sse` / `stdio`
- `endpoint_url`
- `command`
- `args`
- `env_template`
- `created_by`
- `created_at`
- `updated_at`

### MCPConnection

表示某个 workspace 或 user 对 MCPServer 的认证连接，可以复用现有 Connection 体系，也可以作为 Connection 的 provider subtype。

字段建议：

- `connection_id`
- `server_id`
- `owner_type`: `workspace` / `user`
- `owner_id`
- `auth_type`: `none` / `bearer` / `oauth2` / `headers`
- encrypted credential values

### MCPToolSnapshot

保存从 upstream MCP Server 同步到的工具目录快照。

字段建议：

- `id`
- `server_id`
- `connection_id`
- `tool_name`
- `tool_id`
- `description`
- `input_schema`
- `risk_level`
- `last_seen_at`
- `enabled`

`tool_id` 应该用稳定命名，例如：

```text
mcp:<connection_alias>:<tool_name>
```

### MCPToolGrant

表示某个 tool 或 tool group 被授权给 workspace/project/agent/user。

字段建议：

- `id`
- `workspace_id`
- `connection_id`
- `tool_pattern`
- `target_type`: `workspace` / `project` / `agent` / `user`
- `target_id`
- `policy`
- `created_by`

## 调用链路

### list tools

1. Agent 调用 `multigent.list_tools`。
2. Gateway 解析 `MULTIGENT_AGENT_TOKEN` 或 MCP auth token。
3. 查询当前 Agent 可见的 connection grants。
4. 查询这些 connection 的工具快照。
5. 应用 tool-level policy 和 filter。
6. 返回工具目录。

### call tool

1. Agent 调用 `multigent.call_tool`，传入 `tool_id` 和 `arguments`。
2. Gateway 根据 `tool_id` 定位 connection 和 upstream tool。
3. 校验 Agent 是否有调用该 tool 的权限。
4. 校验 risk policy，例如是否需要 human approval。
5. 调用 upstream MCP `tools/call` 或内部 action executor。
6. 对结果做 redaction、大小限制和结构化记录。
7. 写入 audit log 和 run tool event。
8. 返回结果给 Agent。

## Web 需要补的能力

Connections 页面后续可以扩展为：

- `Connections`: 管理账号连接和凭证。
- `MCP Servers`: 管理 MCP server 定义、transport、endpoint、初始化参数。
- `Tools`: 查看从 MCP server 同步来的工具列表。
- `Grants`: 给 workspace/project/agent/user 授权工具或工具组。
- `Audit`: 查看某个 Agent 在某次 run 中调用了哪些工具。

初期可以不拆页面，先在 Connections 里增加 MCP Server 配置和工具同步入口。

## 安全边界

必须坚持以下原则：

- Agent 不拿 provider 原始凭证。
- Agent 不直接访问 Multigent 数据库。
- Agent 只能通过 scoped runtime token 访问 runtime API 或统一 MCP Gateway。
- 每次工具调用必须记录 agent、run、connection、tool、arguments metadata、result status。
- 高风险 tool 可以配置为 require approval。
- 工具返回值必须做大小限制和 secret redaction。
- Upstream MCP Server 的 tool list 不能天然可信，需要 allowlist、人工启用或 workspace admin 审核。

## 实施顺序

### Phase 1: 产品闭环

- 在 Web 上补 MCP Server 管理入口。
- 对 `custom-mcp` 做工具同步：调用 upstream `tools/list`，保存快照。
- 在 Agent detail 中展示已授权 MCP tools。

### Phase 2: Unified MCP Server

- 新增 Multigent MCP Server 进程或 API 子服务。
- 暴露 `multigent.list_tools` 和 `multigent.call_tool`。
- 支持 Agent runtime token 认证。
- `call_tool` 先只支持 upstream MCP tools。

### Phase 3: 权限和审计

- 增加 tool-level grant。
- 增加高风险工具审批策略。
- Run detail 中展示 tool call timeline。

### Phase 4: Transport adapter

- HTTP/Streamable HTTP 直接接。
- SSE 直接接或通过 adapter 接。
- stdio MCP Server 通过 sandbox sidecar 或 gateway worker 托管，不直接跑在 Web API 进程里。

### Phase 5: Native tool mode

- 对少量高频工具支持直接展开为 MCP native tools。
- 默认仍保留 broker mode，避免工具数量爆炸。

## 当前建议

现在不要急着引入一个外部 gateway 项目作为核心依赖。Multigent 的差异化是 Agent team、workspace、RBAC、run、audit 和 connection grant，不是 transport proxy。

短期最合适的路线是：

1. 保留现有 `custom-mcp` runtime proxy。
2. 增加工具目录同步和 tool snapshot。
3. 实现一个 Multigent 自己的 unified MCP Server，默认只暴露 `list_tools` / `call_tool` 两个 broker tools。
4. 后续再按需要借鉴 `mcp-proxy` / `supergateway` 的 transport adapter 思路。
