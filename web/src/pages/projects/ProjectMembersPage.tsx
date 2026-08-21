import { useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Users, Bot, User, UserMinus, Plus, X } from 'lucide-react'
import { cn } from '../../lib/cn'
import { PlaceholderCard } from '../../components/ui/PlaceholderCard'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { useFormatDateTime } from '../../lib/format-datetime'
import { useApiJson } from '../../lib/use-api'
import { apiDelete, apiPost } from '../../lib/api'
import { useWorkspaceAccess } from '../../lib/workspace-access'

const fieldCls =
  'mt-1 w-full rounded-lg border border-neutral-300 bg-white px-2.5 py-1.5 text-sm text-neutral-900 outline-none transition-colors focus:border-sky-400 dark:border-zinc-600 dark:bg-zinc-800 dark:text-zinc-100 dark:[color-scheme:dark]'

type AgentWorker = {
  id: string
  name: string
  displayName?: string
  description?: string
  avatar?: string
  status?: string
}

type ProjectMembership = {
  id: string
  projectId: string
  memberType: string
  memberId: string
  role?: string
  title?: string
  prompt?: string
  autoPickTasks?: boolean
  attentionEnabled?: boolean
  priorityWeight?: number
  createdAt?: string
  agent?: AgentWorker
}

type ProjectMembershipsResp = {
  memberships?: ProjectMembership[]
}

type BillingEntitlementsResp = {
  entitlements?: {
    agentLimit?: number
  }
  usage?: {
    agents?: number
  }
}

function MemberAvatar({ avatar, isHuman = false }: { avatar?: string; isHuman?: boolean }) {
  const IconCmp = isHuman ? User : Bot
  const iconBg = isHuman
    ? 'bg-indigo-100 dark:bg-indigo-900/30'
    : 'bg-violet-100 dark:bg-violet-900/30'
  const iconColor = isHuman
    ? 'text-indigo-600 dark:text-indigo-400'
    : 'text-violet-600 dark:text-violet-400'

  if (avatar) {
    return (
      <img
        src={avatar}
        alt=""
        className="size-10 shrink-0 rounded-lg bg-neutral-100 object-cover dark:bg-zinc-800"
        loading="lazy"
      />
    )
  }

  return (
    <div className={cn('flex size-10 shrink-0 items-center justify-center rounded-lg', iconBg)}>
      <IconCmp className={cn('size-5', iconColor)} strokeWidth={1.8} />
    </div>
  )
}

