import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { apiFetch } from './api'
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

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    apiFetch<WorkspaceAccessSummary>('/api/v1/workspace')
      .then((data) => {
        if (!cancelled) setWorkspace(data)
      })
      .catch(() => {
        if (!cancelled) setWorkspace(null)
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => { cancelled = true }
  }, [reloadKey])

  useEffect(() => {
    const reload = () => setReloadKey((k) => k + 1)
    window.addEventListener('workspace-changed', reload)
    return () => window.removeEventListener('workspace-changed', reload)
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
