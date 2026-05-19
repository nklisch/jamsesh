---
id: bug-e2e-suite-systemic-failures-post-v0.1.0
created: 2026-05-18
tags: [bug, testing, e2e-test]
---

The `e2e` GitHub workflow has been failing red on every push since v0.1.0
shipped (workflow run `26070530857` is representative). Multiple distinct
failure modes across `tests/e2e/chaos`, `tests/e2e/scaffolding`, and
`tests/e2e/failure-mode`. Need systematic investigation — this is not a
single-stride bug.

Observed symptoms from one run:

1. **Router 503 "no backends available"** across multiple chaos tests:
   - `TestCrossPodClockSkew` — `createLeaseSkewSession: want 201; got 503`
     (`cross_pod_clock_skew_test.go:99`)
   - `TestHandoffUnderObjectStorageChaos` — same shape
     (`handoff_under_object_storage_chaos_test.go`)
   - `TestHandoffUnderPodKill` — `podKillCreateSession: want 201; got 503`
     (`handoff_under_pod_kill_test.go:98`)
   - `TestLeaseHolderKilled` — `createLeaseKillSession: want 201; got 503`
     (`lease_holder_killed_test.go:92`)
   - `TestRouterPodDisappears` (multiple sub-cases) — `expected: 201, actual:
     503` (`router_pod_disappears_test.go:104, 214, 441`)

   Looks like the router-backend wiring isn't fully up by the time the test
   posts. Could be a fixture timing race, a health-check threshold change,
   or a regression in the router's "available backend" computation (likely
   related to `staticDiscoverer`/`hint-cache` work that landed around v0.1.0).

2. **WebSocket 401 on upgrade**:
   - `TestRuntimeAndClock/automerger_pause` — `wsclient: dial
     ws://...: failed to WebSocket dial: expected handshake response status
     code 101 but got 401` (`runtime_and_clock_test.go:36`)

   Auth-ticket flow regression — the WS upgrade is rejecting bearer/ticket
   auth. Related to `gate-security-ws-bearer-token-ticket-flow` work?

3. **Rate-limit leakage between subtests**:
   - `TestInterruptedOps/magic_link_ttl_expiry` —
     `POST /api/auth/magic-link/request: status 429 (want 204): "rate_limited"`
     (`interrupted_ops_test.go:277`)

   The auth rate limiter isn't being reset/isolated between subtests in the
   same parent test. The rate-limit window leaks across cases.

4. **Test scaffolding failure** — `FAIL jamsesh/tests/e2e/scaffolding`
   (8.435s). MinIO testcontainers boots with a warning about default
   credentials but appears to come up — root failure inside scaffolding not
   yet localized.

Investigation approach (one stride per cluster):
- Cluster A: "no backends available" 503s — router-backend startup race or
  hint-cache wiring regression.
- Cluster B: WS 401 — ticket-flow auth.
- Cluster C: Rate-limit isolation in `TestInterruptedOps`.
- Cluster D: Scaffolding base failure (may be a precondition for A).

Each cluster likely becomes its own bug story once root-caused. Park here
until someone has a session to triage.
