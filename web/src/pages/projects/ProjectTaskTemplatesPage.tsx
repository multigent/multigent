import { useMemo, useState, type FormEvent, type ReactNode } from 'react'
import { useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Copy, FileText, Trash2 } from 'lucide-react'
import { confirmDialog } from '../../components/ui/ConfirmDialog'
import { PlaceholderCard } from '../../components/ui/PlaceholderCard'
import { showToast } from '../../components/ui/Toast'
import { apiDelete, apiPost, apiPut } from '../../lib/api'
import { cn } from '../../lib/cn'
import { useApiJson } from '../../lib/use-api'

const TASK_TYPES = ['chore', 'feature', 'bug', 'review', 'triage', 'test', 'research'] as const

type ActorBinding = { type: 'agent' | 'human'; id: string }
type AgentRow = { name: string; model?: string }
type WorkflowStep = { id: string; type: string; title: string; actorRole?: string }
type WorkflowRow = { id: string; name: string; steps?: WorkflowStep[] }
type WorkflowListResponse = { workflows: WorkflowRow[] }
type TaskTemplateVariable = { name: string; description?: string; required?: boolean; default?: string }
type TaskTemplate = {
  id: string
  name: string
  description?: string
  project: string
  type?: string
  priority: number
  labels?: string[]
  titleTemplate: string
  descriptionTemplate?: string
  promptTemplate: string
  workflowDefinitionId?: string
  workflowActorBindings?: Record<string, ActorBinding>
  variables?: TaskTemplateVariable[]
}
type TaskTemplateListResponse = { templates: TaskTemplate[] }

type FormState = {
  name: string
  description: string
  type: string
  priority: number
  labels: string
  titleTemplate: string
  descriptionTemplate: string
  promptTemplate: string
  workflowDefinitionId: string
  workflowActorBindings: Record<string, ActorBinding>
  variables: TaskTemplateVariable[]
}

const emptyForm: FormState = {
  name: '',
  description: '',
  type: 'chore',
  priority: 2,
  labels: '',
  titleTemplate: '',
  descriptionTemplate: '',
  promptTemplate: '',
  workflowDefinitionId: '',
  workflowActorBindings: {},
  variables: [],
}

const buttonCls =
  'rounded-lg border border-sky-600 bg-white px-3 py-2 text-sm font-medium text-sky-700 hover:bg-sky-50 dark:border-sky-500 dark:bg-zinc-900 dark:text-sky-400 dark:hover:bg-zinc-800'
const fieldCls =
  'mt-1 w-full rounded-lg border border-neutral-300 bg-white px-2.5 py-1.5 text-sm text-neutral-900 outline-none transition-colors focus:border-sky-400 dark:border-zinc-600 dark:bg-zinc-800 dark:text-zinc-100'

