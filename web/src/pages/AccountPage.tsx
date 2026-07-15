import { useState, type FormEvent, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { i18n } from '../i18n'
import { apiPut } from '../lib/api'
import { useAuth } from '../lib/auth'
import type { ThemeMode } from '../theme/ThemeProvider'
import { useTheme } from '../theme/ThemeProvider'

const selectCls =
  'h-9 w-52 rounded-md border border-neutral-200/80 bg-white px-2.5 text-sm text-neutral-800 outline-none transition-colors focus:border-sky-400 dark:border-zinc-700/60 dark:bg-zinc-900 dark:text-zinc-200 dark:[color-scheme:dark] [&>option]:dark:bg-zinc-900 [&>option]:dark:text-zinc-200'
const inputCls =
  'h-9 min-w-0 rounded-md border border-neutral-200/80 bg-white px-2.5 text-sm text-neutral-800 outline-none transition-colors placeholder:text-neutral-400 focus:border-sky-400 dark:border-zinc-700/60 dark:bg-zinc-900 dark:text-zinc-200 dark:placeholder:text-zinc-600'

function currentLanguage(): string {
  const language = i18n.language
  if (language.startsWith('zh-TW') || language === 'zh-Hant') return 'zh-TW'
  if (language.startsWith('zh')) return 'zh-CN'
  if (language.startsWith('ja')) return 'ja'
  return 'en'
}

function PreferenceRow({ title, description, children }: { title: string; description: string; children: ReactNode }) {
  return (
    <div className="grid gap-3 border-t border-neutral-100 px-5 py-4 first:border-t-0 dark:border-zinc-800 sm:grid-cols-[1fr_auto] sm:items-center">
      <div className="min-w-0">
        <div className="text-sm font-medium text-neutral-900 dark:text-zinc-100">{title}</div>
        <p className="mt-0.5 max-w-xl text-xs leading-5 text-neutral-500 dark:text-zinc-500">{description}</p>
      </div>
      <div className="sm:justify-self-end">{children}</div>
    </div>
  )
}

function CompactPasswordForm() {
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
    <form onSubmit={handleSubmit} className="space-y-3">
      <div className="grid gap-2 sm:grid-cols-3">
        <label className="min-w-0">
          <span className="mb-1 block text-xs font-medium text-neutral-500 dark:text-zinc-500">{t('auth.oldPassword')}</span>
          <input type="password" autoComplete="current-password" value={oldPwd} onChange={(e) => setOldPwd(e.target.value)} className={inputCls} />
        </label>
        <label className="min-w-0">
          <span className="mb-1 block text-xs font-medium text-neutral-500 dark:text-zinc-500">{t('auth.newPassword')}</span>
          <input type="password" autoComplete="new-password" value={newPwd} onChange={(e) => setNewPwd(e.target.value)} className={inputCls} placeholder={t('auth.pwdMinHint')} />
        </label>
        <label className="min-w-0">
          <span className="mb-1 block text-xs font-medium text-neutral-500 dark:text-zinc-500">{t('auth.confirmPassword')}</span>
          <input type="password" autoComplete="new-password" value={confirmPwd} onChange={(e) => setConfirmPwd(e.target.value)} className={inputCls} />
        </label>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <button
          type="submit"
          disabled={saving || !oldPwd || !newPwd || !confirmPwd}
          className="h-8 rounded-md bg-sky-600 px-3 text-sm font-medium text-white transition-colors hover:bg-sky-700 disabled:opacity-50"
        >
          {saving ? t('prompt.saving') : t('auth.changePassword')}
        </button>
        {msg && (
          <p className={`text-sm ${msg.type === 'ok' ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-600 dark:text-red-400'}`}>
            {msg.text}
          </p>
        )}
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
      <div className="mx-auto max-w-4xl">
        <div className="pb-5">
          <h1 className="text-xl font-semibold text-neutral-900 dark:text-zinc-100">{t('account.title')}</h1>
          <p className="mt-0.5 text-sm text-neutral-500 dark:text-zinc-500">{t('account.intro')}</p>
        </div>

        {user && (
          <section className="mb-4 rounded-lg border border-neutral-200/80 bg-white px-5 py-4 dark:border-zinc-700/60 dark:bg-zinc-900/40">
            <div className="flex items-center gap-3">
              <div className="flex size-9 items-center justify-center rounded-full bg-sky-600 text-sm font-semibold text-white">
                {(user.username ?? 'U')[0].toUpperCase()}
              </div>
              <div className="min-w-0">
                <div className="truncate text-sm font-semibold text-neutral-900 dark:text-zinc-100">{user.username}</div>
                <div className="mt-0.5 flex items-center gap-2 text-xs text-neutral-500 dark:text-zinc-500">
                  {user.email && <span className="truncate">{user.email}</span>}
                  <span className="rounded-full bg-neutral-100 px-2 py-0.5 font-medium text-neutral-600 dark:bg-zinc-800 dark:text-zinc-400">{user.role}</span>
                </div>
              </div>
            </div>
          </section>
        )}

        <section className="overflow-hidden rounded-lg border border-neutral-200/80 bg-white dark:border-zinc-700/60 dark:bg-zinc-900/40">
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

        <section className="mt-4 rounded-lg border border-neutral-200/80 bg-white px-5 py-4 dark:border-zinc-700/60 dark:bg-zinc-900/40">
          <div className="mb-3">
            <div className="text-sm font-semibold text-neutral-900 dark:text-zinc-100">{t('auth.changePassword')}</div>
            <p className="mt-0.5 text-xs leading-5 text-neutral-500 dark:text-zinc-500">{t('account.passwordDescription')}</p>
          </div>
          <CompactPasswordForm />
        </section>
      </div>
    </div>
  )
}
