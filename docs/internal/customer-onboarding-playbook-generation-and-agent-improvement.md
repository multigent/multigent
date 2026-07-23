# Customer Onboarding, Playbook Generation, and Agent Improvement

> Status: Draft  
> Scope: customer onboarding, process discovery, playbook generation, OpenSpec research, estimation, agent performance, skill mining

本文档梳理 Multigent 当前商业化落地中几个比较阻塞的问题，并提出产品和技术解决方案。

核心判断：

```text
Multigent 不只是让用户创建 agent、分配任务、跑 workflow。
真正的商业价值在于：帮助客户把已有协作方式转成 agent 可执行、可评估、可持续优化的协作系统。
```

这意味着我们需要补齐五类能力：

1. 用 agent 帮客户梳理现有流程和上下文。
2. 把客户流程、角色、skill、workflow 打包成协作方案。
3. 借鉴 OpenSpec，把模糊需求转成可审阅、可执行、可验证的规格化产物。
4. 评估 agent 工时、效率和交付质量。
5. 从 agent 执行中的重复问题中自动挖掘 skill，提高后续效率。

## 1. 客户流程梳理 Agent

### 问题

新客户真正卡住的不是“不会点按钮创建 workspace”，而是：

- 不知道自己的流程如何迁移到 Multigent。
- 不知道哪些文档、会议记录、任务系统、聊天记录应该纳入上下文。
- 不知道哪些工作适合 agent 自动执行，哪些必须 human review。
- 不知道现有流程里哪些节点应该变成 workflow、task template、skill 或 policy。

如果靠客户自己填写表单，信息一定不完整；如果靠我们人工咨询，又很难规模化。

### 解决方向：Process Discovery Agent

Multigent 应该内置一个“流程梳理 Agent”，帮助客户完成从旧流程到 agent workflow 的迁移。

它不是普通聊天机器人，而是一个受控 onboarding workflow：

```text
连接信息源
-> 采集协作样本
-> 访谈关键负责人
-> 识别重复流程
-> 生成流程地图
-> 生成候选协作方案
-> human review
-> 安装到 workspace
```

### 输入来源

第一阶段可以支持这些来源：

| 来源 | 用途 |
| --- | --- |
| 飞书 / Lark Docs | PRD、会议纪要、流程文档、周报 |
| 飞书 / Lark IM | 项目沟通、需求澄清、bug 反馈、review 习惯 |
| GitHub / GitLab | PR、issue、review、发版节奏 |
| Jira / Linear / Plane / Huly | 任务状态、负责人、周期、拆分习惯 |
| Multigent Chat / Run Logs | agent 执行过程中的失败、追问、人工介入 |
| 手动上传文档 | 老流程说明、SOP、组织结构、角色职责 |

不需要一开始就支持所有系统。第一版最有价值的是：

```text
飞书/Lark 文档 + 飞书/Lark 群聊 + GitHub/GitLab issue/PR + 人工访谈
```

### 输出产物

流程梳理 Agent 应该输出结构化结果，而不是只写一篇总结。

```yaml
process_map:
  name:
  description:
  participants:
    - role:
      human_owner:
      current_tools:
      repeated_decisions:
  stages:
    - name:
      current_inputs:
      current_outputs:
      current_owner:
      agent_candidate:
      human_review_required:
      pain_points:
      automation_opportunity:
  recurring_patterns:
    - pattern:
      evidence:
      suggested_skill:
      suggested_task_template:
  missing_context:
    - item:
      owner_to_ask:
      reason:
  recommended_playbook:
    roles:
    skills:
    workflows:
    task_templates:
    external_tools:
```

### 产品形态

建议在 workspace onboarding 之后新增一个入口：

```text
协作诊断 / Process Discovery
```

用户可以选择：

- 连接工具自动分析。
- 上传文档分析。
- 让 agent 访谈我。
- 让 agent 访谈团队成员。

最终给用户一个可 review 的报告：

- 当前流程图。
- 痛点和阻塞。
- 可自动化节点。
- 必须人工 review 的节点。
- 推荐创建的协作方案。

## 2. 协作方案生成

### 问题

Multigent 当前支持内置协作方案，但还不够：

