# 上下文采集与绑定架构设计

## 背景

Multigent 不应把“已有 Agent 迁移”理解为复制某个 CLI 的原生会话。更稳妥的抽象是：把本地 Agent session、云文档、仓库文档、聊天记录、手动上传文件等都视为可采集、可审计、可授权、可绑定的上下文资产。

这套能力的目标是让用户把已有知识带入 Multigent，并让项目里的 Agent 在运行时可控地读取这些上下文，而不是依赖不可见、不可追溯的历史对话。

## 核心管线

```text
Context Collector
  -> Context Source
  -> Raw Asset
  -> Processor
  -> Knowledge Artifact
  -> Context Binding
  -> Runtime Injection
```

## 概念定义

### Context Collector

Collector 负责“从哪里发现和拿资料”，不负责解释资料。

内置与未来扩展方向：

- Manual Upload Collector：用户手动粘贴或上传 Markdown、JSONL、TXT 等文本内容。
- Workspace File Collector：从 Multigent 文件管理中选择已有文本文件导入。
- Local Agent Session Collector：扫描用户本机或受管机器上的 Claude Code、Codex、Cursor 等 session 文件。
- Local Directory Collector：导入某个目录下的资料。
- Git Repository Collector：导入仓库文档、README、ADR、CHANGELOG。
- Feishu Doc Collector：导入飞书文档。
- Feishu Wiki Collector：导入飞书知识库。
- GitHub Issue/PR Collector：导入 issue、PR、review thread。
- Web Page Collector：导入网页。

### Context Source

Source 是某个 Collector 的一次具体配置。例如：

- “MacBook Pro / Claude Code sessions”
- “cc-connect 飞书知识库”
- “multigent GitHub issues”
- “用户手动上传”

### Raw Asset

Raw Asset 是原始证据层，应尽量保真保存。

示例：

- `.jsonl` session 文件
- Markdown 文档
- PDF / HTML / TXT
- 飞书文档导出内容
- GitHub issue thread

### Processor

Processor 把原始资料变成 Agent 更容易消费的知识产物。

典型处理：

- 文本抽取
- 分块
- 摘要
- 去重
- 脱敏
- 结构化
- 标题生成
- 时间线提取
- 决策提取
- 本地环境假设标注

第一版先不做复杂智能处理，只做“原始内容保留 + 生成知识库文档”。

### Knowledge Artifact

Knowledge Artifact 是处理后的知识产物，通常对应一篇知识库文档。

示例：

- Agent Session Digest
- Project Background
- Key Decisions
- Open Tasks
- Environment Assumptions
- Release Checklist

### Context Binding

Binding 决定一份知识产物给谁使用。

可绑定范围：

- workspace
- project
- agent
- workflow
- workflow step
- task template
- task

第一版先实现 workspace、project、agent 三种范围。

### Runtime Injection

Agent 运行时不应默认塞入全文，而是注入一个上下文清单和读取指令：

```text
你已关联以下上下文资料：
1. [必须读] cc-connect 历史 PM session - doc: doc-xxx
2. [参考] Release SOP - doc: doc-yyy

开始任务前，先用 `mga context list` 查看上下文，并用 `mga context read <id>` 或 `mga docs show <doc-id>` 读取 required=true 的资料。
如果上下文来自旧 session，不要假设旧路径、凭证、运行环境仍然有效。
```

## MVP 范围

第一版实现：

- Collector 抽象和注册表：不同来源都输出统一的 Collected Item。
- Manual Upload Collector 数据模型。
- Workspace File Collector：从文件管理导入文本文件。
- Raw Asset 保存在 `.multigent/context-assets/`。
- Artifact 写入现有 Knowledge Base。
- Context Binding 与 Source / Asset / Artifact 统一保存在 `.multigent/context-index.yaml`。
- 已有知识库文档可以直接绑定给 workspace / project / agent；系统会自动补一条 Artifact 记录。
- Agent context build 时注入绑定上下文清单。
- Runtime API 支持当前 Agent 查看和读取绑定上下文。
- `mga context list/read`。
- 控制面 CLI：
  - `multigent context scan-sessions`
  - `multigent context import-session --path ... --bind-agent project/agent`
  - `multigent context import-file --path ... --bind-project project`
  - `multigent context bind --doc doc-xxx --agent project/agent`
- Client Token / PAT：本地 CLI 可用 workspace-scoped token 调控制面 API 上传文件或 session；这不是运行节点 token。

第一版暂不做：

- 向量搜索。
- 自动深度脱敏。
- Feishu / Notion / Google Drive 全量同步。
- 定时同步。
- 原生 session 续跑。
- 复杂 UI 入口。产品层仍主要通过“知识库 / 文件 / Agent 关联资料”承载，不把 collector 暴露成主概念。

## 当前实现边界

第一版刻意不把“上下文”做成新的主导航。用户视角是：

