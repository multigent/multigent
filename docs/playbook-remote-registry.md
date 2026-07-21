# Playbook Remote Registry

> Status: v1 implementation  
> Scope: official remote registry, catalog metadata, per-locale template files

Multigent supports loading playbook templates from remote registry JSON in addition to the built-in templates.

The official registry is:

```text
https://raw.githubusercontent.com/multigent/playbooks/main/registry.json
```

The runtime can also load private registries for customer-specific playbooks.

## Configuration

Environment variable:

```bash
export MULTIGENT_PLAYBOOK_REGISTRY_URLS="https://raw.githubusercontent.com/multigent/playbooks/main/registry.json"
```

Multiple registries can be separated by comma, semicolon, or newline.

If no custom registry is configured, the `multigent` binary sets the official registry URL at startup. Built-in templates remain available even if the remote registry is unreachable.

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
  "generatedBy": "multigent tools/export-playbooks",
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
      "templateUrls": {
        "en": "playbooks/example-remote-playbook/en.json",
        "zh-CN": "playbooks/example-remote-playbook/zh-CN.json"
      },
      "sha256ByLocale": {
        "en": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
        "zh-CN": "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
      }
    }
  ]
}
```

Each template file is a full `PlaybookTemplate` JSON object. Keeping templates outside `registry.json` keeps list requests small while still allowing large skill prompts and workflow definitions.

The registry also accepts inline templates for local experiments:

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
      "roles": [],
      "skills": [],
      "workflows": []
    }
  ]
}
```

## Runtime Behavior

- Built-in templates are always available.
- Remote catalog metadata is fetched on list requests.
- Full remote templates are fetched on detail/install requests.
- If a remote template has the same ID as a built-in template, the remote template wins.
- If remote fetch fails, built-in templates still work.
- Template file checksums are verified when `sha256ByLocale` or `sha256` is provided.
- HTTP responses are limited to 20 MB.
- HTTP registry fetch has a 5 second timeout.

## Updating the Official Registry

From the `multigent` repository:

```bash
go run ./tools/export-playbooks --out ../playbooks
```

Then review and commit the generated changes in `multigent/playbooks`.
