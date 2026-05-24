---
id: e2e-audit-playground-abandonment-destruction-sweep-journey
kind: story
stage: review
tags: [testing, e2e-test, audit, playground]
parent: feature-e2e-playground-coverage-golden
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Abandonment journey (idle 30m → destruction worker sweeps → tombstone served) has no e2e test

## Severity
High

## Finding type
journey-gap

## Evidence

`internal/portal/playground/worker_test.go` exercises the sweep in seven
tests:
- `TestWorker_SweepDestroysExpiredByHardCap`
- `TestWorker_SweepDestroysExpiredByIdleTimeout`
- `TestWorker_SweepSkipsNonExpiredSessions`
- `TestWorker_GracefulShutdownStopsWithinOneInterval`
- `TestWorker_PurgesTombstonesAfterTTL`
- `TestWorker_ReasonFor_HardCapTakesPriority`
- `TestWorker_RunsEvenWhenCreateDisabled`

All run in-process against a stubStorage. None drives a real
`ticker-sweep-loop` (per `.claude/rules/patterns.md`) against real
Postgres rows with real `idle_timeout_at` timestamps. None verifies that
the `playground-activity-reset` pattern (also documented in patterns.md)
actually fires on a real write — that pattern resets `idle_timeout_at`
inside the playground org-id branch, and getting it wrong silently
extends or truncates session lifetime.

Grep:

```
$ grep -rIn -E "destruction|sweep|hard.cap|idle.timeout" tests/e2e/
(no output)
```

## Why this matters

The destruction worker is the headline safety property of v0.4.0 — "your
anonymous repo will be GONE in N minutes." Production failure modes that
unit tests cannot catch:
- Real Postgres `tx-emit-then-fanout` ordering: if the worker emits the
  destruction event before the row is deleted (or vice versa), downstream
  observers see inconsistent state.
- Real filesystem deletion: `storage.RemoveRepo` against a real bare repo
  on disk (the unit test's `stubStorage.RemoveRepo` is a map delete).
- Real `playground-activity-reset` after a real comment / push / finalize:
  if the reset hook doesn't fire, the worker prematurely destroys an
  active session.
- Real wall-clock idle-timeout math: `time.Now()` vs the
  per-package-clock-interface fallback diverges if one site forgets the
  injection.

## Suggested remedy

Add `tests/e2e/golden/playground_abandonment_destruction_test.go` driving
real time advance via the `/test/clock-advance` hook already used by
`tests/e2e/chaos/runtime_and_clock_test.go`. Configure the worker with a
short sweep interval (e.g. 1s) and short idle timeout (e.g. 30s test-side
configured via env var). Assert:
1. Create anonymous session at T0.
2. Advance portal clock to T0 + 31s.
3. Within 2s of clock advance, sweep fires.
4. `GET /api/playground/sessions/<id>` returns 410 (session ended).
5. `GET /api/playground/sessions/<id>/tombstone` returns 200 with
   `reason=idle_timeout`.
6. Real bare repo on disk inside the portal container is gone — verify
   via `docker exec ls` (same pattern as
   `lifecycle_evict_on_lease_release_test.go > VerifyCacheEvicted`).

Optional companion subtest: write a comment at T0 + 25s, confirm the
`playground-activity-reset` keeps the session alive past T0 + 31s.

## Test sketch

```go
// tests/e2e/golden/playground_abandonment_destruction_test.go
func TestPlayground_AbandonedSessionDestroyed(t *testing.T) {
    ctx := context.Background()
    pg := postgres.Start(ctx, t, postgres.Options{})
    p := portal.Start(ctx, t, portal.Options{
        DBDriver:                 "postgres",
        DBDSN:                    pg.ContainerDSN,
        PlaygroundEnabled:        true,
        PlaygroundIdleTimeout:    "30s",
        PlaygroundSweepInterval:  "1s",
        PlaygroundTombstoneTTL:   "5m",
    })

    sess := createPlayground(t, p.URL)
    repoPath := portalRepoPath("playground", sess.ID)

    // Confirm repo exists on portal disk.
    require.True(t, dockerExecExists(t, p, repoPath))

    portalclock.Advance(t, p, 31*time.Second)
    require.Eventually(t, func() bool {
        resp := getRequest(t, p.URL+"/api/playground/sessions/"+sess.ID, sess.Bearer)
        return resp.StatusCode == 410
    }, 3*time.Second, 100*time.Millisecond)

    tomb := getJSON(t, p.URL+"/api/playground/sessions/"+sess.ID+"/tombstone", "")
    require.Equal(t, "idle_timeout", tomb.Reason)

    // Repo must be GONE on disk.
    require.False(t, dockerExecExists(t, p, repoPath))
}
```

## Implementation notes

Test landed at `tests/e2e/golden/playground_abandonment_destruction_sweep_test.go`
— real-stack test that creates a playground session, calls
`p.AdvanceClock` past the idle-timeout, polls
`GET /api/playground/sessions/{id}/tombstone` for a 200, and asserts
the tombstone payload (`end_reason: idle_timeout`, `members_count: 1`,
`duration_seconds > 0`).

Ships with `t.Skip` linked to
`idea-playground-clock-not-wired-e2etest` — the playground Handler and
destruction Worker in `cmd/portal/main.go` are wired with
`playground.RealClock()` instead of the AdvanceableClock from
testClockProvider, so `POST /test/clock-advance` has zero effect on
playground session expiry checks or destruction-worker sweep decisions.
The test will silently never see the sweep fire until the wiring is
fixed.

**Bug surfaced and parked:**

- `idea-playground-clock-not-wired-e2etest` — `testClockProvider` in
  `cmd/portal/test_clock_advance.go` (only present under `-tags
  e2etest`) has no `playgroundClock()` accessor; `main.go` wires
  `playground.Handler` and `playground.Worker` with
  `playground.RealClock()`. Fix needs:
  1. Add `playgroundClock() playground.Clock` to `testClockProvider`
     that returns a wrapper around the shared `AdvanceableClock`.
  2. In `main.go`, swap `playground.RealClock()` for `tcp.playgroundClock()`
     where `tcp` is the test clock provider (only under build tag).
  3. Add an integration test confirming clock-advance causes the next
     sweep to see expired sessions.

**Anti-tautology discipline (Unit 5 application):**

The test includes a `p.Exec(ctx, []string{"ls", repoPath})` assertion
that the bare repo exists immediately after `POST /api/playground/sessions`,
and a complementary post-destruction assertion that the directory is
gone. Real filesystem, not stub map.

Re-enable with `git grep -n "blocked on idea-playground-clock-not-wired-e2etest"`
when the bug closes.

## Update — blocking bug resolved inline

The `idea-playground-clock-not-wired-e2etest` blocker was resolved as a
single-stride fix at commit `cc55579` (added `playgroundClock()` to
`testClockProvider` and wired both the Handler and Worker via the
nil-check pattern). The `t.Skip` annotation was removed and the test
now passes end-to-end:

- Create playground session → bare repo present on disk
- `p.AdvanceClock(60s)` past idle-timeout
- Tombstone appears within 1-2s with `end_reason=idle, members_count=1,
  duration_seconds=60`
- Post-destruction GET returns 401 (bearer revoked by destruction cascade)
- Bare repo confirmed absent at the on-disk path

Story stays at `stage: review` — the test landed and passes; ready for
the review skill to verify.
