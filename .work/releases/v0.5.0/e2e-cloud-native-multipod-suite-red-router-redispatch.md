---
id: e2e-cloud-native-multipod-suite-red-router-redispatch
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

# Router redispatch and metrics

## Brief
Two router defects keep failure/golden router tests red:

1. **Transparent redispatch on 503** — `router_lease_unavailable_test.go`
   (transparent_redispatch_on_503, bounded retry → 503) does not redispatch to a
   healthy backend / exhaust the bounded retry correctly when a backend reports
   lease-unavailable.
2. **Metric counters** — `router_consistent_hash_test.go` asserts a metric
   counter and gets "-1 not >= 0", and `router_hint_cache_test.go` checks a
   routing-decisions counter that is wrong.

This feature roots-causes and fixes the redispatch path and the router metric
counters across `internal/portal/router/`, `internal/router/`, and
`cmd/jamsesh-router`.

It does NOT cover object-storage sync, lease migration, or the scaffolding clone
gate. Note the golden suite's prometheus parse panic was already fixed in
`ed32b562`; the remaining router failures are real counter/redispatch defects.
Per the parent epic's design decisions this is never-green stabilization —
root-cause forward, no bisect.

## Epic context
- Parent epic: `e2e-cloud-native-multipod-suite-red`
- Position in epic: independent subsystem fix — parallel with objectstore,
  lease, and fuzz. The cluster-smoke integration gate depends on this feature.

## Foundation references
- `docs/ARCHITECTURE.md` — router (consistent-hash ring, hint cache) component
- Primary packages: `internal/portal/router/`, `internal/router/`, `cmd/jamsesh-router`
- Representative red tests (feature-design confirms the exact owned set):
  failure `router_lease_unavailable_test.go`, `router_backend_dead_test.go`;
  golden `router_consistent_hash_test.go`, `router_hint_cache_test.go`

## Root cause per test

1. **`router_lease_unavailable_test.go` — `transparent_redispatch_on_503`**
   - *Real product bug found and fixed.* The proxy's 503-retry path
     (`internal/router/proxy/proxy.go`) streamed the first attempt's response
     straight to the client's `ResponseWriter`. When the chosen pod returned
     503, the 503 status line + body were already flushed, so the retry's
     response could never reach the client — the client always saw the leaked
     503. The unit test that "covered" this (`Test503RetrySucceeds`) passed only
     because `httptest.ResponseRecorder.WriteHeader` ignores the second call and
     the ring happened to pick the 200 pod first, so the leak path was never
     exercised.
   - *Test debt also present.* Two test-harness defects blocked this from even
     running its assertions, and a deeper premise flaw makes it unable to
     exercise re-dispatch:
     - SQL: `SELECT pg_advisory_lock(hashtext($1)::oid)` errors with
       `function pg_advisory_lock(oid) does not exist (42883)` — there is no
       `oid` overload; the portal uses the 64-bit `pg_advisory_lock(bigint)`
       form via `hashtext($1)`. Fixed (dropped the `::oid` cast on the lock arg).
     - Premise: the test drives `GET /api/orgs/{org}/sessions/{id}`, a pure
       Postgres read (`sessions.GetSession`) that never acquires or contends on
       the per-session advisory lease and never 503s on contention. So holding
       the lock cannot induce the 503 re-dispatch needs — the read is always 200
       and re-dispatch is never exercised (tautological pass). Skipped with a
       backlog link (`idea-router-e2e-lease-premise`).

2. **`router_lease_unavailable_test.go` — `bounded_retry_pathology_surfaces_503`**
   - *Test debt (invalid premise).* Same lease-premise flaw: a read GET can
     never return 503 under advisory-lock contention, so the all-pods-503
     condition this subtest asserts is unreachable (empirically returned 200
     after the SQL fix). The router's bounded-retry (exactly one retry, then
     surface 503) is correct and is covered deterministically by the strengthened
     unit test `Test503BothPodsPropagate` (asserts exactly two pod attempts).
     Skipped with a backlog link.

3. **`router_consistent_hash_test.go` (both subtests) — "-1 not >= 0"**
   - *Test debt (invalid premise).* The `-1` is `cluster.LeaseHolder` returning
     "no advisory-lock holder", asserted via `GreaterOrEqualf(holder, 0)`. The
     REST GETs the test drives never acquire the per-session advisory lock (the
     lease is held only by the git/objectstore `LifecycleManager`, wired into the
     git handler in `cmd/portal/main.go`), so `pg_locks` is empty for the session
     and `LeaseHolder` is always `-1` regardless of routing correctness. The
     consistent-hash ring + hint cache product code is correct and unit-tested.
     Skipped with a backlog link.

