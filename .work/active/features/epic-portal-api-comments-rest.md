---
id: epic-portal-api-comments-rest
kind: feature
stage: drafting
tags: [portal]
parent: epic-portal-api
depends_on: [epic-portal-api-events-log, epic-portal-foundation-http-skeleton]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Portal API — Comments REST

## Brief

The comment data model and REST surface. Comments are first-class portal
entities anchored to a commit (optionally narrowed to a file path and
line range), with structured addressing metadata (`@user`,
`@user/branch`, `@all-agents`, `@all-humans`, `@everyone`,
`@auto-merger`) and a kind enum (`question`, `suggestion`,
`action-request`, `fyi`).

Posting comments is via the MCP `post_comment` tool (in
`mcp-endpoint`); humans post via the portal UI which calls MCP via the
portal's internal MCP client OR via a thin REST wrapper this feature
may expose if the UI design pass needs it. List + resolve are REST-only.

**Endpoints delivered**:

- `GET /api/sessions/<id>/comments` — list comments in a session,
  cursor-paginated, filterable by `addressed_to`, `kind`, `resolved`,
  `anchor.commit_sha`, `anchor.file_path`. Returns the canonical
  comment schema from `docs/PROTOCOL.md > Comment schema`.
- `POST /api/sessions/<id>/comments/<comment_id>/resolve` —
  mark a comment resolved with optional `resolution_note`. Emits a
  `comment.resolved` event.

**Internal API surface** (consumed by `mcp-endpoint`'s `post_comment`
and `resolve_comment` tools — these tools are thin proxies to library
functions exported from this feature):

- `CreateComment(ctx, sessionID, comment) (Comment, error)` — inserts
  the comment, emits `comment.added` event, returns the created
  comment with id + timestamp.
- `ResolveComment(ctx, sessionID, commentID, accountID, note)
  (resolvedAt time.Time, error)` — sets `resolved_at`, emits
  `comment.resolved`.

**Schema additions** (sqlc migrations owned by this feature):

- `comments` — id, org_id, session_id, author_account_id,
  author_kind (`human` | `agent`), anchor (`commit_sha`, nullable
  `file_path`, nullable `line_start`, nullable `line_end`), body,
  addressed_to (nullable string), kind (enum), created_at,
  resolved_at (nullable), resolved_by_account_id (nullable),
  resolution_note (nullable).
- `conflict_events` — schema per `docs/PROTOCOL.md > Conflict event
  schema`. The auto-merger from `epic-auto-merger` inserts into this
  table; this feature exposes the read API surface for the comment-like
  parts (conflicts are listed alongside comments in the digest).

**Addressing storage**: the `addressed_to` column is a single string in
the canonical syntax (`@<user>`, `@<user>/<branch>`, etc.); no
denormalization into recipient tables. Filtering by recipient is a
string-prefix or pattern match. Rationale: addressing syntax is open;
keeping it as a string sidesteps a recipient-resolution layer.

**No server-side dedup** (locked at epic-design): clients are
responsible for idempotency. If an agent posts the same comment twice,
the database has two rows.

Does NOT cover the MCP `post_comment` and `resolve_comment` wiring
(`mcp-endpoint` feature owns the tool plumbing).

## Epic context

- Parent epic: `epic-portal-api`
- Position in epic: parallel with sessions-rest and websocket-gateway;
  consumed by mcp-endpoint for tool implementations.

## Foundation references

- `docs/PROTOCOL.md` — Comment schema (canonical row shape), Conflict
  event schema, MCP tools (`post_comment`, `resolve_comment`), REST API
  > Sessions (comments listed under session id)
- `docs/SECURITY.md` — Trust model for participants (adversarial:
  "Comment provocatively (auditable; resolvable; revocable via member
  removal)")
- `docs/UX.md` — Flow: posting a comment

## Inherited epic design decisions

- **No server-side comment dedup**: client responsibility.
- **Pagination**: cursor-based with filter-hash invalidation.
- **Event emission**: `comment.added` / `comment.resolved` per the
  canonical envelope.

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
