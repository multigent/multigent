package main

import (
	"os"
	"strings"

	"github.com/multigent/multigent/internal/api"
	"github.com/multigent/multigent/internal/appconfig"
)

func effectiveServerAddr(cfg *appconfig.Config, fallback string) string {
	if v := strings.TrimSpace(os.Getenv("MULTIGENT_SERVER_ADDR")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("MULTIGENT_API_ADDR")); v != "" {
		return v
	}
	if cfg != nil && strings.TrimSpace(cfg.Server.Addr) != "" {
		return strings.TrimSpace(cfg.Server.Addr)
	}
	return fallback
}

func effectiveAPIKey(cfg *appconfig.Config) string {
	if key := api.ResolveAPIKey(""); key != "" {
		return key
	}
	if cfg != nil {
		return strings.TrimSpace(cfg.Auth.APIKey)
	}
	return ""
}
