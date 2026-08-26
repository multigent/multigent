# Agent Worker 与 Attention Signal 重构设计

本文记录 Multigent 2.x 方向的一次核心架构收敛：保留现有 `task / workflow / task template` 概念，不引入新的 `work item` 产品概念；重构重点只放在 `Agent Worker`、`Attention Signal` 和 IM 协作渠道处理上。

## 背景

Multigent 当前的很多能力已经证明了方向是对的：

- agent 可以被雇佣到项目里，承担任务。
- task 可以绑定 workflow，按节点在人和 agent 之间流转。
- scheduler / heartbeat 可以让 agent 持续醒来工作。
- Feishu / Lark 等 IM 渠道可以让用户与 agent 对话，也可以承载卡片交互。

但我们在真实客户和自用场景里逐渐遇到一个更底层的问题：系统还隐含地把 agent 当成“项目里的一个执行工具”，而不是“一个可以跨项目、跨渠道、长期工作的协作对象”。

典型表现：

- agent 被强绑定在某个 project member 上，导致一个 agent 难以自然地同时参与多个项目。
- IM 单聊、群聊、Web 对话、workflow 节点执行之间的 session 和上下文关系不清晰。
- 外部消息容易被设计成“来了就触发 agent”的 webhook，而不是 agent 自己可以选择关注和处理的环境信息。
- 用户希望 agent 像同事一样被 @、被私聊、主动汇报、定时检查问题，但现有抽象更像“消息触发器 + 执行器”。

这会限制 Multigent 的上限。我们真正想做的是 human-agent collaboration infrastructure，而不是一个 workflow automation 工具。

## 设计原则

### 1. Agent 是协作对象，不是工具

agent 应该像人一样拥有相对稳定的身份、职责、记忆、协作渠道和工作节奏。

他可以：

- 持续 wakeup。
- 主动查看任务、消息、外部系统状态。
- 判断哪些事情要立即处理，哪些可以忽略或延后。
- 同时参与多个项目。
- 在 IM 里被人私聊或 @，也可以主动找人沟通。
- 在 workflow 节点里承担阶段性交付，但不被 workflow 限制成一个“节点函数”。

### 2. 流程仍然重要，但流程是弱规范

workflow 不是 n8n 式的硬编码自动化流。它是人和 agent 协作时共同遵守的阶段规范。

所以这次不改 `task / workflow / task template`：

- `task` 仍然是一件具体要推进的工作。
- `task template` 仍然是创建任务的模板。
- `workflow` 仍然描述任务的协作流转和人工节点。

重构不是为了发明更多概念，而是让 agent 在这些已有概念之上更像一个长期工作的同事。

### 3. Attention 不是 Trigger

`Attention Signal` 不是“事件来了马上触发 agent 执行”的 trigger。

外部世界的信息太多，不能都塞进 agent 的上下文，也不能每条消息都唤醒 agent。正确做法是分层：

- 直接私聊 agent、在群里 @agent、明确分配给 agent 的任务、当前 workflow 节点需要 agent 处理，这些是强 attention。
- 普通群聊消息、GitHub/Linear/Sentry 的普通更新、外部系统流水日志，这些是 ambient context，agent 可以按需查询。

系统只负责把值得注意的信号放到 agent 的注意力队列里；是否处理、怎么处理、何时处理，应尽量交给 agent 自主判断。

### 4. 通用能力优先，避免秘书系统特异化

我们不做一个特殊的“秘书 agent 系统”。任何 agent 都可以拥有秘书能力：

- 定时查看任务和流程状态。
- 发现需要人类判断的问题。
- 通过 Feishu / Lark / Slack 等渠道发消息或卡片。
- 拿到用户授权后代为提交 workflow 决策。

这些应该是通用工具和权限能力，而不是某个特定 agent 的硬编码逻辑。

### 5. 项目上下文仍然是执行边界

这次重构不是否定“agent 在项目下干活”。

恰恰相反，agent 真正执行任务时仍然必须落在明确的项目上下文里。项目提供了它干活所需的边界：

- 项目目标和背景。
- 项目代码、文档、知识库和文件。
- 项目成员与责任关系。
- 项目任务、流程和任务模板。
- 项目级权限、外部工具授权和运行约束。

之前“项目下雇佣 agent”的设计抓住了这个事实，所以方向不是错的。需要修正的是：不要把“agent 本体”和“agent 在某个项目里的岗位”混成同一个对象。

更合理的理解是：

```text
Agent Worker = 这个 agent 是谁
Project Membership = 这个 agent 在某个项目里以什么身份工作
Task / Workflow = 这个 agent 在该项目里推进哪件具体工作
```

每次实际运行都应绑定一个项目上下文：

```text
run agent_worker=nova
project=customer-mcp-server
membership=project-manager
task=t-xxx
```

这样既保留项目上下文隔离，又允许 agent 拥有跨项目的长期身份和经验。

### 6. 第一版不自研上下文压缩

Agent Worker 的 primary session 会变长，这是事实，但第一版先不在 Multigent 里发明一套压缩机制。

原因：

- Claude Code、Codex、Cursor 等 runtime 自己已经在演进上下文管理、压缩和恢复能力。
- 过早在平台层压缩 session，容易切断 agent 的原生能力，反而引入不可控的信息损失。
- 当前更关键的是先把身份、项目边界、注意力队列和 IM 交互模型理顺。

因此第一版明确不做：

- 平台侧 session 自动压缩。
- 平台侧替 agent 总结整段历史后替换原生上下文。
- 把不同 runtime 的历史统一转成一套自研 memory 格式。

所以第一版只做必要的结构化沉淀：

- task receipt。
- handoff。
- daily journal。
- 关键文档进入知识库或文件。

这些沉淀是给 agent 参考的长期材料，不是平台强行压缩 primary session 的替代上下文。

后续如果 primary session 明显影响质量，再引入可观察、可回放、可人工审阅的 memory checkpoint，而不是黑盒摘要。

明确决策：

- 第一版不接管 Claude Code / Codex / Cursor 等 runtime 的压缩机制。
- runtime 原生支持的压缩、恢复、session 管理继续由 runtime 自己处理。
- Multigent 只负责把结构化事实、项目边界、任务状态、外部消息和长期文档整理好，供 agent 主动读取。
- 平台不能在 agent 不知情的情况下替换、摘要或裁剪原生 session。即使后续引入 memory checkpoint，也必须可观察、可回放、可人工审阅，并且作为补充材料进入知识库或文件系统，而不是伪装成完整历史。

这也意味着第一版不把“压缩”作为 2.x 重构的阻塞项。Agent Worker 的身份、项目 membership、attention 队列和运行边界先理顺；Claude Code、Codex、Cursor 等 runtime 自己已有的压缩、resume、session 管理能力继续原样工作。平台只做事实和索引的整理，不替 runtime 做黑盒记忆裁剪。

### 7. 已确认的一版硬约束

当前阶段先按以下约束推进实现，避免把系统重新做成一套复杂的 workflow trigger 平台：

1. **不做平台侧 session 压缩。**
   第一版不接管 Claude Code / Codex / Cursor 等 runtime 的压缩和上下文整理机制。平台只负责提供清晰的身份、项目边界、任务状态、文档和消息索引。

2. **项目上下文在 agent 加入项目时进入 system prompt / context build。**
   Project Membership 不是运行时临时拼接的一句提示。agent 加入某项目后，该项目里的职责、权限边界、项目背景和协作规则应进入稳定的上下文生成链路。真正处理项目任务时，run 仍然必须带明确的 project context。

   这里的关键不是“每次任务临时塞一段项目说明”，而是 agent 加入项目后，它在该项目里的身份就成为稳定上下文的一部分。这样它醒来处理该项目任务时，天然知道自己在这个项目里是谁、能做什么、不能做什么、应该遵守哪些协作规则。

3. **必须有 seen / cursor 机制防止重复消费。**
   `seen` 只表示“这条 signal 已经被注入 wakeup 或被 agent 主动读取过”，不表示事情完成。完成要进入 `handled`，忽略要进入 `ignored`。外部消息源还必须有 cursor，否则 agent 会在每次 wakeup 里重复看到和重复处理同一批消息。这个机制是 2.x 的基础能力，不能只靠 prompt 让 agent 自觉不要重复消费。

   `seen` 解决的是 delivery/read 问题，不解决业务处理问题。它的价值是让系统能审计“这条消息是否已经展示给 agent”，并避免下一次 wakeup 继续把同一批消息当作未读强信号注入。agent 是否真正处理了它，必须由 `handling / handled / ignored`、task 状态或 workflow 状态表达。

4. **IM 是否唤醒属于心跳策略。**
   私聊、@、卡片点击、任务分配、workflow 到达 agent 节点后是否立即唤醒，不由 Feishu / Lark connector 写死，而由 Agent Worker 的 schedule / heartbeat policy 配置。connector 只记录事件和写入 attention。也就是说，“收到 IM 通知是否唤醒”和“收到任务是否唤醒”是同一种工作节奏配置。

   这不是外部工具配置，也不是 connector 的业务逻辑。IM connector 只负责把世界发生的事写成可去重、可审计的 signal；Agent Worker 的 heartbeat policy 决定这些 signal 是否能打断睡眠节奏，还是等下一次周期性 wakeup 统一处理。

   这里有一个必须守住的边界：不能让 `runAgentForIMEvent` 这类 IM 专属 runner 成为主路径。IM 私聊、群聊 @、卡片点击都只是 Attention Signal 的来源之一；未来 Sentry 报错、GitHub 事件、Twitter / X 趋势、竞品动态、用户反馈、灵感 feed 也应该进入同一套 signal 生命周期。外部渠道 handler 只负责记录事实、更新 source cursor、生成 signal；如果策略允许即时响应，也应该调用通用 wakeup，而不是在 IM handler 里拼专属 prompt、直接执行 agent、再专门回 IM。

   这意味着：

   - `source_kind` 可以是 `im_message`、`im_card_action`、`github_event`、`sentry_alert`、`idea_feed` 等。
   - `reason` 可以是 `im_direct_message`、`im_mention`、`card_action`、`error_alert`、`trend_signal`、`workflow_waiting_review` 等。
   - 生命周期永远是 `pending -> seen -> handling -> handled/ignored/expired`。
   - 是否提前唤醒永远是 Agent Worker 的 wake policy，而不是某个 connector 的固定业务逻辑。
   - agent 回复不应由平台替它固定完成；平台提供 IM / workflow / docs / tasks 等工具，agent 自主决定是否回复、回复给谁、是否更新流程、是否标记 signal 完成。

   普通群聊消息也不能简单等同于 attention。更合理的分层是：

   - 私聊和明确 @agent：生成高优先级 attention。
   - 普通群聊消息：进入 channel feed / cursor，默认不打断 agent；agent 可以在 wakeup 时按需拉取最近上下文。
   - 群聊里没有 @ 但包含明确责任、紧急异常或被策略识别为强相关的内容，后续可以由可配置规则或 agent 自己的观察策略提升为 attention。

   因此，2.x 的 IM 处理目标不是把 agent 做成一个“飞书机器人”，而是让飞书、Lark、Slack 等协作渠道成为 agent 感知世界和与人协作的多个入口。

5. **所有运行与写入必须带明确 workspace。**
   Agent Worker 是 workspace 级主体，Attention、Project Membership、Runtime Token、IM Channel 和 Schedule 都不能依赖“当前服务进程刚好切到哪个 workspace”。Web、本地 CLI、SaaS trusted proxy、client token 都必须把 workspace identity 传清楚；服务端进入 handler 前必须切到请求指定的 workspace，并在权限校验后再读写对应数据。否则 2.x 后同名 agent、跨 workspace membership、SaaS 多租户都会出现隐蔽串数据风险。

Owner 于 2026-08-21 补充确认：

- 压缩机制第一版暂不处理，继续让 Claude Code、Codex、Cursor 等 runtime 自己管理。
- 项目上下文应该在 agent 加入项目时进入稳定的 context build / system prompt 链路，而不是每次临时提示。
- Attention Signal 必须有可审计的 seen / cursor 机制，否则 agent 会重复拉取、重复消费、重复回复。
- IM 通知是否唤醒 agent，应作为心跳配置项处理，和“收到消息唤醒”“收到任务唤醒”属于同一类工作节奏策略。
- SaaS / 多 workspace 场景下，所有 Agent Worker、Attention、Schedule 和 Runtime 请求必须显式落到请求所属 workspace，不能默认使用服务当前 root。

这些约束落到实现时，有几个判定标准：

- 任何代码如果在 IM connector 里直接决定“收到消息就运行 agent”，而不是写入 attention 后交给 Agent Worker 的 schedule policy 判断，都应视为回退到 trigger 模型。
- 任何代码如果只靠 prompt 让 agent 自觉“不要重复处理消息”，而没有 `dedupe_key`、`seen` 状态或 source cursor，都应视为不完整实现。
- `seen` 不能被业务方当成“已处理”。它只是 delivery/read cursor，表示平台已经把 signal 暴露给 agent；是否行动、如何行动、是否完成，必须由 `handling / handled / ignored` 或相关 task/workflow 状态表达。
- 项目 membership prompt、项目基础 prompt 和 agent 绑定的 context material 必须进入稳定的 context build。任务、workflow 节点输入和大文件材料则按需引用，不把全部内容塞进 primary session。

## 保留不变的概念

### Task

`task` 仍然是产品里的核心工作对象。

它表示一件已经被创建、可分配、可跟踪、可完成的工作。任务可以来自：

- 用户手动创建。
- task template 创建。
- agent 拆解创建。
- 外部系统同步后转化。

### Task Template

