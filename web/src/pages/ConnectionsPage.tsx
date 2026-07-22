import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { QRCodeSVG } from 'qrcode.react'
import { X } from 'lucide-react'
import { useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { apiDelete, apiFetch, apiPost, apiPut } from '../lib/api'
import { useAuth } from '../lib/auth'
import { cn } from '../lib/cn'
import { confirmDialog } from '../components/ui/ConfirmDialog'
import { primaryOutlineButton } from '../lib/button-styles'
import { formatDateTimeForLanguage, useFormatDateTime } from '../lib/format-datetime'

type ProviderField = { key: string; label: string; inputType: string; required: boolean; secret: boolean }
type ProviderGuide = { title: string; body: string; links?: Array<{ label: string; url: string }> }
type ProviderAction = { name: string; displayName: string; description?: string }
type Provider = {
  provider: string
  displayName: string
  description?: string
  category?: string
  homepageUrl?: string
  iconUrl?: string
  authTypes: string[]
  fields?: ProviderField[]
  actions?: ProviderAction[]
  guides?: ProviderGuide[]
  comingSoon?: boolean
  enabled: boolean
}
type ConnectionGrant = { id: string; targetType: string; targetId: string; createdBy?: string; createdAt: string }
type Connection = {
  id: string
  provider: string
  connectionName: string
  ownerType: string
  ownerId: string
  authType: string
  status: string
  profile?: Record<string, unknown>
  profileSummary?: ConnectionProfileSummary
  grants?: ConnectionGrant[]
  createdBy?: string
  createdAt: string
  updatedAt?: string
}
type ConnectionProfileSummary = {
  displayName?: string
  accountId?: string
  accountName?: string
  accountEmail?: string
  scopes?: string[]
  providerPermissions?: string[]
  actionPolicy?: {
    allowedMethods?: string[]
    blockedMethods?: string[]
    allowedEndpoints?: string[]
    blockedEndpoints?: string[]
  }
}
type ConnectionTestResult = { ok: boolean; status: number; message: string }
type OAuthAuthorizationStart = { authorizationUrl: string; state: string }
type DeviceSetupBegin = { deviceCode: string; qrUrl: string; userCode?: string; interval?: number; expiresIn?: number; baseUrl?: string }
type DeviceSetupPoll = { status: string; stage?: string; deviceCode?: string; qrUrl?: string; userCode?: string; interval?: number; expiresIn?: number; baseUrl?: string; slowDown?: boolean; error?: string; connection?: Connection }
type WorkspaceSummary = { id: string; name: string; currentUserRole?: string; currentUserCanAdmin?: boolean }
type ProjectSummary = { name: string; description?: string }
type ProjectToolInstallResult = { ok: boolean; installed: number; skipped: number; provider: string; connectionId: string }
type OAuthClientConfig = {
  provider: string
  displayName: string
  configured: boolean
  clientId?: string
  expectedRedirectUri: string
  oauth?: { authorizationUrl?: string; tokenUrl?: string; scopes?: string[] }
  extra?: Record<string, unknown>
  updatedAt?: string
}
type TFn = (key: string, options?: Record<string, unknown>) => string
type ConnectionStatusFilter = 'all' | 'configured' | 'not_configured'

const inputCls = 'w-full rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm outline-none focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-100'
const selectCls = 'w-full rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm outline-none focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-100 dark:[color-scheme:dark]'
const customMCPProviderID = 'custom-mcp'
const customMCPCategory = 'Custom MCP Tools'

export default function ConnectionsPage() {
  const { t } = useTranslation()
  const { user } = useAuth()
  const [workspace, setWorkspace] = useState<WorkspaceSummary | null>(null)
  const isWorkspaceAdmin = workspace?.currentUserCanAdmin ?? (!user || user.role === 'admin')
  const [providers, setProviders] = useState<Provider[]>([])
  const [oauthConfigs, setOAuthConfigs] = useState<OAuthClientConfig[]>([])
  const [connections, setConnections] = useState<Connection[]>([])
  const [loading, setLoading] = useState(true)
  const [creatingProvider, setCreatingProvider] = useState<string | null>(null)
  const [editing, setEditing] = useState<Connection | null>(null)
  const [testResults, setTestResults] = useState<Record<string, { loading?: boolean; ok?: boolean; message?: string }>>({})
  const [oauthMessage, setOauthMessage] = useState<{ kind: 'success' | 'error'; text: string } | null>(null)
  const [installingConnection, setInstallingConnection] = useState<Connection | null>(null)
  const [installMessage, setInstallMessage] = useState<{ kind: 'success' | 'error'; text: string } | null>(null)
  const [reloadKey, setReloadKey] = useState(0)
  const [statusFilter, setStatusFilter] = useState<ConnectionStatusFilter>('all')
  const [categoryFilter, setCategoryFilter] = useState('all')
  const [toolSearch, setToolSearch] = useState('')
  const [searchParams, setSearchParams] = useSearchParams()

  useEffect(() => {
    const status = searchParams.get('oauth')
    if (status !== 'success' && status !== 'error') return
    const provider = searchParams.get('oauthProvider') || t('connections.oauthProviderFallback')
    const connectionName = searchParams.get('oauthConnection') || t('connections.connectionFallback')
    if (status === 'success') {
      setOauthMessage({ kind: 'success', text: t('connections.oauthConnected', { provider, connection: connectionName }) })
      setReloadKey(k => k + 1)
    } else {
      const message = searchParams.get('message') || t('connections.oauthFailed')
      setOauthMessage({ kind: 'error', text: `${provider}/${connectionName}: ${message}` })
    }
    const next = new URLSearchParams(searchParams)
    for (const key of ['oauth', 'oauthProvider', 'oauthConnection', 'connectionId', 'message']) {
      next.delete(key)
    }
    setSearchParams(next, { replace: true })
  }, [searchParams, setSearchParams])

  useEffect(() => {
    let cancelled = false
    async function load() {
      setLoading(true)
      try {
        const [providerData, connectionData, workspaceData] = await Promise.all([
          apiFetch<{ providers: Provider[] }>('/api/v1/connectors/providers'),
          apiFetch<{ connections: Connection[] }>('/api/v1/connections'),
          apiFetch<WorkspaceSummary>('/api/v1/workspace').catch(() => null),
        ])
        const oauthData = await apiFetch<{ configs: OAuthClientConfig[] }>('/api/v1/oauth/client-configs').catch(() => ({ configs: [] }))
        if (cancelled) return
        setWorkspace(workspaceData)
        setProviders(providerData.providers ?? [])
        setOAuthConfigs(oauthData.configs ?? [])
        setConnections(connectionData.connections ?? [])
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    void load()
    return () => { cancelled = true }
  }, [reloadKey])

  async function removeConnection(connection: Connection) {
    const ok = await confirmDialog({
      title: t('connections.disconnectConnection'),
      description: t('connections.disconnectConnectionConfirm', { name: `${connection.provider}/${connection.connectionName}` }),
      confirmLabel: t('common.delete'),
      cancelLabel: t('common.cancel'),
    })
    if (!ok) return
    await apiDelete(`/api/v1/connections/${encodeURIComponent(connection.id)}`)
    setReloadKey(k => k + 1)
  }

  async function testConnection(connection: Connection) {
    setTestResults(prev => ({ ...prev, [connection.id]: { loading: true } }))
    try {
      const result = await apiPost<ConnectionTestResult>(`/api/v1/connections/${encodeURIComponent(connection.id)}/test`, {})
      setTestResults(prev => ({
        ...prev,
        [connection.id]: {
          ok: result.ok,
          message: result.ok
            ? t('connections.healthOkHttp', { status: result.status })
            : t('connections.healthFailedHttp', { message: result.message || t('connections.failed'), status: result.status }),
        },
      }))
    } catch (e) {
      setTestResults(prev => ({
        ...prev,
        [connection.id]: { ok: false, message: e instanceof Error ? e.message : String(e) },
      }))
    } finally {
      setReloadKey(k => k + 1)
    }
  }

  const customMCPProvider = providers.find(provider => provider.provider === customMCPProviderID)
  const customMCPConnections = connections
    .filter(connection => connection.provider === customMCPProviderID)
    .sort((a, b) => (b.updatedAt || b.createdAt).localeCompare(a.updatedAt || a.createdAt))
  const filteredCustomMCPConnections = useMemo(() => {
    const query = normalizeToolSearch(toolSearch)
    if (!customMCPProvider || !query) return customMCPConnections
    return customMCPConnections.filter(connection => customMCPConnectionMatchesSearch(customMCPProvider, connection, query, t))
  }, [customMCPProvider, customMCPConnections, toolSearch, t])

  const categoryOptions = useMemo(() => {
    const categories = new Set<string>()
    for (const provider of providers) {
      if (provider.provider === customMCPProviderID) continue
      categories.add(provider.category || 'Other Tools')
    }
    if (customMCPProvider) categories.add(customMCPCategory)
    return Array.from(categories).sort((a, b) => providerCategorySort(a, b))
  }, [providers])
  const filteredProvidersByCategory = useMemo(() => {
    const groups = new Map<string, Provider[]>()
    const query = normalizeToolSearch(toolSearch)
    for (const provider of providers) {
      if (provider.provider === customMCPProviderID) continue
      const category = provider.category || 'Other Tools'
      if (categoryFilter !== 'all' && category !== categoryFilter) continue
      if (query && !providerMatchesSearch(provider, query, t)) continue
      const connected = !!primaryConnectionForProvider(connections, provider.provider)
      if (statusFilter === 'configured' && !connected) continue
      if (statusFilter === 'not_configured' && connected) continue
      groups.set(category, [...(groups.get(category) ?? []), provider])
    }
    return Array.from(groups.entries())
      .sort(([a], [b]) => providerCategorySort(a, b))
      .map(([category, items]) => [category, items.sort((a, b) => a.displayName.localeCompare(b.displayName))] as const)
  }, [providers, connections, statusFilter, categoryFilter, toolSearch, t])
  const showCustomMCPConnections = !!customMCPProvider
    && filteredCustomMCPConnections.length > 0
    && statusFilter !== 'not_configured'
    && (categoryFilter === 'all' || categoryFilter === customMCPCategory)
  const hasFilteredTools = showCustomMCPConnections || filteredProvidersByCategory.length > 0

  function resetFilters() {
    setStatusFilter('all')
    setCategoryFilter('all')
    setToolSearch('')
  }

  return (
    <div className="animate-fade-in px-8 py-6">
      <div className="flex items-center justify-between pb-5">
        <div>
          <h1 className="text-xl font-semibold text-neutral-900 dark:text-zinc-100">{t('connections.title')}</h1>
          <p className="mt-0.5 text-sm text-neutral-500 dark:text-zinc-500">{t('connections.subtitle')}</p>
        </div>
        {customMCPProvider && (
          <button type="button" onClick={() => setCreatingProvider(customMCPProviderID)} className={primaryOutlineButton}>
            {t('connections.newMCPTool')}
          </button>
        )}
      </div>
      {!loading && (
        <div className="mb-5 flex flex-wrap items-end gap-3 rounded-xl border border-neutral-200/80 bg-white px-4 py-3 dark:border-zinc-700/60 dark:bg-zinc-900/40">
          <label className="w-64">
            <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">{t('connections.searchTools')}</span>
            <input
              className={cn(inputCls, 'mt-1')}
              value={toolSearch}
              onChange={event => setToolSearch(event.target.value)}
              placeholder={t('connections.searchToolsPlaceholder')}
            />
          </label>
          <label className="w-44">
            <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">{t('connections.filterStatus')}</span>
            <select className={cn(selectCls, 'mt-1')} value={statusFilter} onChange={event => setStatusFilter(event.target.value as ConnectionStatusFilter)}>
              <option value="all">{t('connections.allStatuses')}</option>
              <option value="configured">{t('connections.configuredOnly')}</option>
              <option value="not_configured">{t('connections.notConfiguredOnly')}</option>
            </select>
          </label>
          <label className="w-56">
            <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">{t('connections.filterCategory')}</span>
            <select className={cn(selectCls, 'mt-1')} value={categoryFilter} onChange={event => setCategoryFilter(event.target.value)}>
              <option value="all">{t('connections.allCategories')}</option>
              {categoryOptions.map(category => (
                <option key={category} value={category}>{providerCategoryLabel(category, t)}</option>
              ))}
            </select>
          </label>
          {(statusFilter !== 'all' || categoryFilter !== 'all' || toolSearch.trim() !== '') && (
            <button type="button" onClick={resetFilters} className="rounded-lg px-3 py-2 text-sm font-medium text-neutral-500 hover:bg-neutral-100 dark:text-zinc-400 dark:hover:bg-zinc-800">
              {t('connections.clearFilters')}
            </button>
          )}
        </div>
      )}
      {oauthMessage && (
        <div className={cn(
          'mb-4 rounded-lg border px-3 py-2 text-xs',
          oauthMessage.kind === 'success'
            ? 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900/50 dark:bg-emerald-950/30 dark:text-emerald-300'
            : 'border-rose-200 bg-rose-50 text-rose-700 dark:border-rose-900/50 dark:bg-rose-950/30 dark:text-rose-300',
        )}>
          {oauthMessage.text}
        </div>
      )}

      {loading ? (
        <div className="flex items-center justify-center gap-2 py-16 text-sm text-neutral-500">
          <div className="size-5 animate-spin rounded-full border-2 border-neutral-300 border-t-sky-600 dark:border-zinc-600 dark:border-t-sky-400" />
          {t('connections.loading')}
        </div>
      ) : (
        <div className="space-y-7">
          {showCustomMCPConnections && customMCPProvider && (
            <section>
              <div className="mb-3 flex items-center justify-between">
                <h2 className="text-sm font-semibold text-neutral-900 dark:text-zinc-100">{t('connections.customMCPTools')}</h2>
                <span className="text-xs text-neutral-400 dark:text-zinc-500">{t('connections.toolCount', { count: filteredCustomMCPConnections.length })}</span>
              </div>
              <div className="grid gap-4 xl:grid-cols-2">
                {filteredCustomMCPConnections.map(connection => (
                  <ExternalToolCard
                    key={connection.id}
                    provider={providerForCustomMCPConnection(customMCPProvider, connection)}
                    connection={connection}
                    isWorkspaceAdmin={isWorkspaceAdmin}
                    currentUsername={user?.username ?? ''}
                    testResults={testResults}
                    onConfigure={() => setCreatingProvider(customMCPProviderID)}
                    onEdit={setEditing}
                  />
                ))}
              </div>
            </section>
          )}
          {filteredProvidersByCategory.map(([category, items]) => (
            <section key={category}>
              <div className="mb-3 flex items-center justify-between">
                <h2 className="text-sm font-semibold text-neutral-900 dark:text-zinc-100">{providerCategoryLabel(category, t)}</h2>
                <span className="text-xs text-neutral-400 dark:text-zinc-500">{t('connections.toolCount', { count: items.length })}</span>
              </div>
              <div className="grid gap-4 xl:grid-cols-2">
                {items.map(provider => (
              <ExternalToolCard
                    key={provider.provider}
                    provider={provider}
                    connection={primaryConnectionForProvider(connections, provider.provider)}
                    oauthConfig={oauthConfigs.find(config => config.provider === provider.provider)}
                    isWorkspaceAdmin={isWorkspaceAdmin}
                    currentUsername={user?.username ?? ''}
                    testResults={testResults}
                    onConfigure={() => setCreatingProvider(provider.provider)}
                    onEdit={setEditing}
                  />
                ))}
              </div>
            </section>
          ))}
          {!hasFilteredTools && (
            <div className="rounded-xl border border-dashed border-neutral-200 bg-white px-6 py-12 text-center dark:border-zinc-700 dark:bg-zinc-900/40">
              <p className="text-sm font-medium text-neutral-700 dark:text-zinc-200">{t('connections.noToolsMatchFilters')}</p>
              <button type="button" onClick={resetFilters} className="mt-3 rounded-lg px-3 py-2 text-sm font-medium text-sky-700 hover:bg-sky-50 dark:text-sky-400 dark:hover:bg-zinc-800">
                {t('connections.clearFilters')}
              </button>
            </div>
          )}
        </div>
      )}

      {creatingProvider && (
        <ConnectionDialog
          providers={providers}
          oauthConfigs={oauthConfigs}
          isWorkspaceAdmin={isWorkspaceAdmin}
          fixedProviderId={creatingProvider}
          onClose={() => setCreatingProvider(null)}
          onCreated={() => { setCreatingProvider(null); setReloadKey(k => k + 1) }}
        />
      )}
      {editing && (
        <ConnectionDialog
          providers={providers}
          oauthConfigs={oauthConfigs}
          isWorkspaceAdmin={isWorkspaceAdmin}
          connection={editing}
          testState={testResults[editing.id]}
          onTest={connection => void testConnection(connection)}
          onDelete={connection => void removeConnection(connection)}
          onInstallToProject={connection => { setEditing(null); setInstallingConnection(connection) }}
          onClose={() => setEditing(null)}
          onCreated={() => { setEditing(null); setReloadKey(k => k + 1) }}
        />
      )}
      {installingConnection && (
        <InstallToolToProjectDialog
          connection={installingConnection}
          onClose={() => setInstallingConnection(null)}
          onInstalled={(result) => {
            setInstallingConnection(null)
            setInstallMessage({
              kind: 'success',
              text: t('connections.installToProjectSuccess', { count: result.installed, skipped: result.skipped }),
            })
            setReloadKey(k => k + 1)
          }}
        />
      )}
      {installMessage && (
        <div className={cn(
          'fixed bottom-5 right-5 z-50 max-w-sm rounded-lg border px-4 py-3 text-sm shadow-lg',
          installMessage.kind === 'success'
            ? 'border-emerald-200 bg-white text-emerald-700 dark:border-emerald-900/50 dark:bg-zinc-900 dark:text-emerald-300'
            : 'border-rose-200 bg-white text-rose-700 dark:border-rose-900/50 dark:bg-zinc-900 dark:text-rose-300',
        )}>
          {installMessage.text}
        </div>
      )}
    </div>
  )
}

function ExternalToolCard({
  provider,
  connection,
  oauthConfig,
  isWorkspaceAdmin,
  currentUsername,
  testResults,
  onConfigure,
  onEdit,
}: {
  provider: Provider
  connection?: Connection
  oauthConfig?: OAuthClientConfig
  isWorkspaceAdmin: boolean
  currentUsername: string
  testResults: Record<string, { loading?: boolean; ok?: boolean; message?: string }>
  onConfigure: () => void
  onEdit: (connection: Connection) => void
}) {
  const { t } = useTranslation()
  const fmtDateTime = useFormatDateTime()
  const latestValidation = connection ? connectionValidation(connection, fmtDateTime) : null
  const oauthSupported = provider.authTypes.includes('oauth2')
  const staticAuthTypes = provider.authTypes.filter(type => type !== 'oauth2')
  const oauthAvailable = oauthSupported && oauthConfig?.configured === true
  const canConfigure = !provider.comingSoon && (staticAuthTypes.length > 0 || oauthAvailable)
  const canManageConnection = connection && (connection.ownerType === 'workspace' ? isWorkspaceAdmin : connection.ownerId === currentUsername)
  const statusLabel = provider.comingSoon
    ? t('connections.comingSoon')
    : connection
      ? t('connections.configured')
      : oauthSupported && staticAuthTypes.length === 0 && !oauthAvailable
        ? t('connections.adminSetupRequired')
        : t('connections.notConfigured')
  return (
    <section className="rounded-xl border border-neutral-200/80 bg-white p-5 dark:border-zinc-700/60 dark:bg-zinc-900/40">
      <div className="flex items-start justify-between gap-4">
        <div className="flex min-w-0 items-start gap-3">
          <ProviderLogo provider={provider} />
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="text-sm font-semibold text-neutral-900 dark:text-zinc-100">{provider.displayName}</h3>
              <span className={cn(
                'rounded-full px-2 py-0.5 text-[11px] font-medium',
                provider.comingSoon
                  ? 'bg-neutral-100 text-neutral-500 dark:bg-zinc-800 dark:text-zinc-400'
                  : connection
                    ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
                    : 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
              )}>
                {statusLabel}
              </span>
            </div>
            <p className="mt-2 line-clamp-2 text-sm text-neutral-500 dark:text-zinc-400">{providerDescription(provider, t)}</p>
          </div>
        </div>
        <button type="button" onClick={() => connection && canManageConnection ? onEdit(connection) : onConfigure()} disabled={connection ? !canManageConnection : !canConfigure} className={cn(primaryOutlineButton, 'shrink-0 justify-center disabled:cursor-not-allowed disabled:opacity-50')}>
          {connection ? t('connections.manageConnection') : t('connections.configureTool')}
        </button>
      </div>
      <div className="mt-4 grid gap-3 sm:grid-cols-3">
        <ToolStat label={t('connections.authMethods')} value={formatAuthTypes(provider.authTypes, oauthAvailable, isWorkspaceAdmin, t)} />
        <ToolStat label={t('connections.credentials')} value={connection ? t('connections.connected') : t('connections.notConfigured')} />
        <ToolStat label={t('connections.actions')} value={provider.provider === customMCPProviderID ? 'MCP' : String(provider.actions?.length ?? 0)} />
      </div>
      <div className="mt-4 flex flex-wrap items-center gap-2 text-xs text-neutral-400 dark:text-zinc-500">
        {oauthSupported && oauthAvailable && (
          <span>{t('connections.oauthEnabled')}</span>
        )}
        {latestValidation && (
          <span>{latestValidation.ok ? t('connections.healthy') : t('connections.failed')} · {latestValidation.atLabel}</span>
        )}
      </div>
      {connection && testResults[connection.id]?.message && (
        <p className={cn('mt-3 truncate text-xs', testResults[connection.id]?.ok ? 'text-emerald-600 dark:text-emerald-300' : 'text-red-600 dark:text-red-300')}>
          {testResults[connection.id]?.message}
        </p>
      )}
    </section>
  )
}

function InstallToolToProjectDialog({
  connection,
  onClose,
  onInstalled,
}: {
  connection: Connection
  onClose: () => void
  onInstalled: (result: ProjectToolInstallResult) => void
}) {
  const { t } = useTranslation()
  const [projects, setProjects] = useState<ProjectSummary[]>([])
  const [project, setProject] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    let cancelled = false
    async function load() {
      setLoading(true)
      try {
        const data = await apiFetch<ProjectSummary[]>('/api/v1/projects')
        if (cancelled) return
        setProjects(data)
        setProject(data[0]?.name ?? '')
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    void load()
    return () => { cancelled = true }
  }, [])

  async function submit() {
    if (!project) return
    setSaving(true)
    try {
      const result = await apiPost<ProjectToolInstallResult>(`/api/v1/projects/${encodeURIComponent(project)}/tool-bindings/install`, {
        connectionId: connection.id,
      })
      onInstalled(result)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal title={t('connections.installToProjectTitle')} onClose={onClose}>
      <div className="space-y-4">
        <div className="rounded-lg bg-neutral-50 px-3 py-2 text-sm text-neutral-600 dark:bg-zinc-800/50 dark:text-zinc-300">
          {t('connections.installToProjectHint', { tool: connection.provider, connection: connection.connectionName })}
        </div>
        {loading ? (
          <div className="flex items-center gap-2 py-6 text-sm text-neutral-500">
            <div className="size-5 animate-spin rounded-full border-2 border-neutral-300 border-t-sky-600 dark:border-zinc-600 dark:border-t-sky-400" />
            {t('common.loading')}
          </div>
        ) : projects.length === 0 ? (
          <p className="rounded-lg bg-neutral-50 px-3 py-3 text-sm text-neutral-500 dark:bg-zinc-800/50 dark:text-zinc-400">
            {t('connections.installToProjectNoProjects')}
          </p>
        ) : (
          <label className="block">
            <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">{t('connections.project')}</span>
            <select className={selectCls} value={project} onChange={e => setProject(e.target.value)}>
              {projects.map(item => (
                <option key={item.name} value={item.name}>{item.name}</option>
              ))}
            </select>
          </label>
        )}
        <div className="flex justify-end gap-2 pt-2">
          <button type="button" onClick={onClose} disabled={saving} className="rounded-lg border border-neutral-300 px-3 py-2 text-sm dark:border-zinc-600">{t('common.cancel')}</button>
          <button type="button" onClick={() => void submit()} disabled={saving || loading || !project} className="rounded-lg bg-sky-600 px-3 py-2 text-sm font-medium text-white disabled:opacity-50">
            {saving ? t('common.saving') : t('connections.installToProjectConfirm')}
          </button>
        </div>
      </div>
    </Modal>
  )
}

function ToolStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg bg-neutral-50 px-3 py-2 dark:bg-zinc-800/50">
      <p className="text-[11px] text-neutral-400 dark:text-zinc-500">{label}</p>
      <p className="mt-1 truncate text-xs font-medium text-neutral-700 dark:text-zinc-200" title={value}>{value}</p>
    </div>
  )
}

function providerCategoryLabel(category: string, t: TFn): string {
  const key = `connections.categories.${categoryKey(category)}`
  const value = t(key)
  return value === key ? category : value
}

function providerCategorySort(a: string, b: string): number {
  const order = ['Developer Tools', 'Cloud And Infrastructure', 'Project Management', 'Knowledge And Docs', 'Communication', 'Design And Data', 'Research And Search', 'Advanced', customMCPCategory, 'Other Tools']
  const ai = order.indexOf(a)
  const bi = order.indexOf(b)
  if (ai === -1 && bi === -1) return a.localeCompare(b)
  if (ai === -1) return 1
  if (bi === -1) return -1
  return ai - bi
}

function providerDescription(provider: Provider, t: TFn): string {
  const key = `connections.providerDescriptions.${provider.provider}`
  const value = t(key)
  if (value !== key) return value
  return provider.description || t('connections.externalToolDefaultDescription')
}

function normalizeToolSearch(value: string): string {
  return value.trim().toLowerCase()
}

function providerMatchesSearch(provider: Provider, query: string, t: TFn): boolean {
  if (!query) return true
  const haystack = [
    provider.provider,
    provider.displayName,
    provider.description || '',
    providerDescription(provider, t),
    provider.category || '',
    providerCategoryLabel(provider.category || 'Other Tools', t),
  ].join(' ').toLowerCase()
  return haystack.includes(query)
}

function customMCPConnectionMatchesSearch(provider: Provider, connection: Connection, query: string, t: TFn): boolean {
  if (!query) return true
  return providerMatchesSearch(providerForCustomMCPConnection(provider, connection), query, t)
    || [
      connection.connectionName,
      connection.provider,
      connection.ownerType,
      connection.ownerId || '',
    ].join(' ').toLowerCase().includes(query)
}

function providerForCustomMCPConnection(provider: Provider, connection: Connection): Provider {
  const profile = connection.profile ?? {}
  const displayName = typeof profile.displayName === 'string' && profile.displayName.trim()
    ? profile.displayName.trim()
    : connection.connectionName || provider.displayName
  const description = typeof profile.description === 'string' && profile.description.trim()
    ? profile.description.trim()
    : provider.description
  return { ...provider, displayName, description }
}

function categoryKey(category: string): string {
  return category.trim().toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '')
}

function ProviderLogo({ provider }: { provider: Provider }) {
  const [failed, setFailed] = useState(false)
  const initials = provider.displayName
    .split(/\s+/)
    .map(part => part[0])
    .join('')
    .slice(0, 2)
    .toUpperCase() || '?'
  const logo = failed ? '' : providerLogoURL(provider)
  return (
    <span className="flex size-10 shrink-0 items-center justify-center rounded-lg border border-neutral-200 bg-white text-xs font-semibold text-neutral-500 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-400">
      {logo ? (
        <img
          alt=""
          className="size-6 object-contain"
          loading="lazy"
          referrerPolicy="no-referrer"
          src={logo}
          onError={() => setFailed(true)}
        />
      ) : initials}
    </span>
  )
}

function providerLogoURL(provider: Provider): string {
  if (provider.iconUrl?.trim()) return provider.iconUrl.trim()
  const domain = providerHomepageDomain(provider) || providerLogoDomains[provider.provider]
  if (!domain) return ''
  return `https://www.google.com/s2/favicons?sz=64&domain=${encodeURIComponent(domain)}`
}

function providerHomepageDomain(provider: Provider): string {
  const homepage = provider.homepageUrl?.trim()
  if (!homepage) return ''
  try {
    return new URL(homepage).hostname
  } catch {
    return ''
  }
}

const providerLogoDomains: Record<string, string> = {
  github: 'github.com',
  gitlab: 'gitlab.com',
  gitee: 'gitee.com',
  feishu: 'feishu.cn',
  lark: 'larksuite.com',
  linear: 'linear.app',
  jira: 'atlassian.com',
  notion: 'notion.so',
  slack: 'slack.com',
  dingtalk_bot: 'dingtalk.com',
  figma: 'figma.com',
  google_drive: 'drive.google.com',
  google_calendar: 'calendar.google.com',
  gmail: 'mail.google.com',
  confluence: 'atlassian.com',
  trello: 'trello.com',
  asana: 'asana.com',
  aws: 'aws.amazon.com',
  gcloud: 'cloud.google.com',
  vercel: 'vercel.com',
  cloudflare: 'cloudflare.com',
  stripe: 'stripe.com',
  sentry: 'sentry.io',
  exa: 'exa.ai',
  brave_search: 'search.brave.com',
}

function primaryConnectionForProvider(connections: Connection[], provider: string): Connection | undefined {
  return connections
    .filter(connection => connection.provider === provider)
    .sort((a, b) => {
      if (a.ownerType !== b.ownerType) return a.ownerType === 'workspace' ? -1 : 1
      return (b.updatedAt || b.createdAt).localeCompare(a.updatedAt || a.createdAt)
    })[0]
}

function formatAuthTypes(authTypes: string[], oauthAvailable: boolean, isWorkspaceAdmin: boolean, t: TFn): string {
  const labels = authTypes
    .filter(type => type !== 'oauth2' || oauthAvailable || isWorkspaceAdmin)
    .map(type => authTypeLabel(type, t))
  return labels.length > 0 ? labels.join(', ') : t('connections.adminSetupRequired')
}

function authTypeLabel(type: string, t: TFn): string {
  if (type === 'api_key') return t('connections.authApiKey')
  if (type === 'custom_credential') return t('connections.authAppCredential')
  if (type === 'oauth2') return t('connections.authOAuth')
  if (type === 'no_auth') return t('connections.authNoAuth')
  return type
}

function providerFieldLabel(field: ProviderField, t: TFn): string {
  const key = `connections.fieldLabels.${field.key}`
  const value = t(key)
  return value === key ? field.label : value
}

function providerGuideTitle(guide: ProviderGuide, t: TFn): string {
  return translatedProviderText('guideTitles', guide.title, t)
}

function providerGuideBody(guide: ProviderGuide, t: TFn): string {
  return translatedProviderText('guideBodies', guide.title, t, guide.body)
}

function providerGuideLinkLabel(label: string, t: TFn): string {
  return translatedProviderText('guideLinks', label, t)
}

function translatedProviderText(group: string, source: string, t: TFn, fallback = source): string {
  const key = `connections.${group}.${sourceKey(source)}`
  const value = t(key)
  return value === key ? fallback : value
}

function sourceKey(source: string): string {
  return source.trim().toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '')
}

