// Package api exposes a JSON HTTP API over an multigent workspace for the web UI and integrations.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/multigent/multigent/internal/builtins"
	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/interaction"
	"github.com/multigent/multigent/internal/store"
	"github.com/multigent/multigent/internal/taskstore"
	"github.com/multigent/multigent/internal/telemetry"
)

type contextKey string

const ctxUserKey contextKey = "auth-user"

// UpdateChecker returns latest version info. Set by the caller.
type UpdateChecker func() (latestVersion, releaseNotes string, hasUpdate bool, channel string, updateCommand string)

// DaemonStatusFunc returns daemon status as a JSON-friendly map.
type DaemonStatusFunc func() map[string]any

// execProcess tracks a running agent exec process for one-shot chat sessions.
type execProcess struct {
	cmd     *exec.Cmd
	started time.Time
}

type connectorDeviceAuthSession struct {
	Provider    string
	AppID       string
	AppSecret   string
	OwnerOpenID string
	Brand       string
	CreatedAt   time.Time
}

// Server serves JSON for one workspace root.
type Server struct {
	workspaceMu            sync.Mutex
	controlDB              controldb.Store
	root                   string
	apiKey                 string
	version                string
	st                     store.Store
	ts                     taskstore.Store
	users                  *UserStore
	sched                  *SchedulerManager
	schedulerDesiredMu     sync.Mutex
	triggers               *triggerManager
	okrStore               *store.OKRStore
	msStore                *store.MilestoneStore
	updateCheck            UpdateChecker
	daemonStatus           DaemonStatusFunc
	execMu                 sync.Mutex
	execProcs              map[string]*execProcess // key = "project/agent"
	interactions           *interaction.Manager
	connectionHealthCancel func()
	connectionHealthDone   chan struct{}
	connectorSetupMu       sync.Mutex
	connectorSetupSessions map[string]connectorDeviceAuthSession
	modelAuthMu            sync.Mutex
	modelAuthSessions      map[string]*modelAuthSession
}

// NewServer builds an API server for the given workspace root.
// If apiKey is non-empty, requests must send Authorization: Bearer <apiKey>.
func NewServer(root, apiKey string) *Server {
	controlDB, err := controldb.OpenDefault()
	if err != nil {
		log.Fatalf("control DB unavailable: %v", err)
	}
	root = normalizeServerWorkspaceRoot(root, controlDB)
	if serverHasAgency(root) {
		if err := builtins.EnsureSkills(root); err != nil {
			log.Printf("ensure builtin skills failed: %v", err)
		}
	}
	sched := newSchedulerManager(root)
	ts := taskstore.NewDB(root, controlDB)
	tm := newTriggerManager(root, sched.binPath, ts)
	tm.StartPoller()
	s := &Server{
		root:                   root,
		apiKey:                 strings.TrimSpace(apiKey),
		controlDB:              controlDB,
		st:                     store.NewDB(root, controlDB),
		ts:                     ts,
		users:                  newUserStore(controlDB),
		sched:                  sched,
		triggers:               tm,
		okrStore:               store.NewOKRStore(root),
		msStore:                store.NewMilestoneStore(root),
		execProcs:              make(map[string]*execProcess),
		interactions:           interaction.NewManager(),
		connectorSetupSessions: make(map[string]connectorDeviceAuthSession),
		modelAuthSessions:      make(map[string]*modelAuthSession),
	}
	go s.restoreDesiredSchedulers()
	s.startConnectionHealthChecker()
	return s
}

func normalizeServerWorkspaceRoot(root string, db controldb.Store) string {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	if serverHasAgency(absRoot) {
		return absRoot
	}
	if db == nil {
		return absRoot
	}
	rows, err := db.ListWorkspaces()
	if err != nil {
		log.Printf("list workspaces while normalizing root failed: %v", err)
		return absRoot
	}
	for _, row := range rows {
		if row.Root != "" && serverWorkspaceRootBelongsToDataRoot(absRoot, row.Root) && serverHasAgency(row.Root) {
			return row.Root
		}
	}
	return absRoot
}

