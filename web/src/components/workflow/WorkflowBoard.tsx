import { useMemo, useState } from 'react'
import { cn } from '../../lib/cn'

export type WorkflowPosition = { x: number; y: number }

export type WorkflowStep = {
  id: string
  type: string
  title: string
  description?: string
  actorRole?: string
  inputSchema?: string
  outputSchema?: string
  reviewPolicy?: string
  position: WorkflowPosition
}

export type WorkflowEdge = {
  id: string
  from: string
  to: string
  label?: string
  policy?: string
}

export type WorkflowDefinition = {
  id: string
  name: string
  description?: string
  version: number
  startStepId: string
  steps: WorkflowStep[]
  edges: WorkflowEdge[]
}

export type WorkflowRun = {
  id: string
  definitionId: string
  project: string
  taskId: string
  status: string
  activeStepId?: string
  startedAt: string
  updatedAt: string
  finishedAt?: string
}

export type WorkflowStepInstance = {
  id: string
  runId: string
  stepId: string
  status: string
  actorType?: string
  actorId?: string
  childTaskId?: string
  reviewItemId?: string
  summary?: string
  startedAt?: string
  updatedAt: string
  finishedAt?: string
  inputArtifact?: string
  outputArtifact?: string
}

type Props = {
  definition: WorkflowDefinition
  run?: WorkflowRun
  instances?: WorkflowStepInstance[]
  compact?: boolean
}

const NODE_W = 190
const NODE_H = 86
const PAD = 48

const typeClass: Record<string, string> = {
  agent_task: 'border-sky-200 bg-sky-50/90 text-sky-900 dark:border-sky-900/70 dark:bg-sky-950/45 dark:text-sky-100',
  human_task: 'border-violet-200 bg-violet-50/90 text-violet-900 dark:border-violet-900/70 dark:bg-violet-950/45 dark:text-violet-100',
  human_review: 'border-amber-200 bg-amber-50/90 text-amber-900 dark:border-amber-900/70 dark:bg-amber-950/45 dark:text-amber-100',
  branch: 'border-fuchsia-200 bg-fuchsia-50/90 text-fuchsia-900 dark:border-fuchsia-900/70 dark:bg-fuchsia-950/45 dark:text-fuchsia-100',
  join: 'border-emerald-200 bg-emerald-50/90 text-emerald-900 dark:border-emerald-900/70 dark:bg-emerald-950/45 dark:text-emerald-100',
  terminal: 'border-neutral-200 bg-neutral-50/90 text-neutral-800 dark:border-zinc-700 dark:bg-zinc-800/70 dark:text-zinc-200',
}

const statusClass: Record<string, string> = {
  pending: 'bg-neutral-100 text-neutral-500 dark:bg-zinc-800 dark:text-zinc-400',
  running: 'bg-sky-100 text-sky-700 dark:bg-sky-900/50 dark:text-sky-300',
  waiting_review: 'bg-amber-100 text-amber-700 dark:bg-amber-900/50 dark:text-amber-300',
  blocked: 'bg-orange-100 text-orange-700 dark:bg-orange-900/50 dark:text-orange-300',
  completed: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/50 dark:text-emerald-300',
  failed: 'bg-red-100 text-red-700 dark:bg-red-900/50 dark:text-red-300',
}

