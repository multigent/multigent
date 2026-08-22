import { Link, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useApiJson } from '../lib/use-api'
import { PlaceholderCard } from '../components/ui/PlaceholderCard'
import ProjectAgentDetailPage from './projects/ProjectAgentDetailPage'

type AgentWorker = {
  id: string
  name: string
  displayName?: string
  description?: string
  avatar?: string
  team?: string
  role?: string
  status?: string
  model?: string
  runtimeModel?: string
  defaultModelAccountId?: string
  defaultRuntimeNodeId?: string
}

type AgentDetailResponse = {
  agent?: AgentWorker
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
  if (!agent) {
    return (
      <div className="p-6">
        <PlaceholderCard title={t('agents.notFound')}>
          <Link to="/agents" className="text-sm font-medium text-sky-600 hover:underline dark:text-sky-400">{t('agents.backToAgents')}</Link>
        </PlaceholderCard>
      </div>
    )
  }

  return (
    <ProjectAgentDetailPage
      projectIdOverride=""
      agentNameOverride={agent.name}
      workspaceAgentId={agent.id}
      workspaceAgent={agent}
    />
  )
}
