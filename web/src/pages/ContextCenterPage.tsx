import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Database, GitBranch, MessageSquare, RefreshCw, Rss, ServerCog } from 'lucide-react'
import { cn } from '../lib/cn'
import { useApiJson } from '../lib/use-api'
import { useFormatDateTime } from '../lib/format-datetime'

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

const pagePad = 'animate-fade-in px-8 py-6'
const cardCls = 'rounded-xl border border-neutral-200 bg-white shadow-sm dark:border-zinc-800 dark:bg-zinc-900/40'
const secondaryButtonCls = 'inline-flex h-9 items-center gap-2 rounded-lg border border-neutral-200 bg-white px-3 text-sm font-medium text-neutral-600 transition-colors hover:bg-neutral-50 disabled:opacity-50 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:bg-zinc-800'

export default function KnowledgeBasePage() {
  const { t } = useTranslation()
  const [reloadKey, setReloadKey] = useState(0)

  return (
    <div className={pagePad}>
      <div className="mb-5 flex flex-wrap items-start justify-between gap-3">
        <div className="max-w-3xl">
          <h1 className="text-lg font-semibold text-neutral-900 dark:text-zinc-100">自动同步器</h1>
          <p className="mt-1 text-sm leading-relaxed text-neutral-500 dark:text-zinc-400">
            这里只放自动同步外部数据的入口，比如 GitHub、Lark / Feishu、RSS、告警源等。<br />
            本地手动上传的文档、文件、会话记录，不放在这里，直接去「文档」或「文件」。
          </p>
        </div>
        <button type="button" onClick={() => setReloadKey(k => k + 1)} className={secondaryButtonCls}>
          <RefreshCw className="size-4" strokeWidth={1.8} />
          {t('common.refresh')}
        </button>
      </div>

      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_24rem]">
        <div className={cardCls}>
          <div className="border-b border-neutral-100 px-5 py-4 dark:border-zinc-800">
            <h2 className="text-sm font-semibold text-neutral-900 dark:text-zinc-100">什么适合做成抓取器</h2>
            <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">
              抓取器只负责“自动从外部拉数据并写入上下文库”。它不承担本地手动上传，也不承担文档 / 文件管理。
            </p>
          </div>

          <div className="grid gap-4 p-5 lg:grid-cols-2">
            <SourceUseCaseCard
              icon={GitBranch}
              title="代码与协作平台"
              body="GitHub / GitLab / Gitee 的 issue、PR、review、release note、comment 同步。"
              tag="自动同步"
            />
            <SourceUseCaseCard
              icon={MessageSquare}
              title="IM 与知识协作"
              body="Lark / 飞书 / Slack 的消息、群聊、卡片回调、会议摘要、讨论沉淀。"
              tag="自动同步"
            />
            <SourceUseCaseCard
              icon={Rss}
              title="外部订阅与告警"
              body="RSS、新闻流、Sentry、Linear、Webhook、定时拉取的第三方信息。"
              tag="自动同步"
            />
            <SourceUseCaseCard
              icon={Database}
              title="后续可扩展来源"
              body="客户系统、内部 SaaS、审批系统、数据面板，只要能自动拉取，都可以做成抓取器。"
              tag="扩展"
            />
          </div>

          <div className="border-t border-neutral-100 p-5 dark:border-zinc-800">
            <div className="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-200">
              本地 Markdown、WIP 备注、session JSONL、截图、录音转写、手工资料，不建议做成抓取器。直接去「文档」或「文件」上传更清楚。
            </div>
          </div>
        </div>

        <div className={cardCls}>
          <div className="border-b border-neutral-100 px-5 py-4 dark:border-zinc-800">
            <h2 className="text-sm font-semibold text-neutral-900 dark:text-zinc-100">已写入的来源</h2>
            <p className="mt-1 text-xs text-neutral-400 dark:text-zinc-500">这里显示已经同步进上下文库的来源记录。</p>
          </div>
          <ContextSourcesPanel reloadKey={reloadKey} />
        </div>
      </div>
    </div>
  )
}

function ContextSourcesPanel({ reloadKey }: { reloadKey: number }) {
  const state = useApiJson<{ sources?: ContextSource[] }>('/api/v1/knowledge-base/sources?limit=200', reloadKey, { keepPreviousDataOnReload: true })
  const sources = state.status === 'ok' ? (state.data.sources ?? []) : []

  if (state.status === 'loading') {
    return <LoadingState />
  }

  if (sources.length === 0) {
    return (
      <EmptyState
        icon={ServerCog}
        title="还没有同步来源"
        body="等你接入自动同步器后，这里会显示每个来源的状态、连接和最近更新时间。"
      />
    )
  }

  return (
    <div className="divide-y divide-neutral-100 dark:divide-zinc-800">
      {sources.map(source => (
        <SourceRow key={source.id} source={source} />
      ))}
    </div>
  )
}

