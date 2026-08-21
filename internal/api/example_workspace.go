package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/multigent/multigent/internal/avatar"
	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/scaffold"
	"github.com/multigent/multigent/internal/store"
	"github.com/multigent/multigent/internal/taskstore"
	workflowstore "github.com/multigent/multigent/internal/workflow"
)

const (
	exampleWorkspaceName  = "Example Workspace"
	exampleProjectName    = "hello-world-relay"
	exampleTeamName       = "collaboration-demo"
	exampleWorkflowID     = "wf-example-hello-world-relay"
	exampleGreeterAgent   = "Lina"
	exampleResponderAgent = "Mira"
	exampleRecorderAgent  = "Nora"
)

func (s *Server) handleCreateExampleWorkspace(w http.ResponseWriter, r *http.Request) {
	cur := s.currentUser(r)
	if cur == nil || cur.Username == "" || cur.Username == "apikey" {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeAuthenticatedUserRequired, "authenticated user required")
		return
	}
	if s.controlDB == nil {
		s.jsonErrorCode(w, http.StatusServiceUnavailable, ErrCodeWorkspaceDatabaseUnavailable, "control database unavailable")
		return
	}
	var body struct {
		Locale string `json:"locale"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := s.readJSON(w, r, &body); err != nil {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
			return
		}
	}
	spec := exampleWorkspaceSpec(preferredExampleLocale(body.Locale, r.Header.Get("Accept-Language")))

	id := newWorkspaceID()
	absRoot, err := filepath.Abs(filepath.Join(defaultWorkspaceDataDir(), id))
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := os.MkdirAll(absRoot, 0o755); err != nil {
		s.serverError(w, err)
		return
	}

	now := time.Now().UTC()
	agency := &entity.Agency{
		Name:        exampleWorkspaceName,
		Description: spec.WorkspaceDescription,
		CreatedBy:   cur.Username,
		CreatedAt:   now.Format(time.RFC3339),
	}
	if err := scaffold.InitAgency(absRoot, agency); err != nil {
		s.serverError(w, err)
		return
	}

	ref := workspaceRef{
		ID:          id,
		Name:        agency.Name,
		Description: agency.Description,
		Root:        absRoot,
		CreatedBy:   agency.CreatedBy,
		CreatedAt:   agency.CreatedAt,
	}
	if err := s.upsertWorkspaceRef(ref); err != nil {
		s.serverError(w, err)
		return
	}
	if err := s.controlDB.UpsertWorkspaceMember(ref.ID, cur.Username, WorkspaceRoleOwner); err != nil {
		s.serverError(w, err)
		return
	}

	exampleStore := store.NewDB(absRoot, s.controlDB)
	exampleTasks := taskstore.NewDB(absRoot, s.controlDB)
	if err := seedExampleWorkspace(absRoot, ref.ID, cur.Username, spec, exampleStore, exampleTasks, s.controlDB); err != nil {
		s.serverError(w, err)
		return
	}

	if err := s.switchWorkspaceRoot(absRoot); err != nil {
		s.serverError(w, err)
		return
	}
	ref.Active = true
	s.auditLog(auditLogInput{
		WorkspaceID:  ref.ID,
		Action:       "workspace.example.create",
		ResourceType: "workspace",
		ResourceID:   ref.ID,
		Summary:      "Example workspace created",
		After: map[string]any{
			"id":        ref.ID,
			"name":      ref.Name,
			"project":   exampleProjectName,
			"workflow":  exampleWorkflowID,
			"createdBy": cur.Username,
		},
		Request: r,
	})
	_ = json.NewEncoder(w).Encode(ref)
}

func (s *Server) handleSeedCurrentExampleWorkspace(w http.ResponseWriter, r *http.Request) {
	cur := s.currentUser(r)
	if cur == nil || cur.Username == "" || cur.Username == "apikey" {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeAuthenticatedUserRequired, "authenticated user required")
		return
	}
	if s.controlDB == nil {
		s.jsonErrorCode(w, http.StatusServiceUnavailable, ErrCodeWorkspaceDatabaseUnavailable, "control database unavailable")
		return
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !s.canAdminWorkspace(r, workspaceID) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeWorkspaceAdminRequired, "workspace admin access required")
		return
	}
	var body struct {
		Locale      string `json:"locale"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := s.readJSON(w, r, &body); err != nil {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
			return
		}
	}
	if existing, err := s.st.Project(exampleProjectName); err == nil && existing != nil {
		s.updateCurrentExampleAgency(body.Name, body.Description, cur.Username)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":        true,
			"seeded":    false,
			"project":   exampleProjectName,
			"workflow":  exampleWorkflowID,
			"workspace": workspaceID,
		})
		return
	}
	spec := exampleWorkspaceSpec(preferredExampleLocale(body.Locale, r.Header.Get("Accept-Language")))
	if strings.TrimSpace(body.Description) == "" {
		body.Description = spec.WorkspaceDescription
	}
	s.updateCurrentExampleAgency(body.Name, body.Description, cur.Username)
	if err := seedExampleWorkspace(s.root, workspaceID, cur.Username, spec, s.st, s.ts, s.controlDB); err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "workspace.example.seed",
		ResourceType: "workspace",
		ResourceID:   workspaceID,
		Summary:      "Example content seeded into current workspace",
		After: map[string]any{
			"project":  exampleProjectName,
			"workflow": exampleWorkflowID,
		},
		Request: r,
	})
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":        true,
		"seeded":    true,
		"project":   exampleProjectName,
		"workflow":  exampleWorkflowID,
		"workspace": workspaceID,
	})
}

