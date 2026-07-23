# Control Plane Assistant Redesign Plan

## 背景

Multigent Web 右下角已经有一个 AI Assistant。它当前更接近本地时代的实现：后端直接寻找宿主机上的 `claude` / `codex` / `gemini` CLI，在 Multigent root 目录执行，并把 `Bash`、`Read`、`Write`、`Edit` 等能力交给 CLI 的权限提示处理。

这不适合作为 SaaS 产品能力。SaaS 里的助手应该是控制面助手：理解用户意图，调用 Multigent 后端定义的受控工具，再由后端执行 RBAC、审计和确认流程。它不应该继承宿主机环境变量，不应该依赖宿主机 CLI 登录态，也不应该直接拥有文件系统和 shell 权限。

## 当前现状

前端：

- 组件：`web/src/components/assistant/AssistantWidget.tsx`
- 入口：`web/src/components/layout/AppShell.tsx`
- API：
  - `POST /api/v1/assistant/chat`
  - `POST /api/v1/assistant/permission`
- 前端会展示 CLI permission request，并允许 `allow` / `deny` / `allowAll`。
- 文案仍有旧概念，例如 “manage your Agency”。

后端：

- 文件：`internal/api/assistant_handlers.go`
- `handleAssistantChat` 当前没有 workspace admin/member 能力分层。
- `resolveAssistantCLI` 自动查找本机 `claude`、`codex`、`gemini`。
- `cmd.Dir = s.root`，`cmd.Env = os.Environ()`。
- Claude 模式使用 `--permission-prompt-tool stdio`，允许工具包括 `Bash(command:*)`、`Read`、`Write`、`Edit`。
- prompt 仍写着 “Agency workspace” 和 “Always use --dir”。

已有可复用基础：

- workspace role：owner/admin/member/guest。
- project/agent RBAC。
- workspace model providers。
- audit log。
- team/project/workflow/task-template/playbook/docs/skills 等管理 API。

## 产品定位

右下角助手不是“项目执行 agent”，而是“Multigent 控制面助手”。

它的职责：

- 帮用户理解 Multigent 当前页面和对象。
- 帮管理员搭建团队、角色、项目、流程、任务模板、协作方案。
- 帮管理员把企业旧流程迁移成 Multigent 可执行流程。
- 帮用户起草任务、文档、prompt、回复、流程节点说明。
- 在需要真实执行开发、跑脚本、访问仓库、调用外部工具时，创建任务并派给具体 agent，由项目 agent 在 sandbox 中执行。

它不应该直接做：

- 跑 shell。
- 读写宿主机文件。
- 修改 git repo。
- 访问 agent 私有 session。
- 使用外部工具凭证。
- 绕过 RBAC 创建/修改资源。

## 权限模型

第一阶段建议先做 admin-only：

- workspace owner/admin 可以打开并使用助手。
- workspace member/guest 默认不显示，或显示“工作区助手未对你开放，请联系管理员”。
- super admin 等同 workspace owner。

原因：

- 初期助手主要用于搭建组织流程，天然涉及高权限操作。
- 先把受控 tool、确认执行、审计链路做稳，再开放普通用户模式。
- 避免普通成员通过自然语言间接创建团队、改角色 prompt、改流程、安装 playbook。

后续再开放普通用户模式：

- 只读查询：查询自己有权限的项目、任务、消息、文档。
- 草稿生成：帮用户起草任务、回复、文档、prompt，但不直接写入全局资产。
- 项目内写操作：只允许用户已有 project manager/operator 权限的范围。

## 模型账号设计

助手必须使用 Multigent 配置的模型账号，而不是宿主机 CLI。

新增 workspace 级配置：

- `assistant_model_provider_id`
- `assistant_enabled`
- `assistant_mode`
  - `admin_only`
  - `workspace_members_readonly`（后续）

设置页面：

- 在 Settings / Model Accounts 附近增加“Workspace Assistant”配置。
- 管理员选择已有 model provider。
- 没有 provider 时引导创建模型账号。

未配置时：

- admin：显示“请先配置模型账号以启用助手”并提供跳转。
- member：显示“工作区助手尚未启用，请联系管理员”或直接隐藏。

模型调用方式：

