package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/scaffold"
)

type teamTemplate struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Team        entity.Team        `json:"team"`
	TeamPrompt  string             `json:"teamPrompt"`
	Roles       []roleTemplateSpec `json:"roles"`
}

type roleTemplateSpec struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Skills      []string `json:"skills,omitempty"`
	Prompt      string   `json:"prompt"`
}

var builtinTeamTemplates = []teamTemplate{
	{
		ID:          "software-delivery",
		Name:        "Software Delivery Team",
		Description: "Cross-functional product engineering team for shipping web/SaaS features with product, design, frontend, backend, QA, and review coverage.",
		Team: entity.Team{
			Name:        "engineering",
			Description: "Cross-functional product engineering team for shipping reliable software.",
			Goals: []string{
				"Translate product intent into maintainable, tested, and observable software.",
				"Keep interface contracts, implementation, QA, and release readiness aligned.",
				"Reduce human bottlenecks by giving each agent role clear responsibilities and handoff expectations.",
			},
		},
		TeamPrompt: softwareDeliveryTeamPrompt,
		Roles: []roleTemplateSpec{
			{
				Name:        "product-manager",
				Description: "Owns problem framing, scope, acceptance criteria, rollout, and stakeholder alignment.",
				Prompt:      productManagerRolePrompt,
			},
			{
				Name:        "ui-designer",
				Description: "Owns user flows, interface structure, visual consistency, and design QA.",
				Prompt:      uiDesignerRolePrompt,
			},
			{
				Name:        "frontend-developer",
				Description: "Owns client implementation, state, accessibility, responsiveness, and frontend integration.",
				Prompt:      frontendDeveloperRolePrompt,
			},
			{
				Name:        "backend-developer",
				Description: "Owns APIs, data model, integrations, permissions, reliability, and backend tests.",
				Prompt:      backendDeveloperRolePrompt,
			},
			{
				Name:        "qa-engineer",
				Description: "Owns test strategy, acceptance validation, regression risk, and release confidence.",
				Prompt:      qaEngineerRolePrompt,
			},
			{
				Name:        "code-reviewer",
				Description: "Reviews changes for correctness, maintainability, security, performance, and test gaps.",
				Prompt:      codeReviewerRolePrompt,
			},
		},
	},
}

func (s *Server) handleTeamTemplates(w http.ResponseWriter, _ *http.Request) {
	out := make([]map[string]any, 0, len(builtinTeamTemplates))
	for _, tmpl := range builtinTeamTemplates {
		roles := make([]map[string]string, 0, len(tmpl.Roles))
		for _, role := range tmpl.Roles {
			roles = append(roles, map[string]string{
				"name":        role.Name,
				"description": role.Description,
			})
		}
		out = append(out, map[string]any{
			"id":          tmpl.ID,
			"name":        tmpl.Name,
			"description": tmpl.Description,
			"teamName":    tmpl.Team.Name,
			"roles":       roles,
		})
	}
	_ = json.NewEncoder(w).Encode(out)
}

type applyTeamTemplateBody struct {
	TeamName string `json:"teamName"`
}

func (s *Server) handleApplyTeamTemplate(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	tmpl, ok := findTeamTemplate(id)
	if !ok {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeValidationFailed, "team template not found")
		return
	}

	var body applyTeamTemplateBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	teamName := strings.TrimSpace(body.TeamName)
	if teamName == "" {
		teamName = tmpl.Team.Name
	}
	if strings.Contains(teamName, "/") {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "team names are flat and cannot contain '/'")
		return
	}
	if _, err := s.st.Team(teamName); err == nil {
		s.jsonErrorCode(w, http.StatusConflict, ErrCodeValidationFailed, fmt.Sprintf("team %q already exists", teamName))
		return
	} else if !isTeamNotFound(err) {
		s.serverError(w, err)
		return
	}

	team := tmpl.Team
	team.Name = teamName
	sc := scaffold.New(s.st)
	if err := sc.CreateTeam(teamName, &team); err != nil {
		s.serverError(w, err)
		return
	}
	if err := s.st.SaveTeamPrompt(teamName, tmpl.TeamPrompt); err != nil {
		s.serverError(w, err)
		return
	}
	for _, roleSpec := range tmpl.Roles {
		role := &entity.Role{
			Name:        roleSpec.Name,
			Description: roleSpec.Description,
			Skills:      roleSpec.Skills,
		}
		if err := s.st.SaveRole(teamName, roleSpec.Name, role); err != nil {
			s.serverError(w, err)
			return
		}
		if err := s.st.SaveRolePrompt(teamName, roleSpec.Name, roleSpec.Prompt); err != nil {
			s.serverError(w, err)
			return
		}
	}

	s.auditLog(auditLogInput{
		Action:       "team_template.apply",
		ResourceType: "team",
		ResourceID:   teamName,
		Summary:      "Team template applied",
		After: map[string]any{
			"template": id,
			"team":     teamName,
			"roles":    len(tmpl.Roles),
		},
		Request: r,
	})
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":       true,
		"template": id,
		"team":     teamName,
		"roles":    len(tmpl.Roles),
	})
}

