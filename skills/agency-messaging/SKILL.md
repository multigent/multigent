---
name: agency-messaging
description: Send, read, and reply to async Multigent messages through the mga runtime CLI.
---

# Skill: Agency Messaging

Use `mga inbox` to exchange async messages from inside an agent sandbox. Messages are non-blocking; the recipient reads them on their next wakeup or in the Web UI.

## Recipients

- Human owner: `human`
- Agent in the same project: `<project>/<agent>`

Runtime agents may send to `human` or agents in the same project.

## Read Messages

```bash
mga inbox messages
mga inbox list --archived
```

## Send A Message

```bash
mga inbox send \
  --to <project>/<agent> \
  --subject "Subject" \
  --body "Message body"

mga inbox send \
  --to human \
  --subject "Progress update" \
  --body "No action needed. Summary: ..."
```

Repeat `--to` to send to multiple recipients.

## Reply

```bash
mga inbox reply <message-id> --body "Reply body"
```

## When To Message

Use messages for:

- Non-blocking status updates.
- Passing context to another agent.
- Asking a peer agent to coordinate on a task.
- Escalating a risk without blocking current work.

Use `mga task confirm-request` instead when you need a human decision before continuing.
