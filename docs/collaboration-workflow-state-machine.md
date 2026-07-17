# Collaboration Workflow State Machine

本文档探讨 Multigent 中“人和 Agent 协作状态机”的设计。

它和普通项目管理里的任务流转接近，但重点不同：

- 普通项目管理关注任务状态，例如 todo / in progress / done。
- Multigent 更关注人和 agent 如何协作、每个阶段需要什么输入、产出什么结构化结果、谁来审核、如何自动或人工流转到下一阶段。

因此，这里讨论的不是 Kanban 状态机，而是 **Collaboration Workflow State Machine**。

## 核心目标

一个需求进入 Multigent 后，系统应该能回答：

```text
当前处于哪个协作阶段？
这个阶段由谁负责，人还是 agent？
这个阶段需要什么输入？
这个阶段必须产出什么？
产出是否满足结构化要求？
是否需要人工 review？
review 通过后流转给谁？
失败或阻塞后回到哪里？
哪些经验需要沉淀？
```

这比单纯的模板更可控，也更适合落地。

模板负责给用户一个初始流程；状态机负责让这个流程真正跑起来、可观察、可审计、可优化。

## 与项目管理任务状态的区别

项目管理里的任务状态通常是：

```text
待处理 -> 进行中 -> 已完成 -> 已关闭
```

Multigent 的协作状态机更像：

```text
需求输入
-> 产品澄清
-> 产品 Spec Review
-> 开发调研
-> 开发方案 Review
-> 开发实现
-> PR Review / Merge
-> 测试用例设计
-> 测试执行
-> 测试报告 Review
-> 上线准备
-> 上线审批
-> 发布
-> 复盘沉淀
```

每个阶段可以由 human 或 agent 负责，也可以由 agent 执行、人做门禁。

## 基本实体

### Task

Task 仍然应该是用户看到的核心工作项。

用户不应该被迫理解一个新的“流程对象”来替代任务。对用户来说，一个需求、一个 bug、一次发布、一次 QA 验收，本质上都是一个 Task。

但 Task 当前的状态不应该直接扩展成流程状态机。

现有 Task 状态描述的是执行队列状态：

```text
pending
in_progress
awaiting_confirmation
blocked
done_success
done_failed
cancelled
```

这些状态服务于 scheduler、runner、队列、统计和归档。它们回答的是：

```text
这个任务是否待执行、正在执行、等待确认、阻塞、成功、失败或取消？
```

协作状态机要回答的是另一个问题：

```text
这个工作项现在处于哪个协作阶段？
当前阶段的输入输出是什么？
谁负责执行？
是否需要 review？
下一步流转到哪里？
```

因此，推荐设计是：

```text
Task 是用户可见工作项。
Workflow Run 是挂在 Task 下面的流程执行层。
Step Instance 是 Workflow Run 中的节点执行记录。
```

一个 Task 可以没有 Workflow Run，仍然按普通任务执行；也可以绑定一个 Workflow Run，进入可视化、结构化、可审计的协作流程。

### Workflow Definition

表示一条协作流程模板，例如：

- 软件研发需求流程。
- Bug 修复流程。
- QA 验收流程。
- 发布流程。
- 客服问题处理流程。

它定义有哪些节点、节点之间如何流转、每个节点的输入输出规范。

### Workflow Run

表示某个具体需求正在执行这条流程。

例如：

```text
需求：增加外部工具 Web Search 能力
Workflow: 软件研发需求流程
当前节点：开发实现
```

Workflow Run 应该绑定到一个 Task：

```yaml
task_id:
workflow_definition_id:
current_step_id:
status: active | completed | cancelled | failed
started_at:
finished_at:
```

这样用户仍然从 Task 进入，但系统可以在 Task 内展示流程图、当前节点、节点产物、review 状态和运行指标。

### Step Definition

表示流程里的一个节点。

每个节点应该定义：

```yaml
id:
name:
purpose:
actor_type: human | agent | either | multiple
recommended_role:
input_schema:
output_schema:
tools_required:
entry_condition:
exit_condition:
review_required:
reviewer_role:
next:
fallback:
timeout_policy:
audit_required:
```

### Step Instance

表示某个 workflow run 里的具体节点执行记录。

它记录：

- 谁执行了。
- 输入是什么。
- 输出是什么。
- 是否通过 schema 校验。
- 是否请求了人工 review。
- review 结果。
- run log。
- 相关 artifacts。
- 流转到哪里。

Step Instance 可以对应一个子 Task，也可以只是一个人工 review 或系统判断节点。

推荐做法：

```text
父 Task = 一个需求 / bug / 工作项。
子 Task = 某个流程节点分配给某个 agent 或 human 的执行单元。
```

