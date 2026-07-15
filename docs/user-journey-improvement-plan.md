# Multigent 用户旅程改进计划

本文档用于指导下一阶段 Multigent 的产品和技术实现。当前阶段不再优先扩展更多 provider，而是先把团队从 0 到第一次让 agent 产生有效结果的路径打通。

Multigent 的核心价值不是“又一个项目管理工具”，也不是“更多连接器列表”，而是让公司可以把人类专业知识、上下文、权限和运行环境组织成一支可持续工作的 agent team。

## 目标

下一阶段目标：

```text
管理员能在 Web 内完成首次上手闭环；
成员能进入自己的工作台并管理自己负责的 agent；
agent 能在受控权限下运行；
人能通过 review/tuning 改善 agent，而不是同步驱动每一步。
```

最小可验收闭环：

```text
注册/登录
-> 创建 workspace
-> 创建项目
-> 添加成员或跳过
-> 创建/雇佣 agent
-> 配置模型 provider
-> 创建连接并 grant 给 agent
-> 下达第一条 task/message
-> agent run
-> 查看结果、日志、成本、连接使用记录
-> 调整 prompt/skill/provider/grant/scheduler
```

## 当前判断

目前连接体系已经足够支撑第一版体验：

- Model provider 支持 workspace/user 级别。
- Connector connection 支持 workspace/user ownership、grant、加密 secret、runtime manifest、MCP proxy、action proxy、OAuth、health check、action policy。
- 已经有 GitHub、Feishu/Lark、Linear、DingTalk Bot、custom HTTP/MCP 等可用基础。

因此近期不要把主要精力放在继续扩展 provider。更重要的是让用户知道：

- 我下一步该做什么。
- 为什么要做这一步。
- 做完后 agent 能获得什么能力。
- 出问题后我去哪里 review 和调教。

## 核心原则

- **Web 优先**：首次上手路径必须能在 Web 内完成，不应该提示用户“用 CLI 雇佣 agent”。
- **角色感知**：admin、project manager、agent operator、普通成员进入系统后看到的默认页面和可操作项不同。
- **权限即产品体验**：UI 隐藏不可用操作，但 API 必须继续做权威校验。
- **人是 agent 负责人**：人负责配置、投喂、审核和调教 agent，不应该成为每一步调用 agent 的同步阻塞点。
- **少概念先闭环**：team/role/skill 很重要，但首次体验不能强制用户先理解完整组织建模。
- **不要历史债**：这是新产品，错误或本地优先的概念应直接替换，而不是兼容。

## 旅程一：首次管理员上手

### 目标体验

管理员第一次进入系统后，不应该看到空洞的 dashboard，而应该看到一个可执行的 workspace setup checklist。

Checklist 建议顺序：

1. 创建 workspace。
2. 创建第一个 project。
3. 添加或邀请成员。
4. 创建第一个 agent。
5. 配置 agent 的模型 provider。
6. 连接外部工具。
7. 把连接 grant 给 project 或 agent。
8. 写入 wakeup prompt 或初始工作说明。
9. 向 agent 发送第一条 task/message。
10. 查看第一次 run 结果。

### 当前缺口

- 有 no-workspace onboarding，但 workspace 创建后缺少明确的下一步引导。
- Overview 对 admin 还不够 operational，没有把 setup checklist、系统健康、agent 状态、blocked run 组织起来。
- “创建项目 -> 创建 agent -> 配模型 -> grant 工具 -> run” 还不是一条清晰 Web flow。
- 一些空状态仍然暗示 CLI，这会破坏 SaaS 产品感。

### 建议实现

- 新增 workspace setup checklist API 或在 Overview 聚合已有数据计算 checklist。
- Overview 首屏展示：
  - setup progress
  - next recommended action
  - active projects
  - agent health
  - running/blocked tasks
  - recent runs
  - recent audit events
- 空 workspace 时弱化 dashboard 数字，强化下一步按钮。
- 所有 checklist item 都链接到具体页面或弹窗。

## 旅程二：邀请和成员加入

### 目标体验

被邀请成员应该能顺滑完成：

```text
打开邀请链接 -> 注册或登录 -> 接受/拒绝邀请 -> 进入 workspace -> 看到自己的 workbench
```

管理员发邀请时应能设置：

- workspace role
- 可访问项目
- 可管理 agent
- 可选 display name
- 过期时间

### 当前缺口

