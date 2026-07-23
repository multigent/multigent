import { useCallback, useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { useParams, useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Edit3, Maximize2, Minimize2, RefreshCw, Send, Sparkles, Square } from 'lucide-react'
import { ConversationLog } from '../../components/ui/ConversationLog'
import { apiFetch, apiDelete, apiUrl } from '../../lib/api'
import { getStoredToken, useAuth } from '../../lib/auth'
import { cn } from '../../lib/cn'

type HistoryResp = {
  sessionId?: string
  content?: string
  truncated?: boolean
  runs?: Array<{ startedAt: string; status: string; logPath: string }>
}

type AgentContext = {
  name?: string
  avatar?: string
  model: string
  runtimeModel?: string
  provider?: string
  sandbox?: {
    provider?: string
  }
  env?: Record<string, string>
  httpAgent?: {
    model?: string
  }
  runtimeReadiness?: RuntimeReadiness
}

type ProviderOption = {
  id: string
  model?: string
}

type RuntimeCheck = {
  key: string
  label: string
  status: 'ok' | 'warning' | 'error'
  detail?: string
  action?: string
  blocking?: boolean
}

type RuntimeReadiness = {
  ready: boolean
  blocking: boolean
  summary: string
  checks?: RuntimeCheck[]
}

function appendLog(prev: string, line: string): string {
  return prev ? `${prev}\n${line}` : line
}

function resetTextareaHeight(el: HTMLTextAreaElement | null) {
  if (el) el.style.height = 'auto'
}

function agentChatDraftKey(projectId?: string, agentName?: string) {
  if (!projectId || !agentName) return null
  return `multigent.agentChatDraft:${projectId}:${agentName}`
}

function readAgentChatDraft(projectId?: string, agentName?: string) {
  const key = agentChatDraftKey(projectId, agentName)
  if (!key) return ''
  try {
    return window.sessionStorage.getItem(key) ?? ''
  } catch {
    return ''
  }
}

export default function ProjectAgentChatPage() {
  const { t } = useTranslation()
  const { user } = useAuth()
  const { projectId, agentName } = useParams<{ projectId: string; agentName: string }>()
  const [searchParams, setSearchParams] = useSearchParams()
  const routeSessionId = searchParams.get('sessionId') ?? ''
  const [sessionId, setSessionId] = useState(routeSessionId)
  const [content, setContent] = useState('')
  const [input, setInput] = useState(() => readAgentChatDraft(projectId, agentName))
  const [loading, setLoading] = useState(false)
  const [runNotice, setRunNotice] = useState<string | null>(null)
  const [historyLoading, setHistoryLoading] = useState(false)
  const [historyTruncated, setHistoryTruncated] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [runtimeWarning, setRuntimeWarning] = useState<string | null>(null)
  const [runtimeReadiness, setRuntimeReadiness] = useState<RuntimeReadiness | null>(null)
  const [runtimeChecking, setRuntimeChecking] = useState(false)
  const [freshNext, setFreshNext] = useState(false)
  const [focusMode, setFocusMode] = useState(false)
  const [agentContext, setAgentContext] = useState<AgentContext | null>(null)
  const [providers, setProviders] = useState<ProviderOption[]>([])
  const [sessionEditorOpen, setSessionEditorOpen] = useState(false)
  const [sessionDraft, setSessionDraft] = useState(routeSessionId)
  const scrollRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)
  const abortRef = useRef<AbortController | null>(null)
  const draftKey = agentChatDraftKey(projectId, agentName)
  const chatKey = projectId && agentName ? `${projectId}/${agentName}` : ''
  const activeChatKeyRef = useRef(chatKey)
  const runtimeBlocked = Boolean(runtimeReadiness?.blocking)

  // Sync sessionId state + URL query param together so page refresh preserves the session.
  const updateSessionId = useCallback((sid: string) => {
    setSessionId(sid)
    setSessionDraft(sid)
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      if (sid) next.set('sessionId', sid)
      else next.delete('sessionId')
      return next
    }, { replace: true })
  }, [setSearchParams])

  const historyPath = useCallback((project: string | undefined, agent: string | undefined, sid: string) => {
    if (!project || !agent) return null
    const base = `/api/v1/projects/${encodeURIComponent(project)}/agents/${encodeURIComponent(agent)}/chat/history`
    return sid ? `${base}?sessionId=${encodeURIComponent(sid)}` : base
  }, [])

  const readinessPath = useCallback((project: string | undefined, agent: string | undefined) => {
    if (!project || !agent) return null
    return `/api/v1/projects/${encodeURIComponent(project)}/agents/${encodeURIComponent(agent)}/runtime/readiness`
  }, [])

  const loadReadiness = useCallback(async (project = projectId, agent = agentName, expectedKey = chatKey) => {
    const path = readinessPath(project, agent)
    if (!path) return null
    setRuntimeChecking(true)
    try {
      const data = await apiFetch<RuntimeReadiness>(path)
      if (activeChatKeyRef.current !== expectedKey) return null
      setRuntimeReadiness(data)
      const warnings = (data.checks ?? []).filter((check) => check.status === 'warning')
      if (data.blocking) {
        setRuntimeWarning(null)
      } else if (warnings.length > 0) {
        setRuntimeWarning(t('agentChat.runtimeMayBeSlow'))
      } else {
        setRuntimeWarning(null)
      }
      return data
    } catch {
      return null
    } finally {
      if (activeChatKeyRef.current === expectedKey) setRuntimeChecking(false)
    }
  }, [agentName, chatKey, projectId, readinessPath, t])

  const loadHistory = useCallback(async (sid: string, project = projectId, agent = agentName, expectedKey = chatKey) => {
    const path = historyPath(project, agent, sid)
    if (!path) return
    setHistoryLoading(true)
    setError(null)
    setRuntimeWarning(null)
    try {
      const data = await apiFetch<HistoryResp>(path)
      if (activeChatKeyRef.current !== expectedKey) return
      updateSessionId(data.sessionId ?? sid)
      setContent(data.content ?? '')
      setHistoryTruncated(Boolean(data.truncated))
    } catch (e) {
      if (activeChatKeyRef.current !== expectedKey) return
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      if (activeChatKeyRef.current === expectedKey) setHistoryLoading(false)
    }
  }, [agentName, chatKey, historyPath, projectId, updateSessionId])

  // Route params can change without remounting this page. Keep every chat view
  // scoped by project/agent and stop any in-flight stream from the previous one.
  useEffect(() => {
    if (!projectId || !agentName) return
    const nextKey = `${projectId}/${agentName}`
    const currentRouteSessionId = new URLSearchParams(window.location.search).get('sessionId') ?? ''
    activeChatKeyRef.current = nextKey
    abortRef.current?.abort()
    abortRef.current = null
    setContent('')
    setInput(readAgentChatDraft(projectId, agentName))
    setError(null)
    setLoading(false)
    setRunNotice(null)
    setHistoryLoading(false)
    setHistoryTruncated(false)
    setFreshNext(false)
    setRuntimeReadiness(null)
    setRuntimeWarning(null)
    setSessionEditorOpen(false)
    setSessionId(currentRouteSessionId)
    setSessionDraft(currentRouteSessionId)
    void loadHistory(currentRouteSessionId, projectId, agentName, nextKey)
    void loadReadiness(projectId, agentName, nextKey)
  }, [projectId, agentName, loadHistory, loadReadiness])

  useEffect(() => {
    if (!projectId || !agentName) return
    let cancelled = false
    const contextPath = `/api/v1/projects/${encodeURIComponent(projectId)}/agents/${encodeURIComponent(agentName)}/context`
    void apiFetch<AgentContext>(contextPath)
      .then((data) => {
        if (!cancelled) {
          setAgentContext(data)
          if (data.runtimeReadiness) setRuntimeReadiness(data.runtimeReadiness)
        }
      })
      .catch(() => {})
    void apiFetch<ProviderOption[]>('/api/v1/providers')
      .then((data) => { if (!cancelled) setProviders(data ?? []) })
      .catch(() => {})
    return () => { cancelled = true }
  }, [projectId, agentName])

  useEffect(() => {
    requestAnimationFrame(() => {
      if (scrollRef.current) scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    })
  }, [content, loading])

  useEffect(() => {
    if (!draftKey) return
    try {
      if (input) window.sessionStorage.setItem(draftKey, input)
      else window.sessionStorage.removeItem(draftKey)
    } catch {
      // Ignore storage failures; losing a draft is better than breaking chat.
    }
  }, [draftKey, input])

  useEffect(() => {
    const el = inputRef.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = Math.min(el.scrollHeight, 128) + 'px'
  }, [input])

  useEffect(() => {
    if (!focusMode) return
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.body.style.overflow = prev
    }
  }, [focusMode])

  async function send() {
    const text = input.trim()
    if (!text || loading || !projectId || !agentName) return
    const runProject = projectId
    const runAgent = agentName
    const runKey = `${runProject}/${runAgent}`
    const runSessionId = sessionId
    const runFreshNext = freshNext

    setInput('')
    resetTextareaHeight(inputRef.current)
    setError(null)
    setRunNotice(null)

    const readiness = await loadReadiness(runProject, runAgent, runKey)
    if (readiness?.blocking) {
      setError(runtimeBlockingMessage(readiness, t))
      setInput(text)
      return
    }

    setLoading(true)
    setRunNotice(t('agentChat.preparingSandbox'))
    setContent((prev) => appendLog(prev, JSON.stringify({ type: 'human', content: text })))

    const controller = new AbortController()
    abortRef.current = controller
    try {
      const headers: Record<string, string> = {
        'Content-Type': 'application/json',
        'Accept': 'text/event-stream',
      }
      const token = getStoredToken()
      if (token) headers.Authorization = `Bearer ${token}`

      const res = await fetch(apiUrl(`/api/v1/projects/${encodeURIComponent(runProject)}/agents/${encodeURIComponent(runAgent)}/chat`), {
        method: 'POST',
        headers,
        body: JSON.stringify({
          message: text,
          sessionId: runSessionId,
          noSession: runFreshNext && !runSessionId,
        }),
        signal: controller.signal,
      })
      if (!res.ok) {
        const errText = await res.text()
        throw new Error(errText || `HTTP ${res.status}`)
      }

      const reader = res.body?.getReader()
      if (!reader) throw new Error('no response body')

      const decoder = new TextDecoder()
      let buffer = ''
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        buffer += decoder.decode(value, { stream: true })
        const parts = buffer.split('\n')
        buffer = parts.pop() ?? ''
        for (const part of parts) {
          if (!part.startsWith('data: ')) continue
          const data = part.slice(6)
          if (!data) continue
          if (activeChatKeyRef.current !== runKey) continue
          try {
            const evt = JSON.parse(data)
            if (evt.type === 'chat_event') {
              if (evt.session_id) updateSessionId(String(evt.session_id))
              if (typeof evt.payload === 'string' && evt.payload) {
                setRunNotice(null)
                setContent((prev) => appendLog(prev, evt.payload))
              }
              continue
            }
            if (evt.type === 'chat_done') {
              if (evt.session_id) updateSessionId(evt.session_id)
              continue
            }
            if (evt.type === 'chat_error') {
              const msg = evt.error ? String(evt.error) : t('agentChat.error')
              setRunNotice(null)
              setError(msg)
              setContent((prev) => appendLog(prev, `=== Error: ${msg} ===`))
              continue
            }
            if (evt.session_id) updateSessionId(evt.session_id)
          } catch (e) {
            const msg = e instanceof Error ? e.message : String(e)
            setError(`${t('agentChat.protocolError')}: ${msg}`)
            continue
          }
        }
      }
    } catch (e) {
      if (activeChatKeyRef.current !== runKey) return
      const stopped = (e as Error).name === 'AbortError'
      const msg = stopped ? t('agentChat.stopped') : (e instanceof Error ? e.message : String(e))
      setError(stopped ? null : msg)
      setContent((prev) => appendLog(prev, `=== ${msg} ===`))
    } finally {
      if (activeChatKeyRef.current !== runKey) return
      abortRef.current = null
      setFreshNext(false)
      setLoading(false)
      setRunNotice(null)
      inputRef.current?.focus()
    }
  }

  function handleKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      void send()
    }
  }

  function stopChat() {
    abortRef.current?.abort()
    if (projectId && agentName) {
      void apiDelete(`/api/v1/projects/${encodeURIComponent(projectId)}/agents/${encodeURIComponent(agentName)}/chat`)
    }
  }

  function startFresh() {
    abortRef.current?.abort()
    updateSessionId('')
    setContent('')
    setError(null)
    setFreshNext(true)
    resetTextareaHeight(inputRef.current)
    inputRef.current?.focus()
  }

  function switchSession() {
    const next = sessionDraft.trim()
    setSessionEditorOpen(false)
    setFreshNext(false)
    updateSessionId(next)
    void loadHistory(next)
  }

  if (!projectId || !agentName) return null

  const agentType = agentContext?.model || ''
  const providerModel = providers.find((p) => p.id === agentContext?.provider)?.model
  const env = agentContext?.env ?? {}
  const concreteModel =
    agentContext?.runtimeModel ||
    agentContext?.httpAgent?.model ||
    env.ANTHROPIC_MODEL ||
    env.CLAUDE_MODEL ||
    env.OPENAI_MODEL ||
    env.CODEX_MODEL ||
    env.GEMINI_MODEL ||
    env.GOOGLE_MODEL ||
    env.CURSOR_MODEL ||
    providerModel ||
    ''
  const userParticipant = {
    name: user?.displayName || user?.username || t('common.user', { defaultValue: 'User' }),
    avatar: user?.avatar,
  }
  const assistantParticipant = {
    name: agentContext?.name || agentName,
    avatar: agentContext?.avatar,
  }

  const chatPanel = (
    <div className={cn(
      'flex h-full flex-col overflow-hidden bg-white dark:bg-zinc-950',
      focusMode && 'mx-auto w-full max-w-5xl rounded-2xl border border-neutral-200/80 shadow-2xl dark:border-zinc-700/70',
    )}>
      <div className="shrink-0 border-b border-neutral-200/70 px-6 py-4 dark:border-zinc-700/50">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="min-w-0">
            <h1 className="truncate text-xl font-semibold text-neutral-900 dark:text-zinc-100">{agentName}</h1>
            <div className="mt-1 flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-sm text-neutral-500 dark:text-zinc-500">
              {agentType && (
                <span className="rounded-md bg-neutral-100 px-2 py-0.5 font-mono text-xs font-semibold text-neutral-700 dark:bg-zinc-800 dark:text-zinc-300">
                  {agentType}
                </span>
              )}
              <span className="font-mono text-xs">
                {t('agentChat.modelLabel')}: {concreteModel || t('agentChat.modelUnknown')}
              </span>
              {sessionId && (
                <span className="break-all font-mono text-xs text-emerald-700 dark:text-emerald-400">
                  {sessionId}
                </span>
              )}
            </div>
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => { setSessionDraft(sessionId); setSessionEditorOpen(true) }}
              disabled={loading}
              className="flex items-center gap-1.5 rounded-md border border-neutral-200 bg-white px-3 py-1.5 text-sm font-medium text-neutral-600 transition-colors hover:bg-neutral-50 disabled:opacity-50 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-400 dark:hover:bg-zinc-800"
            >
              <Edit3 className="size-3.5" strokeWidth={1.8} />
              {t('agentChat.switchSession')}
            </button>
            <button
              type="button"
              onClick={() => void loadHistory(sessionId)}
              disabled={historyLoading || loading}
              className="flex items-center gap-1.5 rounded-md border border-neutral-200 bg-white px-3 py-1.5 text-sm font-medium text-neutral-600 transition-colors hover:bg-neutral-50 disabled:opacity-50 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-400 dark:hover:bg-zinc-800"
            >
              <RefreshCw className={cn('size-3.5', historyLoading && 'animate-spin')} />
              {t('agentChat.reloadHistory')}
            </button>
            <button
              type="button"
              onClick={startFresh}
              disabled={loading}
              className="rounded-md border border-sky-200 bg-sky-50 px-3 py-1.5 text-sm font-medium text-sky-700 transition-colors hover:bg-sky-100 disabled:opacity-50 dark:border-sky-800 dark:bg-sky-900/30 dark:text-sky-400 dark:hover:bg-sky-900/50"
            >
              {t('agentChat.newSession')}
            </button>
            <button
              type="button"
              onClick={() => setFocusMode((v) => !v)}
              className="flex items-center gap-1.5 rounded-md border border-neutral-200 bg-white px-3 py-1.5 text-sm font-medium text-neutral-600 transition-colors hover:bg-neutral-50 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-400 dark:hover:bg-zinc-800"
              title={focusMode ? t('agentChat.exitFocus') : t('agentChat.focusMode')}
            >
              {focusMode ? <Minimize2 className="size-3.5" /> : <Maximize2 className="size-3.5" />}
              {focusMode ? t('agentChat.exitFocus') : t('agentChat.focusMode')}
            </button>
          </div>
        </div>
        {historyTruncated && (
          <p className="mt-2 text-xs text-amber-600 dark:text-amber-400">{t('agentChat.historyTruncated')}</p>
        )}
        {sessionEditorOpen && (
          <div className="mt-3 rounded-xl border border-sky-200/70 bg-sky-50/40 p-3 dark:border-sky-800/50 dark:bg-sky-950/20">
            <label className="block text-xs font-medium text-sky-800 dark:text-sky-300">
              {t('agentChat.sessionIdLabel')}
            </label>
            <div className="mt-2 flex flex-col gap-2 sm:flex-row">
              <input
                autoFocus
                value={sessionDraft}
                onChange={(e) => setSessionDraft(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') switchSession()
                  if (e.key === 'Escape') setSessionEditorOpen(false)
                }}
                placeholder={t('agentChat.sessionIdPlaceholder')}
                className="min-w-0 flex-1 rounded-lg border border-neutral-200 bg-white px-3 py-2 font-mono text-sm text-neutral-800 outline-none focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-200"
              />
              <div className="flex items-center gap-2">
                <button
                  type="button"
                  onClick={switchSession}
                  className="rounded-lg bg-sky-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-sky-700"
                >
                  {t('agentChat.switchSessionConfirm')}
                </button>
                <button
                  type="button"
                  onClick={() => setSessionEditorOpen(false)}
                  className="rounded-lg px-3 py-2 text-sm font-medium text-neutral-500 transition-colors hover:bg-neutral-100 dark:text-zinc-400 dark:hover:bg-zinc-800"
                >
                  {t('forms.cancel')}
                </button>
              </div>
            </div>
            <p className="mt-2 text-xs text-sky-700/80 dark:text-sky-300/80">{t('agentChat.sessionSwitchHint')}</p>
          </div>
        )}
        <RuntimeReadinessBanner
          readiness={runtimeReadiness}
          checking={runtimeChecking}
          onRefresh={() => void loadReadiness()}
        />
        {error && (
          <p className="mt-2 text-xs text-red-500 dark:text-red-400">{error}</p>
        )}
        {!error && runtimeWarning && (
          <p className="mt-2 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs leading-5 text-amber-700 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-300">{runtimeWarning}</p>
        )}
      </div>

      <div ref={scrollRef} className="flex-1 overflow-y-auto px-6 py-5">
        {!content && !historyLoading ? (
          <div className="flex h-full flex-col items-center justify-center text-center">
            <Sparkles className="mb-3 size-10 text-neutral-200 dark:text-zinc-700" strokeWidth={1.3} />
            <p className="text-sm text-neutral-500 dark:text-zinc-400">{t('agentChat.empty')}</p>
            <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">{t('agentChat.emptyHint')}</p>
          </div>
        ) : (
          <div className="space-y-4">
            <ConversationLog content={content} mode="chat" user={userParticipant} assistant={assistantParticipant} />
            {loading && <AgentReplyLoading notice={runNotice} />}
          </div>
        )}
      </div>

      <div className="shrink-0 border-t border-neutral-200/70 bg-white px-6 py-4 dark:border-zinc-700/50 dark:bg-zinc-950">
        <div className="flex items-end gap-3 rounded-xl border border-neutral-200 bg-neutral-50 px-3 py-2 focus-within:border-sky-400 dark:border-zinc-700 dark:bg-zinc-900">
          <textarea
            ref={inputRef}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder={t('agentChat.placeholder')}
            rows={1}
            className="max-h-32 min-h-8 flex-1 resize-none bg-transparent py-1.5 text-sm leading-relaxed text-neutral-800 outline-none placeholder:text-neutral-400 dark:text-zinc-200 dark:placeholder:text-zinc-600"
            onInput={(e) => {
              const el = e.currentTarget
              el.style.height = 'auto'
              el.style.height = Math.min(el.scrollHeight, 128) + 'px'
            }}
          />
          {loading ? (
            <button
              type="button"
              onClick={stopChat}
              className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-red-500 text-white transition-colors hover:bg-red-600"
            >
              <Square className="size-3" fill="currentColor" />
            </button>
          ) : (
            <button
              type="button"
              onClick={() => void send()}
              disabled={!input.trim() || runtimeBlocked || runtimeChecking}
              className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-sky-600 text-white transition-colors hover:bg-sky-700 disabled:opacity-30"
              title={runtimeBlocked ? t('agentChat.runtimeBlocked') : undefined}
            >
              <Send className="size-4" />
            </button>
          )}
        </div>
        <p className="mt-2 text-xs text-neutral-400 dark:text-zinc-500">{t('agentChat.sendHint')}</p>
      </div>
    </div>
  )

  if (focusMode) {
    return (
      <div className="fixed inset-0 z-[80] bg-neutral-100/95 p-3 backdrop-blur-sm dark:bg-zinc-950/95 sm:p-6">
        {chatPanel}
      </div>
    )
  }

  return chatPanel
}