例如：

```text
父 Task：增加 Web Search 外部工具能力
  Workflow Run：软件研发需求流程
  当前节点：Implementation

子 Task 1：PM Agent 输出产品 Spec
子 Task 2：Human PM 审核产品 Spec
子 Task 3：Developer Agent 输出开发方案 Spec
子 Task 4：Human Tech Lead 审核开发方案
子 Task 5：Developer Agent 实现并提交 PR
子 Task 6：QA Agent 输出测试用例和测试报告
```

父 Task 负责展示整体流程状态；子 Task 负责具体执行、run、日志和完成结果。

### Transition Request

Agent 不应该随意修改状态，而应该发起流转请求：

```yaml
from_step:
to_step:
requested_by:
reason:
output_ref:
checks:
review_required:
```

系统根据规则决定：

- 自动通过。
- 进入人工 review。
- 拒绝并要求补充。
- 回退到某个节点。

Agent 侧可以通过受控 API 请求流转，例如：

```text
mga workflow request-transition \
  --task <task-id> \
  --from implementation \
  --to pr_review \
  --artifact <artifact-id> \
  --summary "实现完成，已创建 PR，自测通过"
```

系统必须校验：

- 当前 step 是否允许该 agent 请求流转。
- output schema 是否完整。
- 是否需要人工 review。
- 下一个 step 应该分配给谁。
- 是否要自动创建下一个子 Task。

### Workflow Artifact

每个关键节点都应该产出结构化 artifact，而不是只把结果留在聊天记录里。

示例 artifact：

- Product Spec。
- Engineering Spec。
- Pull Request。
- Test Cases。
- Test Report。
- Release Package。
- Retrospective。

Artifact 应该可被后续节点引用，例如 QA 节点引用 Product Spec、Engineering Spec 和 PR。

### Review Item

人工审核应该是可追踪实体。

Review Item 记录：

- 审核对象。
- 审核人。
- 审核状态。
- 审核意见。
- 审核时间。
- 审核后流转动作。

这样人工 review 不是聊天里的一句话，而是状态机门禁的一部分。

## 推荐研发流程状态机

下面是一条软件研发需求的参考状态机。

### 1. Requirement Intake

目标：接收需求，记录原始背景。

输入：

```yaml
title:
requester:
background:
problem:
expected_outcome:
priority:
deadline:
related_links:
```

输出：

```yaml
intake_summary:
missing_information:
suggested_owner:
suggested_workflow:
```

执行者：

- Human PM。
- PM Agent。

流转：

- 信息完整 -> Product Clarification。
- 信息不足 -> Waiting For Human Input。

### 2. Product Clarification

目标：把模糊需求转成可评审的产品 spec。

输入：

```yaml
intake_summary:
business_goal:
users_affected:
constraints:
non_goals:
existing_references:
```

输出：Product Spec。

```yaml
product_spec:
  problem_statement:
  goals:
  non_goals:
  user_stories:
  acceptance_criteria:
  UX_notes:
  risks:
  rollout_notes:
open_questions:
```

执行者：

- PM Agent 起草。
- Human PM 或负责人审核。

流转：

- Product Spec 完整 -> Product Spec Review。
- 缺关键判断 -> Waiting For Human Input。

### 3. Product Spec Review

目标：人确认产品方向、范围和验收标准。

输入：

```yaml
product_spec:
open_questions:
```

输出：

```yaml
review_result: approved | changes_requested | rejected
review_comments:
scope_changes:
approval_by:
```

执行者：

- Human PM。
- 项目负责人。
- 业务负责人。

流转：

- approved -> Engineering Investigation。
- changes_requested -> Product Clarification。
- rejected -> Closed。

### 4. Engineering Investigation

目标：开发 agent 调研实现路径，输出开发方案 spec。

输入：

```yaml
product_spec:
repo:
related_modules:
technical_constraints:
```

输出：Engineering Spec。

```yaml
engineering_spec:
  affected_files:
  architecture_notes:
  implementation_plan:
  data_model_changes:
  api_changes:
  UI_changes:
  migration_plan:
  test_plan:
  risks:
  rollout_plan:
open_questions:
```

执行者：

- Developer Agent。
- Architect Agent。

流转：

- Engineering Spec 完整 -> Engineering Spec Review。
- 缺上下文或权限 -> Waiting For Human Input / Tool Grant Required。

### 5. Engineering Spec Review

目标：人或高级 agent 审核开发方案。

输入：

```yaml
engineering_spec:
product_spec:
```

输出：

```yaml
review_result: approved | changes_requested | rejected
review_comments:
required_changes:
approval_by:
```