`task template` 仍然用于复用常见任务结构，包括默认描述、输入输出、负责人、绑定 workflow 等。

### Workflow

`workflow` 仍然用于描述 task 的阶段流转。

它不是替代 agent 的思考，而是提供协作边界：

- 当前阶段要交付什么。
- 什么时候需要 human review。
- 哪些输出决定下一步走向。
- 哪些节点允许打回或继续。

### Project Member

项目成员仍然存在，而且仍然是 agent 真正进入项目工作的入口。

调整点只是：项目成员不再等价于 agent 本体。项目成员应该更像“某个 worker 在某个项目里的成员身份”。

也就是说，用户在项目成员页看到的仍然可以是一个 agent，但底层含义变成：

```text
某个 Agent Worker 加入了这个项目，并在这个项目里承担某个角色。
```

## 新的核心抽象

### Agent Worker

`Agent Worker` 是 workspace 级别的 agent 主体。

它描述的是“这个 agent 是谁”，而不是“这个 agent 在某个项目里做什么”。

Agent Worker 不直接替代项目 agent，也不意味着 agent 脱离项目上下文干活。它只是把长期身份抽出来：

- 这个 agent 的名字、画像和长期职责。
- 默认模型和默认运行节点。
- 通用技能和协作渠道。
- 跨项目沉淀的经验和记忆。

当 Agent Worker 进入具体项目时，仍然通过 Project Membership 获得项目身份和项目上下文。

Project Membership 不是运行时临时拼出来的一段说明，而应该在 agent 加入项目时就合并进它的系统上下文生成链路里。也就是说，agent 的 system prompt / runtime context 应稳定包含：

- 该 agent 在这个项目里的角色和职责。
- 该项目的目标、背景、边界和主要协作者。
- 该项目允许它使用的技能、外部工具、运行节点和权限。
- 该项目里它应该关注的任务、流程和沟通规则。

这样 agent 每次进入项目工作时，不需要重新猜“我是谁、这个项目是什么、我在这里能做什么”。但这里合入的是项目成员上下文和项目边界，不是把所有项目资料无差别塞进 primary session。

建议字段：

```yaml
id: agent_worker_id
name: Nova
profile:
  title: CustomerCo Agent 项目管理者
  description: 负责理解 customer-agent 相关项目进展、分配任务、跟进风险。
default_model:
  provider: codex
  model: gpt-5.5
default_runtime:
  node_id: local-node-1
skills:
  - github
  - lark
  - customer-agent-debug
channels:
  - provider: lark
    connection_id: lark-main
memory_policy:
  project_context: readable_when_member
  shared_docs: readable_when_granted
attention_policy:
  direct_message: strong
  mention: strong
  assigned_task: strong
  ambient_channel_message: queryable
```

Agent Worker 可以同时加入多个项目：

```text
Agent Worker: Nova
Project memberships:
- customer-cli / 项目管理者
- customer-mcp-server / 项目管理者
- customer-connectors / 项目管理者
```

这样 Nova 不是三个项目里复制出来的三个 agent，而是一个跨项目工作的 agent。

但 Nova 每次处理具体任务时，仍然在某一个项目上下文里运行。例如处理 `customer-mcp-server` 的任务时，它默认读取的是该项目的 prompt、知识库、任务、流程、成员和授权，而不是把所有项目上下文混在一起。

Agent Worker 也应该拥有自己的工作节奏，而不是每个项目复制一套心跳：

```yaml
schedule:
  interval: 2h
  active_hours: "09:00-22:00"
  active_days: weekdays
  max_tasks_per_cycle: 3
  max_cycle_duration: 45m
  max_concurrent_runs: 1
```

这表示“Nova 这个同事什么时候醒来工作”，不表示“Nova 固定只处理某一个项目”。

### Project Membership

`Project Membership` 表示一个人类或 agent worker 在某个项目里的身份。

它解决的问题是：同一个 agent 在不同项目里可能有不同职责、权限和上下文。

Project Membership 是执行时的关键边界。一个 agent worker 可以长期存在于 workspace，但只有通过 membership 进入某个项目后，才应该获得该项目的上下文和权限。

建议字段：

```yaml
project_id: customer-mcp-server
member_type: agent_worker
worker_id: nova
role: project-manager
permissions:
  - task.read
  - task.write
  - workflow.read
  - inbox.send
project_prompt: |
  在 customer-mcp-server 项目中，你负责跟进 MCP Server 的需求、开发、测试和上线风险。
auto_pick_tasks: true
attention_enabled: true
priority_weight: 1.0
```

对 UI 来说，用户仍然可以在项目成员页看到 agent；只是底层不再把 agent 存成项目私有主体。

对运行时来说，调度 agent 时必须解析出：

```text
agent_worker_id
project_id
project_membership_id
task_id / workflow_run_id
```

缺少 project context 的普通聊天可以存在，但一旦要读项目资料、操作任务、推进 workflow 或使用项目级外部工具，就必须进入明确的项目 membership。

Project Membership 可以控制该 agent 在项目里的自动化程度：

- `auto_pick_tasks`: 是否允许它在 wakeup 时自动接该项目任务。
- `attention_enabled`: 是否把该项目里的相关信号送入它的 attention。
- `priority_weight`: 同一 agent 跨多个项目时，该项目的相对优先级。
- `project_prompt`: 该 agent 在这个项目里的特定职责和上下文。

这样项目仍然能控制边界，但 agent 的工作节奏不再被项目复制。

项目上下文的注入方式也要明确：当 agent 加入某个项目时，该项目 membership 的上下文应合并到 agent 的 system prompt / context build 中。

也就是说，agent 醒来时可以知道：

- 自己加入了哪些项目。
- 每个项目里自己的角色和职责。
- 每个项目的基础背景、目标、规则。
- 哪些项目允许自己自动接活。

但项目的全量文件、知识库和任务详情不应该全部塞进 prompt。需要具体处理某项目时，再通过项目上下文和工具按需读取。

实现约束：

- agent 的基础 system prompt 来自 Agent Worker。
- 每个 project membership 的项目角色、职责、项目背景摘要和权限边界，应合并进 system prompt / context build。
- 合并的是“我在这个项目里是谁、能做什么、应关注什么”，不是全量项目资料。
- 具体任务运行时，再追加该任务、workflow 节点、相关文档和文件引用。
- 如果 agent 没有加入某项目，即使知道项目名，也不能直接读写该项目资源。

### Attention Signal

`Attention Signal` 是给 agent 的注意力信号。

它不是 trigger，不意味着必须马上运行，也不意味着必须塞进 prompt。它只是告诉 agent：“这里有一件你可能应该关注的事”。

建议字段：

```yaml
id: sig_xxx
workspace_id: ws_xxx
agent_worker_id: nova
source:
  kind: lark_message
  channel_binding_id: lark-main
  chat_type: group
  chat_id: oc_xxx
  message_id: om_xxx
actor:
  type: user
  user_id: glenn
reason: mention
priority: normal
summary: Glenn 在 CustomerCo 项目群里 @Nova，询问 MCP Server 联调状态。
refs:
  project_id: customer-mcp-server
  task_id: t_xxx
status: pending
created_at: ...
expires_at: ...
```

强 attention 的来源：

- 用户私聊 agent。
- 群聊里 @agent。
- task 指派给 agent。
- workflow 当前节点轮到 agent。
- human 委托 agent 处理某个待决事项。
- agent 被明确要求跟进某个外部 issue / PR / incident。

弱信息不进入 attention，只进入可查询的外部上下文：

- 普通群聊消息。
- 未 @agent 的讨论。
- GitHub 普通 timeline。
- Sentry 普通告警流。
- Linear 普通状态变化。

agent 可以通过工具自行查询这些环境信息，但系统不强迫它每次都读。

## IM 渠道处理重构

### 当前问题

IM 现在已经能支持 Feishu / Lark 绑定、用户单聊、群聊绑定、卡片点击回调等能力，但抽象仍然偏项目 agent：

```text
project_id + agent_id + channel_binding
```

这会导致几个问题：

- 一个 agent 跨项目工作时，IM 身份应该跟 agent worker 绑定，而不是跟某个项目绑定。
- 群聊里 @agent 时，应该进入这个 agent worker 的 attention，而不是硬触发某个项目 session。
- IM、task、workflow、wakeup 不应该天然拆成多套 session，否则 agent 会被拆成多个人格。

### 目标行为

#### 用户私聊 agent

用户私聊 Nova：

1. IM connector 收到消息。
2. 解析到 channel 绑定的是 `agent_worker_id=nova`。
3. 创建 `AttentionSignal(reason=direct_message)`。
4. 如果策略允许即时响应，可以唤醒 Nova；否则等待下一次 wakeup。
5. 消息进入 Nova 的 primary work session，来源和回复目标作为 interaction metadata 保存。

#### 群聊 @agent

群里 @Nova：

1. IM connector 收到群消息。
2. 判断消息显式 mention 了 Nova。
3. 创建 `AttentionSignal(reason=mention, source.chat_id=...)`。
4. Nova 可以选择回复群，也可以私聊某个人，也可以创建 task。

未 @Nova 的普通群消息：

- 默认只作为 channel history，可被查询。
- 不创建强 attention。
- 不即时唤醒。

#### agent 主动发消息

agent 主动联系用户或群：

1. agent 通过通用消息工具选择接收者。
2. 系统根据用户绑定或群聊 target 找到 provider identity。
3. 发送普通 markdown 消息或交互卡片。
4. 如果是卡片，点击回调也作为 interaction event 进入 agent 的 primary work session。

### Session 策略

一个 Agent Worker 默认只有一个主工作 session：

```text
agent_worker.primary_session_id
```

这个 primary session 表示 agent 的连续工作意识。默认情况下，下面这些输入都进入同一个 primary session：

- schedule wakeup。
- 用户 Web 对话。
- IM 私聊。
- 群聊 @agent。
- task 分配。
- workflow 到达 agent 负责节点。
- 卡片点击回调。
- agent 自己定时检查外部系统后发现的事项。

项目、任务、IM 渠道不是默认 session 边界。它们是 primary session 里的输入来源和操作上下文。

更准确的边界是：

```text
Session 负责 agent 的连续意识。
Project context 负责资源访问边界。
Task / Workflow 负责工作对象和协作状态。
Attention Signal 负责告诉 agent 哪些事情值得关注。
Interaction metadata 负责记录消息来自哪里、将来应该回复到哪里。
```

因此，同一个 agent 可以在一个 primary session 里先处理 `customer-mcp-server` 的任务，再回复 Lark 群里关于 `customer-cli` 的问题。它每次访问项目资源时必须带 project context，但不因为项目不同就默认切 session。

### Child Session

多 session 是 agent 的并发和隔离能力，不是系统默认切分规则。

只有当 agent 主动判断需要并发或隔离时，才创建 child session：

- 长任务需要后台执行，不阻塞 primary session。
- 两个项目任务可以并发推进。
- 某个实验性操作可能污染上下文。
- agent 需要派生一个子 agent 做调研，然后把结果汇总回来。

child session 必须绑定：

```text
parent_session_id
agent_worker_id
project_id
task_id / workflow_run_id
purpose
```

child session 的结果必须回写 primary session，形成 handoff / summary / task receipt。这样 agent 仍然只有一个主体，只是具备可控的并发能力。

## 权限模型

重构后，权限判断不能只看 agent，也不能只看渠道。

原则：

- Agent Worker 能看到什么，取决于它在 workspace 和项目里的 membership。
- 用户通过 IM 委托 agent 做事时，agent 代表用户操作，仍然要校验该用户是否有权限。
- 卡片点击或文本选择只是输入，不直接越权改变 workflow。
- 对 workflow 的操作仍然走现有 task/workflow 权限校验。

委托能力可以逐步增强：

第一版：

- 用户在 IM 里明确回复或点击卡片。
- 系统生成短期 delegation token。
- agent 可以在有效期内代表该用户提交对应 workflow 决策。
- 后端校验“被代表用户”是否是当前节点允许审批的人。

后续版本：

- delegation token 可限制到 task / workflow / action / time window。
- 支持用户委托 agent 批量处理同类事项。
- 支持撤销委托。

## 数据结构调整建议

### 新增 agent_workers

```sql
agent_workers (
  id,
  workspace_id,
  name,
  display_name,
  description,
  default_model_account_id,
  default_runtime_node_id,
  schedule_json,
  attention_policy_json,
  max_concurrent_runs,
  status,
  created_at,
  updated_at
)
```

### 新增 project_memberships

```sql
project_memberships (
  id,
  workspace_id,
  project_id,
  member_type, -- user | agent_worker
  member_id,
  role,
  prompt,
  permissions_json,
  auto_pick_tasks,
  attention_enabled,
  priority_weight,
  created_at,
  updated_at
)
```

### 新增 attention_signals

```sql
attention_signals (
  id,
  workspace_id,
  agent_worker_id,
  dedupe_key,
  source_kind,
  source_id,
  reason,
  priority,
  summary,
  refs_json,
  result_ref,
  status, -- pending | seen | handling | handled | ignored | expired
  created_at,
  seen_at,
  handling_at,
  expires_at,
  handled_at
)
```

### 调整 agent_channel_bindings

从：

```text
workspace_id + project_id + agent_id
```

调整为：

```text
workspace_id + agent_worker_id
```

项目维度不应该是 IM 渠道身份的主体。项目只作为消息上下文或权限范围出现。

### 保留 task / workflow / task_template

不新增 WorkItem。

但 task 的 assignee 存储需要一次性迁到新主体模型：

```text
assignee_type: user | agent_worker
assignee_id: ...
assignee_membership_id: ...
```

