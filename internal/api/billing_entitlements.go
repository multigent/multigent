package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/tasktemplate"
)

type workspaceEntitlements struct {
	PlanCode           string `json:"planCode,omitempty"`
	BillingStatus      string `json:"billingStatus,omitempty"`
	TrialEndsAt        string `json:"trialEndsAt,omitempty"`
	SeatsUsed          int    `json:"seatsUsed,omitempty"`
	WorkspaceLimit     int    `json:"workspaceLimit,omitempty"`
	SeatLimit          int    `json:"seatLimit,omitempty"`
	AgentLimit         int    `json:"agentLimit,omitempty"`
	RuntimeNodeLimit   int    `json:"runtimeNodeLimit,omitempty"`
	TaskTemplateLimit  int    `json:"taskTemplateLimit,omitempty"`
	ConnectionLimit    int    `json:"connectionLimit,omitempty"`
	AuditRetentionDays int    `json:"auditRetentionDays,omitempty"`
	AdvancedRBAC       bool   `json:"advancedRBAC,omitempty"`
	MonthlyRunLimit    int    `json:"monthlyRunLimit,omitempty"`
}

type entitlementUsage struct {
	Seats         int `json:"seats"`
	PendingSeats  int `json:"pendingSeats"`
	Agents        int `json:"agents"`
	RuntimeNodes  int `json:"runtimeNodes"`
	TaskTemplates int `json:"taskTemplates"`
	Connections   int `json:"connections"`
}

func (s *Server) currentEntitlements(r *http.Request) workspaceEntitlements {
	if r != nil {
		if ent, ok := r.Context().Value(ctxEntitlementsKey).(workspaceEntitlements); ok {
			return ent
		}
	}
	return workspaceEntitlements{
		PlanCode:      "local",
		BillingStatus: "unlimited",
	}
}

func decodeTrustedProxyEntitlements(raw string) (*workspaceEntitlements, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var ent workspaceEntitlements
	if err := json.Unmarshal([]byte(raw), &ent); err != nil {
		return nil, err
	}
	return &ent, nil
}

func (s *Server) entitlementUsage(workspaceID string) (entitlementUsage, error) {
	usage := entitlementUsage{}
	if s.controlDB != nil && strings.TrimSpace(workspaceID) != "" {
		members, err := s.controlDB.ListWorkspaceMembers(workspaceID)
		if err != nil {
			return usage, err
		}
		usage.Seats = len(members)
		if s.users != nil {
			invitations, err := s.users.ListInvitations(workspaceID)
			if err != nil {
				return usage, err
			}
			for _, inv := range invitations {
				if strings.EqualFold(inv.Status, "pending") {
					usage.PendingSeats++
				}
			}
		}
		nodes, err := s.controlDB.ListRuntimeNodes(workspaceID)
		if err != nil {
			return usage, err
		}
		usage.RuntimeNodes = len(nodes)
		connections, err := s.controlDB.ListConnections(controldb.ConnectionFilter{WorkspaceID: workspaceID, Status: "active"})
		if err != nil {
			return usage, err
		}
		usage.Connections = len(connections)
	}
	if s.controlDB != nil && strings.TrimSpace(workspaceID) != "" {
		workers, err := s.controlDB.ListAgentWorkers(workspaceID)
		if err != nil {
			return usage, err
		}
		for _, worker := range workers {
			if entity.NormaliseModel(entity.AgentModel(worker.Model)) == entity.ModelHuman {
				continue
			}
			usage.Agents++
		}
		templates, err := tasktemplate.NewStore(s.controlDB, workspaceID).List()
		if err != nil {
			return usage, err
		}
		usage.TaskTemplates = len(templates)
	}
	return usage, nil
}

func entitlementLimitExceeded(limit, current, adding int) bool {
	return limit > 0 && current+adding > limit
}

