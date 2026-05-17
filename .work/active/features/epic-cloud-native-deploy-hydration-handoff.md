---
id: epic-cloud-native-deploy-hydration-handoff
kind: feature
stage: drafting
tags: [portal]
parent: epic-cloud-native-deploy
depends_on: [epic-cloud-native-deploy-object-storage-sync, epic-cloud-native-deploy-lease-fencing, epic-cloud-native-deploy-routing-layer]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Cloud-Native Deploy — Hydration + Handoff

## Epic context

- Parent epic: `epic-cloud-native-deploy`
- Position in epic: phase-2 capstone. Closes the loop on clustered
  mode by making sessions migratable between pods on demand. Depends
  on lease-fencing (lifecycle hooks), object-storage-sync (the source
  to hydrate from), and routing-layer (the trigger surface — handoff
  is observable to clients as a brief 503 + re-dispatch).

## Foundation references

- `docs/ARCHITECTURE.md` — "Data flow: a turn" (request lifecycle
  gains hydration-on-first-request in clustered mode).
- `docs/SPEC.md` — "Recovery" section (handoff is a new failure-and-
  recovery mode the spec needs to acknowledge when this lands).
- `docs/PRINCIPLES.md` — "Recovery is `git fetch`" (justifies the
  best-effort handoff: clients re-fetch and re-sync on the next push,
  so a few-seconds-stale hydration is acceptable).
- `internal/portal/storage/service.go` — the local-disk write surface
  hydration fills before serving.

## Brief

The lifecycle glue that makes the clustered-mode topology actually work
under churn. When a pod acquires a session lease for the first time
(cold start, scale-up, ring rebalance after a peer dies), it hydrates
the session's bare repo from object storage into local disk before
serving any request. When a pod loses a lease (shutdown, idle timeout,
lease-loss event), it drains in-flight uploads and evicts local cache.

This feature closes the loop on the clustered architecture. Without it,
a session is bound to a single pod for its entire lifetime, which makes
scale events and pod loss painful. With it, sessions migrate cleanly
between pods on demand.

Single-instance mode has no concept of handoff — skipped.

## Scope

In:
- Hydration on lease acquisition:
  - Read the session's manifest object from object storage.
  - Stream the listed pack files and loose objects into local
    `<storage>/orgs/<org-id>/sessions/<session-id>.git/`.
  - Write `refs/` and `packed-refs` from the manifest.
  - Verify git repo integrity (`git fsck --no-dangling`) before
    serving.
  - Concurrent downloads with bounded parallelism.
- Hydration metrics: time-to-first-serve after lease acquisition, bytes
  downloaded, objects fetched.
- Eviction on lease loss / release:
  - Wait for in-flight upload queue (from `object-storage-sync`) to
    drain or hit a hard timeout.
  - `rm -rf` the local bare repo path. Local disk is cache only;
    object storage holds truth.
  - Optional retention window: keep the local copy for N minutes
    after eviction in case the same pod re-acquires (configurable,
    default 0 — immediate eviction).
- Idle eviction loop: a background goroutine that releases leases
  for sessions with no activity for the idle window (default 5
  minutes), triggering the eviction path.
- LRU cache size cap: if local-disk usage exceeds a configured
  threshold (`JAMSESH_CACHE_MAX_BYTES`), evict the least-recently-
  active session even if its idle timer hasn't fired.
- Graceful shutdown integration: on `SIGTERM`, release all leases
  (which triggers eviction for each) before the process exits. Uses
  the grace window from `epic-cloud-native-deploy-operational-polish`.

Out:
- Lazy / on-demand object fetch via a custom go-git Storer. v1
  hydrates the whole repo eagerly. Lazy fetch is a future
  optimization worth doing if hydration latency becomes the bottleneck
  for large sessions.
- Predictive pre-hydration ("router hints pod B that session X is
  likely to move there soon"). Cute idea; out of scope for v1.
- Active warm-pool of pre-hydrated sessions. v1 hydrates on demand.

## Design decisions

Inherited from epic. Feature-local:

- **Eager full-repo hydration in v1.** Lazy / per-object fetch is
  elegant but adds a layer of complexity (custom go-git Storer + fault
  handling for failed fetches mid-operation) that we don't need until
  proven necessary. Per-session bare repos are bounded (20–50 MB
  typical per `docs/SELF_HOST.md` §7), so full hydration takes well
  under a second within-region.
- **Eviction is immediate by default.** Local disk is cache; treating
  it as cheap means scale events don't accumulate stale per-pod state.
  Operators who want stickier behavior can tune the retention window.
- **`git fsck` on hydration completion.** Adds a few hundred ms but
  catches corruption before clients see it. Worth it.
- **503 + `Retry-After` is the handoff client contract.** A pod that
  receives a request for a session it doesn't lease (and hasn't
  hydrated) returns 503 with a `Retry-After` header sized to typical
  hydration time; the routing service (which already retries on 503)
  re-dispatches transparently. Clients see at most a brief retry
  loop, no error surface.

## Foundation-doc impact

- `docs/ARCHITECTURE.md` — request-lifecycle section gains the
  hydration-on-first-request step when this lands.

## Notes for design

The handoff race-window is the operational concern: client connects,
router picks pod B, pod B has no local copy, must hydrate before
serving. Worst case is the cold-start latency on the client's first
request. Mitigations:
- Client-side: the `jamsesh` binary's `post-tool-use` hook does
  `git push` which is naturally retry-tolerant; a slow first push is
  benign.
- Server-side: serve a 503 with `Retry-After` while hydrating; the
  router (which already retries on 503) handles re-dispatch
  transparently.
- Long-tail: huge sessions (rare per the 20–50 MB sizing) may take
  multiple seconds. Document the cold-start cost in SELF_HOST.

Eviction-while-uploads-in-flight is a real edge case. The upload queue
from `object-storage-sync` must be drained before `rm -rf` to avoid
orphaned in-memory state pointing at deleted files. The lease handle's
`Release()` flow needs an explicit `drain-uploads → release-lock →
evict-disk` ordering. Resolve in design.

`git fsck` is fast on healthy repos but pathologically slow on large /
broken ones. Consider `git fsck --quick` or a custom go-git
verification pass; resolve in design.
