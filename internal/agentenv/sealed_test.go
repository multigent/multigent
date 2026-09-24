package agentenv

import (
	"strings"
	"testing"
)

func TestSealAndOpen(t *testing.T) {
	t.Setenv("MULTIGENT_CONNECTION_ENCRYPTION_KEY", "agent-env-test-key")
	sealed, err := Seal(map[string]string{"TOKEN": "secret-value", "EMPTY": ""})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sealed["TOKEN"], "sealed:") || strings.Contains(sealed["TOKEN"], "secret-value") {
		t.Fatalf("value was not sealed: %q", sealed["TOKEN"])
	}
	opened, err := Open(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if opened["TOKEN"] != "secret-value" || opened["EMPTY"] != "" {
		t.Fatalf("opened=%#v", opened)
	}
}

func TestOpenAcceptsLegacyPlaintext(t *testing.T) {
	opened, err := Open(map[string]string{"LEGACY": "plain"})
	if err != nil {
		t.Fatal(err)
	}
	if opened["LEGACY"] != "plain" {
		t.Fatalf("opened=%#v", opened)
	}
}