export default function ProjectTaskTemplatesPage() {
  const { t } = useTranslation()
  const { projectId } = useParams<{ projectId: string }>()
  const [reloadKey, setReloadKey] = useState(0)
  const [open, setOpen] = useState(false)
  const [editingTemplate, setEditingTemplate] = useState<TaskTemplate | null>(null)
  const [form, setForm] = useState<FormState>(emptyForm)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  const project = projectId ?? ''
  const templatesState = useApiJson<TaskTemplateListResponse>(
    project ? `/api/v1/projects/${encodeURIComponent(project)}/task-templates` : null,
    reloadKey,
  )
  const workflowsState = useApiJson<WorkflowListResponse>('/api/v1/workflows', reloadKey)
  const agentsState = useApiJson<AgentRow[]>(project ? `/api/v1/projects/${encodeURIComponent(project)}/agents` : null, reloadKey)

  const templates = templatesState.status === 'ok' ? templatesState.data.templates : []
  const workflows = workflowsState.status === 'ok' ? workflowsState.data.workflows : []
  const members = agentsState.status === 'ok' ? agentsState.data : []
  const agentMembers = members.filter((m) => m.model !== 'human')
  const humanMembers = members.filter((m) => m.model === 'human')
  const selectedWorkflow = workflows.find((workflow) => workflow.id === form.workflowDefinitionId)

  const workflowSlots = useMemo(() => {
    const byRole = new Map<string, { role: string; preferredType: 'agent' | 'human'; titles: string[] }>()
    for (const step of selectedWorkflow?.steps ?? []) {
      const role = step.actorRole?.trim()
      if (!role) continue
      const preferredType = step.type === 'human_review' ? 'human' : 'agent'
      const existing = byRole.get(role)
      if (existing) {
        existing.titles.push(step.title)
        if (preferredType === 'human') existing.preferredType = 'human'
      } else {
        byRole.set(role, { role, preferredType, titles: [step.title] })
      }
    }
    return Array.from(byRole.values())
  }, [selectedWorkflow])

  function patchForm(patch: Partial<FormState>) {
    setForm((current) => ({ ...current, ...patch }))
  }

  function defaultBinding(type: 'agent' | 'human'): ActorBinding {
    if (type === 'human') return { type, id: humanMembers[0]?.name || '' }
    return { type, id: agentMembers[0]?.name || '' }
  }

  function onWorkflowChange(id: string) {
    const workflow = workflows.find((item) => item.id === id)
    const bindings: Record<string, ActorBinding> = {}
    for (const step of workflow?.steps ?? []) {
      const role = step.actorRole?.trim()
      if (!role || bindings[role]) continue
      bindings[role] = defaultBinding(step.type === 'human_review' ? 'human' : 'agent')
    }
    patchForm({ workflowDefinitionId: id, workflowActorBindings: bindings })
  }

  function updateBinding(role: string, patch: Partial<ActorBinding>) {
    patchForm({
      workflowActorBindings: {
        ...form.workflowActorBindings,
        [role]: {
          ...(form.workflowActorBindings[role] ?? { type: 'agent', id: '' }),
          ...patch,
          ...(patch.type ? defaultBinding(patch.type) : {}),
        },
      },
    })
  }

  function addVariable() {
    patchForm({ variables: [...form.variables, { name: '', description: '', required: true, default: '' }] })
  }

  function updateVariable(index: number, patch: Partial<TaskTemplateVariable>) {
    patchForm({ variables: form.variables.map((item, i) => (i === index ? { ...item, ...patch } : item)) })
  }

  function removeVariable(index: number) {
    patchForm({ variables: form.variables.filter((_, i) => i !== index) })
  }

  async function copyID(id: string) {
    await navigator.clipboard.writeText(id)
    showToast(t('taskTemplates.idCopied'), 'success')
  }

  async function deleteTemplate(template: TaskTemplate) {
    const ok = await confirmDialog({
      title: t('taskTemplates.deleteTitle'),
      description: t('taskTemplates.deleteConfirm', { name: template.name }),
      confirmLabel: t('common.delete'),
      cancelLabel: t('common.cancel'),
    })
    if (!ok) return
    await apiDelete(`/api/v1/task-templates/${encodeURIComponent(template.id)}`)
    setReloadKey((key) => key + 1)
  }

  function openCreateDialog() {
    setEditingTemplate(null)
    setForm(emptyForm)
    setErr(null)
    setOpen(true)
  }

  function openEditDialog(template: TaskTemplate) {
    setEditingTemplate(template)
    setForm({
      name: template.name,
      description: template.description ?? '',
      type: template.type || 'chore',
      priority: template.priority ?? 2,
      labels: (template.labels ?? []).join(', '),
      titleTemplate: template.titleTemplate,
      descriptionTemplate: template.descriptionTemplate ?? '',
      promptTemplate: template.promptTemplate,
      workflowDefinitionId: template.workflowDefinitionId ?? '',
      workflowActorBindings: template.workflowActorBindings ?? {},
      variables: template.variables ?? [],
    })
    setErr(null)
    setOpen(true)
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setErr(null)
    if (!form.name.trim() || !form.titleTemplate.trim() || !form.promptTemplate.trim()) {
      setErr(t('forms.fillRequired'))
      return
    }
    const missingActor = workflowSlots.find((slot) => !form.workflowActorBindings[slot.role]?.id)
    if (missingActor) {
      setErr(t('workflows.actorBindingsRequired'))
      return
    }
    setBusy(true)
    try {
      const body = {
        name: form.name.trim(),
        description: form.description.trim(),
        type: form.type,
        priority: form.priority,
        labels: form.labels.split(',').map((label) => label.trim()).filter(Boolean),
        titleTemplate: form.titleTemplate.trim(),
        descriptionTemplate: form.descriptionTemplate.trim(),
        promptTemplate: form.promptTemplate.trim(),
        workflowDefinitionId: form.workflowDefinitionId,
        workflowActorBindings: form.workflowDefinitionId ? form.workflowActorBindings : undefined,
        variables: form.variables
          .map((variable) => ({
            name: variable.name.trim(),
            description: variable.description?.trim() ?? '',
            required: Boolean(variable.required),
            default: variable.default?.trim() ?? '',
          }))
          .filter((variable) => variable.name),
      }
      if (editingTemplate) {
        await apiPut<TaskTemplate>(`/api/v1/task-templates/${encodeURIComponent(editingTemplate.id)}`, {
          ...body,
          project,
        })
      } else {
        await apiPost<TaskTemplate>(`/api/v1/projects/${encodeURIComponent(project)}/task-templates`, body)
      }
      setOpen(false)
      setEditingTemplate(null)
      setForm(emptyForm)
      setReloadKey((key) => key + 1)
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <div className="shrink-0 px-6 pt-5 pb-3">
        <div className="flex items-center justify-between gap-4">
          <div>
            <h1 className="text-xl font-semibold text-neutral-900 dark:text-zinc-100">{t('projectNav.taskTemplates')}</h1>
            <p className="mt-0.5 text-sm text-neutral-500 dark:text-zinc-500">{t('taskTemplates.projectSubtitle')}</p>
          </div>
          <button type="button" onClick={openCreateDialog} className={buttonCls}>
            {t('taskTemplates.create')}
          </button>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto px-6 py-3">
        {templatesState.status === 'loading' && <p className="py-12 text-center text-sm text-neutral-500">{t('api.loading')}</p>}
        {templatesState.status === 'error' && (
          <PlaceholderCard title={t('api.loadError')}>
            <p>{templatesState.error.message}</p>
          </PlaceholderCard>
        )}
        {templatesState.status === 'ok' && templates.length === 0 && (
          <div className="flex flex-col items-center justify-center py-20 text-center">
            <div className="mb-4 flex size-14 items-center justify-center rounded-2xl bg-neutral-100 dark:bg-zinc-800/50">
              <FileText className="size-6 text-neutral-400 dark:text-zinc-500" strokeWidth={1.5} />
            </div>
            <p className="text-base font-medium text-neutral-700 dark:text-zinc-300">{t('taskTemplates.emptyTitle')}</p>
            <p className="mt-1 text-sm text-neutral-400 dark:text-zinc-500">{t('taskTemplates.emptyHint')}</p>
          </div>
        )}
        {templatesState.status === 'ok' && templates.length > 0 && (
          <div className="overflow-x-auto rounded-lg border border-neutral-200 bg-white dark:border-zinc-700 dark:bg-zinc-900">
            <table className="min-w-full divide-y divide-neutral-200 dark:divide-zinc-800">
              <thead className="bg-neutral-50 dark:bg-zinc-950/50">
                <tr>
                  <th className="px-4 py-2 text-left text-xs font-medium text-neutral-500 dark:text-zinc-500">{t('taskTemplates.name')}</th>
                  <th className="whitespace-nowrap px-4 py-2 text-left text-xs font-medium text-neutral-500 dark:text-zinc-500">{t('taskTemplates.templateId')}</th>
                  <th className="px-4 py-2 text-left text-xs font-medium text-neutral-500 dark:text-zinc-500">{t('workflows.taskWorkflow')}</th>
                  <th className="px-4 py-2 text-left text-xs font-medium text-neutral-500 dark:text-zinc-500">{t('taskTemplates.variables')}</th>
                  <th className="sticky right-0 w-32 whitespace-nowrap bg-neutral-50 px-4 py-2 text-right text-xs font-medium text-neutral-500 dark:bg-zinc-950 dark:text-zinc-500">{t('common.actions')}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-neutral-100 dark:divide-zinc-800/80">
                {templates.map((template) => {
                  const workflow = workflows.find((item) => item.id === template.workflowDefinitionId)
                  return (
                    <tr key={template.id} className="hover:bg-neutral-50/80 dark:hover:bg-zinc-800/40">
                      <td className="px-4 py-3">
                        <div className="font-medium text-neutral-900 dark:text-zinc-100">{template.name}</div>
                        {template.description ? <div className="mt-0.5 text-xs text-neutral-400 dark:text-zinc-500">{template.description}</div> : null}
                      </td>
                      <td className="whitespace-nowrap px-4 py-3">
                        <button
                          type="button"
                          onClick={() => void copyID(template.id)}
                          className="inline-flex whitespace-nowrap items-center gap-1 rounded-md bg-neutral-100 px-2 py-1 font-mono text-xs text-neutral-600 hover:bg-neutral-200 dark:bg-zinc-800 dark:text-zinc-300 dark:hover:bg-zinc-700"
                          title={t('taskTemplates.copyId')}
                        >
                          {template.id}
                          <Copy className="size-3" strokeWidth={1.8} />
                        </button>
                      </td>
                      <td className="px-4 py-3 text-sm text-neutral-600 dark:text-zinc-400">
                        {workflow?.name || template.workflowDefinitionId || t('workflows.noWorkflow')}
                      </td>
                      <td className="px-4 py-3 text-sm text-neutral-600 dark:text-zinc-400">
                        {(template.variables ?? []).map((variable) => variable.name).join(', ') || '—'}
                      </td>
                      <td className="sticky right-0 w-32 whitespace-nowrap bg-white px-4 py-3 text-right dark:bg-zinc-900">
                        <button type="button" onClick={() => openEditDialog(template)} className="whitespace-nowrap rounded-md px-2 py-1 text-sm text-neutral-600 hover:bg-neutral-100 dark:text-zinc-300 dark:hover:bg-zinc-800">
                          {t('common.edit')}
                        </button>
                        <button type="button" onClick={() => void deleteTemplate(template)} className="whitespace-nowrap rounded-md px-2 py-1 text-sm text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-950/40">
                          {t('common.delete')}
                        </button>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {open ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/45 p-4" role="presentation" onClick={() => !busy && setOpen(false)}>
          <div className="max-h-[min(92vh,820px)] w-full max-w-3xl overflow-y-auto rounded-xl border border-neutral-200 bg-white shadow-lg dark:border-zinc-700 dark:bg-zinc-900 animate-scale-in" onClick={(e) => e.stopPropagation()} role="dialog">
            <div className="border-b border-neutral-200 px-4 py-3 dark:border-zinc-700">
              <h2 className="text-base font-semibold text-neutral-900 dark:text-zinc-100">{editingTemplate ? t('taskTemplates.edit') : t('taskTemplates.create')}</h2>
            </div>
            <form onSubmit={onSubmit} className="space-y-4 px-4 py-3">
              <div className="grid gap-3 sm:grid-cols-2">
                <Field label={t('taskTemplates.name')}>
                  <input value={form.name} onChange={(e) => patchForm({ name: e.target.value })} className={fieldCls} placeholder={t('taskTemplates.namePlaceholder')} />
                </Field>
                <Field label={t('forms.priority')}>
                  <select value={form.priority} onChange={(e) => patchForm({ priority: Number(e.target.value) })} className={fieldCls}>
                    {[0, 1, 2, 3].map((p) => <option key={p} value={p}>P{p} — {t(`forms.priorityLabel.${p}`)}</option>)}
                  </select>
                </Field>
              </div>
              <Field label={t('tasks.description')}>
                <textarea value={form.description} onChange={(e) => patchForm({ description: e.target.value })} rows={2} className={cn(fieldCls, 'resize-y')} />
              </Field>
              <div className="grid gap-3 sm:grid-cols-2">
                <Field label={t('forms.type')}>
                  <select value={form.type} onChange={(e) => patchForm({ type: e.target.value })} className={fieldCls}>
                    {TASK_TYPES.map((type) => <option key={type} value={type}>{t(`forms.taskType.${type}`)}</option>)}
                  </select>
                </Field>
                <Field label={t('tasks.labels')}>
                  <input value={form.labels} onChange={(e) => patchForm({ labels: e.target.value })} className={fieldCls} placeholder="github, release" />
                </Field>
              </div>

              <Field label={t('taskTemplates.titleTemplate')}>
                <input value={form.titleTemplate} onChange={(e) => patchForm({ titleTemplate: e.target.value })} className={fieldCls} placeholder={t('taskTemplates.titlePlaceholder')} />
              </Field>
              <Field label={t('taskTemplates.descriptionTemplate')}>
                <textarea value={form.descriptionTemplate} onChange={(e) => patchForm({ descriptionTemplate: e.target.value })} rows={2} className={cn(fieldCls, 'resize-y')} />
              </Field>
              <Field label={t('taskTemplates.promptTemplate')}>
                <textarea value={form.promptTemplate} onChange={(e) => patchForm({ promptTemplate: e.target.value })} rows={7} className={cn(fieldCls, 'resize-y font-mono')} placeholder={t('taskTemplates.promptPlaceholder')} />
              </Field>

              <Field label={t('workflows.taskWorkflow')}>
                <select value={form.workflowDefinitionId} onChange={(e) => onWorkflowChange(e.target.value)} className={fieldCls}>
                  <option value="">{t('workflows.noWorkflow')}</option>
                  {workflows.map((workflow) => <option key={workflow.id} value={workflow.id}>{workflow.name}</option>)}
                </select>
              </Field>

              {workflowSlots.length > 0 && (
                <div className="rounded-lg border border-neutral-200 p-3 dark:border-zinc-700">
                  <div className="text-sm font-medium text-neutral-800 dark:text-zinc-200">{t('workflows.actorBindings')}</div>
                  <div className="mt-2 grid gap-2">
                    {workflowSlots.map((slot) => {
                      const binding = form.workflowActorBindings[slot.role] ?? defaultBinding(slot.preferredType)
                      const options = binding.type === 'human' ? humanMembers : agentMembers
                      return (
                        <div key={slot.role} className="grid gap-2 sm:grid-cols-[1fr_110px_1.5fr] sm:items-center">
                          <div>
                            <div className="text-sm font-medium text-neutral-700 dark:text-zinc-300">{slot.role}</div>
                            <div className="text-xs text-neutral-400 dark:text-zinc-500">{slot.titles.join(' / ')}</div>
                          </div>
                          <select value={binding.type} onChange={(e) => updateBinding(slot.role, { type: e.target.value as 'agent' | 'human' })} className={fieldCls}>
                            <option value="agent">{t('forms.agent')}</option>
                            <option value="human">{t('taskTemplates.humanActor')}</option>
                          </select>
                          <select value={binding.id} onChange={(e) => updateBinding(slot.role, { id: e.target.value })} className={fieldCls}>
                            {options.map((member) => <option key={member.name} value={member.name}>{member.name}</option>)}
                            {options.length === 0 && (
                              <option value="" disabled>{binding.type === 'human' ? t('taskTemplates.noHumanActors') : t('taskTemplates.noAgentActors')}</option>
                            )}
                          </select>
                        </div>
                      )
                    })}
                  </div>
                </div>
              )}

              <div className="rounded-lg border border-neutral-200 p-3 dark:border-zinc-700">
                <div className="flex items-center justify-between gap-3">
                  <div>
                    <div className="text-sm font-medium text-neutral-800 dark:text-zinc-200">{t('taskTemplates.variables')}</div>
                    <p className="mt-0.5 text-xs text-neutral-400 dark:text-zinc-500">{t('taskTemplates.variablesHint')}</p>
                  </div>
                  <button type="button" onClick={addVariable} className="rounded-lg border border-neutral-200 px-2.5 py-1.5 text-sm text-neutral-700 hover:bg-neutral-50 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800">
                    {t('common.add')}
                  </button>
                </div>
                <div className="mt-2 space-y-2">
                  {form.variables.map((variable, index) => (
                    <div key={index} className="grid gap-2 rounded-lg bg-neutral-50 p-2 dark:bg-zinc-950/50 sm:grid-cols-[1fr_1.4fr_90px_32px]">
                      <input value={variable.name} onChange={(e) => updateVariable(index, { name: e.target.value })} className={fieldCls} placeholder={t('taskTemplates.variableName')} />
                      <input value={variable.description ?? ''} onChange={(e) => updateVariable(index, { description: e.target.value })} className={fieldCls} placeholder={t('taskTemplates.variableDescription')} />
                      <label className="mt-1 flex items-center gap-2 text-sm text-neutral-600 dark:text-zinc-400">
                        <input type="checkbox" checked={Boolean(variable.required)} onChange={(e) => updateVariable(index, { required: e.target.checked })} />
                        {t('taskTemplates.required')}
                      </label>
                      <button type="button" onClick={() => removeVariable(index)} className="mt-1 rounded-md px-2 py-1 text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-950/40">
                        <Trash2 className="size-4" strokeWidth={1.8} />
                      </button>
                    </div>
                  ))}
                </div>
              </div>

              {err ? <p className="rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-950/40 dark:text-red-300">{err}</p> : null}
              <div className="flex justify-end gap-2 border-t border-neutral-200 pt-3 dark:border-zinc-700">
                <button type="button" onClick={() => setOpen(false)} disabled={busy} className="rounded-lg border border-neutral-200 px-3 py-2 text-sm text-neutral-700 hover:bg-neutral-50 disabled:opacity-50 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800">
                  {t('common.cancel')}
                </button>
                <button type="submit" disabled={busy} className={cn(buttonCls, busy && 'opacity-60')}>
                  {t('common.save')}
                </button>
              </div>
            </form>
          </div>
        </div>
      ) : null}
    </div>
  )
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block text-sm">
      <span className="text-neutral-600 dark:text-zinc-400">{label}</span>
      {children}
    </label>
  )
}
