---
name: multigent-admin-ops
description: Use the Multigent CLI to create and maintain teams, roles, projects, agents, workflows, task templates, tasks, docs, schedules, and operating playbooks for a workspace.
disable-model-invocation: true
---

# Multigent Admin Ops

You help a human set up and improve a Multigent workspace by operating the
`multigent` command line. Your job is to turn a company's current collaboration
process into executable Multigent objects: teams, roles, projects, agents,
workflows, task templates, tasks, docs, skills, and schedules.

Prefer concrete CLI actions over abstract advice. When a command may vary by
Multigent version, run `multigent <command> --help` first and adapt.

## Safety Rules

- Do not delete anything unless the user explicitly asks for deletion.
- Do not publish, send external messages, spend money, or deploy production
  changes without explicit human approval.
- Do not expose local filesystem paths to end users as product concepts.
- Prefer JSON files for workflows and task templates; they are easier to review,
  version, copy, and import.
- After changing teams, roles, skills, project prompts, or wakeup prompts, sync
  affected agent context before asking users to run the agents.
- Always report exactly what was created, IDs returned by commands, and what the
  user should click or run next.

## Discover Current Workspace

Run these first:

```bash
multigent version
multigent list teams
multigent list projects
multigent workflow list --format json
multigent task-template list --format json
```

If the user is not inside a Multigent workspace, ask for the workspace root or
start from the directory where `multigent api serve` is running.

## Standard Build Order

Use this order when setting up a company or a project:

1. Understand the business goal and the first workflow the user wants to run.
2. Create or reuse workspace-level teams.
3. Create roles under teams.
4. Create a project.
5. Hire human members and agent members into the project.
6. Create or import workflow definitions.
7. Create project-level task templates with actor bindings.
8. Configure model accounts in Web if not yet configured.
9. Configure schedules and wakeup prompts.
10. Create a test task from the template and run one end-to-end loop.

## Teams And Roles

Teams are functional groups, not projects. Good examples:

- `product`
- `engineering`
- `operations`
- `business`
- `marketing`
- `enablement`

Create a team:

```bash
multigent create team \
  --name "marketing" \
  --desc "Plans, writes, reviews, and distributes launch content."
```

Create a role:

```bash
multigent create role \
  --team "marketing" \
  --name "content-strategist" \
  --desc "Turns campaign goals into article angles, outlines, and distribution plans." \
  --skills "content-distribution-timing,multigent-usage"
```

Bind skills later:

```bash
multigent team skill add --team marketing --skill content-distribution-timing
multigent role skill add --team marketing --role content-strategist --skill multigent-usage
```

After creation, improve prompts in Web or by editing the generated team/role
prompt files if this is a local workspace.

## Projects And Agents

Create a project:

```bash
multigent create project \
  --name "launch-campaign" \
  --desc "Coordinate launch content, assets, review, and publishing."
```

Hire an agent:

```bash
multigent hire \
  --project "launch-campaign" \
  --team "marketing" \
  --role "content-strategist" \
  --model "claudecode" \
  --name "content-claude"
```

Hire a human participant when the workflow needs an explicit reviewer:

```bash
multigent hire \
  --project "launch-campaign" \
  --team "marketing" \
  --role "content-reviewer" \
  --model "human" \
  --name "owner-reviewer"
```

Use the actual workspace user ID for human workflow bindings when possible. Do
not invent a generic `human` user unless the local command explicitly requires
it.

## Workflow Definitions

Workflows are workspace-level state machines. A task can bind to one workflow.
Each step has structured inputs and outputs. Store large artifacts in Multigent
Docs and return `docID` values in workflow outputs.

List built-in templates:

```bash
multigent workflow templates --locale zh-CN
```

Create from a built-in template:

```bash
multigent workflow create \
  --template article-publishing \
  --name "文章宣传发布流程" \
  --locale zh-CN
```

Create from JSON:

```bash
multigent workflow create --file workflow.json
```

Minimal workflow JSON:

```json
{
  "id": "wf-launch-article",
  "name": "Launch Article Workflow",
  "description": "Draft, review, produce assets, assemble, and publish a launch article.",
  "version": 1,
  "startStepId": "outline",
  "steps": [
    {
      "id": "outline",
      "type": "agent_task",
      "title": "Draft article outline",
      "description": "Turn the topic into a clear angle, audience, outline, and required materials.",
      "actorRole": "content-strategist",
      "inputFields": [
        {"name": "topic", "description": "Article topic or campaign goal."},
        {"name": "platforms", "description": "Target publishing platforms."}
      ],
      "outputFields": [
        {"name": "outline_doc_id", "description": "docID of the article outline."},
        {"name": "asset_brief_doc_id", "description": "docID of required image/video material brief."}
      ],
      "position": {"x": 120, "y": 120}
    },
    {
      "id": "outline_review",
      "type": "human_review",
      "title": "Review outline",
      "description": "Approve or request changes to the outline.",
      "actorRole": "content-reviewer",
      "inputFields": [
        {"name": "outline_doc_id", "description": "docID of the outline to review."},
        {"name": "asset_brief_doc_id", "description": "docID of the asset brief to review."}
      ],
      "outputFields": [
        {"name": "decision", "description": "approve or request_changes."},
        {"name": "comments", "description": "Review comments if changes are needed."}
      ],
      "position": {"x": 460, "y": 120}
    },
    {
      "id": "draft",
      "type": "agent_task",
      "title": "Write full article",
      "description": "Write the complete draft and keep image placeholders structured.",
      "actorRole": "content-writer",
      "inputFields": [
        {"name": "outline_doc_id", "description": "Approved outline docID."}
      ],
      "outputFields": [
        {"name": "draft_doc_id", "description": "docID of the complete article draft."}
      ],
      "position": {"x": 820, "y": 120}
    }
  ],
  "edges": [
    {
      "id": "outline_to_review",
      "from": "outline",
      "to": "outline_review",
      "isDefault": true,
      "inputMapping": {
        "outline_doc_id": "outline_doc_id",
        "asset_brief_doc_id": "asset_brief_doc_id"
      }
    },
    {
      "id": "review_back_to_outline",
      "from": "outline_review",
      "to": "outline",
      "label": "request changes",
      "condition": {"field": "decision", "operator": "eq", "value": "request_changes"},
      "inputMapping": {"comments": "comments"}
    },
    {
      "id": "review_to_draft",
      "from": "outline_review",
      "to": "draft",
      "label": "approved",
      "condition": {"field": "decision", "operator": "eq", "value": "approve"},
      "inputMapping": {"outline_doc_id": "outline_doc_id"}
    }
  ]
}
```

