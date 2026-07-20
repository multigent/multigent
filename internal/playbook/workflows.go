package playbook

import "github.com/multigent/multigent/internal/entity"

func startupValidationWorkflow(locale string) entity.WorkflowTemplate {
	field := func(name, en, zh string) entity.WorkflowField {
		return entity.WorkflowField{Name: name, Description: text(locale, en, zh)}
	}
	return entity.WorkflowTemplate{
		ID:          "startup-idea-validation",
		Name:        text(locale, "YC Garry gstack Delivery Loop", "YC Garry gstack 交付闭环"),
		Description: text(locale, "Run the actual gstack method as a workflow: office hours, CEO review, spec, engineering/design review, implementation review, QA, and ship.", "把 gstack 方法作为流程运行：office hours、CEO review、spec、工程/设计评审、实现评审、QA 和发版。"),
		Version:     1,
		Locale:      normalizeLocale(locale),
		StartStepID: "office_hours",
		Steps: []entity.WorkflowStep{
			{
				ID: "office_hours", Type: "agent_task", Title: text(locale, "YC Office Hours", "YC Office Hours"),
				Description:  text(locale, "Use gstack /office-hours to run the product diagnostic and produce a design doc before implementation.", "使用 gstack /office-hours 运行产品诊断，在实现前产出设计文档。"),
				ActorRole:    "yc-office-hours-partner",
				InputFields:  []entity.WorkflowField{field("raw_idea", "Raw idea, product request, founder note, or customer problem.", "原始想法、产品需求、创始人描述或客户问题。")},
				OutputFields: []entity.WorkflowField{field("design_doc_id", "docID for the gstack office-hours design document.", "gstack office-hours 设计文档 docID。"), field("problem_statement", "Specific problem statement after diagnosis.", "诊断后的具体问题陈述。"), field("recommended_next_mode", "Recommended next mode or decision.", "建议的下一步模式或决策。")},
				Position:     entity.WorkflowPosition{X: 80, Y: 180},
				Config:       map[string]string{"color": "sky"},
			},
			{
				ID: "ceo_review", Type: "agent_task", Title: text(locale, "CEO Review", "CEO Review"),
				Description:  text(locale, "Use gstack /plan-ceo-review to challenge ambition, scope, failure modes, and the 10-star version.", "使用 gstack /plan-ceo-review 挑战野心、范围、失败模式和 10-star 版本。"),
				ActorRole:    "yc-founder-reviewer",
				InputFields:  []entity.WorkflowField{field("design_doc_id", "docID from office-hours.", "office-hours 产出的 docID。"), field("problem_statement", "Problem statement to review.", "需要评审的问题陈述。")},
				OutputFields: []entity.WorkflowField{field("ceo_review_doc_id", "docID for CEO review report.", "CEO review 报告 docID。"), field("scope_decision", "expand, selective_expand, hold_scope, reduce, or stop.", "expand、selective_expand、hold_scope、reduce 或 stop。"), field("accepted_changes", "Accepted scope changes or constraints.", "已接受的范围变化或约束。")},
				Position:     entity.WorkflowPosition{X: 360, Y: 180},
				Config:       map[string]string{"color": "violet"},
			},
			{
				ID: "founder_gate", Type: "human_review", Title: text(locale, "Founder Gate", "创始人审核"),
				Description:  text(locale, "Human decides whether to proceed, request changes, or stop after office-hours and CEO review.", "人类基于 office-hours 和 CEO review 决定继续、打回修改或停止。"),
				ActorRole:    "founder",
				InputFields:  []entity.WorkflowField{field("design_doc_id", "Office-hours design docID.", "office-hours 设计文档 docID。"), field("ceo_review_doc_id", "CEO review docID.", "CEO review 文档 docID。"), field("scope_decision", "CEO review scope decision.", "CEO review 范围决策。")},
				OutputFields: []entity.WorkflowField{field("decision", "approve, request_changes, or stop.", "approve、request_changes 或 stop。"), field("comments", "Founder comments and constraints.", "创始人意见和约束。")},
				ReviewPolicy: "manual",
				Position:     entity.WorkflowPosition{X: 640, Y: 180},
				Config:       map[string]string{"color": "amber"},
			},
			{
				ID: "spec_authoring", Type: "agent_task", Title: text(locale, "Spec Authoring", "Spec 编写"),
				Description:  text(locale, "Use gstack /spec to turn the approved direction into a precise executable spec.", "使用 gstack /spec 把已批准方向转成精确可执行 spec。"),
				ActorRole:    "gstack-spec-author",
				InputFields:  []entity.WorkflowField{field("design_doc_id", "Office-hours design docID.", "office-hours 设计文档 docID。"), field("ceo_review_doc_id", "CEO review docID.", "CEO review 文档 docID。"), field("comments", "Founder gate comments.", "创始人审核意见。")},
				OutputFields: []entity.WorkflowField{field("spec_doc_id", "Executable spec docID.", "可执行 spec docID。"), field("acceptance_criteria_doc_id", "Acceptance criteria docID.", "验收标准 docID。")},
				Position:     entity.WorkflowPosition{X: 920, Y: 180},
				Config:       map[string]string{"color": "emerald"},
			},
			{
				ID: "engineering_review", Type: "agent_task", Title: text(locale, "Engineering Review", "工程评审"),
				Description:  text(locale, "Use gstack /plan-eng-review to validate architecture, data flow, failure paths, tests, and observability.", "使用 gstack /plan-eng-review 验证架构、数据流、失败路径、测试和可观测性。"),
				ActorRole:    "gstack-eng-reviewer",
				InputFields:  []entity.WorkflowField{field("spec_doc_id", "Executable spec docID.", "可执行 spec docID。")},
				OutputFields: []entity.WorkflowField{field("engineering_review_doc_id", "Engineering review report docID.", "工程评审报告 docID。"), field("engineering_findings", "Important engineering findings.", "重要工程发现。")},
				Position:     entity.WorkflowPosition{X: 1200, Y: 80},
				Config:       map[string]string{"color": "blue"},
			},
			{
				ID: "design_review", Type: "agent_task", Title: text(locale, "Design Review", "设计评审"),
				Description:  text(locale, "Use gstack /plan-design-review when the work has UX, UI, interaction, or user-facing behavior.", "当工作涉及 UX、UI、交互或用户可见行为时，使用 gstack /plan-design-review。"),
				ActorRole:    "gstack-design-reviewer",
				InputFields:  []entity.WorkflowField{field("spec_doc_id", "Executable spec docID.", "可执行 spec docID。")},
				OutputFields: []entity.WorkflowField{field("design_review_doc_id", "Design review report docID.", "设计评审报告 docID。"), field("design_findings", "Important design findings.", "重要设计发现。")},
				Position:     entity.WorkflowPosition{X: 1200, Y: 280},
				Config:       map[string]string{"color": "pink"},
			},
			{
				ID: "implementation_review", Type: "agent_task", Title: text(locale, "Implementation Review", "实现评审"),
				Description:  text(locale, "Use gstack /review to check implementation quality after code is produced.", "代码产出后使用 gstack /review 检查实现质量。"),
				ActorRole:    "gstack-staff-reviewer",
				InputFields:  []entity.WorkflowField{field("spec_doc_id", "Executable spec docID.", "可执行 spec docID。"), field("engineering_review_doc_id", "Engineering review report docID.", "工程评审报告 docID。"), field("design_review_doc_id", "Design review report docID if applicable.", "如适用，设计评审报告 docID。")},
				OutputFields: []entity.WorkflowField{field("review_report_doc_id", "Code review report docID.", "代码评审报告 docID。"), field("review_decision", "approve or request_changes.", "approve 或 request_changes。")},
				Position:     entity.WorkflowPosition{X: 1480, Y: 180},
				Config:       map[string]string{"color": "slate"},
			},
			{
				ID: "qa", Type: "agent_task", Title: text(locale, "QA", "QA"),
				Description:  text(locale, "Use gstack /qa to test real flows, capture evidence, and verify fixes.", "使用 gstack /qa 测试真实流程、采集证据并验证修复。"),
				ActorRole:    "gstack-qa-lead",
				InputFields:  []entity.WorkflowField{field("review_report_doc_id", "Code review report docID.", "代码评审报告 docID。"), field("spec_doc_id", "Executable spec docID.", "可执行 spec docID。")},
				OutputFields: []entity.WorkflowField{field("qa_report_doc_id", "QA report docID.", "QA 报告 docID。"), field("qa_decision", "pass or fail.", "pass 或 fail。")},
				Position:     entity.WorkflowPosition{X: 1760, Y: 180},
				Config:       map[string]string{"color": "orange"},
			},
			{
				ID: "ship", Type: "agent_task", Title: text(locale, "Ship", "发版"),
				Description:  text(locale, "Use gstack /ship to verify readiness, prepare PR/release evidence, and close the loop.", "使用 gstack /ship 验证就绪状态、准备 PR/发版证据并闭环。"),
				ActorRole:    "gstack-release-engineer",
				InputFields:  []entity.WorkflowField{field("qa_report_doc_id", "QA report docID.", "QA 报告 docID。"), field("qa_decision", "QA decision.", "QA 决策。")},
				OutputFields: []entity.WorkflowField{field("ship_report_doc_id", "Ship report or PR/release evidence docID.", "发版报告或 PR/发版证据 docID。"), field("release_decision", "ready, blocked, or shipped.", "ready、blocked 或 shipped。")},
				Position:     entity.WorkflowPosition{X: 2040, Y: 180},
				Config:       map[string]string{"color": "emerald"},
			},
		},
		Edges: []entity.WorkflowEdge{
			edge("e-office-ceo", "office_hours", "ceo_review", "", nil, nil, true),
			edge("e-ceo-founder", "ceo_review", "founder_gate", "", nil, nil, true),
			edge("e-founder-spec", "founder_gate", "spec_authoring", text(locale, "approved", "通过"), cond("decision", "eq", "approve"), map[string]string{"design_doc_id": "$input.design_doc_id", "ceo_review_doc_id": "$input.ceo_review_doc_id", "comments": "$output.comments"}, false),
			edge("e-founder-rework", "founder_gate", "office_hours", text(locale, "needs changes", "需要修改"), cond("decision", "eq", "request_changes"), map[string]string{"review_comments": "$output.comments", "design_doc_id": "$input.design_doc_id", "ceo_review_doc_id": "$input.ceo_review_doc_id"}, false),
			edge("e-spec-eng", "spec_authoring", "engineering_review", "", nil, nil, true),
			edge("e-spec-design", "spec_authoring", "design_review", text(locale, "if user-facing", "涉及用户体验"), nil, nil, false),
			edge("e-eng-review", "engineering_review", "implementation_review", "", nil, nil, true),
			edge("e-design-review", "design_review", "implementation_review", "", nil, nil, false),
			edge("e-review-qa", "implementation_review", "qa", text(locale, "approved", "通过"), cond("review_decision", "eq", "approve"), nil, false),
			edge("e-review-rework", "implementation_review", "spec_authoring", text(locale, "needs changes", "需要修改"), cond("review_decision", "eq", "request_changes"), nil, false),
			edge("e-qa-ship", "qa", "ship", text(locale, "passed", "通过"), cond("qa_decision", "eq", "pass"), nil, false),
			edge("e-qa-rework", "qa", "implementation_review", text(locale, "failed", "失败"), cond("qa_decision", "eq", "fail"), nil, false),
		},
	}
}

