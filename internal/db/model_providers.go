package db

import (
	"database/sql"
	"errors"
)

func (db *SQLiteStore) UpsertModelProvider(workspaceID string, provider ModelProvider) error {
	if provider.CreatedAt == "" {
		provider.CreatedAt = nowUTC()
	}
	if provider.UpdatedAt == "" {
		provider.UpdatedAt = nowUTC()
	}
	if provider.EnvJSON == "" {
		provider.EnvJSON = "{}"
	}
	_, err := db.sql.Exec(`INSERT INTO model_providers (
	id, workspace_id, name, type, base_url, api_key, model, env_json, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(workspace_id, id) DO UPDATE SET
	name = excluded.name,
	type = excluded.type,
	base_url = excluded.base_url,
	api_key = excluded.api_key,
	model = excluded.model,
	env_json = excluded.env_json,
	updated_at = excluded.updated_at`,
		provider.ID, workspaceID, provider.Name, provider.Type, provider.BaseURL, provider.APIKey,
		provider.Model, provider.EnvJSON, provider.CreatedAt, provider.UpdatedAt)
	return err
}

func (db *SQLiteStore) ModelProviderByID(workspaceID, id string) (ModelProvider, bool, error) {
	row := db.sql.QueryRow(`SELECT id, workspace_id, name, type, base_url, api_key, model, env_json, created_at, updated_at
FROM model_providers WHERE workspace_id = ? AND id = ?`, workspaceID, id)
	p, err := scanModelProvider(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ModelProvider{}, false, nil
	}
	if err != nil {
		return ModelProvider{}, false, err
	}
	return p, true, nil
}

func (db *SQLiteStore) ListModelProviders(workspaceID string) ([]ModelProvider, error) {
	rows, err := db.sql.Query(`SELECT id, workspace_id, name, type, base_url, api_key, model, env_json, created_at, updated_at
FROM model_providers WHERE workspace_id = ? ORDER BY name ASC, id ASC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ModelProvider, 0)
	for rows.Next() {
		p, err := scanModelProvider(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (db *SQLiteStore) DeleteModelProvider(workspaceID, id string) error {
	_, err := db.sql.Exec(`DELETE FROM model_providers WHERE workspace_id = ? AND id = ?`, workspaceID, id)
	return err
}

type modelProviderScanner interface {
	Scan(dest ...any) error
}

func scanModelProvider(row modelProviderScanner) (ModelProvider, error) {
	var p ModelProvider
	err := row.Scan(&p.ID, &p.WorkspaceID, &p.Name, &p.Type, &p.BaseURL, &p.APIKey, &p.Model, &p.EnvJSON, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}
