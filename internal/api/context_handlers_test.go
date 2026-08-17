package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/multigent/multigent/internal/contextpack"
)

func TestWorkspaceMemberCanManageOwnClientTokens(t *testing.T) {
	s, _ := newProviderHandlerTestServer(t)

	createRec := httptest.NewRecorder()
	createReq := providerTestRequest(http.MethodPost, "/api/v1/client-tokens", "member", createClientTokenBody{
		Name:   "member cli",
		Scopes: []string{clientScopeContextRW},
	})
	s.handleClientTokensCreate(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	var created createClientTokenResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.Raw == "" || created.Token.Username != "member" {
		t.Fatalf("unexpected created token: %+v", created)
	}

	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		t.Fatalf("workspace id: %v", err)
	}
	ownerToken := clientTokenRecord{
		ID:        "ctok_owner",
		Name:      "owner cli",
		TokenHash: hashClientToken(clientTokenPrefix + "owner"),
		Username:  "owner",
		Scopes:    []string{clientScopeContextRW},
		CreatedBy: "owner",
		CreatedAt: time.Now().UTC(),
	}
	if err := s.saveClientToken(workspaceID, ownerToken); err != nil {
		t.Fatalf("save owner token: %v", err)
	}

	listRec := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/client-tokens", nil)
	listReq = listReq.WithContext(context.WithValue(listReq.Context(), ctxUserKey, "member"))
	s.handleClientTokensList(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	var listed ClientTokenListRespForTest
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listed.Tokens) != 1 || listed.Tokens[0].Username != "member" || listed.Tokens[0].TokenHash != "" {
		t.Fatalf("unexpected list: %+v", listed.Tokens)
	}
}

type ClientTokenListRespForTest struct {
	Tokens []clientTokenRecord `json:"tokens"`
}

func TestClientTokenCanImportContextContent(t *testing.T) {
	s, _ := newProviderHandlerTestServer(t)
	rawToken := clientTokenPrefix + "test-token"
	rec := clientTokenRecord{
		ID:        "ctok_test",
		Name:      "local uploader",
		TokenHash: hashClientToken(rawToken),
		Username:  "owner",
		Scopes:    []string{clientScopeContextRW},
		CreatedBy: "owner",
		CreatedAt: time.Now().UTC(),
	}
	if err := s.saveClientToken("ws-one", rec); err != nil {
		t.Fatalf("save token: %v", err)
	}
	body := contextImportBody{
		CollectorType: contextpack.CollectorLocalAgentSession,
		Title:         "Imported Codex Session",
		Content:       `{"type":"message","message":{"content":[{"type":"text","text":"important project context"}]}}`,
		SourceName:    "session.jsonl",
		Project:       "demo",
		Tags:          []string{"session"},
		BindScope:     contextpack.ScopeAgent,
		BindScopeID:   "demo/Lina",
		Required:      true,
	}
	rawBody, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/context/import", bytes.NewReader(rawBody))
	req.Header.Set("Authorization", "Bearer "+rawToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(requestedWorkspaceHeader, "ws-one")

	recorder := httptest.NewRecorder()
	s.withTokenAuth(http.HandlerFunc(s.handleContextImport)).ServeHTTP(recorder, req)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("import status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var res contextpack.ImportManualResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if res.Doc == nil || res.Doc.ID == "" {
		t.Fatalf("expected imported doc, got %+v", res.Doc)
	}
	if res.Binding == nil || res.Binding.ScopeID != "demo/Lina" {
		t.Fatalf("expected agent binding, got %+v", res.Binding)
	}
}

func TestClientTokenScopeRequiredForContextImport(t *testing.T) {
	s, _ := newProviderHandlerTestServer(t)
	rawToken := clientTokenPrefix + "readonly"
	rec := clientTokenRecord{
		ID:        "ctok_readonly",
		Name:      "readonly",
		TokenHash: hashClientToken(rawToken),
		Username:  "owner",
		Scopes:    []string{},
		CreatedBy: "owner",
		CreatedAt: time.Now().UTC(),
	}
	if err := s.saveClientToken("ws-one", rec); err != nil {
		t.Fatalf("save token: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/context/import", bytes.NewReader([]byte(`{"title":"x","content":"x"}`)))
	req.Header.Set("Authorization", "Bearer "+rawToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(requestedWorkspaceHeader, "ws-one")

	recorder := httptest.NewRecorder()
	s.withTokenAuth(http.HandlerFunc(s.handleContextImport)).ServeHTTP(recorder, req)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
