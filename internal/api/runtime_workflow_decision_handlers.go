package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	workflowstore "github.com/multigent/multigent/internal/workflow"
)

type runtimeWorkflowDecisionBody struct {
	InteractionID   string            `json:"interactionId"`
	DelegationToken string            `json:"delegationToken"`
	TaskID          string            `json:"taskId"`
	Decision        string            `json:"decision"`
	Comments        string            `json:"comments"`
	Outputs         map[string]string `json:"outputs"`
}

type runtimeWorkflowDecisionResponse struct {
	InteractionID string               `json:"interactionId"`
	SubmittedBy   string               `json:"submittedBy"`
	Workflow      taskWorkflowResponse `json:"workflow"`
}

func (s *Server) handleRuntimeWorkflowDecision(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.runtimeRequireCapability(w, r, "task.use")
	if !ok {
		return
	}
	var body runtimeWorkflowDecisionBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	interactionID := strings.TrimSpace(body.InteractionID)
	taskID := strings.TrimSpace(body.TaskID)
	if interactionID == "" {
		s.jsonError(w, http.StatusBadRequest, "interactionId is required")
		return
	}
	delegation, ok := s.validateWorkflowDecisionDelegation(w, principal, strings.TrimSpace(body.DelegationToken), interactionID)
	if !ok {
		return
	}
	request, found, err := s.controlDB.InteractionRequestByID(principal.WorkspaceID, interactionID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found {
		s.jsonError(w, http.StatusNotFound, "interaction request not found")
		return
	}
	if taskID == "" {
		taskID = taskIDFromInteractionContext(request.ContextJSON)
	}
	if taskID == "" {
		s.jsonError(w, http.StatusBadRequest, "taskId is required")
		return
	}
	if err := validateRuntimeWorkflowDecisionInteraction(principal, request, taskID); err != nil {
		s.jsonError(w, http.StatusForbidden, err.Error())
		return
	}
	if strings.TrimSpace(request.SubmittedBy) != "" && request.SubmittedBy != delegation.UserID {
		s.jsonError(w, http.StatusForbidden, "delegation user does not match interaction submitter")
		return
	}
	if err := s.validateWorkflowDecisionReviewer(principal.WorkspaceID, principal.Project, taskID, delegation.UserID); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errWorkflowDecisionReviewerForbidden) {
			status = http.StatusForbidden
		}
		s.jsonError(w, status, err.Error())
		return
	}
	outputs := body.Outputs
	if outputs == nil {
		outputs = map[string]string{}
	}
	if decision := normalizeWorkflowReviewDecision(body.Decision); decision != "" {
		outputs["decision"] = decision
	}
	if comments := strings.TrimSpace(body.Comments); comments != "" {
		outputs["comments"] = comments
	}
	resp, status, err := s.submitTaskWorkflowReview(r, principal.WorkspaceID, principal.Project, taskID, workflowReviewBody{
		Decision: body.Decision,
		Comments: body.Comments,
		Outputs:  outputs,
	})
	if err != nil {
		if status == http.StatusInternalServerError {
			s.serverError(w, err)
			return
		}
		s.jsonError(w, status, err.Error())
		return
	}
	if err := s.markInteractionRequestHandled(request, outputs); err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  principal.WorkspaceID,
		ActorType:    "agent",
		ActorID:      runtimeAgentAddress(principal),
		Action:       "runtime.workflow.decision.submit",
		ResourceType: "interaction_request",
		ResourceID:   request.ID,
		Summary:      "Runtime agent submitted a workflow decision from an interaction callback",
		After: map[string]any{
			"delegatedUser": delegation.UserID,
			"interactionId": request.ID,
			"taskId":        taskID,
		},
		Request: r,
	})
	_ = json.NewEncoder(w).Encode(runtimeWorkflowDecisionResponse{
		InteractionID: request.ID,
		SubmittedBy:   delegation.UserID,
		Workflow:      resp,
	})
}

func (s *Server) validateWorkflowDecisionDelegation(w http.ResponseWriter, principal runtimeAgentPrincipal, token, interactionID string) (runtimeDelegationPrincipal, bool) {
	if strings.TrimSpace(token) == "" {
		s.jsonError(w, http.StatusForbidden, "delegation token is required")
		return runtimeDelegationPrincipal{}, false
	}
	delegation, ok := s.validateRuntimeDelegationToken(token)
	if !ok {
		s.jsonError(w, http.StatusForbidden, "invalid or expired delegation token")
		return runtimeDelegationPrincipal{}, false
	}
	if delegation.WorkspaceID != principal.WorkspaceID || delegation.Project != principal.Project || delegation.Agent != principal.Agent {
		s.jsonError(w, http.StatusForbidden, "delegation token does not belong to this runtime agent")
		return runtimeDelegationPrincipal{}, false
	}
	if strings.TrimSpace(delegation.InteractionID) != "" && delegation.InteractionID != interactionID {
		s.jsonError(w, http.StatusForbidden, "delegation token does not match interaction")
		return runtimeDelegationPrincipal{}, false
	}
	if !runtimeDelegationHasScope(delegation, "act_as_user") && !runtimeDelegationHasScope(delegation, "workflow.decision") {
		s.jsonError(w, http.StatusForbidden, "delegation token lacks workflow decision scope")
		return runtimeDelegationPrincipal{}, false
	}
	return delegation, true
}