export function WorkflowBoard({ definition, run, instances = [], compact = false }: Props) {
  const instanceByStep = useMemo(() => new Map(instances.map((inst) => [inst.stepId, inst])), [instances])
  const [selectedId, setSelectedId] = useState(definition.startStepId || definition.steps[0]?.id || '')
  const selected = definition.steps.find((s) => s.id === selectedId) ?? definition.steps[0]
  const selectedInst = selected ? instanceByStep.get(selected.id) : undefined

  const bounds = useMemo(() => {
    const maxX = Math.max(900, ...definition.steps.map((s) => s.position.x + NODE_W + PAD))
    const maxY = Math.max(420, ...definition.steps.map((s) => s.position.y + NODE_H + PAD))
    return { width: maxX, height: maxY }
  }, [definition.steps])

  function center(stepId: string) {
    const step = definition.steps.find((s) => s.id === stepId)
    if (!step) return { x: 0, y: 0 }
    return { x: step.position.x + NODE_W / 2, y: step.position.y + NODE_H / 2 }
  }

  return (
    <div className={cn('grid min-h-0 gap-4', compact ? 'grid-cols-1' : 'grid-cols-[minmax(0,1fr)_320px]')}>
      <div className="relative min-h-[420px] overflow-auto rounded-xl border border-neutral-200/80 bg-[radial-gradient(circle_at_1px_1px,rgba(14,165,233,0.16)_1px,transparent_0)] [background-size:22px_22px] dark:border-zinc-700/60 dark:bg-[radial-gradient(circle_at_1px_1px,rgba(125,211,252,0.12)_1px,transparent_0)]">
        <div className="relative" style={{ width: bounds.width, height: bounds.height }}>
          <svg className="pointer-events-none absolute inset-0 h-full w-full">
            <defs>
              <marker id="wf-arrow" markerWidth="10" markerHeight="10" refX="8" refY="3" orient="auto" markerUnits="strokeWidth">
                <path d="M0,0 L0,6 L8,3 z" className="fill-neutral-300 dark:fill-zinc-600" />
              </marker>
            </defs>
            {definition.edges.map((edge) => {
              const from = center(edge.from)
              const to = center(edge.to)
              const midX = (from.x + to.x) / 2
              const d = `M ${from.x} ${from.y} C ${midX} ${from.y}, ${midX} ${to.y}, ${to.x} ${to.y}`
              return (
                <g key={edge.id}>
                  <path d={d} className="fill-none stroke-neutral-300 dark:stroke-zinc-600" strokeWidth="2" markerEnd="url(#wf-arrow)" />
                  {edge.label ? (
                    <text x={midX} y={(from.y + to.y) / 2 - 8} textAnchor="middle" className="fill-neutral-400 text-[11px] dark:fill-zinc-500">
                      {edge.label}
                    </text>
                  ) : null}
                </g>
              )
            })}
          </svg>
          {definition.steps.map((step) => {
            const inst = instanceByStep.get(step.id)
            const active = run?.activeStepId === step.id || inst?.status === 'running'
            const selectedNode = selected?.id === step.id
            return (
              <button
                key={step.id}
                type="button"
                onClick={() => setSelectedId(step.id)}
                className={cn(
                  'absolute flex flex-col items-start rounded-xl border px-3 py-2 text-left shadow-sm transition-all hover:-translate-y-0.5 hover:shadow-md',
                  typeClass[step.type] ?? typeClass.terminal,
                  active && 'ring-2 ring-sky-400 ring-offset-2 ring-offset-white dark:ring-sky-500 dark:ring-offset-zinc-950',
                  selectedNode && 'shadow-lg',
                )}
                style={{ left: step.position.x, top: step.position.y, width: NODE_W, height: NODE_H }}
              >
                <span className="text-[11px] font-semibold uppercase tracking-wide opacity-60">{step.type.replace('_', ' ')}</span>
                <span className="mt-1 line-clamp-1 text-sm font-semibold">{step.title}</span>
                <span className="mt-auto flex w-full items-center justify-between gap-2">
                  <span className="truncate text-xs opacity-60">{step.actorRole || 'system'}</span>
                  {inst ? (
                    <span className={cn('rounded-full px-2 py-0.5 text-[10px] font-medium', statusClass[inst.status] ?? statusClass.pending)}>
                      {inst.status}
                    </span>
                  ) : null}
                </span>
              </button>
            )
          })}
        </div>
      </div>
      {!compact && selected ? (
        <aside className="min-h-[420px] rounded-xl border border-neutral-200/80 bg-white p-4 dark:border-zinc-700/60 dark:bg-zinc-900">
          <div className="flex items-start justify-between gap-3">
            <div>
              <p className="text-xs font-medium uppercase text-neutral-400 dark:text-zinc-500">{selected.type.replace('_', ' ')}</p>
              <h3 className="mt-1 text-base font-semibold text-neutral-900 dark:text-zinc-100">{selected.title}</h3>
            </div>
            {selectedInst ? (
              <span className={cn('rounded-full px-2 py-1 text-xs font-medium', statusClass[selectedInst.status] ?? statusClass.pending)}>
                {selectedInst.status}
              </span>
            ) : null}
          </div>
          <p className="mt-3 text-sm leading-6 text-neutral-600 dark:text-zinc-400">{selected.description || 'No description.'}</p>
          <div className="mt-5 space-y-4 text-sm">
            <Detail label="Actor" value={selected.actorRole || selectedInst?.actorId || 'system'} />
            <Detail label="Input" value={selected.inputSchema || selectedInst?.inputArtifact || 'Not specified'} />
            <Detail label="Output" value={selected.outputSchema || selectedInst?.outputArtifact || 'Not specified'} />
            <Detail label="Review" value={selected.reviewPolicy || selectedInst?.reviewItemId || 'Not required'} />
            {selectedInst?.childTaskId ? <Detail label="Child task" value={selectedInst.childTaskId} mono /> : null}
            {selectedInst?.summary ? <Detail label="Summary" value={selectedInst.summary} /> : null}
          </div>
        </aside>
      ) : null}
    </div>
  )
}

function Detail({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return (
    <div>
      <p className="text-xs font-medium uppercase text-neutral-400 dark:text-zinc-500">{label}</p>
      <p className={cn('mt-1 rounded-lg bg-neutral-50 px-3 py-2 text-neutral-700 dark:bg-zinc-800/70 dark:text-zinc-300', mono && 'font-mono text-xs')}>
        {value}
      </p>
    </div>
  )
}