func (s *Server) updateCurrentExampleAgency(name, description, username string) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" && description == "" {
		return
	}
	agency, err := s.st.Agency()
	if err != nil {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if name != "" {
		agency.Name = name
	}
	if description != "" {
		agency.Description = description
	}
	if strings.TrimSpace(agency.CreatedBy) == "" {
		agency.CreatedBy = username
	}
	if strings.TrimSpace(agency.CreatedAt) == "" {
		agency.CreatedAt = now
	}
	agency.UpdatedAt = now
	_ = s.st.SaveAgency(agency)
}

func seedExampleWorkspace(root, workspaceID, username string, spec exampleLocaleSpec, st store.Store, ts taskstore.Store, db controldb.Store) error {
	if err := st.SaveAgencyPrompt(spec.AgencyPrompt); err != nil {
		return fmt.Errorf("save agency prompt: %w", err)
	}
	if err := st.SaveTeam(exampleTeamName, &entity.Team{
		Name:        exampleTeamName,
		Description: spec.TeamDescription,
		Owners:      []string{username},
		Goals:       spec.TeamGoals,
	}); err != nil {
		return fmt.Errorf("save example team: %w", err)
	}
	if err := st.SaveTeamPrompt(exampleTeamName, spec.TeamPrompt); err != nil {
		return fmt.Errorf("save example team prompt: %w", err)
	}
	for _, role := range exampleRoles(username, spec) {
		if err := st.SaveRole(exampleTeamName, role.Name, role.Role); err != nil {
			return fmt.Errorf("save example role %s: %w", role.Name, err)
		}
		if err := st.SaveRolePrompt(exampleTeamName, role.Name, role.Prompt); err != nil {
			return fmt.Errorf("save example role prompt %s: %w", role.Name, err)
		}
	}
	if err := st.SaveProject(exampleProjectName, &entity.Project{
		Name:        exampleProjectName,
		Description: spec.ProjectDescription,
		Owners:      []string{username},
	}); err != nil {
		return fmt.Errorf("save example project: %w", err)
	}
	if err := st.SaveProjectPrompt(exampleProjectName, spec.ProjectPrompt); err != nil {
		return fmt.Errorf("save example project prompt: %w", err)
	}
	agents := exampleAgents(username)
	if err := seedExampleAgentWorkers(db, workspaceID, agents, spec); err != nil {
		return fmt.Errorf("seed example agent workers: %w", err)
	}
	if err := seedExampleDocs(root, username, spec); err != nil {
		return fmt.Errorf("seed example docs: %w", err)
	}
	wfStore := workflowstore.NewStore(db, workspaceID)
	def := exampleWorkflowDefinition(spec)
	if err := wfStore.SaveDefinition(&def); err != nil {
		return fmt.Errorf("save example workflow definition: %w", err)
	}
	task := exampleTask(username, spec)
	if err := ts.AddTask(exampleProjectName, exampleGreeterAgent, task); err != nil {
		return fmt.Errorf("add example task: %w", err)
	}
	_, _, err := wfStore.StartRun(exampleProjectName, task.ID, def.ID, map[string]entity.WorkflowActorBinding{
		"greeter":        {Type: "agent", ID: exampleGreeterAgent},
		"greetingReview": {Type: "human", ID: username},
		"responder":      {Type: "agent", ID: exampleResponderAgent},
		"recorder":       {Type: "agent", ID: exampleRecorderAgent},
		"finalReview":    {Type: "human", ID: username},
	})
	if err != nil {
		return fmt.Errorf("start example workflow run: %w", err)
	}
	return nil
}

type exampleRoleSeed struct {
	Name   string
	Role   *entity.Role
	Prompt string
}

type exampleRoleText struct {
	Description string
	Prompt      string
}

type exampleLocaleSpec struct {
	WorkspaceDescription string
	TeamDescription      string
	TeamGoals            []string
	TeamPrompt           string
	ProjectDescription   string
	ProjectPrompt        string
	Roles                map[string]exampleRoleText
	DocTitle             string
	DocDescription       string
	DocBody              string
	WorkflowName         string
	WorkflowDescription  string
	StepText             map[string]struct {
		Title       string
		Description string
	}
	FieldText    map[string]string
	EdgeText     map[string]string
	TaskTitle    string
	TaskDesc     string
	TaskPrompt   string
	Schedules    exampleScheduleText
	AgencyPrompt string
}

type exampleScheduleText struct {
	GreeterWakeup       string
	ResponderWakeup     string
	RecorderWakeup      string
	DailyReviewTitle    string
	DailyReviewPrompt   string
	WeeklySummaryTitle  string
	WeeklySummaryPrompt string
}

func preferredExampleLocale(requested, acceptLanguage string) string {
	switch normalizeExampleLocale(requested) {
	case "en", "zh-CN", "zh-TW", "ja":
		return normalizeExampleLocale(requested)
	default:
		return acceptLanguage
	}
}

func normalizeExampleLocale(locale string) string {
	lower := strings.ToLower(strings.TrimSpace(locale))
	switch {
	case lower == "zh-tw" || lower == "zh-hk" || lower == "zh-mo":
		return "zh-TW"
	case lower == "zh" || lower == "zh-cn" || strings.HasPrefix(lower, "zh-cn-"):
		return "zh-CN"
	case lower == "ja" || strings.HasPrefix(lower, "ja-"):
		return "ja"
	case lower == "en" || strings.HasPrefix(lower, "en-"):
		return "en"
	default:
		return ""
	}
}

