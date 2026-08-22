import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Eye, RefreshCw, ShieldCheck, X } from 'lucide-react'
import { apiFetch } from '../lib/api'
import { cn } from '../lib/cn'
import { useFormatDateTime } from '../lib/format-datetime'
import { Pagination } from '../components/ui/Pagination'

type AuditEvent = {
  id: string
  workspaceId: string
  actorType: string
  actorId: string
  action: string
  resourceType: string
  resourceId: string
  summary?: string
  before?: unknown
  after?: unknown
  ip?: string
  userAgent?: string
  createdAt: string
}

type AuditFacets = {
  actorIds?: string[]
  actions?: string[]
  resourceTypes?: string[]
  resourceIds?: string[]
}

const filterSelectCls =
  'h-9 rounded-lg border border-neutral-200 bg-white px-3 text-sm text-neutral-600 outline-none transition-colors hover:border-neutral-300 focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300 dark:[color-scheme:dark]'
const pageSize = 20

export default function AuditPage() {
  const { t } = useTranslation()
  const [events, setEvents] = useState<AuditEvent[]>([])
  const [facets, setFacets] = useState<AuditFacets>({})
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(true)
  const [selected, setSelected] = useState<AuditEvent | null>(null)
  const [filters, setFilters] = useState({
    actorId: '',
    action: '',
    resourceType: '',
    resourceId: '',
  })

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const params = new URLSearchParams()
      params.set('limit', String(pageSize))
      params.set('offset', String((page - 1) * pageSize))
      for (const [key, value] of Object.entries(filters)) {
        if (value.trim()) params.set(key, value.trim())
      }
      const suffix = params.toString() ? `?${params.toString()}` : ''
      const data = await apiFetch<{ events: AuditEvent[]; total?: number; facets?: AuditFacets }>(`/api/v1/audit/events${suffix}`)
      setEvents(data.events ?? [])
      setTotal(data.total ?? data.events?.length ?? 0)
      setFacets(data.facets ?? {})
    } finally {
      setLoading(false)
    }
  }, [filters, page])

  useEffect(() => { void load() }, [load])
  const totalPages = Math.max(1, Math.ceil(total / pageSize))
  const updateFilter = (key: keyof typeof filters, value: string) => {
    setPage(1)
    setFilters(v => ({ ...v, [key]: value }))
  }
  const hasFilters = Boolean(filters.actorId || filters.action || filters.resourceType || filters.resourceId)

  return (
    <div className="animate-fade-in px-8 py-6">
      <div className="flex items-center justify-between pb-5">
        <div>
          <h1 className="text-xl font-semibold text-neutral-900 dark:text-zinc-100">{t('audit.title')}</h1>
          <p className="mt-0.5 text-sm text-neutral-500 dark:text-zinc-500">{t('audit.subtitle')}</p>
        </div>
        <button type="button" onClick={() => void load()} disabled={loading} className="inline-flex items-center gap-2 rounded-lg border border-neutral-200 px-3 py-2 text-sm font-medium text-neutral-600 hover:bg-neutral-50 disabled:opacity-50 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800">
          <RefreshCw className={cn('size-4', loading && 'animate-spin')} strokeWidth={1.8} />
          {t('common.refresh')}
        </button>
      </div>

      <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <FilterSelect label={t('audit.allActors')} value={filters.actorId} onChange={actorId => updateFilter('actorId', actorId)} options={facets.actorIds ?? []} formatOption={formatActorOption} />
          <FilterSelect label={t('audit.allActions')} value={filters.action} onChange={action => updateFilter('action', action)} options={facets.actions ?? []} />
          <FilterSelect label={t('audit.allResourceTypes')} value={filters.resourceType} onChange={resourceType => updateFilter('resourceType', resourceType)} options={facets.resourceTypes ?? []} />
          <FilterSelect label={t('audit.allResourceIds')} value={filters.resourceId} onChange={resourceId => updateFilter('resourceId', resourceId)} options={facets.resourceIds ?? []} />
          {hasFilters && (
            <button
              type="button"
              onClick={() => {
                setPage(1)
                setFilters({ actorId: '', action: '', resourceType: '', resourceId: '' })
              }}
              className="rounded-lg px-2.5 py-2 text-sm font-medium text-neutral-500 transition-colors hover:bg-neutral-100 hover:text-neutral-800 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-200"
            >
              {t('common.clear')}
            </button>
          )}
          <span className="text-xs text-neutral-400 dark:text-zinc-500">
            {t('audit.filteredCount', { count: events.length, total })}
          </span>
        </div>
      </div>

      {loading ? (
        <div className="flex items-center justify-center gap-2 py-16 text-sm text-neutral-500">
          <div className="size-5 animate-spin rounded-full border-2 border-neutral-300 border-t-sky-600 dark:border-zinc-600 dark:border-t-sky-400" />
          {t('api.loading')}
        </div>
      ) : events.length === 0 ? (
        <div className="rounded-xl border border-dashed border-neutral-300 bg-white p-12 text-center dark:border-zinc-700 dark:bg-zinc-900/40">
          <ShieldCheck className="mx-auto mb-3 size-10 text-neutral-300 dark:text-zinc-600" strokeWidth={1.5} />
          <p className="text-sm font-medium text-neutral-600 dark:text-zinc-300">{t('audit.emptyFiltered')}</p>
          <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">{t('audit.emptyHint')}</p>
        </div>
      ) : (
        <div className="overflow-hidden rounded-xl border border-neutral-200 bg-white dark:border-zinc-800 dark:bg-zinc-900/40">
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-neutral-100 text-sm dark:divide-zinc-800">
              <thead className="bg-neutral-50 text-left text-xs font-medium text-neutral-500 dark:bg-zinc-900 dark:text-zinc-400">
                <tr>
                  <th className="px-4 py-3">{t('audit.time')}</th>
                  <th className="px-4 py-3">{t('audit.actor')}</th>
                  <th className="px-4 py-3">{t('audit.action')}</th>
                  <th className="px-4 py-3">{t('audit.resource')}</th>
                  <th className="px-4 py-3">{t('audit.summary')}</th>
                  <th className="px-4 py-3">{t('audit.ip')}</th>
                  <th className="sticky right-0 w-28 min-w-28 bg-neutral-50 px-4 py-3 text-right shadow-[-8px_0_12px_-12px_rgba(0,0,0,0.3)] dark:bg-zinc-900">{t('audit.details')}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-neutral-100 dark:divide-zinc-800">
                {events.map(event => (
                  <AuditEventRow key={event.id} event={event} onOpen={() => setSelected(event)} />
                ))}
              </tbody>
            </table>
          </div>
          <div className="border-t border-neutral-100 px-4 dark:border-zinc-800">
            <Pagination page={page} totalPages={totalPages} onPageChange={setPage} />
          </div>
        </div>
      )}
      {selected && <AuditDetailModal event={selected} onClose={() => setSelected(null)} />}
    </div>
  )
}

