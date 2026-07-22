package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/multigent/multigent/internal/entity"
)

type assistantChatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type assistantChatBody struct {
	Message string             `json:"message"`
	History []assistantChatMsg `json:"history"`
}

const (
	assistantSettingsTable = "assistant_settings"
	assistantSettingsKey   = "default"
	assistantModeControl   = "control_plane"
)

type assistantSettings struct {
	Enabled         bool   `json:"enabled"`
	Mode            string `json:"mode"`
	ModelProviderID string `json:"modelProviderId,omitempty"`
	UpdatedAt       string `json:"updatedAt,omitempty"`
	UpdatedBy       string `json:"updatedBy,omitempty"`
}

type assistantStatusResponse struct {
	Enabled           bool   `json:"enabled"`
	Mode              string `json:"mode"`
	Configured        bool   `json:"configured"`
	CanUse            bool   `json:"canUse"`
	CanAdmin          bool   `json:"canAdmin"`
	ModelProviderID   string `json:"modelProviderId,omitempty"`
	ModelProviderName string `json:"modelProviderName,omitempty"`
	Reason            string `json:"reason,omitempty"`
}

type assistantSettingsBody struct {
	Enabled         *bool  `json:"enabled"`
	ModelProviderID string `json:"modelProviderId"`
}

func (s *Server) handleAssistantStatus(w http.ResponseWriter, r *http.Request) {
	status, ok := s.assistantStatus(w, r)
	if !ok {
		return
	}
	_ = json.NewEncoder(w).Encode(status)
}

func (s *Server) handleAssistantSettings(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !s.checkWorkspaceAccess(w, r, workspaceID) {
		return
	}
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	if s.controlDB == nil {
		s.jsonErrorCode(w, http.StatusServiceUnavailable, ErrCodeWorkspaceDatabaseUnavailable, "control database unavailable")
		return
	}

	var body assistantSettingsBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	providerID := strings.TrimSpace(body.ModelProviderID)
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	if providerID == "" && enabled {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeAssistantModelRequired, "assistant model provider is required")
		return
	}
	if providerID != "" {
		provider, err := s.providerStore().Get(providerID)
		if err != nil {
			s.jsonErrorCode(w, http.StatusNotFound, ErrCodeProviderNotFound, "model provider not found")
			return
		}
		if provider.OwnerType != "" && provider.OwnerType != ConnectionOwnerWorkspace {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeAssistantProviderUnsupported, "assistant requires a workspace model provider")
			return
		}
		if provider.OwnerID != "" && provider.OwnerID != workspaceID {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeAssistantProviderInvalid, "model provider belongs to another workspace")
			return
		}
		if strings.TrimSpace(provider.APIKey) == "" && len(provider.Env) == 0 {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeAssistantModelRequired, "model provider key is required")
			return
		}
		if strings.TrimSpace(provider.Model) == "" && !assistantProviderHasModelEnv(*provider) {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeAssistantModelRequired, "model provider model is required")
			return
		}
	}
	cur := s.currentUser(r)
	updatedBy := ""
	if cur != nil {
		updatedBy = cur.Username
	}
	settings := assistantSettings{
		Enabled:         enabled,
		Mode:            assistantModeControl,
		ModelProviderID: providerID,
		UpdatedAt:       time.Now().UTC().Format(time.RFC3339),
		UpdatedBy:       updatedBy,
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if err := s.controlDB.UpsertRecord(assistantSettingsTable, workspaceID, []string{assistantSettingsKey}, string(raw)); err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "assistant.settings.update",
		ResourceType: "assistant",
		ResourceID:   assistantSettingsKey,
		Summary:      "Control-plane assistant settings updated",
		After:        map[string]any{"enabled": settings.Enabled, "mode": settings.Mode, "modelProviderId": settings.ModelProviderID},
		Request:      r,
	})
	status, ok := s.assistantStatus(w, r)
	if !ok {
		return
	}
	_ = json.NewEncoder(w).Encode(status)
}

func (s *Server) handleAssistantChat(w http.ResponseWriter, r *http.Request) {
	status, ok := s.assistantStatus(w, r)
	if !ok {
		return
	}
	if !status.CanAdmin {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeWorkspaceAdminRequired, "workspace admin access required")
		return
	}
	if !status.CanUse {
		code := ErrCodeAssistantModelRequired
		if status.Reason == "disabled" {
			code = ErrCodeForbidden
		}
		s.jsonErrorCode(w, http.StatusConflict, code, status.Reason)
		return
	}

	var body assistantChatBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	msg := strings.TrimSpace(body.Message)
	if msg == "" {
		s.jsonError(w, http.StatusBadRequest, "message is required")
		return
	}

	provider, err := s.providerStore().Get(status.ModelProviderID)
	if err != nil {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeProviderNotFound, "model provider not found")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	response, err := s.invokeAssistantModel(ctx, *provider, body.History, msg)
	if err != nil {
		s.jsonErrorCode(w, http.StatusBadGateway, ErrCodeUpstreamError, err.Error())
		return
	}
	if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		s.assistantControlPlaneStream(w, response)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"response": response})
}

