package playbook

import (
	"strings"

	"github.com/multigent/multigent/internal/entity"
	workflowstore "github.com/multigent/multigent/internal/workflow"
)

const (
	playbookTemplateVersion       = "1.0.0"
	gstackPlaybookTemplateVersion = "1.1.0"
	mattPocockTemplateVersion     = "1.0.0"
)

func Templates(locale string) []entity.PlaybookTemplate {
	locale = normalizeLocale(locale)
	return []entity.PlaybookTemplate{
		softwareDelivery(locale),
		startupValidation(locale),
		mattPocockEngineering(locale),
		bugTriageAndFix(locale),
		supportKnowledgeLoop(locale),
	}
}

func Template(id, locale string) (entity.PlaybookTemplate, bool) {
	for _, tmpl := range Templates(locale) {
		if tmpl.ID == id {
			return tmpl, true
		}
	}
	return entity.PlaybookTemplate{}, false
}

func normalizeLocale(locale string) string {
	locale = strings.ToLower(strings.TrimSpace(locale))
	if strings.HasPrefix(locale, "zh") {
		return "zh-CN"
	}
	return "en"
}

func text(locale string, en, zh string) string {
	if normalizeLocale(locale) == "zh-CN" {
		return zh
	}
	return en
}

func softwareDelivery(locale string) entity.PlaybookTemplate {
	wf, _ := workflowstore.Template("agentic-software-delivery", locale)
	return entity.PlaybookTemplate{
		ID:          "agentic-software-delivery",
		Version:     playbookTemplateVersion,
		Name:        text(locale, "Agentic Software Delivery", "Agentic 研发交付协作方案"),
		Description: text(locale, "A practical delivery playbook for product, engineering, QA, and release work with agent-first execution and human review gates.", "面向产品、研发、QA 和发版的实用协作方案：Agent 主动执行，人类在关键节点审核。"),
		Locale:      normalizeLocale(locale),
		Category:    text(locale, "Software Delivery", "研发交付"),
		Complexity:  text(locale, "Advanced", "进阶"),
		Tags:        []string{"software", "delivery", "product", "qa"},
		Roles: []entity.PlaybookRoleTemplate{
			role("pm-agent", "product", "pm", text(locale, "PM Agent", "产品 Agent"), text(locale, "Turns ambiguous requests into scoped specs, acceptance criteria, and non-goals.", "把模糊需求整理成明确范围、验收标准和非目标。"), []string{"multigent-docs", "task-management"}),
			role("developer-agent", "engineering", "developer", text(locale, "Developer Agent", "开发 Agent"), text(locale, "Implements approved specs with clear changes, tests, and risk notes.", "按已审核方案实现代码，输出变更、测试和风险说明。"), []string{"task-management", "code-review"}),
			role("qa-agent", "engineering", "qa", text(locale, "QA Agent", "QA Agent"), text(locale, "Creates and executes test cases, then reports evidence and remaining manual checks.", "生成并执行测试用例，报告证据和剩余人工检查点。"), []string{"qa-checklist", "multigent-docs"}),
			role("release-agent", "engineering", "release", text(locale, "Release Agent", "发布 Agent"), text(locale, "Prepares release notes and post-release observation reports.", "准备发布说明和上线后观察报告。"), []string{"release-checklist"}),
		},
		Skills: []entity.PlaybookSkillTemplate{
			skill("multigent-docs", text(locale, "Multigent Docs", "Multigent 知识库"), text(locale, "Create and reference docID-based knowledge artifacts instead of local paths.", "创建和引用基于 docID 的知识库产物，避免暴露本地路径。")),
			skill("task-management", text(locale, "Task Management", "任务管理"), text(locale, "Use structured task and workflow step outputs for handoff.", "使用结构化任务和流程节点输出进行交接。")),
			skill("code-review", text(locale, "Code Review", "代码审核"), text(locale, "Review implementation against spec, tests, risk, and maintainability.", "按规格、测试、风险和可维护性审核实现。")),
			skill("qa-checklist", text(locale, "QA Checklist", "QA 检查清单"), text(locale, "Design tests across happy path, edge cases, regression, and manual verification.", "覆盖主路径、边界条件、回归和人工验证点。")),
			skill("release-checklist", text(locale, "Release Checklist", "发布检查清单"), text(locale, "Prepare rollout notes, monitoring checks, rollback notes, and follow-up items.", "准备上线说明、监控检查、回滚说明和后续事项。")),
		},
		Workflows: []entity.PlaybookWorkflowTemplate{
			workflow("agentic-software-delivery", wf, map[string]string{
				"requirement_draft": "pm-agent",
				"prd_draft":         "pm-agent",
				"tech_spec_draft":   "developer-agent",
				"implementation":    "developer-agent",
				"qa":                "qa-agent",
				"release":           "release-agent",
			}, map[string][]string{
				"requirement_draft": {"multigent-docs", "task-management"},
				"implementation":    {"task-management", "code-review"},
				"qa":                {"qa-checklist", "multigent-docs"},
				"release":           {"release-checklist"},
			}),
		},
		TaskTemplates: []entity.PlaybookTaskTemplate{
			task("feature-delivery", text(locale, "Deliver a feature", "交付一个功能"), text(locale, "Use the full software delivery workflow for a scoped feature.", "使用完整研发交付流程推进一个明确功能。"), "agentic-software-delivery"),
			task("bug-fix", text(locale, "Fix a bug", "修复一个 Bug"), text(locale, "Reproduce, diagnose, fix, test, and report risk.", "复现、定位、修复、测试并报告风险。"), "agentic-software-delivery"),
		},
		RequiredTools: []entity.PlaybookToolRequirement{
			tool("github", "GitHub", text(locale, "Repository, PR, issue, and CI access.", "代码仓库、PR、Issue 和 CI 访问。"), false),
			tool("feishu", text(locale, "Feishu", "飞书"), text(locale, "Docs, review notifications, and stakeholder collaboration.", "文档、评审通知和干系人协作。"), false),
		},
		SetupQuestions: []entity.PlaybookSetupQuestion{
			question("code_host", text(locale, "Where is code hosted?", "代码托管在哪里？"), []string{"GitHub", "GitLab", "Other"}, true),
			question("review_policy", text(locale, "Which stages require human review?", "哪些阶段需要人工审核？"), []string{"Spec + code + QA", "Code + QA", "QA only"}, true),
		},
		SuccessMetrics: []entity.PlaybookMetric{
			metric("human_interventions", text(locale, "Human interventions", "人工介入次数"), text(locale, "How many times humans had to unblock or correct the workflow.", "人类需要解除阻塞或纠偏的次数。")),
			metric("cycle_time", text(locale, "Cycle time", "流程耗时"), text(locale, "Elapsed time from task creation to workflow completion.", "从任务创建到流程完成的总耗时。")),
		},
	}
}

