package connector

const (
	AuthNoAuth           = "no_auth"
	AuthAPIKey           = "api_key"
	AuthCustomCredential = "custom_credential"
	AuthOAuth2           = "oauth2"
)

type Provider struct {
	Provider    string          `json:"provider"`
	DisplayName string          `json:"displayName"`
	AuthTypes   []string        `json:"authTypes"`
	Fields      []ProviderField `json:"fields,omitempty"`
	Enabled     bool            `json:"enabled"`
}

type ProviderField struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	InputType string `json:"inputType"`
	Required  bool   `json:"required"`
	Secret    bool   `json:"secret"`
}

func Defaults() []Provider {
	return []Provider{
		{
			Provider:    "github",
			DisplayName: "GitHub",
			AuthTypes:   []string{AuthAPIKey},
			Fields: []ProviderField{
				{Key: "apiKey", Label: "Personal access token", InputType: "password", Required: true, Secret: true},
			},
			Enabled: true,
		},
		{
			Provider:    "feishu",
			DisplayName: "Feishu / Lark",
			AuthTypes:   []string{AuthCustomCredential},
			Fields: []ProviderField{
				{Key: "appId", Label: "App ID", InputType: "text", Required: true},
				{Key: "appSecret", Label: "App Secret", InputType: "password", Required: true, Secret: true},
			},
			Enabled: true,
		},
		{
			Provider:    "linear",
			DisplayName: "Linear",
			AuthTypes:   []string{AuthAPIKey},
			Fields: []ProviderField{
				{Key: "apiKey", Label: "API key", InputType: "password", Required: true, Secret: true},
			},
			Enabled: true,
		},
		{
			Provider:    "custom-mcp",
			DisplayName: "Custom MCP Server",
			AuthTypes:   []string{AuthCustomCredential, AuthNoAuth},
			Fields: []ProviderField{
				{Key: "serverUrl", Label: "Server URL", InputType: "text", Required: true},
				{Key: "token", Label: "Token", InputType: "password", Secret: true},
			},
			Enabled: true,
		},
	}
}

func SupportsAuth(provider Provider, authType string) bool {
	for _, typ := range provider.AuthTypes {
		if typ == authType {
			return true
		}
	}
	return false
}
