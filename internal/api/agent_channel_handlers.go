package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/imbridge"
)

type agentChannelResponse struct {
	ID              string `json:"id"`
	Provider        string `json:"provider"`
	Status          string `json:"status"`
	ConnectionID    string `json:"connectionId,omitempty"`
	CallbackURL     string `json:"callbackUrl,omitempty"`
	AppID           string `json:"appId,omitempty"`
	AccountsURL     string `json:"accountsUrl,omitempty"`
	ExternalBotID   string `json:"externalBotId,omitempty"`
	ExternalChatID  string `json:"externalChatId,omitempty"`
	ExternalOwnerID string `json:"externalOwnerId,omitempty"`
	Security        struct {
		VerificationTokenConfigured bool `json:"verificationTokenConfigured"`
		EncryptKeyConfigured        bool `json:"encryptKeyConfigured"`
	} `json:"security"`
	Callback struct {
		LastAt    string `json:"lastAt,omitempty"`
		Status    string `json:"status,omitempty"`
		Reason    string `json:"reason,omitempty"`
		MessageID string `json:"messageId,omitempty"`
		Error     string `json:"error,omitempty"`
	} `json:"callback"`
	CreatedBy      string `json:"createdBy,omitempty"`
	CreatedAt      string `json:"createdAt,omitempty"`
	UpdatedAt      string `json:"updatedAt,omitempty"`
	LastActivityAt string `json:"lastActivityAt,omitempty"`
}

type channelSetupPollRequest struct {
	DeviceCode string `json:"deviceCode"`
	BaseURL    string `json:"baseUrl"`
}

type agentChannelSecurityRequest struct {
	VerificationToken *string `json:"verificationToken"`
	EncryptKey        *string `json:"encryptKey"`
}

func (s *Server) handleAgentChannels(w http.ResponseWriter, r *http.Request) {
	project, agent, ok := s.parseProjectAgent(w, r)
	if !ok {
		return
	}
	if !s.checkProjectAccess(w, r, project) {
		return
	}
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}
	bindings, err := s.controlDB.ListAgentChannelBindings(controldb.AgentChannelBindingFilter{
		WorkspaceID: workspaceID,
		ProjectID:   project,
		AgentID:     agent,
	})
	if err != nil {
		s.serverError(w, err)
		return
	}
	out := make([]agentChannelResponse, 0, len(bindings))
	for _, binding := range bindings {
		resp := agentChannelToResponse(binding)
		resp.CallbackURL = requestBaseURL(r) + "/api/v1/im/" + binding.Provider + "/events"
		if secret, ok, err := s.controlDB.ConnectionSecret(binding.ConnectionID); err == nil && ok {
			if values, err := openConnectionSecret(secret); err == nil {
				resp.Security.VerificationTokenConfigured = strings.TrimSpace(values["verificationToken"]) != ""
				resp.Security.EncryptKeyConfigured = strings.TrimSpace(values["encryptKey"]) != ""
			}
		}
		out = append(out, resp)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"channels":  out,
		"providers": imbridge.Providers(),
	})
}

