package playbook

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/multigent/multigent/internal/entity"
)

const EnvRegistryURLs = "MULTIGENT_PLAYBOOK_REGISTRY_URLS"
const DefaultRegistryURL = "https://raw.githubusercontent.com/multigent/playbooks/main/registry.json"

type RemoteRegistry struct {
	SchemaVersion int                       `json:"schemaVersion"`
	Templates     []entity.PlaybookTemplate `json:"templates"`
	Playbooks     []RemoteRegistryPlaybook  `json:"playbooks"`
}

type RemoteRegistryPlaybook struct {
	ID             string                   `json:"id"`
	Version        string                   `json:"version"`
	Name           map[string]string        `json:"name"`
	Description    map[string]string        `json:"description"`
	Category       map[string]string        `json:"category"`
	Complexity     map[string]string        `json:"complexity"`
	Tags           []string                 `json:"tags"`
	Template       *entity.PlaybookTemplate `json:"template,omitempty"`
	TemplateURL    string                   `json:"templateUrl,omitempty"`
	TemplateURLs   map[string]string        `json:"templateUrls,omitempty"`
	SHA256         string                   `json:"sha256,omitempty"`
	SHA256ByLocale map[string]string        `json:"sha256ByLocale,omitempty"`
}

func RegistryURLsFromEnv() []string {
	raw := strings.TrimSpace(os.Getenv(EnvRegistryURLs))
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == ';'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func TemplatesWithRemote(ctx context.Context, locale string, urls []string) []entity.PlaybookTemplate {
	templates := Templates(locale)
	remote, err := RemoteTemplates(ctx, locale, urls)
	if err != nil {
		return templates
	}
	return mergeTemplates(templates, remote)
}

func TemplateWithRemote(ctx context.Context, id, locale string, urls []string) (entity.PlaybookTemplate, bool) {
	if tmpl, ok := remoteTemplateByID(ctx, id, locale, urls); ok {
		return tmpl, true
	}
	for _, tmpl := range Templates(locale) {
		if tmpl.ID == id {
			return tmpl, true
		}
	}
	return entity.PlaybookTemplate{}, false
}

func RemoteTemplates(ctx context.Context, locale string, urls []string) ([]entity.PlaybookTemplate, error) {
	var out []entity.PlaybookTemplate
	for _, url := range urls {
		url = strings.TrimSpace(url)
		if url == "" {
			continue
		}
		templates, err := remoteTemplatesFromURL(ctx, locale, url)
		if err != nil {
			return out, err
		}
		out = append(out, templates...)
	}
	return out, nil
}

func remoteTemplatesFromURL(ctx context.Context, locale, url string) ([]entity.PlaybookTemplate, error) {
	body, err := readRemoteFile(ctx, url, "")
	if err != nil {
		return nil, err
	}
	var registry RemoteRegistry
	if err := json.Unmarshal(body, &registry); err != nil {
		return nil, fmt.Errorf("decode playbook registry %s: %w", url, err)
	}
	out := make([]entity.PlaybookTemplate, 0, len(registry.Templates)+len(registry.Playbooks))
	for _, tmpl := range registry.Templates {
		out = append(out, normalizeRemoteTemplate(tmpl, locale))
	}
	for _, entry := range registry.Playbooks {
		tmpl, ok, err := templateFromRegistryEntry(ctx, locale, url, entry, false)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		out = append(out, tmpl)
	}
	return out, nil
}

func remoteTemplateByID(ctx context.Context, id, locale string, urls []string) (entity.PlaybookTemplate, bool) {
	for _, registryURL := range urls {
		body, err := readRemoteFile(ctx, strings.TrimSpace(registryURL), "")
		if err != nil {
			continue
		}
		var registry RemoteRegistry
		if err := json.Unmarshal(body, &registry); err != nil {
			continue
		}
		for _, tmpl := range registry.Templates {
			if tmpl.ID == id {
				return normalizeRemoteTemplate(tmpl, locale), true
			}
		}
		for _, entry := range registry.Playbooks {
			if entry.ID != id {
				continue
			}
			tmpl, ok, err := templateFromRegistryEntry(ctx, locale, registryURL, entry, true)
			if err != nil || !ok {
				continue
			}
			return tmpl, true
		}
	}
	return entity.PlaybookTemplate{}, false
}

func templateFromRegistryEntry(ctx context.Context, locale, registryURL string, entry RemoteRegistryPlaybook, full bool) (entity.PlaybookTemplate, bool, error) {
	var tmpl entity.PlaybookTemplate
	if entry.Template != nil {
		tmpl = *entry.Template
	} else if full {
		templateURL := localizedURL(entry.TemplateURLs, entry.TemplateURL, locale)
		if templateURL == "" {
			return entity.PlaybookTemplate{}, false, nil
		}
		body, err := readRemoteFile(ctx, resolveRemoteURL(registryURL, templateURL), localizedChecksum(entry, locale))
		if err != nil {
			return entity.PlaybookTemplate{}, false, err
		}
		if err := json.Unmarshal(body, &tmpl); err != nil {
			return entity.PlaybookTemplate{}, false, err
		}
	} else if !full {
		tmpl = entity.PlaybookTemplate{}
	} else {
		return entity.PlaybookTemplate{}, false, nil
	}
	if tmpl.ID == "" {
		tmpl.ID = entry.ID
	}
	if strings.TrimSpace(tmpl.ID) == "" {
		return entity.PlaybookTemplate{}, false, nil
	}
	if tmpl.Version == "" {
		tmpl.Version = entry.Version
	}
	if tmpl.Name == "" {
		tmpl.Name = localized(entry.Name, locale)
	}
	if tmpl.Description == "" {
		tmpl.Description = localized(entry.Description, locale)
	}
	if tmpl.Category == "" {
		tmpl.Category = localized(entry.Category, locale)
	}
	if tmpl.Complexity == "" {
		tmpl.Complexity = localized(entry.Complexity, locale)
	}
	if len(tmpl.Tags) == 0 {
		tmpl.Tags = entry.Tags
	}
	return normalizeRemoteTemplate(tmpl, locale), true, nil
}

func localizedURL(values map[string]string, fallback, locale string) string {
	if value := localized(values, locale); value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}

func localizedChecksum(entry RemoteRegistryPlaybook, locale string) string {
	if value := localized(entry.SHA256ByLocale, locale); value != "" {
		return value
	}
	return strings.TrimSpace(entry.SHA256)
}

func readRemoteFile(ctx context.Context, url string, wantSHA256 string) ([]byte, error) {
	body, err := readRemoteFileUnchecked(ctx, url)
	if err != nil {
		return nil, err
	}
	if wantSHA256 != "" {
		sum := sha256.Sum256(body)
		got := fmt.Sprintf("%x", sum[:])
		if !strings.EqualFold(got, strings.TrimSpace(wantSHA256)) {
			return nil, fmt.Errorf("fetch playbook file %s: sha256 mismatch", url)
		}
	}
	return body, nil
}

func readRemoteFileUnchecked(ctx context.Context, url string) ([]byte, error) {
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("fetch playbook registry %s: status %d", url, resp.StatusCode)
		}
		return io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	}
	path := strings.TrimPrefix(url, "file://")
	return os.ReadFile(path)
}

