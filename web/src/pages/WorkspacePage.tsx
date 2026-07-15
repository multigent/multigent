import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Building2, CalendarDays, Hash, Lock, Plus, RefreshCw, Save, UserRound } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { PlaceholderCard } from '../components/ui/PlaceholderCard'
import { apiPut } from '../lib/api'
import { useApiJson } from '../lib/use-api'

type WorkspaceSummary = {
  id: string
  name: string
  description?: string
  createdBy: string
  createdAt: string
  updatedAt?: string
  teams: number
  projects: number
  agents: number
  tasks: number
}

function formatDate(value?: string) {
  if (!value) return '-'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return value
  return d.toLocaleString()
}

export default function WorkspacePage() {
  const { t } = useTranslation()
  const [reloadKey, setReloadKey] = useState(0)
  const state = useApiJson<WorkspaceSummary>('/api/v1/workspace', reloadKey)
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (state.status === 'ok') {
      setName(state.data.name)
      setDescription(state.data.description ?? '')
    }
  }, [state])

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setSaving(true)
    try {
      await apiPut('/api/v1/workspace', { name, description })
      setReloadKey(v => v + 1)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="animate-fade-in px-8 py-6">
      <div className="pb-5">
        <h1 className="text-xl font-semibold text-neutral-900 dark:text-zinc-100">{t('workspace.title')}</h1>
        <p className="mt-0.5 text-sm text-neutral-500 dark:text-zinc-500">{t('workspace.subtitle')}</p>
      </div>

      {state.status === 'loading' && (
        <div className="flex items-center justify-center gap-2 py-16">
          <div className="size-5 animate-spin rounded-full border-2 border-neutral-300 border-t-sky-600 dark:border-zinc-600 dark:border-t-sky-400" />
          <span className="text-sm text-neutral-500">{t('api.loading')}</span>
        </div>
      )}

      {state.status === 'error' && (
        <PlaceholderCard title={t('api.loadError')}>
          <p className="text-[13px]">{state.error.message}</p>
        </PlaceholderCard>
      )}

      {state.status === 'ok' && (
        <div className="grid gap-5 lg:grid-cols-[minmax(0,1.2fr)_minmax(320px,0.8fr)]">
          <section className="rounded-xl border border-neutral-200/80 bg-white p-5 dark:border-zinc-700/60 dark:bg-zinc-900/30">
            <div className="flex items-start gap-3">
              <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-sky-100 dark:bg-sky-900/30">
                <Building2 className="size-5 text-sky-600 dark:text-sky-400" strokeWidth={1.8} />
              </div>
              <div className="min-w-0">
                <h2 className="truncate text-base font-semibold text-neutral-900 dark:text-zinc-100">{state.data.name}</h2>
                <p className="mt-1 text-sm leading-relaxed text-neutral-500 dark:text-zinc-500">
                  {state.data.description || t('workspace.noDescription')}
                </p>
              </div>
            </div>

            <div className="mt-5 grid gap-3 sm:grid-cols-2">
              <Info icon={UserRound} label={t('workspace.createdBy')} value={state.data.createdBy || '-'} />
              <Info icon={CalendarDays} label={t('workspace.createdAt')} value={formatDate(state.data.createdAt)} />
              <Info icon={RefreshCw} label={t('workspace.updatedAt')} value={formatDate(state.data.updatedAt)} />
              <Info icon={Hash} label="Workspace ID" value={state.data.id} mono />
            </div>

            <div className="mt-5 grid grid-cols-2 gap-3 sm:grid-cols-4">
              <Metric label={t('nav.teams')} value={state.data.teams} />
              <Metric label={t('nav.projects')} value={state.data.projects} />
              <Metric label="Agents" value={state.data.agents} />
              <Metric label="Tasks" value={state.data.tasks} />
            </div>
          </section>

          <section className="rounded-xl border border-neutral-200/80 bg-white p-5 dark:border-zinc-700/60 dark:bg-zinc-900/30">
            <h2 className="text-sm font-semibold text-neutral-900 dark:text-zinc-100">{t('workspace.editTitle')}</h2>
            <form onSubmit={onSubmit} className="mt-4 space-y-4">
              <label className="block">
                <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">{t('workspace.name')}</span>
                <input
                  value={name}
                  onChange={e => setName(e.target.value)}
                  className="mt-1 w-full rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-100"
                />
              </label>
              <label className="block">
                <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">{t('workspace.description')}</span>
                <textarea
                  value={description}
                  onChange={e => setDescription(e.target.value)}
                  rows={4}
                  className="mt-1 w-full resize-none rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-100"
                />
              </label>
              <button
                type="submit"
                disabled={saving || name.trim() === ''}
                className="inline-flex items-center gap-2 rounded-lg bg-sky-600 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-sky-700 disabled:cursor-not-allowed disabled:opacity-50"
              >
                <Save className="size-4" strokeWidth={1.8} />
                {saving ? t('workspace.saving') : t('common.save')}
              </button>
            </form>

            <div className="mt-6 border-t border-neutral-200/70 pt-5 dark:border-zinc-700/60">
              <h3 className="text-sm font-semibold text-neutral-900 dark:text-zinc-100">{t('workspace.switchTitle')}</h3>
              <p className="mt-1.5 text-sm leading-relaxed text-neutral-500 dark:text-zinc-500">{t('workspace.switchHint')}</p>
              <div className="mt-3 flex flex-wrap gap-2">
                <button type="button" disabled className="inline-flex cursor-not-allowed items-center gap-2 rounded-lg border border-neutral-200 px-3 py-2 text-sm font-medium text-neutral-400 dark:border-zinc-700 dark:text-zinc-500">
                  <Plus className="size-4" strokeWidth={1.8} />
                  {t('workspace.createWorkspace')}
                </button>
                <button type="button" disabled className="inline-flex cursor-not-allowed items-center gap-2 rounded-lg border border-neutral-200 px-3 py-2 text-sm font-medium text-neutral-400 dark:border-zinc-700 dark:text-zinc-500">
                  <Lock className="size-4" strokeWidth={1.8} />
                  {t('workspace.switchWorkspace')}
                </button>
              </div>
            </div>
          </section>
        </div>
      )}
    </div>
  )
}

function Info({ icon: Icon, label, value, mono = false }: { icon: LucideIcon; label: string; value: string; mono?: boolean }) {
  return (
    <div className="rounded-lg bg-neutral-50 p-3 dark:bg-zinc-800/50">
      <div className="flex items-center gap-2 text-xs font-medium text-neutral-400 dark:text-zinc-500">
        <Icon className="size-3.5" strokeWidth={1.8} />
        {label}
      </div>
      <p className={mono ? 'mt-1 truncate font-mono text-xs text-neutral-700 dark:text-zinc-300' : 'mt-1 truncate text-sm font-medium text-neutral-800 dark:text-zinc-200'}>{value}</p>
    </div>
  )
}

function Metric({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-lg bg-neutral-50 p-3 text-center dark:bg-zinc-800/50">
      <p className="text-lg font-semibold text-neutral-900 dark:text-zinc-100">{value}</p>
      <p className="text-xs text-neutral-400 dark:text-zinc-500">{label}</p>
    </div>
  )
}
