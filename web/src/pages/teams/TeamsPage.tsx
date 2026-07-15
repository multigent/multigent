import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Users, Puzzle, ArrowRight, User, Database, Layers3, Plus } from 'lucide-react'
import { PlaceholderCard } from '../../components/ui/PlaceholderCard'
import { useApiJson } from '../../lib/use-api'
import { apiPost } from '../../lib/api'

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
  const { t } = useTranslation()
  const [reloadKey, setReloadKey] = useState(0)
  const [applying, setApplying] = useState<string | null>(null)
  const state = useApiJson<TeamRow[]>('/api/v1/teams', reloadKey)
  const templateState = useApiJson<TeamTemplate[]>('/api/v1/team-templates', reloadKey)

  async function applyTemplate(template: TeamTemplate) {
    setApplying(template.id)
    try {
      await apiPost(`/api/v1/team-templates/${encodeURIComponent(template.id)}/apply`, {
        teamName: template.teamName,
      })
      setReloadKey((k) => k + 1)
    } catch (e) {
      alert(String(e))
    } finally {
      setApplying(null)
    }
  }

  return (
    <div className="animate-fade-in px-8 py-6">
      <div className="pb-5">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <h1 className="text-xl font-semibold text-neutral-900 dark:text-zinc-100">{t('nav.teams')}</h1>
            <p className="mt-0.5 text-sm text-neutral-500 dark:text-zinc-500">{t('teams.subtitle')}</p>
          </div>
        </div>
      </div>

      {templateState.status === 'ok' && templateState.data.length > 0 && (
        <section className="mb-6">
          <div className="mb-2 flex items-center gap-2">
            <Layers3 className="size-4 text-neutral-400 dark:text-zinc-500" strokeWidth={1.8} />
            <h2 className="text-sm font-semibold text-neutral-700 dark:text-zinc-300">{t('teams.templatesTitle')}</h2>
          </div>
          <div className="grid gap-3 lg:grid-cols-2">
            {templateState.data.map((template) => {
              const exists = state.status === 'ok' && state.data.some((team) => team.path === template.teamName)
              return (
                <div key={template.id} className="rounded-lg border border-neutral-200/80 bg-white p-4 dark:border-zinc-700/60 dark:bg-zinc-900/40">
                  <div className="flex gap-3">
                    <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-sky-100 dark:bg-sky-900/30">
                      <Layers3 className="size-4.5 text-sky-600 dark:text-sky-400" strokeWidth={1.8} />
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="flex min-w-0 items-start justify-between gap-3">
                        <div className="min-w-0">
                          <h3 className="text-sm font-semibold text-neutral-900 dark:text-zinc-100">{template.name}</h3>
                          <p className="mt-0.5 text-xs leading-relaxed text-neutral-500 dark:text-zinc-500">{template.description}</p>
                        </div>
                        <button
                          type="button"
                          onClick={() => void applyTemplate(template)}
                          disabled={exists || applying === template.id}
                          className="inline-flex shrink-0 items-center gap-1.5 rounded-md bg-sky-600 px-2.5 py-1.5 text-xs font-medium text-white transition-colors hover:bg-sky-700 disabled:bg-neutral-200 disabled:text-neutral-500 dark:disabled:bg-zinc-800 dark:disabled:text-zinc-500"
                        >
                          <Plus className="size-3" strokeWidth={2} />
                          {exists ? t('teams.templateApplied') : applying === template.id ? t('forms.saving') : t('teams.applyTemplate')}
                        </button>
                      </div>
                      <div className="mt-3 flex flex-wrap gap-1.5">
                        {template.roles.map((role) => (
                          <span key={role.name} title={role.description} className="rounded-md bg-neutral-100 px-2 py-0.5 font-mono text-[11px] text-neutral-600 dark:bg-zinc-800 dark:text-zinc-400">
                            {role.name}
                          </span>
                        ))}
                      </div>
                    </div>
                  </div>
                </div>
              )
            })}
          </div>
        </section>
      )}

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
