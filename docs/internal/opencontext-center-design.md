# OpenContext Center 设计草案

> 状态：草案
> 日期：2026-08-25
> 目标：设计一套独立的 Context Center 模块，让 Multigent 能接入、管理、清洗、订阅和检索来自内外部的信息，并逐步与 Attention Signal、知识库和权限系统解耦集成。

## 背景

Multigent 已经有 Agent Worker、任务、流程、IM 协作、Attention Signal 和知识库能力。随着 agent 需要处理的事情变多，仅依赖 agent 自身 session 记忆、system prompt 和少量唤醒信号会不够稳定：

- 信息散落在 IM 群聊、飞书文档、会议纪要、GitHub、Linear、Sentry、网页、推文、新闻、agent session 和本地文件中。
- 不是所有信息都应该进入 prompt；大量信息只应该可查询、可订阅、可沉淀。
- 信息不等于知识。很多信息只是短期事件，少部分会被清洗、提炼、确认后成为长期知识或记忆。
- Signal 不应该承载大量正文。Signal 只应该提醒 agent “有东西值得注意”，正文应通过权限校验后再读取。
- 不同 agent、用户、项目、客户之间需要信息隔离和权限控制。

参考项目：

- `/root/code/spaceship/3rd/opencontext`
- `/root/code/spaceship/3rd/TencentDB-Agent-Memory`

这两个项目的共同启发是：上下文系统应当独立于 agent runtime，不直接调度 agent，而是提供信息接入、存储、过滤、订阅、检索、沉淀和权限边界。

## 核心判断

Context Center 不是知识库，也不是 Attention Signal。

它是一个独立的信息中枢：

```text
Information Source
  -> Collector
  -> Context Item
  -> Context Store
  -> Cleaning / Distillation
  -> Subscription / Permission / Relevance
  -> Attention Signal
  -> Agent pulls context
  -> Knowledge / Memory
```

模块边界：

- Context Center 负责“信息从哪里来、怎么存、怎么查、怎么过滤、怎么订阅”。
- Attention Signal 负责“哪些变化值得 agent 注意”。
- Knowledge Base 负责“哪些信息已经被沉淀成可复用知识”。
- Permission 负责“谁能看什么、谁能订阅什么、谁能沉淀/分享什么”。
- Agent 负责“自主判断是否读取、处理、忽略、记住或沉淀”。

## 产品概念

对用户不要暴露太多新概念。产品界面可以收敛成：

- 信息源：连接飞书、GitHub、Sentry、网页订阅等。
- 资料库/知识库：用户能看到的资料、文档、导入内容。
- 智能体关注范围：agent 关注哪些项目、群聊、文档、Issue、报警。
- 关联资料：agent 能访问哪些上下文。
- 信号：agent 最近有哪些值得注意的变化。

底层可以有更清晰的工程概念：

- `ContextSource`
- `ContextCollector`
- `ContextItem`
- `ContextSubscription`
- `ContextSignalRule`
- `ContextDistillationJob`
- `ContextACL`

## 核心对象

### ContextSource

信息源。表示某类信息来自哪里。

示例：

- Lark/Feishu IM
- Lark/Feishu Doc
- Meeting Minutes
- GitHub Issue / PR
- Linear
- Sentry
- Web Feed
- Twitter/X
- Blog/RSS
- Agent Session
- Local File
- Code Repository

建议字段：

```text
id
workspace_id
type
name
description
connection_ref
status
created_by
created_at
updated_at
metadata
```

### ContextCollector

采集器。可以独立进程运行，也可以是 server 内置 adapter。

职责：

- 从信息源拉取或接收信息。
- 标准化成 ContextItem。
- 做最小必要的去重、脱敏和敏感度标记。
- 不负责判断这是不是“知识”。
- 不直接唤醒 agent。

Collector 可以有多种形态：