4. **`router_hint_cache_test.go`**
   - `hint_cache_overrides_ring_after_503`: *Test debt (two defects).*
     (a) The `requireRouterDecisionsCounter` pre-flight scrapes
     `jamsesh_router_decisions_total` before any proxy traffic; a
     zero-cardinality Prometheus `CounterVec` exports no series until its first
     increment, so the metric reads as "absent" and the subtest skips even when
     the router is healthy. Verified empirically: the metric IS registered and
     DOES appear after the first routed request (the product code is correct).
     (b) The 503-forcing step relies on the same lease premise. Skipped (link).
   - `hint_cache_is_per_session`: *Test debt.* Same lease-premise flaw via
     `RequireLeaseHolder` (times out → `-1`). Skipped (link).

5. **`router_backend_dead_test.go`** — *Test debt.* Relies on
   `RequireLeaseHolder` / `WaitForLeaseMigration` (lease premise) and also tripped
   a 3-pod cold-start flake on the shared Docker host. The package-level comment
   claiming the static discoverer's `Run` loop is never started is **stale** —
   `cmd/jamsesh-router/main.go` DOES start it
   (`go disc.Run(ctx, publishWithMetrics(r.SetPods))`); the eviction product code
   in `internal/router/discovery/static.go` is correct and unit-tested. Skipped
   with a backlog link; corrected the stale comment.

## Product vs test split

- **Product fix (1, in `internal/router/proxy/proxy.go`):** the re-dispatch
  response-leak. Replaced the streaming `statusCapture` with a `bufferedResponse`
  that buffers the first attempt (status/headers/body) so a 503 can be discarded
  and the request transparently re-dispatched to a distinct pod, committing only
  the response the client should see. Hijack/Flush delegate to the real writer so
  WebSocket upgrades and streaming still work (once hijacked/flushed the response
  is "committed" and never retried). Bounded to exactly one retry.
- **Test-debt fixes (in-session):** dropped the invalid `::oid` cast on the
  `pg_advisory_lock`/`pg_advisory_unlock` arguments (matches the portal's bigint
  key); strengthened the proxy unit tests to deterministically exercise the
  re-dispatch leak and the bounded-retry exhaustion; corrected the stale
  static-discoverer comment.
- **Parked (too large / overlaps out-of-scope git path):** the systemic
  lease-premise mismatch — the router e2e suite assumes REST requests acquire the
  per-session lease, but only git/objectstore operations do. Parked as
  `idea-router-e2e-lease-premise`. The affected lease-anchored e2e
  tests/subtests are skipped with explicit backlog links rather than
  fake-passed or left as misleading reds (per test-integrity rules: a skip linked
  to a backlog id is more honest than a green that lies).

## Verification

- `go test ./internal/portal/router/... ./internal/router/... ./internal/portal/metrics/... -count=1` — PASS.
- New/strengthened proxy unit tests (`Test503RetrySucceeds` pins the first
  attempt to the 503 pod and asserts the client sees the retry's 200 with both
  pods hit exactly once; `Test503BothPodsPropagate` asserts exactly two pod
  attempts) — PASS, and would fail against the pre-fix streaming proxy.
- Empirically confirmed `jamsesh_router_decisions_total` is absent before traffic
  and present (`{result="hit_ring"} 1`) after the first routed request.
- e2e (router image rebuilt via `make test-router-image`): after the SQL fix,
  `transparent_redispatch_on_503` returned 200 and `bounded_retry` returned 200
  (proving the read-GET premise is invalid); all four router tests now SKIP
  cleanly with backlog-linked reasons (`go test ./golden/ -run
  'TestRouterConsistentHash|TestRouterHintCache'` and `go test ./failure/ -run
  'TestRouterLeaseUnavailable|TestRouterBackendDead'`) — no false greens, no
  misleading reds.

## Out-of-scope reds observed

- `router_backend_dead_test.go` tripped a 3-pod portal cold-start flake
  (`pod 0 is nil after startup`) on the shared Docker host (noted in the epic; a
  start-retry exists in the portal fixture).
- The lease-premise re-anchoring overlaps the separately-red cross-pod
  git-serving path (scaffolding `cluster_smoke` git clone exits 128) — out of
  scope here, tracked under the parent epic.
