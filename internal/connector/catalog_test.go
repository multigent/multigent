package connector

import "testing"

func TestDefaultProvidersIncludeActionCatalogs(t *testing.T) {
	providers := map[string]Provider{}
	for _, provider := range Defaults() {
		providers[provider.Provider] = provider
	}
	for _, providerID := range []string{"github", "feishu", "lark", "dingtalk_bot"} {
		provider, ok := providers[providerID]
		if !ok {
			t.Fatalf("provider %q missing", providerID)
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
