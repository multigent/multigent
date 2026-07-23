# Multigent RBAC Architecture

Multigent 的权限模型不能只复制传统项目管理工具的「谁能看项目」。在 Agent 团队里，真正敏感的边界是：

- 谁能创建、运行、暂停和调教 Agent。
- Agent 能读取哪些 Context。
- 谁对 Agent 的输出质量负责。
- 谁能把临时经验提升为长期记忆。
- Worker 能不能接入本地或私有资源。

因此 Multigent 的 RBAC 设计采用 Plane 和 Huly/Platform 的折中方案：MVP 保持角色简单，底层能力用稳定 capability 表达，后续 SaaS 商业版可以逐步开放自定义权限。

## References

Plane 的模型值得参考它的简单性：

- Workspace / Project 两层成员关系。
- `Admin / Member / Guest` 粗粒度角色。
- 后端按接口动作鉴权，前端只控制展示入口。

Huly/Platform 的模型值得参考它的抽象：

- 全局账号角色有层级权重。
- Space 有成员、owner、private 等可见性边界。
- Role 和 Permission 分离。
- Capability 可配置，权限由 middleware 统一拦截。

Multigent 不应该一开始照搬 Huly 的通用对象权限系统。我们先做静态角色和 capability 内核，再逐步把 API、任务调度、Agent 执行接入。

## Product Model

新的产品层级收敛为：

```text
Workspace
  Team
  Project
    Agent
    Task
```

- Workspace 是公司、客户或租户边界，承载成员、账单、全局设置、集成、Context Pack 和安全策略。
- Team 是 workspace 内的扁平职能组织结构，例如 Engineering、Product、QA。复杂拆分交给 Project、Task、Role、label 和 milestone。
- Project 是 agent 协作上下文边界，例如一个产品、客户交付、代码库或长期业务方向。
- Task 是需求、Bug、测试、联调、发版和调研的执行边界。
- Agent 是项目内的角色化执行者，由 Agent Owner 负责调教、评估和授权。

Multigent 不引入独立的 Workstream 概念。复杂需求通过 task graph 表达：`parent_task_id`、`depends_on`、`label`、`milestone/release`、`assignee`、`status` 和 `reviewer` 足够覆盖前端、后端、QA、发版等分工。这样更接近 Plane、Huly、Linear、Jira 的用户心智，也避免在 Project 和 Task 之间增加一层难以解释的抽象。

## Resource Model

权限资源分为七类：

| Resource | Purpose |
| --- | --- |
| Workspace | 公司 / 租户级别的成员、账单、集成、全局设置 |
| Team | 扁平职能团队 |
| Project | 产品、客户交付、长期业务方向 |
| Task | 项目内具体需求、Bug、测试、联调、发版和调研任务 |
| Agent | 云端或本地同步出来的 Agent 同事 |
| Context Pack | 可版本化、可授权给 Agent 读取的上下文包 |
| Worker | 本地或云端执行节点，负责跑 Agent job |

Workspace 是商业和安全边界，Project 是协作上下文边界，Task 是执行边界，Agent 是能力和责任边界，Context Pack 是知识和数据边界。

## Role Layers

### Workspace Roles

| Role | Meaning |
| --- | --- |
| Owner | Workspace 最终负责人，可以管理账单、所有权限和所有资源 |
| Admin | Workspace 管理员，可以管理成员、团队、项目、Agent、Context、Worker 和集成 |
| Member | 普通成员，可以参与被授权的项目和 Agent |
| Guest | 外部协作者，默认只读或无权限 |

### Project Roles

| Role | Meaning |
| --- | --- |
| Manager | 管理项目范围、成员、Agent 分配、任务拆解和里程碑 |
| Operator | 可以操作项目内 Agent、创建和推进任务、处理日常协作 |
| Viewer | 只读查看项目状态、任务和产出 |

### Task Roles

| Role | Meaning |
| --- | --- |
| Owner | 对任务目标、验收标准、拆解和最终关闭负责 |
| Assignee | 执行任务的人或 Agent，可以更新状态和产出 |
| Reviewer | 审核任务结果，可以批准、打回或要求补充验证 |
| Viewer | 只读查看任务状态、讨论和产出 |

### Agent Roles

| Role | Meaning |
| --- | --- |
| Owner | 对 Agent 表现负责，可以编辑 prompt/context、批准记忆、调教输出 |
| Operator | 可以运行、暂停、派任务、查看执行 |
| Viewer | 只能查看 Agent 状态、结果和运行记录 |

Agent Owner 是 Multigent 的关键角色。人的职责不是每一步同步驱动 Agent，而是用专业知识训练、约束和评估对应 Agent，让 Agent 逐渐能独立完成更多工作。

### Context Pack Roles