- 优先实现 server-side provider client，复用 model provider 的 type/base_url/api_key/model。
- 不调用 `claude` / `codex` / `gemini` 本地 CLI。
- 初期可以只支持 OpenAI-compatible chat completion stream；第三方网关大多可覆盖。
- 后续扩展 Claude native、Gemini native。

## 工具调用模型

不要给助手 Bash。给它一组 Multigent 内部工具。

工具必须满足：

- 参数 schema 明确。
- 每次调用都带当前 user、workspace、request ID。
- 后端执行 RBAC。
- 写操作进 audit log。
- 高风险操作需要确认。

第一批工具：

| Tool | 权限 | 风险 | 说明 |
|---|---|---|---|
| `get_workspace_overview` | workspace access | low | 汇总工作区对象数量、最近任务、待配置项 |
| `list_projects` | workspace access | low | 只返回用户可见项目 |
| `list_teams` | workspace access | low | 团队列表 |
| `list_workflows` | workspace access | low | 流程列表 |
| `list_playbooks` | workspace access | low | 可安装协作方案 |
| `query_docs` | docs access | low/medium | 只查用户可见知识库 |
| `draft_team_from_goal` | workspace access | low | 只生成草稿 |
| `draft_workflow_from_process` | workspace access | low | 只生成草稿 |
| `create_team` | workspace admin | write | 创建团队 |
| `create_role` | workspace admin | write | 创建角色和 prompt |
| `create_project` | workspace admin | write | 创建项目 |
| `create_workflow` | workspace admin | write | 创建流程 |
| `create_task_template` | project manager/admin | write | 创建项目级任务模板 |
| `install_playbook` | workspace admin | write | 安装协作方案 |
| `update_agent_prompt` | agent operator/admin | write | 修改 agent prompt |
| `create_task` | project operator/admin | write | 创建任务 |

不做第一批：

- 删除 workspace/project/team/agent。
- 修改用户权限。
- 写入模型账号/API key。
- 写入外部工具凭证。
- 直接执行 agent run。
- 直接发外部消息。

这些动作后续即使支持，也必须单独确认。

## 交互设计

聊天不是“一句话直接改系统”。写操作应走 plan/confirm。

标准流程：

1. 用户提出目标：
   - “帮我搭一个用户运营团队。”
   - “把我们 GitHub issue 处理流程做成流程图。”
2. 助手生成计划：
   - 将创建哪些 team/role/workflow/task template。
   - 每个对象的名称、用途、关键 prompt 或节点。
   - 需要哪些模型账号、外部工具、人工负责人。
3. 前端展示确认卡片。
4. 用户确认执行。
5. 后端逐项调用工具。
6. 返回结果和链接。

确认分级：

- 低风险查询：直接执行。
- 草稿生成：直接生成，不写库。
- 普通写操作：确认后执行。
- 高风险操作：暂不支持或要求专门页面完成。

前端需要从“CLI raw log”改成“assistant events”：

- `assistant_message`
- `tool_plan`
- `tool_call_started`
- `tool_call_result`
- `confirmation_required`
- `final_summary`
- `error`

不要显示本地 CLI 技术日志。

## API 设计

新增或替换：

```text
GET  /api/v1/assistant/status
POST /api/v1/assistant/chat
POST /api/v1/assistant/confirm
```

`GET /assistant/status` 返回：

```json
{
  "enabled": true,
  "mode": "admin_only",
  "configured": true,
  "canUse": true,
  "canAdmin": true,
  "modelProviderId": "prov_xxx"
}
```

`POST /assistant/chat` 请求：

```json
{
  "message": "帮我创建一个用户运营团队",
  "history": [],
  "context": {
    "route": "/teams",
    "selectedProject": "cc-connect"
  }
}
```

SSE event 示例：

```text
event: assistant_message
data: {"content":"我会先给你一个创建计划。"}

event: confirmation_required
data: {"confirmationId":"acf_123","title":"创建用户运营团队","actions":[...]}

event: final_summary
data: {"content":"已创建团队和 3 个角色。"}
```

`POST /assistant/confirm`：

```json
{
  "confirmationId": "acf_123",
  "decision": "approve"
}
```

## 后端改造步骤

### Phase 1：关掉高风险本地执行

