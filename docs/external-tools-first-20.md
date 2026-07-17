# External Tools First 20

本文档基于 `/root/code/spaceship/3rd/open-connector` 的 provider catalog，并结合 Multigent 的产品定位，确定第一批优先支持的 20 个外部工具。

选择标准不是“open-connector 里有哪些就全接”，而是优先覆盖 agentic company 最常见的协作链路：

- 研发任务和代码协作。
- 项目管理和 issue 流转。
- 公司知识库和文档。
- 团队沟通和异步通知。
- 设计交付。
- 监控、发布和反馈闭环。

## 参考结论

open-connector 的 provider 设计对我们有三点参考价值：

1. Provider 元数据应该包含 `displayName`、`categories`、`authTypes`、`auth`、`homepageUrl`、`actions`。
2. 多数企业工具至少有一种非 OAuth 接入方式，例如 PAT、API key、app credential 或 webhook token。
3. Google、Slack、Jira、Gmail 等工具更偏 OAuth；如果管理员没有配置 OAuth Client，普通用户不应该看到不可用入口。

## 第一批 20 个外部工具

| Priority | Tool | Category | Primary auth | OAuth when configured | Why first |
|---:|---|---|---|---|---|
| 1 | GitHub | Developer Tools | PAT | Yes | 代码、issue、PR、release 是研发 agent 的核心入口。 |
| 2 | GitLab | Developer Tools | PAT | Later | 很多企业使用 GitLab 或自建 GitLab。 |
| 3 | Gitee | Developer Tools | PAT | Yes | 国内研发团队常见代码托管平台。 |
| 4 | Linear | Project Management | API key | Yes | 现代研发团队常见任务系统，适合 issue/task agent。 |
| 5 | Jira | Project Management | OAuth | Yes | 企业研发管理常见系统，偏 OAuth。 |
| 6 | Notion | Docs / Knowledge | Internal integration token | Yes | 轻量知识库和需求文档常见来源。 |
| 7 | Feishu / Lark | Communication / Docs | App ID + App Secret / quick authorization | Later | 国内/全球团队常用 IM 和云文档入口。产品上按 Feishu、Lark 两个地区工具展示。 |
| 8 | Slack | Communication | OAuth | Yes | 海外团队 IM 入口，适合 agent 通知和交互。 |
| 9 | DingTalk Bot | Communication | Webhook token / signing secret | No | 国内企业通知和机器人消息入口。 |
| 10 | Gmail | Email | OAuth | Yes | 邮件读取、整理、回复草稿等工作流。 |
| 11 | Google Drive | Docs / Storage | OAuth | Yes | 文件、资料、附件和知识库入口。 |
| 12 | Google Docs | Docs | OAuth | Yes | 文档读取、创建、更新和总结。 |
| 13 | Google Sheets | Data / Docs | OAuth | Yes | 表格、运营数据、轻量项目表。 |
| 14 | Google Calendar | Calendar | OAuth | Yes | 日程、会议、提醒和 availability。 |
| 15 | Figma | Design | PAT | Yes | 产品设计、UI 交付、设计状态读取。 |
| 16 | Airtable | Data / Ops | PAT | No | 运营表、CRM 表、内部流程表常见。 |
| 17 | Asana | Project Management | PAT | Later | 非研发项目管理常见工具。 |
| 18 | ClickUp | Project Management | API token | Yes | 通用项目管理，支持 token 和 OAuth。 |
| 19 | Sentry | Monitoring | Auth token | Yes | bug、error、release health，可形成自动 triage。 |
| 20 | Vercel | Deployment | Access token | No | 前端部署、项目状态、发布检查常见。 |

## 分组展示建议

前端不要平铺 20 个毫无层次的卡片。建议按场景分组：

### Developer Tools

- GitHub
- GitLab
- Gitee
- Sentry
- Vercel

### Project Management

- Linear
- Jira
- Asana
- ClickUp

### Knowledge And Docs

- Notion
- Google Drive
- Google Docs
- Google Sheets

### Communication

- Feishu
- Lark
- Slack
- DingTalk Bot
- Gmail
- Google Calendar

### Design And Data

- Figma
- Airtable

## Google Workspace 的产品处理

open-connector 把 Google 工具拆成多个 provider：

- `gmail`
- `googledrive`
- `googledocs`
- `googlesheets`
- `googlecalendar`
- 以及 `googleforms`、`googleslides`、`googletasks` 等。

Multigent 产品上建议先展示为一个分组：

```text
Google Workspace
  Gmail
  Drive
  Docs
  Sheets
  Calendar
```

内部仍然可以按 provider 分开 action 和 scope，因为每个 Google API 的 scope 不同。管理员只需要理解“配置 Google OAuth App”，普通用户只看到可授权的 Google 工具模块。

## Auth 策略

### 默认展示规则

每个工具的接入方式按以下规则展示：

```text
如果工具支持 PAT / API key / app credential：
  展示 Token / API Key / App Credential

如果工具支持 OAuth 且 workspace 已配置 OAuth Client：
  展示 OAuth 授权

如果工具支持 OAuth 但 workspace 未配置 OAuth Client：
  普通用户隐藏 OAuth
  管理员显示“配置 OAuth 应用后可启用”
```

### 第一版优先静态凭证的工具

- GitHub
- GitLab
- Gitee
- Linear
- Notion
- Feishu
- Lark
- DingTalk Bot
- Figma
- Airtable
- Asana
- ClickUp
- Sentry
- Vercel

