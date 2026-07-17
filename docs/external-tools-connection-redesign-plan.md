# External Tools Connection Redesign Plan

本文档定义 Multigent 外部工具接入的产品改造计划。目标是把当前偏工程后台的 `Connections` 体验，改造成普通用户能理解的“外部工具”接入体验。

## 背景

当前系统底层已经有比较完整的连接基础设施：

- `internal/connector/catalog.go` 定义 provider catalog、auth types、credential fields、actions。
- `internal/api/connection_handlers.go` 支持创建 connection、加密 secret、owner scope、grant、删除和更新。
- `internal/api/oauth_client_handlers.go` 支持管理员配置 OAuth Client、OAuth start/callback、token 存储。
- `internal/api/connection_test_handlers.go` 支持 connection test 和 health check。
- `internal/api/runtime_proxy_handlers.go` 支持 agent runtime action proxy，并由 Multigent 服务端注入第三方凭证。
- `web/src/pages/ConnectionsPage.tsx` 已经能管理 connection、OAuth client config、grant、health check。

问题在于产品表达仍然偏底层：

- 用户看到的是 `Connection`，而不是“GitHub / 飞书 / Linear 这些外部工具”。
- 创建弹框一次暴露 provider、owner、auth type、connection name、profile、health check、action policy，普通用户心智负担太重。
- OAuth client config 混在连接页面顶部，普通用户和管理员视角没有清晰分层。
- OAuth 是否可用没有按 workspace 配置动态隐藏。
- 外部工具所需的内置使用说明、工具能力和凭证配置没有统一呈现。

## 产品原则

### 用户视角只讲外部工具

用户应该看到：

```text
外部工具
- GitHub
- GitLab
- 飞书 / Lark
- Linear
- Jira
- Notion
- DingTalk
- Custom HTTP
- Custom MCP
```

不要在主路径里强调：

- connection
- connector
- plugin
- MCP
- skill

这些仍然可以作为内部实现概念存在。

### Skill 不作为用户配置项

这里的 skill 指“外部工具使用说明 / tool skill”，不是原有的 Skill 管理页面。

普通用户不应该被要求编辑外部工具 skill。每个外部工具应该有内置说明，告诉 agent 如何通过 `mga` runtime action / proxy 使用该工具。

用户只需要做三件事：

1. 配置凭证。
2. 测试工具是否可用。
3. 授权给 project / agent。

### 凭证方式优先级

每个外部工具可以有多种接入方式：

- Token / PAT / API Key
- App ID + App Secret
- Webhook Token
- Service Account
- OAuth 授权

默认策略：

```text
优先展示静态凭证 / app credential。
只有当 workspace 管理员已经配置 OAuth Client 时，普通用户才看到 OAuth 授权入口。
```

OAuth 不是主路径的唯一入口。它适合个人账号授权，但会增加管理员配置成本。

### OAuth 展示规则

Provider 支持 OAuth 不代表所有用户都能看到 OAuth。

规则：

```text
if provider supports api_key/custom_credential:
  show token/app credential method

if provider supports oauth2 and workspace OAuth client is configured:
  show OAuth authorization method

if provider supports oauth2 but OAuth client is not configured:
  hide OAuth method for normal users
  show admin-only hint/action: Configure OAuth app
```

这样普通用户不会看到不可用入口，管理员仍然知道可以启用 OAuth。

## 当前能力盘点

### 已有 Provider

当前 `internal/connector/catalog.go` 已有：

| Provider | 当前 auth type | 当前 action/proxy 状态 |
|---|---|---|
| GitHub | `api_key`, `oauth2` | `/user`, repo issues action proxy |
| Feishu | `custom_credential` | tenant token + OpenAPI action proxy |
| Lark | `custom_credential` | tenant token + OpenAPI action proxy |
| Linear | `api_key` | GraphQL action proxy |
| DingTalk Bot | `api_key` | Bot send action proxy |
| Custom MCP | `custom_credential`, `no_auth` | MCP proxy |
| Custom HTTP | `custom_credential` | generic HTTP action proxy |

### 已有数据抽象

当前底层抽象可以保留：

- Provider：平台和能力目录。
- Connection：一次具体凭证接入。
- ConnectionSecret：加密保存的 secret。
- ConnectionGrant：授权给 workspace / project / agent / user。
- OAuthClientConfig：管理员配置的 OAuth App。
- Runtime Manifest：agent 运行时可见的工具清单。

需要改的是产品命名和 UI flow，不需要推倒底层。

## 目标体验

### 外部工具列表页

页面名：

```text
外部工具 / External Tools
```

主内容不再优先展示 connection card，而是展示 provider card。

每个工具卡片显示：

