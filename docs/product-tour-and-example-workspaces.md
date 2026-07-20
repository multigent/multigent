# Product Tour 与 Example Workspace 设计

本文档讨论 Multigent 新用户创建 workspace 后，如何从“空白系统”顺滑进入“第一个可运行的 agent 协作流程”。

核心判断：

- 不要在创建 workspace 流程中强制用户选择协作方案。
- 创建 workspace 后进入真实产品界面，再用 Product Tour 引导用户完成第一条路径。
- Example Workspace 可以作为低成本体验入口，但必须和真实 workspace 明确区分。

## 背景问题

Multigent 的核心价值不是普通项目管理，而是把团队、Agent、知识库、外部工具、任务和流程组织成可持续运行的 agent team。

这带来一个上手难点：

- 空 workspace 没有项目、团队、Agent、流程，用户不知道第一步做什么。
- 协作方案虽然能降低配置成本，但“选择方案”本身也会造成压力。
- 很多用户没有准备好公司流程、模型账号、工具凭证，就无法立刻看到价值。
- 只做菜单式功能介绍无法解释 Multigent 的核心工作方式。

因此 Product Tour 不能只是“这是导航栏、这是按钮”的 UI tour。它应该是一个从空 workspace 到第一个可运行流程的引导器。

## 设计原则

### 1. 目标导向，不是模板导向

第一次进入 workspace 时，不直接问：

> 请选择一个协作方案。

而是问：

> 你想先优化哪类工作？

推荐选项：

- 研发交付
- Bug 修复
- 产品/创业验证
- 客服知识沉淀
- 自定义流程
- 先随便看看

用户选择目标后，系统再推荐对应协作方案。

### 2. Tour 不阻塞使用

Product Tour 必须可以跳过，也可以稍后继续。

原因：

- 有经验用户可能想直接配置模型和 Agent。
- 销售演示或内部测试可能需要快速跳到某个页面。
- 用户第一次登录时不一定有足够信息完成全部配置。

### 3. 让用户尽快看到 Agent 跑起来

Tour 的目标不是把所有设置讲完，而是帮助用户完成最短闭环：

```text
创建 workspace
-> 选择目标场景
-> 安装推荐协作方案
-> 配置模型账号
-> 创建项目
-> 添加 Agent
-> 创建第一个任务
-> 看到流程节点流转和 Agent 输出
```

### 4. 把复杂概念延后

新用户一开始不需要理解：

- playbook 是什么。
- workflow 和 task 的边界。
- skill 如何继承。
- 外部工具连接如何授权给 Agent。
- scheduler 与 wakeup prompt 的区别。

这些概念应该在用户走到对应步骤时渐进出现。

## Product Tour 结构

### 入口时机

触发条件：

- 用户刚创建第一个 workspace。
- 用户进入一个空 workspace，且没有完成 onboarding。
- 用户点击首页的“继续设置 Multigent”。

不触发条件：

- 用户不是 workspace owner/admin。
- workspace 已有项目和 Agent，并且 onboarding 标记已完成。
- 用户明确跳过并选择“不再自动显示”。

### Tour 状态

建议以 workspace 维度记录：

```text
workspace_onboarding_state
  workspace_id
  user_id
  status: not_started / in_progress / skipped / completed
  selected_goal
  recommended_playbook_id
  installed_playbook_id
  current_step
  completed_steps[]
  dismissed_at
  completed_at
```

这里的状态是产品体验状态，不应该影响底层权限和业务数据。

### Step 1：欢迎与定位

目的：用一句话解释 Multigent。

文案方向：

> Multigent 帮你把团队流程、Agent、任务、知识和工具组织起来，让 Agent 能持续接活、交付、反馈。

按钮：

- 开始配置
- 先跳过

### Step 2：选择目标场景

问题：

> 你想先优化哪类工作？

选项与推荐方案：

| 目标 | 推荐协作方案 |
| --- | --- |
| 研发交付 | Agentic Software Delivery |
| Bug 修复 | Bug Triage and Fix |
| 产品/创业验证 | YC Garry Startup Validation |
| 工程协作优化 | Matt Pocock Real Engineering |
| 客服知识沉淀 | Customer Support Knowledge Loop |
| 自定义流程 | 不安装方案，进入空白设置 |

产品上不要让用户直接面对一个很长的模板列表。先选目标，再展示推荐结果。

### Step 3：推荐协作方案

展示：

- 方案名称。
- 适用场景。
- 会预置哪些内容：团队角色、Skill、流程、任务模板。
- 后续都可以修改。