func serverWorkspaceRootBelongsToDataRoot(dataRoot, root string) bool {
	absData, err := filepath.Abs(dataRoot)
	if err != nil {
		return false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	return filepath.Dir(absRoot) == absData && filepath.Base(absRoot) != ".multigent"
}

func serverHasAgency(root string) bool {
	_, err := os.Stat(filepath.Join(root, ".multigent", "agency.yaml"))
	return err == nil
}

// SetVersion sets the build version string exposed via /api/v1/health.
func (s *Server) SetVersion(v string) { s.version = v }

// SetUpdateChecker sets the function used to check for updates.
func (s *Server) SetUpdateChecker(fn UpdateChecker) { s.updateCheck = fn }

// SetDaemonStatus sets the function used to get daemon status.
func (s *Server) SetDaemonStatus(fn DaemonStatusFunc) { s.daemonStatus = fn }

// Shutdown stops all managed scheduler processes and the trigger poller.
func (s *Server) Shutdown() {
	if s.connectionHealthCancel != nil {
		s.connectionHealthCancel()
	}
	if s.connectionHealthDone != nil {
		<-s.connectionHealthDone
	}
	s.triggers.StopPoller()
	s.sched.Cleanup()
	_ = s.controlDB.Close()
}

// Handler returns the root HTTP handler (includes optional auth).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/workspace", s.handleWorkspace)
	mux.HandleFunc("PUT /api/v1/workspace", s.handlePutWorkspace)
	mux.HandleFunc("GET /api/v1/workspaces", s.handleListWorkspaces)
	mux.HandleFunc("POST /api/v1/workspaces", s.handleCreateWorkspace)
	mux.HandleFunc("POST /api/v1/workspaces/example", s.handleCreateExampleWorkspace)
	mux.HandleFunc("POST /api/v1/workspaces/{id}/switch", s.handleSwitchWorkspace)
	mux.HandleFunc("DELETE /api/v1/workspaces/{id}", s.handleDeleteWorkspace)
	mux.HandleFunc("GET /api/v1/audit/events", s.handleAuditEvents)
	mux.HandleFunc("GET /api/v1/connectors/providers", s.handleConnectorProviders)
	mux.HandleFunc("GET /api/v1/connectors/providers/{provider}", s.handleConnectorProvider)
	mux.HandleFunc("POST /api/v1/connectors/providers/{provider}/setup/begin", s.handleConnectorProviderSetupBegin)
	mux.HandleFunc("POST /api/v1/connectors/providers/{provider}/setup/poll", s.handleConnectorProviderSetupPoll)
	mux.HandleFunc("GET /api/v1/oauth/client-configs", s.handleListOAuthClientConfigs)
	mux.HandleFunc("PUT /api/v1/oauth/client-configs/{provider}", s.handleUpsertOAuthClientConfig)
	mux.HandleFunc("DELETE /api/v1/oauth/client-configs/{provider}", s.handleDeleteOAuthClientConfig)
	mux.HandleFunc("POST /api/v1/oauth/authorizations", s.handleStartOAuthAuthorization)
	mux.HandleFunc("GET /api/v1/oauth/callback", s.handleCompleteOAuthAuthorization)
	mux.HandleFunc("GET /api/v1/connections", s.handleListConnections)
	mux.HandleFunc("POST /api/v1/connections", s.handleCreateConnection)
	mux.HandleFunc("GET /api/v1/connections/{id}", s.handleGetConnection)
	mux.HandleFunc("PUT /api/v1/connections/{id}", s.handleUpdateConnection)
	mux.HandleFunc("DELETE /api/v1/connections/{id}", s.handleDeleteConnection)
	mux.HandleFunc("POST /api/v1/connections/{id}/test", s.handleTestConnection)
	mux.HandleFunc("POST /api/v1/connections/health-check", s.handleRunConnectionHealthChecks)
	mux.HandleFunc("GET /api/v1/connections/{id}/grants", s.handleListConnectionGrants)
	mux.HandleFunc("POST /api/v1/connections/{id}/grants", s.handleCreateConnectionGrant)
	mux.HandleFunc("DELETE /api/v1/connections/{id}/grants/{grantId}", s.handleDeleteConnectionGrant)
	mux.HandleFunc("GET /api/v1/agency", s.handleAgency)
	mux.HandleFunc("GET /api/v1/stats", s.handleStats)
	mux.HandleFunc("GET /api/v1/team-templates", s.handleTeamTemplates)
	mux.HandleFunc("POST /api/v1/team-templates/{id}/apply", s.handleApplyTeamTemplate)
	mux.HandleFunc("GET /api/v1/teams", s.handleTeams)
	mux.HandleFunc("POST /api/v1/teams", s.handleCreateTeam)
	mux.HandleFunc("DELETE /api/v1/teams/{team}/roles/{role}", s.handleDeleteRole)
	mux.HandleFunc("DELETE /api/v1/teams/{teamPath...}", s.handleDeleteTeam)
	mux.HandleFunc("GET /api/v1/teams/{teamPath...}", s.handleTeamDetail)
	mux.HandleFunc("GET /api/v1/projects", s.handleProjects)
	mux.HandleFunc("POST /api/v1/projects", s.handleCreateProject)
	mux.HandleFunc("DELETE /api/v1/projects/{name}", s.handleDeleteProject)
	mux.HandleFunc("GET /api/v1/rbac/model", s.handleRBACModel)
	mux.HandleFunc("GET /api/v1/sandbox/capabilities", s.handleSandboxCapabilities)
	mux.HandleFunc("POST /api/v1/projects/{name}/tasks", s.handlePostProjectTask)
	mux.HandleFunc("POST /api/v1/projects/{name}/tasks/from-template", s.handlePostProjectTaskFromTemplate)
	mux.HandleFunc("POST /api/v1/projects/{name}/agents/{agent}/crons/{cronId}/pause", s.handlePostCronPause)
	mux.HandleFunc("POST /api/v1/projects/{name}/agents/{agent}/crons/{cronId}/resume", s.handlePostCronResume)
	mux.HandleFunc("PUT /api/v1/projects/{name}/agents/{agent}/crons/{cronId}", s.handlePutCron)
	mux.HandleFunc("DELETE /api/v1/projects/{name}/agents/{agent}/crons/{cronId}", s.handleDeleteCron)
	mux.HandleFunc("POST /api/v1/projects/{name}/agents/{agent}/crons", s.handlePostCron)
	mux.HandleFunc("POST /api/v1/projects/{name}/agents/{agent}/heartbeat/pause", s.handlePostHeartbeatPause)
	mux.HandleFunc("POST /api/v1/projects/{name}/agents/{agent}/heartbeat/resume", s.handlePostHeartbeatResume)
	mux.HandleFunc("GET /api/v1/projects/{name}/agents/{agent}/heartbeat", s.handleGetHeartbeat)
	mux.HandleFunc("PATCH /api/v1/projects/{name}/agents/{agent}/heartbeat", s.handlePatchHeartbeat)
	mux.HandleFunc("GET /api/v1/projects/{name}/agents/{agent}/env", s.handleGetAgentEnv)
	mux.HandleFunc("POST /api/v1/projects/{name}/agents/{agent}/env", s.handleSetAgentEnv)
	mux.HandleFunc("DELETE /api/v1/projects/{name}/agents/{agent}/env", s.handleDeleteAgentEnv)
	mux.HandleFunc("GET /api/v1/projects/{name}/schedule", s.handleGetProjectSchedule)
	mux.HandleFunc("GET /api/v1/projects/{name}/agents/{agent}/live-log", s.handleAgentLiveLog)
	mux.HandleFunc("POST /api/v1/messages/delete", s.handlePostDeleteMessage)
	mux.HandleFunc("POST /api/v1/messages/mark-read", s.handlePostMarkMessageRead)
	mux.HandleFunc("POST /api/v1/messages/archive", s.handlePostArchiveMessage)
	mux.HandleFunc("POST /api/v1/messages/mark-all-read", s.handlePostMarkAllMessagesRead)
	mux.HandleFunc("POST /api/v1/messages", s.handlePostMessage)
	mux.HandleFunc("POST /api/v1/projects/{name}/messages/mark-all-read", s.handlePostProjectMarkAllMessagesRead)
	mux.HandleFunc("GET /api/v1/telemetry/summary", s.handleTelemetrySummary)
	mux.HandleFunc("GET /api/v1/telemetry/runs", s.handleTelemetryRuns)
	mux.HandleFunc("GET /api/v1/telemetry/log", s.handleTelemetryLog)
	mux.HandleFunc("POST /api/v1/tasks/cancel", s.handlePostCancelTask)
	mux.HandleFunc("POST /api/v1/tasks/archive", s.handlePostArchiveTask)
	mux.HandleFunc("PUT /api/v1/tasks/update", s.handlePutUpdateTask)
	mux.HandleFunc("POST /api/v1/tasks/delete", s.handlePostDeleteTask)
	mux.HandleFunc("GET /api/v1/tasks/{project}/{agent}/{taskId}/comments", s.handleGetComments)
	mux.HandleFunc("POST /api/v1/tasks/{project}/{agent}/{taskId}/comments", s.handlePostComment)
	mux.HandleFunc("DELETE /api/v1/tasks/{project}/{agent}/{taskId}/comments/{commentId}", s.handleDeleteComment)
	mux.HandleFunc("GET /api/v1/projects/{name}/tasks", s.handleProjectTasks)
	mux.HandleFunc("GET /api/v1/projects/{name}/task-templates", s.handleListProjectTaskTemplates)
	mux.HandleFunc("POST /api/v1/projects/{name}/task-templates", s.handleCreateProjectTaskTemplate)
	mux.HandleFunc("GET /api/v1/projects/{name}/messages", s.handleProjectMessages)
	mux.HandleFunc("GET /api/v1/projects/{name}/agents", s.handleProjectAgents)
	mux.HandleFunc("PATCH /api/v1/projects/{name}/agents/{agent}", s.handlePatchAgent)
	mux.HandleFunc("POST /api/v1/projects/{name}/agents/{agent}/runtime/token", s.handleIssueAgentRuntimeToken)
	mux.HandleFunc("GET /api/v1/projects/{name}/agents/{agent}/runtime/connections", s.handleAgentRuntimeConnections)
	mux.HandleFunc("POST /api/v1/projects/{name}/tool-bindings/install", s.handleInstallProjectToolBindings)
	mux.HandleFunc("GET /api/v1/projects/{name}/agents/{agent}/tool-bindings", s.handleListAgentToolBindings)
	mux.HandleFunc("POST /api/v1/projects/{name}/agents/{agent}/tool-bindings", s.handleUpsertAgentToolBinding)
	mux.HandleFunc("DELETE /api/v1/projects/{name}/agents/{agent}/tool-bindings/{bindingId}", s.handleDeleteAgentToolBinding)
	mux.HandleFunc("GET /api/v1/projects/{name}/agents/{agent}/context", s.handleGetAgentContext)
	mux.HandleFunc("GET /api/v1/projects/{name}/agents/{agent}/interactions/active", s.handleAgentInteractionStatus)
	mux.HandleFunc("GET /api/v1/projects/{name}/agents/{agent}/channels", s.handleAgentChannels)
	mux.HandleFunc("POST /api/v1/projects/{name}/agents/{agent}/channels/{provider}/setup/begin", s.handleAgentChannelSetupBegin)
	mux.HandleFunc("POST /api/v1/projects/{name}/agents/{agent}/channels/{provider}/setup/poll", s.handleAgentChannelSetupPoll)
	mux.HandleFunc("PUT /api/v1/projects/{name}/agents/{agent}/channels/{provider}/security", s.handleAgentChannelSecurity)
	mux.HandleFunc("DELETE /api/v1/projects/{name}/agents/{agent}/channels/{provider}", s.handleAgentChannelDelete)
	mux.HandleFunc("GET /api/v1/projects/{name}/agents/{agent}/chat/history", s.handleAgentChatHistory)
	mux.HandleFunc("POST /api/v1/projects/{name}/agents/{agent}/chat", s.handleAgentChat)
	mux.HandleFunc("DELETE /api/v1/projects/{name}/agents/{agent}/chat", s.handleAgentChatStop)
	mux.HandleFunc("POST /api/v1/projects/{name}/agents/{agent}/set-model", s.handleSetModel)
	mux.HandleFunc("PUT /api/v1/projects/{name}/agents/{agent}/env", s.handlePutAgentEnv)
	mux.HandleFunc("PUT /api/v1/projects/{name}/agents/{agent}/wakeup", s.handlePutAgentWakeup)
	mux.HandleFunc("PUT /api/v1/projects/{name}/agents/{agent}/sandbox", s.handlePutAgentSandbox)
	mux.HandleFunc("POST /api/v1/projects/{name}/sync", s.handlePostProjectSync)
	mux.HandleFunc("GET /api/v1/projects/{name}/prompt", s.handleGetProjectPrompt)
	mux.HandleFunc("PUT /api/v1/projects/{name}/prompt", s.handlePutProjectPrompt)
	mux.HandleFunc("GET /api/v1/projects/{name}", s.handleProject)
	mux.HandleFunc("PUT /api/v1/projects/{name}", s.handlePutProject)
	mux.HandleFunc("GET /api/v1/workflows", s.handleListWorkflows)
	mux.HandleFunc("POST /api/v1/workflows", s.handleCreateWorkflow)
	mux.HandleFunc("GET /api/v1/workflow-templates", s.handleListWorkflowTemplates)
	mux.HandleFunc("GET /api/v1/task-templates", s.handleListTaskTemplates)
	mux.HandleFunc("POST /api/v1/task-templates", s.handleCreateTaskTemplate)
	mux.HandleFunc("GET /api/v1/task-templates/{templateId}", s.handleGetTaskTemplate)
	mux.HandleFunc("PUT /api/v1/task-templates/{templateId}", s.handleUpdateTaskTemplate)
	mux.HandleFunc("DELETE /api/v1/task-templates/{templateId}", s.handleDeleteTaskTemplate)
	mux.HandleFunc("GET /api/v1/playbook-templates", s.handleListPlaybookTemplates)
	mux.HandleFunc("GET /api/v1/playbook-installs", s.handleListPlaybookInstalls)
	mux.HandleFunc("POST /api/v1/playbook-templates/{playbookId}/install", s.handleInstallPlaybookTemplate)
	mux.HandleFunc("GET /api/v1/playbook-templates/{playbookId}", s.handleGetPlaybookTemplate)
	mux.HandleFunc("GET /api/v1/workflows/{workflowId}", s.handleGetWorkflow)
	mux.HandleFunc("PUT /api/v1/workflows/{workflowId}", s.handleUpdateWorkflow)
	mux.HandleFunc("DELETE /api/v1/workflows/{workflowId}", s.handleDeleteWorkflow)
	mux.HandleFunc("GET /api/v1/projects/{name}/tasks/{taskId}/workflow", s.handleGetTaskWorkflow)
	mux.HandleFunc("POST /api/v1/projects/{name}/tasks/{taskId}/workflow/review", s.handlePostTaskWorkflowReview)
	mux.HandleFunc("GET /api/v1/prompts/agency", s.handleGetAgencyPrompt)
	mux.HandleFunc("PUT /api/v1/prompts/agency", s.handlePutAgencyPrompt)
	mux.HandleFunc("GET /api/v1/prompts/teams/{teamPath...}", s.handleGetTeamPrompt)
	mux.HandleFunc("PUT /api/v1/prompts/teams/{teamPath...}", s.handlePutTeamPrompt)
	mux.HandleFunc("GET /api/v1/prompts/roles", s.handleGetRolePrompt)
	mux.HandleFunc("PUT /api/v1/prompts/roles", s.handlePutRolePrompt)
	mux.HandleFunc("GET /api/v1/skills", s.handleListSkills)
	mux.HandleFunc("POST /api/v1/skills", s.handleCreateSkill)
	mux.HandleFunc("POST /api/v1/skills/install", s.handleInstallSkill)
	mux.HandleFunc("GET /api/v1/skill-registry", s.handleListSkillRegistry)
	mux.HandleFunc("GET /api/v1/skills/{name}", s.handleGetSkillDetail)
	mux.HandleFunc("PUT /api/v1/skills/{name}", s.handlePutSkillPrompt)
	mux.HandleFunc("GET /api/v1/skills/{name}/files", s.handleGetSkillFiles)
	mux.HandleFunc("POST /api/v1/roles/skills", s.handlePostRoleSkillBind)
	mux.HandleFunc("POST /api/v1/teams/skills", s.handlePostTeamSkillBind)
	mux.HandleFunc("POST /api/v1/roles/create", s.handleCreateRole)
	mux.HandleFunc("POST /api/v1/projects/{name}/hire", s.handleHireAgent)
	mux.HandleFunc("DELETE /api/v1/projects/{name}/agents/{agent}", s.handleFireAgent)
	mux.HandleFunc("POST /api/v1/run", s.handleRunAgent)
	mux.HandleFunc("POST /api/v1/session/reset", s.handleSessionReset)
	mux.HandleFunc("GET /api/v1/assistant/status", s.handleAssistantStatus)
	mux.HandleFunc("PUT /api/v1/assistant/settings", s.handleAssistantSettings)
	mux.HandleFunc("POST /api/v1/assistant/chat", s.handleAssistantChat)
	mux.HandleFunc("GET /api/v1/docs", s.handleDocsList)
	mux.HandleFunc("GET /api/v1/docs/tree", s.handleDocsTree)
	mux.HandleFunc("GET /api/v1/docs/query", s.handleDocsQuery)
	mux.HandleFunc("GET /api/v1/docs/lint", s.handleDocsLint)
	mux.HandleFunc("GET /api/v1/docs/{id}", s.handleDocsGet)
	mux.HandleFunc("POST /api/v1/docs", s.handleDocsAdd)
	mux.HandleFunc("PUT /api/v1/docs/{id}", s.handleDocsUpdate)
	mux.HandleFunc("GET /api/v1/docs/{id}/download", s.handleDocsDownload)
	mux.HandleFunc("DELETE /api/v1/docs/{id}", s.handleDocsDelete)
	mux.HandleFunc("GET /api/v1/docs/{id}/refs", s.handleDocsGetRefs)
	mux.HandleFunc("POST /api/v1/docs/{id}/refs", s.handleDocsAddRef)
	mux.HandleFunc("DELETE /api/v1/docs/{id}/refs/{refId}", s.handleDocsRemoveRef)
	mux.HandleFunc("GET /api/v1/workbench/messages", s.handleWorkbenchMessages)
	mux.HandleFunc("GET /api/v1/workbench/tasks", s.handleWorkbenchTasks)
	mux.HandleFunc("GET /api/v1/workbench/overview", s.handleWorkbenchOverview)
	mux.HandleFunc("GET /api/v1/scheduler/status", s.handleSchedulerStatus)
	mux.HandleFunc("POST /api/v1/scheduler/start", s.handleSchedulerStart)
	mux.HandleFunc("POST /api/v1/scheduler/stop", s.handleSchedulerStop)
	mux.HandleFunc("POST /api/v1/scheduler/wakeup", s.handleSchedulerWakeup)
	mux.HandleFunc("POST /api/v1/scheduler/abort", s.handleSchedulerAbort)
	mux.HandleFunc("GET /api/v1/inbox", s.handleInbox)
	mux.HandleFunc("GET /api/v1/auth/me", s.handleAuthMe)
	mux.HandleFunc("PUT /api/v1/auth/password", s.handleChangePassword)

	mux.HandleFunc("GET /api/v1/providers", s.handleListProviders)
	mux.HandleFunc("POST /api/v1/providers", s.handleAddProvider)
	mux.HandleFunc("GET /api/v1/model-catalog", s.handleModelCatalog)
	mux.HandleFunc("GET /api/v1/providers/cc-switch", s.handleListCCSwitchProviders)
	mux.HandleFunc("POST /api/v1/providers/cc-switch/import", s.handleImportCCSwitchProviders)
	mux.HandleFunc("POST /api/v1/providers/auth/codex/device/begin", s.handleCodexDeviceAuthBegin)
	mux.HandleFunc("POST /api/v1/providers/auth/codex/device/poll", s.handleCodexDeviceAuthPoll)
	mux.HandleFunc("POST /api/v1/providers/auth/{cli}/browser/begin", s.handleCLIBrowserAuthBegin)
	mux.HandleFunc("POST /api/v1/providers/auth/{cli}/browser/poll", s.handleCLIBrowserAuthPoll)
	mux.HandleFunc("POST /api/v1/providers/auth/{cli}/browser/code", s.handleCLIBrowserAuthCode)
	mux.HandleFunc("DELETE /api/v1/providers/auth/sessions/{sessionId}", s.handleModelAuthSessionCancel)
	mux.HandleFunc("PUT /api/v1/providers/{id}", s.handleUpdateProvider)
	mux.HandleFunc("DELETE /api/v1/providers/{id}", s.handleDeleteProvider)

	mux.HandleFunc("GET /api/v1/envvars", s.handleListEnvVars)
	mux.HandleFunc("POST /api/v1/envvars", s.handleCreateEnvVar)
	mux.HandleFunc("PUT /api/v1/envvars/{id}", s.handleUpdateEnvVar)
	mux.HandleFunc("DELETE /api/v1/envvars/{id}", s.handleDeleteEnvVar)

	mux.HandleFunc("GET /api/v1/users", s.handleListUsers)
	mux.HandleFunc("GET /api/v1/users/lookup", s.handleLookupUserByEmail)
	mux.HandleFunc("POST /api/v1/users", s.handleCreateUser)
	mux.HandleFunc("GET /api/v1/invitations", s.handleListInvitations)
	mux.HandleFunc("POST /api/v1/invitations", s.handleCreateInvitation)
	mux.HandleFunc("POST /api/v1/invitations/{token}/revoke", s.handleRevokeInvitation)
	mux.HandleFunc("GET /api/v1/users/{username}", s.handleGetUser)
	mux.HandleFunc("PUT /api/v1/users/{username}", s.handleUpdateUser)
	mux.HandleFunc("PUT /api/v1/users/{username}/workspace-role", s.handleUpdateWorkspaceMemberRole)
	mux.HandleFunc("DELETE /api/v1/users/{username}", s.handleDeleteUser)

	// ── Goals (OKR + Milestones) ──
	mux.HandleFunc("GET /api/v1/okrs", s.handleListOKRs)
	mux.HandleFunc("POST /api/v1/okrs", s.handleCreateOKR)
	mux.HandleFunc("GET /api/v1/okrs/{id}", s.handleGetOKR)
	mux.HandleFunc("PUT /api/v1/okrs/{id}", s.handleUpdateOKR)
	mux.HandleFunc("DELETE /api/v1/okrs/{id}", s.handleDeleteOKR)
	mux.HandleFunc("POST /api/v1/okrs/{id}/key-results", s.handleAddKR)
	mux.HandleFunc("PUT /api/v1/okrs/{id}/key-results/{krId}", s.handleUpdateKR)
	mux.HandleFunc("DELETE /api/v1/okrs/{id}/key-results/{krId}", s.handleDeleteKR)
	mux.HandleFunc("POST /api/v1/okrs/{id}/reviews", s.handleAddReviewNote)
	mux.HandleFunc("GET /api/v1/projects/{name}/milestones", s.handleListMilestones)
	mux.HandleFunc("POST /api/v1/projects/{name}/milestones", s.handleCreateMilestone)
	mux.HandleFunc("GET /api/v1/projects/{name}/milestones/{msId}", s.handleGetMilestone)
	mux.HandleFunc("PUT /api/v1/projects/{name}/milestones/{msId}", s.handleUpdateMilestone)
	mux.HandleFunc("DELETE /api/v1/projects/{name}/milestones/{msId}", s.handleDeleteMilestone)

	// ── Files ──
	mux.HandleFunc("GET /api/v1/files", s.handleListFiles)
	mux.HandleFunc("POST /api/v1/files/upload", s.handleUploadFile)
	mux.HandleFunc("POST /api/v1/files/mkdir", s.handleMkdir)
	mux.HandleFunc("POST /api/v1/files/move", s.handleMoveFile)
	mux.HandleFunc("GET /api/v1/files/content/{path...}", s.handleFileContent)
	mux.HandleFunc("DELETE /api/v1/files/{path...}", s.handleDeleteFile)

	mux.HandleFunc("GET /api/v1/check-update", s.handleCheckUpdate)
	mux.HandleFunc("GET /api/v1/daemon/status", s.handleDaemonStatus)
	mux.HandleFunc("GET /api/v1/auth/settings", s.handleAuthSettings)
	mux.HandleFunc("PUT /api/v1/auth/settings", s.handlePutAuthSettings)

	publicMux := http.NewServeMux()
	publicMux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	publicMux.HandleFunc("POST /api/v1/auth/register", s.handleRegister)
	publicMux.HandleFunc("GET /api/v1/auth/settings/public", s.handlePublicAuthSettings)
	publicMux.HandleFunc("GET /api/v1/invitations/{token}", s.handlePublicInvitation)
	publicMux.HandleFunc("POST /api/v1/invitations/{token}/accept", s.handleAcceptInvitation)
	publicMux.HandleFunc("POST /api/v1/invitations/{token}/reject", s.handleRejectInvitation)
	publicMux.HandleFunc("POST /api/v1/im/{provider}/events", s.handleIMEvent)
	publicMux.HandleFunc("GET /api/v1/health", s.handleHealth)
	runtimeMux := http.NewServeMux()
	runtimeMux.HandleFunc("GET /api/v1/runtime/connections", s.handleRuntimeConnections)
	runtimeMux.HandleFunc("GET /api/v1/runtime/tasks", s.handleRuntimeTasks)
	runtimeMux.HandleFunc("POST /api/v1/runtime/tasks", s.handleRuntimePostTask)
	runtimeMux.HandleFunc("GET /api/v1/runtime/task-templates", s.handleRuntimeTaskTemplates)
	runtimeMux.HandleFunc("POST /api/v1/runtime/tasks/from-template", s.handleRuntimePostTaskFromTemplate)
	runtimeMux.HandleFunc("GET /api/v1/runtime/tasks/{id}", s.handleRuntimeTask)
	runtimeMux.HandleFunc("PUT /api/v1/runtime/tasks/{id}", s.handleRuntimePutTask)
	runtimeMux.HandleFunc("POST /api/v1/runtime/tasks/{id}/complete", s.handleRuntimeTaskComplete)
	runtimeMux.HandleFunc("POST /api/v1/runtime/tasks/{id}/workflow/step/complete", s.handleRuntimeWorkflowStepComplete)
	runtimeMux.HandleFunc("POST /api/v1/runtime/tasks/{id}/confirm-request", s.handleRuntimeTaskConfirmRequest)
	runtimeMux.HandleFunc("GET /api/v1/runtime/tasks/{id}/workflow", s.handleRuntimeTaskWorkflow)
	runtimeMux.HandleFunc("GET /api/v1/runtime/messages", s.handleRuntimeMessages)
	runtimeMux.HandleFunc("POST /api/v1/runtime/messages", s.handleRuntimePostMessage)
	runtimeMux.HandleFunc("POST /api/v1/runtime/messages/{id}/reply", s.handleRuntimeReplyMessage)
	runtimeMux.HandleFunc("GET /api/v1/runtime/docs", s.handleRuntimeDocsList)
	runtimeMux.HandleFunc("POST /api/v1/runtime/docs", s.handleRuntimeDocsCreate)
	runtimeMux.HandleFunc("GET /api/v1/runtime/docs/{id}", s.handleRuntimeDocsGet)
	runtimeMux.HandleFunc("POST /api/v1/runtime/skills/publish", s.handleRuntimeSkillPublish)
	runtimeMux.HandleFunc("POST /api/v1/runtime/mcp", s.handleRuntimeMCPProxy)
	runtimeMux.HandleFunc("POST /api/v1/runtime/mcp/gateway", s.handleRuntimeMCPGateway)
	runtimeMux.HandleFunc("POST /api/v1/runtime/actions", s.handleRuntimeActionProxy)
	publicMux.Handle("/api/v1/runtime/", s.withRuntimeAgentAuth(runtimeMux))
	publicMux.Handle("/", s.withTokenAuth(mux))

	return withCORS(withJSONHeaders(publicMux))
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if reqHeaders := r.Header.Get("Access-Control-Request-Headers"); reqHeaders != "" {
			w.Header().Set("Access-Control-Allow-Headers", reqHeaders)
		} else {
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withJSONHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/download") && !strings.Contains(r.URL.Path, "/files/content/") {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withTokenAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var token string
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			token = strings.TrimPrefix(auth, "Bearer ")
		} else if t := r.URL.Query().Get("_token"); t != "" {
			token = t
		}
		if token == "" {
			s.jsonErrorCode(w, http.StatusUnauthorized, ErrCodeUnauthorized, "unauthorized")
			return
		}

		// Legacy: static API key
		if s.apiKey != "" && token == s.apiKey {
			ctx := context.WithValue(r.Context(), ctxUserKey, "apikey")
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		username, ok := s.users.ValidateToken(token)
		if !ok {
			s.jsonErrorCode(w, http.StatusUnauthorized, ErrCodeUnauthorized, "invalid or expired token")
			return
		}
		if u := s.users.GetUser(username); u != nil && u.Disabled {
			s.jsonErrorCode(w, http.StatusForbidden, ErrCodeForbidden, "account disabled")
			return
		}
		ctx := context.WithValue(r.Context(), ctxUserKey, username)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	v := s.version
	if v == "" {
		v = "dev"
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "version": v})
}

