package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/multigent/multigent/internal/contextpack"
	"github.com/multigent/multigent/internal/store"
)

const contextImportMaxJSONBody = 225 << 20

type contextImportManualBody struct {
	Title       string   `json:"title"`
	Content     string   `json:"content"`
	SourceName  string   `json:"sourceName"`
	Project     string   `json:"project"`
	Tags        []string `json:"tags"`
	Description string   `json:"description"`
	BindScope   string   `json:"bindScope"`
	BindScopeID string   `json:"bindScopeId"`
	Required    bool     `json:"required"`
}

type contextImportBody struct {
	CollectorType string            `json:"collectorType"`
	Title         string            `json:"title"`
	Content       string            `json:"content"`
	SourceName    string            `json:"sourceName"`
	FilePath      string            `json:"filePath"`
	Project       string            `json:"project"`
	Tags          []string          `json:"tags"`
	Description   string            `json:"description"`
	BindScope     string            `json:"bindScope"`
	BindScopeID   string            `json:"bindScopeId"`
	Required      bool              `json:"required"`
	Metadata      map[string]string `json:"metadata"`
}

type contextImportFileBody struct {
	FilePath    string   `json:"filePath"`
	Title       string   `json:"title"`
	Project     string   `json:"project"`
	Tags        []string `json:"tags"`
	Description string   `json:"description"`
	BindScope   string   `json:"bindScope"`
	BindScopeID string   `json:"bindScopeId"`
	Required    bool     `json:"required"`
}

type contextBindingBody struct {
	ArtifactID string `json:"artifactId"`
	DocID      string `json:"docId"`
	ScopeType  string `json:"scopeType"`
	ScopeID    string `json:"scopeId"`
	Mode       string `json:"mode"`
	Required   bool   `json:"required"`
	Priority   int    `json:"priority"`
}

func (s *Server) handleContextArtifacts(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAccess(w, r) {
		return
	}
	scopeType := strings.TrimSpace(r.URL.Query().Get("scopeType"))
	scopeID := strings.TrimSpace(r.URL.Query().Get("scopeId"))
	var scopes []contextpack.ScopeRef
	if scopeType != "" {
		scopes = []contextpack.ScopeRef{{Type: scopeType, ID: scopeID}}
	}
	views, err := contextpack.NewStore(s.root).ListBindingViews(scopes)
	if err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"bindings": views})
}

func (s *Server) handleContextCollectors(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAccess(w, r) {
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"collectors": contextpack.NewRegistry().Specs()})
}