2.x 不再保留旧的 `project/agent` 作为任务负责人主键。迁移脚本负责把旧字段转成 `agent_worker_id + project_membership_id`；迁移后运行时代码只读新字段。

## Schedule 与跨项目执行

### 心跳属于 Agent Worker

旧模型里，heartbeat 通常挂在：

```text
project_id + agent_id
```

这在“每个项目都有一份 agent”时成立，但当 agent 被提升为 workspace 级主体后，这个模型会变得别扭。

更合理的是：

```text
Agent Worker Schedule
  决定这个 agent 什么时候醒、一次看多少信息、能并发多少、如何做优先级。

Project Task Execution
  决定这次 run 进入哪个项目、处理哪个 task、使用哪个项目上下文。
```

这更接近人的工作方式。人不是“每个项目复制一个自己，然后每个项目各自有一个闹钟”；人是“我这个人醒来，看我负责的所有项目，再决定先处理什么”。

### Wakeup 时先做跨项目 triage

Agent Worker 醒来后，第一步不应该直接跑某个项目任务，而应该先做跨项目 attention triage。

它应该看到：

- 分配给自己的 pending / running / blocked tasks。
- 到自己负责节点的 workflow runs。
- 私聊和 @自己的 IM attention。
- inbox 未读消息。
- 自己参与项目的关键状态摘要。

然后决定：

- 现在处理哪个项目。
- 是否需要回复人。
- 是否需要创建或更新任务。
- 是否需要进入某个 workflow 节点。
- 是否需要延后低优先级事项。
- 是否需要启动并发 run。

### 项目仍然控制工作范围

Agent Worker 的 schedule 是全局工作节奏，但项目 membership 决定这个 agent 在项目里能做什么。

示例：

```yaml
worker: nova
schedule:
  interval: 2h
scope:
  projects:
    customer-cli:
      auto_pick_tasks: true
      attention_enabled: true
      priority_weight: 1.0
    customer-mcp-server:
      auto_pick_tasks: true
      attention_enabled: true
      priority_weight: 1.5
    customer-connectors:
      auto_pick_tasks: false
      attention_enabled: true
      priority_weight: 0.8
```

这里 Nova 会定时醒来查看三个项目，但只会自动接 `customer-cli` 和 `customer-mcp-server` 的任务；`customer-connectors` 的信息可以提醒它，但不会自动接活。

### 并发用 Child Session 隔离

同一个 Agent Worker 可以具备并发能力，但并发不是默认行为。

第一版建议保守：

- 默认 `max_concurrent_runs = 1`。
- 所有 wakeup / IM / task / workflow 默认进入 primary session。
- 一个 wakeup cycle 可以在 primary session 里顺序处理多个项目事项。
- 需要并发时，由 agent 主动创建 child session。
- 每个 child session 必须绑定明确的 `project_id / task_id / purpose`。
- child session 完成后必须把结果汇总回 primary session。

示例：

```text
worker=nova
primary-session=s-main
child-run-1: project=customer-mcp-server task=t-001 purpose=开发联调
child-run-2: project=customer-connectors task=t-002 purpose=插件调研
```

这样可以保留 agent 的主体性，同时让并发成为 agent 可自主使用的能力，而不是系统把它默认拆成多个上下文人格。

### 旧项目级心跳迁移

旧配置：

```text
project=customer-cli
agent=nova
heartbeat=2h
```

迁移后：

```text
worker=nova
schedule.interval=2h
membership[customer-cli].auto_pick_tasks=true
membership[customer-cli].attention_enabled=true
```

如果同一个旧 agent 在多个项目里都有 heartbeat：

- interval / active hours 一致时，可以合并到同一个 worker schedule。
- 不一致时，迁移报告里提示人工确认。
- 不保留 project-level heartbeat 兼容逻辑；需要差异化时，迁移成 membership policy 或明确拆成不同 Agent Worker。

## Wakeup 模型

agent wakeup 时，不应该把所有外部事件塞进 prompt。

建议 wakeup 注入：

1. 当前未完成 task。
2. 当前强 attention signals 摘要。
3. 未读 inbox 摘要。
4. 当前 project memberships 摘要。
5. 可用查询工具列表。

示例：

```text
你有 3 个强 attention:
1. Glenn 在 CustomerCo 项目群 @你询问 MCP Server 状态。
2. task t-123 当前分配给你，处于开发联调阶段。
3. workflow t-456 到达你负责的技术方案节点。

你也可以按需查询:
- 最近 24h CustomerCo 项目群消息
- GitHub PR/Issue 更新
- Linear/Sentry 状态
```

这样 agent 有注意力，但仍然保留自主选择。

### 即时唤醒策略

IM 私聊、群聊 @、任务分配、workflow 到达 agent 节点后，是否立即唤醒 agent，应作为 Agent Worker 工作节奏的一部分配置，而不是 IM connector 写死。

建议放在 schedule / heartbeat 设置里，和“收到任务是否唤醒”“收到 inbox 是否唤醒”属于同一类工作节奏配置：

```yaml
wakeup_triggers:
  im_direct_message: true
  im_mention: true
  task_assigned: true
  workflow_step_assigned: true
  card_action: true
  ambient_channel_message: false
```

含义：

- `true`: 生成 attention 后可以立即唤醒 agent。
- `false`: 只进入 attention 队列，等下一次周期性 wakeup 处理。

这样可以保持 agent 的主体性：系统只是把事情放到它的注意力里，并按它的工作节奏决定是否马上醒来，不把 IM 消息硬编码成 webhook trigger。

第一版可以保留轻量系统反馈，例如“已收到”，但这只是产品体验反馈，不代表 agent 已经完整处理。

注意：这里的 `wakeup_triggers` 是心跳/工作节奏配置，不是外部工具配置。

也就是说：

- Feishu / Lark connector 只负责接收消息、保存事件、写 attention。
- 是否因为私聊或 @ 立即唤醒，由 Agent Worker 的 schedule / heartbeat policy 决定。
- 用户可以配置“收到 IM 私聊立即唤醒”“收到 IM 只进入下次定时 wakeup”“任务分配立即唤醒”等策略。
- 普通群聊消息默认不唤醒，但 agent 可以在 wakeup 时按需读取群聊历史。

也就是说，`im_direct_message`、`im_mention`、`task_assigned`、`workflow_step_assigned`、`card_action` 这些能力属于心跳配置的一部分，而不是 connector 的固定行为。connector 不决定 agent 是否马上工作，只把值得关注的信息写入 attention 系统。

这里要避免两个极端：

- 不要把 IM 消息做成外部工具里的固定 webhook trigger，一来消息量不可控，二来会把 agent 重新降级成被动 handler。
- 也不要让私聊和 @ 完全只能等下一次周期性 wakeup，否则用户会感觉 agent 不像同事，响应体验太差。

所以配置层要表达的是“哪些强 attention 可以打断 agent 的睡眠节奏”，而不是“哪些事件直接调用哪个处理函数”。

### Attention 生命周期

Attention Signal 必须有明确生命周期，否则会变成新的消息垃圾堆，或者导致 agent 重复消费同一条消息。

状态建议：

```text
pending  -> 新建，等待 agent 看到
seen     -> 已注入某次 wakeup / 已被 agent 明确读取
handling -> agent 已决定处理，避免并发重复处理
handled  -> 已处理完成
ignored  -> agent 明确忽略或判断无需处理
expired  -> 超过有效期未处理
```

关键规则：

- 同一外部事件必须有 `dedupe_key`，例如 `provider:message_id:action_id`。
- 写入 attention 前先按 `workspace_id + agent_worker_id + dedupe_key` 去重。
- attention 被注入 prompt 或被 agent 工具读取时，标记为 `seen`。
- agent 决定处理时，先原子标记为 `handling`。
- agent 完成后调用工具标记为 `handled`，并可写入 `result_ref`。
- agent 判断无需处理时标记为 `ignored`，最好写明简短原因。
- 超时未处理的 signal 由后台任务标记为 `expired`。

`handled / ignored / expired` 不应立即物理删除。它们需要保留一段时间用于审计和调试。

`handled / ignored / expired` 是终态，不允许被后续 `seen / handling` 写回覆盖，避免重复读取或重复回调把已完成事项重新打开。

这里的 `seen` 需要非常明确：它表示系统已经把这条 signal 暴露给某次 wakeup 或 agent 主动查询结果，不代表事情已经完成。否则 agent 很容易在下一次 wakeup 里重复看到同一批消息，造成重复回复、重复流转、重复建任务。

Attention 与 task 的关系：

- attention 可以不创建 task，只是一次消息或提醒。
- 如果 agent 因 attention 创建了 task，应在 `refs_json` 中关联 `task_id`。
- task 完成后，可以反向把相关 attention 标记为 `handled`。

第一版最重要的是去重和状态推进，不需要做复杂 UI。至少要能在 agent 详情或调试面板看到 pending/seen/handled，方便判断 agent 是否重复消费。

同时还需要一套 connector 级消费游标，不能只靠 `attention_signals.status`。

原因是外部系统消息可能很多，其中大部分不会进入强 attention。如果没有游标，connector 或 agent 查询历史时仍然可能反复扫描和重复消费。

这也是 IM / inbox / workflow/card 能否稳定工作的关键点：系统必须能表示“这条消息是否被 agent 看过”“这个来源消费到了哪里”。否则 agent 每次 wakeup 都会看到同一批历史消息，最终重复回复、重复审批、重复创建任务。

建议新增或抽象出 `attention_cursors`：

```sql
attention_cursors (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  agent_worker_id TEXT NOT NULL,
  source_kind TEXT NOT NULL,       -- lark | feishu | slack | github | linear | sentry | inbox
  source_channel TEXT NOT NULL,    -- chat_id / repo / stream id / inbox recipient
  cursor TEXT NOT NULL DEFAULT '', -- provider message id, timestamp, event id, offset
  seen_until TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL,
  UNIQUE(workspace_id, agent_worker_id, source_kind, source_channel)
)
```

职责划分：

- `attention_signals` 记录“哪些强信号值得 agent 关注，以及处理到了什么状态”。
- `attention_cursors` 记录“某个来源已经看到了哪里”。
- connector 拉取外部消息时先更新 cursor，再按规则写入强 attention。
- agent 主动查询 ambient context 时，也应能传 `since / cursor / limit`，避免重复拉全量。

第一版至少要对 IM 私聊、群聊 @、inbox 和 workflow/card 事件做 `dedupe_key`；对 GitHub/Linear/Sentry 这类外部系统，可以先只做按时间窗口查询，后续再补 cursor。

## 分阶段落地

### Phase 1: 文档和数据模型准备

- 明确 Agent Worker / Project Membership / Attention Signal 的字段和边界。
- 标注现有 `task / workflow / task template` 不迁移概念。
- 梳理旧 `project_id + agent_id` 依赖点。

### Phase 2: 引入 Agent Worker，并切断旧项目 agent 主体

- 新增 agent_workers。
- 项目成员页继续显示 agent，但底层变成 project membership。
- 删除旧的 project-private agent 主体写入路径。
- 删除旧 `project_id + agent_id` agent API；2.x 前端直接使用 worker / membership 新 API。

### Phase 3: IM 渠道迁移到 Agent Worker

- channel binding 改挂 agent_worker。
- 私聊、群聊、卡片回调都进入 worker 级 interaction。
- 群聊 @agent 生成 attention signal。
- 普通群消息只作为可查询历史，不默认唤醒。

### Phase 4: Wakeup 注入 Attention

- scheduler wakeup 时查询 pending attention signals。
- prompt 只注入摘要和查询入口。
- agent 处理后可标记 seen / handled / ignored。

### Phase 5: 跨项目 Agent Worker

- 同一个 agent worker 可加入多个项目。
- task 指派可以选择项目内的 agent worker membership。
- agent 详情页拆成：
  - workspace agent profile。
  - project membership settings。

### Phase 6: 委托与卡片交互泛化

- IM 卡片点击回调进入 agent interaction session。
- agent 可通过 mga workflow 命令提交用户授权过的 workflow 决策。
- delegation token 从宽到窄逐步增强。

## 迁移策略

由于当前外部用户还不多，可以接受 2.x 不兼容迁移，但必须提供清晰脚本。

迁移规则：

1. 每个旧项目 agent 创建一个 Agent Worker。
2. 如果多个项目里有同名 agent：
   - 配置完全一致时自动合并为一个 worker。
   - 配置不同则默认保留多个 worker，并在迁移报告里提示人工合并。
3. 旧项目 agent prompt 拆为：
   - worker profile prompt。
   - project membership prompt。
4. 旧 agent channel binding 迁移到 worker。
5. 旧 task assignee 一次性改写为 `assignee_type + assignee_id + assignee_membership_id`。
6. 旧 workflow 定义不改产品语义，但节点负责人和 workflow run 当前负责人一次性改写为 membership / worker。
7. 旧 project-level heartbeat 一次性迁到 worker schedule + membership policy。
8. 迁移完成后，旧 project-private agent 数据不再参与运行时读取。

迁移报告必须列出：

- 创建了哪些 Agent Worker。
- 哪些旧 agent 被合并。
- 哪些需要人工确认。
- 哪些 channel / session / task / workflow 被改写到了新主键。
- 哪些旧数据被迁移器保存在备份目录中。

## 详细实施计划

这一版重构应该按“新 schema、一次性迁移、硬切运行时”的方式做。

不做长期兼容层，不保留旧写入路径，不让旧字段继续参与业务判断。迁移风险靠完整备份、dry-run 报告和本地副本试跑控制，而不是在代码里背历史债。

### Step 0: 基线备份和迁移实验环境

