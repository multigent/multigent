package api

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	workflowstore "github.com/multigent/multigent/internal/workflow"
)

const (
	workflowNotificationTable = "workflow_notifications"

	workflowNotifyAssigneeKey   = "notifyAssignee"
	workflowNotifyChannelKey    = "notifyChannel"
	workflowNotifyChannelAuto   = "auto"
	workflowNotifyChannelFeishu = "feishu"
	workflowNotifyChannelLark   = "lark"
)

type workflowTriggerEvent struct {
	Type        string
	WorkspaceID string
	Project     string
	TaskID      string
	TaskTitle   string
	Run         entity.WorkflowRun
	Definition  entity.WorkflowDefinition
	Step        entity.WorkflowStep
	Instance    entity.WorkflowStepInstance
}

type workflowNotificationRecord struct {
	ID                string    `json:"id"`
	WorkspaceID       string    `json:"workspaceId"`
	Project           string    `json:"project"`
	TaskID            string    `json:"taskId"`
	TaskTitle         string    `json:"taskTitle"`
	WorkflowRunID     string    `json:"workflowRunId"`
	WorkflowID        string    `json:"workflowId"`
	WorkflowName      string    `json:"workflowName"`
	StepID            string    `json:"stepId"`
	StepTitle         string    `json:"stepTitle"`
	RecipientUserID   string    `json:"recipientUserId"`
	Provider          string    `json:"provider"`
	ConnectionID      string    `json:"connectionId,omitempty"`
	ExternalUserID    string    `json:"externalUserId,omitempty"`
	Status            string    `json:"status"`
	Error             string    `json:"error,omitempty"`
	CallbackTokenHash string    `json:"callbackTokenHash,omitempty"`
	ExternalMessageID string    `json:"externalMessageId,omitempty"`
	OpenURL           string    `json:"openUrl,omitempty"`
	CallbackURL       string    `json:"callbackUrl,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
	ActedAt           time.Time `json:"actedAt,omitempty"`
}

type workflowTriggerCallbackBody struct {
	Decision string            `json:"decision"`
	Comments string            `json:"comments"`
	Outputs  map[string]string `json:"outputs"`
}

type workflowNotificationTarget struct {
	Provider       string
	Connection     controldb.Connection
	ConnectionData map[string]string
	ExternalUserID string
}

type workflowTriggerSendResult struct {
	ExternalMessageID string
}

type workflowTriggerNotifier interface {
	SendWorkflowReview(event workflowTriggerEvent, record workflowNotificationRecord, target workflowNotificationTarget) (workflowTriggerSendResult, error)
}

type feishuWorkflowTriggerNotifier struct{}

func (s *Server) fireWorkflowStepTriggers(workspaceID string, event workflowTriggerEvent, r *http.Request) {
	if s == nil || s.controlDB == nil || strings.TrimSpace(workspaceID) == "" {
		return
	}
	event.WorkspaceID = workspaceID
	if event.Type == "" {
		event.Type = "workflow.step.enter"
	}
	if event.Step.Type != "human_review" && event.Instance.ActorType != "human" {
		return
	}
	if !workflowStepNotifyAssignee(event.Step) {
		return
	}
	if strings.TrimSpace(event.Instance.ActorID) == "" {
		return
	}
	if err := s.triggerHumanReviewNotification(event, r); err != nil {
		log.Printf("[workflow-trigger] human review notification failed workspace=%s task=%s step=%s: %v", workspaceID, event.TaskID, event.Step.ID, err)
	}
}

func (s *Server) triggerHumanReviewNotification(event workflowTriggerEvent, r *http.Request) error {
	reviewer := strings.TrimSpace(event.Instance.ActorID)
	providers := workflowStepNotifyProviders(event.Step)
	if len(providers) == 0 {
		return nil
	}
	targets, err := s.workflowNotificationTargets(event.WorkspaceID, reviewer, providers)
	if err != nil {
		return err
	}
	var lastErr error
	for _, target := range targets {
		notifier, ok := s.workflowTriggerNotifier(target.Provider)
		if !ok {
			continue
		}
		record, err := s.createWorkflowNotificationRecord(event, target.Provider, reviewer, target.ExternalUserID, target.Connection.ID, r)
		if err != nil {
			return err
		}
		result, err := notifier.SendWorkflowReview(event, record, target)
		if err != nil {
			record.Status = "failed"
			record.Error = err.Error()
			record.UpdatedAt = time.Now().UTC()
			_ = s.saveWorkflowNotification(record)
			lastErr = err
			continue
		}
		record.Status = "sent"
		record.ExternalMessageID = result.ExternalMessageID
		record.UpdatedAt = time.Now().UTC()
		if err := s.saveWorkflowNotification(record); err != nil {
			return err
		}
		s.auditLog(auditLogInput{
			WorkspaceID:  event.WorkspaceID,
			ActorType:    "system",
			ActorID:      "workflow-trigger",
			Action:       "workflow.trigger.notification.sent",
			ResourceType: "task",
			ResourceID:   event.Project + "/" + event.TaskID,
			Summary:      fmt.Sprintf("Sent %s review card for workflow step %s", target.Provider, event.Step.ID),
			After:        record,
		})
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	record, err := s.createWorkflowNotificationRecord(event, "none", reviewer, "", "", r)
	if err != nil {
		return err
	}
	record.Status = "pending"
	record.Error = "reviewer has no linked Feishu/Lark identity or no active workspace connection"
	record.UpdatedAt = time.Now().UTC()
	if err := s.saveWorkflowNotification(record); err != nil {
		return err
	}
	return nil
}

func (s *Server) workflowExternalIdentity(workspaceID, provider, userID string) (controldb.ExternalIdentity, bool, error) {
	identities, err := s.controlDB.ListExternalIdentities(controldb.ExternalIdentityFilter{
		WorkspaceID: workspaceID,
		Provider:    provider,
		UserID:      userID,
	})
	if err != nil {
		return controldb.ExternalIdentity{}, false, err
	}
	if len(identities) == 0 {
		return controldb.ExternalIdentity{}, false, nil
	}
	return identities[0], true, nil
}

func workflowStepNotifyAssignee(step entity.WorkflowStep) bool {
	return strings.EqualFold(strings.TrimSpace(step.Config[workflowNotifyAssigneeKey]), "true")
}

func workflowStepNotifyProviders(step entity.WorkflowStep) []string {
	switch strings.ToLower(strings.TrimSpace(step.Config[workflowNotifyChannelKey])) {
	case "", workflowNotifyChannelAuto:
		return []string{workflowNotifyChannelFeishu, workflowNotifyChannelLark}
	case workflowNotifyChannelFeishu:
		return []string{workflowNotifyChannelFeishu}
	case workflowNotifyChannelLark:
		return []string{workflowNotifyChannelLark}
	default:
		return nil
	}
}

func (s *Server) workflowNotificationTargets(workspaceID, reviewer string, providers []string) ([]workflowNotificationTarget, error) {
	targets := make([]workflowNotificationTarget, 0, len(providers))
	for _, provider := range providers {
		identity, ok, err := s.workflowExternalIdentity(workspaceID, provider, reviewer)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		connection, values, ok, err := s.workflowNotificationConnection(workspaceID, provider)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		targets = append(targets, workflowNotificationTarget{
			Provider:       provider,
			Connection:     connection,
			ConnectionData: values,
			ExternalUserID: identity.ExternalUserID,
		})
	}
	return targets, nil
}

func (s *Server) workflowTriggerNotifier(provider string) (workflowTriggerNotifier, bool) {
	switch strings.TrimSpace(provider) {
	case "feishu", "lark":
		return feishuWorkflowTriggerNotifier{}, true
	default:
		return nil, false
	}
}

func (s *Server) workflowNotificationConnection(workspaceID, provider string) (controldb.Connection, map[string]string, bool, error) {
	connections, err := s.controlDB.ListConnections(controldb.ConnectionFilter{
		WorkspaceID: workspaceID,
		Provider:    provider,
		OwnerType:   ConnectionOwnerWorkspace,
		OwnerID:     workspaceID,
		Status:      "active",
	})
	if err != nil {
		return controldb.Connection{}, nil, false, err
	}
	for _, connection := range connections {
		secret, ok, err := s.controlDB.ConnectionSecret(connection.ID)
		if err != nil {
			return controldb.Connection{}, nil, false, err
		}
		if !ok {
			continue
		}
		values, err := openConnectionSecret(secret)
		if err != nil {
			return controldb.Connection{}, nil, false, err
		}
		if strings.TrimSpace(values["appId"]) != "" && strings.TrimSpace(values["appSecret"]) != "" {
			return connection, values, true, nil
		}
	}
	return controldb.Connection{}, nil, false, nil
}

func (s *Server) createWorkflowNotificationRecord(event workflowTriggerEvent, provider, reviewer, externalUserID, connectionID string, r *http.Request) (workflowNotificationRecord, error) {
	now := time.Now().UTC()
	id := newWorkflowNotificationID()
	token, err := newWorkflowCallbackToken()
	if err != nil {
		return workflowNotificationRecord{}, err
	}
	openURL := workflowTaskOpenURL(r, event.Project, event.TaskID)
	callbackURL := workflowTriggerCallbackURL(r, event.WorkspaceID, id, token)
	record := workflowNotificationRecord{
		ID:                id,
		WorkspaceID:       event.WorkspaceID,
		Project:           event.Project,
		TaskID:            event.TaskID,
		TaskTitle:         event.TaskTitle,
		WorkflowRunID:     event.Run.ID,
		WorkflowID:        event.Definition.ID,
		WorkflowName:      event.Definition.Name,
		StepID:            event.Step.ID,
		StepTitle:         event.Step.Title,
		RecipientUserID:   reviewer,
		Provider:          provider,
		ConnectionID:      connectionID,
		ExternalUserID:    externalUserID,
		Status:            "pending",
		CallbackTokenHash: hashWorkflowCallbackToken(token),
		OpenURL:           openURL,
		CallbackURL:       callbackURL,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.saveWorkflowNotification(record); err != nil {
		return workflowNotificationRecord{}, err
	}
	return record, nil
}

func (s *Server) saveWorkflowNotification(record workflowNotificationRecord) error {
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return s.controlDB.UpsertRecord(workflowNotificationTable, record.WorkspaceID, []string{record.ID}, string(raw))
}

func (s *Server) workflowNotification(workspaceID, id string) (workflowNotificationRecord, bool, error) {
	raw, ok, err := s.controlDB.GetRecord(workflowNotificationTable, workspaceID, []string{id})
	if err != nil || !ok {
		return workflowNotificationRecord{}, ok, err
	}
	var record workflowNotificationRecord
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		return workflowNotificationRecord{}, false, err
	}
	return record, true, nil
}

func (feishuWorkflowTriggerNotifier) SendWorkflowReview(event workflowTriggerEvent, record workflowNotificationRecord, target workflowNotificationTarget) (workflowTriggerSendResult, error) {
	baseURL := strings.TrimSpace(target.ConnectionData["baseUrl"])
	if baseURL == "" {
		baseURL = defaultFeishuBaseURL(target.Connection.Provider)
	}
	tenantToken, err := fetchFeishuTenantAccessToken(baseURL, strings.TrimSpace(target.ConnectionData["appId"]), strings.TrimSpace(target.ConnectionData["appSecret"]))
	if err != nil {
		return workflowTriggerSendResult{}, err
	}
	card := workflowReviewFeishuCard(event, record)
	contentRaw, err := json.Marshal(card)
	if err != nil {
		return workflowTriggerSendResult{}, err
	}
	bodyRaw, err := json.Marshal(map[string]any{
		"receive_id": target.ExternalUserID,
		"msg_type":   "interactive",
		"content":    string(contentRaw),
	})
	if err != nil {
		return workflowTriggerSendResult{}, err
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+"/open-apis/im/v1/messages?receive_id_type=open_id", bytes.NewReader(bodyRaw))
	if err != nil {
		return workflowTriggerSendResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+tenantToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return workflowTriggerSendResult{}, fmt.Errorf("send %s workflow card: %w", target.Provider, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxJSONBody+1))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return workflowTriggerSendResult{}, fmt.Errorf("send %s workflow card: %s %s", target.Provider, resp.Status, strings.TrimSpace(string(respBody)))
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			MessageID string `json:"message_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return workflowTriggerSendResult{}, fmt.Errorf("decode %s workflow card response: %w", target.Provider, err)
	}
	if parsed.Code != 0 {
		return workflowTriggerSendResult{}, fmt.Errorf("send %s workflow card: %s", target.Provider, strings.TrimSpace(parsed.Msg))
	}
	return workflowTriggerSendResult{ExternalMessageID: parsed.Data.MessageID}, nil
}

