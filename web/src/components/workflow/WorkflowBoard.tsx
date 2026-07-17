import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  addEdge,
  Background,
  BackgroundVariant,
  Controls,
  Handle,
  MarkerType,
  MiniMap,
  Position,
  ReactFlow,
  type Connection,
  type Edge,
  type Node,
  type NodeChange,
  type NodeProps,
  useEdgesState,
  useNodesState,
} from '@xyflow/react'
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
  scope?: string
  project?: string
  startStepId: string
  steps: WorkflowStep[]
  edges: WorkflowEdge[]
  createdAt?: string
  updatedAt?: string
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
  fill?: boolean
  editable?: boolean
  onChange?: (definition: WorkflowDefinition) => void
  fullscreen?: boolean
  onToggleFullscreen?: () => void
}

const EMPTY_INSTANCES: WorkflowStepInstance[] = []

type WorkflowNodeData = {
  step: WorkflowStep
  status?: string
  active: boolean
}

type WorkflowNode = Node<WorkflowNodeData, 'workflowStep'>

const nodeTypes = { workflowStep: WorkflowStepNode }

const typeClass: Record<string, string> = {
  agent_task: 'border-sky-200 bg-sky-50/95 text-sky-900 dark:border-sky-900/70 dark:bg-sky-950/80 dark:text-sky-100',
  human_task:
    'border-violet-200 bg-violet-50/95 text-violet-900 dark:border-violet-900/70 dark:bg-violet-950/80 dark:text-violet-100',
  human_review:
    'border-amber-200 bg-amber-50/95 text-amber-900 dark:border-amber-900/70 dark:bg-amber-950/80 dark:text-amber-100',
  branch:
    'border-fuchsia-200 bg-fuchsia-50/95 text-fuchsia-900 dark:border-fuchsia-900/70 dark:bg-fuchsia-950/80 dark:text-fuchsia-100',
  join: 'border-emerald-200 bg-emerald-50/95 text-emerald-900 dark:border-emerald-900/70 dark:bg-emerald-950/80 dark:text-emerald-100',
  terminal: 'border-neutral-200 bg-neutral-50/95 text-neutral-800 dark:border-zinc-700 dark:bg-zinc-800/90 dark:text-zinc-200',
}

const statusClass: Record<string, string> = {
  pending: 'bg-neutral-100 text-neutral-500 dark:bg-zinc-800 dark:text-zinc-400',
  running: 'bg-sky-100 text-sky-700 dark:bg-sky-900/50 dark:text-sky-300',
  waiting_review: 'bg-amber-100 text-amber-700 dark:bg-amber-900/50 dark:text-amber-300',
  blocked: 'bg-orange-100 text-orange-700 dark:bg-orange-900/50 dark:text-orange-300',
  completed: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/50 dark:text-emerald-300',
  failed: 'bg-red-100 text-red-700 dark:bg-red-900/50 dark:text-red-300',
}

const edgeClass: Record<string, string> = {
  pending: '#d4d4d8',
  running: '#0ea5e9',
  waiting_review: '#f59e0b',
  completed: '#10b981',
  failed: '#ef4444',
}

const stepTypes = ['agent_task', 'human_task', 'human_review', 'branch', 'join', 'terminal']

const fieldClass =
  'mt-1 w-full rounded-lg border border-neutral-300 bg-white px-3 py-2 text-sm text-neutral-900 outline-none transition-colors focus:border-sky-400 dark:border-zinc-600 dark:bg-zinc-800 dark:text-zinc-100'

