package connector

const (
	AuthNoAuth           = "no_auth"
	AuthAPIKey           = "api_key"
	AuthCustomCredential = "custom_credential"
	AuthOAuth2           = "oauth2"
)

type Provider struct {
	Provider        string               `json:"provider"`
	DisplayName     string               `json:"displayName"`
	Description     string               `json:"description,omitempty"`
	Category        string               `json:"category,omitempty"`
	AuthTypes       []string             `json:"authTypes"`
	Fields          []ProviderField      `json:"fields,omitempty"`
	OAuth           *OAuth2Config        `json:"oauth,omitempty"`
	RuntimeAdapters []ToolRuntimeAdapter `json:"runtimeAdapters,omitempty"`
	Actions         []ProviderAction     `json:"actions,omitempty"`
	Guides          []ProviderGuide      `json:"guides,omitempty"`
	ComingSoon      bool                 `json:"comingSoon,omitempty"`
	Enabled         bool                 `json:"enabled"`
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

const (
	RuntimeAdapterCLI        = "cli"
	RuntimeAdapterMCPGateway = "mcp_gateway"
	RuntimeAdapterHTTPAction = "http_action"
	RuntimeAdapterSkillOnly  = "skill_only"

	CredentialMaterializeServerSide  = "server_side"
	CredentialMaterializeRuntimeEnv  = "runtime_env"
	CredentialMaterializeRuntimeFile = "runtime_file"
)

type ToolRuntimeAdapter struct {
	Type                  string                 `json:"type"`
	Priority              int                    `json:"priority"`
	Description           string                 `json:"description,omitempty"`
	Skills                []string               `json:"skills,omitempty"`
	CLI                   *ToolCLIAdapter        `json:"cli,omitempty"`
	MCPGateway            *ToolMCPGatewayAdapter `json:"mcpGateway,omitempty"`
	HTTPAction            *ToolHTTPActionAdapter `json:"httpAction,omitempty"`
	CredentialMaterialize string                 `json:"credentialMaterialize,omitempty"`
	Audit                 ToolRuntimeAuditPolicy `json:"audit,omitempty"`
}

type ToolCLIAdapter struct {
	Binary      string               `json:"binary"`
	Installer   *ToolInstallerSpec   `json:"installer,omitempty"`
	ConfigFiles []ToolConfigFileSpec `json:"configFiles,omitempty"`
}

type ToolInstallerSpec struct {
	Type    string   `json:"type"`
	Package string   `json:"package,omitempty"`
	Version string   `json:"version,omitempty"`
	Command []string `json:"command,omitempty"`
	Check   []string `json:"check,omitempty"`
}

type ToolConfigFileSpec struct {
	Path        string `json:"path"`
	Format      string `json:"format,omitempty"`
	Description string `json:"description,omitempty"`
}

type ToolMCPGatewayAdapter struct {
	ToolNamespace string   `json:"toolNamespace"`
	ServerPackage string   `json:"serverPackage,omitempty"`
	ServerCommand []string `json:"serverCommand,omitempty"`
}

type ToolHTTPActionAdapter struct {
	ActionNames []string `json:"actionNames,omitempty"`
}

type ToolRuntimeAuditPolicy struct {
	CommandAudit string `json:"commandAudit,omitempty"`
	ProxyAudit   string `json:"proxyAudit,omitempty"`
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
	providers := []Provider{
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
		staticPATProvider("gitlab", "GitLab", "Developer Tools", "Use GitLab repositories, merge requests, issues, and CI/CD context.", "Personal access token", "Create a personal access token from GitLab preferences. Prefer project-scoped tokens for production workspaces.", "GitLab access tokens", "https://docs.gitlab.com/user/profile/personal_access_tokens/", gitLabActions()),
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
			Actions: giteeActions(),
			Enabled: true,
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
		staticPATProvider("notion", "Notion", "Knowledge And Docs", "Use Notion pages, databases, knowledge bases, and lightweight product docs.", "Internal integration token", "Create an internal integration in Notion, copy its token, and explicitly share the target pages or databases with that integration.", "Notion integrations", "https://www.notion.so/my-integrations", notionActions()),
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
		staticPATProvider("figma", "Figma", "Design And Data", "Use Figma files, components, comments, and design handoff context.", "Personal access token", "Create a Figma personal access token from account settings. Grant only file read scopes until write actions are explicitly needed.", "Figma personal access tokens", "https://www.figma.com/developers/api#access-tokens", figmaActions()),
		staticPATProvider("airtable", "Airtable", "Design And Data", "Use Airtable bases for operations tables, CRM data, and lightweight internal systems.", "Personal access token", "Create a scoped personal access token in Airtable developer hub and limit it to specific bases.", "Airtable personal access tokens", "https://airtable.com/create/tokens", airtableActions()),
		staticPATProvider("asana", "Asana", "Project Management", "Use Asana tasks, projects, sections, and non-engineering project workflows.", "Personal access token", "Create a personal access token from Asana developer console. Prefer least-privilege project access.", "Asana personal access tokens", "https://developers.asana.com/docs/personal-access-token", asanaActions()),
		staticPATProvider("clickup", "ClickUp", "Project Management", "Use ClickUp tasks, lists, spaces, and cross-functional work tracking.", "API token", "Create an API token in ClickUp personal settings. Use workspace-level access carefully.", "ClickUp API token", "https://clickup.com/api", clickUpActions()),
		staticPATProvider("sentry", "Sentry", "Developer Tools", "Use Sentry issues, releases, error events, and triage signals.", "Auth token", "Create an internal integration or auth token in Sentry settings. Start with org:read, project:read, and event:read scopes.", "Sentry auth tokens", "https://sentry.io/settings/account/api/auth-tokens/", sentryActions()),
		staticPATProvider("vercel", "Vercel", "Developer Tools", "Use Vercel projects, deployments, checks, and release status.", "Access token", "Create an access token in Vercel account settings. Use team-scoped tokens when possible.", "Vercel access tokens", "https://vercel.com/account/tokens", vercelActions()),
	}
	for i := range providers {
		providers[i].RuntimeAdapters = DefaultRuntimeAdapters(providers[i])
	}
	return providers
}

func DefaultRuntimeAdapters(provider Provider) []ToolRuntimeAdapter {
	switch provider.Provider {
	case "feishu", "lark":
		return []ToolRuntimeAdapter{
			{
				Type:        RuntimeAdapterCLI,
				Priority:    100,
				Description: "Use lark-cli with bundled collaboration skills for docs, wiki, IM, tasks, calendar, and drive workflows.",
				Skills:      []string{"lark-doc", "lark-im", "lark-task", "lark-drive", "lark-wiki", "lark-calendar"},
				CLI: &ToolCLIAdapter{
					Binary: "lark-cli",
					Installer: &ToolInstallerSpec{
						Type:    "npm",
						Package: "@larksuite/cli",
						Version: "latest",
						Check:   []string{"lark-cli --version"},
					},
					ConfigFiles: []ToolConfigFileSpec{
						{Path: "~/.lark-cli/config.json", Format: "json", Description: "Agent-scoped Lark/Feishu CLI credential config."},
					},
				},
				CredentialMaterialize: CredentialMaterializeRuntimeFile,
				Audit:                 ToolRuntimeAuditPolicy{CommandAudit: "best_effort", ProxyAudit: "available"},
			},
			httpActionAdapter(provider.Actions, 10),
		}
	case "github":
		return []ToolRuntimeAdapter{
			{
				Type:        RuntimeAdapterCLI,
				Priority:    100,
				Description: "Use GitHub CLI for repositories, issues, pull requests, releases, and workflow runs.",
				Skills:      []string{"github"},
				CLI: &ToolCLIAdapter{
					Binary: "gh",
					Installer: &ToolInstallerSpec{
						Type:    "system",
						Package: "gh",
						Version: "latest",
						Check:   []string{"gh --version"},
					},
					ConfigFiles: []ToolConfigFileSpec{
						{Path: "~/.config/gh/hosts.yml", Format: "yaml", Description: "Agent-scoped GitHub CLI host credential config."},
					},
				},
				CredentialMaterialize: CredentialMaterializeRuntimeFile,
				Audit:                 ToolRuntimeAuditPolicy{CommandAudit: "best_effort", ProxyAudit: "available"},
			},
			httpActionAdapter(provider.Actions, 20),
		}
	case "gitlab":
		return []ToolRuntimeAdapter{
			{
				Type:        RuntimeAdapterCLI,
				Priority:    90,
				Description: "Use GitLab CLI when available for merge requests, issues, pipelines, and repository workflows.",
				Skills:      []string{"gitlab"},
				CLI: &ToolCLIAdapter{
					Binary: "glab",
					Installer: &ToolInstallerSpec{
						Type:    "system",
						Package: "glab",
						Version: "latest",
						Check:   []string{"glab --version"},
					},
					ConfigFiles: []ToolConfigFileSpec{
						{Path: "~/.config/glab-cli/config.yml", Format: "yaml", Description: "Agent-scoped GitLab CLI credential config."},
					},
				},
				CredentialMaterialize: CredentialMaterializeRuntimeFile,
				Audit:                 ToolRuntimeAuditPolicy{CommandAudit: "best_effort", ProxyAudit: "available"},
			},
			httpActionAdapter(provider.Actions, 20),
		}
	case "figma":
		return []ToolRuntimeAdapter{
			{
				Type:                  RuntimeAdapterMCPGateway,
				Priority:              100,
				Description:           "Use Multigent MCP Gateway to discover and call Figma tools without injecting a large native MCP tool list.",
				Skills:                []string{"figma"},
				CredentialMaterialize: CredentialMaterializeServerSide,
				MCPGateway: &ToolMCPGatewayAdapter{
					ToolNamespace: "figma",
					ServerPackage: "figma-mcp",
				},
				Audit: ToolRuntimeAuditPolicy{ProxyAudit: "required"},
			},
			httpActionAdapter(provider.Actions, 20),
		}
	case "custom-mcp":
		return []ToolRuntimeAdapter{
			{
				Type:                  RuntimeAdapterMCPGateway,
				Priority:              100,
				Description:           "Use Multigent MCP Gateway to call the configured upstream MCP server with scoped runtime authorization.",
				CredentialMaterialize: CredentialMaterializeServerSide,
				MCPGateway: &ToolMCPGatewayAdapter{
					ToolNamespace: "custom-mcp",
				},
				Audit: ToolRuntimeAuditPolicy{ProxyAudit: "required"},
			},
		}
	default:
		if len(provider.Actions) > 0 {
			return []ToolRuntimeAdapter{httpActionAdapter(provider.Actions, 50)}
		}
		return []ToolRuntimeAdapter{
			{
				Type:                  RuntimeAdapterSkillOnly,
				Priority:              10,
				Description:           "Use bundled skills and provider guidance. No executable runtime adapter is configured yet.",
				CredentialMaterialize: CredentialMaterializeServerSide,
				Audit:                 ToolRuntimeAuditPolicy{ProxyAudit: "none"},
			},
		}
	}
}

func httpActionAdapter(actions []ProviderAction, priority int) ToolRuntimeAdapter {
	names := make([]string, 0, len(actions))
	for _, action := range actions {
		if action.Name != "" {
			names = append(names, action.Name)
		}
	}
	return ToolRuntimeAdapter{
		Type:                  RuntimeAdapterHTTPAction,
		Priority:              priority,
		Description:           "Use Multigent runtime action proxy for provider API calls. Raw credentials stay server-side.",
		HTTPAction:            &ToolHTTPActionAdapter{ActionNames: names},
		CredentialMaterialize: CredentialMaterializeServerSide,
		Audit:                 ToolRuntimeAuditPolicy{ProxyAudit: "required"},
	}
}

func staticPATProvider(provider, displayName, category, description, fieldLabel, guideBody, linkLabel, linkURL string, actions []ProviderAction) Provider {
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
		Actions: actions,
		Enabled: true,
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

func gitLabActions() []ProviderAction {
	return []ProviderAction{
		{Name: "get_authenticated_user", DisplayName: "Get authenticated user", Description: "Verify the GitLab credential and read the authenticated user profile.", Method: "GET", Endpoint: "/user", InputSchema: objectSchema(nil, nil)},
		{Name: "list_project_issues", DisplayName: "List project issues", Description: "List issues from a GitLab project.", Method: "GET", Endpoint: "/projects/{project_id}/issues", InputSchema: objectSchema(map[string]any{
			"project_id": stringSchema("URL-encoded numeric project ID or namespace/project path."),
			"state":      enumStringSchema("Issue state filter.", []string{"opened", "closed", "all"}),
		}, []string{"project_id"})},
		{Name: "create_project_issue", DisplayName: "Create project issue", Description: "Create an issue in a GitLab project.", Method: "POST", Endpoint: "/projects/{project_id}/issues", InputSchema: objectSchema(map[string]any{
			"project_id":  stringSchema("URL-encoded numeric project ID or namespace/project path."),
			"title":       stringSchema("Issue title."),
			"description": stringSchema("Issue description in Markdown."),
		}, []string{"project_id", "title"})},
	}
}

func giteeActions() []ProviderAction {
	return []ProviderAction{
		{Name: "get_authenticated_user", DisplayName: "Get authenticated user", Description: "Verify the Gitee credential and read the authenticated user profile.", Method: "GET", Endpoint: "/user", InputSchema: objectSchema(nil, nil)},
		{Name: "list_repository_issues", DisplayName: "List repository issues", Description: "List issues for a Gitee repository.", Method: "GET", Endpoint: "/repos/{owner}/{repo}/issues", InputSchema: objectSchema(map[string]any{
			"owner": stringSchema("Repository owner login."),
			"repo":  stringSchema("Repository name."),
			"state": enumStringSchema("Issue state filter.", []string{"open", "progressing", "closed", "rejected"}),
		}, []string{"owner", "repo"})},
	}
}

func notionActions() []ProviderAction {
	return []ProviderAction{
		{Name: "get_bot_user", DisplayName: "Get bot user", Description: "Verify the Notion integration token and read the bot user profile.", Method: "GET", Endpoint: "/users/me", InputSchema: objectSchema(nil, nil)},
		{Name: "search", DisplayName: "Search workspace", Description: "Search pages and databases shared with the integration.", Method: "POST", Endpoint: "/search", InputSchema: objectSchema(map[string]any{
			"query": stringSchema("Optional search query."),
		}, nil)},
	}
}

func figmaActions() []ProviderAction {
	return []ProviderAction{
		{Name: "get_authenticated_user", DisplayName: "Get authenticated user", Description: "Verify the Figma credential and read the authenticated user profile.", Method: "GET", Endpoint: "/me", InputSchema: objectSchema(nil, nil)},
		{Name: "get_file", DisplayName: "Get file", Description: "Read a Figma file by key.", Method: "GET", Endpoint: "/files/{file_key}", InputSchema: objectSchema(map[string]any{
			"file_key": stringSchema("Figma file key."),
		}, []string{"file_key"})},
	}
}

func airtableActions() []ProviderAction {
	return []ProviderAction{
		{Name: "get_authenticated_user", DisplayName: "Get authenticated user", Description: "Verify the Airtable token and read the authenticated account profile.", Method: "GET", Endpoint: "/meta/whoami", InputSchema: objectSchema(nil, nil)},
		{Name: "list_bases", DisplayName: "List bases", Description: "List Airtable bases visible to the token.", Method: "GET", Endpoint: "/meta/bases", InputSchema: objectSchema(nil, nil)},
	}
}

func asanaActions() []ProviderAction {
	return []ProviderAction{
		{Name: "get_authenticated_user", DisplayName: "Get authenticated user", Description: "Verify the Asana token and read the authenticated user profile.", Method: "GET", Endpoint: "/users/me", InputSchema: objectSchema(nil, nil)},
		{Name: "list_my_tasks", DisplayName: "List my tasks", Description: "List tasks assigned to the authenticated user in a workspace.", Method: "GET", Endpoint: "/tasks", InputSchema: objectSchema(map[string]any{
			"workspace": stringSchema("Asana workspace GID."),
			"assignee":  stringSchema("Assignee GID, or me."),
		}, []string{"workspace", "assignee"})},
	}
}

func clickUpActions() []ProviderAction {
	return []ProviderAction{
		{Name: "get_authenticated_user", DisplayName: "Get authenticated user", Description: "Verify the ClickUp token and read the authenticated user profile.", Method: "GET", Endpoint: "/user", InputSchema: objectSchema(nil, nil)},
		{Name: "list_teams", DisplayName: "List workspaces", Description: "List ClickUp workspaces visible to the token.", Method: "GET", Endpoint: "/team", InputSchema: objectSchema(nil, nil)},
	}
}

func vercelActions() []ProviderAction {
	return []ProviderAction{
		{Name: "get_authenticated_user", DisplayName: "Get authenticated user", Description: "Verify the Vercel token and read the authenticated user profile.", Method: "GET", Endpoint: "/v2/user", InputSchema: objectSchema(nil, nil)},
		{Name: "list_deployments", DisplayName: "List deployments", Description: "List Vercel deployments visible to the token.", Method: "GET", Endpoint: "/v6/deployments", InputSchema: objectSchema(nil, nil)},
	}
}

func sentryActions() []ProviderAction {
	return []ProviderAction{
		{Name: "list_organizations", DisplayName: "List organizations", Description: "Verify the Sentry token and list organizations visible to it.", Method: "GET", Endpoint: "/organizations/", InputSchema: objectSchema(nil, nil)},
		{Name: "list_organization_issues", DisplayName: "List organization issues", Description: "List issues from a Sentry organization.", Method: "GET", Endpoint: "/organizations/{organization_slug}/issues/", InputSchema: objectSchema(map[string]any{
			"organization_slug": stringSchema("Sentry organization slug."),
			"query":             stringSchema("Optional Sentry issue search query."),
		}, []string{"organization_slug"})},
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
