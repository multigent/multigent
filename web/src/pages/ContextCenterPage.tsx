import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import Markdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import {
  Database,
  Plus,
  RefreshCw,
  Rss,
  Search,
  ServerCog,
  X,
} from 'lucide-react'
import { apiFetch, apiPost } from '../lib/api'
import { cn } from '../lib/cn'
import { useApiJson } from '../lib/use-api'
import { useFormatDateTime } from '../lib/format-datetime'

type TabKey = 'sources' | 'collectors' | 'items'

type ContextSource = {
  id: string
  type: string
  name: string
  description?: string
  connectionRef?: string
  status?: string
  metadata?: Record<string, unknown>
  createdBy?: string
  createdAt?: string
  updatedAt?: string
}

type ContextItem = {
  id: string
  sourceId?: string
  sourceType?: string
  sourceItemId?: string
  sourceUrl?: string
  projectId?: string
  agentWorkerId?: string
  authorType?: string
  authorId?: string
  occurredAt?: string
  collectedAt?: string
  title?: string
  summary?: string
  content?: string
  contentRef?: string
  payload?: Record<string, unknown>
  labels?: Record<string, unknown>
  sensitivity?: string
  status?: string
  dedupeKey?: string
  usageCount?: number
  createdAt?: string
  updatedAt?: string
}

const pagePad = 'animate-fade-in px-8 py-6'
const cardCls = 'rounded-xl border border-neutral-200 bg-white shadow-sm dark:border-zinc-800 dark:bg-zinc-900/40'
const inputCls = 'h-9 rounded-lg border border-neutral-200 bg-white px-3 text-sm text-neutral-700 outline-none transition-colors focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-200'
const selectCls = `${inputCls} dark:[color-scheme:dark]`
const secondaryButtonCls = 'inline-flex h-9 items-center gap-2 rounded-lg border border-neutral-200 bg-white px-3 text-sm font-medium text-neutral-600 transition-colors hover:bg-neutral-50 disabled:opacity-50 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:bg-zinc-800'
const primaryButtonCls = 'inline-flex h-9 items-center gap-2 rounded-lg bg-neutral-900 px-3 text-sm font-medium text-white transition-colors hover:bg-neutral-800 disabled:opacity-50 dark:bg-zinc-100 dark:text-zinc-950 dark:hover:bg-white'

export default function ContextCenterPage() {
  const { t } = useTranslation()
  const [tab, setTab] = useState<TabKey>('sources')
  const [reloadKey, setReloadKey] = useState(0)

  return (
    <div className={pagePad}>
      <div className="flex flex-wrap items-center justify-between gap-3 pb-5">
        <div>
          <h1 className="text-xl font-semibold text-neutral-900 dark:text-zinc-100">{t('contextCenter.title', { defaultValue: '上下文' })}</h1>
          <p className="mt-0.5 text-sm text-neutral-500 dark:text-zinc-500">
            {t('contextCenter.subtitle', { defaultValue: '接入群聊、文档、代码仓库和本地会话，让 Agent 按权限主动检索背景信息。' })}
          </p>
        </div>
        <button type="button" onClick={() => setReloadKey(k => k + 1)} className={secondaryButtonCls}>
          <RefreshCw className="size-4" strokeWidth={1.8} />
          {t('common.refresh')}
        </button>
      </div>

      <div className="mb-5 flex w-fit rounded-lg border border-neutral-200 bg-neutral-50 p-1 dark:border-zinc-800 dark:bg-zinc-900">
        <TabButton active={tab === 'sources'} onClick={() => setTab('sources')} icon={Rss} label={t('contextCenter.sources', { defaultValue: '信息源' })} />
        <TabButton active={tab === 'collectors'} onClick={() => setTab('collectors')} icon={ServerCog} label={t('contextCenter.collectors', { defaultValue: '抓取器' })} />
        <TabButton active={tab === 'items'} onClick={() => setTab('items')} icon={Database} label={t('contextCenter.items', { defaultValue: '上下文库' })} />
      </div>

      {tab === 'sources' && <SourcesPanel reloadKey={reloadKey} onChanged={() => setReloadKey(k => k + 1)} />}
      {tab === 'collectors' && <CollectorsPanel reloadKey={reloadKey} />}
      {tab === 'items' && <ItemsPanel reloadKey={reloadKey} />}
    </div>
  )
}

