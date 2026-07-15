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
	Locale   string `json:"locale"`
}

type createTeamBody struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Owners      []string `json:"owners"`
	Skills      []string `json:"skills"`
	TemplateID  string   `json:"templateId"`
	Locale      string   `json:"locale"`
}

func (s *Server) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	var body createTeamBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	teamName := strings.TrimSpace(body.Name)
	if teamName == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "team name is required")
		return
	}
	if strings.Contains(teamName, "/") {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "team names are flat and cannot contain '/'")
		return
	}

	templateID := strings.TrimSpace(body.TemplateID)
	var (
		tmpl *teamTemplate
		ok   bool
	)
	if templateID != "" {
		found, foundOK := findTeamTemplate(templateID)
		if !foundOK {
			s.jsonErrorCode(w, http.StatusNotFound, ErrCodeValidationFailed, "team template not found")
			return
		}
		tmpl = &found
		ok = true
	}

	if ok {
		if err := s.createTeamFromTemplate(r, teamName, *tmpl, strings.TrimSpace(body.Locale)); err != nil {
			s.writeCreateTeamError(w, teamName, err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":       true,
			"team":     teamName,
			"template": templateID,
			"roles":    len(tmpl.Roles),
		})
		return
	}

	if err := s.ensureTeamNameAvailable(teamName); err != nil {
		s.writeCreateTeamError(w, teamName, err)
		return
	}
	team := &entity.Team{
		Name:        teamName,
		Description: strings.TrimSpace(body.Description),
		Owners:      body.Owners,
		Skills:      body.Skills,
	}
	sc := scaffold.New(s.st)
	if err := sc.CreateTeam(teamName, team); err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		Action:       "team.create",
		ResourceType: "team",
		ResourceID:   teamName,
		Summary:      "Team created",
		After: map[string]any{
			"team": teamName,
		},
		Request: r,
	})
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":   true,
		"team": teamName,
	})
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
	if err := s.createTeamFromTemplate(r, teamName, tmpl, strings.TrimSpace(body.Locale)); err != nil {
		s.writeCreateTeamError(w, teamName, err)
		return
	}

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

func (s *Server) createTeamFromTemplate(r *http.Request, teamName string, tmpl teamTemplate, locale string) error {
	if strings.Contains(teamName, "/") {
		return fmt.Errorf("validation: team names are flat and cannot contain '/'")
	}
	if err := s.ensureTeamNameAvailable(teamName); err != nil {
		return err
	}
	localized := localizeTeamTemplate(tmpl, locale)
	team := localized.Team
	team.Name = teamName
	sc := scaffold.New(s.st)
	if err := sc.CreateTeam(teamName, &team); err != nil {
		return err
	}
	if err := s.st.SaveTeamPrompt(teamName, localized.TeamPrompt); err != nil {
		return err
	}
	for _, roleSpec := range localized.Roles {
		role := &entity.Role{
			Name:        roleSpec.Name,
			Description: roleSpec.Description,
			Skills:      roleSpec.Skills,
		}
		if err := s.st.SaveRole(teamName, roleSpec.Name, role); err != nil {
			return err
		}
		if err := s.st.SaveRolePrompt(teamName, roleSpec.Name, roleSpec.Prompt); err != nil {
			return err
		}
	}
	s.auditLog(auditLogInput{
		Action:       "team_template.apply",
		ResourceType: "team",
		ResourceID:   teamName,
		Summary:      "Team template applied",
		After: map[string]any{
			"template": tmpl.ID,
			"team":     teamName,
			"locale":   locale,
			"roles":    len(tmpl.Roles),
		},
		Request: r,
	})
	return nil
}

func (s *Server) ensureTeamNameAvailable(teamName string) error {
	if _, err := s.st.Team(teamName); err == nil {
		return fmt.Errorf("conflict: team %q already exists", teamName)
	} else if !isTeamNotFound(err) {
		return err
	}
	return nil
}

func (s *Server) writeCreateTeamError(w http.ResponseWriter, teamName string, err error) {
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "conflict:"):
		s.jsonErrorCode(w, http.StatusConflict, ErrCodeValidationFailed, fmt.Sprintf("team %q already exists", teamName))
	case strings.HasPrefix(msg, "validation:"):
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, strings.TrimPrefix(msg, "validation: "))
	default:
		s.serverError(w, err)
	}
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

