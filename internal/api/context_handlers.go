package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/multigent/multigent/internal/contextpack"
	"github.com/multigent/multigent/internal/store"
)

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
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	var body contextImportManualBody
	if err := s.readJSON(w, r, &body); err != nil {
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

func (s *Server) handleContextImportFile(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	var body contextImportFileBody
	if err := s.readJSON(w, r, &body); err != nil {
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
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	var body contextBindingBody
	if err := s.readJSON(w, r, &body); err != nil {
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
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	if err := contextpack.NewStore(s.root).RemoveBinding(r.PathValue("id")); err != nil {
		s.jsonError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
