package playbook

import "github.com/multigent/multigent/internal/entity"

func startupValidationWorkflow(locale string) entity.WorkflowTemplate {
	field := func(name, en, zh string) entity.WorkflowField {
		return entity.WorkflowField{Name: name, Description: text(locale, en, zh)}
	}
	return entity.WorkflowTemplate{
		ID:          "startup-idea-validation",
		Name:        text(locale, "Startup Idea Validation", "创业想法验证"),
		Description: text(locale, "Validate an idea before building: clarify the raw idea, collect market evidence, review demand strength, and define a prototype scope.", "开发前验证想法：澄清原始想法、收集市场证据、审核需求强度，并定义原型范围。"),
		Version:     1,
		Locale:      normalizeLocale(locale),
		StartStepID: "idea_intake",
		Steps: []entity.WorkflowStep{
			{
				ID: "idea_intake", Type: "agent_task", Title: text(locale, "Idea Intake", "想法澄清"),
				Description:  text(locale, "Turn the raw idea into a precise problem, target user, current workaround, and riskiest premise.", "把原始想法整理成明确问题、目标用户、当前替代方案和最大风险假设。"),
				ActorRole:    "yc-office-hours-advisor",
				InputFields:  []entity.WorkflowField{field("raw_idea", "Raw idea or founder note.", "原始想法或创始人描述。")},
				OutputFields: []entity.WorkflowField{field("idea_brief_doc_id", "docID for the idea brief.", "想法简报 docID。"), field("risky_premise", "The riskiest premise to validate first.", "最需要优先验证的风险假设。")},
				Position:     entity.WorkflowPosition{X: 80, Y: 180},
				Config:       map[string]string{"color": "sky"},
			},
			{
				ID: "market_evidence", Type: "agent_task", Title: text(locale, "Market Evidence", "市场证据"),
				Description:  text(locale, "Collect evidence about existing alternatives, urgency, budget, switching pressure, and first reachable users.", "收集现有替代方案、紧迫性、预算、迁移动力和第一批可触达用户的证据。"),
				ActorRole:    "market-research-agent",
				InputFields:  []entity.WorkflowField{field("idea_brief_doc_id", "docID for the idea brief.", "想法简报 docID。"), field("risky_premise", "Riskiest premise to validate.", "需要验证的最大风险假设。")},
				OutputFields: []entity.WorkflowField{field("market_evidence_doc_id", "docID for market evidence.", "市场证据 docID。"), field("status_quo_summary", "Summary of current user workaround.", "用户当前替代方案摘要。")},
				Position:     entity.WorkflowPosition{X: 360, Y: 180},
				Config:       map[string]string{"color": "violet"},
			},
			{
				ID: "validation_review", Type: "human_review", Title: text(locale, "Validation Review", "验证审核"),
				Description:  text(locale, "Human reviews whether the evidence is strong enough to continue, weak enough to stop, or unclear enough to research more.", "人类审核证据是否足够继续、应该停止，还是需要继续调研。"),
				ActorRole:    "founder",
				InputFields:  []entity.WorkflowField{field("market_evidence_doc_id", "docID for market evidence.", "市场证据 docID。")},
				OutputFields: []entity.WorkflowField{field("decision", "continue, request_changes, or stop.", "continue、request_changes 或 stop。"), field("comments", "Review comments and founder judgment.", "审核意见和创始人判断。")},
				ReviewPolicy: "manual",
				Position:     entity.WorkflowPosition{X: 640, Y: 180},
				Config:       map[string]string{"color": "amber"},
			},
			{
				ID: "prototype_decision", Type: "agent_task", Title: text(locale, "Prototype Scope", "原型范围"),
				Description:  text(locale, "Define the smallest prototype that validates the riskiest premise within 48 hours.", "定义 48 小时内验证最大风险假设的最小原型范围。"),
				ActorRole:    "prototype-scope-reviewer",
				InputFields:  []entity.WorkflowField{field("market_evidence_doc_id", "docID for market evidence.", "市场证据 docID。"), field("comments", "Founder review comments.", "创始人审核意见。")},
				OutputFields: []entity.WorkflowField{field("prototype_plan_doc_id", "docID for 48-hour prototype plan.", "48 小时原型计划 docID。"), field("success_metric", "Primary metric the prototype should validate.", "原型要验证的核心指标。")},
				Position:     entity.WorkflowPosition{X: 920, Y: 180},
				Config:       map[string]string{"color": "emerald"},
			},
		},
		Edges: []entity.WorkflowEdge{
			edge("e-idea-market", "idea_intake", "market_evidence", "", nil, nil, true),
			edge("e-market-review", "market_evidence", "validation_review", "", nil, nil, true),
			edge("e-review-continue", "validation_review", "prototype_decision", text(locale, "continue", "继续"), cond("decision", "eq", "continue"), map[string]string{"market_evidence_doc_id": "$input.market_evidence_doc_id", "comments": "$output.comments"}, false),
			edge("e-review-rework", "validation_review", "market_evidence", text(locale, "more evidence", "补充证据"), cond("decision", "eq", "request_changes"), map[string]string{"review_comments": "$output.comments", "market_evidence_doc_id": "$input.market_evidence_doc_id"}, false),
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
