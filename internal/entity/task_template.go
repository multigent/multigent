package entity

import (
	"fmt"
	"math/rand"
	"time"
)

type TaskTemplate struct {
	ID          string `json:"id" yaml:"id"`
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	Project  string   `json:"project,omitempty" yaml:"project,omitempty"`
	Type     string   `json:"type,omitempty" yaml:"type,omitempty"`
	Priority int      `json:"priority" yaml:"priority"`
	Labels   []string `json:"labels,omitempty" yaml:"labels,omitempty"`

	TitleTemplate       string `json:"titleTemplate" yaml:"title_template"`
	DescriptionTemplate string `json:"descriptionTemplate,omitempty" yaml:"description_template,omitempty"`
	PromptTemplate      string `json:"promptTemplate" yaml:"prompt_template"`

	WorkflowDefinitionID  string                          `json:"workflowDefinitionId,omitempty" yaml:"workflow_definition_id,omitempty"`
	WorkflowActorBindings map[string]WorkflowActorBinding `json:"workflowActorBindings,omitempty" yaml:"workflow_actor_bindings,omitempty"`
	Variables             []TaskTemplateVariable          `json:"variables,omitempty" yaml:"variables,omitempty"`

	CreatedAt time.Time `json:"createdAt" yaml:"created_at"`
	UpdatedAt time.Time `json:"updatedAt" yaml:"updated_at"`
}

type TaskTemplateVariable struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Required    bool   `json:"required,omitempty" yaml:"required,omitempty"`
	Default     string `json:"default,omitempty" yaml:"default,omitempty"`
}

func NewTaskTemplateID() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return fmt.Sprintf("tt-%s", string(b))
}