执行者：

- Human Tech Lead。
- Senior Developer Agent。
- Reviewer Agent。

流转：

- approved -> Implementation。
- changes_requested -> Engineering Investigation。
- rejected -> Product Clarification 或 Closed。

### 6. Implementation

目标：开发 agent 按方案实现代码。

输入：

```yaml
engineering_spec:
approved_scope:
repo:
branch_policy:
```

输出：

```yaml
implementation_summary:
changed_files:
tests_added:
tests_run:
known_risks:
pull_request:
```

执行者：

- Developer Agent。

流转：

- PR 创建成功 -> PR Review。
- 测试失败 -> Implementation。
- 权限不足 -> Tool Grant Required。
- 任务过大 -> Split Work。

### 7. PR Review / Merge

目标：审核代码并合并到测试环境。

输入：

```yaml
pull_request:
implementation_summary:
tests_run:
```

输出：

```yaml
review_result: approved | changes_requested | rejected
review_comments:
merge_result:
test_environment:
```

执行者：

- Human Reviewer。
- Reviewer Agent。

流转：

- approved and merged -> Test Case Design。
- changes_requested -> Implementation。
- rejected -> Engineering Investigation。

### 8. Test Case Design

目标：根据产品 spec 和开发方案生成测试用例。

输入：

```yaml
product_spec:
engineering_spec:
pull_request:
test_environment:
```

输出：

```yaml
test_cases:
  - id:
    title:
    preconditions:
    steps:
    expected_result:
    priority:
    type: functional | regression | edge_case | integration
coverage_notes:
```

执行者：

- QA Agent。
- Human QA。

流转：

- 测试用例完整 -> Test Execution。
- 测试用例需要确认 -> Human QA Review。

### 9. Test Execution

目标：逐条执行测试用例。

输入：

```yaml
test_cases:
test_environment:
build_version:
```

输出：

```yaml
test_results:
  - case_id:
    status: passed | failed | blocked | skipped
    evidence:
    notes:
bugs_found:
```

执行者：

- QA Agent。
- Human QA。

流转：

- 全部通过 -> Test Report。
- 有失败 -> Bug Fix Loop。
- 有阻塞 -> Waiting For Human Input / Environment Fix。

### 10. Bug Fix Loop

目标：把测试发现的问题交回开发修复。

输入：

```yaml
bugs_found:
failed_test_cases:
evidence:
```

输出：

```yaml
fix_summary:
pull_request:
tests_run:
```

执行者：

- Developer Agent。

流转：

- 修复完成 -> PR Review / Merge。
- 无法复现 -> Human QA Review。

### 11. Test Report

目标：输出可审核的测试报告。

输入：

```yaml
test_results:
bugs_found:
product_spec:
engineering_spec:
```

输出：

```yaml
test_report:
  summary:
  coverage:
  passed_count:
  failed_count:
  blocked_count:
  known_issues:
  release_risk:
  recommendation: ready_to_release | hold | need_more_testing
```

执行者：

- QA Agent 起草。
- Human QA 审核。

流转：

- ready_to_release -> Release Preparation。
- hold -> Bug Fix Loop 或 Product Clarification。

### 12. Release Preparation

目标：准备上线材料。

输入：

```yaml
test_report:
pull_request:
release_scope:
deployment_notes:
```

输出：

```yaml
release_package:
  release_notes:
  deployment_plan:
  rollback_plan:
  monitoring_plan:
  owner:
```

执行者：

- Release Agent。
- DevOps / Human Operator。

流转：

- 材料完整 -> Release Approval。
- 材料缺失 -> Waiting For Human Input。

### 13. Release Approval

目标：人工确认上线。

输入：

```yaml
release_package:
test_report:
known_risks:
```

输出：

```yaml
approval_result: approved | rejected | delayed
approval_by:
comments:
```

执行者：

- Human Release Owner。

流转：

- approved -> Release Execution。
- rejected / delayed -> Release Preparation 或 Closed。

### 14. Release Execution

目标：执行上线并记录结果。

输入：

```yaml
release_package:
approval_result:
```

输出：

```yaml
deployment_result:
  status: success | failed | rolled_back
  version:
  evidence:
  incidents:
```

执行者：

- DevOps。
- Release Agent。

流转：

- success -> Retrospective。
- failed -> Rollback / Incident Handling。

### 15. Retrospective And Memory

目标：复盘并沉淀流程资产。

输入：

```yaml
workflow_run:
human_interventions:
failures:
test_report:
deployment_result:
```

输出：

```yaml
retrospective:
  what_worked:
  what_failed:
  repeated_human_interventions:
  prompt_updates:
  skill_candidates:
  task_template_updates:
  workflow_rule_updates:
```