按钮：

- 使用推荐方案
- 查看详情
- 暂不安装

注意：这里不展示 setup questions。因为当前还没有把回答写入安装结果，展示会造成“只有问题没有答案”的困惑。未来如果安装向导支持答案生成 prompt，再恢复。

### Step 4：配置模型账号

目的：让 Agent 至少能跑起来。

界面应避免暴露环境变量概念，用用户能理解的方式：

- 选择 Agent CLI：Codex / Claude Code / Cursor。
- 选择官方或第三方网关。
- 填 API Key。
- 测试连接。

如果用户选择 Example Workspace，可以提示：

> 只需要配置一个可用模型账号，就可以运行示例 Agent。

### Step 5：配置外部工具

目的：让 Agent 有真实工作上下文。

第一版不强制配置外部工具。用户可以跳过。

可推荐：

- GitHub / GitLab：代码、Issue、PR。
- 飞书 / Lark：文档与沟通。
- Slack：沟通。
- Exa / Brave：Web 搜索。

### Step 6：创建项目与 Agent

如果用户安装了协作方案：

- 从方案推荐团队角色。
- 引导创建一个项目。
- 选择 1-3 个 Agent 作为最小可运行小队。

如果用户没有安装协作方案：

- 引导创建空项目。
- 选择一个通用 Agent。

### Step 7：运行第一个任务

这是 Product Tour 的关键节点。

建议提供几个小任务模板：

- 让第一个 Agent 写一段问候和协作目标。
- 让第二个 Agent 读取上游输出并回应。
- 让第三个 Agent 总结这次协作过程。

这个任务必须足够小，避免用户第一次运行就陷入长时间等待或失败。

成功后展示：

- Agent 输出。
- 流程当前节点。
- 下一步需要谁审核。
- 如果失败，展示如何查看运行日志和调整模型/Prompt。

### Step 8：结束与后续 checklist

Tour 结束后，首页保留 checklist：

- 已安装协作方案。
- 已配置模型账号。
- 已创建项目。
- 已添加 Agent。
- 已运行第一个任务。
- 已邀请成员。
- 已配置外部工具。

Checklist 不应该是强制流程，只是帮助用户知道还差什么。

## Example Workspace

### 为什么需要 Example Workspace

Product Tour 仍然要求用户做配置。Example Workspace 可以让用户更快体验“成品效果”：

```text
进入示例 workspace
-> 配置一个模型账号
-> 给示例 Agent 绑定模型账号
-> 点击运行示例任务
-> 看到完整流程流转
```

它适合：

- 新用户体验产品价值。
- 销售演示。
- 内部 QA。
- 文档教程配套。

### Example Workspace 的定位

Example Workspace 不是用户真实业务 workspace。

它应该明确标记为：

- 示例
- 可重置
- 不建议存放真实凭证或业务数据

UI 上可以有一个醒目的标识：

> Example Workspace：用于体验 Multigent。你可以复制为自己的 workspace。

### Example Workspace 的创建方式

推荐两种方式：

#### 方式 A：系统内置示例模板

用户点击：

```text
创建 workspace
-> 使用示例 workspace
```

系统从内置 seed 创建一个新的 workspace 副本。

特点：

- 每个用户拿到自己的副本。
- 可以安全编辑和删除。
- 不会多人共享同一个示例数据。

#### 方式 B：只读公共示例

系统提供一个公共只读 workspace。

特点：

- 用户无需配置即可浏览。
- 不能运行 Agent。
- 适合展示界面结构，但无法验证真实运行。

不推荐只做方式 B。Multigent 的价值在“Agent 运行和流程流转”，只读示例不足以让用户理解核心价值。

### Example Workspace 应包含什么

第一版建议内置一个纯协作 Hello World 示例，而不是任何垂直业务示例。

内容：

- 一个 workspace：`Example Workspace`。
- 一个项目：`hello-world-relay`。
- 三个 Agent：
  - Greeter Agent
  - Responder Agent
  - Recorder Agent
- 一个流程：
  - 发起问候
  - 人工审核
  - 接力回应
  - 协作总结
  - 最终确认
- 一些知识库文档：
  - 示例说明。
  - 协作规则。
  - 输出示例。
- 一个待运行任务：
  - 让三个 Agent 完成一次 Hello World 协作接力。

用户只需要配置模型账号并绑定给 Agent，就能点击运行。

### Example Workspace 的任务设计

示例任务应该满足：