func openSpecWorkflow(locale string) entity.WorkflowTemplate {
	field := func(name, en, zh string) entity.WorkflowField {
		return entity.WorkflowField{Name: name, Description: text(locale, en, zh)}
	}
	return entity.WorkflowTemplate{
		ID:          "openspec-artifact-guided-delivery",
		Name:        text(locale, "OpenSpec Artifact-Guided Delivery", "OpenSpec 规格化交付闭环"),
		Description: text(locale, "A lightweight OpenSpec-style loop: explore intent, draft proposal, write behavior specs, review, design tasks, implement, verify, and archive reusable knowledge.", "轻量 OpenSpec 风格闭环：探索意图、起草 proposal、编写行为 spec、评审、设计任务、实现、验证，并归档可复用知识。"),
		Version:     1,
		Locale:      normalizeLocale(locale),
		StartStepID: "explore",
		Steps: []entity.WorkflowStep{
			{
				ID: "explore", Type: "agent_task", Title: text(locale, "Explore Intent", "探索意图"),
				Description: text(locale, "Clarify the raw request, inspect available context, and decide whether this should become a scoped change. Large findings must be saved as docs and returned by docID.", "澄清原始请求，检查已有上下文，并判断是否值得形成有边界的变更。大段发现必须写入知识库并返回 docID。"),
				ActorRole:   "openspec-explorer",
				InputFields: []entity.WorkflowField{
					field("raw_request", "Raw idea, bug, customer request, process problem, or change intent.", "原始想法、Bug、客户请求、流程问题或变更意图。"),
					field("known_context_doc_id", "Optional docID for existing context, meeting notes, issue, or customer signal.", "可选，已有上下文、会议纪要、Issue 或客户信号的 docID。"),
				},
				OutputFields: []entity.WorkflowField{
					field("exploration_doc_id", "docID for explored facts, assumptions, risks, and open questions.", "探索事实、假设、风险和未决问题的 docID。"),
					field("change_candidate", "One-sentence scoped change candidate.", "一句话描述的有边界变更候选。"),
					field("decision", "propose, clarify_more, or stop.", "propose、clarify_more 或 stop。"),
				},
				Position: entity.WorkflowPosition{X: 80, Y: 220},
				Config:   map[string]string{"color": "sky"},
			},
			{
				ID: "proposal", Type: "agent_task", Title: text(locale, "Draft Proposal", "起草 Proposal"),
				Description: text(locale, "Draft a proposal that states why the change matters, what is in scope, what is out of scope, expected impact, and risks.", "起草 proposal，说明为什么要改、做什么、不做什么、预期影响和风险。"),
				ActorRole:   "openspec-explorer",
				InputFields: []entity.WorkflowField{
					field("exploration_doc_id", "Exploration docID.", "探索文档 docID。"),
					field("change_candidate", "Scoped change candidate.", "有边界变更候选。"),
				},
				OutputFields: []entity.WorkflowField{
					field("proposal_doc_id", "docID for proposal artifact.", "proposal 产物 docID。"),
					field("scope_summary", "Concise scope summary.", "简洁范围摘要。"),
					field("risk_summary", "Material risks, dependencies, or rollout concerns.", "关键风险、依赖或上线顾虑。"),
				},
				Position: entity.WorkflowPosition{X: 360, Y: 220},
				Config:   map[string]string{"color": "violet"},
			},
			{
				ID: "proposal_review", Type: "human_review", Title: text(locale, "Review Proposal", "审核 Proposal"),
				Description: text(locale, "Human reviewer confirms this is the right problem, scope, and review level before behavior specs are written.", "人类审核人确认问题、范围和评审级别是否正确，然后再进入行为 spec。"),
				ActorRole:   "request-owner",
				InputFields: []entity.WorkflowField{
					field("proposal_doc_id", "Proposal docID.", "proposal docID。"),
					field("scope_summary", "Scope summary.", "范围摘要。"),
					field("risk_summary", "Risk summary.", "风险摘要。"),
				},
				OutputFields: []entity.WorkflowField{
					field("decision", "approve, request_changes, or stop.", "approve、request_changes 或 stop。"),
					field("comments", "Review comments, constraints, or rejection reason.", "审核意见、约束或拒绝原因。"),
				},
				ReviewPolicy: "manual",
				Position:     entity.WorkflowPosition{X: 640, Y: 220},
				Config:       map[string]string{"color": "amber"},
			},
			{
				ID: "specs", Type: "agent_task", Title: text(locale, "Write Behavior Specs", "编写行为 Spec"),
				Description: text(locale, "Write behavior-first requirements and scenarios. Requirements should describe externally observable behavior; implementation details belong in design.", "编写以行为为中心的需求与场景。需求只描述外部可观察行为；实现细节放到 design。"),
				ActorRole:   "openspec-spec-author",
				InputFields: []entity.WorkflowField{
					field("proposal_doc_id", "Approved proposal docID.", "已审核 proposal docID。"),
					field("comments", "Proposal review comments or constraints.", "proposal 审核意见或约束。"),
				},
				OutputFields: []entity.WorkflowField{
					field("behavior_spec_doc_id", "docID for behavior requirements.", "行为需求 docID。"),
					field("acceptance_scenarios_doc_id", "docID for acceptance scenarios and edge cases.", "验收场景和边界情况 docID。"),
					field("spec_delta_summary", "Summary of added, modified, or removed behavior.", "新增、修改或删除行为的摘要。"),
				},
				Position: entity.WorkflowPosition{X: 920, Y: 220},
				Config:   map[string]string{"color": "emerald"},
			},
			{
				ID: "spec_review", Type: "human_review", Title: text(locale, "Review Specs", "审核 Spec"),
				Description: text(locale, "Human reviewer checks whether the behavior spec is observable, right-sized, and testable before implementation design starts.", "人类审核 spec 是否可观察、范围合适、可测试，然后再进入实现设计。"),
				ActorRole:   "request-owner",
				InputFields: []entity.WorkflowField{
					field("behavior_spec_doc_id", "Behavior spec docID.", "行为 spec docID。"),
					field("acceptance_scenarios_doc_id", "Acceptance scenarios docID.", "验收场景 docID。"),
					field("spec_delta_summary", "Behavior delta summary.", "行为变化摘要。"),
				},
				OutputFields: []entity.WorkflowField{
					field("decision", "approve or request_changes.", "approve 或 request_changes。"),
					field("comments", "Concrete spec changes needed, or approval notes.", "具体 spec 修改意见或通过说明。"),
				},
				ReviewPolicy: "manual",
				Position:     entity.WorkflowPosition{X: 1200, Y: 220},
				Config:       map[string]string{"color": "amber"},
			},
			{
				ID: "design_tasks", Type: "agent_task", Title: text(locale, "Design And Tasks", "设计与任务"),
				Description: text(locale, "Write implementation design and an execution checklist. Keep design choices, dependencies, rollout notes, and task order outside the behavior spec.", "编写实现设计和执行清单。把设计选择、依赖、上线说明和任务顺序放在行为 spec 之外。"),
				ActorRole:   "openspec-design-planner",
				InputFields: []entity.WorkflowField{
					field("behavior_spec_doc_id", "Approved behavior spec docID.", "已审核行为 spec docID。"),
					field("acceptance_scenarios_doc_id", "Acceptance scenarios docID.", "验收场景 docID。"),
					field("comments", "Spec review comments if any.", "如有，spec 审核意见。"),
				},
				OutputFields: []entity.WorkflowField{
					field("design_doc_id", "docID for implementation design.", "实现设计 docID。"),
					field("task_plan_doc_id", "docID for ordered task checklist.", "有序任务清单 docID。"),
					field("execution_risks", "Dependencies, rollout risks, and verification risks.", "依赖、上线风险和验证风险。"),
				},
				Position: entity.WorkflowPosition{X: 1480, Y: 220},
				Config:   map[string]string{"color": "blue"},
			},
			{
				ID: "implementation", Type: "agent_task", Title: text(locale, "Implement Change", "实现变更"),
				Description: text(locale, "Execute the approved task plan. If implementation discovers changed assumptions, update artifacts instead of silently diverging.", "执行已审核任务计划。如果实现中发现假设变化，更新产物而不是静默偏离。"),
				ActorRole:   "openspec-implementation-agent",
				InputFields: []entity.WorkflowField{
					field("design_doc_id", "Implementation design docID.", "实现设计 docID。"),
					field("task_plan_doc_id", "Task plan docID.", "任务计划 docID。"),
					field("execution_risks", "Risks and constraints from design.", "设计阶段的风险和约束。"),
					field("review_comments", "Verification comments when this is a rework pass.", "返工时的验证意见。"),
				},
				OutputFields: []entity.WorkflowField{
					field("implementation_summary_doc_id", "docID for implementation summary.", "实现总结 docID。"),
					field("change_artifact_ref", "PR, patch, deployment, document, or other delivery reference.", "PR、补丁、部署、文档或其他交付引用。"),
					field("test_evidence_doc_id", "docID for test commands, checks, screenshots, or other evidence.", "测试命令、检查、截图或其他证据 docID。"),
				},
				Position: entity.WorkflowPosition{X: 1760, Y: 220},
				Config:   map[string]string{"color": "cyan"},
			},
			{
				ID: "verify", Type: "agent_task", Title: text(locale, "Verify Against Specs", "按 Spec 验证"),
				Description: text(locale, "Verify delivered work against the proposal, behavior specs, scenarios, design, tasks, and evidence.", "按 proposal、行为 spec、场景、设计、任务和证据验证交付。"),
				ActorRole:   "openspec-verifier",
				InputFields: []entity.WorkflowField{
					field("proposal_doc_id", "Proposal docID.", "proposal docID。"),
					field("behavior_spec_doc_id", "Behavior spec docID.", "行为 spec docID。"),
					field("acceptance_scenarios_doc_id", "Acceptance scenarios docID.", "验收场景 docID。"),
					field("implementation_summary_doc_id", "Implementation summary docID.", "实现总结 docID。"),
					field("test_evidence_doc_id", "Test evidence docID.", "测试证据 docID。"),
				},
				OutputFields: []entity.WorkflowField{
					field("verification_report_doc_id", "docID for verification report.", "验证报告 docID。"),
					field("decision", "pass or fail.", "pass 或 fail。"),
					field("remaining_risks", "Remaining risks or manual checks.", "剩余风险或人工检查点。"),
				},
				Position: entity.WorkflowPosition{X: 2040, Y: 220},
				Config:   map[string]string{"color": "orange"},
			},
			{
				ID: "archive", Type: "agent_task", Title: text(locale, "Archive Knowledge", "归档知识"),
				Description: text(locale, "Archive the completed change into durable docs and extract reusable skills, workflow improvements, or spec conventions.", "把已完成变更归档为长期文档，并提炼可复用 skill、流程改进或 spec 约定。"),
				ActorRole:   "openspec-archivist",
				InputFields: []entity.WorkflowField{
					field("proposal_doc_id", "Proposal docID.", "proposal docID。"),
					field("behavior_spec_doc_id", "Behavior spec docID.", "行为 spec docID。"),
					field("design_doc_id", "Design docID.", "设计 docID。"),
					field("verification_report_doc_id", "Verification report docID.", "验证报告 docID。"),
				},
				OutputFields: []entity.WorkflowField{
					field("archive_doc_id", "docID for archived final change record.", "最终变更归档 docID。"),
					field("skill_candidates_doc_id", "docID for reusable skill or process improvement candidates.", "可复用 skill 或流程改进候选 docID。"),
				},
				Position: entity.WorkflowPosition{X: 2320, Y: 220},
				Config:   map[string]string{"color": "emerald"},
			},
		},
		Edges: []entity.WorkflowEdge{
			edge("e-explore-proposal", "explore", "proposal", text(locale, "propose", "进入 proposal"), cond("decision", "eq", "propose"), nil, false),
			edge("e-proposal-review", "proposal", "proposal_review", "", nil, nil, true),
			edge("e-proposal-approved", "proposal_review", "specs", text(locale, "approved", "通过"), cond("decision", "eq", "approve"), map[string]string{"proposal_doc_id": "$input.proposal_doc_id", "comments": "$output.comments"}, false),
			edge("e-proposal-rework", "proposal_review", "proposal", text(locale, "needs changes", "需要修改"), cond("decision", "eq", "request_changes"), map[string]string{"proposal_doc_id": "$input.proposal_doc_id", "review_comments": "$output.comments"}, false),
			edge("e-specs-review", "specs", "spec_review", "", nil, nil, true),
			edge("e-spec-approved", "spec_review", "design_tasks", text(locale, "approved", "通过"), cond("decision", "eq", "approve"), map[string]string{"behavior_spec_doc_id": "$input.behavior_spec_doc_id", "acceptance_scenarios_doc_id": "$input.acceptance_scenarios_doc_id", "comments": "$output.comments"}, false),
			edge("e-spec-rework", "spec_review", "specs", text(locale, "needs changes", "需要修改"), cond("decision", "eq", "request_changes"), map[string]string{"behavior_spec_doc_id": "$input.behavior_spec_doc_id", "acceptance_scenarios_doc_id": "$input.acceptance_scenarios_doc_id", "review_comments": "$output.comments"}, false),
			edge("e-design-implementation", "design_tasks", "implementation", "", nil, nil, true),
			edge("e-implementation-verify", "implementation", "verify", "", nil, nil, true),
			edge("e-verify-archive", "verify", "archive", text(locale, "passed", "通过"), cond("decision", "eq", "pass"), nil, false),
			edge("e-verify-rework", "verify", "implementation", text(locale, "failed", "失败"), cond("decision", "eq", "fail"), map[string]string{"review_comments": "$output.remaining_risks", "verification_report_doc_id": "$output.verification_report_doc_id"}, false),
		},
	}
}

