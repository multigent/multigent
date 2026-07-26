import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Play, RefreshCw, X } from 'lucide-react'
import { WorkflowBoard, type WorkflowStep } from '../../components/workflow/WorkflowBoard'
import { ConversationLog } from '../../components/ui/ConversationLog'
import { PlaceholderCard } from '../../components/ui/PlaceholderCard'
import {
  WorkflowRuntimePanel,
  activeWorkflowStepInstance,
  isTerminal,
  startableAgentName,
  taskIdentityLabel,
  workflowHistoryRecords,
  type RunRow,
  type TaskRow,
  type TaskWorkflowData,
} from '../../components/task/TaskModals'
import { apiPost } from '../../lib/api'
import { canOperateAgent, useAuth } from '../../lib/auth'
import { cn } from '../../lib/cn'
import { useFormatDateTime } from '../../lib/format-datetime'
import { useApiJson } from '../../lib/use-api'
import { useWorkspaceAccess } from '../../lib/workspace-access'

type SafeUser = { username: string; displayName?: string; email?: string }
type ProjectMember = { name: string; model?: string; avatar?: string }
type LogData = { content: string; truncated: boolean }

const silentNotFound = [404]
const FOLLOW_POLL_MS = 3500

export default function ProjectTaskFollowPage() {
  const { t } = useTranslation()
  const fmt = useFormatDateTime()
  const { user } = useAuth()
  const { canAdmin } = useWorkspaceAccess()
  const { projectId = '', taskId = '' } = useParams<{ projectId: string; taskId: string }>()
  const [reloadKey, setReloadKey] = useState(0)
  const [reviewComments, setReviewComments] = useState('')
  const [reviewOutputs, setReviewOutputs] = useState<Record<string, string>>({})
  const [reviewBusy, setReviewBusy] = useState<string | null>(null)
  const [reviewErr, setReviewErr] = useState<string | null>(null)
  const [startBusy, setStartBusy] = useState(false)
  const [optimisticStartedAt, setOptimisticStartedAt] = useState<string | null>(null)

  const refresh = useCallback(() => setReloadKey((value) => value + 1), [])
  const tasksState = useApiJson<TaskRow[]>(
    projectId ? `/api/v1/projects/${encodeURIComponent(projectId)}/tasks?scope=all` : null,
    reloadKey,
    { keepPreviousDataOnReload: true },
  )
  const workflowState = useApiJson<TaskWorkflowData>(
    projectId && taskId ? `/api/v1/projects/${encodeURIComponent(projectId)}/tasks/${encodeURIComponent(taskId)}/workflow` : null,
    reloadKey,
    { silentStatuses: silentNotFound, keepPreviousDataOnReload: true },
  )
  const runsState = useApiJson<{ runs: RunRow[] }>(
    projectId ? `/api/v1/telemetry/runs?allTime=1&project=${encodeURIComponent(projectId)}&limit=200` : null,
    reloadKey,
    { keepPreviousDataOnReload: true },
  )
  const usersState = useApiJson<SafeUser[]>('/api/v1/users', 0, { silentStatuses: [403] })
  const membersState = useApiJson<ProjectMember[]>(
    projectId ? `/api/v1/projects/${encodeURIComponent(projectId)}/agents` : null,
    0,
    { silentStatuses: [403] },
  )

  const task = tasksState.status === 'ok' ? (tasksState.data ?? []).find((item) => item.id === taskId) : undefined
  const displayTask = useMemo(() => {
    if (!task || !optimisticStartedAt || task.status === 'in_progress' || isTerminal(task.status)) return task
    return { ...task, status: 'in_progress', startedAt: task.startedAt || optimisticStartedAt, updatedAt: optimisticStartedAt }
  }, [optimisticStartedAt, task])
  const rawWorkflowData = workflowState.status === 'ok' ? workflowState.data : null
  const workflowData = useMemo(
    () => withRunningActiveStep(rawWorkflowData, displayTask, projectId, optimisticStartedAt),
    [displayTask, optimisticStartedAt, projectId, rawWorkflowData],
  )
  const activeStep = workflowData
    ? workflowData.definition.steps.find((step) => step.id === workflowData.run.activeStepId)
    : undefined
  const activeInstance = workflowData ? activeWorkflowStepInstance(workflowData) : undefined
  const workflowRecords = workflowData ? workflowHistoryRecords(workflowData) : []
  const runs = runsState.status === 'ok' ? (runsState.data.runs ?? []) : []
  const activeRun = useMemo(() => findActiveRun(runs, taskId, projectId, activeStep, activeInstance?.actorId), [activeInstance?.actorId, activeStep, projectId, runs, taskId])
  const logState = useApiJson<LogData>(
    activeRun?.logPath ? `/api/v1/telemetry/log?path=${encodeURIComponent(activeRun.logPath)}` : null,
    reloadKey,
    { keepPreviousDataOnReload: true },
  )

  const actorLabels = useMemo(() => {
    const labels = new Map<string, string>()
    if (usersState.status === 'ok') {
      for (const item of usersState.data ?? []) {
        labels.set(item.username, item.displayName || item.email || item.username)
      }
    }
    if (membersState.status === 'ok') {
      for (const member of membersState.data ?? []) {
        if (!member.name) continue
        if (member.model === 'human') {
          labels.set(member.name, labels.get(member.name) || member.name)
        } else {
          labels.set(`${projectId}/${member.name}`, member.name)
        }
      }
    }
    if (displayTask?.assignee) labels.set(displayTask.assignee, taskIdentityLabel(displayTask.assignee, displayTask.assigneeLabel))
    return labels
  }, [displayTask?.assignee, displayTask?.assigneeLabel, membersState, projectId, usersState])

  const startAgent = activeInstance?.actorType === 'agent'
    ? agentNameFromActor(projectId, activeInstance.actorId)
    : displayTask ? startableAgentName(displayTask) : null
  const canStart = Boolean(
    displayTask &&
    startAgent &&
    displayTask.status !== 'in_progress' &&
    !isTerminal(displayTask.status) &&
    (canAdmin || canOperateAgent(user, projectId, startAgent)),
  )
  const canReview = activeStep?.type === 'human_review'

  useEffect(() => {
    if (!displayTask || isTerminal(displayTask.status)) return
    const timer = window.setInterval(refresh, FOLLOW_POLL_MS)
    return () => window.clearInterval(timer)
  }, [refresh, displayTask?.id, displayTask?.status])

  useEffect(() => {
    if (!optimisticStartedAt) return
    if (!task || task.status === 'in_progress' || isTerminal(task.status)) setOptimisticStartedAt(null)
  }, [optimisticStartedAt, task, task?.status])

  useEffect(() => {
    setReviewComments('')
    setReviewOutputs({})
    setReviewErr(null)
  }, [activeStep?.id, activeInstance?.id])

  async function startCurrentAgent() {
    if (!displayTask || !startAgent || !canStart) return
    setStartBusy(true)
    setOptimisticStartedAt(new Date().toISOString())
    try {
      await apiPost('/api/v1/scheduler/wakeup', { project: displayTask.project, agent: startAgent })
      refresh()
      window.setTimeout(refresh, 1000)
    } catch {
      setOptimisticStartedAt(null)
      // Toast handled by API layer.
    } finally {
      setStartBusy(false)
    }
  }

  async function submitWorkflowReview(decision?: string) {
    if (!displayTask || !activeStep) return
    setReviewErr(null)
    const normalizedDecision = normalizeReviewDecision(decision || reviewOutputs.decision || '')
    setReviewBusy(normalizedDecision || 'submit')
    const outputs: Record<string, string> = { ...reviewOutputs }
    if (normalizedDecision) outputs.decision = normalizedDecision
    const outputFieldNames = (activeStep.outputFields ?? []).map((field) => field.name).filter(Boolean)
    const comments = (outputs.comments ?? reviewComments).trim()
    if (outputFieldNames.includes('comments')) outputs.comments = comments
    const missingField = outputFieldNames.find((name) => !String(outputs[name] ?? '').trim())
    if (missingField) {
      setReviewErr(`${t('forms.fillRequired')} ${missingField}`)
      setReviewBusy(null)
      return
    }
    try {
      await apiPost(`/api/v1/projects/${encodeURIComponent(displayTask.project)}/tasks/${encodeURIComponent(displayTask.id)}/workflow/review`, {
        decision,
        comments,
        outputs,
      })
      setReviewComments('')
      setReviewOutputs({})
      refresh()
      window.setTimeout(refresh, 800)
    } catch (e) {
      setReviewErr(e instanceof Error ? e.message : String(e))
    } finally {
      setReviewBusy(null)
    }
  }

  return (
    <div className="fixed inset-0 z-[70] flex flex-col bg-neutral-50 text-neutral-900 dark:bg-zinc-950 dark:text-zinc-100">
      <header className="flex h-14 shrink-0 items-center justify-between border-b border-neutral-200/80 bg-white/90 px-4 backdrop-blur-md dark:border-zinc-800 dark:bg-zinc-950/90">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-xs text-neutral-400 dark:text-zinc-500">
            <Link to={`/projects/${encodeURIComponent(projectId)}/tasks`} className="hover:text-sky-700 dark:hover:text-sky-400">
              {t('projectNav.tasks')}
            </Link>
            <span>/</span>
            <span>{t('tasks.follow')}</span>
          </div>
          <h1 className="truncate text-sm font-semibold text-neutral-900 dark:text-zinc-100">{displayTask?.title || taskId}</h1>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          {displayTask && (
            <span className="rounded-full border border-neutral-200 bg-white px-2.5 py-1 text-xs text-neutral-500 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-400">
              {t(`tasks.status.${displayTask.status}`, { defaultValue: displayTask.status })}
            </span>
          )}
          <button
            type="button"
            onClick={refresh}
            title={t('common.refresh')}
            className="rounded-lg border border-neutral-200 bg-white p-1.5 text-neutral-500 hover:bg-neutral-50 hover:text-neutral-800 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100"
          >
            <RefreshCw className="size-4" />
          </button>
          <Link to={`/projects/${encodeURIComponent(projectId)}/tasks`} className="rounded-lg border border-neutral-200 bg-white p-1.5 text-neutral-500 hover:bg-neutral-50 hover:text-neutral-800 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100">
            <X className="size-4" />
          </Link>
        </div>
      </header>

      <main className="grid min-h-0 flex-1 grid-cols-[minmax(0,1fr)_420px] gap-0 overflow-hidden">
        <section className="min-w-0 border-r border-neutral-200/80 bg-white dark:border-zinc-800 dark:bg-zinc-950">
          {workflowData ? (
            <WorkflowBoard definition={workflowData.definition} run={workflowData.run} instances={workflowData.steps} focusActive fill hideInspector />
          ) : workflowState.status === 'loading' ? (
            <CenteredLoading label={t('tasks.followLoadingWorkflow')} />
          ) : (
            <div className="flex h-full items-center justify-center p-8">
              <PlaceholderCard title={t('tasks.noWorkflowTitle')}>
                <p>{t('tasks.noWorkflowBody')}</p>
              </PlaceholderCard>
            </div>
          )}
        </section>

        <aside className="min-w-0 overflow-y-auto bg-white dark:bg-zinc-950">
          <div className="border-b border-neutral-200/80 px-4 py-3 dark:border-zinc-800">
            <p className="text-xs font-semibold uppercase tracking-wider text-neutral-400 dark:text-zinc-500">{t('tasks.followCurrent')}</p>
            <h2 className="mt-1 text-base font-semibold text-neutral-900 dark:text-zinc-100">{activeStep?.title || t('workflows.detail.notSpecified')}</h2>
            <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-neutral-500 dark:text-zinc-500">
              {activeInstance?.actorId && <span>{t('tasks.colAssignee')}: {actorLabels.get(activeInstance.actorId) || activeInstance.actorId}</span>}
              {activeInstance?.status && <span>{t('api.taskColStatus')}: {t(`workflows.stepStatus.${activeInstance.status}`, { defaultValue: activeInstance.status })}</span>}
              {activeRun?.startedAt && <span>{t('tasks.startedAt')}: {fmt(activeRun.startedAt)}</span>}
            </div>
            <div className="mt-3 flex items-center gap-2">
              <button
                type="button"
                onClick={() => void startCurrentAgent()}
                disabled={!canStart || startBusy || displayTask?.status === 'in_progress'}
                className="rounded-lg border border-sky-600 bg-white px-3 py-2 text-sm font-medium text-sky-700 hover:bg-sky-50 disabled:cursor-not-allowed disabled:opacity-40 dark:border-sky-500 dark:bg-zinc-900 dark:text-sky-400 dark:hover:bg-zinc-800"
              >
                <span className="inline-flex items-center gap-1.5"><Play className={cn('size-3.5', (startBusy || displayTask?.status === 'in_progress') && 'animate-pulse')} />{displayTask?.status === 'in_progress' ? t('tasks.status.in_progress') : startBusy ? t('tasks.starting') : t('tasks.startCurrentAgent')}</span>
              </button>
              {startAgent && <span className="font-mono text-xs text-neutral-400 dark:text-zinc-500">{startAgent}</span>}
            </div>
          </div>

          {workflowData && (
            <WorkflowRuntimePanel
              step={activeStep}
              instance={activeInstance}
              steps={workflowData.definition.steps}
              records={workflowRecords}
              runs={runs}
              taskID={taskId}
              actorLabels={actorLabels}
              canReview={canReview}
              reviewOutputs={reviewOutputs}
              reviewComments={reviewComments}
              reviewBusy={reviewBusy}
              reviewErr={reviewErr}
              onChangeOutput={(name, value) => {
                setReviewOutputs((current) => ({ ...current, [name]: value }))
                if (name === 'comments') setReviewComments(value)
              }}
              onChangeComments={setReviewComments}
              onSubmitReview={(decision) => void submitWorkflowReview(decision)}
            />
          )}

          <section className="border-t border-neutral-200/80 px-4 py-4 dark:border-zinc-800">
            <div className="mb-3 flex items-center justify-between gap-2">
              <div>
                <p className="text-xs font-semibold uppercase tracking-wider text-neutral-400 dark:text-zinc-500">{t('tasks.followLiveOutput')}</p>
                <h3 className="mt-1 text-sm font-semibold text-neutral-900 dark:text-zinc-100">{activeRun ? `${activeRun.agent} · ${activeRun.status}` : t('tasks.noActiveRun')}</h3>
              </div>
              {activeRun?.sessionId && <span className="font-mono text-[11px] text-neutral-400 dark:text-zinc-500">{activeRun.sessionId.slice(0, 8)}…</span>}
            </div>
            <div className="min-h-72">
              {activeRun?.logPath && logState.status === 'ok' ? (
                <ConversationLog content={logState.data.content} mode="chat" assistant={{ name: activeRun.agent }} />
              ) : activeRun?.logPath && logState.status === 'loading' ? (
                <CenteredLoading label={t('tasks.followLoadingOutput')} compact />
              ) : (
                <p className="py-8 text-center text-sm text-neutral-400 dark:text-zinc-500">{t('tasks.followNoOutput')}</p>
              )}
            </div>
          </section>
        </aside>
      </main>
    </div>
  )
}

