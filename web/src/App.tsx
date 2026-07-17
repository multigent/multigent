import { useEffect, useState, type ReactNode } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { AppShell } from './components/layout/AppShell'
import { useAuth } from './lib/auth'
import { ApiError, apiFetch } from './lib/api'
import { WorkspaceAccessProvider, useWorkspaceAccess } from './lib/workspace-access'
import LoginPage from './pages/LoginPage'
import WorkspaceOnboardingPage from './pages/WorkspaceOnboardingPage'
import WorkspacePage from './pages/WorkspacePage'
import WorkbenchPage from './pages/WorkbenchPage'
import OverviewPage from './pages/OverviewPage'
import SettingsPage from './pages/SettingsPage'
import AccountPage from './pages/AccountPage'
import SkillsPage from './pages/SkillsPage'
import ConnectionsPage from './pages/ConnectionsPage'
import AuditPage from './pages/AuditPage'
import { ProjectBranch } from './pages/projects/ProjectBranch'
import ProjectAgentDetailPage from './pages/projects/ProjectAgentDetailPage'
import ProjectAgentChatPage from './pages/projects/ProjectAgentChatPage'
import ProjectMembersPage from './pages/projects/ProjectMembersPage'
import ProjectMessagesPage from './pages/projects/ProjectMessagesPage'
import ProjectRunsPage from './pages/projects/ProjectRunsPage'
import ProjectSchedulePage from './pages/projects/ProjectSchedulePage'
import ProjectsListPage from './pages/projects/ProjectsListPage'
import ProjectSettingsPage from './pages/projects/ProjectSettingsPage'
import ProjectTasksPage from './pages/projects/ProjectTasksPage'
import TeamDetailPage from './pages/teams/TeamDetailPage'
import TeamsPage from './pages/teams/TeamsPage'
import DocsPage from './pages/docs/DocsPage'
import FilesPage from './pages/FilesPage'
import PeoplePage from './pages/PeoplePage'
import OKRPage from './pages/OKRPage'
import ProjectMilestonePage from './pages/projects/ProjectMilestonePage'
import ProjectOKRPage from './pages/projects/ProjectOKRPage'

type WorkspaceRef = { id: string; name: string; active?: boolean }

export default function App() {
  const { token, logout } = useAuth()

  useEffect(() => {
    const onExpired = () => logout()
    window.addEventListener('auth-expired', onExpired)
    return () => window.removeEventListener('auth-expired', onExpired)
  }, [logout])

  if (!token) {
    return <LoginPage />
  }

  return <WorkspaceGate><WorkspaceAccessProvider><AuthenticatedRoutes /></WorkspaceAccessProvider></WorkspaceGate>
}

function WorkspaceGate({ children }: { children: ReactNode }) {
  const { t } = useTranslation()
  const [state, setState] = useState<'loading' | 'ready' | 'empty'>('loading')
  const [reloadKey, setReloadKey] = useState(0)

  useEffect(() => {
    let cancelled = false
    apiFetch<{ workspaces: WorkspaceRef[] }>('/api/v1/workspaces')
      .then((data) => {
        if (cancelled) return
        setState((data.workspaces ?? []).length > 0 ? 'ready' : 'empty')
      })
      .catch((error) => {
        if (cancelled) return
        setState(error instanceof ApiError && error.status === 401 ? 'empty' : 'ready')
      })
    return () => { cancelled = true }
  }, [reloadKey])

  if (state === 'loading') {
    return (
      <div className="flex min-h-screen items-center justify-center bg-white text-sm text-neutral-500 dark:bg-zinc-950 dark:text-zinc-400">
        {t('app.loadingWorkspace')}
      </div>
    )
  }
  if (state === 'empty') {
    return <WorkspaceOnboardingPage onCreated={() => setReloadKey(k => k + 1)} />
  }
  return children
}

function AuthenticatedRoutes() {
  const { t } = useTranslation()
  const { loading, canAdmin } = useWorkspaceAccess()

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-white text-sm text-neutral-500 dark:bg-zinc-950 dark:text-zinc-400">
        {t('app.loadingWorkspaceAccess')}
      </div>
    )
  }

  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route index element={canAdmin ? <OverviewPage /> : <Navigate to="workbench" replace />} />
        <Route path="workspace" element={<WorkspacePage />} />
        {canAdmin && <Route path="teams/:teamId" element={<TeamDetailPage />} />}
        {canAdmin && <Route path="teams" element={<TeamsPage />} />}
        <Route path="projects" element={<ProjectsListPage />} />
        <Route path="projects/:projectId" element={<ProjectBranch />}>
          <Route index element={<Navigate to="tasks" replace />} />
          <Route path="tasks" element={<ProjectTasksPage />} />
          <Route path="goals" element={<ProjectOKRPage />} />
          <Route path="milestones" element={<ProjectMilestonePage />} />
          <Route path="messages" element={<ProjectMessagesPage />} />
          <Route path="members" element={<ProjectMembersPage />} />
          <Route path="members/:agentName/chat" element={<ProjectAgentChatPage />} />
          <Route path="members/:agentName" element={<ProjectAgentDetailPage />} />
          {canAdmin && <Route path="schedule" element={<ProjectSchedulePage />} />}
          <Route path="runs" element={<ProjectRunsPage />} />
          {canAdmin && <Route path="settings" element={<ProjectSettingsPage />} />}
        </Route>
        <Route path="goals" element={<OKRPage />} />
        {canAdmin && <Route path="people" element={<PeoplePage />} />}
        <Route path="workbench" element={<WorkbenchPage />} />
        <Route path="connections" element={<ConnectionsPage />} />
        {canAdmin && <Route path="audit" element={<AuditPage />} />}
        {canAdmin && <Route path="skills" element={<SkillsPage />} />}
        {canAdmin && <Route path="docs/*" element={<DocsPage />} />}
        <Route path="files" element={<FilesPage />} />
        <Route path="settings" element={<SettingsPage />} />
        <Route path="account" element={<AccountPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}
