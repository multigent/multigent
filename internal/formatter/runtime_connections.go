package formatter

import (
	"os"
	"path/filepath"
	"strings"
)

const runtimeConnectionsFilename = "runtime-connections.md"

func runtimeConnectionsGuide() string {
	return strings.TrimSpace(`
## Runtime Connections

Multigent may grant this agent external tool connections such as GitHub, Feishu/Lark, Figma, Linear, or Notion. Each tool declares its recommended runtime adapter: platform CLI, MCP Gateway, HTTP action, or skill-only. Credentials stay managed by Multigent; do not ask humans to paste provider secrets into the chat.

At runtime, Multigent injects:

- `+"`MULTIGENT_CONNECTIONS_FILE`"+`: JSON manifest of connections granted to this agent.
- `+"`MULTIGENT_TOOLS_FILE`"+`: JSON tool runtime plan with recommended adapters, skills, actions, and materialized config paths.
- `+"`MULTIGENT_TOOL_RUNTIME_DIR`"+`: per-run directory for tool adapter config and runtime files.
- `+"`MULTIGENT_TOOL_BIN_DIR`"+`: per-run command wrapper directory. It is prepended to `+"`PATH`"+` so provider CLIs such as `+"`gh`"+` or `+"`lark-cli`"+` can use agent-scoped credentials.
- `+"`MULTIGENT_TOOL_BOOTSTRAP_FILE`"+`: per-run bootstrap script executed before the agent command to install/check provider CLIs declared by runtime adapters.
- `+"`MULTIGENT_TOOL_SKILLS_FILE`"+`: generated Markdown guide that explains how to use the enabled tools and their CLI, MCP Gateway, HTTP action, or skill-only adapters.
- `+"`MULTIGENT_TOOL_CLI_AUDIT_FILE`"+`: best-effort jsonl audit file written by platform CLI wrappers with provider, command name, exit code, and duration metadata.
- `+"`MULTIGENT_API_URL`"+`: Multigent control API base URL.
- `+"`MULTIGENT_AGENT_TOKEN`"+`: scoped runtime token for this agent/run.
- `+"`MULTIGENT_RUN_ID`"+` and `+"`MULTIGENT_WORKSPACE_ID`"+`: run and workspace identifiers.
- `+"`MULTIGENT_FILES_DIR`"+`: read-only directory containing workspace files uploaded to Multigent. In Docker sandboxes this is a container path, not the host path.

Use these commands when available:

`+"```bash"+`
mga runtime tools --format table
mga runtime skill-guide
mga runtime connections --format table
mga runtime channels --format table
mga workflow pending-reviews --format table
mga notify send --to human --subject "Review needed: <task>" --message-format markdown --body "## Decision needed\n\n- Impact: high\n- Recommended action: approve\n- Link: <task or doc URL>" --task <task-id> --urgency review
mga notify send --to source --message-format markdown --body "I saw this and will handle it." --urgency normal
mga notify send --to chat:<group-name> --subject "Team update" --message-format markdown --body "## Update\n\n- Status: running\n- Blocker: none"
mga notify file send --to source --path ./report.md --caption "调试报告已整理好"
mga notify image send --to chat:<group-name> --path ./screenshot.png --caption "复现截图"
mga notify file send --to human --doc doc-20260821-abc123 --caption "这份知识库文档我也作为附件发你"
mga notify card send --to user:<username-or-email> --title "Decision needed" --body "Please choose one option. I will receive your callback as a structured event." --action option_1="Option 1:primary" --action option_2="Option 2" --action request_changes="Request changes:danger:input" --field "Task=<task-id>" --context-json '{"taskId":"<task-id>"}'
mga notify card guide
mga notify card send --to source --card-json-file ./card.json
mga notify card send --to chat:<group-name> --card-json-file ./release-card.template.json --value "title=✅ Release check complete" --value "status=PASS" --value "summary=No blocker found"
mga workflow decision submit --interaction <interaction-id-from-callback> --task <task-id> --decision approve --comments "Approved from collaboration channel"
mga runtime action --connection <alias> --data '{"method":"GET","endpoint":"/path"}'
`+"```"+`

Rules:

- First run `+"`mga runtime skill-guide`"+` or inspect `+"`MULTIGENT_TOOL_SKILLS_FILE`"+` to see how each enabled tool should be used.
- Use `+"`mga runtime tools --format table`"+` to see each tool's `+"`recommendedAdapter`"+`, skills, actions, and connection alias.
- If a tool recommends a platform CLI, use that CLI and its bundled skill, for example `+"`gh`"+` for GitHub or `+"`lark-cli`"+` for Feishu/Lark.
- If a tool recommends HTTP actions, call it with `+"`mga runtime action --connection <alias>`"+` so Multigent can enforce authorization and audit usage.
- MCP Gateway tools are server-side external tools. Use the runtime skill guide to list or call them only when they are granted to you and relevant to the task.
- Use `+"`mga runtime channels --format table`"+` to see human collaboration channels bound to you, including named group chat targets. When a task is blocked, needs review, or needs a time-sensitive human action, use `+"`mga notify send`"+`. Send to `+"`human`"+`, `+"`user:<username-or-email>`"+`, or `+"`chat:<group-name>`"+` when a named group target is listed. When handling an IM mention, reply with `+"`--to source`"+` if you want to answer in the same conversation where the signal came from; use `+"`--to source:<signal-id>`"+` when multiple source signals are present and you need to target one precisely; Multigent will preserve the source conversation and mention/reply context when the provider supports it. Prefer `+"`--message-format markdown`"+` for structured summaries, checklists, links, and review requests. Do not add manual signatures like project/agent names at the bottom; Multigent already keeps internal source metadata. Multigent sends the external message server-side and keeps an internal inbox copy.
- Use `+"`mga notify file send`"+` or `+"`mga notify image send`"+` when the human needs an actual attachment such as a Markdown/HTML report, screenshot, diagram, spreadsheet, or generated artifact. Prefer `+"`--doc <docID>`"+` for knowledge documents and `+"`--path <file>`"+` for files you can read in this runtime. Do not send credentials, raw secrets, huge logs, or unrelated workspace archives.
- When an attention signal includes incoming attachments, download them with `+"`mga attention attachment download <signal-id> --index 1`"+` and inspect the returned local file path. Do not ask the user to upload again just because this sandbox does not have lark-cli/feishu-cli configured.
- Use `+"`mga workflow pending-reviews`"+` during wakeup or project monitoring to inspect human-review gates currently waiting in your project. It is read-only and returns task, workflow step, reviewer, document references, output fields, and route options so you can decide whether to notify the right human.
- Use `+"`mga notify card send`"+` when the human should choose from structured options. Card callbacks are delivered back to your interaction session as structured user events with a Multigent `+"`interactionId`"+`; decide the next step yourself and use protected `+"`mga`"+` commands when state changes are required. For workflow human-review gates, use `+"`mga workflow decision submit --interaction <id> --task <task-id>`"+`. Multigent validates channel identity and the current workflow reviewer before changing workflow state.
- For rich display-only Feishu/Lark cards, first run `+"`mga notify card guide`"+`, then create a Card 2.0 JSON file yourself and send it with `+"`mga notify card send --card-json-file ./card.json`"+`. You can keep your own template JSON with `+"`{{placeholder}}`"+` values and pass `+"`--value key=value`"+`; Multigent only sends the card and handles permissions, it does not impose a fixed card layout. Use action buttons only when you need a callback; display cards do not need actions.
- When a knowledge document references a file under `+"`.multigent/files`"+`, resolve it from `+"`$MULTIGENT_FILES_DIR`"+` instead of using host absolute paths.
- Do not read or expose raw provider secrets. Use the configured CLI, MCP Gateway, or Multigent runtime proxy.
- Do not spam humans. Batch low-priority updates, and notify immediately only for review gates, blockers, external publishing, money/account actions, or explicit human decisions.
- If a needed connection or collaboration channel is missing, report the missing provider and target agent instead of inventing credentials.
`) + "\n"
}

func appendRuntimeConnectionsGuide(sb *strings.Builder) {
	sb.WriteString("\n---\n\n")
	sb.WriteString(runtimeConnectionsGuide())
}

func writeRuntimeConnectionsGuide(contextDir string) (string, error) {
	path := filepath.Join(contextDir, runtimeConnectionsFilename)
	if err := os.WriteFile(path, []byte(runtimeConnectionsGuide()), 0o644); err != nil {
		return "", err
	}
	return runtimeConnectionsFilename, nil
}