function CenteredLoading({ label, compact = false }: { label: string; compact?: boolean }) {
  return (
    <div className={cn('flex items-center justify-center gap-2 text-sm text-neutral-500 dark:text-zinc-500', compact ? 'py-10' : 'h-full')}>
      <div className="size-4 animate-spin rounded-full border-2 border-neutral-300 border-t-sky-600 dark:border-zinc-600 dark:border-t-sky-400" />
      <span>{label}</span>
    </div>
  )
}

function findActiveRun(runs: RunRow[], taskID: string, projectID: string, step?: WorkflowStep, actorID?: string): RunRow | null {
  const actorAgent = agentNameFromActor(projectID, actorID)
  const stepAgent = agentNameFromActor(projectID, step?.actorRole)
  const preferredAgent = actorAgent || stepAgent
  const candidates = runs
    .filter((run) => run.taskId === taskID)
    .filter((run) => !preferredAgent || run.agent === preferredAgent || `${run.project}/${run.agent}` === preferredAgent)
    .sort((a, b) => Date.parse(b.startedAt || '') - Date.parse(a.startedAt || ''))
  if (candidates.length > 0) return candidates[0]
  return runs
    .filter((run) => run.taskId === taskID)
    .sort((a, b) => Date.parse(b.startedAt || '') - Date.parse(a.startedAt || ''))[0] ?? null
}