- 当前本地模式可以用 copy invite link，但还不是生产级邮箱邀请体验。
- 批量邀请、撤销、过期、重新发送还需要补。
- 注册后回到 invite acceptance 的路径需要保证稳定。
- 接受邀请后应该直接进入目标 workspace，而不是让用户迷路。

### 建议实现

- People/Workspace 页面增加 invite management：
  - pending invites
  - accepted/rejected/expired 状态
  - revoke
  - resend/copy link
  - bulk invite textarea
- Login/Register 保留 invite token，并在注册后跳回接受页。
- 接受邀请后切换当前 workspace。
- 拒绝邀请后，如果用户没有 workspace，进入 create workspace onboarding。

## 旅程三：项目和 agent 创建闭环

### 目标体验

管理员或项目负责人可以在 Web 中完成：

```text
Create project
-> Add members
-> Hire/Create agent
-> Assign owner/operator
-> Select model provider
-> Select runtime/sandbox/toolchain profile
-> Grant skills/connections
-> Configure wakeup/scheduler
-> Send first task
```

### 当前缺口

- 项目列表和项目详情已有，但 project creation 与 agent creation 的连续性不够。
- agent 仍然有一些本地文件夹/CLI 概念残留。
- owner/operator 与 agent 的关系已经有数据基础，但缺少强产品化入口。
- agent 的模型、连接、技能、调度配置散落在不同页面，首次创建时不够集中。

### 建议实现

- 做一个 “Create agent” flow，不必一开始做复杂 wizard，但至少包含：
  - name
  - team/role
  - responsible human
  - model provider
  - wakeup prompt
  - connections/grants
  - scheduler toggle
- 创建 agent 成功后进入 agent detail 的 “Ready to run” 状态。
- agent detail 顶部展示缺失配置：
  - missing model provider
  - no owner/operator
  - no wakeup prompt
  - no granted connection
  - scheduler not configured
- 去掉或隐藏普通用户不该看到的本地 path 字段。

## 旅程四：成员工作台

### 目标体验

普通成员登录后默认不应该看到 admin dashboard，而应该看到自己的个人工作台：

- 我的消息。
- 我的任务。
- 我负责或可操作的 agent。
- 我有权限访问的项目。
- 需要我 review 的 runs。
- 我可以调教的 agent 配置项。

### 当前缺口

- 已有 Workbench，但还需要更聚焦 agent owner/operator 的实际工作。
- 成员视角的项目、任务、消息、run 权限还需要继续 endpoint-by-endpoint audit。
- UI 路由里仍有一些 workspace admin 级别的粗粒度判断。

### 建议实现

- Workbench 改成 personal operating console：
  - inbox
  - assigned/related tasks
  - owned/operated agents
  - recent failed/blocked runs
  - pending approvals
- 增加 “My agents” 区块，每个 agent 显示：
  - last run status
  - next scheduled wakeup
  - unresolved messages/tasks
  - quick actions: chat, create task, review runs, tune config
- 项目页面的 schedule/settings 等入口按 project role 和 agent operation permission 显示，而不是只看 workspace admin。

## 旅程五：Agent 负责人调教闭环

### 目标体验

Multigent 最关键的差异化页面应该是 agent operations view。它回答：

```text
这个 agent 最近表现如何？
为什么失败？
用了哪些权限和连接？
人介入了哪些决策？
下次怎么减少人工介入？
```

### 当前缺口

- Agent detail 能看到部分 runtime/connection 信息，但还不是 performance/tuning 工作台。
- run logs、task 状态、token/cost、blocked reason、connection usage、prompt/skill/provider/grant 调整没有集中。
- 缺少 review workflow：approve output、request rerun、create follow-up task、convert intervention into policy/skill/prompt。

### 建议实现

Agent operations view 应包含：

- Summary:
  - success/failure count
  - recent blocked reasons
  - token/cost
  - last run
  - next wakeup
- Recent runs:
  - status
  - task/message source
  - model/provider
  - connection usage
  - log link
- Review actions:
  - approve result
  - request rerun
  - create follow-up task
  - send message to agent
- Tuning actions:
  - edit wakeup prompt
  - adjust model provider
  - adjust connection grants
  - adjust scheduler
  - propose/add skill
- Intervention ledger:
  - repeated human decisions
  - candidate automation
  - suggested prompt/skill/policy updates

## 旅程六：连接和权限解释

### 目标体验

用户需要清楚理解：

