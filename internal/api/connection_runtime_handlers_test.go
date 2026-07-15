package api

import (
	"encoding/json"
	"strings"
	"testing"

	controldb "github.com/multigent/multigent/internal/db"
)

func TestConnectionGrantMatchesAgent(t *testing.T) {
	tests := []struct {
		name      string
		grant     controldb.ConnectionGrant
		workspace string
		project   string
		agent     string
		want      bool
	}{
		{
			name:      "workspace grant matches current workspace",
			grant:     controldb.ConnectionGrant{TargetType: ConnectionTargetWorkspace, TargetID: "ws-one"},
			workspace: "ws-one",
			project:   "tapnow",
			agent:     "dev",
			want:      true,
		},
		{
			name:      "workspace grant rejects another workspace",
			grant:     controldb.ConnectionGrant{TargetType: ConnectionTargetWorkspace, TargetID: "ws-two"},
			workspace: "ws-one",
			project:   "tapnow",
			agent:     "dev",
			want:      false,
		},
		{
			name:      "project grant matches project",
			grant:     controldb.ConnectionGrant{TargetType: ConnectionTargetProject, TargetID: "tapnow"},
			workspace: "ws-one",
			project:   "tapnow",
			agent:     "dev",
			want:      true,
		},
		{
			name:      "agent grant matches exact agent ref",
			grant:     controldb.ConnectionGrant{TargetType: ConnectionTargetAgent, TargetID: "tapnow/dev"},
			workspace: "ws-one",
			project:   "tapnow",
			agent:     "dev",
			want:      true,
		},
		{
			name:      "user grant does not become agent runtime access",
			grant:     controldb.ConnectionGrant{TargetType: ConnectionTargetUser, TargetID: "ella"},
			workspace: "ws-one",
			project:   "tapnow",
			agent:     "dev",
			want:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := connectionGrantMatchesAgent(tt.grant, tt.workspace, tt.project, tt.agent)
			if got != tt.want {
				t.Fatalf("connectionGrantMatchesAgent()=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestAgentRuntimeConnectionResponseDoesNotExposeSecretValues(t *testing.T) {
	connection := controldb.Connection{
		ID:             "conn-one",
		Provider:       "github",
		ConnectionName: "ci",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        "ws-one",
		AuthType:       ConnectionAuthAPIKey,
		ProfileJSON:    `{"provider":"github","connectionName":"ci","visible":"ok","apiKey":"ghp_secret","token":"secret"}`,
	}
	resp := agentRuntimeConnectionToResponse(connection, []controldb.ConnectionGrant{
		{ID: "grant-one", TargetType: ConnectionTargetAgent, TargetID: "tapnow/dev"},
	})
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	body := string(raw)
	for _, forbidden := range []string{"apiKey", "secret", "ciphertext", "nonce", "values"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("runtime response leaked %q: %s", forbidden, body)
		}
	}
	if resp.Profile["visible"] != "ok" {
		t.Fatalf("profile not preserved: %#v", resp.Profile)
	}
	if _, ok := resp.Profile["apiKey"]; ok {
		t.Fatalf("apiKey was not removed from profile: %#v", resp.Profile)
	}
	if len(resp.MatchedGrants) != 1 || resp.MatchedGrants[0].ID != "grant-one" {
		t.Fatalf("matched grants not preserved: %#v", resp.MatchedGrants)
	}
}
