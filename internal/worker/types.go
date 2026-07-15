package worker

import (
	"context"
	"time"
)

// Job is the cloud-control-plane view of work that a local worker can lease.
// It is intentionally smaller than the current task entity so the worker
// protocol can evolve without inheriting every local workspace detail.
type Job struct {
	ID          string            `json:"id"`
	TenantID    string            `json:"tenant_id,omitempty"`
	ProjectID   string            `json:"project_id,omitempty"`
	TaskID      string            `json:"task_id,omitempty"`
	AgentName   string            `json:"agent_name,omitempty"`
	Runtime     string            `json:"runtime,omitempty"`
	Prompt      string            `json:"prompt,omitempty"`
	ContextRefs []string          `json:"context_refs,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type JobResult struct {
	JobID       string            `json:"job_id"`
	Status      string            `json:"status"`
	Summary     string            `json:"summary,omitempty"`
	Error       string            `json:"error,omitempty"`
	Artifacts   []string          `json:"artifacts,omitempty"`
	TokenUsage  map[string]int64  `json:"token_usage,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CompletedAt time.Time         `json:"completed_at"`
}

type Heartbeat struct {
	WorkerID    string            `json:"worker_id"`
	Status      string            `json:"status"`
	Workspace   string            `json:"workspace,omitempty"`
	Capacity    int               `json:"capacity"`
	RunningJobs []string          `json:"running_jobs,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	SentAt      time.Time         `json:"sent_at"`
}

type ControlPlane interface {
	Heartbeat(ctx context.Context, hb Heartbeat) error
	LeaseJob(ctx context.Context, workerID string) (*Job, error)
	CompleteJob(ctx context.Context, result JobResult) error
}

type LocalExecutor interface {
	Execute(ctx context.Context, job Job) (*JobResult, error)
}

type ImportManifest struct {
	Path      string              `json:"path"`
	Files     []ImportContextFile `json:"files"`
	Warnings  []string            `json:"warnings,omitempty"`
	ScannedAt time.Time           `json:"scanned_at"`
}

type ImportContextFile struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
	Size int64  `json:"size"`
}