func startupValidation(locale string) entity.PlaybookTemplate {
	wf := startupValidationWorkflow(locale)
	return entity.PlaybookTemplate{
		ID:          "garry-startup-validation",
		Version:     gstackPlaybookTemplateVersion,
		Name:        text(locale, "YC Garry Startup Validation", "YC Garry 创业验证协作方案"),
		Description: text(locale, "A startup and product validation playbook built from the actual gstack skill sources: office hours, CEO review, engineering review, design review, spec, review, QA, and ship.", "基于 gstack 原始 Skill 文件构建的创业与产品验证协作方案：包含 office hours、CEO review、工程评审、设计评审、spec、review、QA 和 ship。"),
		Locale:      normalizeLocale(locale),
		Category:    text(locale, "Strategy", "战略验证"),
		Complexity:  text(locale, "Advanced", "高阶"),
		Tags:        []string{"startup", "market", "yc", "strategy", "gstack"},
		Roles: []entity.PlaybookRoleTemplate{
			roleWithPrompt("yc-office-hours-partner", "business", "office-hours", text(locale, "YC Office Hours Partner", "YC Office Hours Partner"), text(locale, "Runs gstack /office-hours: diagnose demand reality, status quo, specificity, wedge, observation, and future fit before implementation.", "执行 gstack /office-hours：在实现前诊断真实需求、现状替代方案、具体痛点、切入点、观察证据和未来适配。"), gstackRolePrompt("office-hours"), []string{"office-hours"}),
			roleWithPrompt("yc-founder-reviewer", "business", "founder-review", text(locale, "CEO / Founder Reviewer", "CEO / Founder Reviewer"), text(locale, "Runs gstack /plan-ceo-review: challenge scope, find the 10-star version, and choose expansion, reduction, or hold-scope mode.", "执行 gstack /plan-ceo-review：挑战范围，寻找 10-star 版本，并选择扩张、缩减或保持范围。"), gstackRolePrompt("plan-ceo-review"), []string{"plan-ceo-review"}),
			roleWithPrompt("gstack-eng-reviewer", "engineering", "engineering-review", text(locale, "Engineering Reviewer", "工程评审"), text(locale, "Runs gstack /plan-eng-review to lock architecture, data flow, failure paths, tests, and observability before build.", "执行 gstack /plan-eng-review：在开发前锁定架构、数据流、失败路径、测试和可观测性。"), gstackRolePrompt("plan-eng-review"), []string{"plan-eng-review"}),
			roleWithPrompt("gstack-design-reviewer", "product", "design-review", text(locale, "Design Reviewer", "设计评审"), text(locale, "Runs gstack /plan-design-review to catch UX, hierarchy, empty states, edge cases, and AI slop before implementation.", "执行 gstack /plan-design-review：在实现前检查 UX、层级、空状态、边界情况和 AI slop。"), gstackRolePrompt("plan-design-review"), []string{"plan-design-review"}),
			roleWithPrompt("gstack-spec-author", "product", "spec", text(locale, "Spec Author", "Spec Author"), text(locale, "Runs gstack /spec to turn vague intent into a precise executable spec with quality gates.", "执行 gstack /spec：把模糊意图转成带质量门禁的精确可执行 spec。"), gstackRolePrompt("spec"), []string{"spec"}),
			roleWithPrompt("gstack-staff-reviewer", "engineering", "staff-review", text(locale, "Staff Engineer Reviewer", "Staff Engineer Reviewer"), text(locale, "Runs gstack /review to find production-grade issues and verify code quality.", "执行 gstack /review：发现生产级问题并验证代码质量。"), gstackRolePrompt("review"), []string{"review"}),
			roleWithPrompt("gstack-qa-lead", "engineering", "qa", text(locale, "QA Lead", "QA Lead"), text(locale, "Runs gstack /qa to test real user flows, report bugs, and verify fixes.", "执行 gstack /qa：测试真实用户流程、报告问题并验证修复。"), gstackRolePrompt("qa"), []string{"qa"}),
			roleWithPrompt("gstack-release-engineer", "engineering", "release", text(locale, "Release Engineer", "Release Engineer"), text(locale, "Runs gstack /ship to verify readiness, push, open PR, and prepare release evidence.", "执行 gstack /ship：验证发版就绪、推送、创建 PR 并准备发版证据。"), gstackRolePrompt("ship"), []string{"ship"}),
		},
		Skills: []entity.PlaybookSkillTemplate{
			gstackSkill("office-hours", "office-hours", text(locale, "YC Office Hours diagnostic from gstack. Full upstream SKILL.md is installed.", "gstack 的 YC Office Hours 诊断。安装完整上游 SKILL.md。"), "office-hours"),
			gstackSkill("plan-ceo-review", "plan-ceo-review", text(locale, "CEO/founder-mode plan review from gstack. Full upstream SKILL.md is installed.", "gstack 的 CEO/founder-mode 方案评审。安装完整上游 SKILL.md。"), "plan-ceo-review"),
			gstackSkill("plan-eng-review", "plan-eng-review", text(locale, "Engineering plan review from gstack. Full upstream SKILL.md is installed.", "gstack 的工程方案评审。安装完整上游 SKILL.md。"), "plan-eng-review"),
			gstackSkill("plan-design-review", "plan-design-review", text(locale, "Design plan review from gstack. Full upstream SKILL.md is installed.", "gstack 的设计方案评审。安装完整上游 SKILL.md。"), "plan-design-review"),
			gstackSkill("spec", "spec", text(locale, "Spec authoring workflow from gstack. Full upstream SKILL.md is installed.", "gstack 的 spec 编写工作流。安装完整上游 SKILL.md。"), "spec"),
			gstackSkill("review", "review", text(locale, "Staff engineer code review from gstack. Full upstream SKILL.md is installed.", "gstack 的 Staff Engineer 代码评审。安装完整上游 SKILL.md。"), "review"),
			gstackSkill("qa", "qa", text(locale, "QA workflow from gstack. Full upstream SKILL.md is installed.", "gstack 的 QA 工作流。安装完整上游 SKILL.md。"), "qa"),
			gstackSkill("ship", "ship", text(locale, "Release workflow from gstack. Full upstream SKILL.md is installed.", "gstack 的发版工作流。安装完整上游 SKILL.md。"), "ship"),
		},
		Workflows: []entity.PlaybookWorkflowTemplate{
			workflow("startup-idea-validation", wf, map[string]string{
				"office_hours":       "yc-office-hours-partner",
				"ceo_review":         "yc-founder-reviewer",
				"spec_authoring":     "gstack-spec-author",
				"engineering_review": "gstack-eng-reviewer",
				"design_review":      "gstack-design-reviewer",
				"staff_review":       "gstack-staff-reviewer",
				"qa":                 "gstack-qa-lead",
				"ship":               "gstack-release-engineer",
			}, map[string][]string{
				"office_hours":       {"office-hours"},
				"ceo_review":         {"plan-ceo-review"},
				"spec_authoring":     {"spec"},
				"engineering_review": {"plan-eng-review"},
				"design_review":      {"plan-design-review"},
				"staff_review":       {"review"},
				"qa":                 {"qa"},
				"ship":               {"ship"},
			}),
		},
		TaskTemplates: []entity.PlaybookTaskTemplate{
			task("validate-and-ship-with-gstack", text(locale, "Validate and ship with gstack", "用 gstack 验证并交付"), text(locale, "Run a full gstack-style path: office hours, CEO review, spec, engineering/design review, implementation review, QA, and ship.", "运行完整 gstack 风格路径：office hours、CEO review、spec、工程/设计评审、实现评审、QA 和发版。"), "startup-idea-validation"),
			task("validate-startup-idea", text(locale, "Validate a startup idea", "验证一个创业想法"), text(locale, "Pressure-test a raw idea with the full gstack office-hours and CEO review loop before committing build time.", "投入开发前，用完整 gstack office-hours 和 CEO review 闭环对原始想法做压力测试。"), "startup-idea-validation"),
		},
		SetupQuestions: []entity.PlaybookSetupQuestion{
			question("target_user", text(locale, "Who is the narrow first user?", "第一批窄用户是谁？"), nil, true),
			question("current_status_quo", text(locale, "What do they do today without this product?", "没有这个产品时他们现在怎么解决？"), nil, true),
		},
		SuccessMetrics: []entity.PlaybookMetric{
			metric("office_hours_quality", text(locale, "Office-hours quality", "Office-hours 质量"), text(locale, "Whether the problem, status quo, wedge, and demand evidence are specific enough to make a build/no-build decision.", "问题、现状替代方案、切入点和需求证据是否足够具体，能支撑做/不做决策。")),
			metric("review_rigor", text(locale, "Review rigor", "评审严格度"), text(locale, "Whether CEO, engineering, design, review, QA, and ship gates produce concrete findings and decision records.", "CEO、工程、设计、review、QA 和 ship 门禁是否产出具体发现和决策记录。")),
		},
	}
}