先不要直接改用户正在使用的数据。

准备一个迁移实验目录：

```bash
cp -a /root/code/spaceship/spaceship /root/code/spaceship/spaceship-agent-worker-migration-test
```

验证目标：

- 原 Spaceship workspace 不受影响。
- 迁移副本能启动 Web。
- 迁移后原有项目、任务、流程、成员、模型账号、外部工具连接仍然可见。
- 至少选择 `cc-connect`、`multigent`、`customer` 这类真实项目跑 smoke test。

迁移脚本必须支持：

```bash
multigent migrate agent-worker --dry-run --dir <workspace>
multigent migrate agent-worker --apply --dir <workspace>
multigent migrate agent-worker --report <path>
```

`dry-run` 输出迁移计划，不写入数据；`apply` 写入；`report` 生成 markdown/json 报告。

### Step 1: 新建 2.x Schema

直接建立 2.x 需要的数据结构。旧表或旧 JSON 字段只作为迁移输入，不作为迁移后的运行时依赖。

#### agent_workers

workspace 级 agent 主体。

```sql
CREATE TABLE agent_workers (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  name TEXT NOT NULL,
  display_name TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  avatar TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',

  default_model_account_id TEXT NOT NULL DEFAULT '',
  default_runtime_node_id TEXT NOT NULL DEFAULT '',
  default_runtime_mode TEXT NOT NULL DEFAULT '',

  schedule_json TEXT NOT NULL DEFAULT '{}',
  attention_policy_json TEXT NOT NULL DEFAULT '{}',
  memory_policy_json TEXT NOT NULL DEFAULT '{}',
  skills_json TEXT NOT NULL DEFAULT '[]',

  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,

  UNIQUE(workspace_id, name)
);
```

说明：

- `name` 是稳定机器名，例如 `nova`。
- `display_name` 是用户看到的名字，例如 `Nova`。
- schedule 先放 JSON，避免第一版表结构过细。

#### project_memberships

项目成员身份，统一承载 human 和 agent worker。

```sql
CREATE TABLE project_memberships (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  project_id TEXT NOT NULL,
  member_type TEXT NOT NULL, -- user | agent_worker
  member_id TEXT NOT NULL,

  role TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL DEFAULT '',
  prompt TEXT NOT NULL DEFAULT '',
  permissions_json TEXT NOT NULL DEFAULT '[]',

  auto_pick_tasks INTEGER NOT NULL DEFAULT 1,
  attention_enabled INTEGER NOT NULL DEFAULT 1,
  priority_weight REAL NOT NULL DEFAULT 1.0,

  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,

  UNIQUE(workspace_id, project_id, member_type, member_id)
);
```

说明：

- 项目成员页未来主要读这张表。
- 对 agent 来说，`member_id = agent_workers.id`。
- 对人类来说，`member_id = users.id`。

#### attention_signals

agent 的注意力队列。

```sql
CREATE TABLE attention_signals (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  agent_worker_id TEXT NOT NULL,

  source_kind TEXT NOT NULL, -- inbox | lark_message | feishu_message | task | workflow | github | linear | sentry
  source_id TEXT NOT NULL DEFAULT '',
  source_channel TEXT NOT NULL DEFAULT '',
  dedupe_key TEXT NOT NULL,
  reason TEXT NOT NULL, -- direct_message | mention | assigned_task | workflow_step | delegation | escalation
  priority TEXT NOT NULL DEFAULT 'normal', -- critical | high | normal | low

  actor_type TEXT NOT NULL DEFAULT '',
  actor_id TEXT NOT NULL DEFAULT '',

  summary TEXT NOT NULL DEFAULT '',
  refs_json TEXT NOT NULL DEFAULT '{}',
  payload_json TEXT NOT NULL DEFAULT '{}',
  result_ref TEXT NOT NULL DEFAULT '',

  status TEXT NOT NULL DEFAULT 'pending', -- pending | seen | handling | handled | ignored | expired
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL DEFAULT '',
  seen_at TEXT NOT NULL DEFAULT '',
  handling_at TEXT NOT NULL DEFAULT '',
  handled_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_attention_agent_status
  ON attention_signals(workspace_id, agent_worker_id, status, created_at);

CREATE UNIQUE INDEX idx_attention_dedupe
  ON attention_signals(workspace_id, agent_worker_id, dedupe_key);
```

说明：

- 强信号进入这里。
- 普通外部消息不进这里，只保存在各自 connector 的历史或事件表。
- `refs_json` 存 `project_id / task_id / workflow_run_id / doc_id / message_id` 等引用。

#### agent_sessions 调整

现有 interaction session 应该从 `project_id + agent_id` 迁到 worker primary session 维度。

2.x 直接使用新字段：

```sql
agent_sessions (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  agent_worker_id TEXT NOT NULL,
  session_kind TEXT NOT NULL, -- primary | child
  parent_session_id TEXT NOT NULL DEFAULT '',

  project_context_id TEXT NOT NULL DEFAULT '',
  membership_id TEXT NOT NULL DEFAULT '',
  task_id TEXT NOT NULL DEFAULT '',
  workflow_run_id TEXT NOT NULL DEFAULT '',
  purpose TEXT NOT NULL DEFAULT '',

  runtime_session_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(workspace_id, agent_worker_id, session_kind, parent_session_id, purpose)
)
```

说明：

- 每个 Agent Worker 必须有一个 `session_kind=primary` 的主 session。
- child session 必须有 `parent_session_id`。
- `project_context_id` 可以为空；只有进入具体项目工作或 child run 时才有值。
- `runtime_session_id` 映射到底层 Claude / Codex / Cursor 等 runtime 的 session。

IM、Web、卡片等交互事件不再通过 session key 分裂 session，而是进入 interaction event 表：

```sql
agent_interaction_events (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  agent_worker_id TEXT NOT NULL,
  session_id TEXT NOT NULL, -- 默认 primary session
  source_kind TEXT NOT NULL, -- web | lark | feishu | slack | task | workflow
  source_channel TEXT NOT NULL DEFAULT '',
  conversation_key TEXT NOT NULL DEFAULT '', -- user:<id> / chat:<id> / web:<id>
  actor_type TEXT NOT NULL DEFAULT '',
  actor_id TEXT NOT NULL DEFAULT '',
  refs_json TEXT NOT NULL DEFAULT '{}',
  payload_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
)
```

`conversation_key` 只用于记录来源和回复目标，不用于默认切 session。

#### agent_channel_bindings 调整

2.x 直接把 channel 绑定到 Agent Worker：

```sql
agent_channel_bindings (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  agent_worker_id TEXT NOT NULL,
  provider TEXT NOT NULL,
  connection_id TEXT NOT NULL,
  app_id TEXT NOT NULL DEFAULT '',
  receive_mode TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  created_by TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
)
```

迁移后主体从：

```text
workspace_id + project_id + agent_id
```

变成：

```text
workspace_id + agent_worker_id
```

不保留 `project_id + agent_id` channel 主键。迁移后 IM 渠道属于 agent 主体，项目只作为消息引用和权限上下文。

#### tasks 字段调整

2.x task 直接写新负责人字段：

```sql
assignee_type TEXT NOT NULL, -- user | agent_worker
assignee_id TEXT NOT NULL,
assignee_membership_id TEXT NOT NULL DEFAULT ''
```

迁移后删除旧 `agent` / `project_agent` 负责人字段，运行时代码不得再通过旧字段解析 agent。

#### workflow runs / node assignees 字段调整

workflow 定义本身不改，但 workflow run 的节点负责人解析需要支持 membership。

2.x workflow run 直接记录当前节点负责人：

```sql
current_assignee_type TEXT NOT NULL DEFAULT '', -- user | agent_worker
current_assignee_id TEXT NOT NULL DEFAULT '',
current_assignee_membership_id TEXT NOT NULL DEFAULT ''
```

如果当前存储不是 SQL 表，而是 JSON 文件，也按同名字段扩展。

### Step 2: Entity 和 Store 层切到新模型

不做旧模型 resolver。迁移完成后，业务代码只接受新身份模型：

```go
type AgentDirectory interface {
    GetWorker(ctx, workspaceID, workerID string) (*AgentWorker, error)
    ResolveMembership(ctx, workspaceID, projectID, memberType, memberID string) (*ProjectMembership, error)
    ResolveWorkerForTask(ctx, workspaceID, projectID string, task *Task) (*AgentWorker, *ProjectMembership, error)
}
```

所有核心入口改成 worker/membership 语义：

- workspace agents API：按 `agent_worker_id` 管理主体。
- project members API：按 `project_membership_id` 管理项目身份。
- scheduler：按 worker schedule。
- runtime run：必须带 `agent_worker_id`，执行项目任务时必须带 `project_id + membership_id`。
- workflow step assignment：记录 membership。
- mga runtime principal：携带 worker 和 project context。

旧 `/projects/:project/agents/:agent` 路由可以删除，或仅在前端路由层根据迁移报告做一次性跳转；后端不再用它作为稳定 API。

### Step 3: 迁移旧项目 agent 到 Agent Worker

迁移规则建议先保守，不自动激进合并。

输入旧结构：

```text
projects/<project>/agents/<agent>/
```

生成：

```text
agent_workers/<worker>/
projects/<project>/members/<membership>.yaml
```

第一版合并规则：

1. 每个旧项目 agent 默认生成一个 worker。
2. worker name 默认 `<agent>`。
3. 如果同名 agent 出现在多个项目：
   - 如果模型、角色、prompt、技能、渠道配置完全一致，自动合并。
   - 如果不同，生成 `<agent>-<project>`，并在报告里提示“可人工合并”。
4. 后续提供手动合并工具：

```bash
multigent agent-worker merge --from customer-cli/nova --into nova
```

这样避免一开始误合并导致上下文污染。

### Step 4: Scheduler 改成 Worker Schedule

新增 worker 级 scheduler loop：

```text
for each active agent_worker:
  if schedule due:
    create wakeup cycle
    collect attention signals
    collect eligible project tasks
    collect workflow steps assigned to this worker
    ask/run worker triage
```

旧项目 heartbeat 迁移：

```text
projects/<project>/agents/<agent>/heartbeat.yaml
```

变成：

```text
agent_workers/<worker>/schedule.yaml
projects/<project>/members/<membership>.yaml:
  auto_pick_tasks: true
  attention_enabled: true
```

冲突处理：

- 同一个 worker 多个旧 heartbeat 一致：自动合并。
- 不一致：取最频繁 interval 作为 worker schedule，并在报告中列出差异。
- 不做 `membership.schedule_override` 兼容。确实需要不同节奏时，拆成不同 Agent Worker，或者调整 worker schedule 和项目 priority policy。

### Step 5: Wakeup Prompt 改成 Attention Triage

旧 wakeup prompt 偏“当前项目 agent 的 wakeup”。

新 wakeup prompt 应包含：

```text
你是 <Agent Worker>。

你参与的项目:
- customer-cli: 项目管理者, auto_pick_tasks=true
- customer-mcp-server: 项目管理者, auto_pick_tasks=true

强 attention:
1. Lark 群 @你: ...
2. task t-xxx 分配给你: ...
3. workflow t-yyy 到达你的节点: ...

可选择处理的项目任务:
1. customer-mcp-server / t-001 / P1
2. customer-cli / t-002 / P2

你可以:
- 回复 IM。
- 创建/更新任务。
- 进入某个项目上下文执行任务。
- 标记 attention 为 handled/ignored。
```

注意：这里是 triage，不是把所有项目上下文一次性塞进去。真正处理任务时，再构建具体项目上下文。

### Step 6: Runtime Run 绑定 Project Context

新增运行参数：

```go
type AgentRunContext struct {
    WorkspaceID string
    AgentWorkerID string
    ProjectID string
    MembershipID string
    TaskID string
    WorkflowRunID string
    PrimarySessionID string
    ChildSessionID string
    SourceKind string
    SourceChannel string
}
```

运行时必须满足：

- 普通聊天可以只有 `AgentWorkerID`，不一定有 `ProjectID`。
- 读项目文件、项目知识库、任务、workflow 时必须有 `ProjectID + MembershipID`。
- 默认 run 进入 `PrimarySessionID`。
- agent 主动并发或隔离时，创建 `ChildSessionID`，完成后回写 primary session。
- 同一个 worker 的并发数受 `max_concurrent_runs` 限制。

### Step 7: IM 事件改成 Attention + Interaction

Feishu / Lark 收到消息后：

1. 根据 app/channel 找到 `agent_worker_id`。
2. 找到该 worker 的 primary session。
3. 根据发送者或群聊生成 `conversation_key`，作为来源和回复目标。
4. 写入 interaction event，挂到 primary session。
5. 判断是否强 attention：
   - 私聊：是。
   - 群聊 @agent：是。
   - 群聊普通消息：否，只存历史。
   - 卡片点击：是。
6. 强 attention 写入 `attention_signals`。
7. 是否即时唤醒由 worker attention policy 决定，不由 connector 硬编码。

这样 IM connector 只负责“记录世界发生了什么”和“递送强信号”，不负责决定 agent 具体怎么工作。

### Step 8: 前端改造

2.x UI 直接切新心智，不做旧项目 agent 页面兼容。

#### 一级导航新增 Agents

一级导航应该新增 `Agents` 页面，中文可以叫“智能体”。

这个页面管理 workspace 级 Agent Worker，也就是“公司里有哪些智能体同事”。

它承接原来项目成员页里的“雇佣 agent”能力：

- 创建 agent。
- 设置 agent 名称、头像、描述、长期职责。
- 设置默认模型账号。
- 设置默认运行节点。
- 设置默认技能。
- 设置协作渠道。
- 设置工作节奏 schedule。
- 查看该 agent 参与了哪些项目。
- 查看最近 wakeup、最近运行、token / cost 摘要。

