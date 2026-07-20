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
  const steps = useMemo<TourStep[]>(() => {
    if (!example) {
      return [
        {
          title: t('productTour.blankSteps.welcome.title'),
          body: t('productTour.blankSteps.welcome.body'),
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
          title: t('productTour.steps.team.title'),
          body: t('productTour.blankSteps.team.body'),
          path: '/teams',
          selector: '[data-tour-nav="teams"]',
          placement: 'right',
        },
        {
          title: t('productTour.steps.workflow.title'),
          body: t('productTour.blankSteps.workflow.body'),
          path: '/workflows',
          selector: '[data-tour-nav="workflows"]',
          placement: 'right',
        },
      ]
    }
    return [
      {
        title: t('productTour.steps.welcome.title'),
        body: t('productTour.steps.welcome.body'),
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
        body: t('productTour.steps.team.body'),
        path: '/teams',
        selector: '[data-tour-nav="teams"]',
        placement: 'right',
      },
      {
        title: t('productTour.steps.teamCreate.title'),
        body: t('productTour.steps.teamCreate.body'),
        path: '/teams',
        selector: '[data-tour-team-create]',
        placement: 'bottom',
      },
      {
        title: t('productTour.steps.exampleTeam.title'),
        body: t('productTour.steps.exampleTeam.body'),
        path: '/teams',
        selector: '[data-tour-team-card="collaboration-demo"]',
        placement: 'bottom',
      },
      {
        title: t('productTour.steps.teamRoles.title'),
        body: t('productTour.steps.teamRoles.body'),
        path: '/teams/collaboration-demo',
        selector: '[data-tour-team-roles]',
        placement: 'bottom',
      },
      {
        title: t('productTour.steps.projects.title'),
        body: t('productTour.steps.projects.body'),
        path: '/projects',
        selector: '[data-tour-nav="projects"]',
        placement: 'right',
      },
      {
        title: t('productTour.steps.projectCreate.title'),
        body: t('productTour.steps.projectCreate.body'),
        path: '/projects',
        selector: '[data-tour-project-create]',
        placement: 'bottom',
      },
      {
        title: t('productTour.steps.exampleProject.title'),
        body: t('productTour.steps.exampleProject.body'),
        path: '/projects',
        selector: '[data-tour-project-card="hello-world-relay"]',
        placement: 'bottom',
      },
      {
        title: t('productTour.steps.projectMembers.title'),
        body: t('productTour.steps.projectMembers.body'),
        path: '/projects/hello-world-relay/members',
        selector: '[data-tour-project-nav="members"]',
        placement: 'bottom',
      },
      {
        title: t('productTour.steps.addAgent.title'),
        body: t('productTour.steps.addAgent.body'),
        path: '/projects/hello-world-relay/members',
        selector: '[data-tour-member-add]',
        placement: 'bottom',
      },
      {
        title: t('productTour.steps.exampleAgent.title'),
        body: t('productTour.steps.exampleAgent.body'),
        path: '/projects/hello-world-relay/members',
        selector: '[data-tour-member-card="greeter-agent"]',
        placement: 'bottom',
      },
      {
        title: t('productTour.steps.agentModelConfig.title'),
        body: t('productTour.steps.agentModelConfig.body'),
        path: '/projects/hello-world-relay/members/greeter-agent',
        selector: '[data-tour-agent-model-config]',
        placement: 'bottom',
      },
      {
        title: t('productTour.steps.agentWakeupPrompt.title'),
        body: t('productTour.steps.agentWakeupPrompt.body'),
        path: '/projects/hello-world-relay/members/greeter-agent',
        selector: '[data-tour-agent-wakeup-prompt]',
        placement: 'bottom',
      },
      {
        title: t('productTour.steps.workflow.title'),
        body: t('productTour.steps.workflow.body'),
        path: '/workflows',
        selector: '[data-tour-nav="workflows"]',
        placement: 'right',
      },
      {
        title: t('productTour.steps.workflowCreate.title'),
        body: t('productTour.steps.workflowCreate.body'),
        path: '/workflows',
        selector: '[data-tour-workflow-create]',
        placement: 'bottom',
      },
      {
        title: t('productTour.steps.exampleWorkflow.title'),
        body: t('productTour.steps.exampleWorkflow.body'),
        path: '/workflows',
        selector: '[data-tour-workflow-card="wf-example-hello-world-relay"]',
        placement: 'bottom',
      },
      {
        title: t('productTour.steps.workflowBoard.title'),
        body: t('productTour.steps.workflowBoard.body'),
        path: '/workflows/wf-example-hello-world-relay',
        selector: '[data-tour-workflow-board]',
        placement: 'bottom',
      },
      {
        title: t('productTour.steps.taskCreate.title'),
        body: t('productTour.steps.taskCreate.body'),
        path: '/projects/hello-world-relay/tasks',
        selector: '[data-tour-task-create]',
        placement: 'bottom',
      },
      {
        title: t('productTour.steps.scheduleControl.title'),
        body: t('productTour.steps.scheduleControl.body'),
        path: '/projects/hello-world-relay/schedule',
        selector: '[data-tour-scheduler-control]',
        placement: 'bottom',
      },
      {
        title: t('productTour.steps.runtimeSchedule.title'),
        body: t('productTour.steps.runtimeSchedule.body'),
        path: '/projects/hello-world-relay/schedule',
        selector: '[data-tour-runtime-table]',
        placement: 'bottom',
      },
      {
        title: t('productTour.steps.docs.title'),
        body: t('productTour.steps.docs.body'),
        path: '/docs',
        selector: '[data-tour-nav="docs"]',
        placement: 'right',
      },
      {
        title: t('productTour.steps.docsAdd.title'),
        body: t('productTour.steps.docsAdd.body'),
        path: '/docs',
        selector: '[data-tour-doc-add]',
        placement: 'bottom',
      },
      {
        title: t('productTour.steps.finish.title'),
        body: t('productTour.steps.finish.body'),
        path: '/',
        placement: 'floating',
      },
    ]
  }, [t, example])

  const current = steps[index]
  const isLast = index === steps.length - 1

  useEffect(() => {
    if (open) setIndex(0)
  }, [open])

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
    const retry = window.setInterval(updateTarget, 250)
    const stopRetry = window.setTimeout(() => window.clearInterval(retry), 1500)
    window.addEventListener('resize', updateTarget)
    window.addEventListener('scroll', updateTarget, true)
    return () => {
      window.clearTimeout(timer)
      window.clearTimeout(stopRetry)
      window.clearInterval(retry)
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
