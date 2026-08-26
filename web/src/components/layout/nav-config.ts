import {
  BarChart3,
  BookOpen,
  Bot,
  Briefcase,
  Cable,
  ClipboardList,
  Clock3,
  FolderKanban,
  FolderOpen,
  GitBranch,
  LibraryBig,
  LayoutDashboard,
  ListTodo,
  Milestone,
  MessageSquare,
  Puzzle,
  Settings,
  ShieldCheck,
  Target,
  User,
  Users,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'

/** 一级导航 i18n：`nav.*` */
export type NavKey =
  | 'overview'
  | 'workspace'
  | 'agents'
  | 'schedule'
  | 'runs'
  | 'teams'
  | 'projects'
  | 'workflows'
  | 'playbooks'
  | 'people'
  | 'workbench'
  | 'connections'
  | 'contextCenter'
  | 'audit'
  | 'goals'
  | 'skills'
  | 'docs'
  | 'files'
  | 'settings'

/** 项目内执行面：`projectNav.*` */
export type ProjectNavKey = 'tasks' | 'taskTemplates' | 'goals' | 'milestones' | 'messages' | 'members' | 'schedule' | 'runs' | 'settings'

export type NavItem = {
  to: string
  navKey: NavKey
  icon: LucideIcon
  /** 为 true 时，子路径也算激活（如 /projects/demo/tasks） */
  activePrefix?: string
  /** admin-only nav items are hidden for member users */
  adminOnly?: boolean
}

export type ProjectNavItem = {
  segment: ProjectNavKey
  icon: LucideIcon
  adminOnly?: boolean
}

/** 工作区一级导航（团队 = 编制，项目 = 干活入口） */
export const workspaceNav: NavItem[] = [
  { to: '/', navKey: 'overview', icon: LayoutDashboard, activePrefix: undefined, adminOnly: true },
  { to: '/workbench', navKey: 'workbench', icon: Briefcase, activePrefix: '/workbench' },
  {
    to: '/projects',
    navKey: 'projects',
    icon: FolderKanban,
    activePrefix: '/projects',
  },
  { to: '/agents', navKey: 'agents', icon: Bot, activePrefix: '/agents', adminOnly: true },
  { to: '/schedule', navKey: 'schedule', icon: Clock3, activePrefix: '/schedule', adminOnly: true },
  { to: '/runs', navKey: 'runs', icon: BarChart3, activePrefix: '/runs', adminOnly: true },
  { to: '/workflows', navKey: 'workflows', icon: GitBranch, activePrefix: '/workflows' },
  { to: '/playbooks', navKey: 'playbooks', icon: LibraryBig, activePrefix: '/playbooks', adminOnly: true },
  { to: '/goals', navKey: 'goals', icon: Target, activePrefix: '/goals' },
  { to: '/people', navKey: 'people', icon: User, activePrefix: '/people', adminOnly: true },
  { to: '/teams', navKey: 'teams', icon: Users, activePrefix: '/teams', adminOnly: true },
  { to: '/connections', navKey: 'connections', icon: Cable, activePrefix: '/connections' },
  { to: '/skills', navKey: 'skills', icon: Puzzle, activePrefix: '/skills' },
  { to: '/docs', navKey: 'docs', icon: BookOpen, activePrefix: '/docs' },
  { to: '/files', navKey: 'files', icon: FolderOpen, activePrefix: '/files' },
  { to: '/audit', navKey: 'audit', icon: ShieldCheck, activePrefix: '/audit', adminOnly: true },
  { to: '/settings', navKey: 'settings', icon: Settings, activePrefix: '/settings', adminOnly: true },
]

export const projectSubNav: ProjectNavItem[] = [
  { segment: 'tasks', icon: ListTodo },
  { segment: 'taskTemplates', icon: ClipboardList, adminOnly: true },
  { segment: 'goals', icon: Target },
  { segment: 'milestones', icon: Milestone },
  { segment: 'messages', icon: MessageSquare },
  { segment: 'members', icon: Users },
  { segment: 'runs', icon: BarChart3 },
  { segment: 'settings', icon: Settings, adminOnly: true },
]

export function navKeyFromPath(pathname: string): NavKey {
  if (pathname === '/') return 'overview'
  const seg = pathname.split('/').filter(Boolean)[0]
  const map: Record<string, NavKey> = {
    workspace: 'workspace',
    agents: 'agents',
    schedule: 'schedule',
    runs: 'runs',
    teams: 'teams',
    projects: 'projects',
    workflows: 'workflows',
    playbooks: 'playbooks',
    people: 'people',
    goals: 'goals',
    workbench: 'workbench',
    connections: 'connections',
    context: 'contextCenter',
    audit: 'audit',
    skills: 'skills',
    docs: 'docs',
    files: 'files',
    settings: 'settings',
  }
  return map[seg ?? ''] ?? 'overview'
}

/** `/projects/foo/tasks` → `foo` */
export function projectIdFromPath(pathname: string): string | null {
  const m = /^\/projects\/([^/]+)/.exec(pathname)
  if (!m || m[1] === '') return null
  return decodeURIComponent(m[1])
}

export function projectNavKeyFromPath(pathname: string): ProjectNavKey | null {
  const m =
    /^\/projects\/[^/]+\/(tasks|task-templates|goals|milestones|messages|members|schedule|runs|settings)(?:\/|$)/.exec(pathname)
  if (m?.[1] === 'task-templates') return 'taskTemplates'
  return (m?.[1] as ProjectNavKey) ?? null
}

export function isNavActive(
  pathname: string,
  to: string,
  end: boolean,
  activePrefix?: string,
): boolean {
  if (activePrefix) {
    return pathname === activePrefix || pathname.startsWith(`${activePrefix}/`)
  }
  if (end) return pathname === to
  return pathname === to || pathname.startsWith(`${to}/`)
}
