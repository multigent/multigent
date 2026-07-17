import { Link, NavLink, useLocation, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useEffect, useRef, useState } from 'react'
import { Check, ChevronDown, ChevronRight, Loader2, PanelLeftClose, PanelLeft, Plus, Settings } from 'lucide-react'
import {
  projectIdFromPath,
  projectSubNav,
  workspaceNav,
  isNavActive,
} from './nav-config'
import { cn } from '../../lib/cn'
import { useWorkspaceAccess } from '../../lib/workspace-access'
import { apiFetch, apiPost } from '../../lib/api'

type WorkspaceSummary = {
  id: string
  name: string
  description?: string
  teams: number
  projects: number
  agents: number
  tasks: number
}

type WorkspaceRef = {
  id: string
  name: string
  description?: string
  active?: boolean
}

const linkBase =
  'group relative flex items-center gap-2.5 rounded-lg px-2.5 py-[7px] text-[13px] font-medium transition-all duration-150 outline-none select-none'
const linkIdle =
  'text-neutral-500 hover:bg-neutral-100 hover:text-neutral-900 dark:text-zinc-400 dark:hover:bg-zinc-800/70 dark:hover:text-zinc-100'
const linkActive =
  'bg-neutral-100 text-neutral-900 dark:bg-zinc-800/70 dark:text-zinc-100'

const linkCollapsed =
  'group relative flex items-center justify-center rounded-md p-2 transition-all duration-150 outline-none select-none'

const subLinkBase =
  'group relative flex items-center gap-2 rounded-lg py-[5px] pl-2.5 pr-2 text-[12.5px] font-medium transition-all duration-150 outline-none select-none'
const subLinkIdle =
  'text-neutral-500 hover:bg-neutral-100 hover:text-neutral-800 dark:text-zinc-500 dark:hover:bg-zinc-800/70 dark:hover:text-zinc-200'
const subActive =
  'bg-neutral-100 text-neutral-900 dark:bg-zinc-800/70 dark:text-zinc-100'

