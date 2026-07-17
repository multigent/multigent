package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/multigent/multigent/internal/runtimeguide"
	"github.com/spf13/cobra"
)

const (
	envAPIURL          = "MULTIGENT_API_URL"
	envAgentToken      = "MULTIGENT_AGENT_TOKEN"
	envConnectionsFile = "MULTIGENT_CONNECTIONS_FILE"
	envToolsFile       = "MULTIGENT_TOOLS_FILE"
	envToolSkillsFile  = "MULTIGENT_TOOL_SKILLS_FILE"
	maxJSONBody        = 1 << 20
)

var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

func main() {
	root := &cobra.Command{
		Use:           "mga",
		Short:         "Multigent agent runtime CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(
		newVersionCmd(),
		newRuntimeCmd(),
		newTaskCmd(),
		newInboxCmd(),
		newDocsCmd(),
	)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print mga version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("mga %s\n", version)
			fmt.Printf("  commit : %s\n", commit)
			fmt.Printf("  built  : %s\n", buildDate)
		},
	}
}

func newRuntimeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "runtime",
		Short: "Use scoped runtime tool connections",
	}
	cmd.AddCommand(newRuntimeConnectionsCmd(), newRuntimeToolsCmd(), newRuntimeSkillGuideCmd(), newRuntimeActionCmd(), newRuntimeMCPCmd(), newRuntimeGatewayCmd())
	cmd.AddCommand(newRuntimeVersionCmd())
	return cmd
}

func newRuntimeVersionCmd() *cobra.Command {
	var check bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print runtime CLI version",
		Run: func(cmd *cobra.Command, args []string) {
			_ = check
			fmt.Printf("mga %s\n", version)
			fmt.Printf("  commit : %s\n", commit)
			fmt.Printf("  built  : %s\n", buildDate)
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "check compatibility with the injected Multigent Server")
	return cmd
}

func newRuntimeConnectionsCmd() *cobra.Command {
	var format string
	var refresh bool
	cmd := &cobra.Command{
		Use:   "connections",
		Short: "List tool connections granted to this agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := runtimeConnectionsBody(refresh)
			if err != nil {
				return err
			}
			if format == "table" {
				return printConnectionsTable(body)
			}
			return writeJSON(body)
		},
	}
	cmd.Flags().StringVar(&format, "format", "json", "output format: json or table")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "refresh from runtime API instead of using materialized manifest")
	return cmd
}

func newRuntimeActionCmd() *cobra.Command {
	return newProxyCmd("action", "Send an HTTP action proxy request through a granted connection", "/api/v1/runtime/actions")
}

func newRuntimeToolsCmd() *cobra.Command {
	var format string
	var refresh bool
	cmd := &cobra.Command{
		Use:   "tools",
		Short: "List external tools and recommended runtime adapters for this agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := runtimeToolsBody(refresh)
			if err != nil {
				return err
			}
			if format == "table" {
				return printRuntimeToolsTable(body)
			}
			return printRuntimeToolsJSON(body)
		},
	}
	cmd.Flags().StringVar(&format, "format", "json", "output format: json or table")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "refresh from runtime API instead of using materialized manifest")
	return cmd
}

func newRuntimeSkillGuideCmd() *cobra.Command {
	var refresh bool
	cmd := &cobra.Command{
		Use:   "skill-guide",
		Short: "Print the runtime tool skill guide for this agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !refresh {
				if path := strings.TrimSpace(os.Getenv(envToolSkillsFile)); path != "" {
					body, err := os.ReadFile(path)
					if err == nil {
						_, err = os.Stdout.Write(append(bytes.TrimRight(body, "\n"), '\n'))
						return err
					}
				}
			}
			body, err := runtimeToolsBody(refresh)
			if err != nil {
				return err
			}
			guide, err := runtimeguide.RenderJSON(body)
			if err != nil {
				return err
			}
			_, err = os.Stdout.Write([]byte(guide))
			return err
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "refresh from runtime API instead of using materialized guide")
	return cmd
}

func newRuntimeMCPCmd() *cobra.Command {
	return newProxyCmd("mcp", "Send a JSON-RPC request through a granted MCP connection", "/api/v1/runtime/mcp")
}

func newRuntimeGatewayCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gateway",
		Short: "Use the unified Multigent MCP Gateway",
	}
	cmd.AddCommand(newRuntimeGatewayListToolsCmd(), newRuntimeGatewayCallToolCmd())
	return cmd
}