function WorkflowStepNode({ data, selected }: NodeProps<WorkflowNode>) {
  const { t } = useTranslation()
  const { step, status, active } = data
  return (
    <div
      className={cn(
        'relative flex h-[88px] w-[198px] flex-col rounded-xl border px-3 py-2 text-left shadow-sm transition-shadow',
        typeClass[step.type] ?? typeClass.terminal,
        active && 'ring-2 ring-sky-400 ring-offset-2 ring-offset-white dark:ring-sky-500 dark:ring-offset-zinc-950',
        selected && 'shadow-lg',
      )}
    >
      <Handle
        type="target"
        position={Position.Left}
        className="!h-2.5 !w-2.5 !border-2 !border-white !bg-neutral-400 dark:!border-zinc-950 dark:!bg-zinc-500"
      />
      <Handle
        type="source"
        position={Position.Right}
        className="!h-2.5 !w-2.5 !border-2 !border-white !bg-neutral-400 dark:!border-zinc-950 dark:!bg-zinc-500"
      />
      <span className="text-[11px] font-semibold uppercase opacity-60">{t(`workflows.stepTypes.${step.type}`, { defaultValue: step.type.replace('_', ' ') })}</span>
      <span className="mt-1 line-clamp-1 text-sm font-semibold">{step.title}</span>
      <span className="mt-auto flex w-full items-center justify-between gap-2">
        <span className="truncate text-xs opacity-60">{step.actorRole || 'system'}</span>
        {status ? <span className={cn('rounded-full px-2 py-0.5 text-[10px] font-medium', statusClass[status] ?? statusClass.pending)}>{status}</span> : null}
      </span>
    </div>
  )
}

