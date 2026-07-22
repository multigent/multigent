# User Operations Workflow

This note records how Multigent should model user operations for community and
support channels.

## Goal

User operations should turn scattered community messages into a controlled
human-agent loop:

- agents watch channels and identify actionable messages;
- each actionable message becomes a task with a workflow run;
- low-risk technical replies can be sent by agents, while sensitive or uncertain
  public replies are reviewed by humans;
- product feedback and bugs are routed to PM instead of directly to developers;
- business leads are routed to business/owner instead of being answered casually;
- repeated questions are converted into durable FAQ/docs.

## Wakeup vs Workflow

Wakeup is the routine. It answers: "Should the user-ops agent look around now?"

The user-ops wakeup should:

- check inbox and active tasks;
- check available tools;
- sync or render Discord, Telegram, WeChat, Feishu/Lark, or email channels;
- create at most a few concrete user-message tasks;
- record channel health and stop.

Wakeup should not directly run the full support loop. It should not publish
external replies. Publishing decisions happen inside the workflow.

Workflow is the concrete task lifecycle. It answers: "How does this specific
user message move through the team?"

The workflow starts after a specific user message has been selected. It owns the
state transition, actor routing, review gates, structured outputs, and final
knowledge capture.

## Installed Assets

Workspace: Spaceship

Workflow definition:

- ID: `community-user-ops-loop-v1`
- Name: `用户运营消息处理流程`
- Scope: workspace

Project task template:

- ID: `tt-community-user-message`
- Project: `cc-connect`
- Default user-ops actor: `cc-connect/user-ops-claudecode`
- Default PM actor: `cc-connect/pm`
- Default business actor: `cc-connect/business-success`
- Default human reviewer: `admin`

## Workflow Shape

1. `用户消息分流`
   - Actor: user-ops agent
   - Output route: `ignore`, `direct_reply`, `ask_user_for_info`,
     `bug_or_support`, `product_feedback`, `product_review`, `business_lead`,
     `account_security`, `complaint`, `community_engagement`,
     `escalate_to_owner`

2. `记录并结束`
   - For ignored/no-action messages.

3. `起草用户回复`
   - Agent drafts a language-matched, externally safe reply.

4. `人工审核回复`
   - Human approves, requests changes, or holds the reply only when the draft
     says `approval_required=yes`.

5. `外发回复并记录`
   - Agent sends after either `approval_required=no` for low-risk technical
     replies, or after human approval for sensitive replies. It records message
     ID/link.

6. `FAQ / 反馈沉淀`
   - Agent creates FAQ, support note, feedback report, or receipt doc.

7. `PM 评估反馈`
   - PM decides whether the message should become product feedback, issue/dev
     work, request for more information, or owner escalation.

8. `Business 评估`
   - Business evaluates sponsorship, procurement, private deployment,
     partnership, and enterprise trial requests. It can draft a basic reply,
     ask for contact information, create follow-up work, decline, or escalate
     to owner.

9. `Owner / 管理员决策`
   - Used for sensitive topics: payment, account security, public commitments,
     sensitive sentiment, or roadmap uncertainty.

## Message Classification

The workflow covers these first-class user message categories:

| Route | Meaning | Default handling |
| --- | --- | --- |
| `direct_reply` | Known how-to or technical question with a clear answer. | User-ops drafts and may auto-send if low risk. |
| `ask_user_for_info` | Missing version, logs, reproduction steps, account state, or goal. | User-ops asks for the minimum useful information. |
| `bug_or_support` | Error, install/config failure, suspected product bug, or regression. | PM evaluates whether it is support, product feedback, or dev work. |
| `product_feedback` | Feature request, UX issue, workflow gap, or roadmap suggestion. | PM assesses value, priority, and next action. |
| `product_review` | Praise, light complaint, product opinion, or testimonial candidate. | User-ops replies and captures useful material. |
| `business_lead` | Pricing, sponsorship, procurement, private deployment, partnership, enterprise trial. | Business handles, with owner escalation for commitments. |
| `account_security` | Login, token, permission, privacy, data deletion, or security risk. | Human/owner review required. |
| `complaint` | Strong negative sentiment, public-risk complaint, refund/churn risk. | Human/owner review required before public reply. |
| `community_engagement` | Shared article, screenshot, tutorial, success story, or community contribution. | User-ops engages and records growth material. |
| `ignore` | Spam, duplicate, irrelevant, or no action needed. | Record and end. |
| `escalate_to_owner` | Strategic uncertainty, external commitment, or policy gap. | Owner/admin decision. |

## Agent Command Pattern

When wakeup finds an actionable message:

```bash
mga docs create --title "User message context <date>" \
  --index "cc-connect/user-ops/messages" \
  --tags user-ops,community,cc-connect \
  --content "<channel, sender, original message, context, initial notes>"

mga task create-from-template tt-community-user-message \
  --input channel="<Discord|Telegram|WeChat|Feishu|Lark|Email>" \
  --input sender="<user name or id>" \
  --input message_url="<url or none>" \
  --input message_text="<short original message summary>" \
  --input source_context_doc_id="<docID or none>"
```

When the agent is executing a workflow task:

```bash
mga workflow current <task-id>

mga task step done --id <task-id> \
  --output route=direct_reply \
  --output severity=normal \
  --output triage_doc_id=doc-... \
  --output reply_brief="..." \
  --summary "分流完成"
```

For the reply draft step:

```bash
mga task step done --id <task-id> \
  --output reply_draft_doc_id=doc-... \
  --output reply_text="..." \
  --output send_target="Discord channel/message ..." \
  --output approval_required=no \
  --output approval_reason="纯技术答复，事实来自公开文档，不涉及承诺或敏感信息" \
  --output triage_doc_id=doc-... \
  --summary "回复草稿完成，可自动发送"
```

## Open Questions

- Channel-specific send tools should eventually expose consistent dry-run and
  confirmed-send semantics.
- User identity mapping across Discord, Telegram, WeChat, GitHub, and CRM is not
  yet a first-class product object.
- High-frequency issue detection is currently prompt-driven. Later it should be
  backed by structured metrics and deduplication.
- The auto-send boundary should be evaluated over time. If low-risk replies
  repeatedly need human correction, tighten the prompt or route those cases back
  through review.
