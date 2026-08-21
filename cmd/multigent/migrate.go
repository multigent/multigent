package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/store"
	"github.com/multigent/multigent/internal/taskstore"
	workflowstore "github.com/multigent/multigent/internal/workflow"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run one-way workspace migrations",
	}
	cmd.AddCommand(newMigrateAgentWorkerCmd())
	return cmd
}

type agentWorkerMigrationPlan struct {
	WorkspaceID string                          `json:"workspaceId"`
	Workspace   string                          `json:"workspace"`
	GeneratedAt string                          `json:"generatedAt"`
	BackupPath  string                          `json:"backupPath,omitempty"`
	Workers     []agentWorkerMigrationWorker    `json:"workers"`
	Humans      []agentWorkerMigrationHuman     `json:"humanMemberships,omitempty"`
	Warnings    []string                        `json:"warnings,omitempty"`
	Summary     agentWorkerMigrationPlanSummary `json:"summary"`
}

type agentWorkerMigrationPlanSummary struct {
	ProjectsScanned        int `json:"projectsScanned"`
	AgentsScanned          int `json:"agentsScanned"`
	WorkersPlanned         int `json:"workersPlanned"`
	Memberships            int `json:"memberships"`
	HumanMemberships       int `json:"humanMemberships"`
	ActiveTasksScanned     int `json:"activeTasksScanned"`
	ArchivedTasksScanned   int `json:"archivedTasksScanned"`
	TaskAssigneesToRewrite int `json:"taskAssigneesToRewrite"`
}

type agentWorkerMigrationWorker struct {
	ID              string                     `json:"id"`
	Name            string                     `json:"name"`
	DisplayName     string                     `json:"displayName"`
	Project         string                     `json:"project"`
	LegacyAgent     string                     `json:"legacyAgent"`
	Model           string                     `json:"model"`
	Role            string                     `json:"role,omitempty"`
	Team            string                     `json:"team,omitempty"`
	RuntimeNodeID   string                     `json:"runtimeNodeId,omitempty"`
	RuntimeMode     string                     `json:"runtimeMode,omitempty"`
	Provider        string                     `json:"provider,omitempty"`
	RuntimeModel    string                     `json:"runtimeModel,omitempty"`
	Schedule        *entity.HeartbeatConfig    `json:"schedule,omitempty"`
	Membership      agentWorkerMigrationMember `json:"membership"`
	ChannelBindings int                        `json:"channelBindings"`
	Tasks           agentWorkerMigrationTasks  `json:"tasks"`
}

type agentWorkerMigrationTasks struct {
	Active             int `json:"active"`
	Archived           int `json:"archived"`
	AssigneesToRewrite int `json:"assigneesToRewrite"`
}

type agentWorkerMigrationMember struct {
	ID               string  `json:"id"`
	ProjectID        string  `json:"projectId"`
	MemberType       string  `json:"memberType"`
	MemberID         string  `json:"memberId"`
	Role             string  `json:"role,omitempty"`
	Title            string  `json:"title,omitempty"`
	AutoPickTasks    bool    `json:"autoPickTasks"`
	AttentionEnabled bool    `json:"attentionEnabled"`
	PriorityWeight   float64 `json:"priorityWeight"`
}

type agentWorkerMigrationHuman struct {
	ID          string `json:"id"`
	ProjectID   string `json:"projectId"`
	LegacyAgent string `json:"legacyAgent"`
	MemberID    string `json:"memberId"`
	Role        string `json:"role,omitempty"`
	Title       string `json:"title,omitempty"`
}

