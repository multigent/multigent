import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { QRCodeSVG } from 'qrcode.react'
import { CheckCircle2, Copy, Loader2, MessageSquare, PlugZap, RefreshCw, ShieldCheck, Trash2, X } from 'lucide-react'
import { apiDelete, apiFetch, apiPost, apiPut } from '../../lib/api'
import { cn } from '../../lib/cn'
import { confirmDialog } from '../ui/ConfirmDialog'

type ChannelProvider = { id: 'feishu' | 'lark'; label: string }

type AgentChannel = {
  id: string
  provider: 'feishu' | 'lark'
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
  | { step: 'connected'; provider: ChannelProvider }
  | { step: 'error'; provider?: ChannelProvider; message: string }

type SecurityState =
  | { open: false }
  | { open: true; channel: AgentChannel; verificationToken: string; encryptKey: string; saving: boolean; error?: string }

export function AgentChannelPanel({ project, agentName }: { project: string; agentName: string }) {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(true)
  const [channels, setChannels] = useState<AgentChannel[]>([])
  const [interaction, setInteraction] = useState<InteractionStatus | null>(null)
  const [providers, setProviders] = useState<ChannelProvider[]>([
    { id: 'feishu', label: 'Feishu' },
    { id: 'lark', label: 'Lark' },
  ])
  const [setup, setSetup] = useState<SetupState>({ step: 'idle' })
  const [security, setSecurity] = useState<SecurityState>({ open: false })
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

  async function begin(provider: ChannelProvider) {
    if (pollRef.current) clearInterval(pollRef.current)
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

  function startPoll(state: Extract<SetupState, { step: 'scanning' }>) {
    if (pollRef.current) clearInterval(pollRef.current)
    let interval = Math.max(3, state.interval)
    const tick = async () => {
      try {
        const res = await apiPost<{ status: string; baseUrl?: string; slowDown?: boolean; error?: string; channel?: AgentChannel }>(
          `${basePath}/${state.provider.id}/setup/poll`,
          { deviceCode: state.deviceCode, baseUrl: state.baseUrl || undefined },
        )
        if (res.slowDown) interval += 5
        if (res.status === 'connected' && res.channel) {
          if (pollRef.current) clearInterval(pollRef.current)
          setSetup({ step: 'connected', provider: state.provider })
          await load()
        } else if (res.status === 'denied' || res.status === 'expired' || res.status === 'error') {
          if (pollRef.current) clearInterval(pollRef.current)
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
    await load()
  }

  async function saveSecurity() {
    if (!security.open) return
    setSecurity({ ...security, saving: true, error: undefined })
    try {
      const body: { verificationToken?: string; encryptKey?: string } = {}
      if (security.verificationToken.trim()) body.verificationToken = security.verificationToken.trim()
      if (security.encryptKey.trim()) body.encryptKey = security.encryptKey.trim()
      await apiPut<AgentChannel>(`${basePath}/${security.channel.provider}/security`, body)
      setSecurity({ open: false })
      await load()
    } catch (e) {
      setSecurity({ ...security, saving: false, error: e instanceof Error ? e.message : String(e) })
    }
  }

  const byProvider = new Map(channels.map(channel => [channel.provider, channel]))

  return (
    <section>
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <MessageSquare className="size-4 text-neutral-500 dark:text-zinc-500" strokeWidth={1.8} />
          <h4 className="text-sm font-semibold text-neutral-900 dark:text-zinc-100">{t('agentChannels.title')}</h4>
        </div>
        <button type="button" onClick={() => void load()} className="rounded-md p-1.5 text-neutral-400 hover:bg-neutral-100 hover:text-neutral-700 dark:text-zinc-500 dark:hover:bg-zinc-800 dark:hover:text-zinc-200" title={t('common.refresh')}>
          <RefreshCw className={cn('size-4', loading && 'animate-spin')} />
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
                    <div className="mt-1 space-y-1 text-xs text-neutral-400 dark:text-zinc-500">
                      <p>{t('agentChannels.connectedBy', { user: channel.createdBy || '-' })}</p>
                      {channel.appId && <p>{t('agentChannels.appId', { appId: channel.appId })}</p>}
                      <p>
                        {channel.externalChatId
                          ? t('agentChannels.chatBound', { chatId: channel.externalChatId })
                          : t('agentChannels.chatPending')}
                      </p>
                      <p>{t('agentChannels.lastActivity', { time: channel.lastActivityAt ? new Date(channel.lastActivityAt).toLocaleString() : '-' })}</p>
                      {channel.callbackUrl && (
                        <div className="flex max-w-full items-center gap-1.5">
                          <span className="shrink-0">{t('agentChannels.callbackUrl')}</span>
                          <code className="min-w-0 truncate rounded bg-neutral-100 px-1.5 py-0.5 text-[11px] text-neutral-600 dark:bg-zinc-800 dark:text-zinc-300">{channel.callbackUrl}</code>
                          <button type="button" onClick={() => void navigator.clipboard?.writeText(channel.callbackUrl || '')} className="shrink-0 rounded p-1 text-neutral-400 hover:bg-neutral-100 hover:text-neutral-700 dark:hover:bg-zinc-800 dark:hover:text-zinc-200" title={t('agentChannels.copyCallback')}>
                            <Copy className="size-3.5" />
                          </button>
                        </div>
                      )}
                      <p>
                        {channel.security?.verificationTokenConfigured ? t('agentChannels.securityConfigured') : t('agentChannels.securityNotConfigured')}
                      </p>
                      {channel.callback?.lastAt ? (
                        <p className={cn(channel.callback.error && 'text-red-500 dark:text-red-300')}>
                          {t('agentChannels.lastCallback', {
                            time: new Date(channel.callback.lastAt).toLocaleString(),
                            status: t(`agentChannels.callbackStatus.${channel.callback.status || 'unknown'}`),
                            reason: channel.callback.reason ? t(`agentChannels.callbackReason.${channel.callback.reason}`, { defaultValue: channel.callback.reason }) : '-',
                          })}
                          {channel.callback.error ? ` · ${channel.callback.error}` : ''}
                        </p>
                      ) : (
                        <p>{t('agentChannels.callbackPending')}</p>
                      )}
                    </div>
                  ) : (
                    <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">{t('agentChannels.connectHint')}</p>
                  )}
                </div>
                {channel ? (
                  <div className="flex items-center gap-1">
                    <button type="button" onClick={() => setSecurity({ open: true, channel, verificationToken: '', encryptKey: '', saving: false })} className="rounded-md p-1.5 text-neutral-400 hover:bg-neutral-100 hover:text-neutral-700 dark:text-zinc-500 dark:hover:bg-zinc-800 dark:hover:text-zinc-200" title={t('agentChannels.security')}>
                      <ShieldCheck className="size-4" />
                    </button>
                    <button type="button" onClick={() => void disconnect(channel)} className="rounded-md p-1.5 text-neutral-400 hover:bg-red-50 hover:text-red-600 dark:text-zinc-500 dark:hover:bg-red-950/30 dark:hover:text-red-300" title={t('agentChannels.disconnect')}>
                      <Trash2 className="size-4" />
                    </button>
                  </div>
                ) : (
                  <button type="button" onClick={() => void begin(provider)} disabled={setup.step === 'beginning' || setup.step === 'scanning'}
                    className="inline-flex items-center gap-1.5 rounded-md bg-sky-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-sky-700 disabled:opacity-50">
                    <PlugZap className="size-3.5" />
                    {t('agentChannels.connect')}
                  </button>
                )}
              </div>
            </div>
          )
        })}
      </div>

      {(setup.step === 'beginning' || setup.step === 'scanning' || setup.step === 'connected' || setup.step === 'error') && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 px-4">
          <div className="w-full max-w-sm rounded-xl border border-neutral-200 bg-white p-5 shadow-xl dark:border-zinc-700 dark:bg-zinc-900">
            <div className="flex items-center justify-between gap-3">
              <h3 className="text-base font-semibold text-neutral-900 dark:text-zinc-100">
                {setup.step === 'error' ? t('agentChannels.setupFailed') : t('agentChannels.scanToConnect', { provider: setup.provider.label })}
              </h3>
              <button type="button" onClick={() => setSetup({ step: 'idle' })} className="rounded-md p-1.5 text-neutral-400 hover:bg-neutral-100 hover:text-neutral-700 dark:hover:bg-zinc-800 dark:hover:text-zinc-200">
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

      {security.open && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 px-4">
          <div className="w-full max-w-md rounded-xl border border-neutral-200 bg-white p-5 shadow-xl dark:border-zinc-700 dark:bg-zinc-900">
            <div className="flex items-center justify-between gap-3">
              <h3 className="text-base font-semibold text-neutral-900 dark:text-zinc-100">{t('agentChannels.security')}</h3>
              <button type="button" onClick={() => setSecurity({ open: false })} className="rounded-md p-1.5 text-neutral-400 hover:bg-neutral-100 hover:text-neutral-700 dark:hover:bg-zinc-800 dark:hover:text-zinc-200">
                <X className="size-4" />
              </button>
            </div>
            <p className="mt-1 text-xs text-neutral-500 dark:text-zinc-400">{t('agentChannels.securityHint')}</p>
            <div className="mt-4 space-y-3">
              <label className="block">
                <span className="text-xs font-medium text-neutral-600 dark:text-zinc-300">{t('agentChannels.verificationToken')}</span>
                <input
                  type="password"
                  value={security.verificationToken}
                  onChange={e => setSecurity({ ...security, verificationToken: e.target.value })}
                  placeholder={security.channel.security?.verificationTokenConfigured ? t('agentChannels.keepExisting') : ''}
                  className="mt-1 w-full rounded-md border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-100"
                />
              </label>
              <label className="block">
                <span className="text-xs font-medium text-neutral-600 dark:text-zinc-300">{t('agentChannels.encryptKey')}</span>
                <input
                  type="password"
                  value={security.encryptKey}
                  onChange={e => setSecurity({ ...security, encryptKey: e.target.value })}
                  placeholder={security.channel.security?.encryptKeyConfigured ? t('agentChannels.keepExisting') : ''}
                  className="mt-1 w-full rounded-md border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-100"
                />
              </label>
              {security.error && <div className="rounded-md bg-red-50 px-3 py-2 text-sm text-red-600 dark:bg-red-950/30 dark:text-red-300">{security.error}</div>}
            </div>
            <div className="mt-5 flex justify-end gap-2">
              <button type="button" onClick={() => setSecurity({ open: false })} className="rounded-md border border-neutral-200 px-3 py-2 text-sm text-neutral-600 hover:bg-neutral-50 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800">
                {t('common.cancel')}
              </button>
              <button type="button" onClick={() => void saveSecurity()} disabled={security.saving} className="inline-flex items-center gap-1.5 rounded-md bg-sky-600 px-3 py-2 text-sm font-medium text-white hover:bg-sky-700 disabled:opacity-50">
                {security.saving && <Loader2 className="size-3.5 animate-spin" />}
                {t('common.save')}
              </button>
            </div>
          </div>
        </div>
      )}
    </section>
  )
}
