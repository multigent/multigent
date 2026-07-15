package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const (
	ErrCodeBadRequest         = "bad_request"
	ErrCodeInvalidJSON        = "invalid_json"
	ErrCodeInvalidRequestBody = "invalid_request_body"
	ErrCodeValidationFailed   = "validation_failed"
	ErrCodeUnauthorized       = "unauthorized"
	ErrCodeForbidden          = "forbidden"
	ErrCodeNotFound           = "not_found"
	ErrCodeConflict           = "conflict"
	ErrCodeServiceUnavailable = "service_unavailable"
	ErrCodeUpstreamError      = "upstream_error"
	ErrCodeInternal           = "internal_error"

	ErrCodeAdminRequired                = "admin_required"
	ErrCodeAgentAccessRequired          = "agent_access_required"
	ErrCodeAgentAlreadyExists           = "agent_already_exists"
	ErrCodeAgentManagementRequired      = "agent_management_required"
	ErrCodeAgentNotFound                = "agent_not_found"
	ErrCodeAgentOperatorRequired        = "agent_operator_required"
	ErrCodeAuthenticatedUserRequired    = "authenticated_user_required"
	ErrCodeInvalidAgentName             = "invalid_agent_name"
	ErrCodeInvalidCredentials           = "invalid_credentials"
	ErrCodeInvitationNotFound           = "invitation_not_found"
	ErrCodeProjectAccessRequired        = "project_access_required"
	ErrCodeProjectManagerRequired       = "project_manager_required"
	ErrCodeProjectNotFound              = "project_not_found"
	ErrCodeProjectOperatorRequired      = "project_operator_required"
	ErrCodeSignupDisabled               = "signup_disabled"
	ErrCodeTeamNotFound                 = "team_not_found"
	ErrCodeUserAlreadyExists            = "user_already_exists"
	ErrCodeUserNotFound                 = "user_not_found"
	ErrCodeWorkspaceAccessRequired      = "workspace_access_required"
	ErrCodeWorkspaceAdminRequired       = "workspace_admin_required"
	ErrCodeWorkspaceAlreadyExists       = "workspace_already_exists"
	ErrCodeWorkspaceNameRequired        = "workspace_name_required"
	ErrCodeWorkspaceNotFound            = "workspace_not_found"
	ErrCodeWorkspaceDatabaseUnavailable = "workspace_database_unavailable"

	ErrCodeConnectionAccessRequired     = "connection_access_required"
	ErrCodeConnectionManagementRequired = "connection_management_required"
	ErrCodeConnectionNotFound           = "connection_not_found"
	ErrCodeConnectionGrantNotFound      = "connection_grant_not_found"
	ErrCodeCredentialValuesRequired     = "credential_values_required"
	ErrCodeProviderNotFound             = "provider_not_found"
	ErrCodeRuntimeAgentTokenRequired    = "runtime_agent_token_required"
	ErrCodeRuntimeConnectionNotGranted  = "runtime_connection_not_granted"
	ErrCodeRuntimeCapabilityRequired    = "runtime_capability_required"
	ErrCodeUnsupportedAuthType          = "unsupported_auth_type"
	ErrCodeUnsupportedProvider          = "unsupported_provider"

	ErrCodeAgentNotRunning       = "agent_not_running"
	ErrCodeCronNotFound          = "cron_not_found"
	ErrCodeHeartbeatNotFound     = "heartbeat_not_found"
	ErrCodeInvalidCronSchedule   = "invalid_cron_schedule"
	ErrCodeInvalidDuration       = "invalid_duration"
	ErrCodeProcessNotFound       = "process_not_found"
	ErrCodeSchedulerConflict     = "scheduler_conflict"
	ErrCodeSchedulerNotFound     = "scheduler_not_found"
	ErrCodeSchedulerWakeupFailed = "scheduler_wakeup_failed"
)

type apiErrorResponse struct {
	Error apiErrorBody `json:"error"`
}

type apiErrorBody struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	RequestID string         `json:"requestId"`
}

func (s *Server) jsonError(w http.ResponseWriter, status int, msg string) {
	s.writeAPIError(w, status, classifyErrorCode(status, msg), msg, nil)
}

func (s *Server) jsonErrorCode(w http.ResponseWriter, status int, code, msg string) {
	s.writeAPIError(w, status, code, msg, nil)
}

func (s *Server) jsonErrorDetails(w http.ResponseWriter, status int, code, msg string, details map[string]any) {
	s.writeAPIError(w, status, code, msg, details)
}

func (s *Server) writeAPIError(w http.ResponseWriter, status int, code, msg string, details map[string]any) {
	if strings.TrimSpace(code) == "" {
		code = classifyErrorCode(status, msg)
	}
	if strings.TrimSpace(msg) == "" {
		msg = http.StatusText(status)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Multigent-Error-Code", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiErrorResponse{
		Error: apiErrorBody{
			Code:      code,
			Message:   msg,
			Details:   details,
			RequestID: newErrorRequestID(),
		},
	})
}

func classifyErrorCode(status int, msg string) string {
	switch status {
	case http.StatusBadRequest:
		lower := strings.ToLower(strings.TrimSpace(msg))
		if strings.Contains(lower, "invalid json") || strings.Contains(lower, "invalid json body") || strings.Contains(lower, "invalid body") {
			return ErrCodeInvalidJSON
		}
		if strings.Contains(lower, "required") || strings.Contains(lower, "must be") || strings.Contains(lower, "invalid") {
			return ErrCodeValidationFailed
		}
		return ErrCodeBadRequest
	case http.StatusUnauthorized:
		return ErrCodeUnauthorized
	case http.StatusForbidden:
		return ErrCodeForbidden
	case http.StatusNotFound:
		return ErrCodeNotFound
	case http.StatusConflict:
		return ErrCodeConflict
	case http.StatusServiceUnavailable:
		return ErrCodeServiceUnavailable
	case http.StatusBadGateway:
		return ErrCodeUpstreamError
	case http.StatusInternalServerError:
		return ErrCodeInternal
	default:
		if status >= 500 {
			return ErrCodeInternal
		}
		return ErrCodeBadRequest
	}
}

func newErrorRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err == nil {
		return "err-" + hex.EncodeToString(b[:])
	}
	return "err-" + hex.EncodeToString([]byte(time.Now().UTC().Format("150405.000")))
}
