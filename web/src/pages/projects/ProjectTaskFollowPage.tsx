import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Play, RefreshCw, X } from 'lucide-react'
import { WorkflowBoard } from '../../components/workflow/WorkflowBoard'
import { ConversationLog } from '../../components/ui/ConversationLog'
import { PlaceholderCard } from '../../components/ui/PlaceholderCard'
import {
  WorkflowRuntimePanel,
  activeWorkflowStepInstance,
  isOptionalTerminalReviewDecision,
  isTerminal,
  isWorkflowStepOpen,
  startableAgentName,
  statusColor,
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
type LiveLogData = { content: string; path: string; finished: boolean }

const silentNotFound = [404]
const silentForbidden = [403]
const staticSilentForbiddenOptions = { silentStatuses: silentForbidden }
const FOLLOW_POLL_MS = 3500

export default function ProjectTaskFollowPage() {
  const { t } = useTranslation()
  const fmt = useFormatDateTime()
  const { user } = useAuth()
  const { canAdmin } = useWorkspaceAccess()
  const { projectId = '', taskId = '' } = useParams<{ projectId: string; taskId: string }>()
  const [reloadKey, setReloadKey] = useState(0)
  const [pollKey, setPollKey] = useState(0)
  const [reviewComments, setReviewComments] = useState('')
  const [reviewOutputs, setReviewOutputs] = useState<Record<string, string>>({})
  const [reviewBusy, setReviewBusy] = useState<string | null>(null)
  const [reviewErr, setReviewErr] = useState<string | null>(null)
  const [startBusy, setStartBusy] = useState(false)
  const [optimisticStartedAt, setOptimisticStartedAt] = useState<string | null>(null)
  const [handoffStartedAt, setHandoffStartedAt] = useState<string | null>(null)
  const [showLiveTail, setShowLiveTail] = useState(false)
  const [stepTransition, setStepTransition] = useState<{ from?: string; to: string; at: number } | null>(null)
  const activeStepRef = useRef<string | null>(null)
  const sidePanelRef = useRef<HTMLElement>(null)
  const liveSectionRef = useRef<HTMLElement>(null)
  const liveOutputRef = useRef<HTMLDivElement>(null)

  const refresh = useCallback(() => setReloadKey((value) => value + 1), [])
  const pollRefresh = useCallback(() => setPollKey((value) => value + 1), [])
  const dataReloadKey = reloadKey + pollKey
  const tasksState = useApiJson<TaskRow[]>(
    projectId ? `/api/v1/projects/${encodeURIComponent(projectId)}/tasks?scope=all` : null,
    dataReloadKey,
    { keepPreviousDataOnReload: true },
  )
  const workflowState = useApiJson<TaskWorkflowData>(
    projectId && taskId ? `/api/v1/projects/${encodeURIComponent(projectId)}/tasks/${encodeURIComponent(taskId)}/workflow` : null,
    dataReloadKey,
    { silentStatuses: silentNotFound, keepPreviousDataOnReload: true },
  )
  const usersState = useApiJson<SafeUser[]>('/api/v1/users', 0, staticSilentForbiddenOptions)
  const membersState = useApiJson<ProjectMember[]>(
    projectId ? `/api/v1/projects/${encodeURIComponent(projectId)}/agents` : null,
    0,
    staticSilentForbiddenOptions,
  )

  const task = tasksState.status === 'ok' ? (tasksState.data ?? []).find((item) => item.id === taskId) : undefined
  const rawWorkflowData = workflowState.status === 'ok' ? workflowState.data : null
  const rawActiveStep = rawWorkflowData
    ? rawWorkflowData.definition.steps.find((step) => step.id === rawWorkflowData.run.activeStepId)
    : undefined
  const rawActiveInstance = rawWorkflowData ? activeWorkflowStepInstance(rawWorkflowData) : undefined
  const rawIsAgentStep = rawActiveInstance?.actorType === 'agent' || rawActiveStep?.type === 'agent_task'
  const optimisticRunStartedAt = optimisticStartedAt || (rawIsAgentStep ? handoffStartedAt : null)
  const displayTask = useMemo(() => {
    if (!task || task.status === 'in_progress' || isTerminal(task.status)) return task
    if (!optimisticRunStartedAt) return task
    const startedAt = rawActiveInstance?.startedAt || optimisticRunStartedAt || task.startedAt || new Date().toISOString()
    return { ...task, status: 'in_progress', startedAt, updatedAt: task.updatedAt || startedAt }
  }, [optimisticRunStartedAt, rawActiveInstance?.startedAt, task])
  const workflowData = useMemo(
    () => withRunningActiveStep(rawWorkflowData, displayTask, projectId, optimisticRunStartedAt, Boolean(handoffStartedAt && rawIsAgentStep)),
    [displayTask, handoffStartedAt, optimisticRunStartedAt, projectId, rawIsAgentStep, rawWorkflowData],
  )
  const activeInstance = workflowData ? activeWorkflowStepInstance(workflowData) : undefined
  const activeStep = workflowData
    ? workflowData.definition.steps.find((step) => step.id === (activeInstance?.stepId || workflowData.run.activeStepId))
    : undefined
  const workflowRecords = workflowData ? workflowHistoryRecords(workflowData) : []
  const isCurrentAgentStep = activeInstance?.actorType === 'agent' || activeStep?.type === 'agent_task'
  const currentTaskAgent = displayTask ? startableAgentName(displayTask) : null
  const currentStepAgent = isCurrentAgentStep
    ? agentNameFromActor(projectId, activeInstance?.actorId) || workflowActorAgentForStep(workflowData?.run.actorBindings, activeStep)
    : null
  const startAgent = currentStepAgent || (isCurrentAgentStep ? currentTaskAgent : null)
  const activeStepRunning = isCurrentAgentStep && isRunningStepStatus(activeInstance?.status)
  const runsState = useApiJson<{ runs: RunRow[] }>(
    isCurrentAgentStep && projectId ? `/api/v1/telemetry/runs?allTime=1&project=${encodeURIComponent(projectId)}&limit=200` : null,
    dataReloadKey,
    { keepPreviousDataOnReload: true },
  )
  const runs = runsState.status === 'ok' ? (runsState.data.runs ?? []) : []
  const activeRun = useMemo(
    () => isCurrentAgentStep ? findActiveRun(runs, taskId, projectId, activeInstance?.actorId, activeInstance?.startedAt) : null,
    [activeInstance?.actorId, activeInstance?.startedAt, isCurrentAgentStep, projectId, runs, taskId],
  )
  const activeRunRunning = Boolean(activeRun && isRunningRunStatus(activeRun.status))
  const displayStatus = activeRunRunning || activeStepRunning ? 'in_progress' : (displayTask?.status || '')
  const shouldPollActiveRun = Boolean(isCurrentAgentStep && (activeStepRunning || displayTask?.status === 'in_progress' || activeRunRunning))
  const shouldPollLocalLiveLog = Boolean(shouldPollActiveRun && startAgent && !activeRun?.runtimeRunId)
  const localLiveLogAgent = shouldPollLocalLiveLog && startAgent ? startAgent : null
  const liveLogState = useApiJson<LiveLogData>(
    localLiveLogAgent ? `/api/v1/projects/${encodeURIComponent(projectId)}/agents/${encodeURIComponent(localLiveLogAgent)}/live-log` : null,
    dataReloadKey,
    { keepPreviousDataOnReload: true },
  )
  const logState = useApiJson<LogData>(
    isCurrentAgentStep && !shouldPollActiveRun && activeRun?.logPath ? `/api/v1/telemetry/log?path=${encodeURIComponent(activeRun.logPath)}` : null,
    dataReloadKey,
    { keepPreviousDataOnReload: true },
  )
  const liveLogContent = liveLogState.status === 'ok' ? liveLogState.data.content : ''
  const remoteLogContent = activeRun?.logText || ''
  const liveLogFinished = liveLogState.status === 'ok' && (liveLogState.data.finished || isLiveLogTerminal(liveLogContent))

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
  const agentParticipants = useMemo(() => {
    const participants = new Map<string, { name: string; avatar?: string }>()
    if (membersState.status !== 'ok') return participants
    for (const member of membersState.data ?? []) {
      if (!member.name || member.model === 'human') continue
      const participant = { name: member.name, avatar: member.avatar }
      participants.set(member.name, participant)
      participants.set(`${projectId}/${member.name}`, participant)
    }
    return participants
  }, [membersState, projectId])
  const activeAgentParticipant = useMemo(() => {
    const name = startAgent || activeRun?.agent || t('tasks.noActiveRun')
    const participant = agentParticipants.get(name) || agentParticipants.get(agentNameFromActor(projectId, name) || '')
    return participant || { name: agentNameFromActor(projectId, name) || name }
  }, [activeRun?.agent, agentParticipants, projectId, startAgent, t])
  const liveOutputRunning = Boolean(
    shouldPollActiveRun &&
    !remoteLogContent.trim() &&
    displayStatus === 'in_progress' &&
    (!liveLogState || liveLogState.status !== 'ok' || !liveLogFinished),
  )

  const canStart = Boolean(
    displayTask &&
    startAgent &&
    displayStatus !== 'in_progress' &&
    !isTerminal(displayTask.status) &&
    (canAdmin || canOperateAgent(user, projectId, startAgent)),
  )
  const canReview = Boolean(activeStep?.type === 'human_review' && isWorkflowStepOpen(activeInstance?.status) && !isTerminal(displayTask?.status || ''))

  useEffect(() => {
    if (!shouldPollActiveRun) return
    const timer = window.setInterval(pollRefresh, FOLLOW_POLL_MS)
    return () => window.clearInterval(timer)
  }, [pollRefresh, shouldPollActiveRun])

  useEffect(() => {
    activeStepRef.current = null
    setHandoffStartedAt(null)
    setOptimisticStartedAt(null)
  }, [taskId])

  useEffect(() => {
    const activeStepID = rawWorkflowData?.run.activeStepId || null
    if (!activeStepID) return
    const previousStepID = activeStepRef.current
    activeStepRef.current = activeStepID
    if (!previousStepID || previousStepID === activeStepID) return
    setStepTransition({ from: previousStepID, to: activeStepID, at: Date.now() })
    if (rawIsAgentStep && rawActiveInstance?.status === 'running' && task && !isTerminal(task.status)) {
      setHandoffStartedAt(new Date().toISOString())
    }
  }, [rawActiveInstance?.status, rawIsAgentStep, rawWorkflowData?.run.activeStepId, task])

  useEffect(() => {
    if (!stepTransition) return
    const timer = window.setTimeout(() => setStepTransition(null), 1200)
    return () => window.clearTimeout(timer)
  }, [stepTransition])

  useEffect(() => {
    if (!optimisticStartedAt && !handoffStartedAt) return
    if (!task || task.status === 'in_progress' || isTerminal(task.status)) {
      setOptimisticStartedAt(null)
      setHandoffStartedAt(null)
    }
  }, [handoffStartedAt, optimisticStartedAt, task, task?.status])

  useEffect(() => {
    if (!handoffStartedAt || task?.status === 'in_progress') return
    const timer = window.setTimeout(() => {
      setHandoffStartedAt(null)
      refresh()
    }, 20000)
    return () => window.clearTimeout(timer)
  }, [handoffStartedAt, refresh, task?.status])

  useEffect(() => {
    setReviewComments('')
    setReviewOutputs({})
    setReviewErr(null)
  }, [activeStep?.id, activeInstance?.id])

  useEffect(() => {
    if (!liveOutputRunning) {
      setShowLiveTail(false)
      return
    }
    if (!liveLogContent.trim()) {
      setShowLiveTail(true)
      return
    }
    setShowLiveTail(false)
    const delay = Math.min(2200, Math.max(900, liveLogContent.length * 8))
    const timer = window.setTimeout(() => setShowLiveTail(true), delay)
    return () => window.clearTimeout(timer)
  }, [liveLogContent, liveOutputRunning])

  useEffect(() => {
    if (!shouldPollActiveRun || !liveOutputRef.current) return
    const outputEl = liveOutputRef.current
    const panelEl = sidePanelRef.current
    const scroll = () => {
      if (panelEl) panelEl.scrollTop = panelEl.scrollHeight
      outputEl.scrollTop = outputEl.scrollHeight
    }
    scroll()
    const raf = window.requestAnimationFrame(scroll)
    const timer = window.setTimeout(scroll, 80)
    return () => {
      window.cancelAnimationFrame(raf)
      window.clearTimeout(timer)
    }
  }, [activeRun?.logPath, activeRun?.sessionId, activeStep?.id, liveLogContent, liveOutputRunning, remoteLogContent, shouldPollActiveRun])

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
    const outputs: Record<string, string> = Object.fromEntries(Object.entries(reviewOutputs).map(([key, value]) => [key, String(value ?? '').trim()]))
    if (normalizedDecision) outputs.decision = normalizedDecision
    const outputFieldNames = (activeStep.outputFields ?? []).map((field) => field.name).filter(Boolean)
    const comments = (outputs.comments ?? reviewComments).trim()
    if (outputFieldNames.includes('comments')) outputs.comments = comments
    const decisionOptional = isOptionalTerminalReviewDecision(activeStep, workflowData?.definition)
    const missingField = outputFieldNames.find((name) => !(name === 'decision' && decisionOptional) && !String(outputs[name] ?? '').trim())
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
      setHandoffStartedAt(new Date().toISOString())
      refresh()
      window.setTimeout(refresh, 300)
      window.setTimeout(refresh, 800)
      window.setTimeout(refresh, 1600)
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
            <span className={cn('rounded-full px-2.5 py-1 text-xs font-medium', statusColor[displayStatus] ?? statusColor.pending)}>
              {displayStatus === 'in_progress' && <span className="mr-1.5 inline-block size-1.5 animate-pulse rounded-full bg-current align-middle" />}
              {t(`tasks.status.${displayStatus}`, { defaultValue: displayStatus })}
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
            <WorkflowBoard key={`${workflowData.run.id}:${workflowData.run.activeStepId || ''}`} definition={workflowData.definition} run={workflowData.run} instances={workflowData.steps} branches={workflowData.branches} focusActive fill hideInspector />
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

        <aside ref={sidePanelRef} className="min-w-0 overflow-y-auto bg-white dark:bg-zinc-950">
          {stepTransition && (
            <div className="sticky top-0 z-10 border-b border-sky-100 bg-sky-50/95 px-4 py-2 text-xs font-medium text-sky-700 shadow-sm backdrop-blur-sm dark:border-sky-900/50 dark:bg-sky-950/80 dark:text-sky-300">
              <div className="flex items-center gap-2">
                <span className="size-1.5 rounded-full bg-sky-500" />
                <span>{t('tasks.followHandoff', { defaultValue: 'Moving to the next step…' })}</span>
              </div>
            </div>
          )}
          <div className="border-b border-neutral-200/80 px-4 py-3 dark:border-zinc-800">
            <p className="text-xs font-semibold uppercase tracking-wider text-neutral-400 dark:text-zinc-500">{t('tasks.followCurrent')}</p>
            <h2 className="mt-1 text-base font-semibold text-neutral-900 dark:text-zinc-100">{activeStep?.title || t('workflows.detail.notSpecified')}</h2>
            <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-neutral-500 dark:text-zinc-500">
              {activeInstance?.actorId && <span>{t('tasks.colAssignee')}: {actorLabels.get(activeInstance.actorId) || activeInstance.actorId}</span>}
              {activeInstance?.status && <span>{t('api.taskColStatus')}: {t(`workflows.stepStatus.${activeInstance.status}`, { defaultValue: activeInstance.status })}</span>}
              {activeRun?.startedAt && <span>{t('tasks.startedAt')}: {fmt(activeRun.startedAt)}</span>}
            </div>
            {isCurrentAgentStep && (
              <div className="mt-3 flex items-center gap-2">
                <button
                  type="button"
                  onClick={() => void startCurrentAgent()}
                  disabled={!canStart || startBusy || displayStatus === 'in_progress'}
                  className="rounded-lg border border-sky-600 bg-white px-3 py-2 text-sm font-medium text-sky-700 hover:bg-sky-50 disabled:cursor-not-allowed disabled:opacity-40 dark:border-sky-500 dark:bg-zinc-900 dark:text-sky-400 dark:hover:bg-zinc-800"
                >
                  <span className="inline-flex items-center gap-1.5"><Play className={cn('size-3.5', (startBusy || displayStatus === 'in_progress') && 'animate-pulse')} />{displayStatus === 'in_progress' ? t('tasks.status.in_progress') : startBusy ? t('tasks.starting') : t('tasks.startCurrentAgent')}</span>
                </button>
                {startAgent && <span className="font-mono text-xs text-neutral-400 dark:text-zinc-500">{startAgent}</span>}
              </div>
            )}
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
              hideHeader
              reviewOutputs={reviewOutputs}
              reviewComments={reviewComments}
              reviewBusy={reviewBusy}
              reviewErr={reviewErr}
              docTitles={workflowData.docTitles}
              onChangeOutput={(name, value) => {
                setReviewOutputs((current) => ({ ...current, [name]: value }))
                if (name === 'comments') setReviewComments(value)
              }}
              onChangeComments={setReviewComments}
              onSubmitReview={(decision) => void submitWorkflowReview(decision)}
            />
          )}

          {isCurrentAgentStep && (
            <section ref={liveSectionRef} className="border-t border-neutral-200/80 px-4 py-4 dark:border-zinc-800">
              <div className="mb-3 flex items-center justify-between gap-2">
                <div>
                  <p className="text-xs font-semibold uppercase tracking-wider text-neutral-400 dark:text-zinc-500">{t('tasks.followLiveOutput')}</p>
                  <h3 className="mt-1 text-sm font-semibold text-neutral-900 dark:text-zinc-100">{shouldPollActiveRun && startAgent ? `${startAgent} · ${t('tasks.status.in_progress')}` : activeRun ? `${activeRun.agent} · ${activeRun.status}` : t('tasks.noActiveRun')}</h3>
                </div>
                {activeRun?.sessionId && <span className="font-mono text-[11px] text-neutral-400 dark:text-zinc-500">{activeRun.sessionId.slice(0, 8)}…</span>}
              </div>
              <div ref={liveOutputRef} className="max-h-[calc(100dvh-25rem)] min-h-72 overflow-y-auto overscroll-contain pr-1 transition-opacity duration-300">
                {remoteLogContent ? (
                  <div className="pb-16">
                    <ConversationLog
                      content={remoteLogContent}
                      mode="chat"
                      assistant={activeAgentParticipant}
                      animateLatest={isRunningRunStatus(activeRun?.status)}
                      toolDisplay="compact"
                    />
                  </div>
                ) : shouldPollLocalLiveLog && liveLogState.status === 'ok' ? (
                  <div className="pb-16">
                    {liveLogContent ? (
                      <ConversationLog
                        content={liveLogContent}
                        mode="chat"
                        assistant={activeAgentParticipant}
                        animateLatest
                        toolDisplay="compact"
                        emptyFallback={null}
                      />
                    ) : null}
                    {showLiveTail && <LiveOutputTail participant={activeAgentParticipant} />}
                  </div>
                ) : shouldPollActiveRun && activeRun?.runtimeRunId ? (
                  <LiveOutputTail participant={activeAgentParticipant} />
                ) : shouldPollLocalLiveLog && liveLogState.status === 'loading' ? (
                  <CenteredLoading label={t('tasks.followLoadingOutput')} compact />
                ) : activeRun?.logPath && logState.status === 'ok' ? (
                  <div className="pb-16">
                    <ConversationLog content={logState.data.content} mode="chat" assistant={activeAgentParticipant} toolDisplay="compact" />
                  </div>
                ) : activeRun?.logPath && logState.status === 'loading' ? (
                  <CenteredLoading label={t('tasks.followLoadingOutput')} compact />
                ) : (
                  <p className="py-8 text-center text-sm text-neutral-400 dark:text-zinc-500">{t('tasks.followNoOutput')}</p>
                )}
              </div>
            </section>
          )}
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