列表建议展示：

| 字段 | 说明 |
| --- | --- |
| Agent | 头像、名称、简介 |
| 默认模型 | 当前默认 provider/model |
| 默认运行节点 | local / remote node |
| 工作节奏 | interval / active hours |
| 参与项目 | 项目数量和项目名摘要 |
| 状态 | idle / running / blocked / offline |
| 最近活动 | last wakeup / last message |

主要操作：

- `New agent`
- `Open profile`
- `Add to project`
- `Configure channels`
- `Configure schedule`

#### Workspace Agent Detail

原来的项目成员详情页里属于“agent 本体”的内容，应迁移到 workspace agent 详情页。

保留在 agent 详情页的模块：

- 基本信息：名称、头像、描述、长期职责。
- 默认模型账号。
- 默认运行节点。
- 默认技能。
- 协作渠道：Feishu / Lark / Slack / Web 等。
- 工作节奏：schedule / heartbeat / active hours / max cycle duration / max concurrent runs。
- Attention policy：私聊、@、任务分配、workflow 节点等是否即时提醒或仅进入队列。
- 参与项目列表。
- 全局最近运行记录。
- 全局对话入口。
- 全局上下文绑定：长期记忆、可复用资料、跨项目经验。

不应该放在 workspace agent 详情页的模块：

- 某个项目里的角色说明。
- 某个项目里的项目 prompt。
- 某个项目里的权限。
- 某个项目里的任务列表筛选。
- 某个项目里的 workflow 节点职责。

这些属于 Project Membership。

#### 一级导航新增 Schedule / Work Rhythm

计划和心跳需要从项目里上移。

一级页面可以叫：

- 英文：`Schedule`
- 中文：`计划`

但产品表达上更准确的是 agent 的“工作节奏”，不是项目里的定时任务列表。

这个页面展示所有 Agent Worker 的 schedule：

| 字段 | 说明 |
| --- | --- |
| Agent | 哪个智能体 |
| 工作时间 | active hours / active days |
| 唤醒间隔 | interval |
| 并发限制 | max concurrent runs |
| 单轮限制 | max tasks per cycle / max duration |
| 下一次唤醒 | next wakeup |
| 今日唤醒 | wakeup count |
| 状态 | enabled / paused / running |

操作：

- 启用/暂停 schedule。
- 编辑工作时间。
- 编辑唤醒间隔。
- 编辑并发限制。
- 手动唤醒 agent。
- 查看最近 wakeup 运行。

项目页面不再提供 agent 级 heartbeat 编辑。项目只配置“这个 agent 在本项目里是否自动接活”。

#### 项目成员页

项目成员页仍然保留，但语义改成“这个项目有哪些人和智能体参与”。

原来的 `Hire agent` 改成：

```text
Add agent to project
```

交互：

1. 从 workspace 已有 agent 中选择。
2. 设置该 agent 在本项目中的角色。
3. 填写项目内职责说明 / project prompt。
4. 设置项目权限。
5. 设置是否自动接本项目任务。
6. 设置是否接收本项目 attention。
7. 保存后创建 project membership。

如果用户确实想创建一个全新的 agent，可以在弹窗里提供次级入口：

```text
Create new agent first
```

但主路径应该是“选择已有 agent 加入项目”，而不是每个项目重新雇佣一份 agent。

项目成员列表展示：

- human members。
- agent workers in this project。
- project role。
- auto pick tasks。
- attention enabled。
- 当前项目任务数。
- 当前项目最近活动。

点击 agent 时：

- 打开 Project Membership Detail。
- 顶部展示 agent worker 的简短主体信息。
- 主体内容展示“该智能体在本项目中的设置”。
- 提供跳转到 workspace agent profile 的链接。

#### Project Membership Detail

这是原来项目 agent 详情页中“项目相关”部分的承接页面。

保留：

- 项目角色。
- 项目职责 prompt。
- 项目权限。
- 自动接活开关。
- attention 开关。
- 项目内任务列表。
- 项目内 workflow 节点职责。
- 项目内对话入口。
- 项目内运行记录。

不保留：

- 默认模型账号编辑。
- 默认运行节点编辑。
- 全局协作渠道配置。
- 全局 schedule。

如果某个项目需要覆盖默认模型或运行节点，可以作为高级设置，但第一版建议先不做，避免重新把 agent 本体配置散落回项目。

#### Agent Chat 页面

聊天页不应该在产品心智上拆成“workspace 普通对话”和“project-context 对话”两套。

用户看到的应该始终是：

```text
和 Nova 聊天
```

底层默认进入 Nova 的 primary session。

页面可以显示和切换“当前项目上下文”，但这不是 session 边界。

进入方式：

- 从一级 Agents 页面进入：默认没有项目上下文。
- 从项目成员页进入：默认带上该项目 context。
- 从任务 / workflow follow 进入：默认带上该 task / workflow context。
- 从 IM 进入：根据 message refs 显示可能相关的项目和任务。

页面上需要显示：

- 当前 agent worker。
- 当前 primary session。
- 当前 project context，如果当前对话正在围绕某项目。
- 当前 project membership，如果需要操作项目资源。
- 当前来源：Web / Lark DM / Lark group / Feishu。
- 当前关联 task / workflow，如果有。

如果没有项目上下文，agent 仍然可以聊天、做全局判断、询问用户要处理哪个项目；但不允许直接读写项目任务和 workflow。需要用户选择项目，或者 agent 自己根据用户授权进入某个项目上下文。

如果没有项目上下文，页面应提示：

```text
这是和 Nova 的主工作会话。需要它处理项目任务时，请选择项目上下文。
```

任务执行和 IM 消息也默认进入同一个 primary session。只有 agent 主动创建 child session 时，页面才展示“子会话 / 并发任务”的关系。

#### Attention Inbox

可以在 agent 详情页或一级 Agents 页面里增加一个 Attention 视图。

第一版不一定单独做一级页面，但数据模型要支持。

展示：

- 未处理 attention。
- 来源：Web / Lark / Feishu / task / workflow / GitHub。
- 原因：私聊 / @ / 分配任务 / workflow 节点 / 委托。
- 关联项目和任务。
- 状态：pending / seen / handled / ignored。

这个视图主要用于调试和透明化，让用户知道 agent 为什么被唤醒、它忽略了什么、处理了什么。

#### Runs 页面

Runs 页面需要从 project-only 改成支持 worker 维度。

需要支持筛选：

- by agent worker。
- by project。
- by task。
- by source：schedule / manual / IM / workflow / task。
- by session。

项目 runs 页面仍然可以存在，但它只是过滤 `project_id`。

#### Settings 页面

设置里的模型账号、运行节点、外部工具仍然是 workspace 级。

但展示逻辑要和 Agent Worker 关联起来：

- 模型账号配置后，可以分配给某个 agent worker 作为默认模型。
- 运行节点配置后，可以分配给某个 agent worker 作为默认运行节点。
- 外部工具连接配置后，可以被 agent worker 的技能或协作渠道使用。

项目内不再重复展示这些全局配置，最多展示“当前 agent 使用的是哪个默认配置”。

#### Onboarding / Example Workspace

新手引导也要调整心智：

旧路径：

```text
创建项目 -> 雇佣 agent -> 配任务和流程
```

新路径：

```text
创建 agent -> 把 agent 加入项目 -> 给项目创建任务和流程 -> agent 按自己的工作节奏处理项目任务
```

但为了降低心智负担，引导文案可以仍然使用用户熟悉的话：

```text
先创建一个智能体同事，再把它加入项目。
```

### Step 9: 本地迁移试跑

先在本地副本迁移 Spaceship workspace。

测试项目建议：

1. `cc-connect`
   - PM / QA / release agent 的 schedule 是否迁移。
   - GitHub issue / PR 流程是否还能跑。
   - 增量同步和任务创建是否正常。

2. `multigent`
   - 宣传文章流程是否还能跑。
   - 图片素材、知识库、workflow follow 是否正常。

3. `github-sandbox`
   - 人类审核节点。
   - IM 卡片委托。
   - workflow review。

4. `customer-*`
   - 同一个管理 agent 加入多个项目。
   - wakeup 后能看到多个项目 attention。
   - 实际执行时能进入正确项目上下文。

验收 checklist：

- Web 能打开。
- 项目列表正常。
- 项目成员正常。
- agent 详情正常。
- 任务列表正常。
- workflow follow 正常。
- IM 绑定仍然能用。
- 至少一个 agent wakeup 能跑通。
- 至少一个 workflow 从 agent 节点流转到 human 节点再继续。
- 迁移报告无未解释的高风险项。

### Step 10: 本地切换到 2.x 运行

本地副本验证通过后，直接使用 2.x runtime 跑迁移后的 workspace：

- 新建 agent：只写 `agent_workers`。
- 将 agent 加入项目：只写 `project_memberships`。
- 新建 channel：只绑定 worker。
- 新建 task：只写 worker assignee 字段。
- scheduler：只按 worker schedule 创建 runs。
- 项目成员页：只读 project memberships。

如果这里发现严重问题，回滚方式是恢复迁移前 workspace 备份，而不是让 2.x 代码继续兼容旧结构。

### Step 11: 正式迁移当前本地 workspace

试验副本通过后，再迁移当前本地 workspace。

执行顺序：

1. 停止本地服务和 scheduler。
2. 备份 workspace 和数据库。
3. 运行 `multigent migrate agent-worker --apply`。
4. 启动 2.x 服务。
5. 执行 smoke test。
6. 观察 24 小时 heartbeat、workflow、IM。

### Step 12: 客户环境迁移

客户环境不做在线热迁移。

每个客户环境按下面步骤执行：

1. 约定维护窗口。
2. 停服务。
3. 备份 workspace / DB / runtime 配置。
4. dry-run 生成迁移报告。
5. 人工确认报告。
6. apply 迁移。
7. 启动 2.x。
8. 跑客户关键流程 smoke test。

如果失败，恢复备份并继续停留在 1.x，不在 2.x 里加旧结构兼容。

## 风险与规避

### 风险 1: 跨项目上下文污染

规避：

- runtime run 必须绑定 project context。
- wakeup triage 只给摘要，不塞全量项目资料。
- agent 进入任务执行时再构建具体项目上下文。

### 风险 2: 同名 agent 自动合并错误

规避：

- 第一版保守迁移，配置不完全一致不自动合并。
- 迁移报告列出建议合并项。
- 提供手动 merge 工具。

### 风险 3: Primary session 上下文过长或混乱

规避：

- 所有外部输入先变成 attention 摘要，不把全量消息都塞进 prompt。
- 项目上下文是 tool/resource 操作边界，不是默认 session 边界。
- child session 只用于 agent 主动并发或隔离，完成后必须回写 summary。
- interaction event 保留 `conversation_key`，用于回复到正确的人或群，但不用于拆 session。
- 第一版不自研 session 压缩机制，先依赖 Claude Code / Codex / Cursor 等 runtime 自身的上下文管理能力。
- 后续如果 primary session 变长造成明显质量下降，再补 memory checkpoint / task receipt / daily journal 等压缩机制。

### 风险 4: scheduler 行为变化太大

规避：

- dry-run 报告展示每个旧 heartbeat 会迁到哪里。
- 不一致的 heartbeat 不自动静默兼容，迁移报告必须要求人工确认。
- 本地副本跑至少 24 小时观察。

### 风险 5: UI 心智变复杂

规避：

- 产品语言仍然说“项目成员”和“任务”。
- 不在普通用户界面暴露过多底层术语。
- `Agent Worker` 可以在中文 UI 中叫“智能体”。
- `Project Membership` 不作为显式名词展示，只展示“该智能体在本项目中的设置”。

## 当前不做

这次重构不做：

- 不引入 WorkItem 产品概念。
- 不重命名 task。
- 不重命名 task template。
- 不重做 workflow 引擎。
- 不把所有外部事件都做成 trigger。
- 不做专门的秘书系统。
- 不让 IM 渠道绑定到某个项目。

## 当前实现缺口

截至当前 2.x 重构验证，底层已经有 `agent_workers`、`project_memberships`、`attention_signals`、`attention_cursors`，并且任务和 workflow run 已开始写入新的负责人字段。但还不能认为重构完成，主要缺口如下：

### 1. Scheduler loop 已开始 worker-level 化，但还不是完整 triage

`HeartbeatConfig` 已经落到 Agent Worker 的 `schedule_json`。当前实现已经把 2.x DB workspace 的 scheduler target 按 Agent Worker 聚合：

- 同一个 Agent Worker 加入多个项目时，只启动一个 heartbeat target。
- 这个 target 保留该 worker 的所有项目 membership。
- 每次 wakeup 前，会在这些 membership 中按 pending task 的优先级和创建时间选择一个具体项目上下文执行。
- 没有 pending task 时，仍回到默认 membership 跑 idle wakeup / attention。
- runtime-node scheduler 的 tick 也已开始按 Agent Worker 聚合 membership：从某个项目成员入口启动时，会把同一 worker 的其他可自动接任务 membership 纳入候选，并优先执行最高优先级 pending task 所在项目。
- runtime-node scheduler 的进程 key / desired scheduler 记录 / 停止逻辑已经开始使用 `worker/<id>`。也就是说，从同一 Agent Worker 的不同项目 membership 启动调度时，不会再启动多条重复 runtime-node loop。
- 本地 CLI scheduler 仍保留旧的 `all` / `project` / `project/agent` 进程 key，避免影响开源本地默认运行方式。

