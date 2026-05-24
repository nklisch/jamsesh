---
id: gate-tests-bearer-issuance-tx-split-partial-failure
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
