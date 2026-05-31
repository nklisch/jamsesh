---
id: lease-holder-killed-eager-vs-request-driven-slo
kind: story
stage: done
tags: [portal, infra, testing]
parent: e2e-cloud-native-multipod-suite-red
depends_on: [e2e-cloud-native-multipod-suite-red-lease-migration]
release_binding: null
gate_origin: null
created: 2026-05-31
updated: 2026-05-31
---

# lease_holder_killed SLO: eager vs request-driven migration

## Brief
`chaos/lease_holder_killed_test.go` SIGKILLs the lease holder, then passively
polls `pg_locks` via `WaitForLeaseMigration` for 30s and asserts the lease
migrated to a survivor within the SLO — WITHOUT sending any request to the
survivor. But lease acquisition is **request-driven**
(`LifecycleManager.AcquireForRequest`): there is no background re-acquisition
loop, so a survivor does not acquire until a request routed to it triggers
acquisition. With the takeover bug fixed, acquisition itself now works, but this
test still fails because nothing triggers it.

## Decision needed (do NOT game the test)
- **Request-driven is the intended architecture** → the test is wrong: it must
  route/replay a request to the survivor (as a real router redispatch would)
  before asserting migration within the SLO. Test-debt fix.
- **Eager background migration is a product requirement** (the feature brief's
  "a surviving pod must re-acquire within the 30s SLO after SIGKILL" read as a
  proactive guarantee) → add a background re-acquisition/failover loop. Product
  feature.

The brief is ambiguous on the trigger. Resolve the architecture question first;
do not change the test's assertion to whatever the code happens to do.

## Decision (2026-05-31, user)
**Request-driven is the intended architecture.** In production the router
redispatches a request to a survivor, which triggers `AcquireForRequest`; there
is no background failover loop and none will be added. Therefore the test is
wrong: fix `lease_holder_killed_test.go` to route/replay a real git request to
the surviving pod (as a router redispatch would) before asserting the lease
migrated within the 30s SLO. This is a test-debt fix — do NOT add an eager
background re-acquisition loop, and do NOT weaken the assertion to whatever the
code happens to do.

## References
- `tests/e2e/chaos/lease_holder_killed_test.go`,
  `tests/e2e/fixtures/portalcluster/` (`WaitForLeaseMigration`),
  `internal/portal/lease/`, `internal/portal/storage/objectstore/lifecycle.go`
  (`LifecycleManager.AcquireForRequest`).

## Resolution (2026-05-31) — test fixed + verified (commit `44f949b2`)
Applied the decided request-driven fix: after SIGKILLing the holder, the test
now routes a git push to the surviving pod (as a router redispatch would) to
trigger `AcquireForRequest`, THEN asserts migration within the 30s SLO.

Verified: lease **migrated pod 1 → pod 0 within the SLO ✓** and the monotonic
fencing-token invariant **T2 (2) > T1 (1) ✓** both now pass — the lease-takeover
fix + this request-trigger work. No background failover loop was added.

**New downstream layer (separate):** the test then performs a git clone via the
router URL and gets a **502** — the router still routes to the killed pod. This
is the long-known `bug-router-static-discoverer-not-started` (released in v0.1.0
but evidently persists, or a missing per-request failover timeout); the
`router_pod_disappears` chaos test already references it. Tracked at the epic
level — NOT this story's surface (this story's request-driven test fix is done).