这解决了“同一 worker 被多个项目重复唤醒”的第一层问题，但还不是最终完整形态。正确方向仍然是：

1. scheduler 按 Agent Worker schedule 唤醒。
2. wakeup 后先做跨项目 attention / task triage。
3. 选择具体项目任务后，再进入该项目上下文执行。
4. 执行时仍通过 Project Membership 获取项目角色、权限和上下文。

仍需补齐：

- triage prompt 应让 agent 主动判断多个项目 attention / task，而不是平台只按优先级机械选择。
- cron-only 逻辑仍保留项目成员形态，需要后续合并进 worker-level schedule。
- project-wide / all scheduler start-stop 还没有完整 worker-level 化；第一版优先保证“指定某个 agent 启动 runtime-node scheduler”时不会重复唤醒同一个 Agent Worker。

### 2. Task queue 仍然存储在 project/member 维度

任务模型已经有 `assigneeType / assigneeId / assigneeMembershipId`，但任务记录仍按旧 key 存在 `project/agent/task_id` 下。

第一版可以接受这个物理存储方式，但运行时代码必须逐步做到：

- 任务负责人判断优先读 `assigneeType / assigneeId / assigneeMembershipId`。
- `project/agent` 只作为项目内路由别名和任务队列地址，不再作为身份主键，也不作为旧结构兼容入口。
- 后续如果需要彻底清理，可以把任务物理 key 改成 `project/task_id`，负责人只存在 payload 里。

### 3. IM channel 已优先按 worker 使用，但表结构仍有旧字段

IM channel 已经补了 `agent_worker_id`，但表结构和部分代码仍保留 `project_id / agent_id`。

当前已经做到：

- 项目成员页打开某个 agent 的协作渠道时，底层会优先解析到 Agent Worker。
- runtime 内部调用通知能力时，优先按 Agent Worker 查渠道绑定，不再只查当前 `project/agent`。
- 同一个 Agent Worker 从不同项目上下文调用通知，可以复用同一份渠道绑定。

仍需继续收口：

- 新建 channel 时必须绑定到 Agent Worker。
- 项目成员页可以作为入口打开 channel 设置，但底层不能生成“某项目专属的同一个 agent 渠道”。
- `project_id / agent_id` 后续只作为迁移记录或展示来源，不参与身份判断。
- 同一个 Agent Worker 在不同项目入口打开同一渠道，应看到同一份绑定、同一批用户/群聊绑定、同一个 interaction 主会话。

### 4. 外部工具绑定已开始按 worker 使用

外部工具的 `agent_tool_bindings` 已补 `agent_worker_id`，runtime 解析连接时会优先按 Agent Worker 查绑定：

- 项目成员页仍可以作为入口给 agent 配置外部工具。
- 如果该项目成员解析到 Agent Worker，保存的 tool binding 会绑定到 `agent_worker_id`。
- 同一个 Agent Worker 在另一个项目 membership 下运行时，也能看到同一份 tool binding。
- connection grant 匹配只接受 `agent_worker:<id>` 目标，不再接受旧 `project/agent` 目标。

仍需继续收口：

- 外部工具授权 UI 仍主要在项目成员详情页，不在一级 Agent Worker 详情页。
- connection grant 的公开管理 UI 还没有完整展示 `agent_worker:<id>` 目标。
- 批量安装项目工具时仍以“项目入口”发起，底层可以落 worker binding，但用户心智还需要重新设计。
- env var resolver 已支持 `agent_worker:<id>` target。runtime-node 注入环境变量时会同时匹配当前项目成员和 Agent Worker 身份。
- env var UI 仍主要展示 `project/agent` 字符串，后续需要改成从一级 Agent Worker 页面选择 worker，而不是让用户手写 target。

### 5. Web 对话和 IM 对话的 runtime session 已开始统一

目标模型是 Agent Worker 的 primary session 为主：

- 用户在 Web 和 IM 里找同一个 agent，本质上都进入同一个 Agent Worker 的长期会话模型。
- `conversation_key` 只用于回复到正确的人、群和消息线程，不用于拆分 agent 主体 session。
- agent 可以主动创建 child session 并发处理，但这是 agent 的能力，不是平台按来源写死拆 session。

当前已经做到：

- interaction session 记录带 `agent_worker_id`。
- 当存在 Agent Worker 身份时，新的 interaction 会复用该 worker 最近一次 runtime session，而不是按 Web/IM source 拆 runtime session。
- 不同用户、不同群聊、不同卡片回调仍有各自的 interaction event / conversation source，用于路由回复和审计，但不再决定 agent 的 primary runtime session。

仍需继续检查：

- Web chat、IM chat、card callback、runtime run 的所有入口都应优先携带 worker identity。
- 前端 session 列表需要表达“这是 agent 的主会话 + 多个 conversation source”，避免用户误以为每个来源都是一个独立 agent。

### 5. 前端信息架构还没完全切到 workspace-level agent

当前已新增一级 Agents 页面和一级 Schedule 页面，项目成员也开始表达 membership，但仍有一些页面沿用旧心智：

- 项目成员详情页仍承载大量 agent 详情能力。
- 协作渠道入口仍从项目成员进入。

当前项目成员列表页已经切到 2.x 单数据源：只读 `project_memberships`，不再同时请求旧的 `projects/:project/agents` 列表作为 fallback。项目里添加智能体的主路径也是“选择已有 workspace Agent Worker 加入项目”，而不是在项目内创建一份私有 agent 主体。

当前一级 Schedule 页面已经可以按 Agent Worker 展示工作区级工作节奏、触发策略和运行状态；项目 Schedule 页面继续作为项目过滤视图存在，用于查看和操作该项目内成员的 heartbeat / cron / runtime 情况。

2026-08-21 追加前端收敛：

- Command Palette 的项目成员搜索不再直接读取旧 `/projects/:project/agents` 列表，改为读取 `/projects/:project/memberships` 并只展示 `agent_worker` membership。
- Workbench 创建任务 / 运行 agent 所需的项目 agent 选项也改为读取 memberships。
- `/projects/:project/agents` 后端 endpoint 在 2.x 中是 membership-backed 项目成员视图：只从 `project_memberships` 返回项目内 alias/title、worker id、membership id 和模型信息，不再读取旧文件 agent 列表。
- 已补 `TestProjectAgentsEndpointReadsAgentWorkerMemberships`，约束该 endpoint 返回项目内 membership title，而不是 worker 全局 name，避免路由和任务负责人错位。
- `PATCH /projects/:project/agents/:agent` 在能解析到 Project Membership 时已经切到 2.x 写路径：项目内改名只更新 membership title；头像和默认运行节点更新 Agent Worker；不会再 rename 旧 agent 目录或写旧 agent meta。已补 `TestPatchProjectAgentUpdatesMembershipBackedWorker`。
- `agentExistsInProject` 已改为只解析 Project Worker / Membership。任务创建、runtime token、connection grant、workflow runtime 等共享存在性校验都不再依赖旧 agent 文件。已在 `TestValidateIdentityResolvesAgentWorkerMembership` 覆盖。

仍未完成：

- 项目 agent 详情、聊天、运行环境、tool bindings、context bindings、cron/heartbeat 等页面和 API 路由仍然使用 `/projects/:project/agents/:agent/...` 形式作为项目内成员入口。它们内部应继续优先解析到 Agent Worker + Project Membership，但产品上还没有完全切到 workspace agent profile + project membership settings 的新路由。
- `ProjectAgentDetailPage` 里仍混合“agent 本体设置”和“项目 membership 设置”；其中身份、头像和运行节点保存路径已切到 2.x，但页面信息架构仍需要继续拆分。

Trigger manager 的调用入口目前仍保持 `project / agent` 参数形态，这是为了避免一次性改动所有任务、消息和 workflow 调用点。但内部判定已经开始按 2.x 身份模型收敛：

- 触发前先通过 `project_memberships` 解析 `project / agent` 背后的 Agent Worker。
- 如果该 Agent Worker 的 `schedule_json.triggers` 已配置，则以 worker schedule 为准。
- 如果没有解析到 Agent Worker，触发会被跳过并记录原因；2.x 不再读取旧项目 heartbeat。
- 合法 trigger 已统一为 `message / task / attention / im_direct_message / im_mention / workflow_step_assigned / card_action`。

这意味着 IM 私聊、群聊 @、卡片点击等强 attention 是否立即唤醒，已经可以由 Agent Worker 的工作节奏配置表达，而不是由 Feishu / Lark connector 写死。

剩余边界：

- 触发命令本身仍然调用 `scheduler wakeup --project --agent`，只是底层 context / session / schedule 会优先走 worker。
- 后续如果要完全去掉旧入口，需要把 `Fire(project, agent, ...)` 改成 `FireWorker(workerID, membershipID, ...)`，并把任务、workflow、message 调用点逐步切过去。

Task / workflow 的强 attention 已开始接入：

- Web 创建任务、runtime 创建任务、任务重分配都会为目标 Agent Worker 写入 `AttentionSignal(reason=task_assigned)`。
- workflow 流转到 agent 节点、并行分支子任务创建后，会为目标 Agent Worker 写入 `AttentionSignal(reason=workflow_step_assigned)`。
- signal 通过 `project/agent/task/reason` 做 dedupe，不会因为重复触发或重复保存任务而生成多条 pending signal。
- 任务信号会携带 `taskId / project / membershipId`，如果任务已绑定 active workflow run，还会携带 `workflowRunId / workflowId / workflowStepId / currentAssignee*`。
- 写入 attention 不等于立即唤醒；是否立刻唤醒仍由 Agent Worker 的 `schedule_json.triggers` 决定。

Runtime principal 也已开始携带 2.x 身份：

- runtime run 记录会写入 `agent_worker_id` 和 `project_membership_id`。
- agent runtime token 会携带 `agentWorkerId` 和 `projectMembershipId`，不再只依赖 `project/agent` 反查。
- runtime node 注入环境变量时会提供 `MULTIGENT_AGENT_WORKER_ID` 和 `MULTIGENT_PROJECT_MEMBERSHIP_ID`。
- runtime attention / notify 入口要求 principal 携带 `agentWorkerId` / `projectMembershipId`，不再从旧 token 或旧 `project/agent` 元信息兜底。
- Web chat / readiness 入口已改为通过 Project Membership 解析 Agent Worker meta，避免迁移后忽略 worker 上配置的默认模型账号、运行节点和 runtime model。
- Web chat session 列表和历史读取按 `agent_worker_id / project_membership_id` 查询 runtime-node run；项目内 `project/agent` 仅作为当前 membership 的显示和路由别名。
- 项目 schedule API 枚举 Project Membership，并通过 Agent Worker 的 `schedule_json` 显示 heartbeat；没有 membership 时不再回退旧项目 agent 目录。
- IM 强 attention 即时处理前的 runtime readiness 检查、Web 手动运行前的 readiness 检查，已统一通过 Project Membership 解析 Agent Worker meta，避免读取旧 `project/agent` meta 后忽略 worker 上的模型账号、运行节点和 runtime model。
- 项目成员详情页保存 CLI 类型、模型账号和 runtime model 时，只写入 Agent Worker；无法解析 Project Membership 时直接报错。这样从项目入口配置同一个 Agent Worker，不会只改当前项目里的影子配置。
- 项目成员入口配置模型账号时，授权校验也已改为 worker-aware：能解析到 Agent Worker + Project Membership 时，不再依赖旧 `AgentMeta` 文件判断模型类型和权限，避免纯 2.x 成员无法绑定模型账号。
- runtime task API 与 workflow pending review 枚举项目成员时，已优先读取 Project Membership。迁移后只有 `agent_workers/project_memberships`、没有旧项目 agent 目录的成员，也能通过 `mga tasks` / runtime task API 看到自己的任务和待审核项。
- 项目任务列表、项目消息列表和首页 stats 统计已改为 Project Membership 优先枚举。迁移后只有 worker/membership、没有旧 agent 目录的项目成员，也能在 Web 项目任务、项目消息和总览统计中出现。
- 计费用量的 Agent 数已改为 workspace 级 Agent Worker 口径。同一个 Agent Worker 加入多个项目只计一次；未配置模型的 Agent Worker 也会计入额度，避免通过先创建未配置 agent 绕过限制。
- workflow、scheduler、review 等共用的 `findTaskInProject` 已改为 Project Membership 优先查找任务，避免 worker-only 任务在流程推进、人工审核或调度路径里找不到。
- runtime contacts 已改为 Project Membership 优先枚举同项目 agent。迁移后 agent 通过 `mga contacts` 仍能看到同项目 worker 成员，而不依赖旧 agent 目录。
- 删除模型账号时会同步清理 Agent Worker 的 `DefaultModelAccountID`，避免 2.x worker 继续引用已经删除的 provider。
- Agent Worker 已新增 `runtime_config_json`，用于承载 worker 级运行配置。当前已接入 `env / sandbox / addDirs`：从项目成员入口保存这些配置时，会写入 worker runtime config；`agentMetaForProjectMember` 合成运行 meta 时会读出这些配置。
- CLI `multigent agent set-env / unset-env / list-env` 也已改为 worker-only：命令会解析项目 membership 背后的 Agent Worker，并读写 `runtime_config_json.env`；无法解析 worker 时直接报错。
- wakeup prompt 编辑已从旧 `projects/<project>/agents/<agent>/.multigent/context/wakeup.md` 写路径迁到 Agent Worker schedule：项目成员入口保存 wakeup 时，会直接写入 `schedule_json.wakeupPrompt`；`GET context` 也会从 worker schedule 读取 wakeup 文本，不再读取旧 `@file` 路径。

仍未完成：

