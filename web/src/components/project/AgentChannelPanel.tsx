import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { QRCodeSVG } from 'qrcode.react'
import { CheckCircle2, Loader2, MessageSquare, X } from 'lucide-react'
import { apiDelete, apiFetch, apiPost } from '../../lib/api'
import { cn } from '../../lib/cn'
import { confirmDialog } from '../ui/ConfirmDialog'
import { useFormatDateTime } from '../../lib/format-datetime'

type ChannelProvider = {
  id: string
  label: string
  setupMode?: 'qr' | 'manual'
  fields?: Array<{ name: string; label: string; type: string; required?: boolean; placeholder?: string; help?: string }>
}

type AgentChannel = {
  id: string
  provider: string
  status: string
  connectionId?: string
  callbackUrl?: string
  appId?: string
  accountsUrl?: string
  externalBotId?: string
  externalChatId?: string
  externalOwnerId?: string
  security?: {
    verificationTokenConfigured: boolean
    encryptKeyConfigured: boolean
  }
  callback?: {
    lastAt?: string
    status?: string
    reason?: string
    messageId?: string
    error?: string
  }
  createdBy?: string
  createdAt?: string
  updatedAt?: string
  lastActivityAt?: string
}

type ChannelsResponse = {
  providers: ChannelProvider[]
  channels: AgentChannel[]
}

type InteractionStatus = {
  active: boolean
  session?: {
    id: string
    sourceKind: string
    sourceChannel?: string
    actorId?: string
    status: string
    lockReason: string
    lastActivityAt?: string
  }
  events?: Array<{
    id: string
    actorType: string
    actorId?: string
    channel?: string
    eventType: string
    content?: string
    createdAt?: string
  }>
}

type SetupState =
  | { step: 'idle' }
  | { step: 'beginning'; provider: ChannelProvider }
  | { step: 'scanning'; provider: ChannelProvider; deviceCode: string; qrUrl: string; baseUrl: string; interval: number }
  | { step: 'manual'; provider: ChannelProvider; values: Record<string, string>; submitting?: boolean }
  | { step: 'connected'; provider: ChannelProvider }
  | { step: 'error'; provider?: ChannelProvider; message: string }

