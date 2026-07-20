import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'

type ProductTourProps = {
  workspaceId?: string
  open: boolean
  onClose: () => void
}

export function productTourStorageKey(workspaceId?: string) {
  return `multigent.product-tour.v1.${workspaceId || 'default'}`
}

export function hasSeenProductTour(workspaceId?: string) {
  return localStorage.getItem(productTourStorageKey(workspaceId)) === 'done'
}

export function markProductTourDone(workspaceId?: string) {
  localStorage.setItem(productTourStorageKey(workspaceId), 'done')
}

export default function ProductTour({ workspaceId, open, onClose }: ProductTourProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [index, setIndex] = useState(0)
  const steps = useMemo(() => [
    { title: t('productTour.steps.welcome.title'), body: t('productTour.steps.welcome.body'), path: '/' },
    { title: t('productTour.steps.models.title'), body: t('productTour.steps.models.body'), path: '/settings' },
    { title: t('productTour.steps.team.title'), body: t('productTour.steps.team.body'), path: '/teams' },
    { title: t('productTour.steps.agents.title'), body: t('productTour.steps.agents.body'), path: '/projects/hello-world-relay/members' },
    { title: t('productTour.steps.workflow.title'), body: t('productTour.steps.workflow.body'), path: '/workflows/wf-example-hello-world-relay' },
    { title: t('productTour.steps.task.title'), body: t('productTour.steps.task.body'), path: '/projects/hello-world-relay/tasks' },
    { title: t('productTour.steps.docs.title'), body: t('productTour.steps.docs.body'), path: '/docs' },
  ], [t])
  if (!open) return null

  const current = steps[index]
  const isLast = index === steps.length - 1
  const finish = () => {
    markProductTourDone(workspaceId)
    onClose()
  }
  const goCurrent = () => {
    navigate(current.path)
    if (isLast) {
      finish()
    }
  }

  return (
    <div className="fixed inset-0 z-[80] flex items-center justify-center bg-black/35 px-4 backdrop-blur-sm">
      <div className="w-full max-w-lg rounded-xl border border-neutral-200 bg-white p-5 shadow-xl dark:border-zinc-800 dark:bg-zinc-900">
        <div className="flex items-start justify-between gap-4">
          <div>
            <p className="text-xs font-medium uppercase tracking-wider text-sky-600 dark:text-sky-400">
              {t('productTour.stepCount', { current: index + 1, total: steps.length })}
            </p>
            <h2 className="mt-1 text-lg font-semibold text-neutral-900 dark:text-zinc-100">{current.title}</h2>
          </div>
          <button
            type="button"
            onClick={finish}
            className="rounded-md px-2 py-1 text-sm text-neutral-500 hover:bg-neutral-100 dark:text-zinc-400 dark:hover:bg-zinc-800"
            aria-label={t('forms.close')}
          >
            ×
          </button>
        </div>
        <p className="mt-4 text-sm leading-6 text-neutral-600 dark:text-zinc-300">{current.body}</p>
        <div className="mt-5 h-1.5 overflow-hidden rounded-full bg-neutral-100 dark:bg-zinc-800">
          <div
            className="h-full rounded-full bg-sky-600 transition-all dark:bg-sky-400"
            style={{ width: `${((index + 1) / steps.length) * 100}%` }}
          />
        </div>
        <div className="mt-5 flex flex-wrap items-center justify-between gap-3">
          <button
            type="button"
            onClick={finish}
            className="rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm font-medium text-neutral-600 hover:bg-neutral-50 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:bg-zinc-800"
          >
            {t('productTour.skip')}
          </button>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => setIndex(Math.max(0, index - 1))}
              disabled={index === 0}
              className="rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm font-medium text-neutral-600 hover:bg-neutral-50 disabled:cursor-not-allowed disabled:opacity-40 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:bg-zinc-800"
            >
              {t('productTour.previous')}
            </button>
            <button
              type="button"
              onClick={() => {
                if (isLast) finish()
                else setIndex(index + 1)
              }}
              className="rounded-lg border border-sky-600 bg-white px-3 py-2 text-sm font-medium text-sky-700 hover:bg-sky-50 dark:border-sky-500 dark:bg-zinc-900 dark:text-sky-400 dark:hover:bg-zinc-800"
            >
              {isLast ? t('productTour.finish') : t('productTour.next')}
            </button>
            <button
              type="button"
              onClick={goCurrent}
              className="rounded-lg border border-sky-600 bg-white px-3 py-2 text-sm font-medium text-sky-700 hover:bg-sky-50 dark:border-sky-500 dark:bg-zinc-900 dark:text-sky-400 dark:hover:bg-zinc-800"
            >
              {t('productTour.openStep')}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
