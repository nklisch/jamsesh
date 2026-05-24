---
id: gate-tests-destruction-bearer-revoke-before-cascade-ordering
kind: story
stage: implementing
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
