package entity

import "testing"

func TestDecodeAgentWorkerRuntimeConfigCanonicalKeys(t *testing.T) {
	cfg := DecodeAgentWorkerRuntimeConfig(`{"env":{"K":"V"},"addDirs":["/repo"],"runCommand":" run {prompt_file} ","sandbox":{"provider":"none"}}`)
	if cfg.RunCommand != "run {prompt_file}" {
		t.Fatalf("RunCommand = %q", cfg.RunCommand)
	}
	if len(cfg.AddDirs) != 1 || cfg.AddDirs[0] != "/repo" || cfg.Env["K"] != "V" || cfg.Sandbox == nil {
		t.Fatalf("decoded config = %#v", cfg)
	}
}

func TestDecodeAgentWorkerRuntimeConfigLegacyKeys(t *testing.T) {
	cfg := DecodeAgentWorkerRuntimeConfig(`{"add_dirs":["/legacy"],"run_command":"python3 adapter.py --prompt-file {prompt_file}"}`)
	if cfg.RunCommand != "python3 adapter.py --prompt-file {prompt_file}" {
		t.Fatalf("RunCommand = %q", cfg.RunCommand)
	}
	if len(cfg.AddDirs) != 1 || cfg.AddDirs[0] != "/legacy" {
		t.Fatalf("AddDirs = %#v", cfg.AddDirs)
	}
	if got := DecodeAgentWorkerRuntimeConfig(`{"command":"legacy-cmd"}`).RunCommand; got != "legacy-cmd" {
		t.Fatalf("command fallback = %q", got)
	}
}

func TestDecodeAgentWorkerRuntimeConfigPrefersCanonicalKeys(t *testing.T) {
	cfg := DecodeAgentWorkerRuntimeConfig(`{"runCommand":"new","run_command":"old","command":"older","addDirs":["/new"],"add_dirs":["/old"]}`)
	if cfg.RunCommand != "new" || len(cfg.AddDirs) != 1 || cfg.AddDirs[0] != "/new" {
		t.Fatalf("decoded config = %#v", cfg)
	}
}

func TestAgentWorkerRuntimeConfigEncodeDropsLegacyKeys(t *testing.T) {
	raw := DecodeAgentWorkerRuntimeConfig(`{"run_command":"cmd","add_dirs":["/d"]}`).Encode()
	if raw != `{"addDirs":["/d"],"runCommand":"cmd"}` {
		t.Fatalf("Encode() = %s", raw)
	}
}

func TestAgentWorkerRuntimeConfigApplyToLeavesUnsetFields(t *testing.T) {
	meta := &AgentMeta{RunCommand: "keep", AddDirs: []string{"/keep"}}
	AgentWorkerRuntimeConfig{Env: map[string]string{"A": "B"}}.ApplyTo(meta)
	if meta.RunCommand != "keep" || len(meta.AddDirs) != 1 || meta.Env["A"] != "B" {
		t.Fatalf("meta = %#v", meta)
	}
	AgentWorkerRuntimeConfig{}.ApplyTo(nil)
}
