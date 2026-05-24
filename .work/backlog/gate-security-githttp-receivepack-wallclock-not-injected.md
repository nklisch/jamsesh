---
id: gate-security-githttp-receivepack-wallclock-not-injected
kind: story
stage: drafting
tags: [security, portal, refactor, testing]
parent: null
depends_on: []
release_binding: null
gate_origin: security
created: 2026-05-24
updated: 2026-05-24
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
