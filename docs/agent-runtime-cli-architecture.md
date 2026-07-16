# Agent Runtime CLI Architecture

Multigent 的长期 Agent 集成方式应该是 **skill + CLI**，而不是把 MCP 作为主路径。MCP 可以作为外部工具生态的一种兼容层，但 Multigent 自己的协作、任务、消息、OKR、审计和权限控制应该通过面向 Agent 的 runtime CLI 完成。

## 核心判断

Multigent 不是本地优先项目管理工具，而是在线多 Agent 协作平台。

因此：

- 人类用户主要通过 Web 使用 Multigent。
- 管理员也主要通过 Web 管理 workspace、成员、团队、项目、Agent、连接、权限和审计。
- Agent 在 sandbox 内通过 `mga` CLI 和 Multigent Server 交互。
- `mga` 所有动作都必须走 Server API，由 Server 做身份鉴权、RBAC、审计和策略判断。

这意味着 CLI 要拆成两个产品面：

| CLI | 使用者 | 定位 | 是否进入 sandbox |
| --- | --- | --- | --- |
| `multigent` | 平台开发者、部署者、少量管理员 | 启动服务、迁移、调试、离线维护 | 否 |
| `mga` | sandbox 中的 Agent | 任务、消息、OKR、工具调用、运行时上下文 | 是 |

## 为什么不以 MCP 为主

MCP 的价值是标准化外部工具接入，但它不是 Multigent 的核心协作协议。

Agent 在真实工程循环里经常需要：

- 写脚本批量处理任务。
- 在 loop 中查询任务状态。
- 给其他 Agent 发消息。
- 更新任务、OKR、运行总结。
- 按结构化参数调用平台 API。
- 把结果记录进审计和 run timeline。

这些行为更适合 CLI：

- 对 Agent 更稳定，容易写进 skill。
- 适合 shell、脚本和长 loop。
- 参数、输出、退出码更容易约束。
- 不需要把大量 tool schema 放进上下文。
- 能保留和 Web/API 一致的权限、审计和错误码。

MCP 可以保留为兼容层，尤其用于接入第三方工具。但 Multigent 内部的 agent workflow 应以 `mga` 为准。

## `mga` 的命令边界

`mga` 不应该包含以下能力：

- 创建/删除 workspace。
- 管理系统配置。
- 管理数据库迁移。
- 启动/停止 server。
- 管理全局用户和高权限 RBAC。
- 直接读写 workspace 本地文件作为控制面数据。

`mga` 应该包含：

```bash
mga context show
mga task list
mga task show <id>
mga task add
mga task update <id>
mga task done --id <id> --status success
mga task confirm-request --id <id> --summary "..."
mga message list
mga message send
mga message reply
mga okr list
mga okr update
mga tool list
mga tool call
mga runtime connections
mga runtime action
mga runtime version --check
```

命令输出默认应支持：

- 人类可读 table/text。
- `--json` 结构化输出。
- 稳定错误码。
- 非 0 exit code 表示失败。

## 认证模型

Agent sandbox 初始化时注入：

```text
MULTIGENT_API_URL
MULTIGENT_AGENT_TOKEN
MULTIGENT_RUN_ID
MULTIGENT_WORKSPACE_ID
MULTIGENT_PROJECT
MULTIGENT_AGENT
MULTIGENT_CONNECTIONS_FILE
```

`mga` 不读取用户密码，不读取 workspace 管理 token，不读取 provider 原始凭证。

所有请求都带：

```http
Authorization: Bearer $MULTIGENT_AGENT_TOKEN
```

Server 根据 token 解析：

- workspace
- project
- agent
- run
- capabilities
- granted connections/tools/skills

默认 capability 包括：

- `connection.use`
- `task.use`
- `message.use`
- `okr.use`

Server 可以在发 token 时收窄 capability。每个 mutating command 都必须写 audit log。

## Skill 和 CLI 如何对齐

内置 skill 不应该告诉 Agent 使用管理 CLI。

正确写法：

```bash
mga task done --id "$TASK_ID" --status success --summary "..."
mga message send --to project/agent --subject "..." --body "..."
mga okr update <okr-id> --status on_track
```

Skill 文档必须和 `mga` 的实际命令同步。

后续建议：

- 每个 runtime CLI command 有 machine-readable help schema。
- skill 生成或同步时从 CLI schema 生成示例。
- CLI breaking change 必须同步更新内置 skill。

## Sandbox 初始化

每个 Agent sandbox 默认必须有 `mga`。

推荐路径：

```text
/opt/multigent/mga/bin/mga
```

初始化顺序：

1. 创建隔离 sandbox。
2. 注入 runtime env。
3. 安装或挂载 `mga`。
4. 安装模型 CLI，例如 Codex/Claude/Gemini。
5. materialize 当前 run 的上下文、skills、授权连接 manifest。
6. 执行 Agent CLI。

## CLI 更新策略

`mga` 也应该被当成 toolchain 管理，而不是长期固化在 runtime base image。

推荐模型：

```yaml
runtime_cli:
  name: mga
  version: server
  channel: stable
```

版本解析：

- `server`: 使用与当前 Multigent Server 兼容的默认版本。
- `latest`: 使用最新稳定 runtime CLI。
- `0.3.1`: 固定版本。

每次 run 记录：

- server version
- runtime cli version
- command version check result

启动前执行：

```bash
mga runtime version --check
```

不兼容时：

- 开发环境可以自动更新。
- 生产环境默认阻止 run，并提示管理员升级 runtime profile。

## 当前过渡实现

当前代码已先完成 sandbox 入口收敛：

- Docker sandbox 内默认把 `mga` 暴露到 `/opt/multigent/mga/bin/`。
- sandbox PATH 默认包含 `/opt/multigent/mga/bin`。
- 内置 prompt/skill 使用 `mga` 作为 Agent 侧命令。
- `mga` 已拆成独立 `cmd/mga`，只调用 scoped runtime API。
- Server 已提供 runtime task/message/connections/actions/mcp API，并基于 runtime token capability 做鉴权。

后续必须继续做：

1. Runtime CLI 增加版本校验。
2. 补齐 OKR/context/tool call API。
3. Docker provider 从“挂载当前二进制”升级为“安装指定版本 runtime CLI 到 toolchain cache”。
4. 将 task/message runtime capability 进一步细化成自己、同项目、跨项目等策略。
