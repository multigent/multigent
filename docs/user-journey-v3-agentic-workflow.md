# Multigent User Journey V3: From Existing Workflow To Agentic Workflow

本文档记录 Multigent 下一阶段的用户旅程思考。重点不是继续解释 workspace、team、project、agent 这些对象，而是回答一个更核心的问题：

```text
一个真实公司如何把原来的工作流程，逐步变成 agent 能参与、能执行、能反馈、能持续调教和沉淀的流程。
```

Multigent 的商业价值不应该停留在“创建几个 agent”或“连接几个工具”。真正的价值是帮助客户公司从个人使用 AI，升级为可运营的 agent workforce。

## 核心判断

现有上手路径大致是：

```text
注册
-> 创建 workspace
-> 邀请成员
-> 创建团队
-> 创建项目
-> 添加 agent
-> 配模型和工具
-> 下任务
```

这条路径能创建对象，但还没有完全回答客户最关心的问题：

```text
我公司原来那套流程，怎么变成 agent 可以稳定跑的流程？
```

用户不是为了“建 team/project/agent”而来。他们是为了让一个真实流程被 agent 分担，例如：

- Bug 修复。
- 小需求开发。
- QA 验收。
- PR Review。
- 客服问题处理。
- 会议纪要跟进。
- 每日项目巡检。

因此，Multigent 的 user journey 应该从“对象创建旅程”升级为“流程迁移和 agent 运营旅程”。

## 目标旅程

最终应该形成这样的主线：

```text
选择一个真实流程
-> 由流程助手生成 agent playbook
-> 创建 agent 小队
-> 配置模型、工具和权限
-> 跑第一条真实任务
-> 在 Web / 飞书 / Lark 中调教 agent
-> 复盘 run 和人工介入
-> 把经验沉淀成 prompt、skill、task template、playbook
```

## 1. 注册与 Workspace 建立

用户注册 / 登录后，如果没有 workspace，进入创建 workspace 引导。

Workspace 是公司或团队的权限边界：

- 成员。
- 项目。
- 工具连接。
- 模型账号。
- 审计日志。
- agent workforce。

创建者默认成为 workspace admin。

完成 workspace 创建后，系统可以引导用户邀请成员，但不应该强制。更关键的是引导用户选择第一个试点流程。

## 2. 选择试点流程

系统不应该先问用户“要创建什么 team”，而应该先问：

```text
你想先让 agent 参与哪个真实工作流程？
```

示例选项：

- 软件研发项目。
- Bug 修复流程。
- 小需求交付流程。
- QA 验收流程。
- 内容生产流程。
- 客服支持流程。
- 数据分析流程。
- 自定义流程。

选择试点流程的意义：

- 降低用户理解成本。
- 让后续 team、agent、prompt、tool、schedule 都围绕一个真实目标生成。
- 避免用户面对空白 prompt 和空白项目。

## 3. 流程迁移助手

这是下一阶段最关键的产品能力。

Multigent 应该内置一个流程迁移助手，可以是一个系统 agent，也可以表现为一个 onboarding wizard。它的任务不是简单套模板，而是帮助用户把原来的流程转成 agent 可运行的 Project Playbook。

### 它应该问什么

围绕一个试点流程，助手应该收集：

- 这个流程由谁发起。
- 当前任务在哪里管理。
- 需求或问题描述在哪里产生。
- 相关文档在哪里。
- 代码或产物在哪里。
- 谁负责评审方案。
- 谁负责验收。
- 什么动作需要人工确认。
- 什么动作 agent 可以直接做。
- 用 GitHub、GitLab、Linear、Jira、ONES、飞书、Notion、Figma 还是其他工具。
- 大需求和小 bug 的处理方式是否不同。
- 做完后通知谁。
- 失败或阻塞时升级给谁。

### 它应该生成什么

助手输出一份初版 Project Playbook，包括：

