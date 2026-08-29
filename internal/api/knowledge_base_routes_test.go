package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestKnowledgeBaseRoutes(t *testing.T) {
	s, _ := newProviderHandlerTestServer(t)
	token := s.users.signJWT(jwtPayload{
		Sub: "owner",
		Exp: time.Now().Add(time.Hour).Unix(),
		Iat: time.Now().Add(-time.Minute).Unix(),
	})

	paths := []string{
		"/api/v1/knowledge-base/collectors",
		"/api/v1/knowledge-base/sources",
		"/api/v1/knowledge-base/items",
	}

	for _, path := range paths {
		method := http.MethodGet
		var body io.Reader
		if path == "/api/v1/knowledge-base/items" {
			method = http.MethodPost
			body = strings.NewReader(`{"title":"kb","content":"body","sourceType":"manual"}`)
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, body)
		req = req.WithContext(context.WithValue(req.Context(), ctxUserKey, "owner"))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set(requestedWorkspaceHeader, "ws-one")
		if method == http.MethodPost {
			req.Header.Set("Content-Type", "application/json")
		}
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
			t.Fatalf("path=%s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
	}
}