function LiveOutputTail({ participant }: { participant: { name: string; avatar?: string } }) {
  const initial = [...(participant.name || 'A').trim()][0]?.toUpperCase() || 'A'
  return (
    <div className="mt-4 flex items-start gap-2.5 pl-0.5" aria-label="running">
      <div className="flex size-6 shrink-0 items-center justify-center overflow-hidden rounded-full bg-neutral-100 text-[11px] font-semibold text-sky-700 dark:bg-zinc-800 dark:text-sky-400">
        {participant.avatar ? <img src={participant.avatar} alt="" className="size-full object-cover" /> : initial}
      </div>
      <div className="min-w-0">
        <p className="mb-1 text-xs font-medium text-sky-700 dark:text-sky-400">{participant.name}</p>
        <div className="w-40 rounded-lg bg-neutral-50 px-3.5 py-3 dark:bg-zinc-900/70">
          <div className="space-y-2">
            <span className="block h-2 w-28 animate-pulse rounded-full bg-neutral-200/80 dark:bg-zinc-700/70" />
            <span className="block h-2 w-16 animate-pulse rounded-full bg-sky-200/80 dark:bg-sky-800/60" />
          </div>
        </div>
      </div>
    </div>
  )
}

function findActiveRun(runs: RunRow[], taskID: string, projectID: string, actorID?: string, minStartedAt?: string): RunRow | null {
  const actorAgent = agentNameFromActor(projectID, actorID)
  const preferredAgent = actorAgent
  if (!preferredAgent) return null
  const minTime = minStartedAt ? Date.parse(minStartedAt) : 0
  const candidates = runs
    .filter((run) => run.taskId === taskID)
    .filter((run) => run.agent === preferredAgent || `${run.project}/${run.agent}` === preferredAgent)
    .filter((run) => !minTime || Date.parse(run.startedAt || '') >= minTime)
    .sort((a, b) => Date.parse(b.startedAt || '') - Date.parse(a.startedAt || ''))
  if (candidates.length > 0) return candidates[0]
  return null
}

