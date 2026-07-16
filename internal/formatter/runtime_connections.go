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

Multigent may grant this agent external tool connections such as GitHub, Feishu/Lark, Linear, custom HTTP APIs, or custom MCP servers. Credentials stay inside Multigent; do not ask humans to paste provider secrets into the chat.

At runtime, Multigent injects:

- `+"`MULTIGENT_CONNECTIONS_FILE`"+`: JSON manifest of connections granted to this agent.
- `+"`MULTIGENT_API_URL`"+`: Multigent control API base URL.
- `+"`MULTIGENT_AGENT_TOKEN`"+`: scoped runtime token for this agent/run.
- `+"`MULTIGENT_RUN_ID`"+` and `+"`MULTIGENT_WORKSPACE_ID`"+`: run and workspace identifiers.

Use these commands when available:

`+"```bash"+`
multigent-agent runtime connections --format table
multigent-agent runtime action --connection <alias> --data '{"method":"GET","endpoint":"/path"}'
multigent-agent runtime mcp --connection <alias> --data '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
`+"```"+`

Rules:

- Use connection aliases from the manifest or `+"`multigent-agent runtime connections`"+`.
- Do not read or expose raw provider secrets; call through Multigent runtime proxies.
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