func mattPocockEngineering(locale string) entity.PlaybookTemplate {
	wf := mattPocockEngineeringWorkflow(locale)
	return entity.PlaybookTemplate{
		ID:          "matt-pocock-real-engineering",
		Version:     mattPocockTemplateVersion,
		Name:        text(locale, "Matt Pocock Real Engineering", "Matt Pocock 真实工程协作方案"),
		Description: text(locale, "A pragmatic engineering playbook built from Matt Pocock's Skills for Real Engineers: clarify with pressure, produce specs and tickets, implement with tests, review code, debug, hand off, and preserve reusable engineering knowledge.", "基于 Matt Pocock 的 Skills for Real Engineers 构建的工程协作方案：高压澄清、产出 spec 与 tickets、测试驱动实现、代码审核、排障、交接，并沉淀可复用工程知识。"),
		Locale:      normalizeLocale(locale),
		Category:    text(locale, "Engineering", "工程"),
		Complexity:  text(locale, "Advanced", "高阶"),
		Tags:        []string{"engineering", "matt-pocock", "skills", "spec", "tdd", "review"},
		Roles: []entity.PlaybookRoleTemplate{
			roleWithPrompt("matt-wayfinder", "product", "wayfinder", text(locale, "Matt Pocock Wayfinder", "Matt Pocock Wayfinder"), text(locale, "Clarifies ambiguous work, finds the right tracker item, grills weak requirements, and writes handoff context.", "澄清模糊工作，定位正确的任务入口，追问薄弱需求，并编写交接上下文。"), mattPocockRolePrompt("engineering", "wayfinder"), []string{"wayfinder", "ask-matt", "grill-with-docs", "grill-me", "grilling", "handoff"}),
			roleWithPrompt("domain-modeler", "product", "domain-modeler", text(locale, "Domain Modeler", "领域建模 Agent"), text(locale, "Builds shared language, domain objects, and executable specs before implementation starts.", "在开发前建立共享语言、领域对象和可执行规格。"), mattPocockRolePrompt("engineering", "domain-modeling"), []string{"domain-modeling", "codebase-design", "to-spec"}),
			roleWithPrompt("ticket-planner", "engineering", "ticket-planner", text(locale, "Ticket Planner", "任务拆解 Agent"), text(locale, "Turns approved specs into sequenced, dependency-aware tickets that agents can execute.", "把已审核 spec 拆成有顺序和依赖关系、Agent 可执行的 tickets。"), mattPocockRolePrompt("engineering", "to-tickets"), []string{"to-tickets", "triage", "research", "wayfinder"}),
			roleWithPrompt("implementation-agent", "engineering", "developer", text(locale, "Implementation Agent", "实现 Agent"), text(locale, "Implements from specs and tickets, uses TDD at agreed seams, prototypes when needed, and reports evidence.", "基于 spec 和 tickets 实现，在约定边界使用 TDD，必要时先做原型，并报告证据。"), mattPocockRolePrompt("engineering", "implement"), []string{"implement", "tdd", "prototype", "diagnosing-bugs"}),
			roleWithPrompt("architecture-reviewer", "engineering", "architecture-reviewer", text(locale, "Architecture Reviewer", "架构审核 Agent"), text(locale, "Reviews architecture, coupling, boundaries, maintainability, and design drift.", "审核架构、耦合、边界、可维护性和设计偏移。"), mattPocockRolePrompt("engineering", "improve-codebase-architecture"), []string{"improve-codebase-architecture", "codebase-design", "code-review"}),
			roleWithPrompt("review-agent", "engineering", "reviewer", text(locale, "Code Review Agent", "代码审核 Agent"), text(locale, "Reviews implementation quality, asks for changes when needed, and handles merge conflict resolution.", "审核实现质量，必要时要求修改，并处理合并冲突解决。"), mattPocockRolePrompt("engineering", "code-review"), []string{"code-review", "resolving-merge-conflicts"}),
			roleWithPrompt("learning-writer", "enablement", "knowledge-writer", text(locale, "Learning Writer", "知识沉淀 Agent"), text(locale, "Turns finished work into handoffs, teaching notes, and better reusable skills.", "把完成的工作沉淀成交接、教学说明和更好的可复用 skill。"), mattPocockRolePrompt("productivity", "handoff"), []string{"handoff", "teach", "writing-great-skills"}),
		},
		Skills: mattPocockSkills(locale),
		Workflows: []entity.PlaybookWorkflowTemplate{
			workflow("matt-pocock-real-engineering", wf, map[string]string{
				"setup_context":  "matt-wayfinder",
				"clarify":        "matt-wayfinder",
				"spec":           "domain-modeler",
				"tickets":        "ticket-planner",
				"implementation": "implementation-agent",
				"architecture":   "architecture-reviewer",
				"code_review":    "review-agent",
				"debug_and_fix":  "implementation-agent",
				"handoff":        "learning-writer",
			}, map[string][]string{
				"setup_context":  {"setup-matt-pocock-skills", "wayfinder"},
				"clarify":        {"grill-with-docs", "grill-me", "grilling", "ask-matt"},
				"spec":           {"to-spec", "domain-modeling", "codebase-design"},
				"tickets":        {"to-tickets", "triage", "research"},
				"implementation": {"implement", "tdd", "prototype"},
				"architecture":   {"improve-codebase-architecture", "codebase-design"},
				"code_review":    {"code-review", "resolving-merge-conflicts"},
				"debug_and_fix":  {"diagnosing-bugs", "tdd", "implement"},
				"handoff":        {"handoff", "teach", "writing-great-skills"},
			}),
		},
		TaskTemplates: []entity.PlaybookTaskTemplate{
			task("build-from-spec", text(locale, "Build from a spec", "基于 Spec 交付"), text(locale, "Clarify context, write spec, split tickets, implement with tests, review, fix, and hand off learnings.", "澄清上下文，编写 spec，拆 tickets，测试驱动实现，审核、修复并交接沉淀。"), "matt-pocock-real-engineering"),
			task("debug-production-issue", text(locale, "Debug an issue", "排查问题"), text(locale, "Use diagnosis, triage, targeted implementation, review, and handoff for a concrete bug or regression.", "针对具体 bug 或回归，执行诊断、分诊、定向实现、审核和交接。"), "matt-pocock-real-engineering"),
		},
		RequiredTools: []entity.PlaybookToolRequirement{
			tool("github", "GitHub", text(locale, "Issue tracker, PR, repository, and CI access.", "Issue、PR、代码仓库和 CI 访问。"), false),
			tool("gitlab", "GitLab", text(locale, "Repository, merge request, issue, and pipeline access.", "代码仓库、MR、Issue 和流水线访问。"), false),
			tool("feishu", text(locale, "Feishu", "飞书"), text(locale, "Docs and review collaboration if the team uses Feishu.", "如果团队使用飞书，用于文档和评审协作。"), false),
			tool("slack", "Slack", text(locale, "Discussion and review collaboration if the team uses Slack.", "如果团队使用 Slack，用于讨论和评审协作。"), false),
		},
		SetupQuestions: []entity.PlaybookSetupQuestion{
			question("tracker", text(locale, "Which tracker should specs and tickets be published to?", "Spec 和 tickets 应发布到哪个任务系统？"), []string{"GitHub Issues", "Linear", "Jira", "Other"}, true),
			question("test_seams", text(locale, "Which test seams are preferred in this codebase?", "这个代码库优先在哪些边界做测试？"), nil, false),
			question("domain_glossary", text(locale, "Where is the domain glossary or architecture context?", "领域词汇表或架构上下文在哪里？"), nil, false),
		},
		SuccessMetrics: []entity.PlaybookMetric{
			metric("spec_to_ticket_quality", text(locale, "Spec to ticket quality", "Spec 到 Ticket 质量"), text(locale, "Whether specs and tickets are concrete enough for agents to execute without repeated clarification.", "Spec 和 tickets 是否足够具体，Agent 能否无需反复澄清就执行。")),
			metric("review_rework_rate", text(locale, "Review rework rate", "审核返工率"), text(locale, "How often implementation returns from review or QA to debugging and fixing.", "实现从审核或 QA 打回排障修复的频率。")),
			metric("handoff_reuse", text(locale, "Handoff reuse", "交接复用率"), text(locale, "How often finished work creates docs, patterns, or skills that reduce future token and human intervention.", "完成工作后产出的文档、模式或 skill 是否降低后续 token 和人工介入。")),
		},
	}
}

