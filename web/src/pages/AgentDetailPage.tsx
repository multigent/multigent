import { Link, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Bot } from 'lucide-react'
import { useApiJson } from '../lib/use-api'
import { PlaceholderCard } from '../components/ui/PlaceholderCard'
import ProjectAgentDetailPage from './projects/ProjectAgentDetailPage'

type AgentWorker = {
  id: string
  name: string
  displayName?: string
}

type ProjectMembership = {
  id: string
  projectId: string
  memberType?: string
}

type AgentDetailResponse = {
  agent?: AgentWorker
  memberships?: ProjectMembership[]
}

export default function AgentDetailPage() {
  const { t } = useTranslation()
  const { agentId } = useParams<{ agentId: string }>()
  const state = useApiJson<AgentDetailResponse>(agentId ? `/api/v1/agents/${encodeURIComponent(agentId)}` : null, 0, { keepPreviousDataOnReload: true })

  if (!agentId) return null

  if (state.status === 'loading') {
    return (
      <div className="flex h-full items-center justify-center gap-2 text-sm text-neutral-500 dark:text-zinc-400">
        <div className="size-5 animate-spin rounded-full border-2 border-neutral-300 border-t-sky-600 dark:border-zinc-600 dark:border-t-sky-400" />
        {t('api.loading')}
      </div>
    )
  }

  if (state.status === 'error') {
    return (
      <div className="p-6">
        <PlaceholderCard title={t('api.loadError')}>
          <p className="text-sm text-red-500">{state.error.message}</p>
        </PlaceholderCard>
      </div>
    )
  }

  const agent = state.data.agent
  const membership = (state.data.memberships ?? []).find(item => item.memberType === 'agent_worker') ?? state.data.memberships?.[0]

  if (!agent) {
    return (
      <div className="p-6">
        <PlaceholderCard title={t('agents.notFound')}>
          <Link to="/agents" className="text-sm font-medium text-sky-600 hover:underline dark:text-sky-400">{t('agents.backToAgents')}</Link>
        </PlaceholderCard>
      </div>
    )
  }

  if (!membership?.projectId) {
    return (
      <div className="flex h-full items-center justify-center p-6">
        <div className="max-w-md rounded-xl border border-neutral-200 bg-white p-6 text-center shadow-sm dark:border-zinc-800 dark:bg-zinc-900">
          <div className="mx-auto flex size-12 items-center justify-center rounded-xl bg-violet-100 text-violet-700 dark:bg-violet-900/30 dark:text-violet-300">
            <Bot className="size-6" strokeWidth={1.8} />
          </div>
          <h1 className="mt-4 text-base font-semibold text-neutral-900 dark:text-zinc-100">{agent.displayName || agent.name}</h1>
          <p className="mt-2 text-sm leading-6 text-neutral-500 dark:text-zinc-400">{t('agents.noProjectMembershipDetail')}</p>
          <Link to="/agents" className="mt-4 inline-flex rounded-lg border border-neutral-200 px-3 py-2 text-sm font-medium text-neutral-600 hover:bg-neutral-50 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800">
            {t('agents.backToAgents')}
          </Link>
        </div>
      </div>
    )
  }

  return (
    <ProjectAgentDetailPage
      projectIdOverride={membership.projectId}
      agentNameOverride={agent.name}
      workspaceAgentId={agent.id}
    />
  )
}
