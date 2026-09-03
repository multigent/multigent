# Agent 并发工作会话设计

本文记录 Multigent 对 Agent 并发工作的设计判断。核心目标是：让一个长期 Agent Worker 能像人一样同时推进多件事，但不把系统重新做成一堆失控的临时 Agent。

## 背景

真实客户场景里，一个 Agent 经常会同时拿到多个相似任务。例如：

- `plugin-dev` 同时接入 10 个插件。
- `dev` 同时修 5 个独立 bug。
- `qa` 同时 review 多个 PR。
- `pm` 同时分流一批 issue。

如果一个 Agent 只有一个主会话，所有工作都串行推进，会浪费时间，也不符合“Agent 是长期协作对象”的定位。

但如果平台直接创建 `plugin-dev-1`、`plugin-dev-2`、`plugin-dev-3` 这类长期 Agent，又会带来新的混乱：

- 智能体列表膨胀，用户不知道哪个才是正式成员。
- 模型账号、外部工具、IM 渠道、权限、审计和归档都变复杂。
- 临时执行体容易被误认为可以长期沟通和管理的 Agent。
- 主 Agent 的责任边界被打散，没人负责最终收口。

因此，Multigent 不应该把“并发”建模为“自动创建多个长期 Agent”，而应该建模为：一个 Agent Worker 主体下的多个可追踪工作会话。

## 命名

用户侧建议叫 **工作会话**。

内部和 CLI 可以叫 **Fork Session**。如果底层 runtime 使用 `thread` 这个词，例如某些 CLI 把对话称为 thread，Multigent 仍然统一用 session 表达产品语义。

不建议叫：

- `Execution Lane`：太工程化，用户难以理解。
- `Sub Agent`：容易误导成新的长期 Agent。
- `Worker`：会和 Agent Worker 混淆。
- `plugin-dev-1` 这种编号 Agent：会造成身份和权限混乱。

## 术语

### Agent Worker

长期存在的智能体主体，例如 `plugin-dev`、`manager-agent`、`developer-a`。

它拥有：

- 名称、头像、团队、角色。
- 默认模型和运行节点。
- 长期 prompt / context。
- 项目 membership。
- 外部工具连接权限。
- IM 协作渠道。
- heartbeat / wakeup 策略。

### Primary Session

Agent Worker 的主会话。它用于：

- 读取 attention signal。
- 判断优先级。
- 分配或拆解工作。
- 管理并发工作会话。
- 对人类沟通和汇报。
- 收口任务和流程状态。

第一版可以认为每个 Agent Worker 有一个主要心智主线。平台不主动替它压缩，也不把所有并发细节塞回主会话。

### Fork Session

Fork Session 是某个 Agent Worker 派生出来处理具体事项的并发工作会话。

例如：

```text
plugin-dev
  ├─ fork session: Monday 插件接入
  ├─ fork session: Salesforce 插件接入
  ├─ fork session: Dropbox 插件接入
  └─ fork session: Notion bugfix
```

Fork Session 不是新的长期 Agent。它不会出现在智能体列表里，也不拥有独立的团队、角色、协作渠道或长期身份。

它应该拥有：

- session id。
- title / purpose。
- parent agent worker id。
- 关联 project / task / workflow。
- runtime provider。
- runtime session id 或 thread id。
- 状态：pending / running / waiting / blocked / done / failed / stopped。
- 当前产物：PR、文档、测试结果、阻塞信息。
- 审计记录。

### Runtime Session

底层模型 CLI 的原生 session，例如 Claude Code session、Codex chat、Cursor thread。

Fork Session 可以绑定一个 Runtime Session。不同 runtime 的能力不同，所以不能把 Multigent 的产品抽象直接等同于某个 CLI 的 fork 命令。

## Runtime 能力现状

截至 2026-08-22：

