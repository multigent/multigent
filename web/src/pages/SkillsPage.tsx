import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router-dom'
import Markdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { BookOpen, FileCode, FolderTree, Puzzle, Save, Upload, X } from 'lucide-react'
import { cn } from '../lib/cn'
import { useApiJson } from '../lib/use-api'
import { apiPost, apiPut } from '../lib/api'
import { primaryOutlineButton } from '../lib/button-styles'

type Provenance = { playbookId: string; playbookName: string; templateVersion?: string; customized?: boolean }
type SkillRow = {
  name: string
  description?: string
  provenance?: Provenance
  source?: string
  sourceType?: string
  sourceRef?: string
  version?: string
  managed?: boolean
  dirty?: boolean
}
type SkillRegistry = {
  source?: string
  sourceType?: string
  sourceRef?: string
  version?: string
  managed?: boolean
  dirty?: boolean
  installedAt?: string
  updatedAt?: string
}
type SkillDetail = { name: string; description?: string; prompt: string; provenance?: Provenance; registry?: SkillRegistry; packageDir?: string }
type SkillFileTree = { name: string; files: SkillFile[] }
type SkillFile = { path: string; size: number; mode?: string; content?: string; encoding?: string }

function SkillItem({ skill, defaultOpen }: { skill: SkillRow; defaultOpen?: boolean }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(!!defaultOpen)
  const elRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (defaultOpen && elRef.current) {
      elRef.current.scrollIntoView({ behavior: 'smooth', block: 'start' })
    }
  }, [defaultOpen])
  const detailState = useApiJson<SkillDetail>(open ? `/api/v1/skills/${encodeURIComponent(skill.name)}` : null, 0)
  const filesState = useApiJson<SkillFileTree>(open ? `/api/v1/skills/${encodeURIComponent(skill.name)}/files` : null, 0)

  const [value, setValue] = useState<string | null>(null)
  const [dirty, setDirty] = useState(false)
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)
  const [preview, setPreview] = useState(false)

  const content = value ?? (detailState.status === 'ok' ? detailState.data.prompt : '')

  const save = useCallback(async () => {
    setSaving(true)
    setSaved(false)
    try {
      await apiPut(`/api/v1/skills/${encodeURIComponent(skill.name)}`, { content })
      setDirty(false)
      setSaved(true)
      setTimeout(() => setSaved(false), 2000)
    } catch (e) {
      alert(String(e))
    } finally {
      setSaving(false)
    }
  }, [skill.name, content])

  return (
    <div
      ref={elRef}
      className="overflow-hidden rounded-xl border border-neutral-200/80 bg-white shadow-sm transition-colors dark:border-zinc-700/60 dark:bg-zinc-900/40"
    >
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="flex min-h-24 w-full items-start px-5 py-4 text-left transition-colors hover:bg-neutral-50/80 dark:hover:bg-zinc-800/30"
      >
        <div className="min-w-0 flex-1">
          <p className="font-mono text-sm font-medium text-neutral-900 dark:text-zinc-100">{skill.name}</p>
          {skill.description && (
            <p className="mt-1 line-clamp-2 text-xs leading-relaxed text-neutral-400 dark:text-zinc-500">{skill.description}</p>
          )}
          {skill.provenance && <ProvenanceBadge provenance={skill.provenance} className="mt-2" />}
          <SkillRegistryLine skill={skill} className="mt-2" />
        </div>
      </button>
      {open && (
        <div className="fixed inset-0 z-50 flex items-start justify-center px-4 pt-[8vh]">
          <div className="absolute inset-0 bg-black/30 backdrop-blur-[2px] dark:bg-black/50" onClick={() => setOpen(false)} />
          <div className="relative flex max-h-[82vh] w-full max-w-4xl flex-col overflow-hidden rounded-xl border border-neutral-200/80 bg-white shadow-2xl dark:border-zinc-700/80 dark:bg-zinc-900">
            <div className="flex items-center justify-between border-b border-neutral-200/80 px-5 py-3 dark:border-zinc-700/60">
              <div className="min-w-0">
                <p className="truncate font-mono text-sm font-semibold text-neutral-900 dark:text-zinc-100">{skill.name}</p>
                {skill.description && (
                  <p className="mt-0.5 truncate text-xs text-neutral-400 dark:text-zinc-500">{skill.description}</p>
                )}
                {detailState.status === 'ok' && detailState.data.provenance && <ProvenanceBadge provenance={detailState.data.provenance} className="mt-1.5" />}
                {detailState.status === 'ok' && detailState.data.registry && (
                  <SkillRegistryLine skill={{ name: skill.name, ...detailState.data.registry }} className="mt-1.5" />
                )}
              </div>
              <button
                type="button"
                onClick={() => setOpen(false)}
                className="rounded-md p-1 text-neutral-400 transition-colors hover:bg-neutral-100 hover:text-neutral-700 dark:text-zinc-500 dark:hover:bg-zinc-800 dark:hover:text-zinc-300"
              >
                <X className="size-4" strokeWidth={2} />
              </button>
            </div>
            <div className="flex-1 overflow-auto p-5">
              {detailState.status === 'loading' && (
                <div className="flex items-center justify-center gap-2 py-12">
                  <div className="size-4 animate-spin rounded-full border-2 border-neutral-200 border-t-sky-600" />
                  <span className="text-sm text-neutral-400">{t('api.loading')}</span>
                </div>
              )}
              {detailState.status === 'error' && (
                <p className="py-8 text-center text-sm text-red-500">{detailState.error.message}</p>
              )}
              {detailState.status === 'ok' && (
                <div className="space-y-4">
                <div className="rounded-lg border border-neutral-200/80 bg-white dark:border-zinc-700/60 dark:bg-zinc-900/40">
                  <div className="flex items-center justify-between border-b border-neutral-100 px-4 py-2.5 dark:border-zinc-700/40">
                    <div className="flex items-center gap-2">
                      <BookOpen className="size-4 text-neutral-400 dark:text-zinc-500" strokeWidth={1.8} />
                      <span className="text-sm font-medium text-neutral-700 dark:text-zinc-300">SKILL.md</span>
                      {dirty && <span className="text-[10px] text-amber-500">●</span>}
                      {saved && <span className="text-[10px] text-emerald-500">{t('prompt.saved')}</span>}
                    </div>
                    <div className="flex items-center gap-2">
                      <button
                        type="button"
                        onClick={() => setPreview((p) => !p)}
                        className={cn(
                          'rounded-md px-2 py-1 text-[11px] font-medium transition-colors',
                          preview
                            ? 'bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-400'
                            : 'text-neutral-400 hover:text-neutral-600 dark:text-zinc-500 dark:hover:text-zinc-400',
                        )}
                      >
                        {preview ? t('prompt.edit') : t('prompt.preview')}
                      </button>
                      <button
                        type="button"
                        onClick={save}
                        disabled={saving}
                        className="flex items-center gap-1 rounded-md bg-sky-600 px-2.5 py-1 text-[11px] font-medium text-white transition-colors hover:bg-sky-700 disabled:opacity-50"
                      >
                        <Save className="size-3" strokeWidth={2} />
                        {saving ? t('prompt.saving') : t('prompt.save')}
                      </button>
                    </div>
                  </div>
                  {preview ? (
                    <div className="prose-none max-h-[58vh] overflow-auto p-4 text-sm leading-relaxed text-neutral-800 dark:text-zinc-200">
                      <Markdown remarkPlugins={[remarkGfm]}>{content || '*(empty)*'}</Markdown>
                    </div>
                  ) : (
                    <textarea
                      value={content}
                      onChange={(e) => { setValue(e.target.value); setDirty(true); setSaved(false) }}
                      className="block min-h-[50vh] w-full resize-y bg-transparent p-4 font-mono text-[13px] leading-relaxed text-neutral-800 outline-none placeholder:text-neutral-300 dark:text-zinc-200 dark:placeholder:text-zinc-700"
                      placeholder="Skill prompt..."
                    />
                  )}
                </div>
                <div className="rounded-lg border border-neutral-200/80 bg-white dark:border-zinc-700/60 dark:bg-zinc-900/40">
                  <div className="flex items-center gap-2 border-b border-neutral-100 px-4 py-2.5 dark:border-zinc-700/40">
                    <FolderTree className="size-4 text-neutral-400 dark:text-zinc-500" strokeWidth={1.8} />
                    <span className="text-sm font-medium text-neutral-700 dark:text-zinc-300">{t('skill.files')}</span>
                  </div>
                  <div className="max-h-64 overflow-auto p-3">
                    {filesState.status === 'loading' && <p className="text-sm text-neutral-400">{t('api.loading')}</p>}
                    {filesState.status === 'error' && <p className="text-sm text-red-500">{filesState.error.message}</p>}
                    {filesState.status === 'ok' && (
                      <div className="space-y-1">
                        {(filesState.data.files ?? []).map((file) => (
                          <div key={file.path} className="flex items-center justify-between gap-4 rounded-md px-2 py-1.5 text-xs hover:bg-neutral-50 dark:hover:bg-zinc-800/40">
                            <span className="inline-flex min-w-0 items-center gap-2 font-mono text-neutral-700 dark:text-zinc-300">
                              <FileCode className="size-3.5 shrink-0 text-neutral-400 dark:text-zinc-500" strokeWidth={1.7} />
                              <span className="truncate">{file.path}</span>
                            </span>
                            <span className="shrink-0 text-neutral-400 dark:text-zinc-600">{formatBytes(file.size)}</span>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                </div>
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

function SkillRegistryLine({ skill, className }: { skill: Partial<SkillRow & SkillRegistry>; className?: string }) {
  const { t } = useTranslation()
  const bits: string[] = []
  if (skill.sourceType) bits.push(skill.sourceType)
  if (skill.version) bits.push(`v ${skill.version}`)
  if (skill.managed) bits.push(t('skill.managed'))
  if (skill.dirty) bits.push(t('skill.customized'))
  if (bits.length === 0) return null
  return (
    <div className={cn('flex flex-wrap gap-1.5', className)}>
      {bits.map((bit) => (
        <span key={bit} className="rounded-full bg-neutral-100 px-2 py-0.5 text-[11px] font-medium text-neutral-500 dark:bg-zinc-800 dark:text-zinc-400">
          {bit}
        </span>
      ))}
    </div>
  )
}

function formatBytes(size: number): string {
  if (!Number.isFinite(size) || size < 0) return ''
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
  return `${(size / 1024 / 1024).toFixed(1)} MB`
}

function ProvenanceBadge({ provenance, className }: { provenance: Provenance; className?: string }) {
  const { t } = useTranslation()
  return (
    <span
      className={cn('inline-flex w-fit rounded-full bg-emerald-50 px-2 py-0.5 text-[11px] font-medium text-emerald-700 hover:bg-emerald-100 dark:bg-emerald-950/30 dark:text-emerald-300 dark:hover:bg-emerald-950/50', className)}
    >
      {t('playbooks.fromPlaybook', { name: provenance.playbookName })}
      {provenance.customized ? ` · ${t('playbooks.customized')}` : ''}
    </span>
  )
}

export default function SkillsPage() {
  const { t } = useTranslation()
  const [searchParams] = useSearchParams()
  const openSkill = searchParams.get('open') ?? ''
  const [reloadKey, setReloadKey] = useState(0)
  const [createOpen, setCreateOpen] = useState(false)
  const [installOpen, setInstallOpen] = useState(false)
  const skillsState = useApiJson<SkillRow[]>('/api/v1/skills', reloadKey)
  const skills = skillsState.status === 'ok' ? (skillsState.data ?? []) : []

  return (
    <div className="animate-fade-in px-8 py-6">
      <div className="flex items-start justify-between gap-4 pb-5">
        <div>
          <div className="flex items-center gap-2.5">
            <h1 className="text-xl font-semibold text-neutral-900 dark:text-zinc-100">
              {t('skill.catalogTitle')}
            </h1>
            <span className="rounded-full bg-neutral-100 px-2.5 py-0.5 text-xs font-medium text-neutral-500 dark:bg-zinc-800 dark:text-zinc-500">
              {skills.length}
            </span>
          </div>
          <p className="mt-1 text-sm text-neutral-500 dark:text-zinc-500">{t('skill.catalogHint')}</p>
        </div>
        <div className="flex items-center gap-2">
          <button type="button" onClick={() => setInstallOpen(true)} className={primaryOutlineButton}>
            {t('skill.install')}
          </button>
          <button type="button" onClick={() => setCreateOpen(true)} className={primaryOutlineButton}>
            {t('skill.create')}
          </button>
        </div>
      </div>

      {skillsState.status === 'loading' && (
        <div className="flex items-center gap-2 py-16 justify-center">
          <div className="size-5 animate-spin rounded-full border-2 border-neutral-300 border-t-sky-600 dark:border-zinc-600 dark:border-t-sky-400" />
          <span className="text-sm text-neutral-500">{t('api.loading')}</span>
        </div>
      )}
      {skillsState.status === 'ok' && skills.length === 0 && (
        <div className="flex flex-col items-center justify-center py-24 text-center">
          <div className="mb-4 flex size-16 items-center justify-center rounded-2xl bg-neutral-100 dark:bg-zinc-800/50">
            <Puzzle className="size-7 text-neutral-400 dark:text-zinc-500" strokeWidth={1.5} />
          </div>
          <p className="text-lg font-medium text-neutral-600 dark:text-zinc-400">{t('skill.empty')}</p>
        </div>
      )}
      {skills.length > 0 && (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {skills.map((sk) => (
            <SkillItem key={sk.name} skill={sk} defaultOpen={sk.name === openSkill} />
          ))}
        </div>
      )}
      {createOpen && (
        <CreateSkillDialog
          onClose={() => setCreateOpen(false)}
          onCreated={() => {
            setCreateOpen(false)
            setReloadKey((v) => v + 1)
          }}
        />
      )}
      {installOpen && (
        <InstallSkillDialog
          onClose={() => setInstallOpen(false)}
          onInstalled={() => {
            setInstallOpen(false)
            setReloadKey((v) => v + 1)
          }}
        />
      )}
    </div>
  )
}

function InstallSkillDialog({ onClose, onInstalled }: { onClose: () => void; onInstalled: () => void }) {
  const { t } = useTranslation()
  const [source, setSource] = useState('')
  const [name, setName] = useState('')
  const [saving, setSaving] = useState(false)

  async function install() {
    const src = source.trim()
    if (!src) return
    setSaving(true)
    try {
      await apiPost('/api/v1/skills/install', { source: src, name: name.trim() || undefined, managed: true })
      onInstalled()
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center px-4 pt-[12vh]">
      <div className="absolute inset-0 bg-black/30 backdrop-blur-[2px] dark:bg-black/50" onClick={onClose} />
      <div className="relative w-full max-w-xl overflow-hidden rounded-xl border border-neutral-200/80 bg-white shadow-2xl dark:border-zinc-700/80 dark:bg-zinc-900">
        <div className="flex items-center justify-between border-b border-neutral-200/80 px-5 py-3 dark:border-zinc-700/60">
          <div>
            <h2 className="text-sm font-semibold text-neutral-900 dark:text-zinc-100">{t('skill.installTitle')}</h2>
            <p className="mt-0.5 text-xs text-neutral-500 dark:text-zinc-500">{t('skill.installHint')}</p>
          </div>
          <button type="button" onClick={onClose} className="rounded-md p-1 text-neutral-400 hover:bg-neutral-100 hover:text-neutral-700 dark:hover:bg-zinc-800 dark:hover:text-zinc-300">
            <X className="size-4" strokeWidth={2} />
          </button>
        </div>
        <div className="space-y-4 p-5">
          <label className="block">
            <span className="text-xs font-medium text-neutral-600 dark:text-zinc-400">{t('skill.source')}</span>
            <input
              value={source}
              onChange={(e) => setSource(e.target.value)}
              placeholder="mattpocock/skills 或 https://github.com/owner/repo"
              className="mt-1 w-full rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm outline-none focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-100"
            />
          </label>
          <label className="block">
            <span className="text-xs font-medium text-neutral-600 dark:text-zinc-400">{t('skill.nameOverride')}</span>
            <input value={name} onChange={(e) => setName(e.target.value)} className="mt-1 w-full rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm outline-none focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-100" />
          </label>
          <div className="rounded-lg bg-neutral-50 px-3 py-2 text-xs leading-relaxed text-neutral-500 dark:bg-zinc-950/50 dark:text-zinc-500">
            {t('skill.installRegistryHint')}
          </div>
        </div>
        <div className="flex justify-end gap-2 border-t border-neutral-200/80 px-5 py-3 dark:border-zinc-700/60">
          <button type="button" onClick={onClose} className="rounded-lg px-3 py-2 text-sm font-medium text-neutral-600 hover:bg-neutral-100 dark:text-zinc-400 dark:hover:bg-zinc-800">{t('common.cancel')}</button>
          <button type="button" onClick={() => void install()} disabled={saving || !source.trim()} className="rounded-lg bg-sky-600 px-3 py-2 text-sm font-medium text-white hover:bg-sky-700 disabled:opacity-50">
            {saving ? t('common.installing', { defaultValue: t('api.loading') }) : t('skill.install')}
          </button>
        </div>
      </div>
    </div>
  )
}

function CreateSkillDialog({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [content, setContent] = useState('')
  const [saving, setSaving] = useState(false)

  async function create() {
    const skillName = name.trim()
    if (!skillName) return
    setSaving(true)
    try {
      await apiPost('/api/v1/skills', { name: skillName, description, content })
      onCreated()
    } finally {
      setSaving(false)
    }
  }

  async function onFile(file: File | null) {
    if (!file) return
    const text = await file.text()
    setContent(text)
    if (!name.trim()) {
      setName(file.name.replace(/\.(md|markdown)$/i, '').replace(/[^a-zA-Z0-9_.-]+/g, '-').replace(/^-+|-+$/g, ''))
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center px-4 pt-[8vh]">
      <div className="absolute inset-0 bg-black/30 backdrop-blur-[2px] dark:bg-black/50" onClick={onClose} />
      <div className="relative flex max-h-[84vh] w-full max-w-3xl flex-col overflow-hidden rounded-xl border border-neutral-200/80 bg-white shadow-2xl dark:border-zinc-700/80 dark:bg-zinc-900">
        <div className="flex items-center justify-between border-b border-neutral-200/80 px-5 py-3 dark:border-zinc-700/60">
          <div>
            <h2 className="text-sm font-semibold text-neutral-900 dark:text-zinc-100">{t('skill.createTitle')}</h2>
            <p className="mt-0.5 text-xs text-neutral-500 dark:text-zinc-500">{t('skill.createHint')}</p>
          </div>
          <button type="button" onClick={onClose} className="rounded-md p-1 text-neutral-400 hover:bg-neutral-100 hover:text-neutral-700 dark:hover:bg-zinc-800 dark:hover:text-zinc-300">
            <X className="size-4" strokeWidth={2} />
          </button>
        </div>
        <div className="flex-1 space-y-4 overflow-auto p-5">
          <div className="grid gap-4 sm:grid-cols-2">
            <label className="block">
              <span className="text-xs font-medium text-neutral-600 dark:text-zinc-400">{t('skill.name')}</span>
              <input value={name} onChange={(e) => setName(e.target.value)} placeholder="lark-doc" className="mt-1 w-full rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm outline-none focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-100" />
            </label>
            <label className="block">
              <span className="text-xs font-medium text-neutral-600 dark:text-zinc-400">{t('skill.description')}</span>
              <input value={description} onChange={(e) => setDescription(e.target.value)} className="mt-1 w-full rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm outline-none focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-100" />
            </label>
          </div>
          <label className="flex cursor-pointer items-center justify-between gap-3 rounded-lg border border-dashed border-neutral-300 bg-neutral-50 px-4 py-3 text-sm text-neutral-600 transition-colors hover:border-sky-300 hover:bg-sky-50/60 dark:border-zinc-700 dark:bg-zinc-950/50 dark:text-zinc-400 dark:hover:border-sky-800 dark:hover:bg-sky-950/20">
            <span className="inline-flex items-center gap-2">
              <Upload className="size-4" strokeWidth={1.8} />
              {t('skill.uploadMarkdown')}
            </span>
            <span className="text-xs text-neutral-400 dark:text-zinc-600">SKILL.md</span>
            <input type="file" accept=".md,.markdown,text/markdown,text/plain" className="hidden" onChange={(e) => void onFile(e.target.files?.[0] ?? null)} />
          </label>
          <div className="rounded-lg border border-neutral-200/80 bg-white dark:border-zinc-700/60 dark:bg-zinc-900/40">
            <div className="flex items-center gap-2 border-b border-neutral-100 px-4 py-2.5 dark:border-zinc-700/40">
              <BookOpen className="size-4 text-neutral-400 dark:text-zinc-500" strokeWidth={1.8} />
              <span className="text-sm font-medium text-neutral-700 dark:text-zinc-300">SKILL.md</span>
            </div>
            <textarea
              value={content}
              onChange={(e) => setContent(e.target.value)}
              placeholder={t('skill.contentPlaceholder')}
              className="block min-h-64 w-full resize-y bg-transparent p-4 font-mono text-[13px] leading-relaxed text-neutral-800 outline-none placeholder:text-neutral-300 dark:text-zinc-200 dark:placeholder:text-zinc-700"
            />
          </div>
          <div className="rounded-lg bg-neutral-50 px-3 py-2 text-xs leading-relaxed text-neutral-500 dark:bg-zinc-950/50 dark:text-zinc-500">
            {t('skill.registryHint')}
          </div>
        </div>
        <div className="flex justify-end gap-2 border-t border-neutral-200/80 px-5 py-3 dark:border-zinc-700/60">
          <button type="button" onClick={onClose} className="rounded-lg px-3 py-2 text-sm font-medium text-neutral-600 hover:bg-neutral-100 dark:text-zinc-400 dark:hover:bg-zinc-800">{t('common.cancel')}</button>
          <button type="button" onClick={() => void create()} disabled={saving || !name.trim()} className="rounded-lg bg-sky-600 px-3 py-2 text-sm font-medium text-white hover:bg-sky-700 disabled:opacity-50">
            {saving ? t('common.creating') : t('skill.create')}
          </button>
        </div>
      </div>
    </div>
  )
}
