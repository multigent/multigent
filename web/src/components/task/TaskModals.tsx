import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { ClipboardCopy, FileText, MessageSquare, Pencil, Send, Trash2, X } from 'lucide-react'
import { cn } from '../../lib/cn'
import { apiDelete, apiPost, apiPut } from '../../lib/api'
import { useFormatDateTime } from '../../lib/format-datetime'
import { useApiJson } from '../../lib/use-api'
import { useAuth } from '../../lib/auth'
import { formatGoDuration, taskElapsedLabel } from '../../lib/task-duration'
import { WorkflowBoard, type WorkflowDefinition, type WorkflowField, type WorkflowRun, type WorkflowStep, type WorkflowStepInstance } from '../workflow/WorkflowBoard'
import { TechnicalLog } from '../ui/ConversationLog'

export type TaskRow = {
  id: string
  project: string
  agent: string
  title: string
  type?: string
  priority: number
  status: string
  statusGroup: string
  archived: boolean
  assignee?: string
  assigneeLabel?: string
  description?: string
  prompt?: string
  summary?: string
  labels: string[]
  parentId?: string
  position: number
  createdBy?: string
  createdByLabel?: string
  createdAt: string
  updatedAt: string
  startedAt?: string
  finishedAt?: string
  dueDate?: string
  estimateDuration?: string
}

export type TaskOption = { id: string; title: string; project?: string }

type RunRow = {
  project: string; agent: string; kind: string; status: string
  startedAt: string; finishedAt: string; model?: string
  taskId?: string; taskTitle?: string; logPath?: string
  inputTokens?: number; outputTokens?: number; cacheReadTokens?: number
  errorMsg?: string; command?: string
  sessionId?: string
}

type LogData = { content: string; truncated: boolean }
type TaskWorkflowData = { definition: WorkflowDefinition; run: WorkflowRun; steps: WorkflowStepInstance[] }
type SafeUser = { username: string; displayName?: string; email?: string }

// Restore real line breaks for descriptions whose upstream author stored literal
// "\n" / "\r\n" / "\t" sequences (typically agent-generated text that survived a
// JSON round-trip). Without this, ReactMarkdown renders the backslash sequences
// verbatim instead of as paragraph / line breaks.
function unescapeBreaks(s: string): string {
  return s.replace(/\\r\\n/g, '\n').replace(/\\n/g, '\n').replace(/\\t/g, '\t')
}

export const STATUS_KEYS = ['pending', 'in_progress', 'awaiting_confirmation', 'blocked', 'done_success', 'done_failed', 'cancelled'] as const

export const statusColor: Record<string, string> = {
  pending: 'bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300',
  in_progress: 'bg-sky-100 text-sky-800 dark:bg-sky-900/40 dark:text-sky-300',
  awaiting_confirmation: 'bg-violet-100 text-violet-800 dark:bg-violet-900/40 dark:text-violet-300',
  blocked: 'bg-orange-100 text-orange-800 dark:bg-orange-900/40 dark:text-orange-300',
  done_success: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-300',
  done_failed: 'bg-red-100 text-red-800 dark:bg-red-900/40 dark:text-red-300',
  cancelled: 'bg-neutral-100 text-neutral-600 dark:bg-zinc-800 dark:text-zinc-500',
}

export const priorityLabel: Record<number, { text: string; cls: string }> = {
  0: { text: 'P0', cls: 'text-red-600 dark:text-red-400' },
  1: { text: 'P1', cls: 'text-amber-600 dark:text-amber-400' },
  2: { text: 'P2', cls: 'text-sky-600 dark:text-sky-400' },
  3: { text: 'P3', cls: 'text-neutral-400 dark:text-zinc-500' },
}

export function isTerminal(s: string) {
  return s === 'done_success' || s === 'done_failed' || s === 'cancelled'
}

export function taskIdentityLabel(value?: string, label?: string) {
  return label || value || '—'
}

const fieldCls =
  'w-full rounded-lg border border-neutral-300 bg-white px-3 py-1.5 text-sm text-neutral-900 outline-none transition-colors focus:border-sky-400 dark:border-zinc-600 dark:bg-zinc-800 dark:text-zinc-100'

/* ── Edit modal ─── */

