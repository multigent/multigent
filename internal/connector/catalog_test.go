package connector

import "testing"

func TestDefaultProvidersIncludeActionCatalogs(t *testing.T) {
	providers := map[string]Provider{}
	for _, provider := range Defaults() {
		providers[provider.Provider] = provider
	}
	for _, providerID := range []string{
		"github", "gitlab", "gitee", "feishu", "lark", "linear", "notion", "dingtalk_bot",
		"figma", "airtable", "asana", "clickup", "sentry", "vercel", "aws", "gcloud", "cloudflare", "exa", "brave_search",
		"ssh_key", "git_ssh", "npm_registry", "docker_registry", "custom-mcp",
	} {
		provider, ok := providers[providerID]
		if !ok {
			t.Fatalf("provider %q missing", providerID)
		}
		if provider.ComingSoon {
			t.Fatalf("provider %q should be available", providerID)
		}
		if len(provider.Actions) == 0 && provider.Provider != "ssh_key" && provider.Provider != "git_ssh" && provider.Provider != "npm_registry" && provider.Provider != "docker_registry" && provider.Provider != "aws" && provider.Provider != "gcloud" && provider.Provider != "custom-mcp" {
			t.Fatalf("provider %q has no actions", providerID)
		}
		for _, action := range provider.Actions {
			if action.Name == "" || action.Method == "" || action.Endpoint == "" {
				t.Fatalf("provider %q has incomplete action: %#v", providerID, action)
			}
			if action.InputSchema == nil {
				t.Fatalf("provider %q action %q missing input schema", providerID, action.Name)
			}
		}
	}
}

func TestDefaultProvidersIncludeRuntimeAdapters(t *testing.T) {
	providers := map[string]Provider{}
	for _, provider := range Defaults() {
		providers[provider.Provider] = provider
	}
	tests := []struct {
		provider string
		want     string
		binary   string
	}{
		{provider: "lark", want: RuntimeAdapterCLI, binary: "lark-cli"},
		{provider: "feishu", want: RuntimeAdapterCLI, binary: "lark-cli"},
		{provider: "github", want: RuntimeAdapterCLI, binary: "gh"},
		{provider: "ssh_key", want: RuntimeAdapterCLI, binary: "ssh"},
		{provider: "git_ssh", want: RuntimeAdapterCLI, binary: "git"},
		{provider: "npm_registry", want: RuntimeAdapterCLI, binary: "npm"},
		{provider: "docker_registry", want: RuntimeAdapterCLI, binary: "docker"},
		{provider: "aws", want: RuntimeAdapterCLI, binary: "aws"},
		{provider: "gcloud", want: RuntimeAdapterCLI, binary: "gcloud"},
		{provider: "cloudflare", want: RuntimeAdapterCLI, binary: "wrangler"},
		{provider: "figma", want: RuntimeAdapterMCPGateway},
		{provider: "custom-mcp", want: RuntimeAdapterMCPGateway},
		{provider: "notion", want: RuntimeAdapterHTTPAction},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			provider, ok := providers[tt.provider]
			if !ok {
				t.Fatalf("provider missing")
			}
			if len(provider.RuntimeAdapters) == 0 {
				t.Fatalf("runtime adapters missing")
			}
			got := provider.RuntimeAdapters[0]
			if got.Type != tt.want {
				t.Fatalf("first adapter=%q, want %q", got.Type, tt.want)
			}
			if tt.binary != "" && (got.CLI == nil || got.CLI.Binary != tt.binary) {
				t.Fatalf("cli adapter=%#v, want binary %q", got.CLI, tt.binary)
			}
		})
	}
}

func TestFigmaProviderIncludesOptionalMCPServerURLField(t *testing.T) {
	var figma Provider
	for _, provider := range Defaults() {
		if provider.Provider == "figma" {
			figma = provider
			break
		}
	}
	if figma.Provider == "" {
		t.Fatalf("figma provider missing")
	}
	found := false
	for _, field := range figma.Fields {
		if field.Key == "mcpServerUrl" {
			found = true
			if field.Required || field.Secret {
				t.Fatalf("mcpServerUrl should be optional and non-secret: %#v", field)
			}
		}
	}
	if !found {
		t.Fatalf("figma mcpServerUrl field missing: %#v", figma.Fields)
	}
}
