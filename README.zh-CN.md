# Multigent

Multigent 是一个面向 Agent 时代的协作与执行平台。它的目标不是替换 Linear、Jira、ONES、飞书、GitHub，也不是要求团队放弃本地 Codex、Claude Code、Cursor 等工具，而是在这些工具之上补上一层 Agent 友好的协作架构。

它从 `multigent` 代码迁移而来，但产品方向会继续升级：

- 统一管理 Agent 可用的 Context、角色、任务、评审、运行记录和成本。
- 支持团队继续使用原有项目管理、文档、代码仓库和沟通工具。
- 通过 Local Worker 在客户本地机器执行任务，贴近 repo、凭据、sandbox 和本地 Agent runtime。
- 让人从同步阻塞流程的人，变成角色 Agent 的 owner、reviewer 和流程设计者。

## 当前状态

这是 Multigent 从 multigent 启动后的第一版代码基线。

已经完成：

- Go module 改为 `github.com/multigent/multigent`
- CLI binary 改为 `multigent`
- 本地元数据目录改为 `.multigent`
- NPM package 改为 `@multigent/multigent`
- 增加 Local Worker 的第一层边界：`multigent worker inspect`

当前本地 workspace 流程仍然可用，但后续架构会逐步转向：

```text
SaaS Control Plane  <->  Local Worker  <->  Local Agent Runtimes
```

## 构建

```bash
make web
make build-go
./dist/multigent --help
```

## Worker

查看本地 worker 配置：

```bash
./dist/multigent worker inspect
```

预览 worker 启动配置，不连接云端：

```bash
./dist/multigent worker start --dry-run \
  --id local-dev \
  --control-plane-url https://app.multigent.ai \
  --token test-token \
  --workspace /tmp/multigent-worker
```

Worker 协议还没有假装实现，下一步要真正接入 control plane。详见：

- `docs/local-worker-architecture.md`
