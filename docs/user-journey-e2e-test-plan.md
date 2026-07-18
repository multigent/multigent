# Multigent End-to-End User Journey Test Plan

本文档用于陪同完整验收 Multigent 的核心用户旅程：从第一个用户注册、创建 workspace，到创建团队、项目、Agent、外部工具、流程和任务，再到跑完一条研发需求流程。

目标不是证明每个按钮都能点，而是验证：

```text
一个新客户是否能把自己的研发流程搬到 Multigent，并让 agent 真的参与交付。
```

## 测试原则

- 使用全新的测试数据目录，避免污染现有演示数据。
- 使用脱敏账号、项目名和需求名。
- 每一步都记录：是否顺畅、是否符合用户直觉、是否有权限越界、是否有文案或多语言问题。
- 遇到问题先记录，不中断主流程；只有阻塞后续测试时才现场修复。
- 不使用真实生产密钥。外部工具优先用测试 token、临时应用或跳过真实调用，只验证配置链路和权限链路。

## 测试环境

建议使用一个新的数据目录：

```bash
export MULTIGENT_E2E_ROOT=/root/code/spaceship/multigent_e2e
rm -rf "$MULTIGENT_E2E_ROOT"
mkdir -p "$MULTIGENT_E2E_ROOT"
```

后端 API：

```bash
cd /root/code/spaceship/multigent
make build-go
./dist/multigent --dir "$MULTIGENT_E2E_ROOT" api serve --addr 127.0.0.1:27893
```

前端：

```bash
cd /root/code/spaceship/multigent/web
npm run dev -- --host 127.0.0.1 --port 27894
```

浏览器入口：

```text
http://127.0.0.1:27894
```

服务日志建议同时开两个终端观察：

- API 服务 stdout / stderr。
- 浏览器 DevTools Console + Network。

如果要测试真实 Agent 运行，还需要：

- Docker 可用：`docker info`。
- runtime base image 可用或本地已构建。
- 至少一个模型账号可用，例如 Codex / Claude Code / Cursor 对应 API Key。

## 测试角色

| 角色 | 测试账号 | 用途 |
|---|---|---|
| Workspace Admin | `admin@example.test` | 注册、创建 workspace、配置全局资源、邀请成员、创建流程和项目 |
| PM User | `pm@example.test` | 作为产品负责人参与需求确认和 review |
| Dev User | `dev@example.test` | 作为开发负责人管理开发 Agent |
| QA User | `qa@example.test` | 作为 QA 负责人参与测试节点 |

本地未配置 SMTP 时，邀请流程应提供可复制的邀请链接；配置 SMTP 后，应发出邮件邀请。

## 贯穿样例需求

测试使用一条小型研发需求：

```text
需求：给示例产品增加 Web Search 外部工具能力。
目标：Agent 可以在任务执行时使用已配置的搜索工具完成资料检索，并在运行记录中保留工具使用线索。
验收：
1. PM Agent 能输出一版产品 spec。
2. Dev Agent 能输出技术方案并执行一个最小实现任务。
3. QA Agent 能输出测试用例和测试报告。
4. Human review 节点可以通过或退回。
5. 流程图能展示当前节点、分支和输出。
```

这条需求不依赖真实业务数据，适合验收流程、权限、工具、Agent 运行和可观察性。

## 阶段 0：测试前准备

### 0.1 代码和构建

- [ ] `git status --short` 无非预期改动。
- [ ] `make build-go` 成功。
- [ ] `cd web && npm run build` 成功。
- [ ] 使用全新的 `$MULTIGENT_E2E_ROOT` 启动 API。
- [ ] Vite 前端能访问 API，登录页无 API 连接错误。

通过标准：

- API 健康可用。
- 前端无白屏。
- Console 无持续刷屏错误。

### 0.2 记录模板

测试时每发现一个问题，按以下格式记录：

```markdown
## ISSUE-001

- 阶段：
- 操作：
- 期望：
- 实际：
- 严重程度：blocker / high / medium / low
- 截图/日志：
- 是否阻塞主流程：
- 初步判断：
```

