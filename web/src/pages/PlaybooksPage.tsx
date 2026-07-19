import { useMemo, useState, type ReactNode } from 'react'
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'
import { useNavigate, useParams } from 'react-router-dom'
import { ArrowLeft, CheckCircle2, GitBranch, LibraryBig, ListChecks, Puzzle, ShieldCheck, Users, Wrench } from 'lucide-react'
import { PlaceholderCard } from '../components/ui/PlaceholderCard'
import { showToast } from '../components/ui/Toast'
import { apiPost } from '../lib/api'
import { cn } from '../lib/cn'
import { useApiJson } from '../lib/use-api'

type WorkflowField = { name: string; description?: string }
type WorkflowStep = {
  id: string
  type: string
  title: string
  description?: string
  actorRole?: string
  inputFields?: WorkflowField[]
  outputFields?: WorkflowField[]
}
type WorkflowTemplate = {
  id: string
  name: string
  description?: string
  steps: WorkflowStep[]
  edges: unknown[]
}
type PlaybookRoleTemplate = {
  id: string
  team: string
  role: string
  name: string
  description: string
  prompt?: string
  skills?: string[]
}
type PlaybookSkillTemplate = {
  id: string
  name: string
  description: string
  body?: string
  source?: string
}
type PlaybookWorkflowTemplate = {
  id: string
  name: string
  description: string
  definition: WorkflowTemplate
  roleBindings?: Record<string, string>
  skillBindings?: Record<string, string[]>
}
type PlaybookTaskTemplate = {
  id: string
  title: string
  description: string
  workflowId?: string
}
type PlaybookToolRequirement = {
  provider: string
  name: string
  description: string
  required: boolean
}
type PlaybookSetupQuestion = {
  id: string
  question: string
  description?: string
  options?: string[]
  required: boolean
}
type PlaybookMetric = {
  id: string
  name: string
  description: string
}
type PlaybookTemplate = {
  id: string
  name: string
  description: string
  locale: string
  category: string
  complexity: string
  tags?: string[]
  roles?: PlaybookRoleTemplate[]
  skills?: PlaybookSkillTemplate[]
  workflows?: PlaybookWorkflowTemplate[]
  taskTemplates?: PlaybookTaskTemplate[]
  requiredTools?: PlaybookToolRequirement[]
  setupQuestions?: PlaybookSetupQuestion[]
  successMetrics?: PlaybookMetric[]
}
type PlaybookInstalledObject = {
  type: string
  id: string
  name: string
  parentId?: string
  status: 'created' | 'existing'
}
type PlaybookInstall = {
  id: string
  playbookId: string
  playbookName: string
  locale: string
  createdBy: string
  createdAt: string
  objects: PlaybookInstalledObject[]
}
type ContentPreview = {
  title: string
  subtitle?: string
  label: string
  body: string
}

type PlaybookListResponse = { templates: PlaybookTemplate[] }
type PlaybookInstallsResponse = { installs: PlaybookInstall[] }
type PlaybookInstallResponse = { install: PlaybookInstall; alreadyInstalled?: boolean }

const primaryButtonCls = 'rounded-lg border border-sky-600 bg-white px-3 py-2 text-sm font-medium text-sky-700 hover:bg-sky-50 disabled:opacity-50 dark:border-sky-500 dark:bg-zinc-900 dark:text-sky-400 dark:hover:bg-zinc-800'

