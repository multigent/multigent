# Playbook Remote Registry

> Status: v1 implementation  
> Scope: remote registry fetch, inline playbook templates, future bundle migration

Multigent supports loading playbook templates from remote registry JSON in addition to the built-in templates.

The first implementation intentionally supports inline templates only. Bundle download, signing, and external asset packages should be added after the registry path is proven.

## Configuration

Environment variable:

```bash
export MULTIGENT_PLAYBOOK_REGISTRY_URLS="https://raw.githubusercontent.com/multigent/playbooks/main/registry.json"
```

Multiple registries can be separated by comma, semicolon, or newline.

TOML:

```toml
[playbooks]
registry_urls = [
  "https://raw.githubusercontent.com/multigent/playbooks/main/registry.json"
]
```

Local file paths are also supported for development:

```bash
export MULTIGENT_PLAYBOOK_REGISTRY_URLS="file:///tmp/playbook-registry.json"
```

## Registry JSON

Recommended v1 shape:

```json
{
  "schemaVersion": 1,
  "templates": [
    {
      "id": "example-remote-playbook",
      "version": "1.0.0",
      "name": "Example Remote Playbook",
      "description": "Loaded from a remote registry.",
      "locale": "en",
      "category": "Example",
      "complexity": "Basic",
      "tags": ["example"],
      "roles": [],
      "skills": [],
      "workflows": [],
      "taskTemplates": [],
      "requiredTools": [],
      "setupQuestions": [],
      "successMetrics": []
    }
  ]
}
```

The registry also accepts this wrapper shape, which leaves room for bundle metadata later:

```json
{
  "schemaVersion": 1,
  "playbooks": [
    {
      "id": "example-remote-playbook",
      "version": "1.0.0",
      "name": {
        "en": "Example Remote Playbook",
        "zh-CN": "远程示例协作方案"
      },
      "description": {
        "en": "Loaded from a remote registry.",
        "zh-CN": "从远程 registry 加载。"
      },
      "category": {
        "en": "Example",
        "zh-CN": "示例"
      },
      "complexity": {
        "en": "Basic",
        "zh-CN": "基础"
      },
      "tags": ["example"],
      "template": {
        "id": "example-remote-playbook",
        "version": "1.0.0",
        "roles": [],
        "skills": [],
        "workflows": []
      }
    }
  ]
}
```

## Runtime Behavior

- Built-in templates are always available.
- Remote templates are fetched on list/detail/install requests.
- If a remote template has the same ID as a built-in template, the remote template wins.
- If remote fetch fails, built-in templates still work.
- HTTP responses are limited to 20 MB.
- HTTP registry fetch has a 5 second timeout.

## Next Phase

The next phase should add bundles:

```json
{
  "id": "video-production-studio",
  "version": "1.0.0",
  "bundle": {
    "url": "https://github.com/multigent/playbooks/releases/download/video-production-studio-v1.0.0/video-production-studio.tar.gz",
    "sha256": "..."
  }
}
```

Bundle support should include:

- checksum verification
- source and license metadata
- schema validation
- local cache
- update policy
- install provenance
- optional signature verification