| Runtime | fork 能力 | resume 能力 | 对 Multigent 的含义 |
| --- | --- | --- | --- |
| Claude Code | 支持。官方文档描述了 `/fork`、`--fork-session` / `/branch`，fork 会复制当前历史到新 session。Subagent 也支持持久 transcript 和 resume。 | 支持。`--continue` / `--resume` / `/resume`。 | 可以优先用原生 fork 创建 Fork Session 的 runtime session。 |
| Codex | 支持。官方开发者命令文档描述 `/fork` 和 `codex fork`，会把当前 chat 克隆到新 chat ID。 | 支持。Codex CLI 当前有 resume / continue / session picker 能力。 | 可以优先用原生 fork 创建 Fork Session 的 runtime session。 |
| Cursor CLI | 官方文档明确支持 `--resume [thread id]`、`agent resume`、`agent ls` 历史选择。未确认有官方 CLI fork 命令。 | 支持。 | 第一版不能依赖原生 fork；可用 fresh session + 注入主 Agent 摘要、任务上下文和引用材料来降级实现 Fork Session。 |
| HTTP Agent / 自定义 runtime | 取决于实现。 | 取决于实现。 | 通过 runtime adapter capability 声明支持级别。 |

因此，平台必须提供统一的 Fork Session 接口，底层 runtime adapter 决定如何实现：

```text
Claude Code fork session -> claude 原生 fork
Codex fork session       -> codex 原生 fork
Cursor fork session      -> fresh thread + context material
HTTP fork session        -> adapter 自定义
```

## 核心设计判断

### 1. Fork Session 是 Agent 可使用的系统能力

当 Agent Worker wakeup 后发现自己有多件可并发工作时，它可以主动调用系统能力创建工作会话：

```bash
mga session fork --title "接入 Monday 插件" --task <task_id> --project customer-connectors
```

这不是平台替 Agent 自动拆任务，也不是用户手工创建多个 Agent，而是 Agent 根据自己的判断使用工具。

这符合 Multigent 的原则：平台提供能力和环境，Agent 自主决策。

### 2. Fork Session 不独立 heartbeat

第一版中，Fork Session 不应该拥有自己的 heartbeat。

原因：

- 一旦 Fork Session 自己定时 wakeup，它就接近长期 Agent，身份会变混乱。
- 主 Agent 应该保持统筹责任，否则 10 个 Fork Session 会各自失控。
- 权限和审计应该明确落回 parent Agent Worker。

正确模型：

```text
Agent Worker heartbeat
  -> 查看待处理任务和 attention
  -> 查看已有 Fork Sessions 状态
  -> 决定 fork / resume / stop / collect
  -> 必要时唤醒某个 Fork Session 继续工作
```

也就是说，Fork Session 可以被多次 resume，但 resume 由 parent Agent 或平台根据 parent Agent 的策略触发。

后续可以加入 `auto_continue` 策略，但也必须挂在 parent Agent Worker 的调度规则下，而不是让 Fork Session 变成独立智能体。

### 3. 权限继承，但可收窄

Fork Session 默认继承 parent Agent Worker 的权限：

- 模型账号。
- 运行节点。
- 项目访问。
- 知识库和文件访问。
- 外部工具连接。
- skills 和工具。

这点很重要：并发出来的 session 本质上仍然是该 Agent 在干活，所以它默认具备同等能力。

但创建 Fork Session 时应允许进一步收窄权限。例如：

```bash
mga session fork \
  --title "接入 Monday 插件" \
  --task <task_id> \
  --allow-connection github:customer \
  --allow-connection gcloud:test-readonly
```

第一版可以先只做继承，不做细粒度收窄，但数据模型要预留 capability override。

审计上必须记录：

```text
actor: plugin-dev
fork_session: fs_monday_plugin
runtime_session: <runtime_session_id>
delegated_by: plugin-dev primary session
project: customer-connectors
task: <task_id>
```

用户看到的是 `plugin-dev` 做了这件事，而不是一个新的 `plugin-dev-1`。

### 4. IM 协作渠道默认不属于 Fork Session

Fork Session 默认不直接对外拥有 IM 身份。

原因：

- 用户应该和长期 Agent 沟通，而不是和临时工作会话沟通。
- Fork Session 的消息容易造成多个上下文分裂。
- 主 Agent 应该负责汇总和对人沟通。

第一版建议：

- 人类通过 Agent Worker 的主 IM 渠道沟通。
- 主 Agent 可以把某个 Fork Session 的状态整理后发给人。
- Web UI 可以让用户查看 Fork Session 详情和日志。
- 不提供“直接私聊 Fork Session”。

后续如果确实需要，可以支持从 UI 对某个 Fork Session 留 comment，这个 comment 进入 parent Agent 的 attention，由 parent 决定是否 resume 该 Fork Session。

### 5. Fork Session 完成不等于任务完成

Fork Session 只负责产出局部结果。

例如：

- 代码改动。
- PR。
- 测试结果。
- 文档。
- 阻塞原因。