func exampleWorkspaceSpec(acceptLanguage string) exampleLocaleSpec {
	lang := "en"
	if normalized := normalizeExampleLocale(acceptLanguage); normalized != "" {
		lang = normalized
	} else {
		lower := strings.ToLower(acceptLanguage)
		switch {
		case strings.Contains(lower, "zh-tw") || strings.Contains(lower, "zh-hk") || strings.Contains(lower, "zh-mo"):
			lang = "zh-TW"
		case strings.Contains(lower, "zh"):
			lang = "zh-CN"
		case strings.Contains(lower, "ja"):
			lang = "ja"
		}
	}
	switch lang {
	case "zh-CN":
		return exampleZHSpec(false)
	case "zh-TW":
		return exampleZHSpec(true)
	case "ja":
		return exampleJASpec()
	default:
		return exampleENSpec()
	}
}

func exampleENSpec() exampleLocaleSpec {
	return exampleLocaleSpec{
		WorkspaceDescription: "A built-in learning workspace that helps you prepare a first onboarding note through agent handoff, human review, structured workflow output, shared docs, and scheduler examples.",
		TeamDescription:      "A demo team that prepares a clear onboarding note for a new teammate while demonstrating agent handoff and human review.",
		TeamGoals: []string{
			"Show how a concrete work item moves through multiple agents.",
			"Keep human intervention explicit and lightweight.",
			"Store the onboarding note and handoff artifacts in workspace docs.",
		},
		TeamPrompt: `# Collaboration Demo Team

You demonstrate coordination through one concrete but universal task: preparing a short onboarding note for a new teammate.

Every agent should:
- Read upstream workflow inputs before acting.
- Produce structured workflow outputs.
- Keep the task understandable to non-technical users.
- Make handoffs clear enough that the next actor does not need the human to repeat context.`,
		ProjectDescription: "A small onboarding-note project that proves Multigent can route one concrete task across agents and humans.",
		ProjectPrompt: `# hello-world-relay

This project demonstrates Multigent coordination with a concrete output.

The goal is to prepare a short onboarding note for a new teammate:
1. Draft the first welcome note and explain the collaboration goal.
2. Let a human approve it or request changes.
3. Turn the approved note into practical first steps.
4. Record the final onboarding note and handoff history.
5. Let the human make a final decision.`,
		Roles: map[string]exampleRoleText{
			"greeter": {
				Description: "Drafts the first welcome note and prepares the first handoff.",
				Prompt: `You start onboarding-note collaboration tasks.

Focus on clarity:
- Explain the purpose of Multigent and this demo in one short welcome document.
- Create durable docs for any non-trivial output.
- Hand off with enough context for the next agent to continue without asking the human to repeat themselves.
- Keep the tone simple enough for a non-technical teammate to understand.`,
			},
			"responder": {
				Description: "Reads the approved welcome note and turns it into practical first steps.",
				Prompt: `You turn approved onboarding notes into usable first-step guidance.

Focus on continuity:
- Read the previous step output before acting.
- Preserve the original intent.
- Add practical next steps and a clean handoff for Nora.
- If anything is ambiguous, make the uncertainty explicit instead of inventing context.`,
			},
			"recorder": {
				Description: "Turns the onboarding-note relay into a concise final record.",
				Prompt: `You record onboarding and collaboration outcomes.

Focus on traceability:
- Summarize the final onboarding note and what each participant contributed.
- Store final notes in docs and return doc IDs in workflow outputs.
- Point out where a human intervened and whether the intervention could be reduced next time.`,
			},
		},
		DocTitle:       "New Teammate Onboarding Relay Guide",
		DocDescription: "How to run the built-in onboarding-note collaboration relay.",
		DocBody: `# New Teammate Onboarding Relay Guide

This workspace uses a concrete, universal task: prepare a short onboarding note for a new teammate who is seeing Multigent for the first time.

The loop is simple:

1. One agent drafts the welcome note.
2. A human reviews it or sends it back.
3. Another agent turns the approved note into practical first steps.
4. A final agent records the result and what was learned.
5. The human confirms the final note.

Before running the demo, configure at least one model account and attach it to the three demo agents. Then open the seeded task in the project task list and wake the first agent.

The Schedule page also contains examples:

- Task-triggered heartbeat for Lina.
- Task/message-triggered heartbeat for Mira.
- A weekday queue review cron.
- A Friday onboarding summary cron.`,
		WorkflowName:        "New Teammate Onboarding Relay",
		WorkflowDescription: "A minimal workflow that produces a real onboarding note while demonstrating agent handoff, human review loops, structured outputs, and document references.",
		StepText: exampleStepText(
			"Draft Welcome Note", "Create the first welcome note and a handoff note. Longer content must be stored as docs and returned as doc IDs.",
			"Review Welcome Note", "Review whether the welcome note is clear enough for a new teammate. Approve it or request changes with concrete comments.",
			"Add First Steps", "Read the approved welcome note and add practical first steps with a new handoff.",
			"Record Final Note", "Create the final onboarding note and a concise record of the whole relay.",
			"Final Review", "Confirm whether the final onboarding note is useful and whether the collaboration loop worked.",
		),
		FieldText: exampleFieldTextEN(),
		EdgeText: map[string]string{
			"review":            "review",
			"approved":          "approved",
			"changes_requested": "changes requested",
			"record":            "record",
			"final_review":      "final review",
		},
		TaskTitle: "Prepare a new teammate onboarding note",
		TaskDesc:  "Use the built-in workflow to create a short onboarding note through agent handoff and human review.",
		TaskPrompt: `Prepare a short onboarding note for a new teammate who is seeing Multigent for the first time.

Include:
1. What Multigent is in one paragraph.
2. How tasks, workflows, owners, and wakeups relate to each other.
3. When humans review and when agents continue automatically.
4. What the next agent should add.

Use docs for the required document outputs, then finish the current workflow step with structured output fields exactly as specified by the workflow context.`,
		Schedules: exampleScheduleText{
			GreeterWakeup: `# Wakeup Routine

When you wake up:
1. Check whether there is an active workflow task assigned to you.
2. Read the current workflow step, required inputs, and expected output fields.
3. If there is no task, do nothing except briefly report that the queue is empty.
4. If there is a task, create the required docs, then finish the step with exactly the structured output fields requested by the workflow.

Keep the onboarding note concrete, short, and easy to inspect.`,
			ResponderWakeup: `# Wakeup Routine

When you wake up:
1. Check for workflow tasks or unread messages.
2. Continue only when the upstream handoff is available.
3. Read upstream docIDs before responding.
4. Return the required structured outputs and do not invent missing context.`,
			RecorderWakeup: `# Wakeup Routine

When you wake up:
1. Check for completed upstream relay outputs.
2. Read the referenced docs and summarize what happened.
3. Store final notes as workspace docs.
4. Return docIDs in the workflow output fields.

Call out where human intervention happened and whether it could be reduced next time.`,
			DailyReviewTitle:    "Weekday demo queue review",
			DailyReviewPrompt:   "Review the example project queue. Summarize pending workflow tasks and whether any human review is blocking progress.",
			WeeklySummaryTitle:  "Friday onboarding summary",
			WeeklySummaryPrompt: "Summarize what happened in the example onboarding relay this week, including human interventions and possible process improvements.",
		},
		AgencyPrompt: `# Example Workspace

This workspace is a Multigent onboarding demo.

Rules:
- Keep outputs short, inspectable, and durable.
- Use workspace docs for non-trivial artifacts and return doc IDs in workflow outputs.
- Follow the active workflow step exactly.
- Humans review and coach; agents do the repeatable work.`,
	}
}

