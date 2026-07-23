import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { KeyRound, Plus, Server, Trash2, Pencil, X, Eye, EyeOff, Users, Shield, ShieldCheck, UserPlus, LockKeyhole } from 'lucide-react'
import { useAuth } from '../lib/auth'
import { useWorkspaceAccess } from '../lib/workspace-access'
import { apiFetch, apiPost, apiPut, apiDelete } from '../lib/api'
import { cn } from '../lib/cn'
import { confirmDialog } from '../components/ui/ConfirmDialog'

const selectCls =
  'max-w-xs rounded-md border border-neutral-200/80 bg-neutral-50/50 px-3 py-2 text-sm text-neutral-800 outline-none transition-colors focus:border-sky-400 dark:border-zinc-700/60 dark:bg-zinc-800 dark:text-zinc-200 dark:[color-scheme:dark] [&>option]:dark:bg-zinc-800 [&>option]:dark:text-zinc-200'
const inputCls =
  'block w-full max-w-xs rounded-md border border-neutral-200/80 bg-neutral-50/50 px-3 py-2 text-sm text-neutral-800 outline-none transition-colors placeholder:text-neutral-400 focus:border-sky-400 dark:border-zinc-700/60 dark:bg-zinc-800/50 dark:text-zinc-200 dark:placeholder:text-zinc-600'

type UserRow = {
  username: string; role: string; displayName?: string
  email?: string; avatar?: string; phone?: string; bio?: string
  projects?: { project: string; role: string }[]
  linkedAgents?: string[]; disabled?: boolean; createdAt?: string
}

type ProjectItem = { name: string }

const PROJECT_ROLES = ['viewer', 'operator', 'manager'] as const
const USER_ROLES = ['admin', 'member'] as const

type RBACModel = {
  scopes: { name: string; roles: string[] }[]
  capabilities: { id: string; scope: string; label: string }[]
}
type TFn = (key: string, options?: Record<string, unknown>) => string

type OAuthClientConfig = {
  provider: string
  displayName: string
  configured: boolean
  clientId?: string
  expectedRedirectUri: string
  oauth?: { scopes?: string[] }
  extra?: Record<string, unknown>
}

function RBACSection() {
  const { t } = useTranslation()
  const [model, setModel] = useState<RBACModel | null>(null)
  useEffect(() => {
    apiFetch<RBACModel>('/api/v1/rbac/model').then(setModel).catch(() => {})
  }, [])
  const capsByScope = new Map<string, { id: string; scope: string; label: string }[]>()
  for (const cap of model?.capabilities ?? []) {
    const list = capsByScope.get(cap.scope) ?? []
    list.push(cap)
    capsByScope.set(cap.scope, list)
  }
  return (
    <section className="rounded-xl border border-neutral-200/80 bg-white p-5 dark:border-zinc-700/60 dark:bg-zinc-900/40">
      <div className="flex items-center gap-2 pb-3">
        <LockKeyhole className="size-4 text-neutral-500 dark:text-zinc-500" strokeWidth={1.8} />
        <h3 className="text-base font-semibold text-neutral-900 dark:text-zinc-100">RBAC / Agent permissions</h3>
      </div>
      <p className="mb-4 text-xs text-neutral-400 dark:text-zinc-500">
        Workspace controls people and billing; Project scopes collaboration; Task controls execution; Agent and Context Pack are explicit responsibility and knowledge boundaries.
      </p>
      {!model ? (
        <p className="py-3 text-sm text-neutral-400">{t('forms.loading')}</p>
      ) : (
        <div className="grid gap-3 lg:grid-cols-2">
          {model.scopes.map(scope => (
            <div key={scope.name} className="rounded-lg border border-neutral-200/80 bg-neutral-50/40 p-3 dark:border-zinc-700/60 dark:bg-zinc-800/30">
              <div className="flex items-center justify-between gap-3">
                <h4 className="font-mono text-sm font-semibold text-neutral-900 dark:text-zinc-100">{scope.name}</h4>
                <div className="flex flex-wrap justify-end gap-1">
                  {scope.roles.map(role => (
                    <span key={role} className="rounded-md bg-white px-2 py-0.5 text-[11px] font-medium text-neutral-600 dark:bg-zinc-900 dark:text-zinc-400">{role}</span>
                  ))}
                </div>
              </div>
              <div className="mt-3 flex flex-wrap gap-1.5">
                {(capsByScope.get(scope.name) ?? []).map(cap => (
                  <span key={cap.id} title={cap.label} className="rounded-md bg-sky-50 px-2 py-0.5 font-mono text-[11px] text-sky-700 dark:bg-sky-900/20 dark:text-sky-300">{cap.id}</span>
                ))}
              </div>
            </div>
          ))}
        </div>
      )}
    </section>
  )
}