func mattPocockEngineeringWorkflow(locale string) entity.WorkflowTemplate {
	field := func(name, en, zh string) entity.WorkflowField {
		return entity.WorkflowField{Name: name, Description: text(locale, en, zh)}
	}
	return entity.WorkflowTemplate{
		ID:          "matt-pocock-real-engineering",
		Name:        text(locale, "Matt Pocock Real Engineering Loop", "Matt Pocock 真实工程闭环"),
		Description: text(locale, "A full engineering workflow based on Matt Pocock's stable skills: setup context, grill requirements, produce spec and tickets, implement with TDD, review architecture and code, debug failures, and hand off reusable knowledge.", "基于 Matt Pocock 稳定 skills 的完整工程流程：建立上下文、追问需求、产出 spec 和 tickets、TDD 实现、架构与代码审核、排障修复，并交接可复用知识。"),
		Version:     1,
		Locale:      normalizeLocale(locale),
		StartStepID: "setup_context",
		Steps: []entity.WorkflowStep{
			{
				ID: "setup_context", Type: "agent_task", Title: text(locale, "Set Up Engineering Context", "建立工程上下文"),
				Description: text(locale, "Use setup-matt-pocock-skills and wayfinder to locate tracker rules, domain glossary, architecture notes, test seams, and existing decisions. Large outputs must be knowledge docs and returned by docID.", "使用 setup-matt-pocock-skills 和 wayfinder 定位任务规则、领域词汇、架构说明、测试边界和已有决策。大段输出必须写入知识库并返回 docID。"),
				ActorRole:   "matt-wayfinder",
				InputFields: []entity.WorkflowField{
					field("raw_request", "The raw feature, bug, or engineering request.", "原始功能、Bug 或工程请求。"),
					field("repo_or_project_context", "Available repository, tracker, or project context. Use docID when long.", "已有仓库、任务系统或项目上下文。内容较长时使用 docID。"),
				},
				OutputFields: []entity.WorkflowField{
					field("setup_doc_id", "docID for tracker rules, labels, conventions, and current setup gaps.", "任务规则、标签、约定和当前配置缺口的 docID。"),
					field("domain_context_doc_id", "docID for relevant domain glossary, architecture, and prior decisions.", "相关领域词汇、架构和历史决策的 docID。"),
					field("open_questions", "Specific questions still blocking a good spec.", "仍然阻塞高质量 spec 的具体问题。"),
				},
				Position: entity.WorkflowPosition{X: 80, Y: 220},
				Config:   map[string]string{"color": "sky"},
			},
			{
				ID: "clarify", Type: "agent_task", Title: text(locale, "Grill Requirements", "追问需求"),
				Description: text(locale, "Use grill-with-docs, grill-me, grilling, and ask-matt to pressure-test the request against docs and expose ambiguity before writing the spec.", "使用 grill-with-docs、grill-me、grilling 和 ask-matt，基于文档对需求做压力测试，在写 spec 前暴露模糊点。"),
				ActorRole:   "matt-wayfinder",
				InputFields: []entity.WorkflowField{
					field("raw_request", "Original request.", "原始需求。"),
					field("setup_doc_id", "Setup docID from the previous step.", "上一步的 setup docID。"),
					field("domain_context_doc_id", "Domain context docID from the previous step.", "上一步的领域上下文 docID。"),
					field("open_questions", "Open questions from setup.", "建立上下文后的未决问题。"),
				},
				OutputFields: []entity.WorkflowField{
					field("clarification_doc_id", "docID for clarified intent, assumptions, and answered questions.", "澄清后的意图、假设和已回答问题的 docID。"),
					field("decision_log_doc_id", "docID for decisions made during grilling.", "追问过程中形成的决策记录 docID。"),
					field("spec_readiness", "ready, needs_human_input, or stop.", "ready、needs_human_input 或 stop。"),
				},
				Position: entity.WorkflowPosition{X: 360, Y: 220},
				Config:   map[string]string{"color": "violet"},
			},
			{
				ID: "human_clarification", Type: "human_review", Title: text(locale, "Human Clarification", "人工澄清"),
				Description: text(locale, "A human answers unresolved questions or rejects the request if the problem is not worth execution.", "人类回答未决问题；如果问题不值得执行，也可以终止。"),
				ActorRole:   "request-owner",
				InputFields: []entity.WorkflowField{
					field("clarification_doc_id", "Clarification docID.", "澄清文档 docID。"),
					field("open_questions", "Questions requiring human judgment.", "需要人类判断的问题。"),
				},
				OutputFields: []entity.WorkflowField{
					field("decision", "approve, request_changes, or stop.", "approve、request_changes 或 stop。"),
					field("comments", "Answers, constraints, or rejection reason.", "回答、约束或拒绝原因。"),
				},
				ReviewPolicy: "manual",
				Position:     entity.WorkflowPosition{X: 640, Y: 80},
				Config:       map[string]string{"color": "amber"},
			},
			{
				ID: "spec", Type: "agent_task", Title: text(locale, "Write Spec", "编写 Spec"),
				Description: text(locale, "Use to-spec, domain-modeling, and codebase-design to produce a decision-ready spec. Store the full spec and acceptance criteria as docs and return docIDs.", "使用 to-spec、domain-modeling 和 codebase-design 产出可决策 spec。完整 spec 和验收标准写入知识库并返回 docID。"),
				ActorRole:   "domain-modeler",
				InputFields: []entity.WorkflowField{
					field("clarification_doc_id", "Clarified requirements docID.", "已澄清需求 docID。"),
					field("domain_context_doc_id", "Domain context docID.", "领域上下文 docID。"),
					field("comments", "Human clarification comments if any.", "如有人工澄清意见。"),
				},
				OutputFields: []entity.WorkflowField{
					field("spec_doc_id", "docID for the product/engineering spec.", "产品/工程 spec 的 docID。"),
					field("acceptance_criteria_doc_id", "docID for acceptance criteria.", "验收标准 docID。"),
					field("test_seams_doc_id", "docID for proposed test seams and testing decisions.", "建议测试边界和测试决策 docID。"),
				},
				Position: entity.WorkflowPosition{X: 920, Y: 220},
				Config:   map[string]string{"color": "emerald"},
			},
			{
				ID: "tickets", Type: "agent_task", Title: text(locale, "Split Tickets", "拆分 Tickets"),
				Description: text(locale, "Use to-tickets, triage, research, and wayfinder to split the spec into executable tickets with ordering, dependencies, and risk notes.", "使用 to-tickets、triage、research 和 wayfinder，把 spec 拆成可执行 tickets，包含顺序、依赖和风险说明。"),
				ActorRole:   "ticket-planner",
				InputFields: []entity.WorkflowField{
					field("spec_doc_id", "Spec docID.", "Spec docID。"),
					field("acceptance_criteria_doc_id", "Acceptance criteria docID.", "验收标准 docID。"),
					field("test_seams_doc_id", "Test seams docID.", "测试边界 docID。"),
				},
				OutputFields: []entity.WorkflowField{
					field("tickets_doc_id", "docID for sequenced tickets.", "有序 tickets 的 docID。"),
					field("first_ticket_id", "Tracker ID or docID for the first executable ticket.", "第一个可执行 ticket 的任务系统 ID 或 docID。"),
					field("dependency_notes", "Dependency and parallelization notes.", "依赖关系和可并行说明。"),
				},
				Position: entity.WorkflowPosition{X: 1200, Y: 220},
				Config:   map[string]string{"color": "blue"},
			},
			{
				ID: "implementation", Type: "agent_task", Title: text(locale, "Implement With TDD", "TDD 实现"),
				Description: text(locale, "Use implement, tdd, and prototype where helpful. Produce code, tests, and evidence. The workflow output should reference docs, PRs, or run IDs instead of local paths.", "使用 implement、tdd，必要时使用 prototype。产出代码、测试和证据。流程输出应引用 docID、PR 或 runID，不暴露本地路径。"),
				ActorRole:   "implementation-agent",
				InputFields: []entity.WorkflowField{
					field("tickets_doc_id", "Tickets docID.", "Tickets docID。"),
					field("first_ticket_id", "First ticket or current ticket to implement.", "第一个或当前要实现的 ticket。"),
					field("review_comments", "Review or QA comments when this is a rework pass.", "返工时的审核或 QA 意见。"),
				},
				OutputFields: []entity.WorkflowField{
					field("implementation_summary_doc_id", "docID for implementation summary.", "实现总结 docID。"),
					field("test_evidence_doc_id", "docID for test commands, results, and evidence.", "测试命令、结果和证据 docID。"),
					field("pr_or_change_ref", "PR, branch, patch, or change reference.", "PR、分支、补丁或变更引用。"),
				},
				Position: entity.WorkflowPosition{X: 1480, Y: 220},
				Config:   map[string]string{"color": "cyan"},
			},
			{
				ID: "architecture", Type: "agent_task", Title: text(locale, "Architecture Review", "架构审核"),
				Description: text(locale, "Use improve-codebase-architecture and codebase-design to check maintainability, seams, ownership, and coupling.", "使用 improve-codebase-architecture 和 codebase-design 检查可维护性、测试边界、职责归属和耦合。"),
				ActorRole:   "architecture-reviewer",
				InputFields: []entity.WorkflowField{
					field("spec_doc_id", "Spec docID.", "Spec docID。"),
					field("implementation_summary_doc_id", "Implementation summary docID.", "实现总结 docID。"),
					field("pr_or_change_ref", "PR or change reference.", "PR 或变更引用。"),
				},
				OutputFields: []entity.WorkflowField{
					field("architecture_review_doc_id", "docID for architecture review.", "架构审核 docID。"),
					field("architecture_decision", "approve or request_changes.", "approve 或 request_changes。"),
					field("architecture_comments", "Required architectural changes, if any.", "如有，必须修正的架构意见。"),
				},
				Position: entity.WorkflowPosition{X: 1760, Y: 80},
				Config:   map[string]string{"color": "purple"},
			},
			{
				ID: "code_review", Type: "agent_task", Title: text(locale, "Code Review", "代码审核"),
				Description: text(locale, "Use code-review and resolving-merge-conflicts to review behavior, tests, quality, and merge readiness.", "使用 code-review 和 resolving-merge-conflicts 审核行为、测试、质量和合并就绪度。"),
				ActorRole:   "review-agent",
				InputFields: []entity.WorkflowField{
					field("implementation_summary_doc_id", "Implementation summary docID.", "实现总结 docID。"),
					field("test_evidence_doc_id", "Test evidence docID.", "测试证据 docID。"),
					field("architecture_review_doc_id", "Architecture review docID.", "架构审核 docID。"),
				},
				OutputFields: []entity.WorkflowField{
					field("review_report_doc_id", "docID for code review report.", "代码审核报告 docID。"),
					field("review_decision", "approve or request_changes.", "approve 或 request_changes。"),
					field("review_comments", "Concrete requested changes or approval notes.", "具体修改意见或通过说明。"),
				},
				Position: entity.WorkflowPosition{X: 2040, Y: 220},
				Config:   map[string]string{"color": "slate"},
			},
			{
				ID: "debug_and_fix", Type: "agent_task", Title: text(locale, "Debug And Fix", "排障修复"),
				Description: text(locale, "Use diagnosing-bugs, tdd, and implement to reproduce failures, identify the cause, and produce a targeted fix before returning to review.", "使用 diagnosing-bugs、tdd 和 implement 复现失败、定位原因并定向修复，然后回到审核。"),
				ActorRole:   "implementation-agent",
				InputFields: []entity.WorkflowField{
					field("review_report_doc_id", "Review report docID if review failed.", "审核失败时的审核报告 docID。"),
					field("qa_report_doc_id", "QA report docID if QA failed.", "QA 失败时的 QA 报告 docID。"),
					field("review_comments", "Concrete fix request.", "具体修复要求。"),
				},
				OutputFields: []entity.WorkflowField{
					field("fix_summary_doc_id", "docID for fix summary.", "修复总结 docID。"),
					field("regression_evidence_doc_id", "docID for regression evidence.", "回归验证证据 docID。"),
					field("pr_or_change_ref", "Updated PR, branch, patch, or change reference.", "更新后的 PR、分支、补丁或变更引用。"),
				},
				Position: entity.WorkflowPosition{X: 2320, Y: 420},
				Config:   map[string]string{"color": "rose"},
			},
			{
				ID: "handoff", Type: "agent_task", Title: text(locale, "Handoff And Teach", "交接与沉淀"),
				Description: text(locale, "Use handoff, teach, and writing-great-skills to summarize what changed, what was learned, and what future agents should reuse.", "使用 handoff、teach 和 writing-great-skills 总结变更、经验和后续 Agent 可复用内容。"),
				ActorRole:   "learning-writer",
				InputFields: []entity.WorkflowField{
					field("review_report_doc_id", "Review report docID.", "审核报告 docID。"),
					field("implementation_summary_doc_id", "Implementation summary or latest fix summary docID.", "实现总结或最新修复总结 docID。"),
					field("test_evidence_doc_id", "Test or regression evidence docID.", "测试或回归证据 docID。"),
				},
				OutputFields: []entity.WorkflowField{
					field("handoff_doc_id", "docID for handoff notes.", "交接说明 docID。"),
					field("learnings_doc_id", "docID for reusable learnings, patterns, or skill improvements.", "可复用经验、模式或 skill 改进建议 docID。"),
					field("completion_decision", "done, follow_up_needed, or blocked.", "done、follow_up_needed 或 blocked。"),
				},
				Position: entity.WorkflowPosition{X: 2320, Y: 220},
				Config:   map[string]string{"color": "emerald"},
			},
		},
		Edges: []entity.WorkflowEdge{
			edge("e-setup-clarify", "setup_context", "clarify", "", nil, nil, true),
			edge("e-clarify-spec", "clarify", "spec", text(locale, "ready", "已就绪"), cond("spec_readiness", "eq", "ready"), nil, false),
			edge("e-clarify-human", "clarify", "human_clarification", text(locale, "needs input", "需要澄清"), cond("spec_readiness", "eq", "needs_human_input"), nil, false),
			edge("e-human-spec", "human_clarification", "spec", text(locale, "approved", "通过"), cond("decision", "eq", "approve"), map[string]string{"clarification_doc_id": "$input.clarification_doc_id", "comments": "$output.comments"}, false),
			edge("e-human-clarify", "human_clarification", "clarify", text(locale, "needs changes", "需要修改"), cond("decision", "eq", "request_changes"), map[string]string{"human_comments": "$output.comments", "clarification_doc_id": "$input.clarification_doc_id"}, false),
			edge("e-spec-tickets", "spec", "tickets", "", nil, nil, true),
			edge("e-tickets-implementation", "tickets", "implementation", "", nil, nil, true),
			edge("e-implementation-architecture", "implementation", "architecture", "", nil, nil, true),
			edge("e-architecture-review", "architecture", "code_review", text(locale, "approved", "通过"), cond("architecture_decision", "eq", "approve"), nil, false),
			edge("e-architecture-rework", "architecture", "debug_and_fix", text(locale, "needs changes", "需要修改"), cond("architecture_decision", "eq", "request_changes"), map[string]string{"review_comments": "$output.architecture_comments", "review_report_doc_id": "$output.architecture_review_doc_id"}, false),
			edge("e-review-handoff", "code_review", "handoff", text(locale, "approved", "通过"), cond("review_decision", "eq", "approve"), nil, false),
			edge("e-review-debug", "code_review", "debug_and_fix", text(locale, "needs changes", "需要修改"), cond("review_decision", "eq", "request_changes"), nil, false),
			edge("e-debug-review", "debug_and_fix", "code_review", "", nil, map[string]string{"implementation_summary_doc_id": "$output.fix_summary_doc_id", "test_evidence_doc_id": "$output.regression_evidence_doc_id", "pr_or_change_ref": "$output.pr_or_change_ref"}, true),
		},
	}
}