- project prompt。
- wakeup prompt。
- schedule 建议。
- task templates。
- approval rules。
- required tools / connections。
- recommended agent roles。
- agent ready checklist。
- 需要用户确认的不确定项。

### 它不应该做什么

它不应该把模板生硬搬过来，也不应该假设所有公司流程一样。

正确方式是：

```text
模板提供初始结构；
用户提供现实约束；
流程助手把两者合成可运行 playbook；
后续通过 run 和调教持续修正。
```

### 历史 Agent 对话导入

用户原来的流程不一定只存在于制度文档里。很多真实流程已经散落在负责人和本地 Codex、Claude Code、Cursor 等 agent 的历史对话里。

Multigent 可以支持导入这些历史材料：

- PM 与 agent 的需求讨论。
- 开发与 agent 的实现讨论。
- QA 与 agent 的测试/复现讨论。
- 负责人对 agent 的纠偏、补充说明、验收反馈。
- 飞书 / Lark 群聊中的任务上下文。
- 会议纪要、PRD、技术方案、接口文档。

流程迁移助手应该从这些材料里提取：

- 常见任务类型。
- 关键决策点。
- 角色分工。
- 输入信息格式。
- 输出交付物。
- 验收标准。
- 人工介入原因。
- 常见阻塞和失败模式。
- 隐含规则，例如“大需求要先过方案评审，小 bug 可以直接修”。

这能减少用户从零描述流程的负担，也能避免只依赖模板导致流程脱离真实工作方式。

但导入历史对话时必须注意：

- 用户需要明确选择导入范围。
- 系统要展示提取出的流程事实和依据。
- 重要规则需要负责人确认后才能进入 project playbook。
- 原始材料要可追溯，避免 agent 幻觉式总结。

### 流程状态机

Project Playbook 不应该只是 prompt 文本，还应该逐步沉淀成一个可执行的流程状态机。

例如软件研发流程可以抽象为：

```text
需求收集
-> 范围澄清
-> 方案设计
-> 人工评审
-> 拆分任务
-> 开发执行
-> 自测
-> 联调
-> QA 验收
-> 修复问题
-> 发布准备
-> 复盘沉淀
```

每个状态应该定义：

- 状态名称。
- 进入条件。
- 负责人或 agent role。
- 必要输入。
- 允许动作。
- 预期输出。
- 验收标准。
- 可自动流转条件。
- 需要人工确认的条件。
- 超时或阻塞后的升级策略。

状态机不是为了把公司流程变僵硬，而是为了让人和 agent 都知道：

```text
当前处于哪个阶段；
下一步该谁做；
需要什么输入；
完成后如何判断；
什么情况必须找人。
```

### Web 可视化

流程状态机应该在 Web 上可视化。

用户应该能看到：

- 整个流程图。
- 当前任务所在状态。
- 每个节点的负责人和 agent。
- 当前节点缺少哪些输入。
- 哪些节点是自动流转。
- 哪些节点需要人工 review。
- 每个节点最近的 run、消息、产出和阻塞。
- 从失败节点进入调教或修改 playbook 的入口。

这有两个价值：

1. 用户更容易相信 Multigent 不是黑盒，而是在明确地控制流程。
2. 销售和演示时更直观，客户能看见“旧流程如何变成 agent 可运行流程”。

状态机可视化不一定第一版就做复杂 BPMN。初期可以是简洁的节点图：

```text
节点 -> 状态 -> 输入/输出 -> 负责人 -> 自动/人工门禁
```

### 结构化输入输出

要降低 token 消耗和执行偏差，不能只依赖模糊自然语言。

Multigent 应该尽可能把流程节点的输入输出结构化。

例如 Bug 修复任务的输入可以包括：

```yaml
title:
affected_area:
expected_behavior:
actual_behavior:
reproduction_steps:
environment:
related_logs:
related_files:
acceptance_criteria:
non_goals:
reviewer:
```

小需求实现任务可以包括：

```yaml
goal:
user_scenario:
scope:
non_goals:
ui_requirements:
api_contract:
data_changes:
test_requirements:
release_notes_required:
owner:
reviewer:
```