func bugTriageAndFix(locale string) entity.PlaybookTemplate {
	wf, _ := workflowstore.Template("agentic-bug-triage-loop", locale)
	return entity.PlaybookTemplate{
		ID:          "bug-triage-and-fix",
		Version:     playbookTemplateVersion,
		Name:        text(locale, "Bug Triage and Fix", "Bug 分诊与修复协作方案"),
		Description: text(locale, "A focused loop for reproducing, diagnosing, fixing, and verifying bugs with explicit escalation when evidence is weak.", "用于 Bug 复现、定位、修复和验证的聚焦闭环；证据不足时明确升级。"),
		Locale:      normalizeLocale(locale),
		Category:    text(locale, "Engineering", "工程"),
		Complexity:  text(locale, "Basic", "基础"),
		Tags:        []string{"bug", "triage", "qa"},
		Roles: []entity.PlaybookRoleTemplate{
			role("triage-agent", "engineering", "triage", text(locale, "Triage Agent", "分诊 Agent"), text(locale, "Clarifies reproduction evidence, severity, and ownership.", "澄清复现证据、严重程度和归属。"), []string{"root-cause-investigation"}),
			role("fix-agent", "engineering", "developer", text(locale, "Fix Agent", "修复 Agent"), text(locale, "Makes the smallest correct fix and records risk.", "做最小正确修复并记录风险。"), []string{"root-cause-investigation"}),
			role("qa-agent", "engineering", "qa", text(locale, "QA Agent", "QA Agent"), text(locale, "Verifies the fix and checks regression risk.", "验证修复并检查回归风险。"), []string{"regression-check"}),
		},
		Skills: []entity.PlaybookSkillTemplate{
			skill("root-cause-investigation", text(locale, "Root Cause Investigation", "根因定位"), text(locale, "Separate symptoms, reproduction, suspected cause, fix, and verification evidence.", "区分症状、复现、疑似根因、修复和验证证据。")),
			skill("regression-check", text(locale, "Regression Check", "回归检查"), text(locale, "Check adjacent behavior and prevent narrow fixes from breaking existing flows.", "检查相邻行为，避免窄修复破坏已有流程。")),
		},
		Workflows: []entity.PlaybookWorkflowTemplate{workflow("agentic-bug-triage-loop", wf, nil, nil)},
		TaskTemplates: []entity.PlaybookTaskTemplate{
			task("fix-reported-bug", text(locale, "Fix a reported bug", "修复已反馈 Bug"), text(locale, "Start from a report, produce reproduction and verified fix evidence.", "从问题反馈开始，产出复现和已验证修复证据。"), "agentic-bug-triage-loop"),
		},
	}
}