function isRunningRunStatus(status?: string): boolean {
  const normalized = String(status || '').trim().toLowerCase()
  return normalized === 'running' || normalized === 'in_progress'
}

function isRunningStepStatus(status?: string): boolean {
  const normalized = String(status || '').trim().toLowerCase()
  return normalized === 'running' || normalized === 'in_progress'
}

function isLiveLogTerminal(content: string): boolean {
  return content.includes('=== exit code:') ||
    content.includes('=== finished:') ||
    content.includes('exec complete') ||
    content.includes('"type":"chat_done"') ||
    content.includes('agent exited with error') ||
    content.includes('status : done_success') ||
    content.includes('status : done_failed') ||
    content.includes('status  : done_success') ||
    content.includes('status  : done_failed')
}

function withRunningActiveStep(
  data: TaskWorkflowData | null,
  task: TaskRow | undefined,
  projectID: string,
  optimisticStartedAt: string | null,
  allowActorMismatch = false,
): TaskWorkflowData | null {
  if (!data || !task || task.status !== 'in_progress' || !data.run.activeStepId) return data
  const current = activeWorkflowStepInstance(data)
  const currentStep = data.definition.steps.find((step) => step.id === data.run.activeStepId)
  if (current?.status === 'completed' || current?.status === 'done_success' || current?.status === 'done_failed') return data
  if (current?.actorType === 'human' || currentStep?.type === 'human_review') return data

  const taskAgent = startableAgentName(task) || task.agent
  const actorAgent = agentNameFromActor(projectID, current?.actorId || currentStep?.actorRole)
  if (!allowActorMismatch && actorAgent && taskAgent && actorAgent !== taskAgent) return data

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

function workflowActorAgentForStep(bindings?: Record<string, { type?: string; id?: string }>, step?: { id?: string; actorRole?: string }): string | null {
  if (!bindings || !step) return null
  for (const key of [step.id, step.actorRole]) {
    const binding = key ? bindings[key] : undefined
    if (binding?.type === 'agent' && binding.id?.trim()) return binding.id.trim()
  }
  return null
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
