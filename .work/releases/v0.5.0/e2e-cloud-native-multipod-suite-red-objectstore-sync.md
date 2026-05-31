---
id: e2e-cloud-native-multipod-suite-red-objectstore-sync
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

# Object-storage cross-pod sync / hydration

## Brief
A ref pushed via one pod must be readable — a non-empty SHA — from another pod
before any chaos is applied. Today the chaos prerequisites fail:
`handoff_under_object_storage_chaos_test.go` and `handoff_under_pod_kill_test.go`
report "pod N returned empty SHA for ref `jam/<sid>/<acct>/main`" at the
PREREQUISITE step, before chaos/kill is even injected. The object-storage sync
provider (RPO=0 push to MinIO) or the receiving pod's repo-cache hydration from
object storage is incomplete (or broken) at the moment the second pod is read.

This feature roots-causes and fixes cross-pod base-ref visibility / hydration
timing in the object-storage sync layer so a pushed ref is durably visible
cluster-wide before downstream steps run. Scope is the cloud-native multi-pod
sync path only.

It does NOT cover lease migration, router redispatch/metrics, the scaffolding
clone gate, or playground-specific tests (those are owned by the playground
epics, already done). Per the parent epic's design decisions this is never-green
stabilization — root-cause forward from the current red state, no bisect.

## Epic context
- Parent epic: `e2e-cloud-native-multipod-suite-red`
- Position in epic: independent subsystem fix — parallel with lease, router,
  and fuzz. The cluster-smoke integration gate depends on this feature.

## Foundation references
- `docs/ARCHITECTURE.md` — object-storage sync / RPO=0 component
- Primary package: `internal/portal/storage/objectstore/`
- Representative red tests (feature-design confirms the exact owned set):
  chaos `handoff_under_object_storage_chaos_test.go`,
  `handoff_under_pod_kill_test.go`, `object_storage_partition_test.go`;
  golden `object_storage_rpo0_test.go`, `object_storage_pack_manifest_test.go`

---

## Root cause (verified 2026-05-31)

The "empty SHA at PREREQUISITE" symptom was a **test bug**, not a sync bug.

### 1. PRIMARY (the prerequisite failure) — test-only ref-name mismatch — FIXED
The chaos tests build the *short* push ref `jam/<sid>/<uid>/main`
(`handoff_under_object_storage_chaos_test.go:126`,
`handoff_under_pod_kill_test.go:106`). `gitclient.Push` pushes it as
`HEAD:refs/heads/<ref>` (`tests/e2e/fixtures/gitclient/gitclient.go:161`), so the
stored ref is `refs/heads/jam/...`. But the REST endpoint
`GET /api/orgs/{org}/sessions/{sid}/refs` returns the **full** ref name:
`ListSessionRefs` emits `r.Name().String()` =
`refs/heads/jam/...` (`internal/portal/sessions/state.go:199`). The test helpers
`stosChaosRefTip` / `podKillRefTip` compared the full API value against the short
`jam/...` value (`...chaos_test.go:371`, `...pod_kill_test.go:335`), so they
returned `""` even on the holder pod that had just pushed. The prereq reads from
the same pod it pushed to, so this empty SHA was a pure string mismatch — NOT a
sync/hydration issue. (`RevParse` was unaffected: it uses
`git rev-parse origin/jam/...`, which git resolves locally from
`refs/heads/jam/...`.)

**Confirmed synchronous-write claim:** receive-pack does not 200 until
`Emitter.EmitForUpdates → Syncer.SyncPushPath` completes
(`internal/portal/githttp/receive_pack.go`,
`internal/portal/postreceive/emitter.go`,
`internal/portal/storage/objectstore/sync.go`). RPO=0 on the write path is
genuinely synchronous; only lazy old-pack deletion is async. So the write side is
not the bug.

### 2. SECONDARY (now the BLOCKER) — lease-takeover false "hashtext collision" — OUT OF SCOPE
With the helper fix applied, both tests get PAST the prerequisite and now fail
later, at `WaitForHydration(survivor)`. The survivor pod's git-smart-HTTP
`info/refs` returns **503 dep.object_storage_unavailable**, but the underlying
cause is in the **lease** subsystem, not object storage:

