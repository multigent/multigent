package lark

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegistrationPollTreatsAuthorizationPendingAsPending(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/v1/app/registration" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.Form.Get("action") != "poll" || r.Form.Get("device_code") != "device-one" {
			t.Fatalf("unexpected form: %s", r.Form.Encode())
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"authorization_pending","error_description":"","code":20094}`))
	}))
	defer server.Close()

	resp, err := (RegistrationClient{HTTPClient: server.Client()}).Poll(context.Background(), ProviderFeishu, "device-one", server.URL)
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if resp.Status != "pending" || resp.BaseURL != server.URL {
		t.Fatalf("response=%#v", resp)
	}
}

func TestRegistrationPollReturnsUnknownOAuthErrorAsStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_request","error_description":"bad device code"}`))
	}))
	defer server.Close()

	resp, err := (RegistrationClient{HTTPClient: server.Client()}).Poll(context.Background(), ProviderFeishu, "device-one", server.URL)
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if resp.Status != "error" || !strings.Contains(resp.Error, "invalid_request") {
		t.Fatalf("response=%#v", resp)
	}
}