function TabButton({ active, onClick, icon: Icon, label }: { active: boolean; onClick: () => void; icon: typeof Database; label: string }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'inline-flex h-8 items-center gap-2 rounded-md px-3 text-sm font-medium transition-colors',
        active
          ? 'bg-white text-neutral-900 shadow-sm dark:bg-zinc-800 dark:text-zinc-100'
          : 'text-neutral-500 hover:text-neutral-800 dark:text-zinc-500 dark:hover:text-zinc-200',
      )}
    >
      <Icon className="size-4" strokeWidth={1.8} />
      {label}
    </button>
  )
}

function SourcesPanel({ reloadKey, onChanged }: { reloadKey: number; onChanged: () => void }) {
  const { t } = useTranslation()
  const state = useApiJson<{ sources?: ContextSource[] }>('/api/v1/context/sources?limit=200', reloadKey, { keepPreviousDataOnReload: true })
  const [creating, setCreating] = useState(false)
  const sources = state.status === 'ok' ? (state.data.sources ?? []) : []

  return (
    <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_22rem]">
      <div className={cardCls}>
        <div className="border-b border-neutral-100 px-5 py-4 dark:border-zinc-800">
          <h2 className="text-sm font-semibold text-neutral-900 dark:text-zinc-100">{t('contextCenter.sourceList', { defaultValue: '信息源' })}</h2>
          <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">{t('contextCenter.sourceListHint', { defaultValue: '信息源只描述数据从哪里来；实际抓取由外部 Collector 负责。' })}</p>
        </div>
        {state.status === 'loading' ? (
          <LoadingState />
        ) : sources.length === 0 ? (
          <EmptyState icon={Rss} title={t('contextCenter.noSources', { defaultValue: '还没有信息源' })} body={t('contextCenter.noSourcesHint', { defaultValue: '先创建一个 Lark、GitHub、本地 Session 或 RSS 信息源，再让 Collector 写入标准 ContextItem。' })} />
        ) : (
          <div className="divide-y divide-neutral-100 dark:divide-zinc-800">
            {sources.map(source => <SourceRow key={source.id} source={source} />)}
          </div>
        )}
      </div>
      <div className={cardCls}>
        <div className="border-b border-neutral-100 px-5 py-4 dark:border-zinc-800">
          <h2 className="text-sm font-semibold text-neutral-900 dark:text-zinc-100">{t('contextCenter.createSource', { defaultValue: '添加信息源' })}</h2>
          <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">{t('contextCenter.createSourceHint', { defaultValue: '这里只登记来源和范围，不保存平台密钥。密钥仍通过外部工具或 Collector 自己管理。' })}</p>
        </div>
        <CreateSourceForm creating={creating} setCreating={setCreating} onCreated={onChanged} />
      </div>
    </div>
  )
}

function SourceRow({ source }: { source: ContextSource }) {
  const fmtDateTime = useFormatDateTime()
  return (
    <div className="flex items-start gap-4 px-5 py-4">
      <div className="mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-lg bg-neutral-100 text-neutral-500 dark:bg-zinc-800 dark:text-zinc-400">
        <Rss className="size-4" strokeWidth={1.8} />
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <p className="truncate text-sm font-medium text-neutral-900 dark:text-zinc-100">{source.name}</p>
          <Badge>{source.type}</Badge>
          <StatusBadge status={source.status || 'active'} />
        </div>
        {source.description && <p className="mt-1 text-sm text-neutral-500 dark:text-zinc-400">{source.description}</p>}
        <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-neutral-400 dark:text-zinc-500">
          <span>ID <code className="font-mono">{source.id}</code></span>
          {source.connectionRef && <span>connection <code className="font-mono">{source.connectionRef}</code></span>}
          <span>{fmtDateTime(source.updatedAt || source.createdAt)}</span>
        </div>
      </div>
    </div>
  )
}

