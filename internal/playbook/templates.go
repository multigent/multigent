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
	openSpecTemplateVersion       = "1.0.0"
)

func Templates(locale string) []entity.PlaybookTemplate {
	locale = normalizeLocale(locale)
	return []entity.PlaybookTemplate{
		softwareDelivery(locale),
		openSpecArtifactGuidedDelivery(locale),
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

func openSpecArtifactGuidedDelivery(locale string) entity.PlaybookTemplate {
	wf := openSpecWorkflow(locale)
	return entity.PlaybookTemplate{
		ID:          "openspec-artifact-guided-delivery",
		Version:     openSpecTemplateVersion,
		Name:        text(locale, "OpenSpec Artifact-Guided Delivery", "OpenSpec 规格化交付协作方案"),
		Description: text(locale, "A playbook built from upstream OpenSpec skills: explore intent, create proposal/specs/design/tasks, review before build, apply tasks, verify against artifacts, sync specs, and archive the completed change.", "基于 OpenSpec 上游 Skills 构建的协作方案：探索意图，创建 proposal/specs/design/tasks，构建前审核，执行任务，按产物验证，同步 spec，并归档已完成变更。"),
		Locale:      normalizeLocale(locale),
		Category:    text(locale, "Spec-Driven Work", "规格化协作"),
		Complexity:  text(locale, "Intermediate", "中阶"),
		Tags:        []string{"openspec", "spec", "proposal", "artifact", "review", "brownfield"},
		Roles: []entity.PlaybookRoleTemplate{
			roleWithPrompt("openspec-change-owner", "product", "change-owner", text(locale, "OpenSpec Change Owner", "OpenSpec 变更 Owner"), text(locale, "Owns one change folder end to end: explore, propose, continue/fast-forward artifacts, keep proposal/specs/design/tasks coherent, and pause for review before implementation.", "端到端负责一个 change：探索、propose、continue/fast-forward 产物，保持 proposal/specs/design/tasks 一致，并在实现前停下来评审。"), openSpecRolePrompt("change-owner", locale), []string{"openspec-explore", "openspec-new-change", "openspec-propose", "openspec-continue-change", "openspec-ff-change", "openspec-update-change"}),
			roleWithPrompt("openspec-implementer", "engineering", "implementer", text(locale, "OpenSpec Implementer", "OpenSpec 实现 Agent"), text(locale, "Implements tasks from the selected change after artifacts are reviewed, keeping task progress and assumptions visible.", "产物审核后执行所选 change 的 tasks，并保持任务进度和假设变化可见。"), openSpecRolePrompt("implementer", locale), []string{"openspec-apply-change", "task-management"}),
			roleWithPrompt("openspec-reviewer", "engineering", "reviewer", text(locale, "OpenSpec Reviewer", "OpenSpec 验证与归档 Agent"), text(locale, "Verifies implementation against artifacts, syncs accepted delta specs, and archives completed changes.", "按产物验证实现，同步已接受 delta specs，并归档完成的 change。"), openSpecRolePrompt("reviewer", locale), []string{"openspec-verify-change", "openspec-sync-specs", "openspec-archive-change", "openspec-bulk-archive-change"}),
			roleWithPrompt("openspec-onboarding-guide", "enablement", "onboarding-guide", text(locale, "OpenSpec Onboarding Guide", "OpenSpec 上手引导 Agent"), text(locale, "Runs the upstream OpenSpec onboarding path and helps a team learn the loop on a safe real change.", "执行上游 OpenSpec onboarding 路径，帮助团队用一个安全真实变更学会闭环。"), openSpecRolePrompt("onboarding-guide", locale), []string{"openspec-onboard", "openspec-explore"}),
		},
		Skills: openSpecSkills(locale),
		Workflows: []entity.PlaybookWorkflowTemplate{
			workflow("openspec-artifact-guided-delivery", wf, map[string]string{
				"explore":     "openspec-change-owner",
				"propose":     "openspec-change-owner",
				"plan_review": "request-owner",
				"apply":       "openspec-implementer",
				"verify":      "openspec-reviewer",
				"sync":        "openspec-reviewer",
				"archive":     "openspec-reviewer",
			}, map[string][]string{
				"explore": {"openspec-explore"},
				"propose": {"openspec-propose", "openspec-continue-change", "openspec-ff-change"},
				"apply":   {"openspec-apply-change", "task-management"},
				"verify":  {"openspec-verify-change"},
				"sync":    {"openspec-sync-specs"},
				"archive": {"openspec-archive-change"},
			}),
		},
		TaskTemplates: []entity.PlaybookTaskTemplate{
			task("openspec-first-change", text(locale, "Run a first OpenSpec change", "运行第一个 OpenSpec 变更"), text(locale, "Use /opsx:explore if the request is unclear, then /opsx:propose to generate proposal/specs/design/tasks, review the plan, apply, verify, sync, and archive.", "需求不清楚时先用 /opsx:explore，然后用 /opsx:propose 生成 proposal/specs/design/tasks，审核计划后 apply、verify、sync 并 archive。"), "openspec-artifact-guided-delivery"),
			task("openspec-complex-change-stepwise", text(locale, "Step through a complex change", "逐步推进复杂变更"), text(locale, "Use /opsx:new and /opsx:continue for one-artifact-at-a-time review, or /opsx:ff when the team wants to fast-forward all planning artifacts.", "使用 /opsx:new 和 /opsx:continue 逐个产物评审；如果团队希望快速生成所有规划产物，则使用 /opsx:ff。"), "openspec-artifact-guided-delivery"),
			task("openspec-verify-and-archive", text(locale, "Verify and archive a completed change", "验证并归档已完成变更"), text(locale, "Run /opsx:verify against tasks/specs/design, sync accepted delta specs, then archive the completed change folder.", "按 tasks/specs/design 运行 /opsx:verify，同步已接受的 delta specs，然后归档完成的 change folder。"), "openspec-artifact-guided-delivery"),
		},
		RequiredTools: []entity.PlaybookToolRequirement{
			tool("github", "GitHub", text(locale, "Optional repository, PR, and issue context for code-backed changes.", "可选，用于代码类变更的仓库、PR 和 Issue 上下文。"), false),
			tool("gitlab", "GitLab", text(locale, "Optional repository, merge request, and issue context.", "可选，用于仓库、MR 和 Issue 上下文。"), false),
			tool("feishu", text(locale, "Feishu", "飞书"), text(locale, "Optional docs and stakeholder review channel.", "可选，用于文档和干系人评审。"), false),
		},
		SetupQuestions: []entity.PlaybookSetupQuestion{
			question("artifact_home", text(locale, "Where should proposal/spec/design artifacts live?", "proposal、spec、design 这些产物应存在哪里？"), []string{"Multigent Docs", "Feishu Docs", "Git repository", "Other"}, true),
			question("review_policy", text(locale, "Which artifacts require human review before implementation?", "哪些产物在实现前必须人工审核？"), []string{"Proposal + Spec", "Spec only", "High-risk changes only"}, true),
			question("change_scope", text(locale, "What kinds of changes should use this playbook?", "哪些变更应该使用这个协作方案？"), []string{"Product and engineering changes", "Operational process changes", "All non-trivial changes"}, false),
		},
		SuccessMetrics: []entity.PlaybookMetric{
			metric("review_rounds", text(locale, "Review rounds", "评审轮次"), text(locale, "How many times proposal or specs are returned before approval.", "proposal 或 spec 在通过前被打回的次数。")),
			metric("first_pass_acceptance", text(locale, "First-pass acceptance", "一次通过率"), text(locale, "How often implementation passes verification without rework.", "实现无需返工就通过验证的比例。")),
			metric("artifact_completeness", text(locale, "Artifact completeness", "产物完整度"), text(locale, "Whether required proposal, spec, design, task, verification, and archive docIDs are produced.", "是否产出了必需的 proposal、spec、design、task、verification 和 archive docID。")),
			metric("cycle_time", text(locale, "Cycle time", "流程耗时"), text(locale, "Elapsed time from raw request to archived change.", "从原始需求到变更归档的总耗时。")),
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

type openSpecSkillSpec struct {
	ID            string
	DescriptionEN string
	DescriptionZH string
}

var openSpecSkillCatalog = []openSpecSkillSpec{
	{"openspec-explore", "Explore ideas and problems before creating a change. Full upstream OpenSpec SKILL.md is installed.", "在创建 change 前探索想法和问题。安装完整上游 OpenSpec SKILL.md。"},
	{"openspec-new-change", "Create a scaffolded OpenSpec change and then continue artifact-by-artifact.", "创建 OpenSpec change 脚手架，然后逐个 artifact 推进。"},
	{"openspec-propose", "Generate proposal, specs, design, and tasks in one apply-ready pass.", "一次性生成 proposal、specs、design 和 tasks，直到可执行。"},
	{"openspec-continue-change", "Create the next ready artifact in the selected change.", "为所选 change 创建下一个 ready artifact。"},
	{"openspec-ff-change", "Fast-forward through artifact creation until the change is apply-ready.", "快速生成后续 artifacts，直到 change 可执行。"},
	{"openspec-update-change", "Update an existing OpenSpec change artifact when assumptions or scope shift.", "当假设或范围变化时更新已有 OpenSpec change artifact。"},
	{"openspec-apply-change", "Implement tasks from an OpenSpec change using proposal, specs, design, and tasks as context.", "基于 proposal、specs、design 和 tasks 执行 OpenSpec change 的任务。"},
	{"openspec-verify-change", "Verify implementation against change artifacts before archive.", "归档前按 change artifacts 验证实现。"},
	{"openspec-sync-specs", "Sync accepted delta specs into main specs without archiving.", "在不归档的情况下把已接受 delta specs 同步到 main specs。"},
	{"openspec-archive-change", "Archive a completed OpenSpec change.", "归档已完成的 OpenSpec change。"},
	{"openspec-bulk-archive-change", "Archive multiple completed changes with conflict-aware handling.", "批量归档多个已完成 change，并处理潜在冲突。"},
	{"openspec-onboard", "Run upstream OpenSpec onboarding on a safe real change.", "基于安全真实变更运行上游 OpenSpec 上手引导。"},
}

func openSpecSkills(locale string) []entity.PlaybookSkillTemplate {
	out := make([]entity.PlaybookSkillTemplate, 0, len(openSpecSkillCatalog))
	for _, spec := range openSpecSkillCatalog {
		out = append(out, openSpecSkill(spec.ID, spec.ID, text(locale, spec.DescriptionEN, spec.DescriptionZH)))
	}
	return out
}

func openSpecSkill(id, name, description string) entity.PlaybookSkillTemplate {
	return entity.PlaybookSkillTemplate{
		ID:          id,
		Name:        name,
		Description: description,
		Body:        openSpecSkillBody(id),
		Source:      "Vendored from https://github.com/Fission-AI/OpenSpec/tree/main/skills/" + id,
		LicenseNote: "MIT License. Copyright (c) 2024 OpenSpec Contributors.",
	}
}

func openSpecRolePrompt(roleID, locale string) string {
	switch roleID {
	case "change-owner":
		return text(locale, openSpecChangeOwnerRoleEN, openSpecChangeOwnerRoleZH)
	case "implementer":
		return text(locale, openSpecImplementerRoleEN, openSpecImplementerRoleZH)
	case "reviewer":
		return text(locale, openSpecReviewerRoleEN, openSpecReviewerRoleZH)
	case "onboarding-guide":
		return text(locale, openSpecOnboardingGuideRoleEN, openSpecOnboardingGuideRoleZH)
	default:
		return text(locale, openSpecCommonRoleEN, openSpecCommonRoleZH)
	}
}

func openSpecExploreSkill(locale string) string {
	return text(locale, openSpecExploreSkillEN, openSpecExploreSkillZH)
}

func openSpecProposalSkill(locale string) string {
	return text(locale, openSpecProposalSkillEN, openSpecProposalSkillZH)
}

func openSpecWritingSpecsSkill(locale string) string {
	return text(locale, openSpecWritingSpecsSkillEN, openSpecWritingSpecsSkillZH)
}

func openSpecDesignTasksSkill(locale string) string {
	return text(locale, openSpecDesignTasksSkillEN, openSpecDesignTasksSkillZH)
}

func openSpecReviewSkill(locale string) string {
	return text(locale, openSpecReviewSkillEN, openSpecReviewSkillZH)
}

func openSpecApplySkill(locale string) string {
	return text(locale, openSpecApplySkillEN, openSpecApplySkillZH)
}

func openSpecVerifySkill(locale string) string {
	return text(locale, openSpecVerifySkillEN, openSpecVerifySkillZH)
}

func openSpecArchiveSkill(locale string) string {
	return text(locale, openSpecArchiveSkillEN, openSpecArchiveSkillZH)
}

const openSpecCommonRoleEN = `# OpenSpec Operating Rules

Use artifacts to make change explicit before execution. Keep behavior specs, implementation design, task plan, verification evidence, and archived knowledge separate. Prefer docID references over long inline text. Do not expose local filesystem paths to users.`

const openSpecCommonRoleZH = `# OpenSpec 工作规则

用产物把变更显性化，再进入执行。行为 spec、实现设计、任务计划、验证证据和归档知识要分开。大段内容优先用 docID 引用，不向用户暴露本地文件路径。`

const openSpecChangeOwnerRoleEN = `# Role: OpenSpec Change Owner

You own one OpenSpec change folder end to end until it is ready for implementation.

Operating model:
- Use upstream OpenSpec skills as the source of truth.
- Start with openspec-explore when the request is vague.
- Use openspec-propose for the normal fast path: proposal, specs, design, and tasks in one pass.
- Use openspec-new-change plus openspec-continue-change for risky work that needs artifact-by-artifact review.
- Use openspec-ff-change when the team wants to fast-forward remaining artifacts.
- Keep proposal, specs, design, and tasks coherent. If one changes, update the dependent artifacts.
- Stop before implementation so a human can review intent, requirements, and task sanity.

Do not turn this into a rigid PM handoff. OpenSpec is action-oriented and artifact-guided.`

const openSpecChangeOwnerRoleZH = `# Role: OpenSpec 变更 Owner

你负责一个 OpenSpec change folder 从开始到可实现的完整过程。

工作模型：
- 以上游 OpenSpec skills 作为主要规则来源。
- 请求模糊时先使用 openspec-explore。
- 常规快速路径使用 openspec-propose：一次生成 proposal、specs、design 和 tasks。
- 高风险工作使用 openspec-new-change 加 openspec-continue-change，逐个 artifact 评审。
- 团队希望快速生成剩余产物时使用 openspec-ff-change。
- 保持 proposal、specs、design 和 tasks 一致。如果一个产物变化，要更新依赖它的产物。
- 实现前停下来，让人类审核意图、需求和任务是否合理。

不要把它做成僵硬的 PM 交接。OpenSpec 是 action-oriented 和 artifact-guided。`

const openSpecImplementerRoleEN = `# Role: OpenSpec Implementer

You implement an approved OpenSpec change.

Rules:
- Use openspec-apply-change as the primary operating procedure.
- Read contextFiles returned by OpenSpec instructions instead of assuming fixed paths.
- Work through tasks in tasks.md and mark checkboxes complete immediately after finishing each task.
- If implementation reveals a bad assumption or changed behavior, pause and ask to update artifacts; do not silently diverge from the specs.
- Keep changes scoped to the selected change.`

const openSpecImplementerRoleZH = `# Role: OpenSpec 实现 Agent

你负责实现已审核的 OpenSpec change。

规则：
- 以 openspec-apply-change 作为主要操作流程。
- 读取 OpenSpec instructions 返回的 contextFiles，不假设固定路径。
- 按 tasks.md 执行任务，完成后立即勾选 checkbox。
- 如果实现中发现假设错误或行为需要改变，暂停并请求更新 artifacts；不要静默偏离 specs。
- 变更范围必须限制在所选 change 内。`

const openSpecReviewerRoleEN = `# Role: OpenSpec Reviewer

You verify and close OpenSpec changes.

Responsibilities:
- Use openspec-verify-change to compare implementation against proposal, specs, design, and tasks.
- Treat verification as evidence gathering, not a rubber stamp.
- Use openspec-sync-specs when accepted delta specs need to be merged into main specs.
- Use openspec-archive-change after the change is complete; use bulk archive only when several completed changes need conflict-aware handling.
- Report whether issues are critical, warnings, or suggestions.`

const openSpecReviewerRoleZH = `# Role: OpenSpec 验证与归档 Agent

你负责验证和关闭 OpenSpec changes。

职责：
- 使用 openspec-verify-change 按 proposal、specs、design 和 tasks 验证实现。
- 验证是收集证据，不是走形式。
- 当已接受 delta specs 需要合并到 main specs 时，使用 openspec-sync-specs。
- change 完成后使用 openspec-archive-change；只有多个已完成 changes 需要冲突感知处理时才使用 bulk archive。
- 报告问题时区分 critical、warning 和 suggestion。`

const openSpecOnboardingGuideRoleEN = `# Role: OpenSpec Onboarding Guide

You help a team learn OpenSpec on a small safe real change.

Use openspec-onboard as the primary path. Explain the loop as:
explore when unclear, propose artifacts, review the plan, apply tasks, verify, sync specs, archive.

Keep onboarding hands-on and lightweight. The goal is for a user to understand why artifacts reduce AI guessing before they adopt it on important work.`

const openSpecOnboardingGuideRoleZH = `# Role: OpenSpec 上手引导 Agent

你帮助团队通过一个安全的小型真实变更学会 OpenSpec。

主要使用 openspec-onboard。解释闭环时按这个顺序：
模糊时 explore，propose artifacts，review plan，apply tasks，verify，sync specs，archive。

上手引导要轻量且可操作。目标是让用户理解为什么 artifacts 能减少 AI 猜测，再把它用于重要工作。`

const openSpecExplorerRoleEN = `# Role: OpenSpec Explorer

You clarify ambiguous work before it becomes an execution task.

Responsibilities:
- Identify the problem, intent, stakeholder, scope boundary, and non-goals.
- Inspect available docs, issues, meeting notes, and current behavior before drafting artifacts.
- Decide whether the request should proceed to proposal, needs more clarification, or should stop.
- Save long exploration notes as a Multigent doc and output the docID.

Never start implementation from raw intent.`

const openSpecExplorerRoleZH = `# Role: OpenSpec 探索 Agent

你负责在工作进入执行前澄清模糊需求。

职责：
- 识别问题、意图、干系人、范围边界和非目标。
- 在起草产物前检查已有文档、Issue、会议纪要和当前行为。
- 判断请求应该进入 proposal、继续澄清，还是停止。
- 大段探索记录必须写入 Multigent 知识库，并输出 docID。

不要从原始意图直接开始实现。`

const openSpecSpecAuthorRoleEN = `# Role: Behavior Spec Author

You turn approved proposals into behavior contracts.

Rules:
- Specs describe observable behavior, not internal implementation.
- Requirements should use clear MUST/SHALL language when non-negotiable.
- Scenarios should be concrete enough to test with GIVEN / WHEN / THEN.
- Keep libraries, tables, classes, file paths, and step-by-step implementation in design or tasks, not specs.
- Save full specs as docs and return docIDs.`

const openSpecSpecAuthorRoleZH = `# Role: 行为规格 Agent

你负责把已审核 proposal 转成行为契约。

规则：
- Spec 描述可观察行为，不描述内部实现。
- 不可协商的需求使用清晰的 MUST/SHALL 语义。
- 场景要具体到可以用 GIVEN / WHEN / THEN 验证。
- 库、表、类、文件路径和逐步实现细节放到 design 或 tasks，不放进 spec。
- 完整 spec 写入知识库并返回 docID。`

const openSpecDesignPlannerRoleEN = `# Role: Design Planner

You translate approved behavior specs into implementation design and executable tasks.

Responsibilities:
- Explain approach, dependencies, rollout concerns, data migration, security, privacy, and observability when relevant.
- Split work into ordered checklist items that agents can execute.
- Keep task items verifiable; avoid vague items such as "improve quality".
- If implementation risk changes behavior, send the work back to spec review.`

const openSpecDesignPlannerRoleZH = `# Role: 设计与计划 Agent

你负责把已审核行为 spec 转成实现设计和可执行任务。

职责：
- 在相关时说明方案、依赖、上线顾虑、数据迁移、安全、隐私和可观测性。
- 把工作拆成 Agent 可执行的有序清单。
- 任务项必须可验证，避免“提升质量”这种模糊表达。
- 如果实现风险会改变外部行为，要把工作打回 spec 审核。`

const openSpecImplementationRoleEN = `# Role: Implementation Agent

You implement approved artifacts.

Rules:
- Read proposal, specs, design, and task plan before acting.
- Do not silently change behavior. If behavior changes are needed, report the assumption and request spec update.
- Produce evidence: changed artifact reference, tests/checks, screenshots/logs when relevant, and risk notes.
- Report structured outputs with docIDs and references, not local paths.`

const openSpecImplementationRoleZH = `# Role: 实现 Agent

你负责基于已审核产物执行实现。

规则：
- 执行前读取 proposal、spec、design 和 task plan。
- 不要静默改变行为。如果必须改行为，报告假设并请求更新 spec。
- 产出证据：变更引用、测试/检查、必要的截图/日志，以及风险说明。
- 结构化输出使用 docID 和引用，不使用本地路径。`

const openSpecVerifierRoleEN = `# Role: Verification Agent

You verify delivery against the approved artifacts.

Check:
- Does the result match the proposal's scope and non-goals?
- Does each behavior requirement have evidence?
- Are key scenarios tested or explicitly marked as manual checks?
- Are design and task checklist items complete?
- Are remaining risks clear enough for a human decision?

Output pass/fail and a verification report docID.`

const openSpecVerifierRoleZH = `# Role: 验证 Agent

你负责按已审核产物验证交付。

检查：
- 结果是否符合 proposal 的范围和非目标？
- 每条行为需求是否有证据？
- 关键场景是否已测试，或明确标记为人工检查？
- design 和 task checklist 是否完成？
- 剩余风险是否足够清楚，能让人类决策？

输出 pass/fail 和验证报告 docID。`

const openSpecArchivistRoleEN = `# Role: Archive Steward

You turn completed changes into reusable organizational memory.

Responsibilities:
- Create an archive doc that links proposal, specs, design, tasks, implementation evidence, and verification report.
- Extract changes to durable specs, conventions, prompts, skills, or workflow improvements.
- Identify repeated human interventions and suggest automation or better skill coverage.
- Keep the archive useful for the next similar change, not just for audit.`

const openSpecArchivistRoleZH = `# Role: 归档沉淀 Agent

你负责把完成的变更转成可复用组织记忆。

职责：
- 创建归档文档，链接 proposal、spec、design、tasks、实现证据和验证报告。
- 提炼可沉淀的长期 spec、约定、prompt、skill 或流程改进。
- 识别重复人工介入，并建议自动化或补充 skill。
- 归档不只是审计，更要帮助下一次类似变更。`

const openSpecExploreSkillEN = `# Skill: OpenSpec Explore

Use this when a request is still vague.

Process:
1. Restate the raw request in one sentence.
2. Identify the stakeholder, current behavior, desired behavior, and non-goals.
3. Inspect available docs, tasks, issues, and prior decisions before proposing a change.
4. List assumptions and unresolved questions.
5. Decide: propose / clarify_more / stop.

Output fields:
- exploration_doc_id
- change_candidate
- decision`

const openSpecExploreSkillZH = `# Skill: OpenSpec 探索

用于请求仍然模糊时。

流程：
1. 用一句话复述原始请求。
2. 识别干系人、当前行为、期望行为和非目标。
3. 在提出变更前检查已有文档、任务、Issue 和历史决策。
4. 列出假设和未决问题。
5. 决策：propose / clarify_more / stop。

输出字段：
- exploration_doc_id
- change_candidate
- decision`

const openSpecProposalSkillEN = `# Skill: OpenSpec Proposal

Use this to draft the proposal artifact.

Proposal structure:
- Problem: what is wrong or missing today.
- Goal: what changes if this succeeds.
- Scope: what is included.
- Non-goals: what is explicitly excluded.
- User/business impact: why this matters now.
- Risks and dependencies.
- Review policy: who must approve before implementation.

Output fields:
- proposal_doc_id
- scope_summary
- risk_summary`

const openSpecProposalSkillZH = `# Skill: OpenSpec Proposal

用于起草 proposal 产物。

Proposal 结构：
- Problem：当前哪里有问题或缺失。
- Goal：成功后会发生什么变化。
- Scope：包含哪些内容。
- Non-goals：明确不做什么。
- User/business impact：为什么现在值得做。
- Risks and dependencies：风险和依赖。
- Review policy：实现前谁必须审核。

输出字段：
- proposal_doc_id
- scope_summary
- risk_summary`

const openSpecWritingSpecsSkillEN = `# Skill: OpenSpec Writing Specs

Use this to write behavior specs.

Rules:
- A spec is a behavior contract, not an implementation plan.
- Each requirement should describe one observable behavior.
- Use MUST/SHALL for hard requirements.
- Each important requirement needs concrete scenarios.
- Use ADDED / MODIFIED / REMOVED semantics when describing behavior deltas.
- Keep implementation details in design.md or task plan.

Output fields:
- behavior_spec_doc_id
- acceptance_scenarios_doc_id
- spec_delta_summary`

const openSpecWritingSpecsSkillZH = `# Skill: OpenSpec 规格编写

用于编写行为 spec。

规则：
- Spec 是行为契约，不是实现计划。
- 每条 requirement 描述一个可观察行为。
- 硬性需求使用 MUST/SHALL 语义。
- 重要 requirement 都要有具体 scenario。
- 描述行为变化时使用 ADDED / MODIFIED / REMOVED 语义。
- 实现细节放到 design.md 或 task plan。

输出字段：
- behavior_spec_doc_id
- acceptance_scenarios_doc_id
- spec_delta_summary`

const openSpecDesignTasksSkillEN = `# Skill: OpenSpec Design And Tasks

Use this after behavior specs are approved.

Design should cover:
- Approach and alternatives considered.
- Dependencies and impacted systems.
- Data, migration, security, privacy, rollout, and observability when relevant.
- Open risks and fallback plan.

Tasks should be:
- Ordered.
- Concrete.
- Verifiable.
- Small enough for one agent or one human owner to execute.

Output fields:
- design_doc_id
- task_plan_doc_id
- execution_risks`

const openSpecDesignTasksSkillZH = `# Skill: OpenSpec 设计与任务

用于行为 spec 审核通过后。

Design 应覆盖：
- 方案和考虑过的替代方案。
- 依赖和受影响系统。
- 相关时包含数据、迁移、安全、隐私、上线和可观测性。
- 未决风险和兜底方案。

Tasks 应该：
- 有顺序。
- 具体。
- 可验证。
- 足够小，能由一个 Agent 或一个人负责执行。

输出字段：
- design_doc_id
- task_plan_doc_id
- execution_risks`

const openSpecReviewSkillEN = `# Skill: OpenSpec Review Change

Use this before expensive implementation starts.

Review checklist:
- Is this one change, or should it split?
- Is the problem real and the scope worth doing?
- Are non-goals explicit?
- Are requirements observable and testable?
- Are scenarios concrete and representative?
- Are implementation details kept out of behavior specs?
- Are risks clear enough to approve, rework, or stop?

Output fields:
- decision
- comments`

const openSpecReviewSkillZH = `# Skill: OpenSpec 变更评审

用于昂贵实现开始前。

评审清单：
- 这是一个变更，还是应该拆分？
- 问题是否真实，范围是否值得做？
- 非目标是否明确？
- 需求是否可观察、可测试？
- 场景是否具体且有代表性？
- 实现细节是否没有混入行为 spec？
- 风险是否清楚到可以通过、打回或停止？

输出字段：
- decision
- comments`

const openSpecApplySkillEN = `# Skill: OpenSpec Apply

Use this to execute an approved change.

Rules:
- Read the approved proposal, behavior specs, design, and task plan first.
- Work through tasks in order unless a dependency requires reordering.
- If implementation contradicts a spec, stop and report the mismatch.
- Keep tests and evidence tied to the spec scenarios.
- Do not hide changed assumptions; send them back to review.

Output fields:
- implementation_summary_doc_id
- change_artifact_ref
- test_evidence_doc_id`

const openSpecApplySkillZH = `# Skill: OpenSpec 执行

用于执行已审核变更。

规则：
- 先读取已审核 proposal、行为 spec、design 和 task plan。
- 默认按任务顺序执行，除非依赖要求重新排序。
- 如果实现与 spec 冲突，停止并报告不一致。
- 测试和证据要关联到 spec 场景。
- 不隐藏已变化的假设，把它们打回评审。

输出字段：
- implementation_summary_doc_id
- change_artifact_ref
- test_evidence_doc_id`

const openSpecVerifySkillEN = `# Skill: OpenSpec Verify

Use this to decide whether the change is actually done.

Verify:
- Proposal scope and non-goals.
- Each behavior requirement.
- Each acceptance scenario.
- Task checklist completion.
- Evidence quality.
- Remaining manual checks and rollout risks.

Output fields:
- verification_report_doc_id
- decision
- remaining_risks`

const openSpecVerifySkillZH = `# Skill: OpenSpec 验证

用于判断变更是否真的完成。

验证：
- Proposal 范围和非目标。
- 每条行为需求。
- 每个验收场景。
- 任务清单完成情况。
- 证据质量。
- 剩余人工检查和上线风险。

输出字段：
- verification_report_doc_id
- decision
- remaining_risks`

const openSpecArchiveSkillEN = `# Skill: OpenSpec Archive

Use this after verification passes.

Archive:
- Link proposal, behavior specs, design, task plan, implementation evidence, and verification report.
- Extract durable behavior facts into knowledge docs.
- Record decisions that future agents should not relitigate.
- Identify repeated review comments or failures that should become skills, prompts, or workflow changes.

Output fields:
- archive_doc_id
- skill_candidates_doc_id`

const openSpecArchiveSkillZH = `# Skill: OpenSpec 归档

用于验证通过后。

归档：
- 链接 proposal、行为 spec、design、task plan、实现证据和验证报告。
- 把长期行为事实提炼到知识库。
- 记录后续 Agent 不应重复争论的决策。
- 识别重复评审意见或失败模式，并建议沉淀成 skill、prompt 或流程变化。

输出字段：
- archive_doc_id
- skill_candidates_doc_id`

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