- 外部 daemon：周期性调用 Multigent API 写入 Context Center。
- Webhook receiver：如 GitHub、Sentry。
- IM event adapter：飞书/Lark 长连接事件。
- CLI uploader：本地上传 session、文件、文档。
- Browser/RSS collector：网页、文章、推文、新闻。

### ContextItem

Context Center 的最小信息单位。它可以是原始事件、半结构化内容、文档片段、会话片段、网页快照、PR 更新等。

建议字段：

```text
id
workspace_id
source_id
source_type
source_item_id
source_url
project_id?
agent_worker_id?
author_type?
author_id?
occurred_at
collected_at
title
summary
content_ref
payload
labels
sensitivity
status
dedupe_key
acl_policy_id?
retention_policy
expires_at?
last_used_at?
usage_count
created_at
updated_at
```

说明：

- `payload` 保存结构化原始信息。
- `content_ref` 指向大文本、附件、截图、原文 JSON、Markdown 等对象。
- `labels` 用于筛选，例如 `project=tapnow-mcp-server`、`repo=cc-connect`、`chat_id=xxx`。
- `sensitivity` 类似 OpenContext 的 L1/L2/L3：
  - L1：元信息，低敏。
  - L2：工作内容，需授权。
  - L3：敏感正文、私聊、截图、token、客户资料，显式授权。

### ContextSubscription

订阅关系。定义 agent 或用户关注哪些 ContextItem。

它不是直接把内容注入给 agent，而是决定：

- 哪些 context item 对某个 agent 可见。
- 哪些 context item 会生成 attention signal。
- agent 可按什么过滤器主动拉取。

建议字段：

```text
id
workspace_id
subscriber_type       # agent_worker | user | project | team
subscriber_id
source_ids
label_selectors
max_sensitivity
delivery_mode         # signal_only | searchable | digest | direct
signal_rule_id?
enabled
created_by
created_at
updated_at
```

### ContextSignalRule

把 ContextItem 转成 Attention Signal 的规则。

示例：

- IM 私聊 agent：生成高优先级 direct message signal。
- 群聊 @agent：生成 mention signal。
- 群聊普通消息：默认只入 context，不生成 signal；agent 可以自己拉群聊摘要。
- Sentry P0 报警：生成高优先级 incident signal。
- GitHub issue 新评论：如果关联项目和 agent 订阅匹配，生成 issue update signal。
- 新知识库文档：生成 context updated signal。

建议字段：

```text
id
workspace_id
source_type
match_expression
priority
signal_type
summary_template
include_context_refs
enabled
```

Signal 中只携带摘要和引用：

```json
{
  "type": "im_mention",
  "summary": "Joey 在 MCP 联调群 @mason 反馈 OAuth token 校验失败",
  "context_refs": ["ctx_123", "thread_456"],
  "priority": "high"
}
```

### ContextDistillationJob

清洗、总结和沉淀任务。它把 ContextItem 转成更高阶的知识、记忆或资料。

示例：

- 群聊一周讨论总结。
- 会议纪要提炼成项目决策。
- agent session 提炼成 handoff。
- PR review 记录提炼成 QA checklist。
- 用户反馈沉淀成产品洞察。

建议字段：

```text
id
workspace_id
input_context_ids
output_type             # knowledge_doc | memory | summary | skill | report
output_ref
status
created_by
assigned_agent_id?
started_at?
finished_at?
error?
```

## Collector 协议

Collector 与 Context Center 应当通过稳定 API 解耦。

第一版 API：

```http
POST /api/v1/context/items
POST /api/v1/context/items/batch
GET  /api/v1/context/items
GET  /api/v1/context/items/{id}
POST /api/v1/context/sources
GET  /api/v1/context/sources
```

写入请求：

