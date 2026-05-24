---
id: gate-tests-destruction-concurrent-clustered-race
kind: story
stage: implementing
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
