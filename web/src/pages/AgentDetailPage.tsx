import { useEffect, useState, type ReactNode } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Bot, Cable, Save, Settings2 } from 'lucide-react'
import { useApiJson } from '../lib/use-api'
import { apiPatch } from '../lib/api'
import { cn } from '../lib/cn'
import { PlaceholderCard } from '../components/ui/PlaceholderCard'
import ProjectAgentDetailPage from './projects/ProjectAgentDetailPage'
import { AgentChannelPanel } from '../components/project/AgentChannelPanel'
import { showToast } from '../components/ui/Toast'

type AgentWorker = {
  id: string
  name: string
  displayName?: string
  description?: string
  status?: string
  model?: string
  runtimeModel?: string
  defaultModelAccountId?: string
  defaultRuntimeNodeId?: string
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

type ProviderRow = {
  id: string
  name: string
  type?: string
  model?: string
}

type RuntimeNode = {
  id: string
  name: string
  status?: string
}

type RuntimeNodeListResp = { nodes?: RuntimeNode[] }

const fieldCls =
  'w-full rounded-md border border-neutral-200/80 bg-neutral-50/50 px-3 py-2 text-sm outline-none transition-colors focus:border-sky-400 dark:border-zinc-700/60 dark:bg-zinc-800/50 dark:text-zinc-200 dark:[color-scheme:dark]'
const primaryButtonCls =
  'inline-flex items-center gap-2 rounded-lg bg-sky-600 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-sky-700 disabled:opacity-50'
export default function AgentDetailPage() {
  const { t } = useTranslation()
  const { agentId } = useParams<{ agentId: string }>()
  const [reloadKey, setReloadKey] = useState(0)
  const state = useApiJson<AgentDetailResponse>(agentId ? `/api/v1/agents/${encodeURIComponent(agentId)}` : null, reloadKey, { keepPreviousDataOnReload: true })

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
    return <WorkspaceAgentDetail agent={agent} onSaved={() => setReloadKey(k => k + 1)} />
  }

  return (
    <ProjectAgentDetailPage
      projectIdOverride={membership.projectId}
      agentNameOverride={agent.name}
      workspaceAgentId={agent.id}
    />
  )
}