结构化输入不是为了取消自然语言，而是让自然语言有骨架：

- 必填字段减少遗漏。
- agent 更容易判断任务边界。
- 负责人更容易 review。
- 失败时更容易定位是“输入缺失”还是“执行失败”。
- 后续更容易统计和复用。

每个流程节点也应该有结构化输出，例如：

- 方案文档。
- 实现 summary。
- 测试结果。
- 风险列表。
- 需要人工确认的问题。
- 产物链接。
- 下一步建议。

### 持续校准

流程状态机、结构化字段和模板都不应该一次性定死。

每次 run、人工介入、失败复盘都应该反向校准流程：

- 哪个节点经常缺输入。
- 哪个节点经常需要人补充背景。
- 哪个节点适合自动流转。
- 哪个节点必须人工 review。
- 哪个结构化字段没有用。
- 哪个字段应该新增为必填。
- 哪个任务类型需要拆成更小模板。

Multigent 应该把这些变化做成可 review 的建议，而不是自动修改流程：

```text
系统发现最近 5 次 bug 修复中，QA 都要求补充 reproduction_steps。
是否将 reproduction_steps 设为 Bug 修复任务必填字段？
```

## 4. 创建团队与 Agent 小队

Team Template 仍然有价值，但它不应该是旅程的起点。

用户选择试点流程后，系统可以推荐对应 agent 小队：

软件研发示例：

- PM agent。
- Frontend agent。
- Backend agent。
- QA agent。
- Reviewer agent。
- Release agent。

每个 agent 都应该有：

- 角色。
- 所属项目。
- 人类负责人。
- 模型账号。
- 工具权限。
- 继承的 role/team/project prompt。
- wakeup 或 schedule。

用户不应该觉得自己在配置一堆抽象对象，而应该理解为：

```text
这个流程需要哪些 agent 同事？
每个 agent 负责什么？
谁负责调教它？
它需要哪些工具和权限？
```

## 5. 配置模型、工具和权限

Agent 要真正跑起来，至少需要三类能力。

### 模型账号

例如：

- Codex。
- Claude Code。
- Cursor。
- 其他 agent CLI。

用户不应该理解环境变量。产品上应该表现为：

```text
选择 CLI / 模型
-> 选择官方 API Key 或第三方 Provider
-> 测试是否可用
```

### 外部工具

例如：

- GitHub / GitLab。
- 飞书 / Lark。
- Figma。
- Linear / Jira / ONES。
- Brave / Exa Web Search。

不同工具可以有不同接入形态：

- 平台 CLI + skill。
- MCP Gateway。
- HTTP action。
- skill-only。

但这些不应该暴露成用户必须理解的概念。用户只需要知道：

```text
这个 agent 是否能使用这个工具。
```

### 权限授权

工具连接不等于所有 agent 都能使用。Multigent 需要按 workspace / project / agent 控制可见性和可调用权限。

Agent 运行时应该通过自己的 agent identity 和 runtime token 调用 Multigent API，而不是直接读取数据库或全局凭证。

## 6. Agent Ready Checklist

很多用户不知道为什么 agent 跑不起来或跑不好。

Agent detail 页面应该有一个明确的 readiness 状态：

- 模型账号是否配置。
- 模型是否测试可用。
- 人类负责人是否设置。
- role / team / project prompt 是否完整。
- 关键 skill 是否存在。
- 必要工具是否连接。
- 工具是否授权给该 agent。
- wakeup prompt 是否配置。
- scheduler 是否开启。
- 最近一次 test run 是否成功。

这不是简单的 setup checklist，而是 agent 能否接活的状态判断。

## 7. 第一次真实任务

系统不应该让用户从空白输入框开始。

应该根据 Project Playbook 提供 Task Template：

- 修复 Bug。
- 实现小需求。
- 写技术方案。
- 做代码 Review。
- 做 QA 验收。
- 做发布检查。
- 复盘失败 run。