func (s *Server) assistantStatus(w http.ResponseWriter, r *http.Request) (assistantStatusResponse, bool) {
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return assistantStatusResponse{}, false
	}
	if !s.checkWorkspaceAccess(w, r, workspaceID) {
		return assistantStatusResponse{}, false
	}
	if s.controlDB == nil {
		s.jsonErrorCode(w, http.StatusServiceUnavailable, ErrCodeWorkspaceDatabaseUnavailable, "control database unavailable")
		return assistantStatusResponse{}, false
	}
	canAdmin := s.canAdminWorkspace(r, workspaceID)
	settings, err := s.loadAssistantSettings(workspaceID)
	if err != nil {
		s.serverError(w, err)
		return assistantStatusResponse{}, false
	}
	status := assistantStatusResponse{
		Enabled:         settings.Enabled,
		Mode:            settings.Mode,
		CanAdmin:        canAdmin,
		ModelProviderID: settings.ModelProviderID,
	}
	if !canAdmin {
		status.Reason = "workspace_admin_required"
		return status, true
	}
	if !settings.Enabled {
		status.Reason = "disabled"
		return status, true
	}
	if strings.TrimSpace(settings.ModelProviderID) == "" {
		status.Reason = "model_provider_required"
		return status, true
	}
	provider, err := s.providerStore().Get(settings.ModelProviderID)
	if err != nil {
		status.Reason = "model_provider_missing"
		return status, true
	}
	if provider.OwnerType != "" && provider.OwnerType != ConnectionOwnerWorkspace {
		status.Reason = "workspace_provider_required"
		return status, true
	}
	if provider.OwnerID != "" && provider.OwnerID != workspaceID {
		status.Reason = "workspace_provider_required"
		return status, true
	}
	status.ModelProviderName = provider.Name
	status.Configured = strings.TrimSpace(provider.APIKey) != "" || len(provider.Env) > 0
	if !status.Configured {
		status.Reason = "model_provider_key_required"
		return status, true
	}
	if strings.TrimSpace(provider.Model) == "" && !assistantProviderHasModelEnv(*provider) {
		status.Configured = false
		status.Reason = "model_provider_model_required"
		return status, true
	}
	status.CanUse = true
	return status, true
}

func assistantProviderHasModelEnv(provider entity.APIProvider) bool {
	for _, key := range []string{"OPENAI_MODEL", "ANTHROPIC_MODEL", "CLAUDE_MODEL", "GEMINI_MODEL", "GOOGLE_MODEL", "CURSOR_MODEL"} {
		if strings.TrimSpace(provider.Env[key]) != "" {
			return true
		}
	}
	return false
}

func (s *Server) loadAssistantSettings(workspaceID string) (assistantSettings, error) {
	settings := assistantSettings{Enabled: true, Mode: assistantModeControl}
	raw, ok, err := s.controlDB.GetRecord(assistantSettingsTable, workspaceID, []string{assistantSettingsKey})
	if err != nil || !ok {
		return settings, err
	}
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return assistantSettings{Enabled: true, Mode: assistantModeControl}, err
	}
	if settings.Mode == "" {
		settings.Mode = assistantModeControl
	}
	return settings, nil
}

func (s *Server) assistantControlPlaneStream(w http.ResponseWriter, response string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.jsonError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	payload, _ := json.Marshal(map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"role":    "assistant",
			"content": []map[string]string{{"type": "text", "text": response}},
		},
	})
	fmt.Fprintf(w, "data: %s\n\n", payload)
	fmt.Fprint(w, "data: {\"type\":\"done\"}\n\n")
	flusher.Flush()
}

type assistantLLMMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (s *Server) invokeAssistantModel(ctx context.Context, provider entity.APIProvider, history []assistantChatMsg, message string) (string, error) {
	messages := make([]assistantLLMMessage, 0, len(history)+2)
	systemPrompt := "你是 Multigent 的智能助手。你的职责是帮助工作区管理员理解和配置团队、项目、Agent、流程、任务、模型账号和外部工具。回答要简洁、直接、可执行。当前阶段你只能提供建议和解释，不要声称已经替用户完成创建、删除、发布、转账或其他写操作。"
	for _, item := range history {
		role := strings.TrimSpace(item.Role)
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		messages = append(messages, assistantLLMMessage{Role: role, Content: content})
	}
	messages = append(messages, assistantLLMMessage{Role: "user", Content: message})
	switch strings.ToLower(strings.TrimSpace(provider.Type)) {
	case "anthropic":
		return assistantAnthropicChat(ctx, provider, systemPrompt, messages)
	case "openai", "cursor", "custom", "":
		return assistantOpenAIChat(ctx, provider, systemPrompt, messages)
	default:
		return "", fmt.Errorf("智能助手暂不支持该模型账号类型：%s", provider.Type)
	}
}

