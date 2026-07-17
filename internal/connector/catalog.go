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
	Description string           `json:"description,omitempty"`
	Category    string           `json:"category,omitempty"`
	AuthTypes   []string         `json:"authTypes"`
	Fields      []ProviderField  `json:"fields,omitempty"`
	OAuth       *OAuth2Config    `json:"oauth,omitempty"`
	Actions     []ProviderAction `json:"actions,omitempty"`
	Guides      []ProviderGuide  `json:"guides,omitempty"`
	ComingSoon  bool             `json:"comingSoon,omitempty"`
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

type ProviderGuide struct {
	Title string              `json:"title"`
	Body  string              `json:"body"`
	Links []ProviderGuideLink `json:"links,omitempty"`
}

type ProviderGuideLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

func Defaults() []Provider {
	return []Provider{
		{
			Provider:    "github",
			DisplayName: "GitHub",
			Description: "Use repositories, issues, pull requests, release signals, and code collaboration context.",
			Category:    "Developer Tools",
			AuthTypes:   []string{AuthAPIKey, AuthOAuth2},
			Fields: []ProviderField{
				{Key: "apiKey", Label: "Personal access token", InputType: "password", Required: true, Secret: true},
			},
			OAuth: &OAuth2Config{
				AuthorizationURL: "https://github.com/login/oauth/authorize",
				TokenURL:         "https://github.com/login/oauth/access_token",
				Scopes:           []string{"repo", "read:org", "gist"},
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
			Guides: []ProviderGuide{
				credentialGuide("Personal access token", "Create a fine-grained personal access token in GitHub settings. Start with repository metadata and issues permissions; add pull request and contents permissions only when an agent workflow needs them.", "GitHub token settings", "https://github.com/settings/tokens"),
			},
			Enabled: true,
		},
		staticPATProvider("gitlab", "GitLab", "Developer Tools", "Use GitLab repositories, merge requests, issues, and CI/CD context.", "Personal access token", "Create a personal access token from GitLab preferences. Prefer project-scoped tokens for production workspaces.", "GitLab access tokens", "https://docs.gitlab.com/user/profile/personal_access_tokens/"),
		{
			Provider:    "gitee",
			DisplayName: "Gitee",
			Description: "Use Gitee repositories, issues, pull requests, and domestic code hosting workflows.",
			Category:    "Developer Tools",
			AuthTypes:   []string{AuthAPIKey, AuthOAuth2},
			Fields: []ProviderField{
				{Key: "apiKey", Label: "Personal access token", InputType: "password", Required: true, Secret: true},
			},
			OAuth: &OAuth2Config{
				AuthorizationURL: "https://gitee.com/oauth/authorize",
				TokenURL:         "https://gitee.com/oauth/token",
				Scopes:           []string{"user_info", "projects", "issues", "pull_requests"},
				ScopeSeparator:   " ",
			},
			Guides: []ProviderGuide{
				credentialGuide("Personal access token", "Create a Gitee personal access token with the minimum repository and issue scopes needed by the agent.", "Gitee API docs", "https://gitee.com/api/v5/swagger"),
			},
			ComingSoon: true,
			Enabled:    true,
		},
		{
			Provider:    "feishu",
			DisplayName: "Feishu",
			Description: "Use Feishu OpenAPI for wiki, docs, IM, task, and collaboration workflows in China-region tenants.",
			Category:    "Communication",
			AuthTypes:   []string{AuthCustomCredential},
			Fields: []ProviderField{
				{Key: "baseUrl", Label: "OpenAPI base URL", InputType: "text"},
				{Key: "appId", Label: "App ID", InputType: "text", Required: true},
				{Key: "appSecret", Label: "App Secret", InputType: "password", Required: true, Secret: true},
			},
			Actions: feishuActions(),
			Guides: []ProviderGuide{
				credentialGuide("App credential", "Create an internal app in Feishu developer console, enable the APIs you need, publish it to the tenant, then copy App ID and App Secret. Use https://open.feishu.cn as the default base URL.", "Feishu developer console", "https://open.feishu.cn/app"),
			},
			Enabled: true,
		},
		{
			Provider:    "lark",
			DisplayName: "Lark",
			Description: "Use Lark OpenAPI for wiki, docs, IM, task, and collaboration workflows in global tenants.",
			Category:    "Communication",
			AuthTypes:   []string{AuthCustomCredential},
			Fields: []ProviderField{
				{Key: "baseUrl", Label: "OpenAPI base URL", InputType: "text"},
				{Key: "appId", Label: "App ID", InputType: "text", Required: true},
				{Key: "appSecret", Label: "App Secret", InputType: "password", Required: true, Secret: true},
			},
			Actions: feishuActions(),
			Guides: []ProviderGuide{
				credentialGuide("App credential", "Create an internal app in Lark developer console, enable the APIs you need, publish it to the tenant, then copy App ID and App Secret. Use https://open.larksuite.com as the default base URL.", "Lark developer console", "https://open.larksuite.com/app"),
			},
			Enabled: true,
		},
		{
			Provider:    "linear",
			DisplayName: "Linear",
			Description: "Use Linear issues, projects, cycles, and engineering planning context.",
			Category:    "Project Management",
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
			Guides: []ProviderGuide{
				credentialGuide("API key", "Create a personal API key in Linear account settings. For shared automation, use a dedicated service account when available.", "Linear API settings", "https://linear.app/settings/api"),
			},
			Enabled: true,
		},
		oauthOnlyProvider("jira", "Jira", "Project Management", "Use Jira issues, epics, projects, and enterprise development workflows.", "https://auth.atlassian.com/authorize", "https://auth.atlassian.com/oauth/token", []string{"read:jira-work", "write:jira-work", "read:jira-user"}),
		staticPATProvider("notion", "Notion", "Knowledge And Docs", "Use Notion pages, databases, knowledge bases, and lightweight product docs.", "Internal integration token", "Create an internal integration in Notion, copy its token, and explicitly share the target pages or databases with that integration.", "Notion integrations", "https://www.notion.so/my-integrations"),
		oauthOnlyProvider("slack", "Slack", "Communication", "Use Slack channels, messages, threads, and team notification workflows.", "https://slack.com/oauth/v2/authorize", "https://slack.com/api/oauth.v2.access", []string{"channels:read", "chat:write", "users:read"}),
		{
			Provider:    "dingtalk_bot",
			DisplayName: "DingTalk Bot",
			Description: "Send DingTalk group bot notifications and workflow updates.",
			Category:    "Communication",
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
			Guides: []ProviderGuide{
				credentialGuide("Webhook credential", "Create a custom DingTalk group robot, copy its webhook URL or access token, and add the signing secret if signature validation is enabled.", "DingTalk custom robot", "https://open.dingtalk.com/document/orgapp/custom-robot-access"),
			},
			Enabled: true,
		},
		oauthOnlyProvider("gmail", "Gmail", "Communication", "Use Gmail search, email triage, draft creation, and communication workflows.", "https://accounts.google.com/o/oauth2/v2/auth", "https://oauth2.googleapis.com/token", []string{"https://www.googleapis.com/auth/gmail.readonly", "https://www.googleapis.com/auth/gmail.compose"}),
		oauthOnlyProvider("google_drive", "Google Drive", "Knowledge And Docs", "Use Drive files, folders, attachments, and knowledge sources.", "https://accounts.google.com/o/oauth2/v2/auth", "https://oauth2.googleapis.com/token", []string{"https://www.googleapis.com/auth/drive.readonly"}),
		oauthOnlyProvider("google_docs", "Google Docs", "Knowledge And Docs", "Use Google Docs documents as readable and writable knowledge artifacts.", "https://accounts.google.com/o/oauth2/v2/auth", "https://oauth2.googleapis.com/token", []string{"https://www.googleapis.com/auth/documents", "https://www.googleapis.com/auth/drive.file"}),
		oauthOnlyProvider("google_sheets", "Google Sheets", "Knowledge And Docs", "Use Google Sheets for structured data, lightweight operations tables, and reporting.", "https://accounts.google.com/o/oauth2/v2/auth", "https://oauth2.googleapis.com/token", []string{"https://www.googleapis.com/auth/spreadsheets", "https://www.googleapis.com/auth/drive.file"}),
		oauthOnlyProvider("google_calendar", "Google Calendar", "Communication", "Use calendars, meeting schedules, availability, and reminders.", "https://accounts.google.com/o/oauth2/v2/auth", "https://oauth2.googleapis.com/token", []string{"https://www.googleapis.com/auth/calendar.events", "https://www.googleapis.com/auth/calendar.readonly"}),
		staticPATProvider("figma", "Figma", "Design And Data", "Use Figma files, components, comments, and design handoff context.", "Personal access token", "Create a Figma personal access token from account settings. Grant only file read scopes until write actions are explicitly needed.", "Figma personal access tokens", "https://www.figma.com/developers/api#access-tokens"),
		staticPATProvider("airtable", "Airtable", "Design And Data", "Use Airtable bases for operations tables, CRM data, and lightweight internal systems.", "Personal access token", "Create a scoped personal access token in Airtable developer hub and limit it to specific bases.", "Airtable personal access tokens", "https://airtable.com/create/tokens"),
		staticPATProvider("asana", "Asana", "Project Management", "Use Asana tasks, projects, sections, and non-engineering project workflows.", "Personal access token", "Create a personal access token from Asana developer console. Prefer least-privilege project access.", "Asana personal access tokens", "https://developers.asana.com/docs/personal-access-token"),
		staticPATProvider("clickup", "ClickUp", "Project Management", "Use ClickUp tasks, lists, spaces, and cross-functional work tracking.", "API token", "Create an API token in ClickUp personal settings. Use workspace-level access carefully.", "ClickUp API token", "https://clickup.com/api"),
		oauthOnlyProvider("sentry", "Sentry", "Developer Tools", "Use Sentry issues, releases, error events, and triage signals.", "https://sentry.io/oauth/authorize/", "https://sentry.io/oauth/token/", []string{"event:read", "project:read", "org:read"}),
		staticPATProvider("vercel", "Vercel", "Developer Tools", "Use Vercel projects, deployments, checks, and release status.", "Access token", "Create an access token in Vercel account settings. Use team-scoped tokens when possible.", "Vercel access tokens", "https://vercel.com/account/tokens"),
	}
}

func staticPATProvider(provider, displayName, category, description, fieldLabel, guideBody, linkLabel, linkURL string) Provider {
	return Provider{
		Provider:    provider,
		DisplayName: displayName,
		Description: description,
		Category:    category,
		AuthTypes:   []string{AuthAPIKey},
		Fields: []ProviderField{
			{Key: "apiKey", Label: fieldLabel, InputType: "password", Required: true, Secret: true},
		},
		Guides: []ProviderGuide{
			credentialGuide(fieldLabel, guideBody, linkLabel, linkURL),
		},
		ComingSoon: true,
		Enabled:    true,
	}
}

func oauthOnlyProvider(provider, displayName, category, description, authorizationURL, tokenURL string, scopes []string) Provider {
	return Provider{
		Provider:    provider,
		DisplayName: displayName,
		Description: description,
		Category:    category,
		AuthTypes:   []string{AuthOAuth2},
		OAuth: &OAuth2Config{
			AuthorizationURL: authorizationURL,
			TokenURL:         tokenURL,
			Scopes:           scopes,
			ScopeSeparator:   " ",
			PKCE:             true,
		},
		Guides: []ProviderGuide{
			{Title: "OAuth app", Body: "A workspace admin must configure this provider's OAuth client before users can authorize it. Use least-privilege scopes and a dedicated production OAuth app."},
		},
		ComingSoon: true,
		Enabled:    true,
	}
}

func credentialGuide(title, body, linkLabel, linkURL string) ProviderGuide {
	guide := ProviderGuide{Title: title, Body: body}
	if linkLabel != "" && linkURL != "" {
		guide.Links = []ProviderGuideLink{{Label: linkLabel, URL: linkURL}}
	}
	return guide
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
