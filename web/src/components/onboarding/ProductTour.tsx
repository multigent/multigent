import { useEffect, useMemo, useState, type CSSProperties } from 'react'
import { useTranslation } from 'react-i18next'
import { useLocation, useNavigate } from 'react-router-dom'

type ProductTourProps = {
  workspaceId?: string
  example?: boolean
  open: boolean
  onClose: () => void
}

type TourStep = {
  title: string
  body: string
  path?: string
  selector?: string
  placement?: 'right' | 'bottom' | 'floating'
}

type TargetRect = {
  top: number
  left: number
  width: number
  height: number
}

export function productTourStorageKey(workspaceId?: string) {
  return `multigent.product-tour.v3.${workspaceId || 'default'}`
}

export function hasSeenProductTour(workspaceId?: string) {
  return localStorage.getItem(productTourStorageKey(workspaceId)) === 'done'
}

export function markProductTourDone(workspaceId?: string) {
  localStorage.setItem(productTourStorageKey(workspaceId), 'done')
}

export default function ProductTour({ workspaceId, example = false, open, onClose }: ProductTourProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { pathname } = useLocation()
  const [index, setIndex] = useState(0)
  const [targetRect, setTargetRect] = useState<TargetRect | null>(null)
  const steps = useMemo<TourStep[]>(() => [
    {
      title: t(example ? 'productTour.steps.welcome.title' : 'productTour.blankSteps.welcome.title'),
      body: t(example ? 'productTour.steps.welcome.body' : 'productTour.blankSteps.welcome.body'),
      path: '/',
      placement: 'floating',
    },
    {
      title: t('productTour.steps.models.title'),
      body: t('productTour.steps.models.body'),
      path: '/settings',
      selector: '[data-tour-nav="settings"]',
      placement: 'right',
    },
    {
      title: t('productTour.steps.modelAccounts.title'),
      body: t('productTour.steps.modelAccounts.body'),
      path: '/settings',
      selector: '[data-tour-provider-section]',
      placement: 'bottom',
    },
    {
      title: t('productTour.steps.addModelAccount.title'),
      body: t('productTour.steps.addModelAccount.body'),
      path: '/settings',
      selector: '[data-tour-provider-add]',
      placement: 'bottom',
    },
    {
      title: t('productTour.steps.team.title'),
      body: t(example ? 'productTour.steps.team.body' : 'productTour.blankSteps.team.body'),
      path: '/teams',
      selector: '[data-tour-nav="teams"]',
      placement: 'right',
    },
    {
      title: t('productTour.steps.agents.title'),
      body: t(example ? 'productTour.steps.agents.body' : 'productTour.blankSteps.agents.body'),
      path: example ? '/projects/hello-world-relay/members' : '/projects',
      selector: '[data-tour-nav="projects"]',
      placement: 'right',
    },
    {
      title: t('productTour.steps.workflow.title'),
      body: t(example ? 'productTour.steps.workflow.body' : 'productTour.blankSteps.workflow.body'),
      path: example ? '/workflows/wf-example-hello-world-relay' : '/workflows',
      selector: '[data-tour-nav="workflows"]',
      placement: 'right',
    },
    {
      title: t('productTour.steps.task.title'),
      body: t(example ? 'productTour.steps.task.body' : 'productTour.blankSteps.task.body'),
      path: example ? '/projects/hello-world-relay/tasks' : '/projects',
      selector: '[data-tour-nav="workbench"]',
      placement: 'right',
    },
    {
      title: t('productTour.steps.docs.title'),
      body: t('productTour.steps.docs.body'),
      path: '/docs',
      selector: '[data-tour-nav="docs"]',
      placement: 'right',
    },
  ], [t, example])

  const current = steps[index]
  const isLast = index === steps.length - 1

  useEffect(() => {
    if (!open) return undefined
    function updateTarget() {
      if (!current?.selector) {
        setTargetRect(null)
        return
      }
      const el = document.querySelector(current.selector)
      if (!el) {
        setTargetRect(null)
        return
      }
      const rect = el.getBoundingClientRect()
      setTargetRect({
        top: rect.top,
        left: rect.left,
        width: rect.width,
        height: rect.height,
      })
    }
    const el = current?.selector ? document.querySelector(current.selector) : null
    el?.scrollIntoView({ block: 'center', inline: 'nearest', behavior: 'smooth' })
    const timer = window.setTimeout(updateTarget, 180)
    window.addEventListener('resize', updateTarget)
    window.addEventListener('scroll', updateTarget, true)
    return () => {
      window.clearTimeout(timer)
      window.removeEventListener('resize', updateTarget)
      window.removeEventListener('scroll', updateTarget, true)
    }
  }, [current?.selector, open, pathname])

  if (!open) return null

  const finish = () => {
    markProductTourDone(workspaceId)
    onClose()
  }
  const goStep = (nextIndex: number) => {
    const next = steps[nextIndex]
    if (next?.path && next.path !== pathname) {
      navigate(next.path)
    }
    setIndex(nextIndex)
  }
  const highlightStyle = targetRect
    ? {
        top: Math.max(8, targetRect.top - 6),
        left: Math.max(8, targetRect.left - 6),
        width: targetRect.width + 12,
        height: targetRect.height + 12,
      }
    : null

  return (
    <div className="pointer-events-none fixed inset-0 z-[80]">
      {highlightStyle ? (
        <div
          className="absolute rounded-xl border-2 border-sky-500 bg-sky-400/10 shadow-[0_0_0_6px_rgba(14,165,233,0.14)] transition-all duration-200"
          style={highlightStyle}
        />
      ) : null}
      <div
        className="pointer-events-auto absolute w-[min(25rem,calc(100vw-2rem))] rounded-xl border border-sky-300 bg-sky-950 p-5 text-white shadow-2xl shadow-sky-950/20 transition-all duration-200 dark:border-sky-500/50 dark:bg-zinc-50 dark:text-zinc-950"
        style={floatingCardStyle(targetRect, current.placement)}
      >
        <div className="flex items-start justify-between gap-4">
          <div>
            <p className="text-xs font-medium uppercase tracking-wider text-sky-600 dark:text-sky-400">
              {t('productTour.stepCount', { current: index + 1, total: steps.length })}
            </p>
            <h2 className="mt-1 text-lg font-semibold">{current.title}</h2>
          </div>
          <button
            type="button"
            onClick={finish}
            className="rounded-md px-2 py-1 text-sm text-sky-100 hover:bg-white/10 dark:text-zinc-500 dark:hover:bg-zinc-200/70"
            aria-label={t('forms.close')}
          >
            ×
          </button>
        </div>
        <p className="mt-4 text-sm leading-6 text-sky-50 dark:text-zinc-700">{current.body}</p>
        <div className="mt-5 h-1.5 overflow-hidden rounded-full bg-white/15 dark:bg-zinc-200">
          <div
            className="h-full rounded-full bg-white transition-all dark:bg-sky-600"
            style={{ width: `${((index + 1) / steps.length) * 100}%` }}
          />
        </div>
        <div className="mt-5 flex flex-wrap items-center justify-between gap-3">
          <button
            type="button"
            onClick={finish}
            className="rounded-lg border border-white/20 bg-transparent px-3 py-2 text-sm font-medium text-sky-50 hover:bg-white/10 dark:border-zinc-300 dark:text-zinc-600 dark:hover:bg-zinc-100"
          >
            {t('productTour.skip')}
          </button>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => goStep(Math.max(0, index - 1))}
              disabled={index === 0}
              className="rounded-lg border border-white/20 bg-transparent px-3 py-2 text-sm font-medium text-sky-50 hover:bg-white/10 disabled:cursor-not-allowed disabled:opacity-40 dark:border-zinc-300 dark:text-zinc-600 dark:hover:bg-zinc-100"
            >
              {t('productTour.previous')}
            </button>
            <button
              type="button"
              onClick={() => {
                if (isLast) finish()
                else goStep(index + 1)
              }}
              className="rounded-lg border border-white bg-white px-3 py-2 text-sm font-medium text-sky-800 hover:bg-sky-50 dark:border-sky-600 dark:bg-sky-600 dark:text-white dark:hover:bg-sky-700"
            >
              {isLast ? t('productTour.finish') : t('productTour.next')}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

function floatingCardStyle(rect: TargetRect | null, placement: TourStep['placement']): CSSProperties {
  const margin = 14
  const cardWidth = Math.min(400, window.innerWidth - 32)
  const cardHeightEstimate = 260
  if (!rect || placement === 'floating') {
    return { top: 72, right: 24 }
  }

  let top = rect.top + rect.height + margin
  let left = rect.left
  if (placement === 'right') {
    top = rect.top + rect.height / 2 - cardHeightEstimate / 2
    left = rect.left + rect.width + margin
  }
  top = clamp(top, 16, Math.max(16, window.innerHeight - cardHeightEstimate - 16))
  left = clamp(left, 16, Math.max(16, window.innerWidth - cardWidth - 16))
  return { top, left }
}

function clamp(value: number, min: number, max: number) {
  return Math.max(min, Math.min(max, value))
}