- 不依赖真实外部工具。
- 不依赖私有仓库。
- 不需要复杂环境。
- 可以在 sandbox 中快速完成。
- 输出结果容易肉眼判断。

建议示例：

> 让三个 Agent 完成一次 Hello World 协作接力：第一个 Agent 写问候和目标，人工审核后，第二个 Agent 基于上游内容回应，第三个 Agent 总结整个协作过程。最终产出一份“协作记录”。

这个任务足够小，但能验证：

- Greeter Agent 能创建第一份结构化输出。
- Human review 能打回或通过。
- Responder Agent 能读取上游输出并继续接力。
- Recorder Agent 能汇总前面所有输出。
- 流程能在多个 Agent 和人之间流转。

### Example Workspace 的核心体验目标

Example Workspace 不是为了展示“我们有很多功能”，而是为了让用户在 10 分钟内理解 Multigent 的核心价值：

```text
任务不是直接丢给一个 Agent 聊天。
任务会进入一个清晰流程。
流程中的每一步有输入、输出、负责人和审核规则。
Agent 负责产出，人负责校准，系统负责流转和记录。
```

第一版示例必须优先验证这个闭环，而不是做复杂项目。

衡量标准：

- 用户能看懂 workspace、项目、团队、Agent、流程、任务之间的关系。
- 用户能完成一次“Agent 输出 -> 人工 Review -> 下一个 Agent 接手”的流转。
- 用户能在任务详情里看到上游输出、当前节点、下一步流向和运行记录。
- 用户不需要接入 GitHub、飞书、Slack 等外部工具也能跑通。
- 用户只需要配置一个模型账号，或使用 demo 模式，就能体验最小闭环。

### 推荐示例场景

第一版推荐采用“Hello World Relay”场景，而不是研发、营销、销售、设计等垂直场景。

原因：

- 单纯传递消息太轻，无法体现 Multigent 和普通群聊/Agent Chat 的差异。
- 垂直任务会暗示 Multigent 只适合某个部门或行业，降低通用性。
- Hello World Relay 足够抽象，任何用户都能理解，不需要业务背景或技术背景。
- 这个场景仍然能展示输入输出、人工审核、打回、上下文传递和最终交付。
- 任务足够小，可以避免用户第一次体验就等太久。

示例任务：

> 请三个 Agent 完成一次 Hello World 协作接力。第一个 Agent 写一段问候并说明协作目标；人类审核通过后，第二个 Agent 读取第一步输出并做回应；第三个 Agent 汇总前两步内容，生成一份清晰的协作记录。

这个任务不属于任何专业领域，它只展示“多个 Agent 如何围绕同一个简单目标协作、交接和接受人类审核”。

### 示例数据规格

Example Workspace v1 建议内置以下数据。

Workspace：

```text
name: Example Workspace
description: 用于体验 Multigent Agent 协作流程的示例工作区。
kind: example
resettable: true
```

Project：

```text
name: hello-world-relay
description: 一个用于演示 Agent 协作接力的 Hello World 项目。
```

Team：

```text
name: collaboration-demo
description: 一个最小协作演示团队，包含发起、回应和记录三个角色。
```

Roles：

| Role | 作用 | 关键约束 |
| --- | --- | --- |
| greeter | 发起 Hello World 协作，写第一段问候和目标 | 输出必须清晰、简短，并写入知识库 |
| responder | 读取上游输出，做出回应并补充下一步 | 必须引用上游 docID，不能重新发明上下文 |
| recorder | 汇总前面输出，形成完整协作记录 | 必须说明每个 Agent 做了什么 |

Agents：

| Agent | Role | 默认模型配置 |
| --- | --- | --- |
| greeter-agent | greeter | 待用户绑定 |
| responder-agent | responder | 待用户绑定 |
| recorder-agent | recorder | 待用户绑定 |

Humans：

```text
workspace creator -> owner/admin
greeting reviewer -> workspace creator
final reviewer -> workspace creator
```

Docs：

| Doc ID 建议 | 标题 | 内容 |
| --- | --- | --- |
| demo-intro | 示例说明 | 说明这是一个纯协作 Hello World 示例 |
| demo-rules | 协作规则 | 每个 Agent 必须读取上游 docID，并输出结构化结果 |
| demo-output-sample | 输出示例 | 给用户参考流程输出应该如何展示 |

Workflow：

```text
发起问候 -> 人工审核 -> 接力回应 -> 协作总结 -> 最终确认
```

节点定义：