export function EditTaskModal({ task, taskOptions = [], onClose, onSaved }: { task: TaskRow; taskOptions?: TaskOption[]; onClose: () => void; onSaved: () => void }) {
  const { t } = useTranslation()
  const [title, setTitle] = useState(task.title)
  const [description, setDescription] = useState(task.description ?? '')
  const [status, setStatus] = useState(task.status)
  const [priority, setPriority] = useState(task.priority)
  const [taskType, setTaskType] = useState(task.type ?? '')
  const [summary, setSummary] = useState(task.summary ?? '')
  const [labelsStr, setLabelsStr] = useState((task.labels ?? []).join(', '))
  const [dueDate, setDueDate] = useState(task.dueDate ?? '')
  const [parentId, setParentId] = useState(task.parentId ?? '')
  const [estimateDuration, setEstimateDuration] = useState(task.estimateDuration ?? '')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  const parentChoices = taskOptions.filter((o) => o.id !== task.id)

  const showSummary = isTerminal(status)
  const changed = title !== task.title || description !== (task.description ?? '') ||
    status !== task.status || priority !== task.priority || taskType !== (task.type ?? '') ||
    summary !== (task.summary ?? '') || labelsStr !== (task.labels ?? []).join(', ') ||
    dueDate !== (task.dueDate ?? '') || parentId !== (task.parentId ?? '') ||
    estimateDuration !== (task.estimateDuration ?? '')

  async function onSave() {
    setErr(null)
    setBusy(true)
    try {
      const body: Record<string, unknown> = { project: task.project, agent: task.agent, id: task.id }
      if (title !== task.title) body.title = title
      if (description !== (task.description ?? '')) body.description = description
      if (status !== task.status) body.status = status
      if (priority !== task.priority) body.priority = priority
      if (taskType !== (task.type ?? '')) body.type = taskType
      if (summary !== (task.summary ?? '')) body.summary = summary
      if (labelsStr !== (task.labels ?? []).join(', ')) {
        body.labels = labelsStr.split(',').map(l => l.trim()).filter(Boolean)
      }
      if (dueDate !== (task.dueDate ?? '')) body.dueDate = dueDate || ''
      if (parentId !== (task.parentId ?? '')) body.parentId = parentId || ''
      if (estimateDuration !== (task.estimateDuration ?? '')) body.estimateDuration = estimateDuration || ''
      await apiPut('/api/v1/tasks/update', body)
      onSaved()
      onClose()
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/45 p-4" onClick={() => !busy && onClose()}>
      <div className="max-h-[85vh] w-full max-w-lg overflow-y-auto rounded-xl border border-neutral-200 bg-white shadow-lg dark:border-zinc-700 dark:bg-zinc-900 animate-scale-in" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between border-b border-neutral-200 px-5 py-3 dark:border-zinc-700">
          <h2 className="text-base font-semibold text-neutral-900 dark:text-zinc-100">{t('tasks.edit')}</h2>
          <button type="button" onClick={onClose} className="rounded-md p-1 text-neutral-400 hover:bg-neutral-100 dark:text-zinc-500 dark:hover:bg-zinc-800"><X className="size-4" /></button>
        </div>
        <div className="space-y-3 px-5 py-4">
          <div className="font-mono text-xs text-neutral-400 dark:text-zinc-500">{task.id}</div>
          <label className="block text-sm">
            <span className="text-neutral-600 dark:text-zinc-400">{t('forms.title')}</span>
            <input value={title} onChange={(e) => setTitle(e.target.value)} className={cn(fieldCls, 'mt-1')} />
          </label>
          <label className="block text-sm">
            <span className="text-neutral-600 dark:text-zinc-400">{t('tasks.description')}</span>
            <textarea value={description} onChange={(e) => setDescription(e.target.value)} rows={3} className={cn(fieldCls, 'mt-1 resize-y')} />
          </label>
          <div className="grid grid-cols-2 gap-3">
            <label className="block text-sm">
              <span className="text-neutral-600 dark:text-zinc-400">{t('tasks.filterStatus')}</span>
              <select value={status} onChange={(e) => setStatus(e.target.value)} className={cn(fieldCls, 'mt-1')}>
                {STATUS_KEYS.map((s) => <option key={s} value={s}>{t(`tasks.status.${s}`)}</option>)}
              </select>
            </label>
            <label className="block text-sm">
              <span className="text-neutral-600 dark:text-zinc-400">{t('forms.priority')}</span>
              <select value={priority} onChange={(e) => setPriority(Number(e.target.value))} className={cn(fieldCls, 'mt-1')}>
                {[0, 1, 2, 3].map((p) => <option key={p} value={p}>P{p} — {t(`forms.priorityLabel.${p}`)}</option>)}
              </select>
            </label>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <label className="block text-sm">
              <span className="text-neutral-600 dark:text-zinc-400">{t('forms.type')}</span>
              <select value={taskType} onChange={(e) => setTaskType(e.target.value)} className={cn(fieldCls, 'mt-1')}>
                {['chore', 'feature', 'bug', 'review', 'triage', 'test', 'research'].map((ty) => <option key={ty} value={ty}>{t(`forms.taskType.${ty}`, { defaultValue: ty })}</option>)}
              </select>
            </label>
            <label className="block text-sm">
              <span className="text-neutral-600 dark:text-zinc-400">{t('tasks.dueDate')}</span>
              <input type="date" value={dueDate} onChange={(e) => setDueDate(e.target.value)} className={cn(fieldCls, 'mt-1')} />
            </label>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <label className="block text-sm">
              <span className="text-neutral-600 dark:text-zinc-400">{t('tasks.estimateDuration')}</span>
              <input value={estimateDuration} onChange={(e) => setEstimateDuration(e.target.value)} placeholder="30m" className={cn(fieldCls, 'mt-1')} />
              <p className="mt-0.5 text-xs text-neutral-400 dark:text-zinc-500">{t('tasks.estimateDurationHint')}</p>
            </label>
            <label className="block text-sm">
              <span className="text-neutral-600 dark:text-zinc-400">{t('tasks.parentTask')}</span>
              {parentChoices.length > 0 ? (
                <select value={parentId} onChange={(e) => setParentId(e.target.value)} className={cn(fieldCls, 'mt-1')}>
                  <option value="">{t('tasks.parentTaskNone')}</option>
                  {parentChoices.map((o) => (
                    <option key={o.id} value={o.id}>{o.title} ({o.id})</option>
                  ))}
                </select>
              ) : (
                <input value={parentId} onChange={(e) => setParentId(e.target.value)} placeholder="t-..." className={cn(fieldCls, 'mt-1 font-mono text-xs')} />
              )}
            </label>
          </div>
          <label className="block text-sm">
            <span className="text-neutral-600 dark:text-zinc-400">{t('tasks.labels')}</span>
            <input value={labelsStr} onChange={(e) => setLabelsStr(e.target.value)} placeholder={t('tasks.labelsHint')} className={cn(fieldCls, 'mt-1')} />
          </label>
          {showSummary && (
            <label className="block text-sm">
              <span className="text-neutral-600 dark:text-zinc-400">{t('tasks.summary')}</span>
              <textarea value={summary} onChange={(e) => setSummary(e.target.value)} rows={4} placeholder={t('tasks.summaryPlaceholder')} className={cn(fieldCls, 'mt-1')} />
              {task.createdBy && (
                <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">
                  {t('tasks.willNotifyCreator', { creator: taskIdentityLabel(task.createdBy, task.createdByLabel) })}
                </p>
              )}
            </label>
          )}
          {err && <p className="text-sm text-red-600 dark:text-red-400">{err}</p>}
          <div className="flex justify-end gap-2 pt-1">
            <button type="button" onClick={onClose} disabled={busy} className="rounded-lg border border-neutral-300 px-3 py-1.5 text-sm dark:border-zinc-600">{t('forms.cancel')}</button>
            <button type="button" onClick={() => void onSave()} disabled={busy || !changed} className="rounded-lg bg-sky-600 px-3 py-1.5 text-sm font-medium text-white disabled:opacity-50">{busy ? t('forms.saving') : t('forms.save')}</button>
          </div>
        </div>
      </div>
    </div>
  )
}

/* ── Detail modal ─── */

export function TaskDetailModal({ task, onClose, onEdit, canEdit = true }: { task: TaskRow; onClose: () => void; onEdit: (r: TaskRow) => void; canEdit?: boolean }) {
  const { t } = useTranslation()
  const fmt = useFormatDateTime()

  const prio = priorityLabel[task.priority] ?? priorityLabel[2]
  const sCls = statusColor[task.status] ?? statusColor.pending
  const [workflowVersion, setWorkflowVersion] = useState(0)
  const [reviewComments, setReviewComments] = useState('')
  const [reviewOutputs, setReviewOutputs] = useState<Record<string, string>>({})
  const [reviewBusy, setReviewBusy] = useState<'approved' | 'needs_changes' | null>(null)
  const [reviewErr, setReviewErr] = useState<string | null>(null)

  const runsQuery = `/api/v1/telemetry/runs?allTime=1&project=${encodeURIComponent(task.project)}`
  const runsState = useApiJson<{ runs: RunRow[] }>(runsQuery, 0)
  const matchingRun = useMemo(() => {
    if (runsState.status !== 'ok' || !runsState.data?.runs) return null
    return runsState.data.runs.find((r) => r.taskId === task.id) ?? null
  }, [runsState, task.id])

  const hasLog = Boolean(matchingRun?.logPath)
  const logQuery = hasLog ? `/api/v1/telemetry/log?path=${encodeURIComponent(matchingRun!.logPath!)}` : null
  const logState = useApiJson<LogData>(logQuery, 0)
  const workflowState = useApiJson<TaskWorkflowData>(`/api/v1/projects/${encodeURIComponent(task.project)}/tasks/${encodeURIComponent(task.id)}/workflow`, workflowVersion)
  const usersState = useApiJson<SafeUser[]>('/api/v1/users', 0)
  const actorLabels = useMemo(() => {
    const labels = new Map<string, string>()
    if (usersState.status === 'ok') {
      for (const user of usersState.data ?? []) {
        labels.set(user.username, user.displayName || user.email || user.username)
      }
    }
    if (task.assignee && task.assigneeLabel) labels.set(task.assignee, task.assigneeLabel)
    if (task.createdBy && task.createdByLabel) labels.set(task.createdBy, task.createdByLabel)
    return labels
  }, [task.assignee, task.assigneeLabel, task.createdBy, task.createdByLabel, usersState])
  const activeWorkflowStep = workflowState.status === 'ok'
    ? workflowState.data.definition.steps.find((step) => step.id === workflowState.data.run.activeStepId)
    : undefined
  const activeWorkflowInst = workflowState.status === 'ok'
    ? workflowState.data.steps.find((step) => step.stepId === workflowState.data.run.activeStepId)
    : undefined
  const previousWorkflowOutputs = workflowState.status === 'ok'
    ? workflowState.data.steps.filter((step) => step.stepId !== workflowState.data.run.activeStepId && (step.outputArtifact || step.summary || Object.keys(step.outputValues ?? {}).length > 0))
    : []
  const canReviewWorkflow = activeWorkflowStep?.type === 'human_review'

  useEffect(() => {
    setReviewComments('')
    setReviewOutputs({})
    setReviewErr(null)
  }, [activeWorkflowStep?.id])

  async function submitWorkflowReview(decision: 'approved' | 'needs_changes') {
    setReviewErr(null)
    setReviewBusy(decision)
    const normalizedDecision = decision === 'approved' ? 'approve' : 'request_changes'
    const outputs: Record<string, string> = { ...reviewOutputs, decision: normalizedDecision }
    const outputFieldNames = (activeWorkflowStep?.outputFields ?? []).map((field) => field.name).filter(Boolean)
    const comments = (outputs.comments ?? reviewComments).trim()
    if (outputFieldNames.includes('comments')) outputs.comments = comments
    const missingField = outputFieldNames
      .filter((name) => name !== 'decision')
      .find((name) => !String(outputs[name] ?? '').trim())
    if (missingField) {
      setReviewErr(`${t('forms.fillRequired')} ${missingField}`)
      setReviewBusy(null)
      return
    }
    try {
      await apiPost(`/api/v1/projects/${encodeURIComponent(task.project)}/tasks/${encodeURIComponent(task.id)}/workflow/review`, {
        decision,
        comments,
        outputs,
      })
      setReviewComments('')
      setReviewOutputs({})
      setWorkflowVersion((v) => v + 1)
    } catch (e) {
      setReviewErr(e instanceof Error ? e.message : String(e))
    } finally {
      setReviewBusy(null)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center pt-[3vh]">
      <div className="absolute inset-0 bg-black/30 backdrop-blur-[2px] animate-fade-in dark:bg-black/50" onClick={onClose} />
      <div className="relative w-full max-w-6xl max-h-[94vh] flex flex-col overflow-hidden rounded-xl border border-neutral-200/80 bg-white shadow-2xl animate-scale-in dark:border-zinc-700/80 dark:bg-zinc-900">
        <div className="flex items-center justify-between border-b border-neutral-200/80 px-5 py-3 dark:border-zinc-700/60">
          <div className="flex items-center gap-3 min-w-0">
            <span className={cn('shrink-0 rounded-full px-2.5 py-0.5 text-[11px] font-semibold', sCls)}>{t(`tasks.status.${task.status}`, { defaultValue: task.status })}</span>
            <span className={cn('shrink-0 text-[11px] font-bold', prio.cls)}>{prio.text}</span>
            <span className="truncate text-sm font-medium text-neutral-900 dark:text-zinc-100">{task.title}</span>
          </div>
          <div className="flex items-center gap-1 shrink-0">
            {canEdit && (
              <button type="button" onClick={() => onEdit(task)} className="rounded-md p-1 text-neutral-400 transition-colors hover:bg-neutral-100 hover:text-neutral-700 dark:text-zinc-500 dark:hover:bg-zinc-800" title={t('tasks.edit')}>
                <Pencil className="size-4" strokeWidth={1.8} />
              </button>
            )}
            <button type="button" onClick={onClose} className="rounded-md p-1 text-neutral-400 transition-colors hover:bg-neutral-100 hover:text-neutral-700 dark:text-zinc-500 dark:hover:bg-zinc-800">
              <X className="size-4" strokeWidth={2} />
            </button>
          </div>
        </div>

        <div className="shrink-0 grid grid-cols-2 gap-x-6 gap-y-2.5 border-b border-neutral-100 px-5 py-3 text-sm dark:border-zinc-700/40 sm:grid-cols-3">
          <InfoCell label="ID"><span className="font-mono text-xs">{task.id}</span></InfoCell>
          <InfoCell label={t('tasks.colProject')}><span className="font-mono">{task.project}</span></InfoCell>
          <InfoCell label={t('tasks.colAssignee')}>
            <span title={task.assignee || `${task.project}/${task.agent}`}>{taskIdentityLabel(task.assignee || `${task.project}/${task.agent}`, task.assigneeLabel)}</span>
          </InfoCell>
          <InfoCell label={t('forms.type')}>{task.type ? t(`forms.taskType.${task.type}`, { defaultValue: task.type }) : '—'}</InfoCell>
          <InfoCell label={t('api.taskColUpdated')}>{fmt(task.updatedAt)}</InfoCell>
          {task.estimateDuration && (
            <InfoCell label={t('tasks.estimateDuration')}>
              <span className="tabular-nums">{formatGoDuration(task.estimateDuration)}</span>
            </InfoCell>
          )}
          {task.startedAt && <InfoCell label={t('tasks.startedAt')}>{fmt(task.startedAt)}</InfoCell>}
          {task.finishedAt && <InfoCell label={t('tasks.finishedAt')}>{fmt(task.finishedAt)}</InfoCell>}
          {taskElapsedLabel(task) && (
            <InfoCell label={t('tasks.elapsed')}>
              <span className="tabular-nums">{taskElapsedLabel(task)}</span>
            </InfoCell>
          )}
          {task.dueDate && <InfoCell label={t('tasks.dueDate')}><span className="tabular-nums">{task.dueDate}</span></InfoCell>}
          {task.createdBy && <InfoCell label={t('tasks.createdBy')}><span title={task.createdBy}>{taskIdentityLabel(task.createdBy, task.createdByLabel)}</span></InfoCell>}
          {task.parentId && <InfoCell label={t('tasks.parentTask')}><span className="font-mono text-xs">{task.parentId}</span></InfoCell>}
          {task.labels && task.labels.length > 0 && (
            <InfoCell label={t('tasks.labels')}>
              <div className="flex flex-wrap gap-1">
                {task.labels.map(l => (
                  <span key={l} className="rounded-full bg-indigo-100 px-2 py-0.5 text-[11px] font-medium text-indigo-700 dark:bg-indigo-900/30 dark:text-indigo-400">{l}</span>
                ))}
              </div>
            </InfoCell>
          )}
          {matchingRun && (
            <>
              <InfoCell label={t('runs.model')}><span className="font-mono">{matchingRun.model ?? '—'}</span></InfoCell>
              <InfoCell label={t('runs.colTok')}>
                <span className="tabular-nums">{fmtNum((matchingRun.inputTokens ?? 0) + (matchingRun.outputTokens ?? 0) + (matchingRun.cacheReadTokens ?? 0))} tok</span>
              </InfoCell>
              {matchingRun.sessionId && (
                <InfoCell label={t('runs.sessionLabel')}>
                  <div className="flex items-center gap-1">
                    <span className="font-mono text-xs text-emerald-700 dark:text-emerald-400" title={matchingRun.sessionId}>{matchingRun.sessionId.slice(0, 8)}…</span>
                    <CopyRunResumeCmd model={matchingRun.model} sessionId={matchingRun.sessionId} agent={matchingRun.agent} project={matchingRun.project} />
                  </div>
                </InfoCell>
              )}
            </>
          )}
        </div>

        <div className="flex-1 min-h-0 overflow-y-auto">
        {workflowState.status === 'ok' && (
          <div className="border-b border-neutral-100 px-5 py-4 dark:border-zinc-700/40">
            <div className="mb-3 flex items-center justify-between gap-3">
              <div>
                <span className="text-xs font-semibold uppercase tracking-wider text-sky-500 dark:text-sky-400">{t('workflows.taskWorkflow')}</span>
                <h3 className="mt-1 text-base font-semibold text-neutral-900 dark:text-zinc-100">{workflowState.data.definition.name}</h3>
              </div>
              <span className="rounded-full bg-sky-100 px-2.5 py-1 text-xs font-medium text-sky-700 dark:bg-sky-900/50 dark:text-sky-300">
                {workflowState.data.run.status}
              </span>
            </div>
            <div className="grid min-h-[520px] gap-4 lg:grid-cols-[minmax(0,1.45fr)_minmax(320px,0.85fr)]">
              <WorkflowBoard definition={workflowState.data.definition} run={workflowState.data.run} instances={workflowState.data.steps} focusActive />
              <div className="flex min-h-0 flex-col rounded-xl border border-neutral-200 bg-white dark:border-zinc-700 dark:bg-zinc-950">
                <WorkflowRuntimePanel
                  step={activeWorkflowStep}
                  instance={activeWorkflowInst}
                  steps={workflowState.data.definition.steps}
                  previousOutputs={previousWorkflowOutputs}
                  actorLabels={actorLabels}
                  canReview={canReviewWorkflow}
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
              </div>
            </div>
          </div>
        )}
        {workflowState.status === 'error' && (workflowState.error as { status?: number }).status !== 404 && (
          <div className="border-b border-neutral-100 px-5 py-3 dark:border-zinc-700/40">
            <p className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300">
              {workflowState.error.message}
            </p>
          </div>
        )}

        {task.description && (
          <div className="border-b border-neutral-100 px-5 py-3 dark:border-zinc-700/40">
            <span className="text-xs font-semibold uppercase tracking-wider text-neutral-400 dark:text-zinc-500">{t('tasks.description')}</span>
            <div className="mt-1.5 text-sm text-neutral-700 dark:text-zinc-300">
              <div className="prose prose-sm max-w-none dark:prose-invert"><ReactMarkdown remarkPlugins={[remarkGfm]}>{unescapeBreaks(task.description)}</ReactMarkdown></div>
            </div>
          </div>
        )}

        {task.prompt && (
          <div className="border-b border-neutral-100 px-5 py-3 dark:border-zinc-700/40">
            <span className="text-xs font-semibold uppercase tracking-wider text-neutral-400 dark:text-zinc-500">{t('forms.prompt')}</span>
            <div className="mt-1.5 rounded-lg bg-neutral-50 p-3 text-sm text-neutral-700 dark:bg-zinc-800/50 dark:text-zinc-300">
              <div className="prose prose-sm max-w-none dark:prose-invert"><ReactMarkdown remarkPlugins={[remarkGfm]}>{unescapeBreaks(task.prompt)}</ReactMarkdown></div>
            </div>
          </div>
        )}

        {task.summary && (
          <div className="border-b border-neutral-100 px-5 py-3 dark:border-zinc-700/40">
            <span className="text-xs font-semibold uppercase tracking-wider text-emerald-500 dark:text-emerald-400">{t('tasks.summary')}</span>
            <div className="mt-1.5 rounded-lg bg-emerald-50 p-3 text-sm text-neutral-700 dark:bg-emerald-900/20 dark:text-zinc-300">
              <div className="prose prose-sm max-w-none dark:prose-invert"><ReactMarkdown remarkPlugins={[remarkGfm]}>{unescapeBreaks(task.summary)}</ReactMarkdown></div>
            </div>
          </div>
        )}

        <TaskCommentsSection project={task.project} agent={task.agent} taskId={task.id} />

        <div>
          {matchingRun ? (
            <>
              <div className="flex items-center gap-1.5 px-5 pt-3 pb-2">
                <FileText className="size-3.5 text-neutral-400 dark:text-zinc-500" strokeWidth={1.8} />
                <span className="text-xs font-semibold uppercase tracking-wider text-neutral-400 dark:text-zinc-500">{t('runs.logTitle')}</span>
              </div>
              <div className="px-5 pb-4">
                {hasLog && logState.status === 'loading' && (
                  <div className="flex items-center gap-2 py-6 justify-center">
                    <div className="size-4 animate-spin rounded-full border-2 border-neutral-300 border-t-sky-600 dark:border-zinc-600 dark:border-t-sky-400" />
                    <span className="text-sm text-neutral-500">{t('api.loading')}</span>
                  </div>
                )}
                {hasLog && logState.status === 'error' && <p className="py-4 text-center text-sm text-neutral-400">{t('runs.logNotFound')}</p>}
                {hasLog && logState.status === 'ok' && <TechnicalLog content={logState.data.content} />}
                {!hasLog && <p className="py-4 text-center text-sm text-neutral-400 dark:text-zinc-500">{t('runs.noLog')}</p>}
              </div>
            </>
          ) : runsState.status === 'loading' ? (
            <div className="flex items-center gap-2 py-8 justify-center">
              <div className="size-4 animate-spin rounded-full border-2 border-neutral-300 border-t-sky-600 dark:border-zinc-600 dark:border-t-sky-400" />
              <span className="text-sm text-neutral-500">{t('tasks.loadingRuns')}</span>
            </div>
          ) : (
            <p className="py-8 text-center text-sm text-neutral-400 dark:text-zinc-500">{t('tasks.noRunRecord')}</p>
          )}
        </div>
        </div>
      </div>
    </div>
  )
}

/* ── Helpers ── */

function InfoCell({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <span className="text-xs font-medium text-neutral-400 dark:text-zinc-500">{label}</span>
      <p className="text-neutral-800 dark:text-zinc-200">{children}</p>
    </div>
  )
}

/* ── Task Comments Section ── */

type TaskCommentRow = {
  id: string
  taskId: string
  author: string
  body: string
  createdAt: string
}

function TaskCommentsSection({ project, agent, taskId }: { project: string; agent: string; taskId: string }) {
  const { t } = useTranslation()
  const fmt = useFormatDateTime()
  const { user } = useAuth()
  const [body, setBody] = useState('')
  const [busy, setBusy] = useState(false)
  const [ver, setVer] = useState(0)

  const url = `/api/v1/tasks/${encodeURIComponent(project)}/${encodeURIComponent(agent)}/${encodeURIComponent(taskId)}/comments`
  const state = useApiJson<TaskCommentRow[]>(url, ver)
  const comments = state.status === 'ok' ? state.data ?? [] : []

  const reload = useCallback(() => setVer((v) => v + 1), [])

  async function handleAdd() {
    if (!body.trim()) return
    setBusy(true)
    try {
      await apiPost(url, { author: user?.username ?? 'human', body: body.trim() })
      setBody('')
      reload()
    } finally {
      setBusy(false)
    }
  }

  async function handleDelete(commentId: string) {
    await apiDelete(`${url}/${encodeURIComponent(commentId)}`)
    reload()
  }

  return (
    <div className="shrink-0 border-b border-neutral-100 px-5 py-3 dark:border-zinc-700/40">
      <div className="flex items-center gap-1.5 mb-2">
        <MessageSquare className="size-3.5 text-neutral-400 dark:text-zinc-500" strokeWidth={1.8} />
        <span className="text-xs font-semibold uppercase tracking-wider text-neutral-400 dark:text-zinc-500">{t('tasks.comments')} ({comments.length})</span>
      </div>

      {comments.length > 0 && (
        <div className="space-y-2 mb-3 max-h-56 overflow-y-auto">
          {comments.map((c) => (
            <div key={c.id} className="group rounded-lg bg-neutral-50 px-3 py-2 dark:bg-zinc-800/50">
              <div className="flex items-center justify-between gap-2">
                <div className="flex items-center gap-2 text-xs text-neutral-500 dark:text-zinc-500">
                  <span className="font-semibold text-neutral-700 dark:text-zinc-300">@{c.author}</span>
                  <span>{fmt(c.createdAt)}</span>
                </div>
                {(user?.role === 'admin' || user?.username === c.author) && (
                  <button type="button" onClick={() => void handleDelete(c.id)} className="opacity-0 group-hover:opacity-100 rounded p-0.5 text-neutral-400 hover:text-red-500 transition dark:text-zinc-600 dark:hover:text-red-400" title={t('forms.delete')}>
                    <Trash2 className="size-3" />
                  </button>
                )}
              </div>
              <div className="mt-1 text-sm text-neutral-700 dark:text-zinc-300 prose prose-sm max-w-none dark:prose-invert">
                <ReactMarkdown remarkPlugins={[remarkGfm]}>{c.body}</ReactMarkdown>
              </div>
            </div>
          ))}
        </div>
      )}

      <div className="flex gap-2">
        <input
          value={body}
          onChange={(e) => setBody(e.target.value)}
          onKeyDown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); void handleAdd() } }}
          placeholder={t('tasks.commentPlaceholder')}
          className="flex-1 rounded-lg border border-neutral-300 bg-white px-3 py-1.5 text-sm outline-none transition-colors focus:border-sky-400 dark:border-zinc-600 dark:bg-zinc-800 dark:text-zinc-100"
          disabled={busy}
        />
        <button type="button" onClick={() => void handleAdd()} disabled={busy || !body.trim()} className="rounded-lg bg-sky-600 px-3 py-1.5 text-sm font-medium text-white disabled:opacity-50">
          <Send className="size-3.5" />
        </button>
      </div>
    </div>
  )
}

