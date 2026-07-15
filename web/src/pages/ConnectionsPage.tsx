import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { Cable, KeyRound, Link2, Pencil, Plus, RefreshCw, ShieldCheck, Trash2, X } from 'lucide-react'
import { useSearchParams } from 'react-router-dom'
import { apiDelete, apiFetch, apiPost, apiPut } from '../lib/api'
import { useAuth } from '../lib/auth'
import { cn } from '../lib/cn'

type ProviderField = { key: string; label: string; inputType: string; required: boolean; secret: boolean }
type Provider = { provider: string; displayName: string; authTypes: string[]; fields?: ProviderField[]; enabled: boolean }
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
type ConnectionHealthCheckRunResult = { checked: number; skipped: number; results: Array<{ connectionId: string; ok: boolean; status: number; message: string; error?: string }> }
type OAuthAuthorizationStart = { authorizationUrl: string; state: string }
type ProjectRow = { name: string }
type ProjectAgent = { name: string }
type WorkspaceSummary = { id: string; name: string; currentUserRole?: string; currentUserCanAdmin?: boolean }
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

const inputCls = 'w-full rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm outline-none focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-100'
const selectCls = 'w-full rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm outline-none focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-100 dark:[color-scheme:dark]'

export default function ConnectionsPage() {
  const { user } = useAuth()
  const [workspace, setWorkspace] = useState<WorkspaceSummary | null>(null)
  const isWorkspaceAdmin = workspace?.currentUserCanAdmin ?? (!user || user.role === 'admin')
  const [providers, setProviders] = useState<Provider[]>([])
  const [oauthConfigs, setOAuthConfigs] = useState<OAuthClientConfig[]>([])
  const [connections, setConnections] = useState<Connection[]>([])
  const [projects, setProjects] = useState<ProjectRow[]>([])
  const [agentsByProject, setAgentsByProject] = useState<Record<string, ProjectAgent[]>>({})
  const [loading, setLoading] = useState(true)
  const [creating, setCreating] = useState(false)
  const [editing, setEditing] = useState<Connection | null>(null)
  const [granting, setGranting] = useState<Connection | null>(null)
  const [testResults, setTestResults] = useState<Record<string, { loading?: boolean; ok?: boolean; message?: string }>>({})
  const [healthCheckLoading, setHealthCheckLoading] = useState(false)
  const [healthCheckMessage, setHealthCheckMessage] = useState('')
  const [oauthMessage, setOauthMessage] = useState<{ kind: 'success' | 'error'; text: string } | null>(null)
  const [reloadKey, setReloadKey] = useState(0)
  const [searchParams, setSearchParams] = useSearchParams()

  useEffect(() => {
    const status = searchParams.get('oauth')
    if (status !== 'success' && status !== 'error') return
    const provider = searchParams.get('oauthProvider') || 'OAuth provider'
    const connectionName = searchParams.get('oauthConnection') || 'connection'
    if (status === 'success') {
      setOauthMessage({ kind: 'success', text: `${provider}/${connectionName} connected.` })
      setReloadKey(k => k + 1)
    } else {
      const message = searchParams.get('message') || 'OAuth authorization failed.'
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
        const [providerData, connectionData, projectData, workspaceData] = await Promise.all([
          apiFetch<{ providers: Provider[] }>('/api/v1/connectors/providers'),
          apiFetch<{ connections: Connection[] }>('/api/v1/connections'),
          apiFetch<ProjectRow[]>('/api/v1/projects').catch(() => []),
          apiFetch<WorkspaceSummary>('/api/v1/workspace').catch(() => null),
        ])
        const canAdminWorkspace = workspaceData?.currentUserCanAdmin ?? (!user || user.role === 'admin')
        const oauthData = canAdminWorkspace
          ? await apiFetch<{ configs: OAuthClientConfig[] }>('/api/v1/oauth/client-configs').catch(() => ({ configs: [] }))
          : { configs: [] }
        if (cancelled) return
        setWorkspace(workspaceData)
        setProviders(providerData.providers ?? [])
        setOAuthConfigs(oauthData.configs ?? [])
        setConnections(connectionData.connections ?? [])
        setProjects(projectData ?? [])
        const nextAgents: Record<string, ProjectAgent[]> = {}
        for (const project of projectData ?? []) {
          nextAgents[project.name] = await apiFetch<ProjectAgent[]>(`/api/v1/projects/${encodeURIComponent(project.name)}/agents`).catch(() => [])
        }
        if (!cancelled) setAgentsByProject(nextAgents)
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    void load()
    return () => { cancelled = true }
  }, [reloadKey])

  async function removeConnection(connection: Connection) {
    if (!window.confirm(`Remove ${connection.provider}/${connection.connectionName}?`)) return
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
          message: result.ok ? `OK · HTTP ${result.status}` : `${result.message || 'Failed'} · HTTP ${result.status}`,
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

  async function runHealthChecks() {
    setHealthCheckLoading(true)
    setHealthCheckMessage('')
    try {
      const result = await apiPost<ConnectionHealthCheckRunResult>('/api/v1/connections/health-check', { force: true, limit: 100 })
      setHealthCheckMessage(`Checked ${result.checked}; skipped ${result.skipped}.`)
    } catch (e) {
      setHealthCheckMessage(e instanceof Error ? e.message : String(e))
    } finally {
      setHealthCheckLoading(false)
      setReloadKey(k => k + 1)
    }
  }

  return (
    <div className="animate-fade-in px-8 py-6">
      <div className="flex items-center justify-between pb-5">
        <div>
          <h1 className="text-xl font-semibold text-neutral-900 dark:text-zinc-100">Connections</h1>
          <p className="mt-0.5 text-sm text-neutral-500 dark:text-zinc-500">Connect external tools and grant them to agents.</p>
        </div>
        <div className="flex items-center gap-2">
          <button type="button" onClick={() => setReloadKey(k => k + 1)} className="inline-flex items-center gap-2 rounded-lg border border-neutral-200 px-3 py-2 text-sm font-medium text-neutral-600 hover:bg-neutral-50 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800">
            <RefreshCw className="size-4" strokeWidth={1.8} />
            Refresh
          </button>
          {isWorkspaceAdmin && (
            <button type="button" onClick={() => void runHealthChecks()} disabled={healthCheckLoading} className="inline-flex items-center gap-2 rounded-lg border border-neutral-200 px-3 py-2 text-sm font-medium text-neutral-600 hover:bg-neutral-50 disabled:opacity-50 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800">
              <RefreshCw className={cn('size-4', healthCheckLoading && 'animate-spin')} strokeWidth={1.8} />
              Run checks
            </button>
          )}
          <button type="button" onClick={() => setCreating(true)} className="inline-flex items-center gap-2 rounded-lg bg-sky-600 px-3 py-2 text-sm font-medium text-white hover:bg-sky-700">
            <Plus className="size-4" strokeWidth={1.8} />
            New connection
          </button>
        </div>
      </div>
      {healthCheckMessage && (
        <div className="mb-4 rounded-lg border border-neutral-200 bg-white px-3 py-2 text-xs text-neutral-600 dark:border-zinc-700 dark:bg-zinc-900/50 dark:text-zinc-300">
          {healthCheckMessage}
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

      {isWorkspaceAdmin && oauthConfigs.length > 0 && (
        <OAuthClientConfigsPanel configs={oauthConfigs} onChanged={() => setReloadKey(k => k + 1)} />
      )}

      {loading ? (
        <div className="flex items-center justify-center gap-2 py-16 text-sm text-neutral-500">
          <div className="size-5 animate-spin rounded-full border-2 border-neutral-300 border-t-sky-600 dark:border-zinc-600 dark:border-t-sky-400" />
          Loading connections
        </div>
      ) : connections.length === 0 ? (
        <div className="rounded-xl border border-dashed border-neutral-300 bg-white p-12 text-center dark:border-zinc-700 dark:bg-zinc-900/40">
          <Cable className="mx-auto mb-3 size-10 text-neutral-300 dark:text-zinc-600" strokeWidth={1.5} />
          <p className="text-sm font-medium text-neutral-600 dark:text-zinc-300">No connections yet</p>
          <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">Create a workspace or personal connection to give agents safe access to tools.</p>
        </div>
      ) : (
        <div className="grid gap-4 xl:grid-cols-2">
          {connections.map(connection => (
            <ConnectionCard
              key={connection.id}
              connection={connection}
              provider={providers.find(p => p.provider === connection.provider)}
              canGrant={connection.ownerType === 'workspace' ? isWorkspaceAdmin : connection.ownerId === user?.username}
              canDelete={connection.ownerType === 'workspace' ? isWorkspaceAdmin : connection.ownerId === user?.username}
              canEdit={connection.ownerType === 'workspace' ? isWorkspaceAdmin : connection.ownerId === user?.username}
              testState={testResults[connection.id]}
              onEdit={() => setEditing(connection)}
              onGrant={() => setGranting(connection)}
              onTest={() => void testConnection(connection)}
              onDelete={() => void removeConnection(connection)}
            />
          ))}
        </div>
      )}

      {creating && (
        <ConnectionDialog
          providers={providers}
          isWorkspaceAdmin={isWorkspaceAdmin}
          onClose={() => setCreating(false)}
          onCreated={() => { setCreating(false); setReloadKey(k => k + 1) }}
        />
      )}
      {editing && (
        <ConnectionDialog
          providers={providers}
          isWorkspaceAdmin={isWorkspaceAdmin}
          connection={editing}
          onClose={() => setEditing(null)}
          onCreated={() => { setEditing(null); setReloadKey(k => k + 1) }}
        />
      )}
      {granting && (
        <GrantDialog
          connection={granting}
          workspaceId={workspace?.id ?? ''}
          projects={projects}
          agentsByProject={agentsByProject}
          isWorkspaceAdmin={isWorkspaceAdmin}
          currentUsername={user?.username ?? ''}
          linkedAgents={user?.linkedAgents ?? []}
          onClose={() => setGranting(null)}
          onChanged={() => { setGranting(null); setReloadKey(k => k + 1) }}
        />
      )}
    </div>
  )
}

function OAuthClientConfigsPanel({ configs, onChanged }: { configs: OAuthClientConfig[]; onChanged: () => void }) {
  const [editing, setEditing] = useState<OAuthClientConfig | null>(null)
  return (
    <section className="mb-5 rounded-xl border border-neutral-200/80 bg-white p-4 dark:border-zinc-700/60 dark:bg-zinc-900/40">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold text-neutral-900 dark:text-zinc-100">OAuth client configs</h2>
          <p className="mt-0.5 text-xs text-neutral-400 dark:text-zinc-500">Configure provider OAuth apps once per workspace. Client secrets stay server-side.</p>
        </div>
      </div>
      <div className="mt-3 grid gap-3 lg:grid-cols-2">
        {configs.map(config => (
          <div key={config.provider} className="rounded-lg border border-neutral-200 px-3 py-3 dark:border-zinc-700">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <p className="text-sm font-medium text-neutral-800 dark:text-zinc-100">{config.displayName}</p>
                  <span className={cn('rounded-full px-2 py-0.5 text-[11px] font-medium', config.configured ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300' : 'bg-neutral-100 text-neutral-500 dark:bg-zinc-800 dark:text-zinc-400')}>
                    {config.configured ? 'Configured' : 'Not configured'}
                  </span>
                </div>
                {config.clientId && <p className="mt-1 text-xs text-neutral-500 dark:text-zinc-400">Client ID: {config.clientId}</p>}
                <p className="mt-1 break-all font-mono text-[11px] text-neutral-400 dark:text-zinc-500">{config.expectedRedirectUri}</p>
                {(config.oauth?.scopes?.length ?? 0) > 0 && (
                  <div className="mt-2 flex flex-wrap gap-1.5">
                    {(config.oauth?.scopes ?? []).map(scope => (
                      <span key={scope} className="rounded-md bg-neutral-100 px-2 py-0.5 text-[11px] text-neutral-500 dark:bg-zinc-800 dark:text-zinc-400">{scope}</span>
                    ))}
                  </div>
                )}
              </div>
              <button type="button" onClick={() => setEditing(config)} className="rounded-md p-2 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-800 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100" title="Configure OAuth client">
                <Pencil className="size-4" strokeWidth={1.8} />
              </button>
            </div>
          </div>
        ))}
      </div>
      {editing && (
        <OAuthClientConfigDialog
          config={editing}
          onClose={() => setEditing(null)}
          onSaved={() => { setEditing(null); onChanged() }}
        />
      )}
    </section>
  )
}

function OAuthClientConfigDialog({ config, onClose, onSaved }: { config: OAuthClientConfig; onClose: () => void; onSaved: () => void }) {
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
    if (!window.confirm(`Remove OAuth client config for ${config.displayName}?`)) return
    await apiDelete(`/api/v1/oauth/client-configs/${encodeURIComponent(config.provider)}`)
    onSaved()
  }
  return (
    <Modal title={`OAuth client: ${config.displayName}`} onClose={onClose}>
      <div className="space-y-4">
        <label className="block">
          <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">Client ID</span>
          <input className={inputCls} value={clientId} onChange={e => setClientId(e.target.value)} />
        </label>
        <label className="block">
          <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">Client secret</span>
          <input className={inputCls} type="password" value={clientSecret} onChange={e => setClientSecret(e.target.value)} placeholder={config.configured ? 'Leave blank to keep current secret' : ''} />
        </label>
        <div className="rounded-lg bg-neutral-50 px-3 py-2 dark:bg-zinc-800/50">
          <p className="text-xs font-medium text-neutral-500 dark:text-zinc-400">Redirect URI</p>
          <p className="mt-1 break-all font-mono text-xs text-neutral-600 dark:text-zinc-300">{config.expectedRedirectUri}</p>
        </div>
        <div className="flex justify-between gap-2 pt-2">
          <button type="button" onClick={() => void remove()} disabled={saving || !config.configured} className="rounded-lg border border-red-200 px-3 py-2 text-sm text-red-600 disabled:opacity-50 dark:border-red-900/60 dark:text-red-300">Remove</button>
          <div className="flex gap-2">
            <button type="button" onClick={onClose} disabled={saving} className="rounded-lg border border-neutral-300 px-3 py-2 text-sm dark:border-zinc-600">Cancel</button>
            <button type="button" onClick={() => void submit()} disabled={saving || clientId.trim() === '' || (!config.configured && clientSecret.trim() === '')} className="rounded-lg bg-sky-600 px-3 py-2 text-sm font-medium text-white disabled:opacity-50">Save</button>
          </div>
        </div>
      </div>
    </Modal>
  )
}

function ConnectionCard({ connection, provider, canGrant, canEdit, canDelete, testState, onEdit, onGrant, onTest, onDelete }: { connection: Connection; provider?: Provider; canGrant: boolean; canEdit: boolean; canDelete: boolean; testState?: { loading?: boolean; ok?: boolean; message?: string }; onEdit: () => void; onGrant: () => void; onTest: () => void; onDelete: () => void }) {
  const validation = connectionValidation(connection)
  const health = connectionHealthPolicy(connection)
  const hasActionPolicy = connectionHasActionPolicy(connection)
  const summary = connection.profileSummary
  const accountLabel = [summary?.accountName, summary?.accountEmail].filter(Boolean).join(' · ')
  return (
    <section className="rounded-xl border border-neutral-200/80 bg-white p-5 dark:border-zinc-700/60 dark:bg-zinc-900/40">
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-start gap-3">
          <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-sky-100 dark:bg-sky-900/30">
            <KeyRound className="size-5 text-sky-700 dark:text-sky-300" strokeWidth={1.8} />
          </div>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="truncate text-sm font-semibold text-neutral-900 dark:text-zinc-100">{provider?.displayName ?? connection.provider}</h2>
              <span className="rounded-full bg-neutral-100 px-2 py-0.5 text-[11px] font-medium text-neutral-500 dark:bg-zinc-800 dark:text-zinc-400">{connection.connectionName}</span>
              <span className={cn('rounded-full px-2 py-0.5 text-[11px] font-medium', connection.ownerType === 'workspace' ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300' : 'bg-indigo-100 text-indigo-700 dark:bg-indigo-900/30 dark:text-indigo-300')}>
                {connection.ownerType}
              </span>
              {validation && (
                <span className={cn('rounded-full px-2 py-0.5 text-[11px] font-medium', validation.ok ? 'bg-teal-100 text-teal-700 dark:bg-teal-900/30 dark:text-teal-300' : 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300')}>
                  {validation.ok ? 'Healthy' : 'Failed'}{validation.status ? ` · ${validation.status}` : ''}
                </span>
              )}
              {health.enabled && (
                <span className="rounded-full bg-amber-100 px-2 py-0.5 text-[11px] font-medium text-amber-700 dark:bg-amber-900/30 dark:text-amber-300">
                  Auto-check · {health.intervalMinutes}m
                </span>
              )}
              {hasActionPolicy && (
                <span className="rounded-full bg-violet-100 px-2 py-0.5 text-[11px] font-medium text-violet-700 dark:bg-violet-900/30 dark:text-violet-300">
                  Action policy
                </span>
              )}
            </div>
            <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">{connection.authType} · owner {connection.ownerId}</p>
            {(summary?.accountId || accountLabel) && (
              <p className="mt-1 truncate text-xs text-neutral-500 dark:text-zinc-400" title={[summary?.accountId, accountLabel].filter(Boolean).join(' · ')}>
                {[summary?.accountId, accountLabel].filter(Boolean).join(' · ')}
              </p>
            )}
            {validation && (
              <p className="mt-1 truncate text-xs text-neutral-400 dark:text-zinc-500" title={validation.message || undefined}>
                Validated {validation.atLabel}{validation.message ? ` · ${validation.message}` : ''}
              </p>
            )}
            {health.enabled && health.nextLabel && (
              <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">Next check {health.nextLabel}</p>
            )}
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-1">
          <button type="button" onClick={onTest} disabled={testState?.loading}
            className="rounded-md p-2 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-800 disabled:opacity-50 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100" title="Test connection">
            <RefreshCw className={cn('size-4', testState?.loading && 'animate-spin')} strokeWidth={1.8} />
          </button>
          {canGrant && (
            <button type="button" onClick={onGrant} className="rounded-md p-2 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-800 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100" title="Manage grants">
              <ShieldCheck className="size-4" strokeWidth={1.8} />
            </button>
          )}
          {canEdit && (
            <button type="button" onClick={onEdit} className="rounded-md p-2 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-800 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100" title="Edit connection">
              <Pencil className="size-4" strokeWidth={1.8} />
            </button>
          )}
          {canDelete && (
            <button type="button" onClick={onDelete} className="rounded-md p-2 text-neutral-500 hover:bg-red-50 hover:text-red-600 dark:text-zinc-400 dark:hover:bg-red-900/20 dark:hover:text-red-300" title="Remove">
              <Trash2 className="size-4" strokeWidth={1.8} />
            </button>
          )}
        </div>
      </div>
      <div className="mt-4">
        <p className="text-xs font-medium text-neutral-400 dark:text-zinc-500">Grants</p>
        {connection.grants && connection.grants.length > 0 ? (
          <div className="mt-2 flex flex-wrap gap-1.5">
            {connection.grants.map(grant => (
              <span key={grant.id} className="rounded-md border border-neutral-200 px-2 py-1 text-xs text-neutral-600 dark:border-zinc-700 dark:text-zinc-300">
                {grant.targetType}: {grant.targetId}
              </span>
            ))}
          </div>
        ) : (
          <p className="mt-2 text-xs text-neutral-400 dark:text-zinc-500">No grants yet</p>
        )}
      </div>
      {((summary?.scopes?.length ?? 0) > 0 || (summary?.providerPermissions?.length ?? 0) > 0) && (
        <div className="mt-4 grid gap-3 sm:grid-cols-2">
          <ProfileChips title="Scopes" items={summary?.scopes ?? []} />
          <ProfileChips title="Permissions" items={summary?.providerPermissions ?? []} />
        </div>
      )}
      {testState?.message && (
        <div className={cn('mt-4 rounded-lg border px-3 py-2 text-xs',
          testState.ok
            ? 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900/60 dark:bg-emerald-950/30 dark:text-emerald-300'
            : 'border-red-200 bg-red-50 text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300'
        )}>
          {testState.message}
        </div>
      )}
    </section>
  )
}

function ProfileChips({ title, items }: { title: string; items: string[] }) {
  if (items.length === 0) return null
  return (
    <div>
      <p className="text-xs font-medium text-neutral-400 dark:text-zinc-500">{title}</p>
      <div className="mt-2 flex flex-wrap gap-1.5">
        {items.slice(0, 8).map(item => (
          <span key={item} className="rounded-md border border-neutral-200 px-2 py-1 text-xs text-neutral-600 dark:border-zinc-700 dark:text-zinc-300">{item}</span>
        ))}
        {items.length > 8 && <span className="rounded-md border border-neutral-200 px-2 py-1 text-xs text-neutral-400 dark:border-zinc-700">+{items.length - 8}</span>}
      </div>
    </div>
  )
}

function connectionValidation(connection: Connection): { ok: boolean; status?: number; message: string; atLabel: string } | null {
  const profile = connection.profile ?? {}
  const at = typeof profile.lastValidatedAt === 'string' ? profile.lastValidatedAt : ''
  if (!at) return null
  const ok = profile.lastValidationOK === true
  const status = typeof profile.lastValidationStatus === 'number' ? profile.lastValidationStatus : undefined
  const message = typeof profile.lastValidationMessage === 'string' ? profile.lastValidationMessage : ''
  const timestamp = new Date(at)
  const atLabel = Number.isNaN(timestamp.getTime()) ? at : timestamp.toLocaleString()
  return { ok, status, message, atLabel }
}

function connectionHealthPolicy(connection: Connection): { enabled: boolean; intervalMinutes: number; nextLabel: string } {
  const profile = connection.profile ?? {}
  const enabled = profile.healthCheckEnabled === true
  const rawInterval = typeof profile.healthCheckIntervalMinutes === 'number' ? profile.healthCheckIntervalMinutes : 360
  const intervalMinutes = Math.max(5, Math.min(43200, Math.round(rawInterval || 360)))
  const next = typeof profile.nextHealthCheckAt === 'string' ? profile.nextHealthCheckAt : ''
  const timestamp = next ? new Date(next) : null
  const nextLabel = timestamp && !Number.isNaN(timestamp.getTime()) ? timestamp.toLocaleString() : next
  return { enabled, intervalMinutes, nextLabel }
}

function profileStringList(profile: Record<string, unknown> | undefined, key: string): string[] {
  const raw = profile?.[key]
  if (Array.isArray(raw)) return raw.filter((item): item is string => typeof item === 'string').map(item => item.trim()).filter(Boolean)
  if (typeof raw === 'string') return raw.split(/[\n,]+/).map(item => item.trim()).filter(Boolean)
  return []
}

function connectionHasActionPolicy(connection: Connection): boolean {
  return ['allowedActionMethods', 'blockedActionMethods', 'allowedActionEndpoints', 'blockedActionEndpoints']
    .some(key => profileStringList(connection.profile, key).length > 0)
}

function parsePolicyList(value: string): string[] {
  return value.split(/[\n,]+/).map(item => item.trim()).filter(Boolean)
}

function ConnectionDialog({ providers, isWorkspaceAdmin, connection, onClose, onCreated }: { providers: Provider[]; isWorkspaceAdmin: boolean; connection?: Connection; onClose: () => void; onCreated: () => void }) {
  const isEditing = Boolean(connection)
  const [providerId, setProviderId] = useState(connection?.provider ?? providers[0]?.provider ?? '')
  const provider = providers.find(p => p.provider === providerId)
  const [ownerType, setOwnerType] = useState(connection?.ownerType ?? (isWorkspaceAdmin ? 'workspace' : 'user'))
  const [authType, setAuthType] = useState(connection?.authType ?? provider?.authTypes[0] ?? 'api_key')
  const [connectionName, setConnectionName] = useState(connection?.connectionName ?? 'default')
  const [displayName, setDisplayName] = useState(String(connection?.profile?.displayName ?? ''))
  const [accountId, setAccountId] = useState(String(connection?.profileSummary?.accountId ?? connection?.profile?.accountId ?? ''))
  const [accountName, setAccountName] = useState(String(connection?.profileSummary?.accountName ?? connection?.profile?.accountName ?? ''))
  const [accountEmail, setAccountEmail] = useState(String(connection?.profileSummary?.accountEmail ?? connection?.profile?.accountEmail ?? ''))
  const [scopes, setScopes] = useState((connection?.profileSummary?.scopes ?? profileStringList(connection?.profile, 'scopes')).join('\n'))
  const [providerPermissions, setProviderPermissions] = useState((connection?.profileSummary?.providerPermissions ?? profileStringList(connection?.profile, 'providerPermissions')).join('\n'))
  const [healthCheckEnabled, setHealthCheckEnabled] = useState(connection?.profile?.healthCheckEnabled === true)
  const [healthCheckIntervalMinutes, setHealthCheckIntervalMinutes] = useState(String(connectionHealthPolicy(connection ?? {} as Connection).intervalMinutes))
  const [allowedActionMethods, setAllowedActionMethods] = useState(profileStringList(connection?.profile, 'allowedActionMethods').join('\n'))
  const [blockedActionMethods, setBlockedActionMethods] = useState(profileStringList(connection?.profile, 'blockedActionMethods').join('\n'))
  const [allowedActionEndpoints, setAllowedActionEndpoints] = useState(profileStringList(connection?.profile, 'allowedActionEndpoints').join('\n'))
  const [blockedActionEndpoints, setBlockedActionEndpoints] = useState(profileStringList(connection?.profile, 'blockedActionEndpoints').join('\n'))
  const [values, setValues] = useState<Record<string, string>>({})
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (isEditing) return
    const p = providers.find(x => x.provider === providerId)
    setAuthType(p?.authTypes[0] ?? 'api_key')
    setValues({})
  }, [providerId, providers, isEditing])

  const fields = useMemo(() => {
    if (!provider || authType === 'no_auth') return []
    return provider.fields ?? []
  }, [provider, authType])

  async function submit() {
    if (!provider) return
    setSaving(true)
    try {
      const cleanValues = Object.fromEntries(Object.entries(values).filter(([, value]) => value.trim() !== ''))
      const profile = {
        displayName: displayName.trim() || provider.displayName,
        accountId: accountId.trim(),
        accountName: accountName.trim(),
        accountEmail: accountEmail.trim(),
        scopes: parsePolicyList(scopes),
        providerPermissions: parsePolicyList(providerPermissions),
        healthCheckEnabled,
        healthCheckIntervalMinutes: Math.max(5, Math.min(43200, Number(healthCheckIntervalMinutes) || 360)),
        allowedActionMethods: parsePolicyList(allowedActionMethods),
        blockedActionMethods: parsePolicyList(blockedActionMethods),
        allowedActionEndpoints: parsePolicyList(allowedActionEndpoints),
        blockedActionEndpoints: parsePolicyList(blockedActionEndpoints),
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

  return (
    <Modal title={isEditing ? 'Edit connection' : 'New connection'} onClose={onClose}>
      <div className="space-y-4">
        <label className="block">
          <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">Provider</span>
          <select className={selectCls} value={providerId} onChange={e => setProviderId(e.target.value)} disabled={isEditing}>
            {providers.map(p => <option key={p.provider} value={p.provider}>{p.displayName}</option>)}
          </select>
        </label>
        <div className="grid gap-3 sm:grid-cols-2">
          <label className="block">
            <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">Owner</span>
            <select className={selectCls} value={ownerType} onChange={e => setOwnerType(e.target.value)} disabled={isEditing}>
              {isWorkspaceAdmin && <option value="workspace">Workspace</option>}
              <option value="user">Me</option>
            </select>
          </label>
          <label className="block">
            <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">Auth type</span>
            <select className={selectCls} value={authType} onChange={e => setAuthType(e.target.value)}>
              {(provider?.authTypes ?? []).map(t => <option key={t} value={t}>{t}</option>)}
            </select>
          </label>
        </div>
        <div className="grid gap-3 sm:grid-cols-2">
          <label className="block">
            <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">Connection name</span>
            <input className={inputCls} value={connectionName} onChange={e => setConnectionName(e.target.value)} />
          </label>
          <label className="block">
            <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">Display name</span>
            <input className={inputCls} value={displayName} onChange={e => setDisplayName(e.target.value)} placeholder={provider?.displayName} />
          </label>
        </div>
        {authType === 'oauth2' && (
          <p className="rounded-lg bg-sky-50 px-3 py-2 text-xs text-sky-700 dark:bg-sky-950/30 dark:text-sky-300">
            Saving will start the provider OAuth flow. The callback creates this connection without exposing tokens to the browser.
          </p>
        )}
        {fields.map(field => (
          <label key={field.key} className="block">
            <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">{field.label}{field.required ? ' *' : ''}</span>
            <input
              type={field.secret ? 'password' : 'text'}
              className={inputCls}
              value={values[field.key] ?? ''}
              onChange={e => setValues(v => ({ ...v, [field.key]: e.target.value }))}
              placeholder={isEditing ? 'Leave blank to keep current value' : ''}
            />
          </label>
        ))}
        {isEditing && authType !== 'no_auth' && (
          <p className="rounded-lg bg-neutral-50 px-3 py-2 text-xs text-neutral-500 dark:bg-zinc-800/50 dark:text-zinc-400">
            Existing credential values are hidden. Fill only the fields you want to replace; leave all credential fields blank to keep the current secret.
          </p>
        )}
        <div className="rounded-lg border border-neutral-200 p-3 dark:border-zinc-700">
          <div>
            <p className="text-sm font-medium text-neutral-700 dark:text-zinc-200">Account profile</p>
            <p className="mt-0.5 text-xs text-neutral-400 dark:text-zinc-500">Safe identity and authorization metadata shown to humans and agent runtimes. Do not place secrets here.</p>
          </div>
          <div className="mt-3 grid gap-3 sm:grid-cols-3">
            <label className="block">
              <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">Account ID</span>
              <input className={inputCls} value={accountId} onChange={e => setAccountId(e.target.value)} />
            </label>
            <label className="block">
              <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">Account name</span>
              <input className={inputCls} value={accountName} onChange={e => setAccountName(e.target.value)} />
            </label>
            <label className="block">
              <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">Account email</span>
              <input className={inputCls} value={accountEmail} onChange={e => setAccountEmail(e.target.value)} />
            </label>
          </div>
          <div className="mt-3 grid gap-3 sm:grid-cols-2">
            <label className="block">
              <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">Granted scopes</span>
              <textarea className={`${inputCls} min-h-20 resize-y`} value={scopes} onChange={e => setScopes(e.target.value)} placeholder={'repo\nread:user'} />
            </label>
            <label className="block">
              <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">Provider permissions</span>
              <textarea className={`${inputCls} min-h-20 resize-y`} value={providerPermissions} onChange={e => setProviderPermissions(e.target.value)} placeholder={'Issues: read/write\nWiki: read'} />
            </label>
          </div>
        </div>
        <div className="rounded-lg border border-neutral-200 p-3 dark:border-zinc-700">
          <label className="flex items-center justify-between gap-3">
            <span>
              <span className="block text-sm font-medium text-neutral-700 dark:text-zinc-200">Automatic health checks</span>
              <span className="block text-xs text-neutral-400 dark:text-zinc-500">Multigent periodically validates this connection without exposing secrets to agents.</span>
            </span>
            <input type="checkbox" checked={healthCheckEnabled} onChange={e => setHealthCheckEnabled(e.target.checked)} className="size-4 accent-sky-600" />
          </label>
          {healthCheckEnabled && (
            <label className="mt-3 block">
              <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">Interval minutes</span>
              <input type="number" min={5} max={43200} className={inputCls} value={healthCheckIntervalMinutes} onChange={e => setHealthCheckIntervalMinutes(e.target.value)} />
            </label>
          )}
        </div>
        <div className="rounded-lg border border-neutral-200 p-3 dark:border-zinc-700">
          <div>
            <p className="text-sm font-medium text-neutral-700 dark:text-zinc-200">Runtime action policy</p>
            <p className="mt-0.5 text-xs text-neutral-400 dark:text-zinc-500">Optional allow/block rules for agent HTTP action proxy calls. Endpoint patterns support exact paths, <code>/prefix/*</code>, or <code>*</code>.</p>
          </div>
          <div className="mt-3 grid gap-3 sm:grid-cols-2">
            <label className="block">
              <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">Allowed methods</span>
              <textarea className={`${inputCls} min-h-20 resize-y`} value={allowedActionMethods} onChange={e => setAllowedActionMethods(e.target.value)} placeholder={'GET\nPOST'} />
            </label>
            <label className="block">
              <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">Blocked methods</span>
              <textarea className={`${inputCls} min-h-20 resize-y`} value={blockedActionMethods} onChange={e => setBlockedActionMethods(e.target.value)} placeholder={'DELETE'} />
            </label>
            <label className="block">
              <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">Allowed endpoints</span>
              <textarea className={`${inputCls} min-h-24 resize-y`} value={allowedActionEndpoints} onChange={e => setAllowedActionEndpoints(e.target.value)} placeholder={'/open-apis/wiki/*\n/repos/*'} />
            </label>
            <label className="block">
              <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">Blocked endpoints</span>
              <textarea className={`${inputCls} min-h-24 resize-y`} value={blockedActionEndpoints} onChange={e => setBlockedActionEndpoints(e.target.value)} placeholder={'/admin/*\n/private'} />
            </label>
          </div>
        </div>
        <div className="flex justify-end gap-2 pt-2">
          <button type="button" onClick={onClose} disabled={saving} className="rounded-lg border border-neutral-300 px-3 py-2 text-sm dark:border-zinc-600">Cancel</button>
          <button type="button" onClick={() => void submit()} disabled={saving || !provider} className="rounded-lg bg-sky-600 px-3 py-2 text-sm font-medium text-white disabled:opacity-50">{authType === 'oauth2' && !isEditing ? 'Start OAuth' : isEditing ? 'Save' : 'Create'}</button>
        </div>
      </div>
    </Modal>
  )
}

function GrantDialog({ connection, workspaceId, projects, agentsByProject, isWorkspaceAdmin, currentUsername, linkedAgents, onClose, onChanged }: { connection: Connection; workspaceId: string; projects: ProjectRow[]; agentsByProject: Record<string, ProjectAgent[]>; isWorkspaceAdmin: boolean; currentUsername: string; linkedAgents: string[]; onClose: () => void; onChanged: () => void }) {
  const isUserOwned = connection.ownerType === 'user'
  const isCurrentUserOwner = isUserOwned && connection.ownerId === currentUsername
  const canEditGrants = !isUserOwned || isCurrentUserOwner
  const initialTargetType = isUserOwned ? (isCurrentUserOwner && projects.length > 0 ? 'agent' : 'user') : (isWorkspaceAdmin ? 'workspace' : 'user')
  const [targetType, setTargetType] = useState(initialTargetType)
  const [project, setProject] = useState(projects[0]?.name ?? '')
  const [agent, setAgent] = useState('')
  const [targetId, setTargetId] = useState(initialTargetType === 'workspace' ? workspaceId : initialTargetType === 'user' ? (isUserOwned ? connection.ownerId : currentUsername) : '')
  const [saving, setSaving] = useState(false)

  const linkedAgentRefs = useMemo(() => linkedAgents.filter(ref => ref.includes('/')), [linkedAgents])
  const linkedAgentsByProject = useMemo(() => {
    const out: Record<string, ProjectAgent[]> = {}
    for (const ref of linkedAgentRefs) {
      const [proj, name] = ref.split('/', 2)
      if (!proj || !name) continue
      out[proj] = [...(out[proj] ?? []), { name }]
    }
    return out
  }, [linkedAgentRefs])
  const agentOptions = project ? (agentsByProject[project] ?? []) : []
  const projectOptions = projects.length > 0
    ? projects
    : Object.keys(linkedAgentsByProject).map(name => ({ name }))

  useEffect(() => {
    if (targetType === 'workspace') setTargetId(workspaceId)
    if (targetType === 'project') setTargetId(project)
    if (targetType === 'agent') setTargetId(project && agent ? `${project}/${agent}` : '')
    if (targetType === 'user') setTargetId(isUserOwned ? connection.ownerId : currentUsername)
  }, [targetType, project, agent, workspaceId, isUserOwned, connection.ownerId, currentUsername])

  useEffect(() => {
    if ((isUserOwned || !isWorkspaceAdmin) && projectOptions.length > 0 && !projectOptions.some(p => p.name === project)) {
      setProject(projectOptions[0].name)
      setAgent('')
    }
  }, [isUserOwned, isWorkspaceAdmin, projectOptions, project])

  async function addGrant() {
    setSaving(true)
    try {
      await apiPost(`/api/v1/connections/${encodeURIComponent(connection.id)}/grants`, {
        targetType,
        targetId: targetId.trim(),
      })
      onChanged()
    } finally {
      setSaving(false)
    }
  }

  async function removeGrant(grant: ConnectionGrant) {
    await apiDelete(`/api/v1/connections/${encodeURIComponent(connection.id)}/grants/${encodeURIComponent(grant.id)}`)
    onChanged()
  }

  return (
    <Modal title="Connection grants" onClose={onClose}>
      <div className="space-y-4">
        <div className="rounded-lg bg-neutral-50 p-3 text-sm text-neutral-600 dark:bg-zinc-800/50 dark:text-zinc-300">
          <p className="font-medium text-neutral-900 dark:text-zinc-100">{connection.provider} / {connection.connectionName}</p>
          <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">
            {isUserOwned ? 'Personal connections can only be granted by their owner to that owner or agents they can operate.' : 'Grant this workspace connection to a workspace, project, agent, or user.'}
          </p>
        </div>
        {canEditGrants && (
        <div className="grid gap-3 sm:grid-cols-[160px_minmax(0,1fr)]">
          <label className="block">
            <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">Target type</span>
            <select className={selectCls} value={targetType} onChange={e => setTargetType(e.target.value)}>
              {isWorkspaceAdmin && !isUserOwned && <option value="workspace">Workspace</option>}
              {isWorkspaceAdmin && !isUserOwned && <option value="project">Project</option>}
              {(isWorkspaceAdmin && !isUserOwned) || isCurrentUserOwner ? <option value="agent">Agent</option> : null}
              <option value="user">User</option>
            </select>
          </label>
          {targetType === 'workspace' ? (
            <label className="block">
              <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">Workspace ID</span>
              <input className={inputCls} value={targetId} readOnly />
            </label>
          ) : targetType === 'project' ? (
            <label className="block">
              <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">Project</span>
              <select className={selectCls} value={project} onChange={e => setProject(e.target.value)}>
                {projects.map(p => <option key={p.name} value={p.name}>{p.name}</option>)}
              </select>
            </label>
          ) : targetType === 'agent' ? (
            <div className="grid gap-3 sm:grid-cols-2">
              <label className="block">
                <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">Project</span>
                <select className={selectCls} value={project} onChange={e => { setProject(e.target.value); setAgent('') }}>
                  {projectOptions.map(p => <option key={p.name} value={p.name}>{p.name}</option>)}
                </select>
              </label>
              <label className="block">
                <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">Agent</span>
                <select className={selectCls} value={agent} onChange={e => setAgent(e.target.value)}>
                  <option value="">Select agent</option>
                  {agentOptions.map(a => <option key={a.name} value={a.name}>{a.name}</option>)}
                </select>
              </label>
            </div>
          ) : (
            <label className="block">
              <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">Username</span>
              <input className={inputCls} value={targetId} readOnly placeholder="username" />
            </label>
          )}
        </div>
        )}
        {canEditGrants && (
        <div className="flex justify-end">
          <button type="button" disabled={saving || targetId.trim() === ''} onClick={() => void addGrant()} className="inline-flex items-center gap-2 rounded-lg bg-sky-600 px-3 py-2 text-sm font-medium text-white disabled:opacity-50">
            <Link2 className="size-4" strokeWidth={1.8} />
            Add grant
          </button>
        </div>
        )}
        <div className="border-t border-neutral-200 pt-4 dark:border-zinc-700">
          {connection.grants && connection.grants.length > 0 ? (
            <div className="space-y-2">
              {connection.grants.map(grant => (
                <div key={grant.id} className="flex items-center justify-between rounded-lg border border-neutral-200 px-3 py-2 dark:border-zinc-700">
                  <span className="text-sm text-neutral-700 dark:text-zinc-300">{grant.targetType}: {grant.targetId}</span>
                  {canEditGrants && (
                    <button type="button" onClick={() => void removeGrant(grant)} className="rounded-md p-1.5 text-neutral-400 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20">
                      <Trash2 className="size-4" />
                    </button>
                  )}
                </div>
              ))}
            </div>
          ) : (
            <p className="py-3 text-center text-sm text-neutral-400 dark:text-zinc-500">No grants configured.</p>
          )}
        </div>
      </div>
    </Modal>
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