func (s *Server) handleCheckUpdate(w http.ResponseWriter, _ *http.Request) {
	result := map[string]any{
		"currentVersion": s.version,
		"hasUpdate":      false,
	}
	if s.updateCheck != nil {
		latest, notes, has, channel, command := s.updateCheck()
		result["hasUpdate"] = has
		if channel != "" {
			result["channel"] = channel
		}
		if command != "" {
			result["updateCommand"] = command
		}
		if latest != "" {
			result["latestVersion"] = latest
		}
		if notes != "" {
			result["releaseNotes"] = notes
		}
	}
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Server) handleDaemonStatus(w http.ResponseWriter, _ *http.Request) {
	if s.daemonStatus != nil {
		_ = json.NewEncoder(w).Encode(s.daemonStatus())
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"supported": false, "error": "daemon status not available"})
}

func (s *Server) handleAgency(w http.ResponseWriter, _ *http.Request) {
	a, err := s.st.Agency()
	if err != nil {
		if isNotFoundErr(err) {
			s.jsonErrorCode(w, http.StatusNotFound, ErrCodeNotFound, "agency not found")
			return
		}
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"name":        a.Name,
		"description": a.Description,
		"lang":        a.Lang,
		"createdBy":   a.CreatedBy,
		"createdAt":   a.CreatedAt,
		"updatedAt":   a.UpdatedAt,
	})
}