function CreateSourceForm({ creating, setCreating, onCreated }: { creating: boolean; setCreating: (value: boolean) => void; onCreated: () => void }) {
  const { t } = useTranslation()
  const [type, setType] = useState('lark_im')
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [connectionRef, setConnectionRef] = useState('')

  async function submit() {
    const sourceName = name.trim()
    if (!sourceName) return
    setCreating(true)
    try {
      await apiPost('/api/v1/context/sources', {
        type,
        name: sourceName,
        description: description.trim(),
        connectionRef: connectionRef.trim(),
      })
      setName('')
      setDescription('')
      setConnectionRef('')
      onCreated()
    } finally {
      setCreating(false)
    }
  }

  return (
    <div className="space-y-3 p-5">
      <label className="block">
        <span className="mb-1 block text-xs font-medium text-neutral-500 dark:text-zinc-400">{t('contextCenter.sourceType', { defaultValue: '类型' })}</span>
        <select value={type} onChange={e => setType(e.target.value)} className={selectCls}>
          <option value="lark_im">Lark / Feishu IM</option>
          <option value="lark_doc">Lark / Feishu Doc</option>
          <option value="github">GitHub</option>
          <option value="agent_session">Agent Session</option>
          <option value="local_file">Local File</option>
          <option value="rss">RSS / Web</option>
          <option value="manual">Manual</option>
        </select>
      </label>
      <label className="block">
        <span className="mb-1 block text-xs font-medium text-neutral-500 dark:text-zinc-400">{t('common.name')}</span>
        <input value={name} onChange={e => setName(e.target.value)} className={inputCls} placeholder={t('contextCenter.sourceNamePlaceholder', { defaultValue: 'MCP 联调群' })} />
      </label>
      <label className="block">
        <span className="mb-1 block text-xs font-medium text-neutral-500 dark:text-zinc-400">{t('common.description')}</span>
        <textarea value={description} onChange={e => setDescription(e.target.value)} className="min-h-20 w-full rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-700 outline-none transition-colors focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-200" />
      </label>
      <label className="block">
        <span className="mb-1 block text-xs font-medium text-neutral-500 dark:text-zinc-400">{t('contextCenter.connectionRef', { defaultValue: '关联外部工具连接' })}</span>
        <input value={connectionRef} onChange={e => setConnectionRef(e.target.value)} className={inputCls} placeholder="conn-..." />
      </label>
      <button type="button" onClick={() => void submit()} disabled={creating || !name.trim()} className={primaryButtonCls}>
        <Plus className="size-4" strokeWidth={1.8} />
        {t('contextCenter.createSource', { defaultValue: '添加信息源' })}
      </button>
    </div>
  )
}