func newRuntimeGatewayListToolsCmd() *cobra.Command {
	var provider, adapter, format string
	cmd := &cobra.Command{
		Use:   "list-tools",
		Short: "List gateway-callable tools available to this agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			arguments := map[string]any{}
			if strings.TrimSpace(provider) != "" {
				arguments["provider"] = strings.TrimSpace(provider)
			}
			if strings.TrimSpace(adapter) != "" {
				arguments["adapter"] = strings.TrimSpace(adapter)
			}
			body, err := callMCPGatewayTool("multigent.list_tools", arguments)
			if err != nil {
				return err
			}
			if format == "table" {
				return printGatewayToolsTable(body)
			}
			return writeJSON(body)
		},
	}
	cmd.Flags().StringVar(&provider, "provider", "", "filter by provider")
	cmd.Flags().StringVar(&adapter, "adapter", "", "filter by adapter: cli, mcp_gateway, http_action, or skill_only")
	cmd.Flags().StringVar(&format, "format", "json", "output format: json or table")
	return cmd
}

func newRuntimeGatewayCallToolCmd() *cobra.Command {
	var data, file string
	cmd := &cobra.Command{
		Use:   "call-tool <tool-id>",
		Short: "Call a gateway tool by tool id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if strings.TrimSpace(data) != "" || strings.TrimSpace(file) != "" {
				body, err := readRequestBody(data, file)
				if err != nil {
					return err
				}
				if err := json.Unmarshal(body, &toolArgs); err != nil {
					return fmt.Errorf("tool arguments must be a JSON object")
				}
			}
			body, err := callMCPGatewayTool("multigent.call_tool", map[string]any{
				"tool_id":   args[0],
				"arguments": toolArgs,
			})
			if err != nil {
				return err
			}
			return writeJSON(body)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "JSON tool arguments")
	cmd.Flags().StringVar(&file, "file", "", "read JSON tool arguments from file, or '-' for stdin")
	return cmd
}

func newProxyCmd(use, short, endpoint string) *cobra.Command {
	var connection string
	var data string
	var file string
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := readRequestBody(data, file)
			if err != nil {
				return err
			}
			q := url.Values{}
			if strings.TrimSpace(connection) != "" {
				q.Set("alias", strings.TrimSpace(connection))
			}
			resp, status, err := requestJSONWithStatus(http.MethodPost, endpoint, q, body)
			if err != nil {
				return err
			}
			if err := writeJSON(resp); err != nil {
				return err
			}
			if status < 200 || status >= 300 {
				return fmt.Errorf("runtime proxy returned HTTP %d", status)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&connection, "connection", "", "connection id or runtime alias")
	cmd.Flags().StringVar(&data, "data", "", "JSON request body")
	cmd.Flags().StringVar(&file, "file", "", "read JSON request body from file, or '-' for stdin")
	_ = cmd.MarkFlagRequired("connection")
	return cmd
}

func newTaskCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "task", Short: "Create, inspect, and update tasks"}
	cmd.AddCommand(
		newTaskListCmd(),
		newTaskShowCmd(),
		newTaskAddCmd(),
		newTaskSetCmd(),
		newTaskDoneCmd(),
		newTaskCancelCmd(),
		newTaskConfirmRequestCmd(),
	)
	return cmd
}

func newTaskListCmd() *cobra.Command {
	var status, agent, scope, format string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List runtime-visible tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			if status != "" {
				q.Set("status", status)
			}
			if agent != "" {
				q.Set("agent", agent)
			}
			if scope != "" {
				q.Set("scope", scope)
			}
			body, err := requestJSON(http.MethodGet, "/api/v1/runtime/tasks", q, nil)
			if err != nil {
				return err
			}
			if format == "table" {
				return printTasksTable(body)
			}
			return writeJSON(body)
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "filter by task status")
	cmd.Flags().StringVar(&agent, "agent", "", "filter by agent in the current project")
	cmd.Flags().StringVar(&scope, "scope", "all", "active, archived, or all")
	cmd.Flags().StringVar(&format, "format", "json", "output format: json or table")
	cmd.Flags().Bool("json", false, "print JSON output")
	return cmd
}

func newTaskShowCmd() *cobra.Command {
	var agent string
	cmd := &cobra.Command{
		Use:   "show <task-id>",
		Short: "Show a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			if agent != "" {
				q.Set("agent", agent)
			}
			body, err := requestJSON(http.MethodGet, "/api/v1/runtime/tasks/"+url.PathEscape(args[0]), q, nil)
			if err != nil {
				return err
			}
			return writeJSON(body)
		},
	}
	cmd.Flags().StringVar(&agent, "agent", "", "agent that owns the task")
	return cmd
}

