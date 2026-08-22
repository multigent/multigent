import { useCallback, useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import Markdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import {
  ChevronRight, Bot, BookOpen, Check,
  Settings2, Users, UserCog, Activity, User, Mail, ListTodo, Reply, Send, Container,
  Cable, Server, Clock3, X, GitBranch, RefreshCw,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { cn } from '../../lib/cn'
import { useFormatDateTime } from '../../lib/format-datetime'
import { useApiJson } from '../../lib/use-api'
import { apiDelete, apiFetch, apiPost, apiPut, apiPatch, apiTeamPath } from '../../lib/api'
import { canConfigureAgent, canManageProject, canOperateAgent, isTrustedProxyMode, useAuth } from '../../lib/auth'
import { useWorkspaceAccess } from '../../lib/workspace-access'
import { Pagination } from '../../components/ui/Pagination'
import { AgentChannelPanel } from '../../components/project/AgentChannelPanel'
import { showToast } from '../../components/ui/Toast'

const AGENT_MODELS = [
  'claudecode', 'codex', 'cursor', 'gemini',
] as const

const MODEL_COLORS: Record<string, string> = {
  claudecode:    'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300',
  codex:         'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300',
  cursor:        'bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300',
  gemini:        'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300',
  qoder:         'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300',
  opencode:      'bg-teal-100 text-teal-700 dark:bg-teal-900/30 dark:text-teal-300',
  iflow:         'bg-pink-100 text-pink-700 dark:bg-pink-900/30 dark:text-pink-300',
  'generic-cli': 'bg-neutral-200 text-neutral-700 dark:bg-zinc-700 dark:text-zinc-300',
  'http-agent':  'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
  human:         'bg-indigo-100 text-indigo-700 dark:bg-indigo-900/30 dark:text-indigo-300',
}

const AVATAR_BACKGROUNDS = ['b6e3f4', 'c0aede', 'd1d4f9', 'ffd5dc', 'ffdfbf']

function avatarHash(s: string): number {
  let h = 2166136261
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i)
    h = Math.imul(h, 16777619)
  }
  return h >>> 0
}

function dicebearAvatar(seed: string): string {
  const bg = AVATAR_BACKGROUNDS[avatarHash(seed) % AVATAR_BACKGROUNDS.length]
  return `https://api.dicebear.com/9.x/notionists/svg?seed=${encodeURIComponent(seed)}&backgroundColor=${bg}`
}

function avatarChoices(project: string, agent: string, nonce: number): string[] {
  const base = `${project}-${agent || 'agent'}`
  return Array.from({ length: 8 }, (_, i) => dicebearAvatar(`${base}-${nonce}-${i + 1}`))
}

type SessionInfo = { sessionId?: string; sessionStartedAt?: string; sessionScope?: string }

type HTTPAgentConfig = {
  url?: string
  model?: string
  api_key?: string
  timeout?: string
  stream?: boolean
}

type SandboxConfig = {
  provider: string
  image?: string
  networkMode?: string
  agentCli?: AgentCLIConfig
  resources?: { memoryMb?: number; cpus?: number; timeoutSec?: number }
  docker?: {
    image?: string
    network_mode?: string
    memory_mb?: number
    cpus?: number
  }
  e2b?: {
    template?: string
    timeoutSec?: number
  }
}

type SandboxCapabilities = {
  docker?: { available: boolean; reason?: string }
  kvm?: { available: boolean; reason?: string }
  e2b?: { available: boolean; reason?: string }
  directHost?: { available: boolean; reason?: string }
}

type AgentCLIConfig = {
  vendor?: string
  version?: string
  channel?: string
  binary?: string
  packageManager?: string
  package?: string
}

type AgentContext = {
  contextFile: string
  context?: string
  wakeup: string
  model: string
  runtimeModel?: string
  team: string
  role: string
  avatar?: string
  syncedAt: string | null
  skills: string[]
  skillDetails?: Array<{ name: string; displayName?: string; description?: string }>
  httpAgent?: HTTPAgentConfig
  env?: Record<string, string>
  provider?: string
  runtimeNodeId?: string
  workDir?: string
  sandbox?: SandboxConfig
  addDirs?: string[]
}

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

type AgentWorkerDetail = {
  id: string
  name: string
  displayName?: string
  schedule?: WorkerSchedule
}

type KnowledgeDoc = {
  id: string
  title: string
  index?: string
  description?: string
}

type ContextBindingView = {
  binding: {
    id: string
    docId?: string
    scopeType: string
    scopeId?: string
    required?: boolean
  }
  artifact: {
    id?: string
    docId?: string
    title?: string
    summary?: string
    kind?: string
  }
  doc?: KnowledgeDoc
}

type ContextBindingsResponse = { bindings?: ContextBindingView[] }

type RuntimeNode = {
  id: string
  name: string
  status: string
  os?: string
  arch?: string
  hostname?: string
}

type RuntimeNodeListResp = { nodes: RuntimeNode[] }

function agentModelRequiresModelAccount(model?: string) {
  return model !== 'human' && model !== 'http-agent'
}

const RUNTIME_MODEL_PRESETS: Record<string, string[]> = {
  codex: ['gpt-5.6-sol', 'gpt-5.6-terra', 'gpt-5.6-luna', 'gpt-5.5', 'gpt-5.4', 'gpt-5.4-mini', 'gpt-5.3-codex-spark'],
  claudecode: ['claude-fable-5', 'claude-sonnet-5', 'claude-opus-4-8', 'claude-haiku-4-5', 'claude-haiku-4-5-20251001'],
  gemini: ['gemini-3.6-flash', 'gemini-3.5-flash', 'gemini-3.5-flash-lite', 'gemini-3.1-pro-preview', 'gemini-3.1-pro-preview-customtools', 'gemini-3.1-flash-lite', 'gemini-2.5-pro', 'gemini-2.5-flash', 'gemini-2.5-flash-lite'],
  cursor: ['gpt-5.6-sol', 'gpt-5.6-terra', 'gpt-5.6-luna', 'claude-fable-5', 'claude-sonnet-5', 'claude-opus-4-8', 'auto'],
  opencode: ['gpt-5.6-sol', 'gpt-5.6-terra', 'gpt-5.6-luna', 'gpt-5.5', 'claude-fable-5', 'claude-sonnet-5', 'claude-opus-4-8'],
}

const OFFICIAL_RUNTIME_MODEL_PRESETS: Record<string, string[]> = {
  codex: ['gpt-5.6-sol', 'gpt-5.6-terra', 'gpt-5.6-luna', 'gpt-5.5', 'gpt-5.4', 'gpt-5.4-mini', 'gpt-5.3-codex-spark'],
  openai: ['gpt-5.6-sol', 'gpt-5.6-terra', 'gpt-5.6-luna', 'gpt-5.5', 'gpt-5.4', 'gpt-5.4-mini', 'gpt-5.3-codex-spark'],
  cursor: ['gpt-5.6-sol', 'gpt-5.6-terra', 'gpt-5.6-luna', 'claude-fable-5', 'claude-sonnet-5', 'claude-opus-4-8', 'auto'],
  claudecode: ['claude-fable-5', 'claude-sonnet-5', 'claude-opus-4-8', 'claude-haiku-4-5', 'claude-haiku-4-5-20251001'],
  anthropic: ['claude-fable-5', 'claude-sonnet-5', 'claude-opus-4-8', 'claude-haiku-4-5', 'claude-haiku-4-5-20251001'],
  gemini: ['gemini-3.6-flash', 'gemini-3.5-flash', 'gemini-3.5-flash-lite', 'gemini-3.1-pro-preview', 'gemini-3.1-pro-preview-customtools', 'gemini-3.1-flash-lite', 'gemini-2.5-pro', 'gemini-2.5-flash', 'gemini-2.5-flash-lite'],
}

type ModelCatalog = {
  source?: string
  modelsByCLI?: Record<string, string[]>
  modelsByProviderType?: Record<string, string[]>
  modelsByProvider?: Record<string, string[]>
}

const buttonBaseCls = 'inline-flex h-8 items-center justify-center rounded-md px-3 text-xs font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50'
const primaryButtonCls = cn(buttonBaseCls, 'border border-sky-200 bg-sky-50 text-sky-700 hover:bg-sky-100 dark:border-sky-800 dark:bg-sky-900/20 dark:text-sky-300 dark:hover:bg-sky-900/30')
const secondaryButtonCls = cn(buttonBaseCls, 'border border-neutral-200 bg-white text-neutral-700 hover:border-neutral-300 hover:bg-neutral-50 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:border-zinc-600 dark:hover:bg-zinc-800')
const subtleButtonCls = cn(buttonBaseCls, 'text-neutral-500 hover:bg-neutral-100 hover:text-neutral-800 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-200')
const panelSelectCls = 'h-8 rounded-md border border-neutral-200 bg-white px-2.5 text-sm text-neutral-700 outline-none hover:border-neutral-300 focus:border-sky-400 disabled:opacity-50 dark:border-zinc-700 dark:bg-zinc-800 dark:text-zinc-300 dark:[color-scheme:dark]'
const panelInputCls = 'h-8 rounded-md border border-neutral-200 bg-white px-2.5 text-sm text-neutral-800 outline-none transition-colors focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-800 dark:text-zinc-200 dark:[color-scheme:dark]'

const workerScheduleTriggerOptions = [
  { key: 'message', label: 'schedule.trigger_message' },
  { key: 'task', label: 'schedule.trigger_task' },
  { key: 'attention', label: 'schedule.trigger_attention' },
  { key: 'im_direct_message', label: 'agents.trigger_im_direct_message' },
  { key: 'im_mention', label: 'agents.trigger_im_mention' },
  { key: 'workflow_step_assigned', label: 'agents.trigger_workflow_step_assigned' },
  { key: 'card_action', label: 'agents.trigger_card_action' },
]

function normalizeWorkerSchedule(schedule?: WorkerSchedule) {
  return {
    enabled: Boolean(schedule?.enabled ?? schedule?.Enabled),
    interval: schedule?.interval || schedule?.Interval || '2h',
    jitter: schedule?.jitter || schedule?.Jitter || '',
    activeHours: schedule?.activeHours || schedule?.ActiveHours || '',
    activeDays: schedule?.activeDays || schedule?.ActiveDays || '',
    maxTasksPerCycle: Number(schedule?.maxTasksPerCycle ?? schedule?.MaxTasksPerCycle ?? 5),
    maxCycleDuration: schedule?.maxCycleDuration || schedule?.MaxCycleDuration || '45m',
    wakeupPreset: schedule?.wakeupPreset || schedule?.WakeupPreset || '',
    wakeupCondition: schedule?.wakeupCondition || schedule?.WakeupCondition || '',
    sessionScope: schedule?.sessionScope || schedule?.SessionScope || 'cycle',
    sessionId: schedule?.sessionId || schedule?.SessionID || '',
    triggers: Array.isArray(schedule?.triggers) ? schedule.triggers : [],
  }
}

const WORKER_SCHEDULE_DAYS = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'] as const
const WORKER_SCHEDULE_WEEKDAYS = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri']
const WORKER_SCHEDULE_WEEKENDS = ['Sat', 'Sun']

function parseScheduleDuration(value: string, fallbackNum: number, fallbackUnit: 'm' | 'h') {
  const match = value.trim().match(/^(\d+(?:\.\d+)?)\s*(m|h|s)/)
  if (!match) return { num: fallbackNum, unit: fallbackUnit }
  return { num: Number.parseFloat(match[1]), unit: match[2] === 'h' ? 'h' as const : 'm' as const }
}

function parseScheduleActiveHours(value: string) {
  const match = value.trim().match(/^(\d{1,2}:\d{2})\s*-\s*(\d{1,2}:\d{2})$/)
  if (!match) return { start: '', end: '' }
  const pad = (input: string) => {
    const parts = input.match(/^(\d{1,2}):(\d{2})$/)
    return parts ? `${parts[1].padStart(2, '0')}:${parts[2]}` : input
  }
  return { start: pad(match[1]), end: pad(match[2]) }
}

function parseScheduleActiveDays(value: string): string[] {
  if (!value) return []
  const lower = value.toLowerCase().trim()
  if (lower === 'weekdays') return [...WORKER_SCHEDULE_WEEKDAYS]
  if (lower === 'weekends') return [...WORKER_SCHEDULE_WEEKENDS]
  return value.split(',').map(day => day.trim()).filter(day => (WORKER_SCHEDULE_DAYS as readonly string[]).includes(day))
}

function buildScheduleActiveDays(days: string[]) {
  const sorted = WORKER_SCHEDULE_DAYS.filter(day => days.includes(day))
  if (sorted.length === 0) return ''
  if (sorted.length === 5 && WORKER_SCHEDULE_WEEKDAYS.every(day => sorted.includes(day as (typeof WORKER_SCHEDULE_DAYS)[number]))) return 'weekdays'
  if (sorted.length === 2 && sorted.includes('Sat') && sorted.includes('Sun')) return 'weekends'
  return sorted.join(',')
}

function PromptPreview({ content, emptyText, scroll = true }: { content: string; emptyText: string; scroll?: boolean }) {
  const trimmed = content.trim()
  if (!trimmed) {
    return (
      <div className="p-4 text-sm text-neutral-400 dark:text-zinc-500">
        {emptyText}
      </div>
    )
  }
  return (
    <div className={cn('prose prose-sm prose-neutral dark:prose-invert max-w-none p-4 text-sm leading-relaxed', scroll && 'max-h-[34rem] overflow-auto')}>
      <Markdown remarkPlugins={[remarkGfm]}>{content}</Markdown>
    </div>
  )
}

function PromptEditor({ label, icon: Icon, apiPath, initialContent, canEdit = true, description, emptyText }: { label: string; icon: LucideIcon; apiPath: string; initialContent: string; canEdit?: boolean; description?: string; emptyText?: string }) {
  const { t } = useTranslation()
  const [value, setValue] = useState(initialContent)
  const [editing, setEditing] = useState(false)
  const [dirty, setDirty] = useState(false)
  const [saving, setSaving] = useState(false)
  const [preview, setPreview] = useState(false)
  const [saved, setSaved] = useState(false)
  useEffect(() => {
    setValue(initialContent)
    setDirty(false)
    setSaved(false)
    setEditing(false)
    setPreview(false)
  }, [initialContent])
  const save = useCallback(async () => {
    setSaving(true); setSaved(false)
    try { await apiPut(apiPath, { content: value }); setDirty(false); setEditing(false); setSaved(true); setTimeout(() => setSaved(false), 2000) }
    catch (e) { alert(String(e)) } finally { setSaving(false) }
  }, [apiPath, value])
  function cancelEdit() {
    setValue(initialContent)
    setDirty(false)
    setEditing(false)
    setPreview(false)
  }
  const lineCount = value ? value.split('\n').length : 0
  const editorRows = Math.max(8, Math.min(36, lineCount + 3))
  return (
    <div className="rounded-lg border border-neutral-200/80 bg-white dark:border-zinc-700/60 dark:bg-zinc-900/40">
      <div className="flex items-center justify-between border-b border-neutral-100 px-4 py-2.5 dark:border-zinc-700/40">
        <div className="flex min-w-0 items-start gap-2">
          <Icon className="size-4 text-neutral-400 dark:text-zinc-500" strokeWidth={1.8} />
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <span className="text-sm font-medium text-neutral-700 dark:text-zinc-300">{label}</span>
              {dirty && <span className="text-[10px] text-amber-500">●</span>}
              {saved && <span className="text-[10px] text-emerald-500">{t('prompt.saved')}</span>}
            </div>
            {description && <p className="mt-0.5 text-xs leading-relaxed text-neutral-400 dark:text-zinc-500">{description}</p>}
          </div>
        </div>
        <div className="flex items-center gap-2">
          {editing && (
            <button type="button" onClick={() => setPreview((p) => !p)} className={subtleButtonCls}>
              {preview ? t('prompt.edit') : t('prompt.preview')}
            </button>
          )}
          {canEdit && !editing && (
            <button type="button" onClick={() => setEditing(true)} className={secondaryButtonCls}>
              {t('common.edit')}
            </button>
          )}
          {canEdit && editing && (
            <>
            <button type="button" onClick={cancelEdit} disabled={saving} className={secondaryButtonCls}>
              {t('common.cancel')}
            </button>
            <button type="button" onClick={save} disabled={saving || !dirty} className={primaryButtonCls}>
              {saving ? t('prompt.saving') : t('prompt.save')}
            </button>
            </>
          )}
        </div>
      </div>
      {!editing || preview ? (
        <PromptPreview content={value} emptyText={emptyText || t('prompt.emptyPrompt')} scroll={!editing} />
      ) : (
        <textarea value={value} readOnly={!canEdit} onChange={(e) => { if (!canEdit) return; setValue(e.target.value); setDirty(true); setSaved(false) }}
          className="block w-full resize-y bg-transparent p-4 font-mono text-[13px] leading-relaxed text-neutral-800 outline-none placeholder:text-neutral-300 dark:text-zinc-200 dark:placeholder:text-zinc-700"
          rows={editorRows} placeholder="Markdown prompt..." />
      )}
    </div>
  )
}

function ControlledPromptEditor({ label, icon: Icon, value, onChange, onSave, onCancel, canEdit = true, saving = false, description, emptyText, placeholder }: { label: string; icon: LucideIcon; value: string; onChange: (value: string) => void; onSave: () => void | Promise<void>; onCancel: () => void; canEdit?: boolean; saving?: boolean; description?: string; emptyText: string; placeholder: string }) {
  const { t } = useTranslation()
  const [editing, setEditing] = useState(false)
  const [preview, setPreview] = useState(false)
  const [saved, setSaved] = useState(false)
  const [dirty, setDirty] = useState(false)
  const lineCount = value ? value.split('\n').length : 0
  const editorRows = Math.max(8, Math.min(36, lineCount + 3))
  function cancelEdit() {
    setEditing(false)
    setPreview(false)
    setDirty(false)
    onCancel()
  }
  async function save() {
    await onSave()
    setEditing(false)
    setPreview(false)
    setDirty(false)
    setSaved(true)
    setTimeout(() => setSaved(false), 2000)
  }
  return (
    <div className="rounded-lg border border-neutral-200/80 bg-white dark:border-zinc-700/60 dark:bg-zinc-900/40">
      <div className="flex items-center justify-between border-b border-neutral-100 px-4 py-2.5 dark:border-zinc-700/40">
        <div className="flex min-w-0 items-start gap-2">
          <Icon className="mt-0.5 size-4 text-neutral-400 dark:text-zinc-500" strokeWidth={1.8} />
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <span className="text-sm font-medium text-neutral-700 dark:text-zinc-300">{label}</span>
              {dirty && <span className="text-[10px] text-amber-500">●</span>}
              {saved && <span className="text-[10px] text-emerald-500">{t('prompt.saved')}</span>}
            </div>
            {description && <p className="mt-0.5 text-xs leading-relaxed text-neutral-400 dark:text-zinc-500">{description}</p>}
          </div>
        </div>
        <div className="flex items-center gap-2">
          {editing && (
            <button type="button" onClick={() => setPreview((p) => !p)} className={subtleButtonCls}>
              {preview ? t('prompt.edit') : t('prompt.preview')}
            </button>
          )}
          {canEdit && !editing && (
            <button type="button" onClick={() => setEditing(true)} className={secondaryButtonCls}>
              {t('common.edit')}
            </button>
          )}
          {canEdit && editing && (
            <>
              <button type="button" onClick={cancelEdit} disabled={saving} className={secondaryButtonCls}>
                {t('common.cancel')}
              </button>
              <button type="button" onClick={() => void save()} disabled={saving || !dirty} className={primaryButtonCls}>
                {saving ? t('prompt.saving') : t('prompt.save')}
              </button>
            </>
          )}
        </div>
      </div>
      {!editing || preview ? (
        <PromptPreview content={value} emptyText={emptyText} scroll={!editing} />
      ) : (
        <textarea
          value={value}
          readOnly={!canEdit}
          onChange={(e) => { if (!canEdit) return; onChange(e.target.value); setDirty(true); setSaved(false) }}
          className="block w-full resize-y bg-transparent p-4 font-mono text-[13px] leading-relaxed text-neutral-800 outline-none placeholder:text-neutral-300 dark:text-zinc-200 dark:placeholder:text-zinc-700"
          rows={editorRows}
          placeholder={placeholder}
        />
      )}
    </div>
  )
}

