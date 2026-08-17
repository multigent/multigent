import { useCallback, useEffect, useState, type FormEvent, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Camera, Check, Copy, ImagePlus, KeyRound, Plus, Trash2, X } from 'lucide-react'
import { i18n } from '../i18n'
import { WORKSPACE_ID_KEY, apiDelete, apiFetch, apiPost, apiPut } from '../lib/api'
import { getStoredToken, useAuth, type AuthUser } from '../lib/auth'
import { useFormatDateTime } from '../lib/format-datetime'
import type { ThemeMode } from '../theme/ThemeProvider'
import { useTheme } from '../theme/ThemeProvider'
import { confirmDialog } from '../components/ui/ConfirmDialog'

const selectCls =
  'h-9 w-52 rounded-md border border-neutral-200/80 bg-white px-2.5 text-sm text-neutral-800 outline-none transition-colors focus:border-sky-400 dark:border-zinc-700/60 dark:bg-zinc-900 dark:text-zinc-200 dark:[color-scheme:dark] [&>option]:dark:bg-zinc-900 [&>option]:dark:text-zinc-200'
const inputCls =
  'h-9 w-full min-w-0 rounded-md border border-neutral-200/80 bg-white px-2.5 text-sm text-neutral-800 outline-none transition-colors placeholder:text-neutral-400 focus:border-sky-400 dark:border-zinc-700/60 dark:bg-zinc-900 dark:text-zinc-200 dark:placeholder:text-zinc-600'

function currentLanguage(): string {
  const language = i18n.language
  if (language.startsWith('zh-TW') || language === 'zh-Hant') return 'zh-TW'
  if (language.startsWith('zh')) return 'zh-CN'
  if (language.startsWith('ja')) return 'ja'
  return 'en'
}

function PreferenceRow({ title, description, children }: { title: string; description: string; children: ReactNode }) {
  return (
    <div className="grid gap-3 border-t border-neutral-100 px-5 py-4 first:border-t-0 dark:border-zinc-800 md:grid-cols-[minmax(0,1fr)_minmax(280px,360px)] md:items-center">
      <div className="min-w-0">
        <div className="text-sm font-medium text-neutral-900 dark:text-zinc-100">{title}</div>
        <p className="mt-0.5 max-w-xl text-xs leading-5 text-neutral-500 dark:text-zinc-500">{description}</p>
      </div>
      <div className="md:justify-self-end">{children}</div>
    </div>
  )
}

function SectionHeader({ title, description }: { title: string; description: string }) {
  return (
    <div className="border-b border-neutral-100 px-5 py-4 dark:border-zinc-800">
      <div className="text-sm font-semibold text-neutral-900 dark:text-zinc-100">{title}</div>
      <p className="mt-0.5 max-w-2xl text-xs leading-5 text-neutral-500 dark:text-zinc-500">{description}</p>
    </div>
  )
}

type ClientToken = {
  id: string
  name: string
  username: string
  scopes: string[]
  createdAt?: string
  lastUsedAt?: string
  expiresAt?: string
  revokedAt?: string
}

type ClientTokenListResp = { tokens: ClientToken[] }
type ClientTokenCreateResp = { token: ClientToken; rawToken: string }

