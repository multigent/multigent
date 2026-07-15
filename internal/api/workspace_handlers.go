package api

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/scaffold"
	"github.com/multigent/multigent/internal/store"
	"github.com/multigent/multigent/internal/taskstore"
)

type workspaceSummary struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	Description         string `json:"description,omitempty"`
	CreatedBy           string `json:"createdBy"`
	CreatedAt           string `json:"createdAt"`
	UpdatedAt           string `json:"updatedAt,omitempty"`
	CurrentUserRole     string `json:"currentUserRole,omitempty"`
	CurrentUserCanAdmin bool   `json:"currentUserCanAdmin"`
	Teams               int    `json:"teams"`
	Projects            int    `json:"projects"`
	Agents              int    `json:"agents"`
	Tasks               int    `json:"tasks"`
}

type updateWorkspaceRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type workspaceRef struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	Root         string `json:"-"`
	CreatedBy    string `json:"createdBy,omitempty"`
	CreatedAt    string `json:"createdAt,omitempty"`
	UpdatedAt    string `json:"updatedAt,omitempty"`
	LastOpenedAt string `json:"lastOpenedAt,omitempty"`
	Active       bool   `json:"active,omitempty"`
}

type createWorkspaceRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Switch      bool   `json:"switch"`
}

type workspaceListResponse struct {
	Workspaces []workspaceRef `json:"workspaces"`
}

const (
	WorkspaceRoleOwner  = "owner"
	WorkspaceRoleAdmin  = "admin"
	WorkspaceRoleMember = "member"
	WorkspaceRoleGuest  = "guest"
)

