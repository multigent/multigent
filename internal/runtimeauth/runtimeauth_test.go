package runtimeauth

import (
	"testing"
	"time"
)

func TestIssueAndValidate(t *testing.T) {
	token := Issue("secret", Payload{
		WorkspaceID:  "ws-one",
		Project:      "sample",
		Agent:        "pm",
		RunID:        "run-one",
		Capabilities: []string{"connection.use"},
	}, time.Minute)
	principal, ok := Validate("secret", token)
	if !ok {
		t.Fatalf("token did not validate")
	}
	if principal.WorkspaceID != "ws-one" || principal.Project != "sample" || principal.Agent != "pm" || principal.RunID != "run-one" {
		t.Fatalf("principal mismatch: %#v", principal)
	}
	if _, ok := Validate("wrong-secret", token); ok {
		t.Fatalf("token validated with wrong secret")
	}
}

func TestExpiredTokenDoesNotValidate(t *testing.T) {
	token := Issue("secret", Payload{
		WorkspaceID: "ws-one",
		Project:     "sample",
		Agent:       "pm",
	}, -time.Second)
	if _, ok := Validate("secret", token); ok {
		t.Fatalf("expired token validated")
	}
}

func TestIssueAndValidateDelegation(t *testing.T) {
	token := IssueDelegation("secret", DelegationPayload{
		WorkspaceID:   "ws-one",
		Project:       "sample",
		Agent:         "pm",
		UserID:        "owner",
		InteractionID: "ir-one",
		Scopes:        []string{"act_as_user"},
	}, time.Minute)
	principal, ok := ValidateDelegation("secret", token)
	if !ok {
		t.Fatalf("delegation token did not validate")
	}
	if principal.WorkspaceID != "ws-one" || principal.Project != "sample" || principal.Agent != "pm" || principal.UserID != "owner" || principal.InteractionID != "ir-one" {
		t.Fatalf("delegation principal mismatch: %#v", principal)
	}
	if _, ok := Validate("secret", token); ok {
		t.Fatalf("delegation token validated as runtime token")
	}
}

func TestExpiredDelegationTokenDoesNotValidate(t *testing.T) {
	token := IssueDelegation("secret", DelegationPayload{
		WorkspaceID: "ws-one",
		Project:     "sample",
		Agent:       "pm",
		UserID:      "owner",
	}, -time.Second)
	if _, ok := ValidateDelegation("secret", token); ok {
		t.Fatalf("expired delegation token validated")
	}
}
