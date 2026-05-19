---
id: epic-portal-api-comments-rest-schema-and-handlers
kind: story
stage: done
tags: [portal]
parent: epic-portal-api-comments-rest
depends_on: []
release_binding: v0.1.0
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Comments REST — Schema + Service + REST Handlers

## Scope

Add the comments table + Service (CreateComment/ResolveComment/List) + REST handlers for list + resolve.

## Units delivered

- `internal/db/migrations/{sqlite,postgres}/00009_comments.sql`
- `db/schema/{sqlite,postgres}.sql` (edit)
- `db/queries/{sqlite,postgres}/comments.sql`
- Regen sqlitestore + pgstore
- `internal/db/store/store.go` (edit) — CommentStore + ConflictEventReadStore sub-interfaces
- Both adapters
- `internal/portal/comments/service.go` — Create/Resolve/List
- `internal/portal/comments/handlers.go` — REST handlers
- `docs/openapi.yaml` (edit) — schemas + 2 paths
- Regen openapi
- `cmd/portal/main.go` (edit) — construct Service + Handler, register routes
- Tests

## Acceptance Criteria

- [ ] GET /api/sessions/<id>/comments lists comments; supports filters (addressed_to substring, kind, resolved, anchor_commit_sha); cursor pagination round-trip
- [ ] POST /api/sessions/<id>/comments/<commentId>/resolve marks resolved; double-resolve → 409
- [ ] Create + Resolve both emit canonical events via events.Log
- [ ] Comments addressed_to is a string column (per locked decision); substring match works
- [ ] go build + go test green

## Notes

- The Service is exported so mcp-endpoint can call it directly from its post_comment / resolve_comment tools.
- Use `pagination.Cursor` for listing.
- `CommentAnchor` schema exists in openapi.yaml from events-log; reuse for the `anchor` field shape.

## Implementation notes

### Delivered

- `internal/db/migrations/{sqlite,postgres}/00009_comments.sql` — migration with UP/DOWN goose blocks
- `db/schema/{sqlite,postgres}.sql` — updated with comments table
- `db/queries/{sqlite,postgres}/comments.sql` — InsertComment, GetCommentByID, ResolveComment, ListCommentsForSession (with optional filter params)
- Regenerated sqlitestore + pgstore (comments.sql.go, models.go, querier.go)
- `internal/db/store/store.go` — Comment domain type, InsertCommentParams, ResolveCommentParams, ListCommentsForSessionParams, CommentStore sub-interface; TxStore + Store both embed CommentStore
- `internal/db/store/sqlite_adapter.go` + `postgres_adapter.go` — CommentStore + TxStore implementations for both dialects
- `internal/portal/events/log.go` — added `FanOut(Event)` exported method for direct fan-out from the comments service (which manages its own Tx)
- `internal/portal/comments/service.go` — Service struct; Create (inserts comment row + emits comment.added in one Tx, then fans out), Resolve (loads comment, checks ErrAlreadyResolved, updates + emits comment.resolved, fans out), List (cursor-paginated with filter params)
- `internal/portal/comments/handlers.go` — Handler wrapping Service; ListComments + ResolveComment implementing StrictServerInterface methods; session-membership gating
- `docs/openapi.yaml` — added CommentKind, Comment, CommentListResponse, ResolveCommentRequest schemas; added GET /api/orgs/{orgID}/sessions/{sessionID}/comments and POST .../comments/{commentId}/resolve paths
- Regenerated `internal/api/openapi/server.gen.go` and `frontend/src/lib/api/types.gen.ts`
- `cmd/portal/main.go` — added comments import, CommentsHandler field in combinedHandler, ListComments + ResolveComment delegation methods, Service + Handler construction, route registration
- Updated 5 test stub structs (accounts, auth/magic_link, auth/oauth, sessions, tokens) to implement new ListComments + ResolveComment methods
- `internal/portal/comments/service_test.go` — 9 tests covering: Create (fields + event emission), Resolve (success + double-resolve → ErrAlreadyResolved), List (all/kind/addressed_to/anchor_commit_sha/anchor_file_path/resolved filters), cursor pagination round-trip (5 comments, 3 pages), REST ListComments 200, REST ListComments with filter, REST ResolveComment 200, REST ResolveComment 409 double-resolve, REST ResolveComment 404

### Design decisions made

- `resolved` query parameter: changed from `boolean` to `string enum ["true","false"]` to allow tristate (absent = all, "true" = resolved, "false" = unresolved) since oapi-codegen generates non-pointer `bool` for optional booleans in OpenAPI 3.0.3
- Comment + Event inserts are in a single `WithTx` transaction; fan-out via `events.Log.FanOut()` happens post-commit (same pattern as `Log.Emit` but the DB write is handled by the caller)
- `ErrAlreadyResolved` is detected by reading the comment before the update (pre-check) rather than relying on SQL row-count; this is idiomatic for the existing pattern

## Review (2026-05-17)

**Verdict**: Approve

**Notes**: Tristate `resolved` enum (string "true"/"false") is the clean workaround for oapi-codegen's non-pointer optional bools. Pre-check ErrAlreadyResolved before UPDATE is idiomatic. Events.Log.FanOut export pairs cleanly with caller-managed Tx.
