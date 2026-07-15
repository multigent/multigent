package api

import (
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
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	CreatedBy   string `json:"createdBy"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
	Root        string `json:"root"`
	Teams       int    `json:"teams"`
	Projects    int    `json:"projects"`
	Agents      int    `json:"agents"`
	Tasks       int    `json:"tasks"`
}

type updateWorkspaceRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type workspaceRef struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	Root         string `json:"root"`
	CreatedBy    string `json:"createdBy,omitempty"`
	CreatedAt    string `json:"createdAt,omitempty"`
	UpdatedAt    string `json:"updatedAt,omitempty"`
	LastOpenedAt string `json:"lastOpenedAt,omitempty"`
	Active       bool   `json:"active,omitempty"`
}

type createWorkspaceRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Root        string `json:"root"`
	Switch      bool   `json:"switch"`
}

type workspaceListResponse struct {
	Workspaces []workspaceRef `json:"workspaces"`
}

func (s *Server) handleWorkspace(w http.ResponseWriter, _ *http.Request) {
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

	_ = json.NewEncoder(w).Encode(workspaceSummary{
		Name:        name,
		Description: agency.Description,
		CreatedBy:   createdBy,
		CreatedAt:   createdAt,
		UpdatedAt:   agency.UpdatedAt,
		Root:        s.root,
		Teams:       len(teams),
		Projects:    len(projects),
		Agents:      agentCount,
		Tasks:       len(tasks),
	})
}

func (s *Server) handleListWorkspaces(w http.ResponseWriter, _ *http.Request) {
	refs, err := s.listWorkspaceRefs()
	if err != nil {
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(workspaceListResponse{Workspaces: refs})
}

func (s *Server) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
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

	root := strings.TrimSpace(req.Root)
	if root == "" {
		root = filepath.Join(filepath.Dir(s.root), slugifyWorkspaceName(name))
	}
	absRoot, err := filepath.Abs(root)
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

	cur := s.currentUser(r)
	createdBy := "system"
	if cur != nil && cur.Username != "" && cur.Username != "apikey" {
		createdBy = cur.Username
	}
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
		ID:          workspaceID(absRoot),
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
	_ = s.ensureCurrentUserMembership(ref.ID, createdBy)
	if req.Switch {
		if err := s.switchWorkspaceRoot(absRoot); err != nil {
			s.jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		ref.Active = true
	}
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
	if err := s.switchWorkspaceRoot(row.Root); err != nil {
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(workspaceRefFromDB(row, s.root))
}

func (s *Server) handlePutWorkspace(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
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
	_ = s.upsertWorkspaceRef(workspaceRef{
		ID:          workspaceID(s.root),
		Name:        workspaceDisplayName(agency.Name, s.root),
		Description: agency.Description,
		Root:        s.root,
		CreatedBy:   agency.CreatedBy,
		CreatedAt:   agency.CreatedAt,
		UpdatedAt:   agency.UpdatedAt,
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

func (s *Server) listWorkspaceRefs() ([]workspaceRef, error) {
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
	out := make([]workspaceRef, 0, len(rows))
	for _, row := range rows {
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
	ref := workspaceRef{
		ID:          workspaceID(root),
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
	ref.ID = workspaceID(ref.Root)
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
	return s.controlDB.MarkWorkspaceOpened(workspaceID(root))
}

func workspaceID(root string) string {
	absRoot, _ := filepath.Abs(root)
	sum := sha1.Sum([]byte(absRoot))
	return hex.EncodeToString(sum[:])[:12]
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
	return s.controlDB.UpsertWorkspaceMember(workspaceID, username, u.Role)
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
