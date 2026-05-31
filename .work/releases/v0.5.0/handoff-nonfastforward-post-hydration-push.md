---
id: handoff-nonfastforward-post-hydration-push
kind: story
stage: done
tags: [portal, infra, testing, bug]
parent: e2e-cloud-native-multipod-suite-red
depends_on: [e2e-cloud-native-multipod-suite-red-lease-migration]
release_binding: v0.5.0
gate_origin: null
created: 2026-05-31
updated: 2026-05-31
---

# Handoff non-fast-forward push after hydration

## Brief
With the lease-takeover fix (`bb370a3c`) landed, survivor pods now hydrate from
MinIO successfully (the `503 dep.object_storage_unavailable` symptom is gone).
The two handoff chaos tests (`TestHandoffUnderPodKill`,
`TestHandoffUnderObjectStorageChaos`) — and `failure/TestStaleFencingTokenRejected`
and `golden/TestLeaseAcquireAndFence` — now fail one layer later, at a
post-handoff git push: `! [rejected] (non-fast-forward)`. This layer was
previously masked by the earlier hydration 503, so it surfaces only now
(classic never-green peel).

Diagnosed (lease agent): the e2e `gitclient.Clone` checks out the repo's default
HEAD branch rather than the per-user `jam/<sid>/<uid>/main` ref that hydration
restores, so a commit built on that base is behind the populated jam ref →
non-fast-forward on push.

## Needs classification (product vs test) — root-cause first
- **Test-harness bug?** `gitclient.Clone`/the handoff helpers should clone and
  track the `jam/<sid>/<uid>/main` ref (or fetch+reset to its tip) before
  committing and pushing.
- **Product bug?** After hydration the served repo's default branch / `HEAD`
  does not point at the restored jam ref, so a clone legitimately gets the wrong
  base — a real cross-pod git-serving defect.

Resolve per the project test-integrity rules; do NOT paper over a real product
defect in the test.

## Scope / references
- Affects: chaos `handoff_under_pod_kill_test.go`,
  `handoff_under_object_storage_chaos_test.go`; failure
  `stale_fencing_token_rejected_test.go`; golden `lease_acquire_and_fence_test.go`.
- Spans: `tests/e2e/fixtures/gitclient/`,
  `internal/portal/storage/objectstore/hydrate.go`, `internal/portal/githttp/`.
- Likely shares surface with the `cluster-smoke` integration gate (which also
  pushes post-handoff) — coordinate.

## Resolution (2026-05-31) — CLASSIFIED AS TEST, fixed + verified (commit `44f949b2`)
Classified as a **test-harness** bug, not product: the product cannot point a
single served `HEAD` at "the" jam ref because each user has their own
`jam/<sid>/<uid>/main` ref — so the CLIENT must check out its user ref, exactly
as the production CLI does (`cmd/jamsesh/sessioncmd/join.go`:
`git checkout -b jam/<sid>/<uid>/main <fromRef>`). The e2e `gitclient.Clone`
fixture didn't mirror that, so commits landed on the unborn default branch (a
disconnected root → non-fast-forward).

Fix: `gitclient.Clone` now checks out the existing `jam/<sid>/<uid>/main` ref
after cloning (no-op on first clone); the same checkout was added to
`stale_fencing`'s push helpers (without it git rejects the push *before* the
server's fencing logic runs — a false positive that masks the real assertion).

Verified: `stale_fencing` → **PASS**. The non-fast-forward error is GONE from the
handoff tests (survivors hydrate ✓, the push gets past the base-ref problem).

**New downstream layer (separate, filed):** with non-fast-forward gone, both
handoff tests now fail on a githttp `send-pack: unexpected disconnect while
reading sideband packet / remote end hung up` when pushing to the freshly-
hydrated survivor — a product githttp/receive-pack issue (relates to the
released `bug-receive-pack-report-status-sideband-wrapping`). And
`lease_acquire_and_fence` failed only on a portal container cold-start infra
flake (5×60s start retries exhausted), not code. See the epic body.