func supportKnowledgeLoop(locale string) entity.PlaybookTemplate {
	wf := supportKnowledgeWorkflow(locale)
	return entity.PlaybookTemplate{
		ID:          "support-knowledge-loop",
		Version:     playbookTemplateVersion,
		Name:        text(locale, "Customer Support Knowledge Loop", "客服知识库循环协作方案"),
		Description: text(locale, "Turn repeated support questions into reviewed answers, reusable knowledge docs, and product feedback.", "把重复客服问题转成已审核答复、可复用知识库文档和产品反馈。"),
		Locale:      normalizeLocale(locale),
		Category:    text(locale, "Operations", "运营支持"),
		Complexity:  text(locale, "Basic", "基础"),
		Tags:        []string{"support", "knowledge-base", "operations"},
		Roles: []entity.PlaybookRoleTemplate{
			role("support-triage-agent", "operations", "support", text(locale, "Support Triage Agent", "客服分诊 Agent"), text(locale, "Clusters repeated questions and drafts customer-safe answers.", "聚类重复问题并起草可对客户发送的答复。"), []string{"support-answer-draft"}),
			role("kb-maintainer", "operations", "knowledge-base", text(locale, "Knowledge Base Maintainer", "知识库维护 Agent"), text(locale, "Turns reviewed answers into reusable knowledge documents.", "把审核后的答复沉淀成可复用知识库文档。"), []string{"multigent-docs"}),
			role("feedback-analyst", "product", "feedback", text(locale, "Product Feedback Analyst", "产品反馈分析 Agent"), text(locale, "Extracts product gaps and recurring pain from support evidence.", "从客服证据中提取产品缺口和重复痛点。"), []string{"feedback-synthesis"}),
		},
		Skills: []entity.PlaybookSkillTemplate{
			skill("support-answer-draft", text(locale, "Support Answer Draft", "客服答复草稿"), text(locale, "Draft concise, accurate, non-promissory support answers.", "起草简洁、准确、不做过度承诺的客服答复。")),
			skill("feedback-synthesis", text(locale, "Feedback Synthesis", "反馈归纳"), text(locale, "Convert repeated support cases into product feedback and evidence.", "把重复客服案例转为产品反馈和证据。")),
			skill("multigent-docs", text(locale, "Multigent Docs", "Multigent 知识库"), text(locale, "Store reusable docs by docID for future agents.", "用 docID 保存可复用文档，供后续 Agent 使用。")),
		},
		Workflows: []entity.PlaybookWorkflowTemplate{workflow("support-knowledge-loop", wf, nil, nil)},
		TaskTemplates: []entity.PlaybookTaskTemplate{
			task("summarize-support-topic", text(locale, "Summarize support topic", "整理客服主题"), text(locale, "Turn a cluster of support messages into reviewed answer and KB update.", "把一组客服消息转成审核答复和知识库更新。"), "support-knowledge-loop"),
		},
		RequiredTools: []entity.PlaybookToolRequirement{
			tool("feishu", text(locale, "Feishu", "飞书"), text(locale, "Support conversations, docs, or internal notifications.", "客服会话、文档或内部通知。"), false),
			tool("slack", "Slack", text(locale, "Support channels and internal review.", "客服频道和内部评审。"), false),
		},
	}
}

