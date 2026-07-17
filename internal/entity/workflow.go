package entity

import (
	"fmt"
	"math/rand"
	"time"
)

type WorkflowDefinition struct {
	ID          string         `json:"id" yaml:"id"`
	Name        string         `json:"name" yaml:"name"`
	Description string         `json:"description,omitempty" yaml:"description,omitempty"`
	Version     int            `json:"version" yaml:"version"`
	Scope       string         `json:"scope,omitempty" yaml:"scope,omitempty"` // workspace or project
	Project     string         `json:"project,omitempty" yaml:"project,omitempty"`
	StartStepID string         `json:"startStepId" yaml:"start_step_id"`
	Steps       []WorkflowStep `json:"steps" yaml:"steps"`
	Edges       []WorkflowEdge `json:"edges" yaml:"edges"`
	CreatedAt   time.Time      `json:"createdAt" yaml:"created_at"`
	UpdatedAt   time.Time      `json:"updatedAt" yaml:"updated_at"`
}

type WorkflowStep struct {
	ID           string            `json:"id" yaml:"id"`
	Type         string            `json:"type" yaml:"type"` // agent_task, human_review, branch, join, terminal
	Title        string            `json:"title" yaml:"title"`
	Description  string            `json:"description,omitempty" yaml:"description,omitempty"`
	ActorRole    string            `json:"actorRole,omitempty" yaml:"actor_role,omitempty"`
	InputSchema  string            `json:"inputSchema,omitempty" yaml:"input_schema,omitempty"`
	OutputSchema string            `json:"outputSchema,omitempty" yaml:"output_schema,omitempty"`
	InputFields  []WorkflowField   `json:"inputFields,omitempty" yaml:"input_fields,omitempty"`
	OutputFields []WorkflowField   `json:"outputFields,omitempty" yaml:"output_fields,omitempty"`
	ReviewPolicy string            `json:"reviewPolicy,omitempty" yaml:"review_policy,omitempty"`
	Position     WorkflowPosition  `json:"position" yaml:"position"`
	Config       map[string]string `json:"config,omitempty" yaml:"config,omitempty"`
}

type WorkflowField struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

type WorkflowPosition struct {
	X int `json:"x" yaml:"x"`
	Y int `json:"y" yaml:"y"`
}

type WorkflowEdge struct {
	ID           string                 `json:"id" yaml:"id"`
	From         string                 `json:"from" yaml:"from"`
	To           string                 `json:"to" yaml:"to"`
	Label        string                 `json:"label,omitempty" yaml:"label,omitempty"`
	Policy       string                 `json:"policy,omitempty" yaml:"policy,omitempty"`
	Condition    *WorkflowEdgeCondition `json:"condition,omitempty" yaml:"condition,omitempty"`
	InputMapping map[string]string      `json:"inputMapping,omitempty" yaml:"input_mapping,omitempty"`
	IsDefault    bool                   `json:"isDefault,omitempty" yaml:"is_default,omitempty"`
}

type WorkflowEdgeCondition struct {
	Field    string   `json:"field,omitempty" yaml:"field,omitempty"`
	Operator string   `json:"operator,omitempty" yaml:"operator,omitempty"`
	Value    string   `json:"value,omitempty" yaml:"value,omitempty"`
	Values   []string `json:"values,omitempty" yaml:"values,omitempty"`
}

type WorkflowRun struct {
	ID           string    `json:"id" yaml:"id"`
	DefinitionID string    `json:"definitionId" yaml:"definition_id"`
	Project      string    `json:"project" yaml:"project"`
	TaskID       string    `json:"taskId" yaml:"task_id"`
	Status       string    `json:"status" yaml:"status"`
	ActiveStepID string    `json:"activeStepId,omitempty" yaml:"active_step_id,omitempty"`
	StartedAt    time.Time `json:"startedAt" yaml:"started_at"`
	UpdatedAt    time.Time `json:"updatedAt" yaml:"updated_at"`
	FinishedAt   time.Time `json:"finishedAt,omitempty" yaml:"finished_at,omitempty"`
}

type WorkflowStepInstance struct {
	ID             string    `json:"id" yaml:"id"`
	RunID          string    `json:"runId" yaml:"run_id"`
	StepID         string    `json:"stepId" yaml:"step_id"`
	Status         string    `json:"status" yaml:"status"`
	ActorType      string    `json:"actorType,omitempty" yaml:"actor_type,omitempty"`
	ActorID        string    `json:"actorId,omitempty" yaml:"actor_id,omitempty"`
	ChildTaskID    string    `json:"childTaskId,omitempty" yaml:"child_task_id,omitempty"`
	ReviewItemID   string    `json:"reviewItemId,omitempty" yaml:"review_item_id,omitempty"`
	Summary        string    `json:"summary,omitempty" yaml:"summary,omitempty"`
	StartedAt      time.Time `json:"startedAt,omitempty" yaml:"started_at,omitempty"`
	UpdatedAt      time.Time `json:"updatedAt" yaml:"updated_at"`
	FinishedAt     time.Time `json:"finishedAt,omitempty" yaml:"finished_at,omitempty"`
	InputArtifact  string    `json:"inputArtifact,omitempty" yaml:"input_artifact,omitempty"`
	OutputArtifact string    `json:"outputArtifact,omitempty" yaml:"output_artifact,omitempty"`
}

func NewWorkflowID() string {
	return newShortWorkflowID("wf")
}

func NewWorkflowRunID() string {
	return newShortWorkflowID("wfr")
}

func NewWorkflowStepInstanceID() string {
	return newShortWorkflowID("wfs")
}

func newShortWorkflowID(prefix string) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return fmt.Sprintf("%s-%s", prefix, string(b))
}