function WorkspaceAgentSchedulePanel({ agentId }: { agentId: string }) {
  const { t } = useTranslation()
  const [reloadKey, setReloadKey] = useState(0)
  const [editing, setEditing] = useState(false)
  const [toggling, setToggling] = useState(false)
  const state = useApiJson<{ agent?: AgentWorkerDetail }>(`/api/v1/agents/${encodeURIComponent(agentId)}`, reloadKey, { keepPreviousDataOnReload: true })
  const agent = state.status === 'ok' ? state.data.agent : undefined
  const schedule = normalizeWorkerSchedule(agent?.schedule)

  async function toggleEnabled() {
    if (state.status !== 'ok') return
    setToggling(true)
    try {
      await apiPatch<{ agent?: AgentWorkerDetail }>(`/api/v1/agents/${encodeURIComponent(agentId)}`, {
        schedule: {
          Enabled: !schedule.enabled,
          Interval: schedule.interval || '2h',
          Jitter: schedule.jitter,
          ActiveHours: schedule.activeHours,
          ActiveDays: schedule.activeDays,
          MaxTasksPerCycle: schedule.maxTasksPerCycle || 5,
          MaxCycleDuration: schedule.maxCycleDuration || '45m',
          WakeupPreset: schedule.wakeupPreset,
          WakeupCondition: schedule.wakeupCondition,
          SessionScope: schedule.sessionScope,
          SessionID: schedule.sessionId,
          triggers: schedule.triggers,
        },
      })
      setReloadKey(k => k + 1)
    } finally {
      setToggling(false)
    }
  }

  return (
    <section data-tour-scheduler-control>
      <SectionHeader icon={Clock3} title={t('agents.schedule')} />
      <p className="mt-1 text-sm text-neutral-500 dark:text-zinc-500">{t('agents.scheduleEnabledHint')}</p>
      <div className="mt-3 rounded-lg border border-neutral-200/80 bg-white p-4 dark:border-zinc-700/60 dark:bg-zinc-900/40">
        <div className="flex items-start justify-between gap-4">
          <div className="min-w-0">
            <p className="text-sm font-medium text-neutral-800 dark:text-zinc-200">{schedule.enabled ? t('schedule.hbActive') : t('schedule.off')}</p>
            <p className="mt-1 text-xs text-neutral-500 dark:text-zinc-500">{t('schedule.configEffectHint')}</p>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <button
              type="button"
              onClick={() => void toggleEnabled()}
              disabled={state.status !== 'ok' || toggling}
              className={schedule.enabled ? secondaryButtonCls : primaryButtonCls}
            >
              {schedule.enabled ? t('schedule.disableHb') : t('schedule.enableHb')}
            </button>
            <button type="button" onClick={() => setEditing(true)} className={secondaryButtonCls}>
              {t('common.edit')}
            </button>
          </div>
        </div>
        {state.status === 'loading' && <p className="mt-3 text-sm text-neutral-500 dark:text-zinc-500">{t('api.loading')}</p>}
        {state.status === 'error' && <p className="mt-3 text-sm text-red-500 dark:text-red-400">{state.error.message}</p>}
        {state.status === 'ok' && (
          <div className="mt-3 grid grid-cols-2 gap-2 text-xs sm:grid-cols-4">
            <ScheduleSummaryItem label={t('schedule.statusLabel')} value={schedule.enabled ? t('schedule.hbActive') : t('schedule.off')} />
            <ScheduleSummaryItem label={t('agents.interval')} value={schedule.enabled ? schedule.interval : '-'} mono />
            <ScheduleSummaryItem label={t('schedule.jitter')} value={schedule.jitter || '-'} mono />
            <ScheduleSummaryItem label={t('schedule.conditionLabel')} value={scheduleConditionSummary(schedule.wakeupPreset, schedule.wakeupCondition, t)} />
            <ScheduleSummaryItem label={t('session.scopeLabel')} value={t(`session.scope${schedule.sessionScope[0]?.toUpperCase() ?? 'C'}${schedule.sessionScope.slice(1)}`, { defaultValue: schedule.sessionScope || '-' })} />
            <ScheduleSummaryItem label={t('agents.activeHours')} value={schedule.activeHours || '-'} />
            <ScheduleSummaryItem label={t('agents.activeDays')} value={schedule.activeDays || '-'} />
            <ScheduleSummaryItem label={t('agents.maxTasksPerCycle')} value={String(schedule.maxTasksPerCycle || '-')} />
            <ScheduleSummaryItem label={t('agents.maxCycleDuration')} value={schedule.maxCycleDuration || '-'} />
            <div className="col-span-2">
              <p className="text-neutral-400 dark:text-zinc-500">{t('agents.wakeupTriggers')}</p>
              <div className="mt-1 flex flex-wrap gap-1.5">
                {schedule.triggers.length === 0 ? (
                  <span className="text-neutral-500 dark:text-zinc-400">-</span>
                ) : schedule.triggers.map(trigger => (
                  <span key={trigger} className="rounded-md border border-neutral-200 px-2 py-0.5 text-neutral-500 dark:border-zinc-700 dark:text-zinc-400">
                    {t(`schedule.trigger_${trigger}`, { defaultValue: t(`agents.trigger_${trigger}`, { defaultValue: trigger }) })}
                  </span>
                ))}
              </div>
            </div>
          </div>
        )}
      </div>
      {editing && state.status === 'ok' && (
        <WorkspaceAgentScheduleDialog
          agentId={agentId}
          agentName={agent?.displayName || agent?.name || agentId}
          schedule={schedule}
          onClose={() => setEditing(false)}
          onSaved={() => {
            setEditing(false)
            setReloadKey(k => k + 1)
          }}
        />
      )}
    </section>
  )
}

function scheduleConditionSummary(preset: string, script: string, t: (key: string, options?: Record<string, unknown>) => string) {
  const parts: string[] = []
  if (preset === 'require_tasks') parts.push(t('schedule.conditionTaskPass'))
  if (preset === 'require_messages') parts.push(t('schedule.conditionMessagePass'))
  if (preset === 'require_any') {
    parts.push(t('schedule.conditionTaskPass'))
    parts.push(t('schedule.conditionMessagePass'))
  }
  if (script.trim()) parts.push(t('schedule.conditionScriptPass'))
  return parts.length > 0 ? parts.join(' / ') : t('schedule.presetNone')
}

function ScheduleSummaryItem({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="rounded-md bg-neutral-50 px-2.5 py-2 dark:bg-zinc-800/50">
      <p className="text-neutral-400 dark:text-zinc-500">{label}</p>
      <p className={cn('mt-0.5 truncate text-neutral-700 dark:text-zinc-300', mono && 'font-mono')}>{value}</p>
    </div>
  )
}

