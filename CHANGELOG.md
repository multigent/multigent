# Changelog

## [v0.1.32] - 2026-08-19

### Added

- Agent collaboration channels now support interactive Feishu/Lark cards with multiple action buttons, optional comment input, status updates, and callback handling.
- Runtime agents can send interactive cards with `mga notify card send`, including fields, links, actions, handler hints, and task context.
- Card callbacks now wake the sending Agent as structured interaction events, allowing the Agent to decide the next step instead of hardcoding business behavior in the IM handler.
- Human workflow decisions can now be submitted by Agents only with a short-lived user delegation token through `mga workflow decision submit`.

### Improved

- Workflow decision submission now verifies both the Agent runtime token and the delegated user's authority, preventing Agents from operating human-review steps without user delegation.
- Feishu/Lark card completion updates now avoid dumping long Markdown into cards and show a concise structured result instead.
- Interactive card layout now avoids duplicate separators around input fields and action buttons.

### Tested

- Added coverage for interactive card sending/updating, card callback parsing, interaction request lifecycle, runtime workflow decision delegation, and delegation-token validation.
- Verified the end-to-end path: workflow human review → Feishu card → user click → Agent callback wakeup → delegated `mga workflow decision submit` → workflow completion.

## [v0.1.31] - 2026-08-19

### Added

- Agent collaboration channels now support explicit user identity binding, so one agent channel can notify multiple workspace users without mixing external IM identities.
- Agent collaboration channels can bind named group chats. Agents can discover these targets and send notifications with `mga notify send --to chat:<group-name>`.
- Runtime agents can send server-side human notifications through connected Feishu/Lark/Slack/Telegram/Discord channels, with internal inbox audit records.
- `mga notify send` and `multigent notify send` now support `--message-format markdown` for structured review requests, blockers, and status updates.

### Improved

- Agent channel details now show bound users and bound group chats, plus clearer binding guidance for connected IM channels.
- Runtime connection guidance now tells agents how to inspect collaboration channels and when to notify humans or group chats.
- Sandbox runs refresh the mounted `mga` runtime CLI from the current Multigent server, reducing stale runtime-command failures after upgrades.

### Tested

- Added coverage for per-agent user binding, named group-chat binding, user-target notification routing, chat-target notification routing, and markdown notification formatting.

## [v0.1.30] - 2026-08-16

### Improved

- Agent detail pages now load the large merged context lazily, making member detail views open faster and avoiding unnecessary runtime readiness checks during initial page load.
- Context building now follows symlinked skill package roots, so installed skills with bundled files or scripts are synced into agent sandboxes correctly.

## [v0.1.29] - 2026-08-15

### Fixed

- npm installer now prefers `curl`/`wget` for GitHub Release downloads and adds a timeout to the Node.js fallback downloader, preventing installs from hanging silently on slow or blocked HTTPS connections.

## [v0.1.28] - 2026-08-15

### Added

- External tools can now manage multiple named connections and grant a specific connection to each agent instead of sharing one workspace-wide credential.
- Workflow task template runs now propagate start-step inputs from template variables and prompt fields, including Chinese full-width separators.

### Improved

- Runtime node execution now keeps leases alive during long runs, injects the correct runtime API credentials for remote agents, and avoids leaking loopback proxy settings into sandboxed runs.
- Workflow follow views and task details now render structured markdown and document previews more cleanly for human-review decisions.
- Project schedule loading is faster because runtime readiness uses a lightweight path and telemetry is opened once per request.
- Local install and runtime guidance now prefer `0.0.0.0` when Docker or runtime nodes need to call back into the Multigent server.

### Fixed

- Workflow terminal routes can now be represented explicitly, and route mismatch errors are guarded so agents do not silently advance down the wrong path.
- Agent chat history can fall back to native Claude session JSONL logs when Multigent telemetry does not contain a rendered conversation.
- Runtime Docker callbacks and remote runtime-node runs can complete workflow steps without hitting host loopback connection failures.
- Agent tool grants now check the selected connection, preventing agents from using unassigned external-tool credentials.

## [v0.1.26] - 2026-08-10

### Fixed

