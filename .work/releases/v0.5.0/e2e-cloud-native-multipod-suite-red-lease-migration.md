---
id: e2e-cloud-native-multipod-suite-red-lease-migration
kind: feature
stage: done
tags: [portal, infra, testing, bug]
parent: e2e-cloud-native-multipod-suite-red
depends_on: []
release_binding: v0.5.0
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

## Resolution (2026-05-31, Opus implementation agent)

### Confirmed root cause
`PostgresManager.Acquire` (`internal/portal/lease/postgres.go`) Step 3 ran a
false-positive "hashtext collision" check. After Step 2 wins
`pg_try_advisory_lock(hashtext(session_id))` — which proves there is NO live
holder (advisory locks are session-scoped and auto-release on the holder's PG
connection death) — Step 3 read `SELECT pod_id, released_at FROM leases WHERE
session_id = $1` and returned `ErrAlreadyHeld` whenever the row had a different
`pod_id` and `released_at IS NULL`. That `WHERE session_id = $1` filter can NOT
detect a cross-session collision (a real collision is a DIFFERENT session_id
hashing to the same int32), so it only ever fired on a stale dead-holder row.
Result: a survivor could never take over a lease whose holder exited without
clearing `released_at` (SIGKILL / ungraceful drain). Downstream symptom:
`WaitForHydration(survivor)` → `503 dep.object_storage_unavailable`.

The fix is safe because a TRUE cross-session collision is already excluded at
Step 2: two colliding sessions contend on the same advisory-lock key, so the
second `pg_try_advisory_lock` returns false. By the time control reaches the
upsert we hold the lock ⇒ no live holder ⇒ a same-session `released_at IS NULL`
row is necessarily reclaimable stale state. The `InsertLease` upsert
(`ON CONFLICT (session_id) DO UPDATE`) overwrites pod_id/fencing_token and
re-nulls released_at, completing the takeover with a fresh monotonic token.

### Fix (files changed)
- `internal/portal/lease/postgres.go`: removed the false-positive Step 3
  collision branch and its row-read; renumbered the remaining steps; rewrote the
  `Acquire` doc comment and added an inline comment explaining WHY a same-session
  unreleased row at this point is reclaimable. Dropped now-unused `errors` and
  `log/slog` imports.
- `internal/portal/lease/postgres_test.go` (TEST DEBT): the old
  `TestPostgresCollisionDefensiveCheck` set up the exact dead-holder takeover
  state (same session_id, advisory lock free, different pod_id, released_at NULL)
  and asserted `ErrAlreadyHeld` — it ENCODED THE BUG. Replaced it with
  `TestPostgresReclaimsStaleSameSessionRow`, which asserts the corrected
  behavior: a survivor reclaims the stale row, mints a strictly-greater fencing
  token, and the row ends up owned by the survivor with released_at re-nulled.
  The genuine-conflict test (`TestPostgresAcquireConflictReturnsErrAlreadyHeld`,
  live holder still owns the advisory lock) is unchanged and still passes.

### Verification
- `go test ./internal/portal/lease/... -count=1` → PASS (incl. new takeover test
  and the unchanged live-holder conflict test).
- Rebuilt `jamsesh/portal:e2e` via `make test-portal-image` (router image
  untouched).
- e2e (per-test, Docker exclusive):
  - `failure/TestLeaseAlreadyHeld` → PASS (live-holder 503 path intact — fix did
    not weaken the real conflict guard).
  - `chaos/TestHandoffUnderPodKill` → survivor now HYDRATES from MinIO ✓
    (original `503 dep.object_storage_unavailable` symptom GONE), then FAILS on a
    `non-fast-forward` git push (see Out-of-scope below).
  - `chaos/TestHandoffUnderObjectStorageChaos` → same: hydration succeeds, fails
    on the identical `non-fast-forward` push.
  - `failure/TestStaleFencingTokenRejected`, `golden/TestLeaseAcquireAndFence`
    (monotonic_fencing_tokens subtest) → lease kill+force-release+re-acquire
    works; both then fail on the SAME `non-fast-forward` push.
  - `chaos/TestLeaseHolderKilled` → still red, but for a DIFFERENT, structural
    reason (see Out-of-scope).

### Out-of-scope reds observed (route, do not fix here)
1. **Non-fast-forward post-handoff push** (blocks both handoff tests +
   stale-fencing + lease-acquire-and-fence). The lease takeover now succeeds and
   the survivor hydrates, but the e2e harness's `gitclient.Clone` checks out the
   repo's default HEAD branch rather than the per-user `jam/<session>/<user>/main`
   ref that hydration restores; a 6th commit built on that base is behind the
   populated jam ref → `! [rejected] (non-fast-forward)`. This is a
   hydration/default-branch + e2e-gitclient concern
   (`tests/e2e/fixtures/gitclient`, `internal/portal/storage/objectstore/hydrate.go`,
   githttp) — NOT lease, and outside this feature's edit scope.
2. **`TestLeaseHolderKilled` test-design gap.** Lease acquisition is purely
   request-driven (`LifecycleManager.AcquireForRequest`); there is no background
   re-acquisition loop. The test SIGKILLs the holder then passively polls
   `pg_locks` for 30s via `WaitForLeaseMigration` WITHOUT sending the request that
   would trigger the survivor to acquire. It asserts eager migration the
   architecture deliberately does not provide. Fixing it means adding a
   push-to-survivor trigger before the wait — a test-contract change that should
   be a deliberate decision, not an inline edit by the lease-takeover fix.
3. **`failure/TestRouterLeaseUnavailable`** — router redispatch/503 subsystem,
   out of scope.
4. **`golden/TestLifecycleEvictOnLeaseRelease`** — failed only because the portal
   container never passed `/healthz` after 5 start attempts (Docker resource
   pressure during a back-to-back container-heavy run), not a code defect.

Recommend parking (1) and (2) as their own backlog items under the parent epic.
