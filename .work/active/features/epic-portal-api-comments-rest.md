---
id: epic-portal-api-comments-rest
kind: feature
stage: implementing
tags: [portal]
parent: epic-portal-api
depends_on: [epic-portal-api-events-log, epic-portal-foundation-http-skeleton]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-17
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

## Generated-contracts scope

Per the SPEC.md generated-contracts decision, this feature adds the
following to `docs/openapi.yaml`:

- Endpoints under `paths:`: `GET /api/sessions/{id}/comments`,
  `POST /api/sessions/{id}/comments/{commentId}/resolve`
- Component schemas: `Comment` (the canonical row from PROTOCOL.md's
  Comment schema), `CommentAnchor`, `CommentKind` enum,
  `CommentListResponse`, `ResolveCommentRequest`, `ConflictEvent`
  (read-only shape — auto-merger writes it but this feature exposes
  the read API surface)

The `Comment` schema is reused by the MCP `post_comment` tool's
parameter shape and by the WebSocket `CommentAddedPayload` event — same
type across REST, MCP, and WebSocket, generated once.

Handlers implement the `oapi-codegen`-generated `ServerInterface`
methods. The internal library functions (`CreateComment`,
`ResolveComment`) accept the generated `Comment` struct directly.

## Design decisions

- **Schema**: ships `comments` table (00009 migration). `conflict_events` table already exists (shipped by `auto-merger-outcomes`); this feature just queries it for read endpoints.
- **Package**: `internal/portal/comments/`. Exposes both REST handlers (Handler) AND library functions (`CreateComment`, `ResolveComment`) called from `mcp-endpoint`.
- **Pagination**: reuse `internal/portal/pagination.Cursor`.
- **Single story**: cohesive feature.

## Implementation Units

### Unit 1: Schema

```sql
CREATE TABLE comments (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    author_account_id TEXT NOT NULL REFERENCES accounts(id),
    author_kind TEXT NOT NULL CHECK (author_kind IN ('human','agent')),
    anchor_commit_sha TEXT NOT NULL,
    anchor_file_path TEXT,
    anchor_line_start INTEGER,
    anchor_line_end INTEGER,
    body TEXT NOT NULL,
    addressed_to TEXT,
    kind TEXT NOT NULL CHECK (kind IN ('question','suggestion','action-request','fyi')),
    created_at DATETIME NOT NULL,
    resolved_at DATETIME,
    resolved_by_account_id TEXT REFERENCES accounts(id),
    resolution_note TEXT
);
CREATE INDEX comments_session_idx ON comments(session_id, created_at);
CREATE INDEX comments_addressed_idx ON comments(addressed_to);
```

### Unit 2: Queries

- `InsertComment :exec`
- `GetCommentByID :one`
- `ListCommentsForSession :many` — with optional filters (addressed_to LIKE, kind, resolved IS NULL, anchor_commit_sha)
- `ResolveComment :exec` — sets resolved_at, resolved_by_account_id, resolution_note
- `ListConflictEventsForSession :many` — open events, joined with addressed_to summary

### Unit 3: Library API

```go
package comments

func (s *Service) Create(ctx, params CreateParams) (Comment, error)
func (s *Service) Resolve(ctx, params ResolveParams) (Comment, error)
func (s *Service) List(ctx, params ListParams) ([]Comment, string /*next_cursor*/, error)
```

`Create` inserts comment row + emits `comment.added` event in one Tx.
`Resolve` updates row + emits `comment.resolved` event.

### Unit 4: REST handlers

`GET /api/sessions/<id>/comments` and `POST /api/sessions/<id>/comments/<commentId>/resolve`. RequireOrgRole + session-membership gating.

### Unit 5: openapi additions

Schemas: `Comment`, `CommentAnchor`, `CommentKind`, `CommentListResponse`, `ResolveCommentRequest`, `ConflictEvent` (read-only).

Note: the `CommentAnchor` schema already exists in openapi.yaml (added in events-log's payload shapes). Reuse it. The `Comment` struct in handlers uses fields aligned with `CommentAddedPayload` for consistency.

## Implementation Order

Single story.

## Testing

- Create + list + resolve round-trip
- Filter by addressed_to / kind / resolved / anchor
- Cursor pagination round-trip
- Resolve idempotency (already-resolved comment → 409)

## Risks

- **addressed_to as string**: filtering by recipient does substring match. Performance acceptable for v1 scale.