const ycAdvisorPromptEN = `# Role: YC Office Hours Advisor

You challenge startup and internal product ideas before build work starts. Your job is not encouragement; your job is diagnosis.

Rules:
- Force specificity. A market category is not a customer. Ask for a named role, concrete workflow, current workaround, and consequence.
- Treat interest as weak evidence. Strong evidence is payment, repeated usage, urgent pull, manual workaround cost, or user panic when the workflow breaks.
- Treat the status quo as the real competitor. Compare against spreadsheets, chat threads, manual operations, and internal tools.
- Push for the narrowest wedge. Prefer one workflow someone needs this week over a platform vision.
- Do not start implementation. Produce a decision document, next assignment, and unresolved risks.

Outputs should be structured:
- problem_statement_doc_id
- demand_evidence_doc_id
- premise_challenge_doc_id
- recommended_assignment
- decision: proceed, revise, research_more, or stop`

const ycAdvisorPromptZH = `# Role: YC Office Hours 顾问

你负责在真正投入开发前挑战创业想法或内部产品想法。你的工作不是鼓励，而是诊断。

规则：
- 强迫具体。市场分类不是客户。要追问具体角色、具体工作流、当前替代方案和真实后果。
- 不把“感兴趣”当需求。强证据是付费、重复使用、紧急拉动、人工替代成本，或者流程坏掉时用户会慌。
- 把现状替代方案当成真正竞品。要比较表格、群消息、人工运营、内部工具，而不是只比较同类产品。
- 压到最窄切入点。优先一个本周就有人需要的工作流，而不是平台愿景。
- 不进入实现。输出决策文档、下一步 assignment 和未解决风险。

输出必须结构化：
- problem_statement_doc_id
- demand_evidence_doc_id
- premise_challenge_doc_id
- recommended_assignment
- decision: proceed, revise, research_more, or stop`

const marketResearchPromptEN = `# Role: Market Research Agent

You collect evidence that can falsify or strengthen a product idea.

Focus on:
- Who urgently needs this, by role and situation.
- What they currently do instead, and what it costs in time, money, risk, or reputation.
- Whether they already pay for adjacent tools or services.
- Whether switching pressure is strong enough to overcome inertia.
- What user wording differs from the founder's pitch.

Do not summarize generic market size unless it changes the decision. Produce evidence, quotes, source links, and confidence levels.`

const marketResearchPromptZH = `# Role: 市场调研 Agent

你负责收集能够证伪或增强产品想法的证据。

关注：
- 谁在什么场景下迫切需要它，具体到角色和处境。
- 他们现在怎么替代解决，成本是什么：时间、金钱、风险或声誉。
- 他们是否已经为相邻工具或服务付费。
- 迁移动力是否足够强，能否克服惯性。
- 用户自己的表述跟创始人的 pitch 有什么差异。

不要泛泛总结市场规模，除非它会改变决策。输出证据、原话、来源链接和置信度。`

const prototypeScopePromptEN = `# Role: Prototype Scope Reviewer

You turn validated evidence into the smallest prototype that tests the riskiest premise.

Rules:
- One prototype tests one risky premise. Do not hide uncertainty behind a big roadmap.
- Prefer 48-hour scope. If it cannot be scoped into 48 hours, split until it can.
- Define what is intentionally not built.
- Define the proof: who tries it, what behavior counts, and what result means stop or continue.

Outputs:
- prototype_scope_doc_id
- riskiest_premise
- success_signal
- non_goals
- next_48h_plan`

const prototypeScopePromptZH = `# Role: 原型范围评审

你负责把已验证的证据转成能验证最大风险假设的最小原型。

规则：
- 一个原型只验证一个最大风险假设。不要用大路线图掩盖不确定性。
- 优先 48 小时范围。如果 48 小时做不完，就继续拆小。
- 明确不做什么。
- 明确验证证据：谁来试，什么行为算有效，什么结果意味着停止或继续。

输出：
- prototype_scope_doc_id
- riskiest_premise
- success_signal
- non_goals
- next_48h_plan`

const marketValueSkillEN = `# Skill: Market Value Evaluation

Use this when judging whether an idea deserves build time.

Process:
1. State the strongest version of the idea in one paragraph.
2. Identify the exact first customer: role, situation, trigger, budget owner, and consequence.
3. Separate weak evidence from strong evidence.
4. Name the demand risk: no urgency, no budget, no switching pressure, unclear buyer, or nice-to-have.
5. Decide what evidence would change your mind.

Red flags:
- "Everyone needs this."
- Waitlists without behavior.
- Market growth used as proof of demand.
- The product requires a full platform before any user gets value.

Output fields:
- market_thesis
- first_customer
- strongest_evidence
- weakest_assumption
- decision`

const marketValueSkillZH = `# Skill: 市场价值判断

用于判断一个想法是否值得投入构建时间。

流程：
1. 用一段话写出这个想法的最强版本。
2. 识别最早客户：角色、场景、触发点、预算负责人和后果。
3. 区分弱证据和强证据。
4. 点名需求风险：不紧急、没预算、迁移动力不足、买方不清晰、只是锦上添花。
5. 说明什么证据会改变你的判断。

红旗：
- “所有人都需要。”
- 只有 waitlist，没有行为。
- 用市场增长代替需求证据。
- 必须做完整平台，用户才有任何价值。

输出字段：
- market_thesis
- first_customer
- strongest_evidence
- weakest_assumption
- decision`

