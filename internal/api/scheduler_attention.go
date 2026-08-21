package api

import (
	"fmt"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
)

const (
	attentionWakeupTaskTitle     = "[wakeup] attention"
	attentionWakeupTaskCreatedBy = "heartbeat:attention"
)

func (s *Server) ensurePendingAttentionWakeupTask(workspaceID, project, agent string) (*entity.Task, []string, error) {
	if s == nil || s.ts == nil {
		return nil, nil, nil
	}
	section, ids, err := s.pendingAttentionWakeupSection(workspaceID, project, agent)
	if err != nil || strings.TrimSpace(section) == "" {
		return nil, nil, err
	}
	existing, err := s.ts.ListTasks(project, agent, entity.TaskStatusPending, entity.TaskStatusInProgress)
	if err != nil {
		return nil, nil, err
	}
	for _, task := range existing {
		if task == nil {
			continue
		}
		if task.Type == "wakeup" && strings.TrimSpace(task.CreatedBy) == attentionWakeupTaskCreatedBy {
			if strings.TrimSpace(section) != "" && strings.TrimSpace(task.Prompt) != strings.TrimSpace(section+s.attentionWakeupTaskPromptSuffix()) {
				task.Prompt = section + s.attentionWakeupTaskPromptSuffix()
				task.UpdatedAt = time.Now().UTC()
				_ = s.ts.UpdateTask(project, agent, task)
			}
			return task, ids, nil
		}
	}
	now := time.Now().UTC()
	task := &entity.Task{
		ID:        "t-" + now.Format("20060102") + "-" + randomRuntimeHex(3),
		Title:     attentionWakeupTaskTitle,
		Status:    entity.TaskStatusPending,
		Type:      "wakeup",
		Priority:  0,
		Prompt:    section + s.attentionWakeupTaskPromptSuffix(),
		CreatedBy: attentionWakeupTaskCreatedBy,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.ts.AddTask(project, agent, task); err != nil {
		return nil, nil, err
	}
	return task, ids, nil
}

type apiWakeupI18n struct {
	AttentionHeader string
	AttentionIntro  string
	AttentionHint   string
}

func (s *Server) pendingAttentionWakeupSection(workspaceID, project, agent string) (string, []string, error) {
	if s == nil || s.controlDB == nil || s.agentDirectory == nil {
		return "", nil, nil
	}
	workspaceID = strings.TrimSpace(workspaceID)
	project = strings.TrimSpace(project)
	agent = strings.TrimSpace(agent)
	if workspaceID == "" || project == "" || agent == "" {
		return "", nil, nil
	}
	resolved, ok, err := s.agentDirectory.ResolveProjectMailbox(workspaceID, project+"/"+agent)
	if err != nil || !ok {
		return "", nil, err
	}
	signals, err := s.controlDB.ListAttentionSignals(controldb.AttentionSignalFilter{
		WorkspaceID:   workspaceID,
		AgentWorkerID: resolved.Worker.ID,
		Statuses:      []string{"pending", "seen", "handling"},
		Limit:         20,
	})
	if err != nil || len(signals) == 0 {
		return "", nil, err
	}
	i18n := s.apiWakeupStrings()
	var b strings.Builder
	b.WriteString(i18n.AttentionHeader)
	b.WriteString(i18n.AttentionIntro)
	ids := make([]string, 0, len(signals))
	for _, signal := range signals {
		ids = append(ids, signal.ID)
		b.WriteString("---\n")
		b.WriteString(fmt.Sprintf("ID: `%s`\n", signal.ID))
		b.WriteString(fmt.Sprintf("Source: `%s`", signal.SourceKind))
		if signal.SourceChannel != "" {
			b.WriteString(fmt.Sprintf(" / `%s`", signal.SourceChannel))
		}
		b.WriteString("\n")
		if signal.Reason != "" {
			b.WriteString(fmt.Sprintf("Reason: `%s`\n", signal.Reason))
		}
		if signal.Priority != "" {
			b.WriteString(fmt.Sprintf("Priority: `%s`\n", signal.Priority))
		}
		if signal.ActorID != "" {
			b.WriteString(fmt.Sprintf("Actor: `%s`", signal.ActorID))
			if label := strings.TrimSpace(s.attentionActorDisplayLabel(workspaceID, signal.ActorType, signal.ActorID)); label != "" && label != signal.ActorID {
				b.WriteString(" — " + label)
			}
			if signal.ActorType != "" {
				b.WriteString(fmt.Sprintf(" (%s)", signal.ActorType))
			}
			b.WriteString("\n")
		}
		if signal.Summary != "" {
			b.WriteString(fmt.Sprintf("Summary: %s\n", signal.Summary))
		}
		if signal.PayloadJSON != "" && signal.PayloadJSON != "{}" {
			b.WriteString(fmt.Sprintf("Payload: `%s`\n", trimWakeupPromptValue(signal.PayloadJSON, 800)))
		}
		if signal.RefsJSON != "" && signal.RefsJSON != "{}" {
			b.WriteString(fmt.Sprintf("Refs: `%s`\n", trimWakeupPromptValue(signal.RefsJSON, 800)))
		}
		b.WriteString("\n")
	}
	b.WriteString("---\n\n")
	b.WriteString(i18n.AttentionHint)
	return b.String(), ids, nil
}

func (s *Server) attentionActorDisplayLabel(workspaceID, actorType, actorID string) string {
	if !strings.EqualFold(strings.TrimSpace(actorType), "user") {
		return ""
	}
	actorID = strings.TrimSpace(actorID)
	if actorID == "" || s == nil || s.users == nil {
		return ""
	}
	if s.controlDB != nil {
		if _, ok, err := s.controlDB.WorkspaceMember(strings.TrimSpace(workspaceID), actorID); err != nil || !ok {
			return ""
		}
	}
	user := s.users.GetUser(actorID)
	if user == nil {
		return ""
	}
	return formatUserIdentityLabel(user.Username, user.DisplayName, user.Email)
}

func (s *Server) markAttentionSignalsSeen(workspaceID string, ids []string) {
	if s == nil || s.controlDB == nil || len(ids) == 0 {
		return
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return
	}
	for _, id := range ids {
		_ = s.controlDB.MarkAttentionSignalStatus(workspaceID, id, "seen")
	}
}

func (s *Server) apiWakeupStrings() apiWakeupI18n {
	lang := "en"
	if s != nil && s.st != nil {
		if agency, err := s.st.Agency(); err == nil && agency != nil && strings.TrimSpace(agency.Lang) != "" {
			lang = strings.TrimSpace(agency.Lang)
		}
	}
	if strings.HasPrefix(strings.ToLower(lang), "zh") {
		return apiWakeupI18n{
			AttentionHeader: "## 注意力信号\n\n",
			AttentionIntro:  "系统记录了以下值得你关注的新信号。它们不是强制触发器，请根据职责、优先级和当前上下文自主判断是否处理、忽略、延后或主动联系相关人：\n\n",
			AttentionHint:   "看到这些信号后，系统只会把它们标记为 seen；如果你完成处理，请用可用工具推进任务、回复 IM、更新流程或沉淀记录。处理 IM 私聊、群聊 @ 或卡片回调时，如需回复到原始会话，请优先使用 `mga notify send --to source ...` 或 `mga notify card send --to source ...`，不要猜测群聊名称。你也可以先用 `mga notify react --to source --emoji EYES` 表示已看到，或先发一句短消息再继续深入处理；必要时可以分多条短消息回复，但不要刷屏。runtime 环境中可用 `mga attention mark <signal-id> --status handled` 或 `--status ignored` 明确闭环。\n\n",
		}
	}
	return apiWakeupI18n{
		AttentionHeader: "## Attention Signals\n\n",
		AttentionIntro:  "Multigent recorded the following new signals for your attention. They are not hard triggers; decide whether to handle, ignore, defer, or contact someone based on your role, priority, and current context:\n\n",
		AttentionHint:   "After these signals are shown, Multigent only marks them as seen. If you handle one, use available tools to advance tasks, reply over IM, update workflows, or record notes. When handling an IM direct message, group mention, or card callback, use `mga notify send --to source ...` or `mga notify card send --to source ...` to reply in the original conversation; do not guess the chat name. You may first use `mga notify react --to source --emoji EYES` to acknowledge that you saw it, or send one short reply before continuing deeper work. Multiple short replies are acceptable when they make the conversation clearer, but avoid spam. In runtime environments, use `mga attention mark <signal-id> --status handled` or `--status ignored` to close the loop explicitly.\n\n",
	}
}

func (s *Server) attentionWakeupTaskPromptSuffix() string {
	lang := "en"
	if s != nil && s.st != nil {
		if agency, err := s.st.Agency(); err == nil && agency != nil && strings.TrimSpace(agency.Lang) != "" {
			lang = strings.TrimSpace(agency.Lang)
		}
	}
	if strings.HasPrefix(strings.ToLower(lang), "zh") {
		return "这次唤醒来自注意力信号。请把它当成同事把事情放到你桌面上，而不是必须立即执行的硬触发器。你应该先判断是否与你当前职责相关、是否值得现在处理；需要处理时可以回复协作渠道、推进任务或流程、记录结论，也可以明确忽略或延后。\n\n如果是 IM 对话，尽量像同事一样自然互动：可以先用 `mga notify react --to source --emoji EYES` 表示已看到，或用 `mga notify send --to source --body \"我先看下\"` 发一条短回应，再继续处理。复杂问题可以分几条短消息说明进展和结论，但不要刷屏。\n\n如果 runtime 中可用 `mga attention list`，请先读取最新未关闭队列，避免只依赖本提示里可能已经过时的快照。\n"
	}
	return "This wakeup comes from attention signals. Treat them as work placed on your desk, not hard triggers. First decide whether each signal is relevant to your role and worth handling now. When appropriate, reply through the collaboration channel, advance tasks or workflows, record conclusions, or explicitly ignore/defer the signal.\n\nFor IM conversations, interact like a responsive coworker: you may first run `mga notify react --to source --emoji EYES` to acknowledge the message, or send a short `mga notify send --to source --body \"I am checking\"` reply before doing deeper work. For complex questions, a few short progress/conclusion replies can be better than one long final block, but avoid spam.\n\nIf `mga attention list` is available in your runtime, read the latest open queue first instead of relying only on the snapshot above.\n"
}

func trimWakeupPromptValue(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
