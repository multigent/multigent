# 灵活流程入口与任务 Spec 化设计

日期：2026-08-27

## 背景

Multigent 2.x 已经把 Agent 从项目成员中抽离为 workspace 级协作主体，并通过 Attention Signal、IM 协作渠道、任务和流程让 Agent 可以像同事一样持续工作。

但真实团队协作里，一个更核心的问题开始暴露出来：

> 难点不是 Agent 能不能异步执行，而是模糊的人类意图如何被转成可追踪、可验证、可交接的工作单元。

例如产品同事直接在飞书里对开发 Agent 说“帮我把这个功能改一下”。如果开发 Agent 直接开干，很容易出现：

- 产品表达不符合开发术语。
- 背景、目标、非目标和验收标准不清楚。
- Agent 不知道该遵循哪些开发规范。
- 没有任务、流程、审计和验收记录。
- 后续 QA、人类确认、发布无法自然接上。

所以我们需要重新定义“聊天、任务、Spec、流程”的关系。

## 核心判断

### 聊天是交互通道，不天然等于新任务

IM 私聊、群聊 @、Web Chat、站内消息都只是交互通道。它们会成为 Attention Signal，但不应该天然等于“新任务”。

同一条聊天可能是不同性质的事情：

- **咨询 / 拉齐认知**：用户只是问 Agent 怎么看、了解多少、建议什么；不需要建任务。
- **当前任务补充信息**：Agent 已经在做某个任务，用户补充约束、指出问题、提供截图或反馈；应该关联到当前任务，不一定新建任务。
- **当前任务决策**：用户同意、拒绝、打回、改验收标准；必须写入任务 / 流程 / 审计，不能只停留在聊天里。
- **任务变更请求**：用户提出的新要求改变了范围、风险、架构或验收；应该更新 Spec 或回到上游节点，而不是开发 Agent 静默吸收。
- **新任务意图**：用户要求 Agent 去做一件新的可交付工作；应该创建任务草稿或正式任务。

所以更准确的判断是：

> 聊天是人与 Agent 的自然协作界面；任务是需要被追踪和验收的工作承诺。

Agent 可以在聊天中理解、澄清、咨询和吸收上下文。但只有当事情变成可追踪承诺时，才必须进入任务。

### 任务是承诺

一旦要动代码、动外部系统、发消息、影响线上、产生不可逆结果，就必须进入任务。

任务至少提供：

- 负责人。
- 所属项目。
- 当前状态。
- 输入上下文。
- 执行记录。
- 审计记录。
- 是否绑定流程。

### 任务边界如何判断

不是所有对话都要建任务。任务的边界应该由“是否形成工作承诺”决定。

建议第一版用这些判断标准：

- 会修改代码、配置、数据、权限或外部系统。
- 会产生对外可见交付物，例如 PR、发布、文章、客户回复、运营动作。
- 需要 QA、Review、发布、回滚或人类确认。
- 有明确 owner、优先级、验收标准或截止时间。
- 需要跨人、跨 Agent、跨项目协作。
- 需要未来跟进，不能只靠某个会话记忆。
- 涉及资金、安全、权限、客户承诺或不可逆动作。

如果只是解释、讨论、问状态、拉齐认知、临时建议，通常不需要建任务。但如果讨论产生了明确行动项，Agent 应该主动提出把它沉淀为任务。

### Spec 是交接边界

任务不是一句话需求。真正能让 Agent 或人类稳定协作的是结构化 Spec。

最小 Spec 应包含：

- 背景。
- 问题陈述。
- 目标。
- 非目标。
- 影响范围。
- 验收标准。
- 优先级。
- 相关链接、文档、截图或上下文。
- 是否需要技术方案。
- 是否需要人类确认。

### 流程是协作协议，不是固定流水线

流程不是 n8n 式代码流，也不是必须从第一个节点跑到最后一个节点的单一起点流水线。

流程定义的是一组阶段、输入契约、输出契约和协作规范。

Agent 在流程中仍然有主体性：

- 可以多次 wakeup 才完成一个节点。
- 可以在节点中向人类或其他 Agent 澄清。
- 可以判断当前信息不足，回退到补 Spec 或技术方案节点。
- 可以在确认满足完成条件后主动推进流程。

流程也不是所有沟通的唯一入口。用户可以直接找正在执行任务的 Agent 补充信息。区别在于：

