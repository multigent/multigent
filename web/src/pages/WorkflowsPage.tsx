import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { GitBranch } from 'lucide-react'
import { PlaceholderCard } from '../components/ui/PlaceholderCard'
import { WorkflowBoard, type WorkflowDefinition } from '../components/workflow/WorkflowBoard'
import { useApiJson } from '../lib/use-api'

type WorkflowListResponse = { workflows: WorkflowDefinition[] }

export default function WorkflowsPage() {
  const { t } = useTranslation()
  const state = useApiJson<WorkflowListResponse>('/api/v1/workflows', 0)
  const workflows = state.status === 'ok' ? state.data.workflows : []
  const [selectedId, setSelectedId] = useState('')
  const selected = useMemo(() => workflows.find((wf) => wf.id === selectedId) ?? workflows[0], [workflows, selectedId])

  return (
    <div className="animate-fade-in px-8 py-6">
      <div className="pb-5">
        <h1 className="text-xl font-semibold text-neutral-900 dark:text-zinc-100">{t('nav.workflows')}</h1>
        <p className="mt-0.5 text-sm text-neutral-500 dark:text-zinc-500">{t('workflows.workspaceSubtitle')}</p>
      </div>

      {state.status === 'loading' && (
        <div className="flex items-center justify-center gap-2 py-16">
          <div className="size-5 animate-spin rounded-full border-2 border-neutral-300 border-t-sky-600 dark:border-zinc-600 dark:border-t-sky-400" />
          <span className="text-sm text-neutral-500">{t('api.loading')}</span>
        </div>
      )}

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

      {state.status === 'ok' && selected && (
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
      )}
    </div>
  )
}
