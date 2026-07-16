import { useCallback, useEffect, useState } from 'react'
import { Filter, RefreshCw, ShieldCheck } from 'lucide-react'
import { apiFetch } from '../lib/api'
import { cn } from '../lib/cn'

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

export default function AuditPage() {
  const [events, setEvents] = useState<AuditEvent[]>([])
  const [loading, setLoading] = useState(true)
  const [filters, setFilters] = useState({
    actorId: '',
    action: '',
    resourceType: '',
    resourceId: '',
    limit: '100',
  })

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const params = new URLSearchParams()
      for (const [key, value] of Object.entries(filters)) {
        if (value.trim()) params.set(key, value.trim())
      }
      const suffix = params.toString() ? `?${params.toString()}` : ''
      const data = await apiFetch<{ events: AuditEvent[] }>(`/api/v1/audit/events${suffix}`)
      setEvents(data.events ?? [])
    } finally {
      setLoading(false)
    }
  }, [filters])

  useEffect(() => { void load() }, [load])

  return (
    <div className="animate-fade-in px-8 py-6">
      <div className="flex items-center justify-between pb-5">
        <div>
          <h1 className="text-xl font-semibold text-neutral-900 dark:text-zinc-100">Audit Log</h1>
          <p className="mt-0.5 text-sm text-neutral-500 dark:text-zinc-500">Trace sensitive workspace, connection, provider, runtime, and agent operations.</p>
        </div>
        <button type="button" onClick={() => void load()} disabled={loading} className="inline-flex items-center gap-2 rounded-lg border border-neutral-200 px-3 py-2 text-sm font-medium text-neutral-600 hover:bg-neutral-50 disabled:opacity-50 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800">
          <RefreshCw className={cn('size-4', loading && 'animate-spin')} strokeWidth={1.8} />
          Refresh
        </button>
      </div>

      <section className="mb-5 rounded-xl border border-neutral-200/80 bg-white p-4 dark:border-zinc-700/60 dark:bg-zinc-900/40">
        <div className="mb-3 flex items-center gap-2 text-sm font-semibold text-neutral-800 dark:text-zinc-100">
          <Filter className="size-4 text-neutral-400" strokeWidth={1.8} />
          Filters
        </div>
        <div className="grid gap-3 md:grid-cols-5">
          <FilterField label="Actor" value={filters.actorId} onChange={actorId => setFilters(v => ({ ...v, actorId }))} placeholder="admin or sample/pm" />
          <FilterField label="Action" value={filters.action} onChange={action => setFilters(v => ({ ...v, action }))} placeholder="connection.use" />
          <FilterField label="Resource type" value={filters.resourceType} onChange={resourceType => setFilters(v => ({ ...v, resourceType }))} placeholder="connection" />
          <FilterField label="Resource ID" value={filters.resourceId} onChange={resourceId => setFilters(v => ({ ...v, resourceId }))} placeholder="conn-..." />
          <FilterField label="Limit" value={filters.limit} onChange={limit => setFilters(v => ({ ...v, limit }))} placeholder="100" />
        </div>
      </section>

      {loading ? (
        <div className="flex items-center justify-center gap-2 py-16 text-sm text-neutral-500">
          <div className="size-5 animate-spin rounded-full border-2 border-neutral-300 border-t-sky-600 dark:border-zinc-600 dark:border-t-sky-400" />
          Loading audit events
        </div>
      ) : events.length === 0 ? (
        <div className="rounded-xl border border-dashed border-neutral-300 bg-white p-12 text-center dark:border-zinc-700 dark:bg-zinc-900/40">
          <ShieldCheck className="mx-auto mb-3 size-10 text-neutral-300 dark:text-zinc-600" strokeWidth={1.5} />
          <p className="text-sm font-medium text-neutral-600 dark:text-zinc-300">No audit events match these filters</p>
          <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">Sensitive operations will appear here as they happen.</p>
        </div>
      ) : (
        <div className="space-y-3">
          {events.map(event => <AuditEventCard key={event.id} event={event} />)}
        </div>
      )}
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

function AuditEventCard({ event }: { event: AuditEvent }) {
  const timestamp = new Date(event.createdAt)
  const created = Number.isNaN(timestamp.getTime()) ? event.createdAt : timestamp.toLocaleString()
  const hasBefore = event.before !== undefined && event.before !== null
  const hasAfter = event.after !== undefined && event.after !== null
  return (
    <article className="rounded-xl border border-neutral-200/80 bg-white p-4 dark:border-zinc-700/60 dark:bg-zinc-900/40">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="rounded-full bg-sky-100 px-2 py-0.5 text-xs font-medium text-sky-700 dark:bg-sky-900/30 dark:text-sky-300">{event.action}</span>
            <span className="rounded-full bg-neutral-100 px-2 py-0.5 text-xs font-medium text-neutral-500 dark:bg-zinc-800 dark:text-zinc-400">{event.resourceType}</span>
            {event.resourceId && <span className="max-w-xs truncate rounded-full border border-neutral-200 px-2 py-0.5 text-xs text-neutral-500 dark:border-zinc-700 dark:text-zinc-400">{event.resourceId}</span>}
          </div>
          <p className="mt-2 text-sm font-medium text-neutral-900 dark:text-zinc-100">{event.summary || event.action}</p>
          <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">
            {event.actorType}:{event.actorId} · {created}{event.ip ? ` · ${event.ip}` : ''}
          </p>
        </div>
        <span className="font-mono text-[11px] text-neutral-300 dark:text-zinc-600">{event.id}</span>
      </div>
      {(hasBefore || hasAfter) && (
        <div className="mt-3 grid gap-3 lg:grid-cols-2">
          {hasBefore && <JSONBlock label="Before" value={event.before} />}
          {hasAfter && <JSONBlock label="After" value={event.after} />}
        </div>
      )}
    </article>
  )
}

function JSONBlock({ label, value }: { label: string; value: unknown }) {
  return (
    <div className="rounded-lg bg-neutral-50 p-3 dark:bg-zinc-950/60">
      <p className="mb-2 text-xs font-medium text-neutral-400 dark:text-zinc-500">{label}</p>
      <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-words text-xs leading-relaxed text-neutral-600 dark:text-zinc-300">
        {JSON.stringify(value, null, 2)}
      </pre>
    </div>
  )
}