func workflowReviewFeishuCard(event workflowTriggerEvent, record workflowNotificationRecord) map[string]any {
	summary := strings.TrimSpace(event.Instance.InputArtifact)
	if summary == "" && len(event.Instance.InputValues) > 0 {
		raw, _ := json.MarshalIndent(event.Instance.InputValues, "", "  ")
		summary = string(raw)
	}
	if summary == "" {
		summary = "请审核该流程节点的上游输出。"
	}
	if len([]rune(summary)) > 900 {
		summary = string([]rune(summary)[:900]) + "..."
	}
	return map[string]any{
		"config": map[string]any{"wide_screen_mode": true},
		"header": map[string]any{
			"template": "blue",
			"title":    map[string]string{"tag": "plain_text", "content": "Multigent 人工审核"},
		},
		"elements": []any{
			map[string]any{
				"tag": "div",
				"text": map[string]string{
					"tag":     "lark_md",
					"content": fmt.Sprintf("**%s**\n流程：%s\n节点：%s\n任务：%s", event.Step.Title, event.Definition.Name, event.Step.Title, event.TaskTitle),
				},
			},
			map[string]any{
				"tag": "div",
				"text": map[string]string{
					"tag":     "lark_md",
					"content": "**上游输出**\n" + summary,
				},
			},
			map[string]any{
				"tag": "action",
				"actions": []any{
					map[string]any{
						"tag":  "button",
						"text": map[string]string{"tag": "plain_text", "content": "打开审核"},
						"type": "primary",
						"url":  record.OpenURL,
					},
				},
			},
			map[string]any{
				"tag": "note",
				"elements": []any{
					map[string]string{"tag": "plain_text", "content": "后续版本会支持直接在卡片里通过或打回；当前请打开 Multigent 完成结构化审核。"},
				},
			},
		},
	}
}