- workspace connection 是团队共享能力。
- personal connection 是用户自己的授权，只能给自己有权限操作的 agent 使用。
- grant 给 workspace/project/agent 的影响范围不同。
- agent 永远不直接拿到 secret，只通过 runtime proxy 使用连接。

### 当前缺口

- 连接能力实现已经较多，但 UI 对这些概念解释不够。
- Grant dialog 可用，但用户未必理解目标范围的风险。
- Agent detail 看到 runtime connections，但缺少 “为什么这个 agent 能用这个连接” 的解释。

### 建议实现

- Connections 页面增加轻量说明和空状态示例。
- Connection card 显示：
  - owner scope
  - granted targets
  - last validation
  - action policy summary
  - OAuth/account summary
- Grant dialog 按风险分层：
  - workspace grant: high impact
  - project grant: medium impact
  - agent grant: narrow impact
- Agent detail 的 runtime connections 展示 matched grants reason。

## 旅程七：项目结束后的记忆沉淀

### 目标体验

项目或子任务完成后，应能沉淀：

- 项目事实。
- 决策记录。
- agent 表现经验。
- 可复用 prompt。
- 可复用 skill。
- 连接/权限策略。
- 下次同类任务的流程建议。

### 当前缺口

- 目前更多是 run log 和 task summary，缺少 memory candidate flow。
- 什么能自动继承，什么需要人审批，还没有产品化。

### 建议实现

- 从 run review 里产生 memory candidate。
- memory candidate 有状态：
  - proposed
  - approved
  - edited
  - rejected
  - archived
- 负责人审批后写入对应 scope：
  - workspace memory
  - team/role memory
  - project memory
  - agent memory
- 不自动把所有 run log 变成长记忆，避免污染 context。

## 推荐实现顺序

### Slice 1：Workspace Setup Checklist

目标：管理员进入 workspace 后知道下一步该做什么。

交付：

- Overview 增加 setup checklist。
- checklist item 基于当前 workspace 数据计算状态。
- 空状态按钮跳转到具体 action。
- 去掉明显 CLI-first 的提示。

验收：

- 新 workspace 创建后，admin 看到明确的 next steps。
- 至少能引导到 create project、connections、people、providers 页面。

### Slice 2：Web 内创建 agent 闭环

目标：不依赖 CLI 完成第一个 agent 创建和基础配置。

交付：

- Create agent flow。
- Assign owner/operator。
- Bind model provider。
- Initial wakeup prompt。
- 创建后进入 agent detail。

验收：

- 用户能在 Web 内从项目创建到 agent detail。
- agent detail 能显示缺失配置和下一步操作。

### Slice 3：Personal Workbench

目标：成员登录后看到自己的工作，而不是 admin 视角。

交付：

- Workbench 聚合 my messages、my tasks、my agents、my runs needing review。
- 项目和 agent 列表继续按权限过滤。

验收：

- 非 admin 默认进入 personal workbench。
- 成员看不到无权限项目。
- 成员能快速进入自己负责的 agent。

### Slice 4：Agent Operations View

目标：负责人能 review 和调教 agent。

交付：

- Agent performance summary。
- Recent runs + cost/token + blocked reason。
- Tuning shortcuts。
- Review actions。

验收：

- 负责人能从一个页面判断 agent 表现并做配置调整。
- 失败/阻塞 run 能被快速追踪到原因和后续动作。

### Slice 5：Invite Flow Completion

目标：团队能顺滑把成员拉进 workspace。

交付：

- Bulk invite。
- Invite list/revoke/expiry。
- Accept/reject page polish。
- Register -> invite acceptance continuation。

验收：

- 管理员可批量创建邀请。
- 被邀请用户注册后能回到邀请接受页。
- 拒绝邀请不授予访问权限。

### Slice 6：Memory Candidate Flow

目标：项目经验可控沉淀，不污染 context。

交付：

- Run/task completion 产生 memory candidate。
- Human review approve/edit/reject。
- 写入 workspace/team/project/agent scope。

验收：

- 完成一个任务后能看到可复用经验候选。
- 人审批后才进入长期 context。

## 下一步建议

下一轮代码实现建议从 **Slice 1：Workspace Setup Checklist** 开始。

原因：

- 它直接改善首次上手体验。
- 不需要先重构底层 runtime。
- 能把现有分散能力串成产品路径。
- 做完后后续 Slice 2/3/4 都有明确入口。