func exampleZHSpec(traditional bool) exampleLocaleSpec {
	if traditional {
		spec := exampleZHSpec(false)
		spec.WorkspaceDescription = "內建學習工作區，用一份新成員入門說明展示 Agent 交接、人類審核、結構化輸出、知識庫文檔與調度示例。"
		spec.TeamDescription = "用三個 Agent 和一個人類審核者，完成一份給新成員看的協作入門說明。"
		spec.TeamGoals = []string{"展示一件具體工作如何在多個 Agent 之間流轉。", "讓人類介入保持明確且輕量。", "把入門說明和交接產物沉澱到工作區知識庫。"}
		spec.DocTitle = "新成員入門說明接力指南"
		spec.DocDescription = "如何執行內建的新成員入門說明協作接力。"
		return spec
	}
	return exampleLocaleSpec{
		WorkspaceDescription: "内置学习工作区，用一份新成员入门说明展示 Agent 交接、人类审核、结构化输出、知识库文档与调度示例。",
		TeamDescription:      "用三个 Agent 和一个人类审核者，完成一份给新成员看的协作入门说明。",
		TeamGoals:            []string{"展示一件具体工作如何在多个 Agent 之间流转。", "让人类介入保持明确且轻量。", "把入门说明和交接产物沉淀到工作区知识库。"},
		TeamPrompt: `# 协作演示团队

你们通过一个具体但通用的任务来演示协作机制：为第一次进入 Multigent 的新成员准备一份简短入门说明。

每个 Agent 都应该：
- 先读取上游流程输入，再开始行动。
- 按流程要求输出结构化字段。
- 让完全不懂技术的用户也能看懂。
- 交接信息要足够清楚，避免下一个参与者要求人类重复上下文。`,
		ProjectDescription: "一个新成员入门说明项目，用来证明 Multigent 可以让具体任务在人和多个 Agent 之间流转。",
		ProjectPrompt: `# hello-world-relay

这个项目用一个具体产物演示 Multigent 协作机制。

目标是为第一次进入示例工作区的新成员准备一份简短入门说明：
1. 起草欢迎说明，并解释这次协作的目标。
2. 让人类审核通过或打回。
3. 由另一个 Agent 把通过后的说明整理成可执行上手步骤。
4. 记录最终说明和完整交接过程。
5. 由人类做最终确认。`,
		Roles: map[string]exampleRoleText{
			"greeter": {
				Description: "起草第一版欢迎说明，并准备第一份交接说明。",
				Prompt: `你负责发起新成员入门说明的协作任务。

重点：
- 用一份简短文档说明 Multigent 是什么，以及这个示例任务要验证什么。
- 复杂内容要创建为知识库文档。
- 交接时提供足够上下文，让下一个 Agent 不需要人类重复解释。
- 保持表达简单，让不懂技术的新成员也能看懂。`,
			},
			"responder": {
				Description: "读取已通过的欢迎说明，并整理成可执行的上手步骤。",
				Prompt: `你负责把已通过的欢迎说明变成可操作的上手指引。

重点：
- 行动前先读取上一步输出。
- 保留原始意图。
- 补充新成员最应该先做的 3-5 个步骤，并为记录者准备清晰交接。
- 如果信息不明确，直接标注不确定，不要编造上下文。`,
			},
			"recorder": {
				Description: "把入门说明和接力过程整理成最终记录。",
				Prompt: `你负责记录入门说明和协作结果。

重点：
- 整理最终可读的新成员入门说明。
- 总结每个参与者贡献了什么。
- 把最终记录写入知识库，并在流程输出中返回 docID。
- 指出人类在哪里介入，以及下次是否能减少这种介入。`,
			},
		},
		DocTitle:       "新成员入门说明接力指南",
		DocDescription: "如何运行内置的新成员入门说明协作接力。",
		DocBody: `# 新成员入门说明接力指南

这个工作区使用一个具体、通用的小任务：为第一次看到 Multigent 的新成员准备一份简短入门说明。

这个闭环是：

1. 一个 Agent 起草欢迎说明。
2. 人类审核，通过或打回。
3. 另一个 Agent 基于结构化上游输出整理上手步骤。
4. 最后一个 Agent 记录最终说明和协作过程。
5. 人类确认最终结果。

运行演示前，先配置至少一个模型账号，并绑定到三个演示 Agent。然后打开项目任务列表中的初始任务，唤醒第一个 Agent。

计划页还内置了几个示例：

- Lina 的“有任务就触发”心跳。
- Mira 的“有任务或消息就触发”心跳。
- 工作日每日队列回顾 cron。
- 周五入门说明总结 cron。`,
		WorkflowName:        "新成员入门说明接力",
		WorkflowDescription: "一个最小流程，用具体产物演示 Agent 交接、人类审核循环、结构化输出和 docID 文档引用。",
		StepText: exampleStepText(
			"起草欢迎说明", "创建第一版欢迎说明和交接说明。较长内容必须写入知识库文档，并在输出中返回 docID。",
			"审核欢迎说明", "审核欢迎说明是否足够清楚，是否能让新成员理解下一步。通过或填写具体意见后打回。",
			"补充上手步骤", "读取已通过的欢迎说明，并补充 3-5 个可执行的上手步骤和新的交接说明。",
			"整理最终说明", "整理最终入门说明、接力过程和经验教训。",
			"最终确认", "确认最终入门说明是否可用，以及这次协作闭环是否跑通。",
		),
		FieldText: exampleFieldTextZH(),
		EdgeText: map[string]string{
			"review":            "审核",
			"approved":          "已通过",
			"changes_requested": "请求修改",
			"record":            "记录",
			"final_review":      "最终审核",
		},
		TaskTitle:  "给新成员准备一份 Multigent 协作入门说明",
		TaskDesc:   "让三个 Agent 协作产出一份给新成员看的入门说明，中间经过人工审核，最终沉淀到知识库。",
		TaskPrompt: "目标：为第一次进入 Example Workspace 的新成员，准备一份 3-5 分钟能看懂的协作入门说明。\n\n需要包含：\n1. Multigent 是什么，用一句话和一小段解释清楚。\n2. 流程、任务、负责人、唤醒分别是什么，它们之间是什么关系。\n3. 这个示例项目怎么跑起来：配置模型账号、绑定 Agent、唤醒当前负责人、在工作台审核。\n4. 人类什么时候审核，Agent 什么时候继续。\n5. 下一位 Agent 接力时应该补充什么。\n\n较长内容必须写入知识库，并在流程输出中返回 docID。按当前流程节点要求提交结构化输出字段。",
		Schedules: exampleScheduleText{
			GreeterWakeup: `# Wakeup Routine

每次被唤醒时：
1. 检查是否有分配给你的活动流程任务。
2. 读取当前流程节点、必需输入和期望输出字段。
3. 如果没有任务，只需要简短说明队列为空。
4. 如果有任务，先创建必需文档，再严格按流程要求提交结构化输出字段。

保持入门说明具体、简短、方便检查。`,
			ResponderWakeup: `# Wakeup Routine

每次被唤醒时：
1. 检查是否有流程任务或未读消息。
2. 只有拿到上游欢迎说明和交接内容时才继续。
3. 先读取上游 docID 对应的文档，再开始回应。
4. 输出新成员下一步应该怎么做，并返回流程要求的结构化输出，不要编造缺失上下文。`,
			RecorderWakeup: `# Wakeup Routine

每次被唤醒时：
1. 检查是否已有上游入门说明输出。
2. 读取引用的文档，总结发生了什么。
3. 把最终记录写入工作区知识库。
4. 在流程输出字段中返回 docID。

需要指出人类在哪里介入，以及下次是否可以减少这类介入。`,
			DailyReviewTitle:    "工作日演示队列回顾",
			DailyReviewPrompt:   "回顾示例项目队列，简要总结待处理流程任务，以及是否有人类审核正在阻塞进展。",
			WeeklySummaryTitle:  "周五入门说明总结",
			WeeklySummaryPrompt: "总结本周示例入门说明接力发生了什么，包括人类介入点和可改进的流程。",
		},
		AgencyPrompt: `# Example Workspace

这是一个 Multigent 入门演示工作区。

规则：
- 输出要短、可检查、可沉淀。
- 复杂产物写入工作区知识库，并在流程输出中返回 docID。
- 严格跟随当前流程节点。
- 人类负责审核和调教，Agent 负责可重复执行的工作。`,
	}
}

