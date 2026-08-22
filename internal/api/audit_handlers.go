package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
)

type auditLogInput struct {
	WorkspaceID  string
	ActorType    string
	ActorID      string
	Action       string
	ResourceType string
	ResourceID   string
	Summary      string
	Before       any
	After        any
	Request      *http.Request
}

func (s *Server) auditLog(input auditLogInput) {
	if s == nil || s.controlDB == nil {
		return
	}
	if strings.TrimSpace(input.Action) == "" || strings.TrimSpace(input.ResourceType) == "" {
		return
	}
	workspaceID := strings.TrimSpace(input.WorkspaceID)
	if workspaceID == "" {
		if id, err := s.currentWorkspaceID(); err == nil {
			workspaceID = id
		}
	}
	actorType := strings.TrimSpace(input.ActorType)
	actorID := strings.TrimSpace(input.ActorID)
	if input.Request != nil && actorID == "" {
		if cur := s.currentUser(input.Request); cur != nil && cur.Username != "" {
			actorType = "user"
			actorID = cur.Username
			if cur.Username == "apikey" {
				actorType = "api_key"
			}
		}
	}
	if actorType == "" {
		actorType = "system"
	}
	if actorID == "" {
		actorID = "system"
	}

	event := controldb.AuditEvent{
		ID:           newAuditID(),
		WorkspaceID:  workspaceID,
		ActorType:    actorType,
		ActorID:      actorID,
		Action:       strings.TrimSpace(input.Action),
		ResourceType: strings.TrimSpace(input.ResourceType),
		ResourceID:   strings.TrimSpace(input.ResourceID),
		Summary:      strings.TrimSpace(input.Summary),
		BeforeJSON:   auditJSON(input.Before),
		AfterJSON:    auditJSON(input.After),
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	if input.Request != nil {
		event.IP = requestIP(input.Request)
		event.UserAgent = input.Request.UserAgent()
	}
	_ = s.controlDB.CreateAuditEvent(event)
}

func auditJSON(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func newAuditID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("aud-%d", time.Now().UnixNano())
	}
	return "aud-" + hex.EncodeToString(b[:])
}

func requestIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	for _, header := range []string{"CF-Connecting-IP", "Fly-Client-IP", "True-Client-IP", "X-Real-IP"} {
		value := strings.TrimSpace(r.Header.Get(header))
		if ip := cleanClientIP(value); ip != "" {
			return ip
		}
	}
	if value := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); value != "" {
		for _, part := range strings.Split(value, ",") {
			if ip := cleanClientIP(part); ip != "" {
				return ip
			}
		}
	}
	if value := strings.TrimSpace(r.Header.Get("Forwarded")); value != "" {
		for _, entry := range strings.Split(value, ",") {
			for _, part := range strings.Split(entry, ";") {
				key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
				if !ok || !strings.EqualFold(strings.TrimSpace(key), "for") {
					continue
				}
				if ip := cleanClientIP(value); ip != "" {
					return ip
				}
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return cleanClientIP(host)
	}
	return cleanClientIP(r.RemoteAddr)
}

func cleanClientIP(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	value = strings.Trim(value, `"`)
	if strings.EqualFold(value, "unknown") {
		return ""
	}
	if strings.HasPrefix(value, "[") {
		if host, _, err := net.SplitHostPort(value); err == nil {
			value = host
		} else {
			value = strings.Trim(value, "[]")
		}
	} else if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	if parsed := net.ParseIP(value); parsed != nil {
		return parsed.String()
	}
	return value
}

type auditEventResponse struct {
	ID           string          `json:"id"`
	WorkspaceID  string          `json:"workspaceId"`
	ActorType    string          `json:"actorType"`
	ActorID      string          `json:"actorId"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resourceType"`
	ResourceID   string          `json:"resourceId"`
	Summary      string          `json:"summary,omitempty"`
	Before       json.RawMessage `json:"before,omitempty"`
	After        json.RawMessage `json:"after,omitempty"`
	IP           string          `json:"ip,omitempty"`
	UserAgent    string          `json:"userAgent,omitempty"`
	CreatedAt    string          `json:"createdAt"`
}

func (s *Server) handleAuditEvents(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 100 {
		limit = 100
	}
	offset := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			offset = n
		}
	}
	createdAfter := ""
	if days := s.currentEntitlements(r).AuditRetentionDays; days > 0 {
		createdAfter = time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour).Format(time.RFC3339)
	}
	filter := controldb.AuditEventFilter{
		WorkspaceID:  workspaceID,
		ActorID:      strings.TrimSpace(r.URL.Query().Get("actorId")),
		Action:       strings.TrimSpace(r.URL.Query().Get("action")),
		ResourceType: strings.TrimSpace(r.URL.Query().Get("resourceType")),
		ResourceID:   strings.TrimSpace(r.URL.Query().Get("resourceId")),
		CreatedAfter: createdAfter,
		Limit:        limit,
		Offset:       offset,
	}
	events, err := s.controlDB.ListAuditEvents(filter)
	if err != nil {
		s.serverError(w, err)
		return
	}
	total, err := s.controlDB.CountAuditEvents(filter)
	if err != nil {
		s.serverError(w, err)
		return
	}
	facets, err := s.controlDB.ListAuditEventFacets(filter)
	if err != nil {
		s.serverError(w, err)
		return
	}
	out := make([]auditEventResponse, 0, len(events))
	for _, event := range events {
		out = append(out, auditEventToResponse(event))
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"events": out,
		"total":  total,
		"limit":  limit,
		"offset": offset,
		"facets": map[string]any{
			"actorIds":      facets.ActorIDs,
			"actions":       facets.Actions,
			"resourceTypes": facets.ResourceTypes,
			"resourceIds":   facets.ResourceIDs,
		},
	})
}

func auditEventToResponse(event controldb.AuditEvent) auditEventResponse {
	return auditEventResponse{
		ID:           event.ID,
		WorkspaceID:  event.WorkspaceID,
		ActorType:    event.ActorType,
		ActorID:      event.ActorID,
		Action:       event.Action,
		ResourceType: event.ResourceType,
		ResourceID:   event.ResourceID,
		Summary:      event.Summary,
		Before:       auditRawJSON(event.BeforeJSON),
		After:        auditRawJSON(event.AfterJSON),
		IP:           event.IP,
		UserAgent:    event.UserAgent,
		CreatedAt:    event.CreatedAt,
	}
}

func auditRawJSON(value string) json.RawMessage {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if !json.Valid([]byte(value)) {
		return nil
	}
	return json.RawMessage(value)
}
