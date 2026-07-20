import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { FolderKanban, ArrowRight, X } from 'lucide-react'
import { PlaceholderCard } from '../../components/ui/PlaceholderCard'
import { useApiJson } from '../../lib/use-api'
import { apiPost } from '../../lib/api'
import { primaryOutlineButton } from '../../lib/button-styles'

type ProjectRow = {
  name: string
  description?: string
}

export default function ProjectsListPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [reloadKey, setReloadKey] = useState(0)
  const [createOpen, setCreateOpen] = useState(false)
  const state = useApiJson<ProjectRow[]>('/api/v1/projects', reloadKey)

  return (
    <div className="animate-fade-in px-8 py-6">
      <div className="flex items-start justify-between gap-4 pb-5">
        <div>
          <h1 className="text-xl font-semibold text-neutral-900 dark:text-zinc-100">{t('nav.projects')}</h1>
          <p className="mt-0.5 text-sm text-neutral-500 dark:text-zinc-500">{t('projects.listSubtitle')}</p>
        </div>
        <button
          type="button"
          data-tour-project-create
          onClick={() => setCreateOpen(true)}
          className={primaryOutlineButton}
        >
          {t('projects.create')}
        </button>
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
            <FolderKanban className="size-7 text-neutral-400 dark:text-zinc-500" strokeWidth={1.5} />
          </div>
          <p className="text-lg font-medium text-neutral-600 dark:text-zinc-400">{t('projects.emptyTitle')}</p>
          <p className="mt-1.5 text-sm text-neutral-400 dark:text-zinc-500">{t('api.noProjects')}</p>
        </div>
      )}
      {state.status === 'ok' && state.data.length > 0 && (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {state.data.map((p) => (
            <Link
              key={p.name}
              to={`/projects/${encodeURIComponent(p.name)}/tasks`}
              data-tour-project-card={p.name}
              className="group flex flex-col justify-between rounded-xl border border-neutral-200/80 bg-white p-5 transition-all duration-150 hover:border-sky-300/60 hover:shadow-md dark:border-zinc-700/60 dark:bg-zinc-900/30 dark:hover:border-sky-800/40"
            >
              <div>
                <div className="flex items-center gap-2.5">
                  <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-sky-100 dark:bg-sky-900/30">
                    <FolderKanban className="size-4.5 text-sky-600 dark:text-sky-400" strokeWidth={1.8} />
                  </div>
                  <h3 className="text-sm font-semibold text-neutral-900 dark:text-zinc-100">{p.name}</h3>
                </div>
                {p.description && (
                  <p className="mt-2.5 text-sm leading-relaxed text-neutral-500 dark:text-zinc-500 line-clamp-2">{p.description}</p>
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
      {createOpen && (
        <CreateProjectDialog
          onClose={() => setCreateOpen(false)}
          onCreated={(name) => {
            setCreateOpen(false)
            setReloadKey((v) => v + 1)
            navigate(`/projects/${encodeURIComponent(name)}/tasks`)
          }}
        />
      )}
    </div>
  )
}

function CreateProjectDialog({ onClose, onCreated }: { onClose: () => void; onCreated: (name: string) => void }) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [saving, setSaving] = useState(false)

  async function create() {
    const projectName = name.trim()
    if (!projectName) return
    setSaving(true)
    try {
      await apiPost('/api/v1/projects', { name: projectName, description })
      onCreated(projectName)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center px-4 pt-[12vh]">
      <div className="absolute inset-0 bg-black/30 backdrop-blur-[2px] dark:bg-black/50" onClick={onClose} />
      <div className="relative w-full max-w-lg overflow-hidden rounded-xl border border-neutral-200/80 bg-white shadow-2xl dark:border-zinc-700/80 dark:bg-zinc-900">
        <div className="flex items-center justify-between border-b border-neutral-200/80 px-5 py-3 dark:border-zinc-700/60">
          <div>
            <h2 className="text-sm font-semibold text-neutral-900 dark:text-zinc-100">{t('projects.createTitle')}</h2>
            <p className="mt-0.5 text-xs text-neutral-500 dark:text-zinc-500">{t('projects.createHint')}</p>
          </div>
          <button type="button" onClick={onClose} className="rounded-md p-1 text-neutral-400 hover:bg-neutral-100 hover:text-neutral-700 dark:hover:bg-zinc-800 dark:hover:text-zinc-300">
            <X className="size-4" strokeWidth={2} />
          </button>
        </div>
        <div className="space-y-4 p-5">
          <label className="block">
            <span className="text-xs font-medium text-neutral-600 dark:text-zinc-400">{t('projects.name')}</span>
            <input value={name} onChange={(e) => setName(e.target.value)} placeholder="sample-agent" className="mt-1 w-full rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm outline-none focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-100" />
          </label>
          <label className="block">
            <span className="text-xs font-medium text-neutral-600 dark:text-zinc-400">{t('projects.description')}</span>
            <textarea value={description} onChange={(e) => setDescription(e.target.value)} rows={3} className="mt-1 w-full resize-y rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm outline-none focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-100" />
          </label>
        </div>
        <div className="flex justify-end gap-2 border-t border-neutral-200/80 px-5 py-3 dark:border-zinc-700/60">
          <button type="button" onClick={onClose} className="rounded-lg px-3 py-2 text-sm font-medium text-neutral-600 hover:bg-neutral-100 dark:text-zinc-400 dark:hover:bg-zinc-800">{t('common.cancel')}</button>
          <button type="button" onClick={() => void create()} disabled={saving || !name.trim()} className="rounded-lg bg-sky-600 px-3 py-2 text-sm font-medium text-white hover:bg-sky-700 disabled:opacity-50">
            {saving ? t('common.creating') : t('projects.create')}
          </button>
        </div>
      </div>
    </div>
  )
}
