import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { Cable, KeyRound, Link2, Plus, RefreshCw, ShieldCheck, Trash2, X } from 'lucide-react'
import { apiDelete, apiFetch, apiPost } from '../lib/api'
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
  grants?: ConnectionGrant[]
  createdBy?: string
  createdAt: string
  updatedAt?: string
}
type ProjectRow = { name: string }
type ProjectAgent = { name: string }
type WorkspaceSummary = { id: string; name: string }

const inputCls = 'w-full rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm outline-none focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-100'
const selectCls = 'w-full rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm outline-none focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-100 dark:[color-scheme:dark]'

export default function ConnectionsPage() {
  const { user } = useAuth()
  const isWorkspaceAdmin = !user || user.role === 'admin'
  const [providers, setProviders] = useState<Provider[]>([])
  const [connections, setConnections] = useState<Connection[]>([])
  const [projects, setProjects] = useState<ProjectRow[]>([])
  const [workspace, setWorkspace] = useState<WorkspaceSummary | null>(null)
  const [agentsByProject, setAgentsByProject] = useState<Record<string, ProjectAgent[]>>({})
  const [loading, setLoading] = useState(true)
  const [creating, setCreating] = useState(false)
  const [granting, setGranting] = useState<Connection | null>(null)
  const [reloadKey, setReloadKey] = useState(0)

  useEffect(() => {
    let cancelled = false
    async function load() {
      setLoading(true)
      try {
        const [providerData, connectionData, projectData] = await Promise.all([
          apiFetch<{ providers: Provider[] }>('/api/v1/connectors/providers'),
          apiFetch<{ connections: Connection[] }>('/api/v1/connections'),
          apiFetch<ProjectRow[]>('/api/v1/projects').catch(() => []),
        ])
        const workspaceData = await apiFetch<WorkspaceSummary>('/api/v1/workspace').catch(() => null)
        if (cancelled) return
        setWorkspace(workspaceData)
        setProviders(providerData.providers ?? [])
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
          <button type="button" onClick={() => setCreating(true)} className="inline-flex items-center gap-2 rounded-lg bg-sky-600 px-3 py-2 text-sm font-medium text-white hover:bg-sky-700">
            <Plus className="size-4" strokeWidth={1.8} />
            New connection
          </button>
        </div>
      </div>

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
              canDelete={isWorkspaceAdmin || connection.ownerId === user?.username}
              onGrant={() => setGranting(connection)}
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

function ConnectionCard({ connection, provider, canDelete, onGrant, onDelete }: { connection: Connection; provider?: Provider; canDelete: boolean; onGrant: () => void; onDelete: () => void }) {
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
            </div>
            <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">{connection.authType} · owner {connection.ownerId}</p>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-1">
          <button type="button" onClick={onGrant} className="rounded-md p-2 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-800 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100" title="Manage grants">
            <ShieldCheck className="size-4" strokeWidth={1.8} />
          </button>
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
    </section>
  )
}

function ConnectionDialog({ providers, isWorkspaceAdmin, onClose, onCreated }: { providers: Provider[]; isWorkspaceAdmin: boolean; onClose: () => void; onCreated: () => void }) {
  const [providerId, setProviderId] = useState(providers[0]?.provider ?? '')
  const provider = providers.find(p => p.provider === providerId)
  const [ownerType, setOwnerType] = useState(isWorkspaceAdmin ? 'workspace' : 'user')
  const [authType, setAuthType] = useState(provider?.authTypes[0] ?? 'api_key')
  const [connectionName, setConnectionName] = useState('default')
  const [displayName, setDisplayName] = useState('')
  const [values, setValues] = useState<Record<string, string>>({})
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    const p = providers.find(x => x.provider === providerId)
    setAuthType(p?.authTypes[0] ?? 'api_key')
    setValues({})
  }, [providerId, providers])

  const fields = useMemo(() => {
    if (!provider || authType === 'no_auth') return []
    return provider.fields ?? []
  }, [provider, authType])

  async function submit() {
    if (!provider) return
    setSaving(true)
    try {
      await apiPost('/api/v1/connections', {
        provider: provider.provider,
        ownerType,
        authType,
        connectionName: connectionName.trim() || 'default',
        values,
        profile: { displayName: displayName.trim() || provider.displayName },
      })
      onCreated()
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal title="New connection" onClose={onClose}>
      <div className="space-y-4">
        <label className="block">
          <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">Provider</span>
          <select className={selectCls} value={providerId} onChange={e => setProviderId(e.target.value)}>
            {providers.map(p => <option key={p.provider} value={p.provider}>{p.displayName}</option>)}
          </select>
        </label>
        <div className="grid gap-3 sm:grid-cols-2">
          <label className="block">
            <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">Owner</span>
            <select className={selectCls} value={ownerType} onChange={e => setOwnerType(e.target.value)}>
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
        {fields.map(field => (
          <label key={field.key} className="block">
            <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">{field.label}{field.required ? ' *' : ''}</span>
            <input
              type={field.secret ? 'password' : 'text'}
              className={inputCls}
              value={values[field.key] ?? ''}
              onChange={e => setValues(v => ({ ...v, [field.key]: e.target.value }))}
            />
          </label>
        ))}
        <div className="flex justify-end gap-2 pt-2">
          <button type="button" onClick={onClose} disabled={saving} className="rounded-lg border border-neutral-300 px-3 py-2 text-sm dark:border-zinc-600">Cancel</button>
          <button type="button" onClick={() => void submit()} disabled={saving || !provider} className="rounded-lg bg-sky-600 px-3 py-2 text-sm font-medium text-white disabled:opacity-50">Create</button>
        </div>
      </div>
    </Modal>
  )
}

function GrantDialog({ connection, workspaceId, projects, agentsByProject, isWorkspaceAdmin, currentUsername, linkedAgents, onClose, onChanged }: { connection: Connection; workspaceId: string; projects: ProjectRow[]; agentsByProject: Record<string, ProjectAgent[]>; isWorkspaceAdmin: boolean; currentUsername: string; linkedAgents: string[]; onClose: () => void; onChanged: () => void }) {
  const isUserOwned = connection.ownerType === 'user'
  const isCurrentUserOwner = isUserOwned && connection.ownerId === currentUsername
  const initialTargetType = isUserOwned ? (isCurrentUserOwner && linkedAgents.length > 0 ? 'agent' : 'user') : (isWorkspaceAdmin ? 'workspace' : 'user')
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
  const agentOptions = project ? (isUserOwned || !isWorkspaceAdmin ? (linkedAgentsByProject[project] ?? []) : (agentsByProject[project] ?? [])) : []
  const projectOptions = isUserOwned || !isWorkspaceAdmin
    ? Object.keys(linkedAgentsByProject).map(name => ({ name }))
    : projects

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
          <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">Grant this connection to a workspace, project, agent, or user.</p>
        </div>
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
        <div className="flex justify-end">
          <button type="button" disabled={saving || targetId.trim() === ''} onClick={() => void addGrant()} className="inline-flex items-center gap-2 rounded-lg bg-sky-600 px-3 py-2 text-sm font-medium text-white disabled:opacity-50">
            <Link2 className="size-4" strokeWidth={1.8} />
            Add grant
          </button>
        </div>
        <div className="border-t border-neutral-200 pt-4 dark:border-zinc-700">
          {connection.grants && connection.grants.length > 0 ? (
            <div className="space-y-2">
              {connection.grants.map(grant => (
                <div key={grant.id} className="flex items-center justify-between rounded-lg border border-neutral-200 px-3 py-2 dark:border-zinc-700">
                  <span className="text-sm text-neutral-700 dark:text-zinc-300">{grant.targetType}: {grant.targetId}</span>
                  <button type="button" onClick={() => void removeGrant(grant)} className="rounded-md p-1.5 text-neutral-400 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20">
                    <Trash2 className="size-4" />
                  </button>
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