function ProfileForm({ user, workspaceRole }: { user: AuthUser; workspaceRole: string }) {
  const { t } = useTranslation()
  const { login } = useAuth()
  const [displayName, setDisplayName] = useState(user.displayName ?? '')
  const [avatar, setAvatar] = useState(user.avatar ?? '')
  const [cropSrc, setCropSrc] = useState('')
  const [cropZoom, setCropZoom] = useState(1)
  const [cropX, setCropX] = useState(0)
  const [cropY, setCropY] = useState(0)
  const [saving, setSaving] = useState(false)
  const [msg, setMsg] = useState<{ type: 'ok' | 'err'; text: string } | null>(null)

  function handleFile(file: File | undefined) {
    if (!file) return
    const reader = new FileReader()
    reader.onload = () => {
      setCropSrc(String(reader.result || ''))
      setCropZoom(1)
      setCropX(0)
      setCropY(0)
    }
    reader.readAsDataURL(file)
  }

  async function applyCrop() {
    if (!cropSrc) return
    try {
      const nextAvatar = await cropAvatar(cropSrc, cropZoom, cropX, cropY)
      setAvatar(nextAvatar)
      setCropSrc('')
    } catch (err) {
      setMsg({ type: 'err', text: err instanceof Error ? err.message : String(err) })
    }
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setMsg(null)
    setSaving(true)
    try {
      await apiPut(`/api/v1/users/${encodeURIComponent(user.username)}`, {
        displayName: displayName.trim(),
        avatar: avatar.trim(),
      })
      const updated = await apiFetch<AuthUser>('/api/v1/auth/me')
      const token = getStoredToken()
      if (token) login(token, updated)
      setMsg({ type: 'ok', text: t('account.profileSaved') })
    } catch (err) {
      setMsg({ type: 'err', text: err instanceof Error ? err.message : String(err) })
    } finally {
      setSaving(false)
    }
  }

  const label = displayName || user.displayName || user.username
  const initial = (label || 'U').slice(0, 1).toUpperCase()
  const role = workspaceRole || user.role
  const roleKey = `people.workspaceRole_${role}`
  const roleLabel = role ? t(roleKey) : ''

  return (
    <form onSubmit={handleSubmit}>
      <div className="grid gap-6 px-5 py-5 lg:grid-cols-[260px_minmax(320px,520px)]">
        <div className="flex min-w-0 flex-col items-start">
          <label className="group relative block cursor-pointer">
            <input
              type="file"
              accept="image/*"
              className="sr-only"
              onChange={(e) => {
                handleFile(e.target.files?.[0])
                e.currentTarget.value = ''
              }}
            />
            {avatar ? (
              <img src={avatar} alt="" className="size-24 rounded-full object-cover ring-1 ring-neutral-200 dark:ring-zinc-700" />
            ) : (
              <div className="flex size-24 items-center justify-center rounded-full bg-sky-600 text-3xl font-semibold text-white">
                {initial}
              </div>
            )}
            <span className="absolute inset-0 flex items-center justify-center rounded-full bg-black/0 text-white opacity-0 transition group-hover:bg-black/45 group-hover:opacity-100">
              <Camera className="size-5" strokeWidth={2} />
            </span>
          </label>
          <div className="mt-3 min-w-0">
            <div className="truncate text-sm font-semibold text-neutral-900 dark:text-zinc-100">{label}</div>
            <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-neutral-500 dark:text-zinc-500">
              <span className="truncate">{user.email || user.username}</span>
              {role && (
                <span className="rounded-full bg-neutral-100 px-2 py-0.5 font-medium text-neutral-600 dark:bg-zinc-800 dark:text-zinc-400">
                  {roleLabel === roleKey ? role : roleLabel}
                </span>
              )}
            </div>
          </div>
          <label className="mt-4 inline-flex h-8 cursor-pointer items-center gap-1.5 rounded-md border border-neutral-200 bg-white px-3 text-sm font-medium text-neutral-700 transition-colors hover:bg-neutral-50 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:bg-zinc-800">
            <ImagePlus className="size-4" strokeWidth={1.8} />
            {t('account.changeAvatar')}
            <input
              type="file"
              accept="image/*"
              className="sr-only"
              onChange={(e) => {
                handleFile(e.target.files?.[0])
                e.currentTarget.value = ''
              }}
            />
          </label>
        </div>

        <div className="space-y-4">
          <label className="block">
            <span className="mb-1.5 block text-sm font-medium text-neutral-700 dark:text-zinc-300">{t('users.displayName')}</span>
            <input value={displayName} onChange={(e) => setDisplayName(e.target.value)} className={inputCls} placeholder={user.username} />
            <p className="mt-1.5 text-xs leading-5 text-neutral-500 dark:text-zinc-500">{t('account.displayNameDescription')}</p>
          </label>
        </div>
      </div>
      <div className="flex flex-wrap items-center justify-end gap-3 border-t border-neutral-100 px-5 py-3 dark:border-zinc-800">
        {msg && (
          <p className={`text-sm ${msg.type === 'ok' ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-600 dark:text-red-400'}`}>
            {msg.text}
          </p>
        )}
        <button
          type="submit"
          disabled={saving}
          className="h-8 rounded-md bg-sky-600 px-3 text-sm font-medium text-white transition-colors hover:bg-sky-700 disabled:opacity-50"
        >
          {saving ? t('prompt.saving') : t('common.save')}
        </button>
      </div>
      {cropSrc && (
        <AvatarCropDialog
          src={cropSrc}
          zoom={cropZoom}
          offsetX={cropX}
          offsetY={cropY}
          onZoom={setCropZoom}
          onOffsetX={setCropX}
          onOffsetY={setCropY}
          onCancel={() => setCropSrc('')}
          onApply={() => void applyCrop()}
        />
      )}
    </form>
  )
}