func (s *Server) handlePostWorkflowTriggerCallback(w http.ResponseWriter, r *http.Request) {
	workspaceID := strings.TrimSpace(r.PathValue("workspaceId"))
	notificationID := strings.TrimSpace(r.PathValue("notificationId"))
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if workspaceID == "" || notificationID == "" || token == "" {
		s.jsonError(w, http.StatusBadRequest, "workspace, notification, and token are required")
		return
	}
	record, ok, err := s.workflowNotification(workspaceID, notificationID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !ok {
		s.jsonError(w, http.StatusNotFound, "workflow notification not found")
		return
	}
	if record.CallbackTokenHash == "" || !strings.EqualFold(record.CallbackTokenHash, hashWorkflowCallbackToken(token)) {
		s.jsonError(w, http.StatusForbidden, "invalid callback token")
		return
	}
	if record.Status == "acted" {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "status": "already_acted"})
		return
	}
	var body workflowTriggerCallbackBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Outputs == nil {
		body.Outputs = map[string]string{}
	}
	transition, err := s.submitWorkflowReviewFromTrigger(workspaceID, record, body, r)
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	record.Status = "acted"
	record.ActedAt = time.Now().UTC()
	record.UpdatedAt = record.ActedAt
	if err := s.saveWorkflowNotification(record); err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "done": transition.Done, "nextStep": transition.Run.ActiveStepID})
}

