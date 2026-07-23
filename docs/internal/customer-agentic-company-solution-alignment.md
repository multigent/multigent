# Customer Agentic Company Solution Alignment

本文档用于把 Multigent 的产品开发重新对齐到最初目标：为客户公司提供一套可落地的 Agent 时代协作架构方案，并最终转化为可采购、可试点、可扩展的 SaaS 产品。

参考原始方案：

- 客户 Agentic Company Workflow 原始讨论稿
- `docs/agent-delivery-journey-and-template-system.md`

## 初衷

我们最初不是单纯想做一个 agent 管理工具，而是想给客户公司说明并交付一套新协作方式：

```text
当公司已经开始使用 AI Agent，
真正的效率瓶颈不再是每个人会不会用 Agent，
而是公司有没有一套能让 Agent 稳定接收上下文、执行任务、反馈状态、沉淀经验的协作架构。
```

这套方案要解决的是客户公司里的真实问题：

- 每个人都有自己的 Agent，但 Agent 的上下文分散。
- 公司已有文档和会议纪要，但没有统一的 Agent Context 管理机制。
- 人仍然在反复给 Agent 搬运背景、解释流程、提醒下一步。
- Agent 可以持续 loop，但公司流程仍然围绕人的在线时间运行。
- Agent 做了多少事、失败在哪里、哪些 prompt/skill 有效，缺少统一记录。
- Agent 能执行以后，权限、审计、质量门禁和人工确认必须系统化。

因此，Multigent 的商业价值不是“多一个 Agent 聊天入口”，而是帮客户公司完成从“个人使用 AI”到“公司运营 Agent Workforce”的升级。

## 我们要让客户相信什么

对客户公司的老板和团队来说，文档和产品必须让他们相信三件事。

### 1. 这不是替换现有工具

客户可以继续用飞书、GitHub、GitLab、Linear、Jira、ONES、Notion、本地 Codex、Claude Code 或 Cursor。

Multigent 不应该把自己定位成替代所有系统的颠覆者，而是加一层 Agent 友好的组织、上下文、权限和执行控制面：

```text
已有工具：文档、代码、任务、IM、CI/CD、业务系统
Multigent：Agent 身份、Context、任务入口、权限、调度、运行记录、调教闭环
Agent Runtime：Codex / Claude Code / 其他 CLI，在 sandbox 中执行
```

这会降低客户切入阻力。

### 2. 这不是让人失去控制

客户担心的不是 Agent 不够聪明，而是 Agent 失控、越权、乱改、乱发、不可追溯。

Multigent 必须明确：

- Agent 是有身份的 principal。
- 每个 Agent 有负责人。
- 每个 Agent 只能访问被授权的项目、连接、凭证和技能。
- 关键动作必须人工确认。
- 所有运行、消息、权限使用和失败都要审计。

人不是被移出流程，而是从同步阻塞点变成负责人、审核者、调教者和最终决策者。

### 3. 这可以从一个小流程开始证明 ROI

不要承诺“一次性改造全公司”。对客户最可信的路径是：

```text
选择一个真实但可控的流程
-> 搭建项目 playbook 和 agent 小队
-> 跑 2-4 周
-> 用数据证明人工介入减少、任务处理变快、上下文重复投喂减少、失败可复盘
```

第一版 POC 只要证明一个流程跑通，就足够支撑后续扩展和付费。

## 客户方案与 Multigent 产品能力的映射

| 客户方案诉求 | Multigent 对应能力 | 当前差距 |
|---|---|---|
| 统一 Agent Context 管理 | Workspace / Team / Role / Project Prompt / Skill / Docs | 需要进一步产品化 Project Playbook Template |
| Agent 友好的任务系统 | Task / Message / Run / Scheduler / Wakeup | 需要 Task Template 和项目级任务向导 |
| 人从同步驱动变成异步审核 | Inbox / Confirm Request / Session Lock / External Channels | 需要把 confirm/review 流程在 Web 上做得更自然 |
| Agent 持续 loop | Scheduler / Wakeup / Cron / Run State | 需要模板化 wakeup 和 schedule |
| 权限、审计、隔离 | RBAC / Agent Identity / Sandbox / Audit / Connections Grant | 需要继续补全 agent 权限 UI 和 run 权限解释 |
| 交付结果可复盘 | Run Logs / Tokens / Summary / Audit / Failure Records | 需要 Run Diagnosis 和 Human Intervention Ledger |
| 重复经验沉淀 | Skill / Docs / Prompt / Task Template | 需要从 run/review 一键沉淀的产品入口 |
| 客户低门槛试点 | Team Template / Project Playbook / Agent Ready Checklist | 这是下一阶段最关键产品工作 |

## 当前最大断层

我们现在的产品已经有不少基础模块，但距离客户方案落地还差一个关键中间层：

```text
客户方案讲的是“公司如何协作”；
当前产品更多还是“用户如何创建对象”。
```

要达成方案目的，Multigent 必须把对象创建串成可执行流程：

```text
创建 workspace
-> 选择试点流程
-> 生成 project playbook
-> 创建 agent 小队
-> 配置连接和权限
-> 开始第一个任务
-> 观察 run
-> 调教 agent
-> 沉淀模板
```

也就是说，核心不是再加几个页面，而是让用户知道“下一步该做什么，以及为什么做”。

## 面向客户的试点方案

### 推荐从软件研发流程切入

