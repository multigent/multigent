import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'

const TOKEN_KEY = 'multigent-token'
const USER_KEY = 'multigent-user'
const WORKSPACE_ID_KEY = 'multigent-workspace-id'
const SAAS_PROXY_TOKEN = 'saas-proxy'

export type ProjectAccess = {
  project: string
  role: string // viewer | operator | manager
}

export type AgentAccess = {
  project: string
  agent: string
  role: string // viewer | operator | owner
}

export type AuthUser = {
  username: string
  role: string // admin | member
  workspaceRole?: string
  currentUserCanAdmin?: boolean
  displayName?: string
  email?: string
  avatar?: string
  projects?: ProjectAccess[]
  agentGrants?: AgentAccess[]
  linkedAgents?: string[]
}

type AuthContextType = {
  token: string | null
  user: AuthUser | null
  login: (token: string, user: AuthUser) => void
  logout: () => void
}

const AuthContext = createContext<AuthContextType>({
  token: null,
  user: null,
  login: () => {},
  logout: () => {},
})

export function AuthProvider({ children }: { children: ReactNode }) {
  const [token, setToken] = useState<string | null>(() => initialToken())
  const [user, setUser] = useState<AuthUser | null>(() => {
    try {
      const raw = localStorage.getItem(USER_KEY)
      return raw ? (JSON.parse(raw) as AuthUser) : null
    } catch {
      return null
    }
  })

  const login = useCallback((t: string, u: AuthUser) => {
    localStorage.setItem(TOKEN_KEY, t)
    localStorage.setItem(USER_KEY, JSON.stringify(u))
    localStorage.removeItem(WORKSPACE_ID_KEY)
    setToken(t)
    setUser(u)
  }, [])

  useEffect(() => {
    if (!isSaaSProxyPath() || token !== SAAS_PROXY_TOKEN) return
    let cancelled = false
    fetch('/api/v1/auth/me', {
      headers: {
        Accept: 'application/json',
        Authorization: `Bearer ${SAAS_PROXY_TOKEN}`,
      },
    })
      .then((res) => res.ok ? res.json() : null)
      .then((next: AuthUser | null) => {
        if (!cancelled && next) {
          localStorage.setItem(USER_KEY, JSON.stringify(next))
          setUser(next)
        }
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [token])

  const logout = useCallback(() => {
    if (isSaaSProxyPath()) {
      void fetch('/saas/api/auth/logout', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' })
        .catch(() => null)
        .finally(() => {
          window.location.href = '/sign-up'
        })
    }
    localStorage.removeItem(TOKEN_KEY)
    localStorage.removeItem(USER_KEY)
    localStorage.removeItem(WORKSPACE_ID_KEY)
    setToken(null)
    setUser(null)
  }, [])

  useEffect(() => {
    const onStorage = (e: StorageEvent) => {
      if (e.key === TOKEN_KEY) {
        setToken(e.newValue)
        if (!e.newValue) setUser(null)
      }
    }
    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [])

  const value = useMemo(() => ({ token, user, login, logout }), [token, user, login, logout])

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  return useContext(AuthContext)
}

export function getStoredToken(): string | null {
  if (isSaaSProxyPath()) return SAAS_PROXY_TOKEN
  return localStorage.getItem(TOKEN_KEY)
}

function initialToken(): string | null {
  if (isSaaSProxyPath()) return SAAS_PROXY_TOKEN
  return localStorage.getItem(TOKEN_KEY)
}

function isSaaSProxyPath(): boolean {
  if (typeof window === 'undefined') return false
  if (!hasSaaSModeCookie()) return false
  const match = /^\/([a-z0-9][a-z0-9-]*)(\/|$)/.exec(window.location.pathname)
  return Boolean(match && !appRouteSegments.has(match[1]))
}

function hasSaaSModeCookie(): boolean {
  return typeof document !== 'undefined' && document.cookie.split(';').some((item) => item.trim() === 'mg_saas_mode=1')
}

const appRouteSegments = new Set([
  'account',
  'audit',
  'connections',
  'docs',
  'files',
  'goals',
  'invite',
  'login',
  'playbooks',
  'projects',
  'settings',
  'skills',
  'teams',
  'users',
  'workbench',
  'workflows',
])

const projectRolePower: Record<string, number> = {
  viewer: 1,
  operator: 2,
  manager: 3,
}

const agentRolePower: Record<string, number> = {
  viewer: 1,
  operator: 2,
  owner: 3,
}

export function isSystemAdmin(user: AuthUser | null | undefined): boolean {
  return Boolean(user && user.role === 'admin')
}

export function isWorkspaceAdmin(user: AuthUser | null | undefined): boolean {
  return Boolean(user?.currentUserCanAdmin || user?.workspaceRole === 'owner' || user?.workspaceRole === 'admin' || isSystemAdmin(user))
}

export function projectRole(user: AuthUser | null | undefined, project: string): string | null {
  if (isWorkspaceAdmin(user)) return 'manager'
  return user?.projects?.find((p) => p.project === project)?.role ?? null
}

export function hasLinkedAgent(user: AuthUser | null | undefined, project: string, agent: string): boolean {
  return Boolean(user?.agentGrants?.some((grant) => grant.project === project && grant.agent === agent) || user?.linkedAgents?.includes(`${project}/${agent}`))
}

export function agentRole(user: AuthUser | null | undefined, project: string, agent: string): string | null {
  if (canManageProject(user, project)) return 'owner'
  return user?.agentGrants?.find((grant) => grant.project === project && grant.agent === agent)?.role ?? (hasLinkedAgent(user, project, agent) ? 'operator' : null)
}

export function canAccessProject(user: AuthUser | null | undefined, project: string): boolean {
  if (isSystemAdmin(user)) return true
  if (projectRole(user, project) != null) return true
  return Boolean(user?.agentGrants?.some((grant) => grant.project === project) || user?.linkedAgents?.some((agent) => agent.startsWith(`${project}/`)))
}

export function canOperateProject(user: AuthUser | null | undefined, project: string): boolean {
  const role = projectRole(user, project)
  return (projectRolePower[role ?? ''] ?? 0) >= projectRolePower.operator
}

export function canManageProject(user: AuthUser | null | undefined, project: string): boolean {
  const role = projectRole(user, project)
  return (projectRolePower[role ?? ''] ?? 0) >= projectRolePower.manager
}

export function canOperateAgent(user: AuthUser | null | undefined, project: string, agent: string): boolean {
  if (canOperateProject(user, project)) return true
  const role = agentRole(user, project, agent)
  return (agentRolePower[role ?? ''] ?? 0) >= agentRolePower.operator
}

export function canConfigureAgent(user: AuthUser | null | undefined, project: string, agent: string): boolean {
  if (canManageProject(user, project)) return true
  const role = agentRole(user, project, agent)
  return (agentRolePower[role ?? ''] ?? 0) >= agentRolePower.owner
}