func assistantOpenAIChat(ctx context.Context, provider entity.APIProvider, systemPrompt string, messages []assistantLLMMessage) (string, error) {
	model := strings.TrimSpace(provider.Model)
	if model == "" {
		model = strings.TrimSpace(provider.Env["OPENAI_MODEL"])
	}
	if model == "" {
		return "", fmt.Errorf("智能助手模型账号 %q 缺少默认模型，请在模型账号中填写模型名称", provider.Name)
	}
	apiKey := strings.TrimSpace(provider.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(provider.Env["OPENAI_API_KEY"])
	}
	if apiKey == "" {
		return "", fmt.Errorf("智能助手模型账号 %q 缺少 API Key", provider.Name)
	}
	endpoint := assistantOpenAIEndpoint(provider.BaseURL)
	reqMessages := append([]assistantLLMMessage{{Role: "system", Content: systemPrompt}}, messages...)
	reqBody := map[string]any{
		"model":    model,
		"messages": reqMessages,
		"stream":   false,
	}
	var respBody struct {
		Choices []struct {
			Message assistantLLMMessage `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type,omitempty"`
		} `json:"error,omitempty"`
	}
	if err := assistantPostJSON(ctx, endpoint, apiKey, nil, reqBody, &respBody); err != nil {
		return "", err
	}
	if respBody.Error != nil {
		return "", fmt.Errorf("模型服务错误：%s", respBody.Error.Message)
	}
	if len(respBody.Choices) == 0 {
		return "", fmt.Errorf("模型服务没有返回内容")
	}
	text := strings.TrimSpace(respBody.Choices[0].Message.Content)
	if text == "" {
		return "", fmt.Errorf("模型服务返回了空内容")
	}
	return text, nil
}

func assistantAnthropicChat(ctx context.Context, provider entity.APIProvider, systemPrompt string, messages []assistantLLMMessage) (string, error) {
	model := strings.TrimSpace(provider.Model)
	if model == "" {
		model = strings.TrimSpace(provider.Env["ANTHROPIC_MODEL"])
	}
	if model == "" {
		return "", fmt.Errorf("智能助手模型账号 %q 缺少默认模型，请在模型账号中填写模型名称", provider.Name)
	}
	apiKey := strings.TrimSpace(provider.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(provider.Env["ANTHROPIC_API_KEY"])
	}
	if apiKey == "" {
		apiKey = strings.TrimSpace(provider.Env["ANTHROPIC_AUTH_TOKEN"])
	}
	if apiKey == "" {
		return "", fmt.Errorf("智能助手模型账号 %q 缺少 API Key", provider.Name)
	}
	endpoint := assistantAnthropicEndpoint(provider.BaseURL)
	reqBody := map[string]any{
		"model":      model,
		"system":     systemPrompt,
		"messages":   messages,
		"max_tokens": 2048,
		"stream":     false,
	}
	var respBody struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type,omitempty"`
		} `json:"error,omitempty"`
	}
	headers := map[string]string{"anthropic-version": "2023-06-01"}
	if err := assistantPostJSON(ctx, endpoint, apiKey, headers, reqBody, &respBody); err != nil {
		return "", err
	}
	if respBody.Error != nil {
		return "", fmt.Errorf("模型服务错误：%s", respBody.Error.Message)
	}
	var parts []string
	for _, block := range respBody.Content {
		if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
			parts = append(parts, strings.TrimSpace(block.Text))
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("模型服务没有返回内容")
	}
	return strings.Join(parts, "\n"), nil
}

func assistantPostJSON(ctx context.Context, endpoint, apiKey string, headers map[string]string, body any, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("构造模型请求失败：%w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("构造模型请求失败：%w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	if _, ok := headers["x-api-key"]; ok {
		req.Header.Del("Authorization")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	if strings.Contains(endpoint, "anthropic.com") && req.Header.Get("x-api-key") == "" {
		req.Header.Del("Authorization")
		req.Header.Set("x-api-key", apiKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求模型服务失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		rawResp, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("模型服务返回 HTTP %d：%s", resp.StatusCode, strings.TrimSpace(string(rawResp)))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("解析模型响应失败：%w", err)
	}
	return nil
}

func assistantOpenAIEndpoint(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	if strings.HasSuffix(base, "/chat/completions") {
		return base
	}
	return base + "/chat/completions"
}

func assistantAnthropicEndpoint(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = "https://api.anthropic.com"
	}
	if strings.HasSuffix(base, "/v1/messages") {
		return base
	}
	return base + "/v1/messages"
}