export function AgentChannelPanel({ project, agentName }: { project: string; agentName: string }) {
  const { t } = useTranslation()
  const fmtDateTime = useFormatDateTime()
  const [loading, setLoading] = useState(true)
  const [channels, setChannels] = useState<AgentChannel[]>([])
  const [interaction, setInteraction] = useState<InteractionStatus | null>(null)
  const [providers, setProviders] = useState<ChannelProvider[]>([
    { id: 'feishu', label: 'Feishu' },
    { id: 'lark', label: 'Lark' },
  ])
  const [setup, setSetup] = useState<SetupState>({ step: 'idle' })
  const [detail, setDetail] = useState<{ provider: ChannelProvider; channel: AgentChannel } | null>(null)
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const basePath = `/api/v1/projects/${encodeURIComponent(project)}/agents/${encodeURIComponent(agentName)}/channels`

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await apiFetch<ChannelsResponse>(basePath)
      setChannels(res.channels ?? [])
      if (res.providers?.length) setProviders(res.providers)
      const active = await apiFetch<InteractionStatus>(`/api/v1/projects/${encodeURIComponent(project)}/agents/${encodeURIComponent(agentName)}/interactions/active`)
      setInteraction(active)
    } finally {
      setLoading(false)
    }
  }, [basePath])

  useEffect(() => { void load() }, [load])
  useEffect(() => () => {
    if (pollRef.current) clearInterval(pollRef.current)
  }, [])

  function stopPoll() {
    if (pollRef.current) {
      clearInterval(pollRef.current)
      pollRef.current = null
    }
  }

  function closeSetup() {
    stopPoll()
    setSetup({ step: 'idle' })
  }

  async function begin(provider: ChannelProvider) {
    stopPoll()
    if (provider.setupMode === 'manual') {
      setSetup({ step: 'manual', provider, values: {} })
      return
    }
    setSetup({ step: 'beginning', provider })
    try {
      const res = await apiPost<{ deviceCode: string; qrUrl: string; interval?: number; baseUrl?: string }>(
        `${basePath}/${provider.id}/setup/begin`,
        {},
      )
      const next: SetupState = {
        step: 'scanning',
        provider,
        deviceCode: res.deviceCode,
        qrUrl: res.qrUrl,
        baseUrl: res.baseUrl || '',
        interval: res.interval || 5,
      }
      setSetup(next)
      startPoll(next)
    } catch (e) {
      setSetup({ step: 'error', provider, message: e instanceof Error ? e.message : String(e) })
    }
  }

  async function submitManualSetup() {
    if (setup.step !== 'manual') return
    setSetup({ ...setup, submitting: true })
    try {
      const res = await apiPost<{ status: string; channel?: AgentChannel }>(
        `${basePath}/${setup.provider.id}/setup/manual`,
        { values: setup.values },
      )
      if (res.status === 'connected') {
        setSetup({ step: 'connected', provider: setup.provider })
        await load()
      }
    } catch (e) {
      setSetup({ step: 'error', provider: setup.provider, message: e instanceof Error ? e.message : String(e) })
    }
  }

  function startPoll(state: Extract<SetupState, { step: 'scanning' }>) {
    stopPoll()
    let interval = Math.max(3, state.interval)
    const tick = async () => {
      try {
        const res = await apiPost<{ status: string; baseUrl?: string; slowDown?: boolean; error?: string; channel?: AgentChannel }>(
          `${basePath}/${state.provider.id}/setup/poll`,
          { deviceCode: state.deviceCode, baseUrl: state.baseUrl || undefined },
        )
        if (res.slowDown) interval += 5
        if (res.status === 'connected' && res.channel) {
          stopPoll()
          setSetup({ step: 'connected', provider: state.provider })
          await load()
        } else if (res.status === 'denied' || res.status === 'expired' || res.status === 'error') {
          stopPoll()
          setSetup({ step: 'error', provider: state.provider, message: res.error || t(`agentChannels.${res.status}`) })
        }
      } catch {
        // Keep polling through transient network errors.
      }
    }
    pollRef.current = setInterval(() => void tick(), interval * 1000)
    void tick()
  }

  async function disconnect(channel: AgentChannel) {
    const provider = providers.find(p => p.id === channel.provider)
    const ok = await confirmDialog({
      title: t('agentChannels.disconnect'),
      description: t('agentChannels.disconnectConfirm', { provider: provider?.label || channel.provider }),
      confirmLabel: t('common.delete'),
      cancelLabel: t('common.cancel'),
      tone: 'danger',
    })
    if (!ok) return
    await apiDelete(`${basePath}/${channel.provider}`)
    setDetail(null)
    await load()
  }

  const byProvider = new Map(channels.map(channel => [channel.provider, channel]))

  return (
    <section>
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <MessageSquare className="size-4 text-neutral-500 dark:text-zinc-500" strokeWidth={1.8} />
          <h4 className="text-sm font-semibold text-neutral-900 dark:text-zinc-100">{t('agentChannels.title')}</h4>
        </div>
        <button type="button" onClick={() => void load()} className="rounded-md border border-neutral-200 bg-white px-2.5 py-1 text-xs font-medium text-neutral-600 hover:border-neutral-300 hover:bg-neutral-50 disabled:opacity-50 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:border-zinc-600 dark:hover:bg-zinc-800" disabled={loading}>
          {t('common.refresh')}
        </button>
      </div>
      <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">{t('agentChannels.subtitle')}</p>

      <div className="mt-3 rounded-lg border border-neutral-200/80 bg-neutral-50 px-3 py-2 dark:border-zinc-700/60 dark:bg-zinc-900/40">
        <div className="flex items-center justify-between gap-3">
          <div>
            <p className="text-xs font-medium text-neutral-600 dark:text-zinc-300">{t('agentChannels.sessionStatus')}</p>
            <p className="mt-0.5 text-xs text-neutral-400 dark:text-zinc-500">
              {interaction?.active && interaction.session
                ? t('agentChannels.sessionActive', { source: interaction.session.sourceKind, actor: interaction.session.actorId || '-' })
                : t('agentChannels.sessionIdle')}
            </p>
          </div>
          {interaction?.active && (
            <span className="rounded-md bg-amber-50 px-2 py-1 text-xs font-medium text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">
              {interaction.session?.lockReason || 'active'}
            </span>
          )}
        </div>
        {interaction?.events?.length ? (
          <div className="mt-2 space-y-1">
            {interaction.events.slice(-3).map(event => (
              <div key={event.id} className="truncate text-xs text-neutral-500 dark:text-zinc-400">
                <span className="font-medium text-neutral-600 dark:text-zinc-300">{event.eventType}</span>
                {event.content ? <span> · {event.content}</span> : null}
              </div>
            ))}
          </div>
        ) : null}
      </div>

      <div className="mt-3 grid gap-3 md:grid-cols-2">
        {providers.map(provider => {
          const channel = byProvider.get(provider.id)
          return (
            <div key={provider.id} className="rounded-lg border border-neutral-200/80 bg-white p-3 dark:border-zinc-700/60 dark:bg-zinc-900/40">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium text-neutral-900 dark:text-zinc-100">{provider.label}</span>
                    {channel ? (
                      <span className="inline-flex items-center gap-1 rounded-md bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300">
                        <CheckCircle2 className="size-3" />
                        {t('agentChannels.connected')}
                      </span>
                    ) : (
                      <span className="rounded-md bg-neutral-100 px-2 py-0.5 text-xs font-medium text-neutral-500 dark:bg-zinc-800 dark:text-zinc-400">
                        {t('agentChannels.notConnected')}
                      </span>
                    )}
                  </div>
                  {channel ? (
                    <div className="mt-2 space-y-1 text-xs text-neutral-500 dark:text-zinc-400">
                      <p>{t('agentChannels.lastActivity', { time: fmtDateTime(channel.lastActivityAt) })}</p>
                      {channel.callback?.lastAt ? (
                        <p className={cn('truncate', channel.callback.error && 'text-red-500 dark:text-red-300')}>
                          {t('agentChannels.lastEventSummary', {
                            time: fmtDateTime(channel.callback.lastAt),
                            status: t(`agentChannels.callbackStatus.${channel.callback.status || 'unknown'}`),
                          })}
                        </p>
                      ) : (
                        <p>{t('agentChannels.eventPending')}</p>
                      )}
                    </div>
                  ) : (
                    <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">{t('agentChannels.connectHint')}</p>
                  )}
                </div>
                {channel ? (
                  <div className="flex items-center gap-1">
                    <button type="button" onClick={() => setDetail({ provider, channel })} className="rounded-md border border-neutral-200 bg-white px-2.5 py-1 text-xs font-medium text-neutral-600 hover:border-neutral-300 hover:bg-neutral-50 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:border-zinc-600 dark:hover:bg-zinc-800">
                      {t('agentChannels.details')}
                    </button>
                  </div>
                ) : (
                  <button type="button" onClick={() => void begin(provider)} disabled={setup.step === 'beginning' || setup.step === 'scanning'}
                    className="rounded-md border border-sky-200 bg-sky-50 px-2.5 py-1 text-xs font-medium text-sky-700 hover:bg-sky-100 disabled:opacity-50 dark:border-sky-800 dark:bg-sky-900/20 dark:text-sky-300 dark:hover:bg-sky-900/30">
                    {t('agentChannels.connect')}
                  </button>
                )}
              </div>
            </div>
          )
        })}
      </div>

      {(setup.step === 'beginning' || setup.step === 'scanning' || setup.step === 'manual' || setup.step === 'connected' || setup.step === 'error') && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 px-4">
          <div className="w-full max-w-md rounded-xl border border-neutral-200 bg-white p-5 shadow-xl dark:border-zinc-700 dark:bg-zinc-900">
            <div className="flex items-center justify-between gap-3">
              <h3 className="text-base font-semibold text-neutral-900 dark:text-zinc-100">
                {setup.step === 'error' ? t('agentChannels.setupFailed') : t('agentChannels.scanToConnect', { provider: setup.provider.label })}
              </h3>
              <button type="button" onClick={closeSetup} className="rounded-md p-1.5 text-neutral-400 hover:bg-neutral-100 hover:text-neutral-700 dark:hover:bg-zinc-800 dark:hover:text-zinc-200">
                <X className="size-4" />
              </button>
            </div>

            {setup.step === 'beginning' && (
              <div className="flex items-center gap-2 py-8 text-sm text-neutral-500 dark:text-zinc-400">
                <Loader2 className="size-4 animate-spin" />
                {t('agentChannels.preparingQr')}
              </div>
            )}

            {setup.step === 'scanning' && (
              <div className="mt-4 text-center">
                <div className="inline-flex rounded-lg bg-white p-3">
                  <QRCodeSVG value={setup.qrUrl} size={192} />
                </div>
                <p className="mt-3 text-sm text-neutral-600 dark:text-zinc-300">{t('agentChannels.scanHint', { provider: setup.provider.label })}</p>
                <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">{t('agentChannels.waitingApproval')}</p>
              </div>
            )}

            {setup.step === 'manual' && (
              <div className="mt-4 space-y-3">
                <p className="text-sm text-neutral-500 dark:text-zinc-400">{t('agentChannels.manualHint', { provider: setup.provider.label })}</p>
                {(setup.provider.fields ?? []).map(field => (
                  <label key={field.name} className="block">
                    <span className="text-xs font-medium text-neutral-600 dark:text-zinc-300">
                      {field.label}{field.required ? ' *' : ''}
                    </span>
                    <input
                      type={field.type === 'password' ? 'password' : 'text'}
                      value={setup.values[field.name] ?? ''}
                      placeholder={field.placeholder}
                      onChange={(e) => setSetup({ ...setup, values: { ...setup.values, [field.name]: e.target.value } })}
                      className="mt-1 w-full rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm outline-none focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-100"
                    />
                    {field.help ? <span className="mt-1 block text-xs text-neutral-400 dark:text-zinc-500">{field.help}</span> : null}
                  </label>
                ))}
                <div className="flex justify-end gap-2 pt-2">
                  <button type="button" onClick={closeSetup} className="rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm font-medium text-neutral-600 hover:bg-neutral-50 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:bg-zinc-800">
                    {t('common.cancel')}
                  </button>
                  <button type="button" disabled={setup.submitting} onClick={() => void submitManualSetup()} className="rounded-lg border border-sky-600 bg-white px-3 py-2 text-sm font-medium text-sky-700 hover:bg-sky-50 disabled:opacity-50 dark:border-sky-500 dark:bg-zinc-900 dark:text-sky-400 dark:hover:bg-zinc-800">
                    {setup.submitting ? t('common.loading') : t('agentChannels.connect')}
                  </button>
                </div>
              </div>
            )}

            {setup.step === 'connected' && (
              <div className="mt-4 rounded-lg bg-emerald-50 px-3 py-3 text-sm text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300">
                {t('agentChannels.connectedReady', { provider: setup.provider.label })}
              </div>
            )}

            {setup.step === 'error' && (
              <div className="mt-4 rounded-lg bg-red-50 px-3 py-3 text-sm text-red-600 dark:bg-red-950/30 dark:text-red-300">
                {setup.message}
              </div>
            )}
          </div>
        </div>
      )}

      {detail && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 px-4">
          <div className="w-full max-w-xl rounded-xl border border-neutral-200 bg-white p-5 shadow-xl dark:border-zinc-700 dark:bg-zinc-900">
            <div className="flex items-start justify-between gap-3">
              <div>
                <h3 className="text-base font-semibold text-neutral-900 dark:text-zinc-100">
                  {t('agentChannels.detailsTitle', { provider: detail.provider.label })}
                </h3>
                <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">{receiveModeHint(detail.channel.provider, t, detail.channel.callbackUrl)}</p>
              </div>
              <button type="button" onClick={() => setDetail(null)} className="rounded-md p-1.5 text-neutral-400 hover:bg-neutral-100 hover:text-neutral-700 dark:hover:bg-zinc-800 dark:hover:text-zinc-200">
                <X className="size-4" />
              </button>
            </div>
            <div className="mt-4 grid gap-2 rounded-lg border border-neutral-200/70 bg-neutral-50 p-3 dark:border-zinc-700/60 dark:bg-zinc-950/40">
              <ChannelDetail label={t('agentChannels.statusLabel')} value={t('agentChannels.connected')} />
              <ChannelDetail label={t('agentChannels.connectedByLabel')} value={detail.channel.createdBy || '-'} />
              {detail.channel.appId && <ChannelDetail label={t('agentChannels.appIdLabel')} value={detail.channel.appId} mono />}
              <ChannelDetail
                label={t('agentChannels.ownerIdLabel')}
                value={detail.channel.externalOwnerId || t('agentChannels.ownerPending')}
                mono={Boolean(detail.channel.externalOwnerId)}
              />
              <ChannelDetail
                label={t('agentChannels.botIdLabel')}
                value={detail.channel.externalBotId || t('agentChannels.botPending')}
                mono={Boolean(detail.channel.externalBotId)}
              />
              <ChannelDetail
                label={t('agentChannels.chatIdLabel')}
                value={detail.channel.externalChatId || t('agentChannels.chatPending')}
                mono={Boolean(detail.channel.externalChatId)}
              />
              <ChannelDetail label={t('agentChannels.lastActivityLabel')} value={fmtDateTime(detail.channel.lastActivityAt)} />
              <ChannelDetail label={t('agentChannels.receiveModeLabel')} value={receiveModeLabel(detail.channel.provider, t)} />
              {detail.channel.callback?.lastAt ? (
                <div className={cn('rounded-md bg-white p-2 text-xs text-neutral-500 dark:bg-zinc-900 dark:text-zinc-400', detail.channel.callback.error && 'text-red-500 dark:text-red-300')}>
                  <p className="font-medium text-neutral-700 dark:text-zinc-200">
                    {t('agentChannels.lastEvent', {
                      time: fmtDateTime(detail.channel.callback.lastAt),
                      status: t(`agentChannels.callbackStatus.${detail.channel.callback.status || 'unknown'}`),
                      reason: detail.channel.callback.reason ? t(`agentChannels.callbackReason.${detail.channel.callback.reason}`, { defaultValue: detail.channel.callback.reason }) : '-',
                    })}
                  </p>
                  {detail.channel.callback.error ? <p className="mt-1 break-words">{detail.channel.callback.error}</p> : null}
                </div>
              ) : (
                <ChannelDetail label={t('agentChannels.lastEventLabel')} value={t('agentChannels.eventPending')} />
              )}
            </div>
            <div className="mt-4 flex justify-end gap-2">
              <button type="button" onClick={() => setDetail(null)} className="rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm font-medium text-neutral-600 hover:bg-neutral-50 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:bg-zinc-800">
                {t('common.close')}
              </button>
              <button type="button" onClick={() => void disconnect(detail.channel)} className="rounded-lg border border-red-200 bg-white px-3 py-2 text-sm font-medium text-red-600 hover:bg-red-50 dark:border-red-900/70 dark:bg-zinc-900 dark:text-red-300 dark:hover:bg-red-950/30">
                {t('agentChannels.disconnect')}
              </button>
            </div>
          </div>
        </div>
      )}

    </section>
  )
}

function ChannelDetail({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return (
    <p className="flex min-w-0 items-center gap-1.5">
      <span className="shrink-0 text-neutral-500 dark:text-zinc-400">{label}</span>
      <span className={cn('min-w-0 truncate', mono && 'font-mono text-[11px] text-neutral-500 dark:text-zinc-300')}>{value}</span>
    </p>
  )
}

function receiveModeLabel(provider: string, t: (key: string, options?: Record<string, unknown>) => string) {
  if (provider === 'telegram') return t('agentChannels.pollingMode')
  if (provider === 'discord') return t('agentChannels.gatewayMode')
  if (provider === 'slack') return t('agentChannels.webhookMode')
  return t('agentChannels.websocketMode')
}

function receiveModeHint(provider: string, t: (key: string, options?: Record<string, unknown>) => string, callbackUrl?: string) {
  if (provider === 'telegram') return t('agentChannels.telegramHint')
  if (provider === 'discord') return t('agentChannels.discordHint')
  if (provider === 'slack') return t('agentChannels.slackHint', { callbackUrl: callbackUrl || '-' })
  return t('agentChannels.websocketHint')
}
