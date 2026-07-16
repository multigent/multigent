import { useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Users, Puzzle, ArrowRight, User, Database, X } from 'lucide-react'
import { PlaceholderCard } from '../../components/ui/PlaceholderCard'
import { useApiJson } from '../../lib/use-api'
import { apiPost } from '../../lib/api'
import { primaryOutlineButton } from '../../lib/button-styles'

type TeamRow = {
  path: string
  name: string
  description?: string
  owners?: string[]
  defaultContextPack?: string
  skills?: string[]
}

type TeamTemplate = {
  id: string
  name: string
  description: string
  teamName: string
  roles: Array<{ name: string; description: string }>
}

export default function TeamsPage() {
  const { t, i18n } = useTranslation()
  const [reloadKey, setReloadKey] = useState(0)
  const state = useApiJson<TeamRow[]>('/api/v1/teams', reloadKey)
  const templateState = useApiJson<TeamTemplate[]>('/api/v1/team-templates', reloadKey)

  return (
    <div className="animate-fade-in px-8 py-6">
      <div className="pb-5">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <h1 className="text-xl font-semibold text-neutral-900 dark:text-zinc-100">{t('nav.teams')}</h1>
            <p className="mt-0.5 text-sm text-neutral-500 dark:text-zinc-500">{t('teams.subtitle')}</p>
          </div>
          <CreateTeamDialog
            templates={templateState.status === 'ok' ? templateState.data : []}
            locale={i18n.language}
            onCreated={() => setReloadKey((k) => k + 1)}
          />
        </div>
      </div>

      {state.status === 'loading' && (
        <div className="flex items-center gap-2 py-16 justify-center">
          <div className="size-5 animate-spin rounded-full border-2 border-neutral-300 border-t-sky-600 dark:border-zinc-600 dark:border-t-sky-400" />
          <span className="text-sm text-neutral-500">{t('api.loading')}</span>
        </div>
      )}
      {state.status === 'error' && (
        <PlaceholderCard title={t('api.loadError')}>
          <p className="text-[13px]">{state.error.message}</p>
          <p className="mt-1.5 text-[11px] text-neutral-400 dark:text-zinc-500">{t('api.hintServe')}</p>
        </PlaceholderCard>
      )}
      {state.status === 'ok' && state.data.length === 0 && (
        <div className="flex flex-col items-center justify-center py-24 text-center">
          <div className="mb-4 flex size-16 items-center justify-center rounded-2xl bg-neutral-100 dark:bg-zinc-800/50">
            <Users className="size-7 text-neutral-400 dark:text-zinc-500" strokeWidth={1.5} />
          </div>
          <p className="text-lg font-medium text-neutral-600 dark:text-zinc-400">{t('teams.placeholderTitle')}</p>
          <p className="mt-1.5 text-sm text-neutral-400 dark:text-zinc-500">{t('api.noTeams')}</p>
        </div>
      )}
      {state.status === 'ok' && state.data.length > 0 && (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {state.data.map((row) => (
            <Link
              key={row.path}
              to={`/teams/${encodeURIComponent(row.path)}`}
              className="group flex flex-col justify-between rounded-xl border border-neutral-200/80 bg-white p-5 transition-all duration-150 hover:border-sky-300/60 hover:shadow-md dark:border-zinc-700/60 dark:bg-zinc-900/30 dark:hover:border-sky-800/40"
            >
              <div>
                <div className="flex items-center gap-2.5">
                  <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-sky-100 dark:bg-sky-900/30">
                    <Users className="size-4.5 text-sky-600 dark:text-sky-400" strokeWidth={1.8} />
                  </div>
                  <div className="min-w-0">
                    <h3 className="text-sm font-semibold text-neutral-900 dark:text-zinc-100">{row.name}</h3>
                    <p className="font-mono text-xs text-neutral-400 dark:text-zinc-500">{row.path}</p>
                  </div>
                </div>
                {row.description && (
                  <p className="mt-2.5 text-sm leading-relaxed text-neutral-500 dark:text-zinc-500 line-clamp-2">{row.description}</p>
                )}
                <div className="mt-2.5 flex flex-wrap gap-1.5">
                  {row.owners?.map((owner) => (
                    <span key={owner} className="inline-flex items-center gap-1 rounded-md bg-violet-50 px-2 py-0.5 text-[11px] font-medium text-violet-700 dark:bg-violet-900/20 dark:text-violet-300">
                      <User className="size-3" strokeWidth={2} />
                      {owner}
                    </span>
                  ))}
                  {row.defaultContextPack && (
                    <span className="inline-flex items-center gap-1 rounded-md bg-emerald-50 px-2 py-0.5 text-[11px] font-medium text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300">
                      <Database className="size-3" strokeWidth={2} />
                      {row.defaultContextPack}
                    </span>
                  )}
                </div>
                {row.skills && row.skills.length > 0 && (
                  <div className="mt-2.5 flex flex-wrap gap-1.5">
                    {row.skills.map((sk) => (
                      <span key={sk} className="inline-flex items-center gap-1 rounded-md bg-amber-50 px-2 py-0.5 text-[11px] font-medium text-amber-700 dark:bg-amber-900/20 dark:text-amber-400">
                        <Puzzle className="size-3" strokeWidth={2} />
                        {sk}
                      </span>
                    ))}
                  </div>
                )}
              </div>
              <div className="mt-4 flex items-center justify-end">
                <span className="flex items-center gap-1 text-xs font-medium text-sky-600 opacity-0 transition-opacity group-hover:opacity-100 dark:text-sky-400">
                  <ArrowRight className="size-3.5" strokeWidth={2} />
                </span>
              </div>
            </Link>
          ))}
        </div>
      )}
    </div>
  )
}