| Role | Meaning |
| --- | --- |
| Maintainer | 维护版本、合并、废弃、批准长期记忆 |
| Contributor | 提交或更新上下文候选内容 |
| Viewer | 允许查看，或允许被授权 Agent 读取 |

Context Pack 必须单独授权。不能因为用户在某个项目里，就默认能读取所有上下文；也不能因为 Agent 在某个项目里，就默认能读所有公司知识。

## Capability Model

代码里使用稳定 capability ID，而不是在业务逻辑里散落 `role == admin` 判断。

第一批 capability：

| Capability | Meaning |
| --- | --- |
| `workspace.manage_members` | 管理 workspace 成员 |
| `workspace.manage_billing` | 管理账单和商业设置 |
| `team.manage` | 创建和管理团队 |
| `project.create` | 创建项目 |
| `project.read` | 查看项目 |
| `project.manage` | 管理项目设置 |
| `project.manage_members` | 管理项目成员 |
| `task.create` | 创建任务 |
| `task.read` | 查看任务 |
| `task.update` | 更新任务状态、说明和产出 |
| `task.assign` | 分配任务给人或 Agent |
| `task.review` | 审核任务结果 |
| `agent.create` | 创建或导入 Agent |
| `agent.assign_owner` | 分配 Agent Owner |
| `agent.view` | 查看 Agent 状态和产出 |
| `agent.run` | 运行、唤醒或派任务给 Agent |
| `agent.pause` | 暂停 Agent |
| `agent.edit_prompt` | 编辑 Agent prompt、角色上下文和调教材料 |
| `agent.approve_memory` | 批准 Agent 记忆沉淀 |
| `context.read` | 查看或允许 Agent 读取 Context |
| `context.write` | 新增或更新 Context |
| `context.promote_memory` | 把候选记忆提升为长期 Context |
| `worker.register` | 注册本地或云端 Worker |
| `worker.dispatch` | 向 Worker 下发 job |
| `integration.configure` | 配置外部系统集成 |

## Permission Resolution

一次权限判断按以下顺序执行：

1. System actor 直接放行，仅用于内部任务和迁移。
2. Workspace Owner 放行所有 capability。
3. Workspace Admin 放行大部分管理 capability，但账单和 Owner 级动作保留给 Owner。
4. 检查资源上的显式角色，例如 Project Manager、Agent Owner、Context Maintainer。
5. 检查父级继承，例如 Task 默认继承 Project 可见性，但可被私有任务或敏感 Context 限制收紧。
6. Context Pack 和 Agent 的显式授权优先于泛化项目权限。
7. 后端返回最终结果；前端只负责隐藏按钮，不是安全边界。

## Safety Rules

必须内置以下保护：

- 不能删除或降级最后一个 Workspace Owner。
- 用户不能授予高于自己权限的角色。
- 用户不能修改权限高于自己的用户。
- Worker 注册和远程执行必须独立授权。
- Agent prompt、context、owner、autonomy level、memory approval 的变更必须写 audit log。
- Context Pack 的读取和提升必须可追溯。

## MVP Implementation Plan

### Phase 1: RBAC Kernel

- 新增 `internal/rbac`。
- 定义角色、capability、resource、principal 和 authorizer。
- 兼容现有 Web API 的 `admin/member` 与 `manager/operator/viewer`。
- 增加单元测试覆盖角色层级、继承、Agent Owner、Context 独立授权和 grant guard。

### Phase 2: API Adoption

- 把现有 API 中散落的 `RoleAdmin`、`HasProjectAccess`、`projectRoleLevel` 逐步迁移为 capability check。
- 对高风险接口优先接入：用户管理、Agent run/pause/edit prompt、Context 文档、Worker、Provider/Integration。
- 在 API 层统一返回 denied reason，方便前端解释。

### Phase 3: SaaS Persistence

落库对象建议：

- `users`
- `workspaces`
- `workspace_members`
- `teams`
- `team_members`
- `projects`
- `project_members`
- `tasks`
- `task_relations`
- `task_members`
- `agents`
- `agent_members`
- `context_packs`
- `context_pack_grants`
- `workers`
- `worker_grants`
- `role_capability_overrides`
- `audit_logs`

本地版本和 SaaS 版本都应通过统一的 DB-backed storage interface 管理用户、workspace membership 和授权关系；SQLite 是本地默认实现，后续可替换为 MySQL/Postgres。

## Product Principle

Multigent 的权限系统最终要回答三个问题：

1. 这个人负责哪个 Agent？
2. 这个 Agent 能使用哪些 Context 和工具？
3. 这个流程中哪些动作可以自动做，哪些动作必须让人异步 review？

传统项目管理工具的 RBAC 只能回答「谁能看和改项目」。Multigent 必须进一步回答「谁能组织 Agent 劳动力，以及如何让 Agent 安全地持续工作」。