- 用户不能把自己的流程打包成协作方案。
- 项目跑完后沉淀的流程、skill、角色 prompt 无法形成可复用资产。
- 我们无法把一次客户交付变成下一次客户可安装的模板。

### 解决方向：Playbook Builder

协作方案不应该只来自我们预置。它应该有三种来源：

| 来源 | 说明 |
| --- | --- |
| Built-in Playbook | 我们内置，例如研发交付、YC Garry、Matt Pocock |
| Generated Playbook | 流程梳理 Agent 根据客户上下文生成 |
| Saved Playbook | 用户从现有 workspace 资产保存 |

### 生成流程

```text
Process Discovery Report
-> Generate Draft Playbook
-> Preview assets
-> Human edits
-> Install / Save
```

### Playbook 应包含的内容

```yaml
playbook:
  metadata:
    name:
    version:
    locale:
    source:
    generated_from:
  roles:
    - role_prompt
    - default_skills
    - recommended_model_profile
  skills:
    - name
    - description
    - content
    - source_evidence
  workflows:
    - workflow_definition
    - input_output_specs
    - review_loops
  task_templates:
    - title_template
    - prompt_template
    - workflow_binding
  tool_requirements:
    - provider
    - reason
    - required_actions
  metrics:
    - expected_cycle_time
    - review_count_target
    - token_budget_target
```

### 是否允许用户修改

沿用之前我们已对齐的原则：

- 内置 Playbook 是模板。
- 安装后的角色、skill、workflow 是用户 workspace 里的资产实例。
- 用户修改后，该资产应标记为 `customized`。
- 后续内置 Playbook 更新不直接覆盖用户已修改资产。
- 第一版不做复杂 diff / merge。

建议记录来源：

```yaml
asset_source:
  playbook_id:
  playbook_version:
  asset_id:
  installed_at:
  customized_at:
  update_policy: managed | detached
```

第一版可以只做展示，不做更新机制，但数据字段要留好。

## 3. OpenSpec 研究与 Multigent 的结合

### OpenSpec 的核心价值

OpenSpec 的重点不是任务管理，而是：

```text
把模糊意图变成 proposal / specs / design / tasks 这些可审阅 artifacts，
再让 AI 基于 artifacts 执行和验证。
```

它的几个关键思想值得借鉴：

- **Spec 是行为契约，不是代码实现方案。**
- **Change 是一个自包含文件夹，包含 proposal、spec、design、tasks。**
- **先 review plan，再写代码。**
- **需求可以迭代，不是严格瀑布。**
- **用场景和验收标准减少模糊自然语言。**
- **团队可以把 spec 作为跨 repo 的共享计划源。**

这和 Multigent 当前 workflow 的缺口正好对应：我们有任务和流程流转，但需要更强的规格化 artifact 层。

### 不应该直接照搬的地方

OpenSpec 偏代码项目和本地 repo：

- 以 `openspec/` 文件夹为中心。
- 强调 git / PR / archive。
- 主要服务 AI coding assistant。

Multigent 是 workspace SaaS，应该抽象成：

```text
Doc Artifact / Workflow Artifact / Task Output
```

也就是：

- 不暴露本地路径。
- 用 docID / artifactID。
- artifact 存在 Multigent 知识库或数据库中。
- workflow step 的输入输出引用 artifactID。
- agent 通过 `mga docs` 和 `mga task step done` 读写。

### 建议新增：Spec Artifact

在 Multigent 中引入通用 artifact 类型：

```yaml
artifact:
  id:
  type: proposal | requirement_spec | design_spec | task_plan | test_case | test_report | retrospective
  title:
  format: markdown | html | json
  doc_id:
  schema_id:
  created_by:
  created_at:
  source_run_id:
  workflow_run_id:
  step_id:
```

这样流程节点输出就不是一段聊天总结，而是明确产物：

```text
PM Spec Step -> requirement_spec artifact
Engineering Design Step -> design_spec artifact
QA Step -> test_case + test_report artifacts
Retrospective Step -> skill_candidate artifacts
```

### 建议新增：Spec Review Loop

借鉴 OpenSpec 的 review-before-build：

```text
agent drafts spec
-> human reviews spec
-> approve: continue
-> reject: comments return to draft step
```

