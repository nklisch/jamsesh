---
id: handoff-nonfastforward-post-hydration-push
kind: story
stage: drafting
tags: [portal, infra, testing, bug]
parent: e2e-cloud-native-multipod-suite-red
depends_on: [e2e-cloud-native-multipod-suite-red-lease-migration]
release_binding: null
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
