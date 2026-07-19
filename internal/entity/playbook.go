package entity

import "time"

type PlaybookTemplate struct {
	ID             string                     `json:"id" yaml:"id"`
	Name           string                     `json:"name" yaml:"name"`
	Description    string                     `json:"description" yaml:"description"`
	Locale         string                     `json:"locale" yaml:"locale"`
	Category       string                     `json:"category" yaml:"category"`
	Complexity     string                     `json:"complexity" yaml:"complexity"`
	Tags           []string                   `json:"tags,omitempty" yaml:"tags,omitempty"`
	Roles          []PlaybookRoleTemplate     `json:"roles,omitempty" yaml:"roles,omitempty"`
	Skills         []PlaybookSkillTemplate    `json:"skills,omitempty" yaml:"skills,omitempty"`
	Workflows      []PlaybookWorkflowTemplate `json:"workflows,omitempty" yaml:"workflows,omitempty"`
	TaskTemplates  []PlaybookTaskTemplate     `json:"taskTemplates,omitempty" yaml:"task_templates,omitempty"`
	RequiredTools  []PlaybookToolRequirement  `json:"requiredTools,omitempty" yaml:"required_tools,omitempty"`
	SetupQuestions []PlaybookSetupQuestion    `json:"setupQuestions,omitempty" yaml:"setup_questions,omitempty"`
	SuccessMetrics []PlaybookMetric           `json:"successMetrics,omitempty" yaml:"success_metrics,omitempty"`
}

type PlaybookRoleTemplate struct {
	ID          string   `json:"id" yaml:"id"`
	Team        string   `json:"team" yaml:"team"`
	Role        string   `json:"role" yaml:"role"`
	Name        string   `json:"name" yaml:"name"`
	Description string   `json:"description" yaml:"description"`
	Prompt      string   `json:"prompt,omitempty" yaml:"prompt,omitempty"`
	Skills      []string `json:"skills,omitempty" yaml:"skills,omitempty"`
}

type PlaybookSkillTemplate struct {
	ID          string `json:"id" yaml:"id"`
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
	Body        string `json:"body,omitempty" yaml:"body,omitempty"`
	Source      string `json:"source,omitempty" yaml:"source,omitempty"`
	LicenseNote string `json:"licenseNote,omitempty" yaml:"license_note,omitempty"`
}

type PlaybookWorkflowTemplate struct {
	ID            string              `json:"id" yaml:"id"`
	Name          string              `json:"name" yaml:"name"`
	Description   string              `json:"description" yaml:"description"`
	Definition    WorkflowTemplate    `json:"definition" yaml:"definition"`
	RoleBindings  map[string]string   `json:"roleBindings,omitempty" yaml:"role_bindings,omitempty"`
	SkillBindings map[string][]string `json:"skillBindings,omitempty" yaml:"skill_bindings,omitempty"`
}

type PlaybookTaskTemplate struct {
	ID             string          `json:"id" yaml:"id"`
	Title          string          `json:"title" yaml:"title"`
	Description    string          `json:"description" yaml:"description"`
	Prompt         string          `json:"prompt" yaml:"prompt"`
	WorkflowID     string          `json:"workflowId,omitempty" yaml:"workflow_id,omitempty"`
	RequiredFields []WorkflowField `json:"requiredFields,omitempty" yaml:"required_fields,omitempty"`
}

type PlaybookToolRequirement struct {
	Provider    string `json:"provider" yaml:"provider"`
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
	Required    bool   `json:"required" yaml:"required"`
}

type PlaybookSetupQuestion struct {
	ID          string   `json:"id" yaml:"id"`
	Question    string   `json:"question" yaml:"question"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
	Options     []string `json:"options,omitempty" yaml:"options,omitempty"`
	Required    bool     `json:"required" yaml:"required"`
}

type PlaybookMetric struct {
	ID          string `json:"id" yaml:"id"`
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
}

type PlaybookInstall struct {
	ID           string                    `json:"id"`
	PlaybookID   string                    `json:"playbookId"`
	PlaybookName string                    `json:"playbookName"`
	Locale       string                    `json:"locale"`
	CreatedBy    string                    `json:"createdBy"`
	CreatedAt    time.Time                 `json:"createdAt"`
	Objects      []PlaybookInstalledObject `json:"objects"`
}

type PlaybookInstalledObject struct {
	Type     string `json:"type"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	ParentID string `json:"parentId,omitempty"`
	Status   string `json:"status"` // created or existing
}

type PlaybookObjectProvenance struct {
	ObjectType   string    `json:"objectType"`
	ObjectID     string    `json:"objectId"`
	ParentID     string    `json:"parentId,omitempty"`
	PlaybookID   string    `json:"playbookId"`
	PlaybookName string    `json:"playbookName"`
	InstallID    string    `json:"installId"`
	InstalledBy  string    `json:"installedBy"`
	InstalledAt  time.Time `json:"installedAt"`
	Status       string    `json:"status"`
}