- Workflow task handoff now keeps task assignee, queue state, active step actor, and follow-view status aligned when moving between agents and human review steps.
- Workflow actor bindings now prefer explicit step IDs before role fallback, so multiple steps sharing the same role can still route to different assigned agents.
- Workflow detail reconciliation no longer revives terminal or blocked tasks just because a user opens the workflow view.
- Docker sandbox runs now mount the workspace runtime tool cache, preventing repeated external CLI installs across agent runs.
- Runtime tool installation now uses bounded npm installs with quieter output and fails fast instead of hanging indefinitely.

### Tested

- Added coverage for step-specific workflow actor bindings and Docker runtime tool-cache mounting.
- Re-ran an end-to-end workflow through agent analysis, human review, implementation, QA, final review, and release handoff.

## [v0.1.25] - 2026-08-03

### Added

- Runtime nodes can now preserve chat sessions and expose runtime contacts so agents can message workspace users more reliably.
- Runtime messaging now returns recipient suggestions when a contact cannot be matched exactly.

### Improved

- SaaS proxy mode now hydrates user identity through an abstract identity provider boundary.
- SaaS runtime readiness messaging now focuses users on binding a runtime node instead of showing local Docker checks when hosted execution is disabled.
- Runtime node service controls and routing are more reliable for remote execution.

### Fixed

- Open-source/self-hosted settings no longer show the SaaS-only Plan and Usage panel.
- Invite email delivery prompts now reflect SMTP delivery instead of always showing the local-link fallback.
- SaaS workspace route handling now preserves bare workspace basenames and redirects logout back to the SaaS sign-up flow.

## [v0.1.24] - 2026-07-31

### Fixed

- Workflow branch editors now keep focus while typing, so editing parallel branch titles and settings no longer drops the cursor after each keypress.
- Workflow board node selection now renders from Multigent's local selection state, reducing stale React Flow selection races that could require a second click or briefly highlight multiple nodes.
- Workflow board node clicks now select on pointer press with click fallback, making rapid node-to-node switching more reliable.

## [v0.1.23] - 2026-07-31

### Added

- Workflow parallel stages now expand into an on-canvas subflow preview when selected, making branch structure visible without leaving the workflow board.

### Improved

- Workflow parallel-stage cards now keep branch-count metadata in the lower-right content area without covering node labels.
- Workflow board node and edge handles now align more predictably, with cleaner edge endpoints and less visual offset.
- Workflow board dragging now follows the pointer smoothly, supports edge/center alignment snapping, and lets selected nodes move with arrow keys.
- Parallel subflow preview panels can be dragged temporarily on the canvas without changing the saved workflow definition.
- Parallel branch settings in the inspector now have stronger visual grouping.

## [v0.1.22] - 2026-07-31

### Added

- Workflow human-review steps can now notify assigned users through configured collaboration channels.
- Runtime nodes now support configurable direct host execution for trusted local or remote machines.
- Workflow parallel stages now run each branch as a child subworkflow, allowing branches to evolve beyond a single agent step.
- `multigent workflow scaffold parallel` can generate or save a workflow with a parallel subworkflow stage from the CLI.

### Improved

- Parallel workflow branch editing now hides internal branch IDs and lets users choose system default subflows or existing workflow definitions from one selector.
- Workflow import/update now recursively normalizes and validates nested branch subflows.

### Fixed

- Workflow routing no longer uses substring matching for equality or set membership conditions.
- Runtime task confirmation is blocked for tasks attached to active workflows, so agents must use explicit workflow step routes.

## [v0.1.21] - 2026-07-28

### Fixed

- Workflow board node status badges now use localized labels instead of raw status keys.

## [v0.1.20] - 2026-07-28

### Fixed

- Workflow step statuses now localize in follow views, workflow boards, and task workflow panels.
- The npm package now exposes both `multigent` and `mga` command shims.

## [v0.1.19] - 2026-07-28

### Added

- Workflow task details and follow view now open knowledge-base docIDs in draggable floating preview windows.
- Document preview windows can be resized from the bottom-right corner, zoomed, and opened side by side for comparing multiple upstream artifacts.

### Improved

- Document preview loading now uses a structured skeleton state to avoid layout jumps while content loads.
- Workflow step completion now validates structured docID outputs so agents cannot advance with malformed knowledge-base references.

## [v0.1.18] - 2026-07-27

### Improved