func (s *Server) handleWorkspace(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAccess(w, r) {
		return
	}
	agency, err := s.st.Agency()
	if err != nil {
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	teams, _ := s.st.ListTeams()
	projects, _ := s.st.ListProjects()
	tasks, _ := s.ts.ListAllTaskRecords("")

	agentCount := 0
	for _, project := range projects {
		if project == nil {
			continue
		}
		agents, err := s.st.ListAgents(project.Name)
		if err == nil {
			agentCount += len(agents)
		}
	}

	name := workspaceDisplayName(agency.Name, s.root)
	createdBy := strings.TrimSpace(agency.CreatedBy)
	if createdBy == "" {
		createdBy = "system"
	}
	createdAt := strings.TrimSpace(agency.CreatedAt)
	if createdAt == "" {
		createdAt = workspaceFileTime(s.root)
	}
	id := workspaceID(s.root)
	if existing, ok, err := s.workspaceRefForRoot(s.root); err == nil && ok && existing.ID != "" {
		id = existing.ID
	}
	currentUserRole, currentUserCanAdmin := s.currentWorkspaceRole(r, id)

	_ = json.NewEncoder(w).Encode(workspaceSummary{
		ID:                  id,
		Name:                name,
		Description:         agency.Description,
		CreatedBy:           createdBy,
		CreatedAt:           createdAt,
		UpdatedAt:           agency.UpdatedAt,
		CurrentUserRole:     currentUserRole,
		CurrentUserCanAdmin: currentUserCanAdmin,
		Teams:               len(teams),
		Projects:            len(projects),
		Agents:              agentCount,
		Tasks:               len(tasks),
	})
}

func (s *Server) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	refs, err := s.listWorkspaceRefs(r)
	if err != nil {
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(workspaceListResponse{Workspaces: refs})
}

func (s *Server) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	cur := s.currentUser(r)
	if cur == nil || cur.Username == "" || cur.Username == "apikey" {
		s.jsonError(w, http.StatusForbidden, "authenticated user required")
		return
	}
	var req createWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		s.jsonError(w, http.StatusBadRequest, "workspace name is required")
		return
	}

	id := newWorkspaceID()
	absRoot, err := filepath.Abs(filepath.Join(defaultWorkspaceDataDir(), "workspaces", id))
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := os.Stat(filepath.Join(absRoot, ".multigent", "agency.yaml")); err == nil {
		s.jsonError(w, http.StatusConflict, "workspace already exists at root")
		return
	}
	if err := os.MkdirAll(absRoot, 0o755); err != nil {
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	createdBy := cur.Username
	now := time.Now().UTC().Format(time.RFC3339)
	agency := &entity.Agency{
		Name:        name,
		Description: strings.TrimSpace(req.Description),
		CreatedBy:   createdBy,
		CreatedAt:   now,
	}
	if err := scaffold.InitAgency(absRoot, agency); err != nil {
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ref := workspaceRef{
		ID:          id,
		Name:        name,
		Description: agency.Description,
		Root:        absRoot,
		CreatedBy:   agency.CreatedBy,
		CreatedAt:   agency.CreatedAt,
	}
	if err := s.upsertWorkspaceRef(ref); err != nil {
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.controlDB.UpsertWorkspaceMember(ref.ID, createdBy, WorkspaceRoleOwner); err != nil {
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if req.Switch {
		if err := s.switchWorkspaceRoot(absRoot); err != nil {
			s.jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		ref.Active = true
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  ref.ID,
		Action:       "workspace.create",
		ResourceType: "workspace",
		ResourceID:   ref.ID,
		Summary:      "Workspace created",
		After: map[string]any{
			"id":          ref.ID,
			"name":        ref.Name,
			"description": ref.Description,
			"createdBy":   ref.CreatedBy,
		},
		Request: r,
	})
	_ = json.NewEncoder(w).Encode(ref)
}

func (s *Server) handleSwitchWorkspace(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		s.jsonError(w, http.StatusBadRequest, "workspace id is required")
		return
	}
	if s.controlDB == nil {
		s.jsonError(w, http.StatusServiceUnavailable, "control database unavailable")
		return
	}
	row, ok, err := s.controlDB.WorkspaceByID(id)
	if err != nil {
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		s.jsonError(w, http.StatusNotFound, "workspace not found")
		return
	}
	if !s.checkWorkspaceAccess(w, r, row.ID) {
		return
	}
	if err := s.switchWorkspaceRoot(row.Root); err != nil {
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  row.ID,
		Action:       "workspace.switch",
		ResourceType: "workspace",
		ResourceID:   row.ID,
		Summary:      "Workspace switched",
		After: map[string]any{
			"id":   row.ID,
			"name": row.Name,
		},
		Request: r,
	})
	_ = json.NewEncoder(w).Encode(workspaceRefFromDB(row, s.root))
}

func (s *Server) handlePutWorkspace(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}

	var req updateWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		s.jsonError(w, http.StatusBadRequest, "workspace name is required")
		return
	}

	agency, err := s.st.Agency()
	if err != nil {
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	before := map[string]any{
		"name":        workspaceDisplayName(agency.Name, s.root),
		"description": agency.Description,
	}

	now := time.Now().UTC().Format(time.RFC3339)
	agency.Name = name
	agency.Description = strings.TrimSpace(req.Description)
	if strings.TrimSpace(agency.CreatedBy) == "" {
		if cur := s.currentUser(r); cur != nil && cur.Username != "" && cur.Username != "apikey" {
			agency.CreatedBy = cur.Username
		} else {
			agency.CreatedBy = "system"
		}
	}
	if strings.TrimSpace(agency.CreatedAt) == "" {
		agency.CreatedAt = now
	}
	agency.UpdatedAt = now

	if err := s.st.SaveAgency(agency); err != nil {
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	id, err := s.currentWorkspaceID()
	if err != nil {
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.upsertWorkspaceRef(workspaceRef{
		ID:          id,
		Name:        workspaceDisplayName(agency.Name, s.root),
		Description: agency.Description,
		Root:        s.root,
		CreatedBy:   agency.CreatedBy,
		CreatedAt:   agency.CreatedAt,
		UpdatedAt:   agency.UpdatedAt,
	})
	s.auditLog(auditLogInput{
		WorkspaceID:  id,
		Action:       "workspace.update",
		ResourceType: "workspace",
		ResourceID:   id,
		Summary:      "Workspace updated",
		Before:       before,
		After: map[string]any{
			"name":        workspaceDisplayName(agency.Name, s.root),
			"description": agency.Description,
		},
		Request: r,
	})
	s.handleWorkspace(w, r)
}

func workspaceDisplayName(name, root string) string {
	name = strings.TrimSpace(name)
	if name == "" || filepath.IsAbs(name) || strings.ContainsAny(name, `/\`) {
		base := filepath.Base(filepath.Clean(root))
		if base != "." && base != string(filepath.Separator) && base != "" {
			return base
		}
		return "Multigent"
	}
	return name
}

func workspaceFileTime(root string) string {
	info, err := os.Stat(filepath.Join(root, ".multigent", "agency.yaml"))
	if err != nil {
		return ""
	}
	return info.ModTime().UTC().Format(time.RFC3339)
}

func (s *Server) listWorkspaceRefs(r *http.Request) ([]workspaceRef, error) {
	if err := s.ensureCurrentWorkspaceRegistered(); err != nil {
		return nil, err
	}
	if s.controlDB == nil {
		return nil, fmt.Errorf("control database unavailable")
	}
	rows, err := s.controlDB.ListWorkspaces()
	if err != nil {
		return nil, err
	}
	allowed := map[string]bool{}
	cur := s.currentUser(r)
	if cur != nil && cur.Username != "" && cur.Username != "apikey" {
		memberships, err := s.controlDB.ListWorkspaceMembersForUser(cur.Username)
		if err != nil {
			return nil, err
		}
		for _, m := range memberships {
			allowed[m.WorkspaceID] = true
		}
	} else if cur != nil && cur.Username == "apikey" {
		for _, row := range rows {
			allowed[row.ID] = true
		}
	}
	out := make([]workspaceRef, 0, len(rows))
	for _, row := range rows {
		if !allowed[row.ID] {
			continue
		}
		out = append(out, workspaceRefFromDB(row, s.root))
	}
	return out, nil
}

func (s *Server) ensureCurrentWorkspaceRegistered() error {
	if s.controlDB == nil {
		return fmt.Errorf("control database unavailable")
	}
	agency, err := s.st.Agency()
	if err != nil {
		return err
	}
	root, _ := filepath.Abs(s.root)
	id := workspaceID(root)
	if existing, ok, err := s.workspaceRefForRoot(root); err == nil && ok && existing.ID != "" {
		id = existing.ID
	}
	ref := workspaceRef{
		ID:          id,
		Name:        workspaceDisplayName(agency.Name, root),
		Description: agency.Description,
		Root:        root,
		CreatedBy:   agency.CreatedBy,
		CreatedAt:   agency.CreatedAt,
	}
	if ref.CreatedBy == "" {
		ref.CreatedBy = "system"
	}
	if ref.CreatedAt == "" {
		ref.CreatedAt = workspaceFileTime(root)
	}
	if err := s.upsertWorkspaceRef(ref); err != nil {
		return err
	}
	return s.ensureCurrentUserMembership(ref.ID, ref.CreatedBy)
}

func (s *Server) upsertWorkspaceRef(ref workspaceRef) error {
	if s.controlDB == nil {
		return fmt.Errorf("control database unavailable")
	}
	ref.Root, _ = filepath.Abs(ref.Root)
	if strings.TrimSpace(ref.ID) == "" {
		ref.ID = workspaceID(ref.Root)
	}
	return s.controlDB.UpsertWorkspace(controldb.Workspace{
		ID:           ref.ID,
		Name:         ref.Name,
		Slug:         slugifyWorkspaceName(ref.Name),
		Description:  ref.Description,
		Root:         ref.Root,
		CreatedBy:    ref.CreatedBy,
		CreatedAt:    ref.CreatedAt,
		UpdatedAt:    ref.UpdatedAt,
		LastOpenedAt: ref.LastOpenedAt,
	})
}

func (s *Server) switchWorkspaceRoot(root string) error {
	s.workspaceMu.Lock()
	defer s.workspaceMu.Unlock()

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(absRoot, ".multigent", "agency.yaml")); err != nil {
		return fmt.Errorf("workspace not found at %s", absRoot)
	}
	if samePath(s.root, absRoot) {
		return s.markWorkspaceOpened(absRoot)
	}

	if s.triggers != nil {
		s.triggers.StopPoller()
	}
	if s.sched != nil {
		s.sched.Cleanup()
	}

	s.root = absRoot
	s.st = store.NewDB(absRoot, s.controlDB)
	s.ts = taskstore.NewDB(absRoot, s.controlDB)
	s.sched = newSchedulerManager(absRoot)
	s.triggers = newTriggerManager(absRoot, s.sched.binPath, s.ts)
	s.triggers.StartPoller()
	s.ccStore = store.NewCCConnectStore(absRoot)
	s.okrStore = store.NewOKRStore(absRoot)
	s.msStore = store.NewMilestoneStore(absRoot)
	return s.markWorkspaceOpened(absRoot)
}

func (s *Server) markWorkspaceOpened(root string) error {
	if s.controlDB == nil {
		return nil
	}
	ref, ok, err := s.workspaceRefForRoot(root)
	if err != nil {
		return err
	}
	if ok {
		return s.controlDB.MarkWorkspaceOpened(ref.ID)
	}
	return s.controlDB.MarkWorkspaceOpened(workspaceID(root))
}

func (s *Server) workspaceRefForRoot(root string) (workspaceRef, bool, error) {
	if s.controlDB == nil {
		return workspaceRef{}, false, nil
	}
	rows, err := s.controlDB.ListWorkspaces()
	if err != nil {
		return workspaceRef{}, false, err
	}
	for _, row := range rows {
		if samePath(row.Root, root) {
			return workspaceRefFromDB(row, s.root), true, nil
		}
	}
	return workspaceRef{}, false, nil
}

func (s *Server) currentWorkspaceID() (string, error) {
	ref, ok, err := s.workspaceRefForRoot(s.root)
	if err != nil {
		return "", err
	}
	if ok && ref.ID != "" {
		return ref.ID, nil
	}
	return workspaceID(s.root), nil
}

func (s *Server) checkCurrentWorkspaceAccess(w http.ResponseWriter, r *http.Request) bool {
	id, err := s.currentWorkspaceID()
	if err != nil {
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return false
	}
	return s.checkWorkspaceAccess(w, r, id)
}

func (s *Server) checkWorkspaceAccess(w http.ResponseWriter, r *http.Request, workspaceID string) bool {
	cur := s.currentUser(r)
	if cur != nil && cur.Username == "apikey" {
		return true
	}
	if cur == nil || cur.Username == "" {
		s.jsonError(w, http.StatusForbidden, "workspace access required")
		return false
	}
	if s.controlDB == nil {
		s.jsonError(w, http.StatusServiceUnavailable, "control database unavailable")
		return false
	}
	if _, ok, err := s.controlDB.WorkspaceMember(workspaceID, cur.Username); err != nil {
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return false
	} else if ok {
		return true
	}
	s.jsonError(w, http.StatusForbidden, "workspace access required")
	return false
}

func (s *Server) checkCurrentWorkspaceAdmin(w http.ResponseWriter, r *http.Request) bool {
	id, err := s.currentWorkspaceID()
	if err != nil {
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return false
	}
	cur := s.currentUser(r)
	if cur != nil && cur.Username == "apikey" {
		return true
	}
	if cur == nil || cur.Username == "" {
		s.jsonError(w, http.StatusForbidden, "workspace admin access required")
		return false
	}
	member, ok, err := s.controlDB.WorkspaceMember(id, cur.Username)
	if err != nil {
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return false
	}
	if ok && (member.Role == WorkspaceRoleOwner || member.Role == WorkspaceRoleAdmin) {
		return true
	}
	s.jsonError(w, http.StatusForbidden, "workspace admin access required")
	return false
}

func (s *Server) currentWorkspaceRole(r *http.Request, workspaceID string) (string, bool) {
	cur := s.currentUser(r)
	if cur != nil && cur.Username == "apikey" {
		return WorkspaceRoleOwner, true
	}
	if cur == nil || cur.Username == "" || s.controlDB == nil {
		return "", false
	}
	member, ok, err := s.controlDB.WorkspaceMember(workspaceID, cur.Username)
	if err != nil || !ok {
		return "", false
	}
	return member.Role, member.Role == WorkspaceRoleOwner || member.Role == WorkspaceRoleAdmin
}

func workspaceID(root string) string {
	absRoot, _ := filepath.Abs(root)
	sum := sha1.Sum([]byte(absRoot))
	return hex.EncodeToString(sum[:])[:12]
}

func newWorkspaceID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("ws-%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func defaultWorkspaceDataDir() string {
	if v := strings.TrimSpace(os.Getenv("MULTIGENT_DATA_DIR")); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".multigent"
	}
	return filepath.Join(home, ".multigent")
}

func workspaceRefFromDB(row controldb.Workspace, currentRoot string) workspaceRef {
	name := row.Name
	if name == "" {
		name = filepath.Base(row.Root)
	}
	return workspaceRef{
		ID:           row.ID,
		Name:         name,
		Description:  row.Description,
		Root:         row.Root,
		CreatedBy:    row.CreatedBy,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
		LastOpenedAt: row.LastOpenedAt,
		Active:       samePath(row.Root, currentRoot),
	}
}

func (s *Server) ensureCurrentUserMembership(workspaceID, username string) error {
	if s.controlDB == nil || username == "" || username == "system" {
		return nil
	}
	u := s.users.GetUser(username)
	if u == nil {
		return nil
	}
	role := WorkspaceRoleMember
	if u.Role == RoleAdmin || username == "admin" {
		role = WorkspaceRoleOwner
	}
	return s.controlDB.UpsertWorkspaceMember(workspaceID, username, role)
}

var workspaceSlugInvalid = regexp.MustCompile(`[^a-z0-9-]+`)

func slugifyWorkspaceName(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = strings.ReplaceAll(slug, "_", "-")
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = workspaceSlugInvalid.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "workspace"
	}
	return slug
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