func runtimeDelegationHasScope(delegation runtimeDelegationPrincipal, scope string) bool {
	for _, value := range delegation.Scopes {
		if strings.TrimSpace(value) == scope {
			return true
		}
	}
	return false
}

func validateRuntimeWorkflowDecisionInteraction(principal runtimeAgentPrincipal, request controldb.InteractionRequest, taskID string) error {
	if strings.TrimSpace(request.ProjectID) != principal.Project || strings.TrimSpace(request.AgentID) != principal.Agent {
		return fmt.Errorf("interaction request does not belong to this runtime agent")
	}
	if strings.TrimSpace(request.CreatedBy) != runtimeAgentAddress(principal) {
		return fmt.Errorf("interaction request was not created by this runtime agent")
	}
	if strings.TrimSpace(request.Status) != "submitted" {
		return fmt.Errorf("interaction request is not submitted")
	}
	if strings.TrimSpace(request.SubmittedBy) == "" {
		return fmt.Errorf("interaction request has no submitted user")
	}
	var contextValues map[string]any
	if strings.TrimSpace(request.ContextJSON) != "" {
		_ = json.Unmarshal([]byte(request.ContextJSON), &contextValues)
	}
	if contextTask, ok := contextValues["taskId"].(string); ok && strings.TrimSpace(contextTask) != "" && strings.TrimSpace(contextTask) != taskID {
		return fmt.Errorf("interaction request is for task %q, not %q", strings.TrimSpace(contextTask), taskID)
	}
	return nil
}

func taskIDFromInteractionContext(contextJSON string) string {
	var contextValues map[string]any
	if strings.TrimSpace(contextJSON) == "" || json.Unmarshal([]byte(contextJSON), &contextValues) != nil {
		return ""
	}
	if taskID, ok := contextValues["taskId"].(string); ok {
		return strings.TrimSpace(taskID)
	}
	return ""
}

var errWorkflowDecisionReviewerForbidden = errors.New("interaction submitter is not the current workflow reviewer")

func (s *Server) validateWorkflowDecisionReviewer(workspaceID, project, taskID, submittedBy string) error {
	wfStore := workflowstore.NewStore(s.controlDB, workspaceID)
	run, found, err := wfStore.RunForTask(project, taskID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("workflow run not found for task %q", taskID)
	}
	def, found, err := wfStore.RunDefinition(run)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("workflow definition not found")
	}
	step, found := workflowDefinitionStepByID(def.Steps, run.ActiveStepID)
	if !found {
		return fmt.Errorf("active workflow step not found")
	}
	if strings.TrimSpace(step.Type) != "human_review" {
		return fmt.Errorf("active workflow step is not a human review step")
	}
	instances, err := wfStore.ListStepInstances(run.ID)
	if err != nil {
		return err
	}
	for _, inst := range instances {
		if inst.StepID != run.ActiveStepID {
			continue
		}
		if !workflowReviewActorTypeIsHuman(inst.ActorType) {
			return errWorkflowDecisionReviewerForbidden
		}
		if strings.TrimSpace(inst.ActorID) != strings.TrimSpace(submittedBy) {
			return errWorkflowDecisionReviewerForbidden
		}
		if !workflowStepInstanceOpen(inst.Status) {
			return fmt.Errorf("workflow review step is not open")
		}
		return nil
	}
	return fmt.Errorf("active workflow step instance not found")
}

func workflowReviewActorTypeIsHuman(actorType string) bool {
	switch strings.TrimSpace(actorType) {
	case "human", "user":
		return true
	default:
		return false
	}
}

func (s *Server) markInteractionRequestHandled(request controldb.InteractionRequest, outputs map[string]string) error {
	request.Status = "handled"
	handled := map[string]any{
		"handledAt": time.Now().UTC().Format(time.RFC3339),
		"outputs":   outputs,
	}
	if strings.TrimSpace(request.SubmissionJSON) != "" {
		var submission map[string]any
		if err := json.Unmarshal([]byte(request.SubmissionJSON), &submission); err == nil {
			for key, value := range handled {
				submission[key] = value
			}
			raw, err := json.Marshal(submission)
			if err == nil {
				request.SubmissionJSON = string(raw)
			}
		}
	}
	if request.SubmissionJSON == "" {
		raw, _ := json.Marshal(handled)
		request.SubmissionJSON = string(raw)
	}
	return s.controlDB.UpdateInteractionRequest(request)
}