function WorkflowRuntimePanel({
  step,
  instance,
  steps,
  previousOutputs,
  actorLabels,
  canReview,
  reviewOutputs,
  reviewComments,
  reviewBusy,
  reviewErr,
  onChangeOutput,
  onChangeComments,
  onSubmitReview,
}: {
  step?: WorkflowStep
  instance?: WorkflowStepInstance
  steps: WorkflowStep[]
  previousOutputs: WorkflowStepInstance[]
  actorLabels: Map<string, string>
  canReview: boolean
  reviewOutputs: Record<string, string>
  reviewComments: string
  reviewBusy: 'approved' | 'needs_changes' | null
  reviewErr: string | null
  onChangeOutput: (name: string, value: string) => void
  onChangeComments: (value: string) => void
  onSubmitReview: (decision: 'approved' | 'needs_changes') => void
}) {
  const { t } = useTranslation()
  const outputValues = instance?.outputValues ?? parseWorkflowArtifact(instance?.outputArtifact || instance?.summary || '')
  const inputValues = instance?.inputValues ?? parseWorkflowArtifact(instance?.inputArtifact || '')
  const hasStructuredInput = Object.keys(inputValues).length > 0
  const stepTitleByID = useMemo(() => new Map(steps.map((item) => [item.id, item.title])), [steps])
  const actorLabel = workflowActorLabel(instance?.actorType, instance?.actorId, actorLabels)

  if (!step) {
    return (
      <div className="p-4 text-sm text-neutral-500 dark:text-zinc-500">{t('workflows.detail.notSpecified')}</div>
    )
  }

  const editableOutputFields = (step.outputFields ?? []).filter((field) => field.name !== 'decision')
  const hasInput = Boolean(instance?.inputArtifact?.trim()) || (step.inputFields ?? []).length > 0

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="shrink-0 border-b border-neutral-100 px-4 py-3 dark:border-zinc-800">
        <p className="text-xs font-semibold uppercase tracking-wider text-neutral-400 dark:text-zinc-500">{t('workflows.detail.currentStep')}</p>
        <h4 className="mt-1 text-sm font-semibold text-neutral-900 dark:text-zinc-100">{step.title}</h4>
        <p className="mt-1 text-xs text-neutral-500 dark:text-zinc-500">
          {t(`workflows.stepTypes.${step.type}`, { defaultValue: step.type })} · {actorLabel}
        </p>
      </div>
      <div className="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
        {step.description && (
          <WorkflowPanelBlock title={t('workflows.detail.goal')}>
            <p className="text-sm text-neutral-700 dark:text-zinc-300">{step.description}</p>
          </WorkflowPanelBlock>
        )}

        {hasInput && (
          <WorkflowPanelBlock title={t('workflows.detail.input')}>
            <WorkflowFieldList fields={step.inputFields ?? []} values={inputValues} />
            {instance?.inputArtifact && !hasStructuredInput && <WorkflowArtifact value={instance.inputArtifact} />}
          </WorkflowPanelBlock>
        )}

        {previousOutputs.length > 0 && (
          <WorkflowPanelBlock title={t('workflows.detail.previousOutputs')}>
            <div className="space-y-3">
              {previousOutputs.map((item) => (
                <div key={item.id} className="rounded-lg bg-neutral-50 p-3 dark:bg-zinc-900">
                  <p className="text-xs font-semibold text-neutral-600 dark:text-zinc-300">{stepTitleByID.get(item.stepId) || item.stepId}</p>
                  <WorkflowValuesOrArtifact values={item.outputValues} fields={steps.find((candidate) => candidate.id === item.stepId)?.outputFields} artifact={item.outputArtifact || item.summary} compact />
                </div>
              ))}
            </div>
          </WorkflowPanelBlock>
        )}

        <WorkflowPanelBlock title={t('workflows.detail.output')}>
          {canReview ? (
            <div className="space-y-3">
              {editableOutputFields.length === 0 && (
                <textarea
                  value={reviewComments}
                  onChange={(e) => onChangeComments(e.target.value)}
                  rows={4}
                  placeholder={t('workflows.review.commentsPlaceholder')}
                  className="w-full resize-y rounded-lg border border-neutral-300 bg-white px-3 py-2 text-sm text-neutral-900 outline-none focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100"
                />
              )}
              {editableOutputFields.map((field) => (
                <label key={field.name} className="block">
                  <span className="text-xs font-medium text-neutral-500 dark:text-zinc-500">
                    {field.name}
                    <span className="ml-1 text-red-500">*</span>
                  </span>
                  {field.description && <span className="mt-0.5 block text-xs text-neutral-400 dark:text-zinc-600">{field.description}</span>}
                  <textarea
                    value={field.name === 'comments' ? reviewComments : reviewOutputs[field.name] || ''}
                    onChange={(e) => onChangeOutput(field.name, e.target.value)}
                    rows={field.name === 'comments' ? 3 : 2}
                    placeholder={field.description || field.name}
                    className="mt-1 w-full resize-y rounded-lg border border-neutral-300 bg-white px-3 py-2 text-sm text-neutral-900 outline-none focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100"
                  />
                </label>
              ))}
              {reviewErr && <p className="text-sm text-red-600 dark:text-red-400">{reviewErr}</p>}
              <div className="flex justify-end gap-2">
                <button type="button" onClick={() => onSubmitReview('needs_changes')} disabled={Boolean(reviewBusy)} className="rounded-lg border border-neutral-300 bg-white px-3 py-2 text-sm font-medium text-neutral-600 hover:bg-neutral-50 disabled:opacity-50 dark:border-zinc-600 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:bg-zinc-800">
                  {reviewBusy === 'needs_changes' ? t('forms.working') : t('workflows.review.requestChanges')}
                </button>
                <button type="button" onClick={() => onSubmitReview('approved')} disabled={Boolean(reviewBusy)} className="rounded-lg border border-sky-600 bg-white px-3 py-2 text-sm font-medium text-sky-700 hover:bg-sky-50 disabled:opacity-50 dark:border-sky-500 dark:bg-zinc-900 dark:text-sky-400 dark:hover:bg-zinc-800">
                  {reviewBusy === 'approved' ? t('forms.working') : t('workflows.review.approve')}
                </button>
              </div>
            </div>
          ) : (
            <>
              <WorkflowFieldList fields={step.outputFields ?? []} values={outputValues} />
              {Object.keys(outputValues).length > 0
                ? null
                : <WorkflowValuesOrArtifact values={instance?.outputValues} fields={step.outputFields} artifact={instance?.outputArtifact || instance?.summary} />}
            </>
          )}
        </WorkflowPanelBlock>
      </div>
    </div>
  )
}