```json
{
  "sourceId": "src_lark_mcp_group",
  "sourceType": "lark_im",
  "sourceItemId": "message_abc",
  "occurredAt": "2026-08-25T10:00:00Z",
  "title": "Joey 反馈 OAuth token 校验失败",
  "summary": "Joey 在 MCP 联调群反馈 OAuth token 校验失败，@mason 需要确认。",
  "labels": {
    "project": "tapnow-mcp-server",
    "chat_id": "oc_xxx",
    "message_type": "mention"
  },
  "sensitivity": "L2",
  "payload": {
    "sender": "joey",
    "message_type": "text",
    "mentioned_agents": ["mason"]
  },
  "content": "原始消息正文或对象引用"
}
```

Collector 不应该直接写 Signal，也不应该直接写 Knowledge。它最多写 ContextItem，并由 Context Center 根据规则生成 signal 或触发 distillation。

## Agent 访问方式

Agent 通过 `mga context` 主动访问。

建议命令：

```bash
mga context sources
mga context list --source lark_im --project tapnow-mcp-server --since 24h
mga context search "OAuth token 校验失败" --project tapnow-mcp-server
mga context read ctx_123
mga context thread ctx_123
mga context mark-read ctx_123
mga context summarize --source lark_im --since 7d
```

唤醒 prompt 中只注入轻量索引：

```text
你有 3 条新的 attention signals：
1. Joey 在 MCP 联调群 @你反馈 OAuth token 校验失败。context_ref=ctx_123
2. GitHub PR #1852 有新评论。context_ref=ctx_456
3. Sentry 出现 P1 错误。context_ref=ctx_789

如需详情，使用 mga context read <context_ref>。
```

这样不会把大量信息塞进 prompt，也不会让 agent 被迫消费全部信息。

## 与 Attention Signal 的关系

Context Center 与 Attention Signal 解耦。

Context Center：

- 存信息。
- 过滤信息。
- 提供查询。
- 管理订阅。
- 根据规则“建议”生成 signal。

Attention Signal：

- 只表达“需要注意”。
- 持有 `context_refs`。
- 被 heartbeat / wakeup 系统消费。
- 不保存大量正文。

这意味着同一个 ContextItem 可以：

- 只存档，不产生 signal。
- 产生一个 signal。
- 被多个 agent 的不同订阅规则生成多个 signal。
- 后续被提炼成知识库文档。

## 与知识库的关系

知识库是 Context Center 的下游沉淀结果之一，不是 Context Center 本身。

区别：

| 类型 | 例子 | 特点 |
| --- | --- | --- |
| ContextItem | 一条飞书消息、一个 GitHub 评论、一次命令输出 | 原始、可追溯、可能很快过期 |
| Context Summary | 一周群聊摘要、一次会议摘要 | 压缩后的工作上下文 |
| Knowledge Doc | 设计方案、操作手册、决策记录 | 结构化、可复用、应长期保留 |
| Memory | 某人的偏好、某 agent 的边界、项目事实 | 长期事实，影响后续行为 |
| Skill | 可复用操作流程 | 可执行经验 |

知识库可以作为 ContextSource，知识库更新也可以产生 ContextItem 和 Signal。

但 ContextItem 不应该默认变成知识库文档。只有被人工或 agent 确认有价值的信息，才沉淀为知识。

## 权限模型

权限必须贯穿全链路。

需要校验的动作：

- 创建 source。
- collector 写入 item。
- agent 订阅 source。
- agent 读取 item。
- agent 搜索 context。
- agent 把 context 提炼成知识。
- agent 把 private context 分享给团队或项目。

权限维度：

```text
workspace
project
team
user
agent_worker
source
item
sensitivity
operation
```

原则：

- Signal 里不放敏感正文。
- Agent 看到 signal，不代表可以读取全文。
- 读取 `context_ref` 时必须再次校验权限。
- Collector 凭证只代表写入身份，不代表所有 agent 都可读。
- L3 信息默认不可订阅、不可自动沉淀、不可跨项目共享。

## 遗忘与记忆

Context Center 需要生命周期机制，而不是无限累积。

建议状态：

```text
hot       最近且可能相关
warm      一段时间内可检索
distilled 已被提炼
archived  只保留审计/低频检索
expired   已过期/待删除
deleted   已删除
```