| 节点 | 执行者 | 输入 | 输出 |
| --- | --- | --- | --- |
| 发起问候 | greeter-agent | original_goal, rules_doc_id | greeting_doc_id, handoff_note_doc_id, summary |
| 人工审核 | workspace creator | greeting_doc_id, handoff_note_doc_id | decision, comments |
| 接力回应 | responder-agent | greeting_doc_id, handoff_note_doc_id, review_comments | response_doc_id, next_handoff_doc_id, summary |
| 协作总结 | recorder-agent | greeting_doc_id, response_doc_id, next_handoff_doc_id | collaboration_record_doc_id, learnings_doc_id, summary |
| 最终确认 | workspace creator | collaboration_record_doc_id, learnings_doc_id | decision, comments |

分支：

```text
人工审核 decision=approve -> 接力回应
人工审核 decision=request_changes -> 发起问候
最终确认 decision=approve -> 完成
最终确认 decision=request_changes -> 协作总结
```

注意：

- 输出字段只定义字段名和说明，不要求用户理解字段类型。
- 对用户展示时优先展示字段说明；字段名只作为高级信息或 hover 说明。
- docID 应渲染为可点击的知识库文档链接。

Task：

```text
title: 完成一次 Hello World 协作接力
description: 用于演示多个 Agent 与人工审核之间的协作流转。
prompt:
  请完成一次 Hello World 协作接力。
  目标：
  1. Greeter Agent 写一段简短问候，并说明这次协作的目标。
  2. 人类审核这段问候，可以通过或打回修改。
  3. Responder Agent 读取 Greeter 的输出，做出回应，并补充下一步交接说明。
  4. Recorder Agent 汇总前两位 Agent 的输出，生成一份协作记录。
  5. 大段产物和说明写入知识库文档，并在流程输出中返回 docID。
workflow: demo-hello-world-relay
actor bindings:
  greeter -> greeter-agent
  greeting-reviewer -> workspace creator
  responder -> responder-agent
  recorder -> recorder-agent
  final-reviewer -> workspace creator
```

### Demo Mode 与真实模型模式

Example Workspace 最好支持两种体验方式。

#### Demo Mode

Demo Mode 不调用真实模型，而是使用内置的示例输出驱动流程。

适合：

- 用户没有 API Key。
- 销售演示。
- 文档教程截图。
- 端到端 UI 验证。

Demo Mode 的行为：

- 点击“运行示例”后，系统按固定节奏模拟 Agent 输出。
- 每个 Agent 节点生成预置知识库文档和结构化输出。
- 人工 Review 节点仍然需要用户点击通过或打回。
- 页面明确标注“Demo Mode，不消耗模型 Token”。

Demo Mode 的价值是让用户先理解产品，再要求他配置模型账号。

#### Real Run

Real Run 使用用户配置的模型账号和真实 sandbox 运行。

适合：

- 用户已经有模型账号。
- 用户想验证真实 Agent 执行能力。
- 内部 QA 检查 runtime、CLI、sandbox、workflow 是否连通。

Real Run 的前置条件：

- 至少一个可用模型账号。
- 三个示例 Agent 都绑定模型账号，或者允许一键将同一个模型账号绑定到全部示例 Agent。
- Sandbox runtime 可用。

建议交互：

```text
运行示例
-> 选择 Demo Mode / Real Run
-> 如果选 Real Run 且未配置模型账号，引导去配置
-> 支持“一键绑定给全部示例 Agent”
```

### Example Workspace 的 Product Tour 脚本

Tour 不应该是纯说明文字，而应该跟随用户在真实页面上完成一次体验。

#### Tour 0：进入示例工作区

页面：workspace switcher 或首页。

说明：

> 这是一个可重置的示例工作区。它不会影响你的真实 workspace。我们会用一次 Hello World 协作接力演示 Multigent 如何让 Agent 和人协作。

动作：

- 下一步
- 跳过
- 复制为我的 workspace

#### Tour 1：查看团队和角色

页面：Teams。

说明：

> 团队表示职能分工，角色定义 Agent 的长期职责。示例中只有发起、回应、记录三个角色，不绑定任何行业或部门。

高亮：

- `collaboration-demo` team。
- greeter / responder / recorder roles。

用户动作：

- 点击团队卡片。
- 查看角色列表。

#### Tour 2：查看 Agent

页面：Project members 或 Agent detail。

说明：

> Agent 是项目里的可执行成员。每个 Agent 会继承 workspace、团队、角色、项目、Skill 和任务上下文。

