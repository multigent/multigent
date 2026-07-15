import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Lock, LogIn, Mail, UserPlus } from 'lucide-react'
import { apiLoginPost, apiPublicFetch } from '../lib/api'
import { useAuth, type AuthUser } from '../lib/auth'

type LoginResponse = {
  token: string
  username: string
  role: string
  displayName?: string
  email?: string
  projects?: { project: string; role: string }[]
  linkedAgents?: string[]
}

type InvitationInfo = {
  email: string
  role: string
  displayName?: string
  status: string
  expiresAt: string
}

type Mode = 'login' | 'register' | 'invite'

export default function LoginPage() {
  const { t } = useTranslation()
  const { login } = useAuth()
  const inviteToken = useMemo(() => {
    const match = /^\/invite\/([^/]+)/.exec(window.location.pathname)
    return match ? decodeURIComponent(match[1]) : ''
  }, [])
  const [mode, setMode] = useState<Mode>(inviteToken ? 'invite' : 'login')
  const [email, setEmail] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [invite, setInvite] = useState<InvitationInfo | null>(null)

  useEffect(() => {
    if (!inviteToken) return
    setMode('invite')
    apiPublicFetch<InvitationInfo>(`/api/v1/invitations/${encodeURIComponent(inviteToken)}`)
      .then((info) => {
        setInvite(info)
        setEmail(info.email)
        setDisplayName(info.displayName ?? '')
      })
      .catch((err) => setError(err instanceof Error ? err.message : String(err)))
  }, [inviteToken])

  function finishLogin(res: LoginResponse) {
    const user: AuthUser = {
      username: res.username,
      role: res.role,
      displayName: res.displayName,
      email: res.email,
      projects: res.projects,
      linkedAgents: res.linkedAgents,
    }
    login(res.token, user)
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      if (mode === 'register') {
        const res = await apiLoginPost<LoginResponse>('/api/v1/auth/register', { email, password, displayName })
        finishLogin(res)
      } else if (mode === 'invite') {
        const res = await apiLoginPost<LoginResponse>(`/api/v1/invitations/${encodeURIComponent(inviteToken)}/accept`, { password, displayName })
        finishLogin(res)
      } else {
        const res = await apiLoginPost<LoginResponse>('/api/v1/auth/login', { username: email, password })
        finishLogin(res)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="flex min-h-dvh items-center justify-center bg-neutral-50 px-4 dark:bg-zinc-950">
      <div className="w-full max-w-sm">
        {/* Brand */}
        <div className="mb-8 text-center">
          <h1
            className="text-3xl font-bold tracking-tight text-neutral-900 dark:text-zinc-100"
            style={{ fontFamily: "'Space Grotesk', sans-serif" }}
          >
            Multi<span className="text-sky-600 dark:text-sky-400">gent</span>
          </h1>
          <p className="mt-2 text-sm text-neutral-500 dark:text-zinc-500">{t('auth.loginSubtitle')}</p>
        </div>

        {/* Form */}
        <form onSubmit={handleSubmit} className="rounded-2xl border border-neutral-200/80 bg-white p-6 shadow-sm dark:border-zinc-700/60 dark:bg-zinc-900">
          {!inviteToken && (
            <div className="mb-5 grid grid-cols-2 rounded-lg bg-neutral-100 p-1 text-sm dark:bg-zinc-800">
              <button
                type="button"
                onClick={() => { setMode('login'); setError('') }}
                className={`rounded-md px-3 py-2 font-medium transition-colors ${mode === 'login' ? 'bg-white text-neutral-900 shadow-sm dark:bg-zinc-900 dark:text-zinc-100' : 'text-neutral-500 dark:text-zinc-400'}`}
              >
                {t('auth.loginButton')}
              </button>
              <button
                type="button"
                onClick={() => { setMode('register'); setError('') }}
                className={`rounded-md px-3 py-2 font-medium transition-colors ${mode === 'register' ? 'bg-white text-neutral-900 shadow-sm dark:bg-zinc-900 dark:text-zinc-100' : 'text-neutral-500 dark:text-zinc-400'}`}
              >
                {t('auth.register')}
              </button>
            </div>
          )}
          {error && (
            <div className="mb-4 rounded-lg bg-red-50 px-3.5 py-2.5 text-sm text-red-600 dark:bg-red-900/20 dark:text-red-400">
              {error}
            </div>
          )}
          {mode === 'invite' && invite && (
            <div className="mb-4 rounded-lg bg-sky-50 px-3.5 py-2.5 text-sm text-sky-700 dark:bg-sky-900/20 dark:text-sky-300">
              {t('auth.invitedAs', { email: invite.email, role: invite.role })}
            </div>
          )}

          <label className="block">
            <span className="mb-1.5 block text-sm font-medium text-neutral-700 dark:text-zinc-300">
              {mode === 'login' ? t('auth.emailOrUsername') : t('auth.email')}
            </span>
            <input
              type={mode === 'login' ? 'text' : 'email'}
              autoComplete={mode === 'login' ? 'username' : 'email'}
              autoFocus
              value={email}
              disabled={mode === 'invite'}
              onChange={(e) => setEmail(e.target.value)}
              className="block w-full rounded-lg border border-neutral-200 bg-neutral-50/50 px-3.5 py-2.5 text-sm text-neutral-900 outline-none transition-colors placeholder:text-neutral-400 focus:border-sky-400 focus:ring-1 focus:ring-sky-400/30 dark:border-zinc-700 dark:bg-zinc-800/50 dark:text-zinc-100 dark:placeholder:text-zinc-600"
              placeholder={mode === 'login' ? 'admin or you@example.com' : 'you@example.com'}
            />
          </label>

          {(mode === 'register' || mode === 'invite') && (
            <label className="mt-4 block">
              <span className="mb-1.5 block text-sm font-medium text-neutral-700 dark:text-zinc-300">{t('auth.displayName')}</span>
              <input
                type="text"
                autoComplete="name"
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
                className="block w-full rounded-lg border border-neutral-200 bg-neutral-50/50 px-3.5 py-2.5 text-sm text-neutral-900 outline-none transition-colors placeholder:text-neutral-400 focus:border-sky-400 focus:ring-1 focus:ring-sky-400/30 dark:border-zinc-700 dark:bg-zinc-800/50 dark:text-zinc-100 dark:placeholder:text-zinc-600"
                placeholder="Alice"
              />
            </label>
          )}

          <label className="mt-4 block">
            <span className="mb-1.5 block text-sm font-medium text-neutral-700 dark:text-zinc-300">{t('auth.password')}</span>
            <input
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="block w-full rounded-lg border border-neutral-200 bg-neutral-50/50 px-3.5 py-2.5 text-sm text-neutral-900 outline-none transition-colors placeholder:text-neutral-400 focus:border-sky-400 focus:ring-1 focus:ring-sky-400/30 dark:border-zinc-700 dark:bg-zinc-800/50 dark:text-zinc-100 dark:placeholder:text-zinc-600"
              placeholder="••••••"
            />
          </label>

          <button
            type="submit"
            disabled={loading || !email.trim() || !password.trim() || (mode === 'invite' && !inviteToken)}
            className="mt-6 flex w-full items-center justify-center gap-2 rounded-lg bg-sky-600 px-4 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-sky-700 disabled:opacity-50"
          >
            {loading ? (
              <div className="size-4 animate-spin rounded-full border-2 border-white/30 border-t-white" />
            ) : mode === 'register' || mode === 'invite' ? (
              <UserPlus className="size-4" strokeWidth={2} />
            ) : (
              <LogIn className="size-4" strokeWidth={2} />
            )}
            {mode === 'register' ? t('auth.registerButton') : mode === 'invite' ? t('auth.acceptInvite') : t('auth.loginButton')}
          </button>

          <div className="mt-4 flex items-start gap-2 rounded-lg bg-amber-50/80 px-3 py-2.5 dark:bg-amber-900/10">
            {mode === 'login' ? <Lock className="mt-0.5 size-3.5 shrink-0 text-amber-500" strokeWidth={2} /> : <Mail className="mt-0.5 size-3.5 shrink-0 text-amber-500" strokeWidth={2} />}
            <p className="text-xs leading-relaxed text-amber-700 dark:text-amber-400">
              {mode === 'login' ? t('auth.defaultHint') : t('auth.localEmailHint')}
            </p>
          </div>
        </form>
      </div>
    </div>
  )
}
