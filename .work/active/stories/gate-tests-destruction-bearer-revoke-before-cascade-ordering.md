---
id: gate-tests-destruction-bearer-revoke-before-cascade-ordering
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

# Destruction defense-in-depth ordering (bearer revoke before session-delete) not asserted

## Priority
Medium

## Spec reference
Item: `story-epic-ephemeral-playground-session-lifecycle-destruction`

Acceptance criterion: Destruction routine: "step 4. Revoke all bearers ... step 6. Delete the sessions row (CASCADE handles `oauth_tokens.session_id` — the revoke in step 4 was defense-in-depth in case the cascade fails)."

## Gap type
missing test for valid partition (defense-in-depth verification)

## Suggested test
```go
func TestDestruction_BearerRevokedBeforeSessionDelete(t *testing.T) {
    // Inject a store wrapper that captures mutation sequence.
    // Run Destroy; assert RevokeBearersForSession is called BEFORE DeleteSession.
}
```

## Test location (suggested)
`internal/portal/playground/destruction_test.go`

## Implementation notes

Added `TestDestruction_BearerRevokedBeforeSessionDelete` to
`internal/portal/playground/destruction_test.go`.

The test injects an `orderCapturingStore` wrapper around the real SQLite store.
The wrapper intercepts `RevokeBearersForSession` and `DeleteSession`, appending
`"revoke"` or `"delete"` to a slice on each call. After `Destroy` returns, the
test asserts that the first `"revoke"` entry appears at a lower index than the
first `"delete"` entry, verifying the step-4-before-step-6 invariant.

Using a real store (rather than a pure mock) means the test exercises the full
cascade path: the tombstone is inserted, bearers are revoked, the session row is
deleted with FK cascade, and the anon account is cleaned up. The ordering
assertion is purely additive — it does not interfere with the existing
correctness assertions in the file.
