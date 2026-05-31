---
id: lease-holder-killed-eager-vs-request-driven-slo
kind: story
stage: drafting
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

## References
- `tests/e2e/chaos/lease_holder_killed_test.go`,
  `tests/e2e/fixtures/portalcluster/` (`WaitForLeaseMigration`),
  `internal/portal/lease/`, `internal/portal/storage/objectstore/lifecycle.go`
  (`LifecycleManager.AcquireForRequest`).