- 工具名。
- 说明。
- 状态：未配置 / 已配置 / 部分可用 / 失败。
- 可用接入方式：Token、App Credential、OAuth。
- OAuth 是否已启用。
- 已配置凭证数量。
- 已授权项目/Agent 数量。
- 最近健康检查状态。
- 可用能力数量。

示例：

```text
GitHub
用于读取 issue、创建 issue、读取 PR、评论 PR。

状态：已配置
接入方式：Token、OAuth
凭证：2 个
授权：3 个项目，5 个 Agent
健康检查：正常，10 分钟前
[配置] [授权] [测试]
```

### 工具详情 / 配置弹框

用户点击工具后进入该工具的配置面板。

结构：

```text
GitHub

1. 选择接入方式
   - Token / PAT
   - OAuth 授权（仅配置后展示）

2. 填写凭证
   - Token / App ID / App Secret / Webhook URL 等

3. 如何获取凭证
   - 可展开文档
   - 显示第三方平台后台路径
   - 显示建议权限

4. 测试连接
   - 成功后显示账号信息 / scope / 权限摘要

5. 授权范围
   - workspace / project / agent
```

高级项默认折叠：

- connection name
- action policy
- health check interval
- profile metadata override

普通用户默认不需要看到这些字段。

### 管理员 OAuth Apps 页面

OAuth Client 配置不应该混在外部工具主流程里。

建议入口：

```text
Settings -> External Tool OAuth Apps
```

只对 workspace admin 可见。

每个 OAuth provider 显示：

- provider name
- configured / not configured
- client id
- callback URL
- scopes
- last updated
- configure / delete

管理员配置后，外部工具页面才向普通用户展示 OAuth 授权入口。

## 数据/API 改造计划

### Phase 1：不改 DB，只调整 API 聚合和 UI

新增或复用一个聚合 API：

```http
GET /api/v1/tools
```

返回 provider + 当前用户可见 connection summary + OAuth availability。

可以先在前端聚合已有 API：

- `GET /api/v1/connectors/providers`
- `GET /api/v1/connections`
- admin only: `GET /api/v1/oauth/client-configs`

如果前端聚合变复杂，再下沉到后端。

工具 summary 建议结构：

```json
{
  "provider": "github",
  "displayName": "GitHub",
  "description": "Use GitHub issues, repositories, pull requests, and comments.",
  "authMethods": [
    {
      "type": "api_key",
      "label": "Personal access token",
      "available": true,
      "recommended": true
    },
    {
      "type": "oauth2",
      "label": "OAuth authorization",
      "available": true,
      "configured": true
    }
  ],
  "connections": [],
  "connectionCount": 2,
  "grantCount": 5,
  "lastHealth": {}
}
```

### Phase 2：Provider catalog 增强

当前 provider catalog 只有 fields/actions，不足以支撑产品化引导。

给 provider 增加非敏感元数据：

```go
Description string
Category string
AuthMethods []AuthMethodDescriptor
CredentialGuide []GuideSection
RecommendedScopes []string
RecommendedGrants []string
```

其中 `CredentialGuide` 用于前端展示“如何获取凭证”。

示例：

```json
{
  "title": "Create a GitHub personal access token",
  "steps": [
    "Open GitHub Developer settings.",
    "Create a fine-grained personal access token.",
    "Grant repository access required by your project.",
    "Copy the token and paste it here."
  ],
  "links": [
    {
      "label": "GitHub token settings",
      "url": "https://github.com/settings/tokens"
    }
  ]
}
```

### Phase 3：OAuth availability 权限化

普通用户不能看到未配置 OAuth 的入口。

后端或前端必须计算：

```text
provider supports oauth2
AND workspace has OAuthClientConfig(provider)
```

只有 true 时普通用户才展示 OAuth。

管理员看到：

```text
OAuth available: false
reason: OAuth app not configured
action: Configure OAuth app
```

### Phase 4：创建连接 flow 简化

当前 `ConnectionDialog` 应拆分为两层：

- `ExternalToolDialog`
- `CredentialForm`

普通 flow 字段：

- owner scope：workspace / me
- auth method
- credential fields
- guide
- test button
- grant target

高级字段折叠：

- connection name
- display name
- account profile
- health check
- runtime action policy

`ConnectionDialog` 当前暴露的信息太多，可以保留为内部组件或 advanced mode。

### Phase 5：外部工具内置说明注入 Agent

每个 provider 应该自动生成或携带一份 runtime usage instruction。

Agent 不需要直接读 raw secret，只通过 `mga` 调用：

```bash
mga runtime connections
mga runtime action --connection github_default --data '{...}'
```

未来可以按 provider 注入简短说明：

```text
You can use the GitHub external tool through mga runtime action.
List available tools with `mga runtime connections`.
Do not ask for raw GitHub tokens.
```

这些说明对用户不可编辑，避免加重心智负担。

