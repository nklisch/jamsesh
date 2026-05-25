---
id: gate-tests-destruction-concurrent-clustered-race
kind: story
stage: done
tags: [testing, portal, playground, concurrency]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# Concurrent destruction calls for same session not tested — clustered-mode race uncovered

## Priority
High

## Spec reference
Item: `story-epic-ephemeral-playground-session-lifecycle-destruction`

Acceptance criterion: Parent feature Risks: "Clustered-mode interaction: under JAMSESH_DEPLOY_MODE=clustered, multiple portal pods run. The destruction worker runs on every pod. Concurrent destruction attempts on the same session row could race." Review filed `bug-playground-destruction-clustered-advisory-lock` as backlog.

## Gap type
missing test for adversarial-spec-silent (concurrency)

## Suggested test
```go
func TestDestruction_ConcurrentDestroyCallsForSameSession_NoCorruption(t *testing.T) {
    // Two goroutines call Destroy(ctx, sessionID, "hard_cap") concurrently.
    // Assert: exactly one tombstone row written (idempotency), no panics,
    // anon accounts not double-deleted, bearer revoke + delete idempotent.
}
```
`TestDestruction_Idempotent` covers sequential re-invocation only.

## Test location (suggested)
`internal/portal/playground/destruction_test.go`

## Implementation notes

Added `TestDestruction_ConcurrentDestroyCallsForSameSession_NoCorruption` to
`internal/portal/playground/destruction_test.go`.

**Approach:**
- Two goroutines release simultaneously via a `close(barrier)` pattern, both
  calling `Destroy(ctx, sess, "hard_cap")` on the same session.
- A local `concurrentSafeStorage` type wraps `stubStorage` with a `sync.Mutex`
  so the race detector doesn't fire on the stub's map (a test-infra concern,
  not production).
- SQLite `:memory:` gives each pool connection its own empty database, so
  `rawDBer.RawDB().SetMaxOpenConns(1)` is called to pin all goroutines to the
  same in-memory DB — mirroring the pattern in `automerger/worker_test.go`.
- Assertions cover: no errors from either goroutine, exactly one tombstone,
  session deleted, anon account deleted, bare repo removed.

**Outcome:** The test passes cleanly with `-race`. `Destroy` is already
concurrency-safe at the in-process level — the idempotency guards (ON CONFLICT
DO NOTHING for tombstone, ErrNotFound tolerance for DeleteSession) hold under
concurrent invocation. No race conditions detected. The broader clustered-mode
risk (two pods, no advisory lock) remains documented in
`bug-playground-destruction-clustered-advisory-lock`.

## Review notes

Approve. Barrier-released goroutines force real overlap; SetMaxOpenConns(1)
pins to the shared in-memory SQLite db. Asserts no errors from either
goroutine, exactly-one tombstone, session ErrNotFound, account ErrNotFound,
repo gone. Passes with -race. Test is honest about scope: it pins the
in-process idempotency contract, not cross-pod (clustered) safety which is
tracked separately.