1. 资料先进入知识库或文件管理。
2. 管理员在 Agent 详情页把知识库文档关联给 Agent。
3. Agent 运行时看到“关联资料清单”，必要时通过 `mga context read` 读取正文。

底层保留 Source / Asset / Artifact / Binding，是为了未来能平滑接入本地 Agent session、飞书文档、仓库文档、GitHub issue/PR 等 collector，而不改变 Agent 运行时协议。

本地 Agent session 扫描优先放在用户本机 CLI 侧，而不是 Web 服务侧，也不是运行节点侧的必备能力。原因：

- session 文件在用户本机，SaaS 控制面不应该主动扫描用户 home 目录。
- 扫描只读取候选文件前 256KB 推断标题；真正导入必须由用户显式指定文件。
- 导入后原始文件会复制到 `.multigent/context-assets/`，同时生成知识库文档，后续运行节点不依赖原始本机路径。

这里要明确区分两类机器能力：

- 本地 CLI 上传：用户在自己的电脑上运行 `multigent context import-*`，用自己生成的 Client Token / PAT 把文件内容上传到控制面。Token 只证明“我是这个用户”，不代表自动拥有项目、Agent 或工作区管理权限。
- 运行节点：负责执行 Agent、拉取 run、回传输出、提供 runtime capability。运行节点 token 只用于 worker 身份和执行链路，不用于普通用户上传资料。

认证抽象：

- 第一版：用户在账号设置里自助生成静态 Client Token / PAT，scope 先支持 `context.write`。
- 后续：可以把设备码登录、OAuth、SSO、短期 token 都作为不同 token issuer，最终仍签发同一种 client credential 给 CLI 使用。
- token 必须绑定 workspace 和用户，服务端只保存 hash，不保存明文。用户创建后只显示一次。
- 服务端仍按绑定用户的 RBAC 校验资源权限：绑定 workspace 资料需要工作区管理权限；绑定 project 资料需要项目 operator 以上权限；绑定 agent 资料需要该 Agent operator 以上权限。

第一版控制面 API：

- `GET /api/v1/client-tokens`：列出当前用户在当前工作区的 client tokens，不返回 hash 或明文。
- `POST /api/v1/client-tokens`：创建 token，body 示例 `{"name":"MacBook context uploader","scopes":["context.write"]}`，响应里的 `rawToken` 只显示一次。
- `DELETE /api/v1/client-tokens/{id}`：撤销 token。
- `POST /api/v1/context/import`：CLI 上传内容到知识库并可同时绑定到 workspace/project/agent。

导入大小边界：

- 普通文本资料仍按轻量知识库资料处理，默认上限 5MB。
- 本地 Agent session 作为原始历史材料处理，默认允许到 200MB。原始 JSONL 不会内联进知识库文档，而是保存为 workspace managed file，并生成一张知识库索引卡片。
- Agent 需要完整历史时，通过 `mga context read <id>` 看到 managed file 路径，再按 `$MULTIGENT_FILES_DIR/<relative-path>` 读取原始文件。旧 session 里的路径、凭证和运行状态只能作为历史参考。

常用命令示例：

```bash
# 扫描本机 Codex 会话候选
multigent context scan-sessions --cli codex --limit 20

# 导入一个 Claude Code 会话，并绑定给 demo/Lina
multigent context import-session \
  --path ~/.claude/projects/example/session.jsonl \
  --cli claudecode \
  --title "旧 Claude Code 上下文" \
  --bind-agent demo/Lina \
  --required

# 上传到远端 / SaaS 控制面，不要求本地是 multigent workspace
export MULTIGENT_API_URL=https://app.multigent.dev
export MULTIGENT_WORKSPACE_ID=my-workspace
export MULTIGENT_CLIENT_TOKEN=mgpat_xxx
multigent context import-session \
  --path ~/.codex/sessions/example.jsonl \
  --cli codex \
  --title "旧 Codex 会话" \
  --bind-agent demo/Lina \
  --required

# 直接把已有知识库文档绑定给某个 Agent
multigent context bind --doc doc-20260817-abc123 --agent demo/Lina --required
```

## 权限原则

- 导入原始 session 可能包含敏感信息，默认不应 workspace 全员可见。
- 第一版中，用户可以生成自己的 CLI token；导入和绑定动作按用户在 workspace / project / agent 上的权限校验。
- Agent 只能看到与自己相关的 workspace/project/agent 绑定。
- 后续在 Source、Asset、Artifact 层增加 visibility、allowed projects、allowed agents 和 sensitivity level。

## 设计原则

1. 上下文是资产，不是某个 Agent 的私有记忆。
2. 原始文件用于追溯，知识库文档用于消费。
3. 绑定关系独立于 Agent 配置，避免耦死。
4. Collector、Processor、Binding、Runtime Injection 各自高内聚。
5. 运行时要可审计：Agent 能看到哪些上下文、是否读取过，后续都应能记录。