重要字段：

```text
retention_policy
expires_at
last_used_at
usage_count
importance
confidence
distilled_to
```

初期可以规则化处理：

- IM 普通消息默认 30-90 天。
- @agent、私聊、流程相关消息保留更久。
- 被引用过的 item 延长保留。
- 被沉淀成知识后，原始 item 可归档。
- L3 信息短 TTL，默认不进入摘要。

## 第一版落地范围

第一版目标不是做完整“智能记忆”，而是先搭好解耦底座。

### Backend

- 新增 `internal/contextcenter` 模块。
- 新增 ContextSource / ContextItem / ContextSubscription / ContextSignalRule 实体。
- 提供 Context API。
- 提供 collector PAT 或 service token 写入认证。
- 实现基础 ACL 校验接口，先接现有 workspace / project / agent 权限。
- 支持 item 写入、查询、读取、搜索。
- 支持按 subscription 生成 AttentionSignal。

### CLI

- `mga context list`
- `mga context search`
- `mga context read`
- `mga context mark-read`
- `mga context sources`

### Collector

第一批 collector：

- Lark/Feishu IM collector：消息、@、私聊、群聊、附件元数据。
- Agent session collector：本地 session 文件上传。
- GitHub collector：issue / PR / comment / review。
- Manual file collector：用户 CLI 上传文件/文档。

### Frontend

第一版尽量不暴露复杂概念：

- 信息源列表。
- 资料/上下文列表。
- Agent 详情页：关注范围、关联资料。
- Context item 详情页。
- Subscription 简化配置。

### 不做

- 不做全自动长期记忆抽取。
- 不做复杂重要性模型。
- 不做全量网页爬虫。
- 不做跨 workspace 共享。
- 不把所有 context 自动塞进 agent prompt。

## 后续演进

### 第二阶段：清洗与沉淀

- 支持 distillation job。
- 支持把一组 ContextItem 生成知识库文档。
- 支持 agent 提议沉淀，human 审核后发布。
- 支持会议纪要、群聊摘要、PR 评审总结。

### 第三阶段：记忆与遗忘

- 重要性评分。
- 使用频率提升权重。
- 长期没人用自动降权。
- 私有记忆、团队记忆、项目记忆分层。
- Agent profile 自动更新但需要审计。

### 第四阶段：开放信息网络

- Web/RSS/Twitter/Newsletter collectors。
- 竞品动态、行业新闻、用户声音自动进入 Context Store。
- Agent 自主订阅外部信息源。
- 与营销、产品、战略 agent 结合。

## 与现有系统的集成顺序

1. Context Center 独立落地。
2. IM / GitHub / Session collector 写入 Context Center。
3. `mga context` 让 agent 可按权限读取。
4. Subscription 生成 Attention Signal。
5. Knowledge Base 支持从 ContextItem 创建文档。
6. 权限系统细化到 source/item/subscription。
7. 前端把复杂配置收敛到信息源、关注范围、关联资料。

## 设计原则

- 解耦：Context Center 不直接等于知识库，也不直接等于 signal。
- 可追溯：任何知识都能回到原始 ContextItem。
- 最小注入：prompt 只注入 signal 和索引，不注入大量正文。
- 主体性：agent 自主判断是否读取、处理、忽略、沉淀。
- 权限优先：任何读取和沉淀都必须经过权限校验。
- 本地/客户可控：collector 可以独立部署，敏感数据不必默认进入 SaaS。
- 渐进增强：先存、查、订阅，再做智能清洗和记忆。

## 一句话总结

OpenContext Center 是 Multigent 的信息底座：它让分散在内外部系统里的信息被安全采集、按权限存储、按需订阅、轻量提醒、主动检索，并最终沉淀成知识和记忆。Agent 不再靠一次性 prompt 背下世界，而是像人一样拥有可关注、可查阅、可遗忘、可沉淀的信息环境。