Task Template 应该包含：

- 任务标题。
- 背景输入。
- 必填字段。
- 验收标准。
- 相关工具或文档。
- 是否需要人工审批。
- 完成后需要输出什么 summary。

这能减少用户反复写 prompt，也能减少 token 浪费。

## 8. Web / 飞书 / Lark 调教闭环

Agent 不应该只能通过 Web Chat 调教。

在很多公司里，飞书或 Lark 才是日常工作发生的地方。Multigent 应该允许用户把 agent 连接到飞书 / Lark，然后直接在 IM 中介入 agent 的真实工作上下文。

### 典型路径

```text
用户打开 agent 详情页
-> 点击连接飞书 / Lark
-> 完成扫码或应用绑定
-> 飞书 / Lark 里出现 agent bot
-> 用户直接给 agent 发消息
-> Multigent 校验用户身份和权限
-> 锁定或复用 agent session
-> agent 回复
-> transcript、run、audit 全部回到 Multigent
```

### 关键原则

这不是“训练态”。

更准确地说：

```text
人进入 agent 的真实工作上下文，补充背景、纠正方向、审批决策、要求继续。
```

当同类人工介入重复出现，系统应该提示用户把它沉淀成：

- project prompt。
- role prompt。
- task template。
- skill。
- policy。
- docs。

### Session Lock

一个 agent 同一时间不应该混入多个 mutable session。

如果 agent 正在处理任务，人可以进入同一个 session 观察或介入；但 scheduler 不应该同时再给它塞另一个任务。

这能避免上下文污染。

## 9. Run Review 和失败诊断

Agent 失败时，不能只给日志。

系统应该生成结构化诊断：

- 缺上下文。
- 缺权限。
- 缺工具。
- prompt 不清。
- 任务太大。
- 模型不合适。
- 需要人工确认。
- 外部系统失败。
- 测试失败。

并给出下一步建议：

- 补充文档。
- 授权工具。
- 拆分任务。
- 修改 task template。
- 修改 project prompt。
- 新增 skill。
- 换模型或 provider。
- 发给负责人 review。

这一步是从“agent 跑了一次”到“agent 逐步变好”的核心。

## 10. Human Intervention Ledger

Multigent 应该记录每次人工介入：

- 为什么需要人。
- 是否真的不可自动化。
- 用户补充了什么判断。
- 下次是否可以沉淀成规则、prompt、skill、task template 或 approval policy。

如果同类介入重复出现，系统应该主动提示：

```text
这个问题已经重复出现 3 次，是否沉淀为项目规则？
```

这能让客户看到 agent workflow 的复利。

## 11. 沉淀为公司流程资产

一个流程跑几轮后，应该沉淀出：

- Project Playbook。
- Agent 配置模板。
- Task Template。
- Prompt Rule。
- Skill。
- Docs。
- Human Intervention Ledger。
- 成功 / 失败案例。
- Run diagnosis 数据。

这些资产让下一次项目不再从零开始。

## 当前产品差距

按这条 v3 journey，目前最需要补的能力是：

1. Project Playbook Assistant。
2. Agent Ready Checklist。
3. Task Template。
4. Run Diagnosis。
5. Human Intervention Ledger。
6. 飞书 / Lark 调教闭环的完整 session / transcript / audit。
7. 从 run review 一键沉淀 prompt / skill / task template / docs。

## 一句话总结

Multigent 的用户旅程应该从：

```text
注册 -> 创建 workspace -> 创建 team/project/agent
```

升级为：

```text
选择一个真实流程
-> 由流程助手生成 agent playbook
-> 创建 agent 小队
-> 配工具和权限
-> 跑第一条任务
-> 在 Web / 飞书 / Lark 中调教
-> 把经验沉淀成可复用流程资产
```

这更接近 Multigent 要卖给客户公司的核心价值：不是多一个 agent 聊天工具，而是帮助公司建立一套 agent 时代的协作架构。