export function WorkflowBoard({
  definition,
  run,
  instances = EMPTY_INSTANCES,
  compact = false,
  fill = false,
  editable = false,
  onChange,
  fullscreen = false,
  onToggleFullscreen,
}: Props) {
  const { t } = useTranslation()
  const instanceByStep = useMemo(() => new Map(instances.map((inst) => [inst.stepId, inst])), [instances])
  const [selectedId, setSelectedId] = useState(definition.startStepId || definition.steps[0]?.id || '')
  const [selectedEdgeId, setSelectedEdgeId] = useState('')
  const selected = definition.steps.find((s) => s.id === selectedId) ?? definition.steps[0]
  const selectedInst = selected ? instanceByStep.get(selected.id) : undefined

  const initialNodes = useMemo<WorkflowNode[]>(
    () =>
      definition.steps.map((step) => {
        const inst = instanceByStep.get(step.id)
        const active = run?.activeStepId === step.id || inst?.status === 'running'
        return {
          id: step.id,
          type: 'workflowStep',
          position: step.position,
          sourcePosition: Position.Right,
          targetPosition: Position.Left,
          data: { step, status: inst?.status, active },
        }
      }),
    [definition.steps, instanceByStep, run?.activeStepId],
  )

  const initialEdges = useMemo<Edge[]>(
    () =>
      definition.edges.map((edge) => {
        const sourceInst = instanceByStep.get(edge.from)
        const active = run?.activeStepId === edge.from || sourceInst?.status === 'running'
        const color = active ? edgeClass.running : sourceInst?.status === 'completed' ? edgeClass.completed : edgeClass.pending
        return {
          id: edge.id,
          source: edge.from,
          target: edge.to,
          label: edge.label,
          type: 'smoothstep',
          animated: active,
          markerEnd: { type: MarkerType.ArrowClosed, color },
          style: { stroke: color, strokeWidth: active ? 2.5 : 2 },
          labelStyle: { fill: '#71717a', fontSize: 11, fontWeight: 500 },
          labelBgPadding: [6, 3],
          labelBgBorderRadius: 6,
          labelBgStyle: { fill: 'rgba(255,255,255,0.92)' },
        }
      }),
    [definition.edges, instanceByStep, run?.activeStepId],
  )

  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes)
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges)

  useEffect(() => {
    setNodes(initialNodes)
  }, [initialNodes, setNodes])

  useEffect(() => {
    setEdges(initialEdges)
  }, [initialEdges, setEdges])

  useEffect(() => {
    if (!definition.steps.some((step) => step.id === selectedId)) {
      setSelectedId(definition.startStepId || definition.steps[0]?.id || '')
    }
  }, [definition.startStepId, definition.steps, selectedId])

  function updateDefinition(next: WorkflowDefinition) {
    onChange?.(next)
  }

  function handleNodesChange(changes: NodeChange<WorkflowNode>[]) {
    onNodesChange(changes)
    if (!editable || !onChange) return
    const posByID = new Map<string, { x: number; y: number }>()
    for (const change of changes) {
      if (change.type === 'position' && change.position) {
        posByID.set(change.id, change.position)
      }
    }
    if (posByID.size === 0) return
    updateDefinition({
      ...definition,
      steps: definition.steps.map((step) => {
        const pos = posByID.get(step.id)
        return pos ? { ...step, position: { x: Math.round(pos.x), y: Math.round(pos.y) } } : step
      }),
    })
  }

  function handleConnect(connection: Connection) {
    if (!editable || !connection.source || !connection.target || connection.source === connection.target) return
    const edgeID = `e-${connection.source}-${connection.target}-${Date.now().toString(36)}`
    setEdges((eds) =>
      addEdge(
        {
          ...connection,
          id: edgeID,
          type: 'smoothstep',
          markerEnd: { type: MarkerType.ArrowClosed, color: edgeClass.pending },
          style: { stroke: edgeClass.pending, strokeWidth: 2 },
        },
        eds,
      ),
    )
    updateDefinition({
      ...definition,
      edges: [...definition.edges, { id: edgeID, from: connection.source, to: connection.target }],
    })
  }

  function addNode() {
    if (!editable) return
    const base = selected ?? definition.steps[definition.steps.length - 1]
    const id = `step_${Date.now().toString(36)}`
    const step: WorkflowStep = {
      id,
      type: 'agent_task',
      title: t('workflows.newStepTitle'),
      description: '',
      actorRole: 'agent',
      position: { x: (base?.position.x ?? 80) + 280, y: base?.position.y ?? 180 },
    }
    setSelectedId(id)
    updateDefinition({
      ...definition,
      startStepId: definition.startStepId || id,
      steps: [...definition.steps, step],
    })
  }

  function deleteSelected() {
    if (!editable) return
    if (selectedEdgeId) {
      updateDefinition({ ...definition, edges: definition.edges.filter((edge) => edge.id !== selectedEdgeId) })
      setSelectedEdgeId('')
      return
    }
    if (!selected || definition.steps.length <= 1) return
    const nextSteps = definition.steps.filter((step) => step.id !== selected.id)
    const nextSelected = nextSteps[0]?.id || ''
    setSelectedId(nextSelected)
    updateDefinition({
      ...definition,
      startStepId: definition.startStepId === selected.id ? nextSelected : definition.startStepId,
      steps: nextSteps,
      edges: definition.edges.filter((edge) => edge.from !== selected.id && edge.to !== selected.id),
    })
  }

  function updateSelectedStep(patch: Partial<WorkflowStep>) {
    if (!editable || !selected) return
    updateDefinition({
      ...definition,
      steps: definition.steps.map((step) => (step.id === selected.id ? { ...step, ...patch } : step)),
    })
  }

  return (
    <div className={cn('grid min-h-0 gap-4', fill && 'h-full flex-1', compact ? 'grid-cols-1' : fullscreen ? 'grid-cols-[minmax(0,1fr)_360px]' : 'grid-cols-[minmax(0,1fr)_320px]')}>
      <div
        className={cn(
          'relative overflow-hidden rounded-xl border border-neutral-200/80 bg-white dark:border-zinc-700/60 dark:bg-zinc-950',
          fill ? 'h-full min-h-[560px]' : fullscreen ? 'h-[calc(100vh-150px)]' : compact ? 'h-[360px]' : 'h-[520px]',
        )}
      >
        {(editable || onToggleFullscreen) && (
          <div className="absolute left-3 top-3 z-10 flex flex-wrap gap-2">
            {editable ? (
              <>
                <button type="button" onClick={addNode} className="rounded-lg border border-sky-600 bg-white px-3 py-1.5 text-xs font-medium text-sky-700 shadow-sm hover:bg-sky-50 dark:border-sky-500 dark:bg-zinc-900 dark:text-sky-400 dark:hover:bg-zinc-800">
                  {t('workflows.addNode')}
                </button>
                <button type="button" onClick={deleteSelected} disabled={!selected && !selectedEdgeId} className="rounded-lg border border-neutral-300 bg-white px-3 py-1.5 text-xs font-medium text-neutral-600 shadow-sm hover:bg-neutral-50 disabled:opacity-40 dark:border-zinc-600 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:bg-zinc-800">
                  {selectedEdgeId ? t('workflows.deleteEdge') : t('workflows.deleteNode')}
                </button>
              </>
            ) : null}
            {onToggleFullscreen ? (
              <button type="button" onClick={onToggleFullscreen} className="rounded-lg border border-neutral-300 bg-white px-3 py-1.5 text-xs font-medium text-neutral-600 shadow-sm hover:bg-neutral-50 dark:border-zinc-600 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:bg-zinc-800">
                {fullscreen ? t('workflows.exitFullscreen') : t('workflows.fullscreen')}
              </button>
            ) : null}
          </div>
        )}
        <ReactFlow
          nodes={nodes}
          edges={edges}
          nodeTypes={nodeTypes}
          onNodesChange={handleNodesChange}
          onEdgesChange={onEdgesChange}
          onConnect={handleConnect}
          onNodeClick={(_, node) => {
            setSelectedId(node.id)
            setSelectedEdgeId('')
          }}
          onEdgeClick={(_, edge) => {
            setSelectedEdgeId(edge.id)
            setSelectedId('')
          }}
          onPaneClick={() => setSelectedEdgeId('')}
          fitView
          fitViewOptions={{ padding: 0.18 }}
          minZoom={0.25}
          maxZoom={1.8}
          panOnScroll
          zoomOnPinch
          nodesDraggable={editable}
          nodesConnectable={editable}
          elementsSelectable
          deleteKeyCode={null}
          className="workflow-flow"
        >
          <Background variant={BackgroundVariant.Dots} gap={24} size={1.2} color="rgba(14,165,233,0.22)" />
          <Controls showInteractive={false} position="bottom-left" />
          {!compact ? (
            <MiniMap
              position="bottom-right"
              pannable
              zoomable
              nodeStrokeWidth={3}
              nodeColor={(node) => {
                const data = node.data as WorkflowNodeData
                if (data.active) return '#0ea5e9'
                if (data.status === 'completed') return '#10b981'
                if (data.status === 'failed') return '#ef4444'
                return '#a1a1aa'
              }}
            />
          ) : null}
        </ReactFlow>
      </div>
      {!compact && selected ? (
        <aside className={cn('rounded-xl border border-neutral-200/80 bg-white p-4 dark:border-zinc-700/60 dark:bg-zinc-900', fill ? 'h-full min-h-[560px] overflow-y-auto' : fullscreen ? 'min-h-[calc(100vh-150px)]' : 'min-h-[420px]')}>
          {editable ? (
            <div className="space-y-4 text-sm">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <p className="text-xs font-medium uppercase text-neutral-400 dark:text-zinc-500">{t('workflows.detail.nodeSettings')}</p>
                  <p className="mt-1 font-mono text-xs text-neutral-400 dark:text-zinc-500">{selected.id}</p>
                </div>
                {definition.startStepId === selected.id ? (
                  <span className="rounded-full bg-sky-100 px-2 py-1 text-xs font-medium text-sky-700 dark:bg-sky-900/50 dark:text-sky-300">
                    {t('workflows.detail.startNode')}
                  </span>
                ) : null}
              </div>
              <label className="block">
                <span className="text-xs font-medium uppercase text-neutral-400 dark:text-zinc-500">{t('workflows.detail.title')}</span>
                <input value={selected.title} onChange={(event) => updateSelectedStep({ title: event.target.value })} className={fieldClass} />
              </label>
              <label className="block">
                <span className="text-xs font-medium uppercase text-neutral-400 dark:text-zinc-500">{t('workflows.detail.type')}</span>
                <select value={selected.type} onChange={(event) => updateSelectedStep({ type: event.target.value })} className={fieldClass}>
                  {stepTypes.map((type) => (
                    <option key={type} value={type}>
                      {t(`workflows.stepTypes.${type}`, { defaultValue: type.replace('_', ' ') })}
                    </option>
                  ))}
                </select>
              </label>
              <label className="block">
                <span className="text-xs font-medium uppercase text-neutral-400 dark:text-zinc-500">{t('workflows.detail.description')}</span>
                <textarea value={selected.description || ''} onChange={(event) => updateSelectedStep({ description: event.target.value })} rows={3} className={cn(fieldClass, 'resize-y')} />
              </label>
              <label className="block">
                <span className="text-xs font-medium uppercase text-neutral-400 dark:text-zinc-500">{t('workflows.detail.actor')}</span>
                <input value={selected.actorRole || ''} onChange={(event) => updateSelectedStep({ actorRole: event.target.value })} placeholder="agent" className={fieldClass} />
              </label>
              <label className="block">
                <span className="text-xs font-medium uppercase text-neutral-400 dark:text-zinc-500">{t('workflows.detail.input')}</span>
                <textarea value={selected.inputSchema || ''} onChange={(event) => updateSelectedStep({ inputSchema: event.target.value })} rows={3} className={cn(fieldClass, 'resize-y')} />
              </label>
              <label className="block">
                <span className="text-xs font-medium uppercase text-neutral-400 dark:text-zinc-500">{t('workflows.detail.output')}</span>
                <textarea value={selected.outputSchema || ''} onChange={(event) => updateSelectedStep({ outputSchema: event.target.value })} rows={3} className={cn(fieldClass, 'resize-y')} />
              </label>
              <label className="block">
                <span className="text-xs font-medium uppercase text-neutral-400 dark:text-zinc-500">{t('workflows.detail.review')}</span>
                <input value={selected.reviewPolicy || ''} onChange={(event) => updateSelectedStep({ reviewPolicy: event.target.value })} placeholder={t('workflows.detail.notRequired')} className={fieldClass} />
              </label>
              <label className="flex items-center justify-between gap-3 rounded-lg border border-neutral-200 px-3 py-2 dark:border-zinc-700">
                <span className="text-sm text-neutral-600 dark:text-zinc-300">{t('workflows.detail.setAsStart')}</span>
                <input
                  type="checkbox"
                  checked={definition.startStepId === selected.id}
                  onChange={(event) => event.target.checked && updateDefinition({ ...definition, startStepId: selected.id })}
                  className="size-4 accent-sky-600"
                />
              </label>
            </div>
          ) : (
            <>
              <div className="flex items-start justify-between gap-3">
                <div>
                  <p className="text-xs font-medium uppercase text-neutral-400 dark:text-zinc-500">{t(`workflows.stepTypes.${selected.type}`, { defaultValue: selected.type.replace('_', ' ') })}</p>
                  <h3 className="mt-1 text-base font-semibold text-neutral-900 dark:text-zinc-100">{selected.title}</h3>
                </div>
                {selectedInst ? (
                  <span className={cn('rounded-full px-2 py-1 text-xs font-medium', statusClass[selectedInst.status] ?? statusClass.pending)}>
                    {selectedInst.status}
                  </span>
                ) : null}
              </div>
              <p className="mt-3 text-sm leading-6 text-neutral-600 dark:text-zinc-400">{selected.description || t('workflows.noDescription')}</p>
              <div className="mt-5 space-y-4 text-sm">
                <Detail label={t('workflows.detail.actor')} value={selected.actorRole || selectedInst?.actorId || 'system'} />
                <Detail label={t('workflows.detail.input')} value={selected.inputSchema || selectedInst?.inputArtifact || t('workflows.detail.notSpecified')} />
                <Detail label={t('workflows.detail.output')} value={selected.outputSchema || selectedInst?.outputArtifact || t('workflows.detail.notSpecified')} />
                <Detail label={t('workflows.detail.review')} value={selected.reviewPolicy || selectedInst?.reviewItemId || t('workflows.detail.notRequired')} />
                {selectedInst?.childTaskId ? <Detail label={t('workflows.detail.childTask')} value={selectedInst.childTaskId} mono /> : null}
                {selectedInst?.summary ? <Detail label={t('workflows.detail.summary')} value={selectedInst.summary} /> : null}
              </div>
            </>
          )}
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
