<p align="center">
  <img src="docs/assets/banner.svg" alt="Multigent" width="100%">
</p>

<div align="center">

# Multigent

**让 Agent 真正参与交付的团队协作基础设施。**

Multigent 帮助团队把 Prompt、工具、流程和人工 Review 组织成一套可持续运行的 Agent Workforce。团队可以继续使用原有项目管理、代码仓库、文档和沟通工具；Multigent 负责提供 Agent 友好的上下文、结构化任务、安全执行和可观察交接。

[English](README.md) · [文档](docs/) · [架构](docs/architecture/agent-runtime-cli-architecture.md) · [流程引擎](docs/concepts/collaboration-workflow-state-machine.md) · [路线图](docs/operations/roadmap.md)

</div>

---

## 为什么需要 Multigent

大多数团队并不缺文档、任务系统、代码仓库、会议记录和本地 Agent 工具。真正的问题是：Agent 很难稳定理解同一份上下文，很难按同一套流程推进，很难知道什么时候该请求人类 Review，也很难在没有人同步驱动的情况下持续把事情做完。

Multigent 解决的是这层 Agent 协作架构问题：

- **统一 Agent 上下文**：workspace、团队、角色、项目、任务、知识库、Skill、工具和流程状态集中管理。
- **面向 Agent 的任务执行**：任务可以绑定流程，携带结构化输入输出，并在人和 Agent 之间流转。
- **人类从阻塞点变成审核者**：人主要负责定义规则、补充专业知识、Review 关键产物，而不是每一步都手动唤醒 Agent。
- **外部工具能力化**：GitHub、飞书/Lark、Slack、项目系统、Web Search、设计工具等都可以作为受控工具提供给 Agent。
- **Agent 工作可观察**：运行记录、会话、流程节点、任务历史、Token、日志和审计事件都能在 Web 控制台查看。
- **Sandbox 优先执行**：Agent 默认在隔离环境中运行，只能拿到明确授权的凭证、工具和上下文。

## 产品模型

```text
Workspace
  -> 团队与角色
  -> 项目
  -> Agent 与人类成员
  -> 任务
  -> 流程
  -> 知识库、Skill、模型账号、外部工具
```

Multigent 不要求公司一上来替换 Jira、Linear、Plane、Huly、GitHub、飞书/Lark、Slack 或内部文档系统。它更像是这些系统之上的 Agent 原生协作控制层。

## 核心能力

### Agent Workforce

创建具备角色、模型账号、CLI Runtime、Sandbox、Skill 和外部工具权限的 Agent 同事。Agent 可以通过 Web Chat、任务、流程节点、手动唤醒和定时调度来工作。

### 流程引擎

在可视化白板上设计 workspace 通用流程。流程可以定义节点、执行者、必填输入、必填输出、人工 Review、打回循环、条件分支和交接规则。创建任务时可以选择流程，并把每个角色绑定到具体的人或 Agent。

### 协作方案

安装成套的协作方案，包含团队、角色、Skill 和流程。协作方案让新 workspace 不必从空白开始，可以先用一套经过整理的流程模板跑起来，再根据真实业务微调。

### 外部工具

在 workspace 级别配置外部工具，并授权给需要的 Agent。Multigent 的工具接入会覆盖多种形态：平台 CLI、MCP Gateway、API Key、OAuth App 和运行时物料注入。

### 知识库

文档以 doc ID 方式被引用和读取。Agent 可以通过 Runtime CLI 创建和读取知识库文档，让流程输出沉淀到长期知识，而不是只留在聊天记录里。

### 调度与运行记录

支持任务触发、心跳调度、Cron、手动唤醒等方式。运行记录会保存状态、Runtime Session ID、可用的 Token 统计、日志和流程节点输出。

### 权限与审计

Workspace 角色、项目成员、任务可见性、用户邀请和审计事件都是一等概念。人类和 Agent 都会作为带身份的主体参与权限判断。

## 快速开始

### 安装

macOS / Linux：

```bash
curl -fsSL https://raw.githubusercontent.com/multigent/multigent/main/scripts/install.sh | bash
multigent version
```

Homebrew：

```bash
brew install multigent/tap/multigent
```

Windows PowerShell：

```powershell
irm https://raw.githubusercontent.com/multigent/multigent/main/scripts/install.ps1 | iex
```

npm wrapper：