- 小范围实现细节补充：Agent 可以直接吸收，记录到任务日志，继续执行。
- 影响验收、范围、架构、安全或发布时间：Agent 应该把它转成任务变更或流程决策。
- 明确 approve / reject / request changes：必须推进流程状态或写审计。

这能避免把流程做成僵硬审批系统，同时保留必要的可追踪性。

### “像人”不是“无边界自由”

把 Agent 当协作对象，是为了让它具备主动性、判断力、沟通能力和持续工作能力，而不是让它拥有无限权限。

正确理解：

- Agent 可以像同事一样交流、追问、总结、判断优先级。
- Agent 可以自己认领任务、异步推进、有阻塞时找人确认。
- Agent 可以在 IM 里参与讨论，但不代表任何人都能随意给它派活。
- Agent 的执行权限仍然由 Multigent 的任务、流程、RBAC、外部工具授权和审计约束。
- Agent 应该知道“谁在说话、对方有什么权限、这件事是否需要记录为任务”。

因此，IM 接入不是把 Agent 变成一个随叫随到的聊天机器人，而是给协作对象增加一个更自然的工作界面。

## 不应该做什么

### 不为每个入口创建一套流程

不要创建这些重复流程：

- 产品 -> 开发 -> 测试。
- 开发 -> 测试。
- 测试。

入口组合会持续增长，如果每种入口都复制一套流程，流程模板会爆炸，并且后续维护成本很高。

### 不让开发 Agent 无任务开工

开发 Agent 可以自由聊天和澄清，但不应该在没有任务和 Spec 的情况下直接改代码。

它应该能够识别：

> 这是一个开发诉求，但当前还不是一个合格任务。

然后引导用户补充信息，或创建任务草稿。

### 不把所有反馈都强制打回流程

如果 Agent 正在处理任务，人类直接告诉它“这里实现有问题”“这个边界要注意”“我刚补了截图”，这不应该一律要求用户去 human_review 节点打回。

更合理的规则：

- 当前任务范围内的补充信息：直接关联到任务，Agent 吸收后继续。
- 当前任务范围内的小修正：Agent 可以直接改，并在任务日志说明采纳了什么。
- 改变任务目标、验收标准、风险或架构方向：Agent 应创建变更记录，并请求确认或流转回对应节点。
- 人类明确打回或审批：必须走流程状态变更。

否则流程会变成人类协作的摩擦，而不是协作协议。

### 不强制所有用户只找一个派发 Agent

可以有 PM / Intake / Dispatcher Agent 作为默认入口，但不应该要求所有人只能找它。

真实使用里，人会直接找熟悉的开发 Agent、QA Agent 或项目负责人。系统应该允许这种自然入口，但要求 Agent 把诉求转成规范任务后再执行。

## 目标模型

### 一个流程，多个入口

同一个「通用研发流程」可以包含完整阶段：

```text
需求澄清 / Spec 准备
  -> 技术方案
  -> 技术方案人工审核
  -> 开发实现
  -> 自测
  -> QA 验证
  -> Human Gate
  -> 发布 / 归档
```

但任务可以从不同节点开始：

```text
产品提需求
  -> 从「需求澄清」开始

产品直接找开发 Agent
  -> 开发 Agent 检查 Spec 是否足够
  -> 不足则回到「需求澄清 / Spec 准备」
  -> 足够则从「开发实现」开始

开发同事发现问题
  -> 从「技术方案」或「开发实现」开始

测试同事报 Bug
  -> 从「复现 / 分诊」或「开发实现」开始

已有 PR
  -> 从「QA 验证」开始
```

### 节点是阶段，不是函数调用

每个流程节点需要定义：

- 节点 ID。
- 节点类型：agent、human、review、parallel、summary 等。
- 负责人或候选负责人。
- 必填输入。
- 可选输入。
- 完成条件。
- 输出字段。
- 缺失输入时的回退节点。
- 允许作为入口节点。

示例：

```yaml
node: development
name: 开发实现
entry_allowed: true
required_inputs:
  - problem_statement
  - acceptance_criteria
  - scope
optional_inputs:
  - design_doc_id
  - priority
  - related_links
if_missing:
  route_to: spec_clarification
completion:
  - implementation_summary
  - test_evidence
  - pr_url
next:
  - qa_review
```

这样即使任务从开发节点开始，也不会跳过 Spec 要求。节点可以自己判断输入是否满足。

## 关键用户路径

### 路径 A：用户找 PM / Intake Agent