- `handleAssistantChat` 增加 `checkCurrentWorkspaceAdmin`。
- 如果未配置 assistant model provider，返回结构化错误。
- 移除或停止使用 `resolveAssistantCLI`。
- 移除 `/assistant/permission` 或保留但不再从前端使用。
- prompt 文案改成 Multigent workspace / SaaS 控制面，不再提 agency root 和 `--dir`。

### Phase 2：接入模型账号

- 增加 assistant settings 存储。
- Settings 页面增加 Workspace Assistant 配置。
- 实现 OpenAI-compatible streaming client。
- 使用 model provider 的 API key/base_url/model。
- 前端未配置时显示引导，不发起 chat。

### Phase 3：实现只读工具

- 建立 `internal/assistant` 包：
  - `Tool` interface。
  - `ToolRegistry`。
  - `AssistantRuntime`。
  - `Planner` / `Executor`。
- 先支持只读工具：workspace overview、projects、teams、workflows、playbooks、docs query。
- 每个工具复用现有 store/controlDB，并执行用户权限过滤。

### Phase 4：实现写操作计划与确认

- LLM 不直接调用写工具，而是先返回 action plan。
- 后端生成 `confirmationId`，保存 pending action plan。
- 前端展示确认卡片。
- 用户确认后，后端按工具执行。
- 写操作进入 audit log。

### Phase 5：开放普通用户只读/草稿模式

- 增加 assistant mode。
- member 可以使用只读/草稿工具。
- project operator/manager 可在自己的项目范围内创建任务和草稿。
- workspace 写操作继续 admin-only。

## 前端改造步骤

### Phase 1

- `AssistantWidget` 启动时调用 `/assistant/status`。
- 未配置 / 无权限时展示轻量提示。
- 删除 permission request UI，或者隐藏到 legacy debug。
- 文案从 “Agency” 改为 “Workspace / Multigent”。

### Phase 2

- 聊天区渲染结构化 assistant events。
- 支持确认卡片：
  - 查看计划。
  - 确认执行。
  - 取消。
- 结果中展示对象链接，例如 team/workflow/project 页面。

### Phase 3

- 结合当前 route 传 page context。
- 在不同页面给快捷建议：
  - Teams：帮我创建一个团队。
  - Workflows：帮我把流程转成节点图。
  - Playbooks：推荐适合我的协作方案。
  - Settings：帮我检查还缺哪些配置。

## 审计与安全

必须记录：

- 用户是谁。
- workspace。
- 原始自然语言请求。
- 生成的 action plan。
- 用户是否确认。
- 实际执行的 tool 和参数摘要。
- 成功/失败结果。

不要记录：

- API key。
- 外部工具 token。
- 模型账号明文。
- 大段私密文档原文。

高风险动作默认不支持：

- 删除 workspace。
- 删除项目/团队/agent。
- 改用户角色。
- 禁用用户。
- 修改模型账号或外部工具凭证。
- 外发消息。
- 触发生产发布。

这些动作只能引导用户去专门页面手动完成。

## Sandbox 结论

控制面助手不进 sandbox。

理由：

- 它不执行代码、不访问 repo、不跑外部 CLI。
- 它只调用后端受控 tools。
- 受控 tools 由 Multigent server 执行 RBAC 和 audit。

需要 sandbox 的工作应该转化为任务：

- 改代码。
- 跑测试。
- clone repo。
- 调用 GitHub/gh/lark-cli 等外部 CLI。
- 分析文件系统。

这类请求由助手创建 task，派给具体项目 agent，在对应 agent sandbox 内执行。

## 验收标准

第一版验收：

- 未配置模型账号时，助手不会尝试找本机 CLI。
- 普通 member 默认不可用。
- workspace owner/admin 可配置助手模型账号。
- owner/admin 可以让助手查询 workspace 概况。
- owner/admin 可以让助手生成创建团队/流程的计划。
- 写操作必须确认后才执行。
- 创建后的对象有可点击链接。
- 所有写操作有 audit log。
- 后端不再暴露 `Bash` / `Read` / `Write` / `Edit` 给助手。

## 建议实施顺序

1. 先 admin-only + status API + 未配置模型账号提示。
2. 接 OpenAI-compatible provider streaming。
3. 做只读工具和 workspace overview。
4. 做 action plan + confirmation。
5. 做 create team / create role / create workflow。
6. 做 playbook install 和 task template。
7. 再评估普通用户只读/草稿模式。
