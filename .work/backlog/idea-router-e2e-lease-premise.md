---
id: idea-router-e2e-lease-premise
created: 2026-05-31
tags: [testing, infra, portal]
---

The router e2e suite assumes a REST session request acquires the per-session
Postgres advisory lease, but it does not. The portal's `LifecycleManager`
(`pg_try_advisory_lock(hashtext(sessionID))`, held persistently) is wired ONLY
into the git smart-HTTP handler (`cmd/portal/main.go`:
`gitHandler.Emitter.Lifecycle = objLifecycle`). REST handlers — `GetSession`
(`GET /api/orgs/{org}/sessions/{id}`) and session creation (POST) — read/write
Postgres directly, never take the advisory lock, and never return
`503 dep.lease_unavailable` on contention (only the git/objectstore path does).
Consequently every router test anchored to `cluster.LeaseHolder` (which queries
`pg_locks` for that advisory lock) gets `-1` ("no holder"), and the
503-redispatch tests that hold the advisory lock from the test process never
actually induce a 503 on a read GET. Affected:
`tests/e2e/golden/router_consistent_hash_test.go` (both subtests),
`tests/e2e/golden/router_hint_cache_test.go` (`hint_cache_is_per_session`; the
`hint_cache_overrides_ring_after_503` pre-flight also has a separate
metric-ordering bug — it scrapes `jamsesh_router_decisions_total` before any
proxy traffic, and a zero-cardinality Prometheus CounterVec exports no series
until the first increment), `tests/e2e/failure/router_lease_unavailable_test.go`
(`bounded_retry` premise; `transparent_redispatch` is tautological — a read GET
always returns 200), and `tests/e2e/failure/router_backend_dead_test.go`
(`RequireLeaseHolder`). The correct fix re-anchors these tests to a routing
signal the router actually produces without depending on the portal lease:
drive git push/clone through the router (which DO acquire the lease and 503 on
contention — overlaps the separately-red cross-pod git-serving path), use a
per-pod response identity header, or assert on the router's
`jamsesh_router_decisions_total` counter scraped AFTER traffic flows. The router
product code (consistent-hash ring, hint cache, redispatch, metrics) is correct;
this is an e2e-harness/portal-lease-model mismatch. Parent epic:
`e2e-cloud-native-multipod-suite-red`.
