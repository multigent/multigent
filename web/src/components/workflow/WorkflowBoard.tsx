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
  type NodeProps,
  useEdgesState,
  useNodesState,
} from '@xyflow/react'
import { cn } from '../../lib/cn'

export type WorkflowPosition = { x: number; y: number }

export type WorkflowField = {
  name: string
  description?: string
}

export type WorkflowStep = {
  id: string
  type: string
  title: string
  description?: string
  actorRole?: string
  inputSchema?: string
  outputSchema?: string
  inputFields?: WorkflowField[]
  outputFields?: WorkflowField[]
  reviewPolicy?: string
  position: WorkflowPosition
  config?: Record<string, string>
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

const colorClass: Record<string, string> = {
  neutral: 'border-neutral-200 bg-neutral-50/95 text-neutral-800 dark:border-zinc-700 dark:bg-zinc-800/90 dark:text-zinc-200',
  sky: 'border-sky-200 bg-sky-50/95 text-sky-900 dark:border-sky-900/70 dark:bg-sky-950/80 dark:text-sky-100',
  violet: 'border-violet-200 bg-violet-50/95 text-violet-900 dark:border-violet-900/70 dark:bg-violet-950/80 dark:text-violet-100',
  amber: 'border-amber-200 bg-amber-50/95 text-amber-900 dark:border-amber-900/70 dark:bg-amber-950/80 dark:text-amber-100',
  emerald: 'border-emerald-200 bg-emerald-50/95 text-emerald-900 dark:border-emerald-900/70 dark:bg-emerald-950/80 dark:text-emerald-100',
  rose: 'border-rose-200 bg-rose-50/95 text-rose-900 dark:border-rose-900/70 dark:bg-rose-950/80 dark:text-rose-100',
}

const colorDotClass: Record<string, string> = {
  neutral: 'bg-neutral-400',
  sky: 'bg-sky-500',
  violet: 'bg-violet-500',
  amber: 'bg-amber-500',
  emerald: 'bg-emerald-500',
  rose: 'bg-rose-500',
}

const colorMiniMap: Record<string, string> = {
  neutral: '#a1a1aa',
  sky: '#0ea5e9',
  violet: '#8b5cf6',
  amber: '#f59e0b',
  emerald: '#10b981',
  rose: '#f43f5e',
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
const colorOptions = ['neutral', 'sky', 'violet', 'amber', 'emerald', 'rose']
const ALIGN_THRESHOLD = 28

const fieldClass =
  'mt-1 w-full rounded-lg border border-neutral-300 bg-white px-3 py-2 text-sm text-neutral-900 outline-none transition-colors focus:border-sky-400 dark:border-zinc-600 dark:bg-zinc-800 dark:text-zinc-100'

function legacyFields(schema?: string): WorkflowField[] {
  if (!schema?.trim()) return []
  return schema
    .split(',')
    .map((part) => part.trim())
    .filter(Boolean)
    .map((name) => ({ name }))
}

function schemaFieldsFor(step: WorkflowStep, kind: 'input' | 'output'): WorkflowField[] {
  const fields = kind === 'input' ? step.inputFields : step.outputFields
  if (fields && fields.length > 0) return fields
  return legacyFields(kind === 'input' ? step.inputSchema : step.outputSchema)
}

function normalizeStepFields(step: WorkflowStep): WorkflowStep {
  const inputFields = schemaFieldsFor(step, 'input').map(({ name, description }) => ({ name, description }))
  const outputFields = schemaFieldsFor(step, 'output').map(({ name, description }) => ({ name, description }))
  return {
    ...step,
    inputFields,
    outputFields,
  }
}

function WorkflowStepNode({ data, selected }: NodeProps<WorkflowNode>) {
  const { t } = useTranslation()
  const { step, status, active } = data
  const nodeClass = colorClass[step.config?.color || ''] ?? typeClass[step.type] ?? typeClass.terminal
  return (
    <div
      className={cn(
        'relative flex h-[88px] w-[198px] flex-col rounded-xl border px-3 py-2 text-left shadow-sm transition-shadow',
        nodeClass,
        active && 'ring-2 ring-sky-400 ring-offset-2 ring-offset-white dark:ring-sky-500 dark:ring-offset-zinc-950',
        selected && 'border-sky-500 shadow-lg ring-2 ring-sky-500 ring-offset-2 ring-offset-white dark:border-sky-400 dark:ring-sky-400 dark:ring-offset-zinc-950',
      )}
    >
      {selected ? <span className="absolute -right-1.5 -top-1.5 size-3 rounded-full border-2 border-white bg-sky-500 dark:border-zinc-950 dark:bg-sky-400" /> : null}
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
  const [stepDraft, setStepDraft] = useState<WorkflowStep | null>(selected ?? null)

  useEffect(() => {
    setStepDraft(selected ? normalizeStepFields(selected) : null)
  }, [selected?.id])

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

  function alignedPosition(node: WorkflowNode, allNodes: WorkflowNode[]) {
    let x = Math.round(node.position.x)
    let y = Math.round(node.position.y)
    for (const other of allNodes) {
      if (other.id === node.id) continue
      if (Math.abs(x - other.position.x) <= ALIGN_THRESHOLD) {
        x = Math.round(other.position.x)
      }
      if (Math.abs(y - other.position.y) <= ALIGN_THRESHOLD) {
        y = Math.round(other.position.y)
      }
    }
    return { x, y }
  }

  function persistNodePositions(nextNodes: WorkflowNode[], draggedNode?: WorkflowNode) {
    if (!editable || !onChange) return
    const posByID = new Map(nextNodes.map((node) => [node.id, draggedNode && node.id === draggedNode.id ? alignedPosition(node, nextNodes) : node.position]))
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

  function updateStepDraft(patch: Partial<WorkflowStep>) {
    setStepDraft((current) => (current ? { ...current, ...patch } : current))
  }

  function updateStepDraftColor(color: string) {
    setStepDraft((current) => {
      if (!current) return current
      const config = { ...(current.config ?? {}) }
      if (color) {
        config.color = color
      } else {
        delete config.color
      }
      return { ...current, config }
    })
  }

  function updateStepDraftFields(kind: 'inputFields' | 'outputFields', fields: WorkflowField[]) {
    setStepDraft((current) => (current ? { ...current, [kind]: fields.map(({ name, description }) => ({ name, description })), [kind === 'inputFields' ? 'inputSchema' : 'outputSchema']: '' } : current))
  }

  function saveSelectedStep() {
    if (!editable || !selected || !stepDraft) return
    updateDefinition({
      ...definition,
      steps: definition.steps.map((step) => (step.id === selected.id ? stepDraft : step)),
    })
  }

  const stepDraftChanged = Boolean(stepDraft && selected && JSON.stringify(stepDraft) !== JSON.stringify(selected))

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
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          onNodeDragStop={(_, node, nextNodes) => persistNodePositions(nextNodes as WorkflowNode[], node as WorkflowNode)}
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
          snapToGrid={editable}
          snapGrid={[24, 24]}
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
                if (data.step.config?.color && colorMiniMap[data.step.config.color]) return colorMiniMap[data.step.config.color]
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
          {editable && stepDraft ? (
            <div className="space-y-4 text-sm">
              <label className="block">
                <span className="text-xs font-medium uppercase text-neutral-400 dark:text-zinc-500">{t('workflows.detail.title')}</span>
                <input value={stepDraft.title} onChange={(event) => updateStepDraft({ title: event.target.value })} className={fieldClass} />
              </label>
              <label className="block">
                <span className="text-xs font-medium uppercase text-neutral-400 dark:text-zinc-500">{t('workflows.detail.type')}</span>
                <select value={stepDraft.type} onChange={(event) => updateStepDraft({ type: event.target.value })} className={fieldClass}>
                  {stepTypes.map((type) => (
                    <option key={type} value={type}>
                      {t(`workflows.stepTypes.${type}`, { defaultValue: type.replace('_', ' ') })}
                    </option>
                  ))}
                </select>
              </label>
              <div>
                <span className="text-xs font-medium uppercase text-neutral-400 dark:text-zinc-500">{t('workflows.detail.color')}</span>
                <div className="mt-2 flex flex-wrap gap-2">
                  {colorOptions.map((color) => (
                    <button
                      key={color}
                      type="button"
                      onClick={() => updateStepDraftColor(color)}
                      className={cn(
                        'flex size-8 items-center justify-center rounded-full border border-neutral-200 bg-white dark:border-zinc-700 dark:bg-zinc-900',
                        stepDraft.config?.color === color && 'ring-2 ring-sky-400 ring-offset-2 ring-offset-white dark:ring-offset-zinc-900',
                      )}
                      title={t(`workflows.colors.${color}`)}
                    >
                      <span className={cn('size-4 rounded-full', colorDotClass[color])} />
                    </button>
                  ))}
                </div>
              </div>
              <label className="block">
                <span className="text-xs font-medium uppercase text-neutral-400 dark:text-zinc-500">{t('workflows.detail.description')}</span>
                <textarea value={stepDraft.description || ''} onChange={(event) => updateStepDraft({ description: event.target.value })} rows={3} className={cn(fieldClass, 'resize-y')} />
              </label>
              <label className="block">
                <span className="text-xs font-medium uppercase text-neutral-400 dark:text-zinc-500">{t('workflows.detail.defaultRole')}</span>
                <input value={stepDraft.actorRole || ''} onChange={(event) => updateStepDraft({ actorRole: event.target.value })} placeholder="agent" className={fieldClass} />
              </label>
              <FieldTable
                title={t('workflows.detail.input')}
                fields={stepDraft.inputFields ?? []}
                onChange={(fields) => updateStepDraftFields('inputFields', fields)}
              />
              <FieldTable
                title={t('workflows.detail.output')}
                fields={stepDraft.outputFields ?? []}
                onChange={(fields) => updateStepDraftFields('outputFields', fields)}
              />
              <button
                type="button"
                onClick={saveSelectedStep}
                disabled={!stepDraftChanged || !stepDraft.title.trim()}
                className="w-full rounded-lg border border-sky-600 bg-white px-3 py-2 text-sm font-medium text-sky-700 hover:bg-sky-50 disabled:opacity-50 dark:border-sky-500 dark:bg-zinc-900 dark:text-sky-400 dark:hover:bg-zinc-800"
              >
                {t('workflows.detail.saveNode')}
              </button>
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
                <Detail label={t('workflows.detail.defaultRole')} value={selected.actorRole || selectedInst?.actorId || 'system'} />
                <FieldSummary label={t('workflows.detail.input')} fields={schemaFieldsFor(selected, 'input')} fallback={selected.inputSchema || selectedInst?.inputArtifact || t('workflows.detail.notSpecified')} />
                <FieldSummary label={t('workflows.detail.output')} fields={schemaFieldsFor(selected, 'output')} fallback={selected.outputSchema || selectedInst?.outputArtifact || t('workflows.detail.notSpecified')} />
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

function FieldTable({ title, fields, onChange }: { title: string; fields: WorkflowField[]; onChange: (fields: WorkflowField[]) => void }) {
  const { t } = useTranslation()

  function updateField(index: number, patch: Partial<WorkflowField>) {
    onChange(fields.map((field, i) => (i === index ? { ...field, ...patch } : field)))
  }

  function addField() {
    onChange([...fields, { name: '' }])
  }

  function removeField(index: number) {
    onChange(fields.filter((_, i) => i !== index))
  }

  return (
    <div>
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-medium uppercase text-neutral-400 dark:text-zinc-500">{title}</span>
        <button type="button" onClick={addField} className="rounded-md border border-neutral-300 px-2 py-1 text-xs text-neutral-600 hover:bg-neutral-50 dark:border-zinc-600 dark:text-zinc-300 dark:hover:bg-zinc-800">
          {t('workflows.detail.addField')}
        </button>
      </div>
      <div className="mt-2 overflow-hidden rounded-lg border border-neutral-200 dark:border-zinc-700">
        {fields.length === 0 ? (
          <div className="px-3 py-3 text-xs text-neutral-400 dark:text-zinc-500">{t('workflows.detail.noFields')}</div>
        ) : (
          <div className="divide-y divide-neutral-200 dark:divide-zinc-700">
            {fields.map((field, index) => (
              <div key={index} className="space-y-2 p-2">
                <input
                  value={field.name}
                  onChange={(event) => updateField(index, { name: event.target.value })}
                  placeholder={t('workflows.detail.fieldName')}
                  className="w-full rounded-md border border-neutral-300 bg-white px-2 py-1.5 text-xs text-neutral-900 outline-none focus:border-sky-400 dark:border-zinc-600 dark:bg-zinc-800 dark:text-zinc-100"
                />
                <input
                  value={field.description || ''}
                  onChange={(event) => updateField(index, { description: event.target.value })}
                  placeholder={t('workflows.detail.fieldDescription')}
                  className="w-full rounded-md border border-neutral-300 bg-white px-2 py-1.5 text-xs text-neutral-900 outline-none focus:border-sky-400 dark:border-zinc-600 dark:bg-zinc-800 dark:text-zinc-100"
                />
                <div className="flex justify-end">
                  <button type="button" onClick={() => removeField(index)} className="text-xs text-red-500 hover:text-red-600 dark:text-red-400">
                    {t('common.delete')}
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

function FieldSummary({ label, fields, fallback }: { label: string; fields: WorkflowField[]; fallback: string }) {
  if (fields.length === 0) return <Detail label={label} value={fallback} />
  return (
    <div>
      <p className="text-xs font-medium uppercase text-neutral-400 dark:text-zinc-500">{label}</p>
      <div className="mt-1 overflow-hidden rounded-lg border border-neutral-200 dark:border-zinc-700">
        {fields.map((field, index) => (
          <div key={`${field.name}-${index}`} className="border-b border-neutral-200 px-3 py-2 last:border-b-0 dark:border-zinc-700">
            <span className="block truncate text-sm font-medium text-neutral-700 dark:text-zinc-300">{field.name || '-'}</span>
            {field.description ? <p className="mt-1 text-xs text-neutral-500 dark:text-zinc-500">{field.description}</p> : null}
          </div>
        ))}
      </div>
    </div>
  )
}
