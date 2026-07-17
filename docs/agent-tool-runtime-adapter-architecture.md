# Agent Tool Runtime Adapter Architecture

本文档定义 Multigent 如何把外部工具真正接入到 agent 运行环境中。

核心判断：

- `mga` 只负责 Multigent 控制面：任务、消息、知识库、OKR、审计和 runtime manifest。
- 外部工具不应该统一强行走 `mga`。每个平台应该用它最自然、最稳定、最容易被 agent 使用的方式接入。
- Multigent 负责配置、授权、凭证托管、runtime 注入、审计和可观测性。
- Agent 负责根据同步进去的 skill 和工具说明，使用对应的 CLI、MCP tool 或受控 HTTP action 完成工作。

## 目标

当用户在 Web 上给某个 agent 配置一个工具后，agent 在 sandbox 里应该自然获得这个工具的可用能力：

- 飞书 / Lark：看到飞书相关 skill，并能直接使用 `lark-cli`。
- GitHub：看到 GitHub 相关 skill，并能直接使用 `gh`。
- Figma：通过统一 MCP Gateway 获得 Figma tool。
- Notion / Linear / Jira：根据 provider 能力，使用官方 CLI、MCP、HTTP action 或专用 skill。

用户不应该理解容器路径、环境变量、配置文件细节。agent 也不应该接触 Multigent 数据库或 workspace 全量文件。

## Provider Runtime Adapter

每个外部工具 provider 都应该声明一个 runtime adapter。adapter 是产品层连接和 agent 运行环境之间的桥。

建议结构：

```yaml
provider: lark
display_name: Lark
runtime:
  adapters:
    - type: cli
      priority: 10
      binary: lark-cli
      installer:
        type: script
        version: latest
      config:
        materialize: file
        path: ~/.lark-cli/config.json
      skills:
        - lark-doc
        - lark-im
        - lark-task
    - type: http_action
      priority: 1
      actions:
        - list_docs
        - send_message
credentials:
  materialize:
    mode: runtime_file
    secret_source: connection_secret
audit:
  command_audit: best_effort
  proxy_audit: required
```

字段含义：

- `type`: 这个 provider 的 agent 侧接入形态。
- `priority`: 多种形态同时存在时，默认推荐顺序。
- `binary`: CLI 形态下 agent 实际调用的命令。
- `installer`: sandbox 初始化时如何安装或校验工具。
- `config`: 如何把连接凭证变成该平台 CLI/MCP 能读懂的配置。
- `skills`: 同步给 agent 的使用说明和工作流约束。
- `credentials.materialize`: 凭证如何进入运行环境。
- `audit`: 这种形态能做到什么级别的审计。

## 四种接入形态

### 1. Platform CLI Adapter

适合平台本身已有成熟 CLI 的工具。

例子：

- 飞书 / Lark：`lark-cli`
- GitHub：`gh`
- GitLab：`glab`
- Kubernetes：`kubectl`
- Cloudflare：`wrangler`

运行时准备：

1. 安装或校验 CLI。
2. 从 Multigent connection secret 生成 agent 专属配置文件。
3. 把配置文件放到 agent 专属 runtime home。
4. 同步对应 skill。
5. 在 agent context 中声明可用命令和边界。

凭证策略：

- 不把 workspace 全局凭证直接挂进去。
- 为每个 agent materialize 一份最小权限配置。
- 如果平台 CLI 支持 token 文件，写 agent scoped token file。
- 如果平台 CLI 只能读固定路径，也映射到 agent 自己的 runtime home。

审计策略：

- 短期记录 run 日志和 shell command。
- 中期用 wrapper 包一层 CLI，记录 provider、command、exit code、耗时。
- 对高风险写操作，可以要求 agent 先走 `mga task confirm-request` 或未来的 policy gate。

优点：

- agent 容易用，和人类开发者习惯一致。
- 平台能力覆盖完整，不需要 Multigent 重写所有 API。
- skill 可以沉淀成稳定流程。

风险：

- CLI 调用审计不如 server proxy 精确。
- CLI 配置文件 materialize 要严格隔离。
- 部分 CLI token 权限较大，需要授权范围和 policy 管理。

### 2. Unified MCP Gateway Adapter

适合平台已有 MCP server，或者工具天然适合 tool calling 的场景。

例子：

- Figma MCP
- Browser automation MCP
- Database MCP
- 内部工具 MCP

