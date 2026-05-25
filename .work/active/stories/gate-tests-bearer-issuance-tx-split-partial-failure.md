---
id: gate-tests-bearer-issuance-tx-split-partial-failure
kind: story
stage: done
tags: [testing, portal, playground]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# Bearer issuance + session-creation TX split partial-failure window untested

## Priority
High

## Spec reference
Item: `story-epic-ephemeral-playground-session-lifecycle-rest-endpoints`

Acceptance criterion: Implementation summary: "Bearer issuance split outside the session WithTx (3-step sequence: session TX → bearer issuance → member insert) to avoid SQLite WAL deadlock; partial failure leaves orphaned session that destruction sweep cleans up."

## Gap type
adversarial-spec-silent

## Suggested test
```go
func TestCreatePlaygroundSession_BearerIssuanceFails_OrphanRecovered(t *testing.T) {
    // Inject a failing tokens.Service that errors from
    // IssueAnonymousSessionBearer AFTER the session TX commits.
    // Assert session row persists (orphaned w/o member or bearer);
    // running destruction.Destroy on it later cleans up fully.
}
```

## Test location (suggested)
`internal/portal/playground/handler_test.go`

## Implementation notes

Added `TestCreatePlaygroundSession_BearerIssuanceFails_OrphanRecovered` to
`internal/portal/playground/handler_test.go`. The test uses dialect-loop
coverage via `stores(t)`.

### Changes made

1. **`failingTokensService` extended** — added a `lastSessionID string` field
   that captures the `sessionID` argument on every call to
   `IssueAnonymousSessionBearer` (regardless of fail/pass). This is the "Option
   B" approach from the story: the spy field lets the test recover the
   handler-generated session ID without needing a `ListSessions` scan.

2. **New test** — `TestCreatePlaygroundSession_BearerIssuanceFails_OrphanRecovered`:
   - Arms `failingTokensService.issueErr` so step 2 always errors.
   - POSTs `CreatePlaygroundSession`, asserts 5xx response.
   - Reads `fts.lastSessionID` to get the committed session ID.
   - Calls `store.GetSession` directly — asserts the orphaned row persists.
   - Calls `store.CountSessionMembers` — asserts 0 members (step 3 skipped).
   - Checks `stor.RepoExists` — asserts no bare repo (CreateRepo never reached).
   - Runs `Destruction.Destroy(ctx, sess, "orphan_recovery")`.
   - Asserts `store.GetSession` returns `ErrNotFound` (session deleted).
   - Asserts `CountSessionMembers` returns 0 post-destroy (FK cascade).

### Verification

- `go test ./internal/portal/playground/ -run TestCreatePlaygroundSession_BearerIssuanceFails_OrphanRecovered -count=1` — PASS
- Full suite: all tests pass except the two pre-existing parked failures
  (`TestJoinPlaygroundSession_Success`, `TestJoinPlaygroundSession_WithNickname_UsesIt`).

## Review notes

Approve. Stub fails ONLY at step 2 (bearer issuance), proving step 1
committed and step 3 was skipped. Captures the handler-generated sessionID via
a spy field (a clean alternative to a ListSessions scan), then exercises the
real Destruction cascade and asserts ErrNotFound. Test passes.
