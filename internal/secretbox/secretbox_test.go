package secretbox

import (
	"strings"
	"testing"
)

func TestSealStringWithoutEnvKeyDoesNotStoreRawValue(t *testing.T) {
	t.Setenv(EnvKey, "")

	sealed, err := SealString("sk-secret")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if !IsSealed(sealed) {
		t.Fatalf("secret should be sealed: %q", sealed)
	}
	if strings.Contains(sealed, "sk-secret") {
		t.Fatalf("sealed value leaked raw secret: %q", sealed)
	}

	opened, err := OpenString(sealed)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if opened != "sk-secret" {
		t.Fatalf("opened=%q", opened)
	}
}

func TestSealStringWithEnvKeyEncryptsAndOpensValue(t *testing.T) {
	t.Setenv(EnvKey, "test-encryption-key")

	sealed, err := SealString("sk-secret")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if !IsSealed(sealed) {
		t.Fatalf("secret should be sealed: %q", sealed)
	}
	if strings.Contains(sealed, "sk-secret") || strings.Contains(sealed, "plain-dev") {
		t.Fatalf("sealed value should not expose plaintext metadata: %q", sealed)
	}

	opened, err := OpenString(sealed)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if opened != "sk-secret" {
		t.Fatalf("opened=%q", opened)
	}
}

func TestOpenStringRejectsPlaintext(t *testing.T) {
	if _, err := OpenString("sk-secret"); err == nil {
		t.Fatal("expected plaintext secret to be rejected")
	}
}