```
WARN  lease: hashtext collision detected; releasing advisory lock
      our_pod_id=<survivor> holding_pod_id=<dead/drained holder>
ERROR ... lease: session lease already held by another pod  (→ 503)
```

`PostgresManager.Acquire` (`internal/portal/lease/postgres.go:95-145`):
1. `pg_try_advisory_lock(hashtext(sessionID))` **succeeds** on the survivor (the
   dead/drained holder's session-scoped advisory lock was released when its
   connection died).
2. The "Step 3 defensive hashtext-collision check" (lines 117-145) then reads the
   stale `leases` row, finds `pod_id = <dead holder>` with `released_at IS NULL`
   (the killed/drained holder never wrote `released_at`), mis-classifies this as
   a hashtext collision, releases the advisory lock, and returns `ErrAlreadyHeld`.

Net effect: a survivor can **never** take over a lease whose previous holder
died/drained without cleanly clearing `released_at`. This is a lease-takeover
product bug owned by the lease feature/epic
(`epic-cloud-native-deploy-lease-fencing` lineage), explicitly out of scope for
this object-storage feature ("It does NOT cover lease migration"). It blocks both
handoff tests downstream of the prerequisite this feature owns. `WaitForHydration`
uses the git-smart-HTTP path, not REST `/refs`, so my feature's surface is not the
blocker here.

### 3. LATENT (filed, not exercised) — REST `/refs` has no cross-pod hydration hook
`ListSessionRefs` (`internal/portal/sessions/state.go:165-173`) opens the
pod-local bare repo directly and returns `200` + empty list if the repo is absent
on that pod — with NO hydration hook. The git-smart-HTTP path hydrates on access
via `acquireForGitRequest → Lifecycle.AcquireForRequest → Hydrator.Hydrate`
(`internal/portal/githttp/handler.go:82-90`,
`internal/portal/storage/objectstore/lifecycle.go:201`). So cross-pod REST `/refs`
is non-deterministic. The two chaos tests do NOT exercise this gap (both read
REST `/refs` only AFTER `WaitForHydration` + a git-smart-HTTP push have already
hydrated the survivor pod), so it is latent. Filed as backlog story
`portal-rest-refs-no-cross-pod-hydration`.

## Fix applied
Test-only (test debt → fixed in-session):
- `tests/e2e/chaos/handoff_under_object_storage_chaos_test.go` — `stosChaosRefTip`
  canonicalizes the expected ref to `refs/heads/<ref>` before comparing against
  the API's full ref name.
- `tests/e2e/chaos/handoff_under_pod_kill_test.go` — `podKillRefTip` does the same
  (added `strings` import).

Call sites unchanged: they still pass the short `jam/...` form, consistent with
`gitclient.Push` and `RevParse`.

## Product-bug disposition
- Lease-takeover false-collision (the current blocker): OUT OF SCOPE — lease
  feature/epic owns it. Reported to the orchestrator for routing; not filed here
  to avoid duplicating a lease-epic item. NOT fixed in this feature.
- REST `/refs` no-hydration gap (latent): FILED as
  `.work/backlog/portal-rest-refs-no-cross-pod-hydration.md`.

## Verification
Command (under shared flock):
```
flock /tmp/jamsesh-autopilot-e2e.lock bash -c 'cd tests/e2e && \
  GOTMPDIR=$HOME/.cache/gotmp TMPDIR=$HOME/.cache/gotmp \
  go test ./chaos/ -run "TestHandoffUnderObjectStorageChaos|TestHandoffUnderPodKill" -count=1 -v'
```
Result: BOTH tests now pass the prerequisite REST `/refs` read (logged
`pre-chaos draft tip on pod 0 = <sha>` / `pre-kill draft tip on pod 0 = <sha>` —
previously these fataled with "empty SHA ... prerequisite failure", proving the
helper fix is correct and necessary). Both then FAIL downstream at
`WaitForHydration(survivor)` with `503 dep.object_storage_unavailable`, root-caused
to the lease-takeover false-collision bug (portal logs show
`lease: hashtext collision detected ... lease already held by another pod`).

Status: the object-storage-sync surface this feature owns is correct (write path
synchronous/RPO=0; REST `/refs` prerequisite read fixed). The two handoff tests
remain RED solely due to the out-of-scope lease-takeover product bug. No further
object-storage work is warranted; this feature is complete pending the lease fix
that unblocks the downstream hydration step.
