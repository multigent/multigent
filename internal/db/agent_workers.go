package db

import (
	"database/sql"
	"errors"
	"strings"
)

func (db *SQLiteStore) UpsertAgentWorker(w AgentWorker) error {
	w.ProfilePrompt = normalizeAgentWorkerPrompt(w.ProfilePrompt)
	if w.CreatedAt == "" {
		w.CreatedAt = nowUTC()
	}
	if w.UpdatedAt == "" {
		w.UpdatedAt = nowUTC()
	}
	if w.Status == "" {
		w.Status = "active"
	}
	if w.ScheduleJSON == "" {
		w.ScheduleJSON = "{}"
	}
	if w.AttentionPolicyJSON == "" {
		w.AttentionPolicyJSON = "{}"
	}
	if w.MemoryPolicyJSON == "" {
		w.MemoryPolicyJSON = "{}"
	}
	if w.SkillsJSON == "" {
		w.SkillsJSON = "[]"
	}
	if w.RuntimeConfigJSON == "" {
		w.RuntimeConfigJSON = "{}"
	}
	_, err := db.sql.Exec(`INSERT INTO agent_workers (
		id, workspace_id, name, display_name, description, profile_prompt, avatar, team, role, status,
		model, runtime_model, default_model_account_id, default_runtime_node_id, default_runtime_mode,
		schedule_json, attention_policy_json, memory_policy_json, skills_json,
		runtime_config_json, primary_session_id, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(workspace_id, name) DO UPDATE SET
		display_name = excluded.display_name,
		description = excluded.description,
		profile_prompt = excluded.profile_prompt,
		avatar = excluded.avatar,
		team = excluded.team,
		role = excluded.role,
		status = excluded.status,
		model = excluded.model,
		runtime_model = excluded.runtime_model,
		default_model_account_id = excluded.default_model_account_id,
		default_runtime_node_id = excluded.default_runtime_node_id,
	default_runtime_mode = excluded.default_runtime_mode,
	schedule_json = excluded.schedule_json,
	attention_policy_json = excluded.attention_policy_json,
	memory_policy_json = excluded.memory_policy_json,
	skills_json = excluded.skills_json,
	runtime_config_json = excluded.runtime_config_json,
	primary_session_id = excluded.primary_session_id,
		updated_at = excluded.updated_at`,
		w.ID, w.WorkspaceID, w.Name, w.DisplayName, w.Description, w.ProfilePrompt, w.Avatar, w.Team, w.Role, w.Status,
		w.Model, w.RuntimeModel, w.DefaultModelAccountID, w.DefaultRuntimeNodeID, w.DefaultRuntimeMode,
		w.ScheduleJSON, w.AttentionPolicyJSON, w.MemoryPolicyJSON, w.SkillsJSON,
		w.RuntimeConfigJSON, w.PrimarySessionID, w.CreatedAt, w.UpdatedAt)
	return err
}

func (db *SQLiteStore) AgentWorkerByID(workspaceID, id string) (AgentWorker, bool, error) {
	row := db.sql.QueryRow(`SELECT id, workspace_id, name, display_name, description, profile_prompt, avatar, status,
	team, role,
	model, runtime_model, default_model_account_id, default_runtime_node_id, default_runtime_mode,
	schedule_json, attention_policy_json, memory_policy_json, skills_json,
	runtime_config_json, primary_session_id, created_at, updated_at
FROM agent_workers WHERE workspace_id = ? AND id = ?`, strings.TrimSpace(workspaceID), strings.TrimSpace(id))
	return scanAgentWorkerFound(row)
}

func (db *SQLiteStore) AgentWorkerByName(workspaceID, name string) (AgentWorker, bool, error) {
	row := db.sql.QueryRow(`SELECT id, workspace_id, name, display_name, description, profile_prompt, avatar, status,
	team, role,
	model, runtime_model, default_model_account_id, default_runtime_node_id, default_runtime_mode,
	schedule_json, attention_policy_json, memory_policy_json, skills_json,
	runtime_config_json, primary_session_id, created_at, updated_at
FROM agent_workers WHERE workspace_id = ? AND name = ?`, strings.TrimSpace(workspaceID), strings.TrimSpace(name))
	return scanAgentWorkerFound(row)
}

func (db *SQLiteStore) ListAgentWorkers(workspaceID string) ([]AgentWorker, error) {
	rows, err := db.sql.Query(`SELECT id, workspace_id, name, display_name, description, profile_prompt, avatar, status,
	team, role,
	model, runtime_model, default_model_account_id, default_runtime_node_id, default_runtime_mode,
	schedule_json, attention_policy_json, memory_policy_json, skills_json,
	runtime_config_json, primary_session_id, created_at, updated_at
FROM agent_workers WHERE workspace_id = ? ORDER BY name ASC`, strings.TrimSpace(workspaceID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AgentWorker, 0)
	for rows.Next() {
		w, err := scanAgentWorker(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (db *SQLiteStore) DeleteAgentWorker(workspaceID, id string) error {
	_, err := db.sql.Exec(`DELETE FROM agent_workers WHERE workspace_id = ? AND id = ?`, strings.TrimSpace(workspaceID), strings.TrimSpace(id))
	return err
}

type agentWorkerScanner interface {
	Scan(dest ...any) error
}

func scanAgentWorkerFound(row agentWorkerScanner) (AgentWorker, bool, error) {
	w, err := scanAgentWorker(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentWorker{}, false, nil
	}
	if err != nil {
		return AgentWorker{}, false, err
	}
	return w, true, nil
}

func scanAgentWorker(row agentWorkerScanner) (AgentWorker, error) {
	var w AgentWorker
	err := row.Scan(&w.ID, &w.WorkspaceID, &w.Name, &w.DisplayName, &w.Description, &w.ProfilePrompt, &w.Avatar, &w.Status,
		&w.Team, &w.Role,
		&w.Model, &w.RuntimeModel, &w.DefaultModelAccountID, &w.DefaultRuntimeNodeID, &w.DefaultRuntimeMode,
		&w.ScheduleJSON, &w.AttentionPolicyJSON, &w.MemoryPolicyJSON, &w.SkillsJSON,
		&w.RuntimeConfigJSON, &w.PrimarySessionID, &w.CreatedAt, &w.UpdatedAt)
	if err == nil {
		w.ProfilePrompt = normalizeAgentWorkerPrompt(w.ProfilePrompt)
	}
	return w, err
}

// normalizeAgentWorkerPrompt repairs prompts written with escaped newlines by
// older clients. Agent profile prompts are Markdown-like instructions, so a
// literal "\\n" is never useful presentation and can hide important rules.
func normalizeAgentWorkerPrompt(prompt string) string {
	return strings.ReplaceAll(prompt, `\n`, "\n")
}