- Improved workflow board edge creation with larger connection snapping radius and visible target-node feedback while dragging a connection.
- Improved selected workflow edge styling so users can clearly see which connection is selected without an overly strong highlight.
- Improved new workflow node placement so new nodes appear near the current canvas focus instead of being inserted behind the inspector.

## [v0.1.17] - 2026-07-27

### Fixed

- Switching browser sessions between different users now clears stale workspace selection and falls back to an accessible workspace instead of repeatedly returning 403.
- Workbench messages now include the current user's own mailbox, so messages sent directly to a real user such as `john` appear in that user's workbench.

## [v0.1.16] - 2026-07-27

### Added

- Runtime Nodes can now join a workspace and execute assigned agent runs from a trusted machine.
- Agent settings now support assigning a Runtime Node for remote or host-side execution.

### Changed

- Claude Code direct host execution now keeps `bypassPermissions` but requires the Runtime Node process to run as a non-root user.
- Docker sandbox execution keeps using `IS_SANDBOX=1` with Claude Code's sandbox bypass path, preserving the existing default Docker behavior.

### Fixed

- Workspace switching now updates the active workspace id before reloading workspace-scoped data, preventing the UI from snapping back to the previous workspace.
- Command palette background prefetch no longer leaks stale project-agent requests across workspace switches.

## [v0.1.15] - 2026-07-26

### Changed

- Example Workspace now uses friendlier demo agent names: `Lina`, `Mira`, and `Nora`.
- Product tour, install guide, built-in setup assistant skill, and getting-started docs now refer to the updated demo agents.
- Task follow view now shows cleaner live output handoff behavior between workflow steps.

### Fixed

- Task follow view no longer leaves the "Moving to the next step" handoff banner stuck after the workflow has already advanced.
- Task follow view better preserves the latest completed workflow step when opening a completed task.
- Knowledge-base doc IDs in workflow inputs and outputs now render as document titles when available.

## [v0.1.14] - 2026-07-26

### Fixed

- Agents can no longer be run or manually woken up before a model account is explicitly linked.
- Runtime readiness no longer treats host environment API keys as an implicit model account for SaaS-managed agents.

### Changed

- Project schedule responses now include lightweight runtime readiness so unavailable agents can be disabled before users click wakeup.
- Agent detail and schedule controls now show a clearer model-account-required state.

## [v0.1.13] - 2026-07-26

### Fixed

- Heartbeat cycles now defer idle wakeup routines after processing queued tasks, preventing a task run from being immediately followed by a redundant `[wakeup] routine` run.
- Example Workspace agents no longer burn extra tokens on idle routine runs right after completing workflow task steps.

## [v0.1.12] - 2026-07-26

### Added

- Model account presets now include MiniMax China endpoints for Codex and Claude Code.
- Saving a workspace model account can now apply it to matching unconfigured agents.
- Task details now include a direct start action for pending tasks assigned to agents.

### Changed

- Task start now uses an icon-style action consistent with the task detail header.
- Task start is disabled when the task is running, terminal, or assigned to a human member.

## [v0.1.11] - 2026-07-26

### Changed

- Example Workspace now seeds localized onboarding tasks and workflows based on the user's browser language.
- The built-in demo now uses a concrete new-teammate onboarding note instead of an abstract Hello World relay.
- Product tour and quickstart docs now explain how workflows, tasks, owners, and wakeups work together.

### Fixed

- Task detail modal no longer renders invalid nested paragraph/button markup.

## [v0.1.10] - 2026-07-26

### Added

- Knowledge base list and document detail pages now include a refresh action.

### Changed

- Knowledge base refresh actions use the shared `common.refresh` localization key.

## [v0.1.9] - 2026-07-26

### Fixed

- Switching workspaces no longer leaks stale project and agent data from the previous workspace into workspace-scoped pages.
- Workbench project/agent summaries now clear and reload when the active workspace changes.

## [v0.1.8] - 2026-07-26

### Changed

- README now leads with the Multigent awakening concept image and official website link.
- README screenshots now use the lighter task, workflow, project-member, and task-detail views for clearer product framing.
- Product copy now emphasizes human-agent collaboration instead of positioning Multigent as only an agent-team manager.
- README highlights RBAC and projects where people and agents collaborate as equal accountable participants.

## [v0.1.7] - 2026-07-25