const fieldCls =
  'mt-1 w-full rounded-lg border border-neutral-300 bg-white px-2.5 py-1.5 text-sm text-neutral-900 outline-none transition-colors focus:border-sky-400 dark:border-zinc-600 dark:bg-zinc-800 dark:text-zinc-100'

function CreateTeamDialog({ templates, locale, onCreated }: { templates: TeamTemplate[]; locale: string; onCreated: () => void }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [templateId, setTemplateId] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const selectedTemplate = templates.find((template) => template.id === templateId)

  function reset() {
    setName('')
    setDescription('')
    setTemplateId('')
    setErr(null)
  }

  function openDialog() {
    reset()
    setOpen(true)
  }

  function chooseTemplate(id: string) {
    setTemplateId(id)
    const template = templates.find((item) => item.id === id)
    if (template && name.trim() === '') {
      setName(template.teamName)
    }
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setErr(null)
    const trimmedName = name.trim()
    if (!trimmedName) {
      setErr(t('forms.fillRequired'))
      return
    }
    setBusy(true)
    try {
      await apiPost('/api/v1/teams', {
        name: trimmedName,
        description: description.trim(),
        templateId: templateId || undefined,
        locale,
      })
      setOpen(false)
      onCreated()
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <button
        type="button"
        onClick={openDialog}
        className={primaryOutlineButton}
      >
        {t('teams.createTeam')}
      </button>
      {open && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/45 p-4"
          role="presentation"
          onClick={() => !busy && setOpen(false)}
        >
          <div
            className="max-h-[min(90vh,760px)] w-full max-w-2xl overflow-y-auto rounded-xl border border-neutral-200 bg-white shadow-lg dark:border-zinc-700 dark:bg-zinc-900 animate-scale-in"
            onClick={(e) => e.stopPropagation()}
            role="dialog"
            aria-labelledby="create-team-title"
          >
            <div className="flex items-start justify-between gap-3 border-b border-neutral-200 px-4 py-3 dark:border-zinc-700">
              <div>
                <h2 id="create-team-title" className="text-base font-semibold text-neutral-900 dark:text-zinc-100">
                  {t('teams.createTeam')}
                </h2>
                <p className="mt-0.5 text-xs text-neutral-400 dark:text-zinc-500">{t('teams.createTeamDesc')}</p>
              </div>
              <button
                type="button"
                onClick={() => setOpen(false)}
                disabled={busy}
                className="rounded-md p-1 text-neutral-400 hover:bg-neutral-100 hover:text-neutral-700 dark:hover:bg-zinc-800 dark:hover:text-zinc-200"
                aria-label={t('forms.cancel')}
              >
                <X className="size-4" strokeWidth={2} />
              </button>
            </div>
            <form onSubmit={onSubmit} className="space-y-4 px-4 py-4">
              <label className="block text-sm">
                <span className="text-neutral-600 dark:text-zinc-400">{t('teams.template')}</span>
                <select
                  value={templateId}
                  onChange={(e) => chooseTemplate(e.target.value)}
                  className={fieldCls}
                >
                  <option value="">{t('teams.blankTeam')}</option>
                  {templates.map((template) => (
                    <option key={template.id} value={template.id}>
                      {templateTitle(template, t)}
                    </option>
                  ))}
                </select>
              </label>

              <div className="rounded-lg border border-neutral-200 bg-neutral-50 p-3 dark:border-zinc-700 dark:bg-zinc-950/40">
                <div className="text-sm font-semibold text-neutral-900 dark:text-zinc-100">
                  {selectedTemplate ? templateTitle(selectedTemplate, t) : t('teams.blankTeam')}
                </div>
                <div className="mt-1 text-xs leading-relaxed text-neutral-500 dark:text-zinc-500">
                  {selectedTemplate ? templateDescription(selectedTemplate, t) : t('teams.blankTeamDesc')}
                </div>
              </div>

              <label className="block text-sm">
                <span className="text-neutral-600 dark:text-zinc-400">{t('teams.teamName')} *</span>
                <input
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  className={fieldCls}
                  placeholder={selectedTemplate?.teamName || 'engineering'}
                  autoFocus
                />
              </label>

              {!selectedTemplate && (
                <label className="block text-sm">
                  <span className="text-neutral-600 dark:text-zinc-400">{t('teams.teamDescription')}</span>
                  <input
                    value={description}
                    onChange={(e) => setDescription(e.target.value)}
                    className={fieldCls}
                    placeholder={t('teams.teamDescPlaceholder')}
                  />
                </label>
              )}

              {selectedTemplate && (
                <div className="rounded-lg border border-neutral-200 bg-neutral-50 p-3 dark:border-zinc-700 dark:bg-zinc-950/40">
                  <div className="text-xs font-medium text-neutral-500 dark:text-zinc-400">{t('teams.templateRoles')}</div>
                  <div className="mt-2 flex flex-wrap gap-1.5">
                    {selectedTemplate.roles.map((role) => (
                      <span key={role.name} title={templateRoleDescription(role, t)} className="rounded-md bg-white px-2 py-0.5 font-mono text-[11px] text-neutral-600 ring-1 ring-neutral-200 dark:bg-zinc-900 dark:text-zinc-400 dark:ring-zinc-700">
                        {templateRoleName(role.name, t)}
                      </span>
                    ))}
                  </div>
                </div>
              )}

              {err && <p className="text-sm text-red-600 dark:text-red-400">{err}</p>}
              <div className="flex justify-end gap-2 pt-1">
                <button
                  type="button"
                  onClick={() => setOpen(false)}
                  disabled={busy}
                  className="rounded-lg border border-neutral-300 px-3 py-1.5 text-sm dark:border-zinc-600"
                >
                  {t('forms.cancel')}
                </button>
                <button
                  type="submit"
                  disabled={busy}
                  className="rounded-lg bg-sky-600 px-3 py-1.5 text-sm font-medium text-white disabled:opacity-50"
                >
                  {busy ? t('forms.saving') : t('forms.submit')}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </>
  )
}

function templateTitle(template: TeamTemplate, t: ReturnType<typeof useTranslation>['t']) {
  return t(`teamTemplates.${template.id}.name`, { defaultValue: template.name })
}

function templateDescription(template: TeamTemplate, t: ReturnType<typeof useTranslation>['t']) {
  return t(`teamTemplates.${template.id}.description`, { defaultValue: template.description })
}

function templateRoleName(roleName: string, t: ReturnType<typeof useTranslation>['t']) {
  return t(`teamTemplates.roles.${roleName}.name`, { defaultValue: roleName })
}

function templateRoleDescription(role: { name: string; description: string }, t: ReturnType<typeof useTranslation>['t']) {
  return t(`teamTemplates.roles.${role.name}.description`, { defaultValue: role.description })
}
