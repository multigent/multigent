package store

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/errs"
)

type dbStore struct {
	root        string
	workspaceID string
	db          controldb.Store
	files       Store
}

func NewDB(root string, db controldb.Store) Store {
	workspaceID, _ := ensureWorkspace(root, db)
	return &dbStore{
		root:        root,
		workspaceID: workspaceID,
		db:          db,
		files:       NewFS(root),
	}
}

func (s *dbStore) Root() string { return s.root }

func (s *dbStore) Agency() (*entity.Agency, error)       { return s.files.Agency() }
func (s *dbStore) SaveAgency(a *entity.Agency) error     { return s.files.SaveAgency(a) }
func (s *dbStore) AgencyPrompt() (string, error)         { return s.files.AgencyPrompt() }
func (s *dbStore) SaveAgencyPrompt(content string) error { return s.files.SaveAgencyPrompt(content) }

func (s *dbStore) Team(path string) (*entity.Team, error) {
	var t entity.Team
	if ok, err := s.getJSON("teams", []string{path}, &t); err != nil {
		return nil, err
	} else if !ok {
		return s.files.Team(path)
	}
	return &t, nil
}

func (s *dbStore) SaveTeam(path string, t *entity.Team) error {
	if t.Name == "" {
		t.Name = path
	}
	return s.putJSON("teams", []string{path}, t)
}

func (s *dbStore) DeleteTeam(path string) error {
	roles, err := s.ListRoles(path)
	if err != nil {
		return err
	}
	for _, role := range roles {
		if err := s.db.DeleteRecord("roles", s.workspaceID, []string{path, role.Name}); err != nil {
			return err
		}
	}
	if err := s.db.DeleteRecord("teams", s.workspaceID, []string{path}); err != nil {
		return err
	}
	return s.files.DeleteTeam(path)
}

func (s *dbStore) TeamPrompt(path string) (string, error) { return s.files.TeamPrompt(path) }
func (s *dbStore) SaveTeamPrompt(path string, content string) error {
	return s.files.SaveTeamPrompt(path, content)
}

func (s *dbStore) ListTeams() ([]*TeamEntry, error) {
	fileTeams, fileErr := s.files.ListTeams()
	recs, err := s.db.ListRecords("teams", s.workspaceID, nil)
	if err != nil {
		if fileErr != nil {
			return nil, err
		}
		return fileTeams, nil
	}
	out := make([]*TeamEntry, 0, len(fileTeams)+len(recs))
	seen := make(map[string]bool, len(fileTeams)+len(recs))
	if fileErr == nil {
		for _, entry := range fileTeams {
			if entry == nil {
				continue
			}
			seen[entry.Path] = true
			out = append(out, entry)
		}
	}
	for _, rec := range recs {
		var t entity.Team
		if err := json.Unmarshal([]byte(rec.Payload), &t); err != nil {
			continue
		}
		if len(rec.Key) == 0 || seen[rec.Key[0]] {
			continue
		}
		seen[rec.Key[0]] = true
		out = append(out, &TeamEntry{Path: rec.Key[0], Team: &t})
	}
	return out, nil
}

func (s *dbStore) Role(teamPath, roleName string) (*entity.Role, error) {
	var r entity.Role
	if ok, err := s.getJSON("roles", []string{teamPath, roleName}, &r); err != nil {
		return nil, err
	} else if !ok {
		return s.files.Role(teamPath, roleName)
	}
	return &r, nil
}

func (s *dbStore) SaveRole(teamPath, roleName string, r *entity.Role) error {
	if r.Name == "" {
		r.Name = roleName
	}
	return s.putJSON("roles", []string{teamPath, roleName}, r)
}

func (s *dbStore) DeleteRole(teamPath, roleName string) error {
	if err := s.db.DeleteRecord("roles", s.workspaceID, []string{teamPath, roleName}); err != nil {
		return err
	}
	return s.files.DeleteRole(teamPath, roleName)
}

