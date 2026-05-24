---
id: gate-tests-bare-repo-create-orphan-destruction-cleanup
kind: story
stage: review
tags: [testing, portal, playground]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# Bare-repo create failure mid-create leaves orphaned session — destruction-sweep cleanup not verified end-to-end

## Priority
High

## Spec reference
Item: `story-epic-ephemeral-playground-session-lifecycle-rest-endpoints`

Acceptance criterion: Story 1 AC: "Bare-repo create failure rollback: if `CreateRepo` errors after session insert, session is marked abandoned (destruction sweep will clean up)." Design: "the orphaned state is { session row + creator member row + no bare repo on disk }. The destruction sweep cleans by session_id."

## Gap type
missing test for e2e-seam

## Suggested test
```go
func TestCreatePlaygroundSession_RepoCreateFails_DestructionSweepCleansUp(t *testing.T) {
    // 1. Create session via stubStorage.createError = errors.New("disk full").
    // 2. Verify session row + creator member row remain.
    // 3. Advance clock past hard_cap_at OR set status=expired.
    // 4. Run destruction.Destroy(ctx, sessionID, "manual").
    // 5. Assert session row gone, member row gone, bearer revoked + deleted.
}
```
`TestCreatePlaygroundSession_RepoCreateFails_ReturnsError` asserts orphan
exists but never exercises cleanup end-to-end.

## Test location (suggested)
`internal/portal/playground/handler_test.go`

## Implementation notes

Added `TestCreatePlaygroundSession_RepoCreateFails_DestructionSweepCleansUp` to
`internal/portal/playground/handler_test.go` (immediately after the existing
`TestCreatePlaygroundSession_RepoCreateFails_ReturnsError` test).

The test:
1. Sets `stubStorage.createError = errors.New("disk full")` and drives
   `CreatePlaygroundSession` via the HTTP endpoint — gets 5xx.
2. Uses `ListExpiredPlaygroundSessions` with `Now + 48h` to locate the orphaned
   session row (same technique as the existing sibling test).
3. Asserts orphan state: session row present, `CountSessionMembers` > 0, no bare
   repo in stub storage (since `CreateRepo` was never called successfully).
4. Calls `destruction.Destroy(ctx, sess, "manual")` directly (bypassing the
   worker sweep).
5. Asserts full cleanup: session row gone (`ErrNotFound`), member count zero
   (FK cascade), all anonymous account IDs deleted, bare repo still absent
   (exercises `RemoveRepo` idempotency on a never-created repo).

Verified: `go build ./internal/portal/playground/...` + both
`TestCreatePlaygroundSession_RepoCreateFails_*` tests pass (SQLite dialect).
No new backlog items required — destruction sweep cleanly handles the orphan.