```bash
npm install -g @multigent/multigent
```

Docker 自托管：

```bash
docker run --rm -p 27892:27892 \
  -v multigent-data:/data \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/multigent/multigent:latest
```

打开 `http://127.0.0.1:27892`。

### Agent 运行环境要求

- Docker，用于 Sandbox Agent 执行

Multigent 默认的多架构 runtime 镜像发布在 `ghcr.io/multigent/multigent/runtime-base:latest`。镜像内置同版本的 Linux `mga` runtime CLI，不会再把 macOS 或 Windows 原生二进制挂载进 Linux sandbox。已发布的 GHCR 包必须公开，用户无需执行 `docker login`。

### 启动 Web 控制台

生产式启动：一个二进制同时提供 API 和内置前端。

```bash
multigent --dir ./data start --addr 127.0.0.1:27892 --open
```

前端开发模式：API 与 Vite 热更新分开启动。

```bash
make build
./dist/multigent --dir ./data api serve --addr 127.0.0.1:27893
cd web
npm install
npm run dev
```

打开终端中显示的 Vite 地址，通常是 `http://127.0.0.1:27894`。

## 第一次使用路径

1. 注册第一个用户。
2. 创建 workspace。
3. 邀请成员，或先跳过。
4. 创建或安装团队、角色和协作方案。
5. 创建项目。
6. 给项目添加 Agent。
7. 配置模型账号和外部工具。
8. 创建流程，或使用内置流程。
9. 创建任务并绑定流程。
10. 观察任务在人和 Agent 之间流转，并查看每个节点的结构化输出。

## 架构

```text
┌─────────────────────────┐
│      Web Console        │
│  React + workflow UI    │
└───────────┬─────────────┘
            │ HTTP / SSE
┌───────────▼─────────────┐
│      Go API Server      │
│ auth, RBAC, tasks, docs │
│ workflows, tools, runs  │
└───────────┬─────────────┘
            │
┌───────────▼─────────────┐
│      Storage Layer      │
│ SQLite today, interface │
│ ready for other DBs     │
└───────────┬─────────────┘
            │
┌───────────▼─────────────┐
│   Runtime Materializer  │
│ sandbox, CLI, skills,   │
│ credentials, tools      │
└───────────┬─────────────┘
            │
┌───────────▼─────────────┐
│  Isolated Agent Runtime │
│ Codex, Claude Code,     │
│ Cursor, tool CLIs, MCP  │
└─────────────────────────┘
```

更详细的设计文档：

- [Agent Runtime CLI 架构](docs/architecture/agent-runtime-cli-architecture.md)
- [Runtime Toolchain 架构](docs/architecture/runtime-toolchain-architecture.md)
- [Agent 隔离与权限架构](docs/architecture/agent-isolation-and-permission-architecture.md)
- [SQLite 存储架构](docs/architecture/sqlite-storage-architecture.md)
- [外部工具插件协议](docs/concepts/external-tool-plugin-protocol.md)
- [配置与日志](docs/getting-started/configuration-and-logging.md)
- [发布与分发](docs/operations/release-distribution.md)

## 开发

```bash
make test
make web
make build-go
```

常用命令：

```bash
# 只启动 API
./dist/multigent --dir ./data api serve --addr 127.0.0.1:27893

# 启动 API + 内置 Web
./dist/multigent --dir ./data start --addr 127.0.0.1:27892

# 查看 worker/runtime 配置
./dist/multigent worker inspect
```

配置支持 CLI 参数、环境变量和 TOML 配置文件。详见 [配置与日志](docs/getting-started/configuration-and-logging.md)。

## 当前状态

Multigent 还在快速产品开发阶段。仓库中已经包含 Web 控制台、workspace 模型、用户与邀请、团队与角色、Agent、模型账号、外部工具、任务、流程定义、调度器、Sandbox Runtime 抽象、知识库、协作方案和运行遥测。

近期重点：

- 更顺滑的新用户引导和 Example Workspace；
- 更强的 Sandbox 隔离与 Runtime 物料注入；
- 更完整的流程执行和可视化观测；
- 更好用的外部工具适配；
- 面向自托管和商业化部署的产品包装。

## 许可证

Multigent 采用 [PolyForm Noncommercial License 1.0.0](LICENSE) 以 source-available 方式发布。

未经版权持有人另行书面授权，不允许任何商业使用。
