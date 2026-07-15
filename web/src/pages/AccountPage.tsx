import { useTranslation } from 'react-i18next'
import { i18n } from '../i18n'
import { useAuth } from '../lib/auth'
import type { ThemeMode } from '../theme/ThemeProvider'
import { useTheme } from '../theme/ThemeProvider'
import { ChangePasswordSection } from './SettingsPage'

const selectCls =
  'max-w-xs rounded-md border border-neutral-200/80 bg-neutral-50/50 px-3 py-2 text-sm text-neutral-800 outline-none transition-colors focus:border-sky-400 dark:border-zinc-700/60 dark:bg-zinc-800 dark:text-zinc-200 dark:[color-scheme:dark] [&>option]:dark:bg-zinc-800 [&>option]:dark:text-zinc-200'

function currentLanguage(): string {
  const language = i18n.language
  if (language.startsWith('zh-TW') || language === 'zh-Hant') return 'zh-TW'
  if (language.startsWith('zh')) return 'zh-CN'
  if (language.startsWith('ja')) return 'ja'
  return 'en'
}

export default function AccountPage() {
  const { t } = useTranslation()
  const { theme, setTheme } = useTheme()
  const { user } = useAuth()
  const lang = currentLanguage()

  return (
    <div className="animate-fade-in px-8 py-6">
      <div className="pb-5">
        <h1 className="text-xl font-semibold text-neutral-900 dark:text-zinc-100">{t('account.title')}</h1>
        <p className="mt-0.5 text-sm text-neutral-500 dark:text-zinc-500">{t('account.intro')}</p>
        {user && (
          <p className="mt-1.5 text-sm text-neutral-400 dark:text-zinc-500">
            {t('auth.loggedInAs')} <span className="font-medium text-neutral-700 dark:text-zinc-300">{user.username}</span>
            <span className="ml-2 rounded-full bg-sky-100 px-2 py-0.5 text-xs font-medium text-sky-700 dark:bg-sky-900/30 dark:text-sky-400">{user.role}</span>
          </p>
        )}
      </div>

      <div className="space-y-5">
        <section className="rounded-xl border border-neutral-200/80 bg-white p-5 dark:border-zinc-700/60 dark:bg-zinc-900/40">
          <h3 className="text-base font-semibold text-neutral-900 dark:text-zinc-100">
            {t('settings.languageSection')}
          </h3>
          <label className="mt-3 flex flex-col gap-1.5">
            <span className="text-sm font-medium text-neutral-600 dark:text-zinc-400">{t('language.label')}</span>
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
          </label>
        </section>

        <section className="rounded-xl border border-neutral-200/80 bg-white p-5 dark:border-zinc-700/60 dark:bg-zinc-900/40">
          <h3 className="text-base font-semibold text-neutral-900 dark:text-zinc-100">
            {t('settings.appearanceSection')}
          </h3>
          <label className="mt-3 flex flex-col gap-1.5">
            <span className="text-sm font-medium text-neutral-600 dark:text-zinc-400">{t('theme.appearance')}</span>
            <select
              className={selectCls}
              value={theme}
              onChange={(e) => setTheme(e.target.value as ThemeMode)}
            >
              <option value="light">{t('theme.light')}</option>
              <option value="dark">{t('theme.dark')}</option>
              <option value="system">{t('theme.system')}</option>
            </select>
          </label>
          <p className="mt-3 text-sm text-neutral-400 dark:text-zinc-500">{t('settings.themeHint')}</p>
        </section>

        <ChangePasswordSection />
      </div>
    </div>
  )
}