function connectionValidation(connection: Connection, fmtDateTime: (input: string | number | Date | null | undefined) => string = (input) => formatDateTimeForLanguage(input, undefined)): { ok: boolean; status?: number; message: string; atLabel: string } | null {
  const profile = connection.profile ?? {}
  const at = typeof profile.lastValidatedAt === 'string' ? profile.lastValidatedAt : ''
  if (!at) return null
  const ok = profile.lastValidationOK === true
  const status = typeof profile.lastValidationStatus === 'number' ? profile.lastValidationStatus : undefined
  const message = typeof profile.lastValidationMessage === 'string' ? profile.lastValidationMessage : ''
  const atLabel = fmtDateTime(at)
  return { ok, status, message, atLabel }
}

function ConnectionDialog({
  providers,
  oauthConfigs,
  isWorkspaceAdmin,
  fixedProviderId,
  connection,
  testState,
  onTest,
  onDelete,
  onInstallToProject,
  onClose,
  onCreated,
}: {
  providers: Provider[]
  oauthConfigs: OAuthClientConfig[]
  isWorkspaceAdmin: boolean
  fixedProviderId?: string
  connection?: Connection
  testState?: { loading?: boolean; ok?: boolean; message?: string }
  onTest?: (connection: Connection) => void
  onDelete?: (connection: Connection) => void
  onInstallToProject?: (connection: Connection) => void
  onClose: () => void
  onCreated: () => void
}) {
  const { t } = useTranslation()
  const isEditing = Boolean(connection)
  const [providerId, setProviderId] = useState(connection?.provider ?? fixedProviderId ?? providers[0]?.provider ?? '')
  const provider = providers.find(p => p.provider === providerId)
  const oauthConfig = oauthConfigs.find(config => config.provider === providerId)
  const availableAuthTypes = useMemo(() => {
    const types = provider?.authTypes ?? []
    return types.filter(type => type !== 'oauth2' || oauthConfig?.configured)
  }, [provider, oauthConfig])
  const ownerType = connection?.ownerType ?? (isWorkspaceAdmin ? 'workspace' : 'user')
  const [authType, setAuthType] = useState(connection?.authType ?? availableAuthTypes[0] ?? 'api_key')
  const [connectionName, setConnectionName] = useState(connection?.connectionName ?? (providerId === customMCPProviderID ? '' : 'default'))
  const [values, setValues] = useState<Record<string, string>>({})
  const [saving, setSaving] = useState(false)
  const [deviceSetup, setDeviceSetup] = useState<
    | { step: 'idle' }
    | { step: 'beginning' }
    | { step: 'scanning'; deviceCode: string; qrUrl: string; userCode?: string; baseUrl: string; interval: number }
    | { step: 'connected' }
    | { step: 'error'; message: string }
  >({ step: 'idle' })
  const pollRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const pollGenerationRef = useRef(0)
  const canQuickAuthorize = !isEditing && (
    providerId === 'feishu' ||
    providerId === 'lark' ||
    providerId === 'github'
  )
  const readonlyConnection = isEditing && connection?.authType === 'oauth2'

  useEffect(() => {
    if (isEditing) return
    const p = providers.find(x => x.provider === providerId)
    const nextAuthTypes = (p?.authTypes ?? []).filter(type => type !== 'oauth2' || oauthConfigs.find(config => config.provider === p?.provider)?.configured)
    setAuthType(nextAuthTypes[0] ?? 'api_key')
    setValues({})
    setConnectionName(p?.provider === customMCPProviderID ? '' : 'default')
    stopDevicePoll()
    setDeviceSetup({ step: 'idle' })
  }, [providerId, providers, oauthConfigs, isEditing])

  useEffect(() => () => stopDevicePoll(), [])

  const fields = useMemo(() => {
    if (!provider || authType === 'no_auth') return []
    return provider.fields ?? []
  }, [provider, authType])

  async function submit() {
    if (!provider || provider.comingSoon) return
    setSaving(true)
    try {
      const cleanValues = Object.fromEntries(Object.entries(values).filter(([, value]) => value.trim() !== ''))
      const profile = {
        displayName: provider.provider === customMCPProviderID ? (connectionName.trim() || provider.displayName) : provider.displayName,
        ...(provider.provider === customMCPProviderID && cleanValues.serverUrl ? { serverUrl: cleanValues.serverUrl } : {}),
      }
      if (!connection && authType === 'oauth2') {
        const started = await apiPost<OAuthAuthorizationStart>('/api/v1/oauth/authorizations', {
          provider: provider.provider,
          ownerType,
          connectionName: connectionName.trim() || 'default',
          profile,
        })
        window.location.assign(started.authorizationUrl)
        return
      }
      const body = {
        provider: provider.provider,
        ownerType,
        authType,
        connectionName: connectionName.trim() || 'default',
        values: cleanValues,
        profile,
      }
      if (connection) {
        await apiPut(`/api/v1/connections/${encodeURIComponent(connection.id)}`, body)
      } else {
        await apiPost('/api/v1/connections', body)
      }
      onCreated()
    } finally {
      setSaving(false)
    }
  }

  function stopDevicePoll() {
    pollGenerationRef.current += 1
    if (pollRef.current) {
      clearTimeout(pollRef.current)
      pollRef.current = null
    }
  }

  async function beginDeviceSetup() {
    if (!provider || !canQuickAuthorize) return
    stopDevicePoll()
    setDeviceSetup({ step: 'beginning' })
    try {
      const res = await apiPost<DeviceSetupBegin>(`/api/v1/connectors/providers/${encodeURIComponent(provider.provider)}/setup/begin`, {})
      const next = {
        step: 'scanning' as const,
        deviceCode: res.deviceCode,
        qrUrl: res.qrUrl,
        userCode: res.userCode,
        baseUrl: res.baseUrl || '',
        interval: Math.max(3, res.interval || 5),
      }
      setDeviceSetup(next)
      window.open(res.qrUrl, '_blank', 'noopener,noreferrer')
      startDevicePoll(provider.provider, next)
    } catch (e) {
      setDeviceSetup({ step: 'error', message: e instanceof Error ? e.message : String(e) })
    }
  }

  function startDevicePoll(providerId: string, setup: Extract<typeof deviceSetup, { step: 'scanning' }>) {
    let interval = setup.interval
    const generation = pollGenerationRef.current
    const schedule = () => {
      if (pollGenerationRef.current !== generation) return
      pollRef.current = setTimeout(() => void tick(), interval * 1000)
    }
    const tick = async () => {
      if (pollGenerationRef.current !== generation) return
      try {
        const res = await apiPost<DeviceSetupPoll>(`/api/v1/connectors/providers/${encodeURIComponent(providerId)}/setup/poll`, {
          deviceCode: setup.deviceCode,
          baseUrl: setup.baseUrl || undefined,
        })
        if (res.slowDown) interval = Math.min(interval + 5, 30)
        if (res.status === 'authorize' && res.deviceCode && res.qrUrl) {
          const next = {
            step: 'scanning' as const,
            deviceCode: res.deviceCode,
            qrUrl: res.qrUrl,
            userCode: res.userCode,
            baseUrl: res.baseUrl || '',
            interval: Math.max(3, res.interval || interval),
          }
          setDeviceSetup(next)
          window.open(res.qrUrl, '_blank', 'noopener,noreferrer')
          stopDevicePoll()
          startDevicePoll(providerId, next)
        } else if (res.status === 'connected' && res.connection) {
          stopDevicePoll()
          setDeviceSetup({ step: 'connected' })
          onCreated()
        } else if (res.status === 'denied' || res.status === 'expired' || res.status === 'error') {
          stopDevicePoll()
          setDeviceSetup({ step: 'error', message: res.error || t(`connections.deviceAuth${res.status.charAt(0).toUpperCase()}${res.status.slice(1)}`) })
        } else {
          schedule()
        }
      } catch {
        // Keep polling through transient network errors.
        schedule()
      }
    }
    schedule()
  }

  return (
    <Modal title={isEditing ? t('connections.manageConnection') : t('connections.configureToolTitle', { name: provider?.displayName ?? '' })} onClose={onClose}>
      <div className="space-y-4">
        {!fixedProviderId && !isEditing && (
          <label className="block">
            <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">{t('connections.provider')}</span>
            <select className={selectCls} value={providerId} onChange={e => setProviderId(e.target.value)} disabled={isEditing}>
              {providers.filter(p => !p.comingSoon && p.provider !== customMCPProviderID).map(p => <option key={p.provider} value={p.provider}>{p.displayName}</option>)}
            </select>
          </label>
        )}
        {provider?.description && (
          <div className="rounded-lg bg-neutral-50 px-3 py-2 text-sm text-neutral-600 dark:bg-zinc-800/50 dark:text-zinc-300">
            {providerDescription(provider, t)}
          </div>
        )}
        {!isEditing && provider?.provider === customMCPProviderID && (
          <label className="block">
            <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">{t('connections.mcpToolName')} *</span>
            <input
              className={inputCls}
              value={connectionName}
              onChange={e => setConnectionName(e.target.value)}
              placeholder={t('connections.mcpToolNamePlaceholder')}
            />
          </label>
        )}
        {isEditing && connection && (
          <SavedConnectionSummary connection={connection} />
        )}
        {readonlyConnection && connection ? (
          <>
            <div className="rounded-lg border border-neutral-200 bg-white p-3 dark:border-zinc-700 dark:bg-zinc-900/40">
              <p className="text-sm font-medium text-neutral-900 dark:text-zinc-100">{t('connections.connected')}</p>
              <p className="mt-1 text-xs leading-5 text-neutral-500 dark:text-zinc-400">{t('connections.connectedReadonlyHint')}</p>
              {testState?.message && (
                <p className={cn('mt-2 truncate text-xs', testState.ok ? 'text-emerald-600 dark:text-emerald-300' : 'text-red-600 dark:text-red-300')}>
                  {testState.message}
                </p>
              )}
            </div>
            <div className="flex justify-between gap-2 pt-2">
              <button type="button" onClick={onClose} className="rounded-lg border border-neutral-300 px-3 py-2 text-sm dark:border-zinc-600">{t('common.close')}</button>
              <div className="flex gap-2">
                {isWorkspaceAdmin && connection.ownerType === 'workspace' && (
                  <button type="button" onClick={() => onInstallToProject?.(connection)} className="rounded-lg border border-neutral-300 px-3 py-2 text-sm font-medium text-neutral-700 hover:bg-neutral-50 dark:border-zinc-600 dark:text-zinc-200 dark:hover:bg-zinc-800">{t('connections.installToProject')}</button>
                )}
                <button type="button" onClick={() => onTest?.(connection)} disabled={testState?.loading} className="rounded-lg border border-neutral-300 px-3 py-2 text-sm font-medium text-neutral-700 hover:bg-neutral-50 disabled:opacity-50 dark:border-zinc-600 dark:text-zinc-200 dark:hover:bg-zinc-800">{t('connections.test')}</button>
                <button type="button" onClick={() => onDelete?.(connection)} className="rounded-lg border border-red-200 px-3 py-2 text-sm font-medium text-red-600 hover:bg-red-50 dark:border-red-900/60 dark:text-red-300 dark:hover:bg-red-900/20">{t('connections.disconnect')}</button>
              </div>
            </div>
          </>
        ) : (
          <>
        {isEditing && connection && (
          <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-neutral-200 bg-white p-3 dark:border-zinc-700 dark:bg-zinc-900/40">
            <p className="text-xs leading-5 text-neutral-500 dark:text-zinc-400">{t('connections.credentialsHiddenHint')}</p>
            <div className="flex flex-wrap gap-2">
              {isWorkspaceAdmin && connection.ownerType === 'workspace' && (
                <button type="button" onClick={() => onInstallToProject?.(connection)} className="rounded-lg border border-neutral-300 px-3 py-2 text-sm font-medium text-neutral-700 hover:bg-neutral-50 dark:border-zinc-600 dark:text-zinc-200 dark:hover:bg-zinc-800">{t('connections.installToProject')}</button>
              )}
              <button type="button" onClick={() => onTest?.(connection)} disabled={testState?.loading} className="rounded-lg border border-neutral-300 px-3 py-2 text-sm font-medium text-neutral-700 hover:bg-neutral-50 disabled:opacity-50 dark:border-zinc-600 dark:text-zinc-200 dark:hover:bg-zinc-800">{t('connections.test')}</button>
              <button type="button" onClick={() => onDelete?.(connection)} className="rounded-lg border border-red-200 px-3 py-2 text-sm font-medium text-red-600 hover:bg-red-50 dark:border-red-900/60 dark:text-red-300 dark:hover:bg-red-900/20">{t('connections.disconnect')}</button>
            </div>
            {testState?.message && (
              <p className={cn('w-full truncate text-xs', testState.ok ? 'text-emerald-600 dark:text-emerald-300' : 'text-red-600 dark:text-red-300')}>
                {testState.message}
              </p>
            )}
          </div>
        )}
        {canQuickAuthorize && (
          <div className="rounded-lg border border-sky-100 bg-sky-50 p-3 dark:border-sky-900/60 dark:bg-sky-950/20">
            <div className="flex items-start justify-between gap-3">
              <div>
                <p className="text-sm font-medium text-sky-900 dark:text-sky-100">{t('connections.quickAuthorize')}</p>
                <p className="mt-1 text-xs leading-5 text-sky-700 dark:text-sky-300">{t('connections.deviceAuthHint')}</p>
              </div>
              <button type="button" onClick={() => void beginDeviceSetup()} disabled={deviceSetup.step === 'beginning' || deviceSetup.step === 'scanning'} className="shrink-0 rounded-lg border border-sky-600 bg-white px-3 py-2 text-sm font-medium text-sky-700 hover:bg-sky-50 disabled:opacity-50 dark:border-sky-500 dark:bg-zinc-900 dark:text-sky-400 dark:hover:bg-zinc-800">
                {deviceSetup.step === 'beginning' || deviceSetup.step === 'scanning' ? t('common.loading') : t('connections.openAuthorizationPage')}
              </button>
            </div>
            {deviceSetup.step === 'scanning' && (
              <div className="mt-3 flex flex-col gap-3 rounded-lg bg-white p-3 dark:bg-zinc-900 sm:flex-row sm:items-center">
                <div className="rounded bg-white p-2">
                  <QRCodeSVG value={deviceSetup.qrUrl} size={104} />
                </div>
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium text-neutral-900 dark:text-zinc-100">{t('connections.waitingAuthorization')}</p>
                  {'userCode' in deviceSetup && deviceSetup.userCode ? (
                    <p className="mt-1 text-lg font-semibold tracking-wide text-neutral-900 dark:text-zinc-100">{deviceSetup.userCode}</p>
                  ) : null}
                  <a href={deviceSetup.qrUrl} target="_blank" rel="noreferrer" className="mt-1 block truncate text-xs font-medium text-sky-700 hover:underline dark:text-sky-300">
                    {deviceSetup.qrUrl}
                  </a>
                </div>
              </div>
            )}
            {deviceSetup.step === 'connected' && (
              <p className="mt-2 text-xs font-medium text-emerald-700 dark:text-emerald-300">{t('connections.authorizationConnected')}</p>
            )}
            {deviceSetup.step === 'error' && (
              <p className="mt-2 text-xs font-medium text-red-700 dark:text-red-300">{deviceSetup.message || t('connections.authorizationFailed')}</p>
            )}
          </div>
        )}
        {provider?.provider !== customMCPProviderID && (
          <label className="block">
            <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">{t('connections.authType')}</span>
            <select className={selectCls} value={authType} onChange={e => setAuthType(e.target.value)}>
              {availableAuthTypes.map(type => <option key={type} value={type}>{authTypeLabel(type, t)}</option>)}
            </select>
          </label>
        )}
        {provider?.authTypes.includes('oauth2') && !oauthConfig?.configured && (
          <p className="rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:bg-amber-950/30 dark:text-amber-300">
            {isWorkspaceAdmin ? t('connections.oauthNotConfiguredAdmin') : t('connections.oauthHiddenForUsers')}
          </p>
        )}
        {authType === 'oauth2' && (
          <p className="rounded-lg bg-sky-50 px-3 py-2 text-xs text-sky-700 dark:bg-sky-950/30 dark:text-sky-300">
            {t('connections.oauthStartHint')}
          </p>
        )}
        {fields.map(field => (
          <label key={field.key} className="block">
            <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">{providerFieldLabel(field, t)}{field.required ? ' *' : ''}</span>
            {field.inputType === 'textarea' ? (
              <textarea
                className={cn(inputCls, 'min-h-28 resize-y font-mono text-xs leading-5')}
                value={values[field.key] ?? ''}
                onChange={e => setValues(v => ({ ...v, [field.key]: e.target.value }))}
                placeholder={isEditing ? t('connections.keepCurrentValue') : ''}
              />
            ) : (
              <input
                type={field.secret ? 'password' : field.inputType === 'url' ? 'url' : 'text'}
                className={inputCls}
                value={values[field.key] ?? ''}
                onChange={e => setValues(v => ({ ...v, [field.key]: e.target.value }))}
                placeholder={isEditing ? t('connections.keepCurrentValue') : ''}
              />
            )}
          </label>
        ))}
        {(provider?.guides?.length ?? 0) > 0 && (
          <details className="rounded-lg border border-neutral-200 p-3 dark:border-zinc-700" open={!isEditing}>
            <summary className="cursor-pointer text-sm font-medium text-neutral-700 dark:text-zinc-200">{t('connections.credentialGuide')}</summary>
            <div className="mt-3 space-y-3">
              {(provider?.guides ?? []).map(guide => (
                <div key={guide.title}>
                  <p className="text-xs font-medium text-neutral-600 dark:text-zinc-300">{providerGuideTitle(guide, t)}</p>
                  <p className="mt-1 text-xs leading-5 text-neutral-500 dark:text-zinc-400">{providerGuideBody(guide, t)}</p>
                  {(guide.links ?? []).map(link => (
                    <a key={link.url} href={link.url} target="_blank" rel="noreferrer" className="mt-1 inline-block text-xs font-medium text-sky-700 hover:underline dark:text-sky-300">{providerGuideLinkLabel(link.label, t)}</a>
                  ))}
                </div>
              ))}
            </div>
          </details>
        )}
        <div className="flex justify-end gap-2 pt-2">
          <button type="button" onClick={onClose} disabled={saving} className="rounded-lg border border-neutral-300 px-3 py-2 text-sm dark:border-zinc-600">{t('common.cancel')}</button>
          <button type="button" onClick={() => void submit()} disabled={saving || !provider || provider.comingSoon || availableAuthTypes.length === 0 || (provider.provider === customMCPProviderID && !connectionName.trim())} className="rounded-lg bg-sky-600 px-3 py-2 text-sm font-medium text-white disabled:opacity-50">{authType === 'oauth2' && !isEditing ? t('connections.startOAuth') : isEditing ? t('common.save') : t('common.create')}</button>
        </div>
          </>
        )}
      </div>
    </Modal>
  )
}

