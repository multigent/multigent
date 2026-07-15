package connector

const (
	AuthNoAuth           = "no_auth"
	AuthAPIKey           = "api_key"
	AuthCustomCredential = "custom_credential"
	AuthOAuth2           = "oauth2"
)

type Provider struct {
	Provider    string           `json:"provider"`
	DisplayName string           `json:"displayName"`
	AuthTypes   []string         `json:"authTypes"`
	Fields      []ProviderField  `json:"fields,omitempty"`
	OAuth       *OAuth2Config    `json:"oauth,omitempty"`
	Actions     []ProviderAction `json:"actions,omitempty"`
	Enabled     bool             `json:"enabled"`
}

type ProviderField struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	InputType string `json:"inputType"`
	Required  bool   `json:"required"`
	Secret    bool   `json:"secret"`
}

type OAuth2Config struct {
	AuthorizationURL string          `json:"authorizationUrl"`
	TokenURL         string          `json:"tokenUrl"`
	Scopes           []string        `json:"scopes,omitempty"`
	ScopeSeparator   string          `json:"scopeSeparator,omitempty"`
	PKCE             bool            `json:"pkce,omitempty"`
	ClientFields     []ProviderField `json:"clientFields,omitempty"`
}

type ProviderAction struct {
	Name        string         `json:"name"`
	DisplayName string         `json:"displayName"`
	Description string         `json:"description,omitempty"`
	Method      string         `json:"method"`
	Endpoint    string         `json:"endpoint"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

func Defaults() []Provider {
	return []Provider{
		{
			Provider:    "github",
			DisplayName: "GitHub",
			AuthTypes:   []string{AuthAPIKey, AuthOAuth2},
			Fields: []ProviderField{
				{Key: "apiKey", Label: "Personal access token", InputType: "password", Required: true, Secret: true},
			},
			OAuth: &OAuth2Config{
				AuthorizationURL: "https://github.com/login/oauth/authorize",
				TokenURL:         "https://github.com/login/oauth/access_token",
				Scopes:           []string{"repo", "read:user", "user:email"},
				ScopeSeparator:   " ",
			},
			Actions: []ProviderAction{
				{
					Name:        "get_authenticated_user",
					DisplayName: "Get authenticated user",
					Description: "Verify the GitHub credential and read the authenticated account profile.",
					Method:      "GET",
					Endpoint:    "/user",
					InputSchema: objectSchema(nil, nil),
				},
				{
					Name:        "list_repository_issues",
					DisplayName: "List repository issues",
					Description: "List issues for a repository the credential can access.",
					Method:      "GET",
					Endpoint:    "/repos/{owner}/{repo}/issues",
					InputSchema: objectSchema(map[string]any{
						"owner": stringSchema("Repository owner login."),
						"repo":  stringSchema("Repository name."),
						"state": enumStringSchema("Issue state filter.", []string{"open", "closed", "all"}),
					}, []string{"owner", "repo"}),
				},
				{
					Name:        "create_repository_issue",
					DisplayName: "Create repository issue",
					Description: "Create a new issue in a repository.",
					Method:      "POST",
					Endpoint:    "/repos/{owner}/{repo}/issues",
					InputSchema: objectSchema(map[string]any{
						"owner": stringSchema("Repository owner login."),
						"repo":  stringSchema("Repository name."),
						"title": stringSchema("Issue title."),
						"body":  stringSchema("Issue body in Markdown."),
					}, []string{"owner", "repo", "title"}),
				},
			},
			Enabled: true,
		},
		{
			Provider:    "feishu",
			DisplayName: "Feishu / Lark",
			AuthTypes:   []string{AuthCustomCredential},
			Fields: []ProviderField{
				{Key: "baseUrl", Label: "OpenAPI base URL", InputType: "text"},
				{Key: "appId", Label: "App ID", InputType: "text", Required: true},
				{Key: "appSecret", Label: "App Secret", InputType: "password", Required: true, Secret: true},
			},
			Actions: feishuActions(),
			Enabled: true,
		},
		{
			Provider:    "lark",
			DisplayName: "Lark",
			AuthTypes:   []string{AuthCustomCredential},
			Fields: []ProviderField{
				{Key: "baseUrl", Label: "OpenAPI base URL", InputType: "text"},
				{Key: "appId", Label: "App ID", InputType: "text", Required: true},
				{Key: "appSecret", Label: "App Secret", InputType: "password", Required: true, Secret: true},
			},
			Actions: feishuActions(),
			Enabled: true,
		},
		{
			Provider:    "linear",
			DisplayName: "Linear",
			AuthTypes:   []string{AuthAPIKey},
			Fields: []ProviderField{
				{Key: "apiKey", Label: "API key", InputType: "password", Required: true, Secret: true},
			},
			Actions: []ProviderAction{
				{
					Name:        "list_viewer_assigned_issues",
					DisplayName: "List viewer assigned issues",
					Description: "Query Linear issues assigned to the authenticated viewer.",
					Method:      "POST",
					Endpoint:    "/graphql",
					InputSchema: objectSchema(map[string]any{
						"first": numberSchema("Maximum number of issues to return."),
					}, nil),
				},
			},
			Enabled: true,
		},
		{
			Provider:    "dingtalk_bot",
			DisplayName: "DingTalk Bot",
			AuthTypes:   []string{AuthAPIKey},
			Fields: []ProviderField{
				{Key: "apiKey", Label: "Webhook access token or URL", InputType: "password", Required: true, Secret: true},
				{Key: "signingSecret", Label: "Signing secret", InputType: "password", Secret: true},
			},
			Actions: []ProviderAction{
				{
					Name:        "send_text_message",
					DisplayName: "Send text message",
					Description: "Send a text message through a DingTalk bot webhook.",
					Method:      "POST",
					Endpoint:    "/robot/send",
					InputSchema: objectSchema(map[string]any{
						"text": stringSchema("Text content to send."),
					}, []string{"text"}),
				},
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
		{
			Provider:    "custom-http",
			DisplayName: "Custom HTTP API",
			AuthTypes:   []string{AuthCustomCredential},
			Fields: []ProviderField{
				{Key: "baseUrl", Label: "Base URL", InputType: "text", Required: true},
				{Key: "apiKey", Label: "API key", InputType: "password", Secret: true},
				{Key: "authHeader", Label: "Auth header", InputType: "text"},
				{Key: "authScheme", Label: "Auth scheme", InputType: "text"},
			},
			Enabled: true,
		},
	}
}

func feishuActions() []ProviderAction {
	return []ProviderAction{
		{
			Name:        "list_wiki_spaces",
			DisplayName: "List wiki spaces",
			Description: "List wiki spaces visible to the configured Feishu/Lark app.",
			Method:      "GET",
			Endpoint:    "/open-apis/wiki/v2/spaces",
			InputSchema: objectSchema(map[string]any{
				"page_size": numberSchema("Number of spaces to return."),
			}, nil),
		},
		{
			Name:        "send_im_message",
			DisplayName: "Send IM message",
			Description: "Send an IM message through the Feishu/Lark OpenAPI.",
			Method:      "POST",
			Endpoint:    "/open-apis/im/v1/messages",
			InputSchema: objectSchema(map[string]any{
				"receive_id_type": stringSchema("Receiver ID type, for example open_id, user_id, union_id, email, chat_id."),
				"receive_id":      stringSchema("Receiver identifier."),
				"msg_type":        stringSchema("Message type, for example text."),
				"content":         stringSchema("Message content JSON string required by Feishu/Lark."),
			}, []string{"receive_id_type", "receive_id", "msg_type", "content"}),
		},
	}
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
	}
	if len(properties) > 0 {
		schema["properties"] = properties
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func stringSchema(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func numberSchema(description string) map[string]any {
	return map[string]any{"type": "number", "description": description}
}

func enumStringSchema(description string, values []string) map[string]any {
	return map[string]any{"type": "string", "description": description, "enum": values}
}

func SupportsAuth(provider Provider, authType string) bool {
	for _, typ := range provider.AuthTypes {
		if typ == authType {
			return true
		}
	}
	return false
}