func resolveRemoteURL(base, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") || strings.HasPrefix(ref, "file://") || filepath.IsAbs(ref) {
		return ref
	}
	if strings.HasPrefix(base, "http://") || strings.HasPrefix(base, "https://") {
		u, err := url.Parse(base)
		if err != nil {
			return ref
		}
		r, err := url.Parse(ref)
		if err != nil {
			return ref
		}
		return u.ResolveReference(r).String()
	}
	path := strings.TrimPrefix(base, "file://")
	dir := filepath.Dir(path)
	if strings.HasPrefix(base, "file://") {
		return "file://" + filepath.Join(dir, ref)
	}
	return filepath.Join(dir, ref)
}

func normalizeRemoteTemplate(tmpl entity.PlaybookTemplate, locale string) entity.PlaybookTemplate {
	if tmpl.Locale == "" {
		tmpl.Locale = normalizeLocale(locale)
	}
	return tmpl
}

func localized(values map[string]string, locale string) string {
	if len(values) == 0 {
		return ""
	}
	locale = normalizeLocale(locale)
	if values[locale] != "" {
		return values[locale]
	}
	if locale == "zh-CN" && values["zh"] != "" {
		return values["zh"]
	}
	if values["en"] != "" {
		return values["en"]
	}
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func mergeTemplates(base, remote []entity.PlaybookTemplate) []entity.PlaybookTemplate {
	out := append([]entity.PlaybookTemplate{}, base...)
	index := make(map[string]int, len(out))
	for i, tmpl := range out {
		index[tmpl.ID] = i
	}
	for _, tmpl := range remote {
		if strings.TrimSpace(tmpl.ID) == "" {
			continue
		}
		if i, ok := index[tmpl.ID]; ok {
			out[i] = tmpl
			continue
		}
		index[tmpl.ID] = len(out)
		out = append(out, tmpl)
	}
	return out
}