## 前端改造计划

### 1. 导航和文案

把左侧导航：

```text
Connections -> External Tools / 外部工具
```

保留路由 `/connections` 也可以，先不强制改 URL；但页面标题和文案全部改成 External Tools。

### 2. 列表页布局

从 connection card 列表改成 provider card 目录。

空状态也不再是“暂无连接”，而是展示可接入工具目录。

### 3. 工具卡片

卡片以 provider 为单位，而不是 connection 为单位。

每张卡片聚合：

- provider display name
- connection count
- grant count
- health status
- supported auth methods
- primary action: Configure

### 4. 工具配置弹框

弹框第一步选择 auth method。

Auth method 列表动态生成：

- `api_key` / `custom_credential` 永远展示。
- `oauth2` 仅当配置可用时展示。
- 未配置 OAuth 时，管理员展示配置入口，普通用户隐藏。

### 5. 凭证获取指南

每个 provider 提供 guide panel。

第一版可以 hardcode 在前端或 provider catalog，建议最终放 provider catalog。

### 6. 授权

授权仍使用现有 grant API。

但 UI 文案从：

```text
Connection Grants
```

改为：

```text
授权给项目或 Agent
```

## 后端改造计划

### 1. Provider metadata

扩展 `connector.Provider`：

```go
Description string
Category string
Guides map[string]CredentialGuide
```

保持向后兼容 API response，但这是新项目，不需要保留错误产品语义。

### 2. OAuth availability

新增 helper：

```go
ToolAuthAvailability(provider, workspaceID, userRole)
```

用于告诉前端 OAuth 是否应该展示。

### 3. Tool summary API

可选新增：

```http
GET /api/v1/tools
GET /api/v1/tools/{provider}
```

短期可由前端聚合，长期建议后端提供，避免权限逻辑散落前端。

### 4. Connection test profile 自动填充

当前用户还要手动填 account profile。后续应在 test 成功后自动写入：

- account id
- account name
- email
- scopes
- last validated

GitHub `/user`、Linear viewer、Feishu/Lark token response 或 probe endpoint 都可以逐步补。

### 5. Default action policy

普通用户不应该手动填 allowed endpoints。

Provider catalog 应该提供默认安全 action policy。高级用户才修改。

## 多语言计划

新增或调整 locale 命名空间：

- `externalTools.title`
- `externalTools.subtitle`
- `externalTools.configure`
- `externalTools.credentials`
- `externalTools.authorize`
- `externalTools.health`
- `externalTools.credentialGuide`
- `externalTools.oauthUnavailableForUsers`
- `externalTools.configureOAuthApp`

旧的 `connections.*` 可以逐步保留或重命名，但用户可见文案应该尽快统一为“外部工具”。

## 验收标准

第一阶段验收：

- 左侧导航和页面标题不再显示 Connections，而是 External Tools / 外部工具。
- 页面按第三方平台展示，而不是按 connection 展示。
- GitHub 支持 PAT 创建、测试、授权给 agent。
- OAuth 未配置时，普通用户看不到 OAuth 方式。
- OAuth 已配置时，普通用户能看到 OAuth 授权方式。
- 管理员可以在设置里配置 OAuth Client，并看到 callback URL。
- 凭证获取指南可在工具弹框内展开查看。
- 创建后 agent runtime manifest 仍能拿到对应工具能力。
- 普通用户不需要理解 skill / MCP / connection 才能完成工具接入。

## 风险和取舍

### OAuth 不能完全取消

OAuth 对个人授权体验更丝滑，也适合 Google、Slack、Notion 等平台。不能因为第一版偏企业内部凭证就删除 OAuth。

正确做法是隐藏未配置 OAuth，而不是删除 OAuth 能力。

### 静态凭证也不是永远简单

PAT / app secret 更容易上手，但需要客户自己管理权限和泄漏风险。Multigent 必须提供：

- 加密存储。
- 最小授权建议。
- 健康检查。
- 审计。
- grant 边界。

### 不暴露 skill 会降低灵活性

普通用户不编辑外部工具 skill 是正确的。但高级团队未来可能需要自定义工具使用说明。可以后续做 “Advanced instructions override”，默认隐藏。

## 建议实施顺序

1. 写 provider guide metadata。
2. 改页面文案和导航为 External Tools。
3. 把列表页从 connection card 改成 provider card。
4. 工具弹框支持 auth method 动态展示。
5. OAuth 未配置时对普通用户隐藏，对管理员显示配置入口。
6. 凭证获取指南接入弹框。
7. 简化普通表单，把 advanced 字段折叠。
8. 补 test 成功后的 profile 自动填充。
9. 补工具内置 runtime instruction 注入。
10. 根据真实 POC 再决定是否新增后端 `/api/v1/tools` 聚合 API。
