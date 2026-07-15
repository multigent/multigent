import { useState, type FormEvent, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { i18n } from '../i18n'
import { apiFetch, apiPut } from '../lib/api'
import { getStoredToken, useAuth, type AuthUser } from '../lib/auth'
import type { ThemeMode } from '../theme/ThemeProvider'
import { useTheme } from '../theme/ThemeProvider'

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

function ProfileForm({ user }: { user: AuthUser }) {
  const { t } = useTranslation()
  const { login } = useAuth()
  const [displayName, setDisplayName] = useState(user.displayName ?? '')
  const [avatar, setAvatar] = useState(user.avatar ?? '')
  const [saving, setSaving] = useState(false)
  const [msg, setMsg] = useState<{ type: 'ok' | 'err'; text: string } | null>(null)

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

  return (
    <form onSubmit={handleSubmit}>
      <div className="grid gap-5 px-5 py-4 md:grid-cols-[minmax(0,1fr)_minmax(280px,360px)]">
        <div className="flex min-w-0 items-center gap-3">
          {avatar ? (
            <img src={avatar} alt="" className="size-12 shrink-0 rounded-full object-cover ring-1 ring-neutral-200 dark:ring-zinc-700" />
          ) : (
            <div className="flex size-12 shrink-0 items-center justify-center rounded-full bg-sky-600 text-base font-semibold text-white">
              {initial}
            </div>
          )}
          <div className="min-w-0">
            <div className="truncate text-sm font-semibold text-neutral-900 dark:text-zinc-100">{label}</div>
            <div className="mt-0.5 flex flex-wrap items-center gap-2 text-xs text-neutral-500 dark:text-zinc-500">
              <span className="truncate">{user.email || user.username}</span>
              <span className="rounded-full bg-neutral-100 px-2 py-0.5 font-medium text-neutral-600 dark:bg-zinc-800 dark:text-zinc-400">{user.role}</span>
            </div>
          </div>
        </div>
        <div className="space-y-3">
          <label className="block">
            <span className="mb-1 block text-xs font-medium text-neutral-600 dark:text-zinc-400">{t('users.displayName')}</span>
            <input value={displayName} onChange={(e) => setDisplayName(e.target.value)} className={inputCls} placeholder={user.username} />
          </label>
          <label className="block">
            <span className="mb-1 block text-xs font-medium text-neutral-600 dark:text-zinc-400">{t('users.avatar')}</span>
            <input value={avatar} onChange={(e) => setAvatar(e.target.value)} className={inputCls} placeholder="https://..." />
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
    </form>
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
            <ProfileForm key={`${user.username}:${user.displayName ?? ''}:${user.avatar ?? ''}`} user={user} />
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

        <section className="mt-4 overflow-hidden rounded-lg border border-neutral-200/80 bg-white dark:border-zinc-700/60 dark:bg-zinc-900/40">
          <SectionHeader title={t('auth.changePassword')} description={t('account.passwordDescription')} />
          <PasswordForm />
        </section>
      </div>
    </div>
  )
}