执行者：

- PM Agent。
- Process Agent。
- Human Owner。

流转：

- 沉淀完成 -> Closed。
- 需要确认规则变更 -> Human Review。

## 状态流转原则

### Agent 可以请求流转，但不能随意流转

Agent 完成当前节点后，应该发起 transition request：

```yaml
from_step: implementation
to_step: pr_review
requested_by: project/backend-agent
reason: "实现完成，自测通过，已创建 PR"
output_ref:
  pull_request:
  implementation_summary:
checks:
  output_schema_valid: true
  required_tests_present: true
review_required: true
```

系统根据 workflow definition 判断：

- output schema 是否完整。
- 当前 agent 是否有权限请求该流转。
- 是否需要人工 review。
- 下一个节点分配给谁。

### 人工审核是门禁，不是同步阻塞

人工 review 节点应该异步化：

```text
Agent 请求 review
-> 系统生成 review item
-> Human 在 Web / 飞书 / Lark 中 approve 或 request changes
-> 系统继续流转或回退
```

人不需要全程在线调用 agent，但关键门禁必须由人确认。

### 状态机应支持循环

真实研发流程不是线性的。

常见循环：

```text
Product Spec Review -> Product Clarification
Engineering Spec Review -> Engineering Investigation
PR Review -> Implementation
Test Execution -> Bug Fix Loop -> PR Review -> Test Execution
Release Approval -> Release Preparation
```

状态机必须显式支持这些回路。

### 状态机应支持拆分

大需求可能需要拆成多个子 workflow run：

```text
主需求
-> frontend 子任务
-> backend 子任务
-> QA 子任务
-> release 子任务
```

父 workflow 只在所有必要子 workflow 达到指定状态后继续。

### 状态机应支持并行

有些节点可以并行：

```text
Engineering Investigation
-> Frontend Investigation
-> Backend Investigation
-> QA Test Case Draft
```

系统需要知道哪些并行节点全部完成后才能进入下一阶段。

并行不应该只靠多个任务同时存在来表达，而应该在 workflow definition 里显式建模。

推荐两种表达方式。

#### Parallel Group

当一个阶段拆成多个并行节点时，可以定义 parallel group：

```yaml
step:
  id: implementation_parallel
  type: parallel
  branches:
    - id: frontend_implementation
      actor_role: frontend_agent
    - id: backend_implementation
      actor_role: backend_agent
    - id: qa_test_case_draft
      actor_role: qa_agent
  join_policy: all_success
  next: integration_test
```

`join_policy` 可以支持：

- `all_success`：所有分支成功后进入下一步。
- `any_success`：任一分支成功后进入下一步。
- `quorum`：达到指定数量或比例后进入下一步。
- `manual_join`：由人确认是否汇合。

研发流程中最常见的是 `all_success` 或 `manual_join`。

#### Child Workflow Runs

当一个大需求被拆成多个子需求时，每个子需求可以是独立的 child workflow run：

```yaml
parent_task: feature_checkout_redesign
child_workflows:
  - task: frontend_checkout_ui
    workflow: software_delivery_v1
  - task: backend_checkout_api
    workflow: software_delivery_v1
  - task: qa_checkout_regression
    workflow: qa_validation_v1
join_condition:
  required_children:
    - frontend_checkout_ui
    - backend_checkout_api
    - qa_checkout_regression
  required_state: completed
```

这种方式适合：

- 子需求需要独立 owner。
- 子需求有独立 PR / 测试 / review。
- 子需求可能跨多个 agent 或团队。
- 子需求完成时间不一致。

父 workflow 可以有一个 `Coordination / Integration` 节点，负责等待和汇总子 workflow 的结果。

### 并行节点的输入输出

并行节点也必须结构化。

例如前后端并行开发：

Frontend branch 输出：

```yaml
frontend_summary:
changed_files:
ui_states:
screenshots:
tests_run:
risks:
```

Backend branch 输出：

```yaml
backend_summary:
api_changes:
data_model_changes:
changed_files:
tests_run:
risks:
```

QA branch 输出：

```yaml
test_case_draft:
coverage:
open_questions:
```

Join 节点输入就是这些 branch outputs：

```yaml
frontend_output:
backend_output:
qa_output:
integration_risks:
```

Join 节点可以由 agent 或 human 判断是否进入联调 / 集成测试。

## 通用流程引擎抽象

研发流程只是一个落地场景。Multigent 真正需要的是一个围绕 Task 的通用流程引擎。

推荐抽象如下：

```text
Task
  -> Workflow Run
      -> Step Instance
          -> Child Task / Review Item / Artifact / Agent Run
```

