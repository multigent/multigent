package playbook

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/multigent/multigent/internal/entity"
)

func TestRegistryURLsFromEnv(t *testing.T) {
	t.Setenv(EnvRegistryURLs, "https://example.com/a.json, file:///tmp/b.json\nhttps://example.com/c.json;")

	got := RegistryURLsFromEnv()
	want := []string{"https://example.com/a.json", "file:///tmp/b.json", "https://example.com/c.json"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("urls=%#v want %#v", got, want)
	}
}

func TestRemoteTemplatesFromFileRegistry(t *testing.T) {
	registryPath := writeRegistry(t, RemoteRegistry{
		SchemaVersion: 1,
		Templates: []entity.PlaybookTemplate{
			{
				ID:          "remote-template",
				Version:     "0.1.0",
				Name:        "Remote Template",
				Description: "Fetched from a registry",
				Roles: []entity.PlaybookRoleTemplate{
					{ID: "role", Team: "team", Role: "role", Name: "Role", Prompt: "Prompt"},
				},
			},
		},
	})

	templates, err := RemoteTemplates(context.Background(), "zh-CN", []string{"file://" + registryPath})
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) != 1 {
		t.Fatalf("templates=%d", len(templates))
	}
	if templates[0].ID != "remote-template" || templates[0].Locale != "zh-CN" || len(templates[0].Roles) != 1 {
		t.Fatalf("unexpected template=%#v", templates[0])
	}
}

func TestRemoteTemplatesSkipsFailedRegistry(t *testing.T) {
	registryPath := writeRegistry(t, RemoteRegistry{
		SchemaVersion: 1,
		Templates: []entity.PlaybookTemplate{
			{
				ID:          "fallback-template",
				Version:     "0.1.0",
				Name:        "Fallback Template",
				Description: "Loaded after the first registry failed",
			},
		},
	})

	templates, err := RemoteTemplates(context.Background(), "en", []string{
		filepath.Join(t.TempDir(), "missing.json"),
		"file://" + registryPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) != 1 || templates[0].ID != "fallback-template" {
		t.Fatalf("unexpected fallback templates=%#v", templates)
	}
}

func TestRemotePlaybookWrapperLocalizesMetadata(t *testing.T) {
	registryPath := writeRegistry(t, RemoteRegistry{
		SchemaVersion: 1,
		Playbooks: []RemoteRegistryPlaybook{
			{
				ID:          "wrapped-playbook",
				Version:     "1.2.3",
				Name:        map[string]string{"en": "Wrapped Playbook", "zh-CN": "封装方案"},
				Description: map[string]string{"en": "English description", "zh-CN": "中文描述"},
				Category:    map[string]string{"zh-CN": "演示"},
				Complexity:  map[string]string{"zh-CN": "入门"},
				Tags:        []string{"demo"},
				Template: &entity.PlaybookTemplate{
					Skills: []entity.PlaybookSkillTemplate{
						{ID: "skill", Name: "Skill", Body: "Body"},
					},
				},
			},
		},
	})

	tmpl, ok := TemplateWithRemote(context.Background(), "wrapped-playbook", "zh-CN", []string{registryPath})
	if !ok {
		t.Fatal("wrapped playbook not found")
	}
	if tmpl.Name != "封装方案" || tmpl.Description != "中文描述" || tmpl.Category != "演示" || tmpl.Complexity != "入门" {
		t.Fatalf("metadata not localized: %#v", tmpl)
	}
	if tmpl.Version != "1.2.3" || tmpl.Locale != "zh-CN" || len(tmpl.Skills) != 1 {
		t.Fatalf("unexpected wrapped template=%#v", tmpl)
	}
}

func TestRemotePlaybookWrapperLoadsTemplateURL(t *testing.T) {
	dir := t.TempDir()
	template := entity.PlaybookTemplate{
		ID:          "template-url-playbook",
		Version:     "2.0.0",
		Name:        "Template URL Playbook",
		Description: "Loaded lazily from a template URL",
		Skills: []entity.PlaybookSkillTemplate{
			{ID: "lazy-skill", Name: "Lazy Skill", Body: "Full body from template file"},
		},
	}
	templateBody, err := json.Marshal(template)
	if err != nil {
		t.Fatal(err)
	}
	templatePath := filepath.Join(dir, "playbooks", "template-url-playbook.json")
	if err := os.MkdirAll(filepath.Dir(templatePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(templatePath, templateBody, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(templateBody)
	registryPath := filepath.Join(dir, "registry.json")
	registry := RemoteRegistry{
		SchemaVersion: 1,
		Playbooks: []RemoteRegistryPlaybook{
			{
				ID:             "template-url-playbook",
				Version:        "2.0.0",
				Name:           map[string]string{"zh-CN": "模板 URL 方案"},
				Description:    map[string]string{"zh-CN": "从独立模板文件加载"},
				TemplateURLs:   map[string]string{"zh-CN": "playbooks/template-url-playbook.json"},
				SHA256ByLocale: map[string]string{"zh-CN": fmt.Sprintf("%x", sum[:])},
			},
		},
	}
	registryBody, err := json.Marshal(registry)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(registryPath, registryBody, 0o644); err != nil {
		t.Fatal(err)
	}

	summaries, err := RemoteTemplates(context.Background(), "zh-CN", []string{"file://" + registryPath})
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 || summaries[0].Name != "模板 URL 方案" || len(summaries[0].Skills) != 0 {
		t.Fatalf("summary should use registry metadata without loading full template: %#v", summaries)
	}

	tmpl, ok := TemplateWithRemote(context.Background(), "template-url-playbook", "zh-CN", []string{"file://" + registryPath})
	if !ok {
		t.Fatal("template URL playbook not found")
	}
	if tmpl.Name != "Template URL Playbook" || len(tmpl.Skills) != 1 || tmpl.Skills[0].Body != "Full body from template file" {
		t.Fatalf("unexpected full template=%#v", tmpl)
	}
}

func TestRemoteTemplateOverridesBuiltinByID(t *testing.T) {
	registryPath := writeRegistry(t, RemoteRegistry{
		SchemaVersion: 1,
		Templates: []entity.PlaybookTemplate{
			{
				ID:          "bug-triage-and-fix",
				Version:     "99.0.0",
				Name:        "Remote Bug Flow",
				Description: "Override builtin",
			},
		},
	})

	tmpl, ok := TemplateWithRemote(context.Background(), "bug-triage-and-fix", "en", []string{registryPath})
	if !ok {
		t.Fatal("template not found")
	}
	if tmpl.Name != "Remote Bug Flow" || tmpl.Version != "99.0.0" {
		t.Fatalf("remote did not override builtin: %#v", tmpl)
	}
}

func writeRegistry(t *testing.T, registry RemoteRegistry) string {
	t.Helper()
	body, err := json.Marshal(registry)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "registry.json")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