## 阶段 1：注册、登录与首个 Workspace

### 1.1 注册 Admin

- [ ] 打开登录页。
- [ ] 使用 `admin@example.test` 注册。
- [ ] 设置密码。
- [ ] 登录成功。

通过标准：

- 未加入任何 workspace 时，进入创建 workspace 引导。
- 页面不暴露内部目录、workspace root 或本地文件路径。
- 中英文切换后，注册/登录/错误提示文案完整。

### 1.2 创建 Workspace

- [ ] 创建 workspace：`E2E Product Lab`。
- [ ] 创建者自动成为 workspace admin。
- [ ] 左上角 workspace 下拉展示当前 workspace 名称。
- [ ] 可以进入 workspace 详情。
- [ ] 本地目录下出现 UUID 格式 workspace 目录，而不是 workspace 名称目录。

通过标准：

- UI 不让用户感知底层文件路径。
- workspace 切换 loading 顺滑。
- 审计日志记录 workspace 创建动作。

## 阶段 2：邀请用户与权限边界

### 2.1 邀请成员

- [ ] Admin 进入“用户”页面。
- [ ] 输入 `pm@example.test`，发送邀请。
- [ ] 输入 `dev@example.test`，发送邀请。
- [ ] 输入 `qa@example.test`，发送邀请。
- [ ] 如果 SMTP 未配置，复制邀请链接完成注册。

通过标准：

- 未注册用户不显示“没有找到用户”这类错误。
- 邀请列表使用表格样式，pending / expires 等文案多语言完整。
- 已注册用户和未注册用户的邀请体验都合理。

### 2.2 普通成员权限

- [ ] 用 PM 用户登录。
- [ ] PM 能看到自己加入的 workspace。
- [ ] PM 不能进入只有 admin 才能使用的系统设置项。
- [ ] PM 不能配置 workspace 级 OAuth Client。
- [ ] PM 不能看到未授权项目。

通过标准：

- 前端入口和后端 API 都有权限限制。
- Network 中直接请求无权限接口时返回结构化错误码。

## 阶段 3：创建团队与角色

### 3.1 使用模板创建团队

- [ ] Admin 进入“团队”页面。
- [ ] 点击新建团队。
- [ ] 从模板下拉中选择“软件研发团队”或类似模板。
- [ ] 修改团队名称为 `Engineering Delivery`。
- [ ] 创建成功。

通过标准：

- 团队模板不是团队实例，一份模板可重复创建多个团队。
- 创建弹框支持不使用模板。
- 模板文案在当前语言下展示。
- 团队列表卡片排版简洁，不出现不必要的元信息。

### 3.2 检查角色

- [ ] 团队详情里有 PM / Developer / QA / Designer 等角色。
- [ ] 角色 prompt 可查看和编辑。
- [ ] 可以删除不需要的角色，并有自定义确认弹框。

通过标准：

- 默认角色 prompt 不只是空话，应包含可执行的行为约束。
- 删除动作有二次确认。
- 审计日志记录创建、编辑、删除。

## 阶段 4：配置模型账号

### 4.1 创建模型账号

- [ ] 进入设置里的模型账号。
- [ ] 选择 CLI 类型，例如 Codex。
- [ ] 选择官方 API Key 或第三方网关。
- [ ] 填入测试 API Key。
- [ ] 保存成功。

通过标准：

- 官方 API Key 表单不要求用户填写 Base URL 和模型列表。
- 第三方网关表单才展示 Base URL、模型等字段。
- 没有把环境变量概念暴露给普通用户。
- 凭证保存后不可直接明文查看。

### 4.2 Agent 页面跳转

- [ ] 在 Agent 详情页点击“去添加模型账号”。
- [ ] 跳转到模型账号页面，而不是外部工具页面。

通过标准：

- 跳转目标正确。
- 返回 Agent 页面后状态不丢失。

## 阶段 5：外部工具配置