其中：

- Task 是用户理解的工作项。
- Workflow Run 是这个 Task 当前采用的流程执行实例。
- Step Instance 是流程节点的运行记录。
- Child Task 是分配给具体 human 或 agent 的执行单元。
- Review Item 是人工门禁。
- Artifact 是结构化产物。

### Task 创建时选择流程

Task 创建时应该允许选择一个 workflow definition，但这不意味着每个 Task 都按同一条完整流程跑到底。

推荐产品形态：

```text
创建 Task
  -> 选择任务类型：需求 / Bug / 调研 / 发布 / 客服问题
  -> 系统推荐流程模板
  -> 用户确认或切换流程
  -> Workflow Run 启动
```

示例：

```yaml
task_type: feature
workflow_definition: software_delivery_v1
workflow_mode: auto
```

`workflow_mode` 可以支持：

- `auto`：系统根据复杂度自动裁剪流程。
- `simple`：轻量流程，适合小 bug、小改动。
- `standard`：标准流程，适合普通需求。
- `strict`：强门禁流程，适合高风险发布或客户项目。

这样用户不用每次从零设计流程，但也不会被迫为小需求走完整研发流程。

### 运行时路由与流程裁剪

同一个 workflow definition 内部应该支持路由节点。

流程可以先进入一个 `routing` 或 `system_gate` 节点，由系统、PM agent 或 human 判断：

```text
这个 Task 是小改动，还是需要拆分的大需求？
是否需要产品 spec？
是否需要开发方案 review？
是否需要并行开发？
是否需要 QA 完整测试？
是否需要上线审批？
```

推荐把“是否并行”作为运行时决策，而不是写死在每条需求必经路径里。

示例：

```yaml
  - id: complexity_routing
    type: system_gate
    title: 复杂度路由
    input_schema: task_intake_output_v1
    rules:
      - when: complexity == "small"
        to: simple_implementation
      - when: complexity == "medium"
        to: standard_engineering_spec
      - when: complexity == "large" or split_required == true
        to: decomposition
```

对于小需求：

```text
Requirement Intake
-> Complexity Routing
-> Simple Implementation
-> PR Review
-> Done
```

对于中等需求：

```text
Requirement Intake
-> Product Clarification
-> Engineering Investigation
-> Implementation
-> QA
-> Done
```

对于大需求：

```text
Requirement Intake
-> Product Clarification
-> Engineering Investigation
-> Decomposition
-> Parallel Group / Child Workflow Runs
-> Integration
-> QA
-> Release
```

这里的关键是：

```text
Workflow Definition 是一张可路由的流程图；
Workflow Run 是某个 Task 在这张图上实际走过的路径。
```

因此可视化时也应该区分：

- Definition Graph：完整可选路径。
- Run Path：本次实际走过的路径。
- Skipped Steps：本次被裁剪或跳过的节点。

### 并行与子 Task 的关系

并行 workflow 不应该是所有需求的默认路径。

它只在运行时路由判断需要拆分时出现。

并行 workflow 不一定都要创建 child workflow，但通常都应该创建 child task。

推荐规则：

```text
一个流程节点需要某个 agent 或 human 实际执行
-> 创建一个 child task。

一个大需求拆成独立子需求，每个子需求有自己的生命周期、owner、PR、测试和 review
-> 创建 child task，并为这个 child task 启动 child workflow run。
```

也就是说：

- Parallel Group 的每个分支通常对应一个 child task。
- Child Workflow Run 一定挂在某个 child task 上。
- 父 Task 的 workflow run 不直接替代子 task；它负责协调、等待、聚合和推进。

示例：

```text
父 Task：实现 Web Search 外部工具
  Workflow Run：研发交付流程
    Step：并行实现
      Child Task A：实现 Brave Search 工具接入
      Child Task B：实现 Exa Search 工具接入
      Child Task C：补充 Web Search 测试用例
```

如果 A/B/C 都只是当前阶段的并行工作，可以只创建 child task，不启动 child workflow run。

如果 Brave Search 和 Exa Search 都变成独立需求，各自需要产品确认、开发方案、PR、测试报告，则 A/B 应该各自启动 child workflow run。

### Decomposition Step

为了兼容“有些需求需要拆，有些不需要拆”，可以单独定义一个 decomposition 节点。

这个节点的职责不是执行开发，而是判断是否拆分，以及拆成什么。

输入：

```yaml
task:
product_spec:
engineering_spec:
constraints:
```

输出：

```yaml
decomposition:
  split_required: true | false
  reason:
  branches:
    - title:
      owner_role:
      suggested_agent:
      workflow_definition:
      input:
      dependency:
  join_policy: all_success | manual_join
```