function workflowActorLabel(actorType?: string, actorId?: string, labels?: Map<string, string>) {
  const id = actorId?.trim()
  if (!id) return '-'
  const label = labels?.get(id) || id
  if (actorType === 'human') return label
  return label
}

function WorkflowValuesOrArtifact({ values, fields, artifact, compact = false }: { values?: Record<string, string>; fields?: WorkflowField[]; artifact?: string; compact?: boolean }) {
  const { t } = useTranslation()
  if (values && Object.keys(values).length > 0) {
    return <WorkflowValueMap values={values} fields={fields} compact={compact} />
  }
  if (artifact) {
    return <WorkflowArtifact value={artifact} compact={compact} />
  }
  return <p className="text-sm text-neutral-400 dark:text-zinc-600">{t('workflows.detail.notSpecified')}</p>
}

function WorkflowValueMap({ values, fields = [], compact = false }: { values: Record<string, string>; fields?: WorkflowField[]; compact?: boolean }) {
  const entries = Object.entries(values).filter(([, value]) => String(value ?? '').trim())
  const fieldByName = new Map(fields.map((field) => [field.name, field]))
  if (entries.length === 0) return null
  return (
    <div className={cn('space-y-2', compact ? 'max-h-40 overflow-y-auto' : '')}>
      {entries.map(([key, value]) => {
        const field = fieldByName.get(key)
        return (
          <div key={key} className="rounded-lg bg-neutral-50 p-2.5 dark:bg-zinc-900">
            <WorkflowFieldTitle fieldName={key} description={field?.description} />
            <div className="mt-1 whitespace-pre-wrap break-words text-sm text-neutral-700 dark:text-zinc-300">
              <WorkflowValueText value={String(value)} />
            </div>
          </div>
        )
      })}
    </div>
  )
}