### 5.1 配置搜索工具

- [ ] 进入“外部工具”页面。
- [ ] 找到 Exa 或 Brave Search。
- [ ] 点击配置。
- [ ] 弹框里的标题、描述、字段、指引全部按当前语言展示。
- [ ] 填入测试 API Key 或跳过真实 key，记录缺失时的错误提示。
- [ ] 保存后显示已连接。

通过标准：

- 工具卡片不直接展示连接列表。
- 已连接后进入管理弹框，不允许直接编辑密钥；要先断连再重配。
- 连接测试结果文案清晰。

### 5.2 配置研发工具

可选测试 GitHub 或 Feishu / Lark：

- [ ] GitHub 支持 PAT 或授权方式。
- [ ] Feishu 和 Lark 作为两个工具展示。
- [ ] App ID / App Secret 表单清晰。
- [ ] 凭证获取指引能打开。

通过标准：

- 工具的连接方式不会让用户理解底层 adapter。
- 不出现 custom http api / custom mcp server 这类第一版不面向普通用户的入口。

## 阶段 6：创建项目与成员

### 6.1 创建项目

- [ ] 进入“项目”页面。
- [ ] 点击新建项目。
- [ ] 创建项目：`Search Tool Integration`。
- [ ] 不要求用户填写 repo 或本地路径。
- [ ] 创建后进入项目首页。

通过标准：

- 项目是 SaaS 产品语义，不暴露本地文件夹。
- 项目列表中间没有重复的新建按钮。

### 6.2 添加项目成员

- [ ] 进入项目成员页。
- [ ] 点击“添加成员”。
- [ ] 选择 Human，添加 PM / Dev / QA 用户。
- [ ] 选择 Agent，添加 PM Agent / Dev Agent / QA Agent。

通过标准：

- Human 和 Agent 是顶部独立选择，不混在模型下拉里。
- Human 不需要选择团队。
- Agent 需要选择团队和角色。
- 选择 Agent 时负责人下拉只展示人类用户；选择 Human 时不展示 Agent 相关字段。

## 阶段 7：配置 Agent

### 7.1 PM Agent

- [ ] 进入 PM Agent 详情页。
- [ ] 配置 CLI 类型、模型和凭证。
- [ ] 检查 Prompt / Context。
- [ ] 检查默认 Skill 是否包含 Multigent 内置使用说明。
- [ ] 开启需要的外部工具。
- [ ] 进入 Web 会话，发送一句测试消息。

通过标准：

- Agent 页面核心区域是：模型与凭证、会话与协作渠道、Prompt / Context、能力配置。
- 不展示本地运行路径。
- 不需要用户手动点击“同步上下文”。
- Chat 回复以气泡展示，不只是 raw log。
- 运行记录能统计 token。

### 7.2 Dev / QA Agent

- [ ] 重复配置 Dev Agent 和 QA Agent。
- [ ] 确认每个 Agent 的 session / config 目录相互隔离。
- [ ] 绑定外部工具时，Agent 只看到被启用的工具。

通过标准：

- 不挂载宿主机全局 `~/.codex`、`~/.claude`、`~/.config/gh`。
- 工具凭证通过 Multigent materializer 注入到 agent scoped runtime。
- Agent 不能读取其他 Agent 的 runtime session。

## 阶段 8：创建协作流程

### 8.1 从模板创建流程

- [ ] 进入左侧“流程”入口。
- [ ] 列表页展示流程卡片。
- [ ] 点击新建流程。
- [ ] 选择软件研发流程模板。
- [ ] 创建后进入流程白板。

通过标准：

- 流程是 workspace 级，不属于某个项目。
- 模板按当前语言生成；生成后就是普通流程，不随语言切换改变。
- 创建后不会报“找不到流程”。

### 8.2 编辑流程