export default function ProjectMembersPage() {
  const { t } = useTranslation()
  const { isExample } = useWorkspaceAccess()
  const fmt = useFormatDateTime()
  const { projectId } = useParams<{ projectId: string }>()

  const [reloadKey, setReloadKey] = useState(0)
  const [pendingDelete, setPendingDelete] = useState<{ id: string; name: string } | null>(null)
  const [deleting, setDeleting] = useState(false)
  const membershipsPath =
    projectId != null && projectId !== ''
      ? `/api/v1/projects/${encodeURIComponent(projectId)}/memberships`
      : null
  const membershipsState = useApiJson<ProjectMembershipsResp>(membershipsPath, reloadKey)
  const billingState = useApiJson<BillingEntitlementsResp>('/api/v1/billing/entitlements', reloadKey)
  const memberships = membershipsState.status === 'ok' ? (membershipsState.data.memberships ?? []) : []
  const agentLimit = billingState.status === 'ok' ? (billingState.data?.entitlements?.agentLimit || 0) : 0
  const agentsUsed = billingState.status === 'ok' ? (billingState.data?.usage?.agents || 0) : 0
  const agentLimitReached = agentLimit > 0 && agentsUsed >= agentLimit

  async function deleteMember() {
    if (!projectId || !pendingDelete) return
    setDeleting(true)
    try {
      await apiDelete(`/api/v1/projects/${encodeURIComponent(projectId)}/memberships/${encodeURIComponent(pendingDelete.id)}`)
      setPendingDelete(null)
      setReloadKey((k) => k + 1)
    } catch (e) {
      alert(String(e))
    } finally {
      setDeleting(false)
    }
  }

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <div className="shrink-0 px-6 pt-5 pb-3">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-xl font-semibold text-neutral-900 dark:text-zinc-100">{t('projectNav.members')}</h1>
            <p className="mt-0.5 text-sm text-neutral-500 dark:text-zinc-500">{t('members.subtitle')}</p>
          </div>
          {projectId && !isExample && (
            <AssignAgentDialog
              projectId={projectId}
              existingWorkerIds={memberships.map(member => member.memberId)}
              agentLimitReached={agentLimitReached}
              agentLimitText={agentLimitReached ? `${agentsUsed} / ${agentLimit}` : ''}
              onAssigned={() => setReloadKey((k) => k + 1)}
            />
          )}
        </div>
        {isExample && (
          <div className="mt-4 rounded-xl border border-sky-200 bg-sky-50 px-4 py-3 text-sm text-sky-800 dark:border-sky-900/50 dark:bg-sky-900/20 dark:text-sky-300">
            {t('members.exampleReadonly')}
          </div>
        )}
        {agentLimitReached && !isExample && (
          <div className="mt-4 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-300">
            {t('settings.billingLimitHint')} <span className="font-medium">{agentsUsed} / {agentLimit}</span>
          </div>
        )}
      </div>

      <div className="flex-1 overflow-y-auto px-6 pb-6">
        {membershipsState.status === 'loading' && (
          <div className="flex items-center gap-2 py-12 justify-center">
            <div className="size-5 animate-spin rounded-full border-2 border-neutral-300 border-t-sky-600 dark:border-zinc-600 dark:border-t-sky-400" />
            <span className="text-sm text-neutral-500">{t('api.loading')}</span>
          </div>
        )}
        {membershipsState.status === 'error' && (
          <PlaceholderCard title={t('api.loadError')}>
            <p className="text-[13px]">{membershipsState.error.message}</p>
          </PlaceholderCard>
        )}
        {membershipsState.status === 'ok' && memberships.length === 0 && (
          <div className="flex flex-col items-center justify-center py-20 text-center">
            <div className="mb-3 flex size-14 items-center justify-center rounded-2xl bg-neutral-100 dark:bg-zinc-800/50">
              <Users className="size-6 text-neutral-400 dark:text-zinc-500" strokeWidth={1.5} />
            </div>
            <p className="text-base font-medium text-neutral-600 dark:text-zinc-400">{t('members.emptyTitle')}</p>
          </div>
        )}
        {membershipsState.status === 'ok' && memberships.length > 0 && (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
            {memberships.map((member) => {
              const agent = member.agent
              const name = agent?.name ?? member.memberId
              const displayName = member.title || agent?.displayName || name
              const isAgentWorker = member.memberType === 'agent_worker' && !!agent
              const identityBlock = (
                <div className="flex items-center gap-3">
                  <MemberAvatar avatar={agent?.avatar} isHuman={!isAgentWorker} />
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-semibold text-neutral-900 dark:text-zinc-100">{displayName}</p>
                    <p className="mt-0.5 truncate font-mono text-xs text-neutral-500 dark:text-zinc-500">{name}</p>
                  </div>
                </div>
              )
              return (
                <div
                  key={member.id}
                  data-tour-member-card={name}
                  className="group relative flex flex-col rounded-xl border border-neutral-200/80 bg-white p-4 transition-all duration-150 hover:border-neutral-300 hover:shadow-sm dark:border-zinc-700/60 dark:bg-zinc-900/40 dark:hover:border-zinc-700"
                >
                  {isAgentWorker ? (
                    <Link
                      to={`/projects/${encodeURIComponent(projectId!)}/members/${encodeURIComponent(name)}`}
                      className="block"
                    >
                      {identityBlock}
                    </Link>
                  ) : identityBlock}
                  {agent?.description && (
                    <p className="mt-3 line-clamp-2 text-sm leading-6 text-neutral-600 dark:text-zinc-400">{agent.description}</p>
                  )}
                  <div className="mt-3 flex flex-wrap items-center gap-2">
                    <span className="inline-flex items-center rounded-md bg-violet-50 px-2 py-0.5 text-[11px] font-medium text-violet-700 dark:bg-violet-950/40 dark:text-violet-300">
                      {isAgentWorker ? t('members.agentWorker') : t('members.memberTypeHuman')}
                    </span>
                    {member.role && (
                      <span className="inline-flex items-center rounded-md bg-neutral-100 px-2 py-0.5 text-[11px] font-medium text-neutral-600 dark:bg-zinc-800 dark:text-zinc-300">
                        {member.role}
                      </span>
                    )}
                    <span className="ml-auto text-[11px] text-neutral-400 dark:text-zinc-500">{member.createdAt ? fmt(member.createdAt) : ''}</span>
                    {!isExample && (
                      <button
                        type="button"
                        title={t('members.removeFromProject')}
                        onClick={(e) => {
                          e.preventDefault()
                          setPendingDelete({ id: member.id, name: displayName })
                        }}
                        className="rounded p-1 text-neutral-400 opacity-0 transition-all hover:bg-red-50 hover:text-red-600 group-hover:opacity-100 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                      >
                        <UserMinus className="size-3.5" strokeWidth={1.8} />
                      </button>
                    )}
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </div>
      <ConfirmDialog
        open={pendingDelete != null}
        title={t('members.fire')}
        description={pendingDelete ? t('members.confirmFire', { name: pendingDelete.name }) : ''}
        confirmLabel={t('common.delete')}
        cancelLabel={t('common.cancel')}
        busy={deleting}
        onCancel={() => setPendingDelete(null)}
        onConfirm={() => void deleteMember()}
      />
    </div>
  )
}

function AssignAgentDialog({ projectId, existingWorkerIds, agentLimitReached, agentLimitText, onAssigned }: {
  projectId: string
  existingWorkerIds: string[]
  agentLimitReached?: boolean
  agentLimitText?: string
  onAssigned: () => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [workerId, setWorkerId] = useState('')
  const [role, setRole] = useState('')
  const [title, setTitle] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const workersState = useApiJson<{ agents?: AgentWorker[] }>(open ? '/api/v1/agents' : null)
  const workers = workersState.status === 'ok' ? (workersState.data.agents ?? []) : []
  const availableWorkers = workers.filter(worker => !existingWorkerIds.includes(worker.id))

  function reset() {
    setWorkerId('')
    setRole('')
    setTitle('')
    setErr(null)
  }

  async function assign() {
    setErr(null)
    if (agentLimitReached) {
      setErr(t('settings.billingLimitHint'))
      return
    }
    if (!workerId.trim()) {
      setErr(t('forms.fillRequired'))
      return
    }
    setBusy(true)
    try {
      await apiPost(`/api/v1/projects/${encodeURIComponent(projectId)}/memberships`, {
        workerId: workerId.trim(),
        role: role.trim() || 'member',
        title: title.trim(),
        autoPickTasks: true,
        attentionEnabled: true,
      })
      setOpen(false)
      onAssigned()
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <button
        type="button"
        data-tour-member-add
        onClick={() => { reset(); setOpen(true) }}
        className="inline-flex items-center gap-2 rounded-lg border border-sky-600 bg-white px-3 py-2 text-sm font-medium text-sky-700 hover:bg-sky-50 dark:border-sky-500 dark:bg-zinc-900 dark:text-sky-400 dark:hover:bg-zinc-800"
      >
        <Plus className="size-4" strokeWidth={1.8} />
        {t('members.addMember')}
      </button>
      {open && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/45 p-4" role="presentation" onClick={() => !busy && setOpen(false)}>
          <div
            className="max-h-[min(90vh,620px)] w-full max-w-md overflow-y-auto rounded-xl border border-neutral-200 bg-white shadow-lg dark:border-zinc-700 dark:bg-zinc-900 animate-scale-in"
            onClick={(e) => e.stopPropagation()}
            role="dialog"
            aria-labelledby="assign-agent-title"
          >
            <div className="flex items-start justify-between gap-3 border-b border-neutral-200 px-4 py-3 dark:border-zinc-700">
              <div>
                <h2 id="assign-agent-title" className="text-base font-semibold text-neutral-900 dark:text-zinc-100">
                  {t('members.addMember')}
                </h2>
                <p className="mt-0.5 text-xs text-neutral-400 dark:text-zinc-500">
                  {t('members.assignWorkerDesc')}
                </p>
              </div>
              <button
                type="button"
                onClick={() => setOpen(false)}
                disabled={busy}
                className="rounded-lg p-1.5 text-neutral-400 hover:bg-neutral-100 hover:text-neutral-700 dark:hover:bg-zinc-800 dark:hover:text-zinc-200"
              >
                <X className="size-4" strokeWidth={1.8} />
              </button>
            </div>
            <div className="space-y-3 px-4 py-3">
              {agentLimitReached && (
                <div className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-300">
                  {t('settings.billingLimitHint')} {agentLimitText && <span className="font-medium">{agentLimitText}</span>}
                </div>
              )}
              <label className="block text-sm">
                <span className="text-neutral-600 dark:text-zinc-400">{t('members.agentWorker')} *</span>
                <select value={workerId} onChange={e => setWorkerId(e.target.value)} className={fieldCls} autoFocus>
                  <option value="">{t('members.selectAgentWorker')}</option>
                  {availableWorkers.map(worker => (
                    <option key={worker.id} value={worker.id}>
                      {(worker.displayName || worker.name)} ({worker.name})
                    </option>
                  ))}
                </select>
              </label>
              {workersState.status === 'loading' && <p className="text-xs text-neutral-400">{t('api.loading')}</p>}
              {workersState.status === 'ok' && workers.length === 0 && (
                <p className="text-xs text-amber-600 dark:text-amber-400">{t('members.noWorkspaceAgents')}</p>
              )}
              {workersState.status === 'ok' && workers.length > 0 && availableWorkers.length === 0 && (
                <p className="text-xs text-neutral-500 dark:text-zinc-400">{t('members.allWorkspaceAgentsAssigned')}</p>
              )}
              <label className="block text-sm">
                <span className="text-neutral-600 dark:text-zinc-400">{t('members.projectRole')}</span>
                <input value={role} onChange={e => setRole(e.target.value)} className={fieldCls} placeholder="project-manager" />
              </label>
              <label className="block text-sm">
                <span className="text-neutral-600 dark:text-zinc-400">{t('members.memberTitle')}</span>
                <input value={title} onChange={e => setTitle(e.target.value)} className={fieldCls} placeholder={t('members.memberTitlePlaceholder')} />
              </label>
              {err && <p className="text-sm text-red-600 dark:text-red-400">{err}</p>}
              <div className="flex justify-end gap-2 pt-1">
                <button
                  type="button"
                  onClick={() => setOpen(false)}
                  disabled={busy}
                  className="rounded-lg border border-neutral-300 px-3 py-1.5 text-sm dark:border-zinc-600"
                >
                  {t('forms.cancel')}
                </button>
                <button
                  type="button"
                  onClick={() => void assign()}
                  disabled={busy || !workerId || agentLimitReached}
                  className="rounded-lg bg-sky-600 px-3 py-1.5 text-sm font-medium text-white disabled:opacity-50"
                >
                  {busy ? t('members.adding') : t('members.add')}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </>
  )
}