### Added

- Agent chat pages now provide a selectable session history list instead of requiring users to paste session IDs manually.
- Session history entries show readable titles derived from task titles or the first user message in the run log.

### Fixed

- Page tabs now preserve query parameters, so returning to an agent chat tab keeps the selected `sessionId`.
- Web chat and Feishu/Lark channel conversations no longer reuse or overwrite heartbeat and scheduled-run sessions by default.
- Empty chat history requests no longer fall back to heartbeat sessions, keeping interactive chat state separate from automation state.

## [v0.1.6] - 2026-07-25

### Changed

- Web chat now uses independent interactive sessions instead of overwriting heartbeat or scheduled-run sessions.
- Assistant replies in Web chat render with a typewriter-style live update and expanded thinking sections.
- Runtime readiness checks stay quiet while passing and only block chat input when the sandbox is actually unavailable.
- Feishu/Lark channel replies now send a quick acknowledgement before running the agent.

### Fixed

- Starting a new chat now clears stale Claude/Codex/Cursor session state before sending the next message.
- Chat history no longer gets replaced by stale session loads while a new reply is streaming.
- IM channel replies now extract the clean assistant answer instead of returning raw execution logs.
- Chat error messages are clearer when a CLI session is expired or unrecoverable.

## [v0.1.4] - 2026-07-24

### Added

- Task details now support reassigning ownership to another project agent or human member.
- Reassigning a task to another agent moves it to that agent queue and fires the task trigger.

### Changed

- Browser tab titles now follow the current page instead of staying fixed as `Multigent`.
- Release workflow skips `runtime-base` image publishing when the runtime image sources are unchanged.
- Task assignee editing is now an inline text-to-select interaction in the task detail modal.

### Fixed

- Task assignee display alignment in task details.
- Task reassignment validates project scope and assignee identity before moving queues.

## [v0.1.3] - 2026-07-24

### Added

- Workspace role management and scoped project role assignment.
- Local agent skill import and cross-workspace skill visibility improvements.
- Runtime readiness checks that block agent execution until sandbox prerequisites are available.
- Regional runtime image mirror support and first-install benchmarking utilities.
- Product tour card dragging for the example workspace onboarding flow.

### Changed

- Settings now restrict administrative controls to workspace admins while keeping Skills readable for members and visitors.
- External tool configuration is restricted to workspace admins.
- Docker sandbox startup avoids Linux-only binary mounts on Windows and syncs the runtime CLI into containers.
- Runtime image footprint was reduced to improve first-run download time.
- Command palette search is scoped to the active workspace.

### Fixed

- Workspace-scoped role relationships now apply consistently across UI and API flows.
- Workspace resource permissions are enforced for workflows, files, goals, tools, settings, and related actions.
- Missing loading translations and noisy runtime prompts were cleaned up.
- Docker Desktop and agent CLI binary discovery on macOS were improved.

## [v0.1.1] - 2026-07-23

### Added

- Control-plane assistant settings backed by workspace model accounts.
- Assistant HTTP model invocation for OpenAI-compatible and Anthropic providers.
- Assistant status API so the web UI can show setup and permission state clearly.

### Changed

- Settings page now separates model accounts and the intelligent assistant account.
- The assistant no longer depends on local Claude/Codex/Gemini CLI sessions.

### Fixed

- Removed obsolete assistant permission/session plumbing from the API server.
- Added assistant API coverage for missing configuration and provider validation.

## [v0.5.1] - 2026-04-20

### Added

**Knowledge base document viewer improvements**
- Table of contents (TOC) navigation with scroll-spy and smooth scrolling
- Floating transparent TOC overlay on small screens (avoids content obstruction)
- Copy document relative path button

**File manager module**
- New file browser page for managing agency promotional materials (images, videos, etc.)
- Grid/list view toggle with media thumbnails and preview
- Drag-and-drop file upload, folder creation, and file deletion
- Image zoom viewer and video/audio preview with native controls
- Drag-and-drop file/folder move (including breadcrumb drop targets)
- Copy file path button in grid, list, and preview modal

**Cron job session management**
- Session scope setting for cron jobs: "new each run" or "persistent" (matching heartbeat behavior)
- Persistent session shows session ID with copy-command button for direct CLI resume
- Edit and create forms include session scope selector

