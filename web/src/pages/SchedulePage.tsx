import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Bot, Clock3, Pause, RefreshCw, Zap } from 'lucide-react'
import { useApiJson } from '../lib/use-api'
import { cn } from '../lib/cn'
import { apiFetch, apiPatch } from '../lib/api'
import { showToast } from '../components/ui/Toast'
import { useWorkspaceAccess } from '../lib/workspace-access'
import { useFormatDateTime } from '../lib/format-datetime'

type WorkerSchedule = {
  Enabled?: boolean
  enabled?: boolean
  Jitter?: string
  jitter?: string
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
  WakeupPreset?: string
  wakeupPreset?: string
  WakeupCondition?: string
  wakeupCondition?: string
  SessionScope?: string
  sessionScope?: string
  SessionID?: string
  sessionId?: string
  triggers?: string[]
}

type AgentWorker = {
  id: string
  name: string
  displayName?: string
  avatar?: string
  status?: string
  schedule?: WorkerSchedule
  defaultRuntimeNodeId?: string
  defaultModelAccountId?: string
  memberships?: ProjectMembership[]
}

type ProjectMembership = {
  id: string
  projectId: string
  team?: string
  role?: string
  title?: string
}

type AgentsResponse = {
  agents?: AgentWorker[]
}

type SchedulerInstance = {
  key: string
  running: boolean
  mode?: string
  project?: string
  agent?: string
  startedAt?: string
  error?: string
}

type SchedulerStatusResponse = {
  schedulers?: SchedulerInstance[]
}

type CronSummary = { project: string; agent: string; agentWorkerId?: string; count: number; enabled: number; nextRun?: string }

function summariesForAgent(summaries: CronSummary[], agent: AgentWorker): CronSummary[] {
  return summaries
    .filter(item => (item.agentWorkerId === agent.id || item.agent === agent.name))
}

function earliestNextRun(summaries: CronSummary[], agent: AgentWorker): string | undefined {
  return summariesForAgent(summaries, agent)
    .filter(item => item.nextRun)
    .map(item => item.nextRun as string)
    .sort((a, b) => new Date(a).getTime() - new Date(b).getTime())[0]
}

type NormalizedSchedule = {
  enabled: boolean
  interval: string
  jitter: string
  activeHours: string
  activeDays: string
  maxTasksPerCycle: number
  maxCycleDuration: string
  wakeupPreset: string
  wakeupCondition: string
  sessionScope: string
  sessionId: string
  triggers: string[]
}

const filterSelectCls =
  'h-9 rounded-lg border border-neutral-200 bg-white px-3 text-sm text-neutral-600 outline-none transition-colors hover:border-neutral-300 focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300 dark:[color-scheme:dark]'

function normalizeSchedule(schedule?: WorkerSchedule): NormalizedSchedule {
  return {
    enabled: Boolean(schedule?.enabled ?? schedule?.Enabled),
    interval: schedule?.interval || schedule?.Interval || '30m',
    jitter: schedule?.jitter || schedule?.Jitter || '',
    activeHours: schedule?.activeHours || schedule?.ActiveHours || '',
    activeDays: schedule?.activeDays || schedule?.ActiveDays || '',
    maxTasksPerCycle: Number(schedule?.maxTasksPerCycle ?? schedule?.MaxTasksPerCycle ?? 1),
    maxCycleDuration: schedule?.maxCycleDuration || schedule?.MaxCycleDuration || '',
    wakeupPreset: schedule?.wakeupPreset || schedule?.WakeupPreset || '',
    wakeupCondition: schedule?.wakeupCondition || schedule?.WakeupCondition || '',
    sessionScope: schedule?.sessionScope || schedule?.SessionScope || 'cycle',
    sessionId: schedule?.sessionId || schedule?.SessionID || '',
    triggers: Array.isArray(schedule?.triggers) ? schedule.triggers : [],
  }
}

