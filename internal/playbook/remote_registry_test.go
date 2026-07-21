package playbook

import (
	"context"
	"encoding/json"
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
