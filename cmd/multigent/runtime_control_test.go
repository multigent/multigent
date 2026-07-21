package main

import (
	"os"
	"testing"
)

func TestConfigureRuntimeAPIURL(t *testing.T) {
	t.Setenv("MULTIGENT_API_URL", "")
	if err := configureRuntimeAPIURL("0.0.0.0:27892"); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("MULTIGENT_API_URL"); got != "http://127.0.0.1:27892" {
		t.Fatalf("MULTIGENT_API_URL=%q", got)
	}
}

func TestConfigureRuntimeAPIURLPreservesOverride(t *testing.T) {
	t.Setenv("MULTIGENT_API_URL", "https://multigent.example")
	if err := configureRuntimeAPIURL("127.0.0.1:27892"); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("MULTIGENT_API_URL"); got != "https://multigent.example" {
		t.Fatalf("MULTIGENT_API_URL=%q", got)
	}
}