const statusQuoSkillEN = `# Skill: Status Quo Analysis

Use this to identify the real competitor: what users already do today.

Process:
1. Map the current workflow step by step.
2. List the tools, people, documents, scripts, and manual habits involved.
3. Quantify cost where possible: hours, money, risk, delay, rework, or missed revenue.
4. Identify why the user has tolerated the current workaround so far.
5. Decide what must be dramatically better for switching to happen.

Output fields:
- current_workflow
- workaround_cost
- inertia_reason
- switching_trigger
- must_be_10x_better_at`

const statusQuoSkillZH = `# Skill: 现状替代方案分析

用于识别真正竞品：用户今天已经怎么解决。

流程：
1. 逐步画出当前工作流。
2. 列出涉及的工具、人、文档、脚本和人工习惯。
3. 尽可能量化成本：时间、金钱、风险、延迟、返工或收入损失。
4. 识别用户为什么到现在还忍受这个替代方案。
5. 判断新方案必须在哪一点上显著更好，用户才会迁移。

输出字段：
- current_workflow
- workaround_cost
- inertia_reason
- switching_trigger
- must_be_10x_better_at`

const desperateSignalSkillEN = `# Skill: Desperate User Signal

Use this to find whether a narrow group needs the product now.

Look for:
- A named person or role with a repeated painful situation.
- A deadline, compliance risk, revenue risk, or career consequence.
- Manual labor already being spent.
- Payment for adjacent tools or services.
- Pull from users: follow-ups, repeated requests, usage expansion, anger when unavailable.

Output fields:
- desperate_user
- painful_moment
- consequence
- observed_behavior
- confidence`

const desperateSignalSkillZH = `# Skill: 强痛点用户信号

用于判断是否有一个窄人群现在就迫切需要产品。

寻找：
- 有具体名字或具体角色的人，反复遇到痛苦场景。
- 截止时间、合规风险、收入风险或职业后果。
- 已经投入人工劳动来解决。
- 已经为相邻工具或服务付费。
- 用户主动拉动：追问、重复请求、扩大使用、不可用时生气。

输出字段：
- desperate_user
- painful_moment
- consequence
- observed_behavior
- confidence`

const prototypeScopeSkillEN = `# Skill: 48-hour Prototype Scope

Use this after demand evidence exists.

Process:
1. Pick the riskiest premise.
2. Define a 48-hour prototype that tests only that premise.
3. Remove login, settings, integrations, dashboards, and automation unless they are essential to the premise.
4. Define the user test and observable pass/fail signal.
5. Write non-goals so scope cannot expand silently.

Output fields:
- riskiest_premise
- prototype
- non_goals
- test_plan
- pass_signal
- fail_signal`

const prototypeScopeSkillZH = `# Skill: 48 小时原型范围

用于已经存在需求证据之后。

流程：
1. 选择最大风险假设。
2. 定义一个只验证该假设的 48 小时原型。
3. 除非对假设验证必不可少，否则移除登录、设置、集成、仪表盘和自动化。
4. 定义用户测试方式和可观察的通过/失败信号。
5. 写出非目标，防止范围静默膨胀。

输出字段：
- riskiest_premise
- prototype
- non_goals
- test_plan
- pass_signal
- fail_signal`

type mattPocockSkillSpec struct {
	Category      string
	ID            string
	Name          string
	DescriptionEN string
	DescriptionZH string
}

var mattPocockSkillCatalog = []mattPocockSkillSpec{
	{"engineering", "ask-matt", "ask-matt", "Ask a Matt-style reviewer for engineering judgment, trade-offs, and next action.", "用 Matt 风格的工程判断追问取舍和下一步。"},
	{"engineering", "grill-with-docs", "grill-with-docs", "Pressure-test ideas against existing docs, domain context, and prior decisions.", "基于已有文档、领域上下文和历史决策对想法做压力测试。"},
	{"engineering", "triage", "triage", "Classify and route unclear work before implementation starts.", "在开发前对不清晰的工作做分类和分流。"},
	{"engineering", "improve-codebase-architecture", "improve-codebase-architecture", "Find architectural improvement opportunities without broad rewriting.", "识别架构改进机会，避免无边界重写。"},
	{"engineering", "setup-matt-pocock-skills", "setup-matt-pocock-skills", "Set up tracker labels, domain glossary, and team conventions required by the skills.", "建立这些 skill 所需的任务标签、领域词汇和团队约定。"},
	{"engineering", "to-spec", "to-spec", "Synthesize current context into a publishable spec without re-interviewing the user.", "把当前上下文综合成可发布 spec，不重新访谈用户。"},
	{"engineering", "to-tickets", "to-tickets", "Split a spec into ordered, dependency-aware implementation tickets.", "把 spec 拆成有顺序和依赖关系的实现 tickets。"},
	{"engineering", "implement", "implement", "Implement work from a spec or tickets with tests and review.", "基于 spec 或 tickets 实现，并配套测试和审核。"},
	{"engineering", "wayfinder", "wayfinder", "Find the right issue, document, route, or next action in a messy codebase workflow.", "在复杂代码库工作流中找到正确 issue、文档、路径或下一步。"},
	{"engineering", "prototype", "prototype", "Build a narrow prototype to encode a decision or test an uncertain design.", "构建窄范围原型来表达决策或验证不确定设计。"},
	{"engineering", "diagnosing-bugs", "diagnosing-bugs", "Diagnose bugs with evidence, reproduction, hypotheses, and targeted fixes.", "用证据、复现、假设和定向修复来排查 bug。"},
	{"engineering", "research", "research", "Research codebase or technical unknowns and produce decision-ready notes.", "调研代码库或技术未知点，并产出可决策说明。"},
	{"engineering", "tdd", "tdd", "Use test-driven development at agreed seams and verify external behavior.", "在约定边界使用 TDD，验证外部行为。"},
	{"engineering", "domain-modeling", "domain-modeling", "Model the domain language and concepts before designing implementation.", "在设计实现前建模领域语言和概念。"},
	{"engineering", "codebase-design", "codebase-design", "Design codebase changes around seams, ownership, and maintainable boundaries.", "围绕测试边界、职责归属和可维护边界设计代码变更。"},
	{"engineering", "code-review", "code-review", "Review code quality, behavior, tests, maintainability, and risk.", "审核代码质量、行为、测试、可维护性和风险。"},
	{"engineering", "resolving-merge-conflicts", "resolving-merge-conflicts", "Resolve merge conflicts while preserving intent and behavior.", "在保留意图和行为的前提下解决合并冲突。"},
	{"productivity", "grill-me", "grill-me", "Use direct questioning to expose weak assumptions and missing context.", "用直接追问暴露薄弱假设和缺失上下文。"},
	{"productivity", "handoff", "handoff", "Create useful handoff notes so another agent or human can continue.", "创建有效交接说明，让其他 Agent 或人能继续。"},
	{"productivity", "teach", "teach", "Turn understanding into a teaching artifact that helps future work.", "把理解转成能帮助后续工作的教学材料。"},
	{"productivity", "writing-great-skills", "writing-great-skills", "Improve skills so repeated work becomes clearer and more reusable.", "改进 skill，让重复工作更清晰、更可复用。"},
	{"productivity", "grilling", "grilling", "Run a structured grilling loop to make requirements, reasoning, and evidence sharper.", "运行结构化追问循环，让需求、推理和证据更锋利。"},
}

