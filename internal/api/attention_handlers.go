package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	controldb "github.com/multigent/multigent/internal/db"
)

type patchAttentionSignalRequest struct {
	Status string `json:"status"`
}

func (s *Server) handleAgentAttentionSignals(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}
	worker, found, err := s.agentDirectory.Worker(workspaceID, r.PathValue("id"))
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeAgentNotFound, "agent not found")
		return
	}
	if allowed, err := s.canAccessAgentWorkerForRequest(r, workspaceID, worker); err != nil {
		s.serverError(w, err)
		return
	} else if !allowed {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeAgentAccessRequired, "agent access required")
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	signals, err := s.controlDB.ListAttentionSignals(controldb.AttentionSignalFilter{
		WorkspaceID:   workspaceID,
		AgentWorkerID: worker.ID,
		Status:        strings.TrimSpace(r.URL.Query().Get("status")),
		SourceKind:    strings.TrimSpace(r.URL.Query().Get("sourceKind")),
		Reason:        strings.TrimSpace(r.URL.Query().Get("reason")),
		Limit:         limit,
	})
	if err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"signals": attentionSignalResponses(signals)})
}

func (s *Server) handlePatchAttentionSignal(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}
	var body patchAttentionSignalRequest
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
	signal, found, err := s.controlDB.AttentionSignalByID(workspaceID, id)
	if err != nil {
		s.serverError(w, err)
		return
	} else if !found {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeNotFound, "attention signal not found")
		return
	}
	if strings.TrimSpace(signal.AgentWorkerID) != "" {
		worker, found, err := s.agentDirectory.Worker(workspaceID, signal.AgentWorkerID)
		if err != nil {
			s.serverError(w, err)
			return
		}
		if !found {
			s.jsonErrorCode(w, http.StatusNotFound, ErrCodeAgentNotFound, "agent not found")
			return
		}
		if allowed, err := s.canAccessAgentWorkerForRequest(r, workspaceID, worker); err != nil {
			s.serverError(w, err)
			return
		} else if !allowed {
			s.jsonErrorCode(w, http.StatusForbidden, ErrCodeAgentAccessRequired, "agent access required")
			return
		}
	} else if !s.canAdminWorkspace(r, workspaceID) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeWorkspaceAdminRequired, "workspace admin access required")
		return
	}
	if err := s.controlDB.MarkAttentionSignalStatus(workspaceID, id, status); err != nil {
		s.serverError(w, err)
		return
	}
	signal, _, err = s.controlDB.AttentionSignalByID(workspaceID, id)
	if err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"signal": attentionSignalResponse(signal)})
}

func normalizeAttentionStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending", "seen", "handling", "handled", "ignored", "expired":
		return strings.ToLower(strings.TrimSpace(status))
	default:
		return ""
	}
}

func attentionSignalResponses(signals []controldb.AttentionSignal) []map[string]any {
	out := make([]map[string]any, 0, len(signals))
	for _, signal := range signals {
		out = append(out, attentionSignalResponse(signal))
	}
	return out
}

func attentionSignalResponse(signal controldb.AttentionSignal) map[string]any {
	return map[string]any{
		"id":            signal.ID,
		"workspaceId":   signal.WorkspaceID,
		"agentWorkerId": signal.AgentWorkerID,
		"dedupeKey":     signal.DedupeKey,
		"sourceKind":    signal.SourceKind,
		"sourceId":      signal.SourceID,
		"sourceChannel": signal.SourceChannel,
		"reason":        signal.Reason,
		"priority":      signal.Priority,
		"actorType":     signal.ActorType,
		"actorId":       signal.ActorID,
		"summary":       signal.Summary,
		"refs":          decodeJSONValue(signal.RefsJSON, map[string]any{}),
		"payload":       decodeJSONValue(signal.PayloadJSON, map[string]any{}),
		"resultRef":     signal.ResultRef,
		"status":        signal.Status,
		"createdAt":     signal.CreatedAt,
		"expiresAt":     signal.ExpiresAt,
		"seenAt":        signal.SeenAt,
		"handlingAt":    signal.HandlingAt,
		"handledAt":     signal.HandledAt,
	}
}