function AvatarCropDialog({
  src,
  zoom,
  offsetX,
  offsetY,
  onZoom,
  onOffsetX,
  onOffsetY,
  onCancel,
  onApply,
}: {
  src: string
  zoom: number
  offsetX: number
  offsetY: number
  onZoom: (value: number) => void
  onOffsetX: (value: number) => void
  onOffsetY: (value: number) => void
  onCancel: () => void
  onApply: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/45 p-4" onClick={onCancel}>
      <div className="w-full max-w-md rounded-xl border border-neutral-200 bg-white shadow-xl dark:border-zinc-700 dark:bg-zinc-900" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between border-b border-neutral-100 px-5 py-4 dark:border-zinc-800">
          <div>
            <div className="text-sm font-semibold text-neutral-900 dark:text-zinc-100">{t('account.cropAvatar')}</div>
            <p className="mt-0.5 text-xs text-neutral-500 dark:text-zinc-500">{t('account.cropAvatarHint')}</p>
          </div>
          <button type="button" onClick={onCancel} className="rounded-md p-1.5 text-neutral-400 hover:bg-neutral-100 hover:text-neutral-700 dark:hover:bg-zinc-800 dark:hover:text-zinc-200">
            <X className="size-4" />
          </button>
        </div>

        <div className="px-5 py-5">
          <div className="mx-auto flex size-56 items-center justify-center overflow-hidden rounded-full bg-neutral-100 ring-1 ring-neutral-200 dark:bg-zinc-800 dark:ring-zinc-700">
            <img
              src={src}
              alt=""
              className="h-full w-full object-cover"
              style={{ transform: `translate(${offsetX}px, ${offsetY}px) scale(${zoom})` }}
            />
          </div>

          <div className="mt-5 space-y-3">
            <CropSlider label={t('account.avatarZoom')} min={1} max={2.5} step={0.05} value={zoom} onChange={onZoom} />
            <CropSlider label={t('account.avatarPositionX')} min={-80} max={80} step={1} value={offsetX} onChange={onOffsetX} />
            <CropSlider label={t('account.avatarPositionY')} min={-80} max={80} step={1} value={offsetY} onChange={onOffsetY} />
          </div>
        </div>

        <div className="flex justify-end gap-2 border-t border-neutral-100 px-5 py-3 dark:border-zinc-800">
          <button type="button" onClick={onCancel} className="h-8 rounded-md border border-neutral-200 bg-white px-3 text-sm font-medium text-neutral-700 hover:bg-neutral-50 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:bg-zinc-800">
            {t('common.cancel')}
          </button>
          <button type="button" onClick={onApply} className="h-8 rounded-md bg-sky-600 px-3 text-sm font-medium text-white hover:bg-sky-700">
            {t('account.useAvatar')}
          </button>
        </div>
      </div>
    </div>
  )
}

function CropSlider({ label, min, max, step, value, onChange }: { label: string; min: number; max: number; step: number; value: number; onChange: (value: number) => void }) {
  return (
    <label className="grid gap-2 sm:grid-cols-[90px_1fr] sm:items-center">
      <span className="text-xs font-medium text-neutral-500 dark:text-zinc-500">{label}</span>
      <input
        type="range"
        min={min}
        max={max}
        step={step}
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
        className="w-full accent-sky-600"
      />
    </label>
  )
}

