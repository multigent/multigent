package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	controldb "github.com/multigent/multigent/internal/db"
)

type runtimeAttentionPatchRequest struct {
	Status string `json:"status"`
}

func (s *Server) handleRuntimeAttentionSignals(w http.ResponseWriter, r *http.Request) {
	principal, ok := runtimeAgentFromRequest(r)
	if !ok {
		s.jsonErrorCode(w, http.StatusUnauthorized, ErrCodeRuntimeAgentTokenRequired, "runtime agent token required")
		return
	}
	if !runtimeHasCapability(principal, "attention.use") {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeRuntimeCapabilityRequired, "runtime token lacks attention.use capability")
		return
	}
	workerID := s.runtimePrincipalAgentWorkerID(principal)
	if strings.TrimSpace(workerID) == "" {
		_ = json.NewEncoder(w).Encode(map[string]any{"signals": []map[string]any{}})
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	statuses := []string(nil)
	switch strings.ToLower(status) {
	case "open", "active", "todo":
		status = ""
		statuses = []string{"pending", "seen", "handling"}
	}
	signals, err := s.controlDB.ListAttentionSignals(controldb.AttentionSignalFilter{
		WorkspaceID:   principal.WorkspaceID,
		AgentWorkerID: workerID,
		Status:        status,
		Statuses:      statuses,
		SourceKind:    strings.TrimSpace(r.URL.Query().Get("sourceKind")),
		Reason:        strings.TrimSpace(r.URL.Query().Get("reason")),
		Limit:         limit,
	})
	if err != nil {
		s.serverError(w, err)
		return
	}
	for i := range signals {
		if strings.TrimSpace(signals[i].Status) != "pending" {
			continue
		}
		if err := s.controlDB.MarkAttentionSignalStatus(principal.WorkspaceID, signals[i].ID, "seen"); err != nil {
			s.serverError(w, err)
			return
		}
		updated, found, err := s.controlDB.AttentionSignalByID(principal.WorkspaceID, signals[i].ID)
		if err != nil {
			s.serverError(w, err)
			return
		}
		if found {
			signals[i] = updated
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"signals": attentionSignalResponses(signals)})
}

func (s *Server) handleRuntimePatchAttentionSignal(w http.ResponseWriter, r *http.Request) {
	principal, ok := runtimeAgentFromRequest(r)
	if !ok {
		s.jsonErrorCode(w, http.StatusUnauthorized, ErrCodeRuntimeAgentTokenRequired, "runtime agent token required")
		return
	}
	if !runtimeHasCapability(principal, "attention.use") {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeRuntimeCapabilityRequired, "runtime token lacks attention.use capability")
		return
	}
	workerID := s.runtimePrincipalAgentWorkerID(principal)
	if strings.TrimSpace(workerID) == "" {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeRuntimeCapabilityRequired, "runtime agent is not linked to an agent worker")
		return
	}
	var body runtimeAttentionPatchRequest
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	status := normalizeAttentionStatus(body.Status)
	if status == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "invalid attention status")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "attention signal id is required")
		return
	}
	signal, found, err := s.controlDB.AttentionSignalByID(principal.WorkspaceID, id)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeNotFound, "attention signal not found")
		return
	}
	if strings.TrimSpace(signal.AgentWorkerID) != workerID {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeRuntimeCapabilityRequired, "attention signal belongs to another agent")
		return
	}
	if err := s.controlDB.MarkAttentionSignalStatus(principal.WorkspaceID, id, status); err != nil {
		s.serverError(w, err)
		return
	}
	updated, _, err := s.controlDB.AttentionSignalByID(principal.WorkspaceID, id)
	if err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"signal": attentionSignalResponse(updated)})
}