这和我们现在 workflow 的打回逻辑一致，但需要把 review 对象从“任务文本”升级成“artifact”。

## 4. 工时预估与 Agent 效率评估

### 问题

我们能派活，但 agent 仍然不能稳定预估工时。原因是：

- agent 对代码库和工具链的真实摩擦不确定。
- 同一类任务在不同项目里复杂度差异大。
- agent 可能花大量 token 读上下文、试错、等待工具、重复跑失败命令。
- 人类评估“满意”或“不满意”目前没有结构化反馈。

工时预估不能只靠 agent 自己拍脑袋。

### 解决方向：Estimate = 先验 + 历史 + 实时修正

建议建立三层估算模型：

```text
Initial Estimate
  = task template prior
  + project historical data
  + agent historical data
  + workflow step complexity

Runtime Forecast
  = initial estimate
  + current run progress
  + error/retry signals
  + context/tool latency

Actual Measurement
  = wall time
  + active run time
  + token usage
  + retries
  + human intervention time
  + review cycles
```

### 关键指标

#### 时间指标

| 指标 | 说明 |
| --- | --- |
| cycle_time | 从任务创建到完成的总时间 |
| active_agent_time | agent 实际执行时间 |
| queue_wait_time | 等待调度或等待人的时间 |
| review_wait_time | 等待 human review 的时间 |
| rework_time | 打回后重复执行的时间 |

#### token / 资源指标

| 指标 | 说明 |
| --- | --- |
| total_tokens | 总 token |
| context_tokens | 上下文消耗 |
| reasoning_tokens | 推理消耗，取决于 provider 是否支持 |
| wasted_tokens | 失败 run、重复读取、无效尝试的估算 |
| tokens_per_artifact | 产出一个有效 artifact 的 token |

#### 质量指标

| 指标 | 说明 |
| --- | --- |
| first_pass_acceptance_rate | 第一次提交就通过 review 的比例 |
| review_rounds | review 循环次数 |
| defect_escape_rate | QA / 上线后发现问题比例 |
| human_intervention_count | 人工介入次数 |
| output_schema_pass_rate | 结构化输出一次通过率 |

### 预估产品形态

创建任务时展示：

```text
预计耗时：20-40 分钟
可信度：中
依据：
- 同类任务历史中位数 31 分钟
- 当前 agent 首次通过率 68%
- 该流程通常需要 1 次 human review
```

运行中展示：

```text
当前阶段：开发方案
已用：12 分钟 / 预计 20-40 分钟
风险：输出字段缺失概率较高，因为该 agent 最近 3 次同类节点被打回 2 次
```

### 第一版实现

先不做复杂 ML。只做规则 + 历史聚合：

- task template 默认估时。
- workflow step 默认估时。
- agent 最近 N 次同类任务中位数。
- 超时阈值。
- review 次数。
- token 中位数。

## 5. 从重复问题生成 Skill

### 问题

agent 执行中反复出错时，我们现在主要靠人手动改 prompt 或临时提醒。

这会导致：

- 相同错误反复发生。
- 人工介入没有沉淀。
- 优秀经验只留在单个 session。
- token 反复浪费在解释同一件事。

### 解决方向：Skill Mining Loop

每次任务完成或失败后，系统应该分析：

- 这次是否发生重复问题。
- 人类是否多次给出同类纠正。
- agent 是否反复调用错误命令。
- 是否有可沉淀为 skill 的流程。
- 是否应更新 role prompt、project prompt、workflow 规则或 task template。

### 触发信号

```text
重复失败：
  同一 agent / 同一 workflow step / 同一 error pattern 出现 >= 3 次

重复人工介入：
  human review comments 相似度高

重复工具使用：
  agent 每次都需要人提醒如何使用某个 CLI / 外部工具

重复上下文缺失：
  agent 多次请求同一类文档、接口说明、权限信息

重复输出不合格：
  schema 校验失败字段相同
```

### 输出：Skill Candidate

```yaml
skill_candidate:
  id:
  title:
  problem_pattern:
  evidence:
    - task_id
    - run_id
    - review_comment_id
  proposed_skill:
    name:
    description:
    content:
  target:
    workspace | team | role | project | agent
  expected_benefit:
    reduce_review_rounds:
    reduce_token:
    reduce_failure_rate:
  status: proposed | accepted | rejected | installed
```

