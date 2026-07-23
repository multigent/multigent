# 外部工具 OAuth 应用配置指引

这份文档给工作区管理员使用。用户在产品里也可以直接打开同一份指引的多语言弹框：`工作区设置 -> 外部工具 OAuth 应用 -> 查看配置文档`。

核心规则很简单：

1. 在 Multigent 的 `工作区设置 -> 外部工具 OAuth 应用` 里复制对应平台的 `Redirect URI`。
2. 去第三方平台的开发者后台创建 OAuth 应用。
3. 把 Multigent 的 `Redirect URI` 原样填到第三方平台的 callback / redirect URL。
4. 把第三方平台生成的 `Client ID` 和 `Client Secret` 保存回 Multigent。

如果某个平台没有在这里配置 OAuth 应用，Multigent 会在外部工具页隐藏 OAuth 授权入口，只展示 API Key、PAT、App Credential 等其他已支持的连接方式。

## 开始前

1. 打开 Multigent。
2. 进入 `工作区设置 -> 外部工具 OAuth 应用`。
3. 找到要配置的平台行。
4. 复制该行展示的 `Redirect URI`。
5. 保持页面打开，后面要把第三方平台生成的 `Client ID` 和 `Client Secret` 填回来。

本地开发环境的 Redirect URI 通常类似：

```text
http://127.0.0.1:27893/api/v1/oauth/callback
```

生产环境必须使用公开可访问的 HTTPS 域名。

## GitHub

用于代码仓库、Issue、Pull Request、Release、Workflow 和组织上下文。

1. 打开 `https://github.com/settings/developers`。
2. 进入 `OAuth Apps`。
3. 点击 `New OAuth App`。
4. 填写：
   - `Application name`: `Multigent`
   - `Homepage URL`: 你的 Multigent Web 地址
   - `Authorization callback URL`: 从 Multigent 复制的 Redirect URI
5. 点击 `Register application`。
6. 复制 `Client ID`。
7. 点击 `Generate a new client secret`，立即复制 `Client Secret`。
8. 回到 Multigent 的 GitHub 行，点击 `编辑` 并保存。

注意：GitHub 的 Client Secret 只会完整显示一次。

## Slack

用于 Slack 频道、用户、消息和通知工作流。

1. 打开 `https://api.slack.com/apps`。
2. 点击 `Create New App`，选择 `From scratch`。
3. `App Name` 填 `Multigent`，并选择 Slack workspace。
4. 进入左侧 `OAuth & Permissions`。
5. 在 `Redirect URLs` 添加 Multigent 的 Redirect URI。
6. 在 `Scopes` 添加需要的 Bot Token Scopes 或 User Token Scopes。
7. 进入左侧 `Basic Information`。
8. 复制 `Client ID` 和 `Client Secret`。
9. 回到 Multigent 的 Slack 行保存。

## Google / Gmail / Drive / Docs / Sheets / Calendar

用于 Gmail、Google Drive、Google Docs、Google Sheets、Google Calendar 等工具。

1. 打开 `https://console.cloud.google.com/apis/credentials`。
2. 选择或创建 Google Cloud Project。
3. 进入 `OAuth consent screen`，配置应用名称、支持邮箱和开发者联系方式。
4. 回到 `Credentials`，点击 `Create credentials -> OAuth client ID`。
5. `Application type` 选择 `Web application`。
6. `Authorized JavaScript origins` 填 Multigent Web Origin，例如 `https://multigent.example.com`。
7. `Authorized redirect URIs` 粘贴 Multigent 的 Redirect URI。
8. 创建后复制 `Client ID` 和 `Client Secret`。
9. 回到 Multigent 对应的 Google 工具行保存。

注意：Gmail 等敏感权限在生产环境可能需要 Google 审核。如果出现 `redirect_uri_mismatch`，优先检查 Google Cloud 里的 Redirect URI 是否和 Multigent 完全一致。

## Jira / Atlassian

用于 Jira Issue、Epic、Project 和研发计划上下文。

1. 打开 `https://developer.atlassian.com/console/myapps/`。
2. 点击 `Create app`。
3. 创建或启用 OAuth 2.0 能力。
4. 在 `Permissions` 里添加需要的 Jira API 权限。
5. 在 `Authorization` 中添加 Multigent 的 Redirect URI 作为 Callback URL。
6. 保存应用。
7. 复制 `Client ID`，生成或复制 `Client Secret`。
8. 回到 Multigent 的 Jira 行保存。

## Figma

用于设计文件和用户授权的设计上下文。

1. 打开 `https://www.figma.com/developers/apps`。
2. 创建新应用。
3. `App name` 填 `Multigent`。
4. `Website URL` 填你的 Multigent 访问地址。
5. 找到 OAuth redirect / callback URL 设置，粘贴 Multigent 的 Redirect URI。
6. 保存应用。
7. 复制 `Client ID` 和 `Client Secret`。
8. 回到 Multigent 的 Figma 行保存。

## Feishu / Lark

Feishu 和 Lark 当前在 Multigent 外部工具页使用 `App ID` 和 `App Secret` 配置，不走 `外部工具 OAuth 应用` 这张表。

- Feishu: `https://open.feishu.cn/app`
- Lark: `https://open.larksuite.com/app`

Agent 协作渠道绑定在 Agent 页面处理，它和这里的 OAuth 应用配置是两条流程。

## 常见问题

### 普通用户看不到 OAuth 授权入口

检查对应平台是否已经在 `工作区设置 -> 外部工具 OAuth 应用` 保存了 `Client ID` 和 `Client Secret`。没有配置时，Multigent 会隐藏 OAuth 入口。

### 第三方平台提示 redirect URI mismatch

逐字符检查协议、域名、端口和路径。常见差异包括：

- `http` 和 `https` 不一致
- `127.0.0.1` 和 `localhost` 不一致
- 缺少端口
- 缺少 `/api/v1/oauth/callback`
- 生产域名已经变化但第三方平台没有同步更新

### 授权成功但权限不足

检查 scope 和应用发布状态。部分平台要求应用安装、发布、审核或被组织管理员允许后，用户才能授权。

### Client Secret 泄露

立刻在第三方平台轮换密钥，然后更新 Multigent。不要把 Client Secret 粘贴到 prompt、文档、任务、聊天消息或 Agent notes 中。

## English Reference

This guide is for workspace admins. The same guidance is available as a multilingual in-app dialog from `Workspace Settings -> External tool OAuth apps -> Setup guide`.

The rule is: copy the `Redirect URI` from Multigent, create an OAuth app in the provider developer console, paste that URI as the callback / redirect URL, then save the provider `Client ID` and `Client Secret` back into Multigent.

Provider entry points:

- GitHub: `https://github.com/settings/developers` -> `OAuth Apps` -> `New OAuth App`
- Slack: `https://api.slack.com/apps` -> `Create New App` -> `OAuth & Permissions`
- Google APIs: `https://console.cloud.google.com/apis/credentials` -> `Create credentials` -> `OAuth client ID`
- Jira / Atlassian: `https://developer.atlassian.com/console/myapps/` -> `Create app`
- Figma: `https://www.figma.com/developers/apps` -> create an app and configure the OAuth redirect URL
- Feishu: `https://open.feishu.cn/app`
- Lark: `https://open.larksuite.com/app`

If users cannot see OAuth on the External Tools page, first confirm that the workspace admin has saved a Client ID and Client Secret for that provider. If the provider reports `redirect_uri_mismatch`, compare the provider callback URL with the Multigent Redirect URI character by character.
