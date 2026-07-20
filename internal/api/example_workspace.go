package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/scaffold"
	"github.com/multigent/multigent/internal/store"
	"github.com/multigent/multigent/internal/taskstore"
	workflowstore "github.com/multigent/multigent/internal/workflow"
)

const (
	exampleWorkspaceName = "Example Workspace"
	exampleProjectName   = "hello-world-relay"
	exampleTeamName      = "collaboration-demo"
	exampleWorkflowID    = "wf-example-hello-world-relay"
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
	spec := exampleWorkspaceSpec(r.Header.Get("Accept-Language"))

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
	for _, agent := range exampleAgents(username) {
		if err := os.MkdirAll(st.AgentDir(exampleProjectName, agent.Name), 0o755); err != nil {
			return fmt.Errorf("create example agent dir %s: %w", agent.Name, err)
		}
		if err := st.SaveAgentMeta(exampleProjectName, agent.Name, agent); err != nil {
			return fmt.Errorf("save example agent %s: %w", agent.Name, err)
		}
	}
	if err := seedExampleDocs(root, username, spec); err != nil {
		return fmt.Errorf("seed example docs: %w", err)
	}
	if err := seedExampleWakeupPrompts(st, spec); err != nil {
		return fmt.Errorf("seed example wakeup prompts: %w", err)
	}
	if err := seedExampleSchedules(ts, spec); err != nil {
		return fmt.Errorf("seed example schedules: %w", err)
	}
	wfStore := workflowstore.NewStore(db, workspaceID)
	def := exampleWorkflowDefinition(spec)
	if err := wfStore.SaveDefinition(&def); err != nil {
		return fmt.Errorf("save example workflow definition: %w", err)
	}
	task := exampleTask(username, spec)
	if err := ts.AddTask(exampleProjectName, "greeter-agent", task); err != nil {
		return fmt.Errorf("add example task: %w", err)
	}
	_, _, err := wfStore.StartRun(exampleProjectName, task.ID, def.ID, map[string]entity.WorkflowActorBinding{
		"greeter":        {Type: "agent", ID: "greeter-agent"},
		"greetingReview": {Type: "human", ID: username},
		"responder":      {Type: "agent", ID: "responder-agent"},
		"recorder":       {Type: "agent", ID: "recorder-agent"},
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

func exampleWorkspaceSpec(acceptLanguage string) exampleLocaleSpec {
	lang := "en"
	lower := strings.ToLower(acceptLanguage)
	switch {
	case strings.Contains(lower, "zh-tw") || strings.Contains(lower, "zh-hk") || strings.Contains(lower, "zh-mo"):
		lang = "zh-TW"
	case strings.Contains(lower, "zh"):
		lang = "zh-CN"
	case strings.Contains(lower, "ja"):
		lang = "ja"
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
		WorkspaceDescription: "A built-in learning workspace that demonstrates agent handoff, human review, structured workflow output, shared docs, and scheduler examples without assuming a specific industry.",
		TeamDescription:      "A neutral demo team for practicing agent-to-agent relay and human review.",
		TeamGoals: []string{
			"Show how work moves through multiple agents.",
			"Keep human intervention explicit and lightweight.",
			"Store durable outputs in workspace docs.",
		},
		TeamPrompt: `# Collaboration Demo Team

You demonstrate coordination, not a vertical business process.

Every agent should:
- Read upstream workflow inputs before acting.
- Produce structured workflow outputs.
- Avoid assuming a software, sales, marketing, design, or operations scenario.
- Make handoffs clear enough that the next actor does not need the human to repeat context.`,
		ProjectDescription: "A Hello World relay project that proves Multigent can route one task across agents and humans.",
		ProjectPrompt: `# hello-world-relay

This project exists only to demonstrate Multigent coordination.

The goal is to complete one simple relay:
1. Start with a greeting.
2. Let a human approve or request changes.
3. Continue the relay.
4. Record what happened.
5. Let the human make a final decision.`,
		Roles: map[string]exampleRoleText{
			"greeter": {
				Description: "Starts a clear, friendly relay and prepares the first handoff.",
				Prompt: `You start neutral collaboration relays.

Focus on clarity:
- State the purpose of the relay in one short document.
- Create durable docs for any non-trivial output.
- Hand off with enough context for the next agent to continue without asking the human to repeat themselves.
- Keep the tone simple and universal. Do not assume the workspace is for software, sales, marketing, or any single department.`,
			},
			"responder": {
				Description: "Reads the upstream handoff and responds with the next useful contribution.",
				Prompt: `You continue collaboration relays.

Focus on continuity:
- Read the previous step output before acting.
- Preserve the original intent.
- Add one useful response and a clean handoff for the recorder.
- If anything is ambiguous, make the uncertainty explicit instead of inventing context.`,
			},
			"recorder": {
				Description: "Turns the relay into a concise collaboration record.",
				Prompt: `You record collaboration outcomes.

Focus on traceability:
- Summarize what each participant contributed.
- Store final notes in docs and return doc IDs in workflow outputs.
- Point out where a human intervened and whether the intervention could be reduced next time.`,
			},
		},
		DocTitle:       "Hello World Relay Guide",
		DocDescription: "How to run the built-in Hello World collaboration relay.",
		DocBody: `# Hello World Relay Guide

This workspace is intentionally neutral. It exists to prove a simple loop:

1. One agent starts the work.
2. A human reviews or sends it back.
3. Another agent continues from structured upstream output.
4. A final agent records what happened.
5. The human confirms the result.

Before running the demo, configure at least one model account and attach it to the three demo agents. Then open the seeded task in the project task list and wake the first agent.

The Schedule page also contains examples:

- Task-triggered heartbeat for the greeter agent.
- Task/message-triggered heartbeat for the responder agent.
- A weekday daily review cron.
- A Friday weekly summary cron.`,
		WorkflowName:        "Hello World Collaboration Relay",
		WorkflowDescription: "A minimal workflow that demonstrates agent handoff, human review loops, structured outputs, and document references without assuming a business domain.",
		StepText: exampleStepText(
			"Start Greeting", "Create the first greeting and a handoff note. Longer content must be stored as docs and returned as doc IDs.",
			"Review Greeting", "Review whether the greeting is clear enough. Approve it or request changes with concrete comments.",
			"Continue Relay", "Read the approved greeting and add the next response with a new handoff.",
			"Record Collaboration", "Create a concise record of the whole relay and lessons learned.",
			"Final Review", "Confirm whether the relay demonstrates the expected collaboration loop.",
		),
		FieldText: exampleFieldTextEN(),
		EdgeText: map[string]string{
			"review":            "review",
			"approved":          "approved",
			"changes_requested": "changes requested",
			"record":            "record",
			"final_review":      "final review",
		},
		TaskTitle: "Complete a Hello World collaboration relay",
		TaskDesc:  "Use the built-in workflow to pass one small piece of work from one agent to another, with human review in the middle.",
		TaskPrompt: `Run the Hello World collaboration relay.

Keep the output neutral and easy to inspect. Use docs for the required document outputs, then finish the current workflow step with structured output fields exactly as specified by the workflow context.`,
		Schedules: exampleScheduleText{
			GreeterWakeup: `# Wakeup Routine

When you wake up:
1. Check whether there is an active workflow task assigned to you.
2. Read the current workflow step, required inputs, and expected output fields.
3. If there is no task, do nothing except briefly report that the queue is empty.
4. If there is a task, create the required docs, then finish the step with exactly the structured output fields requested by the workflow.

Keep the relay neutral and easy to inspect.`,
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
			WeeklySummaryTitle:  "Friday relay summary",
			WeeklySummaryPrompt: "Summarize what happened in the example relay this week, including human interventions and possible process improvements.",
		},
		AgencyPrompt: `# Example Workspace

This workspace is a neutral Multigent demo.

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
		spec.WorkspaceDescription = "內建學習工作區，用中性的 Hello World 接力展示 Agent 交接、人類審核、結構化輸出、知識庫文檔與調度示例。"
		spec.TeamDescription = "用於練習 Agent 接力和人類審核的中性示範團隊。"
		spec.TeamGoals = []string{"展示工作如何在多個 Agent 之間流轉。", "讓人類介入保持明確且輕量。", "把長期產物沉澱到工作區知識庫。"}
		spec.DocTitle = "Hello World 接力指南"
		spec.DocDescription = "如何執行內建的 Hello World 協作接力。"
		return spec
	}
	return exampleLocaleSpec{
		WorkspaceDescription: "内置学习工作区，用中性的 Hello World 接力展示 Agent 交接、人类审核、结构化输出、知识库文档与调度示例。",
		TeamDescription:      "用于练习 Agent 接力和人类审核的中性演示团队。",
		TeamGoals:            []string{"展示工作如何在多个 Agent 之间流转。", "让人类介入保持明确且轻量。", "把长期产物沉淀到工作区知识库。"},
		TeamPrompt: `# 协作演示团队

你们演示的是协作机制，不是某个垂直业务流程。

每个 Agent 都应该：
- 先读取上游流程输入，再开始行动。
- 按流程要求输出结构化字段。
- 不要假设这是软件、销售、市场、设计或运营场景。
- 交接信息要足够清楚，避免下一个参与者要求人类重复上下文。`,
		ProjectDescription: "一个 Hello World 接力项目，用来证明 Multigent 可以让任务在人和多个 Agent 之间流转。",
		ProjectPrompt: `# hello-world-relay

这个项目只用于演示 Multigent 协作机制。

目标是完成一个简单接力：
1. 发起一段问候。
2. 让人类审核通过或打回。
3. 由另一个 Agent 接力回应。
4. 记录整个过程。
5. 由人类做最终确认。`,
		Roles: map[string]exampleRoleText{
			"greeter": {
				Description: "发起清晰、友好的接力，并准备第一份交接说明。",
				Prompt: `你负责发起中性的协作接力。

重点：
- 用一份简短文档说明本次接力的目的。
- 复杂内容要创建为知识库文档。
- 交接时提供足够上下文，让下一个 Agent 不需要人类重复解释。
- 保持表达简单、通用，不要假设这是软件、销售、市场或任何单一部门场景。`,
			},
			"responder": {
				Description: "读取上游交接内容，并给出下一步有用回应。",
				Prompt: `你负责延续协作接力。

重点：
- 行动前先读取上一步输出。
- 保留原始意图。
- 添加一个有用回应，并为记录者准备清晰交接。
- 如果信息不明确，直接标注不确定，不要编造上下文。`,
			},
			"recorder": {
				Description: "把接力过程整理成简洁的协作记录。",
				Prompt: `你负责记录协作结果。

重点：
- 总结每个参与者贡献了什么。
- 把最终记录写入知识库，并在流程输出中返回 docID。
- 指出人类在哪里介入，以及下次是否能减少这种介入。`,
			},
		},
		DocTitle:       "Hello World 接力指南",
		DocDescription: "如何运行内置的 Hello World 协作接力。",
		DocBody: `# Hello World 接力指南

这个工作区刻意保持中性。它只用于证明一个简单闭环：

1. 一个 Agent 发起工作。
2. 人类审核，通过或打回。
3. 另一个 Agent 基于结构化上游输出继续。
4. 最后一个 Agent 记录过程。
5. 人类确认结果。

运行演示前，先配置至少一个模型账号，并绑定到三个演示 Agent。然后打开项目任务列表中的初始任务，唤醒第一个 Agent。

计划页还内置了几个示例：

- greeter-agent 的“有任务就触发”心跳。
- responder-agent 的“有任务或消息就触发”心跳。
- 工作日每日队列回顾 cron。
- 周五协作总结 cron。`,
		WorkflowName:        "Hello World 协作接力",
		WorkflowDescription: "一个最小流程，用来演示 Agent 交接、人类审核循环、结构化输出和 docID 文档引用，不绑定任何业务领域。",
		StepText: exampleStepText(
			"发起问候", "创建第一段问候和交接说明。较长内容必须写入知识库文档，并在输出中返回 docID。",
			"审核问候", "审核问候是否足够清楚。通过或填写具体意见后打回。",
			"继续接力", "读取已通过的问候，并添加下一段回应和新的交接说明。",
			"记录协作", "整理整个接力过程和经验教训。",
			"最终确认", "确认这个接力是否展示了预期的协作闭环。",
		),
		FieldText: exampleFieldTextZH(),
		EdgeText: map[string]string{
			"review":            "审核",
			"approved":          "已通过",
			"changes_requested": "请求修改",
			"record":            "记录",
			"final_review":      "最终审核",
		},
		TaskTitle:  "完成一次 Hello World 协作接力",
		TaskDesc:   "使用内置流程，让一个小任务从一个 Agent 流转到另一个 Agent，中间经过人类审核。",
		TaskPrompt: "运行 Hello World 协作接力。\n\n保持输出中性、简短、方便检查。必要文档写入知识库，并按当前流程节点要求提交结构化输出字段。",
		Schedules: exampleScheduleText{
			GreeterWakeup: `# Wakeup Routine

每次被唤醒时：
1. 检查是否有分配给你的活动流程任务。
2. 读取当前流程节点、必需输入和期望输出字段。
3. 如果没有任务，只需要简短说明队列为空。
4. 如果有任务，先创建必需文档，再严格按流程要求提交结构化输出字段。

保持接力内容中性、简短、方便检查。`,
			ResponderWakeup: `# Wakeup Routine

每次被唤醒时：
1. 检查是否有流程任务或未读消息。
2. 只有拿到上游交接内容时才继续。
3. 先读取上游 docID 对应的文档，再开始回应。
4. 返回流程要求的结构化输出，不要编造缺失上下文。`,
			RecorderWakeup: `# Wakeup Routine

每次被唤醒时：
1. 检查是否已有上游接力输出。
2. 读取引用的文档，总结发生了什么。
3. 把最终记录写入工作区知识库。
4. 在流程输出字段中返回 docID。

需要指出人类在哪里介入，以及下次是否可以减少这类介入。`,
			DailyReviewTitle:    "工作日演示队列回顾",
			DailyReviewPrompt:   "回顾示例项目队列，简要总结待处理流程任务，以及是否有人类审核正在阻塞进展。",
			WeeklySummaryTitle:  "周五接力总结",
			WeeklySummaryPrompt: "总结本周示例接力发生了什么，包括人类介入点和可改进的流程。",
		},
		AgencyPrompt: `# Example Workspace

这是一个中性的 Multigent 演示工作区。

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
	spec.DocTitle = "Hello World Relay Guide"
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
		"greeting_doc_id":                   "Doc ID containing the greeting and purpose of the relay.",
		"handoff_note_doc_id":               "Doc ID containing the handoff note for the next actor.",
		"summary":                           "One-sentence summary of this step.",
		"input_greeting_doc_id":             "Greeting document from the previous step.",
		"input_handoff_note_doc_id":         "Handoff document from the previous step.",
		"decision":                          "approve or request_changes.",
		"comments":                          "Review comments. Required even when approving.",
		"approved_greeting_doc_id":          "Approved greeting document.",
		"approved_handoff_note_doc_id":      "Approved handoff document.",
		"review_comments":                   "Human review comments from the approval step.",
		"response_doc_id":                   "Doc ID containing the responder contribution.",
		"next_handoff_doc_id":               "Doc ID containing the handoff for the recorder.",
		"input_response_doc_id":             "Responder contribution document.",
		"input_next_handoff_doc_id":         "Recorder handoff document.",
		"collaboration_record_doc_id":       "Doc ID containing the final collaboration record.",
		"learnings_doc_id":                  "Doc ID containing lessons learned and possible process improvements.",
		"input_collaboration_record_doc_id": "Final collaboration record document.",
		"input_learnings_doc_id":            "Lessons learned document.",
		"final_comments":                    "Final review comments. Required even when approving.",
	}
}

func exampleFieldTextZH() map[string]string {
	return map[string]string{
		"greeting_doc_id":                   "包含问候内容和接力目的的文档 docID。",
		"handoff_note_doc_id":               "给下一个参与者的交接说明 docID。",
		"summary":                           "本步骤的一句话总结。",
		"input_greeting_doc_id":             "上一步产生的问候文档。",
		"input_handoff_note_doc_id":         "上一步产生的交接文档。",
		"decision":                          "approve 或 request_changes。",
		"comments":                          "审核意见。即使通过也要填写。",
		"approved_greeting_doc_id":          "已通过的问候文档。",
		"approved_handoff_note_doc_id":      "已通过的交接文档。",
		"review_comments":                   "人类审核通过时给出的评论。",
		"response_doc_id":                   "接力回应内容的文档 docID。",
		"next_handoff_doc_id":               "给记录者的下一份交接说明 docID。",
		"input_response_doc_id":             "回应者贡献的文档。",
		"input_next_handoff_doc_id":         "给记录者的交接文档。",
		"collaboration_record_doc_id":       "最终协作记录文档 docID。",
		"learnings_doc_id":                  "经验教训和流程改进建议文档 docID。",
		"input_collaboration_record_doc_id": "最终协作记录文档。",
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
		exampleAgent("greeter-agent", "greeter", owner, now),
		exampleAgent("responder-agent", "responder", owner, now),
		exampleAgent("recorder-agent", "recorder", owner, now),
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

func seedExampleWakeupPrompts(st store.Store, spec exampleLocaleSpec) error {
	for agent, prompt := range map[string]string{
		"greeter-agent":   spec.Schedules.GreeterWakeup,
		"responder-agent": spec.Schedules.ResponderWakeup,
		"recorder-agent":  spec.Schedules.RecorderWakeup,
	} {
		wakeupDir := filepath.Join(st.AgentDir(exampleProjectName, agent), ".multigent", "context")
		if err := os.MkdirAll(wakeupDir, 0o755); err != nil {
			return fmt.Errorf("create wakeup dir %s: %w", agent, err)
		}
		if err := os.WriteFile(filepath.Join(wakeupDir, "wakeup.md"), []byte(prompt), 0o644); err != nil {
			return fmt.Errorf("write wakeup prompt %s: %w", agent, err)
		}
	}
	return nil
}

func seedExampleSchedules(ts taskstore.Store, spec exampleLocaleSpec) error {
	const wakeupFile = "@.multigent/context/wakeup.md"
	heartbeats := map[string]*entity.HeartbeatConfig{
		"greeter-agent": {
			Enabled:          true,
			Interval:         "30m",
			WakeupPreset:     "require_tasks",
			WakeupPrompt:     wakeupFile,
			Triggers:         []entity.TriggerType{entity.TriggerOnTask, entity.TriggerOnMessage},
			TriggerDebounce:  "1m",
			SessionScope:     entity.SessionScopeCycle,
			MaxTasksPerCycle: 1,
			Jitter:           "2m",
		},
		"responder-agent": {
			Enabled:          true,
			Interval:         "1h",
			WakeupPreset:     "require_any",
			WakeupPrompt:     wakeupFile,
			Triggers:         []entity.TriggerType{entity.TriggerOnTask, entity.TriggerOnMessage},
			TriggerDebounce:  "2m",
			SessionScope:     entity.SessionScopeCycle,
			MaxTasksPerCycle: 2,
			Jitter:           "3m",
		},
		"recorder-agent": {
			Enabled:          true,
			Interval:         "2h",
			WakeupPreset:     "require_any",
			WakeupPrompt:     wakeupFile,
			Triggers:         []entity.TriggerType{entity.TriggerOnTask, entity.TriggerOnMessage},
			TriggerDebounce:  "5m",
			SessionScope:     entity.SessionScopeCycle,
			MaxTasksPerCycle: 2,
			Jitter:           "5m",
		},
	}
	for agent, hb := range heartbeats {
		if err := ts.SaveHeartbeat(exampleProjectName, agent, hb); err != nil {
			return err
		}
	}
	if err := ts.SaveCrons(exampleProjectName, "greeter-agent", []*entity.Cron{
		{
			ID:           "example-daily-review",
			Title:        spec.Schedules.DailyReviewTitle,
			Schedule:     "0 9 * * 1-5",
			Enabled:      true,
			Prompt:       spec.Schedules.DailyReviewPrompt,
			SessionScope: string(entity.SessionScopeTask),
			Jitter:       "10m",
		},
	}); err != nil {
		return err
	}
	return ts.SaveCrons(exampleProjectName, "recorder-agent", []*entity.Cron{
		{
			ID:           "example-weekly-summary",
			Title:        spec.Schedules.WeeklySummaryTitle,
			Schedule:     "0 17 * * 5",
			Enabled:      true,
			Prompt:       spec.Schedules.WeeklySummaryPrompt,
			SessionScope: string(entity.SessionScopeTask),
			Jitter:       "15m",
		},
	})
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
		Assignee:    exampleProjectName + "/greeter-agent",
		CreatedBy:   username,
		Status:      entity.TaskStatusPending,
		Description: spec.TaskDesc,
		Prompt:      spec.TaskPrompt,
		Labels:      []string{"example", "tour"},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}