func (s *Server) handleStats(w http.ResponseWriter, _ *http.Request) {
	projects, err := s.st.ListProjects()
	if err != nil {
		s.serverError(w, err)
		return
	}
	var pending, inProgress int
	for _, p := range projects {
		agents, err := s.st.ListAgents(p.Name)
		if err != nil {
			continue
		}
		for _, ag := range agents {
			tasks, err := s.ts.ListTasks(p.Name, ag.Name)
			if err != nil {
				continue
			}
			for _, t := range tasks {
				switch t.Status {
				case entity.TaskStatusPending:
					pending++
				case entity.TaskStatusInProgress:
					inProgress++
				}
			}
		}
	}

	runsToday := 0
	db, err := telemetry.OpenReadOnly(s.root)
	if err == nil {
		defer db.Close()
		now := time.Now()
		loc := time.Local
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).UTC()
		end := now.UTC()
		rows, err := telemetry.ReadRuns(db, &start, &end, "")
		if err == nil {
			runsToday = len(rows)
		}
	} else if !errors.Is(err, telemetry.ErrNoDatabase) {
		log.Printf("api stats: telemetry: %v", err)
	}

	_ = json.NewEncoder(w).Encode(map[string]int{
		"pendingTasks":    pending,
		"inProgressTasks": inProgress,
		"runsToday":       runsToday,
	})
}

