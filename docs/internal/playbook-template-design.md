# Playbook Template Design

> 中文产品名暂定：协作方案  
> 英文/代码名：Playbook  
> Status: Draft  
> Scope: template system, workflow templates, team/role templates, skills, onboarding

## Why Playbook

Multigent 现在已经有 team、role、agent、skill、workflow、task、scheduler 和外部工具连接。但这些对象单独存在时，用户仍然需要自己理解：

- 应该建哪些团队和角色。
- 每个角色应该有什么 prompt。
- 哪些 skill 应该给哪些 agent。
- 一个需求应该经过哪些节点。
- 每个节点要产出什么结构化结果。
- 哪些地方需要人审核，哪些地方可以自动流转。

这对熟悉 agent workflow 的用户可以接受，但对企业团队不够友好。

Playbook 要解决的是：

```text
把一类业务协作方式，打包成可安装、可运行、可调整的一整套协作结构。
```

它不是单个 workflow，也不是简单的 team template。它是 roles、skills、workflow、task templates、prompt defaults 和 setup checklist 的组合。

## Naming

中文 UI 暂定叫 **协作方案**。

原因：

- 比“方案”更具体，强调人和 agent 如何协作。
- 比“解决方案”更轻，不像咨询交付包。
- 不占用“工作流”这个词，避免和 workflow 节点编排混淆。
- 后续如果不合适，可以只改 UI 文案，代码仍然保持 `playbook`。

建议命名：

| Layer | English | Chinese |
| --- | --- | --- |
| Playbook | Playbook | 协作方案 |
| Workflow | Workflow | 工作流 |
| Team Template | Team Template | 团队模板 |
| Role Template | Role Template | 角色模板 |
| Skill | Skill | 技能 |
| Task Template | Task Template | 任务模板 |

## Core Distinction

### Workflow

Workflow 只负责表达任务如何流转：

- 节点。
- 连线。
- 条件分支。
- 当前节点。
- 输入输出 spec。
- 运行历史。

Workflow 不应该关心完整团队如何搭建，也不应该内置一整套角色 prompt。

### Skill

Skill 是 agent 的可复用能力说明或操作方法。

例如：

- 如何做 YC 风格市场判断。
- 如何做代码 review。
- 如何做 QA 验证。
- 如何使用 GitHub CLI。
- 如何把输出写入知识库文档。

Skill 可以被多个 role、agent、workflow 节点复用。

### Role

Role 定义一个 agent 应该像什么专业角色。

例如：

- PM。
- Backend Engineer。
- QA。
- Market Analyst。
- Founder Coach。
- Security Reviewer。

Role prompt 是长期身份约束，不应该写具体项目流程。

### Playbook

Playbook 把上面这些组合起来：

```text
Playbook
  -> recommended teams / roles
  -> bundled or referenced skills
  -> workflow templates
  -> task templates
  -> prompt defaults
  -> required external tools
  -> setup questions
  -> success metrics
```

也就是说，Playbook 是“怎么把某类事情做好”的可复用协作结构。

## Garry-Style Example

我们之前把 `garry-style` 做成 workflow template，这个抽象不够准确。

Garry 的核心价值不是“一个开发流程”，而是一组高质量 agent skills 和专家视角。例如 `gstack` 的 `/office-hours` 更像一个 YC office hours advisor：它用一组强约束问题判断一个产品 idea 是否有真实需求、现有替代方案、痛点强度、最小切入点和未来适配性。

这应该被建模成 Playbook：

```text
Garry Startup Validation Playbook
  Roles:
    - YC Office Hours Advisor
    - Founder Strategy Reviewer
    - Market Research Agent
    - Prototype Scope Reviewer

  Skills:
    - status-quo-competitor-analysis
    - desperate-user-signal-check
    - narrowest-wedge-finder
    - future-fit-evaluation
    - 48-hour-prototype-scope

  Workflows:
    - startup-idea-validation

  Task Templates:
    - Validate a startup idea
    - Review market pain evidence
    - Produce a 48-hour prototype plan
```

