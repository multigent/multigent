package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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

type channelManualSetupRequest struct {
	Values map[string]string `json:"values"`
}

type agentChannelSecurityRequest struct {
	VerificationToken *string `json:"verificationToken"`
	EncryptKey        *string `json:"encryptKey"`
}

type agentChannelBindCodeResponse struct {
	Code      string `json:"code"`
	Command   string `json:"command"`
	ExpiresAt string `json:"expiresAt"`
	Provider  string `json:"provider"`
	Project   string `json:"project"`
	Agent     string `json:"agent"`
	Target    string `json:"target"`
	Name      string `json:"name,omitempty"`
}

type agentChannelIdentityResponse struct {
	UserID      string `json:"userId"`
	DisplayName string `json:"displayName,omitempty"`
	Email       string `json:"email,omitempty"`
	BoundAt     string `json:"boundAt,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
}

type agentChannelTargetResponse struct {
	ID             string `json:"id"`
	Type           string `json:"type"`
	Name           string `json:"name"`
	Provider       string `json:"provider"`
	ExternalChatID string `json:"externalChatId,omitempty"`
	CreatedAt      string `json:"createdAt,omitempty"`
	UpdatedAt      string `json:"updatedAt,omitempty"`
	LastActivityAt string `json:"lastActivityAt,omitempty"`
}

type agentChannelBindCodeRequest struct {
	Target string `json:"target"`
	Name   string `json:"name"`
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
	filter := controldb.AgentChannelBindingFilter{
		WorkspaceID: workspaceID,
		ProjectID:   project,
		AgentID:     agent,
	}
	if workerID := s.agentWorkerIDForProjectAgent(workspaceID, project, agent); workerID != "" {
		filter = controldb.AgentChannelBindingFilter{WorkspaceID: workspaceID, AgentWorkerID: workerID}
	}
	bindings, err := s.controlDB.ListAgentChannelBindings(filter)
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

func (s *Server) handleAgentWorkerChannels(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.agentWorkerToolBindingScope(w, r)
	if !ok {
		return
	}
	bindings, err := s.controlDB.ListAgentChannelBindings(controldb.AgentChannelBindingFilter{
		WorkspaceID:   scope.workspaceID,
		AgentWorkerID: scope.worker.ID,
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

func (s *Server) handleAgentWorkerChannelDelete(w http.ResponseWriter, r *http.Request) {
	scope, provider, ok := s.parseAgentWorkerProvider(w, r)
	if !ok {
		return
	}
	binding, found, err := s.findAgentWorkerChannelBinding(scope.workspaceID, scope.worker.ID, provider)
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
		WorkspaceID:  scope.workspaceID,
		Action:       "agent_channel.disconnect",
		ResourceType: "agent_channel",
		ResourceID:   binding.ID,
		Summary:      fmt.Sprintf("Disconnected %s channel for agent worker %s", provider, scope.worker.Name),
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

func (s *Server) handleAgentWorkerChannelSetupBegin(w http.ResponseWriter, r *http.Request) {
	scope, provider, ok := s.parseAgentWorkerProvider(w, r)
	if !ok {
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
		WorkspaceID:  scope.workspaceID,
		Action:       "agent_channel.setup_begin",
		ResourceType: "agent",
		ResourceID:   scope.worker.ID,
		Summary:      fmt.Sprintf("Started %s channel setup for agent worker %s", provider, scope.worker.Name),
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

func (s *Server) handleAgentWorkerChannelSecurity(w http.ResponseWriter, r *http.Request) {
	scope, provider, ok := s.parseAgentWorkerProvider(w, r)
	if !ok {
		return
	}
	var req agentChannelSecurityRequest
	if err := s.readJSON(w, r, &req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	binding, found, err := s.findAgentWorkerChannelBinding(scope.workspaceID, scope.worker.ID, provider)
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
	binding, err := s.saveAgentIMChannel(r, workspaceID, project, agent, s.agentWorkerIDForProjectAgent(workspaceID, project, agent), actualProvider, poll)
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

func (s *Server) handleAgentWorkerChannelSetupPoll(w http.ResponseWriter, r *http.Request) {
	scope, provider, ok := s.parseAgentWorkerProvider(w, r)
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
	binding, err := s.saveAgentIMChannel(r, scope.workspaceID, scope.projectID, scope.agentName, scope.worker.ID, actualProvider, poll)
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

func (s *Server) handleAgentChannelSetupManual(w http.ResponseWriter, r *http.Request) {
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
	channelProvider, ok := imbridge.LookupProvider(provider)
	if !ok {
		s.jsonError(w, http.StatusBadRequest, "unsupported channel provider")
		return
	}
	var req channelManualSetupRequest
	if err := s.readJSON(w, r, &req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	ctx, cancel := contextWithRequestTimeout(r, 25*time.Second)
	defer cancel()
	result, err := channelProvider.ManualSetup(ctx, imbridge.ManualSetupRequest{Values: req.Values})
	if err != nil {
		s.jsonError(w, http.StatusBadGateway, err.Error())
		return
	}
	if result.Provider == "" {
		result.Provider = provider
	}
	binding, err := s.saveManualAgentIMChannel(r, workspaceID, project, agent, s.agentWorkerIDForProjectAgent(workspaceID, project, agent), result)
	if err != nil {
		s.serverError(w, err)
		return
	}
	resp := agentChannelToResponse(binding)
	resp.CallbackURL = requestBaseURL(r) + "/api/v1/im/" + binding.Provider + "/events"
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  "connected",
		"channel": resp,
	})
}

func (s *Server) handleAgentWorkerChannelSetupManual(w http.ResponseWriter, r *http.Request) {
	scope, provider, ok := s.parseAgentWorkerProvider(w, r)
	if !ok {
		return
	}
	channelProvider, ok := imbridge.LookupProvider(provider)
	if !ok {
		s.jsonError(w, http.StatusBadRequest, "unsupported channel provider")
		return
	}
	var req channelManualSetupRequest
	if err := s.readJSON(w, r, &req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	ctx, cancel := contextWithRequestTimeout(r, 25*time.Second)
	defer cancel()
	result, err := channelProvider.ManualSetup(ctx, imbridge.ManualSetupRequest{Values: req.Values})
	if err != nil {
		s.jsonError(w, http.StatusBadGateway, err.Error())
		return
	}
	if result.Provider == "" {
		result.Provider = provider
	}
	binding, err := s.saveManualAgentIMChannel(r, scope.workspaceID, scope.projectID, scope.agentName, scope.worker.ID, result)
	if err != nil {
		s.serverError(w, err)
		return
	}
	resp := agentChannelToResponse(binding)
	resp.CallbackURL = requestBaseURL(r) + "/api/v1/im/" + binding.Provider + "/events"
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  "connected",
		"channel": resp,
	})
}

func (s *Server) handleAgentChannelIdentities(w http.ResponseWriter, r *http.Request) {
	project, agent, provider, ok := s.parseProjectAgentProvider(w, r)
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
	binding, found, err := s.findAgentChannelBinding(workspaceID, project, agent, provider)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found {
		s.jsonError(w, http.StatusNotFound, "agent channel not found")
		return
	}
	identities, err := s.controlDB.ListUserChannelIdentities(controldb.UserChannelIdentityFilter{
		WorkspaceID:      workspaceID,
		ChannelBindingID: binding.ID,
		Provider:         provider,
	})
	if err != nil {
		s.serverError(w, err)
		return
	}
	out := make([]agentChannelIdentityResponse, 0, len(identities))
	for _, identity := range identities {
		item := agentChannelIdentityResponse{
			UserID:    identity.UserID,
			BoundAt:   identity.CreatedAt,
			UpdatedAt: identity.UpdatedAt,
		}
		if user := s.users.GetUser(identity.UserID); user != nil {
			item.DisplayName = user.DisplayName
			item.Email = user.Email
		}
		out = append(out, item)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"provider":   provider,
		"channelId":  binding.ID,
		"identities": out,
	})
}

func (s *Server) handleAgentWorkerChannelIdentities(w http.ResponseWriter, r *http.Request) {
	scope, provider, ok := s.parseAgentWorkerProvider(w, r)
	if !ok {
		return
	}
	binding, found, err := s.findAgentWorkerChannelBinding(scope.workspaceID, scope.worker.ID, provider)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found {
		s.jsonError(w, http.StatusNotFound, "agent channel not found")
		return
	}
	identities, err := s.controlDB.ListUserChannelIdentities(controldb.UserChannelIdentityFilter{
		WorkspaceID:      scope.workspaceID,
		ChannelBindingID: binding.ID,
		Provider:         provider,
	})
	if err != nil {
		s.serverError(w, err)
		return
	}
	out := make([]agentChannelIdentityResponse, 0, len(identities))
	for _, identity := range identities {
		item := agentChannelIdentityResponse{
			UserID:    identity.UserID,
			BoundAt:   identity.CreatedAt,
			UpdatedAt: identity.UpdatedAt,
		}
		if user := s.users.GetUser(identity.UserID); user != nil {
			item.DisplayName = user.DisplayName
			item.Email = user.Email
		}
		out = append(out, item)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"provider":   provider,
		"channelId":  binding.ID,
		"identities": out,
	})
}

func (s *Server) handleAgentChannelTargets(w http.ResponseWriter, r *http.Request) {
	project, agent, provider, ok := s.parseProjectAgentProvider(w, r)
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
	binding, found, err := s.findAgentChannelBinding(workspaceID, project, agent, provider)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found {
		s.jsonError(w, http.StatusNotFound, "agent channel not found")
		return
	}
	targets, err := s.controlDB.ListAgentChannelTargets(controldb.AgentChannelTargetFilter{
		WorkspaceID:      workspaceID,
		ChannelBindingID: binding.ID,
		Provider:         provider,
	})
	if err != nil {
		s.serverError(w, err)
		return
	}
	out := make([]agentChannelTargetResponse, 0, len(targets))
	for _, target := range targets {
		out = append(out, agentChannelTargetResponse{
			ID:             target.ID,
			Type:           target.TargetType,
			Name:           target.DisplayName,
			Provider:       target.Provider,
			ExternalChatID: target.ExternalChatID,
			CreatedAt:      target.CreatedAt,
			UpdatedAt:      target.UpdatedAt,
			LastActivityAt: target.LastActivityAt,
		})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"provider":  provider,
		"channelId": binding.ID,
		"targets":   out,
	})
}

func (s *Server) handleAgentWorkerChannelTargets(w http.ResponseWriter, r *http.Request) {
	scope, provider, ok := s.parseAgentWorkerProvider(w, r)
	if !ok {
		return
	}
	binding, found, err := s.findAgentWorkerChannelBinding(scope.workspaceID, scope.worker.ID, provider)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found {
		s.jsonError(w, http.StatusNotFound, "agent channel not found")
		return
	}
	targets, err := s.controlDB.ListAgentChannelTargets(controldb.AgentChannelTargetFilter{
		WorkspaceID:      scope.workspaceID,
		ChannelBindingID: binding.ID,
		Provider:         provider,
	})
	if err != nil {
		s.serverError(w, err)
		return
	}
	out := make([]agentChannelTargetResponse, 0, len(targets))
	for _, target := range targets {
		out = append(out, agentChannelTargetResponse{
			ID:             target.ID,
			Type:           target.TargetType,
			Name:           target.DisplayName,
			Provider:       target.Provider,
			ExternalChatID: target.ExternalChatID,
			CreatedAt:      target.CreatedAt,
			UpdatedAt:      target.UpdatedAt,
			LastActivityAt: target.LastActivityAt,
		})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"provider":  provider,
		"channelId": binding.ID,
		"targets":   out,
	})
}

func (s *Server) handleAgentChannelBindCode(w http.ResponseWriter, r *http.Request) {
	project, agent, provider, ok := s.parseProjectAgentProvider(w, r)
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
	username := currentUsername(s.currentUser(r))
	if username == "" || username == "system" || username == "apikey" {
		s.jsonError(w, http.StatusUnauthorized, "login is required to create a bind code")
		return
	}
	var req agentChannelBindCodeRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}
	target := strings.ToLower(strings.TrimSpace(req.Target))
	if target == "" {
		target = "user"
	}
	if target != "user" && target != "chat" {
		s.jsonError(w, http.StatusBadRequest, "unsupported bind target")
		return
	}
	targetName := strings.TrimSpace(req.Name)
	if target == "chat" && targetName == "" {
		s.jsonError(w, http.StatusBadRequest, "chat name is required")
		return
	}
	binding, found, err := s.findAgentChannelBinding(workspaceID, project, agent, provider)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found || binding.Status != "connected" {
		s.jsonError(w, http.StatusNotFound, "agent channel is not connected")
		return
	}
	code, err := newHumanBindCode()
	if err != nil {
		s.serverError(w, err)
		return
	}
	now := time.Now().UTC()
	expiresAt := now.Add(10 * time.Minute).Format(time.RFC3339)
	if err := s.controlDB.CreateAgentChannelBindCode(controldb.AgentChannelBindCode{
		Code:             code,
		WorkspaceID:      workspaceID,
		ChannelBindingID: binding.ID,
		UserID:           username,
		TargetType:       target,
		TargetName:       targetName,
		ExpiresAt:        expiresAt,
		CreatedAt:        now.Format(time.RFC3339),
	}); err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		ActorType:    "user",
		ActorID:      username,
		Action:       "agent_channel.bind_code.create",
		ResourceType: "agent_channel",
		ResourceID:   binding.ID,
		Summary:      fmt.Sprintf("Created %s %s bind code for %s/%s", provider, target, project, agent),
		After: map[string]any{
			"provider":  provider,
			"project":   project,
			"agent":     agent,
			"target":    target,
			"name":      targetName,
			"expiresAt": expiresAt,
		},
		Request: r,
	})
	command := "/bind " + code
	if target == "chat" {
		command = "/bind-chat " + code
	}
	_ = json.NewEncoder(w).Encode(agentChannelBindCodeResponse{
		Code:      code,
		Command:   command,
		ExpiresAt: expiresAt,
		Provider:  provider,
		Project:   project,
		Agent:     agent,
		Target:    target,
		Name:      targetName,
	})
}

func (s *Server) handleAgentWorkerChannelBindCode(w http.ResponseWriter, r *http.Request) {
	scope, provider, ok := s.parseAgentWorkerProvider(w, r)
	if !ok {
		return
	}
	username := currentUsername(s.currentUser(r))
	if username == "" || username == "system" || username == "apikey" {
		s.jsonError(w, http.StatusUnauthorized, "login is required to create a bind code")
		return
	}
	var req agentChannelBindCodeRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}
	target := strings.ToLower(strings.TrimSpace(req.Target))
	if target == "" {
		target = "user"
	}
	if target != "user" && target != "chat" {
		s.jsonError(w, http.StatusBadRequest, "unsupported bind target")
		return
	}
	targetName := strings.TrimSpace(req.Name)
	if target == "chat" && targetName == "" {
		s.jsonError(w, http.StatusBadRequest, "chat name is required")
		return
	}
	binding, found, err := s.findAgentWorkerChannelBinding(scope.workspaceID, scope.worker.ID, provider)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found || binding.Status != "connected" {
		s.jsonError(w, http.StatusNotFound, "agent channel is not connected")
		return
	}
	code, err := newHumanBindCode()
	if err != nil {
		s.serverError(w, err)
		return
	}
	now := time.Now().UTC()
	expiresAt := now.Add(10 * time.Minute).Format(time.RFC3339)
	if err := s.controlDB.CreateAgentChannelBindCode(controldb.AgentChannelBindCode{
		Code:             code,
		WorkspaceID:      scope.workspaceID,
		ChannelBindingID: binding.ID,
		UserID:           username,
		TargetType:       target,
		TargetName:       targetName,
		ExpiresAt:        expiresAt,
		CreatedAt:        now.Format(time.RFC3339),
	}); err != nil {
		s.serverError(w, err)
		return
	}
	command := "/bind " + code
	if target == "chat" {
		command = "/bind-chat " + code
	}
	_ = json.NewEncoder(w).Encode(agentChannelBindCodeResponse{
		Code:      code,
		Command:   command,
		ExpiresAt: expiresAt,
		Provider:  provider,
		Agent:     scope.worker.Name,
		Target:    target,
		Name:      targetName,
	})
}

func (s *Server) saveAgentIMChannel(r *http.Request, workspaceID, project, agent, agentWorkerID, provider string, poll imbridge.SetupPollResponse) (controldb.AgentChannelBinding, error) {
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
		"purpose":     "agent_channel",
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
		TargetID:     agentChannelGrantTarget(project, agent, agentWorkerID),
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
		AgentWorkerID:   agentWorkerID,
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
	var existing controldb.AgentChannelBinding
	var found bool
	if strings.TrimSpace(agentWorkerID) != "" {
		existing, found, err = s.findAgentWorkerChannelBinding(workspaceID, agentWorkerID, provider)
	} else {
		existing, found, err = s.findAgentChannelBinding(workspaceID, project, agent, provider)
	}
	if err != nil {
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
		if err := s.upsertAgentChannelUserIdentity(binding, provider, poll.OwnerOpenID, "", requestUsername(r), string(metadataRaw), requestUsername(r), now); err != nil {
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

func (s *Server) saveManualAgentIMChannel(r *http.Request, workspaceID, project, agent, agentWorkerID string, result imbridge.ManualSetupResult) (controldb.AgentChannelBinding, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	provider := result.Provider
	openBaseURL := strings.TrimSpace(result.BaseURL)
	if openBaseURL == "" {
		var err error
		openBaseURL, err = imbridge.MustOpenBaseURL(provider)
		if err != nil {
			return controldb.AgentChannelBinding{}, err
		}
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
	profile := map[string]any{
		"baseUrl": openBaseURL,
		"appId":   result.AppID,
		"purpose": "agent_channel",
		"usage":   "agent_im_channel",
	}
	for k, v := range result.Profile {
		profile[k] = v
	}
	profileRaw, _ := json.Marshal(profile)
	authType := strings.TrimSpace(result.AuthType)
	if authType == "" {
		authType = "token"
	}
	connection := controldb.Connection{
		ID:             connectionID,
		WorkspaceID:    workspaceID,
		Provider:       provider,
		ConnectionName: connectionName,
		OwnerType:      ConnectionOwnerWorkspace,
		OwnerID:        workspaceID,
		AuthType:       authType,
		Status:         "active",
		ProfileJSON:    string(profileRaw),
		CreatedBy:      requestUsername(r),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.controlDB.UpsertConnection(connection); err != nil {
		return controldb.AgentChannelBinding{}, err
	}
	values := map[string]string{"baseUrl": openBaseURL}
	for k, v := range result.SecretValues {
		values[k] = v
	}
	secret, err := sealConnectionSecret(values)
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
		TargetID:     agentChannelGrantTarget(project, agent, agentWorkerID),
		CreatedBy:    requestUsername(r),
		CreatedAt:    now,
	})

	metadataRaw, _ := json.Marshal(map[string]any{
		"appId": result.AppID,
	})
	binding := controldb.AgentChannelBinding{
		ID:              newChannelID("chan"),
		WorkspaceID:     workspaceID,
		AgentWorkerID:   agentWorkerID,
		ProjectID:       project,
		AgentID:         agent,
		Provider:        provider,
		ConnectionID:    connectionID,
		ExternalBotID:   result.ExternalBotID,
		ExternalChatID:  result.ExternalChatID,
		ExternalOwnerID: result.ExternalOwnerID,
		Status:          "connected",
		MetadataJSON:    string(metadataRaw),
		CreatedBy:       requestUsername(r),
		CreatedAt:       now,
		UpdatedAt:       now,
		LastActivityAt:  now,
	}
	var existing controldb.AgentChannelBinding
	var found bool
	if strings.TrimSpace(agentWorkerID) != "" {
		existing, found, err = s.findAgentWorkerChannelBinding(workspaceID, agentWorkerID, provider)
	} else {
		existing, found, err = s.findAgentChannelBinding(workspaceID, project, agent, provider)
	}
	if err != nil {
		return controldb.AgentChannelBinding{}, err
	} else if found {
		binding.ID = existing.ID
		binding.CreatedAt = existing.CreatedAt
		if binding.ExternalChatID == "" {
			binding.ExternalChatID = existing.ExternalChatID
		}
		if binding.ExternalOwnerID == "" {
			binding.ExternalOwnerID = existing.ExternalOwnerID
		}
	}
	if err := s.controlDB.UpsertAgentChannelBinding(binding); err != nil {
		return controldb.AgentChannelBinding{}, err
	}
	if strings.TrimSpace(result.ExternalOwnerID) != "" {
		metadataRaw, _ := json.Marshal(map[string]any{
			"source":      "agent_channel_manual_setup",
			"project":     project,
			"agent":       agent,
			"provider":    provider,
			"connectedAt": now,
		})
		if err := s.controlDB.UpsertExternalIdentity(controldb.ExternalIdentity{
			ID:             newChannelID("ext"),
			WorkspaceID:    workspaceID,
			Provider:       provider,
			ExternalUserID: result.ExternalOwnerID,
			UserID:         requestUsername(r),
			MetadataJSON:   string(metadataRaw),
			CreatedBy:      requestUsername(r),
			CreatedAt:      now,
			UpdatedAt:      now,
		}); err != nil {
			return controldb.AgentChannelBinding{}, err
		}
		if err := s.upsertAgentChannelUserIdentity(binding, provider, result.ExternalOwnerID, result.ExternalChatID, requestUsername(r), string(metadataRaw), requestUsername(r), now); err != nil {
			return controldb.AgentChannelBinding{}, err
		}
	}
	go s.refreshAgentIMBridges()
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

func (s *Server) upsertAgentChannelUserIdentity(binding controldb.AgentChannelBinding, provider, externalUserID, externalChatID, userID, metadataJSON, createdBy, now string) error {
	externalUserID = strings.TrimSpace(externalUserID)
	userID = strings.TrimSpace(userID)
	if externalUserID == "" || userID == "" {
		return nil
	}
	return s.controlDB.UpsertUserChannelIdentity(controldb.UserChannelIdentity{
		ID:               newChannelID("uch"),
		WorkspaceID:      binding.WorkspaceID,
		UserID:           userID,
		ChannelBindingID: binding.ID,
		Provider:         provider,
		ExternalUserID:   externalUserID,
		ExternalChatID:   strings.TrimSpace(externalChatID),
		MetadataJSON:     firstNonEmpty(strings.TrimSpace(metadataJSON), "{}"),
		CreatedBy:        strings.TrimSpace(createdBy),
		CreatedAt:        now,
		UpdatedAt:        now,
	})
}

func agentChannelGrantTarget(project, agent, agentWorkerID string) string {
	if strings.TrimSpace(agentWorkerID) != "" {
		return connectionAgentWorkerTargetID(agentWorkerID)
	}
	return strings.TrimSpace(project) + "/" + strings.TrimSpace(agent)
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

func (s *Server) parseAgentWorkerProvider(w http.ResponseWriter, r *http.Request) (agentWorkerBindingScope, string, bool) {
	scope, ok := s.agentWorkerToolBindingScope(w, r)
	if !ok {
		return agentWorkerBindingScope{}, "", false
	}
	channelProvider, ok := imbridge.LookupProvider(r.PathValue("provider"))
	if !ok {
		s.jsonError(w, http.StatusBadRequest, "unsupported channel provider")
		return agentWorkerBindingScope{}, "", false
	}
	return scope, channelProvider.Info().ID, true
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
	if workerID := s.agentWorkerIDForProjectAgent(workspaceID, project, agent); workerID != "" {
		bindings, err := s.controlDB.ListAgentChannelBindings(controldb.AgentChannelBindingFilter{
			WorkspaceID:   workspaceID,
			AgentWorkerID: workerID,
			Provider:      provider,
		})
		if err != nil {
			return controldb.AgentChannelBinding{}, false, err
		}
		if len(bindings) > 0 {
			return bindings[0], true, nil
		}
	}
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

func (s *Server) findAgentWorkerChannelBinding(workspaceID, workerID, provider string) (controldb.AgentChannelBinding, bool, error) {
	bindings, err := s.controlDB.ListAgentChannelBindings(controldb.AgentChannelBindingFilter{
		WorkspaceID:   workspaceID,
		AgentWorkerID: workerID,
		Provider:      provider,
	})
	if err != nil {
		return controldb.AgentChannelBinding{}, false, err
	}
	if len(bindings) == 0 {
		return controldb.AgentChannelBinding{}, false, nil
	}
	return bindings[0], true, nil
}

func (s *Server) agentWorkerIDForProjectAgent(workspaceID, project, agent string) string {
	workerID, _ := s.agentWorkerContextForProjectAgent(workspaceID, project, agent)
	return workerID
}

func (s *Server) agentWorkerContextForProjectAgent(workspaceID, project, agent string) (workerID, membershipID string) {
	if s == nil || s.agentDirectory == nil {
		return "", ""
	}
	resolved, ok, err := s.agentDirectory.ResolveProjectMailbox(workspaceID, project+"/"+agent)
	if err != nil || !ok {
		return "", ""
	}
	return strings.TrimSpace(resolved.Worker.ID), strings.TrimSpace(resolved.Membership.ID)
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

func newHumanBindCode() (string, error) {
	var b [5]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "MG-" + strings.ToUpper(hex.EncodeToString(b[:]))[:8], nil
}

func contextWithRequestTimeout(r *http.Request, timeout time.Duration) (context.Context, context.CancelFunc) {
	if r == nil {
		return context.WithTimeout(context.Background(), timeout)
	}
	return context.WithTimeout(r.Context(), timeout)
}
