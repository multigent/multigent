package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/multigent/multigent/internal/connector"
	controldb "github.com/multigent/multigent/internal/db"
)

func TestConnectionResponseSanitizesProfileSecrets(t *testing.T) {
	connection := controldb.Connection{
		ID:             "conn-one",
		Provider:       "custom-mcp",
		ConnectionName: "tools",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        "ws-one",
		AuthType:       ConnectionAuthCustomCredential,
		Status:         "active",
		ProfileJSON:    `{"displayName":"Tools","serverUrl":"http://localhost:3000/mcp","token":"secret-token","appSecret":"secret-app","credential":"secret-credential"}`,
		CreatedBy:      "admin",
	}
	resp := connectionToResponse(connection, nil)
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	body := string(raw)
	for _, forbidden := range []string{"secret-token", "secret-app", "secret-credential", "appSecret"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("connection response leaked %q: %s", forbidden, body)
		}
	}
	if _, ok := resp.Profile["credential"]; ok {
		t.Fatalf("credential key was not removed from profile: %#v", resp.Profile)
	}
	if resp.Profile["displayName"] != "Tools" || resp.Profile["serverUrl"] != "http://localhost:3000/mcp" {
		t.Fatalf("safe profile fields not preserved: %#v", resp.Profile)
	}
}

func TestConnectionAuditPayloadSanitizesProfileSecrets(t *testing.T) {
	connection := controldb.Connection{
		ID:             "conn-one",
		Provider:       "github",
		ConnectionName: "default",
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        "ws-one",
		AuthType:       ConnectionAuthAPIKey,
		Status:         "active",
		ProfileJSON:    `{"displayName":"GitHub","apiKey":"ghp_secret","token":"secret-token"}`,
		CreatedBy:      "admin",
	}
	payload := connectionAuditPayload(connection)
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	body := string(raw)
	for _, forbidden := range []string{"ghp_secret", "secret-token", "apiKey"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("audit payload leaked %q: %s", forbidden, body)
		}
	}
	profile, ok := payload["profile"].(map[string]any)
	if !ok || profile["displayName"] != "GitHub" {
		t.Fatalf("safe audit profile not preserved: %#v", payload["profile"])
	}
}