function ExternalToolOAuthSection() {
  const { t, i18n } = useTranslation()
  const [configs, setConfigs] = useState<OAuthClientConfig[]>([])
  const [loading, setLoading] = useState(true)
  const [editing, setEditing] = useState<OAuthClientConfig | null>(null)
  const [showGuide, setShowGuide] = useState(false)

  async function refresh() {
    setLoading(true)
    try {
      const data = await apiFetch<{ configs: OAuthClientConfig[] }>('/api/v1/oauth/client-configs')
      setConfigs(data.configs ?? [])
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void refresh() }, [])

  return (
    <section className="rounded-xl border border-neutral-200/80 bg-white p-5 dark:border-zinc-700/60 dark:bg-zinc-900/40">
      <div className="flex items-start justify-between gap-3 pb-4">
        <div>
          <h3 className="text-base font-semibold text-neutral-900 dark:text-zinc-100">{t('settings.externalToolOAuthTitle')}</h3>
          <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">{t('settings.externalToolOAuthIntro')}</p>
        </div>
        <button
          type="button"
          onClick={() => setShowGuide(true)}
          className="shrink-0 rounded-lg border border-sky-600 bg-white px-3 py-2 text-sm font-medium text-sky-700 hover:bg-sky-50 dark:border-sky-500 dark:bg-zinc-900 dark:text-sky-400 dark:hover:bg-zinc-800"
        >
          {t('settings.oauthSetupGuide')}
        </button>
      </div>
      {loading ? (
        <p className="py-3 text-sm text-neutral-400">{t('forms.loading')}</p>
      ) : (
        <div className="overflow-hidden rounded-lg border border-neutral-200/80 dark:border-zinc-700/60">
          <table className="w-full text-left">
            <thead className="bg-neutral-50 text-xs text-neutral-500 dark:bg-zinc-800/50 dark:text-zinc-400">
              <tr>
                <th className="px-4 py-2.5 font-medium">{t('settings.oauthProvider')}</th>
                <th className="px-4 py-2.5 font-medium">{t('settings.oauthStatus')}</th>
                <th className="px-4 py-2.5 font-medium">Client ID</th>
                <th className="px-4 py-2.5 font-medium">Redirect URI</th>
                <th className="px-4 py-2.5 text-right font-medium">{t('common.actions')}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-neutral-200/80 dark:divide-zinc-700/60">
              {configs.map(config => (
                <tr key={config.provider}>
                  <td className="px-4 py-3 text-sm font-medium text-neutral-800 dark:text-zinc-100">{config.displayName}</td>
                  <td className="px-4 py-3">
                    <span className={cn('rounded-full px-2 py-0.5 text-xs font-medium', config.configured ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300' : 'bg-neutral-100 text-neutral-500 dark:bg-zinc-800 dark:text-zinc-400')}>
                      {config.configured ? t('connections.configured') : t('connections.notConfigured')}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-xs text-neutral-500 dark:text-zinc-400">{config.clientId || '—'}</td>
                  <td className="max-w-sm truncate px-4 py-3 font-mono text-xs text-neutral-400 dark:text-zinc-500" title={config.expectedRedirectUri}>{config.expectedRedirectUri}</td>
                  <td className="px-4 py-3 text-right">
                    <button type="button" onClick={() => setEditing(config)} className="rounded-lg border border-neutral-200 px-3 py-1.5 text-xs font-medium text-neutral-600 hover:bg-neutral-50 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800">
                      {t('common.edit')}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {editing && (
        <OAuthClientConfigDialog
          config={editing}
          onClose={() => setEditing(null)}
          onSaved={() => { setEditing(null); void refresh() }}
        />
      )}
      {showGuide && (
        <OAuthSetupGuideDialog
          lang={i18n.language}
          onClose={() => setShowGuide(false)}
        />
      )}
    </section>
  )
}

type OAuthGuideSection = {
  title: string
  intro?: string
  steps: string[]
  notes?: string[]
}

type OAuthGuideContent = {
  title: string
  intro: string
  beforeTitle: string
  beforeSteps: string[]
  sections: OAuthGuideSection[]
  troubleshootingTitle: string
  troubleshooting: string[]
}

function oauthGuideLocale(language: string): 'en' | 'zh-CN' | 'zh-TW' | 'ja' {
  if (language.startsWith('zh-TW') || language === 'zh-Hant') return 'zh-TW'
  if (language.startsWith('zh')) return 'zh-CN'
  if (language.startsWith('ja')) return 'ja'
  return 'en'
}

function oauthSetupGuideContent(language: string): OAuthGuideContent {
  const locale = oauthGuideLocale(language)
  if (locale === 'zh-CN') {
    return {
      title: '外部工具 OAuth 应用配置指引',
      intro: '管理员先在第三方平台创建 OAuth 应用，把 Multigent 显示的 Redirect URI 配进去，再把平台生成的 Client ID 和 Client Secret 保存回 Multigent。配置完成后，普通用户才能在外部工具页看到 OAuth 授权入口。',
      beforeTitle: '开始前',
      beforeSteps: [
        '打开「工作区设置 -> 外部工具 OAuth 应用」。',
        '找到要配置的平台行，例如 GitHub、Slack、Google Drive、Jira。',
        '复制该行展示的 Redirect URI，后面要原样粘贴到第三方平台。',
        '配置完成后回到该行，点击「编辑」，保存 Client ID 和 Client Secret。',
      ],
      sections: [
        {
          title: 'GitHub',
          intro: '用于代码仓库、Issue、PR、Release、Workflow 等研发上下文。',
          steps: [
            '打开 https://github.com/settings/developers。',
            '进入 OAuth Apps，点击 New OAuth App。',
            'Application name 填 Multigent。',
            'Homepage URL 填你的 Multigent 访问地址。',
            'Authorization callback URL 粘贴 Multigent 的 Redirect URI。',
            '点击 Register application。',
            '复制 Client ID。',
            '点击 Generate a new client secret，立即复制 Client Secret。',
            '回到 Multigent 的 GitHub 行，保存 Client ID 和 Client Secret。',
          ],
          notes: ['GitHub 的 Client Secret 只会完整显示一次。', '如果 Multigent 域名变化，需要同步修改 GitHub OAuth App 的 callback URL。'],
        },
        {
          title: 'Slack',
          intro: '用于 Slack 频道、用户、消息和通知工作流。',
          steps: [
            '打开 https://api.slack.com/apps。',
            '点击 Create New App，选择 From scratch。',
            'App Name 填 Multigent，并选择要安装的 Slack workspace。',
            '进入左侧 OAuth & Permissions。',
            '在 Redirect URLs 点击 Add New Redirect URL。',
            '粘贴 Multigent 的 Redirect URI，点击 Save URLs。',
            '在 Scopes 中添加需要的 Bot Token Scopes 或 User Token Scopes。',
            '进入左侧 Basic Information。',
            '复制 Client ID 和 Client Secret。',
            '回到 Multigent 的 Slack 行，保存 Client ID 和 Client Secret。',
          ],
          notes: ['Slack 的 Redirect URLs 在 OAuth & Permissions 页面。', 'Slack 的 Client ID 和 Client Secret 在 Basic Information 页面。'],
        },
        {
          title: 'Google / Gmail / Drive / Docs / Sheets / Calendar',
          intro: '用于 Gmail、Google Drive、Google Docs、Google Sheets、Google Calendar 等工具。',
          steps: [
            '打开 https://console.cloud.google.com/apis/credentials。',
            '选择或创建一个 Google Cloud Project。',
            '进入 OAuth consent screen，配置应用名称、支持邮箱和开发者联系方式。',
            '回到 Credentials，点击 Create credentials -> OAuth client ID。',
            'Application type 选择 Web application。',
            'Name 填 Multigent。',
            'Authorized JavaScript origins 填你的 Multigent Web Origin，例如 https://multigent.example.com。',
            'Authorized redirect URIs 粘贴 Multigent 的 Redirect URI。',
            '点击 Create。',
            '复制 Client ID 和 Client Secret。',
            '回到 Multigent 对应的 Google 工具行保存。',
          ],
          notes: ['Gmail 通常会触发更严格的 Google 审核；生产环境可能需要完成 OAuth consent screen 验证。', '如果报 redirect_uri_mismatch，通常是 Google Cloud 里的 Redirect URI 和 Multigent 展示的不完全一致。'],
        },
        {
          title: 'Jira / Atlassian',
          intro: '用于 Jira Issue、Epic、Project 和研发计划上下文。',
          steps: [
            '打开 https://developer.atlassian.com/console/myapps/。',
            '点击 Create app。',
            '创建或启用 OAuth 2.0 授权能力。',
            '进入 Permissions，添加需要的 Jira API 权限。',
            '进入 Authorization，添加 Multigent 的 Redirect URI 作为 Callback URL。',
            '保存应用。',
            '复制 Client ID，生成或复制 Client Secret。',
            '回到 Multigent 的 Jira 行保存。',
          ],
          notes: ['Jira 和 Confluence 权限是分开的，不要默认加不需要的权限。'],
        },
        {
          title: 'Figma',
          intro: '用于设计文件和用户授权的设计上下文。',
          steps: [
            '打开 https://www.figma.com/developers/apps。',
            '创建一个新应用。',
            'App name 填 Multigent。',
            'Website URL 填你的 Multigent 访问地址。',
            '找到 OAuth redirect / callback URL 设置，粘贴 Multigent 的 Redirect URI。',
            '保存应用。',
            '复制 Client ID 和 Client Secret。',
            '回到 Multigent 的 Figma 行保存。',
          ],
          notes: ['Figma 也可能通过 Token 或 MCP 方式接入；只有需要用户授权时才使用 OAuth。'],
        },
        {
          title: 'Feishu / Lark',
          intro: '目前 Feishu / Lark 在 Multigent 外部工具页使用 App ID 和 App Secret，不走这个 OAuth 应用表。',
          steps: [
            'Feishu 打开 https://open.feishu.cn/app。',
            'Lark 打开 https://open.larksuite.com/app。',
            '创建企业自建应用，复制 App ID 和 App Secret。',
            '回到 Multigent 外部工具页配置 Feishu 或 Lark。',
            'Agent 聊天渠道绑定则在 Agent 页面使用协作渠道连接流程。',
          ],
        },
      ],
      troubleshootingTitle: '常见问题',
      troubleshooting: [
        '普通用户看不到 OAuth 授权入口：确认管理员已经在本页保存 Client ID 和 Client Secret。',
        'redirect_uri_mismatch：逐字符检查协议、域名、端口和 /api/v1/oauth/callback 路径。',
        '授权后权限不足：补充平台 scope 后，让用户重新授权。',
        'Client Secret 泄露：在平台控制台轮换密钥，并更新 Multigent 配置。',
      ],
    }
  }
  if (locale === 'zh-TW') {
    return {
      title: '外部工具 OAuth 應用配置指引',
      intro: '管理員先在第三方平台建立 OAuth 應用，把 Multigent 顯示的 Redirect URI 配進去，再把平台產生的 Client ID 和 Client Secret 保存回 Multigent。',
      beforeTitle: '開始前',
      beforeSteps: [
        '打開「工作區設定 -> 外部工具 OAuth 應用」。',
        '找到要設定的平台列，例如 GitHub、Slack、Google Drive、Jira。',
        '複製該列展示的 Redirect URI。',
        '配置完成後回到該列，點擊「編輯」，保存 Client ID 和 Client Secret。',
      ],
      sections: [
        {
          title: 'GitHub',
          steps: ['打開 https://github.com/settings/developers。', '進入 OAuth Apps，點擊 New OAuth App。', 'Application name 填 Multigent。', 'Homepage URL 填你的 Multigent 網址。', 'Authorization callback URL 貼上 Multigent 的 Redirect URI。', '註冊後複製 Client ID，產生並複製 Client Secret。', '回到 Multigent 的 GitHub 列保存。'],
          notes: ['GitHub 的 Client Secret 只會完整顯示一次。'],
        },
        {
          title: 'Slack',
          steps: ['打開 https://api.slack.com/apps。', '點擊 Create New App，選擇 From scratch。', 'App Name 填 Multigent 並選擇 workspace。', '進入 OAuth & Permissions。', '在 Redirect URLs 新增 Multigent 的 Redirect URI。', '在 Scopes 加入需要的權限。', '到 Basic Information 複製 Client ID 和 Client Secret。', '回到 Multigent 的 Slack 列保存。'],
        },
        {
          title: 'Google / Gmail / Drive / Docs / Sheets / Calendar',
          steps: ['打開 https://console.cloud.google.com/apis/credentials。', '選擇或建立 Google Cloud Project。', '設定 OAuth consent screen。', 'Create credentials -> OAuth client ID。', 'Application type 選 Web application。', 'Authorized redirect URIs 貼上 Multigent 的 Redirect URI。', '建立後複製 Client ID 和 Client Secret。', '回到 Multigent 對應 Google 工具列保存。'],
          notes: ['Gmail 等敏感 scope 在正式環境可能需要 Google 驗證。'],
        },
        {
          title: 'Jira / Atlassian',
          steps: ['打開 https://developer.atlassian.com/console/myapps/。', 'Create app。', '啟用 OAuth 2.0。', '在 Permissions 加入需要的 Jira API 權限。', '在 Authorization 增加 Multigent Redirect URI。', '複製 Client ID 和 Client Secret 回填 Multigent。'],
        },
        {
          title: 'Figma',
          steps: ['打開 https://www.figma.com/developers/apps。', '建立新 app。', 'App name 填 Multigent。', 'OAuth redirect / callback URL 貼上 Multigent Redirect URI。', '複製 Client ID 和 Client Secret 回填 Multigent。'],
        },
        {
          title: 'Feishu / Lark',
          steps: ['Feishu 使用 https://open.feishu.cn/app。', 'Lark 使用 https://open.larksuite.com/app。', '目前在 Multigent 外部工具頁填 App ID 和 App Secret，不走這個 OAuth 應用表。'],
        },
      ],
      troubleshootingTitle: '常見問題',
      troubleshooting: ['看不到 OAuth 授權入口：先確認本頁已保存 Client ID 和 Client Secret。', 'redirect_uri_mismatch：檢查 Redirect URI 是否完全一致。', '權限不足：補 scope 後重新授權。'],
    }
  }
  if (locale === 'ja') {
    return {
      title: '外部ツール OAuth アプリ設定ガイド',
      intro: '管理者が各プロバイダーの開発者コンソールで OAuth アプリを作成し、Multigent が表示する Redirect URI を登録してから、Client ID と Client Secret を Multigent に保存します。',
      beforeTitle: '始める前に',
      beforeSteps: [
        'Workspace Settings -> External tool OAuth apps を開きます。',
        'GitHub、Slack、Google Drive、Jira など設定したい行を探します。',
        'その行の Redirect URI をコピーします。',
        'プロバイダー側でアプリを作成した後、Client ID と Client Secret を Multigent に保存します。',
      ],
      sections: [
        {
          title: 'GitHub',
          steps: ['https://github.com/settings/developers を開きます。', 'OAuth Apps -> New OAuth App を選びます。', 'Application name に Multigent を入力します。', 'Homepage URL に Multigent の URL を入力します。', 'Authorization callback URL に Multigent の Redirect URI を貼り付けます。', '登録後、Client ID と Client Secret をコピーして Multigent に保存します。'],
        },
        {
          title: 'Slack',
          steps: ['https://api.slack.com/apps を開きます。', 'Create New App -> From scratch を選びます。', 'App Name に Multigent を入力し workspace を選びます。', 'OAuth & Permissions を開きます。', 'Redirect URLs に Multigent の Redirect URI を追加します。', '必要な Scopes を追加します。', 'Basic Information で Client ID と Client Secret をコピーして Multigent に保存します。'],
        },
        {
          title: 'Google / Gmail / Drive / Docs / Sheets / Calendar',
          steps: ['https://console.cloud.google.com/apis/credentials を開きます。', 'Google Cloud Project を選びます。', 'OAuth consent screen を設定します。', 'Create credentials -> OAuth client ID を選びます。', 'Application type は Web application を選びます。', 'Authorized redirect URIs に Multigent の Redirect URI を貼り付けます。', 'Client ID と Client Secret を Multigent に保存します。'],
        },
        {
          title: 'Jira / Atlassian',
          steps: ['https://developer.atlassian.com/console/myapps/ を開きます。', 'Create app を選びます。', 'OAuth 2.0 を有効化します。', 'Permissions で必要な Jira API 権限を追加します。', 'Authorization に Multigent Redirect URI を追加します。', 'Client ID と Client Secret を Multigent に保存します。'],
        },
        {
          title: 'Figma',
          steps: ['https://www.figma.com/developers/apps を開きます。', '新しい app を作成します。', 'App name に Multigent を入力します。', 'OAuth redirect / callback URL に Multigent Redirect URI を貼り付けます。', 'Client ID と Client Secret を Multigent に保存します。'],
        },
        {
          title: 'Feishu / Lark',
          steps: ['Feishu は https://open.feishu.cn/app を使います。', 'Lark は https://open.larksuite.com/app を使います。', '現在は External Tools で App ID と App Secret を設定し、この OAuth アプリ表は使いません。'],
        },
      ],
      troubleshootingTitle: 'トラブルシューティング',
      troubleshooting: ['OAuth ボタンが表示されない場合は、このページで Client ID と Client Secret が保存されているか確認します。', 'redirect_uri_mismatch は Redirect URI の不一致です。', '権限不足の場合は scope を追加して再認可します。'],
    }
  }
  return {
    title: 'External Tool OAuth App Setup Guide',
    intro: 'A workspace admin creates an OAuth app in the provider developer console, pastes the Redirect URI shown by Multigent, then saves the provider Client ID and Client Secret back into Multigent.',
    beforeTitle: 'Before You Start',
    beforeSteps: [
      'Open Workspace Settings -> External tool OAuth apps.',
      'Find the provider row, such as GitHub, Slack, Google Drive, or Jira.',
      'Copy the exact Redirect URI shown in that row.',
      'After creating the provider app, return to this row, click Edit, and save the Client ID and Client Secret.',
    ],
    sections: [
      {
        title: 'GitHub',
        intro: 'For repositories, issues, pull requests, releases, workflows, and organization context.',
        steps: ['Open https://github.com/settings/developers.', 'Choose OAuth Apps, then New OAuth App.', 'Set Application name to Multigent.', 'Set Homepage URL to your Multigent web URL.', 'Paste the Multigent Redirect URI into Authorization callback URL.', 'Register the app.', 'Copy Client ID.', 'Generate a new client secret and copy it immediately.', 'Save Client ID and Client Secret in the GitHub row in Multigent.'],
        notes: ['GitHub shows the Client Secret only once after generation.'],
      },
      {
        title: 'Slack',
        intro: 'For Slack channels, users, messages, and notification workflows.',
        steps: ['Open https://api.slack.com/apps.', 'Click Create New App and choose From scratch.', 'Set App Name to Multigent and choose a workspace.', 'Open OAuth & Permissions from the left sidebar.', 'Under Redirect URLs, add the Multigent Redirect URI and save URLs.', 'Add the required Bot Token Scopes or User Token Scopes.', 'Open Basic Information.', 'Copy Client ID and Client Secret.', 'Save them in the Slack row in Multigent.'],
        notes: ['Redirect URLs live under OAuth & Permissions. Client ID and Client Secret live under Basic Information.'],
      },
      {
        title: 'Google / Gmail / Drive / Docs / Sheets / Calendar',
        intro: 'For Google workspace tools such as Gmail, Drive, Docs, Sheets, and Calendar.',
        steps: ['Open https://console.cloud.google.com/apis/credentials.', 'Select or create a Google Cloud Project.', 'Configure OAuth consent screen.', 'Go back to Credentials and click Create credentials -> OAuth client ID.', 'Choose Web application.', 'Set Name to Multigent.', 'Add your Multigent web origin under Authorized JavaScript origins.', 'Paste the Multigent Redirect URI under Authorized redirect URIs.', 'Create the client.', 'Copy Client ID and Client Secret into the matching Google provider row in Multigent.'],
        notes: ['Gmail and sensitive scopes may require Google verification in production.'],
      },
      {
        title: 'Jira / Atlassian',
        steps: ['Open https://developer.atlassian.com/console/myapps/.', 'Create an app.', 'Enable OAuth 2.0.', 'Add the Jira API permissions your workflow needs.', 'Add the Multigent Redirect URI under Authorization callback URL.', 'Copy Client ID and Client Secret into Multigent.'],
      },
      {
        title: 'Figma',
        steps: ['Open https://www.figma.com/developers/apps.', 'Create a new app.', 'Set App name to Multigent.', 'Paste the Multigent Redirect URI into the OAuth redirect or callback URL field.', 'Copy Client ID and Client Secret into Multigent.'],
      },
      {
        title: 'Feishu / Lark',
        steps: ['Feishu uses https://open.feishu.cn/app.', 'Lark uses https://open.larksuite.com/app.', 'Today Multigent configures Feishu/Lark with App ID and App Secret in External Tools, not through this OAuth app table.'],
      },
    ],
    troubleshootingTitle: 'Troubleshooting',
    troubleshooting: ['OAuth is hidden from users until Client ID and Client Secret are saved here.', 'redirect_uri_mismatch means the provider callback URL does not exactly match the Multigent Redirect URI.', 'If permissions are insufficient, add the missing scopes and ask users to authorize again.'],
  }
}

function OAuthSetupGuideDialog({ lang, onClose }: { lang: string; onClose: () => void }) {
  const { t } = useTranslation()
  const guide = oauthSetupGuideContent(lang)
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/45 p-4" onClick={onClose}>
      <div className="max-h-[88vh] w-full max-w-4xl overflow-y-auto rounded-xl border border-neutral-200 bg-white shadow-xl dark:border-zinc-700 dark:bg-zinc-900" onClick={e => e.stopPropagation()}>
        <div className="sticky top-0 z-10 flex items-center justify-between border-b border-neutral-200 bg-white px-5 py-3 dark:border-zinc-700 dark:bg-zinc-900">
          <h2 className="text-base font-semibold text-neutral-900 dark:text-zinc-100">{guide.title}</h2>
          <button type="button" onClick={onClose} className="rounded-md p-1 text-neutral-400 hover:bg-neutral-100 dark:hover:bg-zinc-800">
            <X className="size-4" />
          </button>
        </div>
        <div className="space-y-6 p-5">
          <p className="text-sm leading-6 text-neutral-600 dark:text-zinc-300">{guide.intro}</p>
          <GuideBlock title={guide.beforeTitle} steps={guide.beforeSteps} />
          {guide.sections.map(section => (
            <GuideBlock key={section.title} {...section} />
          ))}
          <GuideBlock title={guide.troubleshootingTitle} steps={guide.troubleshooting} />
          <div className="flex justify-end border-t border-neutral-100 pt-4 dark:border-zinc-800">
            <button type="button" onClick={onClose} className="rounded-lg border border-neutral-300 px-3 py-2 text-sm dark:border-zinc-600">
              {t('common.close')}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

function GuideBlock({ title, intro, steps, notes }: OAuthGuideSection) {
  return (
    <section className="rounded-lg border border-neutral-200/80 bg-neutral-50/40 p-4 dark:border-zinc-700/60 dark:bg-zinc-800/30">
      <h3 className="text-sm font-semibold text-neutral-900 dark:text-zinc-100">{title}</h3>
      {intro && <p className="mt-2 text-sm leading-6 text-neutral-500 dark:text-zinc-400">{intro}</p>}
      <ol className="mt-3 list-decimal space-y-1.5 pl-5 text-sm leading-6 text-neutral-600 dark:text-zinc-300">
        {steps.map((step, idx) => <li key={`${title}-${idx}`}>{step}</li>)}
      </ol>
      {notes && notes.length > 0 && (
        <div className="mt-3 rounded-md bg-white px-3 py-2 dark:bg-zinc-900/70">
          <ul className="list-disc space-y-1 pl-4 text-xs leading-5 text-neutral-500 dark:text-zinc-400">
            {notes.map((note, idx) => <li key={`${title}-note-${idx}`}>{note}</li>)}
          </ul>
        </div>
      )}
    </section>
  )
}

function OAuthClientConfigDialog({ config, onClose, onSaved }: { config: OAuthClientConfig; onClose: () => void; onSaved: () => void }) {
  const { t } = useTranslation()
  const [clientId, setClientId] = useState(config.clientId ?? '')
  const [clientSecret, setClientSecret] = useState('')
  const [saving, setSaving] = useState(false)

  async function submit() {
    setSaving(true)
    try {
      await apiPut(`/api/v1/oauth/client-configs/${encodeURIComponent(config.provider)}`, {
        clientId: clientId.trim(),
        clientSecret: clientSecret.trim(),
        extra: config.extra ?? {},
      })
      onSaved()
    } finally {
      setSaving(false)
    }
  }

  async function remove() {
    const ok = await confirmDialog({
      title: t('settings.removeOAuthClient'),
      description: t('settings.removeOAuthClientConfirm', { name: config.displayName }),
      confirmLabel: t('common.delete'),
      cancelLabel: t('common.cancel'),
    })
    if (!ok) return
    await apiDelete(`/api/v1/oauth/client-configs/${encodeURIComponent(config.provider)}`)
    onSaved()
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/45 p-4" onClick={onClose}>
      <div className="max-h-[88vh] w-full max-w-2xl overflow-y-auto rounded-xl border border-neutral-200 bg-white shadow-xl dark:border-zinc-700 dark:bg-zinc-900" onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between border-b border-neutral-200 px-5 py-3 dark:border-zinc-700">
          <h2 className="text-base font-semibold text-neutral-900 dark:text-zinc-100">{t('settings.oauthClientTitle', { name: config.displayName })}</h2>
          <button type="button" onClick={onClose} className="rounded-md p-1 text-neutral-400 hover:bg-neutral-100 dark:hover:bg-zinc-800">
            <X className="size-4" />
          </button>
        </div>
        <div className="space-y-4 p-5">
          <label className="block">
            <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">Client ID</span>
            <input className={inputCls} value={clientId} onChange={e => setClientId(e.target.value)} />
          </label>
          <label className="block">
            <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">Client secret</span>
            <input className={inputCls} type="password" value={clientSecret} onChange={e => setClientSecret(e.target.value)} placeholder={config.configured ? t('settings.keepCurrentSecret') : ''} />
          </label>
          <div className="rounded-lg bg-neutral-50 px-3 py-2 dark:bg-zinc-800/50">
            <p className="text-xs font-medium text-neutral-500 dark:text-zinc-400">Redirect URI</p>
            <p className="mt-1 break-all font-mono text-xs text-neutral-600 dark:text-zinc-300">{config.expectedRedirectUri}</p>
          </div>
          <div className="flex justify-between gap-2 pt-2">
            <button type="button" onClick={() => void remove()} disabled={saving || !config.configured} className="rounded-lg border border-red-200 px-3 py-2 text-sm text-red-600 disabled:opacity-50 dark:border-red-900/60 dark:text-red-300">{t('common.delete')}</button>
            <div className="flex gap-2">
              <button type="button" onClick={onClose} disabled={saving} className="rounded-lg border border-neutral-300 px-3 py-2 text-sm dark:border-zinc-600">{t('common.cancel')}</button>
              <button type="button" onClick={() => void submit()} disabled={saving || clientId.trim() === '' || (!config.configured && clientSecret.trim() === '')} className="rounded-lg bg-sky-600 px-3 py-2 text-sm font-medium text-white disabled:opacity-50">{t('common.save')}</button>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

export function UsersSection() {
  const { t } = useTranslation()
  const [users, setUsers] = useState<UserRow[]>([])
  const [projects, setProjects] = useState<ProjectItem[]>([])
  const [loading, setLoading] = useState(true)
  const [editing, setEditing] = useState<{
    isNew: boolean; username: string; displayName: string; role: string
    email: string; avatar: string; phone: string; bio: string
    password: string; disabled: boolean
    projects: { project: string; role: string }[]
    linkedAgents: string[]
  } | null>(null)
  const [saving, setSaving] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const [showPwd, setShowPwd] = useState(false)

  const refresh = useCallback(async () => {
    try {
      const [u, p] = await Promise.all([
        apiFetch<UserRow[]>('/api/v1/users'),
        apiFetch<ProjectItem[]>('/api/v1/projects'),
      ])
      setUsers(u ?? [])
      setProjects(p ?? [])
    } catch { /* ignore */ }
    finally { setLoading(false) }
  }, [])

  useEffect(() => { void refresh() }, [refresh])

  function openNew() {
    setEditing({ isNew: true, username: '', displayName: '', role: 'member', email: '', avatar: '', phone: '', bio: '', password: '', disabled: false, projects: [], linkedAgents: [] })
    setShowPwd(false); setErr(null)
  }

  function openEdit(u: UserRow) {
    setEditing({
      isNew: false, username: u.username, displayName: u.displayName ?? '',
      email: u.email ?? '', avatar: u.avatar ?? '', phone: u.phone ?? '', bio: u.bio ?? '',
      role: u.role, password: '', disabled: u.disabled ?? false,
      projects: u.projects ?? [], linkedAgents: u.linkedAgents ?? [],
    })
    setShowPwd(false); setErr(null)
  }

  async function handleSave() {
    if (!editing) return
    setSaving(true); setErr(null)
    try {
      if (editing.isNew) {
        if (!editing.username.trim() || !editing.password) {
          setErr(t('users.usernamePasswordRequired')); setSaving(false); return
        }
        await apiPost('/api/v1/users', {
          username: editing.username.trim(), password: editing.password,
          role: editing.role, displayName: editing.displayName,
          email: editing.email, avatar: editing.avatar, phone: editing.phone, bio: editing.bio,
        })
        if (editing.projects.length || editing.linkedAgents.length) {
          await apiPut(`/api/v1/users/${encodeURIComponent(editing.username.trim())}`, {
            projects: editing.projects, linkedAgents: editing.linkedAgents,
          })
        }
      } else {
        const body: Record<string, unknown> = {
          role: editing.role, displayName: editing.displayName,
          email: editing.email, avatar: editing.avatar, phone: editing.phone, bio: editing.bio,
          disabled: editing.disabled, projects: editing.projects,
          linkedAgents: editing.linkedAgents,
        }
        if (editing.password) body.password = editing.password
        await apiPut(`/api/v1/users/${encodeURIComponent(editing.username)}`, body)
      }
      setEditing(null)
      await refresh()
    } catch (e) { setErr(e instanceof Error ? e.message : String(e)) }
    finally { setSaving(false) }
  }

  async function handleDelete(username: string) {
    const ok = await confirmDialog({
      title: t('common.delete'),
      description: t('users.confirmDelete', { username }),
      confirmLabel: t('common.delete'),
      cancelLabel: t('common.cancel'),
    })
    if (!ok) return
    try { await apiDelete(`/api/v1/users/${encodeURIComponent(username)}`); await refresh() }
    catch { /* ignore */ }
  }

  function addProjectAccess() {
    if (!editing) return
    const available = projects.filter(p => !editing.projects.some(ep => ep.project === p.name))
    if (!available.length) return
    setEditing({ ...editing, projects: [...editing.projects, { project: available[0].name, role: 'operator' }] })
  }

  function removeProjectAccess(idx: number) {
    if (!editing) return
    setEditing({ ...editing, projects: editing.projects.filter((_, i) => i !== idx) })
  }

  function addLinkedAgent() {
    if (!editing) return
    setEditing({ ...editing, linkedAgents: [...editing.linkedAgents, ''] })
  }

  function removeLinkedAgent(idx: number) {
    if (!editing) return
    setEditing({ ...editing, linkedAgents: editing.linkedAgents.filter((_, i) => i !== idx) })
  }

  const fieldCls = 'w-full rounded-md border border-neutral-200/80 bg-neutral-50/50 px-3 py-2 text-sm outline-none transition-colors focus:border-sky-400 dark:border-zinc-700/60 dark:bg-zinc-800/50 dark:text-zinc-200 dark:[color-scheme:dark]'
  const roleIcon = (role: string) => role === 'admin'
    ? <ShieldCheck className="size-3.5 text-amber-500" />
    : <Shield className="size-3.5 text-sky-500" />

  return (
    <section className="rounded-xl border border-neutral-200/80 bg-white p-5 dark:border-zinc-700/60 dark:bg-zinc-900/40">
      <div className="flex items-center justify-between pb-3">
        <div className="flex items-center gap-2">
          <Users className="size-4 text-neutral-500 dark:text-zinc-500" strokeWidth={1.8} />
          <h3 className="text-base font-semibold text-neutral-900 dark:text-zinc-100">{t('users.title')}</h3>
        </div>
        <button type="button" onClick={openNew}
          className="flex items-center gap-1 rounded-lg bg-sky-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-sky-700">
          <UserPlus className="size-3.5" /> {t('users.add')}
        </button>
      </div>
      <p className="mb-3 text-xs text-neutral-400 dark:text-zinc-500">{t('users.desc')}</p>

      {loading ? (
        <p className="py-4 text-center text-sm text-neutral-400">{t('forms.loading')}</p>
      ) : users.length === 0 ? (
        <p className="py-4 text-center text-sm text-neutral-400 dark:text-zinc-500">{t('users.empty')}</p>
      ) : (
        <div className="space-y-2">
          {users.map(u => (
            <div key={u.username} className={cn(
              'flex items-center justify-between rounded-lg border px-4 py-2.5',
              u.disabled
                ? 'border-neutral-200/50 bg-neutral-100/30 opacity-60 dark:border-zinc-700/40 dark:bg-zinc-800/20'
                : 'border-neutral-200/80 bg-neutral-50/30 dark:border-zinc-700/60 dark:bg-zinc-800/30'
            )}>
              <div className="flex items-center gap-3">
                {roleIcon(u.role)}
                <div className="flex flex-col">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium text-neutral-800 dark:text-zinc-200">{u.username}</span>
                    {u.displayName && <span className="text-xs text-neutral-400 dark:text-zinc-500">({u.displayName})</span>}
                    {u.disabled && <span className="rounded bg-red-100 px-1.5 py-0.5 text-[10px] font-medium text-red-600 dark:bg-red-900/30 dark:text-red-400">{t('users.disabled')}</span>}
                  </div>
                  <span className="text-xs text-neutral-400 dark:text-zinc-500">
                    {u.role}
                    {u.projects && u.projects.length > 0 && ` · ${u.projects.length} ${t('users.projectCount')}`}
                    {u.linkedAgents && u.linkedAgents.length > 0 && ` · ${u.linkedAgents.join(', ')}`}
                  </span>
                </div>
              </div>
              <div className="flex gap-1">
                <button type="button" onClick={() => openEdit(u)}
                  className="rounded p-1 text-neutral-400 hover:bg-neutral-100 hover:text-neutral-600 dark:hover:bg-zinc-800 dark:hover:text-zinc-300">
                  <Pencil className="size-3.5" />
                </button>
                {u.username !== 'admin' && (
                  <button type="button" onClick={() => void handleDelete(u.username)}
                    className="rounded p-1 text-neutral-400 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400">
                    <Trash2 className="size-3.5" />
                  </button>
                )}
              </div>
            </div>
          ))}
        </div>
      )}

      {editing && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/45 p-4" onClick={() => !saving && setEditing(null)}>
          <div className="max-h-[85vh] w-full max-w-lg overflow-y-auto rounded-xl border border-neutral-200 bg-white shadow-lg dark:border-zinc-700 dark:bg-zinc-900 animate-scale-in" onClick={e => e.stopPropagation()}>
            <div className="flex items-center justify-between border-b border-neutral-200 px-5 py-3 dark:border-zinc-700">
              <h2 className="text-base font-semibold text-neutral-900 dark:text-zinc-100">
                {editing.isNew ? t('users.add') : t('users.edit')}
              </h2>
              <button type="button" onClick={() => setEditing(null)} className="rounded-md p-1 text-neutral-400 hover:bg-neutral-100 dark:text-zinc-500 dark:hover:bg-zinc-800">
                <X className="size-4" />
              </button>
            </div>
            <div className="space-y-3 px-5 py-4">
              {/* Username */}
              <label className="flex flex-col gap-1">
                <span className="text-sm font-medium text-neutral-600 dark:text-zinc-400">{t('auth.username')}</span>
                <input value={editing.username} onChange={e => editing.isNew && setEditing({ ...editing, username: e.target.value })}
                  disabled={!editing.isNew} className={cn(fieldCls, !editing.isNew && 'opacity-60')} />
              </label>

              {/* Display Name */}
              <label className="flex flex-col gap-1">
                <span className="text-sm font-medium text-neutral-600 dark:text-zinc-400">{t('users.displayName')}</span>
                <input value={editing.displayName} onChange={e => setEditing({ ...editing, displayName: e.target.value })} className={fieldCls} />
              </label>

              {/* Password */}
              <label className="flex flex-col gap-1">
                <span className="text-sm font-medium text-neutral-600 dark:text-zinc-400">
                  {t('auth.password')}{!editing.isNew && <span className="ml-1 text-xs text-neutral-400">({t('users.passwordOptional')})</span>}
                </span>
                <div className="flex items-center gap-2">
                  <input type={showPwd ? 'text' : 'password'} value={editing.password}
                    onChange={e => setEditing({ ...editing, password: e.target.value })}
                    className={cn(fieldCls, 'flex-1')} placeholder={editing.isNew ? t('auth.pwdMinHint') : t('users.passwordUnchanged')} />
                  <button type="button" onClick={() => setShowPwd(!showPwd)} className="rounded p-1.5 text-neutral-400 hover:text-neutral-600 dark:hover:text-zinc-300">
                    {showPwd ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                  </button>
                </div>
              </label>

              {/* Email */}
              <label className="flex flex-col gap-1">
                <span className="text-sm font-medium text-neutral-600 dark:text-zinc-400">{t('users.email')}</span>
                <input type="email" value={editing.email} onChange={e => setEditing({ ...editing, email: e.target.value })} className={fieldCls} placeholder="alice@example.com" />
              </label>

              {/* Phone */}
              <label className="flex flex-col gap-1">
                <span className="text-sm font-medium text-neutral-600 dark:text-zinc-400">{t('users.phone')}</span>
                <input value={editing.phone} onChange={e => setEditing({ ...editing, phone: e.target.value })} className={fieldCls} placeholder="+86 138..." />
              </label>

              {/* Avatar URL */}
              <label className="flex flex-col gap-1">
                <span className="text-sm font-medium text-neutral-600 dark:text-zinc-400">{t('users.avatar')}</span>
                <input value={editing.avatar} onChange={e => setEditing({ ...editing, avatar: e.target.value })} className={fieldCls} placeholder="https://..." />
              </label>

              {/* Bio */}
              <label className="flex flex-col gap-1">
                <span className="text-sm font-medium text-neutral-600 dark:text-zinc-400">{t('users.bio')}</span>
                <textarea value={editing.bio} onChange={e => setEditing({ ...editing, bio: e.target.value })} rows={2} className={cn(fieldCls, 'resize-none')} />
              </label>

              {/* Role */}
              <label className="flex flex-col gap-1">
                <span className="text-sm font-medium text-neutral-600 dark:text-zinc-400">{t('users.role')}</span>
                <select value={editing.role} onChange={e => setEditing({ ...editing, role: e.target.value })} className={fieldCls}>
                  {USER_ROLES.map(r => <option key={r} value={r}>{t(`users.role_${r}`)}</option>)}
                </select>
              </label>

              {/* Disabled */}
              {!editing.isNew && (
                <label className="flex items-center gap-2 py-1">
                  <input type="checkbox" checked={editing.disabled} onChange={e => setEditing({ ...editing, disabled: e.target.checked })}
                    className="size-4 rounded border-neutral-300 text-sky-600 dark:border-zinc-600" />
                  <span className="text-sm text-neutral-600 dark:text-zinc-400">{t('users.disableAccount')}</span>
                </label>
              )}

              {/* Project Access */}
              {editing.role === 'member' && (
                <div className="rounded-lg border border-neutral-200/80 p-3 dark:border-zinc-700/60">
                  <div className="flex items-center justify-between pb-2">
                    <span className="text-sm font-medium text-neutral-700 dark:text-zinc-300">{t('users.projectAccess')}</span>
                    <button type="button" onClick={addProjectAccess}
                      className="flex items-center gap-1 rounded bg-neutral-100 px-2 py-1 text-xs text-neutral-600 hover:bg-neutral-200 dark:bg-zinc-800 dark:text-zinc-400 dark:hover:bg-zinc-700">
                      <Plus className="size-3" /> {t('users.addProject')}
                    </button>
                  </div>
                  {editing.projects.length === 0 ? (
                    <p className="py-2 text-center text-xs text-neutral-400 dark:text-zinc-500">{t('users.noProjectAccess')}</p>
                  ) : (
                    <div className="space-y-1.5">
                      {editing.projects.map((pa, idx) => (
                        <div key={idx} className="flex items-center gap-2">
                          <select value={pa.project} onChange={e => {
                            const np = [...editing.projects]; np[idx] = { ...np[idx], project: e.target.value }
                            setEditing({ ...editing, projects: np })
                          }} className={cn(fieldCls, 'flex-1 text-xs')}>
                            {projects.map(p => <option key={p.name} value={p.name}>{p.name}</option>)}
                          </select>
                          <select value={pa.role} onChange={e => {
                            const np = [...editing.projects]; np[idx] = { ...np[idx], role: e.target.value }
                            setEditing({ ...editing, projects: np })
                          }} className={cn(fieldCls, 'w-28 text-xs')}>
                            {PROJECT_ROLES.map(r => <option key={r} value={r}>{t(`users.prole_${r}`)}</option>)}
                          </select>
                          <button type="button" onClick={() => removeProjectAccess(idx)} className="rounded p-1 text-neutral-400 hover:text-red-500">
                            <X className="size-3.5" />
                          </button>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              )}

              {/* Linked Agents */}
              <div className="rounded-lg border border-neutral-200/80 p-3 dark:border-zinc-700/60">
                <div className="flex items-center justify-between pb-2">
                  <span className="text-sm font-medium text-neutral-700 dark:text-zinc-300">{t('users.linkedAgents')}</span>
                  <button type="button" onClick={addLinkedAgent}
                    className="flex items-center gap-1 rounded bg-neutral-100 px-2 py-1 text-xs text-neutral-600 hover:bg-neutral-200 dark:bg-zinc-800 dark:text-zinc-400 dark:hover:bg-zinc-700">
                    <Plus className="size-3" /> {t('users.addAgent')}
                  </button>
                </div>
                <p className="mb-2 text-[11px] text-neutral-400 dark:text-zinc-500">{t('users.linkedAgentsHint')}</p>
                {editing.linkedAgents.length === 0 ? (
                  <p className="py-1 text-center text-xs text-neutral-400 dark:text-zinc-500">{t('users.noLinkedAgents')}</p>
                ) : (
                  <div className="space-y-1.5">
                    {editing.linkedAgents.map((agent, idx) => (
                      <div key={idx} className="flex items-center gap-2">
                        <input value={agent} onChange={e => {
                          const na = [...editing.linkedAgents]; na[idx] = e.target.value
                          setEditing({ ...editing, linkedAgents: na })
                        }} className={cn(fieldCls, 'flex-1 font-mono text-xs')} placeholder="project/agent-name" />
                        <button type="button" onClick={() => removeLinkedAgent(idx)} className="rounded p-1 text-neutral-400 hover:text-red-500">
                          <X className="size-3.5" />
                        </button>
                      </div>
                    ))}
                  </div>
                )}
              </div>

              {err && <p className="text-sm text-red-600 dark:text-red-400">{err}</p>}
              <div className="flex justify-end gap-2 pt-1">
                <button type="button" onClick={() => setEditing(null)} disabled={saving}
                  className="rounded-lg border border-neutral-300 px-3 py-1.5 text-sm dark:border-zinc-600">{t('forms.cancel')}</button>
                <button type="button" onClick={() => void handleSave()} disabled={saving || (editing.isNew && (!editing.username.trim() || !editing.password))}
                  className="rounded-lg bg-sky-600 px-3 py-1.5 text-sm font-medium text-white disabled:opacity-50">
                  {saving ? t('forms.saving') : t('forms.save')}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </section>
  )
}

type ProviderRow = {
  id: string; ownerType?: 'workspace' | 'user'; ownerId?: string; name: string; type: string; baseUrl?: string; model?: string
  hasKey: boolean; authMethod?: string; authConfigured?: boolean; env?: Record<string, string>
}
type WorkspaceAccessSummary = { currentUserCanAdmin?: boolean }

type ModelAccountMode = 'official' | 'gateway'
type ModelAccountCLI = 'codex' | 'claudecode' | 'cursor' | 'gemini'
const OFFICIAL_ACCOUNT_CLIS: ModelAccountCLI[] = ['codex', 'claudecode', 'cursor', 'gemini']
const GATEWAY_ACCOUNT_CLIS: ModelAccountCLI[] = ['codex', 'claudecode']

type ProviderPreset = {
  id: string
  label: string
  cli: ModelAccountCLI
  baseUrl: string
  model: string
  hint: string
}

const GATEWAY_ACCOUNT_PRESETS: ProviderPreset[] = [
  { id: 'codex-compatible', label: 'OpenAI Compatible', cli: 'codex', baseUrl: '', model: '', hint: 'Codex / OpenAI-compatible gateways' },
  { id: 'claude-compatible', label: 'Anthropic Compatible', cli: 'claudecode', baseUrl: '', model: '', hint: 'Claude Code-compatible gateways' },
  { id: 'deepseek-codex', label: 'DeepSeek', cli: 'codex', baseUrl: 'https://api.deepseek.com', model: 'deepseek-chat', hint: 'Codex via OpenAI-compatible API' },
  { id: 'deepseek-claude', label: 'DeepSeek', cli: 'claudecode', baseUrl: 'https://api.deepseek.com/anthropic', model: 'deepseek-chat', hint: 'Claude Code-compatible API' },
  { id: 'kimi-codex', label: 'Kimi', cli: 'codex', baseUrl: 'https://api.moonshot.cn/v1', model: 'kimi-k2-turbo-preview', hint: 'Codex via OpenAI-compatible API' },
  { id: 'kimi-coding', label: 'Kimi Coding', cli: 'codex', baseUrl: 'https://api.kimi.com/coding/v1', model: 'kimi-for-coding', hint: 'Codex coding endpoint' },
  { id: 'openrouter', label: 'OpenRouter', cli: 'codex', baseUrl: 'https://openrouter.ai/api/v1', model: '', hint: 'OpenAI-compatible model router' },
]

type ProviderDraft = Partial<ProviderRow> & { apiKey?: string; accountMode?: ModelAccountMode; cli?: ModelAccountCLI; authMethod?: string }
type ModelDeviceAuthState = {
  sessionId: string
  verificationUri: string
  userCode?: string
  requiresCode?: boolean
  cli?: ModelAccountCLI
  status: 'pending' | 'connected' | 'failed'
}
type CCSwitchProviderPreview = { id: string; name: string; cli: string; type: string; baseUrl?: string; model?: string; hasKey: boolean; isCurrent: boolean }
type CCSwitchProviderResponse = { available: boolean; dbPath?: string; providers: CCSwitchProviderPreview[]; searched?: string[]; error?: string }
type AssistantStatus = {
  enabled: boolean
  mode: string
  configured: boolean
  canUse: boolean
  canAdmin: boolean
  modelProviderId?: string
  modelProviderName?: string
  reason?: string
}

function ProvidersSection() {
  const { t } = useTranslation()
  const { user } = useAuth()
  const { canAdmin } = useWorkspaceAccess()
  const [workspace, setWorkspace] = useState<WorkspaceAccessSummary | null>(null)
  const canCreateWorkspaceProvider = workspace?.currentUserCanAdmin ?? canAdmin
  const [providers, setProviders] = useState<ProviderRow[]>([])
  const [loading, setLoading] = useState(true)
  const [editing, setEditing] = useState<ProviderDraft | null>(null)
  const [ccSwitch, setCCSwitch] = useState<CCSwitchProviderResponse | null>(null)
  const [ccSwitchLoading, setCCSwitchLoading] = useState(false)
  const [ccSwitchImporting, setCCSwitchImporting] = useState(false)
  const [ccSwitchSelected, setCCSwitchSelected] = useState<string[]>([])
  const [saving, setSaving] = useState(false)
  const [assistantStatus, setAssistantStatus] = useState<AssistantStatus | null>(null)
  const [assistantProviderId, setAssistantProviderId] = useState('')
  const [assistantSaving, setAssistantSaving] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const [showKey, setShowKey] = useState(false)
  const [deviceAuth, setDeviceAuth] = useState<ModelDeviceAuthState | null>(null)
  const [deviceAuthStarting, setDeviceAuthStarting] = useState(false)
  const [browserAuthCode, setBrowserAuthCode] = useState('')
  const deviceAuthSessionRef = useRef('')

  const refresh = useCallback(async () => {
    try {
      const [workspaceData, data, assistantData] = await Promise.all([
        apiFetch<WorkspaceAccessSummary>('/api/v1/workspace').catch(() => null),
        apiFetch<ProviderRow[]>('/api/v1/providers'),
        apiFetch<AssistantStatus>('/api/v1/assistant/status').catch(() => null),
      ])
      setWorkspace(workspaceData)
      setProviders(data ?? [])
      setAssistantStatus(assistantData)
      setAssistantProviderId(assistantData?.modelProviderId ?? '')
    } catch { /* ignore */ }
    finally { setLoading(false) }
  }, [])

  useEffect(() => { void refresh() }, [refresh])

  function openNew() {
    setEditing({ accountMode: 'official', cli: 'codex', ownerType: canCreateWorkspaceProvider ? 'workspace' : 'user', name: providerOfficialName('codex'), type: providerTypeForCLI('codex'), baseUrl: '', model: '', apiKey: '', authMethod: 'codex_chatgpt' })
    setShowKey(false)
    deviceAuthSessionRef.current = ''
    setDeviceAuth(null)
    setBrowserAuthCode('')
    setErr(null)
  }

  async function openCCSwitchImport() {
    setCCSwitch({ available: false, providers: [] })
    setCCSwitchSelected([])
    setCCSwitchLoading(true)
    setErr(null)
    try {
      const data = await apiFetch<CCSwitchProviderResponse>('/api/v1/providers/cc-switch')
      setCCSwitch(data)
      setCCSwitchSelected((data.providers ?? []).filter(p => p.isCurrent).map(p => p.id))
    } catch (e) {
      setCCSwitch({ available: false, providers: [], error: e instanceof Error ? e.message : String(e) })
    } finally {
      setCCSwitchLoading(false)
    }
  }

  function toggleCCSwitchProvider(id: string) {
    setCCSwitchSelected(prev => prev.includes(id) ? prev.filter(item => item !== id) : [...prev, id])
  }

  async function importCCSwitchProviders() {
    if (ccSwitchSelected.length === 0) return
    setCCSwitchImporting(true)
    setErr(null)
    try {
      await apiPost('/api/v1/providers/cc-switch/import', { ids: ccSwitchSelected })
      setCCSwitch(null)
      setCCSwitchSelected([])
      await refresh()
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    } finally {
      setCCSwitchImporting(false)
    }
  }

  function openEdit(p: ProviderRow) {
    setEditing({ ...p, accountMode: inferModelAccountMode(p), cli: inferModelAccountCLI(p), apiKey: '', authMethod: p.authMethod || (p.hasKey ? 'api_key' : '') })
    setShowKey(false)
    deviceAuthSessionRef.current = ''
    setDeviceAuth(null)
    setBrowserAuthCode('')
    setErr(null)
  }

  async function handleSave() {
    if (!editing || !editing.name?.trim()) return
    setSaving(true); setErr(null)
    try {
      const body: any = {
        ownerType: editing.ownerType || (canCreateWorkspaceProvider ? 'workspace' : 'user'),
        name: editing.name,
        type: providerTypeForCLI(editing.cli),
        baseUrl: editing.accountMode === 'official' ? '' : editing.baseUrl || '',
        model: editing.accountMode === 'official' ? '' : editing.model || '',
        env: {
          ...(editing.env ?? {}),
          MULTIGENT_MODEL_AUTH_METHOD: editing.authMethod || 'api_key',
          MULTIGENT_MODEL_AUTH_STATUS: editing.apiKey || editing.hasKey ? 'configured' : 'not_configured',
        },
      }
      if (editing.apiKey) body.apiKey = editing.apiKey
      if (editing.id) {
        await apiPut(`/api/v1/providers/${editing.id}`, body)
      } else {
        await apiPost('/api/v1/providers', body)
      }
      setEditing(null)
      await refresh()
    } catch (e) { setErr(e instanceof Error ? e.message : String(e)) }
    finally { setSaving(false) }
  }

  async function handleDelete(id: string) {
    const confirmed = await confirmDialog({
      title: t('provider.removeTitle'),
      description: t('provider.removeDescription'),
      confirmLabel: t('forms.delete'),
      cancelLabel: t('forms.cancel'),
      tone: 'danger',
    })
    if (!confirmed) return
    try {
      await apiDelete(`/api/v1/providers/${id}`)
      await refresh()
    } catch { /* ignore */ }
  }

  async function saveAssistantSettings() {
    setAssistantSaving(true)
    setErr(null)
    try {
      const data = await apiPut<AssistantStatus>('/api/v1/assistant/settings', {
        enabled: true,
        modelProviderId: assistantProviderId,
      })
      setAssistantStatus(data)
      setAssistantProviderId(data.modelProviderId ?? '')
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    } finally {
      setAssistantSaving(false)
    }
  }

  async function startCodexChatGPTAuth() {
    if (!editing?.name?.trim()) return
    const cli = editing.cli ?? 'codex'
    const previousSessionID = deviceAuthSessionRef.current
    deviceAuthSessionRef.current = ''
    if (previousSessionID) {
      void apiDelete(`/api/v1/providers/auth/sessions/${encodeURIComponent(previousSessionID)}`).catch(() => {})
    }
    setDeviceAuthStarting(true)
    setErr(null)
    try {
      const path = cli === 'codex' ? '/api/v1/providers/auth/codex/device/begin' : `/api/v1/providers/auth/${encodeURIComponent(cli)}/browser/begin`
      const res = await apiPost<{ sessionId: string; verificationUri: string; userCode?: string; status: 'pending'; expiresIn?: number; requiresCode?: boolean }>(path, {
        ownerType: editing.ownerType || (canCreateWorkspaceProvider ? 'workspace' : 'user'),
        name: editing.name,
      })
      const next: ModelDeviceAuthState = {
        sessionId: res.sessionId,
        verificationUri: res.verificationUri,
        userCode: res.userCode,
        requiresCode: res.requiresCode,
        cli,
        status: 'pending',
      }
      setDeviceAuth(next)
      setBrowserAuthCode('')
      deviceAuthSessionRef.current = next.sessionId
      pollCLINativeAuth(cli, next.sessionId)
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    } finally {
      setDeviceAuthStarting(false)
    }
  }

  function cancelDeviceAuth() {
    const sessionID = deviceAuthSessionRef.current
    deviceAuthSessionRef.current = ''
    if (sessionID) {
      void apiDelete(`/api/v1/providers/auth/sessions/${encodeURIComponent(sessionID)}`).catch(() => {})
    }
    setDeviceAuth(null)
    setBrowserAuthCode('')
  }

  function pollCLINativeAuth(cli: ModelAccountCLI, sessionId: string) {
    window.setTimeout(async () => {
      if (deviceAuthSessionRef.current !== sessionId) return
      try {
        const path = cli === 'codex' ? '/api/v1/providers/auth/codex/device/poll' : `/api/v1/providers/auth/${encodeURIComponent(cli)}/browser/poll`
        const res = await apiPost<{ status: 'pending' | 'connected'; provider?: ProviderRow; requiresCode?: boolean }>(path, { sessionId })
        if (res.status === 'connected') {
          setDeviceAuth(prev => prev && prev.sessionId === sessionId ? { ...prev, status: 'connected' } : prev)
          deviceAuthSessionRef.current = ''
          setEditing(null)
          await refresh()
          return
        }
        pollCLINativeAuth(cli, sessionId)
      } catch (e) {
        setDeviceAuth(prev => prev && prev.sessionId === sessionId ? { ...prev, status: 'failed' } : prev)
        deviceAuthSessionRef.current = ''
        setErr(e instanceof Error ? e.message : String(e))
      }
    }, 2000)
  }

  function closeProviderDialog() {
    if (saving || deviceAuthStarting) return
    const sessionID = deviceAuthSessionRef.current
    deviceAuthSessionRef.current = ''
    if (sessionID) {
      void apiDelete(`/api/v1/providers/auth/sessions/${encodeURIComponent(sessionID)}`).catch(() => {})
    }
    setDeviceAuth(null)
    setBrowserAuthCode('')
    setEditing(null)
  }

  async function submitBrowserAuthCode() {
    if (!deviceAuth?.sessionId || !deviceAuth.cli || !browserAuthCode.trim()) return
    setErr(null)
    try {
      await apiPost(`/api/v1/providers/auth/${encodeURIComponent(deviceAuth.cli)}/browser/code`, {
        sessionId: deviceAuth.sessionId,
        code: browserAuthCode,
      })
      setBrowserAuthCode('')
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    }
  }

  function applyPreset(presetID: string) {
    const preset = GATEWAY_ACCOUNT_PRESETS.find(p => p.id === presetID)
    if (!preset) return
    setEditing(prev => ({
      ...prev,
      name: prev?.name?.trim() && !isGeneratedModelAccountName(prev.name) ? prev.name : preset.label,
      accountMode: 'gateway',
      cli: preset.cli,
      type: providerTypeForCLI(preset.cli),
      baseUrl: preset.baseUrl,
      model: preset.model,
    }))
  }

  function setAccountMode(mode: ModelAccountMode) {
    deviceAuthSessionRef.current = ''
    setDeviceAuth(null)
    setEditing(prev => {
      if (!prev) return prev
      if (mode === 'official') {
        const nextCLI = isOfficialCLI(prev.cli) ? prev.cli : 'codex'
        return {
          ...prev,
          accountMode: 'official',
          cli: nextCLI,
          type: providerTypeForCLI(nextCLI),
          baseUrl: '',
          model: '',
          authMethod: defaultAuthMethodForCLI(nextCLI),
          name: prev.name?.trim() && !isGeneratedModelAccountName(prev.name) ? prev.name : providerOfficialName(nextCLI),
        }
      }
      const nextCLI = isGatewayCLI(prev.cli) ? prev.cli : 'codex'
      return {
        ...prev,
        accountMode: 'gateway',
        cli: nextCLI,
        type: providerTypeForCLI(nextCLI),
        authMethod: 'api_key',
        name: prev.name?.trim() && !isGeneratedModelAccountName(prev.name) ? prev.name : '',
      }
    })
  }

  function setProviderCLI(cli: ModelAccountCLI) {
    deviceAuthSessionRef.current = ''
    setDeviceAuth(null)
    setEditing(prev => {
      if (!prev) return prev
      return {
        ...prev,
        cli,
        type: providerTypeForCLI(cli),
        authMethod: prev.accountMode === 'official' ? defaultAuthMethodForCLI(cli) : 'api_key',
        name: prev.name?.trim() && !isGeneratedModelAccountName(prev.name) ? prev.name : (prev.accountMode === 'official' ? providerOfficialName(cli) : ''),
      }
    })
  }

  const fieldCls = 'w-full rounded-md border border-neutral-200/80 bg-neutral-50/50 px-3 py-2 text-sm outline-none transition-colors focus:border-sky-400 dark:border-zinc-700/60 dark:bg-zinc-800/50 dark:text-zinc-200 dark:[color-scheme:dark]'
  const assistantProviderOptions = providers.filter(p => (p.ownerType ?? 'workspace') === 'workspace' && (p.authConfigured || p.hasKey))

  return (
    <>
      <section id="model-accounts" data-tour-provider-section className="scroll-mt-6 rounded-xl border border-neutral-200/80 bg-white p-5 dark:border-zinc-700/60 dark:bg-zinc-900/40">
        <div className="flex items-center justify-between pb-3">
          <div className="flex items-center gap-2">
            <Server className="size-4 text-neutral-500 dark:text-zinc-500" strokeWidth={1.8} />
            <h3 className="text-base font-semibold text-neutral-900 dark:text-zinc-100">{t('provider.title')}</h3>
          </div>
          <div className="flex items-center gap-2">
            <button type="button" onClick={() => void openCCSwitchImport()}
              className="rounded-lg border border-sky-600 bg-white px-3 py-1.5 text-xs font-medium text-sky-700 hover:bg-sky-50 dark:border-sky-500 dark:bg-zinc-900 dark:text-sky-400 dark:hover:bg-zinc-800">
              {t('provider.importCCSwitch')}
            </button>
            <button type="button" data-tour-provider-add onClick={openNew}
              className="rounded-lg border border-sky-600 bg-white px-3 py-1.5 text-xs font-medium text-sky-700 hover:bg-sky-50 dark:border-sky-500 dark:bg-zinc-900 dark:text-sky-400 dark:hover:bg-zinc-800">
              {t('provider.add')}
            </button>
          </div>
        </div>
        <p className="mb-3 text-xs text-neutral-400 dark:text-zinc-500">{t('provider.desc')}</p>

        {loading ? (
          <p className="py-4 text-center text-sm text-neutral-400">{t('forms.loading')}</p>
        ) : providers.length === 0 && !editing ? (
          <p className="py-4 text-center text-sm text-neutral-400 dark:text-zinc-500">{t('provider.empty')}</p>
        ) : (
          <div className="space-y-2">
            {providers.map(p => (
              <div key={p.id} className="flex items-center justify-between rounded-lg border border-neutral-200/80 bg-neutral-50/30 px-4 py-2.5 dark:border-zinc-700/60 dark:bg-zinc-800/30">
                <div className="flex flex-col">
                  <span className="text-sm font-medium text-neutral-800 dark:text-zinc-200">{p.name}</span>
                  <span className="text-xs text-neutral-400 dark:text-zinc-500">
                    {inferModelAccountMode(p) === 'official' ? t('provider.officialBadge') : t('provider.gatewayBadge')}
                    {' · '}{providerCLILabel(inferModelAccountCLI(p))}
                    {p.model ? ` · ${p.model}` : ''}{p.baseUrl ? ` · ${p.baseUrl}` : ''}{(p.authConfigured || p.hasKey) ? ` · ${providerAuthConfiguredLabel(p, t)}` : ''}
                  </span>
                </div>
                <div className="flex items-center gap-2">
                  <span className={cn('rounded-full px-2 py-0.5 text-[10px] font-medium',
                    p.ownerType === 'user'
                      ? 'bg-violet-50 text-violet-700 dark:bg-violet-900/20 dark:text-violet-300'
                      : 'bg-sky-50 text-sky-700 dark:bg-sky-900/20 dark:text-sky-300'
                  )}>
                    {p.ownerType === 'user' ? t('provider.scopePersonal') : t('provider.scopeWorkspace')}
                  </span>
                  {canManageProviderRow(p, canCreateWorkspaceProvider, user?.username) && (
                    <>
                      <button type="button" onClick={() => openEdit(p)}
                        className="rounded p-1 text-neutral-400 hover:bg-neutral-100 hover:text-neutral-600 dark:hover:bg-zinc-800 dark:hover:text-zinc-300">
                        <Pencil className="size-3.5" />
                      </button>
                      <button type="button" onClick={() => void handleDelete(p.id)}
                        className="rounded p-1 text-neutral-400 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400">
                        <Trash2 className="size-3.5" />
                      </button>
                    </>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}

      {editing && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/45 p-4" onClick={closeProviderDialog}>
          <div className="w-full max-w-md rounded-xl border border-neutral-200 bg-white shadow-lg dark:border-zinc-700 dark:bg-zinc-900 animate-scale-in" onClick={e => e.stopPropagation()}>
            <div className="flex items-center justify-between border-b border-neutral-200 px-5 py-3 dark:border-zinc-700">
              <h2 className="text-base font-semibold text-neutral-900 dark:text-zinc-100">
                {editing.id ? t('provider.edit') : t('provider.add')}
              </h2>
              <button type="button" onClick={closeProviderDialog} className="rounded-md p-1 text-neutral-400 hover:bg-neutral-100 dark:text-zinc-500 dark:hover:bg-zinc-800"><X className="size-4" /></button>
            </div>
            <div className="space-y-3 px-5 py-4">
              {!editing.id && (
                <div className="grid grid-cols-2 gap-2 rounded-lg bg-neutral-100 p-1 dark:bg-zinc-800/70">
                  <button
                    type="button"
                    onClick={() => setAccountMode('official')}
                    className={cn('rounded-md px-3 py-2 text-sm font-medium transition-colors',
                      (editing.accountMode ?? 'official') === 'official'
                        ? 'bg-white text-neutral-900 shadow-sm dark:bg-zinc-950 dark:text-zinc-100'
                        : 'text-neutral-500 hover:text-neutral-800 dark:text-zinc-400 dark:hover:text-zinc-200'
                    )}
                  >
                    {t('provider.modeOfficial')}
                  </button>
                  <button
                    type="button"
                    onClick={() => setAccountMode('gateway')}
                    className={cn('rounded-md px-3 py-2 text-sm font-medium transition-colors',
                      editing.accountMode === 'gateway'
                        ? 'bg-white text-neutral-900 shadow-sm dark:bg-zinc-950 dark:text-zinc-100'
                        : 'text-neutral-500 hover:text-neutral-800 dark:text-zinc-400 dark:hover:text-zinc-200'
                    )}
                  >
                    {t('provider.modeGateway')}
                  </button>
                </div>
              )}
              {!editing.id && (
                <label className="flex flex-col gap-1">
                  <span className="text-sm font-medium text-neutral-600 dark:text-zinc-400">{t('provider.cliLabel')}</span>
                  <select value={editing.cli ?? 'codex'} onChange={e => setProviderCLI(e.target.value as ModelAccountCLI)} className={fieldCls}>
                    {((editing.accountMode ?? 'official') === 'official' ? OFFICIAL_ACCOUNT_CLIS : GATEWAY_ACCOUNT_CLIS).map(cli => (
                      <option key={cli} value={cli}>{providerCLILabel(cli)}</option>
                    ))}
                  </select>
                </label>
              )}
              {!editing.id && !canCreateWorkspaceProvider && (
                <p className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-300">
                  {t('provider.personalFallbackHint')}
                </p>
              )}
              {editing.id && (
                <div className="flex items-center justify-between rounded-lg border border-neutral-200/80 bg-neutral-50/50 px-3 py-2 dark:border-zinc-700/60 dark:bg-zinc-800/40">
                  <span className="text-sm font-medium text-neutral-600 dark:text-zinc-400">{t('provider.scopeLabel')}</span>
                  <span className="text-xs text-neutral-500 dark:text-zinc-400">
                    {editing.ownerType === 'user' ? t('provider.scopePersonal') : t('provider.scopeWorkspace')}
                  </span>
                </div>
              )}
              <label className="flex flex-col gap-1">
                <span className="text-sm font-medium text-neutral-600 dark:text-zinc-400">{t('provider.nameLabel')}</span>
                <input value={editing.name ?? ''} onChange={e => setEditing({ ...editing, name: e.target.value })} className={fieldCls} placeholder={t('provider.namePlaceholder')} />
              </label>
              {(editing.accountMode ?? 'official') === 'official' ? (
                <div className="space-y-2">
                  {supportsBrowserModelAuth(editing.cli) && !editing.id && (
                    <div className="grid grid-cols-2 gap-2 rounded-lg bg-neutral-100 p-1 dark:bg-zinc-800/70">
                      <button
                        type="button"
                        onClick={() => setEditing({ ...editing, authMethod: defaultAuthMethodForCLI(editing.cli), apiKey: '' })}
                        className={cn('rounded-md px-3 py-2 text-sm font-medium transition-colors',
                          (editing.authMethod ?? defaultAuthMethodForCLI(editing.cli)) === defaultAuthMethodForCLI(editing.cli)
                            ? 'bg-white text-neutral-900 shadow-sm dark:bg-zinc-950 dark:text-zinc-100'
                            : 'text-neutral-500 hover:text-neutral-800 dark:text-zinc-400 dark:hover:text-zinc-200'
                        )}
                      >
                        {editing.cli === 'codex' ? t('provider.authChatGPT') : t('provider.authBrowser')}
                      </button>
                      <button
                        type="button"
                        onClick={() => setEditing({ ...editing, authMethod: 'api_key' })}
                        className={cn('rounded-md px-3 py-2 text-sm font-medium transition-colors',
                          editing.authMethod === 'api_key'
                            ? 'bg-white text-neutral-900 shadow-sm dark:bg-zinc-950 dark:text-zinc-100'
                            : 'text-neutral-500 hover:text-neutral-800 dark:text-zinc-400 dark:hover:text-zinc-200'
                        )}
                      >
                        {t('provider.authAPIKey')}
                      </button>
                    </div>
                  )}
                  <p className="rounded-lg border border-sky-100 bg-sky-50 px-3 py-2 text-xs leading-5 text-sky-700 dark:border-sky-900/50 dark:bg-sky-900/20 dark:text-sky-300">
                    {editing.cli === 'codex' && (editing.authMethod ?? 'codex_chatgpt') === 'codex_chatgpt'
                      ? t('provider.codexChatGPTHint')
                      : supportsBrowserModelAuth(editing.cli) && (editing.authMethod ?? defaultAuthMethodForCLI(editing.cli)) === defaultAuthMethodForCLI(editing.cli)
                        ? t('provider.cliBrowserLoginHint')
                        : t('provider.officialHint')}
                  </p>
                </div>
              ) : (
                <>
                  {!editing.id && (
                    <label className="flex flex-col gap-1">
                      <span className="text-sm font-medium text-neutral-600 dark:text-zinc-400">{t('provider.presetLabel')}</span>
                      <select value="" onChange={e => applyPreset(e.target.value)} className={fieldCls}>
                        <option value="">{t('provider.presetPlaceholder')}</option>
                        {GATEWAY_ACCOUNT_PRESETS.filter(preset => preset.cli === (editing.cli ?? 'codex')).map(preset => (
                          <option key={preset.id} value={preset.id}>{preset.label} - {preset.hint}</option>
                        ))}
                      </select>
                    </label>
                  )}
                  <label className="flex flex-col gap-1">
                    <span className="text-sm font-medium text-neutral-600 dark:text-zinc-400">{t('provider.endpointLabel')}</span>
                    <input value={editing.baseUrl ?? ''} onChange={e => setEditing({ ...editing, baseUrl: e.target.value })} className={cn(fieldCls, 'font-mono text-xs')} placeholder={providerEndpointPlaceholder(editing.cli)} />
                  </label>
                  <label className="flex flex-col gap-1">
                    <span className="text-sm font-medium text-neutral-600 dark:text-zinc-400">{t('provider.defaultModelLabel')}</span>
                    <input value={editing.model ?? ''} onChange={e => setEditing({ ...editing, model: e.target.value })} className={cn(fieldCls, 'font-mono text-xs')} placeholder={providerModelPlaceholder(editing.cli)} />
                  </label>
                  <p className="text-xs leading-5 text-neutral-400 dark:text-zinc-500">{t('provider.gatewayHint')}</p>
                </>
              )}
              {((editing.accountMode ?? 'official') !== 'official' || !supportsBrowserModelAuth(editing.cli) || editing.authMethod === 'api_key' || editing.id) && (
                <>
                  <label className="flex flex-col gap-1">
                    <span className="text-sm font-medium text-neutral-600 dark:text-zinc-400">{t('provider.apiKeyLabel')}</span>
                    <div className="flex items-center gap-2">
                      <input
                        type={showKey ? 'text' : 'password'}
                        value={editing.apiKey ?? ''}
                        onChange={e => setEditing({ ...editing, apiKey: e.target.value })}
                        className={cn(fieldCls, 'flex-1 font-mono text-xs')}
                        placeholder={editing.id && editing.hasKey ? t('provider.keyUnchangedHint') : 'sk-...'}
                      />
                      <button type="button" onClick={() => setShowKey(!showKey)}
                        className="rounded p-1.5 text-neutral-400 hover:text-neutral-600 dark:hover:text-zinc-300">
                        {showKey ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                      </button>
                    </div>
                  </label>
                  <p className="text-xs leading-5 text-neutral-400 dark:text-zinc-500">{t('provider.apiKeyHint')}</p>
                </>
              )}
              {deviceAuth && (
                <div className="rounded-lg border border-sky-200 bg-sky-50 px-3 py-3 text-sm text-sky-800 dark:border-sky-900/50 dark:bg-sky-900/20 dark:text-sky-200">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <p className="font-medium">{deviceAuth.status === 'connected' ? t('provider.deviceConnected') : t('provider.deviceWaiting')}</p>
                    <div className="flex flex-wrap items-center gap-2">
                      <a href={deviceAuth.verificationUri} target="_blank" rel="noreferrer" className="rounded-lg border border-sky-600 bg-white px-3 py-1.5 text-xs font-medium text-sky-700 hover:bg-sky-50 dark:border-sky-500 dark:bg-zinc-900 dark:text-sky-300 dark:hover:bg-zinc-800">
                        {t('provider.openAuthPage')}
                      </a>
                      {deviceAuth.status !== 'connected' && (
                        <button type="button" onClick={() => void startCodexChatGPTAuth()} disabled={deviceAuthStarting} className="rounded-lg border border-sky-600 bg-white px-3 py-1.5 text-xs font-medium text-sky-700 hover:bg-sky-50 disabled:opacity-50 dark:border-sky-500 dark:bg-zinc-900 dark:text-sky-300 dark:hover:bg-zinc-800">
                          {deviceAuthStarting ? t('provider.deviceStarting') : t('provider.regenerateAuthLink')}
                        </button>
                      )}
                      {deviceAuth.status !== 'connected' && (
                        <button type="button" onClick={cancelDeviceAuth} disabled={deviceAuthStarting} className="rounded-lg border border-neutral-300 bg-white px-3 py-1.5 text-xs font-medium text-neutral-600 hover:bg-neutral-50 disabled:opacity-50 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:bg-zinc-800">
                          {t('provider.cancelAuth')}
                        </button>
                      )}
                    </div>
                  </div>
                  <p className="mt-2 text-xs leading-5 text-sky-700/80 dark:text-sky-300/80">{t('provider.authLinkHint')}</p>
                  {deviceAuth.userCode && (
                    <>
                      <p className="mt-2 text-xs text-sky-700/80 dark:text-sky-300/80">{t('provider.deviceCodeLabel')}</p>
                      <p className="mt-1 w-fit rounded-md bg-white px-3 py-1 font-mono text-lg font-semibold tracking-wide text-neutral-900 dark:bg-zinc-950 dark:text-zinc-100">{deviceAuth.userCode}</p>
                    </>
                  )}
                  {deviceAuth.requiresCode && (
                    <div className="mt-3 flex gap-2">
                      <input value={browserAuthCode} onChange={e => setBrowserAuthCode(e.target.value)} className={cn(fieldCls, 'font-mono text-xs')} placeholder={t('provider.browserCodePlaceholder')} />
                      <button type="button" onClick={() => void submitBrowserAuthCode()} disabled={!browserAuthCode.trim()} className="rounded-lg border border-sky-600 bg-white px-3 py-2 text-sm font-medium text-sky-700 hover:bg-sky-50 disabled:opacity-50 dark:border-sky-500 dark:bg-zinc-900 dark:text-sky-400 dark:hover:bg-zinc-800">{t('provider.submitCode')}</button>
                    </div>
                  )}
                </div>
              )}
              {err && <p className="text-sm text-red-600 dark:text-red-400">{err}</p>}
              <div className="flex justify-end gap-2 pt-1">
                <button type="button" onClick={closeProviderDialog} disabled={saving || deviceAuthStarting} className="rounded-lg border border-neutral-300 px-3 py-1.5 text-sm dark:border-zinc-600">{t('forms.cancel')}</button>
                {(editing.accountMode ?? 'official') === 'official' && supportsBrowserModelAuth(editing.cli) && (editing.authMethod ?? defaultAuthMethodForCLI(editing.cli)) === defaultAuthMethodForCLI(editing.cli) && !editing.id ? (
                  <button type="button" onClick={() => void startCodexChatGPTAuth()} disabled={deviceAuthStarting || !editing.name?.trim()} className="rounded-lg bg-sky-600 px-3 py-1.5 text-sm font-medium text-white disabled:opacity-50">{deviceAuthStarting ? t('provider.deviceStarting') : t('provider.startDeviceAuth')}</button>
                ) : (
                  <button type="button" onClick={() => void handleSave()} disabled={saving || !editing.name?.trim()} className="rounded-lg bg-sky-600 px-3 py-1.5 text-sm font-medium text-white disabled:opacity-50">{saving ? t('forms.saving') : t('forms.save')}</button>
                )}
              </div>
            </div>
          </div>
        </div>
      )}
      {ccSwitch && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/45 p-4" onClick={() => !ccSwitchImporting && setCCSwitch(null)}>
          <div className="w-full max-w-2xl rounded-xl border border-neutral-200 bg-white shadow-lg dark:border-zinc-700 dark:bg-zinc-900 animate-scale-in" onClick={e => e.stopPropagation()}>
            <div className="flex items-center justify-between border-b border-neutral-200 px-5 py-3 dark:border-zinc-700">
              <div>
                <h2 className="text-base font-semibold text-neutral-900 dark:text-zinc-100">{t('provider.importCCSwitchTitle')}</h2>
                <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">{t('provider.importCCSwitchHint')}</p>
              </div>
              <button type="button" onClick={() => setCCSwitch(null)} className="rounded-md p-1 text-neutral-400 hover:bg-neutral-100 dark:text-zinc-500 dark:hover:bg-zinc-800"><X className="size-4" /></button>
            </div>
            <div className="px-5 py-4">
              {ccSwitchLoading ? (
                <p className="py-8 text-center text-sm text-neutral-400">{t('forms.loading')}</p>
              ) : !ccSwitch.available ? (
                <div className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-700 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-300">
                  <p>{t('provider.ccSwitchUnavailable')}</p>
                  {ccSwitch.error && <p className="mt-1 font-mono text-xs opacity-80">{ccSwitch.error}</p>}
                </div>
              ) : ccSwitch.providers.length === 0 ? (
                <p className="py-8 text-center text-sm text-neutral-400">{t('provider.ccSwitchEmpty')}</p>
              ) : (
                <div className="overflow-hidden rounded-lg border border-neutral-200 dark:border-zinc-700">
                  <table className="min-w-full divide-y divide-neutral-200 text-sm dark:divide-zinc-700">
                    <thead className="bg-neutral-50 dark:bg-zinc-800/70">
                      <tr>
                        <th className="w-10 px-3 py-2"></th>
                        <th className="px-3 py-2 text-left font-medium text-neutral-500 dark:text-zinc-400">{t('provider.nameLabel')}</th>
                        <th className="px-3 py-2 text-left font-medium text-neutral-500 dark:text-zinc-400">{t('provider.cliLabel')}</th>
                        <th className="px-3 py-2 text-left font-medium text-neutral-500 dark:text-zinc-400">{t('provider.defaultModelLabel')}</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-neutral-100 dark:divide-zinc-800">
                      {ccSwitch.providers.map(provider => (
                        <tr key={provider.id} className="bg-white dark:bg-zinc-900/40">
                          <td className="px-3 py-2">
                            <input type="checkbox" checked={ccSwitchSelected.includes(provider.id)} onChange={() => toggleCCSwitchProvider(provider.id)} className="rounded border-neutral-300 text-sky-600 focus:ring-sky-500 dark:border-zinc-600 dark:bg-zinc-800" />
                          </td>
                          <td className="px-3 py-2">
                            <div className="flex flex-col">
                              <span className="font-medium text-neutral-800 dark:text-zinc-200">{provider.name}</span>
                              <span className="text-xs text-neutral-400 dark:text-zinc-500">{provider.isCurrent ? t('provider.ccSwitchCurrent') : ''}{provider.hasKey ? ` · ${t('provider.keyConfigured')}` : ''}</span>
                            </div>
                          </td>
                          <td className="px-3 py-2 text-neutral-500 dark:text-zinc-400">{providerCLILabel(provider.cli as ModelAccountCLI)}</td>
                          <td className="px-3 py-2">
                            <div className="flex flex-col text-xs text-neutral-500 dark:text-zinc-400">
                              <span className="font-mono">{provider.model || '-'}</span>
                              {provider.baseUrl && <span className="max-w-xs truncate font-mono text-neutral-400">{provider.baseUrl}</span>}
                            </div>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
              {err && <p className="mt-3 text-sm text-red-600 dark:text-red-400">{err}</p>}
              <div className="mt-4 flex justify-end gap-2">
                <button type="button" onClick={() => setCCSwitch(null)} disabled={ccSwitchImporting} className="rounded-lg border border-neutral-300 px-3 py-1.5 text-sm dark:border-zinc-600">{t('forms.cancel')}</button>
                <button type="button" onClick={() => void importCCSwitchProviders()} disabled={ccSwitchImporting || !ccSwitch.available || ccSwitchSelected.length === 0} className="rounded-lg bg-sky-600 px-3 py-1.5 text-sm font-medium text-white disabled:opacity-50">{ccSwitchImporting ? t('forms.saving') : t('provider.importSelected')}</button>
              </div>
            </div>
          </div>
        </div>
      )}
      </section>

      {canCreateWorkspaceProvider && (
        <section id="assistant-account" className="scroll-mt-6 rounded-xl border border-neutral-200/80 bg-white p-5 dark:border-zinc-700/60 dark:bg-zinc-900/40">
          <div className="flex items-center gap-2">
            <KeyRound className="size-4 text-neutral-500 dark:text-zinc-500" strokeWidth={1.8} />
            <h3 className="text-base font-semibold text-neutral-900 dark:text-zinc-100">{t('assistant.settingsTitle')}</h3>
          </div>
          <p className="mt-2 text-xs leading-5 text-neutral-500 dark:text-zinc-400">{t('assistant.settingsDesc')}</p>
          {assistantStatus?.canUse && assistantStatus.modelProviderName && (
            <p className="mt-2 text-xs text-sky-700 dark:text-sky-300">{t('assistant.settingsReady', { name: assistantStatus.modelProviderName })}</p>
          )}
          <div className="mt-4 flex max-w-md flex-col gap-2">
            <select
              value={assistantProviderId}
              onChange={e => setAssistantProviderId(e.target.value)}
              className={fieldCls}
              disabled={assistantSaving || assistantProviderOptions.length === 0}
            >
              <option value="">{assistantProviderOptions.length === 0 ? t('assistant.noWorkspaceProvider') : t('assistant.selectProvider')}</option>
              {assistantProviderOptions.map(provider => (
                <option key={provider.id} value={provider.id}>{provider.name}</option>
              ))}
            </select>
            <button
              type="button"
              onClick={() => void saveAssistantSettings()}
              disabled={assistantSaving || !assistantProviderId}
              className="w-fit rounded-lg border border-sky-600 bg-white px-3 py-2 text-sm font-medium text-sky-700 hover:bg-sky-50 disabled:opacity-50 dark:border-sky-500 dark:bg-zinc-900 dark:text-sky-400 dark:hover:bg-zinc-800"
            >
              {assistantSaving ? t('forms.saving') : t('assistant.saveSettings')}
            </button>
          </div>
        </section>
      )}
    </>
  )
}

function canManageProviderRow(provider: ProviderRow, canManageWorkspace: boolean, username?: string): boolean {
  if (provider.ownerType === 'user') {
    return Boolean(username && provider.ownerId === username)
  }
  return canManageWorkspace
}

function inferModelAccountMode(provider: Pick<ProviderRow, 'baseUrl' | 'type'>): ModelAccountMode {
  if (provider.baseUrl?.trim()) return 'gateway'
  if (isKnownProviderType(provider.type)) return 'official'
  return 'gateway'
}

function isKnownProviderType(type?: string): boolean {
  return type === 'openai' || type === 'anthropic' || type === 'cursor' || type === 'gemini'
}

function inferModelAccountCLI(provider: Pick<ProviderRow, 'type'>): ModelAccountCLI {
  switch (provider.type) {
    case 'anthropic': return 'claudecode'
    case 'cursor': return 'cursor'
    case 'gemini': return 'gemini'
    case 'openai':
    default: return 'codex'
  }
}

function isOfficialCLI(cli?: string): cli is ModelAccountCLI {
  return cli === 'codex' || cli === 'claudecode' || cli === 'cursor' || cli === 'gemini'
}

function isGatewayCLI(cli?: string): cli is ModelAccountCLI {
  return cli === 'codex' || cli === 'claudecode'
}

function providerTypeForCLI(cli?: ModelAccountCLI): string {
  switch (cli) {
    case 'claudecode': return 'anthropic'
    case 'cursor': return 'cursor'
    case 'gemini': return 'gemini'
    case 'codex':
    default: return 'openai'
  }
}

function providerCLILabel(cli?: ModelAccountCLI): string {
  switch (cli) {
    case 'claudecode': return 'Claude Code'
    case 'cursor': return 'Cursor'
    case 'gemini': return 'Gemini CLI'
    case 'codex':
    default: return 'Codex'
  }
}

function defaultAuthMethodForCLI(cli?: ModelAccountCLI): string {
  switch (cli) {
    case 'codex': return 'codex_chatgpt'
    case 'claudecode': return 'claudecode_browser'
    case 'cursor': return 'cursor_browser'
    default: return 'api_key'
  }
}

function supportsBrowserModelAuth(cli?: ModelAccountCLI): boolean {
  return cli === 'codex' || cli === 'claudecode' || cli === 'cursor'
}

function providerAuthConfiguredLabel(provider: ProviderRow, t: TFn): string {
  if (provider.authMethod === 'codex_chatgpt') return t('provider.chatGPTConfigured')
  if (provider.authMethod === 'claudecode_browser' || provider.authMethod === 'cursor_browser') return t('provider.browserConfigured')
  return t('provider.keyConfigured')
}

function providerOfficialName(cli?: ModelAccountCLI): string {
  switch (cli) {
    case 'claudecode': return 'Claude Code Official'
    case 'cursor': return 'Cursor Official'
    case 'gemini': return 'Gemini Official'
    case 'codex':
    default: return 'Codex Official'
  }
}

function isGeneratedModelAccountName(name?: string): boolean {
  const trimmed = name?.trim()
  return trimmed === 'Codex Official' || trimmed === 'Claude Code Official' || trimmed === 'Cursor Official' || trimmed === 'Gemini Official'
}

function providerEndpointPlaceholder(cli?: ModelAccountCLI): string {
  switch (cli) {
    case 'claudecode': return 'https://api.example.com/anthropic'
    case 'codex':
    default: return 'https://api.example.com/v1'
  }
}

function providerModelPlaceholder(cli?: ModelAccountCLI): string {
  switch (cli) {
    case 'claudecode': return 'claude-sonnet-4-20250514'
    case 'cursor': return 'auto'
    case 'codex':
    default: return 'gpt-5.6-sol'
  }
}

// ── Workspace Secrets ──────────────────────────────────────────────────────

type SecretRow = { id: string; key: string; value?: string; scope: string; agents?: string[]; description?: string; createdAt: string; updatedAt: string }

function SecretsSection() {
  const { t } = useTranslation()
  const [secrets, setSecrets] = useState<SecretRow[]>([])
  const [revealedIds, setRevealedIds] = useState<Set<string>>(new Set())
  const [showForm, setShowForm] = useState(false)
  const [editId, setEditId] = useState<string | null>(null)
  const [key, setKey] = useState('')
  const [value, setValue] = useState('')
  const [scope, setScope] = useState<'global' | 'agents'>('global')
  const [selectedAgents, setSelectedAgents] = useState<string[]>([])
  const [desc, setDesc] = useState('')

  type AgentOption = { id: string; project: string; name: string }
  const [allAgents, setAllAgents] = useState<AgentOption[]>([])

  const load = useCallback(async () => {
    try {
      const data = await apiFetch('/api/v1/envvars')
      setSecrets(data as SecretRow[])
    } catch { /* ignore */ }
  }, [])

  const loadAgents = useCallback(async () => {
    try {
      const projects = await apiFetch('/api/v1/projects') as { name: string }[]
      const opts: AgentOption[] = []
      for (const p of projects) {
        try {
          const agents = await apiFetch(`/api/v1/projects/${encodeURIComponent(p.name)}/agents`) as { name: string }[]
          for (const a of agents) opts.push({ id: `${p.name}/${a.name}`, project: p.name, name: a.name })
        } catch { /* skip */ }
      }
      setAllAgents(opts)
    } catch { /* ignore */ }
  }, [])

  useEffect(() => { load() }, [load])
  useEffect(() => { loadAgents() }, [loadAgents])

  function resetForm() {
    setShowForm(false); setEditId(null); setKey(''); setValue(''); setScope('global'); setSelectedAgents([]); setDesc('')
  }

  function startEdit(s: SecretRow) {
    setEditId(s.id); setKey(s.key); setValue(''); setScope(s.scope as 'global' | 'agents'); setSelectedAgents(s.agents ?? []); setDesc(s.description ?? ''); setShowForm(true)
  }

  function toggleAgent(agentId: string) {
    setSelectedAgents(prev => prev.includes(agentId) ? prev.filter(a => a !== agentId) : [...prev, agentId])
  }

  async function handleSave(e: React.FormEvent) {
    e.preventDefault()
    const body: Record<string, unknown> = { key, scope, description: desc, agents: scope === 'agents' ? selectedAgents : [] }
    if (value) body.value = value
    try {
      if (editId) {
        await apiPut(`/api/v1/envvars/${editId}`, body)
      } else {
        body.value = value
        await apiPost('/api/v1/envvars', body)
      }
      resetForm(); load()
    } catch { /* toast handled by apiFetch */ }
  }

  async function handleDelete(id: string) {
    const ok = await confirmDialog({
      title: t('common.delete'),
      description: t('secrets.confirmDelete'),
      confirmLabel: t('common.delete'),
      cancelLabel: t('common.cancel'),
    })
    if (!ok) return
    try { await apiDelete(`/api/v1/envvars/${id}`); load() } catch { /* ignore */ }
  }

  const agentsByProject = allAgents.reduce<Record<string, AgentOption[]>>((acc, a) => {
    (acc[a.project] ??= []).push(a); return acc
  }, {})

  return (
    <section className="rounded-xl border border-neutral-200/70 bg-white p-5 dark:border-zinc-700/50 dark:bg-zinc-900">
      <div className="flex items-center justify-between">
        <h3 className="flex items-center gap-2 text-base font-semibold text-neutral-800 dark:text-zinc-100">
          <KeyRound className="size-4 text-sky-600" />
          {t('secrets.title')}
        </h3>
        {!showForm && (
          <button onClick={() => { resetForm(); setShowForm(true) }} className="rounded-lg border border-sky-600 bg-white px-3 py-1.5 text-sm font-medium text-sky-700 hover:bg-sky-50 dark:border-sky-500 dark:bg-zinc-900 dark:text-sky-400 dark:hover:bg-zinc-800">
            {t('secrets.add')}
          </button>
        )}
      </div>
      <p className="mt-1 text-xs text-neutral-500 dark:text-zinc-500">{t('secrets.hint')}</p>

      {showForm && (
        <form onSubmit={handleSave} className="mt-4 space-y-3 rounded-lg border border-neutral-200/70 bg-neutral-50/50 p-4 dark:border-zinc-700/50 dark:bg-zinc-800/30">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="mb-1 block text-xs font-medium text-neutral-600 dark:text-zinc-400">{t('secrets.key')}</label>
              <input value={key} onChange={e => setKey(e.target.value)} placeholder="GITHUB_TOKEN" required className={inputCls + ' !max-w-none'} />
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-neutral-600 dark:text-zinc-400">{t('secrets.value')} {editId && <span className="text-neutral-400">({t('secrets.leaveEmpty')})</span>}</label>
              <input type="password" value={value} onChange={e => setValue(e.target.value)} placeholder="****" className={inputCls + ' !max-w-none'} required={!editId} />
            </div>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="mb-1 block text-xs font-medium text-neutral-600 dark:text-zinc-400">{t('secrets.scope')}</label>
              <select value={scope} onChange={e => setScope(e.target.value as 'global' | 'agents')} className={selectCls + ' !max-w-none'}>
                <option value="global">{t('secrets.scopeGlobal')}</option>
                <option value="agents">{t('secrets.scopeAgents')}</option>
              </select>
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-neutral-600 dark:text-zinc-400">{t('secrets.description')}</label>
              <input value={desc} onChange={e => setDesc(e.target.value)} placeholder={t('secrets.descPlaceholder')} className={inputCls + ' !max-w-none'} />
            </div>
          </div>
          {scope === 'agents' && (
            <div>
              <label className="mb-1 block text-xs font-medium text-neutral-600 dark:text-zinc-400">{t('secrets.agents')}</label>
              {allAgents.length === 0 ? (
                <p className="text-xs text-neutral-400 dark:text-zinc-500">{t('secrets.noAgents')}</p>
              ) : (
                <div className="max-h-40 overflow-y-auto rounded-md border border-neutral-200/80 bg-white p-2 dark:border-zinc-700/60 dark:bg-zinc-800/50">
                  {Object.entries(agentsByProject).map(([proj, agents]) => (
                    <div key={proj} className="mb-1.5 last:mb-0">
                      <div className="mb-1 text-[10px] font-semibold uppercase tracking-wide text-neutral-400 dark:text-zinc-500">{proj}</div>
                      <div className="flex flex-wrap gap-1.5">
                        {agents.map(a => {
                          const active = selectedAgents.includes(a.id)
                          return (
                            <button key={a.id} type="button" onClick={() => toggleAgent(a.id)}
                              className={cn('rounded-full border px-2.5 py-0.5 text-xs font-medium transition-colors',
                                active
                                  ? 'border-sky-500 bg-sky-50 text-sky-700 dark:border-sky-600 dark:bg-sky-900/30 dark:text-sky-400'
                                  : 'border-neutral-200 bg-white text-neutral-500 hover:border-sky-300 hover:text-sky-600 dark:border-zinc-700 dark:bg-zinc-800 dark:text-zinc-400 dark:hover:border-sky-600'
                              )}>
                              {a.name}
                            </button>
                          )
                        })}
                      </div>
                    </div>
                  ))}
                </div>
              )}
              {selectedAgents.length > 0 && (
                <p className="mt-1.5 text-[11px] text-neutral-500 dark:text-zinc-500">
                  {t('secrets.selectedCount', { count: selectedAgents.length })}
                </p>
              )}
            </div>
          )}
          <div className="flex gap-2">
            <button type="submit" className="rounded-md bg-sky-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-sky-700">{editId ? t('common.save') : t('secrets.add')}</button>
            <button type="button" onClick={resetForm} className="rounded-md border border-neutral-300 px-3 py-1.5 text-sm text-neutral-600 hover:bg-neutral-50 dark:border-zinc-600 dark:text-zinc-400 dark:hover:bg-zinc-800">{t('common.cancel')}</button>
          </div>
        </form>
      )}

      {secrets.length > 0 && (
        <div className="mt-4 overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-neutral-200/60 text-left text-xs font-medium text-neutral-500 dark:border-zinc-700/50 dark:text-zinc-500">
                <th className="pb-2 pr-4">{t('secrets.key')}</th>
                <th className="pb-2 pr-4">{t('secrets.value')}</th>
                <th className="pb-2 pr-4">{t('secrets.scope')}</th>
                <th className="pb-2 pr-4">{t('secrets.description')}</th>
                <th className="pb-2 text-right">{t('common.actions')}</th>
              </tr>
            </thead>
            <tbody>
              {secrets.map(s => {
                const revealed = revealedIds.has(s.id)
                return (
                  <tr key={s.id} className="border-b border-neutral-100/60 dark:border-zinc-800/40">
                    <td className="py-2.5 pr-4 font-mono text-xs font-semibold text-neutral-800 dark:text-zinc-200">{s.key}</td>
                    <td className="py-2.5 pr-4">
                      <div className="flex items-center gap-1.5">
                        <span className="font-mono text-xs text-neutral-600 dark:text-zinc-400">{revealed ? (s.value || '—') : '••••••'}</span>
                        <button onClick={() => setRevealedIds(prev => { const n = new Set(prev); revealed ? n.delete(s.id) : n.add(s.id); return n })}
                          className="text-neutral-400 hover:text-sky-600 dark:text-zinc-500 dark:hover:text-sky-400">
                          {revealed ? <EyeOff className="size-3" /> : <Eye className="size-3" />}
                        </button>
                      </div>
                    </td>
                    <td className="py-2.5 pr-4">
                      <span className={cn('inline-block rounded-full px-2 py-0.5 text-[10px] font-medium',
                        s.scope === 'global' ? 'bg-sky-50 text-sky-700 dark:bg-sky-900/20 dark:text-sky-400' : 'bg-violet-50 text-violet-700 dark:bg-violet-900/20 dark:text-violet-400'
                      )}>
                        {s.scope === 'global' ? t('secrets.scopeGlobal') : t('secrets.scopeAgents')}
                      </span>
                      {s.scope === 'agents' && s.agents && s.agents.length > 0 && (
                        <span className="ml-1.5 text-[10px] text-neutral-400 dark:text-zinc-600">{s.agents.join(', ')}</span>
                      )}
                    </td>
                    <td className="py-2.5 pr-4 text-xs text-neutral-500 dark:text-zinc-500">{s.description || '—'}</td>
                    <td className="py-2.5 text-right">
                      <button onClick={() => startEdit(s)} className="mr-2 text-neutral-400 hover:text-sky-600 dark:text-zinc-500 dark:hover:text-sky-400"><Pencil className="size-3.5" /></button>
                      <button onClick={() => handleDelete(s.id)} className="text-neutral-400 hover:text-red-500 dark:text-zinc-500 dark:hover:text-red-400"><Trash2 className="size-3.5" /></button>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}

export default function SettingsPage() {
  const { t } = useTranslation()
  const { user } = useAuth()
  const { canAdmin } = useWorkspaceAccess()

  return (
    <div className="animate-fade-in px-8 py-6">
      <div className="flex items-start justify-between gap-4 pb-5">
        <div>
          <h1 className="text-xl font-semibold text-neutral-900 dark:text-zinc-100">{t('settings.title')}</h1>
          <p className="mt-0.5 text-sm text-neutral-500 dark:text-zinc-500">{t('settings.intro')}</p>
        </div>
        <button
          type="button"
          onClick={() => window.dispatchEvent(new Event('product-tour-start'))}
          className="rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm font-medium text-neutral-600 hover:bg-neutral-50 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:bg-zinc-800"
        >
          {t('productTour.start')}
        </button>
      </div>

      <div className="space-y-5">
        {/* RBAC Model (admin only) */}
        {canAdmin && <RBACSection />}

        {/* Model accounts */}
        <ProvidersSection />

        {/* External tool OAuth apps (admin only) */}
        {canAdmin && <ExternalToolOAuthSection />}

        {/* Workspace Secrets (admin only) */}
        {user?.role === 'admin' && <SecretsSection />}
      </div>
    </div>
  )
}