func newTaskAddCmd() *cobra.Command {
	var agent, title, prompt, typ, description, assignee string
	var priority int
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Create a task in the current runtime project",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(title) == "" || strings.TrimSpace(prompt) == "" {
				return fmt.Errorf("title and prompt are required")
			}
			body, _ := json.Marshal(map[string]any{
				"agent": agent, "title": title, "prompt": prompt, "type": typ,
				"description": description, "priority": priority, "assignee": assignee,
			})
			resp, err := requestJSON(http.MethodPost, "/api/v1/runtime/tasks", nil, body)
			if err != nil {
				return err
			}
			return writeJSON(resp)
		},
	}
	cmd.Flags().StringVar(&agent, "agent", "", "target agent, defaults to current agent")
	cmd.Flags().StringVar(&title, "title", "", "task title")
	cmd.Flags().StringVar(&prompt, "prompt", "", "task prompt")
	cmd.Flags().StringVar(&typ, "type", "chore", "task type")
	cmd.Flags().StringVar(&description, "description", "", "human-readable description")
	cmd.Flags().StringVar(&assignee, "assignee", "", "assignee identity")
	cmd.Flags().IntVar(&priority, "priority", 2, "priority 0-3")
	return cmd
}

func newTaskSetCmd() *cobra.Command {
	var agent, status, summary, errText, title, prompt string
	var priority int
	var setPriority bool
	cmd := &cobra.Command{
		Use:     "set <task-id>",
		Aliases: []string{"update"},
		Short:   "Update a task",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{"agent": agent}
			if status != "" {
				body["status"] = status
			}
			if summary != "" {
				body["summary"] = summary
			}
			if errText != "" {
				body["error"] = errText
			}
			if title != "" {
				body["title"] = title
			}
			if prompt != "" {
				body["prompt"] = prompt
			}
			if setPriority {
				body["priority"] = priority
			}
			raw, _ := json.Marshal(body)
			resp, err := requestJSON(http.MethodPut, "/api/v1/runtime/tasks/"+url.PathEscape(args[0]), nil, raw)
			if err != nil {
				return err
			}
			return writeJSON(resp)
		},
	}
	cmd.Flags().StringVar(&agent, "agent", "", "agent that owns the task")
	cmd.Flags().StringVar(&status, "status", "", "new status")
	cmd.Flags().StringVar(&summary, "summary", "", "task summary")
	cmd.Flags().StringVar(&errText, "error", "", "task error")
	cmd.Flags().StringVar(&title, "title", "", "new title")
	cmd.Flags().StringVar(&prompt, "prompt", "", "new prompt")
	cmd.Flags().IntVar(&priority, "priority", 2, "priority 0-3")
	cmd.PreRun = func(cmd *cobra.Command, args []string) {
		setPriority = cmd.Flags().Changed("priority")
	}
	return cmd
}

