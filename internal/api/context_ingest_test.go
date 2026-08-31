package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientTokenIngestsNormalizedContextIdempotently(t *testing.T) {
	s, _ := newProviderHandlerTestServer(t)
	rawToken := clientTokenPrefix + "context-ingest"
	if err := s.saveClientToken("ws-one", clientTokenRecord{
		ID: "ctok_ingest", Name: "collector", TokenHash: hashClientToken(rawToken),
		Username: "owner", Scopes: []string{clientScopeContextRW}, CreatedBy: "owner", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("save token: %v", err)
	}
	body := contextIngestBody{
		Source: contextSourceBody{ID: "src-test", Type: "rss_atom", Name: "Test feed", ConnectionRef: "https://example.test/feed.xml"},
		Items:  []contextItemBody{{SourceItemID: "entry-1", SourceURL: "https://example.test/1", Title: "Entry one", Content: "content"}},
	}
	rawBody, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	post := func() (int, map[string]any) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/context/ingest", bytes.NewReader(rawBody))
		req.Header.Set("Authorization", "Bearer "+rawToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(requestedWorkspaceHeader, "ws-one")
		recorder := httptest.NewRecorder()
		s.withContextIngestAuth(http.HandlerFunc(s.handleContextIngest)).ServeHTTP(recorder, req)
		var response map[string]any
		_ = json.Unmarshal(recorder.Body.Bytes(), &response)
		return recorder.Code, response
	}
	status, first := post()
	if status != http.StatusCreated || first["created"] != float64(1) {
		t.Fatalf("first ingest status=%d response=%s", status, recorderJSON(first))
	}
	status, second := post()
	if status != http.StatusCreated || second["deduplicated"] != float64(1) {
		t.Fatalf("second ingest status=%d response=%s", status, recorderJSON(second))
	}
}

func TestContextIngestRejectsNonMachineAuthentication(t *testing.T) {
	s, _ := newProviderHandlerTestServer(t)
	for name, testCase := range map[string]struct {
		authorization string
		query         string
	}{
		"missing":     {query: ""},
		"query-token": {query: "?_token=mgpat_not-accepted"},
		"user-token":  {authorization: "Bearer user-token"},
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/context/ingest"+testCase.query, bytes.NewReader([]byte(`{}`)))
			if testCase.authorization != "" {
				req.Header.Set("Authorization", testCase.authorization)
			}
			req.Header.Set(requestedWorkspaceHeader, "ws-one")
			recorder := httptest.NewRecorder()
			s.withContextIngestAuth(http.HandlerFunc(s.handleContextIngest)).ServeHTTP(recorder, req)
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func recorderJSON(value map[string]any) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}
