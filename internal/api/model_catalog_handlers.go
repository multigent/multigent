package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type modelCatalogResponse struct {
	Source               string              `json:"source"`
	ModelsByCLI          map[string][]string `json:"modelsByCLI"`
	ModelsByProviderType map[string][]string `json:"modelsByProviderType"`
}

func (s *Server) handleModelCatalog(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAccess(w, r) {
		return
	}
	catalog := fallbackModelCatalog()
	url := strings.TrimSpace(os.Getenv("MULTIGENT_MODEL_CATALOG_URL"))
	if url != "" {
		if remote, ok := fetchRemoteModelCatalog(r.Context(), url); ok {
			catalog = mergeModelCatalogs(remote, catalog)
			catalog.Source = "remote+builtin"
		}
	}
	_ = json.NewEncoder(w).Encode(catalog)
}

func fallbackModelCatalog() modelCatalogResponse {
	return modelCatalogResponse{
		Source: "builtin",
		ModelsByCLI: map[string][]string{
			"codex":      []string{"gpt-5.6-sol", "gpt-5.1-codex-max", "gpt-5.1-codex", "gpt-5.1", "gpt-5", "gpt-4.1"},
			"cursor":     []string{"auto", "gpt-5.1", "gpt-5", "claude-sonnet-4-20250514", "claude-opus-4-20250514"},
			"claudecode": []string{"claude-sonnet-4-20250514", "claude-opus-4-20250514", "claude-3-7-sonnet-latest"},
			"gemini":     []string{"gemini-2.5-pro", "gemini-2.5-flash", "gemini-2.0-flash"},
			"opencode":   []string{"gpt-5.1", "claude-sonnet-4-20250514"},
		},
		ModelsByProviderType: map[string][]string{
			"openai":    []string{"gpt-5.6-sol", "gpt-5.1-codex-max", "gpt-5.1-codex", "gpt-5.1", "gpt-5", "gpt-4.1"},
			"cursor":    []string{"auto", "gpt-5.1", "gpt-5", "claude-sonnet-4-20250514", "claude-opus-4-20250514"},
			"anthropic": []string{"claude-sonnet-4-20250514", "claude-opus-4-20250514", "claude-3-7-sonnet-latest"},
			"gemini":    []string{"gemini-2.5-pro", "gemini-2.5-flash", "gemini-2.0-flash"},
		},
	}
}

func fetchRemoteModelCatalog(ctx context.Context, url string) (modelCatalogResponse, bool) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return modelCatalogResponse{}, false
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return modelCatalogResponse{}, false
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return modelCatalogResponse{}, false
	}
	var catalog modelCatalogResponse
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&catalog); err != nil {
		return modelCatalogResponse{}, false
	}
	catalog.ModelsByCLI = cleanModelCatalogMap(catalog.ModelsByCLI)
	catalog.ModelsByProviderType = cleanModelCatalogMap(catalog.ModelsByProviderType)
	if len(catalog.ModelsByCLI) == 0 && len(catalog.ModelsByProviderType) == 0 {
		return modelCatalogResponse{}, false
	}
	if catalog.Source == "" {
		catalog.Source = "remote"
	}
	return catalog, true
}

func cleanModelCatalogMap(in map[string][]string) map[string][]string {
	out := map[string][]string{}
	for key, models := range in {
		key = strings.TrimSpace(strings.ToLower(key))
		if key == "" {
			continue
		}
		cleaned := cleanProviderModels(models)
		if len(cleaned) > 0 {
			out[key] = cleaned
		}
	}
	return out
}

func mergeModelCatalogs(primary, fallback modelCatalogResponse) modelCatalogResponse {
	return modelCatalogResponse{
		Source:               primary.Source,
		ModelsByCLI:          mergeModelCatalogMap(primary.ModelsByCLI, fallback.ModelsByCLI),
		ModelsByProviderType: mergeModelCatalogMap(primary.ModelsByProviderType, fallback.ModelsByProviderType),
	}
}

func mergeModelCatalogMap(primary, fallback map[string][]string) map[string][]string {
	out := map[string][]string{}
	for key, models := range fallback {
		out[key] = cleanProviderModels(models)
	}
	for key, models := range primary {
		key = strings.TrimSpace(strings.ToLower(key))
		if key == "" {
			continue
		}
		out[key] = cleanProviderModels(append(models, out[key]...))
	}
	return out
}
