import { useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Bot, BriefcaseBusiness, Clock3, MessageCircle, RefreshCw, Save, X } from 'lucide-react'
import { apiFetch, apiPatch, apiPost, apiTeamPath } from '../lib/api'
import { useApiJson } from '../lib/use-api'
import { PlaceholderCard } from '../components/ui/PlaceholderCard'
import { cn } from '../lib/cn'
import { showToast } from '../components/ui/Toast'
import { primaryOutlineButton } from '../lib/button-styles'
import { useWorkspaceAccess } from '../lib/workspace-access'

type AgentWorker = {
  id: string
  name: string
  displayName?: string
  description?: string
  avatar?: string
  team?: string
  role?: string
  status?: string
  model?: string
  runtimeModel?: string
  defaultModelAccountId?: string
  defaultRuntimeNodeId?: string
  defaultRuntimeMode?: string
  primarySessionId?: string
  schedule?: WorkerSchedule
  skills?: string[]
  memberships?: ProjectMembership[]
  createdAt?: string
  updatedAt?: string
}

type AgentsResponse = {
  agents?: AgentWorker[]
}

type ProviderRow = {
  id: string
  name: string
}

type TeamInfo = { path: string; name: string }
type TeamDetail = { roles?: Array<{ id?: string; name: string; description?: string }> }

type AgentDetailResponse = {
  agent?: AgentWorker
  memberships?: ProjectMembership[]
}

type ProjectMembership = {
  id: string
  projectId: string
  team?: string
  role?: string
  title?: string
  autoPickTasks?: boolean
  attentionEnabled?: boolean
  priorityWeight?: number
}

type AttentionSignal = {
  id: string
  sourceKind?: string
  reason?: string
  summary?: string
  status?: string
  priority?: number
  createdAt?: string
}

type AttentionResponse = {
  signals?: AttentionSignal[]
}

type WorkerSchedule = {
  Enabled?: boolean
  enabled?: boolean
  Interval?: string
  interval?: string
  ActiveHours?: string
  activeHours?: string
  ActiveDays?: string
  activeDays?: string
  MaxTasksPerCycle?: number
  maxTasksPerCycle?: number
  MaxCycleDuration?: string
  maxCycleDuration?: string
  triggers?: string[]
}

const fieldCls =
  'w-full rounded-md border border-neutral-200/80 bg-neutral-50/50 px-3 py-2 text-sm outline-none transition-colors focus:border-sky-400 dark:border-zinc-700/60 dark:bg-zinc-800/50 dark:text-zinc-200 dark:[color-scheme:dark]'
const filterSelectCls =
  'h-9 rounded-lg border border-neutral-200 bg-white px-3 text-sm text-neutral-600 outline-none transition-colors hover:border-neutral-300 focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300 dark:[color-scheme:dark]'