- HTTP agent、run command 等更少用的运行配置已预留在 runtime config 结构中，但前端保存路径还没有完整迁移验证。

最终形态应该是：

- 一级 Agents 页面创建和管理 Agent Worker。
- Agent Worker 详情管理模型账号、默认运行节点、primary session、全局 schedule、协作渠道、跨项目 attention。
- 项目成员页只管理“这个 agent 在本项目的角色、职责、权限、项目上下文和任务关系”。
- 项目页面仍可以提供快捷入口，但不能让用户误以为 agent 只能属于一个项目。

### 6. Seen / cursor 机制需要继续做真实端到端验证

当前已有 `attention_signals` 和 `attention_cursors`。Web/API 可更新 signal 状态，runtime API 与 `mga attention list/mark` 也已经提供给 agent 使用。`mga attention list` 通过 runtime API 读取到 `pending` signal 时，会把这些 signal 推进为 `seen`，避免下次仍作为未读强信号重复注入。

调度 wakeup 注入也已经开始接入：

- 本地 CLI scheduler 在 idle wakeup routine 前会注入 pending attention 摘要，并在运行前把这些 signal 标记为 `seen`。
- runtime-node scheduler 在没有 pending task、准备创建 wakeup task 时，也会把 pending attention 摘要注入 wakeup prompt。
- runtime-node scheduler 只有在 runtime run 成功入队后才把这些 signal 标记为 `seen`，避免入队失败导致 attention 丢失。
- 如果当前已经有明确 pending task，runtime-node scheduler 不会把其他 attention 混进这个任务 prompt，避免干扰具体任务执行。
- `recordIMAttentionSignal` 已经在写入 IM attention signal 时同步推进 `attention_cursors`，并通过 `(workspace_id, agent_worker_id, dedupe_key)` 保证同一 IM message 重放不会产生多条 pending signal。
- 已补 `TestRuntimeWakeupRunMarksAttentionSeenAfterEnqueue`，直接约束 runtime-node wakeup run 成功入队后才把注入的 attention signal 推进为 `seen`，并验证 queued run 携带 `agentWorkerId/projectMembershipId`。
- 已扩展 `TestRecordIMAttentionSignalPersistsSignalAndCursor`，约束同一 IM message 重放时只保留一条 pending signal，并且 cursor 更新到该 message ID。
- 已修复一个 2.x 解析 bug：`ProjectWorker(project, "agent-name")` 过去会先按 workspace 全局 worker name 匹配，可能被同名但未加入该项目的 Agent Worker 抢走，导致项目 membership 上真实绑定的 runtime node / model 配置被忽略。现在解析顺序改为先枚举该项目的 memberships，再按 membership ID、worker ID、membership title、worker name / displayName 匹配；已补 `TestProjectWorkerPrefersProjectMembershipTitleOverGlobalWorkerName` 回归测试。
- 已发现并修复一类重启风险：API 进程重启后，旧 scheduler 子进程可能成为孤儿进程；API 根据 desired scheduler 状态恢复时又启动新的同项目 scheduler，导致同一项目出现两个 `scheduler start --project <project>` 进程，存在重复消费 task / attention 的风险。现在 `scheduler start` 会在 workspace `.multigent/scheduler-locks/` 下持有 root/project/agent 级启动锁，同 scope 已有活进程时直接拒绝启动；已补 `TestAcquireSchedulerStartLockRejectsLiveProcess` 和 `TestAcquireSchedulerStartLockReplacesStaleLock`。

后续仍必须通过真实 IM、任务、workflow 和 scheduler 测试确认：

- 私聊、@、卡片点击不会重复写入重复强 attention。
- 被注入 wakeup 的 signal 会从 pending 推进到 seen。
- agent 处理完成后能通过 `mga attention mark <signal-id> --status handled` 标记 handled，或通过 `--status ignored` 明确忽略。
- connector cursor 能避免外部消息重复扫描。
- 重启服务后不会重复消费同一批消息。

截至 2026-08-21，这一节仍不能判定完成。已有单测覆盖 attention 写入、cursor、wakeup policy、runtime-node wakeup 入队后标记 seen，但还缺三类真实回归：

- 迁移后 workspace 的真实 Feishu / Lark 私聊、群聊 @、卡片点击端到端。
- 真实 runtime node 执行一次模型成功返回，并自动把 run 标记为 `succeeded`。
- 服务重启后，connector cursor 与 attention seen 状态不会导致同一批 IM / card / inbox 信号重复注入或重复回复。scheduler 启动锁只能防止同 scope scheduler 进程重复运行，不能替代真实 IM connector cursor 回归。

### 7. 本地迁移与 E2E 验证记录

2026-08-21 已在本地迁移后的 Spaceship workspace 做过一轮 2.x smoke：

- 迁移输入：`/root/code/spaceship/spaceship`。
- 迁移输出：`/root/code/spaceship/multigent_e2e/6bbcd4cb-f08b-4268-8f93-926e5939eb59`。
- 备份：`/root/code/spaceship/migration-backups/multigent_e2e-spaceship-agent-worker-20260820T194252Z.tar.gz`。
- 迁移报告：`/root/code/spaceship/migration-backups/multigent_e2e-spaceship-agent-worker-report-20260820T194252Z.json`。
- 迁移结果：10 个项目、91 个 legacy project agents、88 个 Agent Workers、91 条 agent project memberships、3 条 human project memberships、45 个 active tasks、182 个 archived tasks、209 处 task assignee rewrite、0 warnings。

本地页面和 API smoke：

- `/agents`、`/schedule`、`/projects/cc-connect/members`、`/projects/cc-connect/schedule`、`/projects/cc-connect/tasks`、`/workbench` 均返回 200。
- 工作区级 Agents 页面返回 88 个 Agent Workers。
- `cc-connect` 项目成员页返回 15 条 memberships。
- 重新构建并用最新 `dist/multigent` 重启本地 E2E API 后，核心 API smoke 仍然通过：
  - `/api/v1/workspace` 返回 200。
  - `/api/v1/agents` 返回 88 个 Agent Workers。
  - `/api/v1/projects/cc-connect/memberships` 返回 15 条 memberships。
  - `/api/v1/projects/cc-connect/agents` 返回 membership-backed agent rows，并带 `agentWorkerId / projectMembershipId`。
  - `/api/v1/projects/cc-connect/schedule` 返回 membership-backed schedule rows；抽查 `dev-codex` 已带 `agentWorkerId=aw_530990e713c940fb`、`projectMembershipId=pm_530990e713c940fb` 和 worker primary session。
  - `/api/v1/projects/cc-connect/agents/dev-codex/chat/sessions` 返回 200。
  - `/api/v1/projects/cc-connect/agents/dev-codex/runtime/readiness` 返回 200。
  - `/api/v1/workbench/tasks` 返回 200。

本轮补充的回归测试：

- `TestBuildForAgentIncludesAgentWorkerMembershipContext`：约束 Agent Worker identity、Project Membership prompt、其他项目 membership 摘要、agent 绑定 context material 都进入 context build。
- `TestAgentChatSessionsIncludeWorkerBackedRuntimeRuns`：约束 runtime run 只有 `agent_worker_id / project_membership_id`、没有旧 `agent_id` 时，项目成员 chat sessions/history 仍能查到。
- `TestProjectScheduleAgentsUseMembershipAliases`：约束项目 schedule 列表使用 Project Membership 的项目内别名，而不是 worker 全局内部名或 legacy 目录名。
- `TestSetModelUpdatesAgentWorkerFromProjectMemberRoute`：约束从项目成员入口修改 agent CLI 类型时写入 Agent Worker。
- `TestPutAgentEnvUpdatesAgentWorkerModelAccountFromProjectMemberRoute`：约束从项目成员入口保存模型账号和 runtime model 时写入 Agent Worker。
- `TestAgentEnvCLIUsesAgentWorkerRuntimeConfig`：约束 CLI `agent set-env/unset-env/list-env` 在迁移后 workspace 中读写 Agent Worker runtime config，而不是旧项目 agent meta。
- `TestRuntimeTasksListUsesProjectMemberships`：约束 runtime task API 使用 Project Membership 枚举项目 agent，而不是只依赖旧 `projects/<project>/agents` 目录。
- `TestGetAgentContextRendersAgentWorkerMembershipContext`：约束项目成员 context API 使用 Agent Worker + Project Membership 当前定义实时合成 prompt，而不是只读旧 agent 目录里的静态 context 文件。
- `TestPutAgentWakeupUsesAgentWorkerSchedule`：约束项目成员入口保存 wakeup prompt 时写入 Agent Worker `schedule_json.wakeupPrompt`，并能被 context API 读回。
- `TestProjectTasksListUsesProjectMemberships` / `TestProjectMessagesListUsesProjectMemberships` / `TestStatsUsesProjectMemberships`：约束 Web 项目任务、项目消息和首页 stats 使用 Project Membership 枚举项目 agent。
- `TestBillingUsageCountsAgentWorkersOnceAcrossProjects`：约束计费用量按 workspace Agent Worker 计数，同一个 Agent Worker 加入多个项目不重复计费。
- `TestFindTaskInProjectUsesProjectMemberships`：约束 workflow / scheduler / review 共用任务查找路径可以找到 worker-only membership 的任务。
- `TestRuntimeContactsListProjectMembershipAgents`：约束 runtime contacts 通过 Project Membership 枚举同项目 agent。
- `TestClearDeletedModelProviderRefsClearsAgentWorkers`：约束删除模型账号时同步清理 Agent Worker 默认模型账号引用。
- `TestAgentEnvCRUDUsesAgentWorkerRuntimeConfig`：约束 worker-backed 项目成员的 env GET/POST/DELETE 使用 Agent Worker runtime config。
- `TestPutAgentSandboxUsesAgentWorkerRuntimeConfig`：约束 worker-backed 项目成员的 sandbox/addDirs 保存到 Agent Worker runtime config，并能被 `agentMetaForProjectMember` 合成到运行 meta。
- `TestRecordIMAttentionSignalPersistsSignalAndCursor`：约束 IM message 写入 attention signal 时同步推进 cursor，同一 message 重放不会生成重复 pending signal。
- `TestRuntimeNodeCompleteMarksNonWorkflowTaskDone`：约束 runtime node 调 `/complete` 后，非 workflow 任务会归档为 `done_success`，run 标记为 `succeeded`，summary 和 runtime session 写回 heartbeat。
- `TestAcquireSchedulerStartLockRejectsLiveProcess`：约束同一个 workspace root / project / agent scope 不能同时启动两个 scheduler 进程。
- `TestAcquireSchedulerStartLockReplacesStaleLock`：约束 scheduler lock 中记录的 PID 已不存在时，可以清理 stale lock 并重新启动。
  - `/api/v1/projects/cc-connect/schedule` 返回 15 条 schedule rows。
  - `/api/v1/projects/cc-connect/tasks?scope=all` 返回 22 条 task rows。
  - `/api/v1/workbench/tasks` 返回 18 条 workbench task rows。
  - Web 路由 `/agents`、`/schedule`、`/projects/cc-connect/members`、`/projects/cc-connect/schedule`、`/projects/cc-connect/tasks`、`/workbench` 均返回 HTML。
- 2026-08-21 再次执行 `make build` 后重启 E2E API，核心 API smoke 仍然通过；当前本地 E2E 进程包括：
  - API：`/root/code/spaceship/multigent/dist/multigent --dir /root/code/spaceship/multigent_e2e api serve --addr 0.0.0.0:27893`
  - runtime node：`/root/code/spaceship/multigent/dist/multigent --dir /root/code/spaceship/multigent_e2e runtime start --concurrency 2 ...`
  - cc-connect scheduler：`/root/code/spaceship/multigent/dist/multigent --dir /root/code/spaceship/multigent_e2e/6bbcd4cb-f08b-4268-8f93-926e5939eb59 scheduler start --project cc-connect`
- 2026-08-21 补充执行 `go test ./...`、`go test ./internal/api -run 'TestGetAgentContextRendersAgentWorkerMembershipContext|TestRuntimeTasksListUsesProjectMemberships'`、`npm run build`（在 `web/` 下）以及 `go build -o dist/multigent ./cmd/multigent && go build -o dist/mga ./cmd/mga`，均通过。随后重启本地 E2E API，并使用最新二进制对迁移后 workspace 做核心 API smoke：
  - `/api/v1/workspace` 返回 200。
  - `/api/v1/stats` 返回 200。
  - `/api/v1/billing/entitlements` 返回 200，usage 中 `agents=88`，与 workspace Agent Workers 数一致。
  - `/api/v1/agents` 返回 200。
  - `/api/v1/projects/cc-connect/memberships` 返回 200。
  - `/api/v1/projects/cc-connect/agents` 返回 200。
  - `/api/v1/projects/cc-connect/schedule` 返回 200。
  - `/api/v1/projects/cc-connect/tasks?scope=all` 返回 200。
  - `/api/v1/projects/cc-connect/messages?archived=all` 返回 200。
  - `/api/v1/projects/cc-connect/agents/dev-codex/chat/sessions` 返回 200。
  - `/api/v1/projects/cc-connect/agents/dev-codex/runtime/readiness` 返回 200。
  - `/api/v1/projects/cc-connect/agents/dev-codex/context?includeReadiness=false` 返回 200，`model=codex`，合成 context 长度约 35k。
  - `/api/v1/workbench/tasks` 返回 200。
  - 抽查 `cc-connect/dev-codex` schedule row 仍带 `agentWorkerId=aw_530990e713c940fb`、`projectMembershipId=pm_530990e713c940fb`。

任务到 attention 的闭环 smoke：