如果 `split_required = false`：

```text
Decomposition -> Simple / Standard Implementation
```

如果 `split_required = true`：

```text
Decomposition -> Parallel Group 或 Child Workflow Runs
```

这样并行是流程运行时的展开结果，而不是每个需求都固定经过的步骤。

### Step 类型

为了避免过度设计，第一版 step 类型可以控制在以下几类：

```yaml
step_types:
  - agent_task
  - human_task
  - human_review
  - system_gate
  - routing
  - decomposition
  - parallel_group
  - join
  - child_workflow
  - terminal
```

含义：

- `agent_task`：创建 child task，分配给 agent 执行。
- `human_task`：创建 child task，分配给 human 执行。
- `human_review`：创建 review item，等待人审核。
- `system_gate`：系统校验 schema、权限、产物、条件。
- `routing`：根据任务复杂度、风险和输入完整度选择路径。
- `decomposition`：判断是否拆分，并生成并行分支或子流程计划。
- `parallel_group`：展开多个并行分支。
- `join`：等待并行分支或 child workflow 汇合。
- `child_workflow`：创建 child task，并启动新的 workflow run。
- `terminal`：流程结束。

### Workflow Definition 最小结构

第一版可以用 JSON/YAML 描述，不急着做复杂可视化编辑器。

```yaml
id: software_delivery_v1
name: 软件研发交付流程
version: 1
entity_type: task
start_step: requirement_intake
steps:
  - id: requirement_intake
    type: agent_task
    title: 需求输入整理
    actor:
      role: pm
    input_schema: requirement_intake_input_v1
    output_schema: requirement_intake_output_v1
    next:
      - to: complexity_routing

  - id: complexity_routing
    type: routing
    title: 复杂度路由
    rules:
      - when: complexity == "small"
        to: simple_implementation
      - when: complexity == "medium"
        to: engineering_spec
      - when: complexity == "large"
        to: decomposition

  - id: implementation_parallel
    type: parallel_group
    title: 并行实现
    branches:
      - step: frontend_implementation
      - step: backend_implementation
      - step: qa_case_draft
    join:
      policy: all_success
      to: integration_review
```

### Workflow Run 最小结构

```yaml
id:
task_id:
workflow_definition_id:
workflow_version:
status: active | completed | failed | cancelled
active_step_ids:
parent_run_id:
parent_step_instance_id:
started_at:
finished_at:
metrics:
```

`active_step_ids` 支持并行节点，因为一个 workflow run 在同一时间可能有多个活跃 step。

### Step Instance 最小结构

```yaml
id:
workflow_run_id:
step_id:
status: pending | running | waiting_review | blocked | completed | failed | cancelled
actor_type: human | agent | system
actor_id:
child_task_id:
review_item_id:
input_artifact_ids:
output_artifact_ids:
agent_run_ids:
started_at:
finished_at:
token_usage:
error:
```

这个结构能同时覆盖：

- agent 执行。
- human 执行。
- 人工 review。
- schema gate。
- 并行分支。
- child workflow。

## 可视化设计

流程引擎的可视化不应该只做成一个 Kanban。Kanban 只能表达“当前任务处于哪个状态”，但表达不了：

- 并行分支。
- 子流程。
- 人工 review 门禁。
- 输入输出产物。
- 流转原因。
- token 和耗时。
- 哪个节点导致阻塞。

推荐第一版做成 **Task 内的 Workflow View**。

### 页面结构

```text
Task Detail
  Header: 标题 / owner / workflow 状态 / 总耗时 / 当前活跃节点
  Main: Workflow Graph
  Right Panel: Step Inspector
  Bottom Panel: Timeline / Artifacts / Metrics
```

### Workflow Graph

Graph 展示 workflow definition，并叠加 workflow run 状态。

节点类型：

- Agent Task：agent 执行节点。
- Human Task：人执行节点。
- Review：人工审核节点。
- Gate：系统校验节点。
- Parallel Group：并行展开节点。
- Join：汇合节点。
- Child Workflow：可折叠子流程节点。
- Terminal：结束节点。

节点状态：

```text
not_started
active
waiting_review
blocked
completed
failed
cancelled
```

图上应该能直接看到：

- 当前活跃节点高亮。
- 已完成节点置灰或打勾。
- 阻塞节点用明确的 blocked 状态。
- 人工 review 节点显示待审核人。
- 并行组显示各分支完成比例。
- child workflow 节点显示子 Task 数和完成数。

### Step Inspector

点击节点后，右侧展示：