export default function PlaybooksPage() {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const params = useParams()
  const locale = i18n.resolvedLanguage || i18n.language || 'en'
  const listState = useApiJson<PlaybookListResponse>(`/api/v1/playbook-templates?locale=${encodeURIComponent(locale)}`, 0)
  const [installReloadKey, setInstallReloadKey] = useState(0)
  const installState = useApiJson<PlaybookInstallsResponse>('/api/v1/playbook-installs', installReloadKey)
  const detailState = useApiJson<PlaybookTemplate>(
    params.playbookId ? `/api/v1/playbook-templates/${encodeURIComponent(params.playbookId)}?locale=${encodeURIComponent(locale)}` : null,
    0,
  )
  const templates = listState.status === 'ok' ? listState.data.templates : []
  const selected = params.playbookId && detailState.status === 'ok' ? detailState.data : undefined
  const installs = installState.status === 'ok' ? installState.data.installs : []
  const installedByPlaybook = useMemo(() => new Map(installs.map((install) => [install.playbookId, install])), [installs])

  if (params.playbookId) {
    return (
      <div className="animate-fade-in px-8 py-6">
        {detailState.status === 'loading' && <Loading label={t('api.loading')} />}
        {detailState.status === 'error' && (
          <PlaceholderCard title={t('api.loadError')}>
            <p className="text-[13px]">{detailState.error.message}</p>
          </PlaceholderCard>
        )}
        {selected && (
          <PlaybookDetail
            playbook={selected}
            install={installedByPlaybook.get(selected.id)}
            onInstalled={() => setInstallReloadKey((v) => v + 1)}
            onBack={() => navigate('/playbooks')}
          />
        )}
      </div>
    )
  }

  return (
    <div className="animate-fade-in px-8 py-6">
      <div className="flex items-start justify-between gap-4 pb-5">
        <div>
          <h1 className="text-xl font-semibold text-neutral-900 dark:text-zinc-100">{t('nav.playbooks')}</h1>
          <p className="mt-0.5 text-sm text-neutral-500 dark:text-zinc-500">{t('playbooks.subtitle')}</p>
        </div>
        <button type="button" disabled className={primaryButtonCls} title={t('playbooks.installSoon')}>
          {t('playbooks.createCustom')}
        </button>
      </div>

      {listState.status === 'loading' && <Loading label={t('api.loading')} />}
      {listState.status === 'error' && (
        <PlaceholderCard title={t('api.loadError')}>
          <p className="text-[13px]">{listState.error.message}</p>
        </PlaceholderCard>
      )}
      {listState.status === 'ok' && templates.length === 0 && (
        <div className="flex flex-col items-center justify-center py-24 text-center">
          <div className="mb-4 flex size-16 items-center justify-center rounded-2xl bg-neutral-100 dark:bg-zinc-800/50">
            <LibraryBig className="size-7 text-neutral-400 dark:text-zinc-500" strokeWidth={1.5} />
          </div>
          <p className="text-lg font-medium text-neutral-600 dark:text-zinc-400">{t('playbooks.emptyTitle')}</p>
        </div>
      )}
      {listState.status === 'ok' && templates.length > 0 && (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {templates.map((playbook) => (
            <button
              type="button"
              key={playbook.id}
              onClick={() => navigate(`/playbooks/${encodeURIComponent(playbook.id)}`)}
              className="group flex min-h-[196px] flex-col justify-between rounded-xl border border-neutral-200/80 bg-white p-5 text-left transition-all duration-150 hover:border-sky-300/60 hover:shadow-md dark:border-zinc-700/60 dark:bg-zinc-900/30 dark:hover:border-sky-800/40"
            >
              <div>
                <div className="flex min-w-0 items-start gap-3">
                  <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-sky-100 dark:bg-sky-900/30">
                    <LibraryBig className="size-5 text-sky-600 dark:text-sky-400" strokeWidth={1.8} />
                  </div>
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <h2 className="line-clamp-1 text-sm font-semibold text-neutral-900 dark:text-zinc-100">{playbook.name}</h2>
                      <span className="rounded-full bg-neutral-100 px-2 py-0.5 text-[11px] font-medium text-neutral-500 dark:bg-zinc-800 dark:text-zinc-400">{playbook.category}</span>
                      {installedByPlaybook.has(playbook.id) && (
                        <span className="rounded-full bg-emerald-50 px-2 py-0.5 text-[11px] font-medium text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300">{t('playbooks.installed')}</span>
                      )}
                    </div>
                    <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">{playbook.complexity}</p>
                  </div>
                </div>
                <p className="mt-3 line-clamp-3 text-sm leading-relaxed text-neutral-500 dark:text-zinc-500">{playbook.description}</p>
              </div>
              <PlaybookStats playbook={playbook} className="mt-4" />
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

function PlaybookDetail({ playbook, install, onBack, onInstalled }: { playbook: PlaybookTemplate; install?: PlaybookInstall; onBack: () => void; onInstalled: () => void }) {
  const { t, i18n } = useTranslation()
  const [installing, setInstalling] = useState(false)
  const [preview, setPreview] = useState<ContentPreview | null>(null)
  const workflows = playbook.workflows ?? []
  const firstWorkflow = workflows[0]
  const steps = firstWorkflow?.definition.steps ?? []

  async function handleInstall() {
    if (installing || install) return
    setInstalling(true)
    try {
      const res = await apiPost<PlaybookInstallResponse>(`/api/v1/playbook-templates/${encodeURIComponent(playbook.id)}/install`, {
        locale: i18n.resolvedLanguage || i18n.language || 'en',
      })
      showToast(res.alreadyInstalled ? t('playbooks.alreadyInstalled') : t('playbooks.installSuccess'), 'success')
      onInstalled()
    } finally {
      setInstalling(false)
    }
  }

  return (
    <div className="space-y-5">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <div className="mb-2 flex items-center gap-2 text-sm text-neutral-400 dark:text-zinc-500">
            <button type="button" onClick={onBack} className="inline-flex items-center gap-1 rounded-md px-1.5 py-1 hover:bg-neutral-100 hover:text-neutral-700 dark:hover:bg-zinc-800 dark:hover:text-zinc-300">
              <ArrowLeft className="size-3.5" strokeWidth={1.8} />
              {t('playbooks.backToList')}
            </button>
          </div>
          <h1 className="text-xl font-semibold text-neutral-900 dark:text-zinc-100">{playbook.name}</h1>
          <p className="mt-1 max-w-4xl text-sm leading-relaxed text-neutral-500 dark:text-zinc-500">{playbook.description}</p>
        </div>
        <button type="button" disabled={installing || Boolean(install)} onClick={handleInstall} className={primaryButtonCls}>
          {install ? t('playbooks.installed') : installing ? t('playbooks.installing') : t('playbooks.install')}
        </button>
      </div>

      {install && <InstallSummary install={install} />}

      <div className="rounded-xl border border-neutral-200/80 bg-white p-5 dark:border-zinc-700/60 dark:bg-zinc-900/30">
        <PlaybookStats playbook={playbook} />
      </div>

      <Panel title={t('playbooks.roles')} icon={<Users className="size-4" strokeWidth={1.8} />}>
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {(playbook.roles ?? []).map((role) => (
            <ExpandableArtifactCard
              key={role.id}
              title={role.name}
              subtitle={`${role.team}/${role.role}`}
              description={role.description}
              tags={role.skills}
              onPreview={() => setPreview({
                title: role.name,
                subtitle: `${role.team}/${role.role}`,
                label: t('playbooks.rolePrompt'),
                body: String(role.prompt ?? '').trim() || t('playbooks.noPromptContent'),
              })}
            />
          ))}
        </div>
      </Panel>

      <Panel title={t('playbooks.skills')} icon={<Puzzle className="size-4" strokeWidth={1.8} />}>
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {(playbook.skills ?? []).map((skill) => (
            <ExpandableArtifactCard
              key={skill.id}
              title={skill.name}
              description={skill.description}
              onPreview={() => setPreview({
                title: skill.name,
                label: t('playbooks.skillPrompt'),
                body: String(skill.body ?? '').trim() || t('playbooks.noPromptContent'),
              })}
            />
          ))}
        </div>
      </Panel>

      <Panel title={t('playbooks.workflow')} icon={<GitBranch className="size-4" strokeWidth={1.8} />}>
        {firstWorkflow ? (
          <div>
            <div className="mb-3 rounded-lg bg-neutral-50 px-3 py-2 dark:bg-zinc-800/50">
              <p className="text-sm font-semibold text-neutral-800 dark:text-zinc-100">{firstWorkflow.name}</p>
              <p className="mt-1 text-sm leading-relaxed text-neutral-500 dark:text-zinc-500">{firstWorkflow.description}</p>
            </div>
            <ol className="grid gap-3 lg:grid-cols-2">
              {steps.map((step, index) => (
                <WorkflowStepCard
                  key={step.id}
                  step={step}
                  index={index}
                  onPreview={() => setPreview({
                    title: step.title,
                    subtitle: `${t(`workflows.stepTypes.${step.type}`, step.type)} · ${step.actorRole || t('playbooks.none')}`,
                    label: t('playbooks.workflowStepContent'),
                    body: formatWorkflowStepPreview(step, t),
                  })}
                />
              ))}
            </ol>
          </div>
        ) : (
          <p className="text-sm text-neutral-400 dark:text-zinc-500">{t('playbooks.none')}</p>
        )}
      </Panel>

      <Panel title={t('playbooks.taskTemplates')} icon={<ListChecks className="size-4" strokeWidth={1.8} />}>
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {(playbook.taskTemplates ?? []).map((task) => (
            <div key={task.id} className="rounded-lg border border-neutral-200/80 bg-neutral-50/60 p-3 dark:border-zinc-700/50 dark:bg-zinc-900/50">
              <p className="text-sm font-semibold text-neutral-800 dark:text-zinc-100">{task.title}</p>
              <p className="mt-1 text-sm leading-relaxed text-neutral-500 dark:text-zinc-500">{task.description}</p>
            </div>
          ))}
        </div>
      </Panel>

      <Panel title={t('playbooks.requiredTools')} icon={<Wrench className="size-4" strokeWidth={1.8} />}>
        {(playbook.requiredTools ?? []).length > 0 ? (
          <div className="grid gap-3 md:grid-cols-2">
            {(playbook.requiredTools ?? []).map((tool) => (
              <div key={tool.provider} className="flex items-start justify-between gap-3 rounded-lg border border-neutral-200/80 bg-neutral-50/60 p-3 dark:border-zinc-700/50 dark:bg-zinc-900/50">
                <div>
                  <p className="text-sm font-semibold text-neutral-800 dark:text-zinc-100">{tool.name}</p>
                  <p className="mt-1 text-sm leading-relaxed text-neutral-500 dark:text-zinc-500">{tool.description}</p>
                </div>
                <span className="shrink-0 rounded-full bg-white px-2 py-0.5 text-[11px] text-neutral-400 dark:bg-zinc-800 dark:text-zinc-500">
                  {tool.required ? t('playbooks.required') : t('playbooks.optional')}
                </span>
              </div>
            ))}
          </div>
        ) : (
          <p className="text-sm text-neutral-400 dark:text-zinc-500">{t('playbooks.noRequiredTools')}</p>
        )}
      </Panel>

      <div className="grid gap-5 lg:grid-cols-2">
        <Panel title={t('playbooks.setupQuestions')} icon={<ShieldCheck className="size-4" strokeWidth={1.8} />}>
          {(playbook.setupQuestions ?? []).length > 0 ? (
            <div className="space-y-3">
              {(playbook.setupQuestions ?? []).map((item) => (
                <div key={item.id} className="rounded-lg bg-neutral-50 px-3 py-2 dark:bg-zinc-800/50">
                  <p className="text-sm font-medium text-neutral-800 dark:text-zinc-100">{item.question}</p>
                  {item.options && item.options.length > 0 && <TagRow values={item.options} className="mt-2" />}
                </div>
              ))}
            </div>
          ) : (
            <p className="text-sm text-neutral-400 dark:text-zinc-500">{t('playbooks.none')}</p>
          )}
        </Panel>
        <Panel title={t('playbooks.successMetrics')} icon={<CheckCircle2 className="size-4" strokeWidth={1.8} />}>
          {(playbook.successMetrics ?? []).length > 0 ? (
            <div className="space-y-3">
              {(playbook.successMetrics ?? []).map((metric) => (
                <div key={metric.id} className="rounded-lg bg-neutral-50 px-3 py-2 dark:bg-zinc-800/50">
                  <p className="text-sm font-medium text-neutral-800 dark:text-zinc-100">{metric.name}</p>
                  <p className="mt-1 text-sm leading-relaxed text-neutral-500 dark:text-zinc-500">{metric.description}</p>
                </div>
              ))}
            </div>
          ) : (
            <p className="text-sm text-neutral-400 dark:text-zinc-500">{t('playbooks.none')}</p>
          )}
        </Panel>
      </div>
      {preview && <ContentPreviewModal preview={preview} onClose={() => setPreview(null)} />}
    </div>
  )
}

function ExpandableArtifactCard({
  title,
  subtitle,
  description,
  tags,
  onPreview,
}: {
  title: string
  subtitle?: string
  description?: string
  tags?: string[]
  onPreview: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className="rounded-lg border border-neutral-200/80 bg-neutral-50/60 p-3 dark:border-zinc-700/50 dark:bg-zinc-900/50">
      <div className="flex items-center justify-between gap-2">
        <p className="min-w-0 truncate text-sm font-semibold text-neutral-800 dark:text-zinc-100">{title}</p>
        {subtitle && <span className="shrink-0 rounded-full bg-white px-2 py-0.5 text-[11px] text-neutral-400 dark:bg-zinc-800 dark:text-zinc-500">{subtitle}</span>}
      </div>
      {description && <p className="mt-1.5 text-sm leading-relaxed text-neutral-500 dark:text-zinc-500">{description}</p>}
      {tags && tags.length > 0 && <TagRow values={tags} className="mt-3" />}
      <button
        type="button"
        onClick={onPreview}
        className="mt-3 rounded-lg border border-neutral-300 bg-white px-3 py-1.5 text-xs font-medium text-neutral-700 hover:bg-neutral-50 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:bg-zinc-800"
      >
        {t('playbooks.viewContent')}
      </button>
    </div>
  )
}

function WorkflowStepCard({ step, index, onPreview }: { step: WorkflowStep; index: number; onPreview: () => void }) {
  const { t } = useTranslation()
  return (
    <li className="rounded-lg border border-neutral-200/70 bg-white px-3 py-2 dark:border-zinc-700/50 dark:bg-zinc-900/40">
      <div className="flex gap-3">
        <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-sky-50 text-xs font-semibold text-sky-700 dark:bg-sky-950/40 dark:text-sky-300">{index + 1}</span>
        <div className="min-w-0 flex-1">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <p className="text-sm font-medium text-neutral-800 dark:text-zinc-100">{step.title}</p>
              <p className="mt-0.5 line-clamp-2 text-xs leading-relaxed text-neutral-500 dark:text-zinc-500">{step.description || step.actorRole}</p>
            </div>
            <button
              type="button"
              onClick={onPreview}
              className="shrink-0 rounded-lg border border-neutral-300 bg-white px-2.5 py-1 text-xs font-medium text-neutral-700 hover:bg-neutral-50 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:bg-zinc-800"
            >
              {t('playbooks.viewContent')}
            </button>
          </div>
        </div>
      </div>
    </li>
  )
}

function ContentPreviewModal({ preview, onClose }: { preview: ContentPreview; onClose: () => void }) {
  const { t } = useTranslation()
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/45 p-4" onClick={onClose}>
      <div
        className="flex max-h-[82vh] w-full max-w-3xl flex-col rounded-xl border border-neutral-200 bg-white shadow-xl dark:border-zinc-700 dark:bg-zinc-900"
        role="dialog"
        aria-modal="true"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="border-b border-neutral-100 px-5 py-4 dark:border-zinc-800">
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <h2 className="truncate text-base font-semibold text-neutral-900 dark:text-zinc-100">{preview.title}</h2>
              {preview.subtitle && <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">{preview.subtitle}</p>}
            </div>
            <button
              type="button"
              onClick={onClose}
              className="rounded-lg border border-neutral-300 bg-white px-3 py-1.5 text-xs font-medium text-neutral-700 hover:bg-neutral-50 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:bg-zinc-800"
            >
              {t('common.close')}
            </button>
          </div>
        </div>
        <div className="min-h-0 flex-1 overflow-auto p-5">
          <p className="mb-2 text-xs font-medium text-neutral-400 dark:text-zinc-500">{preview.label}</p>
          <pre className="whitespace-pre-wrap break-words rounded-lg border border-neutral-200 bg-neutral-50/80 p-4 text-xs leading-relaxed text-neutral-700 dark:border-zinc-700 dark:bg-zinc-950/30 dark:text-zinc-300">{preview.body}</pre>
        </div>
      </div>
    </div>
  )
}

function formatWorkflowStepPreview(step: WorkflowStep, t: TFunction) {
  const renderFields = (fields: WorkflowField[] | undefined) => {
    if (!fields || fields.length === 0) return t('playbooks.none')
    return fields.map((field) => `- ${field.description || field.name}${field.description ? ` (${field.name})` : ''}`).join('\n')
  }
  return [
    `${t('playbooks.stepType')}: ${t(`workflows.stepTypes.${step.type}`, step.type)}`,
    `${t('playbooks.actor')}: ${step.actorRole || t('playbooks.none')}`,
    '',
    `${t('playbooks.stepGoal')}:`,
    step.description || t('playbooks.none'),
    '',
    `${t('playbooks.inputFields')}:`,
    renderFields(step.inputFields),
    '',
    `${t('playbooks.outputFields')}:`,
    renderFields(step.outputFields),
  ].join('\n')
}

function InstallSummary({ install }: { install: PlaybookInstall }) {
  const { t } = useTranslation()
  const grouped = useMemo(() => {
    const out = new Map<string, PlaybookInstalledObject[]>()
    for (const obj of install.objects ?? []) {
      const items = out.get(obj.type) ?? []
      items.push(obj)
      out.set(obj.type, items)
    }
    return Array.from(out.entries())
  }, [install.objects])
  return (
    <section className="rounded-xl border border-emerald-200/80 bg-emerald-50/60 p-4 dark:border-emerald-900/60 dark:bg-emerald-950/20">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <p className="text-sm font-semibold text-emerald-900 dark:text-emerald-100">{t('playbooks.installRecord')}</p>
          <p className="mt-1 text-xs text-emerald-700/80 dark:text-emerald-300/80">
            {t('playbooks.installedBy', { user: install.createdBy || 'system' })}
          </p>
        </div>
        <span className="rounded-full bg-white px-2.5 py-1 text-xs font-medium text-emerald-700 ring-1 ring-emerald-200 dark:bg-emerald-950/30 dark:text-emerald-200 dark:ring-emerald-900">
          {install.objects?.length ?? 0} {t('playbooks.objects')}
        </span>
      </div>
      <div className="mt-3 flex flex-wrap gap-2">
        {grouped.map(([type, objects]) => (
          <span key={type} className="rounded-full bg-white px-2.5 py-1 text-xs text-emerald-800 ring-1 ring-emerald-200 dark:bg-zinc-900/40 dark:text-emerald-200 dark:ring-emerald-900/70">
            {t(`playbooks.objectTypes.${type}`, type)} · {objects.length}
          </span>
        ))}
      </div>
    </section>
  )
}

function PlaybookStats({ playbook, className }: { playbook: PlaybookTemplate; className?: string }) {
  const { t } = useTranslation()
  const stats = [
    { label: t('playbooks.roles'), value: playbook.roles?.length ?? 0 },
    { label: t('playbooks.skills'), value: playbook.skills?.length ?? 0 },
    { label: t('playbooks.workflows'), value: playbook.workflows?.length ?? 0 },
    { label: t('playbooks.taskTemplates'), value: playbook.taskTemplates?.length ?? 0 },
  ]
  return (
    <div className={cn('grid grid-cols-4 gap-2', className)}>
      {stats.map((stat) => (
        <div key={stat.label} className="rounded-lg bg-neutral-50 px-2.5 py-2 text-center dark:bg-zinc-800/50">
          <p className="text-sm font-semibold text-neutral-800 dark:text-zinc-100">{stat.value}</p>
          <p className="mt-0.5 truncate text-[11px] text-neutral-400 dark:text-zinc-500">{stat.label}</p>
        </div>
      ))}
    </div>
  )
}

function Panel({ title, icon, children }: { title: string; icon: ReactNode; children: ReactNode }) {
  return (
    <section className="rounded-xl border border-neutral-200/80 bg-white p-5 dark:border-zinc-700/60 dark:bg-zinc-900/30">
      <div className="mb-4 flex items-center gap-2 text-neutral-700 dark:text-zinc-200">
        <span className="text-sky-600 dark:text-sky-400">{icon}</span>
        <h2 className="text-sm font-semibold">{title}</h2>
      </div>
      {children}
    </section>
  )
}

function TagRow({ values, className }: { values: string[]; className?: string }) {
  return (
    <div className={cn('flex flex-wrap gap-1.5', className)}>
      {values.map((value) => (
        <span key={value} className="rounded-full bg-white px-2 py-0.5 text-[11px] font-medium text-neutral-500 ring-1 ring-neutral-200 dark:bg-zinc-800 dark:text-zinc-400 dark:ring-zinc-700">
          {value}
        </span>
      ))}
    </div>
  )
}

function Loading({ label }: { label: string }) {
  return (
    <div className="flex items-center justify-center py-16 text-sm text-neutral-500 dark:text-zinc-400">
      <div className="mr-2 size-4 animate-spin rounded-full border-2 border-neutral-300 border-t-sky-500 dark:border-zinc-700 dark:border-t-sky-400" />
      {label}
    </div>
  )
}
