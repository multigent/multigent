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
	"time"

	"github.com/spf13/cobra"
)

const (
	runtimeEnvAPIURL          = "MULTIGENT_API_URL"
	runtimeEnvAgentToken      = "MULTIGENT_AGENT_TOKEN"
	runtimeEnvConnectionsFile = "MULTIGENT_CONNECTIONS_FILE"
	maxRuntimeCLIJSONBody     = 1 << 20
)

type runtimeConnectionDocument struct {
	Project     string                         `json:"project"`
	Agent       string                         `json:"agent"`
	Manifest    map[string]any                 `json:"manifest"`
	Connections []runtimeConnectionDocumentRow `json:"connections"`
}

type runtimeConnectionDocumentRow struct {
	ID             string `json:"id"`
	Provider       string `json:"provider"`
	ConnectionName string `json:"connectionName"`
	Runtime        struct {
		Alias string `json:"alias"`
	} `json:"runtime"`
}

func newRuntimeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "runtime",
		Short: "Use scoped runtime APIs from inside an agent sandbox",
		Long: `Use scoped runtime APIs from inside an agent sandbox.

These commands require MULTIGENT_API_URL and MULTIGENT_AGENT_TOKEN injected by
Multigent when an agent run starts. They are intentionally not a management API:
all authorization is checked by the runtime agent token.`,
	}
	cmd.AddCommand(
		newRuntimeConnectionsCmd(),
		newRuntimeActionCmd(),
		newRuntimeMCPCmd(),
	)
	return cmd
}

func newRuntimeConnectionsCmd() *cobra.Command {
	var format string
	var refresh bool
	cmd := &cobra.Command{
		Use:   "connections",
		Short: "Print runtime connections granted to the current agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := runtimeConnectionsBody(refresh)
			if err != nil {
				return err
			}
			if resolveFormat(format) == "table" {
				return printRuntimeConnectionsTable(body)
			}
			_, err = os.Stdout.Write(append(bytes.TrimSpace(body), '\n'))
			return err
		},
	}
	cmd.Flags().StringVar(&format, "format", "", "output format: json or table")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "refresh from runtime API instead of using the materialized file")
	return cmd
}

