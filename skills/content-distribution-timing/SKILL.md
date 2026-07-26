---
name: content-distribution-timing
description: Plan platform-aware content publishing windows, launch reminders, and post-publish timing experiments for social, community, and long-form channels.
---

# Skill: Content Distribution Timing

Use this skill when planning when to publish or schedule external content: X/Twitter, LinkedIn, Reddit, Product Hunt, Hacker News, WeChat Official Account, Zhihu, Juejin, V2EX, Discord, Telegram, email, blogs, or community posts.

This skill does not decide content quality. It prevents good content from being released into a weak timing window without a reason.

## Core Rules

1. Treat timing as a default hypothesis, not truth. Always prefer the account's own analytics when available.
2. Match the audience timezone. Do not use server time unless the audience is actually in that timezone.
3. Separate publish time from prep time. Drafts, images, links, UTM, screenshots, and approval should be ready before the window starts.
4. High-stakes external posts need human approval before publication.
5. Record the chosen slot and reason in the task output or distribution doc.
6. After publishing, schedule a review window to capture impressions, clicks, comments, saves, reposts, leads, and lessons.

## Default Timing Matrix

Use this as a starting point when account analytics are missing.

| Channel | Best default windows | Avoid | Notes |
| --- | --- | --- | --- |
| X / Twitter, Chinese audience | Tue-Thu 07:30-09:30, 12:00-13:30, 19:30-22:00 China time | Weekends for serious B2B/tech posts | Morning for hot takes and memes; evening for deeper threads. |
| X / Twitter, global audience | Tue-Thu 12:00-18:00 audience local time | Weekends | Useful for fast iteration and replies. Stay active for the first hour. |
| LinkedIn | Tue-Thu late morning to afternoon audience local time | Weekends | Best for B2B, hiring, founder, enterprise, and long-form professional posts. |
| Reddit | 14:00-17:00 UTC for many dev/SaaS subreddits | Posting without reading subreddit rules | Fit subreddit culture exactly. Comments in the first hour matter. |
| Product Hunt | Launch starts at 00:00 Pacific Time | Unprepared weekday launch | Weekdays bring more traffic and more competition; weekends may rank easier with lower traffic. Prepare audience before launch day. |
| Hacker News | US/EU workday overlap, usually morning Pacific to early afternoon Eastern | Over-marketing copy | The title and authenticity matter more than exact timing. Stay live in comments. |
| WeChat Official Account | Workday 08:00-09:00, 12:00-13:30, 20:00-22:00 China time | Late night low-quality posts | Good for long-form trust building. Title, cover, and first screen matter. |
| Zhihu | Workday 12:00-13:30, 20:00-23:00 China time | Thin promotional answers | Lead with useful substance. Ads are punished by readers. |
| Juejin / V2EX | Workday morning, lunch, and early evening China time | Weekend low-effort posts | Developer communities prefer concrete lessons, demos, code, and honest tradeoffs. |
| Discord / Telegram community | When moderators can reply immediately | Fire-and-forget posting | Treat as conversation, not broadcast. |
| Email newsletter | Tue-Thu morning audience local time | Monday rush, Friday afternoon | Segment by timezone when possible. |

## X / Twitter Chinese Audience Baseline

For Chinese-language X posts aimed at domestic tech, indie hacker, cross-border, finance, or AI audiences:

- Best days: Tuesday, Wednesday, Thursday.
- Secondary days: Monday, Friday morning/noon.
- Weak days: Saturday and Sunday, unless the content is entertainment, memes, or hot topics.
- Fast traction slots: 07:30-09:30 and 12:00-13:30 China time.
- Deeper discussion slots: 19:30-22:00 China time.
- Practical default:
  - Tue-Thu 08:00: short hook, hot take, meme, launch reminder.
  - Tue-Thu 12:30: practical thread, product update, screenshot post.
  - Tue-Thu 20:30-21:30: long thread, tutorial, worldview essay.

## Output Contract

When you plan a content calendar, output:

```json
{
  "channel": "x-cn",
  "audience_timezone": "Asia/Shanghai",
  "recommended_slots": [
    {
      "local_time": "2026-07-28 08:00",
      "reason": "Tue morning commute window for Chinese tech audience",
      "content_type": "short launch hook"
    }
  ],
  "backup_slots": [],
  "approval_required": true,
  "review_at": "2026-07-29 08:00",
  "metrics_to_check": ["impressions", "engagements", "clicks", "comments", "follows", "leads"],
  "notes": "Use account analytics to update this default after 5-10 posts."
}
```

For a workflow step, return docIDs for long calendars:

- `content_calendar_doc_id`
- `distribution_plan_doc_id`
- `timing_experiment_doc_id`

## Publishing Checklist

Before publishing, verify:

- Content has final approval if it is external, commercial, or brand-sensitive.
- Platform version is adapted to the channel.
- Links and screenshots work.
- UTM or tracking is configured when needed.
- The first-hour owner is assigned to reply to comments.
- Backup slot exists if the current window is missed.
- Follow-up metrics review is scheduled.

## Learning Loop

After every 5-10 posts per platform:

1. Compare timing windows against actual impressions and engagement.
2. Separate content quality from timing. A bad post in a good slot is still a bad post.
3. Update the workspace's timing guide doc with account-specific findings.
4. Prefer the account-specific timing guide over this generic skill.