func exampleJASpec() exampleLocaleSpec {
	spec := exampleENSpec()
	spec.WorkspaceDescription = "Agent handoff、human review、structured output、docs、scheduler examples を中立的に確認する built-in learning workspace です。"
	spec.TeamDescription = "Agent-to-agent relay と human review を練習する中立的な demo team です。"
	spec.TeamGoals = []string{"複数 Agent 間で作業が流れる様子を示す。", "人の介入を明示的かつ軽量にする。", "成果物を workspace docs に保存する。"}
	spec.DocTitle = "New Teammate Onboarding Relay Guide"
	spec.DocDescription = "Built-in Hello World collaboration relay の実行方法。"
	return spec
}

func exampleStepText(gTitle, gDesc, rTitle, rDesc, respTitle, respDesc, recTitle, recDesc, finalTitle, finalDesc string) map[string]struct {
	Title       string
	Description string
} {
	return map[string]struct {
		Title       string
		Description string
	}{
		"greeting":        {Title: gTitle, Description: gDesc},
		"greeting_review": {Title: rTitle, Description: rDesc},
		"response":        {Title: respTitle, Description: respDesc},
		"record":          {Title: recTitle, Description: recDesc},
		"final_review":    {Title: finalTitle, Description: finalDesc},
	}
}

