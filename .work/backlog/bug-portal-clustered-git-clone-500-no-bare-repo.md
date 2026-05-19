---
id: bug-portal-clustered-git-clone-500-no-bare-repo
created: 2026-05-18
tags: [bug, portal, clustered, git]
---

In clustered-mode (`JAMSESH_DEPLOY_MODE=clustered`), a freshly-created
session returns HTTP 500 on the first git operation (clone OR push)
routed to any pod that didn't handle the session-create API call. The
500 surfaces in multiple e2e tests once the prior readyz-timeout race
was fixed — see workflow run 26078389486 on commit 41118d3.

Repro: portalcluster with 2 pods + postgres + minio + router.
1. POST /api/orgs/{oid}/sessions to pod A (or via router) — returns 201.
2. Wait. Router consistent-hashes a `git clone http://router/git/{oid}/{sid}.git`
   request to pod X.
3. If X == A: clone works (bare repo exists locally on A).
4. If X == B (the pod that didn't create the session): pod B returns
   HTTP 500. The bare repo doesn't exist on B's local disk. Object-storage
   hydration is not triggered on git-upload-pack.

Failing tests (representative — every clustered test that clones before
the lease is acquired hits this):
- `TestCrossPodClockSkew` (chaos)
- `TestHandoffUnderPodKill` (chaos)
- `TestLeaseHolderKilled` (chaos)
- `TestRouterBackendDead/dead_pod_removed_from_routing_pool` (failure)
- `TestRouterLeaseUnavailable/transparent_redispatch_on_503` (failure)
- `TestRouterLeaseUnavailable/bounded_retry_pathology_surfaces_503` (failure)
- `TestStaleFencingTokenRejected` (failure)
- `TestFencingTokenFuzz/*` and `TestPackManifestFuzz/*` (fuzz)
- `TestLeaseAlreadyHeld` (failure) — same shape; the test acquires the
  lock side-band, then `git push` returns 500 instead of the expected 503.

Probable root cause (needs verification): in clustered mode, session
creation must ensure either:
- All pods can hydrate the bare repo from object storage on first git
  request (hydration triggered by git-upload-pack/git-receive-pack), OR
- The session-create flow synchronously mirrors the bare-repo init to
  object storage (it might already do this for push events but not for
  fresh-session create — empty repo case missed?), OR
- The router consistent-hashes to the lease-holder pod and waits for
  lease acquisition before routing the git request (currently the
  router routes by session-id hash without lease awareness).

Fix candidates:
1. **Hydrate on git-upload-pack / receive-pack entry**: when the
   smart-HTTP handler can't find the local bare repo, attempt
   `objectstore.Hydrate(sessionID)` before serving. Cleanest match
   for clustered semantics.
2. **Mirror empty bare-repo at session create**: the session-create
   handler should `git init --bare` AND mirror the empty repo manifest
   to object storage. Reads later trigger hydration.
3. **Lease-aware routing**: router pre-acquires (or queries) the
   session's lease holder and forwards git ops there. Likely an
   architectural shift; defer.

(1) is the lowest-risk fix; (2) is operator-friendly because the
empty repo is observably present from any pod immediately.

Probe points:
- `internal/portal/sessions/sessions.go` — session create handler.
  Where does the bare repo get init'd? Is the mirror to object storage
  enqueued?
- `internal/portal/storage/repo.go` — bare-repo init logic.
- `internal/portal/storage/objectstore/hydrate.go` — hydration entry
  points; is there an "hydrate-on-miss" trigger from the git handler?
- `internal/portal/router/router.go` — does the git mount path call
  through the same lease-aware path as the API?

Single-mode (`JAMSESH_DEPLOY_MODE=single`) is unaffected — there's only
one pod, so the bare repo is always local. The bug only surfaces when
two or more pods share state via object storage and the router routes
requests by hash rather than lease.

Acceptance hint (when this gets scoped): every chaos and failure-mode
test in the workflow run above should pass without the symptomatic 500
on first git op.
