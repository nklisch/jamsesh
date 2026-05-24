# Playground Activity Reset on Substantive Action

Every write path that constitutes "substantive collaboration" on a
playground session — comment creation, finalize-attempt, successful git push
— calls `store.ResetSessionIdleTimer` after the primary mutation succeeds,
guarded by `if orgID == playgroundOrgID && <handler>.PlaygroundIdleTimeout >
0`. Failure is logged at Warn via `slog.WarnContext(...)` and intentionally
swallowed: the action succeeded; only the timer reset is best-effort.

## Rationale

Playground sessions self-destruct on idle (no substantive activity for
`JAMSESH_PLAYGROUND_IDLE_TIMEOUT_S`). Without explicit resets, the
`playground.Worker` sweep would mistakenly destroy sessions where
participants are actively collaborating, just not pushing commits. The
pattern co-locates the reset with the action that proves activity, with a
strict definition of "substantive" — presence pings, page loads, and other
ambient signals do NOT call this. The dual guard (`org_id == playgroundOrgID`
AND `IdleTimeout > 0`) means durable sessions pay only a string-comparison
cost and the reset is fully disabled if config didn't wire the timeout.

## Examples

### Example 1: comment creation

**File**: `internal/portal/comments/service.go:214`

```go
// Activity-reset for playground sessions (best-effort). A comment posted
// by any participant constitutes substantive collaboration and resets the
// session's idle timer.
if p.OrgID == playgroundOrgID && s.PlaygroundIdleTimeout > 0 {
    resetAt := s.now()
    if resetErr := s.Store.ResetSessionIdleTimer(ctx, store.ResetSessionIdleTimerParams{
        OrgID:                     p.OrgID,
        SessionID:                 p.SessionID,
        LastSubstantiveActivityAt: resetAt,
        IdleTimeoutAt:             resetAt.Add(s.PlaygroundIdleTimeout),
    }); resetErr != nil {
        slog.WarnContext(ctx, "comments: reset idle timer failed (best-effort)",
            "org", p.OrgID, "session", p.SessionID, "err", resetErr)
    }
}
```

### Example 2: finalize-attempt

**File**: `internal/portal/sessions/handler.go:323`

```go
if orgID == playgroundOrgID && h.playgroundIdleTimeout > 0 {
    now := h.clock.Now().UTC()
    if resetErr := h.store.ResetSessionIdleTimer(ctx, store.ResetSessionIdleTimerParams{
        OrgID:                     orgID,
        SessionID:                 sessionID,
        LastSubstantiveActivityAt: now,
        IdleTimeoutAt:             now.Add(h.playgroundIdleTimeout),
    }); resetErr != nil {
        slog.WarnContext(ctx, "sessions: finalize: reset idle timer failed (best-effort)",
            "org", orgID, "session", sessionID, "err", resetErr)
    }
}
```

### Example 3: post-receive (successful git push)

**File**: `internal/portal/githttp/receive_pack.go:336`

```go
// This runs AFTER a successful push (subprocess exit 0 + storage sync),
// so the activity timestamp reflects genuinely committed work.
if orgID == playgroundOrgID && h.PlaygroundIdleTimeout > 0 {
    now := time.Now().UTC()
    resetErr := h.Store.ResetSessionIdleTimer(r.Context(), store.ResetSessionIdleTimerParams{
        OrgID:                     orgID,
        SessionID:                 sessionID,
        LastSubstantiveActivityAt: now,
        IdleTimeoutAt:             now.Add(h.PlaygroundIdleTimeout),
    })
    if resetErr != nil {
        slog.WarnContext(r.Context(), "receive-pack: reset idle timer failed (best-effort)",
            "org", orgID, "session", sessionID, "err", resetErr)
    }
}
```

3 occurrences, one per "substantive action" surface:
`comments/service.go:214`, `sessions/handler.go:323`,
`githttp/receive_pack.go:336`.

## When to Use

- A new write surface lands on a session, and the product decision is that
  this action proves the participant is still actively collaborating (not
  just present).
- The action's primary mutation is already committed; the timer reset is
  post-success and best-effort.

## When NOT to Use

- Read-only surfaces (GET endpoints, presence pings, WS subscription joins).
  These are not substantive collaboration.
- Internal/automatic mutations not driven by a participant (auto-merge
  events, finalize-lock heartbeat). The whole point of the reset is
  human-driven proof-of-life.
- Hot paths where the extra DB write would dominate latency.

## Common Violations

- Skipping the `IdleTimeout > 0` half of the guard — the reset DB write
  fires even when the destruction worker is disabled, wasting writes.
- Letting `ResetSessionIdleTimer` failure propagate to the caller — the
  comment / finalize / push has already succeeded; surfacing a 500 because
  the timer reset failed would be wrong.
- Calling the reset BEFORE the primary mutation commits — a failed primary
  action would leave the session looking active when no useful work
  happened.
- Using a different "playgroundOrgID" constant per package — every caller
  defines `const playgroundOrgID = "org_playground"` locally with a comment
  explaining the import-cycle avoidance.
