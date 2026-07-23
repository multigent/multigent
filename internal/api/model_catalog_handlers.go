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
			"codex":      []string{"gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gpt-5.5", "gpt-5.4", "gpt-5.4-mini", "gpt-5.3-codex-spark"},
			"cursor":     []string{"auto", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "claude-fable-5", "claude-opus-4-8", "claude-sonnet-5"},
			"claudecode": []string{"claude-fable-5", "claude-opus-4-8", "claude-sonnet-5", "claude-haiku-4-5", "claude-haiku-4-5-20251001"},
			"gemini":     []string{"gemini-3.6-flash", "gemini-3.5-flash", "gemini-3.5-flash-lite", "gemini-3.1-pro-preview", "gemini-3.1-pro-preview-customtools", "gemini-3.1-flash-lite", "gemini-2.5-pro", "gemini-2.5-flash", "gemini-2.5-flash-lite"},
			"opencode":   []string{"gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gpt-5.5", "claude-fable-5", "claude-opus-4-8", "claude-sonnet-5"},
		},
		ModelsByProviderType: map[string][]string{
			"openai":    []string{"gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gpt-5.5", "gpt-5.4", "gpt-5.4-mini", "gpt-5.3-codex-spark"},
			"cursor":    []string{"auto", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "claude-fable-5", "claude-opus-4-8", "claude-sonnet-5"},
			"anthropic": []string{"claude-fable-5", "claude-opus-4-8", "claude-sonnet-5", "claude-haiku-4-5", "claude-haiku-4-5-20251001"},
			"gemini":    []string{"gemini-3.6-flash", "gemini-3.5-flash", "gemini-3.5-flash-lite", "gemini-3.1-pro-preview", "gemini-3.1-pro-preview-customtools", "gemini-3.1-flash-lite", "gemini-2.5-pro", "gemini-2.5-flash", "gemini-2.5-flash-lite"},
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
