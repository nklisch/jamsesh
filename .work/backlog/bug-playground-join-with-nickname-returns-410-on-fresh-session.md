---
id: bug-playground-join-with-nickname-returns-410-on-fresh-session
kind: bug
stage: drafting
tags: [bug, portal, playground]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# `TestJoinPlaygroundSession_WithNickname_UsesIt` fails — join returns 410 on a freshly created session

## Repro

```
go test ./internal/portal/playground/ -run 'TestJoinPlaygroundSession_(Success|WithNickname_UsesIt)'
```

Two failing tests share the same underlying defect:

```
--- TestJoinPlaygroundSession_WithNickname_UsesIt
    handler_test.go:553: join: want 200, got 410
    handler_test.go:558: want nickname=custom-nick, got ""

--- TestJoinPlaygroundSession_Success
    handler_test.go:537: want non-empty nickname
    handler_test.go:545: want members_count=2, got 0
```

Both indicate the join path is returning early without doing the join work
(nickname empty, member count not incremented). The 410 in the
WithNickname variant points to a session-ended check firing on a
just-created session.

## Observation

The test creates a fresh playground session, then attempts to join it with a
custom nickname. The join returns `410 session_ended` instead of `200`. The
session is brand new — `hard_cap_at` and `idle_timeout_at` should both be in
the future. Yet the join handler is treating it as ended.

Pre-existing on `main` as of commit `5de1588` (gate-patterns), independent of
the v0.4.0 gate-cruft work. Surfaced during gate-cruft verification.

## Likely areas

- `internal/portal/playground/handler.go` JoinPlaygroundSession — the
  ended-check that returns 410.
- Possible clock-injection mismatch between handler and test (the handler
  may use `time.Now()` while the test uses `fixedClock` in a window where the
  session's `idle_timeout_at` is in the past relative to wall time but the
  future relative to the injected clock).
- Or the join handler is comparing against `idle_timeout_at` without the
  guard that the session is still in the active status.

## Why parked, not fixed inline

Per project test-integrity discipline (`CLAUDE.md`): real production bugs
surfaced during a test pass are parked, not silently fixed mid-pass. The
v0.4.0 gate-cruft run is mid-flight; this bug pre-dates the run and deserves
its own design + fix cycle.

## Severity

The test failure points to a real defect in the join handler (or a real
defect in how the handler treats freshly-created sessions). Joiners may be
locked out of just-created sessions in some clock-skew window.

Should be scoped + fixed in a dedicated story before v0.4.0 ships, if
reproducible against the wall clock in production. Promote to gate-tests-style
release-bound item if confirmed prod-affecting.
