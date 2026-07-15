package db

import (
	"encoding/json"

	"github.com/multigent/multigent/internal/connector"
)

func (db *SQLiteStore) SeedDefaultConnectorProviders() error {
	for _, provider := range connector.Defaults() {
		enabled := provider.Enabled
		if existing, ok, err := db.ConnectorProviderByID(provider.Provider); err != nil {
			return err
		} else if ok {
			enabled = existing.Enabled
		}
		authTypes, err := json.Marshal(provider.AuthTypes)
		if err != nil {
			return err
		}
		catalog, err := json.Marshal(provider)
		if err != nil {
			return err
		}
		if err := db.UpsertConnectorProvider(ConnectorProvider{
			Provider:      provider.Provider,
			DisplayName:   provider.DisplayName,
			AuthTypesJSON: string(authTypes),
			CatalogJSON:   string(catalog),
			Enabled:       enabled,
		}); err != nil {
			return err
		}
	}
	return nil
}