function SavedConnectionSummary({ connection }: { connection: Connection }) {
  const { t } = useTranslation()
  const profile = connection.profile ?? {}
  const summary = connection.profileSummary
  const appId = typeof profile.appId === 'string' ? profile.appId : ''
  const baseUrl = typeof profile.baseUrl === 'string' ? profile.baseUrl : ''
  const serverUrl = typeof profile.serverUrl === 'string' ? profile.serverUrl : ''
  const ownerOpenId = typeof profile.ownerOpenId === 'string' ? profile.ownerOpenId : ''
  const fields = [
    { label: t('connections.appId'), value: appId },
    { label: t('connections.baseUrl'), value: baseUrl },
    { label: t('connections.mcpServerUrl'), value: serverUrl },
    { label: t('connections.openId'), value: ownerOpenId || summary?.accountId || '' },
    { label: t('connections.authType'), value: authTypeLabel(connection.authType, t) },
  ].filter(item => item.value)
  if (fields.length === 0) return null
  return (
    <div className="rounded-lg border border-neutral-200 bg-neutral-50 p-3 dark:border-zinc-700 dark:bg-zinc-900/50">
      <p className="text-xs font-medium text-neutral-500 dark:text-zinc-400">{t('connections.savedCredentialSummary')}</p>
      <div className="mt-2 grid gap-2 sm:grid-cols-2">
        {fields.map(item => (
          <div key={item.label} className="min-w-0">
            <p className="text-[11px] text-neutral-400 dark:text-zinc-500">{item.label}</p>
            <p className="truncate text-xs font-medium text-neutral-700 dark:text-zinc-200" title={item.value}>{item.value}</p>
          </div>
        ))}
      </div>
    </div>
  )
}

function Modal({ title, children, onClose }: { title: string; children: ReactNode; onClose: () => void }) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/45 p-4" onClick={onClose}>
      <div className="max-h-[88vh] w-full max-w-2xl overflow-y-auto rounded-xl border border-neutral-200 bg-white shadow-xl dark:border-zinc-700 dark:bg-zinc-900" onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between border-b border-neutral-200 px-5 py-3 dark:border-zinc-700">
          <h2 className="text-base font-semibold text-neutral-900 dark:text-zinc-100">{title}</h2>
          <button type="button" onClick={onClose} className="rounded-md p-1 text-neutral-400 hover:bg-neutral-100 dark:hover:bg-zinc-800">
            <X className="size-4" />
          </button>
        </div>
        <div className="p-5">{children}</div>
      </div>
    </div>
  )
}