- [ ] 放大、缩小、拖动画布。
- [ ] 拖动节点，有吸附辅助但不强行错位。
- [ ] 点击节点后有选中态。
- [ ] 编辑节点名称、说明、输入说明、输出说明。
- [ ] 编辑分支条件，例如 Review 通过 / 退回修改。
- [ ] 新增节点、删除节点。
- [ ] 保存。
- [ ] 修改后不保存直接返回，出现未保存确认。
- [ ] Ctrl+Z / Cmd+Z 可撤销误操作。

通过标准：

- 连线不穿过节点主体到不可读。
- 全屏模式包含右侧详情面板。
- 深色模式下 controls 和 minimap 可见。
- 流程描述不出现奇怪滚动条。

## 阶段 9：创建任务并绑定流程

### 9.1 创建父任务

- [ ] 进入项目任务页。
- [ ] 新建任务：`Add Web Search external tool support`。
- [ ] 选择阶段 8 创建的流程。
- [ ] 为流程节点指定执行者：
  - Product Spec：PM Agent。
  - Product Review：PM User。
  - Technical Plan：Dev Agent。
  - Technical Review：Dev User。
  - Implementation：Dev Agent。
  - QA Plan：QA Agent。
  - QA Review：QA User。
  - Release Review：Admin。

通过标准：

- 选择 Agent 时只显示 Agent。
- 选择 Human 时只显示人类用户。
- 未指定必需执行者时不能创建或有明确提示。
- 创建后任务详情能看到 workflow run。

### 9.2 流程上下文注入

- [ ] 唤醒 PM Agent 执行第一个节点。
- [ ] Agent 能知道当前任务、当前流程节点、输入要求、输出要求、可用工具和下一步流转方式。
- [ ] Agent 不需要一次性塞入所有历史上下文，但知道如何查询任务和流程信息。

通过标准：

- Prompt 中包含当前 workflow run / step 的必要信息。
- Agent 能调用 `mga` 或 runtime API 查询任务和流程。
- Agent 完成节点后能提交结构化输出。

## 阶段 10：跑完整研发流程

### 10.1 产品阶段

- [ ] PM Agent 输出产品 spec 初稿。
- [ ] PM User review。
- [ ] 选择退回一次，填写 review 意见。
- [ ] PM Agent 根据意见更新 spec。
- [ ] PM User 通过 review。

通过标准：

- 退回分支能回到指定节点。
- review 意见作为输入传回 Agent。
- 通过后产生最终版产品 spec。
- 节点耗时、人工介入次数被记录。

### 10.2 技术方案阶段

- [ ] Dev Agent 输出技术方案 spec。
- [ ] Dev User review。
- [ ] 通过后进入实现节点。

通过标准：

- 技术方案能引用产品 spec。
- 分支和节点状态在白板里可见。

### 10.3 实现阶段

- [ ] Dev Agent 执行一个最小实现任务。
- [ ] 生成运行记录。
- [ ] 如需要 PR，则输出 PR 链接；如果本地不接真实仓库，则输出实现计划和模拟变更说明。

通过标准：

- Agent 使用配置好的模型和隔离 runtime。
- 外部工具启用时，runtime guide / tool plan 可见。
- token 统计不为 0。
- run summary 能看到成功/失败、日志路径和 session 信息。

### 10.4 QA 阶段

- [ ] QA Agent 根据 spec 输出测试用例。
- [ ] QA User review 测试用例。
- [ ] QA Agent 执行可自动化部分或生成测试报告。
- [ ] QA User 给出通过或退回。

通过标准：

- 测试报告作为节点输出保存。
- 失败项能创建后续 bug 子任务。
- 父任务能展示整体进度。

### 10.5 发布与复盘

- [ ] Release Review 节点由 Admin 处理。
- [ ] 通过后流程进入完成。
- [ ] 生成复盘记录：
  - 总耗时。
  - 每阶段耗时。
  - Agent 运行次数。
  - 人工介入次数。
  - token 消耗。
  - 失败/退回次数。
  - 可沉淀为 prompt / skill / 模板的经验。

通过标准：

- 父任务状态和 workflow run 状态一致。
- 审计日志能回放关键动作。
- 用户能明确知道下一次如何复用这条流程。

