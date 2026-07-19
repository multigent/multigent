import { useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useParams } from 'react-router-dom'
import { ArrowLeft, CheckCircle2, GitBranch, LibraryBig, ListChecks, Puzzle, ShieldCheck, Users, Wrench } from 'lucide-react'
import { PlaceholderCard } from '../components/ui/PlaceholderCard'
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
  skills?: string[]
}
type PlaybookSkillTemplate = {
  id: string
  name: string
  description: string
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

type PlaybookListResponse = { templates: PlaybookTemplate[] }

const primaryButtonCls = 'rounded-lg border border-sky-600 bg-white px-3 py-2 text-sm font-medium text-sky-700 hover:bg-sky-50 disabled:opacity-50 dark:border-sky-500 dark:bg-zinc-900 dark:text-sky-400 dark:hover:bg-zinc-800'

export default function PlaybooksPage() {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const params = useParams()
  const locale = i18n.resolvedLanguage || i18n.language || 'en'
  const listState = useApiJson<PlaybookListResponse>(`/api/v1/playbook-templates?locale=${encodeURIComponent(locale)}`, 0)
  const detailState = useApiJson<PlaybookTemplate>(
    params.playbookId ? `/api/v1/playbook-templates/${encodeURIComponent(params.playbookId)}?locale=${encodeURIComponent(locale)}` : null,
    0,
  )
  const templates = listState.status === 'ok' ? listState.data.templates : []
  const selected = params.playbookId && detailState.status === 'ok' ? detailState.data : undefined

  if (params.playbookId) {
    return (
      <div className="animate-fade-in px-8 py-6">
        {detailState.status === 'loading' && <Loading label={t('api.loading')} />}
        {detailState.status === 'error' && (
          <PlaceholderCard title={t('api.loadError')}>
            <p className="text-[13px]">{detailState.error.message}</p>
          </PlaceholderCard>
        )}
        {selected && <PlaybookDetail playbook={selected} onBack={() => navigate('/playbooks')} />}
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

function PlaybookDetail({ playbook, onBack }: { playbook: PlaybookTemplate; onBack: () => void }) {
  const { t } = useTranslation()
  const workflows = playbook.workflows ?? []
  const firstWorkflow = workflows[0]
  const steps = firstWorkflow?.definition.steps ?? []
  const workflowSummary = useMemo(() => steps.slice(0, 6), [steps])

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
        <button type="button" disabled className={primaryButtonCls} title={t('playbooks.installSoon')}>
          {t('playbooks.install')}
        </button>
      </div>

      <div className="rounded-xl border border-neutral-200/80 bg-white p-5 dark:border-zinc-700/60 dark:bg-zinc-900/30">
        <PlaybookStats playbook={playbook} />
      </div>

      <Panel title={t('playbooks.roles')} icon={<Users className="size-4" strokeWidth={1.8} />}>
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {(playbook.roles ?? []).map((role) => (
            <div key={role.id} className="rounded-lg border border-neutral-200/80 bg-neutral-50/60 p-3 dark:border-zinc-700/50 dark:bg-zinc-900/50">
              <div className="flex items-center justify-between gap-2">
                <p className="text-sm font-semibold text-neutral-800 dark:text-zinc-100">{role.name}</p>
                <span className="rounded-full bg-white px-2 py-0.5 text-[11px] text-neutral-400 dark:bg-zinc-800 dark:text-zinc-500">{role.team}/{role.role}</span>
              </div>
              <p className="mt-1.5 text-sm leading-relaxed text-neutral-500 dark:text-zinc-500">{role.description}</p>
              {role.skills && role.skills.length > 0 && <TagRow values={role.skills} className="mt-3" />}
            </div>
          ))}
        </div>
      </Panel>

      <Panel title={t('playbooks.skills')} icon={<Puzzle className="size-4" strokeWidth={1.8} />}>
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {(playbook.skills ?? []).map((skill) => (
            <div key={skill.id} className="rounded-lg border border-neutral-200/80 bg-neutral-50/60 p-3 dark:border-zinc-700/50 dark:bg-zinc-900/50">
              <p className="text-sm font-semibold text-neutral-800 dark:text-zinc-100">{skill.name}</p>
              <p className="mt-1.5 text-sm leading-relaxed text-neutral-500 dark:text-zinc-500">{skill.description}</p>
            </div>
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
            <ol className="grid gap-2 lg:grid-cols-2">
              {workflowSummary.map((step, index) => (
                <li key={step.id} className="flex gap-3 rounded-lg border border-neutral-200/70 bg-white px-3 py-2 dark:border-zinc-700/50 dark:bg-zinc-900/40">
                  <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-sky-50 text-xs font-semibold text-sky-700 dark:bg-sky-950/40 dark:text-sky-300">{index + 1}</span>
                  <div className="min-w-0">
                    <p className="text-sm font-medium text-neutral-800 dark:text-zinc-100">{step.title}</p>
                    <p className="mt-0.5 line-clamp-2 text-xs leading-relaxed text-neutral-500 dark:text-zinc-500">{step.description || step.actorRole}</p>
                  </div>
                </li>
              ))}
            </ol>
            {steps.length > workflowSummary.length && (
              <p className="mt-2 text-xs text-neutral-400 dark:text-zinc-500">{t('playbooks.moreSteps', { count: steps.length - workflowSummary.length })}</p>
            )}
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
    </div>
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