高亮：

- greeter-agent。
- responder-agent。
- recorder-agent。

用户动作：

- 打开 responder-agent。
- 查看模型与凭证状态。

#### Tour 3：配置模型账号

页面：Model Accounts 或 Agent detail。

说明：

> 真实运行需要模型账号。你可以先配置一个账号，并一键绑定给示例 Agent；也可以选择 Demo Mode 先体验流程。

用户动作：

- 配置模型账号。
- 或点击“使用 Demo Mode”。

#### Tour 4：查看流程白板

页面：Workflows -> demo hello world relay。

说明：

> 流程定义了任务如何在人和 Agent 之间流转。每个节点都有输入、输出和执行者。人工审核节点可以通过，也可以带意见打回上一步。

高亮：

- 发起问候节点。
- 人工审核节点。
- 接力回应节点。
- 协作总结节点。
- 打回连线。

用户动作：

- 点击节点查看输入输出。
- 点击分支连线查看条件。

#### Tour 5：创建或打开示例任务

页面：Tasks。

说明：

> 任务是流程运行的载体。这里的任务会沿着刚才的流程推进，并把每个阶段的输出记录下来。

用户动作：

- 点击“运行示例任务”。
- 或打开已预置的示例任务。

#### Tour 6：观察 Greeter Agent 输出

页面：Task detail。

说明：

> Greeter Agent 会先写一段问候，并说明这次协作目标。大段内容会写入知识库，流程输出只保存 docID 和摘要。

高亮：

- 当前节点。
- 上游输入。
- 节点输出。
- 运行记录链接。

用户动作：

- 等待 Greeter 节点完成。
- 点击 docID 查看文档。

#### Tour 7：人工审核与打回

页面：Task detail 的人工审核区。

说明：

> 人不是同步驱动 Agent 的人，而是关键节点的审核者。你可以通过，也可以填写意见打回，让 Agent 根据意见重做。

用户动作：

- 第一次建议点击“打回”，填写一条修改意见。
- 观察任务回到 Greeter Agent。
- 第二次点击“通过”。

这里是整个示例最关键的教学点：用户能直观看到 review loop。

#### Tour 8：Responder Agent 与 Recorder Agent 接手

页面：Task detail。

说明：

> 流程通过后会自动切换到下一个 Agent。下游 Agent 不需要人重新解释上下文，它可以读取上游输出和知识库文档。

用户动作：

- 观察负责人从 greeter-agent 切换到 responder-agent，再切换到 recorder-agent。
- 查看每个节点的结构化输出。

#### Tour 9：最终确认

页面：Task detail。

说明：

> 最终，人只需要 Review 这份协作记录。所有中间过程都已经被记录，可以回溯，也可以沉淀为后续流程优化依据。

用户动作：

- 点击最终通过。
- 查看任务完成状态。

#### Tour 10：下一步

页面：首页。

说明：

> 你已经跑完一次示例流程。下一步可以复制这个流程到真实 workspace，或用 Product Tour 创建自己的项目和 Agent。

动作：

- 复制流程到我的 workspace。
- 创建真实项目。
- 邀请团队成员。
- 配置外部工具。

### Tour 展示方式

建议使用“轻量浮层 + 页面内高亮 + 底部进度”的方式，而不是全屏教学页。

组件：

```text
TourOverlay
  step title
  short explanation
  target selector
  progress
  primary action
  secondary action
  skip
```

交互原则：

- 每一步只解释一个概念。
- 文案控制在 2-3 句话。
- 必须允许跳过。
- 跳过后首页 checklist 仍保留“继续 Tour”入口。
- 用户主动操作完成后自动进入下一步，不要求每步都点“下一步”。

### Example Workspace 的重置与复制

Example Workspace 应支持：

- 重置示例数据。
- 删除示例 workspace。
- 复制流程到真实 workspace。
- 复制协作方案到真实 workspace。

不建议第一版支持“把示例任务运行历史复制到真实 workspace”。运行历史属于体验数据，不应污染真实 workspace。

### Example Workspace 的权限规则

示例 workspace 中：

- 创建者是 owner/admin。
- 其他被邀请用户默认可以查看示例数据。
- 只有 owner/admin 可以重置示例。
- 如果用户在示例 workspace 配置了模型账号，应清楚标记该凭证作用域。

如果未来支持公共只读示例：

- 只读示例不能运行 Agent。
- 只读示例不能配置凭证。
- 用户点击运行时必须复制为自己的示例副本。