function FilterSelect({ label, value, onChange, options, formatOption }: { label: string; value: string; onChange: (value: string) => void; options: string[]; formatOption?: (value: string) => string }) {
  return (
    <select value={value} onChange={e => onChange(e.target.value)} className={cn(filterSelectCls, 'max-w-[260px]')}>
      <option value="">{label}</option>
      {options.map(option => (
        <option key={option} value={option}>{formatOption ? formatOption(option) : option}</option>
      ))}
    </select>
  )
}

function actorDisplay(actorType: string, actorId: string) {
  const id = actorId || '-'
  if (actorType === 'agent') {
    const parts = id.split('/').filter(Boolean)
    const name = parts.at(-1) || id
    return {
      primary: name,
      secondary: parts.length > 1 ? id : actorType,
      full: `${actorType}:${id}`,
    }
  }
  return {
    primary: id,
    secondary: actorType || '-',
    full: `${actorType || '-'}:${id}`,
  }
}

function formatActorOption(value: string) {
  if (!value.includes('/')) return value
  const parts = value.split('/').filter(Boolean)
  const name = parts.at(-1) || value
  return `${name} · ${value}`
}

function AuditEventRow({ event, onOpen }: { event: AuditEvent; onOpen: () => void }) {
  const fmtDateTime = useFormatDateTime()
  const { t } = useTranslation()
  const created = fmtDateTime(event.createdAt)
  const actor = actorDisplay(event.actorType, event.actorId)
  return (
    <tr className="text-neutral-700 dark:text-zinc-300">
      <td className="whitespace-nowrap px-4 py-3 text-xs text-neutral-500 dark:text-zinc-500">{created}</td>
      <td className="px-4 py-3">
        <div className="min-w-[160px] max-w-[220px]" title={actor.full}>
          <div className="truncate font-medium text-neutral-800 dark:text-zinc-200">{actor.primary}</div>
          <div className="mt-0.5 truncate text-xs text-neutral-400 dark:text-zinc-500">{actor.secondary}</div>
        </div>
      </td>
      <td className="px-4 py-3">
        <span className="rounded-full bg-sky-50 px-2 py-0.5 text-xs font-medium text-sky-700 dark:bg-sky-950/40 dark:text-sky-300">{event.action}</span>
      </td>
      <td className="px-4 py-3">
        <div className="max-w-[220px] truncate">
          <span className="text-xs text-neutral-400 dark:text-zinc-500">{event.resourceType}</span>
          {event.resourceId ? <span className="ml-1 font-mono text-xs">{event.resourceId}</span> : null}
        </div>
      </td>
      <td className="px-4 py-3">
        <div className="max-w-sm truncate">{event.summary || event.action}</div>
      </td>
      <td className="whitespace-nowrap px-4 py-3 text-xs text-neutral-500 dark:text-zinc-500">{event.ip || '-'}</td>
      <td className="sticky right-0 w-28 min-w-28 bg-white px-4 py-3 text-right shadow-[-8px_0_12px_-12px_rgba(0,0,0,0.25)] dark:bg-zinc-900">
        <button type="button" onClick={onOpen} className="inline-flex items-center gap-1 whitespace-nowrap rounded-md px-2 py-1 text-xs font-medium text-neutral-500 transition-colors hover:bg-neutral-100 hover:text-neutral-900 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100">
          <Eye className="size-3.5" strokeWidth={1.8} />
          {t('audit.view')}
        </button>
      </td>
    </tr>
  )
}