func TestConnectorProviderFromDBDrivesCredentialValidation(t *testing.T) {
	catalog, err := json.Marshal(connector.Provider{
		Provider:    "custom-test",
		DisplayName: "Custom Test",
		AuthTypes:   []string{ConnectionAuthCustomCredential},
		Fields: []connector.ProviderField{
			{Key: "serverUrl", Label: "Server URL", Required: true},
			{Key: "token", Label: "Token", Secret: true},
		},
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("marshal catalog: %v", err)
	}
	provider, err := connectorProviderFromDB(controldb.ConnectorProvider{
		Provider:      "custom-test",
		DisplayName:   "Custom Test",
		AuthTypesJSON: `["custom_credential"]`,
		CatalogJSON:   string(catalog),
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("decode provider: %v", err)
	}
	if !providerSupportsAuth(provider, ConnectionAuthCustomCredential) {
		t.Fatalf("custom auth should be supported")
	}
	if err := validateConnectionValues(provider, ConnectionAuthCustomCredential, map[string]string{"token": "t"}); err == nil {
		t.Fatalf("missing required serverUrl should fail")
	}
	if err := validateConnectionValues(provider, ConnectionAuthCustomCredential, map[string]string{"serverUrl": "http://localhost:3000/mcp", "extra": "x"}); err == nil {
		t.Fatalf("unknown credential field should fail")
	}
	if err := validateConnectionValues(provider, ConnectionAuthCustomCredential, map[string]string{"serverUrl": "http://localhost:3000/mcp", "token": "t"}); err != nil {
		t.Fatalf("valid values should pass: %v", err)
	}
}

func TestConnectionByIDRequiresCurrentWorkspace(t *testing.T) {
	db, err := controldb.Open(filepath.Join(t.TempDir(), "multigent.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	currentRoot := filepath.Join(t.TempDir(), "current")
	otherRoot := filepath.Join(t.TempDir(), "other")
	for _, workspace := range []controldb.Workspace{
		{ID: "ws-current", Name: "Current", Slug: "current", Root: currentRoot, CreatedAt: "2026-07-15T00:00:00Z"},
		{ID: "ws-other", Name: "Other", Slug: "other", Root: otherRoot, CreatedAt: "2026-07-15T00:00:00Z"},
	} {
		if err := db.UpsertWorkspace(workspace); err != nil {
			t.Fatalf("workspace %s: %v", workspace.ID, err)
		}
	}
	if err := db.UpsertUser(controldb.User{Username: "owner", CreatedAt: "2026-07-15T00:00:00Z"}); err != nil {
		t.Fatalf("user: %v", err)
	}
	if err := db.UpsertWorkspaceMember("ws-current", "owner", WorkspaceRoleMember); err != nil {
		t.Fatalf("current member: %v", err)
	}
	if err := db.UpsertWorkspaceMember("ws-other", "owner", WorkspaceRoleMember); err != nil {
		t.Fatalf("other member: %v", err)
	}
	if err := db.UpsertConnection(controldb.Connection{
		ID:             "conn-other",
		WorkspaceID:    "ws-other",
		Provider:       "github",
		ConnectionName: "default",
		OwnerType:      ConnectionOwnerUser,
		OwnerID:        "owner",
		AuthType:       ConnectionAuthAPIKey,
		Status:         "active",
		ProfileJSON:    `{}`,
		CreatedAt:      "2026-07-15T00:00:00Z",
		UpdatedAt:      "2026-07-15T00:00:00Z",
	}); err != nil {
		t.Fatalf("connection: %v", err)
	}

	s := &Server{root: currentRoot, controlDB: db, users: newUserStore(db)}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/connections/conn-other", nil)
	req.SetPathValue("id", "conn-other")
	req = req.WithContext(context.WithValue(req.Context(), ctxUserKey, "owner"))
	rec := httptest.NewRecorder()

	s.handleGetConnection(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpdateConnectionKeepsOrReplacesSecretsByOwner(t *testing.T) {
	s, workspaceID := newConnectionGrantPolicyServer(t)
	connection := controldb.Connection{
		ID:             "conn-owner",
		WorkspaceID:    workspaceID,
		Provider:       "custom-http",
		ConnectionName: "api",
		OwnerType:      ConnectionOwnerUser,
		OwnerID:        "owner",
		AuthType:       ConnectionAuthCustomCredential,
		Status:         "active",
		ProfileJSON:    `{"displayName":"Old API","provider":"custom-http","connectionName":"api"}`,
		CreatedBy:      "owner",
		CreatedAt:      "2026-07-15T00:00:00Z",
		UpdatedAt:      "2026-07-15T00:00:00Z",
	}
	if err := s.controlDB.UpsertConnection(connection); err != nil {
		t.Fatalf("connection: %v", err)
	}
	secret, err := sealConnectionSecret(map[string]string{"baseUrl": "https://old.example.test", "apiKey": "old-key"})
	if err != nil {
		t.Fatalf("seal secret: %v", err)
	}
	secret.ConnectionID = connection.ID
	if err := s.controlDB.UpsertConnectionSecret(secret); err != nil {
		t.Fatalf("secret: %v", err)
	}

	adminRec := httptest.NewRecorder()
	adminReq := providerTestRequest(http.MethodPut, "/api/v1/connections/conn-owner", "admin", createConnectionRequest{
		ConnectionName: "admin-edit",
	})
	adminReq.SetPathValue("id", "conn-owner")
	s.handleUpdateConnection(adminRec, adminReq)
	if adminRec.Code != http.StatusForbidden {
		t.Fatalf("admin update status=%d body=%s", adminRec.Code, adminRec.Body.String())
	}

	ownerRec := httptest.NewRecorder()
	ownerReq := providerTestRequest(http.MethodPut, "/api/v1/connections/conn-owner", "owner", createConnectionRequest{
		ConnectionName: "api-v2",
		AuthType:       ConnectionAuthCustomCredential,
		Values:         map[string]string{"apiKey": "new-key"},
		Profile:        map[string]any{"displayName": "New API", "apiKey": "should-not-leak"},
	})
	ownerReq.SetPathValue("id", "conn-owner")
	s.handleUpdateConnection(ownerRec, ownerReq)
	if ownerRec.Code != http.StatusOK {
		t.Fatalf("owner update status=%d body=%s", ownerRec.Code, ownerRec.Body.String())
	}
	body := ownerRec.Body.String()
	if strings.Contains(body, "new-key") || strings.Contains(body, "should-not-leak") {
		t.Fatalf("update response leaked secret: %s", body)
	}
	updated, ok, err := s.controlDB.ConnectionByID("conn-owner")
	if err != nil || !ok {
		t.Fatalf("updated connection ok=%v err=%v", ok, err)
	}
	if updated.ConnectionName != "api-v2" {
		t.Fatalf("connection name=%q", updated.ConnectionName)
	}
	updatedSecret, ok, err := s.controlDB.ConnectionSecret("conn-owner")
	if err != nil || !ok {
		t.Fatalf("updated secret ok=%v err=%v", ok, err)
	}
	values, err := openConnectionSecret(updatedSecret)
	if err != nil {
		t.Fatalf("open secret: %v", err)
	}
	if values["apiKey"] != "new-key" || values["baseUrl"] != "https://old.example.test" {
		t.Fatalf("secret values=%#v", values)
	}
}
