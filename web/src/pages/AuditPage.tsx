import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Eye, Filter, RefreshCw, ShieldCheck, X } from 'lucide-react'
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

const inputCls = 'w-full rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm outline-none focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-100'
const pageSize = 20

export default function AuditPage() {
  const { t } = useTranslation()
  const [events, setEvents] = useState<AuditEvent[]>([])
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
      const data = await apiFetch<{ events: AuditEvent[]; total?: number }>(`/api/v1/audit/events${suffix}`)
      setEvents(data.events ?? [])
      setTotal(data.total ?? data.events?.length ?? 0)
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

      <section className="mb-5 rounded-xl border border-neutral-200/80 bg-white p-4 dark:border-zinc-700/60 dark:bg-zinc-900/40">
        <div className="mb-3 flex items-center gap-2 text-sm font-semibold text-neutral-800 dark:text-zinc-100">
          <Filter className="size-4 text-neutral-400" strokeWidth={1.8} />
          {t('audit.filters')}
        </div>
        <div className="grid gap-3 md:grid-cols-4">
          <FilterField label={t('audit.actor')} value={filters.actorId} onChange={actorId => updateFilter('actorId', actorId)} placeholder="admin" />
          <FilterField label={t('audit.action')} value={filters.action} onChange={action => updateFilter('action', action)} placeholder="connection.use" />
          <FilterField label={t('audit.resourceType')} value={filters.resourceType} onChange={resourceType => updateFilter('resourceType', resourceType)} placeholder="connection" />
          <FilterField label={t('audit.resourceId')} value={filters.resourceId} onChange={resourceId => updateFilter('resourceId', resourceId)} placeholder="conn-..." />
        </div>
      </section>

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
                  <th className="px-4 py-3 text-right">{t('audit.details')}</th>
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

function FilterField({ label, value, onChange, placeholder }: { label: string; value: string; onChange: (value: string) => void; placeholder?: string }) {
  return (
    <label className="block">
      <span className="mb-1 block text-xs font-medium text-neutral-500 dark:text-zinc-400">{label}</span>
      <input className={inputCls} value={value} onChange={e => onChange(e.target.value)} placeholder={placeholder} />
    </label>
  )
}

function AuditEventRow({ event, onOpen }: { event: AuditEvent; onOpen: () => void }) {
  const fmtDateTime = useFormatDateTime()
  const { t } = useTranslation()
  const created = fmtDateTime(event.createdAt)
  return (
    <tr className="text-neutral-700 dark:text-zinc-300">
      <td className="whitespace-nowrap px-4 py-3 text-xs text-neutral-500 dark:text-zinc-500">{created}</td>
      <td className="px-4 py-3">
        <div className="max-w-[180px] truncate">
          <span className="text-xs text-neutral-400 dark:text-zinc-500">{event.actorType}:</span>{event.actorId || '-'}
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
      <td className="px-4 py-3 text-right">
        <button type="button" onClick={onOpen} className="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-neutral-500 transition-colors hover:bg-neutral-100 hover:text-neutral-900 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100">
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
