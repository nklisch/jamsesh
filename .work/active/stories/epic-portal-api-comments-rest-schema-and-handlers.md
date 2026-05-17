---
id: epic-portal-api-comments-rest-schema-and-handlers
kind: story
stage: implementing
tags: [portal]
parent: epic-portal-api-comments-rest
depends_on: []
release_binding: null
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