function RuntimeReadinessBanner({ readiness, checking, onRefresh }: {
  readiness: RuntimeReadiness | null
  checking: boolean
  onRefresh: () => void
}) {
  const { t } = useTranslation()
  if (!readiness && !checking) return null
  const checks = readiness?.checks ?? []
  const visibleChecks = checks.filter((check) => check.status !== 'ok')
  const ready = Boolean(readiness?.ready)
  const blocking = Boolean(readiness?.blocking)
  const cls = blocking
    ? 'border-red-200 bg-red-50 text-red-800 dark:border-red-900/50 dark:bg-red-950/20 dark:text-red-300'
    : ready
      ? 'border-emerald-200 bg-emerald-50 text-emerald-800 dark:border-emerald-900/50 dark:bg-emerald-950/20 dark:text-emerald-300'
      : 'border-amber-200 bg-amber-50 text-amber-800 dark:border-amber-900/50 dark:bg-amber-950/20 dark:text-amber-300'
  const summary = checking
    ? t('agentChat.runtimeCheckingHint')
    : blocking
      ? t('agentChat.runtimeBlockedHint')
      : ready && visibleChecks.length === 0
        ? t('agentChat.runtimeReadyHint')
        : t('agentChat.runtimePreparingHint')
  return (
    <div className={cn('mt-3 rounded-lg border px-3 py-2 text-xs leading-5', cls)}>
      <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <p className="font-medium">
            {checking ? t('agentChat.runtimeChecking') : blocking ? t('agentChat.runtimeBlocked') : ready ? t('agentChat.runtimeReady') : t('agentChat.runtimePreparing')}
          </p>
          <p className="mt-0.5 opacity-80">{summary}</p>
        </div>
        <button
          type="button"
          onClick={onRefresh}
          disabled={checking}
          className="w-fit rounded-md border border-current/20 bg-white/50 px-2.5 py-1 font-medium transition-colors hover:bg-white/80 disabled:opacity-50 dark:bg-zinc-950/20 dark:hover:bg-zinc-950/40"
        >
          {checking ? t('common.loading') : t('common.refresh')}
        </button>
      </div>
      {visibleChecks.length > 0 && (
        <div className="mt-2 space-y-1.5">
          {visibleChecks.map((check) => (
            <div key={check.key} className="rounded-md bg-white/55 px-2.5 py-1.5 dark:bg-zinc-950/20">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium">{check.label}</span>
                <span className="rounded-full border border-current/20 px-1.5 py-0.5 text-[10px] uppercase tracking-wide opacity-80">{check.status}</span>
              </div>
              {check.detail && <p className="mt-0.5 whitespace-pre-wrap opacity-80">{check.detail}</p>}
              {check.action && <p className="mt-0.5 font-mono opacity-90">{check.action}</p>}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

function runtimeBlockingMessage(readiness: RuntimeReadiness, t: (key: string) => string) {
  const blockers = (readiness.checks ?? []).filter((check) => check.blocking || check.status === 'error')
  if (blockers.length === 0) return t('agentChat.runtimeBlocked')
  return [
    t('agentChat.runtimeBlockedBeforeSend'),
    ...blockers.map((check) => {
      const action = check.action ? ` ${check.action}` : ''
      return `${check.label}: ${check.detail ?? check.status}.${action}`
    }),
  ].join('\n')
}

function AgentReplyLoading({ notice }: { notice?: string | null }) {
  return (
    <div className="flex gap-2.5">
      <div className="relative flex size-6 shrink-0 items-center justify-center rounded-full bg-neutral-100 dark:bg-zinc-800">
        <span className="absolute size-5 animate-ping rounded-full bg-sky-400/20" />
        <Sparkles className="relative size-3.5 text-sky-600 dark:text-sky-400" strokeWidth={1.8} />
      </div>
      <div>
        <div className="flex h-10 w-fit items-center gap-1.5 rounded-lg bg-neutral-50 px-3.5 dark:bg-zinc-900/70" aria-label="loading">
          <span className="h-2 w-1 animate-[pulse_1.1s_ease-in-out_infinite] rounded-full bg-sky-500/70" />
          <span className="h-4 w-1 animate-[pulse_1.1s_ease-in-out_infinite_120ms] rounded-full bg-sky-500/80" />
          <span className="h-2.5 w-1 animate-[pulse_1.1s_ease-in-out_infinite_240ms] rounded-full bg-sky-500/70" />
        </div>
        {notice && <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">{notice}</p>}
      </div>
    </div>
  )
}