**Documentation**
- Competitive analysis document (HiClaw + Molecule AI comparison)
- CubeSandbox vs Docker evaluation document

## [v0.5.0] - 2026-04-20

### Added

**Provider management**
- `multigent provider` CLI commands for managing API providers (add/list/remove/set-default)
- Setup guidance system: interactive first-run wizard for new workspaces

**Cron editing & execution**
- Web UI: inline cron editing (expression, prompt, enabled toggle)
- Fire pending crons during heartbeat sleep phase (no longer wait for next full wakeup cycle)

**Agent identity injection**
- Inject agent identity environment variables (`MULTIGENT_AGENT`, `MULTIGENT_PROJECT`, etc.) into every agent process

**Agent abort & sandbox config**
- Abort running agents from web UI and CLI (`multigent run abort`)
- Sandbox configuration panel in agent detail page (image, mounts, env)
- Workbench sort/filter improvements and skill deep-links on agent page

**Draggable page tabs**
- Top bar tabs are now draggable for custom ordering

### Fixed
- Mount `.claude.json` as read-write in Docker sandbox (was read-only, breaking session persistence)
- Remove inline error text flash on schedule page; default cursor style on tabs
- Use explicit PATH in Docker container instead of unexpanded `$PATH` variable
- Use `exec.Command` + stdin pipe instead of `bash -c` for Docker runs (fixes quoting issues)
- Shell-escape Docker args when using stdin prompt redirect
- Include process output tail in run error messages for better diagnostics

## [v0.4.1] - 2026-04-06

### Added

**Environment variable management**
- Workspace-level environment variables (envvars): global or agent-scoped, injected at runtime
- Resolution priority chain: workspace global → agent-scoped → API provider → per-agent env
- CLI commands: `multigent envvar add/list/remove` (alias: `ev`)
- CLI commands: `multigent agent set-env/unset-env/list-env` for per-agent variables
- Web Settings page: envvar CRUD with agent picker (project-grouped multi-select)
- Agent detail page: env panel with inline editing, sensitive value masking, and eye toggle

**Event-driven triggers**
- New scheduling mechanism: trigger agent wakeup on message received or task assigned
- Trigger configuration via CLI (`multigent agent set-trigger`) and web heartbeat editor
- Deduplicated trigger execution with configurable cooldown

**Workbench enhancements**
- Project schedule overview cards: agent count, running agents, scheduler status, task/message counts
- Start/stop individual project schedulers and "start all" button from workbench
- Task tab badge showing pending task count
- Running agents count displayed on project cards

**Knowledge base improvements**
- Document fullscreen mode: hide header/nav, centered content, ESC to exit
- Code block copy fix (no more `[object Object]`)
- Documents sorted by creation time
- i18n-aware date formatting

**Multi-level OKR**
- OKR hierarchy: global, project, team, and agent scopes with parent linking
- Scope tabs with project-level filtering
- Agent dropdown selector for agent-scoped OKRs
- KR target value display: `0/10000 (unit)` format

### Fixed
- Task duplication when status changed to completed/cancelled (missing archive call)
- Heartbeat edit modal overflowing viewport
- Scheduler showing "pending activation" instead of next-window time on inactive days
- Run detail page now shows failure reason; status column fully i18n'd
- Workbench overview colors unified (blue general, green for pending items only)

### Changed
- `secrets.yaml` → `envvars.yaml`; Secret type → EnvVar; API `/secrets` → `/envvars`
- CLI `secret` command → `envvar` (alias `ev`)

### Security
- Fix command injection in wakeup condition pipe chain validation

## [v0.4.0] - 2026-04-06

### Added

**Goal management (OKR & Milestones)**
- OKR system: Objectives with Key Results supporting number/percentage/boolean/currency metric types
- Milestone management: project-level milestones with completion criteria, task labels, and due dates
- OKR web dashboard with inline KR value editing, create/edit/delete modals, and description fields
- Milestone panel with create/edit modal, progress tracking, and i18n-aware date formatting
- CLI commands: `multigent okr list/create/update/delete` with `kr add/update` and `review` subcommands
- CLI commands: `multigent milestone list/create/show/update/delete`
- Agent context injection: active OKR and milestone summaries auto-injected into agent prompts
- Web AI assistant prompt updated with goal management awareness

