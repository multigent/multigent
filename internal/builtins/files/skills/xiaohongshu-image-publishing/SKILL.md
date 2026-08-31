---
name: xiaohongshu-image-publishing
description: Create a practical Xiaohongshu image-post package and save it as an unpublished BrowserWing draft with an ordered set of local images.
---

# Xiaohongshu Image Publishing

Use this skill when preparing a Xiaohongshu image post. The goal is a usable draft in
the creator center, not a final publish action. The user can open the draft and review it
directly, so this flow does not add a separate human-review workflow node.

## Content package

Produce a complete package with:

- a short title suitable for Xiaohongshu, no more than 20 Chinese characters/runes;
- publishable body text only, without internal notes, workflow instructions, or editorial
  commentary;
- plain text only: no Markdown headings, list markers, code fences, or Markdown links; use
  real blank lines to separate paragraphs, and end with 2-5 relevant `#话题` hashtags;
- an ordered image set, with the cover first and the remaining images following the reading
  order;
- optional topics/hashtags only when they fit the actual content.

Use the smallest useful image set, normally 3-9 images. Do not create images merely to fill
slots. Prefer portrait images suitable for the Xiaohongshu feed, keep text large and legible,
and use real product screenshots when they are available. Do not invent product capabilities,
customers, metrics, or screenshots. For text-heavy visuals, generate a clean visual first and
use a deterministic local renderer when exact Chinese text must be legible.

## Image files

Before calling BrowserWing, verify every selected image exists on the Windows host or under a
WSL `/mnt/<drive>/...` path that the bridge can convert. The BrowserWing process runs on the
Windows host: a Linux-only path such as `/tmp/xhs_images/cover.png`, `/root/...`, or a path
inside the Agent sandbox is not readable by it. Copy or render final images into a shared
Windows-visible directory first, for example `/mnt/c/Users/<user>/multigent/xhs/<post-id>/`,
then pass those `/mnt/c/...` paths. Keep the files together in one post package and preserve
their order. Never pass an empty placeholder path.

The dedicated MCP tool accepts the following mapping:

| Order | Parameter |
| --- | --- |
| 1 | `file_path` |
| 2 | `file_path_1` |
| 3 | `file_path_2` |
| ... | ... |
| 9 | `file_path_8` |

The first image is required. Later paths are optional and may be omitted. Do not use
`file_path_9` or an `images` array with the dedicated tool; use the explicit parameter names
so the BrowserWing script receives the paths it supports.

## Create the draft

First inspect the available runtime tools:

```bash
mga runtime gateway list-tools --provider browserwing
```

Call `xiaohongshu.publish_draft` exactly once with the title, final body, and ordered image paths. The tool
invokes BrowserWing script `6281f5c5-a791-4703-9e15-2013477d28a4`, passing `file_path` through
`file_path_8` as needed. All arguments belong in the tool's normal argument object; do not use
unresolved `${title}`, `${content}`, or placeholder paths.

The call is a side-effecting draft save. If it times out, do not retry automatically: the result
may be unknown and a retry can create a duplicate draft or another browser tab. Inspect the
creator center or BrowserWing state first, then ask for an explicit retry.

The tool only saves a draft. Do not call a final publish action. If BrowserWing reports that
the creator session is logged out or requires CAPTCHA/2FA, stop and ask the user to complete
that step in the BrowserWing browser.

## Completion report

Report:

```json
{
  "platform": "xiaohongshu",
  "status": "draft_created|blocked|failed",
  "title": "...",
  "image_count": 0,
  "draft_reference": "...",
  "summary": "...",
  "next_action": "Open the Xiaohongshu creator-center draft for review"
}
```

Do not claim the post was published. The expected result is a draft that contains the actual
title, body, and image set.