- 使用 `cc-connect/dev-codex` 创建临时任务 `t-20260820-x7h5vh`。
- 任务写入后自动标注：
  - `assigneeType=agent_worker`
  - `assigneeId=aw_530990e713c940fb`
  - `assigneeMembershipId=pm_530990e713c940fb`
- 创建任务后写入 `AttentionSignal asig-25713ad572456077db854cdc`，reason 为 `task_assigned`，初始状态为 `pending`。
- 为 `cc-connect/dev-codex` 签发 runtime token 后，token response 正确返回 `agentWorkerId` 和 `membershipId`。
- runtime `GET /api/v1/runtime/attention?status=pending` 能读到该 signal，并自动把状态推进到 `seen`。
- runtime `PATCH /api/v1/runtime/attention/{id}` 能把 signal 标记为 `handled`。
- Web attention API 能读到 handled 状态。
- 临时任务最终已取消，避免干扰真实队列。

`mga` CLI 路径也已验证：

- 使用 `cc-connect/dev-codex` 再创建临时任务 `t-20260820-9cofna`。
- 用该 agent 的 runtime token 设置 `MULTIGENT_API_URL` 和 `MULTIGENT_AGENT_TOKEN`。
- `mga attention list --status pending` 能读到 `asig-5d2a894af1dfa5ece378f58c`，并由 runtime API 自动推进为 `seen`。
- `mga attention mark asig-5d2a894af1dfa5ece378f58c --status handled` 能把 signal 标记为 `handled`。
- 临时任务最终已取消。

workflow 节点流转路径也已验证：

- 创建临时两节点 workflow `wf-qpepcai5`：`dev-codex` 节点完成后流转到 `qa-codex`。
- 创建临时 workflow task `t-20260820-a01ww9`。
- 用 `dev-codex` 的 runtime token 执行 `mga task step done t-20260820-a01ww9 --agent dev-codex ...`。
- 任务成功从 `cc-connect/dev-codex` 流转到 `cc-connect/qa-codex`，并保持 `status=pending`，没有被真实模型自动抢跑。
- workflow run 的 2.x 当前负责人正确变成：
  - `currentAssigneeType=agent_worker`
  - `currentAssigneeId=aw_0a7feb395db21623`
  - `currentAssigneeMembershipId=pm_0a7feb395db21623`
- 目标 Agent Worker 写入 `AttentionSignal asig-4a6a238bf64ab2592e16180c`，reason 为 `workflow_step_assigned`。
- 用 `qa-codex` 的 runtime token 能把该 signal 标记为 `handled`。
- 临时 task 已取消，临时 workflow definition 已删除。

runtime-node 手动 wakeup 路径已做真实验证：

- 复用本地 E2E runtime node `rtn_b352f3c23711b294`，重新签发 join token 并执行 `multigent runtime join --server http://127.0.0.1:27893 --skip-prepare`，节点恢复在线。
- 绑定 `cc-connect/dev-codex` 的 Agent Worker `aw_530990e713c940fb` 到该 runtime node，并插入临时 `im_mention` AttentionSignal。
- 离线节点边界已验证：如果绑定的 runtime node 不在线，`POST /api/v1/scheduler/wakeup` 返回 409 `runtime_not_ready`，不会静默回退到本机 Docker 执行。
- 在线节点路径已验证：`POST /api/v1/scheduler/wakeup` 返回 `queued`，创建 `RuntimeRun rtrun_a858e71c15a2cbc4` 和 wakeup task `t-20260820-b79a7b`；run 正确携带：
  - `desiredRuntimeNodeId=rtn_b352f3c23711b294`
  - `runtimeNodeId=rtn_b352f3c23711b294`（claim 后）
  - `agentWorkerId=aw_530990e713c940fb`
  - `projectMembershipId=pm_530990e713c940fb`
- runtime node `start --once` 成功 heartbeat、claim、fetch spec 并启动 sandbox；Docker 环境里已注入 `MULTIGENT_AGENT_WORKER_ID=aw_530990e713c940fb`、`MULTIGENT_PROJECT_MEMBERSHIP_ID=pm_530990e713c940fb`、`MULTIGENT_AGENT_TOKEN`、`MULTIGENT_API_URL`、`mga` 等运行时变量和工具。
- 该次真实执行卡在 Codex SDK 网络请求重连，最终由测试手动标记 `e2e_interrupted`。因此本轮确认的是 runtime-node queue / claim / spec / sandbox injection 链路，尚未证明模型成功完成并自动回传 `succeeded`。
- 临时 run 已标记 failed，临时 task 已取消，Agent Worker 的默认运行节点和 schedule 已恢复。

仍未覆盖的真实 E2E：

- IM 私聊、群聊 @、卡片点击写入 AttentionSignal 并触发 worker wakeup。目前已有 `internal/api/agent_channel_events_test.go` 覆盖 signal/cursor/wakeup policy 的代码路径，但还没有用真实 Feishu/Lark 长连接跑迁移后 workspace 的端到端回归。
- runtime-node scheduler 在单测中已覆盖 idle wakeup 注入 pending attention、已有 pending task 时不注入、入队成功后标记 seen；runtime node `/complete` 服务端回写路径已覆盖非 workflow 任务成功归档和 run `succeeded`。真实运行节点已经验证到 queue / claim / spec / sandbox injection，但还没有完成一次真实模型成功返回并自动 `succeeded` 的端到端回归。
- 服务重启后，connector cursor 与 attention seen 状态不会导致重复消费。

2026-08-21 最新收敛：

- IM 私聊、群聊 @、交互卡片点击不再走 IM 专属 runner 作为主路径。
- connector handler 的职责收敛为三件事：认证/解析外部事件、写入 `AttentionSignal`、按 Agent Worker 的心跳策略请求通用 wakeup。
- 通用 wakeup 不直接拼某个 IM prompt，而是先确保目标 agent 队列里存在一个高优先级普通任务：
  - `type=wakeup`
  - `created_by=heartbeat:attention`
  - `priority=0`
  - `title=[wakeup] attention`
- scheduler/runtime node 仍然只从任务队列选择要执行的 task。这样 attention 与普通 task、workflow task 共享同一条执行入口，不会再出现“IM handler 绕过任务队列直接运行 agent”的第二套机制。
- 如果 agent 正在运行，系统只保留 attention wakeup task，不启动第二个并发 run；下一轮调度时由 agent 自主决定是否处理、忽略或延后。
- attention wakeup task 的 prompt 明确要求 agent 优先使用 `mga attention list --status pending` 读取最新队列，避免只依赖创建 task 时的快照。
- 已补单测约束：存在普通 pending task 时，attention wakeup task 会以更高优先级被选中；已有 attention wakeup task 时不会重复创建。

2026-08-21 `github-sandbox` E2E 覆盖：

- 测试 workspace：`6bbcd4cb-f08b-4268-8f93-926e5939eb59`，项目：`github-sandbox`，agent：`pm`，Agent Worker：`aw_2241f9ccf6cdc160`。
- 前置状态：`github-sandbox/pm` 已有普通 pending task `t-20260816-tyx5h9`，用于验证 attention 不会被普通任务抢占。
- Case 1：临时绑定过期 runtime node 后调用 wakeup，返回 `runtime_not_ready`，确认离线节点不会静默回退到本机 Docker 或直接执行。
- Case 2：刷新 runtime node last_seen 后再次调用 wakeup，返回 `queued`，创建 runtime run `rtrun_248cd6f7e9375f63` 和 task `t-20260821-bc4d9f`。
- 验证结果：
  - 新 task 为 `[wakeup] attention`。
  - `type=wakeup`，`created_by=heartbeat:attention`，`priority=0`。
  - prompt 包含 `Attention Signals`、signal id、source/reason/payload/refs，以及 `mga attention list --status pending` 指引。
  - 原普通 pending task `t-20260816-tyx5h9` 保持 `pending`，没有被本次 wakeup 选中。
  - attention signal `asig-e2e-attention-1787279083` 从 `pending` 标记为 `seen`。
  - runtime run 正确携带 `agent_worker_id=aw_2241f9ccf6cdc160` 和 `desired_runtime_node_id=rtn_b352f3c23711b294`。
- 清理结果：临时 task 已取消，临时 runtime run 已标记 `cancelled/e2e_cleanup`，`github-sandbox/pm` 默认 runtime node 已恢复为空。

多 workspace / SaaS trusted proxy 路径：

- 已修复后端 workspace 切换只读取 `X-Multigent-Workspace-ID` 的问题。SaaS trusted proxy 请求认证通过后，如果没有普通 workspace header，会读取签名内的 `X-Multigent-Proxy-Workspace-ID` 并在进入 handler 前切到对应 workspace root。
- 已补 `TestTrustedProxyWorkspaceHeaderSwitchesWorkspaceRoot`，覆盖“服务当前 root 是 workspace A，但 trusted proxy 请求要求 workspace B”的路径。
- 这条约束对 Agent Worker / Attention / Schedule 很关键：同名 worker、跨项目 membership 和 runtime node 配置都必须落在请求所属 workspace，不能依赖服务当前 root。

构建和回归：

- `go test ./...` 通过。
- `go test ./internal/api -run 'TestPatchProjectAgentUpdatesMembershipBackedWorker|TestProjectAgentsEndpointReadsAgentWorkerMemberships|TestValidateIdentityResolvesAgentWorkerMembership' -count=1` 通过。
- `go test ./internal/agentdir -count=1` 通过。
- `go test ./internal/api -run 'TestRuntimeWakeup' -count=1` 通过。
- `go test ./internal/api ./internal/db ./cmd/multigent ./cmd/mga ./internal/workflow ./internal/taskstore ./internal/runtimeauth` 通过。
- `cd web && npm exec -- tsc -b` 通过。
- `cd web && npm exec -- vite build` 通过。
- `make build` 通过，生成 `dist/multigent` 和 `dist/mga`。
- 最新一次 `make build` 后已重启本地 E2E API，并重新确认 `/api/v1/workspace`、`/api/v1/agents`、`/api/v1/projects/cc-connect/memberships`、`/api/v1/projects/cc-connect/schedule`、`/api/v1/projects/cc-connect/tasks?scope=all`、`/api/v1/workbench/tasks` 均返回 200。
- 2026-08-21 最新 smoke：重启 E2E API 后重新登录获取本地 JWT，以上 6 个核心 API 均返回 200；Vite 27894 下 `/agents`、`/schedule`、`/projects/cc-connect/members`、`/projects/cc-connect/schedule`、`/projects/cc-connect/tasks`、`/workbench` 均返回 200。
- 2026-08-21 再次重启验证时发现 `cc-connect` 存在一个 API 子进程 scheduler 和一个孤儿 scheduler 并存的问题；清理旧进程并用 scheduler 启动锁版本重启后，进程收敛为 1 个 API + 1 个 `cc-connect` scheduler。手动再次执行同 scope `scheduler start --project cc-connect` 会快速失败并返回 `scheduler already running for project cc-connect`。
- 2026-08-21 追加 smoke：重启后重新登录获取 JWT，`/api/v1/workspace`、`/api/v1/agents`、`/api/v1/projects/cc-connect/memberships`、`/api/v1/projects/cc-connect/schedule`、`/api/v1/projects/cc-connect/agents/dev-codex/context?includeReadiness=false` 均返回 200。
- 2026-08-21 追加真实 runtime-node 尝试：
  - 启动本机 runtime node daemon 后，`rtn_b352f3c23711b294` 从 offline 恢复为 online。
  - 临时把 `cc-connect/dev-codex` 对应 Agent Worker `aw_530990e713c940fb` 绑定到该 runtime node。
  - 创建临时任务 `t-20260820-xbmy67`，通过 Web/API `POST /api/v1/projects/cc-connect/tasks/t-20260820-xbmy67/start` 启动。
  - API 返回 `status=queued`、`runtimeRunId=rtrun_18944495b304718d`，确认这次不是本地 Docker pid 路径，而是 runtime-node queue 路径。
  - runtime node 日志显示成功 claim 并启动 run：`runtime run claimed` / `runtime run started`。
  - Docker 容器成功进入 Codex CLI，但真实模型调用卡在网络传输层：先出现 `Falling back from WebSockets to HTTPS transport. request timed out`，随后 `Reconnecting... waiting for network (Connection failed: error sending request)`。
  - 等待约 5 分钟后任务仍为 `in_progress`，run 仍为 `running`；为避免占用 runtime node，手动取消临时任务、清理容器、把 run 标记为 `failed/e2e_codex_network_timeout`，并恢复 Agent Worker 默认 runtime node 为空。
  - 结论：runtime-node 的 queue / claim / lease / sandbox 启动链路再次得到真实验证，但“真实模型成功返回并自动 `succeeded`”仍未通过，当前阻塞点是 Codex 网络调用。
- 已知未处理项：`npm install` 报告 6 个 audit warnings（1 moderate，5 high），本轮未展开依赖治理。

## 成功标准

重构完成后，应满足：

- 一个 agent 可以自然参与多个项目。
- 用户可以私聊 agent，也可以在群里 @agent。
- agent 能知道自己被谁、在哪个渠道、为了什么事情找了。
- 未 @agent 的普通群消息不会打爆 agent 上下文。
- workflow 仍然可控，但 agent 不再像节点函数。
- task / workflow / task template 的用户心智保持不变。
- IM 卡片、文本消息、Web 对话都进入同一套 agent interaction 和 attention 模型。

## 一句话总结

Multigent 2.x 不需要造更多任务概念；它需要把 agent 的长期身份从项目成员里抽出来，同时保留“每次执行必须落在项目上下文里”的边界，再用 Attention Signal 把任务、流程、IM 和外部世界接到这个主体的注意力系统上。