function withRunningActiveStep(
  data: TaskWorkflowData | null,
  task: TaskRow | undefined,
  projectID: string,
  optimisticStartedAt: string | null,
): TaskWorkflowData | null {
  if (!data || !task || task.status !== 'in_progress' || !data.run.activeStepId) return data
  const current = activeWorkflowStepInstance(data)
  const currentStep = data.definition.steps.find((step) => step.id === data.run.activeStepId)
  if (current?.status === 'completed' || current?.status === 'done_success' || current?.status === 'done_failed') return data
  if (current?.actorType === 'human' || currentStep?.type === 'human_review') return data

  const taskAgent = startableAgentName(task) || task.agent
  const actorAgent = agentNameFromActor(projectID, current?.actorId || currentStep?.actorRole)
  if (actorAgent && taskAgent && actorAgent !== taskAgent) return data

  const startedAt = current?.startedAt || task.startedAt || optimisticStartedAt || new Date().toISOString()
  return {
    ...data,
    run: {
      ...data.run,
      status: data.run.status === 'completed' ? data.run.status : 'running',
      updatedAt: task.updatedAt || data.run.updatedAt,
    },
    steps: data.steps.map((step) => {
      if (step.stepId !== data.run.activeStepId) return step
      return {
        ...step,
        status: 'running',
        startedAt,
        updatedAt: task.updatedAt || step.updatedAt,
      }
    }),
  }
}

function agentNameFromActor(projectID: string, actorID?: string): string | null {
  const raw = String(actorID || '').trim()
  if (!raw) return null
  const prefix = `${projectID}/`
  if (raw.startsWith(prefix)) return raw.slice(prefix.length) || null
  if (raw.includes('/')) return null
  return raw
}

function normalizeReviewDecision(decision: string) {
  switch (decision.trim()) {
    case 'approved':
      return 'approve'
    case 'needs_changes':
      return 'request_changes'
    default:
      return decision.trim()
  }
}
