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
- Local Agent Session Collector：扫描运行节点上的 Claude Code、Codex、Cursor 等 session 文件。
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

## 权限原则

- 导入原始 session 可能包含敏感信息，默认不应 workspace 全员可见。
- 第一版以 workspace admin 管理导入和绑定为主。
- Agent 只能看到与自己相关的 workspace/project/agent 绑定。
- 后续在 Source、Asset、Artifact 层增加 visibility、allowed projects、allowed agents 和 sensitivity level。

## 设计原则

1. 上下文是资产，不是某个 Agent 的私有记忆。
2. 原始文件用于追溯，知识库文档用于消费。
3. 绑定关系独立于 Agent 配置，避免耦死。
4. Collector、Processor、Binding、Runtime Injection 各自高内聚。
5. 运行时要可审计：Agent 能看到哪些上下文、是否读取过，后续都应能记录。