func supportKnowledgeWorkflow(locale string) entity.WorkflowTemplate {
	field := func(name, en, zh string) entity.WorkflowField {
		return entity.WorkflowField{Name: name, Description: text(locale, en, zh)}
	}
	return entity.WorkflowTemplate{
		ID:          "support-knowledge-loop",
		Name:        text(locale, "Support Knowledge Loop", "客服知识库循环"),
		Description: text(locale, "Convert support questions into reviewed answers, reusable knowledge docs, and product feedback.", "把客服问题转成已审核答复、可复用知识库文档和产品反馈。"),
		Version:     1,
		Locale:      normalizeLocale(locale),
		StartStepID: "support_triage",
		Steps: []entity.WorkflowStep{
			{
				ID: "support_triage", Type: "agent_task", Title: text(locale, "Support Triage", "客服分诊"),
				Description:  text(locale, "Cluster repeated questions, identify urgency, and draft a safe answer.", "聚类重复问题，识别紧急程度，并起草安全答复。"),
				ActorRole:    "support-triage-agent",
				InputFields:  []entity.WorkflowField{field("support_messages", "Support message cluster or ticket links.", "客服消息集合或工单链接。")},
				OutputFields: []entity.WorkflowField{field("answer_draft_doc_id", "docID for answer draft.", "答复草稿 docID。"), field("topic_summary", "Short summary of the support topic.", "客服主题摘要。")},
				Position:     entity.WorkflowPosition{X: 80, Y: 180},
				Config:       map[string]string{"color": "sky"},
			},
			{
				ID: "answer_review", Type: "human_review", Title: text(locale, "Answer Review", "答复审核"),
				Description:  text(locale, "Human reviews whether the answer is accurate, safe, and ready to send.", "人类审核答复是否准确、安全、可发送。"),
				ActorRole:    "support-owner",
				InputFields:  []entity.WorkflowField{field("answer_draft_doc_id", "docID for answer draft.", "答复草稿 docID。")},
				OutputFields: []entity.WorkflowField{field("decision", "approve or request_changes.", "approve 或 request_changes。"), field("comments", "Review comments.", "审核意见。")},
				ReviewPolicy: "manual",
				Position:     entity.WorkflowPosition{X: 360, Y: 180},
				Config:       map[string]string{"color": "amber"},
			},
			{
				ID: "kb_update", Type: "agent_task", Title: text(locale, "Knowledge Update", "知识库更新"),
				Description:  text(locale, "Turn the approved answer into a reusable knowledge document.", "把审核后的答复沉淀成可复用知识库文档。"),
				ActorRole:    "kb-maintainer",
				InputFields:  []entity.WorkflowField{field("answer_draft_doc_id", "docID for approved answer.", "已审核答复 docID。"), field("comments", "Review comments to fold in.", "需要吸收的审核意见。")},
				OutputFields: []entity.WorkflowField{field("kb_doc_id", "docID for the reusable knowledge article.", "可复用知识库文档 docID。")},
				Position:     entity.WorkflowPosition{X: 640, Y: 180},
				Config:       map[string]string{"color": "emerald"},
			},
			{
				ID: "product_feedback", Type: "agent_task", Title: text(locale, "Product Feedback", "产品反馈"),
				Description:  text(locale, "Extract product gaps and recurring pain from the support topic.", "从客服主题中提取产品缺口和重复痛点。"),
				ActorRole:    "feedback-analyst",
				InputFields:  []entity.WorkflowField{field("kb_doc_id", "Knowledge article docID.", "知识库文档 docID。"), field("topic_summary", "Support topic summary.", "客服主题摘要。")},
				OutputFields: []entity.WorkflowField{field("feedback_doc_id", "docID for product feedback.", "产品反馈 docID。")},
				Position:     entity.WorkflowPosition{X: 920, Y: 180},
				Config:       map[string]string{"color": "violet"},
			},
		},
		Edges: []entity.WorkflowEdge{
			edge("e-triage-review", "support_triage", "answer_review", "", nil, nil, true),
			edge("e-review-kb", "answer_review", "kb_update", text(locale, "approved", "通过"), cond("decision", "eq", "approve"), map[string]string{"answer_draft_doc_id": "$input.answer_draft_doc_id", "comments": "$output.comments"}, false),
			edge("e-review-rework", "answer_review", "support_triage", text(locale, "needs changes", "需要修改"), cond("decision", "eq", "request_changes"), map[string]string{"review_comments": "$output.comments", "answer_draft_doc_id": "$input.answer_draft_doc_id"}, false),
			edge("e-kb-feedback", "kb_update", "product_feedback", "", nil, nil, true),
		},
	}
}

func cond(field, operator, value string) *entity.WorkflowEdgeCondition {
	return &entity.WorkflowEdgeCondition{Field: field, Operator: operator, Value: value}
}

func edge(id, from, to, label string, condition *entity.WorkflowEdgeCondition, inputMapping map[string]string, isDefault bool) entity.WorkflowEdge {
	return entity.WorkflowEdge{
		ID:           id,
		From:         from,
		To:           to,
		Label:        label,
		Condition:    condition,
		InputMapping: inputMapping,
		IsDefault:    isDefault,
	}
}