export default function SchedulePage() {
  const { t } = useTranslation()
  const { canAdmin } = useWorkspaceAccess()
  const fmt = useFormatDateTime()
  const [reloadKey, setReloadKey] = useState(0)
  const [teamFilter, setTeamFilter] = useState('all')
  const [projectFilter, setProjectFilter] = useState('all')
  const [roleFilter, setRoleFilter] = useState('all')
  const [statusFilter, setStatusFilter] = useState('all')
  const [togglingId, setTogglingId] = useState<string | null>(null)
  const [cronSummaries, setCronSummaries] = useState<CronSummary[]>([])
  const agentsState = useApiJson<AgentsResponse>('/api/v1/agents', reloadKey, { keepPreviousDataOnReload: true })
  const schedulerState = useApiJson<SchedulerStatusResponse>('/api/v1/scheduler/status', reloadKey, { keepPreviousDataOnReload: true })
  const agents = agentsState.status === 'ok' ? (agentsState.data.agents ?? []) : []
  const schedulers = schedulerState.status === 'ok' ? (schedulerState.data.schedulers ?? []) : []
  const runningByKey = useMemo(() => new Map(schedulers.filter(item => item.running).map(item => [item.key, item])), [schedulers])
  const teamOptions = useMemo(() => {
    const values = new Set<string>()
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
    agents.forEach(agent => (agent.memberships ?? []).forEach(member => {
      if (member.role) values.add(member.role)
    }))
    return Array.from(values).sort((a, b) => a.localeCompare(b))
  }, [agents])
  const filteredAgents = useMemo(() => agents.filter(agent => {
    const schedule = normalizeSchedule(agent.schedule)
    const running = Boolean(runningByKey.get(`worker/${agent.id}`))
    const scheduleStatus = running ? 'running' : schedule.enabled ? 'enabled' : 'off'
    if (statusFilter !== 'all' && scheduleStatus !== statusFilter) return false
    if (teamFilter !== 'all' && !(agent.memberships ?? []).some(member => member.team === teamFilter)) return false
    if (projectFilter !== 'all' && !(agent.memberships ?? []).some(member => member.projectId === projectFilter)) return false
    if (roleFilter !== 'all' && !(agent.memberships ?? []).some(member => member.role === roleFilter)) return false
    return true
  }), [agents, projectFilter, roleFilter, runningByKey, statusFilter, teamFilter])
  const hasFilters = teamFilter !== 'all' || projectFilter !== 'all' || roleFilter !== 'all' || statusFilter !== 'all'

  useEffect(() => {
    let cancelled = false
    async function loadCronSummaries() {
      if (projectOptions.length === 0) { setCronSummaries([]); return }
      try {
        const responses = await Promise.all(projectOptions.map(project => apiFetch<{ project: string; agents?: Array<{ name: string; agentWorkerId?: string; heartbeat?: { enabled?: boolean; nextWakeupAt?: string }; crons?: Array<{ enabled: boolean; nextRun?: string }> }> }>(`/api/v1/projects/${encodeURIComponent(project)}/schedule`)))
        if (cancelled) return
        const next: CronSummary[] = []
        responses.forEach(response => (response.agents ?? []).forEach(agent => {
          const crons = agent.crons ?? []
          const enabledCrons = crons.filter(cron => cron.enabled)
          const nextRunCandidates = enabledCrons
            .map(cron => cron.nextRun)
            .filter((value): value is string => Boolean(value))
          if (agent.heartbeat?.enabled && agent.heartbeat.nextWakeupAt) nextRunCandidates.push(agent.heartbeat.nextWakeupAt)
          const nextRun = nextRunCandidates.sort((a, b) => new Date(a).getTime() - new Date(b).getTime())[0]
          if (crons.length > 0 || nextRun) next.push({ project: response.project, agent: agent.name, agentWorkerId: agent.agentWorkerId, count: crons.length, enabled: enabledCrons.length, nextRun })
        }))
        setCronSummaries(next)
      } catch {
        if (!cancelled) setCronSummaries([])
      }
    }
    void loadCronSummaries()
    return () => { cancelled = true }
  }, [projectOptions, reloadKey])

  async function toggleHeartbeat(agent: AgentWorker) {
    const schedule = normalizeSchedule(agent.schedule)
    setTogglingId(agent.id)
    try {
      await apiPatch(`/api/v1/agents/${encodeURIComponent(agent.id)}`, {
        schedule: {
          Enabled: !schedule.enabled,
          Interval: schedule.interval || '30m',
          Jitter: schedule.jitter,
          ActiveHours: schedule.activeHours,
          ActiveDays: schedule.activeDays,
          MaxTasksPerCycle: schedule.maxTasksPerCycle || 1,
          MaxCycleDuration: schedule.maxCycleDuration,
          WakeupPreset: schedule.wakeupPreset,
          WakeupCondition: schedule.wakeupCondition,
          SessionScope: schedule.sessionScope,
          SessionID: schedule.sessionId,
          triggers: schedule.triggers,
        },
      })
      showToast(t('agents.scheduleSaved'), 'success')
      setReloadKey(k => k + 1)
    } finally {
      setTogglingId(null)
    }
  }

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <div className="shrink-0 px-6 pt-5 pb-4">
        <div className="flex items-start justify-between gap-4">
          <div>
            <h1 className="text-xl font-semibold text-neutral-900 dark:text-zinc-100">{t('schedule.workspaceTitle')}</h1>
            <p className="mt-0.5 max-w-2xl text-sm text-neutral-500 dark:text-zinc-500">{t('schedule.workspaceSubtitle')}</p>
          </div>
        </div>
        {agentsState.status === 'ok' && agents.length > 0 && (
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
                <option value="all">{t('agents.filterAllStatuses')}</option>
                <option value="running">{t('schedule.running')}</option>
                <option value="enabled">{t('schedule.hbActive')}</option>
                <option value="off">{t('schedule.off')}</option>
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

      <div className="flex-1 overflow-y-auto px-6 pb-6">
        {(agentsState.status === 'loading' || schedulerState.status === 'loading') && (
          <div className="flex items-center justify-center gap-2 py-16 text-sm text-neutral-500">
            <div className="size-5 animate-spin rounded-full border-2 border-neutral-300 border-t-sky-600 dark:border-zinc-600 dark:border-t-sky-400" />
            {t('api.loading')}
          </div>
        )}
        {(agentsState.status === 'error' || schedulerState.status === 'error') && (
          <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/20 dark:text-red-300">
            {agentsState.status === 'error' ? agentsState.error.message : schedulerState.status === 'error' ? schedulerState.error.message : t('api.loadError')}
          </div>
        )}
        {agentsState.status === 'ok' && schedulerState.status === 'ok' && agents.length === 0 && (
          <div className="flex flex-col items-center justify-center py-20 text-center">
            <div className="mb-3 flex size-14 items-center justify-center rounded-2xl bg-neutral-100 dark:bg-zinc-800/50">
              <Clock3 className="size-6 text-neutral-400 dark:text-zinc-500" strokeWidth={1.5} />
            </div>
            <p className="text-base font-medium text-neutral-700 dark:text-zinc-300">{t('agents.emptyTitle')}</p>
            <p className="mt-1 max-w-md text-sm text-neutral-500 dark:text-zinc-500">{t('agents.emptyHint')}</p>
          </div>
        )}
        {agentsState.status === 'ok' && schedulerState.status === 'ok' && agents.length > 0 && filteredAgents.length === 0 && (
          <div className="flex flex-col items-center justify-center py-20 text-center">
            <p className="text-base font-medium text-neutral-700 dark:text-zinc-300">{t('agents.emptyFilteredTitle')}</p>
            <p className="mt-1 max-w-md text-sm text-neutral-500 dark:text-zinc-500">{t('agents.emptyFilteredHint')}</p>
          </div>
        )}
        {agentsState.status === 'ok' && schedulerState.status === 'ok' && filteredAgents.length > 0 && (
          <div className="overflow-x-auto rounded-lg border border-neutral-200 bg-white dark:border-zinc-800 dark:bg-zinc-900">
            <table className="w-full min-w-[1080px]">
              <thead className="border-b border-neutral-200 bg-neutral-50 text-xs font-medium text-neutral-500 dark:border-zinc-800 dark:bg-zinc-900/70 dark:text-zinc-500">
                <tr>
                  <th className="whitespace-nowrap px-4 py-3 text-left">{t('schedule.agent')}</th>
                  <th className="whitespace-nowrap px-4 py-3 text-left">{t('schedule.statusLabel')}</th>
                  <th className="whitespace-nowrap px-4 py-3 text-left">{t('agents.interval')}</th>
                  <th className="whitespace-nowrap px-4 py-3 text-left">{t('agents.activeHours')}</th>
                  <th className="whitespace-nowrap px-4 py-3 text-left">{t('agents.maxTasksPerCycle')}</th>
                  <th className="whitespace-nowrap px-4 py-3 text-left">{t('agents.wakeupTriggers')}</th>
                  <th className="whitespace-nowrap px-4 py-3 text-left">{t('agents.runtimeNode')}</th>
                  <th className="whitespace-nowrap px-4 py-3 text-left">{t('schedule.cronJobs', { defaultValue: '定时任务' })}</th>
                  <th className="whitespace-nowrap px-4 py-3 text-left">{t('schedule.nextRun')}</th>
                  {canAdmin && <th className="whitespace-nowrap px-4 py-3 text-right">{t('common.actions')}</th>}
                </tr>
              </thead>
              <tbody className="divide-y divide-neutral-100 dark:divide-zinc-800">
                {filteredAgents.map(agent => {
                  const schedule = normalizeSchedule(agent.schedule)
                  const running = runningByKey.get(`worker/${agent.id}`)
                  const nextRun = earliestNextRun(cronSummaries, agent)
                  return (
                    <tr key={agent.id} className="hover:bg-neutral-50/70 dark:hover:bg-zinc-800/40">
                      <td className="px-4 py-3">
                        <Link
                          to={`/agents/${encodeURIComponent(agent.name || agent.id)}`}
                          className="group flex items-center gap-3 rounded-md -m-1 p-1 transition-colors hover:bg-neutral-100 dark:hover:bg-zinc-800"
                        >
                          {agent.avatar ? (
                            <img src={agent.avatar} alt="" className="size-9 rounded-lg bg-neutral-100 object-cover dark:bg-zinc-800" />
                          ) : (
                            <div className="flex size-9 items-center justify-center rounded-lg bg-violet-100 text-violet-700 dark:bg-violet-900/30 dark:text-violet-300">
                              <Bot className="size-4" strokeWidth={1.8} />
                            </div>
                          )}
                          <div className="min-w-0">
                            <p className="truncate text-sm font-semibold text-neutral-900 group-hover:text-sky-700 dark:text-zinc-100 dark:group-hover:text-sky-300">{agent.displayName || agent.name}</p>
                            <p className="truncate font-mono text-xs text-neutral-400 dark:text-zinc-500">{agent.name}</p>
                          </div>
                        </Link>
                      </td>
                      <td className="whitespace-nowrap px-4 py-3">
                        <ScheduleStatus enabled={schedule.enabled} running={Boolean(running)} />
                      </td>
                      <td className="whitespace-nowrap px-4 py-3 font-mono text-sm text-neutral-700 dark:text-zinc-300">{schedule.enabled ? schedule.interval : '-'}</td>
                      <td className="whitespace-nowrap px-4 py-3 text-sm text-neutral-600 dark:text-zinc-400">{schedule.activeHours || '-'}</td>
                      <td className="whitespace-nowrap px-4 py-3 text-sm text-neutral-600 dark:text-zinc-400">{schedule.maxTasksPerCycle || '-'}</td>
                      <td className="px-4 py-3">
                        <div className="flex max-w-xs flex-wrap gap-1.5">
                          {schedule.triggers.length === 0 ? (
                            <span className="text-sm text-neutral-400 dark:text-zinc-500">-</span>
                          ) : schedule.triggers.slice(0, 4).map(trigger => (
                            <span key={trigger} className="rounded-md border border-neutral-200 px-2 py-0.5 text-xs text-neutral-500 dark:border-zinc-700 dark:text-zinc-400">
                              {t(`schedule.trigger_${trigger}`, { defaultValue: t(`agents.trigger_${trigger}`, { defaultValue: trigger }) })}
                            </span>
                          ))}
                          {schedule.triggers.length > 4 && <span className="px-1 py-0.5 text-xs text-neutral-400">+{schedule.triggers.length - 4}</span>}
                        </div>
                      </td>
                      <td className="whitespace-nowrap px-4 py-3 text-sm text-neutral-600 dark:text-zinc-400">{agent.defaultRuntimeNodeId || t('agents.defaultRuntime')}</td>
                      <td className="whitespace-nowrap px-4 py-3 text-sm text-neutral-600 dark:text-zinc-400">
                        {summariesForAgent(cronSummaries, agent).reduce((total, item) => total + item.count, 0) || '-'}
                      </td>
                      <td className="whitespace-nowrap px-4 py-3 font-mono text-sm text-sky-700 dark:text-sky-400">
                        {nextRun ? fmt(nextRun) : '-'}
                      </td>
                      {canAdmin && (
                        <td className="whitespace-nowrap px-4 py-3">
                          <div className="flex flex-nowrap justify-end gap-2">
                            <button
                              type="button"
                              onClick={() => void toggleHeartbeat(agent)}
                              disabled={togglingId === agent.id || Boolean(running)}
                              className="whitespace-nowrap rounded-md border border-neutral-200 px-2.5 py-1.5 text-xs font-medium text-neutral-600 transition-colors hover:border-neutral-300 hover:bg-neutral-50 disabled:opacity-50 dark:border-zinc-700 dark:text-zinc-300 dark:hover:border-zinc-600 dark:hover:bg-zinc-800"
                              title={running ? t('schedule.running') : undefined}
                            >
                              {schedule.enabled ? t('schedule.disableHb') : t('schedule.enableHb')}
                            </button>
                          </div>
                        </td>
                      )}
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}

function ScheduleStatus({ enabled, running }: { enabled: boolean; running: boolean }) {
  const { t } = useTranslation()
  if (running) {
    return (
      <span className="inline-flex whitespace-nowrap items-center gap-1.5 rounded-full bg-emerald-50 px-2.5 py-0.5 text-xs font-medium text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300">
        <span className="size-1.5 rounded-full bg-emerald-500" />
        {t('schedule.running')}
      </span>
    )
  }
  if (enabled) {
    return (
      <span className="inline-flex whitespace-nowrap items-center gap-1.5 rounded-full bg-sky-50 px-2.5 py-0.5 text-xs font-medium text-sky-700 dark:bg-sky-950/40 dark:text-sky-300">
        <Zap className="size-3" strokeWidth={1.8} />
        {t('schedule.hbActive')}
      </span>
    )
  }
  return (
    <span className={cn('inline-flex whitespace-nowrap items-center gap-1.5 rounded-full bg-neutral-100 px-2.5 py-0.5 text-xs font-medium text-neutral-500 dark:bg-zinc-800 dark:text-zinc-400')}>
      <Pause className="size-3" strokeWidth={1.8} />
      {t('schedule.off')}
    </span>
  )
}
