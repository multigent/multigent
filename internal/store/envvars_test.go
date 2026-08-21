package store

import (
	"testing"

	"github.com/multigent/multigent/internal/entity"
)

func TestResolveEnvForAgentTargetsIncludesAgentWorker(t *testing.T) {
	root := t.TempDir()
	es := NewEnvVarStore(root)
	if _, err := es.Add(entity.EnvVar{
		Key:   "GLOBAL_TOKEN",
		Value: "global",
		Scope: entity.EnvVarScopeGlobal,
	}); err != nil {
		t.Fatalf("add global: %v", err)
	}
	if _, err := es.Add(entity.EnvVar{
		Key:    "WORKER_TOKEN",
		Value:  "worker",
		Scope:  entity.EnvVarScopeAgents,
		Agents: []string{"agent_worker:aw-one"},
	}); err != nil {
		t.Fatalf("add worker scoped: %v", err)
	}
	if _, err := es.Add(entity.EnvVar{
		Key:    "LEGACY_TOKEN",
		Value:  "legacy",
		Scope:  entity.EnvVarScopeAgents,
		Agents: []string{"alpha/nova"},
	}); err != nil {
		t.Fatalf("add legacy scoped: %v", err)
	}

	env, err := es.ResolveEnvForAgentTargets("beta", "nova", []string{"agent_worker:aw-one"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if env["GLOBAL_TOKEN"] != "global" {
		t.Fatalf("global env missing: %#v", env)
	}
	if env["WORKER_TOKEN"] != "worker" {
		t.Fatalf("worker env missing: %#v", env)
	}
	if _, ok := env["LEGACY_TOKEN"]; ok {
		t.Fatalf("legacy project env should not leak across projects: %#v", env)
	}
}