function WorkspaceAgentScheduleDialog({ agentId, agentName, schedule, onClose, onSaved }: {
  agentId: string
  agentName: string
  schedule: ReturnType<typeof normalizeWorkerSchedule>
  onClose: () => void
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const intervalParts = parseScheduleDuration(schedule.interval, 2, 'h')
  const [intervalNum, setIntervalNum] = useState(intervalParts.num)
  const [intervalUnit, setIntervalUnit] = useState<'m' | 'h'>(intervalParts.unit)
  const jitterParts = parseScheduleDuration(schedule.jitter, 0, 'm')
  const [jitterNum, setJitterNum] = useState(jitterParts.num)
  const [jitterUnit, setJitterUnit] = useState<'m' | 'h'>(jitterParts.unit)
  const activeHoursParts = parseScheduleActiveHours(schedule.activeHours)
  const [activeStart, setActiveStart] = useState(activeHoursParts.start)
  const [activeEnd, setActiveEnd] = useState(activeHoursParts.end)
  const [days, setDays] = useState<string[]>(parseScheduleActiveDays(schedule.activeDays))
  const [maxTasks, setMaxTasks] = useState(schedule.maxTasksPerCycle)
  const durationParts = parseScheduleDuration(schedule.maxCycleDuration, 45, 'm')
  const [durationNum, setDurationNum] = useState(durationParts.num)
  const [durationUnit, setDurationUnit] = useState<'m' | 'h'>(durationParts.unit)
  const [triggers, setTriggers] = useState<string[]>(schedule.triggers)
  const [wakeupPreset, setWakeupPreset] = useState(schedule.wakeupPreset)
  const [wakeupCondition, setWakeupCondition] = useState(schedule.wakeupCondition)
  const [useScriptCondition, setUseScriptCondition] = useState(Boolean(schedule.wakeupCondition))
  const [sessionScope, setSessionScope] = useState(schedule.sessionScope || 'cycle')
  const [sessionId, setSessionId] = useState(schedule.sessionId)
  const [saving, setSaving] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  const chipCls = 'cursor-pointer rounded-md border px-2 py-1 text-xs font-medium transition-colors'
  const chipActive = 'border-sky-500 bg-sky-50 text-sky-700 dark:border-sky-600 dark:bg-sky-900/30 dark:text-sky-300'
  const chipInactive = 'border-neutral-200 bg-white text-neutral-500 hover:border-neutral-300 hover:text-neutral-700 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-500 dark:hover:border-zinc-600'
  const shortcutCls = 'cursor-pointer text-[11px] text-sky-600 hover:text-sky-800 dark:text-sky-400 dark:hover:text-sky-300'

  function toggleDay(day: string) {
    setDays(prev => prev.includes(day) ? prev.filter(item => item !== day) : [...prev, day])
  }

  function toggleTrigger(key: string) {
    setTriggers(prev => prev.includes(key) ? prev.filter(item => item !== key) : [...prev, key])
  }

  const presetHasTasks = wakeupPreset === 'require_tasks' || wakeupPreset === 'require_any'
  const presetHasMessages = wakeupPreset === 'require_messages' || wakeupPreset === 'require_any'
  function setPresetConditions(next: { tasks: boolean; messages: boolean }) {
    if (next.tasks && next.messages) setWakeupPreset('require_any')
    else if (next.tasks) setWakeupPreset('require_tasks')
    else if (next.messages) setWakeupPreset('require_messages')
    else setWakeupPreset('')
  }

  function toggleScriptCondition() {
    setUseScriptCondition(prev => {
      if (prev) setWakeupCondition('')
      return !prev
    })
  }

  async function save() {
    setErr(null)
    if (useScriptCondition && !wakeupCondition.trim()) {
      setErr(t('schedule.scriptConditionRequired'))
      return
    }
    setSaving(true)
    try {
      await apiPatch<{ agent?: AgentWorkerDetail }>(`/api/v1/agents/${encodeURIComponent(agentId)}`, {
        schedule: {
          Enabled: schedule.enabled,
          Interval: intervalNum > 0 ? `${intervalNum}${intervalUnit}` : '2h',
          Jitter: jitterNum > 0 ? `${jitterNum}${jitterUnit}` : '',
          ActiveHours: activeStart && activeEnd ? `${activeStart}-${activeEnd}` : '',
          ActiveDays: buildScheduleActiveDays(days),
          MaxTasksPerCycle: maxTasks > 0 ? maxTasks : 5,
          MaxCycleDuration: durationNum > 0 ? `${durationNum}${durationUnit}` : '45m',
          WakeupPreset: wakeupPreset,
          WakeupCondition: useScriptCondition ? wakeupCondition.trim() : '',
          SessionScope: sessionScope,
          SessionID: sessionId,
          triggers,
        },
      })
      showToast(t('agents.scheduleSaved'), 'success')
      onSaved()
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/45 p-4" onClick={() => !saving && onClose()}>
      <div className="flex max-h-[90vh] w-full max-w-lg flex-col rounded-xl border border-neutral-200 bg-white shadow-lg dark:border-zinc-700 dark:bg-zinc-900 animate-scale-in" onClick={(event) => event.stopPropagation()}>
        <div className="flex shrink-0 items-center justify-between border-b border-neutral-200 px-5 py-3 dark:border-zinc-700">
          <h2 className="text-base font-semibold text-neutral-900 dark:text-zinc-100">{t('schedule.editHb')} — <span className="font-mono">{agentName}</span></h2>
          <button type="button" onClick={onClose} disabled={saving} className="rounded-md p-1 text-neutral-400 hover:bg-neutral-100 dark:text-zinc-500 dark:hover:bg-zinc-800">
            <X className="size-4" />
          </button>
        </div>
        <div className="flex-1 space-y-4 overflow-y-auto px-5 py-4">
          <ScheduleDialogField label={t('schedule.interval')}>
            <div className="flex items-center gap-2">
              <input type="number" min={1} value={intervalNum} onChange={event => setIntervalNum(Number(event.target.value))} className={cn(panelInputCls, 'w-24 tabular-nums')} />
              <select value={intervalUnit} onChange={event => setIntervalUnit(event.target.value as 'm' | 'h')} className={cn(panelSelectCls, 'w-28')}>
                <option value="m">{t('schedule.unitMinutes')}</option>
                <option value="h">{t('schedule.unitHours')}</option>
              </select>
            </div>
          </ScheduleDialogField>

          <ScheduleDialogField label={t('schedule.jitter')}>
            <div className="flex items-center gap-2">
              <input type="number" min={0} value={jitterNum} onChange={event => setJitterNum(Number(event.target.value))} className={cn(panelInputCls, 'w-24 tabular-nums')} />
              <select value={jitterUnit} onChange={event => setJitterUnit(event.target.value as 'm' | 'h')} className={cn(panelSelectCls, 'w-28')}>
                <option value="m">{t('schedule.unitMinutes')}</option>
                <option value="h">{t('schedule.unitHours')}</option>
              </select>
            </div>
            <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">{t('schedule.jitterHint')}</p>
          </ScheduleDialogField>

          <ScheduleDialogField label={t('schedule.activeHours')}>
            <div className="flex items-center gap-2">
              <input type="time" value={activeStart} onChange={event => setActiveStart(event.target.value)} className={cn(panelInputCls, 'w-32')} />
              <span className="text-neutral-400">—</span>
              <input type="time" value={activeEnd} onChange={event => setActiveEnd(event.target.value)} className={cn(panelInputCls, 'w-32')} />
            </div>
          </ScheduleDialogField>

          <ScheduleDialogField label={t('schedule.activeDaysLabel')}>
            <div className="flex flex-wrap gap-1.5">
              {WORKER_SCHEDULE_DAYS.map(day => (
                <button key={day} type="button" onClick={() => toggleDay(day)} className={cn(chipCls, days.includes(day) ? chipActive : chipInactive)}>{day}</button>
              ))}
            </div>
            <div className="mt-1.5 flex gap-3">
              <button type="button" onClick={() => setDays([...WORKER_SCHEDULE_WEEKDAYS])} className={shortcutCls}>{t('schedule.shortcutWeekdays')}</button>
              <button type="button" onClick={() => setDays([...WORKER_SCHEDULE_WEEKENDS])} className={shortcutCls}>{t('schedule.shortcutWeekends')}</button>
              <button type="button" onClick={() => setDays([...WORKER_SCHEDULE_DAYS])} className={shortcutCls}>{t('schedule.shortcutAll')}</button>
              <button type="button" onClick={() => setDays([])} className={shortcutCls}>{t('schedule.shortcutClear')}</button>
            </div>
          </ScheduleDialogField>

          <ScheduleDialogField label={t('schedule.wakeupCondition')}>
            <div className="space-y-2 rounded-lg border border-neutral-200 bg-neutral-50/50 p-3 dark:border-zinc-700/60 dark:bg-zinc-800/30">
              <div className="flex flex-wrap gap-2">
                <button type="button" onClick={() => setPresetConditions({ tasks: !presetHasTasks, messages: presetHasMessages })} className={cn(chipCls, presetHasTasks ? chipActive : chipInactive)}>
                  {t('schedule.conditionTaskPass')}
                </button>
                <button type="button" onClick={() => setPresetConditions({ tasks: presetHasTasks, messages: !presetHasMessages })} className={cn(chipCls, presetHasMessages ? chipActive : chipInactive)}>
                  {t('schedule.conditionMessagePass')}
                </button>
                <button type="button" onClick={toggleScriptCondition} className={cn(chipCls, useScriptCondition ? chipActive : chipInactive)}>
                  {t('schedule.conditionScriptPass')}
                </button>
              </div>
              {useScriptCondition && (
                <textarea
                  value={wakeupCondition}
                  onChange={event => setWakeupCondition(event.target.value)}
                  rows={3}
                  placeholder={t('schedule.wakeupConditionPlaceholder')}
                  className={cn(panelInputCls, 'min-h-20 w-full resize-y py-2 font-mono text-xs')}
                />
              )}
              <p className="text-xs text-neutral-400 dark:text-zinc-500">{t('schedule.wakeupConditionHint')}</p>
            </div>
          </ScheduleDialogField>

          <ScheduleDialogField label={t('schedule.triggers')}>
            <div className="flex flex-wrap gap-2">
              {workerScheduleTriggerOptions.map(option => (
                <button key={option.key} type="button" onClick={() => toggleTrigger(option.key)} className={cn(chipCls, triggers.includes(option.key) ? chipActive : chipInactive)}>
                  {t(option.label)}
                </button>
              ))}
            </div>
            <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">{t('schedule.triggersHint')}</p>
          </ScheduleDialogField>

          <div className="rounded-lg border border-neutral-200 bg-neutral-50/50 px-4 py-3 dark:border-zinc-700/60 dark:bg-zinc-800/30">
            <p className="mb-2.5 text-xs font-semibold uppercase tracking-wider text-neutral-500 dark:text-zinc-400">{t('session.sessionLabel')}</p>
            <div className="space-y-3">
              <ScheduleDialogField label={t('session.scopeLabel')}>
                <select value={sessionScope} onChange={event => setSessionScope(event.target.value)} className={panelSelectCls}>
                  <option value="cycle">{t('session.scopeCycle')}</option>
                  <option value="task">{t('session.scopeTask')}</option>
                  <option value="persistent">{t('session.scopePersistent')}</option>
                </select>
                <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">{t('session.scopeHint')}</p>
              </ScheduleDialogField>
              <ScheduleDialogField label={t('session.sessionIdLabel')}>
                <div className="flex items-center gap-2">
                  <input
                    value={sessionId}
                    onChange={event => setSessionId(event.target.value)}
                    placeholder={t('session.sessionIdPlaceholder')}
                    className={cn(panelInputCls, 'flex-1 font-mono text-xs')}
                  />
                  {sessionId && (
                    <button type="button" onClick={() => setSessionId('')} className={secondaryButtonCls}>
                      {t('session.clearSession')}
                    </button>
                  )}
                </div>
                <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">{t('session.sessionIdHint')}</p>
              </ScheduleDialogField>
            </div>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <ScheduleDialogField label={t('schedule.maxTasks')}>
              <input type="number" min={1} value={maxTasks} onChange={event => setMaxTasks(Number(event.target.value))} className={cn(panelInputCls, 'w-full tabular-nums')} />
            </ScheduleDialogField>
            <ScheduleDialogField label={t('schedule.maxDuration')}>
              <div className="flex items-center gap-2">
                <input type="number" min={1} value={durationNum} onChange={event => setDurationNum(Number(event.target.value))} className={cn(panelInputCls, 'w-20 tabular-nums')} />
                <select value={durationUnit} onChange={event => setDurationUnit(event.target.value as 'm' | 'h')} className={cn(panelSelectCls, 'w-24')}>
                  <option value="m">{t('schedule.unitMinutes')}</option>
                  <option value="h">{t('schedule.unitHours')}</option>
                </select>
              </div>
            </ScheduleDialogField>
          </div>

          {err && <p className="text-sm text-red-600 dark:text-red-400">{err}</p>}
          <div className="flex justify-end gap-2 pt-1">
            <button type="button" onClick={onClose} disabled={saving} className="rounded-lg border border-neutral-300 px-3 py-1.5 text-sm dark:border-zinc-600">{t('forms.cancel')}</button>
            <button type="button" onClick={() => void save()} disabled={saving} className="rounded-lg bg-sky-600 px-3 py-1.5 text-sm font-medium text-white disabled:opacity-50">
              {saving ? t('forms.saving') : t('forms.save')}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

function ScheduleDialogField({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block text-sm">
      <span className="mb-1 block text-xs font-medium text-neutral-500 dark:text-zinc-500">{label}</span>
      {children}
    </label>
  )
}

function SessionPanel({ project, agentName, canConfigure, canRun, runBlockedReason }: { project: string; agentName: string; canConfigure: boolean; canRun: boolean; runBlockedReason?: string }) {
  const { t } = useTranslation()
  const fmt = useFormatDateTime()
  const hbPath = `/api/v1/projects/${encodeURIComponent(project)}/agents/${encodeURIComponent(agentName)}/heartbeat`
  const [reloadKey, setReloadKey] = useState(0)
  const state = useApiJson<SessionInfo & Record<string, unknown>>(hbPath, reloadKey)
  const [resetting, setResetting] = useState(false)
  const [runResult, setRunResult] = useState<string | null>(null)
  const [running, setRunning] = useState(false)

  if (state.status !== 'ok') return null
  const info = state.data
  const hasSession = !!info.sessionId

  async function doReset() {
    setResetting(true)
    try {
      await apiPost('/api/v1/session/reset', { project, agent: agentName })
      setReloadKey((k) => k + 1)
    } catch (e) { alert(String(e)) }
    finally { setResetting(false) }
  }

  async function doRun() {
    setRunning(true); setRunResult(null)
    try {
      const res = await apiPost<{ ok: boolean; output: string }>('/api/v1/run', { project, agent: agentName })
      setRunResult(res.output || t('session.runDone'))
    } catch (e) { setRunResult(String(e)) }
    finally { setRunning(false); setReloadKey((k) => k + 1) }
  }

  return (
    <div className="rounded-lg border border-neutral-200/80 bg-neutral-50/50 px-4 py-3 dark:border-zinc-700/60 dark:bg-zinc-900/30">
      <h4 className="mb-2 text-xs font-semibold uppercase tracking-wider text-neutral-400 dark:text-zinc-500">{t('agentDetail.webSession')}</h4>
      <div className="flex flex-wrap items-center gap-x-6 gap-y-2 text-xs">
        <span className="text-neutral-500 dark:text-zinc-500">
          Session ID: {hasSession ? <span className="font-mono text-emerald-700 dark:text-emerald-400" title={info.sessionId}>{info.sessionId!.slice(0, 16)}…</span> : <span className="text-neutral-400 dark:text-zinc-500">{t('session.noSession')}</span>}
        </span>
        {info.sessionStartedAt && (
          <span className="text-neutral-500 dark:text-zinc-500">{t('session.startedAt')}: {fmt(info.sessionStartedAt)}</span>
        )}
        <span className="text-neutral-500 dark:text-zinc-500">{t('session.scopeLabel')}: <span className="font-medium text-neutral-700 dark:text-zinc-300">{info.sessionScope === 'task' ? t('session.scopeTask') : t('session.scopeCycle')}</span></span>

        <div className="flex items-center gap-2">
          <Link
            to={`/projects/${encodeURIComponent(project)}/members/${encodeURIComponent(agentName)}/chat${hasSession ? `?sessionId=${encodeURIComponent(info.sessionId!)}` : ''}`}
            className={primaryButtonCls}
          >
            {t('agentChat.openChat')}
          </Link>
          {canConfigure && hasSession && (
            <button type="button" onClick={() => void doReset()} disabled={resetting}
              className={secondaryButtonCls}>
              {resetting ? t('session.resettingSession') : t('session.resetSession')}
            </button>
          )}
          {canRun && (
            <button type="button" onClick={() => void doRun()} disabled={running || Boolean(runBlockedReason)} title={runBlockedReason || undefined}
              className={secondaryButtonCls}>
              {running ? t('session.running') : t('session.run')}
            </button>
          )}
        </div>
      </div>
      {runResult && (
        <pre className="mt-2 max-h-36 overflow-auto rounded-md bg-white p-3 font-mono text-xs leading-relaxed text-neutral-600 dark:bg-zinc-800 dark:text-zinc-400">{runResult}</pre>
      )}
    </div>
  )
}

function ModelSelector({ project, agentName, currentModel, currentHttpAgent, onChanged }: {
  project: string; agentName: string; currentModel: string; currentHttpAgent?: HTTPAgentConfig; onChanged: () => void
}) {
  const { t } = useTranslation()
  const [model, setModel] = useState(currentModel)
  const [httpUrl, setHttpUrl] = useState(currentHttpAgent?.url ?? '')
  const [httpModel, setHttpModel] = useState(currentHttpAgent?.model ?? '')
  const [httpApiKey, setHttpApiKey] = useState(currentHttpAgent?.api_key ?? '')
  const [httpTimeout, setHttpTimeout] = useState(currentHttpAgent?.timeout ?? '10m')
  const [httpStream, setHttpStream] = useState(currentHttpAgent?.stream ?? true)
  const [busy, setBusy] = useState(false)
  const [result, setResult] = useState<{ ok: boolean; msg: string } | null>(null)

  const isHttp = model === 'http-agent'
  const modelDirty = model !== currentModel
  const httpDirty = isHttp && (
    httpUrl !== (currentHttpAgent?.url ?? '') ||
    httpModel !== (currentHttpAgent?.model ?? '') ||
    httpApiKey !== (currentHttpAgent?.api_key ?? '') ||
    httpTimeout !== (currentHttpAgent?.timeout ?? '10m') ||
    httpStream !== (currentHttpAgent?.stream ?? true)
  )
  const dirty = modelDirty || httpDirty

  async function apply() {
    if (isHttp && !httpUrl.trim()) {
      setResult({ ok: false, msg: t('agentDetail.customCliUrlRequired') })
      return
    }
    setBusy(true); setResult(null)
    try {
      const body: Record<string, unknown> = { model }
      if (isHttp) {
        body.httpUrl = httpUrl
        body.httpModel = httpModel
        body.httpApiKey = httpApiKey
        body.httpTimeout = httpTimeout
        body.httpStream = httpStream
      }
      await apiPost<{ ok: boolean }>(
        `/api/v1/projects/${encodeURIComponent(project)}/agents/${encodeURIComponent(agentName)}/set-model`,
        body,
      )
      setResult({ ok: true, msg: t('forms.saved') })
      onChanged()
    } catch (e) {
      setResult({ ok: false, msg: e instanceof Error ? e.message : String(e) })
    } finally { setBusy(false) }
  }

  const inputCls = 'h-7 rounded-md border border-neutral-200 bg-white px-2 text-xs text-neutral-700 outline-none hover:border-neutral-300 focus:border-sky-400 disabled:opacity-50 dark:border-zinc-700 dark:bg-zinc-800 dark:text-zinc-300 dark:[color-scheme:dark]'

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <span className="text-sm text-neutral-500 dark:text-zinc-500">{t('members.agentType')}:</span>
        <select
          value={model}
          onChange={(e) => { setModel(e.target.value); setResult(null) }}
          disabled={busy}
          className={cn(inputCls, 'font-medium')}
        >
          {AGENT_MODELS.map((m) => (
            <option key={m} value={m}>{m}</option>
          ))}
        </select>
        {dirty && (
          <button
            type="button"
            onClick={() => void apply()}
            disabled={busy}
            className={primaryButtonCls}
          >
            {busy ? t('forms.saving') : t('forms.apply')}
          </button>
        )}
        {result && (
          <span className={cn('text-[11px]', result.ok ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-500')}>
            {result.ok && <Check className="mr-0.5 inline size-3" strokeWidth={2} />}
            {result.msg.split('\n')[0]}
          </span>
        )}
      </div>
      {isHttp && (
        <div className="grid grid-cols-[auto_1fr] items-center gap-x-3 gap-y-1.5 rounded-lg border border-amber-200 bg-amber-50/50 p-3 text-xs dark:border-amber-900/40 dark:bg-amber-950/20">
          <span className="text-neutral-500 dark:text-zinc-500">{t('agentDetail.customCliUrl')} *</span>
          <input value={httpUrl} onChange={(e) => setHttpUrl(e.target.value)} disabled={busy}
            placeholder="http://localhost:11434/v1/chat/completions" className={cn(inputCls, 'w-full')} />
          <span className="text-neutral-500 dark:text-zinc-500">{t('agentDetail.customCliModel')}</span>
          <input value={httpModel} onChange={(e) => setHttpModel(e.target.value)} disabled={busy}
            placeholder="llama3.2, gpt-4o, ..." className={cn(inputCls, 'w-full')} />
          <span className="text-neutral-500 dark:text-zinc-500">API Key</span>
          <input value={httpApiKey} onChange={(e) => setHttpApiKey(e.target.value)} disabled={busy}
            type="password" placeholder="Bearer token" className={cn(inputCls, 'w-full')} />
          <span className="text-neutral-500 dark:text-zinc-500">{t('agentDetail.customCliTimeout')}</span>
          <input value={httpTimeout} onChange={(e) => setHttpTimeout(e.target.value)} disabled={busy}
            placeholder="10m" className={cn(inputCls, 'w-24')} />
          <span className="text-neutral-500 dark:text-zinc-500">{t('agentDetail.customCliStream')}</span>
          <label className="flex cursor-pointer items-center gap-1.5">
            <input type="checkbox" checked={httpStream} onChange={(e) => setHttpStream(e.target.checked)} disabled={busy}
              className="size-3.5 rounded border-neutral-300 text-sky-600 focus:ring-sky-400" />
            <span className="text-neutral-500 dark:text-zinc-400">SSE</span>
          </label>
        </div>
      )}
    </div>
  )
}

type ProjectAgentDetailPageProps = {
  projectIdOverride?: string
  agentNameOverride?: string
  workspaceAgentId?: string
  workspaceAgent?: WorkspaceAgentSummary
}

type WorkspaceAgentSummary = {
  id: string
  name: string
  displayName?: string
  description?: string
  profilePrompt?: string
  avatar?: string
  team?: string
  role?: string
  status?: string
  model?: string
  runtimeModel?: string
  defaultModelAccountId?: string
  defaultRuntimeNodeId?: string
  runtimeConfig?: {
    maxForkSessions?: number
  }
}

type WorkspaceAgentMembership = {
  id: string
  projectId: string
  title?: string
  role?: string
  team?: string
}

type WorkspaceTeamInfo = { path: string; name: string }
type WorkspaceTeamDetail = { roles?: Array<{ id?: string; name: string; description?: string }> }

export default function ProjectAgentDetailPage({ projectIdOverride, agentNameOverride, workspaceAgentId, workspaceAgent }: ProjectAgentDetailPageProps = {}) {
  const { t } = useTranslation()
  const { user } = useAuth()
  const { canAdmin: canAdminWorkspace } = useWorkspaceAccess()
  const trustedProxyMode = isTrustedProxyMode()
  const navigate = useNavigate()
  const params = useParams<{ projectId: string; agentName: string }>()
  const projectId = projectIdOverride ?? params.projectId
  const agentName = agentNameOverride ?? params.agentName
  const workspaceOnly = !projectId && Boolean(workspaceAgentId && workspaceAgent)

  const ctxPath = projectId && agentName
    ? `/api/v1/projects/${encodeURIComponent(projectId)}/agents/${encodeURIComponent(agentName)}/context?includeMerged=false&includeReadiness=false`
    : null
  const fullContextPath = projectId && agentName
    ? `/api/v1/projects/${encodeURIComponent(projectId)}/agents/${encodeURIComponent(agentName)}/context`
    : null
  const [ctxReload, setCtxReload] = useState(0)
  const ctxState = useApiJson<AgentContext>(ctxPath, ctxReload, { keepPreviousDataOnReload: true })

  const [editingIdentity, setEditingIdentity] = useState(false)
  const [identityName, setIdentityName] = useState(agentName ?? '')
  const [identityAvatar, setIdentityAvatar] = useState('')
  const [avatarNonce, setAvatarNonce] = useState(() => Date.now())
  const [savingIdentity, setSavingIdentity] = useState(false)
  const [identityError, setIdentityError] = useState<string | null>(null)

  useEffect(() => {
    if (ctxState.status !== 'ok') return
    setIdentityName(agentName ?? '')
    setIdentityAvatar(ctxState.data.avatar ?? '')
  }, [agentName, ctxState.status, ctxState.status === 'ok' ? ctxState.data.avatar : undefined])

  const saveIdentity = useCallback(async () => {
    if (!projectId || !agentName) return
    setSavingIdentity(true)
    setIdentityError(null)
    try {
      const res = await apiPatch<{ ok: boolean; name: string; avatar: string }>(
        `/api/v1/projects/${encodeURIComponent(projectId)}/agents/${encodeURIComponent(agentName)}`,
        { name: identityName.trim(), avatar: identityAvatar.trim() },
      )
      setEditingIdentity(false)
      if (res.name && res.name !== agentName) {
        navigate(workspaceAgentId ? `/agents/${encodeURIComponent(res.name)}` : `/projects/${encodeURIComponent(projectId)}/members/${encodeURIComponent(res.name)}`, { replace: true })
        return
      }
      setIdentityAvatar(res.avatar ?? '')
      setCtxReload((k) => k + 1)
    } catch (e) {
      setIdentityError(e instanceof Error ? e.message : String(e))
    } finally {
      setSavingIdentity(false)
    }
  }, [projectId, agentName, identityName, identityAvatar, navigate, workspaceAgentId])

  const identityAvatarChoices = useMemo(
    () => avatarChoices(projectId ?? '', identityName || agentName || 'agent', avatarNonce),
    [projectId, identityName, agentName, avatarNonce],
  )

  if (workspaceOnly && workspaceAgent) {
    return <WorkspaceAgentOnlyDetail agent={workspaceAgent} />
  }

  if (!projectId || !agentName) return null

  const canManageThisProject = canAdminWorkspace || canManageProject(user, projectId)
  const canConfigureThisAgent = canAdminWorkspace || canConfigureAgent(user, projectId, agentName)
  const canRunThisAgent = canAdminWorkspace || canOperateAgent(user, projectId, agentName)
  const isHuman = ctxState.status === 'ok' && ctxState.data.model === 'human'
  const modelCls = MODEL_COLORS[ctxState.status === 'ok' ? ctxState.data.model : ''] ?? ''
  const HeaderIcon = isHuman ? User : Bot
  const headerIconBg = isHuman ? 'bg-indigo-100 dark:bg-indigo-900/30' : 'bg-violet-100 dark:bg-violet-900/30'
  const headerIconColor = isHuman ? 'text-indigo-600 dark:text-indigo-400' : 'text-violet-600 dark:text-violet-400'
  const avatar = ctxState.status === 'ok' ? ctxState.data.avatar : ''

  return (
    <div className="flex h-full flex-col overflow-hidden">
      {/* Header */}
      <div className="shrink-0 px-6 pt-5 pb-4">
        <div className="flex items-center gap-4">
          {avatar ? (
            <img src={avatar} alt="" className="size-12 shrink-0 rounded-xl bg-neutral-100 object-cover dark:bg-zinc-800" />
          ) : (
            <div className={cn('flex size-12 shrink-0 items-center justify-center rounded-xl', headerIconBg)}>
              <HeaderIcon className={cn('size-6', headerIconColor)} strokeWidth={1.8} />
            </div>
          )}
          <div className="min-w-0 flex-1">
            {editingIdentity ? (
              <div className="max-w-2xl space-y-2">
                <div className="flex flex-wrap items-center gap-2">
                  <input
                    value={identityName}
                    onChange={(e) => setIdentityName(e.target.value)}
                    className="h-8 w-56 rounded-md border border-neutral-200 bg-white px-2.5 font-mono text-sm text-neutral-900 outline-none focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100"
                    placeholder="agent-name"
                  />
                  <input
                    value={identityAvatar}
                    onChange={(e) => setIdentityAvatar(e.target.value)}
                    className="h-8 min-w-0 flex-1 rounded-md border border-neutral-200 bg-white px-2.5 text-sm text-neutral-900 outline-none focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100"
                    placeholder={t('agentIdentity.avatarPlaceholder')}
                  />
                  <button type="button" onClick={() => void saveIdentity()} disabled={savingIdentity}
                    className={primaryButtonCls}>
                    {savingIdentity ? t('forms.saving') : t('forms.save')}
                  </button>
                  <button type="button" onClick={() => { setEditingIdentity(false); setIdentityError(null); setIdentityName(agentName); setIdentityAvatar(avatar ?? '') }} disabled={savingIdentity}
                    className={secondaryButtonCls}>
                    {t('forms.cancel')}
                  </button>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  {identityAvatarChoices.map((url) => (
                    <button
                      key={url}
                      type="button"
                      onClick={() => setIdentityAvatar(url)}
                      className={cn(
                        'rounded-lg border p-0.5 transition-colors',
                        identityAvatar === url
                          ? 'border-sky-500 bg-sky-50 dark:border-sky-400 dark:bg-sky-900/20'
                          : 'border-neutral-200 hover:border-neutral-300 dark:border-zinc-700 dark:hover:border-zinc-600',
                      )}
                      title={t('agentIdentity.chooseAvatar')}
                    >
                      <img src={url} alt="" className="size-8 rounded-md bg-neutral-100 object-cover dark:bg-zinc-800" />
                    </button>
                  ))}
                  <button
                    type="button"
                    onClick={() => setAvatarNonce(Date.now())}
                    className="rounded-md border border-neutral-300 px-2.5 py-1.5 text-xs text-neutral-600 hover:bg-neutral-50 dark:border-zinc-600 dark:text-zinc-300 dark:hover:bg-zinc-800"
                  >
                    {t('agentIdentity.randomAvatars')}
                  </button>
                  {identityAvatar && (
                    <button
                      type="button"
                      onClick={() => setIdentityAvatar('')}
                      className="rounded-md border border-neutral-300 px-2.5 py-1.5 text-xs text-neutral-600 hover:bg-neutral-50 dark:border-zinc-600 dark:text-zinc-300 dark:hover:bg-zinc-800"
                    >
                      {t('agentIdentity.clearAvatar')}
                    </button>
                  )}
                </div>
                {identityError && <p className="text-xs text-red-600 dark:text-red-400">{identityError}</p>}
              </div>
            ) : (
              <div className="flex items-center gap-2">
                <h1 className="truncate text-xl font-semibold text-neutral-900 dark:text-zinc-100">{agentName}</h1>
                {ctxState.status === 'ok' && canManageThisProject && (
                  <button type="button" onClick={() => setEditingIdentity(true)} className={subtleButtonCls}>
                    {t('common.edit')}
                  </button>
                )}
              </div>
            )}
            <div className="mt-1 flex items-center gap-3">
              {ctxState.status === 'ok' && (
                <>
                  <span className={cn('inline-flex items-center rounded-md px-2.5 py-0.5 text-xs font-bold tracking-wide', modelCls)}>
                    {ctxState.data.model}
                  </span>
                  {ctxState.data.team && <span className="text-sm text-neutral-500 dark:text-zinc-500">{ctxState.data.team}</span>}
                  {ctxState.data.role && <span className="text-sm text-neutral-500 dark:text-zinc-500">/ {ctxState.data.role}</span>}
                </>
              )}
            </div>
          </div>
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto px-6 pb-8">
        {ctxState.status === 'loading' && (
          <div className="flex items-center gap-2 py-12 justify-center">
            <div className="size-5 animate-spin rounded-full border-2 border-neutral-300 border-t-sky-600 dark:border-zinc-600 dark:border-t-sky-400" />
            <span className="text-sm text-neutral-500">{t('api.loading')}</span>
          </div>
        )}
        {ctxState.status === 'error' && (
          <p className="py-3 text-sm text-red-500">{ctxState.error.message}</p>
        )}

        {/* ── Human member view ── */}
        {ctxState.status === 'ok' && isHuman && (
          <div className="space-y-8">
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
              {ctxState.data.team && <InfoCard icon={Users} label={t('prompt.team')} value={ctxState.data.team} />}
              {ctxState.data.role && <InfoCard icon={UserCog} label={t('prompt.role')} value={ctxState.data.role} />}
            </div>
            <HumanMessagesPanel project={projectId} member={agentName} />
            <HumanTasksPanel project={projectId} member={agentName} />
          </div>
        )}

        {/* ── Agent member view ── */}
        {ctxState.status === 'ok' && !isHuman && (() => {
          const ctx = ctxState.data
          return (
            <div className="space-y-8">
              {canConfigureThisAgent && (
                <section data-tour-agent-model-config>
                  <SectionHeader icon={Settings2} title={t('agentDetail.modelCredentials')} />
                  <p className="mt-1 text-sm text-neutral-500 dark:text-zinc-500">{t('agentDetail.modelCredentialsHint')}</p>
                  <ModelCredentialsPanel
                    project={projectId}
                    agentName={agentName}
                    ctx={ctx}
                    onChanged={() => setCtxReload((k) => k + 1)}
                  />
                </section>
              )}

              {canConfigureThisAgent && trustedProxyMode && (
                <section>
                  <SectionHeader icon={Server} title={t('agentDetail.runtimeNode')} />
                  <p className="mt-1 text-sm text-neutral-500 dark:text-zinc-500">{t('agentDetail.runtimeNodeBindingHint')}</p>
                  <RuntimeNodeBindingPanel
                    project={projectId}
                    agentName={agentName}
                    runtimeNodeId={ctx.runtimeNodeId}
                    onChanged={() => setCtxReload((k) => k + 1)}
                  />
                </section>
              )}

              <section>
                <SectionHeader icon={Activity} title={t('agentDetail.connectAndChat')} />
                <p className="mt-1 text-sm text-neutral-500 dark:text-zinc-500">{t('agentDetail.connectAndChatHint')}</p>
                <div className="mt-3 space-y-4">
                  <SessionPanel
                    project={projectId}
                    agentName={agentName}
                    canConfigure={canConfigureThisAgent}
                    canRun={canRunThisAgent}
                    runBlockedReason={agentModelRequiresModelAccount(ctx.model) && !ctx.provider ? t('agentDetail.modelAccountRequiredBeforeRun') : undefined}
                  />
                  {canConfigureThisAgent && (
                    <AgentChannelPanel
                      project={projectId}
                      agentName={agentName}
                      agentWorkerId={workspaceAgentId}
                    />
                  )}
                </div>
              </section>

              {canConfigureThisAgent && (
                <>
                  <section>
                    <SectionHeader icon={Cable} title={t('agentDetail.capabilities')} />
                    <p className="mt-1 text-sm text-neutral-500 dark:text-zinc-500">{t('agentDetail.capabilitiesHint')}</p>
                    <div className="mt-3 divide-y divide-neutral-100 rounded-lg border border-neutral-200/80 bg-white dark:divide-zinc-800 dark:border-zinc-700/60 dark:bg-zinc-900/40">
                      <AgentRuntimeConnectionsPanel project={projectId} agentName={agentName} agentWorkerId={workspaceAgentId} />
                      <AgentSkillsPanel skills={ctx.skills ?? []} skillDetails={ctx.skillDetails} />
                    </div>
                  </section>
                </>
              )}

              {workspaceAgentId && canAdminWorkspace && (
                <WorkspaceAgentSchedulePanel agentId={workspaceAgentId} />
              )}

              <section data-tour-agent-wakeup-prompt>
                <SectionHeader icon={BookOpen} title={t('agentDetail.promptContext')} />
                <p className="mt-1 text-sm text-neutral-500 dark:text-zinc-500">{t('agentDetail.promptContextHint')}</p>
                <div className="mt-3 space-y-4">
                  <PromptEditor
                    label={t('prompt.wakeup')}
                    icon={BookOpen}
                    apiPath={`/api/v1/projects/${encodeURIComponent(projectId)}/agents/${encodeURIComponent(agentName)}/wakeup`}
                    initialContent={ctx.wakeup}
                    canEdit={canConfigureThisAgent}
                  />
                  <ContextPanel apiPath={fullContextPath} contextFile={ctx.contextFile} syncedAt={ctx.syncedAt} />
                  <AgentContextBindingsPanel
                    project={projectId}
                    agentName={agentName}
                    canEdit={canConfigureThisAgent}
                    onChanged={() => setCtxReload((k) => k + 1)}
                  />
                </div>
              </section>

              {canConfigureThisAgent && !trustedProxyMode && (
                <details className="group rounded-lg border border-neutral-200/80 bg-white dark:border-zinc-700/60 dark:bg-zinc-900/40">
                  <summary className="flex cursor-pointer items-center gap-2 px-4 py-3 text-sm font-medium text-neutral-700 dark:text-zinc-300">
                    <ChevronRight className="size-4 transition-transform group-open:rotate-90" strokeWidth={2} />
                    <Container className="size-4 text-neutral-400 dark:text-zinc-500" strokeWidth={1.8} />
                    {t('agentDetail.advancedRuntime')}
                  </summary>
                  <div className="border-t border-neutral-100 p-4 dark:border-zinc-800">
                    <div className="space-y-4">
                      <SandboxEditor
                        project={projectId}
                        agentName={agentName}
                        initial={ctx.sandbox}
                        onChanged={() => setCtxReload((k) => k + 1)}
                      />
                      <RuntimeNodeBindingPanel
                        project={projectId}
                        agentName={agentName}
                        runtimeNodeId={ctx.runtimeNodeId}
                        onChanged={() => setCtxReload((k) => k + 1)}
                      />
                    </div>
                  </div>
                </details>
              )}
            </div>
          )
        })()}
      </div>
    </div>
  )
}

function InfoCard({ icon: Icon, label, value, mono }: { icon?: LucideIcon; label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex items-start gap-3 rounded-lg border border-neutral-200/80 bg-neutral-50/50 px-4 py-3 dark:border-zinc-700/60 dark:bg-zinc-800/30">
      {Icon && (
        <div className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-md bg-neutral-100 dark:bg-zinc-700/60">
          <Icon className="size-3.5 text-neutral-500 dark:text-zinc-400" strokeWidth={1.8} />
        </div>
      )}
      <div className="min-w-0">
        <p className="text-xs font-medium text-neutral-400 dark:text-zinc-500">{label}</p>
        <p className={cn('mt-0.5 text-sm font-medium text-neutral-800 dark:text-zinc-200', mono && 'font-mono text-xs')} title={value}>{value}</p>
      </div>
    </div>
  )
}

function WorkspaceAgentOnlyDetail({ agent }: { agent: WorkspaceAgentSummary }) {
  const { t } = useTranslation()
  const { canAdmin: canAdminWorkspace } = useWorkspaceAccess()
  const trustedProxyMode = isTrustedProxyMode()
  const [reloadKey, setReloadKey] = useState(0)
  const agentState = useApiJson<{ agent?: WorkspaceAgentSummary; memberships?: WorkspaceAgentMembership[] }>(`/api/v1/agents/${encodeURIComponent(agent.id)}`, reloadKey, { keepPreviousDataOnReload: true })
  const current = agentState.status === 'ok' && agentState.data.agent ? agentState.data.agent : agent
  const memberships = agentState.status === 'ok' ? (agentState.data.memberships ?? []) : []
  const chatMembership = memberships.find((membership) => membership.projectId) ?? null
  const providersState = useApiJson<ProviderOption[]>('/api/v1/providers', 0, { keepPreviousDataOnReload: true })
  const nodesState = useApiJson<RuntimeNodeListResp>('/api/v1/runtime-nodes', 0, { keepPreviousDataOnReload: true })
  const [ctxReload, setCtxReload] = useState(0)
  const ctxState = useApiJson<AgentContext>(`/api/v1/agents/${encodeURIComponent(agent.id)}/context?includeMerged=false&includeReadiness=false`, ctxReload, { keepPreviousDataOnReload: true })
  const fullContextPath = `/api/v1/agents/${encodeURIComponent(agent.id)}/context`
  const [displayName, setDisplayName] = useState(current.displayName || current.name)
  const [description, setDescription] = useState(current.description || '')
  const [profilePrompt, setProfilePrompt] = useState(current.profilePrompt || '')
  const [team, setTeam] = useState(current.team || '')
  const [role, setRole] = useState(current.role || '')
  const [status, setStatus] = useState(current.status || 'active')
  const [model, setModel] = useState(current.model || 'codex')
  const [runtimeModel, setRuntimeModel] = useState(current.runtimeModel || '')
  const [modelAccountId, setModelAccountId] = useState(current.defaultModelAccountId || '')
  const [runtimeNodeId, setRuntimeNodeId] = useState(current.defaultRuntimeNodeId || '')
  const [editingProfile, setEditingProfile] = useState(false)
  const [profileAvatar, setProfileAvatar] = useState(current.avatar || '')
  const [profileAvatarNonce, setProfileAvatarNonce] = useState(() => Date.now())
  const [savingProfile, setSavingProfile] = useState(false)
  const [savingProfilePrompt, setSavingProfilePrompt] = useState(false)
  const [archiveConfirmOpen, setArchiveConfirmOpen] = useState(false)
  const [archiveConfirmName, setArchiveConfirmName] = useState('')
  const [teams, setTeams] = useState<WorkspaceTeamInfo[]>([])
  const [roles, setRoles] = useState<Array<{ id?: string; name: string; description?: string }>>([])
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    setDisplayName(current.displayName || current.name)
    setDescription(current.description || '')
    setProfilePrompt(current.profilePrompt || '')
    setTeam(current.team || '')
    setRole(current.role || '')
    setStatus(current.status || 'active')
    setModel(current.model || 'codex')
    setRuntimeModel(current.runtimeModel || '')
    setModelAccountId(current.defaultModelAccountId || '')
    setRuntimeNodeId(current.defaultRuntimeNodeId || '')
    setProfileAvatar(current.avatar || '')
  }, [current.id, current.displayName, current.name, current.description, current.profilePrompt, current.team, current.role, current.status, current.model, current.runtimeModel, current.defaultModelAccountId, current.defaultRuntimeNodeId, current.avatar])

  useEffect(() => {
    let cancelled = false
    apiFetch<WorkspaceTeamInfo[]>('/api/v1/teams').then(rows => {
      if (!cancelled) setTeams(rows ?? [])
    }).catch(() => {})
    return () => { cancelled = true }
  }, [])

  useEffect(() => {
    if (!team) {
      setRoles([])
      setRole('')
      return
    }
    let cancelled = false
    apiFetch<WorkspaceTeamDetail>(`/api/v1/teams/${apiTeamPath(team)}`)
      .then(detail => {
        if (cancelled) return
        const next = detail.roles ?? []
        setRoles(next)
        setRole(currentRole => next.some(item => (item.id || item.name) === currentRole) ? currentRole : '')
      })
      .catch(() => {
        if (!cancelled) {
          setRoles([])
          setRole('')
        }
      })
    return () => { cancelled = true }
  }, [team])

  const providers = providersState.status === 'ok' ? (providersState.data ?? []) : []
  const visibleProviders = providers.filter(provider => providerMatchesAgentModel(provider, model))
  const nodes = nodesState.status === 'ok' ? (nodesState.data.nodes ?? []) : []
  const ctx = ctxState.status === 'ok' ? ctxState.data : null
  const avatar = current.avatar || ''
  const modelCls = MODEL_COLORS[model] ?? ''
  const profileAvatarChoices = useMemo(
    () => avatarChoices('', current.name || 'agent', profileAvatarNonce),
    [current.name, profileAvatarNonce],
  )

  function resetProfileDraft() {
    setDisplayName(current.displayName || current.name)
    setDescription(current.description || '')
    setTeam(current.team || '')
    setRole(current.role || '')
    setStatus(current.status || 'active')
    setProfileAvatar(current.avatar || '')
  }

  function toggleProfileEditing() {
    if (editingProfile) {
      resetProfileDraft()
      setEditingProfile(false)
      return
    }
    setEditingProfile(true)
  }

  async function saveProfile() {
    setSavingProfile(true)
    try {
      await apiPatch(`/api/v1/agents/${encodeURIComponent(current.id)}`, {
        displayName: displayName.trim(),
        description: description.trim(),
        avatar: profileAvatar.trim(),
        team,
        role,
      })
      setEditingProfile(false)
      setReloadKey(key => key + 1)
      showToast(t('common.saved'), 'success')
    } finally {
      setSavingProfile(false)
    }
  }

  async function saveProfilePrompt() {
    setSavingProfilePrompt(true)
    try {
      await apiPatch(`/api/v1/agents/${encodeURIComponent(current.id)}`, {
        profilePrompt: profilePrompt.trim(),
      })
      setReloadKey(key => key + 1)
      setCtxReload(key => key + 1)
      showToast(t('common.saved'), 'success')
    } finally {
      setSavingProfilePrompt(false)
    }
  }

  async function archiveAgent() {
    if (archiveConfirmName.trim() !== current.name) return
    setSavingProfile(true)
    try {
      await apiPatch(`/api/v1/agents/${encodeURIComponent(current.id)}`, { status: 'archived' })
      setStatus('archived')
      setArchiveConfirmOpen(false)
      setArchiveConfirmName('')
      setReloadKey(key => key + 1)
      showToast(t('agents.archived', { defaultValue: '智能体已归档' }), 'success')
    } finally {
      setSavingProfile(false)
    }
  }

  async function restoreAgent() {
    setSavingProfile(true)
    try {
      await apiPatch(`/api/v1/agents/${encodeURIComponent(current.id)}`, { status: 'active' })
      setStatus('active')
      setReloadKey(key => key + 1)
      showToast(t('agents.restored', { defaultValue: '智能体已恢复' }), 'success')
    } finally {
      setSavingProfile(false)
    }
  }

  async function save() {
    setSaving(true)
    try {
      await apiPatch(`/api/v1/agents/${encodeURIComponent(current.id)}`, {
        model,
        runtimeModel: runtimeModel.trim(),
        defaultModelAccountId: modelAccountId,
        defaultRuntimeNodeId: runtimeNodeId,
      })
      setReloadKey(key => key + 1)
      showToast(t('common.saved'), 'success')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <div className="shrink-0 px-6 pt-5 pb-4">
        <div className="flex items-center gap-4">
          {avatar ? (
            <img src={avatar} alt="" className="size-12 shrink-0 rounded-xl bg-neutral-100 object-cover dark:bg-zinc-800" />
          ) : (
            <div className="flex size-12 shrink-0 items-center justify-center rounded-xl bg-violet-100 dark:bg-violet-900/30">
              <Bot className="size-6 text-violet-600 dark:text-violet-400" strokeWidth={1.8} />
            </div>
          )}
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="truncate text-xl font-semibold text-neutral-900 dark:text-zinc-100">{current.displayName || current.name}</h1>
              <span className="rounded-full bg-neutral-100 px-2 py-0.5 font-mono text-[11px] text-neutral-500 dark:bg-zinc-800 dark:text-zinc-400">{current.name}</span>
              {canAdminWorkspace && (
                <button type="button" onClick={toggleProfileEditing} className={subtleButtonCls}>
                  {editingProfile ? t('forms.cancel') : t('common.edit')}
                </button>
              )}
            </div>
            <div className="mt-1 flex flex-wrap items-center gap-3">
              <span className={cn('inline-flex items-center rounded-md px-2.5 py-0.5 text-xs font-bold tracking-wide', modelCls)}>{model}</span>
              {team && <span className="text-sm text-neutral-500 dark:text-zinc-500">{team}</span>}
              {role && <span className="text-sm text-neutral-500 dark:text-zinc-500">/ {role}</span>}
            </div>
          </div>
        </div>
        {editingProfile && (
          <div className="mt-4 rounded-lg border border-neutral-200/80 bg-white p-4 dark:border-zinc-700/60 dark:bg-zinc-900/40">
            <div className="grid gap-3 md:grid-cols-2">
              <WorkspaceField label={t('agents.displayName')}>
                <input value={displayName} onChange={e => setDisplayName(e.target.value)} className={workspaceInputCls} />
              </WorkspaceField>
              <WorkspaceField label={t('members.team')}>
                <select value={team} onChange={e => { setTeam(e.target.value); setRole('') }} className={workspaceInputCls}>
                  <option value="">{t('members.selectTeam')}</option>
                  {teams.map(item => <option key={item.path} value={item.path}>{item.name} ({item.path})</option>)}
                </select>
              </WorkspaceField>
              <WorkspaceField label={t('members.role')}>
                <select value={role} onChange={e => setRole(e.target.value)} className={workspaceInputCls} disabled={!team || roles.length === 0}>
                  <option value="">{t('members.noRole')}</option>
                  {roles.map(item => {
                    const value = item.id || item.name
                    return <option key={value} value={value}>{item.name}{item.description ? ` - ${item.description}` : ''}</option>
                  })}
                </select>
              </WorkspaceField>
              <WorkspaceField label={t('agentIdentity.avatarPlaceholder')}>
                <input value={profileAvatar} onChange={e => setProfileAvatar(e.target.value)} className={workspaceInputCls} placeholder="https://..." />
              </WorkspaceField>
              <label className="block md:col-span-2">
                <span className="mb-1 block text-xs font-medium text-neutral-600 dark:text-zinc-400">{t('agents.description')}</span>
                <textarea value={description} onChange={e => setDescription(e.target.value)} rows={3} className={cn(workspaceInputCls, 'h-auto resize-y')} />
              </label>
            </div>
            <div className="mt-3 flex flex-wrap items-center gap-2">
              {profileAvatarChoices.map((url) => (
                <button
                  key={url}
                  type="button"
                  onClick={() => setProfileAvatar(url)}
                  className={cn(
                    'rounded-lg border p-0.5 transition-colors',
                    profileAvatar === url
                      ? 'border-sky-500 bg-sky-50 dark:border-sky-400 dark:bg-sky-900/20'
                      : 'border-neutral-200 hover:border-neutral-300 dark:border-zinc-700 dark:hover:border-zinc-600',
                  )}
                  title={t('agentIdentity.chooseAvatar')}
                >
                  <img src={url} alt="" className="size-8 rounded-md bg-neutral-100 object-cover dark:bg-zinc-800" />
                </button>
              ))}
              <button type="button" onClick={() => setProfileAvatarNonce(Date.now())} className={secondaryButtonCls}>
                {t('agentIdentity.randomAvatars')}
              </button>
              {profileAvatar && (
                <button type="button" onClick={() => setProfileAvatar('')} className={secondaryButtonCls}>
                  {t('agentIdentity.clearAvatar')}
                </button>
              )}
            </div>
            <div className="mt-4 flex justify-end gap-2">
              <button type="button" onClick={() => { resetProfileDraft(); setEditingProfile(false) }} disabled={savingProfile} className={secondaryButtonCls}>
                {t('forms.cancel')}
              </button>
              <button type="button" onClick={() => void saveProfile()} disabled={savingProfile} className={primaryButtonCls}>
                {savingProfile ? t('forms.saving') : t('forms.save')}
              </button>
            </div>
          </div>
        )}
      </div>

      <div className="flex-1 overflow-y-auto px-6 pb-8">
        <div className="space-y-8">
          <section>
            <SectionHeader icon={Settings2} title={t('agentDetail.modelCredentials')} />
            <p className="mt-1 text-sm text-neutral-500 dark:text-zinc-500">{t('agentDetail.modelCredentialsHint')}</p>
            <div className="mt-3 rounded-lg border border-neutral-200/80 bg-white p-4 dark:border-zinc-700/60 dark:bg-zinc-900/40">
              <div className="grid gap-3 md:grid-cols-2">
                <WorkspaceField label={t('agentDetail.cliType')}>
                  <select value={model} onChange={e => { setModel(e.target.value); setModelAccountId('') }} className={workspaceInputCls}>
                    {AGENT_MODELS.map(item => <option key={item} value={item}>{item}</option>)}
                  </select>
                </WorkspaceField>
                <WorkspaceField label={t('agentDetail.modelAccount')}>
                  <select value={modelAccountId} onChange={e => setModelAccountId(e.target.value)} className={workspaceInputCls}>
                    <option value="">{visibleProviders.length > 0 ? t('provider.none') : t('agentDetail.noCredential')}</option>
                    {visibleProviders.map(provider => <option key={provider.id} value={provider.id}>{provider.name} ({provider.type})</option>)}
                  </select>
                </WorkspaceField>
                <WorkspaceField label={t('agentDetail.runtimeModel')}>
                  <input value={runtimeModel} onChange={e => setRuntimeModel(e.target.value)} className={workspaceInputCls} placeholder="gpt-5.5" />
                </WorkspaceField>
                <WorkspaceField label={t('agentDetail.runtimeNode')}>
                  <select value={runtimeNodeId} onChange={e => setRuntimeNodeId(e.target.value)} className={workspaceInputCls}>
                    <option value="">{t('settings.runtimeNodeDefaultLocal')}</option>
                    {nodes.map(node => <option key={node.id} value={node.id}>{node.name || node.id} · {runtimeNodeStatusLabel(t, node.status)}</option>)}
                  </select>
                </WorkspaceField>
              </div>
              {visibleProviders.length === 0 && (
                <Link to="/settings#model-accounts" className={cn(primaryButtonCls, 'mt-3 inline-flex')}>{t('agentDetail.addCredential')}</Link>
              )}
              <div className="mt-4 flex justify-end">
                <button type="button" onClick={() => void save()} disabled={saving} className={primaryButtonCls}>
                  {saving ? t('forms.saving') : t('forms.save')}
                </button>
              </div>
            </div>
          </section>

          <section>
            <SectionHeader icon={Cable} title={t('agentDetail.capabilities')} />
            <p className="mt-1 text-sm text-neutral-500 dark:text-zinc-500">{t('agentDetail.capabilitiesHint')}</p>
            <div className="mt-3 divide-y divide-neutral-100 rounded-lg border border-neutral-200/80 bg-white dark:divide-zinc-800 dark:border-zinc-700/60 dark:bg-zinc-900/40">
              <AgentRuntimeConnectionsPanel project="" agentName={current.name} agentWorkerId={current.id} />
              <AgentSkillsPanel skills={ctx?.skills ?? []} skillDetails={ctx?.skillDetails} />
            </div>
          </section>

          {canAdminWorkspace && <WorkspaceAgentSchedulePanel agentId={current.id} />}

          <section data-tour-agent-wakeup-prompt>
            <SectionHeader icon={BookOpen} title={t('agentDetail.promptContext')} />
            <p className="mt-1 text-sm text-neutral-500 dark:text-zinc-500">{t('agentDetail.promptContextHint')}</p>
            <div className="mt-3 space-y-4">
              <ControlledPromptEditor
                label={t('agentDetail.profilePrompt')}
                icon={UserCog}
                value={profilePrompt}
                onChange={setProfilePrompt}
                onSave={saveProfilePrompt}
                onCancel={() => setProfilePrompt(current.profilePrompt || '')}
                saving={savingProfilePrompt}
                canEdit={canAdminWorkspace}
                emptyText={t('agentDetail.profilePromptEmpty')}
                placeholder={t('agentDetail.profilePromptPlaceholder')}
              />
              <PromptEditor
                label={t('prompt.wakeup')}
                icon={BookOpen}
                apiPath={`/api/v1/agents/${encodeURIComponent(current.id)}/wakeup`}
                initialContent={ctx?.wakeup ?? ''}
                canEdit={canAdminWorkspace}
                emptyText={t('prompt.emptyWakeup')}
              />
              <ContextPanel apiPath={fullContextPath} contextFile={ctx?.contextFile} syncedAt={ctx?.syncedAt} />
              <AgentContextBindingsPanel
                project=""
                agentName={current.name}
                agentWorkerId={current.id}
                canEdit={canAdminWorkspace}
                onChanged={() => setCtxReload((k) => k + 1)}
              />
            </div>
          </section>

          {canAdminWorkspace && (
            <section>
              <SectionHeader icon={Activity} title={t('agentDetail.connectAndChat')} />
              <p className="mt-1 text-sm text-neutral-500 dark:text-zinc-500">{t('agentDetail.connectAndChatHint')}</p>
              <div className="mt-3 space-y-4">
                {chatMembership ? (
                  <SessionPanel
                    project={chatMembership.projectId}
                    agentName={current.name}
                    canConfigure={canAdminWorkspace}
                    canRun={canAdminWorkspace}
                    runBlockedReason={agentModelRequiresModelAccount(ctx?.model) && !ctx?.provider ? t('agentDetail.modelAccountRequiredBeforeRun') : undefined}
                  />
                ) : (
                  <div className="rounded-lg border border-neutral-200/80 bg-neutral-50/50 px-4 py-3 dark:border-zinc-700/60 dark:bg-zinc-900/30">
                    <p className="text-sm text-neutral-500 dark:text-zinc-500">{t('agents.webChatRequiresProject')}</p>
                  </div>
                )}
                <AgentChannelPanel project="" agentName={current.name} agentWorkerId={current.id} />
              </div>
            </section>
          )}

          <AgentForkSessionsPanel agentWorkerId={current.id} canManage={canAdminWorkspace} />

          {canAdminWorkspace && !trustedProxyMode && (
            <details className="group rounded-lg border border-neutral-200/80 bg-white dark:border-zinc-700/60 dark:bg-zinc-900/40">
              <summary className="flex cursor-pointer items-center gap-2 px-4 py-3 text-sm font-medium text-neutral-700 dark:text-zinc-300">
                <ChevronRight className="size-4 transition-transform group-open:rotate-90" strokeWidth={2} />
                <Container className="size-4 text-neutral-400 dark:text-zinc-500" strokeWidth={1.8} />
                {t('agentDetail.advancedRuntime')}
              </summary>
              <div className="border-t border-neutral-100 p-4 dark:border-zinc-800">
                <div className="space-y-4">
                  <RuntimeConcurrencyPanel agent={current} onChanged={() => setReloadKey((k) => k + 1)} />
                  <SandboxEditor
                    project=""
                    agentName={current.name}
                    agentWorkerId={current.id}
                    initial={ctx?.sandbox}
                    onChanged={() => setCtxReload((k) => k + 1)}
                  />
                  <RuntimeNodeBindingPanel
                    project=""
                    agentName={current.name}
                    agentWorkerId={current.id}
                    runtimeNodeId={ctx?.runtimeNodeId ?? current.defaultRuntimeNodeId}
                    onChanged={() => {
                      setCtxReload((k) => k + 1)
                      setReloadKey((k) => k + 1)
                    }}
                  />
                </div>
              </div>
            </details>
          )}

          {canAdminWorkspace && (
            <section className="rounded-lg border border-neutral-200/80 bg-white p-4 dark:border-zinc-700/60 dark:bg-zinc-900/40">
              <SectionHeader icon={Settings2} title={t('agents.dangerZone', { defaultValue: '危险操作' })} />
              <p className="mt-1 text-sm text-neutral-500 dark:text-zinc-500">
                {status === 'archived'
                  ? t('agents.restoreHint', { defaultValue: '这个智能体已归档。恢复后会重新出现在默认智能体列表中。' })
                  : t('agents.archiveHint', { defaultValue: '归档会把这个智能体从默认列表中隐藏，并停止作为日常可用智能体展示。历史记录和配置会保留。' })}
              </p>
              <div className="mt-3 flex justify-end">
                {status === 'archived' ? (
                  <button type="button" onClick={() => void restoreAgent()} disabled={savingProfile} className={secondaryButtonCls}>
                    {t('agents.restore', { defaultValue: '恢复智能体' })}
                  </button>
                ) : (
                  <button type="button" onClick={() => setArchiveConfirmOpen(true)} disabled={savingProfile} className="inline-flex h-8 items-center justify-center rounded-md border border-red-600 bg-red-600 px-3 text-xs font-medium text-white transition-colors hover:bg-red-700 disabled:cursor-not-allowed disabled:opacity-50 dark:border-red-700 dark:bg-red-700 dark:hover:bg-red-600">
                    {t('agents.archive', { defaultValue: '归档' })}
                  </button>
                )}
              </div>
            </section>
          )}
        </div>
      </div>
      {archiveConfirmOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/35 p-4 backdrop-blur-sm">
          <div className="w-full max-w-md rounded-xl border border-neutral-200 bg-white shadow-xl dark:border-zinc-800 dark:bg-zinc-950">
            <div className="border-b border-neutral-100 px-5 py-4 dark:border-zinc-800">
              <h2 className="text-base font-semibold text-neutral-900 dark:text-zinc-100">{t('agents.archiveConfirmTitle', { defaultValue: '归档智能体' })}</h2>
              <p className="mt-1 text-sm text-neutral-500 dark:text-zinc-500">
                {t('agents.archiveConfirm', { name: current.name, defaultValue: `请输入 ${current.name} 确认归档。` })}
              </p>
            </div>
            <div className="px-5 py-4">
              <input
                value={archiveConfirmName}
                onChange={e => setArchiveConfirmName(e.target.value)}
                className={workspaceInputCls}
                autoFocus
                placeholder={current.name}
              />
            </div>
            <div className="flex justify-end gap-2 border-t border-neutral-100 px-5 py-4 dark:border-zinc-800">
              <button type="button" onClick={() => { setArchiveConfirmOpen(false); setArchiveConfirmName('') }} disabled={savingProfile} className={secondaryButtonCls}>
                {t('forms.cancel')}
              </button>
              <button
                type="button"
                onClick={() => void archiveAgent()}
                disabled={savingProfile || archiveConfirmName.trim() !== current.name}
                className="inline-flex h-8 items-center justify-center rounded-md border border-red-200 bg-red-600 px-3 text-xs font-medium text-white transition-colors hover:bg-red-700 disabled:cursor-not-allowed disabled:opacity-50 dark:border-red-900/60 dark:bg-red-700 dark:hover:bg-red-600"
              >
                {savingProfile ? t('forms.saving') : t('agents.archive', { defaultValue: '归档' })}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

type AgentSessionRow = {
  id: string
  title?: string
  purpose?: string
  status?: string
  project?: string
  taskId?: string
  workflowRunId?: string
  lastRunId?: string
  forkMode?: string
  runtimeProvider?: string
  createdAt?: string
  updatedAt?: string
  lastActivityAt?: string
  completedAt?: string
  resultSummary?: string
}

type AgentSessionsResponse = {
  sessions?: AgentSessionRow[]
}

function AgentForkSessionsPanel({ agentWorkerId, canManage }: { agentWorkerId: string; canManage: boolean }) {
  const { t } = useTranslation()
  const formatDateTime = useFormatDateTime()
  const [status, setStatus] = useState('active')
  const [reloadKey, setReloadKey] = useState(0)
  const path = `/api/v1/agent-sessions?agentWorkerId=${encodeURIComponent(agentWorkerId)}&status=${encodeURIComponent(status)}&limit=20`
  const state = useApiJson<AgentSessionsResponse>(path, reloadKey, { keepPreviousDataOnReload: true })
  const sessions = state.status === 'ok' ? (state.data.sessions ?? []) : []

  return (
    <section>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <SectionHeader icon={GitBranch} title={t('agentDetail.forkSessions')} />
          <p className="mt-1 text-sm text-neutral-500 dark:text-zinc-500">{t('agentDetail.forkSessionsHint')}</p>
        </div>
        <div className="flex items-center gap-2">
          <select value={status} onChange={e => setStatus(e.target.value)} className={panelSelectCls}>
            <option value="active">{t('agentDetail.sessionFilterActive')}</option>
            <option value="all">{t('agentDetail.sessionFilterAll')}</option>
            <option value="done">{t('agentDetail.sessionStatus_done')}</option>
            <option value="failed">{t('agentDetail.sessionStatus_failed')}</option>
          </select>
          <button type="button" onClick={() => setReloadKey(key => key + 1)} className={secondaryButtonCls} title={t('common.refresh')}>
            <RefreshCw className="size-3.5" strokeWidth={1.8} />
          </button>
        </div>
      </div>
      <div className="mt-3 overflow-hidden rounded-lg border border-neutral-200/80 bg-white dark:border-zinc-700/60 dark:bg-zinc-900/40">
        {state.status === 'loading' && sessions.length === 0 ? (
          <div className="px-4 py-6 text-sm text-neutral-500 dark:text-zinc-500">{t('common.loading')}</div>
        ) : sessions.length === 0 ? (
          <div className="px-4 py-6 text-sm text-neutral-500 dark:text-zinc-500">{t('agentDetail.noForkSessions')}</div>
        ) : (
          <div className="divide-y divide-neutral-100 dark:divide-zinc-800">
            {sessions.map(session => (
              <div key={session.id} className="px-4 py-3">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="truncate text-sm font-medium text-neutral-900 dark:text-zinc-100">{session.title || session.id}</span>
                      <span className={cn('rounded-full px-2 py-0.5 text-[11px] font-medium', forkSessionStatusClass(session.status))}>
                        {forkSessionStatusLabel(t, session.status)}
                      </span>
                      {session.runtimeProvider && (
                        <span className="rounded-md bg-neutral-100 px-1.5 py-0.5 font-mono text-[11px] text-neutral-500 dark:bg-zinc-800 dark:text-zinc-400">{session.runtimeProvider}</span>
                      )}
                    </div>
                    {session.purpose && <p className="mt-1 line-clamp-2 text-xs text-neutral-500 dark:text-zinc-500">{session.purpose}</p>}
                    <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-neutral-500 dark:text-zinc-500">
                      {session.project && <span>{t('common.project')}: <span className="font-medium text-neutral-700 dark:text-zinc-300">{session.project}</span></span>}
                      {session.taskId && (
                        <span>
                          {t('agentDetail.task')}: {session.project ? (
                            <Link className="font-mono text-sky-600 hover:text-sky-800 dark:text-sky-400" to={`/projects/${encodeURIComponent(session.project)}/tasks/${encodeURIComponent(session.taskId)}/follow`}>
                              {session.taskId}
                            </Link>
                          ) : (
                            <span className="font-mono text-neutral-700 dark:text-zinc-300">{session.taskId}</span>
                          )}
                        </span>
                      )}
                      {session.lastRunId && <span>{t('agentDetail.lastRun')}: <span className="font-mono text-neutral-700 dark:text-zinc-300">{session.lastRunId}</span></span>}
                      <span>{t('agentDetail.lastActive')}: {session.lastActivityAt ? formatDateTime(session.lastActivityAt) : t('agentDetail.notConfigured')}</span>
                    </div>
                    {session.resultSummary && (
                      <p className="mt-2 rounded-md bg-neutral-50 px-3 py-2 text-xs text-neutral-600 dark:bg-zinc-800/60 dark:text-zinc-300">{session.resultSummary}</p>
                    )}
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
      {!canManage && <p className="mt-2 text-xs text-neutral-400 dark:text-zinc-500">{t('agentDetail.forkSessionsReadonly')}</p>}
    </section>
  )
}

function RuntimeConcurrencyPanel({ agent, onChanged }: { agent: WorkspaceAgentSummary; onChanged: () => void }) {
  const { t } = useTranslation()
  const initialMax = Math.max(1, Number(agent.runtimeConfig?.maxForkSessions ?? 5) || 5)
  const [maxForkSessions, setMaxForkSessions] = useState(initialMax)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    setMaxForkSessions(Math.max(1, Number(agent.runtimeConfig?.maxForkSessions ?? 5) || 5))
  }, [agent.id, agent.runtimeConfig?.maxForkSessions])

  async function save() {
    const next = Math.max(1, Math.min(50, Math.floor(maxForkSessions || 5)))
    setSaving(true)
    try {
      await apiPatch(`/api/v1/agents/${encodeURIComponent(agent.id)}`, {
        runtimeConfig: { ...(agent.runtimeConfig ?? {}), maxForkSessions: next },
      })
      setMaxForkSessions(next)
      onChanged()
      showToast(t('common.saved'), 'success')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="rounded-lg border border-neutral-200/80 bg-neutral-50/40 p-3 dark:border-zinc-700/60 dark:bg-zinc-900/40">
      <SubsectionHeader
        title={t('agentDetail.runtimeConcurrency')}
        description={t('agentDetail.runtimeConcurrencyHint')}
      />
      <div className="mt-3 flex flex-wrap items-end gap-3">
        <WorkspaceField label={t('agentDetail.maxForkSessions')}>
          <input
            type="number"
            min={1}
            max={50}
            value={maxForkSessions}
            onChange={e => setMaxForkSessions(Number(e.target.value))}
            className={cn(workspaceInputCls, 'w-32')}
          />
        </WorkspaceField>
        <button type="button" onClick={() => void save()} disabled={saving} className={secondaryButtonCls}>
          {saving ? t('forms.saving') : t('forms.save')}
        </button>
      </div>
    </div>
  )
}

function forkSessionStatusLabel(t: (key: string, options?: Record<string, unknown>) => string, status?: string) {
  const key = (status || 'pending').trim().toLowerCase()
  return t(`agentDetail.sessionStatus_${key}`, { defaultValue: status || 'pending' })
}

function forkSessionStatusClass(status?: string) {
  switch ((status || '').trim().toLowerCase()) {
    case 'running':
      return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
    case 'waiting':
    case 'blocked':
      return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
    case 'done':
      return 'bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300'
    case 'failed':
    case 'cancelled':
      return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
    default:
      return 'bg-neutral-100 text-neutral-600 dark:bg-zinc-800 dark:text-zinc-300'
  }
}

const workspaceInputCls = 'w-full rounded-md border border-neutral-200/80 bg-neutral-50/50 px-3 py-2 text-sm outline-none transition-colors focus:border-sky-400 disabled:opacity-50 dark:border-zinc-700/60 dark:bg-zinc-800/50 dark:text-zinc-200 dark:[color-scheme:dark]'

function WorkspaceField({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block">
      <span className="mb-1 block text-xs font-medium text-neutral-600 dark:text-zinc-400">{label}</span>
      {children}
    </label>
  )
}

const CLI_DEFAULTS: Record<string, { packageManager: string; package: string; binary: string; channel: string; versions: string[] }> = {
  codex: {
    packageManager: 'npm',
    package: '@openai/codex',
    binary: 'codex',
    channel: 'stable',
    versions: ['latest', '0.144.4', '0.144.3', '0.144.2', '0.143.0'],
  },
  'claude-code': {
    packageManager: 'npm',
    package: '@anthropic-ai/claude-code',
    binary: 'claude',
    channel: 'stable',
    versions: ['latest'],
  },
  gemini: {
    packageManager: 'npm',
    package: '@google/gemini-cli',
    binary: 'gemini',
    channel: 'stable',
    versions: ['latest'],
  },
  qoder: {
    packageManager: 'npm',
    package: '@openai/codex',
    binary: 'codex',
    channel: 'stable',
    versions: ['latest', '0.144.4', '0.144.3', '0.144.2', '0.143.0'],
  },
}

function normalizeCliVersion(vendor: string, version: string): string {
  const value = version.trim()
  if (!value) return ''
  if ((vendor === 'codex' || vendor === 'qoder') && ['0.18.0', '0.17.0', '0.16.0'].includes(value)) {
    return 'latest'
  }
  return value
}

function completeAgentCli(input: { vendor: string; version: string }): AgentCLIConfig | null {
  const vendor = input.vendor.trim()
  const version = normalizeCliVersion(vendor, input.version)
  if (!vendor && !version) return null
  const defaults = CLI_DEFAULTS[vendor]
  return {
    vendor,
    version,
    channel: defaults?.channel ?? 'stable',
    packageManager: defaults?.packageManager ?? 'npm',
    package: defaults?.package ?? '',
    binary: defaults?.binary ?? vendor,
  }
}

function SandboxEditor({ project, agentName, agentWorkerId, initial, onChanged }: {
  project: string; agentName: string; agentWorkerId?: string; initial?: SandboxConfig; onChanged: () => void
}) {
  const { t } = useTranslation()
  const capsState = useApiJson<SandboxCapabilities>('/api/v1/sandbox/capabilities', 0)
  const caps = capsState.status === 'ok' ? capsState.data : null
  const initialProvider = initial ? (initial.provider || 'none') : 'docker'
  const [provider, setProvider] = useState(initialProvider)
  const [image, setImage] = useState(initial?.image ?? initial?.docker?.image ?? '')
  const [template, setTemplate] = useState(initial?.e2b?.template ?? '')
  const [network, setNetwork] = useState(initial?.networkMode ?? initial?.docker?.network_mode ?? '')
  const [memoryMb, setMemoryMb] = useState(initial?.resources?.memoryMb ?? initial?.docker?.memory_mb ?? 0)
  const [cpus, setCpus] = useState(initial?.resources?.cpus ?? initial?.docker?.cpus ?? 0)
  const [timeoutSec, setTimeoutSec] = useState(initial?.resources?.timeoutSec ?? initial?.e2b?.timeoutSec ?? 0)
  const [cliVendor, setCliVendor] = useState(initial?.agentCli?.vendor ?? '')
  const [cliVersion, setCliVersion] = useState(normalizeCliVersion(initial?.agentCli?.vendor ?? '', initial?.agentCli?.version ?? ''))
  const [customCliVersion, setCustomCliVersion] = useState(false)
  const [saving, setSaving] = useState(false)
  const [dirty, setDirty] = useState(false)

  useEffect(() => {
    setProvider(initial ? (initial.provider || 'none') : 'docker')
    setImage(initial?.image ?? initial?.docker?.image ?? '')
    setTemplate(initial?.e2b?.template ?? '')
    setNetwork(initial?.networkMode ?? initial?.docker?.network_mode ?? '')
    setMemoryMb(initial?.resources?.memoryMb ?? initial?.docker?.memory_mb ?? 0)
    setCpus(initial?.resources?.cpus ?? initial?.docker?.cpus ?? 0)
    setTimeoutSec(initial?.resources?.timeoutSec ?? initial?.e2b?.timeoutSec ?? 0)
    const nextVendor = initial?.agentCli?.vendor ?? ''
    const nextVersion = normalizeCliVersion(nextVendor, initial?.agentCli?.version ?? '')
    setCliVendor(nextVendor)
    setCliVersion(nextVersion)
    setCustomCliVersion(Boolean(nextVersion && !(CLI_DEFAULTS[nextVendor]?.versions ?? ['latest']).includes(nextVersion)))
    setDirty(false)
  }, [initial])

  const inputCls = 'w-full rounded-md border border-neutral-200/80 bg-white px-3 py-1.5 text-sm text-neutral-800 outline-none transition-colors focus:border-sky-400 focus:ring-1 focus:ring-sky-400/30 dark:border-zinc-700/60 dark:bg-zinc-800/50 dark:text-zinc-200 dark:focus:border-sky-500'
  const labelCls = 'block text-xs font-medium text-neutral-500 dark:text-zinc-400 mb-1'

  async function save() {
    const agentCli = completeAgentCli({ vendor: cliVendor, version: cliVersion })
    setSaving(true)
    try {
      const path = agentWorkerId
        ? `/api/v1/agents/${encodeURIComponent(agentWorkerId)}/sandbox`
        : `/api/v1/projects/${encodeURIComponent(project)}/agents/${encodeURIComponent(agentName)}/sandbox`
      await apiPut(path, {
        provider: provider || 'none',
        image,
        template,
        network,
        memoryMb,
        cpus,
        timeoutSec,
        agentCli,
        addDirs: [],
      })
      setDirty(false)
      onChanged()
    } finally { setSaving(false) }
  }

  function versionOptions(vendor: string): string[] {
    return CLI_DEFAULTS[vendor]?.versions ?? ['latest']
  }

  function handleVendorChange(vendor: string) {
    setCliVendor(vendor)
    const options = versionOptions(vendor)
    const nextVersion = normalizeCliVersion(vendor, cliVersion)
    if (!nextVersion || !options.includes(nextVersion)) {
      setCliVersion(options[0] ?? 'latest')
      setCustomCliVersion(false)
    } else {
      setCliVersion(nextVersion)
      setCustomCliVersion(false)
    }
    setDirty(true)
  }

  return (
    <div className="rounded-lg border border-neutral-200/80 bg-white p-4 dark:border-zinc-700/60 dark:bg-zinc-900/40">
      <div className="mb-3 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Container className="size-4 text-neutral-400 dark:text-zinc-500" strokeWidth={1.8} />
          <span className="text-sm font-medium text-neutral-700 dark:text-zinc-300">{t('sandbox.title')}</span>
        </div>
        {dirty && (
          <button type="button" onClick={() => void save()} disabled={saving}
            className={primaryButtonCls}>
            {saving ? t('common.save') + '...' : t('common.save')}
          </button>
        )}
      </div>
      <div className="space-y-3">
        <div>
          <label className={labelCls}>{t('sandbox.provider')}</label>
          <select value={provider} onChange={(e) => { setProvider(e.target.value); setDirty(true) }} className={inputCls}>
            <option value="none" disabled={caps?.directHost?.available === false && provider !== 'none'}>{t('sandbox.providerDirect')}</option>
            <option value="docker">Docker</option>
            <option value="e2b" disabled={!caps?.e2b?.available && provider !== 'e2b'}>E2B</option>
          </select>
          {provider === 'none' && (
            <p className="mt-1.5 text-[11px] leading-relaxed text-amber-600 dark:text-amber-400">
              {t('sandbox.directHint')}
            </p>
          )}
          {caps?.directHost && !caps.directHost.available && (
            <p className="mt-1.5 text-[11px] leading-relaxed text-amber-600 dark:text-amber-400">
              {t('sandbox.directDisabled')}: {caps.directHost.reason}
            </p>
          )}
          {caps?.e2b && !caps.e2b.available && (
            <p className="mt-1.5 text-[11px] leading-relaxed text-amber-600 dark:text-amber-400">
              {t('sandbox.e2bUnavailable')}: {caps.e2b.reason}
            </p>
          )}
        </div>
        {provider && provider !== 'none' && (
          <>
            <div>
              <label className={labelCls}>{provider === 'e2b' ? t('sandbox.template') : t('sandbox.image')}</label>
              <input type="text" value={image} onChange={(e) => { setImage(e.target.value); setDirty(true) }} placeholder={t('sandbox.imagePlaceholder')} className={inputCls} />
            </div>
            <div className="rounded-lg border border-neutral-200/70 bg-neutral-50/60 p-3 dark:border-zinc-700/50 dark:bg-zinc-800/25">
              <div className="mb-2 flex items-center justify-between">
                <span className="text-xs font-semibold uppercase tracking-wider text-neutral-500 dark:text-zinc-400">{t('sandbox.agentCli')}</span>
                <span className="text-[11px] text-neutral-400 dark:text-zinc-500">{t('sandbox.agentCliHint')}</span>
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className={labelCls}>{t('sandbox.cliVendor')}</label>
                  <select value={cliVendor} onChange={(e) => handleVendorChange(e.target.value)} className={inputCls}>
                    <option value="">{t('sandbox.cliAuto')}</option>
                    <option value="codex">codex</option>
                    <option value="claude-code">claude-code</option>
                    <option value="gemini">gemini</option>
                    <option value="cursor">cursor</option>
                    <option value="opencode">opencode</option>
                    <option value="qoder">qoder</option>
                    <option value="custom">custom</option>
                  </select>
                </div>
                <div>
                  <label className={labelCls}>{t('sandbox.cliVersion')}</label>
                  {!customCliVersion ? (
                    <select
                      value={versionOptions(cliVendor).includes(cliVersion || 'latest') ? (cliVersion || 'latest') : '__custom__'}
                      onChange={(e) => {
                        if (e.target.value === '__custom__') {
                          setCustomCliVersion(true)
                        } else {
                          setCliVersion(e.target.value)
                          setCustomCliVersion(false)
                        }
                        setDirty(true)
                      }}
                      className={inputCls}
                    >
                      {versionOptions(cliVendor).map((version) => (
                        <option key={version} value={version}>{version}</option>
                      ))}
                      <option value="__custom__">{t('sandbox.cliVersionCustom')}</option>
                    </select>
                  ) : (
                    <div className="flex gap-2">
                      <input
                        type="text"
                        value={cliVersion}
                        onChange={(e) => { setCliVersion(e.target.value); setDirty(true) }}
                        placeholder="0.144.4"
                        className={inputCls}
                      />
                      <button
                        type="button"
                        onClick={() => { setCustomCliVersion(false); setCliVersion(versionOptions(cliVendor)[0] ?? 'latest'); setDirty(true) }}
                        className={secondaryButtonCls}
                      >
                        {t('common.cancel')}
                      </button>
                    </div>
                  )}
                </div>
              </div>
            </div>
            {provider === 'e2b' && (
              <div>
                <label className={labelCls}>{t('sandbox.e2bTemplate')}</label>
                <input type="text" value={template} onChange={(e) => { setTemplate(e.target.value); setDirty(true) }} placeholder="multigent-codex" className={inputCls} />
              </div>
            )}
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className={labelCls}>{t('sandbox.network')}</label>
                <select value={network} onChange={(e) => { setNetwork(e.target.value); setDirty(true) }} className={inputCls}>
                  <option value="">bridge ({t('sandbox.networkDefault')})</option>
                  <option value="host">host</option>
                  <option value="none">none ({t('sandbox.networkOffline')})</option>
                </select>
              </div>
              <div>
                <label className={labelCls}>{t('sandbox.memory')}</label>
                <input type="number" value={memoryMb || ''} onChange={(e) => { setMemoryMb(Number(e.target.value)); setDirty(true) }} placeholder={t('sandbox.memoryPlaceholder')} className={inputCls} />
              </div>
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className={labelCls}>{t('sandbox.cpus')}</label>
                <input type="number" step="0.25" value={cpus || ''} onChange={(e) => { setCpus(Number(e.target.value)); setDirty(true) }} placeholder="0 = default" className={inputCls} />
              </div>
              <div>
                <label className={labelCls}>{t('sandbox.timeout')}</label>
                <input type="number" value={timeoutSec || ''} onChange={(e) => { setTimeoutSec(Number(e.target.value)); setDirty(true) }} placeholder="0 = default" className={inputCls} />
              </div>
            </div>
          </>
        )}
      </div>
    </div>
  )
}

function SectionHeader({ icon: Icon, title, action }: { icon: LucideIcon; title: string; action?: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-3">
      <div className="flex min-w-0 items-center gap-2.5">
        <div className="flex size-7 shrink-0 items-center justify-center rounded-lg bg-neutral-100 dark:bg-zinc-800">
          <Icon className="size-3.5 text-neutral-500 dark:text-zinc-400" strokeWidth={1.8} />
        </div>
        <h3 className="truncate text-sm font-semibold text-neutral-800 dark:text-zinc-200">{title}</h3>
      </div>
      {action}
    </div>
  )
}

function SubsectionHeader({ title, description, action }: { title: string; description?: string; action?: React.ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-3">
      <div className="min-w-0">
        <h4 className="text-xs font-semibold uppercase tracking-wider text-neutral-500 dark:text-zinc-400">{title}</h4>
        {description && <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">{description}</p>}
      </div>
      {action}
    </div>
  )
}

type ProviderOption = {
  id: string
  ownerType?: 'workspace' | 'user'
  name: string
  type: string
  baseUrl?: string
  model?: string
  models?: string[]
  authMethod?: string
}

function providerTypesForAgentModel(model: string) {
  switch (model.trim().toLowerCase()) {
    case 'claudecode':
      return ['anthropic']
    case 'codex':
      return ['openai']
    case 'cursor':
      return ['cursor']
    case 'gemini':
      return ['gemini']
    default:
      return []
  }
}

function providerMatchesAgentModel(provider: ProviderOption, model: string) {
  const allowed = providerTypesForAgentModel(model)
  if (allowed.length === 0) return false
  return allowed.includes(provider.type.trim().toLowerCase())
}

function runtimeNodeStatusLabel(t: (key: string, options?: Record<string, unknown>) => string, status?: string) {
  return t(`settings.runtimeNodeStatus_${status || 'pending'}`, { defaultValue: status || 'pending' })
}

function RuntimeNodeBindingPanel({ project, agentName, agentWorkerId, runtimeNodeId, onChanged }: {
  project: string; agentName: string; agentWorkerId?: string; runtimeNodeId?: string; onChanged: () => void
}) {
  const { t } = useTranslation()
  const [editing, setEditing] = useState(false)
  const [nodes, setNodes] = useState<RuntimeNode[]>([])
  const [selected, setSelected] = useState(runtimeNodeId ?? '')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [err, setErr] = useState('')

  useEffect(() => {
    setSelected(runtimeNodeId ?? '')
  }, [runtimeNodeId])

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setErr('')
    apiFetch<RuntimeNodeListResp>('/api/v1/runtime-nodes')
      .then(data => {
        if (!cancelled) setNodes(data.nodes ?? [])
      })
      .catch(e => {
        if (!cancelled) setErr(e instanceof Error ? e.message : String(e))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => { cancelled = true }
  }, [])

  const currentNode = nodes.find(node => node.id === runtimeNodeId)
  const currentValue = currentNode ? currentNode.name || currentNode.id : runtimeNodeId ? t('agentDetail.runtimeNodeUnavailable') : t('agentDetail.runtimeNodeLocal')
  const currentDetail = currentNode
    ? [runtimeNodeStatusLabel(t, currentNode.status), currentNode.hostname, [currentNode.os, currentNode.arch].filter(Boolean).join('/')].filter(Boolean).join(' · ')
    : runtimeNodeId
      ? runtimeNodeId
      : t('agentDetail.runtimeNodeLocalHint')

  async function save() {
    setSaving(true)
    setErr('')
    try {
      if (agentWorkerId) {
        await apiPatch(`/api/v1/agents/${encodeURIComponent(agentWorkerId)}`, {
          defaultRuntimeNodeId: selected,
        })
      } else {
        await apiPatch(`/api/v1/projects/${encodeURIComponent(project)}/agents/${encodeURIComponent(agentName)}`, {
          runtimeNodeId: selected,
        })
      }
      setEditing(false)
      onChanged()
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="mt-3 rounded-lg border border-neutral-200/80 bg-white dark:border-zinc-700/60 dark:bg-zinc-900/40">
      <div className="flex items-start justify-between gap-4 px-4 py-3">
        <div className="flex min-w-0 items-start gap-3">
          <div className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-lg bg-neutral-100 dark:bg-zinc-800">
            <Server className="size-3.5 text-neutral-500 dark:text-zinc-400" strokeWidth={1.8} />
          </div>
          <div className="min-w-0">
            <p className="text-xs font-medium text-neutral-400 dark:text-zinc-500">{t('agentDetail.runtimeNode')}</p>
            <p className="mt-1 truncate text-sm font-medium text-neutral-800 dark:text-zinc-200" title={currentValue}>{currentValue}</p>
            <p className="mt-0.5 truncate text-xs text-neutral-400 dark:text-zinc-500" title={currentDetail}>{currentDetail}</p>
          </div>
        </div>
        <button type="button" onClick={() => setEditing((v) => !v)} className={secondaryButtonCls}>
          {editing ? t('common.done') : t('common.edit')}
        </button>
      </div>
      {editing && (
        <div className="space-y-3 border-t border-neutral-100 p-4 dark:border-zinc-800">
          <label className="block">
            <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">{t('agentDetail.runtimeNodeSelect')}</span>
            <select
              value={selected}
              onChange={e => setSelected(e.target.value)}
              className={cn(panelSelectCls, 'mt-1 w-full')}
              disabled={loading || saving}
            >
              <option value="">{t('agentDetail.runtimeNodeLocal')}</option>
              {nodes.map(node => (
                <option key={node.id} value={node.id}>
                  {(node.name || node.id) + ` · ${runtimeNodeStatusLabel(t, node.status)}`}
                </option>
              ))}
            </select>
          </label>
          <p className="text-xs text-neutral-400 dark:text-zinc-500">{t('agentDetail.runtimeNodeBindingHint')}</p>
          {err && <p className="text-xs text-red-600 dark:text-red-400">{err}</p>}
          <div className="flex justify-end gap-2">
            <button type="button" onClick={() => { setSelected(runtimeNodeId ?? ''); setEditing(false) }} disabled={saving} className={secondaryButtonCls}>
              {t('common.cancel')}
            </button>
            <button type="button" onClick={() => void save()} disabled={saving || selected === (runtimeNodeId ?? '')} className={primaryButtonCls}>
              {saving ? t('common.saving') : t('common.save')}
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

function ModelCredentialsPanel({ project, agentName, ctx, onChanged }: {
  project: string; agentName: string; ctx: AgentContext; onChanged: () => void
}) {
  const { t } = useTranslation()
  const [editing, setEditing] = useState(false)
  const [providerOptions, setProviderOptions] = useState<ProviderOption[]>([])
  const selectedProviderInfo = providerOptions.find((p) => p.id === ctx.provider && providerMatchesAgentModel(p, ctx.model))
  const providerValue = selectedProviderInfo ? selectedProviderInfo.name : t('agentDetail.noCredential')
  const runtimeModel = ctx.runtimeModel || selectedProviderInfo?.model || t('agentDetail.notConfigured')

  useEffect(() => {
    const path = `/api/v1/providers?project=${encodeURIComponent(project)}&agent=${encodeURIComponent(agentName)}`
    void apiFetch<ProviderOption[]>(path).then(data => setProviderOptions(data ?? [])).catch(() => {})
  }, [project, agentName])

  return (
    <div className="mt-3 rounded-lg border border-neutral-200/80 bg-white dark:border-zinc-700/60 dark:bg-zinc-900/40">
      <div className="flex items-start justify-between gap-4 px-4 py-3">
        <div className="grid min-w-0 flex-1 gap-x-8 gap-y-3 sm:grid-cols-3">
          <ReadOnlyField label={t('agentDetail.cliType')} value={ctx.model} valueClassName="font-mono" />
          <ReadOnlyField
            label={t('agentDetail.modelAccount')}
            value={providerValue}
            detail={selectedProviderInfo ? `${selectedProviderInfo.type} · ${selectedProviderInfo.ownerType === 'user' ? t('provider.scopePersonal') : t('provider.scopeWorkspace')}` : undefined}
          />
          <ReadOnlyField label={t('agentDetail.runtimeModel')} value={runtimeModel} valueClassName="font-mono" />
        </div>
        <button type="button" onClick={() => setEditing((v) => !v)} className={secondaryButtonCls}>
          {editing ? t('common.done') : t('common.edit')}
        </button>
      </div>
      {editing && (
        <div className="space-y-4 border-t border-neutral-100 p-4 dark:border-zinc-800">
          <ModelSelector
            project={project}
            agentName={agentName}
            currentModel={ctx.model}
            currentHttpAgent={ctx.httpAgent}
            onChanged={onChanged}
          />
          <EnvEditor
            project={project}
            agentName={agentName}
            model={ctx.model}
            initialRuntimeModel={ctx.runtimeModel}
            initialEnv={ctx.env ?? {}}
            initialProvider={ctx.provider}
            onChanged={onChanged}
          />
        </div>
      )}
    </div>
  )
}

function ReadOnlyField({ label, value, detail, valueClassName }: { label: string; value: string; detail?: string; valueClassName?: string }) {
  return (
    <div className="min-w-0">
      <p className="text-xs font-medium text-neutral-400 dark:text-zinc-500">{label}</p>
      <p className={cn('mt-1 truncate text-sm font-medium text-neutral-800 dark:text-zinc-200', valueClassName)} title={value}>{value}</p>
      {detail && <p className="mt-0.5 truncate text-xs text-neutral-400 dark:text-zinc-500" title={detail}>{detail}</p>}
    </div>
  )
}

function uniqueStrings(items: Array<string | undefined>) {
  const out: string[] = []
  for (const item of items) {
    const value = item?.trim()
    if (!value || out.includes(value)) continue
    out.push(value)
  }
  return out
}

function detectProviderFamily(provider?: ProviderOption) {
  const haystack = [
    provider?.name,
    provider?.baseUrl,
    provider?.model,
    provider?.id,
  ].join(' ').toLowerCase()
  if (!haystack.trim()) return ''
  if (haystack.includes('minimax')) return 'minimax'
  if (haystack.includes('moonshot') || haystack.includes('kimi')) return 'kimi'
  if (haystack.includes('deepseek')) return 'deepseek'
  if (haystack.includes('z.ai') || haystack.includes('zhipu') || haystack.includes('bigmodel') || haystack.includes('glm')) return 'glm'
  if (haystack.includes('xiaomi') || haystack.includes('mimo')) return 'xiaomi'
  return ''
}

function runtimeModelOptionsFor(model: string, provider?: ProviderOption, catalog?: ModelCatalog | null) {
  const normalizedModel = model.trim().toLowerCase()
  const providerType = provider?.type?.trim().toLowerCase() ?? ''
  const isGateway = Boolean(provider?.baseUrl?.trim())
  const providerFamily = detectProviderFamily(provider)
  const familyModels = providerFamily ? (catalog?.modelsByProvider?.[providerFamily] ?? []) : []
  const base = isGateway
    ? familyModels
    : [
        ...(catalog?.modelsByCLI?.[normalizedModel] ?? []),
        ...(catalog?.modelsByProviderType?.[providerType] ?? []),
        ...familyModels,
        ...(OFFICIAL_RUNTIME_MODEL_PRESETS[normalizedModel] ?? []),
        ...(OFFICIAL_RUNTIME_MODEL_PRESETS[providerType] ?? []),
      ]
  return uniqueStrings([
    provider?.model,
    ...(provider?.models ?? []),
    ...base,
    ...(isGateway ? [] : (RUNTIME_MODEL_PRESETS[normalizedModel] ?? [])),
  ])
}

function ContextPanel({ apiPath, contextFile, syncedAt }: { apiPath: string | null; contextFile?: string; syncedAt?: string | null }) {
  const { t } = useTranslation()
  const fmt = useFormatDateTime()
  const [context, setContext] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const loadContext = useCallback(() => {
    if (!apiPath || loading || context !== null) return
    setLoading(true)
    setError(null)
    apiFetch<AgentContext>(apiPath)
      .then((data) => setContext(data.context ?? ''))
      .catch((e) => setError(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false))
  }, [apiPath, context, loading])
  return (
    <details
      className="group mt-3 rounded-lg border border-neutral-200/80 bg-white dark:border-zinc-700/60 dark:bg-zinc-900/40"
      onToggle={(e) => {
        if ((e.currentTarget as HTMLDetailsElement).open) loadContext()
      }}
    >
      <summary className="flex cursor-pointer items-center justify-between gap-3 px-4 py-3">
        <div className="flex min-w-0 items-center gap-2 text-sm font-medium text-neutral-700 dark:text-zinc-300">
          <ChevronRight className="size-4 shrink-0 transition-transform group-open:rotate-90" strokeWidth={2} />
          <span>{t('agentDetail.fullContext')}</span>
          {contextFile && <span className="truncate font-mono text-xs font-normal text-neutral-400 dark:text-zinc-500">{contextFile}</span>}
        </div>
        {syncedAt && <span className="shrink-0 text-xs text-neutral-400 dark:text-zinc-500">{fmt(syncedAt)}</span>}
      </summary>
      <div className="border-t border-neutral-100 bg-neutral-50/60 p-4 dark:border-zinc-800 dark:bg-zinc-950/40">
        {loading && (
          <div className="flex items-center gap-2 py-4 text-sm text-neutral-500 dark:text-zinc-400">
            <div className="size-4 animate-spin rounded-full border-2 border-neutral-300 border-t-sky-600 dark:border-zinc-600 dark:border-t-sky-400" />
            <span>{t('api.loading')}</span>
          </div>
        )}
        {error && <p className="py-3 text-sm text-red-500">{error}</p>}
        {!loading && !error && context !== null && (
          <div className="prose prose-sm prose-neutral dark:prose-invert max-w-none">
            <Markdown remarkPlugins={[remarkGfm]}>{context || t('agentDetail.contextEmpty')}</Markdown>
          </div>
        )}
      </div>
    </details>
  )
}

function AgentContextBindingsPanel({ project, agentName, agentWorkerId, canEdit, onChanged }: {
  project: string
  agentName: string
  agentWorkerId?: string
  canEdit: boolean
  onChanged: () => void
}) {
  const { t } = useTranslation()
  const [reloadKey, setReloadKey] = useState(0)
  const [docs, setDocs] = useState<KnowledgeDoc[]>([])
  const [docId, setDocId] = useState('')
  const [required, setRequired] = useState(true)
  const [saving, setSaving] = useState(false)
  const agentScopeId = agentWorkerId ? `worker:${agentWorkerId}` : `${project}/${agentName}`
  const path = agentWorkerId
    ? `/api/v1/agents/${encodeURIComponent(agentWorkerId)}/context-bindings`
    : `/api/v1/projects/${encodeURIComponent(project)}/agents/${encodeURIComponent(agentName)}/context-bindings`
  const state = useApiJson<ContextBindingsResponse>(path, reloadKey, { keepPreviousDataOnReload: true })

  useEffect(() => {
    if (!canEdit) return
    apiFetch<KnowledgeDoc[]>('/api/v1/docs')
      .then((data) => setDocs(data ?? []))
      .catch(() => setDocs([]))
  }, [canEdit])

  async function addBinding() {
    if (!docId.trim()) return
    setSaving(true)
    try {
      await apiPost('/api/v1/context/bindings', {
        docId: docId.trim(),
        scopeType: 'agent',
        scopeId: agentScopeId,
        required,
      })
      setDocId('')
      setReloadKey((k) => k + 1)
      onChanged()
    } finally {
      setSaving(false)
    }
  }

  async function removeBinding(id: string) {
    await apiDelete(`/api/v1/context/bindings/${encodeURIComponent(id)}`)
    setReloadKey((k) => k + 1)
    onChanged()
  }

  const bindings: ContextBindingView[] = state.status === 'ok' ? (state.data.bindings ?? []) : []
  const directlyBoundDocIds = new Set(
    bindings
      .filter((item) => item.binding.scopeType === 'agent' && item.binding.scopeId === agentScopeId)
      .map((item) => item.artifact.docId || item.binding.docId || item.doc?.id)
      .filter(Boolean) as string[],
  )
  const availableDocs = docs.filter((doc) => !directlyBoundDocIds.has(doc.id))
  const scopeLabel = (binding: ContextBindingView['binding']) => {
    if (binding.scopeType === 'workspace') return t('agentDetail.scopeWorkspace', { defaultValue: 'Workspace' })
    if (binding.scopeType === 'project') return t('agentDetail.scopeProject', { defaultValue: 'Project' })
    return t('agentDetail.scopeAgent', { defaultValue: 'This agent' })
  }
  return (
    <div className="rounded-lg border border-neutral-200/80 bg-white dark:border-zinc-700/60 dark:bg-zinc-900/40">
      <div className="border-b border-neutral-100 px-4 py-3 dark:border-zinc-800">
        <div className="flex items-center justify-between gap-3">
          <div>
            <p className="text-sm font-medium text-neutral-800 dark:text-zinc-100">
              {t('agentDetail.linkedContext', { defaultValue: 'Reference material' })}
            </p>
            <p className="mt-1 text-xs text-neutral-500 dark:text-zinc-500">
              {t('agentDetail.linkedContextHint', { defaultValue: 'Link knowledge docs to this agent. Runs receive a reference list, and the agent can read the content with mga context when needed.' })}
            </p>
          </div>
          {state.status === 'loading' && <div className="size-4 animate-spin rounded-full border-2 border-neutral-300 border-t-sky-600 dark:border-zinc-600 dark:border-t-sky-400" />}
        </div>
      </div>
      <div className="space-y-3 p-4">
        {bindings.length === 0 && (
          <p className="rounded-md bg-neutral-50 px-3 py-2 text-sm text-neutral-500 dark:bg-zinc-950/40 dark:text-zinc-500">
            {t('agentDetail.noLinkedContext', { defaultValue: 'No reference material linked yet.' })}
          </p>
        )}
        {bindings.map((item) => {
          const doc = item.doc
          const title = item.artifact.title || doc?.title || item.artifact.docId || item.binding.docId || item.binding.id
          const docTarget = item.artifact.docId || item.binding.docId || doc?.id
          return (
            <div key={item.binding.id} className="flex items-start justify-between gap-3 rounded-md border border-neutral-100 bg-neutral-50/70 p-3 dark:border-zinc-800 dark:bg-zinc-950/30">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  {docTarget ? (
                    <Link to={`/docs/${encodeURIComponent(docTarget)}`} className="truncate text-sm font-medium text-neutral-800 hover:text-sky-600 dark:text-zinc-100 dark:hover:text-sky-300">
                      {title}
                    </Link>
                  ) : (
                    <p className="truncate text-sm font-medium text-neutral-800 dark:text-zinc-100">{title}</p>
                  )}
                  {item.binding.required && (
                    <span className="rounded-full bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-700 dark:bg-amber-900/30 dark:text-amber-300">
                      {t('agentDetail.requiredContext', { defaultValue: 'Required' })}
                    </span>
                  )}
                  <span className="rounded-full bg-neutral-100 px-2 py-0.5 text-xs font-medium text-neutral-500 dark:bg-zinc-800 dark:text-zinc-400">
                    {scopeLabel(item.binding)}
                  </span>
                </div>
                {(item.artifact.summary || doc?.description) && (
                  <p className="mt-1 line-clamp-2 text-xs text-neutral-500 dark:text-zinc-500">
                    {item.artifact.summary || doc?.description}
                  </p>
                )}
              </div>
              {canEdit && item.binding.scopeType === 'agent' && item.binding.scopeId === agentScopeId && (
                <button type="button" onClick={() => void removeBinding(item.binding.id)} className="shrink-0 text-xs font-medium text-neutral-400 hover:text-red-500">
                  {t('common.delete')}
                </button>
              )}
            </div>
          )
        })}
        {canEdit && (
          <div className="grid gap-2 border-t border-neutral-100 pt-3 dark:border-zinc-800 sm:grid-cols-[1fr_auto_auto]">
            <select
              value={docId}
              onChange={(e) => setDocId(e.target.value)}
              className="min-w-0 rounded-md border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-800 outline-none focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-100"
            >
              <option value="">{t('agentDetail.selectContextDoc', { defaultValue: 'Select a knowledge doc...' })}</option>
              {availableDocs.map((doc) => (
                <option key={doc.id} value={doc.id}>{doc.title || doc.id} · {doc.id}</option>
              ))}
            </select>
            <label className="inline-flex items-center gap-2 rounded-md border border-neutral-200 px-3 py-2 text-sm text-neutral-600 dark:border-zinc-700 dark:text-zinc-300">
              <input type="checkbox" checked={required} onChange={(e) => setRequired(e.target.checked)} />
              {t('agentDetail.requiredContext', { defaultValue: 'Required' })}
            </label>
            <button type="button" onClick={() => void addBinding()} disabled={!docId || saving} className={primaryButtonCls}>
              {saving ? t('common.saving') : t('common.add')}
            </button>
          </div>
        )}
      </div>
    </div>
  )
}

type RuntimeToolBinding = {
  id: string
  connectionId: string
  provider: string
  adapterType?: string
  status: string
}
type AgentToolBindingsResponse = { bindings?: RuntimeToolBinding[] }
type ConnectionOption = {
  id: string
  provider: string
  connectionName: string
  ownerType: string
  ownerId: string
  authType: string
  status?: string
  profile?: Record<string, unknown>
  profileSummary?: { displayName?: string; accountName?: string; accountEmail?: string }
}
type ConnectionsResponse = { connections?: ConnectionOption[] }

function AgentRuntimeConnectionsPanel({ project, agentName, agentWorkerId }: { project: string; agentName: string; agentWorkerId?: string }) {
  const { t } = useTranslation()
  const [reloadKey, setReloadKey] = useState(0)
  const [connections, setConnections] = useState<ConnectionOption[]>([])
  const [connectionId, setConnectionId] = useState('')
  const [saving, setSaving] = useState(false)
  const bindingsPath = agentWorkerId
    ? `/api/v1/agents/${encodeURIComponent(agentWorkerId)}/tool-bindings`
    : `/api/v1/projects/${encodeURIComponent(project)}/agents/${encodeURIComponent(agentName)}/tool-bindings`
  const bindingsState = useApiJson<AgentToolBindingsResponse>(bindingsPath, reloadKey)

  useEffect(() => {
    let cancelled = false
    apiFetch<ConnectionsResponse>('/api/v1/connections')
      .then((res) => {
        if (!cancelled) setConnections((res.connections ?? []).filter((connection) => connection.status !== 'revoked'))
      })
      .catch(() => {
        if (!cancelled) setConnections([])
      })
    return () => { cancelled = true }
  }, [reloadKey])

  const boundConnectionIds = new Set((bindingsState.status === 'ok' ? bindingsState.data.bindings ?? [] : []).map(binding => binding.connectionId))
  const availableConnections = connections.filter(connection => !boundConnectionIds.has(connection.id))
  const enabledToolCount = bindingsState.status === 'ok' ? (bindingsState.data.bindings ?? []).length : 0

  async function enableBinding() {
    if (!connectionId) return
    setSaving(true)
    try {
      await apiPost(bindingsPath, {
        connectionId,
        status: 'enabled',
      })
      setConnectionId('')
      setReloadKey(k => k + 1)
    } finally {
      setSaving(false)
    }
  }

  async function removeBinding(binding: RuntimeToolBinding) {
    setSaving(true)
    try {
      await apiDelete(`${bindingsPath}/${encodeURIComponent(binding.id)}`)
      setReloadKey(k => k + 1)
    } finally {
      setSaving(false)
    }
  }

  return (
    <section className="p-4">
      <SubsectionHeader
        title={t('agentDetail.externalTools')}
        description={t('members.runtimeConnectionsHelp')}
        action={(
          <button
            type="button"
            onClick={() => setReloadKey(k => k + 1)}
            className={secondaryButtonCls}
          >
            {t('common.refresh')}
          </button>
        )}
      />
      <div className="mt-3">
        <div className="mb-4 rounded-lg border border-neutral-200/70 bg-neutral-50/60 p-3 dark:border-zinc-700/60 dark:bg-zinc-800/30">
          <div className="grid gap-2 md:grid-cols-[minmax(0,1fr)_auto]">
            <select
              value={connectionId}
              onChange={(event) => setConnectionId(event.target.value)}
              className="h-8 rounded-md border border-neutral-200 bg-white px-2 text-sm text-neutral-800 outline-none focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100"
            >
              <option value="">{t('agentDetail.selectToolConnection')}</option>
              {availableConnections.map(connection => (
                <option key={connection.id} value={connection.id}>
                  {connectionOptionLabel(connection)} · {connection.ownerType}
                </option>
              ))}
            </select>
            <button type="button" onClick={() => void enableBinding()} disabled={saving || !connectionId} className={primaryButtonCls}>
              {t('agentDetail.enableTool')}
            </button>
          </div>
          {bindingsState.status === 'ok' && enabledToolCount > 0 && (
            <div className="mt-3">
              <p className="mb-2 text-xs text-neutral-500 dark:text-zinc-400">
                {t('agentDetail.enabledToolCount', { count: enabledToolCount })}
              </p>
              <div className="flex flex-wrap gap-2">
                {(bindingsState.data.bindings ?? []).map(binding => {
                  const connection = connections.find(item => item.id === binding.connectionId)
                  const label = toolBindingLabel(binding, connection)
                  return (
                    <span key={binding.id} className="inline-flex max-w-full items-center gap-2 rounded-md border border-neutral-200 bg-white px-2 py-1 text-xs text-neutral-600 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300">
                      <span className="truncate">{label}</span>
                      <span className="shrink-0 text-neutral-400 dark:text-zinc-500">{binding.status}</span>
                      <button type="button" disabled={saving} onClick={() => void removeBinding(binding)} className="shrink-0 font-medium text-red-500 hover:text-red-600 disabled:opacity-50">
                        {t('agentDetail.removeToolBinding')}
                      </button>
                    </span>
                  )
                })}
              </div>
            </div>
          )}
        </div>
      </div>
    </section>
  )
}

function toolBindingLabel(binding: RuntimeToolBinding, connection?: ConnectionOption): string {
  if (!connection) return binding.provider
  return connectionOptionLabel(connection)
}

function connectionOptionLabel(connection: ConnectionOption): string {
  if (connection.provider !== 'custom-mcp') return `${connection.provider} / ${connection.connectionName}`
  const profile = connection.profile ?? {}
  const toolName = typeof profile.toolName === 'string' && profile.toolName.trim()
    ? profile.toolName.trim()
    : connection.profileSummary?.displayName?.trim() || ''
  return toolName ? `${toolName} / ${connection.connectionName}` : connection.connectionName
}

function AgentSkillsPanel({ skills, skillDetails }: { skills: string[]; skillDetails?: Array<{ name: string; displayName?: string; description?: string }> }) {
  const { t } = useTranslation()
  const detailByName = new Map((skillDetails ?? []).map((sk) => [sk.name, sk]))
  return (
    <section className="p-4">
      <SubsectionHeader title={t('agentDetail.inheritedSkills')} description={t('agentDetail.skillsHint')} />
      <div className="mt-3">
        {skills.length > 0 ? (
          <div className="flex flex-wrap gap-2">
            {skills.map((sk) => {
              const detail = detailByName.get(sk)
              const label = detail?.displayName || sk
              return (
                <Link key={sk} to={`/skills?open=${encodeURIComponent(sk)}`}
                  className="inline-flex items-center rounded-md bg-amber-50 px-2.5 py-1 text-sm font-medium text-amber-700 transition-colors hover:bg-amber-100 dark:bg-amber-900/20 dark:text-amber-400 dark:hover:bg-amber-900/40">
                  <span>{label}</span>
                </Link>
              )
            })}
          </div>
        ) : (
          <p className="text-sm text-neutral-400 dark:text-zinc-500">{t('agentDetail.noSkills')}</p>
        )}
      </div>
    </section>
  )
}

function EnvEditor({ project, agentName, model, initialEnv, initialProvider, initialRuntimeModel, onChanged }: {
  project: string; agentName: string; model: string; initialEnv: Record<string, string>; initialProvider?: string; initialRuntimeModel?: string; onChanged: () => void
}) {
  const { t } = useTranslation()
  const [busy, setBusy] = useState(false)
  const [saved, setSaved] = useState(false)
  const [providerOptions, setProviderOptions] = useState<ProviderOption[]>([])
  const [modelCatalog, setModelCatalog] = useState<ModelCatalog | null>(null)
  const [selectedProvider, setSelectedProvider] = useState(initialProvider ?? '')
  const [runtimeModel, setRuntimeModel] = useState(initialRuntimeModel ?? '')
  const [customRuntimeModel, setCustomRuntimeModel] = useState(initialRuntimeModel ?? '')
  const [runtimeModelCustomMode, setRuntimeModelCustomMode] = useState(false)

  useEffect(() => {
    const path = `/api/v1/providers?project=${encodeURIComponent(project)}&agent=${encodeURIComponent(agentName)}`
    void apiFetch<ProviderOption[]>(path).then(data => setProviderOptions(data ?? [])).catch(() => {})
  }, [project, agentName])

  useEffect(() => {
    void apiFetch<ModelCatalog>('/api/v1/model-catalog').then(setModelCatalog).catch(() => setModelCatalog(null))
  }, [])

  useEffect(() => {
    setSelectedProvider(initialProvider ?? '')
    setRuntimeModel(initialRuntimeModel ?? '')
    setCustomRuntimeModel(initialRuntimeModel ?? '')
    setRuntimeModelCustomMode(false)
    setSaved(false)
  }, [initialEnv, initialProvider, initialRuntimeModel])

  const visibleProviderOptions = providerOptions.filter((p) => providerMatchesAgentModel(p, model))
  const selectedProviderExists = Boolean(selectedProvider && visibleProviderOptions.some((p) => p.id === selectedProvider))
  const selectedProviderValue = selectedProviderExists ? selectedProvider : ''
  const selectedProviderInfo = visibleProviderOptions.find((p) => p.id === selectedProviderValue)
  const runtimeModelOptions = runtimeModelOptionsFor(model, selectedProviderInfo, modelCatalog)
  const runtimeModelSelectValue = runtimeModel
    ? runtimeModelOptions.includes(runtimeModel) ? runtimeModel : '__custom__'
    : ''
  const effectiveRuntimeModelSelectValue = runtimeModelCustomMode ? '__custom__' : runtimeModelSelectValue
  const showCustomRuntimeModel = runtimeModelCustomMode || runtimeModelSelectValue === '__custom__'

  async function save() {
    setBusy(true); setSaved(false)
    try {
      const env = { ...initialEnv }
      await apiPut(`/api/v1/projects/${encodeURIComponent(project)}/agents/${encodeURIComponent(agentName)}/env`, { env, provider: selectedProviderValue, runtimeModel })
      setSaved(true)
      setTimeout(() => setSaved(false), 2500)
      onChanged()
    } catch (e) { alert(String(e)) }
    finally { setBusy(false) }
  }

  const inputCls = 'h-8 rounded-md border border-neutral-200 bg-white px-2.5 text-sm text-neutral-700 outline-none hover:border-neutral-300 focus:border-sky-400 disabled:opacity-50 dark:border-zinc-700 dark:bg-zinc-800 dark:text-zinc-300 dark:[color-scheme:dark]'

  return (
    <div>
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-zinc-300">
          {t('members.apiProvider')}
        </h3>
        <div className="flex items-center gap-2">
          {saved && <span className="text-xs text-emerald-500">{t('forms.saved')}</span>}
          <button type="button" onClick={save} disabled={busy}
            className={primaryButtonCls}>
            {busy ? t('forms.saving') : t('forms.save')}
          </button>
        </div>
      </div>

      <div className="grid gap-3 md:grid-cols-2">
        <label className="block">
          <span className="mb-1 block text-xs font-medium text-neutral-600 dark:text-zinc-400">{t('provider.selectLabel')}</span>
          <select value={selectedProviderValue} onChange={e => {
            const nextProviderID = e.target.value
            const nextProvider = providerOptions.find((p) => p.id === nextProviderID)
            const nextOptions = runtimeModelOptionsFor(model, nextProvider, modelCatalog)
            setSelectedProvider(nextProviderID)
            setRuntimeModelCustomMode(false)
            if (!runtimeModel && nextOptions[0]) {
              setRuntimeModel(nextOptions[0])
              setCustomRuntimeModel(nextOptions[0])
            }
            setSaved(false)
          }}
            className={cn(inputCls, 'w-full text-xs')}>
            <option value="">{visibleProviderOptions.length > 0 ? t('provider.none') : t('agentDetail.noCredential')}</option>
            {visibleProviderOptions.map(p => (
              <option key={p.id} value={p.id}>
                {p.name} ({p.type}{p.model ? ` · ${p.model}` : ''} · {p.ownerType === 'user' ? t('provider.scopePersonal') : t('provider.scopeWorkspace')})
              </option>
            ))}
          </select>
        </label>
        {visibleProviderOptions.length === 0 && (
          <div className="flex items-end">
            <Link
              to="/settings#model-accounts"
              className={primaryButtonCls}
            >
              {t('agentDetail.addCredential')}
            </Link>
          </div>
        )}
        <label className="block">
          <span className="mb-1 block text-xs font-medium text-neutral-600 dark:text-zinc-400">{t('provider.runtimeModelLabel')}</span>
          <select
            value={effectiveRuntimeModelSelectValue}
            onChange={e => {
              const next = e.target.value
              if (next === '__custom__') {
                setRuntimeModelCustomMode(true)
                setRuntimeModel(customRuntimeModel || '')
              } else {
                setRuntimeModelCustomMode(false)
                setRuntimeModel(next)
                setCustomRuntimeModel(next)
              }
              setSaved(false)
            }}
            className={cn(inputCls, 'w-full font-mono text-xs')}
          >
            <option value="">{t('provider.useProviderDefault')}</option>
            {runtimeModelOptions.map(option => <option key={option} value={option}>{option}</option>)}
            <option value="__custom__">{t('provider.customRuntimeModel')}</option>
          </select>
          {showCustomRuntimeModel && (
            <input
              value={customRuntimeModel}
              onChange={e => {
                setCustomRuntimeModel(e.target.value)
                setRuntimeModel(e.target.value)
                setSaved(false)
              }}
              className={cn(inputCls, 'mt-2 w-full font-mono text-xs')}
              placeholder={t('provider.runtimeModelPlaceholder')}
            />
          )}
        </label>
        {selectedProvider && (
          <p className="md:col-span-2 text-[11px] text-neutral-400 dark:text-zinc-500">{t('provider.overrideHint')}</p>
        )}
      </div>
    </div>
  )
}

/* ── Human member panels ─────────────────────────────────────────────────── */

type HumanMsg = {
  id: string; from: string; to: string; subject?: string; body: string
  sentAt: string; readAt?: string; archivedAt?: string; mailbox: string
}

function HumanMessagesPanel({ project, member }: { project: string; member: string }) {
  const { t } = useTranslation()
  const fmt = useFormatDateTime()
  const [page, setPage] = useState(1)
  const perPage = 10
  const [reloadKey, setReloadKey] = useState(0)
  const [expanded, setExpanded] = useState<string | null>(null)
  const [replyTo, setReplyTo] = useState<string | null>(null)
  const [replyBody, setReplyBody] = useState('')
  const [replyBusy, setReplyBusy] = useState(false)

  const mailbox = `${project}/${member}`
  const state = useApiJson<HumanMsg[]>(`/api/v1/projects/${encodeURIComponent(project)}/messages?mailbox=${encodeURIComponent(mailbox)}&archived=all`, reloadKey)
  const messages = state.status === 'ok' ? (state.data ?? []) : []
  const totalPages = Math.ceil(messages.length / perPage)
  const paged = useMemo(() => messages.slice((page - 1) * perPage, page * perPage), [messages, page])

  async function sendReply(msg: HumanMsg) {
    if (!replyBody.trim()) return
    setReplyBusy(true)
    try {
      await apiPost('/api/v1/messages', { from: 'human', to: msg.from, body: replyBody.trim() })
      await apiPost('/api/v1/messages/mark-read', { mailbox: msg.mailbox, id: msg.id }).catch(() => {})
      setReplyBody('')
      setReplyTo(null)
      setReloadKey((k) => k + 1)
    } catch (e) { alert(String(e)) }
    finally { setReplyBusy(false) }
  }

  async function markRead(msg: HumanMsg) {
    await apiPost('/api/v1/messages/mark-read', { mailbox: msg.mailbox, id: msg.id })
    setReloadKey((k) => k + 1)
  }

  return (
    <section>
      <SectionHeader icon={Mail} title={t('workbench.tabMessages')} />
      <div className="mt-3 rounded-lg border border-neutral-200/80 bg-white dark:border-zinc-700/60 dark:bg-zinc-900/40">
        {state.status === 'loading' && (
          <div className="flex items-center gap-2 py-8 justify-center">
            <div className="size-4 animate-spin rounded-full border-2 border-neutral-300 border-t-sky-600 dark:border-zinc-600 dark:border-t-sky-400" />
          </div>
        )}
        {state.status === 'ok' && messages.length === 0 && (
          <div className="flex flex-col items-center py-10 text-center">
            <Mail className="mb-2 size-6 text-neutral-300 dark:text-zinc-600" strokeWidth={1.5} />
            <p className="text-sm text-neutral-400 dark:text-zinc-500">{t('workbench.emptyMessages')}</p>
          </div>
        )}
        {state.status === 'ok' && messages.length > 0 && (
          <div className="divide-y divide-neutral-100 dark:divide-zinc-800/40">
            {paged.map((msg) => {
              const unread = !msg.readAt
              const isExpanded = expanded === msg.id
              const isReplying = replyTo === msg.id
              return (
                <div key={msg.id} className="px-4 py-3">
                  <div
                    className="flex cursor-pointer items-center gap-3"
                    onClick={() => { setExpanded(isExpanded ? null : msg.id); setReplyTo(null) }}
                  >
                    {unread && <span className="size-2 shrink-0 rounded-full bg-sky-500" />}
                    {!unread && <span className="size-2 shrink-0" />}
                    <span className="font-mono text-xs font-semibold text-neutral-700 dark:text-zinc-300">{msg.from}</span>
                    <span className="text-xs text-neutral-400 dark:text-zinc-500">→ {msg.to}</span>
                    <span className="ml-auto shrink-0 text-[11px] text-neutral-400 dark:text-zinc-500">{fmt(msg.sentAt)}</span>
                  </div>
                  {!isExpanded && (
                    <p className="mt-1 truncate pl-5 text-xs text-neutral-500 dark:text-zinc-500">
                      {msg.body.replace(/\s+/g, ' ').slice(0, 120)}
                    </p>
                  )}
                  {isExpanded && (
                    <div className="mt-2 pl-5">
                      <div className="prose prose-sm prose-neutral dark:prose-invert max-w-none rounded-md border border-neutral-100 bg-neutral-50/50 p-3 text-xs leading-relaxed dark:border-zinc-700/40 dark:bg-zinc-800/30 [&_pre]:overflow-x-auto">
                        <Markdown remarkPlugins={[remarkGfm]}>{msg.body}</Markdown>
                      </div>
                      <div className="mt-2 flex items-center gap-2">
                        {unread && (
                          <button type="button" onClick={() => void markRead(msg)}
                            className="rounded-md px-2 py-1 text-[11px] font-medium text-sky-700 hover:bg-sky-50 dark:text-sky-400 dark:hover:bg-sky-900/20">
                            {t('forms.markAsRead')}
                          </button>
                        )}
                        <button type="button" onClick={() => { setReplyTo(isReplying ? null : msg.id); setReplyBody('') }}
                          className="flex items-center gap-1 rounded-md px-2 py-1 text-[11px] font-medium text-sky-700 hover:bg-sky-50 dark:text-sky-400 dark:hover:bg-sky-900/20">
                          <Reply className="size-3" strokeWidth={2} />
                          {t('workbench.replyTo')}
                        </button>
                      </div>
                      {isReplying && (
                        <div className="mt-2 rounded-md border border-sky-200/60 bg-sky-50/30 p-3 dark:border-sky-800/40 dark:bg-sky-950/20">
                          <textarea
                            autoFocus value={replyBody} onChange={(e) => setReplyBody(e.target.value)}
                            rows={3} placeholder={t('forms.body')}
                            className="block w-full resize-y rounded-md border border-neutral-200 bg-white p-2 text-xs outline-none focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-200"
                          />
                          <div className="mt-2 flex justify-end gap-2">
                            <button type="button" onClick={() => setReplyTo(null)}
                              className="rounded-md px-2.5 py-1 text-[11px] text-neutral-500 hover:bg-neutral-100 dark:text-zinc-500 dark:hover:bg-zinc-800">
                              {t('forms.cancel')}
                            </button>
                            <button type="button" disabled={replyBusy || !replyBody.trim()} onClick={() => void sendReply(msg)}
                              className="flex items-center gap-1 rounded-md bg-sky-600 px-3 py-1 text-[11px] font-medium text-white hover:bg-sky-700 disabled:opacity-50">
                              <Send className="size-3" strokeWidth={2} />
                              {replyBusy ? t('forms.sending') : t('forms.send')}
                            </button>
                          </div>
                        </div>
                      )}
                    </div>
                  )}
                </div>
              )
            })}
          </div>
        )}
        <Pagination page={page} totalPages={totalPages} onPageChange={setPage} />
      </div>
    </section>
  )
}

type HumanTask = {
  id: string; project: string; agent: string; assignee?: string
  title: string; prompt?: string; status: string; priority: number
  type?: string; createdAt: string; updatedAt: string
}

const taskStatusCls: Record<string, string> = {
  pending: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
  running: 'bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-400',
  done_success: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400',
  done_failed: 'bg-red-100 text-red-600 dark:bg-red-900/30 dark:text-red-400',
  cancelled: 'bg-neutral-100 text-neutral-500 dark:bg-zinc-800 dark:text-zinc-500',
  awaiting_confirmation: 'bg-violet-100 text-violet-700 dark:bg-violet-900/30 dark:text-violet-400',
}

function HumanTasksPanel({ project, member }: { project: string; member: string }) {
  const { t } = useTranslation()
  const fmt = useFormatDateTime()
  const [page, setPage] = useState(1)
  const perPage = 10
  const [expanded, setExpanded] = useState<string | null>(null)

  const state = useApiJson<HumanTask[]>(`/api/v1/projects/${encodeURIComponent(project)}/tasks?agent=${encodeURIComponent(member)}`)
  const tasks = state.status === 'ok' ? (state.data ?? []) : []
  const totalPages = Math.ceil(tasks.length / perPage)
  const paged = useMemo(() => tasks.slice((page - 1) * perPage, page * perPage), [tasks, page])

  return (
    <section>
      <SectionHeader icon={ListTodo} title={t('workbench.tabTasks')} />
      <div className="mt-3 rounded-lg border border-neutral-200/80 bg-white dark:border-zinc-700/60 dark:bg-zinc-900/40">
        {state.status === 'loading' && (
          <div className="flex items-center gap-2 py-8 justify-center">
            <div className="size-4 animate-spin rounded-full border-2 border-neutral-300 border-t-sky-600 dark:border-zinc-600 dark:border-t-sky-400" />
          </div>
        )}
        {state.status === 'ok' && tasks.length === 0 && (
          <div className="flex flex-col items-center py-10 text-center">
            <ListTodo className="mb-2 size-6 text-neutral-300 dark:text-zinc-600" strokeWidth={1.5} />
            <p className="text-sm text-neutral-400 dark:text-zinc-500">{t('workbench.emptyTasks')}</p>
          </div>
        )}
        {state.status === 'ok' && tasks.length > 0 && (
          <div className="divide-y divide-neutral-100 dark:divide-zinc-800/40">
            {paged.map((task) => {
              const sCls = taskStatusCls[task.status] ?? taskStatusCls.pending
              const isExpanded = expanded === task.id
              return (
                <div key={task.id} className="px-4 py-3">
                  <div
                    className="flex cursor-pointer items-center gap-3"
                    onClick={() => setExpanded(isExpanded ? null : task.id)}
                  >
                    <span className={cn('shrink-0 rounded-full px-2 py-0.5 text-[10px] font-semibold', sCls)}>
                      {t(`tasks.status.${task.status}`, { defaultValue: task.status })}
                    </span>
                    <span className="min-w-0 flex-1 truncate text-sm font-medium text-neutral-800 dark:text-zinc-200">{task.title}</span>
                    <span className="shrink-0 text-[11px] text-neutral-400 dark:text-zinc-500">{fmt(task.updatedAt)}</span>
                  </div>
                  {isExpanded && task.prompt && (
                    <div className="mt-2 pl-5">
                      <div className="prose prose-sm prose-neutral dark:prose-invert max-w-none rounded-md border border-neutral-100 bg-neutral-50/50 p-3 text-xs leading-relaxed dark:border-zinc-700/40 dark:bg-zinc-800/30 [&_pre]:overflow-x-auto">
                        <Markdown remarkPlugins={[remarkGfm]}>{task.prompt}</Markdown>
                      </div>
                    </div>
                  )}
                </div>
              )
            })}
          </div>
        )}
        <Pagination page={page} totalPages={totalPages} onPageChange={setPage} />
      </div>
    </section>
  )
}
