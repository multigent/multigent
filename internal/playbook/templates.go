package playbook

import (
	"strings"

	"github.com/multigent/multigent/internal/entity"
	workflowstore "github.com/multigent/multigent/internal/workflow"
)

func Templates(locale string) []entity.PlaybookTemplate {
	locale = normalizeLocale(locale)
	return []entity.PlaybookTemplate{
		softwareDelivery(locale),
		startupValidation(locale),
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
		Name:        text(locale, "Garry Startup Validation", "Garry 创业验证协作方案"),
		Description: text(locale, "A startup idea validation playbook inspired by YC-style office hours: test demand reality, status quo, pain intensity, wedge, and prototype scope before building.", "参考 YC office hours 思路的创业项目验证协作方案：先判断真实需求、现有替代方案、痛点强度、切入点和原型范围，再决定是否开发。"),
		Locale:      normalizeLocale(locale),
		Category:    text(locale, "Strategy", "战略验证"),
		Complexity:  text(locale, "Intermediate", "中阶"),
		Tags:        []string{"startup", "market", "yc", "strategy"},
		Roles: []entity.PlaybookRoleTemplate{
			role("yc-office-hours-advisor", "business", "advisor", text(locale, "YC Office Hours Advisor", "YC Office Hours 顾问"), text(locale, "Challenges startup ideas through demand reality, status quo, desperate user signal, wedge, and future-fit questions.", "通过真实需求、现有替代方案、强痛点用户信号、最小切入点和未来适配性来挑战创业想法。"), []string{"market-value-evaluation", "status-quo-analysis"}),
			role("market-research-agent", "business", "market-research", text(locale, "Market Research Agent", "市场调研 Agent"), text(locale, "Collects evidence about buyers, current alternatives, urgency, and willingness to switch.", "收集买方、当前替代方案、紧迫性和迁移意愿证据。"), []string{"market-value-evaluation"}),
			role("prototype-scope-reviewer", "product", "prototype", text(locale, "Prototype Scope Reviewer", "原型范围评审"), text(locale, "Turns the strongest validated wedge into a concrete 48-hour prototype scope.", "把最强切入点转成可执行的 48 小时原型范围。"), []string{"prototype-scope"}),
		},
		Skills: []entity.PlaybookSkillTemplate{
			skill("market-value-evaluation", text(locale, "Market Value Evaluation", "市场价值判断"), text(locale, "Evaluate whether the idea has a painful, specific, reachable market rather than a vague nice-to-have.", "判断想法是否有具体、强痛点、可触达的市场，而不是泛泛的锦上添花。")),
			skill("status-quo-analysis", text(locale, "Status Quo Analysis", "现状替代方案分析"), text(locale, "Treat the user's current workaround as the real competitor and identify switching pressure.", "把用户当前土办法视为真正竞品，识别迁移动力。")),
			skill("desperate-user-signal", text(locale, "Desperate User Signal", "强痛点用户信号"), text(locale, "Look for evidence that a narrow group urgently needs the product now.", "寻找某个窄人群现在就迫切需要产品的证据。")),
			skill("prototype-scope", text(locale, "48-hour Prototype Scope", "48 小时原型范围"), text(locale, "Define the smallest prototype that tests the riskiest premise.", "定义能验证最大风险假设的最小原型。")),
		},
		Workflows: []entity.PlaybookWorkflowTemplate{
			workflow("startup-idea-validation", wf, map[string]string{
				"idea_intake":        "yc-office-hours-advisor",
				"market_evidence":    "market-research-agent",
				"validation_review":  "yc-office-hours-advisor",
				"prototype_decision": "prototype-scope-reviewer",
			}, map[string][]string{
				"idea_intake":        {"market-value-evaluation", "status-quo-analysis"},
				"market_evidence":    {"market-value-evaluation", "desperate-user-signal"},
				"prototype_decision": {"prototype-scope"},
			}),
		},
		TaskTemplates: []entity.PlaybookTaskTemplate{
			task("validate-startup-idea", text(locale, "Validate a startup idea", "验证一个创业想法"), text(locale, "Pressure-test a raw idea before committing build time.", "在投入开发前对原始想法进行压力测试。"), "startup-idea-validation"),
			task("prototype-scope", text(locale, "Create prototype scope", "制定原型范围"), text(locale, "Produce a 48-hour prototype scope from validated evidence.", "基于验证证据输出 48 小时原型范围。"), "startup-idea-validation"),
		},
		SetupQuestions: []entity.PlaybookSetupQuestion{
			question("target_user", text(locale, "Who is the narrow first user?", "第一批窄用户是谁？"), nil, true),
			question("current_status_quo", text(locale, "What do they do today without this product?", "没有这个产品时他们现在怎么解决？"), nil, true),
		},
		SuccessMetrics: []entity.PlaybookMetric{
			metric("evidence_quality", text(locale, "Evidence quality", "证据质量"), text(locale, "How specific and uncomfortable the demand evidence is.", "需求证据是否具体且足够尖锐。")),
			metric("prototype_clarity", text(locale, "Prototype clarity", "原型清晰度"), text(locale, "Whether the prototype scope tests one risky premise clearly.", "原型是否清晰验证一个关键风险假设。")),
		},
	}
}

func bugTriageAndFix(locale string) entity.PlaybookTemplate {
	wf, _ := workflowstore.Template("agentic-bug-triage-loop", locale)
	return entity.PlaybookTemplate{
		ID:          "bug-triage-and-fix",
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

func role(id, team, roleName, name, description string, skills []string) entity.PlaybookRoleTemplate {
	return entity.PlaybookRoleTemplate{ID: id, Team: team, Role: roleName, Name: name, Description: description, Skills: skills}
}

func skill(id, name, description string) entity.PlaybookSkillTemplate {
	return entity.PlaybookSkillTemplate{ID: id, Name: name, Description: description}
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
