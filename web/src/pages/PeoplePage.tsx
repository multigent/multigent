import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { User, X, Copy } from 'lucide-react'
import { apiFetch, apiPost, apiPut } from '../lib/api'
import { cn } from '../lib/cn'
import { useFormatDateTime } from '../lib/format-datetime'

type PersonRow = {
  username: string; role: string; displayName?: string
  email?: string; avatar?: string; bio?: string
  projects?: { project: string; role: string }[]
  agentGrants?: { project: string; agent: string; role: string }[]
  linkedAgents?: string[]; disabled?: boolean; createdAt?: string
}

type ProjectRow = { name: string; description?: string }
type AgentRow = { name: string; model?: string }

type InvitationRow = {
  token: string
  email: string
  role: string
  displayName?: string
  invitedBy?: string
  status: string
  createdAt: string
  expiresAt: string
  acceptedAt?: string
}

type InvitationCreateResponse = {
  invitation?: InvitationRow
  inviteUrl?: string
  invitations?: {
    invitation: InvitationRow
    inviteUrl: string
    delivery?: string
  }[]
  errors?: {
    email: string
    error: string
  }[]
}

type UserLookupResponse = {
  email: string
  registered: boolean
  alreadyMember: boolean
  pendingInvite: boolean
  user?: {
    username: string
    displayName?: string
    email?: string
    avatar?: string
    disabled?: boolean
  }
}

const fieldCls =
  'w-full rounded-md border border-neutral-200/80 bg-neutral-50/50 px-3 py-2 text-sm outline-none transition-colors focus:border-sky-400 dark:border-zinc-700/60 dark:bg-zinc-800/50 dark:text-zinc-200 dark:[color-scheme:dark]'

function inviteLink(token: string): string {
  return `${window.location.origin}/invite/${encodeURIComponent(token)}`
}

function looksLikeEmail(value: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value.trim())
}

function unregisteredLookup(email: string): UserLookupResponse {
  return {
    email,
    registered: false,
    alreadyMember: false,
    pendingInvite: false,
  }
}

function statusKey(status: string): string {
  switch (status) {
    case 'pending':
    case 'accepted':
    case 'revoked':
    case 'rejected':
      return `people.inviteStatus_${status}`
    default:
      return 'people.inviteStatus_unknown'
  }
}

function roleKey(role: string): string {
  switch (role) {
    case 'owner':
    case 'admin':
    case 'member':
    case 'guest':
      return `people.workspaceRole_${role}`
    default:
      return 'people.workspaceRole_member'
  }
}