function WorkspaceAgentDetail({ agent, onSaved }: { agent: AgentWorker; onSaved: () => void }) {
  const { t } = useTranslation()
  const providersState = useApiJson<ProviderRow[]>('/api/v1/providers', 0, { keepPreviousDataOnReload: true })
  const nodesState = useApiJson<RuntimeNodeListResp>('/api/v1/runtime-nodes', 0, { keepPreviousDataOnReload: true })
  const [displayName, setDisplayName] = useState(agent.displayName || agent.name)
  const [description, setDescription] = useState(agent.description || '')
  const [model, setModel] = useState(agent.model || 'codex')
  const [runtimeModel, setRuntimeModel] = useState(agent.runtimeModel || '')
  const [modelAccountId, setModelAccountId] = useState(agent.defaultModelAccountId || '')
  const [runtimeNodeId, setRuntimeNodeId] = useState(agent.defaultRuntimeNodeId || '')
  const [status, setStatus] = useState(agent.status || 'active')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    setDisplayName(agent.displayName || agent.name)
    setDescription(agent.description || '')
    setModel(agent.model || 'codex')
    setRuntimeModel(agent.runtimeModel || '')
    setModelAccountId(agent.defaultModelAccountId || '')
    setRuntimeNodeId(agent.defaultRuntimeNodeId || '')
    setStatus(agent.status || 'active')
  }, [agent.id, agent.displayName, agent.name, agent.description, agent.model, agent.runtimeModel, agent.defaultModelAccountId, agent.defaultRuntimeNodeId, agent.status])

  const providers = providersState.status === 'ok' ? (providersState.data ?? []) : []
  const nodes = nodesState.status === 'ok' ? (nodesState.data.nodes ?? []) : []

  async function save() {
    setSaving(true)
    try {
      await apiPatch(`/api/v1/agents/${encodeURIComponent(agent.id)}`, {
        displayName: displayName.trim(),
        description: description.trim(),
        model: model.trim(),
        runtimeModel: runtimeModel.trim(),
        defaultModelAccountId: modelAccountId,
        defaultRuntimeNodeId: runtimeNodeId,
        status,
      })
      showToast(t('common.saved'), 'success')
      onSaved()
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <div className="shrink-0 px-6 pt-5 pb-4">
        <div className="flex items-center gap-4">
          <div className="flex size-12 shrink-0 items-center justify-center rounded-xl bg-violet-100 text-violet-700 dark:bg-violet-900/30 dark:text-violet-300">
            <Bot className="size-6" strokeWidth={1.8} />
          </div>
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <h1 className="truncate text-xl font-semibold text-neutral-900 dark:text-zinc-100">{displayName || agent.name}</h1>
              <span className="rounded-md bg-neutral-100 px-2 py-0.5 font-mono text-xs text-neutral-500 dark:bg-zinc-800 dark:text-zinc-400">{agent.name}</span>
            </div>
            <p className="mt-1 text-sm text-neutral-500 dark:text-zinc-500">{t('agents.detailSubtitle')}</p>
          </div>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto px-6 pb-6">
        <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_360px]">
          <section className="rounded-xl border border-neutral-200 bg-white p-5 dark:border-zinc-800 dark:bg-zinc-900/40">
            <SectionTitle icon={<Settings2 className="size-4" />} title={t('agentDetail.identity')} />
            <div className="mt-4 grid gap-4 md:grid-cols-2">
              <Field label={t('agents.displayName')}>
                <input value={displayName} onChange={e => setDisplayName(e.target.value)} className={fieldCls} />
              </Field>
              <Field label={t('agents.status')}>
                <select value={status} onChange={e => setStatus(e.target.value)} className={fieldCls}>
                  <option value="active">{t('agents.status_active')}</option>
                  <option value="paused">{t('agents.status_paused')}</option>
                  <option value="archived">{t('agents.status_archived')}</option>
                </select>
              </Field>
              <Field label={t('agents.model')}>
                <select value={model} onChange={e => setModel(e.target.value)} className={fieldCls}>
                  {['codex', 'claudecode', 'cursor', 'gemini', 'opencode', 'generic-cli', 'http-agent'].map(item => <option key={item} value={item}>{item}</option>)}
                </select>
              </Field>
              <Field label={t('agents.runtimeModel')}>
                <input value={runtimeModel} onChange={e => setRuntimeModel(e.target.value)} className={fieldCls} placeholder="gpt-5.5" />
              </Field>
              <Field label={t('agents.modelAccount')}>
                <select value={modelAccountId} onChange={e => setModelAccountId(e.target.value)} className={fieldCls}>
                  <option value="">{t('agents.defaultModel')}</option>
                  {providers.map(provider => (
                    <option key={provider.id} value={provider.id}>{provider.name || provider.id}</option>
                  ))}
                </select>
              </Field>
              <Field label={t('agents.runtimeNode')}>
                <select value={runtimeNodeId} onChange={e => setRuntimeNodeId(e.target.value)} className={fieldCls}>
                  <option value="">{t('agents.defaultRuntime')}</option>
                  {nodes.map(node => (
                    <option key={node.id} value={node.id}>{node.name || node.id}{node.status ? ` · ${node.status}` : ''}</option>
                  ))}
                </select>
              </Field>
              <label className="block md:col-span-2">
                <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">{t('agents.description')}</span>
                <textarea value={description} onChange={e => setDescription(e.target.value)} rows={4} className={cn(fieldCls, 'mt-1.5 resize-y')} />
              </label>
            </div>
            <div className="mt-5 flex justify-end">
              <button type="button" onClick={() => void save()} disabled={saving} className={primaryButtonCls}>
                <Save className="size-4" strokeWidth={1.8} />
                {saving ? t('common.saving') : t('common.save')}
              </button>
            </div>
          </section>

          <section className="rounded-xl border border-neutral-200 bg-white p-5 dark:border-zinc-800 dark:bg-zinc-900/40">
            <SectionTitle icon={<Cable className="size-4" />} title={t('agentDetail.connectAndChat')} />
            <p className="mt-1 text-sm text-neutral-500 dark:text-zinc-500">{t('agentDetail.connectAndChatHint')}</p>
            <div className="mt-4">
              <AgentChannelPanel project="" agentName={agent.name} agentWorkerId={agent.id} />
            </div>
          </section>
        </div>

        <p className="mt-4 text-sm text-neutral-500 dark:text-zinc-500">{t('agents.noProjectMembershipHint')}</p>
      </div>
    </div>
  )
}

function SectionTitle({ icon, title }: { icon: ReactNode; title: string }) {
  return (
    <div className="flex items-center gap-2 text-sm font-semibold text-neutral-800 dark:text-zinc-200">
      <span className="text-neutral-400 dark:text-zinc-500">{icon}</span>
      {title}
    </div>
  )
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block">
      <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">{label}</span>
      <div className="mt-1.5">{children}</div>
    </label>
  )
}
