# Product Launch and Growth TODO

> Status: Working TODO  
> Owner: Multigent team  
> Related: `customer-onboarding-playbook-generation-and-agent-improvement.md`, `product-tour-and-example-workspaces.md`, `external-tools-first-20.md`

本文档记录 Multigent 接下来仍未完成、但对产品发布、客户上手和商业化非常关键的事项。它不是长期愿景文档，而是可持续维护的执行清单。

## 目标

Multigent 下一阶段要解决的核心问题：

```text
让新客户能用自己的资料、工具和团队流程，快速生成一套可运行、可调教、可复用的 agent 协作系统。
```

这要求我们不只补功能，还要补上手路径、行业模板、外部工具、宣传素材和可演示资产。

## P0：客户上手与流程梳理

- [ ] 增加 `Process Discovery` 入口，帮助客户上传资料、连接工具、访谈负责人。
- [ ] 支持从客户资料中提取流程、角色、输入输出、人工审核点、重复问题。
- [ ] 生成可审阅的 `Process Discovery Report`。
- [ ] 从报告生成候选 Playbook，包括 team/role、skill、workflow、task template。
- [ ] 支持用户 review 后一键安装生成的 Playbook。
- [ ] 支持把客户 workspace 中跑通的资产保存为自定义 Playbook。
- [ ] 在 Product Tour 中加入“如何从旧流程迁移到 Multigent”的引导。
- [ ] 准备一个非研发行业的 example workspace，避免用户误以为 Multigent 只服务软件开发。

## P0：任务模板与流程闭环

- [ ] 完善项目级任务模板的编辑体验，包括复制、导入 JSON、导出 JSON。
- [ ] 在 `mga` 文档和内置 skill 中明确说明如何用模板 ID 创建任务。
- [ ] 支持任务模板预览：变量替换后会生成什么标题、说明和 prompt。
- [ ] 支持从 workflow 一键生成 task template 草稿。
- [ ] 支持 task template 与 playbook 安装来源关联，方便用户知道模板来自哪里。
- [ ] 在 agent wakeup prompt 示例里加入“发现常见事件后用 task template 创建任务”的标准写法。

## P0：在线 Wakeup Gate

当前从旧系统迁移过来的 `wakeup_condition` 曾经依赖本地 shell 脚本，例如 `$AGENCY_DIR/scripts/wakeup-conditions/...`。这不符合 Multigent 的 SaaS 产品形态：用户不应该感知本地路径，也不应该为了判断是否唤醒 agent 而启动完整 sandbox。

目标：把“是否值得唤醒 agent”的判断收敛为 server-side 在线配置，由 Multigent 调度层通过受控的外部工具连接做轻量判断。

- [ ] 将 Wakeup Gate 定义为在线配置对象，而不是本地脚本路径。
- [ ] 第一批内置 Gate：`has_pending_tasks`、`has_unread_messages`、`github_issue_queue`、`github_pr_queue`、`manual_only`。
- [ ] Gate 配置支持参数：外部工具、连接、仓库、队列视图、最小数量、冷却时间、失败处理策略。
- [ ] `github_issue_queue` 支持类似 PM issue inbox 的判断，例如 `pm`、`new`、`false-complete`。
- [ ] `github_pr_queue` 支持类似 QA PR inbox 的判断，例如 `qa`、`ready`、`ci-failing`。
- [ ] Gate 判断运行在 server/worker 侧，使用 workspace connection grant，不在 agent sandbox 里执行。
- [ ] Gate 执行必须有短超时、只读优先、审计日志和可见的 skip reason。
- [ ] 计划页面 UI 不展示脚本/路径，展示为“当 GitHub Issue 队列有 PM 待处理项时唤醒”。
- [ ] Playbook / task template 可以声明推荐 Gate；安装后生成可审阅的 agent 调度配置。
- [ ] 后续设计 Gate Plugin Registry，让高级团队安装受信任的自定义 Gate package，但不允许普通用户直接填写 shell script。

短期策略：

- [x] 当前迁移的 cc-connect 先移除 `$AGENCY_DIR/scripts/...` 本地脚本条件。
- [ ] cc-connect PM 暂时按 `2h + active_hours + require_any` 唤醒，醒来后由 wakeup prompt 使用 GitHub skill/工具检查 issue 队列。
- [ ] 等 `github_issue_queue` / `github_pr_queue` server-side Gate 完成后，再把 cc-connect PM/QA 恢复为“队列为空则不唤醒”的省资源模式。

## P0：外部工具

- [ ] 扩展第一批外部工具覆盖面，优先支持研发、文档、IM、任务系统和搜索。
- [ ] 完善 GitHub、GitLab、Gitee 的连接和健康检查。
- [ ] 完善 Lark / Feishu、DingTalk、Slack 的连接和机器人协作能力。
- [ ] 增加 Jira、Linear、Plane、Huly、ONES 任务系统接入。
- [ ] 增加 Google Drive、Notion、Confluence 等知识源接入。
- [ ] 增加 Exa、Brave Search 等 web search 工具接入。
- [ ] 每个外部工具提供“如何获取 API Key / OAuth App / App Secret”的多语言指引。
- [ ] 明确每个工具给 agent 的运行时接入方式：平台 CLI、统一 gateway、MCP gateway 或 API adapter。

## P0：IM 协作渠道

