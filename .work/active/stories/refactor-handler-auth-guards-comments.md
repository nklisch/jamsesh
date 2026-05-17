---
id: refactor-handler-auth-guards-comments
kind: story
stage: implementing
tags: [refactor, portal]
parent: refactor-handler-auth-guards
depends_on: [refactor-handler-auth-guards-helpers-and-sessions]
release_binding: null
gate_origin: refactor-design
created: 2026-05-17
updated: 2026-05-17
---

# Auth-Guards — Migrate comments handlers

Apply the `handlerauth` helpers from the sister story to
`internal/portal/comments/handlers.go`. Comments handlers gate on org +
session membership before all CRUD operations.

## Files

- Modify: `internal/portal/comments/handlers.go`

## Sites to migrate

- `CreateComment` — currently lines 30-83
- `ListComments` — currently around line 146
- `PatchComment` (or resolve) — currently around line 257

## Implementation notes

- Mirror the per-handler typed wrapper helper pattern from
  `refactor-handler-auth-guards-helpers-and-sessions` (e.g.
  `createCommentFail(f handlerauth.AuthFail) openapi.CreateCommentResponseObject`).
- Comments handlers may need both org and session membership checks — use
  `RequireSessionMember`, which composes both.

## Acceptance

- [ ] `go build ./...` passes
- [ ] `go test ./internal/portal/comments/...` passes
- [ ] `internal/portal/comments/handlers.go` total LoC drops by ~60
- [ ] Zero direct calls to `store.GetOrgMember` / `store.GetSessionMember`
      remain in `comments/handlers.go`

## Risk

LOW. Same as the sister story.

## Rollback

Single-file `git revert`.