func newTaskCancelCmd() *cobra.Command {
	var id, agent, reason string
	cmd := &cobra.Command{
		Use:   "cancel",
		Short: "Cancel a task",
		RunE: func(cmd *cobra.Command, args []string) error {
			if id == "" && len(args) > 0 {
				id = args[0]
			}
			if id == "" {
				return fmt.Errorf("--id or task id argument is required")
			}
			body, _ := json.Marshal(map[string]any{
				"agent": agent, "status": "cancelled", "summary": reason,
			})
			resp, err := requestJSON(http.MethodPut, "/api/v1/runtime/tasks/"+url.PathEscape(id), nil, body)
			if err != nil {
				return err
			}
			return writeJSON(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "task id")
	cmd.Flags().StringVar(&agent, "agent", "", "agent that owns the task")
	cmd.Flags().StringVar(&reason, "reason", "", "cancellation reason")
	return cmd
}

func newTaskDoneCmd() *cobra.Command {
	var id, agent, status, summary, errText string
	cmd := &cobra.Command{
		Use:   "done",
		Short: "Mark a task completed or failed",
		RunE: func(cmd *cobra.Command, args []string) error {
			if id == "" {
				return fmt.Errorf("--id is required")
			}
			if status == "" {
				status = "success"
			}
			body, _ := json.Marshal(map[string]any{"agent": agent, "status": status, "summary": summary, "error": errText})
			resp, err := requestJSON(http.MethodPost, "/api/v1/runtime/tasks/"+url.PathEscape(id)+"/done", nil, body)
			if err != nil {
				return err
			}
			return writeJSON(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "task id")
	cmd.Flags().StringVar(&agent, "agent", "", "agent that owns the task")
	cmd.Flags().StringVar(&status, "status", "success", "success or failed")
	cmd.Flags().StringVar(&summary, "summary", "", "completion summary")
	cmd.Flags().StringVar(&errText, "error", "", "failure reason")
	return cmd
}

func newTaskConfirmRequestCmd() *cobra.Command {
	var id, agent, to, summary, actionHint string
	var actionItems []string
	cmd := &cobra.Command{
		Use:   "confirm-request",
		Short: "Request human or agent confirmation for a task",
		RunE: func(cmd *cobra.Command, args []string) error {
			if id == "" || summary == "" {
				return fmt.Errorf("--id and --summary are required")
			}
			body, _ := json.Marshal(map[string]any{
				"agent": agent, "to": to, "summary": summary,
				"actionHint": actionHint, "actionItems": actionItems,
			})
			resp, err := requestJSON(http.MethodPost, "/api/v1/runtime/tasks/"+url.PathEscape(id)+"/confirm-request", nil, body)
			if err != nil {
				return err
			}
			return writeJSON(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "task id")
	cmd.Flags().StringVar(&agent, "agent", "", "agent that owns the task")
	cmd.Flags().StringVar(&to, "to", "human", "recipient identity")
	cmd.Flags().StringVar(&summary, "summary", "", "confirmation summary")
	cmd.Flags().StringVar(&actionHint, "action-hint", "", "suggested action")
	cmd.Flags().StringArrayVar(&actionItems, "action-item", nil, "action item, repeatable")
	return cmd
}

func newInboxCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "inbox", Aliases: []string{"message", "messages"}, Short: "Send and read runtime messages"}
	cmd.AddCommand(newInboxMessagesCmd(), newInboxSendCmd(), newInboxReplyCmd())
	return cmd
}

func newDocsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "docs", Short: "Read and create knowledge base documents"}
	cmd.AddCommand(newDocsListCmd(), newDocsSearchCmd(), newDocsShowCmd(), newDocsCreateCmd())
	return cmd
}

func newDocsListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List knowledge base documents",
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := requestJSON(http.MethodGet, "/api/v1/runtime/docs", nil, nil)
			if err != nil {
				return err
			}
			return writeJSON(body)
		},
	}
	return cmd
}

func newDocsSearchCmd() *cobra.Command {
	var content bool
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search knowledge base documents",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{"q": []string{args[0]}}
			if content {
				q.Set("content", "true")
			}
			body, err := requestJSON(http.MethodGet, "/api/v1/runtime/docs", q, nil)
			if err != nil {
				return err
			}
			return writeJSON(body)
		},
	}
	cmd.Flags().BoolVar(&content, "content", true, "search document content")
	return cmd
}

func newDocsShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <doc-id>",
		Short: "Show a knowledge base document with content",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := requestJSON(http.MethodGet, "/api/v1/runtime/docs/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return writeJSON(body)
		},
	}
	return cmd
}

func newDocsCreateCmd() *cobra.Command {
	var title, index, tags, description, content, file string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a managed knowledge base document",
		RunE: func(cmd *cobra.Command, args []string) error {
			bodyText := content
			sourceName := ""
			if strings.TrimSpace(file) != "" {
				raw, err := readTextFile(file)
				if err != nil {
					return err
				}
				bodyText = raw
				if file != "-" {
					sourceName = file
				}
			}
			if strings.TrimSpace(bodyText) == "" {
				return fmt.Errorf("--content or --file is required")
			}
			raw, _ := json.Marshal(map[string]any{
				"title": title, "index": index, "tags": splitCSV(tags),
				"description": description, "content": bodyText, "sourceName": sourceName,
			})
			resp, err := requestJSON(http.MethodPost, "/api/v1/runtime/docs", nil, raw)
			if err != nil {
				return err
			}
			return writeJSON(resp)
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "document title")
	cmd.Flags().StringVar(&index, "index", "", "virtual directory")
	cmd.Flags().StringVar(&tags, "tags", "", "comma-separated tags")
	cmd.Flags().StringVar(&description, "description", "", "document description")
	cmd.Flags().StringVar(&content, "content", "", "document content")
	cmd.Flags().StringVar(&file, "file", "", "read document content from file, or '-' for stdin")
	return cmd
}

