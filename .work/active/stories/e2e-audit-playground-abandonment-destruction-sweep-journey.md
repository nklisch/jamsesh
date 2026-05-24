---
id: e2e-audit-playground-abandonment-destruction-sweep-journey
kind: story
stage: drafting
tags: [testing, e2e-test, audit, playground]
parent: null
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
