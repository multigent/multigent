# Multigent Local Worker Architecture

## 核心判断

Multigent 不应该继续把“本地目录”当成产品中心。新的中心应该是云端控制面，负责组织、任务、Context、Agent 配置、审计和计费；Agent 的默认形态应该是云端 Agent 同事。本地机器不是默认执行中心，而是一类接入边界。

这不是抛弃本地能力，而是把本地能力降级为 Runtime Boundary：

- 云端决定：谁要做什么、继承哪些 Context、用哪个 Agent 角色、由谁 review。
- 本地接入：导入已有本地 Agent context，或在私有资源场景下受控执行 Job。
- 人类介入：主要发生在 review、policy、prompt 调教和例外审批，而不是每一步同步驱动 Agent。

## 旧模型的问题

multigent 的本地优先模型适合验证 agent team workflow，但它把几个概念绑在一起：

- workspace = 组织数据存储。
- `.multigent` = 状态、任务、消息、上下文、运行记录。
- scheduler/daemon = 本地自动执行器。
- CLI = 人类操作入口，也是 Agent 操作入口。

这会导致 SaaS 化时边界不清：如果客户继续用自己的 Linear、飞书、GitHub、本地 Codex/Claude Code，我们不能要求他们把公司全部迁进一个本地 workspace。Multigent 应该接管的是 Agent 友好的协作层，而不是替换所有已有系统。

## 新模型

### 1. Control Plane

云端控制面保存标准化对象：

- Organization / Workspace
- Project / Workstream
- Agent Role / Agent Instance
- Context Pack / Context Version
- Job / Run / Review
- Connector / Integration
- Policy / Approval
- Cost / Token / Audit Log

控制面负责做决策和编排：创建 Job、分配 Worker、维护状态机、收集产出、触发 review、沉淀可继承 Context。

### 2. Local Worker Modes

Local Worker 不是一个单一产品形态，而是三种不同模式。它不拥有公司协作真相，只负责把本地资源安全接入云端控制面。

#### import

默认模式。用于把用户现有本地 Agent 的 context、角色经验、技能目录、运行时配置盘点出来，导入 Multigent，创建云端 Agent 同事。

第一版只做安全扫描：

- 扫描已知 context 文件和目录，例如 `AGENTS.md`、`CLAUDE.md`、`.multigent/context/`、旧版本地 context 目录、`.claude/skills/`。
- 默认只读取文件路径、类型和大小，不读取正文。
- 输出 import manifest，后续再由用户选择哪些内容同步到云端。

#### private-resource

用于客户不希望把某些 repo、凭据或内网环境同步到云端的场景。云端仍负责组织、任务、context、review 和审计；Worker 只在本地临时执行需要私有资源的 Job。

职责：

- 连接 Multigent control plane。
- 心跳上报可用状态、容量、运行中的 Job。
- lease 一个 Job。
- 拉取需要的 Context refs 和 repo refs。
- 准备隔离执行目录或 worktree。
- 调用本地 Agent runtime，例如 Codex、Claude Code、Cursor、Gemini、HTTP agent。
- 回传结果、日志、artifact、token usage 和错误。

#### enterprise

用于企业托管环境或客户自有 VPC。语义上仍是云端控制面的一部分，但执行资源在客户受控环境内。

不应该让 Worker 负责：

- 决定需求优先级。
- 决定长期组织记忆是否晋升。
- 保存唯一的任务真相。
- 直接替代客户已有项目管理系统。

### 3. Compatibility Workspace

当前 `.multigent` 本地 workspace 可以作为过渡层保留：

- 开源版和内部 dogfood 仍可本地运行。
- SaaS Worker 可以把 Job materialize 成临时 `.multigent` workspace，以复用现有 runner、taskstore、formatter。
- 长期要把 runner 从 file-store 解耦，让 Worker 不必依赖完整 workspace。

## Job 协议草案

Worker 看到的 Job 应该足够小：

```json
{
  "id": "job_123",
  "tenant_id": "tenant_abc",
  "project_id": "tapnow-agent",
  "workstream": "plugin-connector",
  "agent_name": "backend-connector-dev",
  "runtime": "codex",
  "prompt": "Implement connector OAuth callback handling...",
  "context_refs": [
    "ctx://project/tapnow-agent/base@v12",
    "ctx://workstream/plugin-connector/prd@v4",
    "ctx://api/connectors@v7"
  ]
}
```

Worker 回传 Result：

```json
{
  "job_id": "job_123",
  "status": "success",
  "summary": "Implemented OAuth callback validation and tests.",
  "artifacts": ["git://branch/mg/job_123", "run://logs/job_123"],
  "token_usage": { "input": 12000, "output": 2600 }
}
```

## 迁移路线

### Phase 1: Rename and Product Boundary

已经开始：

- Go module 改为 `github.com/multigent/multigent`。
- CLI binary 改为 `multigent`。
- 本地状态目录改为 `.multigent`。
- 新增 `internal/worker`，定义 `Job`、`JobResult`、`Heartbeat`、`ControlPlane`、`LocalExecutor`。
- 新增 `multigent worker inspect/start/import scan`，先暴露 worker 模式边界。
- 新增 Team 层级字段、Project owner/context 字段和 Workstream 对象。

### Phase 2: Cloud Agent Import

下一步要做：

- 把 `worker import scan` 扩展成 import plan：展示哪些 context 可同步、哪些需要脱敏、哪些需要人工确认。
- 支持从本地 Agent 创建云端 Agent 同事，并绑定 team、role、agent owner、autonomy level。
- 支持 Context Pack 版本化和 Memory Candidate 审核。
- 支持 agent owner 评价和调教云端 Agent 的输出表现。

### Phase 3: Worker Executes Private Jobs

再下一步：

- 抽出现有 runner 为 `LocalExecutor` 实现。
- 支持从 Job 创建临时 execution workspace。
- 支持 repo checkout/worktree 管理。
- 把 task result、log、artifact 标准化。

### Phase 4: Cloud Control Plane Protocol

再往后：

- 实现 HTTP control plane client。
- Worker 注册、心跳、lease、complete、fail、cancel。
- 支持 worker token、租户隔离、最小权限。
- 支持 job cancellation 和 retry。
- 支持 run log streaming。

### Phase 5: SaaS Product Layer

云端产品能力：

- Context Pack 管理和版本化。
- Workstream 状态机。
- Agent Role 配置和评估。
- Review queue。
- Connector sync：飞书、Linear、Jira、GitHub、Slack。
- Cost/token 看板。
- Human Intervention Ledger：统计哪些人工介入可以被 policy、prompt、skill 消除。

## 当前代码边界

`internal/worker` 现在只定义协议和配置，不直接调用现有 `taskstore`。这是刻意的：Worker 应该依赖一个执行接口，而不是继承旧的本地 workspace 数据模型。

短期可以复用旧 runner，长期要让 runner 接收标准 Job，而不是从 `.multigent/tasks.yaml` 读取任务。