export function Sidebar({ collapsed, onToggle }: { collapsed: boolean; onToggle: () => void }) {
  const { t } = useTranslation()
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const { canAdmin } = useWorkspaceAccess()
  const projectId = projectIdFromPath(pathname)
  const [workspace, setWorkspace] = useState<WorkspaceSummary | null>(null)
  const [workspaces, setWorkspaces] = useState<WorkspaceRef[]>([])
  const [workspaceMenuOpen, setWorkspaceMenuOpen] = useState(false)
  const [creatingWorkspace, setCreatingWorkspace] = useState(false)
  const [newWorkspaceName, setNewWorkspaceName] = useState('')
  const [switchingId, setSwitchingId] = useState<string | null>(null)
  const workspaceMenuRef = useRef<HTMLDivElement | null>(null)

  function refreshWorkspaceData() {
    let cancelled = false
    apiFetch<WorkspaceSummary>('/api/v1/workspace')
      .then(data => { if (!cancelled) setWorkspace(data) })
      .catch(() => { if (!cancelled) setWorkspace(null) })
    apiFetch<{ workspaces: WorkspaceRef[] }>('/api/v1/workspaces')
      .then(data => { if (!cancelled) setWorkspaces(data.workspaces ?? []) })
      .catch(() => { if (!cancelled) setWorkspaces([]) })
    return () => { cancelled = true }
  }

  useEffect(() => {
    return refreshWorkspaceData()
  }, [])

  useEffect(() => {
    setWorkspaceMenuOpen(false)
  }, [pathname])

  useEffect(() => {
    function onPointerDown(e: PointerEvent) {
      if (!workspaceMenuRef.current?.contains(e.target as Node)) {
        setWorkspaceMenuOpen(false)
      }
    }
    document.addEventListener('pointerdown', onPointerDown)
    return () => document.removeEventListener('pointerdown', onPointerDown)
  }, [])

  const workspaceName = workspace?.name ?? 'Multigent'
  const workspaceInitial = workspaceName.trim().charAt(0).toUpperCase() || 'M'

  async function switchWorkspace(id: string) {
    setSwitchingId(id)
    try {
      await apiPost(`/api/v1/workspaces/${encodeURIComponent(id)}/switch`, {})
      refreshWorkspaceData()
      setWorkspaceMenuOpen(false)
      window.dispatchEvent(new Event('workspace-changed'))
      navigate('/')
    } finally {
      setSwitchingId(null)
    }
  }

  async function createWorkspace() {
    const name = newWorkspaceName.trim()
    if (!name) return
    setCreatingWorkspace(true)
    try {
      await apiPost('/api/v1/workspaces', { name, switch: true })
      setNewWorkspaceName('')
      refreshWorkspaceData()
      setWorkspaceMenuOpen(false)
      window.dispatchEvent(new Event('workspace-changed'))
      navigate('/')
    } finally {
      setCreatingWorkspace(false)
    }
  }

  return (
    <aside
      className={cn(
        'flex shrink-0 flex-col border-r border-neutral-200/80 bg-white transition-[width] duration-200 dark:border-zinc-700/60 dark:bg-zinc-900',
        collapsed ? 'w-[3.5rem]' : 'w-[14.5rem]',
      )}
    >
      <div ref={workspaceMenuRef} className="relative flex h-11 items-center border-b border-neutral-200/80 px-2 dark:border-zinc-700/60">
        <button
          type="button"
          onClick={() => setWorkspaceMenuOpen(v => !v)}
          className={cn(
            'group flex h-7 w-full items-center rounded-md text-left transition-colors hover:bg-neutral-100 dark:hover:bg-zinc-800/70',
            collapsed ? 'justify-center px-1' : 'gap-2 px-2',
            workspaceMenuOpen && 'bg-neutral-100 dark:bg-zinc-800/70',
          )}
          title={workspaceName}
        >
          <span className="flex size-6 shrink-0 items-center justify-center rounded-md border border-neutral-200 bg-white text-[12px] font-semibold text-sky-700 dark:border-zinc-700 dark:bg-zinc-900 dark:text-sky-300">
            {workspaceInitial}
          </span>
          {!collapsed && (
            <>
              <span className="min-w-0 flex-1 truncate text-[13px] font-semibold text-neutral-900 dark:text-zinc-100">{workspaceName}</span>
              <ChevronDown className={cn('size-3.5 shrink-0 text-neutral-400 transition-transform dark:text-zinc-500', workspaceMenuOpen && 'rotate-180')} strokeWidth={1.8} />
            </>
          )}
        </button>

        {workspaceMenuOpen && (
          <div className={cn(
            'absolute top-12 z-30 w-[19rem] overflow-hidden rounded-lg border border-neutral-200 bg-white shadow-xl dark:border-zinc-700 dark:bg-zinc-900',
            collapsed ? 'left-2' : 'left-2',
          )}>
            <div className="max-h-72 overflow-y-auto border-b border-neutral-200/70 p-2 dark:border-zinc-700/60">
              <p className="px-2 pb-1.5 text-xs font-medium text-neutral-400 dark:text-zinc-500">{t('workspace.currentWorkspace')}</p>
              <div className="space-y-1">
                {(workspaces.length > 0 ? workspaces : [{ id: workspace?.id ?? 'current', name: workspaceName, active: true }]).map((item) => {
                  const initial = item.name.trim().charAt(0).toUpperCase() || 'M'
                  return (
                    <button
                      key={item.id}
                      type="button"
                      disabled={item.active || switchingId !== null}
                      onClick={() => switchWorkspace(item.id)}
                      className={cn(
                        'flex w-full items-center gap-2 rounded-md px-2 py-2 text-left transition-colors',
                        item.active
                          ? 'bg-neutral-50 dark:bg-zinc-800/60'
                          : 'hover:bg-neutral-100 dark:hover:bg-zinc-800/70',
                      )}
                    >
                      <span className="flex size-7 shrink-0 items-center justify-center rounded-md bg-sky-100 text-[13px] font-semibold text-sky-700 dark:bg-sky-900/30 dark:text-sky-300">
                        {initial}
                      </span>
                      <div className="min-w-0 flex-1">
                        <p className="truncate text-sm font-semibold text-neutral-900 dark:text-zinc-100">{item.name}</p>
                        {item.description && <p className="truncate text-xs text-neutral-400 dark:text-zinc-500">{item.description}</p>}
                      </div>
                      {switchingId === item.id ? (
                        <Loader2 className="size-4 shrink-0 animate-spin text-neutral-400 dark:text-zinc-500" strokeWidth={1.8} />
                      ) : item.active ? (
                        <Check className="size-4 shrink-0 text-sky-600 dark:text-sky-400" strokeWidth={1.8} />
                      ) : null}
                    </button>
                  )
                })}
              </div>
            </div>

            <div className="p-2">
              <Link
                to="/workspace"
                className="flex items-center gap-2 rounded-md px-2 py-2 text-sm font-medium text-neutral-700 transition-colors hover:bg-neutral-100 dark:text-zinc-300 dark:hover:bg-zinc-800/70"
              >
                <Settings className="size-4 text-neutral-400 dark:text-zinc-500" strokeWidth={1.8} />
                {t('workspace.viewDetails')}
              </Link>
              <div className="mt-1 rounded-md border border-neutral-200 p-2 dark:border-zinc-700">
                <label className="text-xs font-medium text-neutral-500 dark:text-zinc-400">{t('workspace.newWorkspaceName')}</label>
                <div className="mt-1.5 flex gap-1.5">
                  <input
                    value={newWorkspaceName}
                    onChange={e => setNewWorkspaceName(e.target.value)}
                    onKeyDown={e => {
                      if (e.key === 'Enter') {
                        e.preventDefault()
                        createWorkspace()
                      }
                    }}
                    placeholder={t('workspace.name')}
                    className="min-w-0 flex-1 rounded-md border border-neutral-200 bg-white px-2 py-1.5 text-sm text-neutral-900 outline-none focus:border-sky-400 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-100"
                  />
                  <button
                    type="button"
                    disabled={creatingWorkspace || newWorkspaceName.trim() === ''}
                    onClick={createWorkspace}
                    className="inline-flex items-center justify-center rounded-md bg-sky-600 px-2.5 text-sm font-medium text-white transition-colors hover:bg-sky-700 disabled:cursor-not-allowed disabled:opacity-50"
                    title={t('workspace.createWorkspace')}
                  >
                    {creatingWorkspace ? <Loader2 className="size-4 animate-spin" strokeWidth={1.8} /> : <Plus className="size-4" strokeWidth={1.8} />}
                  </button>
                </div>
              </div>
            </div>

            <div className="border-t border-neutral-200/70 px-4 py-3 dark:border-zinc-700/60">
              <p className="text-xs leading-relaxed text-neutral-400 dark:text-zinc-500">{t('workspace.switchHint')}</p>
            </div>
          </div>
        )}
      </div>

      <nav
        className={cn('flex flex-1 flex-col gap-0.5 overflow-y-auto py-3', collapsed ? 'px-1.5' : 'px-2')}
        aria-label={t('aria.mainNavigation')}
      >
        {workspaceNav.filter(item => !item.adminOnly || canAdmin).map(({ to, navKey, icon: Icon, activePrefix }) => {
          const end = to === '/'
          let active = isNavActive(pathname, to, end, activePrefix)
          if (navKey === 'projects' && projectId) {
            active = true
          }
          const showProjectNest =
            !collapsed && navKey === 'projects' && projectId !== null && projectId !== ''

          if (collapsed) {
            return (
              <NavLink
                key={to}
                to={to}
                end={end}
                className={cn(
                  linkCollapsed,
                  active
                    ? 'bg-neutral-100 text-neutral-900 dark:bg-zinc-800/70 dark:text-zinc-100'
                    : 'text-neutral-500 hover:bg-neutral-100 hover:text-neutral-900 dark:text-zinc-500 dark:hover:bg-zinc-800/70 dark:hover:text-zinc-100',
                )}
                title={t(`nav.${navKey}`)}
              >
                <Icon className="size-4.5 shrink-0" strokeWidth={1.8} />
              </NavLink>
            )
          }

          return (
            <div key={to}>
              <NavLink
                to={to}
                end={end}
                className={cn(linkBase, active ? linkActive : linkIdle)}
              >
                <Icon className="size-4 shrink-0 opacity-80" strokeWidth={1.8} />
                <span className="flex-1">{t(`nav.${navKey}`)}</span>
                {showProjectNest && (
                  <ChevronRight className="size-3 opacity-40 transition-transform duration-150 rotate-90" strokeWidth={2} />
                )}
              </NavLink>

              {showProjectNest && (
                <div className="mt-0.5 ml-[18px] border-l border-neutral-200/70 pl-2.5 dark:border-zinc-700/60 animate-fade-in">
                  <p className="truncate px-2.5 py-1.5 text-[10.5px] font-semibold uppercase tracking-wider text-neutral-400 dark:text-zinc-500">
                    {projectId}
                  </p>
                  <div className="space-y-px">
                    {projectSubNav.filter(item => !item.adminOnly || canAdmin).map(({ segment, icon: SubIcon }) => {
                      const subTo = `/projects/${encodeURIComponent(projectId)}/${segment}`
                      const subActiveState =
                        pathname === subTo || pathname.startsWith(`${subTo}/`)
                      return (
                        <NavLink
                          key={segment}
                          to={subTo}
                          className={cn(
                            subLinkBase,
                            subActiveState ? subActive : subLinkIdle,
                          )}
                        >
                          <SubIcon className="size-3.5 shrink-0 opacity-75" strokeWidth={1.8} />
                          <span>{t(`projectNav.${segment}`)}</span>
                        </NavLink>
                      )
                    })}
                  </div>
                </div>
              )}
            </div>
          )
        })}
      </nav>

      {/* Collapse / expand toggle at bottom */}
      <div className={cn('flex h-10 shrink-0 items-center border-t border-neutral-200/60 dark:border-zinc-700/50', collapsed ? 'justify-center px-1.5' : 'px-2')}>
        <button
          type="button"
          onClick={onToggle}
          className={cn(
            'flex w-full items-center rounded-md transition-colors hover:bg-neutral-100 dark:hover:bg-zinc-800',
            collapsed
              ? 'justify-center p-1.5 text-neutral-400 dark:text-zinc-500'
              : 'gap-2.5 px-2.5 py-[5px] text-[13px] font-medium text-neutral-500 dark:text-zinc-500',
          )}
          title={collapsed ? t('sidebar.expand') : t('sidebar.collapse')}
        >
          {collapsed
            ? <PanelLeft className="size-4" strokeWidth={1.8} />
            : <>
                <PanelLeftClose className="size-4 shrink-0 opacity-80" strokeWidth={1.8} />
                <span>{t('sidebar.collapse')}</span>
              </>
          }
        </button>
      </div>
    </aside>
  )
}