function SourceUseCaseCard({
  icon: Icon,
  title,
  body,
  tag,
}: {
  icon: typeof ServerCog
  title: string
  body: string
  tag: string
}) {
  return (
    <div className="rounded-lg border border-neutral-200 bg-neutral-50 p-4 dark:border-zinc-800 dark:bg-zinc-950/40">
      <div className="flex items-center justify-between gap-3">
        <div className="flex size-8 items-center justify-center rounded-md bg-white text-neutral-500 shadow-sm dark:bg-zinc-900 dark:text-zinc-400">
          <Icon className="size-4" strokeWidth={1.8} />
        </div>
        <span className="rounded-md bg-neutral-900 px-2 py-0.5 text-[11px] font-medium text-white dark:bg-zinc-100 dark:text-zinc-950">{tag}</span>
      </div>
      <p className="mt-3 text-sm font-medium text-neutral-900 dark:text-zinc-100">{title}</p>
      <p className="mt-1 text-xs leading-relaxed text-neutral-500 dark:text-zinc-500">{body}</p>
    </div>
  )
}

function SourceRow({ source }: { source: ContextSource }) {
  const fmtDateTime = useFormatDateTime()
  return (
    <div className="px-5 py-4">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="truncate text-sm font-medium text-neutral-900 dark:text-zinc-100">{collectorDisplayName(source)}</p>
          <p className="mt-1 truncate text-xs text-neutral-400 dark:text-zinc-500">
            {source.name} · {source.type}
          </p>
        </div>
        <StatusBadge status={source.status || 'active'} />
      </div>
      <div className="mt-3 grid gap-2 text-xs text-neutral-500 dark:text-zinc-400">
        <MetaRow label="来源 ID" value={source.id} mono />
        <MetaRow label="连接" value={source.connectionRef || '-'} />
        <MetaRow label="创建时间" value={fmtDateTime(source.createdAt)} />
        <MetaRow label="更新时间" value={fmtDateTime(source.updatedAt)} />
      </div>
    </div>
  )
}

function collectorDisplayName(source: ContextSource) {
  switch (source.type) {
    case 'lark_im':
    case 'lark_doc':
      return 'Lark / Feishu 同步源'
    case 'github':
      return 'GitHub 同步源'
    case 'rss':
      return 'RSS / Web 同步源'
    case 'agent_session':
    case 'local_file':
      return '本地导入记录'
    case 'manual':
      return '手动写入记录'
    default:
      return '自定义同步源'
  }
}

function MetaRow({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex min-w-0 items-start justify-between gap-3">
      <span className="shrink-0 text-neutral-400 dark:text-zinc-500">{label}</span>
      <span className={cn('min-w-0 break-all text-right text-neutral-700 dark:text-zinc-300', mono && 'font-mono')}>{value}</span>
    </div>
  )
}

function LoadingState() {
  const { t } = useTranslation()
  return (
    <div className="flex items-center justify-center gap-2 px-5 py-12 text-sm text-neutral-500 dark:text-zinc-500">
      <div className="size-5 animate-spin rounded-full border-2 border-neutral-300 border-t-sky-600 dark:border-zinc-600 dark:border-t-sky-400" />
      {t('api.loading')}
    </div>
  )
}

function EmptyState({ icon: Icon, title, body }: { icon: typeof ServerCog; title: string; body: string }) {
  return (
    <div className="px-5 py-12 text-center">
      <Icon className="mx-auto mb-3 size-10 text-neutral-300 dark:text-zinc-600" strokeWidth={1.5} />
      <p className="text-sm font-medium text-neutral-700 dark:text-zinc-300">{title}</p>
      <p className="mx-auto mt-1 max-w-sm text-xs leading-relaxed text-neutral-400 dark:text-zinc-500">{body}</p>
    </div>
  )
}

function StatusBadge({ status }: { status: string }) {
  const active = status === 'active'
  return (
    <span
      className={cn(
        'inline-flex rounded-md px-2 py-0.5 text-xs font-medium',
        active ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300' : 'bg-neutral-100 text-neutral-500 dark:bg-zinc-800 dark:text-zinc-400',
      )}
    >
      {status}
    </span>
  )
}
