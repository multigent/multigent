import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { QRCodeSVG } from 'qrcode.react'
import { CheckCircle2, Loader2, MessageSquare, PlugZap, RefreshCw, Trash2, X } from 'lucide-react'
import { apiDelete, apiFetch, apiPost } from '../../lib/api'
import { cn } from '../../lib/cn'
import { confirmDialog } from '../ui/ConfirmDialog'

type ChannelProvider = { id: 'feishu' | 'lark'; label: string }

type AgentChannel = {
  id: string
  provider: 'feishu' | 'lark'
  status: string
  connectionId?: string
  externalBotId?: string
  externalChatId?: string
  externalOwnerId?: string
  createdBy?: string
  createdAt?: string
  updatedAt?: string
  lastActivityAt?: string
}

type ChannelsResponse = {
  providers: ChannelProvider[]
  channels: AgentChannel[]
}

type SetupState =
  | { step: 'idle' }
  | { step: 'beginning'; provider: ChannelProvider }
  | { step: 'scanning'; provider: ChannelProvider; deviceCode: string; qrUrl: string; baseUrl: string; interval: number }
  | { step: 'connected'; provider: ChannelProvider }
  | { step: 'error'; provider?: ChannelProvider; message: string }

export function AgentChannelPanel({ project, agentName }: { project: string; agentName: string }) {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(true)
  const [channels, setChannels] = useState<AgentChannel[]>([])
  const [providers, setProviders] = useState<ChannelProvider[]>([
    { id: 'feishu', label: 'Feishu' },
    { id: 'lark', label: 'Lark' },
  ])
  const [setup, setSetup] = useState<SetupState>({ step: 'idle' })
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const basePath = `/api/v1/projects/${encodeURIComponent(project)}/agents/${encodeURIComponent(agentName)}/channels`

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await apiFetch<ChannelsResponse>(basePath)
      setChannels(res.channels ?? [])
      if (res.providers?.length) setProviders(res.providers)
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
                    <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">
                      {t('agentChannels.connectedBy', { user: channel.createdBy || '-' })}
                    </p>
                  ) : (
                    <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">{t('agentChannels.connectHint')}</p>
                  )}
                </div>
                {channel ? (
                  <button type="button" onClick={() => void disconnect(channel)} className="rounded-md p-1.5 text-neutral-400 hover:bg-red-50 hover:text-red-600 dark:text-zinc-500 dark:hover:bg-red-950/30 dark:hover:text-red-300" title={t('agentChannels.disconnect')}>
                    <Trash2 className="size-4" />
                  </button>
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
    </section>
  )
}
