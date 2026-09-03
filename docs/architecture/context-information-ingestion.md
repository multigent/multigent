# Context Information Ingestion

## Purpose

External information should be collected independently from Agent execution. A
collector writes normalized records into the context center; an Agent decides
whether a record is relevant and whether it should become a publication idea.
The collector is not an Agent trigger and must not inject the full source into a
runtime prompt.

## Collector boundary

`multigent` does not fetch or parse external systems. A collector is an
independent process, possibly installed from a future collector marketplace.
It fetches and normalizes data, then calls the generic API:

```http
POST /api/v1/context/ingest
Authorization: Bearer <client token with context.write>
X-Multigent-Workspace-ID: <workspace id>
Content-Type: application/json
```

The request contains a `source` object and up to 500 normalized `items`:

```json
{
  "source": {
    "id": "src-blog-example",
    "type": "rss_atom",
    "name": "Example Blog",
    "connectionRef": "https://example.com/feed.xml"
  },
  "items": [{
    "sourceItemId": "entry-123",
    "sourceUrl": "https://example.com/posts/123",
    "title": "A post",
    "summary": "Short summary",
    "content": "Normalized content",
    "occurredAt": "2026-08-30T08:00:00Z",
    "authorType": "external",
    "authorId": "author",
    "labels": {"topics": ["agents"]}
  }]
}
```

This endpoint is a machine integration boundary, separate from browser/user
authentication. It accepts only a workspace client token (`mgpat_...`) in the
`Authorization` header with the `context.write` scope. User JWTs, static API
keys, trusted-proxy headers, and query-string tokens are rejected. The token's
user permissions still apply to the selected workspace and any project or
agent scope on an item.

The API enforces workspace access and `context.write`, persists the source and
items, derives a stable dedupe key from `source.id + sourceItemId` when the
collector does not provide one, and returns created/deduplicated counts. It
does not create attention signals. Agents decide what context to subscribe to
and when to inspect it; ingestion is not an Agent trigger.

The standalone development collector is kept outside this repository at
`/root/code/spaceship/local_tools/context-feed-collector`. It currently handles
RSS/Atom and demonstrates the API contract. The same contract can later be
used by GitHub, Lark/Feishu, newsletters, Reddit, or other collectors.

## Signal contract

Signals contain a short summary and references to the context item and source
URL. They mean “new information is available for your judgment”, not “perform
this instruction now”. The Agent can read, defer, ignore, or mark the signal
handled according to its attention policy.

## Planned adapters

- GitHub releases, issues, and discussions
- RSS/Atom blogs and newsletters
- Public community sources with explicit rate limits
- Lark/Feishu documents and meeting artifacts, after user authorization

Authenticated sources must use connection references or environment-managed
credentials. Tokens must never be stored in feed URLs, context item content, or
repository files.

## Required inputs from the workspace owner

The first version can run with public feed URLs. To make it useful for daily
content planning, provide a small curated list of sources with:

- source URL or API identifier
- why the source matters
- preferred topics or tags
- maximum acceptable frequency
- whether it may produce signals for `content-curator`
- sensitivity and publication restrictions

Credentials for private Lark, X, Reddit, or internal sources can be added later
through the existing connection system; they are not required for the public
feed path.
