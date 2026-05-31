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
updated: 2026-05-31
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

## Pre-diagnosis (from the objectstore-sync agent — pending Codex validation)

While fixing `objectstore-sync`, the agent traced the remaining red in BOTH chaos
handoff tests (`handoff_under_object_storage_chaos_test.go`,
`handoff_under_pod_kill_test.go`) to a **lease-takeover false "hashtext
collision"** in `internal/portal/lease/postgres.go` (~lines 95-145):

1. After the holder pod dies/drains, a survivor calls
   `pg_try_advisory_lock(hashtext(sessionID))` and **succeeds** — the dead
   holder's session-scoped advisory lock was auto-released when its Postgres
   connection died (so advisory-lock auto-release itself works).
2. The "Step 3 defensive hashtext-collision check" (lines 117-145) then reads the
   stale `leases` row, finds `pod_id = <dead/drained holder>` with
   `released_at IS NULL` (the killed/drained holder never wrote `released_at`),
   **misclassifies this as a hashtext collision**, releases the advisory lock,
   and returns `ErrAlreadyHeld`.

Net effect: a survivor can **never** take over a lease whose previous holder
exited without cleanly clearing `released_at`. Symptom downstream:
`WaitForHydration(survivor)` → `503 dep.object_storage_unavailable`; portal logs
`lease: hashtext collision detected; releasing advisory lock`.

Likely fix DIRECTION (to validate): the collision check must distinguish a true
hashtext collision (a DIFFERENT session colliding on `hashtext`) from a
stale-but-same-session row — e.g. compare `session_id`, and treat a
`released_at IS NULL` row for the SAME session whose advisory lock we just
acquired as a reclaimable stale lease (take it over) rather than a collision.

**Cross-feature note:** the two chaos handoff tests are jointly owned —
`objectstore-sync` fixed their REST `/refs` prerequisite, but they cannot go
GREEN until this lease-takeover fix lands. Re-verify those two tests as part of
this feature's acceptance.