export default function AgentsPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { canAdmin } = useWorkspaceAccess()
  const [reloadKey, setReloadKey] = useState(0)
  const [showCreate, setShowCreate] = useState(false)
  const [teamFilter, setTeamFilter] = useState('all')
  const [projectFilter, setProjectFilter] = useState('all')
  const [roleFilter, setRoleFilter] = useState('all')
  const [statusFilter, setStatusFilter] = useState('all')
  const state = useApiJson<AgentsResponse>('/api/v1/agents', reloadKey, { keepPreviousDataOnReload: true })
  const providersState = useApiJson<ProviderRow[]>('/api/v1/providers', reloadKey, { keepPreviousDataOnReload: true })
  const agents = state.status === 'ok' ? (state.data.agents ?? []) : []
  const providerNames = useMemo(() => {
    const names = new Map<string, string>()
    if (providersState.status === 'ok') {
      for (const provider of providersState.data ?? []) {
        if (provider.id) names.set(provider.id, provider.name || provider.id)
      }
    }
    return names
  }, [providersState])
  const teamOptions = useMemo(() => {
    const values = new Set<string>()
    agents.forEach(agent => {
      if (agent.team) values.add(agent.team)
    })
    agents.forEach(agent => (agent.memberships ?? []).forEach(member => {
      if (member.team) values.add(member.team)
    }))
    return Array.from(values).sort((a, b) => a.localeCompare(b))
  }, [agents])
  const projectOptions = useMemo(() => {
    const values = new Set<string>()
    agents.forEach(agent => (agent.memberships ?? []).forEach(member => {
      if (member.projectId) values.add(member.projectId)
    }))
    return Array.from(values).sort((a, b) => a.localeCompare(b))
  }, [agents])
  const roleOptions = useMemo(() => {
    const values = new Set<string>()
    agents.forEach(agent => {
      if (agent.role) values.add(agent.role)
    })
    agents.forEach(agent => (agent.memberships ?? []).forEach(member => {
      if (member.role) values.add(member.role)
    }))
    return Array.from(values).sort((a, b) => a.localeCompare(b))
  }, [agents])
  const filteredAgents = useMemo(() => agents.filter(agent => {
    const status = agent.status ?? 'active'
    if (statusFilter === 'all' && status === 'archived') return false
    if (statusFilter !== 'all' && status !== statusFilter) return false
    if (teamFilter !== 'all' && agent.team !== teamFilter && !(agent.memberships ?? []).some(member => member.team === teamFilter)) return false
    if (projectFilter !== 'all' && !(agent.memberships ?? []).some(member => member.projectId === projectFilter)) return false
    if (roleFilter !== 'all' && agent.role !== roleFilter && !(agent.memberships ?? []).some(member => member.role === roleFilter)) return false
    return true
  }), [agents, projectFilter, roleFilter, statusFilter, teamFilter])
  const hasFilters = teamFilter !== 'all' || projectFilter !== 'all' || roleFilter !== 'all' || statusFilter !== 'all'

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <div className="shrink-0 px-6 pt-5 pb-4">
        <div className="flex items-start justify-between gap-4">
          <div>
            <h1 className="text-xl font-semibold text-neutral-900 dark:text-zinc-100">{t('agents.title')}</h1>
            <p className="mt-0.5 max-w-2xl text-sm text-neutral-500 dark:text-zinc-500">{t('agents.subtitle')}</p>
          </div>
          <div className="flex items-center gap-2">
            {canAdmin && (
              <button
                type="button"
                onClick={() => setShowCreate(true)}
                className={primaryOutlineButton}
              >
                {t('agents.create')}
              </button>
            )}
          </div>
        </div>
        {state.status === 'ok' && agents.length > 0 && (
          <div className="mt-4 flex flex-wrap items-center justify-between gap-2">
            <div className="flex flex-wrap items-center gap-2">
              {teamOptions.length > 0 && (
                <select value={teamFilter} onChange={event => setTeamFilter(event.target.value)} className={filterSelectCls}>
                  <option value="all">{t('agents.filterAllTeams')}</option>
                  {teamOptions.map(team => <option key={team} value={team}>{team}</option>)}
                </select>
              )}
              <select value={projectFilter} onChange={event => setProjectFilter(event.target.value)} className={filterSelectCls}>
                <option value="all">{t('agents.filterAllProjects')}</option>
                {projectOptions.map(project => <option key={project} value={project}>{project}</option>)}
              </select>
              <select value={roleFilter} onChange={event => setRoleFilter(event.target.value)} className={filterSelectCls}>
                <option value="all">{t('agents.filterAllRoles')}</option>
                {roleOptions.map(role => <option key={role} value={role}>{role}</option>)}
              </select>
              <select value={statusFilter} onChange={event => setStatusFilter(event.target.value)} className={filterSelectCls}>
                <option value="all">{t('agents.filterAvailableStatuses', { defaultValue: t('agents.filterAllStatuses') })}</option>
                {['active', 'paused', 'archived'].map(status => (
                  <option key={status} value={status}>{t(`agents.status_${status}`, { defaultValue: status })}</option>
                ))}
              </select>
              {hasFilters && (
                <button
                  type="button"
                  onClick={() => {
                    setTeamFilter('all')
                    setProjectFilter('all')
                    setRoleFilter('all')
                    setStatusFilter('all')
                  }}
                  className="rounded-lg px-2.5 py-2 text-sm font-medium text-neutral-500 transition-colors hover:bg-neutral-100 hover:text-neutral-800 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-200"
                >
                  {t('common.clear')}
                </button>
              )}
              <span className="text-xs text-neutral-400 dark:text-zinc-500">
                {t('agents.filteredCount', { count: filteredAgents.length, total: agents.length })}
              </span>
            </div>
            <button
              type="button"
              onClick={() => setReloadKey(k => k + 1)}
              className="inline-flex size-9 items-center justify-center rounded-lg border border-neutral-200 bg-white text-neutral-500 transition-colors hover:bg-neutral-50 hover:text-neutral-900 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100"
              title={t('common.refresh')}
            >
              <RefreshCw className="size-4" strokeWidth={1.8} />
            </button>
          </div>
        )}
      </div>

      <div className="flex-1 overflow-y-auto px-6 pb-6" data-tour-agent-list>
        {state.status === 'loading' && (
          <div className="flex items-center justify-center gap-2 py-16 text-sm text-neutral-500">
            <div className="size-5 animate-spin rounded-full border-2 border-neutral-300 border-t-sky-600 dark:border-zinc-600 dark:border-t-sky-400" />
            {t('api.loading')}
          </div>
        )}
        {state.status === 'error' && (
          <PlaceholderCard title={t('api.loadError')}>
            <p className="text-[13px]">{state.error.message}</p>
          </PlaceholderCard>
        )}
        {state.status === 'ok' && agents.length === 0 && (
          <div className="flex flex-col items-center justify-center py-20 text-center">
            <div className="mb-3 flex size-14 items-center justify-center rounded-2xl bg-neutral-100 dark:bg-zinc-800/50">
              <Bot className="size-6 text-neutral-400 dark:text-zinc-500" strokeWidth={1.5} />
            </div>
            <p className="text-base font-medium text-neutral-700 dark:text-zinc-300">{t('agents.emptyTitle')}</p>
            <p className="mt-1 max-w-md text-sm text-neutral-500 dark:text-zinc-500">{t('agents.emptyHint')}</p>
          </div>
        )}
        {state.status === 'ok' && agents.length > 0 && filteredAgents.length === 0 && (
          <div className="flex flex-col items-center justify-center py-20 text-center">
            <p className="text-base font-medium text-neutral-700 dark:text-zinc-300">{t('agents.emptyFilteredTitle')}</p>
            <p className="mt-1 max-w-md text-sm text-neutral-500 dark:text-zinc-500">{t('agents.emptyFilteredHint')}</p>
          </div>
        )}
        {state.status === 'ok' && filteredAgents.length > 0 && (
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 xl:grid-cols-3">
            {filteredAgents.map(agent => (
              <AgentCard
                key={agent.id}
                agent={agent}
                modelAccountName={agent.defaultModelAccountId ? providerNames.get(agent.defaultModelAccountId) : undefined}
                onOpen={() => navigate(`/agents/${encodeURIComponent(agent.name || agent.id)}`)}
                onOpenChat={(projectId) => navigate(`/projects/${encodeURIComponent(projectId)}/members/${encodeURIComponent(agent.name)}/chat`)}
              />
            ))}
          </div>
        )}
      </div>

      {canAdmin && showCreate && (
        <CreateAgentDialog
          existingNames={agents.map(agent => agent.name)}
          onClose={() => setShowCreate(false)}
          onCreated={(agent) => {
            setShowCreate(false)
            setReloadKey(k => k + 1)
            if (agent.id) navigate(`/agents/${encodeURIComponent(agent.name || agent.id)}`)
          }}
        />
      )}
    </div>
  )
}

