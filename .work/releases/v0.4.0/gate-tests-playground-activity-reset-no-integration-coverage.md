---
id: gate-tests-playground-activity-reset-no-integration-coverage
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

## Implementation notes

Three integration tests added, one per substantive-action surface:

### `internal/portal/comments/service_test.go`
`TestServiceCreate_PlaygroundSession_ResetsIdleTimer` (two subtests)

- Seeds an org with `org_playground` orgID and a session row with `last_substantive_activity_at = T0`, `idle_timeout_at = T0+30m`.
- Injects a `fakeClock` (already declared in `clock_test.go`) at `T0+25m` and sets `Service.PlaygroundIdleTimeout = 30m`.
- Calls `svc.Create(...)` with a playground comment.
- SELECTs the session row and asserts `last_substantive_activity_at > T0` and `idle_timeout_at > T0+30m` (reset moved both forward to `T0+55m`).
- Negative control: same flow with a non-playground orgID — both fields remain equal to their seed values.

### `internal/portal/sessions/handler_test.go`
`TestFinalizeSession_PlaygroundSession_ResetsIdleTimer` (two subtests)

- Introduces a package-local `sessionsFakeClock` (named distinctly to avoid collision with any existing clock types).
- Calls `sessions.NewWithClock(..., clk).WithPlaygroundIdleTimeout(30m)` to wire the clock + timeout.
- Seeds a `org_playground` session with timer fields at T0, advances clock to T0+25m.
- Issues a POST `/api/orgs/org_playground/sessions/{id}/finalize` via the full HTTP stack (strict-handler shim).
- Asserts `last_substantive_activity_at > T0` and `idle_timeout_at > T0+30m`.
- Negative control: durable orgID — fields unchanged.

### `internal/portal/githttp/receive_pack_test.go`
`TestPostReceive_PlaygroundActivityResetsIdleTimer` (two subtests)

- `receive_pack.go` uses `time.Now().UTC()` directly (no injected clock), so T0 is anchored in the distant past (2020-01-01) rather than the future. After a successful push the handler resets both fields to `time.Now()`, which is well after T0.
- Adds `mustCreatePlaygroundSession` helper on `pushEnv` that seeds an org+session with playground timer fields.
- Constructs a `githttp.Handler` with `PlaygroundIdleTimeout = 30m` and `Emitter` set (needed for the full post-receive path).
- Runs a full git push through the HTTP server.
- Asserts `last_substantive_activity_at > T0` and `last_substantive_activity_at >= beforePush` (where `beforePush = time.Now()` captured just before the push).
- Negative control: durable orgID — fields equal T0 after the push.

All tests pass. No production bugs discovered; wiring is correct end-to-end.

## Review notes

Approve. Three integration tests across three call-sites (comments, sessions
finalize, git post-receive) each with a positive (playground) and negative
(durable) control. SELECT-based assertions on `last_substantive_activity_at`
and `idle_timeout_at` after the real handler executes. The git test uses a
real httptest server, real Handler, real storage, real push — not stubbed.
All 6 subtests pass.