func (s *dbStore) RolePrompt(teamPath, roleName string) (string, error) {
	return s.files.RolePrompt(teamPath, roleName)
}
func (s *dbStore) SaveRolePrompt(teamPath, roleName string, content string) error {
	return s.files.SaveRolePrompt(teamPath, roleName, content)
}
func (s *dbStore) RoleDir(teamPath, roleName string) string {
	return filepath.Join(s.root, "teams", teamPath, "roles", roleName)
}
func (s *dbStore) ListRoles(teamPath string) ([]*RoleEntry, error) {
	fileRoles, fileErr := s.files.ListRoles(teamPath)
	recs, err := s.db.ListRecords("roles", s.workspaceID, []string{teamPath})
	if err != nil {
		if fileErr != nil {
			return nil, err
		}
		return fileRoles, nil
	}
	out := make([]*RoleEntry, 0, len(fileRoles)+len(recs))
	seen := make(map[string]bool, len(fileRoles)+len(recs))
	if fileErr == nil {
		for _, entry := range fileRoles {
			if entry == nil {
				continue
			}
			seen[entry.Name] = true
			out = append(out, entry)
		}
	}
	for _, rec := range recs {
		var r entity.Role
		if err := json.Unmarshal([]byte(rec.Payload), &r); err != nil {
			continue
		}
		if len(rec.Key) < 2 || seen[rec.Key[1]] {
			continue
		}
		seen[rec.Key[1]] = true
		out = append(out, &RoleEntry{TeamPath: teamPath, Name: rec.Key[1], Role: &r})
	}
	return out, nil
}

func (s *dbStore) Project(name string) (*entity.Project, error) {
	var p entity.Project
	if ok, err := s.getJSON("projects", []string{name}, &p); err != nil {
		return nil, err
	} else if !ok {
		return s.files.Project(name)
	}
	return &p, nil
}

func (s *dbStore) SaveProject(name string, p *entity.Project) error {
	if p.Name == "" {
		p.Name = name
	}
	return s.putJSON("projects", []string{name}, p)
}

func (s *dbStore) DeleteProject(name string) error {
	agents, err := s.ListAgents(name)
	if err != nil {
		return err
	}
	for _, agent := range agents {
		if err := s.db.DeleteRecord("agents", s.workspaceID, []string{name, agent.Name}); err != nil {
			return err
		}
	}
	if err := s.db.DeleteRecord("projects", s.workspaceID, []string{name}); err != nil {
		return err
	}
	return s.files.DeleteProject(name)
}

func (s *dbStore) ProjectPrompt(name string) (string, error) { return s.files.ProjectPrompt(name) }
func (s *dbStore) SaveProjectPrompt(name string, content string) error {
	return s.files.SaveProjectPrompt(name, content)
}
func (s *dbStore) ListProjects() ([]*entity.Project, error) {
	recs, err := s.db.ListRecords("projects", s.workspaceID, nil)
	if err != nil {
		return nil, err
	}
	out := make([]*entity.Project, 0, len(recs))
	seen := make(map[string]bool, len(recs))
	for _, rec := range recs {
		var p entity.Project
		if err := json.Unmarshal([]byte(rec.Payload), &p); err != nil {
			continue
		}
		if len(rec.Key) > 0 {
			seen[rec.Key[0]] = true
		}
		if p.Name != "" {
			seen[p.Name] = true
		}
		out = append(out, &p)
	}
	fileProjects, fileErr := s.files.ListProjects()
	if fileErr != nil {
		return out, nil
	}
	for _, p := range fileProjects {
		if p == nil || seen[p.Name] {
			continue
		}
		out = append(out, p)
		seen[p.Name] = true
	}
	return out, nil
}
func (s *dbStore) ProjectConfig(name string) (*entity.ProjectConfig, error) {
	return s.files.ProjectConfig(name)
}

func (s *dbStore) Skill(name string) (*entity.Skill, error) { return s.files.Skill(name) }
func (s *dbStore) SkillPrompt(name string) (string, error)  { return s.files.SkillPrompt(name) }
func (s *dbStore) ListSkills() ([]*entity.Skill, error)     { return s.files.ListSkills() }
func (s *dbStore) SkillDir(name string) string              { return s.files.SkillDir(name) }

func (s *dbStore) AgentMeta(project, name string) (*entity.AgentMeta, error) {
	worker, membership, ok, err := s.resolveAgentWorkerMembership(project, name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errs.NotFound("agent", project+"/"+name)
	}
	return agentMetaFromWorkerMembership(project, worker, membership), nil
}

