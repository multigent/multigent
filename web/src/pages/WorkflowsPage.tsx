import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useParams } from 'react-router-dom'
import { GitBranch } from 'lucide-react'
import { PlaceholderCard } from '../components/ui/PlaceholderCard'
import { WorkflowBoard, type WorkflowDefinition, type WorkflowStep } from '../components/workflow/WorkflowBoard'
import { apiPost, apiPut } from '../lib/api'
import { useFormatDateTime } from '../lib/format-datetime'
import { useApiJson } from '../lib/use-api'

type WorkflowListResponse = { workflows: WorkflowDefinition[] }

const blankStep: WorkflowStep = {
  id: 'start',
  type: 'agent_task',
  title: 'Start',
  description: '',
  actorRole: 'agent',
  position: { x: 80, y: 180 },
}

export default function WorkflowsPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const params = useParams()
  const fmt = useFormatDateTime()
  const state = useApiJson<WorkflowListResponse>('/api/v1/workflows', 0)
  const workflows = state.status === 'ok' ? state.data.workflows : []
  const selected = useMemo(() => workflows.find((wf) => wf.id === params.workflowId), [workflows, params.workflowId])

  const [draft, setDraft] = useState<WorkflowDefinition | null>(null)
  const [saving, setSaving] = useState(false)
  const [fullscreen, setFullscreen] = useState(false)

  useEffect(() => {
    setDraft(selected ? structuredClone(selected) : null)
    setFullscreen(false)
  }, [selected])

  async function createBlank() {
    setSaving(true)
    try {
      const created = await apiPost<WorkflowDefinition>('/api/v1/workflows', {
        name: t('workflows.untitledName'),
        description: '',
        startStepId: blankStep.id,
        steps: [blankStep],
        edges: [],
      })
      navigate(`/workflows/${encodeURIComponent(created.id)}`)
    } finally {
      setSaving(false)
    }
  }

  async function duplicateWorkflow(wf: WorkflowDefinition) {
    setSaving(true)
    try {
      const created = await apiPost<WorkflowDefinition>('/api/v1/workflows', {
        name: t('workflows.copyName', { name: wf.name }),
        description: wf.description,
        startStepId: wf.startStepId,
        steps: wf.steps,
        edges: wf.edges,
      })
      navigate(`/workflows/${encodeURIComponent(created.id)}`)
    } finally {
      setSaving(false)
    }
  }

  async function saveDraft() {
    if (!draft) return
    setSaving(true)
    try {
      const saved = await apiPut<WorkflowDefinition>(`/api/v1/workflows/${encodeURIComponent(draft.id)}`, {
        name: draft.name,
        description: draft.description,
        startStepId: draft.startStepId,
        steps: draft.steps,
        edges: draft.edges,
      })
      setDraft(saved)
    } finally {
      setSaving(false)
    }
  }

  if (params.workflowId) {
    return (
      <div className={fullscreen ? 'fixed inset-0 z-50 overflow-auto bg-neutral-50 px-6 py-5 dark:bg-zinc-950' : 'animate-fade-in px-8 py-6'}>
        {state.status === 'loading' && <Loading label={t('api.loading')} />}
        {state.status === 'error' && (
          <PlaceholderCard title={t('api.loadError')}>
            <p className="text-[13px]">{state.error.message}</p>
          </PlaceholderCard>
        )}
        {state.status === 'ok' && !selected && (
          <PlaceholderCard title={t('workflows.notFound')}>
            <button type="button" onClick={() => navigate('/workflows')} className="mt-3 rounded-lg border border-sky-600 bg-white px-3 py-2 text-sm font-medium text-sky-700 hover:bg-sky-50 dark:border-sky-500 dark:bg-zinc-900 dark:text-sky-400 dark:hover:bg-zinc-800">
              {t('workflows.backToList')}
            </button>
          </PlaceholderCard>
        )}
        {draft && (
          <div className="space-y-4">
            {!fullscreen && (
              <div className="flex flex-wrap items-start justify-between gap-4">
                <div className="min-w-[360px] flex-1">
                  <input
                    value={draft.name}
                    onChange={(e) => setDraft({ ...draft, name: e.target.value })}
                    className="block w-full rounded-lg border border-transparent bg-transparent px-0 text-xl font-semibold text-neutral-900 outline-none focus:border-neutral-200 focus:bg-white focus:px-3 dark:text-zinc-100 dark:focus:border-zinc-700 dark:focus:bg-zinc-900"
                  />
                  <input
                    value={draft.description || ''}
                    onChange={(e) => setDraft({ ...draft, description: e.target.value })}
                    placeholder={t('workflows.descriptionPlaceholder')}
                    className="mt-1 block w-full rounded-lg border border-transparent bg-transparent px-0 text-sm text-neutral-500 outline-none focus:border-neutral-200 focus:bg-white focus:px-3 dark:text-zinc-400 dark:focus:border-zinc-700 dark:focus:bg-zinc-900"
                  />
                </div>
                <div className="flex gap-2">
                  <button type="button" onClick={() => navigate('/workflows')} className="rounded-lg border border-neutral-300 bg-white px-3 py-2 text-sm font-medium text-neutral-600 hover:bg-neutral-50 dark:border-zinc-600 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:bg-zinc-800">
                    {t('workflows.backToList')}
                  </button>
                  <button type="button" onClick={() => void duplicateWorkflow(draft)} disabled={saving} className="rounded-lg border border-neutral-300 bg-white px-3 py-2 text-sm font-medium text-neutral-600 hover:bg-neutral-50 disabled:opacity-50 dark:border-zinc-600 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:bg-zinc-800">
                    {t('workflows.duplicate')}
                  </button>
                  <button type="button" onClick={() => void saveDraft()} disabled={saving || !draft.name.trim()} className="rounded-lg border border-sky-600 bg-white px-3 py-2 text-sm font-medium text-sky-700 hover:bg-sky-50 disabled:opacity-50 dark:border-sky-500 dark:bg-zinc-900 dark:text-sky-400 dark:hover:bg-zinc-800">
                    {saving ? t('common.saving') : t('common.save')}
                  </button>
                </div>
              </div>
            )}
            <WorkflowBoard
              definition={draft}
              editable
              fullscreen={fullscreen}
              onToggleFullscreen={() => setFullscreen((v) => !v)}
              onChange={setDraft}
            />
          </div>
        )}
      </div>
    )
  }

  return (
    <div className="animate-fade-in px-8 py-6">
      <div className="flex items-start justify-between gap-4 pb-5">
        <div>
          <h1 className="text-xl font-semibold text-neutral-900 dark:text-zinc-100">{t('nav.workflows')}</h1>
          <p className="mt-0.5 text-sm text-neutral-500 dark:text-zinc-500">{t('workflows.workspaceSubtitle')}</p>
        </div>
        <button type="button" onClick={() => void createBlank()} disabled={saving} className="rounded-lg border border-sky-600 bg-white px-3 py-2 text-sm font-medium text-sky-700 hover:bg-sky-50 disabled:opacity-50 dark:border-sky-500 dark:bg-zinc-900 dark:text-sky-400 dark:hover:bg-zinc-800">
          {t('workflows.createBlank')}
        </button>
      </div>

      {state.status === 'loading' && <Loading label={t('api.loading')} />}

      {state.status === 'error' && (
        <PlaceholderCard title={t('api.loadError')}>
          <p className="text-[13px]">{state.error.message}</p>
        </PlaceholderCard>
      )}

      {state.status === 'ok' && workflows.length === 0 && (
        <div className="flex flex-col items-center justify-center py-24 text-center">
          <div className="mb-4 flex size-16 items-center justify-center rounded-2xl bg-neutral-100 dark:bg-zinc-800/50">
            <GitBranch className="size-7 text-neutral-400 dark:text-zinc-500" strokeWidth={1.5} />
          </div>
          <p className="text-lg font-medium text-neutral-600 dark:text-zinc-400">{t('workflows.emptyTitle')}</p>
          <p className="mt-1.5 text-sm text-neutral-400 dark:text-zinc-500">{t('workflows.emptyHint')}</p>
        </div>
      )}

      {state.status === 'ok' && workflows.length > 0 && (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {workflows.map((wf) => (
            <article key={wf.id} className="rounded-xl border border-neutral-200/80 bg-white p-4 shadow-sm transition-colors hover:border-sky-200 hover:bg-sky-50/40 dark:border-zinc-700/60 dark:bg-zinc-900/40 dark:hover:border-sky-900/70 dark:hover:bg-sky-950/20">
              <button type="button" onClick={() => navigate(`/workflows/${encodeURIComponent(wf.id)}`)} className="block w-full text-left">
                <div className="flex items-start justify-between gap-3">
                  <h2 className="line-clamp-1 text-base font-semibold text-neutral-900 dark:text-zinc-100">{wf.name}</h2>
                  <span className="shrink-0 rounded-full bg-neutral-100 px-2.5 py-1 text-xs text-neutral-500 dark:bg-zinc-800 dark:text-zinc-400">v{wf.version}</span>
                </div>
                <p className="mt-2 line-clamp-2 min-h-10 text-sm text-neutral-500 dark:text-zinc-400">{wf.description || t('workflows.noDescription')}</p>
                <div className="mt-4 flex items-center justify-between text-xs text-neutral-400 dark:text-zinc-500">
                  <span>{t('workflows.stepCount', { count: wf.steps.length })}</span>
                  <span>{fmt(wf.updatedAt)}</span>
                </div>
              </button>
              <div className="mt-4 flex justify-end">
                <button type="button" onClick={() => void duplicateWorkflow(wf)} disabled={saving} className="rounded-lg border border-neutral-300 bg-white px-3 py-1.5 text-xs font-medium text-neutral-600 hover:bg-neutral-50 disabled:opacity-50 dark:border-zinc-600 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:bg-zinc-800">
                  {t('workflows.duplicate')}
                </button>
              </div>
            </article>
          ))}
        </div>
      )}
    </div>
  )
}

function Loading({ label }: { label: string }) {
  return (
    <div className="flex items-center justify-center gap-2 py-16">
      <div className="size-5 animate-spin rounded-full border-2 border-neutral-300 border-t-sky-600 dark:border-zinc-600 dark:border-t-sky-400" />
      <span className="text-sm text-neutral-500">{label}</span>
    </div>
  )
}