不要把每个 MCP server 的所有 tool 直接挂给 agent。默认使用统一 gateway。

推荐形态：

```text
Agent MCP client
  -> Multigent MCP Gateway
    -> list_tools / call_tool
      -> upstream MCP server / provider action / internal tool
```

Agent 侧只看到少量稳定工具：

- `multigent.list_tools`
- `multigent.call_tool`

`list_tools` 返回当前 agent 被授权的工具目录，可以按 provider、category、keyword 过滤。

`call_tool` 根据 `tool_id` 调用真实工具：

- `mcp:figma:inspect_file`
- `mcp:browser:open_page`
- `action:github:list_issues`

凭证策略：

- 原始 provider secret 留在 Multigent server。
- Gateway 调 upstream MCP 时注入凭证。
- Agent 只持有 scoped runtime token。

审计策略：

- 每次 `call_tool` 必须记录 agent、run、connection、tool、arguments metadata、status、耗时。
- 返回内容做大小限制和 secret redaction。

优点：

- 不污染 agent context，不出现上百个 MCP tool schema。
- 权限、审计、限流可以集中做。
- 适合多 provider 统一管理。

风险：

- 相比原生 MCP，多了一层 broker。
- 某些 agent client 可能更偏好原生 tool list，需要后续支持 native mode。

### 3. HTTP Action Adapter

适合没有好用 CLI/MCP，但 API 足够稳定的平台。

例子：

- Notion API
- Linear API
- Sentry API
- Vercel API
- Airtable API

运行时方式：

- Agent 使用 provider skill 了解有哪些 action。
- 调用可以走 `mga runtime action`，或者未来通过 MCP Gateway 的 `multigent.call_tool` 间接调用。

这里 `mga runtime action` 是兜底执行器，不是所有工具的主体验。

凭证策略：

- 原始 API Key/OAuth token 留在 server。
- Agent 发 method、endpoint、query、body。
- Server 端做 allowlist、credential injection 和 redaction。

审计策略：

- Server 可以完整记录 action 调用。
- 必须有 endpoint/method allowlist。
- 写操作后续可接 policy gate。

优点：

- 实现快，安全边界清晰。
- 不需要给 agent 暴露原始凭证。

风险：

- 如果 action catalog 太原子，agent 使用成本高。
- 如果 action catalog 太粗，产品扩展不灵活。

### 4. Skill-Only / Knowledge Adapter

有些“工具”本质上不是外部执行器，而是工作方法、规范、模板和流程。

例子：

- 代码审查规范
- 项目计划模板
- 发版 checklist
- 大需求拆解流程

运行时方式：

- 只同步 skill。
- 不注入外部 CLI/MCP/HTTP action。

这类 adapter 仍然应该归入 tool runtime system，因为它影响 agent 能力配置。

## 统一接入流程

用户在 Web 给 agent 配置工具后，Multigent 在 agent run 前执行：

1. Resolve
   - 查询 agent 可用的 workspace/project/agent/user 连接。
   - 找到每个 provider 的 runtime adapter。
   - 根据优先级选择默认接入方式，也允许管理员覆盖。

2. Prepare
   - 安装或校验 CLI / MCP server / helper binary。
   - 生成 provider 专属配置文件。
   - 生成 MCP Gateway 配置。
   - 生成 runtime manifest。

3. Inject
   - 挂载或写入 agent 专属 runtime home。
   - 注入必要 env。
   - 同步 skills。
   - 更新 agent context 中的工具说明。

4. Run
   - 启动 sandbox。
   - agent 使用 CLI/MCP/action 完成任务。

5. Observe
   - 收集 run log、tool call log、proxy audit。
   - 更新连接 last used、错误状态、调用统计。

## Agent 侧统一体验

虽然底层有多种 adapter，agent 看到的应该是一致的能力说明：

```markdown
## Available External Tools

### Lark

Use `lark-cli` for Feishu/Lark docs, IM, task, calendar, and drive operations.
Read the bundled `lark-*` skills before using it.

### GitHub

Use `gh` for issues, pull requests, repositories, and workflow runs.

### Figma

Use Multigent MCP Gateway:
- `multigent.list_tools` with provider=`figma`
- `multigent.call_tool` with the returned tool id
```

规则：

- Agent 不需要知道 API Key。
- Agent 不应该手写 provider token。
- Agent 优先使用对应 skill 推荐的入口。
- 如果工具缺失，报告缺少哪个 provider/connection，而不是让人粘贴密钥。