1. 用户在飞书或 Web 中说一个模糊需求。
2. PM Agent 判断是否值得进入任务。
3. 信息不足时反问。
4. 信息足够时生成 Spec。
5. 创建任务草稿。
6. 根据任务类型推荐流程和起始节点。
7. 需要人类确认时发卡片或 Web 确认。
8. 任务进入流程执行。

### 路径 B：用户直接找开发 Agent

1. 用户直接对开发 Agent 说“帮我改 X”。
2. 开发 Agent 识别这是潜在开发任务。
3. 开发 Agent 不直接开工，而是检查开发节点的 required inputs。
4. 如果缺信息，开发 Agent 在原会话中澄清。
5. 如果用户补齐，开发 Agent 创建任务草稿或正式任务。
6. 如果任务适合直接开发，从「开发实现」作为起始节点进入流程。
7. 如果任务复杂，先路由到「技术方案」或「需求澄清」。

### 路径 C：已有任务或 PR 从中间节点进入

1. 用户提供已有 PR、Issue 或测试反馈。
2. Agent 判断已经不需要需求澄清。
3. 创建任务并选择中间入口，如 QA 验证、Human Gate。
4. 后续仍遵循流程输出规范和审计。

### 路径 D：用户补充正在执行的任务

1. Agent 正在执行任务，例如开发一个 PR。
2. 用户在 IM 里直接告诉 Agent：“刚看了下，这里权限边界不能这么做。”
3. Agent 判断这条消息是否能关联到当前任务：
   - 用户回复了任务卡片。
   - 消息包含 task / PR / issue / doc 链接。
   - 当前会话最近正在讨论同一任务。
   - Agent 仍不确定时，应该反问“你是指任务 X 吗？”
4. 如果是当前任务补充，Agent 把消息记录为任务 note / attention event。
5. 如果是小范围修正，Agent 继续改。
6. 如果影响方案或验收，Agent 更新 Spec 并请求确认，必要时回退到方案审核节点。

### 路径 E：用户只是找 Agent 聊

1. 用户问 Agent 对某个项目、技术方案、历史背景或业务判断怎么看。
2. Agent 直接回答，不创建任务。
3. 如果讨论中出现明确行动项，Agent 应提醒：

> 这已经像一个需要追踪的工作项了。我可以把它整理成任务草稿。

4. 用户确认后才进入任务或流程。

## 产品能力设计

### 任务草稿

增加 `draft task` 或 `proposed task` 能力。

状态建议：

- `draft`：Agent 或人类正在整理，未进入执行队列。
- `pending_confirmation`：Spec 已足够，等待人类或 PM 确认。
- `pending`：确认后进入执行队列。
- `in_progress`：Agent 正在处理。
- 既有终态保持不变。

草稿任务可以反复补充 Spec，不触发开发。

### 任务 Spec 字段

任务应支持结构化 `spec_json`，而不是只靠 prompt 文本。

第一版字段：

```json
{
  "problemStatement": "",
  "background": "",
  "goals": [],
  "nonGoals": [],
  "scope": "",
  "acceptanceCriteria": [],
  "priority": "",
  "riskLevel": "",
  "relatedLinks": [],
  "contextDocIds": [],
  "needsDesignReview": false,
  "needsHumanApproval": false
}
```

`prompt` 仍然存在，但 Agent 运行时应优先读取结构化 Spec，并把 prompt 当作原始补充材料。

### 流程起始节点

创建任务时允许选择：

- workflow template。
- start node。
- start reason。
- initial assignee。

例如：

```json
{
  "workflowId": "general-engineering",
  "startNodeId": "development",
  "startReason": "user_directly_requested_dev_agent"
}
```

如果 start node 缺少 required inputs，系统或 Agent 应提示补齐，而不是强行执行。

### 节点输入契约

流程定义中需要增加节点输入契约：

- `required_inputs`。
- `optional_inputs`。
- `if_missing.route_to`。
- `entry_allowed`。

第一版不一定做强类型校验，可以先用 schema + prompt 约束：

- UI 展示当前节点需要哪些输入。
- Agent wakeup prompt 注入当前节点输入契约。
- Agent 自行判断是否满足。
- 后续再把必填字段做成服务端校验。

### Agent 收到任务意图时的行为规则

在 Agent profile / role prompt / wakeup prompt 中沉淀通用规则：