export default function PeoplePage() {
  const { t } = useTranslation()
  const fmtDateTime = useFormatDateTime()
  const [people, setPeople] = useState<PersonRow[]>([])
  const [invitations, setInvitations] = useState<InvitationRow[]>([])
  const [loading, setLoading] = useState(true)
  const [creating, setCreating] = useState(false)
  const [form, setForm] = useState({ email: '', role: 'member' })
  const [lookup, setLookup] = useState<UserLookupResponse | null>(null)
  const [lookupLoading, setLookupLoading] = useState(false)
  const [currentWorkspaceRole, setCurrentWorkspaceRole] = useState('')
  const [inviteUrl, setInviteUrl] = useState('')
  const [inviteResults, setInviteResults] = useState<{ email: string; inviteUrl?: string; delivery?: string; error?: string }[]>([])
  const [saving, setSaving] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const [accessEditing, setAccessEditing] = useState<PersonRow | null>(null)
  const [accessProjects, setAccessProjects] = useState<{ project: string; role: string }[]>([])
  const [accessAgents, setAccessAgents] = useState<{ project: string; agent: string; role: string }[]>([])
  const [projectCatalog, setProjectCatalog] = useState<ProjectRow[]>([])
  const [agentsByProject, setAgentsByProject] = useState<Record<string, AgentRow[]>>({})

  const refresh = useCallback(async () => {
    try {
      const [data, inviteData] = await Promise.all([
        apiFetch<PersonRow[]>('/api/v1/users'),
        apiFetch<{ invitations: InvitationRow[] }>('/api/v1/invitations').catch(() => ({ invitations: [] })),
      ])
      setPeople(data ?? [])
      setInvitations(inviteData.invitations ?? [])
      apiFetch<{ currentUserRole?: string }>('/api/v1/workspace')
        .then(workspace => setCurrentWorkspaceRole(workspace.currentUserRole ?? ''))
        .catch(() => setCurrentWorkspaceRole(''))
    } catch { /* ignore */ }
    finally { setLoading(false) }
  }, [])

  useEffect(() => { void refresh() }, [refresh])

  useEffect(() => {
    const email = form.email.trim()
    setLookup(null)
    if (!looksLikeEmail(email)) {
      setLookupLoading(false)
      return
    }
    let cancelled = false
    setLookupLoading(true)
    const timer = window.setTimeout(() => {
      apiFetch<UserLookupResponse>(`/api/v1/users/lookup?email=${encodeURIComponent(email)}`)
        .then((res) => {
          if (!cancelled) setLookup(res)
        })
        .catch(() => {
          if (!cancelled) setLookup(unregisteredLookup(email))
        })
        .finally(() => {
          if (!cancelled) setLookupLoading(false)
        })
    }, 350)
    return () => {
      cancelled = true
      window.clearTimeout(timer)
    }
  }, [form.email])

  async function lookupEmail() {
    const email = form.email.trim()
    if (!looksLikeEmail(email)) {
      setLookup(null)
      return
    }
    setLookupLoading(true)
    setErr(null)
    try {
      const res = await apiFetch<UserLookupResponse>(`/api/v1/users/lookup?email=${encodeURIComponent(email)}`)
      setLookup(res)
    } catch {
      setLookup(unregisteredLookup(email))
    } finally {
      setLookupLoading(false)
    }
  }

  async function handleCreate() {
    if (!form.email.trim()) return
    setSaving(true); setErr(null)
    try {
      const emails = form.email.split(/[\s,;]+/).map(v => v.trim()).filter(Boolean)
      const res = await apiPost<InvitationCreateResponse>('/api/v1/invitations', {
        emails,
        role: form.role,
      })
      setInviteUrl(res.invitation?.token ? inviteLink(res.invitation.token) : (res.inviteUrl ?? ''))
      setInviteResults([
        ...(res.invitations ?? []).map(item => ({
          email: item.invitation.email,
          inviteUrl: inviteLink(item.invitation.token),
          delivery: item.delivery,
        })),
        ...(res.errors ?? []).map(item => ({
          email: item.email,
          error: item.error,
        })),
      ])
      setForm({ email: '', role: 'member' })
      setLookup(null)
      await refresh()
    } catch (e) { setErr(e instanceof Error ? e.message : String(e)) }
    finally { setSaving(false) }
  }

  const inviteRoleOptions = currentWorkspaceRole === 'owner'
    ? ['owner', 'admin', 'member', 'guest']
    : ['admin', 'member', 'guest']

  async function revokeInvite(token: string) {
    setErr(null)
    try {
      await apiPost(`/api/v1/invitations/${encodeURIComponent(token)}/revoke`, {})
      await refresh()
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    }
  }

  async function updateMemberRole(username: string, role: string) {
    setErr(null)
    try {
      await apiPut(`/api/v1/users/${encodeURIComponent(username)}/workspace-role`, { role })
      await refresh()
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    }
  }

  async function openAccessEditor(person: PersonRow) {
    setAccessEditing(person)
    setAccessProjects([...(person.projects ?? [])])
    setAccessAgents([...(person.agentGrants ?? [])])
    setErr(null)
    try {
      const projects = await apiFetch<ProjectRow[]>('/api/v1/projects')
      setProjectCatalog(projects ?? [])
      const pairs = await Promise.all((projects ?? []).map(async project => {
        const agents = await apiFetch<AgentRow[]>(`/api/v1/projects/${encodeURIComponent(project.name)}/agents`).catch(() => [] as AgentRow[])
        return [project.name, agents] as const
      }))
      setAgentsByProject(Object.fromEntries(pairs))
    } catch {
      setProjectCatalog([])
      setAgentsByProject({})
    }
  }

  async function saveAccess() {
    if (!accessEditing) return
    setSaving(true); setErr(null)
    try {
      await apiPut(`/api/v1/users/${encodeURIComponent(accessEditing.username)}`, {
        projects: accessProjects.filter(p => p.project.trim()).map(p => ({ project: p.project, role: p.role || 'viewer' })),
        agentGrants: accessAgents.filter(a => a.project.trim() && a.agent.trim()).map(a => ({ project: a.project, agent: a.agent, role: a.role || 'operator' })),
      })
      setAccessEditing(null)
      await refresh()
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    } finally {
      setSaving(false)
    }
  }

  function addProjectGrant() {
    const first = projectCatalog.find(p => !accessProjects.some(ap => ap.project === p.name))
    if (!first) return
    setAccessProjects([...accessProjects, { project: first.name, role: 'viewer' }])
  }

  function addAgentGrant() {
    const project = projectCatalog[0]?.name ?? ''
    const agent = project ? (agentsByProject[project]?.[0]?.name ?? '') : ''
    setAccessAgents([...accessAgents, { project, agent, role: 'operator' }])
  }

  return (
    <div className="animate-fade-in px-8 py-6">
      <div className="flex items-center justify-between pb-5">
        <div>
          <h1 className="text-xl font-semibold text-neutral-900 dark:text-zinc-100">{t('people.title')}</h1>
          <p className="mt-0.5 text-sm text-neutral-500 dark:text-zinc-500">{t('people.subtitle')}</p>
        </div>
        <button type="button" onClick={() => { setCreating(true); setErr(null); setInviteUrl(''); setInviteResults([]); setLookup(null) }}
          className="rounded-lg border border-sky-600 bg-white px-3 py-2 text-sm font-medium text-sky-700 hover:bg-sky-50 dark:border-sky-500 dark:bg-zinc-900 dark:text-sky-400 dark:hover:bg-zinc-800">
          {t('people.add')}
        </button>
      </div>

      {loading ? (
        <p className="py-12 text-center text-sm text-neutral-400">{t('forms.loading')}</p>
      ) : people.length === 0 ? (
        <div className="rounded-xl border border-dashed border-neutral-300 bg-white p-12 text-center dark:border-zinc-700 dark:bg-zinc-900/40">
          <User className="mx-auto mb-3 size-10 text-neutral-300 dark:text-zinc-600" strokeWidth={1.2} />
          <p className="text-sm font-medium text-neutral-500 dark:text-zinc-400">{t('people.empty')}</p>
          <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">{t('people.emptyHint')}</p>
        </div>
      ) : (
        <div className="overflow-x-auto rounded-xl border border-neutral-200/80 bg-white dark:border-zinc-700/60 dark:bg-zinc-900/40">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-neutral-200 bg-neutral-50 text-xs font-medium uppercase tracking-wide text-neutral-500 dark:border-zinc-800 dark:bg-zinc-900 dark:text-zinc-500">
              <tr>
                <th className="px-4 py-3">{t('people.columnUser')}</th>
                <th className="px-4 py-3">{t('users.email')}</th>
                <th className="px-4 py-3">{t('people.columnRole')}</th>
                <th className="px-4 py-3">{t('people.columnProjects')}</th>
                <th className="px-4 py-3">{t('people.columnAgents')}</th>
                <th className="px-4 py-3">{t('people.columnCreated')}</th>
                <th className="px-4 py-3">{t('people.columnStatus')}</th>
                <th className="px-4 py-3 text-right">{t('people.columnActions')}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-neutral-100 dark:divide-zinc-800">
              {people.map(p => (
                <tr key={p.username} className={cn('hover:bg-neutral-50 dark:hover:bg-zinc-800/50', p.disabled && 'opacity-60')}>
                  <td className="px-4 py-3">
                    <div className="flex items-center gap-2.5">
                      {p.avatar ? (
                        <img src={p.avatar} alt="" className="size-8 rounded-full object-cover ring-1 ring-neutral-200 dark:ring-zinc-700" />
                      ) : (
                        <span className="flex size-8 items-center justify-center rounded-full bg-sky-600 text-xs font-semibold text-white">
                          {(p.displayName || p.email || p.username || 'U').slice(0, 1).toUpperCase()}
                        </span>
                      )}
                      <span className="font-medium text-neutral-900 dark:text-zinc-100">
                        {p.displayName || p.email || p.username}
                      </span>
                    </div>
                  </td>
                  <td className="px-4 py-3 text-neutral-600 dark:text-zinc-400">{p.email || '-'}</td>
                  <td className="px-4 py-3">
                    <select
                      value={p.role}
                      onChange={(e) => void updateMemberRole(p.username, e.target.value)}
                      disabled={p.role === 'owner' && currentWorkspaceRole !== 'owner'}
                      className="rounded-md border border-neutral-200 bg-white px-2 py-1 text-xs font-medium text-neutral-700 outline-none focus:border-sky-400 disabled:opacity-60 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300 dark:[color-scheme:dark]"
                    >
                      {(currentWorkspaceRole === 'owner' ? ['owner', 'admin', 'member', 'guest'] : p.role === 'owner' ? ['owner'] : ['admin', 'member', 'guest']).map(role => (
                        <option key={role} value={role}>{t(roleKey(role))}</option>
                      ))}
                    </select>
                  </td>
                  <td className="px-4 py-3 text-neutral-600 dark:text-zinc-400">{p.projects?.length ?? 0}</td>
                  <td className="px-4 py-3 text-neutral-600 dark:text-zinc-400">{p.agentGrants?.length ?? 0}</td>
                  <td className="px-4 py-3 text-neutral-500 dark:text-zinc-500">{fmtDateTime(p.createdAt)}</td>
                  <td className="px-4 py-3">
                    <span className={cn('rounded-full px-2 py-0.5 text-xs font-medium', p.disabled ? 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300' : 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300')}>
                      {p.disabled ? t('users.disabled') : t('people.statusActive')}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-right">
                    <button type="button" onClick={() => void openAccessEditor(p)} className="rounded px-2 py-1 text-xs font-medium text-sky-700 hover:bg-sky-50 dark:text-sky-300 dark:hover:bg-sky-900/20">
                      {t('people.manageAccess')}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {invitations.length > 0 && (
        <section className="mt-6">
          <div className="mb-3 flex items-center justify-between">
            <h2 className="text-base font-semibold text-neutral-900 dark:text-zinc-100">{t('people.invitations')}</h2>
          </div>
          <div className="overflow-x-auto rounded-xl border border-neutral-200/80 bg-white dark:border-zinc-700/60 dark:bg-zinc-900/40">
            <table className="w-full text-left text-sm">
              <thead className="border-b border-neutral-200 bg-neutral-50 text-xs font-medium uppercase tracking-wide text-neutral-500 dark:border-zinc-800 dark:bg-zinc-900 dark:text-zinc-500">
                <tr>
                  <th className="px-4 py-3">{t('users.email')}</th>
                  <th className="px-4 py-3">{t('people.columnRole')}</th>
                  <th className="px-4 py-3">{t('people.columnStatus')}</th>
                  <th className="px-4 py-3">{t('people.columnCreated')}</th>
                  <th className="px-4 py-3">{t('people.columnExpires')}</th>
                  <th className="px-4 py-3 text-right">{t('people.columnActions')}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-neutral-100 dark:divide-zinc-800">
                {invitations.map(inv => {
                  const link = inviteLink(inv.token)
                  return (
                    <tr key={inv.token} className="hover:bg-neutral-50 dark:hover:bg-zinc-800/50">
                      <td className="px-4 py-3 font-medium text-neutral-900 dark:text-zinc-100">{inv.email}</td>
                      <td className="px-4 py-3">
                        <span className="rounded-full bg-neutral-100 px-2 py-0.5 text-xs font-medium text-neutral-600 dark:bg-zinc-800 dark:text-zinc-300">{t(roleKey(inv.role))}</span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={cn('rounded-full px-2 py-0.5 text-xs font-medium', inv.status === 'pending' ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300' : inv.status === 'accepted' ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300' : 'bg-neutral-100 text-neutral-500 dark:bg-zinc-800 dark:text-zinc-400')}>
                          {t(statusKey(inv.status))}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-neutral-500 dark:text-zinc-500">{fmtDateTime(inv.createdAt)}</td>
                      <td className="px-4 py-3 text-neutral-500 dark:text-zinc-500">{fmtDateTime(inv.expiresAt)}</td>
                      <td className="px-4 py-3">
                        <div className="flex justify-end gap-1.5">
                          <button type="button" onClick={() => navigator.clipboard?.writeText(link)} className="rounded px-2 py-1 text-xs font-medium text-sky-700 hover:bg-sky-50 dark:text-sky-300 dark:hover:bg-sky-900/20">
                            {t('people.copyInviteLink')}
                          </button>
                          {inv.status === 'pending' && (
                            <button type="button" onClick={() => void revokeInvite(inv.token)} className="rounded px-2 py-1 text-xs font-medium text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20">
                              {t('people.revokeInvite')}
                            </button>
                          )}
                        </div>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </section>
      )}

      {/* Create Person Dialog */}
      {creating && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/45 p-4" onClick={() => !saving && setCreating(false)}>
          <div className="w-full max-w-md rounded-xl border border-neutral-200 bg-white shadow-lg dark:border-zinc-700 dark:bg-zinc-900 animate-scale-in" onClick={e => e.stopPropagation()}>
            <div className="flex items-center justify-between border-b border-neutral-200 px-5 py-3 dark:border-zinc-700">
              <h2 className="text-base font-semibold text-neutral-900 dark:text-zinc-100">{t('people.invite')}</h2>
              <button type="button" onClick={() => setCreating(false)} className="rounded-md p-1 text-neutral-400 hover:bg-neutral-100 dark:hover:bg-zinc-800"><X className="size-4" /></button>
            </div>
            <div className="space-y-3 px-5 py-4">
              <label className="flex flex-col gap-1">
                <span className="text-sm font-medium text-neutral-600 dark:text-zinc-400">{t('users.email')} *</span>
                <div className="flex gap-2">
                  <input value={form.email} onChange={e => { setForm({ ...form, email: e.target.value }); setErr(null) }} onBlur={() => void lookupEmail()} className={fieldCls} placeholder="alice@example.com" autoFocus />
                </div>
                <span className="text-xs text-neutral-400 dark:text-zinc-500">{lookupLoading ? t('people.checkingEmail') : t('people.emailSearchHint')}</span>
              </label>
              {lookup && (
                <div className={cn('rounded-lg border p-3 text-sm', lookup.alreadyMember
                  ? 'border-amber-200 bg-amber-50 text-amber-800 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-300'
                  : 'border-sky-200 bg-sky-50 text-sky-800 dark:border-sky-900/50 dark:bg-sky-900/20 dark:text-sky-300')}>
                  <p className="font-medium">
                    {lookup.alreadyMember
                      ? t('people.lookupAlreadyMember')
                      : lookup.registered
                        ? t('people.lookupRegistered')
                        : t('people.lookupUnregistered')}
                  </p>
                  {lookup.user && (
                    <p className="mt-1 text-xs opacity-80">
                      {(lookup.user.displayName || lookup.user.username)} · {lookup.user.email}
                    </p>
                  )}
                  {lookup.pendingInvite && <p className="mt-1 text-xs opacity-80">{t('people.lookupPendingInvite')}</p>}
                </div>
              )}
              <label className="flex flex-col gap-1">
                <span className="text-sm font-medium text-neutral-600 dark:text-zinc-400">{t('people.workspaceRole')}</span>
                <select value={form.role} onChange={e => setForm({ ...form, role: e.target.value })} className={fieldCls}>
                  {inviteRoleOptions.map(role => (
                    <option key={role} value={role}>{t(`people.workspaceRole_${role}`)}</option>
                  ))}
                </select>
              </label>
              {err && <p className="text-sm text-red-600 dark:text-red-400">{err}</p>}
              {inviteUrl && (
                <div className="rounded-lg border border-sky-200 bg-sky-50 p-3 dark:border-sky-900/50 dark:bg-sky-900/20">
                  <p className="mb-2 text-xs text-sky-700 dark:text-sky-300">{t('people.inviteLinkHint')}</p>
                  <div className="flex items-center gap-2">
                    <input readOnly value={inviteUrl} className={cn(fieldCls, 'flex-1 font-mono text-xs')} />
                    <button type="button" onClick={() => navigator.clipboard?.writeText(inviteUrl)} className="rounded p-2 text-sky-700 hover:bg-sky-100 dark:text-sky-300 dark:hover:bg-sky-900/30">
                      <Copy className="size-4" />
                    </button>
                  </div>
                </div>
              )}
              {inviteResults.length > 0 && (
                <div className="max-h-56 space-y-2 overflow-auto rounded-lg border border-neutral-200 bg-neutral-50 p-3 dark:border-zinc-700 dark:bg-zinc-800/40">
                  <p className="text-xs font-medium text-neutral-500 dark:text-zinc-400">{t('people.inviteResults')}</p>
                  {inviteResults.map((item, idx) => (
                    <div key={`${item.email}-${idx}`} className="rounded-md bg-white p-2 dark:bg-zinc-900/60">
                      <div className="flex items-center justify-between gap-2">
                        <p className="truncate text-xs font-medium text-neutral-700 dark:text-zinc-300">{item.email}</p>
                        {item.delivery && (
                          <span className="shrink-0 rounded bg-neutral-100 px-1.5 py-0.5 text-[10px] font-medium text-neutral-500 dark:bg-zinc-800 dark:text-zinc-400">{item.delivery}</span>
                        )}
                      </div>
                      {item.error ? (
                        <p className="mt-1 text-xs text-red-600 dark:text-red-400">{item.error}</p>
                      ) : (
                        <div className="mt-1 flex items-center gap-2">
                          <input readOnly value={item.inviteUrl ?? ''} className={cn(fieldCls, 'flex-1 py-1.5 font-mono text-xs')} />
                          <button type="button" onClick={() => navigator.clipboard?.writeText(item.inviteUrl ?? '')} className="rounded p-1.5 text-sky-700 hover:bg-sky-100 dark:text-sky-300 dark:hover:bg-sky-900/30">
                            <Copy className="size-4" />
                          </button>
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              )}
              <div className="flex justify-end gap-2 pt-1">
                <button type="button" onClick={() => setCreating(false)} disabled={saving} className="rounded-lg border border-neutral-300 px-3 py-1.5 text-sm dark:border-zinc-600">{t('forms.cancel')}</button>
                <button type="button" onClick={() => void handleCreate()} disabled={saving || !looksLikeEmail(form.email) || Boolean(lookup?.alreadyMember) || Boolean(lookup?.pendingInvite)}
                  className="rounded-lg bg-sky-600 px-3 py-1.5 text-sm font-medium text-white disabled:opacity-50">
                  {saving ? t('forms.saving') : t('people.sendInvite')}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {accessEditing && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/45 p-4" onClick={() => !saving && setAccessEditing(null)}>
          <div className="w-full max-w-2xl rounded-xl border border-neutral-200 bg-white shadow-lg dark:border-zinc-700 dark:bg-zinc-900 animate-scale-in" onClick={e => e.stopPropagation()}>
            <div className="flex items-center justify-between border-b border-neutral-200 px-5 py-3 dark:border-zinc-700">
              <div>
                <h2 className="text-base font-semibold text-neutral-900 dark:text-zinc-100">{t('people.accessTitle', { name: accessEditing.displayName || accessEditing.email || accessEditing.username })}</h2>
                <p className="mt-0.5 text-xs text-neutral-500 dark:text-zinc-500">{t('people.accessHint')}</p>
              </div>
              <button type="button" onClick={() => setAccessEditing(null)} className="rounded-md p-1 text-neutral-400 hover:bg-neutral-100 dark:hover:bg-zinc-800"><X className="size-4" /></button>
            </div>
            <div className="max-h-[70vh] space-y-5 overflow-auto px-5 py-4">
              <section>
                <div className="mb-2 flex items-center justify-between">
                  <h3 className="text-sm font-semibold text-neutral-900 dark:text-zinc-100">{t('people.projectAccess')}</h3>
                  <button type="button" onClick={addProjectGrant} disabled={projectCatalog.length === 0} className="rounded-lg border border-sky-600 bg-white px-3 py-1.5 text-xs font-medium text-sky-700 hover:bg-sky-50 disabled:opacity-50 dark:border-sky-500 dark:bg-zinc-900 dark:text-sky-400 dark:hover:bg-zinc-800">
                    {t('people.addProjectAccess')}
                  </button>
                </div>
                <div className="space-y-2">
                  {accessProjects.length === 0 ? (
                    <p className="rounded-lg border border-dashed border-neutral-200 py-4 text-center text-xs text-neutral-400 dark:border-zinc-700 dark:text-zinc-500">{t('people.noProjectAccess')}</p>
                  ) : accessProjects.map((grant, idx) => (
                    <div key={`${grant.project}-${idx}`} className="grid grid-cols-[1fr_150px_auto] gap-2">
                      <select value={grant.project} onChange={e => {
                        const next = [...accessProjects]; next[idx] = { ...next[idx], project: e.target.value }
                        setAccessProjects(next)
                      }} className={fieldCls}>
                        {projectCatalog.map(project => <option key={project.name} value={project.name}>{project.name}</option>)}
                      </select>
                      <select value={grant.role} onChange={e => {
                        const next = [...accessProjects]; next[idx] = { ...next[idx], role: e.target.value }
                        setAccessProjects(next)
                      }} className={fieldCls}>
                        {['viewer', 'operator', 'manager'].map(role => <option key={role} value={role}>{t(`people.projectRole_${role}`)}</option>)}
                      </select>
                      <button type="button" onClick={() => setAccessProjects(accessProjects.filter((_, i) => i !== idx))} className="rounded-lg border border-neutral-200 px-3 text-xs font-medium text-neutral-500 hover:bg-neutral-50 dark:border-zinc-700 dark:text-zinc-400 dark:hover:bg-zinc-800">
                        {t('forms.delete')}
                      </button>
                    </div>
                  ))}
                </div>
              </section>

              <section>
                <div className="mb-2 flex items-center justify-between">
                  <h3 className="text-sm font-semibold text-neutral-900 dark:text-zinc-100">{t('people.agentAccess')}</h3>
                  <button type="button" onClick={addAgentGrant} disabled={projectCatalog.length === 0} className="rounded-lg border border-sky-600 bg-white px-3 py-1.5 text-xs font-medium text-sky-700 hover:bg-sky-50 disabled:opacity-50 dark:border-sky-500 dark:bg-zinc-900 dark:text-sky-400 dark:hover:bg-zinc-800">
                    {t('people.addAgentAccess')}
                  </button>
                </div>
                <p className="mb-2 text-xs text-neutral-500 dark:text-zinc-500">{t('people.agentAccessHint')}</p>
                <div className="space-y-2">
                  {accessAgents.length === 0 ? (
                    <p className="rounded-lg border border-dashed border-neutral-200 py-4 text-center text-xs text-neutral-400 dark:border-zinc-700 dark:text-zinc-500">{t('people.noAgentAccess')}</p>
                  ) : accessAgents.map((grant, idx) => {
                    const agents = agentsByProject[grant.project] ?? []
                    return (
                      <div key={`${grant.project}-${grant.agent}-${idx}`} className="grid grid-cols-[1fr_1fr_140px_auto] gap-2">
                        <select value={grant.project} onChange={e => {
                          const project = e.target.value
                          const next = [...accessAgents]
                          next[idx] = { ...next[idx], project, agent: agentsByProject[project]?.[0]?.name ?? '' }
                          setAccessAgents(next)
                        }} className={fieldCls}>
                          {projectCatalog.map(project => <option key={project.name} value={project.name}>{project.name}</option>)}
                        </select>
                        <select value={grant.agent} onChange={e => {
                          const next = [...accessAgents]; next[idx] = { ...next[idx], agent: e.target.value }
                          setAccessAgents(next)
                        }} className={fieldCls}>
                          {agents.length === 0 ? <option value="">{t('people.noAgentsInProject')}</option> : agents.map(agent => <option key={agent.name} value={agent.name}>{agent.name}</option>)}
                        </select>
                        <select value={grant.role} onChange={e => {
                          const next = [...accessAgents]; next[idx] = { ...next[idx], role: e.target.value }
                          setAccessAgents(next)
                        }} className={fieldCls}>
                          {['viewer', 'operator', 'owner'].map(role => <option key={role} value={role}>{t(`people.agentRole_${role}`)}</option>)}
                        </select>
                        <button type="button" onClick={() => setAccessAgents(accessAgents.filter((_, i) => i !== idx))} className="rounded-lg border border-neutral-200 px-3 text-xs font-medium text-neutral-500 hover:bg-neutral-50 dark:border-zinc-700 dark:text-zinc-400 dark:hover:bg-zinc-800">
                          {t('forms.delete')}
                        </button>
                      </div>
                    )
                  })}
                </div>
              </section>
              {err && <p className="text-sm text-red-600 dark:text-red-400">{err}</p>}
              <div className="flex justify-end gap-2 border-t border-neutral-100 pt-4 dark:border-zinc-800">
                <button type="button" onClick={() => setAccessEditing(null)} disabled={saving} className="rounded-lg border border-neutral-300 px-3 py-1.5 text-sm dark:border-zinc-600">{t('forms.cancel')}</button>
                <button type="button" onClick={() => void saveAccess()} disabled={saving} className="rounded-lg bg-sky-600 px-3 py-1.5 text-sm font-medium text-white disabled:opacity-50">
                  {saving ? t('forms.saving') : t('forms.save')}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