func (s *Server) submitWorkflowReviewFromTrigger(workspaceID string, record workflowNotificationRecord, body workflowTriggerCallbackBody, r *http.Request) (workflowstore.TransitionResult, error) {
	var result workflowstore.TransitionResult
	t, agent, err := s.findTaskInProject(record.Project, record.TaskID)
	if err != nil {
		return result, err
	}
	outputs := body.Outputs
	if outputDecision := normalizeWorkflowReviewDecision(outputs["decision"]); outputDecision != "" {
		outputs["decision"] = outputDecision
	} else if decision := normalizeWorkflowReviewDecision(body.Decision); decision != "" {
		outputs["decision"] = decision
	}
	comments := strings.TrimSpace(body.Comments)
	if comments == "" {
		comments = strings.TrimSpace(outputs["comments"])
	}
	if comments != "" {
		outputs["comments"] = comments
	}
	summary := formatWorkflowReviewFields(outputs)
	wfStore := workflowstore.NewStore(s.controlDB, workspaceID)
	result, err = wfStore.CompleteAndAdvance(record.Project, record.TaskID, summary, "", outputs, "completed")
	if err != nil {
		return result, err
	}
	_ = s.ts.RemoveFromInbox(record.TaskID)
	if result.Done {
		now := time.Now().UTC()
		t.Status = entity.TaskStatusDoneSuccess
		t.Summary = summary
		t.UpdatedAt = now
		t.FinishedAt = &now
		if err := s.ts.PersistTask(record.Project, agent, t); err != nil {
			return result, err
		}
	} else if err := s.activateNextWorkflowStep(workspaceID, record.Project, agent, t, result, r); err != nil {
		return result, err
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		ActorType:    "system",
		ActorID:      "workflow-trigger",
		Action:       "workflow.trigger.callback",
		ResourceType: "task",
		ResourceID:   record.Project + "/" + record.TaskID,
		Summary:      "Workflow review callback advanced task",
		After:        record,
		Request:      r,
	})
	return result, nil
}

