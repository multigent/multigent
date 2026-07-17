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