function AgentCard({ agent, modelAccountName, onOpen, onOpenChat }: { agent: AgentWorker; modelAccountName?: string; onOpen: () => void; onOpenChat: (projectId: string) => void }) {
  const { t } = useTranslation()
  const status = agent.status ?? 'active'
  const displayName = agent.displayName || agent.name
  const skills = Array.isArray(agent.skills) ? agent.skills : []
  const memberships = agent.memberships ?? []
  const firstProjectId = memberships.find(member => member.projectId)?.projectId
  const membershipLabels = memberships
    .map(member => [member.team, member.role].filter(Boolean).join(' / '))
    .filter(Boolean)
  const defaultRoleLabel = [agent.team, agent.role].filter(Boolean).join(' / ')
  const primaryMembershipLabel = [defaultRoleLabel, ...membershipLabels].filter(Boolean).slice(0, 2).join(' · ') || t('agents.unassignedRole')
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onOpen}
      onKeyDown={(event) => {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault()
          onOpen()
        }
      }}
      title={agent.name !== displayName ? agent.name : undefined}
      className="cursor-pointer rounded-lg border border-neutral-200 bg-white p-4 text-left shadow-sm transition-colors hover:border-neutral-300 hover:bg-neutral-50/60 dark:border-zinc-800 dark:bg-zinc-900 dark:hover:border-zinc-700 dark:hover:bg-zinc-800/40"
    >
      <div className="flex items-start gap-3">
        {agent.avatar ? (
          <img src={agent.avatar} alt="" className="size-10 rounded-lg bg-neutral-100 object-cover dark:bg-zinc-800" />
        ) : (
          <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-violet-100 text-violet-700 dark:bg-violet-900/30 dark:text-violet-300">
            <Bot className="size-5" strokeWidth={1.8} />
          </div>
        )}
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <h2 className="truncate text-sm font-semibold text-neutral-900 dark:text-zinc-100">{displayName}</h2>
            <span className={cn(
              'shrink-0 rounded-full px-2 py-0.5 text-[11px] font-medium',
              status === 'active'
                ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300'
                : 'bg-neutral-100 text-neutral-500 dark:bg-zinc-800 dark:text-zinc-400',
            )}>
              {t(`agents.status_${status}`, { defaultValue: status })}
            </span>
          </div>
          {primaryMembershipLabel && (
            <p className="mt-0.5 truncate text-xs text-neutral-400 dark:text-zinc-500">{primaryMembershipLabel}</p>
          )}
        </div>
        {firstProjectId && (
          <button
            type="button"
            onClick={(event) => {
              event.stopPropagation()
              onOpenChat(firstProjectId)
            }}
            className="inline-flex size-8 shrink-0 items-center justify-center rounded-lg text-neutral-400 transition-colors hover:bg-neutral-100 hover:text-neutral-800 dark:text-zinc-500 dark:hover:bg-zinc-800 dark:hover:text-zinc-200"
            title={t('agents.openChat')}
          >
            <MessageCircle className="size-4" strokeWidth={1.8} />
          </button>
        )}
      </div>
      <div className="mt-4 space-y-2 border-t border-neutral-100 pt-3 text-xs dark:border-zinc-800">
        <InfoRow label={t('agents.model')} value={agent.runtimeModel || agent.model || t('agents.defaultModel')} />
        <InfoRow label={t('agents.runtimeNode')} value={agent.defaultRuntimeNodeId || t('agents.defaultRuntime')} />
        <InfoRow label={t('agents.modelAccount')} value={modelAccountName || (agent.defaultModelAccountId ? t('agents.configuredModelAccount') : t('agents.defaultModel'))} />
      </div>
      {skills.length > 0 && (
        <div className="mt-3 flex flex-wrap gap-1.5">
          {skills.slice(0, 6).map(skill => (
            <span key={skill} className="rounded-md border border-neutral-200 px-2 py-0.5 text-xs text-neutral-500 dark:border-zinc-700 dark:text-zinc-400">{skill}</span>
          ))}
          {skills.length > 6 && <span className="px-1 py-0.5 text-xs text-neutral-400">+{skills.length - 6}</span>}
        </div>
      )}
    </div>
  )
}

