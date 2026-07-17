import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useParams } from 'react-router-dom'
import { PlaceholderCard } from '../../components/ui/PlaceholderCard'
import { WorkflowBoard, type WorkflowDefinition } from '../../components/workflow/WorkflowBoard'
import { useApiJson } from '../../lib/use-api'

type WorkflowListResponse = { workflows: WorkflowDefinition[] }

export default function ProjectWorkflowsPage() {
  const { t } = useTranslation()
  const { projectId } = useParams<{ projectId: string }>()
  const path = projectId ? `/api/v1/projects/${encodeURIComponent(projectId)}/workflows` : null
  const state = useApiJson<WorkflowListResponse>(path, 0)
  const workflows = state.status === 'ok' ? state.data.workflows : []
  const [selectedId, setSelectedId] = useState('')
  const selected = useMemo(() => workflows.find((wf) => wf.id === selectedId) ?? workflows[0], [workflows, selectedId])

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <div className="shrink-0 px-6 pt-5 pb-3">
        <div className="flex items-start justify-between gap-4">
          <div>
            <h1 className="text-xl font-semibold text-neutral-900 dark:text-zinc-100">{t('projectNav.workflows')}</h1>
            <p className="mt-0.5 text-sm text-neutral-500 dark:text-zinc-500">{t('workflows.subtitle')}</p>
          </div>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto px-6 pb-6">
        {state.status === 'loading' ? (
          <div className="py-16 text-center text-sm text-neutral-500 dark:text-zinc-400">{t('api.loading')}</div>
        ) : null}
        {state.status === 'error' ? (
          <PlaceholderCard title={t('api.loadError')}>
            <p>{state.error.message}</p>
          </PlaceholderCard>
        ) : null}
        {state.status === 'ok' && selected ? (
          <div className="space-y-4">
            <div className="flex flex-wrap gap-2">
              {workflows.map((wf) => (
                <button
                  key={wf.id}
                  type="button"
                  onClick={() => setSelectedId(wf.id)}
                  className={`rounded-lg border px-3 py-2 text-sm font-medium transition-colors ${
                    selected.id === wf.id
                      ? 'border-sky-600 bg-sky-50 text-sky-700 dark:border-sky-500 dark:bg-sky-950/40 dark:text-sky-300'
                      : 'border-neutral-200 bg-white text-neutral-600 hover:bg-neutral-50 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:bg-zinc-800'
                  }`}
                >
                  {wf.name}
                </button>
              ))}
            </div>
            <section className="rounded-xl border border-neutral-200/80 bg-white p-4 shadow-sm dark:border-zinc-700/60 dark:bg-zinc-900/40">
              <div className="mb-4 flex items-start justify-between gap-4">
                <div>
                  <h2 className="text-lg font-semibold text-neutral-900 dark:text-zinc-100">{selected.name}</h2>
                  <p className="mt-1 max-w-3xl text-sm text-neutral-500 dark:text-zinc-400">{selected.description}</p>
                </div>
                <span className="rounded-full bg-neutral-100 px-2.5 py-1 text-xs text-neutral-500 dark:bg-zinc-800 dark:text-zinc-400">v{selected.version}</span>
              </div>
              <WorkflowBoard definition={selected} />
            </section>
          </div>
        ) : null}
      </div>
    </div>
  )
}