## 数据模型补充

现有 `connections` 只表达“账号连接”。还需要表达“这个连接如何进入 agent runtime”。

建议新增静态或 DB-backed catalog：

### tool_providers

- `provider`
- `display_name`
- `category`
- `auth_types`
- `default_runtime_adapter`
- `supported_runtime_adapters`
- `enabled`

### tool_runtime_adapters

- `id`
- `provider`
- `type`: `cli` / `mcp_gateway` / `http_action` / `skill_only`
- `priority`
- `installer_json`
- `config_template_json`
- `skill_names_json`
- `action_catalog_json`
- `mcp_server_json`
- `audit_policy_json`

### agent_tool_bindings

表示某个 agent 启用了某个工具连接，以及使用哪种 runtime adapter。

- `id`
- `workspace_id`
- `project_id`
- `agent_id`
- `connection_id`
- `provider`
- `adapter_type`
- `status`
- `created_by`
- `created_at`
- `updated_at`

初期可以不单独建表，先从 active workspace connection 推导；但长期需要 agent 级绑定，否则“工具配置给谁用”会不清晰。

## 首批三类闭环

### Lark

目标：证明 Platform CLI Adapter。

- Connection：app id/app secret 或授权后的 token。
- Runtime：安装/注入 `lark-cli`。
- Config：写入 agent 专属 lark config。
- Skills：同步 `lark-doc`、`lark-im`、`lark-task` 等。
- 验证：agent 能读文档、发消息、查任务。

### GitHub

目标：证明 Platform CLI Adapter + 常见开发工具。

- Connection：PAT 或 device flow/OAuth 后的 token。
- Runtime：安装/确认 `gh`。
- Config：写入 agent 专属 `~/.config/gh/hosts.yml`。
- Skills：GitHub issue/PR/workflow skill。
- 验证：agent 能查 issue、创建 issue、查看 PR。

### Figma

目标：证明 Unified MCP Gateway Adapter。

- Connection：Figma token。
- Runtime：不直接把 Figma MCP 全量 tools 暴露给 agent。
- Gateway：`multigent.list_tools(provider=figma)` + `multigent.call_tool(tool_id=...)`。
- Skills：Figma usage skill，告诉 agent 先 list 再 call。
- 验证：agent 能读取文件结构或设计节点信息。

## 和现有代码的关系

已有能力：

- connection、secret、profile、grant、health check。
- runtime token。
- runtime manifest。
- Docker sandbox。
- agent scoped runtime home。
- `mga` 控制面 CLI。
- HTTP action proxy。
- custom MCP proxy。

需要补齐：

1. Provider runtime adapter catalog。
2. Agent tool binding / runtime resolve。
3. CLI adapter 的安装、配置文件 materialize 和隔离。
4. MCP Gateway broker mode。
5. Tool-specific skills 自动同步。
6. Agent detail 页面展示“已启用工具”和接入方式。
7. E2E：Lark CLI、GitHub CLI、Figma MCP Gateway 三条链路。

## 实施顺序

### Phase 1: Adapter Catalog

- 定义 provider runtime adapter schema。
- 给 Lark、GitHub、Figma 写静态 catalog。
- Runtime manifest 增加 `tools` 字段，描述 agent 可用工具和推荐入口。

### Phase 2: CLI Adapter

- 实现 CLI installer/checker。
- 实现 agent 专属 config materializer。
- 先支持 Lark 和 GitHub。
- 同步对应 skills。

### Phase 3: MCP Gateway Broker

- 实现 `multigent.list_tools` 和 `multigent.call_tool`。
- 先接 Figma MCP。
- 将 HTTP action catalog 也映射成 gateway tool。

### Phase 4: Web Product Loop

- 工具页面展示 provider。
- agent 页面可以启用/禁用工具。
- 展示该工具对 agent 的接入方式：CLI / MCP Gateway / HTTP Action / Skill。
- 展示健康状态、最近调用、错误。

### Phase 5: Audit And Policy

- CLI wrapper command audit。
- MCP/action call audit。
- 写操作风险分级。
- 高风险工具调用接 human approval。

## 关键原则

- 不要把所有工具都抽象成 `mga`。
- 不要把所有 MCP tools 直接塞给 agent。
- 不要把原始凭证暴露给 agent。
- 不要让用户理解环境变量和配置路径。
- 每个工具用最自然的 agent 侧入口，但统一由 Multigent 管理配置、权限、注入和审计。