function InfoRow({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex items-center justify-between gap-3">
      <span className="shrink-0 text-neutral-400 dark:text-zinc-500">{label}</span>
      <span className={cn('truncate text-neutral-600 dark:text-zinc-300', mono && 'font-mono')}>{value}</span>
    </div>
  )
}

function CreateAgentDialog({ existingNames, onClose, onCreated }: {
  existingNames: string[]
  onClose: () => void
  onCreated: (agent: AgentWorker) => void
}) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [description, setDescription] = useState('')
  const [team, setTeam] = useState('')
  const [role, setRole] = useState('')
  const [teams, setTeams] = useState<TeamInfo[]>([])
  const [roles, setRoles] = useState<Array<{ id?: string; name: string; description?: string }>>([])
  const [saving, setSaving] = useState(false)
  const normalizedName = name.trim()
  const duplicate = existingNames.includes(normalizedName)
  const invalid = normalizedName !== '' && !/^[A-Za-z0-9_-]+$/.test(normalizedName)

  useEffect(() => {
    let cancelled = false
    apiFetch<TeamInfo[]>('/api/v1/teams')
      .then(rows => {
        if (cancelled) return
        const next = rows ?? []
        setTeams(next)
        if (!team && next[0]?.path) setTeam(next[0].path)
      })
      .catch(() => {})
    return () => { cancelled = true }
  }, [team])

  useEffect(() => {
    if (!team) {
      setRoles([])
      setRole('')
      return
    }
    let cancelled = false
    apiFetch<TeamDetail>(`/api/v1/teams/${apiTeamPath(team)}`)
      .then(detail => {
        if (cancelled) return
        const next = detail.roles ?? []
        setRoles(next)
        setRole(current => current || next[0]?.id || next[0]?.name || '')
      })
      .catch(() => {
        if (!cancelled) {
          setRoles([])
          setRole('')
        }
      })
    return () => { cancelled = true }
  }, [team])

  async function save() {
    if (!normalizedName || duplicate || invalid) return
    setSaving(true)
    try {
      const res = await apiPost<{ agent?: AgentWorker }>('/api/v1/agents', {
        name: normalizedName,
        displayName: displayName.trim(),
        description: description.trim(),
        team,
        role,
      })
      onCreated(res.agent ?? { id: '', name: normalizedName, displayName: displayName.trim(), description: description.trim(), team, role })
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30 p-4 backdrop-blur-sm">
      <div className="w-full max-w-md rounded-xl border border-neutral-200 bg-white shadow-xl dark:border-zinc-800 dark:bg-zinc-900">
        <div className="flex items-center justify-between border-b border-neutral-100 px-5 py-4 dark:border-zinc-800">
          <div>
            <h2 className="text-base font-semibold text-neutral-900 dark:text-zinc-100">{t('agents.create')}</h2>
            <p className="mt-0.5 text-xs text-neutral-500 dark:text-zinc-500">{t('agents.createHint')}</p>
          </div>
          <button type="button" onClick={onClose} className="rounded-lg p-1.5 text-neutral-400 hover:bg-neutral-100 hover:text-neutral-700 dark:hover:bg-zinc-800 dark:hover:text-zinc-200">
            <X className="size-4" strokeWidth={1.8} />
          </button>
        </div>
        <div className="space-y-4 px-5 py-4">
          <label className="block text-sm">
            <span className="text-neutral-600 dark:text-zinc-400">{t('agents.name')} *</span>
            <input className={cn(fieldCls, 'mt-1.5', (duplicate || invalid) && 'border-red-300 focus:border-red-400')} value={name} onChange={e => setName(e.target.value)} placeholder="manager-agent" autoFocus />
            {duplicate && <p className="mt-1 text-xs text-red-600 dark:text-red-400">{t('agents.duplicateName')}</p>}
            {invalid && <p className="mt-1 text-xs text-red-600 dark:text-red-400">{t('agents.invalidName')}</p>}
          </label>
          <label className="block text-sm">
            <span className="text-neutral-600 dark:text-zinc-400">{t('agents.displayName')}</span>
            <input className={cn(fieldCls, 'mt-1.5')} value={displayName} onChange={e => setDisplayName(e.target.value)} placeholder="manager-agent" />
          </label>
          {teams.length > 0 && (
            <label className="block text-sm">
              <span className="text-neutral-600 dark:text-zinc-400">{t('members.team')}</span>
              <select className={cn(fieldCls, 'mt-1.5')} value={team} onChange={e => { setTeam(e.target.value); setRole('') }}>
                <option value="">{t('members.selectTeam')}</option>
                {teams.map(item => (
                  <option key={item.path} value={item.path}>{item.name} ({item.path})</option>
                ))}
              </select>
            </label>
          )}
          {roles.length > 0 && (
            <label className="block text-sm">
              <span className="text-neutral-600 dark:text-zinc-400">{t('members.role')}</span>
              <select className={cn(fieldCls, 'mt-1.5')} value={role} onChange={e => setRole(e.target.value)}>
                <option value="">{t('members.noRole')}</option>
                {roles.map(item => {
                  const value = item.id || item.name
                  return <option key={value} value={value}>{item.name}{item.description ? ` - ${item.description}` : ''}</option>
                })}
              </select>
            </label>
          )}
          <label className="block text-sm">
            <span className="text-neutral-600 dark:text-zinc-400">{t('agents.description')}</span>
            <textarea className={cn(fieldCls, 'mt-1.5 min-h-24 resize-none')} value={description} onChange={e => setDescription(e.target.value)} placeholder={t('agents.descriptionPlaceholder')} />
          </label>
        </div>
        <div className="flex justify-end gap-2 border-t border-neutral-100 px-5 py-4 dark:border-zinc-800">
          <button type="button" onClick={onClose} className="rounded-lg px-3 py-2 text-sm font-medium text-neutral-600 hover:bg-neutral-100 dark:text-zinc-400 dark:hover:bg-zinc-800">{t('common.cancel')}</button>
          <button type="button" onClick={save} disabled={saving || !normalizedName || duplicate || invalid} className="rounded-lg bg-sky-600 px-3 py-2 text-sm font-medium text-white hover:bg-sky-700 disabled:opacity-50">
            {saving ? t('common.creating') : t('agents.create')}
          </button>
        </div>
      </div>
    </div>
  )
}