一个包含 Plugin / Connector 能力建设的研发功能是很好的示例，因为它包含完整链路：

- PM 梳理需求。
- 设计输出 UI。
- 前后端并行开发。
- 接口联调。
- QA 测试。
- Bug 修复。
- 发布准备。
- 复盘沉淀。

这个流程足够真实，也足够可控。它能展示 Multigent 的完整价值，而不是只展示一个 agent 会聊天。

### POC 范围

第一版 POC 不需要全自动。

建议范围：

- 一个 workspace。
- 一个软件研发 project。
- 一个 project playbook。
- 3-5 个 agent：PM、Developer、QA、Release，可选 Designer。
- 一个外部连接：GitHub/GitLab 或飞书。
- 一组任务模板：bug 修复、小需求实现、方案评审、QA 验收、发布检查。
- 一个 wakeup：PM Agent 每日检查任务状态和阻塞。
- 一个 run review 页面：看输出、日志、失败原因、下一步建议。

### POC 成功指标

指标不要只看“跑了多少 Agent”，要看流程有没有变好：

- 50% 以上常规任务可以由 Agent 完成初步处理。
- 人工介入从同步催促变成明确的 confirm/review。
- 每个任务都有结构化输入、输出 summary 和 run log。
- 每周至少沉淀 3 条 prompt/rule/skill/task template 改进。
- 重复 context 投喂次数下降。
- 失败能归因到上下文、权限、工具、prompt、任务过大或模型能力，而不是只剩聊天记录。

这些指标才是客户老板愿意付费的理由。

## 产品上必须补的能力

### 1. Project Playbook Template

这是连接“客户方案”和“产品体验”的核心。

它应该帮助客户把公司流程变成：

- project prompt
- wakeup prompt
- schedule
- task templates
- required connections
- approval rules
- agent role recommendations

没有 Project Playbook，客户会面对空白 prompt，不知道怎么把自己的流程搬进 Multigent。

### 2. Agent Ready Checklist

客户需要知道 agent 为什么还不能稳定干活。

Checklist 应该告诉用户：

- 模型账号是否可用。
- 负责人是否设置。
- Prompt 是否完整。
- Skill 是否绑定。
- 连接和凭证是否授权。
- Wakeup 是否配置。
- 最近 test run 是否成功。

这是小白用户跑通第一条任务的关键。

### 3. Task Template

企业流程落地时，任务输入不能只是一句聊天消息。

至少需要内置：

- Bug 修复模板。
- 小需求实现模板。
- 技术方案模板。
- 代码 Review 模板。
- QA 验收模板。
- 发布检查模板。
- 失败复盘模板。

Task Template 是降低 token 和人工解释成本的直接手段。

### 4. Run Diagnosis

当 agent 失败时，客户不能只看到日志。

系统应该把失败分类：

- 缺上下文。
- 缺权限。
- 缺工具。
- prompt 不清。
- 任务太大。
- 模型不适合。
- 需要人工确认。
- 外部系统失败。

然后给出改进动作。

### 5. Human Intervention Ledger

这是客户方案里的关键思想：人不应该反复处理同类问题。

每次人介入时都应该记录：

- 为什么需要人。
- 是否真的不可自动化。
- 用户补充了什么判断。
- 下次能否沉淀成 prompt、policy、skill 或 task template。

同类介入重复出现，就应该提示沉淀。

## 文档层面的调整建议

原客户方案已经能讲清楚“为什么需要新协作方式”，但如果未来要拿去卖给客户，还需要补三层内容：

### 1. 从 agencycli 更新为 Multigent

原文大量使用 agencycli 和本地工作区表述。现在产品方向已经变化：

- 从本地优先变成 SaaS / workspace。
- 从本地 agent 变成 sandbox agent coworker。
- 从文件上下文变成 DB + docs + skills + project prompt 的统一管理。
- 从 CLI 操作为主变成 Web + agent CLI + 外部 IM 渠道。

所以客户文档应该改成 Multigent，不再强调本地目录和 agencycli。

### 2. 增加“落地试点包”

客户不会只因为理念买单。文档要给出一个具体 2-4 周 POC 包：

- 试点流程。
- 参与角色。
- 要配置的 agent。
- 要接入的工具。
- 交付物。
- 成功指标。
- 风险边界。

### 3. 增加“产品如何支持这个方案”

客户要看到这不是咨询 PPT，而是有产品承载：

- Project Playbook。
- Agent Ready Checklist。
- Task Template。
- Scheduler。
- Run / Audit。
- Skill / Docs。
- Permissions / Sandbox。
- Review / Tuning。

方案文档和产品文档要互相闭环。

## 结论

我们可以达成原方案目的，但不能只靠“理念文章”和“对象管理产品”。

必须补上中间层：

```text
客户流程方法论
  -> Multigent Project Playbook
  -> Agent 小队配置
  -> 任务模板和调度
  -> 可观察 run
  -> 人工介入沉淀
  -> ROI 数据
```

如果这一层做出来，Multigent 就不是一个泛泛的 multi-agent 平台，而是一套能让客户公司逐步变成 agentic company 的落地系统。

下一步建议：

1. 把客户原方案改写成 Multigent 版本。
2. 做第一个“软件研发项目 Playbook Template”。
3. 围绕这个模板实现 Project Create 向导、Agent Ready Checklist 和 Task Template。
4. 用一个真实研发流程跑 POC，拿数据反向打磨销售文档。