func exampleFieldTextEN() map[string]string {
	return map[string]string{
		"greeting_doc_id":                   "Doc ID containing the welcome note and purpose of the demo.",
		"handoff_note_doc_id":               "Doc ID containing the handoff note for the next actor.",
		"summary":                           "One-sentence summary of this step.",
		"input_greeting_doc_id":             "Welcome-note document from the previous step.",
		"input_handoff_note_doc_id":         "Handoff document from the previous step.",
		"decision":                          "approve or request_changes.",
		"comments":                          "Review comments. Required even when approving.",
		"approved_greeting_doc_id":          "Approved welcome-note document.",
		"approved_handoff_note_doc_id":      "Approved handoff document.",
		"review_comments":                   "Human review comments from the approval step.",
		"response_doc_id":                   "Doc ID containing Mira's contribution.",
		"next_handoff_doc_id":               "Doc ID containing the handoff for Nora.",
		"input_response_doc_id":             "First-steps document from Mira.",
		"input_next_handoff_doc_id":         "Handoff document for Nora.",
		"collaboration_record_doc_id":       "Doc ID containing the final collaboration record.",
		"learnings_doc_id":                  "Doc ID containing lessons learned and possible process improvements.",
		"input_collaboration_record_doc_id": "Final collaboration record document.",
		"input_learnings_doc_id":            "Lessons learned document.",
		"final_comments":                    "Final review comments. Required even when approving.",
	}
}

func exampleFieldTextZH() map[string]string {
	return map[string]string{
		"greeting_doc_id":                   "包含欢迎说明和示例目标的文档 docID。",
		"handoff_note_doc_id":               "给下一个参与者的交接说明 docID。",
		"summary":                           "本步骤的一句话总结。",
		"input_greeting_doc_id":             "上一步产生的欢迎说明文档。",
		"input_handoff_note_doc_id":         "上一步产生的交接文档。",
		"decision":                          "approve 或 request_changes。",
		"comments":                          "审核意见。即使通过也要填写。",
		"approved_greeting_doc_id":          "已通过的欢迎说明文档。",
		"approved_handoff_note_doc_id":      "已通过的交接文档。",
		"review_comments":                   "人类审核通过时给出的评论。",
		"response_doc_id":                   "包含新成员上手步骤的文档 docID。",
		"next_handoff_doc_id":               "给记录者的下一份交接说明 docID。",
		"input_response_doc_id":             "上手步骤文档。",
		"input_next_handoff_doc_id":         "给记录者的交接文档。",
		"collaboration_record_doc_id":       "最终入门说明和协作记录文档 docID。",
		"learnings_doc_id":                  "经验教训和流程改进建议文档 docID。",
		"input_collaboration_record_doc_id": "最终入门说明和协作记录文档。",
		"input_learnings_doc_id":            "经验教训文档。",
		"final_comments":                    "最终审核意见。即使通过也要填写。",
	}
}

func exampleRoles(owner string, spec exampleLocaleSpec) []exampleRoleSeed {
	return []exampleRoleSeed{
		{
			Name:   "greeter",
			Role:   &entity.Role{Name: "greeter", Description: spec.Roles["greeter"].Description, Owners: []string{owner}},
			Prompt: spec.Roles["greeter"].Prompt,
		},
		{
			Name:   "responder",
			Role:   &entity.Role{Name: "responder", Description: spec.Roles["responder"].Description, Owners: []string{owner}},
			Prompt: spec.Roles["responder"].Prompt,
		},
		{
			Name:   "recorder",
			Role:   &entity.Role{Name: "recorder", Description: spec.Roles["recorder"].Description, Owners: []string{owner}},
			Prompt: spec.Roles["recorder"].Prompt,
		},
	}
}

func exampleAgents(owner string) []*entity.AgentMeta {
	now := time.Now().UTC()
	return []*entity.AgentMeta{
		exampleAgent(exampleGreeterAgent, "greeter", owner, now),
		exampleAgent(exampleResponderAgent, "responder", owner, now),
		exampleAgent(exampleRecorderAgent, "recorder", owner, now),
	}
}

func exampleAgent(name, role, owner string, now time.Time) *entity.AgentMeta {
	return &entity.AgentMeta{
		Name:          name,
		Project:       exampleProjectName,
		Team:          exampleTeamName,
		Role:          role,
		Model:         entity.ModelClaudeCode,
		HiredAt:       now,
		Avatar:        avatar.RandomURL(exampleProjectName, name),
		Owners:        []string{owner},
		RuntimeMode:   "cloud",
		AutonomyLevel: "L1",
		Sandbox: &entity.SandboxConfig{
			Provider: entity.SandboxDocker,
			Docker: &entity.DockerSandboxConfig{
				NetworkMode: "bridge",
			},
		},
	}
}

