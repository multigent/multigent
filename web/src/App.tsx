import { useEffect, useState, type ReactNode } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Loader2 } from 'lucide-react'
import { AppShell } from './components/layout/AppShell'
import { useAuth } from './lib/auth'
import { ApiError, apiFetch } from './lib/api'
import { WorkspaceAccessProvider, useWorkspaceAccess } from './lib/workspace-access'
import LoginPage from './pages/LoginPage'
import WorkspaceOnboardingPage from './pages/WorkspaceOnboardingPage'
import WorkspacePage from './pages/WorkspacePage'
import WorkflowsPage from './pages/WorkflowsPage'
import PlaybooksPage from './pages/PlaybooksPage'
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
import ProjectTaskTemplatesPage from './pages/projects/ProjectTaskTemplatesPage'
import TeamDetailPage from './pages/teams/TeamDetailPage'
import TeamsPage from './pages/teams/TeamsPage'
import DocsPage from './pages/docs/DocsPage'
import FilesPage from './pages/FilesPage'
import PeoplePage from './pages/PeoplePage'
import OKRPage from './pages/OKRPage'
import ProjectMilestonePage from './pages/projects/ProjectMilestonePage'
import ProjectOKRPage from './pages/projects/ProjectOKRPage'

type WorkspaceRef = { id: string; name: string; active?: boolean }

const WORKSPACE_TRANSITION_MIN_MS = 2000
const WORKSPACE_TRANSITION_STEPS = [
  'workspace.switchingWorkspaceStepPrepare',
  'workspace.switchingWorkspaceStepAccess',
  'workspace.switchingWorkspaceStepReady',
]

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

  return (
    <WorkspaceGate>
      <WorkspaceAccessProvider>
        <WorkspaceSwitchOverlay />
        <AuthenticatedRoutes />
      </WorkspaceAccessProvider>
    </WorkspaceGate>
  )
}

function WorkspaceGate({ children }: { children: ReactNode }) {
  const { t } = useTranslation()
  const [state, setState] = useState<'loading' | 'ready' | 'empty' | 'creatingExample'>('loading')
  const [reloadKey, setReloadKey] = useState(0)

  useEffect(() => {
    let cancelled = false
    apiFetch<{ workspaces: WorkspaceRef[] }>('/api/v1/workspaces')
      .then((data) => {
        if (cancelled) return
        if ((data.workspaces ?? []).length > 0) {
          setState('ready')
          return
        }
        setState('creatingExample')
        apiFetch<WorkspaceRef>('/api/v1/workspaces/example', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
          body: '{}',
        })
          .then(() => {
            if (cancelled) return
            window.dispatchEvent(new Event('workspace-changed'))
            setState('ready')
          })
          .catch(() => {
            if (!cancelled) setState('empty')
          })
      })
      .catch((error) => {
        if (cancelled) return
        setState(error instanceof ApiError && error.status === 401 ? 'empty' : 'ready')
      })
    return () => { cancelled = true }
  }, [reloadKey])

  if (state === 'loading' || state === 'creatingExample') {
    return (
      <div className="flex min-h-screen items-center justify-center bg-white text-sm text-neutral-500 dark:bg-zinc-950 dark:text-zinc-400">
        {state === 'creatingExample' ? t('workspaceOnboarding.creatingExample') : t('app.loadingWorkspace')}
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
        <Route path="workflows" element={<WorkflowsPage />} />
        <Route path="workflows/:workflowId" element={<WorkflowsPage />} />
        {canAdmin && <Route path="playbooks" element={<PlaybooksPage />} />}
        {canAdmin && <Route path="playbooks/:playbookId" element={<PlaybooksPage />} />}
        {canAdmin && <Route path="teams/:teamId" element={<TeamDetailPage />} />}
        {canAdmin && <Route path="teams" element={<TeamsPage />} />}
        <Route path="projects" element={<ProjectsListPage />} />
        <Route path="projects/:projectId" element={<ProjectBranch />}>
          <Route index element={<Navigate to="tasks" replace />} />
          <Route path="tasks" element={<ProjectTasksPage />} />
          {canAdmin && <Route path="task-templates" element={<ProjectTaskTemplatesPage />} />}
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
        <Route path="docs/*" element={<DocsPage />} />
        <Route path="files" element={<FilesPage />} />
        <Route path="settings" element={<SettingsPage />} />
        <Route path="account" element={<AccountPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}

function WorkspaceSwitchOverlay() {
  const { t } = useTranslation()
  const [transitioning, setTransitioning] = useState(false)
  const [step, setStep] = useState(0)

  useEffect(() => {
    function start() {
      setStep(0)
      setTransitioning(true)
    }
    window.addEventListener('workspace-switch-start', start)
    return () => {
      window.removeEventListener('workspace-switch-start', start)
    }
  }, [])

  useEffect(() => {
    if (!transitioning) return
    const timers = [
      window.setTimeout(() => setStep(1), 650),
      window.setTimeout(() => setStep(2), 1300),
      window.setTimeout(() => setTransitioning(false), WORKSPACE_TRANSITION_MIN_MS),
    ]
    return () => timers.forEach((timer) => window.clearTimeout(timer))
  }, [transitioning])

  if (!transitioning) return null

  return (
    <div className="fixed inset-0 z-[90] flex items-center justify-center bg-neutral-50/90 backdrop-blur-sm dark:bg-zinc-950/90">
      <div className="flex min-w-64 flex-col items-center rounded-xl border border-neutral-200 bg-white px-6 py-5 shadow-xl dark:border-zinc-700 dark:bg-zinc-900">
        <Loader2 className="size-6 animate-spin text-sky-600 dark:text-sky-400" strokeWidth={1.8} />
        <p className="mt-3 text-sm font-medium text-neutral-900 dark:text-zinc-100">{t('workspace.switchingWorkspace')}</p>
        <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">{t(WORKSPACE_TRANSITION_STEPS[step] ?? WORKSPACE_TRANSITION_STEPS[0])}</p>
      </div>
    </div>
  )
}
