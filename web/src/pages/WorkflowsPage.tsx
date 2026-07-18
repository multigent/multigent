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
  const [saving, setSaving] = useState(false)
  const [fullscreen, setFullscreen] = useState(false)
  const [createOpen, setCreateOpen] = useState(false)
  const [createMode, setCreateMode] = useState<'blank' | 'template'>('template')
  const [selectedTemplateId, setSelectedTemplateId] = useState('')
  const [workflowName, setWorkflowName] = useState('')
  const descriptionRef = useRef<HTMLTextAreaElement | null>(null)

  useEffect(() => {
    setDraft(selected ? structuredClone(selected) : null)
    setFullscreen(false)
  }, [selected?.id])

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

  function openCreateDialog() {
    const firstTemplate = templates[0]
    setCreateMode(firstTemplate ? 'template' : 'blank')
    setSelectedTemplateId(firstTemplate?.id || '')
    setWorkflowName(firstTemplate?.name || t('workflows.untitledName'))
    setCreateOpen(true)
  }

  function updateSelectedTemplate(templateId: string) {
    const tmpl = templates.find((item) => item.id === templateId)
    setSelectedTemplateId(templateId)
    if (tmpl) setWorkflowName(tmpl.name)
  }

  async function submitCreate() {
    if (createMode === 'blank') {
      await createBlank(workflowName)
      setCreateOpen(false)
      return
    }
    if (!selectedTemplateId) return
    await createFromTemplate(selectedTemplateId, workflowName)
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
      if (params.workflowId === wf.id) navigate('/workflows')
      setReloadKey((key) => key + 1)
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
                </div>
                <div className="flex gap-2">
                  <button type="button" onClick={() => navigate('/workflows')} className="rounded-lg border border-neutral-300 bg-white px-3 py-2 text-sm font-medium text-neutral-600 hover:bg-neutral-50 dark:border-zinc-600 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:bg-zinc-800">
                    {t('workflows.backToList')}
                  </button>
                  <button type="button" onClick={() => void duplicateWorkflow(draft)} disabled={saving} className="rounded-lg border border-neutral-300 bg-white px-3 py-2 text-sm font-medium text-neutral-600 hover:bg-neutral-50 disabled:opacity-50 dark:border-zinc-600 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:bg-zinc-800">
                    {t('workflows.duplicate')}
                  </button>
                  <button type="button" onClick={() => void deleteWorkflow(draft)} disabled={saving} className="rounded-lg border border-red-300 bg-white px-3 py-2 text-sm font-medium text-red-600 hover:bg-red-50 disabled:opacity-50 dark:border-red-900/70 dark:bg-zinc-900 dark:text-red-400 dark:hover:bg-red-950/30">
                    {t('common.delete')}
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
              <div className="grid gap-3 sm:grid-cols-2">
                <button type="button" onClick={() => { setCreateMode('blank'); setWorkflowName(t('workflows.untitledName')) }} className={`rounded-xl border p-4 text-left ${createMode === 'blank' ? 'border-sky-500 bg-sky-50 dark:border-sky-500 dark:bg-sky-950/30' : 'border-neutral-200 hover:bg-neutral-50 dark:border-zinc-700 dark:hover:bg-zinc-800'}`}>
                  <p className="font-medium text-neutral-900 dark:text-zinc-100">{t('workflows.blankWorkflow')}</p>
                  <p className="mt-1 text-sm text-neutral-500 dark:text-zinc-400">{t('workflows.blankWorkflowHint')}</p>
                </button>
                <button type="button" onClick={() => { setCreateMode('template'); updateSelectedTemplate(selectedTemplateId || templates[0]?.id || '') }} disabled={templates.length === 0} className={`rounded-xl border p-4 text-left disabled:opacity-50 ${createMode === 'template' ? 'border-sky-500 bg-sky-50 dark:border-sky-500 dark:bg-sky-950/30' : 'border-neutral-200 hover:bg-neutral-50 dark:border-zinc-700 dark:hover:bg-zinc-800'}`}>
                  <p className="font-medium text-neutral-900 dark:text-zinc-100">{t('workflows.fromTemplate')}</p>
                  <p className="mt-1 text-sm text-neutral-500 dark:text-zinc-400">{t('workflows.fromTemplateHint')}</p>
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
                <label className="block">
                  <span className="text-xs font-medium uppercase text-neutral-400 dark:text-zinc-500">{t('workflows.workflowName')}</span>
                  <input value={workflowName} onChange={(event) => setWorkflowName(event.target.value)} className="mt-1 w-full rounded-lg border border-neutral-300 bg-white px-3 py-2 text-sm text-neutral-900 outline-none focus:border-sky-400 dark:border-zinc-600 dark:bg-zinc-800 dark:text-zinc-100" />
                </label>
            </div>
            <div className="flex justify-end gap-2 border-t border-neutral-200/80 px-5 py-3 dark:border-zinc-700/60">
              <button type="button" onClick={() => setCreateOpen(false)} disabled={saving} className="rounded-lg px-3 py-2 text-sm font-medium text-neutral-600 hover:bg-neutral-100 disabled:opacity-50 dark:text-zinc-400 dark:hover:bg-zinc-800">
                {t('common.cancel')}
              </button>
              <button type="button" onClick={() => void submitCreate()} disabled={saving || (createMode === 'template' && !selectedTemplateId) || !workflowName.trim()} className="rounded-lg bg-sky-600 px-3 py-2 text-sm font-medium text-white hover:bg-sky-700 disabled:opacity-50">
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
                    <p className="mt-0.5 text-xs text-neutral-400 dark:text-zinc-500">{t('workflows.stepCount', { count: wf.steps.length })}</p>
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

function Loading({ label }: { label: string }) {
  return (
    <div className="flex items-center justify-center gap-2 py-16">
      <div className="size-5 animate-spin rounded-full border-2 border-neutral-300 border-t-sky-600 dark:border-zinc-600 dark:border-t-sky-400" />
      <span className="text-sm text-neutral-500">{label}</span>
    </div>
  )
}
