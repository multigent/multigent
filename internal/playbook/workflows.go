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
