package workflow

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/errs"
)

type Store struct {
	db          controldb.Store
	workspaceID string
}

func NewStore(db controldb.Store, workspaceID string) *Store {
	return &Store{db: db, workspaceID: workspaceID}
}

func Templates(locale string) []entity.WorkflowTemplate {
	return []entity.WorkflowTemplate{softwareDeliveryTemplate(locale)}
}

func Template(id, locale string) (entity.WorkflowTemplate, bool) {
	for _, tmpl := range Templates(locale) {
		if tmpl.ID == id {
			return tmpl, true
		}
	}
	return entity.WorkflowTemplate{}, false
}

func DefinitionFromTemplate(templateID, locale, name string) (entity.WorkflowDefinition, bool) {
	tmpl, ok := Template(templateID, locale)
	if !ok {
		return entity.WorkflowDefinition{}, false
	}
	now := time.Now().UTC()
	def := entity.WorkflowDefinition{
		ID:          entity.NewWorkflowID(),
		Name:        strings.TrimSpace(name),
		Description: tmpl.Description,
		Version:     1,
		Scope:       "workspace",
		StartStepID: tmpl.StartStepID,
		Steps:       tmpl.Steps,
		Edges:       tmpl.Edges,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if def.Name == "" {
		def.Name = tmpl.Name
	}
	return def, true
}

func (s *Store) SeedDefaults() error {
	if def, ok, err := s.Definition("software-delivery-v1"); err != nil {
		return err
	} else if ok && def.Scope == "workspace" && def.Project == "" && def.Version >= 4 && def.StartStepID == "requirement_draft" {
		return nil
	}
	now := time.Now().UTC()
	def := entity.WorkflowDefinition{
		ID:          "software-delivery-v1",
		Name:        "Agentic Software Delivery",
		Description: "A configurable human-agent delivery workflow. Agents draft artifacts, humans review only the decision gates, and rejected outputs loop back with structured feedback.",
		Version:     4,
		Scope:       "workspace",
		StartStepID: "requirement_draft",
		CreatedAt:   now,
		UpdatedAt:   now,
		Steps: []entity.WorkflowStep{
			{
				ID: "requirement_draft", Type: "agent_task", Title: "Requirement Draft",
				Description:  "An agent turns an incoming request into a structured understanding: problem, goal, scope, non-goals, risks, and open questions.",
				ActorRole:    "pm-agent",
				InputFields:  []entity.WorkflowField{{Name: "request", Description: "Original user, customer, founder, or internal request."}, {Name: "context", Description: "Known background, links, meeting notes, or existing discussions."}},
				OutputFields: []entity.WorkflowField{{Name: "requirement_draft", Description: "Structured requirement draft."}, {Name: "open_questions", Description: "Questions that still need human or stakeholder clarification."}},
				Position:     entity.WorkflowPosition{X: 80, Y: 180},
				Config:       map[string]string{"color": "sky"},
			},
			{
				ID: "requirement_review", Type: "human_review", Title: "Requirement Review",
				Description:  "A human reviews whether the requirement draft expresses the real problem and whether more clarification is needed.",
				ActorRole:    "product-owner",
				InputFields:  []entity.WorkflowField{{Name: "requirement_draft", Description: "Draft produced by the PM agent."}},
				OutputFields: []entity.WorkflowField{{Name: "decision", Description: "approve, request_changes, or need_discussion."}, {Name: "comments", Description: "Review comments and clarification notes."}},
				ReviewPolicy: "manual",
				Position:     entity.WorkflowPosition{X: 360, Y: 180},
				Config:       map[string]string{"color": "amber"},
			},
			{
				ID: "prd_draft", Type: "agent_task", Title: "PRD Draft",
				Description:  "The PM agent produces the product spec, acceptance criteria, release scope, and non-goals.",
				ActorRole:    "pm-agent",
				InputFields:  []entity.WorkflowField{{Name: "approved_requirement", Description: "Reviewed requirement with comments folded in."}},
				OutputFields: []entity.WorkflowField{{Name: "prd", Description: "Product requirements document or spec."}, {Name: "acceptance_criteria", Description: "Observable acceptance criteria."}},
				Position:     entity.WorkflowPosition{X: 640, Y: 180},
				Config:       map[string]string{"color": "sky"},
			},
			{
				ID: "prd_review", Type: "human_review", Title: "PRD Review",
				Description:  "Product and engineering stakeholders review scope, non-goals, and acceptance criteria.",
				ActorRole:    "product-owner",
				InputFields:  []entity.WorkflowField{{Name: "prd", Description: "PRD draft to review."}},
				OutputFields: []entity.WorkflowField{{Name: "decision", Description: "approve or request_changes."}, {Name: "comments", Description: "Review comments."}, {Name: "approved_prd", Description: "Final PRD when approved."}},
				ReviewPolicy: "manual",
				Position:     entity.WorkflowPosition{X: 920, Y: 180},
				Config:       map[string]string{"color": "amber"},
			},
			{
				ID: "tech_spec_draft", Type: "agent_task", Title: "Technical Spec Draft",
				Description:  "Engineering agents inspect the codebase and produce implementation plan, affected surfaces, test strategy, and task split recommendation.",
				ActorRole:    "engineering-agent",
				InputFields:  []entity.WorkflowField{{Name: "approved_prd", Description: "Reviewed product spec."}},
				OutputFields: []entity.WorkflowField{{Name: "technical_spec", Description: "Implementation plan and technical decisions."}, {Name: "task_split", Description: "Optional child task split for parallel work."}},
				Position:     entity.WorkflowPosition{X: 1200, Y: 180},
				Config:       map[string]string{"color": "violet"},
			},
			{
				ID: "tech_spec_review", Type: "human_review", Title: "Technical Spec Review",
				Description:  "Responsible engineers review the plan before implementation starts.",
				ActorRole:    "tech-lead",
				InputFields:  []entity.WorkflowField{{Name: "technical_spec", Description: "Technical plan to review."}},
				OutputFields: []entity.WorkflowField{{Name: "decision", Description: "approve or request_changes."}, {Name: "comments", Description: "Review comments."}, {Name: "approved_technical_spec", Description: "Final technical spec when approved."}},
				ReviewPolicy: "manual",
				Position:     entity.WorkflowPosition{X: 1480, Y: 180},
				Config:       map[string]string{"color": "amber"},
			},
			{
				ID: "implementation", Type: "agent_task", Title: "Implementation",
				Description:  "Development agents implement the approved technical plan and produce code changes, tests, and a PR or patch summary.",
				ActorRole:    "developer-agent",
				InputFields:  []entity.WorkflowField{{Name: "approved_technical_spec", Description: "Approved implementation plan."}},
				OutputFields: []entity.WorkflowField{{Name: "pr", Description: "Pull request, patch, or change summary."}, {Name: "tests_run", Description: "Tests executed by the agent."}, {Name: "risks", Description: "Known risks or manual checks needed."}},
				Position:     entity.WorkflowPosition{X: 1760, Y: 180},
				Config:       map[string]string{"color": "emerald"},
			},
			{
				ID: "code_review", Type: "human_review", Title: "Code Review",
				Description:  "The responsible human reviews code quality, risk, and whether the output matches the approved spec.",
				ActorRole:    "owner-engineer",
				InputFields:  []entity.WorkflowField{{Name: "pr", Description: "PR or patch to review."}},
				OutputFields: []entity.WorkflowField{{Name: "decision", Description: "approve or request_changes."}, {Name: "comments", Description: "Code review comments."}, {Name: "approved_change", Description: "Approved code artifact."}},
				ReviewPolicy: "manual",
				Position:     entity.WorkflowPosition{X: 2040, Y: 180},
				Config:       map[string]string{"color": "amber"},
			},
			{
				ID: "qa", Type: "agent_task", Title: "QA Test",
				Description:  "QA agents generate and execute test cases, then identify remaining manual test needs.",
				ActorRole:    "qa-agent",
				InputFields:  []entity.WorkflowField{{Name: "approved_change", Description: "Code artifact approved for testing."}},
				OutputFields: []entity.WorkflowField{{Name: "test_cases", Description: "Test cases."}, {Name: "test_report", Description: "Automated and manual test result summary."}},
				Position:     entity.WorkflowPosition{X: 2320, Y: 180},
				Config:       map[string]string{"color": "rose"},
			},
			{
				ID: "qa_review", Type: "human_review", Title: "QA Review",
				Description:  "Human QA or owner reviews the test report and decides whether release can proceed.",
				ActorRole:    "qa-owner",
				InputFields:  []entity.WorkflowField{{Name: "test_report", Description: "Test report to review."}},
				OutputFields: []entity.WorkflowField{{Name: "decision", Description: "approve or request_changes."}, {Name: "comments", Description: "QA feedback."}, {Name: "release_candidate", Description: "Approved release candidate."}},
				ReviewPolicy: "manual",
				Position:     entity.WorkflowPosition{X: 2600, Y: 180},
				Config:       map[string]string{"color": "amber"},
			},
			{
				ID: "release", Type: "agent_task", Title: "Release and Observe",
				Description:  "Release agents prepare rollout notes, execute allowed release steps, and check post-release signals.",
				ActorRole:    "release-agent",
				InputFields:  []entity.WorkflowField{{Name: "release_candidate", Description: "Approved release candidate."}},
				OutputFields: []entity.WorkflowField{{Name: "release_report", Description: "Release result, monitoring checks, and follow-up items."}},
				Position:     entity.WorkflowPosition{X: 2880, Y: 180},
				Config:       map[string]string{"color": "emerald"},
			},
		},
		Edges: []entity.WorkflowEdge{
			edge("e-req-to-review", "requirement_draft", "requirement_review", "", nil, nil, true),
			edge("e-req-review-approve", "requirement_review", "prd_draft", "approved", cond("decision", "eq", "approve"), map[string]string{"approved_requirement": "$input.requirement_draft"}, false),
			edge("e-req-review-rework", "requirement_review", "requirement_draft", "changes requested", cond("decision", "eq", "request_changes"), map[string]string{"review_comments": "$output.comments", "previous_draft": "$input.requirement_draft"}, false),
			edge("e-prd-to-review", "prd_draft", "prd_review", "", nil, nil, true),
			edge("e-prd-review-approve", "prd_review", "tech_spec_draft", "approved", cond("decision", "eq", "approve"), map[string]string{"approved_prd": "$output.approved_prd"}, false),
			edge("e-prd-review-rework", "prd_review", "prd_draft", "changes requested", cond("decision", "eq", "request_changes"), map[string]string{"review_comments": "$output.comments", "previous_prd": "$input.prd"}, false),
			edge("e-tech-to-review", "tech_spec_draft", "tech_spec_review", "", nil, nil, true),
			edge("e-tech-review-approve", "tech_spec_review", "implementation", "approved", cond("decision", "eq", "approve"), map[string]string{"approved_technical_spec": "$output.approved_technical_spec"}, false),
			edge("e-tech-review-rework", "tech_spec_review", "tech_spec_draft", "changes requested", cond("decision", "eq", "request_changes"), map[string]string{"review_comments": "$output.comments", "previous_spec": "$input.technical_spec"}, false),
			edge("e-impl-to-review", "implementation", "code_review", "", nil, nil, true),
			edge("e-code-review-approve", "code_review", "qa", "approved", cond("decision", "eq", "approve"), map[string]string{"approved_change": "$output.approved_change"}, false),
			edge("e-code-review-rework", "code_review", "implementation", "changes requested", cond("decision", "eq", "request_changes"), map[string]string{"review_comments": "$output.comments", "previous_pr": "$input.pr"}, false),
			edge("e-qa-to-review", "qa", "qa_review", "", nil, nil, true),
			edge("e-qa-review-approve", "qa_review", "release", "approved", cond("decision", "eq", "approve"), map[string]string{"release_candidate": "$output.release_candidate"}, false),
			edge("e-qa-review-rework", "qa_review", "qa", "changes requested", cond("decision", "eq", "request_changes"), map[string]string{"review_comments": "$output.comments", "previous_report": "$input.test_report"}, false),
		},
	}
	return s.SaveDefinition(&def)
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

func softwareDeliveryTemplate(locale string) entity.WorkflowTemplate {
	locale = normalizeTemplateLocale(locale)
	text := softwareDeliveryText(locale)
	step := func(id, typ, titleKey, descKey, role, color string, x int, inputs, outputs []entity.WorkflowField) entity.WorkflowStep {
		cfg := map[string]string{}
		if color != "" {
			cfg["color"] = color
		}
		return entity.WorkflowStep{
			ID:           id,
			Type:         typ,
			Title:        text[titleKey],
			Description:  text[descKey],
			ActorRole:    role,
			InputFields:  inputs,
			OutputFields: outputs,
			ReviewPolicy: reviewPolicyForType(typ),
			Position:     entity.WorkflowPosition{X: x, Y: 180},
			Config:       cfg,
		}
	}
	field := func(name, descKey string) entity.WorkflowField {
		return entity.WorkflowField{Name: name, Description: text[descKey]}
	}
	return entity.WorkflowTemplate{
		ID:          "agentic-software-delivery",
		Name:        text["name"],
		Description: text["description"],
		Version:     1,
		Locale:      locale,
		StartStepID: "requirement_draft",
		Steps: []entity.WorkflowStep{
			step("requirement_draft", "agent_task", "requirementDraftTitle", "requirementDraftDesc", "pm-agent", "sky", 80, []entity.WorkflowField{field("request", "requestField"), field("context", "contextField")}, []entity.WorkflowField{field("requirement_draft", "requirementDraftField"), field("open_questions", "openQuestionsField")}),
			step("requirement_review", "human_review", "requirementReviewTitle", "requirementReviewDesc", "product-owner", "amber", 360, []entity.WorkflowField{field("requirement_draft", "requirementDraftReviewField")}, []entity.WorkflowField{field("decision", "decisionField"), field("comments", "commentsField")}),
			step("prd_draft", "agent_task", "prdDraftTitle", "prdDraftDesc", "pm-agent", "sky", 640, []entity.WorkflowField{field("approved_requirement", "approvedRequirementField")}, []entity.WorkflowField{field("prd", "prdField"), field("acceptance_criteria", "acceptanceCriteriaField")}),
			step("prd_review", "human_review", "prdReviewTitle", "prdReviewDesc", "product-owner", "amber", 920, []entity.WorkflowField{field("prd", "prdReviewField")}, []entity.WorkflowField{field("decision", "decisionField"), field("comments", "commentsField"), field("approved_prd", "approvedPRDField")}),
			step("tech_spec_draft", "agent_task", "techSpecDraftTitle", "techSpecDraftDesc", "engineering-agent", "violet", 1200, []entity.WorkflowField{field("approved_prd", "approvedPRDInputField")}, []entity.WorkflowField{field("technical_spec", "technicalSpecField"), field("task_split", "taskSplitField")}),
			step("tech_spec_review", "human_review", "techSpecReviewTitle", "techSpecReviewDesc", "tech-lead", "amber", 1480, []entity.WorkflowField{field("technical_spec", "technicalSpecReviewField")}, []entity.WorkflowField{field("decision", "decisionField"), field("comments", "commentsField"), field("approved_technical_spec", "approvedTechnicalSpecField")}),
			step("implementation", "agent_task", "implementationTitle", "implementationDesc", "developer-agent", "emerald", 1760, []entity.WorkflowField{field("approved_technical_spec", "approvedTechnicalSpecInputField")}, []entity.WorkflowField{field("pr", "prField"), field("tests_run", "testsRunField"), field("risks", "risksField")}),
			step("code_review", "human_review", "codeReviewTitle", "codeReviewDesc", "owner-engineer", "amber", 2040, []entity.WorkflowField{field("pr", "prReviewField")}, []entity.WorkflowField{field("decision", "decisionField"), field("comments", "commentsField"), field("approved_change", "approvedChangeField")}),
			step("qa", "agent_task", "qaTitle", "qaDesc", "qa-agent", "rose", 2320, []entity.WorkflowField{field("approved_change", "approvedChangeInputField")}, []entity.WorkflowField{field("test_cases", "testCasesField"), field("test_report", "testReportField")}),
			step("qa_review", "human_review", "qaReviewTitle", "qaReviewDesc", "qa-owner", "amber", 2600, []entity.WorkflowField{field("test_report", "testReportReviewField")}, []entity.WorkflowField{field("decision", "decisionField"), field("comments", "commentsField"), field("release_candidate", "releaseCandidateField")}),
			step("release", "agent_task", "releaseTitle", "releaseDesc", "release-agent", "emerald", 2880, []entity.WorkflowField{field("release_candidate", "releaseCandidateInputField")}, []entity.WorkflowField{field("release_report", "releaseReportField")}),
		},
		Edges: []entity.WorkflowEdge{
			edge("e-req-to-review", "requirement_draft", "requirement_review", "", nil, nil, true),
			edge("e-req-review-approve", "requirement_review", "prd_draft", text["approved"], cond("decision", "eq", "approve"), map[string]string{"approved_requirement": "$input.requirement_draft"}, false),
			edge("e-req-review-rework", "requirement_review", "requirement_draft", text["changesRequested"], cond("decision", "eq", "request_changes"), map[string]string{"review_comments": "$output.comments", "previous_draft": "$input.requirement_draft"}, false),
			edge("e-prd-to-review", "prd_draft", "prd_review", "", nil, nil, true),
			edge("e-prd-review-approve", "prd_review", "tech_spec_draft", text["approved"], cond("decision", "eq", "approve"), map[string]string{"approved_prd": "$output.approved_prd"}, false),
			edge("e-prd-review-rework", "prd_review", "prd_draft", text["changesRequested"], cond("decision", "eq", "request_changes"), map[string]string{"review_comments": "$output.comments", "previous_prd": "$input.prd"}, false),
			edge("e-tech-to-review", "tech_spec_draft", "tech_spec_review", "", nil, nil, true),
			edge("e-tech-review-approve", "tech_spec_review", "implementation", text["approved"], cond("decision", "eq", "approve"), map[string]string{"approved_technical_spec": "$output.approved_technical_spec"}, false),
			edge("e-tech-review-rework", "tech_spec_review", "tech_spec_draft", text["changesRequested"], cond("decision", "eq", "request_changes"), map[string]string{"review_comments": "$output.comments", "previous_spec": "$input.technical_spec"}, false),
			edge("e-impl-to-review", "implementation", "code_review", "", nil, nil, true),
			edge("e-code-review-approve", "code_review", "qa", text["approved"], cond("decision", "eq", "approve"), map[string]string{"approved_change": "$output.approved_change"}, false),
			edge("e-code-review-rework", "code_review", "implementation", text["changesRequested"], cond("decision", "eq", "request_changes"), map[string]string{"review_comments": "$output.comments", "previous_pr": "$input.pr"}, false),
			edge("e-qa-to-review", "qa", "qa_review", "", nil, nil, true),
			edge("e-qa-review-approve", "qa_review", "release", text["approved"], cond("decision", "eq", "approve"), map[string]string{"release_candidate": "$output.release_candidate"}, false),
			edge("e-qa-review-rework", "qa_review", "qa", text["changesRequested"], cond("decision", "eq", "request_changes"), map[string]string{"review_comments": "$output.comments", "previous_report": "$input.test_report"}, false),
		},
	}
}

func reviewPolicyForType(typ string) string {
	if typ == "human_review" {
		return "manual"
	}
	return ""
}

func normalizeTemplateLocale(locale string) string {
	locale = strings.ToLower(strings.TrimSpace(locale))
	switch {
	case strings.HasPrefix(locale, "zh-tw"), strings.HasPrefix(locale, "zh-hk"):
		return "zh-TW"
	case strings.HasPrefix(locale, "zh"):
		return "zh-CN"
	case strings.HasPrefix(locale, "ja"):
		return "ja"
	default:
		return "en"
	}
}

func softwareDeliveryText(locale string) map[string]string {
	en := map[string]string{
		"name":                            "Agentic Software Delivery",
		"description":                     "A configurable human-agent delivery workflow. Agents draft artifacts, humans review decision gates, and rejected outputs loop back with structured feedback.",
		"approved":                        "approved",
		"changesRequested":                "changes requested",
		"requirementDraftTitle":           "Requirement Draft",
		"requirementDraftDesc":            "An agent turns an incoming request into a structured understanding: problem, goal, scope, non-goals, risks, and open questions.",
		"requirementReviewTitle":          "Requirement Review",
		"requirementReviewDesc":           "A human reviews whether the requirement draft expresses the real problem and whether more clarification is needed.",
		"prdDraftTitle":                   "PRD Draft",
		"prdDraftDesc":                    "The PM agent produces the product spec, acceptance criteria, release scope, and non-goals.",
		"prdReviewTitle":                  "PRD Review",
		"prdReviewDesc":                   "Product and engineering stakeholders review scope, non-goals, and acceptance criteria.",
		"techSpecDraftTitle":              "Technical Spec Draft",
		"techSpecDraftDesc":               "Engineering agents inspect the codebase and produce implementation plan, affected surfaces, test strategy, and task split recommendation.",
		"techSpecReviewTitle":             "Technical Spec Review",
		"techSpecReviewDesc":              "Responsible engineers review the plan before implementation starts.",
		"implementationTitle":             "Implementation",
		"implementationDesc":              "Development agents implement the approved technical plan and produce code changes, tests, and a PR or patch summary.",
		"codeReviewTitle":                 "Code Review",
		"codeReviewDesc":                  "The responsible human reviews code quality, risk, and whether the output matches the approved spec.",
		"qaTitle":                         "QA Test",
		"qaDesc":                          "QA agents generate and execute test cases, then identify remaining manual test needs.",
		"qaReviewTitle":                   "QA Review",
		"qaReviewDesc":                    "Human QA or owner reviews the test report and decides whether release can proceed.",
		"releaseTitle":                    "Release and Observe",
		"releaseDesc":                     "Release agents prepare rollout notes, execute allowed release steps, and check post-release signals.",
		"doneTitle":                       "Done",
		"doneDesc":                        "Workflow completed with artifacts and metrics ready for retrospective.",
		"requestField":                    "Original user, customer, founder, or internal request.",
		"contextField":                    "Known background, links, meeting notes, or existing discussions.",
		"requirementDraftField":           "Structured requirement draft.",
		"openQuestionsField":              "Questions that still need human or stakeholder clarification.",
		"requirementDraftReviewField":     "Draft produced by the PM agent.",
		"decisionField":                   "approve, request_changes, or need_discussion.",
		"commentsField":                   "Review comments and clarification notes.",
		"approvedRequirementField":        "Reviewed requirement with comments folded in.",
		"prdField":                        "Product requirements document or spec.",
		"acceptanceCriteriaField":         "Observable acceptance criteria.",
		"prdReviewField":                  "PRD draft to review.",
		"approvedPRDField":                "Final PRD when approved.",
		"approvedPRDInputField":           "Reviewed product spec.",
		"technicalSpecField":              "Implementation plan and technical decisions.",
		"taskSplitField":                  "Optional child task split for parallel work.",
		"technicalSpecReviewField":        "Technical plan to review.",
		"approvedTechnicalSpecField":      "Final technical spec when approved.",
		"approvedTechnicalSpecInputField": "Approved implementation plan.",
		"prField":                         "Pull request, patch, or change summary.",
		"testsRunField":                   "Tests executed by the agent.",
		"risksField":                      "Known risks or manual checks needed.",
		"prReviewField":                   "PR or patch to review.",
		"approvedChangeField":             "Approved code artifact.",
		"approvedChangeInputField":        "Code artifact approved for testing.",
		"testCasesField":                  "Test cases.",
		"testReportField":                 "Automated and manual test result summary.",
		"testReportReviewField":           "Test report to review.",
		"releaseCandidateField":           "Approved release candidate.",
		"releaseCandidateInputField":      "Approved release candidate.",
		"releaseReportField":              "Release result, monitoring checks, and follow-up items.",
	}
	if locale == "zh-CN" {
		return mergeText(en, map[string]string{
			"name":                            "Agent 研发交付流程",
			"description":                     "一套可配置的人机协作研发流程：Agent 产出文档和代码，人类只审核关键关口，未通过的产出带结构化意见回流修改。",
			"approved":                        "通过",
			"changesRequested":                "需要修改",
			"requirementDraftTitle":           "需求理解草稿",
			"requirementDraftDesc":            "Agent 将原始需求整理成结构化理解：问题、目标、范围、非目标、风险和待澄清问题。",
			"requirementReviewTitle":          "需求审核",
			"requirementReviewDesc":           "人类审核需求草稿是否表达了真实问题，以及是否还需要继续澄清。",
			"prdDraftTitle":                   "产品文档草稿",
			"prdDraftDesc":                    "产品 Agent 输出产品规格、验收标准、发版范围和非目标。",
			"prdReviewTitle":                  "产品文档审核",
			"prdReviewDesc":                   "产品和工程相关负责人审核范围、非目标和验收标准。",
			"techSpecDraftTitle":              "技术方案草稿",
			"techSpecDraftDesc":               "工程 Agent 调研代码库并输出实现方案、影响面、测试策略和任务拆分建议。",
			"techSpecReviewTitle":             "技术方案审核",
			"techSpecReviewDesc":              "负责工程师在开发开始前审核实现方案。",
			"implementationTitle":             "开发实现",
			"implementationDesc":              "开发 Agent 根据通过的技术方案完成代码修改、测试和 PR 或补丁摘要。",
			"codeReviewTitle":                 "代码审核",
			"codeReviewDesc":                  "负责人审核代码质量、风险，以及产出是否符合已确认的方案。",
			"qaTitle":                         "测试执行",
			"qaDesc":                          "测试 Agent 生成并执行测试用例，同时标记仍需人工检查的部分。",
			"qaReviewTitle":                   "测试审核",
			"qaReviewDesc":                    "QA 或负责人审核测试报告并决定是否可以进入发布。",
			"releaseTitle":                    "发布与观察",
			"releaseDesc":                     "发布 Agent 准备发布说明、执行允许的发布动作，并检查上线后信号。",
			"doneTitle":                       "完成",
			"doneDesc":                        "流程完成，产物和指标可用于复盘。",
			"requestField":                    "用户、客户、老板或内部提出的原始需求。",
			"contextField":                    "已知背景、链接、会议记录或历史讨论。",
			"requirementDraftField":           "结构化需求草稿。",
			"openQuestionsField":              "仍需要人类或相关方澄清的问题。",
			"requirementDraftReviewField":     "PM Agent 输出的需求草稿。",
			"decisionField":                   "approve、request_changes 或 need_discussion。",
			"commentsField":                   "审核意见和澄清说明。",
			"approvedRequirementField":        "已合入审核意见的需求。",
			"prdField":                        "产品需求文档或规格说明。",
			"acceptanceCriteriaField":         "可观察的验收标准。",
			"prdReviewField":                  "待审核的产品文档草稿。",
			"approvedPRDField":                "审核通过后的最终产品文档。",
			"approvedPRDInputField":           "已审核的产品规格。",
			"technicalSpecField":              "实现方案和技术决策。",
			"taskSplitField":                  "可选的并行子任务拆分。",
			"technicalSpecReviewField":        "待审核的技术方案。",
			"approvedTechnicalSpecField":      "审核通过后的最终技术方案。",
			"approvedTechnicalSpecInputField": "已通过的实现方案。",
			"prField":                         "PR、补丁或变更摘要。",
			"testsRunField":                   "Agent 已执行的测试。",
			"risksField":                      "已知风险或需要人工检查的事项。",
			"prReviewField":                   "待审核的 PR 或补丁。",
			"approvedChangeField":             "已审核通过的代码产物。",
			"approvedChangeInputField":        "已通过代码审核的产物。",
			"testCasesField":                  "测试用例。",
			"testReportField":                 "自动化和人工测试结果摘要。",
			"testReportReviewField":           "待审核的测试报告。",
			"releaseCandidateField":           "已通过测试准出的发布候选。",
			"releaseCandidateInputField":      "已通过测试准出的发布候选。",
			"releaseReportField":              "发布结果、监控检查和后续事项。",
		})
	}
	if locale == "zh-TW" {
		return mergeText(en, map[string]string{
			"name":                   "Agent 研發交付流程",
			"description":            "一套可配置的人機協作研發流程：Agent 產出文件和程式碼，人類只審核關鍵關口，未通過的產出帶結構化意見回流修改。",
			"approved":               "通過",
			"changesRequested":       "需要修改",
			"requirementDraftTitle":  "需求理解草稿",
			"requirementReviewTitle": "需求審核",
			"prdDraftTitle":          "產品文件草稿",
			"prdReviewTitle":         "產品文件審核",
			"techSpecDraftTitle":     "技術方案草稿",
			"techSpecReviewTitle":    "技術方案審核",
			"implementationTitle":    "開發實作",
			"codeReviewTitle":        "程式碼審核",
			"qaTitle":                "測試執行",
			"qaReviewTitle":          "測試審核",
			"releaseTitle":           "發布與觀察",
			"doneTitle":              "完成",
		})
	}
	if locale == "ja" {
		return mergeText(en, map[string]string{
			"name":                   "Agent ソフトウェアデリバリー",
			"description":            "Agent が成果物を作成し、人が重要なゲートだけをレビューし、差し戻しは構造化されたフィードバックとして戻るワークフローです。",
			"approved":               "承認",
			"changesRequested":       "修正依頼",
			"requirementDraftTitle":  "要件ドラフト",
			"requirementReviewTitle": "要件レビュー",
			"prdDraftTitle":          "PRD ドラフト",
			"prdReviewTitle":         "PRD レビュー",
			"techSpecDraftTitle":     "技術仕様ドラフト",
			"techSpecReviewTitle":    "技術仕様レビュー",
			"implementationTitle":    "実装",
			"codeReviewTitle":        "コードレビュー",
			"qaTitle":                "QA テスト",
			"qaReviewTitle":          "QA レビュー",
			"releaseTitle":           "リリースと監視",
			"doneTitle":              "完了",
		})
	}
	return en
}

func mergeText(base, override map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(override))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range override {
		out[k] = v
	}
	return out
}

func (s *Store) SaveDefinition(def *entity.WorkflowDefinition) error {
	if def.ID == "" {
		def.ID = entity.NewWorkflowID()
	}
	now := time.Now().UTC()
	if def.CreatedAt.IsZero() {
		def.CreatedAt = now
	}
	def.UpdatedAt = now
	if def.Version == 0 {
		def.Version = 1
	}
	if def.Scope == "" {
		def.Scope = "project"
	}
	raw, err := json.Marshal(def)
	if err != nil {
		return err
	}
	return s.db.UpsertRecord("workflow_definitions", s.workspaceID, []string{def.ID}, string(raw))
}

func (s *Store) ListDefinitions() ([]entity.WorkflowDefinition, error) {
	recs, err := s.db.ListRecords("workflow_definitions", s.workspaceID, nil)
	if err != nil {
		return nil, err
	}
	out := make([]entity.WorkflowDefinition, 0, len(recs))
	for _, rec := range recs {
		var def entity.WorkflowDefinition
		if json.Unmarshal([]byte(rec.Payload), &def) == nil && def.Scope == "workspace" && def.Project == "" {
			out = append(out, def)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func (s *Store) Definition(id string) (entity.WorkflowDefinition, bool, error) {
	raw, ok, err := s.db.GetRecord("workflow_definitions", s.workspaceID, []string{id})
	if err != nil || !ok {
		return entity.WorkflowDefinition{}, ok, err
	}
	var def entity.WorkflowDefinition
	if err := json.Unmarshal([]byte(raw), &def); err != nil {
		return entity.WorkflowDefinition{}, false, err
	}
	return def, true, nil
}

func (s *Store) DeleteDefinition(id string) error {
	return s.db.DeleteRecord("workflow_definitions", s.workspaceID, []string{id})
}

func (s *Store) StartRun(project, taskID, definitionID string, actorBindings map[string]entity.WorkflowActorBinding) (entity.WorkflowRun, []entity.WorkflowStepInstance, error) {
	def, ok, err := s.Definition(definitionID)
	if err != nil {
		return entity.WorkflowRun{}, nil, err
	}
	if !ok {
		return entity.WorkflowRun{}, nil, errs.NotFound("workflow_definition", definitionID)
	}
	now := time.Now().UTC()
	run := entity.WorkflowRun{
		ID:            entity.NewWorkflowRunID(),
		DefinitionID:  def.ID,
		Project:       project,
		TaskID:        taskID,
		Status:        "active",
		ActiveStepID:  def.StartStepID,
		ActorBindings: actorBindings,
		StartedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.SaveRun(&run); err != nil {
		return entity.WorkflowRun{}, nil, err
	}
	instances := make([]entity.WorkflowStepInstance, 0, len(def.Steps))
	for _, step := range def.Steps {
		status := "pending"
		started := time.Time{}
		if step.ID == def.StartStepID {
			status = "running"
			started = now
		}
		inst := entity.WorkflowStepInstance{
			ID:        entity.NewWorkflowStepInstanceID(),
			RunID:     run.ID,
			StepID:    step.ID,
			Status:    status,
			StartedAt: started,
			UpdatedAt: now,
		}
		if binding, ok := actorBindings[step.ActorRole]; ok {
			inst.ActorType = binding.Type
			inst.ActorID = binding.ID
		}
		if err := s.SaveStepInstance(&inst); err != nil {
			return entity.WorkflowRun{}, nil, err
		}
		instances = append(instances, inst)
	}
	return run, instances, nil
}

func (s *Store) SaveRun(run *entity.WorkflowRun) error {
	raw, err := json.Marshal(run)
	if err != nil {
		return err
	}
	return s.db.UpsertRecord("workflow_runs", s.workspaceID, []string{run.Project, run.TaskID, run.ID}, string(raw))
}

func (s *Store) RunForTask(project, taskID string) (entity.WorkflowRun, bool, error) {
	recs, err := s.db.ListRecords("workflow_runs", s.workspaceID, []string{project, taskID})
	if err != nil || len(recs) == 0 {
		return entity.WorkflowRun{}, false, err
	}
	var run entity.WorkflowRun
	if err := json.Unmarshal([]byte(recs[0].Payload), &run); err != nil {
		return entity.WorkflowRun{}, false, err
	}
	return run, true, nil
}

func (s *Store) SaveStepInstance(inst *entity.WorkflowStepInstance) error {
	raw, err := json.Marshal(inst)
	if err != nil {
		return err
	}
	return s.db.UpsertRecord("workflow_step_instances", s.workspaceID, []string{inst.RunID, inst.StepID, inst.ID}, string(raw))
}

func (s *Store) ListStepInstances(runID string) ([]entity.WorkflowStepInstance, error) {
	recs, err := s.db.ListRecords("workflow_step_instances", s.workspaceID, []string{runID})
	if err != nil {
		return nil, err
	}
	out := make([]entity.WorkflowStepInstance, 0, len(recs))
	for _, rec := range recs {
		var inst entity.WorkflowStepInstance
		if json.Unmarshal([]byte(rec.Payload), &inst) == nil {
			out = append(out, inst)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StepID < out[j].StepID })
	return out, nil
}

func (s *Store) RecordActiveStepOutput(project, taskID, summary, output, status string) error {
	run, ok, err := s.RunForTask(project, taskID)
	if err != nil || !ok {
		return err
	}
	instances, err := s.ListStepInstances(run.ID)
	if err != nil {
		return err
	}
	for _, inst := range instances {
		if inst.StepID != run.ActiveStepID {
			continue
		}
		now := time.Now().UTC()
		inst.Summary = strings.TrimSpace(summary)
		inst.OutputArtifact = strings.TrimSpace(output)
		if strings.TrimSpace(status) != "" {
			inst.Status = strings.TrimSpace(status)
		}
		inst.FinishedAt = now
		inst.UpdatedAt = now
		return s.SaveStepInstance(&inst)
	}
	return nil
}

type TransitionResult struct {
	Run      entity.WorkflowRun
	Current  entity.WorkflowStepInstance
	Next     *entity.WorkflowStep
	NextInst *entity.WorkflowStepInstance
	Done     bool
}

func (s *Store) CompleteAndAdvance(project, taskID, summary, output string, outputValues map[string]string, status string) (TransitionResult, error) {
	var result TransitionResult
	run, ok, err := s.RunForTask(project, taskID)
	if err != nil || !ok {
		return result, err
	}
	def, ok, err := s.Definition(run.DefinitionID)
	if err != nil || !ok {
		return result, err
	}
	instances, err := s.ListStepInstances(run.ID)
	if err != nil {
		return result, err
	}
	currentStep, ok := stepByID(def.Steps, run.ActiveStepID)
	if !ok {
		return result, nil
	}
	now := time.Now().UTC()
	values, err := normalizeWorkflowOutputValues(currentStep, outputValues, summary, output, strings.TrimSpace(status) == "failed")
	if err != nil {
		return result, err
	}
	output = workflowValuesJSON(values)
	if strings.TrimSpace(summary) == "" {
		summary = workflowSummaryFromValues(values)
	}
	for i := range instances {
		if instances[i].StepID != run.ActiveStepID {
			continue
		}
		instances[i].Summary = strings.TrimSpace(summary)
		instances[i].OutputArtifact = output
		instances[i].OutputValues = values
		instances[i].Status = strings.TrimSpace(status)
		if instances[i].Status == "" {
			instances[i].Status = "completed"
		}
		instances[i].FinishedAt = now
		instances[i].UpdatedAt = now
		if err := s.SaveStepInstance(&instances[i]); err != nil {
			return result, err
		}
		result.Current = instances[i]
		break
	}
	edge, hasNext := chooseNextEdge(def.Edges, currentStep.ID, values, output)
	if !hasNext {
		run.Status = "completed"
		run.ActiveStepID = ""
		run.UpdatedAt = now
		run.FinishedAt = now
		if err := s.SaveRun(&run); err != nil {
			return result, err
		}
		result.Run = run
		result.Done = true
		return result, nil
	}
	nextStep, ok := stepByID(def.Steps, edge.To)
	if !ok {
		run.Status = "completed"
		run.ActiveStepID = ""
		run.UpdatedAt = now
		run.FinishedAt = now
		if err := s.SaveRun(&run); err != nil {
			return result, err
		}
		result.Run = run
		result.Done = true
		return result, nil
	}
	run.ActiveStepID = nextStep.ID
	run.UpdatedAt = now
	if err := s.SaveRun(&run); err != nil {
		return result, err
	}
	for i := range instances {
		if instances[i].StepID != nextStep.ID {
			continue
		}
		instances[i].Status = "running"
		instances[i].StartedAt = now
		instances[i].UpdatedAt = now
		instances[i].InputValues = buildNextInputValues(result.Current, nextStep, edge)
		instances[i].InputArtifact = buildNextInputArtifact(currentStep, result.Current, nextStep, edge)
		if err := s.SaveStepInstance(&instances[i]); err != nil {
			return result, err
		}
		result.NextInst = &instances[i]
		break
	}
	result.Run = run
	result.Next = &nextStep
	return result, nil
}

func normalizeWorkflowOutputValues(step entity.WorkflowStep, values map[string]string, summary, output string, failed bool) (map[string]string, error) {
	out := make(map[string]string)
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	if len(step.OutputFields) == 0 {
		if strings.TrimSpace(output) != "" {
			out["output"] = strings.TrimSpace(output)
		}
		if strings.TrimSpace(summary) != "" {
			out["summary"] = strings.TrimSpace(summary)
		}
		return out, nil
	}
	allowed := make(map[string]entity.WorkflowField, len(step.OutputFields))
	for _, field := range step.OutputFields {
		name := strings.TrimSpace(field.Name)
		if name != "" {
			allowed[name] = field
		}
	}
	if len(out) == 0 && !failed {
		return nil, fmt.Errorf("workflow step %q requires structured outputs: %s", step.Title, strings.Join(workflowFieldNames(step.OutputFields), ", "))
	}
	for key := range out {
		if _, ok := allowed[key]; !ok {
			return nil, fmt.Errorf("workflow output field %q is not defined on step %q", key, step.Title)
		}
	}
	if failed {
		return out, nil
	}
	for _, field := range step.OutputFields {
		name := strings.TrimSpace(field.Name)
		if name == "" {
			continue
		}
		if strings.TrimSpace(out[name]) == "" {
			return nil, fmt.Errorf("workflow output field %q is required for step %q", name, step.Title)
		}
	}
	return out, nil
}

func workflowFieldNames(fields []entity.WorkflowField) []string {
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		if name := strings.TrimSpace(field.Name); name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func workflowValuesJSON(values map[string]string) string {
	if len(values) == 0 {
		return "{}"
	}
	raw, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func workflowSummaryFromValues(values map[string]string) string {
	if v := strings.TrimSpace(values["summary"]); v != "" {
		return v
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	items := make([]string, 0, len(keys))
	for _, key := range keys {
		value := strings.TrimSpace(values[key])
		if value == "" {
			continue
		}
		if len(value) > 80 {
			value = value[:77] + "..."
		}
		items = append(items, key+": "+value)
	}
	return strings.Join(items, "; ")
}

func stepByID(steps []entity.WorkflowStep, id string) (entity.WorkflowStep, bool) {
	for _, step := range steps {
		if step.ID == id {
			return step, true
		}
	}
	return entity.WorkflowStep{}, false
}

func chooseNextEdge(edges []entity.WorkflowEdge, from string, outputValues map[string]string, output string) (entity.WorkflowEdge, bool) {
	var fallback *entity.WorkflowEdge
	for i := range edges {
		edge := edges[i]
		if edge.From != from {
			continue
		}
		if edge.IsDefault || edge.Condition == nil {
			if fallback == nil {
				fallback = &edge
			}
			continue
		}
		if workflowConditionMatches(edge.Condition, outputValues, output) {
			return edge, true
		}
	}
	if fallback != nil {
		return *fallback, true
	}
	return entity.WorkflowEdge{}, false
}

func workflowConditionMatches(cond *entity.WorkflowEdgeCondition, outputValues map[string]string, output string) bool {
	if cond == nil {
		return true
	}
	field := strings.TrimSpace(cond.Field)
	value := strings.TrimSpace(cond.Value)
	if field != "" {
		if actual, ok := outputValues[field]; ok {
			return compareWorkflowValue(actual, cond.Operator, value, cond.Values)
		}
	}
	return compareWorkflowValue(output, cond.Operator, value, cond.Values)
}

func compareWorkflowValue(actual, op, value string, values []string) bool {
	actual = strings.TrimSpace(strings.ToLower(actual))
	value = strings.TrimSpace(strings.ToLower(value))
	op = strings.TrimSpace(strings.ToLower(op))
	switch op {
	case "", "eq":
		return actual == value || strings.Contains(actual, value)
	case "neq":
		return actual != value && !strings.Contains(actual, value)
	case "exists":
		return actual != ""
	case "in":
		for _, item := range values {
			item = strings.TrimSpace(strings.ToLower(item))
			if actual == item || strings.Contains(actual, item) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func buildNextInputValues(currentInst entity.WorkflowStepInstance, next entity.WorkflowStep, edge entity.WorkflowEdge) map[string]string {
	out := make(map[string]string)
	resolve := func(expr string) string {
		expr = strings.TrimSpace(expr)
		switch {
		case strings.HasPrefix(expr, "$output."):
			return currentInst.OutputValues[strings.TrimPrefix(expr, "$output.")]
		case strings.HasPrefix(expr, "$input."):
			return currentInst.InputValues[strings.TrimPrefix(expr, "$input.")]
		default:
			return expr
		}
	}
	if len(edge.InputMapping) > 0 {
		for key, expr := range edge.InputMapping {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			out[key] = strings.TrimSpace(resolve(expr))
		}
		return out
	}
	for _, field := range next.InputFields {
		name := strings.TrimSpace(field.Name)
		if name == "" {
			continue
		}
		if v := strings.TrimSpace(currentInst.OutputValues[name]); v != "" {
			out[name] = v
		}
	}
	return out
}

func buildNextInputArtifact(current entity.WorkflowStep, currentInst entity.WorkflowStepInstance, next entity.WorkflowStep, edge entity.WorkflowEdge) string {
	payload := map[string]any{
		"previous_step": map[string]string{
			"id":    current.ID,
			"title": current.Title,
		},
		"outputs": currentInst.OutputValues,
		"inputs":  buildNextInputValues(currentInst, next, edge),
	}
	if len(next.InputFields) > 0 {
		payload["expected_input_fields"] = workflowFieldNames(next.InputFields)
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return ""
	}
	return string(raw)
}