func (s *Server) handleTeams(w http.ResponseWriter, _ *http.Request) {
	entries, err := s.st.ListTeams()
	if err != nil {
		s.serverError(w, err)
		return
	}
	workspaceID, _ := s.currentWorkspaceID()
	provenance, _ := s.playbookProvenanceMap(workspaceID, "team")
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		if e.Team == nil {
			continue
		}
		var prov *entity.PlaybookObjectProvenance
		if p, ok := provenance[playbookProvenanceKey("", e.Path)]; ok {
			cp := p
			prov = &cp
		}
		out = append(out, map[string]any{
			"path":               e.Path,
			"name":               e.Team.Name,
			"description":        e.Team.Description,
			"owners":             e.Team.Owners,
			"defaultContextPack": e.Team.DefaultContextPack,
			"skills":             e.Team.Skills,
			"goals":              e.Team.Goals,
			"provenance":         prov,
		})
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) handleTeamDetail(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.PathValue("teamPath"), "/")
	if path == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "missing team path")
		return
	}
	t, err := s.st.Team(path)
	if err != nil {
		if isNotFoundErr(err) {
			s.jsonErrorCode(w, http.StatusNotFound, ErrCodeTeamNotFound, "team not found")
			return
		}
		s.serverError(w, err)
		return
	}
	roles, err := s.st.ListRoles(path)
	if err != nil {
		s.serverError(w, err)
		return
	}
	workspaceID, _ := s.currentWorkspaceID()
	roleProvenance, _ := s.playbookProvenanceMap(workspaceID, "role")
	sort.Slice(roles, func(i, j int) bool { return roles[i].Name < roles[j].Name })
	roleOut := make([]map[string]any, 0, len(roles))
	for _, re := range roles {
		if re.Role == nil {
			continue
		}
		sk := re.Role.Skills
		if sk == nil {
			sk = []string{}
		}
		var prov *entity.PlaybookObjectProvenance
		if p, ok := roleProvenance[playbookProvenanceKey(path, re.Name)]; ok {
			cp := p
			prov = &cp
		}
		roleOut = append(roleOut, map[string]any{
			"name":        re.Name,
			"description": re.Role.Description,
			"skills":      sk,
			"provenance":  prov,
		})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"path":               path,
		"name":               t.Name,
		"description":        t.Description,
		"owners":             t.Owners,
		"defaultContextPack": t.DefaultContextPack,
		"skills":             t.Skills,
		"goals":              t.Goals,
		"roles":              roleOut,
		"provenance":         s.playbookObjectProvenanceForRequest(r, "team", "", path),
	})
}

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.st.ListProjects()
	if err != nil {
		s.serverError(w, err)
		return
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].Name < projects[j].Name })
	cur := s.currentUser(r)
	out := make([]map[string]any, 0, len(projects))
	for _, p := range projects {
		if cur.Role != RoleAdmin && !s.canAdminCurrentWorkspace(r) {
			if _, ok := s.users.HasProjectAccess(cur.Username, p.Name); !ok {
				if !currentUserLinkedProject(cur, p.Name) {
					continue
				}
			}
		}
		out = append(out, map[string]any{
			"name":        p.Name,
			"description": p.Description,
			"repo":        p.Repo,
		})
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) checkProjectAccess(w http.ResponseWriter, r *http.Request, project string) bool {
	if !s.canAccessProject(r, project) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeProjectAccessRequired, "no access to this project")
		return false
	}
	return true
}