```text
当你从 IM、Web Chat 或站内消息收到开发、测试、发布、外发等执行诉求时：

1. 先判断这是聊天、咨询、任务意图，还是紧急问题。
2. 如果只是咨询，可以直接回答。
3. 如果是任务意图，不要在没有任务记录的情况下直接执行。
4. 检查是否有足够 Spec：背景、目标、范围、验收标准、优先级。
5. 不足时向发起人澄清。
6. 足够时创建任务草稿或正式任务，并选择合适流程和起始节点。
7. 只有任务进入可执行状态后，才开始开发、测试、发布等动作。
```

补充规则：

```text
当你收到消息时，不要默认把它当成新任务。

先判断它属于：
1. 咨询 / 拉齐认知。
2. 当前任务补充。
3. 当前任务审批或打回。
4. 当前任务变更。
5. 新任务意图。

如果它是当前任务补充：
- 尝试根据 task id、PR、Issue、文档、卡片回复、最近会话上下文关联到具体任务。
- 能明确关联时，把它记录到任务上下文并继续处理。
- 不能明确关联时，先反问，不要猜。

如果它是审批、打回、范围变化、验收变化或高风险决策：
- 必须写入任务 / 流程 / 审计。
- 不要只在 IM 里口头记住。
```

### `mga task` 能力

需要给 Agent 提供更好的 CLI/API：

```bash
mga task draft create \
  --project <project> \
  --title "..." \
  --spec-file spec.json \
  --workflow general-engineering \
  --start-node development

mga task draft update <task-id> --spec-file spec.json

mga task draft submit <task-id>

mga task spec show <task-id>

mga workflow inspect-node \
  --workflow general-engineering \
  --node development
```

这让 Agent 可以自己判断、补齐和提交任务，而不是只能被动执行已有任务。

## UI 设计

### 创建任务弹框

新增高级但不复杂的区域：

- 任务类型。
- 推荐流程。
- 起始阶段。
- Spec 完整度。

起始阶段默认由系统根据任务类型推荐：

- 需求不清楚：需求澄清。
- 已有明确需求：技术方案或开发实现。
- 已有 PR：QA 验证。
- 只需要人确认：Human Gate。

### 流程编辑器

节点配置增加：

- 允许作为起始节点。
- 必填输入。
- 缺失输入时回退节点。
- 节点完成条件。

### 跟随页面

展示当前节点的“输入契约完成度”：

- 已满足字段。
- 缺失字段。
- Agent 为什么认为可以继续或需要回退。

### IM 体验

当 Agent 收到任务意图但信息不足时，应自然回复：

> 这看起来是一个开发需求。我不会直接开工，先帮你补成可执行任务。还缺 3 个信息：验收标准、影响范围、优先级。

当信息足够时：

> 我已经整理成任务草稿，并建议从「开发实现」节点开始。你可以确认后进入流程。

当用户补充当前任务时：

> 收到，我会把这条补充记录到任务 X。它改变了验收标准，所以我会先更新方案并请你确认，不会直接继续开发。

当用户只是咨询时：

> 这个不需要建任务，我先直接给你判断。如果你要我后续推进，我再把它整理成任务。

### 人类协作规范

为了让 Agent 既自然又可控，团队成员也需要一套轻量约定：

- 问问题、拉齐背景、请 Agent 解释：直接聊。
- 补充当前任务：尽量回复任务卡片，或带上 task / PR / Issue / doc 链接。
- 让 Agent 做新事：说清目标、验收、影响范围；Agent 会帮忙补 Spec。
- 审批、打回、变更验收：优先用卡片 / Web；在 IM 里说也可以，但 Agent 必须落审计。
- 不要把“随口一说”当成任务指令；如果希望 Agent 开始做，要明确说“请整理成任务”或“可以进入流程”。

## 技术实现计划

### Phase 1：Prompt 与轻量结构

目标：先让 Agent 行为正确，不大改状态机。

工作：

1. 在通用 Agent 行为 prompt 中补充“任务意图识别与不无任务开工”规则。
2. 给 `mga task` 增加 `spec show` / `draft create` 的最小能力。
3. 任务增加 `spec_json` 字段，先不强制所有任务都有。
4. 创建任务 API 支持 `workflowId + startNodeId`。
5. 流程运行支持从指定节点启动。
6. 前端创建任务弹框支持选择起始节点。

验收：

- 用户直接找开发 Agent，Agent 不会直接改代码。
- Agent 可以创建带 Spec 的任务草稿。
- 任务可从开发节点启动，并继续流转到 QA。