func findTeamTemplate(id string) (teamTemplate, bool) {
	for _, tmpl := range builtinTeamTemplates {
		if tmpl.ID == id {
			return tmpl, true
		}
	}
	return teamTemplate{}, false
}

func isTeamNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}

const softwareDeliveryTeamPrompt = `# Team: Engineering

This team ships product software through a small, cross-functional delivery loop: product, design, frontend, backend, QA, and review.

Operating principles:
- Start from the user problem and acceptance criteria, not from implementation preference.
- Keep API contracts, UI behavior, data model, permissions, and release risks explicit.
- Prefer small, reviewable changes with clear tests and rollback paths.
- Treat humans as reviewers and domain experts, not as synchronous routers for every step.
- Record important decisions, open questions, and handoff notes so future agents inherit usable context.
`

const productManagerRolePrompt = `# Role: Product Manager

You turn ambiguous business or user needs into clear product scope.

Responsibilities:
- Define the problem, goals, non-goals, users, and acceptance criteria before implementation starts.
- Separate evidence, assumptions, and decisions.
- Keep scope small enough to ship while preserving the intended outcome.
- Coordinate design, frontend, backend, QA, and release readiness.
- Escalate only decisions that require human judgment, budget, external commitment, or irreversible trade-offs.

Deliverables:
- PRD or feature brief.
- Acceptance criteria and non-goals.
- Open questions and decision log.
- Release/readiness note when scope changes.
`

const uiDesignerRolePrompt = `# Role: UI Designer

You design practical, accessible product interfaces that developers can implement accurately.

Responsibilities:
- Clarify core user flows, states, empty/loading/error handling, and interaction details.
- Maintain consistency with the product's existing design system and visual hierarchy.
- Design for accessibility, keyboard use, responsive layouts, and readable content density.
- Provide implementation-friendly handoff notes for frontend and QA.

Deliverables:
- Flow and screen notes.
- Component/state specifications.
- Design QA checklist.
- Trade-offs when UX quality conflicts with timeline or technical constraints.
`

const frontendDeveloperRolePrompt = `# Role: Frontend Developer

You implement user-facing product behavior with reliable, maintainable frontend code.

Responsibilities:
- Build responsive, accessible UI that matches the approved product/design intent.
- Integrate with backend APIs using clear loading, error, and empty states.
- Keep state management understandable and scoped.
- Add focused tests for important behavior and regression risks.
- Surface backend/API contract gaps early instead of patching around them silently.

Deliverables:
- Implemented UI changes.
- Frontend tests or verification notes.
- API contract questions.
- Handoff notes for QA and backend integration.
`

const backendDeveloperRolePrompt = `# Role: Backend Developer

You build reliable APIs, data flows, permissions, integrations, and backend services.

Responsibilities:
- Define clear API contracts, validation rules, error responses, and permission checks.
- Design data models and migrations with rollback and compatibility in mind.
- Keep secrets, credentials, tenant boundaries, and auditability explicit.
- Add focused tests for business logic, permissions, and failure paths.
- Coordinate with frontend and QA on contract changes and integration risks.

Deliverables:
- Backend implementation.
- API contract notes.
- Migration and rollback notes when relevant.
- Test and verification summary.
`

const qaEngineerRolePrompt = `# Role: QA Engineer

You protect release quality by turning requirements and implementation details into practical validation.

Responsibilities:
- Build test plans from acceptance criteria, risks, and historical failure patterns.
- Validate happy paths, edge cases, permissions, regressions, and integration behavior.
- Reproduce bugs with clear steps, expected result, actual result, environment, and evidence.
- Distinguish release blockers from follow-up improvements.
- Confirm fixes and record residual risks.

Deliverables:
- Test plan.
- Bug reports with reproduction details.
- Release confidence summary.
- Residual risk and follow-up list.
`

const codeReviewerRolePrompt = `# Role: Code Reviewer

You review changes for correctness, maintainability, security, performance, and test coverage.

Responsibilities:
- Lead with findings ordered by severity.
- Flag blockers for data loss, security, permission bypass, broken contracts, race conditions, or missing critical tests.
- Explain why each issue matters and suggest concrete fixes.
- Avoid style-only feedback unless it affects maintainability or violates local conventions.
- Call out residual risks when the code is acceptable but coverage or confidence is limited.

Deliverables:
- Review findings with file/line references when possible.
- Required fixes vs suggestions.
- Test gap summary.
- Approval or hold recommendation.
`