**Multi-user support**
- People management page: create/edit/delete user accounts with username/password
- RBAC permission model design
- Person detail page with editable profile fields (email, avatar, phone, bio)
- Human hiring flow via web UI

**IM platform integration (cc-connect)**
- cc-connect API proxy: connect agents to Feishu/WeChat via QR code scanning
- Settings page: one-stop cc-connect configuration panel
- Agent detail page: IM connection panel for binding IM accounts per agent
- Explicit project creation wizard with auto-restart polling

**Task & workbench enhancements**
- Kanban board view: list/board toggle with drag-and-drop status changes
- Batch operations: bulk cancel/archive/delete tasks
- Workbench kanban: unified message/task kanban in workbench
- Fire/remove agent or human member from projects

**Scheduler & operations**
- Heartbeat session management with SessionID tracking
- Context usage statistics (token consumption per agent)
- Unified API provider management via web UI (key + base URL configuration)
- AI assistant interactive permissions: allow/deny/allow-all for tool calls
- Run records track actual API model and base URL used
- Graceful scheduler shutdown on Ctrl+C

### Fixed
- Claude thinking signature validation error auto-retry (backtick variant)
- Codex Docker sandbox seccomp permission (`bwrap` namespace creation)
- Knowledge base third-level directory navigation
- Scheduler `ActiveDays` configuration not being respected
- Workbench reply textarea hiding while typing
- cc-connect project name path encoding with URL-safe separators
- Dark mode select dropdown option styling across all pages
- React error #310 in workbench message detail modal

### Changed
- Page header buttons unified to outline style across OKR, milestone, people, and task pages
- Date display follows i18n locale conventions (Intl.DateTimeFormat)
- Workbench defaults to inbox tab; reply available from message detail modal
- Markdown rendering in message detail modal

## [v0.3.0] - 2026-04-03

### Added

**Knowledge base (docs)**
- `multigent docs add` — index documents by file path with virtual directory structure
- `multigent docs list / tree / show / update / move / remove / search` — full document management
- Web document viewer with Notion-style Markdown rendering, syntax highlighting, and YAML frontmatter stripping
- Collapsible sidebar with virtual directory tree navigation
- Document download via authenticated API endpoint
- URL deep-linking: access documents directly via `/docs/<index>/<slug>`

**AI assistant**
- Built-in AI assistant widget (floating, draggable, resizable) powered by Claude CLI
- Streaming chat with tool permission handling (`--allowedTools`)
- Pre-loaded multigent SKILL for guided operations

**Scheduler & heartbeat**
- Daemon service management (`multigent service install/start/stop/status/uninstall`)
- Version update checking with footer notification in web UI
- Heartbeat UX: wakeup presets (pending tasks / unread messages), live log viewer
- Wakeup auto-sync: editing wakeup prompt on web UI immediately regenerates CLAUDE.md
- Explicit wakeup trigger prompt directs agents to follow `wakeup.md` steps

**Cursor agent support**
- `--force --trust` flags for full sandbox permissions in headless mode
- Token usage parsing for Cursor's camelCase stream-json format (`inputTokens`/`outputTokens`)
- Conversation log viewer adapted for Cursor tool_call/thinking events

**Web UI polish**
- Dark mode contrast overhaul: layered backgrounds (zinc-950 content / zinc-900 chrome)
- Breadcrumb bar with brand indicator and improved typography
- Agent detail page restructured with section headers and info cards
- Context compression env vars configurable per agent (Claude Code autocompact)
- Formatted conversation log for schedule wakeup results (reuses run viewer)

### Fixed
- `task add` now requires explicit `--created-by` flag; rejects `<project>/human` format
- Wakeup prompt path corrected to `.multigent/context/wakeup.md` across all docs
- `inbox reply` default `from` field set to original recipient (was incorrectly `human`)
- AI assistant: YAML frontmatter in SKILL no longer passed as CLI argument
- AI assistant: position validation prevents widget disappearing off-screen
- Schedule page: agent column links to member detail page
- `sync --force` now always reports "synced" instead of misleading "skipped"
- Version compare strips git-describe suffixes for accurate footer display
- Dark mode text contrast improved globally (zinc-700→600→500 cascade)

