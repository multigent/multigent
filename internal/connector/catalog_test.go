package connector

import "testing"

func TestDefaultProvidersIncludeActionCatalogs(t *testing.T) {
	providers := map[string]Provider{}
	for _, provider := range Defaults() {
		providers[provider.Provider] = provider
	}
	for _, providerID := range []string{
		"github", "gitlab", "gitee", "feishu", "lark", "linear", "notion", "dingtalk_bot",
		"figma", "airtable", "asana", "clickup", "sentry", "vercel",
	} {
		provider, ok := providers[providerID]
		if !ok {
			t.Fatalf("provider %q missing", providerID)
		}
		if provider.ComingSoon {
			t.Fatalf("provider %q should be available", providerID)
		}
		if len(provider.Actions) == 0 {
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
		{provider: "figma", want: RuntimeAdapterMCPGateway},
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