func (s *Server) handleContextImportManual(w http.ResponseWriter, r *http.Request) {
	var body contextImportManualBody
	if err := s.readJSON(w, r, &body); err != nil {
		return
	}
	if !s.checkContextWriteAccess(w, r, body.Project, body.BindScope, body.BindScopeID) {
		return
	}
	if !s.requireClientScope(w, r, clientScopeContextRW) {
		return
	}
	createdBy := "human"
	if cur := s.currentUser(r); cur != nil && cur.Username != "" {
		createdBy = cur.Username
	}
	res, err := contextpack.NewStore(s.root).ImportManual(contextpack.ImportManualInput{
		Title:       body.Title,
		Content:     body.Content,
		SourceName:  body.SourceName,
		CreatedBy:   createdBy,
		Project:     body.Project,
		Tags:        body.Tags,
		Description: body.Description,
		BindScope:   body.BindScope,
		BindScopeID: body.BindScopeID,
		Required:    body.Required,
	})
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) handleContextImport(w http.ResponseWriter, r *http.Request) {
	var body contextImportBody
	if err := s.readJSONMax(w, r, &body, contextImportMaxJSONBody); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	if !s.checkContextWriteAccess(w, r, body.Project, body.BindScope, body.BindScopeID) {
		return
	}
	if !s.requireClientScope(w, r, clientScopeContextRW) {
		return
	}
	createdBy := "human"
	if cur := s.currentUser(r); cur != nil && cur.Username != "" {
		createdBy = cur.Username
	}
	res, err := contextpack.NewStore(s.root).ImportContent(contextpack.ImportContentInput{
		CollectorType: strings.TrimSpace(body.CollectorType),
		Title:         body.Title,
		Content:       body.Content,
		SourceName:    body.SourceName,
		FilePath:      body.FilePath,
		CreatedBy:     createdBy,
		Project:       body.Project,
		Tags:          body.Tags,
		Description:   body.Description,
		BindScope:     body.BindScope,
		BindScopeID:   body.BindScopeID,
		Required:      body.Required,
		Metadata:      body.Metadata,
	})
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) handleContextImportFile(w http.ResponseWriter, r *http.Request) {
	var body contextImportFileBody
	if err := s.readJSON(w, r, &body); err != nil {
		return
	}
	if !s.checkContextWriteAccess(w, r, body.Project, body.BindScope, body.BindScopeID) {
		return
	}
	if !s.requireClientScope(w, r, clientScopeContextRW) {
		return
	}
	createdBy := "human"
	if cur := s.currentUser(r); cur != nil && cur.Username != "" {
		createdBy = cur.Username
	}
	res, err := contextpack.NewStore(s.root).ImportFile(contextpack.ImportFileInput{
		FilePath:    body.FilePath,
		Title:       body.Title,
		CreatedBy:   createdBy,
		Project:     body.Project,
		Tags:        body.Tags,
		Description: body.Description,
		BindScope:   body.BindScope,
		BindScopeID: body.BindScopeID,
		Required:    body.Required,
	})
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) handleContextCreateBinding(w http.ResponseWriter, r *http.Request) {
	var body contextBindingBody
	if err := s.readJSON(w, r, &body); err != nil {
		return
	}
	project, agent := projectAgentFromContextScope(body.ScopeType, body.ScopeID)
	if !s.checkContextWriteAccess(w, r, project, body.ScopeType, body.ScopeID) {
		return
	}
	if agent != "" && !s.canOperateAgent(r, project, agent) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeAgentOperatorRequired, "agent operator access required")
		return
	}
	if !s.requireClientScope(w, r, clientScopeContextRW) {
		return
	}
	createdBy := "human"
	if cur := s.currentUser(r); cur != nil && cur.Username != "" {
		createdBy = cur.Username
	}
	binding, err := contextpack.NewStore(s.root).AddBinding(contextpack.Binding{
		ArtifactID: strings.TrimSpace(body.ArtifactID),
		DocID:      strings.TrimSpace(body.DocID),
		ScopeType:  strings.TrimSpace(body.ScopeType),
		ScopeID:    strings.TrimSpace(body.ScopeID),
		Mode:       strings.TrimSpace(body.Mode),
		Required:   body.Required,
		Priority:   body.Priority,
		CreatedBy:  createdBy,
	})
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(binding)
}

func (s *Server) handleContextDeleteBinding(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAccess(w, r) {
		return
	}
	if !s.requireClientScope(w, r, clientScopeContextRW) {
		return
	}
	cs := contextpack.NewStore(s.root)
	bindingID := strings.TrimSpace(r.PathValue("id"))
	idx, err := cs.Load()
	if err != nil {
		s.serverError(w, err)
		return
	}
	var target *contextpack.Binding
	for i := range idx.Bindings {
		if idx.Bindings[i].ID == bindingID {
			target = &idx.Bindings[i]
			break
		}
	}
	if target == nil {
		s.jsonError(w, http.StatusNotFound, "context binding not found")
		return
	}
	project, _ := projectAgentFromContextScope(target.ScopeType, target.ScopeID)
	if !s.checkContextWriteAccess(w, r, project, target.ScopeType, target.ScopeID) {
		return
	}
	if err := cs.RemoveBinding(bindingID); err != nil {
		s.jsonError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) checkContextWriteAccess(w http.ResponseWriter, r *http.Request, project, scopeType, scopeID string) bool {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	project = strings.TrimSpace(project)
	if !s.checkCurrentWorkspaceAccess(w, r) {
		return false
	}
	switch normalizeContextScopeType(scopeType) {
	case "":
		if project == "" {
			if !s.canAdminCurrentWorkspace(r) {
				s.jsonErrorCode(w, http.StatusForbidden, ErrCodeWorkspaceAdminRequired, "workspace admin access required")
				return false
			}
			return true
		}
		if !s.canOperateProject(r, project) {
			s.jsonErrorCode(w, http.StatusForbidden, ErrCodeProjectOperatorRequired, "project operator access required")
			return false
		}
		return true
	case contextpack.ScopeWorkspace:
		if !s.canAdminCurrentWorkspace(r) {
			s.jsonErrorCode(w, http.StatusForbidden, ErrCodeWorkspaceAdminRequired, "workspace admin access required")
			return false
		}
		return true
	case contextpack.ScopeProject:
		if scopeID == "" {
			scopeID = project
		}
		if scopeID == "" || !s.canOperateProject(r, scopeID) {
			s.jsonErrorCode(w, http.StatusForbidden, ErrCodeProjectOperatorRequired, "project operator access required")
			return false
		}
		return true
	case contextpack.ScopeAgent:
		p, a, ok := strings.Cut(scopeID, "/")
		if !ok || strings.TrimSpace(p) == "" || strings.TrimSpace(a) == "" {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "agent scope must use project/agent")
			return false
		}
		if !s.canOperateAgent(r, strings.TrimSpace(p), strings.TrimSpace(a)) {
			s.jsonErrorCode(w, http.StatusForbidden, ErrCodeAgentOperatorRequired, "agent operator access required")
			return false
		}
		return true
	default:
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "invalid context binding scope")
		return false
	}
}

