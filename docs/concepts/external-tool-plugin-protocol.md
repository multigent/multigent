# External MCP Tool Protocol

本文档定义 Multigent 自定义外部工具的第一版接入方式。

当前结论：**先只支持 MCP Server**。HTTP Tool Server 暂不进入产品，因为它要求用户额外维护 manifest、接口 schema 和调用规范，使用成本高，也不如 MCP 在 agent 生态里成熟。

## 产品目标

用户可以在「外部工具」里新建一个 MCP 工具：

1. 输入工具名称。
2. 输入 MCP Server URL。
3. 可选输入 Bearer token 或自定义认证 header。
4. 点击测试连接。
5. 测试通过后，把这个工具安装或授权给项目/Agent。
6. Agent 在运行时通过 Multigent MCP Gateway 发现和调用这个工具。

面向普通用户，产品上仍然叫「外部工具」。MCP 是创建自定义工具时的一种接入方式，不要求普通用户理解 runtime 细节。

## 为什么选 MCP

MCP 已经解决了自定义工具最难的三个问题：

- 工具发现：`tools/list`
- 参数描述：每个 tool 自带 input schema
- 标准调用：`tools/call`

因此 Multigent 不需要再定义一套 HTTP manifest，也不用让用户手写接口清单。用户只要提供一个可访问的 MCP Server，Multigent 就可以把它挂到 agent runtime。

## 架构

```text
Agent sandbox
  -> mga runtime gateway list-tools / call-tool
  -> Multigent API
  -> runtime scoped auth + RBAC + audit
  -> configured MCP Server
  -> tools/list or tools/call
```

关键点：

- MCP Server 凭证只保存在 Multigent 服务端，不注入 agent sandbox。
- Agent 只能看到已授权给自己的工具。
- 所有调用通过 Multigent API 代理，方便审计、限流和 secret redaction。
- 对 agent 侧保持统一：不直接给每个 agent 配一堆 MCP server，避免工具列表膨胀。

## 连接配置

第一版字段：

```json
{
  "name": "GitHub Workflow",
  "provider": "custom-mcp",
  "serverUrl": "http://127.0.0.1:39091/mcp",
  "token": "optional-token",
  "authHeader": "Authorization",
  "authScheme": "Bearer"
}
```

字段说明：

- `serverUrl`: MCP Server 的 HTTP JSON-RPC endpoint，必填。
- `token`: 可选。服务端代理调用时使用，不暴露给 Agent。
- `authHeader`: 可选。默认 `Authorization`。
- `authScheme`: 可选。默认 `Bearer`。

如果 MCP Server 不需要认证，可以只填 `serverUrl`。

## 连接测试

测试连接时，Multigent 调用：

```json
{
  "jsonrpc": "2.0",
  "id": "multigent-health-check",
  "method": "tools/list"
}
```

成功标准：

- HTTP 状态码为 2xx。
- 响应是合法 JSON。
- MCP Server 返回 `result.tools` 或兼容的 tools list 结构。

后续可以把 tools list 快照保存下来，用于 UI 展示和变更提醒。

## Agent 使用方式

Agent 不直接拿 MCP Server URL，也不直接拿 token。

运行环境里默认安装 `mga`，Agent 通过 runtime guide 使用：

```bash
mga runtime gateway list-tools --provider custom-mcp
mga runtime gateway call-tool mcp:custom-mcp_<connection>:<tool_name> --data '{"key":"value"}'
```

后续如果 Codex / Claude Code / Cursor 对 MCP 配置能力足够稳定，也可以由 runtime materializer 自动写入对应 CLI 的 MCP 配置。但第一版仍以统一 gateway 为准，降低工具膨胀和权限泄露风险。

## GitHub Workflow 的落地方式

`github-workflow` 不应该变成 Multigent 内核里的定时同步概念。更合适的方式是：

```text
github-workflow MCP Server
  - 自己维护 issue / PR registry
  - 自己执行 sync、去重、分流状态维护
  - 对外暴露 MCP tools

Multigent
  - 只保存 MCP endpoint 和认证
  - 授权给 PM / QA / Release Agent
  - 在 agent runtime 中通过 MCP Gateway 调用
```

示例工具：

- `github_workflow.sync`
- `github_workflow.list_triage_issues`
- `github_workflow.update_issue_state`
- `github_workflow.list_pr_candidates`
- `github_workflow.record_release_decision`

这样 PM wakeup 可以专注做 issue 分流，QA wakeup 可以专注做 PR 观察和 release 候选判断，不需要 Multigent 为每个业务系统新增专用后台 worker。

## 暂不做

第一版暂不支持：

- HTTP Tool Server manifest。
- 任意本地 shell command 工具。
- 任意数据库直连。
- 未经过 MCP schema 描述的裸 HTTP proxy。
- 用户在 UI 里编辑每个 MCP tool 的细粒度 action policy。

这些能力可以以后再评估，但不进入当前产品路径。