func (s *dbStore) SaveAgentMeta(project, name string, meta *entity.AgentMeta) error {
	return fmt.Errorf("AgentMeta writes are not supported in 2.x; create or update AgentWorker and ProjectMembership instead")
}
func (s *dbStore) DeleteAgentMeta(project, name string) error {
	if err := s.db.DeleteRecord("agents", s.workspaceID, []string{project, name}); err != nil {
		return err
	}
	return nil
}
func (s *dbStore) ListAgents(project string) ([]*AgentEntry, error) {
	out := make([]*AgentEntry, 0)
	seen := make(map[string]bool)
	memberships, err := s.db.ListProjectMemberships(controldb.ProjectMembershipFilter{
		WorkspaceID: s.workspaceID,
		ProjectID:   project,
		MemberType:  "agent_worker",
	})
	if err != nil {
		return nil, err
	}
	for _, membership := range memberships {
		worker, ok, err := s.db.AgentWorkerByID(s.workspaceID, membership.MemberID)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		name := firstNonEmptyString(membership.Title, worker.Name)
		if name == "" || seen[name] {
			continue
		}
		meta := agentMetaFromWorkerMembership(project, worker, membership)
		out = append(out, &AgentEntry{Project: project, Name: name, Meta: meta})
		seen[name] = true
	}
	return out, nil
}

func agentMetaFromWorkerMembership(project string, worker controldb.AgentWorker, membership controldb.ProjectMembership) *entity.AgentMeta {
	name := firstNonEmptyString(membership.Title, worker.DisplayName, worker.Name)
	model := entity.AgentModel(strings.TrimSpace(worker.Model))
	if model == "" {
		model = entity.ModelHuman
	}
	createdAt := time.Now().UTC()
	if parsed, err := time.Parse(time.RFC3339, worker.CreatedAt); err == nil {
		createdAt = parsed
	}
	return &entity.AgentMeta{
		Name:          name,
		Project:       strings.TrimSpace(project),
		Role:          strings.TrimSpace(membership.Role),
		Model:         model,
		RuntimeModel:  strings.TrimSpace(worker.RuntimeModel),
		RuntimeMode:   strings.TrimSpace(worker.DefaultRuntimeMode),
		RuntimeNodeID: strings.TrimSpace(worker.DefaultRuntimeNodeID),
		Provider:      strings.TrimSpace(worker.DefaultModelAccountID),
		Avatar:        strings.TrimSpace(worker.Avatar),
		HiredAt:       createdAt,
	}
}

func (s *dbStore) AgentWorkerContext(projectName, agentName string) (AgentWorkerContext, error) {
	worker, membership, ok, err := s.resolveAgentWorkerMembership(projectName, agentName)
	if err != nil || !ok {
		return AgentWorkerContext{}, err
	}
	memberships, err := s.db.ListProjectMemberships(controldb.ProjectMembershipFilter{
		WorkspaceID: s.workspaceID,
		MemberType:  "agent_worker",
		MemberID:    worker.ID,
	})
	if err != nil {
		return AgentWorkerContext{}, err
	}
	var skills []string
	_ = json.Unmarshal([]byte(firstNonEmptyString(worker.SkillsJSON, "[]")), &skills)
	var b strings.Builder
	b.WriteString("## Agent Worker Identity\n\n")
	writeContextLine(&b, "Worker ID", worker.ID)
	writeContextLine(&b, "Name", worker.Name)
	writeContextLine(&b, "Display name", firstNonEmptyString(worker.DisplayName, worker.Name))
	writeContextLine(&b, "Description", worker.Description)
	writeContextLine(&b, "Default model", worker.Model)
	writeContextLine(&b, "Runtime model", worker.RuntimeModel)
	writeContextLine(&b, "Primary session", worker.PrimarySessionID)
	b.WriteString("\n")
	b.WriteString("This is your workspace-level identity. Project, task, workflow, and IM channel inputs are contexts for this same Agent Worker; they are not separate agent identities.\n\n")

	b.WriteString("## Current Project Membership\n\n")
	writeContextLine(&b, "Project", membership.ProjectID)
	writeContextLine(&b, "Membership ID", membership.ID)
	writeContextLine(&b, "Project title", firstNonEmptyString(membership.Title, worker.DisplayName, worker.Name))
	writeContextLine(&b, "Project role", membership.Role)
	writeContextLine(&b, "Auto-pick project tasks", fmt.Sprintf("%v", membership.AutoPickTasks))
	writeContextLine(&b, "Project attention enabled", fmt.Sprintf("%v", membership.AttentionEnabled))
	if strings.TrimSpace(membership.PermissionsJSON) != "" && strings.TrimSpace(membership.PermissionsJSON) != "[]" {
		writeContextLine(&b, "Project permissions", membership.PermissionsJSON)
	}
	if strings.TrimSpace(membership.Prompt) != "" {
		b.WriteString("\n### Project-specific responsibility\n\n")
		b.WriteString(strings.TrimSpace(membership.Prompt))
		b.WriteString("\n")
	}

	if len(memberships) > 1 {
		b.WriteString("\n## Other Project Memberships\n\n")
		for _, item := range memberships {
			if item.ID == membership.ID {
				continue
			}
			fmt.Fprintf(&b, "- `%s`: role `%s`, title `%s`, auto-pick `%v`, attention `%v`\n",
				item.ProjectID,
				firstNonEmptyString(item.Role, "member"),
				firstNonEmptyString(item.Title, worker.DisplayName, worker.Name),
				item.AutoPickTasks,
				item.AttentionEnabled,
			)
		}
		b.WriteString("\nWhen you work on a concrete task, use the project context and permissions for that task's project. Do not read or mutate a project unless you are a member of it.\n")
	}
	return AgentWorkerContext{WorkerID: worker.ID, Layer: strings.TrimSpace(b.String()), SkillNames: skills}, nil
}