在这个例子里，workflow 只是其中一部分。真正可复用的是“角色 + 判断 skill + 结构化流转”的组合。

## Product Behavior

### 1. Browse Playbooks

Workspace 管理员可以进入“协作方案”页面，看到内置和自定义 Playbook。

卡片展示：

- 名称。
- 场景。
- 适合团队。
- 包含多少 roles、skills、workflows、task templates。
- 需要哪些外部工具连接。
- 复杂度和推荐使用阶段。

### 2. Install Playbook

安装 Playbook 时，不直接覆盖用户现有对象，而是让用户确认生成哪些内容：

- 创建推荐团队。
- 创建推荐角色。
- 添加内置 skill。
- 创建 workflow。
- 创建 task templates。
- 创建推荐 agent。
- 生成项目 prompt 草稿。
- 生成 wakeup prompt 草稿。

第一版可以简化为：

```text
选择 Playbook
-> 填写名称和目标项目
-> 勾选要创建的内容
-> 预览
-> 安装到 workspace
```

### 3. Create Project From Playbook

更常见的路径是创建项目时选择 Playbook：

```text
Create Project
-> Choose Playbook
-> Answer setup questions
-> Generate project prompt / workflow / task templates
-> Create or bind agents
```

例如软件交付 Playbook 可以问：

- 代码托管用 GitHub 还是 GitLab？
- 文档用飞书、Notion 还是 Multigent Docs？
- 大需求是否必须先过产品审核？
- 技术方案由谁审核？
- agent 是否可以直接提交 PR？
- QA 是否需要人工准出？

### 4. Customize After Install

安装后的 Playbook 会生成实际对象。用户后续改的是实际对象，不再回写内置模板。

这点很重要：

- 模板是 source。
- 用户安装后的 team、role、skill、workflow 是 instance。
- instance 可以随企业流程变化而修改。
- 未来可以支持“保存为自定义 Playbook”。

## Data Model Draft

第一版可以只做内置模板，不急着建复杂数据库表。但数据结构应该按最终状态设计。

```go
type PlaybookTemplate struct {
    ID              string
    Name            string
    Description     string
    Locale          string
    Category        string
    Tags            []string
    Complexity      string
    SetupQuestions  []PlaybookSetupQuestion
    Roles           []PlaybookRoleTemplate
    Skills          []PlaybookSkillTemplate
    Workflows       []PlaybookWorkflowTemplate
    TaskTemplates   []PlaybookTaskTemplate
    RequiredTools   []PlaybookToolRequirement
    SuccessMetrics  []PlaybookMetric
}
```

### PlaybookRoleTemplate

```go
type PlaybookRoleTemplate struct {
    ID          string
    Team        string
    Role        string
    Name        string
    Description string
    Prompt      string
    Skills      []string
}
```

### PlaybookSkillTemplate

```go
type PlaybookSkillTemplate struct {
    ID          string
    Name        string
    Description string
    Body        string
    Source      string
    LicenseNote string
}
```

### PlaybookWorkflowTemplate

```go
type PlaybookWorkflowTemplate struct {
    ID          string
    Name        string
    Description string
    Definition  WorkflowTemplate
    RoleBindings map[string]string
    SkillBindings map[string][]string
}
```

`RoleBindings` 用于说明 workflow 节点倾向由哪个角色执行。例如：

```json
{
  "market_interview": "yc-office-hours-advisor",
  "wedge_selection": "founder-strategy-reviewer",
  "prototype_plan": "prototype-scope-reviewer"
}
```

`SkillBindings` 用于说明某个节点需要哪些 skill：

```json
{
  "market_interview": ["desperate-user-signal-check", "status-quo-competitor-analysis"],
  "prototype_plan": ["48-hour-prototype-scope"]
}
```

### PlaybookTaskTemplate

```go
type PlaybookTaskTemplate struct {
    ID          string
    Title       string
    Description string
    Prompt      string
    WorkflowID  string
    RequiredFields []WorkflowField
}
```

