package runtimeauth

import (
	"testing"
	"time"
)

func TestIssueAndValidate(t *testing.T) {
	token := Issue("secret", Payload{
		WorkspaceID:  "ws-one",
		Project:      "tapnow",
		Agent:        "pm",
		RunID:        "run-one",
		Capabilities: []string{"connection.use"},
	}, time.Minute)
	principal, ok := Validate("secret", token)
	if !ok {
		t.Fatalf("token did not validate")
	}
	if principal.WorkspaceID != "ws-one" || principal.Project != "tapnow" || principal.Agent != "pm" || principal.RunID != "run-one" {
		t.Fatalf("principal mismatch: %#v", principal)
	}
	if _, ok := Validate("wrong-secret", token); ok {
		t.Fatalf("token validated with wrong secret")
	}
}

func TestExpiredTokenDoesNotValidate(t *testing.T) {
	token := Issue("secret", Payload{
		WorkspaceID: "ws-one",
		Project:     "tapnow",
		Agent:       "pm",
	}, -time.Second)
	if _, ok := Validate("secret", token); ok {
		t.Fatalf("expired token validated")
	}
}