### 第一版必须准备 OAuth 的工具

- Jira
- Slack
- Gmail
- Google Drive
- Google Docs
- Google Sheets
- Google Calendar

这些工具如果管理员未配置 OAuth Client，就先不向普通用户展示连接入口，或者展示为“需要管理员启用”。

## 实施优先级

### Wave 1：已有基础上产品化

目标：最快让外部工具页面可用。

- GitHub
- Linear
- Feishu / Lark
- DingTalk Bot

说明：这些已经在当前 Multigent 中有 provider/action/proxy 基础，适合先把外部工具页面、凭证配置、连接测试和授权流程跑通。Custom HTTP / Custom MCP 不进入第一版外部工具产品目录，避免把高级开发者能力混入普通用户主路径。

### Wave 2：补齐研发与文档主链路

目标：支持软件研发 POC。

- GitLab
- Gitee
- Notion
- Figma
- Sentry
- Vercel

当前状态：这些已补到 token-first 最小可用，不再只是 Coming soon。后续还需要补更完整的 action 集、provider-specific health check 文案，以及写入型 action 的风险策略。

### Wave 3：补齐项目管理和 Google Workspace

目标：覆盖更多客户现有协作系统。

- Jira
- Slack
- Gmail
- Google Drive
- Google Docs
- Google Sheets
- Google Calendar
- Asana
- ClickUp
- Airtable

当前状态：Asana、ClickUp、Airtable 已补到 token-first 最小可用。Google Workspace、Jira、Slack 仍主要依赖 OAuth client 配置，暂不伪装成完整可用。

## 暂缓但应保留候选

这些工具很有价值，但不建议进入第一批 20 个核心工具。

| Tool | Reason |
|---|---|
| Trello | 与 Asana / ClickUp / Jira / Linear 重叠，优先级稍低。 |
| HubSpot | CRM/销售场景重要，但不是研发交付 POC 第一优先。 |
| Intercom | 客服场景重要，适合后续 support playbook。 |
| Stripe | 商业/支付敏感度高，权限和审计要求更高。 |
| Supabase | 对部分开发团队有价值，但不如 GitHub/GitLab/Sentry/Vercel 普遍。 |
| Cloudflare DNS / R2 | DevOps 有价值，但高风险操作多，需更成熟的 approval policy。 |
| Google Slides / Forms / Tasks | 可作为 Google Workspace 后续模块。 |
| Discord Bot | 更偏社区场景，企业协作优先级低于 Slack/Feishu/DingTalk。 |

## 对当前改造计划的影响

`docs/external-tools-connection-redesign-plan.md` 中的外部工具页面应按这个 first 20 设计：

1. Provider catalog 增加 `category`、`description`、`credentialGuide`、`recommendedScopes`。
2. 前端工具目录按 category 分组。
3. Google Workspace 支持产品分组，内部 provider 分开。
4. OAuth 未配置时普通用户隐藏 OAuth-only 工具的连接入口。
5. 产品层第一版每个外部工具只展示一个主连接；底层可以保留多 connection 扩展能力，但不暴露连接池管理。
6. Wave 1 先复用当前已有 provider，不要一次性补完 20 个 runtime executor。

第一版可验收目标：

```text
用户进入“外部工具”页面，可以清楚看到 20 个目标工具。
已有实现的工具可以配置、测试、授权，并能通过 runtime action proxy 被 agent 使用。
尚未实现 runtime action 的 OAuth-first 工具显示为 Coming soon 或 Admin setup required。
普通用户不会看到不可用的 OAuth 入口。
每个工具在产品层只展示一个主连接，避免第一版引入多连接选择和优先级复杂度。
```

## 当前实现矩阵

当前代码里 Feishu 与 Lark 已拆成两个 provider，所以是 **20 个逻辑工具 / 21 个 provider 条目**。

### Token-first 已最小可用

| Provider | Auth | Current runtime baseline |
|---|---|---|
| GitHub | Device flow + PAT + OAuth when configured | user、issues |
| GitLab | PAT | user、project issues |
| Gitee | PAT + OAuth metadata | user、repository issues |
| Feishu | quick authorization + app credential | wiki spaces、IM message |
| Lark | quick authorization + app credential | wiki spaces、IM message |
| Linear | API key | viewer assigned issues |
| Notion | Internal integration token | bot user、search |
| DingTalk Bot | Webhook token / signing secret | send text message |
| Figma | PAT | user、file |
| Airtable | PAT | whoami、bases |
| Asana | PAT | user、tasks |
| ClickUp | API token | user、teams |
| Sentry | Auth token | organizations、issues |
| Vercel | Access token | user、deployments |

### OAuth-first 仍待补完整闭环

| Provider | Current state | Notes |
|---|---|---|
| Jira | OAuth metadata only | 还需要站点/cloud id 选择与 API base URL 绑定。 |
| Slack | OAuth metadata only | 还需要 workspace app 配置、bot/user token 边界和消息 action。 |
| Gmail | OAuth metadata only | 需要 Google OAuth client、scope 组合、邮件读取/草稿 action。 |
| Google Drive | OAuth metadata only | 需要 Drive 文件列表、下载、权限边界。 |
| Google Docs | OAuth metadata only | 需要 Docs 读取/写入 action。 |
| Google Sheets | OAuth metadata only | 需要 Sheets 读取/写入 action。 |
| Google Calendar | OAuth metadata only | 需要日程读取/创建 action。 |
