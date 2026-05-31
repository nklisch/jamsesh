---
id: e2e-cloud-native-multipod-suite-red-lease-migration
kind: feature
stage: drafting
tags: [portal, infra, testing, bug]
parent: e2e-cloud-native-multipod-suite-red
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# Lease migration on Postgres connection drop

## Brief
When a lease-holding pod is SIGKILLed, its Postgres advisory lock must
auto-release on connection drop and a surviving pod must re-acquire within the
30s SLO. Today `lease_holder_killed_test.go` fails: "lease did not migrate from
pod X within 30s SLO after SIGKILL" — pointing at advisory-lock auto-release on
the Postgres connection drop and the re-acquisition path in
`PostgresManager.Acquire`. The golden `lifecycle_evict_on_lease_release_test.go`
exercises the related evict-on-release path.

This feature roots-causes and fixes the advisory-lock auto-release plus the
re-acquisition / takeover path in `internal/portal/lease/` so lease migration
meets the SLO after an abrupt holder loss.

It does NOT cover object-storage sync, router redispatch/metrics, or the
scaffolding clone gate. Per the parent epic's design decisions this is
never-green stabilization — root-cause forward, no bisect.

## Epic context
- Parent epic: `e2e-cloud-native-multipod-suite-red`
- Position in epic: independent subsystem fix — parallel with objectstore,
  router, and fuzz. The cluster-smoke integration gate depends on this feature.

## Foundation references
- `docs/ARCHITECTURE.md` — lease / advisory-lock component
- Primary package: `internal/portal/lease/` (`postgres.go`, `PostgresManager.Acquire`)
- Representative red tests (feature-design confirms the exact owned set):
  chaos `lease_holder_killed_test.go`;
  golden `lifecycle_evict_on_lease_release_test.go`, `lease_acquire_and_fence_test.go`;
  failure `lease_already_held_test.go`, `stale_fencing_token_rejected_test.go`