Export a workflow for sharing:

```bash
multigent workflow export wf-launch-article --out workflow.json
```

## Task Templates

Task templates are project-level shortcuts. Use them when agents or humans need
to create repeatable tasks without remembering workflow IDs and actor bindings.

Create a task template JSON:

```json
{
  "id": "tt-launch-article",
  "name": "Launch article",
  "description": "Create a launch article through outline, review, draft, asset, final review, and publish steps.",
  "project": "launch-campaign",
  "type": "content",
  "priority": 1,
  "labels": ["launch", "article", "workflow"],
  "titleTemplate": "Launch article: {{topic}}",
  "descriptionTemplate": "Prepare and publish an article for {{platforms}}.",
  "promptTemplate": "Topic: {{topic}}\nTarget platforms: {{platforms}}\nAudience: {{audience}}\nGoal: {{goal}}\n\nUse the bound workflow. Store long outputs as Multigent Docs and return docIDs.",
  "workflowDefinitionId": "wf-launch-article",
  "workflowActorBindings": {
    "outline": {"type": "agent", "id": "content-claude"},
    "outline_review": {"type": "human", "id": "admin"},
    "draft": {"type": "agent", "id": "writer-codex"}
  },
  "variables": [
    {"name": "topic", "description": "Article topic.", "required": true},
    {"name": "platforms", "description": "Target platforms.", "required": true},
    {"name": "audience", "description": "Target reader.", "required": true},
    {"name": "goal", "description": "Business goal.", "required": true}
  ]
}
```

Import it:

```bash
multigent task-template create \
  --project "launch-campaign" \
  --file task-template.json
```

Create a task from the template:

```bash
multigent task add \
  --template "tt-launch-article" \
  --var topic="Why agents need workflows" \
  --var platforms="微信公众号, 掘金, X" \
  --var audience="AI builders and team leads" \
  --var goal="Drive internal beta signups" \
  --created-by "admin"
```

Override actor bindings when needed:

```bash
multigent task add \
  --template "tt-launch-article" \
  --var topic="..." \
  --var platforms="..." \
  --var audience="..." \
  --var goal="..." \
  --binding outline=agent:content-claude \
  --binding outline_review=human:admin \
  --created-by "admin"
```

## Knowledge Base Docs

Add durable context before creating tasks:

```bash
multigent docs add \
  --path ./notes/customer-interview-summary.md \
  --title "Customer interview summary" \
  --index "launch/research" \
  --created-by admin \
  --tag launch \
  --tag customer
```

Search docs:

```bash
multigent docs search "pricing" --content
multigent docs query "How should we position Multigent?"
```

When creating workflow outputs, prefer returning `docID` instead of long text.

## Schedules And Wakeup

Use scheduler commands to let agents work without manual prompting:

```bash
multigent scheduler heartbeat \
  --project "launch-campaign" \
  --agent "content-claude" \
  --enable \
  --interval 2h \
  --active-hours "09:00-23:00"

multigent start  # the service owns the workspace scheduler
multigent scheduler status
```

Wake an agent manually:

```bash
multigent fire --project launch-campaign --agent content-claude
```

If `fire` is unavailable in the installed version, run:

```bash
multigent scheduler --help
multigent run --project launch-campaign --agent content-claude
```

## Validation Checklist

After setup, verify:

```bash
multigent list teams
multigent role list --team marketing
multigent list projects
multigent list agents --project launch-campaign
multigent workflow list --format json
multigent task-template list --project launch-campaign --format json
multigent task list --project launch-campaign --agent content-claude
```

Then open the Web UI and check:

- The project exists.
- The agents are project members.
- The workflow appears in Workflows.
- The task template appears under the project.
- A task created from the template has a workflow run.
- The first workflow step has the expected responsible person or agent.

## Response Format

When you finish, report:

```markdown
## Created
- Teams:
- Roles:
- Projects:
- Agents:
- Workflows:
- Task templates:
- Tasks:
- Docs:

## Needs Human Action
- Model accounts to configure:
- External tools to connect:
- Human review bindings to confirm:

## Next Test
1. Open:
2. Click:
3. Wake:
4. Expected result:
```
