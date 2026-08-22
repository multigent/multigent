package runtimeexec

import "github.com/multigent/multigent/internal/entity"

const KindExecPrompt = "exec_prompt"
const KindTask = "task"
const KindForkSession = "fork_session"

type Spec struct {
	Kind              string            `json:"kind"`
	WorkspaceID       string            `json:"workspaceId"`
	ProjectID         string            `json:"projectId"`
	AgentID           string            `json:"agentId"`
	TaskID            string            `json:"taskId,omitempty"`
	ForkSessionID     string            `json:"forkSessionId,omitempty"`
	SessionID         string            `json:"sessionId,omitempty"`
	Prompt            string            `json:"prompt"`
	Agent             entity.AgentMeta  `json:"agent"`
	ProviderEnv       map[string]string `json:"providerEnv,omitempty"`
	RuntimeControlEnv map[string]string `json:"runtimeControlEnv"`
}