func newMigrateAgentWorkerCmd() *cobra.Command {
	var apply bool
	var dryRun bool
	var reportPath string
	var backupPath string
	var skipBackup bool
	cmd := &cobra.Command{
		Use:   "agent-worker",
		Short: "Migrate project-scoped agents to 2.x Agent Workers",
		RunE: func(cmd *cobra.Command, args []string) error {
			if apply && dryRun {
				return fmt.Errorf("--apply and --dry-run cannot be used together")
			}
			root, err := resolveMigrationRoot()
			if err != nil {
				return err
			}
			var s store.Store = store.NewFS(root)
			var ts taskstore.Store = taskstore.New(root)
			workspaceID := legacyMigrationWorkspaceID(root)
			var db *controldb.SQLiteStore
			var channelDB interface {
				ListAgentChannelBindings(filter controldb.AgentChannelBindingFilter) ([]controldb.AgentChannelBinding, error)
			}
			if apply || migrationControlDBExists(root) {
				var err error
				db, workspaceID, err = openMigrationWorkspaceDB(root)
				if err != nil {
					if apply {
						return err
					}
					planWarning := fmt.Sprintf("control DB exists but could not be opened for dry-run: %v", err)
					plan, buildErr := buildAgentWorkerMigrationPlan(root, workspaceID, s, ts, channelDB)
					if buildErr != nil {
						return buildErr
					}
					plan.Warnings = append(plan.Warnings, planWarning)
					if reportPath != "" {
						if err := writeJSONFile(cleanReportPath(root, reportPath), plan); err != nil {
							return err
						}
					}
					return printJSON(plan)
				}
				defer db.Close()
				if apply {
					if err := ensureMigrationWorkspace(db, root, workspaceID); err != nil {
						return err
					}
				}
				s = store.NewDB(root, db)
				ts = taskstore.NewDB(root, db)
				channelDB = db
			}
			plan, err := buildAgentWorkerMigrationPlan(root, workspaceID, s, ts, channelDB)
			if err != nil {
				return err
			}
			if reportPath != "" {
				if err := writeJSONFile(cleanReportPath(root, reportPath), plan); err != nil {
					return err
				}
			}
			if apply {
				if !skipBackup {
					path := strings.TrimSpace(backupPath)
					if path == "" {
						path = defaultMigrationBackupPath(root)
					} else {
						path = cleanReportPath(root, path)
					}
					if err := createWorkspaceBackupArchive(root, path); err != nil {
						return fmt.Errorf("create migration backup: %w", err)
					}
					plan.BackupPath = path
				}
				if err := applyAgentWorkerMigrationPlan(db, ts, plan); err != nil {
					return err
				}
				if reportPath != "" {
					if err := writeJSONFile(cleanReportPath(root, reportPath), plan); err != nil {
						return err
					}
				}
			}
			return printJSON(plan)
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "apply the migration plan")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the migration plan without writing data")
	cmd.Flags().StringVar(&reportPath, "report", "", "write the migration report JSON to this path")
	cmd.Flags().StringVar(&backupPath, "backup", "", "backup archive path for --apply (default: sibling .tar.gz)")
	cmd.Flags().BoolVar(&skipBackup, "skip-backup", false, "skip automatic workspace backup before --apply")
	return cmd
}

func migrationControlDBExists(root string) bool {
	candidates := []string{}
	if dataDir := strings.TrimSpace(os.Getenv("MULTIGENT_CONTROL_DATA_DIR")); dataDir != "" {
		candidates = append(candidates, filepath.Join(dataDir, ".multigent", "multigent.db"))
	}
	if dataDir := strings.TrimSpace(os.Getenv("MULTIGENT_DATA_DIR")); dataDir != "" {
		candidates = append(candidates, filepath.Join(dataDir, ".multigent", "multigent.db"))
	}
	if absRoot, err := filepath.Abs(root); err == nil {
		candidates = append(candidates,
			filepath.Join(filepath.Dir(absRoot), ".multigent", "multigent.db"),
			filepath.Join(absRoot, ".multigent", "multigent.db"),
		)
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

func defaultMigrationBackupPath(root string) string {
	base := filepath.Base(filepath.Clean(root))
	if base == "." || base == string(filepath.Separator) || strings.TrimSpace(base) == "" {
		base = "workspace"
	}
	name := fmt.Sprintf("%s-agent-worker-backup-%s.tar.gz", base, time.Now().UTC().Format("20060102T150405Z"))
	return filepath.Join(filepath.Dir(filepath.Clean(root)), name)
}

func createWorkspaceBackupArchive(root, dest string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	dest, err = filepath.Abs(dest)
	if err != nil {
		return err
	}
	if strings.HasPrefix(dest, root+string(filepath.Separator)) || dest == root {
		return fmt.Errorf("backup archive must not be inside the workspace being backed up")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".tmp"
	_ = os.Remove(tmp)
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	defer func() {
		_ = tw.Close()
		_ = gz.Close()
	}()
	base := filepath.Base(root)
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		mode := info.Mode()
		if mode&os.ModeType != 0 && mode&os.ModeSymlink == 0 && !mode.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(filepath.Join(base, rel))
		if rel == "." {
			name = base
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = name
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			header.Linkname = target
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tw, src)
		closeErr := src.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	}); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		return err
	}
	ok = true
	return nil
}

func resolveMigrationRoot() (string, error) {
	if root, err := resolveRoot(); err == nil {
		return root, nil
	}
	if strings.TrimSpace(globalDir) == "" {
		return "", fmt.Errorf("not inside a multigent workspace")
	}
	root, err := filepath.Abs(globalDir)
	if err != nil {
		return "", fmt.Errorf("workspace: resolve path: %w", err)
	}
	root = filepath.Clean(root)
	if _, err := os.Stat(filepath.Join(root, ".agencycli", "agency.yaml")); err == nil {
		_ = os.Setenv("MULTIGENT_DATA_DIR", root)
		return root, nil
	}
	if _, err := os.Stat(filepath.Join(root, "projects")); err == nil {
		_ = os.Setenv("MULTIGENT_DATA_DIR", root)
		return root, nil
	}
	return "", fmt.Errorf("not inside a multigent or legacy agency workspace: %s", root)
}

func openMigrationWorkspaceDB(root string) (*controldb.SQLiteStore, string, error) {
	if strings.TrimSpace(os.Getenv("MULTIGENT_DATA_DIR")) == "" {
		setCLIDataDirForWorkspace(root)
	}
	db, err := controldb.OpenDefault()
	if err != nil {
		return nil, "", err
	}
	if id, err := workspaceIDForRoot(db, root); err != nil {
		_ = db.Close()
		return nil, "", err
	} else if id != "" {
		return db, id, nil
	}
	return db, legacyMigrationWorkspaceID(root), nil
}

func ensureMigrationWorkspace(db controldb.Store, root, workspaceID string) error {
	if _, ok, err := db.WorkspaceByID(workspaceID); err != nil {
		return err
	} else if ok {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	name := filepath.Base(root)
	if strings.TrimSpace(name) == "" || name == "." || name == string(filepath.Separator) {
		name = "Multigent Workspace"
	}
	return db.UpsertWorkspace(controldb.Workspace{
		ID:        workspaceID,
		Name:      name,
		Slug:      name,
		Root:      root,
		CreatedAt: now,
		UpdatedAt: now,
	})
}

func legacyMigrationWorkspaceID(root string) string {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	sum := sha1.Sum([]byte(filepath.Clean(absRoot)))
	return hex.EncodeToString(sum[:])[:12]
}

func buildAgentWorkerMigrationPlan(root, workspaceID string, s store.Store, ts interface {
	GetHeartbeat(project, agent string) (*entity.HeartbeatConfig, error)
	ListTasks(project, agent string, filter ...entity.TaskStatus) ([]*entity.Task, error)
	ListArchivedTasks(project, agent string) ([]*entity.Task, error)
}, db interface {
	ListAgentChannelBindings(filter controldb.AgentChannelBindingFilter) ([]controldb.AgentChannelBinding, error)
}) (agentWorkerMigrationPlan, error) {
	projects, err := s.ListProjects()
	if err != nil {
		return agentWorkerMigrationPlan{}, err
	}
	plan := agentWorkerMigrationPlan{
		WorkspaceID: workspaceID,
		Workspace:   root,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Workers:     []agentWorkerMigrationWorker{},
		Humans:      []agentWorkerMigrationHuman{},
		Warnings:    []string{},
	}
	seenWorkerNames := map[string]bool{}
	for _, project := range projects {
		if project == nil {
			continue
		}
		plan.Summary.ProjectsScanned++
		agents, err := s.ListAgents(project.Name)
		if err != nil {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("list agents for %s: %v", project.Name, err))
			continue
		}
		if len(agents) == 0 {
			legacyAgents, err := listLegacyMigrationAgents(root, project.Name)
			if err != nil {
				plan.Warnings = append(plan.Warnings, fmt.Sprintf("list legacy agents for %s: %v", project.Name, err))
			} else {
				agents = legacyAgents
			}
		}
		for _, agent := range agents {
			if agent == nil || agent.Meta == nil {
				continue
			}
			plan.Summary.AgentsScanned++
			if agent.Meta.Model == entity.ModelHuman {
				memberID := strings.TrimSpace(agent.Name)
				if memberID == "" {
					memberID = strings.TrimSpace(agent.Meta.Name)
				}
				if memberID == "" {
					plan.Warnings = append(plan.Warnings, fmt.Sprintf("skip human member with empty id in project %s", project.Name))
					continue
				}
				title := strings.TrimSpace(agent.Meta.Name)
				if title == "" {
					title = memberID
				}
				plan.Humans = append(plan.Humans, agentWorkerMigrationHuman{
					ID:          stableMigrationID("pmu", workspaceID, project.Name, memberID),
					ProjectID:   project.Name,
					LegacyAgent: agent.Name,
					MemberID:    memberID,
					Role:        agent.Meta.Role,
					Title:       title,
				})
				continue
			}
			workerName := uniqueWorkerName(project.Name, agent.Name, seenWorkerNames)
			workerID := stableMigrationID("aw", workspaceID, project.Name, agent.Name)
			membershipID := stableMigrationID("pm", workspaceID, project.Name, agent.Name)
			hb, err := ts.GetHeartbeat(project.Name, agent.Name)
			if err != nil {
				plan.Warnings = append(plan.Warnings, fmt.Sprintf("read heartbeat for %s/%s: %v", project.Name, agent.Name, err))
			}
			taskImpact, err := migrationTaskImpact(ts, project.Name, agent.Name)
			if err != nil {
				plan.Warnings = append(plan.Warnings, fmt.Sprintf("read tasks for %s/%s: %v", project.Name, agent.Name, err))
			}
			plan.Summary.ActiveTasksScanned += taskImpact.Active
			plan.Summary.ArchivedTasksScanned += taskImpact.Archived
			plan.Summary.TaskAssigneesToRewrite += taskImpact.AssigneesToRewrite
			channelBindings := 0
			if db != nil {
				channels, _ := db.ListAgentChannelBindings(controldb.AgentChannelBindingFilter{
					WorkspaceID: workspaceID,
					ProjectID:   project.Name,
					AgentID:     agent.Name,
				})
				channelBindings = len(channels)
			}
			autoPick := hb != nil && hb.Enabled
			plan.Workers = append(plan.Workers, agentWorkerMigrationWorker{
				ID:              workerID,
				Name:            workerName,
				DisplayName:     agent.Name,
				Project:         project.Name,
				LegacyAgent:     agent.Name,
				Model:           string(agent.Meta.Model),
				Role:            agent.Meta.Role,
				Team:            agent.Meta.Team,
				RuntimeNodeID:   agent.Meta.RuntimeNodeID,
				RuntimeMode:     agent.Meta.RuntimeMode,
				Provider:        agent.Meta.Provider,
				RuntimeModel:    agent.Meta.RuntimeModel,
				Schedule:        hb,
				ChannelBindings: channelBindings,
				Tasks:           taskImpact,
				Membership: agentWorkerMigrationMember{
					ID:               membershipID,
					ProjectID:        project.Name,
					MemberType:       "agent_worker",
					MemberID:         workerID,
					Role:             agent.Meta.Role,
					Title:            agent.Name,
					AutoPickTasks:    autoPick,
					AttentionEnabled: true,
					PriorityWeight:   1.0,
				},
			})
		}
	}
	plan.Summary.WorkersPlanned = len(plan.Workers)
	plan.Summary.HumanMemberships = len(plan.Humans)
	plan.Summary.Memberships = len(plan.Workers) + len(plan.Humans)
	return plan, nil
}

func migrationTaskImpact(ts interface {
	ListTasks(project, agent string, filter ...entity.TaskStatus) ([]*entity.Task, error)
	ListArchivedTasks(project, agent string) ([]*entity.Task, error)
}, project, agent string) (agentWorkerMigrationTasks, error) {
	active, err := ts.ListTasks(project, agent)
	if err != nil {
		return agentWorkerMigrationTasks{}, err
	}
	archived, err := ts.ListArchivedTasks(project, agent)
	if err != nil {
		return agentWorkerMigrationTasks{}, err
	}
	out := agentWorkerMigrationTasks{Active: len(active), Archived: len(archived)}
	for _, task := range append(active, archived...) {
		if task == nil {
			continue
		}
		if strings.TrimSpace(task.Assignee) == project+"/"+agent {
			out.AssigneesToRewrite++
		}
	}
	return out, nil
}

func loadMigrationTasks(root, project, agent, file string) ([]*entity.Task, error) {
	paths := []string{
		filepath.Join(root, "projects", project, "agents", agent, ".multigent", file),
		filepath.Join(root, "projects", project, "agents", agent, ".agencycli", file),
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var tasks []*entity.Task
		if err := yaml.Unmarshal(data, &tasks); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return tasks, nil
	}
	return nil, nil
}

func listLegacyMigrationAgents(root, project string) ([]*store.AgentEntry, error) {
	base := filepath.Join(root, "projects", project, "agents")
	entries, err := os.ReadDir(base)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	agents := make([]*store.AgentEntry, 0)
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		metaPath := filepath.Join(base, entry.Name(), ".agencycli", "agent.yaml")
		data, err := os.ReadFile(metaPath)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var meta entity.AgentMeta
		if err := yaml.Unmarshal(data, &meta); err != nil {
			return nil, fmt.Errorf("%s: %w", metaPath, err)
		}
		if meta.Name == "" {
			meta.Name = entry.Name()
		}
		if meta.Project == "" {
			meta.Project = project
		}
		agents = append(agents, &store.AgentEntry{Project: project, Name: entry.Name(), Meta: &meta})
	}
	return agents, nil
}