## Runtime Relationship

当 task 绑定 workflow 后：

1. Workflow 决定当前节点。
2. 当前节点决定推荐 actor role。
3. Task 创建时的 actor bindings 决定具体 agent 或 human。
4. Playbook 决定这个 role 默认应具备哪些 skills。
5. Runner materializer 把当前 agent 已授权的 skills、tools、docs、workflow context 注入 sandbox。

所以 Playbook 不直接参与每次运行调度。它的主要作用是生成和维护可运行的配置资产。

## UI Proposal

### Navigation

新增 workspace 级导航：

```text
协作方案
```

它和“工作流”“团队”“技能”平级，但第一版可以只对 admin 可见。

### Playbook List

列表展示：

- 内置协作方案。
- 用户自定义协作方案。
- 已安装次数或当前 workspace 是否已安装。

### Playbook Detail

详情页分区：

- Overview。
- Roles。
- Skills。
- Workflows。
- Task Templates。
- Required Tools。
- Setup Questions。

### Install Modal

安装弹框：

- 输入生成名称前缀。
- 选择目标项目或创建新项目。
- 勾选要生成的对象。
- 显示冲突预览。
- 点击安装。

## Built-In Playbook Candidates

第一批不需要太多，但要质量高。

### 1. Agentic Software Delivery

用于常规软件研发交付。

包含：

- PM、Developer、QA、Reviewer、Release。
- 软件交付 workflow。
- bug fix / feature / QA task templates。
- 默认 Multigent docs/task/mga skills。

### 2. Startup Validation

参考 gstack `/office-hours` 思路，但不要直接复制其实现。

包含：

- YC Office Hours Advisor。
- Market Research Agent。
- Founder Strategy Reviewer。
- Prototype Scope Reviewer。
- startup idea validation workflow。
- idea validation task template。

### 3. Bug Triage And Fix

用于小型 bug 快速闭环。

包含：

- Bug Triage。
- Developer。
- QA。
- root-cause-investigation skill。
- regression-check skill。

### 4. Content Production

用于内容团队。

包含：

- Content Strategist。
- Writer。
- Editor。
- Distribution Operator。
- topic brief -> draft -> review -> publish checklist workflow。

### 5. Customer Support Knowledge Loop

用于客服知识库自动沉淀。

包含：

- Support Triage。
- Knowledge Base Maintainer。
- Product Feedback Analyst。
- ticket summary -> answer draft -> KB update -> product feedback workflow。

## Implementation Plan

### Phase 1: Product Model And Built-In API

- Add `PlaybookTemplate` entity.
- Add built-in playbook registry.
- Add `GET /api/v1/playbook-templates`.
- Add `GET /api/v1/playbook-templates/{id}`.
- Keep persistence out of scope unless user installs a playbook.

### Phase 2: Install Playbook

- Add install endpoint.
- Generate roles, skills, workflows, and task templates.
- Do not overwrite existing objects silently.
- Record audit log: `playbook.install`.

### Phase 3: UI

- Add “协作方案” navigation.
- Add list/detail/install UI.
- Add create project from playbook path.

### Phase 4: Save As Playbook

- Let admin package selected teams, roles, skills, workflows, task templates into a custom playbook.
- Store custom playbooks in DB.
- Support export/import JSON.

## Open Decisions

1. 中文名是否长期使用“协作方案”。
2. Playbook 安装时是否允许覆盖已有 role / skill / workflow。
3. Skill 是否先只支持 Markdown body，还是同时支持附件和脚本。
4. Task Template 是否现在落库，还是先只作为 Playbook 详情展示。
5. Playbook 是否和 project 强绑定，还是 workspace 级安装后可被多个项目复用。

当前建议：

- Playbook template 是 workspace 级可安装模板。
- 安装后生成的 workflow 是 workspace 级，可被多个项目复用。
- 安装后生成的 role/skill 是 workspace 级。
- 创建 task 时仍然选择具体 project 和具体 workflow。

