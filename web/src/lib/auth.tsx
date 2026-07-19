import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'

const TOKEN_KEY = 'multigent-token'
const USER_KEY = 'multigent-user'

export type ProjectAccess = {
  project: string
  role: string // viewer | operator | manager
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
  const [token, setToken] = useState<string | null>(() => localStorage.getItem(TOKEN_KEY))
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
    setToken(t)
    setUser(u)
  }, [])

  const logout = useCallback(() => {
    localStorage.removeItem(TOKEN_KEY)
    localStorage.removeItem(USER_KEY)
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
  return localStorage.getItem(TOKEN_KEY)
}

const projectRolePower: Record<string, number> = {
  viewer: 1,
  operator: 2,
  manager: 3,
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
  return Boolean(user?.linkedAgents?.includes(`${project}/${agent}`))
}

export function canAccessProject(user: AuthUser | null | undefined, project: string): boolean {
  if (isSystemAdmin(user)) return true
  if (projectRole(user, project) != null) return true
  return Boolean(user?.linkedAgents?.some((agent) => agent.startsWith(`${project}/`)))
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
  return hasLinkedAgent(user, project, agent)
}

export function canConfigureAgent(user: AuthUser | null | undefined, project: string, agent: string): boolean {
  if (canManageProject(user, project)) return true
  return hasLinkedAgent(user, project, agent)
}