func newRuntimeMCPCmd() *cobra.Command {
	var connection string
	var data string
	var file string
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Send a JSON-RPC request through a granted runtime MCP connection",
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := readRuntimeRequestBody(data, file)
			if err != nil {
				return err
			}
			target, err := resolveRuntimeConnectionTarget(connection)
			if err != nil {
				return err
			}
			respBody, status, err := runtimePostJSON("/api/v1/runtime/mcp", target.query(), body)
			if err != nil {
				return err
			}
			if _, err := os.Stdout.Write(append(bytes.TrimSpace(respBody), '\n')); err != nil {
				return err
			}
			if status < 200 || status >= 300 {
				return fmt.Errorf("runtime MCP proxy returned HTTP %d", status)
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

func newRuntimeActionCmd() *cobra.Command {
	var connection string
	var data string
	var file string
	cmd := &cobra.Command{
		Use:   "action",
		Short: "Send an HTTP action proxy request through a granted runtime connection",
		Long: `Send an HTTP action proxy request through a granted runtime connection.

The request body must be JSON:
{"method":"GET","endpoint":"/path","query":{"k":"v"}}

The endpoint must be a relative path. Multigent injects the connection's
server-side credentials; do not include Authorization headers in the request.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := readRuntimeRequestBody(data, file)
			if err != nil {
				return err
			}
			target, err := resolveRuntimeConnectionTarget(connection)
			if err != nil {
				return err
			}
			respBody, status, err := runtimePostJSON("/api/v1/runtime/actions", target.query(), body)
			if err != nil {
				return err
			}
			if _, err := os.Stdout.Write(append(bytes.TrimSpace(respBody), '\n')); err != nil {
				return err
			}
			if status < 200 || status >= 300 {
				return fmt.Errorf("runtime action proxy returned HTTP %d", status)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&connection, "connection", "", "connection id or runtime alias")
	cmd.Flags().StringVar(&data, "data", "", "JSON proxy request body")
	cmd.Flags().StringVar(&file, "file", "", "read JSON proxy request body from file, or '-' for stdin")
	_ = cmd.MarkFlagRequired("connection")
	return cmd
}

type runtimeConnectionTarget struct {
	ID    string
	Alias string
}

func (t runtimeConnectionTarget) query() url.Values {
	q := url.Values{}
	if t.ID != "" {
		q.Set("connection", t.ID)
	}
	if t.Alias != "" {
		q.Set("alias", t.Alias)
	}
	return q
}

func runtimeConnectionsBody(refresh bool) ([]byte, error) {
	if !refresh {
		if path := strings.TrimSpace(os.Getenv(runtimeEnvConnectionsFile)); path != "" {
			if body, err := os.ReadFile(path); err == nil && json.Valid(body) {
				return body, nil
			}
		}
	}
	return runtimeGetJSON("/api/v1/runtime/connections", nil)
}

func runtimeGetJSON(path string, query url.Values) ([]byte, error) {
	return runtimeRequestJSON(http.MethodGet, path, query, nil)
}

func runtimePostJSON(path string, query url.Values, body []byte) ([]byte, int, error) {
	respBody, status, err := runtimeRequestJSONWithStatus(http.MethodPost, path, query, body)
	return respBody, status, err
}

func runtimeRequestJSON(method, path string, query url.Values, body []byte) ([]byte, error) {
	respBody, _, err := runtimeRequestJSONWithStatus(method, path, query, body)
	return respBody, err
}

func runtimeRequestJSONWithStatus(method, path string, query url.Values, body []byte) ([]byte, int, error) {
	apiURL := strings.TrimRight(strings.TrimSpace(os.Getenv(runtimeEnvAPIURL)), "/")
	token := strings.TrimSpace(os.Getenv(runtimeEnvAgentToken))
	if apiURL == "" || token == "" {
		return nil, 0, fmt.Errorf("%s and %s are required in agent runtime", runtimeEnvAPIURL, runtimeEnvAgentToken)
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
	client := http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxRuntimeCLIJSONBody+1))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if len(respBody) > maxRuntimeCLIJSONBody {
		return nil, resp.StatusCode, fmt.Errorf("runtime response too large")
	}
	return respBody, resp.StatusCode, nil
}

func printRuntimeConnectionsTable(body []byte) error {
	var doc runtimeConnectionDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, "ID\tPROVIDER\tNAME\tALIAS")
	for _, c := range doc.Connections {
		fmt.Fprintf(os.Stdout, "%s\t%s\t%s\t%s\n", c.ID, c.Provider, c.ConnectionName, c.Runtime.Alias)
	}
	return nil
}

func readRuntimeRequestBody(data, file string) ([]byte, error) {
	if strings.TrimSpace(data) != "" && strings.TrimSpace(file) != "" {
		return nil, fmt.Errorf("use only one of --data or --file")
	}
	var body []byte
	var err error
	switch {
	case strings.TrimSpace(data) != "":
		body = []byte(data)
	case strings.TrimSpace(file) == "-":
		body, err = io.ReadAll(io.LimitReader(os.Stdin, maxRuntimeCLIJSONBody+1))
	case strings.TrimSpace(file) != "":
		body, err = os.ReadFile(file)
	default:
		return nil, fmt.Errorf("one of --data or --file is required")
	}
	if err != nil {
		return nil, err
	}
	if len(body) > maxRuntimeCLIJSONBody {
		return nil, fmt.Errorf("runtime request body too large")
	}
	if !json.Valid(body) {
		return nil, fmt.Errorf("runtime request body must be valid JSON")
	}
	return body, nil
}

func resolveRuntimeConnectionTarget(value string) (runtimeConnectionTarget, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return runtimeConnectionTarget{}, fmt.Errorf("connection is required")
	}
	if body, err := runtimeConnectionsBody(false); err == nil {
		var doc runtimeConnectionDocument
		if json.Unmarshal(body, &doc) == nil {
			for _, c := range doc.Connections {
				if c.ID == value {
					return runtimeConnectionTarget{ID: value}, nil
				}
				if c.Runtime.Alias == value {
					return runtimeConnectionTarget{Alias: value}, nil
				}
			}
		}
	}
	if strings.HasPrefix(value, "conn_") {
		return runtimeConnectionTarget{ID: value}, nil
	}
	return runtimeConnectionTarget{Alias: value}, nil
}