function AuditDetailModal({ event, onClose }: { event: AuditEvent; onClose: () => void }) {
  const { t } = useTranslation()
  const fmtDateTime = useFormatDateTime()
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 px-4 py-6">
      <div className="max-h-[88vh] w-full max-w-4xl overflow-hidden rounded-xl border border-neutral-200 bg-white shadow-2xl dark:border-zinc-800 dark:bg-zinc-950">
        <div className="flex items-start justify-between gap-4 border-b border-neutral-100 px-5 py-4 dark:border-zinc-800">
          <div className="min-w-0">
            <h2 className="text-base font-semibold text-neutral-900 dark:text-zinc-100">{t('audit.detailTitle')}</h2>
            <p className="mt-1 truncate text-xs text-neutral-500 dark:text-zinc-500">
              {event.action} · {event.resourceType}{event.resourceId ? `:${event.resourceId}` : ''} · {fmtDateTime(event.createdAt)}
            </p>
          </div>
          <button type="button" onClick={onClose} className="rounded-lg p-1.5 text-neutral-400 transition-colors hover:bg-neutral-100 hover:text-neutral-700 dark:hover:bg-zinc-800 dark:hover:text-zinc-200">
            <X className="size-4" strokeWidth={1.8} />
          </button>
        </div>
        <div className="max-h-[72vh] overflow-y-auto px-5 py-4">
          <div className="grid gap-3 pb-4 text-sm md:grid-cols-2">
            <Meta label={t('audit.eventId')} value={event.id} mono />
            <Meta label={t('audit.actor')} value={`${event.actorType}:${event.actorId}`} />
            <Meta label={t('audit.summary')} value={event.summary || '-'} />
            <Meta label={t('audit.userAgent')} value={event.userAgent || '-'} />
          </div>
          <div className="grid gap-4 lg:grid-cols-2">
            <JSONBlock label={t('audit.before')} value={event.before ?? null} />
            <JSONBlock label={t('audit.after')} value={event.after ?? null} />
          </div>
        </div>
      </div>
    </div>
  )
}

function Meta({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="min-w-0 rounded-lg bg-neutral-50 px-3 py-2 dark:bg-zinc-900/70">
      <p className="text-xs text-neutral-400 dark:text-zinc-500">{label}</p>
      <p className={cn('mt-1 truncate text-neutral-700 dark:text-zinc-300', mono && 'font-mono text-xs')}>{value}</p>
    </div>
  )
}

function JSONBlock({ label, value }: { label: string; value: unknown }) {
  return (
    <div className="rounded-lg bg-neutral-50 p-3 dark:bg-zinc-950/60">
      <p className="mb-2 text-xs font-medium text-neutral-500 dark:text-zinc-400">{label}</p>
      <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-words text-xs leading-relaxed text-neutral-600 dark:text-zinc-300">
        {JSON.stringify(value, null, 2)}
      </pre>
    </div>
  )
}
