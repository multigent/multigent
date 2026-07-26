# Multigent 和 Multica 有什么不同？

> 这不是竞品拉踩。两个产品都在探索 Agent 进入真实工作流之后，团队应该怎么管理它们。

## 一句话

Multica 更像“托管 Coding Agent 的任务平台”；Multigent 更像“人和智能体协作的控制台”。

## Multica 更强调什么

从公开资料和代码看，Multica 的核心体验更偏：

- 本地 daemon 或云端 runtime。
- 自动检测本机 Claude Code、Codex、Cursor 等 CLI。
- 把 issue 分配给 agent。
- agent 接活、执行、回传进度。
- coding agent 生命周期管理。

这条路对开发者很丝滑，尤其适合“把代码任务派给 agent 做”。

## Multigent 更强调什么

Multigent 不只关心“agent 在哪里执行”，更关心：

- 人和智能体是否在同一个项目里协作。
- 任务能不能绑定流程。
- 流程节点能不能在人和 agent 之间流转。
- 人工审核、打回、继续执行是否可控。
- 上下文、知识库、Skill、外部工具是否能收敛管理。
- 模型账号、工具凭证、项目权限是否可控。
- 每次运行、每次流转、每次输出是否可审计和复盘。

Multigent 的目标不是替代 GitHub、Linear、Jira、Plane、飞书或本地 Agent CLI，而是在这些工具之上补一个 Agent 原生协作控制层。

## 一个具体差异

如果只是：

```text
把一个 coding issue 派给某个 agent 跑
```

Multica 的路径会非常直接。

如果你想表达：

```text
PM agent 先整理需求 -> 人类审核 -> Dev agent 开发 -> QA agent 测试 -> 人类最终确认 -> 记录沉淀
```

Multigent 更强调这个流程本身，包括每个节点的输入、输出、负责人、审核和打回。

## 是否会支持远端个人电脑 Agent？

会考虑，但 Multigent 不会回到“本地项目管理器”的定位。

更合适的设计是：

```text
Multigent 控制面 + 可接入的 Runtime Node
```

也就是用户可以把自己的电脑或私有服务器作为“运行节点”接入，但任务、流程、权限、上下文和审计仍然由 Multigent 管理。

## 怎么选？

如果你主要想快速托管 coding agent，可以试 Multica。

如果你想让 agent 进入团队协作流程，和人一起做任务、审核、打回、交接、沉淀知识，Multigent 更符合这个方向。

最好的方式是都试一下。Multigent 也在快速迭代，很多能力还在内测阶段。

