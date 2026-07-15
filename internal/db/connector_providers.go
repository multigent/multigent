package db

import (
	"database/sql"
	"errors"
)

func (db *SQLiteStore) UpsertConnectorProvider(provider ConnectorProvider) error {
	if provider.CreatedAt == "" {
		provider.CreatedAt = nowUTC()
	}
	if provider.UpdatedAt == "" {
		provider.UpdatedAt = nowUTC()
	}
	if provider.AuthTypesJSON == "" {
		provider.AuthTypesJSON = "[]"
	}
	if provider.CatalogJSON == "" {
		provider.CatalogJSON = "{}"
	}
	enabled := 0
	if provider.Enabled {
		enabled = 1
	}
	_, err := db.sql.Exec(`INSERT INTO connector_providers (
	provider, display_name, auth_types_json, catalog_json, enabled, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(provider) DO UPDATE SET
	display_name = excluded.display_name,
	auth_types_json = excluded.auth_types_json,
	catalog_json = excluded.catalog_json,
	enabled = excluded.enabled,
	updated_at = excluded.updated_at`,
		provider.Provider, provider.DisplayName, provider.AuthTypesJSON, provider.CatalogJSON,
		enabled, provider.CreatedAt, provider.UpdatedAt)
	return err
}

func (db *SQLiteStore) ConnectorProviderByID(provider string) (ConnectorProvider, bool, error) {
	row := db.sql.QueryRow(`SELECT provider, display_name, auth_types_json, catalog_json, enabled, created_at, updated_at
FROM connector_providers WHERE provider = ?`, provider)
	p, err := scanConnectorProvider(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ConnectorProvider{}, false, nil
	}
	if err != nil {
		return ConnectorProvider{}, false, err
	}
	return p, true, nil
}

func (db *SQLiteStore) ListConnectorProviders(includeDisabled bool) ([]ConnectorProvider, error) {
	query := `SELECT provider, display_name, auth_types_json, catalog_json, enabled, created_at, updated_at
FROM connector_providers`
	if !includeDisabled {
		query += ` WHERE enabled = 1`
	}
	query += ` ORDER BY display_name ASC, provider ASC`
	rows, err := db.sql.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ConnectorProvider, 0)
	for rows.Next() {
		p, err := scanConnectorProvider(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

type connectorProviderScanner interface {
	Scan(dest ...any) error
}

func scanConnectorProvider(row connectorProviderScanner) (ConnectorProvider, error) {
	var p ConnectorProvider
	var enabled int
	err := row.Scan(&p.Provider, &p.DisplayName, &p.AuthTypesJSON, &p.CatalogJSON, &enabled, &p.CreatedAt, &p.UpdatedAt)
	p.Enabled = enabled != 0
	return p, err
}
