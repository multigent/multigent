package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/multigent/multigent/internal/entity"
	playbookstore "github.com/multigent/multigent/internal/playbook"
)

type registry struct {
	SchemaVersion int             `json:"schemaVersion"`
	GeneratedBy   string          `json:"generatedBy"`
	Playbooks     []registryEntry `json:"playbooks"`
}

type registryEntry struct {
	ID             string            `json:"id"`
	Version        string            `json:"version"`
	Name           map[string]string `json:"name"`
	Description    map[string]string `json:"description"`
	Category       map[string]string `json:"category"`
	Complexity     map[string]string `json:"complexity"`
	Tags           []string          `json:"tags,omitempty"`
	TemplateURLs   map[string]string `json:"templateUrls"`
	SHA256ByLocale map[string]string `json:"sha256ByLocale"`
}

func main() {
	outDir := flag.String("out", "", "output directory")
	flag.Parse()
	if *outDir == "" {
		fmt.Fprintln(os.Stderr, "missing --out")
		os.Exit(2)
	}
	if err := export(*outDir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func export(outDir string) error {
	locales := []string{"en", "zh-CN"}
	byID := make(map[string]map[string]entity.PlaybookTemplate)
	for _, locale := range locales {
		for _, tmpl := range playbookstore.Templates(locale) {
			if byID[tmpl.ID] == nil {
				byID[tmpl.ID] = make(map[string]entity.PlaybookTemplate)
			}
			byID[tmpl.ID][locale] = tmpl
		}
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	reg := registry{SchemaVersion: 1, GeneratedBy: "multigent tools/export-playbooks"}
	for _, id := range ids {
		entry := registryEntry{
			ID:             id,
			Name:           map[string]string{},
			Description:    map[string]string{},
			Category:       map[string]string{},
			Complexity:     map[string]string{},
			TemplateURLs:   map[string]string{},
			SHA256ByLocale: map[string]string{},
		}
		for _, locale := range locales {
			tmpl, ok := byID[id][locale]
			if !ok {
				continue
			}
			if entry.Version == "" {
				entry.Version = tmpl.Version
			}
			entry.Name[locale] = tmpl.Name
			entry.Description[locale] = tmpl.Description
			entry.Category[locale] = tmpl.Category
			entry.Complexity[locale] = tmpl.Complexity
			entry.Tags = mergeTags(entry.Tags, tmpl.Tags)

			rel := filepath.ToSlash(filepath.Join("playbooks", id, locale+".json"))
			body, err := json.MarshalIndent(tmpl, "", "  ")
			if err != nil {
				return err
			}
			body = append(body, '\n')
			abs := filepath.Join(outDir, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(abs, body, 0o644); err != nil {
				return err
			}
			sum := sha256.Sum256(body)
			entry.TemplateURLs[locale] = rel
			entry.SHA256ByLocale[locale] = fmt.Sprintf("%x", sum[:])
		}
		sort.Strings(entry.Tags)
		reg.Playbooks = append(reg.Playbooks, entry)
	}

	body, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(filepath.Join(outDir, "registry.json"), body, 0o644)
}

func mergeTags(existing, next []string) []string {
	seen := make(map[string]bool, len(existing)+len(next))
	var out []string
	for _, tag := range append(existing, next...) {
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
	}
	return out
}