function CollectorsPanel({ reloadKey }: { reloadKey: number }) {
  const { t } = useTranslation()
  const state = useApiJson<{ sources?: ContextSource[] }>('/api/v1/context/sources?limit=200', reloadKey, { keepPreviousDataOnReload: true })
  const sources = state.status === 'ok' ? (state.data.sources ?? []) : []
  const command = `curl -X POST "$MULTIGENT_API_URL/api/v1/context/items/batch" \\
  -H "Authorization: Bearer $MULTIGENT_CLIENT_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{"items":[{"sourceType":"lark_im","title":"...","content":"...","dedupeKey":"lark:message-id"}]}'`

  return (
    <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_24rem]">
      <div className={cardCls}>
        <div className="border-b border-neutral-100 px-5 py-4 dark:border-zinc-800">
          <h2 className="text-sm font-semibold text-neutral-900 dark:text-zinc-100">{t('contextCenter.collectorRuntime', { defaultValue: '外部 Collector' })}</h2>
          <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">
            {t('contextCenter.collectorRuntimeHint', { defaultValue: 'Collector 是独立进程：它从飞书、GitHub、RSS 或本地文件抓取数据，然后通过标准 API 写入 Context Center。' })}
          </p>
        </div>
        <div className="grid gap-4 p-5 lg:grid-cols-3">
          <CollectorStep index="1" title={t('contextCenter.collectorStepSource', { defaultValue: '登记信息源' })} body={t('contextCenter.collectorStepSourceBody', { defaultValue: '先创建 source，明确数据从哪里来，以及大致归属范围。' })} />
          <CollectorStep index="2" title={t('contextCenter.collectorStepToken', { defaultValue: '生成写入凭证' })} body={t('contextCenter.collectorStepTokenBody', { defaultValue: '在账户页生成 context.write token，交给本地或客户侧 collector。' })} />
          <CollectorStep index="3" title={t('contextCenter.collectorStepPush', { defaultValue: '批量写入 ContextItem' })} body={t('contextCenter.collectorStepPushBody', { defaultValue: 'Collector 做去重、脱敏和敏感度标记后，调用 batch API 写入。' })} />
        </div>
        <div className="border-t border-neutral-100 p-5 dark:border-zinc-800">
          <p className="mb-2 text-xs font-medium text-neutral-500 dark:text-zinc-400">{t('contextCenter.writeApiExample', { defaultValue: '标准写入示例' })}</p>
          <pre className="overflow-x-auto rounded-lg bg-neutral-950 p-4 text-xs leading-relaxed text-zinc-100"><code>{command}</code></pre>
        </div>
      </div>

      <div className={cardCls}>
        <div className="border-b border-neutral-100 px-5 py-4 dark:border-zinc-800">
          <h2 className="text-sm font-semibold text-neutral-900 dark:text-zinc-100">{t('contextCenter.collectorStatus', { defaultValue: '抓取状态' })}</h2>
          <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">{t('contextCenter.collectorStatusHint', { defaultValue: '第一版先用信息源和写入记录表达状态；后续会加入 collector 注册、心跳、任务和日志。' })}</p>
        </div>
        <div className="divide-y divide-neutral-100 dark:divide-zinc-800">
          {sources.length === 0 ? (
            <EmptyState icon={ServerCog} title={t('contextCenter.noCollectorSources', { defaultValue: '暂无可抓取来源' })} body={t('contextCenter.noCollectorSourcesHint', { defaultValue: '创建信息源后，可以启动外部 collector 写入数据。' })} compact />
          ) : sources.map(source => (
            <div key={source.id} className="px-5 py-4">
              <div className="flex items-center justify-between gap-3">
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium text-neutral-900 dark:text-zinc-100">{source.name}</p>
                  <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">{source.type} · {source.id}</p>
                </div>
                <StatusBadge status={source.status || 'active'} />
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

function CollectorStep({ index, title, body }: { index: string; title: string; body: string }) {
  return (
    <div className="rounded-lg border border-neutral-200 bg-neutral-50 p-4 dark:border-zinc-800 dark:bg-zinc-950/40">
      <div className="mb-3 flex size-7 items-center justify-center rounded-md bg-white text-xs font-semibold text-neutral-700 shadow-sm dark:bg-zinc-900 dark:text-zinc-200">{index}</div>
      <p className="text-sm font-medium text-neutral-900 dark:text-zinc-100">{title}</p>
      <p className="mt-1 text-xs leading-relaxed text-neutral-500 dark:text-zinc-500">{body}</p>
    </div>
  )
}

function ItemsPanel({ reloadKey }: { reloadKey: number }) {
  const { t } = useTranslation()
  const [query, setQuery] = useState('')
  const [sourceType, setSourceType] = useState('')
  const [selected, setSelected] = useState<ContextItem | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [localReload, setLocalReload] = useState(0)
  const params = useMemo(() => {
    const q = new URLSearchParams()
    q.set('limit', '100')
    if (query.trim()) q.set('q', query.trim())
    if (sourceType) q.set('sourceType', sourceType)
    return q.toString()
  }, [query, sourceType])
  const state = useApiJson<{ items?: ContextItem[] }>(`/api/v1/context/items?${params}`, reloadKey + localReload, { keepPreviousDataOnReload: true })
  const items = state.status === 'ok' ? (state.data.items ?? []) : []

  const openItem = useCallback(async (item: ContextItem) => {
    setDetailLoading(true)
    setSelected(item)
    try {
      const data = await apiFetch<{ item: ContextItem }>(`/api/v1/context/items/${encodeURIComponent(item.id)}`)
      setSelected(data.item)
    } finally {
      setDetailLoading(false)
    }
  }, [])

  return (
    <div className={cardCls}>
      <div className="border-b border-neutral-100 px-5 py-4 dark:border-zinc-800">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 className="text-sm font-semibold text-neutral-900 dark:text-zinc-100">{t('contextCenter.itemList', { defaultValue: '上下文库' })}</h2>
            <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">{t('contextCenter.itemListHint', { defaultValue: '这里是 Collector 写入的标准信息项。Agent 通过 mga context 按权限检索，而不是一次性注入所有内容。' })}</p>
          </div>
          <button type="button" onClick={() => setLocalReload(k => k + 1)} className={secondaryButtonCls}>
            <RefreshCw className="size-4" strokeWidth={1.8} />
            {t('common.refresh')}
          </button>
        </div>
        <div className="mt-4 flex flex-wrap items-center gap-2">
          <div className="relative min-w-72">
            <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-neutral-400" strokeWidth={1.8} />
            <input value={query} onChange={e => setQuery(e.target.value)} className={cn(inputCls, 'w-full pl-9')} placeholder={t('contextCenter.searchPlaceholder', { defaultValue: '搜索标题、摘要、正文或标签…' })} />
          </div>
          <select value={sourceType} onChange={e => setSourceType(e.target.value)} className={selectCls}>
            <option value="">{t('contextCenter.allSourceTypes', { defaultValue: '全部类型' })}</option>
            <option value="lark_im">Lark / Feishu IM</option>
            <option value="lark_doc">Lark / Feishu Doc</option>
            <option value="github">GitHub</option>
            <option value="agent_session">Agent Session</option>
            <option value="local_file">Local File</option>
            <option value="rss">RSS / Web</option>
            <option value="manual">Manual</option>
          </select>
          {(query || sourceType) && (
            <button type="button" onClick={() => { setQuery(''); setSourceType('') }} className="rounded-lg px-2.5 py-2 text-sm font-medium text-neutral-500 transition-colors hover:bg-neutral-100 hover:text-neutral-800 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-200">
              {t('common.clear')}
            </button>
          )}
          <span className="text-xs text-neutral-400 dark:text-zinc-500">{t('contextCenter.itemCount', { defaultValue: '{{count}} 条', count: items.length })}</span>
        </div>
      </div>

      {state.status === 'loading' ? (
        <LoadingState />
      ) : items.length === 0 ? (
        <EmptyState icon={Database} title={t('contextCenter.noItems', { defaultValue: '还没有上下文' })} body={t('contextCenter.noItemsHint', { defaultValue: '启动 Collector 或用 API 写入数据后，这里会显示标准化后的 ContextItem。' })} />
      ) : (
        <div className="overflow-x-auto">
          <table className="min-w-full divide-y divide-neutral-100 text-sm dark:divide-zinc-800">
            <thead className="bg-neutral-50 text-left text-xs font-medium text-neutral-500 dark:bg-zinc-900 dark:text-zinc-400">
              <tr>
                <th className="px-4 py-3">{t('contextCenter.contextTitle', { defaultValue: '标题' })}</th>
                <th className="px-4 py-3">{t('contextCenter.source', { defaultValue: '来源' })}</th>
                <th className="px-4 py-3">{t('contextCenter.scope', { defaultValue: '范围' })}</th>
                <th className="px-4 py-3">{t('contextCenter.sensitivity', { defaultValue: '敏感度' })}</th>
                <th className="px-4 py-3">{t('contextCenter.collectedAt', { defaultValue: '采集时间' })}</th>
                <th className="sticky right-0 w-24 bg-neutral-50 px-4 py-3 text-right dark:bg-zinc-900">{t('common.actions')}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-neutral-100 dark:divide-zinc-800">
              {items.map(item => <ContextItemRow key={item.id} item={item} onOpen={() => void openItem(item)} />)}
            </tbody>
          </table>
        </div>
      )}
      {selected && <ContextItemModal item={selected} loading={detailLoading} onClose={() => setSelected(null)} />}
    </div>
  )
}

function ContextItemRow({ item, onOpen }: { item: ContextItem; onOpen: () => void }) {
  const fmtDateTime = useFormatDateTime()
  return (
    <tr className="text-neutral-700 dark:text-zinc-300">
      <td className="max-w-md px-4 py-3">
        <p className="truncate font-medium text-neutral-900 dark:text-zinc-100">{item.title || item.id}</p>
        {item.summary && <p className="mt-1 line-clamp-2 text-xs leading-relaxed text-neutral-500 dark:text-zinc-500">{item.summary}</p>}
        <p className="mt-1 truncate font-mono text-[11px] text-neutral-400 dark:text-zinc-600">{item.id}</p>
      </td>
      <td className="whitespace-nowrap px-4 py-3">
        <Badge>{item.sourceType || '-'}</Badge>
      </td>
      <td className="px-4 py-3 text-xs text-neutral-500 dark:text-zinc-400">
        {item.projectId || item.agentWorkerId || '-'}
      </td>
      <td className="whitespace-nowrap px-4 py-3"><SensitivityBadge value={item.sensitivity || 'L2'} /></td>
      <td className="whitespace-nowrap px-4 py-3 text-xs text-neutral-500 dark:text-zinc-400">{fmtDateTime(item.collectedAt || item.createdAt)}</td>
      <td className="sticky right-0 bg-white px-4 py-3 text-right dark:bg-zinc-900">
        <button type="button" onClick={onOpen} className="rounded-md px-2.5 py-1.5 text-xs font-medium text-neutral-600 hover:bg-neutral-100 dark:text-zinc-300 dark:hover:bg-zinc-800">
          查看
        </button>
      </td>
    </tr>
  )
}

function ContextItemModal({ item, loading, onClose }: { item: ContextItem; loading: boolean; onClose: () => void }) {
  const { t } = useTranslation()
  const fmtDateTime = useFormatDateTime()
  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center px-4 pt-[7vh]">
      <div className="absolute inset-0 bg-black/30 backdrop-blur-[2px] dark:bg-black/50" onClick={onClose} />
      <div className="relative flex max-h-[86vh] w-full max-w-5xl flex-col overflow-hidden rounded-xl border border-neutral-200 bg-white shadow-2xl dark:border-zinc-700 dark:bg-zinc-900">
        <div className="flex items-start justify-between gap-4 border-b border-neutral-100 px-5 py-4 dark:border-zinc-800">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="truncate text-base font-semibold text-neutral-900 dark:text-zinc-100">{item.title || item.id}</h2>
              <Badge>{item.sourceType || '-'}</Badge>
              <SensitivityBadge value={item.sensitivity || 'L2'} />
            </div>
            <p className="mt-1 font-mono text-xs text-neutral-400 dark:text-zinc-500">{item.id}</p>
          </div>
          <button type="button" onClick={onClose} className="rounded-md p-2 text-neutral-400 hover:bg-neutral-100 hover:text-neutral-700 dark:hover:bg-zinc-800 dark:hover:text-zinc-200">
            <X className="size-4" strokeWidth={1.8} />
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto p-5">
          {loading ? <LoadingState compact /> : (
            <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_18rem]">
              <div>
                {item.summary && (
                  <div className="mb-4 rounded-lg bg-neutral-50 p-4 text-sm leading-relaxed text-neutral-700 dark:bg-zinc-950/50 dark:text-zinc-300">
                    {item.summary}
                  </div>
                )}
                <div className="prose prose-sm max-w-none dark:prose-invert">
                  <Markdown remarkPlugins={[remarkGfm]}>{item.content || item.contentRef || t('contextCenter.noContent', { defaultValue: '没有正文内容。' })}</Markdown>
                </div>
              </div>
              <div className="space-y-3 text-xs">
                <Meta label={t('contextCenter.source', { defaultValue: '来源' })} value={`${item.sourceType || '-'} / ${item.sourceId || '-'}`} />
                <Meta label={t('contextCenter.sourceItem', { defaultValue: '源对象' })} value={item.sourceItemId || '-'} />
                <Meta label={t('contextCenter.scope', { defaultValue: '范围' })} value={item.projectId || item.agentWorkerId || '-'} />
                <Meta label={t('contextCenter.author', { defaultValue: '作者' })} value={`${item.authorType || '-'} / ${item.authorId || '-'}`} />
                <Meta label={t('contextCenter.occurredAt', { defaultValue: '发生时间' })} value={fmtDateTime(item.occurredAt)} />
                <Meta label={t('contextCenter.collectedAt', { defaultValue: '采集时间' })} value={fmtDateTime(item.collectedAt)} />
                <Meta label={t('contextCenter.dedupeKey', { defaultValue: '去重键' })} value={item.dedupeKey || '-'} mono />
                {item.sourceUrl && (
                  <a href={item.sourceUrl} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-xs font-medium text-sky-600 hover:text-sky-700 dark:text-sky-400">
                    {t('contextCenter.openOriginal', { defaultValue: '打开原始链接' })}
                  </a>
                )}
                <pre className="max-h-56 overflow-auto rounded-lg bg-neutral-950 p-3 text-[11px] leading-relaxed text-zinc-100">{JSON.stringify({ labels: item.labels, payload: item.payload }, null, 2)}</pre>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

function LoadingState({ compact = false }: { compact?: boolean }) {
  const { t } = useTranslation()
  return (
    <div className={cn('flex items-center justify-center gap-2 text-sm text-neutral-500 dark:text-zinc-500', compact ? 'py-8' : 'py-16')}>
      <div className="size-5 animate-spin rounded-full border-2 border-neutral-300 border-t-sky-600 dark:border-zinc-600 dark:border-t-sky-400" />
      {t('api.loading')}
    </div>
  )
}

function EmptyState({ icon: Icon, title, body, compact = false }: { icon: typeof Database; title: string; body: string; compact?: boolean }) {
  return (
    <div className={cn('text-center', compact ? 'px-5 py-8' : 'px-8 py-16')}>
      <Icon className="mx-auto mb-3 size-10 text-neutral-300 dark:text-zinc-600" strokeWidth={1.5} />
      <p className="text-sm font-medium text-neutral-700 dark:text-zinc-300">{title}</p>
      <p className="mx-auto mt-1 max-w-md text-xs leading-relaxed text-neutral-400 dark:text-zinc-500">{body}</p>
    </div>
  )
}

function Badge({ children }: { children: string }) {
  return <span className="inline-flex rounded-md bg-neutral-100 px-2 py-0.5 text-xs font-medium text-neutral-500 dark:bg-zinc-800 dark:text-zinc-400">{children}</span>
}

function StatusBadge({ status }: { status: string }) {
  const active = status === 'active'
  return (
    <span className={cn(
      'inline-flex rounded-md px-2 py-0.5 text-xs font-medium',
      active ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300' : 'bg-neutral-100 text-neutral-500 dark:bg-zinc-800 dark:text-zinc-400',
    )}>{status}</span>
  )
}

function SensitivityBadge({ value }: { value: string }) {
  const cls = value === 'L3'
    ? 'bg-rose-50 text-rose-700 dark:bg-rose-950/40 dark:text-rose-300'
    : value === 'L1'
      ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300'
      : 'bg-amber-50 text-amber-700 dark:bg-amber-950/40 dark:text-amber-300'
  return <span className={cn('inline-flex rounded-md px-2 py-0.5 text-xs font-medium', cls)}>{value}</span>
}

function Meta({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return (
    <div>
      <p className="mb-1 text-neutral-400 dark:text-zinc-500">{label}</p>
      <p className={cn('break-words text-neutral-700 dark:text-zinc-300', mono && 'font-mono')}>{value}</p>
    </div>
  )
}
