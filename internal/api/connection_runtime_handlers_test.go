package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/multigent/multigent/internal/connector"
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
			project:   "sample",
			agent:     "dev",
			want:      true,
		},
		{
			name:      "workspace grant rejects another workspace",
			grant:     controldb.ConnectionGrant{TargetType: ConnectionTargetWorkspace, TargetID: "ws-two"},
			workspace: "ws-one",
			project:   "sample",
			agent:     "dev",
			want:      false,
		},
		{
			name:      "project grant matches project",
			grant:     controldb.ConnectionGrant{TargetType: ConnectionTargetProject, TargetID: "sample"},
			workspace: "ws-one",
			project:   "sample",
			agent:     "dev",
			want:      true,
		},
		{
			name:      "agent grant matches exact agent ref",
			grant:     controldb.ConnectionGrant{TargetType: ConnectionTargetAgent, TargetID: "sample/dev"},
			workspace: "ws-one",
			project:   "sample",
			agent:     "dev",
			want:      true,
		},
		{
			name:      "user grant does not become agent runtime access",
			grant:     controldb.ConnectionGrant{TargetType: ConnectionTargetUser, TargetID: "ella"},
			workspace: "ws-one",
			project:   "sample",
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
		ProfileJSON:    `{"provider":"github","connectionName":"ci","visible":"ok","apiKey":"ghp_secret","token":"secret","accountName":"octo","accountEmail":"octo@example.test","scopes":["repo"],"providerPermissions":["Issues:write"]}`,
	}
	resp := agentRuntimeConnectionToResponse(connection, []controldb.ConnectionGrant{
		{ID: "grant-one", TargetType: ConnectionTargetAgent, TargetID: "sample/dev"},
	}, []connector.ProviderAction{{Name: "get_authenticated_user", Method: "GET", Endpoint: "/user"}}, []connector.ToolRuntimeAdapter{
		{Type: connector.RuntimeAdapterCLI, Priority: 100, Skills: []string{"github"}},
		{Type: connector.RuntimeAdapterHTTPAction, Priority: 20, HTTPAction: &connector.ToolHTTPActionAdapter{ActionNames: []string{"get_authenticated_user"}}},
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
	if resp.Runtime.Alias != "github-ci" {
		t.Fatalf("runtime alias=%q", resp.Runtime.Alias)
	}
	if resp.Runtime.MCPProxy.Path != agentConnectionManifest().MCPProxyPath {
		t.Fatalf("mcp proxy path=%q", resp.Runtime.MCPProxy.Path)
	}
	if len(resp.Runtime.MCPProxy.Headers) == 0 {
		t.Fatalf("mcp proxy headers missing")
	}
	if len(resp.Runtime.Actions) != 1 || resp.Runtime.Actions[0].Name != "get_authenticated_user" {
		t.Fatalf("runtime actions missing: %#v", resp.Runtime.Actions)
	}
	if resp.Runtime.RecommendedAdapter != connector.RuntimeAdapterCLI {
		t.Fatalf("recommended adapter=%q", resp.Runtime.RecommendedAdapter)
	}
	if len(resp.Runtime.Adapters) != 2 {
		t.Fatalf("runtime adapters missing: %#v", resp.Runtime.Adapters)
	}
	if resp.Profile["visible"] != "ok" {
		t.Fatalf("profile not preserved: %#v", resp.Profile)
	}
	if _, ok := resp.Profile["apiKey"]; ok {
		t.Fatalf("apiKey was not removed from profile: %#v", resp.Profile)
	}
	if resp.ProfileSummary.AccountName != "octo" || resp.ProfileSummary.AccountEmail != "octo@example.test" {
		t.Fatalf("profile summary identity=%#v", resp.ProfileSummary)
	}
	if strings.Join(resp.ProfileSummary.Scopes, ",") != "repo" || strings.Join(resp.ProfileSummary.ProviderPermissions, ",") != "Issues:write" {
		t.Fatalf("profile summary grants=%#v", resp.ProfileSummary)
	}
	if len(resp.MatchedGrants) != 1 || resp.MatchedGrants[0].ID != "grant-one" {
		t.Fatalf("matched grants not preserved: %#v", resp.MatchedGrants)
	}
}

func TestRuntimeAdaptersForProviderConnectionFiltersHTTPActions(t *testing.T) {
	provider := connector.Provider{
		Provider: "github",
		Actions: []connector.ProviderAction{
			{Name: "allowed", Method: "GET", Endpoint: "/user"},
			{Name: "blocked", Method: "DELETE", Endpoint: "/repos/{owner}/{repo}"},
		},
		RuntimeAdapters: []connector.ToolRuntimeAdapter{
			{Type: connector.RuntimeAdapterCLI, Priority: 100, CLI: &connector.ToolCLIAdapter{Binary: "gh"}},
			{Type: connector.RuntimeAdapterHTTPAction, Priority: 20, HTTPAction: &connector.ToolHTTPActionAdapter{ActionNames: []string{"allowed", "blocked"}}},
		},
	}
	connection := controldb.Connection{ProfileJSON: `{"allowedActionMethods":["GET"],"allowedActionEndpoints":["/user"]}`}
	actions := runtimeActionsForProviderConnection(connection, provider)
	adapters := runtimeAdaptersForProviderConnection(provider, actions)
	if len(actions) != 1 || actions[0].Name != "allowed" {
		t.Fatalf("actions=%#v", actions)
	}
	if len(adapters) != 2 {
		t.Fatalf("adapters=%#v", adapters)
	}
	var httpActions []string
	for _, adapter := range adapters {
		if adapter.Type == connector.RuntimeAdapterHTTPAction {
			httpActions = adapter.HTTPAction.ActionNames
		}
	}
	if strings.Join(httpActions, ",") != "allowed" {
		t.Fatalf("http action adapter was not filtered: %#v", httpActions)
	}
}

func TestRuntimeToolsFromConnectionsSummarizesAgentSideEntry(t *testing.T) {
	connection := agentRuntimeConnectionResponse{
		ID:             "conn-lark",
		Provider:       "lark",
		ConnectionName: "default",
		Runtime: connectionRuntimeSpec{
			Alias:              "lark",
			RecommendedAdapter: connector.RuntimeAdapterCLI,
			Adapters: []connector.ToolRuntimeAdapter{
				{Type: connector.RuntimeAdapterCLI, Priority: 100, Skills: []string{"lark-doc", "lark-im"}},
				{Type: connector.RuntimeAdapterHTTPAction, Priority: 10, HTTPAction: &connector.ToolHTTPActionAdapter{ActionNames: []string{"send_message"}}},
			},
			Actions: []connector.ProviderAction{{Name: "send_message", Method: "POST", Endpoint: "/open-apis/im/v1/messages"}},
		},
	}
	tools := runtimeToolsFromConnections([]agentRuntimeConnectionResponse{connection})
	if len(tools) != 1 {
		t.Fatalf("tools=%#v", tools)
	}
	if tools[0].RecommendedAdapter != connector.RuntimeAdapterCLI || tools[0].ConnectionAlias != "lark" {
		t.Fatalf("tool summary=%#v", tools[0])
	}
	if strings.Join(tools[0].Skills, ",") != "lark-doc,lark-im" {
		t.Fatalf("tool skills=%#v", tools[0].Skills)
	}
}

func TestRuntimeActionsForConnectionUsesProviderCatalogAndActionPolicy(t *testing.T) {
	users := newTestUserStore(t)
	s := &Server{controlDB: users.db, users: users}
	if err := users.db.UpsertWorkspace(controldb.Workspace{ID: "ws-one", Name: "One", Slug: "one"}); err != nil {
		t.Fatalf("workspace: %v", err)
	}
	provider := connector.Provider{
		Provider:    "catalog-test",
		DisplayName: "Catalog Test",
		AuthTypes:   []string{ConnectionAuthNoAuth},
		Actions: []connector.ProviderAction{
			{Name: "allowed", DisplayName: "Allowed", Method: "GET", Endpoint: "/v1/items"},
			{Name: "blocked", DisplayName: "Blocked", Method: "DELETE", Endpoint: "/v1/items/1"},
			{Name: "outside_endpoint", DisplayName: "Outside", Method: "GET", Endpoint: "/admin/items"},
		},
		Enabled: true,
	}
	authTypes, _ := json.Marshal(provider.AuthTypes)
	catalog, _ := json.Marshal(provider)
	if err := users.db.UpsertConnectorProvider(controldb.ConnectorProvider{
		Provider:      provider.Provider,
		DisplayName:   provider.DisplayName,
		AuthTypesJSON: string(authTypes),
		CatalogJSON:   string(catalog),
		Enabled:       true,
	}); err != nil {
		t.Fatalf("provider: %v", err)
	}
	connection := controldb.Connection{
		ID:             "conn-catalog",
		WorkspaceID:    "ws-one",
		Provider:       "catalog-test",
		ConnectionName: "default",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        "ws-one",
		AuthType:       ConnectionAuthNoAuth,
		Status:         "active",
		ProfileJSON:    `{"allowedActionMethods":["GET"],"allowedActionEndpoints":["/v1/*"]}`,
		CreatedBy:      "owner",
	}
	actions, err := s.runtimeActionsForConnection(connection)
	if err != nil {
		t.Fatalf("runtime actions: %v", err)
	}
	if len(actions) != 1 || actions[0].Name != "allowed" {
		t.Fatalf("unexpected runtime actions: %#v", actions)
	}
}

func TestRuntimeConnectionAlias(t *testing.T) {
	tests := map[string]struct {
		provider string
		name     string
		want     string
	}{
		"default connection": {provider: "github", name: "default", want: "github"},
		"named connection":   {provider: "custom-mcp", name: "Team Tools", want: "custom-mcp-team-tools"},
		"empty fallback":     {provider: " ", name: " ", want: "connection"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := runtimeConnectionAlias(tt.provider, tt.name); got != tt.want {
				t.Fatalf("runtimeConnectionAlias()=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveAgentRuntimeConnectionsUsesWorkspaceDefaultAndGrantRules(t *testing.T) {
	users := newTestUserStore(t)
	s := &Server{controlDB: users.db, users: users}
	workspaceID := "ws-one"
	if err := users.db.UpsertWorkspace(controldb.Workspace{ID: workspaceID, Name: "One", Slug: "one"}); err != nil {
		t.Fatalf("workspace: %v", err)
	}
	granted := controldb.Connection{
		ID:             "conn-granted",
		WorkspaceID:    workspaceID,
		Provider:       "custom-mcp",
		ConnectionName: "tools",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       ConnectionAuthNoAuth,
		Status:         "active",
		ProfileJSON:    `{"displayName":"Tools","token":"must-not-leak"}`,
	}
	ungranted := granted
	ungranted.ID = "conn-ungranted"
	ungranted.ConnectionName = "other"
	ungranted.OwnerType = ConnectionOwnerUser
	ungranted.OwnerID = "owner"
	workspaceDefault := granted
	workspaceDefault.ID = "conn-workspace-default"
	workspaceDefault.ConnectionName = "default-tools"
	if err := users.db.UpsertConnection(granted); err != nil {
		t.Fatalf("granted connection: %v", err)
	}
	if err := users.db.UpsertConnection(ungranted); err != nil {
		t.Fatalf("ungranted connection: %v", err)
	}
	if err := users.db.UpsertConnection(workspaceDefault); err != nil {
		t.Fatalf("workspace default connection: %v", err)
	}
	if err := users.db.CreateConnectionGrant(controldb.ConnectionGrant{
		ID:           "grant-project",
		WorkspaceID:  workspaceID,
		ConnectionID: granted.ID,
		TargetType:   ConnectionTargetProject,
		TargetID:     "sample",
	}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	connections, err := s.resolveAgentRuntimeConnections(workspaceID, "sample", "pm")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	gotIDs := map[string]bool{}
	for _, connection := range connections {
		gotIDs[connection.ID] = true
	}
	if len(connections) != 2 || !gotIDs[granted.ID] || !gotIDs[workspaceDefault.ID] {
		t.Fatalf("connections=%#v", connections)
	}
	for _, connection := range connections {
		if _, ok := connection.Profile["token"]; ok {
			t.Fatalf("runtime profile leaked token: %#v", connection.Profile)
		}
	}
}

func TestRuntimeConnectionsRequiresConnectionCapability(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/connections", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxRuntimeAgentKey, runtimeAgentPrincipal{
		WorkspaceID:  "ws-one",
		Project:      "sample",
		Agent:        "pm",
		Capabilities: []string{"task.read"},
	}))
	rec := httptest.NewRecorder()

	s.handleRuntimeConnections(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAgentRuntimeConnectionsRequireAgentOperatorAccess(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	grantProjectRoleForTest(t, s, workspaceID, "viewer", ProjectRoleViewer)
	grantProjectRoleForTest(t, s, workspaceID, "operator", ProjectRoleOperator)

	viewerReq := agentRuntimeConnectionsRequest("viewer", "sample", "pm")
	viewerRec := httptest.NewRecorder()
	s.handleAgentRuntimeConnections(viewerRec, viewerReq)
	if viewerRec.Code != http.StatusForbidden {
		t.Fatalf("viewer status=%d body=%s", viewerRec.Code, viewerRec.Body.String())
	}

	operatorReq := agentRuntimeConnectionsRequest("operator", "sample", "backend")
	operatorRec := httptest.NewRecorder()
	s.handleAgentRuntimeConnections(operatorRec, operatorReq)
	if operatorRec.Code != http.StatusOK {
		t.Fatalf("operator status=%d body=%s", operatorRec.Code, operatorRec.Body.String())
	}

	ownerReq := agentRuntimeConnectionsRequest("owner", "sample", "pm")
	ownerRec := httptest.NewRecorder()
	s.handleAgentRuntimeConnections(ownerRec, ownerReq)
	if ownerRec.Code != http.StatusOK {
		t.Fatalf("linked owner status=%d body=%s", ownerRec.Code, ownerRec.Body.String())
	}
}

func agentRuntimeConnectionsRequest(username, project, agent string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project+"/agents/"+agent+"/runtime/connections", nil)
	req.SetPathValue("name", project)
	req.SetPathValue("agent", agent)
	return req.WithContext(context.WithValue(req.Context(), ctxUserKey, username))
}