function WorkflowPanelBlock({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section>
      <h5 className="mb-2 text-xs font-semibold uppercase tracking-wider text-neutral-400 dark:text-zinc-500">{title}</h5>
      {children}
    </section>
  )
}

function WorkflowFieldList({ fields, values }: { fields: WorkflowField[]; values: Record<string, string> }) {
  if (fields.length === 0) return null
  return (
    <div className="mb-2 space-y-2">
      {fields.map((field) => (
        <div key={field.name} className="rounded-lg border border-neutral-100 bg-white p-2.5 dark:border-zinc-800 dark:bg-zinc-950">
          <WorkflowFieldTitle fieldName={field.name} description={field.description} />
          {values[field.name] && (
            <div className="mt-1.5 whitespace-pre-wrap break-words text-sm text-neutral-800 dark:text-zinc-200">
              <WorkflowValueText value={values[field.name]} />
            </div>
          )}
        </div>
      ))}
    </div>
  )
}

function WorkflowFieldTitle({ fieldName, description }: { fieldName: string; description?: string }) {
  const title = description?.trim() || fieldName
  return (
    <div>
      <p className="text-xs font-semibold text-neutral-700 dark:text-zinc-300" title={fieldName}>{title}</p>
      {description?.trim() && (
        <p className="mt-0.5 font-mono text-[10px] text-neutral-400 dark:text-zinc-600">{fieldName}</p>
      )}
    </div>
  )
}