func seedExampleDocs(root, username string, spec exampleLocaleSpec) error {
	ds := store.NewDocsStore(root)
	return ds.AddManagedContent(&store.DocEntry{
		Title:       spec.DocTitle,
		Index:       "examples/hello-world-relay",
		CreatedBy:   username,
		Tags:        []string{"example", "tour"},
		Description: spec.DocDescription,
	}, spec.DocBody, "hello-world-relay-guide.md")
}

func seedExampleAgentWorkers(db controldb.Store, workspaceID string, agents []*entity.AgentMeta, spec exampleLocaleSpec) error {
	if db == nil {
		return fmt.Errorf("control database unavailable")
	}
	heartbeats := map[string]*entity.HeartbeatConfig{
		exampleGreeterAgent: {
			Enabled:          false,
			Interval:         "30m",
			WakeupPreset:     "require_tasks",
			WakeupPrompt:     spec.Schedules.GreeterWakeup,
			Triggers:         []entity.TriggerType{entity.TriggerOnTask, entity.TriggerOnMessage},
			TriggerDebounce:  "1m",
			SessionScope:     entity.SessionScopeCycle,
			MaxTasksPerCycle: 1,
			Jitter:           "2m",
		},
		exampleResponderAgent: {
			Enabled:          false,
			Interval:         "1h",
			WakeupPreset:     "require_any",
			WakeupPrompt:     spec.Schedules.ResponderWakeup,
			Triggers:         []entity.TriggerType{entity.TriggerOnTask, entity.TriggerOnMessage},
			TriggerDebounce:  "2m",
			SessionScope:     entity.SessionScopeCycle,
			MaxTasksPerCycle: 2,
			Jitter:           "3m",
		},
		exampleRecorderAgent: {
			Enabled:          false,
			Interval:         "2h",
			WakeupPreset:     "require_any",
			WakeupPrompt:     spec.Schedules.RecorderWakeup,
			Triggers:         []entity.TriggerType{entity.TriggerOnTask, entity.TriggerOnMessage},
			TriggerDebounce:  "5m",
			SessionScope:     entity.SessionScopeCycle,
			MaxTasksPerCycle: 2,
			Jitter:           "5m",
		},
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, meta := range agents {
		if meta == nil {
			continue
		}
		workerID := exampleAgentWorkerID(meta.Name)
		runtimeConfig := agentWorkerRuntimeConfig{Sandbox: meta.Sandbox, AddDirs: meta.AddDirs}
		worker := controldb.AgentWorker{
			ID:                  workerID,
			WorkspaceID:         workspaceID,
			Name:                meta.Name,
			DisplayName:         meta.Name,
			Avatar:              meta.Avatar,
			Status:              "available",
			Model:               string(meta.Model),
			DefaultRuntimeMode:  meta.RuntimeMode,
			ScheduleJSON:        marshalExampleHeartbeat(heartbeats[meta.Name]),
			AttentionPolicyJSON: "{}",
			MemoryPolicyJSON:    "{}",
			SkillsJSON:          "[]",
			RuntimeConfigJSON:   encodeAgentWorkerRuntimeConfig(runtimeConfig),
			PrimarySessionID:    "sess_" + randomHex(16),
			CreatedAt:           now,
			UpdatedAt:           now,
		}
		if err := db.UpsertAgentWorker(worker); err != nil {
			return fmt.Errorf("upsert worker %s: %w", meta.Name, err)
		}
		membership := controldb.ProjectMembership{
			ID:               "pm_" + stableExampleID(exampleProjectName+"-"+meta.Name),
			WorkspaceID:      workspaceID,
			ProjectID:        exampleProjectName,
			MemberType:       "agent_worker",
			MemberID:         worker.ID,
			Role:             meta.Role,
			Title:            meta.Name,
			PermissionsJSON:  "[]",
			AutoPickTasks:    true,
			AttentionEnabled: true,
			PriorityWeight:   1,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		if err := db.UpsertProjectMembership(membership); err != nil {
			return fmt.Errorf("upsert project membership %s: %w", meta.Name, err)
		}
	}
	return nil
}

func marshalExampleHeartbeat(hb *entity.HeartbeatConfig) string {
	if hb == nil {
		return "{}"
	}
	copy := *hb
	copy.Triggers = normalizeHeartbeatTriggers(copy.Triggers)
	raw, err := json.Marshal(&copy)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func exampleAgentWorkerID(name string) string {
	return "aw_" + stableExampleID(exampleProjectName+"-"+name)
}

func stableExampleID(raw string) string {
	id := strings.ToLower(strings.TrimSpace(raw))
	id = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' {
			return r
		}
		if r >= '0' && r <= '9' {
			return r
		}
		return '_'
	}, id)
	id = strings.Trim(id, "_")
	if id == "" {
		return randomHex(8)
	}
	return id
}

