package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
)

type contextSourceBody struct {
	ID            string         `json:"id"`
	Type          string         `json:"type"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	ConnectionRef string         `json:"connectionRef"`
	Status        string         `json:"status"`
	Config        map[string]any `json:"config"`
	Metadata      map[string]any `json:"metadata"`
}

type contextItemBody struct {
	ID            string         `json:"id"`
	SourceID      string         `json:"sourceId"`
	SourceType    string         `json:"sourceType"`
	SourceItemID  string         `json:"sourceItemId"`
	SourceURL     string         `json:"sourceUrl"`
	ProjectID     string         `json:"projectId"`
	AgentWorkerID string         `json:"agentWorkerId"`
	AuthorType    string         `json:"authorType"`
	AuthorID      string         `json:"authorId"`
	OccurredAt    string         `json:"occurredAt"`
	Title         string         `json:"title"`
	Summary       string         `json:"summary"`
	Content       string         `json:"content"`
	ContentRef    string         `json:"contentRef"`
	Payload       map[string]any `json:"payload"`
	Labels        map[string]any `json:"labels"`
	Sensitivity   string         `json:"sensitivity"`
	Status        string         `json:"status"`
	DedupeKey     string         `json:"dedupeKey"`
	ACLPolicyID   string         `json:"aclPolicyId"`
	Retention     string         `json:"retention"`
	ExpiresAt     string         `json:"expiresAt"`
}

type contextItemsBatchBody struct {
	Items []contextItemBody `json:"items"`
}

// contextIngestBody is the stable boundary for external collectors. The API
// accepts normalized records only; source-specific fetching and parsing stay
// outside the multigent process.
type contextIngestBody struct {
	Source contextSourceBody `json:"source"`
	Items  []contextItemBody `json:"items"`
}

type contextSubscriptionBody struct {
	ID             string         `json:"id"`
	SubscriberType string         `json:"subscriberType"`
	SubscriberID   string         `json:"subscriberId"`
	SourceIDs      []string       `json:"sourceIds"`
	LabelFilter    map[string]any `json:"labelFilter"`
	MaxSensitivity string         `json:"maxSensitivity"`
	DeliveryMode   string         `json:"deliveryMode"`
	SignalRule     map[string]any `json:"signalRule"`
	Status         string         `json:"status"`
}

func (s *Server) handleContextCenterSources(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleContextCenterSourcesList(w, r)
	case http.MethodPost:
		s.handleContextCenterSourcesCreate(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleContextCenterSourcesList(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}
	if !s.checkCurrentWorkspaceAccess(w, r) {
		return
	}
	sources, err := s.controlDB.ListContextSources(controldb.ContextSourceFilter{
		WorkspaceID: workspaceID,
		Type:        strings.TrimSpace(r.URL.Query().Get("type")),
		Status:      strings.TrimSpace(r.URL.Query().Get("status")),
		Limit:       queryLimit(r, 100),
	})
	if err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"sources": contextSourceResponses(sources)})
}

func (s *Server) handleContextCenterSourcesCreate(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}
	if !s.checkCurrentWorkspaceAccess(w, r) || !s.requireClientScope(w, r, clientScopeContextRW) {
		return
	}
	var body contextSourceBody
	if err := s.readJSON(w, r, &body); err != nil {
		return
	}
	sourceType := strings.TrimSpace(body.Type)
	name := strings.TrimSpace(body.Name)
	if sourceType == "" || name == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "source type and name are required")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	createdBy := currentRequestUsername(s, r)
	source := controldb.ContextSource{
		ID:            firstNonEmpty(strings.TrimSpace(body.ID), newContextID("ctxsrc")),
		WorkspaceID:   workspaceID,
		Type:          sourceType,
		Name:          name,
		Description:   strings.TrimSpace(body.Description),
		ConnectionRef: strings.TrimSpace(body.ConnectionRef),
		Status:        firstNonEmpty(strings.TrimSpace(body.Status), "active"),
		ConfigJSON:    marshalJSONObject(body.Config),
		MetadataJSON:  marshalJSONObject(body.Metadata),
		CreatedBy:     createdBy,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.controlDB.UpsertContextSource(source); err != nil {
		s.serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"source": contextSourceResponse(source)})
}

func (s *Server) handleContextCenterItems(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleContextCenterItemsList(w, r)
	case http.MethodPost:
		s.handleContextCenterItemsCreate(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleContextCenterItemsList(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}
	if !s.checkCurrentWorkspaceAccess(w, r) {
		return
	}
	items, err := s.controlDB.ListContextItems(contextItemFilterFromQuery(workspaceID, r))
	if err != nil {
		s.serverError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if !s.contextItemReadableByRequest(r, workspaceID, item) {
			continue
		}
		out = append(out, contextItemResponse(item, false))
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"items": out})
}

func (s *Server) handleContextCenterItemsCreate(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}
	var body contextItemBody
	if err := s.readJSONMax(w, r, &body, contextImportMaxJSONBody); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	item, ok := s.contextItemFromBody(w, r, workspaceID, body)
	if !ok {
		return
	}
	if err := s.controlDB.UpsertContextItem(item); err != nil {
		s.serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"item": contextItemResponse(item, true)})
}

func (s *Server) handleContextCenterItemsBatch(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}
	var body contextItemsBatchBody
	if err := s.readJSONMax(w, r, &body, contextImportMaxJSONBody); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	if len(body.Items) == 0 {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "items are required")
		return
	}
	if len(body.Items) > 500 {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "too many items")
		return
	}
	created := make([]map[string]any, 0, len(body.Items))
	for _, raw := range body.Items {
		item, ok := s.contextItemFromBody(w, r, workspaceID, raw)
		if !ok {
			return
		}
		if err := s.controlDB.UpsertContextItem(item); err != nil {
			s.serverError(w, err)
			return
		}
		created = append(created, contextItemResponse(item, false))
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"items": created, "accepted": len(created)})
}

func (s *Server) handleContextIngest(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}
	if !s.checkCurrentWorkspaceAccess(w, r) || !s.requireClientScope(w, r, clientScopeContextRW) {
		return
	}
	var body contextIngestBody
	if err := s.readJSONMax(w, r, &body, contextImportMaxJSONBody); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	sourceType := strings.TrimSpace(body.Source.Type)
	sourceName := strings.TrimSpace(body.Source.Name)
	if sourceType == "" || sourceName == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "source.type and source.name are required")
		return
	}
	if len(body.Items) == 0 {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "items are required")
		return
	}
	if len(body.Items) > 500 {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "too many items")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	source := controldb.ContextSource{
		ID:            firstNonEmpty(strings.TrimSpace(body.Source.ID), newContextID("ctxsrc")),
		WorkspaceID:   workspaceID,
		Type:          sourceType,
		Name:          sourceName,
		Description:   strings.TrimSpace(body.Source.Description),
		ConnectionRef: strings.TrimSpace(body.Source.ConnectionRef),
		Status:        firstNonEmpty(strings.TrimSpace(body.Source.Status), "active"),
		ConfigJSON:    marshalJSONObject(body.Source.Config),
		MetadataJSON:  marshalJSONObject(body.Source.Metadata),
		CreatedBy:     currentRequestUsername(s, r),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.controlDB.UpsertContextSource(source); err != nil {
		s.serverError(w, err)
		return
	}
	created := 0
	deduplicated := 0
	items := make([]map[string]any, 0, len(body.Items))
	for _, raw := range body.Items {
		raw.SourceID = source.ID
		raw.SourceType = source.Type
		if strings.TrimSpace(raw.DedupeKey) == "" && strings.TrimSpace(raw.SourceItemID) != "" {
			raw.DedupeKey = stableContextHash(source.ID + "\x00" + strings.TrimSpace(raw.SourceItemID))
		}
		item, ok := s.contextItemFromBody(w, r, workspaceID, raw)
		if !ok {
			return
		}
		if strings.TrimSpace(raw.ID) == "" && item.DedupeKey != "" {
			item.ID = "ctx-" + item.DedupeKey
		}
		existing, found, err := s.controlDB.ContextItemByID(workspaceID, item.ID)
		if err != nil {
			s.serverError(w, err)
			return
		}
		if found && existing.DedupeKey == item.DedupeKey {
			deduplicated++
		} else {
			created++
		}
		if err := s.controlDB.UpsertContextItem(item); err != nil {
			s.serverError(w, err)
			return
		}
		items = append(items, contextItemResponse(item, false))
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"source":       contextSourceResponse(source),
		"items":        items,
		"fetched":      len(body.Items),
		"created":      created,
		"deduplicated": deduplicated,
	})
}

func (s *Server) handleContextCenterItemGet(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}
	if !s.checkCurrentWorkspaceAccess(w, r) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	item, found, err := s.controlDB.ContextItemByID(workspaceID, id)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeNotFound, "context item not found")
		return
	}
	if !s.contextItemReadableByRequest(r, workspaceID, item) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeForbidden, "context item access required")
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"item": contextItemResponse(item, true)})
}

func (s *Server) handleContextCenterSubscriptions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleContextCenterSubscriptionsList(w, r)
	case http.MethodPost:
		s.handleContextCenterSubscriptionsCreate(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleContextCenterSubscriptionsList(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}
	if !s.checkCurrentWorkspaceAccess(w, r) {
		return
	}
	subs, err := s.controlDB.ListContextSubscriptions(controldb.ContextSubscriptionFilter{
		WorkspaceID:    workspaceID,
		SubscriberType: strings.TrimSpace(r.URL.Query().Get("subscriberType")),
		SubscriberID:   strings.TrimSpace(r.URL.Query().Get("subscriberId")),
		Status:         strings.TrimSpace(r.URL.Query().Get("status")),
		Limit:          queryLimit(r, 100),
	})
	if err != nil {
		s.serverError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(subs))
	for _, sub := range subs {
		if !s.contextSubscriptionReadableByRequest(r, workspaceID, sub) {
			continue
		}
		out = append(out, contextSubscriptionResponse(sub))
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"subscriptions": out})
}

func (s *Server) handleContextCenterSubscriptionsCreate(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := s.currentWorkspaceForRequest(w, r)
	if !ok {
		return
	}
	if !s.checkCurrentWorkspaceAccess(w, r) || !s.requireClientScope(w, r, clientScopeContextRW) {
		return
	}
	var body contextSubscriptionBody
	if err := s.readJSON(w, r, &body); err != nil {
		return
	}
	subType := strings.TrimSpace(body.SubscriberType)
	subID := strings.TrimSpace(body.SubscriberID)
	if subType == "" || subID == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "subscriberType and subscriberId are required")
		return
	}
	if !s.canManageContextSubscriber(r, workspaceID, subType, subID) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeForbidden, "subscriber access required")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	sub := controldb.ContextSubscription{
		ID:              firstNonEmpty(strings.TrimSpace(body.ID), newContextID("ctxsub")),
		WorkspaceID:     workspaceID,
		SubscriberType:  subType,
		SubscriberID:    subID,
		SourceIDsJSON:   marshalJSONArray(body.SourceIDs),
		LabelFilterJSON: marshalJSONObject(body.LabelFilter),
		MaxSensitivity:  firstNonEmpty(strings.TrimSpace(body.MaxSensitivity), "L2"),
		DeliveryMode:    firstNonEmpty(strings.TrimSpace(body.DeliveryMode), "searchable"),
		SignalRuleJSON:  marshalJSONObject(body.SignalRule),
		Status:          firstNonEmpty(strings.TrimSpace(body.Status), "active"),
		CreatedBy:       currentRequestUsername(s, r),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.controlDB.UpsertContextSubscription(sub); err != nil {
		s.serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"subscription": contextSubscriptionResponse(sub)})
}

func (s *Server) handleRuntimeContextCenterItems(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.runtimeRequireCapability(w, r, "docs.use")
	if !ok {
		return
	}
	workerID := s.runtimePrincipalAgentWorkerID(principal)
	filter := contextItemFilterFromQuery(principal.WorkspaceID, r)
	items, err := s.controlDB.ListContextItems(filter)
	if err != nil {
		s.serverError(w, err)
		return
	}
	includeContent := r.URL.Query().Get("content") == "1" || strings.EqualFold(r.URL.Query().Get("content"), "true")
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if !runtimeCanReadContextItem(principal, workerID, item) {
			continue
		}
		out = append(out, contextItemResponse(item, includeContent))
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"items": out})
}

func (s *Server) handleRuntimeContextCenterItemGet(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.runtimeRequireCapability(w, r, "docs.use")
	if !ok {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	item, found, err := s.controlDB.ContextItemByID(principal.WorkspaceID, id)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeNotFound, "context item not found")
		return
	}
	if !runtimeCanReadContextItem(principal, s.runtimePrincipalAgentWorkerID(principal), item) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeForbidden, "context item access required")
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"item": contextItemResponse(item, true)})
}

func (s *Server) contextItemFromBody(w http.ResponseWriter, r *http.Request, workspaceID string, body contextItemBody) (controldb.ContextItem, bool) {
	if !s.checkCurrentWorkspaceAccess(w, r) || !s.requireClientScope(w, r, clientScopeContextRW) {
		return controldb.ContextItem{}, false
	}
	sourceType := strings.TrimSpace(body.SourceType)
	if sourceType == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "sourceType is required")
		return controldb.ContextItem{}, false
	}
	projectID := strings.TrimSpace(body.ProjectID)
	agentWorkerID := strings.TrimSpace(body.AgentWorkerID)
	if projectID != "" && !s.canOperateProject(r, projectID) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeProjectOperatorRequired, "project operator access required")
		return controldb.ContextItem{}, false
	}
	if agentWorkerID != "" && !s.canAdminWorkspace(r, workspaceID) && !s.currentUserCanOperateAgentWorker(r, workspaceID, agentWorkerID) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeAgentOperatorRequired, "agent operator access required")
		return controldb.ContextItem{}, false
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return controldb.ContextItem{
		ID:            firstNonEmpty(strings.TrimSpace(body.ID), newContextID("ctx")),
		WorkspaceID:   workspaceID,
		SourceID:      strings.TrimSpace(body.SourceID),
		SourceType:    sourceType,
		SourceItemID:  strings.TrimSpace(body.SourceItemID),
		SourceURL:     strings.TrimSpace(body.SourceURL),
		ProjectID:     projectID,
		AgentWorkerID: agentWorkerID,
		AuthorType:    strings.TrimSpace(body.AuthorType),
		AuthorID:      strings.TrimSpace(body.AuthorID),
		OccurredAt:    strings.TrimSpace(body.OccurredAt),
		CollectedAt:   now,
		Title:         strings.TrimSpace(body.Title),
		Summary:       strings.TrimSpace(body.Summary),
		ContentText:   strings.TrimSpace(body.Content),
		ContentRef:    strings.TrimSpace(body.ContentRef),
		PayloadJSON:   marshalJSONObject(body.Payload),
		LabelsJSON:    marshalJSONObject(body.Labels),
		Sensitivity:   normalizeContextSensitivity(body.Sensitivity),
		Status:        firstNonEmpty(strings.TrimSpace(body.Status), "active"),
		DedupeKey:     strings.TrimSpace(body.DedupeKey),
		ACLPolicyID:   strings.TrimSpace(body.ACLPolicyID),
		Retention:     strings.TrimSpace(body.Retention),
		ExpiresAt:     strings.TrimSpace(body.ExpiresAt),
		CreatedAt:     now,
		UpdatedAt:     now,
	}, true
}

func contextItemFilterFromQuery(workspaceID string, r *http.Request) controldb.ContextItemFilter {
	return controldb.ContextItemFilter{
		WorkspaceID:   workspaceID,
		SourceID:      strings.TrimSpace(r.URL.Query().Get("sourceId")),
		SourceType:    strings.TrimSpace(r.URL.Query().Get("sourceType")),
		ProjectID:     strings.TrimSpace(r.URL.Query().Get("project")),
		AgentWorkerID: strings.TrimSpace(r.URL.Query().Get("agentWorkerId")),
		Status:        strings.TrimSpace(r.URL.Query().Get("status")),
		Query:         strings.TrimSpace(r.URL.Query().Get("q")),
		Since:         strings.TrimSpace(r.URL.Query().Get("since")),
		Limit:         queryLimit(r, 100),
	}
}

func (s *Server) contextItemReadableByRequest(r *http.Request, workspaceID string, item controldb.ContextItem) bool {
	if s.canAdminWorkspace(r, workspaceID) {
		return true
	}
	if strings.TrimSpace(item.AgentWorkerID) != "" {
		return s.currentUserCanOperateAgentWorker(r, workspaceID, item.AgentWorkerID)
	}
	if strings.TrimSpace(item.ProjectID) != "" {
		return s.canAccessProject(r, item.ProjectID)
	}
	return s.userCanAccessWorkspace(r, workspaceID)
}

func (s *Server) contextSubscriptionReadableByRequest(r *http.Request, workspaceID string, sub controldb.ContextSubscription) bool {
	if s.canAdminWorkspace(r, workspaceID) {
		return true
	}
	return s.canManageContextSubscriber(r, workspaceID, sub.SubscriberType, sub.SubscriberID)
}

func (s *Server) canManageContextSubscriber(r *http.Request, workspaceID, subType, subID string) bool {
	switch strings.TrimSpace(subType) {
	case "agent_worker":
		return s.currentUserCanOperateAgentWorker(r, workspaceID, subID)
	case "user":
		cur := s.currentUser(r)
		return cur != nil && cur.Username == subID
	case "project":
		return s.canOperateProject(r, subID)
	case "workspace":
		return s.canAdminWorkspace(r, workspaceID)
	default:
		return false
	}
}

func runtimeCanReadContextItem(principal runtimeAgentPrincipal, workerID string, item controldb.ContextItem) bool {
	if strings.TrimSpace(item.WorkspaceID) != strings.TrimSpace(principal.WorkspaceID) {
		return false
	}
	if strings.TrimSpace(item.AgentWorkerID) != "" && strings.TrimSpace(item.AgentWorkerID) != strings.TrimSpace(workerID) {
		return false
	}
	if strings.TrimSpace(item.ProjectID) != "" && strings.TrimSpace(item.ProjectID) != strings.TrimSpace(principal.Project) {
		return false
	}
	return true
}

func contextSourceResponses(sources []controldb.ContextSource) []map[string]any {
	out := make([]map[string]any, 0, len(sources))
	for _, source := range sources {
		out = append(out, contextSourceResponse(source))
	}
	return out
}

func contextSourceResponse(source controldb.ContextSource) map[string]any {
	return map[string]any{
		"id":            source.ID,
		"workspaceId":   source.WorkspaceID,
		"type":          source.Type,
		"name":          source.Name,
		"description":   source.Description,
		"connectionRef": source.ConnectionRef,
		"status":        source.Status,
		"config":        decodeJSONValue(source.ConfigJSON, map[string]any{}),
		"metadata":      decodeJSONValue(source.MetadataJSON, map[string]any{}),
		"createdBy":     source.CreatedBy,
		"createdAt":     source.CreatedAt,
		"updatedAt":     source.UpdatedAt,
	}
}

func contextItemResponse(item controldb.ContextItem, includeContent bool) map[string]any {
	out := map[string]any{
		"id":            item.ID,
		"workspaceId":   item.WorkspaceID,
		"sourceId":      item.SourceID,
		"sourceType":    item.SourceType,
		"sourceItemId":  item.SourceItemID,
		"sourceUrl":     item.SourceURL,
		"projectId":     item.ProjectID,
		"agentWorkerId": item.AgentWorkerID,
		"authorType":    item.AuthorType,
		"authorId":      item.AuthorID,
		"occurredAt":    item.OccurredAt,
		"collectedAt":   item.CollectedAt,
		"title":         item.Title,
		"summary":       item.Summary,
		"contentRef":    item.ContentRef,
		"payload":       decodeJSONValue(item.PayloadJSON, map[string]any{}),
		"labels":        decodeJSONValue(item.LabelsJSON, map[string]any{}),
		"sensitivity":   item.Sensitivity,
		"status":        item.Status,
		"dedupeKey":     item.DedupeKey,
		"aclPolicyId":   item.ACLPolicyID,
		"retention":     item.Retention,
		"expiresAt":     item.ExpiresAt,
		"lastUsedAt":    item.LastUsedAt,
		"usageCount":    item.UsageCount,
		"createdAt":     item.CreatedAt,
		"updatedAt":     item.UpdatedAt,
	}
	if includeContent {
		out["content"] = item.ContentText
	}
	return out
}

func contextSubscriptionResponse(sub controldb.ContextSubscription) map[string]any {
	return map[string]any{
		"id":             sub.ID,
		"workspaceId":    sub.WorkspaceID,
		"subscriberType": sub.SubscriberType,
		"subscriberId":   sub.SubscriberID,
		"sourceIds":      decodeJSONValue(sub.SourceIDsJSON, []any{}),
		"labelFilter":    decodeJSONValue(sub.LabelFilterJSON, map[string]any{}),
		"maxSensitivity": sub.MaxSensitivity,
		"deliveryMode":   sub.DeliveryMode,
		"signalRule":     decodeJSONValue(sub.SignalRuleJSON, map[string]any{}),
		"status":         sub.Status,
		"createdBy":      sub.CreatedBy,
		"createdAt":      sub.CreatedAt,
		"updatedAt":      sub.UpdatedAt,
	}
}

func normalizeContextSensitivity(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "L1", "L2", "L3":
		return strings.ToUpper(strings.TrimSpace(value))
	case "1":
		return "L1"
	case "3":
		return "L3"
	default:
		return "L2"
	}
}

func queryLimit(r *http.Request, def int) int {
	limit := def
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 500 {
		limit = 500
	}
	return limit
}

func marshalJSONObject(value map[string]any) string {
	if len(value) == 0 {
		return "{}"
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func marshalJSONArray(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	clean := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			clean = append(clean, strings.TrimSpace(value))
		}
	}
	raw, err := json.Marshal(clean)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

func currentRequestUsername(s *Server, r *http.Request) string {
	if cur := s.currentUser(r); cur != nil && cur.Username != "" {
		return cur.Username
	}
	return "system"
}

func newContextID(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return prefix + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return prefix + "-" + hex.EncodeToString(b[:])
}

func stableContextHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:32]
}