最终是否标记 task done、workflow 是否进入下一节点，原则上由 parent Agent Worker 收口。

这能避免多个 Fork Session 同时改 task 状态导致冲突。

例外：如果 Fork Session 绑定的是一个明确子任务，且系统允许 Fork Session 完成子任务，则 Fork Session 可以完成自己的子任务，但 parent task 的收口仍由 parent Agent 完成。

## 建议的 `mga` 能力

第一版面向 Agent 暴露 session 命令：

```bash
# 创建工作会话
mga session fork \
  --title "接入 Monday 插件" \
  --project customer-connectors \
  --task <task_id> \
  --prompt-file ./fork-session-prompt.md

# 查看当前 Agent 的工作会话
mga session list --kind fork --status active

# 查看某个工作会话状态
mga session status <session_id>

# 继续运行某个工作会话
mga session resume <session_id> \
  --prompt "继续处理上次阻塞点，重点验证 OAuth callback"

# 停止某个工作会话
mga session stop <session_id> \
  --reason "该插件本期不接入"

# 收集结果
mga session collect <session_id>
```

命令输出必须支持 `--json`，方便 Agent 批量管理。

## Runtime Adapter 能力声明

不同 runtime adapter 应声明：

```go
type RuntimeSessionCapabilities struct {
    CanResume bool
    CanForkFromCurrent bool
    CanForkFromSessionID bool
    CanListSessions bool
    CanNameSession bool
}
```

创建 Fork Session 时按能力选择策略：

1. 如果支持 `CanForkFromSessionID`，从 parent primary session fork。
2. 如果只支持 `CanForkFromCurrent`，在当前运行中调用 runtime 的 fork 能力。
3. 如果不支持 fork 但支持 resume，新建 fresh session，并注入 parent summary、task context、project context、引用文档。
4. 如果连 resume 都不支持，则 Fork Session 只能作为一次性 run，不能长期续跑。

这能保证 Multigent 的产品语义稳定，不被底层 runtime 差异绑死。

## 数据模型草案

新增 `agent_sessions`，用 `session_kind` 区分 `primary` 和 `fork`。第一版先落地 `fork`，后续可以把 Agent Worker 的主会话也迁入同一张表：

```text
id
workspace_id
agent_worker_id
session_kind       # primary / fork
parent_session_id
project_id
project_membership_id
task_id
workflow_instance_id
title
purpose
initial_prompt
status
runtime_provider
runtime_session_id
fork_mode          # native_fork / fresh_with_context / adapter_custom
permission_policy  # inherit / narrowed
capabilities_json
result_summary
result_refs_json
created_by_run_id
last_run_id
created_at
updated_at
last_activity_at
completed_at
```

运行记录需要能关联 Fork Session：

```text
runs.fork_session_id nullable
```

审计日志需要记录：

```text
actor_type=agent_worker
actor_id=<parent agent>
fork_session_id=<session>
run_id=<run>
```

## UI 草案

### 智能体详情页

新增“工作会话”区域：

- active fork sessions 数量。
- 每个 Fork Session 的 title、项目、任务、状态、最近更新时间。
- 点击进入 Fork Session 详情。

### 任务 Follow 页面

如果任务下有多个 Fork Session，显示：

```text
当前由 plugin-dev 并行处理 4 个工作会话

✓ Monday 插件接入        PR ready
… Salesforce 插件接入    waiting auth
✕ Dropbox 插件接入       failed
… Notion bugfix          running
```

用户能查看每个 Fork Session 的日志和产物，但默认仍和 parent Agent 沟通。

### 运行页面

运行记录显示：

```text
Agent: plugin-dev
Session: Monday 插件接入
Project: customer-connectors
Run: run_xxx
```

避免显示成 `customer-connectors/plugin-dev-1`。

## MVP 落地顺序

### Phase 1：只做可追踪的 Fork Session，不做自动并发调度

- 新增 Fork Session 数据模型。
- `mga session fork/list/status/resume/stop/collect`。
- Runtime adapter 支持 capability 声明。
- Claude Code / Codex 优先接 native fork。
- Cursor 用 fresh session + context 注入降级。
- runs / audit 关联 Fork Session。
- Web 展示 Fork Session 列表和状态。

Agent 是否 fork、fork 几个、什么时候 resume，由 Agent 自己决定。

### Phase 1.5：最小运行面闭环