const docIDPattern = /\bdoc-\d{8}-[a-z0-9]+\b/gi

function WorkflowValueText({ value }: { value: string }) {
  const text = String(value ?? '')
  const parts: ReactNode[] = []
  let lastIndex = 0
  for (const match of text.matchAll(docIDPattern)) {
    const docID = match[0]
    const index = match.index ?? 0
    if (index > lastIndex) {
      parts.push(text.slice(lastIndex, index))
    }
    parts.push(<DocIDLink key={`${docID}-${index}`} docID={docID} />)
    lastIndex = index + docID.length
  }
  if (lastIndex < text.length) {
    parts.push(text.slice(lastIndex))
  }
  return <>{parts.length > 0 ? parts : text}</>
}

function DocIDLink({ docID }: { docID: string }) {
  return (
    <a
      href={`/docs/${encodeURIComponent(docID)}`}
      target="_blank"
      rel="noreferrer"
      className="font-medium text-sky-700 underline decoration-sky-300 underline-offset-2 hover:text-sky-800 dark:text-sky-400 dark:decoration-sky-700 dark:hover:text-sky-300"
      onClick={(e) => e.stopPropagation()}
    >
      {docID}
    </a>
  )
}

function WorkflowArtifact({ value, compact = false }: { value: string; compact?: boolean }) {
  return (
    <div className={cn('rounded-lg bg-neutral-50 p-3 text-sm text-neutral-700 dark:bg-zinc-900 dark:text-zinc-300', compact ? 'max-h-40 overflow-y-auto' : 'max-h-64 overflow-y-auto')}>
      <div className="prose prose-sm max-w-none dark:prose-invert">
        <ReactMarkdown remarkPlugins={[remarkGfm]}>{unescapeBreaks(value)}</ReactMarkdown>
      </div>
    </div>
  )
}

