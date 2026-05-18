---
id: epic-e2e-cnd-coverage-hydration-handoff-lifecycle
kind: story
stage: implementing
tags: [e2e-test, testing, portal]
parent: epic-e2e-cnd-coverage-hydration-handoff
depends_on: [epic-e2e-cnd-coverage-hydration-handoff-golden]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Hydration-Handoff Lifecycle — Evict-on-Lease-Release Cache Cleanup

## Scope

One lifecycle test: after a session's lease is released (idle eviction), the
pod's local bare-repo cache for that session is cleared from disk. Verifies
the LifecycleManager's eviction path and ensures the cache respects the
configured idle timeout.

This is the "cleanup" side of the handoff contract — the golden tests verify
that state is preserved after migration; this test verifies that the originating
pod does not retain stale cache after eviction.

## Unit 1: `tests/e2e/golden/lifecycle_evict_on_lease_release_test.go`

```
Package: golden_test
Test: TestLifecycleEvictOnLeaseRelease
```

**Invariant:** "After idle eviction, a pod's local bare-repo cache for a
session is removed from disk. A subsequent request to that pod for the same
session requires re-hydration (not served from stale local cache)."

**Stack:** `postgres.Start` + `minio.Start` + `mailhog.Start` +
`portalcluster.Start(Pods: 2, Router: false)` with accelerated eviction knobs:

```go
PortalExtraEnv: map[string]string{
    "JAMSESH_HYDRATION_IDLE_TIMEOUT_S":      "5",   // evict after 5s idle
    "JAMSESH_HYDRATION_IDLE_CHECK_PERIOD_S": "2",   // scan every 2s
    "JAMSESH_STORAGE_PATH":                  "/var/jamsesh",  // deterministic path
    "JAMSESH_EMAIL_PROVIDER":               "smtp",
    // ... mailhog SMTP vars
},
```

**Setup:**
1. Alice signs in via pod 0, creates org + session.
2. Push 3 commits via pod 0 to establish the cache and lease. Record
   `draftTipBefore`.
3. `RequireLeaseHolder` confirms pod 0.
4. Confirm cache is present: `VerifyCacheEvicted` should return false at
   this point (cache exists). Alternatively, a positive check:
   `VerifyCachePresent(ctx, t, c, 0, sessionID)` — if this helper doesn't
   exist, skip the positive check and just assert the negative (evicted)
   state later.

**Wait for eviction:**
5. `time.Sleep(10 * time.Second)` — two scan periods with 5s timeout.
   The LifecycleManager scans at t=2s, t=4s; the 5s idle threshold is
   exceeded at t=5s from the last push. By t=10s, at least one scan
   period has fired after the idle threshold expired.

**Assertions:**
6. `VerifyCacheEvicted(ctx, t, c, 0, sessionID)` — the directory
   `/var/jamsesh/sessions/<sessionID>/` must be absent or empty in pod 0's
   container. If present, the eviction did not fire — park as a bug.
7. `LeaseHolder(ctx, t, sessionID)` should return -1 (no lock held) since
   the idle eviction path releases the advisory lock alongside the cache
   eviction. If it returns 0 (pod 0 still holds the lock), that is a Medium
   bug (cache evicted but lock not released) — log it but do not fatal; the
   lock will be released when pod 0 is torn down by Testcontainers.
8. **Re-hydration round-trip:** Push a 4th commit via pod 0 directly. This
   forces pod 0 to re-acquire the lease and re-hydrate from MinIO before
   serving the push.
9. `WaitForHydration(ctx, t, c.Pods[0], orgID, sessionID, accessToken, 30*time.Second)`
   — wait for re-hydration to complete.
10. Query the draft tip from pod 0. It must equal or advance past
    `draftTipBefore` (all 3 pre-eviction commits + the 4th commit).

**LRU scope note (in test source as comment):**
```go
// This test covers idle (time-driven) eviction only. LRU eviction
// (memory-pressure-driven via JAMSESH_HYDRATION_CACHE_MAX_BYTES) is not
// tested — container memory is not a reliable test lever. See risks in
// epic-e2e-cnd-coverage-hydration-handoff body.
```

## Acceptance criteria

- [ ] `TestLifecycleEvictOnLeaseRelease` green; `VerifyCacheEvicted` confirms
      pod 0's cache is cleared after the idle timeout
- [ ] Re-hydration round-trip succeeds; draft tip reflects all commits including
      pre-eviction state
- [ ] Advisory lock is released alongside eviction (assert `LeaseHolder == -1`
      after eviction; park if not, but do not block the test)
- [ ] LRU scope note is present in test source
- [ ] No in-process mocks

## Test integrity (from parent feature)

- If `VerifyCacheEvicted` returns "cache still present" after 10s with a 5s
  idle timeout and 2s check period — that is a **High** production bug (the
  LifecycleManager's idle scanner is not running or the env var is not
  honored). Park via `/agile-workflow:park`. Land the test with:
  ```go
  t.Skip("bug-<id>: JAMSESH_HYDRATION_IDLE_TIMEOUT_S not honored — " +
      "cache not evicted after 2× idle timeout period")
  ```
- If the re-hydration succeeds but the draft tip is missing pre-eviction
  commits — that is a **Critical** bug (eviction corrupted the session's
  MinIO state). Park immediately.
- Never game: do not remove the `VerifyCacheEvicted` call because the
  docker exec is flaky. Fix the flakiness (check the path env var, check
  the container exec API) — a green test that skips the eviction check is
  not a lifecycle test.
