---
id: gate-security-githttp-receivepack-wallclock-not-injected
kind: story
stage: done
tags: [security, portal, refactor, testing]
parent: feature-playground-hardening
depends_on: []
release_binding: null
gate_origin: security
created: 2026-05-24
updated: 2026-05-25
---

# Per-session activity-reset on push runs under wall-clock `time.Now()` rather than the injected playground clock

## Severity
Low

## Domain
Authentication & Authorization

## Location
`internal/portal/githttp/receive_pack.go:336-347`

## Evidence
```go
if orgID == playgroundOrgID && h.PlaygroundIdleTimeout > 0 {
    now := time.Now().UTC()
    resetErr := h.Store.ResetSessionIdleTimer(r.Context(), store.ResetSessionIdleTimerParams{
        OrgID:                     orgID,
        SessionID:                 sessionID,
        LastSubstantiveActivityAt: now,
        IdleTimeoutAt:             now.Add(h.PlaygroundIdleTimeout),
    })
```

The rest of the playground subsystem (`handler.go`, `worker.go`,
`destruction.go`) uses an injected `Clock` (per the
`per-package-clock-interface` pattern) so `/test/clock-advance` can move all
expirations together. Using wall-clock here means a clock-advance e2e test
that bumps the destruction worker past `idle_timeout_at` can be silently
undone by a real-time push that resets to the wall clock. Not exploitable in
production, but it makes the idle-cap behaviour racy under load and
untestable via the established clock-injection contract.

## Remediation direction
Inject a `Clock` into `githttp.Handler` (mirror the playground package shape)
and use `h.Clock.Now().UTC()` for the timestamp.

## Implementation notes

- Added `internal/portal/githttp/clock.go` with `Clock interface{ Now() time.Time }`,
  `realClock{}`, and `RealClock() Clock`. Matches the `per-package-clock-interface`
  pattern — no import coupling with the `playground` package; `*testclock.AdvanceableClock`
  satisfies both structurally.
- `githttp.Handler` gains a `Clock Clock` field. When nil, `receive_pack.go` falls
  back to `RealClock()` (no panic on stale wiring).
- `receive_pack.go` activity-reset block now uses `clk := h.Clock; if clk == nil { clk = RealClock() }; now := clk.Now().UTC()`.
- `cmd/portal/main.go` passes `Clock: githttp.RealClock()` at handler construction.
- `TestPostReceive_PlaygroundActivityResetsIdleTimer` rewritten to use a
  `fixedGitHTTPClock` and assert exact-time equality (`TPush` and `TPush+idleTimeout`),
  replacing the `> beforePush` wall-clock heuristic.
- New `TestPostReceive_PlaygroundActivityReset_NilClockFallsBackToRealClock`
  exercises the nil-Clock fallback path so a future refactor cannot accidentally
  break the no-panic guarantee.

Verified: `go test ./internal/portal/githttp/... -count 1` passes.

## Review (2026-05-25)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Textbook application of the per-package-clock-interface pattern. Nil-Clock fallback at call site is the right defensive default; production wiring in `cmd/portal/main.go` injects `RealClock()`. Tests now assert exact-time equality (much stronger than the prior wall-clock heuristic) plus a fallback test pins the nil-Clock no-panic guarantee.