func applyAgentWorkerMigrationPlan(db controldb.Store, ts taskstore.Store, plan agentWorkerMigrationPlan) error {
	now := time.Now().UTC().Format(time.RFC3339)
	for _, human := range plan.Humans {
		staleWorkerID := stableMigrationID("aw", plan.WorkspaceID, human.ProjectID, human.LegacyAgent)
		staleMembershipID := stableMigrationID("pm", plan.WorkspaceID, human.ProjectID, human.LegacyAgent)
		_ = db.DeleteProjectMembership(plan.WorkspaceID, staleMembershipID)
		_ = db.DeleteAgentWorker(plan.WorkspaceID, staleWorkerID)
		membership := controldb.ProjectMembership{
			ID:               human.ID,
			WorkspaceID:      plan.WorkspaceID,
			ProjectID:        human.ProjectID,
			MemberType:       "user",
			MemberID:         human.MemberID,
			Role:             human.Role,
			Title:            human.Title,
			PermissionsJSON:  `["task.read","task.write","workflow.read","workflow.write","inbox.send"]`,
			AutoPickTasks:    false,
			AttentionEnabled: true,
			PriorityWeight:   1.0,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		if err := db.UpsertProjectMembership(membership); err != nil {
			return fmt.Errorf("upsert human project membership %s: %w", human.ID, err)
		}
	}
	for _, item := range plan.Workers {
		scheduleJSON := "{}"
		if item.Schedule != nil {
			data, err := json.Marshal(item.Schedule)
			if err != nil {
				return fmt.Errorf("marshal schedule for %s: %w", item.Name, err)
			}
			scheduleJSON = string(data)
		}
		worker := controldb.AgentWorker{
			ID:                    item.ID,
			WorkspaceID:           plan.WorkspaceID,
			Name:                  item.Name,
			DisplayName:           item.DisplayName,
			Status:                "active",
			Model:                 item.Model,
			RuntimeModel:          item.RuntimeModel,
			DefaultRuntimeNodeID:  item.RuntimeNodeID,
			DefaultRuntimeMode:    item.RuntimeMode,
			DefaultModelAccountID: item.Provider,
			PrimarySessionID:      stableMigrationID("sess", plan.WorkspaceID, item.ID),
			ScheduleJSON:          scheduleJSON,
			AttentionPolicyJSON:   `{"im_direct_message":true,"im_mention":true,"task_assigned":true,"workflow_step_assigned":true,"card_action":true,"ambient_channel_message":false}`,
			MemoryPolicyJSON:      "{}",
			SkillsJSON:            "[]",
			CreatedAt:             now,
			UpdatedAt:             now,
		}
		if err := db.UpsertAgentWorker(worker); err != nil {
			return fmt.Errorf("upsert agent worker %s: %w", item.Name, err)
		}
		membership := controldb.ProjectMembership{
			ID:               item.Membership.ID,
			WorkspaceID:      plan.WorkspaceID,
			ProjectID:        item.Membership.ProjectID,
			MemberType:       item.Membership.MemberType,
			MemberID:         item.Membership.MemberID,
			Role:             item.Membership.Role,
			Title:            item.Membership.Title,
			PermissionsJSON:  `["task.read","task.write","workflow.read","workflow.write","inbox.send"]`,
			AutoPickTasks:    item.Membership.AutoPickTasks,
			AttentionEnabled: item.Membership.AttentionEnabled,
			PriorityWeight:   item.Membership.PriorityWeight,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		if err := db.UpsertProjectMembership(membership); err != nil {
			return fmt.Errorf("upsert project membership %s: %w", item.Membership.ID, err)
		}
		if err := migrateWorkerTasksToDB(db, ts, plan.WorkspaceID, item); err != nil {
			return err
		}
		if err := migrateWorkerChannelBindings(db, plan.WorkspaceID, item); err != nil {
			return err
		}
		if err := migrateWorkerInteractionSessions(db, plan.WorkspaceID, item); err != nil {
			return err
		}
		if err := migrateWorkerInteractionRequests(db, plan.WorkspaceID, item); err != nil {
			return err
		}
		if err := migrateWorkerRuntimeRuns(db, plan.WorkspaceID, item); err != nil {
			return err
		}
	}
	if err := migrateWorkflowRuns(db, plan.WorkspaceID); err != nil {
		return err
	}
	return nil
}

func migrateWorkerChannelBindings(db controldb.Store, workspaceID string, item agentWorkerMigrationWorker) error {
	bindings, err := db.ListAgentChannelBindings(controldb.AgentChannelBindingFilter{
		WorkspaceID: workspaceID,
		ProjectID:   item.Project,
		AgentID:     item.LegacyAgent,
	})
	if err != nil {
		return fmt.Errorf("list channel bindings for %s/%s: %w", item.Project, item.LegacyAgent, err)
	}
	for _, binding := range bindings {
		binding.AgentWorkerID = item.ID
		if err := db.UpsertAgentChannelBinding(binding); err != nil {
			return fmt.Errorf("migrate channel binding %s: %w", binding.ID, err)
		}
	}
	return nil
}

func migrateWorkerInteractionSessions(db controldb.Store, workspaceID string, item agentWorkerMigrationWorker) error {
	sessions, err := db.ListInteractionSessions(controldb.InteractionSessionFilter{
		WorkspaceID: workspaceID,
		ProjectID:   item.Project,
		AgentID:     item.LegacyAgent,
		Limit:       500,
	})
	if err != nil {
		return fmt.Errorf("list interaction sessions for %s/%s: %w", item.Project, item.LegacyAgent, err)
	}
	for _, session := range sessions {
		session.AgentWorkerID = item.ID
		if err := db.UpdateInteractionSession(session); err != nil {
			return fmt.Errorf("migrate interaction session %s: %w", session.ID, err)
		}
	}
	return nil
}

func migrateWorkerInteractionRequests(db controldb.Store, workspaceID string, item agentWorkerMigrationWorker) error {
	for _, status := range []string{"active", "submitted", "expired", "cancelled"} {
		requests, err := db.ListInteractionRequests(controldb.InteractionRequestFilter{
			WorkspaceID: workspaceID,
			ProjectID:   item.Project,
			AgentID:     item.LegacyAgent,
			Status:      status,
			Limit:       500,
		})
		if err != nil {
			return fmt.Errorf("list interaction requests for %s/%s: %w", item.Project, item.LegacyAgent, err)
		}
		for _, request := range requests {
			request.AgentWorkerID = item.ID
			if err := db.UpdateInteractionRequest(request); err != nil {
				return fmt.Errorf("migrate interaction request %s: %w", request.ID, err)
			}
		}
	}
	return nil
}

func migrateWorkerRuntimeRuns(db controldb.Store, workspaceID string, item agentWorkerMigrationWorker) error {
	for _, status := range []string{"queued", "running", "completed", "failed", "cancelled"} {
		runs, err := db.ListRuntimeRuns(controldb.RuntimeRunFilter{
			WorkspaceID: workspaceID,
			ProjectID:   item.Project,
			AgentID:     item.LegacyAgent,
			Status:      status,
			Limit:       500,
		})
		if err != nil {
			return fmt.Errorf("list runtime runs for %s/%s: %w", item.Project, item.LegacyAgent, err)
		}
		for _, run := range runs {
			run.AgentWorkerID = item.ID
			run.ProjectMembershipID = item.Membership.ID
			if err := db.UpsertRuntimeRun(run); err != nil {
				return fmt.Errorf("migrate runtime run %s: %w", run.ID, err)
			}
		}
	}
	return nil
}

func migrateWorkflowRuns(db controldb.Store, workspaceID string) error {
	records, err := db.ListRecords("workflow_runs", workspaceID, nil)
	if err != nil {
		return fmt.Errorf("list workflow runs: %w", err)
	}
	wfStore := workflowstore.NewStore(db, workspaceID)
	for _, record := range records {
		var run entity.WorkflowRun
		if err := json.Unmarshal([]byte(record.Payload), &run); err != nil {
			return fmt.Errorf("parse workflow run %s: %w", strings.Join(record.Key, "/"), err)
		}
		project := strings.TrimSpace(run.Project)
		taskID := strings.TrimSpace(run.TaskID)
		if project == "" || taskID == "" {
			continue
		}
		if _, err := wfStore.ReconcileRunCurrentAssignee(run); err != nil {
			return fmt.Errorf("reconcile workflow run assignee for %s task %s: %w", project, taskID, err)
		}
	}
	return nil
}

func migrateWorkerTasksToDB(db controldb.Store, ts taskstore.Store, workspaceID string, item agentWorkerMigrationWorker) error {
	active, err := ts.ListTasks(item.Project, item.LegacyAgent)
	if err != nil {
		return fmt.Errorf("load active tasks for %s/%s: %w", item.Project, item.LegacyAgent, err)
	}
	archived, err := ts.ListArchivedTasks(item.Project, item.LegacyAgent)
	if err != nil {
		return fmt.Errorf("load archived tasks for %s/%s: %w", item.Project, item.LegacyAgent, err)
	}
	for _, task := range append(active, archived...) {
		if task == nil || strings.TrimSpace(task.ID) == "" {
			continue
		}
		normalizeMigratedTaskAssignee(task, item)
		raw, err := json.Marshal(task)
		if err != nil {
			return fmt.Errorf("marshal task %s/%s/%s: %w", item.Project, item.LegacyAgent, task.ID, err)
		}
		if err := db.UpsertRecord("tasks", workspaceID, []string{item.Project, item.LegacyAgent, task.ID}, string(raw)); err != nil {
			return fmt.Errorf("upsert task %s/%s/%s: %w", item.Project, item.LegacyAgent, task.ID, err)
		}
	}
	return nil
}

func normalizeMigratedTaskAssignee(task *entity.Task, item agentWorkerMigrationWorker) {
	if task == nil {
		return
	}
	legacy := item.Project + "/" + item.LegacyAgent
	if strings.TrimSpace(task.Assignee) == "" || strings.TrimSpace(task.Assignee) == legacy {
		task.Assignee = legacy
		task.AssigneeType = "agent_worker"
		task.AssigneeID = item.ID
		task.AssigneeMembershipID = item.Membership.ID
		return
	}
	if !strings.Contains(strings.TrimSpace(task.Assignee), "/") {
		task.AssigneeType = "user"
		task.AssigneeID = strings.TrimSpace(task.Assignee)
		task.AssigneeMembershipID = ""
	}
}

var nonWorkerNameChars = regexp.MustCompile(`[^a-z0-9-]+`)

func uniqueWorkerName(project, agent string, seen map[string]bool) string {
	base := slugPart(agent)
	if base == "" {
		base = "agent"
	}
	if !seen[base] {
		seen[base] = true
		return base
	}
	next := slugPart(project + "-" + agent)
	if next == "" {
		next = base + "-" + slugPart(project)
	}
	for i := 2; seen[next]; i++ {
		next = fmt.Sprintf("%s-%d", base, i)
	}
	seen[next] = true
	return next
}

func slugPart(s string) string {
	out := strings.ToLower(strings.TrimSpace(s))
	out = strings.ReplaceAll(out, "_", "-")
	out = nonWorkerNameChars.ReplaceAllString(out, "-")
	out = strings.Trim(out, "-")
	return out
}

func stableMigrationID(prefix string, parts ...string) string {
	sum := sha1.Sum([]byte(strings.Join(parts, "\x00")))
	return prefix + "_" + hex.EncodeToString(sum[:])[:16]
}

func cleanReportPath(root, path string) string {
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}