### Changed
- Schedule heartbeat status label: "等待中" → "待激活" for clarity
- Heartbeat wakeup preconditions check all non-completed task statuses (not just pending)

## [v0.2.2] - 2026-03-30

### Fixed
- npm install: Gitee fallback download URL pointed to wrong repository name

## [v0.2.1] - 2026-03-30

### Added
- Workbench: sent messages view with direction filter (inbox / sent / all)
- Task completion summary field with notification to task creator
- Agent model switching (including http-agent) from the web UI
- Copy-to-clipboard resume command in schedule runtime session column
- Refresh buttons on all table/filter pages
- Multi-page tab bar in header for quick page switching

### Fixed
- Unread message badge not updating after processing messages
- Scheduler next wakeup time showing stale values outside active window
- Message dialog recipients only showing one project's agents
- `forms.save` i18n key not applied in locale files
- Runs page table cell alignment
- Task type labels missing i18n support

### Changed
- Rename "Agency Console" to "AgencyCli" across all locales
- Workbench tasks panel defaults to showing pending tasks
- Simplified tab titles to show only the last breadcrumb segment
- Refresh buttons styled consistently with filter buttons

## [v0.2.0] - 2026-03-29

### Added

**Web console (built-in)**
- Single-binary web console served by `multigent start` — no separate frontend deployment needed
- Frontend built with React + TypeScript + Tailwind CSS, embedded via `//go:embed`
- Workbench page: unified operator hub for messages and tasks with batch operations
- Full message management: send (multi-recipient), reply, filter (read/unread/archived/from), batch archive/delete
- Full task management: create, edit (status/priority/type), view detail with execution logs, batch cancel/archive/delete
- Schedule management: tabbed Heartbeat / Cron / Runtime views with CRUD operations
- Run management: filterable table with Markdown-rendered conversation logs
- Agent hiring and role creation from the web UI
- Project settings page for editing project prompts
- Skills page for viewing team and agent skills
- Manual agent wakeup and `multigent run` from the Workbench
- Session management: view session ID/scope, switch scope (cycle/task), reset session
- Scheduler start/stop control from the web UI
- Authentication: username/password login with JWT tokens, user settings page
- i18n: English, 简体中文, 繁體中文, 日本語
- Plane-inspired professional UI: responsive sidebar, card layouts, sticky table columns, global footer

**CLI enhancements**
- `multigent start` — unified command serving API + embedded web console on a single port
- `multigent run` — manually execute an agent with optional prompt or next pending task
- `multigent session reset` — clear agent session
- `--project` and `--agent` filters for `scheduler start`
- SQLite telemetry: persistent agent run data with `runs summary` and `runs agents` commands
- `agent set-model` — change agent model after hiring

**API**
- `POST /api/v1/run` — trigger agent execution with optional prompt
- `POST /api/v1/session/reset` — reset agent session
- `POST /api/v1/roles/create` — create new roles within teams
- `POST /api/v1/projects/{name}/hire` — hire new agents into projects
- `GET /api/v1/version` — dynamic version endpoint

**Build & release**
- Makefile: `web`, `web-install`, `web-dev` targets; `build` now embeds frontend automatically
- Cross-platform release archives embed the web console

### Changed

- Scheduler startup banner refactored with lipgloss for cleaner terminal rendering
- Scheduler table columns aligned with proper width handling
- `inbox send` now requires `--from` flag and validates identities

## [v0.1.1] - 2026-03-21

### Added

- `scheduler heartbeat pause <project>/<agent>` — temporarily halt heartbeat without removing config; scheduler stays alive
- `scheduler heartbeat resume <project>/<agent>` — resume a paused heartbeat
- `scheduler cron list <project>/<agent>` — list all crons with enabled status
- `scheduler cron pause <project>/<agent> <cron-id>` — disable a cron
- `scheduler cron resume <project>/<agent> <cron-id>` — re-enable a paused cron
- `scheduler cron delete <project>/<agent> <cron-id>` — remove a cron entirely
- `--model human` support for multiple human identities in inbox routing

### Fixed

