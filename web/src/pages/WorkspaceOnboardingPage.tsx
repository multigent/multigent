import { useState, type FormEvent } from 'react'
import { Building2, Loader2, Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { apiPost } from '../lib/api'

export default function WorkspaceOnboardingPage({ onCreated }: { onCreated: () => void }) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    const trimmed = name.trim()
    if (!trimmed) return
    setBusy(true)
    try {
      await apiPost('/api/v1/workspaces', {
        name: trimmed,
        description: description.trim(),
        switch: true,
      })
      window.dispatchEvent(new Event('workspace-changed'))
      onCreated()
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-neutral-50 px-4 text-neutral-900 dark:bg-zinc-950 dark:text-zinc-100">
      <div className="w-full max-w-md rounded-xl border border-neutral-200 bg-white p-6 shadow-sm dark:border-zinc-800 dark:bg-zinc-900">
        <div className="flex items-center gap-3">
          <div className="flex size-10 items-center justify-center rounded-lg bg-sky-100 dark:bg-sky-900/30">
            <Building2 className="size-5 text-sky-700 dark:text-sky-300" strokeWidth={1.8} />
          </div>
          <div>
            <h1 className="text-lg font-semibold">{t('workspaceOnboarding.title')}</h1>
            <p className="mt-0.5 text-sm text-neutral-500 dark:text-zinc-400">{t('workspaceOnboarding.subtitle')}</p>
          </div>
        </div>

        <form onSubmit={submit} className="mt-6 space-y-4">
          <label className="block">
            <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">{t('workspaceOnboarding.name')}</span>
            <input
              value={name}
              onChange={e => setName(e.target.value)}
              autoFocus
              className="mt-1 w-full rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm outline-none focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-950"
              placeholder="Acme AI Team"
            />
          </label>
          <label className="block">
            <span className="text-xs font-medium text-neutral-500 dark:text-zinc-400">{t('workspaceOnboarding.description')}</span>
            <textarea
              value={description}
              onChange={e => setDescription(e.target.value)}
              rows={3}
              className="mt-1 w-full resize-none rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm outline-none focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-950"
              placeholder={t('workspaceOnboarding.descriptionPlaceholder')}
            />
          </label>
          <button
            type="submit"
            disabled={busy || name.trim() === ''}
            className="inline-flex w-full items-center justify-center gap-2 rounded-lg bg-sky-600 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-sky-700 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {busy ? <Loader2 className="size-4 animate-spin" strokeWidth={1.8} /> : <Plus className="size-4" strokeWidth={1.8} />}
            {busy ? t('workspaceOnboarding.creating') : t('workspaceOnboarding.create')}
          </button>
        </form>
      </div>
    </div>
  )
}
