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

- 让 PM Agent 根据一句话需求产出需求 Spec。
- 让 Dev Agent 实现一个非常小的独立页面或脚本。
- 让 QA Agent 根据已有输出写测试清单。

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

第一版建议内置一个研发交付示例。

内容：

- 一个 workspace：`Example Product Lab`。
- 一个项目：`base64-tool-demo`。
- 三个 Agent：
  - PM Agent
  - Dev Agent
  - QA Agent
- 一个流程：
  - 需求整理
  - 人工审核
  - 开发实现
  - QA 测试
  - 交付总结
- 一些知识库文档：
  - 项目背景。
  - 需求示例。
  - 验收标准示例。
- 一个待运行任务：
  - 交付一个纯前端 Base64 编解码页面。

用户只需要配置模型账号并绑定给 Agent，就能点击运行。

### Example Workspace 的任务设计

示例任务应该满足：

- 不依赖真实外部工具。
- 不依赖私有仓库。
- 不需要复杂环境。
- 可以在 sandbox 中快速完成。
- 输出结果容易肉眼判断。

建议示例：

> 创建一个单文件 HTML Base64 编解码工具。页面包含输入框、编码按钮、解码按钮、输出区域和错误提示。无需构建工具，无需外部依赖。

这个任务足够小，但能验证：

- PM Agent 能整理需求和验收标准。
- Human review 能打回或通过。
- Dev Agent 能实现。
- QA Agent 能输出测试报告。
- 流程能在多个 Agent 和人之间流转。

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

- 内置一个研发交付示例模板。
- 创建用户专属示例 workspace 副本。
- 示例 workspace 有明显标识。
- 包含示例项目、Agent、流程、知识库和待运行任务。
- 用户配置模型账号后可运行。

### 暂不做

- 复杂问卷生成定制 playbook。
- 从用户真实聊天记录自动提炼流程。
- 多个行业级示例 workspace。
- 示例 workspace 的多人协作演示。
- 一键把 example workspace 全量迁移为正式生产空间。

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