func (s *Server) handleAgentChannelDelete(w http.ResponseWriter, r *http.Request) {
	project, agent, provider, ok := s.parseProjectAgentProvider(w, r)
	if !ok {
		return
	}
	if !s.canOperateAgent(r, project, agent) {
		s.jsonError(w, http.StatusForbidden, "agent operator access required")
		return
	}
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}
	binding, found, err := s.findAgentChannelBinding(workspaceID, project, agent, provider)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		return
	}
	if err := s.controlDB.DeleteAgentChannelBinding(binding.ID); err != nil {
		s.serverError(w, err)
		return
	}
	go s.refreshAgentIMBridges()
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "agent_channel.disconnect",
		ResourceType: "agent_channel",
		ResourceID:   binding.ID,
		Summary:      fmt.Sprintf("Disconnected %s channel for %s/%s", provider, project, agent),
		Before:       agentChannelToResponse(binding),
		Request:      r,
	})
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *Server) handleAgentChannelSetupBegin(w http.ResponseWriter, r *http.Request) {
	project, agent, provider, ok := s.parseProjectAgentProvider(w, r)
	if !ok {
		return
	}
	if !s.canOperateAgent(r, project, agent) {
		s.jsonError(w, http.StatusForbidden, "agent operator access required")
		return
	}
	if _, ok := s.currentWorkspaceForRequest(w, r); !ok {
		return
	}
	channelProvider, ok := imbridge.LookupProvider(provider)
	if !ok {
		s.jsonError(w, http.StatusBadRequest, "unsupported channel provider")
		return
	}
	ctx, cancel := contextWithRequestTimeout(r, 20*time.Second)
	defer cancel()
	resp, err := channelProvider.BeginSetup(ctx)
	if err != nil {
		s.jsonError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.auditLog(auditLogInput{
		Action:       "agent_channel.setup_begin",
		ResourceType: "agent",
		ResourceID:   project + "/" + agent,
		Summary:      fmt.Sprintf("Started %s channel setup for %s/%s", provider, project, agent),
		After: map[string]any{
			"provider": provider,
			"baseUrl":  resp.BaseURL,
		},
		Request: r,
	})
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleAgentChannelSecurity(w http.ResponseWriter, r *http.Request) {
	project, agent, provider, ok := s.parseProjectAgentProvider(w, r)
	if !ok {
		return
	}
	if !s.canOperateAgent(r, project, agent) {
		s.jsonError(w, http.StatusForbidden, "agent operator access required")
		return
	}
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}
	var req agentChannelSecurityRequest
	if err := s.readJSON(w, r, &req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	binding, found, err := s.findAgentChannelBinding(workspaceID, project, agent, provider)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found {
		s.jsonError(w, http.StatusNotFound, "agent channel is not connected")
		return
	}
	secret, found, err := s.controlDB.ConnectionSecret(binding.ConnectionID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	values := map[string]string{}
	if found {
		values, err = openConnectionSecret(secret)
		if err != nil {
			s.serverError(w, err)
			return
		}
	}
	if req.VerificationToken != nil {
		values["verificationToken"] = strings.TrimSpace(*req.VerificationToken)
	}
	if req.EncryptKey != nil {
		values["encryptKey"] = strings.TrimSpace(*req.EncryptKey)
	}
	next, err := sealConnectionSecret(values)
	if err != nil {
		s.serverError(w, err)
		return
	}
	next.ConnectionID = binding.ConnectionID
	if err := s.controlDB.UpsertConnectionSecret(next); err != nil {
		s.serverError(w, err)
		return
	}
	binding.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := s.controlDB.UpsertAgentChannelBinding(binding); err != nil {
		s.serverError(w, err)
		return
	}
	resp := agentChannelToResponse(binding)
	resp.CallbackURL = requestBaseURL(r) + "/api/v1/im/" + binding.Provider + "/events"
	resp.Security.VerificationTokenConfigured = strings.TrimSpace(values["verificationToken"]) != ""
	resp.Security.EncryptKeyConfigured = strings.TrimSpace(values["encryptKey"]) != ""
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "agent_channel.security_updated",
		ResourceType: "agent_channel",
		ResourceID:   binding.ID,
		Summary:      fmt.Sprintf("Updated %s channel security for %s/%s", provider, project, agent),
		After: map[string]any{
			"provider":                    provider,
			"verificationTokenConfigured": resp.Security.VerificationTokenConfigured,
			"encryptKeyConfigured":        resp.Security.EncryptKeyConfigured,
		},
		Request: r,
	})
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleAgentChannelSetupPoll(w http.ResponseWriter, r *http.Request) {
	project, agent, provider, ok := s.parseProjectAgentProvider(w, r)
	if !ok {
		return
	}
	if !s.canOperateAgent(r, project, agent) {
		s.jsonError(w, http.StatusForbidden, "agent operator access required")
		return
	}
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}
	var req channelSetupPollRequest
	if err := s.readJSON(w, r, &req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	channelProvider, ok := imbridge.LookupProvider(provider)
	if !ok {
		s.jsonError(w, http.StatusBadRequest, "unsupported channel provider")
		return
	}
	ctx, cancel := contextWithRequestTimeout(r, 20*time.Second)
	defer cancel()
	poll, err := channelProvider.PollSetup(ctx, req.DeviceCode, req.BaseURL)
	if err != nil {
		s.jsonError(w, http.StatusBadGateway, err.Error())
		return
	}
	if poll.Status != "completed" {
		_ = json.NewEncoder(w).Encode(poll)
		return
	}
	actualProvider := poll.Provider
	if actualProvider == "" {
		actualProvider = provider
	}
	binding, err := s.saveAgentIMChannel(r, workspaceID, project, agent, actualProvider, poll)
	if err != nil {
		s.serverError(w, err)
		return
	}
	resp := map[string]any{
		"status":  "connected",
		"baseUrl": poll.BaseURL,
		"channel": agentChannelToResponse(binding),
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) saveAgentIMChannel(r *http.Request, workspaceID, project, agent, provider string, poll imbridge.SetupPollResponse) (controldb.AgentChannelBinding, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	openBaseURL, err := imbridge.MustOpenBaseURL(provider)
	if err != nil {
		return controldb.AgentChannelBinding{}, err
	}
	connectionName := agentChannelConnectionName(project, agent)
	connectionID := ""
	connections, err := s.controlDB.ListConnections(controldb.ConnectionFilter{
		WorkspaceID: workspaceID,
		Provider:    provider,
		OwnerType:   ConnectionOwnerWorkspace,
		OwnerID:     workspaceID,
	})
	if err != nil {
		return controldb.AgentChannelBinding{}, err
	}
	for _, connection := range connections {
		if connection.ConnectionName == connectionName {
			connectionID = connection.ID
			break
		}
	}
	if connectionID == "" {
		connectionID = newChannelID("conn")
	}

	profileRaw, _ := json.Marshal(map[string]any{
		"baseUrl":     openBaseURL,
		"accountsUrl": poll.BaseURL,
		"appId":       poll.AppID,
		"ownerOpenId": poll.OwnerOpenID,
		"usage":       "agent_im_channel",
	})
	connection := controldb.Connection{
		ID:             connectionID,
		WorkspaceID:    workspaceID,
		Provider:       provider,
		ConnectionName: connectionName,
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       "app_secret",
		Status:         "active",
		ProfileJSON:    string(profileRaw),
		CreatedBy:      requestUsername(r),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.controlDB.UpsertConnection(connection); err != nil {
		return controldb.AgentChannelBinding{}, err
	}
	secret, err := sealConnectionSecret(map[string]string{
		"baseUrl":   openBaseURL,
		"appId":     poll.AppID,
		"appSecret": poll.AppSecret,
	})
	if err != nil {
		return controldb.AgentChannelBinding{}, err
	}
	secret.ConnectionID = connectionID
	if err := s.controlDB.UpsertConnectionSecret(secret); err != nil {
		return controldb.AgentChannelBinding{}, err
	}
	_ = s.controlDB.CreateConnectionGrant(controldb.ConnectionGrant{
		ID:           newChannelID("grant"),
		WorkspaceID:  workspaceID,
		ConnectionID: connectionID,
		TargetType:   ConnectionTargetAgent,
		TargetID:     project + "/" + agent,
		CreatedBy:    requestUsername(r),
		CreatedAt:    now,
	})

	metadataRaw, _ := json.Marshal(map[string]any{
		"accountsUrl": poll.BaseURL,
		"appId":       poll.AppID,
	})
	binding := controldb.AgentChannelBinding{
		ID:              newChannelID("chan"),
		WorkspaceID:     workspaceID,
		ProjectID:       project,
		AgentID:         agent,
		Provider:        provider,
		ConnectionID:    connectionID,
		ExternalOwnerID: poll.OwnerOpenID,
		Status:          "connected",
		MetadataJSON:    string(metadataRaw),
		CreatedBy:       requestUsername(r),
		CreatedAt:       now,
		UpdatedAt:       now,
		LastActivityAt:  now,
	}
	if existing, found, err := s.findAgentChannelBinding(workspaceID, project, agent, provider); err != nil {
		return controldb.AgentChannelBinding{}, err
	} else if found {
		binding.ID = existing.ID
		binding.CreatedAt = existing.CreatedAt
	}
	if err := s.controlDB.UpsertAgentChannelBinding(binding); err != nil {
		return controldb.AgentChannelBinding{}, err
	}
	go s.refreshAgentIMBridges()
	if strings.TrimSpace(poll.OwnerOpenID) != "" {
		metadataRaw, _ := json.Marshal(map[string]any{
			"source":      "agent_channel_setup",
			"project":     project,
			"agent":       agent,
			"connectedAt": now,
		})
		if err := s.controlDB.UpsertExternalIdentity(controldb.ExternalIdentity{
			ID:             newChannelID("ext"),
			WorkspaceID:    workspaceID,
			Provider:       provider,
			ExternalUserID: poll.OwnerOpenID,
			UserID:         requestUsername(r),
			MetadataJSON:   string(metadataRaw),
			CreatedBy:      requestUsername(r),
			CreatedAt:      now,
			UpdatedAt:      now,
		}); err != nil {
			return controldb.AgentChannelBinding{}, err
		}
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "agent_channel.connected",
		ResourceType: "agent_channel",
		ResourceID:   binding.ID,
		Summary:      fmt.Sprintf("Connected %s channel for %s/%s", provider, project, agent),
		After:        agentChannelToResponse(binding),
		Request:      r,
	})
	return binding, nil
}

func (s *Server) parseProjectAgentProvider(w http.ResponseWriter, r *http.Request) (string, string, string, bool) {
	project, agent, ok := s.parseProjectAgent(w, r)
	if !ok {
		return "", "", "", false
	}
	if !s.checkProjectAccess(w, r, project) {
		return "", "", "", false
	}
	channelProvider, ok := imbridge.LookupProvider(r.PathValue("provider"))
	if !ok {
		s.jsonError(w, http.StatusBadRequest, "unsupported channel provider")
		return "", "", "", false
	}
	provider := channelProvider.Info().ID
	return project, agent, provider, true
}

func (s *Server) currentWorkspaceForRequest(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, err := s.currentWorkspaceID()
	if err != nil {
		s.jsonErrorCode(w, http.StatusServiceUnavailable, ErrCodeWorkspaceDatabaseUnavailable, "control database unavailable")
		return "", false
	}
	if !s.checkWorkspaceAccess(w, r, id) {
		return "", false
	}
	return id, true
}

func (s *Server) findAgentChannelBinding(workspaceID, project, agent, provider string) (controldb.AgentChannelBinding, bool, error) {
	bindings, err := s.controlDB.ListAgentChannelBindings(controldb.AgentChannelBindingFilter{
		WorkspaceID: workspaceID,
		ProjectID:   project,
		AgentID:     agent,
		Provider:    provider,
	})
	if err != nil {
		return controldb.AgentChannelBinding{}, false, err
	}
	if len(bindings) == 0 {
		return controldb.AgentChannelBinding{}, false, nil
	}
	return bindings[0], true, nil
}

func agentChannelToResponse(binding controldb.AgentChannelBinding) agentChannelResponse {
	var meta struct {
		AppID        string `json:"appId"`
		AccountsURL  string `json:"accountsUrl"`
		LastCallback struct {
			At        string `json:"at"`
			Status    string `json:"status"`
			Reason    string `json:"reason"`
			MessageID string `json:"messageId"`
			Error     string `json:"error"`
		} `json:"lastCallback"`
	}
	_ = json.Unmarshal([]byte(binding.MetadataJSON), &meta)
	resp := agentChannelResponse{
		ID:              binding.ID,
		Provider:        binding.Provider,
		Status:          binding.Status,
		ConnectionID:    binding.ConnectionID,
		AppID:           meta.AppID,
		AccountsURL:     meta.AccountsURL,
		ExternalBotID:   binding.ExternalBotID,
		ExternalChatID:  binding.ExternalChatID,
		ExternalOwnerID: binding.ExternalOwnerID,
		CreatedBy:       binding.CreatedBy,
		CreatedAt:       binding.CreatedAt,
		UpdatedAt:       binding.UpdatedAt,
		LastActivityAt:  binding.LastActivityAt,
	}
	resp.Callback.LastAt = meta.LastCallback.At
	resp.Callback.Status = meta.LastCallback.Status
	resp.Callback.Reason = meta.LastCallback.Reason
	resp.Callback.MessageID = meta.LastCallback.MessageID
	resp.Callback.Error = meta.LastCallback.Error
	return resp
}

func agentChannelConnectionName(project, agent string) string {
	return "agent-" + strings.NewReplacer("/", "-", " ", "-").Replace(project+"-"+agent)
}

func requestUsername(r *http.Request) string {
	if r == nil {
		return "system"
	}
	if username, ok := r.Context().Value(ctxUserKey).(string); ok && strings.TrimSpace(username) != "" {
		return strings.TrimSpace(username)
	}
	return "system"
}

func newChannelID(prefix string) string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(b[:])
}

func contextWithRequestTimeout(r *http.Request, timeout time.Duration) (context.Context, context.CancelFunc) {
	if r == nil {
		return context.WithTimeout(context.Background(), timeout)
	}
	return context.WithTimeout(r.Context(), timeout)
}