func exampleWorkflowDefinition(spec exampleLocaleSpec) entity.WorkflowDefinition {
	field := func(name, desc string) entity.WorkflowField {
		return entity.WorkflowField{Name: name, Description: desc}
	}
	step := func(id, typ, title, desc, role, color string, x, y int, inputs, outputs []entity.WorkflowField) entity.WorkflowStep {
		return entity.WorkflowStep{
			ID:           id,
			Type:         typ,
			Title:        title,
			Description:  desc,
			ActorRole:    role,
			InputFields:  inputs,
			OutputFields: outputs,
			Position:     entity.WorkflowPosition{X: x, Y: y},
			Config:       map[string]string{"color": color},
		}
	}
	edge := func(id, from, to, label string, condition *entity.WorkflowEdgeCondition, mapping map[string]string, def bool) entity.WorkflowEdge {
		return entity.WorkflowEdge{ID: id, From: from, To: to, Label: label, Condition: condition, InputMapping: mapping, IsDefault: def}
	}
	cond := func(field, value string) *entity.WorkflowEdgeCondition {
		return &entity.WorkflowEdgeCondition{Field: field, Operator: "eq", Value: value}
	}

	return entity.WorkflowDefinition{
		ID:          exampleWorkflowID,
		Name:        spec.WorkflowName,
		Description: spec.WorkflowDescription,
		Version:     1,
		Scope:       "workspace",
		StartStepID: "greeting",
		Steps: []entity.WorkflowStep{
			step("greeting", "agent_task", spec.StepText["greeting"].Title, spec.StepText["greeting"].Description, "greeter", "sky", 80, 120, nil, []entity.WorkflowField{
				field("greeting_doc_id", spec.FieldText["greeting_doc_id"]),
				field("handoff_note_doc_id", spec.FieldText["handoff_note_doc_id"]),
				field("summary", spec.FieldText["summary"]),
			}),
			step("greeting_review", "human_review", spec.StepText["greeting_review"].Title, spec.StepText["greeting_review"].Description, "greetingReview", "amber", 460, 120, []entity.WorkflowField{
				field("greeting_doc_id", spec.FieldText["input_greeting_doc_id"]),
				field("handoff_note_doc_id", spec.FieldText["input_handoff_note_doc_id"]),
			}, []entity.WorkflowField{
				field("decision", spec.FieldText["decision"]),
				field("comments", spec.FieldText["comments"]),
			}),
			step("response", "agent_task", spec.StepText["response"].Title, spec.StepText["response"].Description, "responder", "emerald", 840, 120, []entity.WorkflowField{
				field("greeting_doc_id", spec.FieldText["approved_greeting_doc_id"]),
				field("handoff_note_doc_id", spec.FieldText["approved_handoff_note_doc_id"]),
				field("review_comments", spec.FieldText["review_comments"]),
			}, []entity.WorkflowField{
				field("response_doc_id", spec.FieldText["response_doc_id"]),
				field("next_handoff_doc_id", spec.FieldText["next_handoff_doc_id"]),
				field("summary", spec.FieldText["summary"]),
			}),
			step("record", "agent_task", spec.StepText["record"].Title, spec.StepText["record"].Description, "recorder", "violet", 1220, 120, []entity.WorkflowField{
				field("response_doc_id", spec.FieldText["input_response_doc_id"]),
				field("next_handoff_doc_id", spec.FieldText["input_next_handoff_doc_id"]),
			}, []entity.WorkflowField{
				field("collaboration_record_doc_id", spec.FieldText["collaboration_record_doc_id"]),
				field("learnings_doc_id", spec.FieldText["learnings_doc_id"]),
				field("summary", spec.FieldText["summary"]),
			}),
			step("final_review", "human_review", spec.StepText["final_review"].Title, spec.StepText["final_review"].Description, "finalReview", "slate", 1600, 120, []entity.WorkflowField{
				field("collaboration_record_doc_id", spec.FieldText["input_collaboration_record_doc_id"]),
				field("learnings_doc_id", spec.FieldText["input_learnings_doc_id"]),
			}, []entity.WorkflowField{
				field("decision", spec.FieldText["decision"]),
				field("comments", spec.FieldText["final_comments"]),
			}),
		},
		Edges: []entity.WorkflowEdge{
			edge("e-greeting-review", "greeting", "greeting_review", spec.EdgeText["review"], nil, map[string]string{
				"greeting_doc_id":     "$output.greeting_doc_id",
				"handoff_note_doc_id": "$output.handoff_note_doc_id",
			}, true),
			edge("e-review-approve", "greeting_review", "response", spec.EdgeText["approved"], cond("decision", "approve"), map[string]string{
				"greeting_doc_id":     "$input.greeting_doc_id",
				"handoff_note_doc_id": "$input.handoff_note_doc_id",
				"review_comments":     "$output.comments",
			}, false),
			edge("e-review-rework", "greeting_review", "greeting", spec.EdgeText["changes_requested"], cond("decision", "request_changes"), map[string]string{
				"review_comments": "$output.comments",
			}, false),
			edge("e-response-record", "response", "record", spec.EdgeText["record"], nil, map[string]string{
				"response_doc_id":     "$output.response_doc_id",
				"next_handoff_doc_id": "$output.next_handoff_doc_id",
			}, true),
			edge("e-record-final", "record", "final_review", spec.EdgeText["final_review"], nil, map[string]string{
				"collaboration_record_doc_id": "$output.collaboration_record_doc_id",
				"learnings_doc_id":            "$output.learnings_doc_id",
			}, true),
			edge("e-final-rework", "final_review", "record", spec.EdgeText["changes_requested"], cond("decision", "request_changes"), map[string]string{
				"review_comments": "$output.comments",
			}, false),
		},
	}
}

func exampleTask(username string, spec exampleLocaleSpec) *entity.Task {
	now := time.Now().UTC()
	return &entity.Task{
		ID:          entity.NewTaskID(),
		Title:       spec.TaskTitle,
		Type:        entity.TaskTypeChore,
		Priority:    2,
		Assignee:    exampleProjectName + "/" + exampleGreeterAgent,
		CreatedBy:   username,
		Status:      entity.TaskStatusPending,
		Description: spec.TaskDesc,
		Prompt:      spec.TaskPrompt,
		Labels:      []string{"example", "tour"},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}