func newInboxMessagesCmd() *cobra.Command {
	var archived bool
	cmd := &cobra.Command{
		Use:     "messages",
		Aliases: []string{"list"},
		Short:   "List messages for the current agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			if archived {
				q.Set("archived", "all")
			}
			body, err := requestJSON(http.MethodGet, "/api/v1/runtime/messages", q, nil)
			if err != nil {
				return err
			}
			return writeJSON(body)
		},
	}
	cmd.Flags().BoolVar(&archived, "archived", false, "include archived messages")
	return cmd
}

func newInboxSendCmd() *cobra.Command {
	var to []string
	var subject, body string
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send a message from the current agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(to) == 0 || strings.TrimSpace(body) == "" {
				return fmt.Errorf("--to and --body are required")
			}
			raw, _ := json.Marshal(map[string]any{"to": to, "subject": subject, "body": body})
			resp, err := requestJSON(http.MethodPost, "/api/v1/runtime/messages", nil, raw)
			if err != nil {
				return err
			}
			return writeJSON(resp)
		},
	}
	cmd.Flags().StringArrayVar(&to, "to", nil, "recipient identity, repeatable")
	cmd.Flags().StringVar(&subject, "subject", "", "message subject")
	cmd.Flags().StringVar(&body, "body", "", "message body")
	return cmd
}

func newInboxReplyCmd() *cobra.Command {
	var subject, body string
	cmd := &cobra.Command{
		Use:   "reply <message-id>",
		Short: "Reply to a message in the current agent mailbox",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(body) == "" {
				return fmt.Errorf("--body is required")
			}
			raw, _ := json.Marshal(map[string]any{"subject": subject, "body": body})
			resp, err := requestJSON(http.MethodPost, "/api/v1/runtime/messages/"+url.PathEscape(args[0])+"/reply", nil, raw)
			if err != nil {
				return err
			}
			return writeJSON(resp)
		},
	}
	cmd.Flags().StringVar(&subject, "subject", "", "reply subject")
	cmd.Flags().StringVar(&body, "body", "", "reply body")
	return cmd
}

func runtimeConnectionsBody(refresh bool) ([]byte, error) {
	if !refresh {
		if path := strings.TrimSpace(os.Getenv(envConnectionsFile)); path != "" {
			if body, err := os.ReadFile(path); err == nil && json.Valid(body) {
				return body, nil
			}
		}
	}
	return requestJSON(http.MethodGet, "/api/v1/runtime/connections", nil, nil)
}

func runtimeToolsBody(refresh bool) ([]byte, error) {
	if !refresh {
		if path := strings.TrimSpace(os.Getenv(envToolsFile)); path != "" {
			if body, err := os.ReadFile(path); err == nil && json.Valid(body) {
				return body, nil
			}
		}
	}
	return runtimeConnectionsBody(refresh)
}

func requestJSON(method, path string, query url.Values, body []byte) ([]byte, error) {
	resp, status, err := requestJSONWithStatus(method, path, query, body)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("runtime API returned HTTP %d: %s", status, strings.TrimSpace(string(resp)))
	}
	return resp, nil
}

func requestJSONWithStatus(method, path string, query url.Values, body []byte) ([]byte, int, error) {
	apiURL := strings.TrimRight(strings.TrimSpace(os.Getenv(envAPIURL)), "/")
	token := strings.TrimSpace(os.Getenv(envAgentToken))
	if apiURL == "" || token == "" {
		return nil, 0, fmt.Errorf("%s and %s are required", envAPIURL, envAgentToken)
	}
	u, err := url.Parse(apiURL + path)
	if err != nil {
		return nil, 0, err
	}
	if len(query) > 0 {
		u.RawQuery = query.Encode()
	}
	req, err := http.NewRequest(method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxJSONBody+1))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if len(respBody) > maxJSONBody {
		return nil, resp.StatusCode, fmt.Errorf("runtime response too large")
	}
	return respBody, resp.StatusCode, nil
}

func readRequestBody(data, file string) ([]byte, error) {
	if strings.TrimSpace(data) != "" {
		body := []byte(data)
		if !json.Valid(body) {
			return nil, fmt.Errorf("--data must be valid JSON")
		}
		return body, nil
	}
	if strings.TrimSpace(file) != "" {
		var body []byte
		var err error
		if file == "-" {
			body, err = io.ReadAll(io.LimitReader(os.Stdin, maxJSONBody+1))
		} else {
			body, err = os.ReadFile(file)
		}
		if err != nil {
			return nil, err
		}
		body = bytes.TrimSpace(body)
		if !json.Valid(body) {
			return nil, fmt.Errorf("--file must contain valid JSON")
		}
		return body, nil
	}
	return nil, fmt.Errorf("--data or --file is required")
}

