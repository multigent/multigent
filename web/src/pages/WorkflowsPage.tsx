import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useParams } from 'react-router-dom'
import { GitBranch, X } from 'lucide-react'
import { PlaceholderCard } from '../components/ui/PlaceholderCard'
import { confirmDialog } from '../components/ui/ConfirmDialog'
import { WorkflowBoard, type WorkflowDefinition, type WorkflowStep } from '../components/workflow/WorkflowBoard'
import { apiDelete, apiPost, apiPut } from '../lib/api'
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
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const params = useParams()
  const [reloadKey, setReloadKey] = useState(0)
  const state = useApiJson<WorkflowListResponse>('/api/v1/workflows', reloadKey)
  const templateLocale = i18n.resolvedLanguage || i18n.language || 'en'
  const templateState = useApiJson<{ templates: WorkflowDefinition[] }>(`/api/v1/workflow-templates?locale=${encodeURIComponent(templateLocale)}`, 0)
  const detailState = useApiJson<WorkflowDefinition>(params.workflowId ? `/api/v1/workflows/${encodeURIComponent(params.workflowId)}` : null, 0)
  const workflows = state.status === 'ok' ? state.data.workflows : []
  const templates = templateState.status === 'ok' ? templateState.data.templates : []
  const selected = useMemo(() => (params.workflowId && detailState.status === 'ok' ? detailState.data : undefined), [params.workflowId, detailState])

  const [draft, setDraft] = useState<WorkflowDefinition | null>(null)
  const [savedDraft, setSavedDraft] = useState<WorkflowDefinition | null>(null)
  const [saving, setSaving] = useState(false)
  const [fullscreen, setFullscreen] = useState(false)
  const [createOpen, setCreateOpen] = useState(false)
  const [createMode, setCreateMode] = useState<'blank' | 'template' | 'json'>('template')
  const [selectedTemplateId, setSelectedTemplateId] = useState('')
  const [workflowName, setWorkflowName] = useState('')
  const [workflowJSON, setWorkflowJSON] = useState('')
  const [createError, setCreateError] = useState('')
  const descriptionRef = useRef<HTMLTextAreaElement | null>(null)
  const wasWorkflowDetailRef = useRef(false)

  useEffect(() => {
    const next = selected ? structuredClone(selected) : null
    setDraft(next)
    setSavedDraft(next ? structuredClone(next) : null)
    setFullscreen(false)
  }, [selected?.id])

  useEffect(() => {
    if (params.workflowId) {
      wasWorkflowDetailRef.current = true
      return
    }
    if (wasWorkflowDetailRef.current) {
      wasWorkflowDetailRef.current = false
      setReloadKey((key) => key + 1)
    }
  }, [params.workflowId])

  const dirty = Boolean(draft && savedDraft && workflowEditableJSON(draft) !== workflowEditableJSON(savedDraft))

  useEffect(() => {
    if (!dirty) return
    function handleBeforeUnload(event: BeforeUnloadEvent) {
      event.preventDefault()
      event.returnValue = ''
    }
    window.addEventListener('beforeunload', handleBeforeUnload)
    return () => window.removeEventListener('beforeunload', handleBeforeUnload)
  }, [dirty])

  useEffect(() => {
    const el = descriptionRef.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = `${Math.min(el.scrollHeight, 96)}px`
  }, [draft?.description])

  async function createBlank(name = t('workflows.untitledName')) {
    setSaving(true)
    try {
      const created = await apiPost<WorkflowDefinition>('/api/v1/workflows', {
        name,
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

  async function createFromTemplate(templateId: string, name: string) {
    setSaving(true)
    try {
      const created = await apiPost<WorkflowDefinition>('/api/v1/workflows', {
        templateId,
        locale: templateLocale,
        name,
      })
      setCreateOpen(false)
      navigate(`/workflows/${encodeURIComponent(created.id)}`)
    } finally {
      setSaving(false)
    }
  }

  async function createFromJSON(raw: string, fallbackName: string) {
    const parsed = parseWorkflowJSON(raw)
    if (parsed.steps.length === 0) {
      throw new Error(t('workflows.importJSONNoSteps'))
    }
    const name = workflowName.trim() || parsed.name || fallbackName || t('workflows.untitledName')
    const created = await apiPost<WorkflowDefinition>('/api/v1/workflows', {
      name,
      description: parsed.description || '',
      startStepId: parsed.startStepId || parsed.steps[0].id,
      steps: parsed.steps,
      edges: parsed.edges,
    })
    setCreateOpen(false)
    navigate(`/workflows/${encodeURIComponent(created.id)}`)
  }

  function openCreateDialog() {
    const firstTemplate = templates[0]
    setCreateMode(firstTemplate ? 'template' : 'blank')
    setSelectedTemplateId(firstTemplate?.id || '')
    setWorkflowName(firstTemplate?.name || t('workflows.untitledName'))
    setWorkflowJSON('')
    setCreateError('')
    setCreateOpen(true)
  }

  function updateSelectedTemplate(templateId: string) {
    const tmpl = templates.find((item) => item.id === templateId)
    setSelectedTemplateId(templateId)
    if (tmpl) setWorkflowName(tmpl.name)
  }

  async function submitCreate() {
    setCreateError('')
    try {
      if (createMode === 'blank') {
        await createBlank(workflowName)
        setCreateOpen(false)
        return
      }
      if (createMode === 'json') {
        await createFromJSON(workflowJSON, workflowName)
        return
      }
      if (!selectedTemplateId) return
      await createFromTemplate(selectedTemplateId, workflowName)
    } catch (e) {
      setCreateError(e instanceof Error ? e.message : String(e))
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

  async function deleteWorkflow(wf: WorkflowDefinition) {
    const ok = await confirmDialog({
      title: t('workflows.deleteWorkflow'),
      description: t('workflows.confirmDeleteWorkflow', { name: wf.name }),
      confirmLabel: t('common.delete'),
      cancelLabel: t('common.cancel'),
      tone: 'danger',
    })
    if (!ok) return
    setSaving(true)
    try {
      await apiDelete(`/api/v1/workflows/${encodeURIComponent(wf.id)}`)
      if (params.workflowId === wf.id) {
        navigate('/workflows')
      }
      setReloadKey((key) => key + 1)
    } finally {
      setSaving(false)
    }
  }

  async function confirmLeaveIfDirty() {
    if (!dirty) return true
    return confirmDialog({
      title: t('workflows.unsavedTitle'),
      description: t('workflows.unsavedDescription'),
      confirmLabel: t('workflows.discardChanges'),
      cancelLabel: t('common.cancel'),
      tone: 'danger',
    })
  }

  async function backToList() {
    if (!(await confirmLeaveIfDirty())) return
    navigate('/workflows')
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
      setSavedDraft(structuredClone(saved))
    } finally {
      setSaving(false)
    }
  }

  if (params.workflowId) {
    return (
      <div className={fullscreen ? 'fixed inset-0 z-50 flex h-dvh flex-col overflow-hidden bg-neutral-50 px-6 py-5 dark:bg-zinc-950' : 'flex h-full min-h-full flex-col px-8 py-6 animate-fade-in'}>
        {detailState.status === 'loading' && <Loading label={t('api.loading')} />}
        {detailState.status === 'error' && (
          <PlaceholderCard title={t('api.loadError')}>
            <p className="text-[13px]">{detailState.error.message}</p>
          </PlaceholderCard>
        )}
        {detailState.status === 'ok' && !selected && (
          <PlaceholderCard title={t('workflows.notFound')}>
            <button type="button" onClick={() => navigate('/workflows')} className="mt-3 rounded-lg border border-sky-600 bg-white px-3 py-2 text-sm font-medium text-sky-700 hover:bg-sky-50 dark:border-sky-500 dark:bg-zinc-900 dark:text-sky-400 dark:hover:bg-zinc-800">
              {t('workflows.backToList')}
            </button>
          </PlaceholderCard>
        )}
        {draft && (
          <div className="flex min-h-0 flex-1 flex-col gap-4">
            {!fullscreen && (
              <div className="flex shrink-0 flex-wrap items-start justify-between gap-4">
                <div className="min-w-[320px] max-w-3xl flex-1">
                  <input
                    value={draft.name}
                    onChange={(e) => setDraft({ ...draft, name: e.target.value })}
                    className="block w-full rounded-lg border border-transparent bg-transparent px-0 text-xl font-semibold text-neutral-900 outline-none focus:border-neutral-200 focus:bg-white focus:px-3 dark:text-zinc-100 dark:focus:border-zinc-700 dark:focus:bg-zinc-900"
                  />
                  <textarea
                    ref={descriptionRef}
                    value={draft.description || ''}
                    onChange={(e) => setDraft({ ...draft, description: e.target.value })}
                    placeholder={t('workflows.descriptionPlaceholder')}
                    rows={1}
                    className="mt-1 block w-full resize-none overflow-hidden whitespace-pre-wrap rounded-lg border border-transparent bg-transparent px-0 py-0.5 text-sm leading-5 text-neutral-500 outline-none focus:border-neutral-200 focus:bg-white focus:px-3 dark:text-zinc-400 dark:focus:border-zinc-700 dark:focus:bg-zinc-900"
                  />
                  {draft.provenance && <ProvenancePill name={draft.provenance.playbookName} customized={draft.provenance.customized} />}
                </div>
                <div className="flex gap-2">
                  {dirty && (
                    <span className="self-center rounded-full bg-amber-50 px-2.5 py-1 text-xs font-medium text-amber-700 dark:bg-amber-950/30 dark:text-amber-300">
                      {t('workflows.unsavedBadge')}
                    </span>
                  )}
                  <button type="button" onClick={() => void backToList()} className="rounded-lg border border-neutral-300 bg-white px-3 py-2 text-sm font-medium text-neutral-600 hover:bg-neutral-50 dark:border-zinc-600 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:bg-zinc-800">
                    {t('workflows.backToList')}
                  </button>
                  <button type="button" onClick={() => void duplicateWorkflow(draft)} disabled={saving} className="rounded-lg border border-neutral-300 bg-white px-3 py-2 text-sm font-medium text-neutral-600 hover:bg-neutral-50 disabled:opacity-50 dark:border-zinc-600 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:bg-zinc-800">
                    {t('workflows.duplicate')}
                  </button>
                  <button type="button" onClick={() => void deleteWorkflow(draft)} disabled={saving} className="rounded-lg border border-red-300 bg-white px-3 py-2 text-sm font-medium text-red-600 hover:bg-red-50 disabled:opacity-50 dark:border-red-900/70 dark:bg-zinc-900 dark:text-red-400 dark:hover:bg-red-950/30">
                    {t('common.delete')}
                  </button>
                  <button type="button" onClick={() => void saveDraft()} disabled={saving || !dirty || !draft.name.trim()} className="rounded-lg border border-sky-600 bg-white px-3 py-2 text-sm font-medium text-sky-700 hover:bg-sky-50 disabled:opacity-50 dark:border-sky-500 dark:bg-zinc-900 dark:text-sky-400 dark:hover:bg-zinc-800">
                    {saving ? t('common.saving') : t('common.save')}
                  </button>
                </div>
              </div>
            )}
            <WorkflowBoard
              definition={draft}
              editable
              fill
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
        <button type="button" onClick={openCreateDialog} disabled={saving} className="rounded-lg border border-sky-600 bg-white px-3 py-2 text-sm font-medium text-sky-700 hover:bg-sky-50 disabled:opacity-50 dark:border-sky-500 dark:bg-zinc-900 dark:text-sky-400 dark:hover:bg-zinc-800">
          {t('workflows.createBlank')}
        </button>
      </div>

      {createOpen ? (
        <div className="fixed inset-0 z-50 flex items-start justify-center px-4 pt-[12vh]">
          <div className="absolute inset-0 bg-black/30 backdrop-blur-[2px] dark:bg-black/50" onClick={() => !saving && setCreateOpen(false)} />
          <div className="relative w-full max-w-2xl overflow-hidden rounded-xl border border-neutral-200/80 bg-white shadow-2xl dark:border-zinc-700/80 dark:bg-zinc-900">
            <div className="flex items-start justify-between gap-4 border-b border-neutral-200/80 px-5 py-3 dark:border-zinc-700/60">
              <div>
                <h2 className="text-sm font-semibold text-neutral-900 dark:text-zinc-100">{t('workflows.createWorkflow')}</h2>
                <p className="mt-0.5 text-xs text-neutral-500 dark:text-zinc-500">{t('workflows.createWorkflowHint')}</p>
              </div>
              <button type="button" onClick={() => setCreateOpen(false)} disabled={saving} className="rounded-md p-1 text-neutral-400 hover:bg-neutral-100 hover:text-neutral-700 disabled:opacity-50 dark:hover:bg-zinc-800 dark:hover:text-zinc-300">
                <X className="size-4" strokeWidth={2} />
              </button>
            </div>
            <div className="space-y-4 p-5">
              <div className="grid gap-3 sm:grid-cols-3">
                <button type="button" onClick={() => { setCreateMode('blank'); setWorkflowName(t('workflows.untitledName')) }} className={`rounded-xl border p-4 text-left ${createMode === 'blank' ? 'border-sky-500 bg-sky-50 dark:border-sky-500 dark:bg-sky-950/30' : 'border-neutral-200 hover:bg-neutral-50 dark:border-zinc-700 dark:hover:bg-zinc-800'}`}>
                  <p className="font-medium text-neutral-900 dark:text-zinc-100">{t('workflows.blankWorkflow')}</p>
                  <p className="mt-1 text-sm text-neutral-500 dark:text-zinc-400">{t('workflows.blankWorkflowHint')}</p>
                </button>
                <button type="button" onClick={() => { setCreateMode('template'); updateSelectedTemplate(selectedTemplateId || templates[0]?.id || '') }} disabled={templates.length === 0} className={`rounded-xl border p-4 text-left disabled:opacity-50 ${createMode === 'template' ? 'border-sky-500 bg-sky-50 dark:border-sky-500 dark:bg-sky-950/30' : 'border-neutral-200 hover:bg-neutral-50 dark:border-zinc-700 dark:hover:bg-zinc-800'}`}>
                  <p className="font-medium text-neutral-900 dark:text-zinc-100">{t('workflows.fromTemplate')}</p>
                  <p className="mt-1 text-sm text-neutral-500 dark:text-zinc-400">{t('workflows.fromTemplateHint')}</p>
                </button>
                <button type="button" onClick={() => { setCreateMode('json'); setWorkflowName(workflowName || t('workflows.untitledName')) }} className={`rounded-xl border p-4 text-left ${createMode === 'json' ? 'border-sky-500 bg-sky-50 dark:border-sky-500 dark:bg-sky-950/30' : 'border-neutral-200 hover:bg-neutral-50 dark:border-zinc-700 dark:hover:bg-zinc-800'}`}>
                  <p className="font-medium text-neutral-900 dark:text-zinc-100">{t('workflows.importJSON')}</p>
                  <p className="mt-1 text-sm text-neutral-500 dark:text-zinc-400">{t('workflows.importJSONHint')}</p>
                </button>
              </div>
              {createMode === 'template' ? (
                <div className="space-y-3">
                  <label className="block">
                    <span className="text-xs font-medium uppercase text-neutral-400 dark:text-zinc-500">{t('workflows.template')}</span>
                    <select value={selectedTemplateId} onChange={(event) => updateSelectedTemplate(event.target.value)} className="mt-1 w-full rounded-lg border border-neutral-300 bg-white px-3 py-2 text-sm text-neutral-900 outline-none focus:border-sky-400 dark:border-zinc-600 dark:bg-zinc-800 dark:text-zinc-100">
                      {templates.map((tmpl) => (
                        <option key={tmpl.id} value={tmpl.id}>{tmpl.name}</option>
                      ))}
                    </select>
                  </label>
                  {templates.find((item) => item.id === selectedTemplateId) ? (
                    <p className="rounded-lg bg-neutral-50 px-3 py-2 text-sm text-neutral-500 dark:bg-zinc-800/70 dark:text-zinc-400">
                      {templates.find((item) => item.id === selectedTemplateId)?.description}
                    </p>
                  ) : null}
                </div>
              ) : null}
              {createMode === 'json' ? (
                <label className="block">
                  <span className="text-xs font-medium uppercase text-neutral-400 dark:text-zinc-500">{t('workflows.importJSON')}</span>
                  <textarea
                    value={workflowJSON}
                    onChange={(event) => setWorkflowJSON(event.target.value)}
                    rows={9}
                    placeholder={t('workflows.importJSONPlaceholder')}
                    className="mt-1 w-full resize-y rounded-lg border border-neutral-300 bg-white px-3 py-2 font-mono text-xs text-neutral-900 outline-none focus:border-sky-400 dark:border-zinc-600 dark:bg-zinc-800 dark:text-zinc-100"
                  />
                </label>
              ) : null}
              <label className="block">
                <span className="text-xs font-medium uppercase text-neutral-400 dark:text-zinc-500">{t('workflows.workflowName')}</span>
                <input value={workflowName} onChange={(event) => setWorkflowName(event.target.value)} className="mt-1 w-full rounded-lg border border-neutral-300 bg-white px-3 py-2 text-sm text-neutral-900 outline-none focus:border-sky-400 dark:border-zinc-600 dark:bg-zinc-800 dark:text-zinc-100" />
              </label>
              {createError ? <p className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300">{createError}</p> : null}
            </div>
            <div className="flex justify-end gap-2 border-t border-neutral-200/80 px-5 py-3 dark:border-zinc-700/60">
              <button type="button" onClick={() => setCreateOpen(false)} disabled={saving} className="rounded-lg px-3 py-2 text-sm font-medium text-neutral-600 hover:bg-neutral-100 disabled:opacity-50 dark:text-zinc-400 dark:hover:bg-zinc-800">
                {t('common.cancel')}
              </button>
              <button type="button" onClick={() => void submitCreate()} disabled={saving || (createMode === 'template' && !selectedTemplateId) || (createMode !== 'json' && !workflowName.trim()) || (createMode === 'json' && !workflowJSON.trim())} className="rounded-lg bg-sky-600 px-3 py-2 text-sm font-medium text-white hover:bg-sky-700 disabled:opacity-50">
                {saving ? t('common.saving') : t('common.create')}
              </button>
            </div>
          </div>
        </div>
      ) : null}

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
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {workflows.map((wf) => (
            <button
              type="button"
              key={wf.id}
              onClick={() => navigate(`/workflows/${encodeURIComponent(wf.id)}`)}
              className="group flex min-h-[132px] flex-col rounded-xl border border-neutral-200/80 bg-white p-5 text-left transition-all duration-150 hover:border-sky-300/60 hover:shadow-md dark:border-zinc-700/60 dark:bg-zinc-900/30 dark:hover:border-sky-800/40"
            >
              <div>
                <div className="flex min-w-0 items-center gap-2.5">
                  <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-sky-100 dark:bg-sky-900/30">
                    <GitBranch className="size-4.5 text-sky-600 dark:text-sky-400" strokeWidth={1.8} />
                  </div>
                  <div className="min-w-0">
                    <h2 className="line-clamp-1 text-sm font-semibold text-neutral-900 dark:text-zinc-100">{wf.name}</h2>
                    <div className="mt-0.5 flex flex-wrap items-center gap-1.5">
                      <p className="text-xs text-neutral-400 dark:text-zinc-500">{t('workflows.stepCount', { count: wf.steps.length })}</p>
                      {wf.provenance && <ProvenancePill name={wf.provenance.playbookName} customized={wf.provenance.customized} />}
                    </div>
                  </div>
                </div>
                <div className="mt-2.5 block w-full text-left">
                  <p className="line-clamp-2 text-sm leading-relaxed text-neutral-500 dark:text-zinc-500">{wf.description || t('workflows.noDescription')}</p>
                </div>
              </div>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

function ProvenancePill({ name, customized }: { name: string; customized?: boolean }) {
  const { t } = useTranslation()
  return (
    <span className="rounded-full bg-emerald-50 px-2 py-0.5 text-[11px] font-medium text-emerald-700 dark:bg-emerald-950/30 dark:text-emerald-300">
      {t('playbooks.fromPlaybook', { name })}
      {customized ? ` · ${t('playbooks.customized')}` : ''}
    </span>
  )
}

function workflowEditableJSON(workflow: WorkflowDefinition) {
  return JSON.stringify({
    name: workflow.name,
    description: workflow.description || '',
    startStepId: workflow.startStepId,
    steps: workflow.steps,
    edges: workflow.edges,
  })
}

function parseWorkflowJSON(raw: string): Pick<WorkflowDefinition, 'name' | 'description' | 'startStepId' | 'steps' | 'edges'> {
  const parsed = JSON.parse(raw) as unknown
  const source = normalizeWorkflowJSONSource(parsed)
  if (!source || typeof source !== 'object') {
    throw new Error('Invalid workflow JSON')
  }
  const record = source as Record<string, unknown>
  const steps = Array.isArray(record.steps) ? record.steps : []
  const edges = Array.isArray(record.edges) ? record.edges : []
  return {
    name: typeof record.name === 'string' ? record.name : '',
    description: typeof record.description === 'string' ? record.description : '',
    startStepId: typeof record.startStepId === 'string' ? record.startStepId : '',
    steps: steps.map((step, index) => normalizeImportedStep(step, index)),
    edges: edges.map((edge, index) => normalizeImportedEdge(edge, index)).filter((edge): edge is WorkflowDefinition['edges'][number] => Boolean(edge)),
  }
}

function normalizeWorkflowJSONSource(value: unknown): unknown {
  if (!value || typeof value !== 'object') return value
  const record = value as Record<string, unknown>
  if (record.workflow) return record.workflow
  if (record.definition) return record.definition
  return value
}

function normalizeImportedStep(value: unknown, index: number): WorkflowStep {
  if (!value || typeof value !== 'object') {
    return { id: `step_${index + 1}`, type: 'agent_task', title: `Step ${index + 1}`, actorRole: 'agent', position: { x: 80 + index * 280, y: 180 } }
  }
  const record = value as Record<string, unknown>
  const id = typeof record.id === 'string' && record.id.trim() ? record.id.trim() : `step_${index + 1}`
  const position = normalizeImportedPosition(record.position, index)
  return {
    id,
    type: record.type === 'human_review' ? 'human_review' : 'agent_task',
    title: typeof record.title === 'string' && record.title.trim() ? record.title.trim() : id,
    description: typeof record.description === 'string' ? record.description : '',
    actorRole: typeof record.actorRole === 'string' ? record.actorRole : typeof record.actor_role === 'string' ? record.actor_role : '',
    inputFields: normalizeImportedFields(record.inputFields ?? record.input_fields),
    outputFields: normalizeImportedFields(record.outputFields ?? record.output_fields),
    reviewPolicy: record.type === 'human_review' ? 'manual' : '',
    position,
    config: normalizeImportedConfig(record.config),
  }
}

function normalizeImportedPosition(value: unknown, index: number) {
  if (value && typeof value === 'object') {
    const record = value as Record<string, unknown>
    return {
      x: typeof record.x === 'number' ? record.x : 80 + index * 280,
      y: typeof record.y === 'number' ? record.y : 180,
    }
  }
  return { x: 80 + index * 280, y: 180 }
}

function normalizeImportedFields(value: unknown) {
  if (!Array.isArray(value)) return []
  return value
    .map((item) => {
      if (typeof item === 'string') return { name: item }
      if (!item || typeof item !== 'object') return null
      const record = item as Record<string, unknown>
      const name = typeof record.name === 'string' ? record.name.trim() : ''
      if (!name) return null
      return { name, description: typeof record.description === 'string' ? record.description : '' }
    })
    .filter((item): item is { name: string; description?: string } => Boolean(item))
}

function normalizeImportedConfig(value: unknown) {
  if (!value || typeof value !== 'object') return undefined
  const out: Record<string, string> = {}
  for (const [key, val] of Object.entries(value as Record<string, unknown>)) {
    if (typeof val === 'string') out[key] = val
  }
  return Object.keys(out).length > 0 ? out : undefined
}

function normalizeImportedEdge(value: unknown, index: number): WorkflowDefinition['edges'][number] | null {
  if (!value || typeof value !== 'object') return null
  const record = value as Record<string, unknown>
  const from = typeof record.from === 'string' ? record.from.trim() : typeof record.source === 'string' ? record.source.trim() : ''
  const to = typeof record.to === 'string' ? record.to.trim() : typeof record.target === 'string' ? record.target.trim() : ''
  if (!from || !to) return null
  return {
    id: typeof record.id === 'string' && record.id.trim() ? record.id.trim() : `edge_${index + 1}`,
    from,
    to,
    label: typeof record.label === 'string' ? record.label : '',
    condition: normalizeImportedCondition(record.condition),
    inputMapping: normalizeImportedMapping(record.inputMapping ?? record.input_mapping),
    isDefault: Boolean(record.isDefault ?? record.is_default),
  }
}

function normalizeImportedCondition(value: unknown) {
  if (!value || typeof value !== 'object') return undefined
  const record = value as Record<string, unknown>
  const field = typeof record.field === 'string' ? record.field : ''
  const operator = typeof record.operator === 'string' ? record.operator : ''
  const val = typeof record.value === 'string' ? record.value : ''
  const values = Array.isArray(record.values) ? record.values.filter((item): item is string => typeof item === 'string') : undefined
  if (!field && !operator && !val && (!values || values.length === 0)) return undefined
  return { field, operator, value: val, values }
}

function normalizeImportedMapping(value: unknown) {
  if (!value || typeof value !== 'object') return undefined
  const out: Record<string, string> = {}
  for (const [key, val] of Object.entries(value as Record<string, unknown>)) {
    if (typeof val === 'string') out[key] = val
  }
  return Object.keys(out).length > 0 ? out : undefined
}

function Loading({ label }: { label: string }) {
  return (
    <div className="flex items-center justify-center gap-2 py-16">
      <div className="size-5 animate-spin rounded-full border-2 border-neutral-300 border-t-sky-600 dark:border-zinc-600 dark:border-t-sky-400" />
      <span className="text-sm text-neutral-500">{label}</span>
    </div>
  )
}
