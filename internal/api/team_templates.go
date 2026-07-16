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
	newSimpleTeamTemplate(
		"product-discovery",
		"Product Discovery Team",
		"Product team for turning market signals, user problems, and business goals into validated roadmap scope.",
		"product",
		"Product discovery and roadmap team.",
		[]string{
			"Find real user and business problems before committing engineering capacity.",
			"Turn ambiguous opportunities into validated bets, roadmap items, and acceptance criteria.",
			"Keep scope, evidence, risks, and success metrics explicit.",
		},
		[]roleTemplateSpec{
			simpleRole("product-strategist", "Owns product direction, opportunity framing, roadmap trade-offs, and strategic alignment."),
			simpleRole("user-researcher", "Owns customer interviews, usability findings, evidence quality, and insight synthesis."),
			simpleRole("product-analyst", "Owns product metrics, funnel analysis, cohort trends, and experiment readouts."),
			simpleRole("prd-writer", "Turns product decisions into clear PRDs, acceptance criteria, non-goals, and launch notes."),
		},
	),
	newSimpleTeamTemplate(
		"growth-business",
		"Growth & Business Team",
		"Commercial team for pipeline, partnerships, pricing, campaigns, CRM hygiene, and revenue experiments.",
		"business",
		"Growth, sales, partnerships, and revenue operations team.",
		[]string{
			"Create repeatable demand and revenue motions instead of one-off sales effort.",
			"Keep customer pipeline, pricing, experiments, and follow-ups measurable.",
			"Coordinate market messaging, partnerships, and customer feedback with product.",
		},
		[]roleTemplateSpec{
			simpleRole("growth-marketer", "Owns acquisition experiments, campaign planning, messaging tests, and channel performance."),
			simpleRole("sales-operator", "Owns CRM hygiene, lead qualification, follow-ups, pipeline reporting, and sales handoffs."),
			simpleRole("partnership-manager", "Owns partner research, outreach plans, collaboration proposals, and relationship tracking."),
			simpleRole("revenue-analyst", "Owns pricing analysis, revenue metrics, conversion tracking, and business experiment readouts."),
		},
	),
	newSimpleTeamTemplate(
		"customer-operations",
		"Customer Operations Team",
		"Operations team for support, onboarding, customer feedback loops, incident communication, and knowledge base maintenance.",
		"operations",
		"Customer support, onboarding, and operational quality team.",
		[]string{
			"Reduce customer friction through fast support, clear onboarding, and useful documentation.",
			"Turn repeated support issues into product feedback, knowledge base updates, or automation.",
			"Keep customer-facing communication accurate, timely, and reviewable.",
		},
		[]roleTemplateSpec{
			simpleRole("support-specialist", "Owns support triage, customer replies, issue classification, and escalation quality."),
			simpleRole("onboarding-specialist", "Owns customer onboarding flows, setup checklists, adoption blockers, and early success signals."),
			simpleRole("knowledge-manager", "Owns help center content, internal runbooks, FAQ quality, and documentation freshness."),
			simpleRole("incident-communicator", "Owns customer-facing incident updates, status notes, postmortem summaries, and communication timing."),
		},
	),
	newSimpleTeamTemplate(
		"agent-enablement",
		"Agent Enablement Team",
		"Internal platform team for agent context, skills, prompts, tool access, evaluation, and workflow improvement.",
		"enablement",
		"Agent workforce enablement and workflow improvement team.",
		[]string{
			"Make agents easier to onboard, steer, evaluate, and reuse across projects.",
			"Turn repeated human intervention into prompts, skills, policies, or product improvements.",
			"Keep context, tool access, and evaluation loops observable and maintainable.",
		},
		[]roleTemplateSpec{
			simpleRole("agent-ops-manager", "Owns agent staffing, responsibilities, wakeup routines, escalation rules, and operating cadence."),
			simpleRole("prompt-engineer", "Owns role prompts, workflow prompts, prompt tests, and prompt versioning."),
			simpleRole("skill-builder", "Owns reusable skills, tool wrappers, MCP integrations, and agent-facing documentation."),
			simpleRole("eval-engineer", "Owns agent evaluation cases, output review rubrics, regression checks, and quality metrics."),
		},
	),
	newSimpleTeamTemplate(
		"data-insights",
		"Data & Insights Team",
		"Analytics team for metrics definitions, dashboards, experiment analysis, research synthesis, and decision support.",
		"data",
		"Data, analytics, and decision-support team.",
		[]string{
			"Turn scattered operational and product data into decision-ready insights.",
			"Keep metric definitions, dashboards, and experiment readouts trustworthy.",
			"Separate facts, assumptions, and recommendations in every analysis.",
		},
		[]roleTemplateSpec{
			simpleRole("data-analyst", "Owns metric definitions, SQL analysis, dashboard interpretation, and decision notes."),
			simpleRole("analytics-engineer", "Owns data modeling, transformation quality, metric pipelines, and data reliability checks."),
			simpleRole("experiment-analyst", "Owns experiment design review, result analysis, statistical caveats, and recommendation quality."),
			simpleRole("research-synthesizer", "Owns synthesis across qualitative notes, market research, support signals, and product data."),
		},
	),
}