func (s *Server) rejectEntitlementLimit(w http.ResponseWriter, resource string, limit, current, adding int) {
	s.jsonErrorDetails(w, http.StatusPaymentRequired, ErrCodeBillingLimitExceeded, fmt.Sprintf("%s limit exceeded", resource), map[string]any{
		"resource": resource,
		"limit":    limit,
		"current":  current,
		"adding":   adding,
	})
}

func (s *Server) checkSeatEntitlement(w http.ResponseWriter, r *http.Request, workspaceID string, adding int, includePending bool) bool {
	ent := s.currentEntitlements(r)
	if ent.SeatLimit <= 0 {
		return true
	}
	usage, err := s.entitlementUsage(workspaceID)
	if err != nil {
		s.serverError(w, err)
		return false
	}
	current := usage.Seats
	if includePending {
		current += usage.PendingSeats
	}
	if entitlementLimitExceeded(ent.SeatLimit, current, adding) {
		s.rejectEntitlementLimit(w, "seats", ent.SeatLimit, current, adding)
		return false
	}
	return true
}

func (s *Server) checkAgentEntitlement(w http.ResponseWriter, r *http.Request, adding int) bool {
	ent := s.currentEntitlements(r)
	if ent.AgentLimit <= 0 {
		return true
	}
	workspaceID, _ := s.currentWorkspaceID()
	usage, err := s.entitlementUsage(workspaceID)
	if err != nil {
		s.serverError(w, err)
		return false
	}
	if entitlementLimitExceeded(ent.AgentLimit, usage.Agents, adding) {
		s.rejectEntitlementLimit(w, "agents", ent.AgentLimit, usage.Agents, adding)
		return false
	}
	return true
}

func (s *Server) checkRuntimeNodeEntitlement(w http.ResponseWriter, r *http.Request, workspaceID string, adding int) bool {
	ent := s.currentEntitlements(r)
	if ent.RuntimeNodeLimit <= 0 {
		return true
	}
	usage, err := s.entitlementUsage(workspaceID)
	if err != nil {
		s.serverError(w, err)
		return false
	}
	if entitlementLimitExceeded(ent.RuntimeNodeLimit, usage.RuntimeNodes, adding) {
		s.rejectEntitlementLimit(w, "runtime_nodes", ent.RuntimeNodeLimit, usage.RuntimeNodes, adding)
		return false
	}
	return true
}

func (s *Server) checkTaskTemplateEntitlement(w http.ResponseWriter, r *http.Request, adding int) bool {
	ent := s.currentEntitlements(r)
	if ent.TaskTemplateLimit <= 0 {
		return true
	}
	workspaceID, _ := s.currentWorkspaceID()
	usage, err := s.entitlementUsage(workspaceID)
	if err != nil {
		s.serverError(w, err)
		return false
	}
	if entitlementLimitExceeded(ent.TaskTemplateLimit, usage.TaskTemplates, adding) {
		s.rejectEntitlementLimit(w, "task_templates", ent.TaskTemplateLimit, usage.TaskTemplates, adding)
		return false
	}
	return true
}

func (s *Server) checkConnectionEntitlement(w http.ResponseWriter, r *http.Request, workspaceID string, adding int) bool {
	ent := s.currentEntitlements(r)
	if ent.ConnectionLimit <= 0 {
		return true
	}
	usage, err := s.entitlementUsage(workspaceID)
	if err != nil {
		s.serverError(w, err)
		return false
	}
	if entitlementLimitExceeded(ent.ConnectionLimit, usage.Connections, adding) {
		s.rejectEntitlementLimit(w, "connections", ent.ConnectionLimit, usage.Connections, adding)
		return false
	}
	return true
}

func (s *Server) handleBillingEntitlements(w http.ResponseWriter, r *http.Request) {
	workspaceID, _ := s.currentWorkspaceID()
	usage, err := s.entitlementUsage(workspaceID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if r.Header.Get("X-Multigent-Example-Workspace") == "1" {
		usage = entitlementUsage{}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"entitlements": s.currentEntitlements(r),
		"usage":        usage,
	})
}