func (s *Server) handleProject(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !s.checkProjectAccess(w, r, name) {
		return
	}
	p, err := s.st.Project(name)
	if err != nil {
		if isNotFoundErr(err) {
			s.jsonErrorCode(w, http.StatusNotFound, ErrCodeProjectNotFound, "project not found")
			return
		}
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"name":        p.Name,
		"description": p.Description,
		"repo":        p.Repo,
	})
}

func (s *Server) handlePutProject(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !s.checkProjectManager(w, r, name) {
		return
	}
	p, err := s.st.Project(name)
	if err != nil {
		if isNotFoundErr(err) {
			s.jsonErrorCode(w, http.StatusNotFound, ErrCodeProjectNotFound, "project not found")
			return
		}
		s.serverError(w, err)
		return
	}
	var body struct {
		Description string `json:"description"`
		Repo        string `json:"repo"`
	}
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	p.Description = body.Description
	p.Repo = body.Repo
	if err := s.st.SaveProject(name, p); err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *Server) handleProjectAgents(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !s.checkProjectAccess(w, r, name) {
		return
	}
	if _, err := s.st.Project(name); err != nil {
		if isNotFoundErr(err) {
			s.jsonErrorCode(w, http.StatusNotFound, ErrCodeProjectNotFound, "project not found")
			return
		}
		s.serverError(w, err)
		return
	}
	agents, err := s.st.ListAgents(name)
	if err != nil {
		s.serverError(w, err)
		return
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].Name < agents[j].Name })
	out := make([]map[string]any, 0, len(agents))
	for _, a := range agents {
		if a.Meta == nil {
			continue
		}
		if !s.canAccessAgent(r, name, a.Name) {
			continue
		}
		out = append(out, map[string]any{
			"name":    a.Name,
			"model":   string(a.Meta.Model),
			"team":    a.Meta.Team,
			"project": a.Meta.Project,
			"hiredAt": a.Meta.HiredAt.UTC().Format(time.RFC3339Nano),
			"avatar":  a.Meta.Avatar,
		})
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) handlePatchAgent(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("name")
	agent := r.PathValue("agent")
	if !s.checkProjectManager(w, r, project) {
		return
	}
	meta, err := s.st.AgentMeta(project, agent)
	if err != nil {
		if isNotFoundErr(err) {
			s.jsonErrorCode(w, http.StatusNotFound, ErrCodeAgentNotFound, "agent not found")
			return
		}
		s.serverError(w, err)
		return
	}
	var body struct {
		Name   string `json:"name"`
		Avatar string `json:"avatar"`
	}
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	newName := strings.TrimSpace(body.Name)
	if newName == "" {
		newName = agent
	}
	if !validAgentName(newName) {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidAgentName, "invalid agent name")
		return
	}

	if newName != agent {
		hb, _ := s.ts.GetHeartbeat(project, agent)
		if hb != nil && hb.LastWakeupStatus == "running" && hb.PID > 0 && processAlive(hb.PID) {
			s.jsonError(w, http.StatusConflict, "cannot rename a running agent")
			return
		}
		oldDir := s.st.AgentDir(project, agent)
		newDir := s.st.AgentDir(project, newName)
		if _, err := os.Stat(newDir); err == nil {
			s.jsonErrorCode(w, http.StatusConflict, ErrCodeAgentAlreadyExists, "target agent already exists")
			return
		} else if !os.IsNotExist(err) {
			s.serverError(w, err)
			return
		}
		if err := os.Rename(oldDir, newDir); err != nil {
			s.serverError(w, err)
			return
		}
		meta.Name = newName
		meta.Project = project
		if err := s.st.SaveAgentMeta(project, newName, meta); err != nil {
			s.serverError(w, err)
			return
		}
		if cfg, err := s.ts.GetProjectConfig(project); err == nil && cfg != nil {
			changed := false
			for i := range cfg.Agents {
				if cfg.Agents[i].Name == agent {
					cfg.Agents[i].Name = newName
					changed = true
				}
			}
			if changed {
				_ = s.ts.SaveProjectConfig(project, cfg)
			}
		}
	}

	meta.Avatar = strings.TrimSpace(body.Avatar)
	if err := s.st.SaveAgentMeta(project, newName, meta); err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "name": newName, "avatar": meta.Avatar})
}