func (s *dbStore) resolveAgentWorkerMembership(projectName, agentName string) (controldb.AgentWorker, controldb.ProjectMembership, bool, error) {
	if s == nil || s.db == nil {
		return controldb.AgentWorker{}, controldb.ProjectMembership{}, false, nil
	}
	memberships, err := s.db.ListProjectMemberships(controldb.ProjectMembershipFilter{
		WorkspaceID: s.workspaceID,
		ProjectID:   strings.TrimSpace(projectName),
		MemberType:  "agent_worker",
	})
	if err != nil {
		return controldb.AgentWorker{}, controldb.ProjectMembership{}, false, err
	}
	for _, membership := range memberships {
		worker, ok, err := s.db.AgentWorkerByID(s.workspaceID, membership.MemberID)
		if err != nil {
			return controldb.AgentWorker{}, controldb.ProjectMembership{}, false, err
		}
		if !ok {
			continue
		}
		if sameStoreIdentity(membership.Title, agentName) || sameStoreIdentity(worker.Name, agentName) || sameStoreIdentity(worker.DisplayName, agentName) {
			return worker, membership, true, nil
		}
	}
	return controldb.AgentWorker{}, controldb.ProjectMembership{}, false, nil
}

func writeContextLine(b *strings.Builder, key, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	fmt.Fprintf(b, "- %s: %s\n", key, value)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func sameStoreIdentity(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func (s *dbStore) AgentDir(project, name string) string {
	return filepath.Join(s.root, "projects", project, "agents", name)
}
func (s *dbStore) FiredAgentDir(project, firedDirName string) string {
	return filepath.Join(s.root, "projects", project, "agents", ".fired", firedDirName)
}
func (s *dbStore) ListFiredAgents(project string) ([]*FiredAgentEntry, error) {
	return nil, nil
}

func (s *dbStore) getJSON(table string, key []string, out any) (bool, error) {
	payload, ok, err := s.db.GetRecord(table, s.workspaceID, key)
	if err != nil || !ok {
		return ok, err
	}
	return true, json.Unmarshal([]byte(payload), out)
}

func (s *dbStore) putJSON(table string, key []string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.db.UpsertRecord(table, s.workspaceID, key, string(raw))
}

func workspaceID(root string) string {
	absRoot, _ := filepath.Abs(root)
	sum := sha1.Sum([]byte(absRoot))
	return hex.EncodeToString(sum[:])[:12]
}

func ensureWorkspace(root string, db controldb.Store) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	if !hasWorkspaceConfig(absRoot) {
		return workspaceID(absRoot), nil
	}
	if rows, err := db.ListWorkspaces(); err == nil {
		for _, row := range rows {
			if samePath(row.Root, absRoot) && row.ID != "" {
				return row.ID, nil
			}
		}
	}
	name := filepath.Base(absRoot)
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = "Multigent Workspace"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	id := workspaceID(absRoot)
	if base := filepath.Base(absRoot); base != "." && base != string(filepath.Separator) && base != "" {
		id = base
	}
	return id, db.UpsertWorkspace(controldb.Workspace{
		ID:        id,
		Name:      name,
		Slug:      name,
		Root:      absRoot,
		UpdatedAt: now,
	})
}

func hasWorkspaceConfig(absRoot string) bool {
	if _, err := os.Stat(filepath.Join(absRoot, ".multigent", "agency.yaml")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(absRoot, ".agencycli", "agency.yaml")); err == nil {
		return true
	}
	return false
}

func samePath(a, b string) bool {
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	if errA == nil {
		a = aa
	}
	if errB == nil {
		b = bb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}