```text
节点目的
执行者
输入 schema
实际输入
输出 schema
实际输出
关联 child task
关联 agent run
关联 review item
关联 artifacts
错误和阻塞原因
下一步允许动作
```

这比只看聊天记录更适合客户理解流程，也方便人介入调教。

### Timeline

Timeline 展示 workflow run 的事件流：

```text
Workflow started
Step started
Child task created
Agent run started
Artifact produced
Transition requested
Review requested
Review approved
Step completed
Parallel branch joined
Workflow completed
```

Timeline 是审计和复盘的基础。

### Metrics Overlay

Graph 或 Inspector 上应该显示关键指标：

- 节点耗时。
- 等人耗时。
- agent 执行耗时。
- token 消耗。
- retry 次数。
- review 次数。
- schema 校验失败次数。

这样流程图不只是“好看”，而是能帮助客户看出流程瓶颈。

### 可视化编辑器的边界

第一版不建议直接做复杂拖拽式流程编辑器。

推荐顺序：

1. 先支持只读 Workflow Graph，从 workflow definition 自动渲染。
2. 支持在表单里编辑 step、transition、schema、reviewer。
3. 支持从模板创建 workflow definition。
4. 后续再做拖拽式编辑器。

这样可以先保证流程引擎的数据模型正确，而不是过早陷入画布交互复杂度。

### 前端实现建议

如果做 React 版本，Workflow Graph 可以基于 React Flow 实现。

React Flow 适合：

- 节点和边。
- 自定义节点。
- 子流程 / 分组节点。
- 交互式 Inspector。
- 后续升级成编辑器。

但第一版要把 React Flow 当成展示层，不要让画布状态成为流程定义的真实数据源。真实数据源应该是后端的 workflow definition 和 workflow run。

## 参考系统观察

### Plane

Plane 的核心还是 project / issue / state / cycle / module / view。

它对我们的启发是：

- Task/Issue 仍然应该是用户的主要入口。
- 状态、周期、模块、视图适合做项目管理。
- 默认状态组可以保持简单。

但 Plane 没有解决 agent-human 协作流程的问题：

- 没有节点级输入输出 schema。
- 没有 agent run 和 token 绑定。
- 没有人工 review 作为流程门禁。
- 没有 child workflow 的执行模型。

所以 Multigent 不应该把 workflow 简化成 issue state。

### Huly

Huly 的 process 设计更接近我们要的流程引擎。

可借鉴点：

- Process / State / Transition / Execution 分层。
- Transition 上挂 actions。
- Execution 有日志。
- ToDo 和 ApproveRequest 可以作为人工节点。
- SubProcess 和 parent execution 可以表达子流程。
- Context DSL 用来引用流程上下文。

但 Multigent 的核心对象不同。我们要围绕 Task、Agent Run、Artifact、Review Item 和 Token 指标设计，而不是直接照搬 Huly 的 card/process 模型。

### BPMN / Camunda

BPMN 的成熟概念值得借鉴：

- Task。
- User Task。
- Service Task。
- Gateway。
- Parallel Gateway。
- Subprocess。
- Process Instance。

但 BPMN 对普通客户和 agent 产品来说太重。Multigent 可以吸收它的表达能力，但不直接把 BPMN 作为第一版用户界面。

### Temporal

Temporal 的 child workflow 模型适合我们内部理解：

```text
Parent workflow run
  -> Child workflow run
```

它强调 durable execution、parent/child 关系、child workflow 生命周期策略。

Multigent 可以借鉴这种执行语义，但用户侧仍然展示为：

```text
父 Task
  -> 子 Task
      -> 子流程
```

### Airflow

Airflow 的 DAG、TaskGroup、Dynamic Task Mapping 对并行可视化有参考价值：

- DAG 图适合展示依赖关系。
- TaskGroup 适合降低复杂图的视觉噪音。
- 动态任务映射说明并行任务可以在运行时根据上游输出展开。

但 Airflow 更偏数据管道，不适合直接作为人和 agent 协作模型。

## 流程评估指标

协作状态机必须自带评估指标，否则客户无法判断流程是否真的变好了。

指标应该覆盖 workflow、step、agent、human intervention 和 token/resource 几个层面。

### Workflow 级指标

用于评估一个需求从进入到完成的整体效率。

```yaml
workflow_started_at:
workflow_finished_at:
total_elapsed_time:
active_execution_time:
waiting_human_time:
waiting_agent_time:
blocked_time:
rework_count:
transition_count:
review_count:
success_rate:
```

重点不是只看总耗时，而是拆出：

- agent 实际执行时间。
- 人工等待时间。
- 阻塞时间。
- 返工次数。

### Step 级指标

每个节点都应该能评估：

