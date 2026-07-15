package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJSONErrorUsesStructuredEnvelope(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()

	s.jsonError(rec, http.StatusBadRequest, "invalid JSON body")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rec.Code)
	}
	if rec.Header().Get("X-Multigent-Error-Code") != ErrCodeInvalidJSON {
		t.Fatalf("error code header=%q", rec.Header().Get("X-Multigent-Error-Code"))
	}
	var body apiErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != ErrCodeInvalidJSON || body.Error.Message != "invalid JSON body" || body.Error.RequestID == "" {
		t.Fatalf("unexpected body=%#v", body)
	}
}

func TestJSONErrorCodeUsesExplicitCode(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()

	s.jsonErrorCode(rec, http.StatusUnauthorized, ErrCodeInvalidCredentials, "invalid credentials")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", rec.Code)
	}
	if rec.Header().Get("X-Multigent-Error-Code") != ErrCodeInvalidCredentials {
		t.Fatalf("error code header=%q", rec.Header().Get("X-Multigent-Error-Code"))
	}
	var body apiErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != ErrCodeInvalidCredentials || body.Error.Message != "invalid credentials" {
		t.Fatalf("unexpected body=%#v", body)
	}
}

func TestServerErrorDoesNotExposeInternalDetails(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()

	s.serverError(rec, errForTest("database password leaked"))

	var body apiErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if rec.Code != http.StatusInternalServerError || body.Error.Code != ErrCodeInternal || body.Error.Message != "internal error" {
		t.Fatalf("unexpected error response status=%d body=%#v", rec.Code, body)
	}
	if strings.Contains(rec.Body.String(), "database password leaked") {
		t.Fatalf("server error leaked internal detail")
	}
}

type errForTest string

func (e errForTest) Error() string { return string(e) }
