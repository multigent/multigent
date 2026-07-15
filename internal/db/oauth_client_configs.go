package db

import (
	"database/sql"
	"errors"
)

func (db *SQLiteStore) UpsertOAuthClientConfig(config OAuthClientConfig) error {
	if config.CreatedAt == "" {
		config.CreatedAt = nowUTC()
	}
	if config.UpdatedAt == "" {
		config.UpdatedAt = nowUTC()
	}
	if config.ExtraJSON == "" {
		config.ExtraJSON = "{}"
	}
	_, err := db.sql.Exec(`INSERT INTO oauth_client_configs (
	workspace_id, provider, client_id, secret_ciphertext, nonce, key_version, extra_json, created_by, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(workspace_id, provider) DO UPDATE SET
	client_id = excluded.client_id,
	secret_ciphertext = excluded.secret_ciphertext,
	nonce = excluded.nonce,
	key_version = excluded.key_version,
	extra_json = excluded.extra_json,
	updated_at = excluded.updated_at`,
		config.WorkspaceID, config.Provider, config.ClientID, config.SecretCiphertext, config.Nonce,
		config.KeyVersion, config.ExtraJSON, config.CreatedBy, config.CreatedAt, config.UpdatedAt)
	return err
}

func (db *SQLiteStore) OAuthClientConfigByProvider(workspaceID, provider string) (OAuthClientConfig, bool, error) {
	row := db.sql.QueryRow(`SELECT workspace_id, provider, client_id, secret_ciphertext, nonce, key_version, extra_json, created_by, created_at, updated_at
FROM oauth_client_configs WHERE workspace_id = ? AND provider = ?`, workspaceID, provider)
	config, err := scanOAuthClientConfig(row)
	if errors.Is(err, sql.ErrNoRows) {
		return OAuthClientConfig{}, false, nil
	}
	if err != nil {
		return OAuthClientConfig{}, false, err
	}
	return config, true, nil
}

func (db *SQLiteStore) ListOAuthClientConfigs(workspaceID string) ([]OAuthClientConfig, error) {
	rows, err := db.sql.Query(`SELECT workspace_id, provider, client_id, secret_ciphertext, nonce, key_version, extra_json, created_by, created_at, updated_at
FROM oauth_client_configs WHERE workspace_id = ? ORDER BY provider ASC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]OAuthClientConfig, 0)
	for rows.Next() {
		config, err := scanOAuthClientConfig(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, config)
	}
	return out, rows.Err()
}

func (db *SQLiteStore) DeleteOAuthClientConfig(workspaceID, provider string) error {
	_, err := db.sql.Exec(`DELETE FROM oauth_client_configs WHERE workspace_id = ? AND provider = ?`, workspaceID, provider)
	return err
}

type oauthClientConfigScanner interface {
	Scan(dest ...any) error
}

func scanOAuthClientConfig(row oauthClientConfigScanner) (OAuthClientConfig, error) {
	var config OAuthClientConfig
	err := row.Scan(&config.WorkspaceID, &config.Provider, &config.ClientID, &config.SecretCiphertext,
		&config.Nonce, &config.KeyVersion, &config.ExtraJSON, &config.CreatedBy, &config.CreatedAt, &config.UpdatedAt)
	return config, err
}
