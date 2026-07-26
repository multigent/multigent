# 跑通 Hello World 协作接力

这个示例不是业务场景，也不是研发流程。它只用来验证 Multigent 的核心闭环：

```text
Agent 接任务 -> 人类审核 -> Agent 继续接力 -> Agent 记录结果 -> 人类确认
```

如果你刚安装好 Multigent，建议先跑这个示例，而不是一上来搭自己的公司流程。

## 1. 进入 Example Workspace

首次注册后，Multigent 会自动创建一个示例工作区：

```text
Example Workspace
```

如果没有看到，可以在左上角工作区下拉里切换。

示例项目叫：

```text
hello-world-relay
```

## 2. 配置模型账号

进入：

```text
设置 -> 模型账号
```

添加一个可用账号。任选一种即可：

- Claude Code 官方账号或第三方 Anthropic-compatible 网关。
- Codex 官方账号或 OpenAI-compatible 网关。
- Cursor 账号。

保存后回到：

```text
项目 -> hello-world-relay -> Members
```

依次点开三个 Agent：

- `greeter-agent`
- `responder-agent`
- `recorder-agent`

在 Agent 详情页的“模型与凭证”里选择刚刚配置的模型账号。

## 3. 准备 Docker Sandbox

Agent 默认在 Docker sandbox 中运行。先确认 Docker 可用：

```bash
docker info
```

如果 Docker 正常，建议提前准备运行环境：

```bash
multigent sandbox prepare
```

第一次准备会拉取 runtime 镜像并安装 Agent CLI，可能需要几分钟。准备好后再运行 Agent，体验会稳定很多。

## 4. 打开初始任务

进入：

```text
项目 -> hello-world-relay -> Tasks
```

找到任务：

```text
完成一次 Hello World 协作接力
```

点开任务详情，可以看到它绑定了 `Hello World 协作接力` 流程。

## 5. 触发第一个 Agent

有两种方式。

方式一：在任务详情或任务列表里手动运行当前负责人。

方式二：进入：

```text
项目 -> hello-world-relay -> Schedule
```

找到 `greeter-agent`，点击：

```text
手动唤醒
```

`greeter-agent` 会读取当前流程节点，创建第一份接力文档，然后提交结构化输出。

## 6. 人类审核

当任务流转到人工审核节点后，当前负责人会变成你的用户账号。

进入：

```text
工作台 -> Tasks
```

或者回到：

```text
项目 -> hello-world-relay -> Tasks
```

点开任务详情，查看上游输出。如果内容可以接受，点击通过；如果不满意，填写修改意见并打回。

## 7. 继续流转

审核通过后，任务会流转到下一个 Agent：

```text
responder-agent
```

再次手动唤醒当前负责人，或者等待它的任务触发心跳。

后续流程会继续流转到：

```text
recorder-agent -> 最终确认
```

## 8. 看运行结果

可以在几个地方观察：

- `Tasks`：看任务当前节点、负责人、上游输出、结构化输出。
- `Runs`：看每次 Agent 执行记录。
- `Knowledge Base`：看 Agent 创建的 docID 文档。
- `Workflows`：看流程图和节点流转设计。

## 常见卡点

### 任务没有动

通常是没有触发当前负责人。进入 `Schedule` 找到当前负责人，点击“手动唤醒”。

### Agent 运行失败

先检查：

```bash
docker info
multigent sandbox doctor
multigent sandbox prepare
```

如果是模型账号问题，回到 Agent 详情页重新选择模型与凭证。

### 看不到任务

检查当前工作区是不是 `Example Workspace`，当前项目是不是 `hello-world-relay`。

### 人类审核后没有继续

审核只是推进流程状态。下一个 Agent 仍然需要被唤醒，可以手动唤醒，也可以等待任务触发心跳。

