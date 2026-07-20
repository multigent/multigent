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
		Name:        text(locale, "OpenSpec Artifact-Guided Loop", "OpenSpec 产物驱动闭环"),
		Description: text(locale, "The real OpenSpec operating loop: explore when unclear, propose a complete change folder, review artifacts before build, apply tasks, verify against artifacts, sync accepted specs, and archive.", "真实 OpenSpec 操作闭环：模糊时探索，propose 完整 change folder，构建前审核 artifacts，执行 tasks，按 artifacts 验证，同步已接受 specs，并归档。"),
		Version:     1,
		Locale:      normalizeLocale(locale),
		StartStepID: "explore",
		Steps: []entity.WorkflowStep{
			{
				ID: "explore", Type: "agent_task", Title: text(locale, "Explore", "探索"),
				Description: text(locale, "Use openspec-explore when the request is vague. This is a thinking stance, not an implementation phase: inspect context, clarify options, and decide whether to start a change.", "需求模糊时使用 openspec-explore。这是思考姿态，不是实现阶段：检查上下文、澄清选项，并判断是否要启动 change。"),
				ActorRole:   "openspec-change-owner",
				InputFields: []entity.WorkflowField{
					field("raw_request", "Raw idea, bug, customer request, process problem, or change intent.", "原始想法、Bug、客户请求、流程问题或变更意图。"),
					field("context_doc_id", "Optional docID for existing context, issue, notes, or current behavior.", "可选，已有上下文、Issue、纪要或当前行为的 docID。"),
				},
				OutputFields: []entity.WorkflowField{
					field("exploration_summary", "Short summary of what was learned and the recommended next move.", "探索发现和建议下一步的简短摘要。"),
					field("change_name", "Suggested kebab-case OpenSpec change name, if ready.", "如已准备好，建议的 kebab-case OpenSpec change 名称。"),
					field("decision", "propose, continue_exploring, or stop.", "propose、continue_exploring 或 stop。"),
				},
				Position: entity.WorkflowPosition{X: 80, Y: 220},
				Config:   map[string]string{"color": "sky"},
			},
			{
				ID: "propose", Type: "agent_task", Title: text(locale, "Propose Change", "创建变更产物"),
				Description: text(locale, "Use openspec-propose for the normal fast path. It creates the change folder and generates proposal.md, specs, design.md, and tasks.md until the change is apply-ready. For risky work, use openspec-new-change + openspec-continue-change instead.", "常规快速路径使用 openspec-propose。它创建 change folder，并生成 proposal.md、specs、design.md 和 tasks.md，直到 change 可执行。高风险工作改用 openspec-new-change + openspec-continue-change。"),
				ActorRole:   "openspec-change-owner",
				InputFields: []entity.WorkflowField{
					field("raw_request", "Original request or refined exploration result.", "原始请求或探索后的精炼结论。"),
					field("change_name", "Kebab-case change name.", "kebab-case change 名称。"),
					field("artifact_mode", "fast_path for openspec-propose, or stepwise for new+continue.", "fast_path 表示 openspec-propose；stepwise 表示 new+continue。"),
				},
				OutputFields: []entity.WorkflowField{
					field("change_name", "Created OpenSpec change name.", "已创建的 OpenSpec change 名称。"),
					field("proposal_doc_id", "docID or artifact reference for proposal.md.", "proposal.md 的 docID 或 artifact 引用。"),
					field("specs_doc_id", "docID or artifact reference for delta specs.", "delta specs 的 docID 或 artifact 引用。"),
					field("design_doc_id", "docID or artifact reference for design.md.", "design.md 的 docID 或 artifact 引用。"),
					field("tasks_doc_id", "docID or artifact reference for tasks.md.", "tasks.md 的 docID 或 artifact 引用。"),
					field("apply_ready", "true or false.", "true 或 false。"),
				},
				Position: entity.WorkflowPosition{X: 380, Y: 220},
				Config:   map[string]string{"color": "violet"},
			},
			{
				ID: "plan_review", Type: "human_review", Title: text(locale, "Review Plan", "审核计划"),
				Description: text(locale, "Human reviews proposal first, then specs, then tasks. The goal is to catch wrong scope while it is still Markdown, before implementation cost is paid.", "人类先审核 proposal，再审核 specs，最后审核 tasks。目标是在实现成本发生前，用 Markdown 阶段发现范围错误。"),
				ActorRole:   "request-owner",
				InputFields: []entity.WorkflowField{
					field("proposal_doc_id", "Proposal artifact reference.", "proposal artifact 引用。"),
					field("specs_doc_id", "Delta specs artifact reference.", "delta specs artifact 引用。"),
					field("tasks_doc_id", "Tasks artifact reference.", "tasks artifact 引用。"),
				},
				OutputFields: []entity.WorkflowField{
					field("decision", "approve, request_changes, or stop.", "approve、request_changes 或 stop。"),
					field("comments", "Scope, requirement, scenario, or task changes requested.", "要求修改的范围、需求、场景或任务意见。"),
				},
				ReviewPolicy: "manual",
				Position:     entity.WorkflowPosition{X: 700, Y: 220},
				Config:       map[string]string{"color": "amber"},
			},
			{
				ID: "apply", Type: "agent_task", Title: text(locale, "Apply", "执行"),
				Description: text(locale, "Use openspec-apply-change. Read contextFiles from OpenSpec instructions, implement pending tasks from tasks.md, and mark checkboxes complete as work is done.", "使用 openspec-apply-change。读取 OpenSpec instructions 返回的 contextFiles，执行 tasks.md 中的 pending tasks，并完成后勾选 checkbox。"),
				ActorRole:   "openspec-implementer",
				InputFields: []entity.WorkflowField{
					field("change_name", "OpenSpec change to implement.", "要实现的 OpenSpec change。"),
					field("tasks_doc_id", "Tasks artifact reference.", "tasks artifact 引用。"),
					field("review_comments", "Plan review comments if this is a rework pass.", "返工时的计划审核意见。"),
				},
				OutputFields: []entity.WorkflowField{
					field("implementation_summary", "Summary of tasks completed this pass.", "本轮完成任务摘要。"),
					field("progress", "N/M task progress from tasks.md.", "tasks.md 中 N/M 的进度。"),
					field("change_artifact_ref", "PR, patch, deployment, document, or other delivery reference.", "PR、补丁、部署、文档或其他交付引用。"),
					field("blocked_reason", "Reason if implementation paused.", "如果实现暂停，说明原因。"),
				},
				Position: entity.WorkflowPosition{X: 1020, Y: 220},
				Config:   map[string]string{"color": "cyan"},
			},
			{
				ID: "verify", Type: "agent_task", Title: text(locale, "Verify", "验证"),
				Description: text(locale, "Use openspec-verify-change to compare implementation with tasks, specs, and design across completeness, correctness, and coherence.", "使用 openspec-verify-change，从 completeness、correctness、coherence 三个维度按 tasks、specs 和 design 验证实现。"),
				ActorRole:   "openspec-reviewer",
				InputFields: []entity.WorkflowField{
					field("change_name", "OpenSpec change to verify.", "要验证的 OpenSpec change。"),
					field("change_artifact_ref", "Implementation artifact reference.", "实现产物引用。"),
				},
				OutputFields: []entity.WorkflowField{
					field("verification_report_doc_id", "docID or artifact reference for verification report.", "验证报告 docID 或 artifact 引用。"),
					field("decision", "pass, warning, or fail.", "pass、warning 或 fail。"),
					field("critical_issues", "Critical issues that block archive.", "阻塞归档的 critical issues。"),
				},
				Position: entity.WorkflowPosition{X: 1340, Y: 220},
				Config:   map[string]string{"color": "orange"},
			},
			{
				ID: "sync", Type: "agent_task", Title: text(locale, "Sync Specs", "同步 Specs"),
				Description: text(locale, "Use openspec-sync-specs when accepted delta specs should be merged into main specs before or during archive.", "当已接受 delta specs 需要合并到 main specs 时，使用 openspec-sync-specs。"),
				ActorRole:   "openspec-reviewer",
				InputFields: []entity.WorkflowField{
					field("change_name", "OpenSpec change whose delta specs should sync.", "需要同步 delta specs 的 OpenSpec change。"),
					field("verification_report_doc_id", "Verification report reference.", "验证报告引用。"),
				},
				OutputFields: []entity.WorkflowField{
					field("sync_summary", "Capabilities and requirements updated in main specs.", "main specs 中更新的 capabilities 和 requirements。"),
					field("sync_decision", "synced, skipped, or no_delta_specs.", "synced、skipped 或 no_delta_specs。"),
				},
				Position: entity.WorkflowPosition{X: 1660, Y: 220},
				Config:   map[string]string{"color": "blue"},
			},
			{
				ID: "archive", Type: "agent_task", Title: text(locale, "Archive", "归档"),
				Description: text(locale, "Use openspec-archive-change to move the completed change into archive with its full context preserved. Use bulk archive only for multiple completed changes.", "使用 openspec-archive-change 把完成的 change 移入 archive 并保留完整上下文。只有多个完成 changes 时才使用 bulk archive。"),
				ActorRole:   "openspec-reviewer",
				InputFields: []entity.WorkflowField{
					field("change_name", "OpenSpec change to archive.", "要归档的 OpenSpec change。"),
					field("sync_summary", "Spec sync summary if any.", "如有，spec 同步摘要。"),
				},
				OutputFields: []entity.WorkflowField{
					field("archive_ref", "Archive reference or docID.", "归档引用或 docID。"),
					field("final_summary", "What changed, what shipped, and what future agents should know.", "发生了什么变更、交付了什么、后续 Agent 应知道什么。"),
				},
				Position: entity.WorkflowPosition{X: 1980, Y: 220},
				Config:   map[string]string{"color": "emerald"},
			},
		},
		Edges: []entity.WorkflowEdge{
			edge("e-explore-propose", "explore", "propose", text(locale, "ready", "可创建"), cond("decision", "eq", "propose"), nil, false),
			edge("e-propose-review", "propose", "plan_review", "", nil, nil, true),
			edge("e-review-apply", "plan_review", "apply", text(locale, "approved", "通过"), cond("decision", "eq", "approve"), map[string]string{"change_name": "$input.change_name", "tasks_doc_id": "$input.tasks_doc_id", "review_comments": "$output.comments"}, false),
			edge("e-review-rework", "plan_review", "propose", text(locale, "needs artifact changes", "需要修改产物"), cond("decision", "eq", "request_changes"), map[string]string{"change_name": "$input.change_name", "review_comments": "$output.comments"}, false),
			edge("e-apply-verify", "apply", "verify", "", nil, nil, true),
			edge("e-verify-sync", "verify", "sync", text(locale, "passed", "通过"), cond("decision", "eq", "pass"), nil, false),
			edge("e-verify-warning-sync", "verify", "sync", text(locale, "warnings accepted", "接受警告"), cond("decision", "eq", "warning"), nil, false),
			edge("e-verify-rework", "verify", "apply", text(locale, "failed", "失败"), cond("decision", "eq", "fail"), map[string]string{"review_comments": "$output.critical_issues", "verification_report_doc_id": "$output.verification_report_doc_id"}, false),
			edge("e-sync-archive", "sync", "archive", "", nil, nil, true),
		},
	}
}

