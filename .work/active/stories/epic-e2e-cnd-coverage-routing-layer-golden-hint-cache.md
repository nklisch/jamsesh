---
id: epic-e2e-cnd-coverage-routing-layer-golden-hint-cache
kind: story
stage: implementing
tags: [e2e-test, testing, portal, infra]
parent: epic-e2e-cnd-coverage-routing-layer
depends_on: [epic-e2e-cnd-coverage-routing-layer-golden-consistent-hash]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Routing Layer — Golden: Hint-Cache Override

## Scope

Implement `tests/e2e/golden/router_hint_cache_test.go`.

Two subtests:

### 1. `hint_cache_overrides_ring_after_503`

This is the primary invariant: the hint cache is populated on success and
invalidated on 503.

Steps:
1. Start a 2-pod cluster with the router. Identify which pod the ring assigns
   for a chosen `session_id` (call it pod A) by inspecting
   `cluster.LeaseHolder` after one successful request.
2. Simulate pod A holding the lease, pod B not. When pod A returns 503 (lease
   held elsewhere — simulated by the `router_backend_503_helper` described in
   the feature body's Helper section), the router invalidates the hint and
   retries via `Ring.GetNext`, landing on pod B.
3. After the retry, make a subsequent request for the same `session_id`. The
   router now has NO hint for this session (invalidated on 503; not re-set on
   retry per the proxy.go comment "Don't update hint on retry"). So it falls
   back to the ring (pod A again). This is correct behavior — the hint cache
   will repopulate on the next clean success.
4. Assert the hint repopulates: after a clean success (pod A serves it), the
   next request for the same session goes to pod A via hint (not ring).

**Caveat on hint-cache observability**: The hint cache is internal to the
router container and is not directly observable. Verify indirectly:
- The `/metrics` endpoint on the router exposes `router_decisions_total` with
  labels `hit_cache`, `hit_ring`, `retry`, etc.
- Scrape `/metrics` before and after the sequence; assert `hit_cache` counter
  increments after a clean success on the same session.

### 2. `hint_cache_is_per_session`

Start 3-pod cluster. Route requests for 3 distinct session IDs; verify via
LeaseHolder that all 3 route to different pods (or at least 2 distinct ones,
depending on hash placement). Assert that routing each session correctly is
stable — i.e., the hint for session A does not affect routing for session B.
This guards against a blanket-replacement bug where the cache stores the last
pod globally rather than per-session.

## Hint-cache TTL note

The hint cache TTL defaults to 5 minutes (confirmed in `internal/router/cache/
hint.go`; set via `cache.New(10_000, cfg.HintCacheTTL)` in main.go). The TTL
is YAML-only in the current router config — there is no env-var binding for
`HintCacheTTL` (noted in `router.go` Options struct: "HintCacheTTL is YAML-only
… currently a no-op"). Tests that need short TTL should NOT rely on TTL
expiry — instead rely on the 503-driven invalidation path (which is the
tested invariant) or the per-session correctness check (subtest 2). A follow-on
story to add short-TTL support via config YAML mount is filed separately.

## Setup

```go
pg  := postgres.Start(ctx, t, postgres.Options{})
mn  := minio.Start(ctx, t, minio.Options{})
c   := portalcluster.Start(ctx, t, portalcluster.Options{
    Pods:        2,  // or 3 for subtest 2
    Postgres:    pg,
    ObjectStore: mn,
    Router:      true,
})
```

For subtest 1, to trigger a portal 503 from pod A without killing it: see the
`router_backend_503_helper` designed in the feature body. The helper makes pod
A return 503 to the router's next request for a specific session by temporarily
blocking advisory lock acquisition (or by having the test send a request to
that session while holding the lock from a separate Postgres connection).

## Invariant

> After a 503-driven hint invalidation, the cache repopulates on the next clean
> success; per-session hints do not bleed across sessions.

## Assertion targets

- `router_decisions_total{result="hit_cache"}` increments after a clean success
  on a warm session.
- `router_decisions_total{result="hit_ring"}` used only on the first request or
  after invalidation.
- `cluster.LeaseHolder(ctx, t, sessionA)` != `cluster.LeaseHolder(ctx, t, sessionB)`
  for distinct sessions (when hash placement differs).

## Acceptance criteria

- [ ] Subtest `hint_cache_overrides_ring_after_503` passes; metrics show
      `hit_cache` after warm session.
- [ ] Subtest `hint_cache_is_per_session` passes; distinct sessions route to
      their correct pods independently.
- [ ] Metrics scraping via `GET <router.URL>/metrics` used for cache-hit
      observability — no in-process router state inspection.

## Test-integrity rules

- **Park production bugs, don't hide them.** If the metrics show the hint cache
  never populates (`hit_cache` stays 0 across many requests), park the bug.
- **Never game an assertion.** Do not skip the metrics check and pass the test
  on response-status alone — that would make it tautological.
