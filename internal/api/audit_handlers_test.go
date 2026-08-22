package api

import (
	"net/http"
	"testing"
)

func TestRequestIPPrefersClientHeaders(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://example.test", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.10, 127.0.0.1")
	if got := requestIP(req); got != "203.0.113.10" {
		t.Fatalf("ip=%q", got)
	}
}

func TestRequestIPUsesForwardedHeader(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://example.test", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("Forwarded", `for="[2001:db8::1]:443";proto=https`)
	if got := requestIP(req); got != "2001:db8::1" {
		t.Fatalf("ip=%q", got)
	}
}

func TestRequestIPFallsBackToRemoteAddr(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://example.test", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.RemoteAddr = "198.51.100.7:4567"
	req.Header.Set("X-Forwarded-For", "unknown")
	if got := requestIP(req); got != "198.51.100.7" {
		t.Fatalf("ip=%q", got)
	}
}