func localizeTeamTemplate(tmpl teamTemplate, locale string) teamTemplate {
	if !strings.HasPrefix(strings.ToLower(locale), "zh") {
		return tmpl
	}
	if tmpl.ID != "software-delivery" {
		return tmpl
	}
	localized := tmpl
	localized.Team.Description = "面向软件交付的跨职能团队，覆盖产品、设计、前端、后端、测试和代码评审。"
	localized.Team.Goals = []string{
		"把产品意图转化为可维护、可测试、可观测的软件交付。",
		"让接口契约、界面行为、数据模型、权限和发布风险保持一致。",
		"通过清晰的角色职责和交接机制，减少人作为同步路由器造成的阻塞。",
	}
	localized.TeamPrompt = softwareDeliveryTeamPromptZH
	roleDescriptions := map[string]string{
		"product-manager":    "负责问题定义、范围控制、验收标准、发布节奏和干系人对齐。",
		"ui-designer":        "负责用户流程、界面结构、视觉一致性、交互状态和设计验收。",
		"frontend-developer": "负责前端实现、状态管理、可访问性、响应式表现和接口联调。",
		"backend-developer":  "负责 API、数据模型、集成、权限、可靠性和后端测试。",
		"qa-engineer":        "负责测试策略、验收验证、回归风险和发布信心。",
		"code-reviewer":      "负责从正确性、可维护性、安全性、性能和测试缺口角度评审代码。",
	}
	rolePrompts := map[string]string{
		"product-manager":    productManagerRolePromptZH,
		"ui-designer":        uiDesignerRolePromptZH,
		"frontend-developer": frontendDeveloperRolePromptZH,
		"backend-developer":  backendDeveloperRolePromptZH,
		"qa-engineer":        qaEngineerRolePromptZH,
		"code-reviewer":      codeReviewerRolePromptZH,
	}
	for i := range localized.Roles {
		name := localized.Roles[i].Name
		if desc := roleDescriptions[name]; desc != "" {
			localized.Roles[i].Description = desc
		}
		if prompt := rolePrompts[name]; prompt != "" {
			localized.Roles[i].Prompt = prompt
		}
	}
	return localized
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

const softwareDeliveryTeamPromptZH = `# Team: Engineering

这个团队负责通过产品、设计、前端、后端、测试和评审组成的小型跨职能闭环交付软件。

工作原则:
- 先从用户问题和验收标准出发，而不是从实现偏好出发。
- 明确 API 契约、UI 行为、数据模型、权限和发布风险。
- 优先交付小而可评审的变更，并保留清晰的测试和回滚路径。
- 让人主要承担评审、判断和领域知识输入，而不是成为每一步的同步路由器。
- 记录重要决策、开放问题和交接信息，让后续 Agent 继承可执行上下文。
`

const productManagerRolePromptZH = `# Role: Product Manager

你负责把模糊的业务或用户需求转化为清晰的产品范围。

职责:
- 在进入实现前定义问题、目标、非目标、用户和验收标准。
- 区分证据、假设和已确认决策。
- 控制范围，让需求足够小而可交付，同时不牺牲核心目标。
- 协调设计、前端、后端、测试和发布准备。
- 只升级需要人类判断、预算、外部承诺或不可逆取舍的问题。

交付物:
- PRD 或功能 brief。
- 验收标准和非目标。
- 开放问题和决策记录。
- 范围变化时的发布/就绪说明。
`

const uiDesignerRolePromptZH = `# Role: UI Designer

你负责设计务实、可访问、可被开发准确实现的产品界面。

职责:
- 澄清核心用户流程、状态、空态、加载、错误和交互细节。
- 保持与现有设计系统和视觉层级一致。
- 关注可访问性、键盘使用、响应式布局和信息密度。
- 提供便于前端和 QA 执行的交接说明。

交付物:
- 流程和页面说明。
- 组件和状态规格。
- 设计验收清单。
- 当体验质量与时间或技术限制冲突时，记录取舍。
`

const frontendDeveloperRolePromptZH = `# Role: Frontend Developer

你负责用可靠、可维护的前端代码实现面向用户的产品行为。

职责:
- 构建响应式、可访问、符合产品和设计意图的界面。
- 集成后端 API，并提供清晰的加载、错误和空状态。
- 保持状态管理简单、可理解、边界清晰。
- 为重要行为和回归风险补充聚焦测试。
- 及早暴露后端/API 契约问题，而不是静默绕过。

交付物:
- 已实现的 UI 改动。
- 前端测试或验证说明。
- API 契约问题。
- 面向 QA 和后端联调的交接说明。
`

const backendDeveloperRolePromptZH = `# Role: Backend Developer

你负责构建可靠的 API、数据流、权限、集成和后端服务。

职责:
- 定义清晰的 API 契约、校验规则、错误响应和权限检查。
- 设计数据模型和迁移，并考虑回滚和兼容性。
- 明确处理密钥、凭证、租户边界和审计性。
- 为业务逻辑、权限和失败路径补充聚焦测试。
- 与前端和 QA 对齐契约变更和集成风险。

交付物:
- 后端实现。
- API 契约说明。
- 涉及迁移时的迁移和回滚说明。
- 测试和验证总结。
`

const qaEngineerRolePromptZH = `# Role: QA Engineer

你负责把需求和实现细节转化为可执行的验证，保护发布质量。

职责:
- 基于验收标准、风险和历史问题设计测试计划。
- 验证主流程、边界情况、权限、回归和集成行为。
- 用清晰步骤、期望结果、实际结果、环境和证据复现问题。
- 区分发布阻塞和后续改进。
- 确认修复结果并记录残余风险。

交付物:
- 测试计划。
- 带复现细节的缺陷报告。
- 发布信心总结。
- 残余风险和后续事项。
`

const codeReviewerRolePromptZH = `# Role: Code Reviewer

你从正确性、可维护性、安全性、性能和测试覆盖角度评审变更。

职责:
- 按严重程度优先列出发现的问题。
- 对数据丢失、安全、权限绕过、契约破坏、竞态或关键测试缺失提出阻塞意见。
- 说明每个问题为什么重要，并给出具体修复建议。
- 除非影响可维护性或违反本地约定，否则避免只提风格问题。
- 当代码可以接受但覆盖率或信心有限时，明确残余风险。

交付物:
- 带文件/行号的问题清单。
- 必须修复项与建议项。
- 测试缺口总结。
- 通过或暂缓建议。
`