## 阶段 11：外部协作渠道

### 11.1 Web Chat

- [ ] 从 Agent 详情页进入 Web Chat。
- [ ] 与当前 Agent 对话。
- [ ] 同一个 session 继续对话时保留上下文。
- [ ] Agent 正在执行任务时，交互 lease / 占用关系清晰。

通过标准：

- 用户知道当前是在调教 Agent，还是在让 Agent 接任务。
- 不会并发写坏同一个 runtime session。

### 11.2 Feishu / Lark 渠道

如果可用，测试：

- [ ] 在 Agent 详情页连接 Feishu 或 Lark。
- [ ] 扫码或授权完成。
- [ ] Web 上显示渠道已连接。
- [ ] 在对应机器人聊天里给 Agent 发消息。
- [ ] Web 上能看到会话状态或事件记录。

通过标准：

- 关闭二维码弹框后停止 poll。
- Agent 渠道连接状态准确。
- 飞书 / Lark 交互等价于 Web Chat 的一次会话调用。

## 阶段 12：多语言、权限和删除

### 12.1 多语言

- [ ] 切换中文。
- [ ] 切换英文。
- [ ] 重点检查：
  - 外部工具弹框。
  - 流程节点详情。
  - 用户邀请列表。
  - Loading 文案。
  - 删除确认弹框。
  - 时间格式。

通过标准：

- 没有明显裸英文 key，例如 `common.close`。
- 中文时间格式符合中文用户习惯。

### 12.2 删除和禁用

- [ ] 删除测试 Skill。
- [ ] 删除测试项目。
- [ ] 删除测试团队。
- [ ] 删除测试流程。
- [ ] 断开外部工具连接。
- [ ] 禁用或移除用户。
- [ ] 删除 workspace。

通过标准：

- 所有危险操作都有自定义确认弹框。
- 删除后列表和详情页状态正确。
- 无权限用户不能删除。
- 审计日志完整。

## 最终验收标准

一次完整测试通过需要满足：

- 注册到 workspace 创建路径顺畅。
- 用户邀请和权限边界没有明显漏洞。
- 团队、角色、项目、Agent 创建路径顺畅。
- 模型账号、外部工具、Agent 绑定链路跑通。
- 流程模板创建、编辑、保存、撤销、未保存提醒可用。
- 任务能绑定流程，并为流程节点指定人或 Agent。
- Agent 执行时知道当前任务和流程节点。
- 至少一条样例需求能从产品 spec 走到 QA / 发布 / 复盘。
- run、token、审计、人工介入记录可观察。
- UI 没有明显英文残留、路径泄露、布局错乱或权限入口误导。

## 我们测试时的分工

### Owner 操作

- 在浏览器里走主流程。
- 输入测试账号、测试凭证或授权。
- 判断页面是否符合真实用户直觉。
- 对产品交互提出即时反馈。

### Codex 陪测

- 启动 / 重启 API 和前端。
- 监控服务日志和浏览器报错。
- 查询数据库或本地目录确认隔离结构。
- 记录问题并判断严重程度。
- 对阻塞问题现场修复并提交。
- 每完成一个大阶段更新测试记录。

## 本轮建议的最小必测路径

如果时间有限，先走这条主线：

```text
注册 Admin
-> 创建 workspace
-> 邀请 1 个成员
-> 创建研发团队模板
-> 创建模型账号
-> 配置 1 个外部工具
-> 创建项目
-> 添加 PM/Dev/QA Agent
-> 创建研发流程模板
-> 创建任务并绑定流程
-> 指定节点执行者
-> PM Agent 跑产品 spec
-> Human review 退回一次
-> PM Agent 修正并通过
-> Dev Agent 跑技术方案或实现
-> QA Agent 跑测试用例
-> 查看 run / token / audit / workflow 状态
```

这条路径能最快暴露 Multigent 是否真正从“对象管理工具”走向“Agent 协作流程平台”。