### Phase 2：节点输入契约

目标：把“入口校验”变成流程定义的一部分。

工作：

1. Workflow node 增加 `requiredInputs / optionalInputs / ifMissingRouteTo / entryAllowed`。
2. 流程编辑器支持配置。
3. 任务进入节点时注入输入契约。
4. Agent wakeup prompt 明确：未满足输入契约时应补齐或回退。
5. 跟随页面展示输入契约完成度。

验收：

- 从开发节点启动但缺验收标准时，Agent 会请求补充或回退需求澄清。
- 从 QA 节点启动但缺 PR URL 时，Agent 不会假装 review。

### Phase 3：服务端校验与自动推荐

目标：减少纯 prompt 约束的不确定性。

工作：

1. 服务端根据 node required inputs 校验是否允许开始执行。
2. 不满足时任务停在 `needs_spec` 或 `draft`。
3. 基于任务类型和 Spec 完整度推荐起始节点。
4. 支持 Agent 调用 API 获取推荐。

验收：

- 缺关键 Spec 时，服务端不会把任务派到开发执行。
- Agent 和 UI 都能看到推荐原因。

### Phase 4：多入口协作优化

目标：让 IM、Web、任务、流程自然融合。

工作：

1. IM 中检测任务意图并生成任务草稿卡片。
2. 用户可以在 IM 卡片中补充信息或确认创建任务。
3. Agent 可以把聊天上下文链接到任务 Spec。
4. 支持把某段 IM 对话保存为 context artifact。

验收：

- 用户在飞书中自然提需求，Agent 能澄清、生成任务草稿、确认后进入流程。
- 人类无需手动打开 Web 建任务，但 Web 中仍然有完整审计。

## 数据模型草案

### tasks

新增或扩展：

```sql
spec_json TEXT NOT NULL DEFAULT '{}',
workflow_id TEXT NOT NULL DEFAULT '',
workflow_start_node_id TEXT NOT NULL DEFAULT '',
intake_source_kind TEXT NOT NULL DEFAULT '',
intake_source_id TEXT NOT NULL DEFAULT '',
```

状态增加：

```text
draft
needs_spec
pending_confirmation
```

是否要新增状态需要谨慎评估；第一版也可以用现有 `pending / awaiting_confirmation` 加 `spec_status` 过渡。

### workflow node

定义扩展：

```json
{
  "entryAllowed": true,
  "requiredInputs": ["problem_statement", "acceptance_criteria", "scope"],
  "optionalInputs": ["design_doc_id", "priority", "related_links"],
  "ifMissing": {
    "routeTo": "spec_clarification"
  },
  "completionCriteria": ["implementation_summary", "test_evidence", "pr_url"]
}
```

### attention signal

不需要改为任务。Attention Signal 只记录入口和上下文引用：

```json
{
  "intent": "current_task_feedback",
  "confidence": 0.86,
  "suggestedProject": "example-mcp-server",
  "suggestedWorkflow": "general-engineering",
  "suggestedStartNode": "development",
  "refs": {
    "taskId": "t-xxx",
    "workflowRunId": "wfr-xxx",
    "issueUrl": "https://github.com/org/repo/issues/1",
    "prUrl": "",
    "docIds": []
  },
  "actor": {
    "userId": "u_xxx",
    "displayName": "owner-a",
    "roles": ["workspace_admin", "project_owner"]
  },
  "auth": {
    "canComment": true,
    "canApprove": true,
    "canAssignWork": true
  }
}
```

是否创建任务由 Agent 或用户确认决定。

Attention Signal 的重点不是触发器，而是把“发生了什么、谁说的、可能关联什么、对方有什么权限”交给 Agent 判断。

IM 消息只是 Signal 的一种来源。未来 Sentry、Linear、GitHub、飞书文档、会议纪要、竞品新闻、Context Collector 都可以产生 Signal，但它们都不应该直接等价为任务。

## 对现有架构的影响

### 保留现有 task / workflow / task template

不引入新的 WorkItem 概念。

现有 task 仍然是唯一工作单元。变化是：

- task 可以带 Spec。
- task 可以从 workflow 中间节点启动。
- task 可以先是 draft。

### 保留 Agent 主体性

流程不应该把 Agent 变成节点函数。流程只定义协作协议和输入输出契约。

Agent 在节点中仍然可以：

- 澄清。
- 查询上下文。
- 找人确认。
- 创建子任务或 fork session。
- 暂停等待信息。
- 判断是否满足完成条件。

