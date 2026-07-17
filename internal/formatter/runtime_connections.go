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
- `+"`MULTIGENT_API_URL`"+`: Multigent control API base URL.
- `+"`MULTIGENT_AGENT_TOKEN`"+`: scoped runtime token for this agent/run.
- `+"`MULTIGENT_RUN_ID`"+` and `+"`MULTIGENT_WORKSPACE_ID`"+`: run and workspace identifiers.

Use these commands when available:

`+"```bash"+`
mga runtime tools --format table
mga runtime connections --format table
mga runtime action --connection <alias> --data '{"method":"GET","endpoint":"/path"}'
mga runtime mcp --connection <alias> --data '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
`+"```"+`

Rules:

- First inspect `+"`MULTIGENT_CONNECTIONS_FILE`"+` or run `+"`mga runtime tools --format table`"+` to see each tool's `+"`recommendedAdapter`"+`, skills, actions, and connection alias.
- If a tool recommends a platform CLI, use that CLI and its bundled skill, for example `+"`gh`"+` for GitHub or `+"`lark-cli`"+` for Feishu/Lark.
- If a tool recommends MCP Gateway, use the configured MCP Gateway tools rather than attaching every provider MCP server directly.
- Use `+"`mga runtime action`"+` only for provider HTTP actions or as the documented fallback path.
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