- Scheduler `active_hours` timing: `waitDur` is now correctly capped to the remaining window so displayed "next at" times are accurate and the scheduler never schedules a wakeup outside the active window
- Scheduler now shows accurate "next at" time when the projected wake falls outside the active window (shows window closing time instead)
- Scheduler: moved `LastWakeup` assignment to after all checks so window-skip does not corrupt elapsed-time calculation for the next cycle
- Scheduler: fixed jitter being negated when multiple agents have wake times that all fall before the window opens on restart
- Sandbox: agent `AddDirs` are now correctly mounted into Docker containers (previously only the project-level `repo:` was checked, which was always empty when repos are defined per-agent in `AgentSpec.repos`)

### Changed

- `scheduler heartbeat configure` renamed from `scheduler heartbeat` (subcommands added); old usage still works via flags
- Scheduler startup log now shows which agents have `active_hours` windows configured

## [v0.1.0] - 2026-03-19

First public release of multigent.

### Added

**Context management**
- Agency / team / sub-team / role / project scaffolding with `create` commands
- Layered context merging: `agency → team chain → role → project`, auto-assembled at `hire` time
- Support for 8 agent runtimes: `claudecode`, `codex`, `gemini`, `cursor`, `qoder`, `opencode`, `iflow`, `generic-cli`
- Skills system: reusable capability definitions with bundled files and `{{SKILL_DIR}}` resolution
- `sync` command with SHA-256 change detection — only re-generates changed layers
- `hire` / `assign` / `fire` (soft + hard delete) agent lifecycle commands
- `--dir` global flag to operate on any workspace from anywhere

**Task automation**
- Per-agent task queues with 6-state lifecycle (`pending → in_progress → done_success / done_failed / awaiting_confirmation / cancelled`)
- Priority ordering: 0=critical, 1=high, 2=normal, 3=low
- `task add / list / show / cancel / retry / stop-all / tokens`
- `task confirm-request` — agent escalates to human inbox (non-blocking, task archived)
- `run` (queue-based) and `exec` (one-shot) execution modes

**Heartbeat scheduler**
- Non-overlapping wakeup loop per agent: drain queue → sleep → repeat
- `active_hours` and `active_days` scheduling windows
- Startup jitter: prevents thundering herd when scheduler restarts
- Renamed from `daemon` to `scheduler` (aliases: `sched`, `s`)

**Wakeup routines**
- `wakeup.md` per agent: runs as synthetic task when queue is empty
- Enables fully autonomous proactive agents (scan issues, review PRs, etc.)
- Unread inbox messages auto-injected at top of wakeup prompt

**Cron scheduling**
- `cron add / list / delete / enable / disable` with standard crontab syntax
- Crons enqueue tasks; picked up on next heartbeat wakeup

**Inbox: task confirmations**
- Human confirmation inbox: `inbox list / show / confirm / reject / comment / forward`
- `--to` flag on `task confirm-request` to route to another agent instead of human

**Inbox: async messaging**
- Non-blocking message delivery between any participants (human or agent)
- `inbox send` with group send support (`--to` flag repeatable)
- `inbox messages` with `--from`, `--all`, `--archived`, `--mark-read` filters
- `inbox reply` — threaded replies by message ID
- `inbox fwd` — forward messages to one or more recipients with optional `--note`
- Per-message status: `inbox read / archive / delete` (alias: `rm`)

**Project blueprints**
- Declarative `project.yaml` defining agents, heartbeats, crons, and playbooks
- `project apply` — one command to hire all agents + configure schedules + install playbooks
- `project show / blueprints` — inspect project configuration

**Agent playbooks**
- `agent-playbooks/` directory for wakeup routine templates
- `playbook:` field in project blueprint installs as `wakeup.md` on `project apply`
- Playbooks included in template archives

**Templates**
- `template pack` — bundle agency as shareable `.tar.gz` (teams, roles, skills, playbooks, blueprints)
- `template info` — inspect metadata (local file, directory, or HTTPS URL)
- `create agency --template` — bootstrap from local file, directory, or remote URL
- `template.json` metadata: name, version, author, email, description, keywords

**Docker sandbox**
- Isolated container execution per task
- Auto-mounts: agent dir, project repo, agency workspace, credentials, `multigent` binary
- API keys forwarded as environment variables
- Supports `claudecode` and `codex` sandbox images

**Dashboard**
- `multigent overview` (aliases: `status`, `stat`) — ANSI TUI showing agents, heartbeat status, teams, skills, inbox summary
- Correct East Asian wide-character column width handling