func videoProductionStudioWorkflow(locale string) entity.WorkflowTemplate {
	field := func(name, en, zh string) entity.WorkflowField {
		return entity.WorkflowField{Name: name, Description: text(locale, en, zh)}
	}
	return entity.WorkflowTemplate{
		ID:          "video-production-studio",
		Name:        text(locale, "Video Production Studio", "视频制作工作室"),
		Description: text(locale, "A creative production workflow from brief to publish package, with structured artifacts and human gates for concept, script, storyboard, assets, and final QA.", "从需求简报到发布包的创意制作流程，使用结构化产物，并在创意、脚本、分镜、素材和最终 QA 设置人工门禁。"),
		Version:     1,
		Locale:      normalizeLocale(locale),
		StartStepID: "intake",
		Steps: []entity.WorkflowStep{
			{
				ID: "intake", Type: "agent_task", Title: text(locale, "Creative Intake", "创意需求收集"),
				Description: text(locale, "Clarify goal, audience, channel, duration, constraints, claims, brand rules, and approval policy. Long details must be written to docs and returned as docIDs.", "澄清目标、受众、渠道、时长、约束、事实陈述、品牌规则和审核策略。长内容必须写入知识库并返回 docID。"),
				ActorRole:   "video-producer",
				InputFields: []entity.WorkflowField{
					field("raw_request", "Raw video request, campaign goal, reference, or stakeholder note.", "原始视频需求、活动目标、参考或干系人说明。"),
					field("brand_context_doc_id", "Optional docID for brand, tone, legal, or product context.", "可选，品牌、语气、法务或产品上下文 docID。"),
				},
				OutputFields: []entity.WorkflowField{
					field("brief_doc_id", "docID for the production brief.", "制作 brief 的 docID。"),
					field("audience", "Target audience and situation.", "目标受众和具体场景。"),
					field("delivery_shape", "Format, duration, channel, aspect ratio, and deadline.", "格式、时长、渠道、画幅和截止时间。"),
					field("approval_policy", "Which gates require human approval.", "哪些节点需要人工审核。"),
				},
				Position: entity.WorkflowPosition{X: 80, Y: 220},
				Config:   map[string]string{"color": "sky"},
			},
			{
				ID: "reference", Type: "agent_task", Title: text(locale, "Reference Analysis", "参考分析"),
				Description: text(locale, "Analyze references or market examples. Extract reusable structure and taste notes without copying protected expression.", "分析参考或市场案例，提取可复用结构和审美说明，但不复制受保护表达。"),
				ActorRole:   "creative-director",
				InputFields: []entity.WorkflowField{
					field("brief_doc_id", "Production brief docID.", "制作 brief docID。"),
					field("reference_links", "Reference URLs, uploaded files, or example descriptions.", "参考链接、上传文件或案例描述。"),
				},
				OutputFields: []entity.WorkflowField{
					field("reference_analysis_doc_id", "docID for structure, pacing, visual language, and risks found in references.", "参考的结构、节奏、视觉语言和风险分析 docID。"),
					field("reuse_principles", "What may be reused as abstract principles.", "可作为抽象原则复用的内容。"),
					field("avoid_copying_notes", "What must not be copied.", "必须避免复制的内容。"),
				},
				Position: entity.WorkflowPosition{X: 360, Y: 220},
				Config:   map[string]string{"color": "violet"},
			},
			{
				ID: "proposal", Type: "agent_task", Title: text(locale, "Concept Proposal", "创意提案"),
				Description: text(locale, "Create differentiated concepts, production paths, rough cost, tool needs, risks, and recommendation.", "产出差异化创意方案、制作路径、粗略成本、工具需求、风险和推荐选择。"),
				ActorRole:   "creative-director",
				InputFields: []entity.WorkflowField{
					field("brief_doc_id", "Production brief docID.", "制作 brief docID。"),
					field("reference_analysis_doc_id", "Reference analysis docID.", "参考分析 docID。"),
				},
				OutputFields: []entity.WorkflowField{
					field("proposal_doc_id", "docID for 2-3 concept options and recommendation.", "2-3 个创意方案和推荐选择的 docID。"),
					field("recommended_concept", "Selected recommendation and rationale.", "推荐方案和理由。"),
					field("tool_plan_doc_id", "docID for required providers, credentials, cost range, and fallback path.", "所需 provider、凭证、成本区间和兜底路径 docID。"),
					field("decision", "ready_for_review or blocked.", "ready_for_review 或 blocked。"),
				},
				Position: entity.WorkflowPosition{X: 640, Y: 220},
				Config:   map[string]string{"color": "purple"},
			},
			{
				ID: "proposal_review", Type: "human_review", Title: text(locale, "Proposal Review", "提案审核"),
				Description: text(locale, "Human approves one concept, requests changes, or stops before expensive generation starts.", "人类在昂贵生成开始前批准一个方案、要求修改或停止。"),
				ActorRole:   "request-owner",
				InputFields: []entity.WorkflowField{
					field("proposal_doc_id", "Proposal docID.", "提案 docID。"),
					field("tool_plan_doc_id", "Tool plan docID.", "工具方案 docID。"),
				},
				OutputFields: []entity.WorkflowField{
					field("decision", "approve, request_changes, or stop.", "approve、request_changes 或 stop。"),
					field("approved_concept", "Approved concept ID or description.", "已批准方案 ID 或描述。"),
					field("comments", "Creative constraints, budget limits, or changes.", "创意约束、预算限制或修改意见。"),
				},
				ReviewPolicy: "manual",
				Position:     entity.WorkflowPosition{X: 920, Y: 220},
				Config:       map[string]string{"color": "amber"},
			},
			{
				ID: "script", Type: "agent_task", Title: text(locale, "Write Script", "编写脚本"),
				Description: text(locale, "Write timed script, narration, on-screen text, key beats, and factual claim checklist.", "编写带时长的脚本、旁白、屏幕文字、关键节拍和事实核查清单。"),
				ActorRole:   "script-writer",
				InputFields: []entity.WorkflowField{
					field("brief_doc_id", "Production brief docID.", "制作 brief docID。"),
					field("proposal_doc_id", "Approved proposal docID.", "已审核提案 docID。"),
					field("approved_concept", "Approved concept.", "已批准创意方案。"),
					field("comments", "Proposal review comments.", "提案审核意见。"),
				},
				OutputFields: []entity.WorkflowField{
					field("script_doc_id", "docID for full timed script.", "完整带时长脚本 docID。"),
					field("claim_check_doc_id", "docID for factual claims and required verification.", "事实陈述和所需核查 docID。"),
					field("estimated_duration", "Estimated video duration.", "预计视频时长。"),
				},
				Position: entity.WorkflowPosition{X: 1200, Y: 220},
				Config:   map[string]string{"color": "emerald"},
			},
			{
				ID: "script_review", Type: "human_review", Title: text(locale, "Script Review", "脚本审核"),
				Description: text(locale, "Human reviews message, claim safety, tone, pacing, and whether the script matches the approved concept.", "人类审核信息、事实安全、语气、节奏，以及脚本是否匹配已批准创意。"),
				ActorRole:   "request-owner",
				InputFields: []entity.WorkflowField{
					field("script_doc_id", "Script docID.", "脚本 docID。"),
					field("claim_check_doc_id", "Claim check docID.", "事实核查 docID。"),
				},
				OutputFields: []entity.WorkflowField{
					field("decision", "approve, request_changes, or stop.", "approve、request_changes 或 stop。"),
					field("comments", "Script edits or constraints.", "脚本修改意见或约束。"),
				},
				ReviewPolicy: "manual",
				Position:     entity.WorkflowPosition{X: 1480, Y: 220},
				Config:       map[string]string{"color": "amber"},
			},
			{
				ID: "storyboard", Type: "agent_task", Title: text(locale, "Storyboard", "分镜"),
				Description: text(locale, "Turn script into scene plan, shot intent, visual treatment, asset requirements, and storyboard acceptance criteria.", "把脚本转成场景计划、镜头意图、视觉处理、素材需求和分镜验收标准。"),
				ActorRole:   "storyboard-planner",
				InputFields: []entity.WorkflowField{
					field("script_doc_id", "Approved script docID.", "已审核脚本 docID。"),
					field("comments", "Script review comments.", "脚本审核意见。"),
				},
				OutputFields: []entity.WorkflowField{
					field("storyboard_doc_id", "docID for storyboard and scene list.", "分镜和场景列表 docID。"),
					field("asset_requirements_doc_id", "docID for required assets by scene.", "按场景列出的素材需求 docID。"),
					field("visual_acceptance_doc_id", "docID for visual acceptance criteria.", "视觉验收标准 docID。"),
				},
				Position: entity.WorkflowPosition{X: 1760, Y: 220},
				Config:   map[string]string{"color": "blue"},
			},
			{
				ID: "storyboard_review", Type: "human_review", Title: text(locale, "Storyboard Review", "分镜审核"),
				Description: text(locale, "Human approves scene direction before asset generation starts.", "人类在素材生成前审核场景方向。"),
				ActorRole:   "request-owner",
				InputFields: []entity.WorkflowField{
					field("storyboard_doc_id", "Storyboard docID.", "分镜 docID。"),
					field("asset_requirements_doc_id", "Asset requirements docID.", "素材需求 docID。"),
				},
				OutputFields: []entity.WorkflowField{
					field("decision", "approve, request_changes, or stop.", "approve、request_changes 或 stop。"),
					field("comments", "Storyboard changes, scene removals, or asset constraints.", "分镜修改、场景删除或素材约束。"),
				},
				ReviewPolicy: "manual",
				Position:     entity.WorkflowPosition{X: 2040, Y: 220},
				Config:       map[string]string{"color": "amber"},
			},
			{
				ID: "assets", Type: "agent_task", Title: text(locale, "Produce Assets", "制作素材"),
				Description: text(locale, "Generate or collect assets, write asset manifest with source/provenance/cost, and prepare review contact sheet.", "生成或收集素材，编写包含来源/溯源/成本的素材清单，并准备审核用 contact sheet。"),
				ActorRole:   "asset-director",
				InputFields: []entity.WorkflowField{
					field("storyboard_doc_id", "Approved storyboard docID.", "已审核分镜 docID。"),
					field("asset_requirements_doc_id", "Asset requirements docID.", "素材需求 docID。"),
					field("comments", "Storyboard review comments.", "分镜审核意见。"),
				},
				OutputFields: []entity.WorkflowField{
					field("asset_manifest_doc_id", "docID for asset manifest with provenance, prompts, cost, and status.", "包含溯源、提示词、成本和状态的素材清单 docID。"),
					field("asset_review_doc_id", "docID for contact sheet or asset review package.", "contact sheet 或素材审核包 docID。"),
					field("cost_snapshot", "Current spend or estimated spend.", "当前花费或估算花费。"),
				},
				Position: entity.WorkflowPosition{X: 2320, Y: 220},
				Config:   map[string]string{"color": "cyan"},
			},
			{
				ID: "asset_review", Type: "human_review", Title: text(locale, "Asset Review", "素材审核"),
				Description: text(locale, "Human approves assets before editing and composition. Bad assets should be caught here, not after final render.", "人类在剪辑合成前审核素材。坏素材应该在这里发现，而不是最终渲染后才发现。"),
				ActorRole:   "request-owner",
				InputFields: []entity.WorkflowField{
					field("asset_manifest_doc_id", "Asset manifest docID.", "素材清单 docID。"),
					field("asset_review_doc_id", "Asset review package docID.", "素材审核包 docID。"),
				},
				OutputFields: []entity.WorkflowField{
					field("decision", "approve, request_changes, or stop.", "approve、request_changes 或 stop。"),
					field("comments", "Asset replacement, style, or quality feedback.", "素材替换、风格或质量意见。"),
				},
				ReviewPolicy: "manual",
				Position:     entity.WorkflowPosition{X: 2600, Y: 220},
				Config:       map[string]string{"color": "amber"},
			},
			{
				ID: "edit_plan", Type: "agent_task", Title: text(locale, "Edit Plan", "剪辑方案"),
				Description: text(locale, "Create timeline, cut decisions, subtitles, music, transitions, and render instructions.", "创建时间线、剪辑决策、字幕、音乐、转场和渲染说明。"),
				ActorRole:   "editor-composer",
				InputFields: []entity.WorkflowField{
					field("script_doc_id", "Script docID.", "脚本 docID。"),
					field("storyboard_doc_id", "Storyboard docID.", "分镜 docID。"),
					field("asset_manifest_doc_id", "Approved asset manifest docID.", "已审核素材清单 docID。"),
				},
				OutputFields: []entity.WorkflowField{
					field("edit_decisions_doc_id", "docID for timeline and edit decisions.", "时间线和剪辑决策 docID。"),
					field("render_plan_doc_id", "docID for render settings and validation plan.", "渲染设置和校验方案 docID。"),
				},
				Position: entity.WorkflowPosition{X: 2880, Y: 220},
				Config:   map[string]string{"color": "slate"},
			},
			{
				ID: "compose", Type: "agent_task", Title: text(locale, "Compose / Render", "合成渲染"),
				Description: text(locale, "Render the approved edit plan and produce technical validation evidence.", "按已审核剪辑方案渲染成片，并产出技术校验证据。"),
				ActorRole:   "editor-composer",
				InputFields: []entity.WorkflowField{
					field("edit_decisions_doc_id", "Edit decisions docID.", "剪辑决策 docID。"),
					field("render_plan_doc_id", "Render plan docID.", "渲染方案 docID。"),
				},
				OutputFields: []entity.WorkflowField{
					field("render_ref", "Final render or draft render reference.", "最终或草稿成片引用。"),
					field("render_report_doc_id", "docID for ffprobe, audio, subtitle, duration, and visual QA evidence.", "ffprobe、音频、字幕、时长和视觉 QA 证据 docID。"),
					field("known_issues", "Known issues or empty string.", "已知问题或空字符串。"),
				},
				Position: entity.WorkflowPosition{X: 3160, Y: 220},
				Config:   map[string]string{"color": "indigo"},
			},
			{
				ID: "qa_review", Type: "agent_task", Title: text(locale, "Production QA", "制作 QA"),
				Description: text(locale, "Review final output against brief, script, storyboard, asset approvals, audio/subtitle quality, brand fit, and delivery promise.", "按 brief、脚本、分镜、素材审核、音频/字幕质量、品牌一致性和交付承诺审核最终输出。"),
				ActorRole:   "production-reviewer",
				InputFields: []entity.WorkflowField{
					field("render_ref", "Render reference.", "成片引用。"),
					field("render_report_doc_id", "Render report docID.", "渲染报告 docID。"),
					field("brief_doc_id", "Brief docID.", "brief docID。"),
				},
				OutputFields: []entity.WorkflowField{
					field("qa_report_doc_id", "docID for final QA report.", "最终 QA 报告 docID。"),
					field("decision", "pass, request_changes, or stop.", "pass、request_changes 或 stop。"),
					field("comments", "Final production notes or blocking issues.", "最终制作意见或阻塞问题。"),
				},
				Position: entity.WorkflowPosition{X: 3440, Y: 220},
				Config:   map[string]string{"color": "orange"},
			},
			{
				ID: "publish_package", Type: "agent_task", Title: text(locale, "Publish Package", "发布包"),
				Description: text(locale, "Prepare title, description, platform copy, thumbnail notes, version summary, reuse notes, and archive handoff.", "准备标题、描述、平台文案、封面说明、版本总结、复用说明和归档交接。"),
				ActorRole:   "video-producer",
				InputFields: []entity.WorkflowField{
					field("render_ref", "Approved render reference.", "已通过审核的成片引用。"),
					field("qa_report_doc_id", "Final QA report docID.", "最终 QA 报告 docID。"),
				},
				OutputFields: []entity.WorkflowField{
					field("publish_package_doc_id", "docID for publish copy, platform notes, and final handoff.", "发布文案、平台说明和最终交接 docID。"),
					field("archive_summary_doc_id", "docID for reusable production learnings and assets.", "可复用制作经验和素材归档 docID。"),
					field("completion_decision", "ready_to_publish, published, or follow_up_needed.", "ready_to_publish、published 或 follow_up_needed。"),
				},
				Position: entity.WorkflowPosition{X: 3720, Y: 220},
				Config:   map[string]string{"color": "emerald"},
			},
		},
		Edges: []entity.WorkflowEdge{
			edge("e-intake-reference", "intake", "reference", "", nil, nil, true),
			edge("e-reference-proposal", "reference", "proposal", "", nil, nil, true),
			edge("e-proposal-review", "proposal", "proposal_review", text(locale, "ready", "待审核"), cond("decision", "eq", "ready_for_review"), nil, false),
			edge("e-proposal-script", "proposal_review", "script", text(locale, "approved", "通过"), cond("decision", "eq", "approve"), map[string]string{"approved_concept": "$output.approved_concept", "comments": "$output.comments"}, false),
			edge("e-proposal-rework", "proposal_review", "proposal", text(locale, "needs changes", "需要修改"), cond("decision", "eq", "request_changes"), map[string]string{"review_comments": "$output.comments"}, false),
			edge("e-script-review", "script", "script_review", "", nil, nil, true),
			edge("e-script-storyboard", "script_review", "storyboard", text(locale, "approved", "通过"), cond("decision", "eq", "approve"), map[string]string{"comments": "$output.comments"}, false),
			edge("e-script-rework", "script_review", "script", text(locale, "needs changes", "需要修改"), cond("decision", "eq", "request_changes"), map[string]string{"review_comments": "$output.comments"}, false),
			edge("e-storyboard-review", "storyboard", "storyboard_review", "", nil, nil, true),
			edge("e-storyboard-assets", "storyboard_review", "assets", text(locale, "approved", "通过"), cond("decision", "eq", "approve"), map[string]string{"comments": "$output.comments"}, false),
			edge("e-storyboard-rework", "storyboard_review", "storyboard", text(locale, "needs changes", "需要修改"), cond("decision", "eq", "request_changes"), map[string]string{"review_comments": "$output.comments"}, false),
			edge("e-assets-review", "assets", "asset_review", "", nil, nil, true),
			edge("e-assets-edit", "asset_review", "edit_plan", text(locale, "approved", "通过"), cond("decision", "eq", "approve"), map[string]string{"comments": "$output.comments"}, false),
			edge("e-assets-rework", "asset_review", "assets", text(locale, "needs changes", "需要修改"), cond("decision", "eq", "request_changes"), map[string]string{"review_comments": "$output.comments"}, false),
			edge("e-edit-compose", "edit_plan", "compose", "", nil, nil, true),
			edge("e-compose-qa", "compose", "qa_review", "", nil, nil, true),
			edge("e-qa-publish", "qa_review", "publish_package", text(locale, "passed", "通过"), cond("decision", "eq", "pass"), nil, false),
			edge("e-qa-rework", "qa_review", "edit_plan", text(locale, "needs changes", "需要修改"), cond("decision", "eq", "request_changes"), map[string]string{"review_comments": "$output.comments", "qa_report_doc_id": "$output.qa_report_doc_id"}, false),
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
