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

const (
	GitHubRegistryURL = "https://raw.githubusercontent.com/multigent/playbooks/main/registry.json"
	GiteeRegistryURL  = "https://gitee.com/multigent/playbooks/raw/main/registry.json"
)

var DefaultRegistryURLs = []string{
	GitHubRegistryURL,
	GiteeRegistryURL,
}

const DefaultRegistryURL = GitHubRegistryURL

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
	if isDefaultRegistryMirrorSet(urls) {
		return firstSuccessfulRemoteTemplates(ctx, locale, urls)
	}
	var out []entity.PlaybookTemplate
	var lastErr error
	for _, url := range urls {
		url = strings.TrimSpace(url)
		if url == "" {
			continue
		}
		templates, err := remoteTemplatesFromURL(ctx, locale, url)
		if err != nil {
			lastErr = err
			continue
		}
		out = append(out, templates...)
	}
	if len(out) == 0 && lastErr != nil {
		return out, lastErr
	}
	return out, nil
}

func firstSuccessfulRemoteTemplates(ctx context.Context, locale string, urls []string) ([]entity.PlaybookTemplate, error) {
	type result struct {
		templates []entity.PlaybookTemplate
		err       error
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch := make(chan result, len(urls))
	started := 0
	for _, registryURL := range urls {
		registryURL = strings.TrimSpace(registryURL)
		if registryURL == "" {
			continue
		}
		started++
		go func(url string) {
			templates, err := remoteTemplatesFromURL(ctx, locale, url)
			ch <- result{templates: templates, err: err}
		}(registryURL)
	}
	var lastErr error
	for i := 0; i < started; i++ {
		res := <-ch
		if res.err == nil {
			cancel()
			return res.templates, nil
		}
		lastErr = res.err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, nil
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
	if isDefaultRegistryMirrorSet(urls) {
		return firstSuccessfulTemplateByID(ctx, id, locale, urls)
	}
	for _, registryURL := range urls {
		tmpl, ok, err := remoteTemplateByIDFromURL(ctx, id, locale, registryURL)
		if err == nil && ok {
			return tmpl, true
		}
	}
	return entity.PlaybookTemplate{}, false
}

func firstSuccessfulTemplateByID(ctx context.Context, id, locale string, urls []string) (entity.PlaybookTemplate, bool) {
	type result struct {
		template entity.PlaybookTemplate
		ok       bool
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch := make(chan result, len(urls))
	started := 0
	for _, registryURL := range urls {
		registryURL = strings.TrimSpace(registryURL)
		if registryURL == "" {
			continue
		}
		started++
		go func(url string) {
			tmpl, ok, err := remoteTemplateByIDFromURL(ctx, id, locale, url)
			if err != nil {
				ch <- result{}
				return
			}
			ch <- result{template: tmpl, ok: ok}
		}(registryURL)
	}
	for i := 0; i < started; i++ {
		res := <-ch
		if res.ok {
			cancel()
			return res.template, true
		}
	}
	return entity.PlaybookTemplate{}, false
}

func remoteTemplateByIDFromURL(ctx context.Context, id, locale, registryURL string) (entity.PlaybookTemplate, bool, error) {
	registryURL = strings.TrimSpace(registryURL)
	body, err := readRemoteFile(ctx, registryURL, "")
	if err != nil {
		return entity.PlaybookTemplate{}, false, err
	}
	var registry RemoteRegistry
	if err := json.Unmarshal(body, &registry); err != nil {
		return entity.PlaybookTemplate{}, false, err
	}
	for _, tmpl := range registry.Templates {
		if tmpl.ID == id {
			return normalizeRemoteTemplate(tmpl, locale), true, nil
		}
	}
	for _, entry := range registry.Playbooks {
		if entry.ID != id {
			continue
		}
		tmpl, ok, err := templateFromRegistryEntry(ctx, locale, registryURL, entry, true)
		if err != nil || !ok {
			return entity.PlaybookTemplate{}, false, err
		}
		return tmpl, true, nil
	}
	return entity.PlaybookTemplate{}, false, nil
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

func isDefaultRegistryMirrorSet(urls []string) bool {
	seen := map[string]bool{}
	for _, registryURL := range urls {
		registryURL = strings.TrimSpace(registryURL)
		if registryURL != "" {
			seen[registryURL] = true
		}
	}
	if len(seen) != len(DefaultRegistryURLs) {
		return false
	}
	for _, registryURL := range DefaultRegistryURLs {
		if !seen[registryURL] {
			return false
		}
	}
	return true
}
