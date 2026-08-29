package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/multigent/multigent/internal/contextpack"
	"github.com/multigent/multigent/internal/store"
)

func TestKnowledgeBaseAgentCanImportAndReadItem(t *testing.T) {
	s, _ := newProviderHandlerTestServer(t)
	rawToken := clientTokenPrefix + "writer"
	rec := clientTokenRecord{
		ID:        "ctok_writer",
		Name:      "writer",
		TokenHash: hashClientToken(rawToken),
		Username:  "owner",
		Scopes:    []string{clientScopeContextRW},
		CreatedBy: "owner",
		CreatedAt: time.Now().UTC(),
	}
	if err := s.saveClientToken("ws-one", rec); err != nil {
		t.Fatalf("save token: %v", err)
	}

	importBody := contextImportBody{
		CollectorType: contextpack.CollectorLocalAgentSession,
		Title:         "Knowledge Base Migration Note",
		Content:       "This is the important content for agent readback.",
		SourceName:    "session.jsonl",
		Project:       "sample",
		Tags:          []string{"migration"},
		BindScope:     contextpack.ScopeAgent,
		BindScopeID:   "sample/pm",
		Required:      true,
	}
	rawBody, _ := json.Marshal(importBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge-base/import", bytes.NewReader(rawBody))
	req.Header.Set("Authorization", "Bearer "+rawToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(requestedWorkspaceHeader, "ws-one")
	req = req.WithContext(context.WithValue(req.Context(), ctxUserKey, "owner"))

	recorder := httptest.NewRecorder()
	s.withTokenAuth(http.HandlerFunc(s.handleContextImport)).ServeHTTP(recorder, req)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("import status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var imported contextpack.ImportManualResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &imported); err != nil {
		t.Fatalf("decode import response: %v", err)
	}
	if imported.Doc == nil || imported.Doc.ID == "" {
		t.Fatalf("missing doc in import response: %+v", imported.Doc)
	}

	if imported.Doc == nil {
		t.Fatalf("missing doc from import result")
	}
	doc, err := store.NewDocsStore(s.root).Get(imported.Doc.ID)
	if err != nil {
		t.Fatalf("read doc: %v", err)
	}
	if doc == nil || doc.Title != "Knowledge Base Migration Note" {
		t.Fatalf("unexpected doc: %#v", doc)
	}
	docPath := doc.FilePath
	if !filepath.IsAbs(docPath) {
		docPath = filepath.Join(s.root, docPath)
	}
	content, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read doc content: %v", err)
	}
	if !strings.Contains(string(content), "original session transcript is stored as a workspace file") {
		t.Fatalf("doc content should be reference-style guidance: %s", string(content))
	}
	managedPath := imported.Asset.Metadata["managedFilePath"]
	if managedPath == "" {
		t.Fatalf("missing managed file path metadata: %#v", imported.Asset.Metadata)
	}
	rawSession, err := os.ReadFile(filepath.Join(s.root, ".multigent", "files", managedPath))
	if err != nil {
		t.Fatalf("read managed session file: %v", err)
	}
	if !strings.Contains(string(rawSession), "important content for agent readback") {
		t.Fatalf("managed session file missing imported content: %s", string(rawSession))
	}

	views, err := contextpack.NewStore(s.root).ListBindingViews(contextpack.AgentScopes("sample", "pm"))
	if err != nil {
		t.Fatalf("list binding views: %v", err)
	}
	if len(views) != 1 || views[0].Doc == nil || views[0].Doc.ID != imported.Doc.ID {
		t.Fatalf("unexpected binding views: %#v", views)
	}
	layer, err := contextpack.BuildAgentContextLayer(s.root, "sample", "pm")
	if err != nil {
		t.Fatalf("build agent context layer: %v", err)
	}
	if !strings.Contains(layer, "Knowledge Base Migration Note") || !strings.Contains(layer, "mga context read") {
		t.Fatalf("agent context layer missing imported doc:\n%s", layer)
	}
}
