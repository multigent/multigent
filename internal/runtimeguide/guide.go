package runtimeguide

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type Plan struct {
	Tools []Tool `json:"tools"`
}

type Tool struct {
	Provider           string    `json:"provider"`
	DisplayName        string    `json:"displayName,omitempty"`
	ConnectionAlias    string    `json:"connectionAlias,omitempty"`
	ConnectionName     string    `json:"connectionName,omitempty"`
	RecommendedAdapter string    `json:"recommendedAdapter,omitempty"`
	Adapters           []Adapter `json:"adapters,omitempty"`
	Skills             []string  `json:"skills,omitempty"`
	Actions            []Action  `json:"actions,omitempty"`
}

type Adapter struct {
	Type        string   `json:"type"`
	Priority    int      `json:"priority"`
	Description string   `json:"description,omitempty"`
	Skills      []string `json:"skills,omitempty"`
	CLI         *CLI     `json:"cli,omitempty"`
	HTTPAction  *HTTP    `json:"httpAction,omitempty"`
}

type CLI struct {
	Binary string `json:"binary,omitempty"`
}

type HTTP struct {
	ActionNames []string `json:"actionNames,omitempty"`
}

type Action struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName,omitempty"`
	Method      string `json:"method,omitempty"`
	Endpoint    string `json:"endpoint,omitempty"`
}

func RenderJSON(body []byte) (string, error) {
	var plan Plan
	if err := json.Unmarshal(body, &plan); err != nil {
		return "", err
	}
	return Render(plan), nil
}

func Render(plan Plan) string {
	var sb strings.Builder
	sb.WriteString("# Runtime Tool Skills\n\n")
	sb.WriteString("Multigent generated this guide for the external tools currently enabled for this agent. Use it together with `mga runtime tools --format table`.\n\n")
	if len(plan.Tools) == 0 {
		sb.WriteString("No external tools are enabled for this run.\n")
		return sb.String()
	}
	for _, tool := range plan.Tools {
		renderTool(&sb, tool)
	}
	return strings.TrimRight(sb.String(), "\n") + "\n"
}

func renderTool(sb *strings.Builder, tool Tool) {
	title := firstNonEmpty(tool.DisplayName, tool.Provider, tool.ConnectionAlias, "External Tool")
	alias := firstNonEmpty(tool.ConnectionAlias, tool.Provider)
	sb.WriteString("## " + title)
	if alias != "" {
		sb.WriteString(" (`" + alias + "`)")
	}
	sb.WriteString("\n\n")
	writeKV(sb, "Provider", tool.Provider)
	writeKV(sb, "Connection", firstNonEmpty(tool.ConnectionName, tool.ConnectionAlias))
	writeKV(sb, "Recommended adapter", tool.RecommendedAdapter)
	skills := uniqueStrings(append([]string{}, tool.Skills...))
	for _, adapter := range tool.Adapters {
		skills = uniqueStrings(append(skills, adapter.Skills...))
	}
	if len(skills) > 0 {
		writeKV(sb, "Skills", strings.Join(skills, ", "))
	}
	sb.WriteString("\n")
	adapters := sortedAdapters(tool.Adapters)
	if len(adapters) == 0 {
		sb.WriteString("- No runtime adapter is configured. Use the listed skills as guidance and report missing executable capability if needed.\n\n")
		return
	}
	for _, adapter := range adapters {
		renderAdapter(sb, alias, tool.Provider, adapter)
	}
	if len(tool.Actions) > 0 {
		sb.WriteString("Available HTTP actions:\n")
		for _, action := range tool.Actions {
			name := firstNonEmpty(action.Name, action.DisplayName)
			if name == "" {
				continue
			}
			detail := strings.TrimSpace(strings.Join([]string{action.Method, action.Endpoint}, " "))
			if detail != "" {
				sb.WriteString(fmt.Sprintf("- `%s`: %s\n", name, detail))
			} else {
				sb.WriteString(fmt.Sprintf("- `%s`\n", name))
			}
		}
		sb.WriteString("\n")
	}
}

func renderAdapter(sb *strings.Builder, alias, provider string, adapter Adapter) {
	label := firstNonEmpty(adapter.Type, "unknown")
	sb.WriteString("Adapter `" + label + "`:\n")
	if adapter.Description != "" {
		sb.WriteString("- " + adapter.Description + "\n")
	}
	switch adapter.Type {
	case "cli":
		binary := ""
		if adapter.CLI != nil {
			binary = adapter.CLI.Binary
		}
		if binary == "" {
			binary = "<provider-cli>"
		}
		sb.WriteString("- Use the platform CLI. Credentials and config are already scoped to this agent runtime.\n")
		sb.WriteString("- Start with `" + binary + " --help` or the provider-specific skill instructions.\n")
	case "mcp_gateway":
		sb.WriteString("- Use the unified Multigent MCP Gateway instead of attaching provider MCP servers directly.\n")
		sb.WriteString("- List tools: `mga runtime gateway list-tools --provider " + shellArg(provider) + " --format table`.\n")
		sb.WriteString("- Call tools: `mga runtime gateway call-tool <tool-id> --data '{...}'`.\n")
	case "http_action":
		sb.WriteString("- Use the audited HTTP action proxy through Multigent.\n")
		if alias != "" {
			sb.WriteString("- Example: `mga runtime action --connection " + shellArg(alias) + " --data '{\"method\":\"GET\",\"endpoint\":\"/path\"}'`.\n")
		} else {
			sb.WriteString("- Example: `mga runtime action --data '{\"method\":\"GET\",\"endpoint\":\"/path\"}'`.\n")
		}
	case "skill_only":
		sb.WriteString("- No executable runtime is configured. Follow the listed skills and report if a tool call is required.\n")
	default:
		sb.WriteString("- Unknown adapter type. Inspect `mga runtime tools --format json` before using it.\n")
	}
	if len(adapter.Skills) > 0 {
		sb.WriteString("- Adapter skills: `" + strings.Join(uniqueStrings(adapter.Skills), "`, `") + "`.\n")
	}
	sb.WriteString("\n")
}

func sortedAdapters(adapters []Adapter) []Adapter {
	out := append([]Adapter(nil), adapters...)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Priority > out[j].Priority
	})
	return out
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func writeKV(sb *strings.Builder, key, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	sb.WriteString("- " + key + ": `" + strings.TrimSpace(value) + "`\n")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func shellArg(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "''"
	}
	if strings.ContainsAny(value, " \t\n'\"") {
		return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
	}
	return value
}