```yaml
step_started_at:
step_finished_at:
elapsed_time:
actor_type:
actor_id:
run_count:
retry_count:
review_required:
review_wait_time:
schema_validation_failures:
blocked_reason:
```

这可以回答：

- 哪个阶段最慢。
- 哪个阶段最常返工。
- 哪个阶段经常缺输入。
- 哪个阶段经常需要人介入。

### Token 与运行消耗

每个 step 和 workflow 都应该聚合 token 和运行成本指标。

```yaml
input_tokens:
output_tokens:
total_tokens:
model:
provider:
run_count:
tool_call_count:
connection_usage:
```

即使美元成本不一定精确，也应该至少能评估：

- 哪些节点 token 消耗高。
- 哪些 agent 经常重复读取上下文。
- 哪些任务模板能降低 token。
- 哪些流程拆分能减少无效消耗。

### Human Intervention 指标

记录人介入的原因和频率：

```yaml
human_intervention_count:
intervention_reasons:
repeated_intervention_patterns:
average_response_time:
converted_to_rule_count:
converted_to_skill_count:
converted_to_template_count:
```

这直接对应 Multigent 的核心价值：

```text
人不是同步阻塞点，而是审核者、调教者和规则沉淀者。
```

如果某类介入重复出现，系统应该建议沉淀：

```text
最近 5 个 Engineering Spec Review 中，有 4 次 reviewer 要求补充 rollback_plan。
是否把 rollback_plan 加入 Engineering Spec 必填字段？
```

### 并行效率指标

并行流程需要额外指标：

```yaml
parallel_group_elapsed_time:
branch_elapsed_times:
slowest_branch:
blocked_branches:
join_wait_time:
integration_rework_count:
```

这可以回答：

- 并行是否真的缩短了总耗时。
- 哪个分支成为瓶颈。
- join 阶段是否因为上下文不一致而返工。
- 是否应该把某个并行节点提前或延后。

### 指标的产品展示

Web 上可以在 workflow run 页面展示：

- 总耗时。
- 当前阶段耗时。
- 人工等待时间。
- Agent 执行时间。
- Token 消耗。
- 返工次数。
- 阻塞原因分布。
- 最慢节点。
- 重复人工介入建议。

这些指标既服务客户 ROI，也服务流程调教。

## Web 产品形态

状态机应该在 Web 上成为项目或需求的核心视图。

### Flow View

展示整条流程：

- 当前节点。
- 已完成节点。
- 等待人工 review 的节点。
- 阻塞节点。
- 自动流转节点。
- 回退路径。

### Step Detail

点击节点后展示：

- 节点目的。
- 当前执行者。
- 输入。
- 输出。
- schema 校验结果。
- 关联 run。
- 关联消息。
- 关联 artifacts。
- review 状态。
- 下一步可选动作。

### Review Panel

人类 reviewer 应该能直接处理：

- approve。
- request changes。
- reject。
- reassign。
- ask agent。
- update workflow rule。

### Artifact Panel

每个阶段产物都应该结构化展示：

- 产品 Spec。
- 开发方案 Spec。
- PR。
- 测试用例。
- 测试报告。
- 发布包。
- 复盘报告。

这些产物应该可以被后续节点引用，而不是散落在聊天记录里。

## 与模板系统的关系

模板负责生成默认 workflow definition。

状态机负责执行和治理 workflow run。

```text
Project Playbook Template
-> Workflow Definition
-> Workflow Run
-> Step Instance
-> Artifacts / Reviews / Transitions
-> Retrospective
-> Template Update Proposal
```

模板是起点，不是终点。

真实流程会通过运行、失败、人工介入和复盘不断更新。

## 当前产品差距

要落地协作状态机，Multigent 需要补以下能力：

1. Workflow Definition 数据模型。
2. Workflow Run / Step Instance 数据模型。
3. Transition Request 和 Review Item。
4. 节点级 input_schema / output_schema。
5. Artifact 存储和引用。
6. Web Flow View。
7. Agent 在运行时调用 `request_transition` 的能力。
8. Human 在 Web / 飞书 / Lark 中处理 review 的能力。
9. Run 失败后自动关联到当前 step。
10. Retrospective 后生成 workflow/template 更新建议。

## 一句话总结

Multigent 的状态机不是为了模拟项目管理工具的任务状态，而是为了定义：

```text
人和 agent 如何围绕一个真实需求协作；
每个阶段必须输入什么、输出什么；
agent 何时可以自动推进；
何时必须请求人类审核；
如何从失败和人工介入中持续优化流程。
```

这会比单纯提供模板更可控，也更能让客户看到 Multigent 如何把公司流程变成 agent 可运行的流程。