func workflowTaskOpenURL(r *http.Request, project, taskID string) string {
	base := strings.TrimRight(strings.TrimSpace(workflowWebBaseURL(r)), "/")
	return base + "/projects/" + url.PathEscape(project) + "/tasks?task=" + url.QueryEscape(taskID)
}

func workflowTriggerCallbackURL(r *http.Request, workspaceID, notificationID, token string) string {
	base := strings.TrimRight(requestBaseURL(r), "/")
	return base + "/api/v1/workspaces/" + url.PathEscape(workspaceID) + "/workflow/triggers/" + url.PathEscape(notificationID) + "/callback?token=" + url.QueryEscape(token)
}

func workflowWebBaseURL(r *http.Request) string {
	if v := strings.TrimSpace(os.Getenv("MULTIGENT_WEB_BASE_URL")); v != "" {
		return v
	}
	if v := strings.TrimSpace(r.Header.Get("X-Multigent-Web-Base-URL")); v != "" {
		return v
	}
	base := requestBaseURL(r)
	if strings.Contains(base, "://127.0.0.1:27893") || strings.Contains(base, "://localhost:27893") {
		return strings.Replace(base, ":27893", ":27894", 1)
	}
	return base
}

func newWorkflowNotificationID() string {
	var b [9]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("wfn-%d", time.Now().UnixNano())
	}
	return "wfn-" + base64.RawURLEncoding.EncodeToString(b[:])
}

func newWorkflowCallbackToken() (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func hashWorkflowCallbackToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}
