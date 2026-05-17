---
id: refactor-handler-auth-guards-comments
kind: story
stage: review
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

## Implementation notes

**Handlers migrated: 3** (`CreateComment`, `ListComments`, `ResolveComment`)

**LoC delta:** -74 net (52 insertions, 126 deletions; file: 402 → 328 lines)

**Auth pattern used:** `handlerauth.RequireSessionMember` for all three handlers.
The original code ran separate `GetOrgMember` + `GetSessionMember` checks. Both
were replaced by the single `RequireSessionMember` call, which composes account
extraction and session-membership verification. The explicit org-membership check
was dropped — being a session member implies org membership in this system.

**403-vs-404 ordering change:** All three handlers previously checked org
membership, then session membership, then fetched the session for a 404 check.
After migration, the auth guard (`RequireSessionMember`) runs first — before any
404 fetch. A non-member requesting a non-existent session now receives 403 instead
of 404. This is the intended "don't leak existence" posture, consistent with the
sibling story.

**Per-handler fail wrappers added:** `createCommentFail`, `listCommentsFail`,
`resolveCommentFail` — following the sessions template exactly.

**Handlers deliberately NOT migrated:** none. All three handlers followed the
standard pattern cleanly.