### Example Workspace 与真实 Workspace 的关系

用户可以有三种动作：

- 直接体验：在示例副本中运行任务。
- 复制为正式 workspace：把示例中的团队、流程、知识库复制出来。
- 删除示例：不影响真实 workspace。

不要让用户在正式 workspace 和 example workspace 之间混淆。

### 凭证安全

Example Workspace 不应该内置任何真实凭证。

如果用户配置模型账号：

- 明确提示该模型账号属于当前用户或当前 workspace。
- 不要自动授权给所有真实 workspace。
- 示例 workspace 删除时，询问是否同时删除仅用于示例的凭证。

## 推荐用户路径

最终建议组合：

```text
注册
-> 创建 workspace
-> 进入 workspace 首页
-> Product Tour 自动打开
-> 选择目标场景
-> 推荐协作方案
-> 用户选择：
   A. 安装推荐方案并继续配置真实 workspace
   B. 进入 Example Workspace 快速体验
   C. 跳过，自己探索
```

这样既不会在创建 workspace 时增加压力，也给了用户一个最快体验路径。

## 第一版实现范围

### Product Tour v1

- Workspace 创建后触发。
- 支持跳过。
- 支持选择目标场景。
- 支持推荐协作方案并跳转安装。
- 首页展示 onboarding checklist。
- Workspace 维度记录状态。

### Example Workspace v1

- 内置一个通用协作示例模板。
- 创建用户专属示例 workspace 副本。
- 示例 workspace 有明显标识。
- 包含示例项目、Agent、流程、知识库和待运行任务。
- 用户配置模型账号后可运行。
- 支持 Demo Mode，允许无模型账号跑通固定示例流程。
- 支持一键绑定同一个模型账号给示例 Agent。
- 支持重置示例 workspace。
- Product Tour 可以逐步高亮团队、Agent、流程、任务、人工审核和输出记录。

### 暂不做

- 复杂问卷生成定制 playbook。
- 从用户真实聊天记录自动提炼流程。
- 多个行业级示例 workspace。
- 示例 workspace 的多人协作演示。
- 一键把 example workspace 全量迁移为正式生产空间。
- 真实外部工具接入演示。
- 让示例 workspace 自动读取用户真实仓库或真实企业文档。

## 后续演进

### 安装向导

未来协作方案安装时，可以把 `setupQuestions` 做成真正的表单：

```text
选择协作方案
-> 回答关键问题
-> 根据答案生成项目 prompt、流程默认执行人、任务模板和工具建议
-> 预览
-> 安装
```

这时“配置问题”才应该重新出现在用户界面中。

### 流程迁移助手

更长期的方向是引入一个系统 Agent：

- 收集用户旧流程文档。
- 分析已有会议纪要和历史 Agent 对话。
- 提取角色、节点、输入输出、审核点。
- 生成 workspace/project 的流程建议。
- 让人确认后安装。

这会让 Multigent 从“模板产品”升级成“帮助企业迁移到 Agent 协作架构的系统”。

### 从历史 Agent 对话生成流程

用户后续可以导入历史 Codex / Claude Code / Cursor Agent 对话记录。系统分析这些对话，提取：

- 常见任务类型。
- 人类反复补充的背景。
- Agent 经常缺失的输入。
- 成功交付时的步骤。
- 失败或返工时的原因。

这些信息可以用于生成：

- 项目 prompt。
- 角色 prompt。
- workflow 节点。
- task template。
- review checklist。

这会把 Example Workspace 的教学路径延伸到真实企业迁移路径。

### 流程运行指标

Example Workspace 也应该演示 Multigent 如何衡量流程质量。

建议指标：

- 总耗时。
- 每个节点耗时。
- 人工 Review 次数。
- 打回次数。
- Agent token 消耗。
- 节点失败次数。
- 任务完成率。

这些指标能帮助用户理解：Multigent 不是只让流程“看起来可控”，还要持续降低人工介入和 token 消耗。

## 结论

Product Tour 和 Example Workspace 应该同时做，但职责不同：

- Product Tour 负责把用户从空 workspace 引导到第一个真实配置。
- Example Workspace 负责让用户在最少配置下体验完整价值。

第一版最合理的设计是：

```text
创建 workspace 后进入首页
-> Product Tour 自动打开
-> 推荐真实配置路径
-> 同时提供 Example Workspace 快速体验入口
```

这样既降低新用户心智压力，也能更快展示 Multigent 的核心价值：Agent 可以在清晰流程中接活、交付、反馈和沉淀。