- [ ] 将 Lark / Feishu agent channel 打磨成可演示闭环。
- [ ] 支持 Slack agent channel。
- [ ] 支持 DingTalk agent channel。
- [ ] 支持企业微信 / WeCom agent channel。
- [ ] agent 详情页展示渠道连接状态、最后消息时间、最近人工介入记录。
- [ ] 支持从 IM 中对 agent 进行调教、补充上下文、触发任务、查看任务状态。
- [ ] 支持 IM 中的人类 review：通过 / 打回 / 补充意见。

## P0：支持更多 Agent CLI

- [ ] 完善 Codex、Claude Code、Cursor 的模型账号配置和运行时注入。
- [ ] 增加 Gemini CLI 的完整支持。
- [ ] 增加 Qoder CLI 的完整支持。
- [ ] 增加 OpenCode 的完整支持。
- [ ] 增加 iFlow CLI 的完整支持。
- [ ] 支持自定义 Generic CLI 的配置模板。
- [ ] 每个 CLI 都要有：安装器、版本选择、凭证注入、模型选择、session 持久化、token 统计解析。
- [ ] 增加 CLI compatibility matrix，展示每个 CLI 支持的功能能力。

## P1：Playbook 与模板生态

- [ ] 将内置 Playbook 从代码中进一步抽离到 `multigent/playbooks` 远程仓库。
- [ ] 支持从 GitHub / Gitee 镜像拉取 Playbook registry，自动选择可访问源。
- [ ] 增加更多行业 Playbook：销售、客户成功、运营、内容制作、招聘、研究分析。
- [ ] 对每个 Playbook 提供完整预览：角色 prompt、skill 内容、workflow 白板、task template。
- [ ] 支持 Playbook 版本号、安装记录、资产来源标记。
- [ ] 用户修改来自 Playbook 的资产时，标记为 customized，后续不自动更新。
- [ ] 支持把用户 workspace 中的资产打包发布为内部 Playbook。

## P1：Skill Registry

- [ ] 完善跨 workspace 的 skill registry。
- [ ] 支持 skill publish、install、update、version。
- [ ] 支持 skill 附带脚本、参考资料、依赖声明。
- [ ] 支持 agent 在 sandbox 中安装 skill 后回写注册到 Multigent。
- [ ] 明确 skill 文件和脚本如何同步到 agent sandbox。
- [ ] 支持从执行失败、重复人工介入、常见错误中自动建议新 skill。

## P1：运行质量与评估

- [ ] 设计 agent 工作效率指标：任务完成率、返工次数、人工介入次数、平均耗时、token 消耗。
- [ ] 支持 workflow 每个节点的耗时和 token 统计。
- [ ] 支持 task template 维度的成功率统计。
- [ ] 支持 Playbook 维度的 ROI 统计。
- [ ] 增加 agent quality report，展示近期失败原因和改进建议。
- [ ] 支持从 run logs 里提取可复用经验，沉淀为 skill 或 prompt patch。
- [ ] 支持人工 review 质量反馈，让 agent 后续可被调教。

## P1：品牌、官网与传播素材

- [ ] 制作正式 Logo。
- [ ] 制作品牌色、字体、图标风格和基础视觉规范。
- [ ] 准备一句核心宣传语，解释 Multigent 是什么。
- [ ] 准备 3-5 条不同客户视角的价值主张：老板、PM、研发负责人、运营负责人、销售负责人。
- [ ] 准备官网首屏文案。
- [ ] 准备 README 顶部视觉和更强的产品截图。
- [ ] 准备一组漂亮截图：workspace、project、agent detail、workflow board、task detail、playbook detail、external tools。
- [ ] 准备一段 60-90 秒产品宣传片脚本。
- [ ] 准备一段 5 分钟 demo 视频脚本。
- [ ] 准备面向客户 POC 的演示 deck。

## P1：截图与演示环境

- [ ] 准备一套干净、可复现的 demo data seed。
- [ ] 示例 workspace 需要包含：团队、角色、agent、workflow、task template、任务流转记录、知识库文档。
- [ ] 支持一键重置 demo workspace。
- [ ] 为 README、官网、社媒准备固定尺寸截图。
- [ ] 准备暗色模式和亮色模式截图。
- [ ] 准备非研发场景截图，用于展示通用协作价值。

## P2：分发与收费

- [ ] 完善 Homebrew 安装。
- [ ] 完善 npm 包发布。
- [ ] 完善 Docker 镜像发布，包括 runtime base image。
- [ ] 准备 self-hosted 部署文档。
- [ ] 明确开源版与商业版边界。
- [ ] 设计 license / activation / workspace limit / agent limit。
- [ ] 设计团队版或企业版定价假设。
- [ ] 准备 POC 合同和试点交付清单。

## 开放问题

- [ ] Process Discovery 是作为系统 agent、wizard，还是两者结合？
- [ ] 客户资料采集是否需要先做离线导入，避免一开始就做大量 OAuth？
- [ ] Playbook 生成后，哪些资产默认可编辑，哪些应该只读？
- [ ] 外部工具是继续使用“外部工具”概念，还是在用户侧包装成“Agent 能力”？
- [ ] 非研发行业的第一个强 demo 场景选什么？
- [ ] 宣传定位是 “agent workflow platform”、 “agentic company operating system”，还是更具体的“AI team orchestration platform”？

## 最近建议顺序

1. 先补齐 Process Discovery 产品设计和最小实现入口。
2. 把项目级 task template 和 workflow/playbook 打通得更顺。
3. 扩展 Lark / Feishu、GitHub、Slack、DingTalk 这几类最容易演示的工具。
4. 做一套漂亮 demo workspace 和截图。
5. 做 Logo、官网首屏、README 视觉和短 demo 视频脚本。
