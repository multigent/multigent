package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
)

func TestAssistantStatusRequiresWorkspaceAdminAndModelProvider(t *testing.T) {
	s, _ := newProviderHandlerTestServer(t)

	memberRec := httptest.NewRecorder()
	s.handleAssistantStatus(memberRec, providerTestRequest(http.MethodGet, "/api/v1/assistant/status", "member", nil))
	if memberRec.Code != http.StatusOK {
		t.Fatalf("member status code=%d body=%s", memberRec.Code, memberRec.Body.String())
	}
	var memberStatus assistantStatusResponse
	if err := json.Unmarshal(memberRec.Body.Bytes(), &memberStatus); err != nil {
		t.Fatalf("decode member status: %v", err)
	}
	if memberStatus.CanUse || memberStatus.CanAdmin || memberStatus.Reason != "workspace_admin_required" {
		t.Fatalf("unexpected member status: %#v", memberStatus)
	}

	ownerRec := httptest.NewRecorder()
	s.handleAssistantStatus(ownerRec, providerTestRequest(http.MethodGet, "/api/v1/assistant/status", "owner", nil))
	if ownerRec.Code != http.StatusOK {
		t.Fatalf("owner status code=%d body=%s", ownerRec.Code, ownerRec.Body.String())
	}
	var ownerStatus assistantStatusResponse
	if err := json.Unmarshal(ownerRec.Body.Bytes(), &ownerStatus); err != nil {
		t.Fatalf("decode owner status: %v", err)
	}
	if ownerStatus.CanUse || !ownerStatus.CanAdmin || ownerStatus.Reason != "model_provider_required" {
		t.Fatalf("unexpected owner status: %#v", ownerStatus)
	}
}

func TestAssistantSettingsUsesWorkspaceProviderOnly(t *testing.T) {
	s, _ := newProviderHandlerTestServer(t)

	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.controlDB.UpsertModelProvider("ws-one", controldb.ModelProvider{
		ID:          "personal-leftover",
		WorkspaceID: "ws-one",
		OwnerType:   ConnectionOwnerUser,
		OwnerID:     "member",
		Name:        "Personal",
		Type:        "openai",
		APIKey:      "sealed:test",
		EnvJSON:     "{}",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("insert personal provider: %v", err)
	}

	memberSettingsRec := httptest.NewRecorder()
	s.handleAssistantSettings(memberSettingsRec, providerTestRequest(http.MethodPut, "/api/v1/assistant/settings", "member", assistantSettingsBody{
		ModelProviderID: "personal-leftover",
	}))
	if memberSettingsRec.Code != http.StatusForbidden {
		t.Fatalf("member settings code=%d body=%s", memberSettingsRec.Code, memberSettingsRec.Body.String())
	}

	ownerPersonalRec := httptest.NewRecorder()
	s.handleAssistantSettings(ownerPersonalRec, providerTestRequest(http.MethodPut, "/api/v1/assistant/settings", "owner", assistantSettingsBody{
		ModelProviderID: "personal-leftover",
	}))
	if ownerPersonalRec.Code != http.StatusNotFound {
		t.Fatalf("owner personal settings code=%d body=%s", ownerPersonalRec.Code, ownerPersonalRec.Body.String())
	}

	workspaceRec := httptest.NewRecorder()
	s.handleAddProvider(workspaceRec, providerTestRequest(http.MethodPost, "/api/v1/providers", "owner", providerBody{
		Name:   "Workspace OpenAI",
		Type:   "openai",
		APIKey: "sk-workspace",
		Model:  "gpt-test",
	}))
	if workspaceRec.Code != http.StatusOK {
		t.Fatalf("create workspace provider code=%d body=%s", workspaceRec.Code, workspaceRec.Body.String())
	}
	var workspace map[string]any
	_ = json.Unmarshal(workspaceRec.Body.Bytes(), &workspace)
	workspaceID, _ := workspace["id"].(string)

	ownerSettingsRec := httptest.NewRecorder()
	s.handleAssistantSettings(ownerSettingsRec, providerTestRequest(http.MethodPut, "/api/v1/assistant/settings", "owner", assistantSettingsBody{
		ModelProviderID: workspaceID,
	}))
	if ownerSettingsRec.Code != http.StatusOK {
		t.Fatalf("owner settings code=%d body=%s", ownerSettingsRec.Code, ownerSettingsRec.Body.String())
	}
	var status assistantStatusResponse
	if err := json.Unmarshal(ownerSettingsRec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode assistant status: %v", err)
	}
	if !status.CanUse || !status.Configured || status.ModelProviderID != workspaceID || status.ModelProviderName != "Workspace OpenAI" {
		t.Fatalf("unexpected configured status: %#v", status)
	}
}

func TestAssistantChatUsesConfiguredModelProvider(t *testing.T) {
	s, _ := newProviderHandlerTestServer(t)
	var captured map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-workspace" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "模型真实回复"}},
			},
		})
	}))
	defer upstream.Close()

	workspaceRec := httptest.NewRecorder()
	s.handleAddProvider(workspaceRec, providerTestRequest(http.MethodPost, "/api/v1/providers", "owner", providerBody{
		Name:    "Workspace OpenAI",
		Type:    "openai",
		BaseURL: upstream.URL + "/v1",
		APIKey:  "sk-workspace",
		Model:   "gpt-test",
	}))
	var workspace map[string]any
	_ = json.Unmarshal(workspaceRec.Body.Bytes(), &workspace)
	workspaceID, _ := workspace["id"].(string)
	settingsRec := httptest.NewRecorder()
	s.handleAssistantSettings(settingsRec, providerTestRequest(http.MethodPut, "/api/v1/assistant/settings", "owner", assistantSettingsBody{
		ModelProviderID: workspaceID,
	}))
	if settingsRec.Code != http.StatusOK {
		t.Fatalf("settings code=%d body=%s", settingsRec.Code, settingsRec.Body.String())
	}

	chatRec := httptest.NewRecorder()
	req := providerTestRequest(http.MethodPost, "/api/v1/assistant/chat", "owner", assistantChatBody{Message: "hello"})
	req.Header.Set("Accept", "text/event-stream")
	s.handleAssistantChat(chatRec, req)
	if chatRec.Code != http.StatusOK {
		t.Fatalf("chat code=%d body=%s", chatRec.Code, chatRec.Body.String())
	}
	body := chatRec.Body.String()
	if !strings.Contains(body, "模型真实回复") || strings.Contains(body, "Command:") {
		t.Fatalf("unexpected chat stream: %s", body)
	}
	if captured["model"] != "gpt-test" {
		t.Fatalf("upstream request missing model: %#v", captured)
	}
}