func callMCPGatewayTool(name string, arguments map[string]any) ([]byte, error) {
	reqBody, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      name,
			"arguments": arguments,
		},
	})
	if err != nil {
		return nil, err
	}
	body, err := requestJSON(http.MethodPost, "/api/v1/runtime/mcp/gateway", nil, reqBody)
	if err != nil {
		return nil, err
	}
	return unwrapMCPGatewayTextResult(body)
}

func unwrapMCPGatewayTextResult(body []byte) ([]byte, error) {
	var resp struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("MCP gateway error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	for _, content := range resp.Result.Content {
		if content.Type == "text" && json.Valid([]byte(content.Text)) {
			return []byte(content.Text), nil
		}
	}
	return body, nil
}

func readTextFile(file string) (string, error) {
	var body []byte
	var err error
	if file == "-" {
		body, err = io.ReadAll(io.LimitReader(os.Stdin, maxJSONBody+1))
	} else {
		body, err = os.ReadFile(file)
	}
	if err != nil {
		return "", err
	}
	if len(body) > maxJSONBody {
		return "", fmt.Errorf("file too large")
	}
	return string(body), nil
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func writeJSON(body []byte) error {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		body = []byte("{}")
	}
	_, err := os.Stdout.Write(append(body, '\n'))
	return err
}

func printConnectionsTable(body []byte) error {
	var doc struct {
		Connections []struct {
			ID             string `json:"id"`
			Provider       string `json:"provider"`
			ConnectionName string `json:"connectionName"`
			Runtime        struct {
				Alias string `json:"alias"`
			} `json:"runtime"`
		} `json:"connections"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ALIAS\tPROVIDER\tNAME\tID")
	for _, c := range doc.Connections {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", c.Runtime.Alias, c.Provider, c.ConnectionName, c.ID)
	}
	return tw.Flush()
}

func printRuntimeToolsJSON(body []byte) error {
	var doc struct {
		Tools json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return err
	}
	if len(doc.Tools) == 0 {
		doc.Tools = []byte("[]")
	}
	return writeJSON(doc.Tools)
}

func printRuntimeToolsTable(body []byte) error {
	var doc struct {
		Tools []struct {
			Provider           string   `json:"provider"`
			DisplayName        string   `json:"displayName"`
			ConnectionAlias    string   `json:"connectionAlias"`
			ConnectionName     string   `json:"connectionName"`
			RecommendedAdapter string   `json:"recommendedAdapter"`
			Skills             []string `json:"skills"`
			Actions            []struct {
				Name string `json:"name"`
			} `json:"actions"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ALIAS\tPROVIDER\tADAPTER\tSKILLS\tACTIONS")
	for _, tool := range doc.Tools {
		names := make([]string, 0, len(tool.Actions))
		for _, action := range tool.Actions {
			if strings.TrimSpace(action.Name) != "" {
				names = append(names, action.Name)
			}
		}
		provider := tool.DisplayName
		if provider == "" {
			provider = tool.Provider
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			tool.ConnectionAlias,
			provider,
			tool.RecommendedAdapter,
			strings.Join(tool.Skills, ","),
			strings.Join(names, ","),
		)
	}
	return tw.Flush()
}

func printGatewayToolsTable(body []byte) error {
	var tools []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Provider    string `json:"provider"`
		Connection  string `json:"connection"`
		Adapter     string `json:"adapter"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &tools); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tPROVIDER\tCONNECTION\tADAPTER\tNAME")
	for _, tool := range tools {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", tool.ID, tool.Provider, tool.Connection, tool.Adapter, tool.Name)
	}
	return tw.Flush()
}

func printTasksTable(body []byte) error {
	var rows []struct {
		ID       string `json:"id"`
		Agent    string `json:"agent"`
		Status   string `json:"status"`
		Title    string `json:"title"`
		Priority int    `json:"priority"`
	}
	if err := json.Unmarshal(body, &rows); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tAGENT\tSTATUS\tP\tTITLE")
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n", r.ID, r.Agent, r.Status, r.Priority, r.Title)
	}
	return tw.Flush()
}