function cropAvatar(src: string, zoom: number, offsetX: number, offsetY: number): Promise<string> {
  return new Promise((resolve, reject) => {
    const img = new Image()
    img.onload = () => {
      const size = 384
      const canvas = document.createElement('canvas')
      canvas.width = size
      canvas.height = size
      const ctx = canvas.getContext('2d')
      if (!ctx) {
        reject(new Error('Canvas is not available'))
        return
      }
      ctx.fillStyle = '#f4f4f5'
      ctx.fillRect(0, 0, size, size)
      const scale = Math.max(size / img.width, size / img.height) * zoom
      const width = img.width * scale
      const height = img.height * scale
      const dx = (size - width) / 2 + offsetX * (size / 224)
      const dy = (size - height) / 2 + offsetY * (size / 224)
      ctx.drawImage(img, dx, dy, width, height)
      resolve(canvas.toDataURL('image/jpeg', 0.88))
    }
    img.onerror = () => reject(new Error('Could not load image'))
    img.src = src
  })
}

function ClientTokensSection() {
  const { t } = useTranslation()
  const formatDateTime = useFormatDateTime()
  const [tokens, setTokens] = useState<ClientToken[]>([])
  const [loading, setLoading] = useState(true)
  const [creating, setCreating] = useState(false)
  const [name, setName] = useState('')
  const [createdToken, setCreatedToken] = useState<ClientTokenCreateResp | null>(null)
  const [copied, setCopied] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const data = await apiFetch<ClientTokenListResp>('/api/v1/client-tokens')
      setTokens(data.tokens ?? [])
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { void load() }, [load])

  const activeTokens = tokens.filter(token => !token.revokedAt)
  const workspaceID = localStorage.getItem(WORKSPACE_ID_KEY) || '<workspace-id>'
  const sampleCommand = createdToken ? [
    `export MULTIGENT_API_URL=${window.location.origin}`,
    `export MULTIGENT_WORKSPACE_ID=${workspaceID}`,
    `export MULTIGENT_CLIENT_TOKEN=${createdToken.rawToken}`,
    'multigent context import-session --path ~/.codex/sessions/example.jsonl --cli codex --bind-agent demo/Lina --required',
  ].join('\n') : ''

  async function copyText(text: string, key: string) {
    await navigator.clipboard?.writeText(text)
    setCopied(key)
    window.setTimeout(() => setCopied(current => current === key ? '' : current), 1600)
  }

  async function createToken() {
    const trimmed = name.trim()
    if (!trimmed) return
    setCreating(true)
    try {
      const data = await apiPost<ClientTokenCreateResp>('/api/v1/client-tokens', {
        name: trimmed,
        scopes: ['context.write'],
      })
      setCreatedToken(data)
      setName('')
      await load()
    } finally {
      setCreating(false)
    }
  }

  async function revokeToken(token: ClientToken) {
    const ok = await confirmDialog({
      title: t('account.clientTokenRevokeTitle'),
      description: t('account.clientTokenRevokeConfirm', { name: token.name }),
      confirmLabel: t('forms.delete'),
      cancelLabel: t('forms.cancel'),
      tone: 'danger',
    })
    if (!ok) return
    await apiDelete(`/api/v1/client-tokens/${encodeURIComponent(token.id)}`)
    await load()
  }

  return (
    <section className="mt-4 overflow-hidden rounded-lg border border-neutral-200/80 bg-white dark:border-zinc-700/60 dark:bg-zinc-900/40">
      <SectionHeader title={t('account.clientTokensTitle')} description={t('account.clientTokensDescription')} />
      <div className="space-y-4 px-5 py-5">
        <div className="rounded-lg border border-neutral-200/80 bg-neutral-50/40 p-4 dark:border-zinc-700/60 dark:bg-zinc-800/30">
          <div className="flex items-start gap-2">
            <KeyRound className="mt-0.5 size-4 text-neutral-500 dark:text-zinc-500" strokeWidth={1.8} />
            <div className="min-w-0 flex-1">
              <p className="text-sm font-medium text-neutral-900 dark:text-zinc-100">{t('account.createClientToken')}</p>
              <p className="mt-1 text-xs leading-5 text-neutral-500 dark:text-zinc-500">{t('account.createClientTokenDesc')}</p>
              <div className="mt-3 flex flex-col gap-2 sm:flex-row sm:items-center">
                <input
                  className="h-9 w-full max-w-md rounded-md border border-neutral-200/80 bg-white px-2.5 text-sm text-neutral-800 outline-none transition-colors placeholder:text-neutral-400 focus:border-sky-400 dark:border-zinc-700/60 dark:bg-zinc-900 dark:text-zinc-200 dark:placeholder:text-zinc-600"
                  value={name}
                  onChange={event => setName(event.target.value)}
                  placeholder={t('account.clientTokenNamePlaceholder')}
                />
                <button
                  type="button"
                  onClick={() => void createToken()}
                  disabled={creating || !name.trim()}
                  className="inline-flex h-9 items-center justify-center gap-1.5 rounded-md bg-sky-600 px-3 text-sm font-medium text-white transition-colors hover:bg-sky-700 disabled:opacity-50"
                >
                  <Plus className="size-4" />
                  {creating ? t('common.creating') : t('account.createClientTokenButton')}
                </button>
              </div>
            </div>
          </div>
        </div>

        {createdToken && (
          <div className="rounded-lg border border-emerald-200 bg-emerald-50/70 p-4 dark:border-emerald-900/50 dark:bg-emerald-950/20">
            <div className="flex items-start justify-between gap-3">
              <div>
                <p className="text-sm font-semibold text-emerald-800 dark:text-emerald-200">{t('account.clientTokenCreated')}</p>
                <p className="mt-1 text-xs leading-5 text-emerald-700/80 dark:text-emerald-300/80">{t('account.clientTokenCreatedDesc')}</p>
              </div>
              <button
                type="button"
                onClick={() => setCreatedToken(null)}
                className="rounded-md p-1 text-emerald-700 hover:bg-emerald-100 dark:text-emerald-300 dark:hover:bg-emerald-900/40"
                aria-label={t('common.close')}
              >
                <X className="size-4" />
              </button>
            </div>
            <div className="mt-3 flex items-center gap-2 rounded-md border border-emerald-200 bg-white px-3 py-2 dark:border-emerald-900/50 dark:bg-zinc-950">
              <code className="min-w-0 flex-1 truncate text-xs text-neutral-700 dark:text-zinc-200">{createdToken.rawToken}</code>
              <button
                type="button"
                onClick={() => void copyText(createdToken.rawToken, 'token')}
                className="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-neutral-500 hover:bg-neutral-100 hover:text-neutral-700 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-200"
              >
                {copied === 'token' ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
                {copied === 'token' ? t('common.copied') : t('common.copy')}
              </button>
            </div>
            <div className="mt-3 rounded-md border border-emerald-200 bg-white p-3 dark:border-emerald-900/50 dark:bg-zinc-950">
              <div className="flex items-center justify-between gap-3">
                <p className="text-xs font-medium text-neutral-500 dark:text-zinc-400">{t('account.clientTokenCommandExample')}</p>
                <button
                  type="button"
                  onClick={() => void copyText(sampleCommand, 'command')}
                  className="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-neutral-500 hover:bg-neutral-100 hover:text-neutral-700 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-200"
                >
                  {copied === 'command' ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
                  {copied === 'command' ? t('common.copied') : t('account.copyCommand')}
                </button>
              </div>
              <pre className="mt-2 overflow-x-auto whitespace-pre-wrap font-mono text-xs leading-5 text-neutral-700 dark:text-zinc-300">{sampleCommand}</pre>
            </div>
          </div>
        )}

        <div className="overflow-hidden rounded-lg border border-neutral-200/80 dark:border-zinc-700/60">
          <table className="w-full text-left">
            <thead className="bg-neutral-50 text-xs text-neutral-500 dark:bg-zinc-800/50 dark:text-zinc-400">
              <tr>
                <th className="px-4 py-2.5 font-medium">{t('account.clientTokenName')}</th>
                <th className="px-4 py-2.5 font-medium">{t('account.clientTokenScopes')}</th>
                <th className="px-4 py-2.5 font-medium">{t('account.clientTokenLastUsed')}</th>
                <th className="px-4 py-2.5 font-medium">{t('account.clientTokenCreatedAt')}</th>
                <th className="px-4 py-2.5 text-right font-medium">{t('common.actions')}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-neutral-200/80 dark:divide-zinc-700/60">
              {loading ? (
                <tr><td colSpan={5} className="px-4 py-6 text-center text-sm text-neutral-400">{t('forms.loading')}</td></tr>
              ) : activeTokens.length === 0 ? (
                <tr><td colSpan={5} className="px-4 py-6 text-center text-sm text-neutral-400">{t('account.noClientTokens')}</td></tr>
              ) : activeTokens.map(token => (
                <tr key={token.id}>
                  <td className="px-4 py-3">
                    <p className="text-sm font-medium text-neutral-900 dark:text-zinc-100">{token.name}</p>
                    <p className="mt-0.5 font-mono text-xs text-neutral-400 dark:text-zinc-500">{token.id}</p>
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex flex-wrap gap-1">
                      {(token.scopes ?? []).map(scope => (
                        <span key={scope} className="rounded-md bg-sky-50 px-2 py-0.5 font-mono text-[11px] text-sky-700 dark:bg-sky-900/20 dark:text-sky-300">{scope}</span>
                      ))}
                    </div>
                  </td>
                  <td className="px-4 py-3 text-xs text-neutral-500 dark:text-zinc-400">{token.lastUsedAt ? formatDateTime(token.lastUsedAt) : '-'}</td>
                  <td className="px-4 py-3 text-xs text-neutral-500 dark:text-zinc-400">{token.createdAt ? formatDateTime(token.createdAt) : '-'}</td>
                  <td className="px-4 py-3 text-right">
                    <button
                      type="button"
                      onClick={() => void revokeToken(token)}
                      className="inline-flex items-center gap-1 rounded-md border border-red-200 px-2.5 py-1.5 text-xs font-medium text-red-600 hover:bg-red-50 dark:border-red-900/60 dark:text-red-300 dark:hover:bg-red-950/30"
                    >
                      <Trash2 className="size-3.5" />
                      {t('account.revokeClientToken')}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </section>
  )
}

function PasswordForm() {
  const { t } = useTranslation()
  const [oldPwd, setOldPwd] = useState('')
  const [newPwd, setNewPwd] = useState('')
  const [confirmPwd, setConfirmPwd] = useState('')
  const [saving, setSaving] = useState(false)
  const [msg, setMsg] = useState<{ type: 'ok' | 'err'; text: string } | null>(null)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setMsg(null)
    if (newPwd.length < 6) {
      setMsg({ type: 'err', text: t('auth.pwdTooShort') })
      return
    }
    if (newPwd !== confirmPwd) {
      setMsg({ type: 'err', text: t('auth.pwdMismatch') })
      return
    }
    setSaving(true)
    try {
      await apiPut('/api/v1/auth/password', { oldPassword: oldPwd, newPassword: newPwd })
      setMsg({ type: 'ok', text: t('auth.pwdChanged') })
      setOldPwd('')
      setNewPwd('')
      setConfirmPwd('')
    } catch (err) {
      setMsg({ type: 'err', text: err instanceof Error ? err.message : String(err) })
    } finally {
      setSaving(false)
    }
  }

  return (
    <form onSubmit={handleSubmit}>
      <div className="divide-y divide-neutral-100 dark:divide-zinc-800">
        <PreferenceRow title={t('auth.oldPassword')} description={t('account.currentPasswordDescription')}>
          <input type="password" autoComplete="current-password" value={oldPwd} onChange={(e) => setOldPwd(e.target.value)} className={inputCls} />
        </PreferenceRow>
        <PreferenceRow title={t('auth.newPassword')} description={t('account.newPasswordDescription')}>
          <input type="password" autoComplete="new-password" value={newPwd} onChange={(e) => setNewPwd(e.target.value)} className={inputCls} placeholder={t('auth.pwdMinHint')} />
        </PreferenceRow>
        <PreferenceRow title={t('auth.confirmPassword')} description={t('account.confirmPasswordDescription')}>
          <input type="password" autoComplete="new-password" value={confirmPwd} onChange={(e) => setConfirmPwd(e.target.value)} className={inputCls} />
        </PreferenceRow>
      </div>
      <div className="flex flex-wrap items-center justify-end gap-3 border-t border-neutral-100 px-5 py-3 dark:border-zinc-800">
        {msg && (
          <p className={`text-sm ${msg.type === 'ok' ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-600 dark:text-red-400'}`}>
            {msg.text}
          </p>
        )}
        <button
          type="submit"
          disabled={saving || !oldPwd || !newPwd || !confirmPwd}
          className="h-8 rounded-md bg-sky-600 px-3 text-sm font-medium text-white transition-colors hover:bg-sky-700 disabled:opacity-50"
        >
          {saving ? t('prompt.saving') : t('auth.changePassword')}
        </button>
      </div>
    </form>
  )
}

export default function AccountPage() {
  const { t } = useTranslation()
  const { theme, setTheme } = useTheme()
  const { user } = useAuth()
  const lang = currentLanguage()
  const [workspaceRole, setWorkspaceRole] = useState('')

  useEffect(() => {
    if (!user) {
      setWorkspaceRole('')
      return
    }
    let cancelled = false
    apiFetch<{ currentUserRole?: string }>('/api/v1/workspace')
      .then((workspace) => { if (!cancelled) setWorkspaceRole(workspace.currentUserRole ?? '') })
      .catch(() => { if (!cancelled) setWorkspaceRole('') })
    return () => { cancelled = true }
  }, [user])

  return (
    <div className="animate-fade-in px-8 py-6">
      <div>
        <div className="pb-5">
          <h1 className="text-xl font-semibold text-neutral-900 dark:text-zinc-100">{t('account.title')}</h1>
          <p className="mt-0.5 text-sm text-neutral-500 dark:text-zinc-500">{t('account.intro')}</p>
        </div>

        {user && (
          <section className="mb-4 overflow-hidden rounded-lg border border-neutral-200/80 bg-white dark:border-zinc-700/60 dark:bg-zinc-900/40">
            <SectionHeader title={t('account.profileTitle')} description={t('account.profileDescription')} />
            <ProfileForm key={`${user.username}:${user.displayName ?? ''}:${user.avatar ?? ''}`} user={user} workspaceRole={workspaceRole} />
          </section>
        )}

        <section className="overflow-hidden rounded-lg border border-neutral-200/80 bg-white dark:border-zinc-700/60 dark:bg-zinc-900/40">
          <SectionHeader title={t('account.preferencesTitle')} description={t('account.preferencesDescription')} />
          <PreferenceRow
            title={t('settings.languageSection')}
            description={t('account.languageDescription')}
          >
            <select
              className={selectCls}
              value={lang}
              onChange={(e) => void i18n.changeLanguage(e.target.value)}
            >
              <option value="en">{t('language.en')}</option>
              <option value="zh-CN">{t('language.zhCN')}</option>
              <option value="zh-TW">{t('language.zhTW')}</option>
              <option value="ja">{t('language.ja')}</option>
            </select>
          </PreferenceRow>

          <PreferenceRow
            title={t('settings.appearanceSection')}
            description={t('account.appearanceDescription')}
          >
            <select
              className={selectCls}
              value={theme}
              onChange={(e) => setTheme(e.target.value as ThemeMode)}
            >
              <option value="light">{t('theme.light')}</option>
              <option value="dark">{t('theme.dark')}</option>
              <option value="system">{t('theme.system')}</option>
            </select>
          </PreferenceRow>
        </section>

        {user && <ClientTokensSection />}

        <section className="mt-4 overflow-hidden rounded-lg border border-neutral-200/80 bg-white dark:border-zinc-700/60 dark:bg-zinc-900/40">
          <SectionHeader title={t('auth.changePassword')} description={t('account.passwordDescription')} />
          <PasswordForm />
        </section>
      </div>
    </div>
  )
}
