package store

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
		return nil, errs.NotFound("team", path)
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
	recs, err := s.db.ListRecords("teams", s.workspaceID, nil)
	if err != nil {
		return nil, err
	}
	out := make([]*TeamEntry, 0, len(recs))
	for _, rec := range recs {
		var t entity.Team
		if err := json.Unmarshal([]byte(rec.Payload), &t); err != nil {
			continue
		}
		out = append(out, &TeamEntry{Path: rec.Key[0], Team: &t})
	}
	return out, nil
}

func (s *dbStore) Role(teamPath, roleName string) (*entity.Role, error) {
	var r entity.Role
	if ok, err := s.getJSON("roles", []string{teamPath, roleName}, &r); err != nil {
		return nil, err
	} else if !ok {
		return nil, fmt.Errorf("store: role %q/%q not found", teamPath, roleName)
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
	recs, err := s.db.ListRecords("roles", s.workspaceID, []string{teamPath})
	if err != nil {
		return nil, err
	}
	out := make([]*RoleEntry, 0, len(recs))
	for _, rec := range recs {
		var r entity.Role
		if err := json.Unmarshal([]byte(rec.Payload), &r); err != nil {
			continue
		}
		out = append(out, &RoleEntry{TeamPath: teamPath, Name: rec.Key[1], Role: &r})
	}
	return out, nil
}

func (s *dbStore) Project(name string) (*entity.Project, error) {
	var p entity.Project
	if ok, err := s.getJSON("projects", []string{name}, &p); err != nil {
		return nil, err
	} else if !ok {
		return nil, errs.NotFound("project", name)
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
	for _, rec := range recs {
		var p entity.Project
		if err := json.Unmarshal([]byte(rec.Payload), &p); err != nil {
			continue
		}
		out = append(out, &p)
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
	var meta entity.AgentMeta
	if ok, err := s.getJSON("agents", []string{project, name}, &meta); err != nil {
		return nil, err
	} else if !ok {
		return nil, errs.NotFound("agent", project+"/"+name)
	}
	return &meta, nil
}

func (s *dbStore) SaveAgentMeta(project, name string, meta *entity.AgentMeta) error {
	if meta.Name == "" {
		meta.Name = name
	}
	if meta.Project == "" {
		meta.Project = project
	}
	return s.putJSON("agents", []string{project, name}, meta)
}
func (s *dbStore) DeleteAgentMeta(project, name string) error {
	if err := s.db.DeleteRecord("agents", s.workspaceID, []string{project, name}); err != nil {
		return err
	}
	return s.files.DeleteAgentMeta(project, name)
}
func (s *dbStore) ListAgents(project string) ([]*AgentEntry, error) {
	recs, err := s.db.ListRecords("agents", s.workspaceID, []string{project})
	if err != nil {
		return nil, err
	}
	out := make([]*AgentEntry, 0, len(recs))
	for _, rec := range recs {
		var meta entity.AgentMeta
		if err := json.Unmarshal([]byte(rec.Payload), &meta); err != nil {
			continue
		}
		out = append(out, &AgentEntry{Project: project, Name: rec.Key[1], Meta: &meta})
	}
	return out, nil
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