function parseWorkflowArtifact(value: string): Record<string, string> {
  const out: Record<string, string> = {}
  const unescaped = unescapeBreaks(value)
  try {
    const parsed = JSON.parse(unescaped)
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      const record = parsed as Record<string, unknown>
      const wrappedValues = record.inputs && typeof record.inputs === 'object' && !Array.isArray(record.inputs)
        ? record.inputs
        : record.outputs && typeof record.outputs === 'object' && !Array.isArray(record.outputs)
          ? record.outputs
          : record
      for (const [key, raw] of Object.entries(wrappedValues as Record<string, unknown>)) {
        if (typeof raw === 'string') out[key] = raw
        else out[key] = JSON.stringify(raw)
      }
      return out
    }
  } catch {
    // Fall back to old field: value snippets for incomplete historical runs.
  }
  const lines = unescaped.split('\n')
  for (const line of lines) {
    const match = line.match(/^\s*(?:[-*]\s*)?`?([A-Za-z0-9_.-]+)`?\s*[:：]\s*(.+?)\s*$/)
    if (!match) continue
    out[match[1]] = match[2]
  }
  return out
}

function fmtNum(n: number): string {
  return n.toLocaleString()
}

function buildRunResumeCmd(model: string | undefined, sessionId: string, agent: string, project: string): string {
  const m = (model ?? '').toLowerCase()
  if (m.includes('claude')) return `claude --resume ${sessionId}`
  if (m.includes('codex'))  return `codex exec resume ${sessionId}`
  if (m.includes('gemini')) return `gemini --resume ${sessionId}`
  if (m.includes('cursor')) return `agent --resume ${sessionId}`
  return `# session: ${sessionId}  (agent: ${agent}, project: ${project})`
}

function CopyRunResumeCmd({ model, sessionId, agent, project }: { model?: string; sessionId: string; agent: string; project: string }) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)
  function doCopy() {
    const cmd = buildRunResumeCmd(model, sessionId, agent, project)
    void navigator.clipboard.writeText(cmd).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    })
  }
  return (
    <button
      type="button"
      onClick={doCopy}
      title={t('schedule.copyResumeCmd')}
      className="shrink-0 rounded-md p-1 text-neutral-400 transition-colors hover:bg-neutral-100 hover:text-neutral-600 dark:text-zinc-500 dark:hover:bg-zinc-800 dark:hover:text-zinc-300"
    >
      {copied
        ? <span className="text-[10px] font-medium text-emerald-600 dark:text-emerald-400">✓</span>
        : <ClipboardCopy className="size-3.5" strokeWidth={2} />}
    </button>
  )
}