只记录 Fork Session 还不够。主 Agent 需要能真的把某个工作会话丢到 runtime 里跑，并能在下一次检查时知道它是否完成。

第一版运行面语义：

- `mga session resume <session_id> --run`：为该 Fork Session 排一个真实 runtime run。
- runtime run 的 `kind = fork_session`。
- runtime run 记录 `fork_session_id`，并注入：
  - `MULTIGENT_FORK_SESSION_ID`
  - `MULTIGENT_RUN_ID`
  - `MULTIGENT_AGENT_WORKER_ID`
  - `MULTIGENT_PROJECT_MEMBERSHIP_ID`
- Fork Session run 不会自动完成父任务；它只更新自己的 session 状态。
- 子 session 完成时可以主动调用：

```bash
mga session collect <session_id> --summary "..."
```

- runtime run finish 时也会兜底同步：
  - `last_run_id`
  - `runtime_session_id`
  - `status = done / failed / stopped`
  - `result_summary`

任务绑定语义：

```bash
mga task add \
  --title "接入 Monday 插件" \
  --prompt "..." \
  --fork-session <session_id>
```

这会把 `MULTIGENT_FORK_SESSION_ID` 写入任务 vars。调度器启动该任务时：

- 对应 runtime run 会带上 `fork_session_id`。
- 运行环境会注入 `MULTIGENT_FORK_SESSION_ID`。
- 该任务完成时既推进 task 状态，也同步 Fork Session 状态。

两种模式的区别：

| 模式 | 用途 | 是否自动完成任务 |
| --- | --- | --- |
| `session resume --run` | 主 Agent 派生一段并发探索/调研/执行 | 否，只更新 Fork Session |
| `task add --fork-session` 后由调度器跑任务 | 任务明确绑定某个工作会话处理 | 是，任务和 Fork Session 都更新 |

主 Agent 的推荐循环：

1. 发现有多个相对独立的工作单元。
2. `mga session fork` 创建工作会话。
3. 对短任务用 `mga session resume --run`。
4. 对需要进入任务队列和流程可观察的工作，用 `mga task add --fork-session`。
5. 定期 `mga session list --status active` 和 `mga session status <id>`。
6. 汇总 done / failed / blocked 的结果，再决定继续推进、打回、等待人类或完成父任务。

### Phase 2：加入并发上限和资源保护

- Agent Worker 增加 `max_concurrent_fork_sessions`。
- Runtime node 增加并发容量。
- Scheduler 避免同一 Agent 无限 fork。
- Fork Session 运行锁，防止同一 Fork Session 被重复 resume。
- UI 显示排队、运行、阻塞。

### Phase 3：更好的收口和批量决策

- parent Agent 可以批量 collect Fork Sessions。
- 支持 Fork Session result schema。
- human review 可以看到多个 Fork Session 的汇总。
- 允许用户批量 approve / request changes。

### Phase 4：可选提升为正式 Agent

如果某个 Fork Session 变成长期职责，用户可以手动“沉淀为智能体”：

```text
Fork Session -> new Agent Worker
```

这必须是显式操作，不能自动发生。

## 不做什么

第一版不做：

- 自动把每个任务变成子 Agent。
- 让 Fork Session 出现在智能体列表。
- Fork Session 独立 heartbeat。
- Fork Session 独立 IM 绑定。
- 平台自动决定任务拆分策略。
- 跨 runtime 做强一致的原生 fork 语义。

## 结论

Multigent 应该把并发能力建在 Agent Worker 的主体性之上：

```text
Agent Worker 是长期协作对象。
Fork Session 是它派生出来的并发工作会话。
Runtime Session 是底层 CLI 的具体实现。
Task / Workflow 是协作规范和可观察里程碑。
```

这样既能支持一个 Agent 同时处理多件事，又不会把系统做成一堆难以管理的临时 Agent。

最重要的是，这个设计保持了 Multigent 的核心判断：Agent 不是被平台调用的工具，而是拥有判断力、责任边界和持续工作的协作主体。

## 参考资料

- Claude Code Docs: Create custom subagents, `/fork` and resumable subagent behavior.
- Claude Code Docs: How Claude Code works, `--resume` and `--fork-session` / `/branch`.
- OpenAI Codex developer commands: `/fork` and `codex fork`.
- Cursor Docs: Cursor CLI history, `--resume [thread id]`, `agent resume`, `agent ls`.