### 与 Attention Signal 解耦

Attention Signal 负责告诉 Agent “有东西值得看”。它不直接创建任务，也不直接推进流程。

Agent 看完信号后，才决定是否：

- 回复。
- 忽略。
- 记录。
- 创建任务草稿。
- 推进已有任务。
- 请求 human gate。

### 与权限和审计结合

IM 里能聊天，不代表能派活；能派活，不代表能审批；能审批，也只代表在某个任务或流程节点上有权限。

第一版需要做到：

- Signal 记录真实来源：用户、渠道、群聊、消息 ID、时间。
- 绑定用户时尽量识别 Multigent 用户身份。
- Agent wakeup 时知道发消息的人是谁、是否已绑定、有什么角色。
- Agent 调用任务 / 流程操作时，服务端按被代表用户或委托 token 做权限校验。
- 所有由 IM 引发的任务创建、Spec 变更、审批、打回都写入审计。

这能让 Agent 像同事一样沟通，但不像无边界脚本一样执行任意指令。

## 效率提升到底来自哪里

Multigent 的效率提升不应该被理解为“每个人多一个聊天机器人”。

真正的杠杆来自：

- **Agent 自主认领和推进任务**：人类不需要持续盯进度。
- **异步执行**：Agent 可以在人类离线时继续调研、开发、测试、整理材料。
- **只在关键节点找人**：方向、风险、验收、外发、资金、安全等需要人类判断时才打扰。
- **IM 缩短澄清链路**：人不用打开系统也能补充上下文、确认、打回。
- **流程保持一致性**：不同 Agent 和人类都遵循同一套 Spec、Review、QA、发布协议。
- **上下文沉淀复用**：Agent 的经验、项目背景、历史决策不只留在某个会话里。
- **权限和审计兜底**：允许自然沟通，但关键动作仍然可追踪、可回滚、可问责。

所以最佳形态不是“人手一个 Agent”，也不是“所有事找一个超级 Agent”，而是：

> 团队拥有一组带职责、权限、上下文和流程意识的 Agent；人类在关键判断上介入，日常推进交给 Agent 异步完成。

## 风险与边界

### 风险：Spec 变成用户负担

解决：

- 用户仍然可以自然语言说话。
- Spec 由 Agent 帮用户整理。
- UI 展示结构化结果，但不要求用户一开始填完整表单。

### 风险：Agent 过度保守

如果 prompt 写得太强，Agent 可能什么都要求建任务。

解决：

- 区分咨询和任务意图。
- 低风险回答、解释、查状态不需要建任务。
- 动代码、发布、外发、资金和权限变更必须建任务。

### 风险：流程入口太自由导致失控

解决：

- 只有 `entryAllowed=true` 的节点能作为起始节点。
- 节点 required inputs 不满足时不能执行或必须回退。
- 所有入口选择写入审计。

### 风险：流程定义复杂

解决：

- 默认模板保持简单。
- 高级字段折叠。
- 第一版只配置 required inputs 和 entryAllowed。

## 建议下一步

1. 先更新通用研发流程模板，明确哪些节点允许作为入口。
2. 给 Agent prompt 增加“任务意图识别，不无任务开工”的规则。
3. 增加任务 `spec_json` 的后端字段和 API。
4. 支持创建任务时指定 workflow start node。
5. 增加“当前任务补充信息”的 Signal 到 Task 关联能力。
6. 增加任务 note / decision / change event 的基础模型。
7. 在本地 `github-sandbox` 做 E2E：
   - 产品用户直接找 Dev Agent。
   - Dev Agent 澄清并创建任务草稿。
   - 信息足够后从开发节点进入流程。
   - Dev 完成后流转 QA。
   - QA 到 Human Gate。
   - 用户在 Dev 执行中通过 IM 补充约束，Agent 正确关联当前任务。
   - 用户在 IM 中提出范围变化，Agent 不静默吸收，而是更新 Spec 并请求确认。
   - 用户只是咨询 Agent 项目背景，Agent 直接回答，不创建任务。

## 一句话总结

Multigent 不应该为每种人类入口复制流程，也不应该让 IM 变成无约束派活通道。它应该把流程升级为“可从多个阶段进入的协作协议”，让聊天成为自然交互界面、任务成为可追踪承诺、Spec 成为交接边界、流程成为人与 Agent 共同遵守的弱规范。
