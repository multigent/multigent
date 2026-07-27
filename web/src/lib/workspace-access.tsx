import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { WORKSPACE_ID_KEY, apiFetch } from './api'
import { useAuth } from './auth'

type WorkspaceAccessSummary = {
  id?: string
  name?: string
  currentUserRole?: string
  currentUserCanAdmin?: boolean
}

type WorkspaceAccessContextValue = {
  loading: boolean
  workspace: WorkspaceAccessSummary | null
  role: string
  canAdmin: boolean
  reload: () => void
}

const WorkspaceAccessContext = createContext<WorkspaceAccessContextValue>({
  loading: true,
  workspace: null,
  role: '',
  canAdmin: false,
  reload: () => {},
})

export function WorkspaceAccessProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth()
  const [workspace, setWorkspace] = useState<WorkspaceAccessSummary | null>(null)
  const [loading, setLoading] = useState(true)
  const [reloadKey, setReloadKey] = useState(0)
  const userAccessKey = [
    user?.username ?? '',
    user?.role ?? '',
    user?.workspaceRole ?? '',
    user?.currentUserCanAdmin ? '1' : '0',
  ].join(':')

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    apiFetch<WorkspaceAccessSummary>('/api/v1/workspace')
      .then((data) => {
        if (cancelled) return
        setWorkspace(data)
        if (data?.id) {
          localStorage.setItem(WORKSPACE_ID_KEY, data.id)
        } else {
          localStorage.removeItem(WORKSPACE_ID_KEY)
        }
      })
      .catch(() => {
        if (cancelled) return
        setWorkspace(null)
        localStorage.removeItem(WORKSPACE_ID_KEY)
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => { cancelled = true }
  }, [reloadKey, userAccessKey])

  useEffect(() => {
    const reload = () => setReloadKey((k) => k + 1)
    window.addEventListener('workspace-changed', reload)
    window.addEventListener('workspace-access-denied', reload)
    return () => {
      window.removeEventListener('workspace-changed', reload)
      window.removeEventListener('workspace-access-denied', reload)
    }
  }, [])

  const value = useMemo(() => {
    const role = workspace?.currentUserRole ?? ''
    const canAdmin = workspace?.currentUserCanAdmin ?? Boolean(user && user.role === 'admin')
    return {
      loading,
      workspace,
      role,
      canAdmin,
      reload: () => setReloadKey((k) => k + 1),
    }
  }, [loading, user, workspace])

  return <WorkspaceAccessContext.Provider value={value}>{children}</WorkspaceAccessContext.Provider>
}

export function useWorkspaceAccess() {
  return useContext(WorkspaceAccessContext)
}
