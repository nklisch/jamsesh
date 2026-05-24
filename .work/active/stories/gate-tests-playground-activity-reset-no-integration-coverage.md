---
id: gate-tests-playground-activity-reset-no-integration-coverage
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

# Activity-reset wiring on substantive events has zero integration coverage across all three call-sites

## Priority
Critical

## Spec reference
Item: `story-epic-ephemeral-playground-session-lifecycle-abuse-caps`

Acceptance criterion: Substantive activity events reset the idle timer correctly (verified by post-event SELECT of `last_substantive_activity_at`) — Story 3 AC #5 of `feature-epic-ephemeral-playground-session-lifecycle`. Parent feature Risks: "Activity-reset miss = sessions die early ... Mitigation: integration test that pushes a commit, waits 25 min, pushes another commit, then waits past the original 30m mark — verifies session still alive because second push reset the timer."

## Gap type
missing test for valid partition + e2e-seam

## Suggested test
```go
// internal/portal/githttp/receive_pack_test.go
func TestPostReceive_PlaygroundActivityResetsIdleTimer(t *testing.T) {
    // 1. Create playground session: last_substantive_activity_at = T0,
    //    idle_timeout_at = T0 + 30m.
    // 2. Advance clock to T0 + 25m.
    // 3. Drive a successful post-receive on the playground session.
    // 4. SELECT last_substantive_activity_at, idle_timeout_at — assert both
    //    moved forward (timer reset).
    // 5. Negative control: same flow on a durable session — values UNCHANGED.
}
// Mirror in comments/service_test.go and sessions/handler_test.go.
```

## Test location (suggested)
`internal/portal/githttp/receive_pack_test.go`, `internal/portal/comments/service_test.go`, `internal/portal/sessions/handler_test.go`