### 产品形态

建议在 Agent 详情页和 Workflow Run 复盘页增加：

```text
可沉淀改进
```

展示：

- 建议新增 skill。
- 建议修改 prompt。
- 建议新增 task template。
- 建议调整 workflow 输出字段。
- 建议添加外部工具或文档。

用户点击“采纳”后：

- 生成 skill draft。
- 选择应用范围。
- 保存后自动进入下一次 run 的 materialization。

### Skill 不应该什么都装进去

需要区分：

| 问题类型 | 应沉淀为 |
| --- | --- |
| 工具使用方法 | Skill |
| 长期角色职责 | Role Prompt |
| 项目特定事实 | Project Context / Docs |
| 流程流转规则 | Workflow Definition |
| 任务输入模板 | Task Template |
| 权限或审批规则 | Policy / RBAC |

Skill 的边界应该是：

```text
可复用的方法、检查清单、工具操作步骤、质量标准。
```

不要把大量项目事实塞进 skill。

## 6. 统一方案：Agentic Adoption Loop

这五件事不是孤立功能，应该串成一个闭环：

```text
Discover
  采集客户流程、上下文、历史协作样本

Model
  生成流程地图、角色、skill、workflow、task template

Run
  用 Multigent workflow 执行真实任务

Measure
  记录耗时、token、review、失败、人工介入、质量

Improve
  生成 skill / prompt / workflow / playbook 改进建议

Package
  打包成客户自己的 Playbook，或沉淀为内置行业 Playbook
```

这应该成为 Multigent 商业化的核心叙事：

```text
不是卖一个多 agent 调度工具，
而是帮助公司把真实工作流程转成可运行、可度量、可持续优化的 agent workforce。
```

## 7. 建议开发优先级

### P0：先让客户能上手

1. Process Discovery Agent 初版。
2. 支持从文档/聊天/任务样本生成流程诊断报告。
3. 支持从诊断报告生成 draft playbook。
4. 支持 playbook preview 和安装。

### P1：让流程产物更可靠

1. 引入 artifact 类型。
2. workflow step 输出支持 artifact 引用。
3. 借鉴 OpenSpec，提供 proposal/spec/design/tasks/test report 的默认 artifact 模板。
4. human review 针对 artifact，而不是只针对 task summary。

### P2：让效率可度量

1. step / workflow / agent 的耗时与 token 聚合。
2. first-pass acceptance rate。
3. review rounds。
4. task estimate 初版。
5. agent performance view。

### P3：让系统越跑越好

1. Skill Candidate 生成。
2. 人工采纳 skill。
3. skill 应用范围选择。
4. playbook 保存与导出。
5. 客户自定义 playbook 版本管理。

## 8. 对 OpenSpec 的下一步研究任务

建议单独建一个 research task，重点研究：

1. OpenSpec 的 artifact schema 如何组织。
2. `proposal/spec/design/tasks` 哪些概念适合抽象到非研发场景。
3. `store` 概念如何映射到 Multigent workspace docs。
4. OpenSpec 的 slash command / skill 生成机制是否可借鉴到 Multigent playbook。
5. OpenSpec 的 validation 能否转成 Multigent workflow output schema validator。

最终目标不是集成 OpenSpec CLI，而是吸收它的核心模型：

```text
artifact-guided workflow + behavior-first spec + review-before-execution
```

## 9. 关键决策

### 决策 1：Playbook 需要支持生成

是。否则客户流程无法沉淀成产品资产。

### 决策 2：Spec Artifact 是必要抽象

是。只靠 task summary 和 chat log 不足以支撑企业流程。

### 决策 3：工时预估先做规则和历史统计

是。不要一开始做复杂模型。先把数据采集和展示打通。

### 决策 4：Skill 生成必须 human approve

是。自动生成可以，但自动安装会污染上下文。第一版必须人工确认。

### 决策 5：OpenSpec 不直接作为依赖

暂时不直接依赖。Multigent 应吸收思想和结构，但保持自己的 SaaS workspace / workflow / docs / agent runtime 模型。