func validAgentName(name string) bool {
	if name == "" || strings.HasPrefix(name, ".") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

type taskRow struct {
	ID               string    `json:"id"`
	Project          string    `json:"project"`
	Agent            string    `json:"agent"`
	Title            string    `json:"title"`
	Type             string    `json:"type,omitempty"`
	Assignee         string    `json:"assignee,omitempty"`
	AssigneeLabel    string    `json:"assigneeLabel,omitempty"`
	Description      string    `json:"description,omitempty"`
	Prompt           string    `json:"prompt,omitempty"`
	Priority         int       `json:"priority"`
	Status           string    `json:"status"`
	StatusGroup      string    `json:"statusGroup"`
	Archived         bool      `json:"archived"`
	Summary          string    `json:"summary,omitempty"`
	Labels           []string  `json:"labels"`
	ParentID         string    `json:"parentId,omitempty"`
	Position         float64   `json:"position"`
	CreatedBy        string    `json:"createdBy,omitempty"`
	CreatedByLabel   string    `json:"createdByLabel,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
	StartedAt        string    `json:"startedAt,omitempty"`
	FinishedAt       string    `json:"finishedAt,omitempty"`
	DueDate          string    `json:"dueDate,omitempty"`
	EstimateDuration string    `json:"estimateDuration,omitempty"`
}

func taskToRow(t *entity.Task, project, agent string, archived bool) taskRow {
	labels := t.Labels
	if labels == nil {
		labels = []string{}
	}
	r := taskRow{
		ID: t.ID, Project: project, Agent: agent,
		Title: t.Title, Type: string(t.Type), Assignee: t.Assignee,
		Description: t.Description, Prompt: userVisibleTaskPrompt(t.Prompt),
		Priority: t.Priority, Status: string(t.Status),
		StatusGroup: string(t.Status.Group()),
		Archived:    archived, Summary: t.Summary,
		Labels: labels, ParentID: t.ParentID, Position: t.Position,
		CreatedBy: t.CreatedBy,
		CreatedAt: t.CreatedAt.UTC(), UpdatedAt: t.UpdatedAt.UTC(),
		EstimateDuration: t.EstimateDuration,
	}
	if t.StartedAt != nil {
		r.StartedAt = t.StartedAt.UTC().Format(time.RFC3339Nano)
	}
	if t.FinishedAt != nil {
		r.FinishedAt = t.FinishedAt.UTC().Format(time.RFC3339Nano)
	}
	if t.DueDate != nil {
		r.DueDate = t.DueDate.UTC().Format("2006-01-02")
	}
	return r
}

func (s *Server) taskToRow(t *entity.Task, project, agent string, archived bool) taskRow {
	row := taskToRow(t, project, agent, archived)
	row.AssigneeLabel = s.identityLabel(row.Assignee)
	row.CreatedByLabel = s.identityLabel(row.CreatedBy)
	return row
}

func (s *Server) identityLabel(identity string) string {
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return ""
	}
	if strings.Contains(identity, "/") {
		return identity
	}
	if identity == "human" {
		return "Human"
	}
	if s != nil && s.users != nil {
		if u := s.users.GetUser(identity); u != nil {
			if label := strings.TrimSpace(u.DisplayName); label != "" {
				return label
			}
			if email := strings.TrimSpace(u.Email); email != "" {
				return email
			}
		}
	}
	return identity
}

func userVisibleTaskPrompt(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	const marker = "Original task prompt:"
	for strings.Contains(prompt, "Continue this workflow task from the current active step.") {
		before, after, ok := strings.Cut(prompt, marker)
		_ = before
		if !ok {
			break
		}
		prompt = strings.TrimSpace(after)
	}
	return prompt
}

func (s *Server) handleProjectTasks(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !s.checkProjectAccess(w, r, name) {
		return
	}
	if _, err := s.st.Project(name); err != nil {
		if isNotFoundErr(err) {
			s.jsonError(w, http.StatusNotFound, "project not found")
			return
		}
		s.serverError(w, err)
		return
	}

	qStatus := r.URL.Query().Get("status")
	qAgent := r.URL.Query().Get("agent")
	qPriority := r.URL.Query().Get("priority")
	qLabel := r.URL.Query().Get("label")
	qParent := r.URL.Query().Get("parent_id")
	qGroup := r.URL.Query().Get("status_group") // backlog, active, done
	qScope := r.URL.Query().Get("scope")        // "active" (default), "archived", "all"
	if qScope == "" {
		qScope = "all"
	}

	agents, err := s.st.ListAgents(name)
	if err != nil {
		s.serverError(w, err)
		return
	}

	isWakeupTask := func(t *entity.Task) bool {
		return strings.HasPrefix(t.Title, "[wakeup]") || t.Type == "wakeup"
	}

	matchFilter := func(t *entity.Task) bool {
		if isWakeupTask(t) {
			return false
		}
		if qStatus != "" && string(t.Status) != qStatus {
			return false
		}
		if qPriority != "" && fmt.Sprintf("%d", t.Priority) != qPriority {
			return false
		}
		if qGroup != "" && string(t.Status.Group()) != qGroup {
			return false
		}
		if qLabel != "" {
			found := false
			for _, l := range t.Labels {
				if l == qLabel {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		if qParent != "" && t.ParentID != qParent {
			return false
		}
		return true
	}

	rows := make([]taskRow, 0)
	seenIDs := make(map[string]bool)
	matchesAssigneeFilter := func(t *entity.Task) bool {
		if qAgent == "" {
			return true
		}
		assignee := strings.TrimSpace(t.Assignee)
		if qAgent == "human" {
			return assignee == "human"
		}
		return assignee == qAgent || assignee == name+"/"+qAgent
	}
	for _, ag := range agents {
		if !s.canAccessAgent(r, name, ag.Name) {
			continue
		}
		if qScope == "active" || qScope == "all" {
			tasks, err := s.ts.ListTasks(name, ag.Name)
			if err == nil {
				for _, t := range tasks {
					if !matchFilter(t) || !matchesAssigneeFilter(t) || seenIDs[t.ID] {
						continue
					}
					seenIDs[t.ID] = true
					rows = append(rows, s.taskToRow(t, name, ag.Name, false))
				}
			}
		}
		if qScope == "archived" || qScope == "all" {
			archived, err := s.ts.ListArchivedTasks(name, ag.Name)
			if err == nil {
				for _, t := range archived {
					if !matchFilter(t) || !matchesAssigneeFilter(t) || seenIDs[t.ID] {
						continue
					}
					seenIDs[t.ID] = true
					rows = append(rows, s.taskToRow(t, name, ag.Name, true))
				}
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].UpdatedAt.Equal(rows[j].UpdatedAt) {
			return rows[i].ID < rows[j].ID
		}
		return rows[i].UpdatedAt.After(rows[j].UpdatedAt)
	})
	_ = json.NewEncoder(w).Encode(rows)
}

type msgRow struct {
	ID         string     `json:"id"`
	From       string     `json:"from"`
	To         string     `json:"to"`
	Subject    string     `json:"subject,omitempty"`
	Body       string     `json:"body"`
	SentAt     time.Time  `json:"sentAt"`
	ReadAt     *time.Time `json:"readAt,omitempty"`
	ArchivedAt *time.Time `json:"archivedAt,omitempty"`
	Mailbox    string     `json:"mailbox"`
}

func (s *Server) handleProjectMessages(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !s.checkProjectAccess(w, r, name) {
		return
	}
	if _, err := s.st.Project(name); err != nil {
		if isNotFoundErr(err) {
			s.jsonError(w, http.StatusNotFound, "project not found")
			return
		}
		s.serverError(w, err)
		return
	}
	agents, err := s.st.ListAgents(name)
	if err != nil {
		s.serverError(w, err)
		return
	}
	q := r.URL.Query()
	archivedMode := strings.TrimSpace(strings.ToLower(q.Get("archived")))
	if archivedMode == "" && (q.Get("includeArchived") == "1" || strings.EqualFold(q.Get("includeArchived"), "true")) {
		archivedMode = "all"
	}
	if archivedMode == "" {
		archivedMode = "no"
	}
	if archivedMode != "no" && archivedMode != "yes" && archivedMode != "all" {
		s.jsonError(w, http.StatusBadRequest, "archived must be no, yes, or all")
		return
	}
	readFilter := strings.TrimSpace(strings.ToLower(q.Get("read")))
	if readFilter == "" {
		readFilter = "all"
	}
	if readFilter != "all" && readFilter != "read" && readFilter != "unread" {
		s.jsonError(w, http.StatusBadRequest, "read must be all, read, or unread")
		return
	}
	fromQ := strings.TrimSpace(q.Get("from"))
	toQ := strings.TrimSpace(q.Get("to"))
	mailboxFilter := strings.TrimSpace(q.Get("mailbox"))
	if mailboxFilter != "" {
		if err := s.validateIdentity(mailboxFilter, "mailbox"); err != nil {
			s.jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		parts := strings.SplitN(mailboxFilter, "/", 2)
		if len(parts) != 2 || parts[0] != name || !s.agentExistsInProject(name, parts[1]) {
			s.jsonError(w, http.StatusBadRequest, "mailbox must be project/agent for an agent in this project")
			return
		}
		if !s.canAccessAgent(r, name, parts[1]) {
			s.jsonError(w, http.StatusForbidden, "agent access required")
			return
		}
	}

	rows := make([]msgRow, 0)
	seen := map[string]struct{}{}
	useAll := archivedMode == "all" || archivedMode == "yes"

	add := func(recipient string) {
		var msgs []*entity.Message
		var err error
		if useAll {
			msgs, err = s.ts.ListAllMessages(recipient)
		} else {
			msgs, err = s.ts.ListMessages(recipient)
		}
		if err != nil {
			return
		}
		for _, m := range msgs {
			if m == nil {
				continue
			}
			if !messagePassesFilters(m, archivedMode, readFilter, fromQ, toQ) {
				continue
			}
			if _, ok := seen[m.ID]; ok {
				continue
			}
			seen[m.ID] = struct{}{}
			sent := m.SentAt.UTC()
			var read *time.Time
			if m.ReadAt != nil {
				t := m.ReadAt.UTC()
				read = &t
			}
			var arch *time.Time
			if m.ArchivedAt != nil {
				t := m.ArchivedAt.UTC()
				arch = &t
			}
			rows = append(rows, msgRow{
				ID:         m.ID,
				From:       m.From,
				To:         m.To,
				Subject:    m.Subject,
				Body:       m.Body,
				SentAt:     sent,
				ReadAt:     read,
				ArchivedAt: arch,
				Mailbox:    recipient,
			})
		}
	}

	if mailboxFilter != "" {
		add(mailboxFilter)
	} else {
		for _, ag := range agents {
			if !s.canAccessAgent(r, name, ag.Name) {
				continue
			}
			add(name + "/" + ag.Name)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].SentAt.After(rows[j].SentAt)
	})
	_ = json.NewEncoder(w).Encode(rows)
}

func messagePassesFilters(m *entity.Message, archivedMode, readFilter, fromQ, toQ string) bool {
	switch archivedMode {
	case "no":
		if m.ArchivedAt != nil {
			return false
		}
	case "yes":
		if m.ArchivedAt == nil {
			return false
		}
	}
	switch readFilter {
	case "read":
		if m.ReadAt == nil {
			return false
		}
	case "unread":
		if m.ReadAt != nil {
			return false
		}
	}
	if fromQ != "" && !strings.Contains(strings.ToLower(m.From), strings.ToLower(fromQ)) {
		return false
	}
	if toQ != "" && !strings.Contains(strings.ToLower(m.To), strings.ToLower(toQ)) {
		return false
	}
	return true
}

func (s *Server) handleInbox(w http.ResponseWriter, _ *http.Request) {
	items, err := s.ts.ListInbox()
	if err != nil {
		s.serverError(w, err)
		return
	}
	if items == nil {
		items = []*entity.InboxItem{}
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		if it == nil {
			continue
		}
		out = append(out, map[string]any{
			"taskId":      it.TaskID,
			"project":     it.Project,
			"agent":       it.Agent,
			"title":       it.Title,
			"summary":     it.Summary,
			"recipient":   it.Recipient(),
			"actionHint":  it.ActionHint,
			"actionItems": it.ActionItems,
		})
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) serverError(w http.ResponseWriter, err error) {
	log.Printf("api: %v", err)
	s.writeAPIError(w, http.StatusInternalServerError, ErrCodeInternal, "internal error", nil)
}

func isNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	return strings.Contains(err.Error(), "not found")
}

// ResolveAPIKey returns the API key from flag or MULTIGENT_WEB_API_KEY.
func ResolveAPIKey(flag string) string {
	if strings.TrimSpace(flag) != "" {
		return strings.TrimSpace(flag)
	}
	return strings.TrimSpace(os.Getenv("MULTIGENT_WEB_API_KEY"))
}
