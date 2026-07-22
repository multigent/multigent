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

Use these commands when available:

`+"```bash"+`
mga runtime tools --format table
mga runtime skill-guide
mga runtime connections --format table
mga runtime action --connection <alias> --data '{"method":"GET","endpoint":"/path"}'
`+"```"+`

Rules:

- First run `+"`mga runtime skill-guide`"+` or inspect `+"`MULTIGENT_TOOL_SKILLS_FILE`"+` to see how each enabled tool should be used.
- Use `+"`mga runtime tools --format table`"+` to see each tool's `+"`recommendedAdapter`"+`, skills, actions, and connection alias.
- If a tool recommends a platform CLI, use that CLI and its bundled skill, for example `+"`gh`"+` for GitHub or `+"`lark-cli`"+` for Feishu/Lark.
- If a tool recommends HTTP actions, call it with `+"`mga runtime action --connection <alias>`"+` so Multigent can enforce authorization and audit usage.
- MCP Gateway tools are server-side external tools. Use the runtime skill guide to list or call them only when they are granted to you and relevant to the task.
- Do not read or expose raw provider secrets. Use the configured CLI, MCP Gateway, or Multigent runtime proxy.
- If a needed connection is missing, report the missing provider and target agent instead of inventing credentials.
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