func normalizeContextScopeType(scopeType string) string {
	switch strings.TrimSpace(scopeType) {
	case "", contextpack.ScopeWorkspace, contextpack.ScopeProject, contextpack.ScopeAgent:
		return strings.TrimSpace(scopeType)
	default:
		return strings.TrimSpace(scopeType)
	}
}

func projectAgentFromContextScope(scopeType, scopeID string) (string, string) {
	if strings.TrimSpace(scopeType) != contextpack.ScopeAgent {
		if strings.TrimSpace(scopeType) == contextpack.ScopeProject {
			return strings.TrimSpace(scopeID), ""
		}
		return "", ""
	}
	project, agent, ok := strings.Cut(strings.TrimSpace(scopeID), "/")
	if !ok {
		return "", ""
	}
	return strings.TrimSpace(project), strings.TrimSpace(agent)
}

func (s *Server) handleAgentContextBindings(w http.ResponseWriter, r *http.Request) {
	project := strings.TrimSpace(r.PathValue("name"))
	agent := strings.TrimSpace(r.PathValue("agent"))
	if !s.checkProjectAccess(w, r, project) {
		return
	}
	if !s.agentExistsInProject(project, agent) {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeAgentNotFound, "agent not found")
		return
	}
	views, err := contextpack.NewStore(s.root).ListBindingViews(contextpack.AgentScopes(project, agent))
	if err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"bindings": views})
}

func (s *Server) handleRuntimeContextList(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.runtimeRequireCapability(w, r, "docs.use")
	if !ok {
		return
	}
	views, err := contextpack.NewStore(s.root).ListBindingViews(contextpack.AgentScopes(principal.Project, principal.Agent))
	if err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"bindings": views})
}

func (s *Server) handleRuntimeContextGet(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.runtimeRequireCapability(w, r, "docs.use")
	if !ok {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	views, err := contextpack.NewStore(s.root).ListBindingViews(contextpack.AgentScopes(principal.Project, principal.Agent))
	if err != nil {
		s.serverError(w, err)
		return
	}
	for _, view := range views {
		if view.Artifact.ID != id && view.Artifact.DocID != id && view.Binding.ID != id {
			continue
		}
		ds := store.NewDocsStore(s.root)
		docID := view.Artifact.DocID
		if docID == "" {
			docID = view.Binding.DocID
		}
		doc, err := ds.Get(docID)
		if err != nil {
			s.jsonError(w, http.StatusNotFound, "context document not found")
			return
		}
		content, err := ds.ReadContent(doc.FilePath)
		if err != nil {
			s.serverError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"binding":  view.Binding,
			"artifact": view.Artifact,
			"doc":      doc,
			"content":  content,
		})
		return
	}
	s.jsonError(w, http.StatusNotFound, "context not found or not granted to this agent")
}