const triggerOptions = [
  { key: 'message', label: 'agents.trigger_message' },
  { key: 'task', label: 'agents.trigger_task' },
  { key: 'im_direct_message', label: 'agents.trigger_im_direct_message' },
  { key: 'im_mention', label: 'agents.trigger_im_mention' },
  { key: 'workflow_step_assigned', label: 'agents.trigger_workflow_step_assigned' },
  { key: 'card_action', label: 'agents.trigger_card_action' },
]

export function AgentDetailDialog({ agent, onClose, onSaved }: {
  agent: AgentWorker
  onClose: () => void
  onSaved: (agent: AgentWorker) => void
}) {
  const { t } = useTranslation()
  const [reloadKey, setReloadKey] = useState(0)
  const detailState = useApiJson<AgentDetailResponse>(`/api/v1/agents/${encodeURIComponent(agent.id)}`, reloadKey, { keepPreviousDataOnReload: true })
  const attentionState = useApiJson<AttentionResponse>(`/api/v1/agents/${encodeURIComponent(agent.id)}/attention?limit=8`, reloadKey, { keepPreviousDataOnReload: true })
  const detailAgent = detailState.status === 'ok' && detailState.data.agent ? detailState.data.agent : agent
  const memberships = detailState.status === 'ok' ? (detailState.data.memberships ?? []) : []
  const signals = attentionState.status === 'ok' ? (attentionState.data.signals ?? []) : []
  const schedule = normalizeSchedule(detailAgent.schedule)
  const [enabled, setEnabled] = useState(schedule.enabled)
  const [interval, setInterval] = useState(schedule.interval)
  const [activeHours, setActiveHours] = useState(schedule.activeHours)
  const [activeDays, setActiveDays] = useState(schedule.activeDays)
  const [maxTasks, setMaxTasks] = useState(String(schedule.maxTasksPerCycle))
  const [maxCycleDuration, setMaxCycleDuration] = useState(schedule.maxCycleDuration)
  const [triggers, setTriggers] = useState<string[]>(schedule.triggers)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    const next = normalizeSchedule(detailAgent.schedule)
    setEnabled(next.enabled)
    setInterval(next.interval)
    setActiveHours(next.activeHours)
    setActiveDays(next.activeDays)
    setMaxTasks(String(next.maxTasksPerCycle))
    setMaxCycleDuration(next.maxCycleDuration)
    setTriggers(next.triggers)
  }, [detailAgent.id, detailAgent.schedule])

  function toggleTrigger(key: string) {
    setTriggers(prev => prev.includes(key) ? prev.filter(item => item !== key) : [...prev, key])
  }

  async function saveSchedule() {
    setSaving(true)
    try {
      const maxTasksParsed = Number.parseInt(maxTasks, 10)
      const res = await apiPatch<{ agent?: AgentWorker }>(`/api/v1/agents/${encodeURIComponent(agent.id)}`, {
        schedule: {
          Enabled: enabled,
          Interval: interval.trim() || '2h',
          ActiveHours: activeHours.trim(),
          ActiveDays: activeDays.trim(),
          MaxTasksPerCycle: Number.isFinite(maxTasksParsed) && maxTasksParsed > 0 ? maxTasksParsed : 5,
          MaxCycleDuration: maxCycleDuration.trim() || '45m',
          triggers,
        },
      })
      if (res.agent) onSaved(res.agent)
      showToast(t('agents.scheduleSaved'), 'success')
      setReloadKey(k => k + 1)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/35 p-4 backdrop-blur-sm" role="presentation" onClick={onClose}>
      <div
        className="flex max-h-[min(92vh,760px)] w-full max-w-4xl flex-col overflow-hidden rounded-xl border border-neutral-200 bg-white shadow-xl dark:border-zinc-800 dark:bg-zinc-950"
        role="dialog"
        aria-labelledby="agent-detail-title"
        onClick={e => e.stopPropagation()}
      >
        <div className="flex items-start justify-between gap-4 border-b border-neutral-100 px-5 py-4 dark:border-zinc-800">
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <h2 id="agent-detail-title" className="truncate text-base font-semibold text-neutral-900 dark:text-zinc-100">
                {detailAgent.displayName || detailAgent.name}
              </h2>
              <span className="rounded-full bg-neutral-100 px-2 py-0.5 font-mono text-[11px] text-neutral-500 dark:bg-zinc-800 dark:text-zinc-400">
                {detailAgent.name}
              </span>
            </div>
            <p className="mt-1 max-w-2xl text-sm text-neutral-500 dark:text-zinc-500">{detailAgent.description || t('agents.detailSubtitle')}</p>
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => setReloadKey(k => k + 1)}
              className="inline-flex size-8 items-center justify-center rounded-lg border border-neutral-200 text-neutral-500 hover:bg-neutral-50 dark:border-zinc-700 dark:text-zinc-400 dark:hover:bg-zinc-800"
              title={t('common.refresh')}
            >
              <RefreshCw className="size-4" strokeWidth={1.8} />
            </button>
            <button type="button" onClick={onClose} className="rounded-lg p-1.5 text-neutral-400 hover:bg-neutral-100 hover:text-neutral-700 dark:hover:bg-zinc-800 dark:hover:text-zinc-200">
              <X className="size-4" strokeWidth={1.8} />
            </button>
          </div>
        </div>
        <div className="grid min-h-0 flex-1 grid-cols-1 overflow-y-auto lg:grid-cols-[1fr_320px]">
          <div className="space-y-5 p-5">
            <section data-tour-scheduler-control>
              <SectionTitle icon={<Clock3 className="size-4" />} title={t('agents.schedule')} />
              <div className="mt-3 rounded-lg border border-neutral-200 bg-white p-4 dark:border-zinc-800 dark:bg-zinc-900/60">
                <label className="flex items-center justify-between gap-3">
                  <span>
                    <span className="block text-sm font-medium text-neutral-800 dark:text-zinc-200">{t('agents.scheduleEnabled')}</span>
                    <span className="mt-0.5 block text-xs text-neutral-500 dark:text-zinc-500">{t('agents.scheduleEnabledHint')}</span>
                  </span>
                  <input type="checkbox" checked={enabled} onChange={e => setEnabled(e.target.checked)} className="size-4 accent-sky-600" />
                </label>
                <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2">
                  <Field label={t('agents.interval')}>
                    <input value={interval} onChange={e => setInterval(e.target.value)} className={fieldCls} placeholder="2h" />
                  </Field>
                  <Field label={t('agents.maxTasksPerCycle')}>
                    <input value={maxTasks} onChange={e => setMaxTasks(e.target.value)} className={fieldCls} inputMode="numeric" placeholder="5" />
                  </Field>
                  <Field label={t('agents.activeHours')}>
                    <input value={activeHours} onChange={e => setActiveHours(e.target.value)} className={fieldCls} placeholder="08:00-23:00" />
                  </Field>
                  <Field label={t('agents.activeDays')}>
                    <input value={activeDays} onChange={e => setActiveDays(e.target.value)} className={fieldCls} placeholder="Mon,Tue,Wed,Thu,Fri" />
                  </Field>
                  <Field label={t('agents.maxCycleDuration')}>
                    <input value={maxCycleDuration} onChange={e => setMaxCycleDuration(e.target.value)} className={fieldCls} placeholder="45m" />
                  </Field>
                </div>
                <div className="mt-4">
                  <p className="text-xs font-medium text-neutral-500 dark:text-zinc-400">{t('agents.wakeupTriggers')}</p>
                  <div className="mt-2 grid grid-cols-1 gap-2 sm:grid-cols-2">
                    {triggerOptions.map(option => (
                      <label key={option.key} className="flex items-center gap-2 rounded-md border border-neutral-200 px-3 py-2 text-sm text-neutral-700 dark:border-zinc-700 dark:text-zinc-300">
                        <input type="checkbox" checked={triggers.includes(option.key)} onChange={() => toggleTrigger(option.key)} className="size-4 accent-sky-600" />
                        {t(option.label)}
                      </label>
                    ))}
                  </div>
                </div>
                <div className="mt-4 flex justify-end">
                  <button
                    type="button"
                    onClick={() => void saveSchedule()}
                    disabled={saving}
                    className="inline-flex items-center gap-2 rounded-lg bg-neutral-900 px-3 py-2 text-sm font-medium text-white disabled:opacity-50 dark:bg-zinc-100 dark:text-zinc-900"
                  >
                    <Save className="size-4" strokeWidth={1.8} />
                    {saving ? t('common.saving') : t('common.save')}
                  </button>
                </div>
              </div>
            </section>
            <section>
              <SectionTitle icon={<BriefcaseBusiness className="size-4" />} title={t('agents.projectMemberships')} />
              <div className="mt-3 space-y-2">
                {detailState.status === 'loading' && <p className="text-sm text-neutral-500">{t('api.loading')}</p>}
                {memberships.length === 0 && detailState.status === 'ok' && (
                  <p className="rounded-lg border border-neutral-200 px-3 py-3 text-sm text-neutral-500 dark:border-zinc-800 dark:text-zinc-500">{t('agents.noMemberships')}</p>
                )}
                {memberships.map(member => (
                  <div key={member.id} className="rounded-lg border border-neutral-200 bg-white px-3 py-3 dark:border-zinc-800 dark:bg-zinc-900/60">
                    <div className="flex items-center justify-between gap-3">
                      <span className="font-mono text-sm font-medium text-neutral-800 dark:text-zinc-200">{member.projectId}</span>
                      <span className="rounded-md bg-neutral-100 px-2 py-0.5 text-xs text-neutral-500 dark:bg-zinc-800 dark:text-zinc-400">{member.role || 'member'}</span>
                    </div>
                    <p className="mt-1 text-xs text-neutral-500 dark:text-zinc-500">
                      {member.title || detailAgent.displayName || detailAgent.name} · {member.autoPickTasks ? t('agents.autoPickEnabled') : t('agents.autoPickDisabled')} · {member.attentionEnabled ? t('agents.attentionEnabled') : t('agents.attentionDisabled')}
                    </p>
                  </div>
                ))}
              </div>
            </section>
          </div>
          <aside className="border-t border-neutral-100 p-5 dark:border-zinc-800 lg:border-t-0 lg:border-l">
            <SectionTitle icon={<MessageCircle className="size-4" />} title={t('agents.attention')} />
            <div className="mt-3 space-y-2">
              {attentionState.status === 'loading' && <p className="text-sm text-neutral-500">{t('api.loading')}</p>}
              {signals.length === 0 && attentionState.status === 'ok' && (
                <p className="rounded-lg border border-neutral-200 px-3 py-3 text-sm text-neutral-500 dark:border-zinc-800 dark:text-zinc-500">{t('agents.noAttention')}</p>
              )}
              {signals.map(signal => (
                <div key={signal.id} className="rounded-lg border border-neutral-200 bg-white px-3 py-2.5 dark:border-zinc-800 dark:bg-zinc-900/60">
                  <div className="flex items-center justify-between gap-2">
                    <span className="truncate text-xs font-medium text-neutral-500 dark:text-zinc-400">{signal.sourceKind || '-'}</span>
                    <span className="rounded-full bg-neutral-100 px-2 py-0.5 text-[11px] text-neutral-500 dark:bg-zinc-800 dark:text-zinc-400">
                      {t(`agents.attention_${signal.status || 'pending'}`, { defaultValue: signal.status || 'pending' })}
                    </span>
                  </div>
                  <p className="mt-1 line-clamp-3 text-sm leading-5 text-neutral-700 dark:text-zinc-300">{signal.summary || signal.reason || signal.id}</p>
                </div>
              ))}
            </div>
          </aside>
        </div>
      </div>
    </div>
  )
}

function SectionTitle({ icon, title }: { icon: ReactNode; title: string }) {
  return (
    <div className="flex items-center gap-2 text-sm font-semibold text-neutral-800 dark:text-zinc-200">
      <span className="text-neutral-400 dark:text-zinc-500">{icon}</span>
      {title}
    </div>
  )
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block text-sm">
      <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">{label}</span>
      {children}
    </label>
  )
}

function normalizeSchedule(raw?: WorkerSchedule): Required<Pick<WorkerSchedule, 'enabled' | 'interval' | 'activeHours' | 'activeDays' | 'maxTasksPerCycle' | 'maxCycleDuration' | 'triggers'>> {
  return {
    enabled: raw?.enabled ?? raw?.Enabled ?? false,
    interval: raw?.interval ?? raw?.Interval ?? '2h',
    activeHours: raw?.activeHours ?? raw?.ActiveHours ?? '',
    activeDays: raw?.activeDays ?? raw?.ActiveDays ?? '',
    maxTasksPerCycle: raw?.maxTasksPerCycle ?? raw?.MaxTasksPerCycle ?? 5,
    maxCycleDuration: raw?.maxCycleDuration ?? raw?.MaxCycleDuration ?? '45m',
    triggers: raw?.triggers ?? [],
  }
}