func newSimpleTeamTemplate(id, name, description, teamName, teamDescription string, goals []string, roles []roleTemplateSpec) teamTemplate {
	return teamTemplate{
		ID:          id,
		Name:        name,
		Description: description,
		Team: entity.Team{
			Name:        teamName,
			Description: teamDescription,
			Goals:       goals,
		},
		TeamPrompt: simpleTeamPrompt(name, description, goals),
		Roles:      roles,
	}
}

func simpleRole(name, description string) roleTemplateSpec {
	return roleTemplateSpec{
		Name:        name,
		Description: description,
		Prompt:      simpleRolePrompt(name, description),
	}
}

func simpleTeamPrompt(name, description string, goals []string) string {
	var b strings.Builder
	b.WriteString("# Team: ")
	b.WriteString(name)
	b.WriteString("\n\n")
	b.WriteString(description)
	b.WriteString("\n\nOperating principles:\n")
	for _, goal := range goals {
		b.WriteString("- ")
		b.WriteString(goal)
		b.WriteString("\n")
	}
	b.WriteString("- Record decisions, open questions, risks, and handoff notes so future agents inherit usable context.\n")
	return b.String()
}

func simpleRolePrompt(name, description string) string {
	return fmt.Sprintf(`# Role: %s

%s

Responsibilities:
- Clarify the requested outcome before producing work.
- Keep assumptions, evidence, decisions, and risks explicit.
- Produce concise artifacts that other humans or agents can use directly.
- Escalate only decisions that require human judgment, external commitment, money, or irreversible trade-offs.

Deliverables:
- Clear work output for this role.
- Verification notes or evidence used.
- Open questions and recommended next step.
`, name, description)
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
	if loc, ok := zhTeamTemplateLocalizations[tmpl.ID]; ok {
		localized := tmpl
		localized.Team.Description = loc.TeamDescription
		localized.Team.Goals = loc.Goals
		localized.TeamPrompt = simpleTeamPromptZH(localized.Name, loc.Description, loc.Goals)
		for i := range localized.Roles {
			name := localized.Roles[i].Name
			if desc := loc.RoleDescriptions[name]; desc != "" {
				localized.Roles[i].Description = desc
				localized.Roles[i].Prompt = simpleRolePromptZH(name, desc)
			}
		}
		return localized
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

type zhTeamTemplateLocalization struct {
	Description      string
	TeamDescription  string
	Goals            []string
	RoleDescriptions map[string]string
}

var zhTeamTemplateLocalizations = map[string]zhTeamTemplateLocalization{
	"product-discovery": {
		Description:     "负责把市场信号、用户问题和业务目标转化为经过验证的路线图范围。",
		TeamDescription: "产品发现和路线图团队。",
		Goals: []string{
			"在投入工程资源前找到真实用户问题和业务问题。",
			"把模糊机会转化为经过验证的下注、路线图事项和验收标准。",
			"让范围、证据、风险和成功指标保持明确。",
		},
		RoleDescriptions: map[string]string{
			"product-strategist": "负责产品方向、机会定义、路线图取舍和战略对齐。",
			"user-researcher":    "负责用户访谈、可用性发现、证据质量和洞察综合。",
			"product-analyst":    "负责产品指标、漏斗分析、分群趋势和实验解读。",
			"prd-writer":         "把产品决策转化为清晰的 PRD、验收标准、非目标和发布说明。",
		},
	},
	"growth-business": {
		Description:     "负责线索、合作、定价、活动、CRM 维护和收入实验的商业团队。",
		TeamDescription: "增长、销售、合作和收入运营团队。",
		Goals: []string{
			"建立可重复的需求和收入动作，而不是依赖一次性销售努力。",
			"让客户线索、定价、实验和跟进过程可衡量。",
			"把市场信息、合作机会和客户反馈与产品团队对齐。",
		},
		RoleDescriptions: map[string]string{
			"growth-marketer":     "负责获客实验、活动计划、信息测试和渠道表现。",
			"sales-operator":      "负责 CRM 维护、线索筛选、跟进、pipeline 报告和销售交接。",
			"partnership-manager": "负责合作方研究、触达计划、合作方案和关系跟踪。",
			"revenue-analyst":     "负责定价分析、收入指标、转化跟踪和商业实验解读。",
		},
	},
	"customer-operations": {
		Description:     "负责支持、 onboarding、客户反馈闭环、事故沟通和知识库维护的运营团队。",
		TeamDescription: "客户支持、 onboarding 和运营质量团队。",
		Goals: []string{
			"通过快速支持、清晰 onboarding 和有用文档降低客户摩擦。",
			"把重复支持问题转化为产品反馈、知识库更新或自动化。",
			"让面向客户的沟通准确、及时、可回溯。",
		},
		RoleDescriptions: map[string]string{
			"support-specialist":    "负责支持分流、客户回复、问题分类和升级质量。",
			"onboarding-specialist": "负责客户 onboarding 流程、设置清单、采用阻塞和早期成功信号。",
			"knowledge-manager":     "负责帮助中心内容、内部 runbook、FAQ 质量和文档新鲜度。",
			"incident-communicator": "负责客户侧事故更新、状态说明、复盘摘要和沟通节奏。",
		},
	},
	"agent-enablement": {
		Description:     "负责 Agent 上下文、技能、提示词、工具访问、评估和流程改进的内部平台团队。",
		TeamDescription: "Agent workforce 赋能和流程改进团队。",
		Goals: []string{
			"让 Agent 更容易被雇佣、调教、评估，并跨项目复用。",
			"把重复人工介入沉淀成 prompt、skill、policy 或产品改进。",
			"让上下文、工具访问和评估循环可观察、可维护。",
		},
		RoleDescriptions: map[string]string{
			"agent-ops-manager": "负责 Agent 编制、职责、唤醒例程、升级规则和运行节奏。",
			"prompt-engineer":   "负责角色提示词、流程提示词、提示词测试和版本管理。",
			"skill-builder":     "负责可复用技能、工具封装、MCP 集成和面向 Agent 的文档。",
			"eval-engineer":     "负责 Agent 评估案例、输出评审标准、回归检查和质量指标。",
		},
	},
	"data-insights": {
		Description:     "负责指标定义、仪表盘、实验分析、研究综合和决策支持的数据团队。",
		TeamDescription: "数据、分析和决策支持团队。",
		Goals: []string{
			"把分散的运营和产品数据转化为可决策洞察。",
			"让指标定义、仪表盘和实验解读可信。",
			"在每次分析中区分事实、假设和建议。",
		},
		RoleDescriptions: map[string]string{
			"data-analyst":         "负责指标定义、SQL 分析、仪表盘解读和决策说明。",
			"analytics-engineer":   "负责数据建模、转换质量、指标管线和数据可靠性检查。",
			"experiment-analyst":   "负责实验设计评审、结果分析、统计 caveat 和建议质量。",
			"research-synthesizer": "负责综合定性记录、市场研究、支持信号和产品数据。",
		},
	},
}

func simpleTeamPromptZH(name, description string, goals []string) string {
	var b strings.Builder
	b.WriteString("# Team: ")
	b.WriteString(name)
	b.WriteString("\n\n")
	b.WriteString(description)
	b.WriteString("\n\n工作原则:\n")
	for _, goal := range goals {
		b.WriteString("- ")
		b.WriteString(goal)
		b.WriteString("\n")
	}
	b.WriteString("- 记录决策、开放问题、风险和交接信息，让后续 Agent 继承可执行上下文。\n")
	return b.String()
}

func simpleRolePromptZH(name, description string) string {
	return fmt.Sprintf(`# Role: %s

%s

职责:
- 先澄清目标，再开始产出。
- 明确记录假设、证据、决策和风险。
- 产出能被其他人或 Agent 直接复用的简洁材料。
- 只升级需要人类判断、外部承诺、资金或不可逆取舍的问题。

交付物:
- 符合该角色职责的工作产出。
- 验证说明或使用的证据。
- 开放问题和建议下一步。
`, name, description)
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