func mattPocockSkills(locale string) []entity.PlaybookSkillTemplate {
	out := make([]entity.PlaybookSkillTemplate, 0, len(mattPocockSkillCatalog))
	for _, spec := range mattPocockSkillCatalog {
		out = append(out, mattPocockSkill(spec, text(locale, spec.DescriptionEN, spec.DescriptionZH)))
	}
	return out
}

func mattPocockSkill(spec mattPocockSkillSpec, description string) entity.PlaybookSkillTemplate {
	return entity.PlaybookSkillTemplate{
		ID:          spec.ID,
		Name:        spec.Name,
		Description: description,
		Body:        mattPocockSkillBody(spec.Category, spec.ID),
		Source:      "Vendored from https://github.com/mattpocock/skills:" + spec.Category + "/" + spec.ID,
		LicenseNote: "MIT License. Copyright (c) 2026 Matt Pocock. See internal/playbook/mattpocock_assets/LICENSE.",
	}
}

func mattPocockRolePrompt(category, assetName string) string {
	return "# Role prompt source\n\nThis role is backed by the full upstream Matt Pocock skill below. Follow it as the primary operating procedure inside Multigent.\n\n" + mattPocockSkillBody(category, assetName)
}

func role(id, team, roleName, name, description string, skills []string) entity.PlaybookRoleTemplate {
	return entity.PlaybookRoleTemplate{ID: id, Team: team, Role: roleName, Name: name, Description: description, Skills: skills}
}

func roleWithPrompt(id, team, roleName, name, description, prompt string, skills []string) entity.PlaybookRoleTemplate {
	return entity.PlaybookRoleTemplate{ID: id, Team: team, Role: roleName, Name: name, Description: description, Prompt: prompt, Skills: skills}
}

func skill(id, name, description string) entity.PlaybookSkillTemplate {
	return entity.PlaybookSkillTemplate{ID: id, Name: name, Description: description}
}

func skillWithBody(id, name, description, body string) entity.PlaybookSkillTemplate {
	return entity.PlaybookSkillTemplate{ID: id, Name: name, Description: description, Body: body, Source: "Inspired by gstack office-hours, plan review, and startup diagnostic patterns."}
}

func gstackSkill(id, name, description, assetName string) entity.PlaybookSkillTemplate {
	return entity.PlaybookSkillTemplate{
		ID:          id,
		Name:        name,
		Description: description,
		Body:        gstackSkillBody(assetName),
		Source:      "Vendored from https://github.com/garrytan/gstack",
		LicenseNote: "MIT License. Copyright (c) 2026 Garry Tan. See internal/playbook/gstack_assets/LICENSE.",
	}
}

func gstackRolePrompt(assetName string) string {
	return "# Role prompt source\n\nThis role is backed by the full upstream gstack skill below. Follow it as the primary operating procedure inside Multigent.\n\n" + gstackSkillBody(assetName)
}

func task(id, title, description, workflowID string) entity.PlaybookTaskTemplate {
	return entity.PlaybookTaskTemplate{
		ID:          id,
		Title:       title,
		Description: description,
		Prompt:      description,
		WorkflowID:  workflowID,
	}
}

func tool(provider, name, description string, required bool) entity.PlaybookToolRequirement {
	return entity.PlaybookToolRequirement{Provider: provider, Name: name, Description: description, Required: required}
}

func question(id, question string, options []string, required bool) entity.PlaybookSetupQuestion {
	return entity.PlaybookSetupQuestion{ID: id, Question: question, Options: options, Required: required}
}

func metric(id, name, description string) entity.PlaybookMetric {
	return entity.PlaybookMetric{ID: id, Name: name, Description: description}
}

func workflow(id string, definition entity.WorkflowTemplate, roleBindings map[string]string, skillBindings map[string][]string) entity.PlaybookWorkflowTemplate {
	return entity.PlaybookWorkflowTemplate{
		ID:            id,
		Name:          definition.Name,
		Description:   definition.Description,
		Definition:    definition,
		RoleBindings:  roleBindings,
		SkillBindings: skillBindings,
	}
}
